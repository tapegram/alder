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
