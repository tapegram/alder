package eff

import (
	"context"
	"fmt"
)

// Run interprets t eagerly, node by node, and returns its result:
// (value, nil) | (zero, *ErrFail) | (zero, ErrNotFound) | (zero, another
// error from the context, the store or a Call).
//
// kind KTask: Begin a transaction on env.Store before the first step and
// set env.Tx; Commit after the result; Rollback instead on any error or
// panic (a panic keeps unwinding after the rollback, with its original
// stack). env.Tx is restored when Run returns. If env.Store is nil the
// Task runs with no transaction.
//
// kind KQuery: no transaction; a Call with write=true is not run and Run
// returns an internal error.
//
// Every Call is logged at level "effect" with its name and appended to
// env.Trace before it runs.
//
// Depth guarantee: Run is a loop over an explicit continuation stack kept
// on the heap, so Bind, Map and All chains of any length or nesting depth
// (left- or right-nested) run in constant Go stack. Run starts no
// goroutines.
func Run[T any](env *Env, kind Kind, t Task[T]) (T, error) {
	var zero T
	if kind != KTask || env.Store == nil {
		v, err := interpret(env, kind, t.node)
		if err != nil {
			return zero, err
		}
		return as[T](v), nil
	}

	tx, err := env.Store.Begin(ctx(env))
	if err != nil {
		return zero, err
	}
	prev := env.Tx
	env.Tx = tx
	done := false
	defer func() {
		env.Tx = prev
		if !done { // a panic is unwinding through us
			rollback(env, tx)
		}
	}()

	v, err := interpret(env, kind, t.node)
	if err != nil {
		done = true
		rollback(env, tx)
		return zero, err
	}
	done = true
	if err := tx.Commit(); err != nil {
		return zero, err
	}
	return as[T](v), nil
}

// RunQuery runs q as a Query: Run(env, KQuery, T(q)).
func RunQuery[T any](env *Env, q Query[T]) (T, error) {
	return Run(env, KQuery, Task[T]{q.node})
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
				return nil, fmt.Errorf("eff: internal error: write effect %q reached from a Query", x.name)
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
			return nil, fmt.Errorf("eff: internal error: invalid effect node %T (zero Task or Query?)", n)
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
