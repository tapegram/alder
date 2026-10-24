package lsp

import (
	"encoding/json"
	"fmt"
	"strings"

	"alder/internal/check"
	"alder/internal/diag"
	"alder/internal/markup"
	"alder/internal/syntax"
	"alder/internal/types"
)

// hoverReq answers textDocument/hover: markdown with the signature (or type) of what the cursor
// names and its doc comment, or null when there is nothing to show or the last good build no
// longer holds there.
func hoverReq(s *server, params json.RawMessage) (any, error) {
	var p textDocumentPositionParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &rpcError{codeInvalidParams, err.Error()}
	}
	rel, ok := s.rel(p.TextDocument.URI)
	if !ok {
		return nil, nil
	}
	a, m, path, pos, ok := s.at(rel, p.Position, true)
	if !ok {
		return nil, nil
	}
	text := hoverText(a, m, path, pos)
	if text == "" {
		return nil, nil
	}
	return map[string]any{"contents": map[string]any{"kind": "markdown", "value": text}}, nil
}

// hoverText is the hover at pos, "" for none: a hole's type; what target names (a local's type,
// a value's or constructor's signature with its doc comment, a module); else the type of the
// innermost expression.
func hoverText(a *analysis, m *syntax.Module, path []syntax.Spanned, pos diag.Pos) string {
	n := path[len(path)-1]
	sig := func(name string, t types.Type) string {
		return "```alder\n" + name + " : " + types.StringIn(t, m.Name) + "\n```"
	}
	switch n := n.(type) {
	case *syntax.HoleExpr:
		if t, ok := a.res.Types[n]; ok && t != nil {
			return sig("?"+n.Name, t)
		}
		return ""
	case *syntax.HtmlAttr, *syntax.Element:
		if h := markupHover(path); h != "" {
			return h
		}
	}
	// a page hook's name at its declaration, or the page header itself
	if d := declAt(m, pos); d != nil && a.prog.Pages[m.Name] != nil {
		switch d := d.(type) {
		case *syntax.PageDecl:
			if path[len(path)-1] == syntax.Spanned(d) { // the header, not a type inside its parameters
				return pageHover(a, m, d)
			}
		case *syntax.ValueDecl:
			if syntax.Contains(nameSpan(d, m), pos) {
				if h := hookHover(a, m.Name, d.Name); h != "" {
					return withDoc(h, syntax.DocOf(m.Lines, d.GetSpan().Start.Line))
				}
			}
		}
	}
	if ref, local, ok := target(a, m, path, pos); ok {
		switch {
		case local != nil:
			if t, ok := a.res.Binds[local.Key]; ok && t != nil {
				return sig(local.Name, t)
			}
		case ref.Kind == syntax.RefNone:
			return moduleHover(a, ref.Module)
		case ref.Kind == syntax.RefType:
			// a type reached through a qualifier hovers with its module's full path
			if tn, ok := n.(*syntax.TypeName); ok && tn.Module != "" && a.prog.Modules[ref.Module] != nil {
				return withDoc("```alder\ntype "+ref.Module+"."+ref.Name+"\n```", declDoc(a, ref))
			}
		case ref.Kind == syntax.RefCtor:
			name := ref.Name
			if cr, ok := n.(*syntax.CtorRef); ok && cr.Module != "" && a.prog.Modules[ref.Module] != nil {
				name = ref.Module + "." + ref.Name // a brand written `Todo.Tag` is `Table.Todo.Tag` in full
			}
			if ci := a.res.Ctors[ref.Module+"."+ref.Name]; ci != nil && ci.Union != nil {
				var t types.Type = ci.Union
				if ci.Arg != nil {
					t = &types.Fun{Params: []types.Type{ci.Arg}, Result: ci.Union}
				}
				return withDoc(sig(name, t), declDoc(a, ref))
			}
			if t := a.res.Types[n]; t != nil && name != ref.Name {
				return withDoc(sig(name, t), declDoc(a, ref))
			}
		case ref.Kind == syntax.RefValue, ref.Kind == syntax.RefStd, ref.Kind == syntax.RefComponent:
			if h := hookHover(a, ref.Module, ref.Name); h != "" {
				return withDoc(h, declDoc(a, ref))
			}
			t, ok := a.res.Values[ref.Module+"."+ref.Name]
			if !ok || t == nil {
				t = a.res.Types[n]
			}
			if t != nil {
				name := ref.Name
				if v, ok := n.(*syntax.Var); ok && v.Module != "" && a.prog.Modules[ref.Module] != nil {
					name = ref.Module + "." + ref.Name // `Todo.find` is `Table.Todo.find` in full
				}
				return withDoc(sig(name, t), declDoc(a, ref))
			}
		}
	}
	if t, ok := a.res.Types[n]; ok && t != nil {
		return "```alder\n" + types.StringIn(t, m.Name) + "\n```"
	}
	return ""
}

// markupHover is the hover of an element or an attribute named by the last node of path: the
// manifest's description of a built-in tag or attribute, "" for a component, a custom element or
// a name the manifest does not know.
func markupHover(path []syntax.Spanned) string {
	switch n := path[len(path)-1].(type) {
	case *syntax.Element:
		if t := markup.HTML.Tag(n.Tag); t != nil && n.Ref.Kind != syntax.RefComponent {
			return "`<" + t.Name + ">` — " + t.Description
		}
	case *syntax.HtmlAttr:
		for i := len(path) - 2; i >= 0; i-- {
			if el, ok := path[i].(*syntax.Element); ok {
				if a := markup.HTML.Attr(el.Tag, n.Name); a != nil {
					return "`" + a.Name + "` — " + a.Description
				}
				break
			}
		}
	}
	return ""
}

// declDoc is the doc comment above the declaration of ref, in its app or stdlib module.
func declDoc(a *analysis, ref syntax.Ref) string {
	mod, d := findDecl(a, ref)
	if d == nil {
		return ""
	}
	return syntax.DocOf(mod.Lines, d.GetSpan().Start.Line)
}

// withDoc is sig followed by doc, when there is one, as its own paragraph.
func withDoc(sig, doc string) string {
	if doc == "" {
		return sig
	}
	return sig + "\n\n" + doc
}

// moduleHover is "`Name` · its file · how many declarations": Alder has no export list, so
// every declaration counts.
func moduleHover(a *analysis, name string) string {
	mod := modulePath(a, name)
	if mod == nil {
		return ""
	}
	return fmt.Sprintf("`%s` · %s · %d declarations", name, mod.File, len(mod.Decls))
}

// hookHover is the hover of page module mod's hook name (load, view, update): its signature as
// checked (the required one when the module has none yet) and the hook's role. "" when mod is
// not a page or name is not a hook.
func hookHover(a *analysis, mod, name string) string {
	if a.prog.Pages[mod] == nil {
		return ""
	}
	sig, role, ok := check.HookDoc(name)
	if !ok {
		return ""
	}
	if a.res != nil {
		if t, has := a.res.Values[mod+"."+name]; has && t != nil {
			sig = types.StringIn(t, mod)
		}
	}
	return "```alder\n" + name + " : " + sig + "\n```\n\n" + role
}

// pageHover is the hover of a page header: its path, parameter record and `requires`, `role` and `silo` clauses.
func pageHover(a *analysis, m *syntax.Module, d *syntax.PageDecl) string {
	lines := []string{"page \"" + d.Path + "\""}
	if a.res != nil {
		if pi := a.res.Pages[m.Name]; pi != nil && pi.Params != nil {
			anon := *pi.Params // the generated record is named `Params`; show its fields
			anon.Anonymous = true
			lines = append(lines, "params : "+types.StringIn(&anon, m.Name))
		}
	}
	if len(d.Requires) > 0 {
		lines = append(lines, "requires "+strings.Join(d.Requires, ", "))
	}
	if len(d.Roles) > 0 {
		lines = append(lines, "roles "+strings.Join(d.Roles, " | "))
	}
	if d.Silo != "" {
		lines = append(lines, "silo "+d.Silo)
	}
	return "```alder\n" + strings.Join(lines, "\n") + "\n```\n\n" +
		"The page's route and what a request needs. `load`, `view` and `update` are its hooks; " +
		"`Params` is this record, `Msg` its messages."
}
