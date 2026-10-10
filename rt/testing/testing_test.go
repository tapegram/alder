package rtesting

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"alder/rt/db"
	"alder/rt/eff"
)

// row is what generated code would emit for `table T { id: TId @id, title: Text }` in module M.
type row struct{ ID, Title string }

var tb = &db.Table[row]{
	Name:    "M.T",
	Columns: []db.Column{{Name: "id", Type: db.CText}, {Name: "title", Type: db.CText}},
	ID:      "id",
	Encode:  func(r row) []any { return []any{r.ID, r.Title} },
	Decode: func(v []any) (row, error) {
		if len(v) != 2 {
			return row{}, errors.New("row: wrong column count")
		}
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

var tables = []db.TableDesc{tb.Desc()}

// insertStep is a write step against the test's store, as generated Db.insert code does; name
// is its trace entry.
func insertStep(name, title string) eff.Task[string] {
	return eff.Call(name, true, func(e *eff.Env) (string, error) {
		return db.InsertRow(e.Ctx, e.Store.(db.Store), db.TxOf(e), tb, []any{nil, title}, e.Now())
	})
}

func countStep() eff.Task[int64] {
	return eff.Call("Db.count T", false, func(e *eff.Env) (int64, error) {
		return db.CountRows(e.Ctx, e.Store.(db.Store), db.TxOf(e), db.From(tb))
	})
}

func titlesStep() eff.Task[[]string] {
	return eff.Call("Db.all T", false, func(e *eff.Env) ([]string, error) {
		rs, err := db.All(e.Ctx, e.Store.(db.Store), db.TxOf(e), db.OrderBy(db.Asc("title"), db.From(tb)))
		out := []string{}
		for _, r := range rs {
			out = append(out, r.Title)
		}
		return out, err
	})
}

// res is what generated Test.run code turns Inner's result into (Ok v / Err msg).
type res struct {
	msg   string
	ok    bool
	trace []string
}

// testRun is the Test.run step: it runs t under a savepoint with Inner.
func testRun[T any](t eff.Task[T]) eff.Task[res] {
	return eff.Call("Test.run", true, func(e *eff.Env) (res, error) {
		_, msg, ok, trace, err := Inner(e, t)
		return res{msg, ok, trace}, err
	})
}

func then[A, B any](a eff.Task[A], b eff.Task[B]) eff.Task[B] {
	return eff.Bind(a, func(A) eff.Task[B] { return b })
}

func one(c Case) []Result { return Run([]Case{c}, Options{Tables: tables}) }

func TestInner_RollbackToSavepoint(t *testing.T) {
	// outer insert; Test.run(inner insert then Fail) → Err, the inner insert undone, outer kept
	task := then(insertStep("Db.insert T", "outer"),
		eff.Bind(testRun(then(insertStep("Db.insert T", "inner"), eff.Fail[int]("nope"))), func(r res) eff.Task[bool] {
			return eff.Map(func(n int64) bool {
				return n == 1 && !r.ok && r.msg == "nope" && reflect.DeepEqual(r.trace, []string{"Db.insert T"})
			}, countStep())
		}))
	rs := one(Case{Module: "M", Name: "rollback", Task: func() eff.Task[bool] { return task }})
	if len(rs) != 1 || !rs[0].OK {
		t.Fatalf("%+v", rs)
	}
	if !reflect.DeepEqual(rs[0].Trace, []string{"Db.insert T", "Test.run", "Db.insert T", "Db.count T"}) {
		t.Fatalf("outer trace %v", rs[0].Trace)
	}
}

func TestInner_SuccessKeepsWritesAndValue(t *testing.T) {
	var got string
	var gotOK bool
	task := then(eff.Call("Test.run", true, func(e *eff.Env) (struct{}, error) {
		v, msg, ok, _, err := Inner(e, then(insertStep("Db.insert T", "kept"), eff.Pure("v")))
		got, gotOK = v+msg, ok
		return struct{}{}, err
	}), eff.Map(func(n int64) bool { return n == 1 }, countStep()))
	rs := one(Case{Module: "M", Name: "ok", Task: func() eff.Task[bool] { return task }})
	if !rs[0].OK || got != "v" || !gotOK {
		t.Fatalf("%+v got=%q ok=%v", rs, got, gotOK)
	}
}

func TestInner_NotFoundMessageAndSessionRestore(t *testing.T) {
	var r1, r2 res
	var after any
	setAndFail := then(eff.Call("Session.put", true, func(e *eff.Env) (struct{}, error) {
		e.Session = "changed"
		return struct{}{}, nil
	}), eff.Fail[int]("bad"))
	task := eff.Bind(testRun(eff.NotFound[int]()), func(a res) eff.Task[bool] {
		r1 = a
		return eff.Bind(testRun(setAndFail), func(b res) eff.Task[bool] {
			r2 = b
			return eff.Call("Session.get", false, func(e *eff.Env) (bool, error) {
				after = e.Session
				return true, nil
			})
		})
	})
	rs := Run([]Case{{Module: "M", Name: "nf", Task: func() eff.Task[bool] { return task }}}, Options{Tables: tables, Session: "orig"})
	if !rs[0].OK {
		t.Fatalf("%+v", rs)
	}
	if r1.ok || r1.msg != "not found" {
		t.Fatalf("NotFound: %+v", r1)
	}
	if r2.ok || r2.msg != "bad" || !reflect.DeepEqual(r2.trace, []string{"Session.put"}) {
		t.Fatalf("fail: %+v", r2)
	}
	if after != "orig" {
		t.Fatalf("session after a failed Test.run = %v, want orig", after)
	}
}

// recSP is the SQLite store with its savepoint names recorded.
type recSP struct {
	*db.SQLite
	names []string
}

func (r *recSP) Savepoint(ctx context.Context, tx db.Tx, name string) error {
	r.names = append(r.names, name)
	return r.SQLite.Savepoint(ctx, tx, name)
}

func TestInner_NestedAndSequential(t *testing.T) {
	s, err := db.OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	if err := s.Ensure(ctx, tables, true); err != nil {
		t.Fatal(err)
	}
	rec := &recSP{SQLite: s}
	env := &eff.Env{Ctx: ctx, Store: rec, Caps: map[string]any{}, Now: func() time.Time { return defaultNow }}

	var seq1, seq2, nestOuter, nestInner, failOuter res
	nested := then(insertStep("n1", "n1"),
		eff.Bind(testRun(then(insertStep("n2", "n2"), eff.Fail[string]("inner"))), func(r res) eff.Task[string] {
			nestInner = r
			return insertStep("n3", "n3")
		}))
	// a released inner savepoint is still undone when its enclosing one rolls back
	releasedThenFail := then(testRun(insertStep("r1", "r1")), eff.Fail[string]("outer"))
	task := eff.Bind(testRun(insertStep("s1", "s1")), func(r res) eff.Task[[]string] {
		seq1 = r
		return eff.Bind(testRun(insertStep("s2", "s2")), func(r res) eff.Task[[]string] {
			seq2 = r
			return eff.Bind(testRun(nested), func(r res) eff.Task[[]string] {
				nestOuter = r
				return eff.Bind(testRun(releasedThenFail), func(r res) eff.Task[[]string] {
					failOuter = r
					return titlesStep()
				})
			})
		})
	})
	titles, err := eff.Run(env, eff.KTask, task)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(titles, []string{"n1", "n3", "s1", "s2"}) {
		t.Fatalf("rows %v", titles)
	}
	if !seq1.ok || !reflect.DeepEqual(seq1.trace, []string{"s1"}) || !seq2.ok || !reflect.DeepEqual(seq2.trace, []string{"s2"}) {
		t.Fatalf("sequential windows: %+v %+v", seq1, seq2)
	}
	if !nestOuter.ok || !reflect.DeepEqual(nestOuter.trace, []string{"n1", "Test.run", "n2", "n3"}) {
		t.Fatalf("nested outer: %+v", nestOuter)
	}
	if nestInner.ok || nestInner.msg != "inner" || !reflect.DeepEqual(nestInner.trace, []string{"n2"}) {
		t.Fatalf("nested inner: %+v", nestInner)
	}
	if failOuter.ok || failOuter.msg != "outer" || !reflect.DeepEqual(failOuter.trace, []string{"Test.run", "r1"}) {
		t.Fatalf("released then failed: %+v", failOuter)
	}
	if len(rec.names) != 6 {
		t.Fatalf("savepoints %v (want 6)", rec.names)
	}
	seen := map[string]bool{}
	for _, n := range rec.names {
		if seen[n] || !strings.HasPrefix(n, "alder_t") {
			t.Fatalf("savepoint names %v must be distinct alder_t<n>", rec.names)
		}
		seen[n] = true
	}
}

func TestInner_OtherErrorPropagates(t *testing.T) {
	var innerErr error
	task := then(eff.Call("Test.run", true, func(e *eff.Env) (struct{}, error) {
		_, _, _, _, err := Inner(e, then(insertStep("Db.insert T", "x"), eff.Call("Disk", true, func(*eff.Env) (int, error) {
			return 0, errors.New("disk")
		})))
		innerErr = err
		return struct{}{}, err
	}), eff.Pure(true))
	rs := one(Case{Module: "M", Name: "disk", Task: func() eff.Task[bool] { return task }})
	if innerErr == nil || innerErr.Error() != "disk" {
		t.Fatalf("Inner err=%v", innerErr)
	}
	if rs[0].OK || rs[0].Error != "disk" {
		t.Fatalf("%+v", rs[0])
	}
	if !reflect.DeepEqual(rs[0].Trace, []string{"Test.run", "Db.insert T", "Disk"}) {
		t.Fatalf("trace %v", rs[0].Trace)
	}
}

func TestInner_OutsideTaskIsError(t *testing.T) {
	env := &eff.Env{Ctx: context.Background()}
	if _, _, _, _, err := Inner(env, eff.Pure(1)); !errors.Is(err, eff.ErrInternal) {
		t.Fatalf("err=%v", err)
	}
}

func TestTrace_DropsItsOwnStep(t *testing.T) {
	var got []string
	task := then(insertStep("Db.insert T", "a"), eff.Call("Test.trace", false, func(e *eff.Env) (bool, error) {
		got = Trace(e)
		return true, nil
	}))
	if rs := one(Case{Module: "M", Name: "trace", Task: func() eff.Task[bool] { return task }}); !rs[0].OK {
		t.Fatalf("%+v", rs)
	}
	if !reflect.DeepEqual(got, []string{"Db.insert T"}) {
		t.Fatalf("Trace %v", got)
	}
	if tr := Trace(&eff.Env{}); len(tr) != 0 {
		t.Fatalf("empty trace %v", tr)
	}
}

func TestRun_PureAndSides(t *testing.T) {
	rs := Run([]Case{
		{Module: "M", Name: "neq", Pure: func() bool { return Eq(int64(1), int64(2)) }},
		{Module: "M", Name: "plain", Pure: func() bool { return true }},
		{Module: "M", Name: "eq", Pure: func() bool { return Eq("a", "a") }},
	}, Options{})
	if len(rs) != 3 {
		t.Fatalf("%+v", rs)
	}
	if rs[0].OK || rs[0].Left != "1" || rs[0].Right != "2" || rs[0].Module != "M" || rs[0].Name != "neq" {
		t.Fatalf("neq: %+v", rs[0])
	}
	if !rs[1].OK || rs[1].Left != "" || rs[1].Right != "" || rs[1].Trace != nil {
		t.Fatalf("plain (no sides from the previous case): %+v", rs[1])
	}
	if !rs[2].OK || rs[2].Left != `"a"` || rs[2].Right != `"a"` {
		t.Fatalf("eq: %+v", rs[2])
	}
}

func TestRun_PanicIsFailure(t *testing.T) {
	rs := Run([]Case{
		{Module: "M", Name: "boom", Pure: func() bool { panic("boom") }},
		{Module: "M", Name: "taskboom", Task: func() eff.Task[bool] {
			return then(insertStep("Db.insert T", "x"), eff.Call("Boom", true, func(*eff.Env) (bool, error) { panic("kaboom") }))
		}},
		{Module: "M", Name: "next", Pure: func() bool { return true }},
	}, Options{Tables: tables})
	if len(rs) != 3 || rs[0].OK || !strings.Contains(rs[0].Error, "boom") {
		t.Fatalf("pure panic: %+v", rs)
	}
	if rs[1].OK || !strings.Contains(rs[1].Error, "kaboom") || !reflect.DeepEqual(rs[1].Trace, []string{"Db.insert T", "Boom"}) {
		t.Fatalf("task panic: %+v", rs[1])
	}
	if !rs[2].OK {
		t.Fatalf("the next case still runs: %+v", rs[2])
	}
}

func TestRun_UnwiredCapability(t *testing.T) {
	rs := one(Case{Module: "M", Name: "cap", Task: func() eff.Task[bool] {
		return eff.Call("Auth.Login.Password.check", true, func(*eff.Env) (bool, error) {
			panic("alder: capability \"Auth.Login.Password\" is not wired")
		})
	}})
	if rs[0].OK || rs[0].Error != "capability Auth.Login.Password is not wired in tests" {
		t.Fatalf("%+v", rs[0])
	}
}

func TestRun_FreshStorePerCase(t *testing.T) {
	rs := Run([]Case{
		{Module: "M", Name: "insert", Task: func() eff.Task[bool] {
			return then(insertStep("Db.insert T", "x"), eff.Map(func(n int64) bool { return n == 1 }, countStep()))
		}},
		{Module: "M", Name: "count", Task: func() eff.Task[bool] {
			return eff.Map(func(n int64) bool { return n == 0 }, countStep())
		}},
	}, Options{Tables: tables})
	if !rs[0].OK || !rs[1].OK {
		t.Fatalf("%+v", rs)
	}
}

func TestRun_FixedNowAndCaps(t *testing.T) {
	var now time.Time
	var caps map[string]any
	rs := one(Case{Module: "M", Name: "now", Task: func() eff.Task[bool] {
		return eff.Call("Clock.now", false, func(e *eff.Env) (bool, error) {
			now, caps = e.Now(), e.Caps
			return true, nil
		})
	}})
	if !rs[0].OK || !now.Equal(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)) || caps == nil {
		t.Fatalf("%+v now=%v caps=%v", rs, now, caps)
	}
	if rs[0].Trace == nil || rs[0].Ms < 0 {
		t.Fatalf("%+v", rs[0])
	}
}

func TestRun_Filter(t *testing.T) {
	cs := []Case{
		{Module: "M", Name: "a", Pure: func() bool { return true }},
		{Module: "N", Name: "a", Pure: func() bool { return true }},
	}
	rs := Run(cs, Options{Run: regexp.MustCompile(`^N "a"$`)})
	if len(rs) != 1 || rs[0].Module != "N" {
		t.Fatalf("%+v", rs)
	}
}

func mainCases() []Case {
	return []Case{
		{Module: "M", Name: "a", Pure: func() bool { return Eq(int64(1), int64(1)) }},
		{Module: "M", Name: "b", Pure: func() bool { return Eq(int64(1), int64(2)) }},
	}
}

func runMain(t *testing.T, cs []Case, args ...string) (string, string, int) {
	t.Helper()
	var out, errb bytes.Buffer
	code := Main(cs, tables, nil, nil, args, &out, &errb)
	return out.String(), errb.String(), code
}

func TestMain_TextAndJSON(t *testing.T) {
	t.Setenv("ALDER_TEST_RUN", "")
	t.Setenv("ALDER_TEST_JSON", "")

	out, _, code := runMain(t, mainCases())
	if want := "ok   M \"a\"\nFAIL M \"b\"\n  left:  1\n  right: 2\n1 passed, 1 failed\n"; out != want || code != 1 {
		t.Fatalf("text: code %d\n%q\nwant\n%q", code, out, want)
	}

	out, errs, code := runMain(t, mainCases(), "--json")
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if code != 1 || len(lines) != 2 {
		t.Fatalf("json: code %d out %q", code, out)
	}
	var a, b map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &a); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(lines[1]), &b); err != nil {
		t.Fatal(err)
	}
	if a["module"] != "M" || a["name"] != "a" || a["ok"] != true || !reflect.DeepEqual(a["trace"], []any{}) {
		t.Fatalf("json a: %v", a)
	}
	if _, has := a["error"]; has {
		t.Fatalf("empty error must be omitted: %v", a)
	}
	if _, has := a["ms"]; !has {
		t.Fatalf("ms missing: %v", a)
	}
	if b["ok"] != false || b["left"] != "1" || b["right"] != "2" {
		t.Fatalf("json b: %v", b)
	}
	if !strings.Contains(errs, "1 passed, 1 failed") {
		t.Fatalf("json summary on stderr: %q", errs)
	}

	out, _, code = runMain(t, mainCases(), "-run", `M "a"`)
	if out != "ok   M \"a\"\n1 passed, 0 failed\n" || code != 0 {
		t.Fatalf("-run: code %d %q", code, out)
	}

	if _, errs, code = runMain(t, mainCases(), "-run", "("); code != 2 || errs == "" {
		t.Fatalf("bad -run: code %d err %q", code, errs)
	}

	// the environment, and flags winning over it
	t.Setenv("ALDER_TEST_RUN", `"b"`)
	t.Setenv("ALDER_TEST_JSON", "1")
	out, _, code = runMain(t, mainCases())
	if code != 1 || strings.Count(out, "\n") != 1 || !strings.Contains(out, `"name":"b"`) {
		t.Fatalf("env: code %d %q", code, out)
	}
	out, _, code = runMain(t, mainCases(), "-run", `"a"`, "--json=false")
	if out != "ok   M \"a\"\n1 passed, 0 failed\n" || code != 0 {
		t.Fatalf("flags win: code %d %q", code, out)
	}
}

func TestMain_TaskFailurePrintsErrorAndTrace(t *testing.T) {
	t.Setenv("ALDER_TEST_RUN", "")
	t.Setenv("ALDER_TEST_JSON", "")
	cs := []Case{{Module: "Todos.Index", Name: "Toggle flips done", Task: func() eff.Task[bool] {
		return then(insertStep("Db.insert T", "x"), eff.Fail[bool]("no such todo"))
	}}}
	out, _, code := runMain(t, cs)
	want := "FAIL Todos.Index \"Toggle flips done\"\n  error: no such todo\n  trace: [\"Db.insert T\"]\n0 passed, 1 failed\n"
	if out != want || code != 1 {
		t.Fatalf("code %d\n%q\nwant\n%q", code, out, want)
	}
}

func TestMain_NoCases(t *testing.T) {
	t.Setenv("ALDER_TEST_RUN", "")
	t.Setenv("ALDER_TEST_JSON", "")
	out, _, code := runMain(t, nil)
	if out != "0 passed, 0 failed\n" || code != 0 {
		t.Fatalf("code %d %q", code, out)
	}
	out, errs, code := runMain(t, nil, "--json")
	if out != "" || code != 0 || !strings.Contains(errs, "0 passed, 0 failed") {
		t.Fatalf("json: code %d out %q err %q", code, out, errs)
	}
}

func TestRun_MsIsWallTime(t *testing.T) {
	t.Setenv("ALDER_TEST_RUN", "")
	t.Setenv("ALDER_TEST_JSON", "")
	slow := []Case{{Module: "M", Name: "slow", Pure: func() bool { time.Sleep(2 * time.Millisecond); return true }}}
	if rs := Run(slow, Options{}); rs[0].Ms <= 0 {
		t.Fatalf("Ms = %v, want > 0", rs[0].Ms)
	}
	out, _, _ := runMain(t, slow, "--json")
	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatal(err)
	}
	if ms, _ := m["ms"].(float64); ms <= 0 {
		t.Fatalf("json ms = %v, want > 0", m["ms"])
	}
}

func TestMain_TextStaysLineStructured(t *testing.T) {
	t.Setenv("ALDER_TEST_RUN", "")
	t.Setenv("ALDER_TEST_JSON", "")
	cs := []Case{
		{Module: "M", Name: "two\nlines", Pure: func() bool { return true }},
		{Module: "M", Name: `say "hi"`, Pure: func() bool { return true }},
		{Module: "M", Name: "multi", Task: func() eff.Task[bool] { return eff.Fail[bool]("first\nsecond") }},
	}
	out, _, code := runMain(t, cs)
	want := "ok   M \"two\\nlines\"\n" +
		"ok   M \"say \\\"hi\\\"\"\n" +
		"FAIL M \"multi\"\n  error: first\n    second\n  trace: []\n" +
		"2 passed, 1 failed\n"
	if out != want || code != 1 {
		t.Fatalf("code %d\n%q\nwant\n%q", code, out, want)
	}
}

// failWriter fails every write.
type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("closed pipe") }

func TestMain_JSONWriteErrorIsUsageExit(t *testing.T) {
	t.Setenv("ALDER_TEST_RUN", "")
	t.Setenv("ALDER_TEST_JSON", "")
	var errb bytes.Buffer
	code := Main(mainCases(), tables, nil, nil, []string{"--json"}, failWriter{}, &errb)
	if code != 2 || !strings.Contains(errb.String(), "closed pipe") {
		t.Fatalf("code %d err %q", code, errb.String())
	}
}

// A capability's error inside Test.run is an Err the test can observe: the savepoint is rolled
// back, the session restored, and the message names the capability.
func TestInner_CapabilityErrorIsErr(t *testing.T) {
	var r res
	var after any
	capStep := eff.Call("Password.verify", false, func(e *eff.Env) (bool, error) {
		e.Session = "changed"
		return false, eff.CapError("Password.verify", errors.New("hash store unreachable"))
	})
	task := then(insertStep("Db.insert T", "outer"),
		eff.Bind(testRun(then(insertStep("Db.insert T", "inner"), capStep)), func(a res) eff.Task[bool] {
			r = a
			return eff.Bind(eff.Call("Session.get", false, func(e *eff.Env) (any, error) { return e.Session, nil }), func(s any) eff.Task[bool] {
				after = s
				return eff.Map(func(n int64) bool { return n == 1 }, countStep())
			})
		}))
	rs := Run([]Case{{Module: "M", Name: "cap", Task: func() eff.Task[bool] { return task }}}, Options{Tables: tables, Session: "start"})
	if !rs[0].OK || r.ok || r.msg != "Password.verify: hash store unreachable" || after != "start" {
		t.Fatalf("%+v r=%+v session=%v", rs[0], r, after)
	}
}

// When a failing test's two sides print the same, the report says they are different values.
func TestMain_SamePrintedSidesNote(t *testing.T) {
	t.Setenv("ALDER_TEST_RUN", "")
	t.Setenv("ALDER_TEST_JSON", "")
	nan := math.NaN()
	cs := []Case{{Module: "M", Name: "nan", Pure: func() bool { return Eq(nan, nan) }},
		{Module: "M", Name: "other", Pure: func() bool { _ = Eq(int64(1), int64(1)); return false }}}
	out, _, code := runMain(t, cs)
	want := "FAIL M \"nan\"\n  left:  NaN\n  right: NaN\n  note:  the sides print the same but are different values\n" +
		"FAIL M \"other\"\n  left:  1\n  right: 1\n0 passed, 2 failed\n"
	if out != want || code != 1 {
		t.Fatalf("code %d\n%q\nwant\n%q", code, out, want)
	}
}
