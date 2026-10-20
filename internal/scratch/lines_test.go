package scratch

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"alder/internal/diag"
)

func TestSourceLine_Regions(t *testing.T) {
	// a source whose module line is line 1, with three imports inserted as lines 2-4
	b := Build{InsertLine: 2, LineOffset: 3, ExtraFile: "Scratch.alder"}
	for l, want := range map[int]int{1: 1, 2: 0, 3: 0, 4: 0, 5: 2, 9: 6} {
		got, ok := b.SourceLine(l)
		if ok != (want != 0) || got != want {
			t.Errorf("SourceLine(%d) = %d, %v; want %d", l, got, ok, want)
		}
	}
	// synthesized module line and imports at the top: lines 1-2 inserted
	b = Build{InsertLine: 1, LineOffset: 2, ExtraFile: "Scratch.alder"}
	if _, ok := b.SourceLine(1); ok {
		t.Error("the synthesized module line maps to the source")
	}
	if l, ok := b.SourceLine(3); !ok || l != 1 {
		t.Errorf("SourceLine(3) = %d, %v", l, ok)
	}
	// nothing inserted
	if l, ok := (Build{}).SourceLine(7); !ok || l != 7 {
		t.Errorf("no insertion: %d %v", l, ok)
	}
}

func TestMapDiag(t *testing.T) {
	b := Build{InsertLine: 2, LineOffset: 3, ExtraFile: "Scratch.alder"}
	sp := func(file string, l int) diag.Span {
		return diag.Span{File: file, Start: diag.Pos{Line: l, Col: 1}, End: diag.Pos{Line: l, Col: 4}}
	}
	// after the insertion: span and fix move up
	d, ok := b.MapDiag(diag.Diagnostic{Code: "E0201", Span: sp("Scratch.alder", 7), Fix: diag.InsertAt(6, 1, "import X\n")})
	if !ok || d.Span.Start.Line != 4 || d.Span.End.Line != 4 || d.Fix == nil || d.Fix.Line != 3 || d.Fix.EndLine != 3 {
		t.Fatalf("after: %v %+v %+v", ok, d.Span, d.Fix)
	}
	// a fix landing on an inserted line is dropped; the diagnostic stays
	d, ok = b.MapDiag(diag.Diagnostic{Code: "E0201", Span: sp("Scratch.alder", 7), Fix: diag.InsertAt(4, 1, "import X\n")})
	if !ok || d.Fix != nil || d.Span.Start.Line != 4 {
		t.Fatalf("fix on an inserted line: %v %+v", ok, d)
	}
	// before the insertion (the module line): unchanged
	d, ok = b.MapDiag(diag.Diagnostic{Code: "E0011", Span: sp("Scratch.alder", 1)})
	if !ok || d.Span.Start.Line != 1 {
		t.Fatalf("before: %v %+v", ok, d.Span)
	}
	// on an inserted line: not the caller's text
	if _, ok = b.MapDiag(diag.Diagnostic{Code: "W0001", Span: sp("Scratch.alder", 3)}); ok {
		t.Fatal("a diagnostic on an inserted line mapped")
	}
	// E0009's "at line N" names the caller's line too
	d, ok = b.MapDiag(diag.Diagnostic{Code: "E0009", Span: sp("Scratch.alder", 8), Message: "`x` is already declared at line 6"})
	if !ok || d.Span.Start.Line != 5 || d.Message != "`x` is already declared at line 3" {
		t.Fatalf("E0009: %v %+v", ok, d)
	}
	// another file: unchanged
	orig := diag.Diagnostic{Code: "E0003", Span: sp("app/Todo.alder", 3), Fix: diag.InsertAt(3, 1, "x")}
	if d, ok = b.MapDiag(orig); !ok || d.Span != orig.Span || d.Fix != orig.Fix {
		t.Fatalf("app diagnostic changed: %+v", d)
	}
}

func TestCompile_MapDiagWithModuleLine(t *testing.T) {
	src := "-- a comment\nmodule Scratch\n\nit : Int\nit = \"x\"\n"
	b := Compile("../../examples/todos", &Module{Name: "Scratch", Source: src}, printMain, "", t.TempDir())
	if b.Err != nil || len(b.Diags) != 1 || b.InsertLine != 3 || b.ExtraFile != "Scratch.alder" || b.Runnable() {
		t.Fatalf("%v insert %d file %q %+v", b.Err, b.InsertLine, b.ExtraFile, b.Diags)
	}
	if d, ok := b.MapDiag(b.Diags[0]); !ok || d.Span.Start.Line != 5 {
		t.Fatalf("%v %+v", ok, d.Span)
	}
}

func TestCompile_DeclaredNameMismatch(t *testing.T) {
	b := Compile("../../examples/todos", &Module{Name: "Scratch", Source: "module Other\n\nit : Int\nit = 1\n"}, printMain, "", t.TempDir())
	if b.Err == nil || !strings.Contains(b.Err.Error(), "module Other") || len(b.Diags) != 0 || b.Runnable() {
		t.Fatalf("%v %+v", b.Err, b.Diags)
	}
}

func TestHeaderLines(t *testing.T) {
	src := "-- c\nmodule Scratch\n\nimport Todo\n-- c\nimport User\nit : Int\nimport Theme\n"
	modLine, declared, imports := headerLines(src)
	if modLine != 1 || declared != "Scratch" || strings.Join(imports, ",") != "Todo,User" {
		t.Fatalf("%d %q %v", modLine, declared, imports)
	}
	if modLine, declared, imports = headerLines("it : Int\nimport Todo\n"); modLine != -1 || declared != "" || len(imports) != 0 {
		t.Fatalf("no header: %d %q %v", modLine, declared, imports)
	}
	// only the header's imports are taken as the caller's: Todo after a declaration is inserted
	out, n, _ := withImports("module Scratch\nit : Int\nit = 1\n-- import Todo\n", "Scratch", []string{"App", "Todo"})
	if n != 1 || !strings.HasPrefix(out, "module Scratch\nimport Todo\n") {
		t.Fatalf("%d %q", n, out)
	}
}

// A program that leaves a process holding its output does not keep Run waiting.
func TestRun_GrandchildDoesNotHang(t *testing.T) {
	needGo(t)
	main := "package main\n\nimport (\n\t\"os\"\n\t\"os/exec\"\n\n\t_ \"app\"\n)\n\nfunc main() {\n\tc := exec.Command(\"sleep\", \"30\")\n\tc.Stdout, c.Stderr = os.Stdout, os.Stderr\n\tc.Start()\n}\n"
	b := Compile("../../examples/todos", nil, main, "", t.TempDir())
	defer b.Close()
	if !b.Runnable() {
		t.Fatal(b.Err, b.Diags)
	}
	start := time.Now()
	_, stderr, code := Run(b, nil, "")
	if took := time.Since(start); took > WaitDelay+5*time.Second || code == 0 {
		t.Fatalf("took %v, exit %d, stderr %q", took, code, stderr)
	}
}

// At the time limit the program and what it started are killed.
func TestRunTimeout_KillsGroup(t *testing.T) {
	needGo(t)
	main := "package main\n\nimport (\n\t\"os\"\n\t\"os/exec\"\n\n\t_ \"app\"\n)\n\nfunc main() {\n\tc := exec.Command(\"sleep\", \"30\")\n\tc.Stdout = os.Stdout\n\tc.Run()\n}\n"
	b := Compile("../../examples/todos", nil, main, "", t.TempDir())
	defer b.Close()
	if !b.Runnable() {
		t.Fatal(b.Err, b.Diags)
	}
	start := time.Now()
	_, stderr, code := RunTimeout(b, nil, "", time.Second)
	if took := time.Since(start); took > 4*time.Second || code != -1 || !strings.Contains(stderr, "timed out after 1s") {
		t.Fatalf("took %v, exit %d, stderr %q", took, code, stderr)
	}
	// a cancelled context (the caller's interrupt) kills the group too
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(500*time.Millisecond, cancel)
	start = time.Now()
	_, stderr, code = RunContext(ctx, b, nil, "")
	if took := time.Since(start); took > 3*time.Second || code != -1 || !strings.Contains(stderr, "was interrupted") {
		t.Fatalf("cancel: took %v, exit %d, stderr %q", took, code, stderr)
	}
}

// timedWriter records when each write arrived.
type timedWriter struct {
	mu    sync.Mutex
	buf   strings.Builder
	first time.Time
}

func (w *timedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.first.IsZero() {
		w.first = time.Now()
	}
	return w.buf.Write(p)
}

// RunStream hands the program's output over as it is written, and passes its stdin through.
func TestRunStream(t *testing.T) {
	needGo(t)
	main := "package main\n\nimport (\n\t\"fmt\"\n\t\"io\"\n\t\"os\"\n\t\"time\"\n\n\t_ \"app\"\n)\n\nfunc main() {\n\tfmt.Println(\"first\")\n\tfmt.Fprintln(os.Stderr, \"note\")\n\ttime.Sleep(time.Second)\n\tb, _ := io.ReadAll(os.Stdin)\n\tfmt.Print(string(b))\n\tos.Exit(3)\n}\n"
	b := Compile("../../examples/todos", nil, main, "", t.TempDir())
	defer b.Close()
	if !b.Runnable() {
		t.Fatal(b.Err, b.Diags)
	}
	var out, errw timedWriter
	start := time.Now()
	code := RunStream(context.Background(), b, nil, strings.NewReader("in\n"), &out, &errw)
	end := time.Now()
	if code != 3 || out.buf.String() != "first\nin\n" || errw.buf.String() != "note\n" {
		t.Fatalf("exit %d out %q err %q", code, out.buf.String(), errw.buf.String())
	}
	if end.Sub(out.first) < 700*time.Millisecond || out.first.Before(start) {
		t.Fatalf("the first line arrived %v before the end, want it streamed", end.Sub(out.first))
	}
	// RunContext keeps its contract on top of it
	o, e, code := RunContext(context.Background(), b, nil, "x")
	if code != 3 || o != "first\nx" || e != "note\n" {
		t.Fatalf("RunContext: exit %d out %q err %q", code, o, e)
	}
}

// RunTethered gives the program EnvStdinEOF=1 and a stdin pipe that stays open for the run (a
// main honouring the variable keeps running until it is done), and keeps RunContext's
// contract.
func TestRunTethered(t *testing.T) {
	needGo(t)
	main := "package main\n\nimport (\n\t\"fmt\"\n\t\"os\"\n\t\"time\"\n\n\t_ \"app\"\n)\n\nfunc main() {\n\tgo func() {\n\t\tb := make([]byte, 1)\n\t\t_, err := os.Stdin.Read(b)\n\t\tfmt.Fprintln(os.Stderr, \"stdin:\", err)\n\t}()\n\ttime.Sleep(300 * time.Millisecond)\n\tfmt.Print(os.Getenv(\"" + EnvStdinEOF + "\"))\n\tos.Exit(3)\n}\n"
	b := Compile("../../examples/todos", nil, main, "", t.TempDir())
	defer b.Close()
	if !b.Runnable() {
		t.Fatal(b.Err, b.Diags)
	}
	// rt's init cleared the variable (a process the program starts does not inherit it)
	o, e, code := RunTethered(context.Background(), b, []string{"X=1"})
	if code != 3 || o != "" || e != "" {
		t.Fatalf("exit %d out %q err %q (want stdin open for the whole run)", code, o, e)
	}
	// stdin at end of file (the parent gone): the program says so and exits 2
	var out, errOut strings.Builder
	code = RunStream(context.Background(), b, []string{EnvStdinEOF + "=1"}, strings.NewReader(""), &out, &errOut)
	// the main's own "stdin: EOF" line and the tether's line race for stderr: either order is fine
	if code != 2 || !strings.Contains(errOut.String(), "alder: parent gone\n") {
		t.Fatalf("stdin closed: exit %d out %q err %q", code, out.String(), errOut.String())
	}
}

func TestMapDiag_BaseLines(t *testing.T) {
	b := Build{ExtraFile: "app/Todo.alder", BaseLines: 40, InsertLine: 1, LineOffset: 40}
	own := diag.Diagnostic{Span: diag.Span{File: "app/Todo.alder", Start: diag.Pos{Line: 7, Col: 1}, End: diag.Pos{Line: 7, Col: 3}}}
	if m, ok := b.MapDiag(own); !ok || m.Span.Start.Line != 7 {
		t.Errorf("a module line is left alone: %+v %v", m, ok)
	}
	in := diag.Diagnostic{Span: diag.Span{File: "app/Todo.alder", Start: diag.Pos{Line: 42, Col: 1}, End: diag.Pos{Line: 42, Col: 3}}}
	if m, ok := b.MapDiag(in); !ok || m.Span.Start.Line != 2 || m.Span.End.Line != 2 {
		t.Errorf("an input line maps to line-BaseLines: %+v %v", m, ok)
	}
}

func TestInSession(t *testing.T) {
	at := func(file string, line int) diag.Diagnostic {
		return diag.Diagnostic{Span: diag.Span{File: file, Start: diag.Pos{Line: line, Col: 1}, End: diag.Pos{Line: line, Col: 2}}}
	}
	in := Build{ExtraFile: "app/Todo.alder", BaseLines: 40, InsertLine: 1, LineOffset: 40}
	scr := Build{ExtraFile: "Scratch.alder", InsertLine: 1, LineOffset: 3}
	cases := []struct {
		b    Build
		d    diag.Diagnostic
		want bool
	}{
		{in, at("app/Todo.alder", 41), true},  // the session's first line
		{in, at("app/Todo.alder", 40), false}, // the module's last line
		{in, at("app/Todo.alder", 7), false},
		{in, at("app/User.alder", 41), false}, // another module
		{scr, at("Scratch.alder", 1), true},   // the extra module, any line
		{scr, at("app/Todo.alder", 5), false},
		{Build{}, at("Scratch.alder", 1), false}, // no extra module
	}
	for i, c := range cases {
		if got := c.b.InSession(c.d); got != c.want {
			t.Errorf("case %d: InSession(%s:%d) = %v, want %v", i, c.d.Span.File, c.d.Span.Start.Line, got, c.want)
		}
		ch := Checked{ExtraFile: c.b.ExtraFile, BaseLines: c.b.BaseLines, InsertLine: c.b.InsertLine, LineOffset: c.b.LineOffset}
		if got := ch.InSession(c.d); got != c.want {
			t.Errorf("case %d: Checked.InSession = %v, want %v", i, got, c.want)
		}
	}
}
