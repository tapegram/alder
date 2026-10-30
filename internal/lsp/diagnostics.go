package lsp

import (
	"fmt"
	"slices"

	"alder/internal/diag"
)

// publish sends a's diagnostics, one textDocument/publishDiagnostics per file: every document a
// was built from (with its version, even with nothing to report), every other file with
// diagnostics, and every file that had some at the previous publish and has none now (an empty
// list clears them). Analysis goroutine only.
func (s *server) publish(a *analysis) {
	byRel := map[string][]Diagnostic{}
	for _, d := range a.diags {
		if d.Span.File == "" {
			continue // nothing to attach it to
		}
		byRel[d.Span.File] = append(byRel[d.Span.File], s.enc.wireDiagnostic(a.lines[d.Span.File], lspDiagnostic(d)))
	}
	var rels []string
	for rel := range a.versions {
		rels = append(rels, rel)
	}
	for rel := range byRel {
		if _, open := a.versions[rel]; !open {
			rels = append(rels, rel)
		}
	}
	for rel := range s.reported {
		if _, open := a.versions[rel]; !open && byRel[rel] == nil {
			rels = append(rels, rel)
		}
	}
	slices.Sort(rels)
	for _, rel := range rels {
		p := publishDiagnosticsParams{URI: s.uriOf(rel), Diagnostics: byRel[rel]}
		if p.Diagnostics == nil {
			p.Diagnostics = []Diagnostic{}
		}
		if v, open := a.versions[rel]; open {
			p.Version = &v
		}
		if err := s.c.send(map[string]any{"jsonrpc": "2.0", "method": "textDocument/publishDiagnostics", "params": p}); err != nil {
			fmt.Fprintf(s.o.Log, "alder lsp: %v\n", err)
		}
	}
	s.reported = map[string]bool{}
	for rel := range byRel {
		s.reported[rel] = true
	}
}

// lspDiagnostic is d in LSP terms: severity 1 (error), 2 (warning) or 4 (hint, for a hole's
// W0002), positions shifted to 0-based, and the fix, when d has one, as data.fix.
func lspDiagnostic(d diag.Diagnostic) Diagnostic {
	sev := 1
	switch {
	case d.Code == "W0002":
		sev = 4
	case d.Severity == "warning":
		sev = 2
	}
	out := Diagnostic{
		Range:    lspRange(d.Span.Start.Line, d.Span.Start.Col, d.Span.End.Line, d.Span.End.Col),
		Severity: sev, Code: d.Code, Source: "alder", Message: d.Message,
	}
	if f := d.Fix; f != nil {
		out.Data = &diagnosticData{Fix: &TextEdit{Range: lspRange(f.Line, f.Col, f.EndLine, f.EndCol), NewText: f.Replace}}
	}
	return out
}

// lspRange is a compiler span (1-based line and byte column, exclusive end) as an LSP range.
func lspRange(line, col, endLine, endCol int) Range {
	pos := func(l, c int) Position { return Position{Line: max(l-1, 0), Character: max(c-1, 0)} }
	return Range{Start: pos(line, col), End: pos(endLine, endCol)}
}
