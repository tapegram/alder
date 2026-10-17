package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"slices"
	"strconv"
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
// BEGIN IMMEDIATE transactions, and, when ALDER_BACKUP_URL is set,
// wal_autocheckpoint=0: the backup shipper checkpoints instead.
type SQLite struct {
	db *sql.DB

	mu     sync.RWMutex
	tables map[string]TableDesc // by Alder name; filled by Ensure

	remote bool // a database on Turso (libsql.go), not a file
}

// Remote reports whether the store is a database on Turso (ALDER_DB=libsql://…, or a silo's
// child database) rather than an SQLite file.
func (s *SQLite) Remote() bool { return s.remote }

// OpenSQLite opens (creating if needed) the database at path. ":memory:"
// is a private in-memory database; it is held on a single connection so
// every query sees it. A path libsql://<host> is a database on Turso, reached
// over its HTTP protocol (libsql.go; its token from the environment: turso.go).
func OpenSQLite(path string) (*SQLite, error) { return openSQLite(path, false) }

// OpenSQLiteReader opens the file database at path read-only (PRAGMA query_only: every write
// statement fails with SQLite's "attempt to write a readonly database"). A Task of an app with
// silos reads another store than its own through one (Stores), outside its transaction.
func OpenSQLiteReader(path string) (*SQLite, error) {
	if path == ":memory:" {
		return nil, fmt.Errorf("db: a reader needs a file database")
	}
	return openSQLite(path, true)
}

func openSQLite(path string, reader bool) (*SQLite, error) {
	if IsRemote(path) {
		return openRemote(path, reader)
	}
	if path == "" || strings.Contains(path, "?") {
		return nil, fmt.Errorf("db: invalid SQLite path %q", path)
	}
	const common = "_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	dsn := path + "?" + common
	if reader {
		dsn += "&_pragma=query_only(1)"
	} else if path != ":memory:" {
		dsn += "&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_txlock=immediate"
		if os.Getenv("ALDER_BACKUP_URL") != "" {
			// the backup shipper (rt/backup) checkpoints, right after copying the WAL's frames
			dsn += "&_pragma=wal_autocheckpoint(0)"
		}
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
	// a new file database turns to WAL on its first connection, which SQLite refuses at once
	// (no busy wait) while another process holds it: retry for as long as busy_timeout waits
	deadline := time.Now().Add(5 * time.Second)
	for {
		err := d.Ping()
		if err == nil {
			break
		}
		if lockedErr(err) == err || time.Now().After(deadline) {
			d.Close()
			return nil, lockedErr(fmt.Errorf("db: open %s: %w", path, err))
		}
		time.Sleep(10 * time.Millisecond)
	}
	return &SQLite{db: d, tables: map[string]TableDesc{}}, nil
}

// Close closes the database.
func (s *SQLite) Close() error { return s.db.Close() }

// Ping runs SELECT 1 on the database: nil when it answers (the /_alder/health route of rt/web).
func (s *SQLite) Ping(ctx context.Context) error {
	var n int
	return s.db.QueryRowContext(ctx, "SELECT 1").Scan(&n)
}

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

var _ Savepointer = (*SQLite)(nil)

// Savepoint opens savepoint name inside tx (Savepointer). name must be an identifier the
// caller controls (rtesting uses alder_t<n>); it is not quoted.
func (s *SQLite) Savepoint(ctx context.Context, tx eff.Tx, name string) error {
	return s.savepointStmt(ctx, tx, "SAVEPOINT "+name)
}

// Release commits savepoint name into its transaction (Savepointer): its writes stay and
// are committed or rolled back with the transaction.
func (s *SQLite) Release(ctx context.Context, tx eff.Tx, name string) error {
	return s.savepointStmt(ctx, tx, "RELEASE "+name)
}

// RollbackTo undoes everything since savepoint name was opened and keeps the savepoint
// (Savepointer); the transaction's earlier writes stay.
func (s *SQLite) RollbackTo(ctx context.Context, tx eff.Tx, name string) error {
	return s.savepointStmt(ctx, tx, "ROLLBACK TO "+name)
}

func (s *SQLite) savepointStmt(ctx context.Context, tx eff.Tx, stmt string) error {
	if tx == nil {
		return errors.New("db: savepoint outside a transaction")
	}
	q, err := s.on(tx)
	if err != nil {
		return err
	}
	_, err = q.ExecContext(ctx, stmt)
	return err
}

type querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// on is tx's *sql.Tx, or the database itself when tx is nil.
func (s *SQLite) on(tx Tx) (querier, error) {
	if tx == nil {
		return recQ{s.db}, nil
	}
	if err := txErr(tx); err != nil {
		return nil, err
	}
	t, ok := tx.(*sqliteTx)
	if !ok {
		return nil, fmt.Errorf("db: transaction %T does not belong to the SQLite store", tx)
	}
	return recQ{t.tx}, nil
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
	return s.GetWhere(ctx, tx, table, id, "", nil)
}

// GetWhere reads row id of table when its column col also holds val (col "" for no such
// condition): one SELECT … WHERE id = ? AND col = ?.
func (s *SQLite) GetWhere(ctx context.Context, tx Tx, table string, id string, col string, val any) ([]any, bool, error) {
	d, err := s.desc(table)
	if err != nil {
		return nil, false, err
	}
	sel := SQL{Where: quote(d.ID) + " = ?", Args: []any{id}}
	if col != "" {
		sel.Where += " AND " + quote(col) + " = ?"
		sel.Args = append(sel.Args, val)
	}
	rows, err := s.Query(ctx, tx, table, d.Columns, sel)
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
	stmt := "SELECT " + colList(cols) + " FROM " + quote(SQLName(table)) + rest
	rows, err := q.QueryContext(ctx, stmt, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out [][]any
	defer func() { rowsRead(ctx, stmt, len(out)) }()
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
	stmt := "SELECT COUNT(*) FROM (SELECT 1 FROM " + quote(SQLName(table)) + rest + ")"
	err = q.QueryRowContext(ctx, stmt, args...).Scan(&n)
	if err == nil {
		rowsRead(ctx, stmt, 1)
	}
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
	_, err := s.UpdateWhere(ctx, tx, table, cols, idCol, updatedCol, id, "", nil, vals, now)
	return err
}

// UpdateWhere implements Store.UpdateWhere: one statement, UPDATE … SET … WHERE id = ? [AND
// col = ?], whose rows-affected count it returns. The id and the table's @created column (as
// passed to Ensure) are never rewritten.
func (s *SQLite) UpdateWhere(ctx context.Context, tx Tx, table string, cols []Column, idCol, updatedCol, id, col string, val any, vals []any, now time.Time) (int64, error) {
	if len(vals) != len(cols) {
		return 0, fmt.Errorf("db: update %s: %d values for %d columns", table, len(vals), len(cols))
	}
	q, err := s.on(tx)
	if err != nil {
		return 0, err
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
	if len(sets) == 0 { // nothing to rewrite: the statement still says whether the row is there
		sets = append(sets, quote(idCol)+" = "+quote(idCol))
	}
	args = append(args, id)
	where := quote(idCol) + " = ?"
	if col != "" {
		where += " AND " + quote(col) + " = ?"
		args = append(args, val)
	}
	stmt := "UPDATE " + quote(SQLName(table)) + " SET " + strings.Join(sets, ", ") + " WHERE " + where
	res, err := q.ExecContext(ctx, stmt, args...)
	if err != nil {
		return 0, s.refErr(ctx, q, table, cols, vals, uniqueErr(table, err))
	}
	return res.RowsAffected()
}

// Delete implements Store.Delete; a missing row is not an error.
func (s *SQLite) Delete(ctx context.Context, tx Tx, table string, idCol, id string) error {
	_, err := s.DeleteWhere(ctx, tx, table, idCol, id, "", nil)
	return err
}

// DeleteWhere implements Store.DeleteWhere: one statement, DELETE … WHERE id = ? [AND col = ?],
// whose rows-affected count it returns.
func (s *SQLite) DeleteWhere(ctx context.Context, tx Tx, table string, idCol, id, col string, val any) (int64, error) {
	q, err := s.on(tx)
	if err != nil {
		return 0, err
	}
	where := quote(idCol) + " = ?"
	args := []any{id}
	if col != "" {
		where += " AND " + quote(col) + " = ?"
		args = append(args, val)
	}
	res, err := q.ExecContext(ctx, "DELETE FROM "+quote(SQLName(table))+" WHERE "+where, args...)
	if isForeignKey(err) {
		// SQLite does not say which row blocks. A `@restrict` row referring to this one directly
		// is named by its table and column; else it is one a `@cascade` chain reaches.
		if by := s.restrictReferrer(ctx, q, table, id); by != "" {
			return 0, &eff.ErrFail{Msg: shortName(table) + " " + id + " is referenced by a row of " + by}
		}
		return 0, &eff.ErrFail{Msg: shortName(table) + " " + id + " is referenced by another row, directly or through rows that `@cascade` would delete with it"}
	}
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// restrictReferrer is "Comment (`Comment.post`)" for the first table, by name, with a
// `@restrict` @ref column to table holding a row that names id; "" when there is none.
func (s *SQLite) restrictReferrer(ctx context.Context, q querier, table, id string) string {
	s.mu.RLock()
	names := make([]string, 0, len(s.tables))
	for n := range s.tables {
		names = append(names, n)
	}
	slices.Sort(names)
	type ref struct{ table, col string }
	var refs []ref
	for _, n := range names {
		for _, c := range s.tables[n].Columns {
			if c.Ref == table && c.OnDelete == Restrict {
				refs = append(refs, ref{n, c.Name})
			}
		}
	}
	s.mu.RUnlock()
	for _, r := range refs {
		var one int
		stmt := "SELECT 1 FROM " + quote(SQLName(r.table)) + " WHERE " + quote(r.col) + " = ? LIMIT 1"
		err := q.QueryRowContext(ctx, stmt, id).Scan(&one)
		if err != nil {
			rowsRead(ctx, stmt, 0)
			continue
		}
		rowsRead(ctx, stmt, 1)
		return shortName(r.table) + " (`" + shortName(r.table) + "." + r.col + "`)"
	}
	return ""
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

// displayName is the name a person knows the table of SQL name sql by: "Todo__Todo" → "Todo"
// (a table lives in the module named after it, and module names have no "__").
func displayName(sql string) string {
	if i := strings.LastIndex(sql, "__"); i >= 0 {
		return sql[i+2:]
	}
	return sql
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

// CreateStmt is the one-line CREATE TABLE statement Ensure creates d with and hashes; byName
// resolves @ref. The migration generator (internal/migrate) diffs over it.
func CreateStmt(d TableDesc, byName map[string]TableDesc) (string, error) {
	return createStmt(d, byName)
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
			switch c.OnDelete { // Restrict, the default, writes nothing: SQLite's default refuses
			case Cascade:
				def += " ON DELETE CASCADE"
			case SetNull:
				def += " ON DELETE SET NULL"
			}
		}
		defs[i] = def
	}
	return "CREATE TABLE " + quote(SQLName(d.Name)) + " (" + strings.Join(defs, ", ") + ") STRICT", nil
}

// Format is the storage format version Ensure records in _alder_meta ("format"). Bump it
// whenever createStmt (or anything else about how a table is laid out in SQL) changes, so a
// database made by an older runtime is recognised instead of silently misread.
const Format = "1"

// Ensure creates tables and records, in _alder_meta, the storage format
// version (Format) and a hash of their CREATE TABLE statements sorted, so the
// order the tables are listed in does not matter. When the stored format or
// hash differs (or app tables exist without them): with dev, every app table
// is dropped and the schema recreated empty; without dev, Ensure returns an
// error — "database format v0 is older than this runtime (v1)" for a format
// change, one naming the tables that changed for a schema change — and
// touches nothing. It also creates the index of every @ref column (IndexStmts,
// CREATE INDEX IF NOT EXISTS: a database made before them gets them), which the
// hash does not cover.
//
// Ensure is for an app without migrations; an app with them calls Migrate instead.
func (s *SQLite) Ensure(ctx context.Context, tables []TableDesc, dev bool) (err error) {
	defer func() { err = lockedErr(err) }()
	sc, err := schemaOf(tables)
	if err != nil {
		return err
	}
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, createMetaTable); err != nil {
		return err
	}
	oldHash, err := metaValue(ctx, conn, "schema_hash")
	if err != nil {
		return err
	}
	oldFormat, err := metaValue(ctx, conn, "format")
	if err != nil {
		return err
	}
	if oldHash == sc.hash && oldFormat == Format {
		for _, st := range IndexStmts(tables) {
			if _, err := conn.ExecContext(ctx, st); err != nil {
				return fmt.Errorf("db: %s: %w", st, err)
			}
		}
		s.register(tables)
		return nil
	}
	existing, err := appTables(ctx, conn)
	if err != nil {
		return err
	}
	if len(existing) > 0 && !dev && oldFormat != Format {
		return formatErr(oldFormat)
	}
	if len(existing) > 0 && !dev {
		oldSchema, err := metaValue(ctx, conn, "schema")
		if err != nil {
			return err
		}
		return fmt.Errorf("db: schema mismatch: the database's schema (hash %.12s) differs from the app's (hash %.12s); changed tables: %s; %s",
			oldHash, sc.hash, strings.Join(changedTables(tables, sc.stmts, oldSchema), ", "), DevRemedy)
	}
	if err := s.recreate(ctx, conn, sc, tables, existing); err != nil {
		return err
	}
	s.register(tables)
	return nil
}

// CreateTables creates the tables extra in a database that already has the app's tables app
// (made by Ensure or Migrate): a test's fake capabilities keep their rows in them. Each gets a
// plain CREATE TABLE (its @ref columns resolved against app and extra) and the index of each
// @ref column, all in one transaction, and is registered with the store. Nothing is recorded in
// _alder_meta and the app's schema hash is untouched, so a table of extra is never part of the
// app's schema; for the same reason Migrate must not run on the database afterwards (its final
// check would see extra as drift). A table of extra that already exists (or is one of app) is an
// error, and nothing is created.
func (s *SQLite) CreateTables(ctx context.Context, extra, app []TableDesc) (err error) {
	defer func() { err = lockedErr(err) }()
	if len(extra) == 0 {
		return nil
	}
	byName := map[string]TableDesc{}
	for _, d := range app {
		byName[d.Name] = d
	}
	for _, d := range extra {
		if _, dup := byName[d.Name]; dup {
			return fmt.Errorf("db: table %s is already one of the app's tables or listed twice", d.Name)
		}
		byName[d.Name] = d
	}
	stmts := make([]string, 0, len(extra))
	for _, d := range extra {
		st, err := createStmt(d, byName)
		if err != nil {
			return err
		}
		stmts = append(stmts, st)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()
	for _, d := range extra {
		exists, err := tableExists(ctx, tx, SQLName(d.Name))
		if err != nil {
			return err
		}
		if exists {
			return fmt.Errorf("db: table %s already exists", d.Name)
		}
	}
	for _, st := range append(stmts, IndexStmts(extra)...) {
		if _, err := tx.ExecContext(ctx, st); err != nil {
			return fmt.Errorf("db: %s: %w", st, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.register(extra)
	return nil
}

// register makes tables known to the store's operations (desc).
func (s *SQLite) register(tables []TableDesc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, d := range tables {
		s.tables[d.Name] = d
	}
}

// formatErr is the refusal of a database of another storage format (oldFormat "" is v0).
func formatErr(oldFormat string) error {
	if oldFormat == "" {
		oldFormat = "0"
	}
	rel := "older"
	if n, ferr := strconv.Atoi(oldFormat); ferr == nil && n > formatNumber() {
		rel = "newer"
	}
	return fmt.Errorf("db: database format v%s is %s than this runtime (v%s); %s", oldFormat, rel, Format, DevRemedy)
}

// recreate drops the tables existing and creates sc's (tables'), their @ref indexes and the
// _alder_meta record, in one transaction with foreign keys off (recreateTx).
func (s *SQLite) recreate(ctx context.Context, conn *sql.Conn, sc appSchema, tables []TableDesc, existing []string) (err error) {
	// Dropping a referenced table with foreign keys on would cascade checks;
	// the pragma can only change outside a transaction.
	restore, err := foreignKeysOff(ctx, conn)
	if err != nil {
		return err
	}
	defer func() {
		if ferr := restore(); ferr != nil && err == nil {
			err = ferr
		}
	}()
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err = recreateTx(ctx, tx, sc, tables, existing); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

// recreateTx drops the tables existing and creates sc's (tables'), their @ref indexes and the
// _alder_meta record, inside tx (whose connection has foreign keys off).
func recreateTx(ctx context.Context, tx *sql.Tx, sc appSchema, tables []TableDesc, existing []string) error {
	for _, name := range existing {
		if _, err := tx.ExecContext(ctx, "DROP TABLE "+quote(name)); err != nil {
			return err
		}
	}
	for _, st := range append(slices.Clone(sc.stmts), IndexStmts(tables)...) {
		if _, err := tx.ExecContext(ctx, st); err != nil {
			return fmt.Errorf("db: %s: %w", st, err)
		}
	}
	return putMeta(ctx, tx, sc.schema, sc.hash)
}

func formatNumber() int {
	n, _ := strconv.Atoi(Format)
	return n
}

// appTables lists every table except SQLite's own, _alder_meta and _alder_migrations.
func appTables(ctx context.Context, q querier) ([]string, error) {
	rows, err := q.QueryContext(ctx, `SELECT "name" FROM sqlite_schema WHERE "type" = 'table' AND "name" NOT LIKE 'sqlite\_%' ESCAPE '\' AND "name" NOT IN ('_alder_meta', '_alder_migrations', '_alder_schema', '_alder_lock') ORDER BY "name"`)
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
			out = append(out, shortName(d.Name))
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
			out = append(out, "removed "+displayName(strings.Trim(name, `"`)))
		}
	}
	if len(out) == 0 {
		out = append(out, "(unknown)")
	}
	return out
}

// rowsRead records n as the rows statement stmt read, when the running effect has a dev
// Recorder (recQ records a read before its rows are read).
func rowsRead(ctx context.Context, stmt string, n int) {
	if rc := rec(ctx); rc != nil {
		rc.SetRows(stmt, int64(n))
	}
}

// recQ wraps a querier: when the running effect has a dev Recorder (eff.Env.Rec) each statement is
// recorded with its arguments and duration; otherwise it is a plain pass-through.
type recQ struct{ q querier }

func rec(ctx context.Context) *eff.Recorder {
	if e := eff.EnvFrom(ctx); e != nil {
		return e.Rec
	}
	return nil
}

func (r recQ) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	rc := rec(ctx)
	if rc == nil {
		return r.q.ExecContext(ctx, query, args...)
	}
	t0 := time.Now()
	res, err := r.q.ExecContext(ctx, query, args...)
	n := int64(-1)
	if err == nil {
		if a, e := res.RowsAffected(); e == nil {
			n = a
		}
	}
	rc.AddSQL(eff.SQLRec{Stmt: query, Args: args, Ms: eff.Ms(time.Since(t0)), Rows: n})
	return res, err
}

func (r recQ) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	rc := rec(ctx)
	if rc == nil {
		return r.q.QueryContext(ctx, query, args...)
	}
	t0 := time.Now()
	rows, err := r.q.QueryContext(ctx, query, args...)
	rc.AddSQL(eff.SQLRec{Stmt: query, Args: args, Ms: eff.Ms(time.Since(t0)), Rows: -1})
	return rows, err
}

func (r recQ) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	rc := rec(ctx)
	if rc == nil {
		return r.q.QueryRowContext(ctx, query, args...)
	}
	t0 := time.Now()
	row := r.q.QueryRowContext(ctx, query, args...)
	rc.AddSQL(eff.SQLRec{Stmt: query, Args: args, Ms: eff.Ms(time.Since(t0)), Rows: -1})
	return row
}
