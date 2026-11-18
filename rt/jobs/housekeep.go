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
	// housekeeping warns about its kind.
	Unclaimable = 24 * time.Hour
)

// housekeeper keeps a store's jobs table for one Start's workers, one of them at a time.
type housekeeper struct {
	store  eff.Store
	kinds  map[string]Kind
	log    func(level, msg string)
	mu     sync.Mutex
	warned map[string]bool // the unknown kinds already warned about
}

func newHousekeeper(s eff.Store, kinds []Kind, log func(level, msg string)) *housekeeper {
	h := &housekeeper{store: s, kinds: map[string]Kind{}, log: log, warned: map[string]bool{}}
	for _, k := range kinds {
		h.kinds[k.Name] = k
	}
	return h
}

// run keeps the table as of now, unless another worker is keeping it already:
//   - a running row past its lease is re-queued with LostError (due after the backoff, its
//     attempt counted), or made dead when that attempt was its last;
//   - done rows finished more than KeepDone ago and dead ones more than KeepDead ago are deleted;
//   - a kind this build does not know, with a row queued and due for more than Unclaimable, is
//     a warn line, once per kind.
//
// An error is a warn line; the next run tries again.
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
		_, err = q.ExecContext(ctx, `DELETE FROM "_alder_jobs" WHERE ("state" = 'done' AND COALESCE("finished", "created") < ?) OR ("state" = 'dead' AND COALESCE("finished", "created") < ?)`,
			now.Add(-KeepDone).UnixMilli(), now.Add(-KeepDead).UnixMilli())
	}
	if err == nil {
		err = h.unclaimable(ctx, q, now)
	}
	if err != nil && ctx.Err() == nil {
		h.log("warn", "jobs: housekeeping: "+err.Error())
	}
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
