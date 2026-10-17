package db

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

// The tables of the migration tests: Post (id, title), then with a nullable summary (v2), then
// with a non-null views column filled by a rebuild (v3); Comment references Post.
var (
	postV1   = TableDesc{Name: "Post.Post", ID: "id", Columns: []Column{{Name: "id", Type: CText}, {Name: "title", Type: CText}}}
	postV2   = TableDesc{Name: "Post.Post", ID: "id", Columns: append(slices.Clone(postV1.Columns), Column{Name: "summary", Type: CText, Nullable: true})}
	postV3   = TableDesc{Name: "Post.Post", ID: "id", Columns: append(slices.Clone(postV2.Columns), Column{Name: "views", Type: CInt})}
	commentT = TableDesc{Name: "Comment.Comment", ID: "id", Columns: []Column{{Name: "id", Type: CText}, {Name: "post", Type: CText, Ref: "Post.Post"}}}
)

func create(t *testing.T, d TableDesc, others ...TableDesc) string {
	t.Helper()
	by := map[string]TableDesc{d.Name: d}
	for _, o := range others {
		by[o.Name] = o
	}
	st, err := createStmt(d, by)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

// migrations are the three files taking an empty database to v1, v2 and v3, as `alder migrate`
// writes them (statements one per line, each ending in `;`).
// hashOf is the header hash (12 hex digits) of the schema of tables.
func hashOf(t *testing.T, tables ...TableDesc) string {
	t.Helper()
	sc, err := schemaOf(tables)
	if err != nil {
		t.Fatal(err)
	}
	return short(sc.hash)
}

func migrations(t *testing.T) []Migration {
	newPost := postV3
	newPost.Name = "Post.Post__new"
	empty, v1, v2, v3 := hashOf(t), hashOf(t, postV1, commentT), hashOf(t, postV2, commentT), hashOf(t, postV3, commentT)
	return []Migration{
		{Name: "0001_initial", From: empty, To: v1, SQL: strings.Join([]string{
			create(t, commentT, postV1) + ";",
			create(t, postV1) + ";",
			`CREATE INDEX IF NOT EXISTS "Comment__Comment__post" ON "Comment__Comment"("post");`,
		}, "\n")},
		{Name: "0002_add_summary", From: v1, To: v2, SQL: `ALTER TABLE "Post__Post" ADD COLUMN "summary" TEXT;`},
		{Name: "0003_add_views", From: v2, To: v3, SQL: strings.Join([]string{
			create(t, newPost) + ";",
			`INSERT INTO "Post__Post__new" ("id", "title", "summary", "views") SELECT "id", "title", "summary", 0 FROM "Post__Post";`,
			`DROP TABLE "Post__Post";`,
			`ALTER TABLE "Post__Post__new" RENAME TO "Post__Post";`,
		}, "\n")},
	}
}

func openMem(t *testing.T) *SQLite {
	t.Helper()
	s, err := OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func recordedNames(t *testing.T, s *SQLite) []string {
	t.Helper()
	rows, err := s.db.Query(`SELECT "name" FROM "_alder_migrations" ORDER BY "name"`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var n string
		rows.Scan(&n)
		out = append(out, n)
	}
	return out
}

func exec(t *testing.T, s *SQLite, q string) {
	t.Helper()
	if _, err := s.db.Exec(q); err != nil {
		t.Fatal(err)
	}
}

func tableSQL(t *testing.T, s *SQLite, name string) string {
	t.Helper()
	var st string
	if err := s.db.QueryRow(`SELECT "sql" FROM sqlite_schema WHERE "name" = ?`, name).Scan(&st); err != nil {
		t.Fatal(err)
	}
	return st
}

func TestMigrate_AppliesInOrder(t *testing.T) {
	ctx := context.Background()
	s := openMem(t)
	ms := migrations(t)
	applied, err := s.Migrate(ctx, []TableDesc{postV1, commentT}, ms[:1], false)
	if err != nil || !slices.Equal(applied, []string{"0001_initial"}) {
		t.Fatalf("first: %v %v", applied, err)
	}
	exec(t, s, `INSERT INTO "Post__Post" VALUES ('p1', 'Hello')`)
	exec(t, s, `INSERT INTO "Comment__Comment" VALUES ('c1', 'p1')`)
	tables := []TableDesc{postV3, commentT}
	applied, err = s.Migrate(ctx, tables, ms, false)
	if err != nil || !slices.Equal(applied, []string{"0002_add_summary", "0003_add_views"}) {
		t.Fatalf("second: %v %v", applied, err)
	}
	var title string
	var summary any
	var views int64
	if err := s.db.QueryRow(`SELECT "title", "summary", "views" FROM "Post__Post" WHERE "id" = 'p1'`).Scan(&title, &summary, &views); err != nil {
		t.Fatal(err)
	}
	if title != "Hello" || summary != nil || views != 0 {
		t.Fatalf("row after migrating: %q %v %d", title, summary, views)
	}
	if n := rawCount(t, s, `SELECT COUNT(*) FROM "Comment__Comment"`); n != 1 {
		t.Fatalf("comments: %d", n)
	}
	if got, want := tableSQL(t, s, "Post__Post"), create(t, postV3); got != want {
		t.Fatalf("Post after the rebuild:\n%s\nwant\n%s", got, want)
	}
	sc, _ := schemaOf(tables)
	var hash string
	s.db.QueryRow(`SELECT "value" FROM "_alder_meta" WHERE "key" = 'schema_hash'`).Scan(&hash)
	if hash != sc.hash {
		t.Fatalf("recorded hash %s, want the app's %s", hash, sc.hash)
	}
	// the store knows the tables
	if _, ok, err := s.Get(ctx, nil, "Post.Post", "p1"); !ok || err != nil {
		t.Fatalf("Get after Migrate: %v %v", ok, err)
	}
	// foreign keys are back on
	var fk int
	s.db.QueryRow("PRAGMA foreign_keys").Scan(&fk)
	if fk != 1 {
		t.Fatal("foreign keys left off")
	}
}

func TestMigrate_RecordsAndSkips(t *testing.T) {
	ctx := context.Background()
	path := t.TempDir() + "/app.db"
	s, err := OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	ms := migrations(t)
	tables := []TableDesc{postV3, commentT}
	if applied, err := s.Migrate(ctx, tables, ms, false); err != nil || len(applied) != 3 {
		t.Fatalf("%v %v", applied, err)
	}
	if got := recordedNames(t, s); !slices.Equal(got, []string{"0001_initial", "0002_add_summary", "0003_add_views"}) {
		t.Fatalf("recorded %v", got)
	}
	exec(t, s, `INSERT INTO "Post__Post" VALUES ('p1', 'Hello', NULL, 3)`)
	s.Close()
	s, err = OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if applied, err := s.Migrate(ctx, tables, ms, false); err != nil || len(applied) != 0 {
		t.Fatalf("again: %v %v", applied, err)
	}
	if n := rawCount(t, s, `SELECT COUNT(*) FROM "Post__Post"`); n != 1 {
		t.Fatalf("rows: %d", n)
	}
}

func TestMigrate_FailureRollsBack(t *testing.T) {
	ctx := context.Background()
	s := openMem(t)
	ms := migrations(t)
	bad := Migration{Name: "0002_bad", From: ms[0].To, To: ms[1].To, SQL: `ALTER TABLE "Post__Post" ADD COLUMN "summary" TEXT;` + "\n" + `INSERT INTO "Nope" VALUES (1);`}
	applied, err := s.Migrate(ctx, []TableDesc{postV2, commentT}, []Migration{ms[0], bad}, false)
	if err == nil || !strings.HasPrefix(err.Error(), "migration 0002_bad failed: ") || !strings.Contains(err.Error(), "Nope") {
		t.Fatalf("err = %v", err)
	}
	if !slices.Equal(applied, []string{"0001_initial"}) {
		t.Fatalf("applied %v", applied)
	}
	if got := recordedNames(t, s); !slices.Equal(got, []string{"0001_initial"}) {
		t.Fatalf("recorded %v", got)
	}
	if got := tableSQL(t, s, "Post__Post"); got != create(t, postV1) {
		t.Fatalf("the failed migration's ADD COLUMN stayed: %s", got)
	}
	// fixed, it applies over the state 0001 left (whose hash was recorded with it)
	if applied, err := s.Migrate(ctx, []TableDesc{postV2, commentT}, ms[:2], false); err != nil || !slices.Equal(applied, []string{"0002_add_summary"}) {
		t.Fatalf("after the fix: %v %v", applied, err)
	}
}

func TestMigrate_GapRefused(t *testing.T) {
	ctx := context.Background()
	s := openMem(t)
	ms := migrations(t)
	if _, err := s.Migrate(ctx, []TableDesc{postV2, commentT}, ms[:2], false); err != nil {
		t.Fatal(err)
	}
	for _, dev := range []bool{false, true} {
		_, err := s.Migrate(ctx, []TableDesc{postV3, commentT}, []Migration{ms[0], ms[2]}, dev)
		if err == nil || err.Error() != "migration 0002_add_summary is recorded as applied but its file is missing: restore it from version control" {
			t.Fatalf("dev=%v: err = %v", dev, err)
		}
	}
	if got := recordedNames(t, s); !slices.Equal(got, []string{"0001_initial", "0002_add_summary"}) {
		t.Fatalf("0003 applied over the gap: %v", got)
	}
	// a file numbered before one already applied is refused too
	late := Migration{Name: "0001_z_late", SQL: `SELECT 1;`}
	_, err := s.Migrate(ctx, []TableDesc{postV2, commentT}, []Migration{ms[0], late, ms[1]}, false)
	if err == nil || !strings.Contains(err.Error(), "migration 0001_z_late is not applied but 0002_add_summary") {
		t.Fatalf("out of order: %v", err)
	}
}

func TestMigrate_AdoptsEnsuredDB(t *testing.T) {
	ctx := context.Background()
	s := openMem(t)
	tables := []TableDesc{postV1, commentT}
	if err := s.Ensure(ctx, tables, true); err != nil {
		t.Fatal(err)
	}
	exec(t, s, `INSERT INTO "Post__Post" VALUES ('p1', 'Hello')`)
	// its tables are where 0001 leads: 0001 is recorded without running, 0002 applies, data kept
	var logs []string
	applied, err := s.MigrateLog(ctx, []TableDesc{postV2, commentT}, migrations(t)[:2], false, func(m string) { logs = append(logs, m) })
	if err != nil || !slices.Equal(applied, []string{"0002_add_summary"}) {
		t.Fatalf("prod: %v %v", applied, err)
	}
	if !slices.Equal(logs, []string{"adopted the database at 0001_initial", "migrated 0002_add_summary"}) {
		t.Fatalf("logs %q", logs)
	}
	if n := rawCount(t, s, `SELECT COUNT(*) FROM "Post__Post"`); n != 1 {
		t.Fatalf("adopting lost data: %d rows", n)
	}
	if got := recordedNames(t, s); !slices.Equal(got, []string{"0001_initial", "0002_add_summary"}) {
		t.Fatalf("recorded %v", got)
	}
	if applied, err := s.Migrate(ctx, []TableDesc{postV2, commentT}, migrations(t)[:2], false); err != nil || len(applied) != 0 {
		t.Fatalf("after adopting: %v %v", applied, err)
	}
}

// A database not made by migrations whose tables no migration leads to: refused in prod (no
// dev-mode advice), recreated in dev.
func TestMigrate_AdoptionMatchesNone(t *testing.T) {
	ctx := context.Background()
	s := openMem(t)
	odd := TableDesc{Name: "Post.Post", ID: "id", Columns: []Column{{Name: "id", Type: CText}, {Name: "other", Type: CInt}}}
	if err := s.Ensure(ctx, []TableDesc{odd, commentT}, true); err != nil {
		t.Fatal(err)
	}
	exec(t, s, `INSERT INTO "Post__Post" VALUES ('p1', 1)`)
	tables := []TableDesc{postV1, commentT}
	ms := migrations(t)[:1]
	_, err := s.Migrate(ctx, tables, ms, false)
	if !errors.Is(err, ErrNotMigrated) || err.Error() != "this database was not created by migrations and matches none of them: if the app made it before it had migrations, generate 0001 from the deployed version (check out that commit, run `alder migrate`), then migrate forward; otherwise back it up; to start over, run in dev mode (ALDER_DEV=1 recreates the database empty: every row is deleted)" {
		t.Fatalf("prod: %v", err)
	}
	if n := rawCount(t, s, `SELECT COUNT(*) FROM "Post__Post"`); n != 1 {
		t.Fatal("the refusal touched data")
	}
	var logs []string
	applied, err := s.MigrateLog(ctx, tables, ms, true, func(m string) { logs = append(logs, m) })
	if err != nil || len(applied) != 0 {
		t.Fatalf("dev: %v %v", applied, err)
	}
	if !slices.Equal(logs, []string{"recreated the database (dev mode): it was not created by migrations and matches none of them"}) {
		t.Fatalf("logs %q", logs)
	}
	if got := recordedNames(t, s); !slices.Equal(got, []string{"0001_initial"}) {
		t.Fatalf("recorded %v", got)
	}
	if n := rawCount(t, s, `SELECT COUNT(*) FROM sqlite_schema WHERE "type" = 'index' AND "name" = 'Comment__Comment__post'`); n != 1 {
		t.Fatal("the recreated database lacks the @ref index")
	}
	if applied, err := s.Migrate(ctx, tables, ms, false); err != nil || len(applied) != 0 {
		t.Fatalf("after recreating: %v %v", applied, err)
	}
}

// A file missing from the numbering that was never applied: 0001 applied, 0002 deleted, 0003
// present. The chain (0003 expects 0002's schema) refuses before anything runs.
func TestUnrecordedNumberGap(t *testing.T) {
	ctx := context.Background()
	s := openMem(t)
	ms := migrations(t)
	if _, err := s.Migrate(ctx, []TableDesc{postV1, commentT}, ms[:1], false); err != nil {
		t.Fatal(err)
	}
	for _, dev := range []bool{false, true} {
		applied, err := s.Migrate(ctx, []TableDesc{postV3, commentT}, []Migration{ms[0], ms[2]}, dev)
		want := "migration 0003_add_views expects the schema " + ms[1].To + " but the database is at " + ms[0].To + " (is a file missing, or was the database changed outside migrations?)"
		if err == nil || err.Error() != want || len(applied) != 0 {
			t.Fatalf("dev=%v: %v %v\nwant %s", dev, applied, err, want)
		}
	}
	if got := recordedNames(t, s); !slices.Equal(got, []string{"0001_initial"}) {
		t.Fatalf("recorded %v", got)
	}
	if got := tableSQL(t, s, "Post__Post"); got != create(t, postV1) {
		t.Fatalf("0003 ran: %s", got)
	}
	// restored, everything applies
	if applied, err := s.Migrate(ctx, []TableDesc{postV3, commentT}, ms, false); err != nil || len(applied) != 2 {
		t.Fatalf("restored: %v %v", applied, err)
	}
}

// A migration another process recorded first (two starting at once) is skipped, not re-run.
func TestMigrate_RecordedMeanwhileSkipped(t *testing.T) {
	ctx := context.Background()
	s := openMem(t)
	ms := migrations(t)
	if _, err := s.Migrate(ctx, []TableDesc{postV1, commentT}, ms[:1], false); err != nil {
		t.Fatal(err)
	}
	conn, err := s.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, `INSERT INTO "_alder_migrations" VALUES ('0002_add_summary', 'x')`); err != nil {
		t.Fatal(err)
	}
	ran, err := applyMigration(ctx, conn, ms[1])
	if err != nil || ran {
		t.Fatalf("ran=%v err=%v", ran, err)
	}
	conn.Close()
	if got := tableSQL(t, s, "Post__Post"); got != create(t, postV1) {
		t.Fatalf("the skipped migration ran: %s", got)
	}
}

func TestMigrate_DriftRefused(t *testing.T) {
	ctx := context.Background()
	s := openMem(t)
	ms := migrations(t)
	if _, err := s.Migrate(ctx, []TableDesc{postV1, commentT}, ms[:1], false); err != nil {
		t.Fatal(err)
	}
	exec(t, s, `INSERT INTO "Post__Post" VALUES ('p1', 'Hello')`)
	exec(t, s, `ALTER TABLE "Post__Post" ADD COLUMN "extra" TEXT`)
	handMade := tableSQL(t, s, "Post__Post")
	for _, upTo := range []int{1, 2} { // with nothing to apply, and with 0002 pending
		tables := []TableDesc{postV1, commentT}
		if upTo == 2 {
			tables[0] = postV2
		}
		applied, err := s.Migrate(ctx, tables, ms[:upTo], false)
		if !errors.Is(err, ErrDrift) || !strings.Contains(err.Error(), "changed tables: Post)") || len(applied) != 0 {
			t.Fatalf("upTo %d: %v %v", upTo, applied, err)
		}
		if !strings.Contains(err.Error(), "changed outside migrations since 0001_initial") {
			t.Fatalf("message: %v", err)
		}
		if tableSQL(t, s, "Post__Post") != handMade || !slices.Equal(recordedNames(t, s), []string{"0001_initial"}) {
			t.Fatal("a refusal changed the database")
		}
	}
	applied, err := s.Migrate(ctx, []TableDesc{postV2, commentT}, ms[:2], true)
	if err != nil || len(applied) != 0 {
		t.Fatalf("dev: %v %v", applied, err)
	}
	if got := tableSQL(t, s, "Post__Post"); got != create(t, postV2) {
		t.Fatalf("dev recreate: %s", got)
	}
	if applied, err := s.Migrate(ctx, []TableDesc{postV2, commentT}, ms[:2], false); err != nil || len(applied) != 0 {
		t.Fatalf("after dev recreate: %v %v", applied, err)
	}
}

// Migrations that do not lead to the app's tables (the app changed, `alder migrate` not run):
// refused after applying, or recreated in dev mode.
func TestMigrate_NotTheAppsSchema(t *testing.T) {
	ctx := context.Background()
	s := openMem(t)
	ms := migrations(t)
	// the last file does not lead to the app's schema: refused before anything runs
	_, err := s.Migrate(ctx, []TableDesc{postV2, commentT}, ms[:1], false)
	want := "db: the database's tables are not the app's schema: 0001_initial leads to the schema " + ms[0].To + ", but the app's is " + ms[1].To + ": the migrations do not produce the app's schema"
	if !errors.Is(err, ErrDrift) || err.Error() != want {
		t.Fatalf("err = %v\nwant %s", err, want)
	}
	if n := rawCount(t, s, `SELECT COUNT(*) FROM sqlite_schema WHERE "name" = 'Post__Post'`); n != 0 {
		t.Fatal("a refused chain ran")
	}
	var logs []string
	if applied, err := s.MigrateLog(ctx, []TableDesc{postV2, commentT}, ms[:1], true, func(m string) { logs = append(logs, m) }); err != nil || len(applied) != 0 {
		t.Fatalf("dev: %v %v", applied, err)
	}
	if !slices.Equal(logs, []string{"recreated the database (dev mode): the migrations do not produce the app's schema"}) {
		t.Fatalf("logs %q", logs)
	}
	if got := tableSQL(t, s, "Post__Post"); got != create(t, postV2) {
		t.Fatalf("dev recreate: %s", got)
	}
	// a file whose statements do not make the schema its header says: rolled back
	s2 := openMem(t)
	lying := ms[0]
	lying.SQL = create(t, postV2) + ";\n" + create(t, commentT, postV2) + ";"
	_, err = s2.Migrate(ctx, []TableDesc{postV1, commentT}, []Migration{lying}, false)
	if err == nil || err.Error() != "migration 0001_initial does not produce the schema its header names (to "+ms[0].To+", got "+ms[1].To+"): the file was edited; restore it from version control" {
		t.Fatalf("lying file: %v", err)
	}
	if n := rawCount(t, s2, `SELECT COUNT(*) FROM sqlite_schema WHERE "name" = 'Post__Post'`); n != 0 {
		t.Fatal("the lying file's tables stayed")
	}
	// the database is at the right schema but not the app's (nothing pending): "the database is at"
	s3 := openMem(t)
	if _, err := s3.Migrate(ctx, []TableDesc{postV1, commentT}, ms[:1], false); err != nil {
		t.Fatal(err)
	}
	_, err = s3.Migrate(ctx, []TableDesc{postV2, commentT}, ms[:1], false)
	want = "db: the database's tables are not the app's schema: the database is at the schema " + ms[0].To + " (after 0001_initial), but the app's is " + ms[1].To + ": the migrations do not produce the app's schema"
	if !errors.Is(err, ErrDrift) || !errors.Is(err, ErrSchemaAhead) || err.Error() != want {
		t.Fatalf("nothing pending: %v\nwant %s", err, want)
	}
}

func TestMigrate_ForeignKeyCheck(t *testing.T) {
	ctx := context.Background()
	s := openMem(t)
	ms := migrations(t)
	tables := []TableDesc{postV1, commentT}
	if _, err := s.Migrate(ctx, tables, ms[:1], false); err != nil {
		t.Fatal(err)
	}
	exec(t, s, `INSERT INTO "Post__Post" VALUES ('p1', 'Hello')`)
	exec(t, s, `INSERT INTO "Comment__Comment" VALUES ('c1', 'p1')`)
	drop := Migration{Name: "0002_drop_posts", From: ms[0].To, To: ms[0].To, SQL: `DELETE FROM "Post__Post";`}
	_, err := s.Migrate(ctx, tables, []Migration{ms[0], drop}, false)
	if err == nil || err.Error() != "migration 0002_drop_posts: rows in Comment refer to rows this migration deletes: delete or re-point them first: add an UPDATE or DELETE at the top of 0002_drop_posts.sql (it was not applied)" {
		t.Fatalf("err = %v", err)
	}
	if n := rawCount(t, s, `SELECT COUNT(*) FROM "Post__Post"`); n != 1 {
		t.Fatal("the failed migration's DELETE stayed")
	}
	if got := recordedNames(t, s); !slices.Equal(got, []string{"0001_initial"}) {
		t.Fatalf("recorded %v", got)
	}
}

func TestEnsure_RefIndexes(t *testing.T) {
	ctx := context.Background()
	s := openMem(t)
	tables := []TableDesc{postV1, commentT}
	if err := s.Ensure(ctx, tables, true); err != nil {
		t.Fatal(err)
	}
	const idx = `SELECT COUNT(*) FROM sqlite_schema WHERE "type" = 'index' AND "name" = 'Comment__Comment__post' AND "tbl_name" = 'Comment__Comment'`
	if n := rawCount(t, s, idx); n != 1 {
		t.Fatal("Ensure did not create the @ref index")
	}
	var before string
	s.db.QueryRow(`SELECT "value" FROM "_alder_meta" WHERE "key" = 'schema_hash'`).Scan(&before)
	// a database made before the indexes: the next Ensure (same hash) adds them, in prod too
	exec(t, s, `DROP INDEX "Comment__Comment__post"`)
	exec(t, s, `INSERT INTO "Post__Post" VALUES ('p1', 'Hello')`)
	if err := s.Ensure(ctx, tables, false); err != nil {
		t.Fatal(err)
	}
	if n := rawCount(t, s, idx); n != 1 {
		t.Fatal("Ensure did not add the missing @ref index")
	}
	if n := rawCount(t, s, `SELECT COUNT(*) FROM "Post__Post"`); n != 1 {
		t.Fatal("adding the index touched data")
	}
	var after string
	s.db.QueryRow(`SELECT "value" FROM "_alder_meta" WHERE "key" = 'schema_hash'`).Scan(&after)
	if before != after {
		t.Fatal("the indexes changed the schema hash")
	}
	want := []string{`CREATE INDEX IF NOT EXISTS "Comment__Comment__post" ON "Comment__Comment"("post")`}
	if got := IndexStmts(tables); !slices.Equal(got, want) {
		t.Fatalf("IndexStmts = %q", got)
	}
}

func TestReadMigrations(t *testing.T) {
	fsys := fstest.MapFS{
		"migrations/0002_add_summary.sql": {Data: []byte("-- alder migration 0002_add_summary\r\n-- from 111111111111\r\n-- to 222222222222\r\n-- step: add column Post.summary\r\n\r\nALTER TABLE \"Post__Post\" ADD COLUMN \"summary\" TEXT;\r\n")},
		"migrations/0001_initial.sql":     {Data: []byte("-- alder migration 0001_initial\n-- from 000000000000\n-- to 111111111111\n\nCREATE TABLE \"A\" (\"id\" TEXT NOT NULL PRIMARY KEY) STRICT;\nINSERT INTO \"A\" VALUES ('-- not a comment;');\n")},
		"migrations/schema.json":          {Data: []byte("{}")},
	}
	got := ReadMigrations(fsys)
	want := []Migration{
		{Name: "0001_initial", From: "000000000000", To: "111111111111", SQL: "CREATE TABLE \"A\" (\"id\" TEXT NOT NULL PRIMARY KEY) STRICT;\nINSERT INTO \"A\" VALUES ('-- not a comment;');"},
		{Name: "0002_add_summary", From: "111111111111", To: "222222222222", SQL: `ALTER TABLE "Post__Post" ADD COLUMN "summary" TEXT;`},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q", got)
	}
	if st := got[0].statements(); len(st) != 2 || st[1] != `INSERT INTO "A" VALUES ('-- not a comment;')` {
		t.Fatalf("statements %q", st)
	}
	for name, data := range map[string]string{
		"no header":      "CREATE TABLE \"A\" (\"id\" TEXT) STRICT;\n",
		"another name":   "-- alder migration 0009_other\n\nSELECT 1;\n",
		"no semicolon":   "-- alder migration 0001_x\n\nSELECT 1\n",
		"header comment": "-- just a comment\n\nSELECT 1;\n",
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s: no panic", name)
				}
			}()
			ReadMigrations(fstest.MapFS{"migrations/0001_x.sql": {Data: []byte(data)}})
		}()
	}
}

func TestParseMigration(t *testing.T) {
	for text, want := range map[string]string{
		"-- alder migration 0002_x\n-- from a\n-- to b\n\nSELECT 1;\nSELECT 2\n": "migration 0002_x: line 6: a statement must end in `;`",
		"-- alder migration 0002_y\n-- from a\n-- to b\n\nSELECT 1;\n":           "migration 0002_x: line 1: the first line must be `-- alder migration 0002_x`",
		"-- alder migration 0002_x\n-- to b\n\nSELECT 1;\n":                      "migration 0002_x: the header lacks its `-- from` or `-- to` line",
	} {
		if _, err := ParseMigration("0002_x", text); err == nil || err.Error() != want {
			t.Errorf("%q: %v, want %s", text, err, want)
		}
	}
	m, err := ParseMigration("0002_x", "-- alder migration 0002_x\n-- from aaaaaaaaaaaa\n-- to bbbbbbbbbbbb\n-- step: x\n\nSELECT 1;\n")
	if err != nil || !reflect.DeepEqual(m, Migration{Name: "0002_x", From: "aaaaaaaaaaaa", To: "bbbbbbbbbbbb", SQL: "SELECT 1;"}) {
		t.Fatalf("%+v %v", m, err)
	}
	// a byte-order mark, and comment lines among the statements (a hand-added statement's note)
	m, err = ParseMigration("0002_x", "\ufeff-- alder migration 0002_x\r\n-- from aaaaaaaaaaaa\r\n-- to bbbbbbbbbbbb\r\n\r\n-- keep the drafts: make them public first\r\nUPDATE \"T\" SET \"x\" = 1;\r\n  -- indented note\r\nSELECT 1;\r\n-- trailing note\r\n")
	if err != nil || !reflect.DeepEqual(m, Migration{Name: "0002_x", From: "aaaaaaaaaaaa", To: "bbbbbbbbbbbb", SQL: "UPDATE \"T\" SET \"x\" = 1;\nSELECT 1;"}) {
		t.Fatalf("BOM and comments: %+v %v", m, err)
	}
	m, err = ParseMigration("0002_x", "-- alder migration 0002_x\n-- from aaaaaaaaaaaa\n-- to bbbbbbbbbbbb\n-- step: drop column Post.summary\n-- DROPS DATA: column Post.summary\n\nSELECT 1;\n")
	if err != nil || len(m.DropsData) != 1 || m.DropsData[0] != "column Post.summary" {
		t.Fatalf("DROPS DATA: %+v %v", m, err)
	}
}

// Two processes starting at once on a database made by Ensure: one adopts it, the other finds
// it adopted and goes on; neither fails.
func TestMigrate_ConcurrentAdoption(t *testing.T) {
	ctx := context.Background()
	ms := migrations(t)
	for i := range 20 {
		path := t.TempDir() + "/app.db"
		s0, err := OpenSQLite(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := s0.Ensure(ctx, []TableDesc{postV1, commentT}, false); err != nil {
			t.Fatal(err)
		}
		s0.Close()
		errs := make(chan error, 3)
		for range 3 {
			go func() {
				s, err := OpenSQLite(path)
				if err != nil {
					errs <- err
					return
				}
				defer s.Close()
				_, err = s.Migrate(ctx, []TableDesc{postV2, commentT}, ms[:2], false)
				errs <- err
			}()
		}
		for range 3 {
			if err := <-errs; err != nil {
				t.Fatalf("run %d: %v", i, err)
			}
		}
		s, _ := OpenSQLite(path)
		if got := recordedNames(t, s); !slices.Equal(got, []string{"0001_initial", "0002_add_summary"}) {
			t.Fatalf("run %d: recorded %v", i, got)
		}
		s.Close()
	}
}

// An adoption followed by a broken chain is refused before the adoption is recorded: nothing
// changes, not even _alder_migrations.
func TestMigrate_AdoptionThenBrokenChain(t *testing.T) {
	ctx := context.Background()
	s := openMem(t)
	if err := s.Ensure(ctx, []TableDesc{postV1, commentT}, false); err != nil {
		t.Fatal(err)
	}
	ms := migrations(t)
	_, err := s.Migrate(ctx, []TableDesc{postV3, commentT}, []Migration{ms[0], ms[2]}, false)
	if err == nil || !strings.HasPrefix(err.Error(), "migration 0003_add_views expects the schema ") {
		t.Fatalf("err = %v", err)
	}
	if n := rawCount(t, s, `SELECT COUNT(*) FROM sqlite_schema WHERE "name" = '_alder_migrations'`); n != 0 {
		t.Fatal("the refusal created _alder_migrations")
	}
	// and with a last file that does not lead to the app's schema
	_, err = s.Migrate(ctx, []TableDesc{postV3, commentT}, ms[:2], false)
	if !errors.Is(err, ErrDrift) {
		t.Fatalf("err = %v", err)
	}
	if n := rawCount(t, s, `SELECT COUNT(*) FROM sqlite_schema WHERE "name" = '_alder_migrations'`); n != 0 {
		t.Fatal("the refusal created _alder_migrations")
	}
}

// The body-edit drill: a file applied nowhere yet whose statements were edited after `alder
// migrate` wrote it (its header unchanged) is rolled back before it commits: the data stays, it
// is not recorded, and the error names the file and the remedy. Restored, it applies.
func TestMigrate_EditedBodyRolledBack(t *testing.T) {
	ctx := context.Background()
	path := t.TempDir() + "/app.db"
	s, err := OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ms := migrations(t)
	if _, err := s.Migrate(ctx, []TableDesc{postV1, commentT}, ms[:1], false); err != nil {
		t.Fatal(err)
	}
	exec(t, s, `INSERT INTO "Post__Post" VALUES ('p1', 'Hello')`)
	edited := ms[1]
	edited.SQL = `ALTER TABLE "Post__Post" DROP COLUMN "title";`
	tables := []TableDesc{postV2, commentT}
	applied, err := s.Migrate(ctx, tables, []Migration{ms[0], edited}, false)
	got := hashOf(t, TableDesc{Name: "Post.Post", ID: "id", Columns: []Column{{Name: "id", Type: CText}}}, commentT)
	want := "migration 0002_add_summary does not produce the schema its header names (to " + ms[1].To + ", got " + got + "): the file was edited; restore it from version control"
	if err == nil || err.Error() != want || len(applied) != 0 {
		t.Fatalf("%v %v\nwant %s", applied, err, want)
	}
	var title string
	if err := s.db.QueryRow(`SELECT "title" FROM "Post__Post" WHERE "id" = 'p1'`).Scan(&title); err != nil || title != "Hello" {
		t.Fatalf("data after the rollback: %q %v", title, err)
	}
	if got := recordedNames(t, s); !slices.Equal(got, []string{"0001_initial"}) {
		t.Fatalf("recorded %v", got)
	}
	var hash string
	s.db.QueryRow(`SELECT "value" FROM "_alder_meta" WHERE "key" = 'schema_hash'`).Scan(&hash)
	if short(hash) != ms[0].To {
		t.Fatalf("recorded schema %s, want 0001's %s", short(hash), ms[0].To)
	}
	// in dev mode too: an edited file is never recreated over
	if _, err := s.Migrate(ctx, tables, []Migration{ms[0], edited}, true); err == nil || err.Error() != want {
		t.Fatalf("dev: %v", err)
	}
	if applied, err := s.Migrate(ctx, tables, ms[:2], false); err != nil || !slices.Equal(applied, []string{"0002_add_summary"}) {
		t.Fatalf("restored: %v %v", applied, err)
	}
	if n := rawCount(t, s, `SELECT COUNT(*) FROM "Post__Post" WHERE "title" = 'Hello'`); n != 1 {
		t.Fatal("the restored file lost the row")
	}
}

// An executable older than the database (it lacks the newest recorded migration) says so.
func TestMigrate_OlderExecutable(t *testing.T) {
	ctx := context.Background()
	s := openMem(t)
	ms := migrations(t)
	if _, err := s.Migrate(ctx, []TableDesc{postV3, commentT}, ms, false); err != nil {
		t.Fatal(err)
	}
	for _, dev := range []bool{false, true} {
		_, err := s.Migrate(ctx, []TableDesc{postV1, commentT}, ms[:1], dev)
		want := "migration 0002_add_summary is recorded as applied but its file is missing; this executable predates the database (it has had 0003_add_views): deploy a build that includes it, or restore a backup taken before it"
		if err == nil || err.Error() != want {
			t.Fatalf("dev=%v: %v\nwant %s", dev, err, want)
		}
	}
	if got := tableSQL(t, s, "Post__Post"); got != create(t, postV3) {
		t.Fatalf("the refusal changed the table: %s", got)
	}
}

// An adopted database (made by Ensure before the @ref indexes existed) gets its indexes, and so
// does a migrated one whose index was dropped by hand; neither changes the schema hash.
func TestMigrate_IndexesAfterAdoption(t *testing.T) {
	ctx := context.Background()
	const idx = `SELECT COUNT(*) FROM sqlite_schema WHERE "type" = 'index' AND "name" = 'Comment__Comment__post'`
	for _, upTo := range []int{1, 2} { // adopted at the last file, and adopted then migrated
		s := openMem(t)
		if err := s.Ensure(ctx, []TableDesc{postV1, commentT}, false); err != nil {
			t.Fatal(err)
		}
		exec(t, s, `DROP INDEX "Comment__Comment__post"`)
		tables := []TableDesc{postV1, commentT}
		if upTo == 2 {
			tables[0] = postV2
		}
		if _, err := s.Migrate(ctx, tables, migrations(t)[:upTo], false); err != nil {
			t.Fatal(err)
		}
		if n := rawCount(t, s, idx); n != 1 {
			t.Fatalf("upTo %d: the adopted database lacks the @ref index", upTo)
		}
	}
	s := openMem(t)
	ms := migrations(t)
	if _, err := s.Migrate(ctx, []TableDesc{postV3, commentT}, ms, false); err != nil {
		t.Fatal(err)
	}
	exec(t, s, `DROP INDEX "Comment__Comment__post"`)
	if _, err := s.Migrate(ctx, []TableDesc{postV3, commentT}, ms, false); err != nil {
		t.Fatal(err)
	}
	if n := rawCount(t, s, idx); n != 1 {
		t.Fatal("a dropped @ref index was not recreated")
	}
}

// Three instances starting at once on a new file database: they take turns, every migration
// applies once, none refuses.
func TestMigrate_ConcurrentStarts(t *testing.T) {
	ctx := context.Background()
	ms := migrations(t)
	tables := []TableDesc{postV3, commentT}
	for i := range 10 {
		path := t.TempDir() + "/app.db"
		errs := make(chan error, 3)
		applied := make(chan []string, 3)
		for range 3 {
			go func() {
				s, err := OpenSQLite(path)
				if err != nil {
					errs <- err
					applied <- nil
					return
				}
				defer s.Close()
				a, err := s.Migrate(ctx, tables, ms, false)
				errs <- err
				applied <- a
			}()
		}
		var all []string
		for range 3 {
			if err := <-errs; err != nil {
				t.Fatalf("run %d: %v", i, err)
			}
			all = append(all, <-applied...)
		}
		slices.Sort(all)
		if !slices.Equal(all, []string{"0001_initial", "0002_add_summary", "0003_add_views"}) {
			t.Fatalf("run %d: applied %v", i, all)
		}
		s, _ := OpenSQLite(path)
		if got := recordedNames(t, s); !slices.Equal(got, []string{"0001_initial", "0002_add_summary", "0003_add_views"}) {
			t.Fatalf("run %d: recorded %v", i, got)
		}
		if got := tableSQL(t, s, "Post__Post"); got != create(t, postV3) {
			t.Fatalf("run %d: %s", i, got)
		}
		s.Close()
	}
}

// SQLite's "database is locked" names its likely cause.
func TestLockedErr(t *testing.T) {
	path := t.TempDir() + "/app.db"
	holder, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_txlock=immediate")
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Close()
	tx, err := holder.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`CREATE TABLE "x" ("a" INTEGER)`); err != nil {
		t.Fatal(err)
	}
	other, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(0)&_txlock=immediate")
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	_, busy := other.Begin()
	if busy == nil {
		t.Fatal("no busy error")
	}
	got := lockedErr(busy)
	if !strings.HasPrefix(got.Error(), "db: the database is locked: another instance is migrating or serving; wait and retry (") || !errors.Is(got, busy) {
		t.Fatalf("got %v", got)
	}
	if plain := errors.New("x"); lockedErr(plain) != plain {
		t.Fatal("another error was rewritten")
	}
}

// A pending migration that drops data is refused on a database that had migrations applied,
// with nothing changed, unless ALDER_ALLOW_DATA_LOSS=1 (or dev mode); a fresh database applies
// it (there is nothing to lose).
func TestMigrate_DropsDataRefused(t *testing.T) {
	ctx := context.Background()
	ms := migrations(t)
	ms[2].DropsData = []string{"Post.summary: rebuilt"}
	tables := []TableDesc{postV3, commentT}
	fresh := openMem(t)
	if applied, err := fresh.Migrate(ctx, tables, ms, false); err != nil || len(applied) != 3 {
		t.Fatalf("fresh: %v %v", applied, err)
	}
	s := openMem(t)
	if _, err := s.Migrate(ctx, []TableDesc{postV2, commentT}, ms[:2], false); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvAllowDataLoss, "")
	_, err := s.Migrate(ctx, tables, ms, false)
	if !errors.Is(err, ErrDropsData) || !strings.Contains(err.Error(), "DROPS DATA 0003_add_views: Post.summary: rebuilt; to apply it, take a backup and run with ALDER_ALLOW_DATA_LOSS=1") {
		t.Fatalf("err = %v", err)
	}
	if got := recordedNames(t, s); !slices.Equal(got, []string{"0001_initial", "0002_add_summary"}) {
		t.Errorf("recorded after the refusal: %v", got)
	}
	t.Setenv(EnvAllowDataLoss, "1")
	if applied, err := s.Migrate(ctx, tables, ms, false); err != nil || !slices.Equal(applied, []string{"0003_add_views"}) {
		t.Fatalf("allowed: %v %v", applied, err)
	}
}
