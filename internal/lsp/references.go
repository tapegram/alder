package lsp

import (
	"encoding/json"

	"alder/internal/diag"
	"alder/internal/syntax"
)

// referencesReq answers textDocument/references: every use of what the cursor names across
// the program's last good build, the declaration's name first when the client asks for it. A
// place in a file that has changed above it since that build is left out (Ruling R11).
func referencesReq(s *server, params json.RawMessage) (any, error) {
	var p struct {
		textDocumentPositionParams
		Context struct {
			IncludeDeclaration bool `json:"includeDeclaration"`
		} `json:"context"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &rpcError{codeInvalidParams, err.Error()}
	}
	locs := []Location{}
	rel, ok := s.rel(p.TextDocument.URI)
	if !ok {
		return locs, nil
	}
	a, m, path, pos, ok := s.at(rel, p.Position, true)
	if !ok {
		return locs, nil
	}
	ref, local, ok := target(a, m, path, pos)
	if !ok || local == nil && ref.Kind == syntax.RefNone {
		return locs, nil
	}
	var spans []diag.Span
	if p.Context.IncludeDeclaration {
		if sp, ok := declSpan(a, ref, local); ok {
			spans = append(spans, sp)
		}
	}
	spans = append(spans, uses(a, ref, local)...)
	for _, sp := range spans {
		if loc, ok := s.location(a, sp.File, sp); ok {
			locs = append(locs, loc)
		}
	}
	return locs, nil
}

// declSpan is the name span of the declaration ref (or the local binder) names, in its file.
func declSpan(a *analysis, ref syntax.Ref, local *binder) (diag.Span, bool) {
	if local != nil {
		return local.Span, local.Span.File != ""
	}
	if a.prog.Modules[ref.Module] == nil {
		return diag.Span{}, false // a stdlib declaration has no file (Ruling R12)
	}
	mod, d := findDecl(a, ref)
	if d == nil {
		return diag.Span{}, false
	}
	sp := nameSpan(d, mod)
	sp.File = mod.File
	return sp, true
}

// namespace groups the kinds of Ref that name the same declaration. A use can carry a
// different kind from its declaration's: a capitalised value used as an element is
// RefComponent where the declaration gives RefValue, and a table used as a value is RefTable
// where the declaration gives RefType.
func namespace(k syntax.RefKind) int {
	switch k {
	case syntax.RefType, syntax.RefTable:
		return 1
	case syntax.RefCtor:
		return 2
	case syntax.RefValue, syntax.RefStd, syntax.RefPage, syntax.RefComponent, syntax.RefCapability:
		return 3
	}
	return 0
}

// uses is the name span of every use of ref in the program, in module order then source order.
// For a local (local non-nil) it is the uses of that binder in its own module.
func uses(a *analysis, ref syntax.Ref, local *binder) []diag.Span {
	var out []diag.Span
	for _, name := range a.prog.Order {
		m := a.prog.Modules[name]
		if m == nil || local != nil && m.File != local.Span.File {
			continue
		}
		var walk func(path []syntax.Spanned)
		walk = func(path []syntax.Spanned) {
			n := path[len(path)-1]
			if sp, ok := useSpan(m, n, path, ref, local); ok {
				sp.File = m.File
				out = append(out, sp)
			}
			for _, c := range syntax.Children(n) {
				walk(append(path[:len(path):len(path)], c))
			}
		}
		walk([]syntax.Spanned{m})
	}
	return out
}

// useSpan is the span of the name n uses when n is a use of ref (or of the local binder).
func useSpan(m *syntax.Module, n syntax.Spanned, path []syntax.Spanned, ref syntax.Ref, local *binder) (diag.Span, bool) {
	var r syntax.Ref
	var qual, name string
	switch n := n.(type) {
	case *syntax.Var:
		r, qual, name = n.Ref, n.Module, n.Name
	case *syntax.CtorRef:
		r, qual, name = n.Ref, n.Module, n.Name
	case *syntax.TypeName:
		r, qual, name = n.Ref, n.Module, n.Name
	case *syntax.PCtor:
		r, name = n.Ref, n.Name
	case *syntax.Element:
		r, qual, name = n.Ref, n.Module, n.Tag
	default:
		return diag.Span{}, false
	}
	if local != nil {
		if _, isVar := n.(*syntax.Var); !isVar || r.Kind != syntax.RefLocal || name != local.Name {
			return diag.Span{}, false
		}
		if b := innermost(scopeAt(path, n.GetSpan().Start), name); b == nil || b.Key != local.Key {
			return diag.Span{}, false
		}
		return wordSpan(m, n.GetSpan(), 0, name), true
	}
	ns := namespace(ref.Kind)
	if ns == 0 || namespace(r.Kind) != ns || r.Module != ref.Module || r.Name != ref.Name {
		return diag.Span{}, false
	}
	skip := 0
	if qual != "" {
		skip = len(qual) + 1
	}
	if _, isEl := n.(*syntax.Element); isEl {
		skip++ // past the `<`
	}
	return wordSpan(m, n.GetSpan(), skip, name), true
}
