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

func env(s *fakeStore) *Env {
	return &Env{Ctx: context.Background(), Store: s, Caps: map[string]any{}, Now: time.Now, Log: func(string, string) {}}
}

func TestRun_BindOrderAndTrace(t *testing.T) {
	s := &fakeStore{}
	e := env(s)
	var seen []int
	step := func(n int) Task[int] {
		return Call("step", true, func(*Env) (int, error) { seen = append(seen, n); return n, nil })
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
	prog := Bind(Call("ins", true, func(*Env) (int, error) { return 1, nil }), func(int) Task[int] { return Fail[int]("boom") })
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
	prog := Call("p", true, func(*Env) (int, error) { panic("x") })
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
	if err == nil || len(s.txs) != 0 {
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
	if v, err := Run(env(s), KTask, T(Q(Pure(5)))); err != nil || v != 5 || len(s.txs) != 1 {
		t.Fatal(v, err, len(s.txs))
	}
}

func TestRun_ReadCallInTaskSeesTx(t *testing.T) {
	s := &fakeStore{}
	e := env(s)
	var saw Tx
	prog := Call("Db.get Todo", false, func(e *Env) (int, error) { saw = e.Tx; return 1, nil })
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
		Call("x", true, func(*Env) (int, error) { return 0, boom }),
		Call("x", true, func(*Env) (int, error) { return 0, ErrNotFound }),
		Bind(Pure(1), func(int) Task[int] { return NotFound[int]() }),
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
		Run(e, KTask, Call("p", true, func(*Env) (int, error) { panic("x") }))
	}()
	if got != "x" || !s.txs[0].rolledBack || s.txs[0].committed || e.Tx != nil {
		t.Fatalf("got=%v tx=%+v envTx=%v", got, s.txs[0], e.Tx)
	}
}

type badStore struct{}

func (badStore) Begin(context.Context) (Tx, error) { return nil, errors.New("no db") }

func TestRun_BeginErrorRunsNothing(t *testing.T) {
	ran := false
	e := env(nil)
	e.Store = badStore{}
	_, err := Run(e, KTask, Call("x", true, func(*Env) (int, error) { ran = true; return 0, nil }))
	if err == nil || ran {
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
	if !errors.Is(err, context.Canceled) || ran || !s.txs[0].rolledBack {
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
	if _, err := Run(env(&fakeStore{}), KTask, Task[int]{}); err == nil {
		t.Fatal("zero Task ran")
	}
}
