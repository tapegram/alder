package std

import (
	"math"
	"strconv"
)

// The pure half of stdlib/Random.alder (the Task-returning Random.int and Random.id are in
// effects.go). A Seed is one 64-bit SplitMix64 state; a Generator steps it. Every algorithm
// here is pinned by name and test vectors (random_test.go), so a seeded run reproduces across
// versions and platforms: changing any of them changes users' data.
//
// Steps: every primitive generator (intRange, floatRange, normal, uniform, weighted) takes
// exactly one step, degenerate inputs included; constant takes none; list n takes n steps of
// its generator; map adds none; map2 runs its first generator then its second; andThen runs
// its generator, then the one its function returns.

// Seed is Alder's `Random.Seed`: the SplitMix64 state.
type Seed struct{ State uint64 }

// AlderShow prints a seed as the expression that re-creates it, `Random.seed N` (rt.Shower).
func (s Seed) AlderShow() (string, bool) {
	return "Random.seed " + strconv.FormatInt(int64(s.State), 10), true
}

// next is one SplitMix64 step (Steele, Lea, Flood 2014; Vigna's splitmix64.c): the output and
// the seed after it.
func (s Seed) next() (uint64, Seed) {
	st := s.State + 0x9E3779B97F4A7C15
	z := st
	z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
	z = (z ^ (z >> 27)) * 0x94D049BB133111EB
	return z ^ (z >> 31), Seed{st}
}

// unit is an output as a Float strictly in (0, 1): its top 53 bits, centred in their interval.
func unit(o uint64) float64 { return (float64(o>>11) + 0.5) * 0x1p-53 }

// Generator is Alder's `Random.Generator a`: a value drawn from a seed, and the seed after.
type Generator[A any] func(Seed) (A, Seed)

// RandomSeed is Random.seed: the state n as an unsigned 64-bit value (two's complement), so
// every Int is a seed.
func RandomSeed(n int64) Seed { return Seed{uint64(n)} }

// RandomStep is Random.step: the generator's value and the seed after its steps.
func RandomStep[A any](g Generator[A], s Seed) struct {
	Value A
	Seed  Seed
} {
	v, next := g(s)
	return struct {
		Value A
		Seed  Seed
	}{v, next}
}

// RandomConstant is Random.constant: a, with no step.
func RandomConstant[A any](a A) Generator[A] { return func(s Seed) (A, Seed) { return a, s } }

// RandomIntRange is Random.intRange: min when max <= min; otherwise min + output mod (span+1),
// or the output itself over the full Int range. One step either way.
func RandomIntRange(r struct{ Min, Max int64 }) Generator[int64] {
	return func(s Seed) (int64, Seed) {
		o, next := s.next()
		if r.Max <= r.Min {
			return r.Min, next
		}
		span := uint64(r.Max) - uint64(r.Min)
		if span == math.MaxUint64 {
			return int64(o), next
		}
		return int64(uint64(r.Min) + o%(span+1)), next
	}
}

// RandomFloatRange is Random.floatRange: min + (max - min) * unit; min when max <= min or
// either is NaN. One step.
func RandomFloatRange(r struct{ Min, Max float64 }) Generator[float64] {
	return func(s Seed) (float64, Seed) {
		o, next := s.next()
		if !(r.Max > r.Min) {
			return r.Min, next
		}
		return r.Min + float64((r.Max-r.Min)*unit(o)), next // float64: no fused multiply-add (horner)
	}
}

// RandomNormal is Random.normal: mean + sd * q(unit), q Acklam's inverse normal CDF. One step.
func RandomNormal(r struct{ Mean, Sd float64 }) Generator[float64] {
	return func(s Seed) (float64, Seed) {
		o, next := s.next()
		return r.Mean + float64(r.Sd*acklam(unit(o))), next // float64: no fused multiply-add (horner)
	}
}

// acklam is Peter Acklam's rational approximation (2003) of the inverse standard normal CDF at
// p in (0, 1), relative error below 1.15e-9, without the refinement step: the central formula
// for 0.02425 <= p <= 0.97575, the tail formula outside.
func acklam(p float64) float64 {
	const low = 0.02425
	switch {
	case p < low:
		return acklamTail(p)
	case p > 1-low:
		return -acklamTail(1 - p)
	}
	q := p - 0.5
	r := float64(q * q)
	num := float64(horner(r, -39.69683028665376, 220.9460984245205, -275.9285104469687, 138.3577518672690, -30.66479806614716, 2.506628277459239) * q)
	den := horner(r, -54.47609879822406, 161.5858368580409, -155.6989798598866, 66.80131188771972, -13.28068155288572, 1)
	return num / den
}

// acklamTail is the lower tail of acklam (p < 0.02425).
func acklamTail(p float64) float64 {
	q := math.Sqrt(float64(-2 * math.Log(p)))
	num := horner(q, -0.007784894002430293, -0.3223964580411365, -2.400758277161838, -2.549732539343734, 4.374664141464968, 2.938163982698783)
	den := horner(q, 0.007784695709041462, 0.3224671290700398, 2.445134137142996, 3.754408661907416, 1)
	return num / den
}

// horner evaluates the polynomial with coefficients cs (highest power first) at x. Every
// product is rounded before its sum (the explicit float64 conversion forbids a fused
// multiply-add, which Go may otherwise use on arm64 and not on amd64), so the result is the
// same on every platform.
func horner(x float64, cs ...float64) float64 {
	acc := cs[0]
	for _, c := range cs[1:] {
		acc = float64(acc*x) + c
	}
	return acc
}

// RandomUniform is Random.uniform first rest: the item at intRange 0..len(rest) of
// first :: rest. One step.
func RandomUniform[A any](first A, rest []A) Generator[A] {
	idx := RandomIntRange(struct{ Min, Max int64 }{0, int64(len(rest))})
	return func(s Seed) (A, Seed) {
		i, next := idx(s)
		if i == 0 {
			return first, next
		}
		return rest[i-1], next
	}
}

// RandomWeighted is Random.weighted (Elm's): an item with probability proportional to the
// absolute value of its weight. One step.
func RandomWeighted[A any](first struct {
	Weight float64
	Value  A
}, rest []struct {
	Weight float64
	Value  A
}) Generator[A] {
	return func(s Seed) (A, Seed) {
		o, next := s.next()
		return pickWeighted(unit(o), first, rest), next
	}
}

// pickWeighted walks first :: rest with t = u * total: the first item with t <= |weight|, else
// t minus |weight| and on; the last item when none is (rounding). All weights zero gives first.
func pickWeighted[A any](u float64, first struct {
	Weight float64
	Value  A
}, rest []struct {
	Weight float64
	Value  A
}) A {
	total := math.Abs(first.Weight)
	for _, w := range rest {
		total += math.Abs(w.Weight)
	}
	if !(total > 0) || len(rest) == 0 { // all zero (or a NaN weight), or one item
		return first.Value
	}
	t := u * total
	if t <= math.Abs(first.Weight) {
		return first.Value
	}
	t -= math.Abs(first.Weight)
	for _, w := range rest {
		if t <= math.Abs(w.Weight) {
			return w.Value
		}
		t -= math.Abs(w.Weight)
	}
	return rest[len(rest)-1].Value
}

// RandomList is Random.list: n values of g in order; [] and no step when n <= 0.
func RandomList[A any](n int64, g Generator[A]) Generator[[]A] {
	return func(s Seed) ([]A, Seed) {
		xs := make([]A, 0, max(n, 0))
		for i := int64(0); i < n; i++ {
			var x A
			x, s = g(s)
			xs = append(xs, x)
		}
		return xs, s
	}
}

// RandomMap is Random.map: no step of its own.
func RandomMap[A, B any](f func(A) B, g Generator[A]) Generator[B] {
	return func(s Seed) (B, Seed) {
		a, s := g(s)
		return f(a), s
	}
}

// RandomMap2 is Random.map2: ga, then gb from the seed ga leaves.
func RandomMap2[A, B, C any](f func(A, B) C, ga Generator[A], gb Generator[B]) Generator[C] {
	return func(s Seed) (C, Seed) {
		a, s := ga(s)
		b, s := gb(s)
		return f(a, b), s
	}
}

// RandomAndThen is Random.andThen: g, then the generator f makes of its value.
func RandomAndThen[A, B any](f func(A) Generator[B], g Generator[A]) Generator[B] {
	return func(s Seed) (B, Seed) {
		a, s := g(s)
		return f(a)(s)
	}
}
