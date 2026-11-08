package lsp

import (
	"encoding/json"
	"fmt"
	"strings"

	"alder/internal/diag"
	"alder/internal/elements"
	"alder/internal/markup"
	"alder/internal/scratch"
	"alder/internal/syntax"
	"alder/internal/theme"
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
	var text string
	if scratch.IsScratchFile(rel) {
		pl, ok := s.scratchAt(rel, p.Position, true)
		if !ok {
			return nil, nil
		}
		text = hoverOrTheme(pl.v.a, pl.m, pl.path, pl.pos)
	} else {
		a, m, path, pos, ok := s.at(rel, p.Position, true)
		if !ok {
			return nil, nil
		}
		text = hoverOrTheme(a, m, path, pos)
	}
	if text == "" {
		return nil, nil
	}
	return map[string]any{"contents": map[string]any{"kind": "markdown", "value": text}}, nil
}

// hoverOrTheme is a theme declaration's field hover, else hoverText when pos has an expression path.
func hoverOrTheme(a *analysis, m *syntax.Module, path []syntax.Spanned, pos diag.Pos) string {
	if d, isTheme := declAt(m, pos).(*syntax.ThemeDecl); isTheme {
		return themeHover(d.Fields, pos)
	}
	if len(path) > 0 {
		return hoverText(a, m, path, pos)
	}
	return ""
}

// hoverText is the hover at pos, "" for none: a hole's type; what target names (a local's type,
// a value's or constructor's signature with its doc comment, a module); else the type of the
// innermost expression.
func hoverText(a *analysis, m *syntax.Module, path []syntax.Spanned, pos diag.Pos) string {
	n := path[len(path)-1]
	sig := func(name string, t types.Type) string {
		return "```alder\n" + name + " : " + types.StringQ(t, m.Name, a.prog.Qualify(m.Name)) + "\n```"
	}
	switch n := n.(type) {
	case *syntax.HoleExpr:
		if t, ok := a.res.Types[n]; ok && t != nil {
			return sig("?"+n.Name, t)
		}
		return ""
	case *syntax.HtmlAttr, *syntax.Element:
		if h := markupHover(manifestOf(a), path); h != "" {
			return h
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
		case ref.Kind == syntax.RefRoute:
			if mod, d := findDecl(a, ref); d != nil {
				if rd, ok := d.(*syntax.RouteDecl); ok {
					return withDoc("```alder\n"+routeHover(mod, rd)+"\n```", syntax.DocOf(mod.Lines, rd.Span.Start.Line))
				}
			}
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
			t, ok := a.res.Values[ref.Module+"."+ref.Name]
			if !ok || t == nil {
				t = a.res.Types[n]
			}
			if t != nil {
				name := ref.Name
				if v, ok := n.(*syntax.Var); ok && v.Module != "" && a.prog.Modules[ref.Module] != nil {
					name = ref.Module + "." + ref.Name // `Todo.find` is `Table.Todo.find` in full
				}
				return withDoc(sig(name, t)+helperLine(a, ref), declDoc(a, ref))
			}
		}
	}
	if t, ok := a.res.Types[n]; ok && t != nil {
		return "```alder\n" + types.StringQ(t, m.Name, a.prog.Qualify(m.Name)) + "\n```"
	}
	return ""
}

// markupHover is the hover of an element or an attribute named by the last node of path: the
// manifest's description of a custom element or its attribute, or of a built-in HTML tag or
// attribute; "" for a component or a name neither knows.
func markupHover(m *elements.Manifest, path []syntax.Spanned) string {
	switch n := path[len(path)-1].(type) {
	case *syntax.Element:
		if n.Ref.Kind == syntax.RefComponent {
			return ""
		}
		if strings.Contains(n.Tag, "-") {
			if el := m.Lookup(n.Tag); el != nil {
				return elementHover(el)
			}
		}
		if t := markup.HTML.Tag(n.Tag); t != nil {
			return "`<" + t.Name + ">` — " + t.Description
		}
	case *syntax.HtmlAttr:
		for i := len(path) - 2; i >= 0; i-- {
			if el, ok := path[i].(*syntax.Element); ok {
				if strings.Contains(el.Tag, "-") {
					if ce := m.Lookup(el.Tag); ce != nil {
						if at := ce.Attr(n.Name); at != nil {
							return attrLine(*at)
						}
					}
				}
				if a := markup.HTML.Attr(el.Tag, n.Name); a != nil {
					return "`" + a.Name + "` — " + a.Description
				}
				break
			}
		}
	}
	return ""
}

// manifestOf is the custom elements manifest of the analysis: the checker's merged one, else the
// built-in alder-ui.
func manifestOf(a *analysis) *elements.Manifest {
	if a != nil && a.res != nil && a.res.Elements != nil {
		return a.res.Elements
	}
	return elements.Builtin()
}

// elementHover is the markdown hover of custom element el: its description, its attributes in
// manifest order and its slots.
func elementHover(el *elements.Element) string {
	var b strings.Builder
	b.WriteString("`<" + el.Tag + ">`")
	if el.Description != "" {
		b.WriteString(" — " + el.Description)
	}
	if len(el.Attrs) > 0 {
		b.WriteString("\n\n**Attributes**")
		for _, at := range el.Attrs {
			b.WriteString("\n- " + attrLine(at))
		}
	}
	if len(el.Slots) > 0 {
		b.WriteString("\n\n**Slots**: ")
		for i, s := range el.Slots {
			if i > 0 {
				b.WriteString("; ")
			}
			name := s.Name
			if name == "" {
				name = "default"
			}
			b.WriteString(name)
			if s.Description != "" {
				b.WriteString(" — " + s.Description)
			}
		}
	}
	return b.String()
}

// attrLine is "`name`: `type` = `default` — description", leaving out what the attribute lacks.
func attrLine(at elements.Attr) string {
	s := "`" + at.Name + "`"
	if at.Type != "" {
		s += ": `" + at.Type + "`"
	}
	if at.Default != "" {
		s += " = `" + at.Default + "`"
	}
	if at.Description != "" {
		s += " — " + at.Description
	}
	return s
}

// declDoc is the doc comment above the declaration of ref, in its app or stdlib module.
func declDoc(a *analysis, ref syntax.Ref) string {
	mod, d := findDecl(a, ref)
	if d == nil {
		return ""
	}
	return syntax.DocOf(mod.Lines, d.GetSpan().Start.Line)
}

// helperLine is the hover's note on a test helper (#4), "" for any other value.
func helperLine(a *analysis, ref syntax.Ref) string {
	if why := a.res.TestOnly[ref.Module+"."+ref.Name]; why != "" {
		return "\n\ntest helper (uses " + why + ")"
	}
	return ""
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

// routeHover is a route's method, path, parameters, form and guard, as `PATCH /todos/{id}/toggle
// { id: Todo.TodoId }  requires user` (two spaces between the parts).
func routeHover(mod *syntax.Module, d *syntax.RouteDecl) string {
	parts := []string{d.Method + " " + d.Path}
	if d.Params != nil {
		parts = append(parts, spanText(mod.Lines, d.Params.Span))
	}
	if d.Form != "" {
		parts = append(parts, "form "+d.Form)
	}
	if g := guardText(d.Guard); g != "" {
		parts = append(parts, g)
	}
	return strings.Join(parts, "  ")
}

// guardText is a `requires` clause as written: `requires user, org, role Admin | Mod, silo org`;
// "" for none.
func guardText(g syntax.Guard) string {
	var cl []string
	if len(g.Requires) > 0 {
		cl = append(cl, strings.Join(g.Requires, ", "))
	}
	if len(g.Roles) > 0 {
		cl = append(cl, "role "+strings.Join(g.Roles, " | "))
	}
	if g.Silo != "" {
		cl = append(cl, "silo "+g.Silo)
	}
	if len(cl) == 0 {
		return ""
	}
	return "requires " + strings.Join(cl, ", ")
}

// spanText is the source text of sp in lines, its whitespace (line breaks too) folded to single
// spaces; "" when sp is outside lines.
func spanText(lines []string, sp diag.Span) string {
	if sp.Start.Line < 1 || sp.End.Line > len(lines) || sp.End.Line < sp.Start.Line {
		return ""
	}
	var b strings.Builder
	for l := sp.Start.Line; l <= sp.End.Line; l++ {
		line := lines[l-1]
		lo, hi := 0, len(line)
		if l == sp.Start.Line {
			lo = min(max(sp.Start.Col-1, 0), len(line))
		}
		if l == sp.End.Line {
			hi = min(max(sp.End.Col-1, lo), len(line))
		}
		b.WriteString(line[lo:hi] + " ")
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// themeHover is the hover of a token's name in App's theme (fs, or a record inside it): its CSS
// name, kind, description and alder-ui's default values; "" elsewhere.
func themeHover(fs []*syntax.DeployField, pos diag.Pos) string {
	for _, f := range fs {
		if f.Value != nil && f.Value.Kind == "record" && syntax.Contains(f.Value.Span, pos) {
			return themeHover(f.Value.Rec, pos)
		}
		if !syntax.Contains(f.NameSpan, pos) {
			continue
		}
		t, ok := theme.Lookup(f.Name)
		if !ok {
			return ""
		}
		d := theme.Defaults()[t.CSS]
		return fmt.Sprintf("`%s`: `%s`, a %s token of alder-ui %s\n\n%s\n\nalder-ui's default: light `%s`, dark `%s`", t.Field, t.CSS, t.Kind, theme.Version(), t.Description, d.Light, d.Dark)
	}
	return ""
}
