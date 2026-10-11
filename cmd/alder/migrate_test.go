package main

import (
	"bytes"
	"database/sql"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"alder/internal/migrate"
)

func runMigrate(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := runMigrateTo(args, &out, &errb)
	return code, out.String(), errb.String()
}

// freshBlog is a copy of examples/blog without its migrations/ (tempApp: the working
// directory is the copy's parent).
func freshBlog(t *testing.T) string {
	t.Helper()
	dir := tempApp(t, "../../examples/blog", "blog")
	if err := os.RemoveAll(filepath.Join(dir, "migrations")); err != nil {
		t.Fatal(err)
	}
	return dir
}

// editApp replaces old with new in the app file rel of dir, which must contain old.
func editApp(t *testing.T, dir, rel, old, new string) {
	t.Helper()
	path := filepath.Join(dir, "app", rel)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), old) {
		t.Fatalf("%s lacks %q", rel, old)
	}
	if err := os.WriteFile(path, []byte(strings.Replace(string(b), old, new, 1)), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// migrationsList is the names in dir/migrations, sorted.
func migrationsList(t *testing.T, dir string) []string {
	t.Helper()
	entries, _ := os.ReadDir(filepath.Join(dir, "migrations"))
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	slices.Sort(names)
	return names
}

// mustMigrate runs `alder migrate dir args…`, which must exit 0 and print want.
func mustMigrate(t *testing.T, want string, args ...string) {
	t.Helper()
	code, out, errs := runMigrate(t, args...)
	if code != 0 || out != want || errs != "" {
		t.Fatalf("migrate %v: exit %d\n%s\nstderr: %s\nwant:\n%s", args, code, out, errs, want)
	}
}

// mustCheck is `alder check dir`: wantE0019 says whether it reports E0019 (and nothing else);
// otherwise it is clean.
func mustCheck(t *testing.T, dir string, wantE0019 string) {
	t.Helper()
	code, out, errs := run(t, dir)
	if wantE0019 == "" {
		if code != 0 || out != "" || errs != "" {
			t.Fatalf("check: exit %d\n%s%s", code, out, errs)
		}
		return
	}
	want := "migrations/schema.json:1:1: error[E0019]: " + wantE0019 + "\n"
	if code != 1 || out != want {
		t.Fatalf("check: exit %d\n%s%s\nwant %s", code, out, errs, want)
	}
}

// The first migration of each example is what the examples commit (so the committed files are
// current), the app then checks clean, and a second run changes nothing (Review Focus 5).
func TestMigrate_Init(t *testing.T) {
	for _, ex := range []string{"todos", "blog"} {
		t.Run(ex, func(t *testing.T) {
			committed, err := filepath.Abs(filepath.Join("../../examples", ex, "migrations"))
			if err != nil {
				t.Fatal(err)
			}
			dir := tempApp(t, "../../examples/"+ex, ex)
			os.RemoveAll(filepath.Join(dir, "migrations"))
			code, out, errs := runMigrate(t, dir)
			if code != 0 || errs != "" || !strings.HasPrefix(out, "wrote "+ex+"/migrations/0001_initial.sql\n  create table ") ||
				!strings.HasSuffix(out, "updated "+ex+"/migrations/schema.json\n") {
				t.Fatalf("migrate: exit %d\n%s%s", code, out, errs)
			}
			if got := migrationsList(t, dir); !slices.Equal(got, []string{"0001_initial.sql", "schema.json"}) {
				t.Fatalf("migrations/ = %v", got)
			}
			for _, f := range []string{"0001_initial.sql", "schema.json"} {
				if got, want := readFile(t, filepath.Join(dir, "migrations", f)), readFile(t, filepath.Join(committed, f)); got != want {
					t.Errorf("examples/%s/migrations/%s is not what alder migrate writes:\n%s", ex, f, got)
				}
			}
			mustCheck(t, dir, "")
			mustMigrate(t, "no changes since 0001_initial\n", dir)
			if got := migrationsList(t, dir); len(got) != 2 {
				t.Fatalf("a run without a change wrote %v", got)
			}
		})
	}
}

// Chapter 8's worked change, on the blog: a nullable field (ADD COLUMN), renamed with
// @renamed (RENAME COLUMN; the attribute removed afterwards checks clean: Review Focus 1),
// then dropped (DROP COLUMN, DROPS DATA in the file and the output). E0019 comes before each
// migration and is gone after it.
func TestMigrate_AddRenameDrop(t *testing.T) {
	dir := freshBlog(t)
	runMigrate(t, dir)

	editApp(t, dir, "Post.alder", "  created: Time @created,\n", "  created: Time @created,\n  summary: Maybe Text = Nothing,\n")
	editApp(t, dir, "Post.alder", "created: Time.epoch }", "created: Time.epoch, summary: Nothing }")
	mustCheck(t, dir, "the schema changed since migration 0001 (table Post: column summary): run `alder migrate`")
	mustMigrate(t, "wrote blog/migrations/0002_add_summary.sql\n  add column Post.summary\nupdated blog/migrations/schema.json\n", dir)
	if f := readFile(t, filepath.Join(dir, "migrations", "0002_add_summary.sql")); !strings.HasSuffix(f, "\n\nALTER TABLE \"Post__Post\" ADD COLUMN \"summary\" TEXT;\n") {
		t.Fatalf("0002:\n%s", f)
	}
	mustCheck(t, dir, "")

	editApp(t, dir, "Post.alder", "  summary: Maybe Text = Nothing,\n", "  excerpt: Maybe Text = Nothing @renamed \"summary\",\n")
	editApp(t, dir, "Post.alder", "summary: Nothing }", "excerpt: Nothing }")
	mustCheck(t, dir, "the schema changed since migration 0002 (table Post: column excerpt): run `alder migrate`")
	mustMigrate(t, "wrote blog/migrations/0003_rename_summary.sql\n  rename column Post.summary to excerpt\nupdated blog/migrations/schema.json\n", dir)
	if f := readFile(t, filepath.Join(dir, "migrations", "0003_rename_summary.sql")); !strings.HasSuffix(f, "\n\nALTER TABLE \"Post__Post\" RENAME COLUMN \"summary\" TO \"excerpt\";\n") {
		t.Fatalf("0003:\n%s", f)
	}
	editApp(t, dir, "Post.alder", ` @renamed "summary"`, "")
	mustCheck(t, dir, "")
	mustMigrate(t, "no changes since 0003_rename_summary\n", dir)

	editApp(t, dir, "Post.alder", "  excerpt: Maybe Text = Nothing,\n", "")
	editApp(t, dir, "Post.alder", ", excerpt: Nothing }", " }")
	mustCheck(t, dir, "the schema changed since migration 0003 (table Post: column excerpt): run `alder migrate`")
	mustMigrate(t, "wrote blog/migrations/0004_drop_excerpt.sql\n  drop column Post.excerpt\n!! DROPS DATA: column Post.excerpt\nupdated blog/migrations/schema.json\n", dir)
	f := readFile(t, filepath.Join(dir, "migrations", "0004_drop_excerpt.sql"))
	if !strings.Contains(f, "\n-- DROPS DATA: column Post.excerpt\n") || !strings.HasSuffix(f, "\n\nALTER TABLE \"Post__Post\" DROP COLUMN \"excerpt\";\n") {
		t.Fatalf("0004:\n%s", f)
	}
	mustCheck(t, dir, "")
	// back where it started: the last file leads to 0001's schema
	_, _, to1, _, _ := migrate.ParseHeader(readFile(t, filepath.Join(dir, "migrations", "0001_initial.sql")))
	_, _, to4, _, _ := migrate.ParseHeader(f)
	if to1 != to4 || to1 == "" {
		t.Fatalf("0004 leads to %s, 0001 to %s", to4, to1)
	}
}

// A new non-null field needs a default: without one migrate refuses and writes nothing; with
// one the table is rebuilt, the rows filled with the default (internal/migrate: ADD COLUMN …
// DEFAULT would leave a DEFAULT in the table SQLite stores, which the app's CREATE TABLE has
// not).
func TestMigrate_NonNullField(t *testing.T) {
	dir := freshBlog(t)
	writeScore(t, dir)
	runMigrate(t, dir)
	editApp(t, dir, "Score.alder", "  points: Int,\n", "  points: Int,\n  likes: Int,\n")
	before := migrationsList(t, dir)
	code, out, errs := runMigrate(t, dir)
	if code != 1 || out != "alder migrate: Score.likes: a new non-null column needs a default (`= 0`) so existing rows can be filled\n" || errs != "" {
		t.Fatalf("migrate without a default: exit %d\n%s%s", code, out, errs)
	}
	if got := migrationsList(t, dir); !slices.Equal(got, before) {
		t.Fatalf("a refused migration wrote %v", got)
	}
	editApp(t, dir, "Score.alder", "  likes: Int,\n", "  likes: Int = 0,\n")
	mustMigrate(t, "wrote blog/migrations/0002_change_score.sql\n  rebuild table Score: Score.likes: new column filled with 0\nupdated blog/migrations/schema.json\n", dir)
	f := readFile(t, filepath.Join(dir, "migrations", "0002_change_score.sql"))
	if !strings.Contains(f, `INSERT INTO "Score__Score__alder__new" ("id", "points", "likes") SELECT "id", "points", 0 FROM "Score__Score";`) {
		t.Fatalf("0002:\n%s", f)
	}
	mustCheck(t, dir, "")
}

// writeScore adds module Score, a table no code inserts into, to the app at dir.
func writeScore(t *testing.T, dir string) {
	t.Helper()
	src := "module Score\n\ntype ScoreId = brand Text\n\ntable Score {\n  id: ScoreId @id,\n  points: Int,\n}\n"
	if err := os.WriteFile(filepath.Join(dir, "app", "Score.alder"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Run twice without a change, migrate writes nothing and says so (Review Focus 5). A changed
// default alone is no change to the database (defaults are not in its tables), but schema.json
// records it.
func TestMigrate_NoChange(t *testing.T) {
	dir := freshBlog(t)
	runMigrate(t, dir)
	mustMigrate(t, "no changes since 0001_initial\n", dir)
	mustMigrate(t, "no changes since 0001_initial\n", dir, "--dry-run")
	editApp(t, dir, "Post.alder", "published: Bool = False,", "published: Bool = True,")
	mustCheck(t, dir, "")
	snap := filepath.Join(dir, "migrations", "schema.json")
	mustMigrate(t, "no changes since 0001_initial\n", dir, "--dry-run")
	if strings.Contains(readFile(t, snap), `"default": "1"`) {
		t.Fatal("a dry run rewrote schema.json")
	}
	mustMigrate(t, "no changes since 0001_initial\nupdated blog/migrations/schema.json (defaults only)\n", dir)
	if !strings.Contains(readFile(t, snap), `"default": "1"`) {
		t.Fatalf("schema.json keeps the old default:\n%s", readFile(t, snap))
	}
	if got := migrationsList(t, dir); !slices.Equal(got, []string{"0001_initial.sql", "schema.json"}) {
		t.Fatalf("migrations/ = %v", got)
	}
	mustMigrate(t, "no changes since 0001_initial\n", dir)
}

// --dry-run prints the file it would write and writes nothing (Review Focus 5); --name names it.
func TestMigrate_DryRun(t *testing.T) {
	dir := freshBlog(t)
	runMigrate(t, dir)
	editApp(t, dir, "User.alder", "  created: Time @created,\n", "  created: Time @created,\n  bio: Maybe Text = Nothing,\n")
	before := migrationsList(t, dir)
	snap := readFile(t, filepath.Join(dir, "migrations", "schema.json"))
	code, out, errs := runMigrate(t, dir, "--dry-run")
	if code != 0 || errs != "" || !strings.HasPrefix(out, "-- alder migration 0002_add_bio\n-- from ") ||
		!strings.HasSuffix(out, "-- step: add column User.bio\n\nALTER TABLE \"User__User\" ADD COLUMN \"bio\" TEXT;\n\ndry run: blog/migrations/0002_add_bio.sql not written\n") {
		t.Fatalf("dry run: exit %d\n%s%s", code, out, errs)
	}
	if got := migrationsList(t, dir); !slices.Equal(got, before) || readFile(t, filepath.Join(dir, "migrations", "schema.json")) != snap {
		t.Fatalf("dry run wrote %v", got)
	}
	mustCheck(t, dir, "the schema changed since migration 0001 (table User: column bio): run `alder migrate`")

	code, _, errs = runMigrate(t, dir, "--name", "Bad Name")
	if code != 2 || !strings.Contains(errs, `migration name "Bad Name": use lower-case letters, digits and single underscores`) {
		t.Fatalf("bad --name: exit %d\n%s", code, errs)
	}
	mustMigrate(t, "wrote blog/migrations/0002_user_bio.sql\n  add column User.bio\nupdated blog/migrations/schema.json\n", dir, "--name", "user_bio")
	if code, _, _ := runMigrate(t, "--name"); code != 2 {
		t.Fatalf("--name without a slug: exit %d", code)
	}
}

// An app whose tables changed since its last migration is refused by check, build, test and
// eval with E0019; migrate itself does not report it (it is the fix).
func TestMigrate_E0019Everywhere(t *testing.T) {
	dir := tempApp(t, "../../examples/blog", "blog") // with the committed 0001_initial
	editApp(t, dir, "User.alder", "  created: Time @created,\n", "  created: Time @created,\n  bio: Maybe Text = Nothing,\n")
	const msg = "the schema changed since migration 0001 (table User: column bio): run `alder migrate`"
	mustCheck(t, dir, msg)
	code, out, errs := runBuild(t, "--emit", dir)
	if code != 1 || !strings.Contains(out, "error[E0019]: "+msg) {
		t.Fatalf("build: exit %d\n%s%s", code, out, errs)
	}
	code, out, errs = runTest(t, dir)
	if code != 1 || !strings.Contains(out+errs, "error[E0019]: "+msg) {
		t.Fatalf("test: exit %d\n%s%s", code, out, errs)
	}
	code, out, errs = runEval(t, "", dir, "1 + 1")
	if code != 1 || out != "" || !strings.Contains(errs, "error[E0019]: "+msg) {
		t.Fatalf("eval: exit %d\n%s%s", code, out, errs)
	}
	code, out, errs = runMigrate(t, dir, "--dry-run")
	if code != 0 || strings.Contains(out+errs, "E0019") {
		t.Fatalf("migrate reported E0019: exit %d\n%s%s", code, out, errs)
	}
}

// migrations/ that disagree with themselves are refused, and nothing is written: a
// schema.json that is not the last file's result, a schema.json without files, files without
// a schema.json.
func TestMigrate_SnapshotMismatch(t *testing.T) {
	dir := freshBlog(t)
	runMigrate(t, dir)
	mdir := filepath.Join(dir, "migrations")
	first := readFile(t, filepath.Join(mdir, "schema.json"))
	editApp(t, dir, "User.alder", "  created: Time @created,\n", "  created: Time @created,\n  bio: Maybe Text = Nothing,\n")
	runMigrate(t, dir)
	os.WriteFile(filepath.Join(mdir, "schema.json"), []byte(first), 0o644) // a snapshot from before 0002
	editApp(t, dir, "User.alder", "  bio: Maybe Text = Nothing,\n", "  bio: Maybe Text = Nothing,\n  site: Maybe Text = Nothing,\n")
	before := migrationsList(t, dir)
	code, out, errs := runMigrate(t, dir)
	if code != 1 || out != "" || errs != "alder migrate: migrations/schema.json does not match 0002_add_bio.sql's result; restore the snapshot from git or regenerate the migration\n" {
		t.Fatalf("mismatch: exit %d\n%s%s", code, out, errs)
	}
	if got := migrationsList(t, dir); !slices.Equal(got, before) {
		t.Fatalf("wrote %v", got)
	}

	os.Remove(filepath.Join(mdir, "schema.json"))
	code, _, errs = runMigrate(t, dir)
	if code != 1 || errs != "alder migrate: migrations/ has migration files but no schema.json, the schema they lead to: restore it from git\n" {
		t.Fatalf("no snapshot: exit %d\n%s", code, errs)
	}
	os.WriteFile(filepath.Join(mdir, "schema.json"), []byte(first), 0o644)
	os.Remove(filepath.Join(mdir, "0001_initial.sql"))
	os.Remove(filepath.Join(mdir, "0002_add_bio.sql"))
	code, _, errs = runMigrate(t, dir)
	if code != 1 || errs != "alder migrate: migrations/schema.json has no migration file: restore the migrations from git, or delete schema.json to start again from 0001_initial\n" {
		t.Fatalf("no files: exit %d\n%s", code, errs)
	}
}

// A type change on a table with rows: the rebuild keeps them, converted, across a restart.
// The dev database is made by migrations (eval --run), rows go in at 0002, the type changes
// (0003, a rebuild) and the next eval --run, which opens the database outside dev mode as a
// restarted server does, applies 0003 and reads the rows back.
func TestMigrate_RebuildKeepsRows(t *testing.T) {
	needGoTool(t)
	dir := freshBlog(t)
	writeScore(t, dir)
	runMigrate(t, dir)
	code, out, errs := runEval(t, "", dir, "--run", "Db.from Score |> Db.count")
	if code != 0 || out != "0 : Int\n" {
		t.Fatalf("eval --run: exit %d\n%s%s", code, out, errs)
	}
	devDB, err := sql.Open("sqlite", filepath.Join("build", "blog", "dev.db"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = devDB.Exec(`INSERT INTO "Score__Score" ("id", "points") VALUES ('a', 3), ('b', 4)`)
	devDB.Close()
	if err != nil {
		t.Fatal(err)
	}
	editApp(t, dir, "Score.alder", "  points: Int,\n", "  points: Float,\n  bonus: Int = 7,\n")
	mustMigrate(t, "wrote blog/migrations/0002_change_score.sql\n  rebuild table Score: Score.points: Int -> Float; Score.bonus: new column filled with 7\nupdated blog/migrations/schema.json\n", dir)
	code, out, errs = runEval(t, "", dir, "--run", "Db.from Score |> Db.orderBy Score.points.asc |> Db.all")
	if code != 0 || out != `[{ id: "a", points: 3.0, bonus: 7 }, { id: "b", points: 4.0, bonus: 7 }] : List Score`+"\n" {
		t.Fatalf("eval --run after the rebuild: exit %d\n%s%s", code, out, errs)
	}
}

func TestDeriveSlug(t *testing.T) {
	const T = "Post__Post"
	for _, c := range []struct {
		steps []migrate.Step
		want  string
	}{
		{[]migrate.Step{{Kind: migrate.AddColumn, Table: T, Column: "summary"}}, "add_summary"},
		{[]migrate.Step{{Kind: migrate.AddColumn, Table: T, Column: "publishedAt"}}, "add_published_at"},
		{[]migrate.Step{{Kind: migrate.CreateTable, Table: "Comment__Comment"}, {Kind: migrate.AddIndex, Table: "Comment__Comment", Column: "post"}}, "create_comment"},
		{[]migrate.Step{{Kind: migrate.RenameColumn, Table: T, Column: "summary", NewName: "excerpt"}}, "rename_summary"},
		{[]migrate.Step{{Kind: migrate.RenameTable, Table: "Article__Article", NewName: T}}, "rename_article"},
		{[]migrate.Step{{Kind: migrate.DropColumn, Table: T, Column: "excerpt"}, {Kind: migrate.DropTable, Table: "Draft__Draft"}}, "drop_excerpt_drop_draft"},
		{[]migrate.Step{{Kind: migrate.Rebuild, Table: T}, {Kind: migrate.Rebuild, Table: "BlogUser__BlogUser"}}, "change_post_change_blog_user"},
		{[]migrate.Step{{Kind: migrate.AddColumn, Table: T, Column: "a"}, {Kind: migrate.AddColumn, Table: "User__User", Column: "a"}}, "add_a"},
		{[]migrate.Step{{Kind: migrate.AddIndex, Table: T, Column: "author"}}, "change"},
		// joined while within 40 characters
		{[]migrate.Step{{Kind: migrate.AddColumn, Table: T, Column: "summary"}, {Kind: migrate.AddColumn, Table: T, Column: "subtitle"}, {Kind: migrate.AddColumn, Table: T, Column: "coverImage"}, {Kind: migrate.AddColumn, Table: T, Column: "x"}}, "add_summary_add_subtitle_add_cover_image"},
		{[]migrate.Step{{Kind: migrate.AddColumn, Table: T, Column: "aVeryLongColumnNameThatGoesOnAndOnAndOn"}}, "add_a_very_long_column_name_that_goes_on"},
	} {
		got := deriveSlug(c.steps)
		if got != c.want || len(got) > maxSlug {
			t.Errorf("deriveSlug(%v) = %q, want %q", c.steps, got, c.want)
		}
		if _, err := migrate.NextName(t.TempDir(), got); err != nil {
			t.Errorf("%q is not a slug NextName takes: %v", got, err)
		}
	}
}
