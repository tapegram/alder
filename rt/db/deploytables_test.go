package db

import (
	"context"
	"reflect"
	"testing"
	"time"

	"alder/internal/tursofake"
)

// The deploy tables of a shared database on Turso: made idempotently; rows put (an upsert),
// listed newest first, deleted; the current build set and read back.
func TestDeployTables_RoundTrip(t *testing.T) {
	f := tursofake.New(t, "acme")
	st, err := OpenSQLite(tursoEnv(t, f))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	for range 2 {
		if err := EnsureDeployTables(ctx, st); err != nil {
			t.Fatal(err)
		}
	}
	if cur, err := CurrentBuild(ctx, st); err != nil || cur != "" {
		t.Fatalf("current before any: %q %v", cur, err)
	}
	t0 := time.Date(2026, 10, 17, 10, 0, 0, 0, time.UTC)
	a := DeployRow{ID: "20261017-100000-aaaa", Record: `{"id":"a"}`, Schema: `{"tables":[]}`, SchemaHash: "abc", Migrations: `["0001_initial"]`, Pitr: "2026-10-17T10:00:00Z", Created: t0}
	b := DeployRow{ID: "20261017-110000-bbbb", Record: `{"id":"b"}`, Schema: `{}`, SchemaHash: "def", Migrations: `["0001_initial","0002_x"]`, Created: t0.Add(time.Hour)}
	for _, r := range []DeployRow{a, b} {
		if err := PutDeployRow(ctx, st, r); err != nil {
			t.Fatal(err)
		}
	}
	// an upsert: the same id again replaces its row
	a.Record = `{"id":"a","v":2}`
	if err := PutDeployRow(ctx, st, a); err != nil {
		t.Fatal(err)
	}
	rows, err := DeployRows(ctx, st)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(rows, []DeployRow{b, a}) {
		t.Fatalf("rows\n%+v\nwant\n%+v", rows, []DeployRow{b, a})
	}
	// a row without migrations stores the empty list
	c := DeployRow{ID: "c", Created: t0.Add(-time.Hour)}
	if err := PutDeployRow(ctx, st, c); err != nil {
		t.Fatal(err)
	}
	rows, _ = DeployRows(ctx, st)
	if len(rows) != 3 || rows[2].ID != "c" || rows[2].Migrations != "[]" {
		t.Fatalf("c: %+v", rows)
	}
	for _, id := range []string{a.ID, b.ID} {
		if err := SetCurrent(ctx, st, id); err != nil {
			t.Fatal(err)
		}
		if cur, err := CurrentBuild(ctx, st); err != nil || cur != id {
			t.Fatalf("current %q %v, want %s", cur, err, id)
		}
	}
	if err := DeleteDeployRows(ctx, st, []string{a.ID, "c", "never"}); err != nil {
		t.Fatal(err)
	}
	if err := DeleteDeployRows(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	rows, _ = DeployRows(ctx, st)
	if len(rows) != 1 || rows[0].ID != b.ID {
		t.Fatalf("after delete: %+v", rows)
	}
}

// Review Focus 2: the deploy tables are not the app's. On a store with no app tables
// HasAppTables stays false; on a migrated one, no drift is reported and MigrateLog with nothing
// pending applies nothing; CheckShared sees the database as it was.
func TestDeployTables_NotAppTables(t *testing.T) {
	ctx := context.Background()
	empty := openMem(t)
	if err := EnsureDeployTables(ctx, empty); err != nil {
		t.Fatal(err)
	}
	if has, err := HasAppTables(ctx, empty); err != nil || has {
		t.Fatalf("HasAppTables %v %v", has, err)
	}
	if tables, _ := appTables(ctx, empty.db); len(tables) != 0 {
		t.Fatalf("app tables %v", tables)
	}
	if state, _, err := CheckShared(ctx, empty, []TableDesc{postV3, commentT}, migrations(t)); err != nil || state != SharedFresh {
		t.Fatalf("CheckShared on an empty store with deploy tables: %q %v", state, err)
	}

	s := openMem(t)
	ms := migrations(t)
	tables := []TableDesc{postV3, commentT}
	if _, err := s.Migrate(ctx, tables, ms, false); err != nil {
		t.Fatal(err)
	}
	if err := EnsureDeployTables(ctx, s); err != nil {
		t.Fatal(err)
	}
	if err := PutDeployRow(ctx, s, DeployRow{ID: "x", Created: time.Now()}); err != nil {
		t.Fatal(err)
	}
	var logged []string
	applied, err := s.MigrateLog(ctx, tables, ms, false, func(m string) { logged = append(logged, m) })
	if err != nil || len(applied) != 0 || len(logged) != 0 {
		t.Fatalf("MigrateLog: %v %v %v", applied, logged, err)
	}
	if state, _, err := CheckShared(ctx, s, tables, ms); err != nil || state != SharedOK {
		t.Fatalf("CheckShared: %q %v", state, err)
	}
	if err := s.Ensure(ctx, tables, false); err != nil {
		t.Fatalf("Ensure sees drift: %v", err)
	}
}
