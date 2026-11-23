package jobs_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"alder/rt/applog"
	"alder/rt/eff"
	"alder/rt/jobs"
	"alder/rt/metrics"
	rtesting "alder/rt/testing"
)

// idLines collects the lines written through Options.LogID: "<id> <level> <msg>".
type idLines struct {
	mu  sync.Mutex
	all []string
}

func (l *idLines) logID(id, level, msg string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.all = append(l.all, id+" "+level+" "+msg)
}

func (l *idLines) lines() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.all...)
}

// with is the line that starts with prefix, or "".
func (l *idLines) with(prefix string) string {
	for _, x := range l.lines() {
		if strings.HasPrefix(x, prefix) {
			return x
		}
	}
	return ""
}

// metricValue is the value of series in c's exposition, or "".
func metricValue(t *testing.T, c *metrics.Collector, series string) string {
	t.Helper()
	var b strings.Builder
	if err := c.Write(&b, "b"); err != nil {
		t.Fatal(err)
	}
	for _, l := range strings.Split(b.String(), "\n") {
		if v, ok := strings.CutPrefix(l, series+" "); ok {
			return v
		}
	}
	return ""
}

// Spec §9: one line per attempt under the job's id: done at info, a failure at warn with the
// error and the next time, dead at error; Log.* inside the job goes under the job's id too. The
// metrics count each attempt by kind and outcome.
func TestJobLog_Lines(t *testing.T) {
	st := openFile(t)
	c := rtesting.NewClock(t0)
	c.Set(t0)
	lg := &idLines{}
	m := metrics.New("")
	logging := eff.Call("Log.info", false, func(e *eff.Env) (struct{}, error) {
		e.Log("info", "hello from the job")
		return struct{}{}, nil
	})
	done := kind(logging)
	failing := kind(eff.Fail[struct{}]("smtp: timeout"))
	failing.Name = "Mail.Flaky"
	dead := kind(eff.Fail[struct{}]("no such user"))
	dead.Name, dead.Retries = "Mail.Once", 0
	insertRow(t, st, "aaaa0001", done.Name, "queued", t0, 0, nil, nil)
	insertRow(t, st, "aaaa0002", failing.Name, "queued", t0, 1, nil, nil) // its second attempt
	insertRow(t, st, "aaaa0003", dead.Name, "queued", t0, 0, nil, nil)
	stop := jobs.Start(context.Background(), jobs.Options{
		Store: st,
		Kinds: []jobs.Kind{done, failing, dead},
		Env: func(ctx context.Context, k jobs.Kind) *eff.Env {
			return &eff.Env{Ctx: ctx, Store: st, Now: c.Now} // no Log: the worker's, under the job's id
		},
		Workers:   1,
		Now:       c.Now,
		Log:       func(level, msg string) { t.Errorf("a line without an id: %s %s", level, msg) },
		LogID:     lg.logID,
		Metrics:   m,
		Poll:      time.Hour,
		Housekeep: time.Hour,
	})
	t.Cleanup(func() { stop(0) })
	eventually(t, "three attempt lines", func() bool {
		return lg.with("aaaa0001 info job ") != "" && lg.with("aaaa0002 warn job ") != "" && lg.with("aaaa0003 error job ") != ""
	})
	stop(time.Second)

	if got := lg.with("aaaa0001 info hello"); got != "aaaa0001 info hello from the job" {
		t.Errorf("the job's Log.info: %q in %q", got, lg.lines())
	}
	if got := lg.with("aaaa0001 info job "); !strings.HasPrefix(got, "aaaa0001 info job Signup.SendWelcome aaaa0001 done ") || !strings.HasSuffix(got, "ms attempt 1") {
		t.Errorf("done: %q in %q", got, lg.lines())
	}
	if got, want := lg.with("aaaa0002 warn job "), `aaaa0002 warn job Mail.Flaky aaaa0002 failed attempt 2/5 "smtp: timeout", next in 1m`; got != want {
		t.Errorf("failed: %q, want %q", got, want)
	}
	if got, want := lg.with("aaaa0003 error job "), `aaaa0003 error job Mail.Once aaaa0003 failed attempt 1/1 "no such user", dead`; got != want {
		t.Errorf("dead: %q, want %q", got, want)
	}
	for series, want := range map[string]string{
		`alder_jobs_total{kind="Signup.SendWelcome",outcome="done"}`: "1",
		`alder_jobs_total{kind="Mail.Flaky",outcome="failed"}`:       "1",
		`alder_jobs_total{kind="Mail.Once",outcome="dead"}`:          "1",
		`alder_job_duration_seconds_count{kind="Mail.Flaky"}`:        "1",
		`alder_jobs_running`: "0",
		`alder_jobs_queued`:  "0", // the retry is due at t0 + 1m
	} {
		if got := metricValue(t, m, series); got != want {
			t.Errorf("%s = %q, want %q", series, got, want)
		}
	}
	c.Set(t0.Add(time.Minute))
	if got := metricValue(t, m, "alder_jobs_queued"); got != "1" {
		t.Errorf("alder_jobs_queued at t0 + 1m = %q, want 1", got)
	}
}

// Spec §6a: an attempt that held the write transaction more than 2 s is marked in its line.
// LockHeld is 0 under env.Within, so the Outcomes are made here.
func TestJobLog_LockHeld(t *testing.T) {
	const mark = "held the write lock 2 s: write last, or fan out — book 17"
	out := jobs.Outcome{ID: "a1b2c3d4", Kind: "Digest.Send", Attempt: 1, Attempts: 5, Took: 2200 * time.Millisecond, LockHeld: 2100 * time.Millisecond}
	j := out.Line()
	if !strings.Contains(j.Text(), mark) || j.Level() != applog.Warn {
		t.Fatalf("2.1 s: %s %q", j.Level(), j.Text())
	}
	out.LockHeld = 1900 * time.Millisecond
	if j := out.Line(); strings.Contains(j.Text(), "write lock") || j.Level() != applog.Info {
		t.Fatalf("1.9 s: %s %q", j.Level(), j.Text())
	}
}

// Spec §6: a cut-off attempt's line is "cancelled", at info, with no error (Err is the
// cancellation, which is no failure).
func TestJobLog_Cancelled(t *testing.T) {
	out := jobs.Outcome{ID: "a1b2c3d4", Kind: "Signup.SendWelcome", Err: errors.New("cancelled"), Cancelled: true, Attempt: 1, Attempts: 5, Took: 120 * time.Millisecond}
	j := out.Line()
	if j.Outcome != "cancelled" || j.Err != "" || j.Next != 0 || j.Level() != applog.Info ||
		j.Text() != "job Signup.SendWelcome a1b2c3d4 cancelled 120ms attempt 1" {
		t.Fatalf("%+v %s %q", j, j.Level(), j.Text())
	}
}

// Options.Line, when set, gets each attempt's record instead of LogID (the generated App passes
// applog.Logger.Job).
func TestJobLog_LineHook(t *testing.T) {
	st := openFile(t)
	c := rtesting.NewClock(t0)
	c.Set(t0)
	var mu sync.Mutex
	var got []applog.Job
	insertRow(t, st, "aaaa0001", welcome.Kind, "queued", t0, 0, nil, nil)
	lg := &idLines{}
	stop := jobs.Start(context.Background(), jobs.Options{
		Store:     st,
		Kinds:     []jobs.Kind{kind(eff.Pure(struct{}{}))},
		Env:       func(ctx context.Context, k jobs.Kind) *eff.Env { return &eff.Env{Ctx: ctx, Store: st, Now: c.Now} },
		Workers:   1,
		Now:       c.Now,
		LogID:     lg.logID,
		Line:      func(j *applog.Job) { mu.Lock(); got = append(got, *j); mu.Unlock() },
		Poll:      time.Hour,
		Housekeep: time.Hour,
	})
	t.Cleanup(func() { stop(0) })
	eventually(t, "the attempt", func() bool { mu.Lock(); defer mu.Unlock(); return len(got) == 1 })
	stop(time.Second)
	if j := got[0]; j.ID != "aaaa0001" || j.Kind != welcome.Kind || j.Outcome != "done" || j.Attempt != 1 || j.Attempts != 5 {
		t.Fatalf("%+v", j)
	}
	if l := lg.with("aaaa0001 info job "); l != "" {
		t.Fatalf("the attempt also went to LogID: %q", l)
	}
	if l := lg.with("aaaa0001 effect Job.done"); l == "" {
		t.Fatalf("the job's effect steps are not under its id: %q", lg.lines())
	}
}
