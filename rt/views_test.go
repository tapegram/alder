package rt

import "testing"

// `++` with an empty side is the other side, capped so that appending to it reallocates (#37).
func TestAppend_EmptySide(t *testing.T) {
	for name, f := range map[string]func(xs []int64) []int64{
		"[] ++ xs":  func(xs []int64) []int64 { return Append([]int64{}, xs) },
		"nil ++ xs": func(xs []int64) []int64 { return Append(nil, xs) },
		"xs ++ []":  func(xs []int64) []int64 { return Append(xs, []int64{}) },
		"xs ++ nil": func(xs []int64) []int64 { return Append(xs, nil) },
	} {
		xs := make([]int64, 2, 8)
		xs[0], xs[1] = 1, 2
		got := f(xs)
		if !Eq(got, []int64{1, 2}) || cap(got) != len(got) {
			t.Errorf("%s = %v (cap %d)", name, got, cap(got))
		}
		_ = append(got, 9)
		if xs[:3][2] != 0 {
			t.Errorf("%s: append to the result wrote into xs", name)
		}
	}
	a, b := []int64{1}, []int64{2}
	got := Append(a, b)
	if !Eq(got, []int64{1, 2}) || &got[0] == &a[0] {
		t.Fatalf("[1] ++ [2] = %v, must be fresh", got)
	}
}
