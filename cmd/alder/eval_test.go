package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
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
	code := runEvalTo(args, strings.NewReader(stdin), &out, &errb)
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
	needGoTool(t)
	t.Chdir(t.TempDir())
	evalOK(t, "3 : Int\n", "1 + 2")
}

func TestEval_UsesAppModules(t *testing.T) {
	needGoTool(t)
	t.Chdir(t.TempDir())
	evalOK(t, "0 : Int\n", "Todo.countOpen []")
	evalOK(t, "Dark : Theme\n", "Theme.flip Light")
}

func TestEval_Record(t *testing.T) {
	needGoTool(t)
	t.Chdir(t.TempDir())
	evalOK(t, "{ a: 1, b: \"x\" } : { a: Int, b: Text }\n", `{ a: 1, b: "x" }`)
}

func TestEval_LetLines(t *testing.T) {
	needGoTool(t)
	t.Chdir(t.TempDir())
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
	needGoTool(t)
	t.Chdir(t.TempDir())
	evalOK(t, "[] : List a\n", "[]")
	evalOK(t, "<Query> : Query Int\n", "Db.from Todo |> Db.count")
	if _, err := os.Stat(filepath.Join("build", "todos", "dev.db")); err == nil {
		t.Errorf("a Query without --run opened the dev database")
	}
	evalOK(t, "0 : Int\n", "--run", "Db.from Todo |> Db.count")
	if _, err := os.Stat(filepath.Join("build", "todos", "dev.db")); err != nil {
		t.Errorf("--run did not create the dev database: %v", err)
	}
	// a Task is refused
	code, stdout, stderr := runEval(t, "", todosDir(t), "--run", "Task.succeed 1")
	if code != 1 || stdout != "" || !strings.Contains(stderr, "a Task is not run by eval") {
		t.Errorf("Task: exit %d, stdout %q\nstderr: %s", code, stdout, stderr)
	}
}

func TestEval_TypeError(t *testing.T) {
	needGoTool(t)
	t.Chdir(t.TempDir())
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
	needGoTool(t)
	tmp := t.TempDir()
	copyDir(t, todosDir(t), filepath.Join(tmp, "todos"))
	t.Chdir(tmp)
	path := filepath.Join("todos", "app", "Theme.alder")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(path, append(b, []byte("\nbroken : Int\nbroken = \"x\"\n")...), 0o644)
	code, stdout, stderr := runEval(t, "", "todos", "1 + 2")
	if code != 1 || stdout != "" {
		t.Errorf("exit %d, stdout %q\nstderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "app/Theme.alder:") || !strings.Contains(stderr, "E0003") || !strings.Contains(stderr, `broken = "x"`) {
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
	needGoTool(t)
	t.Chdir(t.TempDir())
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
	needGoTool(t)
	t.Chdir(t.TempDir())
	dir := todosDir(t)
	var wg sync.WaitGroup
	errs := make([]string, 4)
	for i := range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var out, errb bytes.Buffer
			code := runEvalTo([]string{dir, fmt.Sprintf("%d * 10", i)}, strings.NewReader(""), &out, &errb)
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
	if _, err := os.Stat(filepath.Join("build", "todos", "scratch", "eval.lock")); err != nil {
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
	needGoTool(t)
	t.Chdir(t.TempDir())
	dir := todosDir(t)
	for _, args := range [][]string{{"Db.from Todo"}, {"--run", "Db.from Todo"}} {
		code, stdout, stderr := runEval(t, "", append([]string{dir}, args...)...)
		if code != 0 || stdout != "<Select> : Select Todo\n" {
			t.Errorf("%q: exit %d, stdout %q\nstderr: %s", args, code, stdout, stderr)
		}
		hint := strings.Contains(stderr, "pipe it to Db.all, Db.first or Db.count")
		if hint != (args[0] == "--run") {
			t.Errorf("%q: stderr %q", args, stderr)
		}
	}
	if _, err := os.Stat(filepath.Join("build", "todos", "scratch", "eval", "scratch")); err == nil {
		t.Errorf("a Select was built")
	}
	code, stdout, _ := runEval(t, "", dir, "--json", "--run", "Db.from Todo")
	j := decodeEval(t, stdout)
	if code != 0 || *j.Value != "<Select>" || *j.Type != "Select Todo" || !strings.Contains(j.Hint, "Db.all") {
		t.Errorf("json: exit %d, %s", code, stdout)
	}
	// a Select inside a value is opaque too (rt.Show)
	evalOK(t, "{ s: <Select>, n: 1 } : { s: Select Todo, n: Int }\n", "{ s: Db.from Todo, n: 1 }")
}

func TestEval_LetForms(t *testing.T) {
	needGoTool(t)
	t.Chdir(t.TempDir())
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
	needGoTool(t)
	t.Chdir(t.TempDir())
	dir := todosDir(t)
	// a failing let is reported once; the expression using it adds nothing
	code, _, stderr := runEval(t, "let a = 1 + \"x\"\nlet b = a\na + 1", dir, "-")
	if code != 1 || strings.Count(stderr, "error[") != 1 || !strings.Contains(stderr, "E0003") {
		t.Errorf("exit %d, want one E0003\nstderr: %s", code, stderr)
	}
	// a value that uses itself: E0007 without a ?type fix, and never the name `it`
	for _, in := range []string{"let f = \\n -> f n\n1", "it + 1"} {
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
