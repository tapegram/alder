package db

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base32"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"alder/rt/eff"
)

// The stores of an app with silos (M15).
//
// `silo org { Project, Issue }` keeps those tables in one database per value of the session's
// `org`; every other table lives in the shared database. On disk (Stores.Dir is the directory of
// ALDER_DB, the shared database's file):
//
//	<dir>/app.db                       the shared store (ALDER_DB; any name)
//	<dir>/silos/<key>/<value>.db       one silo: <key> the session field, <value> the id's text
//
// A silo database holds the silo's tables only. Their `@ref` columns to a table of the same silo
// keep their REFERENCES (a @cascade inside the silo works); one to a shared table is a plain
// column (SQLite cannot refer into another database). The shared database is made by the app's
// migrations as before, so it also has the siloed tables, always empty.
//
// Each silo database records, in the one row of `_alder_schema`, the schema hash of the binary
// that last created or migrated it (the hash of every table's CREATE TABLE, the shared store's
// `_alder_meta` schema_hash) and that binary's last migration ("" without migrations). When a
// process first opens a silo it compares the row with its own (openSilo):
//
//   - equal: the silo is served, and the result cached for the process;
//   - behind (the row names an earlier migration of this binary): the pending migrations are
//     applied on open, each one's statements on the silo's tables only, in one transaction,
//     unless one of them DROPS DATA of the silo's tables: then the silo is not served (the
//     app's maintenance page, 503) and is listed `pending-data-loss` until `alder migrate
//     silos <env> --allow-data-loss` (the binary's --migrate-silos) applies it;
//   - ahead (the row names a migration this binary does not have: a newer build migrated it):
//     not served, listed `ahead`; the remedy is the newer build, or restoring the silo.
//
// A request for a silo that is not served answers the maintenance page; every other silo, and
// the shared store, go on serving.

// Silo is one `silo key { T, … }` of App: the session field and its tables (module-qualified,
// as Table.Name).
type Silo struct {
	Key    string
	Tables []string
	// Ref is the table the key's values are ids of (the siloed tables' partition column's
	// @ref, module-qualified: "Org.Org"): a silo whose row there is gone is orphaned.
	Ref string
}

// SiloState is a silo database's state, as `alder status` lists it (the binary's --silo-status).
type SiloState string

const (
	SiloOK        SiloState = "ok"                // served
	SiloMigrating SiloState = "migrating"         // behind, by migrations it applies when it is next opened
	SiloDataLoss  SiloState = "pending-data-loss" // behind by a migration that drops data: maintenance
	SiloAhead     SiloState = "ahead"             // migrated by a newer build: maintenance
	SiloFailed    SiloState = "failed"            // cannot be opened, or a migration failed: maintenance
	// SiloOrphaned is a silo whose key row (its Org) is gone: nothing reaches it any more, and
	// `alder silos prune <env>` (the binary's --prune-silos) removes its database.
	SiloOrphaned SiloState = "orphaned"
)

// MaintenanceError is the error of a step on a silo that is not served: a route answers the
// app's maintenance page (503). errors.As finds it through any wrapping.
type MaintenanceError struct {
	Key, Value string
	State      SiloState
	Detail     string
}

func (e *MaintenanceError) Error() string {
	return "silo " + e.Key + "/" + e.Value + ": " + string(e.State) + ": " + e.Detail
}

// StoresConfig configures the stores of an app with silos.
type StoresConfig struct {
	// Dir is the directory of the silos (Dir/silos/<key>/<value>.db): the shared database's
	// (SiloDir). "" keeps every silo in memory (a shared store in memory; tests). A libsql://
	// URL (the shared database on Turso) puts every silo in a child database on Turso
	// (stores_turso.go), reached with the runtime's Turso settings (TursoConfigFromEnv).
	Dir string
	// Shared is the shared database (ALDER_DB), which --silo-status reads to find the orphaned
	// silos ("": not looked for).
	Shared     string
	Silos      []Silo
	Tables     []TableDesc // every table of the app (Tables)
	Migrations []Migration // the app's migrations (none: an app made by Ensure)
	Dev        bool        // a silo whose schema changed without migrations is recreated
	// IdleClose is how long an open silo goes unused before it is closed (0: idleClose).
	IdleClose time.Duration
	// AllowDataLoss applies a pending migration that drops data on open (--migrate-silos
	// --allow-data-loss); without it such a silo is not served.
	AllowDataLoss bool
	Log           func(level, msg string) // nil: no log
}

// Stores is the shared store and the silos of an app with `silo` declarations. Generated code
// puts it in every Env (eff.Env.Stores); rt/db's operations route each step on a siloed table to
// its silo (route). It is safe for concurrent use.
type Stores struct {
	shared *SQLite
	reader *SQLite // a read-only connection to the shared file; nil in memory (shared itself)
	cfg    StoresConfig
	silos  map[string]*siloSchema // by key
	hash   string                 // what a silo's _alder_schema row records: siloHash of the app's schema and the silos
	last   string                 // the app's last migration, "" for none
	test   bool                   // NewTestStores
	remote *Turso                 // silos on Turso (Dir is a libsql:// URL)
	app    string                 // the app's name on Turso (the databases' prefix)

	// mu guards open (and sps) only: a silo is opened and checked outside it, under its own
	// entry's ready channel, so a slow migration of one silo never holds up another (I1).
	mu    sync.Mutex
	open  map[string]*siloDB // by "key/value"
	swept time.Time          // the last eviction sweep (sweep)
	// draining is set by Drain (a graceful shutdown): no silo is opened any more, and every
	// write to an open one re-checks its schema row first (M3).
	draining atomic.Bool
	sps      []string // test: the savepoints open on the test's transactions, outermost first
}

// siloSchema is a silo's tables as its databases hold them.
type siloSchema struct {
	key     string
	descs   []TableDesc     // @ref to a table outside the silo removed
	names   map[string]bool // Alder names
	sqlName map[string]bool // SQL names
	short   []string        // their names without the module, for DROPS DATA lines
	stmts   []string        // CREATE TABLE statements
	indexes []string        // the @ref indexes (of the app's descs: a removed @ref keeps its index)
	hash    string          // of stmts, as liveSchema computes it
}

// siloDB is one opened silo.
type siloDB struct {
	key, value string
	store      *SQLite
	reader     *SQLite // a read-only connection (nil in memory)
	state      SiloState
	detail     string
	checked    time.Time
	tx         Tx // test: its transaction, begun on first use
	// ready is closed once the silo is opened and checked: until then the fields above are the
	// opener's alone, and every other request for the silo waits on it (only it).
	ready chan struct{}
	used  atomic.Int64 // unix nanoseconds of its last use, for the idle eviction
}

// recheck is how long a silo found not servable stays so before a request opens it again (an
// `alder migrate silos` run meanwhile makes it servable).
const recheck = 60 * time.Second

// EnvSiloRecheck shortens recheck, for alder's own tests (a duration such as "200ms"): a silo
// found not servable is opened again that much later. Production never sets it.
const EnvSiloRecheck = "ALDER_SILO_RECHECK"

// recheckAfter is recheck, or EnvSiloRecheck's duration when it is set.
func recheckAfter() time.Duration {
	if d, err := time.ParseDuration(os.Getenv(EnvSiloRecheck)); err == nil && d > 0 {
		return d
	}
	return recheck
}

// idleClose is how long an open silo goes unused before it is closed (its connections freed),
// unless StoresConfig.IdleClose says otherwise; it is opened (and checked) again on next use.
const idleClose = 10 * time.Minute

// OpenStores is the stores of an app with silos over the shared store shared (opened, migrated)
// at sharedPath. Silos are opened on first use.
func OpenStores(shared *SQLite, sharedPath string, cfg StoresConfig) (*Stores, error) {
	s, err := newStores(shared, cfg)
	if err != nil {
		return nil, err
	}
	if sharedPath != "" && sharedPath != ":memory:" {
		r, err := OpenSQLiteReader(sharedPath)
		if err != nil {
			return nil, err
		}
		r.register(cfg.Tables)
		s.reader = r
	} else {
		s.cfg.Dir = ""
	}
	liveMu.Lock()
	live = append(live, s)
	liveMu.Unlock()
	return s, nil
}

// NewTestStores is the stores of one test (rt/testing): every silo a fresh in-memory database,
// created when the test first touches it. A test's Task reads and writes every store inside
// one transaction per store, so the test's Env.Store is TestStore, whose savepoints span them all.
func NewTestStores(shared *SQLite, cfg StoresConfig) (*Stores, error) {
	cfg.Dir = ""
	s, err := newStores(shared, cfg)
	if err != nil {
		return nil, err
	}
	s.test = true
	return s, nil
}

func newStores(shared *SQLite, cfg StoresConfig) (*Stores, error) {
	sc, err := schemaOf(cfg.Tables)
	if err != nil {
		return nil, err
	}
	s := &Stores{shared: shared, cfg: cfg, silos: map[string]*siloSchema{}, hash: siloHash(short(sc.hash), cfg.Silos), open: map[string]*siloDB{}}
	if IsRemote(cfg.Dir) {
		tc := TursoConfigFromEnv()
		if tc.Org == "" || tc.App == "" || tc.Token == "" {
			return nil, fmt.Errorf("db: the silos are on Turso (ALDER_DB is %s), which needs %s, %s and %s", cfg.Dir, EnvTursoOrg, EnvTursoApp, EnvTursoToken)
		}
		s.remote, s.app = NewTurso(tc), tc.App
	}
	if n := len(cfg.Migrations); n > 0 {
		s.last = cfg.Migrations[n-1].Name
	}
	for _, si := range cfg.Silos {
		ss, err := newSiloSchema(si, sc.byName)
		if err != nil {
			return nil, err
		}
		s.silos[si.Key] = ss
	}
	return s, nil
}

// siloHash is the hash a silo's _alder_schema row records (hex sha256): of the app's schema
// hash as a migration file's header records it (appShort) and of the silos' membership (each
// key's tables), so a build that moves a table into a silo — whose tables are unchanged, as is
// the app's schema hash — does not find a silo made before the move up to date: the silo is
// behind by the migration the move wrote (`alder new silo --empty`), which creates the table in
// it. A migration's To is such a short hash, so a row that names no migration (M4) is matched
// to the one that leads to it.
func siloHash(appShort string, silos []Silo) string {
	lines := []string{appShort}
	for _, si := range silos {
		lines = append(lines, "silo "+si.Key+": "+strings.Join(slices.Sorted(slices.Values(si.Tables)), ", "))
	}
	slices.Sort(lines[1:])
	sum := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	return hex.EncodeToString(sum[:])
}

func newSiloSchema(si Silo, byName map[string]TableDesc) (*siloSchema, error) {
	ss := &siloSchema{key: si.Key, names: map[string]bool{}, sqlName: map[string]bool{}}
	for _, n := range si.Tables {
		ss.names[n] = true
		ss.sqlName[SQLName(n)] = true
		ss.short = append(ss.short, shortName(n))
	}
	var orig []TableDesc
	for _, n := range si.Tables {
		d, ok := byName[n]
		if !ok {
			return nil, fmt.Errorf("db: silo %s: no table %s", si.Key, n)
		}
		orig = append(orig, d)
		d.Columns = slices.Clone(d.Columns)
		for i, c := range d.Columns {
			if c.Ref != "" && !ss.names[c.Ref] {
				d.Columns[i].Ref, d.Columns[i].OnDelete = "", Restrict
			}
		}
		ss.descs = append(ss.descs, d)
	}
	sc, err := schemaOf(ss.descs)
	if err != nil {
		return nil, err
	}
	ss.stmts, ss.hash = sc.stmts, sc.hash
	ss.indexes = IndexStmts(orig)
	return ss, nil
}

// Shared is the shared store.
func (s *Stores) Shared() *SQLite { return s.shared }

// Close closes every silo (a test's transactions are rolled back first) and the reader.
func (s *Stores) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var errs []error
	for _, d := range s.open {
		if d.tx != nil {
			d.tx.Rollback()
		}
		if d.store != nil {
			errs = append(errs, d.store.Close())
		}
		if d.reader != nil {
			errs = append(errs, d.reader.Close())
		}
	}
	s.open = map[string]*siloDB{}
	liveMu.Lock()
	if i := slices.Index(live, s); i >= 0 {
		live = slices.Delete(live, i, i+1)
	}
	liveMu.Unlock()
	if s.reader != nil {
		errs = append(errs, s.reader.Close())
	}
	return errors.Join(errs...)
}

// SiloDir is StoresConfig.Dir for the shared database at path: its directory, or for a
// database on Turso (libsql://…) the URL itself (the silos are its child databases).
func SiloDir(path string) string {
	if IsRemote(path) {
		return path
	}
	return filepath.Dir(path)
}

// SiloPath is the file of silo key's value under dir: dir/silos/<key>/<SiloFileName(value)>. The
// value must be an id (validID: NewID's alphabet, 1 to 64 characters); anything else is refused.
func SiloPath(dir, key, value string) (string, error) {
	if err := safeName(key); err != nil {
		return "", fmt.Errorf("db: silo key %q: %v", key, err)
	}
	if err := safeName(value); err != nil {
		return "", fmt.Errorf("db: silo %s value %q: %v", key, value, err)
	}
	return filepath.Join(dir, "silos", key, SiloFileName(value)), nil
}

// SiloFileName is the file name of the silo of id value: the id's text in base32hex (RFC 4648
// §7), lowercase, without padding, then ".db". Ids are case-sensitive and a file system may not
// be, so the id's own text is never the name ("a" and "A" are two silos, "a.db" and "A.db" one
// file on macOS); the encoding is reversible (SiloValue), uses [0-9a-v] only, and a 64-character
// id makes a 103-character name.
func SiloFileName(value string) string {
	return strings.ToLower(siloEnc.EncodeToString([]byte(value))) + ".db"
}

// SiloValue is the id a silo file name (SiloFileName's) stands for; false for any other name.
func SiloValue(name string) (string, bool) {
	enc, ok := strings.CutSuffix(name, ".db")
	if !ok || enc != strings.ToLower(enc) {
		return "", false
	}
	b, err := siloEnc.DecodeString(strings.ToUpper(enc))
	if err != nil || !validID(string(b)) || SiloFileName(string(b)) != name {
		return "", false
	}
	return string(b), true
}

var siloEnc = base32.HexEncoding.WithPadding(base32.NoPadding)

// idPattern is NewID's alphabet (base64url) and the longest id a silo is opened for.
var idPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// validID reports whether v can be the id of a silo's key row: 1 to 64 characters of NewID's
// alphabet. A request for any other value is not-found, and no file is made for it.
func validID(v string) bool { return idPattern.MatchString(v) }

// safeName refuses a silo key or value that is not an id (validID).
func safeName(v string) error {
	if !validID(v) {
		return errors.New("not an id: 1 to 64 letters, digits, _ or -")
	}
	return nil
}

// silo is the opened silo key/value: checked when first opened (openSilo), its state cached.
// A silo not served is a *MaintenanceError (checked again after recheck). The map lock is held
// only to find or add the entry: the opening and checking (a migration, perhaps) happen outside
// it, and a request for another silo — or for an open one — never waits for them; a request
// for the same silo waits on its entry's ready channel.
func (s *Stores) silo(ctx context.Context, key, value string) (*siloDB, error) {
	ss := s.silos[key]
	if ss == nil {
		return nil, fmt.Errorf("db: %w: no silo %q", eff.ErrInternal, key)
	}
	if !validID(value) {
		return nil, eff.ErrNotFound // no row has such an id: nothing to open, no file made
	}
	id := key + "/" + value
	now := time.Now()
	s.mu.Lock()
	s.sweep(now)
	d := s.open[id]
	if d == nil { // first open: its key row must exist (one shared read), else not-found, no file
		s.mu.Unlock()
		ok, err := s.keyExists(ctx, key, value)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, eff.ErrNotFound
		}
		s.mu.Lock()
		d = s.open[id]
	}
	if d != nil && isReady(d) && d.state != SiloOK && now.Sub(d.checked) > recheckAfter() {
		delete(s.open, id) // a failed silo holds no connection (openSilo's fail closed it)
		d = nil
	}
	opener := false
	if d == nil {
		if s.draining.Load() {
			s.mu.Unlock()
			return nil, &MaintenanceError{Key: key, Value: value, State: SiloFailed, Detail: "the process is shutting down (a deploy's drain): it opens no silo"}
		}
		d = &siloDB{key: key, value: value, ready: make(chan struct{})}
		s.open[id] = d
		opener = true
	}
	d.used.Store(now.UnixNano()) // under the lock: a sweep never closes a silo found here
	s.mu.Unlock()
	if opener {
		o := s.openSilo(ctx, ss, value, s.cfg.AllowDataLoss)
		d.store, d.reader, d.state, d.detail, d.checked = o.store, o.reader, o.state, o.detail, o.checked
		close(d.ready)
	} else {
		select {
		case <-d.ready:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if d.state != SiloOK {
		return nil, &MaintenanceError{Key: key, Value: value, State: d.state, Detail: d.detail}
	}
	return d, nil
}

// keyExists reports whether silo key's value has its key row (an Org of that id) in the shared
// store, the table Silo.Ref names: read through the running effect's transaction when it is
// the shared store's (a test's, or a Task on the shared store), else outside any.
func (s *Stores) keyExists(ctx context.Context, key, value string) (bool, error) {
	ref := ""
	for _, si := range s.cfg.Silos {
		if si.Key == key {
			ref = si.Ref
		}
	}
	if ref == "" || s.shared == nil {
		return true, nil // generated before Silo.Ref: not checked
	}
	d, err := s.shared.desc(ref)
	if err != nil {
		return false, err
	}
	var q querier = recQ{s.shared.db}
	if e := eff.EnvFrom(ctx); e != nil && e.Tx != nil && (s.test || e.SiloKey == "") {
		if q, err = s.shared.on(e.Tx); err != nil {
			return false, err
		}
	} else if s.reader != nil {
		q = recQ{s.reader.db}
	}
	var one int
	err = q.QueryRowContext(ctx, "SELECT 1 FROM "+quote(SQLName(ref))+" WHERE "+quote(d.ID)+" = ?", value).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// isReady reports whether d is opened and checked.
func isReady(d *siloDB) bool {
	select {
	case <-d.ready:
		return true
	default:
		return false
	}
}

// sweep (under s.mu, at most once a minute) closes the silos unused for IdleClose (M2): not a
// test's, whose transaction stays open. A Task holding one runs far shorter than that.
func (s *Stores) sweep(now time.Time) {
	if s.test || now.Sub(s.swept) < time.Minute {
		return
	}
	s.swept = now
	idle := s.cfg.IdleClose
	if idle <= 0 {
		idle = idleClose
	}
	for id, d := range s.open {
		if isReady(d) && d.state == SiloOK && d.tx == nil && now.Sub(time.Unix(0, d.used.Load())) > idle {
			d.close()
			delete(s.open, id)
		}
	}
}

// Drain starts a graceful shutdown of every Stores of the process (the generated main calls it
// on SIGINT/SIGTERM, before the requests in flight are let finish): no silo is opened any
// more, and a write to an open one first re-checks its schema row (route), so an old process
// draining while a new build migrates a silo never writes into the migrated database.
func Drain() {
	liveMu.Lock()
	defer liveMu.Unlock()
	for _, s := range live {
		s.draining.Store(true)
	}
}

// openHook, set by a test, runs as a silo starts opening (a slow migration).
var openHook func(key, value string)

var (
	liveMu sync.Mutex
	live   []*Stores // every Stores OpenStores made, for Drain
)

// recheckWrite is the write-time re-check of a draining process (M3): the silo's schema row,
// read inside the Task's transaction, must still be this binary's; otherwise another build
// migrated it meanwhile and the silo answers the maintenance page from now on.
func (s *Stores) recheckWrite(ctx context.Context, st Store, tx Tx, key, value string) error {
	sq, ok := st.(*SQLite)
	if !ok || !s.draining.Load() {
		return nil
	}
	q, err := sq.on(tx)
	if err != nil {
		return err
	}
	hash, _, _, err := schemaRow(ctx, q)
	if err != nil || hash == s.hash {
		return err
	}
	detail := "another build migrated it while this process drains: the new build serves it"
	s.mu.Lock()
	if d := s.open[key+"/"+value]; d != nil && isReady(d) {
		d.state, d.detail, d.checked = SiloAhead, detail, time.Now()
	}
	s.mu.Unlock()
	return &MaintenanceError{Key: key, Value: value, State: SiloAhead, Detail: detail}
}

func (d *siloDB) close() {
	if d.store != nil {
		d.store.Close()
	}
	if d.reader != nil {
		d.reader.Close()
	}
}

// openSilo opens silo ss's value and checks it (see the package comment's silo section).
func (s *Stores) openSilo(ctx context.Context, ss *siloSchema, value string, allowDataLoss bool) *siloDB {
	if openHook != nil {
		openHook(ss.key, value)
	}
	d := &siloDB{key: ss.key, value: value, checked: time.Now()}
	fail := func(state SiloState, detail string) *siloDB {
		d.close()
		d.store, d.reader, d.state, d.detail = nil, nil, state, detail
		s.logf("warn", fmt.Sprintf("silo %s/%s: %s: %s", ss.key, value, state, detail))
		return d
	}
	if s.remote != nil {
		return s.openChild(ctx, d, ss, value, fail)
	}
	path := ":memory:"
	if s.cfg.Dir != "" {
		p, err := SiloPath(s.cfg.Dir, ss.key, value)
		if err != nil {
			return fail(SiloFailed, err.Error())
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return fail(SiloFailed, err.Error())
		}
		path = p
	}
	st, err := OpenSQLite(path)
	if err != nil {
		return fail(SiloFailed, err.Error())
	}
	d.store = st
	state, detail, err := s.check(ctx, st, ss, allowDataLoss, true)
	if err != nil {
		return fail(SiloFailed, err.Error())
	}
	if state != SiloOK {
		return fail(state, detail)
	}
	if detail != "" {
		s.logf("info", fmt.Sprintf("silo %s/%s: %s", ss.key, value, detail))
	}
	st.register(ss.descs)
	if path != ":memory:" {
		r, err := OpenSQLiteReader(path)
		if err != nil {
			return fail(SiloFailed, err.Error())
		}
		r.register(ss.descs)
		d.reader = r
	}
	d.state = SiloOK
	return d
}

func (s *Stores) logf(level, msg string) {
	if s.cfg.Log != nil {
		s.cfg.Log(level, msg)
	}
}

const createSchemaTable = `CREATE TABLE IF NOT EXISTS "_alder_schema" ("key" TEXT NOT NULL PRIMARY KEY, "hash" TEXT NOT NULL, "migration" TEXT NOT NULL, "at" TEXT NOT NULL)`

// schemaRow is the silo database's `_alder_schema` row: the schema hash and last migration of
// the binary that last created or migrated it; ok is false when there is none.
func schemaRow(ctx context.Context, q querier) (hash, migration string, ok bool, err error) {
	exists, err := tableExists(ctx, q, "_alder_schema")
	if err != nil || !exists {
		return "", "", false, err
	}
	err = q.QueryRowContext(ctx, `SELECT "hash", "migration" FROM "_alder_schema" WHERE "key" = 'schema'`).Scan(&hash, &migration)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", false, nil
	}
	return hash, migration, err == nil, err
}

func putSchemaRow(ctx context.Context, q querier, hash, migration string) error {
	if _, err := q.ExecContext(ctx, createSchemaTable); err != nil {
		return err
	}
	_, err := q.ExecContext(ctx, `INSERT INTO "_alder_schema" ("key", "hash", "migration", "at") VALUES ('schema', ?, ?, ?) ON CONFLICT ("key") DO UPDATE SET "hash" = excluded."hash", "migration" = excluded."migration", "at" = excluded."at"`,
		hash, migration, EncodeTime(time.Now()))
	return err
}

// plan is what a silo database needs: its state, the migrations it is behind by (to apply),
// whether it is new (no row, no tables: create it), and why it is not servable.
type plan struct {
	state   SiloState
	detail  string
	fresh   bool
	pending []Migration
	drops   []string
	rename  bool // ok, but the row names an older migration than this build's last
}

// inspect reads silo ss's database (q) and plans it against this binary, writing nothing.
func (s *Stores) inspect(ctx context.Context, q querier, ss *siloSchema, allowDataLoss bool) (plan, error) {
	hash, mig, ok, err := schemaRow(ctx, q)
	if err != nil {
		return plan{}, err
	}
	if !ok {
		existing, err := appTables(ctx, q)
		if err != nil {
			return plan{}, err
		}
		if len(existing) == 0 {
			return plan{state: SiloMigrating, fresh: true}, nil
		}
		return plan{state: SiloFailed, detail: "the database has tables but no _alder_schema row: it was not made by this app's runtime; restore it from a backup"}, nil
	}
	if hash == s.hash {
		// the row names the migration that made it so; a later one that changed none of the
		// tables leaves it so, and the row is brought to this build's last (check)
		return plan{state: SiloOK, rename: s.last != "" && mig != s.last}, nil
	}
	ms := s.cfg.Migrations
	if len(ms) == 0 {
		if s.cfg.Dev {
			return plan{state: SiloMigrating, fresh: true, detail: "recreated (dev mode): the app's schema changed"}, nil
		}
		return plan{state: SiloFailed, detail: fmt.Sprintf("its schema (%.12s) is not the app's (%.12s) and the app has no migrations: run `alder migrate`", hash, s.hash)}, nil
	}
	at := -1
	for i, m := range ms {
		if m.Name == mig || mig == "" && siloHash(m.To, s.cfg.Silos) == hash { // "": made before the app had migrations (M4)
			at = i
		}
	}
	switch {
	case mig != "" && at < 0 && mig > ms[len(ms)-1].Name:
		return plan{state: SiloAhead, detail: fmt.Sprintf("a newer build migrated it (to %s, which this build does not have): deploy that build again, or restore the silo from a backup taken before it (`alder restore <env> --silo %s:<id> --to <time>`)", mig, ss.key)}, nil
	case at < 0:
		return plan{state: SiloFailed, detail: fmt.Sprintf("it was migrated to %q, which is not among this build's migrations: restore the file from version control", mig)}, nil
	case at == len(ms)-1:
		return plan{state: SiloFailed, detail: fmt.Sprintf("it is at the last migration (%s) but its schema hash %.12s is not the app's %.12s: the tables, or the tables of the silos, changed without `alder migrate` (a table moved into a silo by hand: `alder new silo --empty` writes the migration of the move)", mig, hash, s.hash)}, nil
	}
	p := plan{state: SiloMigrating, pending: ms[at+1:]}
	for _, m := range p.pending {
		for _, line := range m.DropsData {
			if ss.mentions(line) {
				p.drops = append(p.drops, m.Name+": "+line)
			}
		}
	}
	if len(p.drops) > 0 && !allowDataLoss && !s.cfg.Dev && os.Getenv(EnvAllowDataLoss) != "1" {
		p.state = SiloDataLoss
		p.detail = "a pending migration DROPS DATA (" + strings.Join(p.drops, "; ") + "): take a backup and run `alder migrate silos <env> --allow-data-loss`"
	}
	return p, nil
}

// mentions reports whether a DROPS DATA line ("column Project.x", "table Project", "Project.n:
// …") names a table of the silo.
func (ss *siloSchema) mentions(line string) bool {
	if strings.HasPrefix(line, "shared rows of ") { // a move into a silo: the shared database's rows
		return false
	}
	for _, n := range ss.short {
		if strings.Contains(line, "table "+n) || strings.Contains(line, n+".") {
			return true
		}
	}
	return false
}

// check brings silo database st to this binary's schema when it can (apply false: only
// inspects): it returns the state and, for a silo not served or one it migrated, a detail.
func (s *Stores) check(ctx context.Context, st *SQLite, ss *siloSchema, allowDataLoss, apply bool) (SiloState, string, error) {
	if !apply {
		p, err := s.inspect(ctx, st.db, ss, allowDataLoss)
		return p.state, p.detail, err
	}
	conn, err := st.db.Conn(ctx)
	if err != nil {
		return "", "", lockedErr(err)
	}
	defer conn.Close()
	restore, err := foreignKeysOff(ctx, conn)
	if err != nil {
		return "", "", lockedErr(err)
	}
	defer restore()
	tx, err := conn.BeginTx(ctx, nil) // BEGIN IMMEDIATE on a file: another process opening it waits
	if err != nil {
		return "", "", lockedErr(err)
	}
	defer tx.Rollback()
	p, err := s.inspect(ctx, tx, ss, allowDataLoss)
	if err != nil {
		return "", "", err
	}
	switch {
	case p.state == SiloOK && !p.rename:
		return SiloOK, "", nil
	case p.state == SiloOK: // the tables are this build's: only the row's migration changes
	case p.state != SiloMigrating:
		return p.state, p.detail, nil
	case p.fresh:
		existing, err := appTables(ctx, tx)
		if err != nil {
			return "", "", err
		}
		for _, n := range existing {
			if _, err := tx.ExecContext(ctx, "DROP TABLE "+quote(n)); err != nil {
				return "", "", err
			}
		}
		for _, stmt := range append(slices.Clone(ss.stmts), ss.indexes...) {
			if _, err := tx.ExecContext(ctx, stmt); err != nil {
				return "", "", fmt.Errorf("db: %s: %w", stmt, err)
			}
		}
	default:
		for _, m := range p.pending {
			for _, stmt := range ss.statements(m) {
				if _, err := tx.ExecContext(ctx, stmt); err != nil {
					return SiloFailed, fmt.Sprintf("migration %s failed: %v", m.Name, err), nil
				}
			}
		}
		if child, err := foreignKeyViolation(ctx, tx); err != nil || child != "" {
			if err == nil {
				err = fmt.Errorf("rows in %s refer to rows the migrations delete", child)
			}
			return SiloFailed, "migrating it failed: " + err.Error(), nil
		}
		if _, live, err := liveSchema(ctx, tx); err != nil {
			return "", "", err
		} else if live != ss.hash {
			return SiloFailed, fmt.Sprintf("after %s its tables are not the silo's (%.12s, want %.12s)", p.pending[len(p.pending)-1].Name, live, ss.hash), nil
		}
	}
	if err := putSchemaRow(ctx, tx, s.hash, s.last); err != nil {
		return "", "", err
	}
	if err := tx.Commit(); err != nil {
		return "", "", lockedErr(err)
	}
	detail := p.detail
	if len(p.pending) > 0 {
		var names []string
		for _, m := range p.pending {
			names = append(names, m.Name)
		}
		detail = "migrated " + strings.Join(names, ", ")
	}
	return SiloOK, detail, nil
}

var (
	stmtTarget = regexp.MustCompile(`^(?:CREATE TABLE(?: IF NOT EXISTS)?|ALTER TABLE|DROP TABLE(?: IF EXISTS)?|INSERT INTO|UPDATE|DELETE FROM)\s+"((?:[^"]|"")*)"`)
	indexOn    = regexp.MustCompile(`^CREATE (?:UNIQUE )?INDEX .* ON "((?:[^"]|"")*)"`)
	dropIndex  = regexp.MustCompile(`^DROP INDEX(?: IF EXISTS)? "((?:[^"]|"")*)__[^"_]+"`)
	references = regexp.MustCompile(` REFERENCES "((?:[^"]|"")*)"\("(?:[^"]|"")*"\)(?: ON DELETE (?:CASCADE|SET NULL))?`)
)

// statements are migration m's statements on the silo's tables, as the silo holds them: a
// statement on another table is left out, and a REFERENCES to a table outside the silo removed.
func (ss *siloSchema) statements(m Migration) []string {
	var out []string
	for _, st := range m.statements() {
		var target string
		if x := stmtTarget.FindStringSubmatch(st); x != nil {
			target = x[1]
		} else if x := indexOn.FindStringSubmatch(st); x != nil {
			target = x[1]
		} else if x := dropIndex.FindStringSubmatch(st); x != nil {
			target = x[1]
		}
		target = strings.TrimSuffix(strings.ReplaceAll(target, `""`, `"`), "__alder__new")
		if !ss.sqlName[target] {
			continue
		}
		st = references.ReplaceAllStringFunc(st, func(ref string) string {
			if ss.sqlName[references.FindStringSubmatch(ref)[1]] {
				return ref
			}
			return ""
		})
		out = append(out, st)
	}
	return out
}

// ---- routing

// route is the store and transaction a step op on table t runs on in the running effect, when
// the app has silos (eff.Env.Stores); otherwise st and tx as given. vals are an insert's
// column values (nil for any other step): a row names its own silo.
//
// A shared table: st and tx when the Task's store is the shared one (or in a test); otherwise a
// read-only connection to the shared store outside the transaction (a write is a bug: the
// checker's one-store rule, E0031). A siloed table: the silo of its key's value — the row's,
// else the request's (Env.Keys), else the session's — which is st and tx when it is the
// Task's own store; a read of another silo goes through a read-only connection outside the
// transaction (in a Query, the store itself), a write to another value of the Task's key fails
// ("the row's org is not this request's"). In a test every store has its own transaction for
// the whole test (TestStore). A silo not served is its *MaintenanceError.
func route[T any](ctx context.Context, st Store, tx Tx, t *Table[T], op string, write bool, vals []any) (Store, Tx, error) {
	e := eff.EnvFrom(ctx)
	if e == nil || t == nil {
		return st, tx, nil
	}
	reg, _ := e.Stores.(*Stores)
	if reg == nil {
		return st, tx, nil
	}
	name := op + " " + shortName(t.Name)
	if t.Silo == "" {
		if reg.test && write && e.Pending && e.SiloKey == "" { // Test.run's first write binds it
			e.Pending, e.Bound = false, true
		}
		if reg.test && write && (e.Bound || e.Pending) && e.SiloKey != "" { // a test fails where production does
			return nil, nil, fmt.Errorf("db: %w: %s writes the shared store in a Task on silo %s (one store per Task)", eff.ErrInternal, name, e.SiloKey)
		}
		if reg.test || e.SiloKey == "" {
			return st, tx, nil
		}
		if write {
			return nil, nil, fmt.Errorf("db: %w: %s writes the shared store in a Task on silo %s (one store per Task)", eff.ErrInternal, name, e.SiloKey)
		}
		if e.Kind == eff.KQuery || reg.reader == nil {
			return reg.shared, nil, nil
		}
		return readOnly{reg.reader, errReaderWrite}, nil, nil
	}
	v, err := siloValue(e, t, name, vals)
	if err != nil {
		return nil, nil, err
	}
	if reg.test {
		if write {
			if err := testWrite(e, t, name, v); err != nil {
				return nil, nil, err
			}
		}
		return reg.testSilo(ctx, t.Silo, v)
	}
	if e.SiloKey == t.Silo && e.SiloValue == v {
		if write {
			if err := reg.recheckWrite(ctx, st, tx, t.Silo, v); err != nil {
				return nil, nil, err
			}
		}
		return st, tx, nil
	}
	if write {
		if e.SiloKey == t.Silo {
			return nil, nil, &eff.ErrFail{Msg: name + ": the row's " + t.Silo + " is not this request's"}
		}
		return nil, nil, fmt.Errorf("db: %w: %s writes silo %s in a Task on the %s store (one store per Task)", eff.ErrInternal, name, t.Silo, storeName(e))
	}
	d, err := reg.silo(ctx, t.Silo, v)
	if err != nil {
		return nil, nil, err
	}
	if e.Kind == eff.KQuery || d.reader == nil {
		return d.store, nil, nil
	}
	return readOnly{d.reader, errReaderWrite}, nil, nil
}

// testWrite is, in a test, the check production makes of a write of value v of t's silo key
// (I3): from a request's Task (bound by Enter) the one-store rule and the row's own value; from
// the test's body, a row whose value is not the session's (when it has one) is refused as a
// request would refuse it ("the row's org is not this request's"). Test.run's Task (Pending) is
// bound by its first write (route binds it on a write of the shared store), then checked as a
// request's is.
func testWrite[T any](e *eff.Env, t *Table[T], name, v string) error {
	if e.Pending && (e.SiloKey == "" || e.SiloKey == t.Silo) {
		// Test.run's first write binds it to this silo's value v, once v passes the check a
		// test's body makes (below): the request's value, else the session's
		if err := testWrite(&eff.Env{Keys: e.Keys, Session: e.Session}, t, name, v); err != nil {
			return err
		}
		e.Pending, e.Bound, e.SiloKey, e.SiloValue = false, true, t.Silo, v
		return nil
	}
	if e.Bound || e.Pending { // Pending here: bound to silo SiloKey, written another
		switch {
		case e.SiloKey != t.Silo:
			return fmt.Errorf("db: %w: %s writes silo %s in a Task on the %s store (one store per Task)", eff.ErrInternal, name, t.Silo, storeName(e))
		case e.SiloValue != v:
			return &eff.ErrFail{Msg: name + ": the row's " + t.Silo + " is not this request's"}
		}
		return nil
	}
	cur, ok := e.Keys[t.Silo]
	if !ok && t.User != nil {
		cur, ok = t.User(e.Session)
	}
	if ok && cur != v {
		return &eff.ErrFail{Msg: name + ": the row's " + t.Silo + " is not this request's"}
	}
	return nil
}

func storeName(e *eff.Env) string {
	if e.SiloKey == "" {
		return "shared"
	}
	return "silo " + e.SiloKey
}

// siloValue is the value of t's silo key a step on it (named name) uses: the row's (an
// insert), else Env.Keys', else the session's.
func siloValue[T any](e *eff.Env, t *Table[T], name string, vals []any) (string, error) {
	if vals != nil {
		for i, c := range t.Columns {
			if c.Name == t.Owner && i < len(vals) {
				if v, ok := vals[i].(string); ok && v != "" {
					return v, nil
				}
			}
		}
	}
	if v, ok := e.Keys[t.Silo]; ok {
		return v, nil
	}
	if t.User != nil {
		if v, ok := t.User(e.Session); ok {
			return v, nil
		}
	}
	return "", &eff.ErrFail{Msg: name + ": `" + shortName(t.Name) + "` lives in `silo " + t.Silo + "` and the session has no " + t.Silo}
}

// Enter binds the Task about to run on e to the store of silo key (the arm's store, which the
// checker computed; "" for the shared one): the value is the request's (Env.Keys, set by its
// page). eff.Run then begins the Task's transaction there. A silo not served is its
// *MaintenanceError (the route answers the maintenance page). Without silos, or in a test, it
// changes nothing.
func Enter(e *eff.Env, key string) error {
	reg, _ := e.Stores.(*Stores)
	if reg == nil {
		return nil
	}
	e.Bound = true
	if key == "" {
		return nil
	}
	v, ok := e.Keys[key]
	if !ok {
		return fmt.Errorf("db: %w: a Task on silo %s in a request not narrowed to one", eff.ErrInternal, key)
	}
	d, err := reg.silo(e.Ctx, key, v)
	if err != nil {
		return err
	}
	if reg.test { // the test's stores stay (TestStore); the binding is checked on each write
		e.SiloKey, e.SiloValue = key, v
		return nil
	}
	e.Store, e.SiloKey, e.SiloValue = d.store, key, v
	return nil
}

// Narrow records that the request is narrowed to value of key (a page's `requires key`, or the
// admin form's `silo key` parameter): the scoped operations and the routing read it before the
// session.
func Narrow(e *eff.Env, key, value string) {
	if e.Keys == nil {
		e.Keys = map[string]string{}
	}
	e.Keys[key] = value
}

// Member reports whether membership table t has a row whose column userCol is user and keyCol
// is key: `via`'s check (Session.put of a `via` field), inside the running Task's transaction
// on the shared store (the membership table is shared: it is not partitioned).
func Member[T any](ctx context.Context, st Store, tx Tx, t *Table[T], userCol, user, keyCol, key string) (bool, error) {
	st, tx, err := route(ctx, st, tx, t, "Session.put", false, nil)
	if err != nil {
		return false, err
	}
	n, err := CountRows(ctx, st, tx, Where(CEq(keyCol, key), Where(CEq(userCol, user), From(t))))
	return n > 0, err
}

// ---- tests

// TestStore is the Env.Store of a test of an app with silos: the shared store, whose savepoints
// (Test.run, and each request of the test's browser) are taken on every silo's transaction too,
// and on a silo's begun later, so a failed step undoes its writes in every store.
type TestStore struct {
	*SQLite
	Stores *Stores
}

// Savepoint opens savepoint name on tx (the shared store's) and on every silo's transaction.
func (t TestStore) Savepoint(ctx context.Context, tx eff.Tx, name string) error {
	if err := t.SQLite.Savepoint(ctx, tx, name); err != nil {
		return err
	}
	s := t.Stores
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, d := range s.open {
		if d.tx != nil {
			if err := d.store.Savepoint(ctx, d.tx, name); err != nil {
				return err
			}
		}
	}
	s.sps = append(s.sps, name)
	return nil
}

// Release releases savepoint name everywhere.
func (t TestStore) Release(ctx context.Context, tx eff.Tx, name string) error {
	return t.each(ctx, tx, name, true, (*SQLite).Release)
}

// RollbackTo rolls every store back to savepoint name.
func (t TestStore) RollbackTo(ctx context.Context, tx eff.Tx, name string) error {
	return t.each(ctx, tx, name, false, (*SQLite).RollbackTo)
}

func (t TestStore) each(ctx context.Context, tx eff.Tx, name string, release bool, f func(*SQLite, context.Context, eff.Tx, string) error) error {
	if err := f(t.SQLite, ctx, tx, name); err != nil {
		return err
	}
	s := t.Stores
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, d := range s.open {
		if d.tx != nil {
			if err := f(d.store, ctx, d.tx, name); err != nil {
				return err
			}
		}
	}
	if release {
		if i := slices.Index(s.sps, name); i >= 0 {
			s.sps = s.sps[:i]
		}
	}
	return nil
}

// testSilo is silo key/value of a test, its transaction begun (with the savepoints open
// elsewhere) on first use.
func (s *Stores) testSilo(ctx context.Context, key, value string) (Store, Tx, error) {
	d, err := s.silo(ctx, key, value)
	if err != nil {
		return nil, nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if d.tx == nil {
		tx, err := d.store.Begin(context.WithoutCancel(ctx))
		if err != nil {
			return nil, nil, err
		}
		for _, sp := range s.sps {
			if err := d.store.Savepoint(ctx, tx, sp); err != nil {
				tx.Rollback()
				return nil, nil, err
			}
		}
		d.tx = tx
	}
	return d.store, d.tx, nil
}

// TestSilo is the store of silo key/value in a test of an app with silos, for the test's own
// checks (rt/testing): nil when the test never touched it.
func (s *Stores) TestSilo(key, value string) *SQLite {
	s.mu.Lock()
	defer s.mu.Unlock()
	if d := s.open[key+"/"+value]; d != nil {
		return d.store
	}
	return nil
}

// ---- the binary's maintenance tasks: --migrate-silos and --silo-status

// SiloFile is one silo database found under a directory.
type SiloFile struct {
	Key, Value, Path string
}

// SiloFiles lists the silo databases under dir (dir/silos/<key>/<value>.db) of the keys of
// silos, sorted.
func SiloFiles(dir string, silos []Silo) ([]SiloFile, error) {
	var out []SiloFile
	for _, si := range silos {
		paths, err := filepath.Glob(filepath.Join(dir, "silos", si.Key, "*.db"))
		if err != nil {
			return nil, err
		}
		slices.Sort(paths)
		for _, p := range paths {
			if v, ok := SiloValue(filepath.Base(p)); ok { // another file is none of the app's
				out = append(out, SiloFile{Key: si.Key, Value: v, Path: p})
			}
		}
	}
	return out, nil
}

// SiloStatus is one line of --silo-status: a silo file and its state.
type SiloStatus struct {
	Key    string    `json:"key"`
	ID     string    `json:"id"`
	State  SiloState `json:"state"`
	Detail string    `json:"detail,omitempty"`
}

// StatusSilos writes one JSON line (SiloStatus) per silo database — each file under cfg.Dir,
// or on Turso each child database — inspecting each without changing it: ok, migrating
// (behind: it migrates when next opened, or by `alder migrate silos`), pending-data-loss,
// ahead, failed, or orphaned (its key row is gone from the shared database, cfg.Shared).
func StatusSilos(ctx context.Context, cfg StoresConfig, out io.Writer) error {
	s, err := newStores(nil, cfg)
	if err != nil {
		return err
	}
	es, err := s.entries(ctx)
	if err != nil {
		return err
	}
	orphaned, done, err := s.orphans(ctx)
	if err != nil {
		return err
	}
	defer done()
	enc := json.NewEncoder(out)
	enc.SetEscapeHTML(false)
	for _, e := range es {
		line := SiloStatus{Key: e.key, ID: e.value}
		if orphaned != nil && orphaned(e.key, e.value) {
			line.State, line.Detail = SiloOrphaned, orphanDetail(s, e.key)
		} else {
			st, err := s.openEntry(ctx, e, true)
			if err == nil {
				line.State, line.Detail, err = s.state(ctx, st, e, false, false)
				st.Close()
			}
			if err != nil {
				line.State, line.Detail = SiloFailed, err.Error()
			}
		}
		if err := enc.Encode(line); err != nil {
			return err
		}
	}
	return nil
}

// MigrateSilos opens and checks every silo file under cfg.Dir (cfg.AllowDataLoss: also
// applying a migration that drops data), writing one line each: "org/<id>: ok", "org/<id>:
// migrated 0003_x", or "org/<id>: pending-data-loss: …". It returns how many are not served.
// On Turso the silos are the databases the platform API lists for each key (ChildName).
func MigrateSilos(ctx context.Context, cfg StoresConfig, out io.Writer) (int, error) {
	s, err := newStores(nil, cfg)
	if err != nil {
		return 0, err
	}
	es, err := s.entries(ctx)
	if err != nil {
		return 0, err
	}
	bad := 0
	for _, e := range es {
		state, detail := SiloFailed, ""
		st, err := s.openEntry(ctx, e, false)
		if err == nil {
			state, detail, err = s.state(ctx, st, e, cfg.AllowDataLoss, true)
			st.Close()
		}
		if err != nil {
			state, detail = SiloFailed, err.Error()
		}
		if state != SiloOK {
			bad++
		}
		fmt.Fprintln(out, stateLine(e.key+"/"+e.value, state, detail))
	}
	return bad, nil
}
