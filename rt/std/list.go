// Package std implements the Alder standard library (effects.go: the effectful part). Each Alder
// function `Module.name` is the Go function `<Module><Name>`, taking the
// Alder parameters in order with mapped types. Zero-parameter Alder values
// (Dict.empty, Time.epoch, Html.none) are Go functions of no arguments.
// Anonymous Alder records map to Go anonymous structs whose fields are the
// PascalCased Alder fields in declaration order.
package std

import (
	"cmp"
	"slices"

	"alder/rt"
)

func ListLength[A any](xs []A) int64 { return int64(len(xs)) }

func ListIsEmpty[A any](xs []A) bool { return len(xs) == 0 }

func ListHead[A any](xs []A) rt.Maybe[A] {
	if len(xs) == 0 {
		return rt.Nothing[A]()
	}
	return rt.Just(xs[0])
}

func ListLast[A any](xs []A) rt.Maybe[A] {
	if len(xs) == 0 {
		return rt.Nothing[A]()
	}
	return rt.Just(xs[len(xs)-1])
}

// clampLen bounds n to [0, len].
func clampLen(n int64, length int) int {
	return int(min(max(n, 0), int64(length)))
}

// ListTake keeps the first n elements (none for n <= 0). The result is a view of xs: O(1),
// no allocation (#37). Sharing is safe because no list Alder code can see is ever written to:
// Alder has no assignment, gen never emits an index store on a list, and the runtime only
// writes into slices it has just made (TestNoWritesToParameters in rt pins this). The view's
// capacity is its length, so an append to it must reallocate rather than write into xs.
func ListTake[A any](n int64, xs []A) []A {
	k := clampLen(n, len(xs))
	return xs[:k:k]
}

// ListDrop removes the first n elements (none for n <= 0): a view of xs, as ListTake.
func ListDrop[A any](n int64, xs []A) []A {
	k := clampLen(n, len(xs))
	return xs[k:len(xs):len(xs)]
}

func ListReverse[A any](xs []A) []A {
	out := append([]A{}, xs...)
	slices.Reverse(out)
	return out
}

func ListMap[A, B any](f func(A) B, xs []A) []B {
	out := make([]B, len(xs))
	for i, x := range xs {
		out[i] = f(x)
	}
	return out
}

func ListFilter[A any](f func(A) bool, xs []A) []A {
	out := []A{}
	for _, x := range xs {
		if f(x) {
			out = append(out, x)
		}
	}
	return out
}

func ListFilterMap[A, B any](f func(A) rt.Maybe[B], xs []A) []B {
	out := []B{}
	for _, x := range xs {
		if m := f(x); m.Ok {
			out = append(out, m.Val)
		}
	}
	return out
}

func ListFind[A any](f func(A) bool, xs []A) rt.Maybe[A] {
	for _, x := range xs {
		if f(x) {
			return rt.Just(x)
		}
	}
	return rt.Nothing[A]()
}

func ListAny[A any](f func(A) bool, xs []A) bool { return slices.ContainsFunc(xs, f) }

func ListAll[A any](f func(A) bool, xs []A) bool {
	for _, x := range xs {
		if !f(x) {
			return false
		}
	}
	return true
}

func ListMember[A any](x A, xs []A) bool {
	return slices.ContainsFunc(xs, func(y A) bool { return rt.Eq(x, y) })
}

// ListFoldl folds from the left: f x1 acc, then f x2 that, ...
func ListFoldl[A, B any](f func(A, B) B, acc B, xs []A) B {
	for _, x := range xs {
		acc = f(x, acc)
	}
	return acc
}

func ListSum(xs []int64) int64 {
	var s int64
	for _, x := range xs {
		s += x
	}
	return s
}

func ListSumFloat(xs []float64) float64 {
	var s float64
	for _, x := range xs {
		s += x
	}
	return s
}

// ListSortBy sorts ascending by key; equal keys keep their order.
func ListSortBy[A any](key func(A) int64, xs []A) []A { return sortByKey(key, xs) }

// ListSortByText sorts ascending by key (byte order); equal keys keep their order.
func ListSortByText[A any](key func(A) string, xs []A) []A { return sortByKey(key, xs) }

func sortByKey[A any, K cmp.Ordered](key func(A) K, xs []A) []A {
	type keyed struct {
		k K
		x A
	}
	ks := make([]keyed, len(xs))
	for i, x := range xs {
		ks[i] = keyed{key(x), x}
	}
	slices.SortStableFunc(ks, func(a, b keyed) int { return cmp.Compare(a.k, b.k) })
	out := make([]A, len(xs))
	for i, k := range ks {
		out[i] = k.x
	}
	return out
}

func ListIndexed[A any](xs []A) []struct {
	Index int64
	Item  A
} {
	out := make([]struct {
		Index int64
		Item  A
	}, len(xs))
	for i, x := range xs {
		out[i].Index, out[i].Item = int64(i), x
	}
	return out
}

// ListRange is inclusive at both ends; empty when from > to.
func ListRange(from, to int64) []int64 {
	out := []int64{}
	for i := from; i <= to; i++ {
		out = append(out, i)
		if i == to { // avoid overflow at MaxInt64
			break
		}
	}
	return out
}

func ListConcat[A any](xss [][]A) []A {
	out := []A{}
	for _, xs := range xss {
		out = append(out, xs...)
	}
	return out
}

func ListAppend[A any](a, b []A) []A { return rt.Append(a, b) }

// ListUnique keeps the first occurrence of each element. It hashes rt.Key, so
// it is linear and agrees with Eq, except that every NaN is one element.
func ListUnique[A any](xs []A) []A {
	out := []A{}
	seen := map[string]struct{}{}
	for _, x := range xs {
		k := rt.Key(x)
		if _, ok := seen[k]; !ok {
			seen[k] = struct{}{}
			out = append(out, x)
		}
	}
	return out
}

// ListGroupBy groups by key; keys in first-seen order, items in input order.
func ListGroupBy[A any](key func(A) string, xs []A) rt.Dict[string, []A] {
	var order []string
	groups := map[string][]A{}
	for _, x := range xs {
		k := key(x)
		if _, ok := groups[k]; !ok {
			order = append(order, k)
		}
		groups[k] = append(groups[k], x)
	}
	var d rt.Dict[string, []A]
	for _, k := range order {
		d.Put(k, groups[k])
	}
	return d
}

// ListIndexedMap maps with the zero-based index.
func ListIndexedMap[A, B any](f func(int64, A) B, xs []A) []B {
	out := make([]B, len(xs))
	for i, x := range xs {
		out[i] = f(int64(i), x)
	}
	return out
}

// ListMap2 stops at the shorter list.
func ListMap2[A, B, C any](f func(A, B) C, xs []A, ys []B) []C {
	out := make([]C, min(len(xs), len(ys)))
	for i := range out {
		out[i] = f(xs[i], ys[i])
	}
	return out
}

// ListMap3 stops at the shortest list.
func ListMap3[A, B, C, D any](f func(A, B, C) D, xs []A, ys []B, zs []C) []D {
	out := make([]D, min(len(xs), len(ys), len(zs)))
	for i := range out {
		out[i] = f(xs[i], ys[i], zs[i])
	}
	return out
}

// ListFoldr folds from the right: f xn acc, then f xn-1 that, ...
func ListFoldr[A, B any](f func(A, B) B, acc B, xs []A) B {
	for i := len(xs) - 1; i >= 0; i-- {
		acc = f(xs[i], acc)
	}
	return acc
}

func ListConcatMap[A, B any](f func(A) []B, xs []A) []B {
	out := []B{}
	for _, x := range xs {
		out = append(out, f(x)...)
	}
	return out
}

func ListIntersperse[A any](sep A, xs []A) []A {
	out := make([]A, 0, max(2*len(xs)-1, 0))
	for i, x := range xs {
		if i > 0 {
			out = append(out, sep)
		}
		out = append(out, x)
	}
	return out
}

// ListPartition splits by the predicate; both halves keep input order.
func ListPartition[A any](f func(A) bool, xs []A) struct {
	Yes []A
	No  []A
} {
	r := struct {
		Yes []A
		No  []A
	}{[]A{}, []A{}}
	for _, x := range xs {
		if f(x) {
			r.Yes = append(r.Yes, x)
		} else {
			r.No = append(r.No, x)
		}
	}
	return r
}

// ListTail drops the first element (Nothing for an empty list); the result is a copy.
func ListTail[A any](xs []A) rt.Maybe[[]A] {
	if len(xs) == 0 {
		return rt.Nothing[[]A]()
	}
	return rt.Just(ListDrop(1, xs))
}

// ListRepeat makes n copies of x (none for n <= 0).
func ListRepeat[A any](n int64, x A) []A {
	if n <= 0 {
		return []A{}
	}
	out := make([]A, n)
	for i := range out {
		out[i] = x
	}
	return out
}

func extreme[T cmp.Ordered](xs []T, better func(a, b T) bool) rt.Maybe[T] {
	if len(xs) == 0 {
		return rt.Nothing[T]()
	}
	m := xs[0]
	for _, x := range xs[1:] {
		if better(x, m) {
			m = x
		}
	}
	return rt.Just(m)
}

func ListMaximum(xs []int64) rt.Maybe[int64] {
	return extreme(xs, func(a, b int64) bool { return a > b })
}

func ListMinimum(xs []int64) rt.Maybe[int64] {
	return extreme(xs, func(a, b int64) bool { return a < b })
}

// ListMaximumFloat orders NaN lowest (as cmp.Compare does), so it only wins in an all-NaN list.
func ListMaximumFloat(xs []float64) rt.Maybe[float64] {
	return extreme(xs, func(a, b float64) bool { return cmp.Compare(a, b) > 0 })
}

// ListMinimumFloat orders NaN lowest (as cmp.Compare does), so a NaN element is the minimum.
func ListMinimumFloat(xs []float64) rt.Maybe[float64] {
	return extreme(xs, func(a, b float64) bool { return cmp.Compare(a, b) < 0 })
}

// ListSortByFloat sorts ascending by key; equal keys keep their order, NaN keys sort first.
func ListSortByFloat[A any](key func(A) float64, xs []A) []A { return sortByKey(key, xs) }
