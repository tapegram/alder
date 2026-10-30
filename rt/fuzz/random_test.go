package fuzz

import (
	"testing"

	"alder/rt/std"
)

func dice100() std.Generator[int64] {
	return std.RandomIntRange(struct{ Min, Max int64 }{0, 100})
}

// The simplest choice, 0, is the generator's value at Random.seed 0.
func TestFromRandom_ZeroIsSeedZero(t *testing.T) {
	c := Replay([]uint64{0})
	got := FromRandom(dice100())(c)
	want := std.RandomStep(dice100(), std.RandomSeed(0)).Value
	if got != want || len(c.Recorded()) != 1 {
		t.Fatalf("got %d (%d choices), want %d", got, len(c.Recorded()), want)
	}
	// the choice is the seed's state, the full 64 bits
	c = Replay([]uint64{^uint64(0)})
	if got, want := FromRandom(dice100())(c), std.RandomStep(dice100(), std.RandomSeed(-1)).Value; got != want {
		t.Fatalf("max choice: %d, want %d", got, want)
	}
}

// Shrinking lowers the one choice (the seed): a failing `n < 50` ends at a seed whose value is
// >= 50 and whose predecessor's is not, the shrinker's binary search being exact on a single
// choice; a direct scan confirms both, and that the minimal seed's value is the scan's.
func TestFromRandom_Shrinks(t *testing.T) {
	gen := FromRandom(dice100())
	fails := func(n int64) bool { return n >= 50 }
	seq := failing(t, gen, fails)
	best, steps := Shrink(seq, replaying(gen, fails), 1000)
	if len(best) != 1 {
		t.Fatalf("shrunk to %v", best)
	}
	at := func(s uint64) int64 { return std.RandomStep(dice100(), std.Seed{State: s}).Value }
	if !fails(at(best[0])) || (best[0] > 0 && fails(at(best[0]-1))) {
		t.Fatalf("shrunk to seed %d (value %d, %d steps): not a boundary", best[0], at(best[0]), steps)
	}
	var first uint64
	for fails(at(first)) == false {
		first++
	}
	if best[0] != first {
		t.Logf("shrunk to seed %d; the smallest failing seed is %d", best[0], first)
	}
}
