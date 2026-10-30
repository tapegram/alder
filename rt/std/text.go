package std

import (
	"math"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"unicode"
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
	a, b := sliceBounds(start, end, len(rs))
	if a >= b {
		return ""
	}
	return string(rs[a:b])
}

// sliceBounds normalises a [start, end) range over length items: negative
// indices count from the end and both are clamped to [0, length].
func sliceBounds(start, end int64, length int) (int, int) {
	n := int64(length)
	idx := func(i int64) int64 {
		if i < 0 {
			i += n
		}
		return min(max(i, 0), n)
	}
	return int(idx(start)), int(idx(end))
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
// https: URL, a path of this site ("/…", not "//…") or a fragment ("#top") is kept
// (rt.SafeURL); anything else —
// javascript:, data:, other schemes, protocol-relative or whitespace-prefixed URLs — becomes
// "#", a link and a redirect that go nowhere.
func UrlExternal(u string) rt.Page {
	if !rt.SafeURL(u) {
		return rt.Page{Path: "#"}
	}
	return rt.Page{Path: u}
}

// UrlAbsolute is `Url.absolute`: the page's URL (rt.PageURL, what `href` renders) with the
// public base (rt.PublicURL) in front when it is a path of this site; any other page (an
// external URL, a fragment, a sanitised "#") is its URL unchanged.
func UrlAbsolute(p rt.Page) string {
	u := rt.PageURL(p)
	if strings.HasPrefix(u, "/") {
		return rt.PublicURL + u
	}
	return u
}

// NavNoticed is `Nav.noticed text nav`: nav's answer with a toast showing text.
func NavNoticed(t string, n rt.Nav) rt.Nav { return rt.Noticed(t, n) }

// TextToBytes is the text's UTF-8 bytes.
func TextToBytes(s string) []byte { return []byte(s) }

func clampCount(n, length int64) int64 { return min(max(n, 0), length) }

// padRunes is k code points of pad repeated endlessly, starting at code point offset
// start of pad (k > 0, pad not empty), built without allocating more than k code points.
func padRunes(k, start int64, pad string) string {
	pr := []rune(pad)
	var b strings.Builder
	for i := int64(0); i < k; i++ {
		b.WriteRune(pr[(start+i)%int64(len(pr))])
	}
	return b.String()
}

// padPrefix is the first k code points of pad repeated.
func padPrefix(k int64, pad string) string { return padRunes(k, 0, pad) }

// padSuffix is the last k code points of pad repeated (a whole number of times).
func padSuffix(k int64, pad string) string {
	l := int64(utf8.RuneCountInString(pad))
	return padRunes(k, (l-k%l)%l, pad)
}

// TextPadLeft is `Text.padLeft n pad s`: Text.right n (repeat n pad ++ s)
// unless s already has n code points or pad is empty.
func TextPadLeft(n int64, pad, s string) string {
	if pad == "" || int64(utf8.RuneCountInString(s)) >= n {
		return s
	}
	return padSuffix(n-int64(utf8.RuneCountInString(s)), pad) + s
}

// TextPadRight is `Text.padRight n pad s`: Text.left n (s ++ repeat n pad)
// unless s already has n code points or pad is empty.
func TextPadRight(n int64, pad, s string) string {
	if pad == "" || int64(utf8.RuneCountInString(s)) >= n {
		return s
	}
	return s + padPrefix(n-int64(utf8.RuneCountInString(s)), pad)
}

// TextLeft is the first n code points.
func TextLeft(n int64, s string) string {
	rs := []rune(s)
	return string(rs[:clampCount(n, int64(len(rs)))])
}

// TextRight is the last n code points.
func TextRight(n int64, s string) string {
	rs := []rune(s)
	l := int64(len(rs))
	return string(rs[l-clampCount(n, l):])
}

// TextDropLeft drops the first n code points.
func TextDropLeft(n int64, s string) string {
	rs := []rune(s)
	return string(rs[clampCount(n, int64(len(rs))):])
}

// TextDropRight drops the last n code points.
func TextDropRight(n int64, s string) string {
	rs := []rune(s)
	l := int64(len(rs))
	return string(rs[:l-clampCount(n, l)])
}

// TextRepeat repeats s n times; n <= 0 gives "". An enormous n exhausts memory or
// panics in strings.Repeat (the router recovers); this mirrors List.range.
func TextRepeat(n int64, s string) string {
	if n <= 0 {
		return ""
	}
	return strings.Repeat(s, int(n))
}

// TextReverse reverses the code points.
func TextReverse(s string) string {
	rs := []rune(s)
	for i, j := 0, len(rs)-1; i < j; i, j = i+1, j-1 {
		rs[i], rs[j] = rs[j], rs[i]
	}
	return string(rs)
}

func TextTrimLeft(s string) string { return strings.TrimLeftFunc(s, unicode.IsSpace) }

func TextTrimRight(s string) string { return strings.TrimRightFunc(s, unicode.IsSpace) }

// TextConcat is `Text.join ""`.
func TextConcat(xs []string) string { return strings.Join(xs, "") }

// TextLines splits on "\n", strips a trailing "\r" from each line and drops
// the empty line after a final "\n".
func TextLines(s string) []string {
	if s == "" {
		return []string{}
	}
	parts := strings.Split(s, "\n")
	if parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	for i, p := range parts {
		parts[i] = strings.TrimSuffix(p, "\r")
	}
	return parts
}

// TextWords splits on runs of whitespace.
func TextWords(s string) []string {
	ws := strings.Fields(s)
	if ws == nil {
		return []string{}
	}
	return ws
}

// TextIndexes is `Text.indexes needle haystack`: every code-point offset
// where needle starts, overlapping included; an empty needle gives [].
func TextIndexes(needle, hay string) []int64 {
	out := []int64{}
	if needle == "" {
		return out
	}
	h, nd := []rune(hay), []rune(needle)
	for i := 0; i+len(nd) <= len(h); i++ {
		if string(h[i:i+len(nd)]) == needle {
			out = append(out, int64(i))
		}
	}
	return out
}

// TextToCodePoints is the text's code points.
func TextToCodePoints(s string) []int64 {
	out := []int64{}
	for _, r := range s {
		out = append(out, int64(r))
	}
	return out
}

// TextFromCodePoints builds text from code points; invalid scalar values
// become U+FFFD.
func TextFromCodePoints(xs []int64) string {
	var b strings.Builder
	for _, x := range xs {
		if x < 0 || x > unicode.MaxRune || (x >= 0xD800 && x <= 0xDFFF) {
			b.WriteRune(utf8.RuneError)
		} else {
			b.WriteRune(rune(x))
		}
	}
	return b.String()
}

// TextBuilder is the constant Text.builder: the empty builder (#37).
func TextBuilder() rt.TextBuilder { return rt.TextBuilder{} }

// TextAdd is `Text.add piece b`.
func TextAdd(s string, b rt.TextBuilder) rt.TextBuilder { return b.Add(s) }

// TextBuild is `Text.build b`.
func TextBuild(b rt.TextBuilder) string { return b.Build() }
