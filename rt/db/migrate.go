package db

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"slices"
	"strings"
	"time"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// Migration is one migration file of an app (migrations/NNNN_slug.sql, written by `alder
// migrate`): Name is the file's name without ".sql" ("0003_add_comment"), From and To the
// header's `-- from` and `-- to` schema hashes (their first 12 hex digits: the schema the file
// expects and the one it leaves), SQL its statements (the header removed), one per line, each
// ending in `;`. DropsData are the header's `-- DROPS DATA:` lines (what applying the file
// loses), without the prefix.
type Migration struct {
	Name, From, To, SQL string
	DropsData           []string
}

// EnvAllowDataLoss lets MigrateLog apply a migration that drops data to a database that holds
// data: `alder deploy --allow-data-loss` sets it for the one run that applies it (chapter 16).
const EnvAllowDataLoss = "ALDER_ALLOW_DATA_LOSS"

// ErrDropsData is MigrateLog's refusal of a migration that drops data, without
// EnvAllowDataLoss=1, outside dev mode, on a database that already had migrations applied.
var ErrDropsData = errors.New("a migration drops data")

// HashLen is how many hex digits of a schema hash a migration file's header records.
const HashLen = 12

// ParseMigration parses the migration file named name (without ".sql") whose text is text.
// The header is the leading lines that start with `--` (the rule internal/migrate's Render
// writes by): its first line must be `-- alder migration <name>`, and it must have a
// `-- from <hash>` and a `-- to <hash>` line. After it, blank lines and `--` comment lines are
// skipped and every other line is one statement ending in `;` (a statement added by hand goes
// on one line too). A leading byte-order mark is ignored. An error names the migration and, for a line, its number:
// "migration 0002_x: line 7: a statement must end in `;`".
func ParseMigration(name, text string) (Migration, error) {
	m := Migration{Name: name}
	text = strings.TrimPrefix(text, "\ufeff") // a byte-order mark an editor may add
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	i := 0
	for i < len(lines) && strings.HasPrefix(lines[i], "--") {
		switch l := lines[i]; {
		case strings.HasPrefix(l, "-- from "):
			m.From = strings.TrimSpace(strings.TrimPrefix(l, "-- from "))
		case strings.HasPrefix(l, "-- to "):
			m.To = strings.TrimSpace(strings.TrimPrefix(l, "-- to "))
		case strings.HasPrefix(l, "-- DROPS DATA: "):
			m.DropsData = append(m.DropsData, strings.TrimSpace(strings.TrimPrefix(l, "-- DROPS DATA: ")))
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
		if t := strings.TrimSpace(l); t == "" || strings.HasPrefix(t, "--") {
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

// ErrSchemaAhead is wrapped (with ErrDrift) by Migrate's refusal of migrations whose last file
// does not lead to the app's schema: the app's tables changed and `alder migrate` was not run.
var ErrSchemaAhead = errors.New("the migrations do not produce the app's schema")

// DevRemedy ends every refusal that dev mode would get past by recreating the database; it says
// what that costs. cmd/alder rewrites it for the databases it manages itself.
const DevRemedy = "to start over, run in dev mode (ALDER_DEV=1 recreates the database empty: every row is deleted)"

// notMigratedRemedy follows ErrNotMigrated's text in its refusal.
const notMigratedRemedy = "if the app made it before it had migrations, generate 0001 from the deployed version (check out that commit, run `alder migrate`), then migrate forward; otherwise back it up; " + DevRemedy

// Migrate is MigrateLog without a log.
func (s *SQLite) Migrate(ctx context.Context, tables []TableDesc, ms []Migration, dev bool) (applied []string, err error) {
	return s.MigrateLog(ctx, tables, ms, dev, nil)
}

// MigrateLog brings the database to the app's schema by applying every migration in ms not yet
// recorded in _alder_migrations, in order, each in its own transaction with foreign keys off;
// then it checks that the database's tables are the app's (the hash of their CREATE TABLE
// texts, Ensure's recipe) and creates the index of every @ref column (IndexStmts, as Ensure
// does: an adopted database gets them too). It returns the names of the migrations it applied,
// also when a later one fails (those committed). logf, when set, is told "migrated <name>" as
// each one commits, "adopted the database at <name>" and "recreated the database (dev mode):
// <reason>".
//
// Everything before the first migration runs (reading what is recorded, the tables, the
// format; adopting; the drift and chain checks; a dev recreate) happens in one BEGIN IMMEDIATE
// transaction, so two instances starting at once see each other's work whole: one waits, then
// finds the migrations recorded and skips them. A database another process holds past the busy
// timeout is refused with "db: the database is locked: another instance is migrating or
// serving; wait and retry (…)".
//
// Before applying anything it checks the chain: the first pending file's From is the
// database's schema hash, each next file's From the previous one's To, and the last To the
// app's hash. A broken chain is refused with nothing changed: "migration 0003_sub expects the
// schema 8f3a1b2c3d4e but the database is at 2c91aa00bb11 (is a file missing, or was the
// database changed outside migrations?)"; a last To that is not the app's wraps ErrDrift and
// ErrSchemaAhead (in dev mode: a recreate).
//
// Each migration runs in its own transaction: it checks again that the database is at the
// file's From (another process may have moved it), runs the statements, runs PRAGMA
// foreign_key_check, and checks that the tables are now the file's To; any failure rolls the
// file back (the ones before it stay applied). A file whose statements do not produce its To was
// edited after it was written: "migration 0010_drop_excerpt does not produce the schema its
// header names (to 63dc…, got f94a…): the file was edited; restore it from version control".
// A failed statement is "migration <name> failed: <sqlite error>"; rows left referring to rows
// the migration deleted name the referring table and the remedy (an UPDATE or DELETE at the
// top of the file, which was not applied). A migration another process recorded meanwhile is
// skipped.
//
// A database with app tables but no _alder_migrations (made by Ensure, before the app had
// migrations) is adopted when its tables are the schema some file leads to (its To): that file
// and those before it are recorded as applied without running them, and the rest apply as
// usual. When none matches: in dev mode it is recreated, else refused (ErrNotMigrated, with
// notMigratedRemedy).
//
// Other refusals (an error, nothing changed): a recorded migration whose file is missing (a
// newer one than every file: this executable predates the database), a migration not applied
// that comes before one that is, in dev mode too; tables changed since the last migration
// (ErrDrift) or another storage Format, unless dev, which recreates the database as Ensure does
// and records every migration of ms as applied (the fresh tables are where they lead).
func (s *SQLite) MigrateLog(ctx context.Context, tables []TableDesc, ms []Migration, dev bool, logf func(string)) (applied []string, err error) {
	s.resetStmts() // the schema may change: no prepared statement outlives it
	defer s.resetStmts()
	defer func() { err = lockedErr(err) }()
	if logf == nil {
		logf = func(string) {}
	}
	sc, err := schemaOf(tables)
	if err != nil {
		return nil, err
	}
	for i, m := range ms {
		if i > 0 && ms[i-1].Name >= m.Name {
			return nil, fmt.Errorf("db: migrations out of order: %s after %s", m.Name, ms[i-1].Name)
		}
	}
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	// foreign keys go off before the first transaction (the pragma cannot change inside one):
	// a rebuild drops and renames referenced tables
	restore, err := foreignKeysOff(ctx, conn)
	if err != nil {
		return nil, err
	}
	defer func() {
		if ferr := restore(); ferr != nil && err == nil {
			err = ferr
		}
	}()

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	r, err := s.prepare(ctx, tx, sc, tables, ms, dev)
	if err != nil || r.recreated != "" {
		if err == nil {
			err = tx.Commit()
		}
		if err != nil {
			tx.Rollback()
			return nil, err
		}
		s.register(tables)
		logf("recreated the database (dev mode): " + r.recreated)
		return nil, nil
	}
	if err := dropsData(r, ms, dev); err != nil {
		tx.Rollback()
		return nil, err
	}
	if len(r.pending) > 0 { // the read phase (and an adoption) commit; each file has its own transaction
		if err := tx.Commit(); err != nil {
			tx.Rollback()
			return nil, err
		}
		if r.adopted != "" {
			logf("adopted the database at " + r.adopted)
		}
		for _, m := range r.pending {
			ran, err := applyMigration(ctx, conn, m)
			if err != nil {
				return applied, err
			}
			if ran {
				applied = append(applied, m.Name)
				logf("migrated " + m.Name)
			}
		}
		if tx, err = conn.BeginTx(ctx, nil); err != nil {
			return applied, err
		}
	}
	recreated, err := s.finish(ctx, tx, sc, tables, ms, dev)
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		tx.Rollback()
		return applied, err
	}
	if len(r.pending) == 0 && r.adopted != "" {
		logf("adopted the database at " + r.adopted)
	}
	s.register(tables)
	if recreated {
		logf("recreated the database (dev mode): the migrations do not produce the app's schema")
		return nil, nil
	}
	return applied, nil
}

// dropsData refuses pending migrations that drop data (ErrDropsData) unless dev mode,
// EnvAllowDataLoss=1, or a database that had no migration applied (every file pending, none
// adopted: there is no data to lose).
func dropsData(r prepared, ms []Migration, dev bool) error {
	if dev || os.Getenv(EnvAllowDataLoss) == "1" || len(r.pending) == len(ms) && r.adopted == "" {
		return nil
	}
	var lines []string
	for _, m := range r.pending {
		for _, d := range m.DropsData {
			lines = append(lines, m.Name+": "+d)
		}
	}
	if len(lines) == 0 {
		return nil
	}
	return fmt.Errorf("db: %w, and nothing was applied: DROPS DATA %s; to apply it, take a backup and run with %s=1 (`alder deploy --allow-data-loss` does)",
		ErrDropsData, strings.Join(lines, "; DROPS DATA "), EnvAllowDataLoss)
}

// prepared is what MigrateLog's read phase (prepare) found: the migrations to apply, the one a
// database made by Ensure was adopted at, or why it was recreated in dev mode.
type prepared struct {
	pending   []Migration
	adopted   string // the adopted file's name, or ""
	recreated string // the dev recreate's reason, or ""
}

// prepare is MigrateLog's read phase, inside tx: it adopts a database made by Ensure, checks
// what is recorded against ms, the drift since the last migration and the chain, and in dev
// mode recreates the database instead of refusing what dev mode recreates. An error leaves tx
// to be rolled back (nothing changed: an adoption is undone with it).
func (s *SQLite) prepare(ctx context.Context, tx *sql.Tx, sc appSchema, tables []TableDesc, ms []Migration, dev bool) (r prepared, err error) {
	recreate := func(reason string) (prepared, error) {
		existing, err := appTables(ctx, tx)
		if err != nil {
			return r, err
		}
		if err := recreateTx(ctx, tx, sc, tables, existing); err != nil {
			return r, err
		}
		if err := recordAll(ctx, tx, ms); err != nil {
			return r, err
		}
		return prepared{recreated: reason}, nil
	}
	if _, err := tx.ExecContext(ctx, createMetaTable); err != nil {
		return r, err
	}
	hasLog, err := tableExists(ctx, tx, migrationsTable)
	if err != nil {
		return r, err
	}
	existing, err := appTables(ctx, tx)
	if err != nil {
		return r, err
	}
	oldFormat, err := metaValue(ctx, tx, "format")
	if err != nil {
		return r, err
	}
	if len(existing) > 0 && oldFormat != Format {
		if dev {
			return recreate("its storage format is not this runtime's")
		}
		return r, formatErr(oldFormat)
	}
	if len(existing) > 0 && !hasLog { // made by Ensure: adopt it at the file leading to its tables
		live, liveHash, err := liveSchema(ctx, tx)
		if err != nil {
			return r, err
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
			return r, fmt.Errorf("%w: %s", ErrNotMigrated, notMigratedRemedy)
		}
		if _, err := tx.ExecContext(ctx, createMigrationsTable); err != nil {
			return r, err
		}
		if err := record(ctx, tx, ms[:at+1]); err != nil {
			return r, err
		}
		if err := putMeta(ctx, tx, live, liveHash); err != nil {
			return r, err
		}
		r.adopted = ms[at].Name
	}
	if _, err := tx.ExecContext(ctx, createMigrationsTable); err != nil {
		return r, err
	}
	recorded, err := appliedMigrations(ctx, tx)
	if err != nil {
		return r, err
	}
	if r.pending, err = pendingAfter(recorded, ms); err != nil {
		return r, err
	}

	live, liveHash, err := liveSchema(ctx, tx)
	if err != nil {
		return r, err
	}
	if len(recorded) > 0 { // changed by hand since the last migration?
		recHash, err := metaValue(ctx, tx, "schema_hash")
		if err != nil {
			return r, err
		}
		if liveHash != recHash {
			if dev {
				return recreate("it was changed outside migrations")
			}
			recSchema, err := metaValue(ctx, tx, "schema")
			if err != nil {
				return r, err
			}
			return r, fmt.Errorf("db: %w: the database was changed outside migrations since %s (changed tables: %s): restore a backup; %s",
				ErrDrift, recorded[len(recorded)-1], strings.Join(schemaDiff(recSchema, live), ", "), DevRemedy)
		}
	}
	if broken, drift := checkChain(liveHash, r.pending, ms, sc.hash); broken != nil {
		return r, broken
	} else if drift != nil {
		if dev {
			return recreate("the migrations do not produce the app's schema")
		}
		return r, drift
	}
	return r, nil
}

// pendingAfter is the migrations of ms that recorded (the names _alder_migrations holds,
// sorted) lacks, in order. It refuses a recorded name without its file (one after every file of
// ms: this executable predates the database, migrationAt's ahead) and a file not applied that
// comes before one that is.
func pendingAfter(recorded []string, ms []Migration) ([]Migration, error) {
	known := map[string]bool{}
	for _, m := range ms {
		known[m.Name] = true
	}
	done := map[string]bool{}
	for _, name := range recorded {
		if !known[name] {
			if _, ahead := migrationAt(name, ms); ahead || len(ms) == 0 {
				newest := recorded[len(recorded)-1]
				return nil, fmt.Errorf("migration %s is recorded as applied but its file is missing; this executable predates the database (it has had %s): deploy a build that includes it, or restore a backup taken before it", name, newest)
			}
			return nil, fmt.Errorf("migration %s is recorded as applied but its file is missing: restore it from version control", name)
		}
		done[name] = true
	}
	var pending []Migration
	for _, m := range ms {
		if done[m.Name] {
			continue
		}
		if len(recorded) > 0 && m.Name < recorded[len(recorded)-1] {
			return nil, fmt.Errorf("migration %s is not applied but %s, which comes after it, is: migrations apply in order; delete the file and run `alder migrate` again (only if it has not run against a database you keep)", m.Name, recorded[len(recorded)-1])
		}
		pending = append(pending, m)
	}
	return pending, nil
}

// migrationAt places a database's last migration, name, among this build's ms: at is its
// index (-1 when ms has no file of that name), and ahead reports a name after every file of ms
// (a newer build migrated the database). The shared database (pendingAfter, CheckShared) and a
// silo's (Stores.inspect) are placed alike.
func migrationAt(name string, ms []Migration) (at int, ahead bool) {
	at = -1
	for i, m := range ms {
		if m.Name == name {
			at = i
		}
	}
	return at, name != "" && at < 0 && len(ms) > 0 && name > ms[len(ms)-1].Name
}

// SharedState is how the shared database stands against this build (CheckShared).
type SharedState string

const (
	SharedOK     SharedState = "ok"     // its tables are this build's: serve
	SharedBehind SharedState = "behind" // this build has migrations it has not had: `alder deploy` runs them
	SharedAhead  SharedState = "ahead"  // a newer build migrated it: serve the maintenance page
	SharedFresh  SharedState = "fresh"  // no tables at all: no deploy has migrated it yet
)

// CheckShared compares the shared database s with this build (its tables and migrations ms)
// without changing it: it reads what MigrateLog would (_alder_migrations, _alder_meta, the live
// tables) and writes nothing, takes no write lock, creates no table. A binary serving a database
// on Turso calls it instead of migrating on start (several instances would race; `alder deploy`
// migrates from the deploying machine before the new build starts).
//
// The detail says what was found: the pending migrations of SharedBehind, the newer migration
// of SharedAhead. On SharedOK it registers tables with s, as MigrateLog does, so that s serves
// them. A database that is in none of the four states is an error, with the text
// MigrateLog's refusal would have: a recorded migration whose file is missing (and that is not
// newer than every file), a file not applied before one that is, another storage format, tables
// changed outside migrations since the last one, or tables that are not the app's although
// nothing is pending.
func CheckShared(ctx context.Context, s *SQLite, tables []TableDesc, ms []Migration) (SharedState, string, error) {
	sc, err := schemaOf(tables)
	if err != nil {
		return "", "", err
	}
	q := s.db
	hasMeta, err := tableExists(ctx, q, "_alder_meta")
	if err != nil {
		return "", "", err
	}
	hasLog, err := tableExists(ctx, q, migrationsTable)
	if err != nil {
		return "", "", err
	}
	existing, err := appTables(ctx, q)
	if err != nil {
		return "", "", err
	}
	if !hasMeta && !hasLog && len(existing) == 0 {
		return SharedFresh, "the database has no tables", nil
	}
	if hasMeta && len(existing) > 0 {
		if old, err := metaValue(ctx, q, "format"); err != nil {
			return "", "", err
		} else if old != Format {
			return "", "", formatErr(old)
		}
	}
	var recorded []string
	if hasLog {
		if recorded, err = appliedMigrations(ctx, q); err != nil {
			return "", "", err
		}
	}
	live, liveHash, err := liveSchema(ctx, q)
	if err != nil {
		return "", "", err
	}
	if len(recorded) == 0 {
		switch {
		case len(ms) > 0:
			return SharedBehind, "no migration is recorded: a deploy runs " + strings.Join(names(ms), ", "), nil
		case liveHash != sc.hash:
			return SharedBehind, "its tables are not this build's", nil
		}
		s.register(tables)
		return SharedOK, "", nil
	}
	newest := recorded[len(recorded)-1]
	if _, ahead := migrationAt(newest, ms); ahead || len(ms) == 0 {
		return SharedAhead, fmt.Sprintf("a newer build migrated it (to %s, which this build does not have)", newest), nil
	}
	pending, err := pendingAfter(recorded, ms)
	if err != nil {
		return "", "", err
	}
	if !hasMeta {
		return "", "", fmt.Errorf("db: %w: _alder_migrations records %s but _alder_meta is missing: restore a backup", ErrDrift, newest)
	}
	recHash, err := metaValue(ctx, q, "schema_hash")
	if err != nil {
		return "", "", err
	}
	if liveHash != recHash {
		recSchema, err := metaValue(ctx, q, "schema")
		if err != nil {
			return "", "", err
		}
		return "", "", fmt.Errorf("db: %w: the database was changed outside migrations since %s (changed tables: %s): restore a backup",
			ErrDrift, newest, strings.Join(schemaDiff(recSchema, live), ", "))
	}
	if len(pending) > 0 {
		return SharedBehind, "pending: " + strings.Join(names(pending), ", "), nil
	}
	if liveHash != sc.hash {
		return "", "", fmt.Errorf("db: %w: the database is at %s, this build's last migration, but its tables are not the app's: the app's tables changed without `alder migrate`", ErrDrift, newest)
	}
	s.register(tables)
	return SharedOK, "", nil
}

// names are the migrations' names.
func names(ms []Migration) []string {
	out := make([]string, len(ms))
	for i, m := range ms {
		out[i] = m.Name
	}
	return out
}

// finish is MigrateLog's last step, inside tx: the tables must be the app's (else, in dev
// mode, they are recreated: recreated is true); it records the app's schema in _alder_meta and
// creates the @ref indexes.
func (s *SQLite) finish(ctx context.Context, tx *sql.Tx, sc appSchema, tables []TableDesc, ms []Migration, dev bool) (recreated bool, err error) {
	live, liveHash, err := liveSchema(ctx, tx)
	if err != nil {
		return false, err
	}
	if liveHash != sc.hash {
		if !dev {
			last := "the last migration"
			if len(ms) > 0 {
				last = ms[len(ms)-1].Name
			}
			return false, fmt.Errorf("db: %w: after %s the tables differ from the app's (changed tables: %s): the migrations do not produce the app's schema, or the database was changed outside them",
				ErrDrift, last, strings.Join(schemaDiff(live, sc.schema), ", "))
		}
		existing, err := appTables(ctx, tx)
		if err != nil {
			return false, err
		}
		if err := recreateTx(ctx, tx, sc, tables, existing); err != nil {
			return false, err
		}
		return true, recordAll(ctx, tx, ms)
	}
	if err := putMeta(ctx, tx, sc.schema, sc.hash); err != nil {
		return false, err
	}
	for _, st := range IndexStmts(tables) {
		if _, err := tx.ExecContext(ctx, st); err != nil {
			return false, fmt.Errorf("db: %s: %w", st, err)
		}
	}
	return false, nil
}

// recordAll makes _alder_migrations record exactly ms (a dev recreate: the fresh tables are
// where they lead).
func recordAll(ctx context.Context, tx *sql.Tx, ms []Migration) error {
	if _, err := tx.ExecContext(ctx, createMigrationsTable); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM "_alder_migrations"`); err != nil {
		return err
	}
	return record(ctx, tx, ms)
}

// checkChain checks that pending, applied to a database whose schema hash is liveHash, run in
// a chain (each file's From the schema before it) to the app's hash appHash. broken is the
// refusal of a file that does not start where it should (a file missing or edited); drift,
// wrapping ErrDrift and ErrSchemaAhead, says the last file does not lead to the app's schema.
func checkChain(liveHash string, pending, ms []Migration, appHash string) (broken, drift error) {
	at := short(liveHash)
	for _, m := range pending {
		if m.From != at {
			return notAt(m, at), nil
		}
		at = m.To
	}
	if at == short(appHash) {
		return nil, nil
	}
	if len(pending) == 0 {
		last := "its last migration"
		if len(ms) > 0 {
			last = ms[len(ms)-1].Name
		}
		return nil, fmt.Errorf("db: %w: the database is at the schema %s (after %s), but the app's is %s: %w",
			ErrDrift, at, last, short(appHash), ErrSchemaAhead)
	}
	return nil, fmt.Errorf("db: %w: %s leads to the schema %s, but the app's is %s: %w",
		ErrDrift, pending[len(pending)-1].Name, at, short(appHash), ErrSchemaAhead)
}

// notAt is the refusal of migration m when the database's schema (its short hash) is at, not
// m's From.
func notAt(m Migration, at string) error {
	return fmt.Errorf("migration %s expects the schema %s but the database is at %s (is a file missing, or was the database changed outside migrations?)", m.Name, m.From, at)
}

// lockedErr names the likely cause of SQLite's "database is locked" (another process held the
// write lock past the busy timeout); other errors pass through.
func lockedErr(err error) error {
	if err == nil {
		return nil
	}
	var se *sqlite.Error
	busy := errors.As(err, &se) && (se.Code()&0xff == sqlite3.SQLITE_BUSY || se.Code()&0xff == sqlite3.SQLITE_LOCKED)
	if !busy && !strings.Contains(err.Error(), "database is locked") {
		return err
	}
	return fmt.Errorf("db: the database is locked: another instance is migrating or serving; wait and retry (%w)", err)
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

// applyMigration runs m in one transaction (foreign keys are already off): it checks that the
// database is at m.From, runs m's statements, then PRAGMA foreign_key_check, checks that the
// tables are now m.To (a file edited after it was written is caught here, rolled back), then
// records m in _alder_migrations with the new schema in _alder_meta. ran is false when another
// process recorded m first (read inside the transaction, which on a file database holds the
// write lock from its start), and nothing was done.
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
	_, before, err := liveSchema(ctx, tx)
	if err != nil {
		return false, err
	}
	if short(before) != m.From {
		return false, notAt(m, short(before))
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
		err = fmt.Errorf("migration %s: rows in %s refer to rows this migration deletes: delete or re-point them first: add an UPDATE or DELETE at the top of %s.sql (it was not applied)", m.Name, child, m.Name)
		return false, err
	}
	schema, hash, err := liveSchema(ctx, tx)
	if err != nil {
		return false, err
	}
	if short(hash) != m.To {
		err = fmt.Errorf("migration %s does not produce the schema its header names (to %s, got %s): the file was edited; restore it from version control", m.Name, m.To, short(hash))
		return false, err
	}
	if err = record(ctx, tx, []Migration{m}); err != nil {
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
	rows, err := q.QueryContext(ctx, `SELECT "sql" FROM sqlite_schema WHERE "type" = 'table' AND "name" NOT LIKE 'sqlite\_%' ESCAPE '\' AND "name" NOT IN (`+internalTables+`)`)
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

// schemaDiff names the tables (by their display name, "Post") whose CREATE TABLE text differs
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
			m[displayName(strings.Trim(name, `"`))] = st
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
func tableExists(ctx context.Context, q querier, name string) (bool, error) {
	var n int
	err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_schema WHERE "type" = 'table' AND "name" = ?`, name).Scan(&n)
	return n > 0, err
}

// appliedMigrations are the names _alder_migrations records, sorted.
func appliedMigrations(ctx context.Context, q querier) ([]string, error) {
	rows, err := q.QueryContext(ctx, `SELECT "name" FROM "_alder_migrations" ORDER BY "name"`)
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
