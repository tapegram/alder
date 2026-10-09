package rt

import (
	"reflect"
	"testing"
)

func TestMaybe_OrElse(t *testing.T) {
	if got := OrElse(Just[int64](3), 0); got != 3 {
		t.Fatalf("OrElse(Just 3) = %d", got)
	}
	if got := OrElse(Nothing[int64](), 7); got != 7 {
		t.Fatalf("OrElse(Nothing) = %d", got)
	}
	var zero Maybe[string]
	if zero != Nothing[string]() {
		t.Fatal("zero Maybe is not Nothing")
	}
}

func TestResult_Constructors(t *testing.T) {
	ok := Ok[string, int64](1)
	if !ok.IsOk || ok.Val != 1 {
		t.Fatalf("Ok = %+v", ok)
	}
	e := Err[string, int64]("bad")
	if e.IsOk || e.Err != "bad" {
		t.Fatalf("Err = %+v", e)
	}
}

func TestDict_InsertionOrderAndGet(t *testing.T) {
	var d Dict[string, int64]
	d = d.Insert("b", 1).Insert("a", 2).Insert("c", 3)
	d2 := d.Insert("b", 10) // update keeps position, does not touch d
	if got := d2.Keys(); !reflect.DeepEqual(got, []string{"b", "a", "c"}) {
		t.Fatalf("Keys = %v", got)
	}
	if got := d2.Values(); !reflect.DeepEqual(got, []int64{10, 2, 3}) {
		t.Fatalf("Values = %v", got)
	}
	if v, ok := d.Get("b"); !ok || v != 1 {
		t.Fatalf("original dict mutated: b = %d, %v", v, ok)
	}
	if _, ok := d.Get("zz"); ok {
		t.Fatal("Get of a missing key reported ok")
	}
	d3 := d2.Remove("a")
	if got := d3.Keys(); !reflect.DeepEqual(got, []string{"b", "c"}) {
		t.Fatalf("Keys after Remove = %v", got)
	}
	if d2.Len() != 3 || d3.Len() != 2 {
		t.Fatalf("Len = %d, %d", d2.Len(), d3.Len())
	}
	if !d3.Has("c") || d3.Has("a") {
		t.Fatal("Has wrong after Remove")
	}
}

func TestConcatAppendUnit(t *testing.T) {
	if got := Concat("a", "", "bc", "é"); got != "abcé" {
		t.Fatalf("Concat = %q", got)
	}
	a := make([]int64, 1, 10)
	a[0] = 1
	x := Append(a, []int64{2})
	y := Append(a, []int64{3})
	if !reflect.DeepEqual(x, []int64{1, 2}) || !reflect.DeepEqual(y, []int64{1, 3}) {
		t.Fatalf("Append aliased: %v %v", x, y)
	}
	if Unit() != (struct{}{}) {
		t.Fatal("Unit")
	}
}

func TestDate_Format(t *testing.T) {
	if got := Format("Jan 2, 2006", Date{2026, 10, 9}); got != "Oct 9, 2026" {
		t.Fatalf("Format = %q", got)
	}
	if got := Format("2006-01-02", Date{2026, 1, 5}); got != "2026-01-05" {
		t.Fatalf("Format ISO = %q", got)
	}
}
