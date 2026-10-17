package db

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
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
	if err := f.Create("blog", "default"); err != nil {
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
	if err := f.Create("blog", "default"); err != nil {
		t.Fatal(err)
	}
	if err := st.Ensure(ctx, []TableDesc{orgDesc}, false); err != nil {
		t.Fatalf("after the database was made again: %v", err)
	}
}

// openTursoApp is openApp on the fake: the shared database migrated to version v, the silos
// databases of their own at Turso.
func openTursoApp(t *testing.T, path string, v int) (*Stores, StoresConfig) {
	t.Helper()
	cfg := tursoConfig(t, path, v)
	shared, err := OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { shared.Close() })
	if _, err := shared.Migrate(context.Background(), cfg.Tables, cfg.Migrations, false); err != nil {
		t.Fatal(err)
	}
	s, err := OpenStores(shared, path, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s, cfg
}

func tursoConfig(t *testing.T, path string, v int) StoresConfig {
	cfg := siloConfig(t, SiloDir(path), v)
	cfg.Silos[0].Ref, cfg.Shared = "Org.Org", path
	return cfg
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

// Silos on Turso: a database of its own per org, created by the runtime through the API on
// first use (plain: no schema database) and made at the build's schema, with its _alder_schema
// row; each org sees its own rows. A later build finds the databases behind: it migrates one
// on open (a migration that drops no data), or lists it `migrating` until something opens it
// or `alder migrate silos` (MigrateSilos) migrates every one through the API's list; a
// migration that drops data is `pending-data-loss` (maintenance) until --allow-data-loss. The
// plan is verified on the largest databases (by the API's usage, the first by name when it
// gives none), a database changed by hand failing by name; ahead, orphaned and pruned.
func TestTurso_Silos(t *testing.T) {
	f := tursofake.New(t, "acme")
	path := tursoEnv(t, f)
	s, cfg := openTursoApp(t, path, 1)
	ctx := context.Background()
	for _, org := range []string{"A", "B", "C"} {
		if _, err := s.shared.Insert(ctx, nil, "Org.Org", orgDesc.Columns, "id", "", "", []any{nil, org}, clock); err != nil {
			t.Fatal(err)
		}
	}
	ids := map[string]string{}
	rows, _ := s.shared.Query(ctx, nil, "Org.Org", orgDesc.Columns, SQL{})
	for _, r := range rows {
		ids[r[1].(string)] = r[0].(string)
	}
	insert := func(s *Stores, org, name string) {
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
	insert(s, "A", "pa")
	insert(s, "A", "pa2")
	insert(s, "B", "pb")
	insert(s, "C", "pc")
	child := map[string]string{}
	for _, o := range []string{"A", "B", "C"} {
		child[o] = ChildName("blog", "org", ids[o])
	}
	if v, ok := ChildValue("blog", "org", child["A"]); !ok || v != ids["A"] {
		t.Fatalf("ChildValue(%s) = %q %v", child["A"], v, ok)
	}
	if names := strings.Join(f.Names(), " "); strings.Contains(names, "schema") || !strings.Contains(names, child["A"]) || !strings.Contains(names, child["B"]) {
		t.Fatalf("databases: %s", names)
	}
	var n int
	f.DB(child["A"]).QueryRow(`SELECT COUNT(*) FROM "Project__Project"`).Scan(&n)
	if n != 2 {
		t.Fatalf("A's database holds %d", n)
	}
	f.DB(child["B"]).QueryRow(`SELECT COUNT(*) FROM "Project__Project" WHERE "name" LIKE 'pa%'`).Scan(&n)
	if n != 0 {
		t.Fatalf("B's database holds A's rows")
	}
	var hash, mig string
	if err := f.DB(child["A"]).QueryRow(`SELECT "hash", "migration" FROM "_alder_schema"`).Scan(&hash, &mig); err != nil || hash != s.hash || mig != "0001_initial" {
		t.Fatalf("A's _alder_schema: %q %q %v", hash, mig, err)
	}
	if st := statusLines(t, cfg); len(st) != 3 || st[ids["A"]].State != SiloOK {
		t.Fatalf("status: %v", st)
	}

	// version 2 (adds a column): verified on the largest first (A); a database changed by
	// hand fails, named; nothing is migrated by verifying
	cfg2 := tursoConfig(t, path, 2)
	var out bytes.Buffer
	if bad, err := VerifySilos(ctx, cfg2, 1, &out); err != nil || bad != 0 || !strings.Contains(out.String(), "org/"+ids["A"]+" (database "+child["A"]+"): ok") || strings.Count(out.String(), "\n") != 1 {
		t.Fatalf("verify (sample 1: the largest, A): %d %v\n%s", bad, err, out.String())
	}
	f.Exec(child["B"], `CREATE TABLE "Stray" ("x" TEXT)`)
	out.Reset()
	if bad, err := VerifySilos(ctx, cfg2, 5, &out); err != nil || bad != 1 || !strings.Contains(out.String(), "org/"+ids["B"]+" (database "+child["B"]+"): FAIL: after the migrations its tables") {
		t.Fatalf("verify with a changed database: %d %v\n%s", bad, err, out.String())
	}
	f.Exec(child["B"], `DROP TABLE "Stray"`)
	// without usage sizes: the first by name
	for _, c := range child {
		f.NoSize(c)
	}
	out.Reset()
	if bad, err := VerifySilos(ctx, cfg2, 1, &out); err != nil || bad != 0 || strings.Count(out.String(), "\n") != 1 {
		t.Fatalf("verify without sizes: %d %v\n%s", bad, err, out.String())
	}

	// v2 serves: every silo is behind (migrating) until opened; A migrates on open
	sharedV2, _ := OpenSQLite(path)
	defer sharedV2.Close()
	if _, err := sharedV2.Migrate(ctx, cfg2.Tables, cfg2.Migrations, false); err != nil {
		t.Fatal(err)
	}
	if st := statusLines(t, cfg2); st[ids["A"]].State != SiloMigrating || st[ids["B"]].State != SiloMigrating {
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
	if st := statusLines(t, cfg2); st[ids["A"]].State != SiloOK || st[ids["B"]].State != SiloMigrating {
		t.Fatalf("after A's open: %v", st)
	}
	// alder migrate silos: every remaining one, through the API's list
	out.Reset()
	if bad, err := MigrateSilos(ctx, cfg2, &out); err != nil || bad != 0 || !strings.Contains(out.String(), "org/"+ids["B"]+": migrated 0002_note") || !strings.Contains(out.String(), "org/"+ids["A"]+": ok") {
		t.Fatalf("migrate silos: %d %v\n%s", bad, err, out.String())
	}

	// v3 drops a column: pending-data-loss (maintenance) until --allow-data-loss
	cfg3 := tursoConfig(t, path, 3)
	cfg3.Migrations = append(siloMigrations(t, 2), Migration{Name: "0003_drop_name", SQL: `ALTER TABLE "Project__Project" DROP COLUMN "name";`, DropsData: []string{"column Project.name"}})
	cfg3.Tables = []TableDesc{orgDesc, projDesc(append([]Column{{Name: "id", Type: CText}}, projCols2[2:]...))}
	sc3, err := schemaOf(cfg3.Tables)
	if err != nil {
		t.Fatal(err)
	}
	cfg3.Migrations[2].From, cfg3.Migrations[2].To = cfg3.Migrations[1].To, short(sc3.hash)
	sharedV3, _ := OpenSQLite(path)
	defer sharedV3.Close()
	os.Setenv(EnvAllowDataLoss, "1")
	_, err = sharedV3.Migrate(ctx, cfg3.Tables, cfg3.Migrations, false)
	os.Unsetenv(EnvAllowDataLoss)
	if err != nil {
		t.Fatal(err)
	}
	s3, err := OpenStores(sharedV3, path, cfg3)
	if err != nil {
		t.Fatal(err)
	}
	defer s3.Close()
	var me *MaintenanceError
	if _, err := s3.silo(ctx, "org", ids["C"]); !errors.As(err, &me) || me.State != SiloDataLoss {
		t.Fatalf("C on v3: %v", err)
	}
	out.Reset()
	if bad, err := MigrateSilos(ctx, cfg3, &out); err != nil || bad != 3 {
		t.Fatalf("migrate silos without --allow-data-loss: %d %v\n%s", bad, err, out.String())
	}
	cfg3.AllowDataLoss = true
	out.Reset()
	if bad, err := MigrateSilos(ctx, cfg3, &out); err != nil || bad != 0 || !strings.Contains(out.String(), "org/"+ids["C"]+": migrated 0003_drop_name") {
		t.Fatalf("migrate silos --allow-data-loss: %d %v\n%s", bad, err, out.String())
	}
	cfg3.AllowDataLoss = false

	// ahead: a newer build's row
	f.Exec(child["A"], `UPDATE "_alder_schema" SET "hash" = 'f00d', "migration" = '9999_future'`)
	if st := statusLines(t, cfg3); st[ids["A"]].State != SiloAhead || !strings.Contains(st[ids["A"]].Detail, "alder restore <env> --silo org:<id> --to <time>") {
		t.Fatalf("ahead: %v", st)
	}

	// an orphan: B's Org row deleted; listed, then pruned (A is kept: not an orphan)
	if _, err := sharedV3.DeleteWhere(ctx, nil, "Org.Org", "id", ids["B"], "", nil); err != nil {
		t.Fatal(err)
	}
	if st := statusLines(t, cfg3); st[ids["B"]].State != SiloOrphaned || !strings.Contains(st[ids["B"]].Detail, "no Org row has this id any more") {
		t.Fatalf("orphan: %v", st)
	}
	out.Reset()
	kept, err := PruneSilos(ctx, cfg3, []string{"org:" + ids["B"], "org:" + ids["A"]}, &out)
	if err != nil || kept != 1 || !strings.Contains(out.String(), "org/"+ids["B"]+": removed") || !strings.Contains(out.String(), "org/"+ids["A"]+": kept: its key row exists") {
		t.Fatalf("prune: %d %v\n%s", kept, err, out.String())
	}
	if strings.Contains(strings.Join(f.Names(), " "), child["B"]) {
		t.Fatal("B's database was not deleted")
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

// A silo's database that another process created first ("already exists") is not an error:
// child opens it.
func TestTurso_CreateExists(t *testing.T) {
	f := tursofake.New(t, "acme")
	tursoEnv(t, f)
	tc := NewTurso(TursoConfigFromEnv())
	name := ChildName("blog", "org", "x")
	if err := f.Create(name, "default"); err != nil {
		t.Fatal(err)
	}
	_, err := tc.Create(context.Background(), CreateOptions{Name: name})
	if !IsTursoExists(err) {
		t.Fatalf("a second create: %v", err)
	}
	if IsTursoExists(&TursoError{Status: 500, Message: "boom"}) || IsTursoExists(nil) {
		t.Fatal("not an exists")
	}
}
