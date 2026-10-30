package db

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// #48 measured a prepared-statement cache for the store and left it out: it saved time per
// statement but cost allocations (docs/perf/2026-10-bench-baseline.md). What stays: the probe
// that measured it, and the store's behaviour across a schema change that any such cache would
// have to keep.

func openFile(t testing.TB) *SQLite {
	t.Helper()
	s, err := OpenSQLite(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func postTitles(t *testing.T, s *SQLite) []string {
	t.Helper()
	rows, err := s.Query(context.Background(), nil, "Post.Post", postV1.Columns, SQL{Order: `"id"`})
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, r := range rows {
		out = append(out, r[1].(string))
	}
	return out
}

// A migration that rebuilds a table (0003: create, copy, drop, rename) and adds a column, then
// queries of the changed table: the INSERT text run before fails on the new NOT NULL column (it
// is not answered by a statement planned for the old table), the old SELECT reads the rebuilt
// table, and the new column is read.
func TestStore_MigrationThenQuery(t *testing.T) {
	s := openFile(t)
	ctx := context.Background()
	ms := migrations(t)
	if _, err := s.Migrate(ctx, []TableDesc{postV1, commentT}, ms[:1], false); err != nil {
		t.Fatal(err)
	}
	q, _ := s.on(nil)
	insert := `INSERT INTO "Post__Post" ("id", "title") VALUES (?, ?)`
	if _, err := q.ExecContext(ctx, insert, "1", "a"); err != nil {
		t.Fatal(err)
	}
	if got := postTitles(t, s); len(got) != 1 {
		t.Fatal(got)
	}
	if _, err := s.Migrate(ctx, []TableDesc{postV3, commentT}, ms, false); err != nil {
		t.Fatal(err)
	}
	_, err := q.ExecContext(ctx, insert, "2", "b")
	if err == nil || !strings.Contains(err.Error(), "NOT NULL constraint failed: Post__Post.views") {
		t.Fatalf("the old insert: %v", err)
	}
	if _, err := q.ExecContext(ctx, `INSERT INTO "Post__Post" ("id", "title", "views") VALUES (?, ?, ?)`, "2", "b", 5); err != nil {
		t.Fatal(err)
	}
	if got := postTitles(t, s); len(got) != 2 {
		t.Fatalf("the old select: %v", got)
	}
	rows, err := s.Query(ctx, nil, "Post.Post", postV3.Columns, SQL{Order: `"id"`})
	if err != nil || len(rows) != 2 || rows[0][3] != int64(0) || rows[1][3] != int64(5) {
		t.Fatalf("after the migration: %v %v", rows, err)
	}
}

// A table dropped (Ensure in dev mode recreates the database) fails a query of it with the
// table error, not a stale answer.
func TestStore_DroppedTable(t *testing.T) {
	s := openFile(t)
	ctx := context.Background()
	if err := s.Ensure(ctx, []TableDesc{postV1}, false); err != nil {
		t.Fatal(err)
	}
	postTitles(t, s)
	other := TableDesc{Name: "Tag.Tag", ID: "id", Columns: []Column{{Name: "id", Type: CText}}}
	if err := s.Ensure(ctx, []TableDesc{other}, true); err != nil {
		t.Fatal(err)
	}
	_, err := s.Query(ctx, nil, "Post.Post", postV1.Columns, SQL{Order: `"id"`})
	if err == nil || !strings.Contains(err.Error(), "no such table: Post__Post") {
		t.Fatalf("after the drop: %v", err)
	}
}

// BenchmarkSQLiteGetByID is a select by id of a 6-column row among 1,000, in a file database:
// unprepared (database/sql's QueryContext, what the store does), prepared once on the pool, and
// inside a transaction both ways (Tx.StmtContext of the prepared statement). Run it with
// -benchmem: docs/perf/2026-10-bench-baseline.md has its numbers.
func BenchmarkSQLiteGetByID(b *testing.B) {
	s := openFile(b)
	ctx := context.Background()
	desc := TableDesc{Name: "Bench.Row", ID: "id", Columns: []Column{{Name: "id", Type: CText}, {Name: "title", Type: CText},
		{Name: "n", Type: CInt}, {Name: "done", Type: CBool}, {Name: "x", Type: CFloat}, {Name: "note", Type: CText}}}
	if err := s.Ensure(ctx, []TableDesc{desc}, false); err != nil {
		b.Fatal(err)
	}
	for i := range 1000 {
		if _, err := s.db.ExecContext(ctx, `INSERT INTO "Bench__Row" VALUES (?, ?, ?, ?, ?, ?)`, fmt.Sprint(i), "title", i, 0, 1.5, "note"); err != nil {
			b.Fatal(err)
		}
	}
	text := `SELECT "id", "title", "n", "done", "x", "note" FROM "Bench__Row" WHERE "id" = ?`
	scan := func(b *testing.B, rows *sql.Rows, err error) {
		if err != nil {
			b.Fatal(err)
		}
		vals := make([]any, 6)
		ptrs := make([]any, 6)
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		for rows.Next() {
			if err := rows.Scan(ptrs...); err != nil {
				b.Fatal(err)
			}
		}
		rows.Close()
	}
	st, err := s.db.PrepareContext(ctx, text)
	if err != nil {
		b.Fatal(err)
	}
	defer st.Close()
	b.Run("unprepared", func(b *testing.B) {
		for i := 0; b.Loop(); i++ {
			rows, err := s.db.QueryContext(ctx, text, fmt.Sprint(i%1000))
			scan(b, rows, err)
		}
	})
	b.Run("prepared", func(b *testing.B) {
		for i := 0; b.Loop(); i++ {
			rows, err := st.QueryContext(ctx, fmt.Sprint(i%1000))
			scan(b, rows, err)
		}
	})
	b.Run("tx-unprepared", func(b *testing.B) {
		tx, _ := s.db.BeginTx(ctx, nil)
		defer tx.Rollback()
		for i := 0; b.Loop(); i++ {
			rows, err := tx.QueryContext(ctx, text, fmt.Sprint(i%1000))
			scan(b, rows, err)
		}
	})
	b.Run("tx-prepared", func(b *testing.B) {
		tx, _ := s.db.BeginTx(ctx, nil)
		defer tx.Rollback()
		for i := 0; b.Loop(); i++ {
			rows, err := tx.StmtContext(ctx, st).QueryContext(ctx, fmt.Sprint(i%1000))
			scan(b, rows, err)
		}
	})
}
