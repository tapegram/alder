package html

import (
	"hash/fnv"
	"strconv"
	"strings"
)

// Patch is one element patch of a Datastar answer: replace (morph, mode outer) the element
// Selector names — "#id", or "body" — with Elements, its markup in the page after the message.
type Patch struct {
	Selector string
	Elements string
}

// ShownHeader is the request header a message's post (ClickPost, SubmitPost) carries: the Hash
// of the <body> the page shows, as its <al-shown> element (datastar.ShownTag) says; "" for none.
const ShownHeader = "Alder-Shown"

// Hash is the shown hash of a rendered <body>: FNV-1a 64 of it, 16 lower-case hex digits. It
// is not a secret: a client that lies only mispatches its own page.
func Hash(body string) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(body))
	s := strconv.FormatUint(h.Sum64(), 16)
	return strings.Repeat("0", 16-len(s)) + s
}

// BodyOf is the <body>…</body> of a rendered page, inclusive, or the page wrapped in <body>
// when it has none.
func BodyOf(page string) string {
	i, j, ok := bodyRange(page)
	if !ok {
		return "<body>" + page + "</body>"
	}
	return page[i:j]
}

// bodyRange is where the <body> element of page starts and ends.
func bodyRange(page string) (int, int, bool) {
	i := startTagAt(page, "body")
	j := strings.LastIndex(page, "</body>")
	if i < 0 || j < i {
		return 0, 0, false
	}
	return i, j + len("</body>"), true
}

// startTagAt is the index of the first <name> or <name …> start tag in page, or -1.
func startTagAt(page, name string) int {
	open := "<" + name
	for i := 0; ; {
		j := strings.Index(page[i:], open)
		if j < 0 {
			return -1
		}
		k := i + j + len(open)
		if k < len(page) && (page[k] == '>' || page[k] == ' ') {
			return i + j
		}
		i = k
	}
}

// regionID reports whether id can be a region's: `^[A-Za-z_][A-Za-z0-9_-]*$`, so `#id` is a
// selector without escaping.
func regionID(id string) bool {
	if id == "" {
		return false
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c == '_':
		case i > 0 && (c >= '0' && c <= '9' || c == '-'):
		default:
			return false
		}
	}
	return true
}

// node is a region in one render: its markup's range and its direct child regions.
type node struct {
	id         string
	start, end int
	kids       []*node
}

// tree is the body of r as a region tree: the body the root (id ""), its regions those of
// keep. ok is false when r has no <body>.
func tree(r Rendered, keep map[string]bool) (*node, bool) {
	s, e, ok := bodyRange(r.Page)
	if !ok {
		return nil, false
	}
	root := &node{start: s, end: e}
	stack := []*node{root}
	for _, g := range r.Regions {
		if g.Inert || !keep[g.ID] || g.Start < s || g.End > e {
			continue
		}
		for len(stack) > 1 && g.Start >= stack[len(stack)-1].end {
			stack = stack[:len(stack)-1]
		}
		n := &node{id: g.ID, start: g.Start, end: g.End}
		top := stack[len(stack)-1]
		top.kids = append(top.kids, n)
		stack = append(stack, n)
	}
	return root, true
}

// ids counts the ids of r's regions, inert ones included.
func ids(r Rendered) map[string]int {
	m := make(map[string]int, len(r.Regions))
	for _, g := range r.Regions {
		m[g.ID]++
	}
	return m
}

// Fragments is the element patches that turn the page before into the page after, both
// rendered by RenderTracked, in document order; none when their bodies are equal.
//
// A region is an element whose id matches regionID and is unique in both renders, outside
// <head> and <template>; the <body> is the outermost one. Starting at the body, a region whose
// markup is equal on both sides needs nothing; one whose skeleton — its markup with each
// direct child region cut out — is equal on both sides (the same child regions in the same
// order, and everything between them identical) is compared child by child; any other is one
// patch of its after-markup. Everything outside a patched region is proved equal by a
// skeleton, so applying the patches to the before-DOM gives the after-markup exactly. A page
// with an id in Html.unsafe markup (RawIDs) has no regions but the body.
func Fragments(before, after Rendered) []Patch {
	keep := map[string]bool{}
	if !before.RawIDs && !after.RawIDs {
		nb, na := ids(before), ids(after)
		for id, n := range na {
			if n == 1 && nb[id] == 1 && regionID(id) {
				keep[id] = true
			}
		}
	}
	tb, okb := tree(before, keep)
	ta, oka := tree(after, keep)
	if !okb || !oka {
		if BodyOf(before.Page) == BodyOf(after.Page) {
			return nil
		}
		return []Patch{{Selector: "body", Elements: BodyOf(after.Page)}}
	}
	var out []Patch
	var compare func(b, a *node)
	compare = func(b, a *node) {
		mb, ma := before.Page[b.start:b.end], after.Page[a.start:a.end]
		if mb == ma {
			return
		}
		if sameSkeleton(before.Page, after.Page, b, a) {
			for i := range a.kids {
				compare(b.kids[i], a.kids[i])
			}
			return
		}
		sel := "body"
		if a.id != "" {
			sel = "#" + a.id
		}
		out = append(out, Patch{Selector: sel, Elements: ma})
	}
	compare(tb, ta)
	return out
}

// sameSkeleton reports whether regions b (in page pb) and a (in pa) have the same direct child
// regions in the same order with identical markup around them.
func sameSkeleton(pb, pa string, b, a *node) bool {
	if len(b.kids) != len(a.kids) {
		return false
	}
	posB, posA := b.start, a.start
	for i := range b.kids {
		kb, ka := b.kids[i], a.kids[i]
		if kb.id != ka.id || pb[posB:kb.start] != pa[posA:ka.start] {
			return false
		}
		posB, posA = kb.end, ka.end
	}
	return pb[posB:b.end] == pa[posA:a.end]
}
