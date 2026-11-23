package jobs_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"alder/rt/db"
	"alder/rt/eff"
	"alder/rt/jobs"
)

// claim does what the worker's claim does to row id (attempts + 1, running) and gives the row.
func claim(t *testing.T, st *db.SQLite, id string) jobs.Row {
	t.Helper()
	ctx := context.Background()
	if _, err := raw(t, st).ExecContext(ctx, `UPDATE "_alder_jobs" SET "state" = 'running', "attempts" = "attempts" + 1 WHERE "id" = ?`, id); err != nil {
		t.Fatal(err)
	}
	r := jobs.Row{ID: id, State: "running"}
	if err := raw(t, st).QueryRowContext(ctx, `SELECT "kind", "params", "attempts", "run_at", "created" FROM "_alder_jobs" WHERE "id" = ?`, id).
		Scan(&r.Kind, &r.Params, &r.Attempts, &r.RunAt, &r.Created); err != nil {
		t.Fatal(err)
	}
	return r
}

// kind is a job kind whose run is task, with the spec's defaults.
func kind(task eff.Task[struct{}]) jobs.Kind {
	return jobs.Kind{
		Name:    welcome.Kind,
		Run:     func(*eff.Env, string) (eff.Task[struct{}], error) { return task, nil },
		Retries: 4,
		Timeout: 5 * time.Minute,
	}
}

// insertNote writes a row of noteTable the way generated code does.
func insertNote(text string) eff.Task[struct{}] {
	return eff.Call("Db.insert Note", true, func(e *eff.Env) (struct{}, error) {
		_, err := e.Store.(db.Store).Insert(e.Ctx, db.TxOf(e), noteTable.Name, noteTable.Columns, "id", "", "", []any{nil, text}, e.Now())
		return struct{}{}, err
	})
}

func then(a, b eff.Task[struct{}]) eff.Task[struct{}] {
	return eff.Bind(a, func(struct{}) eff.Task[struct{}] { return b })
}

type jobRow struct {
	state      string
	runAt      int64
	attempts   int
	err        *string
	finished   *int64
	leaseUntil *int64
}

func read(t *testing.T, st *db.SQLite, id string) jobRow {
	t.Helper()
	var r jobRow
	if err := raw(t, st).QueryRowContext(context.Background(), `SELECT "state", "run_at", "attempts", "error", "finished", "lease_until" FROM "_alder_jobs" WHERE "id" = ?`, id).
		Scan(&r.state, &r.runAt, &r.attempts, &r.err, &r.finished, &r.leaseUntil); err != nil {
		t.Fatal(err)
	}
	return r
}

func notes(t *testing.T, st *db.SQLite) int {
	return count(t, st, `SELECT COUNT(*) FROM "Note__Note"`)
}

func TestRunOne_Done(t *testing.T) {
	st := open(t)
	env, _ := newEnv(st)
	id := runEnqueue(t, env, welcome, nil)
	out := jobs.RunOne(env, kind(insertNote("hi")), claim(t, st, id))
	if out.Err != nil || out.Dead || out.ID != id || out.Kind != welcome.Kind {
		t.Fatalf("outcome: %+v", out)
	}
	if r := read(t, st, id); r.state != "done" || r.finished == nil || *r.finished != t0.UnixMilli() || r.err != nil {
		t.Fatalf("job row: %+v", r)
	}
	if n := notes(t, st); n != 1 {
		t.Fatalf("notes: %d", n)
	}

	// a job that writes nothing: the done mark is the transaction's one statement
	id = runEnqueue(t, env, welcome, nil)
	row := claim(t, st, id)
	stats := &eff.Stats{}
	env.Stats = stats
	out = jobs.RunOne(env, kind(eff.Pure(struct{}{})), row)
	env.Stats = nil
	if out.Err != nil {
		t.Fatal(out.Err)
	}
	if stats.Exec != 1 || stats.Query != 0 {
		t.Fatalf("statements: %+v", stats)
	}
	if r := read(t, st, id); r.state != "done" {
		t.Fatalf("job row: %+v", r)
	}
}

func TestRunOne_FailRetries(t *testing.T) {
	waits := []time.Duration{10 * time.Second, time.Minute, 10 * time.Minute, time.Hour}
	st := open(t)
	env, _ := newEnv(st)
	failing := kind(then(insertNote("half"), eff.Fail[struct{}]("x")))
	id := runEnqueue(t, env, welcome, nil)
	for attempt := 1; attempt <= 5; attempt++ {
		out := jobs.RunOne(env, failing, claim(t, st, id))
		if out.Err == nil || out.Err.Error() != "x" {
			t.Fatalf("attempt %d: %v", attempt, out.Err)
		}
		r := read(t, st, id)
		if r.err == nil || *r.err != "x" || r.attempts != attempt {
			t.Fatalf("attempt %d: row %+v", attempt, r)
		}
		if n := notes(t, st); n != 0 {
			t.Fatalf("attempt %d: the Task's write was kept", attempt)
		}
		if attempt == 5 {
			if !out.Dead || r.state != "dead" {
				t.Fatalf("attempt 5: %+v %+v", out, r)
			}
			break
		}
		want := t0.Add(waits[attempt-1])
		if out.Dead || r.state != "queued" || r.runAt != want.UnixMilli() || !out.Next.Equal(want) {
			t.Fatalf("attempt %d: %+v %+v", attempt, out, r)
		}
	}

	once := failing
	once.Retries = 0
	id = runEnqueue(t, env, welcome, nil)
	if out := jobs.RunOne(env, once, claim(t, st, id)); !out.Dead || read(t, st, id).state != "dead" {
		t.Fatalf("retries 0: %+v", out)
	}

	six := failing
	six.Retries = 6
	id = runEnqueue(t, env, welcome, nil)
	for attempt := 1; attempt <= 7; attempt++ {
		out := jobs.RunOne(env, six, claim(t, st, id))
		if attempt == 7 {
			if !out.Dead {
				t.Fatalf("retries 6, attempt 7: %+v", out)
			}
			break
		}
		if attempt >= 5 && (out.Dead || !out.Next.Equal(t0.Add(time.Hour))) {
			t.Fatalf("retries 6, attempt %d: %+v", attempt, out)
		}
	}
}

func TestRunOne_ErrorText(t *testing.T) {
	st := open(t)
	env, _ := newEnv(st)
	for _, c := range []struct {
		name string
		task eff.Task[struct{}]
		want string
	}{
		{"fail", eff.Fail[struct{}]("x"), "x"},
		{"panic", eff.Call("Boom", false, func(*eff.Env) (struct{}, error) { panic("boom\nat line 2") }), "boom"},
		{"long", eff.Fail[struct{}](strings.Repeat("y", 5000)), strings.Repeat("y", 4096)},
	} {
		id := runEnqueue(t, env, welcome, nil)
		out := jobs.RunOne(env, kind(c.task), claim(t, st, id))
		if out.Err == nil {
			t.Fatalf("%s: no error", c.name)
		}
		if r := read(t, st, id); r.err == nil || *r.err != c.want {
			t.Fatalf("%s: error %v", c.name, r.err)
		}
	}
}

func TestRunOne_DecodeError(t *testing.T) {
	st := open(t)
	env, _ := newEnv(st)
	k := kind(eff.Pure(struct{}{}))
	k.Run = func(*eff.Env, string) (eff.Task[struct{}], error) {
		return eff.Task[struct{}]{}, errors.New(`field "user": missing`)
	}
	k.Retries = 4
	id := runEnqueue(t, env, welcome, nil)
	out := jobs.RunOne(env, k, claim(t, st, id))
	r := read(t, st, id)
	if !out.Dead || r.state != "dead" || r.attempts != 1 || r.err == nil || *r.err != `cannot decode the parameters: field "user": missing` {
		t.Fatalf("%+v %+v", out, r)
	}
}

func TestRunOne_Timeout(t *testing.T) {
	st := open(t)
	env, _ := newEnv(st)
	slow := eff.Call("Slow", false, func(e *eff.Env) (struct{}, error) {
		select {
		case <-e.Ctx.Done():
			return struct{}{}, e.Ctx.Err()
		case <-time.After(5 * time.Second):
			return struct{}{}, nil
		}
	})
	k := kind(slow)
	k.Timeout = 50 * time.Millisecond
	id := runEnqueue(t, env, welcome, nil)
	out := jobs.RunOne(env, k, claim(t, st, id))
	if out.Err == nil || out.Err.Error() != "timed out after 50ms" {
		t.Fatalf("outcome: %v", out.Err)
	}
	if r := read(t, st, id); r.err == nil || *r.err != "timed out after 50ms" || r.state != "queued" {
		t.Fatalf("row: %+v", r)
	}
}

// LockHeld is the time on the clock between the attempt's transaction beginning and its end.
func TestRunOne_LockHeld(t *testing.T) {
	st := open(t)
	env, c := newEnv(st)
	wait := eff.Call("Wait", false, func(*eff.Env) (struct{}, error) { c.Advance(3 * time.Second); return struct{}{}, nil })
	id := runEnqueue(t, env, welcome, nil)
	out := jobs.RunOne(env, kind(then(insertNote("early"), wait)), claim(t, st, id))
	if out.Err != nil || out.LockHeld != 3*time.Second || out.Took != 3*time.Second {
		t.Fatalf("write early: %+v", out)
	}
	id = runEnqueue(t, env, welcome, nil)
	out = jobs.RunOne(env, kind(then(wait, insertNote("late"))), claim(t, st, id))
	if out.Err != nil || out.LockHeld != 0 || out.Took != 3*time.Second {
		t.Fatalf("write late: %+v", out)
	}
}

func TestBackoff(t *testing.T) {
	for attempt, want := range map[int]time.Duration{
		1: 10 * time.Second, 2: time.Minute, 3: 10 * time.Minute, 4: time.Hour, 5: time.Hour, 7: time.Hour,
	} {
		if got := jobs.Backoff(attempt); got != want {
			t.Errorf("Backoff(%d) = %v, want %v", attempt, got, want)
		}
	}
}

// Only the attempt's own timeout is "timed out after": a deadline of the caller's context, and
// a job's own failure once its timeout has passed, are recorded as they are.
func TestRunOne_TimeoutCause(t *testing.T) {
	st := open(t)
	env, _ := newEnv(st)
	waitDone := eff.Call("Wait", false, func(e *eff.Env) (struct{}, error) {
		<-e.Ctx.Done()
		return struct{}{}, e.Ctx.Err()
	})
	k := kind(waitDone)
	k.Timeout = time.Hour
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	env.Ctx = ctx
	id := runEnqueue(t, &eff.Env{Ctx: context.Background(), Store: st, Now: env.Now}, welcome, nil)
	out := jobs.RunOne(env, k, claim(t, st, id))
	if out.Err == nil || strings.HasPrefix(out.Err.Error(), "timed out") {
		t.Fatalf("parent deadline: %v", out.Err)
	}
	env.Ctx = context.Background()

	ownFail := eff.Call("Fail", false, func(e *eff.Env) (struct{}, error) {
		<-e.Ctx.Done()
		return struct{}{}, &eff.ErrFail{Msg: "x"}
	})
	k = kind(ownFail)
	k.Timeout = 20 * time.Millisecond
	id = runEnqueue(t, env, welcome, nil)
	if out := jobs.RunOne(env, k, claim(t, st, id)); out.Err == nil || out.Err.Error() != "x" {
		t.Fatalf("own failure after the timeout: %v", out.Err)
	}
}

// cancelRaw cancels row id the way another process does: a statement on its own connection,
// outside the job's transaction.
func cancelRaw(t *testing.T, q db.Execer, id string) eff.Task[struct{}] {
	return eff.Call("cancel elsewhere", false, func(*eff.Env) (struct{}, error) {
		_, err := q.ExecContext(context.Background(), `UPDATE "_alder_jobs" SET "state" = 'cancelled', "finished" = 1, "lease_until" = NULL WHERE "id" = ?`, id)
		return struct{}{}, err
	})
}

// The backstop: a row cancelled while its job runs refuses the done mark, so the job's writes
// roll back, the row stays cancelled, and the attempt is Cancelled.
func TestRunOne_Backstop(t *testing.T) {
	st := open(t)
	env, _ := newEnv(st)
	id := runEnqueue(t, env, welcome, nil)
	out := jobs.RunOne(env, kind(then(cancelRaw(t, raw(t, st), id), insertNote("late"))), claim(t, st, id))
	if !out.Cancelled || out.Err == nil || out.Err.Error() != "cancelled" || out.Dead || !out.Next.IsZero() || out.Stopped {
		t.Fatalf("outcome %+v", out)
	}
	if n := notes(t, st); n != 0 {
		t.Fatalf("notes %d: the cancelled job's write was kept", n)
	}
	if r := read(t, st, id); r.state != "cancelled" {
		t.Fatalf("row %+v", r)
	}
}

// A job that fails on its own after another process cancelled its row: the failure is not
// recorded over the cancel, for a retry or a death (review focus 1).
func TestRunOne_FailureAfterCancelKeepsCancelled(t *testing.T) {
	for _, retries := range []int{4, 0} {
		st := open(t)
		env, _ := newEnv(st)
		id := runEnqueue(t, env, welcome, nil)
		k := kind(then(cancelRaw(t, raw(t, st), id), eff.Fail[struct{}]("smtp down")))
		k.Retries = retries
		out := jobs.RunOne(env, k, claim(t, st, id))
		if !out.Cancelled || out.Err == nil || out.Err.Error() != "cancelled" || out.Dead || !out.Next.IsZero() {
			t.Fatalf("retries %d: outcome %+v", retries, out)
		}
		if r := read(t, st, id); r.state != "cancelled" || r.err != nil {
			t.Fatalf("retries %d: row %+v", retries, r)
		}
	}
}

// A row in another store than the run's, cancelled while its job runs: the job has committed by
// the mark, so its writes stand, and the mark's refusal is a warn line.
func TestRunOne_ApartMarkAfterCancelWarns(t *testing.T) {
	_, shared, s := siloApp(t)
	env := orgEnv(t, shared, s, "A")
	silo := env.Store.(*db.SQLite)
	id := runEnqueue(t, env, welcome, nil)
	k := kind(then(cancelRaw(t, raw(t, silo), id), insertNote("stands")))
	run := sharedEnv(shared, s)
	var lines []string
	run.Log = func(level, msg string) {
		if level != "effect" {
			lines = append(lines, level+" "+msg)
		}
	}
	if out := jobs.RunOne(run, k, claimOn(t, placeOf(env), []jobs.Kind{k}, id)); out.Err != nil || out.Cancelled {
		t.Fatalf("outcome %+v", out)
	}
	if n := notes(t, shared); n != 1 {
		t.Fatalf("notes %d: the job's own commit is kept", n)
	}
	if r := read(t, silo, id); r.state != "cancelled" {
		t.Fatalf("row %+v", r)
	}
	if len(lines) != 1 || !strings.HasPrefix(lines[0], "warn ") || !strings.Contains(lines[0], "ran to its end before the cancel was seen; its writes stand") {
		t.Fatalf("log %q", lines)
	}
}
