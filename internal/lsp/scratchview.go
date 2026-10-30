package lsp

import (
	"encoding/json"
	"slices"
	"strings"
	"sync"
	"unicode/utf8"

	"alder/internal/diag"
	"alder/internal/scratch"
	"alder/internal/syntax"
)

// A scratch document is not one of the app's modules, so the app's analysis knows nothing of it.
// Its editor features (hover, definition, completion, code actions) read a view instead: the
// scratch module checked with the app (scratch.Check, as a run checks it, against the app on
// disk), with the map between the file's positions and the module's. The module differs from the
// file in two ways: Check inserts lines (a `module` line and the implicit imports, or, after an
// `in` header, the module's own text above the file's), and each watch's `> ` is `itK = ` in the
// module, shifting its first line's columns (scratch.File.Map.Shift).

// scratchView is the analysis of one text of a scratch document.
type scratchView struct {
	text     string
	app      *analysis         // the app's last good analysis when it was built: a newer one rebuilds it
	a        *analysis         // the checked program; nil when the text did not check (a parse error)
	file     string            // the scratch module's file in a.prog (Checked.ExtraFile)
	insert   int               // Checked.InsertLine: the first inserted line of the module
	offset   int               // Checked.LineOffset: how many lines were inserted there
	inserted []string          // those lines
	diags    []diag.Diagnostic // the file's own (ParseFile's, then the check's), positioned in it
}

// scratchViews are the views of the open scratch documents, by rel: the one of the current text
// and the last that checked. The fast worker builds them; didClose drops them.
type scratchViews struct {
	mu        sync.Mutex
	cur, good map[string]*scratchView
}

// scratchView is the view of rel's text (which the caller has read) and the last view of rel
// that checked (it, when the text checks); good is nil before any text of rel has checked. A
// view is rebuilt when the text or the app's last good analysis has changed.
func (s *server) scratchView(rel, text string) (cur, good *scratchView) {
	app := s.lastGood()
	v := &s.views
	v.mu.Lock()
	if c := v.cur[rel]; c != nil && c.text == text && c.app == app {
		defer v.mu.Unlock()
		return c, v.good[rel]
	}
	v.mu.Unlock()
	cur = buildScratchView(s.rootReal, rel, text)
	cur.app = app
	v.mu.Lock()
	defer v.mu.Unlock()
	if s.docs.get(rel) == nil { // not open (or closed meanwhile): nothing to keep
		if cur.a != nil {
			return cur, cur
		}
		return cur, nil
	}
	if v.cur == nil {
		v.cur, v.good = map[string]*scratchView{}, map[string]*scratchView{}
	}
	v.cur[rel] = cur
	if cur.a != nil {
		v.good[rel] = cur
	}
	return cur, v.good[rel]
}

// dropScratchView forgets rel's views: the document has closed.
func (s *server) dropScratchView(rel string) {
	s.views.mu.Lock()
	defer s.views.mu.Unlock()
	delete(s.views.cur, rel)
	delete(s.views.good, rel)
}

// buildScratchView checks text, the scratch file rel, with the app at root.
func buildScratchView(root, rel, text string) *scratchView {
	f, parsed := scratch.ParseFile(rel, text)
	name := f.ModuleName()
	if name == "" {
		name = "Scratch"
	}
	c := scratch.Check(root, &scratch.Module{Name: name, In: f.In, Source: f.Source, Infer: true})
	v := &scratchView{text: text, file: c.ExtraFile, insert: c.InsertLine, offset: c.LineOffset, diags: parsed}
	for _, d := range c.Diags {
		if c.InSession(d) {
			v.diags = append(v.diags, f.Positioned(d, c))
		}
	}
	prog := c.Program()
	if prog == nil || c.ExtraFile == "" {
		return v
	}
	a := &analysis{prog: prog, res: c.Result, lines: map[string][]string{}}
	for _, m := range prog.Modules {
		a.lines[m.File] = m.Lines
	}
	m := moduleOfFile(a, v.file)
	if m == nil {
		return v
	}
	if v.offset > 0 {
		lo := v.insert - 1
		if lo < 0 || lo+v.offset > len(m.Lines) {
			return v
		}
		v.inserted = slices.Clone(m.Lines[lo : lo+v.offset])
	}
	v.a = a
	return v
}

// moduleOfFile is a's module read from file, or nil.
func moduleOfFile(a *analysis, file string) *syntax.Module {
	for _, m := range a.prog.Modules {
		if m.File == file {
			return m
		}
	}
	return nil
}

// toModule is the module position of p, a position in the file whose current text parses to nf.
func (v *scratchView) toModule(nf scratch.File, p diag.Pos) diag.Pos {
	col := p.Col
	if p.Line >= 1 && p.Line <= len(nf.Map.Shift) {
		col -= nf.Map.Shift[p.Line-1]
	}
	line := p.Line
	if v.offset > 0 && line >= v.insert {
		line += v.offset
	}
	return diag.Pos{Line: line, Col: max(col, 1)}
}

// fromModule is the file position of p, a module position; false on a line Check inserted.
func (v *scratchView) fromModule(nf scratch.File, p diag.Pos) (diag.Pos, bool) {
	line := p.Line
	if v.offset > 0 && line >= v.insert {
		if line < v.insert+v.offset {
			return p, false
		}
		line -= v.offset
	}
	if line < 1 || line > len(nf.Map.Shift) {
		return p, false
	}
	return diag.Pos{Line: line, Col: max(1, p.Col+nf.Map.Shift[line-1])}, true
}

// moduleLines is the module's text as the file's current text (nf) makes it, to judge the
// view's freshness by, as an app module's current lines are judged (Ruling R11).
func (v *scratchView) moduleLines(nf scratch.File) []string {
	src := strings.Split(nf.Source, "\n")
	if v.offset == 0 {
		return src
	}
	at := min(max(v.insert-1, 0), len(src))
	return slices.Concat(src[:at], v.inserted, src[at:])
}

// scratchPlace is `at` for a scratch document: what a position names in its view.
type scratchPlace struct {
	v    *scratchView
	nf   scratch.File // the current text, parsed
	m    *syntax.Module
	path []syntax.Spanned
	pos  diag.Pos // in the module
}

// scratchAt is the place p names in the scratch document rel, through the last view that checked;
// false when there is none, or when the module's lines up to p's (with it when inclusive) differ
// from those the current text makes.
func (s *server) scratchAt(rel string, p Position, inclusive bool) (*scratchPlace, bool) {
	if p.Line < 0 || p.Character < 0 {
		return nil, false
	}
	text := s.text(rel)
	_, good := s.scratchView(rel, text)
	if good == nil {
		return nil, false
	}
	nf, _ := scratch.ParseFile(rel, text)
	pos := good.toModule(nf, diag.Pos{Line: p.Line + 1, Col: p.Character + 1})
	if !freshLines(good.a, good.file, good.moduleLines(nf), pos.Line, inclusive) {
		return nil, false
	}
	m := moduleOfFile(good.a, good.file)
	if m == nil {
		return nil, false
	}
	return &scratchPlace{v: good, nf: nf, m: m, path: syntax.NodeAt(m, pos), pos: pos}, true
}

// scratchDefinition is the location of the declaration of what p names in the scratch document
// rel: in the file itself (its own declarations, locals), or in the app (an `in` module's own
// text included).
func (s *server) scratchDefinition(rel string, p Position) (Location, bool) {
	pl, ok := s.scratchAt(rel, p, true)
	if !ok {
		return Location{}, false
	}
	file, sp, ok := definition(pl.v.a, pl.m, pl.path, pl.pos)
	if !ok {
		return Location{}, false
	}
	if file != pl.v.file {
		return s.location(pl.v.a, file, sp)
	}
	start, ok1 := pl.v.fromModule(pl.nf, sp.Start)
	end, ok2 := pl.v.fromModule(pl.nf, sp.End)
	switch {
	case ok1 && ok2:
		if !freshLines(pl.v.a, pl.v.file, pl.v.moduleLines(pl.nf), sp.End.Line, true) {
			return Location{}, false
		}
		return Location{URI: s.uriOf(rel), Range: lspRange(start.Line, start.Col, end.Line, end.Col)}, true
	case strings.HasPrefix(file, "app/"): // an `in` module's own text, above the file's
		return s.location(pl.v.a, file, sp)
	}
	return Location{}, false
}

// scratchCompletion is completion at p in the scratch document rel: the contexts of the current
// text, with the scopes and types of the last view that checked, and each edit in the file's
// columns. Without any such view it is the completion of a file the analysis does not know.
func (s *server) scratchCompletion(rel string, p Position) []completionItem {
	text := s.text(rel)
	_, good := s.scratchView(rel, text)
	nf, _ := scratch.ParseFile(rel, text)
	if p.Line < 0 || p.Line >= len(nf.Lines) || p.Character < 0 {
		return nil
	}
	if good == nil {
		if cx := s.completionAt(rel, p); cx != nil {
			return cx.items()
		}
		return nil
	}
	fl := nf.Lines[p.Line]
	col := min(p.Character, len(fl))
	for col > 0 && col < len(fl) && !utf8.RuneStart(fl[col]) {
		col--
	}
	lines := good.moduleLines(nf)
	mp := good.toModule(nf, diag.Pos{Line: p.Line + 1, Col: col + 1})
	if mp.Line < 1 || mp.Line > len(lines) {
		return nil
	}
	ml := lines[mp.Line-1]
	mcol := min(mp.Col-1, len(ml))
	cx := &completion{a: good.a, m: moduleOfFile(good.a, good.file), lines: lines, line: mp.Line - 1, col: mcol,
		prefix: ml[:mcol], pos: diag.Pos{Line: mp.Line, Col: mcol + 1}}
	if cx.m != nil {
		cx.fresh = freshLines(cx.a, good.file, lines, cx.pos.Line, false)
		cx.same = cx.fresh && freshLines(cx.a, good.file, lines, cx.pos.Line, true)
	}
	items := cx.items()
	shift := nf.Map.Shift[p.Line]
	for i := range items {
		r := &items[i].TextEdit.Range
		r.Start = Position{p.Line, max(r.Start.Character+shift, 0)}
		r.End = Position{p.Line, max(r.End.Character+shift, 0)}
	}
	return items
}

// scratchActions are the compiler's fixes for the diagnostics of the scratch document rel's
// current text on rg's lines. Generators are not offered: they write the app's modules.
func (s *server) scratchActions(rel string, rg Range) []codeAction {
	x := s.docs.get(rel)
	if x == nil {
		return nil
	}
	cur, _ := s.scratchView(rel, x.Text)
	v := x.Version
	doc := versionedDocument{URI: s.uriOf(rel), Version: &v}
	var fixes []codeAction
	for _, d := range cur.diags {
		if d.Fix == nil {
			continue
		}
		ld := lspDiagnostic(d)
		if !linesOverlap(ld.Range, rg) {
			continue
		}
		fixes = append(fixes, codeAction{
			Title: "Fix: " + d.Message, Kind: "quickfix", Diagnostics: []Diagnostic{ld},
			Edit: &workspaceEdit{DocumentChanges: []textDocumentEdit{{TextDocument: doc, Edits: []TextEdit{*ld.Data.Fix}}}},
		})
	}
	return disjointFixes(fixes)
}

// inlayHint is an LSP InlayHint with a plain label.
type inlayHint struct {
	Position    Position `json:"position"`
	Label       string   `json:"label"`
	PaddingLeft bool     `json:"paddingLeft,omitempty"`
}

// scratchShown is the last alder/scratch a scratch document was sent: its text then, and the
// watches' results.
type scratchShown struct {
	lines   []string
	watches []scratch.WatchResult
}

// inlayHintReq answers textDocument/inlayHint: in a scratch document, each watch's result from the
// last alder/scratch at the end of the watch's last line, for an editor without alder.nvim (which
// shows them as virtual text). A watch whose lines have changed since that run has none until
// the next. Other documents have none.
func inlayHintReq(s *server, params json.RawMessage) (any, error) {
	var p struct {
		TextDocument struct {
			URI string `json:"uri"`
		} `json:"textDocument"`
		Range Range `json:"range"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &rpcError{codeInvalidParams, err.Error()}
	}
	out := []inlayHint{}
	rel, ok := s.rel(p.TextDocument.URI)
	if !ok || !scratch.IsScratchFile(rel) {
		return out, nil
	}
	s.scratch.mu.Lock()
	sh, ok := s.scratch.shown[rel]
	s.scratch.mu.Unlock()
	if !ok {
		return out, nil
	}
	now := strings.Split(s.text(rel), "\n")
	for _, w := range sh.watches {
		label := hintLabel(w)
		if label == "" || w.Line < 1 || w.End < w.Line || w.End > len(now) || w.End > len(sh.lines) ||
			!slices.Equal(now[w.Line-1:w.End], sh.lines[w.Line-1:w.End]) {
			continue
		}
		line := w.End - 1
		if line < p.Range.Start.Line || line > p.Range.End.Line {
			continue
		}
		out = append(out, inlayHint{Position: Position{line, len(now[line])}, Label: label, PaddingLeft: true})
	}
	return out, nil
}

// maxHint caps an inlay hint's label, in runes.
const maxHint = 120

// hintLabel is a watch's result as one line: `⇒ value : Type` (a multi-line or cut value shows
// its first line and `…`), `: Type` before the values, `✗ error` for a failure; "" for nothing.
func hintLabel(w scratch.WatchResult) string {
	first := func(s string) (string, bool) {
		l, _, more := strings.Cut(s, "\n")
		return l, more
	}
	var label string
	switch {
	case w.Error != "":
		l, _ := first(w.Error)
		label = "✗ " + l
	case w.Value != "":
		l, more := first(w.Value)
		if more || w.Truncated {
			l += " …"
		}
		label = "⇒ " + l
		if w.Type != "" {
			label += " : " + w.Type
		}
	case w.Type != "":
		label = ": " + w.Type
	}
	if r := []rune(label); len(r) > maxHint {
		label = string(r[:maxHint-1]) + "…"
	}
	return label
}
