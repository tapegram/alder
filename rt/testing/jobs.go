package rtesting

// Test.jobs and Test.work (#168): a test runs no worker. Job.enqueue in a test writes its row in
// the test's transaction like any write, so Test.jobs lists it and it is gone with the case;
// Test.work runs the due rows itself, one after another on the test's goroutine, with
// jobs.RunOne, the core the worker runs too.

import (
	"cmp"
	"database/sql"
	"errors"
	"slices"
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

// Jobs is Test.jobs: every row of the test's jobs tables, oldest first (created), with its
// status (runAt only while it is queued): the shared store's and, in an app with silos, every
// test silo's (spec §8), each read in its own transaction for the test.
func Jobs() eff.Task[[]JobRow] {
	return eff.Call("Test.jobs", false, func(e *eff.Env) ([]JobRow, error) {
		ps, err := places(e)
		if err != nil {
			return nil, err
		}
		type listed struct {
			r       JobRow
			created int64
			at      int // the place's index, then the row's rowid: the order of a tie
			rowid   int64
		}
		var all []listed
		for i, p := range ps {
			q, err := db.On(p.Store, p.Tx)
			if err != nil {
				return nil, err
			}
			rows, err := q.QueryContext(e.Ctx, `SELECT "id", "kind", "params", "state", "attempts", "error", "run_at", "created", "rowid" FROM "_alder_jobs"`)
			if err != nil {
				return nil, err
			}
			for rows.Next() {
				var (
					l      = listed{at: i}
					state  string
					s      jobs.Status
					errTxt sql.NullString
					runAt  int64
				)
				if err := rows.Scan(&l.r.Id, &l.r.Job.Kind, &l.r.Job.Params, &state, &s.Attempts, &errTxt, &runAt, &l.created, &l.rowid); err != nil {
					rows.Close()
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
				if l.r.Status, err = std.JobStatusOf(s); err != nil {
					rows.Close()
					return nil, err
				}
				all = append(all, l)
			}
			rows.Close()
			if err := rows.Err(); err != nil {
				return nil, err
			}
		}
		slices.SortStableFunc(all, func(a, b listed) int {
			return cmp.Or(cmp.Compare(a.created, b.created), cmp.Compare(a.at, b.at), cmp.Compare(a.rowid, b.rowid))
		})
		out := []JobRow{}
		for _, l := range all {
			out = append(out, l.r)
		}
		return out, nil
	})
}

// Work is Test.work: it runs every job of kinds due at the test clock's time, oldest first
// (run_at, then created) across the test's stores — the shared one and, in an app with silos,
// every test silo (spec §8) — and goes round again while a round ran something, so the jobs those
// jobs enqueued run too. Each is claimed as the worker claims a row (attempts + 1, running) in the
// store it is in and run by jobs.RunOne on an Env that is the test's (its store and stores,
// capabilities with Test.fake's fakes, clock, log and session check, no session) Within the
// test's transaction, so its Task runs under a savepoint: a failure undoes its writes to the
// savepoint (in every store) and records the attempt in the row's store (the error, and the
// retry Backoff on or the row's death). RunOne binds the Task to its store as the worker does: a
// `silo k` job to its parameter's silo, any other to the shared store, so a write of another
// store fails as it fails in production; a row in another store than the run's is marked done
// after the job's savepoint is released, without the crash window. A row of a kind not in kinds
// stays queued. Past MaxWorkRuns runs it fails the test. It gives each run in order with how it
// ended (Err with the row's error text).
//
// Schedule ticks (spec §8): each call first inserts the tick of every scheduled kind without a
// queued one, due after the clock's time (jobs.EnsureTicksOn), as the worker does at its start
// and every housekeeping — in the shared store only, where ticks live. A queued tick always
// exists after that, since claiming one inserts the next in the same transaction
// (jobs.ClaimedTick), so it is the case's first Test.work that creates the ticks, from its clock.
func Work(kinds []jobs.Kind) eff.Task[[]JobRun] {
	return eff.Call("Test.work", true, func(e *eff.Env) ([]JobRun, error) {
		byName := map[string]jobs.Kind{}
		for _, k := range kinds {
			byName[k.Name] = k
		}
		ps, err := places(e)
		if err != nil {
			return nil, err
		}
		q, err := db.On(ps[0].Store, ps[0].Tx)
		if err != nil {
			return nil, err
		}
		if err := jobs.EnsureTicksOn(e.Ctx, q, kinds, e.Now()); err != nil {
			return nil, err
		}
		sp, _ := e.Store.(eff.Savepointer)
		within := &eff.Within{Tx: ps[0].Tx, Savepoints: sp}
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
				if err := claim(e, byName[row.Kind], &row); err != nil {
					return nil, err
				}
				je := &eff.Env{Ctx: e.Ctx, Store: e.Store, Caps: e.Caps, Now: e.Now, Sleep: e.Sleep, Log: e.Log, Within: within,
					Stores: e.Stores, CheckSession: e.CheckSession}
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

// places are where the test's job rows are: its shared store in the test's transaction, first,
// then, in an app with silos, every test silo in its own (db.Stores.OpenSilos).
func places(e *eff.Env) ([]db.Place, error) {
	tx, err := e.Transaction()
	if err != nil {
		return nil, err
	}
	if tx == nil {
		return nil, errors.New("Test.jobs, Test.work: the test has no database")
	}
	ps := []db.Place{{Store: e.Store, Tx: tx}}
	if reg, _ := e.Stores.(*db.Stores); reg != nil {
		silos, err := reg.OpenSilos(e.Ctx)
		if err != nil {
			return nil, err
		}
		ps = append(ps, silos...)
	}
	return ps, nil
}

// dueRows is the queued rows of kinds byName due at the test clock's time in every store of the
// test, oldest first (run_at, created; a tie in the order of the stores, then of the rows), each
// with the store it is in (Row.In).
func dueRows(e *eff.Env, byName map[string]jobs.Kind) ([]jobs.Row, error) {
	ps, err := places(e)
	if err != nil {
		return nil, err
	}
	type found struct {
		r     jobs.Row
		at    int
		rowid int64
	}
	var all []found
	for i, p := range ps {
		q, err := db.On(p.Store, p.Tx)
		if err != nil {
			return nil, err
		}
		rows, err := q.QueryContext(e.Ctx, `SELECT "id", "kind", "params", "attempts", "run_at", "created", "schedule", "scheduled_for", "rowid" FROM "_alder_jobs" WHERE "state" = 'queued' AND "run_at" <= ?`, e.Now().UnixMilli())
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			f := found{r: jobs.Row{State: string(jobs.Queued), In: p}, at: i}
			var schedAt sql.NullInt64
			if err := rows.Scan(&f.r.ID, &f.r.Kind, &f.r.Params, &f.r.Attempts, &f.r.RunAt, &f.r.Created, &f.r.Schedule, &schedAt, &f.rowid); err != nil {
				rows.Close()
				return nil, err
			}
			if schedAt.Valid {
				f.r.ScheduledFor = &schedAt.Int64
			}
			if _, ok := byName[f.r.Kind]; ok {
				all = append(all, f)
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	slices.SortStableFunc(all, func(a, b found) int {
		return cmp.Or(cmp.Compare(a.r.RunAt, b.r.RunAt), cmp.Compare(a.r.Created, b.r.Created), cmp.Compare(a.at, b.at), cmp.Compare(a.rowid, b.rowid))
	})
	var out []jobs.Row
	for _, f := range all {
		out = append(out, f.r)
	}
	return out, nil
}

// claim does to row, of kind k, what the worker's claim does: attempts + 1, running, started
// now, and for a tick the next one (jobs.ClaimedTick), all in the transaction of the row's store.
func claim(e *eff.Env, k jobs.Kind, row *jobs.Row) error {
	q, err := db.On(row.In.Store, row.In.Tx)
	if err != nil {
		return err
	}
	at := e.Now()
	now := at.UnixMilli()
	if _, err := q.ExecContext(e.Ctx, `UPDATE "_alder_jobs" SET "state" = 'running', "attempts" = "attempts" + 1, "started" = ? WHERE "id" = ?`, now, row.ID); err != nil {
		return err
	}
	if err := jobs.ClaimedTick(e.Ctx, q, k, *row, at); err != nil {
		return err
	}
	row.Attempts++
	row.State, row.Started = string(jobs.Running), &now
	return nil
}
