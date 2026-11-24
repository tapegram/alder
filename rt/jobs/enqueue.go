package jobs

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"alder/rt"
	"alder/rt/db"
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
// Task's transaction commits, a new row in a silo puts the silo in the active set of this
// process's workers (spec §7), and a new row wakes them (Wake). A process with no workers on its
// stores (ALDER_JOBS=0: the web service of a `workers:` deploy) has no active set: a new row in
// a silo instead upserts the silo's hint in the shared store (db.SiloHintsTable, touched now),
// at once, outside the Task's transaction and so before it commits, for the --jobs-only workers
// to read (Options.Hints). A hint that cannot be written fails the Task, so a committed row in a
// silo always has one; a Task that fails after it leaves a hint with no row, which costs a worker
// one look in that silo. A test's stores write no hint.
func Enqueue(env *eff.Env, j rt.Job, runAt time.Time, key *string) (string, error) {
	if len(j.Params) > MaxParams {
		return "", &eff.ErrFail{Msg: fmt.Sprintf("Job.enqueue %s: its parameters take %d bytes; a job's parameters are at most 64 KB (%d bytes)", j.Kind, len(j.Params), MaxParams)}
	}
	at, err := db.RuntimeOn(env)
	if err != nil {
		return "", err
	}
	q, err := db.On(at.Store, at.Tx)
	if err != nil {
		return "", err
	}
	ctx := ctxOf(env)
	var id string
	for range idTries {
		id = newID()
		res, err := q.ExecContext(ctx, `INSERT INTO "_alder_jobs" ("id", "kind", "params", "key", "state", "run_at", "created", "priority") VALUES (?, ?, ?, ?, 'queued', ?, ?, ?) ON CONFLICT DO NOTHING`,
			id, j.Kind, j.Params, key, runAt.UnixMilli(), env.Now().UnixMilli(), j.Priority)
		if err != nil {
			return "", err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return "", err
		}
		if n == 1 {
			reg, _ := env.Stores.(*db.Stores)
			if at.Key != "" && reg != nil && !reg.Testing() && activeOf(reg) == nil {
				if err := writeHint(ctx, reg, at.Key, at.Value, env.Now()); err != nil {
					return "", err
				}
			}
			env.AfterCommit(func() {
				if at.Key != "" && reg != nil {
					touch(reg, at.Key, at.Value)
				}
				Wake()
			})
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

// writeHint upserts the hint of silo key/value in the shared store of s, touched at now: an
// autocommit statement, apart from the silo's transaction (as keyExists reads the shared store
// from a Task bound to a silo).
func writeHint(ctx context.Context, s *db.Stores, key, value string, now time.Time) error {
	q, err := db.On(s.Shared(), nil)
	if err != nil {
		return err
	}
	return Hint(ctx, q, key, value, now)
}

// Hint upserts the hint of silo key/value in the shared store's _alder_jobs_silos through
// shared, touched at now.
func Hint(ctx context.Context, shared db.Execer, key, value string, now time.Time) error {
	_, err := shared.ExecContext(ctx, `INSERT INTO "_alder_jobs_silos" ("key", "value", "touched") VALUES (?, ?, ?) ON CONFLICT ("key", "value") DO UPDATE SET "touched" = excluded."touched"`,
		key, value, now.UnixMilli())
	return err
}

// Status is what Job.status gives: the row's state, its attempts so far, its last failure, and,
// while it is queued, when it is due.
type Status struct {
	State    State
	Attempts int
	Error    *string
	RunAt    *time.Time
}

// StatusOf is the status of job id, nil when there is no such row (never enqueued where it
// looks, or pruned). It looks in the store the effect running on env is bound to and, in an app
// with silos, in the shared store after a silo, or first in the silo of the org a page is
// narrowed to (db.RuntimeReads): a row is where the Task that enqueued it was bound.
func StatusOf(env *eff.Env, id string) (*Status, error) {
	places, err := db.RuntimeReads(env)
	if err != nil {
		return nil, err
	}
	for _, p := range places {
		q, err := db.On(p.Store, p.Tx)
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
			continue
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
	return nil, nil
}
