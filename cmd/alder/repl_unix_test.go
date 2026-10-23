//go:build unix

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// replEval starts `alder repl examples/todos` as a process in the directory work (a fresh one
// when ""), has it evaluate a pure expression that runs for minutes, and returns the process and
// the path of the scratch program once that program is running.
func replEval(t *testing.T, bin, work string) (*exec.Cmd, string) {
	t.Helper()
	needGoTool(t)
	if bin == "" {
		bin = alderBinary(t)
	}
	if work == "" {
		work = t.TempDir()
	}
	cmd := exec.Command(bin, "repl", todosDir(t))
	cmd.Dir = work
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	// a pure value: the generated package computes it while it initializes, before the main
	// runs (rt's init tethers it all the same)
	io.WriteString(stdin, longEval+"\n")
	// the executable's path as the REPL resolved it (macOS's /var is /private/var), by a part
	// unique to this test
	prog := filepath.Join(filepath.Base(filepath.Dir(work)), filepath.Base(work), "build", filepath.Base(todosDir(t)), "scratch", fmt.Sprintf("repl-%d", cmd.Process.Pid), "scratch")
	t.Cleanup(func() { _ = exec.Command("pkill", "-9", "-f", prog).Run() }) // when the test fails
	deadline := time.Now().Add(60 * time.Second)
	for !scratchRunning(prog) {
		if time.Now().After(deadline) {
			t.Fatalf("%s did not start within 60 s", prog)
		}
		time.Sleep(50 * time.Millisecond)
	}
	return cmd, prog
}

// scratchRunning reports whether a process runs the executable prog (pgrep -f).
func scratchRunning(prog string) bool {
	return exec.Command("pgrep", "-f", prog).Run() == nil
}

// wantGone fails unless no process runs prog within d.
func wantGone(t *testing.T, prog string, d time.Duration) {
	t.Helper()
	deadline := time.Now().Add(d)
	for scratchRunning(prog) {
		if time.Now().After(deadline) {
			t.Fatalf("%s still runs %v after the REPL ended", prog, d)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// A REPL killed outright (SIGKILL: nothing runs in it) leaves no evaluation running: the
// program reads end of file on the stdin pipe only the REPL held.
func TestRepl_KillLeavesNoOrphan(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("spawns processes")
	}
	cmd, prog := replEval(t, "", "")
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
	wantGone(t, prog, 2*time.Second)
}

// SIGTERM, and SIGHUP (the terminal closed), stop the evaluation and the REPL, which exits 1
// and removes its build directory.
func TestRepl_SIGTERMAndSIGHUP(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("spawns processes")
	}
	bin := alderBinary(t)
	for _, sig := range []syscall.Signal{syscall.SIGTERM, syscall.SIGHUP} {
		t.Run(sig.String(), func(t *testing.T) {
			work := t.TempDir()
			cmd, prog := replEval(t, bin, work)
			_ = cmd.Process.Signal(sig)
			err := cmd.Wait()
			var ee *exec.ExitError
			if !errors.As(err, &ee) || ee.ExitCode() != 1 {
				t.Errorf("after %v: %v, want exit 1", sig, err)
			}
			wantGone(t, prog, 2*time.Second)
			dir := filepath.Join(work, "build", "todos", "scratch", fmt.Sprintf("repl-%d", cmd.Process.Pid))
			if _, err := os.Stat(dir); !os.IsNotExist(err) {
				t.Errorf("%s after %v: %v, want removed", dir, sig, err)
			}
		})
	}
}

// Two REPLs on one app at once (one busy evaluating) do not wait for each other: each builds in
// a directory of its own.
func TestRepl_TwoAtOnce(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("spawns processes")
	}
	bin := alderBinary(t)
	work := t.TempDir()
	busy, prog := replEval(t, bin, work)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second) // waiting would be minutes
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "repl", todosDir(t))
	cmd.Dir = work
	cmd.Stdin = strings.NewReader("1 + 1\n")
	start := time.Now()
	out, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(out), "2 : Int\n") {
		t.Fatalf("the second REPL: %v\n%s", err, out)
	}
	if d := time.Since(start); d > 30*time.Second {
		t.Errorf("the second REPL took %v", d)
	}
	if !scratchRunning(prog) || busy.ProcessState != nil {
		t.Error("the first REPL's evaluation ended")
	}
}
