package scratch

import (
	"context"
	"errors"
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
	defer b.Close()
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
	defer b.Close()
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
	b := Compile("../../examples/todos", extra, printMain, "", out)
	if b.Err != nil || b.Binary == "" {
		t.Fatalf("%v %v", b.Err, b.Diags)
	}
	b.Close()
	first := time.Since(start)
	start = time.Now()
	b = Compile("../../examples/todos", extra, printMain, "", out)
	if b.Err != nil || b.Binary == "" {
		t.Fatalf("%v %v", b.Err, b.Diags)
	}
	b.Close()
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
	defer b.Close()
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
	defer b.Close()
	if b.Err != nil {
		t.Fatal(b.Err)
	}
	out, errOut, code := Run(b, []string{"SCRATCH_X=a"}, "b")
	if out != "ab" || errOut != "e" || code != 3 {
		t.Fatalf("%q %q %d", out, errOut, code)
	}
}

// The inferred module may have any name; an app that already has a module of that name is an
// Err naming it, never a diagnostic pinned on the extra module's first line.
func TestCheck_ModuleName(t *testing.T) {
	needGo(t)
	app := filepath.Join(t.TempDir(), "todos")
	if err := os.CopyFS(app, os.DirFS("../../examples/todos")); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(app, "app", "Scratch.alder"), []byte("module Scratch\n\nx : Int\nx = 1\n"), 0o644)
	c := Check(app, &Module{Name: "Scratch", Source: "it = 1\n", Infer: true})
	var exists *ModuleExistsError
	if !errors.As(c.Err, &exists) || exists.Name != "Scratch" || len(c.Diags) != 0 {
		t.Fatalf("err %v, diags %v", c.Err, c.Diags)
	}
	main := strings.ReplaceAll(printMain, "Scratch__it", "Probe__it")
	b := Compile(app, &Module{Name: "Probe", Source: "it = Scratch.x + 1\n", Infer: true}, main, "", t.TempDir())
	defer b.Close()
	if !b.Runnable() {
		t.Fatalf("%v %v", b.Err, b.Diags)
	}
	if out, _, code := Run(b, nil, ""); code != 0 || strings.TrimSpace(out) != "2" {
		t.Fatalf("%q %d", out, code)
	}
}

// CompileChecked builds what Check checked, without checking again.
func TestCompileChecked(t *testing.T) {
	needGo(t)
	c := Check("../../examples/todos", &Module{Source: "it = Todo.countOpen []\n", Infer: true})
	if c.Err != nil || len(c.Diags) != 0 || c.Result == nil {
		t.Fatalf("%v %v", c.Err, c.Diags)
	}
	b := CompileChecked(c, printMain, "", t.TempDir())
	defer b.Close()
	if out, _, code := Run(b, nil, ""); code != 0 || strings.TrimSpace(out) != "0" {
		t.Fatalf("%q %d %v %v", out, code, b.Err, b.Diags)
	}
}

// A Runnable Build holds the lock on its directory until Close: a second Compile into it waits.
func TestCompile_LockHeldUntilClose(t *testing.T) {
	needGo(t)
	out := t.TempDir()
	extra := &Module{Source: "it = 1\n", Infer: true}
	b := Compile("../../examples/todos", extra, printMain, "", out)
	if !b.Runnable() {
		t.Fatalf("%v %v", b.Err, b.Diags)
	}
	if _, err := os.Stat(out + ".lock"); err != nil {
		t.Fatal(err)
	}
	done := make(chan Build)
	go func() { done <- Compile("../../examples/todos", extra, printMain, "", out) }()
	select {
	case b2 := <-done:
		b2.Close()
		t.Fatal("a second Compile did not wait for Close")
	case <-time.After(500 * time.Millisecond):
	}
	if got, _, code := Run(b, nil, ""); code != 0 || strings.TrimSpace(got) != "1" {
		t.Fatalf("%q %d", got, code)
	}
	b.Close()
	b.Close() // twice is fine
	b2 := <-done
	defer b2.Close()
	if !b2.Runnable() {
		t.Fatalf("%v %v", b2.Err, b2.Diags)
	}
	// a Compile that would wait says so, and stops waiting when its context is done
	c := Check("../../examples/todos", extra)
	waited := make(chan struct{}, 2)
	ctx, cancel := context.WithCancel(context.Background())
	got := make(chan Build)
	go func() {
		got <- CompileCheckedWith(ctx, c, printMain, "", out, Options{Waiting: func() { waited <- struct{}{} }})
	}()
	select {
	case <-waited:
	case b3 := <-got:
		b3.Close()
		t.Fatal("a Compile did not wait for the lock")
	case <-time.After(5 * time.Second):
		t.Fatal("Waiting was not called")
	}
	cancel()
	select {
	case b3 := <-got:
		if b3.Runnable() || !errors.Is(b3.Err, context.Canceled) {
			t.Fatalf("after cancel: runnable %v, err %v", b3.Runnable(), b3.Err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the wait for the lock did not stop when the context was done")
	}
	// a Build that is not Runnable holds nothing: two failed Compiles in a row do not wait
	bad := &Module{Source: "it = 1 + \"x\"\n", Infer: true}
	b2.Close()
	for range 2 {
		if b := Compile("../../examples/todos", bad, printMain, "", out); b.Runnable() || len(b.Diags) == 0 {
			t.Fatal("bad module compiled")
		}
	}
}

// A done context stops the build: no binary, and Err is the context's error.
func TestCompileCheckedWith_Cancelled(t *testing.T) {
	needGo(t)
	c := Check("../../examples/todos", &Module{Source: "it = 1\n", Infer: true})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	b := CompileCheckedWith(ctx, c, printMain, "", t.TempDir(), Options{})
	defer b.Close()
	if b.Runnable() || !errors.Is(b.Err, context.Canceled) {
		t.Fatalf("runnable %v, err %v", b.Runnable(), b.Err)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Errorf("the cancelled build took %v", d)
	}
}

// With NoVet a main that builds but does not vet runs; one that does not build either is told
// apart from a generated-code failure by the vet CompileCheckedWith then runs: the main or the
// adapters "does not vet", as without NoVet.
func TestCompileCheckedWith_NoVet(t *testing.T) {
	needGo(t)
	extra := &Module{Name: "Scratch", Source: "it : Int\nit = Todo.countOpen []\n"}
	vetBad := strings.Replace(printMain, "fmt.Println(app.Scratch__it)", `fmt.Printf("%d\n", "x"); fmt.Println(app.Scratch__it)`, 1)
	out := t.TempDir()
	c := Check("../../examples/todos", extra)
	if b := CompileCheckedWith(context.Background(), c, vetBad, "", out, Options{}); b.Err == nil || !strings.Contains(b.Err.Error(), "does not vet") {
		b.Close()
		t.Fatalf("with vet: %v", b.Err)
	}
	b := CompileCheckedWith(context.Background(), c, vetBad, "", out, Options{NoVet: true})
	if b.Err != nil || !b.Runnable() {
		t.Fatalf("no vet: %v", b.Err)
	}
	if got, _, code := Run(b, nil, ""); code != 0 || !strings.HasSuffix(strings.TrimSpace(got), "0") {
		t.Errorf("%q %d", got, code)
	}
	b.Close()
	broken := strings.Replace(printMain, "fmt.Println(app.Scratch__it)", `var n int = "x"; fmt.Println(n, app.Scratch__it)`, 1)
	b = CompileCheckedWith(context.Background(), c, broken, "", out, Options{NoVet: true})
	defer b.Close()
	if b.Err == nil || !strings.Contains(b.Err.Error(), "(the main or the adapters) does not vet") || b.Runnable() {
		t.Errorf("neither vets nor builds: %v", b.Err)
	}
}
