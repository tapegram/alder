package scratch

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"alder/internal/diag"
)

// todos copies examples/todos to a temp dir so a test may edit it.
func todos(t *testing.T) string {
	t.Helper()
	dst := t.TempDir()
	if err := os.CopyFS(dst, os.DirFS("../../examples/todos")); err != nil {
		t.Fatal(err)
	}
	return dst
}

func TestCheck_InModuleSeesUnqualifiedNames(t *testing.T) {
	c := Check("../../examples/todos", &Module{In: "Todo", Source: "it = countOpen []\n", Infer: true})
	if c.Err != nil || len(c.Diags) != 0 || c.Result == nil {
		t.Fatalf("err %v, diags %v", c.Err, c.Diags)
	}
	if _, ok := c.Result.Values["Todo.it"]; !ok {
		t.Error("Todo.it was not inferred")
	}
	if c.ExtraFile != "app/Todo.alder" || c.BaseLines == 0 {
		t.Errorf("ExtraFile %q, BaseLines %d", c.ExtraFile, c.BaseLines)
	}
}

func TestCheck_InModuleMapsInputDiagnostics(t *testing.T) {
	c := Check("../../examples/todos", &Module{In: "Todo", Source: "it = countOpen 1\n", Infer: true})
	var got []diag.Diagnostic
	for _, d := range c.Diags {
		m, ok := c.MapDiag(d)
		if ok {
			got = append(got, m)
		}
	}
	if len(got) == 0 || got[0].Span.Start.Line != 1 {
		t.Fatalf("want the error on input line 1, got %+v", got)
	}
	if !c.InSession(c.Diags[0]) {
		t.Errorf("the input's diagnostic is not InSession: %+v", c.Diags[0])
	}
}

func TestCheck_InModuleKeepsModuleDiagnostics(t *testing.T) {
	dir := todos(t)
	p := filepath.Join(dir, "app", "Todo.alder")
	b, _ := os.ReadFile(p)
	os.WriteFile(p, append(b, []byte("\nbroken : Int\nbroken = \"x\"\n")...), 0o644)
	c := Check(dir, &Module{In: "Todo", Source: "it = 1\n", Infer: true})
	var found bool
	for _, d := range c.Diags {
		m, ok := c.MapDiag(d)
		if ok && m.Span.File == "app/Todo.alder" && m.Span.Start.Line <= c.BaseLines && m.Code == "E0003" {
			found = true
			if m.Span.Start.Line != d.Span.Start.Line || c.InSession(d) {
				t.Errorf("the module's own error was mapped or counted as the session's: %+v", m)
			}
		}
	}
	if !found {
		t.Errorf("the module's own error was not reported at its line: %+v", c.Diags)
	}
}

func TestCheck_InModuleNoTrailingNewline(t *testing.T) {
	dir := todos(t)
	p := filepath.Join(dir, "app", "Todo.alder")
	b, _ := os.ReadFile(p)
	os.WriteFile(p, append([]byte(strings.TrimRight(string(b), "\n")), []byte("\n-- trailing comment")...), 0o644)
	c := Check(dir, &Module{In: "Todo", Source: "it = countOpen []\n", Infer: true})
	if c.Err != nil || len(c.Diags) != 0 {
		t.Fatalf("err %v, diags %v", c.Err, c.Diags)
	}
}

func TestCheck_InModuleNoTrailingNewlineAfterCode(t *testing.T) {
	dir := todos(t)
	p := filepath.Join(dir, "app", "Todo.alder")
	b, _ := os.ReadFile(p)
	os.WriteFile(p, []byte(strings.TrimRight(string(b), "\n")), 0o644)
	c := Check(dir, &Module{In: "Todo", Source: "it = countOpen []\n", Infer: true})
	if c.Err != nil || len(c.Diags) != 0 {
		t.Fatalf("err %v, diags %v", c.Err, c.Diags)
	}
	if _, ok := c.Result.Values["Todo.it"]; !ok {
		t.Error("Todo.it was not inferred")
	}
}

func TestCheck_InModuleDeclares(t *testing.T) {
	c := Check("../../examples/todos", &Module{In: "Todo", Source: "it = 1\n", Infer: true})
	if !c.Declares("countOpen") || c.Declares("it") || c.Declares("nope") {
		t.Error("Declares answers for the module's own values only")
	}
}

func TestCheck_InUnknownModule(t *testing.T) {
	c := Check("../../examples/todos", &Module{In: "Nope", Source: "it = 1\n", Infer: true})
	var e *NoModuleError
	if !errors.As(c.Err, &e) || e.Name != "Nope" {
		t.Errorf("want NoModuleError, got %v", c.Err)
	}
}
