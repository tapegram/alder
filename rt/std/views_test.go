package std

import (
	"testing"

	"alder/rt"
)

// spare is [1, 2, 3] with spare capacity behind it, so a view that kept that capacity would let
// an append write into it.
func spare() []int64 {
	xs := make([]int64, 3, 10)
	copy(xs, []int64{1, 2, 3})
	return xs
}

func TestListViews_Nil(t *testing.T) {
	var none []int64
	for name, got := range map[string][]int64{
		"take nil": ListTake(2, none), "drop nil": ListDrop(2, none),
		"take 0": ListTake(0, []int64{1}), "drop all": ListDrop(5, []int64{1}),
	} {
		if !rt.Eq(got, []int64{}) || len(got) != 0 {
			t.Errorf("%s = %v, want []", name, got)
		}
	}
	eq(t, "tail nil", ListTail(none), rt.Nothing[[]int64]())
	tl := ListTail([]int64{1})
	if !tl.Ok || len(tl.Val) != 0 {
		t.Fatalf("tail [1] = %v, want Just []", tl)
	}
}

// Appending to a view must reallocate: a view never has spare capacity, so it cannot write into
// the list it was cut from (#37; the "no writes" rule is pinned by rt's TestNoWritesToParameters).
func TestListViews_NoAlias(t *testing.T) {
	views := map[string]func([]int64) []int64{
		"take":     func(xs []int64) []int64 { return ListTake(2, xs) },
		"drop":     func(xs []int64) []int64 { return ListDrop(1, xs) },
		"tail":     func(xs []int64) []int64 { return ListTail(xs).Val },
		"take all": func(xs []int64) []int64 { return ListTake(9, xs) },
		"drop 0":   func(xs []int64) []int64 { return ListDrop(0, xs) },
		"++ empty": func(xs []int64) []int64 { return ListAppend(xs, nil) },
	}
	for name, view := range views {
		xs := spare()
		v := view(xs)
		if cap(v) != len(v) {
			t.Errorf("%s: cap %d != len %d", name, cap(v), len(v))
		}
		_ = append(v, 99)
		if !rt.Eq(xs, []int64{1, 2, 3}) || xs[:4][3] != 0 {
			t.Errorf("%s: append to the view changed the source: %v", name, xs[:4])
		}
	}
}

// take, drop and tail are views of their input: O(1), no allocation.
func TestListViews_Share(t *testing.T) {
	xs := spare()
	if &ListTake(2, xs)[0] != &xs[0] || &ListDrop(1, xs)[0] != &xs[1] || &ListTail(xs).Val[0] != &xs[1] {
		t.Fatal("take/drop/tail copied their input")
	}
	ys := []int64{5}
	if &ListAppend(ys, nil)[0] != &ys[0] || &ListAppend(nil, ys)[0] != &ys[0] {
		t.Fatal("++ with an empty side copied the other side")
	}
	if n := testing.AllocsPerRun(10, func() { _, _, _ = ListTake(2, xs), ListDrop(1, xs), ListTail(xs) }); n != 0 {
		t.Fatalf("views allocated %v times", n)
	}
}

// Appending to the source (into its spare capacity) leaves views cut earlier as they were.
func TestListViews_SourceAppend(t *testing.T) {
	xs := spare()
	tk, dr, tl := ListTake(2, xs), ListDrop(1, xs), ListTail(xs).Val
	_ = append(xs, 4, 5)
	eq(t, "take", tk, []int64{1, 2})
	eq(t, "drop", dr, []int64{2, 3})
	eq(t, "tail", tl, []int64{2, 3})
}

// rec48 is a 48-byte record, the size of a typical result row.
type rec48 struct{ A, B, C, D, E, F int64 }

// BenchmarkListTakeDrop takes and drops half of 100k records (#37: views, so O(1) and no allocation).
func BenchmarkListTakeDrop(b *testing.B) {
	xs := make([]rec48, 100_000)
	for i := range xs {
		xs[i].A = int64(i)
	}
	b.ReportAllocs()
	var n int
	for b.Loop() {
		n += len(ListTake(50_000, xs)) + len(ListDrop(50_000, xs))
	}
	_ = n
}
