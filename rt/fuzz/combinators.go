package fuzz

import (
	"math"
	"math/rand/v2"

	"alder/rt"
	"alder/rt/std"
)

// The Go side of stdlib/Fuzz.alder, one function per declaration, in its order. As for every
// stdlib function, a constant (Fuzz.bool) is a function of no parameters, a record argument is
// the anonymous struct gen emits for it, and the Alder types are those of gen's GoType.
// Fuzz.any, Fuzz.forAll and Fuzz.forAllTask are not here: the compiler and the test runner
// own them.

// FromRandom is Fuzz.fromRandom: one choice over the full 64 bits is a Random.Seed's state,
// and the value is g's at that seed. Shrinking lowers the seed toward 0 (it cannot see inside
// g), so the simplest case is g's value at Random.seed 0.
func FromRandom[A any](g std.Generator[A]) Fuzzer[A] {
	return func(c *Choices) A {
		v, _ := g(std.Seed{State: c.Choice(math.MaxUint64)})
		return v
	}
}

// Constant is Fuzz.constant: always a, with no choice.
func Constant[A any](a A) Fuzzer[A] { return func(*Choices) A { return a } }

// BoolF is Fuzz.bool: False or True, even when fresh.
func BoolF() Fuzzer[bool] { return func(c *Choices) bool { return Bool(c, 0.5) } }

// IntF is Fuzz.int (see Int).
func IntF() Fuzzer[int64] { return Int }

// IntRangeF is Fuzz.intRange (see IntRange).
func IntRangeF(r struct{ Min, Max int64 }) Fuzzer[int64] {
	return func(c *Choices) int64 { return IntRange(c, r.Min, r.Max) }
}

// FloatF is Fuzz.float (see Float).
func FloatF() Fuzzer[float64] { return Float }

// FloatRangeF is Fuzz.floatRange (see FloatRange).
func FloatRangeF(r struct{ Min, Max float64 }) Fuzzer[float64] {
	return func(c *Choices) float64 { return FloatRange(c, r.Min, r.Max) }
}

// TextF is Fuzz.text (see Text).
func TextF() Fuzzer[string] { return Text }

// TextOf is Fuzz.textOf: between Min and Max code points.
func TextOf(r struct{ Min, Max int64 }) Fuzzer[string] {
	return func(c *Choices) string { return textOf(c, r.Min, r.Max) }
}

// Char is Fuzz.char: one code point, 20% non-ASCII when fresh.
func Char() Fuzzer[string] {
	return func(c *Choices) string { return string(char(c, true, 0.2)) }
}

// List is Fuzz.list: a continue bit before each element (stop = 0), length biased small.
func List[A any](f Fuzzer[A]) Fuzzer[[]A] {
	return listOf(0, -1, f)
}

// ListOf is Fuzz.listOf: Min elements, then a continue bit before each further one up to Max
// (a Max below Min, a negative one included, means Min).
func ListOf[A any](r struct{ Min, Max int64 }, f Fuzzer[A]) Fuzzer[[]A] {
	lo := max(r.Min, 0)
	return listOf(lo, max(r.Max, lo), f)
}

// listOf draws lo elements, then a continue bit before each further one up to hi (no bound
// when hi < 0, which only List passes).
func listOf[A any](lo, hi int64, f Fuzzer[A]) Fuzzer[[]A] {
	return func(c *Choices) []A {
		xs := []A{}
		p := lengthBias(c)
		for i := int64(0); hi < 0 || i < hi; i++ {
			if i >= lo && !more(c, p) {
				break
			}
			xs = append(xs, f(c))
		}
		return xs
	}
}

// Maybe is Fuzz.maybe: Nothing (choice 0, the simpler) or Just (75% when fresh).
func Maybe[A any](f Fuzzer[A]) Fuzzer[rt.Maybe[A]] {
	return func(c *Choices) rt.Maybe[A] {
		if Bool(c, 0.75) {
			return rt.Just(f(c))
		}
		return rt.Nothing[A]()
	}
}

// Result is Fuzz.result: Ok (choice 0, the first constructor) or Err, even when fresh.
func Result[E, A any](fe Fuzzer[E], fa Fuzzer[A]) Fuzzer[rt.Result[E, A]] {
	return func(c *Choices) rt.Result[E, A] {
		if Bool(c, 0.5) {
			return rt.Err[E, A](fe(c))
		}
		return rt.Ok[E](fa(c))
	}
}

// Pair is Fuzz.pair: first, then second.
func Pair[A, B any](fa Fuzzer[A], fb Fuzzer[B]) Fuzzer[struct {
	First  A
	Second B
}] {
	return func(c *Choices) struct {
		First  A
		Second B
	} {
		a := fa(c)
		return struct {
			First  A
			Second B
		}{a, fb(c)}
	}
}

// Dict is Fuzz.dict: entries drawn as a list of key, value (a repeated key keeps its first
// position and its last value).
func Dict[K, V any](fk Fuzzer[K], fv Fuzzer[V]) Fuzzer[rt.Dict[K, V]] {
	return DictOf(-1, fk, fv)
}

// DictOf is Dict with at most max entries drawn (no bound when max < 0): a derived generator's
// Dict that holds its own type keeps within its depth budget with it.
func DictOf[K, V any](max int64, fk Fuzzer[K], fv Fuzzer[V]) Fuzzer[rt.Dict[K, V]] {
	return func(c *Choices) rt.Dict[K, V] {
		var d rt.Dict[K, V]
		p := lengthBias(c)
		for i := int64(0); (max < 0 || i < max) && more(c, p); i++ {
			k := fk(c)
			d.Put(k, fv(c))
		}
		return d
	}
}

// OneOf is Fuzz.oneOf: an alternative, the first the simplest, uniform when fresh.
func OneOf[A any](fs []Fuzzer[A]) Fuzzer[A] {
	return func(c *Choices) A {
		if len(fs) == 0 {
			panic("Fuzz.oneOf: the list of fuzzers is empty")
		}
		return fs[c.Choice(uint64(len(fs)-1))](c)
	}
}

// OneOfValues is Fuzz.oneOfValues: a value, the first the simplest, uniform when fresh.
func OneOfValues[A any](vs []A) Fuzzer[A] {
	return func(c *Choices) A {
		if len(vs) == 0 {
			panic("Fuzz.oneOfValues: the list of values is empty")
		}
		return vs[c.Choice(uint64(len(vs)-1))]
	}
}

// Frequency is Fuzz.frequency: an alternative picked by weight when fresh. The choice indexes
// the alternatives of positive weight, so the simplest is the first of those and a shrunk
// choice never picks a weight of 0.
func Frequency[A any](ws []struct {
	Weight int64
	Fuzzer Fuzzer[A]
}) Fuzzer[A] {
	var live []Fuzzer[A]
	var weights []uint64
	var total uint64
	for _, w := range ws {
		if w.Weight > 0 {
			live = append(live, w.Fuzzer)
			wt := uint64(min(w.Weight, 1<<40)) // so the total cannot overflow
			weights = append(weights, wt)
			total += wt
		}
	}
	return func(c *Choices) A {
		if len(live) == 0 {
			panic("Fuzz.frequency: no fuzzer has a positive weight")
		}
		i := c.draw(uint64(len(live)-1), func(r *rand.Rand) uint64 {
			x := uniform(r, total-1)
			for i, w := range weights {
				if x < w {
					return uint64(i)
				}
				x -= w
			}
			return uint64(len(weights) - 1)
		})
		return live[i](c)
	}
}

// Map is Fuzz.map.
func Map[A, B any](f func(A) B, fa Fuzzer[A]) Fuzzer[B] {
	return func(c *Choices) B { return f(fa(c)) }
}

// Map2 is Fuzz.map2: a, then b.
func Map2[A, B, C any](f func(A, B) C, fa Fuzzer[A], fb Fuzzer[B]) Fuzzer[C] {
	return func(c *Choices) C {
		a := fa(c)
		return f(a, fb(c))
	}
}

// AndThen is Fuzz.andThen: draw a, then from the fuzzer k picks for it.
func AndThen[A, B any](k func(A) Fuzzer[B], fa Fuzzer[A]) Fuzzer[B] {
	return func(c *Choices) B { return k(fa(c))(c) }
}

// FilterF is Fuzz.filter: Filter with the rejections named "Fuzz.filter" in the health check.
func FilterF[A any](p func(A) bool, f Fuzzer[A]) Fuzzer[A] { return Filter("Fuzz.filter", p, f) }

// Filter is a value of f that p accepts, or panic Reject{what} (no retry: the runner counts
// rejections for the health check). what names the predicate in that check's message:
// "Fuzz.filter" for Fuzz.filter (FilterF), the brand's name for a derived brand's check.
func Filter[A any](what string, p func(A) bool, f Fuzzer[A]) Fuzzer[A] {
	return func(c *Choices) A {
		a := f(c)
		if !p(a) {
			panic(Reject{What: what})
		}
		return a
	}
}
