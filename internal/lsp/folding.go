package lsp

import (
	"encoding/json"
	"sort"
	"strings"

	"alder/internal/diag"
	"alder/internal/scratch"
	"alder/internal/semtok"
	"alder/internal/syntax"
)

// syntaxDoc is a document's current text parsed for the syntax requests (folding, selection): an
// app module as is; a scratch file as its module source behind a `module Scratch` line, as the run
// parses it (semtok.ParseScratch), with the map back to the file's positions.
type syntaxDoc struct {
	m       *syntax.Module
	lines   []string           // the document's lines
	scratch *semtok.ScratchDoc // a scratch file's parse; nil for a module
}

func parseSyntaxDoc(rel, text string) syntaxDoc {
	lines := strings.Split(text, "\n")
	if !scratch.IsScratchFile(rel) {
		m, _ := syntax.ParseFile(rel, []byte(text))
		return syntaxDoc{m: m, lines: lines}
	}
	sd := semtok.ParseScratch(rel, text)
	return syntaxDoc{m: sd.Module, lines: lines, scratch: &sd}
}

// watches are a scratch file's watches (none for a module).
func (d syntaxDoc) watches() []scratch.Watch {
	if d.scratch == nil {
		return nil
	}
	return d.scratch.File.Watches
}

// toFile is p, a position in d.m, in the document (both 1-based); toModule is its inverse.
func (d syntaxDoc) toFile(p diag.Pos) diag.Pos {
	if d.scratch == nil {
		return p
	}
	return d.scratch.ToFile(p)
}

func (d syntaxDoc) toModule(p diag.Pos) diag.Pos {
	if d.scratch == nil {
		return p
	}
	return d.scratch.ToModule(p)
}

// FoldingRange is an LSP folding range: 0-based first and last lines, and a kind for comment and
// import runs.
type FoldingRange struct {
	StartLine int    `json:"startLine"`
	EndLine   int    `json:"endLine"`
	Kind      string `json:"kind,omitempty"`
}

// foldingRangeReq answers textDocument/foldingRange from the current text: one range per
// multi-line node of the kinds folds.scm lists, each scratch watch, each run of two or more
// `--` comment lines (kind comment) and the run of imports (kind imports), outermost first in
// source order.
func foldingRangeReq(s *server, params json.RawMessage) (any, error) {
	var p textDocumentParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &rpcError{codeInvalidParams, err.Error()}
	}
	out := []FoldingRange{}
	rel, ok := s.alderDoc(p.TextDocument.URI)
	if !ok {
		return out, nil
	}
	d := parseSyntaxDoc(rel, s.text(rel))
	if d.m == nil {
		return out, nil
	}
	seen := map[[2]int]bool{}
	add := func(start, end int, kind string) { // 1-based document lines
		if end > start && !seen[[2]int{start, end}] {
			seen[[2]int{start, end}] = true
			out = append(out, FoldingRange{StartLine: start - 1, EndLine: end - 1, Kind: kind})
		}
	}
	span := func(sp diag.Span) {
		end := d.toFile(sp.End).Line
		if sp.End.Col == 1 { // nothing of the node is on its end line
			end--
		}
		add(d.toFile(sp.Start).Line, end, "")
	}
	var walk func(n syntax.Spanned)
	walk = func(n syntax.Spanned) {
		switch n.(type) {
		case *syntax.ValueDecl, *syntax.TypeDecl, *syntax.TableDecl, *syntax.CapDecl, *syntax.FakeDecl,
			*syntax.SessionDecl, *syntax.DeployDecl, *syntax.ThemeDecl, *syntax.TestDecl, *syntax.FormDecl,
			*syntax.Block, *syntax.Match, *syntax.Element, *syntax.RecordLit, *syntax.ListLit:
			span(n.GetSpan())
		}
		for _, c := range syntax.Children(n) {
			walk(c)
		}
	}
	walk(d.m)
	for _, w := range d.watches() {
		add(w.Line, w.End, "")
	}
	if n := len(d.m.Imports); n > 0 {
		add(d.toFile(d.m.Imports[0].Span.Start).Line, d.toFile(d.m.Imports[n-1].Span.End).Line, "imports")
	}
	runStart, runEnd := 0, 0
	for _, c := range d.m.Comments {
		l := d.toFile(diag.Pos{Line: c.Line, Col: c.Col}).Line
		if l < 1 || l > len(d.lines) || !strings.HasPrefix(strings.TrimSpace(d.lines[l-1]), "--") {
			continue // a comment after code on its line
		}
		if runEnd > 0 && l == runEnd+1 {
			runEnd = l
			continue
		}
		add(runStart, runEnd, "comment")
		runStart, runEnd = l, l
	}
	add(runStart, runEnd, "comment")
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].StartLine < out[j].StartLine || out[i].StartLine == out[j].StartLine && out[i].EndLine > out[j].EndLine
	})
	return out, nil
}
