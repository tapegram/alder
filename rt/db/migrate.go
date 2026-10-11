package db

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"
	"time"
)

// Migration is one migration file of an app (migrations/NNNN_slug.sql, written by `alder
// migrate`): Name is the file's name without ".sql" ("0003_add_comment"), From and To the
// header's `-- from` and `-- to` schema hashes (their first 12 hex digits: the schema the file
// expects and the one it leaves), SQL its statements (the header removed), one per line, each
// ending in `;`.
type Migration struct{ Name, From, To, SQL string }

// HashLen is how many hex digits of a schema hash a migration file's header records.
const HashLen = 12

// ParseMigration parses the migration file named name (without ".sql") whose text is text.
// The header is the leading lines that start with `--` (the rule internal/migrate's Render
// writes by): its first line must be `-- alder migration <name>`, and it must have a
// `-- from <hash>` and a `-- to <hash>` line. After it, blank lines are skipped and every other
// line is one statement ending in `;`. An error names the migration and, for a line, its number:
// "migration 0002_x: line 7: a statement must end in `;`".
func ParseMigration(name, text string) (Migration, error) {
	m := Migration{Name: name}
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	i := 0
	for i < len(lines) && strings.HasPrefix(lines[i], "--") {
		switch l := lines[i]; {
		case strings.HasPrefix(l, "-- from "):
			m.From = strings.TrimSpace(strings.TrimPrefix(l, "-- from "))
		case strings.HasPrefix(l, "-- to "):
			m.To = strings.TrimSpace(strings.TrimPrefix(l, "-- to "))
		}
		i++
	}
	if len(lines) == 0 || lines[0] != "-- alder migration "+name {
		return m, fmt.Errorf("migration %s: line 1: the first line must be `-- alder migration %s`", name, name)
	}
	if m.From == "" || m.To == "" {
		return m, fmt.Errorf("migration %s: the header lacks its `-- from` or `-- to` line", name)
	}
	var stmts []string
	for j, l := range lines[i:] {
		if strings.TrimSpace(l) == "" {
			continue
		}
		if !strings.HasSuffix(strings.TrimSpace(l), ";") {
			return m, fmt.Errorf("migration %s: line %d: a statement must end in `;`", name, i+j+1)
		}
		stmts = append(stmts, l)
	}
	m.SQL = strings.Join(stmts, "\n")
	return m, nil
}

// ReadMigrations reads migrations/*.sql of fsys (the app's embedded migrations) in name order,
// each by ParseMigration. A malformed file panics: `alder check` (buildkit.SchemaDrift) refuses
// one before a build, so it is a bug of the build, found when the app starts.
func ReadMigrations(fsys fs.FS) []Migration {
	paths, err := fs.Glob(fsys, "migrations/*.sql")
	if err != nil {
		panic(fmt.Sprintf("db: reading the migrations: %v", err))
	}
	slices.Sort(paths)
	out := make([]Migration, 0, len(paths))
	for _, p := range paths {
		b, err := fs.ReadFile(fsys, p)
		if err != nil {
			panic(fmt.Sprintf("db: reading %s: %v", p, err))
		}
		m, err := ParseMigration(strings.TrimSuffix(path.Base(p), ".sql"), string(b))
		if err != nil {
			panic("db: " + err.Error())
		}
		out = append(out, m)
	}
	return out
}

// statements are m's statements without their `;`.
func (m Migration) statements() []string {
	var out []string
	for _, l := range strings.Split(m.SQL, "\n") {
		l = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(l), ";"))
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}

// ErrDrift is wrapped by Migrate's error when, with every migration applied, the database's
// tables are not the app's: the database was changed outside migrations, or the migrations do
// not produce the app's schema.
var ErrDrift = errors.New("the database's tables are not the app's schema")

// appSchema is the schema Ensure and Migrate compare a database with.
type appSchema struct {
	stmts  []string // CREATE TABLE statements, in the tables' order
	schema string   // stmts sorted, joined by "\n": what _alder_meta records
	hash   string   // sha256 of schema, hex
	byName map[string]TableDesc
}

// schemaOf computes the CREATE TABLE statements of tables and their hash.
func schemaOf(tables []TableDesc) (appSchema, error) {
	sc := appSchema{byName: map[string]TableDesc{}, stmts: make([]string, len(tables))}
	for _, d := range tables {
		if _, dup := sc.byName[d.Name]; dup {
			return sc, fmt.Errorf("db: table %s listed twice", d.Name)
		}
		sc.byName[d.Name] = d
	}
	for i, d := range tables {
		st, err := createStmt(d, sc.byName)
		if err != nil {
			return sc, err
		}
		sc.stmts[i] = st
	}
	sc.schema, sc.hash = hashSchema(sc.stmts)
	return sc, nil
}

// hashSchema is stmts sorted and joined by "\n", and its sha256 in hex.
func hashSchema(stmts []string) (string, string) {
	schema := strings.Join(slices.Sorted(slices.Values(stmts)), "\n")
	sum := sha256.Sum256([]byte(schema))
	return schema, hex.EncodeToString(sum[:])
}

// IndexStmts are the CREATE INDEX IF NOT EXISTS statements of the @ref columns of tables (an
// index named "<SQL table>__<column>"), sorted by index name. Ensure creates them; they are
// not part of the schema hash. internal/migrate names them the same way (a test there checks).
func IndexStmts(tables []TableDesc) []string {
	type ix struct{ name, stmt string }
	var ixs []ix
	for _, d := range tables {
		t := SQLName(d.Name)
		for _, c := range d.Columns {
			if c.Ref != "" {
				name := t + "__" + c.Name
				ixs = append(ixs, ix{name, "CREATE INDEX IF NOT EXISTS " + quote(name) + " ON " + quote(t) + "(" + quote(c.Name) + ")"})
			}
		}
	}
	slices.SortFunc(ixs, func(a, b ix) int { return strings.Compare(a.name, b.name) })
	out := make([]string, len(ixs))
	for i, x := range ixs {
		out[i] = x.stmt
	}
	return out
}

// migrationsTable records the migrations applied to a database: one row per file, by name.
const migrationsTable = "_alder_migrations"

const createMigrationsTable = `CREATE TABLE IF NOT EXISTS "_alder_migrations" ("name" TEXT NOT NULL PRIMARY KEY, "applied" TEXT NOT NULL)`

// ErrNotMigrated is wrapped by Migrate's refusal of a database that has app tables but was not
// made by migrations, and whose tables are the schema of none of them.
var ErrNotMigrated = errors.New("this database was not created by migrations and matches none of them")

// Migrate is MigrateLog without a log.
func (s *SQLite) Migrate(ctx context.Context, tables []TableDesc, ms []Migration, dev bool) (applied []string, err error) {
	return s.MigrateLog(ctx, tables, ms, dev, nil)
}

// MigrateLog brings the database to the app's schema by applying every migration in ms not yet
// recorded in _alder_migrations, in order, each in its own transaction with foreign keys off
// and PRAGMA foreign_key_check after; then it checks that the database's tables are the app's
// (the hash of their CREATE TABLE texts, Ensure's recipe). It returns the names of the
// migrations it applied, also when a later one fails (those committed). logf, when set, is told
// "migrated <name>" as each one commits, "adopted the database at <name>" and "recreated the
// database (dev mode): <reason>".
//
// Before applying anything it checks the chain: the first pending file's From is the
// database's schema hash, each next file's From the previous one's To, and the last To the
// app's hash. A broken chain is refused with nothing changed: "migration 0003_sub expects the
// schema 8f3a1b2c3d4e but the database is at 2c91aa00bb11 (is a file missing, or was the
// database changed outside migrations?)"; a last To that is not the app's wraps ErrDrift (in
// dev mode: a recreate).
//
// A database with app tables but no _alder_migrations (made by Ensure, before the app had
// migrations) is adopted when its tables are the schema some file leads to (its To): that file
// and those before it are recorded as applied without running them, and the rest apply as
// usual. When none matches: in dev mode it is recreated, else refused (ErrNotMigrated): "this
// database was not created by migrations and matches none of them: back it up, then recreate
// it (or add a migration that produces its schema)".
//
// Other refusals (an error, nothing changed): a recorded migration whose file is missing, or a
// migration not applied that comes before one that is ("migration 0002_add_comment is recorded
// as applied but its file is missing"), in dev mode too; tables changed since the last
// migration (ErrDrift) or another storage Format, unless dev, which recreates the database as
// Ensure does and records every migration of ms as applied (the fresh tables are where they
// lead).
//
// A failed statement rolls its migration back (the ones before it stay applied) and returns
// "migration <name> failed: <sqlite error>"; a migration that leaves rows referring to rows it
// deleted fails the same way, naming the referring table. A migration another process recorded
// meanwhile (two starting at once) is skipped.
func (s *SQLite) MigrateLog(ctx context.Context, tables []TableDesc, ms []Migration, dev bool, logf func(string)) (applied []string, err error) {
	if logf == nil {
		logf = func(string) {}
	}
	sc, err := schemaOf(tables)
	if err != nil {
		return nil, err
	}
	known := map[string]bool{}
	for i, m := range ms {
		if i > 0 && ms[i-1].Name >= m.Name {
			return nil, fmt.Errorf("db: migrations out of order: %s after %s", m.Name, ms[i-1].Name)
		}
		known[m.Name] = true
	}
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, createMetaTable); err != nil {
		return nil, err
	}
	recreate := func(reason string) ([]string, error) {
		existing, err := appTables(ctx, conn)
		if err != nil {
			return nil, err
		}
		err = s.recreate(ctx, conn, sc, tables, existing, func(tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, createMigrationsTable); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM "_alder_migrations"`); err != nil {
				return err
			}
			return record(ctx, tx, ms)
		})
		if err != nil {
			return nil, err
		}
		s.register(tables)
		logf("recreated the database (dev mode): " + reason)
		return nil, nil
	}

	hasLog, err := tableExists(ctx, conn, migrationsTable)
	if err != nil {
		return nil, err
	}
	existing, err := appTables(ctx, conn)
	if err != nil {
		return nil, err
	}
	oldFormat, err := metaValue(ctx, conn, "format")
	if err != nil {
		return nil, err
	}
	if len(existing) > 0 && oldFormat != Format {
		if dev {
			return recreate("its storage format is not this runtime's")
		}
		return nil, formatErr(oldFormat)
	}
	if len(existing) > 0 && !hasLog {
		_, liveHash, err := liveSchema(ctx, conn)
		if err != nil {
			return nil, err
		}
		at := -1
		for i, m := range ms {
			if m.To == short(liveHash) {
				at = i
			}
		}
		if at < 0 {
			if dev {
				return recreate("it was not created by migrations and matches none of them")
			}
			return nil, fmt.Errorf("%w: back it up, then recreate it (or add a migration that produces its schema)", ErrNotMigrated)
		}
		// the files after the adopted one must lead from it to the app's schema before anything
		// is recorded, so a refusal changes nothing
		if broken, drift := checkChain(liveHash, ms[at+1:], ms, sc.hash); broken != nil {
			return nil, broken
		} else if drift != nil {
			if dev {
				return recreate("the migrations do not produce the app's schema")
			}
			return nil, drift
		}
		adopted, err := adopt(ctx, conn, ms[:at+1])
		if err != nil {
			return nil, err
		}
		if adopted {
			logf("adopted the database at " + ms[at].Name)
		}
	}
	// created only now: a refused database stays as it was, so a later dev run still sees it was
	// not made by migrations
	if _, err := conn.ExecContext(ctx, createMigrationsTable); err != nil {
		return nil, err
	}
	recorded, err := appliedMigrations(ctx, conn)
	if err != nil {
		return nil, err
	}
	done := map[string]bool{}
	for _, r := range recorded {
		if !known[r] {
			return nil, fmt.Errorf("migration %s is recorded as applied but its file is missing", r)
		}
		done[r] = true
	}
	var pending []Migration
	for _, m := range ms {
		if done[m.Name] {
			continue
		}
		if len(recorded) > 0 && m.Name < recorded[len(recorded)-1] {
			return nil, fmt.Errorf("migration %s is not applied but %s, which comes after it, is: migrations apply in order, so give it a number after %s", m.Name, recorded[len(recorded)-1], recorded[len(recorded)-1])
		}
		pending = append(pending, m)
	}

	live, liveHash, err := liveSchema(ctx, conn)
	if err != nil {
		return nil, err
	}
	// changed by hand since the last migration?
	if len(recorded) > 0 {
		recHash, err := metaValue(ctx, conn, "schema_hash")
		if err != nil {
			return nil, err
		}
		if liveHash != recHash {
			if dev {
				return recreate("it was changed outside migrations")
			}
			recSchema, err := metaValue(ctx, conn, "schema")
			if err != nil {
				return nil, err
			}
			return nil, fmt.Errorf("db: %w: the database was changed outside migrations since %s (changed tables: %s); back it up and restore it, or run in dev mode to recreate it",
				ErrDrift, recorded[len(recorded)-1], strings.Join(schemaDiff(recSchema, live), ", "))
		}
	}
	// the chain: every pending file starts where the one before it (or the database) is
	if broken, drift := checkChain(liveHash, pending, ms, sc.hash); broken != nil {
		return nil, broken
	} else if drift != nil {
		if dev {
			return recreate("the migrations do not produce the app's schema")
		}
		return nil, drift
	}
	last := "the last migration"
	if len(ms) > 0 {
		last = ms[len(ms)-1].Name
	}

	if len(pending) > 0 {
		restore, err := foreignKeysOff(ctx, conn)
		if err != nil {
			return nil, err
		}
		defer func() {
			if ferr := restore(); ferr != nil && err == nil {
				err = ferr
			}
		}()
		for _, m := range pending {
			ran, err := applyMigration(ctx, conn, m)
			if err != nil {
				return applied, err
			}
			if ran {
				applied = append(applied, m.Name)
				logf("migrated " + m.Name)
			}
		}
	}

	live, liveHash, err = liveSchema(ctx, conn)
	if err != nil {
		return applied, err
	}
	if liveHash != sc.hash {
		if dev {
			return recreate("the migrations do not produce the app's schema")
		}
		return applied, fmt.Errorf("db: %w: after %s the tables differ from the app's (changed tables: %s): the migrations do not produce the app's schema, or the database was changed outside them",
			ErrDrift, last, strings.Join(schemaDiff(live, sc.schema), ", "))
	}
	if len(applied) == 0 { // a database whose meta predates its last migration's record: record it now
		if err := putMeta(ctx, conn, sc.schema, sc.hash); err != nil {
			return applied, err
		}
	}
	s.register(tables)
	return applied, nil
}

// checkChain checks that pending, applied to a database whose schema hash is liveHash, run in
// a chain (each file's From the schema before it) to the app's hash appHash. broken is the
// refusal of a file that does not start where it should (a file missing or edited); drift,
// wrapping ErrDrift, says the last file does not lead to the app's schema.
func checkChain(liveHash string, pending, ms []Migration, appHash string) (broken, drift error) {
	at := short(liveHash)
	for _, m := range pending {
		if m.From != at {
			return fmt.Errorf("migration %s expects the schema %s but the database is at %s (is a file missing, or was the database changed outside migrations?)", m.Name, m.From, at), nil
		}
		at = m.To
	}
	if at != short(appHash) {
		last := "the last migration"
		if len(ms) > 0 {
			last = ms[len(ms)-1].Name
		}
		return nil, fmt.Errorf("db: %w: %s leads to the schema %s, but the app's is %s: the migrations do not produce the app's schema",
			ErrDrift, last, at, short(appHash))
	}
	return nil, nil
}

// adopt records ms (the files up to the one whose To is the database's schema) as applied
// without running them, with the database's schema in _alder_meta, in one transaction. It is
// false when another process got there first (it found _alder_migrations with rows inside
// the transaction, which on a file database holds the write lock from its start): nothing done.
func adopt(ctx context.Context, conn *sql.Conn, ms []Migration) (bool, error) {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_schema WHERE "type" = 'table' AND "name" = '_alder_migrations'`).Scan(&n); err != nil {
		return false, err
	}
	if n > 0 {
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM "_alder_migrations"`).Scan(&n); err != nil {
			return false, err
		}
		if n > 0 {
			return false, nil
		}
	}
	if _, err := tx.ExecContext(ctx, createMigrationsTable); err != nil {
		return false, err
	}
	if err := record(ctx, tx, ms); err != nil {
		return false, err
	}
	live, liveHash, err := liveSchema(ctx, tx)
	if err != nil {
		return false, err
	}
	if err := putMeta(ctx, tx, live, liveHash); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

// short is a schema hash as a migration file's header records it.
func short(hash string) string { return hash[:min(len(hash), HashLen)] }

// record records ms as applied in _alder_migrations (inside tx).
func record(ctx context.Context, tx *sql.Tx, ms []Migration) error {
	for _, m := range ms {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO "_alder_migrations" ("name", "applied") VALUES (?, ?)`, m.Name, EncodeTime(time.Now())); err != nil {
			return err
		}
	}
	return nil
}

// applyMigration runs m in one transaction (foreign keys are already off): its statements,
// then PRAGMA foreign_key_check, then its record in _alder_migrations and the new schema hash.
// ran is false when another process recorded m first (read inside the transaction, which on a
// file database holds the write lock from its start), and nothing was done.
func applyMigration(ctx context.Context, conn *sql.Conn, m Migration) (ran bool, err error) {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() {
		if err != nil || !ran {
			tx.Rollback()
		}
	}()
	var n int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM "_alder_migrations" WHERE "name" = ?`, m.Name).Scan(&n); err != nil {
		return false, err
	}
	if n > 0 {
		return false, nil
	}
	for _, st := range m.statements() {
		if _, err = tx.ExecContext(ctx, st); err != nil {
			return false, fmt.Errorf("migration %s failed: %v", m.Name, err)
		}
	}
	child, err := foreignKeyViolation(ctx, tx)
	if err != nil {
		return false, fmt.Errorf("migration %s failed: %v", m.Name, err)
	}
	if child != "" {
		err = fmt.Errorf("migration %s: rows in %s refer to rows this migration deletes: delete or re-point them first", m.Name, child)
		return false, err
	}
	if err = record(ctx, tx, []Migration{m}); err != nil {
		return false, err
	}
	schema, hash, err := liveSchema(ctx, tx)
	if err != nil {
		return false, err
	}
	if err = putMeta(ctx, tx, schema, hash); err != nil {
		return false, err
	}
	if err = tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

// foreignKeyViolation is the Alder name ("Comment") of the first table PRAGMA
// foreign_key_check reports a row of, or "".
func foreignKeyViolation(ctx context.Context, q querier) (string, error) {
	rows, err := q.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return "", err
	}
	defer rows.Close()
	if !rows.Next() {
		return "", rows.Err()
	}
	var table string
	var rowid, parent, fkid any
	if err := rows.Scan(&table, &rowid, &parent, &fkid); err != nil {
		return "", err
	}
	return shortName(strings.ReplaceAll(table, "__", ".")), nil
}

// foreignKeysOff turns foreign keys off on conn (it can only change outside a transaction) and
// returns the function restoring the connection's setting (from the DSN).
func foreignKeysOff(ctx context.Context, conn *sql.Conn) (func() error, error) {
	var fk int64
	if err := conn.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&fk); err != nil {
		return nil, err
	}
	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys = OFF"); err != nil {
		return nil, err
	}
	return func() error {
		restore := "PRAGMA foreign_keys = OFF"
		if fk == 1 {
			restore = "PRAGMA foreign_keys = ON"
		}
		_, err := conn.ExecContext(context.Background(), restore)
		return err
	}, nil
}

const createMetaTable = `CREATE TABLE IF NOT EXISTS "_alder_meta" ("key" TEXT NOT NULL PRIMARY KEY, "value" TEXT NOT NULL)`

// metaValue is _alder_meta's value for key, "" when absent.
func metaValue(ctx context.Context, q querier, key string) (string, error) {
	var v string
	err := q.QueryRowContext(ctx, `SELECT "value" FROM "_alder_meta" WHERE "key" = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return v, err
}

// putMeta records schema, its hash and the storage Format in _alder_meta.
func putMeta(ctx context.Context, q querier, schema, hash string) error {
	for _, kv := range [][2]string{{"schema_hash", hash}, {"schema", schema}, {"format", Format}} {
		if _, err := q.ExecContext(ctx, `INSERT INTO "_alder_meta" ("key", "value") VALUES (?, ?) ON CONFLICT ("key") DO UPDATE SET "value" = excluded."value"`, kv[0], kv[1]); err != nil {
			return err
		}
	}
	return nil
}

// liveSchema is the database's app tables' CREATE TABLE texts as SQLite stores them, sorted
// and joined by "\n", and their hash: for a database made by Ensure or by migrations, what
// schemaOf computes for its tables.
func liveSchema(ctx context.Context, q querier) (string, string, error) {
	rows, err := q.QueryContext(ctx, `SELECT "sql" FROM sqlite_schema WHERE "type" = 'table' AND "name" NOT LIKE 'sqlite\_%' ESCAPE '\' AND "name" NOT IN ('_alder_meta', '_alder_migrations')`)
	if err != nil {
		return "", "", err
	}
	defer rows.Close()
	var stmts []string
	for rows.Next() {
		var st string
		if err := rows.Scan(&st); err != nil {
			return "", "", err
		}
		stmts = append(stmts, st)
	}
	if err := rows.Err(); err != nil {
		return "", "", err
	}
	schema, hash := hashSchema(stmts)
	return schema, hash, nil
}

// schemaDiff names the tables (by Alder name, "Post.Post") whose CREATE TABLE text differs
// between the two schema texts (joined statements): changed, added or removed.
func schemaDiff(a, b string) []string {
	byTable := func(schema string) map[string]string {
		m := map[string]string{}
		for _, st := range strings.Split(schema, "\n") {
			if st == "" {
				continue
			}
			rest, _ := strings.CutPrefix(st, "CREATE TABLE ")
			name := rest
			if i := strings.Index(rest, " ("); i >= 0 {
				name = rest[:i]
			}
			name = strings.ReplaceAll(strings.Trim(name, `"`), "__", ".")
			m[name] = st
		}
		return m
	}
	am, bm := byTable(a), byTable(b)
	var out []string
	for n, st := range am {
		if bm[n] != st {
			out = append(out, n)
		}
	}
	for n := range bm {
		if _, ok := am[n]; !ok {
			out = append(out, n)
		}
	}
	slices.Sort(out)
	if len(out) == 0 {
		out = append(out, "(unknown)")
	}
	return out
}

// tableExists reports whether the database has a table named name.
func tableExists(ctx context.Context, conn *sql.Conn, name string) (bool, error) {
	var n int
	err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_schema WHERE "type" = 'table' AND "name" = ?`, name).Scan(&n)
	return n > 0, err
}

// appliedMigrations are the names _alder_migrations records, sorted.
func appliedMigrations(ctx context.Context, conn *sql.Conn) ([]string, error) {
	rows, err := conn.QueryContext(ctx, `SELECT "name" FROM "_alder_migrations" ORDER BY "name"`)
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
