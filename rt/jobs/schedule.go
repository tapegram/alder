package jobs

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	"alder/rt/db"
	"alder/rt/eff"
	"alder/rt/jobs/cron"
)

// Schedules (spec §4). A schedule tick is a row of its kind with schedule = 1 and
// scheduled_for (= its run_at), params {}: claimed, retried, logged and shown like any job. At
// most one tick of a kind is queued (the _alder_jobs_tick index): EnsureTicks inserts it when
// there is none, at the workers' start and every housekeeping, and the claim of a tick inserts
// the next one. A claimed tick is no longer its kind's queued tick: the claim sets its schedule
// to 0 (it keeps scheduled_for), so its retry, a re-queue after a lost lease or a stop, and a
// retry by hand of a dead tick are queued beside the next tick without a conflict.

// ScheduleError is a scheduled kind whose next tick cannot be found: its cron expression does
// not parse or never matches. The checker refuses both (E0050), so only a build that bypassed
// it meets one.
type ScheduleError struct {
	Kind string
	Err  error
}

func (e *ScheduleError) Error() string {
	return "jobs: the schedule of " + e.Kind + ": " + e.Err.Error()
}
func (e *ScheduleError) Unwrap() error { return e.Err }

// day is the length of the day an interval must divide to be put on the grid.
const day = 24 * time.Hour

// parsed caches a cron expression's Schedule (or its error), by its text.
var parsed sync.Map // string → cronParse

type cronParse struct {
	s   cron.Schedule
	err error
}

// scheduledKind reports whether k is scheduled: by interval or by cron.
func scheduledKind(k Kind) bool { return k.Every > 0 || k.Cron != "" }

// Next is when k's tick after `after` is due, in UTC: for k.Cron the first matching minute
// strictly after `after`; for k.Every, after + Every rounded down to a multiple of Every when
// Every divides a day (so instances and restarts land on the same grid, aligned on midnight
// UTC), else after + Every. An error (*ScheduleError) for a cron expression that does not parse
// or matches no time, or a kind that is not scheduled.
func Next(k Kind, after time.Time) (time.Time, error) {
	switch {
	case k.Cron != "":
		v, ok := parsed.Load(k.Cron)
		if !ok {
			s, err := cron.Parse(k.Cron)
			v, _ = parsed.LoadOrStore(k.Cron, cronParse{s, err})
		}
		p := v.(cronParse)
		if p.err != nil {
			return time.Time{}, &ScheduleError{k.Name, fmt.Errorf("cron %q: %w", k.Cron, p.err)}
		}
		t := p.s.Next(after)
		if t.IsZero() {
			return time.Time{}, &ScheduleError{k.Name, fmt.Errorf("cron %q matches no time after %s", k.Cron, after.UTC().Format(time.RFC3339))}
		}
		return t, nil
	case k.Every > 0:
		t := after.Add(k.Every)
		if day%k.Every == 0 {
			t = t.Truncate(k.Every) // a multiple since the zero Time, a midnight UTC
		}
		return t.UTC(), nil
	}
	return time.Time{}, &ScheduleError{k.Name, errors.New("it has no `every` or `cron`")}
}

// EnsureTicks inserts, for each scheduled kind of kinds without a queued tick in s, its tick
// due at Next(k, now). Two callers at once, in one process or several, insert one: the
// _alder_jobs_tick index refuses the second, which is ignored. A kind whose next tick cannot be
// found (*ScheduleError) gets none; the others do, and the errors are joined.
func EnsureTicks(ctx context.Context, s eff.Store, kinds []Kind, now time.Time) error {
	q, err := db.On(s, nil)
	if err != nil {
		return err
	}
	return EnsureTicksOn(ctx, q, kinds, now)
}

// EnsureTicksOn is EnsureTicks on q: a test's transaction (Test.work).
func EnsureTicksOn(ctx context.Context, q db.Execer, kinds []Kind, now time.Time) error {
	var errs []error
	for _, k := range kinds {
		if !scheduledKind(k) {
			continue
		}
		at, err := Next(k, now)
		if err == nil {
			err = insertTick(ctx, q, k.Name, at, now)
		}
		if err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// ClaimedTick does, on q in the claim's transaction, what claiming row (a row of kind k, just
// claimed at now) asks when it is its kind's queued tick: the row stops being the queued tick
// (schedule 0) and the next tick is inserted, due at Next(k, max(scheduled_for, now)), so a
// tick missed while the app was down runs once, late, and the next is on the grid. Nothing for
// any other row, or a kind no longer scheduled (its last tick runs). An error the claim must
// roll back on is a statement's; a *ScheduleError comes after the statements, and the claim
// stands without a next tick (EnsureTicks reports the kind again at each housekeeping).
func ClaimedTick(ctx context.Context, q db.Execer, k Kind, row Row, now time.Time) error {
	if !row.Schedule {
		return nil
	}
	if _, err := q.ExecContext(ctx, `UPDATE "_alder_jobs" SET "schedule" = 0 WHERE "id" = ?`, row.ID); err != nil {
		return err
	}
	if !scheduledKind(k) {
		return nil
	}
	after := now
	if row.ScheduledFor != nil {
		if t := time.UnixMilli(*row.ScheduledFor); t.After(after) {
			after = t
		}
	}
	at, err := Next(k, after)
	if err != nil {
		return err
	}
	return insertTick(ctx, q, k.Name, at, now)
}

// insertTick inserts kind's queued tick due at `at`, created now, unless one is queued already.
// An id another row has is drawn again, up to idTries times, as Enqueue does.
func insertTick(ctx context.Context, q db.Execer, kind string, at, now time.Time) error {
	var id string
	for range idTries {
		id = newID()
		res, err := q.ExecContext(ctx, `INSERT INTO "_alder_jobs" ("id", "kind", "params", "state", "run_at", "created", "schedule", "scheduled_for") VALUES (?, ?, '{}', 'queued', ?, ?, 1, ?) ON CONFLICT DO NOTHING`,
			id, kind, at.UnixMilli(), now.UnixMilli(), at.UnixMilli())
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n == 1 {
			return nil
		}
		var old string
		err = q.QueryRowContext(ctx, `SELECT "id" FROM "_alder_jobs" WHERE "kind" = ? AND "schedule" = 1 AND "state" = 'queued'`, kind).Scan(&old)
		if !errors.Is(err, sql.ErrNoRows) {
			return err // nil: the kind's tick is queued already
		}
	}
	return fmt.Errorf("jobs: job id %s is taken (%d ids drawn)", id, idTries)
}
