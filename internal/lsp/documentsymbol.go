package lsp

import (
	"encoding/json"
	"strings"

	"alder/internal/diag"
	"alder/internal/syntax"
)

// DocumentSymbol is an LSP document symbol: a declaration's name, type, kind, whole range, the
// range of its name, and its members.
type DocumentSymbol struct {
	Name           string           `json:"name"`
	Detail         string           `json:"detail,omitempty"`
	Kind           int              `json:"kind"`
	Range          Range            `json:"range"`
	SelectionRange Range            `json:"selectionRange"`
	Children       []DocumentSymbol `json:"children,omitempty"`
}

// The LSP SymbolKinds documentSymbolReq uses beside symbolKinds' function 12, value 13, type 23,
// constructor 22 and job 24.
const (
	symClass     = 5  // a fake
	symMethod    = 6  // a route or page, and a capability's entries
	symField     = 8  // the fields of a record type, table or form
	symInterface = 11 // a capability
	symObject    = 19 // session, deploy, theme, login, choose, silo
)

// documentSymbolReq answers textDocument/documentSymbol with the outline of the current text,
// parsed as foldingRangeReq does (so unsaved text and type errors are fine): every top-level
// declaration in source order, as hierarchical symbols. Kinds: functions (a declaration with
// parameters or a function signature) 12, values 13, types 23 with constructors 22 (a record's
// fields 8), jobs 24 with their `Name.run` arm 12, routes and pages 6 with a route's
// `Route.member` declarations 12, messages 22, tables and forms 23 (fields 8), capabilities 11
// (entries 6), fakes 5 (entries 12), tests 12, session, deploy, theme, login, choose and silo 19.
// Detail is the written type signature (a route's is its method and path). An arm or member
// whose job or route is not in the file is a top-level function; a parent's range grows to cover
// its arm or members. Text that parses to no declaration falls back to the last good build's
// outline of the file, else []. Never fails.
func documentSymbolReq(s *server, params json.RawMessage) (out any, err error) {
	empty := []DocumentSymbol{}
	defer func() {
		if r := recover(); r != nil {
			out, err = empty, nil
		}
	}()
	var p textDocumentParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &rpcError{codeInvalidParams, err.Error()}
	}
	rel, ok := s.alderDoc(p.TextDocument.URI)
	if !ok {
		return empty, nil
	}
	text := s.text(rel)
	d := parseSyntaxDoc(rel, text)
	if d.m == nil {
		return empty, nil
	}
	if len(d.m.Decls) == 0 && d.scratch == nil && strings.TrimSpace(text) != "" {
		// nothing parsed: the last good build's module of the file, in that build's text
		a := s.lastGood()
		if a == nil || a.prog == nil {
			return empty, nil
		}
		for _, m := range a.prog.Modules {
			if m.File == rel {
				return s.outline(syntaxDoc{m: m, lines: a.lines[rel]}), nil
			}
		}
		return empty, nil
	}
	return s.outline(d), nil
}

// outline is the symbols of d's module.
func (s *server) outline(d syntaxDoc) []DocumentSymbol {
	b := symbolBuilder{s: s, d: d}
	owners := map[string]bool{} // the jobs and routes of the file
	for _, decl := range d.m.Decls {
		switch decl := decl.(type) {
		case *syntax.JobDecl:
			owners[decl.Name] = true
		case *syntax.RouteDecl:
			owners[decl.Name] = true
		}
	}
	out := []DocumentSymbol{}
	at := map[string]int{} // the index in out of a job or route, by name
	var members []*syntax.ValueDecl
	for _, decl := range d.m.Decls {
		if vd, ok := decl.(*syntax.ValueDecl); ok && owners[vd.Route] {
			members = append(members, vd)
			continue
		}
		sym, ok := b.decl(decl)
		if !ok {
			continue
		}
		switch decl := decl.(type) {
		case *syntax.JobDecl:
			at[decl.Name] = len(out)
		case *syntax.RouteDecl:
			at[decl.Name] = len(out)
		}
		out = append(out, sym)
	}
	for _, vd := range members {
		sym, _ := b.decl(vd)
		if i, ok := at[vd.Route]; ok {
			out[i].Children = append(out[i].Children, sym)
			out[i].Range = b.cover(out[i].Range, sym.Range)
		}
	}
	return out
}

// symbolBuilder makes symbols of one document's declarations.
type symbolBuilder struct {
	s *server
	d syntaxDoc
}

func before(a, b Position) bool {
	return a.Line < b.Line || a.Line == b.Line && a.Character < b.Character
}

// cover is the smallest range holding a and b.
func (b symbolBuilder) cover(a, c Range) Range {
	if before(c.Start, a.Start) {
		a.Start = c.Start
	}
	if before(a.End, c.End) {
		a.End = c.End
	}
	return a
}

// rng is a module span as a range of the document in the client's encoding.
func (b symbolBuilder) rng(sp diag.Span) Range {
	st, en := b.d.toFile(sp.Start), b.d.toFile(sp.End)
	return b.s.enc.wireRange(b.d.lines, lspRange(st.Line, st.Col, en.Line, en.Col))
}

// nameIn is the range of the first occurrence of name in the first line of sp, else the empty
// range at sp's start.
func (b symbolBuilder) nameIn(sp diag.Span, name string) Range {
	at := diag.Span{Start: sp.Start, End: sp.Start}
	if l := sp.Start.Line; l >= 1 && l <= len(b.d.m.Lines) && sp.Start.Col >= 1 {
		line := b.d.m.Lines[l-1]
		if lo := min(sp.Start.Col-1, len(line)); strings.HasPrefix(line[lo:], name) {
			at.End.Col = sp.Start.Col + len(name)
		} else if i := strings.Index(line[lo:], name); i >= 0 {
			at = diag.Span{Start: diag.Pos{Line: l, Col: sp.Start.Col + i}, End: diag.Pos{Line: l, Col: sp.Start.Col + i + len(name)}}
		}
	}
	return b.rng(at)
}

// text is the source of sp collapsed to one line, "" for a span outside the module.
func (b symbolBuilder) text(sp diag.Span) string {
	return strings.Join(strings.Fields(spanText(b.d.m.Lines, sp)), " ")
}

func (b symbolBuilder) sym(name string, kind int, sp, sel diag.Span, detail string) DocumentSymbol {
	return DocumentSymbol{Name: name, Detail: detail, Kind: kind, Range: b.rng(sp), SelectionRange: b.rng(sel)}
}

// named is a symbol whose name is written at the start of its declaration, or after the keyword
// word.
func (b symbolBuilder) named(name string, kind int, sp diag.Span, detail string) DocumentSymbol {
	sym := b.sym(name, kind, sp, sp, detail)
	sym.SelectionRange = b.nameIn(sp, name)
	return sym
}

func (b symbolBuilder) fields(fs []*syntax.Field) []DocumentSymbol {
	var out []DocumentSymbol
	for _, f := range fs {
		out = append(out, b.named(f.Name, symField, f.Span, b.text(f.Type.GetSpan())))
	}
	return out
}

// decl is the symbol of one top-level declaration; ok is false for none.
func (b symbolBuilder) decl(decl syntax.Decl) (sym DocumentSymbol, ok bool) {
	if decl == nil {
		return sym, false
	}
	switch d := decl.(type) {
	case *syntax.ValueDecl:
		kind := 13
		if _, fn := d.Sig.(*syntax.FunType); fn || len(d.Params) > 0 || d.Route != "" {
			kind = 12
		}
		detail := ""
		if d.Sig != nil {
			detail = b.text(d.Sig.GetSpan())
		}
		return b.named(d.FullName(), kind, d.Span, detail), true
	case *syntax.TypeDecl:
		sym = b.named(d.Name, 23, d.Span, "")
		for _, c := range d.Union {
			sym.Children = append(sym.Children, b.named(c.Name, 22, c.Span, ""))
		}
		if d.Record != nil {
			sym.Children = b.fields(d.Record.Fields)
		}
		return sym, true
	case *syntax.JobDecl:
		return b.sym(d.Name, 24, d.Span, d.NameSpan, ""), true
	case *syntax.RouteDecl:
		return b.sym(d.Name, symMethod, d.Span, d.NameSpan, d.Method+" "+d.Path), true
	case *syntax.PageDecl:
		return b.named("page "+d.Path, symMethod, d.Span, ""), true
	case *syntax.MsgDecl:
		return b.named(d.Name, 22, d.Span, ""), true
	case *syntax.FormDecl:
		sym = b.named(d.Name, 23, d.Span, "")
		sym.Children = b.fields(d.Fields)
		return sym, true
	case *syntax.TableDecl:
		sym = b.named(d.Name, 23, d.Span, "")
		sym.Children = b.fields(d.Fields)
		return sym, true
	case *syntax.CapDecl:
		sym = b.named(d.Name, symInterface, d.Span, "")
		for _, f := range d.Entries {
			sym.Children = append(sym.Children, b.named(f.Name, symMethod, f.Span, b.text(f.Type.GetSpan())))
		}
		return sym, true
	case *syntax.FakeDecl:
		sym = b.sym("fake "+d.Name, symClass, d.Span, d.NameSpan, "")
		for _, t := range d.Tables {
			if c, ok := b.decl(t); ok {
				sym.Children = append(sym.Children, c)
			}
		}
		for _, e := range d.Entries {
			if c, ok := b.decl(e); ok {
				sym.Children = append(sym.Children, c)
			}
		}
		return sym, true
	case *syntax.TestDecl:
		return b.sym("test "+d.Name, 12, d.Span, d.Span, ""), true
	case *syntax.SessionDecl:
		return b.named("session", symObject, d.Span, ""), true
	case *syntax.DeployDecl:
		return b.named("deploy", symObject, d.Span, ""), true
	case *syntax.ThemeDecl:
		return b.named("theme", symObject, d.Span, ""), true
	case *syntax.LoginDecl:
		return b.named("login", symObject, d.Span, ""), true
	case *syntax.ChooseDecl:
		return b.sym("choose "+d.Key, symObject, d.Span, d.KeySpan, ""), true
	case *syntax.SiloDecl:
		return b.sym("silo "+d.Key, symObject, d.Span, d.KeySpan, ""), true
	}
	return sym, false
}
