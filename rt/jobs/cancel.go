package jobs

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"alder/rt/db"
	"alder/rt/eff"
)

// errCancelled is the cause of a cut-off attempt's context and the done mark's refusal of a
// cancelled row (spec §3): its text is Test.work's `Err "cancelled"`.
var errCancelled = errors.New("cancelled")

// cancelStmt cancels a queued or running row: not a schedule's queued tick (a claimed tick has
// schedule 0). Its attempts, error, started and worker are kept.
const cancelStmt = `UPDATE "_alder_jobs" SET "state" = 'cancelled', "finished" = ?, "lease_until" = NULL WHERE "id" = ? AND "state" IN ('queued', 'running') AND "schedule" = 0`

// Cancel is Job.cancel: it cancels job id where Job.status finds it (db.RuntimeReads), in the
// first place that has the row. In the bound store that is the Task's transaction, so a Task that
// fails afterwards un-cancels; in the shared store reached from a silo-bound Task it is a
// transaction of its own, committed at once (decision 10). It is true when the row was queued or
// running and is now cancelled; false when there is no such row, it is final, or it is a
// schedule's queued tick. A running row's attempt in this process is cut off (cutOff) once the
// cancel is committed; a queued row's cancel is an info line.
func Cancel(env *eff.Env, id string) (bool, error) {
	places, err := db.RuntimeReads(env)
	if err != nil {
		return false, err
	}
	ctx := ctxOf(env)
	for _, p := range places {
		kind, was, err := cancelAt(ctx, p, id, env.Now().UnixMilli())
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil || was == "" {
			return false, err
		}
		after := func(f func()) { f() }
		if p.Tx != nil {
			after = env.AfterCommit
		}
		switch was {
		case Running:
			after(func() { cutOff(id) })
		case Queued:
			if env.Log != nil {
				after(func() { env.Log("info", fmt.Sprintf("job %s %s cancelled (was queued)", kind, id)) })
			}
		}
		return true, nil
	}
	return false, nil
}

// cancelAt cancels row id in p at now (cancelStmt): kind is the row's, was its state before the
// cancel, "" when it was not cancelled (final, or a queued tick); sql.ErrNoRows when p has no such
// row. The read and the update are in one transaction — p's, else one of its own committed
// before cancelAt returns — so was is the state the update changed: a worker's claim cannot come
// between them.
func cancelAt(ctx context.Context, p db.Place, id string, now int64) (kind string, was State, err error) {
	tx := p.Tx
	if tx == nil {
		if tx, err = p.Store.Begin(ctx); err != nil {
			return "", "", err
		}
		defer func() {
			if err != nil {
				tx.Rollback()
				return
			}
			if err = tx.Commit(); err != nil {
				was = ""
			}
		}()
	}
	q, err := db.On(p.Store, tx)
	if err != nil {
		return "", "", err
	}
	var state string
	if err := q.QueryRowContext(ctx, `SELECT "kind", "state" FROM "_alder_jobs" WHERE "id" = ?`, id).Scan(&kind, &state); err != nil {
		return "", "", err
	}
	res, err := q.ExecContext(ctx, cancelStmt, now, id)
	if err != nil {
		return "", "", err
	}
	if n, err := res.RowsAffected(); err != nil || n != 1 {
		return kind, "", err
	}
	return kind, State(state), nil
}

// running holds the cancel of each attempt running in this process, by its job id (pool.run
// stores it for the attempt's duration): cutOff calls it.
var running sync.Map // job id → context.CancelCauseFunc

// cutOff cancels the attempt of job id running in this process with errCancelled: its next
// effect step sees the context ended, eff.Run rolls its writes back, and RunOne makes it
// Outcome.Cancelled. It is false when none runs here.
func cutOff(id string) bool {
	c, ok := running.Load(id)
	if ok {
		c.(context.CancelCauseFunc)(errCancelled)
	}
	return ok
}

// watch is the pool's watcher (spec §3, decision 12): on each tick, while attempts run, it reads
// which of their rows are cancelled — once per store that has one: the shared store, a silo
// through at (a closed silo is not opened) — and cuts those attempts off (cutOff). A read that
// fails is a warn line, rate-limited as a claim's (claimed). It returns when ctx ends.
func (p *pool) watch(ctx context.Context, tick <-chan time.Time) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick:
		}
		ids := map[siloID][]any{} // the shared store is siloID{}
		p.attempts.Range(func(k, v any) bool {
			pl := v.(db.Place)
			s := siloID{pl.Key, pl.Value}
			ids[s] = append(ids[s], k)
			return true
		})
		for s, in := range ids {
			if ctx.Err() != nil {
				return
			}
			var err error
			name := "the shared store"
			if s == (siloID{}) {
				var q db.Execer
				if q, err = db.On(p.o.Store, nil); err == nil {
					err = cutCancelled(ctx, q, in)
				}
			} else if p.active != nil {
				name = s.String()
				_, err = p.at(ctx, s, func(pl db.Place) error {
					q, err := db.On(pl.Store, pl.Tx)
					if err != nil {
						return err
					}
					return cutCancelled(ctx, q, in)
				})
			}
			if err != nil && ctx.Err() == nil {
				p.claimed(name, fmt.Errorf("reading the cancelled jobs: %w", err), p.o.Now())
			}
		}
	}
}

// cutCancelled cuts off the attempts of the jobs of ids whose rows in q are cancelled.
func cutCancelled(ctx context.Context, q db.Execer, ids []any) error {
	rows, err := q.QueryContext(ctx, `SELECT "id" FROM "_alder_jobs" WHERE "id" IN (`+strings.TrimSuffix(strings.Repeat("?, ", len(ids)), ", ")+`) AND "state" = 'cancelled'`, ids...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return err
		}
		cutOff(id)
	}
	return rows.Err()
}
