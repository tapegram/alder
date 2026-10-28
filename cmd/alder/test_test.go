package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// runTest runs `alder test args…`, building under the test's own build directory
// (testBuildDir), so nothing lands in cmd/alder/build.
func runTest(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := runTestTo(withBuildDir(t, args), &out, &errb)
	return code, out.String(), errb.String()
}

// tempTodos copies examples/todos to a temporary directory, appends extra to the app file rel,
// and returns the copy's directory.
func tempTodos(t *testing.T, rel, extra string) string {
	t.Helper()
	tmp := t.TempDir()
	copyDir(t, todosDir(t), filepath.Join(tmp, "todos"))
	path := filepath.Join(tmp, "todos", "app", rel)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(b, []byte(extra)...), 0o644); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(tmp, "todos")
}

func TestTest_Todos(t *testing.T) {
	t.Parallel()
	needGoTool(t)
	code, stdout, stderr := runTest(t, todosDir(t))
	if code != 0 {
		t.Fatalf("exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	for _, want := range []string{
		`ok   Auth.Login "an unknown email is rejected"`,
		`ok   Auth.Login "logging in through the form"`,
		`ok   Auth.Login "a visitor is sent to log in, and can log out"`,
		`ok   Table.Todo "countOpen ignores done todos"`,
		`ok   Todos.Index "Toggle flips done"`,
		`ok   Todos.Index "Toggle by another user is not found"`,
		`ok   Todos.Index "Delete by another user is not found"`,
		`ok   Todos.Index "adding a todo through the page"`,
		`ok   Todos.Index "toggling a todo through the page"`,
		`ok   Todos.Index "a stranger cannot see my todo"`,
		`ok   App "Logout clears the user and goes to log in"`,
		`ok   Table.Todo "Active keeps only open todos"`,
		`ok   Table.Todo "each filter has its label"`,
		`ok   Table.Todo "a user with todos cannot be deleted"`,
		`ok   Todos.Index "Delete removes the todo"`,
		`ok   Todos.Index "the newest todo is listed first"`,
		`ok   Todos.Index "Db.mine is what Me sees"`,
		`ok   Todos.Show "a visitor is sent to log in from a todo"`,
		`ok   Todos.Show "saving changes the title and the due date"`,
		`ok   Todos.Show "deleting from the todo's page"`,
		`ok   App "Logout makes the role Member"`,
		`ok   Auth.Login "logging in copies the role from the user's row"`,
		// the generated admin (book 15)
		`ok   Admin.Index "the dashboard counts the todos and lists the newest five"`,
		`ok   Admin.Todos.Index "a Member gets no page, even one made Admin since logging in"`,
		`ok   Admin.Todos.Index "a demoted Admin is refused at once"`,
		`ok   Admin.Todos.Index "filtering by done lists the rows with that value"`,
		`ok   Admin.Users.Delete "an Admin cannot delete their own account"`,
		"116 passed, 0 failed\n",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout lacks %q:\n%s", want, stdout)
		}
	}
}

func TestTest_Failing(t *testing.T) {
	t.Parallel()
	needGoTool(t)
	dir := tempTodos(t, "Table/Todo.alder", "\ntest \"wrong\" = countOpen [] == 1\n")
	code, stdout, stderr := runTest(t, dir)
	if code != 1 {
		t.Fatalf("exit %d, want 1\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	for _, want := range []string{"FAIL Table.Todo \"wrong\"\n  left:  0\n  right: 1\n", "116 passed, 1 failed\n"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout lacks %q:\n%s", want, stdout)
		}
	}
}

// A property test runs: a failing one is shrunk to a two-element list and reported with its
// argument, the sides of its `==` and the seed; --json adds "property".
func TestTest_Property(t *testing.T) {
	t.Parallel()
	needGoTool(t)
	dir := tempTodos(t, "Table/Todo.alder", "\ntest \"rev\" = \\xs -> List.reverse xs == xs\n")
	code, stdout, stderr := runTest(t, dir)
	if code != 1 {
		t.Fatalf("exit %d, want 1\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	for _, want := range []string{"FAIL Table.Todo \"rev\"  (failed on run ", "  seed: ", "116 passed, 1 failed\n"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout lacks %q:\n%s", want, stdout)
		}
	}
	if !regexp.MustCompile(`\n  xs: \[-?\d+, -?\d+\]  \(xs : List Int\)\n  left:  \[-?\d+, -?\d+\]\n  right: \[-?\d+, -?\d+\]\n`).MatchString(stdout) {
		t.Errorf("no two-element case with its sides:\n%s", stdout)
	}
	code, stdout, stderr = runTest(t, dir, "--json", "-run", "rev")
	if code != 1 || !strings.Contains(stdout, `"property":{"runs":100,"seed":`) || !strings.Contains(stdout, `"args":[{"name":"xs","type":"List Int","value":"[`) {
		t.Errorf("exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
}

// --fuzz and --seed reach the test program: a passing property runs, a failing one reports the
// seed given and the paste-ready case; bad values are refused before the build; --help lists
// both and says --seed does not override saved failures.
func TestTest_PropertyFlags(t *testing.T) {
	t.Parallel()
	needGoTool(t)
	dir := tempTodos(t, "Table/Todo.alder", "\ntest \"same\" = \\(n : Int) -> n == n\n\ntest \"rev\" = \\xs -> List.reverse xs == xs\n")
	if code, stdout, stderr := runTest(t, dir, "--fuzz", "5", "--seed", "3", "-run", "same"); code != 0 || !strings.Contains(stdout, "ok   Table.Todo \"same\"\n") {
		t.Fatalf("exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	code, stdout, stderr := runTest(t, dir, "--fuzz=5", "--seed=3", "-run", "rev")
	if code != 1 || !strings.Contains(stdout, " of 5, shrunk in ") || !strings.Contains(stdout, "  seed: 3\n  to keep this case:\n    test \"rev (case)\" = List.reverse [") {
		t.Fatalf("exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	for _, bad := range [][]string{{"--fuzz", "0"}, {"--fuzz", "x"}, {"--seed", "-1"}} {
		if code, _, stderr := runTest(t, append(bad, dir)...); code != 2 || !strings.Contains(stderr, "bad "+bad[0]) {
			t.Errorf("%v: exit %d %s", bad, code, stderr)
		}
	}
	var help bytes.Buffer
	if !helpTo([]string{"test", "--help"}, &help) {
		t.Fatal("no help")
	}
	for _, want := range []string{"[--fuzz N]", "[--seed S]", "--seed does not override saved failures"} {
		if !strings.Contains(help.String(), want) {
			t.Errorf("help lacks %q:\n%s", want, help.String())
		}
	}
}

// A failing property's shrunk case is saved under the app's build directory, build/<app>/fuzz
// (which the scaffold's .gitignore keeps out of git): one file for the module's property.
func TestTest_PropertyDB(t *testing.T) {
	t.Parallel()
	needGoTool(t)
	dir := tempTodos(t, "Table/Todo.alder", "\ntest \"rev\" = \\xs -> List.reverse xs == xs\n")
	if code, stdout, stderr := runTest(t, dir, "-run", "rev"); code != 1 {
		t.Fatalf("exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	es, err := os.ReadDir(filepath.Join(testBuildDir(t), "todos", "fuzz", "Table.Todo"))
	if err != nil || len(es) != 1 {
		t.Fatalf("saved failures: %v %v", es, err)
	}
	if got := filepath.Join(appBuildDir("", dir), "fuzz"); got != filepath.Join("build", "todos", "fuzz") {
		t.Errorf("the saved failures of an app without --build-dir are in %s", got)
	}
	ignore, err := os.ReadFile("../../internal/scaffold/files/.gitignore")
	if err != nil || !regexp.MustCompile(`(?m)^/?build/$`).Match(ignore) {
		t.Errorf("the scaffold does not ignore build/: %v\n%s", err, ignore)
	}
}

// A property that crashes the test program (a stack overflow) fails with the crash and the
// seed and run that reproduce it, not its arguments.
func TestTest_PropertyFatalCrash(t *testing.T) {
	t.Parallel()
	needGoTool(t)
	dir := tempTodos(t, "Table/Todo.alder", "\nloop : Int -> Int\nloop n = loop (n + 1) + 1\n\ntest \"deep\" = \\(n : Int) -> loop n == 1\n")
	code, stdout, stderr := runTest(t, dir, "-run", "deep", "--seed", "11")
	if code != 1 || !strings.HasPrefix(stdout, "FAIL Table.Todo \"deep\"\n  error: fatal error: stack overflow\n") ||
		!strings.Contains(stdout, "\n  seed: 11\n  run: 1\n0 passed, 1 failed\n") || strings.Contains(stdout, "  n: ") {
		t.Fatalf("exit %d\nstdout %q\nstderr %q", code, stdout, stderr)
	}
	code, stdout, _ = runTest(t, dir, "-run", "deep", "--seed", "11", "--json")
	if code != 1 || !strings.Contains(stdout, `"property":{"runs":0,"seed":11,"run":1,`) {
		t.Errorf("--json: exit %d stdout %q", code, stdout)
	}
}

func TestTest_JSON(t *testing.T) {
	t.Parallel()
	needGoTool(t)
	code, stdout, stderr := runTest(t, todosDir(t), "--json")
	if code != 0 {
		t.Fatalf("exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	if len(lines) != 116 {
		t.Fatalf("want one line per test, got %d:\n%s", len(lines), stdout)
	}
	for _, l := range lines {
		var r struct {
			Module, Name, File string
			Line, Col          int
			OK                 *bool
			Trace              []string
		}
		if err := json.Unmarshal([]byte(l), &r); err != nil {
			t.Fatalf("%v: %s", err, l)
		}
		if r.Module == "" || r.Name == "" || r.OK == nil || !*r.OK {
			t.Errorf("want a passing test with module and name: %s", l)
		}
		if r.File == "" || !strings.HasPrefix(r.File, "app/") || r.Line <= 0 || r.Col <= 0 {
			t.Errorf("no position in %s", l)
		}
	}
	toggle := slices.IndexFunc(lines, func(l string) bool { return strings.Contains(l, `"name":"Toggle flips done"`) })
	if toggle < 0 || !strings.Contains(lines[toggle], `"trace":["Db.insert User","Db.insert Todo","Test.login","Test.run","Db.updateMine Todo","Db.get Todo"]`) {
		t.Errorf("the effect test's line lacks its trace (line %d):\n%s", toggle, stdout)
	}
	if stderr != "116 passed, 0 failed\n" {
		t.Errorf("stderr %q, want the summary", stderr)
	}
}

func TestTest_Run(t *testing.T) {
	t.Parallel()
	needGoTool(t)
	code, stdout, stderr := runTest(t, todosDir(t), "-run", "countOpen")
	if code != 0 || stdout != "ok   Table.Todo \"countOpen ignores done todos\"\n1 passed, 0 failed\n" {
		t.Errorf("exit %d, stdout %q\nstderr: %s", code, stdout, stderr)
	}
}

func TestTest_NoMatch(t *testing.T) {
	t.Parallel()
	needGoTool(t)
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
	t.Parallel()
	needGoTool(t)
	dir := tempTodos(t, "Table/Todo.alder", "\nbroken : Int\nbroken = \"x\"\n")
	code, stdout, stderr := runTest(t, dir)
	if code != 1 {
		t.Fatalf("exit %d, want 1\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "app/Table/Todo.alder:") || !strings.Contains(stdout, "E0003") || !strings.Contains(stdout, `broken = "x"`) {
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
	t.Parallel()
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
	t.Parallel()
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
	code, stdout, stderr := runTest(t, dir, "-run", "capability's error")
	if code != 0 || stdout != "ok   Auth.Login \"a capability's error is an Err\"\n1 passed, 0 failed\n" {
		t.Errorf("exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
}

// alder test makes a test build: the corpus app fake-ok's tests install its fakes (Test.fake),
// drive them and read the fake's table back. With adapters as well, a test that
// installs the fake uses the fake and one that does not uses the adapter (each test has its
// own capabilities), and the production build (alder build) of the same app has no fakes.
func TestTest_Fakes(t *testing.T) {
	t.Parallel()
	needGoTool(t)
	src, err := filepath.Abs("../../testdata/corpus/fake-ok")
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "fake-ok")
	copyDir(t, src, dir)
	code, stdout, stderr := runTest(t, dir)
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
	for path, body := range map[string]string{"app/App.alder": app, "adapters/payments/stripe.go": stripe, "adapters/markdown/markdown.go": markdown} {
		path = filepath.Join(dir, path)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	f, err := os.OpenFile(filepath.Join(dir, "app", "Checkout.alder"), os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(real); err != nil {
		t.Fatal(err)
	}
	f.Close()
	code, stdout, stderr = runTest(t, dir)
	if code != 0 || !strings.Contains(stdout, "4 passed, 0 failed\n") {
		t.Fatalf("with adapters: exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	var out, errb bytes.Buffer
	if code := runBuildTo([]string{"--build-dir", testBuildDir(t), dir}, &out, &errb); code != 0 {
		t.Fatalf("alder build: exit %d\n%s%s", code, out.String(), errb.String())
	}
	gen, err := os.ReadFile(filepath.Join(testBuildDir(t), "fake-ok", "Payments_alder.go"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(gen), "fake__") || strings.Contains(string(gen), "Payments__Charge") {
		t.Errorf("the production build has fake code:\n%s", gen)
	}
}

// TestTest_Browser runs the browser-ok corpus app's tests through `alder test` (M5e): the
// browser steps against the app's real routes — RF3 (a click on a page opened with a parameter
// keeps it), RF4 (`requires` redirects to the login page, Test.login, the login form with the
// Password fake, App.Logout), RF5 (a step inside Test.run; a panicking update is RequestFailed, its
// writes undone) — and then the failures a step reports: a page not shown, a control the page
// does not render (and, when it renders the message for other rows, which), a form the page does
// not render, a field the form does not declare — and, with two inputs taken out of NewNote's
// form, a field the form has no input for (C1): a defaulted field and an unticked box still
// submit, a set field and a ticked box fail the step.
// TestTest_FilePages runs file-ok (M17): Test.fetch on text, zip, byte-exact html, a 503, a
// 404, a requires redirect and a page whose load waits 20 s on the test clock, not the wall.
func TestTest_FilePages(t *testing.T) {
	t.Parallel()
	needGoTool(t)
	src, err := filepath.Abs("../../testdata/corpus/file-ok")
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "file-ok")
	copyDir(t, src, dir)
	start := time.Now()
	code, stdout, stderr := runTest(t, dir)
	if code != 0 || !strings.Contains(stdout, "9 passed, 0 failed\n") {
		t.Fatalf("exit %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if time.Since(start) > 2*time.Minute {
		t.Fatalf("the 20 s delay must be virtual; took %v", time.Since(start))
	}
}

// An `<al-button href>` puts the attributes that describe its link on the `<a>` it renders:
// `Test.has "#edit a[data-alder-reload]"` finds it there and not on the element.
func TestTest_AlButtonAnchorAttrs(t *testing.T) {
	t.Parallel()
	needGoTool(t)
	src, err := filepath.Abs("../../testdata/corpus/al-button-anchor-attrs-ok")
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "al-button-anchor-attrs-ok")
	copyDir(t, src, dir)
	code, stdout, stderr := runTest(t, dir)
	if code != 0 || !strings.Contains(stdout, "1 passed, 0 failed\n") {
		t.Fatalf("exit %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
}

func TestTest_Browser(t *testing.T) {
	t.Parallel()
	needGoTool(t)
	src, err := filepath.Abs("../../testdata/corpus/browser-ok")
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "browser-ok")
	copyDir(t, src, dir)
	code, stdout, stderr := runTest(t, dir)
	if code != 0 || !strings.Contains(stdout, "15 passed, 0 failed\n") {
		t.Fatalf("exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	failing := `
test "submit a form every row renders, by its name" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: "Me" }
  let _ <- Db.insert Note { title: Text.NonEmpty "A", owner: me }
  let _ <- Db.insert Note { title: Text.NonEmpty "B", owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Notes.Index {})
  let _ <- Test.submit Title { title: Text.NonEmpty "x" }
  Task.succeed True
}

test "click with no page open" = {
  let _ <- Test.click Boom
  Task.succeed True
}

test "submit to a row the page does not render" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: "Me" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Notes.Index {})
  let _ <- Test.submit (Rename { id: Note.NoteId "zzz" }) { title: Text.NonEmpty "x" }
  Task.succeed True
}

test "get a page that requires a user" = {
  let _ <- Test.get (Notes.Index {})
  Task.succeed True
}

test "click a message the page does not render" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: "Me" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Notes.Index {})
  let _ <- Test.click (Toggle { id: Note.NoteId "zzz" })
  Task.succeed True
}

test "submit a form the page does not render" = {
  let _ <- Test.get (Auth.Login {})
  let _ <- Test.submit NewNote { title: Text.NonEmpty "Milk", due: Nothing, urgent: False, contact: Nothing }
  Task.succeed True
}

test "submit a field the form does not declare" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: "Me" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Notes.Index {})
  let _ <- Test.submitRaw NewNote [{ name: "colour", value: "red" }]
  Task.succeed True
}

test "click another row's toggle" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: "Me" }
  let _ <- Db.insert Note { title: Text.NonEmpty "A", owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Notes.Index {})
  let _ <- Test.click (Toggle { id: Note.NoteId "zzz" })
  Task.succeed True
}

test "a set field the form has no input for" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: "Me" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Notes.Index {})
  let _ <- Test.submit NewNote { title: Text.NonEmpty "Milk", due: Nothing, urgent: False, weight: 2.5, contact: Nothing }
  Task.succeed True
}

test "a ticked box the form has no input for" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: "Me" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Notes.Index {})
  let _ <- Test.submit NewNote { title: Text.NonEmpty "Milk", due: Nothing, urgent: True, contact: Nothing }
  Task.succeed True
}

test "a raw field the form has no input for" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: "Me" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Notes.Index {})
  let _ <- Test.submitRaw NewNote [{ name: "title", value: "Milk" }, { name: "weight", value: "1.5" }]
  Task.succeed True
}

test "a defaulted field and an unticked box need no input" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: "Me" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Notes.Index {})
  let r <- Test.submit NewNote { title: Text.NonEmpty "Milk", due: Nothing, urgent: False, weight: 1.5, contact: Nothing }
  Task.succeed (r.answer == Redirected (Notes.Index {}))
}

test "get a page that is not found" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: "Me" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Notes.Show { id: Note.NoteId "nope" })
  Task.succeed True
}
`
	index := dir + "/app/Notes/Index.alder"
	srcB, err := os.ReadFile(index)
	if err != nil {
		t.Fatal(err)
	}
	view := string(srcB)
	cut := strings.NewReplacer(`<Input field="urgent" label="Urgent" />`, "", `<Input field="weight" label="Weight" />`, "").Replace(view)
	if strings.Count(cut, "<Input") != strings.Count(view, "<Input")-2 {
		t.Fatal("the inputs to take out are not in Notes.Index's view")
	}
	if err := os.WriteFile(index, []byte(cut+failing), 0o644); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr = runTest(t, dir, "-run", "Notes.Index")
	for _, want := range []string{
		"FAIL Notes.Index \"get a page that requires a user\"\n  error: GET /: redirected to /login (requires user)\n",
		"FAIL Notes.Index \"click a message the page does not render\"\n  error: the page / has no button or form for Notes.Index.Toggle { id: \"zzz\" }\n",
		"FAIL Notes.Index \"click another row's toggle\"\n  error: the page / has no button or form for Notes.Index.Toggle { id: \"zzz\" } (the page has Toggle for id: ",
		"FAIL Notes.Index \"a set field the form has no input for\"\n  error: the page's form NewNote has no input `weight`\n",
		"FAIL Notes.Index \"a raw field the form has no input for\"\n  error: the page's form NewNote has no input `weight`\n",
		"FAIL Notes.Index \"a ticked box the form has no input for\"\n  error: the page's form NewNote has no input `urgent`\n",
		"ok   Notes.Index \"a defaulted field and an unticked box need no input\"\n",
		"FAIL Notes.Index \"submit a form the page does not render\"\n  error: the page /login has no form NewNote (Notes.Index.Submit)\n",
		"FAIL Notes.Index \"submit a field the form does not declare\"\n  error: form NewNote has no field `colour`\n",
		"FAIL Notes.Index \"get a page that is not found\"\n  error: GET /notes/nope: not found\n",
		"FAIL Notes.Index \"submit a form every row renders, by its name\"\n  error: the page has 2 forms for Notes.Index.Rename: write the message with its parameters\n",
		"FAIL Notes.Index \"click with no page open\"\n  error: no page is open: `Test.get` one first\n",
		"FAIL Notes.Index \"submit to a row the page does not render\"\n  error: the page / has no form Title (Notes.Index.Rename { id: \"zzz\" })\n",
		`trace: ["Db.insert User", "Test.login", "Test.get Notes.Index", "Db.all Note", "Time.now", "Test.click Notes.Index.Toggle"]`,
		"13 passed, 12 failed\n",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("missing %q in\n%s\nstderr: %s", want, stdout, stderr)
		}
	}
	if code != 1 {
		t.Errorf("exit %d", code)
	}
}

// TestTest_Rules runs the rules-ok corpus app (M11, declared rules: an `@owner` table, the five
// scoped operations, `requires user, role R`) under `alder test` and `alder test --mutate`:
// a stranger's Test.open of my todo is PageMissing; a Reader on an Author page is PageMissing; a
// visitor is Redirected to the login page; a stale page's click on another's row is not found
// and changes nothing (a write before it in the Task undone: RF2); a helper using Db.mine from a
// test without Test.login fails with "Db.mine Todo: no signed-in user"; a demoted admin still
// passes the cookie's role check while the re-read idiom's page is not found (RF3). Every
// mutant, `scope dropped` and `role dropped` among them, is killed.
func TestTest_Rules(t *testing.T) {
	t.Parallel()
	needGoTool(t)
	src, err := filepath.Abs("../../testdata/corpus/rules-ok")
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "rules-ok")
	copyDir(t, src, dir)
	code, stdout, stderr := runTest(t, dir)
	for _, want := range []string{
		"ok   Todos.Show \"a stranger's open of my todo is not found\"\n",
		"ok   Admin.Index \"a reader on an author page is not found\"\n",
		"ok   Todos.Index \"a visitor is sent to log in\"\n",
		"ok   Todos.Index \"a stranger's toggle on a stale page is not found\"\n",
		"ok   Todos.Index \"a stranger's duplicate on a stale page is not found and undone\"\n",
		"ok   Todos.Index \"a scoped update of another's row fails the whole task\"\n",
		"ok   Todos.Index \"a scoped helper needs a signed-in user\"\n",
		"ok   Admin.Index \"a demoted admin still passes the cookie check\"\n",
		"ok   Todos.Index \"updateMine never gives a todo away\"\n",
		"27 passed, 0 failed\n",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("missing %q in\n%s\nstderr: %s", want, stdout, stderr)
		}
	}
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout, stderr)
	}

	code, stdout, stderr = runTest(t, dir, "--mutate")
	if code != 0 || !strings.HasSuffix(stdout, "27 passed, 0 failed\n31 mutants: 31 killed, 0 survived  (score 1.00)\n") {
		t.Fatalf("--mutate: exit %d\n%s\n%s", code, stdout, stderr)
	}
	// without the tests that look at a stranger or a role, exactly those mutants survive
	code, stdout, _ = runTest(t, dir, "--mutate", "-run", `^(?:User|Todo |Admin\.Index "(an author|a visitor)|Admin\.Users "(an admin|a visitor)|Todos\.Index "(a visitor|adding|toggling|renaming|removing|duplicating)|Todos\.Show "(the owner|a visitor))`)
	for _, want := range []string{
		"  Admin.Index   requires:6   role dropped       requires user, role Author | Admin → requires user\n",
		"  Admin.Users   requires:7   role dropped       requires user, role Admin → requires user\n",
		"  Todos.Index   update:39    scope dropped      Db.updateMine Todo p.id (\\t -> { t | done: not t.done }) → Db.update Todo p.id (\\t -> { t | done: not t.done }), not found when missing\n",
		"  Todos.Index   update:43    scope dropped      Db.getMine Todo p.id → Db.get Todo p.id, not found when missing\n",
		"  Todos.Index   update:48    scope dropped      Db.deleteMine Todo p.id → Db.delete Todo p.id, not found when missing\n",
		"  Todos.Show    load:12      scope dropped      Db.getMine Todo params.id → Db.get Todo params.id, not found when missing\n",
	} {
		if code != 0 || !strings.Contains(stdout, want) {
			t.Errorf("exit %d: missing survivor %q in\n%s", code, want, stdout)
		}
	}

	// the helper form outside Test.run: the test fails with the operation's text
	index := filepath.Join(dir, "app", "Todos", "Index.alder")
	b, err := os.ReadFile(index)
	if err != nil {
		t.Fatal(err)
	}
	helper := "\ntest \"a helper without a signed-in user\" = {\n  let ts <- recent |> Db.all\n  Task.succeed (List.length ts == 0)\n}\n"
	if err := os.WriteFile(index, append(b, helper...), 0o644); err != nil {
		t.Fatal(err)
	}
	code, stdout, _ = runTest(t, dir, "-run", "helper without")
	want := "FAIL Todos.Index \"a helper without a signed-in user\"\n  error: Db.mine Todo: no signed-in user\n  trace: [\"Db.mine Todo\", \"Db.all Todo\"]\n0 passed, 1 failed\n"
	if code != 1 || stdout != want {
		t.Fatalf("exit %d\n%s\nwant:\n%s", code, stdout, want)
	}
}

// TestTest_Silos runs testdata/corpus/silo-ok's tests — Test.run of a page's update runs on the
// store of the message's arm (Tag's shared one beside the `_` arm's silo), as the request does —
// and then a Test.run whose Task writes two stores, which fails as the request would (C1: the
// one-store rule holds for a test's own Tasks, not only for the checker's arms).
func TestTest_Silos(t *testing.T) {
	t.Parallel()
	needGoTool(t)
	src, err := filepath.Abs("../../testdata/corpus/silo-ok")
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "silo-ok")
	copyDir(t, src, dir)
	code, stdout, stderr := runTest(t, dir)
	for _, want := range []string{
		"ok   Projects.Index \"labels are scoped by org in the shared store\"\n",
		"ok   Projects.Index \"Test.run of Tag writes the shared store, of Seed the org's\"\n",
		"ok   Orgs \"a non-member's switch from the form is the form's error\"\n",
		"23 passed, 0 failed\n",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("missing %q in\n%s\nstderr: %s", want, stdout, stderr)
		}
	}
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout, stderr)
	}
	index := filepath.Join(dir, "app", "Projects", "Index.alder")
	b, err := os.ReadFile(index)
	if err != nil {
		t.Fatal(err)
	}
	two := "\ntest \"two stores in one Test.run\" = {\n  let s <- setup\n  let _ <- Test.login { user: Just s.me, org: Just s.a }\n" +
		"  let r <- Test.run {\n    let _ <- Db.insert Label { name: \"l\", org: s.a }\n    Db.insert Project { name: \"p\", org: s.a }\n  }\n  Task.succeed True\n}\n"
	if err := os.WriteFile(index, append(b, two...), 0o644); err != nil {
		t.Fatal(err)
	}
	code, stdout, _ = runTest(t, dir, "-run", "two stores")
	want := "FAIL Projects.Index \"two stores in one Test.run\"\n  error: db: internal error: Db.insert Project writes silo org in a Task on the shared store (one store per Task)\n"
	if code != 1 || !strings.HasPrefix(stdout, want) || !strings.HasSuffix(stdout, "0 passed, 1 failed\n") {
		t.Fatalf("exit %d\n%s\nwant:\n%s", code, stdout, want)
	}
}

func TestTest_Usage(t *testing.T) {
	t.Parallel()
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
	if entries, _ := os.ReadDir(testBuildDir(t)); len(entries) != 0 {
		t.Errorf("a usage error built something: %v", entries)
	}
}

// slowTodos is a copy of examples/todos whose Todo module adds a quick test, a test that
// runs for hours, and one that overflows the stack.
func slowTodos(t *testing.T) string {
	return tempTodos(t, "Table/Todo.alder", `
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
	t.Parallel()
	needGoTool(t)
	dir := slowTodos(t)
	var out firstWrite
	var errb bytes.Buffer
	code := runTestTo(withBuildDir(t, []string{dir, "-run", `"quick"|"hang"`, "-timeout", "2s"}), &out, &errb)
	end := time.Now()
	want := "ok   Table.Todo \"quick\"\nFAIL Table.Todo \"hang\"\n  error: timed out after 2s\n1 passed, 1 failed\n"
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
	t.Parallel()
	needGoTool(t)
	dir := slowTodos(t)
	code, stdout, stderr := runTest(t, dir, "-run", "countOpen|overflow")
	if code != 1 || !strings.HasPrefix(stdout, "ok   Table.Todo \"countOpen ignores done todos\"\nFAIL Table.Todo \"overflow\"\n  error: fatal error: stack overflow\n    app.Table__Todo__loop(") {
		t.Fatalf("exit %d\nstdout %q\nstderr %q", code, stdout, stderr)
	}
	// the summary counts the streamed result and the crash, and is the last line
	if strings.Count(stdout, "\n") != 6 || !strings.HasSuffix(stdout, "\n1 passed, 1 failed\n") || strings.Contains(stderr, "goroutine") || len(stderr) > 300 ||
		!strings.Contains(stderr, "crashed running Table.Todo \"overflow\"") {
		t.Errorf("want a short report ending in the summary\nstdout %q\nstderr %q", stdout, stderr)
	}
	code, stdout, stderr = runTest(t, dir, "-run", "countOpen|overflow", "--json")
	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	var r struct {
		Module, Name, Error string
		OK                  *bool
	}
	if code != 1 || len(lines) != 2 || json.Unmarshal([]byte(lines[1]), &r) != nil ||
		r.Module != "Table.Todo" || r.Name != "overflow" || r.OK == nil || *r.OK || !strings.HasPrefix(r.Error, "fatal error: stack overflow\n") {
		t.Errorf("--json: exit %d stdout %q", code, stdout)
	}
	if !strings.HasSuffix(stderr, "\n1 passed, 1 failed\n") {
		t.Errorf("--json: the summary is not the last line of stderr: %q", stderr)
	}
}

// TestHelperAlderTest is `alder test` in a process of its own, for TestTest_KilledParent.
func TestHelperAlderTest(t *testing.T) {
	t.Parallel()
	args := os.Getenv("ALDER_HELPER_TEST_ARGS")
	if args == "" {
		t.Skip("a helper process")
	}
	os.Exit(runTestTo(strings.Split(args, "\x1f"), os.Stdout, os.Stderr))
}

// SIGKILL of `alder test` takes the test program with it (its stdin reaches end of file).
func TestTest_KilledParent(t *testing.T) {
	t.Parallel()
	needGoTool(t)
	dir := slowTodos(t)
	bd := testBuildDir(t)
	real, _ := filepath.EvalSymlinks(bd)
	cmd := exec.Command(os.Args[0], "-test.run=^TestHelperAlderTest$")
	cmd.Env = append(os.Environ(), "ALDER_HELPER_TEST_ARGS="+dir+"\x1f--build-dir\x1f"+bd+"\x1f-run\x1fhang")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Process.Kill()
	// wait for the hanging test to be running
	status := filepath.Join(bd, "todos", "scratch", "test", "status")
	for start := time.Now(); ; time.Sleep(50 * time.Millisecond) {
		if b, _ := os.ReadFile(status); string(b) == "Table.Todo\thang\n" {
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
			if strings.Contains(l, bd) || strings.Contains(l, real) {
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

// A typed hole stops the run with a reason; a build problem under --json is one object (an
// adapter that does not compile is the user's: exit 1).
func TestTest_HoleAndJSONError(t *testing.T) {
	t.Parallel()
	needGoTool(t)
	dir := tempTodos(t, "Table/Todo.alder", "\ntest \"h\" = ?todo == 1\n")
	code, _, stderr := runTest(t, dir)
	if code != 1 || !strings.Contains(stderr, "W0002") || !strings.Contains(stderr, "alder test: no tests ran (fill the hole): ?todo : ") {
		t.Errorf("hole: exit %d stderr %q", code, stderr)
	}
	if err := os.WriteFile(filepath.Join(dir, "adapters", "bad.go"), []byte("package adapters\n\nfunc broken() int { return \"x\" }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "app", "Table", "Todo.alder"), mustRead(t, filepath.Join(todosDir(t), "app", "Table", "Todo.alder")), 0o644); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := runTest(t, dir, "--json")
	var r map[string]string
	if code != 1 || strings.Count(stdout, "\n") != 1 || json.Unmarshal([]byte(stdout), &r) != nil || len(r) != 1 || !strings.Contains(r["error"], "bad.go") {
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

// mailAlder is a capability with a fake and a test using it, and no adapter: fakes first.
const mailAlder = `module Mail

capability Mailer {
  send : Text -> Task Unit,
}

fake Mailer {
  send to = Task.succeed ()
}

test "the fake mailer sends" = {
  let _ <- Test.fake Mailer
  let _ <- Mailer.send "ada@example.com"
  let steps <- Test.trace
  Task.succeed (List.member "Mailer.send" steps)
}
`

// unwiredWarning is the warning a test build of todos with mailAlder prints.
const unwiredWarning = "warning: 1 capability without an adapter runs unwired in tests:\n" +
	"capability `Mail.Mailer` needs `adapters/mail/` with `func Mailer() app.Cap__Mail__Mailer`\n"

// alder test does not refuse a missing adapter: the capability runs unwired, with a warning,
// so a test with Test.fake passes (fakes first, before any Go) and one without fails "not
// wired in tests".
func TestTest_MissingAdapter(t *testing.T) {
	t.Parallel()
	needGoTool(t)
	dir := tempTodos(t, "App.alder", "")
	if err := os.WriteFile(filepath.Join(dir, "app", "Mail.alder"), []byte(mailAlder), 0o644); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := runTest(t, dir)
	if code != 0 || !strings.HasPrefix(stderr, unwiredWarning) || !strings.Contains(stdout, `ok   Mail "the fake mailer sends"`) {
		t.Fatalf("exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	replaceIn(t, filepath.Join(dir, "app", "Mail.alder"), "  let _ <- Test.fake Mailer\n", "")
	code, stdout, stderr = runTest(t, dir)
	if code != 1 || !strings.Contains(stdout, "capability Mail.Mailer is not wired in tests") {
		t.Fatalf("unfaked: exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
}

// Adapters alder build refuses, or that do not compile with the app, are the user's: alder
// test exits 1 with alder build's message.
func TestTest_AdapterRefusals(t *testing.T) {
	t.Parallel()
	needGoTool(t)
	dir := tempTodos(t, "App.alder", "")
	if err := os.WriteFile(filepath.Join(dir, "app", "Mail.alder"), []byte(mailAlder), 0o644); err != nil {
		t.Fatal(err)
	}
	write := func(rel, body string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, rel)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, rel), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("adapters/mail/mail.go", "package mail\n\nfunc Mailer() int { return 1 }\n")
	code, stdout, stderr := runTest(t, dir)
	if code != 1 || stdout != "" || !strings.Contains(stderr, "alder test: the adapters of todos (copied to "+filepath.Join(testBuildDir(t), "todos", "scratch", "test", "adapters")+") do not compile with the app: ") ||
		!strings.Contains(stderr, "does not implement app.Cap__Mail__Mailer") {
		t.Fatalf("wrong signature: exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	write("adapters/mail/mail.go", "package mail\n\nfunc Mailer(host string) int { return 1 }\n")
	code, _, stderr = runTest(t, dir)
	if code != 0 || !strings.Contains(stderr, "(found func Mailer(host string): it must take no parameters)") {
		t.Fatalf("near miss: exit %d\nstderr: %s", code, stderr)
	}
	write("adapters/go.mod", "module adapters\n")
	code, _, stderr = runTest(t, dir)
	if code != 1 || !strings.Contains(stderr, "adapters/go.mod: adapters/ is part of the generated module; delete it") {
		t.Fatalf("go.mod: exit %d\nstderr: %s", code, stderr)
	}
}

// mutateOK copies the mutation-testing corpus app (testdata/corpus/mutate-ok: one test per
// behaviour, each killing one mutant) into the temporary working directory as "mutate-ok",
// appending extra to app/Note.alder.
func mutateOK(t *testing.T, extra string) string {
	t.Helper()
	src, err := filepath.Abs("../../testdata/corpus/mutate-ok")
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "mutate-ok")
	copyDir(t, src, dir)
	if extra != "" {
		f, err := os.OpenFile(filepath.Join(dir, "app", "Note.alder"), os.O_APPEND|os.O_WRONLY, 0)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if _, err := f.WriteString(extra); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// mutateOKTests are the mutate-ok app's tests, as -run matches them.
var mutateOKTests = []string{
	`Note "a pinned note is labelled pinned"`,
	`Note "deleting a user deletes their notes"`,
	`Notes "a visitor is sent to log in"`,
	`Notes "I see only my notes"`,
	`Notes "adding a note stores it"`,
	`Notes "adding a note goes back to the list"`,
	`Notes "a new note is not pinned"`,
}

// TestTest_Mutate: `alder test --mutate` on an app with one test per behaviour kills every
// mutant; leaving one test out (-run selects the others) leaves exactly its mutant, one case
// per operator class; --json, --min and the refusals.
func TestTest_Mutate(t *testing.T) {
	t.Parallel()
	needGoTool(t)
	dir := mutateOK(t, "")
	code, stdout, stderr := runTest(t, dir, "--mutate")
	want := `ok   Note "a pinned note is labelled pinned"
ok   Note "deleting a user deletes their notes"
ok   Notes "a visitor is sent to log in"
ok   Notes "I see only my notes"
ok   Notes "adding a note stores it"
ok   Notes "adding a note goes back to the list"
ok   Notes "a new note is not pinned"
7 passed, 0 failed
7 mutants: 7 killed, 0 survived  (score 1.00)
`
	if code != 0 || stdout != want {
		t.Fatalf("exit %d\nstdout:\n%s\nwant:\n%s\nstderr: %s", code, stdout, want, stderr)
	}

	without := func(left string) string {
		var keep []string
		for _, k := range mutateOKTests {
			if k != left {
				keep = append(keep, regexp.QuoteMeta(k))
			}
		}
		return "^(" + strings.Join(keep, "|") + ")$"
	}
	for _, c := range []struct{ class, left, survivor string }{
		{"authorization", `Notes "a visitor is sent to log in"`, `  Notes   requires:6   requires dropped   requires user → served without user (user = User.UserId "mutant")`},
		{"data", `Notes "I see only my notes"`, `  Notes   load:12   where dropped   Db.where (Note.owner.eq session.user) → the condition dropped`},
		{"flow", `Notes "adding a note goes back to the list"`, `  Notes   update:26   redirect → rerender   Redirect (Notes {}) → Rerender`},
		{"logic", `Note "a pinned note is labelled pinned"`, `  Note   label:18   condition negated   if note.pinned → if not note.pinned`},
		{"schema", `Notes "a new note is not pinned"`, `  Note   Note:13   default changed   pinned = False → pinned = True`},
	} {
		t.Run(c.class, func(t *testing.T) {
			code, stdout, stderr := runTest(t, dir, "--mutate", "-run", without(c.left))
			want := "6 passed, 0 failed\n7 mutants: 6 killed, 1 survived  (score 0.86)\nsurvived:\n" + c.survivor + "\n"
			if code != 0 || !strings.HasSuffix(stdout, want) {
				t.Fatalf("exit %d\nstdout:\n%s\nwant suffix:\n%s\nstderr: %s", code, stdout, want, stderr)
			}
		})
	}

	// --min: below the score exits 1 with the same report; at or above it 0
	run := without(`Notes "a new note is not pinned"`)
	if code, stdout, _ := runTest(t, dir, "--mutate", "--min", "0.9", "-run", run); code != 1 || !strings.Contains(stdout, "(score 0.86)\nsurvived:\n") {
		t.Fatalf("--min 0.9: exit %d\n%s", code, stdout)
	}
	if code, _, _ := runTest(t, dir, "--mutate", "--min=0.85", "-run", run); code != 0 {
		t.Fatalf("--min 0.85: exit %d", code)
	}
	if code, _, stderr := runTest(t, dir, "--min", "0.5"); code != 2 || !strings.Contains(stderr, "--min needs --mutate") {
		t.Fatalf("--min without --mutate: exit %d %s", code, stderr)
	}

	// --json: the tests' objects, the mutants, the score
	code, stdout, stderr = runTest(t, dir, "--mutate", "--json", "-run", run)
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if code != 0 || len(lines) != 8 {
		t.Fatalf("--json: exit %d\n%s\n%s", code, stdout, stderr)
	}
	var ms struct {
		Mutants []struct {
			ID           int
			Module, Decl string
			Line         int
			Kind         string
			Killed       bool
			By, Note     string
			Tables       string
		}
	}
	if err := json.Unmarshal([]byte(lines[6]), &ms); err != nil || len(ms.Mutants) != 7 {
		t.Fatalf("mutants object %q: %v", lines[6], err)
	}
	if m := ms.Mutants[0]; m.Kind != "default changed" || m.Killed || m.By != "" || m.Tables != "ensure" {
		t.Fatalf("mutant 1: %+v", m)
	}
	if m := ms.Mutants[3]; m.Kind != "requires dropped" || !m.Killed || m.By != `Notes "a visitor is sent to log in"` || m.Tables != "" {
		t.Fatalf("mutant 4: %+v", m)
	}
	if lines[7] != `{"score":0.8571428571428571,"killed":6,"survived":1,"suppressed":0,"skipped":0}` {
		t.Fatalf("score object %s", lines[7])
	}

	// no tests selected: nothing can kill a mutant (exit 1 only with --min)
	if code, stdout, _ := runTest(t, dir, "--mutate", "-run", "nothing matches"); code != 0 || stdout != "0 passed, 0 failed\nno test matches -run: nothing can kill a mutant\n" {
		t.Fatalf("no tests: exit %d\n%s", code, stdout)
	}
	if code, _, _ := runTest(t, dir, "--mutate", "--min", "0.5", "-run", "nothing matches"); code != 1 {
		t.Fatalf("no tests with --min: exit %d", code)
	}
}

// TestTest_MutateFailingSuite: a suite that does not pass measures nothing: the normal report,
// then the refusal.
func TestTest_MutateFailingSuite(t *testing.T) {
	t.Parallel()
	needGoTool(t)
	dir := mutateOK(t, "\ntest \"broken\" = label { id: NoteId \"a\", title: \"x\", pinned: False, owner: User.UserId \"u\" } == \"pinned\"\n")
	code, stdout, stderr := runTest(t, dir, "--mutate")
	if code != 1 || !strings.Contains(stdout, "FAIL Note \"broken\"\n") || !strings.HasSuffix(stdout, "7 passed, 1 failed\ntests fail: fix them before measuring mutants\n") {
		t.Fatalf("exit %d\nstdout:\n%s\nstderr: %s", code, stdout, stderr)
	}
}

// TestTest_MutateSuiteCrashes: a suite whose own (unmutated) run crashes the test program
// measures nothing either: the crash's report, then the refusal (with --json on stderr, after
// the summary).
func TestTest_MutateSuiteCrashes(t *testing.T) {
	t.Parallel()
	needGoTool(t)
	dir := mutateOK(t, "\nspin : Int -> Int\nspin n = spin (n + 1) + 1\n\ntest \"overflow\" = spin 0 == 1\n")
	code, stdout, stderr := runTest(t, dir, "--mutate")
	if code != 1 || !strings.Contains(stdout, "FAIL Note \"overflow\"\n  error: fatal error: stack overflow") || !strings.HasSuffix(stdout, " passed, 1 failed\ntests fail: fix them before measuring mutants\n") ||
		!strings.Contains(stderr, "crashed running Note \"overflow\"") {
		t.Fatalf("exit %d\nstdout:\n%s\nstderr: %s", code, stdout, stderr)
	}
	code, stdout, stderr = runTest(t, dir, "--mutate", "--json")
	if code != 1 || strings.Contains(stdout, "tests fail") || !strings.HasSuffix(stderr, " passed, 1 failed\ntests fail: fix them before measuring mutants\n") {
		t.Fatalf("--json: exit %d\nstdout:\n%s\nstderr: %s", code, stdout, stderr)
	}
}

// TestTest_MutateCrashAndHang: a mutant that overflows the stack ends the test program; it is
// killed as crashed and the run goes on in a new program from the next mutant. A mutant that
// computes for hours is killed at -timeout as timed out, and the run goes on too.
func TestTest_MutateCrashAndHang(t *testing.T) {
	// serial: timing: the 2s per-mutant -timeout must see the stack-overflow crash first; under parallel load it reports "timed out"
	needGoTool(t)
	src, err := filepath.Abs("../../testdata/corpus/mutate-crash")
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "mutate-crash")
	copyDir(t, src, dir)
	start := time.Now()
	code, stdout, stderr := runTest(t, dir, "--mutate", "-timeout", "2s", "--json")
	if code != 0 {
		t.Fatalf("exit %d\nstdout:\n%s\nstderr: %s", code, stdout, stderr)
	}
	if d := time.Since(start); d > time.Minute {
		t.Fatalf("the run took %v", d)
	}
	if strings.Contains(stderr, "stack overflow") || strings.Contains(stderr, "goroutine ") {
		t.Fatalf("the crash report reached stderr:\n%s", stderr)
	}
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	want := `{"mutants":[` +
		`{"id":1,"module":"Calc","file":"app/Calc.alder","decl":"loop","line":9,"col":15,"kind":"condition negated","before":"if stop","after":"if not stop","killed":true,"by":"Calc \"loop stops after one step\"","note":""},` +
		`{"id":2,"module":"Calc","file":"app/Calc.alder","decl":"loop","line":9,"col":48,"kind":"bool flipped","before":"True","after":"False","killed":true,"by":"Calc \"loop stops after one step\"","note":"crashed"},` +
		`{"id":3,"module":"Calc","file":"app/Calc.alder","decl":"fib","line":12,"col":9,"kind":"condition negated","before":"if n < 2","after":"if not (n < 2)","killed":true,"by":"Calc \"work is fib 10\"","note":""},` +
		`{"id":4,"module":"Calc","file":"app/Calc.alder","decl":"fib","line":12,"col":12,"kind":"operator flipped","before":"n < 2","after":"n <= 2","killed":true,"by":"Calc \"work is fib 10\"","note":""},` +
		`{"id":5,"module":"Calc","file":"app/Calc.alder","decl":"work","line":15,"col":18,"kind":"condition negated","before":"if fast","after":"if not fast","killed":true,"by":"Calc \"work is fib 10\"","note":"timed out"}]}`
	if len(lines) != 4 || lines[2] != want || lines[3] != `{"score":1,"killed":5,"survived":0,"suppressed":0,"skipped":0}` {
		t.Fatalf("stdout:\n%s\nwant mutants:\n%s", stdout, want)
	}
}
