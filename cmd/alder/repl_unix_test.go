//go:build unix

package main

import (
	"errors"
	"io"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// replEval starts `alder repl examples/todos` as a process in a fresh directory, has it run a
// task that runs for minutes, and returns the process and the path of the scratch
// program once that program is running.
func replEval(t *testing.T) (*exec.Cmd, string) {
	t.Helper()
	needGoTool(t)
	bin := alderBinary(t)
	work := t.TempDir()
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
	// computed inside main (in eff.Run), not while the generated package initializes: a value
	// computed at package initialization runs before any main code, the tether included
	io.WriteString(stdin, `:run! (Task.map (\n -> List.range n 100000 |> List.map (\i -> List.range 1 100000 |> List.length) |> List.sum) (Task.succeed 1))`+"\n")
	// the executable's path as the REPL resolved it (macOS's /var is /private/var), by a part
	// unique to this test
	prog := filepath.Join(filepath.Base(filepath.Dir(work)), filepath.Base(work), "build", filepath.Base(todosDir(t)), "scratch", "repl", "scratch")
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
	if testing.Short() {
		t.Skip("spawns processes")
	}
	cmd, prog := replEval(t)
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
	wantGone(t, prog, 2*time.Second)
}

// SIGTERM stops the evaluation and the REPL, which exits 1.
func TestRepl_SIGTERM(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns processes")
	}
	cmd, prog := replEval(t)
	_ = cmd.Process.Signal(syscall.SIGTERM)
	err := cmd.Wait()
	var ee *exec.ExitError
	if !errors.As(err, &ee) || ee.ExitCode() != 1 {
		t.Errorf("after SIGTERM: %v, want exit 1", err)
	}
	wantGone(t, prog, 2*time.Second)
}
