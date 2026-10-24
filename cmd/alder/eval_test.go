package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"alder/internal/scratch"
	"alder/rt/db"
	"time"
)

// todosAbs is examples/todos, made absolute before any test changes directory.
var todosAbs, todosAbsErr = filepath.Abs("../../examples/todos")

// todosDir is examples/todos, absolute, so a test may run from a temporary directory.
func todosDir(t *testing.T) string {
	t.Helper()
	if todosAbsErr != nil {
		t.Fatal(todosAbsErr)
	}
	return todosAbs
}

func runEval(t *testing.T, stdin string, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := runEvalTo(withBuildDir(t, args), strings.NewReader(stdin), &out, &errb)
	return code, out.String(), errb.String()
}

// evalIn runs `alder eval <todos> args…` from a fresh working directory and wants exit 0 and
// exactly want on stdout.
func evalOK(t *testing.T, want string, args ...string) {
	t.Helper()
	code, stdout, stderr := runEval(t, "", append([]string{todosDir(t)}, args...)...)
	if code != 0 || stdout != want {
		t.Errorf("eval %q: exit %d, stdout %q (want %q)\nstderr: %s", args, code, stdout, want, stderr)
	}
}

func TestEval_PureValue(t *testing.T) {
	t.Parallel()
	needGoTool(t)
	evalOK(t, "3 : Int\n", "1 + 2")
}

func TestEval_UsesAppModules(t *testing.T) {
	t.Parallel()
	needGoTool(t)
	evalOK(t, "0 : Int\n", "Todo.countOpen []")
	evalOK(t, "\"Active\" : Text\n", "Todo.filterLabel Active")
}

func TestEval_Record(t *testing.T) {
	t.Parallel()
	needGoTool(t)
	evalOK(t, "{ a: 1, b: \"x\" } : { a: Int, b: Text }\n", `{ a: 1, b: "x" }`)
}

func TestEval_LetLines(t *testing.T) {
	t.Parallel()
	needGoTool(t)
	code, stdout, stderr := runEval(t, "let xs = [1, 2]\nList.length xs\n", todosDir(t), "-")
	if code != 0 || stdout != "2 : Int\n" {
		t.Errorf("exit %d, stdout %q\nstderr: %s", code, stdout, stderr)
	}
	// a type error on a let line names that line and the column in the input as typed
	code, stdout, stderr = runEval(t, "let a = 1\nlet b = a + \"x\"\nb\n", todosDir(t), "-")
	if code != 1 || stdout != "" || !strings.Contains(stderr, "input:2:") {
		t.Errorf("let error: exit %d, stdout %q\nstderr: %s", code, stdout, stderr)
	}
}

func TestEval_PolymorphicAndEffect(t *testing.T) {
	t.Parallel()
	needGoTool(t)
	evalOK(t, "[] : List a\n", "[]")
	evalOK(t, "<Query> : Query Int\n", "Db.from Todo |> Db.count")
	if _, err := os.Stat(filepath.Join(testBuildDir(t), "todos", "dev.db")); err == nil {
		t.Errorf("a Query without --run opened the dev database")
	}
	evalOK(t, "0 : Int\n", "--run", "Db.from Todo |> Db.count")
	if _, err := os.Stat(filepath.Join(testBuildDir(t), "todos", "dev.db")); err != nil {
		t.Errorf("--run did not create the dev database: %v", err)
	}
	// a Task is refused
	code, stdout, stderr := runEval(t, "", todosDir(t), "--run", "Task.succeed 1")
	if code != 1 || stdout != "" || !strings.Contains(stderr, "a Task is not run by eval") {
		t.Errorf("Task: exit %d, stdout %q\nstderr: %s", code, stdout, stderr)
	}
}

func TestEval_TypeError(t *testing.T) {
	t.Parallel()
	needGoTool(t)
	code, stdout, stderr := runEval(t, "", todosDir(t), `1 + "x"`)
	if code != 1 || stdout != "" {
		t.Errorf("exit %d, stdout %q\nstderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "E0003") || !strings.Contains(stderr, "input:1:1:") || !strings.Contains(stderr, "  1 + \"x\"\n") {
		t.Errorf("stderr lacks E0003 at input:1 with the input line:\n%s", stderr)
	}
	if strings.Contains(stderr, "Scratch") || strings.Contains(stderr, "it =") {
		t.Errorf("stderr shows the scratch module:\n%s", stderr)
	}
	// an unknown name and a parse error are diagnostics too
	for _, expr := range []string{"nope + 1", "1 +"} {
		code, stdout, stderr = runEval(t, "", todosDir(t), expr)
		if code != 1 || stdout != "" || !strings.Contains(stderr, "input:1:") {
			t.Errorf("%q: exit %d, stdout %q\nstderr: %s", expr, code, stdout, stderr)
		}
	}
}

func TestEval_BrokenAppReportsDiagnostics(t *testing.T) {
	t.Parallel()
	needGoTool(t)
	tmp := t.TempDir()
	copyDir(t, todosDir(t), filepath.Join(tmp, "todos"))
	path := filepath.Join(tmp, "todos", "app", "Table", "Todo.alder")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(path, append(b, []byte("\nbroken : Int\nbroken = \"x\"\n")...), 0o644)
	code, stdout, stderr := runEval(t, "", filepath.Join(tmp, "todos"), "1 + 2")
	if code != 1 || stdout != "" {
		t.Errorf("exit %d, stdout %q\nstderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "app/Table/Todo.alder:") || !strings.Contains(stderr, "E0003") || !strings.Contains(stderr, `broken = "x"`) {
		t.Errorf("stderr lacks the app's diagnostic with its source line:\n%s", stderr)
	}
	// no app at all is a usage/internal failure
	code, _, stderr = runEval(t, "", filepath.Join(tmp, "nowhere"), "1")
	if code != 2 {
		t.Errorf("missing app: exit %d\nstderr: %s", code, stderr)
	}
}

type evalJSON struct {
	Value       *string           `json:"value"`
	Type        *string           `json:"type"`
	Diagnostics []json.RawMessage `json:"diagnostics"`
	Error       string            `json:"error"`
	Hint        string            `json:"hint"`
}

func decodeEval(t *testing.T, stdout string) evalJSON {
	t.Helper()
	var j evalJSON
	dec := json.NewDecoder(strings.NewReader(stdout))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&j); err != nil {
		t.Fatalf("stdout is not one JSON object: %v\n%s", err, stdout)
	}
	if j.Value == nil || j.Type == nil || j.Diagnostics == nil {
		t.Fatalf("JSON lacks value, type or diagnostics: %s", stdout)
	}
	return j
}

func TestEval_JSON(t *testing.T) {
	t.Parallel()
	needGoTool(t)
	dir := todosDir(t)
	code, stdout, stderr := runEval(t, "", dir, "--json", `"a" ++ "b"`)
	j := decodeEval(t, stdout)
	if code != 0 || *j.Value != `"ab"` || *j.Type != "Text" || len(j.Diagnostics) != 0 || stderr != "" {
		t.Errorf("ok: exit %d, %+v, stderr %q", code, j, stderr)
	}
	code, stdout, _ = runEval(t, "", dir, "--json", `1 + "x"`)
	j = decodeEval(t, stdout)
	if code != 1 || *j.Value != "" || *j.Type != "" || len(j.Diagnostics) == 0 {
		t.Fatalf("type error: exit %d, %s", code, stdout)
	}
	var d struct {
		Code, File string
		Line, Col  int
	}
	if err := json.Unmarshal(j.Diagnostics[0], &d); err != nil || d.Code != "E0003" || d.File != "input" || d.Line != 1 || d.Col < 1 {
		t.Errorf("diagnostic %s (%v)", j.Diagnostics[0], err)
	}
	code, stdout, _ = runEval(t, "", dir, "--json", "Task.succeed 1")
	j = decodeEval(t, stdout)
	if code != 1 || *j.Value != "" || !strings.Contains(j.Error, "a Task is not run by eval") {
		t.Errorf("task: exit %d, %s", code, stdout)
	}
	code, stdout, _ = runEval(t, "", filepath.Join(t.TempDir(), "nowhere"), "--json", "1")
	j = decodeEval(t, stdout)
	if code != 2 || j.Error == "" {
		t.Errorf("missing app: exit %d, %s", code, stdout)
	}
}

func TestEval_Concurrent(t *testing.T) {
	t.Parallel()
	needGoTool(t)
	dir, bd := todosDir(t), testBuildDir(t)
	var wg sync.WaitGroup
	errs := make([]string, 4)
	for i := range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var out, errb bytes.Buffer
			code := runEvalTo([]string{"--build-dir", bd, dir, fmt.Sprintf("%d * 10", i)}, strings.NewReader(""), &out, &errb)
			if want := fmt.Sprintf("%d : Int\n", i*10); code != 0 || out.String() != want {
				errs[i] = fmt.Sprintf("eval %d: exit %d, stdout %q (want %q), stderr %s", i, code, out.String(), want, errb.String())
			}
		}()
	}
	wg.Wait()
	for _, e := range errs {
		if e != "" {
			t.Error(e)
		}
	}
	if _, err := os.Stat(filepath.Join(bd, "todos", "scratch", "eval.lock")); err != nil {
		t.Errorf("no lock file: %v", err)
	}
}

func needGoTool(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not on PATH")
	}
}

func TestEval_Select(t *testing.T) {
	t.Parallel()
	needGoTool(t)
	dir := todosDir(t)
	for _, args := range [][]string{{"Db.from Todo"}, {"--run", "Db.from Todo"}} {
		code, stdout, stderr := runEval(t, "", append([]string{dir}, args...)...)
		if code != 0 || stdout != "<Select> : Select Table.Todo.Todo\n" {
			t.Errorf("%q: exit %d, stdout %q\nstderr: %s", args, code, stdout, stderr)
		}
		hint := strings.Contains(stderr, "pipe it to Db.all, Db.first or Db.count")
		if hint != (args[0] == "--run") {
			t.Errorf("%q: stderr %q", args, stderr)
		}
	}
	if _, err := os.Stat(filepath.Join(testBuildDir(t), "todos", "scratch", "eval", "scratch")); err == nil {
		t.Errorf("a Select was built")
	}
	code, stdout, _ := runEval(t, "", dir, "--json", "--run", "Db.from Todo")
	j := decodeEval(t, stdout)
	if code != 0 || *j.Value != "<Select>" || *j.Type != "Select Table.Todo.Todo" || !strings.Contains(j.Hint, "Db.all") {
		t.Errorf("json: exit %d, %s", code, stdout)
	}
	// a Select inside a value is opaque too (rt.Show)
	evalOK(t, "{ s: <Select>, n: 1 } : { s: Select Table.Todo.Todo, n: Int }\n", "{ s: Db.from Todo, n: 1 }")
}

func TestEval_LetForms(t *testing.T) {
	t.Parallel()
	needGoTool(t)
	dir := todosDir(t)
	cases := []struct{ in, out string }{
		{"   1 + 2", "3 : Int\n"},                                 // the first item's indentation is ignored
		{"  let a = 1\nlet b =\n    a + 1\nb * 10", "20 : Int\n"}, // and continuation lines are indented
		{"let x : Int = 1\nx + 1", "2 : Int\n"},                   // an annotated let
		{"let none : List Int = []\nnone", "[] : List Int\n"},
	}
	for _, c := range cases {
		code, stdout, stderr := runEval(t, c.in, dir, "-")
		if code != 0 || stdout != c.out {
			t.Errorf("%q: exit %d, stdout %q (want %q)\nstderr: %s", c.in, code, stdout, c.out, stderr)
		}
	}
	// an annotated let's error is positioned in the input
	code, _, stderr := runEval(t, "let x : Text = 1\nx", dir, "-")
	if code != 1 || !strings.Contains(stderr, "input:1:16:") || !strings.Contains(stderr, "  let x : Text = 1\n") {
		t.Errorf("annotated let error: exit %d\nstderr: %s", code, stderr)
	}
}

func TestEval_NoCascade(t *testing.T) {
	t.Parallel()
	needGoTool(t)
	dir := todosDir(t)
	// a failing let is reported once; the expression using it adds nothing
	code, _, stderr := runEval(t, "let a = 1 + \"x\"\nlet b = a\na + 1", dir, "-")
	if code != 1 || strings.Count(stderr, "error[") != 1 || !strings.Contains(stderr, "E0003") {
		t.Errorf("exit %d, want one E0003\nstderr: %s", code, stderr)
	}
	// a value that uses one defined after it, or a non-function that uses itself: E0007 without
	// a ?type fix, and never the name `it`
	for _, in := range []string{"let f = \\n -> g n\nlet g = \\n -> n\n1", "let v = v + 1\nv", "it + 1"} {
		code, _, stderr = runEval(t, in, dir, "-")
		if code != 1 || !strings.Contains(stderr, "E0007") || strings.Contains(stderr, "?type") || strings.Contains(stderr, "`it`") && !strings.Contains(stderr, "eval does not define") {
			t.Errorf("%q: exit %d\nstderr: %s", in, code, stderr)
		}
	}
	code, _, stderr = runEval(t, "", dir, "1 +")
	if code != 1 || strings.Contains(stderr, "`it`") {
		t.Errorf("parse error names it: exit %d\nstderr: %s", code, stderr)
	}
}

func TestEval_LetRecAndInlineLambda(t *testing.T) {
	t.Parallel()
	needGoTool(t)
	dir := todosDir(t)
	for _, c := range []struct{ in, out string }{
		{"let fact n = if n <= 1 then 1 else n * fact (n - 1)\nfact 5", "120 : Int\n"},
		{"let fact n = if n <= 1 then 1 else n * fact (n - 1)\n1", "1 : Int\n"}, // unused: still compiles
		{"(\\a b -> a + b) 1 2", "3 : Int\n"},
	} {
		code, stdout, stderr := runEval(t, c.in, dir, "-")
		if code != 0 || stdout != c.out {
			t.Errorf("%q: exit %d, stdout %q (want %q)\nstderr: %s", c.in, code, stdout, c.out, stderr)
		}
	}
}

// A brand constructor is usable from eval, as from a `test` (E0011 elsewhere).
func TestEval_BrandConstructors(t *testing.T) {
	t.Parallel()
	needGoTool(t)
	evalOK(t, "/todos/x : Page\n", `Todos.Show { id: TodoId "x" }`)
	evalOK(t, "\"x\" : Text.NonEmpty\n", `Text.NonEmpty "x"`)
}

// An app with its own Scratch module: eval says to pass --module, and works with it.
func TestEval_ModuleFlag(t *testing.T) {
	t.Parallel()
	needGoTool(t)
	app := filepath.Join(t.TempDir(), "todos")
	if err := os.CopyFS(app, os.DirFS(todosDir(t))); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app, "app", "Scratch.alder"), []byte("module Scratch\n\nx : Int\nx = 41\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := runEval(t, "", app, "Scratch.x + 1")
	if code != 2 || stdout != "" || !strings.Contains(stderr, "the app already has a module named Scratch; pass --module <Name>") || strings.Contains(stderr, "E0009") {
		t.Errorf("without --module: exit %d, stdout %q\nstderr: %s", code, stdout, stderr)
	}
	for _, args := range [][]string{{app, "--module", "Probe", "Scratch.x + 1"}, {"--module=Probe", app, "Scratch.x + 1"}} {
		if code, stdout, stderr := runEval(t, "", args...); code != 0 || stdout != "42 : Int\n" {
			t.Errorf("%q: exit %d, stdout %q\nstderr: %s", args, code, stdout, stderr)
		}
	}
	// a diagnostic of the renamed module is still the input's
	if code, _, stderr := runEval(t, "", app, "--module", "Probe", "1 + \"x\""); code != 1 || !strings.Contains(stderr, "input:1:") {
		t.Errorf("error under --module: exit %d\nstderr: %s", code, stderr)
	}
	if code, _, stderr := runEval(t, "", app, "--module", "probe", "1"); code != 2 || !strings.Contains(stderr, "module name") {
		t.Errorf("bad --module: exit %d\nstderr: %s", code, stderr)
	}
	// a --module that is taken: not "pass --module", which was passed
	if code, _, stderr := runEval(t, "", app, "--module", "Layout", "1"); code != 2 || !strings.Contains(stderr, "the app already has a module named Layout; give --module another name") {
		t.Errorf("--module Layout: exit %d\nstderr: %s", code, stderr)
	}
}

// A line starting with a closer continues the item: a multi-line match or element pastes as
// written, its `}` or `</tag>` at column 1.
func TestEval_Closers(t *testing.T) {
	t.Parallel()
	needGoTool(t)
	dir := todosDir(t)
	for _, c := range []struct{ in, out string }{
		{"match Just 1 {\n  Just v -> v,\n  Nothing -> 0,\n}\n", "1 : Int\n"},
		{"<ul>\n  <li>a</li>\n</ul>\n", "<ul><li>a</li></ul> : Html a\n"},
		{"let r = {\n  a: 1,\n}\nr.a\n", "1 : Int\n"},
	} {
		code, stdout, stderr := runEval(t, c.in, dir, "-")
		if code != 0 || stdout != c.out {
			t.Errorf("%q: exit %d, stdout %q (want %q)\nstderr: %s", c.in, code, stdout, c.out, stderr)
		}
	}
}

// A duplicate let names the input's line, not the scratch module's.
func TestEval_DuplicateNamesInputLine(t *testing.T) {
	t.Parallel()
	needGoTool(t)
	code, _, stderr := runEval(t, "-- first\nlet x = 1\nlet x = 2\nx", todosDir(t), "-")
	if code != 1 || !strings.Contains(stderr, "input:3:5: error[E0009]: `x` is already declared at line 2\n") {
		t.Errorf("exit %d\nstderr: %s", code, stderr)
	}
}

// `alder eval <dir>` alone asks for the expression instead of evaluating the path.
func TestEval_DirWithoutExpression(t *testing.T) {
	t.Parallel()
	code, stdout, stderr := runEval(t, "", todosDir(t))
	if code != 2 || stdout != "" || !strings.Contains(stderr, "expected an expression after the app directory") {
		t.Errorf("exit %d, stdout %q\nstderr: %s", code, stdout, stderr)
	}
}

// Ctrl-C on a running eval stops it and its program: the program runs in its own process group,
// which eval kills.
func TestEval_InterruptKillsProgram(t *testing.T) {
	t.Parallel()
	needGoTool(t)
	work := t.TempDir()
	bin := filepath.Join(work, "alder")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	long := "List.foldl (\\x acc -> acc + List.length (List.range 1 100000)) 0 (List.range 1 1000000)"
	cmd := exec.Command(bin, "eval", todosDir(t), long)
	cmd.Dir = work
	var errb bytes.Buffer
	cmd.Stderr = &errb
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	program := filepath.Join(work, "build", "todos", "scratch", "eval", "scratch")
	running := func() bool { return exec.Command("pgrep", "-f", "^"+program).Run() == nil }
	deadline := time.Now().Add(2 * time.Minute)
	for !running() {
		if time.Now().After(deadline) {
			cmd.Process.Kill()
			t.Fatalf("the program never started\nstderr: %s", errb.String())
		}
		time.Sleep(100 * time.Millisecond)
	}
	start := time.Now()
	cmd.Process.Signal(os.Interrupt)
	err := cmd.Wait()
	if took := time.Since(start); took > 2*time.Second {
		t.Errorf("eval took %v to stop", took)
	}
	if ee, ok := err.(*exec.ExitError); !ok || ee.ExitCode() != 1 || !strings.Contains(errb.String(), "interrupted") {
		t.Errorf("eval: %v\nstderr: %s", err, errb.String())
	}
	if running() {
		exec.Command("pkill", "-f", "^"+program).Run()
		t.Error("the program outlived eval")
	}
}

// An inline lambda applied to more arguments than it has parameters is E0008, not an internal
// error in the generated Go.
func TestEval_InlineLambdaArity(t *testing.T) {
	t.Parallel()
	needGoTool(t)
	dir := todosDir(t)
	for _, in := range []string{"(\\a -> \\b -> a + b) 1 2", "(\\a -> a) 1 2"} {
		code, _, stderr := runEval(t, "", dir, in)
		if code != 1 || !strings.Contains(stderr, "E0008") || strings.Contains(stderr, "internal error") {
			t.Errorf("%q: exit %d\nstderr: %s", in, code, stderr)
		}
	}
	evalOK(t, "3 : Int\n", "(\\a b -> a + b) 1 2")
}

// A stdlib module's name is taken, like an app module's.
func TestEval_ModuleFlagStdlibName(t *testing.T) {
	t.Parallel()
	needGoTool(t)
	for _, name := range []string{"List", "Http", "Prelude"} {
		code, stdout, stderr := runEval(t, "", todosDir(t), "--module", name, "1")
		if code != 2 || stdout != "" || !strings.Contains(stderr, name+" is a standard library module; give --module another name") {
			t.Errorf("%s: exit %d, stdout %q\nstderr: %s", name, code, stdout, stderr)
		}
	}
}

// Unbounded recursion is one line, not Go's stack trace.
func TestEval_StackOverflow(t *testing.T) {
	t.Parallel()
	needGoTool(t)
	code, stdout, stderr := runEval(t, "let loop n = loop n\nloop 1", todosDir(t), "-")
	if code != 1 || stdout != "" || strings.TrimSpace(stderr) != "alder eval: stack overflow (unbounded recursion?)" {
		t.Errorf("exit %d, stdout %q\nstderr (%d lines): %.500s", code, stdout, strings.Count(stderr, "\n"), stderr)
	}
}

// The Query and Task mains run with the app's default session; `--run` of a Query still prints
// its result.
func TestEval_RunHasSession(t *testing.T) {
	t.Parallel()
	for _, k := range []scratch.MainKind{scratch.MainQuery, scratch.MainTask} {
		m := scratch.Main(k, false, "x")
		if !strings.Contains(m, "Session: app.TestSession()") || !strings.Contains(m, "Caps: app.Caps{}.Registry()") || strings.Contains(m, "app/caps") {
			t.Errorf("kind %d:\n%s", k, m)
		}
	}
	if m := scratch.Main(scratch.MainTask, false, "x"); !strings.Contains(m, "eff.Run(env, eff.KTask, x)") {
		t.Errorf("task main:\n%s", m)
	}
	for _, k := range []scratch.MainKind{scratch.MainPure, scratch.MainQuery, scratch.MainTask} {
		m := scratch.Main(k, true, "x")
		if !strings.Contains(m, "\t\"alder/rt\"\n") { // rt's init is the stdin tether (rt/tether.go)
			t.Errorf("kind %d: does not import alder/rt:\n%s", k, m)
		}
	}
	if m := scratch.Main(scratch.MainPure, true, "x"); !strings.Contains(m, "func main() { fmt.Print(rt.Show(x)) }") {
		t.Errorf("pure main:\n%s", m)
	}
	needGoTool(t)
	evalOK(t, "[] : List Table.Todo.Todo\n", "--run", "Db.from Todo |> Db.all")
}

// `--run` wires the app's adapters (the generated caps package) when adapters/ holds Go files,
// and none otherwise.
func TestEval_RunWithAdapters(t *testing.T) {
	t.Parallel()
	m := scratch.Main(scratch.MainQuery, true, "x")
	if !strings.Contains(m, "\t\"app/caps\"\n") || !strings.Contains(m, "Caps: caps.Caps().Registry()") {
		t.Errorf("query main with adapters:\n%s", m)
	}
	needGoTool(t)
	tmp := t.TempDir()
	app := filepath.Join(tmp, "todos")
	copyDir(t, todosDir(t), app)
	mainGo := filepath.Join(testBuildDir(t), "todos", "scratch", "eval", "cmd", "main.go")
	for _, withAdapters := range []bool{true, false} {
		if !withAdapters {
			if err := os.RemoveAll(filepath.Join(app, "adapters")); err != nil {
				t.Fatal(err)
			}
		}
		code, stdout, stderr := runEval(t, "", app, "--run", "Db.from Todo |> Db.count")
		if code != 0 || stdout != "0 : Int\n" {
			t.Fatalf("adapters %v: exit %d, stdout %q\nstderr: %s", withAdapters, code, stdout, stderr)
		}
		b, err := os.ReadFile(mainGo)
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.Contains(string(b), "caps.Caps().Registry()"); got != withAdapters {
			t.Errorf("adapters %v: main.go:\n%s", withAdapters, b)
		}
	}
}

// A let bound to a record literal is a structural record: where a table's record is expected,
// the mismatch says to annotate it.
func TestEval_StructuralRecordHint(t *testing.T) {
	t.Parallel()
	in := "let t = { id: Todo.TodoId \"a\", title: Text.NonEmpty \"x\", done: False, due: Nothing, owner: User.UserId \"u\", created: Time.epoch }\nTodo.countOpen [t]\n"
	code, _, stderr := runEval(t, in, todosDir(t), "-")
	if code != 1 || !strings.Contains(stderr, "; the record has Todo's fields: annotate the let (`let x : Todo = …`), or give the value a signature") {
		t.Errorf("exit %d\nstderr: %s", code, stderr)
	}
}

// A refusal of build/<app>/dev.db says what recreates it, and what that costs: Ensure's schema
// mismatch and Migrate's drift end in the same remedy.
func TestDevDBHint(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	todo := db.TableDesc{Name: "Todo.Todo", ID: "id", Columns: []db.Column{{Name: "id", Type: db.CText}}}
	todo2 := db.TableDesc{Name: "Todo.Todo", ID: "id", Columns: []db.Column{{Name: "id", Type: db.CText}, {Name: "x", Type: db.CInt}}}
	s, err := db.OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Ensure(ctx, []db.TableDesc{todo}, true); err != nil {
		t.Fatal(err)
	}
	err = s.Ensure(ctx, []db.TableDesc{todo2}, false)
	if err == nil {
		t.Fatal("no mismatch")
	}
	want := "; changed tables: Todo; to start over, start `alder run` (it recreates " + filepath.Join("build", "todos", "dev.db") + " empty: every row is deleted) or delete it"
	if got := devDBHint(err.Error(), filepath.Join("build", "todos")); !strings.HasSuffix(got, want) {
		t.Errorf("Ensure: got %q", got)
	}
	drift := "db: the database's tables are not the app's schema: the database was changed outside migrations since 0001_initial (changed tables: Todo): restore a backup; " + db.DevRemedy
	if got := devDBHint(drift, filepath.Join("build", "todos")); strings.Contains(got, "ALDER_DEV") || !strings.Contains(got, "start `alder run`") {
		t.Errorf("Migrate: got %q", got)
	}
}

// An eval that finds another holding the app's eval lock says it waits, and goes on when the
// lock is released.
func TestEval_WaitsForLock(t *testing.T) {
	t.Parallel()
	needGoTool(t)
	bd := testBuildDir(t)
	held := scratch.Compile(todosDir(t), &scratch.Module{Source: "it = 1\n", Infer: true}, pureMainFor("app.Scratch__it"), "eval", filepath.Join(bd, "todos", "scratch", "eval"))
	if !held.Runnable() {
		t.Fatalf("%v %v", held.Err, held.Diags)
	}
	var stdout, stderr syncBuffer
	done := make(chan int, 1)
	go func() { done <- runEvalTo([]string{"--build-dir", bd, todosDir(t), "1 + 1"}, nil, &stdout, &stderr) }()
	deadline := time.Now().Add(10 * time.Second)
	for !strings.Contains(stderr.String(), "alder eval: waiting for another alder eval of this app…\n") {
		if time.Now().After(deadline) {
			held.Close()
			t.Fatalf("no waiting message: %s", stderr.String())
		}
		time.Sleep(20 * time.Millisecond)
	}
	select {
	case code := <-done:
		t.Fatalf("eval did not wait: exit %d", code)
	default:
	}
	held.Close()
	select {
	case code := <-done:
		if code != 0 || stdout.String() != "2 : Int\n" {
			t.Errorf("exit %d, stdout %q\nstderr: %s", code, stdout.String(), stderr.String())
		}
	case <-time.After(30 * time.Second):
		t.Fatal("eval did not finish after the lock was released")
	}
}

// pureMainFor is the scratch main printing the Go expression it.
func pureMainFor(it string) string { return scratch.Main(scratch.MainPure, false, it) }

// A capability without an adapter does not stop eval: it runs unwired, with the warning, and
// `1 + 1` is evaluated; an adapter that does not compile is the user's (exit 1, build's text).
func TestEval_MissingAdapter(t *testing.T) {
	t.Parallel()
	needGoTool(t)
	dir := tempTodos(t, "App.alder", "")
	if err := os.WriteFile(filepath.Join(dir, "app", "Mail.alder"), []byte(mailAlder), 0o644); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := runEval(t, "", dir, "1 + 1")
	if code != 0 || stdout != "2 : Int\n" || stderr != unwiredWarning {
		t.Fatalf("exit %d, stdout %q\nstderr: %s", code, stdout, stderr)
	}
	if err := os.MkdirAll(filepath.Join(dir, "adapters", "mail"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "adapters", "mail", "mail.go"), []byte("package mail\n\nfunc Mailer() int { return 1 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, _, stderr = runEval(t, "", dir, "1 + 1")
	if code != 1 || !strings.Contains(stderr, "alder eval: the adapters of todos (copied to "+filepath.Join(testBuildDir(t), "todos", "scratch", "eval", "adapters")+") do not compile with the app: ") {
		t.Fatalf("broken adapter: exit %d\nstderr: %s", code, stderr)
	}
}

func TestEval_InModule(t *testing.T) {
	t.Parallel()
	needGoTool(t)
	evalOK(t, "0 : Int\n", "--in", "Table.Todo", "countOpen []")
	// the module's own names unqualified, and the names of a module it imports qualified
	evalOK(t, "\"a\" : Table.Todo.TodoId\n", "--in", "Table.Todo", "TodoId \"a\"")
	evalOK(t, "\"u\" : Table.User.UserId\n", "--in", "Table.Todo", "User.UserId \"u\"")
	evalOK(t, "\"Active\" : Text\n", "--in", "Table.Todo", "filterLabel Active")
}

func TestEval_InNestedModule(t *testing.T) {
	t.Parallel()
	needGoTool(t)
	// Todos.Index's own `update`, and Todo (which it imports) qualified
	code, stdout, stderr := runEval(t, "", todosDir(t), "--in", "Todos.Index", "update")
	if code != 0 || !strings.HasPrefix(stdout, "<function> : Todos.Index.Msg -> ") {
		t.Errorf("exit %d, stdout %q\nstderr: %s", code, stdout, stderr)
	}
	evalOK(t, "0 : Int\n", "--in=Todos.Index", "Todo.countOpen []")
}

func TestEval_InModuleDiagnosticsNameTheInput(t *testing.T) {
	t.Parallel()
	needGoTool(t)
	code, stdout, stderr := runEval(t, "", todosDir(t), "--in", "Table.Todo", "countOpen 1")
	if code != 1 || stdout != "" || !strings.Contains(stderr, "input:1:") {
		t.Errorf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}
}

// A broken declaration of the module itself is the app's diagnostic, not the input's.
func TestEval_InModuleOwnDiagnosticNamesTheModule(t *testing.T) {
	t.Parallel()
	needGoTool(t)
	tmp := t.TempDir()
	copyDir(t, todosDir(t), filepath.Join(tmp, "todos"))
	path := filepath.Join(tmp, "todos", "app", "Table", "Todo.alder")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(path, append(b, []byte("\nbroken : Int\nbroken = \"x\"\n")...), 0o644)
	code, stdout, stderr := runEval(t, "", filepath.Join(tmp, "todos"), "--in", "Table.Todo", "1 + 2")
	if code != 1 || stdout != "" || !strings.Contains(stderr, "app/Table/Todo.alder:") || strings.Contains(stderr, "input:") {
		t.Errorf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}
}

func TestEval_InUnknownModule(t *testing.T) {
	t.Parallel()
	code, _, stderr := runEval(t, "", todosDir(t), "--in", "Nope", "1")
	if code != 2 || !strings.Contains(stderr, "no module Nope") {
		t.Errorf("exit %d, stderr %q", code, stderr)
	}
}

func TestEval_InBadName(t *testing.T) {
	t.Parallel()
	code, _, stderr := runEval(t, "", todosDir(t), "--in", "todo", "1")
	if code != 2 || !strings.Contains(stderr, "--in wants") {
		t.Errorf("exit %d, stderr %q", code, stderr)
	}
}

func TestEval_InWithModuleIsUsage(t *testing.T) {
	t.Parallel()
	code, _, stderr := runEval(t, "", todosDir(t), "--in", "Table.Todo", "--module", "Probe", "1")
	if code != 2 || !strings.Contains(stderr, "--in") {
		t.Errorf("exit %d, stderr %q", code, stderr)
	}
}
