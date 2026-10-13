package generate

import (
	"strings"
	"unicode"

	"alder/internal/syntax"
)

// Fill generates `alder fill <Module>.<decl>[.<arm>] -`: the typed hole of that declaration
// (its only one, or the one named hole, `?name` or `name`) replaced by expr, in parentheses
// when the place needs them. Apply then writes it only if the app checks: the expression may
// leave holes of its own, but not errors. It refuses an address that names no declaration with
// a hole, a declaration with several holes and no hole named, and an empty expression.
func Fill(p *Program, address, hole, expr string) ([]Edit, *Hole, error) {
	expr = strings.TrimRight(expr, " \t\r\n")
	for strings.HasPrefix(expr, "\n") {
		expr = expr[1:]
	}
	if strings.TrimSpace(expr) == "" {
		return nil, nil, Refusef("standard input is empty: write the expression that fills the hole")
	}
	if _, ds, more, at := syntax.ParseExpr("stdin", []byte(expr)); more != "" {
		return nil, nil, Refusef("`alder fill` takes one expression; found more after it: %s at line %d, column %d of standard input", more, at.Line, at.Col)
	} else if len(ds) > 0 {
		return nil, nil, Refusef("`alder fill` takes one expression; standard input does not parse as one: %s (line %d, column %d)", ds[0].Message, ds[0].Span.Start.Line, ds[0].Span.Start.Col)
	}
	mod, decl, arm, err := parseAddress(p, address)
	if err != nil {
		return nil, nil, err
	}
	hole = strings.TrimPrefix(hole, "?")
	var in, named []Hole
	for _, h := range HoleIndex(p.Prog, p.Diags) {
		if h.Module == mod && h.Decl == decl && (arm == "" || h.Arm == arm) {
			in = append(in, h)
			if hole == "" || h.Name == hole {
				named = append(named, h)
			}
		}
	}
	switch {
	case len(in) == 0:
		return nil, nil, Refusef("`%s` has no typed hole (`alder fill --list` lists the app's holes)", address)
	case len(named) == 0:
		return nil, nil, Refusef("`%s` has no hole `?%s` (it has %s)", address, hole, holeNames(in))
	case len(named) > 1 && hole == "":
		return nil, nil, Refusef("`%s` has %d holes (%s): name one with --hole", address, len(named), holeNames(named))
	case len(named) > 1:
		return nil, nil, Refusef("`%s` has %d holes named `?%s`: fill them one at a time by naming a declaration that holds one", address, len(named), hole)
	}
	h := named[0]
	m := p.Modules[h.Module]
	lines := strings.Split(strings.TrimSuffix(m.Source, "\n"), "\n")
	sp := h.Span
	if sp.Start.Line != sp.End.Line || sp.Start.Line > len(lines) {
		return nil, nil, Refusef("internal error: the hole ?%s is not on one line", h.Name)
	}
	l := lines[sp.Start.Line-1]
	if sp.Start.Col < 1 || sp.End.Col-1 > len(l) || l[sp.Start.Col-1:sp.End.Col-1] != "?"+h.Name {
		return nil, nil, Refusef("internal error: the hole ?%s is not where the checker says", h.Name)
	}
	before, after := l[:sp.Start.Col-1], l[sp.End.Col-1:]
	if needsParens(before, after, expr) {
		expr = "(" + expr + ")"
	}
	// a multi-line expression continues at the hole's line's indentation
	exprLines := strings.Split(expr, "\n")
	for i := 1; i < len(exprLines); i++ {
		if exprLines[i] != "" {
			exprLines[i] = indentOf(l) + exprLines[i]
		}
	}
	lines[sp.Start.Line-1] = before + strings.Join(exprLines, "\n") + after
	return []Edit{{Path: m.File, Body: strings.Join(lines, "\n") + "\n"}}, &h, nil
}

// parseAddress splits `Module.decl[.arm]`: the module is the longest prefix naming one of the
// app's modules; decl is a value's name, a fake's, or a test's name in double quotes.
func parseAddress(p *Program, address string) (mod, decl, arm string, err error) {
	if i := strings.Index(address, `."`); i > 0 {
		mod = address[:i]
		rest := address[i+1:]
		j := strings.Index(rest[1:], `"`)
		if j < 0 {
			return "", "", "", Refusef("`%s`: the test's name has no closing `\"`", address)
		}
		decl, rest = rest[:j+2], rest[j+2:]
		if rest != "" {
			return "", "", "", Refusef("`%s` is not a declaration: write `Module.decl`, `Module.update.Msg` or `Module.\"test name\"`", address)
		}
		if p.Modules[mod] == nil {
			return "", "", "", Refusef("there is no module `%s`", mod)
		}
		return mod, decl, "", nil
	}
	segs := strings.Split(address, ".")
	for i := len(segs) - 1; i >= 1; i-- {
		if p.Modules[strings.Join(segs[:i], ".")] != nil {
			mod, rest := strings.Join(segs[:i], "."), segs[i:]
			if len(rest) > 2 {
				return "", "", "", Refusef("`%s` is not a declaration: write `Module.decl`, `Module.update.Msg` or `Module.\"test name\"`", address)
			}
			decl = rest[0]
			if len(rest) == 2 {
				arm = rest[1]
			}
			return mod, decl, arm, nil
		}
	}
	return "", "", "", Refusef("`%s` names no module of this app: write `Module.decl`, such as `Home.update`", address)
}

// holeNames lists holes for a message: "?a, ?b".
func holeNames(hs []Hole) string {
	var names []string
	for _, h := range hs {
		names = append(names, "?"+h.Name)
	}
	return strings.Join(names, ", ")
}

// needsParens reports whether expr must be parenthesised to replace a hole between the texts
// before and after it on its line: not when it is atomic (a name, a literal, one bracketed
// whole), nor where any expression may stand (after `->`, `=`, `<-`, `:`, an opening bracket or
// a comma, before a closing one, a comma or the end of the line); after an operator only when it
// has an operator or a keyword of its own at its top level.
func needsParens(before, after, expr string) bool {
	if atomic(expr) {
		return false
	}
	b := strings.TrimRight(before, " ")
	a := strings.TrimLeft(after, " ")
	opens := b == "" || strings.HasSuffix(b, "->") || strings.HasSuffix(b, "<-") || strings.HasSuffix(b, ":") ||
		strings.HasSuffix(b, "{") || strings.HasSuffix(b, "(") || strings.HasSuffix(b, "[") || strings.HasSuffix(b, ",") ||
		(strings.HasSuffix(b, "=") && !strings.HasSuffix(b, "==") && !strings.HasSuffix(b, "!=") && !strings.HasSuffix(b, "<=") && !strings.HasSuffix(b, ">="))
	closes := a == "" || strings.HasPrefix(a, ",") || strings.HasPrefix(a, "}") || strings.HasPrefix(a, ")") || strings.HasPrefix(a, "]")
	if opens && closes {
		return false
	}
	last := rune(0)
	if b != "" {
		last = rune(b[len(b)-1])
	}
	application := unicode.IsLetter(last) || unicode.IsDigit(last) || last == ')' || last == ']' || last == '}' || last == '"' || last == '_'
	if application {
		return true
	}
	return topLevelOperator(expr)
}

// atomic reports whether expr is one name or number, or one bracketed whole (`( … )`, `[ … ]`,
// `{ … }`) or string.
func atomic(expr string) bool {
	isWord := expr != ""
	for _, r := range expr {
		if !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '_') {
			isWord = false
		}
	}
	return isWord || closing(expr, 0) == len(expr)-1
}

// closing is the index of the bracket or quote that closes the one at s[i], skipping strings
// and nested brackets; -1 when s[i] opens nothing or is not closed.
func closing(s string, i int) int {
	if i >= len(s) {
		return -1
	}
	if s[i] == '"' {
		for j := i + 1; j < len(s); j++ {
			switch s[j] {
			case '\\':
				j++
			case '"':
				return j
			}
		}
		return -1
	}
	if !strings.ContainsRune("([{", rune(s[i])) {
		return -1
	}
	depth := 0
	for j := i; j < len(s); j++ {
		switch s[j] {
		case '"':
			if j = closing(s, j); j < 0 {
				return -1
			}
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			if depth--; depth == 0 {
				return j
			}
		}
	}
	return -1
}

// topLevelOperator reports whether expr has, outside brackets and strings, a binary operator, or
// starts with a keyword (`if`, `match`, `let`) or a lambda.
func topLevelOperator(expr string) bool {
	t := strings.TrimSpace(expr)
	for _, kw := range []string{"if ", "match ", "let ", "\\"} {
		if strings.HasPrefix(t, kw) {
			return true
		}
	}
	depth, inStr := 0, false
	for i := 0; i < len(t); i++ {
		ch := t[i]
		switch {
		case inStr:
			if ch == '\\' {
				i++
			} else if ch == '"' {
				inStr = false
			}
		case ch == '"':
			inStr = true
		case ch == '(' || ch == '[' || ch == '{':
			depth++
		case ch == ')' || ch == ']' || ch == '}':
			depth--
		case depth == 0 && strings.ContainsRune("=!<>+*/%&|?", rune(ch)):
			return true
		case depth == 0 && ch == '-' && i > 0 && t[i-1] == ' ' && i+1 < len(t) && t[i+1] == ' ':
			return true
		}
	}
	return false
}
