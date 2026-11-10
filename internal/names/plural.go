package names

import (
	"strings"
	"unicode"
)

// Collection is the path segment of a table's collection: the table name in lower case, its
// words joined by `-`, the last word pluralised. `BlogPost` is `blog-posts`, `Entry` is
// `entries`. There is no inflector: `Person` is `persons`.
func Collection(table string) string {
	rs := []rune(table)
	var words []string
	start := 0
	for i := 1; i < len(rs); i++ {
		prev, cur := rs[i-1], rs[i]
		split := (unicode.IsLower(prev) || unicode.IsDigit(prev)) && unicode.IsUpper(cur)
		if !split && unicode.IsUpper(prev) && unicode.IsUpper(cur) && i+1 < len(rs) && unicode.IsLower(rs[i+1]) {
			split = true
		}
		if split {
			words = append(words, string(rs[start:i]))
			start = i
		}
	}
	words = append(words, string(rs[start:]))
	for i, w := range words {
		words[i] = strings.ToLower(w)
	}
	last := len(words) - 1
	words[last] = pluralise(words[last])
	return strings.Join(words, "-")
}

func pluralise(w string) string {
	switch {
	case strings.HasSuffix(w, "y") && len(w) > 1 && !strings.ContainsRune("aeiou", rune(w[len(w)-2])):
		return w[:len(w)-1] + "ies"
	case strings.HasSuffix(w, "s"), strings.HasSuffix(w, "x"), strings.HasSuffix(w, "z"),
		strings.HasSuffix(w, "ch"), strings.HasSuffix(w, "sh"):
		return w + "es"
	}
	return w + "s"
}
