package jobs_test

import (
	"context"
	"strings"
	"testing"

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
