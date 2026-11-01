package semtok

import (
	"alder/internal/diag"
	"alder/internal/syntax"
)

// run is the dotted name at the start of ts, `A` or `A.B.C`: its tokens, dots included.
func run(ts []syntax.Token) []syntax.Token {
	n := 0
	for n < len(ts) && ts[n].Kind == syntax.UIdent {
		n++
		if n+1 < len(ts) && ts[n].Kind == syntax.Dot && ts[n+1].Kind == syntax.UIdent {
			n++
			continue
		}
		break
	}
	return ts[:n]
}

// dotted colours a dotted name: each segment but the last is a namespace, the last is last.
func (w *walker) dotted(name []syntax.Token, last Kind) {
	for i, t := range name {
		switch {
		case t.Kind == syntax.Dot:
		case i == len(name)-1:
			w.set(t, last, 0)
		default:
			w.set(t, TNamespace, 0)
		}
	}
}

// negative makes `-1` (a `-` then a number, the literal's span sp) one number token.
func (w *walker) negative(sp diag.Span) {
	if ts := w.in(sp); len(ts) == 2 && ts[0].Kind == syntax.Minus {
		w.cls[pos(ts[0])] = class{kind: TNumber, end: sp.End}
		w.set(ts[1], none, 0)
	}
}

// first is the first token of sp, if any.
func (w *walker) first(sp diag.Span) (syntax.Token, bool) {
	if ts := w.in(sp); len(ts) > 0 {
		return ts[0], true
	}
	return syntax.Token{}, false
}

// keywordAt colours the first token of sp as a keyword (a contextual one the lexer does not reserve).
func (w *walker) keywordAt(sp diag.Span) {
	if t, ok := w.first(sp); ok {
		w.set(t, TKeyword, 0)
	}
}

// rule records what n says of its own tokens: the transcription of highlights.scm's patterns.
func (w *walker) rule(n syntax.Spanned) {
	switch n := n.(type) {
	case *syntax.Module: // `module A.B`
		if ts := w.in(n.Span); len(ts) > 0 && ts[0].Kind == syntax.Keyword && ts[0].Text == "module" {
			w.dotted(run(ts[1:]), TNamespace)
		}
	case *syntax.Import: // `import A.B [as C]`
		ts := w.in(n.Span)
		if len(ts) < 2 {
			break
		}
		name := run(ts[1:])
		w.dotted(name, TNamespace)
		if rest := ts[1+len(name):]; len(rest) > 0 && rest[0].Kind == syntax.LIdent && rest[0].Text == "as" {
			w.set(rest[0], TKeyword, 0)
			if len(rest) > 1 && rest[1].Kind == syntax.UIdent {
				w.set(rest[1], TNamespace, 0)
			}
		}
	case *syntax.ValueDecl: // `f : T` / `f x y = …`, and a fake's entries
		ts := w.in(n.Span)
		if len(ts) == 0 {
			break
		}
		w.set(ts[0], TFunction, Declaration)
		if n.Sig != nil {
			end := n.Sig.GetSpan().End
			for _, t := range ts {
				if !before(pos(t), end) && t.Kind == ts[0].Kind && t.Text == ts[0].Text {
					w.set(t, TFunction, Declaration) // the definition line's name
					break
				}
			}
		}
		for _, p := range n.Params {
			w.setAt(p.Span.Start, TParameter, Declaration)
		}
	case *syntax.TypeDecl: // `type T a b = …`
		ts := w.in(n.Span)
		for i, t := range ts {
			if t.Kind != syntax.UIdent {
				continue
			}
			w.set(t, TType, Declaration)
			for _, p := range ts[i+1:] {
				if p.Kind != syntax.LIdent {
					break
				}
				w.set(p, TType, 0) // a type parameter: (type_var) @type
			}
			break
		}
	case *syntax.Ctor:
		if t, ok := w.first(n.Span); ok {
			w.set(t, TEnumMember, Declaration)
		}
	case *syntax.BrandType: // `brand Base [where …]`
		if ts := w.in(n.Span); len(ts) > 1 && ts[1].Kind == syntax.UIdent {
			w.set(ts[1], TType, 0)
		}
	case *syntax.TypeName:
		w.dotted(run(w.in(n.Span)), TType)
	case *syntax.TypeVar:
		if t, ok := w.first(n.Span); ok {
			w.set(t, TType, 0)
		}
	case *syntax.Field: // `name: T [via U] [= d] [as "K"] [@attr…]`
		if t, ok := w.first(n.Span); ok {
			w.set(t, TProperty, 0)
		}
		if n.Via != "" {
			w.keywordAt(n.ViaSpan)
		}
		if n.Wire != "" {
			w.keywordAt(n.WireSpan)
		}
	case *syntax.Attr: // `@name [arg]`
		ts := w.in(n.Span)
		if len(ts) > 1 {
			w.set(ts[1], TDecorator, 0)
		}
		if len(ts) > 2 {
			switch ts[2].Kind {
			case syntax.UIdent:
				w.set(ts[2], TType, 0)
			case syntax.LIdent:
				w.set(ts[2], TProperty, 0)
			}
		}
	case *syntax.PageDecl: // `requires user, org, role R | S, silo k`
		for i, sp := range n.FieldSpans {
			ts := w.in(sp)
			if i == 0 && len(ts) > 1 { // FieldSpans[0] is RequiresSpan, from `requires`
				w.set(ts[1], TProperty, 0)
			} else if i > 0 && len(ts) > 0 {
				w.set(ts[0], TProperty, 0)
			}
		}
		if n.Roles != nil {
			w.keywordAt(n.RolesSpan)
		}
		if ts := w.in(n.SiloSpan); n.Silo != "" && len(ts) > 1 {
			w.set(ts[0], TKeyword, 0)
			w.set(ts[1], TProperty, 0)
		}
	case *syntax.SiloDecl: // `silo k { T, U }`
		w.keywordAt(n.Span)
		if t, ok := w.first(n.KeySpan); ok {
			w.set(t, TProperty, 0)
		}
		for _, sp := range n.TableSpans {
			if t, ok := w.first(sp); ok {
				w.set(t, TType, 0)
			}
		}
	case *syntax.ChooseDecl: // `choose k = A.B`
		w.keywordAt(n.Span)
		if t, ok := w.first(n.KeySpan); ok {
			w.set(t, TProperty, 0)
		}
		w.pageAfterEqual(n.Span)
	case *syntax.LoginDecl: // `login = A.B`
		w.pageAfterEqual(n.Span)
	case *syntax.MsgDecl: // `msg M [{ … }] [form F]`
		ts := w.in(n.Span)
		from := diag.Pos{}
		if n.Params != nil {
			from = n.Params.Span.End
		}
		for _, t := range ts[min(2, len(ts)):] {
			if t.Kind == syntax.LIdent && t.Text == "form" && !before(pos(t), from) {
				w.set(t, TKeyword, 0)
			}
		}
	case *syntax.TableDecl: // `table` is a keyword token only at the top level; in a fake it is a name
		w.keywordAt(n.Span)
	case *syntax.DeployDecl:
		w.keywordAt(n.Span)
		w.deployFields(n.Fields)
	case *syntax.ThemeDecl:
		w.keywordAt(n.Span)
		w.deployFields(n.Fields)
	}
}

// pageAfterEqual colours the dotted module name after the `=` of sp's tokens as namespaces.
func (w *walker) pageAfterEqual(sp diag.Span) {
	ts := w.in(sp)
	for i, t := range ts {
		if t.Kind == syntax.Equal {
			w.dotted(run(ts[i+1:]), TNamespace)
			return
		}
	}
}

// deployFields colours a deploy or theme record: names are properties, a target's kind a
// constructor, and the values inside.
func (w *walker) deployFields(fs []*syntax.DeployField) {
	for _, f := range fs {
		if t, ok := w.first(f.NameSpan); ok {
			w.set(t, TProperty, 0)
		}
		w.deployValue(f.Value)
	}
}

func (w *walker) deployValue(v *syntax.DeployValue) {
	if v == nil {
		return
	}
	switch v.Kind {
	case "int", "float":
		w.negative(v.Span)
	case "ctor":
		if t, ok := w.first(v.Span); ok {
			w.set(t, TEnumMember, 0)
		}
		w.deployValue(v.Arg)
		w.deployFields(v.Rec)
	case "record":
		w.deployFields(v.Rec)
	case "list":
		for _, e := range v.List {
			w.deployValue(e)
		}
	}
}
