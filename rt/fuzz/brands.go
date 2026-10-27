package fuzz

import (
	"math"
	"strings"

	"alder/rt/web"
)

// Generators of the stdlib's refinement brands (spec §0a): each draws values of its brand
// directly, since filtering random Text would trip the health check, and then checks the value
// with the brand's own predicate, the one a form or URL decoder applies (rt/web, as gen's
// brandCheck emits it). A value that fails is a bug here; it is rejected like a filter's.

// NonEmpty is Text.NonEmpty: a code point that is not whitespace, then any Text.
func NonEmpty(c *Choices) string {
	uni := textMode(c)
	first := char(c, uni, 0.3)
	if strings.TrimSpace(string(first)) == "" {
		first = rune(asciiOrder[0])
	}
	return checked("Text.NonEmpty", web.NonEmpty, string(first)+textRest(c, uni))
}

// Secret is Text.Secret: one code point or more, whitespace allowed.
func Secret(c *Choices) string {
	uni := textMode(c)
	return checked("Text.Secret", web.Secret, string(char(c, uni, 0.3))+textRest(c, uni))
}

// Email is Text.Email: local@domain, the local part a word of letters, digits and ". + - _",
// the domain one or more lower-case words joined by dots. The simplest is "a@a".
func Email(c *Choices) string {
	local := word(c, emailLocal)
	domain := word(c, hostChars)
	p := lengthBias(c)
	for more(c, p) {
		domain += "." + word(c, hostChars)
	}
	return checked("Text.Email", web.Email, local+"@"+domain)
}

// URL is Text.Url: http or https, a host of dot-joined lower-case words, then path segments.
// The simplest is "http://a".
func URL(c *Choices) string {
	scheme := "http"
	if Bool(c, 0.5) {
		scheme = "https"
	}
	host := word(c, hostChars)
	p := lengthBias(c)
	for more(c, p) {
		host += "." + word(c, hostChars)
	}
	path := ""
	for more(c, p) {
		path += "/" + word(c, pathChars)
	}
	return checked("Text.Url", web.URL, scheme+"://"+host+path)
}

// Positive is Int.Positive: 1 and up, mostly small, sometimes huge (Int's magnitudes).
func Positive(c *Choices) int64 {
	n := 1 + int64(c.draw(math.MaxInt64-1, magnitude))
	if !(n > 0) {
		panic(Reject{What: "Int.Positive"})
	}
	return n
}

// NonNegative is Int.NonNegative: 0 and up, mostly small, sometimes huge.
func NonNegative(c *Choices) int64 {
	n := int64(c.draw(math.MaxInt64, magnitude))
	if !(n >= 0) {
		panic(Reject{What: "Int.NonNegative"})
	}
	return n
}

const (
	hostChars  = "abcdefghijklmnopqrstuvwxyz0123456789"
	pathChars  = hostChars + "-_."
	emailLocal = hostChars + "ABCDEFGHIJKLMNOPQRSTUVWXYZ.+-_"
)

// word draws one character of alphabet or more (the first character simplest).
func word(c *Choices, alphabet string) string {
	n := uint64(len(alphabet) - 1)
	b := []byte{alphabet[c.Choice(n)]}
	p := lengthBias(c)
	for more(c, p) {
		b = append(b, alphabet[c.Choice(n)])
	}
	return string(b)
}

// textRest is the rest of a Text after its first code point: a continue bit per code point.
func textRest(c *Choices, uni bool) string {
	var b strings.Builder
	p := lengthBias(c)
	for more(c, p) {
		b.WriteRune(char(c, uni, 0.3))
	}
	return b.String()
}

// checked is s when the brand's predicate ok accepts it, else a Reject naming the brand.
func checked(brand string, ok func(string) bool, s string) string {
	if !ok(s) {
		panic(Reject{What: brand})
	}
	return s
}
