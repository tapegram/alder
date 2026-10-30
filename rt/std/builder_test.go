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

func renderBuild() string {
	b := TextBuilder()
	b = TextAdd("[", b)
	for i := range 10 {
		if i > 0 {
			b = TextAdd(",", b)
		}
		b = TextAdd("[", b)
		for j := range 10 {
			if j > 0 {
				b = TextAdd(",", b)
			}
			b = TextAdd("[", b)
			for k := range 100 {
				if k > 0 {
					b = TextAdd(",", b)
				}
				b = TextAdd(benchRow(i, j, k), b)
			}
			b = TextAdd("]", b)
		}
		b = TextAdd("]", b)
	}
	return TextBuild(TextAdd("]", b))
}

// BenchmarkTextBuild is BenchmarkTextJoinNested's output through Text.Builder: each byte is
// copied once into the result, after the row interpolation that makes it.
func BenchmarkTextBuild(b *testing.B) {
	out := renderBuild()
	if out != renderJoinNested() {
		b.Fatal("builder and joins disagree")
	}
	b.ReportAllocs()
	b.SetBytes(int64(len(out)))
	for b.Loop() {
		out = renderBuild()
	}
	_ = out
}

func TestTextBuilder(t *testing.T) {
	empty := TextBuilder()
	eq(t, "zero value", empty, rt.TextBuilder{})
	eq(t, "build empty", TextBuild(empty), "")
	b := TextAdd("c", TextAdd("b", TextAdd("a", empty)))
	eq(t, "three adds", TextBuild(b), "abc")
	eq(t, "add empty is b", TextAdd("", b), b)
	eq(t, "add empty to empty", TextAdd("", empty), empty)
	eq(t, "build twice", TextBuild(b), "abc")
	eq(t, "still usable", TextBuild(TextAdd("d", b)), "abcd")
	b0 := TextAdd("x", empty)
	b1, b2 := TextAdd("a", b0), TextAdd("b", b0)
	eq(t, "fork a", TextBuild(b1), "xa")
	eq(t, "fork b", TextBuild(b2), "xb")
	eq(t, "fork base", TextBuild(b0), "x")
}

func TestTextBuilder_Million(t *testing.T) {
	b := TextBuilder()
	for range 1_000_000 {
		b = TextAdd("ab", b)
	}
	s := TextBuild(b)
	if len(s) != 2_000_000 || s[:4] != "abab" || s[len(s)-2:] != "ab" {
		t.Fatalf("len %d", len(s))
	}
}

func TestTextBuilder_EqKeyShow(t *testing.T) {
	ab := TextAdd("b", TextAdd("a", TextBuilder()))
	ab2 := TextAdd("ab", TextBuilder())
	if !rt.Eq(ab, ab2) || rt.Eq(ab, TextAdd("x", ab2)) || rt.Eq(TextBuilder(), ab) {
		t.Fatal("Eq on builders compares their text")
	}
	if !rt.Eq([]rt.TextBuilder{ab}, []rt.TextBuilder{ab2}) {
		t.Fatal("Eq inside a list")
	}
	if rt.Key(ab) != rt.Key(ab2) || rt.Key(ab) == rt.Key(TextBuilder()) {
		t.Fatal("Key on builders is their text")
	}
	eq(t, "show empty", rt.Show(TextBuilder()), "Text.builder")
	eq(t, "show ab", rt.Show(ab), `Text.add "ab" Text.builder`)
	eq(t, "show as arg", rt.Show(rt.Just(ab)), `Just (Text.add "ab" Text.builder)`)
	eq(t, "show quotes", rt.Show(TextAdd("a\"\n", TextBuilder())), `Text.add "a\"\n" Text.builder`)
}
