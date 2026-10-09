// Package std implements the pure Alder standard library. Each Alder
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

// ListTake keeps the first n elements (none for n <= 0).
func ListTake[A any](n int64, xs []A) []A {
	return append([]A{}, xs[:clampLen(n, len(xs))]...)
}

// ListDrop removes the first n elements (none for n <= 0).
func ListDrop[A any](n int64, xs []A) []A {
	return append([]A{}, xs[clampLen(n, len(xs)):]...)
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

// ListUnique keeps the first occurrence of each element.
func ListUnique[A any](xs []A) []A {
	out := []A{}
	for _, x := range xs {
		if !ListMember(x, out) {
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
