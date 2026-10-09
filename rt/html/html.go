// Package html builds HTML trees for generated Alder views. Text and
// attribute values are kept raw in the tree and escaped exactly once, by
// Render or RenderWith. Form controls (Input) are deferred: they are built at
// render time from the Control and the request's FormState, if any.
package html

import (
	"html"
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
// "data-on:submit" → SubmitPost(url), anything else (such as "action") → url.
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

// RenderWith serialises a node like Render, filling deferred form controls from
// st (see FormState). st may be nil.
func RenderWith(n Node, st *FormState) string {
	var b strings.Builder
	render(&b, n, st)
	return b.String()
}

func render(b *strings.Builder, n Node, st *FormState) {
	switch n.kind {
	case kCtrl:
		render(b, n.control.build(st), st)
	case kText:
		b.WriteString(escape(n.text))
	case kRaw:
		b.WriteString(n.text)
	case kFrag:
		for _, c := range n.children {
			render(b, c, st)
		}
	case kElem:
		b.WriteByte('<')
		b.WriteString(n.tag)
		for _, a := range n.attrs {
			if a.Omit {
				continue
			}
			b.WriteByte(' ')
			b.WriteString(a.Name)
			if !a.Bool {
				b.WriteString(`="`)
				b.WriteString(escape(a.value(st)))
				b.WriteByte('"')
			}
		}
		if void[n.tag] {
			b.WriteString(" />")
			return
		}
		b.WriteByte('>')
		for _, c := range n.children {
			render(b, c, st)
		}
		b.WriteString("</")
		b.WriteString(n.tag)
		b.WriteByte('>')
	}
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
