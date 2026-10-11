package html

import (
	"fmt"
	"html"
	"strings"

	"alder/internal/selector"
)

// Parse parses markup that Render produced (the parser is for the renderer's own output, which
// is always well-formed: lower-case tags, quoted attributes, the void elements written `<br />`,
// raw text inside <script> and <style>) back into a Node: elements with their attributes in
// order (a bare attribute is a Bool one), text with its entities decoded, and the contents of
// <script>/<style>, comments and a doctype as Raw. Render(Parse(Render(x))) is Render(x).
//
// It is lenient where the renderer's output never needs it, so markup a user wrote with
// Html.unsafe still parses: tag and attribute names are lower-cased, single-quoted and unquoted
// attribute values are read, `<br>` without the slash is void, an end tag closes the open element
// of its name (and those inside it), an end tag matching no open element is dropped, and the
// elements still open at the end are closed. It fails only on a `<` that starts a tag it cannot
// read to its `>`.
//
// The result is the single top-level node, or a fragment (Nodes) of them; a page with nothing
// in it is None.
func Parse(s string) (Node, error) {
	p := &parser{in: s}
	root := &Node{kind: kFrag}
	stack := []*Node{root}
	top := func() *Node { return stack[len(stack)-1] }
	appendTo := func(n Node) {
		t := top()
		if n.kind == kText {
			if k := len(t.children); k > 0 && t.children[k-1].kind == kText {
				t.children[k-1].text += n.text
				return
			}
		}
		t.children = append(t.children, n)
	}
	for p.pos < len(p.in) {
		lt := strings.IndexByte(p.in[p.pos:], '<')
		if lt < 0 {
			appendTo(Text(html.UnescapeString(p.in[p.pos:])))
			break
		}
		if lt > 0 {
			appendTo(Text(html.UnescapeString(p.in[p.pos : p.pos+lt])))
			p.pos += lt
		}
		rest := p.in[p.pos:]
		switch {
		case strings.HasPrefix(rest, "<!--"):
			end := strings.Index(rest[4:], "-->")
			if end < 0 {
				return Node{}, p.errorf("an unterminated comment")
			}
			appendTo(Raw(rest[:4+end+3]))
			p.pos += 4 + end + 3
		case strings.HasPrefix(rest, "<!"):
			end := strings.IndexByte(rest, '>')
			if end < 0 {
				return Node{}, p.errorf("an unterminated declaration")
			}
			appendTo(Raw(rest[:end+1]))
			p.pos += end + 1
		case strings.HasPrefix(rest, "</"):
			end := strings.IndexByte(rest, '>')
			if end < 0 {
				return Node{}, p.errorf("an unterminated end tag")
			}
			name := strings.ToLower(strings.TrimSpace(rest[2:end]))
			p.pos += end + 1
			for i := len(stack) - 1; i > 0; i-- {
				if stack[i].tag == name {
					for len(stack) > i {
						closeTop(&stack)
					}
					break
				}
			}
		case len(rest) > 1 && isNameStart(rest[1]):
			el, selfClosed, err := p.startTag()
			if err != nil {
				return Node{}, err
			}
			if void[el.tag] || selfClosed {
				appendTo(el)
				continue
			}
			if el.tag == "script" || el.tag == "style" {
				closeTag := "</" + el.tag
				end := indexFold(p.in[p.pos:], closeTag)
				if end < 0 {
					end = len(p.in) - p.pos
				}
				if end > 0 {
					el.children = []Node{Raw(p.in[p.pos : p.pos+end])}
				}
				p.pos += end
				if gt := strings.IndexByte(p.in[p.pos:], '>'); gt >= 0 {
					p.pos += gt + 1
				}
				appendTo(el)
				continue
			}
			t := top()
			t.children = append(t.children, el)
			stack = append(stack, &t.children[len(t.children)-1])
		default:
			// a '<' that starts no tag is text
			appendTo(Text("<"))
			p.pos++
		}
	}
	for len(stack) > 1 {
		closeTop(&stack)
	}
	switch len(root.children) {
	case 0:
		return None(), nil
	case 1:
		return root.children[0], nil
	}
	return *root, nil
}

// closeTop pops the innermost open element. Children were appended through the pointer, so
// nothing needs copying back: the parent's slice element is the node.
func closeTop(stack *[]*Node) { *stack = (*stack)[:len(*stack)-1] }

type parser struct {
	in  string
	pos int
}

func (p *parser) errorf(format string, args ...any) error {
	line := 1 + strings.Count(p.in[:p.pos], "\n")
	return fmt.Errorf("html: line %d: %s", line, fmt.Sprintf(format, args...))
}

// startTag reads `<name attr="v" …>` or `… />` at p.pos.
func (p *parser) startTag() (Node, bool, error) {
	start := p.pos
	p.pos++
	n := nameLen(p.in[p.pos:])
	el := Node{kind: kElem, tag: strings.ToLower(p.in[p.pos : p.pos+n])}
	p.pos += n
	for {
		p.skipSpace()
		if p.pos >= len(p.in) {
			p.pos = start
			return Node{}, false, p.errorf("the start tag <%s is not closed", el.tag)
		}
		switch c := p.in[p.pos]; {
		case c == '>':
			p.pos++
			return el, false, nil
		case c == '/' && strings.HasPrefix(p.in[p.pos:], "/>"):
			p.pos += 2
			return el, true, nil
		case c == '/':
			p.pos++
		default:
			k := attrNameLen(p.in[p.pos:])
			if k == 0 {
				p.pos = start
				return Node{}, false, p.errorf("cannot read the attributes of <%s", el.tag)
			}
			name := strings.ToLower(p.in[p.pos : p.pos+k])
			p.pos += k
			p.skipSpace()
			if p.pos >= len(p.in) || p.in[p.pos] != '=' {
				el.attrs = append(el.attrs, Attr{Name: name, Bool: true})
				continue
			}
			p.pos++
			p.skipSpace()
			var v string
			if p.pos < len(p.in) && (p.in[p.pos] == '"' || p.in[p.pos] == '\'') {
				q := p.in[p.pos]
				end := strings.IndexByte(p.in[p.pos+1:], q)
				if end < 0 {
					p.pos = start
					return Node{}, false, p.errorf("an attribute value of <%s is not closed", el.tag)
				}
				v = p.in[p.pos+1 : p.pos+1+end]
				p.pos += end + 2
			} else {
				e := p.pos
				for e < len(p.in) && !isSpace(p.in[e]) && p.in[e] != '>' {
					e++
				}
				v = p.in[p.pos:e]
				p.pos = e
			}
			el.attrs = append(el.attrs, Attr{Name: name, Value: html.UnescapeString(v)})
		}
	}
}

func (p *parser) skipSpace() {
	for p.pos < len(p.in) && isSpace(p.in[p.pos]) {
		p.pos++
	}
}

func isSpace(c byte) bool { return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' }

func isNameStart(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }

func nameLen(s string) int {
	i := 0
	for i < len(s) && !isSpace(s[i]) && s[i] != '>' && s[i] != '/' {
		i++
	}
	return i
}

func attrNameLen(s string) int {
	i := 0
	for i < len(s) && !isSpace(s[i]) && s[i] != '>' && s[i] != '=' && s[i] != '/' && s[i] != '"' && s[i] != '\'' {
		i++
	}
	return i
}

// indexFold is strings.Index ignoring ASCII case.
func indexFold(s, sub string) int {
	return strings.Index(strings.ToLower(s), strings.ToLower(sub))
}

// Equal reports whether n and m are the same tree: the same elements with the same attributes
// (in order) and the same text, comparing the trees as rendered markup would show them —
// fragments are flattened, adjacent text is joined and empty text dropped, so a view built from
// pieces equals the page parsed from its markup. Deferred form controls are compared as they
// render without a form state.
func (n Node) Equal(m Node) bool {
	return equalNodes(normalize([]Node{n}), normalize([]Node{m}))
}

func equalNodes(a, b []Node) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		x, y := a[i], b[i]
		if x.kind != y.kind || x.tag != y.tag || x.text != y.text || len(x.attrs) != len(y.attrs) {
			return false
		}
		for j := range x.attrs {
			if x.attrs[j].Name != y.attrs[j].Name || x.attrs[j].Bool != y.attrs[j].Bool ||
				(!x.attrs[j].Bool && x.attrs[j].Value != y.attrs[j].Value) {
				return false
			}
		}
		if !equalNodes(x.children, y.children) {
			return false
		}
	}
	return true
}

// normalize is ns as plain elements, text and raw nodes: fragments flattened, controls and
// message attributes resolved (rendered and parsed back), omitted attributes dropped, adjacent
// text joined, empty text and zero nodes dropped.
func normalize(ns []Node) []Node {
	var out []Node
	var add func(n Node)
	add = func(n Node) {
		switch n.kind {
		case kNone:
		case kFrag:
			for _, c := range n.children {
				add(c)
			}
		case kCtrl:
			p, err := Parse(Render(n))
			if err == nil {
				add(p)
			}
		case kText:
			if n.text == "" {
				return
			}
			if k := len(out); k > 0 && out[k-1].kind == kText {
				out[k-1].text += n.text
				return
			}
			out = append(out, n)
		case kRaw:
			out = append(out, n)
		case kElem:
			if n.tag == "button" && clickMsg(n.attrs) != nil {
				if p, err := Parse(Render(n)); err == nil {
					add(p)
				}
				return
			}
			e := Node{kind: kElem, tag: n.tag}
			for _, a := range n.attrs {
				if a.Omit {
					continue
				}
				v := a.value(nil)
				if a.Bool {
					v = ""
				} else if a.Name == "href" {
					if p, err := Parse(Render(El("a", []Attr{A("href", v)}))); err == nil {
						v, _ = AttrOf(p, "href") // the renderer's "#" for an unsafe link
					}
				}
				e.attrs = append(e.attrs, Attr{Name: a.Name, Value: v, Bool: a.Bool})
			}
			e.children = normalize(n.children)
			out = append(out, e)
		}
	}
	for _, n := range ns {
		add(n)
	}
	return out
}

// Selector is a parsed selector of the subset (package alder/internal/selector, which generated
// code cannot import: it is re-exported here).
type Selector = selector.Selector

// ParseSelector parses s as a selector of the subset: a tag, `#id`, `.class`, `[attr]`,
// `[attr=value]`, compound, descendant and child. Its error names the offending token.
func ParseSelector(s string) (Selector, error) { return selector.Parse(s) }

// MustParseSelector is ParseSelector for a selector literal the checker accepted; it panics on
// an error.
func MustParseSelector(s string) Selector { return selector.MustParse(s) }

// Select is the elements of n (n itself included) that sel matches, in document order.
// Fragments and controls are looked through as they render.
func Select(n Node, sel selector.Selector) []Node {
	if len(sel.Steps) == 0 {
		return nil
	}
	var out []Node
	var path []*Node
	var walk func(ns []Node)
	walk = func(ns []Node) {
		for i := range ns {
			c := &ns[i]
			if c.kind != kElem {
				continue
			}
			path = append(path, c)
			if matches(sel.Steps, path) {
				out = append(out, *c)
			}
			walk(c.children)
			path = path[:len(path)-1]
		}
	}
	walk(normalize([]Node{n}))
	return out
}

// matches reports whether the last element of path matches the last of steps, and its
// ancestors (path's earlier elements) the steps before, through their combinators.
func matches(steps []selector.Step, path []*Node) bool {
	last := len(steps) - 1
	if len(path) == 0 || !matchStep(steps[last], path[len(path)-1]) {
		return false
	}
	if last == 0 {
		return true
	}
	anc := path[:len(path)-1]
	if steps[last].Comb == selector.Child {
		return matches(steps[:last], anc)
	}
	for i := len(anc); i > 0; i-- {
		if matches(steps[:last], anc[:i]) {
			return true
		}
	}
	return false
}

func matchStep(s selector.Step, n *Node) bool {
	if s.Tag != "" && s.Tag != n.tag {
		return false
	}
	if s.ID != "" {
		if v, ok := AttrOf(*n, "id"); !ok || v != s.ID {
			return false
		}
	}
	if len(s.Classes) > 0 {
		v, _ := AttrOf(*n, "class")
		have := strings.Fields(v)
		for _, c := range s.Classes {
			found := false
			for _, h := range have {
				if h == c {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
	}
	for _, a := range s.Attrs {
		v, ok := AttrOf(*n, a.Name)
		if !ok || a.HasValue && v != a.Value {
			return false
		}
	}
	return true
}

// TextContent is the text of n and everything inside it, whitespace collapsed: runs of spaces,
// tabs and newlines are one space, and none at either end. The contents of <script> and
// <style>, comments and raw markup are not text.
func TextContent(n Node) string {
	var b strings.Builder
	var walk func(ns []Node)
	walk = func(ns []Node) {
		for _, c := range ns {
			switch c.kind {
			case kText:
				b.WriteString(c.text)
			case kElem:
				if c.tag != "script" && c.tag != "style" {
					walk(c.children)
				}
			}
		}
	}
	walk(normalize([]Node{n}))
	return strings.Join(strings.Fields(b.String()), " ")
}

// AttrOf is the value of n's attribute name ("" for a bare one such as `checked`) and whether n
// is an element that has it.
func AttrOf(n Node, name string) (string, bool) {
	if n.kind != kElem {
		return "", false
	}
	for _, a := range n.attrs {
		if a.Name == name && !a.Omit {
			if a.Bool {
				return "", true
			}
			return a.value(nil), true
		}
	}
	return "", false
}

// Tag is n's element name, "" for anything but an element.
func (n Node) Tag() string {
	if n.kind != kElem {
		return ""
	}
	return n.tag
}

// ReplaceElement is doc with its first element named tag (in document order) replaced by with
// (whose own fragments are flattened in place), and whether there was one.
func ReplaceElement(doc Node, tag string, with Node) (Node, bool) {
	done := false
	var walk func(n Node) Node
	walk = func(n Node) Node {
		if done {
			return n
		}
		if n.kind == kElem && n.tag == tag {
			done = true
			return with
		}
		if n.kind == kElem || n.kind == kFrag {
			cs := make([]Node, len(n.children))
			for i, c := range n.children {
				cs[i] = walk(c)
			}
			n.children = cs
		}
		return n
	}
	out := walk(doc)
	return out, done
}

// AppendTo is doc with nodes appended to the children of its first element named tag, and
// whether there was one.
func AppendTo(doc Node, tag string, nodes ...Node) (Node, bool) {
	el := Select(doc, selector.Selector{Steps: []selector.Step{{Tag: tag}}})
	if len(el) == 0 {
		return doc, false
	}
	e := el[0]
	e.children = append(append([]Node{}, e.children...), nodes...)
	return ReplaceElement(doc, tag, e)
}

// Children is the child nodes of an element or fragment n (nil for anything else).
func (n Node) Children() []Node {
	if n.kind != kElem && n.kind != kFrag {
		return nil
	}
	return append([]Node{}, n.children...)
}
