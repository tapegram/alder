package ui

import (
	"bufio"
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/muesli/termenv"

	"alder/tui/internal/feed"
	"alder/tui/internal/store"
)

// TestMain forces the 16-colour ANSI profile, so that anything styled through lipgloss shows
// the same escapes in the goldens as on a terminal, and makes local time UTC, so that the
// Requests table shows the same times on every machine.
func TestMain(m *testing.M) {
	lipgloss.SetColorProfile(termenv.ANSI)
	time.Local = time.UTC
	os.Exit(m.Run())
}

// clock is the fixed time the tests inject as Options.Now.
var clock = time.Date(2026, 11, 1, 14, 2, 20, 0, time.UTC)

// fixture is the events of tui/testdata/feeds/<name>.ndjson, in order.
func fixture(t *testing.T, name string) []feed.Event {
	t.Helper()
	f, err := os.Open(filepath.Join("..", "..", "testdata", "feeds", name+".ndjson"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var evs []feed.Event
	sc := bufio.NewScanner(f)
	sc.Buffer(nil, 1<<20)
	for sc.Scan() {
		ev, err := feed.Decode(sc.Bytes())
		if err != nil {
			t.Fatal(err)
		}
		evs = append(evs, ev)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return evs
}

// options is the Options every test starts from: the app shelf on localhost:8080, alder-tui and
// alder both v0.15.0 (the version the fixtures' hello names), the fixed clock, and an events channel the test feeds.
func options(events chan feed.Event) Options {
	return Options{
		App: "shelf", Dir: "/Users/dev/shelf", Addr: "localhost:8080",
		Version: "v0.15.0", AlderVersion: "v0.15.0",
		Events:  events,
		Now:     func() time.Time { return clock },
		OpenURL: func(string) error { return nil },
		Copy:    func(string) error { return nil },
	}
}

// start runs the model of o in a 100×30 terminal and sends evs on its events channel.
func start(t *testing.T, o Options, events chan feed.Event, evs ...feed.Event) *teatest.TestModel {
	t.Helper()
	tm := teatest.NewTestModel(t, New(o), teatest.WithInitialTermSize(100, 30))
	for _, ev := range evs {
		events <- ev
	}
	return tm
}

// waitText waits until the program's output holds s.
func waitText(t *testing.T, tm *teatest.TestModel, s string) {
	t.Helper()
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool { return bytes.Contains(b, []byte(s)) },
		teatest.WithDuration(3*time.Second))
}

// finalFrame quits the program and returns its model's last frame.
func finalFrame(t *testing.T, tm *teatest.TestModel) (*root, []byte) {
	t.Helper()
	if err := tm.Quit(); err != nil {
		t.Fatal(err)
	}
	m := tm.FinalModel(t, teatest.WithFinalTimeout(3*time.Second)).(*root)
	return m, []byte(m.View())
}

func TestFrame_Basic(t *testing.T) {
	events := make(chan feed.Event, 64)
	tm := start(t, options(events), events, fixture(t, "basic")...)
	waitText(t, tm, "/search")
	_, frame := finalFrame(t, tm)
	lines := strings.Split(string(frame), "\n")
	if !strings.Contains(lines[0], "shelf  \x1b[32m● ok\x1b[0m  localhost:8080  alder v0.15.0") {
		t.Errorf("status bar = %q", lines[0])
	}
	if !strings.Contains(lines[0], "\x1b[1m1 Requests\x1b[0m") {
		t.Errorf("tab 1 is not the current one: %q", lines[0])
	}
	if !strings.HasSuffix(lines[len(lines)-1], "\x1b[2malder run · ALDER_LOG=debug\x1b[0m") {
		t.Errorf("footer = %q", lines[len(lines)-1])
	}
	if len(lines) != 30 {
		t.Errorf("frame has %d lines, want 30", len(lines))
	}
	teatest.RequireEqualOutput(t, frame)
}

func TestFrame_Building(t *testing.T) {
	events := make(chan feed.Event, 64)
	hello := fixture(t, "basic")[0]
	h := *hello.Hello
	h.Build = "building"
	hello.Hello = &h
	tm := start(t, options(events), events, hello)
	waitText(t, tm, "building")
	_, frame := finalFrame(t, tm)
	if !bytes.Contains(frame, []byte("\x1b[33m● building\x1b[0m")) {
		t.Errorf("no yellow building dot in\n%s", frame)
	}
	teatest.RequireEqualOutput(t, frame)
}

func TestFrame_Problems(t *testing.T) {
	events := make(chan feed.Event, 64)
	tm := start(t, options(events), events, fixture(t, "failed")...)
	waitText(t, tm, "failed (2)")
	tm.Type("p")
	waitText(t, tm, "type mismatch")
	_, frame := finalFrame(t, tm)
	for _, want := range []string{
		"\x1b[1mapp/Books.al:14:12:\x1b[0m \x1b[1;31merror[E0002]\x1b[0m: unknown name `findBook`",
		"\x1b[1mapp/Main.al:31:5:\x1b[0m \x1b[1;31merror[E0003]\x1b[0m: type mismatch: expected Html Msg, found String",
	} {
		if !bytes.Contains(frame, []byte(want)) {
			t.Errorf("no %q in\n%s", want, frame)
		}
	}
	teatest.RequireEqualOutput(t, frame)
}

func TestFrame_Exited(t *testing.T) {
	events := make(chan feed.Event, 64)
	tm := start(t, options(events), events, fixture(t, "exited")...)
	tm.Type("p")
	waitText(t, tm, "the app exited (code 2)")
	_, frame := finalFrame(t, tm)
	for _, want := range []string{
		"the app exited (code 2)",
		`migrating dev.db`,
		`migration 0003_loans.sql: table loans already exists`,
		"alder: the app stopped: migrations failed",
	} {
		if !bytes.Contains(frame, []byte(want)) {
			t.Errorf("no %q in\n%s", want, frame)
		}
	}
	teatest.RequireEqualOutput(t, frame)
}

func TestFrame_Mismatch(t *testing.T) {
	events := make(chan feed.Event, 64)
	o := options(events)
	o.Version = "v0.9.0"
	tm := start(t, o, events, fixture(t, "basic")[0])
	waitText(t, tm, "upgrade both")
	_, frame := finalFrame(t, tm)
	if !bytes.Contains(frame, []byte("\x1b[33malder-tui v0.9.0 with alder v0.15.0: upgrade both togeth…\x1b[0m")) {
		t.Errorf("no yellow mismatch warning in\n%s", frame)
	}
	teatest.RequireEqualOutput(t, frame)
}

// The mismatch is judged against the alder producing the feed once a hello has arrived, not the
// alder on PATH: a PATH alder of another version is no warning when the feed's alder matches.
func TestFrame_MismatchUsesHello(t *testing.T) {
	events := make(chan feed.Event, 64)
	o := options(events)
	o.AlderVersion = "v0.5.0" // the alder on PATH; the feed's is v0.15.0, as is the TUI
	tm := start(t, o, events, fixture(t, "basic")[0])
	waitText(t, tm, "alder v0.15.0")
	m, frame := finalFrame(t, tm)
	if bytes.Contains(frame, []byte("upgrade both together")) {
		t.Errorf("a warning although the feed's alder matches:\n%s", frame)
	}
	if got := versionPart(m.ctx); got != "alder v0.15.0" {
		t.Errorf("versionPart = %q, want the hello's version", got)
	}
}

// Before a hello arrives the version comes from `alder help --json`.
func TestVersionPart_BeforeHello(t *testing.T) {
	c := &Ctx{Opts: Options{Version: "v0.15.0", AlderVersion: "v0.5.0"}}
	if got := versionPart(c); !strings.Contains(got, "alder-tui v0.15.0 with alder v0.5.0") {
		t.Errorf("versionPart = %q, want the PATH alder's mismatch", got)
	}
}

func TestFrame_NewBootResets(t *testing.T) {
	events := make(chan feed.Event, 64)
	evs := fixture(t, "basic")
	tm := start(t, options(events), events, evs...)
	waitText(t, tm, "/search")
	hello := evs[0]
	h := *hello.Hello
	h.Boot, h.Gen = "a17e9b30", "a17e9b30-1"
	hello.Hello, hello.Seq = &h, 1
	events <- hello
	waitText(t, tm, "no requests yet")
	m, frame := finalFrame(t, tm)
	if n := len(m.ctx.Requests.All()); n != 0 {
		t.Errorf("the requests table kept %d requests across the new boot", n)
	}
	if n := len(m.ctx.Logs.View(m.ctx.LogFilter)); n != 0 {
		t.Errorf("the scrollback kept %d lines across the new boot", n)
	}
	teatest.RequireEqualOutput(t, frame)
}

func TestFrame_Restart(t *testing.T) {
	got := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got <- r.Method + " " + r.URL.Path
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()
	events := make(chan feed.Event, 64)
	o := options(events)
	o.Addr = srv.Listener.Addr().String()
	o.HTTP = srv.Client()
	tm := start(t, o, events, fixture(t, "basic")[0])
	tm.Type("R")
	select {
	case req := <-got:
		if req != "POST /_dev/restart" {
			t.Errorf("the dev server got %q, want POST /_dev/restart", req)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the dev server got no request")
	}
	finalFrame(t, tm)
}

// key is the key message for s: a named key (enter, esc, tab, shift+tab, ctrl+c) or runes.
func key(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "shift+tab":
		return tea.KeyMsg{Type: tea.KeyShiftTab}
	case "ctrl+c":
		return tea.KeyMsg{Type: tea.KeyCtrlC}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

// press sends each key to m in turn and runs the commands they return, feeding back the
// messages, as the program would.
func press(m *root, keys ...string) {
	for _, k := range keys {
		run(m, key(k))
	}
}

// run sends msg to m and runs the command it returns, feeding back its message.
func run(m *root, msg tea.Msg) {
	_, cmd := m.Update(msg)
	if cmd == nil {
		return
	}
	if next := cmd(); next != nil {
		if _, ok := next.(tea.QuitMsg); !ok {
			run(m, next)
		}
	}
}

// newRoot is the root model of o, sized 100×30, with evs applied.
func newRoot(o Options, evs ...feed.Event) *root {
	m := New(o).(*root)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	for _, ev := range evs {
		m.Update(eventMsg(ev))
	}
	return m
}

func TestKeys_Tabs(t *testing.T) {
	m := newRoot(options(nil))
	press(m, "3")
	if m.tab != 2 {
		t.Fatalf("after 3 the tab is %d", m.tab+1)
	}
	if !strings.Contains(m.View(), "coming in the next PR") {
		t.Errorf("the Data placeholder does not say it is coming:\n%s", m.View())
	}
	press(m, "tab", "tab", "tab")
	if m.tab != 0 {
		t.Errorf("tab from Routes goes to %d, want 1", m.tab+1)
	}
	press(m, "shift+tab")
	if m.tab != 4 {
		t.Errorf("shift+tab from Requests goes to %d, want 5", m.tab+1)
	}
}

// claimer is a screen that claims every key while it is set, as Request detail and a text input
// do, and records the keys it was given.
type claimer struct {
	placeholder
	claim bool
	got   []string
}

func (s *claimer) Claims(msg tea.KeyMsg) bool { return s.claim }

func (s *claimer) Update(msg tea.Msg, c *Ctx) (Screen, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		s.got = append(s.got, k.String())
	}
	return s, nil
}

func TestKeys_ScreenClaimsBeforeGlobals(t *testing.T) {
	m := newRoot(options(nil))
	s := &claimer{placeholder: placeholder{title: "Requests", command: func(*Ctx) string { return "" }}, claim: true}
	m.screens[0] = s
	press(m, "2", "p", "tab", "q")
	if m.tab != 0 || m.overlay != overlayNone {
		t.Errorf("a claimed key reached the globals: tab %d, overlay %d", m.tab+1, m.overlay)
	}
	if strings.Join(s.got, " ") != "2 p tab q" {
		t.Errorf("the screen got %q", s.got)
	}
	_, cmd := m.Update(key("ctrl+c"))
	if cmd == nil || cmd() != tea.Quit() {
		t.Error("ctrl+c does not quit while the screen claims keys")
	}
	s.claim = false
	press(m, "2")
	if m.tab != 1 {
		t.Errorf("an unclaimed 2 left the tab at %d", m.tab+1)
	}
}

func TestKeys_Overlay(t *testing.T) {
	m := newRoot(options(nil))
	press(m, "?")
	v := m.View()
	for _, want := range []string{"Keys", "problems", "restart the app", "quit"} {
		if !strings.Contains(v, want) {
			t.Errorf("the keys overlay has no %q:\n%s", want, v)
		}
	}
	press(m, "esc")
	if m.overlay != overlayNone {
		t.Error("esc does not close the keys overlay")
	}
}

func TestFooter_FocusCopiesCommand(t *testing.T) {
	var copied []string
	o := options(nil)
	o.Copy = func(s string) error { copied = append(copied, s); return nil }
	m := newRoot(o)
	press(m, "y")
	if len(copied) != 0 {
		t.Fatalf("y without the footer focused copied %q", copied)
	}
	press(m, "F", "y")
	if strings.Join(copied, "|") != "alder run · ALDER_LOG=debug" {
		t.Errorf("copied %q", copied)
	}
}

func TestProblems_EnterCopiesLocation(t *testing.T) {
	var copied []string
	o := options(nil)
	o.Copy = func(s string) error { copied = append(copied, s); return nil }
	m := newRoot(o, fixture(t, "failed")...)
	press(m, "p", "enter", "j", "enter")
	if strings.Join(copied, "|") != "app/Books.al:14:12|app/Main.al:31:5" {
		t.Errorf("copied %q", copied)
	}
}

func TestProblems_BuildText(t *testing.T) {
	m := newRoot(options(nil), feed.Event{Type: "build", Build: &feed.Build{State: "failed", Text: "go build: exit status 2\nmain.go:3: undefined: x"}})
	press(m, "p")
	v := m.View()
	for _, want := range []string{"go build: exit status 2", "main.go:3: undefined: x"} {
		if !strings.Contains(v, want) {
			t.Errorf("the Problems overlay has no %q:\n%s", want, v)
		}
	}
}

func TestStatus_Disconnected(t *testing.T) {
	m := newRoot(options(nil), fixture(t, "basic")[0], feed.Event{Type: "disconnected"})
	if !strings.Contains(m.View(), "\x1b[33mdisconnected\x1b[0m") {
		t.Errorf("no yellow disconnected in the status bar:\n%s", m.View())
	}
}

func TestKeys_OpenBrowser(t *testing.T) {
	var opened []string
	o := options(nil)
	o.OpenURL = func(u string) error { opened = append(opened, u); return nil }
	m := newRoot(o)
	press(m, "o")
	if strings.Join(opened, "|") != "http://localhost:8080/" {
		t.Errorf("opened %q", opened)
	}
}

// starting is the options of a TUI that starts its own dev server: Open prints lines, then waits
// for release before it returns the feed events, or err when it is set.
func starting(events chan feed.Event, release chan struct{}, err error, lines ...string) Options {
	o := options(nil)
	o.Open = func(line func(string)) (Feed, error) {
		for _, l := range lines {
			line(l)
		}
		<-release
		if err != nil {
			return Feed{}, err
		}
		return Feed{Events: events, Owned: true}, nil
	}
	return o
}

func TestFrame_Starting(t *testing.T) {
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	o := starting(nil, release, nil, "alder run: building shelf",
		"alder run: dev server on localhost:8080; the app starts once it builds")
	tm := teatest.NewTestModel(t, New(o), teatest.WithInitialTermSize(100, 30))
	tm.Send(key("2"))
	waitText(t, tm, "the app starts once it builds")
	m, frame := finalFrame(t, tm)
	if !strings.Contains(strings.Split(string(frame), "\n")[0], "shelf  \x1b[2m● starting\x1b[0m  localhost:8080") {
		t.Errorf("status bar = %q", strings.Split(string(frame), "\n")[0])
	}
	if !m.ctx.Starting {
		t.Error("Starting = false while Open has not returned")
	}
	teatest.RequireEqualOutput(t, frame)
}

func TestStart_EarlyLinesBeforeHello(t *testing.T) {
	events, release := make(chan feed.Event, 64), make(chan struct{})
	o := starting(events, release, nil, "alder run: building shelf")
	tm := teatest.NewTestModel(t, New(o), teatest.WithInitialTermSize(100, 30))
	tm.Send(key("2"))
	waitText(t, tm, "building shelf")
	close(release)
	for _, ev := range fixture(t, "basic") {
		events <- ev
	}
	waitText(t, tm, "panic: runtime error")
	m, _ := finalFrame(t, tm)
	all := m.ctx.Logs.View(store.Filter{})
	if len(all) < 2 || !all[0].Run || all[0].Text != "alder run: building shelf" || !strings.Contains(all[1].Text, "dev server on") {
		t.Errorf("the early line is not first in the scrollback: %+v", all)
	}
	if m.ctx.Starting || m.ctx.State != "ok" || !m.ctx.Opts.Owned {
		t.Errorf("after the hello Starting=%v State=%q Owned=%v", m.ctx.Starting, m.ctx.State, m.ctx.Opts.Owned)
	}
}

func TestStart_OpenFails(t *testing.T) {
	release := make(chan struct{})
	close(release)
	want := errors.New("alder run exited before its feed answered on localhost:8080:\nbuild failed")
	tm := teatest.NewTestModel(t, New(starting(nil, release, want, "build failed")),
		teatest.WithInitialTermSize(100, 30))
	fm := tm.FinalModel(t, teatest.WithFinalTimeout(3*time.Second))
	if err := StartErr(fm); err != want {
		t.Errorf("StartErr = %v, want %v", err, want)
	}
	if err := StartErr(New(options(nil))); err != nil {
		t.Errorf("StartErr of a model that has not failed = %v", err)
	}
}
