package rtesting

// Test.jobs and Test.work (#168): a test runs no worker. Job.enqueue in a test writes its row in
// the test's transaction like any write, so Test.jobs lists it and it is gone with the case;
// Test.work runs the due rows itself, one after another on the test's goroutine, with
// jobs.RunOne, the core the worker runs too.

import (
	"database/sql"
	"errors"
	"time"

	"alder/rt"
	"alder/rt/db"
	"alder/rt/eff"
	"alder/rt/jobs"
	"alder/rt/std"
)

// JobRow is an element of Test.jobs: { id: Job.Id, job: Job, status: Job.Status }, the Go type
// gen gives that record (an alias, so the generated code's anonymous struct is this type).
type JobRow = struct {
	Id     string
	Job    rt.Job
	Status std.JobStatusRecord
}

// JobRun is an element of Test.work: { id: Job.Id, job: Job, result: Result Text Unit }.
type JobRun = struct {
	Id     string
	Job    rt.Job
	Result rt.Result[string, struct{}]
}

// MaxWorkRuns is the most attempts one Test.work runs: past it, jobs keep enqueueing jobs and
// the test fails.
const MaxWorkRuns = 1000

// errKeepEnqueueing is Test.work's failure past MaxWorkRuns. It is not a Task failure, so a
// Test.run around Test.work does not turn it into an Err.
var errKeepEnqueueing = errors.New("Test.work: jobs keep enqueueing jobs")

// Jobs is Test.jobs: every row of the test's jobs table, oldest first, with its status (runAt
// only while it is queued). kinds is the app's Jobs(); a row of any kind is listed.
func Jobs(kinds []jobs.Kind) eff.Task[[]JobRow] {
	return eff.Call("Test.jobs", false, func(e *eff.Env) ([]JobRow, error) {
		q, err := jobsOn(e)
		if err != nil {
			return nil, err
		}
		rows, err := q.QueryContext(e.Ctx, `SELECT "id", "kind", "params", "state", "attempts", "error", "run_at" FROM "_alder_jobs" ORDER BY "created", "rowid"`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		out := []JobRow{}
		for rows.Next() {
			var (
				r      JobRow
				state  string
				s      jobs.Status
				errTxt sql.NullString
				runAt  int64
			)
			if err := rows.Scan(&r.Id, &r.Job.Kind, &r.Job.Params, &state, &s.Attempts, &errTxt, &runAt); err != nil {
				return nil, err
			}
			s.State = jobs.State(state)
			if errTxt.Valid {
				s.Error = &errTxt.String
			}
			if s.State == jobs.Queued {
				t := time.UnixMilli(runAt).UTC()
				s.RunAt = &t
			}
			r.Status = std.JobStatusOf(s)
			out = append(out, r)
		}
		return out, rows.Err()
	})
}

// Work is Test.work: it runs every job of kinds due at the test clock's time, oldest first
// (run_at, then created), and goes round again while a round ran something, so the jobs those
// jobs enqueued run too. Each is claimed as the worker claims a row (attempts + 1, running) and
// run by jobs.RunOne on an Env that is the test's (its store, capabilities with Test.fake's
// fakes, clock and log, no session) Within the test's transaction, so its Task runs under a
// savepoint: a failure undoes its writes to the savepoint and records the attempt in the test's
// transaction (the error, and the retry Backoff on or the row's death). A row of a kind not in
// kinds stays queued. Past MaxWorkRuns runs it fails the test. It gives each run in order with
// how it ended (Err with the row's error text).
func Work(kinds []jobs.Kind) eff.Task[[]JobRun] {
	return eff.Call("Test.work", true, func(e *eff.Env) ([]JobRun, error) {
		byName := map[string]jobs.Kind{}
		for _, k := range kinds {
			byName[k.Name] = k
		}
		tx, err := e.Transaction()
		if err != nil {
			return nil, err
		}
		if tx == nil {
			return nil, errors.New("Test.work: the test has no database")
		}
		sp, _ := e.Store.(eff.Savepointer)
		within := &eff.Within{Tx: tx, Savepoints: sp}
		out := []JobRun{}
		for {
			due, err := dueRows(e, byName)
			if err != nil {
				return nil, err
			}
			if len(due) == 0 {
				return out, nil
			}
			for _, row := range due {
				if len(out) == MaxWorkRuns {
					return nil, errKeepEnqueueing
				}
				if err := claim(e, &row); err != nil {
					return nil, err
				}
				je := &eff.Env{Ctx: e.Ctx, Store: e.Store, Caps: e.Caps, Now: e.Now, Sleep: e.Sleep, Log: e.Log, Within: within, Stores: e.Stores}
				o := jobs.RunOne(je, byName[row.Kind], row)
				run := JobRun{Id: row.ID, Job: rt.Job{Kind: row.Kind, Params: row.Params}, Result: rt.Ok[string](struct{}{})}
				if o.Err != nil {
					run.Result = rt.Err[string, struct{}](o.Err.Error())
				}
				out = append(out, run)
			}
		}
	})
}

// dueRows is the queued rows of kinds byName due at the test clock's time, oldest first.
func dueRows(e *eff.Env, byName map[string]jobs.Kind) ([]jobs.Row, error) {
	q, err := jobsOn(e)
	if err != nil {
		return nil, err
	}
	rows, err := q.QueryContext(e.Ctx, `SELECT "id", "kind", "params", "attempts", "run_at", "created" FROM "_alder_jobs" WHERE "state" = 'queued' AND "run_at" <= ? ORDER BY "run_at", "created", "rowid"`, e.Now().UnixMilli())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []jobs.Row
	for rows.Next() {
		r := jobs.Row{State: string(jobs.Queued)}
		if err := rows.Scan(&r.ID, &r.Kind, &r.Params, &r.Attempts, &r.RunAt, &r.Created); err != nil {
			return nil, err
		}
		if _, ok := byName[r.Kind]; ok {
			out = append(out, r)
		}
	}
	return out, rows.Err()
}

// claim does to row what the worker's claim does: attempts + 1, running, started now.
func claim(e *eff.Env, row *jobs.Row) error {
	q, err := jobsOn(e)
	if err != nil {
		return err
	}
	now := e.Now().UnixMilli()
	if _, err := q.ExecContext(e.Ctx, `UPDATE "_alder_jobs" SET "state" = 'running', "attempts" = "attempts" + 1, "started" = ? WHERE "id" = ?`, now, row.ID); err != nil {
		return err
	}
	row.Attempts++
	row.State, row.Started = string(jobs.Running), &now
	return nil
}

// jobsOn is where the running test reads and writes its jobs table: its transaction.
func jobsOn(e *eff.Env) (db.Execer, error) {
	tx, err := e.Transaction()
	if err != nil {
		return nil, err
	}
	return db.On(e.Store, tx)
}
