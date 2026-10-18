package lsp

import (
	"encoding/json"

	"alder/internal/diag"
	"alder/internal/syntax"
	"alder/internal/types"
)

// definitionReq answers textDocument/definition: where what the cursor names is declared, as
// []Location ([] for nothing, for a stdlib name (Ruling R12), or when the last good build no
// longer holds there).
func definitionReq(s *server, params json.RawMessage) (any, error) {
	var p textDocumentPositionParams
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
	if file, sp, ok := definition(a, m, path, pos); ok {
		locs = append(locs, Location{URI: s.uriOf(file), Range: spanRange(sp)})
	}
	return locs, nil
}

// definition is the file and span of the declaration of what the cursor names: a field of a
// named record for an access, a local's binder, a module's start, an app declaration's name.
func definition(a *analysis, m *syntax.Module, path []syntax.Spanned, pos diag.Pos) (file string, sp diag.Span, ok bool) {
	if acc, isAcc := path[len(path)-1].(*syntax.Access); isAcc {
		return fieldDefinition(a, acc)
	}
	ref, local, ok := target(a, m, path, pos)
	switch {
	case !ok:
		return "", diag.Span{}, false
	case local != nil:
		return m.File, local.Span, true
	case ref.Kind == syntax.RefStd:
		return "", diag.Span{}, false
	case ref.Kind == syntax.RefNone:
		if mod := a.prog.Modules[ref.Module]; mod != nil {
			return mod.File, diag.Span{Start: diag.Pos{Line: 1, Col: 1}, End: diag.Pos{Line: 1, Col: 1}}, true
		}
		return "", diag.Span{}, false
	}
	if a.prog.Modules[ref.Module] == nil {
		return "", diag.Span{}, false // a stdlib declaration: no file to go to (Ruling R12)
	}
	mod, d := findDecl(a, ref)
	if d == nil {
		return "", diag.Span{}, false
	}
	return mod.File, nameSpan(d, mod), true
}

// fieldDefinition is the field an access names, when its base is a named record declared in
// the app (a record type, a table or a form).
func fieldDefinition(a *analysis, acc *syntax.Access) (string, diag.Span, bool) {
	r, ok := a.res.Types[acc.Expr].(*types.Record)
	if !ok || r.Anonymous {
		return "", diag.Span{}, false
	}
	if r.Def != nil {
		r = r.Def
	}
	mod := a.prog.Modules[r.Module]
	if mod == nil {
		return "", diag.Span{}, false
	}
	for _, d := range mod.Decls {
		var fields []*syntax.Field
		switch d := d.(type) {
		case *syntax.TypeDecl:
			if d.Name == r.Name && d.Record != nil {
				fields = d.Record.Fields
			}
		case *syntax.TableDecl:
			if d.Name == r.Name {
				fields = d.Fields
			}
		case *syntax.FormDecl:
			if d.Name == r.Name {
				fields = d.Fields
			}
		}
		for _, f := range fields {
			if f.Name == acc.Field {
				return mod.File, nameSpan(f, mod), true
			}
		}
	}
	return "", diag.Span{}, false
}

// spanRange is a compiler span as an LSP range.
func spanRange(sp diag.Span) Range {
	return lspRange(sp.Start.Line, sp.Start.Col, sp.End.Line, sp.End.Col)
}
