package generate

import (
	"maps"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"alder/internal/names"
	"alder/internal/syntax"
)

// Fill generates `alder fill <Module>.<decl>[.<arm>] -`: the typed hole of that declaration
// (its only one, or the one named hole, `?name` or `name`) replaced by expr, in parentheses
// when the place needs them. Apply then writes it only if the app checks: the expression may
// leave holes of its own, but not errors. It refuses an address that names no declaration with
// a hole, a declaration with several holes and no hole named, and an empty expression. A module
// of the app that expr names qualified (`Member.x`) and the hole's module does not import is
// imported (imports: their names); a standard library module needs no import, and a name that
// is not a module's is left to the check. A short qualifier (`Index.x`) of a module not yet
// imported also counts when exactly one module of the app would get it as its canonical
// qualifier beside the imports the module has; when several would, fill refuses and names them.
func Fill(p *Program, address, hole, expr string) (edits []Edit, h *Hole, imports []string, err error) {
	if err := controlChars(expr); err != nil {
		return nil, nil, nil, err
	}
	expr = strings.TrimRight(expr, " \t\r\n")
	for strings.HasPrefix(expr, "\n") {
		expr = expr[1:]
	}
	if strings.TrimSpace(expr) == "" {
		return nil, nil, nil, Refusef("standard input is empty: write the expression that fills the hole")
	}
	parsed, ds, more, at := syntax.ParseExpr("stdin", []byte(expr))
	if more != "" {
		hint := ""
		if more == "`->`" {
			hint = " (a lambda starts with `\\`: `\\a b -> …`)"
		}
		return nil, nil, nil, Refusef("`alder fill` takes one expression; found more after it: %s at line %d, column %d of standard input%s", more, at.Line, at.Col, hint)
	} else if len(ds) > 0 {
		return nil, nil, nil, Refusef("`alder fill` takes one expression; standard input does not parse as one: %s (line %d, column %d)", ds[0].Message, ds[0].Span.Start.Line, ds[0].Span.Start.Col)
	}
	h, in, err := findHole(p, address, hole)
	if err != nil {
		return nil, nil, nil, err
	}
	hole = strings.TrimPrefix(hole, "?")
	m := p.Modules[h.Module]
	if err := fillShape(m, h, parsed, hole); err != nil {
		return nil, nil, nil, err
	}
	lines := strings.Split(strings.TrimSuffix(m.Source, "\n"), "\n")
	sp := h.Span
	if sp.Start.Line != sp.End.Line || sp.Start.Line > len(lines) {
		return nil, nil, nil, Refusef("internal error: the hole ?%s is not on one line", h.Name)
	}
	l := lines[sp.Start.Line-1]
	if sp.Start.Col < 1 || sp.End.Col-1 > len(l) || l[sp.Start.Col-1:sp.End.Col-1] != "?"+h.Name {
		return nil, nil, nil, Refusef("internal error: the hole ?%s is not where the checker says", h.Name)
	}
	before, after := l[:sp.Start.Col-1], l[sp.End.Col-1:]
	inner, ok := soleInBlock(lines, sp.Start.Line, h.Name, expr)
	oneLine := false
	if !ok && strings.HasSuffix(before, "{ ") && strings.TrimSpace(after) == "}" {
		// `{ ?test }` on one line, as `alder fmt` writes a block with only a hole
		inner, ok = blockBody(expr)
		oneLine = ok
	}
	if ok {
		// the hole is a block's whole body (`alder new test`'s `{ ?test }`) and expr a block:
		// expr's statements become that block's, rather than a block in a block
		e := newSrcEdit(m.Source)
		if oneLine {
			e.replaceLine(sp.Start.Line, strings.TrimSuffix(before, " ")+"\n"+indent(inner, indentOf(l)+"  ")+"\n"+indentOf(l)+strings.TrimLeft(after, " "))
		} else {
			e.replaceLine(sp.Start.Line, indent(inner, indentOf(l)))
		}
		dropGuidance(e, m.Syntax, h.Decl, h.Name == "expected", onlyExpectedLeft(in, h))
		if err := fillImports(e, p, m, expr, &imports); err != nil {
			return nil, nil, nil, err
		}
		return []Edit{{Path: m.File, Body: e.result()}}, h, imports, nil
	}
	// The expression goes between the hole line's text before and after it. A comment in it would
	// comment out the rest of that line (`n -- ` in `?a + 1 * n`), and an operator in it could bind
	// differently from the hole (`1 + 1` in `?a * 2`): its comments are cut (by the lexer), and
	// any expression but an atomic one is parenthesised; formatting drops the parentheses that
	// are not needed.
	expr = syntax.StripComments(expr)
	if strings.TrimSpace(expr) == "" {
		return nil, nil, nil, Refusef("standard input has only comments: write the expression that fills the hole")
	}
	if !atomic(expr) {
		expr = "(" + expr + ")"
	}
	// a multi-line expression continues at the hole's line's indentation
	exprLines := strings.Split(expr, "\n")
	for i := 1; i < len(exprLines); i++ {
		if exprLines[i] != "" {
			exprLines[i] = indentOf(l) + exprLines[i]
		}
	}
	e := newSrcEdit(m.Source)
	e.replaceLine(sp.Start.Line, before+strings.Join(exprLines, "\n")+after)
	dropGuidance(e, m.Syntax, h.Decl, h.Name == "expected", onlyExpectedLeft(in, h))
	if err := fillImports(e, p, m, expr, &imports); err != nil {
		return nil, nil, nil, err
	}
	return []Edit{{Path: m.File, Body: e.result()}}, h, imports, nil
}

// FindHole is the typed hole that `alder fill <address>` (with --hole hole, "" for none, `?name`
// or `name`) would fill, with Fill's refusals: an address that names no declaration with a
// hole, a hole the declaration does not have, several holes and none named.
func FindHole(p *Program, address, hole string) (*Hole, error) {
	h, _, err := findHole(p, address, hole)
	return h, err
}

// findHole is FindHole, with every hole of the declaration (in).
func findHole(p *Program, address, hole string) (h *Hole, in []Hole, err error) {
	mod, decl, arm, err := parseAddress(p, address)
	if err != nil {
		return nil, nil, err
	}
	hole = strings.TrimPrefix(hole, "?")
	var named []Hole
	for _, h := range HoleIndex(p.Prog, p.Diags) {
		if h.Module == mod && h.Decl == decl && (arm == "" || h.Arm == arm) {
			in = append(in, h)
			if hole == "" || h.Name == hole {
				named = append(named, h)
			}
		}
	}
	switch {
	case len(in) == 0 && arm != "":
		return nil, nil, Refusef("`%s` has no typed hole: a fill replaces a hole, and this arm has none; to rewrite it, give `alder fill --decl %s.update -` the whole `match msg { … }` (`alder fill --list` lists the app's holes)", address, mod)
	case len(in) == 0:
		return nil, nil, Refusef("`%s` has no typed hole; to rewrite it whole, `alder fill --decl %s -` (`alder fill --list` lists the app's holes)", address, shellAddress(address))
	case len(named) == 0:
		return nil, nil, Refusef("`%s` has no hole `?%s` (it has %s)", address, hole, holeNames(in))
	case len(named) > 1 && hole == "":
		return nil, nil, Refusef("`%s` has %d holes (%s): name one with --hole", address, len(named), holeNames(named))
	case len(named) > 1:
		return nil, nil, Refusef("`%s` has %d holes named `?%s`: fill them one at a time by naming a declaration that holds one", address, len(named), hole)
	}
	return &named[0], in, nil
}

// fillImports adds to e the imports of the modules of the app that expr names, by their full
// path, by a short qualifier (shortModules) or as a bare table's, and module m does not import
// yet; it adds their names to *imports.
func fillImports(e *srcEdit, p *Program, m *Module, expr string, imports *[]string) error {
	short, err := shortModules(p, m, expr)
	if err != nil {
		return err
	}
	have := map[string]bool{m.Name: true}
	for _, im := range m.Syntax.Imports {
		have[im.Module] = true
	}
	for _, q := range append(qualifiers(p, expr), short...) {
		if !have[q] {
			have[q] = true
			*imports = append(*imports, q)
		}
	}
	addImports(e, p, m.Syntax, *imports)
	return nil
}

// soleInBlock reports whether the hole ?name on line n (1-based) of lines is alone on its
// line, between a line that opens a block (`… {`) and one that closes it (`}`), and expr is
// one block: then it returns the block's statements, dedented, to put in the hole's place.
func soleInBlock(lines []string, n int, name, expr string) (string, bool) {
	if n < 2 || n >= len(lines) || strings.TrimSpace(lines[n-1]) != "?"+name ||
		!strings.HasSuffix(strings.TrimRight(lines[n-2], " "), "{") || !strings.HasPrefix(strings.TrimSpace(lines[n]), "}") {
		return "", false
	}
	return blockBody(expr)
}

// blockBody is the statements of expr when it is a block written over several lines, without
// its braces and their common indentation.
func blockBody(expr string) (string, bool) {
	if !strings.HasPrefix(expr, "{") || closing(expr, 0) != len(expr)-1 {
		return "", false
	}
	body := strings.Split(strings.Trim(expr[1:len(expr)-1], "\n"), "\n")
	if len(body) == 0 || strings.TrimSpace(strings.Join(body, "")) == "" || len(body) == 1 && !strings.Contains(expr, "\n") {
		return "", false // `{ x }` on one line: put as it is
	}
	common := -1
	for _, l := range body {
		if strings.TrimSpace(l) == "" {
			continue
		}
		if k := len(indentOf(l)); common < 0 || k < common {
			common = k
		}
	}
	for i, l := range body {
		if strings.TrimSpace(l) == "" {
			body[i] = ""
		} else {
			body[i] = l[common:]
		}
	}
	return strings.Join(body, "\n"), true
}

// qualifiers are the modules of the app (not the standard library's) that expr names a value,
// type or constructor of, qualified (`Member.x`, `Admin.Layout.isAdmin`): for each dotted name
// outside strings and comments, the longest prefix of its capitalised segments that is a
// module of the app, followed by one more segment, but App and the pages, which are named
// without an import; a table's module, named by the table's name as a qualifier (`User.UserId`
// for Table.User); and a table's (or type's) module named bare, as its record type is
// (`Db.from Team`). In first-use order.
func qualifiers(p *Program, expr string) []string {
	var out []string
	seen := map[string]bool{}
	for _, segs := range dottedNames(expr) {
		if len(segs) == 1 { // a table's record type: `Db.from Team` (module Table.Team, or Team)
			mod := ""
			if t := p.TableNamed(segs[0]); t != nil {
				mod = t.Module
			} else if p.namesOwnType(segs[0]) {
				mod = segs[0]
			}
			if mod != "" && !seen[mod] {
				seen[mod] = true
				out = append(out, mod)
			}
		}
		found := false
		for k := len(segs) - 1; k >= 1; k-- {
			name := strings.Join(segs[:k], ".")
			if p.Modules[name] != nil && !p.Std[name] {
				// App and the pages are named without an import (`App.Logout`, `Teams.Show.Remove`)
				if !seen[name] && name != "App" && p.PageNamed(name) == nil {
					seen[name] = true
					out = append(out, name)
				}
				found = true
				break
			}
		}
		// a table's name as the qualifier of its module's member (`User.UserId`, `Room.name`)
		if t := p.TableNamed(segs[0]); !found && len(segs) >= 2 && t != nil && !seen[t.Module] {
			seen[t.Module] = true
			out = append(out, t.Module)
		}
	}
	return out
}

// dottedNames are the names of expr that start with a capital letter, outside strings and
// comments, each as its segments (`Admin.Layout.isAdmin` is Admin, Layout, isAdmin), in order of
// appearance. It stops at an unterminated string.
func dottedNames(expr string) [][]string {
	var out [][]string
	for i := 0; i < len(expr); i++ {
		switch ch := expr[i]; {
		case ch >= utf8.RuneSelf: // a multi-byte character is no part of a name: step over all of it
			_, size := utf8.DecodeRuneInString(expr[i:])
			i += size - 1
		case ch == '"':
			if j := closing(expr, i); j > 0 {
				i = j
			} else {
				return out
			}
		case ch == '-' && strings.HasPrefix(expr[i:], "--"):
			for i < len(expr) && expr[i] != '\n' {
				i++
			}
		case 'A' <= ch && ch <= 'Z' && (i == 0 || !isNameByte(expr[i-1])):
			j := i
			for j < len(expr) && (isNameByte(expr[j]) || expr[j] == '.') {
				j++
			}
			out = append(out, strings.Split(strings.TrimRight(expr[i:j], "."), "."))
			i = j - 1
		case isNameByte(ch):
			for i+1 < len(expr) && (isNameByte(expr[i+1]) || expr[i+1] == '.') {
				i++
			}
		}
	}
	return out
}

// shortModules are the modules of the app that expr names by a short qualifier (`Index.helper`)
// without m importing them: the one module that would get that qualifier were it imported
// beside m's imports (the canonical qualifier of chapter 3). A written qualifier that m already
// has, that names a standard-library module, or that no module would get is left to the check.
// A qualifier that several modules would get is an error naming them, since fill cannot know
// which was meant.
func shortModules(p *Program, m *Module, expr string) ([]string, error) {
	isStd := func(n string) bool { return p.Std[n] }
	var old []string
	have := map[string]bool{m.Name: true}
	for _, im := range m.Syntax.Imports {
		have[im.Module] = true
		if !p.Std[im.Module] {
			old = append(old, im.Module)
		}
	}
	taken := names.Taken(m.Syntax, isStd)
	quals := canonicalQuals(m.Name, old, taken)
	cands := map[string][]string{} // qualifier -> the not-yet-imported modules that would get it
	var all []string
	for x := range p.Modules {
		if !have[x] && !p.Std[x] && x != "App" && p.PageNamed(x) == nil { // a page is never named by a short qualifier for a member
			all = append(all, x)
		}
	}
	slices.Sort(all)
	var out []string
	seen := map[string]bool{}
	for _, segs := range dottedNames(expr) {
		q := segs[0]
		if len(segs) < 2 || seen[q] || p.Std[q] || p.Modules[q] != nil {
			continue // not a qualified name, seen, or a module's full path (qualifiers finds those)
		}
		seen[q] = true
		if slices.Contains(slices.Collect(maps.Values(quals)), q) {
			continue
		}
		if _, done := cands[q]; !done {
			for _, x := range all {
				if canonicalQuals(m.Name, append(slices.Clone(old), x), taken)[x] == q {
					cands[q] = append(cands[q], x)
				}
			}
		}
		switch len(cands[q]) {
		case 0:
		case 1:
			out = append(out, cands[q][0])
		default:
			return nil, Refusef("`%s` in the expression could be the qualifier of %s, and `%s` imports none of them: write the import you mean first, then fill", q, "`"+strings.Join(cands[q], "`, `")+"`", m.Name)
		}
	}
	return out, nil
}

// namesOwnType reports whether name is a module of the app (not the standard library's) that
// declares a table or type of its own name (`Team`, whose record type is `Team`): written bare,
// that type needs the module imported.
func (p *Program) namesOwnType(name string) bool {
	m := p.Modules[name]
	if m == nil || p.Std[name] {
		return false
	}
	for _, d := range m.Syntax.Decls {
		switch d := d.(type) {
		case *syntax.TableDecl:
			if d.Name == name {
				return true
			}
		case *syntax.TypeDecl:
			if d.Name == name {
				return true
			}
		}
	}
	return false
}

func isNameByte(b byte) bool {
	return b == '_' || b >= '0' && b <= '9' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z'
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
			if len(rest) == 1 && strings.Contains(decl, " ") { // `Mod."test name"` unquoted by the shell
				decl = `"` + decl + `"`
			}
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

// onlyExpectedLeft reports whether, once h is filled, the holes of its declaration (in) left are
// at most ?expected: the loop note, which names the clicked row's holes, is then done with.
func onlyExpectedLeft(in []Hole, h *Hole) bool {
	for i := range in {
		if in[i].Span != h.Span && in[i].Name != "expected" {
			return false
		}
	}
	return true
}

// dropGuidance removes the comments `alder new msg` writes above a test it generates, once
// they are done with: the paragraph saying what to fill ?expected with (expected: that hole
// was filled), and the note on the rows of a loop (all: the test has no hole left but
// ?expected, so the clicked row's holes the note names are filled). decl is
// the test's name in quotes; any other declaration is left alone.
func dropGuidance(e *srcEdit, m *syntax.Module, decl string, expected, all bool) {
	for _, d := range m.Decls {
		t, ok := d.(*syntax.TestDecl)
		if !ok || quote(t.Name) != decl {
			continue
		}
		testLine := t.Span.Start.Line
		start := declStart(e, t)
		for n := start; n < testLine; n++ {
			l := e.line(n)
			switch {
			case expected && strings.HasPrefix(l, "-- Fill ?expected with "):
				for k := n; k < testLine; k++ {
					e.deleteLine(k)
				}
				return
			case all && strings.HasPrefix(l, "-- The button is in the view's `{for ") && n+1 < testLine:
				e.deleteLine(n)
				e.deleteLine(n + 1)
				n++
			}
		}
	}
}

// controlChars refuses standard input holding a control character other than a tab or a line
// end: what a shell's `echo` makes of the backslash escapes in a lambda (`echo '\from to -> …'`
// writes a form feed and `rom`; `\a` a bell, `\b` a backspace, `\v` a vertical tab). The
// expression lost its backslash: a here-document (`<<'EOF'`) or `printf '%s\n'` keeps it.
func controlChars(src string) error {
	line, col := 1, 0
	for _, r := range src {
		col++
		switch {
		case r == '\n':
			line, col = line+1, 0
		case r == '\t' || r == '\r':
		case r < 0x20 || r == 0x7f:
			return Refusef("standard input has the control character U+%04X at line %d, column %d: a shell's `echo` turns `\\f`, `\\a`, `\\b` and `\\v` into control characters (so `\\from` became a form feed and `rom`); pass the expression in a here-document (`<<'EOF'`) or with `printf '%%s\\n'`", r, line, col)
		}
	}
	return nil
}

// shellAddress is address as a shell argument: single-quoted when it holds a test's name.
func shellAddress(address string) string {
	if strings.ContainsAny(address, " \"") {
		return "'" + address + "'"
	}
	return address
}

// fillShape refuses what a hole cannot take for a reason `alder fill` can name: a lambda over
// the parameters the declaration binds already (when the hole is its whole body), and a block of
// a test (a `Task Bool`) in a hole that is not a Task, such as a generated test's `?expected`.
func fillShape(m *Module, h *Hole, expr syntax.Expr, hole string) error {
	if v := valueDecl(m.Syntax, h.Decl); v != nil && h.Arm == "" {
		if _, whole := v.Body.(*syntax.HoleExpr); whole {
			if err := boundLambda(expr, v); err != nil {
				return err
			}
		}
	}
	if _, isBlock := expr.(*syntax.Block); isBlock && strings.HasPrefix(h.Decl, `"`) && !strings.HasPrefix(h.Type, "Task ") {
		return Refusef("the hole ?%s is a `%s`, not a test's whole block: send the expression for it, or, to replace the whole test, use `alder fill --decl %s -`", h.Name, h.Type, shellAddress(h.Module+"."+h.Decl))
	}
	return nil
}

// boundLambda refuses expr when it is a lambda over parameters that value declaration v binds
// already (`load params session = …` filled with `\params session -> …`).
func boundLambda(expr syntax.Expr, v *syntax.ValueDecl) error {
	l, ok := expr.(*syntax.Lambda)
	if !ok || len(l.Params) == 0 {
		return nil
	}
	bound := map[string]bool{}
	for _, b := range v.Params {
		bound[b.Name] = true
	}
	var names []string
	for _, a := range l.Params {
		if !bound[a.Name] {
			return nil
		}
		names = append(names, a.Name)
	}
	list := strings.Join(names, " ")
	return Refusef("the parameters `%s` are bound already: send the body without `\\%s ->`", list, list)
}
