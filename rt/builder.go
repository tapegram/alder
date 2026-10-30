package rt

import "unsafe"

// TextBuilder is Alder's `Text.Builder`: an accumulator for a large Text (#37). The zero value
// is the empty builder (`Text.builder`). Add is O(1) and Build copies each byte once, so a
// render that adds whole rows allocates about twice its output, where nested Text.join and
// interpolation copy every byte once per level.
//
// It is a persistent list of pieces, newest first. Nodes are never changed after they are made,
// so two builders that share a prefix (`add "a" b0` and `add "b" b0`) are both correct: an Alder
// value may be shared (a top-level let, or across requests), and a builder that appended into a
// shared buffer would let one use see another's pieces.
type TextBuilder struct{ n *tbNode }

type tbNode struct {
	s    string
	prev *tbNode
	size int // total bytes of this piece and every piece before it
}

// Add is `Text.add s b`: b with s after its pieces; b itself when s is "".
func (b TextBuilder) Add(s string) TextBuilder {
	if s == "" {
		return b
	}
	return TextBuilder{&tbNode{s: s, prev: b.n, size: b.Len() + len(s)}}
}

// Len is the length in bytes of the Text that Build makes.
func (b TextBuilder) Len() int {
	if b.n == nil {
		return 0
	}
	return b.n.size
}

// Build is `Text.build b`: the pieces in order, in one allocation, filled from the end by a loop
// (no recursion, so a builder of a million pieces is fine).
func (b TextBuilder) Build() string {
	size := b.Len()
	if size == 0 {
		return ""
	}
	buf := make([]byte, size)
	end := size
	for n := b.n; n != nil; n = n.prev {
		end -= copy(buf[end-len(n.s):end], n.s)
	}
	// buf is never written again and nothing else holds it, so the string may share it (the
	// idiom of strings.Builder.String).
	return unsafe.String(unsafe.SliceData(buf), size)
}

// AlderEqual: two builders are == when they build the same Text.
func (b TextBuilder) AlderEqual(other any) bool {
	o := other.(TextBuilder)
	return b.Len() == o.Len() && b.Build() == o.Build()
}

// AlderKey: a builder as a Dict key is keyed by its Text.
func (b TextBuilder) AlderKey() string { return b.Build() }

// AlderShow prints an expression that rebuilds an equal builder.
func (b TextBuilder) AlderShow() (string, bool) {
	if b.Len() == 0 {
		return "Text.builder", false
	}
	return "Text.add " + quote(b.Build()) + " Text.builder", true
}
