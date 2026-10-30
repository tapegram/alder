package rtesting

import (
	"fmt"
	"math"
	"sync"
	"time"

	"alder/rt/eff"
)

// Clock is a test's time. Until the test sets it, its first reading is start and each later
// one is a second after the one before (the tick), so the rows a test inserts get distinct
// `@created` times in the order they were inserted and a test can observe an ordering by
// creation time (`Db.orderBy T.created.desc`). Once set (Test.setClock) it stands still: every
// reading is the set time until Test.advance or a delay moves it. A delay (Env.Sleep) advances
// it by the delay at once, set or ticking. It is deterministic: the same test reads the same
// times on every run.
type Clock struct {
	mu   sync.Mutex
	next time.Time
	tick bool // false once set
}

func NewClock(start time.Time) *Clock { return &Clock{next: start, tick: true} }

func (c *Clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := c.next
	if c.tick {
		c.next = c.next.Add(time.Second)
	}
	return t
}

func (c *Clock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.next = c.next.Add(d)
}

// Set makes the next reading t and stops the tick for the rest of the test.
func (c *Clock) Set(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.next, c.tick = t, false
}

// Ticking is a clock's Now: each reading advances it a second.
func Ticking(start time.Time) func() time.Time { return NewClock(start).Now }

// clockKey carries a test case's *Clock in its Env's context (newCaseEnv), for Test.setClock
// and Test.advance. The clock is a test-only thing, so it is not an eff.Env field.
type clockKey struct{}

func clockOf(e *eff.Env) *Clock {
	if e.Ctx != nil {
		if c, ok := e.Ctx.Value(clockKey{}).(*Clock); ok {
			return c
		}
	}
	// unreachable from a `test` (the checker allows Test.* only there, and every test case
	// has a clock); the message is for a build that bypasses the runner
	panic("alder: this test has no clock")
}

// SetClock is Test.setClock: Time.now, Date.today and @created read t from here on, and the
// clock stands still. Not transactional: a set inside a failed Test.run is kept.
func SetClock(t time.Time) eff.Task[struct{}] {
	t = t.UTC()
	return eff.Call("Test.setClock "+t.Format(time.RFC3339), false, func(e *eff.Env) (struct{}, error) {
		clockOf(e).Set(t)
		return struct{}{}, nil
	})
}

// maxAdvanceMs is the largest advance a time.Duration holds (about 292 years): a larger ms
// saturates to it rather than wrapping into the past.
const maxAdvanceMs = math.MaxInt64 / int64(time.Millisecond)

// Advance is Test.advance: the clock moves ms milliseconds forward (a negative ms is 0; no
// upper clamp below maxAdvanceMs, where it saturates), keeping its state, set or ticking.
func Advance(ms int64) eff.Task[struct{}] {
	ms = min(max(ms, 0), maxAdvanceMs) // saturates: the clock only moves forward, never wraps
	return eff.Call(fmt.Sprintf("Test.advance %dms", ms), false, func(e *eff.Env) (struct{}, error) {
		clockOf(e).Advance(time.Duration(ms) * time.Millisecond)
		return struct{}{}, nil
	})
}
