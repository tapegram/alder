package fuzz

import (
	"reflect"
	"slices"
	"testing"
)

// catch runs f and returns what it panicked with, or nil.
func catch(f func()) (v any) {
	defer func() { v = recover() }()
	f()
	return nil
}

func TestReplayDeterministic(t *testing.T) {
	gen := List(IntF())
	a := gen(Fresh(42))
	c := Fresh(42)
	b := gen(c)
	if !slices.Equal(a, b) {
		t.Fatalf("Fresh(42) twice: %v vs %v", a, b)
	}
	if r := gen(Replay(c.Recorded())); !slices.Equal(r, b) {
		t.Fatalf("Replay(Recorded()) = %v, want %v", r, b)
	}
	// another seed eventually draws something else
	diff := false
	for s := uint64(1); s < 20 && !diff; s++ {
		diff = !slices.Equal(gen(Fresh(s)), a)
	}
	if !diff {
		t.Fatal("every seed drew the same list")
	}
}

func TestReplayClamps(t *testing.T) {
	c := Replay([]uint64{7, 2})
	if got := c.Choice(3); got != 3 {
		t.Fatalf("Choice(3) on 7 = %d, want 3", got)
	}
	if got := c.Choice(5); got != 2 {
		t.Fatalf("Choice(5) on 2 = %d, want 2", got)
	}
	if !slices.Equal(c.Recorded(), []uint64{3, 2}) {
		t.Fatalf("Recorded = %v, want the clamped [3 2]", c.Recorded())
	}
}

func TestReplayOverrun(t *testing.T) {
	gen := ListOf(struct{ Min, Max int64 }{3, 3}, IntF())
	v := catch(func() { gen(Replay([]uint64{1})) })
	if v != (Overrun{}) {
		t.Fatalf("panic = %#v, want Overrun{}", v)
	}
}

func TestMaxChoices(t *testing.T) {
	gen := func(c *Choices) int {
		for range 9000 {
			c.Choice(1)
		}
		return 0
	}
	if v := catch(func() { gen(Fresh(1)) }); v != (Overrun{}) {
		t.Fatalf("fresh: panic = %#v, want Overrun{}", v)
	}
	if v := catch(func() { gen(Replay(make([]uint64, 9000))) }); v != (Overrun{}) {
		t.Fatalf("replay: panic = %#v, want Overrun{}", v)
	}
}

func TestEncodeRoundTrip(t *testing.T) {
	for _, seq := range [][]uint64{{}, {0}, {1, 2, 3}, {127, 128, 300, 1 << 40, ^uint64(0)}} {
		got, err := Decode(Encode(seq))
		if err != nil {
			t.Fatalf("Decode(Encode(%v)): %v", seq, err)
		}
		if !reflect.DeepEqual(got, seq) {
			t.Fatalf("round trip %v -> %v", seq, got)
		}
	}
	if _, err := Decode([]byte{0x80}); err == nil {
		t.Fatal("Decode of a truncated uvarint: no error")
	}
}
