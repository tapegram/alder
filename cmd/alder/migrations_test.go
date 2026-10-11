package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"alder/internal/buildkit"
	_ "alder/rt/db" // the sqlite driver
)

// tempApp copies the app at src (relative to this package) to <tmp>/<name>, makes tmp the
// working directory (builds land there) and returns name.
func tempApp(t *testing.T, src, name string) string {
	t.Helper()
	abs, err := filepath.Abs(src)
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	copyDir(t, abs, filepath.Join(tmp, name))
	t.Chdir(tmp)
	return name
}

// E0019: an app whose tables changed since its last migration is refused by check (text and
// --json), build, test and eval alike.
func TestCheck_E0019(t *testing.T) {
	dir := tempApp(t, "../../testdata/drift/E0019-schema-changed", "posts")
	const want = "migrations/schema.json:1:1: error[E0019]: the schema changed since migration 0001 (table Post: column body): run `alder migrate`\n"
	code, out, _ := run(t, dir)
	if code != 1 || out != want {
		t.Fatalf("check: exit %d\n%s", code, out)
	}
	code, out, _ = run(t, dir, "--json")
	if code != 1 || !strings.HasPrefix(out, `{"code":"E0019","severity":"error","file":"migrations/schema.json","line":1,"col":1,`) {
		t.Fatalf("check --json: exit %d\n%s", code, out)
	}
	code, out, errs := runBuild(t, "--emit", dir)
	if code != 1 || !strings.Contains(out, "error[E0019]") {
		t.Fatalf("build: exit %d\n%s%s", code, out, errs)
	}
	if _, err := os.Stat(filepath.Join("build", "posts")); err == nil {
		t.Fatal("build wrote the module of a refused app")
	}
	code, out, errs = runTest(t, dir)
	if code != 1 || !strings.Contains(out+errs, "error[E0019]") {
		t.Fatalf("test: exit %d\n%s%s", code, out, errs)
	}
	code, out, errs = runEval(t, "", dir, "1 + 1")
	if code != 1 || out != "" || !strings.Contains(errs, "error[E0019]") {
		t.Fatalf("eval: exit %d\n%s%s", code, out, errs)
	}
}

// An app with migrations: its tests run on the tables the migrations make, and fail, every
// one, when the migrations do not make the app's tables; eval --run creates the dev database
// through them; the build embeds them.
func TestMigrations_TestEvalBuild(t *testing.T) {
	needGoTool(t)
	dir := tempApp(t, "../../testdata/golden/gen_migrations", "notes")
	note := filepath.Join(dir, "app", "Note.alder")
	b, err := os.ReadFile(note)
	if err != nil {
		t.Fatal(err)
	}
	tests := `
test "a note is stored" = {
  let _ <- Db.insert Note { title: "hello" }
  let n <- Db.from Note |> Db.count
  Task.succeed (n == 1)
}

test "pure" = 1 + 1 == 2
`
	if err := os.WriteFile(note, append(b, tests...), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, errs := runTest(t, dir)
	if code != 0 || !strings.Contains(out, "2 passed, 0 failed") {
		t.Fatalf("test: exit %d\n%s%s", code, out, errs)
	}
	code, out, errs = runEval(t, "", dir, "--run", "Db.from Note |> Db.count")
	if code != 0 || out != "0 : Int\n" {
		t.Fatalf("eval --run: exit %d\n%s%s", code, out, errs)
	}
	devDB, err := sql.Open("sqlite", filepath.Join("build", "notes", "dev.db"))
	if err != nil {
		t.Fatal(err)
	}
	var recorded string
	err = devDB.QueryRow(`SELECT group_concat("name") FROM "_alder_migrations"`).Scan(&recorded)
	devDB.Close()
	if err != nil || recorded != "0001_initial" {
		t.Fatalf("eval --run did not create the dev database by migrations: %q %v", recorded, err)
	}
	code, out, errs = runBuild(t, "--emit", dir)
	if code != 0 {
		t.Fatalf("build: exit %d\n%s%s", code, out, errs)
	}
	if _, err := os.Stat(filepath.Join("build", "notes", "migrations", "0001_initial.sql")); err != nil {
		t.Fatalf("build did not copy the migrations: %v", err)
	}
	if _, err := os.Stat(filepath.Join("build", "notes", "migrations", "schema.json")); err == nil {
		t.Fatal("build copied schema.json, which the binary does not use")
	}

	// a migration that no longer makes the tables the snapshot (and the app) says
	sql := filepath.Join(dir, "migrations", "0001_initial.sql")
	s, err := os.ReadFile(sql)
	if err != nil {
		t.Fatal(err)
	}
	broken := strings.Replace(string(s), `, "title" TEXT NOT NULL`, "", 1)
	if broken == string(s) {
		t.Fatal("the migration lacks the title column")
	}
	if err := os.WriteFile(sql, []byte(broken), 0o644); err != nil {
		t.Fatal(err)
	}
	if !buildkit.HasMigrations(dir) {
		t.Fatal("HasMigrations")
	}
	code, out, errs = runTest(t, dir)
	if code != 1 || strings.Count(out, "the migrations do not produce the app's schema: run `alder migrate`") != 2 || !strings.Contains(out, "0 passed, 2 failed") {
		t.Fatalf("test with a broken migration: exit %d\n%s%s", code, out, errs)
	}
}

// alder run rebuilds when `alder migrate` writes migrations/ (so E0019's overlay clears).
func TestWatch_Migrations(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "migrations"), 0o755)
	before := snapshot(dir)
	os.WriteFile(filepath.Join(dir, "migrations", "0001_initial.sql"), []byte("-- alder migration 0001_initial\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "migrations", "schema.json"), []byte("{}"), 0o644)
	os.WriteFile(filepath.Join(dir, "migrations", "notes.txt"), []byte("x"), 0o644)
	after := snapshot(dir)
	if _, ok := after["migrations/0001_initial.sql"]; !ok || len(after) != 2 {
		t.Fatalf("snapshot %v", after)
	}
	if k := diff(before, after); k != sourceChange {
		t.Fatalf("diff = %v, want a source change", k)
	}
}

// eval --run on a dev database made before the app had migrations: adopted when its tables are
// where a migration leads; otherwise refused with the path to delete.
func TestEval_AdoptsDevDB(t *testing.T) {
	needGoTool(t)
	dir := tempApp(t, "../../testdata/golden/gen_migrations", "notes")
	path := filepath.Join("build", "notes", "dev.db")
	const meta = `CREATE TABLE "_alder_meta" ("key" TEXT NOT NULL PRIMARY KEY, "value" TEXT NOT NULL); INSERT INTO "_alder_meta" VALUES ('format', '1')`
	os.MkdirAll(filepath.Dir(path), 0o755)
	sqlText, err := os.ReadFile(filepath.Join(dir, "migrations", "0001_initial.sql"))
	if err != nil {
		t.Fatal(err)
	}
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range strings.Split(string(sqlText), "\n") {
		if strings.HasSuffix(l, ";") && !strings.HasPrefix(l, "--") {
			if _, err := conn.Exec(l); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := conn.Exec(`INSERT INTO "Note__Note" VALUES ('n1', 'kept', '2026-01-01T00:00:00.000000000Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(meta); err != nil { // as Ensure records it
		t.Fatal(err)
	}
	conn.Close()
	code, out, errs := runEval(t, "", dir, "--run", "Db.from Note |> Db.count")
	if code != 0 || out != "1 : Int\n" {
		t.Fatalf("adopted: exit %d\n%s%s", code, out, errs)
	}

	os.Remove(path)
	conn, _ = sql.Open("sqlite", path)
	if _, err := conn.Exec(`CREATE TABLE "Note__Note" ("id" TEXT NOT NULL PRIMARY KEY) STRICT; ` + meta); err != nil {
		t.Fatal(err)
	}
	conn.Close()
	code, out, errs = runEval(t, "", dir, "--run", "Db.from Note |> Db.count")
	abs, _ := filepath.Abs(path)
	if code != 1 || !strings.Contains(errs, "matches none of them") || !strings.Contains(errs, "delete "+abs+" to start a new one") {
		t.Fatalf("no match: exit %d\n%s%s", code, out, errs)
	}
}
