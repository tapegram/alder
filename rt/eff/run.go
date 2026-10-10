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
	prevCtx, prevTx, prevBegin, prevKind := env.Ctx, env.Tx, env.beginTx, env.Kind
	if EnvFrom(env.Ctx) != env {
		env.Ctx = context.WithValue(env.Ctx, envKey{}, env)
	}
	env.Tx, env.beginTx, env.Kind = nil, nil, kind
	if kind == KTask && env.Store != nil {
		store := env.Store
		env.beginTx = func() (Tx, error) { return store.Begin(ctx(env)) }
	}
	session := env.Session
	done := false
	defer func() {
		tx := env.Tx
		env.Ctx, env.Tx, env.beginTx, env.Kind = prevCtx, prevTx, prevBegin, prevKind
		if !done && kind == KTask { // a panic is unwinding through us
			if tx != nil {
				rollback(env, tx)
			}
			env.Session = session
		}
	}()

	v, err := interpret(env, kind, t.node)
	tx := env.Tx
	if err == nil && tx != nil && kind == KTask {
		err = tx.Commit()
		tx = nil // a failed commit is not rolled back again
	}
	done = true
	if err != nil {
		if kind == KTask {
			if tx != nil {
				rollback(env, tx)
			}
			env.Session = session
		}
		return zero, err
	}
	return as[T](v), nil
}

// RunQuery runs q as a Query: Run(env, KQuery, T(q)).
func RunQuery[T any](env *Env, q Query[T]) (T, error) {
	return Run(env, KQuery, Task[T]{q.node})
}

// RunInner interprets t inside the Run that is running env — the same transaction, the same
// Trace, the same Session — and returns its result or error without committing, rolling back
// or restoring anything: the enclosing Run decides. A Call's function uses it to run a Task it
// was given (Test.run, through rtesting.Inner, which brackets it with a savepoint so a failure
// undoes only t's writes). Outside a running Task (env.Kind is not KTask: Run sets it while
// it runs a Task and restores it after) it is an internal error.
func RunInner[T any](env *Env, t Task[T]) (T, error) {
	var zero T
	if env.Kind != KTask {
		return zero, fmt.Errorf("eff: %w: RunInner outside a running Task", ErrInternal)
	}
	v, err := interpret(env, KTask, t.node)
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
			v, err := x.fn(env)
			if err != nil {
				return nil, err
			}
			n = pure{v}
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

// ctx is env.Ctx, or Background when it is nil.
func ctx(env *Env) context.Context {
	if env.Ctx == nil {
		return context.Background()
	}
	return env.Ctx
}
