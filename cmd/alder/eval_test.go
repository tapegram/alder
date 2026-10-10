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
	if !strings.Contains(stderr, "E0003") || !strings.Contains(stderr, "input:1:") || !strings.Contains(stderr, "  1 + \"x\"\n") {
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
