package scratch

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// printMain prints Scratch's `it` (the emitter names it Scratch__it).
const printMain = `package main

import (
	"fmt"

	"app"
)

func main() { fmt.Println(app.Scratch__it) }
`

func needGo(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not on PATH")
	}
}

func TestCompile_PureValueAgainstTodos(t *testing.T) {
	needGo(t)
	b := Compile("../../examples/todos", &Module{Name: "Scratch", Source: "it : Int\nit = Todo.countOpen []\n"}, printMain, "", t.TempDir())
	if b.Err != nil || len(b.Diags) != 0 {
		t.Fatalf("%v %v", b.Err, b.Diags)
	}
	out, stderr, code := Run(b, nil, "")
	if code != 0 || strings.TrimSpace(out) != "0" {
		t.Fatalf("%q %q %d", out, stderr, code)
	}
}

func TestCompile_DiagnosticsMapToExtraModule(t *testing.T) {
	b := Compile("../../examples/todos", &Module{Name: "Scratch", Source: "it : Int\nit = \"x\"\n"}, printMain, "", t.TempDir())
	if b.Err != nil {
		t.Fatal(b.Err)
	}
	if len(b.Diags) != 1 || b.Diags[0].Code != "E0003" || b.Diags[0].Span.Start.Line-b.LineOffset != 2 {
		t.Fatalf("offset %d: %+v", b.LineOffset, b.Diags)
	}
	if !strings.HasPrefix(b.Diags[0].Span.File, "Scratch") {
		t.Fatal(b.Diags[0].Span.File)
	}
	if b.Binary != "" {
		t.Fatalf("binary %q despite an error", b.Binary)
	}
	// a source with its own module line and an import of its own: only the missing imports are
	// added, after the module line; an unused import the caller wrote is still reported
	b = Compile("../../examples/todos", &Module{Name: "Scratch", Source: "-- a comment\nmodule Scratch\nimport Todo\n\nit : Int\nit = \"x\"\n"}, printMain, "", t.TempDir())
	if len(b.Diags) != 2 || b.Diags[0].Code != "W0001" || b.Diags[0].Span.Start.Line-b.LineOffset != 3 ||
		b.Diags[1].Code != "E0003" || b.Diags[1].Span.Start.Line-b.LineOffset != 6 {
		t.Fatalf("offset %d: %+v", b.LineOffset, b.Diags)
	}
}

// An error in the app is a diagnostic naming the app's file, not a build error; after it is fixed
// a Compile into the same directory succeeds.
func TestCompile_AppErrorIsDiagnostic(t *testing.T) {
	needGo(t)
	app := filepath.Join(t.TempDir(), "todos")
	if err := os.CopyFS(app, os.DirFS("../../examples/todos")); err != nil {
		t.Fatal(err)
	}
	todo := filepath.Join(app, "app", "Todo.alder")
	orig, err := os.ReadFile(todo)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(todo, append(append([]byte{}, orig...), "\nbroken : Int\nbroken = \"x\"\n"...), 0o644)
	out := t.TempDir()
	extra := &Module{Name: "Scratch", Source: "it : Int\nit = Todo.countOpen []\n"}
	b := Compile(app, extra, printMain, "", out)
	if b.Err != nil || len(b.Diags) == 0 || b.Binary != "" {
		t.Fatalf("err %v binary %q diags %+v", b.Err, b.Binary, b.Diags)
	}
	if b.Diags[0].Span.File != "app/Todo.alder" {
		t.Fatalf("diagnostic file %q", b.Diags[0].Span.File)
	}
	os.WriteFile(todo, orig, 0o644)
	b = Compile(app, extra, printMain, "", out)
	if b.Err != nil || len(b.Diags) != 0 || b.Binary == "" {
		t.Fatalf("after the fix: %v %+v", b.Err, b.Diags)
	}
	if got, _, code := Run(b, nil, ""); code != 0 || strings.TrimSpace(got) != "0" {
		t.Fatalf("%q %d", got, code)
	}
}

// A second Compile into the same directory reuses Go's build cache.
func TestCompile_ReusesCache(t *testing.T) {
	needGo(t)
	out := t.TempDir()
	extra := &Module{Name: "Scratch", Source: "it : Int\nit = Todo.countOpen []\n"}
	start := time.Now()
	if b := Compile("../../examples/todos", extra, printMain, "", out); b.Err != nil || b.Binary == "" {
		t.Fatalf("%v %v", b.Err, b.Diags)
	}
	first := time.Since(start)
	start = time.Now()
	if b := Compile("../../examples/todos", extra, printMain, "", out); b.Err != nil || b.Binary == "" {
		t.Fatalf("%v %v", b.Err, b.Diags)
	}
	second := time.Since(start)
	const floor = 3 * time.Second // a loaded machine may be slow even with every package cached
	if second > first/2 && second > floor {
		t.Fatalf("second compile %v, first %v: the cache is not reused", second, first)
	}
	t.Logf("first %v, second %v", first, second)
}

func TestCompile_NoExtraModule(t *testing.T) {
	needGo(t)
	main := "package main\n\nimport (\n\t\"fmt\"\n\n\t\"app\"\n)\n\nfunc main() { fmt.Println(len(app.Tables)) }\n"
	b := Compile("../../examples/todos", nil, main, "", t.TempDir())
	if b.Err != nil || len(b.Diags) != 0 || b.LineOffset != 0 {
		t.Fatalf("%v %v", b.Err, b.Diags)
	}
	if out, _, code := Run(b, nil, ""); code != 0 || strings.TrimSpace(out) != "2" {
		t.Fatalf("%q %d", out, code)
	}
}

// A main that does not compile is a build error carrying Go's message.
func TestCompile_BadMainIsErr(t *testing.T) {
	needGo(t)
	b := Compile("../../examples/todos", nil, "package main\n\nfunc main() { undefinedThing() }\n", "", t.TempDir())
	if b.Err == nil || b.Binary != "" || !strings.Contains(b.Err.Error(), "undefinedThing") {
		t.Fatalf("%v %q", b.Err, b.Binary)
	}
}

func TestRun_EnvStdinExit(t *testing.T) {
	needGo(t)
	main := "package main\n\nimport (\n\t\"fmt\"\n\t\"io\"\n\t\"os\"\n\n\t_ \"app\"\n)\n\nfunc main() {\n\tb, _ := io.ReadAll(os.Stdin)\n\tfmt.Print(os.Getenv(\"SCRATCH_X\"), string(b))\n\tfmt.Fprint(os.Stderr, \"e\")\n\tos.Exit(3)\n}\n"
	b := Compile("../../examples/todos", nil, main, "", t.TempDir())
	if b.Err != nil {
		t.Fatal(b.Err)
	}
	out, errOut, code := Run(b, []string{"SCRATCH_X=a"}, "b")
	if out != "ab" || errOut != "e" || code != 3 {
		t.Fatalf("%q %q %d", out, errOut, code)
	}
}
