// Package ui is the TUI's Bubble Tea program: the root model, which holds the stores and the
// state of the dev server, and draws the status bar, the current screen, the footer and the
// overlays; and the screens, one per tab.
package ui

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"alder/tui/internal/devproc"
	"alder/tui/internal/feed"
	"alder/tui/internal/look"
	"alder/tui/internal/routes"
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
// alder-tui's own version and alder's (from `alder help --json`), the dev server's feed or how to
// open it, the runner for alder subprocesses, the HTTP client for the dev server's endpoints, the
// clock, whether the TUI owns the dev server, and how to open a URL in the browser and copy text
// to the clipboard.
//
// Open, when set, opens the dev server once the program has started, off the update loop, so the
// terminal is not blank while a dev server the TUI started builds the app. It is given a function
// that takes each line the dev server prints before its feed answers, which the Logs screen shows
// as they arrive, and returns the feed, or the error the program then quits with (see StartErr).
// Without Open the program reads Events.
type Options struct {
	App, Dir, Addr, Version, AlderVersion string
	Events                                <-chan feed.Event
	Open                                  func(line func(string)) (Feed, error)
	Runner                                runner.Runner
	HTTP                                  *http.Client
	Now                                   func() time.Time
	Owned                                 bool
	OpenURL                               func(string) error
	Copy                                  func(string) error
}

// Feed is the dev server's feed as Options.Open returns it, whether the TUI owns the dev server,
// and, when it does, the channel that carries its exit should it die (see devproc.Session.Exited).
type Feed struct {
	Events <-chan feed.Event
	Owned  bool
	Exited <-chan devproc.Exit
}

// Ctx is the state the screens share: the options, the stores, whether Options.Open has yet to
// return and its error when it failed, the last hello, the last build event of the dev server's
// current boot and the build state, the app's exit when it has exited, the owned dev server's
// exit when it has died (until a hello comes again), whether the feed is disconnected, the
// active filters of the Logs and Requests screens, the current tab, and the app's routes as the Routes screen last
// loaded them, with the route the Requests screen is filtered to (nil for none). RouteFilter is separate from ReqFilter, the text filter, and both apply.
type Ctx struct {
	Opts         Options
	Logs         *store.Logs
	Requests     *store.Requests
	Starting     bool
	StartErr     error
	Hello        *feed.Hello
	Build        *feed.Build
	State        string
	Exited       *feed.App
	RunExited    *devproc.Exit
	Disconnected bool
	LogFilter    store.Filter
	ReqFilter    string
	Routes       []routes.Route
	RouteFilter  *routes.Route
	Tab          int // the current tab, as of the end of the last Update
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

// Opener is a Screen that does something each time it becomes the current tab, such as the
// Tests screen, which runs the suite the first time.
type Opener interface {
	Open(c *Ctx) tea.Cmd
}

// Starter is a Screen that does something when the program starts, such as the Routes screen,
// which loads the routes so they are there when its tab is opened.
type Starter interface {
	Start(c *Ctx) tea.Cmd
}

// Stopper is a Screen that has something to end when the program quits, such as the Tests
// screen, which kills the run in flight so that no alder test outlives the program.
type Stopper interface {
	Stop()
}

// The messages the program sends itself: a feed event, the end of the feed, the exit of the
// owned dev server, and a line for the footer (the outcome of a copy, an open or a restart).
type (
	eventMsg      feed.Event
	feedClosedMsg struct{}
	runExitedMsg  devproc.Exit
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
	start   *startQueue // what Options.Open reports, while it runs
}

// New is the program's model for o. Its screens are Requests, Logs, Data, Tests and Routes.
func New(o Options) tea.Model {
	if o.HTTP == nil {
		o.HTTP = http.DefaultClient
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	m := &root{
		ctx: &Ctx{Opts: o, Logs: store.NewLogs(logLines), Requests: store.NewRequests(requestRows),
			Starting: o.Open != nil, LogFilter: store.Filter{Level: "info"}},
		screens: []Screen{
			newRequestsScreen(),
			newLogsScreen(),
			newDataScreen(),
			newTestsScreen(),
			newRoutesScreen(),
		},
	}
	if o.Open != nil {
		m.start = &startQueue{ready: make(chan struct{}, 1)}
	}
	return m
}

// StartErr is why the program of model m quit without a dev server: the error Options.Open
// returned, or nil.
func StartErr(m tea.Model) error {
	if r, ok := m.(*root); ok {
		return r.ctx.StartErr
	}
	return nil
}

// Stop stops every screen of the program's model m that is a Stopper, as quitting from a key
// does. main calls it once the program has returned, however it ended, so that a signal also
// leaves no alder test running. Stopping twice is harmless.
func Stop(m tea.Model) {
	if r, ok := m.(*root); ok {
		r.stopScreens()
	}
}

// Init opens the dev server when the options say how, reading what Open reports as it reports
// it; otherwise it reads the feed.
func (m *root) Init() tea.Cmd {
	var cmds []tea.Cmd
	for _, s := range m.screens {
		if st, ok := s.(Starter); ok {
			cmds = append(cmds, st.Start(m.ctx))
		}
	}
	if q := m.start; q != nil {
		open := m.ctx.Opts.Open
		return tea.Batch(append(cmds, func() tea.Msg {
			f, err := open(q.line)
			q.finish(f, err)
			return nil
		}, q.next())...)
	}
	return tea.Batch(append(cmds, readEvent(m.ctx.Opts.Events))...)
}

// startQueue carries what Options.Open reports to the update loop, in order: the lines the dev
// server prints, then Open's outcome. Open never waits on the loop to hand a line on, so it
// returns even once the program has quit and nothing reads the queue any more.
type startQueue struct {
	mu    sync.Mutex
	lines []string
	done  *startMsg
	ready chan struct{} // holds a token while there is something to read
}

// startMsg is what the queue held when it was read: the lines, then Open's outcome once it has
// returned (done is set).
type startMsg struct {
	lines []string
	done  bool
	feed  Feed
	err   error
}

func (q *startQueue) line(l string) {
	q.mu.Lock()
	q.lines = append(q.lines, l)
	q.mu.Unlock()
	q.signal()
}

func (q *startQueue) finish(f Feed, err error) {
	q.mu.Lock()
	q.done = &startMsg{done: true, feed: f, err: err}
	q.mu.Unlock()
	q.signal()
}

func (q *startQueue) signal() {
	select {
	case q.ready <- struct{}{}:
	default:
	}
}

// next waits for the queue to hold something and takes all of it.
func (q *startQueue) next() tea.Cmd {
	return func() tea.Msg {
		<-q.ready
		q.mu.Lock()
		defer q.mu.Unlock()
		msg := startMsg{lines: q.lines}
		if q.done != nil {
			msg.done, msg.feed, msg.err = true, q.done.feed, q.done.err
		}
		q.lines = nil
		return msg
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

// Update answers msg and then clamps the state View reads (the Problems cursor), so View never
// has to.
func (m *root) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	model, cmd := m.update(msg)
	m.ctx.Tab = m.tab
	m.probs.clamp(m.ctx)
	return model, cmd
}

func (m *root) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, m.toScreens(msg)
	case startMsg:
		return m, m.started(msg)
	case eventMsg:
		m.apply(feed.Event(msg))
		cmd := m.toScreens(msg)
		return m, tea.Batch(cmd, readEvent(m.ctx.Opts.Events))
	case feedClosedMsg:
		m.ctx.Disconnected = true
		return m, nil
	case runExitedMsg:
		m.runExited(devproc.Exit(msg))
		return m, m.toScreens(msg)
	case flashMsg:
		m.flash = string(msg)
		return m, nil
	case switchTabMsg:
		if msg.rid != "" {
			m.ctx.LogFilter.Rid = msg.rid
		}
		var told tea.Cmd
		if msg.route != nil {
			m.ctx.RouteFilter = msg.route
			told = m.toScreens(routeFilterMsg{})
		}
		prev := m.tab
		m.tab = msg.tab
		return m, tea.Batch(told, m.opened(prev))
	case tea.KeyMsg:
		prev := m.tab
		cmd := m.key(msg)
		return m, tea.Batch(cmd, m.opened(prev))
	}
	return m, m.toScreens(msg)
}

// started records what Options.Open reported: each line as a run line, then, once Open has
// returned, its feed, which the program reads from then on, or its error, which quits.
func (m *root) started(msg startMsg) tea.Cmd {
	c := m.ctx
	for _, l := range msg.lines {
		c.Logs.Add(feed.Event{Type: "run", T: c.Opts.Now().UTC(), Run: &feed.Run{Text: l}})
	}
	if !msg.done {
		return m.start.next()
	}
	m.start, c.Starting = nil, false
	if msg.err != nil {
		c.StartErr = msg.err
		return m.quit()
	}
	c.Opts.Events, c.Opts.Owned = msg.feed.Events, msg.feed.Owned
	return tea.Batch(readEvent(c.Opts.Events), readExit(msg.feed.Exited))
}

// readExit waits for the owned dev server's exit; a channel closed with no exit (the TUI stopped
// it) sends nothing.
func readExit(ch <-chan devproc.Exit) tea.Cmd {
	if ch == nil {
		return nil
	}
	return func() tea.Msg {
		if e, ok := <-ch; ok {
			return runExitedMsg(e)
		}
		return nil
	}
}

// runExited records the owned dev server's death: the status bar shows it, and the scrollback
// gets the stderr lines it printed last and then the exit, as errors.
func (m *root) runExited(e devproc.Exit) {
	c := m.ctx
	c.RunExited = &e
	var lines []string
	if e.StderrTail != "" {
		lines = strings.Split(e.StderrTail, "\n")
	}
	for _, l := range append(lines, runExitText(e)) {
		c.Logs.Add(feed.Event{Type: "log", T: c.Opts.Now().UTC(),
			Log: &feed.Log{Record: &feed.Record{Time: c.Opts.Now().UTC(), Level: "ERROR", Msg: l}}})
	}
}

// runExitText is the line that says the owned dev server died: `alder run exited (code N)`.
func runExitText(e devproc.Exit) string {
	return "alder run exited (code " + strconv.Itoa(e.Code) + ")"
}

// apply records an event: a hello from another boot (another dev server) resets the stores and
// the build and exit it knew, and any hello clears the dev server's death; the build state
// follows the hello, then each build event.
func (m *root) apply(e feed.Event) {
	c := m.ctx
	switch {
	case e.Type == "hello" && e.Hello != nil:
		if c.Hello != nil && e.Hello.Boot != c.Hello.Boot {
			c.Logs.Reset()
			c.Requests.Reset()
			c.Build, c.Exited = nil, nil
		}
		c.Hello, c.State, c.Disconnected, c.RunExited = e.Hello, e.Hello.Build, false, nil
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
		return m.quit()
	}
	switch m.overlay {
	case overlayKeys:
		switch k {
		case "esc", "?":
			m.overlay = overlayNone
		case "q":
			return m.quit()
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
		return m.quit()
	default:
		return m.toCurrent(msg)
	}
	return nil
}

// quit stops every screen that is a Stopper, then quits the program. Every way out from a key or
// the feed goes through it; a signal goes through main's call of Stop.
func (m *root) quit() tea.Cmd {
	m.stopScreens()
	return tea.Quit
}

// stopScreens calls Stop on every screen that is a Stopper.
func (m *root) stopScreens() {
	for _, s := range m.screens {
		if st, ok := s.(Stopper); ok {
			st.Stop()
		}
	}
}

// opened tells the current screen it has been opened, when it is an Opener and the tab was prev
// before.
func (m *root) opened(prev int) tea.Cmd {
	if m.tab == prev {
		return nil
	}
	if o, ok := m.screens[m.tab].(Opener); ok {
		return o.Open(m.ctx)
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
func (m *root) copy(s string) tea.Cmd { return copyText(m.ctx, s) }

// copyText copies s to the clipboard of c's options and says so in the footer.
func copyText(c *Ctx, s string) tea.Cmd {
	cp := c.Opts.Copy
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
func (m *root) open(url string) tea.Cmd { return openURL(m.ctx, url) }

// openURL opens url in the browser of c's options; the footer shows a failure.
func openURL(c *Ctx, url string) tea.Cmd {
	op := c.Opts.OpenURL
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
		body = m.probs.view(m.ctx, inner, bodyH)
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
