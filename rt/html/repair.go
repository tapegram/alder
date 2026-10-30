package html

import "strings"

// The browser's HTML parser does not build every tree the renderer writes: it repairs invalid
// nesting the checker accepts (`<p>` holding a `<div>` closes the `<p>`, so the `<div>` and what
// follows land after it). A region around such markup is not one element in the client's DOM
// with that markup inside, so patching it (mode outer) would misplace what follows: the renderer
// makes such regions inert (RenderTracked) and the patch happens at a region outside them, or
// the body is morphed. Repair models the same rules on a tree, as a test's browser holds a page.
//
// The rules are a subset of the HTML5 tree-construction rules, checked against the elements open
// at a start tag (innermost last); each is either exact or errs on the side of a repair.

// repair is what the parser does with a start tag in the context of the open elements.
type repair uint8

const (
	keep   repair = iota // inserted as written
	closes               // closes open elements first (open[at:]), then is inserted
	drops                // the tag is ignored; its content goes to the current node
	foster               // inserted before the nearest open <table> (foster parenting)
	asText               // inside an element whose content is text (textarea, title, noscript)
)

// closesP are the start tags that close an open <p> (in button scope).
var closesP = setOf("address article aside blockquote center details dialog dir div dl fieldset figcaption figure footer form h1 h2 h3 h4 h5 h6 header hgroup hr li dd dt main menu nav ol p pre section summary table ul listing plaintext xmp search")

// scopeStops end the search for an open element "in scope".
var scopeStops = setOf("applet caption html table td th marquee object template")

// tableContent are the elements allowed directly in each table-section element; anything else
// is foster parented. (td, th and tr in a table get the implied wrappers, not fostered.)
var tableContent = map[string]map[string]bool{
	"table": setOf("caption colgroup col thead tbody tfoot tr td th script template style"),
	"tbody": setOf("tr td th script template style"),
	"thead": setOf("tr td th script template style"),
	"tfoot": setOf("tr td th script template style"),
	"tr":    setOf("td th script template style"),
}

// selectContent are the start tags a <select> keeps; the others are ignored.
var selectContent = setOf("option optgroup hr script template")

// textContentOf are the elements whose content the parser reads as text.
var textContentOf = setOf("textarea title noscript")

var headings = setOf("h1 h2 h3 h4 h5 h6")

func setOf(s string) map[string]bool {
	m := map[string]bool{}
	for _, w := range strings.Fields(s) {
		m[w] = true
	}
	return m
}

// repairOf is what the parser does with start tag tag when open are the open elements
// (innermost last), and for closes the index of the first element it closes.
func repairOf(open []string, tag string) (repair, int) {
	cur := ""
	if len(open) > 0 {
		cur = open[len(open)-1]
	}
	for _, o := range open {
		if textContentOf[o] {
			return asText, 0
		}
	}
	switch tag {
	case "html":
		if len(open) > 0 {
			return drops, 0
		}
	case "head", "body":
		for _, o := range open {
			if o == "body" || o == "head" {
				return drops, 0
			}
		}
	}
	for i := len(open) - 1; i >= 0; i-- {
		if open[i] == "select" {
			if !selectContent[tag] {
				return drops, 0
			}
			break
		}
	}
	if allowed, ok := tableContent[cur]; ok && !allowed[tag] {
		return foster, 0
	}
	if closesP[tag] {
		if i := inScope(open, "p", nil); i >= 0 {
			return closes, i
		}
	}
	switch {
	case tag == "a":
		if i := inScope(open, "a", nil); i >= 0 {
			return closes, i
		}
	case tag == "form":
		for _, o := range open {
			if o == "form" {
				return drops, 0
			}
		}
	case tag == "button":
		if i := inScope(open, "button", nil); i >= 0 {
			return closes, i
		}
	case tag == "li":
		if i := inScope(open, "li", setOf("ul ol menu")); i >= 0 {
			return closes, i
		}
	case tag == "dd" || tag == "dt":
		if i := inScope(open, "dd", setOf("dl")); i >= 0 {
			return closes, i
		}
		if i := inScope(open, "dt", setOf("dl")); i >= 0 {
			return closes, i
		}
	case headings[tag]:
		if headings[cur] {
			return closes, len(open) - 1
		}
	case tag == "option" || tag == "optgroup":
		if cur == "option" {
			return closes, len(open) - 1
		}
	}
	return keep, 0
}

// inScope is the index of the innermost open element named tag, searching outwards and
// stopping at a scope boundary (scopeStops, and stops), or -1.
func inScope(open []string, tag string, stops map[string]bool) int {
	for i := len(open) - 1; i >= 0; i-- {
		o := open[i]
		if o == tag {
			return i
		}
		if scopeStops[o] || stops[o] || (tag == "p" && o == "button") {
			return -1
		}
	}
	return -1
}

// textFosters reports whether text that is not all whitespace, written when cur is the current
// node, is foster parented.
func textFosters(cur, text string) bool {
	_, table := tableContent[cur]
	return table && strings.TrimLeft(text, " \t\n\r\f") != ""
}

// Repair is n as the browser's parser would build it from n's markup, by the rules above: an
// element that closes open ones lands after them (and an end tag </p> with no open <p> is an
// empty <p>), a dropped start tag leaves its content to the current node, foster-parented
// content goes before the table, and elements inside textarea, title or noscript are their
// markup as text. A test's browser holds pages repaired, as a browser does.
func Repair(n Node) Node {
	t := &builder{}
	root := &bnode{n: Node{kind: kFrag}}
	t.stack = []*bnode{root}
	t.node(n)
	out := root.build()
	if len(out.children) == 1 {
		return out.children[0]
	}
	return out
}

type bnode struct {
	n    Node // without children
	kids []*bnode
}

func (b *bnode) build() Node {
	n := b.n
	if len(b.kids) > 0 {
		n.children = make([]Node, len(b.kids))
		for i, k := range b.kids {
			n.children[i] = k.build()
		}
	}
	return n
}

type builder struct {
	stack []*bnode // stack[0] is the document; the open elements follow
}

func (t *builder) open() []string {
	tags := make([]string, len(t.stack)-1)
	for i, b := range t.stack[1:] {
		tags[i] = b.n.tag
	}
	return tags
}

func (t *builder) cur() *bnode { return t.stack[len(t.stack)-1] }

// fosterParent inserts b before the innermost open table, in that table's parent.
func (t *builder) fosterParent(b *bnode) {
	for i := len(t.stack) - 1; i >= 1; i-- {
		if t.stack[i].n.tag == "table" {
			p := t.stack[i-1]
			for j, k := range p.kids {
				if k == t.stack[i] {
					p.kids = append(p.kids[:j], append([]*bnode{b}, p.kids[j:]...)...)
					return
				}
			}
		}
	}
	t.cur().kids = append(t.cur().kids, b)
}

func (t *builder) node(n Node) {
	switch n.kind {
	case kText, kRaw, kRawText:
		b := &bnode{n: n}
		if n.kind == kText && textFosters(t.cur().n.tag, n.text) {
			t.fosterParent(b)
			return
		}
		t.cur().kids = append(t.cur().kids, b)
	case kFrag:
		for _, c := range n.children {
			t.node(c)
		}
	case kCtrl:
		t.node(n.control.build(nil, false, nil))
	case kElem:
		act, at := repairOf(t.open(), n.tag)
		switch act {
		case asText:
			t.cur().kids = append(t.cur().kids, &bnode{n: Text(Render(n))})
			return
		case drops:
			for _, c := range n.children {
				t.node(c)
			}
			return
		case closes:
			t.stack = t.stack[:at+1]
		}
		b := &bnode{n: Node{kind: kElem, tag: n.tag, attrs: n.attrs}}
		if act == foster {
			t.fosterParent(b)
		} else {
			t.cur().kids = append(t.cur().kids, b)
		}
		if void[n.tag] {
			return
		}
		t.stack = append(t.stack, b)
		for _, c := range n.children {
			t.node(c)
		}
		t.end(n.tag)
	}
}

// end is the end tag of tag: the innermost open element of that name is closed with those inside
// it; with none, a </p> is an empty <p>, and anything else is ignored.
func (t *builder) end(tag string) {
	for i := len(t.stack) - 1; i >= 1; i-- {
		if t.stack[i].n.tag == tag {
			t.stack = t.stack[:i]
			return
		}
	}
	if tag == "p" {
		t.cur().kids = append(t.cur().kids, &bnode{n: Node{kind: kElem, tag: "p"}})
	}
}
