package main

import (
	"bytes"
	"fmt"
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
	code := runReplTo(withBuildDir(t, args), strings.NewReader(input), &out, &out, nil)
	return code, out.String()
}

// replTodos runs the REPL over input against examples/todos from a fresh working directory
// and wants exit 0.
func replTodos(t *testing.T, input string) string {
	t.Helper()
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
	go func() { r.done <- runReplTo(withBuildDir(t, []string{dir}), pr, r.out, r.out, r.intr) }()
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
	t.Parallel()
	needGoTool(t)
	out := replTodos(t, "let x = 1\nx + 2\n")
	wantIn(t, out, "alder> x : Int\n", "alder> 3 : Int\n")
}

func TestRepl_Type(t *testing.T) {
	t.Parallel()
	out := replTodos(t, ":type Todo.countOpen\n")
	wantIn(t, out, "Todo.countOpen : List Todo -> Int\n")
}

func TestRepl_Diagnostic(t *testing.T) {
	t.Parallel()
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
	if strings.Contains(out, "alder> y : ") { // the line a kept let prints
		t.Errorf("a let with an error was kept:\n%s", out)
	}
	// what follows `let` is not a name
	out = replTodos(t, "let = 4\nlet x 3 = 4\n")
	wantIn(t, out, "alder repl: a let name starts with a lowercase letter or `_` (`let todo = …`)\n", "let x (line 2): error[")
	if strings.Contains(out, "alder> x : ") {
		t.Errorf("a let with an error was kept:\n%s", out)
	}
}

// A let's name is a value's: an uppercase one would be shadowed by the module or type of that
// name; `it` is the expression's.
func TestRepl_LetNames(t *testing.T) {
	t.Parallel()
	out := replTodos(t, "let Todo = 1\nlet ünï = 1\nlet it = 1\n:type Todo.countOpen\n")
	if n := strings.Count(out, "alder repl: a let name starts with a lowercase letter or `_` (`let todo = …`)\n"); n != 2 {
		t.Errorf("want the lowercase rule twice, got %d:\n%s", n, out)
	}
	wantIn(t, out, "alder repl: a let may not be named `it`, the name the REPL gives the expression; let another name\n",
		"Todo.countOpen : List Todo -> Int\n")
	if strings.Contains(out, "input line") {
		t.Errorf("the REPL says input line:\n%s", out)
	}
}

// Only the lets an expression uses are computed with it: a let that fails at run time breaks
// the evaluations that use it, named, and no other.
func TestRepl_UnusedLetsNotComputed(t *testing.T) {
	t.Parallel()
	needGoTool(t)
	out := replTodos(t, "let loop n = loop (n + 1)\nlet boom = loop 0\nlet k = 40\n1 + 1\nk + 2\nboom + 1\n")
	wantIn(t, out, "alder> 2 : Int\n", "alder> 42 : Int\n",
		"alder repl: stack overflow (unbounded recursion?) (the lets it uses — loop, boom — are computed with it)\n")
}

func TestReachable(t *testing.T) {
	t.Parallel()
	mk := func(name, src string) replEntry {
		return replEntry{item: item{Let: name != "", Name: name, Lines: []string{src}}}
	}
	lets := []replEntry{mk("a", "a = 1"), mk("b", "b = a + 1"), mk("c", `c = "b"`), mk("d", "d = 4"), mk("e", "e = c")}
	var names []string
	for _, l := range reachable(lets, mk("", "it = e + 1")) {
		names = append(names, l.Name)
	}
	// e uses c; c's string mentions b, which is taken (over-taking is fine), and b uses a
	if got := strings.Join(names, ","); got != "a,b,c,e" {
		t.Errorf("got %s", got)
	}
}

// A redefinition that a later let no longer checks with is refused, the let named.
func TestRepl_RedefineRefused(t *testing.T) {
	t.Parallel()
	out := replTodos(t, "let a = 1\nlet b = a + 1\nlet a = \"x\"\n:type a\n")
	wantIn(t, out, "alder repl: a not redefined: let b (line 2) uses it: error[E0003]", "a : Int\n")
	if strings.Contains(out, "redefined a\n") {
		t.Errorf("redefined:\n%s", out)
	}
}

// A command's name ends at the first character that is not a letter or `!`; :type prints its
// expression on one line.
func TestRepl_CommandSyntax(t *testing.T) {
	t.Parallel()
	out := replTodos(t, ":type(1)\n:run!(Db.from Todo |> Db.all)\n:type [1,\\\n  2]\n")
	wantIn(t, out, "(1) : Int\n", "`:run!` wants a Task; use `:run` for a Query", "[1, 2] : List Int\n")
}

// A line longer than the limit is refused, and the REPL reads on.
func TestRepl_LineTooLong(t *testing.T) {
	t.Parallel()
	out := replTodos(t, strings.Repeat("x", maxLine+1)+"\n:type 1\n")
	wantIn(t, out, "alder repl: line too long (limit 1 MB); ignored\n", "1 : Int\n")
}

// The REPL builds in a directory of its own, removed when it ends; it removes those of REPLs
// that are gone, and keeps those of live ones.
func TestRepl_BuildDirs(t *testing.T) {
	t.Parallel()
	needGoTool(t)
	scratchDir := filepath.Join(testBuildDir(t), "todos", "scratch")
	gone := filepath.Join(scratchDir, "repl-99999999") // no such pid
	zero := filepath.Join(scratchDir, "repl-0")        // not a process (kill(0, 0) is the caller's group)
	live := filepath.Join(scratchDir, "repl-1")        // init
	for _, d := range []string{gone, zero, live} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(d+".lock", nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	r := startRepl(t, todosDir(t))
	r.send(t, "1 + 1")
	r.waitOut(t, "2 : Int\n", 1, 30*time.Second)
	own := filepath.Join(scratchDir, fmt.Sprintf("repl-%d", os.Getpid()))
	if _, err := os.Stat(filepath.Join(own, "scratch")); err != nil {
		t.Errorf("the REPL's build: %v", err)
	}
	for _, p := range []string{gone, gone + ".lock", zero, zero + ".lock"} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("%s: %v, want removed", p, err)
		}
	}
	for _, p := range []string{live, live + ".lock"} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("%s of a live pid: %v", p, err)
		}
	}
	r.in.Close()
	select {
	case code := <-r.done:
		r.done <- code // for startRepl's cleanup
	case <-time.After(10 * time.Second):
		t.Fatal("the REPL did not end")
	}
	for _, p := range []string{own, own + ".lock"} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("%s after the REPL ended: %v", p, err)
		}
	}
}

func TestPidAlive(t *testing.T) {
	t.Parallel()
	if !pidAlive(os.Getpid()) {
		t.Error("this process is not alive")
	}
	for _, pid := range []int{0, -1, 99999999} {
		if pidAlive(pid) {
			t.Errorf("pid %d alive", pid)
		}
	}
}

func TestRepl_Redefine(t *testing.T) {
	t.Parallel()
	out := replTodos(t, "let x = 1\nlet x = \"a\"\n:type x\nlet x = x + 1\n:type x\n")
	wantIn(t, out, "x : Int\n", "redefined x\n", "x : Text\n")
	if strings.Count(out, "x : Text\n") != 3 { // the let, :type, and :type after the failed redefinition
		t.Errorf("want x : Text three times:\n%s", out)
	}
}

// Redefining a let replaces it in place: the lets that use it see the new definition.
func TestRepl_RedefineDependents(t *testing.T) {
	t.Parallel()
	needGoTool(t)
	out := replTodos(t, "let a = 1\nlet b = a + 1\nlet a = 5\nb\n")
	wantIn(t, out, "redefined a\na : Int\n", "6 : Int\n")
}

func TestRepl_Reset(t *testing.T) {
	t.Parallel()
	out := replTodos(t, "let x = 1\n:reset\n:type x\n")
	wantIn(t, out, "x : Int\n", "expr:1:1: error[E0002]")
}

func TestRepl_Continuation(t *testing.T) {
	t.Parallel()
	out := replTodos(t, "let xs = [1,\\\n2]\n:type xs\nlet ys = [1,\\\n\"a\"]\n")
	wantIn(t, out, "    …> ", "xs : List Int\n", "xs : List Int\n",
		// line 5 is the let's second line, entered at line 4
		"let ys (line 5): error[", "  \"a\"]\n  ^")
}

func TestRepl_QueryHints(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	needGoTool(t)
	out := replTodos(t, ":run (Db.from Todo |> Db.count)\n")
	wantIn(t, out, "0 : Int\n")
}

func TestRepl_RunBangAndFailure(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	needGoTool(t)
	out := replTodos(t, ":html <p>hi</p>\n")
	wantIn(t, out, "alder> <p>hi</p>\n")
}

// replTemp copies examples/todos to a temporary directory, makes it the working directory,
// gives the copy's Todo module a function probe nothing in the app uses, and returns the
// copy's directory.
func replTemp(t *testing.T) string {
	t.Helper()
	return tempTodos(t, "Table/Todo.alder", "\nprobe : Int -> Int\nprobe n = n + 1\n")
}

// dropProbe removes probe from the copy's Todo module.
func dropProbe(t *testing.T, dir string) {
	t.Helper()
	path := filepath.Join(dir, "app", "Table", "Todo.alder")
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
	t.Parallel()
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
	if strings.Contains(r.out.String(), "dropped let k") {
		t.Errorf("k was dropped:\n%s", r.out.String())
	}
	// the watcher sees the edit too, but the :reload already re-checked it: no second notice
	time.Sleep(time.Second)
	if strings.Contains(r.out.String(), "app changed") {
		t.Errorf("a second notice after :reload:\n%s", r.out.String())
	}
}

// An app with an error: its diagnostics are printed once, then each line says it in one line;
// :reload keeps the session; fixing the file makes the REPL work.
func TestRepl_BrokenApp(t *testing.T) {
	t.Parallel()
	needGoTool(t)
	dir := replTemp(t)
	path := filepath.Join(dir, "app", "Table", "Todo.alder")
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

// A let dropped on reload drops the lets that use it, in turn.
func TestRepl_ReloadDropsDependents(t *testing.T) {
	t.Parallel()
	dir := replTemp(t)
	r := startRepl(t, dir)
	r.send(t, "let n = Todo.probe 1")
	r.send(t, "let m = n + 1")
	r.send(t, "let k = 2")
	r.waitOut(t, "k : Int\n", 1, 10*time.Second)
	dropProbe(t, dir)
	r.send(t, ":reload")
	r.waitOut(t, "dropped let m: error[E0002]", 1, 10*time.Second)
	wantIn(t, r.out.String(), "dropped let n: error[")
	r.send(t, ":type k")
	r.waitOut(t, "k : Int\n", 2, 10*time.Second)
}

func TestRepl_AutoReload(t *testing.T) {
	t.Parallel()
	dir := replTemp(t)
	r := startRepl(t, dir)
	r.send(t, "let n = Todo.probe 1")
	r.waitOut(t, "n : Int\n", 1, 10*time.Second)
	time.Sleep(50 * time.Millisecond) // a new modification time
	dropProbe(t, dir)
	r.waitOut(t, "app changed: session re-checked\n", 1, 2*time.Second)
	r.waitOut(t, "dropped let n: ", 1, time.Second)
}

// longEval is a pure expression that runs for minutes.
const longEval = `List.range 1 100000 |> List.map (\i -> List.range 1 100000 |> List.length) |> List.sum`

func TestRepl_InterruptDuringEval(t *testing.T) {
	t.Parallel()
	needGoTool(t)
	r := startRepl(t, todosDir(t))
	r.send(t, longEval)
	// built (the directory is new: the executable is this build's), so running
	bin := filepath.Join(testBuildDir(t), "todos", "scratch", fmt.Sprintf("repl-%d", os.Getpid()), "scratch")
	deadline := time.Now().Add(60 * time.Second)
	for {
		if _, err := os.Stat(bin); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("not built within 60 s")
		}
		time.Sleep(20 * time.Millisecond)
	}
	start := time.Now()
	r.intr <- struct{}{}
	r.waitOut(t, "alder repl: interrupted\n", 1, 10*time.Second)
	if d := time.Since(start); d > 5*time.Second {
		t.Errorf("the interrupt took %v", d)
	}
	r.send(t, "1 + 1")
	r.waitOut(t, "2 : Int\n", 1, 30*time.Second)
}

// An interrupt during the build stops it (go build is killed).
func TestRepl_InterruptDuringBuild(t *testing.T) {
	t.Parallel()
	needGoTool(t)
	r := startRepl(t, todosDir(t))
	r.send(t, longEval)
	// building: the build's work directory appears before go build starts; cmd/main.go is
	// written just before it (the first build, vetted, in a new directory, takes longer than
	// the interrupt)
	work := filepath.Join(testBuildDir(t), "todos", "scratch", fmt.Sprintf("repl-%d", os.Getpid()), "cmd", "main.go")
	deadline := time.Now().Add(60 * time.Second)
	for {
		if _, err := os.Stat(work); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the build did not start within 60 s")
		}
		time.Sleep(5 * time.Millisecond)
	}
	start := time.Now()
	r.intr <- struct{}{}
	r.waitOut(t, "alder repl: interrupted\n", 1, 10*time.Second)
	if d := time.Since(start); d > 3*time.Second {
		t.Errorf("the interrupt took %v", d)
	}
}

func TestRepl_InterruptAtPrompt(t *testing.T) {
	t.Parallel()
	r := startRepl(t, todosDir(t))
	r.waitOut(t, "alder> ", 1, 10*time.Second)
	r.intr <- struct{}{}
	r.waitOut(t, "(use :quit or Ctrl-D to leave)\n", 1, 5*time.Second)
	r.waitOut(t, "alder> ", 2, time.Second)
	r.send(t, ":type 1")
	r.waitOut(t, "1 : Int\n", 1, 10*time.Second)
}

func TestRepl_EOFQuits(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	out := replTodos(t, ":foo\n:help\n")
	wantIn(t, out, "unknown command :foo (try :help)\n", ":run! expr", ":reload")
}

func TestRepl_Usage(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{{"."}, {todosDir(t), "--json"}, {todosDir(t), "extra"}, {todosDir(t), "--module", "lower"}} {
		if code, out := runRepl(t, "", args...); code != 2 {
			t.Errorf("%q: exit %d, want 2\n%s", args, code, out)
		}
	}
	code, out := runRepl(t, "", todosDir(t), "--module", "Layout")
	if code != 2 || !strings.Contains(out, "the app already has a module named Layout; give --module another name\n") {
		t.Errorf("--module Layout: exit %d\n%s", code, out)
	}
}

// A capability without an adapter does not stop the REPL: the warning is printed once, and
// expressions evaluate.
func TestRepl_MissingAdapter(t *testing.T) {
	t.Parallel()
	needGoTool(t)
	dir := tempTodos(t, "App.alder", "")
	if err := os.WriteFile(filepath.Join(dir, "app", "Mail.alder"), []byte(mailAlder), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out := runRepl(t, "1 + 1\n2 + 3\n", dir)
	if code != 0 || strings.Count(out, unwiredWarning) != 1 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	wantIn(t, out, "2 : Int", "5 : Int")
}

func TestRepl_InModule(t *testing.T) {
	t.Parallel()
	needGoTool(t)
	code, out := runRepl(t, "countOpen []\nlet n = countOpen []\nn + 1\n", todosDir(t), "--in", "Table.Todo")
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	wantIn(t, out, "alder repl: todos, in Table.Todo", "0 : Int\n", "n : Int\n", "1 : Int\n")
}

func TestRepl_InRefusesShadowingLet(t *testing.T) {
	t.Parallel()
	needGoTool(t)
	_, out := runRepl(t, "let countOpen = 1\ncountOpen []\n", todosDir(t), "--in", "Table.Todo")
	wantIn(t, out, "alder repl: Table.Todo already declares countOpen", "0 : Int\n")
}

func TestRepl_InCommandSwitchesAndKeepsLets(t *testing.T) {
	t.Parallel()
	needGoTool(t)
	// a let that uses Todo's names unqualified survives :in Todo and is dropped by :in (app scope)
	_, out := runRepl(t, "let a = 1\n:in Table.Todo\nlet n = countOpen []\n:in\nn\na\n", todosDir(t))
	wantIn(t, out, "in Table.Todo\n", "n : Int\n", "dropped let n:", "1 : Int\n")
	if strings.Contains(out, "dropped let a") {
		t.Errorf("a does not depend on the module:\n%s", out)
	}
}

func TestRepl_InUnknownModule(t *testing.T) {
	t.Parallel()
	needGoTool(t)
	code, out := runRepl(t, "", todosDir(t), "--in", "Nope")
	if code != 2 || !strings.Contains(out, "no module Nope") {
		t.Errorf("exit %d\n%s", code, out)
	}
	_, out = runRepl(t, ":in Nope\n1\n", todosDir(t))
	wantIn(t, out, "no module Nope", "1 : Int\n")
}

func TestRepl_InWithModuleIsUsage(t *testing.T) {
	t.Parallel()
	code, out := runRepl(t, "", todosDir(t), "--in", "Table.Todo", "--module", "Probe")
	if code != 2 || !strings.Contains(out, "--in") {
		t.Errorf("exit %d\n%s", code, out)
	}
}

// --in takes a nested module by its dotted name, as eval's does; :in too.
func TestRepl_InNestedModule(t *testing.T) {
	t.Parallel()
	needGoTool(t)
	code, out := runRepl(t, ":type update\n", todosDir(t), "--in", "Todos.Index")
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	wantIn(t, out, "alder repl: todos, in Todos.Index", "update : ")
	_, out = runRepl(t, ":in Todos.Index\n:type update\n:in todo\n", todosDir(t))
	wantIn(t, out, "in Todos.Index\n", "update : ", ":in wants an app module name")
}

// A broken declaration of the module itself is the app's error, named in app/Table/Todo.alder, under
// --in at startup; :in to it is refused as the app's errors, not as a missing module.
func TestRepl_InBrokenModule(t *testing.T) {
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
	code, out := runRepl(t, "", filepath.Join(tmp, "todos"), "--in", "Table.Todo")
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	wantIn(t, out, "app/Table/Todo.alder:", "the app has errors")
	if strings.Contains(out, "expr:") || strings.Contains(out, "let ") {
		t.Errorf("the module's own error is not the session's:\n%s", out)
	}
	_, out = runRepl(t, ":in Table.Todo\n", filepath.Join(tmp, "todos"))
	wantIn(t, out, "app/Table/Todo.alder:", "alder repl: not in Table.Todo: the app has errors")
	if strings.Contains(out, "no module") {
		t.Errorf("a broken module is not a missing one:\n%s", out)
	}
}

// TestReplPrintsDeprecation: the banner stays on standard output, as the editor plugin detects
// it there; the deprecation line follows it on standard error.
func TestReplPrintsDeprecation(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runReplTo(withBuildDir(t, []string{todosDir(t)}), strings.NewReader(""), &stdout, &stderr, nil)
	if code != 0 {
		t.Fatalf("exit %d\n%s%s", code, stdout.String(), stderr.String())
	}
	const line = "alder repl is deprecated: use a scratch file (alder scratch --watch, or save scratch.alder in the editor); it will be removed in the next release"
	if !strings.Contains(stderr.String(), line+"\n") {
		t.Errorf("stderr lacks the deprecation line:\n%s", stderr.String())
	}
	if strings.Contains(stdout.String(), "deprecated") {
		t.Errorf("stdout carries the deprecation:\n%s", stdout.String())
	}
	if !strings.HasPrefix(stdout.String(), "alder repl: todos (:help for the commands, :quit to leave)\n") {
		t.Errorf("the banner changed:\n%s", stdout.String())
	}
}
