package jobs

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"strings"
	"time"
	"unicode/utf8"

	"alder/rt/db"
	"alder/rt/eff"
)

// Kind is a job kind this build knows (generated: app.Jobs()), with its options' values.
type Kind struct {
	Name string // the qualified name, "Signup.SendWelcome": a row's kind
	// Run decodes a row's params and builds its Task (the job's run arm); the error is a decode
	// failure, and the row goes dead.
	Run       func(env *eff.Env, params string) (eff.Task[struct{}], error)
	Retries   int           // attempts after the first
	Timeout   time.Duration // the most one attempt may take; 0: no limit
	Every     time.Duration // 0 when not scheduled by interval
	Cron      string        // "" when not scheduled by cron
	SiloParam string        // "" when the job runs on the shared store
}

// MaxError is the most bytes of a failure's text the row keeps (4 KB).
const MaxError = 4096

// Outcome is how one attempt ended.
type Outcome struct {
	ID, Kind string
	Err      error // nil when done; its text is the row's error (before MaxError cuts it)
	Dead     bool  // the row is dead: past its retries, or its params cannot be decoded
	// Stopped: the workers' stop cancelled the attempt; the row is queued as before its claim
	// (the attempt is not counted, no error) and Err is the cancellation.
	Stopped  bool
	Attempt  int       // this attempt, 1 for the first (the row's attempts, counting it)
	Attempts int       // the most the job may have: its kind's retries plus one
	Next     time.Time // the retry's run_at when Err != nil && !Dead
	Took     time.Duration
	LockHeld time.Duration // how long the attempt's write transaction was open
}

// Backoff is the wait before the retry after a failed attempt (1 for the first): 10 s, 1 min,
// 10 min, then 1 h for the fourth and every later one.
func Backoff(attempt int) time.Duration {
	switch {
	case attempt <= 1:
		return 10 * time.Second
	case attempt == 2:
		return time.Minute
	case attempt == 3:
		return 10 * time.Minute
	}
	return time.Hour
}

// RunOne runs one attempt of row, which the caller has claimed (attempts counts this one, state
// running), as kind k, on env. The job's Task runs as one transaction whose last write marks the
// row done, so the job's writes and the mark commit together; a job that writes nothing commits
// the mark alone. The attempt runs under k.Timeout (env.Ctx carries it).
//
// A `silo k` job (k.SiloParam) runs bound to the silo its parameter names: k.Run narrows env to
// the parameter's value (db.Narrow) and RunOne binds the Task there (db.Enter); any other job runs
// on the shared store. The row is in row.In's store (env's when it is zero), the one its claimer
// found it in (a database, by db.Place.Same: a silo closed and opened again since is the same).
// When that is not the database the Task runs on, the done mark is a second short transaction
// after the job's commit, `WHERE "state" = 'running' AND "worker" IS` the claim's worker, so a
// lease expiry that re-queued the row in between (spec §6's window), or another worker's claim
// of it since, is left as it is; a mark that fails is a warn line, and the row's lease runs the
// job again.
//
// An attempt the workers' stop cancelled (Start) is not a failure: the row is queued again, its
// attempts not counted, its error, started and worker cleared (Outcome.Stopped) — only while it
// is still running under row.Worker: an attempt that outlived its lease, whose row was re-queued
// and claimed again elsewhere, leaves that claim alone.
//
// On a failure (Task.fail, a not found, a capability's error, a panic, the timeout, a silo that is
// not served) the Task's transaction is rolled back and a second short transaction in the row's
// store records the error and either the retry (queued, due Backoff(attempts) from now) or, past
// k.Retries, the row's death. Params k.Run cannot decode make the row dead at once whatever
// k.Retries says: the same build cannot decode them on a retry. Under env.Within (a test) the
// record is written in Within.Tx instead (in row.In.Tx, for a row in a test silo).
func RunOne(env *eff.Env, k Kind, row Row) Outcome {
	out := Outcome{ID: row.ID, Kind: row.Kind, Attempt: row.Attempts, Attempts: k.Retries + 1}
	at, base := row.In, ctxOf(env)
	if at.Store == nil {
		at = db.Place{Store: env.Store, Key: env.SiloKey, Value: env.SiloValue}
		if w := env.Within; w != nil {
			at.Tx = w.Tx
		}
	}
	reg, _ := env.Stores.(*db.Stores)
	prevCtx, prevBegin := env.Ctx, env.OnBegin
	start := env.Now()
	var began time.Time
	held := false
	env.OnBegin = func() {
		began, held = env.Now(), true
		if prevBegin != nil {
			prevBegin()
		}
	}
	ctx, cancel := base, context.CancelFunc(func() {})
	if k.Timeout > 0 {
		ctx, cancel = context.WithTimeoutCause(base, k.Timeout, errTimedOut)
	}
	env.Ctx = ctx
	err, decode, apart := attempt(env, k, row, at)
	end := env.Now()
	// the attempt's own timeout, not its caller's deadline, and not a failure of the job's own
	// that came after the timeout
	timedOut := context.Cause(ctx) == errTimedOut && errors.Is(err, context.DeadlineExceeded)
	stopped := context.Cause(base) == errStopped
	cancel()
	env.Ctx, env.OnBegin = prevCtx, prevBegin
	out.Took = end.Sub(start)
	if held {
		out.LockHeld = end.Sub(began)
	}
	if err == nil {
		if apart {
			if merr := record(reg, at, base, `UPDATE "_alder_jobs" SET "state" = 'done', "finished" = ?, "lease_until" = NULL WHERE "id" = ? AND "state" = 'running' AND "worker" IS ?`, env.Now().UnixMilli(), row.ID, workerArg(row)); merr != nil && env.Log != nil {
				env.Log("warn", fmt.Sprintf("job %s %s: marking it done in its store: %v (it ran; its lease runs it again)", row.Kind, row.ID, merr))
			}
		}
		return out
	}
	switch {
	case decode:
		err = &failure{"cannot decode the parameters: " + err.Error(), err}
	case stopped:
		out.Err, out.Stopped = err, true
		if rerr := record(reg, at, base, `UPDATE "_alder_jobs" SET "state" = 'queued', "attempts" = "attempts" - 1, "error" = NULL, "lease_until" = NULL, "started" = NULL, "worker" = NULL WHERE "id" = ? AND "state" = 'running' AND "worker" IS ?`, row.ID, workerArg(row)); rerr != nil {
			out.Err = errors.Join(err, fmt.Errorf("jobs: re-queueing the stopped attempt: %w", rerr))
		}
		return out
	case timedOut:
		err = &failure{"timed out after " + durationText(k.Timeout), err}
	}
	out.Err = err
	out.Dead = decode || row.Attempts > k.Retries
	now := env.Now()
	msg := clip(err.Error())
	var rerr error
	if out.Dead {
		rerr = record(reg, at, base, `UPDATE "_alder_jobs" SET "state" = 'dead', "error" = ?, "finished" = ?, "lease_until" = NULL WHERE "id" = ?`, msg, now.UnixMilli(), row.ID)
	} else {
		out.Next = now.Add(Backoff(row.Attempts))
		rerr = record(reg, at, base, `UPDATE "_alder_jobs" SET "state" = 'queued', "run_at" = ?, "error" = ?, "lease_until" = NULL WHERE "id" = ?`, out.Next.UnixMilli(), msg, row.ID)
	}
	if rerr != nil {
		out.Err = errors.Join(err, fmt.Errorf("jobs: recording the failure: %w", rerr))
	}
	return out
}

// attempt decodes row's params, binds env to the store k's jobs run on, and runs the job's Task
// followed by the done mark when the row (in at's store) is in that store; apart is set when it
// is not, and the caller marks it. decode is set when k.Run could not decode the params. A panic
// is the attempt's error (its first line), logged with its stack at level "error"; eff.Run has
// rolled the transaction back by then.
func attempt(env *eff.Env, k Kind, row Row, at db.Place) (err error, decode, apart bool) {
	defer func() {
		if v := recover(); v != nil {
			first, _, _ := strings.Cut(fmt.Sprint(v), "\n")
			err, decode = &failure{first, nil}, false
			if env.Log != nil {
				env.Log("error", fmt.Sprintf("job %s %s: panic: %v\n%s", row.Kind, row.ID, v, debug.Stack()))
			}
		}
	}()
	task, err := k.Run(env, row.Params)
	if err != nil {
		return err, true, false
	}
	if err := db.Enter(env, k.SiloParam); err != nil {
		return err, false, false
	}
	runs, err := db.RuntimeOn(env)
	if err != nil {
		return err, false, false
	}
	if !runs.Same(at) {
		_, err = eff.Run(env, eff.KTask, task)
		return err, false, true
	}
	done := eff.Call("Job.done", true, func(e *eff.Env) (struct{}, error) {
		q, err := on(e)
		if err != nil {
			return struct{}{}, err
		}
		_, err = q.ExecContext(ctxOf(e), `UPDATE "_alder_jobs" SET "state" = 'done', "finished" = ?, "lease_until" = NULL WHERE "id" = ?`, e.Now().UnixMilli(), row.ID)
		return struct{}{}, err
	})
	_, err = eff.Run(env, eff.KTask, eff.Bind(task, func(struct{}) eff.Task[struct{}] { return done }))
	return err, false, false
}

// record runs one statement on the jobs table of at's store: in at.Tx when it is set (a test's
// transaction), else in a transaction of its own, on ctx without its cancellation: a failure is
// recorded even when the attempt was cancelled. A silo's store is the one open now (reg.Place),
// not the one the row was claimed from, which the idle sweep may have closed since.
func record(reg *db.Stores, at db.Place, ctx context.Context, stmt string, args ...any) error {
	ctx = context.WithoutCancel(ctx)
	if at.Key != "" && at.Tx == nil && reg != nil {
		live, err := reg.Place(ctx, at.Key, at.Value)
		if err != nil {
			return err
		}
		at = live
	}
	if at.Tx != nil {
		q, err := db.On(at.Store, at.Tx)
		if err != nil {
			return err
		}
		_, err = q.ExecContext(ctx, stmt, args...)
		return err
	}
	if at.Store == nil {
		return errors.New("jobs: no store")
	}
	tx, err := at.Store.Begin(ctx)
	if err != nil {
		return err
	}
	q, err := db.On(at.Store, tx)
	if err == nil {
		_, err = q.ExecContext(ctx, stmt, args...)
	}
	if err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

// workerArg is the worker a guarded statement expects to hold row: row.Worker, or NULL for a row
// Test.work claimed (it names no worker).
func workerArg(row Row) any {
	if row.Worker == "" {
		return nil
	}
	return row.Worker
}

// errTimedOut is the cause of an attempt's context when its kind's timeout ends it.
var errTimedOut = errors.New("jobs: the attempt timed out")

// failure is an attempt's error with the text the row records: a timeout, a decode failure, a
// panic. Unwrap is the error under it (nil for a panic).
type failure struct {
	msg string
	err error
}

func (f *failure) Error() string { return f.msg }
func (f *failure) Unwrap() error { return f.err }

// clip is s cut to at most MaxError bytes, at a character boundary.
func clip(s string) string {
	if len(s) <= MaxError {
		return s
	}
	n := MaxError
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

// durationText is d as an Alder duration literal: "5m", "30s", "50ms", "1h" (d.String() when no
// single unit divides it).
func durationText(d time.Duration) string {
	for _, u := range []struct {
		d    time.Duration
		name string
	}{{time.Hour, "h"}, {time.Minute, "m"}, {time.Second, "s"}, {time.Millisecond, "ms"}} {
		if d > 0 && d%u.d == 0 {
			return fmt.Sprintf("%d%s", d/u.d, u.name)
		}
	}
	return d.String()
}
