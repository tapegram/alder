package fuzz

import (
	"math"
	"math/rand/v2"
	"slices"
	"strings"
	"sync"
	"unicode"
)

// coin picks 1 with probability p: the fresh bias of a two-way choice.
func coin(p float64) func(*rand.Rand) uint64 {
	return func(r *rand.Rand) uint64 {
		if r.Float64() < p {
			return 1
		}
		return 0
	}
}

// Bool is True (choice 1) with probability p when fresh; False is the simpler.
func Bool(c *Choices, p float64) bool {
	return c.draw(1, coin(p)) == 1
}

// boundaries are magnitudes that find overflow and off-by-one bugs.
var boundaries = []uint64{
	1 << 63, 1<<63 - 1, 1 << 62, 1<<53 + 1, 1 << 53, 1 << 32, 1<<32 - 1, 1 << 31, 1<<31 - 1,
	1 << 16, 1<<16 - 1, 1 << 15, 256, 255, 128, 127,
}

// Int draws a magnitude in [0, 2^63] then a sign (0 = positive), so the simplest Int is 0 and
// shrinking moves toward it. Fresh: 55% in 0..10, 20% in 0..1000, 12% below 2^32, 7% anywhere,
// 6% a boundary (2^63 is MinInt64 when negative, MaxInt64 when positive).
func Int(c *Choices) int64 {
	m := c.draw(1<<63, func(r *rand.Rand) uint64 {
		switch x := r.Float64(); {
		case x < 0.55:
			return r.Uint64N(11)
		case x < 0.75:
			return r.Uint64N(1001)
		case x < 0.87:
			return r.Uint64N(1 << 32)
		case x < 0.94:
			return uniform(r, 1<<63)
		default:
			return boundaries[r.IntN(len(boundaries))]
		}
	})
	if Bool(c, 0.5) {
		return -int64(m) // 2^63 wraps to MinInt64, whose negation is itself
	}
	if m == 1<<63 {
		return math.MaxInt64
	}
	return int64(m)
}

// IntRange draws an Int in [min, max] (just min when max <= min, with no choice). The
// simplest value is the one nearest 0; when the range holds 0 the choices run 0, 1, -1, 2,
// -2, … and then the longer side. Fresh: 10% min, 10% max, 40% among the 17 simplest, 40%
// uniform.
func IntRange(c *Choices, min, max int64) int64 {
	if max <= min {
		return min
	}
	val, idx := zigzag(min, max)
	span := uint64(max) - uint64(min)
	k := c.draw(span, func(r *rand.Rand) uint64 {
		switch x := r.Float64(); {
		case x < 0.1:
			return idx(min)
		case x < 0.2:
			return idx(max)
		case x < 0.6:
			return r.Uint64N(minU(span, 16) + 1)
		default:
			return uniform(r, span)
		}
	})
	return val(k)
}

func minU(a, b uint64) uint64 {
	if a < b {
		return a
	}
	return b
}

// zigzag orders [lo, hi] simplest first: val is the k-th value, idx its inverse.
func zigzag(lo, hi int64) (val func(uint64) int64, idx func(int64) uint64) {
	switch {
	case lo >= 0:
		return func(k uint64) int64 { return int64(uint64(lo) + k) },
			func(v int64) uint64 { return uint64(v) - uint64(lo) }
	case hi <= 0:
		return func(k uint64) int64 { return int64(uint64(hi) - k) },
			func(v int64) uint64 { return uint64(hi) - uint64(v) }
	}
	a, b := uint64(hi), -uint64(lo) // the counts of positive and negative values
	m := minU(a, b)
	val = func(k uint64) int64 {
		switch {
		case k == 0:
			return 0
		case k <= 2*m && k%2 == 1:
			return int64((k + 1) / 2)
		case k <= 2*m:
			return -int64(k / 2)
		case a > b:
			return int64(k - m)
		default:
			return -int64(k - m)
		}
	}
	idx = func(v int64) uint64 {
		switch {
		case v == 0:
			return 0
		case v > 0 && uint64(v) <= m:
			return 2*uint64(v) - 1
		case v > 0:
			return uint64(v) + m
		case -uint64(v) <= m:
			return 2 * -uint64(v)
		default:
			return -uint64(v) + m
		}
	}
	return val, idx
}

var negZero = math.Copysign(0, -1)

// specialFloats are the values that break float code.
var specialFloats = []float64{
	0, negZero, 1, -1, 0.5, 0.1, -0.1, 1e300, -1e300,
	math.MaxFloat64, -math.MaxFloat64, math.SmallestNonzeroFloat64, -math.SmallestNonzeroFloat64,
	2.2250738585072014e-308, 1 << 53, 1<<53 + 1,
}

// Float draws a finite Float, never NaN or Inf. A kind comes first, simplest first:
//
//	0 integral: magnitude in [0, 2^53] then sign (45% fresh; 0 and -0 are here)
//	1 decimal: an integer over 10^d, d in 1..6 fresh (30%)
//	2 any bits (10%): tiny, subnormal and huge; a NaN or Inf becomes ±MaxFloat64
//	3 a special value (15%): 0, -0, ±MaxFloat64, ±the smallest subnormal, 1e300, 2^53 …
func Float(c *Choices) float64 {
	kind := c.draw(3, func(r *rand.Rand) uint64 {
		switch x := r.Float64(); {
		case x < 0.45:
			return 0
		case x < 0.75:
			return 1
		case x < 0.85:
			return 2
		default:
			return 3
		}
	})
	switch kind {
	case 0:
		m := c.draw(1<<53, func(r *rand.Rand) uint64 {
			switch x := r.Float64(); {
			case x < 0.6:
				return r.Uint64N(11)
			case x < 0.9:
				return r.Uint64N(1001)
			default:
				return uniform(r, 1<<53)
			}
		})
		return signed(c, float64(m))
	case 1:
		m := c.draw(1<<53, func(r *rand.Rand) uint64 {
			if r.Float64() < 0.5 {
				return r.Uint64N(101)
			}
			return r.Uint64N(1_000_001)
		})
		d := c.draw(17, func(r *rand.Rand) uint64 { return 1 + r.Uint64N(6) })
		return signed(c, float64(m)/math.Pow10(int(d)))
	case 2:
		f := math.Float64frombits(c.Choice(math.MaxUint64))
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return math.Copysign(math.MaxFloat64, f)
		}
		return f
	default:
		return specialFloats[c.Choice(uint64(len(specialFloats)-1))]
	}
}

// signed draws a sign (0 = positive) for f.
func signed(c *Choices, f float64) float64 {
	if Bool(c, 0.5) {
		return -f
	}
	return f
}

// FloatRange draws a Float in [min, max] (just min when max <= min or either is NaN; an
// infinite bound is ±MaxFloat64). The simplest value is the one nearest 0 (the anchor); a
// side bit then a fraction t in [0, 1] step from the anchor toward max (side 0) or min.
// Fresh: 15% the anchor, 15% the far end, 70% uniform t.
func FloatRange(c *Choices, min, max float64) float64 {
	if math.IsNaN(min) || math.IsNaN(max) || max <= min {
		return min
	}
	min, max = math.Max(min, -math.MaxFloat64), math.Min(max, math.MaxFloat64)
	anchor := math.Min(math.Max(0, min), max)
	side := Bool(c, 0.5)
	target := max
	if (side && min < anchor) || max == anchor {
		target = min
	}
	u := c.draw(1<<53, func(r *rand.Rand) uint64 {
		switch x := r.Float64(); {
		case x < 0.15:
			return 0
		case x < 0.3:
			return 1 << 53
		default:
			return r.Uint64N(1<<53 + 1)
		}
	})
	v := anchor + float64(u)/(1<<53)*(target-anchor)
	return math.Min(math.Max(v, min), max)
}

// asciiOrder is the printable ASCII, simplest first, then the three whitespace controls that
// have Alder escapes.
const asciiOrder = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789 " +
	"!\"#$%&'()*+,-./:;<=>?@[\\]^_`{|}~" + "\n\t\r"

const (
	asciiPrintable = 95 // asciiOrder[:95] are printable
	asciiCount     = uint64(len(asciiOrder))
)

// run is the code points lo..hi; start is the number of code points in the runs before.
type run struct {
	lo, hi rune
	start  uint64
}

// unicodeRuns is every printable code point (unicode.IsPrint) from U+00A0 to U+1FFFF, as runs,
// and their total count.
var unicodeRuns = sync.OnceValues(func() ([]run, uint64) {
	var runs []run
	var total uint64
	for r := rune(0xA0); r <= 0x1FFFF; r++ {
		if !unicode.IsPrint(r) {
			continue
		}
		if n := len(runs); n > 0 && runs[n-1].hi == r-1 {
			runs[n-1].hi = r
		} else {
			runs = append(runs, run{r, r, total})
		}
		total++
	}
	return runs, total
})

// codePoint is the i-th code point in simplest-first order: asciiOrder, then the printable
// non-ASCII code points ascending.
func codePoint(i uint64) rune {
	if i < asciiCount {
		return rune(asciiOrder[i])
	}
	runs, _ := unicodeRuns()
	i -= asciiCount
	j, found := slices.BinarySearchFunc(runs, i, func(r run, i uint64) int {
		switch {
		case r.start > i:
			return 1
		case r.start+uint64(r.hi-r.lo) < i:
			return -1
		}
		return 0
	})
	if !found {
		j = len(runs) - 1
	}
	return runs[j].lo + rune(i-runs[j].start)
}

// char draws one code point; with uni false only from asciiOrder. Fresh: pUni non-ASCII,
// 3% of the rest whitespace controls.
func char(c *Choices, uni bool, pUni float64) rune {
	n := asciiCount - 1
	_, total := unicodeRuns()
	if uni {
		n += total
	}
	return codePoint(c.draw(n, func(r *rand.Rand) uint64 {
		switch x := r.Float64(); {
		case uni && x < pUni:
			return asciiCount + r.Uint64N(total)
		case r.Float64() < 0.03:
			return asciiPrintable + r.Uint64N(asciiCount-asciiPrintable)
		default:
			return r.Uint64N(asciiPrintable)
		}
	}))
}

// textMode draws whether a Text may hold non-ASCII (1; 25% fresh).
func textMode(c *Choices) bool { return Bool(c, 0.25) }

// Text draws printable code points plus \n \t \r, so every value has an Alder literal. A mode
// bit (non-ASCII allowed) comes first, then a continue bit per code point. Mostly short ASCII,
// sometimes Unicode (30% of code points in a Unicode-mode text), sometimes empty.
func Text(c *Choices) string {
	uni := textMode(c)
	var b strings.Builder
	p := lengthBias(c)
	for more(c, p) {
		b.WriteRune(char(c, uni, 0.3))
	}
	return b.String()
}

// textOf is Text with between lo and hi code points.
func textOf(c *Choices, lo, hi int64) string {
	lo = max(lo, 0)
	hi = max(hi, lo)
	uni := textMode(c)
	var b strings.Builder
	p := lengthBias(c)
	for i := int64(0); i < hi; i++ {
		if i >= lo && !more(c, p) {
			break
		}
		b.WriteRune(char(c, uni, 0.3))
	}
	return b.String()
}

// lengthBias is the fresh chance of one more element of a list or text, picked per list:
// 10% empty, 75% about 3 more, 15% about 19 more. It makes no choice.
func lengthBias(c *Choices) float64 {
	switch {
	case c.rng == nil:
		return 0
	case c.chance(0.1):
		return 0
	case c.chance(0.83):
		return 0.75
	default:
		return 0.95
	}
}

// more draws a continue bit: 1 = one more element, 0 = stop (the simpler).
func more(c *Choices, p float64) bool { return Bool(c, p) }
