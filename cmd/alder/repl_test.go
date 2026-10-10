package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// runRepl runs `alder repl args…` over the lines of input and returns its exit code and its
// standard output and error interleaved (one buffer: the REPL writes from one goroutine).
func runRepl(t *testing.T, input string, args ...string) (int, string) {
	t.Helper()
	var out bytes.Buffer
	code := runReplTo(args, strings.NewReader(input), &out, &out, nil)
	return code, out.String()
}

// replTodos runs the REPL over input against examples/todos from a fresh working directory
// and wants exit 0.
func replTodos(t *testing.T, input string) string {
	t.Helper()
	t.Chdir(t.TempDir())
	code, out := runRepl(t, input, todosDir(t))
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	return out
}

func wantIn(t *testing.T, out string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(out, w) {
			t.Errorf("output lacks %q:\n%s", w, out)
		}
	}
}

// liveRepl is a REPL running in the background, fed through a pipe.
type liveRepl struct {
	in   *io.PipeWriter
	out  *syncBuffer
	intr chan struct{}
	done chan int
}

func startRepl(t *testing.T, dir string) *liveRepl {
	t.Helper()
	pr, pw := io.Pipe()
	r := &liveRepl{in: pw, out: &syncBuffer{}, intr: make(chan struct{}, 1), done: make(chan int, 1)}
	go func() { r.done <- runReplTo([]string{dir}, pr, r.out, r.out, r.intr) }()
	t.Cleanup(func() {
		pw.Close()
		select {
		case <-r.done:
		case <-time.After(10 * time.Second):
			t.Error("the REPL did not quit at end of input")
		}
	})
	return r
}

func (r *liveRepl) send(t *testing.T, line string) {
	t.Helper()
	if _, err := io.WriteString(r.in, line+"\n"); err != nil {
		t.Fatal(err)
	}
}

// waitOut waits until the output holds want n times.
func (r *liveRepl) waitOut(t *testing.T, want string, n int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if strings.Count(r.out.String(), want) >= n {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("no %q (×%d) within %v:\n%s", want, n, timeout, r.out.String())
}

func TestRepl_LetAndExpr(t *testing.T) {
	needGoTool(t)
	out := replTodos(t, "let x = 1\nx + 2\n")
	wantIn(t, out, "alder> x : Int\n", "alder> 3 : Int\n")
}

func TestRepl_Type(t *testing.T) {
	out := replTodos(t, ":type Todo.countOpen\n")
	wantIn(t, out, "Todo.countOpen : List Todo -> Int\n")
}

func TestRepl_Diagnostic(t *testing.T) {
	out := replTodos(t, "1 + \"x\"\nlet y = 1 + \"x\"\n:type y\n")
	wantIn(t, out,
		"expr:1:1: error[E0003]",
		"  1 + \"x\"\n  ^^^^^^^\n",
		// the let's diagnostic names the REPL line it was entered at, over its own text
		"let y (line 2): error[E0003]",
		"  let y = 1 + \"x\"\n          ^^^^^^^\n",
		// and the let was not kept
		"expr:1:1: error[E0002]",
	)
	if strings.Contains(out, "y : ") {
		t.Errorf("a let with an error was kept:\n%s", out)
	}
	// what follows `let` is not a name
	out = replTodos(t, "let 3 = 4\n")
	wantIn(t, out, "let (line 1): error[")
	if strings.Contains(out, "y : ") {
		t.Errorf("a let with an error was kept:\n%s", out)
	}
}

func TestRepl_Redefine(t *testing.T) {
	out := replTodos(t, "let x = 1\nlet x = \"a\"\n:type x\nlet x = x + 1\n:type x\n")
	wantIn(t, out, "x : Int\n", "redefined x\n", "x : Text\n")
	if strings.Count(out, "x : Text\n") != 3 { // the let, :type, and :type after the failed redefinition
		t.Errorf("want x : Text three times:\n%s", out)
	}
}

// Redefining a let replaces it in place: the lets that use it see the new definition.
func TestRepl_RedefineDependents(t *testing.T) {
	needGoTool(t)
	out := replTodos(t, "let a = 1\nlet b = a + 1\nlet a = 5\nb\n")
	wantIn(t, out, "redefined a\na : Int\n", "6 : Int\n")
}

func TestRepl_Reset(t *testing.T) {
	out := replTodos(t, "let x = 1\n:reset\n:type x\n")
	wantIn(t, out, "x : Int\n", "expr:1:1: error[E0002]")
}

func TestRepl_Continuation(t *testing.T) {
	out := replTodos(t, "let xs = [1,\\\n2]\n:type xs\nlet ys = [1,\\\n\"a\"]\n")
	wantIn(t, out, "    …> ", "xs : List Int\n", "xs : List Int\n",
		// line 5 is the let's second line, entered at line 4
		"let ys (line 5): error[", "  \"a\"]\n  ^")
}

func TestRepl_QueryHints(t *testing.T) {
	out := replTodos(t, "Db.from Todo \\\n  |> Db.count\nDb.from Todo |> Db.all\nTask.fail \"no\"\nDb.from Todo\n:run Task.fail \"no\"\n:run! (Db.from Todo |> Db.all)\n:html 1\n")
	wantIn(t, out,
		"<Query> : Query (List Todo)\n", "`:run ",
		"hint: `:run Db.from Todo |> Db.count` runs it", // a multi-line input on one line
		"<Task> : Task a\n", "`:run! ",
		"<Select> : Select Todo\n", "Db.all, Db.first or Db.count",
		"`:run` wants a Query; use `:run!` for a Task",
		"`:run!` wants a Task; use `:run` for a Query",
		"`:html` wants an Html value; 1 : Int",
	)
}

func TestRepl_Run(t *testing.T) {
	needGoTool(t)
	out := replTodos(t, ":run (Db.from Todo |> Db.count)\n")
	wantIn(t, out, "0 : Int\n")
}

func TestRepl_RunBangAndFailure(t *testing.T) {
	needGoTool(t)
	out := replTodos(t, strings.Join([]string{
		`:run! (Db.insert User { email: Text.Email "a@b.c", name: Text.NonEmpty "A", passwordHash: "" })`,
		`:run (Db.from User |> Db.count)`,
		`:run! (Task.fail "no")`,
		// a write before the failure is rolled back with it
		`:run! {\`,
		`  let _ <- Db.insert User { email: Text.Email "b@b.c", name: Text.NonEmpty "B", passwordHash: "" }\`,
		`  Task.fail "late"\`,
		`}`,
		`:run (Db.from User |> Db.count)`,
	}, "\n")+"\n")
	wantIn(t, out, " : User.UserId\n", "alder repl: the task failed: no\n", "alder repl: the task failed: late\n")
	if n := strings.Count(out, "1 : Int\n"); n != 2 {
		t.Errorf("want the count 1 twice, got %d:\n%s", n, out)
	}
}

func TestRepl_Html(t *testing.T) {
	needGoTool(t)
	out := replTodos(t, ":html <p>hi</p>\n")
	wantIn(t, out, "alder> <p>hi</p>\n")
}

// replTemp copies examples/todos to a temporary directory, makes it the working directory,
// gives the copy's Todo module a function probe nothing in the app uses, and returns the
// copy's directory.
func replTemp(t *testing.T) string {
	t.Helper()
	return tempTodos(t, "Todo.alder", "\nprobe : Int -> Int\nprobe n = n + 1\n")
}

// dropProbe removes probe from the copy's Todo module.
func dropProbe(t *testing.T, dir string) {
	t.Helper()
	path := filepath.Join(dir, "app", "Todo.alder")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	i := strings.Index(src, "\nprobe : ")
	if i < 0 {
		t.Fatal("no probe in Todo.alder")
	}
	if err := os.WriteFile(path, []byte(src[:i]), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRepl_ReloadDropsBrokenLet(t *testing.T) {
	dir := replTemp(t)
	r := startRepl(t, dir)
	r.send(t, "let n = Todo.probe 1")
	r.send(t, "let k = 2")
	r.waitOut(t, "k : Int\n", 1, 10*time.Second)
	dropProbe(t, dir)
	r.send(t, ":reload")
	r.waitOut(t, "dropped let n: error[", 1, 10*time.Second)
	r.send(t, ":type k")
	r.waitOut(t, "k : Int\n", 2, 10*time.Second)
	r.send(t, ":type n")
	r.waitOut(t, "expr:1:1: error[E0002]", 1, 10*time.Second)
	// the watcher sees the edit too, but the :reload already re-checked it: no second notice
	time.Sleep(time.Second)
	if strings.Contains(r.out.String(), "app changed") {
		t.Errorf("a second notice after :reload:\n%s", r.out.String())
	}
}

// An app with an error: its diagnostics are printed once, then each line says it in one line;
// :reload keeps the session; fixing the file makes the REPL work.
func TestRepl_BrokenApp(t *testing.T) {
	needGoTool(t)
	dir := replTemp(t)
	path := filepath.Join(dir, "app", "Todo.alder")
	good, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(append([]byte(nil), good...), "\nbroken : Int\nbroken = \"x\"\n"...), 0o644); err != nil {
		t.Fatal(err)
	}
	r := startRepl(t, dir)
	r.waitOut(t, "the app has errors; fix them (the REPL re-checks when you save)", 1, 10*time.Second)
	r.send(t, "1 + 1")
	r.waitOut(t, "alder repl: the app has errors (shown above); fix them and save\n", 1, 10*time.Second)
	r.send(t, ":reload")
	r.waitOut(t, "the session is kept until the app checks\n", 1, 10*time.Second)
	out := r.out.String()
	if n := strings.Count(out, "error[E0003]"); n != 1 {
		t.Errorf("the app's error printed %d times, want once:\n%s", n, out)
	}
	if strings.Contains(out, "session re-checked") {
		t.Errorf(":reload of a broken app said re-checked:\n%s", out)
	}
	time.Sleep(50 * time.Millisecond)
	if err := os.WriteFile(path, good, 0o644); err != nil {
		t.Fatal(err)
	}
	r.waitOut(t, "app changed: session re-checked\n", 1, 3*time.Second)
	r.send(t, "1 + 1")
	r.waitOut(t, "2 : Int\n", 1, 30*time.Second)
}

func TestRepl_AutoReload(t *testing.T) {
	dir := replTemp(t)
	r := startRepl(t, dir)
	r.send(t, "let n = Todo.probe 1")
	r.waitOut(t, "n : Int\n", 1, 10*time.Second)
	time.Sleep(50 * time.Millisecond) // a new modification time
	dropProbe(t, dir)
	r.waitOut(t, "app changed: session re-checked\n", 1, 2*time.Second)
	r.waitOut(t, "dropped let n: ", 1, time.Second)
}

func TestRepl_InterruptDuringEval(t *testing.T) {
	needGoTool(t)
	t.Chdir(t.TempDir())
	r := startRepl(t, todosDir(t))
	r.send(t, `List.range 1 100000 |> List.map (\i -> List.range 1 100000 |> List.length) |> List.sum`)
	time.Sleep(1500 * time.Millisecond) // built and running (the build alone stops too)
	start := time.Now()
	r.intr <- struct{}{}
	r.waitOut(t, "alder repl: interrupted\n", 1, 10*time.Second)
	if d := time.Since(start); d > 5*time.Second {
		t.Errorf("the interrupt took %v", d)
	}
	r.send(t, "1 + 1")
	r.waitOut(t, "2 : Int\n", 1, 30*time.Second)
}

func TestRepl_InterruptAtPrompt(t *testing.T) {
	t.Chdir(t.TempDir())
	r := startRepl(t, todosDir(t))
	r.waitOut(t, "alder> ", 1, 10*time.Second)
	r.intr <- struct{}{}
	r.waitOut(t, "(use :quit or Ctrl-D to leave)\n", 1, 5*time.Second)
	r.waitOut(t, "alder> ", 2, time.Second)
	r.send(t, ":type 1")
	r.waitOut(t, "1 : Int\n", 1, 10*time.Second)
}

func TestRepl_EOFQuits(t *testing.T) {
	out := replTodos(t, "let x = 1\n")
	if !strings.HasSuffix(out, "alder> \n") {
		t.Errorf("want the last prompt ended at end of input:\n%s", out)
	}
	if code, _ := runRepl(t, ":q\n:type 1\n", todosDir(t)); code != 0 {
		t.Errorf(":q: exit %d", code)
	}
	_, out = runRepl(t, ":quit\n:type 1\n", todosDir(t))
	if strings.Contains(out, "1 : Int") {
		t.Errorf("read on after :quit:\n%s", out)
	}
}

func TestRepl_UnknownCommand(t *testing.T) {
	out := replTodos(t, ":foo\n:help\n")
	wantIn(t, out, "unknown command :foo (try :help)\n", ":run! expr", ":reload")
}

func TestRepl_Usage(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, args := range [][]string{{"."}, {todosDir(t), "--json"}, {todosDir(t), "extra"}, {todosDir(t), "--module", "lower"}} {
		if code, out := runRepl(t, "", args...); code != 2 {
			t.Errorf("%q: exit %d, want 2\n%s", args, code, out)
		}
	}
	code, out := runRepl(t, "", todosDir(t), "--module", "Todo")
	if code != 2 || !strings.Contains(out, "the app already has a module named Todo") {
		t.Errorf("--module Todo: exit %d\n%s", code, out)
	}
}
