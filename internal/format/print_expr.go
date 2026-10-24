package format

import (
	"strings"

	"alder/internal/syntax"
)

// expr prints an expression where any expression may stand: a body, a field's value, a list
// element, a branch. Parentheses inside it are re-derived from precedence (prec.go): the source's
// are not kept.
func (p *printer) expr(e syntax.Expr) Doc {
	switch e := e.(type) {
	case *syntax.IntLit, *syntax.FloatLit, *syntax.StrLit, *syntax.HoleExpr:
		return Text(p.spanText(e.GetSpan())) // verbatim, interpolations and all
	case *syntax.Element:
		return p.element(e)
	case *syntax.BoolLit:
		if e.Value {
			return Text("True")
		}
		return Text("False")
	case *syntax.UnitLit:
		return Text("()")
	case *syntax.Var:
		return Text(qual(e.Module, e.Name))
	case *syntax.CtorRef:
		return Text(qual(e.Module, e.Name))
	case *syntax.ListLit:
		return p.list(e)
	case *syntax.RecordLit:
		return p.recordLit(e)
	case *syntax.RecordUpdate:
		return p.recordUpdate(e)
	case *syntax.Access:
		base := e.Expr
		paren := p.level(base) < levelPostfix
		switch base.(type) {
		case *syntax.CtorRef, *syntax.IntLit, *syntax.FloatLit: // `M.x` would read as a qualified name, `1.x` as a number
			paren = true
		}
		return Concat(p.operand(base, paren), Text("."+e.Field))
	case *syntax.App:
		return p.app(e)
	case *syntax.Lambda:
		return Concat(Text(lambdaHead(e)), p.after(e.Body))
	case *syntax.If:
		return p.ifChain(e)
	case *syntax.Match:
		return p.match(e)
	case *syntax.Block:
		return p.block(e)
	case *syntax.BinOp:
		return p.binOp(e)
	}
	panic("format: unknown expression")
}

func qual(module, name string) string {
	if module == "" {
		return name
	}
	return module + "." + name
}

// inBrackets prints e inside brackets, where application arguments may start with `{` again.
func (p *printer) inBrackets(e syntax.Expr) Doc {
	saved := p.noBrace
	p.noBrace = false
	defer func() { p.noBrace = saved }()
	return p.expr(e)
}

// operand prints e, in parentheses when paren is set.
func (p *printer) operand(e syntax.Expr, paren bool) Doc {
	if paren {
		return Concat(Text("("), p.inBrackets(e), Text(")"))
	}
	return p.expr(e)
}

// opensBody reports whether e, as a body after `=`, `->` or `<-`, stays on that line and opens
// there K&R style: a match, a block with statements, a record, a list, a lambda, or an
// application whose last argument is a lambda, record, list or block (it breaks inside that).
func opensBody(e syntax.Expr) bool {
	switch e := e.(type) {
	case *syntax.Match, *syntax.Block, *syntax.RecordLit, *syntax.RecordUpdate, *syntax.ListLit, *syntax.Lambda:
		return true
	case *syntax.App:
		switch e.Args[len(e.Args)-1].(type) {
		case *syntax.Lambda, *syntax.Block, *syntax.RecordLit, *syntax.RecordUpdate, *syntax.ListLit:
			return true
		}
	}
	return false
}

// after prints a body that follows `=`, `->` or `<-` (which the caller has printed, with no
// space after it): on the same line when it opens there (opensBody) or fits, else on the next
// line, indented.
func (p *printer) after(e syntax.Expr) Doc {
	if p.hasLead(e) { // the comments go on lines of their own, the body after them
		return Nest(Concat(HardLine, p.lead(e, true), p.expr(e)))
	}
	if opensBody(e) {
		return Concat(Text(" "), p.expr(e))
	}
	return Group(Nest(Concat(Line, p.expr(e))))
}

func lambdaHead(l *syntax.Lambda) string {
	var b strings.Builder
	b.WriteString(`\`)
	for i, x := range l.Params {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(x.Name)
	}
	b.WriteString(" ->")
	return b.String()
}

// ---- application

// app prints `f a b`. Application cannot span lines, so the arguments stay on the function's
// line; a line that is too long breaks inside an argument ("hugging"): the last one when it is a
// lambda, record, list, block or parenthesised expression, any other when it is a lambda,
// record, list or block (`List.map (\row -> {` … `}) rows`). Other arguments never break, and a
// line with nothing to break inside stays long.
func (p *printer) app(e *syntax.App) Doc {
	doc := []Doc{Flat(p.operand(e.Fn, p.level(e.Fn) < levelPostfix))}
	last := len(e.Args) - 1
	for i, a := range e.Args {
		doc = append(doc, Text(" "))
		switch {
		case i == last && p.huggable(a), i < last && opensArg(a):
			doc = append(doc, p.hug(a))
		default:
			doc = append(doc, Flat(p.arg(a)))
		}
	}
	return Concat(doc...)
}

// opensArg reports whether an argument other than the last may break inside: a lambda, a record,
// a list or a block.
func opensArg(e syntax.Expr) bool {
	switch e.(type) {
	case *syntax.Lambda, *syntax.RecordLit, *syntax.RecordUpdate, *syntax.ListLit, *syntax.Block:
		return true
	}
	return false
}

// argNeedsParens reports whether e needs parentheses as an application argument.
func (p *printer) argNeedsParens(e syntax.Expr) bool {
	return p.level(e) < levelPostfix || p.noBrace && startsWithBrace(e)
}

// startsWithBrace reports whether e is printed starting with `{`: in a match subject, an
// argument that does is read as the start of the arms.
func startsWithBrace(e syntax.Expr) bool {
	switch e := e.(type) {
	case *syntax.RecordLit, *syntax.RecordUpdate, *syntax.Block:
		return true
	case *syntax.Access:
		return startsWithBrace(e.Expr)
	}
	return false
}

func (p *printer) arg(e syntax.Expr) Doc { return p.operand(e, p.argNeedsParens(e)) }

func (p *printer) huggable(e syntax.Expr) bool {
	if p.argNeedsParens(e) {
		return true
	}
	switch e.(type) {
	case *syntax.RecordLit, *syntax.RecordUpdate, *syntax.ListLit, *syntax.Block:
		return true
	}
	return false
}

// hug prints an argument so that a long line breaks inside it.
func (p *printer) hug(e syntax.Expr) Doc {
	if !p.argNeedsParens(e) {
		return p.expr(e) // a record, list or block breaks by its own rules
	}
	saved := p.noBrace
	p.noBrace = false
	defer func() { p.noBrace = saved }()
	switch x := e.(type) {
	case *syntax.Lambda:
		// `(\x ->` … `)`, or `(\x -> {` … `})` when the body opens on the arrow's line
		if opensBody(x.Body) && !p.hasLead(x.Body) {
			return Concat(Text("("+lambdaHead(x)+" "), p.expr(x.Body), Text(")"))
		}
		return Group(Concat(Text("("+lambdaHead(x)), Nest(Concat(Line, p.lead(x.Body, true), p.expr(x.Body))), SoftLine, Text(")")))
	default:
		if opensBody(x) {
			return Concat(Text("("), p.expr(x), Text(")"))
		}
		return Group(Concat(Text("("), Nest(Concat(SoftLine, p.expr(x))), SoftLine, Text(")")))
	}
}

// ---- binary operators

// binOp prints a chain of operators of one level (`a + b - c`, `x |> f |> g`): on one line if it
// fits, else one operator per line, operator first, indented. Two or more pipes are always
// one per line. A chain with an operand that always spans lines keeps its operators inline.
func (p *printer) binOp(e *syntax.BinOp) Doc {
	lv, as := prec(e.Op)
	sameLevel := func(x syntax.Expr) (*syntax.BinOp, bool) {
		b, ok := x.(*syntax.BinOp)
		if !ok {
			return nil, false
		}
		l, _ := prec(b.Op)
		return b, l == lv
	}
	// ops[i-1] stands between operands[i-1] and operands[i]; left[i] is set for an operand on its
	// operator's left
	var operands []syntax.Expr
	var ops []string
	var left []bool
	switch as {
	case assocLeft:
		cur := e
		for {
			operands = append([]syntax.Expr{cur.R}, operands...)
			ops = append([]string{cur.Op}, ops...)
			left = append([]bool{false}, left...)
			if b, ok := sameLevel(cur.L); ok {
				cur = b
				continue
			}
			operands = append([]syntax.Expr{cur.L}, operands...)
			left = append([]bool{true}, left...)
			break
		}
	case assocRight:
		cur := e
		for {
			operands = append(operands, cur.L)
			ops = append(ops, cur.Op)
			left = append(left, true)
			if b, ok := sameLevel(cur.R); ok {
				cur = b
				continue
			}
			operands = append(operands, cur.R)
			left = append(left, false)
			break
		}
	default:
		operands, ops, left = []syntax.Expr{e.L, e.R}, []string{e.Op}, []bool{true, false}
	}
	// every operator in the chain has e.Op's level and associativity
	docs := make([]Doc, len(operands))
	multiline := false // an operand spans lines whatever the width (a vertical record, a block)
	for i, x := range operands {
		docs[i] = p.operand(x, p.needsParens(e.Op, x, left[i]))
		_, hard := measure(docs[i])
		multiline = multiline || hard
	}
	sep := Line
	switch {
	case lv == levelPipe && len(ops) >= 2:
		sep = HardLine
	case multiline: // the operand's own lines break the expression: `[` … `] == 2`
		sep = Text(" ")
	}
	var rest []Doc
	for i := 1; i < len(operands); i++ {
		rest = append(rest, sep, Text(ops[i-1]+" "), docs[i])
	}
	return Group(Concat(docs[0], Nest(Concat(rest...))))
}

// startsWithMinus reports whether e is printed starting with `-`: on a line of its own that
// would continue the line before as a subtraction.
func (p *printer) startsWithMinus(e syntax.Expr) bool {
	switch x := e.(type) {
	case *syntax.IntLit, *syntax.FloatLit:
		return strings.HasPrefix(p.spanText(x.GetSpan()), "-")
	case *syntax.BinOp:
		return !p.needsParens(x.Op, x.L, true) && p.startsWithMinus(x.L)
	}
	return false
}

// ---- if, match, blocks

// ifChain prints `if c then a else b` on one line if it fits, else each branch on its own
// line, indented, with `else if` chains flattened at the same indent. A block branch opens on
// the `then` or `else` line, K&R style: `if c then {` … `} else {` … `}`.
func (p *printer) ifChain(e *syntax.If) Doc { return Group(Concat(p.ifParts(e)...)) }

// ifParts is ifChain's content, outside a group: a markup `{if}` whose `else` is an `if`
// expression continues the chain in its own group.
func (p *printer) ifParts(e *syntax.If) []Doc {
	var parts []Doc
	// a block branch opens on the `then` or `else` line, unless comments lead it
	opens := func(x syntax.Expr) bool {
		_, ok := x.(*syntax.Block)
		return ok && !p.hasLead(x)
	}
	branch := func(x syntax.Expr) {
		if opens(x) {
			parts = append(parts, Text(" "), p.expr(x))
		} else {
			parts = append(parts, Nest(Concat(Line, p.lead(x, true), p.expr(x))))
		}
	}
	for {
		parts = append(parts, Text("if "), p.expr(e.Cond), Text(" then"))
		branch(e.Then)
		if opens(e.Then) {
			parts = append(parts, Text(" else"))
		} else {
			parts = append(parts, Line, Text("else"))
		}
		next, ok := e.Else.(*syntax.If)
		if !ok || p.hasLead(next) {
			break
		}
		parts = append(parts, Text(" "))
		e = next
	}
	branch(e.Else)
	return parts
}

// match prints `match subject {`, then each arm on its own line with a trailing comma.
func (p *printer) match(e *syntax.Match) Doc {
	saved := p.noBrace
	p.noBrace = true
	subj := p.expr(e.Subject)
	p.noBrace = false
	defer func() { p.noBrace = saved }()
	if len(e.Arms) == 0 && len(p.c.dangling[e]) == 0 {
		return Concat(Text("match "), subj, Text(" { }"))
	}
	return Concat(Text("match "), subj, Text(" {"), p.arms(e, e.Arms, func(a *syntax.Arm) Doc { return p.after(a.Body) }), HardLine, Text("}"))
}

// arms prints match arms one per line, each with a trailing comma and its comments, then the
// match's dangling comments, indented.
func (p *printer) arms(group any, arms []*syntax.Arm, body func(*syntax.Arm) Doc) Doc {
	var out []Doc
	for i, a := range arms {
		out = append(out, HardLine, p.lead(a, i == 0), p.pattern(a.Pat), Text(" ->"), body(a), Text(","), p.trail(a))
	}
	return Nest(Concat(append(out, p.dangle(group, len(arms) == 0, nil))...))
}

// block prints `{`, the statements and the result one per line, `}`. A blank line between two
// of them in the source is kept (one, however many there were). A block with only a result is
// `{ x }` if it fits. It stays a block: the checker treats some blocks differently from their
// result (a test's body, an `update` that is a match).
func (p *printer) block(b *syntax.Block) Doc {
	stmts, result := b.Stmts, b.Result
	saved := p.noBrace
	p.noBrace = false
	defer func() { p.noBrace = saved }()
	if len(stmts) == 0 {
		return Group(Concat(Text("{"), Nest(Concat(Line, p.withComments(result, p.expr(result)), p.dangle(b, false, nil))), Line, Text("}")))
	}
	var items []Doc
	prevEnd := 0
	add := func(n syntax.Spanned, d Doc) {
		if len(items) > 0 {
			items = append(items, HardLine)
			if p.blankBetween(prevEnd, p.startLine(n)) {
				items = append(items, HardLine)
			}
		}
		items = append(items, p.withComments(n, d))
		prevEnd = n.GetSpan().End.Line
	}
	for _, s := range stmts {
		head := "let " + s.Name
		var doc []Doc
		if s.Type != nil {
			doc = append(doc, Text(head+" : "), p.typ(s.Type))
			head = ""
		}
		op := " ="
		if s.Bind {
			op = " <-"
		}
		doc = append(doc, Text(head+op), p.after(s.Expr))
		add(s, Concat(doc...))
	}
	// a result starting with `-` on its own line would continue the last statement
	add(result, p.operand(result, len(stmts) > 0 && p.startsWithMinus(result)))
	return Concat(Text("{"), Nest(Concat(HardLine, Concat(items...), p.dangle(b, false, nil))), HardLine, Text("}"))
}

// ---- records and lists

func (p *printer) list(e *syntax.ListLit) Doc {
	items := make([]Doc, len(e.Elems))
	nodes := make([]any, len(e.Elems))
	for i, x := range e.Elems {
		items[i], nodes[i] = p.inBrackets(x), x
	}
	return p.bracketed(e, nodes, items)
}

// bracketed prints `[a, b]`, or one item per line with trailing commas; no items is `[]`.
func (p *printer) bracketed(group any, nodes []any, items []Doc) Doc {
	if len(items) == 0 && len(p.c.dangling[group]) == 0 {
		return Text("[]")
	}
	first := SoftLine
	if len(items) == 0 {
		first = Text("")
	}
	return Group(Concat(Text("["), Nest(Concat(first, p.commaItems(group, nodes, items, true))), SoftLine, Text("]")))
}

func (p *printer) fieldInits(fs []*syntax.FieldInit) []Doc {
	items := make([]Doc, len(fs))
	for i, f := range fs {
		items[i] = Concat(Text(f.Name+": "), p.inBrackets(f.Value))
	}
	return items
}

// recordLit prints `{ a: 1, b: 2 }`: 3 or more fields always one per line. The empty record is
// `{}`.
func (p *printer) recordLit(e *syntax.RecordLit) Doc {
	if len(e.Fields) == 0 && len(p.c.dangling[e]) == 0 {
		return Text("{}")
	}
	return p.braced(e, initItems(e.Fields), p.fieldInits(e.Fields), len(e.Fields) >= 3)
}

// recordUpdate prints `{ r | a: 1 }`, or `{ r |`, one field per line, `}`. The grammar takes no
// trailing comma here.
func (p *printer) recordUpdate(e *syntax.RecordUpdate) Doc {
	first := Line
	if len(e.Fields) >= 3 {
		first = HardLine
	}
	return Group(Concat(Text("{ "), p.inBrackets(e.Base), Text(" |"),
		Nest(Concat(first, p.commaItems(e, initItems(e.Fields), p.fieldInits(e.Fields), false))), Line, Text("}")))
}

// ---- patterns

// pattern prints a pattern where any pattern may stand (an arm's).
func (p *printer) pattern(pt syntax.Pattern) Doc {
	switch pt := pt.(type) {
	case *syntax.PWild:
		return Text("_")
	case *syntax.PVar:
		return Text(pt.Name)
	case *syntax.PLit:
		return Text(p.spanText(pt.Span))
	case *syntax.PCtor:
		if pt.Arg == nil {
			return Text(pt.Name)
		}
		return Concat(Text(pt.Name+" "), p.subPattern(pt.Arg, true))
	case *syntax.PRecord:
		if len(pt.Fields) == 0 {
			return Text("{}")
		}
		items := make([]Doc, len(pt.Fields))
		for i, f := range pt.Fields {
			items[i] = Concat(Text(f.Name+": "), p.subPattern(f.Pat, false))
		}
		return p.braced(nil, nil, items, false)
	case *syntax.PList:
		// one line: the grammar takes no line break inside a list pattern
		var items []Doc
		for _, x := range pt.Elems {
			items = append(items, p.subPattern(x, false))
		}
		if pt.Rest != "" {
			items = append(items, Text(".."+pt.Rest))
		}
		return Flat(Concat(Text("["), join(items, Text(", ")), Text("]")))
	}
	panic("format: unknown pattern")
}

// subPattern prints a constructor's argument (ctorArg) or a field's or list element's pattern:
// a constructor with an argument in parentheses, and as a constructor's argument also a list or
// a negative literal (the grammar does not start an argument with `[` or `-`).
func (p *printer) subPattern(pt syntax.Pattern, ctorArg bool) Doc {
	paren := false
	switch x := pt.(type) {
	case *syntax.PCtor:
		paren = x.Arg != nil
	case *syntax.PList:
		paren = ctorArg
	case *syntax.PLit:
		paren = ctorArg && strings.HasPrefix(p.spanText(x.Span), "-")
	}
	if paren {
		return Concat(Text("("), p.pattern(pt), Text(")"))
	}
	return p.pattern(pt)
}
