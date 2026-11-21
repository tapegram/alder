package jobs_test

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"alder/rt/db"
	"alder/rt/eff"
	"alder/rt/jobs"
	"alder/rt/metrics"
	rtesting "alder/rt/testing"
)

// The active set (spec §7): the workers poll the shared store's jobs table and those of the
// silos in their active set — every silo with jobs found by one scan at start, and every silo
// an enqueue of this process touched since — and a silo leaves the set after an hour with
// nothing due.

// reopened is the Stores of the app at dir with no silo open (another process's, or after a
// restart).
func reopened(t *testing.T, dir string, shared *db.SQLite) *db.Stores {
	return reopenedIdle(t, dir, shared, 0)
}

// reopenedIdle is reopened with its silos closed after idle without a use (0: the default).
func reopenedIdle(t *testing.T, dir string, shared *db.SQLite, idle time.Duration) *db.Stores {
	t.Helper()
	cfg := siloCfg
	cfg.Dir, cfg.IdleClose = dir, idle
	s, err := db.OpenStores(shared, filepath.Join(dir, "app.db"), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// housekept counts the housekeeping passes of the workers startSilos starts.
type housekept struct{ n atomic.Int64 }

// wait returns once two more passes have ended: one began after the call.
func (h *housekept) wait(t *testing.T) {
	t.Helper()
	n := h.n.Load()
	eventually(t, "two housekeeping passes", func() bool { return h.n.Load() >= n+2 })
}

// startSilos starts one worker on shared and the silos of s, with clock c, its lines in lg and
// each attempt's Outcome sent on the returned channel; hk counts its housekeeping passes.
func startSilos(t *testing.T, shared *db.SQLite, s *db.Stores, c *rtesting.Clock, lg *lines, o jobs.Options) (<-chan jobs.Outcome, *housekept) {
	t.Helper()
	hk := &housekept{}
	outs := make(chan jobs.Outcome, 8)
	o.Store, o.Stores, o.Workers, o.Now, o.Log = shared, s, 1, c.Now, lg.log
	o.Env = func(ctx context.Context, k jobs.Kind) *eff.Env {
		return &eff.Env{Ctx: ctx, Store: shared, Stores: s, Now: c.Now}
	}
	o.OnAttempt = func(out jobs.Outcome) { outs <- out }
	if o.Poll == 0 {
		o.Poll = time.Millisecond
	}
	if o.Housekeep == 0 {
		o.Housekeep = time.Hour
	}
	stop := jobs.StartHousekept(context.Background(), o, func() { hk.n.Add(1) })
	t.Cleanup(func() { stop(0) })
	return outs, hk
}

func outcome(t *testing.T, outs <-chan jobs.Outcome) jobs.Outcome {
	t.Helper()
	select {
	case o := <-outs:
		return o
	case <-time.After(5 * time.Second):
		t.Fatal("no attempt ran")
	}
	return jobs.Outcome{}
}

// scanned waits for the line of the workers' start scan.
func scanned(t *testing.T, lg *lines) {
	t.Helper()
	eventually(t, "the start scan", func() bool { return len(lg.with("info", "jobs: the start scan")) == 1 })
}

// Two silo files on disk, A with a due row and B with none, neither open, and C with a due row
// whose org is gone: the worker runs A's row after its start; B is not opened, and C, which no
// request can reach, is dropped from the set without its row run.
func TestActiveSet_StartScan(t *testing.T) {
	dir, shared, s1 := siloApp(t)
	insertOrgs(t, shared, "C")
	idA := runEnqueue(t, orgEnv(t, shared, s1, "A"), welcome, nil)
	orgEnv(t, shared, s1, "B") // B's file, its jobs table empty
	idC := runEnqueue(t, orgEnv(t, shared, s1, "C"), welcome, nil)
	s1.Close()
	if _, err := raw(t, shared).ExecContext(context.Background(), `DELETE FROM "Org__Org" WHERE "id" = 'C'`); err != nil {
		t.Fatal(err)
	}
	s := reopened(t, dir, shared)
	c := rtesting.NewClock(t0)
	c.Set(t0)
	lg := &lines{}
	outs, _ := startSilos(t, shared, s, c, lg, jobs.Options{Kinds: []jobs.Kind{kind(insertNote("from A"))}})
	if o := outcome(t, outs); o.ID != idA || o.Err != nil {
		t.Fatalf("outcome %+v", o)
	}
	scanned(t, lg)
	if got := lg.with("info", "jobs: the start scan"); got[0] != "info jobs: the start scan looked in 3 silo(s): 2 with jobs" {
		t.Errorf("scan line %q", got)
	}
	eventually(t, "C to leave the set", func() bool { return slices.Equal(jobs.Active(s), []string{"org/A"}) })
	if r := read(t, siloFile(t, dir, "A"), idA); r.state != "done" {
		t.Fatalf("A's row %+v", r)
	}
	if r := read(t, siloFile(t, dir, "C"), idC); r.state != "queued" || r.attempts != 0 {
		t.Fatalf("C's row %+v", r)
	}
	ps, err := s.OpenSilos(context.Background())
	if err != nil || len(ps) != 1 || ps[0].Value != "A" {
		t.Fatalf("open silos %+v %v: only A's should be", ps, err)
	}
}

// A silo an enqueue of this process touched joins the set, and its row runs at once; with
// nothing due there for an hour, it leaves.
func TestActiveSet_Enqueue(t *testing.T) {
	_, shared, s := siloApp(t)
	c := rtesting.NewClock(t0)
	c.Set(t0)
	lg := &lines{}
	outs, hk := startSilos(t, shared, s, c, lg, jobs.Options{Kinds: []jobs.Kind{kind(insertNote("from A"))}, Housekeep: 2 * time.Millisecond})
	scanned(t, lg)
	if got := jobs.Active(s); len(got) != 0 {
		t.Fatalf("active at start: %q", got)
	}
	id := runEnqueue(t, orgEnv(t, shared, s, "A"), welcome, nil)
	if o := outcome(t, outs); o.ID != id || o.Err != nil {
		t.Fatalf("outcome %+v", o)
	}
	if got := jobs.Active(s); !slices.Equal(got, []string{"org/A"}) {
		t.Fatalf("active after the enqueue: %q", got)
	}
	// a row queued for later (a retry's backoff, an enqueueAt) keeps it, however long
	later, err := eff.Run(orgEnv(t, shared, s, "B"), eff.KTask, eff.Call("Job.enqueueAt", true, func(e *eff.Env) (string, error) {
		return jobs.Enqueue(e, welcome, t0.Add(3*time.Hour), nil)
	}))
	if err != nil {
		t.Fatal(err)
	}
	c.Set(t0.Add(59 * time.Minute))
	hk.wait(t)
	if got := jobs.Active(s); !slices.Equal(got, []string{"org/A", "org/B"}) {
		t.Fatalf("active at 59 min: %q", got)
	}
	c.Set(t0.Add(time.Hour + time.Minute))
	eventually(t, "A to leave the set", func() bool { return slices.Equal(jobs.Active(s), []string{"org/B"}) })
	c.Set(t0.Add(3 * time.Hour))
	if o := outcome(t, outs); o.ID != later || o.Err != nil {
		t.Fatalf("outcome %+v", o)
	}
}

// The queued gauge and /_alder/health's stall check read the active silos too.
func TestActiveSet_QueuedAndHealth(t *testing.T) {
	dir, shared, s1 := siloApp(t)
	k := kind(insertNote("late"))
	id := runEnqueue(t, orgEnv(t, shared, s1, "A"), welcome, nil)
	if _, err := raw(t, siloFile(t, dir, "A")).ExecContext(context.Background(), `UPDATE "_alder_jobs" SET "run_at" = ? WHERE "id" = ?`, t0.Add(time.Minute).UnixMilli(), id); err != nil {
		t.Fatal(err)
	}
	s1.Close()
	s := reopened(t, dir, shared)
	c := rtesting.NewClock(t0)
	c.Set(t0)
	lg := &lines{}
	m := metrics.New("")
	h := &jobs.Health{}
	_, _ = startSilos(t, shared, s, c, lg, jobs.Options{Kinds: []jobs.Kind{k}, Metrics: m, Health: h, Poll: time.Hour}) // one look, then idle for an hour
	scanned(t, lg)
	if got := metricValue(t, m, "alder_jobs_queued"); got != "0" {
		t.Errorf("alder_jobs_queued at t0 = %q, want 0", got)
	}
	c.Set(t0.Add(7 * time.Minute))
	if got := metricValue(t, m, "alder_jobs_queued"); got != "1" {
		t.Errorf("alder_jobs_queued at t0 + 7m = %q, want 1 (silo A's row)", got)
	}
	if got := h.State(context.Background()); got != "stalled" {
		t.Errorf("health %q, want stalled (silo A's row due 6 min ago)", got)
	}
}

// A claim that keeps failing is a warn line the first time, then at most one a minute for that
// store while it fails, and one line when it works again (the injected clock).
func TestWorker_ClaimErrorRateLimited(t *testing.T) {
	st := openFile(t)
	c := rtesting.NewClock(t0)
	c.Set(t0)
	lg := &lines{}
	if _, err := raw(t, st).ExecContext(context.Background(), `ALTER TABLE "_alder_jobs" RENAME TO "away"`); err != nil {
		t.Fatal(err)
	}
	start(t, st, c, lg, 2, time.Millisecond, kind(eff.Pure(struct{}{})))
	claims := func() []string { return lg.with("warn", "jobs: claiming a job") }
	eventually(t, "the first warn line", func() bool { return len(claims()) == 1 })
	time.Sleep(30 * time.Millisecond) // many polls of both workers
	c.Set(t0.Add(59 * time.Second))
	time.Sleep(10 * time.Millisecond)
	if got := claims(); len(got) != 1 || !strings.HasPrefix(got[0], "warn jobs: claiming a job in the shared store: ") || !strings.Contains(got[0], "_alder_jobs") {
		t.Fatalf("lines within the minute: %q", got)
	}
	c.Set(t0.Add(time.Minute))
	eventually(t, "the second warn line", func() bool { return len(claims()) == 2 })
	time.Sleep(10 * time.Millisecond)
	if got := claims(); len(got) != 2 {
		t.Fatalf("lines in the second minute: %q", got)
	}
	if _, err := raw(t, st).ExecContext(context.Background(), `ALTER TABLE "away" RENAME TO "_alder_jobs"`); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the line when it clears", func() bool {
		return len(lg.with("info", "jobs: claiming a job in the shared store works again")) == 1
	})
	time.Sleep(10 * time.Millisecond)
	if got := lg.with("info", "jobs: claiming a job"); len(got) != 1 {
		t.Fatalf("lines after it cleared: %q", got)
	}
	if got := claims(); len(got) != 2 {
		t.Fatalf("warn lines after it cleared: %q", got)
	}
}

// A silo whose only row is queued for later stays in the set but is not kept open by the
// workers: it closes at its idle time and stays closed under their polls, and it is opened
// again, and its row run, once the row comes due.
func TestActiveSet_FutureRowStaysClosed(t *testing.T) {
	dir, shared, s1 := siloApp(t)
	id, err := eff.Run(orgEnv(t, shared, s1, "A"), eff.KTask, eff.Call("Job.enqueueAt", true, func(e *eff.Env) (string, error) {
		return jobs.Enqueue(e, welcome, t0.Add(2*time.Hour), nil)
	}))
	if err != nil {
		t.Fatal(err)
	}
	s1.Close()
	s := reopenedIdle(t, dir, shared, 50*time.Millisecond)
	c := rtesting.NewClock(t0)
	c.Set(t0)
	lg := &lines{}
	outs, hk := startSilos(t, shared, s, c, lg, jobs.Options{Kinds: []jobs.Kind{kind(insertNote("later"))}, Housekeep: 2 * time.Millisecond})
	scanned(t, lg)
	open := func() bool {
		ps, err := s.OpenSilos(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		return len(ps) == 1
	}
	hk.wait(t) // the first look opened A and found its row due at t0 + 2h
	eventually(t, "A to close at its idle time", func() bool { return !open() })
	for range 5 { // many polls (each 1 ms) and passes later, it is still closed and in the set
		hk.wait(t)
	}
	if open() {
		t.Fatal("the workers' polls opened A again before its row was due")
	}
	if got := jobs.Active(s); !slices.Equal(got, []string{"org/A"}) {
		t.Fatalf("active %q", got)
	}
	c.Set(t0.Add(2 * time.Hour))
	if o := outcome(t, outs); o.ID != id || o.Err != nil {
		t.Fatalf("outcome %+v", o)
	}
	if r := read(t, siloFile(t, dir, "A"), id); r.state != "done" {
		t.Fatalf("row %+v", r)
	}
}
