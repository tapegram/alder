package adapters

import (
	"context"
	"strings"

	"alder/rt"
	"alder/rt/html"
	"app"
)

// markdown implements Markdown.Markdown: a post's body as HTML.
type markdown struct{}

// Render is Markdown.render.
func (markdown) Render(_ context.Context, args app.Args__Markdown__Markdown__Render) (html.Node, error) {
	return Markdown(args.Value), nil
}

// Markdown renders a small subset of Markdown:
//
//   - paragraphs, separated by blank lines; a single line break inside a paragraph is a <br>;
//   - `code`: its content is literal, nothing inside it is formatting;
//   - **strong** and *emphasis*, each closed by the next ** or * and never empty;
//   - [text](url) for an absolute http: or https: URL with a host (rt.SafeURL); any other link,
//     such as javascript:, data: or a relative one, stays as its literal source text.
//
// Everything else, an unclosed * or ` or [ included, is literal text. The result is built from
// html.El and html.Text nodes, never html.Raw: the renderer escapes every piece of text, so no
// input can produce markup. Rendering is linear in the input's length (a closing delimiter is
// searched for once per position it can close: see finder).
func Markdown(src string) html.Node {
	src = strings.ReplaceAll(src, "\r\n", "\n")
	src = strings.ReplaceAll(src, "\r", "\n")
	var paras []html.Node
	var lines []string
	flush := func() {
		if len(lines) == 0 {
			return
		}
		var kids []html.Node
		for i, l := range lines {
			if i > 0 {
				kids = append(kids, html.El("br", nil))
			}
			kids = append(kids, inline(l, true)...)
		}
		paras = append(paras, html.El("p", nil, kids...))
		lines = nil
	}
	for _, l := range strings.Split(src, "\n") {
		l = strings.TrimSpace(l)
		if l == "" {
			flush()
			continue
		}
		lines = append(lines, l)
	}
	flush()
	if len(paras) == 0 {
		return html.None()
	}
	return html.Nodes(paras...)
}

// inline is s's inline formatting: code spans, strong, emphasis and (when links is true) links.
// The content of a strong span has no "**" and that of an emphasis no "*" (each closes at the
// next one), and a link's text is formatted without links, so recursion is a few levels deep.
func inline(s string, links bool) []html.Node {
	var out []html.Node
	var text strings.Builder
	emit := func(n html.Node) {
		if text.Len() > 0 {
			out = append(out, html.Text(text.String()))
			text.Reset()
		}
		out = append(out, n)
	}
	tick, star2, star, close := finder{s: s, sub: "`"}, finder{s: s, sub: "**"}, finder{s: s, sub: "*"}, finder{s: s, sub: "]("}
	paren := finder{s: s, sub: ")"}
	for i := 0; i < len(s); {
		switch {
		case s[i] == '`':
			if j := tick.next(i + 2); j >= 0 {
				emit(html.El("code", nil, html.Text(s[i+1:j])))
				i = j + 1
				continue
			}
		case strings.HasPrefix(s[i:], "**"):
			if j := star2.next(i + 3); j >= 0 && spanned(s[i+2:j]) {
				emit(html.El("strong", nil, inline(s[i+2:j], links)...))
				i = j + 2
				continue
			}
			text.WriteString("**")
			i += 2
			continue
		case s[i] == '*':
			if j := star.next(i + 2); j >= 0 && spanned(s[i+1:j]) {
				emit(html.El("em", nil, inline(s[i+1:j], links)...))
				i = j + 1
				continue
			}
		case s[i] == '[' && links:
			if j := close.next(i + 1); j >= 0 {
				if k := paren.next(j + 2); k >= 0 {
					if url := strings.TrimSpace(s[j+2 : k]); safeLink(url) { // TrimSpace scans only the ends
						emit(html.El("a", []html.Attr{html.A("href", url)}, inline(s[i+1:j], false)...))
						i = k + 1
						continue
					}
				}
			}
		}
		text.WriteByte(s[i])
		i++
	}
	if text.Len() > 0 {
		out = append(out, html.Text(text.String()))
	}
	return out
}

// spanned reports whether s can be the content of an emphasis or strong span: not empty and
// not starting or ending with a space, so "2 * 3 * 4" stays literal.
func spanned(s string) bool {
	return s != "" && s[0] != ' ' && s[len(s)-1] != ' '
}

// maxURL is the longest URL a link may have. Longer ones stay text, which also keeps rendering
// linear: "[a](" repeated thousands of times would otherwise check ever longer candidate URLs.
const maxURL = 2048

// safeLink reports whether url may be a link in a post: an absolute http: or https: URL with a
// host and no control characters, as rt.SafeURL accepts it, at most maxURL bytes long.
// rt.SafeURL also accepts this site's paths and fragments; a post links only to the web.
func safeLink(url string) bool {
	if len(url) > maxURL {
		return false
	}
	return (hasPrefixFold(url, "http://") || hasPrefixFold(url, "https://")) && rt.SafeURL(url)
}

// hasPrefixFold is strings.HasPrefix ignoring ASCII case.
func hasPrefixFold(s, prefix string) bool {
	return len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix)
}

// finder finds the next occurrence of sub in s at or after a position, remembering its last
// answer: positions are asked for in increasing order, so each part of s is searched once, and
// an input such as 50 000 unclosed "*" stays linear instead of quadratic.
type finder struct {
	s, sub string
	from   int // the position last asked for
	at     int // its answer: the index of sub, or -1 for none at or after from
	asked  bool
}

// next is the index of the first sub at or after from, or -1.
func (f *finder) next(from int) int {
	if f.asked && from >= f.from && (f.at < 0 || f.at >= from) {
		return f.at
	}
	f.asked, f.from, f.at = true, from, -1
	if from <= len(f.s) {
		if j := strings.Index(f.s[from:], f.sub); j >= 0 {
			f.at = from + j
		}
	}
	return f.at
}
