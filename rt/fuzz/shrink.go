package fuzz

import "slices"

// Shrink minimises a failing choice sequence (minithesis-style). fails replays a candidate and
// reports whether the property still fails on it; an Overrun or Reject panic inside it counts
// as not failing (any other panic propagates). A candidate is kept only if it is shortlex-
// smaller than the best so far and still fails.
//
// The passes repeat until a full round changes nothing or budget calls of fails are spent:
// delete blocks of 8, 4, 3, 2 and 1 choices (3 is one Int element of a list: continue bit,
// magnitude, sign); zero blocks of 8, 4, 2 and 1; lower each choice by binary search toward
// 0; sort adjacent pairs ascending; and move a choice's whole value onto one of the next four
// (so a + b >= 100 ends at 0 and 100, not 40 and 60). steps is the number of calls of fails.
func Shrink(seq []uint64, fails func([]uint64) bool, budget int) (best []uint64, steps int) {
	s := &shrinker{best: slices.Clone(seq), fails: fails, budget: budget, seen: map[string]bool{}}
	for s.steps < s.budget {
		before := s.best
		s.deleteBlocks()
		s.zeroBlocks()
		s.lower()
		s.sortPairs()
		s.redistribute()
		if slices.Equal(before, s.best) {
			break
		}
	}
	return s.best, s.steps
}

type shrinker struct {
	best   []uint64
	fails  func([]uint64) bool
	budget int
	steps  int
	seen   map[string]bool // candidates already run
	// invalid reports whether the last candidate run panicked Overrun or Reject
	invalid bool
}

func (s *shrinker) spent() bool { return s.steps >= s.budget }

// consider runs cand if it is new, shortlex-smaller than best and the budget allows, and keeps
// it if it still fails.
func (s *shrinker) consider(cand []uint64) bool {
	s.invalid = false
	if s.spent() || !shortlexLess(cand, s.best) {
		return false
	}
	key := string(Encode(cand))
	if s.seen[key] {
		return false
	}
	s.seen[key] = true
	s.steps++
	if s.run(cand) {
		s.best = cand
		return true
	}
	return false
}

func (s *shrinker) run(cand []uint64) (failed bool) {
	defer func() {
		if v := recover(); v != nil {
			switch v.(type) {
			case Overrun, Reject:
				failed, s.invalid = false, true
			default:
				panic(v)
			}
		}
	}()
	return s.fails(cand)
}

// with is best with best[i] = v.
func (s *shrinker) with(i int, v uint64) []uint64 {
	c := slices.Clone(s.best)
	c[i] = v
	return c
}

func (s *shrinker) deleteBlocks() {
	for _, k := range []int{8, 4, 3, 2, 1} {
		for i := len(s.best) - k; i >= 0 && !s.spent(); i-- {
			cand := slices.Concat(s.best[:i], s.best[i+k:])
			if s.consider(cand) {
				i = min(i+1, len(s.best)-k+1) // try the same place again
				continue
			}
			// a deleted element often leaves a count before it one too high
			if i > 0 && cand[i-1] > 0 {
				cand = slices.Clone(cand)
				cand[i-1]--
				if s.consider(cand) {
					i = min(i+1, len(s.best)-k+1)
				}
			}
		}
	}
}

func (s *shrinker) zeroBlocks() {
	for _, k := range []int{8, 4, 2, 1} {
		for i := len(s.best) - k; i >= 0 && !s.spent(); i-- {
			if !slices.ContainsFunc(s.best[i:i+k], func(v uint64) bool { return v != 0 }) {
				continue
			}
			cand := slices.Clone(s.best)
			clear(cand[i : i+k])
			s.consider(cand)
		}
	}
}

// lower finds, for each choice, a smaller value that still fails: 0, else by binary search
// between a value that does not fail (lo) and one that does (hi). A midpoint the generator
// rejects (or that overruns) says nothing about the property, so its neighbour mid+1 is tried
// too: a filter keeping odd numbers would otherwise stop the search at any odd number.
func (s *shrinker) lower() {
	for i := 0; i < len(s.best) && !s.spent(); i++ {
		if s.best[i] == 0 || s.consider(s.with(i, 0)) {
			continue
		}
		lo, hi := uint64(0), s.best[i]
		for lo+1 < hi && !s.spent() {
			mid := lo + (hi-lo)/2
			switch {
			case s.consider(s.with(i, mid)):
				hi = mid
			case s.invalid && mid+1 < hi && s.consider(s.with(i, mid+1)):
				hi = mid + 1
			default:
				lo = mid
			}
		}
	}
}

func (s *shrinker) sortPairs() {
	for i := 0; i+1 < len(s.best) && !s.spent(); i++ {
		if s.best[i] > s.best[i+1] {
			cand := slices.Clone(s.best)
			cand[i], cand[i+1] = cand[i+1], cand[i]
			s.consider(cand)
		}
	}
}

func (s *shrinker) redistribute() {
	for i := 0; i < len(s.best) && !s.spent(); i++ {
		for j := i + 1; j < len(s.best) && j <= i+4 && s.best[i] != 0 && !s.spent(); j++ {
			v := s.best[i]
			if s.best[j] > ^uint64(0)-v {
				continue
			}
			cand := s.with(i, 0)
			cand[j] += v
			s.consider(cand)
		}
	}
}

// shortlexLess orders shorter sequences first, then lexicographically.
func shortlexLess(a, b []uint64) bool {
	if len(a) != len(b) {
		return len(a) < len(b)
	}
	return slices.Compare(a, b) < 0
}
