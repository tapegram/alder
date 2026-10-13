package ir

import (
	"fmt"

	"alder/internal/syntax"
	"alder/internal/types"
)

// htmlNode lowers an HTML child, arm body or branch. An expression that is not HTML syntax
// (such as `Html.none`) becomes an HExpr.
func (l *lowerer) htmlNode(e syntax.Expr) Expr {
	switch x := e.(type) {
	case *syntax.HtmlText:
		return &HText{Typed: Typed{l.typeOf(x)}, Text: x.Text}
	case *syntax.Element:
		return l.element(x)
	case *syntax.HtmlExpr:
		return &HExpr{Typed: Typed{l.typeOf(x)}, Expr: l.expr(x.Expr), Kind: hexprKind(l.typeOf(x.Expr))}
	case *syntax.HtmlFor:
		return &HFor{Typed: Typed{l.typeOf(x)}, Var: x.Var, T: l.bindType(x), List: l.expr(x.List), Body: l.htmlNode(x.Body)}
	case *syntax.HtmlIf:
		return &HIf{Typed: Typed{l.typeOf(x)}, Cond: l.expr(x.Cond), Then: l.htmlNode(x.Then), Else: l.htmlNode(x.Else)}
	case *syntax.HtmlMatch:
		return l.match(x.Subject, x.Arms, l.typeOf(x), true, posOf(x.Span))
	}
	t := l.typeOf(e)
	return &HExpr{Typed: Typed{t}, Expr: l.expr(e), Kind: hexprKind(t)}
}

func hexprKind(t types.Type) HExprKind {
	if app, ok := t.(*types.App); ok {
		switch app.Con {
		case "Html":
			return HExprHtml
		case "List":
			return HExprList
		}
	}
	return HExprText
}

func (l *lowerer) element(x *syntax.Element) Expr {
	t := l.typeOf(x)
	if x.Ref.Kind == syntax.RefComponent {
		if x.Ref.Module == "Prelude" {
			return l.input(x, x.Tag, t)
		}
		return l.component(x, t)
	}
	if kind, ok := l.res.Wrapped[x]; ok { // <al-input field="…">: the form control in its element
		return l.input(x, kind, t)
	}
	var submit *syntax.HtmlAttr
	var attrs []HAttr
	for _, a := range x.Attrs {
		attrs = append(attrs, l.attr(a))
		if x.Tag == "form" && a.Name == "submit" && a.Value != nil {
			submit = a
		}
	}
	if submit == nil {
		return &Elem{Typed: Typed{t}, Tag: x.Tag, Attrs: attrs, Children: l.children(x.Children)}
	}
	out := &FormEl{Typed: Typed{t}, Attrs: attrs}
	out.Msg, out.Page = l.submitted(submit.Value)
	saved := l.form
	l.form = nil
	if out.Msg != nil {
		l.form = out.Msg.Form
	}
	out.Children = l.children(x.Children)
	l.form = saved
	return out
}

func (l *lowerer) children(cs []syntax.Expr) []Expr {
	var out []Expr
	for _, c := range cs {
		out = append(out, l.htmlNode(c))
	}
	return out
}

// submitted finds the message a submit value names: a constructor, bare or applied.
func (l *lowerer) submitted(e syntax.Expr) (*Msg, *Page) {
	var ct *syntax.CtorRef
	switch v := e.(type) {
	case *syntax.CtorRef:
		ct = v
	case *syntax.App:
		ct, _ = v.Fn.(*syntax.CtorRef)
	}
	if ct == nil || ct.Ref.Kind != syntax.RefCtor {
		return nil, nil
	}
	msg := l.msgs[key(ct.Ref.Module, ct.Ref.Name)]
	if msg == nil {
		return nil, nil
	}
	return msg, l.pages[ct.Ref.Module]
}

func (l *lowerer) attr(a *syntax.HtmlAttr) HAttr {
	if a.Value == nil {
		return HAttr{Name: a.Name, Kind: AttrBool, Value: &Lit{Typed: Typed{types.Bool}, Kind: LitBool, Bool: true}}
	}
	v := l.expr(a.Value)
	kind := AttrText
	switch {
	case a.Name == "href":
		kind = AttrHref
	case a.Name == "click":
		kind = AttrClick
	case a.Name == "submit":
		kind = AttrSubmit
	default:
		switch t := v.Type().(type) {
		case *types.Con:
			if t.Name == "Bool" {
				kind = AttrBool
			}
		case *types.Union:
			kind = AttrMaybe // the checker allows only Text and Maybe Text here
		}
	}
	return HAttr{Name: a.Name, Kind: kind, Value: v}
}

// component lowers <Comp attrs>children</Comp> to a call of the component with a props record.
func (l *lowerer) component(x *syntax.Element, t types.Type) Expr {
	fn := l.values[key(x.Ref.Module, x.Ref.Name)]
	if fn == nil {
		panic(fmt.Sprintf("ir: component %s.%s at %s was not lowered", x.Ref.Module, x.Ref.Name, pos(x.Span)))
	}
	sig := fn.Type.(*types.Fun)
	all := l.recordType(sig.Params[0])
	props := &RecordType{} // the props record without `children`, which Comp carries itself
	for _, f := range all.Fields {
		if f.Name != "children" {
			props.Fields = append(props.Fields, f)
		}
	}
	given := map[string]syntax.Expr{}
	for _, a := range x.Attrs {
		given[a.Name] = a.Value
	}
	out := &Comp{Typed: Typed{t}, Fn: fn, Props: l.recordOf(props, propsType(props), given, pos(x.Span))}
	out.Children = l.children(x.Children)
	return out
}

func propsType(rt *RecordType) types.Type {
	rec := &types.Record{Anonymous: true}
	for _, f := range rt.Fields {
		rec.Fields = append(rec.Fields, types.Field{Name: f.Name, Type: f.Type})
	}
	return rec
}

// input lowers <Input>, <Textarea>, <Select> and <Errors>, bound to the enclosing form, and a
// built-in input element wrapping one (check.Result.Wrapped: kind "Input", "Textarea",
// "Select" or "Checkbox"), whose tag and other attributes become Wrap and WrapAttrs.
func (l *lowerer) input(x *syntax.Element, kind string, t types.Type) Expr {
	out := &Input{Typed: Typed{t}, Form: l.form}
	switch kind {
	case "Input", "Checkbox":
		out.Kind = InputInput
	case "Textarea":
		out.Kind = InputTextarea
	case "Select":
		out.Kind = InputSelect
	case "Errors":
		out.Kind = InputErrors
	}
	if kind != x.Tag {
		out.Wrap = x.Tag
	}
	for _, a := range x.Attrs {
		switch a.Name {
		case "field":
			out.Field, _ = literalText(a.Value)
		case "label":
			if s, ok := literalText(a.Value); ok {
				out.Label = s
			} else {
				out.LabelExpr = l.expr(a.Value)
			}
		case "value":
			out.Value = l.expr(a.Value)
		case "form":
			if out.Wrap == "" {
				out.BareForm = true
			}
		default:
			if out.Wrap != "" {
				out.WrapAttrs = append(out.WrapAttrs, l.attr(a))
			}
		}
	}
	return out
}

// literalText is the text of a string literal without interpolation.
func literalText(e syntax.Expr) (string, bool) {
	s, ok := e.(*syntax.StrLit)
	if !ok {
		return "", false
	}
	text := ""
	for _, p := range s.Parts {
		if p.Expr != nil {
			return "", false
		}
		text += p.Text
	}
	return text, true
}
