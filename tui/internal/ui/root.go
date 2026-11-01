// Package ui is the TUI's Bubble Tea program: the root model, which holds the stores and the
// state of the dev server, and draws the status bar, the current screen, the footer and the
// overlays; and the screens, one per tab.
package ui

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"alder/tui/internal/feed"
	"alder/tui/internal/look"
	"alder/tui/internal/runner"
	"alder/tui/internal/store"
)

// The TUI keeps the last logLines lines of the log scrollback and the last requestRows answered
// requests.
const (
	logLines    = 20000
	requestRows = 5000
)

// restartTimeout bounds the POST /_dev/restart that R sends.
const restartTimeout = 5 * time.Second

// Options is what the program is given: the app's name and directory, the dev server's address,
// alder-tui's own version and alder's (from `alder help --json`), the dev server's feed, the
// lines an owned dev server printed before its feed answered, the runner for alder subprocesses,
// the HTTP client for the dev server's endpoints, the clock, whether the TUI owns the dev server,
// and how to open a URL in the browser and copy text to the clipboard.
type Options struct {
	App, Dir, Addr, Version, AlderVersion string
	Events                                <-chan feed.Event
	Early                                 <-chan string
	Runner                                runner.Runner
	HTTP                                  *http.Client
	Now                                   func() time.Time
	Owned                                 bool
	OpenURL                               func(string) error
	Copy                                  func(string) error
}

// Ctx is the state the screens share: the options, the stores, the last hello, the last build
// event of the dev server's current boot and the build state, the app's exit when it has exited,
// whether the feed is disconnected, and the active filters of the Logs and Requests screens.
type Ctx struct {
	Opts         Options
	Logs         *store.Logs
	Requests     *store.Requests
	Hello        *feed.Hello
	Build        *feed.Build
	State        string
	Exited       *feed.App
	Disconnected bool
	LogFilter    store.Filter
	ReqFilter    string
}

// Screen is one tab. Update gets every key the frame does not keep for itself and every other
// message; View draws the screen in width×height cells; Keys are the keys the footer lists for
// it; Command is the real command, shown right-aligned in the footer, that prints the same data.
type Screen interface {
	Title() string
	Update(msg tea.Msg, c *Ctx) (Screen, tea.Cmd)
	View(width, height int, c *Ctx) string
	Keys() []Key
	Command(c *Ctx) string
}

// Claimer is a Screen that can claim keys before the frame's global keys: a screen with an
// open detail (which owns tab, y, enter and esc) or an active text input (which owns every key
// but ctrl+c) claims them. ctrl+c always quits.
type Claimer interface {
	Claims(msg tea.KeyMsg) bool
}

// The messages the program sends itself: a feed event, the early lines of an owned dev server,
// the end of the feed, and a line for the footer (the outcome of a copy, an open or a restart).
type (
	eventMsg      feed.Event
	earlyMsg      []string
	feedClosedMsg struct{}
	flashMsg      string
)

// The overlays drawn over the current screen.
const (
	overlayNone = iota
	overlayKeys
	overlayProblems
)

// root is the program's model.
type root struct {
	ctx     *Ctx
	screens []Screen
	tab     int
	overlay int
	focus   bool   // the footer is focused, so y copies its command
	flash   string // a line shown in place of the footer's keys until the next key
	width   int
	height  int
	probs   problems
}

// New is the program's model for o. Its screens are Requests, Logs, Data, Tests and Routes.
func New(o Options) tea.Model {
	if o.HTTP == nil {
		o.HTTP = http.DefaultClient
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	return &root{
		ctx: &Ctx{Opts: o, Logs: store.NewLogs(logLines), Requests: store.NewRequests(requestRows),
			LogFilter: store.Filter{Level: "info"}},
		screens: []Screen{
			requestsScreen{},
			logsScreen{},
			placeholder{title: "Data", command: func(c *Ctx) string { return "sqlite3 build/" + appName(c) + "/dev.db" }},
			placeholder{title: "Tests", command: func(*Ctx) string { return "alder test --json" }},
			placeholder{title: "Routes", command: func(*Ctx) string { return "alder routes --json" }},
		},
	}
}

// Init reads the early lines of an owned dev server, then the feed.
func (m *root) Init() tea.Cmd {
	if m.ctx.Opts.Early != nil {
		return readEarly(m.ctx.Opts.Early)
	}
	return readEvent(m.ctx.Opts.Events)
}

// readEarly reads the early lines until their channel is closed.
func readEarly(ch <-chan string) tea.Cmd {
	return func() tea.Msg {
		var lines []string
		for l := range ch {
			lines = append(lines, l)
		}
		return earlyMsg(lines)
	}
}

// readEvent reads the feed's next event. The stores are touched only in Update, so the event is
// read here and applied there, and Update asks for the next one.
func readEvent(ch <-chan feed.Event) tea.Cmd {
	if ch == nil {
		return nil
	}
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return feedClosedMsg{}
		}
		return eventMsg(ev)
	}
}

func (m *root) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case earlyMsg:
		for _, l := range msg {
			m.ctx.Logs.Add(feed.Event{Type: "run", T: m.ctx.Opts.Now().UTC(), Run: &feed.Run{Text: l}})
		}
		return m, readEvent(m.ctx.Opts.Events)
	case eventMsg:
		m.apply(feed.Event(msg))
		cmd := m.toScreens(msg)
		return m, tea.Batch(cmd, readEvent(m.ctx.Opts.Events))
	case feedClosedMsg:
		return m, nil
	case flashMsg:
		m.flash = string(msg)
		return m, nil
	case tea.KeyMsg:
		return m, m.key(msg)
	}
	return m, m.toScreens(msg)
}

// apply records an event: a hello from another boot (another dev server) resets the stores and
// the build and exit it knew; the build state follows the hello, then each build event.
func (m *root) apply(e feed.Event) {
	c := m.ctx
	switch {
	case e.Type == "hello" && e.Hello != nil:
		if c.Hello != nil && e.Hello.Boot != c.Hello.Boot {
			c.Logs.Reset()
			c.Requests.Reset()
			c.Build, c.Exited = nil, nil
		}
		c.Hello, c.State, c.Disconnected = e.Hello, e.Hello.Build, false
	case e.Type == "disconnected":
		c.Disconnected = true
	case e.Type == "build" && e.Build != nil:
		c.Build, c.State = e.Build, e.Build.State
	case e.Type == "app" && e.App != nil:
		switch e.App.State {
		case "exited":
			c.Exited = e.App
		case "started":
			c.Exited = nil
		}
	}
	c.Logs.Add(e)
	c.Requests.Add(e)
}

// toScreens hands a message other than a key to every screen, so each follows the feed whether
// or not it is the current one.
func (m *root) toScreens(msg tea.Msg) tea.Cmd {
	var cmds []tea.Cmd
	for i, s := range m.screens {
		var cmd tea.Cmd
		m.screens[i], cmd = s.Update(msg, m.ctx)
		cmds = append(cmds, cmd)
	}
	return tea.Batch(cmds...)
}

// key routes a key: ctrl+c quits; an open overlay takes the keys next, then the focused footer,
// then a screen that claims the key; then the global keys; anything left goes to the current
// screen.
func (m *root) key(msg tea.KeyMsg) tea.Cmd {
	m.flash = ""
	k := msg.String()
	if k == "ctrl+c" {
		return tea.Quit
	}
	switch m.overlay {
	case overlayKeys:
		switch k {
		case "esc", "?":
			m.overlay = overlayNone
		case "q":
			return tea.Quit
		}
		return nil
	case overlayProblems:
		return m.problemsKey(k)
	}
	if m.focus {
		m.focus = false
		switch k {
		case "y":
			return m.copy(m.screens[m.tab].Command(m.ctx))
		case "esc", "F":
			return nil
		}
	}
	cur := m.screens[m.tab]
	if cl, ok := cur.(Claimer); ok && cl.Claims(msg) {
		return m.toCurrent(msg)
	}
	switch k {
	case "1", "2", "3", "4", "5":
		m.tab = int(k[0] - '1')
	case "tab":
		m.tab = (m.tab + 1) % len(m.screens)
	case "shift+tab":
		m.tab = (m.tab + len(m.screens) - 1) % len(m.screens)
	case "?":
		m.overlay = overlayKeys
	case "p":
		m.overlay, m.probs.cursor = overlayProblems, 0
	case "R":
		return m.restart()
	case "o":
		return m.open("http://" + m.ctx.Opts.Addr + "/")
	case "F":
		m.focus = true
	case "q":
		return tea.Quit
	default:
		return m.toCurrent(msg)
	}
	return nil
}

// toCurrent hands a message to the current screen only.
func (m *root) toCurrent(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	m.screens[m.tab], cmd = m.screens[m.tab].Update(msg, m.ctx)
	return cmd
}

// copy copies s to the clipboard and says so in the footer.
func (m *root) copy(s string) tea.Cmd {
	cp := m.ctx.Opts.Copy
	return func() tea.Msg {
		if cp == nil {
			return flashMsg(look.Palette.Yellow("copy: no clipboard"))
		}
		if err := cp(s); err != nil {
			return flashMsg(look.Palette.Yellow("copy: " + err.Error()))
		}
		return flashMsg("copied " + s)
	}
}

// open opens url in the browser; the footer shows a failure.
func (m *root) open(url string) tea.Cmd {
	op := m.ctx.Opts.OpenURL
	return func() tea.Msg {
		if op == nil {
			return flashMsg(look.Palette.Yellow("open: no browser"))
		}
		if err := op(url); err != nil {
			return flashMsg(look.Palette.Yellow("open: " + err.Error()))
		}
		return nil
	}
}

// restart asks the dev server to restart the app, POST http://<addr>/_dev/restart; the footer
// shows the outcome.
func (m *root) restart() tea.Cmd {
	client, url := m.ctx.Opts.HTTP, "http://"+m.ctx.Opts.Addr+"/_dev/restart"
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), restartTimeout)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
		if err != nil {
			return flashMsg(look.Palette.Yellow("restart: " + err.Error()))
		}
		resp, err := client.Do(req)
		if err != nil {
			return flashMsg(look.Palette.Yellow("restart: " + err.Error()))
		}
		resp.Body.Close()
		if resp.StatusCode/100 != 2 {
			return flashMsg(look.Palette.Yellow("restart: " + resp.Status))
		}
		return flashMsg("restart requested")
	}
}

// View draws the frame: the status bar, the current screen (or an overlay) inside a one-cell
// margin, a rule and the footer, exactly height lines.
func (m *root) View() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}
	bodyH := max(m.height-3, 0)
	inner := max(m.width-2, 0)
	var body string
	switch m.overlay {
	case overlayKeys:
		body = keysView(m.screens[m.tab])
	case overlayProblems:
		body = m.probs.view(m.ctx, bodyH)
	default:
		body = m.screens[m.tab].View(inner, bodyH, m.ctx)
	}
	lines := make([]string, 0, m.height)
	lines = append(lines, m.statusBar())
	bl := strings.Split(body, "\n")
	for i := range bodyH {
		l := ""
		if i < len(bl) && bl[i] != "" {
			l = ansi.Truncate(" "+bl[i], m.width, "…")
		}
		lines = append(lines, l)
	}
	lines = append(lines, " "+strings.Repeat("─", inner), m.footer())
	return strings.Join(lines[:min(len(lines), m.height)], "\n")
}

// appName is the app's name: the hello's, else the one in the options.
func appName(c *Ctx) string {
	if c.Hello != nil && c.Hello.App != "" {
		return c.Hello.App
	}
	return c.Opts.App
}

// placeholder is a tab whose screen comes in the next PR.
type placeholder struct {
	title   string
	command func(c *Ctx) string
}

func (p placeholder) Title() string                          { return p.title }
func (p placeholder) Update(tea.Msg, *Ctx) (Screen, tea.Cmd) { return p, nil }
func (p placeholder) View(int, int, *Ctx) string             { return "coming in the next PR" }
func (p placeholder) Keys() []Key                            { return nil }
func (p placeholder) Command(c *Ctx) string                  { return p.command(c) }

// requestsScreen is the Requests tab until its table arrives: the count of answered requests.
type requestsScreen struct{}

func (requestsScreen) Title() string                            { return "Requests" }
func (s requestsScreen) Update(tea.Msg, *Ctx) (Screen, tea.Cmd) { return s, nil }
func (requestsScreen) View(_, _ int, c *Ctx) string {
	return fmt.Sprintf("%d requests", len(c.Requests.All()))
}
func (requestsScreen) Keys() []Key         { return nil }
func (requestsScreen) Command(*Ctx) string { return "alder run · ALDER_LOG=debug" }

// logsScreen is the Logs tab until its scrollback view arrives: the count of lines at the level
// filter.
type logsScreen struct{}

func (logsScreen) Title() string                            { return "Logs" }
func (s logsScreen) Update(tea.Msg, *Ctx) (Screen, tea.Cmd) { return s, nil }
func (logsScreen) View(_, _ int, c *Ctx) string {
	return fmt.Sprintf("%d lines", len(c.Logs.View(c.LogFilter)))
}
func (logsScreen) Keys() []Key { return nil }
func (logsScreen) Command(c *Ctx) string {
	return "ALDER_LOG=" + c.LogFilter.Level + " alder run"
}
