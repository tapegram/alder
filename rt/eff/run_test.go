package eff_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"alder/rt/db"
	"alder/rt/eff"
)

// row and tb: the table generated code would emit for `table T { id: TId @id, title: Text }`.
type row struct{ ID, Title string }

var tb = &db.Table[row]{
	Name:    "M.T",
	Columns: []db.Column{{Name: "id", Type: db.CText}, {Name: "title", Type: db.CText}},
	ID:      "id",
	Encode:  func(r row) []any { return []any{r.ID, r.Title} },
	Decode: func(v []any) (row, error) {
		id, err := db.DecodeText(v[0])
		if err != nil {
			return row{}, err
		}
		title, err := db.DecodeText(v[1])
		return row{id, title}, err
	},
	ReadID: func(r row) string { return r.ID },
	SetID:  func(r row, id string) row { r.ID = id; return r },
}

func insert(title string) eff.Task[string] {
	return eff.Call("Db.insert T", true, func(e *eff.Env) (string, error) {
		return db.InsertRow(e.Ctx, e.Store.(db.Store), db.TxOf(e), tb, []any{nil, title}, e.Now())
	})
}

func count() eff.Task[int64] {
	return eff.Call("Db.count T", false, func(e *eff.Env) (int64, error) {
		return db.CountRows(e.Ctx, e.Store.(db.Store), db.TxOf(e), db.From(tb))
	})
}

func then[A, B any](a eff.Task[A], b eff.Task[B]) eff.Task[B] {
	return eff.Bind(a, func(A) eff.Task[B] { return b })
}

// recSP records the savepoint statements the store runs.
type recSP struct {
	*db.SQLite
	log []string
}

func (r *recSP) Savepoint(ctx context.Context, tx eff.Tx, name string) error {
	r.log = append(r.log, "SAVEPOINT")
	return r.SQLite.Savepoint(ctx, tx, name)
}

func (r *recSP) Release(ctx context.Context, tx eff.Tx, name string) error {
	r.log = append(r.log, "RELEASE")
	return r.SQLite.Release(ctx, tx, name)
}

func (r *recSP) RollbackTo(ctx context.Context, tx eff.Tx, name string) error {
	r.log = append(r.log, "ROLLBACK TO")
	return r.SQLite.RollbackTo(ctx, tx, name)
}

// outer opens a one-connection in-memory store with T, begins the outer transaction (as a
// test's Run does) and returns an Env factory for runs within it. Any attempt to open a second
// transaction or read outside the outer one would block on the single connection: ctx gives up
// after two seconds so such a bug fails instead of hanging.
func outer(t *testing.T) (*recSP, eff.Tx, func() *eff.Env) {
	t.Helper()
	s, err := db.OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	t.Cleanup(cancel)
	if err := s.Ensure(ctx, []db.TableDesc{tb.Desc()}, true); err != nil {
		t.Fatal(err)
	}
	tx, err := s.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { tx.Rollback() })
	rec := &recSP{SQLite: s}
	mk := func() *eff.Env {
		return &eff.Env{Ctx: ctx, Store: rec, Now: func() time.Time { return time.Unix(0, 0) },
			Within: &eff.Within{Tx: tx, Savepoints: rec}}
	}
	return rec, tx, mk
}

// outerCount counts T's rows as the outer transaction sees them.
func outerCount(t *testing.T, mk func() *eff.Env) int64 {
	t.Helper()
	n, err := eff.RunQuery(mk(), eff.Q(count()))
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestRun_WithinWritesAreTheOuterTransactions(t *testing.T) {
	rec, tx, mk := outer(t)
	if _, err := db.InsertRow(context.Background(), rec.SQLite, tx.(db.Tx), tb, []any{nil, "the test's"}, time.Unix(0, 0)); err != nil {
		t.Fatal(err)
	}
	env := mk()
	var seen eff.Tx
	n, err := eff.Run(env, eff.KTask, then(insert("a"), eff.Bind(eff.Call("tx", false, func(e *eff.Env) (int64, error) {
		seen, _ = e.Transaction()
		return 0, nil
	}), func(int64) eff.Task[int64] { return count() })))
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("inside: %d rows, want the test's and its own", n)
	}
	if seen != tx {
		t.Fatalf("Transaction() = %v, want the outer one", seen)
	}
	if env.Tx != nil {
		t.Fatal("env.Tx not restored")
	}
	if got := outerCount(t, mk); got != 2 {
		t.Fatalf("outer sees %d rows", got)
	}
	if want := []string{"SAVEPOINT", "RELEASE"}; !equal(rec.log, want) {
		t.Fatalf("savepoints = %v, want %v", rec.log, want)
	}
	// nothing was committed: rolling the outer transaction back leaves no rows
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	n, err = db.CountRows(context.Background(), rec.SQLite, nil, db.From(tb))
	if err != nil || n != 0 {
		t.Fatalf("after the outer rollback: %d rows, %v", n, err)
	}
}

func TestRun_WithinFailureUndoesOnlyItsWrites(t *testing.T) {
	rec, _, mk := outer(t)
	if _, err := eff.Run(mk(), eff.KTask, insert("kept")); err != nil {
		t.Fatal(err)
	}
	env := mk()
	env.Session = "before"
	_, err := eff.Run(env, eff.KTask, then(insert("gone"), then(eff.Call("Session.put", true, func(e *eff.Env) (int, error) { e.Session = "after"; return 0, nil }), eff.Fail[int]("nope"))))
	var f *eff.ErrFail
	if !errors.As(err, &f) || f.Msg != "nope" {
		t.Fatalf("err = %v", err)
	}
	if env.Session != "before" {
		t.Fatalf("session = %v", env.Session)
	}
	if got := outerCount(t, mk); got != 1 {
		t.Fatalf("outer sees %d rows, want 1", got)
	}
	if want := []string{"SAVEPOINT", "RELEASE", "SAVEPOINT", "ROLLBACK TO", "RELEASE"}; !equal(rec.log, want) {
		t.Fatalf("savepoints = %v, want %v", rec.log, want)
	}
}

func TestRun_WithinPanicUndoesItsWrites(t *testing.T) {
	_, _, mk := outer(t)
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("no panic")
			}
		}()
		eff.Run(mk(), eff.KTask, then(insert("gone"), eff.Call("boom", false, func(*eff.Env) (int, error) { panic("boom") })))
	}()
	if got := outerCount(t, mk); got != 0 {
		t.Fatalf("outer sees %d rows after a panic", got)
	}
}

func TestRun_WithinNests(t *testing.T) {
	rec, _, mk := outer(t)
	// a run within, whose Call runs two more within (another Env each, as two requests of a
	// step inside Test.run would): the failing inner one is undone, the others kept
	nested := func(title string, fail bool) eff.Task[int] {
		return eff.Call("nested "+title, true, func(e *eff.Env) (int, error) {
			t := then(insert(title), eff.Pure(0))
			if fail {
				t = then(insert(title), eff.Fail[int]("inner"))
			}
			_, err := eff.Run(mk(), eff.KTask, t)
			var f *eff.ErrFail
			if errors.As(err, &f) {
				return 0, nil
			}
			return 0, err
		})
	}
	_, err := eff.Run(mk(), eff.KTask, then(insert("outer"), then(nested("ok", false), nested("bad", true))))
	if err != nil {
		t.Fatal(err)
	}
	if got := outerCount(t, mk); got != 2 {
		t.Fatalf("outer sees %d rows, want 2 (outer, ok)", got)
	}
	// an enclosing run that fails undoes its nested runs' released writes too
	_, err = eff.Run(mk(), eff.KTask, then(nested("x", false), eff.Fail[int]("outer")))
	if err == nil {
		t.Fatal("no error")
	}
	if got := outerCount(t, mk); got != 2 {
		t.Fatalf("outer sees %d rows after the failed enclosing run, want 2", got)
	}
	if len(rec.log) == 0 {
		t.Fatal("no savepoints")
	}
}

func TestRun_WithinWithoutSavepoints(t *testing.T) {
	_, tx, mk := outer(t)
	env := mk()
	env.Within = &eff.Within{Tx: tx}
	if _, err := eff.Run(env, eff.KTask, insert("a")); err != nil {
		t.Fatal(err)
	}
	if _, err := eff.Run(env, eff.KTask, then(insert("b"), eff.Fail[int]("x"))); err == nil {
		t.Fatal("no error")
	}
	if got := outerCount(t, mk); got != 2 {
		t.Fatalf("outer sees %d rows: without savepoints a failure undoes nothing", got)
	}
}

func TestRun_WithinQueryRefusesWrites(t *testing.T) {
	_, _, mk := outer(t)
	_, err := eff.RunQuery(mk(), eff.Q(insert("x")))
	if !errors.Is(err, eff.ErrInternal) {
		t.Fatalf("err = %v", err)
	}
}

func TestSavepointName_Fresh(t *testing.T) {
	a, b := eff.SavepointName(), eff.SavepointName()
	if a == b || a[:7] != "alder_t" {
		t.Fatalf("%s %s", a, b)
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
