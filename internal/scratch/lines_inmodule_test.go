package scratch

import (
	"testing"

	"alder/internal/diag"
)

func TestMapDiag_InModuleDuplicateNamesTheFile(t *testing.T) {
	b := Build{ExtraFile: "app/X.alder", InsertLine: 1, LineOffset: 10, BaseLines: 10}
	sp := func(l int) diag.Span {
		return diag.Span{File: "app/X.alder", Start: diag.Pos{Line: l, Col: 5}, End: diag.Pos{Line: l, Col: 6}}
	}
	d, ok := b.MapDiag(diag.Diagnostic{Code: "E0009", Span: sp(12), Message: "`x` is already declared at line 4"})
	if !ok || d.Span.Start.Line != 2 || d.Message != "`x` is already declared in app/X.alder:4" {
		t.Fatalf("%v %+v", ok, d)
	}
	d, _ = b.MapDiag(diag.Diagnostic{Code: "E0009", Span: sp(14), Message: "`x` is already declared at line 12"})
	if d.Message != "`x` is already declared at line 2" {
		t.Fatalf("session line: %q", d.Message)
	}
}
