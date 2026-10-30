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

// pipeComparisonHint is the hint of `x |> f == y`, which reads as `x |> (f == y)`: "" unless the
// right side of the pipe x is a comparison.
func (mc *modCtx) pipeComparisonHint(x *syntax.BinOp) string {
	cmp, ok := x.R.(*syntax.BinOp)
	if !ok || !isComparison(cmp.Op) {
		return ""
	}
	piped, fn, rhs := mc.operand(x.L, 0), mc.operand(cmp.L, 1), mc.operand(cmp.R, 5)
	if piped == "" || fn == "" || rhs == "" {
		return fmt.Sprintf("; `|>` binds looser than `%s`: put the pipe in parentheses, `(x |> f) %s y`", cmp.Op, cmp.Op)
	}
	return fmt.Sprintf("; `|>` binds looser than `%s`, so this reads as `%s |> (%s %s %s)`; write `(%s |> %s) %s %s`",
		cmp.Op, piped, fn, cmp.Op, rhs, piped, fn, cmp.Op, rhs)
}

// defaultHint adds the hint of `a && m ?? d`, which reads as `(a && m) ?? d`, to the mismatches
// reported at the left side of x (a `??`) since the bag held from diagnostics. The left side must
// be a `&&` or `||`; the operand the default was meant for is its last one (descending into a
// last `&&` or `||` not written in parentheses).
func (mc *modCtx) defaultHint(x *syntax.BinOp, from int) {
	l, ok := x.L.(*syntax.BinOp)
	if !ok || (l.Op != "&&" && l.Op != "||") {
		return
	}
	parent, last := l, l.R
	for {
		b, ok := last.(*syntax.BinOp)
		if !ok || (b.Op != "&&" && b.Op != "||") || b.Span.End != parent.Span.End {
			break
		}
		parent, last = b, b.R
	}
	hint := mc.defaultHintText(x, l, parent, last)
	// the left side is a Bool where a Maybe is expected, and its last operand, a Maybe, is
	// where `&&` expects a Bool: both mismatches get the hint
	items := mc.c.bag.Items
	for i := from; i < len(items); i++ {
		if sp := items[i].Span; items[i].Code == diag.TypeMismatch && (sp == l.Span || sp == last.GetSpan()) {
			items[i].Message += hint
		}
	}
}

// defaultHintText is defaultHint's text: last is the operand the default was meant for, and
// parent the `&&` or `||` it is the right side of.
func (mc *modCtx) defaultHintText(x, l, parent *syntax.BinOp, last syntax.Expr) string {
	generic := fmt.Sprintf("; `??` binds looser than `%s`: put the `??` and the value it defaults in parentheses", parent.Op)
	whole, def := mc.operand(l, 0), mc.operand(x.R, 1)
	opnd := mc.operand(last, 2)
	// The text before the operand, less the parentheses the operand was written in: after the
	// operand, up to the end of the left side, only those closing parentheses may follow.
	file := l.Span.File
	before := mc.source(diag.Span{File: file, Start: l.Span.Start, End: last.GetSpan().Start})
	after := mc.source(diag.Span{File: file, Start: last.GetSpan().End, End: l.Span.End})
	if whole == "" || def == "" || opnd == "" || before == "" || strings.Contains(before, "\n") || strings.Trim(after, ") ") != "" {
		return generic
	}
	n := strings.Count(after, ")")
	trimmed := strings.TrimRight(before, " ")
	for i := 0; i < n; i++ {
		if !strings.HasSuffix(trimmed, "(") {
			return generic
		}
		trimmed = strings.TrimRight(strings.TrimSuffix(trimmed, "("), " ")
	}
	if n > 0 {
		opnd = mc.source(last.GetSpan())
		opnd = strings.Repeat("(", n) + opnd + strings.Repeat(")", n)
	}
	return fmt.Sprintf("; `??` binds looser than `%s`, so this reads as `(%s) ?? %s`; write `%s (%s ?? %s)`",
		parent.Op, mc.source(l.Span), def, trimmed, opnd, def)
}
