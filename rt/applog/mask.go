package applog

import (
	"net/url"
	"strings"
)

// Mask is what a masked value is replaced with: the dev panel's mask.
const Mask = "••••"

// sensitiveWords are the parts of a parameter or field name that say it holds a secret: `key`
// (nightvote-replay's), and what OAuth, reset links and signed URLs name theirs.
var sensitiveWords = [...]string{"key", "token", "secret", "pass", "auth", "sig", "code", "session"}

// Sensitive reports whether a parameter or field named name holds a secret: its lowercase form
// contains key, token, secret, pass, auth, sig, code or session. A false positive hides a
// harmless value; a miss leaks a secret.
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

// LooksSecret reports whether s looks like a secret: a URL with a password
// (`scheme://user:pass@host`), a text beginning like a known key (`sk_`, `ghp_`, `AKIA`,
// `xoxb-`, `xoxp-`, `-----BEGIN`), or 32 or more base64 or hex characters without a space. A
// path (starting `/` or `./`, or with a `/` and no `=` or `+`) is never taken for the last kind.
// The checker refuses such a literal in a `deploy` declaration (E0030); the log masks it.
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
	if len(s) >= 32 && !strings.HasPrefix(s, "/") && !strings.HasPrefix(s, "./") && !(strings.Contains(s, "/") && !strings.ContainsAny(s, "=+")) {
		for _, r := range s {
			if !(r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '+' || r == '/' || r == '=') {
				return false
			}
		}
		return true
	}
	return false
}

// MaskURL is u's path and query as a log line shows them, with every secret replaced by Mask:
//
//   - the path, escaped (no raw control character reaches a line), with each segment that
//     LooksSecret (as written or decoded) replaced;
//   - the query's pairs in their order, each key kept (one that LooksSecret replaced), and a
//     value replaced when its key is Sensitive or it LooksSecret (decoded), or when the pair
//     does not parse (a bad % escape, a raw `;`): it is masked on the raw text, never dropped.
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
	q := u.RawQuery
	for first := true; ; first = false {
		pair, rest, more := strings.Cut(q, "&")
		if !first {
			b.WriteByte('&')
		}
		maskPair(&b, pair)
		if !more {
			break
		}
		q = rest
	}
	return b.String()
}

// maskPair writes the query pair k[=v] masked (MaskURL).
func maskPair(b *strings.Builder, pair string) {
	k, v, hasValue := strings.Cut(pair, "=")
	key, kerr := url.QueryUnescape(k)
	if kerr != nil {
		key = k
	}
	if secretText(k, key) {
		b.WriteString(Mask)
	} else {
		b.WriteString(k)
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
	b.WriteString(v)
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
