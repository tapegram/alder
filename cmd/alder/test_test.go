package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// runTest runs `alder test args…`. Tests that use examples/todos itself first move to a fresh
// working directory (inTemp), so the build lands there, not in cmd/alder/build.
func runTest(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := runTestTo(args, &out, &errb)
	return code, out.String(), errb.String()
}

// tempTodos copies examples/todos to a temporary directory, makes that the working directory
// (so the build lands there too), appends extra to the app file rel, and returns the copy's
// directory, "todos".
func tempTodos(t *testing.T, rel, extra string) string {
	t.Helper()
	tmp := t.TempDir()
	copyDir(t, todosDir(t), filepath.Join(tmp, "todos"))
	t.Chdir(tmp)
	path := filepath.Join("todos", "app", rel)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(b, []byte(extra)...), 0o644); err != nil {
		t.Fatal(err)
	}
	return "todos"
}

// inTemp makes a fresh temporary directory the working directory.
func inTemp(t *testing.T) { t.Chdir(t.TempDir()) }

func TestTest_Todos(t *testing.T) {
	needGoTool(t)
	inTemp(t)
	code, stdout, stderr := runTest(t, todosDir(t))
	if code != 0 {
		t.Fatalf("exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	for _, want := range []string{
		`ok   Theme "flip alternates"`,
		`ok   Todo "countOpen ignores done todos"`,
		`ok   Todos.Index "Toggle flips done"`,
		`ok   Todos.Index "Toggle by another user is not found"`,
		"4 passed, 0 failed\n",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout lacks %q:\n%s", want, stdout)
		}
	}
}

func TestTest_Failing(t *testing.T) {
	needGoTool(t)
	dir := tempTodos(t, "Todo.alder", "\ntest \"wrong\" = countOpen [] == 1\n")
	code, stdout, stderr := runTest(t, dir)
	if code != 1 {
		t.Fatalf("exit %d, want 1\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	for _, want := range []string{"FAIL Todo \"wrong\"\n  left:  0\n  right: 1\n", "4 passed, 1 failed\n"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout lacks %q:\n%s", want, stdout)
		}
	}
}

func TestTest_JSON(t *testing.T) {
	needGoTool(t)
	inTemp(t)
	code, stdout, stderr := runTest(t, todosDir(t), "--json")
	if code != 0 {
		t.Fatalf("exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	if len(lines) != 4 {
		t.Fatalf("want one line per test, got %d:\n%s", len(lines), stdout)
	}
	for _, l := range lines {
		var r struct {
			Module, Name string
			OK           *bool
			Trace        []string
		}
		if err := json.Unmarshal([]byte(l), &r); err != nil {
			t.Fatalf("%v: %s", err, l)
		}
		if r.Module == "" || r.Name == "" || r.OK == nil || !*r.OK {
			t.Errorf("want a passing test with module and name: %s", l)
		}
	}
	if !strings.Contains(lines[2], `"trace":["Db.insert User","Db.insert Todo","Test.run","Db.get Todo","Db.update Todo","Db.get Todo"]`) {
		t.Errorf("the effect test's line lacks its trace: %s", lines[2])
	}
	if stderr != "4 passed, 0 failed\n" {
		t.Errorf("stderr %q, want the summary", stderr)
	}
}

func TestTest_Run(t *testing.T) {
	needGoTool(t)
	inTemp(t)
	code, stdout, stderr := runTest(t, todosDir(t), "-run", "Theme")
	if code != 0 || stdout != "ok   Theme \"flip alternates\"\n1 passed, 0 failed\n" {
		t.Errorf("exit %d, stdout %q\nstderr: %s", code, stdout, stderr)
	}
}

func TestTest_NoMatch(t *testing.T) {
	needGoTool(t)
	inTemp(t)
	code, stdout, stderr := runTest(t, todosDir(t), "-run", "zzz")
	if code != 0 || stdout != "0 passed, 0 failed\n" {
		t.Errorf("exit %d, stdout %q\nstderr: %s", code, stdout, stderr)
	}
	code, stdout, stderr = runTest(t, todosDir(t), "-run", "zzz", "--json")
	if code != 0 || stdout != "" || stderr != "0 passed, 0 failed\n" {
		t.Errorf("--json: exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}
}

func TestTest_CheckError(t *testing.T) {
	needGoTool(t)
	dir := tempTodos(t, "Theme.alder", "\nbroken : Int\nbroken = \"x\"\n")
	code, stdout, stderr := runTest(t, dir)
	if code != 1 {
		t.Fatalf("exit %d, want 1\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "app/Theme.alder:") || !strings.Contains(stdout, "E0003") || !strings.Contains(stdout, `broken = "x"`) {
		t.Errorf("stdout lacks the diagnostic with its source line:\n%s", stdout)
	}
	if strings.Contains(stdout, "passed") {
		t.Errorf("tests ran despite the error:\n%s", stdout)
	}
	// --json: one object holding the diagnostics, and nothing else
	code, stdout, _ = runTest(t, dir, "--json")
	var r struct {
		Diagnostics []struct{ Code string } `json:"diagnostics"`
	}
	if code != 1 || json.Unmarshal([]byte(stdout), &r) != nil || len(r.Diagnostics) == 0 || r.Diagnostics[0].Code != "E0003" {
		t.Errorf("--json: exit %d, stdout %q", code, stdout)
	}
}

func TestTest_Unwired(t *testing.T) {
	needGoTool(t)
	dir := tempTodos(t, filepath.Join("Auth", "Login.alder"), `
test "verify rejects a stranger" = {
  let found <- Password.verify { email: Text.Email "x@y.z", password: "nope" }
  Task.succeed (found == Nothing)
}
`)
	if err := os.RemoveAll(filepath.Join(dir, "adapters")); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := runTest(t, dir)
	if code != 1 {
		t.Fatalf("exit %d, want 1\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	want := "FAIL Auth.Login \"verify rejects a stranger\"\n  error: capability Auth.Login.Password is not wired in tests\n  trace: [\"Password.verify\"]\n"
	if !strings.Contains(stdout, want) {
		t.Errorf("stdout lacks %q:\n%s", want, stdout)
	}
}

func TestTest_Usage(t *testing.T) {
	inTemp(t)
	for _, args := range [][]string{
		{todosDir(t), "extra"},
		{todosDir(t), "--verbose"},
		{todosDir(t), "-run"},
	} {
		code, stdout, stderr := runTest(t, args...)
		if code != 2 || stdout != "" || !strings.Contains(stderr, "usage: alder test") {
			t.Errorf("%q: exit %d, stdout %q, stderr %q", args, code, stdout, stderr)
		}
	}
	code, stdout, stderr := runTest(t, todosDir(t), "-run", "(")
	if code != 2 || stdout != "" || !strings.Contains(stderr, "alder test: bad -run pattern") {
		t.Errorf("bad regexp: exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	code, _, stderr = runTest(t, "nowhere")
	if code != 2 || !strings.Contains(stderr, "no .alder files") {
		t.Errorf("no app: exit %d, stderr %q", code, stderr)
	}
	if entries, _ := os.ReadDir("."); len(entries) != 0 {
		t.Errorf("a usage error built something: %v", entries)
	}
}
