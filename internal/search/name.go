package search

import (
	"sort"
	"strings"
	"unicode"
)

// searchName is name search on a trimmed query, restricted to source when it is not empty.
func (ix *Index) searchName(q, source string) []Hit {
	if q == "" {
		return nil
	}
	orig := q
	if dot := strings.LastIndex(orig, "."); dot >= 0 {
		orig = orig[dot+1:]
	}
	q = strings.ToLower(q)
	var pool []*Entry
	for i := range ix.Entries {
		if e := &ix.Entries[i]; source == "" || e.Source == source {
			pool = append(pool, e)
		}
	}

	type ranked struct {
		e    *Entry
		tier int
	}
	var found []ranked
	if dot := strings.LastIndex(q, "."); dot >= 0 {
		pool = inModule(pool, q[:dot])
		name := q[dot+1:]
		for _, e := range pool {
			if name == "" {
				found = append(found, ranked{e, 0})
			} else if t := nameTier(e, name); t >= 0 {
				found = append(found, ranked{e, t})
			}
		}
		if name == "" {
			return hitsOf(found, func(r ranked) *Entry { return r.e })
		}
	} else {
		for _, e := range pool {
			if t := nameTier(e, q); t >= 0 {
				found = append(found, ranked{e, t})
			}
		}
	}
	sort.SliceStable(found, func(i, j int) bool {
		a, b := found[i], found[j]
		if a.tier != b.tier {
			return a.tier < b.tier
		}
		if len(a.e.Name) != len(b.e.Name) {
			return len(a.e.Name) < len(b.e.Name)
		}
		if sa, sb := sourceRank(a.e.Source), sourceRank(b.e.Source); sa != sb {
			return sa < sb
		}
		// Between names equal but for case, the one typed exactly comes first.
		if ea, eb := a.e.Name == orig, b.e.Name == orig; ea != eb {
			return ea
		}
		return a.e.Module+"."+a.e.Name < b.e.Module+"."+b.e.Name
	})
	return hitsOf(found, func(r ranked) *Entry { return r.e })
}

func hitsOf[T any](xs []T, entry func(T) *Entry) []Hit {
	hits := make([]Hit, len(xs))
	for i, x := range xs {
		hits[i] = Hit{Entry: entry(x)}
	}
	return hits
}

// sourceRank orders sources: app first, stdlib second, anything else after.
func sourceRank(source string) int {
	switch source {
	case "app":
		return 0
	case "stdlib":
		return 1
	}
	return 2
}

// inModule keeps the entries whose module matches part (lower case): equal to it or ending in "."+part;
// failing any, those whose module's last segment starts with it.
func inModule(pool []*Entry, part string) []*Entry {
	var out []*Entry
	for _, e := range pool {
		m := strings.ToLower(e.Module)
		if m == part || strings.HasSuffix(m, "."+part) {
			out = append(out, e)
		}
	}
	if len(out) > 0 {
		return out
	}
	for _, e := range pool {
		m := strings.ToLower(e.Module)
		if i := strings.LastIndex(m, "."); strings.HasPrefix(m[i+1:], part) {
			out = append(out, e)
		}
	}
	return out
}

// nameTier is the match tier of a lower-case query against an entry's name, or -1 for no match.
func nameTier(e *Entry, q string) int {
	lower := strings.ToLower(e.Name)
	switch {
	case lower == q:
		return 0
	case strings.HasPrefix(lower, q):
		return 1
	case atWordBoundary(e.Name, lower, q):
		return 2
	case strings.Contains(lower, q):
		return 3
	case isSubsequence(q, lower):
		return 4
	case isSubsequence(q, strings.ToLower(e.Module)+"."+lower):
		return 5
	}
	return -1
}

// atWordBoundary reports whether q occurs in lower at the start of a camelCase hump of name.
func atWordBoundary(name, lower, q string) bool {
	nr, lr := []rune(name), []rune(lower)
	if len(nr) != len(lr) { // case folding changed the length: skip the hump test
		return false
	}
	qr := []rune(q)
	for i := 0; i+len(qr) <= len(lr); i++ {
		if i > 0 && !unicode.IsUpper(nr[i]) && (unicode.IsLetter(nr[i-1]) || unicode.IsDigit(nr[i-1])) {
			continue
		}
		if string(lr[i:i+len(qr)]) == q {
			return true
		}
	}
	return false
}

// isSubsequence reports whether the runes of q occur in s in order.
func isSubsequence(q, s string) bool {
	qr := []rune(q)
	k := 0
	for _, c := range s {
		if k < len(qr) && c == qr[k] {
			k++
		}
	}
	return k == len(qr)
}
