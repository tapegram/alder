package generate

import (
	"slices"
	"strings"

	"alder/internal/syntax"
)

// srcEdit collects whole-line insertions into a module's source, positioned by the lines of
// the source as it is (1-based): every position refers to the original text, however many
// lines are inserted above it.
type srcEdit struct {
	lines []string
	ins   []lineInsert
	repl  map[int]*string // line → its replacement; nil deletes it
}

type lineInsert struct {
	before int // insert before this line; len(lines)+1 appends
	seq    int
	text   []string
}

func newSrcEdit(src string) *srcEdit {
	return &srcEdit{lines: strings.Split(strings.TrimSuffix(src, "\n"), "\n"), repl: map[int]*string{}}
}

// replaceLine replaces line n with text.
func (e *srcEdit) replaceLine(n int, text string) { e.repl[n] = &text }

// deleteLine removes line n.
func (e *srcEdit) deleteLine(n int) { e.repl[n] = nil }

// insertBefore inserts text (lines separated by "\n") before line n. Insertions before the
// same line keep the order they were made in.
func (e *srcEdit) insertBefore(n int, text string) {
	e.ins = append(e.ins, lineInsert{before: n, seq: len(e.ins), text: strings.Split(text, "\n")})
}

// appendText adds text at the end of the file.
func (e *srcEdit) appendText(text string) { e.insertBefore(len(e.lines)+1, text) }

// line is line n (1-based) of the original source.
func (e *srcEdit) line(n int) string {
	if n < 1 || n > len(e.lines) {
		return ""
	}
	return e.lines[n-1]
}

// result is the edited source, with a final newline.
func (e *srcEdit) result() string {
	ins := slices.Clone(e.ins)
	slices.SortStableFunc(ins, func(a, b lineInsert) int {
		if a.before != b.before {
			return a.before - b.before
		}
		return a.seq - b.seq
	})
	var out []string
	k := 0
	for n := 1; n <= len(e.lines)+1; n++ {
		for k < len(ins) && ins[k].before == n {
			out = append(out, ins[k].text...)
			k++
		}
		if n > len(e.lines) {
			continue
		}
		if r, ok := e.repl[n]; ok {
			if r != nil {
				out = append(out, *r)
			}
			continue
		}
		out = append(out, e.lines[n-1])
	}
	return strings.Join(out, "\n") + "\n"
}

// indentOf is the leading spaces of line l.
func indentOf(l string) string { return l[:len(l)-len(strings.TrimLeft(l, " "))] }

// addImports inserts `import M` for each module of want that m does not import yet: in its
// sorted place when m's imports are sorted, else after the last; after its `module` line when
// it has none.
func addImports(e *srcEdit, m *syntax.Module, want []string) {
	have := map[string]bool{}
	last := 0
	for _, im := range m.Imports {
		have[im.Module] = true
		last = max(last, im.Span.End.Line)
	}
	var add []string
	for _, w := range want {
		if !have[w] && w != m.Name {
			add = append(add, "import "+w)
		}
	}
	if len(add) == 0 {
		return
	}
	slices.Sort(add)
	sorted := slices.IsSortedFunc(m.Imports, func(a, b *syntax.Import) int { return strings.Compare(a.Module, b.Module) })
	if last != 0 && sorted { // each in its place
		for _, a := range add {
			at := last + 1
			for _, im := range m.Imports {
				if "import "+im.Module > a {
					at = im.Span.Start.Line
					break
				}
			}
			e.insertBefore(at, a)
		}
		return
	}
	if last == 0 {
		mod := 1
		for n := 1; n <= len(e.lines); n++ {
			if strings.HasPrefix(e.line(n), "module ") {
				mod = n
				break
			}
		}
		e.insertBefore(mod+1, "\n"+strings.Join(add, "\n"))
		return
	}
	e.insertBefore(last+1, strings.Join(add, "\n"))
}

// declStart is the first line of declaration d: its signature's, for a value with one, and any
// comment lines right above it.
func declStart(e *srcEdit, d syntax.Decl) int {
	n := d.GetSpan().Start.Line
	if v, ok := d.(*syntax.ValueDecl); ok && v.Sig != nil {
		n = min(n, v.Sig.GetSpan().Start.Line)
	}
	for n > 1 && strings.HasPrefix(strings.TrimSpace(e.line(n-1)), "--") {
		n--
	}
	return n
}

// valueDecl is module m's value declaration name, or nil.
func valueDecl(m *syntax.Module, name string) *syntax.ValueDecl {
	for _, d := range m.Decls {
		if v, ok := d.(*syntax.ValueDecl); ok && v.Name == name {
			return v
		}
	}
	return nil
}

// declared reports whether module m declares a type, table, form, message, capability or
// constructor named name.
func declared(m *syntax.Module, name string) bool {
	for _, d := range m.Decls {
		switch d := d.(type) {
		case *syntax.TypeDecl:
			if d.Name == name {
				return true
			}
			for _, c := range d.Union {
				if c.Name == name {
					return true
				}
			}
		case *syntax.TableDecl:
			if d.Name == name {
				return true
			}
		case *syntax.FormDecl:
			if d.Name == name {
				return true
			}
		case *syntax.MsgDecl:
			if d.Name == name {
				return true
			}
		case *syntax.CapDecl:
			if d.Name == name {
				return true
			}
		}
	}
	return false
}

// testNamed reports whether module m has a test named name.
func testNamed(m *syntax.Module, name string) bool {
	for _, d := range m.Decls {
		if t, ok := d.(*syntax.TestDecl); ok && t.Name == name {
			return true
		}
	}
	return false
}

// viewSpot is where a generated form or button goes in a page's `view`: before line Line (the
// closing tag of the last `al-card` the root element holds, through elements only, or else of
// the root element itself), indented by Indent; InCard says which.
type viewSpot struct {
	Line   int
	Indent string
	InCard bool
	// Head and Tail split the line of an element written on one line (as `alder fmt` writes
	// `<Layout.Page title="Notes"><h1>Notes</h1></Layout.Page>`) before its closing tag: the
	// markup then goes between them, on lines of its own, before Line+1.
	Head, Tail string
}

// insert puts text (lines indented at the spot) at the spot.
func (s viewSpot) insert(e *srcEdit, text string) {
	if s.Tail == "" {
		e.insertBefore(s.Line, text)
		return
	}
	e.replaceLine(s.Line, s.Head)
	e.insertBefore(s.Line+1, text)
	e.insertBefore(s.Line+1, strings.TrimSuffix(s.Indent, "  ")+s.Tail) // the closing tag, at the element's indentation
}

// findViewSpot finds the spot in module m's `view` (with rootOnly, the end of the root element,
// whatever cards it holds). The view must be one element; written on one line, the spot splits
// it before its closing tag.
func findViewSpot(e *srcEdit, m *syntax.Module, rootOnly bool) (viewSpot, bool) {
	v := valueDecl(m, "view")
	if v == nil {
		return viewSpot{}, false
	}
	root, ok := v.Body.(*syntax.Element)
	if !ok {
		return viewSpot{}, false
	}
	var cards []*syntax.Element
	var walk func(el *syntax.Element)
	walk = func(el *syntax.Element) {
		for _, c := range el.Children {
			if ce, ok := c.(*syntax.Element); ok {
				if ce.Tag == "al-card" {
					cards = append(cards, ce)
				}
				walk(ce)
			}
		}
	}
	walk(root)
	target, inCard := root, false
	if len(cards) > 0 && !rootOnly {
		target, inCard = cards[len(cards)-1], true
	}
	close := target.Span.End.Line
	l := e.line(close)
	if close == target.Span.Start.Line { // on one line: split it before the closing tag
		end := min(max(target.Span.End.Col-1, 0), len(l))
		tag := target.Tag
		if target.Module != "" {
			tag = target.Module + "." + tag
		}
		at := strings.LastIndex(l[:end], "</"+tag)
		if at <= len(indentOf(l)) {
			return viewSpot{}, false
		}
		ind := indentOf(l)
		if !strings.HasPrefix(l[len(ind):], "<") { // `view model session = <…>…</…>`: the element continues the line
			ind += "  "
		}
		return viewSpot{Line: close, Indent: ind + "  ", InCard: inCard, Head: l[:at], Tail: l[at:]}, true
	}
	if !strings.HasPrefix(strings.TrimSpace(l), "</") {
		return viewSpot{}, false
	}
	return viewSpot{Line: close, Indent: indentOf(l) + "  ", InCard: inCard}, true
}

// updateArmSpot is the line before which a new arm of m's `update` goes (the `}` closing its
// `match msg { … }`) and the arms' indentation; ok is false when m has no `update` or it is not
// such a match.
func updateArmSpot(e *srcEdit, m *syntax.Module) (line int, ind string, has, ok bool) {
	v := valueDecl(m, "update")
	if v == nil {
		return 0, "", false, true
	}
	mt, isMatch := v.Body.(*syntax.Match)
	if !isMatch {
		return 0, "", true, false
	}
	close := mt.Span.End.Line
	l := e.line(close)
	if strings.TrimSpace(l) != "}" || close == mt.Span.Start.Line {
		return 0, "", true, false
	}
	return close, indentOf(l) + "  ", true, true
}
