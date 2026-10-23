//go:build unix

package main

import (
	"bufio"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestRun_ChildExitsWhenParentDies: a dev server killed with SIGKILL (a harness timeout) takes
// the app with it: the child reads EOF on its stdin pipe and shuts down.
func TestRun_ChildExitsWhenParentDies(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("spawns processes")
	}
	needGoTool(t)
	bin := alderBinary(t)
	work := t.TempDir()
	copyDir(t, todosDir(t), filepath.Join(work, "todos"))
	cmd := exec.Command(bin, "run", "todos")
	cmd.Dir = work
	cmd.Env = append(os.Environ(), "ALDER_ADDR=127.0.0.1:0")
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _, _ = cmd.Process.Wait() })
	// alder run announces `dev server on <addr> → app on <child>` once the app is listening
	line := waitForLine(t, out, "dev server on ", 120*time.Second)
	childPID := childOf(t, cmd.Process.Pid)
	t.Cleanup(func() { _ = syscall.Kill(childPID, syscall.SIGKILL) }) // when the test fails
	_ = cmd.Process.Kill()                                            // SIGKILL: no cleanup runs in the parent
	_, _ = cmd.Process.Wait()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if !processAlive(childPID) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("the app (pid %d) is still running 3 s after its dev server was killed (%s)", childPID, line)
}

// alderBinary builds cmd/alder into a temporary directory and returns the executable's path.
func alderBinary(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "alder")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return bin
}

// waitForLine reads r line by line until a line starts with prefix and returns it; it fails
// when r ends first or no such line comes within timeout. The rest of r is drained in the
// background, so the writer never blocks.
func waitForLine(t *testing.T, r io.Reader, prefix string, timeout time.Duration) string {
	t.Helper()
	found := make(chan string, 1)
	go func() {
		sc := bufio.NewScanner(r)
		for sc.Scan() {
			if strings.HasPrefix(sc.Text(), prefix) {
				found <- sc.Text()
				break
			}
		}
		close(found)
		_, _ = io.Copy(io.Discard, r)
	}()
	select {
	case line, ok := <-found:
		if !ok {
			t.Fatalf("output ended without a line starting with %q", prefix)
		}
		return line
	case <-time.After(timeout):
		t.Fatalf("no line starting with %q within %v", prefix, timeout)
	}
	return ""
}

// childOf is the PID of pid's one child process (pgrep -P).
func childOf(t *testing.T, pid int) int {
	t.Helper()
	out, err := exec.Command("pgrep", "-P", strconv.Itoa(pid)).Output()
	if err != nil {
		t.Fatalf("pgrep -P %d: %v", pid, err)
	}
	fields := strings.Fields(string(out))
	if len(fields) != 1 {
		t.Fatalf("pid %d has %d children (%q), want 1", pid, len(fields), out)
	}
	child, err := strconv.Atoi(fields[0])
	if err != nil {
		t.Fatal(err)
	}
	return child
}

// processAlive reports whether a process with this PID exists (signal 0). A zombie counts as
// gone only once it is reaped; the app's parent is dead, so init reaps it at once.
func processAlive(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}

// The production server ignores its stdin: without ALDER_EXIT_ON_STDIN_EOF (which only alder run
// sets) a stdin at end of file from the start (/dev/null, as under a service manager) does not
// stop it.
func TestServer_IgnoresStdinEOF(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("spawns processes")
	}
	needGoTool(t)
	work := t.TempDir()
	var errb strings.Builder
	bin, code := buildApp(todosDir(t), filepath.Join(work, "build", "todos"), &errb)
	if code != 0 {
		t.Fatalf("build: exit %d\n%s", code, errb.String())
	}
	devNull, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer devNull.Close()
	cmd := exec.Command(bin)
	cmd.Dir = work
	cmd.Stdin = devNull
	var env []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "ALDER_") {
			env = append(env, kv)
		}
	}
	cmd.Env = append(env, "ALDER_ADDR=127.0.0.1:0", "ALDER_DEV=1", "ALDER_DB="+filepath.Join(work, "t.db"))
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan error, 1)
	t.Cleanup(func() { _ = cmd.Process.Kill(); <-exited })
	waitForLine(t, out, "listening on ", 30*time.Second)
	go func() { exited <- cmd.Wait() }()
	select {
	case err := <-exited:
		exited <- err
		t.Fatalf("the server stopped at stdin's end of file: %v", err)
	case <-time.After(time.Second):
	}
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-exited:
		exited <- err
		if err != nil {
			t.Errorf("after SIGTERM: %v, want exit 0", err)
		}
	case <-time.After(10 * time.Second):
		t.Error("the server did not stop on SIGTERM")
	}
}
