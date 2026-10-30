package format

import (
	"strconv"
	"strings"

	"alder/internal/syntax"
)

// Paste prints e, an expression of module m, as the formatter lays it out, except that each
// variable in holes is written as the placeholder of the property argument holes gives it:
// "\x00" mode n "\x00", n its index. The placeholders are filled with Alder literals when a
// property fails (rtesting.Paste), so the mode says what the place needs: 'a' where an
// application would need parentheses (an argument or function of an application, the base of
// a field access or record update), 'p' where it would not; 'A' and 'P' are those inside a
// match subject, where a literal starting with `{` would open the arms. ok is false when a
// hole is inside a string literal (its interpolations are printed verbatim) or markup.
func Paste(m *syntax.Module, e syntax.Expr, holes map[*syntax.Var]int) (out string, ok bool) {
	places := map[*syntax.Var]string{}
	if !pastePlaces(e, false, false, holes, places) {
		return "", false
	}
	p := newPrinter([]byte(strings.Join(m.Lines, "\n")))
	p.c = attach(m)
	p.holes = places
	return strings.TrimRight(Render(p.expr(e), Width), "\n"), true
}

// Pattern prints pt, a pattern of module m, on one line as the formatter lays it out (the
// checker quotes an unreachable arm's pattern with it).
func Pattern(m *syntax.Module, pt syntax.Pattern) string {
	p := newPrinter([]byte(strings.Join(m.Lines, "\n")))
	p.c = attach(m) // a record pattern's braces ask it for dangling comments
	return Render(Flat(p.pattern(pt)), Width)
}

// pastePlaces records the placeholder of each hole under n: atom says n stands where an
// application needs parentheses, subject that it is inside a match subject. It reports false
// for a hole inside a string literal or markup.
func pastePlaces(n syntax.Spanned, atom, subject bool, holes map[*syntax.Var]int, places map[*syntax.Var]string) bool {
	if v, ok := n.(*syntax.Var); ok {
		i, hole := holes[v]
		if !hole {
			return true
		}
		mode := byte('p')
		if atom {
			mode = 'a'
		}
		if subject {
			mode -= 'a' - 'A'
		}
		places[v] = "\x00" + string(mode) + strconv.Itoa(i) + "\x00"
		return true
	}
	switch x := n.(type) {
	case *syntax.StrLit, *syntax.Element:
		return !hasHole(x, holes)
	case *syntax.App:
		if !pastePlaces(x.Fn, true, subject, holes, places) {
			return false
		}
		for _, a := range x.Args {
			if !pastePlaces(a, true, subject, holes, places) {
				return false
			}
		}
		return true
	case *syntax.Access:
		return pastePlaces(x.Expr, true, subject, holes, places)
	case *syntax.RecordUpdate:
		if !pastePlaces(x.Base, true, true, holes, places) {
			return false
		}
		for _, f := range x.Fields {
			if !pastePlaces(f, false, subject, holes, places) {
				return false
			}
		}
		return true
	case *syntax.Match:
		if !pastePlaces(x.Subject, false, true, holes, places) {
			return false
		}
		for _, a := range x.Arms {
			if !pastePlaces(a, false, subject, holes, places) {
				return false
			}
		}
		return true
	}
	for _, c := range syntax.Children(n) {
		if !pastePlaces(c, false, subject, holes, places) {
			return false
		}
	}
	return true
}

// hasHole reports whether a hole is n or below it.
func hasHole(n syntax.Spanned, holes map[*syntax.Var]int) bool {
	if v, ok := n.(*syntax.Var); ok {
		_, hole := holes[v]
		return hole
	}
	for _, c := range syntax.Children(n) {
		if hasHole(c, holes) {
			return true
		}
	}
	return false
}
