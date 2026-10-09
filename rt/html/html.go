// Package html builds HTML trees for generated Alder views. Text and
// attribute values are kept raw in the tree and escaped exactly once, by
// Render.
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
)

// Node is an HTML node. Its zero value renders nothing.
type Node struct {
	kind     kind
	tag      string // kElem
	text     string // kText, kRaw
	attrs    []Attr // kElem
	children []Node // kElem, kFrag
}

// Attr is one attribute. Bool attributes render bare (`disabled`); Omit
// attributes do not render at all.
type Attr struct {
	Name, Value string
	Bool        bool
	Omit        bool
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
func Render(n Node) string {
	var b strings.Builder
	render(&b, n)
	return b.String()
}

func render(b *strings.Builder, n Node) {
	switch n.kind {
	case kText:
		b.WriteString(html.EscapeString(n.text))
	case kRaw:
		b.WriteString(n.text)
	case kFrag:
		for _, c := range n.children {
			render(b, c)
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
				b.WriteString(html.EscapeString(a.Value))
				b.WriteByte('"')
			}
		}
		if void[n.tag] {
			b.WriteString(" />")
			return
		}
		b.WriteByte('>')
		for _, c := range n.children {
			render(b, c)
		}
		b.WriteString("</")
		b.WriteString(n.tag)
		b.WriteByte('>')
	}
}
