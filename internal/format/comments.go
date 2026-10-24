package format

import (
	"strings"

	"alder/internal/diag"
	"alder/internal/syntax"
)

// Comment attachment (spec §5). Comments are not in the AST: after parsing, one pass over the
// item lists in source order, with a cursor into Module.Comments, gives each comment a place:
//   - leading: on its own line before an item (a declaration, import, statement, field, arm,
//     list element, constructor, attribute, markup child) or before a body (after `=`, `->`,
//     `<-`, `then`, `else`); printed on its own line(s) above it, at its indent;
//   - trailing: on the line where an item ends; printed after it (and its comma), one space
//     before, which forces the group around it vertical;
//   - after: a region's end marker (`-- @admin end`) on its own line stays with the item before
//     it, printed on the next line, so the region keeps its items and nothing else;
//   - dangling: after the last item of a bracketed group (or of the file, or of an element's
//     attributes or children); printed at the end of the group at the items' indent. Comments
//     between a signature and its definition dangle on the signature and stay between them.
//
// A comment the attacher cannot place there (between an operator and its operand, inside a
// pattern, …) leads the smallest item or body around it. To keep the comments in source order,
// the comments already placed inside that item move up with it. Comment lines of markup text
// inside a text child are printed with the text (markupText), not attached.

// attached holds each comment's place, keyed by node: leading and trailing by the item or body,
// dangling by the group (the module for the end of the file, the element for its attributes and
// its children).
type attached struct {
	leading, trailing, dangling map[any][]syntax.Comment
	// after holds the own-line comments printed just after an item (region end markers)
	after map[any][]syntax.Comment
}

// RegionEnd is a region's end marker, generate.RegionEnd (internal/generate/regions.go; not
// imported: the generators import this package, and a test there keeps the two equal).
const RegionEnd = "-- @admin end"

type place uint8

const (
	placeLeading place = iota
	placeTrailing
	placeDangling
	placeAfter
	placeText // a comment line of a markup text child, printed with the text
)

type attacher struct {
	cs     []syntax.Comment
	lines  []string
	i      int // the next comment to place
	kind   []place
	at     []any
	frames []frame
}

// frame is an item or body being walked: an unplaceable comment inside it leads it.
type frame struct {
	node  any
	first int // the first comment that may be placed inside node
}

// attach places every comment of m.
func attach(m *syntax.Module) *attached {
	n := len(m.Comments)
	a := &attacher{cs: m.Comments, lines: m.Lines, kind: make([]place, n), at: make([]any, n)}
	a.frames = []frame{{node: m}}
	// a header above `module`, and a comment on the module line
	for a.i < n && before(a.cs[a.i], m.Span.Start) {
		a.put(placeLeading, m)
	}
	for a.i < n && a.cs[a.i].Line == m.Span.Start.Line {
		a.put(placeTrailing, m)
	}
	var items []any
	for _, im := range m.Imports {
		items = append(items, im)
	}
	for _, d := range m.Decls {
		items = append(items, d)
	}
	a.items(m, m.Span.Start, items, diag.Pos{Line: len(m.Lines) + 1})
	a.flush(diag.Pos{Line: len(m.Lines) + 2})
	out := &attached{map[any][]syntax.Comment{}, map[any][]syntax.Comment{}, map[any][]syntax.Comment{}, map[any][]syntax.Comment{}}
	for i, c := range a.cs {
		switch a.kind[i] {
		case placeLeading:
			out.leading[a.at[i]] = append(out.leading[a.at[i]], c)
		case placeTrailing:
			out.trailing[a.at[i]] = append(out.trailing[a.at[i]], c)
		case placeDangling:
			out.dangling[a.at[i]] = append(out.dangling[a.at[i]], c)
		case placeAfter:
			out.after[a.at[i]] = append(out.after[a.at[i]], c)
		}
	}
	return out
}

// before reports whether c starts before pos.
func before(c syntax.Comment, pos diag.Pos) bool {
	return c.Line < pos.Line || c.Line == pos.Line && c.Col < pos.Col
}

func spanOf(n any) diag.Span {
	switch n := n.(type) {
	case *syntax.DeployField:
		return n.Span
	case *syntax.DeployValue:
		return n.Span
	case interface{ GetSpan() diag.Span }:
		return n.GetSpan()
	}
	panic("format: node without a span")
}

// put places the next comment.
func (a *attacher) put(k place, n any) {
	a.kind[a.i], a.at[a.i] = k, n
	a.i++
}

// more reports whether a comment before pos is left to place.
func (a *attacher) more(pos diag.Pos) bool { return a.i < len(a.cs) && before(a.cs[a.i], pos) }

// flush places the comments before pos as leading the innermost frame, with every comment already
// placed inside it (except those printed with markup text), so that the order is kept.
func (a *attacher) flush(pos diag.Pos) {
	for a.more(pos) {
		f := &a.frames[len(a.frames)-1]
		for j := f.first; j < a.i; j++ {
			if a.kind[j] != placeText {
				a.kind[j], a.at[j] = placeLeading, f.node
			}
		}
		a.put(placeLeading, f.node)
		f.first = a.i
	}
}

// within walks n as a frame: what cannot be placed inside it leads it.
func (a *attacher) within(n any, walk func()) {
	a.frames = append(a.frames, frame{n, a.i})
	walk()
	a.flush(spanOf(n).End)
	a.frames = a.frames[:len(a.frames)-1]
}

// items places the comments of a group of items that starts at start and ends at end: before an
// item, leading it (or trailing the item before, on its last line); after the last, trailing it
// or dangling on the group.
func (a *attacher) items(group any, start diag.Pos, items []any, end diag.Pos) {
	a.flush(start)
	var prev any
	trails := func() bool { return prev != nil && a.cs[a.i].Line == spanOf(prev).End.Line && !a.cs[a.i].Markup }
	// a region's end marker after an item, with no comment leading the next item before it,
	// stays with the item
	led := false
	ends := func() bool { return prev != nil && !led && strings.HasPrefix(a.cs[a.i].Text, RegionEnd) }
	for _, it := range items {
		led = false
		for a.more(spanOf(it).Start) {
			if trails() {
				a.put(placeTrailing, prev)
			} else if ends() {
				a.put(placeAfter, prev)
			} else {
				led = true
				a.put(placeLeading, it)
			}
		}
		a.within(it, func() { a.walk(it) })
		prev = it
	}
	for a.more(end) && !a.cs[a.i].Markup {
		if trails() {
			a.put(placeTrailing, prev)
		} else if ends() {
			a.put(placeAfter, prev)
		} else {
			led = true
			a.put(placeDangling, group)
		}
	}
}

// body places the comments before a body (after `=`, `->`, `then`, …) as leading it, then walks
// it as a frame.
func (a *attacher) body(e syntax.Expr) {
	for a.more(e.GetSpan().Start) {
		a.put(placeLeading, e)
	}
	a.within(e, func() { a.walk(e) })
}

func exprItems(xs []syntax.Expr) []any {
	out := make([]any, len(xs))
	for i, x := range xs {
		out[i] = x
	}
	return out
}

func fieldItems(fs []*syntax.Field) []any {
	out := make([]any, len(fs))
	for i, f := range fs {
		out[i] = f
	}
	return out
}

// defLine is the line where a value's definition (`name params =`) starts: the first line after
// its signature that is neither blank nor a comment.
func (a *attacher) defLine(d *syntax.ValueDecl) int {
	l := d.Sig.GetSpan().End.Line + 1
	for ; l <= len(a.lines); l++ {
		t := strings.TrimSpace(a.lines[l-1])
		if t != "" && !strings.HasPrefix(t, "--") {
			break
		}
	}
	return l
}

// walk visits n's parts in source order.
func (a *attacher) walk(n any) {
	switch n := n.(type) {
	case nil, *syntax.Import:
	// ---- declarations
	case *syntax.ValueDecl:
		if n.Sig != nil {
			a.flush(n.Sig.GetSpan().Start) // inside `x :`, before the type: they lead the declaration
			a.walk(n.Sig)
			// comments between the signature and the definition stay there
			def := diag.Pos{Line: a.defLine(n), Col: 1}
			if n.Body == nil {
				def = n.Span.End
			}
			for a.more(def) {
				if a.cs[a.i].Line == n.Sig.GetSpan().End.Line {
					a.put(placeTrailing, n.Sig)
				} else {
					a.put(placeDangling, n.Sig)
				}
			}
		}
		if n.Body != nil {
			a.body(n.Body)
		}
	case *syntax.TypeDecl:
		switch {
		case n.Record != nil:
			a.walk(n.Record)
		case n.Brand != nil:
			a.walk(n.Brand.Check)
		case n.Union != nil:
			items := make([]any, len(n.Union))
			for i, c := range n.Union {
				items[i] = c
			}
			a.items(n, n.Span.Start, items, n.Span.End)
		}
	case *syntax.Ctor:
		a.walk(n.Arg)
	case *syntax.PageDecl:
		a.walk(n.Params)
	case *syntax.MsgDecl:
		if n.Params != nil {
			a.walk(n.Params)
		}
	case *syntax.FormDecl:
		a.items(n, n.Span.Start, fieldItems(n.Fields), n.Span.End)
	case *syntax.TableDecl:
		a.items(n, n.Span.Start, fieldItems(n.Fields), n.Span.End)
	case *syntax.SessionDecl:
		a.items(n, n.Span.Start, fieldItems(n.Fields), n.Span.End)
	case *syntax.CapDecl:
		a.items(n, n.Span.Start, fieldItems(n.Entries), n.Span.End)
	case *syntax.FakeDecl:
		a.items(n, n.Span.Start, fakeItems(n), n.Span.End)
	case *syntax.DeployDecl:
		a.items(n, n.Span.Start, deployItems(n.Fields), n.Span.End)
	case *syntax.DeployField:
		a.walk(n.Value)
	case *syntax.DeployValue:
		switch {
		case n.Kind == "list":
			items := make([]any, len(n.List))
			for i, v := range n.List {
				items[i] = v
			}
			a.items(n, n.Span.Start, items, n.Span.End)
		case n.Rec != nil:
			a.items(n, n.Span.Start, deployItems(n.Rec), n.Span.End)
		}
	case *syntax.TestDecl:
		a.body(n.Body)
	case *syntax.SiloDecl, *syntax.LoginDecl, *syntax.ChooseDecl:
	case *syntax.Field:
		a.walk(n.Type)
		if n.Default != nil {
			a.walk(n.Default)
		}
	// ---- types
	case *syntax.RecordType:
		a.items(n, n.Span.Start, fieldItems(n.Fields), n.Span.End)
	case *syntax.TypeName:
		for _, x := range n.Args {
			a.walk(x)
		}
	case *syntax.FunType:
		for _, x := range n.Params {
			a.walk(x)
		}
		a.walk(n.Result)
	case *syntax.TypeVar:
	// ---- expressions
	case *syntax.ListLit:
		a.items(n, n.Span.Start, exprItems(n.Elems), n.Span.End)
	case *syntax.RecordLit:
		a.items(n, n.Span.Start, initItems(n.Fields), n.Span.End)
	case *syntax.RecordUpdate:
		a.walk(n.Base)
		a.items(n, n.Base.GetSpan().End, initItems(n.Fields), n.Span.End)
	case *syntax.FieldInit:
		a.walk(n.Value)
	case *syntax.Access:
		a.walk(n.Expr)
	case *syntax.App:
		a.walk(n.Fn)
		for _, x := range n.Args {
			a.walk(x)
		}
	case *syntax.Lambda:
		a.body(n.Body)
	case *syntax.If:
		a.walk(n.Cond)
		a.body(n.Then)
		a.body(n.Else)
	case *syntax.Match:
		a.walk(n.Subject)
		arms := make([]any, len(n.Arms))
		for i, x := range n.Arms {
			arms[i] = x
		}
		a.items(n, n.Subject.GetSpan().End, arms, n.Span.End)
	case *syntax.Arm:
		a.walk(n.Pat)
		a.body(n.Body)
	case *syntax.Block:
		items := make([]any, 0, len(n.Stmts)+1)
		for _, s := range n.Stmts {
			items = append(items, s)
		}
		a.items(n, n.Span.Start, append(items, n.Result), n.Span.End)
	case *syntax.LetStmt:
		if n.Type != nil {
			a.walk(n.Type)
		}
		a.body(n.Expr)
	case *syntax.BinOp:
		a.walk(n.L)
		a.walk(n.R)
	case *syntax.Element:
		a.element(n)
	case *syntax.HtmlAttr:
		a.walk(n.Value)
	case *syntax.HtmlExpr:
		a.walk(n.Expr)
	case *syntax.HtmlFor:
		a.walk(n.List)
		a.body(n.Body)
	case *syntax.HtmlIf:
		a.walk(n.Cond)
		a.body(n.Then)
		a.body(n.Else)
	case *syntax.HtmlMatch:
		a.walk(n.Subject)
		arms := make([]any, len(n.Arms))
		for i, x := range n.Arms {
			arms[i] = x
		}
		a.items(n, n.Subject.GetSpan().End, arms, n.Span.End)
	case *syntax.IntLit, *syntax.FloatLit, *syntax.StrLit, *syntax.BoolLit, *syntax.UnitLit,
		*syntax.Var, *syntax.CtorRef, *syntax.HoleExpr, *syntax.HtmlText:
	default: // patterns hold no item lists: a comment inside one leads the arm
	}
}

// element places an element's comments: in its start tag, among the attributes; between its
// children, where only comment lines of markup text can be, leading the next child or dangling
// on the element. A text child's own comment lines (up to the next child) are printed with it.
func (a *attacher) element(e *syntax.Element) {
	attrs := make([]any, len(e.Attrs))
	for i, x := range e.Attrs {
		attrs[i] = x
	}
	tagEnd := e.Span.End
	if len(e.Children) > 0 {
		tagEnd = e.Children[0].GetSpan().Start
	}
	a.items(e, e.Span.Start, attrs, tagEnd)
	for i, c := range e.Children {
		if _, ok := c.(*syntax.HtmlText); ok {
			end := e.Span.End
			if i+1 < len(e.Children) {
				end = e.Children[i+1].GetSpan().Start
			}
			for a.more(end) && a.cs[a.i].Markup {
				a.put(placeText, c)
			}
			continue
		}
		for a.more(c.GetSpan().Start) && a.cs[a.i].Markup {
			a.put(placeLeading, c)
		}
		a.walk(c)
	}
	for a.more(e.Span.End) && a.cs[a.i].Markup {
		a.put(placeDangling, e)
	}
}

func initItems(fs []*syntax.FieldInit) []any {
	out := make([]any, len(fs))
	for i, f := range fs {
		out[i] = f
	}
	return out
}

func deployItems(fs []*syntax.DeployField) []any {
	out := make([]any, len(fs))
	for i, f := range fs {
		out[i] = f
	}
	return out
}

// ---- printing

// lead prints n's leading comments, each on its own line before n, keeping one blank line where
// the source has any between two of them or between the last and n, and before the first unless
// first is set (n opens its group, or the caller places the blank line before it).
func (p *printer) lead(n any, first bool) Doc {
	cs := p.c.leading[n]
	if len(cs) == 0 {
		return Text("")
	}
	var out []Doc
	for i, c := range cs {
		if i == 0 && !first && p.blankAbove(c.Line) || i > 0 && p.blankBetween(cs[i-1].Line, c.Line) {
			out = append(out, HardLine)
		}
		out = append(out, Text(c.Text), HardLine)
	}
	if last := cs[len(cs)-1].Line; p.blankBetween(last, spanOf(n).Start.Line) {
		out = append(out, HardLine)
	}
	return Concat(out...)
}

// trail prints n's trailing comment after it, then the comments that follow it on lines of
// their own (a region's end marker), keeping one blank line where the source has any.
func (p *printer) trail(n any) Doc {
	var out []Doc
	for _, c := range p.c.trailing[n] {
		out = append(out, Trailing(" "+c.Text))
	}
	for _, c := range p.c.after[n] {
		out = append(out, HardLine)
		if p.blankAbove(c.Line) {
			out = append(out, HardLine)
		}
		out = append(out, Text(c.Text))
	}
	return Concat(out...)
}

// withComments wraps an item's document in its leading and trailing comments, for a list whose
// printer places the blank lines between items (declarations, statements, a fake's items).
func (p *printer) withComments(n any, d Doc) Doc {
	return Concat(p.lead(n, true), d, p.trail(n))
}

// dangle prints the comments at the end of a group, each on a line of its own (a line break
// first), keeping one blank line where the source has any before one, except before the first
// when first is set (the group has no items). keep picks markup or code comments.
func (p *printer) dangle(group any, first bool, keep func(syntax.Comment) bool) Doc {
	var out []Doc
	for _, c := range p.c.dangling[group] {
		if keep != nil && !keep(c) {
			continue
		}
		out = append(out, HardLine)
		if !(first && len(out) == 1) && p.blankAbove(c.Line) {
			out = append(out, HardLine)
		}
		out = append(out, Text(c.Text))
	}
	return Concat(out...)
}

func (p *printer) hasLead(n any) bool  { return len(p.c.leading[n]) > 0 }
func (p *printer) hasTrail(n any) bool { return len(p.c.trailing[n]) > 0 }

// startLine is the line where n starts with its leading comments.
func (p *printer) startLine(n any) int {
	if cs := p.c.leading[n]; len(cs) > 0 {
		return min(cs[0].Line, spanOf(n).Start.Line)
	}
	return spanOf(n).Start.Line
}

// blankAbove reports whether the line above source line l is blank.
func (p *printer) blankAbove(l int) bool {
	return l >= 2 && l-1 <= len(p.lines) && strings.TrimSpace(p.lines[l-2]) == ""
}

// commaItems prints the inside of a bracketed group: the items separated by `,` and a line,
// each with its comments, a trailing comma when broken (if trailingComma), and the group's
// dangling comments last.
func (p *printer) commaItems(group any, nodes []any, items []Doc, trailingComma bool) Doc {
	var out []Doc
	for i, d := range items {
		if i > 0 {
			out = append(out, Line)
		}
		var n any
		if nodes != nil {
			n = nodes[i]
		}
		out = append(out, p.lead(n, i == 0), d)
		switch {
		case i < len(items)-1:
			out = append(out, Text(","))
		case trailingComma:
			out = append(out, IfBreak(Text(","), Text("")))
		}
		out = append(out, p.trail(n))
	}
	out = append(out, p.dangle(group, len(items) == 0, nil))
	return Concat(out...)
}
