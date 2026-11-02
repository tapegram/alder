package data

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestGuard(t *testing.T) {
	for _, stmt := range []string{
		"select 1",
		"  -- c\nWITH x AS (select 1) select * from x",
		"explain query plan select 1",
		"select 1;  ",
		"/* a */ SELECT ';' -- ; drop\n",
		"select 1; -- done",
	} {
		if err := Guard(stmt); err != nil {
			t.Errorf("Guard(%q) = %v, want ok", stmt, err)
		}
	}
	for stmt, want := range map[string]string{
		"delete from t":          "only select, with and explain run here (the database is opened read-only)",
		"":                       "only select, with and explain run here (the database is opened read-only)",
		"-- select\npragma x":    "only select, with and explain run here (the database is opened read-only)",
		"selectx from t":         "only select, with and explain run here (the database is opened read-only)",
		"select 1; drop table t": "one statement at a time",
		"select 1;/**/select 2":  "one statement at a time",
	} {
		if err := Guard(stmt); err == nil || err.Error() != want {
			t.Errorf("Guard(%q) = %v, want %q", stmt, err, want)
		}
	}
}

// books is the map's table of books: an id, a title and a year.
var books = Table{Name: "Books.Book", SQL: "Books__Book", ID: "id", Cols: []Col{
	{Name: "id", Type: "Id Book", SQL: "id", SQLType: "TEXT"},
	{Name: "title", Type: "String", SQL: "title", SQLType: "TEXT"},
	{Name: "year", Type: "Int", SQL: "year", SQLType: "INTEGER"},
}}

// makeDB writes a database at path holding books with n rows: ids b000…, titles t000… in the
// reverse order of the ids, and years 1900+i.
func makeDB(t *testing.T, path string, n int) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+uriPath(path))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE "Books__Book" ("id" TEXT NOT NULL PRIMARY KEY, "title" TEXT NOT NULL, "year" INTEGER NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	for i := range n {
		id := "b" + pad3(i)
		title := "t" + pad3(n-1-i)
		if _, err := db.Exec(`INSERT INTO "Books__Book" VALUES (?, ?, ?)`, id, title, 1900+i); err != nil {
			t.Fatal(err)
		}
	}
}

func pad3(i int) string {
	s := strconv.Itoa(i)
	return strings.Repeat("0", 3-len(s)) + s
}

func openTemp(t *testing.T, n int) *DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "dev.db")
	makeDB(t, path, n)
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestOpen_ReadOnly(t *testing.T) {
	db := openTemp(t, 3)
	ctx := context.Background()
	cols, rows, err := db.Query(ctx, `select id, year from "Books__Book" order by id`)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"id", "year"}; !reflect.DeepEqual(cols, want) {
		t.Errorf("cols = %q, want %q", cols, want)
	}
	if want := [][]string{{"b000", "1900"}, {"b001", "1901"}, {"b002", "1902"}}; !reflect.DeepEqual(rows, want) {
		t.Errorf("rows = %q, want %q", rows, want)
	}
	if _, _, err := db.Query(ctx, `delete from "Books__Book"`); err == nil {
		t.Error("Query ran a delete")
	}
	// A write through the handle itself, past Guard, is refused by SQLite.
	err = db.exec(ctx, `delete from "Books__Book"`)
	if err == nil || !strings.Contains(err.Error(), "readonly") && !strings.Contains(err.Error(), "read-only") &&
		!strings.Contains(err.Error(), "query_only") {
		t.Errorf("a write through the handle = %v, want a read-only error", err)
	}
	if n, err := db.Count(ctx, books); err != nil || n != 3 {
		t.Errorf("after the refused write Count = %d, %v; want 3", n, err)
	}
	if _, err := Open(filepath.Join(t.TempDir(), "none.db")); err == nil {
		t.Error("Open of a missing file succeeded (read-only must not create it)")
	}
}

// A path holding the characters a URI gives meaning to opens the file it names.
func TestOpen_OddPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a#b?c%20d", "dev.db")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	makeDB(t, path, 2)
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if n, err := db.Count(context.Background(), books); err != nil || n != 2 {
		t.Errorf("Count = %d, %v; want 2", n, err)
	}
}

func TestQuery_Limit(t *testing.T) {
	db := openTemp(t, 0)
	_, rows, err := db.Query(context.Background(), "with recursive n(i) as (select 1 union all select i+1 from n where i < 1500) select i from n")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != QueryRows {
		t.Errorf("rows = %d, want %d", len(rows), QueryRows)
	}
}

func TestCountPageSort(t *testing.T) {
	db := openTemp(t, 150)
	ctx := context.Background()
	if n, err := db.Count(ctx, books); err != nil || n != 150 {
		t.Fatalf("Count = %d, %v; want 150", n, err)
	}
	cols, rows, err := db.Page(ctx, books, "", false, 0)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"id", "title", "year"}; !reflect.DeepEqual(cols, want) {
		t.Errorf("cols = %q, want %q", cols, want)
	}
	if len(rows) != 100 || rows[0][0] != "b000" {
		t.Errorf("first page: %d rows, first %q", len(rows), rows[0])
	}
	_, rows, err = db.Page(ctx, books, "", false, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 50 || rows[0][0] != "b100" {
		t.Errorf("page at 100: %d rows, first %q", len(rows), rows[0])
	}
	_, rows, err = db.Page(ctx, books, "title", true, 0)
	if err != nil {
		t.Fatal(err)
	}
	if rows[0][1] != "t149" {
		t.Errorf("by title desc the first row is %q, want t149", rows[0])
	}
	_, rows, err = db.Page(ctx, books, "nope", true, 0)
	if err != nil {
		t.Fatal(err)
	}
	if rows[0][0] != "b149" {
		t.Errorf("an unknown column sorts by %q first, want the id (b149)", rows[0])
	}
	// An identifier is quoted, so a column the map names oddly stays one identifier: the
	// statement still parses as one and returns its page.
	odd := books
	odd.Cols = append([]Col{}, books.Cols...)
	odd.Cols = append(odd.Cols, Col{Name: "x", SQL: `x" desc; drop table t --`})
	if _, rows, err := db.Page(ctx, odd, odd.Cols[3].SQL, false, 0); err != nil || len(rows) != 100 {
		t.Errorf("an oddly named column: %d rows, %v", len(rows), err)
	}
}

func TestMissingTable(t *testing.T) {
	db := openTemp(t, 1)
	gone := Table{Name: "Books.Gone", SQL: "Books__Gone"}
	if _, err := db.Count(context.Background(), gone); err == nil || Message(err) != "no such table: Books__Gone" {
		t.Errorf("Count of a missing table = %v, want no such table: Books__Gone", err)
	}
	if _, _, err := db.Page(context.Background(), gone, "", false, 0); err == nil {
		t.Error("Page of a missing table succeeded")
	}
}

func TestDatabases_Silos(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"silos/org/b2.db", "silos/org/a1.db", "silos/org/a1.db-wal", "silos/org/notes.txt"} {
		p := filepath.Join(dir, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	m := Map{Silos: []Silo{{Key: "org", Ref: "Orgs.Org", Path: "silos/org/{value}.db", Tables: []string{"Issues.Issue"}}}}
	got := Databases(dir, m)
	want := []Choice{
		{Label: "shared", Path: filepath.Join(dir, "dev.db")},
		{Label: "org/a1", Path: filepath.Join(dir, "silos", "org", "a1.db"), Silo: "org"},
		{Label: "org/b2", Path: filepath.Join(dir, "silos", "org", "b2.db"), Silo: "org"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Databases = %+v\nwant %+v", got, want)
	}
	if got := Databases(dir, Map{}); len(got) != 1 || got[0].Label != "shared" {
		t.Errorf("without silos Databases = %+v, want shared alone", got)
	}
	// Siloed tables are listed for silo databases only, shared tables for shared only.
	m.Tables = []Table{{Name: "Orgs.Org", SQL: "Orgs__Org"}, {Name: "Issues.Issue", SQL: "Issues__Issue", Silo: "org"}}
	if ts := m.TablesFor(want[0]); len(ts) != 1 || ts[0].Name != "Orgs.Org" {
		t.Errorf("shared tables = %+v", ts)
	}
	if ts := m.TablesFor(want[1]); len(ts) != 1 || ts[0].Name != "Issues.Issue" {
		t.Errorf("silo tables = %+v", ts)
	}
}

func TestParseMap(t *testing.T) {
	b := []byte(`{"build":"b1","map":{"pages":[],"tables":[{"name":"Books.Book","sql":"Books__Book","columns":["id: Id Book"],` +
		`"cols":[{"name":"id","type":"Id Book","sql":"id","sqlType":"TEXT"}],"id":"id"},{"name":"Issues.Issue","sql":"Issues__Issue",` +
		`"columns":[],"cols":[],"silo":"org"}],"session":[],"silos":[{"key":"org","ref":"Orgs.Org","tables":["Issues.Issue"],` +
		`"path":"silos/org/{value}.db"}]},"caps":[],"migrations":["0001_init.sql"],"routes":[]}`)
	m, err := ParseMap(b)
	if err != nil {
		t.Fatal(err)
	}
	want := Map{Build: "b1", Migrations: []string{"0001_init.sql"},
		Tables: []Table{
			{Name: "Books.Book", SQL: "Books__Book", ID: "id", Cols: []Col{{Name: "id", Type: "Id Book", SQL: "id", SQLType: "TEXT"}}},
			{Name: "Issues.Issue", SQL: "Issues__Issue", Silo: "org", Cols: []Col{}},
		},
		Silos: []Silo{{Key: "org", Ref: "Orgs.Org", Path: "silos/org/{value}.db", Tables: []string{"Issues.Issue"}}}}
	if !reflect.DeepEqual(m, want) {
		t.Errorf("ParseMap = %+v\nwant %+v", m, want)
	}
	if _, err := ParseMap([]byte("<html>")); err == nil {
		t.Error("ParseMap of HTML succeeded")
	}
}
