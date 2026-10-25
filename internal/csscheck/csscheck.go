// Package csscheck holds the checks the tests run over the CSS Alder writes itself: the dev
// panel, the error strip and the pending bar it injects into a page, and the examples' and the
// book's stylesheets.
package csscheck

import (
	"regexp"
	"strings"
)

// VarsWithoutLiteralFallback is each var() in css, in order of its start, whose fallback is
// missing, empty, a CSS-wide keyword (inherit, initial, unset, revert, revert-layer) or itself
// holds a var(): the text of the whole var(...). A var() nested in another's fallback is checked
// on its own as well. Such a var() leaves the property to the cascade when the custom property
// is not defined (a page without alder-ui), so injected CSS must not use one.
func VarsWithoutLiteralFallback(css string) []string {
	var out []string
	for i := 0; ; {
		j := strings.Index(css[i:], "var(")
		if j < 0 {
			return out
		}
		start := i + j
		i = start + len("var(")
		end, comma := closeParen(css, i)
		if end < 0 {
			return append(out, css[start:])
		}
		if comma < 0 || !literal(css[comma+1:end]) {
			out = append(out, css[start:end+1])
		}
	}
}

// closeParen is the index of the ")" closing the parenthesis opened just before from, and of
// the first comma at that depth (-1 if none); -1, -1 when it is not closed.
func closeParen(css string, from int) (end, comma int) {
	depth, comma := 0, -1
	for k := from; k < len(css); k++ {
		switch css[k] {
		case '(':
			depth++
		case ')':
			if depth == 0 {
				return k, comma
			}
			depth--
		case ',':
			if depth == 0 && comma < 0 {
				comma = k
			}
		}
	}
	return -1, -1
}

var wideKeywords = map[string]bool{"inherit": true, "initial": true, "unset": true, "revert": true, "revert-layer": true}

func literal(fallback string) bool {
	f := strings.TrimSpace(fallback)
	return f != "" && !wideKeywords[strings.ToLower(f)] && !strings.Contains(f, "var(")
}

var alToken = regexp.MustCompile(`--al-[a-z0-9-]+`)

// RemovedALTokens is each --al-* custom property named in css, in order, other than the
// --al-width-* layout widths, which alder-ui v0.4 keeps: every other --al- token was removed.
func RemovedALTokens(css string) []string {
	var out []string
	for _, m := range alToken.FindAllString(css, -1) {
		if !strings.HasPrefix(m, "--al-width-") {
			out = append(out, m)
		}
	}
	return out
}
