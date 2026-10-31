package theme

import "strings"

// Theme is a set of token values by field name: the theme-wide tokens in Root, the colours of
// each scheme in Light and Dark. It is the shape the importer returns, the theme page edits and
// the `theme` declaration is written from. A Font value is its families joined by ", ".
type Theme struct {
	Root  map[string]string `json:"root"`
	Light map[string]string `json:"light"`
	Dark  map[string]string `json:"dark"`
}

// Empty reports whether the theme sets nothing.
func (t Theme) Empty() bool {
	return len(t.Root) == 0 && len(t.Light) == 0 && len(t.Dark) == 0
}

// Families splits a font stack on the commas outside quotes, trims each family, drops a
// surrounding pair of `'` or `"`, and drops the empty ones.
func Families(v string) []string {
	var out []string
	add := func(s string) {
		s = strings.TrimSpace(s)
		if n := len(s); n >= 2 && (s[0] == '"' || s[0] == '\'') && s[n-1] == s[0] {
			s = strings.TrimSpace(s[1 : n-1])
		}
		if s != "" {
			out = append(out, s)
		}
	}
	for _, part := range splitTop(v, ',') {
		add(part)
	}
	return out
}
