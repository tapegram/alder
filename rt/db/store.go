package db

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"time"

	"alder/rt"
	"alder/rt/eff"
)

// Store is what the Db.* effect steps call. table is always the Alder
// (module-qualified) table name; the store maps it to SQL. tx may be nil —
// inside a Query Env.Tx is nil — and then the store reads (or writes, outside
// eff.Run) without a transaction.
type Store interface {
	eff.Store
	Get(ctx context.Context, tx Tx, table string, id string) (row []any, found bool, err error)
	Query(ctx context.Context, tx Tx, table string, cols []Column, sel SQL) (rows [][]any, err error)
	Count(ctx context.Context, tx Tx, table string, sel SQL) (int64, error)
	// Insert stores vals (Columns order, nil at the id/created/updated
	// positions), filling the id with NewID and created/updated with now.
	Insert(ctx context.Context, tx Tx, table string, cols []Column, idCol, createdCol, updatedCol string, vals []any, now time.Time) (id string, err error)
	// Update rewrites every column of row id except the id and the @created
	// column from vals (Columns order), setting updatedCol (if any) to now.
	Update(ctx context.Context, tx Tx, table string, cols []Column, idCol, updatedCol, id string, vals []any, now time.Time) error
	Delete(ctx context.Context, tx Tx, table string, idCol, id string) error
}

// Tx is a Store's transaction.
type Tx interface{ eff.Tx }

// Savepointer is a Store whose transactions support savepoints (SQLite does).
//
// A savepoint is a named mark inside an open transaction. Savepoint sets the
// mark; RollbackTo undoes every write made since the mark (the writes made
// before it, earlier in the same transaction, stay) and keeps the mark;
// Release forgets the mark and keeps its writes as part of the enclosing
// transaction (or the enclosing savepoint), so they are committed or rolled
// back with it. SQLite nests savepoints: one opened while another is open
// belongs to it, and rolling back the outer one undoes the inner one too.
// A Test.run uses one so a failed task undoes only its own writes while the
// test's earlier writes stay. Every method needs a transaction: given a nil
// tx it returns an error ("db: savepoint outside a transaction").
type Savepointer interface {
	Savepoint(ctx context.Context, tx Tx, name string) error
	Release(ctx context.Context, tx Tx, name string) error
	RollbackTo(ctx context.Context, tx Tx, name string) error
}

// TxOf is the running Task's transaction as a Tx, begun on first use
// (eff.Env.Transaction), or nil in a Query. If beginning it fails, TxOf
// returns a Tx that carries the error: every operation of this package (and
// of the SQLite store) given it returns that error, which fails the Task.
func TxOf(e *eff.Env) Tx {
	tx, err := e.Transaction()
	if err != nil {
		return beginFailed{err}
	}
	if tx == nil {
		return nil
	}
	return tx.(Tx)
}

// TxFrom is TxOf of the Env running with ctx (eff.EnvFrom), or nil when
// ctx carries none. A capability implementation passes it, with StoreFrom,
// to the store to write inside the running Task's transaction:
//
//	func (a audit) Record(ctx context.Context, msg string) (struct{}, error) {
//		st := db.StoreFrom(ctx)
//		_, err := st.Insert(ctx, db.TxFrom(ctx), "Audit.Entry", cols, "id", "", "", []any{nil, msg}, time.Now())
//		return struct{}{}, err
//	}
func TxFrom(ctx context.Context) Tx {
	e := eff.EnvFrom(ctx)
	if e == nil {
		return nil
	}
	return TxOf(e)
}

// StoreFrom is the Store of the Env running with ctx (eff.EnvFrom), or nil.
// While that Env runs a Query (Env.Kind is eff.KQuery) it is a read-only
// view of the Store: a capability entry typed Query may be implemented in
// Go, and a write from it would run outside any transaction, so Insert,
// Update, Delete and Begin return an eff.ErrInternal error ("write from a
// Query") and write nothing; Get, Query and Count pass through.
func StoreFrom(ctx context.Context) Store {
	e := eff.EnvFrom(ctx)
	if e == nil {
		return nil
	}
	st, _ := e.Store.(Store)
	if st != nil && e.Kind == eff.KQuery {
		return readOnly{st}
	}
	return st
}

// readOnly is the Store StoreFrom gives during a Query: reads pass through
// to the embedded Store, every write method fails.
type readOnly struct{ Store }

var errQueryWrite = fmt.Errorf("db: %w: write from a Query", eff.ErrInternal)

func (readOnly) Begin(context.Context) (eff.Tx, error) { return nil, errQueryWrite }

func (readOnly) Insert(context.Context, Tx, string, []Column, string, string, string, []any, time.Time) (string, error) {
	return "", errQueryWrite
}

func (readOnly) Update(context.Context, Tx, string, []Column, string, string, string, []any, time.Time) error {
	return errQueryWrite
}

func (readOnly) Delete(context.Context, Tx, string, string, string) error { return errQueryWrite }

// beginFailed is the Tx TxOf returns when beginning the transaction failed.
type beginFailed struct{ err error }

func (b beginFailed) Commit() error   { return b.err }
func (b beginFailed) Rollback() error { return b.err }

// txErr is the begin error tx carries, or nil.
func txErr(tx Tx) error {
	if b, ok := tx.(beginFailed); ok {
		return b.err
	}
	return nil
}

// NewID is a fresh 20-character URL-safe id from crypto/rand.
func NewID() string {
	var b [15]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("db: crypto/rand: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(b[:])
}

// GetRow is Db.get.
func GetRow[T any](ctx context.Context, st Store, tx Tx, t *Table[T], id string) (rt.Maybe[T], error) {
	if err := txErr(tx); err != nil {
		return rt.Nothing[T](), err
	}
	row, found, err := st.Get(ctx, tx, t.Name, id)
	if err != nil || !found {
		return rt.Nothing[T](), err
	}
	v, err := t.Decode(row)
	if err != nil {
		return rt.Nothing[T](), fmt.Errorf("db: decode %s: %w", t.Name, err)
	}
	return rt.Just(v), nil
}

// All is Db.all. No rows is an empty, non-nil slice.
func All[T any](ctx context.Context, st Store, tx Tx, s Select[T]) ([]T, error) {
	if err := txErr(tx); err != nil {
		return nil, err
	}
	q := s.SQL()
	if q.Err != nil {
		return nil, q.Err
	}
	rows, err := st.Query(ctx, tx, s.Table.Name, s.Table.Columns, q)
	if err != nil {
		return nil, err
	}
	out := make([]T, 0, len(rows))
	for _, row := range rows {
		v, err := s.Table.Decode(row)
		if err != nil {
			return nil, fmt.Errorf("db: decode %s: %w", s.Table.Name, err)
		}
		out = append(out, v)
	}
	return out, nil
}

// First is Db.first: the first row of s (s with limit 1, or with its own
// limit when that is smaller: `Db.limit 0 |> Db.first` is Nothing).
func First[T any](ctx context.Context, st Store, tx Tx, s Select[T]) (rt.Maybe[T], error) {
	n := int64(1)
	if s.Limit != nil {
		n = min(*s.Limit, 1)
	}
	rows, err := All(ctx, st, tx, Limit(n, s))
	if err != nil || len(rows) == 0 {
		return rt.Nothing[T](), err
	}
	return rt.Just(rows[0]), nil
}

// CountRows is Db.count: the number of rows s yields (honouring its limit
// and offset).
func CountRows[T any](ctx context.Context, st Store, tx Tx, s Select[T]) (int64, error) {
	if err := txErr(tx); err != nil {
		return 0, err
	}
	q := s.SQL()
	if q.Err != nil {
		return 0, q.Err
	}
	return st.Count(ctx, tx, s.Table.Name, q)
}

// InsertRow is Db.insert; vals is the Insert record's columns in Columns
// order with nil at the id/created/updated positions. It returns the new id.
func InsertRow[T any](ctx context.Context, st Store, tx Tx, t *Table[T], vals []any, now time.Time) (string, error) {
	if err := txErr(tx); err != nil {
		return "", err
	}
	return st.Insert(ctx, tx, t.Name, t.Columns, t.ID, t.Created, t.Updated, vals, now)
}

// UpdateRow is Db.update: read row id, apply f, write it back with the
// @updated column set to now. A missing row, or an f that changes the id,
// fails with *eff.ErrFail and writes nothing.
func UpdateRow[T any](ctx context.Context, st Store, tx Tx, t *Table[T], id string, f func(T) T, now time.Time) error {
	cur, err := GetRow(ctx, st, tx, t, id)
	if err != nil {
		return err
	}
	if !cur.Ok {
		return &eff.ErrFail{Msg: fmt.Sprintf("Db.update %s: no row with id %q", shortName(t.Name), id)}
	}
	next := f(cur.Val)
	if t.ReadID(next) != id {
		return &eff.ErrFail{Msg: fmt.Sprintf("Db.update %s: update may not change the id", shortName(t.Name))}
	}
	return st.Update(ctx, tx, t.Name, t.Columns, t.ID, t.Updated, id, t.Encode(next), now)
}

// DeleteRow is Db.delete. Deleting a missing row is not an error.
func DeleteRow[T any](ctx context.Context, st Store, tx Tx, t *Table[T], id string) error {
	if err := txErr(tx); err != nil {
		return err
	}
	return st.Delete(ctx, tx, t.Name, t.ID, id)
}
