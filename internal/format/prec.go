package format

import (
	"strings"

	"alder/internal/syntax"
)

// assoc is how a binary operator groups with another of its own level.
type assoc int

const (
	assocLeft  assoc = iota // a - b - c is (a - b) - c
	assocRight              // a ?? b ?? c is a ?? (b ?? c)
	assocNone               // a == b == c does not parse
)

// Precedence levels, lowest first, as the parser's parse* chain (internal/syntax/parser_expr.go)
// nests them: parsePipe, parseDefault, parseOr, parseAnd, parseCmp, parseAdd, parseMul, parseApp,
// parsePostfix.
const (
	// levelOpen is a lambda, `if` or `match`: printed in parentheses wherever it is an operand,
	// a function, an argument or a field's base. A lambda's body and an `if`'s `else` run as far
	// right as they can, and `if`/`match` cannot start an argument. A lambda as the last argument
	// (`f \x -> e`) parses without them; parenthesising it anyway is deliberate, the one place the
	// parentheses are not minimal: Alder code (and Elm) always writes `f (\x -> e)`.
	levelOpen = iota
	levelPipe
	levelDefault
	levelOr
	levelAnd
	levelCmp
	levelAdd
	levelMul
	// levelApp is an application, and anything else that cannot be an argument without
	// parentheses: a negative literal (an argument cannot start with `-`) and markup (nor `<`).
	levelApp
	levelPostfix // field access
	levelAtom
)

// prec returns a binary operator's level and associativity.
func prec(op string) (int, assoc) {
	switch op {
	case "|>":
		return levelPipe, assocLeft
	case "??":
		return levelDefault, assocRight // parseDefault: parseOr ?? parseDefault
	case "||":
		return levelOr, assocLeft
	case "&&":
		return levelAnd, assocLeft
	case "==", "!=", "<", "<=", ">", ">=":
		return levelCmp, assocNone // parseCmp: parseAdd op parseAdd, and comparisons do not chain
	case "+", "-", "++":
		return levelAdd, assocLeft
	case "*", "/", "%":
		return levelMul, assocLeft
	}
	panic("format: unknown operator " + op)
}

// level is the precedence level of an expression as printed.
func (p *printer) level(e syntax.Expr) int {
	switch e := e.(type) {
	case *syntax.BinOp:
		l, _ := prec(e.Op)
		return l
	case *syntax.Lambda, *syntax.If, *syntax.Match:
		return levelOpen
	case *syntax.App, *syntax.Element:
		return levelApp
	case *syntax.IntLit, *syntax.FloatLit:
		if strings.HasPrefix(p.spanText(e.GetSpan()), "-") { // a negative literal, `-0` too
			return levelApp
		}
	case *syntax.Access:
		return levelPostfix
	}
	return levelAtom
}

// needsParens reports whether a child of the binary operator op, on the left or the right,
// must be parenthesised.
func (p *printer) needsParens(op string, child syntax.Expr, left bool) bool {
	l, a := prec(op)
	c := p.level(child)
	if c != l {
		return c < l
	}
	if left {
		return a != assocLeft
	}
	return a != assocRight
}
