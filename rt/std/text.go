package std

import (
	"math"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"alder/rt"
)

// TextLength counts Unicode code points.
func TextLength(s string) int64 { return int64(utf8.RuneCountInString(s)) }

func TextIsEmpty(s string) bool { return s == "" }

func TextTrim(s string) string { return strings.TrimSpace(s) }

func TextToUpper(s string) string { return strings.ToUpper(s) }

func TextToLower(s string) string { return strings.ToLower(s) }

// TextContains is `Text.contains needle haystack`.
func TextContains(needle, haystack string) bool { return strings.Contains(haystack, needle) }

// TextStartsWith is `Text.startsWith prefix text`.
func TextStartsWith(prefix, s string) bool { return strings.HasPrefix(s, prefix) }

// TextEndsWith is `Text.endsWith suffix text`.
func TextEndsWith(suffix, s string) bool { return strings.HasSuffix(s, suffix) }

// TextReplace is `Text.replace old new text`, replacing every occurrence.
func TextReplace(old, new, s string) string { return strings.ReplaceAll(s, old, new) }

// TextSplit is `Text.split sep text`.
func TextSplit(sep, s string) []string { return strings.Split(s, sep) }

// TextJoin is `Text.join sep parts`.
func TextJoin(sep string, parts []string) string { return strings.Join(parts, sep) }

// TextSlice takes code points [start, end). Negative indices count from
// the end; indices are clamped to the text; start >= end gives "".
func TextSlice(start, end int64, s string) string {
	rs := []rune(s)
	n := int64(len(rs))
	idx := func(i int64) int64 {
		if i < 0 {
			i += n
		}
		return min(max(i, 0), n)
	}
	a, b := idx(start), idx(end)
	if a >= b {
		return ""
	}
	return string(rs[a:b])
}

var regexCache sync.Map // pattern string -> *regexp.Regexp (nil when invalid)

// TextMatches is `Text.matches regex text`: true when the regex (RE2
// syntax) matches anywhere in text; anchor with ^…$ for a whole match. An
// invalid regex matches nothing.
func TextMatches(re, s string) bool {
	v, ok := regexCache.Load(re)
	if !ok {
		r, err := regexp.Compile(re)
		if err != nil {
			r = nil
		}
		v, _ = regexCache.LoadOrStore(re, r)
	}
	r := v.(*regexp.Regexp)
	return r != nil && r.MatchString(s)
}

func TextFromInt(n int64) string { return strconv.FormatInt(n, 10) }

// TextFromFloat uses the shortest decimal that round-trips, never an
// exponent: 3.5 → "3.5", 2.0 → "2".
func TextFromFloat(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

// TextToInt parses a base-10 integer with an optional sign; anything else
// (spaces, decimals, overflow) is Nothing.
func TextToInt(s string) rt.Maybe[int64] {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return rt.Nothing[int64]()
	}
	return rt.Just(n)
}

// TextToFloat parses a finite decimal number; NaN and infinities are Nothing.
func TextToFloat(s string) rt.Maybe[float64] {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return rt.Nothing[float64]()
	}
	return rt.Just(f)
}

// UrlExternal is `Url.external`: a link to a URL outside the app. Only an absolute http: or
// https: URL or a path of this site ("/…", not "//…") is kept (rt.SafeURL); anything else —
// javascript:, data:, other schemes, protocol-relative or whitespace-prefixed URLs — becomes
// "#", a link and a redirect that go nowhere.
func UrlExternal(u string) rt.Page {
	if !rt.SafeURL(u) || u != "" && u[0] == '#' {
		return rt.Page{Path: "#"}
	}
	return rt.Page{Path: u}
}
