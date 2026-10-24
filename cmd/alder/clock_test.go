package main

import (
	"sync"
	"testing"
	"time"
)

// fakeClock is a clock for the deploy tests (deployCmds.useFakeClock): Sleep advances it at
// once instead of waiting.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock(start time.Time) *fakeClock { return &fakeClock{now: start} }

// Now is the clock's time.
func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// Sleep advances the clock by d without waiting.
func (c *fakeClock) Sleep(d time.Duration) {
	if d <= 0 {
		return
	}
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

// pollUntil polls cond every 20 ms until it holds (true) or limit passes (false): a test waits
// for an outcome this way instead of sleeping a fixed time, then asserts it as before.
func pollUntil(limit time.Duration, cond func() bool) bool {
	for end := time.Now().Add(limit); ; time.Sleep(20 * time.Millisecond) {
		if cond() {
			return true
		}
		if time.Now().After(end) {
			return false
		}
	}
}

func TestFakeClock(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	c := newFakeClock(start)
	begin := time.Now()
	c.Sleep(time.Hour)
	if time.Since(begin) > 100*time.Millisecond {
		t.Error("Sleep waited")
	}
	if !c.Now().Equal(start.Add(time.Hour)) {
		t.Errorf("after Sleep(1h): %s", c.Now())
	}
}
