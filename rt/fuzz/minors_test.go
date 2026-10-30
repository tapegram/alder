package fuzz

import (
	"math"
	"testing"
)

// A FloatRange whose bounds are infinite and empty (max <= min) is still finite.
func TestFloatRangeInfiniteBounds(t *testing.T) {
	inf := math.Inf(1)
	for _, r := range [][2]float64{{inf, inf}, {-inf, -inf}, {inf, 0}, {0, -inf}, {inf, -inf}, {-inf, inf}} {
		for _, c := range []*Choices{Fresh(1), Replay(make([]uint64, 8))} {
			if x := FloatRange(c, r[0], r[1]); math.IsInf(x, 0) || math.IsNaN(x) {
				t.Errorf("FloatRange%v = %v, want a finite float", r, x)
			}
		}
	}
	if x := FloatRange(Replay(make([]uint64, 8)), 3, 3); x != 3 {
		t.Errorf("FloatRange{3,3} = %v, want 3", x)
	}
}

// Frequency never picks a weight of 0, takes the first positive one when simplest, and (like
// OneOf) fails with a clear message when nothing can be drawn.
func TestFrequencyAndOneOfEmpty(t *testing.T) {
	type alt = struct {
		Weight int64
		Fuzzer Fuzzer[int64]
	}
	f := Frequency([]alt{{0, Constant(int64(1))}, {3, Constant(int64(2))}, {1, Constant(int64(3))}})
	seen := map[int64]bool{}
	c := Fresh(5)
	for range 200 {
		seen[f(c)] = true
	}
	if seen[1] || !seen[2] || !seen[3] {
		t.Errorf("Frequency drew %v", seen)
	}
	if x := f(Replay(make([]uint64, 4))); x != 2 {
		t.Errorf("simplest Frequency = %d, want 2", x)
	}
	for name, draw := range map[string]func(){
		"Fuzz.oneOf":       func() { OneOf[int64](nil)(Fresh(1)) },
		"Fuzz.oneOfValues": func() { OneOfValues[int64](nil)(Fresh(1)) },
		"Fuzz.frequency":   func() { Frequency([]alt{{0, Constant(int64(1))}})(Fresh(1)) },
	} {
		msg, ok := catch(draw).(string)
		if !ok || len(msg) < len(name) || msg[:len(name)] != name {
			t.Errorf("%s on nothing panicked with %v, want a message naming it", name, msg)
		}
	}
}
