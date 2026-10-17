package db

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"alder/internal/tursofake"
	"alder/rt/eff"
)

// The Store over Turso's HTTP protocol, against the fake (internal/tursofake): the same SQLite
// Store code, its database/sql handle a stream per connection.

// tursoEnv points the runtime's Turso settings at fake f for app "blog" and returns the shared
// database's ALDER_DB (created in the fake).
func tursoEnv(t *testing.T, f *tursofake.Server) string {
	t.Helper()
	t.Setenv(EnvTursoAPI, f.URL)
	t.Setenv(EnvTursoOrg, f.Org)
	t.Setenv(EnvTursoGroup, "default")
	t.Setenv(EnvTursoApp, "blog")
	t.Setenv(EnvTursoToken, f.APIToken)
	t.Setenv(EnvTursoDB, "blog")
	t.Setenv(EnvDBToken, "")
	if err := f.Create("blog", "default", "", false); err != nil {
		t.Fatal(err)
	}
	d, ok, err := NewTurso(TursoConfigFromEnv()).Database(context.Background(), "blog")
	if err != nil || !ok {
		t.Fatalf("%v %v", ok, err)
	}
	return "libsql://" + d.Hostname
}

// CRUD, the refusals' texts (UNIQUE, a @ref to nothing, a @restrict delete), and the values'
// types, over the protocol.
func TestTurso_StoreCRUD(t *testing.T) {
	f := tursofake.New(t, "acme")
	path := tursoEnv(t, f)
	st, err := OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if !st.Remote() {
		t.Fatal("not remote")
	}
	ctx := context.Background()
	todo := TableDesc{Name: "Todo.Todo", ID: "id", Columns: []Column{{Name: "id", Type: CText}, {Name: "title", Type: CText, Unique: true}, {Name: "n", Type: CInt}, {Name: "x", Type: CFloat, Nullable: true}, {Name: "done", Type: CBool}, {Name: "org", Type: CText, Nullable: true, Ref: "Org.Org"}}}
	if err := st.Ensure(ctx, []TableDesc{orgDesc, todo}, false); err != nil {
		t.Fatal(err)
	}
	id, err := st.Insert(ctx, nil, "Todo.Todo", todo.Columns, "id", "", "", []any{nil, "a", int64(3), 1.5, int64(1), nil}, clock)
	if err != nil {
		t.Fatal(err)
	}
	row, found, err := st.Get(ctx, nil, "Todo.Todo", id)
	if err != nil || !found {
		t.Fatalf("get: %v %v", found, err)
	}
	if row[1] != "a" || row[2] != int64(3) || row[3] != 1.5 || row[4] != int64(1) || row[5] != nil {
		t.Fatalf("row %#v", row)
	}
	var fail *eff.ErrFail
	if _, err := st.Insert(ctx, nil, "Todo.Todo", todo.Columns, "id", "", "", []any{nil, "a", int64(1), nil, int64(0), nil}, clock); !errors.As(err, &fail) || fail.Msg != "Todo.title must be unique" {
		t.Fatalf("unique: %v", err)
	}
	if _, err := st.Insert(ctx, nil, "Todo.Todo", todo.Columns, "id", "", "", []any{nil, "b", int64(1), nil, int64(0), "nope"}, clock); !errors.As(err, &fail) || fail.Msg != "Todo.org: no Org with that id" {
		t.Fatalf("@ref: %v", err)
	}
	n, err := st.UpdateWhere(ctx, nil, "Todo.Todo", todo.Columns, "id", "", id, "", nil, []any{id, "a2", int64(4), nil, int64(0), nil}, clock)
	if err != nil || n != 1 {
		t.Fatalf("update %d %v", n, err)
	}
	rows, err := st.Query(ctx, nil, "Todo.Todo", todo.Columns, SQL{Where: `"n" = ?`, Args: []any{int64(4)}})
	if err != nil || len(rows) != 1 || rows[0][1] != "a2" {
		t.Fatalf("query %v %v", rows, err)
	}
	c, err := st.Count(ctx, nil, "Todo.Todo", SQL{})
	if err != nil || c != 1 {
		t.Fatalf("count %d %v", c, err)
	}
	if n, err := st.DeleteWhere(ctx, nil, "Todo.Todo", "id", id, "", nil); err != nil || n != 1 {
		t.Fatalf("delete %d %v", n, err)
	}
	// the database itself is the fake's file: the row is gone there
	var left int
	if err := f.DB("blog").QueryRow(`SELECT COUNT(*) FROM "Todo__Todo"`).Scan(&left); err != nil || left != 0 {
		t.Fatalf("left %d %v", left, err)
	}
}

// A transaction is one stream: rolled back, its writes are gone; committed, they stay; a
// savepoint inside it works; another stream does not see uncommitted writes.
func TestTurso_Transactions(t *testing.T) {
	f := tursofake.New(t, "acme")
	st, err := OpenSQLite(tursoEnv(t, f))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	if err := st.Ensure(ctx, []TableDesc{orgDesc}, false); err != nil {
		t.Fatal(err)
	}
	tx, err := st.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Insert(ctx, tx, "Org.Org", orgDesc.Columns, "id", "", "", []any{nil, "gone"}, clock); err != nil {
		t.Fatal(err)
	}
	if n, _ := st.Count(ctx, nil, "Org.Org", SQL{}); n != 0 {
		t.Fatalf("another stream sees %d uncommitted rows", n)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	tx, _ = st.Begin(ctx)
	st.Insert(ctx, tx, "Org.Org", orgDesc.Columns, "id", "", "", []any{nil, "kept"}, clock)
	if err := st.Savepoint(ctx, tx, "sp1"); err != nil {
		t.Fatal(err)
	}
	st.Insert(ctx, tx, "Org.Org", orgDesc.Columns, "id", "", "", []any{nil, "undone"}, clock)
	if err := st.RollbackTo(ctx, tx, "sp1"); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	rows, err := st.Query(ctx, nil, "Org.Org", orgDesc.Columns, SQL{})
	if err != nil || len(rows) != 1 || rows[0][1] != "kept" {
		t.Fatalf("after commit: %v %v", rows, err)
	}
}

// A database deleted and created again refuses its old tokens (401): the store asks for a
// fresh one and goes on; a stream the server dropped is taken again outside a transaction.
func TestTurso_TokenRefreshAndExpiredStream(t *testing.T) {
	f := tursofake.New(t, "acme")
	st, err := OpenSQLite(tursoEnv(t, f))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	if err := st.Ensure(ctx, []TableDesc{orgDesc}, false); err != nil {
		t.Fatal(err)
	}
	f.ExpireStreams()
	if n, err := st.Count(ctx, nil, "Org.Org", SQL{}); err != nil || n != 0 {
		t.Fatalf("after the streams expired: %d %v", n, err)
	}
	api := NewTurso(TursoConfigFromEnv())
	if err := api.Delete(ctx, "blog"); err != nil {
		t.Fatal(err)
	}
	if err := f.Create("blog", "default", "", false); err != nil {
		t.Fatal(err)
	}
	if err := st.Ensure(ctx, []TableDesc{orgDesc}, false); err != nil {
		t.Fatalf("after the database was made again: %v", err)
	}
}

// openTursoApp is openApp on the fake: the shared database migrated to version v, the silos
// children of blog-org-schema (migrated as alder deploy does: MigrateSilos with SchemaOnly).
func openTursoApp(t *testing.T, f *tursofake.Server, path string, v int) (*Stores, StoresConfig) {
	t.Helper()
	cfg := siloConfig(t, SiloDir(path), v)
	cfg.Silos[0].Ref = "Org.Org"
	cfg.Shared = path
	shared, err := OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { shared.Close() })
	ctx := context.Background()
	if _, err := shared.Migrate(ctx, cfg.Tables, cfg.Migrations, false); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	so := cfg
	so.SchemaOnly, so.AllowDataLoss = true, true
	if bad, err := MigrateSilos(ctx, so, &out); err != nil || bad != 0 {
		t.Fatalf("schema database: %d %v\n%s", bad, err, out.String())
	}
	s, err := OpenStores(shared, path, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s, cfg
}

func statusLines(t *testing.T, cfg StoresConfig) map[string]SiloStatus {
	t.Helper()
	var out bytes.Buffer
	if err := StatusSilos(context.Background(), cfg, &out); err != nil {
		t.Fatal(err)
	}
	got := map[string]SiloStatus{}
	for _, l := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if l == "" {
			continue
		}
		var s SiloStatus
		if err := json.Unmarshal([]byte(l), &s); err != nil {
			t.Fatalf("%q: %v", l, err)
		}
		got[s.ID] = s
	}
	return got
}

// Silos on Turso: a child database per org, created by the runtime on first use under the
// schema database (blog-org-schema), with the schema database's tables and the _alder_schema
// row; each org sees its own rows; a migration of the schema database reaches the children,
// a held child (a straggler) is `migrating` (maintenance) until its change arrives, then ok;
// an orphaned child is listed and pruned; the plan is verified on the largest children.
func TestTurso_Silos(t *testing.T) {
	f := tursofake.New(t, "acme")
	path := tursoEnv(t, f)
	s, cfg := openTursoApp(t, f, path, 1)
	ctx := context.Background()
	for _, org := range []string{"A", "B"} {
		if _, err := s.shared.Insert(ctx, nil, "Org.Org", orgDesc.Columns, "id", "", "", []any{nil, org}, clock); err != nil {
			t.Fatal(err)
		}
	}
	// ids are what the rows got; insert projects under each org's id by name
	ids := map[string]string{}
	rows, _ := s.shared.Query(ctx, nil, "Org.Org", orgDesc.Columns, SQL{})
	for _, r := range rows {
		ids[r[1].(string)] = r[0].(string)
	}
	insert := func(org, name string) {
		t.Helper()
		env := siloEnv(s, ids[org])
		Narrow(env, "org", ids[org])
		if err := Enter(env, "org"); err != nil {
			t.Fatal(err)
		}
		task := eff.Call("Db.insertScoped Project", true, func(e *eff.Env) (string, error) {
			return InsertScopedRow(e.Ctx, e.Store.(Store), TxOf(e), projT, []any{nil, name, nil}, clock)
		})
		if _, err := eff.Run(env, eff.KTask, task); err != nil {
			t.Fatal(err)
		}
	}
	insert("A", "pa")
	insert("A", "pa2")
	insert("B", "pb")
	childA, childB := ChildName("blog", "org", ids["A"]), ChildName("blog", "org", ids["B"])
	if v, ok := ChildValue("blog", "org", childA); !ok || v != ids["A"] {
		t.Fatalf("ChildValue(%s) = %q %v", childA, v, ok)
	}
	names := strings.Join(f.Names(), " ")
	if !strings.Contains(names, "blog-org-schema") || !strings.Contains(names, childA) || !strings.Contains(names, childB) {
		t.Fatalf("databases: %s", names)
	}
	var n int
	f.DB(childA).QueryRow(`SELECT COUNT(*) FROM "Project__Project"`).Scan(&n)
	if n != 2 {
		t.Fatalf("A's child holds %d", n)
	}
	f.DB(childB).QueryRow(`SELECT COUNT(*) FROM "Project__Project" WHERE "name" LIKE 'pa%'`).Scan(&n)
	if n != 0 {
		t.Fatalf("B's child holds A's rows")
	}
	f.DB("blog-org-schema").QueryRow(`SELECT COUNT(*) FROM "Project__Project"`).Scan(&n)
	if n != 0 {
		t.Fatalf("the schema database holds rows")
	}
	var hash, mig string
	if err := f.DB(childA).QueryRow(`SELECT "hash", "migration" FROM "_alder_schema"`).Scan(&hash, &mig); err != nil || hash != s.hash || mig != "0001_initial" {
		t.Fatalf("A's _alder_schema: %q %q %v", hash, mig, err)
	}
	if st := statusLines(t, cfg); st[ids["A"]].State != SiloOK || st[ids["B"]].State != SiloOK || len(st) != 2 {
		t.Fatalf("status: %v", st)
	}

	// version 2 (adds a nullable column): verified on the largest children first, then the
	// schema database migrated; B is a straggler
	cfg2 := siloConfig(t, SiloDir(path), 2)
	cfg2.Silos[0].Ref, cfg2.Shared = "Org.Org", path
	var out bytes.Buffer
	if bad, err := VerifySilos(ctx, cfg2, 1, &out); err != nil || bad != 0 || !strings.Contains(out.String(), "org/"+ids["A"]+" (database "+childA+"): ok") || strings.Contains(out.String(), ids["B"]) {
		t.Fatalf("verify (sample 1: the largest, A): %d %v\n%s", bad, err, out.String())
	}
	// a child changed by hand fails the plan, named; nothing is migrated by verifying
	if err := f.Exec(childB, `CREATE TABLE "Stray" ("x" TEXT)`); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if bad, err := VerifySilos(ctx, cfg2, 5, &out); err != nil || bad != 1 || !strings.Contains(out.String(), "org/"+ids["B"]+" (database "+childB+"): FAIL: its tables") {
		t.Fatalf("verify with a changed child: %d %v\n%s", bad, err, out.String())
	}
	f.Exec(childB, `DROP TABLE "Stray"`)
	f.Hold(childB)
	sharedV2, _ := OpenSQLite(path)
	defer sharedV2.Close()
	if _, err := sharedV2.Migrate(ctx, cfg2.Tables, cfg2.Migrations, false); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	so := cfg2
	so.SchemaOnly = true
	if bad, err := MigrateSilos(ctx, so, &out); err != nil || bad != 0 || strings.TrimSpace(out.String()) != "org (schema database blog-org-schema): migrated 0002_note" {
		t.Fatalf("migrate the schema database: %d %v\n%s", bad, err, out.String())
	}
	st := statusLines(t, cfg2)
	if st[ids["A"]].State != SiloOK || st[ids["B"]].State != SiloMigrating {
		t.Fatalf("after the switch: %v", st)
	}
	s2, err := OpenStores(sharedV2, path, cfg2)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	if _, err := s2.silo(ctx, "org", ids["A"]); err != nil {
		t.Fatalf("A on v2: %v", err)
	}
	var me *MaintenanceError
	if _, err := s2.silo(ctx, "org", ids["B"]); !errors.As(err, &me) || me.State != SiloMigrating {
		t.Fatalf("B, a straggler, on v2: %v", err)
	}
	if err := f.Release(childB); err != nil {
		t.Fatal(err)
	}
	if st := statusLines(t, cfg2); st[ids["B"]].State != SiloOK {
		t.Fatalf("B after its change arrived: %v", st)
	}
	out.Reset()
	if bad, err := MigrateSilos(ctx, cfg2, &out); err != nil || bad != 0 || !strings.Contains(out.String(), "org/"+ids["B"]+": took this build's schema from the schema database") {
		t.Fatalf("migrate silos: %d %v\n%s", bad, err, out.String())
	}

	// ahead: a newer build's row
	f.Exec(childA, `UPDATE "_alder_schema" SET "hash" = 'f00d', "migration" = '9999_future'`)
	if st := statusLines(t, cfg2); st[ids["A"]].State != SiloAhead || !strings.Contains(st[ids["A"]].Detail, "alder restore <env> --silo org:<id> --to <time>") {
		t.Fatalf("ahead: %v", st)
	}

	// an orphan: B's Org row deleted; listed, then pruned (A is kept: not an orphan)
	if _, err := sharedV2.DeleteWhere(ctx, nil, "Org.Org", "id", ids["B"], "", nil); err != nil {
		t.Fatal(err)
	}
	if st := statusLines(t, cfg2); st[ids["B"]].State != SiloOrphaned || !strings.Contains(st[ids["B"]].Detail, "no Org row has this id any more") {
		t.Fatalf("orphan: %v", st)
	}
	out.Reset()
	kept, err := PruneSilos(ctx, cfg2, []string{"org:" + ids["B"], "org:" + ids["A"]}, &out)
	if err != nil || kept != 1 || !strings.Contains(out.String(), "org/"+ids["B"]+": removed") || !strings.Contains(out.String(), "org/"+ids["A"]+": kept: its key row exists") {
		t.Fatalf("prune: %d %v\n%s", kept, err, out.String())
	}
	if strings.Contains(strings.Join(f.Names(), " "), childB) {
		t.Fatal("B's child database was not deleted")
	}
}

// A migration that rebuilds a siloed table (or copies rows) cannot reach the children through
// the schema database: refused before anything is migrated.
func TestTurso_RebuildRefused(t *testing.T) {
	f := tursofake.New(t, "acme")
	path := tursoEnv(t, f)
	_, cfg := openTursoApp(t, f, path, 1)
	cfg.Migrations = append(cfg.Migrations, Migration{Name: "0002_rebuild", SQL: `CREATE TABLE "Project__Project__alder__new" ("id" TEXT NOT NULL PRIMARY KEY) STRICT;
INSERT INTO "Project__Project__alder__new" ("id") SELECT "id" FROM "Project__Project";
DROP TABLE "Project__Project";
ALTER TABLE "Project__Project__alder__new" RENAME TO "Project__Project";`})
	cfg.Tables = []TableDesc{orgDesc, projDesc([]Column{{Name: "id", Type: CText}})}
	var out bytes.Buffer
	bad, err := VerifySilos(context.Background(), cfg, 5, &out)
	if err != nil || bad != 1 || !strings.Contains(out.String(), "org: FAIL: migration 0002_rebuild copies or changes rows of a table in silo org") {
		t.Fatalf("%d %v\n%s", bad, err, out.String())
	}
}

// The deploy lock of a database on Turso: a row; a second holder is refused until it is
// released or expires.
func TestTurso_Lock(t *testing.T) {
	f := tursofake.New(t, "acme")
	st, err := OpenSQLite(tursoEnv(t, f))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	now := time.Date(2026, 10, 17, 10, 0, 0, 0, time.UTC)
	if held, err := TakeLock(ctx, st, "me", time.Hour, now); err != nil || held != "" {
		t.Fatalf("%q %v", held, err)
	}
	if held, err := TakeLock(ctx, st, "you", time.Hour, now.Add(time.Minute)); err != nil || !strings.HasPrefix(held, "me until ") {
		t.Fatalf("%q %v", held, err)
	}
	if held, _ := TakeLock(ctx, st, "you", time.Hour, now.Add(2*time.Hour)); held != "" {
		t.Fatalf("an expired lock was not taken over: %q", held)
	}
	ReleaseLock(ctx, st, "you")
	if held, _ := TakeLock(ctx, st, "me", time.Hour, now); held != "" {
		t.Fatalf("after release: %q", held)
	}
	// the lock table is not an app table
	if tables, _ := appTables(ctx, st.db); len(tables) != 0 {
		t.Fatalf("app tables %v", tables)
	}
}
