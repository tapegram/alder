package generate

import (
	"strings"

	"alder/internal/syntax"
	"alder/internal/types"
)

// findFor is the `{for <binder> in …}` of module m's `view` whose loop variable is binder, and
// how many there are.
func findFor(m *syntax.Module, binder string) (*syntax.HtmlFor, int) {
	v := valueDecl(m, "view")
	if v == nil {
		return nil, 0
	}
	var found *syntax.HtmlFor
	n := 0
	var walk func(e syntax.Expr)
	walk = func(e syntax.Expr) {
		switch x := e.(type) {
		case *syntax.Element:
			for _, a := range x.Attrs {
				if a.Value != nil {
					walk(a.Value)
				}
			}
			for _, c := range x.Children {
				walk(c)
			}
		case *syntax.HtmlExpr:
			walk(x.Expr)
		case *syntax.HtmlFor:
			if x.Var == binder {
				if found == nil {
					found = x
				}
				n++
			}
			walk(x.Body)
		case *syntax.HtmlIf:
			walk(x.Then)
			if x.Else != nil {
				walk(x.Else)
			}
		case *syntax.If:
			walk(x.Then)
			walk(x.Else)
		case *syntax.HtmlMatch:
			for _, a := range x.Arms {
				walk(a.Body)
			}
		case *syntax.Match:
			for _, a := range x.Arms {
				walk(a.Body)
			}
		case *syntax.Block:
			walk(x.Result)
		}
	}
	walk(v.Body)
	return found, n
}

// binderValue is the value a message's field takes from a loop's binder of type bt, when it
// has one: the binder's own field of the same name (`m.user` for `user`), else, for a field of
// a table's id type, the binder's id when the binder is a row of that table (`u.id`); "" for none.
func binderValue(p *Program, binder string, bt types.Type, field, fieldType, home string) string {
	r, ok := bt.(*types.Record)
	if !ok {
		return ""
	}
	for _, f := range r.Fields {
		if f.Name == field && typeText(p, f.Type, home) == fieldType {
			return binder + "." + field
		}
	}
	for _, t := range p.Tables {
		info := p.Res.Tables[t.Module+"."+t.Name]
		if info == nil || info.Record == nil || types.String(info.Record) != types.String(r) {
			continue
		}
		if typeText(p, t.IDType, home) == fieldType {
			return binder + "." + t.IDField
		}
	}
	return ""
}

// addToFor puts markup at the end of the element that is the body of loop f: on a line of its
// own before the element's closing tag when that tag has a line of its own, else just before it.
func addToFor(e *srcEdit, f *syntax.HtmlFor, markup string) bool {
	el, ok := f.Body.(*syntax.Element)
	if !ok || el.Span.End.Line < 1 {
		return false
	}
	end := el.Span.End
	l := e.line(end.Line)
	closeTag := "</" + el.Tag + ">"
	if el.Module != "" {
		closeTag = "</" + el.Module + "." + el.Tag + ">"
	}
	// the span's end is the closing tag's last character, or the one after it
	at := -1
	for _, stop := range []int{end.Col, end.Col - 1} {
		if a := stop - len(closeTag); a >= 0 && stop <= len(l) && l[a:stop] == closeTag {
			at = a
			break
		}
	}
	if at < 0 {
		return false // self-closing, or not where the parser says
	}
	if strings.TrimSpace(l[:at]) == "" && end.Line != el.Span.Start.Line {
		e.insertBefore(end.Line, indent(markup, indentOf(l)+"  "))
		return true
	}
	e.replaceLine(end.Line, l[:at]+markup+l[at:])
	return true
}

// kebab is a message's name as a class: "SendInvite" → "send-invite".
func kebab(name string) string { return strings.ReplaceAll(strings.ToLower(Humanize(name)), " ", "-") }
