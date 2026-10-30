package eff

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// ErrInternal marks an error that is a bug in the compiler or the runtime, not in the app: a
// write effect reached from a Query, an invalid effect node. Test with errors.Is.
var ErrInternal = errors.New("internal error")

// Run interprets t eagerly, node by node, and returns its result:
// (value, nil) | (zero, *ErrFail) | (zero, ErrNotFound) | (zero, another
// error from the context, the store or a Call).
//
// kind KTask: the transaction is begun lazily. Run begins none itself; the
// first Call that asks for one (env.Transaction, which db.TxOf and
// db.TxFrom use) begins it on env.Store and records it in env.Tx, so a Task
// that never touches the database holds no database lock. After the result
// Run commits that transaction if one was begun; on any error or panic it
// rolls it back instead (a panic keeps unwinding after the rollback, with
// its original stack) and also restores env.Session to its value when Run
// started, so a failed Task leaves neither rows nor session changes behind.
// A Task that succeeds — including one producing Reject — keeps both. env.Tx
// is restored when Run returns. If env.Store is nil no transaction is ever
// begun.
//
// kind KQuery: no transaction (env.Tx is nil and env.Transaction gives nil);
// a Call with write=true is not run and Run returns an internal error.
// env.Kind is KQuery while it runs, so db.StoreFrom(ctx) gives a capability
// implementation a store that refuses writes.
//
// Every Call is logged at level "effect" with its name and appended to
// env.Trace before it runs.
//
// Run first fills a nil env.Ctx with context.Background() and a nil env.Now
// with time.Now, so Call functions (generated code passes e.Ctx to the store
// and to capabilities, and e.Now() to Db.insert/update) may use both
// unchecked. While Run runs, env.Ctx carries env (EnvFrom), so a capability
// implementation can reach the running effect's transaction
// (db.TxFrom(ctx)); env.Kind is kind. Both are restored when Run returns.
//
// Depth guarantee: Run is a loop over an explicit continuation stack kept
// on the heap, so Bind, Map and All chains of any length or nesting depth
// (left- or right-nested) run in constant Go stack. Run starts no
// goroutines.
func Run[T any](env *Env, kind Kind, t Task[T]) (T, error) {
	var zero T
	if env.Ctx == nil {
		env.Ctx = context.Background()
	}
	if env.Now == nil {
		env.Now = time.Now
	}
	prevCtx, prevTx, prevBegin, prevKind, prevActive := env.Ctx, env.Tx, env.beginTx, env.Kind, env.active
	if EnvFrom(env.Ctx) != env {
		env.Ctx = context.WithValue(env.Ctx, envKey{}, env)
	}
	env.Tx, env.beginTx, env.Kind, env.active = nil, nil, kind, true
	w := env.Within
	if w != nil && w.Tx == nil {
		w = nil
	}
	sp := "" // the savepoint taken on w.Tx, once taken
	switch {
	case w != nil && kind == KQuery:
		env.Tx = w.Tx // a Query reads through the outer transaction
	case w != nil:
		// the savepoint is taken now, not when a step first asks for the transaction: a
		// Call may run effects of its own within the same transaction (another Env's Run),
		// and a failure of this Task must undo those too
		if w.Savepoints != nil {
			name := SavepointName()
			if err := w.Savepoints.Savepoint(spCtx(env), w.Tx, name); err != nil {
				env.Ctx, env.Tx, env.beginTx, env.Kind, env.active = prevCtx, prevTx, prevBegin, prevKind, prevActive
				return zero, err
			}
			sp = name
		}
		env.Tx = w.Tx
	case kind == KTask && env.Store != nil:
		store := env.Store
		env.beginTx = func() (Tx, error) { return store.Begin(ctx(env)) }
	}
	// commit ends a Task's transaction (or savepoint) after success; undo after a failure.
	commit := func(tx Tx) error {
		if w == nil {
			return tx.Commit()
		}
		if sp == "" {
			return nil
		}
		err := w.Savepoints.Release(spCtx(env), w.Tx, sp)
		if err != nil {
			// the savepoint is still open with the Task's writes in it: undo them, so a
			// failed release leaves nothing behind in the outer transaction
			if rerr := w.Savepoints.RollbackTo(spCtx(env), w.Tx, sp); rerr == nil {
				if rerr = w.Savepoints.Release(spCtx(env), w.Tx, sp); rerr != nil {
					err = errors.Join(err, rerr)
				}
			} else {
				err = errors.Join(err, rerr)
			}
		}
		return err
	}
	undo := func(tx Tx) {
		if w == nil {
			rollback(env, tx)
			return
		}
		if sp == "" {
			return
		}
		err := w.Savepoints.RollbackTo(spCtx(env), w.Tx, sp)
		if err == nil {
			// ROLLBACK TO keeps the savepoint open; release it so it does not linger
			err = w.Savepoints.Release(spCtx(env), w.Tx, sp)
		}
		if err != nil && env.Log != nil {
			env.Log("warn", "rollback: "+err.Error())
		}
	}
	session, put := env.Session, env.SessionPut
	done := false
	defer func() {
		tx := env.Tx
		env.Ctx, env.Tx, env.beginTx, env.Kind, env.active = prevCtx, prevTx, prevBegin, prevKind, prevActive
		if !done && kind == KTask { // a panic is unwinding through us
			if tx != nil {
				undo(tx)
			}
			env.Session, env.SessionPut = session, put
		}
	}()

	v, err := interpret(env, kind, t.node)
	tx := env.Tx
	if err == nil && tx != nil && kind == KTask {
		err = commit(tx)
		tx = nil // a failed commit is not rolled back again
	}
	done = true
	if err != nil {
		if kind == KTask {
			if tx != nil {
				undo(tx)
			}
			env.Session, env.SessionPut = session, put
		}
		return zero, err
	}
	return as[T](v), nil
}

// RunQuery runs q as a Query: Run(env, KQuery, T(q)). With a Recorder it is counted
// (Recorder.Loads): in a request, a Query run is a page's `load`.
func RunQuery[T any](env *Env, q Query[T]) (T, error) {
	if env.Rec != nil {
		env.Rec.AddLoad()
	}
	return Run(env, KQuery, Task[T]{q.node})
}

// RunInner interprets t inside the Run that is running env — the same transaction, the same
// Trace, the same Session, the same kind — and returns its result or error without committing,
// rolling back or restoring anything: the enclosing Run decides. A Call's function uses it to
// run an effect it was given: Test.run (through rtesting.Inner, which brackets it with a
// savepoint so a failure undoes only t's writes), and a fake's entry (the generated fake
// capability's method finds env with EnvFrom(ctx)). t runs as env.Kind: inside a Query it is a
// Query (a write Call it reaches is the usual internal error), inside a Task a Task. Outside a
// running effect (no Run is running env) it is an internal error.
func RunInner[T any](env *Env, t Task[T]) (T, error) {
	var zero T
	if !env.active {
		return zero, fmt.Errorf("eff: %w: RunInner outside a running effect", ErrInternal)
	}
	v, err := interpret(env, env.Kind, t.node)
	if err != nil {
		return zero, err
	}
	return as[T](v), nil
}

// interpret is the eager loop. stack holds the continuations of the binds
// we are inside of, innermost last.
func interpret(env *Env, kind Kind, n node) (any, error) {
	var stack []func(any) node
	for {
		switch x := n.(type) {
		case pure:
			if len(stack) == 0 {
				return x.v, nil
			}
			f := stack[len(stack)-1]
			stack[len(stack)-1] = nil
			stack = stack[:len(stack)-1]
			n = f(x.v)
		case bind:
			stack = append(stack, x.f)
			n = x.e
		case call:
			if kind == KQuery && x.write {
				return nil, fmt.Errorf("eff: %w: write effect %q reached from a Query", ErrInternal, x.name)
			}
			if err := ctx(env).Err(); err != nil {
				return nil, err
			}
			if env.Log != nil {
				env.Log("effect", x.name)
			}
			env.Trace = append(env.Trace, x.name)
			var t0 time.Time
			if env.Rec != nil {
				t0 = time.Now()
			}
			v, err := x.fn(env)
			if env.Rec != nil {
				env.Rec.AddStep(StepRec{Name: x.name, Ms: Ms(time.Since(t0)), Cap: CapKind(env, x.name)})
			}
			if err != nil {
				return nil, err
			}
			n = pure{v}
		case mark:
			if env.Rec != nil {
				if x.on {
					env.Rec.BeginGroup()
				} else {
					env.Rec.EndGroup()
				}
			}
			n = pure{nil}
		case fail:
			return nil, &ErrFail{Msg: x.msg}
		case notFound:
			return nil, ErrNotFound
		default:
			return nil, fmt.Errorf("eff: %w: invalid effect node %T (zero Task or Query?)", ErrInternal, n)
		}
	}
}

func rollback(env *Env, tx Tx) {
	if err := tx.Rollback(); err != nil && env.Log != nil {
		env.Log("warn", "rollback: "+err.Error())
	}
}

// spCtx is the context of the savepoint statements Run issues under a Within: env's without
// its cancellation, so a Task whose context is cancelled (or past its deadline) still releases
// or rolls back to its savepoint instead of leaving it open in the outer transaction.
func spCtx(env *Env) context.Context { return context.WithoutCancel(ctx(env)) }

// ctx is env.Ctx, or Background when it is nil.
func ctx(env *Env) context.Context {
	if env.Ctx == nil {
		return context.Background()
	}
	return env.Ctx
}
