package generate

import (
	"strings"

	"alder/internal/syntax"
)

// requalify rewrites, in every line of the edit (as replaced so far) and every insertion, the
// module qualifiers named by the keys of rename to their values: `Admin.Layout.isAdmin` and
// `<Admin.Layout.Page>` with Admin.Layout → Layout. Only a name written with a member after the
// module is rewritten, so a page value (`Admin.Layout {}`) keeps its full path. bare maps a
// qualifier that changed to its new one for the name written alone (`Todo { … }`, `: Todo`, an
// eponymous declaration): such a name is never a page value, whose spelling is a full path.
func (e *srcEdit) requalify(rename, bare map[string]string) {
	if len(rename) == 0 && len(bare) == 0 {
		return
	}
	for n := 1; n <= len(e.lines); n++ {
		if r, ok := e.repl[n]; ok && r == nil {
			continue // deleted
		}
		if cur := e.current(n); requalified(cur, rename, bare) != cur {
			e.replaceLine(n, requalified(cur, rename, bare))
		}
	}
	for i := range e.ins {
		for j, l := range e.ins[i].text {
			e.ins[i].text[j] = requalified(l, rename, bare)
		}
	}
}

// requalified is s with the qualifiers of rename rewritten (requalify), outside strings and
// comments.
func requalified(s string, rename, bare map[string]string) string {
	if !strings.ContainsAny(s, "ABCDEFGHIJKLMNOPQRSTUVWXYZ") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == '"':
			j := i + 1
			for j < len(s) && s[j] != '"' && s[j] != '\n' {
				if s[j] == '\\' {
					j++
				}
				j++
			}
			j = min(j+1, len(s))
			b.WriteString(s[i:j])
			i = j
		case c == '-' && strings.HasPrefix(s[i:], "--"):
			j := strings.IndexByte(s[i:], '\n')
			if j < 0 {
				j = len(s) - i
			}
			b.WriteString(s[i : i+j])
			i += j
		case isNameByte(c):
			j := i
			for j < len(s) && isNameByte(s[j]) {
				j++
			}
			if 'A' <= c && c <= 'Z' && (i == 0 || s[i-1] != '.') {
				k, out := requalifyChain(s, i, rename, bare)
				b.WriteString(out)
				i = k
				continue
			}
			b.WriteString(s[i:j])
			i = j
		default:
			b.WriteByte(c)
			i++
		}
	}
	return b.String()
}

// requalifyChain handles the dotted name of capitalised segments starting at s[i] (`Admin.Layout`
// in `Admin.Layout.Page` or `Admin.Layout.isAdmin`): its longest prefix that is a key of rename
// and has a segment after it is replaced. It returns where the name ends (before a member)
// and the text for s[i:end].
func requalifyChain(s string, i int, rename, bare map[string]string) (int, string) {
	var ends []int // the end of each capitalised segment
	j := i
	for {
		k := j
		for k < len(s) && isNameByte(s[k]) {
			k++
		}
		ends = append(ends, k)
		if k+1 < len(s) && s[k] == '.' && 'A' <= s[k+1] && s[k+1] <= 'Z' {
			j = k + 1
			continue
		}
		j = k
		break
	}
	member := j+1 < len(s) && s[j] == '.' && isNameByte(s[j+1]) // a lower-case member follows
	if !member && len(ends) == 1 {                              // a bare name: an eponymous type or table, through the old qualifier
		if q, ok := bare[s[i:j]]; ok {
			return j, q
		}
	}
	for n := len(ends); n >= 1; n-- {
		if n == len(ends) && !member {
			continue // the whole name with nothing after it: a page value, or a type
		}
		if q, ok := rename[s[i:ends[n-1]]]; ok {
			return j, q + s[ends[n-1]:j]
		}
	}
	return j, s[i:j]
}

// canonical is body, the text of a module a generator wrote at path with `import P` lines and
// full module paths, written with canonical qualifiers (design rule 9): each import with its
// alias, each reference through its qualifier. Text that does not parse is returned as it is.
func canonical(p *Program, path, body string) string {
	m, ds := syntax.ParseFile(path, []byte(body))
	for _, d := range ds {
		if d.Severity == "error" {
			return body
		}
	}
	e := newSrcEdit(body)
	retarget(e, p, m, nil, true)
	if out := e.result(); strings.TrimSuffix(out, "\n") != strings.TrimSuffix(body, "\n") {
		return out
	}
	return body
}
