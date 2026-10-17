package db

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"alder/rt/eff"
)

// item is what generated code would emit for
//
//	table Item { id: ItemId @id, title: Text, owner: User.UserId @ref User @owner,
//	             updated: Time @updated }
//
// with App's `session { user: Maybe User.UserId }` (here: itemSession).
type item struct {
	ID, Title, Owner string
	Updated          time.Time
}

type itemSession struct{ user string } // "" is Nothing

var itemTable = &Table[item]{
	Name:    "Item.Item",
	Columns: []Column{{Name: "id", Type: CText}, {Name: "title", Type: CText}, {Name: "owner", Type: CText}, {Name: "updated", Type: CTime}},
	ID:      "id",
	Updated: "updated",
	Encode:  func(r item) []any { return []any{r.ID, r.Title, r.Owner, EncodeTime(r.Updated)} },
	Decode: func(v []any) (r item, err error) {
		if r.ID, err = DecodeText(v[0]); err != nil {
			return r, err
		}
		if r.Title, err = DecodeText(v[1]); err != nil {
			return r, err
		}
		if r.Owner, err = DecodeText(v[2]); err != nil {
			return r, err
		}
		r.Updated, err = DecodeTime(v[3])
		return r, err
	},
	ReadID: func(r item) string { return r.ID },
	SetID:  func(r item, id string) item { r.ID = id; return r },
	Owner:  "owner",
	User: func(s any) (string, bool) {
		a, ok := s.(itemSession)
		return a.user, ok && a.user != ""
	},
}

func openItems(t *testing.T) *SQLite {
	t.Helper()
	s, err := OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if err := s.Ensure(context.Background(), []TableDesc{itemTable.Desc()}, true); err != nil {
		t.Fatal(err)
	}
	return s
}

// put stores an item of owner directly (no session), returning its id.
func put(t *testing.T, s *SQLite, title, owner string) string {
	t.Helper()
	id, err := InsertRow(context.Background(), s, nil, itemTable, []any{nil, title, owner, nil}, clock)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func itemEnv(s *SQLite, user string) *eff.Env {
	return &eff.Env{Ctx: context.Background(), Store: s, Now: func() time.Time { return clock }, Session: itemSession{user}}
}

func titleOf(t *testing.T, s *SQLite, id string) string {
	t.Helper()
	r, err := GetRow(context.Background(), s, nil, itemTable, id)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Ok {
		return "(gone)"
	}
	return r.Val.Title
}

func call[T any](name string, write bool, fn func(e *eff.Env) (T, error)) eff.Task[T] {
	return eff.Call(name, write, fn)
}

func TestMine_SelectsTheUsersRows(t *testing.T) {
	s := openItems(t)
	put(t, s, "a1", "ann")
	put(t, s, "b1", "bob")
	put(t, s, "a2", "ann")
	env := itemEnv(s, "ann")
	all := call("Db.all Item", false, func(e *eff.Env) ([]item, error) {
		return All(e.Ctx, e.Store.(Store), TxOf(e), OrderBy(Asc("title"), Mine(itemTable)))
	})
	got, err := eff.RunQuery(env, eff.Q(all))
	if err != nil {
		t.Fatal(err)
	}
	var titles []string
	for _, r := range got {
		titles = append(titles, r.Title)
	}
	if !slices.Equal(titles, []string{"a1", "a2"}) {
		t.Fatalf("Db.mine: %v", titles)
	}
	if !slices.Equal(env.Trace, []string{"Db.mine Item", "Db.all Item"}) {
		t.Fatalf("trace %q", env.Trace)
	}
	// composes with where (the owner predicate is ANDed first) and count/first
	env.Trace = nil
	n, err := eff.RunQuery(env, eff.Q(call("Db.count Item", false, func(e *eff.Env) (int64, error) {
		return CountRows(e.Ctx, e.Store.(Store), TxOf(e), Where(CEq("title", "b1"), Mine(itemTable)))
	})))
	if err != nil || n != 0 {
		t.Fatalf("count of bob's title as ann: %d %v", n, err)
	}
	first, err := eff.RunQuery(env, eff.Q(call("Db.first Item", false, func(e *eff.Env) (string, error) {
		r, err := First(e.Ctx, e.Store.(Store), TxOf(e), OrderBy(Desc("title"), Mine(itemTable)))
		return r.Val.Title, err
	})))
	if err != nil || first != "a2" {
		t.Fatalf("first: %q %v", first, err)
	}
	if !slices.Equal(env.Trace, []string{"Db.mine Item", "Db.count Item", "Db.mine Item", "Db.first Item"}) {
		t.Fatalf("trace %q", env.Trace)
	}
	// an unresolved Db.mine select is never rendered
	if q := Mine(itemTable).SQL(); !errors.Is(q.Err, eff.ErrInternal) {
		t.Fatalf("SQL of a Mine select: %v", q.Err)
	}
}

// Without a signed-in user every scoped operation fails, naming itself; nothing is written.
func TestMine_NoSignedInUser(t *testing.T) {
	s := openItems(t)
	id := put(t, s, "a1", "ann")
	env := itemEnv(s, "")
	cases := []struct {
		want string
		task eff.Task[struct{}]
	}{
		{"Db.mine Item: no signed-in user", call("Db.all Item", false, func(e *eff.Env) (struct{}, error) {
			_, err := All(e.Ctx, e.Store.(Store), TxOf(e), Mine(itemTable))
			return struct{}{}, err
		})},
		{"Db.getMine Item: no signed-in user", call("Db.getMine Item", false, func(e *eff.Env) (struct{}, error) {
			_, err := GetMineRow(e.Ctx, e.Store.(Store), TxOf(e), itemTable, id)
			return struct{}{}, err
		})},
		{"Db.updateMine Item: no signed-in user", call("Db.updateMine Item", true, func(e *eff.Env) (struct{}, error) {
			return struct{}{}, UpdateMineRow(e.Ctx, e.Store.(Store), TxOf(e), itemTable, id, func(r item) item { r.Title = "x"; return r }, e.Now())
		})},
		{"Db.deleteMine Item: no signed-in user", call("Db.deleteMine Item", true, func(e *eff.Env) (struct{}, error) {
			return struct{}{}, DeleteMineRow(e.Ctx, e.Store.(Store), TxOf(e), itemTable, id)
		})},
		{"Db.insertMine Item: no signed-in user", call("Db.insertMine Item", true, func(e *eff.Env) (struct{}, error) {
			_, err := InsertMineRow(e.Ctx, e.Store.(Store), TxOf(e), itemTable, []any{nil, "x", nil, nil}, e.Now())
			return struct{}{}, err
		})},
	}
	for _, c := range cases {
		_, err := eff.Run(env, eff.KTask, c.task)
		var f *eff.ErrFail
		if !errors.As(err, &f) || f.Msg != c.want {
			t.Errorf("%s: err %v", c.want, err)
		}
	}
	if n := count2(t, s); n != 1 || titleOf(t, s, id) != "a1" {
		t.Fatalf("rows %d, title %q", n, titleOf(t, s, id))
	}
	// no Env at all (a context that carries none) is no user either
	if _, err := GetMineRow(context.Background(), s, nil, itemTable, id); err == nil || !strings.Contains(err.Error(), "no signed-in user") {
		t.Fatalf("no Env: %v", err)
	}
}

func count2(t *testing.T, s *SQLite) int64 {
	t.Helper()
	n, err := CountRows(context.Background(), s, nil, From(itemTable))
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestMine_GetMine(t *testing.T) {
	s := openItems(t)
	mine := put(t, s, "a1", "ann")
	theirs := put(t, s, "b1", "bob")
	env := itemEnv(s, "ann")
	get := func(id string) (item, error) {
		return eff.RunQuery(env, eff.Q(call("Db.getMine Item", false, func(e *eff.Env) (item, error) {
			return GetMineRow(e.Ctx, e.Store.(Store), TxOf(e), itemTable, id)
		})))
	}
	if r, err := get(mine); err != nil || r.Title != "a1" {
		t.Fatalf("mine: %+v %v", r, err)
	}
	if _, err := get(theirs); err != eff.ErrNotFound {
		t.Fatalf("theirs: %v", err)
	}
	if _, err := get("missing"); err != eff.ErrNotFound {
		t.Fatalf("missing: %v", err)
	}
}

// The store's predicate writes: one statement each, the rows affected counted.
func TestStore_UpdateWhereDeleteWhere(t *testing.T) {
	s := openItems(t)
	ctx := context.Background()
	id := put(t, s, "a1", "ann")
	vals := itemTable.Encode(item{ID: id, Title: "changed", Owner: "ann"})
	for _, c := range []struct {
		owner, id string
		want      int64
	}{{"bob", id, 0}, {"ann", "missing", 0}, {"ann", id, 1}} {
		n, err := s.UpdateWhere(ctx, nil, itemTable.Name, itemTable.Columns, "id", "updated", c.id, "owner", c.owner, vals, clock)
		if err != nil || n != c.want {
			t.Fatalf("UpdateWhere %s %s: %d %v, want %d", c.owner, c.id, n, err, c.want)
		}
		if want := map[int64]string{0: "a1", 1: "changed"}[n]; titleOf(t, s, id) != want {
			t.Fatalf("after UpdateWhere %s %s: %q", c.owner, c.id, titleOf(t, s, id))
		}
	}
	for _, c := range []struct {
		owner, id string
		want      int64
	}{{"bob", id, 0}, {"ann", "missing", 0}, {"ann", id, 1}} {
		n, err := s.DeleteWhere(ctx, nil, itemTable.Name, "id", c.id, "owner", c.owner)
		if err != nil || n != c.want {
			t.Fatalf("DeleteWhere %s %s: %d %v, want %d", c.owner, c.id, n, err, c.want)
		}
	}
	if titleOf(t, s, id) != "(gone)" {
		t.Fatal("the row is still there")
	}
	// a Query's store refuses both
	ro := readOnly{s, errQueryWrite}
	if _, err := ro.UpdateWhere(ctx, nil, itemTable.Name, itemTable.Columns, "id", "", id, "owner", "ann", vals, clock); !errors.Is(err, eff.ErrInternal) {
		t.Fatalf("UpdateWhere from a Query: %v", err)
	}
	if _, err := ro.DeleteWhere(ctx, nil, itemTable.Name, "id", id, "owner", "ann"); !errors.Is(err, eff.ErrInternal) {
		t.Fatalf("DeleteWhere from a Query: %v", err)
	}
}

func TestMine_UpdateDeleteInsert(t *testing.T) {
	s := openItems(t)
	mine := put(t, s, "a1", "ann")
	theirs := put(t, s, "b1", "bob")
	env := itemEnv(s, "ann")
	later := clock.Add(time.Hour)
	env.Now = func() time.Time { return later }
	update := func(id string) error {
		_, err := eff.Run(env, eff.KTask, call("Db.updateMine Item", true, func(e *eff.Env) (struct{}, error) {
			return struct{}{}, UpdateMineRow(e.Ctx, e.Store.(Store), TxOf(e), itemTable, id, func(r item) item { r.Title += "!"; return r }, e.Now())
		}))
		return err
	}
	if err := update(mine); err != nil || titleOf(t, s, mine) != "a1!" {
		t.Fatalf("update mine: %v %q", err, titleOf(t, s, mine))
	}
	if r, _ := GetRow(context.Background(), s, nil, itemTable, mine); !r.Val.Updated.Equal(later) {
		t.Fatalf("@updated %v", r.Val.Updated)
	}
	if err := update(theirs); err != eff.ErrNotFound || titleOf(t, s, theirs) != "b1" {
		t.Fatalf("update theirs: %v %q", err, titleOf(t, s, theirs))
	}
	if err := update("missing"); err != eff.ErrNotFound {
		t.Fatalf("update missing: %v", err)
	}
	// an f that changes the id fails
	_, err := eff.Run(env, eff.KTask, call("Db.updateMine Item", true, func(e *eff.Env) (struct{}, error) {
		return struct{}{}, UpdateMineRow(e.Ctx, e.Store.(Store), TxOf(e), itemTable, mine, func(r item) item { r.ID = "x"; return r }, e.Now())
	}))
	var f *eff.ErrFail
	if !errors.As(err, &f) || f.Msg != "Db.updateMine Item: update may not change the id" {
		t.Fatalf("id change: %v", err)
	}

	del := func(id string) error {
		_, err := eff.Run(env, eff.KTask, call("Db.deleteMine Item", true, func(e *eff.Env) (struct{}, error) {
			return struct{}{}, DeleteMineRow(e.Ctx, e.Store.(Store), TxOf(e), itemTable, id)
		}))
		return err
	}
	if err := del(theirs); err != eff.ErrNotFound || titleOf(t, s, theirs) != "b1" {
		t.Fatalf("delete theirs: %v", err)
	}
	if err := del(mine); err != nil || titleOf(t, s, mine) != "(gone)" {
		t.Fatalf("delete mine: %v", err)
	}
	if err := del(mine); err != eff.ErrNotFound {
		t.Fatalf("delete again: %v", err)
	}

	id, err := eff.Run(env, eff.KTask, call("Db.insertMine Item", true, func(e *eff.Env) (string, error) {
		return InsertMineRow(e.Ctx, e.Store.(Store), TxOf(e), itemTable, []any{nil, "new", nil, nil}, e.Now())
	}))
	if err != nil {
		t.Fatal(err)
	}
	if r, _ := GetRow(context.Background(), s, nil, itemTable, id); !r.Ok || r.Val.Owner != "ann" || r.Val.Title != "new" {
		t.Fatalf("insertMine: %+v", r)
	}
}

// RF2: a scoped update of another user's row inside a Task that wrote first is not found, and
// the whole transaction rolls back: the earlier insert is gone, the other row unchanged.
func TestMine_UpdateOfAnothersRowRollsBackTheTask(t *testing.T) {
	s := openItems(t)
	theirs := put(t, s, "b1", "bob")
	env := itemEnv(s, "ann")
	ins := call("Db.insertMine Item", true, func(e *eff.Env) (string, error) {
		return InsertMineRow(e.Ctx, e.Store.(Store), TxOf(e), itemTable, []any{nil, "first", nil, nil}, e.Now())
	})
	task := eff.Bind(ins, func(string) eff.Task[struct{}] {
		return call("Db.updateMine Item", true, func(e *eff.Env) (struct{}, error) {
			return struct{}{}, UpdateMineRow(e.Ctx, e.Store.(Store), TxOf(e), itemTable, theirs, func(r item) item { r.Title = "stolen"; return r }, e.Now())
		})
	})
	if _, err := eff.Run(env, eff.KTask, task); err != eff.ErrNotFound {
		t.Fatalf("err %v, want not found", err)
	}
	if n := count2(t, s); n != 1 {
		t.Fatalf("%d rows: the insert was not rolled back", n)
	}
	if titleOf(t, s, theirs) != "b1" {
		t.Fatalf("their row changed: %q", titleOf(t, s, theirs))
	}
	if !slices.Equal(env.Trace, []string{"Db.insertMine Item", "Db.updateMine Item"}) {
		t.Fatalf("trace %q", env.Trace)
	}
}

// Db.updateMine never writes the owner column: an f that gives the row away changes the other
// fields only.
func TestMine_UpdateKeepsTheOwner(t *testing.T) {
	s := openItems(t)
	mine := put(t, s, "a1", "ann")
	env := itemEnv(s, "ann")
	_, err := eff.Run(env, eff.KTask, call("Db.updateMine Item", true, func(e *eff.Env) (struct{}, error) {
		return struct{}{}, UpdateMineRow(e.Ctx, e.Store.(Store), TxOf(e), itemTable, mine, func(r item) item { r.Title = "moved"; r.Owner = "bob"; return r }, e.Now())
	}))
	if err != nil {
		t.Fatal(err)
	}
	r, _ := GetRow(context.Background(), s, nil, itemTable, mine)
	if r.Val.Owner != "ann" || r.Val.Title != "moved" {
		t.Fatalf("after an owner-changing fn: %+v", r.Val)
	}
}

// A bulk delete is Task.all of one Db.delete per id: each is its own step, but the statements
// share a Recorder group (the dev panel labels them bulk); one outside a Task.all has none.
func TestRecorderGroupsTaskAll(t *testing.T) {
	s := openItems(t)
	ids := []string{put(t, s, "a1", "ann"), put(t, s, "a2", "ann"), put(t, s, "a3", "ann"), put(t, s, "a4", "ann")}
	env := itemEnv(s, "ann")
	env.Rec = &eff.Recorder{}
	del := func(id string) eff.Task[struct{}] {
		return call("Db.delete Item", true, func(e *eff.Env) (struct{}, error) {
			return struct{}{}, DeleteRow(e.Ctx, e.Store.(Store), TxOf(e), itemTable, id)
		})
	}
	var tasks []eff.Task[struct{}]
	for _, id := range ids[:3] {
		tasks = append(tasks, del(id))
	}
	if _, err := eff.Run(env, eff.KTask, eff.All(tasks)); err != nil {
		t.Fatal(err)
	}
	if _, err := eff.Run(env, eff.KTask, del(ids[3])); err != nil {
		t.Fatal(err)
	}
	sql := env.Rec.SQL
	if len(sql) != 4 || len(env.Rec.Steps) != 4 {
		t.Fatalf("%d statements, %d steps", len(sql), len(env.Rec.Steps))
	}
	if sql[0].Group == 0 || sql[1].Group != sql[0].Group || sql[2].Group != sql[0].Group || sql[3].Group != 0 {
		t.Errorf("groups: %d %d %d %d", sql[0].Group, sql[1].Group, sql[2].Group, sql[3].Group)
	}
}
