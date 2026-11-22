package jobs_test

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"testing"
	"time"

	"alder/rt/db"
	"alder/rt/eff"
	"alder/rt/jobs"
	rtesting "alder/rt/testing"
)

// Silo hints (#168, PR 5): a process that runs no workers (ALDER_JOBS=0, the web service of a
// `workers:` deploy) has no active set for its enqueues to touch, so it names each silo it
// enqueues into in the shared store's _alder_jobs_silos, inside the Task's transaction (before
// the silo commits); the --jobs-only workers (Options.Hints) read and clear that table every
// poll and put the silos in their active set.

// hint is the touched time (ms) of the hint of org's silo in shared; ok false when there is none.
func hint(t *testing.T, shared *db.SQLite, org string) (touched int64, ok bool) {
	t.Helper()
	err := raw(t, shared).QueryRowContext(context.Background(), `SELECT "touched" FROM "_alder_jobs_silos" WHERE "key" = 'org' AND "value" = ?`, org).Scan(&touched)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false
	}
	if err != nil {
		t.Fatal(err)
	}
	return touched, true
}

func hints(t *testing.T, shared *db.SQLite) int {
	t.Helper()
	return count(t, shared, `SELECT COUNT(*) FROM "_alder_jobs_silos"`)
}

// An enqueue into a silo with no workers in this process leaves a hint, one per silo, its
// touched the enqueuer's clock; an enqueue on the shared store leaves none; the workers are
// woken all the same (harmless without any).
func TestHint_WrittenWithoutWorkers(t *testing.T) {
	_, shared, s := siloApp(t)
	jobs.Woken()
	runEnqueue(t, orgEnv(t, shared, s, "A"), welcome, nil)
	if ms, ok := hint(t, shared, "A"); !ok || ms != t0.UnixMilli() {
		t.Fatalf("hint %d %v, want %d", ms, ok, t0.UnixMilli())
	}
	if !jobs.Woken() {
		t.Error("the enqueue did not wake")
	}
	env := orgEnv(t, shared, s, "A")
	env.Now = func() time.Time { return t0.Add(time.Second) }
	runEnqueue(t, env, welcome, nil)
	if ms, ok := hint(t, shared, "A"); !ok || ms != t0.Add(time.Second).UnixMilli() {
		t.Fatalf("hint after the second enqueue %d %v", ms, ok)
	}
	runEnqueue(t, sharedEnv(shared, s), welcome, nil)
	if n := hints(t, shared); n != 1 {
		t.Fatalf("%d hints, want 1", n)
	}
}

// A process with workers on its stores writes no hint: its own active set has the silo.
func TestHint_NotWrittenWithWorkers(t *testing.T) {
	_, shared, s := siloApp(t)
	c := rtesting.NewClock(t0)
	c.Set(t0)
	lg := &lines{}
	outs, _ := startSilos(t, shared, s, c, lg, jobs.Options{Kinds: []jobs.Kind{kind(insertNote("from A"))}})
	scanned(t, lg)
	id := runEnqueue(t, orgEnv(t, shared, s, "A"), welcome, nil)
	if o := outcome(t, outs); o.ID != id || o.Err != nil {
		t.Fatalf("outcome %+v", o)
	}
	if n := hints(t, shared); n != 0 {
		t.Fatalf("%d hints", n)
	}
	if got := jobs.Active(s); !slices.Equal(got, []string{"org/A"}) {
		t.Fatalf("active %q", got)
	}
}

// Only workers started with Hints read the table: ones without leave a hint another process
// wrote; ones with it run the silo's job, clear the hint and keep the silo in their set.
func TestHint_OnlyJobsOnlyReads(t *testing.T) {
	dir, shared, s := siloApp(t)
	c := rtesting.NewClock(t0)
	c.Set(t0)
	k := []jobs.Kind{kind(insertNote("from A"))}
	other := reopened(t, dir, shared)
	lg := &lines{}
	outs, _ := startSilos(t, shared, other, c, lg, jobs.Options{Kinds: k})
	scanned(t, lg)
	id := runEnqueue(t, orgEnv(t, shared, s, "A"), welcome, nil) // s: no workers here
	select {
	case o := <-outs:
		t.Fatalf("a worker without Hints ran %+v", o)
	case <-time.After(50 * time.Millisecond):
	}
	if _, ok := hint(t, shared, "A"); !ok {
		t.Fatal("the hint went")
	}
	// the first workers keep running: they never look in A
	third := reopened(t, dir, shared)
	lg = &lines{}
	hinted, _ := startSilos(t, shared, third, c, lg, jobs.Options{Kinds: k, Hints: true})
	if o := outcome(t, hinted); o.ID != id || o.Err != nil {
		t.Fatalf("outcome %+v", o)
	}
	if _, ok := hint(t, shared, "A"); ok {
		t.Fatal("the hint is still there")
	}
	if got := jobs.Active(third); !slices.Equal(got, []string{"org/A"}) {
		t.Fatalf("active %q", got)
	}
}

// A hint whose Task failed (its job row rolled back with it) is harmless: the worker clears it,
// polls the empty silo, and drops it after ActiveFor.
func TestHint_FailedTaskIsHarmless(t *testing.T) {
	dir, shared, s := siloApp(t)
	failing := eff.Bind(enqueue(welcome, nil), func(string) eff.Task[string] {
		return eff.Call("Fail", false, func(*eff.Env) (string, error) { return "", &eff.ErrFail{Msg: "no"} })
	})
	if _, err := eff.Run(orgEnv(t, shared, s, "A"), eff.KTask, failing); err == nil {
		t.Fatal("the Task did not fail")
	}
	if n := count(t, siloFile(t, dir, "A"), `SELECT COUNT(*) FROM "_alder_jobs"`); n != 0 {
		t.Fatalf("%d job rows after the rollback", n)
	}
	if _, ok := hint(t, shared, "A"); !ok {
		t.Fatal("no hint: it is written before the silo commits")
	}
	c := rtesting.NewClock(t0)
	c.Set(t0)
	lg := &lines{}
	w := reopened(t, dir, shared)
	outs, hk := startSilos(t, shared, w, c, lg, jobs.Options{Kinds: []jobs.Kind{kind(insertNote("x"))}, Hints: true, Housekeep: 2 * time.Millisecond})
	eventually(t, "the hint to be read", func() bool { _, ok := hint(t, shared, "A"); return !ok })
	if got := jobs.Active(w); !slices.Equal(got, []string{"org/A"}) {
		t.Fatalf("active %q", got)
	}
	hk.wait(t)
	select {
	case o := <-outs:
		t.Fatalf("ran %+v", o)
	default:
	}
	c.Set(t0.Add(time.Hour + time.Minute))
	eventually(t, "A to leave the set", func() bool { return len(jobs.Active(w)) == 0 })
}

// A read of the hints that fails is a warn line, then at most one a minute, and a line when it
// works again.
func TestHint_ReadErrorRateLimited(t *testing.T) {
	_, shared, s := siloApp(t)
	if _, err := raw(t, shared).ExecContext(context.Background(), `ALTER TABLE "_alder_jobs_silos" RENAME TO "away"`); err != nil {
		t.Fatal(err)
	}
	c := rtesting.NewClock(t0)
	c.Set(t0)
	lg := &lines{}
	startSilos(t, shared, s, c, lg, jobs.Options{Kinds: []jobs.Kind{kind(insertNote("x"))}, Hints: true})
	warns := func() []string { return lg.with("warn", "jobs: claiming a job in the silo hints: ") }
	eventually(t, "the first warn line", func() bool { return len(warns()) == 1 })
	c.Set(t0.Add(30 * time.Second))
	time.Sleep(20 * time.Millisecond)
	if got := warns(); len(got) != 1 {
		t.Fatalf("lines within the minute: %q", got)
	}
	c.Set(t0.Add(time.Minute))
	eventually(t, "the second warn line", func() bool { return len(warns()) == 2 })
	if _, err := raw(t, shared).ExecContext(context.Background(), `ALTER TABLE "away" RENAME TO "_alder_jobs_silos"`); err != nil {
		t.Fatal(err)
	}
	c.Set(t0.Add(2 * time.Minute))
	eventually(t, "the line when it clears", func() bool {
		return len(lg.with("info", "jobs: claiming a job in the silo hints works again")) == 1
	})
}

// The hints are read every poll, not every Poll by the injected clock: under a frozen clock a
// hint written after the workers' first read is still picked up (the read is throttled by poll
// ticks, so eight workers cost one read a Poll, but a test clock that never moves does not stop
// it). A's hint is there before the start and is taken by the first read (its job runs); B's
// silo does not exist at the start scan and is reached only through its later hint.
func TestHint_ReadEveryPollUnderAFrozenClock(t *testing.T) {
	dir, shared, s := siloApp(t)
	idA := runEnqueue(t, orgEnv(t, shared, s, "A"), welcome, nil) // s: no workers here
	c := rtesting.NewClock(t0)
	c.Set(t0) // frozen: never moved again
	lg := &lines{}
	w := reopened(t, dir, shared)
	outs, _ := startSilos(t, shared, w, c, lg, jobs.Options{Kinds: []jobs.Kind{kind(insertNote("x"))}, Hints: true})
	if o := outcome(t, outs); o.ID != idA || o.Err != nil {
		t.Fatalf("A's outcome %+v", o)
	}
	if _, ok := hint(t, shared, "A"); ok {
		t.Fatal("A's hint is still there after the first read")
	}
	idB := runEnqueue(t, orgEnv(t, shared, s, "B"), welcome, nil)
	if _, ok := hint(t, shared, "B"); !ok {
		t.Fatal("no hint for B")
	}
	if o := outcome(t, outs); o.ID != idB || o.Err != nil {
		t.Fatalf("B's outcome %+v", o)
	}
	if _, ok := hint(t, shared, "B"); ok {
		t.Fatal("B's hint is still there")
	}
}

// A hint tick wakes an idle worker: with a Poll of an hour (its idle wait never ends in the
// test) and no Wake, a hint written while the worker waits is read and its job run on the next
// tick, so a hint waits at most one Poll (plus a claim turn) for an idle worker. B's row is kept
// from the start scan and the first read (done, its hint deleted), then queued and hinted again
// by hand, as another process's Enqueue would but without its Wake.
func TestHint_TickWakesAnIdleWorker(t *testing.T) {
	dir, shared, s := siloApp(t)
	idA := runEnqueue(t, orgEnv(t, shared, s, "A"), welcome, nil)
	idB := runEnqueue(t, orgEnv(t, shared, s, "B"), welcome, nil)
	ctx := context.Background()
	b := raw(t, siloFile(t, dir, "B"))
	if _, err := b.ExecContext(ctx, `UPDATE "_alder_jobs" SET "state" = 'done', "finished" = ? WHERE "id" = ?`, t0.UnixMilli(), idB); err != nil {
		t.Fatal(err)
	}
	if _, err := raw(t, shared).ExecContext(ctx, `DELETE FROM "_alder_jobs_silos" WHERE "value" = 'B'`); err != nil {
		t.Fatal(err)
	}
	c := rtesting.NewClock(t0)
	c.Set(t0)
	lg := &lines{}
	w := reopened(t, dir, shared)
	outs := make(chan jobs.Outcome, 8)
	tick := make(chan time.Time, 1)
	o := jobs.Options{Kinds: []jobs.Kind{kind(insertNote("x"))}, Hints: true, Store: shared, Stores: w, Workers: 1, Now: c.Now, Log: lg.log,
		Poll: time.Hour, Housekeep: time.Hour, OnAttempt: func(out jobs.Outcome) { outs <- out }}
	o.Env = func(ctx context.Context, k jobs.Kind) *eff.Env {
		return &eff.Env{Ctx: ctx, Store: shared, Stores: w, Now: c.Now}
	}
	stop := jobs.StartHintTicked(ctx, o, nil, tick)
	t.Cleanup(func() { stop(0) })
	if out := outcome(t, outs); out.ID != idA || out.Err != nil {
		t.Fatalf("A's outcome %+v", out)
	}
	if _, err := b.ExecContext(ctx, `UPDATE "_alder_jobs" SET "state" = 'queued', "finished" = NULL WHERE "id" = ?`, idB); err != nil {
		t.Fatal(err)
	}
	if _, err := raw(t, shared).ExecContext(ctx, `INSERT INTO "_alder_jobs_silos" ("key", "value", "touched") VALUES ('org', 'B', ?)`, t0.UnixMilli()); err != nil {
		t.Fatal(err)
	}
	tick <- t0
	if out := outcome(t, outs); out.ID != idB || out.Err != nil {
		t.Fatalf("B's outcome %+v", out)
	}
}
