package fuzz

import (
	"math"
	"math/rand/v2"
	"time"

	"alder/rt"
)

// The draws behind the generators the compiler derives from a type (spec §4): gen emits one
// fuzz__ function per type, built from these and the primitives.

// Depth is the depth budget of a derived generator at the root: each level of a recursive
// type spends one, and at 0 only non-recursive constructors (and empty recursive lists) are
// drawn.
const Depth = 4

// Ctor draws the constructor of a union whose constructors are recursive where rec says
// (their payload holds the union again), and returns its index. The choice puts the
// non-recursive constructors first, in declaration order, then the recursive ones, so the
// simplest value is the first non-recursive constructor. At depth 0 only the non-recursive
// ones can be drawn (all of them when every constructor is recursive). Fresh, a recursive
// constructor is drawn with chance 1/2 at the root (depth Depth), halving at each level below.
func Ctor(c *Choices, rec []bool, depth int) int {
	var plain, recs []int
	for i, r := range rec {
		if r {
			recs = append(recs, i)
		} else {
			plain = append(plain, i)
		}
	}
	if len(plain) == 0 {
		plain, recs = recs, nil
	}
	if depth <= 0 {
		recs = nil
	}
	order := append(plain, recs...)
	if len(order) == 0 {
		panic("fuzz: a union with no constructors")
	}
	pRec := 0.0
	if len(recs) > 0 {
		pRec = math.Ldexp(0.5, -(Depth - min(depth, Depth)))
	}
	k := c.draw(uint64(len(order)-1), func(r *rand.Rand) uint64 {
		if r.Float64() < pRec {
			return uint64(len(plain)) + r.Uint64N(uint64(len(recs)))
		}
		return r.Uint64N(uint64(len(plain)))
	})
	return order[k]
}

const (
	clockDays = 20454   // the test clock's start (rtesting's Ticking), 2026-01-01, in days after the epoch
	dateSpan  = 3 * 366 // days either side of an anchor
)

// anchorDays draws the day a Date or Time is near: the epoch (0, the simpler) or the test
// clock's start, then a day offset within about three years, nearest 0 simplest.
func anchorDays(c *Choices) int64 {
	base := int64(0)
	if Bool(c, 0.5) {
		base = clockDays
	}
	return base + IntRange(c, -dateSpan, dateSpan)
}

// Date draws a Date within about three years of the epoch or of 2026-01-01; the epoch is the
// simplest.
func Date(c *Choices) rt.Date {
	return rt.DateOf(time.Unix(0, 0).UTC().AddDate(0, 0, int(anchorDays(c))))
}

// Time draws a Time in whole milliseconds within about three years of the epoch or of
// 2026-01-01: a day as Date does, then the millisecond of the day (midnight simplest). The
// epoch is the simplest.
func Time(c *Choices) time.Time {
	ms := anchorDays(c)*86_400_000 + IntRange(c, 0, 86_399_999)
	return time.UnixMilli(ms).UTC()
}

// Bytes draws a short byte string, biased small: the UTF-8 of a Text, so every value prints
// as `Text.toBytes "…"`.
func Bytes(c *Choices) []byte { return []byte(Text(c)) }
