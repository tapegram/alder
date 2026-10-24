package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"alder/internal/scratch"
)

func runScratch(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := runScratchTo(withBuildDir(t, args), &out, &errb)
	return code, out.String(), errb.String()
}

func writeScratch(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestScratch runs `alder scratch` on one temporary copy of examples/todos, from inside it
// (serial: t.Chdir). The cases that run the program share a file, so they share its build.
func TestScratch(t *testing.T) {
	needGoTool(t)
	tmp := t.TempDir()
	app := filepath.Join(tmp, "todos")
	copyDir(t, todosDir(t), app)
	os.RemoveAll(filepath.Join(app, "build"))
	t.Chdir(app)
	writeScratch(t, "scratch.alder", "-- scratch\n> 1 + 1\n> Todo.countOpen []\n")

	t.Run("Text", func(t *testing.T) {
		code, stdout, stderr := runScratch(t, "scratch.alder")
		if code != 0 || !strings.Contains(stdout, "scratch.alder:2: 1 + 1\n  ⇒ 2 : Int\n") {
			t.Errorf("exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
		}
	})
	t.Run("JSON", func(t *testing.T) {
		code, stdout, stderr := runScratch(t, "scratch.alder", "--json")
		var got struct {
			Watches []struct {
				Line, End int
				Value     string
				Type      string
				Kind      string
			}
			Diagnostics []json.RawMessage
			Ran         bool
		}
		if err := json.Unmarshal([]byte(stdout), &got); err != nil {
			t.Fatalf("%v\nstdout: %s", err, stdout)
		}
		if code != 0 || !got.Ran || len(got.Watches) != 2 || got.Watches[0].Value != "2" || got.Watches[0].Type != "Int" || got.Watches[0].Line != 2 || got.Watches[0].End != 2 || got.Watches[0].Kind != "pure" {
			t.Errorf("exit %d, got %+v\nstderr: %s", code, got, stderr)
		}
	})
	t.Run("DefaultFile", func(t *testing.T) {
		code, stdout, stderr := runScratch(t)
		if code != 0 || !strings.Contains(stdout, "scratch.alder:2: 1 + 1\n  ⇒ 2 : Int\n") {
			t.Errorf("exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
		}
	})
	t.Run("ErrorsExit1", func(t *testing.T) {
		writeScratch(t, "err.scratch.alder", "> 1 + \"x\"\n")
		code, stdout, stderr := runScratch(t, "err.scratch.alder")
		if code != 1 || !strings.Contains(stderr, "err.scratch.alder:1:3") {
			t.Errorf("exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
		}
		code, stdout, _ = runScratch(t, "err.scratch.alder", "--json")
		if code != 1 || !strings.Contains(stdout, `"ran":false`) {
			t.Errorf("json: exit %d, stdout %s", code, stdout)
		}
	})
	t.Run("HoleExits1", func(t *testing.T) {
		writeScratch(t, "hole.scratch.alder", "> 1 + ?x\n")
		code, stdout, stderr := runScratch(t, "hole.scratch.alder")
		if code != 1 || !strings.Contains(stderr, "W0002") || !strings.Contains(stderr, "typed hole") || !strings.Contains(stderr, "\nalder scratch: the file has 1 typed hole(s)") {
			t.Errorf("exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
		}
	})
	t.Run("EffectNotRun", func(t *testing.T) {
		writeScratch(t, "eff.scratch.alder", "> Db.from Todo |> Db.all\n")
		code, stdout, stderr := runScratch(t, "eff.scratch.alder")
		if code != 0 || !strings.Contains(stdout, "<Query>") || !strings.Contains(stdout, "(not run: scratch evaluates pure values only)") {
			t.Errorf("stdout: %s\nstderr: %s", stdout, stderr)
		}
	})
	t.Run("WatchErrorExits1", func(t *testing.T) {
		writeScratch(t, "boom.scratch.alder", "> 1 / 0\n> 2 + 2\n")
		code, stdout, stderr := runScratch(t, "boom.scratch.alder")
		if code != 1 || !strings.Contains(stdout, "  ✗ ") || !strings.Contains(stdout, "⇒ 4 : Int") {
			t.Errorf("exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
		}
	})
	t.Run("InitPanicExits1", func(t *testing.T) { // reuses boom's build directory
		writeScratch(t, "boom.scratch.alder", "boom : Int\nboom = 1 / 0\n\n> 1 + 1\n")
		code, stdout, stderr := runScratch(t, "boom.scratch.alder")
		if code != 1 || !strings.Contains(stdout, "  ✗ not run: the program failed before any watch ran: runtime error: integer divide by zero") {
			t.Errorf("exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
		}
	})
	t.Run("BuildDir", func(t *testing.T) {
		bd := t.TempDir()
		if code, _, stderr := runScratch(t, "scratch.alder", "--build-dir", bd); code != 0 {
			t.Fatalf("exit %d\nstderr: %s", code, stderr)
		}
		if m, _ := filepath.Glob(filepath.Join(bd, "todos", "scratch", "file-????????????")); len(m) != 1 {
			t.Errorf("no build under %s/todos/scratch/file-*: %v", bd, m)
		}
	})
	t.Run("Truncates", func(t *testing.T) {
		writeScratch(t, "big.scratch.alder", "> List.range 1 20000\n")
		code, stdout, stderr := runScratch(t, "big.scratch.alder")
		if code != 0 || !strings.HasSuffix(stdout, "\n"+scratchTruncatedLine+"\n") || len(stdout) < scratch.MaxValue {
			t.Errorf("exit %d, %d bytes of stdout, tail %q\nstderr: %s", code, len(stdout), stdout[max(0, len(stdout)-80):], stderr)
		}
		code, stdout, _ = runScratch(t, "big.scratch.alder", "--json")
		if code != 0 || !strings.Contains(stdout, `"truncated":true`) {
			t.Errorf("json: exit %d, truncated missing in %d bytes", code, len(stdout))
		}
	})
	t.Run("UsageErrorIsJSONWithJSONAnywhere", func(t *testing.T) {
		for _, args := range [][]string{{"--json", "--bogus"}, {"--bogus", "--json"}, {"a", "b", "--json"}} {
			code, stdout, stderr := runScratch(t, args...)
			var v scratchResult
			if code != 2 || stderr != "" || json.Unmarshal([]byte(stdout), &v) != nil || v.Error != scratchUsage {
				t.Errorf("%v: exit %d\nstdout: %s\nstderr: %s", args, code, stdout, stderr)
			}
		}
		if code, stdout, stderr := runScratch(t, "--bogus"); code != 2 || stdout != "" || stderr != scratchUsage+"\n" {
			t.Errorf("text: exit %d, stdout %q, stderr %q", code, stdout, stderr)
		}
	})
	t.Run("Interrupted", func(t *testing.T) { // Ctrl-C before the run finishes: exit 1, no results
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		var out, errb bytes.Buffer
		code := runScratchCtx(ctx, time.Second, withBuildDir(t, []string{"scratch.alder"}), &out, &errb)
		if code != 1 || out.String() != "" || !strings.Contains(errb.String(), "alder scratch: interrupted") {
			t.Errorf("exit %d\nstdout: %s\nstderr: %s", code, out.String(), errb.String())
		}
	})
	t.Run("AppDiagnosticKeepsItsFile", func(t *testing.T) { // on a private copy of the app
		priv := filepath.Join(t.TempDir(), "todos")
		copyDir(t, todosDir(t), priv)
		os.RemoveAll(filepath.Join(priv, "build"))
		orig, err := os.ReadFile(filepath.Join(priv, "app", "Todo.alder"))
		if err != nil {
			t.Fatal(err)
		}
		writeScratch(t, filepath.Join(priv, "app", "Todo.alder"), string(orig)+"\nbroken : Int\nbroken = \"x\"\n")
		writeScratch(t, filepath.Join(priv, "scratch.alder"), "> 1\n")
		code, _, stderr := runScratch(t, filepath.Join(priv, "scratch.alder"))
		if code != 1 || !strings.Contains(stderr, "app/Todo.alder:") || !strings.Contains(stderr, "broken = \"x\"") {
			t.Errorf("exit %d\nstderr: %s", code, stderr)
		}
	})
	t.Run("RefusesAppFile", func(t *testing.T) {
		writeScratch(t, "app/foo.scratch.alder", "> 1\n")
		defer os.Remove("app/foo.scratch.alder")
		code, _, stderr := runScratch(t, "app/foo.scratch.alder")
		want := "alder scratch: app/foo.scratch.alder is not a scratch file (a name ending in scratch.alder, outside app/)"
		if code != 2 || !strings.Contains(stderr, want) {
			t.Errorf("exit %d, stderr %s", code, stderr)
		}
	})
	t.Run("ModuleWithInHeader", func(t *testing.T) {
		writeScratch(t, "in.scratch.alder", "in Todo\n> 1\n")
		code, _, stderr := runScratch(t, "in.scratch.alder", "--module", "Probe")
		if code != 2 || !strings.Contains(stderr, "--in and --module do not combine") {
			t.Errorf("exit %d, stderr %s", code, stderr)
		}
		code, _, stderr = runScratch(t, "scratch.alder", "--module", "bad name")
		if code != 2 || !strings.Contains(stderr, "--module wants a module name") {
			t.Errorf("exit %d, stderr %s", code, stderr)
		}
	})
	t.Run("NoApp", func(t *testing.T) {
		empty := t.TempDir()
		writeScratch(t, filepath.Join(empty, "scratch.alder"), "> 1\n")
		code, _, stderr := runScratch(t, filepath.Join(empty, "scratch.alder"))
		if code != 2 || !strings.Contains(stderr, "alder scratch: no app found above") {
			t.Errorf("exit %d, stderr %s", code, stderr)
		}
	})
}

func TestScratchPrintsWatches(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		w    scratch.WatchResult
		want string
	}{
		{"multi-line", scratch.WatchResult{Line: 3, Expr: "f\n  x", Type: "Text", Value: "a\nb"}, "s:3: f\n  ⇒ a\n    b : Text\n"},
		{"truncated", scratch.WatchResult{Line: 1, Expr: "x", Type: "Text", Value: "a", Truncated: true}, "s:1: x\n  ⇒ a : Text\n  … (truncated at 64 KB)\n"},
		{"error", scratch.WatchResult{Line: 1, Expr: "x", Type: "Int", Error: "boom\nmore"}, "s:1: x\n  ✗ boom\n    more\n"},
		{"effect", scratch.WatchResult{Line: 1, Expr: "x", Type: "Query Int", Value: "<Query>", Note: "not run: scratch evaluates pure values only"}, "s:1: x\n  <Query> : Query Int   (not run: scratch evaluates pure values only)\n"},
		{"typed, not run", scratch.WatchResult{Line: 1, Expr: "x", Type: "Int"}, "s:1: x\n  : Int\n"},
		{"untyped, not run", scratch.WatchResult{Line: 1, Expr: "x"}, "s:1: x\n  (not run)\n"},
	}
	for _, c := range cases {
		var b bytes.Buffer
		printScratchWatch(&b, "s", c.w)
		if b.String() != c.want {
			t.Errorf("%s: got %q, want %q", c.name, b.String(), c.want)
		}
	}
}

// An app with a capability that has no adapter runs unwired: the warning goes to stderr, not stdout.
func TestScratchUnwiredWarning(t *testing.T) {
	t.Parallel()
	needGoTool(t)
	dir := tempTodos(t, "App.alder", "")
	if err := os.WriteFile(filepath.Join(dir, "app", "Mail.alder"), []byte(mailAlder), 0o644); err != nil {
		t.Fatal(err)
	}
	f := filepath.Join(dir, "scratch.alder")
	writeScratch(t, f, "> 1 + 1\n")
	code, stdout, stderr := runScratch(t, f)
	if code != 0 || !strings.HasSuffix(stdout, "  ⇒ 2 : Int\n") || stderr != unwiredWarning {
		t.Errorf("exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
}

func TestHelpJSON_Scratch(t *testing.T) {
	t.Parallel()
	for _, c := range commandTree {
		if c.Name != "scratch" {
			continue
		}
		have := map[string]bool{}
		for _, f := range c.Flags {
			have[f.Name] = true
		}
		for _, want := range []string{"--watch", "--json", "--build-dir", "--module"} {
			if !have[want] {
				t.Errorf("scratch lists no %s flag", want)
			}
		}
		if c.Usage != scratchUsage {
			t.Errorf("usage %q", c.Usage)
		}
		return
	}
	t.Error("scratch is not in commandTree")
}

// syncBuf is a bytes.Buffer safe for a watch run's writes and the test's reads.
type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// scratchWatcher runs `alder scratch --watch` on a scratch file in a copy of examples/todos,
// polling every 20 ms, until stop is called (which returns the exit code).
type scratchWatcher struct {
	t         *testing.T
	file      string
	out, errb *syncBuf
	cancel    func()
	done      chan int
	once      sync.Once
	code      int
	stamp     time.Time
}

func startScratchWatch(t *testing.T, text string, extra ...string) *scratchWatcher {
	t.Helper()
	needGoTool(t)
	app := filepath.Join(t.TempDir(), "todos")
	copyDir(t, todosDir(t), app)
	os.RemoveAll(filepath.Join(app, "build"))
	w := &scratchWatcher{t: t, file: filepath.Join(app, "scratch.alder"), out: &syncBuf{}, errb: &syncBuf{}, done: make(chan int, 1), stamp: time.Now()}
	writeScratch(t, w.file, text)
	ctx, cancel := context.WithCancel(context.Background())
	w.cancel = cancel
	args := withBuildDir(t, append([]string{"--watch", w.file}, extra...))
	go func() { w.done <- runScratchCtx(ctx, 20*time.Millisecond, args, w.out, w.errb) }()
	t.Cleanup(func() { w.stop() })
	return w
}

// edit rewrites the file with a modification time later than every earlier write.
func (w *scratchWatcher) edit(text string) {
	w.t.Helper()
	writeScratch(w.t, w.file, text)
	w.stamp = w.stamp.Add(2 * time.Second)
	os.Chtimes(w.file, w.stamp, w.stamp)
}

// waitFor waits until the stream holds want.
func (w *scratchWatcher) waitFor(b *syncBuf, want string) {
	w.t.Helper()
	deadline := time.Now().Add(90 * time.Second)
	for !strings.Contains(b.String(), want) {
		if time.Now().After(deadline) {
			w.t.Fatalf("no %q\nstdout: %s\nstderr: %s", want, w.out.String(), w.errb.String())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// stop cancels the run (Ctrl-C) and returns the exit code; it may be called again.
func (w *scratchWatcher) stop() int {
	w.once.Do(func() {
		w.cancel()
		w.code = <-w.done
	})
	return w.code
}

func TestScratchWatchReruns(t *testing.T) {
	w := startScratchWatch(t, "> 1\n")
	w.waitFor(w.out, "⇒ 1 : Int")
	w.edit("> 2\n")
	w.waitFor(w.out, "⇒ 2 : Int")
	if !strings.Contains(w.out.String(), "── ") {
		t.Errorf("no run header: %q", w.out.String())
	}
	if code := w.stop(); code != 0 {
		t.Errorf("exit %d after Ctrl-C, stderr: %s", code, w.errb.String())
	}
}

// A change during a run cancels it: its header stays, marked superseded, and the next run follows.
func TestScratchWatchMarksSupersededRun(t *testing.T) {
	w := startScratchWatch(t, "> 1\n")
	w.waitFor(w.out, "── ") // the first run, building from scratch, has started
	w.edit("> 2\n")
	w.waitFor(w.out, "  (superseded)\n")
	w.waitFor(w.out, "⇒ 2 : Int")
	if strings.Contains(w.out.String(), "⇒ 1 : Int") {
		t.Errorf("the superseded run's value was printed: %q", w.out.String())
	}
	if code := w.stop(); code != 0 {
		t.Errorf("exit %d, stderr: %s", code, w.errb.String())
	}
}

func TestScratchWatchSurvivesBadSave(t *testing.T) {
	w := startScratchWatch(t, "> 1\n")
	w.waitFor(w.out, "⇒ 1 : Int")
	w.edit("> 1 +\n")
	w.waitFor(w.errb, "scratch.alder:2:1: error")
	w.edit("> 3\n")
	w.waitFor(w.out, "⇒ 3 : Int")
	os.Remove(w.file)
	w.waitFor(w.errb, "is gone; waiting for it")
	if n := strings.Count(w.out.String(), "── "); n != 4 { // first run, bad save, good save, gone
		t.Errorf("%d run headers, want 4: %q", n, w.out.String())
	}
	if code := w.stop(); code != 0 {
		t.Errorf("exit %d, stderr: %s", code, w.errb.String())
	}
}

func TestScratchWatchJSONFailsWithOneLine(t *testing.T) {
	t.Parallel()
	missing := filepath.Join(t.TempDir(), "nope.scratch.alder")
	var out, errb bytes.Buffer
	code := runScratchTo([]string{"--watch", "--json", missing}, &out, &errb)
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if code != 2 || len(lines) != 1 || !strings.Contains(lines[0], `"error"`) {
		t.Errorf("exit %d, stdout %q", code, out.String())
	}
}

func TestScratchWatchJSONStreams(t *testing.T) {
	w := startScratchWatch(t, "> 1\n", "--json")
	w.waitFor(w.out, `"value":"1"`)
	w.edit("> 2\n")
	w.waitFor(w.out, `"value":"2"`)
	if code := w.stop(); code != 0 {
		t.Errorf("exit %d after Ctrl-C, stderr: %s", code, w.errb.String())
	}
	out := w.out.String()
	if strings.Contains(out, "──") {
		t.Errorf("header in JSON stream: %q", out)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 2 {
		t.Fatalf("want a line per run, got %q", out)
	}
	for _, l := range lines {
		var v scratchResult
		if err := json.Unmarshal([]byte(l), &v); err != nil {
			t.Errorf("line %q: %v", l, err)
		}
	}
}
