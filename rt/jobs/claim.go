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
// running and gives it. The kinds' placeholders replace KINDS.
const claimStmt = `UPDATE _alder_jobs SET state='running', attempts=attempts+1, lease_until=?, worker=? WHERE id = (SELECT id FROM _alder_jobs WHERE state='queued' AND run_at <= ? AND kind IN (KINDS) ORDER BY run_at, created LIMIT 1) RETURNING *`

// Claim claims the oldest row of s due at now whose kind is one of kinds: it is marked running
// by worker, its attempts count one more, and its lease runs until now + lease. The claim is one
// statement in a transaction of its own (BEGIN IMMEDIATE on a file database), so two workers,
// in one process or several on one database, never claim the same row. nil when no row is due.
func Claim(ctx context.Context, s eff.Store, kinds []string, now time.Time, lease time.Duration, worker string) (*Row, error) {
	if len(kinds) == 0 {
		return nil, nil
	}
	stmt := strings.Replace(claimStmt, "KINDS", strings.TrimSuffix(strings.Repeat("?, ", len(kinds)), ", "), 1)
	args := []any{now.Add(lease).UnixMilli(), worker, now.UnixMilli()}
	for _, k := range kinds {
		args = append(args, k)
	}
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
	if err := tx.Commit(); err != nil {
		return nil, err
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
