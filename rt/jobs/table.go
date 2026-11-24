// Package jobs is the runtime of background jobs (#168): the jobs table each store holds, the
// enqueue a Task makes inside its own transaction, a job's status, and RunOne, which runs one
// attempt of a claimed row and records how it ended. The worker loop and Test.work are its two
// callers.
//
// Every time this package reads is the Env's clock (env.Now), never the wall clock, so it is
// tested with a test Clock. Times in the table are milliseconds since the epoch.
package jobs

import (
	"context"

	"alder/rt/db"
	"alder/rt/eff"
)

// Table is the jobs table of a store: one row per job, created with the runtime's other
// `_alder_` tables (db.EnsureJobsTable).
const Table = db.JobsTable

// EnsureTable creates the jobs table and its indexes when they do not exist and records
// jobs_schema = 2 in _alder_meta. It is idempotent; db.Ensure and db.Migrate run it.
func EnsureTable(ctx context.Context, x db.Execer) error { return db.EnsureJobsTable(ctx, x) }

// Row is a row of the jobs table. Times are milliseconds since the epoch; a nil pointer is NULL.
type Row struct {
	ID           string  `json:"id"`
	Kind         string  `json:"kind"`
	Params       string  `json:"-"` // never printed: it may hold personal data
	State        string  `json:"state"`
	Worker       string  `json:"worker"`
	Key          *string `json:"key"`
	RunAt        int64   `json:"run_at"`
	Created      int64   `json:"created"`
	Started      *int64  `json:"started"`
	Finished     *int64  `json:"finished"`
	LeaseUntil   *int64  `json:"lease_until"`
	ScheduledFor *int64  `json:"scheduled_for"`
	Attempts     int     `json:"attempts"`
	Priority     int     `json:"priority"` // higher is claimed first (claimStmt)
	Error        *string `json:"error"`
	// Schedule: the row is its kind's queued schedule tick, or was until this claim (ClaimedTick
	// clears it in the table; a tick keeps its ScheduledFor).
	Schedule bool `json:"schedule"`
	// In is where the row is, which is not a column: the store its claimer found it in and, in
	// a test, the transaction its statements run in. Zero: the store of the Env RunOne is given
	// (its Within's transaction in a test).
	In db.Place `json:"-"`
}

// State is a job row's state.
type State string

const (
	Queued  State = "queued"  // waiting for its run_at and a free worker
	Running State = "running" // claimed by a worker
	Done    State = "done"    // its attempt committed
	Dead    State = "dead"    // failed past its retries, or its parameters cannot be decoded
	// Cancelled was taken back by Job.cancel, the Jobs page or `alder jobs cancel`.
	Cancelled State = "cancelled"
)

// on is where the running effect of env writes the jobs table: its Task's transaction (begun
// on first use), or the database itself in a Query outside any transaction. That is the store the
// Task is bound to (db.RuntimeOn): a silo's once db.Enter bound it to one, in a test too.
func on(env *eff.Env) (db.Execer, error) {
	p, err := db.RuntimeOn(env)
	if err != nil {
		return nil, err
	}
	return db.On(p.Store, p.Tx)
}

// ctxOf is env.Ctx, or Background when it is nil.
func ctxOf(env *eff.Env) context.Context {
	if env.Ctx == nil {
		return context.Background()
	}
	return env.Ctx
}
