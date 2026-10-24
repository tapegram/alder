package generate

import (
	"fmt"
	"slices"
	"strings"
	"unicode"

	"alder/internal/format"
)

// Formatted is body (the text of path, an Alder file a generator writes) as `alder fmt` prints
// it. When it cannot be formatted (it does not parse, or the formatter's safety check fails) it
// is body unchanged and a warning, which the command prints on standard error after its name:
// the generator still writes the file, and the tests catch the bug.
func Formatted(path, body string) (out string, warning string) {
	b, diags, err := format.Source(path, []byte(body))
	switch {
	case len(diags) > 0:
		return body, notFormatted(fmt.Sprintf("%s:%d:%d: %s", path, diags[0].Span.Start.Line, diags[0].Span.Start.Col, diags[0].Message))
	case err != nil:
		return body, notFormatted(err.Error())
	}
	return string(b), ""
}

// notFormatted is the warning of a file written unformatted, for reason.
func notFormatted(reason string) string {
	return "output not formatted: " + reason + "; please file an issue"
}

// importSet collects the modules a generated module imports.
type importSet struct {
	home string          // the module being written: never imported
	std  map[string]bool // the standard library's modules: never imported
	set  map[string]bool
}

func newImports(home string, std map[string]bool) *importSet {
	return &importSet{home: home, std: std, set: map[string]bool{}}
}

// add imports module m, unless it is the home module, a standard-library module or "".
func (s *importSet) add(m string) {
	if m != "" && m != s.home && m != "Prelude" && !s.std[m] {
		s.set[m] = true
	}
}

// list is the imports, sorted.
func (s *importSet) list() []string {
	var out []string
	for m := range s.set {
		out = append(out, m)
	}
	slices.Sort(out)
	return out
}

// lines is the import lines, sorted, each ending in a newline.
func (s *importSet) lines() string {
	var b strings.Builder
	for _, m := range s.list() {
		b.WriteString("import " + m + "\n")
	}
	return b.String()
}

// Humanize turns an identifier into words for a label: "passwordHash" → "Password hash",
// "NewPost" → "New post", "Posts.Index" → "Posts index".
func Humanize(name string) string {
	var words []string
	var cur []rune
	rs := []rune(strings.ReplaceAll(name, ".", " "))
	flush := func() {
		if len(cur) > 0 {
			words = append(words, strings.ToLower(string(cur)))
			cur = nil
		}
	}
	for i, r := range rs {
		switch {
		case r == ' ' || r == '_':
			flush()
		case unicode.IsUpper(r) && i > 0 && (unicode.IsLower(rs[i-1]) || unicode.IsDigit(rs[i-1])):
			flush()
			cur = append(cur, r)
		default:
			cur = append(cur, r)
		}
	}
	flush()
	if len(words) == 0 {
		return ""
	}
	s := strings.Join(words, " ")
	return strings.ToUpper(s[:1]) + s[1:]
}

// LowerFirst is s with its first letter lower-cased.
func LowerFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}

// UpperFirst is s with its first letter upper-cased.
func UpperFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// Plural is the English plural of a lower-case word, for a field naming a list of rows:
// "post" → "posts", "category" → "categories", "box" → "boxes".
func Plural(w string) string {
	switch {
	case strings.HasSuffix(w, "s"), strings.HasSuffix(w, "x"), strings.HasSuffix(w, "sh"), strings.HasSuffix(w, "ch"):
		return w + "es"
	case strings.HasSuffix(w, "y") && len(w) > 1 && !strings.ContainsRune("aeiou", rune(w[len(w)-2])):
		return w[:len(w)-1] + "ies"
	}
	return w + "s"
}

// Title is the heading of a page module: its last segment, or for `….Index` the one before,
// in words ("Posts.Index" → "Posts", "Admin.UserList" → "User list").
func Title(module string) string {
	segs := strings.Split(module, ".")
	last := segs[len(segs)-1]
	if last == "Index" && len(segs) > 1 {
		last = segs[len(segs)-2]
	}
	return Humanize(last)
}

// indent prefixes every non-empty line of text with prefix.
func indent(text, prefix string) string {
	lines := strings.Split(text, "\n")
	for i, l := range lines {
		if l != "" {
			lines[i] = prefix + l
		}
	}
	return strings.Join(lines, "\n")
}

// Snake is an identifier in lower case with `_` between its words: "publishedAt" →
// "published_at" (a file or migration name).
func Snake(name string) string { return strings.ReplaceAll(strings.ToLower(Humanize(name)), " ", "_") }
