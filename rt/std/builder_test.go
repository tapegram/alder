package std

import (
	"strconv"
	"strings"
	"testing"

	"alder/rt"
)

// The nested-render shape of #37: 10 items × 10 options × 100 rows, each row ~335 bytes of
// interpolated JSON, every level wrapped in brackets and joined with commas.
const benchPad = "lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. ut enim ad minim veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat. duis aute irure dolor in reprehenderit in voluptate velit esse cillum dolore eu fugiat"

func benchRow(i, j, k int) string {
	return rt.Concat(`{"item":`, strconv.Itoa(i), `,"option":`, strconv.Itoa(j), `,"row":`, strconv.Itoa(k), `,"note":"`, benchPad, `"}`)
}

func renderJoinNested() string {
	items := make([]string, 10)
	for i := range items {
		opts := make([]string, 10)
		for j := range opts {
			rows := make([]string, 100)
			for k := range rows {
				rows[k] = benchRow(i, j, k)
			}
			opts[j] = rt.Concat("[", TextJoin(",", rows), "]")
		}
		items[i] = rt.Concat("[", TextJoin(",", opts), "]")
	}
	return rt.Concat("[", TextJoin(",", items), "]")
}

// BenchmarkTextJoinNested is the comparison for BenchmarkTextBuild: each level copies every byte.
func BenchmarkTextJoinNested(b *testing.B) {
	out := renderJoinNested()
	b.ReportAllocs()
	b.SetBytes(int64(len(out)))
	for b.Loop() {
		out = renderJoinNested()
	}
	if !strings.HasPrefix(out, "[[[") {
		b.Fatal(out[:10])
	}
}
