package ui

import (
	"crypto/sha1"
	"encoding/hex"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

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
	if want := `alder test --json -run '^Books "lend \(twice\)"$'`; !strings.Contains(m.footer(), want) {
		t.Errorf("footer = %q, want %q", m.footer(), want)
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
	if !strings.Contains(m.footer(), "alder test --json --mutate") {
		t.Errorf("footer = %q", m.footer())
	}
	teatest.RequireEqualOutput(t, []byte(frame))
}
