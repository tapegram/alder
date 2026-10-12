// Package selector parses the CSS selector subset Alder's browser tests query documents with
// (Test.text, Test.has, …): a tag, `#id`, `.class`, `[attr]` and `[attr=value]`, combined into a
// compound (`li.done`), and compounds combined by descendant (`ul li`) or child (`ul > li`). It
// is shared by the checker (E0023 for a selector literal that does not parse) and by rt/html,
// which matches a parsed Selector against a document.
package selector

import (
	"strings"
)

// Selector is a parsed selector: one or more compound steps, left to right. Steps[0].Comb is
// Descendant and means nothing; every later step's Comb relates it to the step before.
type Selector struct {
	Steps []Step
}

// Comb is how a step relates to the step before it.
type Comb byte

const (
	// Descendant: the element is anywhere inside an element matching the step before (`ul li`).
	Descendant Comb = ' '
	// Child: the element's parent matches the step before (`ul > li`).
	Child Comb = '>'
)

// Step is one compound selector and its combinator.
type Step struct {
	Comb    Comb
	Tag     string // "" for any element
	ID      string // "" for none
	Classes []string
	Attrs   []Attr
}

// Attr is `[name]` (HasValue false) or `[name=value]`.
type Attr struct {
	Name, Value string
	HasValue    bool
}

// Subset is the description of the grammar every Error ends with.
const Subset = "a selector is a tag, `#id`, `.class`, `[attr]`, `[attr=value]`, combined with spaces or `>`"

// Error is why a text is not a selector. Token is the offending token ("" at the end of the
// input) and After the selector text before it, trimmed ("" when the token starts it).
type Error struct {
	Input  string
	Token  string
	After  string
	Reason string // set instead of Token/After for an empty selector
	// Hint, when set, says what was probably meant: "tags are lower case: `h1`", "`x` is not an
	// HTML element (a custom element has a hyphen)", "quote the value: `[href="/x"]`".
	Hint string
}

// Error is "`#sumary x` is not a selector: unexpected `x` after `#sumary` (<Subset>)", with
// "; <Hint>" before the parenthesis when there is a hint.
func (e *Error) Error() string {
	var why string
	switch {
	case e.Reason != "":
		why = e.Reason
	case e.Token == "" && e.After == "":
		why = "it is empty"
	case e.Token == "":
		why = "unexpected end after `" + e.After + "`"
	case e.After == "":
		why = "unexpected `" + e.Token + "` at the start"
	default:
		why = "unexpected `" + e.Token + "` after `" + e.After + "`"
	}
	if e.Hint != "" {
		why += "; " + e.Hint
	}
	return "`" + e.Input + "` is not a selector: " + why + " (" + Subset + ")"
}

// String is the selector written back in canonical form (`ul > li.done[data-x="1"]`).
func (s Selector) String() string {
	var b strings.Builder
	for i, st := range s.Steps {
		if i > 0 {
			if st.Comb == Child {
				b.WriteString(" > ")
			} else {
				b.WriteByte(' ')
			}
		}
		b.WriteString(st.Tag)
		if st.ID != "" {
			b.WriteString("#" + st.ID)
		}
		for _, c := range st.Classes {
			b.WriteString("." + c)
		}
		for _, a := range st.Attrs {
			b.WriteString("[" + a.Name)
			if a.HasValue {
				b.WriteString(`="` + a.Value + `"`)
			}
			b.WriteByte(']')
		}
	}
	return b.String()
}

// MustParse is Parse for a text known to be a selector (a literal the checker accepted); it
// panics otherwise.
func MustParse(s string) Selector {
	sel, err := Parse(s)
	if err != nil {
		panic(err)
	}
	return sel
}

// Parse parses s as a selector of the subset. A tag must be an HTML element name (or a custom
// element's, which has a hyphen), lower case. An id, class or attribute name is letters, digits,
// `-`, `_` (an attribute name also `:` and `.`, as Datastar's `data-on:click` has); attribute
// names are case-insensitive (`[DATA-X]` is `[data-x]`, as rt/html.Parse reads them), values
// are not. An attribute value is an identifier or a quoted string ("…" or '…', no escapes).
func Parse(s string) (Selector, error) {
	p := &parser{in: s}
	return p.parse()
}

type parser struct {
	in  string
	pos int
}

func (p *parser) fail(start, end int) error {
	tok := ""
	if start < len(p.in) {
		tok = p.in[start:min(end, len(p.in))]
	}
	return &Error{Input: p.in, Token: tok, After: strings.TrimSpace(p.in[:start])}
}

func (p *parser) skipSpace() bool {
	start := p.pos
	for p.pos < len(p.in) && isSpace(p.in[p.pos]) {
		p.pos++
	}
	return p.pos > start
}

func (p *parser) parse() (Selector, error) {
	var sel Selector
	p.skipSpace()
	if p.pos == len(p.in) {
		return sel, &Error{Input: p.in}
	}
	comb := Descendant
	for {
		st, err := p.compound()
		if err != nil {
			return Selector{}, err
		}
		st.Comb = comb
		sel.Steps = append(sel.Steps, st)
		spaced := p.skipSpace()
		if p.pos == len(p.in) {
			return sel, nil
		}
		switch {
		case p.in[p.pos] == '>':
			p.pos++
			p.skipSpace()
			if p.pos == len(p.in) {
				return Selector{}, p.fail(p.pos, p.pos)
			}
			comb = Child
		case spaced:
			comb = Descendant
		default:
			return Selector{}, p.fail(p.pos, p.pos+tokenLen(p.in[p.pos:]))
		}
	}
}

// compound parses one compound step at p.pos.
func (p *parser) compound() (Step, error) {
	var st Step
	start := p.pos
	if n := identLen(p.in[p.pos:], false); n > 0 {
		tag := p.in[p.pos : p.pos+n]
		if !knownTag(tag) {
			err := p.fail(p.pos, p.pos+n).(*Error)
			if lower := strings.ToLower(tag); knownTag(lower) {
				err.Hint = "tags are lower case: `" + lower + "`"
			} else {
				err.Hint = "`" + tag + "` is not an HTML element (a custom element has a hyphen)"
			}
			return st, err
		}
		st.Tag = tag
		p.pos += n
	}
	for p.pos < len(p.in) {
		c := p.in[p.pos]
		switch c {
		case '#', '.':
			n := identLen(p.in[p.pos+1:], false)
			if n == 0 {
				return st, p.fail(p.pos, p.pos+1+tokenLen(p.in[p.pos+1:]))
			}
			name := p.in[p.pos+1 : p.pos+1+n]
			if c == '#' {
				if st.ID != "" {
					return st, p.fail(p.pos, p.pos+1+n)
				}
				st.ID = name
			} else {
				st.Classes = append(st.Classes, name)
			}
			p.pos += 1 + n
		case '[':
			a, err := p.attr()
			if err != nil {
				return st, err
			}
			st.Attrs = append(st.Attrs, a)
		default:
			if isSpace(c) || c == '>' {
				if p.pos == start {
					return st, p.fail(p.pos, p.pos+1)
				}
				return st, nil
			}
			return st, p.fail(p.pos, p.pos+tokenLen(p.in[p.pos:]))
		}
	}
	return st, nil
}

// attr parses `[name]` or `[name=value]` at p.pos (which is at '[').
func (p *parser) attr() (Attr, error) {
	var a Attr
	p.pos++
	n := identLen(p.in[p.pos:], true)
	if n == 0 {
		return a, p.fail(p.pos, p.pos+tokenLen(p.in[p.pos:]))
	}
	a.Name = strings.ToLower(p.in[p.pos : p.pos+n]) // attribute names are case-insensitive
	p.pos += n
	if p.pos < len(p.in) && p.in[p.pos] == '=' {
		p.pos++
		a.HasValue = true
		valueStart := p.pos
		if p.pos < len(p.in) && (p.in[p.pos] == '"' || p.in[p.pos] == '\'') {
			q := p.in[p.pos]
			end := strings.IndexByte(p.in[p.pos+1:], q)
			if end < 0 {
				return a, p.fail(p.pos, len(p.in))
			}
			a.Value = p.in[p.pos+1 : p.pos+1+end]
			p.pos += end + 2
		} else {
			m := identLen(p.in[p.pos:], false)
			if m == 0 || p.pos+m < len(p.in) && p.in[p.pos+m] != ']' {
				return a, p.unquoted(a.Name, valueStart, p.pos+m)
			}
			a.Value = p.in[p.pos : p.pos+m]
			p.pos += m
		}
	}
	if p.pos >= len(p.in) {
		return a, p.fail(len(p.in), len(p.in))
	}
	if p.in[p.pos] != ']' {
		return a, p.fail(p.pos, p.pos+tokenLen(p.in[p.pos:]))
	}
	p.pos++
	return a, nil
}

// unquoted is the error of an attribute value at start that is not an identifier, failing at
// at: with the hint to quote it, up to the next `]` (`[href=/x]`: "quote the value:
// `[href="/x"]`").
func (p *parser) unquoted(name string, start, at int) error {
	err := p.fail(at, at+tokenLen(p.in[at:])).(*Error)
	if end := strings.IndexByte(p.in[start:], ']'); end > 0 && !strings.ContainsAny(p.in[start:start+end], `"'`) {
		err.Hint = "quote the value: `[" + name + `="` + p.in[start:start+end] + `"]` + "`"
	}
	return err
}

func isSpace(c byte) bool { return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' }

// identLen is the length of the identifier at the start of s: letters, digits, '-' and '_'
// (and ':' and '.' for an attribute name).
func identLen(s string, attrName bool) int {
	i := 0
	for i < len(s) {
		c := s[i]
		ok := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c >= 0x80
		if attrName && (c == ':' || c == '.') {
			ok = true
		}
		if !ok {
			break
		}
		i++
	}
	return i
}

// tokenLen is the length of the token at the start of s an error names: an identifier, else
// one character (a whole UTF-8 sequence).
func tokenLen(s string) int {
	if n := identLen(s, false); n > 0 {
		return n
	}
	if s == "" {
		return 0
	}
	n := 1
	for n < len(s) && s[n]&0xC0 == 0x80 {
		n++
	}
	return n
}

// knownTag reports whether tag is an HTML element name (lower case) or a custom element's
// (one with a hyphen).
func knownTag(tag string) bool {
	if strings.Contains(tag, "-") {
		return tag == strings.ToLower(tag) && tag[0] >= 'a' && tag[0] <= 'z'
	}
	return strings.Contains(strings.ReplaceAll(htmlTags, "\n", " "), " "+tag+" ")
}

// htmlTags are the HTML element names (and a few of SVG's and MathML's), space-separated and
// between spaces: a string, not a map, so the package has no init work in a production binary,
// which links rt/html (and so this package) without ever parsing a selector.
const htmlTags = ` a abbr address area article aside audio b base bdi bdo blockquote body br button
canvas caption cite code col colgroup data datalist dd del details dfn dialog div dl dt em embed
fieldset figcaption figure footer form h1 h2 h3 h4 h5 h6 head header hgroup hr html i iframe img
input ins kbd label legend li link main map mark menu meta meter nav noscript object ol optgroup
option output p param picture pre progress q rp rt ruby s samp script search section select slot
small source span strong style sub summary sup svg table tbody td template textarea tfoot th
thead time title tr track u ul var video wbr math path g circle rect line polyline polygon `
