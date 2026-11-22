package main

import (
	"database/sql"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
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
