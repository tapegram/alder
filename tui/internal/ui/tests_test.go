package ui

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/charmbracelet/x/exp/teatest"

	"alder/tui/internal/runner"
)

// The lines of `alder test --json` the Tests screen's fake runner answers with.
const (
	passLend    = `{"module":"Books","name":"lend a book","file":"app/Books.al","line":10,"col":1,"ok":true,"trace":[],"ms":1.2}`
	failTwice   = `{"module":"Books","name":"lend (twice)","file":"app/Books.al","line":20,"col":1,"ok":false,"left":"Lent 2","right":"Lent 1","note":"a book lends once","trace":["Books.lend app/Books.al:14","Books.update app/Books.al:31"],"ms":3.4}`
	passTwice   = `{"module":"Books","name":"lend (twice)","file":"app/Books.al","line":20,"col":1,"ok":true,"trace":[],"ms":2.9}`
	failProp    = `{"module":"Search","name":"finds by title","file":"app/Search.al","line":5,"col":1,"ok":false,"left":"Nothing","right":"Just 1","trace":[],"ms":40.5,"property":{"runs":100,"seed":4242,"run":37,"shrinks":3,"args":[{"name":"title","type":"String","value":"\"\""}],"case":"finds by title \"\""}}`
	passEmpty   = `{"module":"Search","name":"empty query","file":"app/Search.al","line":30,"col":1,"ok":true,"trace":[],"ms":0.8}`
	passProp    = `{"module":"Search","name":"finds by title","file":"app/Search.al","line":5,"col":1,"ok":true,"trace":[],"ms":38.0,"property":{"runs":100,"seed":4242,"run":0,"shrinks":0,"args":[]}}`
	mutantsLine = `{"mutants":[` +
		`{"id":1,"module":"Books","file":"app/Books.al","decl":"lend","line":14,"col":9,"kind":"arith","before":"n + 1","after":"n - 1","killed":true,"by":"Books \"lend a book\"","note":""},` +
		`{"id":2,"module":"Books","file":"app/Books.al","decl":"lend","line":16,"col":5,"kind":"cond","before":"n > 0","after":"n >= 0","killed":false,"by":"","note":""}]}`
	scoreLine = `{"score":0.5,"killed":1,"survived":1,"suppressed":2,"skipped":3}`
)

// suite is the suite's run: four tests, two of them failing, one a property.
var suite = strings.Join([]string{passLend, failTwice, failProp, passEmpty}, "\n")

// testsOptions is options() with a fake runner that answers the suite, and dir as the app's
// directory when it is set.
func testsOptions(dir string) (Options, *runner.Fake) {
	f := &runner.Fake{}
	f.On("test --json", suite, 1)
	o := options(nil)
	o.Runner = f
	if dir != "" {
		o.Dir = dir
	}
	return o, f
}

func TestTests_Screen(t *testing.T) {
	o, f := testsOptions("")
	m := newRoot(o, fixture(t, "basic")...)
	if len(f.Calls()) != 0 {
		t.Fatalf("the suite ran before the Tests tab was opened: %q", f.Calls())
	}
	press(m, "4")
	if want := []string{"test --json"}; !reflect.DeepEqual(f.Calls(), want) {
		t.Fatalf("opening Tests called %q, want %q", f.Calls(), want)
	}
	frame := m.View()
	lines := strings.Split(frame, "\n")
	if want := " 2 passed, 2 failed  0.05s"; lines[1] != want {
		t.Errorf("summary = %q, want %q", lines[1], want)
	}
	for _, want := range []string{
		`Books   "lend a book"     1.2ms   ` + "\x1b[32mok\x1b[0m",
		`Books   "lend (twice)"    3.4ms   ` + "\x1b[1;31mFAIL\x1b[0m",
		`Search  "finds by title"  40.5ms  ` + "\x1b[1;31mFAIL\x1b[0m",
		`Search  "empty query"     0.8ms   ` + "\x1b[32mok\x1b[0m",
	} {
		if rowOf(frame, want) == "" {
			t.Errorf("no row %q in\n%s", want, frame)
		}
	}
	if !strings.HasSuffix(lines[len(lines)-1], "\x1b[2malder test --json\x1b[0m") {
		t.Errorf("footer = %q", lines[len(lines)-1])
	}
	// Leaving and coming back does not run the suite again; r does.
	press(m, "1", "4")
	if len(f.Calls()) != 1 {
		t.Errorf("reopening Tests ran the suite again: %q", f.Calls())
	}
	press(m, "r")
	if len(f.Calls()) != 2 {
		t.Errorf("r does not run the suite: %q", f.Calls())
	}
	teatest.RequireEqualOutput(t, []byte(frame))
}

func TestTests_Unfold(t *testing.T) {
	o, f := testsOptions("")
	one := `test --json -run ^Books "lend \(twice\)"$`
	f.On(one, passTwice, 0)
	m := newRoot(o, fixture(t, "basic")...)
	press(m, "4", "enter")
	if strings.Contains(m.View(), "left:") {
		t.Errorf("enter on a passing test unfolds it:\n%s", m.View())
	}
	press(m, "j", "enter")
	frame := m.View()
	for _, want := range []string{"left:  Lent 2", "right: Lent 1", "note:  a book lends once", "trace:", "Books.lend app/Books.al:14"} {
		if !strings.Contains(frame, want) {
			t.Errorf("the unfolded failure has no %q:\n%s", want, frame)
		}
	}
	if len(f.Calls()) != 1 {
		t.Errorf("unfolding ran a test: %q", f.Calls())
	}
	teatest.RequireEqualOutput(t, []byte(frame))
	// enter again reruns only that test, whose new result replaces the old one in the list.
	press(m, "enter")
	if want := []string{"test --json", one}; !reflect.DeepEqual(f.Calls(), want) {
		t.Fatalf("calls %q, want %q", f.Calls(), want)
	}
	after := m.View()
	if row := rowOf(after, `"lend (twice)"`); !strings.Contains(row, "2.9ms") || !strings.Contains(row, "\x1b[32mok\x1b[0m") {
		t.Errorf("the rerun's result is not in the list: %q", row)
	}
	if strings.Split(after, "\n")[1] != " 3 passed, 1 failed  0.05s" {
		t.Errorf("summary after the rerun = %q", strings.Split(after, "\n")[1])
	}
	if want := `alder test --json -run '^Books "lend \(twice\)"$'`; !strings.Contains(wideFooter(m), want) {
		t.Errorf("footer = %q, want %q", wideFooter(m), want)
	}
}

func TestTests_SavedCase(t *testing.T) {
	dir := t.TempDir()
	o, _ := testsOptions(dir)
	m := newRoot(o, fixture(t, "basic")...)
	press(m, "4", "j", "j", "enter")
	frame := m.View()
	if want := `property: runs 100 seed 4242 shrinks 3 case finds by title ""`; !strings.Contains(frame, want) {
		t.Errorf("no %q in\n%s", want, frame)
	}
	if strings.Contains(frame, "replays saved case") {
		t.Errorf("replays saved case without the saved file:\n%s", frame)
	}
	sum := sha1.Sum([]byte("finds by title"))
	saved := filepath.Join(dir, "build", "shelf", "fuzz", "Search", hex.EncodeToString(sum[:])[:16])
	if err := os.MkdirAll(filepath.Dir(saved), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(saved, []byte("1 2 3"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(m.View(), "replays saved case") {
		t.Errorf("no replays saved case with %s present:\n%s", saved, m.View())
	}
}

func TestTests_WatchQueues(t *testing.T) {
	o, f := testsOptions(t.TempDir())
	m := newRoot(o)
	s := m.screens[3].(*testsScreen)
	_, cmd := m.Update(key("4"))
	m.Update(key("w")) // the watch's next signal is read by the command dropped here
	if !s.watching {
		t.Fatal("w does not turn the watch on")
	}
	started := cmd()
	if len(f.Calls()) != 1 {
		t.Fatalf("calls %q", f.Calls())
	}
	_, cmd = m.Update(started)
	// Two changes arrive while the run is in flight: they queue one rerun and kill nothing.
	m.Update(testsChangeMsg{id: s.watchID})
	m.Update(testsChangeMsg{id: s.watchID})
	if !s.running || !s.queued {
		t.Fatalf("a change during the run: running %v queued %v", s.running, s.queued)
	}
	if len(f.Calls()) != 1 {
		t.Fatalf("a change during the run started another: %q", f.Calls())
	}
	run(m, cmd())
	if want := []string{"test --json", "test --json"}; !reflect.DeepEqual(f.Calls(), want) {
		t.Errorf("calls %q, want exactly one more after the run ended", f.Calls())
	}
	if s.running || s.queued {
		t.Errorf("after the rerun: running %v queued %v", s.running, s.queued)
	}
	m.Update(key("w"))
	if s.watching {
		t.Error("w does not turn the watch off")
	}
}

func TestTests_Mutate(t *testing.T) {
	o, f := testsOptions("")
	f.On("test --json --mutate", strings.Join([]string{passLend, passTwice, passProp, passEmpty, mutantsLine, scoreLine}, "\n"), 0)
	m := newRoot(o, fixture(t, "basic")...)
	press(m, "4", "m")
	if want := []string{"test --json", "test --json --mutate"}; !reflect.DeepEqual(f.Calls(), want) {
		t.Fatalf("calls %q, want %q", f.Calls(), want)
	}
	frame := m.View()
	for _, want := range []string{
		"score 0.50  killed 1  survived 1  suppressed 2  skipped 3",
		"app/Books.al:16  n > 0 → n >= 0",
	} {
		if !strings.Contains(frame, want) {
			t.Errorf("no %q in\n%s", want, frame)
		}
	}
	if strings.Contains(frame, "n + 1 → n - 1") {
		t.Errorf("a killed mutant is listed:\n%s", frame)
	}
	if !strings.Contains(wideFooter(m), "alder test --json --mutate") {
		t.Errorf("footer = %q", wideFooter(m))
	}
	teatest.RequireEqualOutput(t, []byte(frame))
}

// pressViewing presses keys as the program does, drawing the frame after each one, and returns
// the last frame.
func pressViewing(m *root, keys ...string) string {
	frame := m.View()
	for _, k := range keys {
		press(m, k)
		frame = m.View()
	}
	return frame
}

// js is n presses of j.
func js(n int) []string {
	keys := make([]string, n)
	for i := range keys {
		keys[i] = "j"
	}
	return keys
}

func TestTests_ScrollPastList(t *testing.T) {
	o, f := testsOptions("")
	var lines []string
	for i := range 30 {
		lines = append(lines, strings.Replace(passLend, "lend a book", "lend book "+strconv.Itoa(i), 1))
	}
	f.On("test --json --mutate", strings.Join(append(lines, mutantsLine, scoreLine), "\n"), 0)
	long := strings.Replace(failTwice, `"trace":["Books.lend app/Books.al:14","Books.update app/Books.al:31"]`,
		`"trace":["t1","t2","t3","t4","t5","t6","t7","t8","t9","t10","t11","t12","t13","t14","t15","t16","t17","t18","t19","t20","t21","t22","t23","t24","t25"]`, 1)
	f.On("test --json", strings.Join(append(lines[:29:29], long), "\n"), 1)
	m := newRoot(o, fixture(t, "basic")...)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	press(m, "4", "m")
	frame := m.View()
	if strings.Contains(frame, "score 0.50") {
		t.Fatalf("the mutation block fits at 80x24 already; the test needs a longer list:\n%s", frame)
	}
	// j through the list reaches the last row, under which the mutation block now shows.
	frame = pressViewing(m, js(30)...)
	for _, want := range []string{"score 0.50  killed 1  survived 1  suppressed 2  skipped 3", "app/Books.al:16  n > 0 → n >= 0"} {
		if !strings.Contains(frame, want) {
			t.Errorf("no %q after moving to the last row:\n%s", want, frame)
		}
	}
	if !strings.Contains(wideFooter(m), "alder test --json --mutate") {
		t.Errorf("footer = %q", wideFooter(m))
	}
	if cur := cursorRow(pressViewing(m, "k")); !strings.Contains(cur, "lend book 28") {
		t.Errorf("k from the last row puts the cursor on %q, want lend book 28", cur)
	}

	// An unfolded trace longer than the pane on the last row: j scrolls to its tail, and further
	// j stays there.
	press(m, "r")
	frame = pressViewing(m, append(js(29), "enter")...)
	if !strings.Contains(frame, "left:  Lent 2") || strings.Contains(frame, "t25") {
		t.Fatalf("the unfolded failure is not shown from its top:\n%s", frame)
	}
	frame = pressViewing(m, js(30)...)
	if !strings.Contains(frame, "    t25") {
		t.Errorf("the trace's tail is unreachable:\n%s", frame)
	}
	// k scrolls back up the trace to the row before it moves the cursor.
	frame = pressViewing(m, "k", "k", "k", "k", "k", "k", "k", "k", "k", "k", "k")
	if cur := cursorRow(frame); !strings.Contains(cur, `"lend (twice)"`) || !strings.Contains(frame, "left:  Lent 2") {
		t.Errorf("k does not scroll back to the unfolded row:\n%s", frame)
	}
	if cur := cursorRow(pressViewing(m, "k")); !strings.Contains(cur, "lend book 28") {
		t.Errorf("k from the top of the block puts the cursor on %q, want lend book 28", cur)
	}
}

func TestTests_RerunKeepsScore(t *testing.T) {
	o, f := testsOptions("")
	diag := `{"diagnostics":[{"code":"W1","severity":"warning","file":"app/Books.al","line":3,"col":1,"message":"unused import"}]}`
	f.On("test --json --mutate", strings.Join([]string{diag, passLend, failTwice, passProp, passEmpty, mutantsLine, scoreLine}, "\n"), 1)
	f.On(`test --json -run ^Books "lend \(twice\)"$`, passTwice, 0)
	m := newRoot(o, fixture(t, "basic")...)
	press(m, "4", "m", "j", "enter", "enter")
	if want := []string{"test --json", "test --json --mutate", `test --json -run ^Books "lend \(twice\)"$`}; !reflect.DeepEqual(f.Calls(), want) {
		t.Fatalf("calls %q, want %q", f.Calls(), want)
	}
	frame := m.View()
	for _, want := range []string{
		"score 0.50  killed 1  survived 1  suppressed 2  skipped 3",
		"app/Books.al:16  n > 0 → n >= 0",
		"app/Books.al:3:1  unused import",
	} {
		if !strings.Contains(frame, want) {
			t.Errorf("no %q after rerunning one test:\n%s", want, frame)
		}
	}
	if row := rowOf(frame, `"lend (twice)"`); !strings.Contains(row, "2.9ms") {
		t.Errorf("the rerun's result is not in the list: %q", row)
	}
}

func TestTests_Empty(t *testing.T) {
	o, f := testsOptions("")
	f.On("test --json", "", 0)
	m := newRoot(o, fixture(t, "basic")...)
	press(m, "4")
	if frame := m.View(); !strings.Contains(frame, "no tests") {
		t.Errorf("an empty run does not say no tests:\n%s", frame)
	}
	// enter on an empty list does nothing, and the footer still offers unfold.
	press(m, "enter")
	if len(f.Calls()) != 1 {
		t.Errorf("enter on an empty list ran %q", f.Calls())
	}
	if k := m.screens[3].Keys()[0]; k != (Key{"enter", "unfold"}) {
		t.Errorf("enter key on an empty list = %+v", k)
	}
}

func TestTests_Error(t *testing.T) {
	t.Run("stderr", func(t *testing.T) {
		o, f := testsOptions("")
		f.OnStderr("test --json", "alder: no app.al in /Users/dev/shelf\n", 1)
		m := newRoot(o, fixture(t, "basic")...)
		press(m, "4")
		if frame := m.View(); !strings.Contains(frame, "alder: no app.al in /Users/dev/shelf") {
			t.Errorf("a run that failed with only stderr does not show it:\n%s", frame)
		}
	})
	t.Run("missing binary", func(t *testing.T) {
		o, _ := testsOptions("")
		o.Runner, o.Dir = runner.Exec("/nonexistent/alder"), t.TempDir()
		m := newRoot(o, fixture(t, "basic")...)
		press(m, "4")
		if frame := m.View(); !strings.Contains(frame, "/nonexistent/alder") || strings.Contains(frame, "running") {
			t.Errorf("a missing alder does not show why:\n%s", frame)
		}
	})
}

func TestTests_StopCancels(t *testing.T) {
	// streaming is a root whose Tests screen has its suite's stream open, not yet read.
	streaming := func(t *testing.T) (*root, *testsScreen) {
		o, _ := testsOptions("")
		m := newRoot(o, fixture(t, "basic")...)
		s := m.screens[3].(*testsScreen)
		_, cmd := m.Update(key("4"))
		m.Update(cmd())
		if s.stream == nil {
			t.Fatal("no stream open after opening Tests")
		}
		return m, s
	}
	canceled := func(t *testing.T, st runner.Stream) {
		t.Helper()
		if _, _, err := st.Wait(); !errors.Is(err, context.Canceled) {
			t.Errorf("the stream's Wait = %v, want context.Canceled", err)
		}
	}
	t.Run("Stop", func(t *testing.T) {
		_, s := streaming(t)
		st := s.stream
		s.Stop()
		canceled(t, st)
	})
	for _, k := range []string{"q", "ctrl+c"} {
		t.Run(k, func(t *testing.T) {
			m, s := streaming(t)
			st := s.stream
			_, cmd := m.Update(key(k))
			if cmd == nil {
				t.Fatalf("%s does not quit", k)
			}
			if _, ok := cmd().(tea.QuitMsg); !ok {
				t.Fatalf("%s does not quit", k)
			}
			canceled(t, st)
		})
	}
}
