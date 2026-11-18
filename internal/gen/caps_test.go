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
// o.Jobs job workers; on shutdown it shuts the server down, then stops the workers, then the
// backup, then closes the app, and so does a failed Serve (the workers with no grace). The maintenance tasks open no App and start no worker.
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
			order := []string{"jobs.WorkersFromEnv", "app.Open", "a.Handler", "a.StartJobs", "srv.Shutdown", "stopJobs", "stopBackup", "a.Close"}
			at := 0
			for _, c := range got {
				if at < len(order) && c == order[at] {
					at++
				}
			}
			if at != len(order) {
				t.Errorf("caps %v silos %v: main calls %v, want %v in that order (stopped at %s)", withCaps, withSilos, got, order, order[at])
			}
			for _, c := range []string{"app.Serve", "app.Open", "a.StartJobs"} {
				if n := strings.Count(string(src), c+"("); c == "app.Serve" && n != 0 || c != "app.Serve" && n != 1 {
					t.Errorf("caps %v silos %v: %d calls of %s", withCaps, withSilos, n, c)
				}
			}
			// a failed Serve stops the workers at once (their attempts queued again), the backup and
			// the app before it exits
			failed := "case err := <-served:\n\t\tfmt.Fprintln(os.Stderr, \"alder:\", err)\n\t\tstopJobs(0) // the attempts running are queued again for the next start\n\t\tstopBackup()\n\t\ta.Close()\n\t\tos.Exit(1)"
			for _, want := range []string{failed, "a.StartJobs(o.Jobs)", "Jobs:      workers,", "workers, err := jobs.WorkersFromEnv() // ALDER_JOBS\n\tif err != nil {\n\t\tfmt.Fprintln(os.Stderr, \"alder:\", err)\n\t\tos.Exit(1)\n\t}"} {
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
