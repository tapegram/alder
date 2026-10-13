// Package html builds HTML trees for generated Alder views. Text and
// attribute values are kept raw in the tree and escaped exactly once, by
// Render or RenderWith. Form controls (Input) are deferred: they are built at
// render time from the Control and the request's FormState, if any.
package html

import (
	"html"
	"maps"
	"net/url"
	"slices"
	"strings"

	"alder/rt"
)

type kind uint8

const (
	kNone kind = iota
	kElem
	kText
	kRaw
	kFrag
	kCtrl
)

// Node is an HTML node. Its zero value renders nothing.
type Node struct {
	kind     kind
	tag      string   // kElem
	text     string   // kText, kRaw
	attrs    []Attr   // kElem
	children []Node   // kElem, kFrag
	control  *Control // kCtrl
}

// Attr is one attribute. Bool attributes render bare (`disabled`); Omit
// attributes do not render at all. An attribute made by MsgAttr has its value
// computed at render time from its message.
type Attr struct {
	Name, Value string
	Bool        bool
	Omit        bool
	msg         *rt.MsgRef
}

// MsgAttr is an attribute naming message m, resolved when rendered: m's URL,
// carrying the parameters of the page being rendered (FormState.PageParams; none
// without a state). Its value is, by name: "data-on:click" → "@post('<url>')",
// "data-on:submit" → SubmitPost(url), anything else (such as "action") → url. A
// <button> with a "data-on:click" MsgAttr renders as a click button (RenderWith).
func MsgAttr(name string, m rt.MsgRef) Attr { return Attr{Name: name, msg: &m} }

// value is a's value as rendered with st.
func (a Attr) value(st *FormState) string {
	if a.msg == nil {
		return a.Value
	}
	var page []rt.KV
	if st != nil {
		page = st.PageParams
	}
	url := a.msg.URL(page)
	switch a.Name {
	case "data-on:click":
		return "@post('" + url + "')"
	case "data-on:submit":
		return SubmitPost(url)
	}
	return url
}

// El is an element.
func El(tag string, attrs []Attr, children ...Node) Node {
	return Node{kind: kElem, tag: tag, attrs: attrs, children: children}
}

// Text is text content, escaped at render time.
func Text(s string) Node { return Node{kind: kText, text: s} }

// Raw is unescaped markup. Only Html.unsafe produces it.
func Raw(s string) Node { return Node{kind: kRaw, text: s} }

// Nodes is a fragment: its children render in place, with no wrapper.
func Nodes(ns ...Node) Node { return Node{kind: kFrag, children: ns} }

// None renders nothing.
func None() Node { return Node{} }

// A is a text attribute.
func A(name, value string) Attr { return Attr{Name: name, Value: value} }

// B is a boolean attribute: present when on, omitted otherwise.
func B(name string, on bool) Attr { return Attr{Name: name, Bool: true, Omit: !on} }

// M is an optional attribute: Nothing omits it.
func M(name string, v rt.Maybe[string]) Attr {
	return Attr{Name: name, Value: v.Val, Omit: !v.Ok}
}

var void = map[string]bool{
	"area": true, "base": true, "br": true, "col": true, "embed": true, "hr": true, "img": true,
	"input": true, "link": true, "meta": true, "source": true, "track": true, "wbr": true,
}

// Render serialises a node, escaping text and attribute values. Tag and
// attribute names are trusted literals supplied by the emitter, never escaped.
// It is RenderWith(n, nil): form controls render their initial values and empty
// error lists.
func Render(n Node) string { return RenderWith(n, nil) }

// String is Render(n): rt.Show prints Html values as their markup.
func (n Node) String() string { return Render(n) }

// AlderEqual is Html's equality (rt.Equaler): two nodes are equal when they render to the same
// markup (Render), so `<p>{Html.text "a"}{Html.text "b"}</p> == <p>ab</p>` is True, as are two
// views whose printed sides in a failing test would be identical.
func (n Node) AlderEqual(other any) bool { return Render(n) == Render(other.(Node)) }

// AlderKey is Html's Dict key (rt.Keyer): its rendered markup, so two nodes equal under
// AlderEqual are one key.
func (n Node) AlderKey() string { return Render(n) }

// RenderWith serialises a node like Render, filling deferred form controls from
// st (see FormState). st may be nil.
//
// A `click` button — a <button> with a "data-on:click" MsgAttr — renders so that it works
// without JavaScript too. Outside a <form> it is wrapped in a form of its own:
//
//	<form method="post" action="<url>" class="_alder-click" data-on:submit="@post('<url>', {contentType: 'form'})"><button type="submit" …>…</button></form>
//
// (style form._alder-click as display: inline to keep the button in line). Inside a <form>,
// where a form cannot nest, it is a submit button posting to its own URL instead of the form's:
//
//	<button type="submit" formaction="<url>" formmethod="post" formnovalidate data-on:click__prevent="@post('<url>')" …>
//
// Without JavaScript the browser then posts the enclosing form's fields too, which a message
// without a form ignores; with it, Datastar posts no fields. Any other element with a click
// renders data-on:click="@post('<url>')" and needs JavaScript.
func RenderWith(n Node, st *FormState) string {
	var b strings.Builder
	r := renderer{b: &b, st: st}
	r.render(n)
	return b.String()
}

// renderer is one rendering: the request's form state and where in the tree it is.
type renderer struct {
	b      *strings.Builder
	st     *FormState
	inForm bool     // inside a <form> element
	form   string   // the message key (rt.MsgRef.Key) of the enclosing <form>, "" for none
	orphan []string // the posting form's field errors no <Errors field> of it shows ("title must …")
}

func (r *renderer) render(n Node) {
	b := r.b
	switch n.kind {
	case kCtrl:
		r.render(n.control.build(r.st, r.active(n.control), r.orphan))
	case kText:
		b.WriteString(escape(n.text))
	case kRaw:
		b.WriteString(n.text)
	case kFrag:
		for _, c := range n.children {
			r.render(c)
		}
	case kElem:
		if n.tag == "button" {
			if m := clickMsg(n.attrs); m != nil {
				r.clickButton(n, m)
				return
			}
		}
		saved := *r
		if n.tag == "form" {
			r.inForm, r.form, r.orphan = true, formKey(n.attrs), nil
			if st := r.st; st != nil && st.Form != "" && st.Msg != "" && st.Msg == r.form {
				r.orphan = orphanErrors(n, st)
			}
		}
		r.open(n.tag, n.attrs)
		if void[n.tag] {
			*r = saved
			return
		}
		for _, c := range n.children {
			r.render(c)
		}
		b.WriteString("</")
		b.WriteString(n.tag)
		b.WriteByte('>')
		*r = saved
	}
}

// open writes the start tag of tag with attrs (" />" for a void element).
func (r *renderer) open(tag string, attrs []Attr) {
	b := r.b
	b.WriteByte('<')
	b.WriteString(tag)
	for _, a := range attrs {
		if a.Omit {
			continue
		}
		b.WriteByte(' ')
		b.WriteString(a.Name)
		if !a.Bool {
			v := a.value(r.st)
			if a.Name == "href" && !rt.SafeURL(v) {
				v = "#" // no javascript:, data: or protocol-relative link (rt.SafeURL)
			}
			b.WriteString(`="`)
			b.WriteString(escape(v))
			b.WriteByte('"')
		}
	}
	if void[tag] {
		b.WriteString(" />")
		return
	}
	b.WriteByte('>')
}

// clickButton writes button n, whose click posts message m (see RenderWith).
func (r *renderer) clickButton(n Node, m *rt.MsgRef) {
	url := r.url(m)
	attrs := []Attr{A("type", "submit")}
	if r.inForm {
		attrs = append(attrs, A("formaction", url), A("formmethod", "post"), B("formnovalidate", true),
			A("data-on:click__prevent", "@post('"+url+"')"))
	}
	for _, a := range n.attrs {
		if a.Name != "type" && a.Name != "data-on:click" {
			attrs = append(attrs, a)
		}
	}
	button := Node{kind: kElem, tag: "button", attrs: attrs, children: n.children}
	if r.inForm {
		saved := *r
		r.render(button)
		*r = saved
		return
	}
	r.render(El("form", []Attr{A("method", "post"), A("action", url), A("class", "_alder-click"), A("data-on:submit", SubmitPost(url))}, button))
}

// url is m's URL on the page being rendered (MsgAttr).
func (r *renderer) url(m *rt.MsgRef) string {
	var page []rt.KV
	if r.st != nil {
		page = r.st.PageParams
	}
	return m.URL(page)
}

// orphanErrors are the field errors of st that no <Errors field="…"> of its form inside form
// shows (nor a wrapped control, such as <al-input field="…">, which lists its own), each
// prefixed with its field's name, fields in name order: `<Errors form />` lists them after the
// form's own error, so no decoding error is dropped silently.
func orphanErrors(form Node, st *FormState) []string {
	shown := map[string]bool{}
	var walk func(Node)
	walk = func(n Node) {
		switch n.kind {
		case kCtrl:
			if c := n.control; (c.Kind == "errors" || c.Wrap != "") && c.Name != "" && c.Form == st.Form {
				shown[c.Name] = true // a wrapped control shows its field's errors itself
			}
		case kElem, kFrag:
			for _, c := range n.children {
				walk(c)
			}
		}
	}
	walk(form)
	var out []string
	for _, name := range slices.Sorted(maps.Keys(st.Errors)) {
		if !shown[name] {
			for _, m := range st.Errors[name] {
				out = append(out, name+" "+m)
			}
		}
	}
	return out
}

// clickMsg is the message of attrs' "data-on:click" MsgAttr, or nil.
func clickMsg(attrs []Attr) *rt.MsgRef {
	for _, a := range attrs {
		if a.Name == "data-on:click" && a.msg != nil && !a.Omit {
			return a.msg
		}
	}
	return nil
}

// formKey is the message key of a <form> with attrs: its "action" MsgAttr's (FormTo), else its
// literal action URL's, "" for none.
func formKey(attrs []Attr) string {
	for _, a := range attrs {
		if a.Name != "action" || a.Omit {
			continue
		}
		if a.msg != nil {
			return a.msg.Key()
		}
		if u, err := url.Parse(a.Value); err == nil {
			return rt.MsgKey(u.Path, u.Query())
		}
	}
	return ""
}

// active reports whether control c shows the request's form state: the state is a form's
// (Form set) and c is a control of that Alder form inside the very <form> that posted the
// message (FormState.Msg), so another form of the same type — a per-row form of another row —
// renders its initial values.
func (r *renderer) active(c *Control) bool {
	st := r.st
	return st != nil && st.Form != "" && st.Form == c.Form && st.Msg != "" && st.Msg == r.form
}

// escape escapes text or an attribute value: html.EscapeString's five characters, and "\r" as
// "&#13;", so no rendered page carries a raw carriage return (a Datastar patch splits its lines
// on it; rt/datastar normalises line ends too).
func escape(s string) string {
	e := html.EscapeString(s)
	if strings.IndexByte(e, '\r') >= 0 {
		e = strings.ReplaceAll(e, "\r", "&#13;")
	}
	return e
}

// Form is a `<form submit={…}>` posting to action: with JavaScript Datastar posts it, without
// it the browser does (method="post"). Datastar sends the form's fields url-encoded, as the
// browser would ({contentType: 'form'}; its default is the page's signals as JSON). attrs are
// the element's other attributes.
func Form(action string, attrs []Attr, children ...Node) Node {
	all := append([]Attr{A("method", "post"), A("action", action), A("data-on:submit", SubmitPost(action))}, attrs...)
	return El("form", all, children...)
}

// FormTo is a `<form submit={…}>` posting message m, like Form, with its action and
// data-on:submit resolved when rendered (MsgAttr).
func FormTo(m rt.MsgRef, attrs []Attr, children ...Node) Node {
	all := append([]Attr{A("method", "post"), MsgAttr("action", m), MsgAttr("data-on:submit", m)}, attrs...)
	return El("form", all, children...)
}

// SubmitPost is the Datastar action of a form's data-on:submit: post the form's fields
// url-encoded to action.
func SubmitPost(action string) string { return "@post('" + action + "', {contentType: 'form'})" }
