package db

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"

	"alder/rt/eff"
)

// SQLite is the Store backed by an SQLite database (modernc.org/sqlite, no
// cgo). Every connection runs with foreign_keys=ON and busy_timeout=5000;
// a file database also uses journal_mode=WAL, synchronous=NORMAL and
// BEGIN IMMEDIATE transactions.
type SQLite struct {
	db *sql.DB

	mu     sync.RWMutex
	tables map[string]TableDesc // by Alder name; filled by Ensure
}

// OpenSQLite opens (creating if needed) the database at path. ":memory:"
// is a private in-memory database; it is held on a single connection so
// every query sees it.
func OpenSQLite(path string) (*SQLite, error) {
	if path == "" || strings.Contains(path, "?") {
		return nil, fmt.Errorf("db: invalid SQLite path %q", path)
	}
	const common = "_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	dsn := path + "?" + common
	if path != ":memory:" {
		dsn += "&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_txlock=immediate"
	}
	d, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if path == ":memory:" {
		d.SetMaxOpenConns(1)
		d.SetConnMaxLifetime(0)
		d.SetConnMaxIdleTime(0)
	}
	if err := d.Ping(); err != nil {
		d.Close()
		return nil, fmt.Errorf("db: open %s: %w", path, err)
	}
	return &SQLite{db: d, tables: map[string]TableDesc{}}, nil
}

// Close closes the database.
func (s *SQLite) Close() error { return s.db.Close() }

type sqliteTx struct{ tx *sql.Tx }

func (t *sqliteTx) Commit() error   { return t.tx.Commit() }
func (t *sqliteTx) Rollback() error { return t.tx.Rollback() }

// Begin starts a transaction (eff.Store).
func (s *SQLite) Begin(ctx context.Context) (eff.Tx, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	return &sqliteTx{tx}, nil
}

type querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// on is tx's *sql.Tx, or the database itself when tx is nil.
func (s *SQLite) on(tx Tx) (querier, error) {
	if tx == nil {
		return s.db, nil
	}
	t, ok := tx.(*sqliteTx)
	if !ok {
		return nil, fmt.Errorf("db: transaction %T does not belong to the SQLite store", tx)
	}
	return t.tx, nil
}

func (s *SQLite) desc(table string) (TableDesc, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	d, ok := s.tables[table]
	if !ok {
		return TableDesc{}, fmt.Errorf("db: unknown table %q (not passed to Ensure)", table)
	}
	return d, nil
}

func colList(cols []Column) string {
	names := make([]string, len(cols))
	for i, c := range cols {
		names[i] = quote(c.Name)
	}
	return strings.Join(names, ", ")
}

// Get reads row id of table (a table passed to Ensure) in Columns order.
func (s *SQLite) Get(ctx context.Context, tx Tx, table string, id string) ([]any, bool, error) {
	d, err := s.desc(table)
	if err != nil {
		return nil, false, err
	}
	rows, err := s.Query(ctx, tx, table, d.Columns, SQL{Where: quote(d.ID) + " = ?", Args: []any{id}})
	if err != nil || len(rows) == 0 {
		return nil, false, err
	}
	return rows[0], true, nil
}

// tail renders WHERE/ORDER BY/LIMIT/OFFSET and appends the limit/offset args.
func tail(sel SQL) (string, []any) {
	var b strings.Builder
	args := append([]any(nil), sel.Args...)
	if sel.Where != "" {
		b.WriteString(" WHERE " + sel.Where)
	}
	if sel.Order != "" {
		b.WriteString(" ORDER BY " + sel.Order)
	}
	if sel.Limit != nil || sel.Offset != nil {
		b.WriteString(" LIMIT ?")
		if sel.Limit != nil {
			args = append(args, *sel.Limit)
		} else {
			args = append(args, int64(-1)) // no limit
		}
		if sel.Offset != nil {
			b.WriteString(" OFFSET ?")
			args = append(args, *sel.Offset)
		}
	}
	return b.String(), args
}

// Query reads cols of the rows sel selects.
func (s *SQLite) Query(ctx context.Context, tx Tx, table string, cols []Column, sel SQL) ([][]any, error) {
	q, err := s.on(tx)
	if err != nil {
		return nil, err
	}
	rest, args := tail(sel)
	rows, err := q.QueryContext(ctx, "SELECT "+colList(cols)+" FROM "+quote(SQLName(table))+rest, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out [][]any
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		out = append(out, vals)
	}
	return out, rows.Err()
}

// Count counts the rows sel selects (honouring its limit and offset).
func (s *SQLite) Count(ctx context.Context, tx Tx, table string, sel SQL) (int64, error) {
	q, err := s.on(tx)
	if err != nil {
		return 0, err
	}
	sel.Order = ""
	rest, args := tail(sel)
	var n int64
	err = q.QueryRowContext(ctx, "SELECT COUNT(*) FROM (SELECT 1 FROM "+quote(SQLName(table))+rest+")", args...).Scan(&n)
	return n, err
}

// Insert implements Store.Insert.
func (s *SQLite) Insert(ctx context.Context, tx Tx, table string, cols []Column, idCol, createdCol, updatedCol string, vals []any, now time.Time) (string, error) {
	if len(vals) != len(cols) {
		return "", fmt.Errorf("db: insert %s: %d values for %d columns", table, len(vals), len(cols))
	}
	q, err := s.on(tx)
	if err != nil {
		return "", err
	}
	id := NewID()
	args := append([]any(nil), vals...)
	stamp := EncodeTime(now)
	for i, c := range cols {
		switch c.Name {
		case idCol:
			args[i] = id
		case createdCol, updatedCol:
			args[i] = stamp
		}
	}
	stmt := "INSERT INTO " + quote(SQLName(table)) + " (" + colList(cols) + ") VALUES (" +
		strings.TrimSuffix(strings.Repeat("?, ", len(cols)), ", ") + ")"
	if _, err := q.ExecContext(ctx, stmt, args...); err != nil {
		return "", s.refErr(ctx, q, table, cols, args, uniqueErr(table, err))
	}
	return id, nil
}

// Update implements Store.Update. The id and the table's @created column
// (as passed to Ensure) are never rewritten.
func (s *SQLite) Update(ctx context.Context, tx Tx, table string, cols []Column, idCol, updatedCol, id string, vals []any, now time.Time) error {
	if len(vals) != len(cols) {
		return fmt.Errorf("db: update %s: %d values for %d columns", table, len(vals), len(cols))
	}
	q, err := s.on(tx)
	if err != nil {
		return err
	}
	created := ""
	if d, err := s.desc(table); err == nil {
		created = d.Created
	}
	var sets []string
	var args []any
	for i, c := range cols {
		switch c.Name {
		case idCol, created:
			continue
		case updatedCol:
			args = append(args, EncodeTime(now))
		default:
			args = append(args, vals[i])
		}
		sets = append(sets, quote(c.Name)+" = ?")
	}
	if len(sets) == 0 {
		return nil
	}
	args = append(args, id)
	stmt := "UPDATE " + quote(SQLName(table)) + " SET " + strings.Join(sets, ", ") + " WHERE " + quote(idCol) + " = ?"
	if _, err := q.ExecContext(ctx, stmt, args...); err != nil {
		return s.refErr(ctx, q, table, cols, vals, uniqueErr(table, err))
	}
	return nil
}

// Delete implements Store.Delete; a missing row is not an error.
func (s *SQLite) Delete(ctx context.Context, tx Tx, table string, idCol, id string) error {
	q, err := s.on(tx)
	if err != nil {
		return err
	}
	_, err = q.ExecContext(ctx, "DELETE FROM "+quote(SQLName(table))+" WHERE "+quote(idCol)+" = ?", id)
	if isForeignKey(err) {
		return &eff.ErrFail{Msg: shortName(table) + " " + id + " is referenced by another row"}
	}
	return err
}

// isForeignKey reports whether err is SQLite's foreign-key constraint failure.
func isForeignKey(err error) bool {
	if err == nil {
		return false
	}
	var se *sqlite.Error
	if errors.As(err, &se) && se.Code() == sqlite3.SQLITE_CONSTRAINT_FOREIGNKEY {
		return true
	}
	return strings.Contains(err.Error(), "FOREIGN KEY constraint failed")
}

// refErr turns a foreign-key failure of an insert or update of vals (cols order) into
// &eff.ErrFail{Msg: "<Table>.<col>: no <RefTable> with that id"}, naming the first @ref column
// whose value is not an id of its table (SQLite does not say which); other errors pass through.
func (s *SQLite) refErr(ctx context.Context, q querier, table string, cols []Column, vals []any, err error) error {
	if !isForeignKey(err) {
		return err
	}
	for i, c := range cols {
		if c.Ref == "" || i >= len(vals) || vals[i] == nil {
			continue
		}
		ref, derr := s.desc(c.Ref)
		if derr != nil {
			continue
		}
		var one int
		qerr := q.QueryRowContext(ctx, "SELECT 1 FROM "+quote(SQLName(ref.Name))+" WHERE "+quote(ref.ID)+" = ?", vals[i]).Scan(&one)
		if errors.Is(qerr, sql.ErrNoRows) {
			return &eff.ErrFail{Msg: shortName(table) + "." + c.Name + ": no " + shortName(c.Ref) + " with that id"}
		}
	}
	return &eff.ErrFail{Msg: shortName(table) + ": a @ref column names a row that does not exist"}
}

// uniqueErr turns SQLite's "UNIQUE constraint failed: Todo__Todo.title" into
// &eff.ErrFail{Msg: "Todo.title must be unique"}; other errors pass through.
func uniqueErr(table string, err error) error {
	var se *sqlite.Error
	isUnique := errors.As(err, &se) &&
		(se.Code() == sqlite3.SQLITE_CONSTRAINT_UNIQUE || se.Code() == sqlite3.SQLITE_CONSTRAINT_PRIMARYKEY)
	msg := err.Error()
	const marker = "UNIQUE constraint failed: "
	at := strings.Index(msg, marker)
	if !isUnique && at < 0 {
		return err
	}
	name := shortName(table)
	if at >= 0 {
		rest := msg[at+len(marker):]
		if end := strings.IndexAny(rest, ", ()"); end >= 0 {
			rest = rest[:end]
		}
		if col, ok := strings.CutPrefix(rest, SQLName(table)+"."); ok && col != "" {
			return &eff.ErrFail{Msg: name + "." + col + " must be unique"}
		}
	}
	return &eff.ErrFail{Msg: name + ": a @unique column must be unique (could not identify which: " + msg + ")"}
}

// shortName is the table name without its module: "Todo.Todo" → "Todo".
func shortName(table string) string {
	if i := strings.LastIndex(table, "."); i >= 0 {
		return table[i+1:]
	}
	return table
}

func sqlType(t ColType) string {
	switch t {
	case CInt, CBool:
		return "INTEGER"
	case CFloat:
		return "REAL"
	}
	return "TEXT"
}

// createStmt is the one-line CREATE TABLE for d; byName resolves @ref.
func createStmt(d TableDesc, byName map[string]TableDesc) (string, error) {
	if !slices.ContainsFunc(d.Columns, func(c Column) bool { return c.Name == d.ID }) {
		return "", fmt.Errorf("db: table %s: id column %q is not among its columns", d.Name, d.ID)
	}
	defs := make([]string, len(d.Columns))
	for i, c := range d.Columns {
		def := quote(c.Name) + " " + sqlType(c.Type)
		if !c.Nullable {
			def += " NOT NULL"
		}
		if c.Name == d.ID {
			def += " PRIMARY KEY"
		} else if c.Unique {
			def += " UNIQUE"
		}
		switch c.Type {
		case CBool:
			def += " CHECK (" + quote(c.Name) + " IN (0, 1))"
		case CEnum:
			lits := make([]string, len(c.Enum))
			for j, e := range c.Enum {
				lits[j] = literal(e)
			}
			def += " CHECK (" + quote(c.Name) + " IN (" + strings.Join(lits, ", ") + "))"
		}
		if c.Ref != "" {
			ref, ok := byName[c.Ref]
			if !ok {
				return "", fmt.Errorf("db: %s.%s: @ref to unknown table %q", d.Name, c.Name, c.Ref)
			}
			def += " REFERENCES " + quote(SQLName(ref.Name)) + "(" + quote(ref.ID) + ")"
		}
		defs[i] = def
	}
	return "CREATE TABLE " + quote(SQLName(d.Name)) + " (" + strings.Join(defs, ", ") + ") STRICT", nil
}

// Ensure creates tables (in order) and records a hash of their CREATE TABLE
// statements in _alder_meta. When the stored hash differs (or app tables
// exist without one): with dev, every app table is dropped and the schema
// recreated empty; without dev, Ensure returns an error naming the tables
// that changed and touches nothing.
func (s *SQLite) Ensure(ctx context.Context, tables []TableDesc, dev bool) (err error) {
	byName := map[string]TableDesc{}
	for _, d := range tables {
		if _, dup := byName[d.Name]; dup {
			return fmt.Errorf("db: table %s listed twice", d.Name)
		}
		byName[d.Name] = d
	}
	stmts := make([]string, len(tables))
	for i, d := range tables {
		if stmts[i], err = createStmt(d, byName); err != nil {
			return err
		}
	}
	schema := strings.Join(stmts, "\n")
	sum := sha256.Sum256([]byte(schema))
	hash := hex.EncodeToString(sum[:])

	conn, err := s.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS "_alder_meta" ("key" TEXT NOT NULL PRIMARY KEY, "value" TEXT NOT NULL)`); err != nil {
		return err
	}
	meta := func(key string) (string, error) {
		var v string
		err := conn.QueryRowContext(ctx, `SELECT "value" FROM "_alder_meta" WHERE "key" = ?`, key).Scan(&v)
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return v, err
	}
	oldHash, err := meta("schema_hash")
	if err != nil {
		return err
	}
	register := func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		for _, d := range tables {
			s.tables[d.Name] = d
		}
	}
	if oldHash == hash {
		register()
		return nil
	}
	existing, err := appTables(ctx, conn)
	if err != nil {
		return err
	}
	if len(existing) > 0 && !dev {
		oldSchema, err := meta("schema")
		if err != nil {
			return err
		}
		return fmt.Errorf("db: schema mismatch: the database's schema (hash %.12s) differs from the app's (hash %.12s); changed tables: %s; run in dev mode to recreate the database",
			oldHash, hash, strings.Join(changedTables(tables, stmts, oldSchema), ", "))
	}

	// Dropping a referenced table with foreign keys on would cascade checks;
	// the pragma can only change outside a transaction. Restore the
	// connection's setting (from the DSN) afterwards.
	var fk int64
	if err := conn.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&fk); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys = OFF"); err != nil {
		return err
	}
	defer func() {
		restore := "PRAGMA foreign_keys = OFF"
		if fk == 1 {
			restore = "PRAGMA foreign_keys = ON"
		}
		if _, ferr := conn.ExecContext(context.Background(), restore); ferr != nil && err == nil {
			err = ferr
		}
	}()
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()
	for _, name := range existing {
		if _, err = tx.ExecContext(ctx, "DROP TABLE "+quote(name)); err != nil {
			return err
		}
	}
	for _, st := range stmts {
		if _, err = tx.ExecContext(ctx, st); err != nil {
			return fmt.Errorf("db: %s: %w", st, err)
		}
	}
	for k, v := range map[string]string{"schema_hash": hash, "schema": schema} {
		if _, err = tx.ExecContext(ctx, `INSERT INTO "_alder_meta" ("key", "value") VALUES (?, ?) ON CONFLICT ("key") DO UPDATE SET "value" = excluded."value"`, k, v); err != nil {
			return err
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	register()
	return nil
}

// appTables lists every table except SQLite's own and _alder_meta.
func appTables(ctx context.Context, conn *sql.Conn) ([]string, error) {
	rows, err := conn.QueryContext(ctx, `SELECT "name" FROM sqlite_schema WHERE "type" = 'table' AND "name" NOT LIKE 'sqlite\_%' ESCAPE '\' AND "name" <> '_alder_meta' ORDER BY "name"`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// changedTables names the tables whose CREATE statement is not in oldSchema,
// and the old tables that are gone.
func changedTables(tables []TableDesc, stmts []string, oldSchema string) []string {
	old := strings.Split(oldSchema, "\n")
	var out []string
	cur := map[string]bool{}
	for i, d := range tables {
		cur[`CREATE TABLE `+quote(SQLName(d.Name))+` `] = true
		if !slices.Contains(old, stmts[i]) {
			out = append(out, d.Name)
		}
	}
	for _, st := range old {
		if st == "" {
			continue
		}
		rest, _ := strings.CutPrefix(st, "CREATE TABLE ")
		name := rest
		if i := strings.Index(rest, " ("); i >= 0 {
			name = rest[:i]
		}
		if !cur[`CREATE TABLE `+name+` `] {
			out = append(out, "removed "+name)
		}
	}
	if len(out) == 0 {
		out = append(out, "(unknown)")
	}
	return out
}
