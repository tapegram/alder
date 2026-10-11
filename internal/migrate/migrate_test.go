package migrate

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"alder/internal/check"
	"alder/internal/diag"
	"alder/internal/gen"
	"alder/internal/ir"
	"alder/internal/names"
	"alder/internal/types"
	"alder/rt/db"
)

var update = flag.Bool("update", false, "rewrite golden files")

// ---- building schemas

func col(name string, t db.ColType) Column { return Column{Name: name, Type: t} }
func text(name string) Column              { return col(name, db.CText) }
func enum(name string, ctors ...string) Column {
	return Column{Name: name, Type: db.CEnum, Enum: ctors}
}
func (c Column) null() Column            { c.Nullable = true; return c }
func (c Column) uniq() Column            { c.Unique = true; return c }
func (c Column) ref(t string) Column     { c.Ref = t + "." + t; return c }
func (c Column) on(r db.OnDelete) Column { c.OnDelete = r; return c }
func (c Column) def(lit string) Column   { c.Default = &lit; return c }
func (c Column) from(old string) Column  { c.RenamedFrom = old; return c }

func tbl(name string, cols ...Column) Table {
	return Table{Name: name + "." + name, ID: "id", Columns: append([]Column{text("id")}, cols...)}
}

func schema(ts ...Table) Schema {
	s := Schema{Format: db.Format, Tables: ts}
	slices.SortFunc(s.Tables, func(a, b Table) int { return strings.Compare(a.Name, b.Name) })
	return s
}

func user(extra ...Column) Table {
	return tbl("User", append([]Column{text("name"), text("email").uniq()}, extra...)...)
}

func post(cols ...Column) Table { return tbl("Post", cols...) }

// postCols are Post's columns in the base schema; with replaces some by name (keeping order).
func postCols(with ...Column) []Column {
	cs := []Column{
		text("title"), text("body"), text("author").ref("User"),
		col("views", db.CInt), enum("status", "Draft", "Published", "Archived"),
		col("score", db.CFloat), col("published", db.CBool), col("day", db.CDate), col("at", db.CTime),
		text("summary").null(),
	}
	for _, w := range with {
		for i := range cs {
			if cs[i].Name == w.Name || cs[i].Name == w.RenamedFrom {
				cs[i] = w
			}
		}
	}
	return cs
}

func without(cs []Column, name string) []Column {
	return slices.DeleteFunc(slices.Clone(cs), func(c Column) bool { return c.Name == name })
}

func comment(cols ...Column) Table {
	if len(cols) == 0 {
		cols = commentCols()
	}
	return tbl("Comment", cols...)
}

func commentCols() []Column {
	return []Column{text("post").ref("Post").on(db.Cascade), text("body"), text("parent").null().ref("Comment").on(db.SetNull)}
}

func base() Schema { return schema(user(), post(postCols()...), comment()) }

// ---- the cases

type diffCase struct {
	name     string
	old, new Schema
}

func cases() []diffCase {
	b := base()
	renamedPost := post(postCols()...)
	renamedPost.Name, renamedPost.RenamedFrom = "Article.Article", "Post.Post"
	commentToArticle := comment(text("post").ref("Article").on(db.Cascade), text("body"), text("parent").null().ref("Comment").on(db.SetNull))
	idRenamed := post(postCols()...)
	idRenamed.Columns[0] = text("postId").from("id")
	idRenamed.ID = "postId"
	renamedAndChanged := post(postCols(col("views", db.CFloat))...)
	renamedAndChanged.Name, renamedAndChanged.RenamedFrom = "Article.Article", "Post.Post"
	withRole := schema(user(enum("role", "A", "B", "C").null()), post(postCols()...), comment())
	return []diffCase{
		{"initial", Schema{}, b},
		{"no_change", b, b},
		{"default_only", b, schema(user(), post(postCols(col("views", db.CInt).def("5"))...), comment())},
		{"brand_only", b, b}, // a brand change (Text.NonEmpty -> Text) is the same CText: no step
		{"create_table", b, schema(user(), post(postCols()...), comment(), tbl("Tag", text("post").ref("Post"), text("label").uniq()))},
		{"drop_table", b, schema(user(), post(postCols()...))},
		{"drop_referenced_table", b, schema(post(without(postCols(), "author")...), comment())},
		{"rename_table", b, schema(user(), renamedPost, commentToArticle)},
		{"rename_table_and_rebuild", b, schema(user(), renamedAndChanged, commentToArticle)},
		{"rename_table_missing", b, schema(user(), func() Table {
			t := post(postCols()...)
			t.Name, t.RenamedFrom = "Article.Article", "Story.Story"
			return t
		}(), commentToArticle)},
		{"add_nullable_column", b, schema(user(text("bio").null()), post(postCols()...), comment())},
		{"add_nullable_ref_column", b, schema(user(), post(append(postCols(), text("editor").null().ref("User").on(db.SetNull))...), comment())},
		{"add_nonnull_with_default", b, schema(user(col("karma", db.CInt).def("0"), text("motto").def("'it''s' || char(10) || 'new'")), post(postCols()...), comment())},
		{"add_nonnull_without_default", b, schema(user(col("karma", db.CInt)), post(postCols()...), comment())},
		{"add_nonnull_date_without_default", b, schema(user(col("born", db.CDate)), post(postCols()...), comment())},
		{"add_nonnull_ref", b, schema(user(), post(append(postCols(), text("editor").ref("User"))...), comment())},
		{"add_unique_column", b, schema(user(text("handle").null().uniq()), post(postCols()...), comment())},
		{"add_column_mid_table", b, schema(tbl("User", text("name"), text("bio").null(), text("email").uniq()), post(postCols()...), comment())},
		{"drop_column", b, schema(user(), post(without(postCols(), "summary")...), comment())},
		{"drop_unique_column", b, schema(tbl("User", text("name")), post(postCols()...), comment())},
		{"drop_ref_column", b, schema(user(), post(without(postCols(), "author")...), comment())},
		{"rename_column", b, schema(user(), post(postCols(text("heading").from("title"))...), comment())},
		{"rename_ref_column", b, schema(user(), post(postCols()...), comment(text("article").ref("Post").on(db.Cascade).from("post"), text("body"), text("parent").null().ref("Comment").on(db.SetNull)))},
		{"rename_missing_column", b, schema(user(), post(append([]Column{text("heading").from("headline")}, postCols()[1:]...)...), comment())},
		{"rename_column_still_declared", b, schema(user(), post(append(postCols(), text("heading").from("title"))...), comment())},
		{"rename_in_rebuild", b, schema(user(), post(postCols(text("heading").from("title"), col("views", db.CFloat))...), comment())},
		{"nullable_enum_remove", withRole, schema(user(enum("role", "A", "B").null()), post(postCols()...), comment())},
		{"nullable_enum_to_nonnull_default", withRole, schema(user(enum("role", "A", "B").def("'A'")), post(postCols()...), comment())},
		{"nullable_enum_to_nonnull_no_default", withRole, schema(user(enum("role", "A", "B", "C")), post(postCols()...), comment())},
		{"rename_and_drop_and_add", b, schema(user(), post(append(without(postCols(text("heading").from("title")), "summary"), text("subtitle").null())...), comment())},
		{"rename_id", b, schema(user(), idRenamed, comment())},
		{"int_to_text", b, schema(user(), post(postCols(text("views"))...), comment())},
		{"text_to_int", b, schema(tbl("User", col("name", db.CInt), text("email").uniq()), post(postCols()...), comment())},
		{"int_to_float", b, schema(user(), post(postCols(col("views", db.CFloat))...), comment())},
		{"float_to_int", b, schema(user(), post(postCols(col("score", db.CInt))...), comment())},
		{"bool_to_int", b, schema(user(), post(postCols(col("published", db.CInt))...), comment())},
		{"int_to_bool", b, schema(user(), post(postCols(col("views", db.CBool))...), comment())},
		{"text_to_enum", b, schema(user(), post(postCols(enum("title", "12", "abc"))...), comment())},
		{"enum_to_text", b, schema(user(), post(postCols(text("status"))...), comment())},
		{"date_to_int", b, schema(user(), post(postCols(col("day", db.CInt))...), comment())},
		{"nullable_to_nonnull_default", b, schema(user(), post(postCols(text("summary").def("''"))...), comment())},
		{"nullable_to_nonnull_no_default", b, schema(user(), post(postCols(text("summary"))...), comment())},
		{"nonnull_to_nullable", b, schema(user(), post(postCols(text("body").null())...), comment())},
		{"enum_add", b, schema(user(), post(postCols(enum("status", "Draft", "Published", "Archived", "Hidden"))...), comment())},
		{"enum_remove", b, schema(user(), post(postCols(enum("status", "Draft", "Published"))...), comment())},
		{"enum_reorder", b, schema(user(), post(postCols(enum("status", "Published", "Draft", "Archived"))...), comment())},
		{"unique_add", b, schema(tbl("User", text("name").uniq(), text("email").uniq()), post(postCols()...), comment())},
		{"unique_remove", b, schema(tbl("User", text("name"), text("email")), post(postCols()...), comment())},
		{"ref_add", b, schema(user(), post(postCols()...), comment(text("post").ref("Post").on(db.Cascade), text("body").ref("User"), text("parent").null().ref("Comment").on(db.SetNull)))},
		{"delete_rule_change", b, schema(user(), post(postCols()...), comment(text("post").ref("Post"), text("body"), text("parent").null().ref("Comment").on(db.SetNull)))},
		{"reorder_columns", b, schema(tbl("User", text("email").uniq(), text("name")), post(postCols()...), comment())},
	}
}

// ---- goldens and the property

func TestDiffGolden(t *testing.T) {
	seen := map[string]bool{}
	for _, c := range cases() {
		if seen[c.name] {
			t.Fatalf("duplicate case %s", c.name)
		}
		seen[c.name] = true
		t.Run(c.name, func(t *testing.T) {
			m, err := Plan(c.old, c.new, "0002_"+c.name)
			var got string
			if err != nil {
				got = "error: " + err.Error() + "\n"
			} else {
				got = Render(m) + apply(t, c.old, c.new, m)
				// deterministic: the same inputs give the same plan
				m2, _ := Plan(c.old, c.new, "0002_"+c.name)
				if !reflect.DeepEqual(m, m2) {
					t.Fatal("Plan is not deterministic")
				}
			}
			compareGolden(t, filepath.Join("testdata", "diff", c.name+".golden"), got)
		})
	}
}

func compareGolden(t *testing.T, golden, got string) {
	t.Helper()
	if *update {
		if err := os.MkdirAll(filepath.Dir(golden), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("missing golden %s (run with -update)", golden)
	}
	if got != string(want) {
		t.Fatalf("golden mismatch for %s\n got:\n%s\nwant:\n%s", golden, got, want)
	}
}

func openMem(t *testing.T) *sql.DB {
	t.Helper()
	d, err := sql.Open("sqlite", ":memory:?_pragma=foreign_keys(0)")
	if err != nil {
		t.Fatal(err)
	}
	d.SetMaxOpenConns(1)
	t.Cleanup(func() { d.Close() })
	return d
}

func exec(t *testing.T, q interface {
	Exec(string, ...any) (sql.Result, error)
}, stmts []string) {
	t.Helper()
	for _, st := range stmts {
		if _, err := q.Exec(st); err != nil {
			t.Fatalf("%s: %v", st, err)
		}
	}
}

// seedValue is row i's value for column c of a seeded table: every table has rows r0, r1, r2,
// so a reference to row i of any table holds.
func seedValue(t Table, c Column, i int) any {
	switch {
	case c.Name == t.ID:
		return fmt.Sprintf("r%d", i)
	case c.Nullable && i == 0:
		return nil
	case c.Ref != "" && c.Ref == t.Name:
		return fmt.Sprintf("r%d", i-1) // a self-reference: the previous row
	case c.Ref != "":
		return fmt.Sprintf("r%d", i)
	}
	switch c.Type {
	case db.CInt:
		return []int64{0, 1, 5}[i]
	case db.CFloat:
		return []float64{0, 1.5, 2}[i]
	case db.CBool:
		return int64(i % 2)
	case db.CDate:
		return fmt.Sprintf("2026-01-0%d", i+1)
	case db.CTime:
		return fmt.Sprintf("2026-01-0%dT00:00:00.000000000Z", i+1)
	case db.CEnum:
		return c.Enum[i%len(c.Enum)]
	}
	return []string{"12", "abc", "7.5"}[i]
}

// apply is the property: old's own migration builds a database, seeded with three rows per
// table; m's statements (foreign keys off, one transaction) then leave exactly new's CREATE
// TABLE statements and @ref indexes, every table keeps its rows unless a step says it drops
// some, and every value of the new tables reads back as its column's type. It returns a line
// for the golden with each table's row count and the foreign-key check.
func apply(t *testing.T, old, new Schema, m Migration) string {
	t.Helper()
	d := openMem(t)
	first, err := Plan(Schema{}, old, "0001_initial")
	if err != nil {
		t.Fatal(err)
	}
	exec(t, d, first.SQL)
	for _, tb := range old.Tables {
		for i := range 3 {
			var cols, marks []string
			var vals []any
			for _, c := range tb.Columns {
				cols, marks, vals = append(cols, quote(c.Name)), append(marks, "?"), append(vals, seedValue(tb, c, i))
			}
			exec1 := "INSERT INTO " + quote(db.SQLName(tb.Name)) + " (" + strings.Join(cols, ", ") + ") VALUES (" + strings.Join(marks, ", ") + ")"
			if _, err := d.Exec(exec1, vals...); err != nil {
				t.Fatalf("seed %s: %v", tb.Name, err)
			}
		}
	}
	tx, err := d.Begin()
	if err != nil {
		t.Fatal(err)
	}
	exec(t, tx, m.SQL)
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	// the tables and indexes are exactly new's
	live := func(kind string) []string {
		rows, err := d.Query(`SELECT "sql" FROM sqlite_schema WHERE "type" = ? AND "sql" IS NOT NULL AND "name" NOT LIKE 'sqlite\_%' ESCAPE '\' ORDER BY "sql"`, kind)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		var out []string
		for rows.Next() {
			var s string
			if err := rows.Scan(&s); err != nil {
				t.Fatal(err)
			}
			out = append(out, s)
		}
		return out
	}
	want, err := new.CreateStmts()
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(want)
	if got := live("table"); !slices.Equal(got, want) {
		t.Fatalf("live tables differ from new's CREATE TABLE statements\n got:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	var wantIdx []string
	for _, ix := range indexes(new) {
		wantIdx = append(wantIdx, "CREATE INDEX "+quote(ix.name())+" ON "+quote(db.SQLName(ix.table))+"("+quote(ix.col)+")")
	}
	slices.Sort(wantIdx)
	if got := live("index"); !slices.Equal(got, wantIdx) {
		t.Fatalf("live indexes differ\n got:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(wantIdx, "\n"))
	}

	// rows: kept unless a step of the table drops some; values decode as their column types
	p, _ := pairTables(old, new)
	var counts []string
	for _, tb := range new.Tables {
		sqlT := db.SQLName(tb.Name)
		var n int
		if err := d.QueryRow("SELECT count(*) FROM " + quote(sqlT)).Scan(&n); err != nil {
			t.Fatal(err)
		}
		counts = append(counts, fmt.Sprintf("%s=%d", short(tb.Name), n))
		want := 0
		if _, ok := p.from[tb.Name]; ok {
			want = 3
		}
		mayDrop := slices.ContainsFunc(m.Steps, func(s Step) bool { return s.Table == sqlT && strings.Contains(s.DropsData, "rows ") })
		if n != want && !(mayDrop && n < want) {
			t.Errorf("table %s has %d rows, want %d (no step drops rows)", tb.Name, n, want)
		}
		checkValues(t, d, tb)
	}
	var violations int
	rows, err := d.Query("PRAGMA foreign_key_check")
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		violations++
	}
	rows.Close()
	var ok string
	if err := d.QueryRow("PRAGMA integrity_check").Scan(&ok); err != nil || ok != "ok" {
		t.Fatalf("integrity_check: %s %v", ok, err)
	}
	return fmt.Sprintf("\n-- applied to seeded rows: %s; foreign_key_check: %d\n", strings.Join(counts, " "), violations)
}

// checkValues reads every row of tb and checks each value against its column (rt/db's decoders).
func checkValues(t *testing.T, d *sql.DB, tb Table) {
	t.Helper()
	var cols []string
	for _, c := range tb.Columns {
		cols = append(cols, quote(c.Name))
	}
	rows, err := d.Query("SELECT " + strings.Join(cols, ", ") + " FROM " + quote(db.SQLName(tb.Name)))
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			t.Fatal(err)
		}
		for i, c := range tb.Columns {
			v := vals[i]
			if v == nil {
				if !c.Nullable {
					t.Errorf("%s.%s: NULL in a non-null column", tb.Name, c.Name)
				}
				continue
			}
			var err error
			switch c.Type {
			case db.CText, db.CEnum:
				_, err = db.DecodeText(v)
				if err == nil && c.Type == db.CEnum && !slices.Contains(c.Enum, v.(string)) {
					err = fmt.Errorf("%q is not a constructor", v)
				}
			case db.CInt:
				_, err = db.DecodeInt(v)
			case db.CFloat:
				_, err = db.DecodeFloat(v)
			case db.CBool:
				_, err = db.DecodeBool(v)
			case db.CDate:
				_, err = db.DecodeDate(v)
			case db.CTime:
				_, err = db.DecodeTime(v)
			}
			if err != nil {
				t.Errorf("%s.%s: %v", tb.Name, c.Name, err)
			}
		}
	}
}

// ---- other API

func TestDiff_StepOrder(t *testing.T) {
	b := base()
	n := schema(
		tbl("Account", text("name"), text("bio").null()), // User renamed? no: a new table
		func() Table { p := post(postCols(text("heading").from("title"), col("views", db.CFloat))...); return p }(),
		comment(append(commentCols(), text("note").null())...),
	)
	steps, err := Diff(b, n)
	if err != nil {
		t.Fatal(err)
	}
	var kinds []string
	for _, s := range steps {
		kinds = append(kinds, s.Kind.String()+" "+s.Table+" "+s.Column)
	}
	// Post's author refers to the dropped User: the column is kept (a REFERENCES to a dropped
	// table is not checked while foreign keys are off) — this schema is not a valid app, only
	// an order check
	want := []string{"DropTable User__User ", "CreateTable Account__Account ", "AddColumn Comment__Comment note", "Rebuild Post__Post "}
	if !slices.Equal(kinds, want) {
		t.Fatalf("got  %q\nwant %q", kinds, want)
	}
}

func TestRenderParseHeader(t *testing.T) {
	m, err := Plan(base(), schema(user(), post(without(postCols(enum("status", "Draft")), "summary")...), comment()), "0003_trim")
	if err != nil {
		t.Fatal(err)
	}
	text := Render(m)
	name, from, to, drops, err := ParseHeader(text)
	if err != nil {
		t.Fatal(err)
	}
	var want []string
	for _, s := range m.Steps {
		if s.DropsData != "" {
			want = append(want, s.DropsData)
		}
	}
	if name != m.Name || from != m.FromHash || to != m.ToHash || !slices.Equal(drops, want) || len(drops) == 0 {
		t.Fatalf("ParseHeader = %q %q %q %q, want %q %q %q %q", name, from, to, drops, m.Name, m.FromHash, m.ToHash, want)
	}
	if len(from) != 12 || len(to) != 12 {
		t.Fatalf("hashes %q %q are not 12 hex digits", from, to)
	}
	// the statements are the lines after the header, one per line, each ending in `;`
	var stmts []string
	inHeader := true
	for _, line := range strings.Split(strings.TrimSuffix(text, "\n"), "\n") {
		if inHeader && strings.HasPrefix(line, "--") {
			continue
		}
		inHeader = false
		if line != "" {
			if !strings.HasSuffix(line, ";") {
				t.Fatalf("statement line %q does not end in ;", line)
			}
			stmts = append(stmts, strings.TrimSuffix(line, ";"))
		}
	}
	if !slices.Equal(stmts, m.SQL) {
		t.Fatalf("statements %q, want %q", stmts, m.SQL)
	}
	for _, bad := range []string{"", "CREATE TABLE x;\n", "-- alder migration 0001_x\n\nX;\n", "-- something else\n-- from a\n-- to b\n"} {
		if _, _, _, _, err := ParseHeader(bad); err == nil {
			t.Errorf("ParseHeader(%q): no error", bad)
		}
	}
	// CRLF line ends are tolerated
	if n, _, _, _, err := ParseHeader(strings.ReplaceAll(text, "\n", "\r\n")); err != nil || n != m.Name {
		t.Errorf("CRLF: %q %v", n, err)
	}
}

func TestNextName(t *testing.T) {
	dir := t.TempDir()
	if n, err := NextName(filepath.Join(dir, "missing"), "initial"); err != nil || n != "0001_initial" {
		t.Fatalf("missing dir: %q %v", n, err)
	}
	if n, err := NextName(dir, "initial"); err != nil || n != "0001_initial" {
		t.Fatalf("empty dir: %q %v", n, err)
	}
	for _, f := range []string{"0001_initial.sql", "0003_add_comment.sql", "schema.json", "0009_notes.txt", "12_x.sql"} {
		if err := os.WriteFile(filepath.Join(dir, f), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if n, err := NextName(dir, "add_tags"); err != nil || n != "0004_add_tags" {
		t.Fatalf("got %q %v", n, err)
	}
	for _, bad := range []string{"", "Add", "a-b", "a__b", "_a", "a b"} {
		if _, err := NextName(dir, bad); err == nil {
			t.Errorf("slug %q accepted", bad)
		}
	}
}

func TestSnapshotRoundTrip(t *testing.T) {
	s := base()
	s.Tables[1].Columns[4] = s.Tables[1].Columns[4].def("0")
	s.Tables[0].Created = "created"
	path := filepath.Join(t.TempDir(), "schema.json")
	if err := WriteSnapshot(path, s); err != nil {
		t.Fatal(err)
	}
	got, err := ReadSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, s) {
		t.Fatalf("round trip:\n got %+v\nwant %+v", got, s)
	}
	b, _ := os.ReadFile(path)
	for _, want := range []string{`"type": "Enum"`, `"onDelete": "cascade"`, `"default": "0"`, `"ref": "User.User"`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("snapshot lacks %s:\n%s", want, b)
		}
	}
	if err := os.WriteFile(path, []byte(`{"format":"1","tables":[{"name":"A.A","id":"id","columns":[{"name":"id","type":"Blob"}]}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadSnapshot(path); err == nil || !strings.Contains(err.Error(), `unknown type "Blob"`) {
		t.Fatalf("bad type: %v", err)
	}
}

// Hash is Ensure's: the hash Ensure records for a schema's descriptors.
func TestHashIsEnsures(t *testing.T) {
	for _, s := range []Schema{base(), {Format: db.Format}, programSchema(t, "../../examples/blog"), programSchema(t, "../../examples/todos")} {
		path := filepath.Join(t.TempDir(), "x.db")
		st, err := db.OpenSQLite(path)
		if err != nil {
			t.Fatal(err)
		}
		var descs []db.TableDesc
		for _, tb := range s.Tables {
			descs = append(descs, tb.Desc())
		}
		if err := st.Ensure(context.Background(), descs, false); err != nil {
			t.Fatal(err)
		}
		st.Close()
		d, err := sql.Open("sqlite", path)
		if err != nil {
			t.Fatal(err)
		}
		var recorded string
		err = d.QueryRow(`SELECT "value" FROM "_alder_meta" WHERE "key" = 'schema_hash'`).Scan(&recorded)
		d.Close()
		if err != nil {
			t.Fatal(err)
		}
		if h, err := s.Hash(); err != nil || h != recorded {
			t.Fatalf("Hash = %s, %v; Ensure recorded %s", h, err, recorded)
		}
	}
}

// FromProgram's columns are the descriptors internal/gen writes into the generated app.
func TestFromProgramMatchesGen(t *testing.T) {
	for _, dir := range []string{"../../examples/blog", "../../examples/todos", "../../testdata/corpus/renamed-attrs-ok"} {
		p := programIR(t, dir)
		files, err := gen.Emit(p, gen.Options{})
		if err != nil {
			t.Fatal(err)
		}
		var all strings.Builder
		for _, b := range files {
			all.Write(b)
		}
		for _, tb := range FromProgram(p).Tables {
			var want strings.Builder
			want.WriteString("Columns: []db.Column{\n")
			for _, c := range tb.Columns {
				fmt.Fprintf(&want, "\t\t{Name: %q, Type: db.C%s", c.Name, colTypeNames[c.Type])
				if c.Nullable {
					want.WriteString(", Nullable: true")
				}
				if c.Type == db.CEnum {
					fmt.Fprintf(&want, ", Enum: []string{%s}", strings.Join(quoteAll(c.Enum), ", "))
				}
				if c.Unique {
					want.WriteString(", Unique: true")
				}
				if c.Ref != "" {
					fmt.Fprintf(&want, ", Ref: %q", c.Ref)
				}
				switch c.OnDelete {
				case db.Cascade:
					want.WriteString(", OnDelete: db.Cascade")
				case db.SetNull:
					want.WriteString(", OnDelete: db.SetNull")
				}
				want.WriteString("},\n")
			}
			if !strings.Contains(all.String(), want.String()) {
				t.Errorf("%s: gen has no descriptor\n%s", tb.Name, want.String())
			}
		}
	}
}

func quoteAll(xs []string) []string {
	out := make([]string, len(xs))
	for i, x := range xs {
		out[i] = fmt.Sprintf("%q", x)
	}
	return out
}

// programSchema loads, checks and lowers the app at dir and returns its Schema.
func programSchema(t *testing.T, dir string) Schema {
	t.Helper()
	return FromProgram(programIR(t, dir))
}

// programIR loads, checks and lowers the app at dir.
func programIR(t *testing.T, dir string) *ir.Program {
	t.Helper()
	std, err := types.LoadStd()
	if err != nil {
		t.Fatal(err)
	}
	prog, bag, ioErrs := names.Load(dir, std)
	for _, e := range ioErrs {
		t.Fatal(e)
	}
	prog.Resolve(bag)
	prog.MarkFailed(bag)
	res := check.Program(prog, bag)
	if bag.HasErrors() {
		var sb strings.Builder
		for _, d := range bag.Items {
			if d.Severity == "error" {
				sb.WriteString(diag.RenderText(d, nil) + "\n")
			}
		}
		t.Fatalf("%s does not check:\n%s", dir, sb.String())
	}
	return ir.Lower(prog, res)
}

// FromProgram writes the examples' schemas as goldens (their snapshots, as `alder migrate` would
// write them) and carries @renamed and literal defaults.
func TestFromProgram(t *testing.T) {
	for _, app := range []string{"blog", "todos"} {
		s := programSchema(t, "../../examples/"+app)
		path := filepath.Join(t.TempDir(), "schema.json")
		if err := WriteSnapshot(path, s); err != nil {
			t.Fatal(err)
		}
		b, _ := os.ReadFile(path)
		compareGolden(t, filepath.Join("testdata", app+"_schema.json"), string(b))
	}
	s := programSchema(t, "../../testdata/corpus/renamed-attrs-ok")
	p := s.table("Post.Post")
	if p == nil || p.RenamedFrom != "Article.Article" {
		t.Fatalf("Post: %+v", p)
	}
	var got []string
	for _, c := range p.Columns {
		x := c.Name + ":" + colTypeNames[c.Type]
		if c.Nullable {
			x += "?"
		}
		if c.Default != nil {
			x += "=" + *c.Default
		}
		if c.RenamedFrom != "" {
			x += "<" + c.RenamedFrom
		}
		got = append(got, x)
	}
	want := "id:Text title:Text<heading count:Int=0<total ratio:Float=-0.5 shown:Bool=1 role:Enum='Reader' note:Text?=NULL level:Enum?='Admin' motto:Text='it''s {done}'"
	if strings.Join(got, " ") != want {
		t.Fatalf("got  %s\nwant %s", strings.Join(got, " "), want)
	}
	// the @renamed survive into a Diff against the old names
	old := schema(Table{Name: "Article.Article", ID: "id", Columns: []Column{text("id"), text("heading"), col("total", db.CInt), col("ratio", db.CFloat), col("shown", db.CBool), enum("role", "Reader", "Admin"), text("note").null(), enum("level", "Reader", "Admin").null(), text("motto")}})
	steps, err := Diff(old, s)
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	for _, st := range steps {
		lines = append(lines, st.String())
	}
	if w := "rename table Article to Post|rename column Post.heading to title|rename column Post.total to count"; strings.Join(lines, "|") != w {
		t.Fatalf("steps %q, want %q", strings.Join(lines, "|"), w)
	}
}

func TestIndexes(t *testing.T) {
	want := []string{
		`CREATE INDEX IF NOT EXISTS "Comment__Comment__parent" ON "Comment__Comment"("parent")`,
		`CREATE INDEX IF NOT EXISTS "Comment__Comment__post" ON "Comment__Comment"("post")`,
		`CREATE INDEX IF NOT EXISTS "Post__Post__author" ON "Post__Post"("author")`,
	}
	if got := base().Indexes(); !slices.Equal(got, want) {
		t.Fatalf("got %q", got)
	}
}
