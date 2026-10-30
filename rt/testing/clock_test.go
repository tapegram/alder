package rtesting

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"alder/rt/eff"
)

var setAt = time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)

// A set clock stands still: every reading is the set time until something moves it.
func TestClock_SetStandsStill(t *testing.T) {
	c := NewClock(defaultNow)
	c.Set(setAt)
	for i := 0; i < 3; i++ {
		if got := c.Now(); !got.Equal(setAt) {
			t.Fatalf("reading %d: %v, want %v", i, got, setAt)
		}
	}
	c.Advance(6 * time.Second)
	for i := 0; i < 2; i++ {
		if got := c.Now(); !got.Equal(setAt.Add(6 * time.Second)) {
			t.Fatalf("after advance, reading %d: %v", i, got)
		}
	}
}

// Advancing a ticking clock keeps the tick.
func TestClock_AdvanceKeepsTick(t *testing.T) {
	c := NewClock(defaultNow)
	c.Now()
	c.Advance(5 * time.Second)
	if got := c.Now(); !got.Equal(defaultNow.Add(6 * time.Second)) {
		t.Fatalf("got %v", got)
	}
	if got := c.Now(); !got.Equal(defaultNow.Add(7 * time.Second)) {
		t.Fatalf("the tick stopped: %v", got)
	}
}

// A second set moves the clock again, backwards too.
func TestClock_SetTwiceMovesBack(t *testing.T) {
	c := NewClock(defaultNow)
	c.Set(setAt)
	c.Set(defaultNow)
	if got := c.Now(); !got.Equal(defaultNow) {
		t.Fatalf("got %v", got)
	}
	if got := c.Now(); !got.Equal(defaultNow) {
		t.Fatalf("a set clock ticked: %v", got)
	}
}

// readNow is a Time.now step: it appends e.Now() to *into.
func readNow(into *[]time.Time) eff.Task[struct{}] {
	return eff.Call("Time.now", false, func(e *eff.Env) (struct{}, error) {
		*into = append(*into, e.Now())
		return struct{}{}, nil
	})
}

// traceInto is a Test.trace step that keeps the trace and succeeds.
func traceInto(into *[]string) eff.Task[bool] {
	return eff.Call("Test.trace", false, func(e *eff.Env) (bool, error) {
		*into = Trace(e)
		return true, nil
	})
}

func TestRun_SetClockStandsStill(t *testing.T) {
	var reads []time.Time
	var trace []string
	rs := one(Case{Module: "M", Name: "set", Task: func() eff.Task[bool] {
		return then(SetClock(setAt), then(readNow(&reads), then(readNow(&reads), then(readNow(&reads), traceInto(&trace)))))
	}})
	if !rs[0].OK || len(reads) != 3 {
		t.Fatalf("%+v reads=%v", rs, reads)
	}
	for _, r := range reads {
		if !r.Equal(setAt) {
			t.Fatalf("reads %v, want all %v", reads, setAt)
		}
	}
	want := []string{"Test.setClock 2026-03-04T05:06:07Z", "Time.now", "Time.now", "Time.now"}
	if !reflect.DeepEqual(trace, want) {
		t.Fatalf("trace %q, want %q", trace, want)
	}
}

// A negative advance is 0; an advance after a set moves the set time.
func TestRun_AdvanceClampsAndTraces(t *testing.T) {
	var reads []time.Time
	var trace []string
	rs := one(Case{Module: "M", Name: "advance", Task: func() eff.Task[bool] {
		return then(SetClock(setAt), then(Advance(-5), then(readNow(&reads), then(Advance(6000), then(readNow(&reads), traceInto(&trace))))))
	}})
	if !rs[0].OK || len(reads) != 2 || !reads[0].Equal(setAt) || !reads[1].Equal(setAt.Add(6*time.Second)) {
		t.Fatalf("%+v reads=%v", rs, reads)
	}
	want := []string{"Test.setClock 2026-03-04T05:06:07Z", "Test.advance 0ms", "Time.now", "Test.advance 6000ms", "Time.now"}
	if !reflect.DeepEqual(trace, want) {
		t.Fatalf("trace %q, want %q", trace, want)
	}
}

// Advancing a clock the test never set keeps it ticking.
func TestRun_AdvanceTicking(t *testing.T) {
	var reads []time.Time
	rs := one(Case{Module: "M", Name: "tick", Task: func() eff.Task[bool] {
		return then(readNow(&reads), then(Advance(60000), then(readNow(&reads), then(readNow(&reads), eff.Pure(true)))))
	}})
	want := []time.Time{defaultNow, defaultNow.Add(61 * time.Second), defaultNow.Add(62 * time.Second)}
	if !rs[0].OK || !reflect.DeepEqual(reads, want) {
		t.Fatalf("%+v reads=%v want %v", rs, reads, want)
	}
}

// The clock is not transactional: a set inside a Test.run whose task fails is kept.
func TestRun_SetClockKeptAfterFailedRun(t *testing.T) {
	var reads []time.Time
	rs := one(Case{Module: "M", Name: "kept", Task: func() eff.Task[bool] {
		return eff.Bind(testRun(then(SetClock(setAt), eff.Fail[int]("nope"))), func(r res) eff.Task[bool] {
			return then(readNow(&reads), eff.Pure(!r.ok && r.msg == "nope"))
		})
	}})
	if !rs[0].OK || len(reads) != 1 || !reads[0].Equal(setAt) {
		t.Fatalf("%+v reads=%v", rs, reads)
	}
}

// Each case has its own clock: a set in one does not reach the next.
func TestRun_SetClockPerCase(t *testing.T) {
	var reads []time.Time
	rs := Run([]Case{
		{Module: "M", Name: "a", Task: func() eff.Task[bool] { return then(SetClock(setAt), eff.Pure(true)) }},
		{Module: "M", Name: "b", Task: func() eff.Task[bool] { return then(readNow(&reads), eff.Pure(true)) }},
	}, Options{Tables: tables})
	if !rs[0].OK || !rs[1].OK || len(reads) != 1 || !reads[0].Equal(defaultNow) {
		t.Fatalf("%+v reads=%v", rs, reads)
	}
}

// Outside a test's Env there is no clock to set (unreachable from a `test`).
func TestSetClock_NoClockPanics(t *testing.T) {
	defer func() {
		r := recover()
		if s, _ := r.(string); !strings.Contains(s, "alder: this test has no clock") {
			t.Fatalf("panic %v", r)
		}
	}()
	eff.Run(&eff.Env{Ctx: context.Background()}, eff.KTask, SetClock(setAt))
	t.Fatal("no panic")
}
