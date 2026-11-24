package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"alder/internal/deploy"
	"alder/internal/tursofake"
	"alder/rt/db"
	"alder/rt/jobs"
)

// `alder jobs --env <name>` (#285, spec §1, §3, §6-§8): the jobs of a deployed environment, read
// and changed by the current build's --jobs task at the box or machine.

// remoteAt is a remote source's words as runJobsEnv builds them, for the printer tests.
var remoteAt = jobsPlace{head: "at Fake box:/srv/demo, build b1", flags: " --env production", who: "build"}

// jobsBody is out without its header (the first line) and the closing hint line: the lines a
// local and a remote listing of the same rows share.
func jobsBody(out string) string {
	var keep []string
	for i, l := range strings.Split(out, "\n") {
		if i == 0 || strings.HasPrefix(l, "`alder jobs retry <id>") {
			continue
		}
		keep = append(keep, l)
	}
	return strings.Join(keep, "\n")
}

// adminLinesOf are the list lines the --jobs task prints for the database files paths (the
// shared one first, then silo "org/<value>" files), through JSON and DecodeAdmin as the CLI
// reads them.
func adminLinesOf(t *testing.T, shared string, silos map[string]string) []jobs.AdminLine {
	t.Helper()
	ctx := context.Background()
	var out bytes.Buffer
	enc := json.NewEncoder(&out)
	line := func(silo, path string) {
		st, err := db.OpenSQLiteReader(path)
		if err != nil {
			t.Fatal(err)
		}
		defer st.Close()
		q, _ := db.On(st, nil)
		c, err := jobs.Counts(ctx, q)
		if err != nil {
			t.Fatal(err)
		}
		l, err := jobs.List(ctx, q)
		if err != nil {
			t.Fatal(err)
		}
		al := jobs.AdminLine{Silo: silo, Counts: c, Dead: l.Dead}
		for _, r := range l.Queued {
			if r.Schedule {
				al.Ticks = append(al.Ticks, r)
			}
		}
		enc.Encode(al)
	}
	line("", shared)
	for silo, path := range silos {
		line(silo, path)
	}
	lines, msgs := jobs.DecodeAdmin(out.String())
	if len(msgs) > 0 {
		t.Fatalf("messages: %q", msgs)
	}
	return lines
}

// The remote listing prints, for the same rows, every line the local one does but the header
// and the hint (Review Focus 5: a dead row whose error spans lines, a tick's, a silo's).
func TestJobsEnv_ReportParity(t *testing.T) {
	t.Parallel()
	dir, bd, dev, silo := jobsFixture(t)
	_, local := runCmd(t, runJobsTo, dir, "--build-dir", bd)
	var so, se bytes.Buffer
	r := reportOf(adminLinesOf(t, dev, map[string]string{"org/o1": silo}))
	code := printJobs(&so, &se, remoteAt.withSilos(r.silos), r)
	if code != 0 || se.Len() > 0 {
		t.Fatalf("exit %d\n%s", code, se.String())
	}
	if got, want := jobsBody(so.String()), jobsBody(local); got != want {
		t.Fatalf("remote:\n%s\nlocal:\n%s", so.String(), local)
	}
	lines := strings.Split(strings.TrimSuffix(so.String(), "\n"), "\n")
	if lines[0] != "jobs at Fake box:/srv/demo, build b1, and 1 silo database:" {
		t.Errorf("header: %q", lines[0])
	}
	if last := lines[len(lines)-1]; last != "`alder jobs retry <id> --env production` queues one again; `alder jobs retry --dead --env production` queues them all again" {
		t.Errorf("hint: %q", last)
	}
}

// A store the task could not read is a missed line after the table, exit 2; the header counts
// the silo databases looked at.
func TestJobsEnv_ReportMissed(t *testing.T) {
	t.Parallel()
	lines, _ := jobs.DecodeAdmin(`{"silo":"","counts":{"queued":1}}
{"silo":"org/A","counts":{}}
{"silo":"org/B","error":"disk I/O error"}
`)
	var so, se bytes.Buffer
	r := reportOf(lines)
	code := printJobs(&so, &se, remoteAt.withSilos(r.silos), r)
	if code != 2 || !strings.HasPrefix(so.String(), "jobs at Fake box:/srv/demo, build b1, and 2 silo databases:\n") || !strings.Contains(se.String(), "silo org/B: not read: disk I/O error") {
		t.Fatalf("exit %d\n%s%s", code, so.String(), se.String())
	}
	lines, _ = jobs.DecodeAdmin(`{"silo":"","error":"no such file"}` + "\n")
	so.Reset()
	se.Reset()
	r = reportOf(lines)
	if code := printJobs(&so, &se, remoteAt.withSilos(r.silos), r); code != 2 || !strings.HasPrefix(so.String(), "jobs at Fake box:/srv/demo, build b1:\n") || !strings.Contains(se.String(), "the shared database: not read: no such file") {
		t.Fatalf("exit %d\n%s%s", code, so.String(), se.String())
	}
}

// A retry's line read back from the task: the row as it was, its silo, and a silo under
// maintenance (which only the build knows) gets its note; the workers are "a running build's".
func TestJobsEnv_RetryLines(t *testing.T) {
	t.Parallel()
	lines, _ := jobs.DecodeAdmin(`{"silo":"org/A","row":{"id":"dead0003","kind":"Reports.Digest","state":"dead","run_at":0,"created":0,"attempts":1,"error":"x","finished":null,"schedule":false,"scheduled_for":null,"key":null,"priority":0},"maintenance":"a pending migration DROPS DATA (0002_x)"}` + "\n")
	if len(lines) != 1 {
		t.Fatalf("lines: %+v", lines)
	}
	var so bytes.Buffer
	printRetried(&so, remoteAt, placedOf(lines[0]))
	want := "queued job dead0003 (Reports.Digest) again in silo org/A, due now: attempts 0, its error cleared\n" +
		"  in silo org/A, a running build's workers find it only while the silo is in their active set (a job queued or running there in the last hour) or after they restart; a `--jobs-only` worker reads the silo hint within a poll\n" +
		"  in silo org/A, under maintenance (a pending migration DROPS DATA (0002_x)): the workers do not look there until `alder migrate silos` serves it again\n"
	if so.String() != want {
		t.Fatalf("got\n%s\nwant\n%s", so.String(), want)
	}
}

// An older build answers --jobs with its usage and exit 2: told apart from the task's own
// answers.
func TestJobsEnv_OldBuild(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		out  string
		code int
		want bool
	}{
		{"usage: app [--jobs-only | --migrate-only]\n", 2, true},
		{"{\"time\":\"x\",\"level\":\"INFO\",\"msg\":\"start\"}\nusage: app [--migrate-only]\n", 2, true},
		{"{\"silo\":\"\",\"counts\":{}}\nusage: x\n", 2, false},
		{"usage: x\n", 1, false},
		{"the shared database: disk I/O error\n", 2, false},
	} {
		if got := oldBuild(c.out, c.code); got != c.want {
			t.Errorf("oldBuild(%q, %d) = %v", c.out, c.code, got)
		}
	}
}

// The usage errors of --env, before anything is read.
func TestJobsEnv_Usage(t *testing.T) {
	t.Parallel()
	c := newDeployCmds(t)
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"--env", "production", "--db", "x.db"}, "alder jobs: --db takes a file; --env an environment"},
		{[]string{"--db=x.db", "--env=production"}, "alder jobs: --db takes a file; --env an environment"},
		{[]string{"cancel", "abc", "--env", "production"}, "alder jobs: cancel <id> works on a local database only"},
		{[]string{"--yes"}, jobsUsage},
		{[]string{"--env", "production", "--yes"}, jobsUsage},
		{[]string{"cancel", "abc", "--yes"}, jobsUsage},
		{[]string{"--host", "h"}, jobsUsage},
		{[]string{"retry", "x", "--host=h", "--yes"}, jobsUsage},
		{[]string{"--env="}, jobsUsage},
	} {
		if code, out := runCmd(t, c.jobs, tc.args...); code != 2 || !strings.Contains(out, tc.want) {
			t.Errorf("%v: exit %d\n%s", tc.args, code, out)
		}
	}
}

// A Script environment keeps its own state: ScriptUnavailable, exit 2.
func TestJobsEnv_Script(t *testing.T) {
	t.Parallel()
	dir := deployScaffold(t, `deploy {
  production: Script "./scripts/record.sh",
}`, nil)
	c := newDeployCmds(t)
	want := deploy.ScriptUnavailable("jobs", "production").Error()
	if code, out := runCmd(t, c.jobs, "--env", "production", dir); code != 2 || !strings.Contains(out, want) {
		t.Fatalf("exit %d\n%s", code, out)
	}
	if code, out := runCmd(t, c.jobs, "--env", "staging", dir); code != 2 || !strings.Contains(out, "has no environment `staging`") {
		t.Fatalf("unknown environment: exit %d\n%s", code, out)
	}
}

// A Render environment is refused plainly: `alder jobs --env` does not reach one yet (#285 PR 3).
func TestJobsEnv_Render(t *testing.T) {
	t.Parallel()
	dir, _, c := renderApp(t, rdDecl)
	if code, out := runCmd(t, c.jobs, "--env", "production", dir); code != 2 || !strings.Contains(out, "alder jobs: alder jobs --env does not reach a Render environment yet") || strings.Contains(out, "internal error") {
		t.Fatalf("exit %d\n%s", code, out)
	}
}

// boxLockFree is whether the box's deploy lock can be taken right now (and it is released).
func boxLockFree(t *testing.T, c *deployCmds, box *sshBox) error {
	t.Helper()
	s := newSshTarget(c.e(), "siloapp", "production", irSsh(box), &irDeployNone)
	defer s.Close()
	if _, err := s.Connect(t.Context()); err != nil {
		t.Fatal(err)
	}
	unlock, err := s.Lock(t.Context())
	if err != nil {
		return err
	}
	unlock()
	return nil
}

// `alder jobs --env` at an Ssh box with a file database and a silo: the listing (the same row
// lines as the local command over the same files), a silo retry unconfirmed then confirmed
// (the row queued, the silo's hint written, no lock left), retry --dead and its no-op, run, a
// kind with parameters, and the lock held by another run.
func TestJobs_EnvSsh(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("a deploy end to end: a real build at the loopback box")
	}
	box := startBox(t)
	dir, dc := siloDeployApp(t, box, fmt.Sprintf(`deploy {
  production: Ssh %q { domain: "localhost", path: %q },
  proxy: None,
  keep: 3,
}`, box.host, box.root))
	box.write(t, "env", fmt.Sprintf("ALDER_SECRET=%s\n", siloSecret))
	code, out, errs := dc.deploy("production", dir, "--unsafe-skip-tests")
	if code != 0 {
		t.Fatalf("deploy: %d\n%s%s", code, out, errs)
	}
	id := deployedID(t, out)
	shared := filepath.Join(box.root, "db", "app.db")
	for _, st := range seedShared {
		sqlExec(t, shared, st)
	}
	newSiloClient(t, box).add("A", "a1") // A's silo file, made by the app
	siloA := filepath.Join(box.root, "db", "silos", "org", db.SiloFileName("A"))
	// kinds the build does not declare stay queued once retried (its workers leave them)
	jobsDB(t, shared,
		jobsRow{id: "deadS001", kind: "Mail.Send", state: "dead", attempts: 5, err: "smtp: timeout\nat line 2", finished: 2 * time.Minute},
		jobsRow{id: "tickD001", kind: "Reports.Nightly", state: "dead", attempts: 5, err: "boom", finished: time.Minute, scheduledFor: "2026-11-16 03:00"},
	)
	jobsDB(t, siloA, jobsRow{id: "deadA001", kind: "Reports.Digest", state: "dead", attempts: 1, err: "digest: no org\nat line 7", finished: 3 * time.Minute})

	// the listing
	code, so, se := runJobsCmd(dc, "--env", "production", dir)
	if code != 0 {
		t.Fatalf("list: %d\n%s%s", code, so, se)
	}
	lines := strings.Split(strings.TrimSuffix(so, "\n"), "\n")
	if want := "jobs at Ssh alder-test-box:" + box.root + ", build " + id + ", and 1 silo database:"; lines[0] != want {
		t.Fatalf("header %q, want %q\n%s", lines[0], want, so)
	}
	_, local := runCmd(t, runJobsTo, "--db", shared)
	if jobsBody(so) != jobsBody(local) {
		t.Fatalf("remote:\n%s\nlocal:\n%s", so, local)
	}
	if last := lines[len(lines)-1]; !strings.HasSuffix(last, "queues them all again") || strings.Count(last, " --env production`") != 2 {
		t.Fatalf("hint: %q", last)
	}
	if !strings.Contains(squash(so), "deadA001 Reports.Digest 1 attempt digest: no org … (in silo org/A)") || !strings.Contains(so, "smtp: timeout …") || !strings.Contains(so, "(the schedule tick of 2026-11-16 03:00 UTC)") {
		t.Fatalf("rows:\n%s", so)
	}

	// a retry not confirmed (the test's stdin is no terminal): nothing queued, no lock left
	code, so, se = runJobsCmd(dc, "retry", "deadA001", "--env", "production", dir)
	if code != 2 || !strings.Contains(se, "not confirmed: nothing was queued (pass --yes to confirm without a terminal)") {
		t.Fatalf("retry unconfirmed: %d\n%s%s", code, so, se)
	}
	if r := jobsGet(t, siloA, "deadA001"); r.State != "dead" {
		t.Fatalf("after an unconfirmed retry: %+v", r)
	}
	if err := boxLockFree(t, dc, box); err != nil {
		t.Fatalf("lock after an unconfirmed retry: %v", err)
	}

	// confirmed: queued in its silo, the hint written, the lock released
	code, so, se = runJobsCmd(dc, "retry", "deadA001", "--env", "production", dir, "--yes")
	if code != 0 || !strings.Contains(so, "queued job deadA001 (Reports.Digest) again in silo org/A, due now") ||
		!strings.Contains(so, "a running build's workers") || !strings.Contains(so, "; a `--jobs-only` worker reads the silo hint within a poll") {
		t.Fatalf("retry --yes: %d\n%s%s", code, so, se)
	}
	if r := jobsGet(t, siloA, "deadA001"); r.State != "queued" || r.Attempts != 0 {
		t.Fatalf("retried row: %+v", r)
	}
	if n := sqlCount(t, shared, `SELECT COUNT(*) FROM "_alder_jobs_silos" WHERE "key" = 'org' AND "value" = 'A'`); n != 1 {
		t.Fatalf("hint rows: %d", n)
	}
	if err := boxLockFree(t, dc, box); err != nil {
		t.Fatalf("lock after a retry: %v", err)
	}

	// retry --dead: the two shared rows; again: no dead job, asked nothing (Review Focus 2)
	code, so, se = runJobsCmd(dc, "retry", "--dead", "--env", "production", dir, "--yes")
	if code != 0 || !strings.Contains(so, "queued 2 dead jobs again, due now: ") || !strings.Contains(so, "deadS001") || !strings.Contains(so, "tickD001: it is the Reports.Nightly schedule tick of 2026-11-16 03:00 UTC") {
		t.Fatalf("retry --dead: %d\n%s%s", code, so, se)
	}
	if err := boxLockFree(t, dc, box); err != nil {
		t.Fatalf("lock after retry --dead: %v", err)
	}
	code, so, se = runJobsCmd(dc, "retry", "--dead", "--env", "production", dir)
	if code != 0 || so != "no dead job\n" || se != "" {
		t.Fatalf("retry --dead with none: %d\n%s%s", code, so, se)
	}

	// run: a kind without parameters; one with parameters is the build's refusal
	code, so, se = runJobsCmd(dc, "run", "Ping", "--env", "production", dir, "--yes")
	if code != 0 || !strings.Contains(so, "queued a run of Reports.Ping, due now: job ") {
		t.Fatalf("run Ping: %d\n%s%s", code, so, se)
	}
	if err := boxLockFree(t, dc, box); err != nil {
		t.Fatalf("lock after run: %v", err)
	}
	code, so, se = runJobsCmd(dc, "run", "Digest", "--env", "production", dir, "--yes")
	if code != 2 || !strings.Contains(se, "Reports.Digest takes parameters (org): only a job without parameters is run from here") {
		t.Fatalf("run Digest: %d\n%s%s", code, so, se)
	}

	// the lock held by another run: refused naming the holder, nothing changed
	jobsDB(t, shared, jobsRow{id: "deadS002", kind: "Mail.Send", state: "dead", attempts: 5, err: "e", finished: 4 * time.Minute})
	s := newSshTarget(dc.e(), "siloapp", "production", irSsh(box), &irDeployNone)
	defer s.Close()
	if _, err := s.Connect(t.Context()); err != nil {
		t.Fatal(err)
	}
	unlock, err := s.Lock(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	code, so, se = runJobsCmd(dc, "retry", "deadS002", "--env", "production", dir, "--yes")
	unlock()
	if code != 1 || !strings.Contains(se, "is held by ") {
		t.Fatalf("retry under the lock: %d\n%s%s", code, so, se)
	}
	if r := jobsGet(t, shared, "deadS002"); r.State != "dead" {
		t.Fatalf("after a refused retry: %+v", r)
	}
}

// runJobsCmd is `alder jobs args` with c's environment: its exit, stdout and stderr.
func runJobsCmd(c *deployCmds, args ...string) (int, string, string) {
	var so, se bytes.Buffer
	code := c.jobs(args, &so, &se)
	return code, so.String(), se.String()
}

// The same at a box whose database is on Turso (the fake): the shared database and org A's
// child, read and retried by the build at the box with the token in its environment file.
func TestJobs_EnvSshTurso(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("a deploy end to end: a real build at the loopback box")
	}
	box := startBox(t)
	f := tursofake.New(t, "acme")
	dir, dc := siloDeployApp(t, box, fmt.Sprintf(`deploy {
  production: Ssh %q { domain: "localhost", path: %q, database: Turso "acme" { app: "siloapp", sample: 2 } },
  proxy: None,
  keep: 3,
}`, box.host, box.root))
	clock := dc.useFakeClock()
	f.SetNow(clock.Now)
	dc.env.Turso = db.TursoConfig{API: f.URL, Token: f.APIToken}
	box.write(t, "env", fmt.Sprintf("ALDER_SECRET=%s\nTURSO_API_TOKEN=%s\nALDER_TURSO_API=%s\n", siloSecret, f.APIToken, f.URL))
	code, out, errs := dc.deploy("production", dir, "--unsafe-skip-tests")
	if code != 0 {
		t.Fatalf("deploy: %d\n%s%s", code, out, errs)
	}
	id := deployedID(t, out)
	for _, st := range seedShared {
		if err := f.Exec("siloapp", st); err != nil {
			t.Fatal(err)
		}
	}
	newSiloClient(t, box).add("A", "a1")
	childA := db.ChildName("siloapp", "org", "A")
	jobsInsert(t, f.DB("siloapp"), jobsRow{id: "deadS001", kind: "Mail.Send", state: "dead", attempts: 5, err: "smtp: timeout\nat line 2", finished: 2 * time.Minute})
	jobsInsert(t, f.DB(childA), jobsRow{id: "deadA001", kind: "Reports.Digest", state: "dead", attempts: 1, err: "x", finished: 3 * time.Minute})

	code, so, se := runJobsCmd(dc, "--env", "production", dir)
	if code != 0 || !strings.HasPrefix(so, "jobs at Ssh alder-test-box:"+box.root+", build "+id+", and 1 silo database:\n") ||
		!strings.Contains(squash(so), "deadA001 Reports.Digest 1 attempt x (in silo org/A)") || !strings.Contains(squash(so), "deadS001 Mail.Send 5 attempts smtp: timeout …") {
		t.Fatalf("list: %d\n%s%s", code, so, se)
	}
	code, so, se = runJobsCmd(dc, "retry", "deadA001", "--env", "production", dir, "--yes")
	if code != 0 || !strings.Contains(so, "queued job deadA001 (Reports.Digest) again in silo org/A, due now") {
		t.Fatalf("retry: %d\n%s%s", code, so, se)
	}
	var state string
	if err := f.DB(childA).QueryRow(`SELECT "state" FROM "_alder_jobs" WHERE "id" = 'deadA001'`).Scan(&state); err != nil || state != "queued" {
		t.Fatalf("retried row: %q %v", state, err)
	}
	var n int
	f.DB("siloapp").QueryRow(`SELECT COUNT(*) FROM "_alder_jobs_silos" WHERE "key" = 'org' AND "value" = 'A'`).Scan(&n)
	if n != 1 {
		t.Fatalf("hint rows: %d", n)
	}
}

// A box with no build yet, and a build made before --jobs (its executable answers with its
// usage and exit 2): not deployed, and the old-build sentence, never the usage text.
func TestJobs_EnvOldBuild(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("the loopback box")
	}
	box := startBox(t)
	if box.container != "" {
		t.Skip("writes the box's files from the test")
	}
	dir := deployScaffold(t, fmt.Sprintf(`deploy {
  production: Ssh %q { domain: "localhost", path: %q },
  proxy: None,
}`, box.host, box.root), nil)
	c := box.cmds(t)
	if err := os.MkdirAll(box.root, 0o700); err != nil {
		t.Fatal(err)
	}
	code, so, se := runJobsCmd(c, "--env", "production", dir)
	if code != 1 || !strings.Contains(se, "environment production is not deployed yet: no build serves there") {
		t.Fatalf("no build: %d\n%s%s", code, so, se)
	}

	const id = "0123456789abcdef"
	b := filepath.Join(box.root, "builds", id)
	writeFile(t, filepath.Join(b, "run"), "#!/bin/sh\nexec \"$(dirname \"$0\")/app\" \"$@\"\n")
	writeFile(t, filepath.Join(b, "app"), "#!/bin/sh\necho 'usage: app [--migrate-only]' >&2\nexit 2\n")
	os.Chmod(filepath.Join(b, "run"), 0o755)
	os.Chmod(filepath.Join(b, "app"), 0o755)
	writeFile(t, filepath.Join(box.root, "current"), id+"\n")
	want := "build " + id + " at production was made by an alder without `alder jobs --env`: `alder deploy production` with this alder, then try again"
	for _, args := range [][]string{{"--env", "production", dir}, {"retry", "x", "--env", "production", dir, "--yes"}} {
		code, so, se := runJobsCmd(c, args...)
		if code != 1 || !strings.Contains(se, want) || strings.Contains(so+se, "usage:") {
			t.Fatalf("%v: %d\n%s%s", args, code, so, se)
		}
	}
	if err := boxLockFree(t, c, box); err != nil {
		t.Fatalf("lock after the old build's retry: %v", err)
	}
}

// A code outside the task's own 0, 1 and 2 (127: no run script at the box) is 1, not reached.
func TestJobsExit(t *testing.T) {
	t.Parallel()
	for in, want := range map[int]int{0: 0, 1: 1, 2: 2, 3: 1, 127: 1, 137: 1, -1: 1} {
		if got := jobsExit(in); got != want {
			t.Errorf("jobsExit(%d) = %d, want %d", in, got, want)
		}
	}
}
