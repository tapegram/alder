package db

import (
	"context"
	"strings"
	"testing"

	"alder/internal/tursofake"
)

// CheckShared compares this build's migrations with what the shared database recorded,
// writing nothing: fresh, behind, ahead, equal; a database changed by hand is an error.
func TestStartCheck_States(t *testing.T) {
	ctx := context.Background()
	ms := migrations(t)
	v3 := []TableDesc{postV3, commentT}
	v2 := []TableDesc{postV2, commentT}

	// fresh: nothing there, and nothing created by looking
	s := openMem(t)
	state, detail, err := CheckShared(ctx, s, v3, ms)
	if err != nil || state != SharedFresh {
		t.Fatalf("fresh: %q %q %v", state, detail, err)
	}
	if ok, _ := tableExists(ctx, s.db, "_alder_meta"); ok {
		t.Fatal("CheckShared created _alder_meta")
	}
	if ok, _ := tableExists(ctx, s.db, migrationsTable); ok {
		t.Fatal("CheckShared created _alder_migrations")
	}

	// behind by one: the detail names it; nothing is migrated
	if _, err := s.Migrate(ctx, v2, ms[:2], false); err != nil {
		t.Fatal(err)
	}
	state, detail, err = CheckShared(ctx, s, v3, ms)
	if err != nil || state != SharedBehind || !strings.Contains(detail, "0003_add_views") {
		t.Fatalf("behind: %q %q %v", state, detail, err)
	}
	if got := strings.Join(recordedNames(t, s), " "); got != "0001_initial 0002_add_summary" {
		t.Fatalf("CheckShared migrated: %s", got)
	}

	// equal
	state, detail, err = CheckShared(ctx, s, v2, ms[:2])
	if err != nil || state != SharedOK {
		t.Fatalf("ok: %q %q %v", state, detail, err)
	}

	// ahead: the database has 0003, this build stops at 0002
	if _, err := s.Migrate(ctx, v3, ms, false); err != nil {
		t.Fatal(err)
	}
	state, detail, err = CheckShared(ctx, s, v2, ms[:2])
	if err != nil || state != SharedAhead || !strings.Contains(detail, "0003_add_views") {
		t.Fatalf("ahead: %q %q %v", state, detail, err)
	}

	// changed by hand since the last migration: an error, not a state to serve
	exec(t, s, `CREATE TABLE "Stray" ("x" TEXT)`)
	if state, _, err := CheckShared(ctx, s, v3, ms); err == nil || !strings.Contains(err.Error(), "changed outside migrations") {
		t.Fatalf("drift: %q %v", state, err)
	}

	// made by Ensure (no migrations recorded): behind, a deploy adopts it
	e := openMem(t)
	if err := e.Ensure(ctx, v2, false); err != nil {
		t.Fatal(err)
	}
	if state, _, err := CheckShared(ctx, e, v3, ms); err != nil || state != SharedBehind {
		t.Fatalf("ensured: %q %v", state, err)
	}
	// an app without migrations: its tables are the app's, or it is behind
	if state, _, err := CheckShared(ctx, e, v2, nil); err != nil || state != SharedOK {
		t.Fatalf("no migrations, same tables: %q %v", state, err)
	}
	if state, _, err := CheckShared(ctx, e, v3, nil); err != nil || state != SharedBehind {
		t.Fatalf("no migrations, other tables: %q %v", state, err)
	}
}

// The same check over Turso's protocol (the fake), where the runtime meets it.
func TestStartCheck_Turso(t *testing.T) {
	f := tursofake.New(t, "acme")
	st, err := OpenSQLite(tursoEnv(t, f))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	ms := migrations(t)
	if state, _, err := CheckShared(ctx, st, []TableDesc{postV3, commentT}, ms); err != nil || state != SharedFresh {
		t.Fatalf("fresh: %q %v", state, err)
	}
	if _, err := st.Migrate(ctx, []TableDesc{postV2, commentT}, ms[:2], false); err != nil {
		t.Fatal(err)
	}
	if state, _, err := CheckShared(ctx, st, []TableDesc{postV3, commentT}, ms); err != nil || state != SharedBehind {
		t.Fatalf("behind: %q %v", state, err)
	}
	if state, _, err := CheckShared(ctx, st, []TableDesc{postV2, commentT}, ms[:2]); err != nil || state != SharedOK {
		t.Fatalf("ok: %q %v", state, err)
	}
	if state, _, err := CheckShared(ctx, st, []TableDesc{postV1, commentT}, ms[:1]); err != nil || state != SharedAhead {
		t.Fatalf("ahead: %q %v", state, err)
	}
}
