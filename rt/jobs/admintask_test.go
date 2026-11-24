package jobs_test

import (
	"bytes"
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"alder/rt/db"
	"alder/rt/jobs"
)

// The app binary's --jobs task (jobs.Admin, spec §4): list, retry <id>, retry --dead and run
// <Kind> over the shared database file and the silo files under its directory, as JSON lines.

// adminRow is a row put in a database file by putJobs.
type adminRow struct {
	id, kind, state, key string
	schedule             bool
}

// adminT0 is the rows' run_at and created.
var adminT0 = time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)

// putJobs makes the database file path (and its directory) with a jobs table holding rows, by
// the statements cmd/alder/jobs_test.go's jobsDB uses.
func putJobs(t *testing.T, path string, rows ...adminRow) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	st, err := db.OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	q, err := db.On(st, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := jobs.EnsureTable(ctx, q); err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		var finished, errText, key any
		if r.state == "dead" || r.state == "done" {
			finished, errText = adminT0.Add(time.Minute).UnixMilli(), "boom"
		}
		if r.key != "" {
			key = r.key
		}
		kind := r.kind
		if kind == "" {
			kind = "K.J"
		}
		if _, err := q.ExecContext(ctx, `INSERT INTO "_alder_jobs" ("id", "kind", "params", "state", "run_at", "created", "attempts", "error", "finished", "schedule", "scheduled_for", "key", "priority") VALUES (?, ?, '{}', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			r.id, kind, r.state, adminT0.UnixMilli(), adminT0.UnixMilli(), 3, errText, finished, r.schedule, nil, key, 0); err != nil {
			t.Fatal(err)
		}
	}
}

// adminGet is row id of the database file path, nil when there is none.
func adminGet(t *testing.T, path, id string) *jobs.Row {
	t.Helper()
	st, err := db.OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	q, _ := db.On(st, nil)
	r, err := jobs.Get(context.Background(), q, id)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// adminHints is the _alder_jobs_silos rows of the database file path, as "key/value".
func adminHints(t *testing.T, path string) []string {
	t.Helper()
	st, err := db.OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	q, _ := db.On(st, nil)
	rows, err := q.QueryContext(context.Background(), `SELECT "key", "value" FROM "_alder_jobs_silos" ORDER BY "key", "value"`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			t.Fatal(err)
		}
		out = append(out, k+"/"+v)
	}
	return out
}

// adminSilo is the path of silo org/value under dir.
func adminSilo(t *testing.T, dir, value string) string {
	t.Helper()
	p, err := db.SiloPath(dir, "org", value)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// adminApp is the config of an app with `silo org` whose shared file is dir/app.db.
func adminApp(dir string) db.StoresConfig {
	return db.StoresConfig{Dir: dir, Shared: filepath.Join(dir, "app.db"), Silos: []db.Silo{{Key: "org"}}}
}

// runAdmin runs Admin with args: its exit code, its report lines and other stdout lines, and
// its stderr.
func runAdmin(t *testing.T, cfg db.StoresConfig, kinds []jobs.Kind, args ...string) (int, []jobs.AdminLine, []string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := jobs.Admin(context.Background(), cfg, kinds, args, &out, &errOut)
	lines, msgs := jobs.DecodeAdmin(out.String())
	return code, lines, msgs, errOut.String()
}

func rowIDs(rows []jobs.Row) []string {
	var out []string
	for _, r := range rows {
		out = append(out, r.ID)
	}
	return out
}

func TestAdmin_List(t *testing.T) {
	dir := t.TempDir()
	cfg := adminApp(dir)
	putJobs(t, cfg.Shared,
		adminRow{id: "tick0001", state: "queued", schedule: true},
		adminRow{id: "run00001", state: "running"},
		adminRow{id: "dead0001", state: "dead"},
		adminRow{id: "dead0002", state: "dead"})
	putJobs(t, adminSilo(t, dir, "A"), adminRow{id: "deadA001", state: "dead"})
	b := adminSilo(t, dir, "B")
	if err := os.MkdirAll(filepath.Dir(b), 0o755); err != nil {
		t.Fatal(err)
	}
	st, err := db.OpenSQLite(b)
	if err != nil {
		t.Fatal(err)
	}
	if q, _ := db.On(st, nil); q != nil {
		if _, err := q.ExecContext(context.Background(), `CREATE TABLE t (x INTEGER)`); err != nil {
			t.Fatal(err)
		}
	}
	st.Close()
	c := adminSilo(t, dir, "C")
	if err := os.MkdirAll(c, 0o755); err != nil { // a directory where the file should be
		t.Fatal(err)
	}

	code, lines, msgs, stderr := runAdmin(t, cfg, nil, "list")
	if code != 2 || len(msgs) != 0 {
		t.Fatalf("exit %d, messages %q, stderr %q; want 2", code, msgs, stderr)
	}
	if len(lines) != 4 {
		t.Fatalf("%d lines, want 4: %+v", len(lines), lines)
	}
	sh := lines[0]
	if sh.Silo != "" || sh.Error != "" || sh.Counts[jobs.Queued] != 1 || sh.Counts[jobs.Running] != 1 || sh.Counts[jobs.Dead] != 2 {
		t.Errorf("shared: %+v", sh)
	}
	if !reflect.DeepEqual(rowIDs(sh.Ticks), []string{"tick0001"}) || len(rowIDs(sh.Dead)) != 2 {
		t.Errorf("shared ticks %v, dead %v", rowIDs(sh.Ticks), rowIDs(sh.Dead))
	}
	if a := lines[1]; a.Silo != "org/A" || a.Counts[jobs.Dead] != 1 || !reflect.DeepEqual(rowIDs(a.Dead), []string{"deadA001"}) {
		t.Errorf("org/A: %+v", a)
	}
	if b := lines[2]; b.Silo != "org/B" || b.Error != "" || b.Counts == nil || b.Counts[jobs.Dead] != 0 || len(b.Dead) != 0 {
		t.Errorf("org/B: %+v", b)
	}
	if c := lines[3]; c.Silo != "org/C" || c.Error == "" {
		t.Errorf("org/C: %+v", c)
	}

	if err := os.RemoveAll(c); err != nil {
		t.Fatal(err)
	}
	if code, lines, _, stderr := runAdmin(t, cfg, nil, "list"); code != 0 || len(lines) != 3 {
		t.Errorf("without org/C: exit %d, %d lines, stderr %q; want 0, 3", code, len(lines), stderr)
	}
}

func TestAdmin_RetryID(t *testing.T) {
	dir := t.TempDir()
	cfg := adminApp(dir)
	putJobs(t, cfg.Shared, adminRow{id: "deadS001", state: "dead"}, adminRow{id: "queue001", state: "queued"})
	a := adminSilo(t, dir, "A")
	putJobs(t, a, adminRow{id: "deadA001", state: "dead"})

	code, lines, _, stderr := runAdmin(t, cfg, nil, "retry", "deadS001")
	if code != 0 || len(lines) != 1 || lines[0].Silo != "" || lines[0].Row == nil || lines[0].Row.State != "dead" {
		t.Fatalf("retry in shared: exit %d, %+v, stderr %q", code, lines, stderr)
	}
	if h := adminHints(t, cfg.Shared); len(h) != 0 {
		t.Errorf("hints after a shared retry: %v, want none", h)
	}

	code, lines, _, stderr = runAdmin(t, cfg, nil, "retry", "deadA001")
	if code != 0 || len(lines) != 1 || lines[0].Silo != "org/A" || lines[0].Row == nil || lines[0].Row.State != "dead" || lines[0].Row.ID != "deadA001" {
		t.Fatalf("retry in org/A: exit %d, %+v, stderr %q", code, lines, stderr)
	}
	if r := adminGet(t, a, "deadA001"); r.State != "queued" || r.Attempts != 0 {
		t.Errorf("org/A's row: %+v", r)
	}
	if h := adminHints(t, cfg.Shared); !reflect.DeepEqual(h, []string{"org/A"}) {
		t.Errorf("hints: %v, want [org/A]", h)
	}

	code, lines, _, _ = runAdmin(t, cfg, nil, "retry", "x")
	if code != 1 || len(lines) != 1 || !reflect.DeepEqual(lines[0].Refused, []string{"no such job x"}) {
		t.Errorf("unknown id: exit %d, %+v", code, lines)
	}
	code, lines, _, _ = runAdmin(t, cfg, nil, "retry", "queue001")
	if code != 1 || len(lines) != 1 || !reflect.DeepEqual(lines[0].Refused, []string{"job queue001 is queued: only a dead or cancelled job is retried"}) {
		t.Errorf("queued row: exit %d, %+v", code, lines)
	}
}

func TestAdmin_RetryOrphaned(t *testing.T) {
	dir := t.TempDir()
	cfg := adminApp(dir)
	cfg.Silos[0].Ref = "Org.Org"
	cfg.Tables = []db.TableDesc{orgDesc}
	putJobs(t, cfg.Shared)
	st, err := db.OpenSQLite(cfg.Shared)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Ensure(context.Background(), []db.TableDesc{orgDesc}, false); err != nil {
		t.Fatal(err)
	}
	st.Close()
	b := adminSilo(t, dir, "B")
	putJobs(t, b, adminRow{id: "deadB001", state: "dead"})

	code, lines, _, stderr := runAdmin(t, cfg, nil, "retry", "deadB001")
	want := []string{"silo org/B is orphaned: nothing runs there (alder silos prune)"}
	if code != 1 || len(lines) != 1 || !reflect.DeepEqual(lines[0].Refused, want) {
		t.Fatalf("exit %d, %+v, stderr %q", code, lines, stderr)
	}
	if r := adminGet(t, b, "deadB001"); r.State != "dead" {
		t.Errorf("the row: %+v, want still dead", r)
	}
}

func TestAdmin_RetryDead(t *testing.T) {
	dir := t.TempDir()
	cfg := adminApp(dir)
	putJobs(t, cfg.Shared,
		adminRow{id: "deadS001", state: "dead", key: "k"},
		adminRow{id: "live0001", state: "queued", key: "k"},
		adminRow{id: "deadS002", state: "dead"})
	putJobs(t, adminSilo(t, dir, "A"), adminRow{id: "deadA001", state: "dead"})

	code, lines, _, stderr := runAdmin(t, cfg, nil, "retry", "--dead")
	if code != 1 {
		t.Fatalf("exit %d, stderr %q; want 1", code, stderr)
	}
	var rows, refused []string
	for _, l := range lines {
		if l.Row != nil {
			rows = append(rows, l.Silo+":"+l.Row.ID)
		}
		refused = append(refused, l.Refused...)
	}
	if !reflect.DeepEqual(rows, []string{":deadS002", "org/A:deadA001"}) {
		t.Errorf("retried: %v", rows)
	}
	if !reflect.DeepEqual(refused, []string{`job deadS001 is not retried: job live0001 of K.J holds its key "k" (queued or running)`}) {
		t.Errorf("refused: %q", refused)
	}
	if last := lines[len(lines)-1]; len(last.Refused) == 0 {
		t.Errorf("the refused line is not last: %+v", lines)
	}
	if h := adminHints(t, cfg.Shared); !reflect.DeepEqual(h, []string{"org/A"}) {
		t.Errorf("hints: %v, want [org/A]", h)
	}
}

func TestAdmin_Run(t *testing.T) {
	dir := t.TempDir()
	cfg := adminApp(dir)
	kinds := []jobs.Kind{{Name: "Reports.Ping", Priority: 3}, {Name: "Reports.Digest", Params: []string{"org"}}}

	code, lines, _, stderr := runAdmin(t, cfg, kinds, "run", "Ping")
	if code != 0 || len(lines) != 1 || lines[0].Kind != "Reports.Ping" || lines[0].ID == "" {
		t.Fatalf("run Ping: exit %d, %+v, stderr %q", code, lines, stderr)
	}
	r := adminGet(t, cfg.Shared, lines[0].ID)
	if r == nil || r.State != "queued" || r.Params != "{}" || r.Schedule || r.Kind != "Reports.Ping" || r.Priority != 3 {
		t.Errorf("the queued row: %+v", r)
	}

	code, _, _, stderr = runAdmin(t, cfg, kinds, "run", "Digest")
	if code != 2 || !strings.Contains(stderr, "Reports.Digest takes parameters (org)") {
		t.Errorf("run Digest: exit %d, stderr %q", code, stderr)
	}
	if code, _, _, stderr := runAdmin(t, cfg, kinds, "run", "Nope"); code != 2 || !strings.Contains(stderr, "the app has no job Nope") {
		t.Errorf("run Nope: exit %d, stderr %q", code, stderr)
	}
}

func TestAdmin_Usage(t *testing.T) {
	cfg := adminApp(t.TempDir())
	for _, args := range [][]string{{}, {"retry"}, {"frob"}} {
		code, lines, _, stderr := runAdmin(t, cfg, nil, args...)
		if code != 2 || len(lines) != 0 || !strings.HasPrefix(stderr, "usage:") {
			t.Errorf("%q: exit %d, %+v, stderr %q", args, code, lines, stderr)
		}
	}
}

// list opens the shared file as a reader: it writes nothing, and leaves no -wal file.
func TestAdmin_ListReaderOnly(t *testing.T) {
	dir := t.TempDir()
	cfg := adminApp(dir)
	putJobs(t, cfg.Shared, adminRow{id: "dead0001", state: "dead"})
	list := func() {
		t.Helper()
		if code, lines, _, stderr := runAdmin(t, cfg, nil, "list"); code != 0 || len(lines) != 1 {
			t.Fatalf("exit %d, %+v, stderr %q", code, lines, stderr)
		}
	}

	before, err := os.Stat(cfg.Shared)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(cfg.Shared + "-wal"); !os.IsNotExist(err) {
		t.Fatalf("a -wal file before list: %v", err)
	}
	list()
	after, err := os.Stat(cfg.Shared)
	if err != nil {
		t.Fatal(err)
	}
	if !after.ModTime().Equal(before.ModTime()) || after.Size() != before.Size() {
		t.Errorf("the shared file changed: %v %d → %v %d", before.ModTime(), before.Size(), after.ModTime(), after.Size())
	}
	if _, err := os.Stat(cfg.Shared + "-wal"); !os.IsNotExist(err) {
		t.Errorf("a -wal file after list: %v", err)
	}

	// data_version, read on a connection held across list, changes when another commits a write
	held, err := sql.Open("sqlite", cfg.Shared+"?_pragma=query_only(1)")
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()
	conn, err := held.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	version := func() int {
		var v int
		if err := conn.QueryRowContext(context.Background(), `PRAGMA data_version`).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	v0 := version()
	list()
	if v := version(); v != v0 {
		t.Errorf("data_version %d → %d: list wrote to the shared file", v0, v)
	}
}

// schema1Jobs makes a store an older build left: a schema-1 jobs table (no priority column),
// as cmd/alder/jobs_test.go's schema1Jobs.
var schema1Jobs = []string{
	`CREATE TABLE IF NOT EXISTS "_alder_meta" ("key" TEXT NOT NULL PRIMARY KEY, "value" TEXT NOT NULL)`,
	`CREATE TABLE IF NOT EXISTS "_alder_jobs" (
  "id"            TEXT    NOT NULL PRIMARY KEY,
  "kind"          TEXT    NOT NULL,
  "params"        TEXT    NOT NULL,
  "key"           TEXT,
  "state"         TEXT    NOT NULL,
  "run_at"        INTEGER NOT NULL,
  "created"       INTEGER NOT NULL,
  "started"       INTEGER,
  "finished"      INTEGER,
  "attempts"      INTEGER NOT NULL DEFAULT 0,
  "error"         TEXT,
  "lease_until"   INTEGER,
  "worker"        TEXT,
  "schedule"      INTEGER NOT NULL DEFAULT 0,
  "scheduled_for" INTEGER
) STRICT`,
	`CREATE INDEX IF NOT EXISTS "_alder_jobs_due" ON "_alder_jobs" ("state", "run_at")`,
	`CREATE UNIQUE INDEX IF NOT EXISTS "_alder_jobs_once" ON "_alder_jobs" ("kind", "key") WHERE "key" IS NOT NULL AND "state" IN ('queued', 'running')`,
	`CREATE UNIQUE INDEX IF NOT EXISTS "_alder_jobs_tick" ON "_alder_jobs" ("kind") WHERE "schedule" = 1 AND "state" = 'queued'`,
	`CREATE TABLE IF NOT EXISTS "_alder_jobs_silos" (
  "key"     TEXT    NOT NULL,
  "value"   TEXT    NOT NULL,
  "touched" INTEGER NOT NULL,
  PRIMARY KEY ("key", "value")
) STRICT`,
	`INSERT INTO "_alder_meta" ("key", "value") VALUES ('jobs_schema', '1')`,
	`INSERT INTO "_alder_jobs" ("id", "kind", "params", "state", "run_at", "created", "attempts", "error", "finished") VALUES
  ('dead0001', 'A.B', '{}', 'dead', 1, 1, 5, 'e1', 3),
  ('dead0002', 'A.B', '{}', 'dead', 1, 1, 5, 'e2', 2)`,
}

// schema1Store makes the database file path a store with a schema-1 jobs table and two dead
// rows.
func schema1Store(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	st, err := db.OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	q, _ := db.On(st, nil)
	for _, s := range schema1Jobs {
		if _, err := q.ExecContext(context.Background(), s); err != nil {
			t.Fatal(err)
		}
	}
}

// adminSchemaOf is the jobs_schema of the database file path.
func adminSchemaOf(t *testing.T, path string) string {
	t.Helper()
	st, err := db.OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	q, _ := db.On(st, nil)
	v, err := db.JobsSchemaOf(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// A store whose jobs table is still schema 1 (the app has not opened it since the upgrade) is
// moved to the current schema first, as `alder jobs` does, then listed and retried.
func TestAdmin_Schema1Store(t *testing.T) {
	dir := t.TempDir()
	cfg := adminApp(dir)
	schema1Store(t, cfg.Shared)
	a := adminSilo(t, dir, "A")
	schema1Store(t, a)

	code, lines, _, stderr := runAdmin(t, cfg, nil, "list")
	if code != 0 || len(lines) != 2 {
		t.Fatalf("list: exit %d, %+v, stderr %q", code, lines, stderr)
	}
	for _, l := range lines {
		if l.Error != "" || l.Counts[jobs.Dead] != 2 || len(l.Dead) != 2 {
			t.Errorf("list %q: %+v", l.Silo, l)
		}
	}
	for _, p := range []string{cfg.Shared, a} {
		if v := adminSchemaOf(t, p); v != db.JobsSchema {
			t.Errorf("%s: jobs_schema %q, want %q", p, v, db.JobsSchema)
		}
	}

	b := adminSilo(t, dir, "B")
	schema1Store(t, b)
	code, lines, _, stderr = runAdmin(t, cfg, nil, "retry", "--dead")
	if code != 0 || len(lines) != 6 {
		t.Fatalf("retry --dead: exit %d, %+v, stderr %q", code, lines, stderr)
	}
	if r := adminGet(t, b, "dead0001"); r.State != "queued" || r.Attempts != 0 {
		t.Errorf("org/B's row: %+v", r)
	}
	if v := adminSchemaOf(t, b); v != db.JobsSchema {
		t.Errorf("org/B: jobs_schema %q", v)
	}
}
