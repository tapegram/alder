package deploy

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"alder/internal/ir"
	"alder/internal/tursofake"
	"alder/rt/db"
)

// The Turso database's own steps against the fake (internal/tursofake); the whole deploy, with
// the app's executable, is cmd/alder's TestDeployTurso_*.

func fakeTurso(t *testing.T, f *tursofake.Server, silos ...string) *Turso {
	t.Helper()
	tt := NewTurso(&ir.Database{Kind: "turso", Org: f.Org, Group: "default", App: "blog"})
	tt.Silos = silos
	tt.API = db.NewTurso(db.TursoConfig{API: f.URL, Org: f.Org, Group: "default", App: "blog", Token: f.APIToken})
	return tt
}

// The preflight: a group that does not exist is refused with nothing created; then the shared
// database is created (once) and its host returned; a name too long for Turso is refused.
func TestTurso_Preflight(t *testing.T) {
	f := tursofake.New(t, "acme")
	ctx := context.Background()
	tt := fakeTurso(t, f, "org")
	tt.Group = "eu"
	_, err := tt.Preflight(ctx)
	var r *Refusal
	if !errors.As(err, &r) || !strings.Contains(r.Msg, "Turso organization acme has no group eu: create it (`turso group create eu`)") {
		t.Fatalf("missing group: %v", err)
	}
	if len(f.Names()) != 0 {
		t.Fatalf("created %v", f.Names())
	}
	tt.Group = "default"
	host, err := tt.Preflight(ctx)
	if err != nil || !strings.HasSuffix(host, "/db/blog") {
		t.Fatalf("%q %v", host, err)
	}
	if _, err := tt.Preflight(ctx); err != nil {
		t.Fatalf("again: %v", err)
	}
	if strings.Join(f.Names(), ",") != "blog" {
		t.Fatalf("databases %v", f.Names())
	}
	if ok, err := tt.HaveTables(ctx); err != nil || ok {
		t.Fatalf("a new database has tables: %v %v", ok, err)
	}
	long := fakeTurso(t, f, "organisation")
	long.App = "an-app-name-long-enough-x"
	if _, err := long.Preflight(ctx); !errors.As(err, &r) || !strings.Contains(r.Msg, "more than Turso's 64") {
		t.Fatalf("long name: %v", err)
	}
	// without a token on this machine
	t.Setenv(db.EnvTursoToken, "")
	none := NewTurso(&ir.Database{Org: "acme", App: "blog"})
	if _, err := none.Preflight(ctx); !errors.As(err, &r) || !strings.Contains(r.Msg, "this machine's environment does not set TURSO_API_TOKEN") {
		t.Fatalf("no token: %v", err)
	}
}

// The deploy lock is a row of the shared database: a second run is refused, naming the holder.
func TestTurso_DeployLock(t *testing.T) {
	f := tursofake.New(t, "acme")
	ctx := context.Background()
	a, b := fakeTurso(t, f), fakeTurso(t, f)
	unlock, err := a.Lock(ctx, "first")
	if err != nil {
		t.Fatal(err)
	}
	unlock() // no database yet: nothing to lock there
	if _, err := a.Preflight(ctx); err != nil {
		t.Fatal(err)
	}
	unlock, err = a.Lock(ctx, "first")
	if err != nil {
		t.Fatal(err)
	}
	var r *Refusal
	if _, err := b.Lock(ctx, "second"); !errors.As(err, &r) || !strings.Contains(r.Msg, "another deploy, rollback or restore holds the database blog at Turso (its _alder_lock row: first (") {
		t.Fatalf("second: %v", err)
	}
	unlock()
	unlock2, err := b.Lock(ctx, "second")
	if err != nil {
		t.Fatalf("after release: %v", err)
	}
	unlock2()
}

// Point-in-time: a copy of the database as it was at a time; a silo's database restored in
// place (copied at the time, the current one kept, created again from the copy).
func TestTurso_PointInTime(t *testing.T) {
	f := tursofake.New(t, "acme")
	ctx := context.Background()
	clock := time.Date(2026, 10, 17, 10, 0, 0, 0, time.UTC)
	f.SetNow(func() time.Time { return clock })
	tt := fakeTurso(t, f, "org")
	tt.Now = func() time.Time { return clock }
	child := db.ChildName("blog", "org", "Org1")
	if err := f.Create(child, "default"); err != nil {
		t.Fatal(err)
	}
	f.Exec(child, `CREATE TABLE "P" ("x" TEXT)`)
	f.Exec(child, `INSERT INTO "P" VALUES ('before')`)
	// the fake's points in time are its commits through the protocol: write through it
	clock = clock.Add(time.Hour)
	writeVia(t, tt, child, `INSERT INTO "P" VALUES ('noon')`)
	clock = clock.Add(time.Hour)
	writeVia(t, tt, child, `INSERT INTO "P" VALUES ('after')`)
	var w bytes.Buffer
	kept, err := tt.RestoreChild(ctx, "org", "Org1", time.Date(2026, 10, 17, 11, 30, 0, 0, time.UTC), &w)
	if err != nil {
		t.Fatalf("%v\n%s", err, w.String())
	}
	if got := rowsOf(t, f, child); got != "before,noon" {
		t.Fatalf("restored: %s\n%s", got, w.String())
	}
	if got := rowsOf(t, f, kept); got != "before,noon,after" {
		t.Fatalf("kept %s: %s", kept, got)
	}
	names := strings.Join(f.Names(), ",")
	if strings.Contains(names, "-r") {
		t.Fatalf("the restore's copy was not deleted: %s", names)
	}
	// it is still listed as the silo's database
	kids, err := tt.api().Children(ctx, "blog", "org")
	if err != nil || len(kids) != 1 || kids[0].Name != child {
		t.Fatalf("children %v %v", kids, err)
	}
}

func writeVia(t *testing.T, tt *Turso, name, stmt string) {
	t.Helper()
	ctx := context.Background()
	st, err := tt.api().Open(ctx, name, false)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := db.ExecRaw(ctx, st, stmt); err != nil {
		t.Fatal(err)
	}
}

func rowsOf(t *testing.T, f *tursofake.Server, name string) string {
	t.Helper()
	rows, err := f.DB(name).Query(`SELECT "x" FROM "P" ORDER BY rowid`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var x string
		rows.Scan(&x)
		out = append(out, x)
	}
	return strings.Join(out, ",")
}
