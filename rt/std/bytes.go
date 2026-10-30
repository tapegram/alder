package std

import (
	"unicode/utf8"

	"alder/rt"
)

// BytesLength counts bytes, not code points (Text.length counts those).
func BytesLength(b []byte) int64 { return int64(len(b)) }

// BytesToText is Just the text when b is valid UTF-8.
func BytesToText(b []byte) rt.Maybe[string] {
	if !utf8.Valid(b) {
		return rt.Nothing[string]()
	}
	return rt.Just(string(b))
}

// BytesEmpty is Bytes.empty.
func BytesEmpty() []byte { return []byte{} }

// BytesIsEmpty is Bytes.isEmpty.
func BytesIsEmpty(b []byte) bool { return len(b) == 0 }

// BytesSlice takes bytes [start, end) by Text.slice's index rule; the
// result is a fresh slice.
func BytesSlice(start, end int64, b []byte) []byte {
	i, j := sliceBounds(start, end, len(b))
	if i >= j {
		return []byte{}
	}
	return append([]byte{}, b[i:j]...)
}

// BytesAppend joins a and b into a fresh slice.
func BytesAppend(a, b []byte) []byte {
	out := make([]byte, 0, len(a)+len(b))
	return append(append(out, a...), b...)
}

// BytesConcat joins all the slices into a fresh slice.
func BytesConcat(bs [][]byte) []byte {
	n := 0
	for _, b := range bs {
		n += len(b)
	}
	out := make([]byte, 0, n)
	for _, b := range bs {
		out = append(out, b...)
	}
	return out
}

// BytesToList is each byte as an Int, 0..255.
func BytesToList(b []byte) []int64 {
	out := make([]int64, len(b))
	for i, v := range b {
		out[i] = int64(v)
	}
	return out
}

// BytesFromList keeps the low eight bits of each Int.
func BytesFromList(xs []int64) []byte {
	out := make([]byte, len(xs))
	for i, v := range xs {
		out[i] = byte(v)
	}
	return out
}
