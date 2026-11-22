package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"alder/internal/tursofake"
	"alder/rt/db"
	"alder/rt/jobs"
)

// TestStartCheck_Binary serves testdata/corpus/silo-ok (the M15 silo fixture, with a home page)
// with ALDER_DB=libsql://… at the fake Turso: the binary checks the shared database instead of
// migrating it (several instances would race). Two builds: v1 (0001) and v2 (0002 adds
// Project.due).
//   - Behind: the database at 0001, v2 exits 1 with "the database is behind this build: run
//     `alder deploy`", and nothing is migrated.
//   - Ahead: the database at 0002, v1 serves the maintenance page (503) on every route and
//     /_alder/health answers 503 "ahead: …".
//   - Equal: v2 serves, and /_alder/health answers 200 with ALDER_BUILD; with the fake closed,
//     503 {"db":"unreachable"} (no host, no driver error).
//   - FileStillMigrates: v2 with a file ALDER_DB migrates it on start (M14 unchanged).
func TestStartCheck_Binary(t *testing.T) {
	t.Parallel()
	needGoTool(t)
	if testing.Short() {
		t.Skip("builds two binaries")
	}
	dir := tempAppAbs(t, "../../testdata/corpus/silo-ok", "siloapp")
	tmp := filepath.Dir(dir)
	os.RemoveAll(filepath.Join(dir, "build"))
	writeFile(t, filepath.Join(dir, "app", "Home.alder"), "module Home\n\nroute Home = GET \"/\" { }\n\nHome.load : Home.Params -> Session -> Query Int\nHome.load params session = Query.succeed 1\n\nHome.view : Int -> Session -> Html\nHome.view n session = <html><body><h1>home</h1></body></html>\n")
	build := func(name string) string {
		t.Helper()
		if code, out, errs := runMigrate(t, dir); code != 0 {
			t.Fatalf("migrate %s: %d\n%s%s", name, code, out, errs)
		}
		if code, out, errs := runBuild(t, dir); code != 0 {
			t.Fatalf("build %s: %d\n%s%s", name, code, out, errs)
		}
		bin := filepath.Join(tmp, name)
		if err := os.Rename(filepath.Join(testBuildDir(t), "siloapp", "siloapp"), bin); err != nil {
			t.Fatal(err)
		}
		return bin
	}
	v1 := build("v1")
	editApp(t, dir, "Project.alder", "  name: Text,\n", "  name: Text,\n  due: Maybe Text = Nothing,\n")
	v2 := build("v2")

	f := tursofake.New(t, "acme")
	// the binaries' Turso settings, in their environment (base): the process's is not changed
	turso := []string{db.EnvTursoAPI + "=" + f.URL, db.EnvTursoOrg + "=" + f.Org, db.EnvTursoGroup + "=default", db.EnvTursoApp + "=siloapp",
		db.EnvTursoToken + "=" + f.APIToken, db.EnvTursoDB + "=siloapp", db.EnvDBToken + "="}
	if err := f.Create("siloapp", "default"); err != nil {
		t.Fatal(err)
	}
	d, ok, err := db.NewTurso(db.TursoConfig{API: f.URL, Org: f.Org, Group: "default", App: "siloapp", Token: f.APIToken}).Database(context.Background(), "siloapp")
	if err != nil || !ok {
		t.Fatalf("%v %v", ok, err)
	}
	remote := "libsql://" + d.Hostname
	base := append([]string{"ALDER_DEV=", "ALDER_SECRET=" + siloSecret, "ALDER_ADDR=127.0.0.1:0"}, turso...)
	run := func(bin, dbPath string, args ...string) (int, string) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		var out []byte
		var err error
		retryETXTBSY(func() error {
			cmd := exec.CommandContext(ctx, bin, args...)
			cmd.Env = append(append(os.Environ(), base...), "ALDER_DB="+dbPath)
			out, err = cmd.CombinedOutput()
			if errors.Is(err, syscall.ETXTBSY) {
				return err
			}
			return nil
		})
		if ctx.Err() != nil {
			t.Fatalf("%s %v still running after 60 s (it served?):\n%s", bin, args, out)
		}
		code := 0
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else if err != nil {
			t.Fatal(err)
		}
		return code, string(out)
	}
	// serveBin starts bin as build/siloapp/siloapp (a new file each time: macOS kills a process
	// whose executable was rewritten in place), stopped when t ends
	serveBin := func(t *testing.T, bin string, env ...string) *running {
		t.Helper()
		exe := filepath.Join(tmp, "build", "siloapp", "siloapp")
		os.MkdirAll(filepath.Dir(exe), 0o755)
		os.Remove(exe)
		b, err := os.ReadFile(bin)
		if err == nil {
			err = os.WriteFile(exe, b, 0o755)
		}
		if err != nil {
			t.Fatal(err)
		}
		return startAppNamed(t, tmp, "siloapp", append(base, env...)...)
	}
	get := func(t *testing.T, app *running, path string) (int, string) {
		t.Helper()
		res, err := http.Get("http://" + app.addr + path)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return res.StatusCode, string(b)
	}
	recorded := func() string {
		t.Helper()
		rows, err := f.DB("siloapp").Query(`SELECT "name" FROM "_alder_migrations" ORDER BY "name"`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		var names []string
		for rows.Next() {
			var n string
			rows.Scan(&n)
			names = append(names, n)
		}
		return strings.Join(names, " ")
	}
	// jobsTable reports whether the shared database has the jobs table
	jobsTable := func() bool {
		t.Helper()
		var n int
		if err := f.DB("siloapp").QueryRow(`SELECT COUNT(*) FROM sqlite_schema WHERE "type" = 'table' AND "name" = '_alder_jobs'`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n == 1
	}
	if code, out := run(v1, remote, "--migrate-only"); code != 0 {
		t.Fatalf("v1 --migrate-only: %d\n%s", code, out)
	}
	if !jobsTable() {
		t.Fatal("--migrate-only left no _alder_jobs")
	}

	t.Run("Behind", func(t *testing.T) {
		code, out := run(v2, remote)
		if code != 1 || !strings.Contains(out, "alder: the database is behind this build: run `alder deploy`\n") {
			t.Fatalf("%d\n%s", code, out)
		}
		if got := recorded(); got != "0001_initial" {
			t.Fatalf("recorded after the refusal: %q", got)
		}
		// TestJobsOnly_Behind: the worker refuses as the web build does
		code, out = run(v2, remote, "--jobs-only")
		if code != 1 || !strings.Contains(out, "alder: the database is behind this build: run `alder deploy`\n") {
			t.Fatalf("--jobs-only: %d\n%s", code, out)
		}
		if got := recorded(); got != "0001_initial" {
			t.Fatalf("recorded after --jobs-only's refusal: %q", got)
		}
	})

	if code, out := run(v2, remote, "--migrate-only"); code != 0 {
		t.Fatalf("v2 --migrate-only: %d\n%s", code, out)
	}
	t.Run("Ahead", func(t *testing.T) {
		app := serveBin(t, v1, "ALDER_DB="+remote, "ALDER_BUILD=b1")
		for _, path := range []string{"/", "/projects", "/nowhere"} {
			if code, body := get(t, app, path); code != 503 || !strings.Contains(body, "down for maintenance") { // App.maintenance
				t.Fatalf("%s: %d\n%s", path, code, body)
			}
		}
		code, body := get(t, app, "/_alder/health")
		var h map[string]string
		json.Unmarshal([]byte(body), &h)
		if code != 503 || h["build"] != "b1" || !strings.HasPrefix(h["db"], "ahead: a newer build migrated it (to 0002_") || !strings.HasSuffix(h["db"], ", which this build does not have)") {
			t.Fatalf("health: %d %s", code, body)
		}
		if strings.Contains(body, d.Hostname) || strings.Contains(body, "libsql") || strings.Contains(body, f.URL) {
			t.Fatalf("health names the database: %s", body)
		}
		if got := recorded(); !strings.HasPrefix(got, "0001_initial 0002_") {
			t.Fatalf("recorded: %q", got)
		}
	})

	// TestJobsOnly_Ahead: v1 --jobs-only on the database v2 migrated claims nothing (a due Ping,
	// queued before it starts, stays queued while it runs) and stops at once on SIGTERM; v2
	// --jobs-only, whose build the database is, runs it. The 10 s re-read is rt/jobs's
	// TestJobsOnly_Ahead (an injected clock). "Claims nothing" is an absence, so it takes a
	// bounded wait: a built binary has no clock to drive and logs nothing while AwaitBuild waits.
	// Workers that started (runJobs skipping AwaitBuild, or AwaitBuild returning at once) would
	// claim the due Ping in their first loop, right after the start housekeeping that logs
	// "jobs: the start scan" (rt/jobs pool.work), well within a poll (jobs.DefaultPoll, 1 s): the
	// test waits two polls and a half after runJobs' first line, then finds Ping unclaimed and no
	// start-scan line, and finds both still so once v1 has exited.
	t.Run("JobsOnlyAhead", func(t *testing.T) {
		// The Ahead build's streams go first (as Turso expires an idle one). Its four workers
		// (serving runs them even on an ahead database: main's StartJobs) claim every poll in a
		// transaction of their own on a Hrana stream: BEGIN, then the claim's UPDATE, which takes
		// the database's write lock, then ROLLBACK when no row is due. Killed (serveBin's Kill)
		// between the UPDATE and the ROLLBACK, the fake's stream keeps that transaction, and the
		// INSERT below would wait out the fake's busy_timeout (5 s) and fail with SQLITE_BUSY.
		f.ExpireStreams()
		env := append(append([]string{}, base...), "ALDER_DB="+remote)
		pingState := func() (state string, attempts int) {
			t.Helper()
			if err := f.DB("siloapp").QueryRow(`SELECT "state", "attempts" FROM "_alder_jobs" WHERE "id" = '0000000c'`).Scan(&state, &attempts); err != nil {
				t.Fatal(err)
			}
			return state, attempts
		}
		if _, err := f.DB("siloapp").Exec(`INSERT INTO "_alder_jobs" ("id", "kind", "params", "state", "run_at", "created") VALUES ('0000000c', 'Reports.Ping', '{}', 'queued', 0, 0)`); err != nil {
			t.Fatal(err)
		}
		p := startJobsOnly(t, v1, env...)
		p.waitLog(t, "the database is ahead of this build, no job runs until it is not (looked at again every 10 s): a newer build migrated it (to 0002_")
		// runJobs' first line, logged once its SIGTERM handler is in place (Open's line above
		// comes before it: a SIGTERM then would kill the process instead)
		p.waitLog(t, "jobs only: 8 job workers, no listener")
		time.Sleep(2*jobs.DefaultPoll + jobs.DefaultPoll/2) // an absence: see above
		if state, attempts := pingState(); state != "queued" || attempts != 0 || strings.Contains(p.stderr.String(), "jobs: the start scan") {
			t.Fatalf("Ping while v1 waits: %s, %d attempts; want queued, never claimed; stderr:\n%s", state, attempts, p.stderr)
		}
		start := time.Now()
		if code := p.stop(t, syscall.SIGTERM); code != 0 || time.Since(start) > 3*time.Second {
			t.Fatalf("SIGTERM while waiting: exit %d after %v; stderr:\n%s", code, time.Since(start), p.stderr)
		}
		if strings.Contains(p.stderr.String(), "jobs: the start scan") { // the workers' first housekeeping
			t.Fatalf("the workers started:\n%s", p.stderr)
		}
		if state, attempts := pingState(); state != "queued" || attempts != 0 {
			t.Fatalf("Ping: %s, %d attempts; want queued, never claimed", state, attempts)
		}
		p = startJobsOnly(t, v2, env...)
		within(t, 15*time.Second, "v2 to run Ping", func() bool { state, _ := pingState(); return state == "done" })
		if code := p.stop(t, syscall.SIGTERM); code != 0 {
			t.Fatalf("v2: exit %d; stderr:\n%s", code, p.stderr)
		}
	})

	t.Run("Equal", func(t *testing.T) {
		// a database migrated before the jobs table: the serving binary creates it at open. The
		// Ahead build's streams go first, for the cause JobsOnlyAhead gives (a claim's transaction
		// left open on the fake by the kill would make the DROP wait out busy_timeout and fail
		// with SQLITE_BUSY). After JobsOnlyAhead this finds nothing to drop (both --jobs-only
		// builds there are stopped by SIGTERM, which ends their claims before they exit); it is
		// for a -run that keeps Ahead but skips JobsOnlyAhead.
		f.ExpireStreams()
		if _, err := f.DB("siloapp").Exec(`DROP TABLE "_alder_jobs"`); err != nil {
			t.Fatal(err)
		}
		app := serveBin(t, v2, "ALDER_DB="+remote, "ALDER_BUILD=b2")
		if !jobsTable() {
			t.Fatal("serving left no _alder_jobs")
		}
		if code, body := get(t, app, "/_alder/health"); code != 200 || body != `{"build":"b2","db":"ok","jobs":"ok"}`+"\n" {
			t.Fatalf("health: %d %q", code, body)
		}
		if code, body := get(t, app, "/"); code != 200 || !strings.Contains(body, "home") {
			t.Fatalf("home: %d\n%s", code, body)
		}
		// the database goes away: 503 "unreachable", and nothing of the driver's error
		f.Close()
		if code, body := get(t, app, "/_alder/health"); code != 503 || body != `{"build":"b2","db":"unreachable","jobs":"ok"}`+"\n" {
			t.Fatalf("health with the database gone: %d %q", code, body)
		}
	})

	t.Run("FileStillMigrates", func(t *testing.T) {
		file := filepath.Join(t.TempDir(), "app.db")
		app := serveBin(t, v2, "ALDER_DB="+file)
		if code, _ := get(t, app, "/_alder/health"); code != 200 {
			t.Fatalf("health: %d", code)
		}
		if n := sqlCount(t, file, `SELECT COUNT(*) FROM "_alder_migrations"`); n != 2 {
			t.Fatalf("the file has %d migrations recorded", n)
		}
	})
}
