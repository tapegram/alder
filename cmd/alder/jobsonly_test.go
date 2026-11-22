package main

import (
	"database/sql"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"alder/rt/db"
	"alder/rt/web"
)

// `app --jobs-only` (#168, spec §7a): the built binary runs its job workers and no listener.

// jobsProc is a running `bin --jobs-only`.
type jobsProc struct {
	cmd            *exec.Cmd
	stdout, stderr *syncBuf
	done           chan struct{} // closed when it has exited
}

// startJobsOnly starts bin --jobs-only with env added to the environment (ALDER_JOBS and the
// log settings cleared first); it is killed when the test ends unless it exited.
func startJobsOnly(t *testing.T, bin string, env ...string) *jobsProc {
	t.Helper()
	p := &jobsProc{stdout: &syncBuf{}, stderr: &syncBuf{}, done: make(chan struct{})}
	var base []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "ALDER_") {
			base = append(base, kv)
		}
	}
	if err := retryETXTBSY(func() error {
		p.cmd = exec.Command(bin, "--jobs-only")
		p.cmd.Env = append(base, env...)
		p.cmd.Stdout, p.cmd.Stderr = p.stdout, p.stderr
		return p.cmd.Start()
	}); err != nil {
		t.Fatal(err)
	}
	go func() { _ = p.cmd.Wait(); close(p.done) }()
	t.Cleanup(func() {
		select {
		case <-p.done:
		default:
			_ = p.cmd.Process.Kill()
			<-p.done
		}
	})
	return p
}

// waitLog waits (60 s at most) for a line of its standard error that contains s.
func (p *jobsProc) waitLog(t *testing.T, s string) {
	t.Helper()
	deadline := time.After(60 * time.Second)
	for !strings.Contains(p.stderr.String(), s) {
		select {
		case <-p.done:
			t.Fatalf("exited (%v) before %q; stderr:\n%s", p.cmd.ProcessState, s, p.stderr)
		case <-deadline:
			t.Fatalf("no %q within 60 s; stderr:\n%s", s, p.stderr)
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// stop sends sig and returns the exit code (failing the test after 20 s).
func (p *jobsProc) stop(t *testing.T, sig os.Signal) int {
	t.Helper()
	if err := p.cmd.Process.Signal(sig); err != nil {
		t.Fatal(err)
	}
	select {
	case <-p.done:
	case <-time.After(20 * time.Second):
		t.Fatalf("still running 20 s after %v; stderr:\n%s", sig, p.stderr)
	}
	return p.cmd.ProcessState.ExitCode()
}

// within polls cond every 10 ms for at most d.
func within(t *testing.T, d time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// jobState is the state and attempts of job id in the database file path, and its error.
func jobState(t *testing.T, path, id string) (state string, attempts int, errText sql.NullString) {
	t.Helper()
	d, err := sql.Open("sqlite", sqliteDSN(path))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if err := d.QueryRow(`SELECT "state", "attempts", "error" FROM "_alder_jobs" WHERE "id" = ?`, id).Scan(&state, &attempts, &errText); err != nil {
		t.Fatal(err)
	}
	return state, attempts, errText
}

// TestJobsOnly_NoListener and TestJobsOnly_DefaultWorkers on testdata/jobsonly (job Write
// inserts a Note; job Hold waits on the Gate adapter until its attempt is cancelled):
//   - NoListener: with ALDER_ADDR set to a free port, nothing listens there and nothing is
//     printed to standard output; a queued Write runs; a running Hold, on SIGTERM, is cancelled
//     after the grace and queued again uncounted, and the process exits 0.
//   - DefaultWorkers: ALDER_JOBS unset is 8 workers, ALDER_JOBS=2 is 2.
func TestJobsOnly_Binary(t *testing.T) {
	t.Parallel()
	needGoTool(t)
	if testing.Short() {
		t.Skip("builds a binary")
	}
	dir := tempAppAbs(t, "testdata/jobsonly", "worker")
	if code, out, errs := runBuild(t, dir); code != 0 {
		t.Fatalf("build: %d\n%s%s", code, out, errs)
	}
	bin := filepath.Join(testBuildDir(t), "worker", "worker")
	const secret = "ALDER_SECRET=0123456789abcdef-jobs-only"

	t.Run("NoListener", func(t *testing.T) {
		file := filepath.Join(t.TempDir(), "app.db")
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		addr := ln.Addr().String()
		ln.Close()
		p := startJobsOnly(t, bin, "ALDER_DB="+file, "ALDER_ADDR="+addr, secret)
		p.waitLog(t, "jobs only: 8 job workers, no listener")
		if c, err := net.DialTimeout("tcp", addr, time.Second); err == nil {
			c.Close()
			t.Fatalf("something listens on ALDER_ADDR %s", addr)
		}
		d, err := sql.Open("sqlite", sqliteDSN(file))
		if err != nil {
			t.Fatal(err)
		}
		defer d.Close()
		for _, row := range [][2]string{{"0000000a", "Work.Write"}, {"0000000b", "Work.Hold"}} {
			if _, err := d.Exec(`INSERT INTO "_alder_jobs" ("id", "kind", "params", "state", "run_at", "created") VALUES (?, ?, '{}', 'queued', 0, 0)`, row[0], row[1]); err != nil {
				t.Fatal(err)
			}
		}
		within(t, 15*time.Second, "Write done and Hold running", func() bool {
			w, _, _ := jobState(t, file, "0000000a")
			h, _, _ := jobState(t, file, "0000000b")
			return w == "done" && h == "running"
		})
		if n := sqlCount(t, file, `SELECT COUNT(*) FROM "Table__Note__Note"`); n != 1 {
			t.Fatalf("%d notes, want 1", n)
		}
		start := time.Now()
		if code := p.stop(t, syscall.SIGTERM); code != 0 {
			t.Fatalf("after SIGTERM: exit %d; stderr:\n%s", code, p.stderr)
		}
		if el := time.Since(start); el < 4*time.Second {
			t.Errorf("stopped in %v: the running attempt had no grace", el)
		}
		if state, attempts, e := jobState(t, file, "0000000b"); state != "queued" || attempts != 0 || e.Valid {
			t.Fatalf("Hold after SIGTERM: %s, %d attempts, error %v; want queued again, uncounted", state, attempts, e)
		}
		if out := p.stdout.String(); out != "" {
			t.Fatalf("standard output %q", out)
		}
	})

	t.Run("DefaultWorkers", func(t *testing.T) {
		p := startJobsOnly(t, bin, "ALDER_DB="+filepath.Join(t.TempDir(), "app.db"), secret, "ALDER_JOBS=2")
		p.waitLog(t, "jobs only: 2 job workers, no listener")
		if code := p.stop(t, syscall.SIGTERM); code != 0 {
			t.Fatalf("exit %d; stderr:\n%s", code, p.stderr)
		}
		p = startJobsOnly(t, bin, "ALDER_DB="+filepath.Join(t.TempDir(), "app.db"), secret, "ALDER_JOBS=x")
		<-p.done
		if code := p.cmd.ProcessState.ExitCode(); code != 1 || !strings.Contains(p.stderr.String(), "alder: ALDER_JOBS=x: want a whole number of job workers below 65536 (0: none)") {
			t.Fatalf("ALDER_JOBS=x: exit %d; stderr:\n%s", code, p.stderr)
		}
	})
}

// TestJobsOnly_SiloHint: testdata/corpus/silo-ok's binary on one database file, twice — a web
// instance with ALDER_JOBS=0 (a `workers:` deploy's web service) and `--jobs-only` (its worker).
// The worker starts first, and its start scan finds no silo with jobs; then POST /reports as a
// member of org A queues Ping in A's silo (Queue writes the silo), and the web instance, which
// has no workers, leaves a hint for org/A in the shared database's _alder_jobs_silos. The only
// way the worker can reach A now is that hint: it runs Ping there and clears the hint.
func TestJobsOnly_SiloHint(t *testing.T) {
	t.Parallel()
	needGoTool(t)
	if testing.Short() {
		t.Skip("builds a binary")
	}
	dir := tempAppAbs(t, "../../testdata/corpus/silo-ok", "siloapp")
	tmp := filepath.Dir(dir)
	os.RemoveAll(filepath.Join(dir, "build"))
	if code, out, errs := runMigrate(t, dir); code != 0 {
		t.Fatalf("migrate: %d\n%s%s", code, out, errs)
	}
	if code, out, errs := runBuild(t, dir); code != 0 {
		t.Fatalf("build: %d\n%s%s", code, out, errs)
	}
	bin := filepath.Join(testBuildDir(t), "siloapp", "siloapp")
	// startAppNamed runs build/siloapp/siloapp under tmp
	exe := filepath.Join(tmp, "build", "siloapp", "siloapp")
	os.MkdirAll(filepath.Dir(exe), 0o755)
	if b, err := os.ReadFile(bin); err != nil || os.WriteFile(exe, b, 0o755) != nil {
		t.Fatalf("copying the binary: %v", err)
	}
	data := t.TempDir()
	appDB := filepath.Join(data, "app.db")
	siloA := filepath.Join(data, "silos", "org", db.SiloFileName("A"))
	env := []string{"ALDER_DB=" + appDB, "ALDER_DEV=", "ALDER_SECRET=" + siloSecret}

	app := startAppNamed(t, tmp, "siloapp", append([]string{"ALDER_JOBS=0"}, env...)...) // migrates the file
	for _, st := range []string{
		`INSERT INTO "User__User" ("id", "name") VALUES ('me', 'Me')`,
		`INSERT INTO "Org__Org" ("id", "name") VALUES ('A', 'Org A')`,
		`INSERT INTO "Membership__Membership" ("id", "user", "org") VALUES ('m1', 'me', 'A')`,
	} {
		sqlExec(t, appDB, st)
	}
	p := startJobsOnly(t, bin, env...)
	p.waitLog(t, "jobs: the start scan looked in ")
	if s := p.stderr.String(); !strings.Contains(s, " silo(s): 0 with jobs") {
		t.Fatalf("the start scan found jobs:\n%s", s)
	}

	cookie, err := web.SessionCookie([]byte(siloSecret), siloCodec{}, siloSession{"me", "A"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodPost, "http://"+app.addr+"/reports", strings.NewReader(""))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", "http://"+app.addr+"/reports")
	req.AddCookie(&http.Cookie{Name: web.CookieName, Value: cookie})
	c := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode >= 400 {
		t.Fatalf("POST /reports: %d\n%s", res.StatusCode, body)
	}
	ping := func() string {
		t.Helper()
		d, err := sql.Open("sqlite", sqliteDSN(siloA))
		if err != nil {
			t.Fatal(err)
		}
		defer d.Close()
		var state string
		if err := d.QueryRow(`SELECT "state" FROM "_alder_jobs" WHERE "kind" = 'Reports.Ping'`).Scan(&state); err != nil {
			t.Fatal(err)
		}
		return state
	}
	within(t, 15*time.Second, "the worker to run Ping in A's silo", func() bool { return ping() == "done" })
	if n := sqlCount(t, appDB, `SELECT COUNT(*) FROM "_alder_jobs_silos"`); n != 0 {
		t.Fatalf("%d hints left", n)
	}
	if n := sqlCount(t, appDB, `SELECT COUNT(*) FROM "_alder_jobs"`); n != 0 {
		t.Fatalf("%d jobs in the shared database: Ping was not queued in the silo", n)
	}
	if code := p.stop(t, syscall.SIGTERM); code != 0 {
		t.Fatalf("exit %d; stderr:\n%s", code, p.stderr)
	}
}
