package scratch

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"alder/internal/diag"
)

// runFile parses text as scratch.alder and runs it against a temp copy of examples/todos.
func runFile(t *testing.T, text string, o FileOptions) FileResult {
	t.Helper()
	return runFileIn(t, todos(t), text, o)
}

// runFileIn is runFile against the app at dir.
func runFileIn(t *testing.T, dir, text string, o FileOptions) FileResult {
	t.Helper()
	f, ds := ParseFile("scratch.alder", text)
	if len(ds) > 0 && o.Parsed == nil {
		t.Fatalf("parse diagnostics: %v", ds)
	}
	if o.Out == "" {
		o.Out = t.TempDir()
	}
	return RunFile(context.Background(), dir, f, o)
}

// mixed is one file, one build, for the tests below that read sharedRun: pure values (a
// function, a polymorphic value), a watch that panics between two good ones, an effect, a
// value past MaxValue and one holding the frame delimiter \x1e.
const mixed = `-- a scratch file
> 1 + 1
> Todo.countOpen
> []
> 1 / 0
> 3 * 2
> Db.all (Db.from Todo)
> List.range 1 20000
> "x` + "\x1e" + `9"
`

var shared struct {
	once    sync.Once
	res     FileResult
	onTypes []FileResult // what OnTypes was given, in order
}

// sharedRun runs mixed once for every test that reads it.
func sharedRun(t *testing.T) FileResult {
	t.Helper()
	needGo(t)
	shared.once.Do(func() {
		shared.res = runFile(t, mixed, FileOptions{OnTypes: func(r FileResult) { shared.onTypes = append(shared.onTypes, r) }})
	})
	r := shared.res
	if r.Err != nil || !r.Ran || len(r.Watches) != 8 {
		t.Fatalf("err %v, ran %v, note %q, diags %v, watches %+v", r.Err, r.Ran, r.Note, r.Diags, r.Watches)
	}
	return r
}

// want checks a watch's result.
func want(t *testing.T, w WatchResult, line int, typ, value, errMsg string) {
	t.Helper()
	if w.Line != line || w.Type != typ || w.Value != value || w.Error != errMsg {
		t.Errorf("got %+v; want line %d, type %q, value %q, error %q", w, line, typ, value, errMsg)
	}
}

func TestRunFilePure(t *testing.T) {
	r := sharedRun(t)
	want(t, r.Watches[0], 2, "Int", "2", "")
	want(t, r.Watches[1], 3, "List Table.Todo.Todo -> Int", "<function>", "")
	want(t, r.Watches[2], 4, "List a", "[]", "")
	if r.Watches[0].Expr != "1 + 1" || r.Note != "" {
		t.Errorf("expr %q, note %q", r.Watches[0].Expr, r.Note)
	}
}

// Review Focus 1: a panic fails its own watch only.
func TestRunFileWatchPanicIsolated(t *testing.T) {
	r := sharedRun(t)
	want(t, r.Watches[2], 4, "List a", "[]", "")
	want(t, r.Watches[3], 5, "Int", "", "runtime error: integer divide by zero")
	want(t, r.Watches[4], 6, "Int", "6", "")
}

func TestRunFileEffectNotRun(t *testing.T) {
	r := sharedRun(t)
	w := r.Watches[5]
	if w.Value != "<Query>" || !strings.HasPrefix(w.Type, "Query (List ") || w.Note != "not run: scratch evaluates pure values only" || w.Error != "" {
		t.Errorf("%+v", w)
	}
}

func TestRunFileTruncates(t *testing.T) {
	r := sharedRun(t)
	w := r.Watches[6]
	if len(w.Value) != MaxValue || !w.Truncated || !strings.HasPrefix(w.Value, "[1, 2, 3") || w.Type != "List Int" {
		t.Errorf("len %d, truncated %v, type %q, value %.40q", len(w.Value), w.Truncated, w.Type, w.Value)
	}
	if r.Watches[0].Truncated {
		t.Error("a short value is truncated")
	}
}

// A value's own \x1e cannot end its frame: the program writes it as U+FFFD.
func TestRunFileDelimiterInValue(t *testing.T) {
	r := sharedRun(t)
	want(t, r.Watches[7], 9, "Text", "\"x\uFFFD9\"", "")
}

// truncate cuts at MaxValue bytes, backing off to a rune boundary.
func TestTruncate(t *testing.T) {
	if s, cut := truncate("short"); s != "short" || cut {
		t.Errorf("%q %v", s, cut)
	}
	ascii := strings.Repeat("a", MaxValue+10)
	if s, cut := truncate(ascii); len(s) != MaxValue || !cut {
		t.Errorf("len %d %v", len(s), cut)
	}
	// "é" is 2 bytes: one starting at MaxValue-1 straddles the cut.
	multi := strings.Repeat("a", MaxValue-1) + "é" + "b"
	s, cut := truncate(multi)
	if len(s) != MaxValue-1 || !cut || !utf8.ValidString(s) {
		t.Errorf("len %d, cut %v, valid %v", len(s), cut, utf8.ValidString(s))
	}
}

func TestRunFileOnTypes(t *testing.T) {
	sharedRun(t)
	if len(shared.onTypes) != 1 {
		t.Fatalf("OnTypes called %d times", len(shared.onTypes))
	}
	got := shared.onTypes[0]
	if got.Ran || len(got.Watches) != 8 {
		t.Fatalf("%+v", got)
	}
	for i, w := range got.Watches {
		if i == 5 { // the effect's placeholder is final
			continue
		}
		if w.Type == "" || w.Value != "" || w.Error != "" {
			t.Errorf("watch %d: %+v", i+1, w)
		}
	}
	if got.Watches[5].Type == "" || got.Watches[5].Value != "<Query>" {
		t.Errorf("effect: %+v", got.Watches[5])
	}
}

func TestRunFileInModule(t *testing.T) {
	t.Parallel()
	needGo(t)
	r := runFile(t, "in Table.Todo\n\n> countOpen []\n> filterLabel Active\n", FileOptions{})
	if r.Err != nil || !r.Ran {
		t.Fatalf("err %v, note %q, diags %v", r.Err, r.Note, r.Diags)
	}
	want(t, r.Watches[0], 3, "Int", "0", "")
	want(t, r.Watches[1], 4, "Text", `"Active"`, "")
}

// Review Focus 2: a crash mid-file keeps the values before it.
func TestRunFileCrashMidFile(t *testing.T) {
	t.Parallel()
	needGo(t)
	r := runFile(t, "loop : Int -> Int\nloop n = 1 + loop n\n\n> 1 + 1\n> loop 1\n> 2\n", FileOptions{})
	if r.Err != nil || !r.Ran {
		t.Fatalf("err %v, note %q, diags %v", r.Err, r.Note, r.Diags)
	}
	want(t, r.Watches[0], 4, "Int", "2", "")
	want(t, r.Watches[1], 5, "Int", "", "stack overflow (unbounded recursion?)")
	want(t, r.Watches[2], 6, "Int", "", "not run: the program crashed at line 5")
	if !strings.Contains(r.Note, "stack overflow (unbounded recursion?)") {
		t.Errorf("note %q", r.Note)
	}
}

// A panic while the package initialises (a constant of the file's own) comes before any watch.
func TestRunFileInitPanic(t *testing.T) {
	t.Parallel()
	needGo(t)
	r := runFile(t, "boom : Int\nboom = 1 / 0\n\n> 1 + 1\n", FileOptions{})
	if r.Err != nil || !r.Ran || r.Watches[0].Value != "" {
		t.Fatalf("err %v, ran %v, watches %+v", r.Err, r.Ran, r.Watches)
	}
	if !strings.HasPrefix(r.Note, "the program failed before any watch ran: ") || !strings.Contains(r.Note, "integer divide by zero") {
		t.Errorf("note %q", r.Note)
	}
	if strings.Contains(r.Note, "panic: ") {
		t.Errorf("note repeats the panic prefix: %q", r.Note)
	}
	if w := r.Watches[0]; w.Error != "not run: "+r.Note {
		t.Errorf("watch error %q, want %q", w.Error, "not run: "+r.Note)
	}
}

func TestRunFileDiagnosticsPositioned(t *testing.T) {
	r := runFile(t, "-- scratch\nx = 1\n\n> 1 + \"a\"\n> 2\n", FileOptions{})
	if r.Ran || r.Err != nil || r.Note != "" {
		t.Fatalf("ran %v, err %v, note %q", r.Ran, r.Err, r.Note)
	}
	var found bool
	for _, d := range r.Diags {
		if d.Severity == "error" {
			found = true
			if d.Span.File != "scratch.alder" || d.Span.Start.Line != 4 || d.Span.Start.Col < 3 || d.Span.Start.Col > 9 {
				t.Errorf("%+v", d)
			}
			if strings.Contains(d.Message, "it1") {
				t.Errorf("message names the generated it1: %q", d.Message)
			}
		}
	}
	if !found {
		t.Fatalf("no error: %+v", r.Diags)
	}
	if r.Watches[1].Type != "Int" {
		t.Errorf("the good watch has no type: %+v", r.Watches[1])
	}
}

func TestRunFileHole(t *testing.T) {
	r := runFile(t, "> 1 + ?n\n> 2\n", FileOptions{})
	if r.Ran || r.Err != nil || !strings.Contains(r.Note, "1 typed hole(s)") {
		t.Fatalf("ran %v, err %v, note %q", r.Ran, r.Err, r.Note)
	}
	if r.Watches[0].Type != "Int" || r.Watches[1].Type != "Int" {
		t.Errorf("%+v", r.Watches)
	}
}

// FileOptions.Parsed: ParseFile's diagnostics come first; an error among them stops the build,
// with the types still computed and given to OnTypes.
func TestRunFileParsedDiagnostics(t *testing.T) {
	text := "> 1 + 1\n>! Task.succeed 1\n"
	f, ds := ParseFile("scratch.alder", text)
	if len(ds) != 1 {
		t.Fatalf("parse diagnostics %v", ds)
	}
	calls := 0
	r := RunFile(context.Background(), todos(t), f, FileOptions{Out: t.TempDir(), Parsed: ds, OnTypes: func(r FileResult) {
		calls++
		if r.Watches[0].Type != "Int" {
			t.Errorf("OnTypes: %+v", r.Watches)
		}
	}})
	if r.Ran || r.Err != nil || r.Note != "" || calls != 1 {
		t.Fatalf("ran %v, err %v, note %q, OnTypes calls %d", r.Ran, r.Err, r.Note, calls)
	}
	if len(r.Diags) == 0 || r.Diags[0].Message != ds[0].Message {
		t.Errorf("diags %+v", r.Diags)
	}
}

// The module errors are diagnostics in the file, and nothing is checked.
func TestRunFileModuleErrors(t *testing.T) {
	calls := 0
	onTypes := func(FileResult) { calls++ }
	r := runFile(t, "-- header\nin Nope\n> 1\n", FileOptions{OnTypes: onTypes})
	if len(r.Diags) != 1 || r.Diags[0].Message != "no module Nope in the app" || r.Diags[0].Span.Start != (diag.Pos{Line: 2, Col: 4}) || r.Diags[0].Span.File != "scratch.alder" || r.Ran || r.Err != nil {
		t.Errorf("%+v", r)
	}
	r = runFile(t, "> 1\n", FileOptions{Module: "Table.Todo", OnTypes: onTypes})
	if len(r.Diags) != 1 || r.Diags[0].Message != "the app already has a module named Table.Todo; pass --module <Name>" || r.Diags[0].Span.Start != (diag.Pos{Line: 1, Col: 1}) || r.Ran || r.Err != nil {
		t.Errorf("%+v", r)
	}
	if calls != 0 {
		t.Errorf("OnTypes called %d times", calls)
	}
}

// A file with nothing to run, or whose app has errors, is not built, and says why.
func TestRunFileNothingToRun(t *testing.T) {
	r := runFile(t, "-- nothing yet\n", FileOptions{})
	if r.Ran || r.Err != nil || r.Note != "the file has no watches; add a line `> expr`" {
		t.Errorf("ran %v, err %v, note %q", r.Ran, r.Err, r.Note)
	}
	r = runFile(t, "> Db.all (Db.from Todo)\n", FileOptions{})
	if r.Ran || r.Err != nil || r.Note != "nothing to run: the file's watches are all effects" || r.Watches[0].Value != "<Query>" {
		t.Errorf("ran %v, err %v, note %q, %+v", r.Ran, r.Err, r.Note, r.Watches)
	}
	dir := todos(t)
	p := filepath.Join(dir, "app", "Table", "Todo.alder")
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, append(b, []byte("\nbroken : Int\nbroken = \"x\"\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	r = runFileIn(t, dir, "> 1\n", FileOptions{})
	if r.Ran || r.Err != nil || r.Note != "the app has errors; fix them and save" || len(r.Diags) == 0 || r.Diags[0].Span.File != "app/Table/Todo.alder" || r.Watches[0].Type != "Int" {
		t.Errorf("ran %v, err %v, note %q, diags %v, %+v", r.Ran, r.Err, r.Note, r.Diags, r.Watches)
	}
}

// A typed hole in the app stops the run too, and is not blamed on the file.
func TestRunFileAppHole(t *testing.T) {
	dir := todos(t)
	p := filepath.Join(dir, "app", "Table", "Todo.alder")
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, append(b, []byte("\nholey : Int\nholey = 1 + ?n\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	r := runFileIn(t, dir, "> 1\n", FileOptions{})
	if r.Ran || r.Err != nil || r.Note != "the app has 1 typed hole(s); fill them to run the file" {
		t.Errorf("ran %v, err %v, note %q, diags %v", r.Ran, r.Err, r.Note, r.Diags)
	}
}

// A half-typed watch (or declaration) before another watch is reported at its own end, in
// the user's words: never at the next watch's generated `itK = `.
func TestRunFileIncompleteBeforeWatch(t *testing.T) {
	cases := []struct {
		name, text string
		line, col  int
		next       int // the watch line the message names
	}{
		{"watch", "> 1 +\n> 2\n", 1, 6, 2},
		{"declaration", "x = 1 +\n\n-- next\n> 2\n", 1, 8, 4},
	}
	for _, c := range cases {
		r := runFile(t, c.text, FileOptions{})
		if r.Ran || r.Err != nil {
			t.Fatalf("%s: ran %v, err %v", c.name, r.Ran, r.Err)
		}
		var errs []diag.Diagnostic
		for _, d := range r.Diags {
			if d.Severity == "error" {
				errs = append(errs, d)
			}
		}
		if len(errs) == 0 {
			t.Fatalf("%s: no error: %+v", c.name, r.Diags)
		}
		for _, d := range errs {
			if d.Span.Start.Line != c.line || d.Span.Start.Col != c.col {
				t.Errorf("%s: at %d:%d, want %d:%d: %+v", c.name, d.Span.Start.Line, d.Span.Start.Col, c.line, c.col, d)
			}
			if strings.Contains(d.Message, "=") || strings.Contains(d.Message, "it1") || strings.Contains(d.Message, "it2") || strings.Contains(d.Message, "`it") {
				t.Errorf("%s: message names generated syntax: %q", c.name, d.Message)
			}
			if !strings.Contains(d.Message, fmt.Sprintf("line %d", c.next)) {
				t.Errorf("%s: message %q does not name line %d", c.name, d.Message, c.next)
			}
		}
	}
}

func TestRunFileWatchEndAndKind(t *testing.T) {
	r := sharedRun(t)
	wantKinds := []string{"pure", "pure", "pure", "pure", "pure", "query", "pure", "pure"}
	for i, w := range r.Watches {
		if w.Kind != wantKinds[i] || w.End < w.Line {
			t.Errorf("watch %d: kind %q, line %d, end %d", i, w.Kind, w.Line, w.End)
		}
	}
	r = runFile(t, "> 1 +\n    2\n> Db.all (Db.from Todo)\n", FileOptions{})
	if r.Watches[0].Line != 1 || r.Watches[0].End != 2 || r.Watches[1].End != 3 {
		t.Errorf("%+v", r.Watches)
	}
	// a watch whose type is unknown has no kind
	f, _ := ParseFile("scratch.alder", "> nosuchname\n")
	r = RunFile(context.Background(), todos(t), f, FileOptions{Out: t.TempDir()})
	if r.Watches[0].Kind != "" || r.Watches[0].Type != "" {
		t.Errorf("%+v", r.Watches[0])
	}
}

func TestFilePurposeAbsolutizesRelativeNames(t *testing.T) {
	abs, err := filepath.Abs("sub/x.scratch.alder")
	if err != nil {
		t.Fatal(err)
	}
	if FilePurpose("sub/x.scratch.alder") != FilePurpose(abs) {
		t.Error("a relative name and its absolute path differ")
	}
	if FilePurpose(abs) == FilePurpose(abs+"2") || !strings.HasPrefix(FilePurpose(abs), "file-") || len(FilePurpose(abs)) != len("file-")+12 {
		t.Errorf("%q", FilePurpose(abs))
	}
}

// With no Out, RunFile builds under the app's build directory in the file's own purpose directory.
func TestRunFileDefaultOut(t *testing.T) {
	needGo(t)
	dir := todos(t)
	f, _ := ParseFile("scratch.alder", "> 1 + 1\n")
	r := RunFile(context.Background(), dir, f, FileOptions{})
	if r.Err != nil || !r.Ran || r.Watches[0].Value != "2" {
		t.Fatalf("err %v, ran %v, %+v", r.Err, r.Ran, r.Watches)
	}
}

func TestRunFileTimeout(t *testing.T) {
	needGo(t)
	r := runFile(t, "fib : Int -> Int\nfib n =\n    if n < 2 then n else fib (n - 1) + fib (n - 2)\n\n> 1\n> fib 60\n> 3\n", FileOptions{Timeout: 2 * time.Second})
	if r.Err != nil || !r.Ran {
		t.Fatalf("err %v, ran %v, note %q", r.Err, r.Ran, r.Note)
	}
	if r.Watches[0].Value != "1" || r.Watches[1].Error == "" || !strings.Contains(r.Watches[2].Error, "not run") || r.Note == "" {
		t.Errorf("note %q, %+v", r.Note, r.Watches)
	}
	if !strings.Contains(r.Note, "timed out after 2s") && !strings.Contains(r.Watches[1].Error, "timed out after 2s") {
		t.Errorf("no timeout mentioned: note %q, %+v", r.Note, r.Watches)
	}
}
