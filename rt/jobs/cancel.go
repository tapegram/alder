package jobs

import (
	"database/sql"
	"errors"
	"fmt"

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
// statement of its own, at once (decision 10). It is true when the row was queued or running and
// is now cancelled; false when there is no such row, it is final, or it is a schedule's queued
// tick. A running row's attempt in this process is cut off (cutOff) once the cancel is committed;
// a queued row's cancel is an info line.
func Cancel(env *eff.Env, id string) (bool, error) {
	places, err := db.RuntimeReads(env)
	if err != nil {
		return false, err
	}
	ctx := ctxOf(env)
	for _, p := range places {
		q, err := db.On(p.Store, p.Tx)
		if err != nil {
			return false, err
		}
		var kind, state string
		err = q.QueryRowContext(ctx, `SELECT "kind", "state" FROM "_alder_jobs" WHERE "id" = ?`, id).Scan(&kind, &state)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return false, err
		}
		res, err := q.ExecContext(ctx, cancelStmt, env.Now().UnixMilli(), id)
		if err != nil {
			return false, err
		}
		n, err := res.RowsAffected()
		if err != nil || n != 1 {
			return false, err
		}
		after := func(f func()) { f() }
		if p.Tx != nil {
			after = env.AfterCommit
		}
		switch State(state) {
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

// cutOff cancels the attempt of job id running in this process with errCancelled; it is false
// when none runs here. Until the registry of running attempts lands it is always false.
func cutOff(id string) bool {
	return false
}
