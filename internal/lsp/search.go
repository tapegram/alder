package lsp

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"alder/internal/search"
	"alder/internal/syntax"
	"alder/internal/types"
)

type searchParams struct {
	Query    string    `json:"query"`
	URI      string    `json:"uri,omitempty"`
	Position *Position `json:"position,omitempty"`
	Limit    int       `json:"limit,omitempty"` // 0 is the engine's default, negative no limit
}

// searchResult is alder/search's answer: `alder search --json`'s object.
type searchResult struct {
	Query   string       `json:"query"`
	Mode    string       `json:"mode"`
	Note    string       `json:"note"`
	Results []searchItem `json:"results"`
}

// searchItem has searchRowJSON's fields (`alder search --json`) with the declaration's place as a
// file URI and an LSP range in place of file, line and col; stdlib entries have neither.
type searchItem struct {
	Name      string `json:"name"`
	Module    string `json:"module"`
	Kind      string `json:"kind"`
	Signature string `json:"signature"`
	Doc       string `json:"doc"`
	Source    string `json:"source"`
	Effect    string `json:"effect,omitempty"`
	Match     string `json:"match,omitempty"`
	Args      *int   `json:"args,omitempty"` // set in produces mode, including 0
	URI       string `json:"uri,omitempty"`
	Range     *Range `json:"range,omitempty"`
}

// searchReq answers alder/search (params searchParams): the standard library's and the app's
// declarations found by name or by type, from the last build without errors (searchSnapshot).
// With no query and a position on a typed hole, the query is the hole's (holeQuery), and the
// answer's query is the text searched. A query never fails: malformed params are the only error,
// and a panic gives the empty result.
func searchReq(s *server, params json.RawMessage) (out any, err error) {
	var p searchParams
	if len(params) > 0 {
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, &rpcError{codeInvalidParams, err.Error()}
		}
	}
	q := strings.TrimSpace(p.Query)
	empty := searchResult{Query: q, Mode: "name", Results: []searchItem{}}
	defer func() {
		if r := recover(); r != nil {
			out, err = empty, nil
		}
	}()
	if q == "" && p.URI != "" && p.Position != nil {
		if rel, ok := s.rel(p.URI); ok {
			if hq, ok := s.holeQuery(rel, *p.Position); ok {
				q = hq
				empty.Query = q
			}
		}
	}
	a := s.searchSnapshot()
	if a == nil {
		return empty, nil
	}
	ix := a.searchIndex()
	if ix == nil {
		return empty, nil
	}
	resp := ix.Search(q, search.Options{Limit: p.Limit})
	res := searchResult{Query: resp.Query, Mode: resp.Mode, Note: resp.Note, Results: make([]searchItem, 0, len(resp.Results))}
	for _, h := range resp.Results {
		e := h.Entry
		it := searchItem{Name: e.Name, Module: e.Module, Kind: string(e.Kind), Signature: e.Signature, Doc: e.Doc,
			Source: e.Source, Effect: e.Effect, Match: h.Match}
		if resp.Mode == "produces" {
			n := h.Args
			it.Args = &n
		}
		if e.File != "" {
			r := s.enc.wireRange(a.lines[e.File], lspRange(e.Line, e.Col, e.EndLine, e.EndCol))
			it.URI, it.Range = s.uriOf(e.File), &r
		}
		res.Results = append(res.Results, it)
	}
	return res, nil
}

// searchSnapshot is the analysis alder/search reads: the last build without errors, else (before
// there is one) the last good build, whose index is then the standard library's alone
// (searchIndex); nil before the first build.
func (s *server) searchSnapshot() *analysis {
	if a := s.lastClean(); a != nil {
		return a
	}
	return s.lastGood()
}

// holeQuery is the query text for the hole under a position: ": " + the hole's type, as
// types.String prints it (whole module paths, so the text resolves from any module; variables
// the checker left unresolved print as `_`). ok is false off a typed hole of the last good
// build, or when the hole's line has changed since.
func (s *server) holeQuery(rel string, p Position) (query string, ok bool) {
	cx := s.completionAt(rel, p)
	if cx == nil || cx.m == nil {
		return "", false
	}
	h := cx.holeAt()
	if h == nil {
		return "", false
	}
	t := cx.a.res.Types[h]
	if t == nil {
		return "", false
	}
	return ": " + types.String(cx.a.res.U.Apply(t)), true
}

type searchEditParams struct {
	URI      string   `json:"uri"`
	Position Position `json:"position"`
	Module   string   `json:"module"`
	Name     string   `json:"name"`
	Kind     string   `json:"kind"`
	Args     int      `json:"args,omitempty"`
}

// searchEditReq answers alder/searchEdit (params searchEditParams): the edit that writes the
// picked result (an entry of alder/search's index) at a position of the document. On a typed hole
// of the last good build it replaces the hole, with Args new holes after the name (the whole in
// parentheses where the hole is a function or an argument of an application); elsewhere it
// inserts the name at the position. The name is written as the file writes it: bare for the
// Prelude and the file's own module, else qualified, with an import line added for an app module
// the file does not import. An item not in the index, or a position not in the document, is
// codeInvalidParams.
func searchEditReq(s *server, params json.RawMessage) (any, error) {
	var p searchEditParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &rpcError{codeInvalidParams, err.Error()}
	}
	rel, ok := s.rel(p.URI)
	if !ok {
		return nil, &rpcError{codeInvalidParams, "not a document of the app: " + p.URI}
	}
	if !s.searchHas(p.Module, p.Name, p.Kind) {
		return nil, &rpcError{codeInvalidParams, fmt.Sprintf("no %s %s.%s in the search index", p.Kind, p.Module, p.Name)}
	}
	var version *int
	if d := s.docs.get(rel); d != nil {
		v := d.Version
		version = &v
	}
	cx := s.completionAt(rel, p.Position)
	if cx == nil {
		return nil, &rpcError{codeInvalidParams, fmt.Sprintf("position %d:%d is not in %s", p.Position.Line, p.Position.Character, p.URI)}
	}

	qual, imp := p.Module, ""
	switch m := cx.m; {
	case p.Module == "Prelude" || m != nil && p.Module == m.Name:
		qual = ""
	case m == nil:
	case cx.a.prog.Std[p.Module] != nil || imports(m, p.Module):
		qual = cx.a.prog.QualifierFor(m.Name, p.Module)
	default:
		qual, imp = cx.a.prog.QualifierFor(m.Name, p.Module), importClause(cx.a.prog, m, p.Module)
	}
	text := p.Name
	if qual != "" {
		text = qual + "." + p.Name
	}

	// where: the hole under the position, else the position itself
	at := Range{Start: Position{cx.line, cx.col}, End: Position{cx.line, cx.col}}
	if h := cx.holeAt(); h != nil && h.Span.Start.Line == h.Span.End.Line {
		at = Range{Start: Position{h.Span.Start.Line - 1, h.Span.Start.Col - 1}, End: Position{h.Span.End.Line - 1, h.Span.End.Col - 1}}
		if p.Args > 0 {
			text += " " + strings.Join(newHoles(cx.lines, p.Args), " ")
			if inApp(cx.m, h) {
				text = "(" + text + ")"
			}
		}
	}
	edits := []TextEdit{s.enc.wireEdit(cx.lines, TextEdit{Range: at, NewText: text})}
	if imp != "" {
		line, newText := importEdit(cx.lines, p.Module, imp)
		pos := Position{line, 0}
		edits = append([]TextEdit{{Range: Range{Start: pos, End: pos}, NewText: newText}}, edits...)
	}
	return &workspaceEdit{DocumentChanges: []textDocumentEdit{{
		TextDocument: versionedDocument{URI: s.uriOf(rel), Version: version}, Edits: edits}}}, nil
}

// searchHas reports whether alder/search's index (searchSnapshot) has the entry.
func (s *server) searchHas(module, name, kind string) bool {
	a := s.searchSnapshot()
	if a == nil {
		return false
	}
	ix := a.searchIndex()
	if ix == nil {
		return false
	}
	for _, e := range ix.Entries {
		if e.Module == module && e.Name == name && string(e.Kind) == kind {
			return true
		}
	}
	return false
}

// imports reports whether m imports path.
func imports(m *syntax.Module, path string) bool {
	for _, im := range m.Imports {
		if im.Module == path {
			return true
		}
	}
	return false
}

// newHoles is n hole names, ?a, ?b, … (then ?a1, ?b1, …), that no hole in lines uses.
func newHoles(lines []string, n int) []string {
	used := map[string]bool{}
	for _, l := range lines {
		for _, g := range reHoleName.FindAllStringSubmatch(l, -1) {
			used[g[1]] = true
		}
	}
	var out []string
	for i := 0; len(out) < n; i++ {
		name := string(rune('a' + i%26))
		if i >= 26 {
			name += strconv.Itoa(i / 26)
		}
		if !used[name] {
			out = append(out, "?"+name)
		}
	}
	return out
}

var reHoleName = regexp.MustCompile(`\?([a-z][A-Za-z0-9_]*)`)

// inApp reports whether hole h is the function or an argument of an application in m.
func inApp(m *syntax.Module, h *syntax.HoleExpr) bool {
	path := syntax.NodeAt(m, h.Span.Start)
	if len(path) < 2 || path[len(path)-1] != syntax.Spanned(h) {
		return false
	}
	app, ok := path[len(path)-2].(*syntax.App)
	if !ok {
		return false
	}
	return app.Fn == syntax.Expr(h) || slices.Contains(app.Args, syntax.Expr(h))
}

// importEdit is where `import <clause>` goes in lines, the current text of a module, and the text
// inserted there: among the imports in sorted order, as the formatter writes them (after the last
// import, for a path that sorts last); with no imports, after the `module` line and a blank line.
// It reads the text itself, which may have changed since the last build.
func importEdit(lines []string, path, clause string) (line int, text string) {
	module, first, after := -1, -1, -1
scan:
	for i, l := range lines {
		t := strings.TrimSpace(l)
		switch {
		case module < 0:
			if reModuleLine.MatchString(l) {
				module = i
			}
		case reImportLine.MatchString(l):
			if first < 0 {
				first = i
			}
			if reImportLine.FindStringSubmatch(l)[1] < path {
				after = i + 1
			}
		case t == "" || strings.HasPrefix(t, "--"):
		default:
			break scan
		}
	}
	switch {
	case after >= 0:
		return after, "import " + clause + "\n"
	case first >= 0:
		return first, "import " + clause + "\n"
	}
	return module + 1, "\nimport " + clause + "\n"
}

var (
	reModuleLine = regexp.MustCompile(`^module\s`)
	reImportLine = regexp.MustCompile(`^import\s+([A-Za-z0-9_.]+)`)
)
