package fuzz

import (
	"testing"
	"time"
	"unicode/utf8"

	"alder/rt/web"
)

// Each stdlib brand's generator draws only values its form check (rt/web, as brandCheck emits
// it) accepts: fresh, and from the simplest (all-zero) sequence.
func TestStdBrandsPassTheirChecks(t *testing.T) {
	texts := []struct {
		name  string
		gen   func(*Choices) string
		check func(string) bool
	}{
		{"NonEmpty", NonEmpty, web.NonEmpty},
		{"Email", Email, web.Email},
		{"URL", URL, web.URL},
		{"Secret", Secret, web.Secret},
	}
	for _, b := range texts {
		seen := map[string]bool{}
		for i := range 1000 {
			s := b.gen(Fresh(uint64(i)))
			if !b.check(s) || !utf8.ValidString(s) {
				t.Fatalf("%s drew %q, which its check rejects", b.name, s)
			}
			seen[s] = true
		}
		if len(seen) < 300 {
			t.Errorf("%s: only %d distinct values in 1000 draws", b.name, len(seen))
		}
		if s := b.gen(zeros()); !b.check(s) {
			t.Errorf("%s: the simplest value %q fails its check", b.name, s)
		}
	}
	ints := []struct {
		name  string
		gen   func(*Choices) int64
		check func(int64) bool
	}{
		{"Positive", Positive, func(n int64) bool { return n > 0 }},
		{"NonNegative", NonNegative, func(n int64) bool { return n >= 0 }},
	}
	for _, b := range ints {
		small := 0
		for i := range 1000 {
			n := b.gen(Fresh(uint64(i)))
			if !b.check(n) {
				t.Fatalf("%s drew %d", b.name, n)
			}
			if n <= 10 {
				small++
			}
		}
		if small < 300 {
			t.Errorf("%s: only %d of 1000 draws are at most 10", b.name, small)
		}
	}
	if Positive(zeros()) != 1 || NonNegative(zeros()) != 0 {
		t.Errorf("simplest Positive %d, NonNegative %d; want 1, 0", Positive(zeros()), NonNegative(zeros()))
	}
}

func TestDeriveHelpers(t *testing.T) {
	// Ctor: non-recursive constructors are the simplest; at depth 0 only they are drawn
	rec := []bool{true, false, true, false}
	if got := Ctor(zeros(), rec, Depth); got != 1 {
		t.Errorf("simplest constructor %d, want 1 (the first non-recursive)", got)
	}
	recursive := map[int]int{}
	for i := range 2000 {
		c := Fresh(uint64(i))
		if k := Ctor(c, rec, 0); rec[k] {
			t.Fatalf("depth 0 drew recursive constructor %d", k)
		}
		for d := 1; d <= Depth; d++ {
			if rec[Ctor(c, rec, d)] {
				recursive[d]++
			}
		}
	}
	for d := 1; d < Depth; d++ {
		if recursive[d] >= recursive[d+1] {
			t.Errorf("recursive constructors at depth %d: %d, at %d: %d; want fewer deeper", d, recursive[d], d+1, recursive[d+1])
		}
	}
	if recursive[1] == 0 {
		t.Error("depth 1 never drew a recursive constructor")
	}
	if got := Ctor(zeros(), []bool{true, true}, 0); got != 0 {
		t.Errorf("all-recursive union at depth 0: %d, want 0", got)
	}

	// Date and Time: the epoch is the simplest; draws stay near the epoch or 2026-01-01
	if d := Date(zeros()); DaysOf(d) != 0 {
		t.Errorf("simplest Date %v, want the epoch", d)
	}
	if tm := Time(zeros()); !tm.Equal(time.UnixMilli(0)) {
		t.Errorf("simplest Time %v, want the epoch", tm)
	}
	near2026, nearEpoch := 0, 0
	for i := range 1000 {
		c := Fresh(uint64(i))
		d, tm := Date(c), Time(c)
		for _, y := range []int{d.Y, tm.UTC().Year()} {
			switch {
			case y >= 1966 && y <= 1974:
				nearEpoch++
			case y >= 2022 && y <= 2030:
				near2026++
			default:
				t.Fatalf("year %d is near neither the epoch nor 2026", y)
			}
		}
		if tm.UnixMilli()*int64(time.Millisecond) != tm.UnixNano() {
			t.Fatalf("Time %v is not whole milliseconds", tm)
		}
	}
	if near2026 < 500 || nearEpoch < 500 {
		t.Errorf("near the epoch %d, near 2026 %d of 2000", nearEpoch, near2026)
	}

	// Bytes: UTF-8 (so it prints as Text.toBytes "…"), empty when simplest
	if b := Bytes(zeros()); len(b) != 0 {
		t.Errorf("simplest Bytes %v", b)
	}
	for i := range 500 {
		if b := Bytes(Fresh(uint64(i))); !utf8.Valid(b) {
			t.Fatalf("Bytes %v is not UTF-8", b)
		}
	}
}

// ListOf clamps a negative Max: `Fuzz.listOf { min: 0, max: -1 }` draws only [].
func TestListOfNegativeMax(t *testing.T) {
	f := ListOf(struct{ Min, Max int64 }{0, -1}, IntF())
	for i := range 200 {
		if xs := f(Fresh(uint64(i))); len(xs) != 0 {
			t.Fatalf("listOf { min: 0, max: -1 } drew %v", xs)
		}
	}
	g := ListOf(struct{ Min, Max int64 }{2, -1}, IntF())
	if xs := g(Fresh(1)); len(xs) != 2 {
		t.Fatalf("listOf { min: 2, max: -1 } drew %v, want 2 elements", xs)
	}
}

// FilterF is Fuzz.filter: its rejections name "Fuzz.filter" (R7).
func TestFilterF(t *testing.T) {
	defer func() {
		if r, ok := recover().(Reject); !ok || r.What != "Fuzz.filter" {
			t.Fatalf("recovered %#v", r)
		}
	}()
	FilterF(func(int64) bool { return false }, IntF())(Fresh(1))
}

// zeros replays the all-zero sequence: every draw its simplest value.
func zeros() *Choices { return Replay(make([]uint64, 64)) }
