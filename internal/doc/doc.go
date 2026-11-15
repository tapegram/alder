// Package doc reads `--` doc comments, as syntax.DocOf returns them, into a summary, details,
// examples and a book line, and renders them as markdown. It is lenient: any text parses, and
// Problems is where the stdlib format is enforced.
package doc

import (
	"regexp"
	"strings"
)

type Example struct {
	Source string // its lines joined by "\n", four spaces removed from the front of each
	Line   int    // 0-based index of its first line in the text given to Parse
}

type Doc struct {
	Summary  string   // the first paragraph, its lines joined by a space; "" when the text starts with an example or a Book line
	Details  []string // the other prose paragraphs, in order, as written
	Examples []Example
	Book     string // the text after "Book: " (`chapter 9, "List"`); "" when absent

	paras []para // every paragraph in the order written, for Markdown
}

type paraKind int

const (
	prose paraKind = iota
	examples
	bookLine
)

type para struct {
	kind     paraKind
	text     string    // prose: the lines as written; bookLine: the text after "Book: "
	examples []Example // examples only
}

const indent = "    "

// Parse splits text into paragraphs at blank lines. A paragraph whose every line starts with four
// spaces is examples, a last one-line paragraph starting "Book: " is the book line, and anything
// else is prose.
func Parse(text string) Doc {
	var d Doc
	lines := strings.Split(text, "\n")
	for start := 0; start < len(lines); {
		if blank(lines[start]) {
			start++
			continue
		}
		end := start
		for end < len(lines) && !blank(lines[end]) {
			end++
		}
		d.paras = append(d.paras, classify(lines[start:end], start))
		start = end
	}
	// Only the last paragraph can be the book line; an earlier one is prose.
	for i := range d.paras {
		p := &d.paras[i]
		if p.kind == bookLine && i != len(d.paras)-1 {
			p.kind, p.text = prose, "Book: "+p.text
		}
	}
	for i, p := range d.paras {
		switch p.kind {
		case prose:
			if i == 0 {
				d.Summary = joinTrimmed(p.text)
			} else {
				d.Details = append(d.Details, p.text)
			}
		case examples:
			d.Examples = append(d.Examples, p.examples...)
		case bookLine:
			d.Book = p.text
		}
	}
	return d
}

func blank(line string) bool { return strings.Trim(line, " ") == "" }

// classify reads one paragraph; first is the index of its first line in the whole text.
func classify(lines []string, first int) para {
	if len(lines) == 1 && strings.HasPrefix(lines[0], "Book: ") {
		return para{kind: bookLine, text: strings.TrimSpace(strings.TrimPrefix(lines[0], "Book: "))}
	}
	for _, l := range lines {
		if !strings.HasPrefix(l, indent) {
			return para{kind: prose, text: strings.Join(lines, "\n")}
		}
	}
	p := para{kind: examples}
	for i, l := range lines {
		src := strings.TrimPrefix(l, indent)
		// A line indented exactly four spaces starts an example (so does the paragraph's first
		// line, whatever its depth); a deeper line continues the one before.
		if i == 0 || !strings.HasPrefix(src, " ") {
			p.examples = append(p.examples, Example{Source: src, Line: first + i})
			continue
		}
		last := &p.examples[len(p.examples)-1]
		last.Source += "\n" + src
	}
	return p
}

func joinTrimmed(text string) string {
	lines := strings.Split(text, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimSpace(l)
	}
	return strings.Join(lines, " ")
}

// Markdown is the paragraphs in the order written, each run of examples as one fenced `alder`
// block, then the book line in italics (d.Book, or book when d.Book is ""). "" for an empty doc.
func (d Doc) Markdown(book string) string {
	if len(d.paras) == 0 {
		return ""
	}
	var blocks []string
	var run []Example
	flush := func() {
		if len(run) > 0 {
			blocks = append(blocks, fenced(run))
			run = nil
		}
	}
	for _, p := range d.paras {
		switch p.kind {
		case prose:
			flush()
			blocks = append(blocks, p.text)
		case examples:
			run = append(run, p.examples...)
		}
	}
	flush()
	if d.Book != "" {
		book = d.Book
	}
	if book != "" {
		blocks = append(blocks, "*Book: "+book+"*")
	}
	return strings.Join(blocks, "\n\n")
}

// fenced is one `alder` code block holding examples, its fence longer than any run of backticks
// inside them.
func fenced(exs []Example) string {
	srcs := make([]string, len(exs))
	longest := 0
	for i, e := range exs {
		srcs[i] = e.Source
		n := 0
		for _, r := range e.Source {
			if r == '`' {
				n++
				longest = max(longest, n)
			} else {
				n = 0
			}
		}
	}
	fence := strings.Repeat("`", max(3, longest+1))
	return fence + "alder\n" + strings.Join(srcs, "\n") + "\n" + fence
}

var bookRef = regexp.MustCompile(`^(?:(chapter) ([0-9]+)|(appendix) ([A-Z])), "([^"]+)"$`)

// ParseBook splits `chapter 9, "List"` into ("chapter", "9", "List") and `appendix B, "X"` into ("appendix", "B", "X").
func ParseBook(ref string) (kind, number, heading string, ok bool) {
	m := bookRef.FindStringSubmatch(ref)
	if m == nil {
		return "", "", "", false
	}
	if m[1] != "" {
		return m[1], m[2], m[5], true
	}
	return m[3], m[4], m[5], true
}

// Strict names the parts the stdlib format requires of a declaration: a function needs an
// example, a module needs a Book line.
type Strict struct{ Example, Book bool }

// Problems are the ways text breaks the stdlib format; nil when it follows it.
func Problems(text string, need Strict) []string {
	if strings.Trim(text, " \n") == "" {
		return []string{"no doc comment"}
	}
	d := Parse(text)
	var probs []string
	if p := d.paras[0]; p.kind != prose || strings.Contains(p.text, "\n") || !strings.HasSuffix(p.text, ".") {
		probs = append(probs, "the summary is not one line ending in a full stop")
	}
	if need.Example && len(d.Examples) == 0 {
		probs = append(probs, "no example")
	}
	for _, e := range d.Examples {
		if strings.Contains(e.Source, "\t") {
			probs = append(probs, "a tab in an example")
			break
		}
	}
	seenExample := false
	for _, p := range d.paras {
		if p.kind == examples {
			seenExample = true
		} else if p.kind == prose && seenExample {
			probs = append(probs, "prose follows the examples")
			break
		}
	}
	if d.Book == "" && need.Book {
		probs = append(probs, "no Book line")
	}
	if _, _, _, ok := ParseBook(d.Book); d.Book != "" && !ok {
		probs = append(probs, "the Book line is not `Book: chapter N, \"Heading\"` or `Book: appendix X, \"Heading\"`")
	}
	return probs
}
