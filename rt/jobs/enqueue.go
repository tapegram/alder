package jobs

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"alder/rt"
	"alder/rt/eff"
)

// MaxParams is the most bytes a job's encoded parameters may take (64 KB).
const MaxParams = 65536

// idTries is how many ids Enqueue draws before it gives up on a row whose id is taken.
const idTries = 3

// NewID is a job id: 8 lowercase hex characters from crypto/rand. Unlike a request id
// (applog.NewID, a per-process seed plus a counter) it does not follow a sequence, so ids drawn
// by two processes on one database collide only by chance, and a fresh draw escapes a collision.
func NewID() string {
	var b [4]byte
	rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// newID is the id source Enqueue draws from (a test replaces it).
var newID = NewID

// Enqueue inserts j as a queued row due at runAt, in the transaction of the Task running on env
// (begun if it is not yet), so the row is committed or rolled back with the Task's writes. It
// returns the row's id (8 hex characters, NewID); an id another row has is drawn again, up to
// idTries times. With a key, the key is j.Kind's: while a row of the kind with that key is
// queued or running, nothing is inserted and that row's id comes back; a done or dead row
// releases the key. Parameters longer than MaxParams fail the Task (*eff.ErrFail). Once the
// Task's transaction commits, a new row wakes this process's workers (Wake).
func Enqueue(env *eff.Env, j rt.Job, runAt time.Time, key *string) (string, error) {
	if len(j.Params) > MaxParams {
		return "", &eff.ErrFail{Msg: fmt.Sprintf("Job.enqueue %s: its parameters take %d bytes; a job's parameters are at most 64 KB (%d bytes)", j.Kind, len(j.Params), MaxParams)}
	}
	q, err := on(env)
	if err != nil {
		return "", err
	}
	ctx := ctxOf(env)
	var id string
	for range idTries {
		id = newID()
		res, err := q.ExecContext(ctx, `INSERT INTO "_alder_jobs" ("id", "kind", "params", "key", "state", "run_at", "created") VALUES (?, ?, ?, ?, 'queued', ?, ?) ON CONFLICT DO NOTHING`,
			id, j.Kind, j.Params, key, runAt.UnixMilli(), env.Now().UnixMilli())
		if err != nil {
			return "", err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return "", err
		}
		if n == 1 {
			env.AfterCommit(Wake)
			return id, nil
		}
		if key == nil {
			continue
		}
		var old string
		err = q.QueryRowContext(ctx, `SELECT "id" FROM "_alder_jobs" WHERE "kind" = ? AND "key" = ? AND "state" IN ('queued', 'running')`, j.Kind, *key).Scan(&old)
		if !errors.Is(err, sql.ErrNoRows) {
			return old, err
		}
	}
	return "", fmt.Errorf("jobs: job id %s is taken (%d ids drawn)", id, idTries)
}

// Status is what Job.status gives: the row's state, its attempts so far, its last failure, and,
// while it is queued, when it is due.
type Status struct {
	State    State
	Attempts int
	Error    *string
	RunAt    *time.Time
}

// StatusOf is the status of job id in the store the effect running on env is bound to, nil when
// there is no such row (never enqueued there, or pruned).
func StatusOf(env *eff.Env, id string) (*Status, error) {
	q, err := on(env)
	if err != nil {
		return nil, err
	}
	var (
		state  string
		s      Status
		runAt  int64
		errTxt sql.NullString
	)
	err = q.QueryRowContext(ctxOf(env), `SELECT "state", "attempts", "error", "run_at" FROM "_alder_jobs" WHERE "id" = ?`, id).Scan(&state, &s.Attempts, &errTxt, &runAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	s.State = State(state)
	if errTxt.Valid {
		s.Error = &errTxt.String
	}
	if s.State == Queued {
		t := time.UnixMilli(runAt).UTC()
		s.RunAt = &t
	}
	return &s, nil
}
