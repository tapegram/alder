package ir

import (
	"fmt"

	"alder/internal/check"
	"alder/internal/syntax"
	"alder/internal/types"
)

// htmlNode lowers an HTML child, arm body or branch. An expression that is not HTML syntax
// (such as `Html.none`) becomes an HExpr.
func (l *lowerer) htmlNode(e syntax.Expr) Expr {
	switch x := e.(type) {
	case *syntax.HtmlText:
		return &HText{Typed: Typed{l.typeOf(x)}, Text: x.Text, Raw: x.Raw}
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
		el := &Elem{Typed: Typed{t}, Tag: x.Tag, Attrs: attrs, Children: l.children(x.Children)}
		if r, ok := l.res.Rendered[x]; ok {
			completeBuiltin(el, r)
		}
		return el
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

// completeBuiltin adds what the renderer writes for built-in element el (check.Result.Rendered):
//
//	"status", "alert"  the role on an `<al-alert>`: role="status" or role="alert"
//	"popover"          popover="manual" on an `<al-toasts>`
//	"dialog"           <al-dialog heading=… dismissible><dialog id=… aria-labelledby=…>[close form][<h2 slot=header id=…>]children</dialog></al-dialog>
//	"button"           <al-button …><button type="…" data-on:click=… disabled>children</button></al-button>
//	"a"                <al-button …><a href=…>children</a></al-button>
//
// The inner `<button>` takes `click` and `type` (as written, else "button") and a copy of
// `disabled`, which stays on the element for its style; the inner `<a>` takes `href` and the
// attributes that describe the link (check.AnchorAttrs: `target`, `rel`, `data-alder-reload`, …),
// which move off the element. Every other attribute stays on the element. A link cannot be
// disabled, so a disabled one renders as `<a aria-disabled="true">` without `href` (m2), which
// takes the same link attributes; when `disabled` is computed, both are rendered under an `if`.
func completeBuiltin(el *Elem, r string) {
	text := func(s string) Expr { return &Lit{Typed: Typed{types.Text}, Kind: LitText, Text: s} }
	switch r {
	case "status", "alert":
		el.Attrs = append(el.Attrs, HAttr{Name: "role", Kind: AttrText, Value: text(r)})
		return
	}
	switch r {
	case "dialog":
		completeDialog(el, text)
		return
	case "own-dialog": // the author's `<dialog>` is theirs, but a morph must not close it
		for _, c := range el.Children {
			if d, ok := c.(*Elem); ok && d.Tag == "dialog" {
				has := false
				for _, a := range d.Attrs {
					has = has || a.Name == "data-preserve-attr"
				}
				if !has {
					d.Attrs = append(d.Attrs, HAttr{Name: "data-preserve-attr", Kind: AttrText, Value: text("open")})
				}
			}
		}
		return
	case "popover":
		el.Attrs = append(el.Attrs, HAttr{Name: "popover", Kind: AttrText, Value: text("manual")})
		return
	}
	var outer, inner, anchor []HAttr
	var typ, disabled *HAttr
	for _, a := range el.Attrs {
		if a.Name == "disabled" {
			disabled = &a
		}
		switch {
		case a.Name == "href" && r == "a", a.Name == "click":
			inner = append(inner, a)
		case r == "a" && check.AnchorAttrs[a.Name]:
			inner = append(inner, a)
			anchor = append(anchor, a)
		case a.Name == "type":
			typ = &a
			outer = append(outer, a)
		case a.Name == "disabled" && r == "button":
			inner = append(inner, a)
			outer = append(outer, a)
		default:
			outer = append(outer, a)
		}
	}
	if r == "button" {
		kind := HAttr{Name: "type", Kind: AttrText, Value: text("button")}
		if typ != nil {
			kind = *typ
		}
		inner = append([]HAttr{kind}, inner...)
	}
	el.Attrs = outer
	link := &Elem{Typed: el.Typed, Tag: r, Attrs: inner, Children: el.Children}
	if r != "a" || disabled == nil {
		el.Children = []Expr{link}
		return
	}
	off := &Elem{Typed: el.Typed, Tag: "a", Attrs: append([]HAttr{{Name: "aria-disabled", Kind: AttrText, Value: text("true")}}, anchor...), Children: el.Children}
	switch v, ok := disabled.Value.(*Lit); {
	case ok && v.Kind == LitBool && v.Bool:
		el.Children = []Expr{off}
	case ok && v.Kind == LitBool:
		el.Children = []Expr{link}
	default:
		el.Children = []Expr{&HIf{Typed: el.Typed, Cond: disabled.Value, Then: off, Else: link}}
	}
}

// completeDialog wraps the children of an `<al-dialog>` in the native `<dialog>` the library's
// stylesheet and the invoker commands (`commandfor`, `command="show-modal"`) act on: the
// element's `id` moves to it, a `heading` becomes the header slot's `<h2>` (unless a child
// already has `slot="header"`) and names the dialog (`aria-labelledby` the `<h2>`, whose id is
// the dialog's plus "-heading"; `aria-label` when there is no such `<h2>` to point at), and a
// `dismissible` dialog gets a `<form method="dialog">` with a close button first ("Close" and
// the heading), and closes on a click on the backdrop (`closedby="any"`). `dismissible={False}`
// adds neither; a computed `dismissible` adds both under an `if`.
func completeDialog(el *Elem, text func(string) Expr) {
	var outer, dlg []HAttr
	var heading, id, dismissible Expr
	for _, a := range el.Attrs {
		switch a.Name {
		case "id":
			dlg = append(dlg, a)
			if a.Kind == AttrText {
				id = a.Value
			}
		case "heading":
			if a.Kind == AttrText { // a Maybe Text heading draws no `<h2>` and names nothing
				heading = a.Value
			}
			outer = append(outer, a)
		case "dismissible":
			if v, ok := a.Value.(*Lit); !ok || v.Kind != LitBool || v.Bool {
				dismissible = a.Value
			}
			outer = append(outer, a)
		default:
			outer = append(outer, a)
		}
	}
	hasHeader := false
	for _, c := range el.Children {
		if e, ok := c.(*Elem); ok {
			for _, a := range e.Attrs {
				if l, ok := a.Value.(*Lit); a.Name == "slot" && ok && l.Text == "header" {
					hasHeader = true
				}
			}
		}
	}
	str := func(parts ...Expr) Expr { // literal parts are joined here
		joined := ""
		for _, p := range parts {
			l, ok := p.(*Lit)
			if !ok || l.Kind != LitText {
				return &Str{Typed: Typed{types.Text}, Parts: parts}
			}
			joined += l.Text
		}
		return text(joined)
	}
	var h2 *Elem
	if heading != nil && !hasHeader {
		h2 = &Elem{Typed: el.Typed, Tag: "h2", Attrs: []HAttr{{Name: "slot", Kind: AttrText, Value: text("header")}},
			Children: []Expr{&HExpr{Typed: el.Typed, Expr: heading, Kind: HExprText}}}
	}
	switch {
	case h2 != nil && id != nil:
		hid := str(id, text("-heading"))
		h2.Attrs = append(h2.Attrs, HAttr{Name: "id", Kind: AttrText, Value: hid})
		dlg = append(dlg, HAttr{Name: "aria-labelledby", Kind: AttrText, Value: hid})
	case heading != nil:
		dlg = append(dlg, HAttr{Name: "aria-label", Kind: AttrText, Value: heading})
	}
	// a morph of the page keeps the dialog open: the browser sets `open`, the server never does,
	// and Datastar leaves an attribute named here alone
	dlg = append(dlg, HAttr{Name: "data-preserve-attr", Kind: AttrText, Value: text("open")})
	var kids []Expr
	if dismissible != nil {
		label := text("Close")
		if heading != nil {
			label = str(text("Close "), heading)
		}
		btn := &Elem{Typed: el.Typed, Tag: "button", Attrs: []HAttr{
			{Name: "type", Kind: AttrText, Value: text("submit")},
			{Name: "aria-label", Kind: AttrText, Value: label},
		}, Children: []Expr{&HText{Typed: el.Typed, Text: "\u00d7"}}}
		var close Expr = &Elem{Typed: el.Typed, Tag: "form", Attrs: []HAttr{{Name: "method", Kind: AttrText, Value: text("dialog")}}, Children: []Expr{btn}}
		closedby := text("any")
		if v, ok := dismissible.(*Lit); !ok || v.Kind != LitBool {
			none := &HExpr{Typed: el.Typed, Expr: &Var{Typed: el.Typed, Kind: VarStd, Module: "Html", Name: "none"}, Kind: HExprHtml}
			close = &HIf{Typed: el.Typed, Cond: dismissible, Then: close, Else: none}
			// a modal dialog closes on Escape alone by default ("closerequest")
			closedby = &If{Typed: Typed{types.Text}, Cond: dismissible, Then: text("any"), Else: text("closerequest")}
		}
		dlg = append(dlg, HAttr{Name: "closedby", Kind: AttrText, Value: closedby})
		kids = append(kids, close)
	}
	if h2 != nil {
		kids = append(kids, h2)
	}
	kids = append(kids, el.Children...)
	el.Attrs = outer
	el.Children = []Expr{&Elem{Typed: el.Typed, Tag: "dialog", Attrs: dlg, Children: kids}}
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
			switch {
			case t.Name == "Bool":
				kind = AttrBool
			case t.Name == "Page" && a.Name == "src": // `src={Url.external "…"}`: its URL
				kind = AttrHref
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
		case "options":
			out.Options = l.expr(a.Value)
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
