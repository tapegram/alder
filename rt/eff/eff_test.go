package eff

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

type fakeTx struct{ committed, rolledBack bool }

func (t *fakeTx) Commit() error   { t.committed = true; return nil }
func (t *fakeTx) Rollback() error { t.rolledBack = true; return nil }

type fakeStore struct{ txs []*fakeTx }

func (s *fakeStore) Begin(ctx context.Context) (Tx, error) {
	tx := &fakeTx{}
	s.txs = append(s.txs, tx)
	return tx, nil
}

// dbCall is a Call that uses the store, as generated Db.* steps do (db.TxOf): it asks for the
// Task's transaction first, which begins it on first use.
func dbCall[T any](name string, write bool, fn func(*Env) (T, error)) Task[T] {
	return Call(name, write, func(e *Env) (T, error) {
		if _, err := e.Transaction(); err != nil {
			var zero T
			return zero, err
		}
		return fn(e)
	})
}

func env(s *fakeStore) *Env {
	return &Env{Ctx: context.Background(), Store: s, Caps: map[string]any{}, Now: time.Now, Log: func(string, string) {}}
}

func TestRun_BindOrderAndTrace(t *testing.T) {
	s := &fakeStore{}
	e := env(s)
	var seen []int
	step := func(n int) Task[int] {
		return dbCall("step", true, func(*Env) (int, error) { seen = append(seen, n); return n, nil })
	}
	prog := Bind(step(1), func(a int) Task[int] { return Bind(step(2), func(b int) Task[int] { return Pure(a + b) }) })
	v, err := Run(e, KTask, prog)
	if err != nil || v != 3 || !reflect.DeepEqual(seen, []int{1, 2}) {
		t.Fatalf("v=%d err=%v seen=%v", v, err, seen)
	}
	if !reflect.DeepEqual(e.Trace, []string{"step", "step"}) {
		t.Fatalf("trace %v", e.Trace)
	}
	if len(s.txs) != 1 || !s.txs[0].committed || s.txs[0].rolledBack {
		t.Fatalf("tx %+v", s.txs)
	}
}
func TestRun_RollbackOnFail(t *testing.T) {
	s := &fakeStore{}
	prog := Bind(dbCall("ins", true, func(*Env) (int, error) { return 1, nil }), func(int) Task[int] { return Fail[int]("boom") })
	_, err := Run(env(s), KTask, prog)
	var ef *ErrFail
	if !errors.As(err, &ef) || ef.Msg != "boom" {
		t.Fatalf("err %v", err)
	}
	if !s.txs[0].rolledBack || s.txs[0].committed {
		t.Fatalf("tx %+v", s.txs[0])
	}
}
func TestRun_RollbackOnPanic(t *testing.T) {
	s := &fakeStore{}
	prog := dbCall("p", true, func(*Env) (int, error) { panic("x") })
	func() { defer func() { recover() }(); Run(env(s), KTask, prog) }()
	if !s.txs[0].rolledBack {
		t.Fatal("no rollback on panic")
	}
}
func TestRun_NotFoundShortCircuits(t *testing.T) {
	called := false
	prog := Bind(NotFound[int](), func(int) Task[int] { called = true; return Pure(1) })
	_, err := Run(env(&fakeStore{}), KTask, prog)
	if err != ErrNotFound || called {
		t.Fatal(err, called)
	}
}
func TestRun_QueryRefusesWrite(t *testing.T) {
	s := &fakeStore{}
	_, err := RunQuery(env(s), Q(Call("w", true, func(*Env) (int, error) { return 0, nil })))
	if !errors.Is(err, ErrInternal) || len(s.txs) != 0 {
		t.Fatalf("err=%v txs=%d", err, len(s.txs))
	}
}
func TestRun_AllEmpty(t *testing.T) {
	v, err := Run(env(&fakeStore{}), KTask, All[int](nil))
	if err != nil || len(v) != 0 {
		t.Fatal(v, err)
	}
}
func TestRun_AllStopsAtFirstFailure(t *testing.T) {
	n := 0
	es := []Task[int]{Call("a", false, func(*Env) (int, error) { n++; return 1, nil }), Fail[int]("f"), Call("c", false, func(*Env) (int, error) { n++; return 3, nil })}
	_, err := Run(env(&fakeStore{}), KTask, All(es))
	if err == nil || n != 1 {
		t.Fatal(err, n)
	}
}
func TestRun_MapAndCapAndSession(t *testing.T) {
	e := env(&fakeStore{})
	e.Session = 1
	e.Caps["X"] = func() int { return 41 }
	prog := Map(func(x int) int { return x + 1 }, Call("X.get", false, func(e *Env) (int, error) { e.Session = 2; return e.Caps["X"].(func() int)(), nil }))
	v, err := Run(e, KTask, prog)
	if err != nil || v != 42 || e.Session != 2 {
		t.Fatal(v, err, e.Session)
	}
}

// Tests beyond the brief.

func TestRun_DeepBindChains(t *testing.T) {
	const n = 100000
	inc := Call("inc", false, func(*Env) (int, error) { return 1, nil })
	// Left-nested: Bind(Bind(Bind(...))).
	left := Pure(0)
	for range n {
		left = Bind(left, func(x int) Task[int] { return Pure(x + 1) })
	}
	// Right-nested, built lazily: each continuation returns the next Bind.
	var right func(i, acc int) Task[int]
	right = func(i, acc int) Task[int] {
		if i == n {
			return Pure(acc)
		}
		return Bind(inc, func(x int) Task[int] { return right(i+1, acc+x) })
	}
	for name, prog := range map[string]Task[int]{"left": left, "right": right(0, 0)} {
		v, err := Run(env(&fakeStore{}), KTask, prog)
		if err != nil || v != n {
			t.Fatalf("%s: v=%d err=%v", name, v, err)
		}
	}
	// All over many effects, and a deep Map chain.
	es := make([]Task[int], n)
	for i := range es {
		es[i] = Pure(i)
	}
	vs, err := Run(env(&fakeStore{}), KTask, All(es))
	if err != nil || len(vs) != n || vs[n-1] != n-1 {
		t.Fatalf("all: len=%d err=%v", len(vs), err)
	}
	m := Pure(0)
	for range n {
		m = Map(func(x int) int { return x + 1 }, m)
	}
	if v, err := Run(env(&fakeStore{}), KTask, m); err != nil || v != n {
		t.Fatalf("map: v=%d err=%v", v, err)
	}
}

func TestMap_Composition(t *testing.T) {
	f := func(x int) int { return x * 2 }
	g := func(x int) string { return string(rune('a' + x)) }
	src := Call("src", false, func(*Env) (int, error) { return 3, nil })
	a, errA := Run(env(&fakeStore{}), KTask, Map(g, Map(f, src)))
	b, errB := Run(env(&fakeStore{}), KTask, Map(func(x int) string { return g(f(x)) }, src))
	if errA != nil || errB != nil || a != "g" || a != b {
		t.Fatalf("a=%q b=%q", a, b)
	}
	id, err := Run(env(&fakeStore{}), KTask, Map(func(x int) int { return x }, src))
	if err != nil || id != 3 {
		t.Fatal(id, err)
	}
}

func TestTQ_RoundTrip(t *testing.T) {
	q := Q(Pure(7))
	// Bind inside a Query: T, Bind, Q.
	q2 := Q(Bind(T(q), func(x int) Task[int] { return Pure(x + 1) }))
	if v, err := RunQuery(env(&fakeStore{}), q2); err != nil || v != 8 {
		t.Fatal(v, err)
	}
	s := &fakeStore{}
	if v, err := Run(env(s), KTask, T(Q(Pure(5)))); err != nil || v != 5 || len(s.txs) != 0 {
		t.Fatal(v, err, len(s.txs))
	}
}

func TestRun_ReadCallInTaskSeesTx(t *testing.T) {
	s := &fakeStore{}
	e := env(s)
	var saw Tx
	prog := dbCall("Db.get Todo", false, func(e *Env) (int, error) { saw = e.Tx; return 1, nil })
	if _, err := Run(e, KTask, prog); err != nil {
		t.Fatal(err)
	}
	if len(s.txs) != 1 || saw != s.txs[0] || !s.txs[0].committed {
		t.Fatalf("saw=%v txs=%+v", saw, s.txs)
	}
	if e.Tx != nil {
		t.Fatal("Tx not restored after Run")
	}
}

func TestRun_QueryReadsWithoutTx(t *testing.T) {
	s := &fakeStore{}
	e := env(s)
	var logged []string
	e.Log = func(level, msg string) { logged = append(logged, level+":"+msg) }
	v, err := RunQuery(e, Q(Call("Db.get Todo", false, func(e *Env) (int, error) {
		if e.Tx != nil {
			t.Error("Tx set in a Query")
		}
		return 4, nil
	})))
	if err != nil || v != 4 || len(s.txs) != 0 {
		t.Fatal(v, err, len(s.txs))
	}
	if !reflect.DeepEqual(logged, []string{"effect:Db.get Todo"}) || !reflect.DeepEqual(e.Trace, []string{"Db.get Todo"}) {
		t.Fatal(logged, e.Trace)
	}
}

func TestRun_RollbackOnCallErrorAndNotFound(t *testing.T) {
	boom := errors.New("disk")
	for _, prog := range []Task[int]{
		dbCall("x", true, func(*Env) (int, error) { return 0, boom }),
		dbCall("x", true, func(*Env) (int, error) { return 0, ErrNotFound }),
		Bind(dbCall("x", true, func(*Env) (int, error) { return 1, nil }), func(int) Task[int] { return NotFound[int]() }),
	} {
		s := &fakeStore{}
		_, err := Run(env(s), KTask, prog)
		if err == nil || !s.txs[0].rolledBack || s.txs[0].committed {
			t.Fatalf("err=%v tx=%+v", err, s.txs[0])
		}
	}
}

func TestRun_PanicPropagatesAndRestoresTx(t *testing.T) {
	s := &fakeStore{}
	e := env(s)
	var got any
	func() {
		defer func() { got = recover() }()
		Run(e, KTask, dbCall("p", true, func(*Env) (int, error) { panic("x") }))
	}()
	if got != "x" || !s.txs[0].rolledBack || s.txs[0].committed || e.Tx != nil {
		t.Fatalf("got=%v tx=%+v envTx=%v", got, s.txs[0], e.Tx)
	}
}

type badStore struct{}

func (badStore) Begin(context.Context) (Tx, error) { return nil, errors.New("no db") }

// A Begin error fails the Task at its first store access: that step does not run, nor does
// anything after it; steps before it (that did not touch the store) did run.
func TestRun_BeginErrorFailsAtFirstStoreAccess(t *testing.T) {
	var ran []string
	e := env(nil)
	e.Store = badStore{}
	step := func(name string, db bool) Task[int] {
		f := func(*Env) (int, error) { ran = append(ran, name); return 0, nil }
		if db {
			return dbCall(name, true, f)
		}
		return Call(name, false, f)
	}
	prog := Bind(step("cap", false), func(int) Task[int] {
		return Bind(step("db", true), func(int) Task[int] { return step("after", false) })
	})
	_, err := Run(e, KTask, prog)
	if err == nil || err.Error() != "no db" || !reflect.DeepEqual(ran, []string{"cap"}) {
		t.Fatal(err, ran)
	}
}

func TestRun_CancelledContextStopsBeforeCall(t *testing.T) {
	c, cancel := context.WithCancel(context.Background())
	cancel()
	s := &fakeStore{}
	e := env(s)
	e.Ctx = c
	ran := false
	_, err := Run(e, KTask, Call("x", false, func(*Env) (int, error) { ran = true; return 0, nil }))
	if !errors.Is(err, context.Canceled) || ran || len(s.txs) != 0 {
		t.Fatal(err, ran)
	}
}

func TestRun_TreeIsReusable(t *testing.T) {
	prog := All([]Task[int]{Pure(1), Pure(2)})
	a, _ := Run(env(&fakeStore{}), KTask, prog)
	b, _ := Run(env(&fakeStore{}), KTask, prog)
	a[0] = 99
	if !reflect.DeepEqual(b, []int{1, 2}) {
		t.Fatal(b)
	}
}

func TestRun_NilInterfaceValues(t *testing.T) {
	v, err := Run(env(&fakeStore{}), KTask, Bind(Pure[error](nil), func(e error) Task[error] { return Pure(e) }))
	if v != nil || err != nil {
		t.Fatal(v, err)
	}
}

func TestRun_ZeroTaskIsAnError(t *testing.T) {
	if _, err := Run(env(&fakeStore{}), KTask, Task[int]{}); !errors.Is(err, ErrInternal) {
		t.Fatalf("zero Task: err = %v, want ErrInternal", err)
	}
}

// Run fills a nil Ctx and Now, so a Call may pass e.Ctx on and call e.Now().
func TestRun_FillsCtxAndNow(t *testing.T) {
	e := &Env{}
	v, err := Run(e, KTask, Call("c", false, func(e *Env) (bool, error) {
		return e.Ctx != nil && !e.Now().IsZero(), nil
	}))
	if err != nil || !v || e.Ctx == nil || e.Now == nil {
		t.Fatalf("v=%v err=%v ctx=%v", v, err, e.Ctx)
	}
}

// I2: a Task begins its transaction lazily. A Task that never touches the store begins none
// (so it holds no database lock); one that does begins exactly one, however many steps use it.
func TestRun_LazyBegin(t *testing.T) {
	s := &fakeStore{}
	e := env(s)
	prog := Bind(Call("Cap.x", true, func(e *Env) (int, error) {
		if e.Tx != nil {
			t.Error("Tx before any store access")
		}
		return 1, nil
	}), func(int) Task[int] { return Pure(2) })
	if v, err := Run(e, KTask, prog); err != nil || v != 2 || len(s.txs) != 0 {
		t.Fatalf("no store access: v=%d err=%v txs=%d", v, err, len(s.txs))
	}
	two := Bind(dbCall("a", false, func(*Env) (int, error) { return 1, nil }), func(int) Task[int] {
		return dbCall("b", true, func(*Env) (int, error) { return 2, nil })
	})
	if _, err := Run(e, KTask, two); err != nil || len(s.txs) != 1 || !s.txs[0].committed {
		t.Fatalf("two store steps: err=%v txs=%+v", err, s.txs)
	}
	if e.Tx != nil {
		t.Fatal("Tx not restored")
	}
	// Transaction in a Query is nil and begins nothing.
	q := Q(Call("r", false, func(e *Env) (bool, error) { tx, err := e.Transaction(); return tx == nil && err == nil, nil }))
	if ok, err := RunQuery(e, q); err != nil || !ok || len(s.txs) != 1 {
		t.Fatalf("Query: ok=%v err=%v txs=%d", ok, err, len(s.txs))
	}
}

// I1: a Task that fails (Fail, NotFound, an error, a panic) restores the session it started
// with; a successful one — whatever its result — keeps its changes.
func TestRun_SessionRollback(t *testing.T) {
	put := func(v int) Task[struct{}] {
		return Call("Session.put", true, func(e *Env) (struct{}, error) { e.Session = v; return struct{}{}, nil })
	}
	then := func(t2 Task[int]) Task[int] { return Bind(put(2), func(struct{}) Task[int] { return t2 }) }
	for name, prog := range map[string]Task[int]{
		"fail":     then(Fail[int]("no")),
		"notFound": then(NotFound[int]()),
		"error":    then(Call("x", false, func(*Env) (int, error) { return 0, errors.New("disk") })),
	} {
		e := env(&fakeStore{})
		e.Session = 1
		if _, err := Run(e, KTask, prog); err == nil || e.Session != 1 {
			t.Errorf("%s: err=%v session=%v, want 1", name, err, e.Session)
		}
	}
	e := env(&fakeStore{})
	e.Session = 1
	func() {
		defer func() { recover() }()
		Run(e, KTask, then(Call("p", false, func(*Env) (int, error) { panic("x") })))
	}()
	if e.Session != 1 {
		t.Errorf("panic: session=%v, want 1", e.Session)
	}
	if _, err := Run(e, KTask, then(Pure(0))); err != nil || e.Session != 2 {
		t.Errorf("success: err=%v session=%v, want 2", err, e.Session)
	}
}

// I3: while Run runs, Env.Ctx carries the Env, so a capability reached with only ctx can find
// the running Task (and, through rt/db, its transaction); the Ctx is restored afterwards.
func TestRun_CtxCarriesEnv(t *testing.T) {
	s := &fakeStore{}
	e := env(s)
	before := e.Ctx
	var got *Env
	var tx Tx
	prog := Call("Cap.x", true, func(e *Env) (int, error) {
		got = EnvFrom(e.Ctx)
		tx, _ = EnvFrom(e.Ctx).Transaction()
		return 0, nil
	})
	if _, err := Run(e, KTask, prog); err != nil || got != e || tx == nil || len(s.txs) != 1 || tx != s.txs[0] || !s.txs[0].committed {
		t.Fatalf("err=%v got=%p env=%p tx=%v txs=%+v", err, got, e, tx, s.txs)
	}
	if e.Ctx != before || EnvFrom(e.Ctx) != nil {
		t.Fatal("Ctx not restored after Run")
	}
	if EnvFrom(context.Background()) != nil || EnvFrom(nil) != nil {
		t.Fatal("EnvFrom of a plain context")
	}
}
