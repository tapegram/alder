package db

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"alder/internal/tursofake"
)

// The statement cache (stmtQ): each text prepared once per store, emptied when the schema may
// change, capped, never on Turso.

func openFile(t testing.TB) *SQLite {
	t.Helper()
	s, err := OpenSQLite(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func stmtCount(s *SQLite) int {
	s.stmtMu.Lock()
	defer s.stmtMu.Unlock()
	return len(s.stmts)
}

func titles(t *testing.T, s *SQLite, tx Tx) []string {
	t.Helper()
	rows, err := s.Query(context.Background(), tx, "Post.Post", postV1.Columns, SQL{Order: `"id"`})
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, r := range rows {
		out = append(out, r[1].(string))
	}
	return out
}

func insertPost(t *testing.T, s *SQLite, tx Tx, id, title string) {
	t.Helper()
	q, err := s.on(tx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.ExecContext(context.Background(), `INSERT INTO "Post__Post" ("id", "title") VALUES (?, ?)`, id, title); err != nil {
		t.Fatal(err)
	}
}

// A second Query of the same text reuses the prepared statement: the cache holds one entry for
// it, and a transaction's statements go through the same one.
func TestStmtCache_Reuse(t *testing.T) {
	s := openFile(t)
	ctx := context.Background()
	if err := s.Ensure(ctx, []TableDesc{postV1}, false); err != nil {
		t.Fatal(err)
	}
	insertPost(t, s, nil, "1", "a")
	titles(t, s, nil)
	st := s.cached(`SELECT "id", "title" FROM "Post__Post" ORDER BY "id"`)
	if got := titles(t, s, nil); len(got) != 1 || got[0] != "a" || stmtCount(s) != 2 || st == nil {
		t.Fatalf("%v, %d cached", got, stmtCount(s))
	}
	if s.cached(`SELECT "id", "title" FROM "Post__Post" ORDER BY "id"`) != st {
		t.Fatal("prepared again")
	}
	tx, err := s.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	insertPost(t, s, tx, "2", "b")
	if got := titles(t, s, tx); len(got) != 2 || stmtCount(s) != 2 {
		t.Fatalf("%v, %d cached", got, stmtCount(s))
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

// Ensure, MigrateLog, CreateTables and Close empty the cache and close its statements: a
// statement kept from before fails as a closed one.
func TestStmtCache_ResetOnSchemaChange(t *testing.T) {
	ctx := context.Background()
	extra := TableDesc{Name: "Fake.Sent", ID: "id", Columns: []Column{{Name: "id", Type: CText}}}
	for _, tc := range []struct {
		name string
		do   func(s *SQLite) error
	}{
		{"Ensure", func(s *SQLite) error { return s.Ensure(ctx, []TableDesc{postV1}, false) }},
		{"MigrateLog", func(s *SQLite) error {
			_, err := s.MigrateLog(ctx, []TableDesc{postV1}, nil, true, nil)
			return err
		}},
		{"CreateTables", func(s *SQLite) error { return s.CreateTables(ctx, []TableDesc{extra}, []TableDesc{postV1}) }},
		{"Close", func(s *SQLite) error { return s.Close() }},
	} {
		s := openFile(t)
		if err := s.Ensure(ctx, []TableDesc{postV1}, false); err != nil {
			t.Fatal(err)
		}
		titles(t, s, nil)
		kept := s.cached(`SELECT "id", "title" FROM "Post__Post" ORDER BY "id"`)
		if kept == nil {
			t.Fatalf("%s: nothing cached", tc.name)
		}
		if err := tc.do(s); err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if n := stmtCount(s); n != 0 {
			t.Errorf("%s: %d cached after it", tc.name, n)
		}
		if _, err := kept.Query(); err == nil || !strings.Contains(err.Error(), "statement is closed") {
			t.Errorf("%s: the kept statement still runs: %v", tc.name, err)
		}
		if tc.name != "Close" {
			titles(t, s, nil) // and the store goes on preparing
		}
	}
}

// At 257 texts the cache is emptied (the 256 closed) and the 257th goes in; the 258th is cached
// beside it.
func TestStmtCache_Cap(t *testing.T) {
	s := openMem(t)
	ctx := context.Background()
	q, _ := s.on(nil)
	first := ""
	for i := 1; i <= stmtCap; i++ {
		text := fmt.Sprintf("SELECT %d", i)
		if i == 1 {
			first = text
		}
		if _, err := q.ExecContext(ctx, text); err != nil {
			t.Fatal(err)
		}
	}
	kept := s.cached(first)
	if stmtCount(s) != stmtCap || kept == nil {
		t.Fatalf("%d cached", stmtCount(s))
	}
	if _, err := q.ExecContext(ctx, "SELECT 257"); err != nil {
		t.Fatal(err)
	}
	if n := stmtCount(s); n != 1 {
		t.Fatalf("after 257: %d cached", n)
	}
	if _, err := kept.Exec(); err == nil {
		t.Error("an emptied statement still runs")
	}
	if _, err := q.ExecContext(ctx, "SELECT 258"); err != nil || stmtCount(s) != 2 {
		t.Fatalf("after 258: %d cached (%v)", stmtCount(s), err)
	}
}

// A statement cached before its table is dropped (Ensure in dev mode recreates the database)
// fails with the table error after it, not a stale answer.
func TestStmtCache_DroppedTable(t *testing.T) {
	s := openFile(t)
	ctx := context.Background()
	if err := s.Ensure(ctx, []TableDesc{postV1}, false); err != nil {
		t.Fatal(err)
	}
	insertPost(t, s, nil, "1", "a")
	titles(t, s, nil)
	other := TableDesc{Name: "Tag.Tag", ID: "id", Columns: []Column{{Name: "id", Type: CText}}}
	if err := s.Ensure(ctx, []TableDesc{other}, true); err != nil {
		t.Fatal(err)
	}
	_, err := s.Query(ctx, nil, "Post.Post", postV1.Columns, SQL{Order: `"id"`})
	if err == nil || !strings.Contains(err.Error(), "no such table: Post__Post") {
		t.Fatalf("after the drop: %v", err)
	}
}

// A migration that rebuilds a table (0003: create, copy, drop, rename) and adds a column, then a
// query of the changed table: the new column is read, and the text cached before it reads the
// table as it is now.
func TestStmtCache_MigrationThenQuery(t *testing.T) {
	s := openFile(t)
	ctx := context.Background()
	ms := migrations(t)
	if _, err := s.Migrate(ctx, []TableDesc{postV1, commentT}, ms[:1], false); err != nil {
		t.Fatal(err)
	}
	insertPost(t, s, nil, "1", "a")
	if got := titles(t, s, nil); len(got) != 1 {
		t.Fatal(got)
	}
	if _, err := s.Migrate(ctx, []TableDesc{postV3, commentT}, ms, false); err != nil {
		t.Fatal(err)
	}
	q, _ := s.on(nil)
	// the INSERT text run before: the rebuilt table's views has no default, so it now fails
	_, err := q.ExecContext(ctx, `INSERT INTO "Post__Post" ("id", "title") VALUES (?, ?)`, "2", "b")
	if err == nil || !strings.Contains(err.Error(), "NOT NULL constraint failed: Post__Post.views") {
		t.Fatalf("the old insert: %v", err)
	}
	if _, err := q.ExecContext(ctx, `INSERT INTO "Post__Post" ("id", "title", "views") VALUES (?, ?, ?)`, "2", "b", 5); err != nil {
		t.Fatal(err)
	}
	if got := titles(t, s, nil); len(got) != 2 {
		t.Fatalf("the old select: %v", got)
	}
	rows, err := s.Query(ctx, nil, "Post.Post", postV3.Columns, SQL{Order: `"id"`})
	if err != nil || len(rows) != 2 || rows[0][3] != int64(0) || rows[1][3] != int64(5) {
		t.Fatalf("after the migration: %v %v", rows, err)
	}
}

// The remote store (Turso) caches nothing.
func TestStmtCache_RemoteCachesNothing(t *testing.T) {
	f := tursofake.New(t, "acme")
	st, err := OpenSQLite(tursoEnv(t, f))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	if err := st.Ensure(ctx, []TableDesc{postV1}, false); err != nil {
		t.Fatal(err)
	}
	insertPost(t, st, nil, "1", "a")
	if got := titles(t, st, nil); len(got) != 1 || stmtCount(st) != 0 {
		t.Fatalf("%v, %d cached", got, stmtCount(st))
	}
}

// ":memory:" holds one connection, which a transaction holds: the transaction prepares its own
// statements (a statement prepared on the pool would wait for the connection forever), and the
// store's statements prepared before the transaction run on it too.
func TestStmtCache_MemoryInTransaction(t *testing.T) {
	s := openMem(t)
	ctx := context.Background()
	if err := s.Ensure(ctx, []TableDesc{postV1}, false); err != nil {
		t.Fatal(err)
	}
	insertPost(t, s, nil, "1", "a")
	titles(t, s, nil) // cached on the store
	tx, err := s.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	insertPost(t, s, tx, "2", "b")
	insertPost(t, s, tx, "3", "c")
	if got := titles(t, s, tx); len(got) != 3 {
		t.Fatal(got)
	}
	if _, found, err := s.Get(ctx, tx, "Post.Post", "2"); err != nil || !found {
		t.Fatalf("get: %v %v", found, err)
	}
	if n := len(tx.(*sqliteTx).stmts); n == 0 {
		t.Error("the transaction prepared nothing")
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if got := titles(t, s, nil); len(got) != 1 {
		t.Fatal(got)
	}
}

// Concurrent queries through the cache are safe (run with -race).
func TestStmtCache_Concurrent(t *testing.T) {
	s := openFile(t)
	ctx := context.Background()
	if err := s.Ensure(ctx, []TableDesc{postV1}, false); err != nil {
		t.Fatal(err)
	}
	insertPost(t, s, nil, "1", "a")
	var wg sync.WaitGroup
	errs := make(chan error, 64)
	for g := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range 40 {
				// a few shared texts and some of each goroutine's own, past the cap
				text := fmt.Sprintf(`SELECT "id", "title" FROM "Post__Post" WHERE "id" = ? AND %d = %d`, i%5, i%5)
				if i%2 == 0 {
					text = fmt.Sprintf(`SELECT "id", "title" FROM "Post__Post" WHERE "id" = ? AND %d = %d`, g*1000+i, g*1000+i)
				}
				q, _ := s.on(nil)
				var id, title string
				if err := q.QueryRowContext(ctx, text, "1").Scan(&id, &title); err != nil {
					errs <- err
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

// BenchmarkSQLiteGetByID is a select by id of a 6-column row among 1,000, in a file database:
// unprepared (database/sql's QueryContext, as before the cache), through the cache, and inside
// a transaction both ways (docs/perf/2026-10-bench-baseline.md).
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
	b.Run("unprepared", func(b *testing.B) {
		for i := 0; b.Loop(); i++ {
			rows, err := s.db.QueryContext(ctx, text, fmt.Sprint(i%1000))
			scan(b, rows, err)
		}
	})
	b.Run("cached", func(b *testing.B) {
		q, _ := s.on(nil)
		for i := 0; b.Loop(); i++ {
			rows, err := q.QueryContext(ctx, text, fmt.Sprint(i%1000))
			scan(b, rows, err)
		}
	})
	b.Run("tx-unprepared", func(b *testing.B) {
		tx, _ := s.Begin(ctx)
		defer tx.Rollback()
		for i := 0; b.Loop(); i++ {
			rows, err := tx.(*sqliteTx).tx.QueryContext(ctx, text, fmt.Sprint(i%1000))
			scan(b, rows, err)
		}
	})
	b.Run("tx-cached", func(b *testing.B) {
		tx, _ := s.Begin(ctx)
		defer tx.Rollback()
		q, _ := s.on(tx)
		for i := 0; b.Loop(); i++ {
			rows, err := q.QueryContext(ctx, text, fmt.Sprint(i%1000))
			scan(b, rows, err)
		}
	})
}
