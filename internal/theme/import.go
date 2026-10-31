package theme

import (
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
)

// Unplaced is a declaration the importer could not put in a Theme, with why. Block is the
// selector text as written (`:root, .light`); for a rule inside `@media (prefers-color-scheme:
// dark)` it is `@media (prefers-color-scheme: dark) { :root }`, so the CSS stays faithful.
type Unplaced struct{ Block, Name, Value, Reason string }

// Skipped counts the at-rules the importer dropped silently (`@theme`, `@custom-variant`, …):
// N of them, and Kinds names each kind once, in source order (`@theme inline`).
type Skipped struct {
	N     int
	Kinds []string
}

// NoThemeError is the error Import returns when the input holds no theme. A caller that read
// the CSS from a file sets Name to its name, so the message names it.
type NoThemeError struct{ Name string }

func (e *NoThemeError) Error() string {
	in := "the input"
	if e.Name != "" {
		in = "`" + e.Name + "`"
	}
	return "no theme in " + in + ": expected a `:root { --primary: … }` or `.dark { … }` block"
}

// Import reads a CSS export (shadcn v3 or v4, tweakcn, a hand-written `:root` and `.dark`) into
// a Theme, the declarations it could not place and the at-rules it skipped. It is an error
// (*NoThemeError) when no token of alder-ui was found. It is pure: the caller reads the file.
func Import(css string) (Theme, []Unplaced, Skipped, error) {
	im := &importer{
		th: Theme{Root: map[string]string{}, Light: map[string]string{}, Dark: map[string]string{}},
	}
	im.walk(css, false)
	im.resolveDark()
	sort.SliceStable(im.un, func(i, j int) bool { return im.un[i].seq < im.un[j].seq })
	un := make([]Unplaced, len(im.un))
	for i, u := range im.un {
		un[i] = u.Unplaced
	}
	if im.th.Empty() {
		return im.th, un, im.sk, &NoThemeError{}
	}
	return im.th, un, im.sk, nil
}

type seqUnplaced struct {
	Unplaced
	seq int
}

// pendingDark is a theme-wide token seen in a dark block: it is accepted only when root's value
// is the same, which is known once the whole input has been read.
type pendingDark struct {
	field string
	val   string
	u     Unplaced
	seq   int
}

type importer struct {
	th      Theme
	un      []seqUnplaced
	sk      Skipped
	pending []pendingDark
	seq     int
}

type decl struct{ name, value string }

func (im *importer) unplace(block, name, value, reason string) {
	im.seq++
	im.un = append(im.un, seqUnplaced{Unplaced{block, name, value, reason}, im.seq})
}

// skipKeywords are the at-rules that carry no theme: Tailwind's and its build's.
var skipKeywords = map[string]bool{
	"theme": true, "import": true, "tailwind": true, "plugin": true, "custom-variant": true,
	"source": true, "utility": true, "variant": true, "config": true, "reference": true,
}

var reDarkMedia = regexp.MustCompile(`prefers-color-scheme\s*:\s*dark`)

func (im *importer) walk(css string, darkMedia bool) {
	for _, n := range nodes(stripComments(css)) {
		if strings.HasPrefix(n.prelude, "@") {
			kw, rest := atKeyword(n.prelude)
			switch {
			case skipKeywords[kw]:
				kind := "@" + kw
				if kw == "theme" {
					if f := strings.Fields(rest); len(f) > 0 {
						kind += " " + f[0]
					}
				}
				im.sk.N++
				if !slices.Contains(im.sk.Kinds, kind) {
					im.sk.Kinds = append(im.sk.Kinds, kind)
				}
			case n.hasBody && (kw == "media" || kw == "layer" || kw == "supports" || kw == "container"):
				im.walk(n.body, darkMedia || kw == "media" && reDarkMedia.MatchString(rest))
			}
			continue
		}
		if !n.hasBody {
			continue
		}
		im.rule(n.prelude, declarations(n.body), darkMedia)
	}
}

func atKeyword(prelude string) (kw, rest string) {
	s := prelude[1:]
	i := strings.IndexAny(s, " \t\r\n(")
	if i < 0 {
		return strings.ToLower(s), ""
	}
	return strings.ToLower(s[:i]), strings.TrimSpace(s[i:])
}

var (
	reDarkSel  = regexp.MustCompile(`\.dark($|[^\w-])|\[\s*data-theme\s*=\s*["']?dark["']?\s*\]`)
	reLightSel = regexp.MustCompile(`\.light($|[^\w-])|\[\s*data-theme\s*=\s*["']?light["']?\s*\]`)
	reRootSel  = regexp.MustCompile(`:root|(^|[\s>+~])html($|[^\w-])`)
)

func (im *importer) rule(selector string, decls []decl, darkMedia bool) {
	var props []decl
	index := map[string]int{}
	for _, d := range decls {
		if !strings.HasPrefix(d.name, "--") {
			continue
		}
		if i, ok := index[d.name]; ok { // the last declaration of a name wins
			props[i].value = d.value
			continue
		}
		index[d.name] = len(props)
		props = append(props, d)
	}
	if len(props) == 0 {
		return
	}
	written := collapse(selector)
	block := written
	var light, dark bool
	for _, s := range splitTop(selector, ',') {
		switch {
		case reDarkSel.MatchString(s):
			dark = true
		case darkMedia && reRootSel.MatchString(s):
			dark = true
		case reLightSel.MatchString(s) || reRootSel.MatchString(s):
			light = true
		}
	}
	if darkMedia {
		block = "@media (prefers-color-scheme: dark) { " + written + " }"
	}
	if !light && !dark {
		for _, d := range props {
			im.unplace(block, d.name, clean(d.value), "`"+written+"` is not a scheme selector")
		}
		return
	}
	for _, d := range props {
		im.place(block, d, light, dark)
	}
}

func (im *importer) place(block string, d decl, light, dark bool) {
	value := clean(d.value)
	tok, ok := ByCSS(d.name)
	if !ok {
		if strings.HasPrefix(d.name, "--al-") {
			if why, removed := Removed[FieldName("--al-"+d.name[2:])]; removed {
				im.unplace(block, d.name, value, "`"+d.name+"` was removed in alder-ui v0.4: "+why)
				return
			}
		}
		im.unplace(block, d.name, value, "not a token of alder-ui "+Version())
		return
	}
	val, reason := check(tok, value)
	if reason != "" {
		im.unplace(block, d.name, value, reason)
		return
	}
	if tok.Kind == Colour {
		if light {
			im.th.Light[tok.Field] = val
		}
		if dark {
			im.th.Dark[tok.Field] = val
		}
		return
	}
	if light {
		im.th.Root[tok.Field] = val
	}
	if dark && !light {
		im.seq++
		p := pendingDark{tok.Field, val, Unplaced{block, d.name, value, "theme-wide in alder-ui; the dark value differs"}, im.seq}
		for i, q := range im.pending { // the last declaration across dark blocks wins
			if q.field == tok.Field {
				im.pending[i] = p
				return
			}
		}
		im.pending = append(im.pending, p)
	}
}

// resolveDark accepts the theme-wide tokens of dark blocks that match root's value and
// unplaces the rest.
func (im *importer) resolveDark() {
	for _, p := range im.pending {
		if v, ok := im.th.Root[p.field]; ok && v == p.val {
			continue
		}
		im.un = append(im.un, seqUnplaced{p.u, p.seq})
	}
}

var (
	reImportant = regexp.MustCompile(`(?i)\s*!\s*important\s*$`)
	reChannels  = regexp.MustCompile(`^-?[\d.]+(?:deg)?\s+[\d.]+%\s+[\d.]+%(?:\s*/\s*[\d.]+%?)?$`)
)

// clean is a declaration's value as written: `!important` dropped, whitespace collapsed.
func clean(v string) string {
	return collapse(reImportant.ReplaceAllString(strings.TrimSpace(v), ""))
}

func collapse(s string) string { return strings.Join(strings.Fields(s), " ") }

// check validates value against the token's kind. It returns the value to place (a font stack
// joined, bare HSL channels wrapped) or, when it fails, the reason.
func check(tok Token, value string) (string, string) {
	switch tok.Kind {
	case Colour:
		if reChannels.MatchString(value) { // shadcn v3: `222.2 47.4% 11.2%` for `hsl(var(--x))`
			value = "hsl(" + value + ")"
		}
		if name, ok := Alias(value); ok {
			if t, known := ByCSS(name); !known || t.Kind != Colour {
				return "", "`" + value + "` is not a colour token"
			}
			return value, ""
		}
		if !ColourOK(value) {
			return "", fmt.Sprintf("`%s: %s` is not a colour alder-ui accepts: `#4f46e5`, `oklch(0.55 0.22 285)`, `var(--accent)`", tok.CSS, value)
		}
	case Length:
		if !LengthOK(value) {
			return "", fmt.Sprintf("`%s: %s` is not a length alder-ui accepts: `0.5rem`, `8px`", tok.CSS, value)
		}
	case Font:
		fams := Families(value)
		ok := len(fams) > 0
		for _, f := range fams {
			ok = ok && FamilyOK(f) && !strings.Contains(f, "(")
		}
		if !ok {
			return "", fmt.Sprintf("`%s: %s` is not a font stack alder-ui accepts: family names such as `Inter, system-ui, sans-serif`", tok.CSS, value)
		}
		return strings.Join(fams, ", "), ""
	case Shadow:
		if !ShadowOK(value) {
			return "", fmt.Sprintf("`%s: %s` is not a shadow alder-ui accepts: `none` or `0 1px 2px 0 rgb(0 0 0 / 0.05)`", tok.CSS, value)
		}
	}
	return value, ""
}

// UnplacedCSS is the unplaced declarations as valid CSS, grouped by the block each came from in
// first-seen order, each with its reason as a comment: the text to paste into an app's own
// stylesheet.
func UnplacedCSS(us []Unplaced) string {
	var order []string
	groups := map[string][]Unplaced{}
	for _, u := range us {
		if _, ok := groups[u.Block]; !ok {
			order = append(order, u.Block)
		}
		groups[u.Block] = append(groups[u.Block], u)
	}
	var b strings.Builder
	for i, block := range order {
		if i > 0 {
			b.WriteString("\n")
		}
		indent, closing := "", ""
		sel := block
		if rest, ok := strings.CutPrefix(block, "@media "); ok {
			if j := strings.Index(rest, " { "); j >= 0 && strings.HasSuffix(rest, " }") {
				b.WriteString("@media " + rest[:j] + " {\n")
				sel = rest[j+3 : len(rest)-2]
				indent, closing = "  ", "}\n"
			}
		}
		b.WriteString(indent + sel + " {\n")
		for _, u := range groups[block] {
			reason := strings.ReplaceAll(u.Reason, "*/", "* /")
			fmt.Fprintf(&b, "%s  %s: %s; /* %s */\n", indent, u.Name, u.Value, reason)
		}
		b.WriteString(indent + "}\n" + closing)
	}
	return b.String()
}

// ---- the scanner

type node struct {
	prelude, body string
	hasBody       bool
}

// stripComments drops `/* … */` outside strings.
func stripComments(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c == '"' || c == '\'':
			j := endString(s, i)
			b.WriteString(s[i:j])
			i = j - 1
		case c == '/' && i+1 < len(s) && s[i+1] == '*':
			j := strings.Index(s[i+2:], "*/")
			if j < 0 {
				return b.String()
			}
			i += j + 3
			b.WriteByte(' ')
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// endString is the index just past the string literal starting at s[i].
func endString(s string, i int) int {
	q := s[i]
	for j := i + 1; j < len(s); j++ {
		switch s[j] {
		case '\\':
			j++
		case q:
			return j + 1
		}
	}
	return len(s)
}

// nodes reads the statements and blocks at the top level of s: a prelude ended by `;`, or by a
// `{ … }` body.
func nodes(s string) []node {
	var out []node
	i := 0
	for i < len(s) {
		start, depth := i, 0
	scan:
		for ; i < len(s); i++ {
			switch s[i] {
			case '"', '\'':
				i = endString(s, i) - 1
			case '(', '[':
				depth++
			case ')', ']':
				depth--
			case ';':
				if depth <= 0 {
					break scan
				}
			case '{':
				if depth <= 0 {
					break scan
				}
			}
		}
		prelude := strings.TrimSpace(s[start:min(i, len(s))])
		if i >= len(s) || s[i] == ';' {
			if prelude != "" {
				out = append(out, node{prelude: prelude})
			}
			i++
			continue
		}
		// s[i] is `{`: find its match.
		bodyStart, level := i+1, 1
		for i++; i < len(s) && level > 0; i++ {
			switch s[i] {
			case '"', '\'':
				i = endString(s, i) - 1
			case '{':
				level++
			case '}':
				level--
			}
		}
		end := i - 1
		if level > 0 {
			end = len(s)
		}
		out = append(out, node{prelude: prelude, body: s[bodyStart:max(bodyStart, end)], hasBody: true})
	}
	return out
}

// declarations are the `name: value` pairs at the top level of a rule's body; a nested rule is
// ignored.
func declarations(body string) []decl {
	var out []decl
	for _, piece := range splitTop(body, ';') {
		if strings.ContainsAny(piece, "{}") {
			continue
		}
		name, value, ok := strings.Cut(piece, ":")
		if !ok {
			continue
		}
		out = append(out, decl{strings.TrimSpace(name), strings.TrimSpace(value)})
	}
	return out
}

// splitTop splits s at sep outside strings, parentheses, brackets and braces.
func splitTop(s string, sep byte) []string {
	var out []string
	start, depth := 0, 0
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c == '"' || c == '\'':
			i = endString(s, i) - 1
		case c == '(' || c == '[' || c == '{':
			depth++
		case c == ')' || c == ']' || c == '}':
			depth--
		case c == sep && depth <= 0:
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	return append(out, s[start:])
}
