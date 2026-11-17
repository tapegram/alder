package jobs_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"alder/rt"
	"alder/rt/db"
	"alder/rt/eff"
	"alder/rt/jobs"
	rtesting "alder/rt/testing"
)

var t0 = time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)

// noteTable is an app table the jobs of these tests write.
var noteTable = db.TableDesc{Name: "Note.Note", ID: "id", Columns: []db.Column{{Name: "id", Type: db.CText}, {Name: "text", Type: db.CText}}}

// open is an in-memory store with noteTable and the jobs table (Ensure creates it).
func open(t *testing.T) *db.SQLite {
	t.Helper()
	st, err := db.OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.Ensure(context.Background(), []db.TableDesc{noteTable}, true); err != nil {
		t.Fatal(err)
	}
	return st
}

// newEnv is an Env on st whose clock stands still at t0 (the returned Clock moves it).
func newEnv(st *db.SQLite) (*eff.Env, *rtesting.Clock) {
	c := rtesting.NewClock(t0)
	c.Set(t0)
	return &eff.Env{Ctx: context.Background(), Store: st, Now: c.Now}, c
}

func raw(t *testing.T, st *db.SQLite) db.Execer {
	t.Helper()
	q, err := db.On(st, nil)
	if err != nil {
		t.Fatal(err)
	}
	return q
}

func count(t *testing.T, st *db.SQLite, query string, args ...any) int {
	t.Helper()
	var n int
	if err := raw(t, st).QueryRowContext(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func enqueue(j rt.Job, key *string) eff.Task[string] {
	return eff.Call("Job.enqueue", true, func(e *eff.Env) (string, error) {
		return jobs.Enqueue(e, j, e.Now(), key)
	})
}

func runEnqueue(t *testing.T, env *eff.Env, j rt.Job, key *string) string {
	t.Helper()
	id, err := eff.Run(env, eff.KTask, enqueue(j, key))
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func setState(t *testing.T, st *db.SQLite, id, state string) {
	t.Helper()
	if _, err := raw(t, st).ExecContext(context.Background(), `UPDATE "_alder_jobs" SET "state" = ? WHERE "id" = ?`, state, id); err != nil {
		t.Fatal(err)
	}
}

func ptr[T any](v T) *T { return &v }

var welcome = rt.Job{Kind: "Signup.SendWelcome", Params: `{"user":"u1"}`}

func TestEnsureTable_Idempotent(t *testing.T) {
	st := open(t) // Ensure ran it once
	for range 2 {
		if err := jobs.EnsureTable(context.Background(), raw(t, st)); err != nil {
			t.Fatal(err)
		}
	}
	var v string
	if err := raw(t, st).QueryRowContext(context.Background(), `SELECT "value" FROM "_alder_meta" WHERE "key" = 'jobs_schema'`).Scan(&v); err != nil || v != "1" {
		t.Fatalf("jobs_schema: %q %v", v, err)
	}
}

func TestEnqueue_InCallersTransaction(t *testing.T) {
	st := open(t)
	env, _ := newEnv(st)
	failing := eff.Bind(enqueue(welcome, nil), func(string) eff.Task[struct{}] { return eff.Fail[struct{}]("no") })
	if _, err := eff.Run(env, eff.KTask, failing); err == nil {
		t.Fatal("want the Task's failure")
	}
	if n := count(t, st, `SELECT COUNT(*) FROM "_alder_jobs"`); n != 0 {
		t.Fatalf("rows after a failed Task: %d", n)
	}
	id := runEnqueue(t, env, welcome, nil)
	if len(id) != 8 {
		t.Fatalf("id %q: want 8 hex characters", id)
	}
	var kind, params, state string
	var runAt, created int64
	var attempts int
	if err := raw(t, st).QueryRowContext(context.Background(), `SELECT "kind", "params", "state", "run_at", "created", "attempts" FROM "_alder_jobs" WHERE "id" = ?`, id).
		Scan(&kind, &params, &state, &runAt, &created, &attempts); err != nil {
		t.Fatal(err)
	}
	if kind != welcome.Kind || params != welcome.Params || state != "queued" || runAt != t0.UnixMilli() || created != t0.UnixMilli() || attempts != 0 {
		t.Fatalf("row: %s %s %s %d %d %d", kind, params, state, runAt, created, attempts)
	}
}

func TestEnqueueOnce(t *testing.T) {
	st := open(t)
	env, _ := newEnv(st)
	a := runEnqueue(t, env, welcome, ptr("u1"))
	if b := runEnqueue(t, env, welcome, ptr("u1")); b != a {
		t.Fatalf("same key while queued: %s then %s", a, b)
	}
	if n := count(t, st, `SELECT COUNT(*) FROM "_alder_jobs"`); n != 1 {
		t.Fatalf("rows: %d", n)
	}
	setState(t, st, a, "running")
	if b := runEnqueue(t, env, welcome, ptr("u1")); b != a {
		t.Fatalf("same key while running: %s then %s", a, b)
	}
	setState(t, st, a, "done")
	b := runEnqueue(t, env, welcome, ptr("u1"))
	if b == a {
		t.Fatal("after done: the old id came back")
	}
	setState(t, st, b, "dead")
	c := runEnqueue(t, env, welcome, ptr("u1"))
	if c == b || c == a {
		t.Fatal("after dead: an old id came back")
	}
	digest := rt.Job{Kind: "Org.Digest", Params: `{}`}
	if d := runEnqueue(t, env, digest, ptr("u1")); d == c {
		t.Fatal("another kind shares the key")
	}
	if n := count(t, st, `SELECT COUNT(*) FROM "_alder_jobs" WHERE "key" = 'u1'`); n != 4 {
		t.Fatalf("rows with the key: %d", n)
	}
}

func TestEnqueue_ParamsTooLarge(t *testing.T) {
	st := open(t)
	env, _ := newEnv(st)
	big := rt.Job{Kind: "Signup.SendWelcome", Params: strings.Repeat("x", 65537)}
	_, err := eff.Run(env, eff.KTask, enqueue(big, nil))
	var f *eff.ErrFail
	if !errors.As(err, &f) || !strings.Contains(f.Msg, "64 KB") {
		t.Fatalf("65537 bytes: %v", err)
	}
	ok := rt.Job{Kind: "Signup.SendWelcome", Params: strings.Repeat("x", 65536)}
	runEnqueue(t, env, ok, nil)
}

func TestStatus(t *testing.T) {
	st := open(t)
	env, _ := newEnv(st)
	id := runEnqueue(t, env, welcome, nil)
	s, err := jobs.StatusOf(env, id)
	if err != nil || s == nil {
		t.Fatalf("status: %v %v", s, err)
	}
	if s.State != jobs.Queued || s.Attempts != 0 || s.Error != nil || s.RunAt == nil || !s.RunAt.Equal(t0) {
		t.Fatalf("status: %+v", s)
	}
	if s, err := jobs.StatusOf(env, "00000000"); err != nil || s != nil {
		t.Fatalf("unknown id: %v %v", s, err)
	}
}
