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
	_, err = s.Insert(ctx, nil, "Note.Note", note.Columns, "id", "", "", []any{nil, "ghost"}, clock)
	if err == nil {
		t.Fatal("dangling @ref: want foreign key error")
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
