package fuzz

import (
	"slices"
	"testing"
)

// failing finds a seed whose fresh draw fails, and returns its recorded sequence.
func failing[A any](t *testing.T, gen Fuzzer[A], fails func(A) bool) []uint64 {
	t.Helper()
	for s := uint64(1); s < 100000; s++ {
		c := Fresh(s)
		var failed bool
		if catch(func() { failed = fails(gen(c)) }) == nil && failed {
			return c.Recorded()
		}
	}
	t.Fatal("no failing seed")
	return nil
}

// replaying turns a property over values into one over choice sequences, as the runner does.
func replaying[A any](gen Fuzzer[A], fails func(A) bool) func([]uint64) bool {
	return func(seq []uint64) bool { return fails(gen(Replay(seq))) }
}

func TestShrinkList(t *testing.T) {
	gen := List(IntF())
	fails := func(xs []int64) bool { return slices.ContainsFunc(xs, func(x int64) bool { return x >= 10 }) }
	for _, longer := range []int{0, 2, 4} {
		seq := failing(t, gen, func(xs []int64) bool { return fails(xs) && len(xs) > longer })
		best, steps := Shrink(seq, replaying(gen, fails), 1000)
		if got := gen(Replay(best)); !slices.Equal(got, []int64{10}) {
			t.Fatalf("shrunk %v to %v (%d steps), want [10]", gen(Replay(seq)), got, steps)
		}
	}
}

func TestShrinkPair(t *testing.T) {
	gen := Pair(IntF(), IntF())
	fails := func(p struct{ First, Second int64 }) bool { return p.First+p.Second >= 100 }
	for _, bigger := range []int64{100, 1000, 1 << 40} {
		seq := failing(t, gen, func(p struct{ First, Second int64 }) bool {
			return fails(p) && p.First > 0 && p.Second > 0 && p.First+p.Second > bigger
		})
		best, steps := Shrink(seq, replaying(gen, fails), 1000)
		p := gen(Replay(best))
		if p.First+p.Second != 100 || (p.First != 0 && p.Second != 0) {
			t.Fatalf("shrunk %v to %v (%d steps), want a pair summing to 100 with one side 0", gen(Replay(seq)), p, steps)
		}
	}
}

func TestShrinkIgnoresOverrunAndReject(t *testing.T) {
	gen := Filter("odd", func(n int64) bool { return n%2 != 0 }, IntF())
	fails := func(n int64) bool { return n > 50 }
	seq := failing(t, gen, fails)
	best, _ := Shrink(seq, replaying(gen, fails), 1000)
	if n := gen(Replay(best)); n != 51 {
		t.Fatalf("shrunk to %d, want 51", n)
	}
}

// TestShrinkFromManySeeds shrinks every failing fresh draw of the first 1000 seeds: the
// minimum must not depend on where shrinking starts.
func TestShrinkFromManySeeds(t *testing.T) {
	listGen := List(IntF())
	listFails := func(xs []int64) bool { return slices.ContainsFunc(xs, func(x int64) bool { return x >= 10 }) }
	pairGen := Pair(IntF(), IntF())
	pairFails := func(p struct{ First, Second int64 }) bool { return p.First+p.Second >= 100 }
	oddGen := Filter("odd", func(n int64) bool { return n%2 != 0 }, IntF())
	oddFails := func(n int64) bool { return n > 50 }
	for s := uint64(1); s <= 1000; s++ {
		if c := Fresh(s); listFails(listGen(c)) {
			best, _ := Shrink(c.Recorded(), replaying(listGen, listFails), 1000)
			if got := listGen(Replay(best)); !slices.Equal(got, []int64{10}) {
				t.Fatalf("seed %d: list shrunk to %v", s, got)
			}
		}
		if c := Fresh(s); pairFails(pairGen(c)) {
			best, _ := Shrink(c.Recorded(), replaying(pairGen, pairFails), 1000)
			if p := pairGen(Replay(best)); p.First+p.Second != 100 || (p.First != 0 && p.Second != 0) {
				t.Fatalf("seed %d: pair shrunk to %v", s, p)
			}
		}
		c := Fresh(s)
		var n int64
		if catch(func() { n = oddGen(c) }) == nil && oddFails(n) {
			best, _ := Shrink(c.Recorded(), replaying(oddGen, oddFails), 1000)
			if m := oddGen(Replay(best)); m != 51 {
				t.Fatalf("seed %d: odd %d shrunk to %d", s, n, m)
			}
		}
	}
}

func TestShrinkBudget(t *testing.T) {
	seq := make([]uint64, 200)
	for i := range seq {
		seq[i] = uint64(i*7919)%1000 + 1
	}
	calls := 0
	_, steps := Shrink(seq, func([]uint64) bool { calls++; return true }, 1000)
	if steps > 1000 || calls != steps {
		t.Fatalf("steps %d, calls %d: want steps == calls <= 1000", steps, calls)
	}
}
