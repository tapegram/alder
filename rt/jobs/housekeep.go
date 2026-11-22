package jobs

import (
	"context"
	"fmt"
	"sync"
	"time"

	"alder/rt/db"
	"alder/rt/eff"
)

const (
	// LostError is the error of a row whose lease ran out while it was running: the process
	// that claimed it stopped without recording how the attempt ended.
	LostError = "lost: the process stopped"
	// KeepDone and KeepDead are how long a done and a dead row are kept before housekeeping
	// deletes them.
	KeepDone = 7 * 24 * time.Hour
	KeepDead = 30 * 24 * time.Hour
	// Unclaimable is how long a queued row of a kind this build does not know waits before
	// housekeeping warns about its kind, and how long past its scheduled_for a queued tick of
	// such a kind is when housekeeping deletes it.
	Unclaimable = 24 * time.Hour
)

// housekeeper keeps a store's jobs table for one Start's workers, one of them at a time.
type housekeeper struct {
	store  eff.Store
	all    []Kind // the kinds, in the order given
	kinds  map[string]Kind
	log    func(level, msg string)
	mu     sync.Mutex
	warned map[string]bool // the unknown kinds already warned about
	// silos, when set, keeps the silos' tables too: it runs keep, their expired leases and their
	// pruning, on each silo of the active set (pool.keepSilos). Ticks are only ever in the
	// shared store.
	silos func(ctx context.Context, now time.Time, keep func(db.Execer) error)
}

func newHousekeeper(s eff.Store, kinds []Kind, log func(level, msg string)) *housekeeper {
	h := &housekeeper{store: s, all: kinds, kinds: map[string]Kind{}, log: log, warned: map[string]bool{}}
	for _, k := range kinds {
		h.kinds[k.Name] = k
	}
	return h
}

// run keeps the table as of now, unless another worker is keeping it already:
//   - a running row past its lease is re-queued with LostError (due after the backoff, its
//     attempt counted), or made dead when that attempt was its last;
//   - done rows finished more than KeepDone ago and dead ones more than KeepDead ago are deleted;
//   - a queued tick of a kind this build does not know (its kind's queued tick, or a tick
//     queued for its retry: scheduled_for is set), due (scheduled_for) more than Unclaimable
//     ago, is deleted with a warn line (a job renamed or removed: the new name has its own
//     tick; a tick not yet due, such as a new weekly kind's after a rollback, is kept);
//   - a kind this build does not know, with a row queued and due for more than Unclaimable, is
//     a warn line, once per kind;
//   - each scheduled kind without a queued tick gets one (EnsureTicks).
//
// Then, in each silo h.silos keeps, the expired leases and the pruning; nothing about ticks or
// unknown kinds, which concern the shared store's ticks and rows alone. An error is a warn line;
// the next run tries again.
func (h *housekeeper) run(ctx context.Context, now time.Time) {
	if !h.mu.TryLock() {
		return
	}
	defer h.mu.Unlock()
	q, err := db.On(h.store, nil)
	if err == nil {
		err = h.leases(ctx, q, now)
	}
	if err == nil {
		err = prune(ctx, q, now)
	}
	if err == nil {
		err = h.unknownTicks(ctx, q, now)
	}
	if err == nil {
		err = h.unclaimable(ctx, q, now)
	}
	if err == nil {
		err = EnsureTicksOn(ctx, q, h.all, now)
	}
	if err != nil && ctx.Err() == nil {
		h.log("warn", "jobs: housekeeping: "+err.Error())
	}
	if h.silos == nil {
		return
	}
	h.silos(ctx, now, func(q db.Execer) error {
		if err := h.leases(ctx, q, now); err != nil {
			return err
		}
		return prune(ctx, q, now)
	})
}

// prune deletes the done rows finished more than KeepDone before now and the dead ones more
// than KeepDead before.
func prune(ctx context.Context, q db.Execer, now time.Time) error {
	_, err := q.ExecContext(ctx, `DELETE FROM "_alder_jobs" WHERE ("state" = 'done' AND COALESCE("finished", "created") < ?) OR ("state" = 'dead' AND COALESCE("finished", "created") < ?)`,
		now.Add(-KeepDone).UnixMilli(), now.Add(-KeepDead).UnixMilli())
	return err
}

// leases re-queues (or makes dead) every running row whose lease ended before now.
func (h *housekeeper) leases(ctx context.Context, q db.Execer, now time.Time) error {
	rows, err := q.QueryContext(ctx, `SELECT "id", "kind", "attempts" FROM "_alder_jobs" WHERE "state" = 'running' AND "lease_until" < ?`, now.UnixMilli())
	if err != nil {
		return err
	}
	type lost struct {
		id, kind string
		attempts int
	}
	var all []lost
	for rows.Next() {
		var l lost
		if err := rows.Scan(&l.id, &l.kind, &l.attempts); err != nil {
			rows.Close()
			return err
		}
		all = append(all, l)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, l := range all {
		// a row of a kind this build does not know is re-queued for a build that does
		k, known := h.kinds[l.kind]
		// the lease is checked again: another instance may have kept this row in the meantime
		if known && l.attempts > k.Retries {
			_, err = q.ExecContext(ctx, `UPDATE "_alder_jobs" SET "state" = 'dead', "error" = ?, "finished" = ?, "lease_until" = NULL WHERE "id" = ? AND "state" = 'running' AND "lease_until" < ?`,
				LostError, now.UnixMilli(), l.id, now.UnixMilli())
		} else {
			_, err = q.ExecContext(ctx, `UPDATE "_alder_jobs" SET "state" = 'queued', "error" = ?, "run_at" = ?, "lease_until" = NULL WHERE "id" = ? AND "state" = 'running' AND "lease_until" < ?`,
				LostError, now.Add(Backoff(l.attempts)).UnixMilli(), l.id, now.UnixMilli())
		}
		if err != nil {
			return err
		}
		h.log("warn", fmt.Sprintf("job %s %s: %s", l.kind, l.id, LostError))
	}
	return nil
}

// unknownTicks deletes the queued ticks of kinds this build does not know whose time
// (scheduled_for) is more than Unclaimable before now: rows with scheduled_for, whether still
// their kind's queued tick (schedule 1) or queued for a retry after a claim cleared it. The age
// is the tick's due time, not its created (ruling R3): a rollback to a build that lacks a new
// weekly kind keeps that kind's tick, made long ago and not yet due, for the build that knows it.
func (h *housekeeper) unknownTicks(ctx context.Context, q db.Execer, now time.Time) error {
	before := now.Add(-Unclaimable).UnixMilli()
	rows, err := q.QueryContext(ctx, `SELECT "id", "kind" FROM "_alder_jobs" WHERE "scheduled_for" IS NOT NULL AND "state" = 'queued' AND "scheduled_for" < ? ORDER BY "kind"`, before)
	if err != nil {
		return err
	}
	var gone [][2]string
	for rows.Next() {
		var id, kind string
		if err := rows.Scan(&id, &kind); err != nil {
			rows.Close()
			return err
		}
		if _, known := h.kinds[kind]; !known {
			gone = append(gone, [2]string{id, kind})
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, g := range gone {
		// still queued: a build that knows the kind may have claimed it in the meantime
		res, err := q.ExecContext(ctx, `DELETE FROM "_alder_jobs" WHERE "id" = ? AND "scheduled_for" IS NOT NULL AND "state" = 'queued'`, g[0])
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 1 {
			h.log("warn", fmt.Sprintf("jobs: deleted the queued tick %s of kind %s (a schedule tick, or one queued for its retry): this build does not know the kind and the tick was due more than a day ago (a job renamed or removed)", g[0], g[1]))
		}
	}
	return nil
}

// unclaimable warns, once per kind, about the kinds this build does not know with a row queued
// and due for more than Unclaimable.
func (h *housekeeper) unclaimable(ctx context.Context, q db.Execer, now time.Time) error {
	rows, err := q.QueryContext(ctx, `SELECT "kind", COUNT(*) FROM "_alder_jobs" WHERE "state" = 'queued' AND "run_at" <= ? GROUP BY "kind" ORDER BY "kind"`, now.Add(-Unclaimable).UnixMilli())
	if err != nil {
		return err
	}
	defer rows.Close()
	var lines []string
	for rows.Next() {
		var (
			kind string
			n    int
		)
		if err := rows.Scan(&kind, &n); err != nil {
			return err
		}
		if _, known := h.kinds[kind]; known || h.warned[kind] {
			continue
		}
		h.warned[kind] = true
		lines = append(lines, fmt.Sprintf("jobs: %d job(s) of kind %s have waited more than a day: no running build knows the kind (a job of an older build, or one renamed)", n, kind))
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, l := range lines {
		h.log("warn", l)
	}
	return nil
}
