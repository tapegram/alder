package std

import (
	"math"
	"slices"
	"testing"

	"alder/rt"
)

// The SplitMix64 reference rows (seed = state; the seed-0 row is Vigna's published sequence).
func TestSplitMix64_Vectors(t *testing.T) {
	for seed, want := range map[uint64][3]uint64{
		0:  {0xe220a8397b1dcdaf, 0x6e789e6aa1b965f4, 0x06c45d188009454f},
		1:  {0x910a2dec89025cc1, 0xbeeb8da1658eec67, 0xf893a2eefb32555e},
		42: {0xbdd732262feb6e95, 0x28efe333b266f103, 0x47526757130f9f52},
	} {
		s := Seed{seed}
		var got [3]uint64
		for i := range got {
			got[i], s = s.next()
		}
		if got != want {
			t.Errorf("seed %d: %x, want %x", seed, got, want)
		}
	}
}

func TestRandomSeed_Show(t *testing.T) {
	s := RandomSeed(-5)
	if s.State != uint64(math.MaxUint64-4) {
		t.Fatalf("state %d", s.State)
	}
	if got := rt.Show(s); got != "Random.seed (-5)" {
		t.Errorf("Show = %q", got)
	}
	if got := rt.Show(rt.Just(s)); got != "Just (Random.seed (-5))" {
		t.Errorf("Show Just = %q", got)
	}
	if got := rt.Show(RandomSeed(7)); got != "Random.seed 7" {
		t.Errorf("Show = %q", got)
	}
}

func TestRandomSeed_Eq(t *testing.T) {
	if !rt.Eq(RandomSeed(-1), Seed{math.MaxUint64}) {
		t.Error("equal states are not Eq")
	}
	if rt.Eq(RandomSeed(1), RandomSeed(2)) {
		t.Error("different states are Eq")
	}
}

func dice() Generator[int64] { return RandomIntRange(struct{ Min, Max int64 }{1, 6}) }

func TestRandomIntRange_Vectors(t *testing.T) {
	r := RandomStep(dice(), RandomSeed(42))
	if r.Value != 2 || r.Seed != RandomSeed(-7046029254386353089) {
		t.Errorf("1..6 from 42 = %d, %s", r.Value, rt.Show(r.Seed))
	}
	if got := rt.Show(r); got != "{ value: 2, seed: Random.seed (-7046029254386353089) }" {
		t.Errorf("Show = %s", got)
	}
	full := RandomStep(RandomIntRange(struct{ Min, Max int64 }{math.MinInt64, math.MaxInt64}), RandomSeed(0))
	if full.Value != -2152535657050944081 {
		t.Errorf("full range from 0 = %d", full.Value)
	}
	five := RandomStep(RandomList(5, dice()), RandomSeed(1))
	if !slices.Equal(five.Value, []int64{6, 2, 1, 6, 4}) {
		t.Errorf("five dice from 1 = %v", five.Value)
	}
}

func TestRandomFloatRange_Vector(t *testing.T) {
	r := RandomStep(RandomFloatRange(struct{ Min, Max float64 }{0, 1}), RandomSeed(42))
	if r.Value != 0.7415648787718233 {
		t.Errorf("0..1 from 42 = %v", r.Value)
	}
}

// one step of the PRNG from s
func stepped(s Seed) Seed { _, n := s.next(); return n }

func TestRandomDegenerate(t *testing.T) {
	s := RandomSeed(9)
	for _, r := range []struct{ Min, Max int64 }{{5, 5}, {5, 1}} {
		got := RandomStep(RandomIntRange(r), s)
		if got.Value != 5 || got.Seed != stepped(s) {
			t.Errorf("intRange %v = %d, seed %v", r, got.Value, got.Seed)
		}
	}
	for _, r := range []struct{ Min, Max float64 }{{2, 2}, {2, 1}, {2, math.NaN()}} {
		got := RandomStep(RandomFloatRange(r), s)
		if got.Value != 2 || got.Seed != stepped(s) {
			t.Errorf("floatRange %v = %v, seed %v", r, got.Value, got.Seed)
		}
	}
	nan := RandomStep(RandomFloatRange(struct{ Min, Max float64 }{math.NaN(), 1}), s)
	if !math.IsNaN(nan.Value) || nan.Seed != stepped(s) {
		t.Errorf("floatRange NaN..1 = %v", nan.Value)
	}
}

func TestRandomConstantNoStep(t *testing.T) {
	r := RandomStep(RandomConstant("x"), RandomSeed(3))
	if r.Value != "x" || r.Seed != RandomSeed(3) {
		t.Errorf("constant = %v", r)
	}
}

func TestRandomListNonPositive(t *testing.T) {
	for _, n := range []int64{0, -3} {
		r := RandomStep(RandomList(n, dice()), RandomSeed(3))
		if r.Value == nil || len(r.Value) != 0 || r.Seed != RandomSeed(3) {
			t.Errorf("list %d = %v, %v", n, r.Value, r.Seed)
		}
	}
}

func TestRandomMap(t *testing.T) {
	r := RandomStep(RandomMap(func(n int64) int64 { return n * 10 }, dice()), RandomSeed(42))
	if r.Value != 20 || r.Seed != stepped(RandomSeed(42)) {
		t.Errorf("map = %v", r)
	}
}

func TestRandomMap2AndThenOrder(t *testing.T) {
	s0 := RandomSeed(11)
	a := RandomStep(dice(), s0)
	b := RandomStep(dice(), a.Seed)
	pair := RandomStep(RandomMap2(func(x, y int64) [2]int64 { return [2]int64{x, y} }, dice(), dice()), s0)
	if pair.Value != [2]int64{a.Value, b.Value} || pair.Seed != b.Seed {
		t.Errorf("map2 = %v, want %d %d", pair, a.Value, b.Value)
	}
	// andThen: the second generator is chosen by the first value and sees the first's seed
	nested := RandomStep(RandomAndThen(func(n int64) Generator[[]int64] { return RandomList(n, dice()) }, dice()), s0)
	want := RandomStep(RandomList(a.Value, dice()), a.Seed)
	if !slices.Equal(nested.Value, want.Value) || nested.Seed != want.Seed {
		t.Errorf("andThen = %v, want %v", nested, want)
	}
}

func TestRandomDice(t *testing.T) {
	var counts [7]int
	s := RandomSeed(3)
	for range 60000 {
		r := RandomStep(dice(), s)
		counts[r.Value]++
		s = r.Seed
	}
	for face := 1; face <= 6; face++ {
		if math.Abs(float64(counts[face])-10000) > 300 {
			t.Errorf("face %d: %d of 60000", face, counts[face])
		}
	}
}

func TestAcklam(t *testing.T) {
	if q := acklam(0.5); q != 0 {
		t.Errorf("q 0.5 = %v", q)
	}
	for p, want := range map[float64]float64{0.975: 1.959964, 0.01: -2.326348, 0.9: 1.281552, 0.001: -3.090232, 0.999: 3.090232} {
		if q := acklam(p); math.Abs(q-want) > 1e-6 {
			t.Errorf("q %v = %v, want %v", p, q, want)
		}
	}
}

func TestRandomNormal_Vector(t *testing.T) {
	r := RandomStep(RandomNormal(struct{ Mean, Sd float64 }{0, 1}), RandomSeed(42))
	if r.Value != 0.6481773615835266 || r.Seed != stepped(RandomSeed(42)) {
		t.Errorf("normal from 42 = %v", r.Value)
	}
	scaled := RandomStep(RandomNormal(struct{ Mean, Sd float64 }{10, 2}), RandomSeed(42))
	if scaled.Value != 10+2*r.Value {
		t.Errorf("normal 10 2 from 42 = %v", scaled.Value)
	}
}

func TestRandomNormal_Moments(t *testing.T) {
	const n = 100000
	g := RandomNormal(struct{ Mean, Sd float64 }{0, 1})
	s := RandomSeed(7)
	var sum, sq float64
	for range n {
		var x float64
		x, s = g(s)
		sum += x
		sq += x * x
	}
	mean := sum / n
	sd := math.Sqrt(sq/n - mean*mean)
	if math.Abs(mean) > 0.01 || math.Abs(sd-1) > 0.01 {
		t.Errorf("mean %v, sd %v", mean, sd)
	}
}

func TestRandomUniform(t *testing.T) {
	s := RandomSeed(42)
	r := RandomStep(RandomUniform("x", []string{}), s)
	if r.Value != "x" || r.Seed != stepped(s) {
		t.Errorf("uniform x [] = %v", r)
	}
	xs := []string{"a", "b", "c", "d", "e", "f"}
	for i := range int64(50) {
		s := RandomSeed(i)
		idx := RandomStep(RandomIntRange(struct{ Min, Max int64 }{0, 5}), s)
		got := RandomStep(RandomUniform(xs[0], xs[1:]), s)
		if got.Value != xs[idx.Value] || got.Seed != idx.Seed {
			t.Fatalf("seed %d: %v, want index %d", i, got.Value, idx.Value)
		}
	}
}

type wv = struct {
	Weight float64
	Value  string
}

func TestRandomWeighted(t *testing.T) {
	s := RandomSeed(42) // unit 0.7415…
	one := func(w wv, ws ...wv) string {
		r := RandomStep(RandomWeighted(w, ws), s)
		if r.Seed != stepped(s) {
			t.Errorf("weighted took other than one step")
		}
		return r.Value
	}
	// t = 0.74 * 4 = 2.97: past |1| (1.97), past |-1| (0.97), within 2
	if got := one(wv{1, "a"}, wv{-1, "b"}, wv{2, "c"}); got != "c" {
		t.Errorf("by absolute weight: %s", got)
	}
	// a negative weight counts as its absolute value: t = 0.74 * 3 = 2.22 > |-2|
	if got := one(wv{-2, "a"}, wv{1, "b"}); got != "b" {
		t.Errorf("negative weight: %s", got)
	}
	if got := one(wv{0, "a"}, wv{0, "b"}, wv{0, "c"}); got != "a" {
		t.Errorf("all zero: %s", got)
	}
	if got := one(wv{5, "only"}); got != "only" {
		t.Errorf("one item: %s", got)
	}
	// rounding: a t past every weight gives the last
	if got := pickWeighted(1.0000001, wv{0.5, "a"}, []wv{{0.5, "b"}}); got != "b" {
		t.Errorf("rounding: %s", got)
	}
	// t = u * total is rounded before the subtraction: fused (FNMSUB on arm64) this picks 2
	r := RandomStep(RandomWeighted(wv{0.3, "0"}, []wv{{0.09213983536241055, "1"}, {0.3, "2"}}), RandomSeed(1))
	if r.Value != "1" {
		t.Errorf("weighted from seed 1 = %s, want 1 (unfused)", r.Value)
	}
}

// Pinned floats, each the unfused IEEE value (checked against a Python transcription): dev
// machines are arm64 and CI amd64, so a platform difference fails here somewhere. The lower
// tail goes through pinnedLog; on these seeds arm64's math.Log differs from it in the last bit.
func TestRandomNormal_TailVectors(t *testing.T) {
	g := RandomNormal(struct{ Mean, Sd float64 }{0, 1})
	for seed, want := range map[int64][2]float64{ // pinnedLog of the unit, the normal
		1853:  {-4.561072978965022, -2.3097557788667946},
		9644:  {-3.727146966532986, -1.9762826664892639},
		10963: {-3.8617956956797084, -2.0329220036779874},
		76276: {-4.722901338046117, -2.370195117052575},
	} {
		o, _ := RandomSeed(seed).next()
		if got := pinnedLog(unit(o)); got != want[0] {
			t.Errorf("seed %d: log %v, want %v", seed, got, want[0])
		}
		if got, _ := g(RandomSeed(seed)); got != want[1] {
			t.Errorf("seed %d: normal %v, want %v", seed, got, want[1])
		}
	}
	// the upper tail is the lower one mirrored
	if acklam(1-0.010450839394496092) != -acklam(0.010450839394496092) {
		t.Error("upper tail is not the mirror of the lower")
	}
}

// pinnedLog agrees with math.Log to within an ulp or so over the tail's whole range.
func TestPinnedLog(t *testing.T) {
	for p := 1e-17; p < 0.03; p *= 1.01 {
		if got, want := pinnedLog(p), math.Log(p); math.Abs(got-want) > 2e-16*math.Abs(want) {
			t.Fatalf("pinnedLog %v = %v, math.Log %v", p, got, want)
		}
	}
}

// The seed whose first output is all ones: unit stays below 1, so normal is finite and
// floatRange never reaches max (a 53-bit unit rounded to exactly 1.0 here).
func TestRandomMaxOutput(t *testing.T) {
	s := RandomSeed(3558559446808474027)
	if o, _ := s.next(); o != math.MaxUint64 {
		t.Fatalf("output %x", o)
	}
	if u, _ := s.next(); unit(u) != 0.9999999999999999 {
		t.Errorf("unit = %v", unit(u))
	}
	if n, _ := RandomNormal(struct{ Mean, Sd float64 }{0, 1})(s); n != 8.209536145151493 {
		t.Errorf("normal = %v", n)
	}
	for _, hi := range []float64{1, 10} {
		if f, _ := RandomFloatRange(struct{ Min, Max float64 }{0, hi})(s); !(f < hi) {
			t.Errorf("floatRange 0..%v = %v", hi, f)
		}
	}
	// and the smallest output: unit stays above 0
	if unit(0) != 0x1p-53 {
		t.Errorf("unit 0 = %v", unit(0))
	}
}
