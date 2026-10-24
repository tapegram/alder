package generate

import (
	"slices"
	"strings"

	"alder/internal/diag"
	"alder/internal/names"
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

// current is line n as replaced so far (the original when it is not).
func (e *srcEdit) current(n int) string {
	if r, ok := e.repl[n]; ok && r != nil {
		return *r
	}
	return e.line(n)
}

// insertAt inserts text into line n before its byte column col (1-based, as a span's), in the
// line as replaced so far: several insertions into one line go right to left.
func (e *srcEdit) insertAt(n, col int, text string) {
	l := e.current(n)
	c := min(max(col-1, 0), len(l))
	e.replaceLine(n, l[:c]+text+l[c:])
}

// appendItem adds item at the end of the items of a `{ … }` (a record type's fields, a page's
// parameters, a silo's tables) whose span is braces and whose items' spans are items, in
// whatever layout the braces are written: after the last item, or inside empty braces. Apply's
// formatting then lays it out.
func appendItem(e *srcEdit, braces diag.Span, items []diag.Span, item string) {
	if len(items) == 0 {
		e.insertAt(braces.Start.Line, braces.Start.Col+1, " "+item)
		return
	}
	last := items[len(items)-1]
	e.insertAt(last.End.Line, last.End.Col, ", "+item)
}

// fieldSpans are the spans of fs.
func fieldSpans(fs []*syntax.Field) []diag.Span {
	var out []diag.Span
	for _, f := range fs {
		out = append(out, f.Span)
	}
	return out
}

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

// addImports inserts the import of each module of want that m does not import yet: in its
// sorted place when m's imports are sorted, else after the last; after its `module` line when
// it has none. Each import is written with its canonical qualifier (design rule 3): `import P`,
// or `import P as Q` when Q is not P's last segment. An addition that changes an existing
// import's qualifier rewrites that import's line, and its references in the file, to the new
// one; text the generator wrote with a module's full path (`Admin.Layout.isAdmin`) comes out
// with its qualifier (`Layout.isAdmin`), a page value (`Admin.Layout {}`) as it is. It returns
// the qualifier of every import of m, after the additions.
func addImports(e *srcEdit, p *Program, m *syntax.Module, want []string) map[string]string {
	return retarget(e, p, m, want, false)
}

// retarget is addImports; with force it also rewrites every import line that is not written in
// its canonical form (a generated file, whose imports a generator wrote as `import P`).
func retarget(e *srcEdit, p *Program, m *syntax.Module, want []string, force bool) map[string]string {
	have := map[string]bool{}
	last := 0
	var oldPaths []string
	for _, im := range m.Imports {
		have[im.Module] = true
		last = max(last, im.Span.End.Line)
		if !p.Std[im.Module] {
			oldPaths = append(oldPaths, im.Module)
		}
	}
	var add []string
	paths := slices.Clone(oldPaths)
	for _, w := range want {
		if !have[w] && w != m.Name && !slices.Contains(add, w) {
			add = append(add, w)
			if !p.Std[w] {
				paths = append(paths, w)
			}
		}
	}
	taken := names.Taken(m, func(n string) bool { return p.Std[n] })
	quals := canonicalQuals(m.Name, paths, taken)
	was := canonicalQuals(m.Name, oldPaths, taken)
	for _, im := range m.Imports { // a standard-library module keeps its name
		if p.Std[im.Module] {
			quals[im.Module], was[im.Module] = im.Module, im.Module
		}
	}
	for _, w := range add {
		if p.Std[w] {
			quals[w] = w
		}
	}
	// the references to rewrite: a module's full path, and a qualifier that changed
	rename, bare := map[string]string{}, map[string]string{}
	for _, im := range m.Imports {
		if p.Std[im.Module] {
			continue
		}
		q := quals[im.Module]
		if was[im.Module] != q {
			rename[was[im.Module]] = q
			if was[im.Module] != im.Module { // a proper suffix of the path, so no page value
				bare[was[im.Module]] = q
			}
			e.replaceLine(im.Span.Start.Line, importLine(im.Module, q))
		} else if force && e.current(im.Span.Start.Line) != importLine(im.Module, q) {
			e.replaceLine(im.Span.Start.Line, importLine(im.Module, q))
		}
	}
	for _, path := range paths {
		if quals[path] != path {
			rename[path] = quals[path]
		}
	}
	e.requalify(rename, bare)
	if len(add) == 0 {
		return quals
	}
	slices.Sort(add)
	sorted := slices.IsSortedFunc(m.Imports, func(a, b *syntax.Import) int { return strings.Compare(a.Module, b.Module) })
	if last != 0 && sorted { // each in its place
		for _, a := range add {
			at := last + 1
			for _, im := range m.Imports {
				if im.Module > a {
					at = im.Span.Start.Line
					break
				}
			}
			e.insertBefore(at, importLine(a, quals[a]))
		}
		return quals
	}
	var lines []string
	for _, a := range add {
		lines = append(lines, importLine(a, quals[a]))
	}
	if last == 0 {
		mod := 1
		for n := 1; n <= len(e.lines); n++ {
			if strings.HasPrefix(e.line(n), "module ") {
				mod = n
				break
			}
		}
		e.insertBefore(mod+1, "\n"+strings.Join(lines, "\n"))
		return quals
	}
	e.insertBefore(last+1, strings.Join(lines, "\n"))
	return quals
}

// canonicalQuals is names.CanonicalQualifiers, with a path that has no usable qualifier (E0206)
// written with its full path.
func canonicalQuals(self string, paths []string, taken func(string) bool) map[string]string {
	quals, bad := names.CanonicalQualifiers(self, paths, taken)
	for _, b := range bad {
		quals[b] = b
	}
	return quals
}

// importLine is the import of path written with qualifier q: `import P`, or `import P as Q`
// when Q is not P's last segment.
func importLine(path, q string) string {
	if names.NeedsAlias(path, q) {
		return "import " + path + " as " + q
	}
	return "import " + path
}

// qualified is name (a member, or "" for the module itself) of the module at path, written with
// its qualifier from quals (addImports' result).
func qualified(quals map[string]string, path, name string) string {
	q, ok := quals[path]
	if !ok {
		q = path
	}
	if name == "" {
		return q
	}
	return q + "." + name
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
