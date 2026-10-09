package db

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"alder/rt"
	"alder/rt/eff"
)

// todo is what generated code would emit for
//
//	table Todo { id: TodoId @id, title: Text @unique, done: Bool, due: Maybe Date,
//	             created: Time @created, updated: Time @updated, prio: Prio }
type todo struct {
	ID      string
	Title   string
	Done    bool
	Due     rt.Maybe[rt.Date]
	Created time.Time
	Updated time.Time
	Prio    string // Low | Normal | High
}

func todoColumns() []Column {
	return []Column{
		{Name: "id", Type: CText},
		{Name: "title", Type: CText, Unique: true},
		{Name: "done", Type: CBool},
		{Name: "due", Type: CDate, Nullable: true},
		{Name: "created", Type: CTime},
		{Name: "updated", Type: CTime},
		{Name: "prio", Type: CEnum, Enum: []string{"Low", "Normal", "High"}},
	}
}

var todoTable = &Table[todo]{
	Name:    "Todo.Todo",
	Columns: todoColumns(),
	ID:      "id",
	Created: "created",
	Updated: "updated",
	Encode: func(t todo) []any {
		return []any{
			t.ID, t.Title, EncodeBool(t.Done),
			EncodeMaybe(t.Due, func(d rt.Date) any { return EncodeDate(d) }),
			EncodeTime(t.Created), EncodeTime(t.Updated), t.Prio,
		}
	},
	Decode: func(v []any) (todo, error) {
		var t todo
		var err error
		if len(v) != 7 {
			return t, errors.New("todo: wrong column count")
		}
		if t.ID, err = DecodeText(v[0]); err != nil {
			return t, err
		}
		if t.Title, err = DecodeText(v[1]); err != nil {
			return t, err
		}
		if t.Done, err = DecodeBool(v[2]); err != nil {
			return t, err
		}
		if t.Due, err = DecodeMaybe(v[3], DecodeDate); err != nil {
			return t, err
		}
		if t.Created, err = DecodeTime(v[4]); err != nil {
			return t, err
		}
		if t.Updated, err = DecodeTime(v[5]); err != nil {
			return t, err
		}
		if t.Prio, err = DecodeText(v[6]); err != nil {
			return t, err
		}
		return t, nil
	},
	ReadID: func(t todo) string { return t.ID },
	SetID:  func(t todo, id string) todo { t.ID = id; return t },
}

// insertVals is what generated code passes for an Insert record: nil at id/created/updated.
func insertVals(title string, done bool, due rt.Maybe[rt.Date], prio string) []any {
	return []any{nil, title, EncodeBool(done),
		EncodeMaybe(due, func(d rt.Date) any { return EncodeDate(d) }), nil, nil, prio}
}

var clock = time.Date(2026, 10, 9, 12, 0, 0, 123456789, time.UTC)

func open(t *testing.T) *SQLite {
	t.Helper()
	s, err := OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if err := s.Ensure(context.Background(), []TableDesc{todoTable.Desc()}, true); err != nil {
		t.Fatal(err)
	}
	return s
}

func newEnv(s *SQLite) *eff.Env {
	return &eff.Env{Ctx: context.Background(), Store: s, Now: func() time.Time { return clock }}
}

func insertTask(t *Table[todo], vals []any) eff.Task[string] {
	return eff.Call("Db.insert Todo", true, func(e *eff.Env) (string, error) {
		return InsertRow(e.Ctx, e.Store.(Store), TxOf(e), t, vals, e.Now())
	})
}

func count(t *testing.T, s *SQLite, sel Select[todo]) int64 {
	t.Helper()
	n, err := CountRows(context.Background(), s, nil, sel)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestStore_InsertGetRoundTrip(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	id, err := InsertRow(ctx, s, nil, todoTable, insertVals("milk", true, rt.Nothing[rt.Date](), "High"), clock)
	if err != nil {
		t.Fatal(err)
	}
	if len(id) != 20 {
		t.Fatalf("id %q: want 20 chars", id)
	}
	id2, err := InsertRow(ctx, s, nil, todoTable, insertVals("eggs", false, rt.Just(rt.Date{Y: 2026, M: 2, D: 28}), "Low"), clock)
	if err != nil {
		t.Fatal(err)
	}
	if id2 == id {
		t.Fatal("ids not unique")
	}
	got, err := GetRow(ctx, s, nil, todoTable, id)
	if err != nil {
		t.Fatal(err)
	}
	want := todo{ID: id, Title: "milk", Done: true, Created: clock, Updated: clock, Prio: "High"}
	if !got.Ok || !rt.Eq(got.Val, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	got2, err := GetRow(ctx, s, nil, todoTable, id2)
	if err != nil {
		t.Fatal(err)
	}
	if !got2.Ok || !rt.Eq(got2.Val.Due, rt.Just(rt.Date{Y: 2026, M: 2, D: 28})) || got2.Val.Done || got2.Val.Prio != "Low" {
		t.Fatalf("got %+v", got2)
	}
	var stored any
	if err := s.db.QueryRow(`SELECT "due" FROM "Todo__Todo" WHERE "id" = ?`, id2).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != "2026-02-28" {
		t.Fatalf("stored due %#v", stored)
	}
	missing, err := GetRow(ctx, s, nil, todoTable, "nope")
	if err != nil || missing.Ok {
		t.Fatalf("missing: %+v %v", missing, err)
	}
	// Decode reports a type mismatch instead of panicking.
	if _, err := todoTable.Decode([]any{int64(1), "x", int64(0), nil, "bad", "bad", "Low"}); err == nil {
		t.Fatal("decode of int id: want error")
	}
	if _, err := DecodeBool("yes"); err == nil {
		t.Fatal("DecodeBool(string): want error")
	}
}

func TestStore_TimeRoundTripUTC(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	tokyo, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		t.Skip("no tzdata:", err)
	}
	created := time.Date(2026, 10, 9, 21, 30, 5, 987654321, tokyo)
	id, err := InsertRow(ctx, s, nil, todoTable, insertVals("t", false, rt.Nothing[rt.Date](), "Low"), created)
	if err != nil {
		t.Fatal(err)
	}
	got, err := GetRow(ctx, s, nil, todoTable, id)
	if err != nil || !got.Ok {
		t.Fatal(got, err)
	}
	if !got.Val.Created.Equal(created) || !rt.Eq(got.Val.Created, created.UTC()) {
		t.Fatalf("created %v, want %v", got.Val.Created, created)
	}
	var text string
	if err := s.db.QueryRow(`SELECT "created" FROM "Todo__Todo" WHERE "id" = ?`, id).Scan(&text); err != nil {
		t.Fatal(err)
	}
	if text != "2026-10-09T12:30:05.987654321Z" {
		t.Fatalf("stored %q", text)
	}
	var done any
	if err := s.db.QueryRow(`SELECT "done" FROM "Todo__Todo" WHERE "id" = ?`, id).Scan(&done); err != nil {
		t.Fatal(err)
	}
	if done != int64(0) {
		t.Fatalf("stored done %#v, want int64 0", done)
	}
}

func TestStore_UniqueViolationFailsTask(t *testing.T) {
	s := open(t)
	env := newEnv(s)
	task := eff.Bind(insertTask(todoTable, insertVals("a", false, rt.Nothing[rt.Date](), "Low")), func(string) eff.Task[string] {
		return insertTask(todoTable, insertVals("a", true, rt.Nothing[rt.Date](), "High"))
	})
	_, err := eff.Run(env, eff.KTask, task)
	var f *eff.ErrFail
	if !errors.As(err, &f) {
		t.Fatalf("err %v (%T): want *eff.ErrFail", err, err)
	}
	if f.Msg != "Todo.title must be unique" {
		t.Fatalf("msg %q", f.Msg)
	}
	if n := count(t, s, From(todoTable)); n != 0 {
		t.Fatalf("count %d after rollback", n)
	}
	// Update into a duplicate fails the same way.
	ctx := context.Background()
	if _, err := InsertRow(ctx, s, nil, todoTable, insertVals("a", false, rt.Nothing[rt.Date](), "Low"), clock); err != nil {
		t.Fatal(err)
	}
	idB, err := InsertRow(ctx, s, nil, todoTable, insertVals("b", false, rt.Nothing[rt.Date](), "Low"), clock)
	if err != nil {
		t.Fatal(err)
	}
	err = UpdateRow(ctx, s, nil, todoTable, idB, func(x todo) todo { x.Title = "a"; return x }, clock)
	if !errors.As(err, &f) || !strings.Contains(f.Msg, "title") {
		t.Fatalf("update dup: %v", err)
	}
}

func TestStore_RollbackLeavesNoRow(t *testing.T) {
	s := open(t)
	env := newEnv(s)
	task := eff.Bind(insertTask(todoTable, insertVals("a", false, rt.Nothing[rt.Date](), "Low")), func(string) eff.Task[int] {
		return eff.Fail[int]("nope")
	})
	if _, err := eff.Run(env, eff.KTask, task); err == nil {
		t.Fatal("want failure")
	}
	if n := count(t, s, From(todoTable)); n != 0 {
		t.Fatalf("count %d", n)
	}
	// And a successful task commits.
	if _, err := eff.Run(env, eff.KTask, insertTask(todoTable, insertVals("b", false, rt.Nothing[rt.Date](), "Low"))); err != nil {
		t.Fatal(err)
	}
	if n := count(t, s, From(todoTable)); n != 1 {
		t.Fatalf("count %d after commit", n)
	}
}

func TestStore_UpdateRejectsIdChange(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	id, err := InsertRow(ctx, s, nil, todoTable, insertVals("a", false, rt.Nothing[rt.Date](), "Low"), clock)
	if err != nil {
		t.Fatal(err)
	}
	err = UpdateRow(ctx, s, nil, todoTable, id, func(x todo) todo { x.ID = "other"; x.Title = "changed"; return x }, clock)
	var f *eff.ErrFail
	if !errors.As(err, &f) || !strings.Contains(f.Msg, "may not change the id") {
		t.Fatalf("err %v", err)
	}
	got, _ := GetRow(ctx, s, nil, todoTable, id)
	if !got.Ok || got.Val.Title != "a" {
		t.Fatalf("row changed: %+v", got)
	}
	// Updating a missing row fails too.
	err = UpdateRow(ctx, s, nil, todoTable, "missing", func(x todo) todo { return x }, clock)
	if !errors.As(err, &f) {
		t.Fatalf("missing row: %v", err)
	}
}

func TestStore_UpdateSetsUpdated(t *testing.T) {
	s := open(t)
	env := newEnv(s)
	id, err := eff.Run(env, eff.KTask, insertTask(todoTable, insertVals("a", false, rt.Nothing[rt.Date](), "Low")))
	if err != nil {
		t.Fatal(err)
	}
	later := clock.Add(time.Hour)
	env.Now = func() time.Time { return later }
	upd := eff.Call("Db.update Todo", true, func(e *eff.Env) (struct{}, error) {
		return struct{}{}, UpdateRow(e.Ctx, e.Store.(Store), TxOf(e), todoTable, id, func(x todo) todo {
			x.Done = true
			x.Created = time.Time{} // ignored: @created is never rewritten
			return x
		}, e.Now())
	})
	if _, err := eff.Run(env, eff.KTask, upd); err != nil {
		t.Fatal(err)
	}
	got, err := GetRow(context.Background(), s, nil, todoTable, id)
	if err != nil || !got.Ok {
		t.Fatal(got, err)
	}
	if !got.Val.Done || !got.Val.Updated.Equal(later) || !got.Val.Created.Equal(clock) {
		t.Fatalf("got %+v", got.Val)
	}
}

func TestStore_SelectSQL(t *testing.T) {
	render := func(q SQL) string {
		out := q.Where
		if q.Order != "" {
			out += " ORDER BY " + q.Order
		}
		if q.Limit != nil {
			out += " LIMIT " + strconv.FormatInt(*q.Limit, 10)
		}
		if q.Offset != nil {
			out += " OFFSET " + strconv.FormatInt(*q.Offset, 10)
		}
		return out
	}
	base := From(todoTable)
	cases := []struct {
		name string
		sel  Select[todo]
		want string
		args []any
	}{
		{"none", base, "", nil},
		{"eq desc limit", Limit(1, OrderBy(Desc("created"), Where(CEq("done", int64(1)), base))),
			`"done" = ? ORDER BY "created" DESC LIMIT 1`, []any{int64(1)}},
		{"contains", Where(CContains("title", `50%_off\`), base),
			`lower("title") LIKE lower(?) ESCAPE '\'`, []any{`%50\%\_off\\%`}},
		{"in empty", Where(CIn("id", nil), base), `0=1`, nil},
		{"in", Where(CIn("id", []any{"a", "b"}), base), `"id" IN (?, ?)`, []any{"a", "b"}},
		{"nesting", Where(CNot(COr([]Cond{CEq("title", "x"), CAnd([]Cond{CGt("created", "t"), CIsNull("due")})})), base),
			`NOT (("title" = ? OR ("created" > ? AND "due" IS NULL)))`, []any{"x", "t"}},
		{"two wheres", Where(CIsNotNull("due"), Where(CLe("created", "t"), base)),
			`("created" <= ? AND "due" IS NOT NULL)`, []any{"t"}},
		{"ops", Where(CAnd([]Cond{CNe("title", "x"), CLt("created", "a"), CGe("created", "b"), CEq("due", nil)}), base),
			`("title" IS NOT ? AND "created" < ? AND "created" >= ? AND "due" IS NULL)`, []any{"x", "a", "b"}},
		{"enum order + offset", Offset(5, OrderBy(Asc("title"), OrderBy(Desc("prio"), base))),
			`ORDER BY CASE "prio" WHEN 'Low' THEN 0 WHEN 'Normal' THEN 1 WHEN 'High' THEN 2 END DESC, "title" ASC OFFSET 5`, nil},
		{"quoted ident", Where(CEq(`we"ird`, "x"), base), `"we""ird" = ?`, []any{"x"}},
		{"enum compares by declaration", Where(CLt("prio", "High"), Where(CEq("prio", "Low"), base)),
			`("prio" = ? AND CASE "prio" WHEN 'Low' THEN 0 WHEN 'Normal' THEN 1 WHEN 'High' THEN 2 END < ?)`, []any{"Low", int64(2)}},
		{"and empty is true", Where(CAnd(nil), base), `1=1`, nil},
		{"or empty is false", Where(COr([]Cond{}), base), `0=1`, nil},
		{"not of empty or", Where(CNot(COr(nil)), base), `NOT (0=1)`, nil},
		// a nullable (Maybe) column: every condition is two-valued
		{"nullable eq", Where(CNot(CEq("due", "2026-01-01")), base), `NOT ("due" IS ?)`, []any{"2026-01-01"}},
		{"nullable ne", Where(CNe("due", "2026-01-01"), base), `"due" IS NOT ?`, []any{"2026-01-01"}},
		{"nullable in nothing", Where(CIn("due", []any{nil}), base), `"due" IS NULL`, nil},
		{"nullable in just and nothing", Where(CNot(CIn("due", []any{"a", nil})), base),
			`NOT (COALESCE((("due" IN (?) OR "due" IS NULL)), 0))`, []any{"a"}},
		{"nullable in just", Where(CNot(CIn("due", []any{"a"})), base), `NOT (COALESCE(("due" IN (?)), 0))`, []any{"a"}},
		{"nullable lt", Where(CNot(CLt("due", "a")), base), `NOT (COALESCE(("due" < ?), 0))`, []any{"a"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			q := c.sel.SQL()
			got := strings.TrimSpace(render(q))
			if got != c.want {
				t.Fatalf("got  %s\nwant %s", got, c.want)
			}
			if len(q.Args) != len(c.args) {
				t.Fatalf("args %#v, want %#v", q.Args, c.args)
			}
			for i := range c.args {
				if q.Args[i] != c.args[i] {
					t.Fatalf("args %#v, want %#v", q.Args, c.args)
				}
			}
		})
	}
	// Builders don't share state between derived selects.
	a := Where(CEq("done", int64(0)), base)
	b := Where(CEq("done", int64(1)), a)
	c := Where(CEq("title", "x"), a)
	if len(b.Where) != 2 || len(c.Where) != 2 || b.Where[1].Col != "done" || c.Where[1].Col != "title" {
		t.Fatal("selects share a backing array")
	}
}

// Conditions on a nullable column match by Alder's two-valued logic, not SQL's three-valued
// one: Db.not of a condition matches exactly the rows it does not (C1).
func TestStore_NullableConditions(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	d := func(day int) rt.Maybe[rt.Date] { return rt.Just(rt.Date{Y: 2026, M: 1, D: day}) }
	for i, due := range []rt.Maybe[rt.Date]{d(1), d(2), rt.Nothing[rt.Date]()} {
		if _, err := InsertRow(ctx, s, nil, todoTable, insertVals("t"+strconv.Itoa(i), false, due, "Low"), clock); err != nil {
			t.Fatal(err)
		}
	}
	enc := func(m rt.Maybe[rt.Date]) any { return EncodeMaybe(m, func(x rt.Date) any { return EncodeDate(x) }) }
	cases := []struct {
		name string
		c    Cond
		want string
	}{
		{"eq", CEq("due", enc(d(1))), "t0"},
		{"not eq", CNot(CEq("due", enc(d(1)))), "t1 t2"},
		{"in nothing", CIn("due", []any{enc(rt.Nothing[rt.Date]())}), "t2"},
		{"in just, nothing", CIn("due", []any{enc(d(1)), nil}), "t0 t2"},
		{"not in just, nothing", CNot(CIn("due", []any{enc(d(1)), nil})), "t1"},
		{"not in just", CNot(CIn("due", []any{enc(d(1))})), "t1 t2"},
		{"not lt", CNot(CLt("due", enc(d(2)))), "t1 t2"},
		{"not ne", CNot(CNe("due", enc(d(1)))), "t0"},
		{"or not", COr([]Cond{CNot(CGt("due", enc(d(1)))), CEq("title", "zz")}), "t0 t2"},
	}
	for _, c := range cases {
		rows, err := All(ctx, s, nil, OrderBy(Asc("title"), Where(c.c, From(todoTable))))
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, r := range rows {
			got = append(got, r.Title)
		}
		if strings.Join(got, " ") != c.want {
			t.Errorf("%s: got %v, want %s", c.name, got, c.want)
		}
	}
	// ordering by a Maybe column: Nothing first ascending, last descending (book chapter 8)
	for _, o := range []struct {
		ord  Order
		want string
	}{{Asc("due"), "t2 t0 t1"}, {Desc("due"), "t1 t0 t2"}} {
		rows, err := All(ctx, s, nil, OrderBy(o.ord, From(todoTable)))
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, r := range rows {
			got = append(got, r.Title)
		}
		if strings.Join(got, " ") != o.want {
			t.Errorf("order %+v: got %v, want %s", o.ord, got, o.want)
		}
	}
}

func TestStore_AllFirstCount(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	titles := []string{"first", "second", "third"}
	for i, title := range titles {
		if _, err := InsertRow(ctx, s, nil, todoTable, insertVals(title, i != 1, rt.Nothing[rt.Date](), "Normal"),
			clock.Add(time.Duration(i)*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	all, err := All(ctx, s, nil, OrderBy(Desc("created"), From(todoTable)))
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 || all[0].Title != "third" || all[1].Title != "second" || all[2].Title != "first" {
		t.Fatalf("all %+v", all)
	}
	first, err := First(ctx, s, nil, OrderBy(Asc("created"), From(todoTable)))
	if err != nil || !first.Ok || first.Val.Title != "first" {
		t.Fatalf("first %+v %v", first, err)
	}
	if zero, err := First(ctx, s, nil, Limit(0, From(todoTable))); err != nil || zero.Ok {
		t.Fatalf("limit 0 |> first = %+v %v, want Nothing", zero, err)
	}
	if two, err := First(ctx, s, nil, Limit(2, Offset(2, OrderBy(Asc("created"), From(todoTable))))); err != nil || !two.Ok || two.Val.Title != "third" {
		t.Fatalf("limit 2 offset 2 |> first = %+v %v", two, err)
	}
	none, err := First(ctx, s, nil, Where(CEq("title", "zzz"), From(todoTable)))
	if err != nil || none.Ok {
		t.Fatalf("none %+v %v", none, err)
	}
	if n := count(t, s, Where(CEq("done", EncodeBool(true)), From(todoTable))); n != 2 {
		t.Fatalf("count done %d", n)
	}
	if n := count(t, s, Where(CContains("title", "IR"), From(todoTable))); n != 2 { // fIRst, thIRd
		t.Fatalf("count contains %d", n)
	}
	if n := count(t, s, Limit(1, Offset(1, From(todoTable)))); n != 1 {
		t.Fatalf("count with limit %d", n)
	}
	page, err := All(ctx, s, nil, Offset(1, OrderBy(Asc("created"), From(todoTable))))
	if err != nil || len(page) != 2 || page[0].Title != "second" {
		t.Fatalf("offset only %+v %v", page, err)
	}
	empty, err := All(ctx, s, nil, Where(CIn("title", nil), From(todoTable)))
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("empty %#v %v", empty, err)
	}
}

func TestStore_EnsureRecreatesInDev(t *testing.T) {
	ctx := context.Background()
	path := t.TempDir() + "/app.db"
	s, err := OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	a := todoTable.Desc()
	if err := s.Ensure(ctx, []TableDesc{a}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := InsertRow(ctx, s, nil, todoTable, insertVals("a", false, rt.Nothing[rt.Date](), "Low"), clock); err != nil {
		t.Fatal(err)
	}
	// Same schema again: nothing happens, data kept.
	if err := s.Ensure(ctx, []TableDesc{a}, false); err != nil {
		t.Fatal(err)
	}
	if n := count(t, s, From(todoTable)); n != 1 {
		t.Fatalf("count %d", n)
	}
	b := a
	b.Columns = append(append([]Column{}, a.Columns...), Column{Name: "note", Type: CText, Nullable: true})
	err = s.Ensure(ctx, []TableDesc{b}, false)
	if err == nil || !strings.Contains(err.Error(), "schema") {
		t.Fatalf("non-dev changed schema: %v", err)
	}
	if n := count(t, s, From(todoTable)); n != 1 {
		t.Fatalf("non-dev Ensure touched data: %d", n)
	}
	if err := s.Ensure(ctx, []TableDesc{b}, true); err != nil {
		t.Fatal(err)
	}
	if n := count(t, s, From(todoTable)); n != 0 {
		t.Fatalf("count %d after dev recreate", n)
	}
	if _, err := s.db.Exec(`INSERT INTO "Todo__Todo" ("id","title","done","created","updated","prio","note") VALUES ('x','t',0,'c','u','Low','n')`); err != nil {
		t.Fatalf("new column missing: %v", err)
	}
	s.Close()
	// Reopening sees the stored hash.
	s2, err := OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	if err := s2.Ensure(ctx, []TableDesc{b}, false); err != nil {
		t.Fatal(err)
	}
	if err := s2.Ensure(ctx, []TableDesc{a}, false); err == nil {
		t.Fatal("reverting schema in non-dev: want error")
	}
}

// The schema hash covers the sorted CREATE statements, so listing the tables in another order
// is no change; the storage format version is checked separately (I5).
func TestStore_EnsureOrderAndFormat(t *testing.T) {
	ctx := context.Background()
	path := t.TempDir() + "/app.db"
	s, err := OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	user := TableDesc{Name: "User.User", ID: "id", Columns: []Column{{Name: "id", Type: CText}}}
	todo := todoTable.Desc()
	if err := s.Ensure(ctx, []TableDesc{todo, user}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := InsertRow(ctx, s, nil, todoTable, insertVals("a", false, rt.Nothing[rt.Date](), "Low"), clock); err != nil {
		t.Fatal(err)
	}
	if err := s.Ensure(ctx, []TableDesc{user, todo}, false); err != nil {
		t.Fatalf("swapped tables: %v", err)
	}
	if n := count(t, s, From(todoTable)); n != 1 {
		t.Fatalf("swapped tables touched data: %d", n)
	}
	var format string
	if err := s.db.QueryRow(`SELECT "value" FROM "_alder_meta" WHERE "key" = 'format'`).Scan(&format); err != nil || format != Format {
		t.Fatalf("stored format %q %v, want %q", format, err, Format)
	}
	// A database made before formats were recorded is v0.
	if _, err := s.db.Exec(`DELETE FROM "_alder_meta" WHERE "key" = 'format'`); err != nil {
		t.Fatal(err)
	}
	err = s.Ensure(ctx, []TableDesc{todo, user}, false)
	if err == nil || !strings.Contains(err.Error(), "database format v0 is older than this runtime (v"+Format+")") {
		t.Fatalf("old format in prod: %v", err)
	}
	if n := count(t, s, From(todoTable)); n != 1 {
		t.Fatalf("prod Ensure touched data: %d", n)
	}
	if _, err := s.db.Exec(`INSERT INTO "_alder_meta" ("key", "value") VALUES ('format', '99')`); err != nil {
		t.Fatal(err)
	}
	if err := s.Ensure(ctx, []TableDesc{todo, user}, false); err == nil || !strings.Contains(err.Error(), "v99 is newer") {
		t.Fatalf("newer format in prod: %v", err)
	}
	if err := s.Ensure(ctx, []TableDesc{todo, user}, true); err != nil {
		t.Fatal(err)
	}
	if n := count(t, s, From(todoTable)); n != 0 {
		t.Fatalf("dev Ensure over another format kept %d rows", n)
	}
	if err := s.Ensure(ctx, []TableDesc{user, todo}, false); err != nil {
		t.Fatalf("after dev recreate: %v", err)
	}
}

func TestStore_EnsureRefs(t *testing.T) {
	ctx := context.Background()
	s, err := OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	user := TableDesc{Name: "User.User", ID: "id", Columns: []Column{{Name: "id", Type: CText}}}
	note := TableDesc{Name: "Note.Note", ID: "id", Columns: []Column{
		{Name: "id", Type: CText}, {Name: "owner", Type: CText, Ref: "User.User"}}}
	if err := s.Ensure(ctx, []TableDesc{note, user}, true); err != nil {
		t.Fatal(err)
	}
	failMsg := func(what string, err error, want string) {
		t.Helper()
		var ef *eff.ErrFail
		if !errors.As(err, &ef) || ef.Msg != want {
			t.Fatalf("%s: err = %v, want ErrFail %q", what, err, want)
		}
	}
	_, err = s.Insert(ctx, nil, "Note.Note", note.Columns, "id", "", "", []any{nil, "ghost"}, clock)
	failMsg("dangling insert", err, "Note.owner: no User with that id")
	uid, err := s.Insert(ctx, nil, "User.User", user.Columns, "id", "", "", []any{nil}, clock)
	if err != nil {
		t.Fatal(err)
	}
	nid, err := s.Insert(ctx, nil, "Note.Note", note.Columns, "id", "", "", []any{nil, uid}, clock)
	if err != nil {
		t.Fatal(err)
	}
	failMsg("dangling update", s.Update(ctx, nil, "Note.Note", note.Columns, "id", "", nid, []any{nid, "ghost"}, clock),
		"Note.owner: no User with that id")
	failMsg("delete referenced", s.Delete(ctx, nil, "User.User", "id", uid), "User "+uid+" is referenced by another row")
	// Inside a Task the failure rolls back the Task's earlier writes.
	_, err = eff.Run(newEnv(s), eff.KTask, eff.Bind(
		eff.Call("Db.insert User", true, func(e *eff.Env) (string, error) {
			return s.Insert(e.Ctx, TxOf(e), "User.User", user.Columns, "id", "", "", []any{nil}, clock)
		}),
		func(string) eff.Task[string] {
			return eff.Call("Db.insert Note", true, func(e *eff.Env) (string, error) {
				return s.Insert(e.Ctx, TxOf(e), "Note.Note", note.Columns, "id", "", "", []any{nil, "ghost"}, clock)
			})
		}))
	failMsg("dangling insert in a Task", err, "Note.owner: no User with that id")
	var users int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM "User__User"`).Scan(&users); err != nil || users != 1 {
		t.Fatalf("users after rolled-back Task: %d %v", users, err)
	}
	if _, err := s.db.Exec(`DELETE FROM "Note__Note"`); err != nil {
		t.Fatal(err)
	}
	// Dev recreate with refs in place drops cleanly.
	note.Columns = append(note.Columns, Column{Name: "x", Type: CInt, Nullable: true})
	if err := s.Ensure(ctx, []TableDesc{note, user}, true); err != nil {
		t.Fatal(err)
	}
}

func TestStore_QueryWithoutTx(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	id, err := InsertRow(ctx, s, nil, todoTable, insertVals("a", false, rt.Nothing[rt.Date](), "Low"), clock)
	if err != nil {
		t.Fatal(err)
	}
	env := newEnv(s)
	q := eff.Q(eff.Bind(eff.Call("Db.get Todo", false, func(e *eff.Env) (rt.Maybe[todo], error) {
		if e.Tx != nil || TxOf(e) != nil {
			t.Error("Query has a tx")
		}
		return GetRow(e.Ctx, e.Store.(Store), TxOf(e), todoTable, id)
	}), func(m rt.Maybe[todo]) eff.Task[[]todo] {
		return eff.Call("Db.all Todo", false, func(e *eff.Env) ([]todo, error) {
			return All(e.Ctx, e.Store.(Store), TxOf(e), From(todoTable))
		})
	}))
	rows, err := eff.RunQuery(env, q)
	if err != nil || len(rows) != 1 || rows[0].ID != id {
		t.Fatalf("rows %+v %v", rows, err)
	}
	if err := DeleteRow(ctx, s, nil, todoTable, id); err != nil {
		t.Fatal(err)
	}
	if n := count(t, s, From(todoTable)); n != 0 {
		t.Fatalf("count after delete %d", n)
	}
}

func TestEncodeListAndDecodeEnum(t *testing.T) {
	got := EncodeList([]bool{true, false}, func(b bool) any { return EncodeBool(b) })
	if len(got) != 2 || got[0] != int64(1) || got[1] != int64(0) {
		t.Fatalf("EncodeList = %#v", got)
	}
	if got := EncodeList[string](nil, func(s string) any { return s }); got == nil || len(got) != 0 {
		t.Fatalf("EncodeList(nil) = %#v, want empty non-nil", got)
	}
	names, values := []string{"Low", "Normal", "High"}, []int{10, 11, 12}
	if i, err := DecodeEnum("High", names, values); err != nil || i != 12 {
		t.Fatalf("DecodeEnum(High) = %d, %v", i, err)
	}
	if i, err := DecodeEnum([]byte("Low"), names, values); err != nil || i != 10 {
		t.Fatalf("DecodeEnum([]byte Low) = %d, %v", i, err)
	}
	for _, bad := range []any{"Urgent", int64(1), nil} {
		if _, err := DecodeEnum(bad, names, values); err == nil {
			t.Errorf("DecodeEnum(%#v): no error", bad)
		}
	}
}

// Stored times compare and order as the times do, also when they differ by less than a second
// (RFC3339Nano text would put "…12:00:00.5Z" before "…12:00:00Z").
func TestStore_SubSecondTimeOrder(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	whole := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	half := whole.Add(500 * time.Millisecond)
	for _, c := range []struct {
		title string
		at    time.Time
	}{{"half", half}, {"whole", whole}} {
		if _, err := InsertRow(ctx, s, nil, todoTable, insertVals(c.title, false, rt.Nothing[rt.Date](), "Low"), c.at); err != nil {
			t.Fatal(err)
		}
	}
	before, err := All(ctx, s, nil, Where(CLt("created", EncodeTime(half)), From(todoTable)))
	if err != nil || len(before) != 1 || before[0].Title != "whole" {
		t.Fatalf("created < 12:00:00.5: %+v, %v", before, err)
	}
	after, err := All(ctx, s, nil, Where(CGt("created", EncodeTime(whole)), From(todoTable)))
	if err != nil || len(after) != 1 || after[0].Title != "half" {
		t.Fatalf("created > 12:00:00: %+v, %v", after, err)
	}
	desc, err := All(ctx, s, nil, OrderBy(Desc("created"), From(todoTable)))
	if err != nil || len(desc) != 2 || desc[0].Title != "half" || !desc[0].Created.Equal(half) {
		t.Fatalf("order by created desc: %+v, %v", desc, err)
	}
}

func TestEncodeTimeFixedWidth(t *testing.T) {
	at := time.Date(2026, 10, 9, 21, 0, 0, 500000000, time.FixedZone("x", 9*3600))
	if got := EncodeTime(at); got != "2026-10-09T12:00:00.500000000Z" {
		t.Fatalf("EncodeTime = %q", got)
	}
	if got := EncodeTime(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)); got != "2026-01-02T03:04:05.000000000Z" {
		t.Fatalf("EncodeTime whole second = %q", got)
	}
	// both the fixed form and the shorter RFC3339Nano form of earlier databases decode
	for _, s := range []string{"2026-10-09T12:00:00.500000000Z", "2026-10-09T12:00:00.5Z", "2026-10-09T12:00:00Z"} {
		got, err := DecodeTime(s)
		if err != nil || got.Location() != time.UTC || got.Truncate(time.Second) != time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC) {
			t.Errorf("DecodeTime(%q) = %v, %v", s, got, err)
		}
	}
}

// I3: a capability implementation reaches the running Task's transaction through its ctx
// (TxFrom, StoreFrom), so its writes commit and roll back with the Task's — and on ":memory:",
// held on one connection, it does not deadlock against the Task's open transaction.
func TestStore_CapabilityWritesInTaskTx(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	// record is what a Go capability method looks like: it has only ctx and its arguments.
	record := func(ctx context.Context, title string) (string, error) {
		return InsertRow(ctx, StoreFrom(ctx), TxFrom(ctx), todoTable, insertVals(title, false, rt.Nothing[rt.Date](), "Low"), clock)
	}
	capCall := func(title string) eff.Task[string] {
		return eff.Call("Audit.record", true, func(e *eff.Env) (string, error) { return record(e.Ctx, title) })
	}
	env := newEnv(s)
	_, err := eff.Run(env, eff.KTask, eff.Bind(insertTask(todoTable, insertVals("mine", false, rt.Nothing[rt.Date](), "Low")),
		func(string) eff.Task[string] {
			return eff.Bind(capCall("cap"), func(string) eff.Task[string] { return eff.Fail[string]("boom") })
		}))
	if err == nil || err.Error() != "boom" {
		t.Fatalf("failing Task: %v", err)
	}
	if n := count(t, s, From(todoTable)); n != 0 {
		t.Fatalf("rolled-back Task left %d rows", n)
	}
	if _, err := eff.Run(env, eff.KTask, capCall("cap")); err != nil {
		t.Fatal(err)
	}
	if n := count(t, s, Where(CEq("title", "cap"), From(todoTable))); n != 1 {
		t.Fatalf("committed capability write: %d rows", n)
	}
	if TxFrom(ctx) != nil || StoreFrom(ctx) != nil {
		t.Fatal("TxFrom/StoreFrom of a context with no Env")
	}
}

// I2: a Task begins its transaction (and, on a file database, takes SQLite's write lock) only
// when it first touches the store. A Task waiting in a slow non-database step does not block
// other Tasks; Tasks that do write serialize correctly.
func TestStore_ConcurrentTasksOnFile(t *testing.T) {
	ctx := context.Background()
	s, err := OpenSQLite(t.TempDir() + "/app.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Ensure(ctx, []TableDesc{todoTable.Desc()}, true); err != nil {
		t.Fatal(err)
	}
	ins := func(title string) eff.Task[string] {
		return insertTask(todoTable, insertVals(title, false, rt.Nothing[rt.Date](), "Low"))
	}
	slow := eff.Bind(eff.Call("Slow.wait", true, func(*eff.Env) (struct{}, error) {
		time.Sleep(time.Second)
		return struct{}{}, nil
	}), func(struct{}) eff.Task[string] { return ins("slow") })
	slowDone := make(chan error, 1)
	go func() {
		_, err := eff.Run(newEnv(s), eff.KTask, slow)
		slowDone <- err
	}()
	time.Sleep(100 * time.Millisecond)
	start := time.Now()
	env := newEnv(s)
	env.Session = 1
	if _, err := eff.Run(env, eff.KTask, eff.Call("Session.put", true, func(e *eff.Env) (struct{}, error) { e.Session = 2; return struct{}{}, nil })); err != nil || env.Session != 2 {
		t.Fatalf("session-only Task: %v %v", err, env.Session)
	}
	if _, err := eff.Run(newEnv(s), eff.KTask, ins("fast")); err != nil {
		t.Fatalf("db Task beside a slow one: %v", err)
	}
	if d := time.Since(start); d > 500*time.Millisecond {
		t.Fatalf("Tasks beside a slow non-db step took %v (blocked by it)", d)
	}
	if err := <-slowDone; err != nil {
		t.Fatal(err)
	}

	// Two writers, each Task reading the count then inserting "n=<count>" (@unique): only if
	// the Tasks serialize does every insert see a fresh count.
	const per = 15
	countThenInsert := eff.Bind(eff.Call("Db.count Todo", false, func(e *eff.Env) (int64, error) {
		return CountRows(e.Ctx, e.Store.(Store), TxOf(e), From(todoTable))
	}), func(n int64) eff.Task[string] { return ins("n=" + strconv.FormatInt(n, 10)) })
	errs := make(chan error, 2*per)
	for range 2 {
		go func() {
			for range per {
				_, err := eff.Run(newEnv(s), eff.KTask, countThenInsert)
				errs <- err
			}
		}()
	}
	for range 2 * per {
		if err := <-errs; err != nil {
			t.Fatalf("serialized writer: %v", err)
		}
	}
	if n := count(t, s, From(todoTable)); n != 2+2*per {
		t.Fatalf("rows %d, want %d", n, 2+2*per)
	}
}

// A capability entry typed Query may be implemented in Go; inside a Query the store it reaches
// through StoreFrom refuses writes (they would otherwise run in autocommit), while reads pass
// through. The same implementation run from a Task writes.
func TestStoreFrom_QueryRefusesWrite(t *testing.T) {
	s := open(t)
	record := func(ctx context.Context, title string) (string, error) {
		return StoreFrom(ctx).Insert(ctx, TxFrom(ctx), todoTable.Name, todoTable.Columns, "id", "created", "updated",
			insertVals(title, false, rt.Nothing[rt.Date](), "Low"), clock)
	}
	capCall := func(title string) eff.Task[string] {
		// the capability entry is typed Query, so its Call is a read
		return eff.Call("Audit.peek", false, func(e *eff.Env) (string, error) { return record(e.Ctx, title) })
	}
	env := newEnv(s)
	_, err := eff.RunQuery(env, eff.Q(capCall("q")))
	if !errors.Is(err, eff.ErrInternal) || !strings.Contains(err.Error(), "write from a Query") {
		t.Fatalf("write from a Query: %v", err)
	}
	if n := count(t, s, From(todoTable)); n != 0 {
		t.Fatalf("Query write left %d rows", n)
	}
	// every write method refuses; reads pass through
	q := eff.Call("Audit.peek", false, func(e *eff.Env) (int64, error) {
		st := StoreFrom(e.Ctx)
		if err := st.Update(e.Ctx, nil, todoTable.Name, todoTable.Columns, "id", "updated", "x", nil, clock); !errors.Is(err, eff.ErrInternal) {
			t.Errorf("Update from a Query: %v", err)
		}
		if err := st.Delete(e.Ctx, nil, todoTable.Name, "id", "x"); !errors.Is(err, eff.ErrInternal) {
			t.Errorf("Delete from a Query: %v", err)
		}
		if _, err := st.Begin(e.Ctx); !errors.Is(err, eff.ErrInternal) {
			t.Errorf("Begin from a Query: %v", err)
		}
		return CountRows(e.Ctx, st, TxFrom(e.Ctx), From(todoTable))
	})
	if n, err := eff.RunQuery(env, eff.Q(q)); err != nil || n != 0 {
		t.Fatalf("read from a Query: %d %v", n, err)
	}
	if _, err := eff.Run(env, eff.KTask, capCall("t")); err != nil {
		t.Fatalf("write from a Task: %v", err)
	}
	if n := count(t, s, Where(CEq("title", "t"), From(todoTable))); n != 1 {
		t.Fatalf("Task write: %d rows", n)
	}
}
