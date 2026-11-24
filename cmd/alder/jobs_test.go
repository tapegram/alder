package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"alder/rt/db"
	"alder/rt/jobs"
)

// `alder jobs` (#168, spec §9): the jobs of a dev database file and its silo files.

// jobsT0 is the time the test rows are made at.
var jobsT0 = time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)

// jobsRow is a row a test puts in a jobs table.
type jobsRow struct {
	id, kind, state, err string
	key                  string // "": none
	attempts             int
	priority             int
	finished             time.Duration // after jobsT0; 0: none
	schedule             bool
	scheduledFor         string // "2006-01-02 15:04" UTC; "": none
}

// jobsDB makes the database file path (and its directory) with a jobs table holding rows.
func jobsDB(t *testing.T, path string, rows ...jobsRow) {
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
		var finished, sched, errText any
		if r.finished != 0 {
			finished = jobsT0.Add(r.finished).UnixMilli()
		}
		if r.scheduledFor != "" {
			at, err := time.Parse("2006-01-02 15:04", r.scheduledFor)
			if err != nil {
				t.Fatal(err)
			}
			sched = at.UnixMilli()
		}
		if r.err != "" {
			errText = r.err
		}
		var key any
		if r.key != "" {
			key = r.key
		}
		runAt := jobsT0.UnixMilli()
		if s, ok := sched.(int64); ok {
			runAt = s
		}
		if _, err := q.ExecContext(ctx, `INSERT INTO "_alder_jobs" ("id", "kind", "params", "state", "run_at", "created", "attempts", "error", "finished", "schedule", "scheduled_for", "key", "priority") VALUES (?, ?, '{}', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			r.id, r.kind, r.state, runAt, jobsT0.UnixMilli(), r.attempts, errText, finished, r.schedule, sched, key, r.priority); err != nil {
			t.Fatal(err)
		}
	}
}

// jobsGet is row id of the database file path, nil when there is none.
func jobsGet(t *testing.T, path, id string) *jobs.Row {
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

// squash is out with each line's runs of spaces made one, and its trailing ones dropped.
func squash(out string) string {
	lines := strings.Split(out, "\n")
	for i, l := range lines {
		lines[i] = strings.Join(strings.Fields(l), " ")
	}
	return strings.Join(lines, "\n")
}

// jobsFixture is an app dir (jobs-ok, with a parameterless cron job Nightly added), a build dir,
// and its dev database with rows in the shared store and in silo org/o1.
func jobsFixture(t *testing.T) (dir, bd, dev, silo string) {
	t.Helper()
	dir = tempAppAbs(t, "../../testdata/corpus/jobs-ok", "jobsapp")
	editApp(t, dir, "Signup.alder", "job First = {}\n", "job Nightly = {} cron \"0 3 * * *\" priority 3\n\nNightly.run : Nightly.Params -> Task Unit\nNightly.run p = Task.succeed ()\n\njob First = {}\n")
	bd = t.TempDir()
	dev = filepath.Join(bd, "jobsapp", "dev.db")
	jobsDB(t, dev,
		jobsRow{id: "q0000001", kind: "Signup.First", state: "queued"},
		jobsRow{id: "tick0001", kind: "Signup.Nightly", state: "queued", schedule: true, scheduledFor: "2026-11-17 03:00", priority: 3},
		jobsRow{id: "run00001", kind: "Signup.Second", state: "running", attempts: 1},
		jobsRow{id: "dead0001", kind: "Signup.SendWelcome", state: "dead", attempts: 5, err: "smtp: timeout\nat line 2", finished: 2 * time.Minute},
		jobsRow{id: "dead0002", kind: "Signup.Nightly", state: "dead", attempts: 5, err: "boom", finished: time.Minute, scheduledFor: "2026-11-16 03:00"},
		jobsRow{id: "done0001", kind: "Signup.First", state: "done", attempts: 1, finished: time.Minute},
		jobsRow{id: "canc0001", kind: "Signup.First", state: "cancelled", finished: time.Minute},
	)
	silo = filepath.Join(bd, "jobsapp", "silos", "org", db.SiloFileName("o1"))
	jobsDB(t, silo, jobsRow{id: "dead0003", kind: "Reports.Digest", state: "dead", attempts: 1, err: "x", finished: 3 * time.Minute})
	return dir, bd, dev, silo
}

// `alder jobs` prints the count in each state, each scheduled kind's next tick and the dead rows
// (id, kind, attempts, error), the shared database's and its silos'.
func TestJobs_List(t *testing.T) {
	t.Parallel()
	dir, bd, dev, _ := jobsFixture(t)
	code, out := runCmd(t, runJobsTo, dir, "--build-dir", bd)
	want := squash(`jobs in ` + dev + ` and 1 silo database:
  queued 2
  running 1
  dead 3
  cancelled 1
  done 1
next schedule ticks:
  Signup.Nightly 2026-11-17 03:00 UTC priority 3
dead jobs, the last finished first:
  dead0003  Reports.Digest      1 attempt   x (in silo org/o1)
  dead0001  Signup.SendWelcome  5 attempts  smtp: timeout …
  dead0002  Signup.Nightly      5 attempts  boom (the schedule tick of 2026-11-16 03:00 UTC)
` + "`alder jobs retry <id>` queues one again; `alder jobs retry --dead` queues them all again\n")
	if code != 0 || squash(out) != want {
		t.Fatalf("exit %d\n%s\nwant\n%s", code, out, want)
	}
}

// `alder jobs retry <id>` queues a dead row again (attempts 0, error cleared), in the shared
// database or a silo; a retried tick says it runs once; a row not dead, and an unknown id, exit 1.
func TestJobs_RetryID(t *testing.T) {
	t.Parallel()
	dir, bd, dev, silo := jobsFixture(t)
	code, out := runCmd(t, runJobsTo, "retry", "dead0001", dir, "--build-dir", bd)
	if code != 0 || !strings.Contains(out, "queued job dead0001 (Signup.SendWelcome) again") {
		t.Fatalf("retry: exit %d\n%s", code, out)
	}
	if r := jobsGet(t, dev, "dead0001"); r.State != "queued" || r.Attempts != 0 || r.Error != nil {
		t.Fatalf("retried row: %+v", r)
	}
	code, out = runCmd(t, runJobsTo, "retry", "dead0002", dir, "--build-dir", bd)
	if code != 0 || !strings.Contains(out, "runs once: no next tick is queued after it") {
		t.Fatalf("retry of a tick: exit %d\n%s", code, out)
	}
	if r := jobsGet(t, dev, "dead0002"); r.State != "queued" || r.Schedule || r.ScheduledFor == nil {
		t.Fatalf("retried tick: %+v", r)
	}
	code, out = runCmd(t, runJobsTo, "retry", "dead0003", dir, "--build-dir", bd)
	if code != 0 || !strings.Contains(out, "in silo org/o1") {
		t.Fatalf("retry in a silo: exit %d\n%s", code, out)
	}
	if r := jobsGet(t, silo, "dead0003"); r.State != "queued" || r.Attempts != 0 {
		t.Fatalf("retried silo row: %+v", r)
	}
	if !strings.Contains(out, "or after they restart; a `--jobs-only` worker reads the silo hint within a poll") {
		t.Fatalf("silo note:\n%s", out)
	}
	code, out = runCmd(t, runJobsTo, "retry", "canc0001", dir, "--build-dir", bd)
	if code != 0 || !strings.Contains(out, "queued job canc0001 (Signup.First) again") {
		t.Fatalf("retry of a cancelled row: exit %d\n%s", code, out)
	}
	if r := jobsGet(t, dev, "canc0001"); r.State != "queued" || r.Attempts != 0 {
		t.Fatalf("retried cancelled row: %+v", r)
	}
	code, out = runCmd(t, runJobsTo, "retry", "done0001", dir, "--build-dir", bd)
	if code != 1 || !strings.Contains(out, "job done0001 is done: only a dead or cancelled job is retried") {
		t.Fatalf("retry of a done row: exit %d\n%s", code, out)
	}
	code, out = runCmd(t, runJobsTo, "retry", "nosuch01", dir, "--build-dir", bd)
	if code != 1 || !strings.Contains(out, "no job nosuch01 in "+dev+" or its 1 silo database") {
		t.Fatalf("retry of an unknown id: exit %d\n%s", code, out)
	}
}

// A silo file that cannot be read does not stop the listing: the other stores print, the missed
// one follows on stderr and the exit is 2.
func TestJobs_ListMissed(t *testing.T) {
	t.Parallel()
	dir, bd, dev, _ := jobsFixture(t)
	bad := filepath.Join(bd, "jobsapp", "silos", "org", db.SiloFileName("o2"))
	if err := os.MkdirAll(bad, 0o755); err != nil {
		t.Fatal(err)
	}
	var so, se strings.Builder
	code := runJobsTo([]string{dir, "--build-dir", bd}, &so, &se)
	out := squash(so.String())
	if code != 2 || !strings.Contains(out, "jobs in "+dev+" and 2 silo databases:") || !strings.Contains(out, "dead 3") || !strings.Contains(out, "dead0003 Reports.Digest 1 attempt x (in silo org/o1)") || !strings.Contains(out, "dead0001 Signup.SendWelcome") {
		t.Fatalf("exit %d\nstdout:\n%s", code, so.String())
	}
	if want := "alder jobs: silo org/o2 (" + bad + "): not read: "; !strings.Contains(se.String(), want) {
		t.Fatalf("stderr:\n%s\nwant %q", se.String(), want)
	}
}

// A retry of a row in a silo writes the silo's hint in the dev database (for a --jobs-only
// worker); a retry of a shared row writes none.
func TestJobs_RetryWritesHint(t *testing.T) {
	t.Parallel()
	dir, bd, dev, _ := jobsFixture(t)
	if code, out := runCmd(t, runJobsTo, "retry", "dead0001", dir, "--build-dir", bd); code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	if n := sqlCount(t, dev, `SELECT COUNT(*) FROM "_alder_jobs_silos"`); n != 0 {
		t.Fatalf("%d hints after a shared retry", n)
	}
	if code, out := runCmd(t, runJobsTo, "retry", "dead0003", dir, "--build-dir", bd); code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	if n := sqlCount(t, dev, `SELECT COUNT(*) FROM "_alder_jobs_silos" WHERE "key" = 'org' AND "value" = 'o1'`); n != 1 {
		t.Fatalf("%d hints for org/o1", n)
	}
}

// `alder jobs retry --dead` queues every dead row again, the silos' too.
func TestJobs_RetryDead(t *testing.T) {
	t.Parallel()
	dir, bd, dev, silo := jobsFixture(t)
	code, out := runCmd(t, runJobsTo, "retry", "--dead", dir, "--build-dir", bd)
	if code != 0 || !strings.Contains(out, "queued 3 dead jobs again") {
		t.Fatalf("retry --dead: exit %d\n%s", code, out)
	}
	for _, id := range []string{"dead0001", "dead0002"} {
		if r := jobsGet(t, dev, id); r.State != "queued" || r.Attempts != 0 || r.Error != nil {
			t.Errorf("%s: %+v", id, r)
		}
	}
	if r := jobsGet(t, silo, "dead0003"); r.State != "queued" || r.Attempts != 0 {
		t.Errorf("silo row: %+v", r)
	}
	code, out = runCmd(t, runJobsTo, "retry", "--dead", dir, "--build-dir", bd)
	if code != 0 || !strings.Contains(out, "no dead job") {
		t.Fatalf("retry --dead with none: exit %d\n%s", code, out)
	}
}

// `alder jobs retry --dead` that fails part way (a statement of RetryDead's, or a later
// database) still says what it queued again before the failure, then exits 2: those rows are
// queued (each retry commits on its own).
func TestJobs_RetryDeadPartialFailure(t *testing.T) {
	t.Parallel()
	t.Run("RetryDead", func(t *testing.T) {
		dev := filepath.Join(t.TempDir(), "dev.db")
		jobsDB(t, dev,
			jobsRow{id: "dead0001", kind: "A.B", state: "dead", attempts: 5, err: "e", finished: 2 * time.Minute},
			jobsRow{id: "dead0002", kind: "A.B", state: "dead", attempts: 5, err: "e", finished: time.Minute},
		)
		// the last finished first: dead0001 is queued again, then dead0002's UPDATE fails
		sqlExec(t, dev, `CREATE TRIGGER "boom" BEFORE UPDATE ON "_alder_jobs" WHEN OLD."id" = 'dead0002' BEGIN SELECT RAISE(ABORT, 'boom'); END`)
		var so, se strings.Builder
		code := runJobsTo([]string{"retry", "--dead", "--db", dev}, &so, &se)
		if code != 2 || so.String() != "queued 1 dead job again, due now: dead0001\n" || !strings.Contains(se.String(), dev+": ") || !strings.Contains(se.String(), "boom") {
			t.Fatalf("exit %d\nstdout:\n%s\nstderr:\n%s", code, so.String(), se.String())
		}
		if r := jobsGet(t, dev, "dead0001"); r.State != "queued" {
			t.Fatalf("dead0001: %+v", r)
		}
	})
	t.Run("LaterDatabase", func(t *testing.T) {
		dev := filepath.Join(t.TempDir(), "dev.db")
		jobsDB(t, dev, jobsRow{id: "dead0001", kind: "A.B", state: "dead", attempts: 5, err: "e", finished: time.Minute})
		silo := filepath.Join(db.SiloDir(dev), "silos", "org", db.SiloFileName("o1"))
		jobsDB(t, silo, jobsRow{id: "dead0003", kind: "R.D", state: "dead", attempts: 1, err: "x", finished: time.Minute})
		bad := filepath.Join(db.SiloDir(dev), "silos", "org", db.SiloFileName("o2"))
		if err := os.WriteFile(bad, []byte(strings.Repeat("not a database ", 100)), 0o644); err != nil {
			t.Fatal(err)
		}
		var so, se strings.Builder
		code := runJobsTo([]string{"retry", "--dead", "--db", dev}, &so, &se)
		want := "queued 2 dead jobs again, due now: dead0001, dead0003\n" +
			"  dead0003: in silo org/o1, a running app's workers find it only while the silo is in their active set (a job queued or running there in the last hour) or after they restart; a `--jobs-only` worker reads the silo hint within a poll\n"
		if code != 2 || so.String() != want || !strings.Contains(se.String(), "silo org/o2 ("+bad+"): ") {
			t.Fatalf("exit %d\nstdout:\n%s\nstderr:\n%s", code, so.String(), se.String())
		}
	})
}

// A dead job whose key a queued or running job of its kind holds is not retried: `retry <id>`
// exits 1 naming the holder; `retry --dead` queues the others again, reports the refused one and
// exits 1.
func TestJobs_RetryKeyHeld(t *testing.T) {
	t.Parallel()
	dev := filepath.Join(t.TempDir(), "dev.db")
	jobsDB(t, dev,
		jobsRow{id: "dead0001", kind: "A.B", state: "dead", attempts: 5, err: "e", finished: 2 * time.Minute, key: "k"},
		jobsRow{id: "held0001", kind: "A.B", state: "queued", key: "k"},
		jobsRow{id: "dead0002", kind: "A.B", state: "dead", attempts: 5, err: "e", finished: time.Minute},
	)
	const refused = `job dead0001 is not retried: job held0001 of A.B holds its key "k" (queued or running)`
	code, out := runCmd(t, runJobsTo, "retry", "dead0001", "--db", dev)
	if code != 1 || !strings.Contains(out, refused) {
		t.Fatalf("retry <id>: exit %d\n%s", code, out)
	}
	if r := jobsGet(t, dev, "dead0001"); r.State != "dead" {
		t.Fatalf("dead0001: %+v", r)
	}
	var so, se strings.Builder
	code = runJobsTo([]string{"retry", "--dead", "--db", dev}, &so, &se)
	if code != 1 || so.String() != "queued 1 dead job again, due now: dead0002\n" || !strings.Contains(se.String(), refused) {
		t.Fatalf("retry --dead: exit %d\nstdout:\n%s\nstderr:\n%s", code, so.String(), se.String())
	}
	if r := jobsGet(t, dev, "dead0001"); r.State != "dead" {
		t.Fatalf("dead0001 after --dead: %+v", r)
	}
}

// `alder jobs run Nightly` queues a row of the kind with `{}` that is not a tick; a kind with
// parameters, and one the app does not have, exit 2.
func TestJobs_Run(t *testing.T) {
	t.Parallel()
	dir, bd, dev, _ := jobsFixture(t)
	code, out := runCmd(t, runJobsTo, "run", "Nightly", dir, "--build-dir", bd)
	if code != 0 || !strings.Contains(out, "queued a run of Signup.Nightly") {
		t.Fatalf("run: exit %d\n%s", code, out)
	}
	st, err := db.OpenSQLite(dev)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	q, _ := db.On(st, nil)
	var id, params string
	var schedule int
	var sched any
	if err := q.QueryRowContext(context.Background(), `SELECT "id", "params", "schedule", "scheduled_for" FROM "_alder_jobs" WHERE "kind" = 'Signup.Nightly' AND "state" = 'queued' AND "id" != 'tick0001'`).Scan(&id, &params, &schedule, &sched); err != nil {
		t.Fatal(err)
	}
	if params != "{}" || schedule != 0 || sched != nil || !strings.Contains(out, "job "+id) {
		t.Fatalf("run row %s %s %d %v\n%s", id, params, schedule, sched, out)
	}
	if r := jobsGet(t, dev, "tick0001"); r.State != "queued" || !r.Schedule {
		t.Fatalf("the tick changed: %+v", r)
	}
	code, out = runCmd(t, runJobsTo, "run", "SendWelcome", dir, "--build-dir", bd)
	if code != 2 || !strings.Contains(out, "Signup.SendWelcome takes parameters (user)") {
		t.Fatalf("run of a job with parameters: exit %d\n%s", code, out)
	}
	code, out = runCmd(t, runJobsTo, "run", "Nope", dir, "--build-dir", bd)
	if code != 2 || !strings.Contains(out, "the app has no job Nope") {
		t.Fatalf("run of no job: exit %d\n%s", code, out)
	}
}

// `alder jobs run` on an app with a type error exits 1 and queues nothing.
func TestJobs_RunTypeError(t *testing.T) {
	t.Parallel()
	dir, bd, dev, _ := jobsFixture(t)
	writeFile(t, filepath.Join(dir, "app", "Bad.alder"), "module Bad\n\nbad : Int\nbad = \"text\"\n")
	code, out := runCmd(t, runJobsTo, "run", "Nightly", dir, "--build-dir", bd)
	if code != 1 || !strings.Contains(out, "the app in "+dir+" has errors (`alder check` lists them): fix them first") {
		t.Fatalf("exit %d\n%s", code, out)
	}
	if n := sqlCount(t, dev, `SELECT COUNT(*) FROM "_alder_jobs" WHERE "kind" = 'Signup.Nightly' AND "state" = 'queued' AND "schedule" = 0`); n != 0 {
		t.Fatalf("%d runs queued", n)
	}
}

// --db reads another file; a missing database and bad arguments exit 2.
func TestJobs_DBFlagAndUsage(t *testing.T) {
	t.Parallel()
	other := filepath.Join(t.TempDir(), "prod-copy.db")
	jobsDB(t, other, jobsRow{id: "deadx001", kind: "A.B", state: "dead", attempts: 2, err: "e", finished: time.Minute})
	code, out := runCmd(t, runJobsTo, "--db", other)
	if code != 0 || !strings.Contains(out, "jobs in "+other+":") || !strings.Contains(squash(out), "deadx001 A.B 2 attempts e") {
		t.Fatalf("--db: exit %d\n%s", code, out)
	}
	for _, args := range [][]string{{"--db", "libsql://app-acme.turso.io"}, {"retry", "--dead", "--db=libsql://app-acme.turso.io"}} {
		if code, out := runCmd(t, runJobsTo, args...); code != 2 || !strings.Contains(out, "--db takes a database file, not a database on Turso (libsql://app-acme.turso.io)") {
			t.Errorf("%v: exit %d\n%s", args, code, out)
		}
	}
	code, out = runCmd(t, runJobsTo, "--db", filepath.Join(t.TempDir(), "none.db"))
	if code != 2 || !strings.Contains(out, "no database at") {
		t.Fatalf("missing database: exit %d\n%s", code, out)
	}
	for _, args := range [][]string{{"retry"}, {"retry", "--dead", "a", "b"}, {"retry", "dead0001", "--dead"}, {"--db="}, {"--db=", "x"}, {"run", "X", "--dead"}, {"run"}, {"--dead"}, {"--nope"}, {"a", "b"}} {
		if code, out := runCmd(t, runJobsTo, args...); code != 2 || !strings.Contains(out, jobsUsage) {
			t.Errorf("%v: exit %d\n%s", args, code, out)
		}
	}
}

// `alder jobs --help` prints the usage line and what each form does.
func TestJobs_Help(t *testing.T) {
	t.Parallel()
	var out strings.Builder
	if !helpTo([]string{"jobs", "--help"}, &out) || !strings.HasPrefix(out.String(), jobsUsage+"\n") || !strings.Contains(out.String(), "A retried schedule tick runs once") {
		t.Fatalf("help:\n%s", out.String())
	}
}

// schema1Jobs is the jobs table as jobs_schema 1 made it (before #283's priority column and
// claim index), verbatim as rt/db's upgrade tests keep it.
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
}

// schema1DB makes the database file path a store an older build left: a schema-1 jobs table
// with two dead rows.
func schema1DB(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, st := range schema1Jobs {
		sqlExec(t, path, st)
	}
	sqlExec(t, path, `INSERT INTO "_alder_jobs" ("id", "kind", "params", "state", "run_at", "created", "attempts", "error", "finished") VALUES
  ('dead0001', 'A.B', '{}', 'dead', 1, 1, 5, 'e1', 3),
  ('dead0002', 'A.B', '{}', 'dead', 1, 1, 5, 'e2', 2)`)
}

// jobsSchemaOf is the jobs_schema row of the database file path.
func jobsSchemaOf(t *testing.T, path string) int {
	t.Helper()
	return sqlCount(t, path, `SELECT CAST("value" AS INTEGER) FROM "_alder_meta" WHERE "key" = 'jobs_schema'`)
}

// `alder jobs`, `retry <id>` and `retry --dead` on a store whose jobs table is still schema 1
// (the app has not opened it since the upgrade) move it to schema 2 first, as the app does, and
// then work; a store with no jobs table is still left without one.
func TestJobs_Schema1Store(t *testing.T) {
	t.Parallel()
	t.Run("List", func(t *testing.T) {
		dev := filepath.Join(t.TempDir(), "dev.db")
		schema1DB(t, dev)
		code, out := runCmd(t, runJobsTo, "--db", dev)
		if code != 0 || !strings.Contains(squash(out), "dead 2") || !strings.Contains(out, "dead0001") {
			t.Fatalf("exit %d\n%s", code, out)
		}
		if v := jobsSchemaOf(t, dev); v != 2 {
			t.Fatalf("jobs_schema %d", v)
		}
		if n := sqlCount(t, dev, `SELECT COUNT(*) FROM pragma_table_info('_alder_jobs') WHERE "name" = 'priority'`); n != 1 {
			t.Fatal("no priority column")
		}
	})
	t.Run("RetryID", func(t *testing.T) {
		dev := filepath.Join(t.TempDir(), "dev.db")
		schema1DB(t, dev)
		code, out := runCmd(t, runJobsTo, "retry", "dead0001", "--db", dev)
		if code != 0 || !strings.Contains(out, "queued job dead0001 (A.B) again") {
			t.Fatalf("exit %d\n%s", code, out)
		}
		if r := jobsGet(t, dev, "dead0001"); r.State != "queued" || r.Attempts != 0 || r.Priority != 0 {
			t.Fatalf("retried row: %+v", r)
		}
		if v := jobsSchemaOf(t, dev); v != 2 {
			t.Fatalf("jobs_schema %d", v)
		}
	})
	t.Run("RetryDead", func(t *testing.T) {
		dev := filepath.Join(t.TempDir(), "dev.db")
		schema1DB(t, dev)
		var so, se strings.Builder
		code := runJobsTo([]string{"retry", "--dead", "--db", dev}, &so, &se)
		if code != 0 || so.String() != "queued 2 dead jobs again, due now: dead0001, dead0002\n" {
			t.Fatalf("exit %d\nstdout:\n%s\nstderr:\n%s", code, so.String(), se.String())
		}
		if v := jobsSchemaOf(t, dev); v != 2 {
			t.Fatalf("jobs_schema %d", v)
		}
	})
	t.Run("NoJobsTable", func(t *testing.T) {
		dev := filepath.Join(t.TempDir(), "dev.db")
		sqlExec(t, dev, `CREATE TABLE "t" ("x" INTEGER)`)
		code, out := runCmd(t, runJobsTo, "--db", dev)
		if code != 0 {
			t.Fatalf("exit %d\n%s", code, out)
		}
		if n := sqlCount(t, dev, `SELECT COUNT(*) FROM sqlite_schema WHERE "name" = '_alder_jobs'`); n != 0 {
			t.Fatal("a jobs table was made")
		}
	})
}

// `alder jobs cancel <id>` cancels a queued or running row (in a silo too), refuses a tick and a
// final row with the panel's wording (exit 1), and an unknown id (exit 1).
func TestJobs_Cancel(t *testing.T) {
	t.Parallel()
	dir, bd, dev, silo := jobsFixture(t)
	jobsDB(t, silo, jobsRow{id: "sq000001", kind: "Reports.Digest", state: "queued"})
	code, out := runCmd(t, runJobsTo, "cancel", "sq000001", dir, "--build-dir", bd)
	if code != 0 || !strings.Contains(out, "cancelled job sq000001 (Reports.Digest) in silo org/o1") || !strings.Contains(out, "in silo org/o1, a running app's workers") {
		t.Fatalf("cancel in a silo: exit %d\n%s", code, out)
	}
	if r := jobsGet(t, silo, "sq000001"); r.State != "cancelled" {
		t.Fatalf("cancelled silo row: %+v", r)
	}
	code, out = runCmd(t, runJobsTo, "cancel", "q0000001", dir, "--build-dir", bd)
	if code != 0 || !strings.Contains(out, "cancelled job q0000001 (Signup.First)\n") || !strings.Contains(out, "a running app's workers stop it within a poll if it is running there") {
		t.Fatalf("cancel: exit %d\n%s", code, out)
	}
	if r := jobsGet(t, dev, "q0000001"); r.State != "cancelled" {
		t.Fatalf("cancelled row: %+v", r)
	}
	code, out = runCmd(t, runJobsTo, "cancel", "run00001", dir, "--build-dir", bd)
	if code != 0 || !strings.Contains(out, "cancelled job run00001 (Signup.Second)") {
		t.Fatalf("cancel of a running row: exit %d\n%s", code, out)
	}
	code, out = runCmd(t, runJobsTo, "cancel", "tick0001", dir, "--build-dir", bd)
	if code != 1 || !strings.Contains(out, "job tick0001: it is `Nightly`'s queued tick; remove its schedule to stop it") {
		t.Fatalf("cancel of a tick: exit %d\n%s", code, out)
	}
	if r := jobsGet(t, dev, "tick0001"); r.State != "queued" {
		t.Fatalf("tick: %+v", r)
	}
	code, out = runCmd(t, runJobsTo, "cancel", "done0001", dir, "--build-dir", bd)
	if code != 1 || !strings.Contains(out, "job done0001 is done: only a queued or running job is cancelled") {
		t.Fatalf("cancel of a done row: exit %d\n%s", code, out)
	}
	code, out = runCmd(t, runJobsTo, "cancel", "nosuch01", dir, "--build-dir", bd)
	if code != 1 || !strings.Contains(out, "no job nosuch01 in "+dev+" or its 1 silo database") {
		t.Fatalf("cancel of an unknown id: exit %d\n%s", code, out)
	}
	code, _ = runCmd(t, runJobsTo, "cancel", dir, "--build-dir", bd)
	if code != 2 {
		t.Errorf("cancel without an id: exit %d", code)
	}
	var help strings.Builder
	if !helpTo([]string{"jobs", "--help"}, &help) || !strings.Contains(help.String(), "cancel <id>") {
		t.Fatalf("help:\n%s", help.String())
	}
}
