package fuzz

import (
	"math"
	"testing"
	"unicode/utf8"

	"alder/rt"
)

func TestTextPrintable(t *testing.T) {
	sawUnicode, sawEmpty := false, false
	c := Fresh(7)
	for i := range 10000 {
		if i%500 == 0 {
			c = Fresh(uint64(i) + 7) // keep each sequence well under MaxChoices
		}
		s := Text(c)
		if !utf8.ValidString(s) {
			t.Fatalf("invalid UTF-8: %q", s)
		}
		sawEmpty = sawEmpty || s == ""
		for _, r := range s {
			if r < 0x20 && r != '\n' && r != '\t' && r != '\r' {
				t.Fatalf("control rune %U in %q", r, s)
			}
			if r >= 0x7f && r <= 0x9f {
				t.Fatalf("DEL/C1 rune %U in %q", r, s)
			}
			sawUnicode = sawUnicode || r > 0x7f
		}
	}
	if !sawUnicode || !sawEmpty {
		t.Fatalf("unicode %v, empty %v: want both", sawUnicode, sawEmpty)
	}
}

func TestFloatFinite(t *testing.T) {
	zero, negZero, huge := false, false, false
	c := Fresh(3)
	for i := range 10000 {
		if i%1000 == 0 {
			c = Fresh(uint64(i) + 3)
		}
		x := Float(c)
		if math.IsNaN(x) || math.IsInf(x, 0) {
			t.Fatalf("non-finite %v", x)
		}
		zero = zero || (x == 0 && !math.Signbit(x))
		negZero = negZero || (x == 0 && math.Signbit(x))
		huge = huge || math.Abs(x) > 1e300
	}
	if !zero || !negZero || !huge {
		t.Fatalf("0 %v, -0 %v, huge %v: want all", zero, negZero, huge)
	}
}

func TestIntMostlySmallSometimesExtreme(t *testing.T) {
	small, extreme := 0, 0
	c := Fresh(11)
	for i := range 2000 {
		if i%500 == 0 {
			c = Fresh(uint64(i) + 11)
		}
		n := Int(c)
		if n >= -100 && n <= 100 {
			small++
		}
		if n == math.MinInt64 || n == math.MaxInt64 {
			extreme++
		}
	}
	if small < 1000 || extreme == 0 {
		t.Fatalf("small %d of 2000, extreme %d", small, extreme)
	}
}

func TestRangesStayInRange(t *testing.T) {
	var c *Choices
	for i := range 3000 {
		if i%100 == 0 {
			c = Fresh(uint64(i))
		}
		for _, r := range [][2]int64{{-3, 10}, {5, 9}, {-9, -5}, {math.MinInt64, math.MaxInt64}, {4, 4}, {0, 1}, {9, 2}} {
			if n := IntRange(c, r[0], r[1]); n < r[0] || n > max(r[0], r[1]) {
				t.Fatalf("IntRange%v = %d", r, n)
			}
		}
		if x := FloatRangeF(struct{ Min, Max float64 }{-1.5, 2.5})(c); x < -1.5 || x > 2.5 {
			t.Fatalf("FloatRange{-1.5,2.5} = %v", x)
		}
		if x := FloatRangeF(struct{ Min, Max float64 }{10, 20})(c); x < 10 || x > 20 {
			t.Fatalf("FloatRange{10,20} = %v", x)
		}
		if x := FloatRangeF(struct{ Min, Max float64 }{-math.MaxFloat64, math.MaxFloat64})(c); math.IsInf(x, 0) || math.IsNaN(x) {
			t.Fatalf("FloatRange{full} = %v", x)
		}
		s := TextOf(struct{ Min, Max int64 }{2, 4})(c)
		if n := utf8.RuneCountInString(s); n < 2 || n > 4 {
			t.Fatalf("TextOf{2,4} = %q (%d code points)", s, n)
		}
		if n := utf8.RuneCountInString(Char()(c)); n != 1 {
			t.Fatalf("Char drew %d code points", n)
		}
		if xs := ListOf(struct{ Min, Max int64 }{1, 3}, IntF())(c); len(xs) < 1 || len(xs) > 3 {
			t.Fatalf("ListOf{1,3} = %v", xs)
		}
	}
}

func TestRangeEdgesAndSimplest(t *testing.T) {
	zeros := func() *Choices { return Replay(make([]uint64, 64)) }
	if n := IntRange(zeros(), -3, 10); n != 0 {
		t.Fatalf("simplest IntRange(-3,10) = %d, want 0", n)
	}
	if n := IntRange(zeros(), 5, 9); n != 5 {
		t.Fatalf("simplest IntRange(5,9) = %d, want 5", n)
	}
	if n := IntRange(zeros(), -9, -5); n != -5 {
		t.Fatalf("simplest IntRange(-9,-5) = %d, want -5", n)
	}
	if x := FloatRangeF(struct{ Min, Max float64 }{2, 3})(zeros()); x != 2 {
		t.Fatalf("simplest FloatRange{2,3} = %v, want 2", x)
	}
	sawMin, sawMax := false, false
	c := Fresh(9)
	for range 500 {
		n := IntRange(c, -3, 1000)
		sawMin, sawMax = sawMin || n == -3, sawMax || n == 1000
	}
	if !sawMin || !sawMax {
		t.Fatalf("IntRange edges: min %v max %v", sawMin, sawMax)
	}
}

func TestSmallerIsSimpler(t *testing.T) {
	zeros := func() *Choices { return Replay(make([]uint64, 64)) }
	if n := Int(zeros()); n != 0 {
		t.Fatalf("Int = %d", n)
	}
	if x := Float(zeros()); x != 0 || math.Signbit(x) {
		t.Fatalf("Float = %v", x)
	}
	if xs := List(IntF())(zeros()); len(xs) != 0 {
		t.Fatalf("List = %v", xs)
	}
	if s := Text(zeros()); s != "" {
		t.Fatalf("Text = %q", s)
	}
	if s := Char()(zeros()); s != "a" {
		t.Fatalf("Char = %q", s)
	}
	if b := BoolF()(zeros()); b {
		t.Fatal("Bool = True")
	}
	if s := OneOf([]Fuzzer[string]{Constant("first"), Constant("second")})(zeros()); s != "first" {
		t.Fatalf("OneOf = %q", s)
	}
	if s := OneOfValues([]string{"a", "b"})(zeros()); s != "a" {
		t.Fatalf("OneOfValues = %q", s)
	}
	if m := Maybe(IntF())(zeros()); m.Ok {
		t.Fatalf("Maybe = %v, want Nothing", m)
	}
	if r := Result(TextF(), IntF())(zeros()); !r.IsOk || r.Val != 0 {
		t.Fatalf("Result = %v, want Ok 0", r)
	}
	if d := Dict(IntF(), IntF())(zeros()); d.Len() != 0 {
		t.Fatalf("Dict has %d entries", d.Len())
	}
	if p := Pair(IntF(), TextF())(zeros()); p.First != 0 || p.Second != "" {
		t.Fatalf("Pair = %v", p)
	}
}

func TestCombinators(t *testing.T) {
	c := Fresh(21)
	for i := range 200 {
		if i%20 == 0 {
			c = Fresh(uint64(i) + 21)
		}
		if n := Map(func(n int64) int64 { return n * 0 }, IntF())(c); n != 0 {
			t.Fatalf("Map = %d", n)
		}
		if s := Map2(func(a, b string) string { return a + b }, Constant("x"), Constant("y"))(c); s != "xy" {
			t.Fatalf("Map2 = %q", s)
		}
		ys := AndThen(func(n int64) Fuzzer[[]bool] {
			return ListOf(struct{ Min, Max int64 }{n, n}, BoolF())
		}, IntRangeF(struct{ Min, Max int64 }{0, 5}))(c)
		if len(ys) > 5 {
			t.Fatalf("AndThen list = %v", ys)
		}
		d := Dict(IntRangeF(struct{ Min, Max int64 }{0, 3}), TextF())(c)
		if d.Len() > 4 {
			t.Fatalf("Dict with %d keys from 4 possible", d.Len())
		}
		var _ rt.Dict[int64, string] = d
		var _ rt.Maybe[int64] = Maybe(IntF())(c)
		var _ rt.Result[string, int64] = Result(TextF(), IntF())(c)
	}
	f := Frequency([]struct {
		Weight int64
		Fuzzer Fuzzer[string]
	}{{0, Constant("never")}, {3, Constant("a")}, {1, Constant("b")}})
	seen := map[string]int{}
	for range 400 {
		seen[f(c)]++
	}
	if seen["never"] > 0 || seen["a"] <= seen["b"] || seen["b"] == 0 {
		t.Fatalf("Frequency counts %v", seen)
	}
	if s := f(Replay([]uint64{0})); s != "a" {
		t.Fatalf("simplest Frequency = %q, want the first positive weight", s)
	}
	v := catch(func() { Filter("Slug", func(int64) bool { return false }, IntF())(c) })
	if v != (Reject{What: "Slug"}) {
		t.Fatalf("Filter panic = %#v", v)
	}
	if n := Filter("any", func(int64) bool { return true }, Constant(int64(4)))(c); n != 4 {
		t.Fatalf("Filter pass = %d", n)
	}
}
