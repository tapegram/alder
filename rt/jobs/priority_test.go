package jobs_test

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"alder/rt/db"
	"alder/rt/jobs"
)

// insertPri writes a queued job row of kind A.B with the given priority, run_at and created.
func insertPri(t *testing.T, st *db.SQLite, id string, priority int, runAt, created time.Time) {
	t.Helper()
	if _, err := raw(t, st).ExecContext(context.Background(), `INSERT INTO "_alder_jobs" ("id", "kind", "params", "state", "run_at", "created", "priority") VALUES (?, 'A.B', '{}', 'queued', ?, ?, ?)`,
		id, runAt.UnixMilli(), created.UnixMilli(), priority); err != nil {
		t.Fatal(err)
	}
}

// The claim takes the due row of the highest priority first, then the earliest run_at, then the
// earliest created; a negative priority runs after the default 0; a row not yet due blocks none.
func TestClaim_PriorityOrder(t *testing.T) {
	st := openFile(t)
	at := func(s int) time.Time { return t0.Add(time.Duration(s) * time.Second) }
	insertPri(t, st, "low", 0, at(1), at(0))
	insertPri(t, st, "high", 10, at(5), at(0))
	insertPri(t, st, "neg", -5, at(0), at(0))
	insertPri(t, st, "tie", 10, at(5), at(1))
	insertPri(t, st, "high2", 10, at(60), at(0))
	kinds := []jobs.Kind{timed("A.B", time.Minute)}
	for _, want := range []string{"high", "tie", "low", "neg"} {
		if r := claimAt(t, st, kinds, at(10)); r.ID != want {
			t.Fatalf("claimed %s, want %s", r.ID, want)
		}
	}
	if r, err := jobs.Claim(context.Background(), st, kinds, at(10), "w"); r != nil || err != nil {
		t.Fatalf("claimed %+v %v: high2 is not due", r, err)
	}
}

func TestClaim_ReturnsPriority(t *testing.T) {
	st := openFile(t)
	insertPri(t, st, "p7", 7, t0, t0)
	if r := claimAt(t, st, []jobs.Kind{timed("A.B", time.Minute)}, t0); r.Priority != 7 {
		t.Fatalf("claimed row's priority %d, want 7", r.Priority)
	}
}

// v1's claim (RETURNING *, fifteen destinations) against a schema-2 table fails to scan: spec
// §5's documented behaviour of an older build after the upgrade.
func TestClaim_OldBuildFailsOnSchema2(t *testing.T) {
	const v1ClaimStmt = `UPDATE _alder_jobs SET state='running', attempts=attempts+1, lease_until=LEASE, worker=?, started=? WHERE id = (SELECT id FROM _alder_jobs WHERE state='queued' AND run_at <= ? AND kind IN (KINDS) ORDER BY run_at, created LIMIT 1) RETURNING *`
	st := openFile(t)
	insertPri(t, st, "row1", 0, t0, t0)
	stmt := strings.Replace(strings.Replace(v1ClaimStmt, "LEASE", "?", 1), "KINDS", "?", 1)
	var (
		id, kind, params, state     string
		key, errTxt, worker         sql.NullString
		runAt, created              int64
		started, fin, lease, schedA sql.NullInt64
		attempts                    int
		schedule                    bool
	)
	err := raw(t, st).QueryRowContext(context.Background(), stmt, t0.Add(time.Minute).UnixMilli(), "w", t0.UnixMilli(), t0.UnixMilli(), "A.B").
		Scan(&id, &kind, &params, &key, &state, &runAt, &created, &started, &fin, &attempts, &errTxt, &lease, &worker, &schedule, &schedA)
	if err == nil || !strings.Contains(err.Error(), "destination") {
		t.Fatalf("v1 claim on schema 2: %v, want a Scan error", err)
	}
}
