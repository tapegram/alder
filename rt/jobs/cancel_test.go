package jobs_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"alder/rt/db"
	"alder/rt/eff"
	"alder/rt/jobs"
)

// Job.cancel (spec §3): the row goes cancelled where Job.status finds it, in the Task's
// transaction in the bound store, at once in the shared store reached from a silo-bound Task.

// cancelTask is Job.cancel id as std.JobCancel makes it.
func cancelTask(id string) eff.Task[bool] {
	return eff.Call("Job.cancel "+id, true, func(e *eff.Env) (bool, error) { return jobs.Cancel(e, id) })
}

func runCancel(t *testing.T, env *eff.Env, id string) bool {
	t.Helper()
	ok, err := eff.Run(env, eff.KTask, cancelTask(id))
	if err != nil {
		t.Fatal(err)
	}
	return ok
}

// A queued row: cancelled, finished now, its lease cleared, its attempts and error kept, an info
// line; a second cancel finds it final.
func TestCancel_Queued(t *testing.T) {
	st := open(t)
	env, _ := newEnv(st)
	var lines []string
	env.Log = func(level, msg string) {
		if level != "effect" {
			lines = append(lines, level+" "+msg)
		}
	}
	id := runEnqueue(t, env, welcome, nil)
	if _, err := raw(t, st).ExecContext(context.Background(), `UPDATE "_alder_jobs" SET "attempts" = 2, "error" = 'smtp down', "lease_until" = 1 WHERE "id" = ?`, id); err != nil {
		t.Fatal(err)
	}
	if !runCancel(t, env, id) {
		t.Fatal("Cancel of a queued row: false")
	}
	r := read(t, st, id)
	if r.state != "cancelled" || r.finished == nil || *r.finished != t0.UnixMilli() || r.leaseUntil != nil ||
		r.attempts != 2 || r.err == nil || *r.err != "smtp down" {
		t.Fatalf("row %+v", r)
	}
	if want := "info job Signup.SendWelcome " + id + " cancelled (was queued)"; len(lines) != 1 || lines[0] != want {
		t.Fatalf("log %q, want %q", lines, want)
	}
	if runCancel(t, env, id) {
		t.Fatal("a second Cancel: true")
	}
}

// No row, a done or dead row, and a schedule's queued tick are False and left as they are; a
// claimed tick (schedule 0 since its claim) is cancelled like any run.
func TestCancel_NoRowFinalTick(t *testing.T) {
	st := open(t)
	env, _ := newEnv(st)
	if runCancel(t, env, "00000000") {
		t.Fatal("an unknown id: true")
	}
	for _, state := range []string{"done", "dead"} {
		id := runEnqueue(t, env, welcome, nil)
		setState(t, st, id, state)
		if runCancel(t, env, id) {
			t.Fatalf("a %s row: true", state)
		}
		if r := read(t, st, id); r.state != state || r.finished != nil {
			t.Fatalf("a %s row: %+v", state, r)
		}
	}
	tick := runEnqueue(t, env, welcome, nil)
	if _, err := raw(t, st).ExecContext(context.Background(), `UPDATE "_alder_jobs" SET "schedule" = 1 WHERE "id" = ?`, tick); err != nil {
		t.Fatal(err)
	}
	if runCancel(t, env, tick) {
		t.Fatal("a queued tick: true")
	}
	if r := read(t, st, tick); r.state != "queued" {
		t.Fatalf("a queued tick: %+v", r)
	}
	if _, err := raw(t, st).ExecContext(context.Background(), `UPDATE "_alder_jobs" SET "schedule" = 0, "state" = 'running' WHERE "id" = ?`, tick); err != nil {
		t.Fatal(err)
	}
	if !runCancel(t, env, tick) {
		t.Fatal("a claimed tick: false")
	}
	if r := read(t, st, tick); r.state != "cancelled" {
		t.Fatalf("a claimed tick: %+v", r)
	}
}

// A cancelled row frees its key: enqueueOnce with it makes a new row.
func TestCancel_ReleasesKey(t *testing.T) {
	st := open(t)
	env, _ := newEnv(st)
	first := runEnqueue(t, env, welcome, ptr("k"))
	if !runCancel(t, env, first) {
		t.Fatal("Cancel: false")
	}
	if next := runEnqueue(t, env, welcome, ptr("k")); next == first {
		t.Fatalf("enqueueOnce after the cancel gave the cancelled row %s", first)
	}
}

// In the bound store the cancel is in the Task's transaction: a Task that fails afterwards
// un-cancels (review focus 4).
func TestCancel_InTaskTxRollsBack(t *testing.T) {
	st := open(t)
	env, _ := newEnv(st)
	id := runEnqueue(t, env, welcome, nil)
	task := eff.Bind(cancelTask(id), func(bool) eff.Task[bool] { return eff.Fail[bool]("refused") })
	if _, err := eff.Run(env, eff.KTask, task); err == nil {
		t.Fatal("the Task succeeded")
	}
	if r := read(t, st, id); r.state != "queued" || r.finished != nil {
		t.Fatalf("row %+v: the failed Task's cancel was kept", r)
	}
}

// A Task bound to silo A cancels a row of the shared store at once, apart from its transaction
// (decision 10): the row stays cancelled when the Task fails afterwards.
func TestCancel_SiloTaskSharedRow(t *testing.T) {
	_, shared, s := siloApp(t)
	id := runEnqueue(t, sharedEnv(shared, s), welcome, nil)
	var got bool
	task := eff.Bind(cancelTask(id), func(ok bool) eff.Task[bool] { got = ok; return eff.Fail[bool]("refused") })
	if _, err := eff.Run(orgEnv(t, shared, s, "A"), eff.KTask, task); err == nil || !strings.Contains(err.Error(), "refused") {
		t.Fatalf("the Task: %v", err)
	}
	if !got {
		t.Fatal("Cancel of the shared row from silo A: false")
	}
	if r := read(t, shared, id); r.state != "cancelled" {
		t.Fatalf("row %+v: the shared row's cancel was rolled back with the silo's Task", r)
	}
}

// A running attempt cut off (spec §3): the job's writes are in a store of their own (its Env's),
// so the jobs table stays free while the job holds its write transaction. noteThenBlock inserts a
// note and blocks until its context ends; started is told once it has inserted.
func noteThenBlock(started chan<- struct{}) jobs.Kind {
	return kind(eff.Bind(insertNote("cut off"), func(struct{}) eff.Task[struct{}] {
		return eff.Call("Block", false, func(e *eff.Env) (struct{}, error) {
			started <- struct{}{}
			<-e.Ctx.Done()
			return struct{}{}, e.Ctx.Err()
		})
	}))
}

// cutWorkers starts one worker on the jobs table of jobsSt (and the silos of s, when set) whose
// attempts write to app; outs gets each attempt's Outcome.
func cutWorkers(t *testing.T, jobsSt, app *db.SQLite, s *db.Stores, poll time.Duration, k jobs.Kind) (outs chan jobs.Outcome, stop func(time.Duration)) {
	t.Helper()
	outs = make(chan jobs.Outcome, 4)
	stop = jobs.Start(context.Background(), jobs.Options{
		Store: jobsSt, Stores: s, Kinds: []jobs.Kind{k}, Workers: 1, Poll: poll, Housekeep: time.Hour,
		Now:       func() time.Time { return t0 },
		Env:       func(ctx context.Context, k jobs.Kind) *eff.Env { return &eff.Env{Ctx: ctx, Store: app} },
		OnAttempt: func(o jobs.Outcome) { outs <- o },
	})
	t.Cleanup(func() { stop(0) })
	return outs, stop
}

// waitStarted waits for the job to have inserted its note.
func waitStarted(t *testing.T, started <-chan struct{}) {
	t.Helper()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("the job did not start")
	}
}

// cutOffWithin checks that the attempt of id ends Cancelled within d, its note rolled back, its
// row in st cancelled with one attempt, and nothing else recorded.
func cutOffWithin(t *testing.T, outs <-chan jobs.Outcome, d time.Duration, st, app *db.SQLite, id string) {
	t.Helper()
	select {
	case o := <-outs:
		if o.ID != id || !o.Cancelled || o.Stopped || o.Dead {
			t.Fatalf("outcome %+v", o)
		}
	case <-time.After(d):
		t.Fatalf("the attempt was not cut off within %v", d)
	}
	if n := notes(t, app); n != 0 {
		t.Fatalf("notes %d: the cut-off attempt's write was kept", n)
	}
	if r := read(t, st, id); r.state != "cancelled" || r.attempts != 1 || r.err != nil {
		t.Fatalf("row %+v", r)
	}
	select {
	case o := <-outs:
		t.Fatalf("a second outcome %+v", o)
	default:
	}
}

// Job.cancel in the process that runs the attempt cuts it off at once (the registry; no poll).
func TestWorker_CancelInProcess(t *testing.T) {
	st, app := openFile(t), openFile(t)
	env, _ := newEnv(st)
	id := runEnqueue(t, env, welcome, nil)
	started := make(chan struct{}, 1)
	outs, _ := cutWorkers(t, st, app, nil, time.Hour, noteThenBlock(started))
	waitStarted(t, started)
	if r := read(t, st, id); r.state != "running" {
		t.Fatalf("row %+v", r)
	}
	if !runCancel(t, env, id) {
		t.Fatal("Cancel of a running row: false")
	}
	cutOffWithin(t, outs, time.Second, st, app, id)
}

// A row cancelled behind the pool's back (another process, alder jobs cancel) is cut off by the
// watcher within a few polls.
func TestWorker_CancelFromElsewhere(t *testing.T) {
	st, app := openFile(t), openFile(t)
	env, _ := newEnv(st)
	id := runEnqueue(t, env, welcome, nil)
	started := make(chan struct{}, 1)
	outs, _ := cutWorkers(t, st, app, nil, 20*time.Millisecond, noteThenBlock(started))
	waitStarted(t, started)
	if _, err := raw(t, st).ExecContext(context.Background(), `UPDATE "_alder_jobs" SET "state" = 'cancelled' WHERE "id" = ?`, id); err != nil {
		t.Fatal(err)
	}
	cutOffWithin(t, outs, time.Second, st, app, id)
}

// The watcher reads a silo's table too (pool.at): a silo row cancelled there is cut off.
func TestWorker_CancelInSilo(t *testing.T) {
	_, shared, s := siloApp(t)
	env := orgEnv(t, shared, s, "A")
	silo := env.Store.(*db.SQLite)
	id := runEnqueue(t, env, welcome, nil)
	app := openFile(t)
	started := make(chan struct{}, 1)
	outs, _ := cutWorkers(t, shared, app, s, 20*time.Millisecond, noteThenBlock(started))
	waitStarted(t, started)
	if _, err := raw(t, silo).ExecContext(context.Background(), `UPDATE "_alder_jobs" SET "state" = 'cancelled' WHERE "id" = ?`, id); err != nil {
		t.Fatal(err)
	}
	cutOffWithin(t, outs, time.Second, silo, app, id)
}

// stop still re-queues an attempt it cancels: the attempt's own cancel does not hide stop's
// cause.
func TestWorker_StopStillRequeues(t *testing.T) {
	st, app := openFile(t), openFile(t)
	env, _ := newEnv(st)
	id := runEnqueue(t, env, welcome, nil)
	started := make(chan struct{}, 1)
	outs, stop := cutWorkers(t, st, app, nil, time.Hour, noteThenBlock(started))
	waitStarted(t, started)
	stop(0)
	select {
	case o := <-outs:
		if !o.Stopped || o.Cancelled {
			t.Fatalf("outcome %+v", o)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no outcome after stop")
	}
	if r := read(t, st, id); r.state != "queued" || r.attempts != 0 {
		t.Fatalf("row %+v", r)
	}
	if n := notes(t, app); n != 0 {
		t.Fatalf("notes %d", n)
	}
}
