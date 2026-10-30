package check

import (
	"fmt"
	"strings"

	"alder/internal/diag"
	"alder/internal/syntax"
)

// Precedence-trap hints (alder#10). The precedence stays as it is (loosest first: `|>`, `??`,
// `||`, `&&`, comparisons, `+ - ++`, `* / %`); these helpers only add a hint to the E0003 the trap
// already produces, showing the parenthesized form written with the program's own
// sub-expressions. They are kept apart from the mismatch messages themselves.

// binPrec is the binding strength of a binary operator (higher binds tighter), 0 for none.
func binPrec(op string) int {
	switch op {
	case "|>":
		return 1
	case "??":
		return 2
	case "||":
		return 3
	case "&&":
		return 4
	case "==", "!=", "<", "<=", ">", ">=":
		return 5
	case "+", "-", "++":
		return 6
	case "*", "/", "%":
		return 7
	}
	return 0
}

func isComparison(op string) bool { return binPrec(op) == 5 }

// operand is e's source text as an operand of a binary operator of strength prec: in
// parentheses when e binds no tighter (or is a lambda, `if` or `match`, which extend to the
// right). "" when e's text spans several lines, so a hint never prints a broken layout.
func (mc *modCtx) operand(e syntax.Expr, prec int) string {
	src := mc.source(e.GetSpan())
	if src == "" || strings.Contains(src, "\n") {
		return ""
	}
	wrap := false
	switch x := e.(type) {
	case *syntax.BinOp:
		wrap = binPrec(x.Op) <= prec
	case *syntax.Lambda, *syntax.If, *syntax.Match:
		wrap = true
	}
	if wrap {
		return "(" + src + ")"
	}
	return src
}

// pipeComparisonHint is the hint of `x |> f == y`, which reads as `x |> (f == y)`: "" unless a
// comparison lies on the leftmost spine of the pipe's right side (`x |> f == y && ok` too). The
// rewrite pipes into the spine's leftmost operand, the function the pipe was meant for.
func (mc *modCtx) pipeComparisonHint(x *syntax.BinOp) string {
	var cmp *syntax.BinOp
	leaf := x.R
	for {
		b, ok := leaf.(*syntax.BinOp)
		if !ok {
			break
		}
		if cmp == nil && isComparison(b.Op) {
			cmp = b
		}
		leaf = b.L
	}
	if cmp == nil {
		return ""
	}
	piped, fn, right := mc.operand(x.L, 0), mc.operand(leaf, 1), mc.source(x.R.GetSpan())
	rewrite := ""
	if piped != "" && fn != "" {
		rewrite = mc.spliced(x.R.GetSpan(), leaf.GetSpan(), "("+piped+" |> "+fn+")")
	}
	if rewrite == "" || strings.Contains(right, "\n") {
		return fmt.Sprintf("; `|>` binds looser than `%s`: put the pipe in parentheses, `(x |> f) %s y`", cmp.Op, cmp.Op)
	}
	return fmt.Sprintf("; `|>` binds looser than `%s`, so this reads as `%s |> (%s)`; write `%s`", cmp.Op, piped, right, rewrite)
}

func isLogic(op string) bool { return op == "&&" || op == "||" }

// defaultHint adds the hint of `a && m ?? d`, which reads as `(a && m) ?? d`, to the mismatches
// reported, since the bag held from diagnostics, at the left side of x (a `??`) and at the
// operand the default was meant for. The left side must be a `&&` or `||`; that operand is the
// last one of its trailing chain of `&&` and `||`, through any parentheses (`a && (b && m)`):
// the innermost, which `&&` or `||` rejected as not a Bool.
func (mc *modCtx) defaultHint(x *syntax.BinOp, from int) {
	l, ok := x.L.(*syntax.BinOp)
	if !ok || !isLogic(l.Op) {
		return
	}
	parent, last := l, l.R
	for {
		b, ok := last.(*syntax.BinOp)
		if !ok || !isLogic(b.Op) {
			break
		}
		parent, last = b, b.R
	}
	items := mc.c.bag.Items
	maybe := false // the operand was rejected: it is the Maybe the default belongs to
	for i := from; i < len(items); i++ {
		if items[i].Code == diag.TypeMismatch && items[i].Span == last.GetSpan() {
			maybe = true
		}
	}
	hint := mc.defaultHintText(x, l, parent, last, maybe)
	for i := from; i < len(items); i++ {
		if sp := items[i].Span; items[i].Code == diag.TypeMismatch && (sp == l.Span || sp == last.GetSpan()) {
			items[i].Message += hint
		}
	}
}

// defaultHintText is defaultHint's text: last is the operand the default was meant for, parent
// the `&&` or `||` it is the right side of, and maybe whether last was rejected as not a Bool
// (without that, which operand was meant is not known and the hint gives only the rule).
func (mc *modCtx) defaultHintText(x, l, parent *syntax.BinOp, last syntax.Expr, maybe bool) string {
	generic := fmt.Sprintf("; `??` binds looser than `%s`: put the `??` and the value it defaults in parentheses", parent.Op)
	def, opnd, whole := mc.operand(x.R, 1), mc.operand(last, 2), mc.source(l.Span)
	if !maybe || def == "" || opnd == "" || whole == "" || strings.Contains(whole, "\n") {
		return generic
	}
	rewrite := mc.spliced(l.Span, last.GetSpan(), "("+opnd+" ?? "+def+")")
	if rewrite == "" {
		return generic
	}
	return fmt.Sprintf("; `??` binds looser than `%s`, so this reads as `(%s) ?? %s`; write `%s`", parent.Op, whole, def, rewrite)
}

// spliced is outer's source text with inner's replaced by text, which brings its own
// parentheses: any written around inner alone (`((m))`) are replaced too. "" unless both are
// on one line.
func (mc *modCtx) spliced(outer, inner diag.Span, text string) string {
	if outer.Start.Line != outer.End.Line || inner.Start.Line != outer.Start.Line || inner.End.Line != outer.Start.Line {
		return ""
	}
	line := lineOf(mc.m, outer.Start.Line)
	os, oe, is, ie := outer.Start.Col-1, outer.End.Col-1, inner.Start.Col-1, inner.End.Col-1
	if os < 0 || oe > len(line) || is < os || ie > oe || is > ie {
		return ""
	}
	for {
		before := strings.TrimRight(line[os:is], " ")
		after := strings.TrimLeft(line[ie:oe], " ")
		if !strings.HasSuffix(before, "(") || !strings.HasPrefix(after, ")") {
			break
		}
		is, ie = os+len(before)-1, oe-len(after)+1
	}
	return strings.TrimSpace(line[os:is] + text + line[ie:oe])
}
