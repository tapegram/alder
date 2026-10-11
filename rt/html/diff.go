package html

import (
	"fmt"
	"strconv"
	"strings"
)

// Difference is where two documents first differ (Diff): At is the path of the element whose
// content differs, as a selector-like chain from the body down ("body > main > ul#todos";
// "the document" for the top level), What how it differs ("left 1 child <li>, right 0"), and
// Left and Right that element's markup on each side, cut to a short context (contextLen).
type Difference struct {
	At, What    string
	Left, Right string
}

// contextLen is how many characters of an element's markup a Difference shows on each side.
const contextLen = 120

// Diff is the first difference between a and b, compared as Equal compares them (the trees as
// their markup shows them), depth first in document order; ok is false when they are equal.
// Where two elements have different numbers of children the difference is theirs (the counts
// and the children's names); otherwise the first pair of children that differs decides: a
// different element or text, a different attribute, or a difference further down.
func Diff(a, b Node) (Difference, bool) {
	return diffNodes(nil, Node{}, Node{}, normalize([]Node{a}), normalize([]Node{b}))
}

// diffNodes compares the children a and b of the elements pa and pb (zero at the top) at path.
func diffNodes(path []string, pa, pb Node, a, b []Node) (Difference, bool) {
	at := "the document"
	if len(path) > 0 {
		at = strings.Join(path, " > ")
	}
	ctx := func(n Node, ns []Node) string {
		if n.kind == kElem {
			return context(n)
		}
		return context(Node{kind: kFrag, children: ns})
	}
	if len(a) != len(b) {
		return Difference{At: at, What: "left " + count(a) + ", right " + count(b), Left: ctx(pa, a), Right: ctx(pb, b)}, true
	}
	for i := range a {
		x, y := a[i], b[i]
		switch {
		case x.kind != y.kind || x.tag != y.tag:
			return Difference{At: at, What: fmt.Sprintf("child %d is %s on the left, %s on the right", i+1, describe(x), describe(y)),
				Left: ctx(pa, a), Right: ctx(pb, b)}, true
		case x.kind != kElem:
			if x.text != y.text {
				return Difference{At: at, What: fmt.Sprintf("child %d: left %s, right %s", i+1, describe(x), describe(y)),
					Left: ctx(pa, a), Right: ctx(pb, b)}, true
			}
		default:
			here := append(append([]string{}, path...), segment(a, i))
			if what, differ := diffAttrs(x, y); differ {
				return Difference{At: strings.Join(here, " > "), What: what, Left: context(x), Right: context(y)}, true
			}
			if x.tag == "html" && len(path) == 0 {
				here = nil // a path starts at the body
			}
			if !equalNodes(x.children, y.children) {
				return diffNodes(here, x, y, x.children, y.children)
			}
		}
	}
	return Difference{}, false
}

// diffAttrs says how the attributes of elements x and y differ: the first name whose value
// differs, or that one of them lacks.
func diffAttrs(x, y Node) (string, bool) {
	val := func(n Node, name string) (string, bool) {
		for _, a := range n.attrs {
			if a.Name == name {
				if a.Bool {
					return "present", true
				}
				return strconv.Quote(a.Value), true
			}
		}
		return "none", false
	}
	var names []string
	for _, a := range x.attrs {
		names = append(names, a.Name)
	}
	for _, a := range y.attrs {
		names = append(names, a.Name)
	}
	for _, name := range names {
		l, _ := val(x, name)
		r, _ := val(y, name)
		if l != r {
			return fmt.Sprintf("attribute %s: left %s, right %s", name, l, r), true
		}
	}
	if len(x.attrs) == len(y.attrs) {
		for i := range x.attrs {
			if x.attrs[i].Name != y.attrs[i].Name {
				return "the attributes are in another order", true
			}
		}
	}
	return "", false
}

// segment is the step in a path of element siblings[i]: its tag, then `#id`, or else its
// classes (`li.done`); `:nth-child(k)` (k counting the elements) when a sibling has the same
// step.
func segment(siblings []Node, i int) string {
	step := func(n Node) string {
		s := n.tag
		if id, ok := AttrOf(n, "id"); ok && id != "" {
			return s + "#" + id
		}
		if class, ok := AttrOf(n, "class"); ok {
			for _, c := range strings.Fields(class) {
				s += "." + c
			}
		}
		return s
	}
	mine, same, k, pos := step(siblings[i]), 0, 0, 0
	for j, s := range siblings {
		if s.kind != kElem {
			continue
		}
		k++
		if j == i {
			pos = k
		}
		if step(s) == mine {
			same++
		}
	}
	if same > 1 {
		return mine + ":nth-child(" + strconv.Itoa(pos) + ")"
	}
	return mine
}

// count is "0", "1 child <li>" or "3 children <li>, <li>, text "x"" (at most three named).
func count(ns []Node) string {
	if len(ns) == 0 {
		return "0"
	}
	var ds []string
	for i, n := range ns {
		if i == 3 {
			ds = append(ds, "…")
			break
		}
		ds = append(ds, describe(n))
	}
	word := "children"
	if len(ns) == 1 {
		word = "child"
	}
	return fmt.Sprintf("%d %s %s", len(ns), word, strings.Join(ds, ", "))
}

// describe is a node as a difference names it: `<li>`, text "Milk" (cut), raw markup.
func describe(n Node) string {
	switch n.kind {
	case kElem:
		return "<" + n.tag + ">"
	case kText:
		return "text " + strconv.Quote(cut(n.text, 40))
	}
	return "raw " + strconv.Quote(cut(n.text, 40))
}

// context is n's markup on one line, cut to contextLen characters.
func context(n Node) string {
	return cut(strings.Join(strings.Fields(Render(n)), " "), contextLen)
}

// cut is s with at most max characters, "…" ending one that was cut.
func cut(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max-1]) + "…"
}
