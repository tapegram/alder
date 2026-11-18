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
// An attempt the workers' stop cancelled (Start) is not a failure: the row is queued again as it
// was before its claim, its attempts not counted and its error cleared (Outcome.Stopped).
//
// On a failure (Task.fail, a not found, a capability's error, a panic, the timeout) the Task's
// transaction is rolled back and a second short transaction records the error and either the
// retry (queued, due Backoff(attempts) from now) or, past k.Retries, the row's death. Params k.Run
// cannot decode make the row dead at once whatever k.Retries says: the same build cannot decode
// them on a retry. Under env.Within (a test) the record is written in Within.Tx instead.
func RunOne(env *eff.Env, k Kind, row Row) Outcome {
	out := Outcome{ID: row.ID, Kind: row.Kind}
	store, base := env.Store, ctxOf(env)
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
	err, decode := attempt(env, k, row)
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
		return out
	}
	switch {
	case decode:
		err = &failure{"cannot decode the parameters: " + err.Error(), err}
	case stopped:
		out.Err, out.Stopped = err, true
		if rerr := record(env, store, base, `UPDATE "_alder_jobs" SET "state" = 'queued', "attempts" = "attempts" - 1, "error" = NULL, "lease_until" = NULL WHERE "id" = ?`, row.ID); rerr != nil {
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
		rerr = record(env, store, base, `UPDATE "_alder_jobs" SET "state" = 'dead', "error" = ?, "finished" = ?, "lease_until" = NULL WHERE "id" = ?`, msg, now.UnixMilli(), row.ID)
	} else {
		out.Next = now.Add(Backoff(row.Attempts))
		rerr = record(env, store, base, `UPDATE "_alder_jobs" SET "state" = 'queued', "run_at" = ?, "error" = ?, "lease_until" = NULL WHERE "id" = ?`, out.Next.UnixMilli(), msg, row.ID)
	}
	if rerr != nil {
		out.Err = errors.Join(err, fmt.Errorf("jobs: recording the failure: %w", rerr))
	}
	return out
}

// attempt decodes row's params and runs its Task followed by the done mark. decode is set when
// k.Run could not decode them. A panic is the attempt's error (its first line), logged with its
// stack at level "error"; eff.Run has rolled the transaction back by then.
func attempt(env *eff.Env, k Kind, row Row) (err error, decode bool) {
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
		return err, true
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
	return err, false
}

// record runs one statement on the jobs table of store in a transaction of its own (in the
// test's transaction under env.Within), on ctx without its cancellation: a failure is recorded
// even when the attempt was cancelled.
func record(env *eff.Env, store eff.Store, ctx context.Context, stmt string, args ...any) error {
	ctx = context.WithoutCancel(ctx)
	if w := env.Within; w != nil && w.Tx != nil {
		q, err := db.On(store, w.Tx)
		if err != nil {
			return err
		}
		_, err = q.ExecContext(ctx, stmt, args...)
		return err
	}
	if store == nil {
		return errors.New("jobs: no store")
	}
	tx, err := store.Begin(ctx)
	if err != nil {
		return err
	}
	q, err := db.On(store, tx)
	if err == nil {
		_, err = q.ExecContext(ctx, stmt, args...)
	}
	if err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
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
