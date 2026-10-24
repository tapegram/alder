package main

import (
	"sync"
	"testing"
	"time"
)

// fakeClock is a clock for the deploy tests: Sleep advances it at once instead of waiting.
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

// useFakeClock makes the deploy commands' targets read and wait by a fake clock that starts
// now, for the rest of the test, and polls fast (fastPolls). Serial tests only: deployTiming is
// the package's.
func useFakeClock(t *testing.T) *fakeClock {
	t.Helper()
	c := newFakeClock(time.Now())
	old := deployTiming
	deployTiming.Now, deployTiming.Sleep = c.Now, c.Sleep
	t.Cleanup(func() { deployTiming = old })
	fastPolls(t)
	return c
}

// fastPolls makes the deploy commands' Ssh scripts and Render polls look again every 10 ms
// (instead of 0.2 s to 3 s) for the rest of the test. Serial tests only.
func fastPolls(t *testing.T) {
	t.Helper()
	old := deployTiming.Poll
	deployTiming.Poll = 10 * time.Millisecond
	t.Cleanup(func() { deployTiming.Poll = old })
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
