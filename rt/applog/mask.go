package applog

import (
	"net/url"
	"strings"
)

// Mask is what a masked value is replaced with: the dev panel's mask.
const Mask = "••••"

// sensitiveWords are the parts of a parameter or field name that say it holds a secret: `key`
// (nightvote-replay's), and what OAuth, reset links, signed URLs and login forms name theirs.
var sensitiveWords = [...]string{"key", "token", "secret", "pass", "auth", "sig", "code", "session", "pw", "jwt", "otp", "bearer", "credential"}

// Sensitive reports whether a parameter or field named name holds a secret: its lowercase form
// contains key, token, secret, pass, auth, sig, code, session, pw (pwd), jwt, otp, bearer or
// credential. A false positive hides a harmless value; a miss leaks a secret.
func Sensitive(name string) bool {
	for _, w := range sensitiveWords {
		if containsFold(name, w) {
			return true
		}
	}
	return false
}

// containsFold reports whether s contains the lowercase ASCII word w, ignoring ASCII case,
// without allocating.
func containsFold(s, w string) bool {
	for i := 0; i+len(w) <= len(s); i++ {
		j := 0
		for ; j < len(w); j++ {
			c := s[i+j]
			if 'A' <= c && c <= 'Z' {
				c += 'a' - 'A'
			}
			if c != w[j] {
				break
			}
		}
		if j == len(w) {
			return true
		}
	}
	return false
}

// LooksSecret reports whether s looks like a secret:
//
//   - a URL with a password (`scheme://user:pass@host`);
//   - a text beginning like a known key (`sk_`, `ghp_`, `AKIA`, `xoxb-`, `xoxp-`, `-----BEGIN`);
//   - a JWT (`eyJ…` with two dots);
//   - 32 or more base64 or hex characters without a space;
//   - 32 or more URL-safe token characters (letters, digits, `-`, `_`, `.`, and base64's `+`,
//     `/`, `=`) holding an upper-case letter, a lower-case letter and a digit: a base64url
//     token. A UUID (hex, one case), a slug or a file name has not all three.
//
// A path (starting `/` or `./`, or with a `/` and no `=` or `+`) is never taken for the last two
// kinds. The checker refuses such a literal in a `deploy` declaration (E0030); the log masks it.
func LooksSecret(s string) bool {
	if i := strings.Index(s, "://"); i >= 0 {
		auth := s[i+3:]
		if j := strings.IndexAny(auth, "/?#"); j >= 0 {
			auth = auth[:j]
		}
		if at := strings.LastIndex(auth, "@"); at >= 0 && strings.Contains(auth[:at], ":") {
			return true
		}
	}
	for _, p := range []string{"sk_", "ghp_", "AKIA", "xoxb-", "xoxp-", "-----BEGIN"} {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	if strings.HasPrefix(s, "eyJ") && strings.Count(s, ".") >= 2 && !strings.ContainsAny(s, " /") {
		return true
	}
	if len(s) < 32 || strings.HasPrefix(s, "/") || strings.HasPrefix(s, "./") || (strings.Contains(s, "/") && !strings.ContainsAny(s, "=+")) {
		return false
	}
	base64, upper, lower, digit := true, false, false, false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'A' && c <= 'Z':
			upper = true
		case c >= 'a' && c <= 'z':
			lower = true
		case c >= '0' && c <= '9':
			digit = true
		case c == '+' || c == '/' || c == '=':
		case c == '-' || c == '_' || c == '.':
			base64 = false
		default:
			return false
		}
	}
	return base64 || upper && lower && digit
}

// MaskURL is u's path and query as a log line shows them, with every secret replaced by Mask:
//
//   - the path, escaped (no raw control character reaches a line), with each segment that
//     LooksSecret (as written or decoded) replaced;
//   - the query's pairs in their order, each key kept (one that LooksSecret replaced), and a
//     value replaced when its key is Sensitive or it LooksSecret (decoded), or when the pair
//     does not parse (a bad % escape, a raw `;`): it is masked on the raw text, never dropped.
//     A value that is itself a URL (`?next=https%3A%2F%2F…%3Ftoken%3D…`) keeps its shape with
//     its own path, query and fragment masked by the same rules. Bytes outside printable ASCII
//     are %-escaped.
//
// A URL without a query and without a secret-looking segment is its escaped path, without
// allocating.
func MaskURL(u *url.URL) string {
	p := maskPath(u.EscapedPath())
	if u.RawQuery == "" {
		return p
	}
	var b strings.Builder
	b.Grow(len(p) + 1 + len(u.RawQuery))
	b.WriteString(p)
	b.WriteByte('?')
	maskQuery(&b, u.RawQuery, 0)
	return b.String()
}

// maxDepth is how many URLs inside a parameter's value MaskURL looks into.
const maxDepth = 2

// maskQuery writes the raw query q (or a fragment of pairs) masked.
func maskQuery(b *strings.Builder, q string, depth int) {
	for first := true; ; first = false {
		pair, rest, more := strings.Cut(q, "&")
		if !first {
			b.WriteByte('&')
		}
		maskPair(b, pair, depth)
		if !more {
			return
		}
		q = rest
	}
}

// maskPair writes the query pair k[=v] masked (MaskURL).
func maskPair(b *strings.Builder, pair string, depth int) {
	k, v, hasValue := strings.Cut(pair, "=")
	key, kerr := url.QueryUnescape(k)
	if kerr != nil {
		key = k
	}
	if secretText(k, key) {
		b.WriteString(Mask)
	} else {
		writeRaw(b, k)
	}
	if !hasValue {
		return
	}
	b.WriteByte('=')
	val, verr := url.QueryUnescape(v)
	if kerr != nil || verr != nil || strings.Contains(pair, ";") || Sensitive(key) || secretText(v, val) {
		b.WriteString(Mask)
		return
	}
	if inner, ok := maskInner(val, depth+1); ok {
		// the masked URL, encoded as a value again, with the masks left readable
		b.WriteString(strings.ReplaceAll(url.QueryEscape(inner), escapedMask, Mask))
		return
	}
	writeRaw(b, v)
}

var escapedMask = url.QueryEscape(Mask)

// maskInner is s masked when s is a URL holding something to hide (its path, query or
// fragment), and whether it was.
func maskInner(s string, depth int) (string, bool) {
	if depth > maxDepth || !strings.ContainsAny(s, "/?#") {
		return "", false
	}
	u, err := url.Parse(s)
	if err != nil || u.Opaque != "" {
		return "", false
	}
	var b strings.Builder
	if u.Scheme != "" {
		b.WriteString(u.Scheme + ":")
	}
	if u.Host != "" {
		b.WriteString("//" + u.Host)
	}
	b.WriteString(maskPath(u.EscapedPath()))
	if u.RawQuery != "" {
		b.WriteByte('?')
		maskQuery(&b, u.RawQuery, depth)
	}
	if f := u.EscapedFragment(); f != "" {
		b.WriteByte('#')
		maskQuery(&b, f, depth)
	}
	out := b.String()
	return out, strings.Contains(out, Mask)
}

// writeRaw writes s as written, with each byte outside printable ASCII (a control, DEL, any
// byte of a multi-byte character) %-escaped: a log line holds no raw control character.
func writeRaw(b *strings.Builder, s string) {
	const hex = "0123456789ABCDEF"
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < 0x20 || c >= 0x7f {
			b.WriteByte('%')
			b.WriteByte(hex[c>>4])
			b.WriteByte(hex[c&15])
			continue
		}
		b.WriteByte(c)
	}
}

// secretText reports whether a text, as written (raw) or decoded, LooksSecret.
func secretText(raw, decoded string) bool {
	return LooksSecret(raw) || (decoded != raw && LooksSecret(decoded))
}

// maskPath is the escaped path p with each segment that looks like a secret replaced; p itself
// when there is none.
func maskPath(p string) string {
	if !secretSegment(p) {
		return p
	}
	segs := strings.Split(p, "/")
	for i, s := range segs {
		if segmentSecret(s) {
			segs[i] = Mask
		}
	}
	return strings.Join(segs, "/")
}

// secretSegment reports whether any segment of the escaped path p looks like a secret, without
// allocating unless a segment holds an escape.
func secretSegment(p string) bool {
	for p != "" {
		s, rest, _ := strings.Cut(p, "/")
		if segmentSecret(s) {
			return true
		}
		p = rest
	}
	return false
}

// segmentSecret reports whether the escaped path segment s looks like a secret, as written or
// decoded.
func segmentSecret(s string) bool {
	if s == "" {
		return false
	}
	if LooksSecret(s) {
		return true
	}
	if strings.IndexByte(s, '%') >= 0 {
		if d, err := url.PathUnescape(s); err == nil && LooksSecret(d) {
			return true
		}
	}
	return false
}
