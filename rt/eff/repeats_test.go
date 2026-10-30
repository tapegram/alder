package eff

import (
	"context"
	"strings"
	"testing"
)

// sel is n records of statement stmt, the i-th with argument args[i%len(args)], in group g.
func sel(stmt string, n int, g int, args ...any) []SQLRec {
	var out []SQLRec
	for i := 0; i < n; i++ {
		out = append(out, SQLRec{Stmt: stmt, Args: []any{args[i%len(args)]}, Ms: 0.5, Group: g})
	}
	return out
}

const byID = `SELECT "id", "title", "n" FROM "Table__Row" WHERE "id" = ?`

func TestRepeats_Threshold(t *testing.T) {
	if got := Repeats(sel(byID, 4, 0, "a", "b", "c", "d"), RepeatThreshold); len(got) != 0 {
		t.Fatalf("4 selects: %v", got)
	}
	got := Repeats(sel(byID, 5, 0, "a", "b", "c", "d", "e"), RepeatThreshold)
	if len(got) != 1 || got[0].Count != 5 || got[0].Args != 5 || got[0].Stmt != byID || got[0].Ms != 2.5 {
		t.Fatalf("5 selects: %+v", got)
	}
}

func TestRepeats_Args(t *testing.T) {
	got := Repeats(sel(byID, 5, 0, "a", "b", "c"), RepeatThreshold)
	if len(got) != 1 || got[0].Args != 3 {
		t.Fatalf("%+v", got)
	}
}

func TestRepeats_BulkWrites(t *testing.T) {
	del := `DELETE FROM "Table__Row" WHERE "id" = ?`
	if got := Repeats(sel(del, 5, 1, "a", "b", "c", "d", "e"), RepeatThreshold); len(got) != 0 {
		t.Fatalf("5 deletes under one Task.all: %v", got)
	}
	two := append(sel(del, 3, 1, "a", "b", "c"), sel(del, 2, 2, "d", "e")...)
	if got := Repeats(two, RepeatThreshold); len(got) != 1 || got[0].Count != 5 {
		t.Fatalf("5 deletes in two groups: %v", got)
	}
	if got := Repeats(sel(del, 5, 0, "a"), RepeatThreshold); len(got) != 1 {
		t.Fatalf("5 deletes outside a Task.all: %v", got)
	}
	if got := Repeats(sel(byID, 5, 1, "a", "b", "c", "d", "e"), RepeatThreshold); len(got) != 1 {
		t.Fatalf("5 selects under one Task.all: %v", got)
	}
}

func TestRepeats_Order(t *testing.T) {
	other := `SELECT "id" FROM "Table__Tag" WHERE "row" = ?`
	recs := append(sel(byID, 5, 0, "a"), sel(other, 7, 0, "b")...)
	got := Repeats(recs, RepeatThreshold)
	if len(got) != 2 || got[0].Stmt != other || got[0].Count != 7 || got[1].Count != 5 {
		t.Fatalf("%+v", got)
	}
}

func TestRepeat_String(t *testing.T) {
	r := Repeat{Stmt: `SELECT "id", "title", "n", "done", "created" FROM "Table__Row" WHERE "id" = ?`, Count: 100, Args: 100, Ms: 3.214}
	want := `N+1: SELECT "id", "title", … FROM "Table__Row" WHERE "id" = ? ran 100 times (100 argument sets, 3.21 ms): one Db.get per row; read them together with Db.where (Row.id.in ids), or in the page's first query`
	if got := r.String(); got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
	r = Repeat{Stmt: `SELECT "id" FROM "Table__Tag" WHERE "row" = ?`, Count: 5, Args: 2, Ms: 1}
	want = `N+1: SELECT "id" FROM "Table__Tag" WHERE "row" = ? ran 5 times (2 argument sets, 1.00 ms): one query per row; read them together with Db.where (Tag.row.in values), or in the page's first query`
	if got := r.String(); got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
}

// The same query with the same arguments: the hint says so.
func TestRepeat_StringOneArgumentSet(t *testing.T) {
	r := Repeat{Stmt: byID, Count: 6, Args: 1, Ms: 1}
	if got := r.String(); !strings.HasSuffix(got, "(1 argument set, 1.00 ms): the same query ran 6 times with the same arguments; run it once and pass its result along") {
		t.Fatal(got)
	}
}

// `IN (?, ?)` and `IN (?, ?, ?)` are one shape.
func TestRepeats_InListsOneShape(t *testing.T) {
	var recs []SQLRec
	for n := 2; n <= 6; n++ {
		recs = append(recs, SQLRec{Stmt: `SELECT "id" FROM "T" WHERE "id" IN (` + strings.TrimSuffix(strings.Repeat("?, ", n), ", ") + `)`, Args: []any{n}})
	}
	got := Repeats(recs, RepeatThreshold)
	if len(got) != 1 || got[0].Count != 5 || got[0].Stmt != `SELECT "id" FROM "T" WHERE "id" IN (?, …)` {
		t.Fatalf("%+v", got)
	}
}

// Each load run is counted on its own: a load that runs a select 3 times, run twice (a
// Rerender), is no repeat; statements between loads (update's) are a part of their own.
func TestRepeatsCut(t *testing.T) {
	env := &Env{Ctx: context.Background(), Rec: &Recorder{}}
	load := Q(Call("Db.all", false, func(e *Env) (int, error) {
		for range 3 {
			e.Rec.AddSQL(SQLRec{Stmt: "SELECT all", Args: []any{}})
		}
		return 0, nil
	}))
	for range 2 {
		if _, err := RunQuery(env, load); err != nil {
			t.Fatal(err)
		}
		env.Rec.AddSQL(SQLRec{Stmt: "SELECT all", Args: []any{}}) // update's own, between
	}
	if got := RepeatsCut(env.Rec.SQL, env.Rec.Cuts, RepeatThreshold); len(got) != 0 {
		t.Fatalf("%+v (cuts %v)", got, env.Rec.Cuts)
	}
	if got := Repeats(env.Rec.SQL, RepeatThreshold); len(got) != 1 {
		t.Fatalf("uncut: %+v", got)
	}
	for range 5 {
		env.Rec.AddSQL(SQLRec{Stmt: "SELECT all", Args: []any{}})
	}
	if got := RepeatsCut(env.Rec.SQL, env.Rec.Cuts, RepeatThreshold); len(got) != 1 || got[0].Count != 6 {
		t.Fatalf("after 5 more: %+v", got)
	}
}

func TestRunQuery_CountsLoads(t *testing.T) {
	env := &Env{Ctx: context.Background(), Rec: &Recorder{}}
	for i := 0; i < 2; i++ {
		if _, err := RunQuery(env, Q(Pure(i))); err != nil {
			t.Fatal(err)
		}
	}
	if env.Rec.Loads != 2 {
		t.Fatalf("loads %d", env.Rec.Loads)
	}
	if _, err := RunQuery(&Env{Ctx: context.Background()}, Q(Pure(1))); err != nil { // no Recorder: nothing counted
		t.Fatal(err)
	}
}
