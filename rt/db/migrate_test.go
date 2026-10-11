package db

import (
	"context"
	"errors"
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
func migrations(t *testing.T) []Migration {
	newPost := postV3
	newPost.Name = "Post.Post__new"
	return []Migration{
		{Name: "0001_initial", SQL: strings.Join([]string{
			create(t, commentT, postV1) + ";",
			create(t, postV1) + ";",
			`CREATE INDEX IF NOT EXISTS "Comment__Comment__post" ON "Comment__Comment"("post");`,
		}, "\n")},
		{Name: "0002_add_summary", SQL: `ALTER TABLE "Post__Post" ADD COLUMN "summary" TEXT;`},
		{Name: "0003_add_views", SQL: strings.Join([]string{
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
	bad := Migration{Name: "0002_bad", SQL: `ALTER TABLE "Post__Post" ADD COLUMN "summary" TEXT;` + "\n" + `INSERT INTO "Nope" VALUES (1);`}
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
		if err == nil || err.Error() != "migration 0002_add_summary is recorded as applied but its file is missing" {
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
	ms := migrations(t)[:1]
	_, err := s.Migrate(ctx, tables, ms, false)
	if err == nil || err.Error() != "this database was not created by migrations: run in dev mode once, or back it up and delete it" {
		t.Fatalf("prod: %v", err)
	}
	if n := rawCount(t, s, `SELECT COUNT(*) FROM "Post__Post"`); n != 1 {
		t.Fatalf("prod refusal touched data: %d", n)
	}
	applied, err := s.Migrate(ctx, tables, ms, true)
	if err != nil || !slices.Equal(applied, []string{"0001_initial"}) {
		t.Fatalf("dev: %v %v", applied, err)
	}
	if n := rawCount(t, s, `SELECT COUNT(*) FROM "Post__Post"`); n != 0 {
		t.Fatalf("dev did not recreate: %d rows", n)
	}
	if got := recordedNames(t, s); !slices.Equal(got, []string{"0001_initial"}) {
		t.Fatalf("recorded %v", got)
	}
	if n := rawCount(t, s, `SELECT COUNT(*) FROM sqlite_schema WHERE "type" = 'index' AND "name" = 'Comment__Comment__post'`); n != 1 {
		t.Fatal("the recreated database lacks the @ref index")
	}
	// now it is a migrated database: prod accepts it, and the next migration applies
	if applied, err := s.Migrate(ctx, tables, ms, false); err != nil || len(applied) != 0 {
		t.Fatalf("after adopting: %v %v", applied, err)
	}
	if applied, err := s.Migrate(ctx, []TableDesc{postV2, commentT}, migrations(t)[:2], false); err != nil || !slices.Equal(applied, []string{"0002_add_summary"}) {
		t.Fatalf("next migration: %v %v", applied, err)
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
		if !errors.Is(err, ErrDrift) || !strings.Contains(err.Error(), "changed tables: Post.Post") || len(applied) != 0 {
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
	if err != nil || !slices.Equal(applied, []string{"0001_initial", "0002_add_summary"}) {
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
	_, err := s.Migrate(ctx, []TableDesc{postV2, commentT}, ms[:1], false)
	if !errors.Is(err, ErrDrift) || !strings.Contains(err.Error(), "after 0001_initial the tables differ from the app's (changed tables: Post.Post)") {
		t.Fatalf("err = %v", err)
	}
	if applied, err := s.Migrate(ctx, []TableDesc{postV2, commentT}, ms[:1], true); err != nil || len(applied) != 1 {
		t.Fatalf("dev: %v %v", applied, err)
	}
	if got := tableSQL(t, s, "Post__Post"); got != create(t, postV2) {
		t.Fatalf("dev recreate: %s", got)
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
	drop := Migration{Name: "0002_drop_posts", SQL: `DELETE FROM "Post__Post";`}
	_, err := s.Migrate(ctx, tables, []Migration{ms[0], drop}, false)
	if err == nil || err.Error() != "migration 0002_drop_posts: rows in Comment refer to rows this migration deletes: delete or re-point them first" {
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
		{Name: "0001_initial", SQL: "CREATE TABLE \"A\" (\"id\" TEXT NOT NULL PRIMARY KEY) STRICT;\nINSERT INTO \"A\" VALUES ('-- not a comment;');"},
		{Name: "0002_add_summary", SQL: `ALTER TABLE "Post__Post" ADD COLUMN "summary" TEXT;`},
	}
	if !slices.Equal(got, want) {
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
