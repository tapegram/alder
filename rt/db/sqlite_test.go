package db

import (
	"context"
	"slices"
	"testing"
)

// Ensure and Migrate create the jobs table where they create _alder_meta.
func TestEnsureTable_ByEnsureAndMigrate(t *testing.T) {
	ctx := context.Background()
	s := openMem(t)
	if err := s.Ensure(ctx, []TableDesc{postV1}, false); err != nil {
		t.Fatal(err)
	}
	if ok, err := tableExists(ctx, s.db, JobsTable); err != nil || !ok {
		t.Fatalf("after Ensure: %v %v", ok, err)
	}
	m := openMem(t)
	if _, err := m.Migrate(ctx, []TableDesc{postV1, commentT}, migrations(t)[:1], false); err != nil {
		t.Fatal(err)
	}
	if ok, err := tableExists(ctx, m.db, JobsTable); err != nil || !ok {
		t.Fatalf("after Migrate: %v %v", ok, err)
	}
	if v, err := metaValue(ctx, m.db, "jobs_schema"); err != nil || v != "1" {
		t.Fatalf("jobs_schema after Migrate: %q %v", v, err)
	}
}

// A dev recreate deletes every job row with the app's rows: a queued job of the old schema
// would run against tables made anew. Ensure's and Migrate's recreates both.
func TestRecreate_DeletesJobs(t *testing.T) {
	ctx := context.Background()
	job := func(id, state string) string {
		return `INSERT INTO "_alder_jobs" ("id", "kind", "params", "state", "run_at", "created") VALUES ('` + id + `', 'K', '{}', '` + state + `', 0, 0)`
	}
	s := openMem(t)
	if err := s.Ensure(ctx, []TableDesc{postV1}, true); err != nil {
		t.Fatal(err)
	}
	exec(t, s, job("0000000a", "queued"))
	exec(t, s, job("0000000b", "done"))
	if err := s.Ensure(ctx, []TableDesc{postV1}, true); err != nil {
		t.Fatal(err)
	}
	if n := rawCount(t, s, `SELECT COUNT(*) FROM "_alder_jobs"`); n != 2 {
		t.Fatalf("no recreate: %d job rows, want 2", n)
	}
	if err := s.Ensure(ctx, []TableDesc{postV2}, true); err != nil {
		t.Fatal(err)
	}
	if n := rawCount(t, s, `SELECT COUNT(*) FROM "_alder_jobs"`); n != 0 {
		t.Fatalf("after Ensure's recreate: %d job rows", n)
	}
	m := openMem(t)
	if err := m.Ensure(ctx, []TableDesc{postV1}, true); err != nil {
		t.Fatal(err)
	}
	exec(t, m, job("0000000c", "running"))
	if _, err := m.Migrate(ctx, []TableDesc{postV1, commentT}, migrations(t)[:1], true); err != nil {
		t.Fatal(err)
	}
	if n := rawCount(t, m, `SELECT COUNT(*) FROM "_alder_jobs"`); n != 0 {
		t.Fatalf("after Migrate's recreate: %d job rows", n)
	}
}

// Every `_alder_` table is the runtime's: _alder_jobs and an invented _alder_future are not app
// tables, and neither makes drift for Ensure or Migrate.
func TestAppTables_IgnoresAlderPrefix(t *testing.T) {
	ctx := context.Background()
	s := openMem(t)
	if err := s.Ensure(ctx, []TableDesc{postV1}, false); err != nil {
		t.Fatal(err)
	}
	exec(t, s, `CREATE TABLE "_alder_future" ("x" TEXT)`)
	got, err := appTables(ctx, s.db)
	if err != nil || !slices.Equal(got, []string{"Post__Post"}) {
		t.Fatalf("appTables: %v %v", got, err)
	}
	if err := s.Ensure(ctx, []TableDesc{postV1}, false); err != nil {
		t.Fatalf("Ensure again: %v", err)
	}
	m := openMem(t)
	ms := migrations(t)[:1]
	if _, err := m.Migrate(ctx, []TableDesc{postV1, commentT}, ms, false); err != nil {
		t.Fatal(err)
	}
	exec(t, m, `CREATE TABLE "_alder_future" ("x" TEXT)`)
	if _, err := m.Migrate(ctx, []TableDesc{postV1, commentT}, ms, false); err != nil {
		t.Fatalf("Migrate again: %v", err)
	}
}
