package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
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
		`ok   Todo "countOpen ignores done todos"`,
		`ok   Todos.Index "Toggle flips done"`,
		`ok   Todos.Index "Toggle by another user is not found"`,
		"3 passed, 0 failed\n",
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
	for _, want := range []string{"FAIL Todo \"wrong\"\n  left:  0\n  right: 1\n", "3 passed, 1 failed\n"} {
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
	if len(lines) != 3 {
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
	if !strings.Contains(lines[1], `"trace":["Db.insert User","Db.insert Todo","Test.run","Db.get Todo","Db.update Todo","Db.get Todo"]`) {
		t.Errorf("the effect test's line lacks its trace: %s", lines[1])
	}
	if stderr != "3 passed, 0 failed\n" {
		t.Errorf("stderr %q, want the summary", stderr)
	}
}

func TestTest_Run(t *testing.T) {
	needGoTool(t)
	inTemp(t)
	code, stdout, stderr := runTest(t, todosDir(t), "-run", "countOpen")
	if code != 0 || stdout != "ok   Todo \"countOpen ignores done todos\"\n1 passed, 0 failed\n" {
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
	dir := tempTodos(t, "Todo.alder", "\nbroken : Int\nbroken = \"x\"\n")
	code, stdout, stderr := runTest(t, dir)
	if code != 1 {
		t.Fatalf("exit %d, want 1\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "app/Todo.alder:") || !strings.Contains(stdout, "E0003") || !strings.Contains(stdout, `broken = "x"`) {
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

// A capability's Go error inside Test.run is an Err naming the capability, not a failed test.
func TestTest_CapabilityError(t *testing.T) {
	needGoTool(t)
	dir := tempTodos(t, filepath.Join("Auth", "Login.alder"), `
test "a capability's error is an Err" = {
  let r <- Test.run (Password.verify { email: Text.Email "x@y.z", password: "nope" })
  Task.succeed (r.value == Err "Password.verify: hash store unreachable")
}
`)
	adapter := `package auth

import (
	"context"
	"errors"

	"alder/rt"
	"app"
)

func LoginPassword() app.Cap__Auth__Login__Password { return password{} }

type password struct{}

func (password) Verify(context.Context, app.Args__Auth__Login__Password__Verify) (rt.Maybe[string], error) {
	return rt.Nothing[string](), errors.New("hash store unreachable")
}
`
	if err := os.WriteFile(filepath.Join(dir, "adapters", "auth", "password.go"), []byte(adapter), 0o644); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := runTest(t, dir, "-run", "Auth.Login")
	if code != 0 || stdout != "ok   Auth.Login \"a capability's error is an Err\"\n1 passed, 0 failed\n" {
		t.Errorf("exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
}

// alder test makes a test build: the corpus app fake-ok's tests install its fakes (Test.fake),
// drive them and read the fake's table back. With adapters as well, a test that
// installs the fake uses the fake and one that does not uses the adapter (each test has its
// own capabilities), and the production build (alder build) of the same app has no fakes.
func TestTest_Fakes(t *testing.T) {
	needGoTool(t)
	src, err := filepath.Abs("../../testdata/corpus/fake-ok")
	if err != nil {
		t.Fatal(err)
	}
	inTemp(t)
	copyDir(t, src, "fake-ok")
	code, stdout, stderr := runTest(t, "fake-ok")
	if code != 0 || !strings.Contains(stdout, "3 passed, 0 failed\n") || !strings.Contains(stdout, `ok   Checkout "a charge is recorded"`) {
		t.Fatalf("exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	app := `module App

session { n: Int = 0 }

error : Error -> Html Never
error err = <html><body><p>error</p></body></html>

notFound : Html Never
notFound = <html><body><p>nf</p></body></html>
`
	real := `
test "the real adapter without Test.fake" = {
  let user <- Db.insert User.User { name: "Ada" }
  let r <- Stripe.charge { cents: Int.Positive 5, user: user }
  let n <- Db.from Payments.Charge |> Db.count
  Task.succeed (r == Err "real" && n == 0)
}
`
	stripe := `package payments

import (
	"context"

	"alder/rt"
	"app"
)

func Stripe() app.Cap__Payments__Stripe { return stripe{} }

type stripe struct{}

func (stripe) Charge(context.Context, app.Args__Payments__Stripe__Charge) (rt.Result[string, string], error) {
	return rt.Err[string, string]("real"), nil
}
func (stripe) Refund(context.Context, app.Args__Payments__Stripe__Refund) (rt.Result[string, struct{}], error) {
	return rt.Err[string, struct{}]("real"), nil
}
func (stripe) Balance(context.Context) (int64, error) { return 0, nil }
`
	markdown := `package markdown

import (
	"context"

	"alder/rt/html"
	"app"
)

func Markdown() app.Cap__Markdown__Markdown { return md{} }

type md struct{}

func (md) Render(_ context.Context, a app.Args__Markdown__Markdown__Render) (html.Node, error) {
	return html.Text(a.Value), nil
}
`
	for path, body := range map[string]string{"fake-ok/app/App.alder": app, "fake-ok/adapters/payments/stripe.go": stripe, "fake-ok/adapters/markdown/markdown.go": markdown} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	f, err := os.OpenFile("fake-ok/app/Checkout.alder", os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(real); err != nil {
		t.Fatal(err)
	}
	f.Close()
	code, stdout, stderr = runTest(t, "fake-ok")
	if code != 0 || !strings.Contains(stdout, "4 passed, 0 failed\n") {
		t.Fatalf("with adapters: exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	var out, errb bytes.Buffer
	if code := runBuildTo([]string{"fake-ok"}, &out, &errb); code != 0 {
		t.Fatalf("alder build: exit %d\n%s%s", code, out.String(), errb.String())
	}
	gen, err := os.ReadFile(filepath.Join("build", "fake-ok", "Payments_alder.go"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(gen), "fake__") || strings.Contains(string(gen), "Payments__Charge") {
		t.Errorf("the production build has fake code:\n%s", gen)
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

// slowTodos is a copy of examples/todos whose Todo module adds a quick test, a test that
// runs for hours, and one that overflows the stack.
func slowTodos(t *testing.T) string {
	return tempTodos(t, "Todo.alder", `
loop : Int -> Int
loop n = loop (n + 1) + 1

spin : Int -> Int
spin n = List.foldl (\i a -> a + List.foldl (\j b -> b + List.sum (List.range 1 n)) 0 (List.range 1 n)) 0 (List.range 1 n)

test "quick" = spin 3 > 0

test "hang" = spin 100000 > 0

test "overflow" = loop 0 == 1
`)
}

// firstWrite records the output and when its first write arrived.
type firstWrite struct {
	mu    sync.Mutex
	buf   bytes.Buffer
	first time.Time
}

func (w *firstWrite) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.first.IsZero() {
		w.first = time.Now()
	}
	return w.buf.Write(p)
}

// Results stream as tests finish; at -timeout the running test fails as timed out.
func TestTest_StreamAndTimeout(t *testing.T) {
	needGoTool(t)
	dir := slowTodos(t)
	var out firstWrite
	var errb bytes.Buffer
	code := runTestTo([]string{dir, "-run", `"quick"|"hang"`, "-timeout", "2s"}, &out, &errb)
	end := time.Now()
	want := "ok   Todo \"quick\"\nFAIL Todo \"hang\"\n  error: timed out after 2s\n1 passed, 1 failed\n"
	if code != 1 || out.buf.String() != want {
		t.Fatalf("exit %d\nstdout %q\nwant   %q\nstderr: %s", code, out.buf.String(), want, errb.String())
	}
	if gap := end.Sub(out.first); gap < time.Second {
		t.Errorf("the first result arrived only %v before the end: not streamed", gap)
	}
	if !strings.Contains(errb.String(), "timed out after 2s") {
		t.Errorf("stderr %q", errb.String())
	}
}

// A test that crashes the program (a stack overflow) fails by name, without the Go trace.
func TestTest_Crash(t *testing.T) {
	needGoTool(t)
	dir := slowTodos(t)
	code, stdout, stderr := runTest(t, dir, "-run", "countOpen|overflow")
	if code != 1 || !strings.HasPrefix(stdout, "ok   Todo \"countOpen ignores done todos\"\nFAIL Todo \"overflow\"\n  error: fatal error: stack overflow\n    app.Todo__loop(") {
		t.Fatalf("exit %d\nstdout %q\nstderr %q", code, stdout, stderr)
	}
	// the summary counts the streamed result and the crash, and is the last line
	if strings.Count(stdout, "\n") != 6 || !strings.HasSuffix(stdout, "\n1 passed, 1 failed\n") || strings.Contains(stderr, "goroutine") || len(stderr) > 300 ||
		!strings.Contains(stderr, "crashed running Todo \"overflow\"") {
		t.Errorf("want a short report ending in the summary\nstdout %q\nstderr %q", stdout, stderr)
	}
	code, stdout, stderr = runTest(t, dir, "-run", "countOpen|overflow", "--json")
	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	var r struct {
		Module, Name, Error string
		OK                  *bool
	}
	if code != 1 || len(lines) != 2 || json.Unmarshal([]byte(lines[1]), &r) != nil ||
		r.Module != "Todo" || r.Name != "overflow" || r.OK == nil || *r.OK || !strings.HasPrefix(r.Error, "fatal error: stack overflow\n") {
		t.Errorf("--json: exit %d stdout %q", code, stdout)
	}
	if !strings.HasSuffix(stderr, "\n1 passed, 1 failed\n") {
		t.Errorf("--json: the summary is not the last line of stderr: %q", stderr)
	}
}

// TestHelperAlderTest is `alder test` in a process of its own, for TestTest_KilledParent.
func TestHelperAlderTest(t *testing.T) {
	args := os.Getenv("ALDER_HELPER_TEST_ARGS")
	if args == "" {
		t.Skip("a helper process")
	}
	os.Exit(runTestTo(strings.Split(args, "\x1f"), os.Stdout, os.Stderr))
}

// SIGKILL of `alder test` takes the test program with it (its stdin reaches end of file).
func TestTest_KilledParent(t *testing.T) {
	needGoTool(t)
	dir := slowTodos(t)
	wd, _ := os.Getwd()
	real, _ := filepath.EvalSymlinks(wd)
	cmd := exec.Command(os.Args[0], "-test.run=^TestHelperAlderTest$")
	cmd.Env = append(os.Environ(), "ALDER_HELPER_TEST_ARGS="+dir+"\x1f-run\x1fhang")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Process.Kill()
	// wait for the hanging test to be running
	status := filepath.Join(wd, "build", "todos", "scratch", "test", "status")
	for start := time.Now(); ; time.Sleep(50 * time.Millisecond) {
		if b, _ := os.ReadFile(status); string(b) == "Todo\thang\n" {
			break
		}
		if time.Since(start) > 2*time.Minute {
			t.Fatal("the test never started")
		}
	}
	children := func() []string {
		out, _ := exec.Command("ps", "-A", "-o", "pid=,command=").Output()
		var ps []string
		for _, l := range strings.Split(string(out), "\n") {
			if strings.Contains(l, filepath.Join(wd, "build")) || strings.Contains(l, filepath.Join(real, "build")) {
				ps = append(ps, strings.TrimSpace(l))
			}
		}
		return ps
	}
	if len(children()) == 0 {
		t.Fatal("found no test program running")
	}
	cmd.Process.Signal(syscall.SIGKILL)
	cmd.Wait()
	for start := time.Now(); len(children()) > 0; time.Sleep(50 * time.Millisecond) {
		if time.Since(start) > 2*time.Second {
			left := children()
			for _, l := range left {
				pid, _, _ := strings.Cut(l, " ")
				exec.Command("kill", "-9", pid).Run()
			}
			t.Fatalf("the test program outlived alder test: %q", left)
		}
	}
}

// A typed hole stops the run with a reason; a build problem under --json is one object.
func TestTest_HoleAndJSONError(t *testing.T) {
	needGoTool(t)
	dir := tempTodos(t, "Todo.alder", "\ntest \"h\" = ?todo == 1\n")
	code, _, stderr := runTest(t, dir)
	if code != 1 || !strings.Contains(stderr, "W0002") || !strings.HasSuffix(stderr, "alder test: no tests ran (fill the hole)\n") {
		t.Errorf("hole: exit %d stderr %q", code, stderr)
	}
	if err := os.WriteFile(filepath.Join(dir, "adapters", "bad.go"), []byte("package adapters\n\nfunc broken() int { return \"x\" }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "app", "Todo.alder"), mustRead(t, filepath.Join(todosDir(t), "app", "Todo.alder")), 0o644); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := runTest(t, dir, "--json")
	var r map[string]string
	if code != 2 || strings.Count(stdout, "\n") != 1 || json.Unmarshal([]byte(stdout), &r) != nil || len(r) != 1 || !strings.Contains(r["error"], "bad.go") {
		t.Errorf("build error: exit %d stdout %q stderr %q", code, stdout, stderr)
	}
	code, stdout, _ = runTest(t, dir, "--json", "-run", "(")
	if code != 2 || !strings.HasPrefix(stdout, `{"error":"bad -run pattern`) {
		t.Errorf("bad -run: exit %d stdout %q", code, stdout)
	}
	code, stdout, stderr = runTest(t, dir, "-timeout", "soon")
	if code != 2 || stdout != "" || !strings.Contains(stderr, "bad -timeout") {
		t.Errorf("bad -timeout: exit %d stdout %q stderr %q", code, stdout, stderr)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// alder test checks the adapters as alder build does, with the same texts: a missing
// constructor stops the build before anything is written.
func TestTest_MissingAdapter(t *testing.T) {
	needGoTool(t)
	dir := tempTodos(t, "App.alder", "")
	replaceIn(t, filepath.Join(dir, "adapters", "auth", "password.go"), "func LoginPassword()", "func loginPassword()")
	code, stdout, stderr := runTest(t, dir)
	want := "todos is missing adapters for 1 capability:\n" +
		"capability `Auth.Login.Password` needs `adapters/auth/` with `func LoginPassword() app.Cap__Auth__Login__Password`\n"
	if code != 2 || stdout != "" || !strings.Contains(stderr, want) {
		t.Fatalf("exit %d\nstdout: %s\nstderr: %s\nwant it to contain %q", code, stdout, stderr, want)
	}
	if _, err := os.Stat(filepath.Join("build", "todos", "scratch", "test", "go.mod")); !os.IsNotExist(err) {
		t.Errorf("the scratch module was written: %v", err)
	}
}
