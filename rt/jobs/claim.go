package jobs

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"alder/rt/db"
	"alder/rt/eff"
)

// claimStmt is spec §3's claim: one statement that marks the oldest due row of a known kind
// running and gives it. LEASE becomes a CASE giving the claimed row's own kind's lease (each
// kind's WHEN ? THEN ?), KINDS the kinds' placeholders; the claim also records when the attempt
// started.
const claimStmt = `UPDATE _alder_jobs SET state='running', attempts=attempts+1, lease_until=LEASE, worker=?, started=? WHERE id = (SELECT id FROM _alder_jobs WHERE state='queued' AND run_at <= ? AND kind IN (KINDS) ORDER BY run_at, created LIMIT 1) RETURNING *`

// Lease is how long a claimed row of kind k is leased: its timeout (maxTimeout for a kind with
// none) plus LeaseMargin. Past it, housekeeping takes the row for lost.
func Lease(k Kind) time.Duration {
	t := k.Timeout
	if t <= 0 {
		t = maxTimeout
	}
	return t + LeaseMargin
}

// Claim claims the oldest row of s due at now whose kind is one of kinds: it is marked running
// by worker, started now, its attempts count one more, and its lease runs until now + Lease of
// its kind. The claim is one statement in a transaction of its own (BEGIN IMMEDIATE on a file
// database), so two workers, in one process or several on one database, never claim the same
// row. nil when no row is due.
//
// Claiming a tick also inserts its kind's next tick, in the same transaction (ClaimedTick): the
// claim and the next tick commit together. When the next tick cannot be found (a
// *ScheduleError), the claim commits without it and Claim returns the row and that error.
func Claim(ctx context.Context, s eff.Store, kinds []Kind, now time.Time, worker string) (*Row, error) {
	if len(kinds) == 0 {
		return nil, nil
	}
	var lease strings.Builder
	var args, names []any
	lease.WriteString("CASE kind")
	for _, k := range kinds {
		lease.WriteString(" WHEN ? THEN ?")
		args = append(args, k.Name, now.Add(Lease(k)).UnixMilli())
		names = append(names, k.Name)
	}
	lease.WriteString(" END")
	stmt := strings.Replace(claimStmt, "LEASE", lease.String(), 1)
	stmt = strings.Replace(stmt, "KINDS", strings.TrimSuffix(strings.Repeat("?, ", len(kinds)), ", "), 1)
	args = append(append(args, worker, now.UnixMilli(), now.UnixMilli()), names...)
	tx, err := s.Begin(ctx)
	if err != nil {
		return nil, err
	}
	q, err := db.On(s, tx)
	if err != nil {
		tx.Rollback()
		return nil, err
	}
	r, err := scanRow(q.QueryRowContext(ctx, stmt, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, tx.Rollback()
	}
	if err != nil {
		tx.Rollback()
		return nil, err
	}
	var schedErr *ScheduleError
	if r.Schedule {
		for _, k := range kinds {
			if k.Name == r.Kind {
				err = ClaimedTick(ctx, q, k, *r, now)
				break
			}
		}
		if err != nil && !errors.As(err, &schedErr) {
			tx.Rollback()
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	if schedErr != nil {
		return r, schedErr
	}
	return r, nil
}

// scanRow reads a whole row of the jobs table, its columns in the table's order (SELECT *,
// RETURNING *).
func scanRow(sc interface{ Scan(...any) error }) (*Row, error) {
	var (
		r              Row
		key, errTxt    sql.NullString
		worker         sql.NullString
		started, fin   sql.NullInt64
		lease, schedAt sql.NullInt64
	)
	if err := sc.Scan(&r.ID, &r.Kind, &r.Params, &key, &r.State, &r.RunAt, &r.Created, &started, &fin,
		&r.Attempts, &errTxt, &lease, &worker, &r.Schedule, &schedAt); err != nil {
		return nil, err
	}
	r.Key, r.Error, r.Worker = str(key), str(errTxt), worker.String
	r.Started, r.Finished, r.LeaseUntil, r.ScheduledFor = i64(started), i64(fin), i64(lease), i64(schedAt)
	return &r, nil
}

func str(v sql.NullString) *string {
	if !v.Valid {
		return nil
	}
	return &v.String
}

func i64(v sql.NullInt64) *int64 {
	if !v.Valid {
		return nil
	}
	return &v.Int64
}
