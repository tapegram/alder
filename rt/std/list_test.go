package std

import (
	"fmt"
	"math"
	"testing"

	"alder/rt"
)

func TestList_Batch6(t *testing.T) {
	eq(t, "indexedMap", ListIndexedMap(func(i int64, s string) string { return fmt.Sprint(i) + s }, []string{"a", "b"}), []string{"0a", "1b"})
	xs := []int64{1, 2, 3}
	eq(t, "map2 shortest", ListMap2(func(a, b int64) int64 { return a + b }, xs, []int64{10, 20}), []int64{11, 22})
	eq(t, "map2 input", xs, []int64{1, 2, 3})
	eq(t, "map3 shortest", ListMap3(func(a, b, c int64) int64 { return a + b + c }, []int64{1, 2}, []int64{10, 20}, []int64{100}), []int64{111})
	eq(t, "foldr order", ListFoldr(func(x string, acc string) string { return acc + x }, "", []string{"a", "b", "c"}), "cba")
	in := []int64{1, 2}
	eq(t, "concatMap", ListConcatMap(func(x int64) []int64 { return []int64{x, x} }, in), []int64{1, 1, 2, 2})
	eq(t, "concatMap input", in, []int64{1, 2})
	eq(t, "intersperse", ListIntersperse(int64(0), []int64{1, 2, 3}), []int64{1, 0, 2, 0, 3})
	eq(t, "intersperse one", ListIntersperse(int64(0), []int64{1}), []int64{1})
	eq(t, "intersperse empty", ListIntersperse(int64(0), []int64{}), []int64{})
	p := ListPartition(func(x int64) bool { return x%2 == 0 }, []int64{1, 2, 3, 4})
	eq(t, "partition yes", p.Yes, []int64{2, 4})
	eq(t, "partition no", p.No, []int64{1, 3})
	tin := []int64{1, 2}
	tl := ListTail(tin)
	eq(t, "tail", tl, rt.Just([]int64{2}))
	tl.Val[0] = 99
	eq(t, "tail input", tin, []int64{1, 2})
	eq(t, "tail empty", ListTail([]int64{}), rt.Nothing[[]int64]())
	eq(t, "repeat", ListRepeat(3, "x"), []string{"x", "x", "x"})
	eq(t, "repeat negative", ListRepeat(-1, "x"), []string{})
	eq(t, "maximum", ListMaximum([]int64{3, 9, 2}), rt.Just[int64](9))
	eq(t, "minimum", ListMinimum([]int64{3, 9, 2}), rt.Just[int64](2))
	eq(t, "maximum empty", ListMaximum([]int64{}), rt.Nothing[int64]())
	eq(t, "maximumFloat", ListMaximumFloat([]float64{0.5, 2.5}), rt.Just(2.5))
	eq(t, "minimumFloat empty", ListMinimumFloat(nil), rt.Nothing[float64]())
	// NaN counts lowest (cmp.Compare): it never wins a maximum unless alone, and is the minimum.
	eq(t, "maximumFloat skips NaN", ListMaximumFloat([]float64{math.NaN(), 1}), rt.Just(1.0))
	eq(t, "maximumFloat NaN last", ListMaximumFloat([]float64{1, math.NaN()}), rt.Just(1.0))
	if m := ListMaximumFloat([]float64{math.NaN(), math.NaN()}); !m.Ok || !math.IsNaN(m.Val) {
		t.Error("maximumFloat all-NaN should be NaN")
	}
	if m := ListMinimumFloat([]float64{1, math.NaN()}); !m.Ok || !math.IsNaN(m.Val) {
		t.Error("minimumFloat should be NaN when a NaN is present")
	}
	eq(t, "minimumFloat no NaN", ListMinimumFloat([]float64{2, 1}), rt.Just(1.0))
}

func TestList_Batch6SortByFloat(t *testing.T) {
	type it struct {
		k float64
		n string
	}
	got := ListSortByFloat(func(i it) float64 { return i.k }, []it{{2, "a"}, {1, "b"}, {2, "c"}, {1, "d"}})
	eq(t, "stable", got, []it{{1, "b"}, {1, "d"}, {2, "a"}, {2, "c"}})
	got = ListSortByFloat(func(i it) float64 { return i.k }, []it{{1, "one"}, {math.NaN(), "nan"}, {0, "zero"}})
	eq(t, "nan len", len(got), 3)
	eq(t, "nan order", []string{got[0].n, got[1].n, got[2].n}, []string{"nan", "zero", "one"})
}
