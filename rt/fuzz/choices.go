// Package fuzz is the engine behind Alder's property tests (stdlib/Fuzz.alder): a generator
// (a Fuzzer) draws every random decision it makes from a Choices, which records the sequence
// of choices so that a failing case can be replayed and shrunk (minithesis-style: the shrinker
// minimises the recorded sequence, never the values, so user code has no shrinkers).
//
// Choices are ordered so that smaller is simpler: the all-zero sequence draws 0, "", [],
// False, Nothing and every first alternative, and lowering a choice moves its value toward
// that. Fresh draws are biased (small values most of the time, extremes some of the time);
// the bias lives only in how a fresh choice is picked, so a replay needs no randomness.
package fuzz

import (
	"encoding/binary"
	"errors"
	"math/rand/v2"
)

// MaxChoices bounds one attempt: 8192 choices of 8 bytes is the 64 KiB "generator draws too
// much" health check (spec §5).
const MaxChoices = 8192

// Overrun is the panic value of a draw past the end of a replayed sequence, or past
// MaxChoices. The attempt counts as neither pass nor fail; the runner tells the two causes
// apart by mode (an Overrun during a fresh draw means the generator draws too much).
type Overrun struct{}

// Reject is the panic value of a Fuzz.filter or brand predicate that said no. What names the
// predicate for the health check's message.
type Reject struct{ What string }

// Fuzzer is Alder's `Fuzzer a`: a function that draws an a from choices.
type Fuzzer[A any] func(*Choices) A

// Choices is the source of every random decision of one attempt: fresh from a PRNG, or a
// replay of a recorded sequence. Either way each choice made is recorded.
type Choices struct {
	rng      *rand.Rand // nil when replaying
	replay   []uint64
	pos      int
	recorded []uint64
}

// Fresh draws new choices from a PCG seeded with seed.
func Fresh(seed uint64) *Choices {
	return &Choices{rng: rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))}
}

// Replay replays seq: the i-th choice of range [0, n] is min(seq[i], n). Drawing past the end
// panics Overrun{}.
func Replay(seq []uint64) *Choices {
	return &Choices{replay: seq}
}

// Recorded is the sequence of choices made so far (as clamped on replay).
func (c *Choices) Recorded() []uint64 { return c.recorded }

// Choice is a value in [0, n], uniform when fresh.
func (c *Choices) Choice(n uint64) uint64 {
	return c.draw(n, func(r *rand.Rand) uint64 { return uniform(r, n) })
}

// draw makes one choice in [0, n]: fresh, pick picks it (clamped to n); on replay it is the
// next replayed value, clamped.
func (c *Choices) draw(n uint64, pick func(*rand.Rand) uint64) uint64 {
	if len(c.recorded) >= MaxChoices {
		panic(Overrun{})
	}
	var v uint64
	if c.rng != nil {
		v = pick(c.rng)
	} else {
		if c.pos >= len(c.replay) {
			panic(Overrun{})
		}
		v = c.replay[c.pos]
		c.pos++
	}
	v = min(v, n)
	c.recorded = append(c.recorded, v)
	return v
}

// chance is a fresh-only coin with probability p; it consumes randomness but makes no
// choice, so it only shapes the bias of a fresh draw.
func (c *Choices) chance(p float64) bool {
	return c.rng != nil && c.rng.Float64() < p
}

// uniform is uniform in [0, n].
func uniform(r *rand.Rand, n uint64) uint64 {
	if n == ^uint64(0) {
		return r.Uint64()
	}
	return r.Uint64N(n + 1)
}

// Encode writes seq as a stream of uvarints, the saved-failure format.
func Encode(seq []uint64) []byte {
	var b []byte
	for _, v := range seq {
		b = binary.AppendUvarint(b, v)
	}
	return b
}

// Decode reads a stream written by Encode.
func Decode(b []byte) ([]uint64, error) {
	seq := []uint64{}
	for len(b) > 0 {
		v, n := binary.Uvarint(b)
		if n <= 0 {
			return nil, errors.New("fuzz: malformed choice sequence")
		}
		seq = append(seq, v)
		b = b[n:]
	}
	return seq, nil
}
