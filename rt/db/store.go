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

// TxOf is e.Tx as a Tx, or nil when e.Tx is nil (in a Query).
func TxOf(e *eff.Env) Tx {
	if e.Tx == nil {
		return nil
	}
	return e.Tx.(Tx)
}

// ErrUnique is the generic @unique failure, used only when the violated
// column cannot be identified. A store returns a fresh
// &eff.ErrFail{Msg: "<Table>.<column> must be unique"} otherwise.
var ErrUnique = &eff.ErrFail{Msg: "unique constraint violated"}

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
	rows, err := st.Query(ctx, tx, s.Table.Name, s.Table.Columns, s.SQL())
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

// First is Db.first: the first row of s (s with limit 1).
func First[T any](ctx context.Context, st Store, tx Tx, s Select[T]) (rt.Maybe[T], error) {
	rows, err := All(ctx, st, tx, Limit(1, s))
	if err != nil || len(rows) == 0 {
		return rt.Nothing[T](), err
	}
	return rt.Just(rows[0]), nil
}

// CountRows is Db.count: the number of rows s yields (honouring its limit
// and offset).
func CountRows[T any](ctx context.Context, st Store, tx Tx, s Select[T]) (int64, error) {
	return st.Count(ctx, tx, s.Table.Name, s.SQL())
}

// InsertRow is Db.insert; vals is the Insert record's columns in Columns
// order with nil at the id/created/updated positions. It returns the new id.
func InsertRow[T any](ctx context.Context, st Store, tx Tx, t *Table[T], vals []any, now time.Time) (string, error) {
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
	return st.Delete(ctx, tx, t.Name, t.ID, id)
}
