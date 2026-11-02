package ui

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"alder/tui/internal/look"
	"alder/tui/internal/runner"
	"alder/tui/internal/testrun"
	"alder/tui/internal/watch"
)

// The watch looks at these directories of the app, every watchEvery, and signals a change once
// the files have settled for watchDebounce.
var watchDirs = []string{"app", "adapters", "migrations"}

const (
	watchEvery    = 300 * time.Millisecond
	watchDebounce = 300 * time.Millisecond
)

// The messages of a run and of the watch: the stream of run gen has started (or could not), it
// gave a line, it has ended; and the watch with the given id saw a change.
type (
	testsStartedMsg struct {
		gen    int
		stream runner.Stream
		err    error
	}
	testsLineMsg struct {
		gen  int
		line string
	}
	testsDoneMsg struct {
		gen    int
		code   int
		stderr []byte
	}
	testsChangeMsg struct{ id int }
)

// testsScreen is the Tests tab: a summary line, a rule, and the list of results, one row per
// test (module, name, time, ok or FAIL), then a mutation run's score and its surviving mutants.
// The suite runs, `alder test --json`, when the tab is first opened and on r. enter on a failure
// unfolds it; enter on the unfolded failure reruns that test alone, and its new result replaces
// the old one in the list. w toggles the watch, which reruns the suite when the app's files
// change; m runs the mutation tests.
//
// A run is never killed: r or a change while a run is in flight queues one rerun of the suite
// for when it ends, and enter or m then do nothing.
type testsScreen struct {
	opened   bool
	run      *testrun.Run     // the last run, or the one in flight; nil before the first
	single   bool             // run reruns one test, so its result is merged into rows
	rows     []testrun.Result // the list
	gen      int              // the generation of run, so the messages of an older one are dropped
	running  bool
	queued   bool // a rerun of the suite waits for the run in flight to end
	stream   runner.Stream
	stderr   string // what the last run wrote to stderr, shown when it failed with nothing else to show
	cursor   int    // the row under the cursor
	unfolded string // the key of the unfolded failure, or ""
	top      int    // the first line of the list drawn

	watching bool
	watchID  int // counts the watches started, so a stopped one's signals are dropped
	stop     context.CancelFunc
	changes  <-chan struct{}
}

func newTestsScreen() *testsScreen { return &testsScreen{} }

func (*testsScreen) Title() string { return "Tests" }

// rowKey identifies a test in the list: its module and name.
func rowKey(r testrun.Result) string { return r.Module + "\x00" + r.Name }

// Open runs the suite the first time the tab is opened.
func (s *testsScreen) Open(c *Ctx) tea.Cmd {
	if s.opened {
		return nil
	}
	s.opened = true
	return s.start(c, nil, false)
}

// start starts a run of `alder test` with testrun.Args(one, mutate) and asks for its stream. The
// list starts over, unless the run is of one test.
func (s *testsScreen) start(c *Ctx, one *testrun.Result, mutate bool) tea.Cmd {
	s.gen++
	s.run = &testrun.Run{Args: testrun.Args(one, mutate)}
	s.single, s.running, s.stderr = one != nil, true, ""
	if one == nil {
		s.rows, s.cursor, s.unfolded, s.top = nil, 0, "", 0
	}
	gen, r, dir, args := s.gen, c.Opts.Runner, c.Opts.Dir, s.run.Args
	return func() tea.Msg {
		if r == nil {
			return testsStartedMsg{gen: gen, err: fmt.Errorf("no alder to run")}
		}
		st, err := r.Stream(context.Background(), dir, args...)
		return testsStartedMsg{gen: gen, stream: st, err: err}
	}
}

// nextLine reads the stream's next line, or once it has ended its exit code and stderr.
func nextLine(gen int, st runner.Stream) tea.Cmd {
	return func() tea.Msg {
		if l, ok := <-st.Lines(); ok {
			return testsLineMsg{gen: gen, line: l}
		}
		code, stderr := st.Wait()
		return testsDoneMsg{gen: gen, code: code, stderr: stderr}
	}
}

// waitChange waits for the watch's next signal; a closed channel (the watch was stopped) gives
// no message.
func waitChange(id int, ch <-chan struct{}) tea.Cmd {
	return func() tea.Msg {
		if _, ok := <-ch; !ok {
			return nil
		}
		return testsChangeMsg{id: id}
	}
}

func (s *testsScreen) Update(msg tea.Msg, c *Ctx) (Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case testsStartedMsg:
		if msg.gen != s.gen {
			return s, nil
		}
		if msg.err != nil {
			s.run.Error = msg.err.Error()
			return s, s.ended(c, -1)
		}
		s.stream = msg.stream
		return s, nextLine(msg.gen, msg.stream)
	case testsLineMsg:
		if msg.gen != s.gen {
			return s, nil
		}
		n := len(s.run.Results)
		testrun.Parse(msg.line, s.run)
		for _, r := range s.run.Results[n:] {
			s.add(r)
		}
		return s, nextLine(msg.gen, s.stream)
	case testsDoneMsg:
		if msg.gen != s.gen {
			return s, nil
		}
		s.stderr = strings.TrimSpace(string(msg.stderr))
		return s, s.ended(c, msg.code)
	case testsChangeMsg:
		if !s.watching || msg.id != s.watchID {
			return s, nil
		}
		wait := waitChange(s.watchID, s.changes)
		if s.running {
			s.queued = true
			return s, wait
		}
		return s, tea.Batch(wait, s.start(c, nil, false))
	case tea.KeyMsg:
		return s, s.key(msg.String(), c)
	}
	return s, nil
}

// add puts a result into the list: a rerun of one test replaces the row of that test, and any
// other result is appended.
func (s *testsScreen) add(r testrun.Result) {
	if s.single {
		for i := range s.rows {
			if rowKey(s.rows[i]) == rowKey(r) {
				s.rows[i] = r
				if r.OK && s.unfolded == rowKey(r) {
					s.unfolded = ""
				}
				return
			}
		}
	}
	s.rows = append(s.rows, r)
}

// ended records the end of the run with its exit code, and starts the queued rerun if there is
// one.
func (s *testsScreen) ended(c *Ctx, code int) tea.Cmd {
	s.run.Code, s.run.Done, s.running, s.stream = code, true, false, nil
	if s.queued {
		s.queued = false
		return s.start(c, nil, false)
	}
	return nil
}

// key answers a key: r runs the suite (or queues it behind the run in flight), m runs the
// mutation tests, w toggles the watch, j/k and the arrows move the cursor, and enter unfolds the
// failure under the cursor or, when it is unfolded, reruns it.
func (s *testsScreen) key(k string, c *Ctx) tea.Cmd {
	switch k {
	case "r":
		if s.running {
			s.queued = true
			return nil
		}
		return s.start(c, nil, false)
	case "m":
		if s.running {
			return nil
		}
		return s.start(c, nil, true)
	case "w":
		return s.toggleWatch(c)
	case "k", "up":
		s.cursor = max(s.cursor-1, 0)
	case "j", "down":
		s.cursor = max(min(s.cursor+1, len(s.rows)-1), 0)
	case "enter":
		if s.cursor >= len(s.rows) || s.rows[s.cursor].OK {
			return nil
		}
		r := s.rows[s.cursor]
		if s.unfolded != rowKey(r) {
			s.unfolded = rowKey(r)
			return nil
		}
		if s.running {
			return nil
		}
		return s.start(c, &r, false)
	}
	return nil
}

// toggleWatch starts the watch on the app's directory, or stops it.
func (s *testsScreen) toggleWatch(c *Ctx) tea.Cmd {
	if s.watching {
		s.watching = false
		s.stop()
		s.stop, s.changes = nil, nil
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.watching, s.stop = true, cancel
	s.watchID++
	s.changes = watch.Poll(ctx, c.Opts.Dir, watchDirs, watchEvery, watchDebounce)
	return waitChange(s.watchID, s.changes)
}

// View draws the summary, a rule, and the list, keeping the cursor's row in view.
func (s *testsScreen) View(width, height int, c *Ctx) string {
	head := []string{s.summary(), strings.Repeat("─", width)}
	body := s.body(c)
	h := max(height-len(head), 1)
	at := 0 // the line of the cursor's row in body
	for i, b := range body {
		if b.row == s.cursor && b.cursor {
			at = i
		}
	}
	s.top = max(min(s.top, at, len(body)-h), 0)
	if at >= s.top+h {
		s.top = at - h + 1
	}
	lines := head
	for _, b := range body[s.top:min(s.top+h, len(body))] {
		lines = append(lines, ansi.Truncate(b.text, width, "…"))
	}
	return strings.Join(lines, "\n")
}

// testsLine is a line of the list: its text, the row it belongs to (-1 for none), and whether it is
// the row's own line, which the cursor marks.
type testsLine struct {
	text   string
	row    int
	cursor bool
}

// body is the list's lines: the run's error or diagnostics, a row per test with its unfolded
// failure under it, and a mutation run's score and survivors; or what the screen is waiting for.
func (s *testsScreen) body(c *Ctx) []testsLine {
	var out []testsLine
	add := func(text string) { out = append(out, testsLine{text: text, row: -1}) }
	if s.run == nil {
		return []testsLine{{text: "  " + look.Palette.Dim("press r to run the tests"), row: -1}}
	}
	if s.run.Error != "" {
		add("  " + look.Mark("error") + " " + clean(s.run.Error))
	}
	for _, d := range s.run.Diagnostics {
		add(fmt.Sprintf("  %s %s:%d:%d  %s", look.Mark(d.Severity), clean(d.File), d.Line, d.Col, clean(d.Message)))
	}
	rows := make([][]string, len(s.rows))
	for i, r := range s.rows {
		mark := "FAIL"
		if r.OK {
			mark = "ok"
		}
		rows[i] = []string{clean(r.Module), clean(quoteName(r.Name)), strconv.FormatFloat(r.Ms, 'f', 1, 64) + "ms", look.Mark(mark)}
	}
	for i, text := range look.Columns(rows, 2) {
		mark := "  "
		if i == s.cursor {
			mark = "> "
		}
		out = append(out, testsLine{text: mark + text, row: i, cursor: true})
		if r := s.rows[i]; !r.OK && s.unfolded == rowKey(r) {
			for _, u := range s.unfold(r, c) {
				out = append(out, testsLine{text: "    " + u, row: i})
			}
		}
	}
	switch {
	case len(s.rows) == 0 && s.running:
		add("  " + look.Palette.Dim("running…"))
	case len(s.rows) == 0 && s.run.Error == "" && s.run.Diagnostics == nil && s.run.Done:
		if s.run.Code != 0 && s.stderr != "" {
			for _, l := range strings.Split(s.stderr, "\n") {
				add("  " + clean(l))
			}
		} else {
			add("  " + look.Palette.Dim("no tests"))
		}
	}
	if sc := s.run.Score; sc != nil {
		add("")
		add(look.Palette.Bold("Mutation"))
		add(fmt.Sprintf("  score %.2f  killed %d  survived %d  suppressed %d  skipped %d", sc.Score, sc.Killed, sc.Survived, sc.Suppressed, sc.Skipped))
		for _, m := range s.run.Mutants {
			if !m.Killed {
				add(fmt.Sprintf("  %s:%d  %s → %s", clean(m.File), m.Line, clean(m.Before), clean(m.After)))
			}
		}
	}
	return out
}

// unfold is a failure's detail: left, right, note and error when it has them, its trace, and a
// property's runs, seed, shrinks and case, with `replays saved case` when alder has saved the
// case under build/<app>/fuzz and will replay it first on the next run.
func (s *testsScreen) unfold(r testrun.Result, c *Ctx) []string {
	var out []string
	for _, f := range [][2]string{{"left:  ", r.Left}, {"right: ", r.Right}, {"note:  ", r.Note}, {"error: ", r.Error}} {
		if f[1] != "" {
			out = append(out, f[0]+clean(f[1]))
		}
	}
	if len(r.Trace) > 0 {
		out = append(out, "trace:")
		for _, t := range r.Trace {
			out = append(out, "  "+clean(t))
		}
	}
	if p := r.Property; p != nil {
		l := fmt.Sprintf("property: runs %d seed %d shrinks %d", p.Runs, p.Seed, p.Shrinks)
		if p.Case != "" {
			l += " case " + clean(p.Case)
		}
		out = append(out, l)
		if savedCase(c, r) {
			out = append(out, "replays saved case")
		}
	}
	return out
}

// savedCase is whether alder has saved the failing case of property test r, at
// build/<app>/fuzz/<Module>/<the first 16 hex digits of sha1(name)> (rt/testing's savedPath).
func savedCase(c *Ctx, r testrun.Result) bool {
	sum := sha1.Sum([]byte(r.Name))
	_, err := os.Stat(filepath.Join(c.Opts.Dir, "build", appName(c), "fuzz", r.Module, hex.EncodeToString(sum[:])[:16]))
	return err == nil
}

// quoteName is a test's name as alder's report prints it: in double quotes, escaped only when it
// holds a newline or a double quote (rt/testing's QuoteName).
func quoteName(name string) string {
	if strings.ContainsAny(name, "\n\"") {
		return strconv.Quote(name)
	}
	return "\"" + name + "\""
}

// summary is `N passed, M failed  Ts` over the list, the time the total of the tests' own, then
// `running` while a run is in flight, `rerun queued` when one waits, and `watching` while the
// watch is on.
func (s *testsScreen) summary() string {
	passed, failed, ms := 0, 0, 0.0
	for _, r := range s.rows {
		if r.OK {
			passed++
		} else {
			failed++
		}
		ms += r.Ms
	}
	parts := []string{fmt.Sprintf("%d passed, %d failed  %.2fs", passed, failed, ms/1000)}
	if s.running {
		parts = append(parts, look.Palette.Dim("running"))
	}
	if s.queued {
		parts = append(parts, look.Palette.Dim("rerun queued"))
	}
	if s.watching {
		parts = append(parts, "watching")
	}
	return strings.Join(parts, "  ")
}

func (s *testsScreen) Keys() []Key {
	enter := Key{"enter", "unfold"}
	if s.cursor < len(s.rows) && s.unfolded == rowKey(s.rows[s.cursor]) {
		enter = Key{"enter", "rerun"}
	}
	w := Key{"w", "watch"}
	if s.watching {
		w = Key{"w", "stop watch"}
	}
	return []Key{enter, {"r", "run"}, w, {"m", "mutate"}}
}

// Command is the command of the last run, `alder test --json` before the first, its arguments
// quoted for a POSIX shell where they need it.
func (s *testsScreen) Command(*Ctx) string {
	args := testrun.Args(nil, false)
	if s.run != nil {
		args = s.run.Args
	}
	return "alder " + shellJoin(args)
}

// shellJoin joins args with spaces, putting in single quotes each one that holds anything but
// letters, digits and `-_./=:^`, so the line pastes into a shell as the same arguments.
func shellJoin(args []string) string {
	out := make([]string, len(args))
	for i, a := range args {
		safe := a != ""
		for _, r := range a {
			if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("-_./=:^", r)) {
				safe = false
				break
			}
		}
		if safe {
			out[i] = a
		} else {
			out[i] = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
		}
	}
	return strings.Join(out, " ")
}
