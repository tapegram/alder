package gen

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"alder/rt/jobs"
)

// calls are the functions body calls, in source order, as written ("app.Open", "stopJobs").
func calls(body ast.Node) []string {
	var out []string
	ast.Inspect(body, func(n ast.Node) bool {
		if c, ok := n.(*ast.CallExpr); ok {
			var b strings.Builder
			if err := printExpr(&b, c.Fun); err == nil {
				out = append(out, b.String())
			}
		}
		return true
	})
	return out
}

// printExpr writes an identifier or a selector chain ("a.b.c"); anything else is an error.
func printExpr(b *strings.Builder, e ast.Expr) error {
	switch e := e.(type) {
	case *ast.Ident:
		b.WriteString(e.Name)
		return nil
	case *ast.SelectorExpr:
		if err := printExpr(b, e.X); err != nil {
			return err
		}
		b.WriteString("." + e.Sel.Name)
		return nil
	}
	return errors.New("not a name")
}

// funcBody is the body of the top-level function name in f.
func funcBody(t *testing.T, f *ast.File, name string) *ast.BlockStmt {
	t.Helper()
	for _, d := range f.Decls {
		if fd, ok := d.(*ast.FuncDecl); ok && fd.Name.Name == name {
			return fd.Body
		}
	}
	t.Fatalf("no func %s", name)
	return nil
}

// TestMain_StartsJobs: the generated main opens the app, serves its Handler and starts
// o.Jobs job workers; on shutdown it stops the workers claiming and shuts the server down at once
// (#288: no claims while requests drain; the attempts running and the requests share the 5 s),
// then stops the backup, then closes the app, and so does a failed Serve (the workers with no grace). The maintenance tasks open no App and start no worker.
func TestMain_StartsJobs(t *testing.T) {
	for _, withCaps := range []bool{false, true} {
		for _, withSilos := range []bool{false, true} {
			src := Main("todos", withCaps, withSilos)
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, "main.go", src, 0)
			if err != nil {
				t.Fatal(err)
			}
			got := calls(funcBody(t, f, "main"))
			order := []string{"jobs.WorkersFromEnv", "app.Open", "a.Handler", "a.StartJobs", "stopJobs", "srv.Shutdown", "stopBackup", "a.Close"}
			at := 0
			for _, c := range got {
				if at < len(order) && c == order[at] {
					at++
				}
			}
			if at != len(order) {
				t.Errorf("caps %v silos %v: main calls %v, want %v in that order (stopped at %s)", withCaps, withSilos, got, order, order[at])
			}
			// a.StartJobs twice: serving, and runJobs (--jobs-only)
			for c, want := range map[string]int{"app.Serve": 0, "app.Open": 1, "a.StartJobs": 2} {
				if n := strings.Count(string(src), c+"("); n != want {
					t.Errorf("caps %v silos %v: %d calls of %s", withCaps, withSilos, n, c)
				}
			}
			// a failed Serve stops the workers at once (their attempts queued again), the backup and
			// the app before it exits
			failed := "case err := <-served:\n\t\tfmt.Fprintln(os.Stderr, \"alder:\", err)\n\t\tstopJobs(0) // the attempts running are queued again for the next start\n\t\tstopBackup()\n\t\ta.Close()\n\t\tos.Exit(1)"
			readJobs := "if jobsOnly {\n\t\tworkers, err = jobs.JobsOnlyWorkersFromEnv() // ALDER_JOBS: default 8\n\t} else {\n\t\tworkers, err = jobs.WorkersFromEnv() // ALDER_JOBS: default 4\n\t}\n\tif err != nil {\n\t\tfmt.Fprintln(os.Stderr, \"alder:\", err)\n\t\tos.Exit(1)\n\t}"
			drain := "go func() {\n\t\tstopJobs(time.Until(deadline)) // stops claiming at once; the attempts running get the same 5 s as the requests\n\t\tclose(jobsStopped)\n\t}()\n\tif err := srv.Shutdown(grace); err != nil {\n\t\tfmt.Fprintln(os.Stderr, \"alder: shutting down:\", err)\n\t}\n\t<-jobsStopped"
			for _, want := range []string{failed, drain, "a.StartJobs(o.Jobs)", "Jobs:      workers,", "JobsOnly:  jobsOnly,", readJobs} {
				if !strings.Contains(string(src), want) {
					t.Errorf("caps %v silos %v: main has no %q", withCaps, withSilos, want)
				}
			}
			m := calls(funcBody(t, f, "maintain"))
			if slices.ContainsFunc(m, func(c string) bool { return strings.Contains(c, "StartJobs") || c == "app.Open" || c == "app.Serve" }) {
				t.Errorf("caps %v silos %v: maintain calls %v", withCaps, withSilos, m)
			}
		}
	}
}

// TestOptions_AlderJobs: ALDER_JOBS unset or empty is 4 workers, 0 none, N N; a value that is
// not a whole number makes the built command exit 1 with a message before it opens the app.
func TestOptions_AlderJobs(t *testing.T) {
	for v, want := range map[string]int{"": 4, "0": 0, "1": 1, " 12 ": 12} {
		t.Setenv(jobs.WorkersEnv, v)
		if n, err := jobs.WorkersFromEnv(); err != nil || n != want {
			t.Errorf("ALDER_JOBS=%q: %d, %v; want %d", v, n, err, want)
		}
	}
	os.Unsetenv(jobs.WorkersEnv)
	if n, err := jobs.WorkersFromEnv(); err != nil || n != 4 {
		t.Errorf("ALDER_JOBS unset: %d, %v; want 4", n, err)
	}
	for _, v := range []string{"x", "-1", "1.5", "65536"} {
		t.Setenv(jobs.WorkersEnv, v)
		if _, err := jobs.WorkersFromEnv(); err == nil || err.Error() != "ALDER_JOBS="+v+": want a whole number of job workers below 65536 (0: none)" {
			t.Errorf("ALDER_JOBS=%q: %v", v, err)
		}
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not on PATH")
	}
	dir := emitApp(t, "../../testdata/golden/gen_jobs")
	if err := os.MkdirAll(filepath.Join(dir, "cmd"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "cmd", "main.go"), Main("jobs", true, false), 0o644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "jobs-app")
	goRun(t, dir, "build", "-o", bin, "./cmd")
	base := []string{"ALDER_DB=:memory:", "ALDER_DEV=1", "ALDER_ADDR=127.0.0.1:0", "ALDER_PUBLIC_URL=", "ALDER_LOG=", "ALDER_LOG_FORMAT=", "ALDER_METRICS=", "ALDER_SLOW_MS=", "ALDER_BACKUP_URL="}
	for _, c := range []struct {
		v      string
		code   int
		stderr string
	}{
		{"x", 1, "alder: ALDER_JOBS=x: want a whole number of job workers below 65536 (0: none)\n"},
		{"-1", 1, "alder: ALDER_JOBS=-1: want a whole number of job workers below 65536 (0: none)\n"},
		// read, then Open refuses the stub adapter: the value got past the read
		{"0", 1, "alder: capability Email.Email is not wired: adapters/email/: func Email returned nil\n"},
	} {
		cmd := exec.Command(bin)
		cmd.Env = append(append(os.Environ(), base...), "ALDER_JOBS="+c.v)
		var stderr strings.Builder
		cmd.Stderr = &stderr
		err := cmd.Run()
		var ee *exec.ExitError
		if !errors.As(err, &ee) || ee.ExitCode() != c.code || stderr.String() != c.stderr {
			t.Errorf("ALDER_JOBS=%s: %v, stderr %q; want exit %d, %q", c.v, err, stderr.String(), c.code, c.stderr)
		}
	}
}

// TestMain_JobsOnly: `app --jobs-only` (spec §7a) reads ALDER_JOBS with the default 8, opens the
// app and, before the backup and the listener, runs runJobs: it waits out a database ahead of
// the build (a.AwaitBuild) before it starts the workers, and on SIGINT or SIGTERM stops them with
// a 5 s grace (an app with silos drains first) and closes the app. It never calls Handler.
func TestMain_JobsOnly(t *testing.T) {
	for _, withSilos := range []bool{false, true} {
		src := Main("todos", true, withSilos)
		f, err := parser.ParseFile(token.NewFileSet(), "main.go", src, 0)
		if err != nil {
			t.Fatal(err)
		}
		got := calls(funcBody(t, f, "main"))
		order := []string{"jobs.JobsOnlyWorkersFromEnv", "app.Open", "runJobs", "backup.Start", "net.Listen", "a.Handler"}
		at := 0
		for _, c := range got {
			if at < len(order) && c == order[at] {
				at++
			}
		}
		if at != len(order) {
			t.Errorf("silos %v: main calls %v, want %v in that order (stopped at %s)", withSilos, got, order, order[at])
		}
		for _, want := range []string{
			"jobsOnly := len(os.Args) == 2 && os.Args[1] == \"--jobs-only\"\n\tif len(os.Args) > 1 && !jobsOnly {\n\t\tos.Exit(maintain(os.Args[1:], dbPath, dev))\n\t}",
			"if jobsOnly {\n\t\tos.Exit(runJobs(a, o.Jobs, lg.Log))\n\t}",
			"[--jobs-only | --migrate-only | --backup-to FILE",
		} {
			if !strings.Contains(string(src), want) {
				t.Errorf("silos %v: main has no %q", withSilos, want)
			}
		}
		rj := calls(funcBody(t, f, "runJobs"))
		order = []string{"signal.NotifyContext", "a.AwaitBuild", "a.StartJobs", "stop", "stopJobs", "a.Close"}
		if withSilos {
			order = []string{"signal.NotifyContext", "a.AwaitBuild", "a.StartJobs", "stop", "db.Drain", "stopJobs", "a.Close"}
		}
		at = 0
		for _, c := range rj {
			if at < len(order) && c == order[at] {
				at++
			}
		}
		if at != len(order) || slices.ContainsFunc(rj, func(c string) bool {
			return strings.Contains(c, "Handler") || strings.Contains(c, "Listen") || strings.Contains(c, "backup")
		}) {
			t.Errorf("silos %v: runJobs calls %v, want %v in that order and no listener or backup", withSilos, rj, order)
		}
		if !strings.Contains(string(src), "stopJobs(5 * time.Second)") {
			t.Errorf("silos %v: runJobs gives the attempts no 5 s grace", withSilos)
		}
	}
}
