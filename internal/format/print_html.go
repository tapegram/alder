package format

import (
	"strings"

	"alder/internal/syntax"
)

// Markup layout. An element with two or more element children (counting the elements directly
// inside its `{for}`, `{if}` and `{match}` children) is always vertical, one child per line, as is
// one with a blank line between children; any other element prints on one line when it fits,
// otherwise its children go one per line, indented. A break is placed only where the JSX whitespace rule
// (book chapter 5) gives the same text either way:
//   - at whitespace holding a newline in the source (dropped or turned into one space whether it
//     is printed as a newline or not);
//   - between two non-text children with nothing between them;
//   - after the start tag and before the end tag, unless text touches it with a space.
//
// Text touching a sibling node, and same-line spaces between nodes, glue the nodes around them
// into one run that is never split. The safety check (Dump equality) is the oracle.

// element prints an element: its start tag (attributes on its line if they fit, else one per
// line with `>` on the last), its children, its end tag. A `<script>` or `<style>` body is
// printed verbatim.
func (p *printer) element(e *syntax.Element) Doc {
	name := qual(e.Module, e.Tag)
	if len(e.Children) == 0 {
		if strings.HasSuffix(p.spanText(e.Span), "/>") {
			return p.startTag(e, " />")
		}
		return Concat(p.startTag(e, ">"), Text("</"+name+">"))
	}
	if t, ok := e.Children[0].(*syntax.HtmlText); ok && t.Raw {
		return Concat(p.startTag(e, ">"), Text(t.Text), Text("</"+name+">"))
	}
	pieces, forced := p.markupChildren(e.Children)
	render := func(pc piece) Doc {
		switch {
		case pc.d != nil:
			return pc.d
		case pc.glue:
			return Text("")
		case forced && pc.kind == breakBlank:
			return Concat(HardLine, HardLine)
		case forced:
			return HardLine
		case pc.kind == breakSpace:
			return Line
		}
		return SoftLine
	}
	// pieces alternate boundary, content, boundary, …, boundary
	var inner []Doc
	for _, pc := range pieces[:len(pieces)-1] {
		inner = append(inner, render(pc))
	}
	return Group(Concat(p.startTag(e, ">"), Nest(Concat(inner...)), render(pieces[len(pieces)-1]), Text("</"+name+">")))
}

// startTag prints `<name attr…` and then end (`>` or ` />`): on one line if it fits, else one
// attribute per line, indented, end on the last attribute's line.
func (p *printer) startTag(e *syntax.Element, end string) Doc {
	var attrs []Doc
	for _, a := range e.Attrs {
		attrs = append(attrs, Line, p.htmlAttr(a))
	}
	return Group(Concat(Text("<"+qual(e.Module, e.Tag)), Nest(Concat(attrs...)), Text(end)))
}

// htmlAttr prints `name`, `name="text"` (the string as written) or `name={expr}`.
func (p *printer) htmlAttr(a *syntax.HtmlAttr) Doc {
	if a.Value == nil {
		return Text(a.Name)
	}
	if s, ok := a.Value.(*syntax.StrLit); ok && !p.bracedAttr(a) {
		return Text(a.Name + "=" + p.spanText(s.Span))
	}
	return Concat(Text(a.Name+"={"), p.inBrackets(a.Value), Text("}"))
}

// bracedAttr reports whether the attribute's value is written in braces (`a={"x"}`): the first
// byte after the name and `=` (and any blanks around it) is `{`.
func (p *printer) bracedAttr(a *syntax.HtmlAttr) bool {
	for _, c := range p.src[p.offset(a.Span.Start)+len(a.Name):] {
		switch c {
		case ' ', '\t', '\n', '=':
			continue
		}
		return c == '{'
	}
	return false
}

type breakKind int

const (
	breakSoft  breakKind = iota // nothing when on one line
	breakSpace                  // a space when on one line: whitespace inside text
	breakBlank                  // a blank line, which keeps the element vertical
)

// piece is a child's printed content (d set), or a boundary: glue (nothing, never a line break)
// or a break of some kind.
type piece struct {
	d    Doc
	glue bool
	kind breakKind
}

// childEdges describes a child at the boundaries on either side of it. All fields are false
// for a node other than text.
type childEdges struct {
	text               bool
	leadNL, trailNL    bool // the text starts (ends) with whitespace holding a newline
	leadWS, trailWS    bool // the text starts (ends) with same-line whitespace
	leadBlank          bool // that leading (trailing) whitespace holds a blank line
	trailBlank         bool
	leadDash           bool // the text starts with `--` (it must not start a line: it would be a comment)
	startLine, endLine int  // source lines of a node other than text
}

// markupChildren lays out an element's children as boundary, child pieces…, boundary, …,
// boundary; forced reports that the element must be vertical (two or more element children, a
// blank line, a comment line in text).
func (p *printer) markupChildren(children []syntax.Expr) ([]piece, bool) {
	forced := elementChildren(children) >= 2
	edges := make([]childEdges, len(children))
	contents := make([][]piece, len(children))
	for i, c := range children {
		if t, ok := c.(*syntax.HtmlText); ok {
			var f bool
			edges[i], contents[i], f = p.markupText(t)
			forced = forced || f
			continue
		}
		edges[i].startLine, edges[i].endLine = c.GetSpan().Start.Line, c.GetSpan().End.Line
		contents[i] = []piece{{d: p.markupChild(c)}}
	}
	start, end := childEdges{}, childEdges{}
	boundary := func(l, r *childEdges, outer bool) piece {
		switch {
		case l.trailNL || r.leadNL:
			blank := !outer && (l.trailBlank || r.leadBlank)
			forced = forced || blank
			if blank {
				return piece{kind: breakBlank}
			}
			return piece{kind: breakSoft}
		case l.trailWS || r.leadWS || r.leadDash:
			return piece{glue: true}
		case outer:
			return piece{kind: breakSoft}
		case !l.text && !r.text:
			if p.blankBetween(l.endLine, r.startLine) {
				forced = true
				return piece{kind: breakBlank}
			}
			return piece{kind: breakSoft}
		}
		return piece{glue: true} // text touching a node
	}
	var out []piece
	out = append(out, boundary(&start, &edges[0], true))
	for i := range children {
		if i > 0 {
			out = append(out, boundary(&edges[i-1], &edges[i], false))
		}
		out = append(out, contents[i]...)
	}
	out = append(out, boundary(&edges[len(children)-1], &end, true))
	// a run of glued children is one unit: nothing inside it breaks unless it must
	for i := 1; i < len(out)-1; i++ {
		if out[i].d != nil && (out[i-1].glue || out[i+1].glue) {
			out[i].d = Flat(out[i].d)
		}
	}
	return out, forced
}

// markupText splits a text child at its whitespace runs that hold a newline: each is a break
// that prints as a space when the element is on one line, as the parser reads it. Text that
// starts or ends with such whitespace drops it (the boundary breaks there). forced reports a
// blank line or a comment line inside the text.
func (p *printer) markupText(t *syntax.HtmlText) (childEdges, []piece, bool) {
	start := p.offset(t.Span.Start)
	raw := string(p.src[start:syntax.TextEnd(p.src, start)]) // where the lexer ends the text
	body := strings.TrimLeft(raw, " \t\n")
	lead := raw[:len(raw)-len(body)]
	body = strings.TrimRight(body, " \t\n")
	trail := raw[len(lead)+len(body):]
	ed := childEdges{text: true}
	ed.leadNL, ed.trailNL = strings.Contains(lead, "\n"), strings.Contains(trail, "\n")
	ed.leadWS, ed.trailWS = lead != "" && !ed.leadNL, trail != "" && !ed.trailNL
	ed.leadBlank, ed.trailBlank = strings.Count(lead, "\n") >= 2, strings.Count(trail, "\n") >= 2
	ed.leadDash = !ed.leadNL && strings.HasPrefix(body, "--")
	if body == "" { // same-line whitespace only, between two nodes
		return ed, []piece{{d: Text(raw)}}, false
	}
	var pieces []piece
	forced := false
	blank := false
	for i, ln := range strings.Split(body, "\n") {
		ln = strings.TrimRight(strings.TrimLeft(ln, " \t"), " \t")
		if i > 0 {
			if ln == "" {
				blank = true
				continue
			}
			kind := breakSpace
			if blank {
				kind, forced = breakBlank, true
			}
			forced = forced || strings.HasPrefix(ln, "--") // a comment line stays on its own line
			pieces = append(pieces, piece{kind: kind})
			blank = false
		} else {
			forced = forced || ed.leadNL && strings.HasPrefix(ln, "--")
		}
		pieces = append(pieces, piece{d: Text(ln)})
	}
	// the first line keeps its leading and the last its trailing same-line spaces, which matter
	if ed.leadWS {
		pieces[0].d = Concat(Text(lead), pieces[0].d)
	}
	if ed.trailWS {
		last := len(pieces) - 1
		pieces[last].d = Concat(pieces[last].d, Text(trail))
	}
	return ed, pieces, forced
}

// markupChild prints a child other than text.
func (p *printer) markupChild(c syntax.Expr) Doc {
	switch c := c.(type) {
	case *syntax.Element:
		return p.element(c)
	case *syntax.HtmlExpr:
		return Concat(Text("{"), p.inBrackets(c.Expr), Text("}"))
	case *syntax.HtmlFor:
		// `{for x in xs -> <li>…</li>}`, or the body on the next line, indented
		return Group(Concat(Text("{for "+c.Var+" in "), p.inBrackets(c.List), Text(" ->"),
			Nest(Concat(Line, p.markupBody(c.Body))), Text("}")))
	case *syntax.HtmlIf:
		// `{if c then a else b}`, or `{if c then` / a / `else` / b`}`, each branch indented
		return Group(Concat(Text("{if "), p.inBrackets(c.Cond), Text(" then"),
			Nest(Concat(Line, p.markupBody(c.Then))), Line, Text("else"),
			Nest(Concat(Line, p.markupBody(c.Else))), Text("}")))
	case *syntax.HtmlMatch:
		return p.markupMatch(c)
	}
	panic("format: unknown markup child")
}

// markupBody prints the body of a markup `for`, `if` branch or `match` arm: an element, or an
// expression without braces.
func (p *printer) markupBody(b syntax.Expr) Doc {
	if x, ok := b.(*syntax.HtmlExpr); ok {
		return p.inBrackets(x.Expr)
	}
	return p.markupChild(b)
}

// markupMatch prints `{match subject {`, each arm on its own line with a trailing comma, `}}`.
func (p *printer) markupMatch(e *syntax.HtmlMatch) Doc {
	saved := p.noBrace
	p.noBrace = true
	subj := p.expr(e.Subject)
	p.noBrace = false
	defer func() { p.noBrace = saved }()
	if len(e.Arms) == 0 {
		return Concat(Text("{match "), subj, Text(" { }}"))
	}
	arms := make([]Doc, len(e.Arms))
	for i, a := range e.Arms {
		var body Doc
		if x, ok := a.Body.(*syntax.HtmlExpr); ok {
			body = p.after(x.Expr)
		} else {
			body = Group(Nest(Concat(Line, p.markupBody(a.Body))))
		}
		arms[i] = Concat(p.pattern(a.Pat), Text(" ->"), body, Text(","))
	}
	return Concat(Text("{match "), subj, Text(" {"), Nest(Concat(HardLine, join(arms, HardLine))), HardLine, Text("}}"))
}

// elementChildren counts the elements among an element's children, and those directly inside
// its `{for}`, `{if}` and `{match}` children (a body, a branch, an arm).
func elementChildren(children []syntax.Expr) int {
	n := 0
	count := func(b syntax.Expr) {
		if _, ok := b.(*syntax.Element); ok {
			n++
		}
	}
	for _, c := range children {
		switch c := c.(type) {
		case *syntax.Element:
			n++
		case *syntax.HtmlFor:
			count(c.Body)
		case *syntax.HtmlIf:
			count(c.Then)
			count(c.Else)
		case *syntax.HtmlMatch:
			for _, a := range c.Arms {
				count(a.Body)
			}
		}
	}
	return n
}
