//go:build turso

package db

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"strings"
	"testing"
	"time"
)

// The run against the real Turso (manual: chapter 16, "Turso"): with
//
//	TURSO_API_TOKEN=… ALDER_TURSO_ORG=<org> [ALDER_TURSO_GROUP=default] go test -tags turso ./rt/db -run TestTursoReal -v
//
// it creates databases named alder-t-<random>-…, checks what the fake stands in for — the
// platform calls, the protocol (CRUD, a transaction, a savepoint, the refusals' texts), a silo's
// database created and listed, DDL inside a transaction, the usage call, a copy at a point in
// time —
// and deletes them at the end. Every call it makes is one alder makes.
func TestTursoReal(t *testing.T) {
	cfg := TursoConfigFromEnv()
	if cfg.Token == "" || cfg.Org == "" {
		t.Skip("set TURSO_API_TOKEN and ALDER_TURSO_ORG to run against Turso")
	}
	var b [3]byte
	rand.Read(b[:])
	app := "alder-t-" + hex.EncodeToString(b[:])
	cfg.App = app
	api := NewTurso(cfg)
	ctx := context.Background()
	var made []string
	t.Cleanup(func() {
		for i := len(made) - 1; i >= 0; i-- {
			if err := api.Delete(context.Background(), made[i]); err != nil {
				t.Logf("deleting %s: %v (delete it by hand)", made[i], err)
			}
		}
	})
	create := func(o CreateOptions) TursoDB {
		t.Helper()
		d, err := api.Create(ctx, o)
		if err != nil {
			t.Fatalf("create %s: %v", o.Name, err)
		}
		made = append(made, o.Name)
		if d.Hostname == "" {
			if d, _, err = api.Database(ctx, o.Name); err != nil || d.Hostname == "" {
				t.Fatalf("no host for %s: %v", o.Name, err)
			}
		}
		return d
	}
	if ok, err := api.GroupExists(ctx, cfg.Group); err != nil || !ok {
		t.Fatalf("group %s: %v %v", cfg.Group, ok, err)
	}

	// the shared database through ALDER_DB=libsql://…
	shared := create(CreateOptions{Name: app})
	t.Setenv(EnvTursoDB, app)
	t.Setenv(EnvDBToken, "")
	st, err := OpenSQLite("libsql://" + shared.Hostname)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	todo := TableDesc{Name: "Todo.Todo", ID: "id", Columns: []Column{{Name: "id", Type: CText}, {Name: "title", Type: CText, Unique: true}, {Name: "n", Type: CInt}}}
	if err := st.Ensure(ctx, []TableDesc{todo}, false); err != nil {
		t.Fatal(err)
	}
	tx, err := st.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Insert(ctx, tx, "Todo.Todo", todo.Columns, "id", "", "", []any{nil, "a", int64(1)}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := st.Savepoint(ctx, tx, "sp1"); err != nil {
		t.Fatal(err)
	}
	st.Insert(ctx, tx, "Todo.Todo", todo.Columns, "id", "", "", []any{nil, "b", int64(2)}, time.Now())
	if err := st.RollbackTo(ctx, tx, "sp1"); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if n, err := st.Count(ctx, nil, "Todo.Todo", SQL{}); err != nil || n != 1 {
		t.Fatalf("count %d %v", n, err)
	}
	if _, err := st.Insert(ctx, nil, "Todo.Todo", todo.Columns, "id", "", "", []any{nil, "a", int64(3)}, time.Now()); err == nil || !strings.Contains(err.Error(), "must be unique") {
		t.Fatalf("unique: %v", err)
	}
	// the deploy lock row
	if held, err := TakeLock(ctx, st, "real", time.Minute, time.Now()); err != nil || held != "" {
		t.Fatalf("lock %q %v", held, err)
	}
	ReleaseLock(ctx, st, "real")
	if n, err := api.StorageBytes(ctx, app); err != nil {
		t.Fatalf("usage: %d %v", n, err)
	}

	// a silo's database: created plainly, listed by its name's prefix, and migrated the way the
	// runtime does (DDL inside a transaction: the open-time check, the deploy's dry run)
	child := ChildName(app, "org", NewID())
	create(CreateOptions{Name: child})
	cdb, err := api.Open(ctx, child, false)
	if err != nil {
		t.Fatal(err)
	}
	defer cdb.Close()
	if err := ExecRaw(ctx, cdb, `CREATE TABLE "P" ("id" TEXT NOT NULL PRIMARY KEY, "x" TEXT) STRICT`); err != nil {
		t.Fatal(err)
	}
	ctxTx, err := cdb.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ctxTx.ExecContext(ctx, `ALTER TABLE "P" ADD COLUMN "y" TEXT`); err != nil {
		t.Fatalf("DDL inside a transaction: %v", err)
	}
	if err := ctxTx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if kids, err := api.Children(ctx, app, "org"); err != nil || len(kids) != 1 || kids[0].Name != child {
		t.Fatalf("children: %v %v", kids, err)
	}

	// a copy of the shared database at a point in time
	time.Sleep(2 * time.Second)
	at := time.Now().UTC().Truncate(time.Second)
	time.Sleep(2 * time.Second)
	if _, err := st.Insert(ctx, nil, "Todo.Todo", todo.Columns, "id", "", "", []any{nil, "later", int64(9)}, time.Now()); err != nil {
		t.Fatal(err)
	}
	cp := create(CreateOptions{Name: app + "-r1", Seed: &TursoSeed{Name: app, Timestamp: at}})
	t.Setenv(EnvTursoDB, cp.Name)
	cst, err := OpenSQLite("libsql://" + cp.Hostname)
	if err != nil {
		t.Fatal(err)
	}
	defer cst.Close()
	cst.register([]TableDesc{todo})
	if n, err := cst.Count(ctx, nil, "Todo.Todo", SQL{}); err != nil || n != 1 {
		t.Fatalf("the copy at %s has %d rows (%v), want 1", at.Format(time.RFC3339), n, err)
	}
	if os.Getenv("ALDER_TURSO_KEEP") == "1" {
		made = nil
	}
}
