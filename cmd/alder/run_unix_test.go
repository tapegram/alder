//go:build unix

package main

import (
	"bufio"
	"io"
	"net/http"
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

// A child that exits on its own shows on the feed as an app event with its exit status and the
// end of its output.
func TestRun_FeedAppExited(t *testing.T) {
	t.Parallel()
	r := startRun(t)
	lines := liveEvents(t, r)
	r.d.mu.Lock()
	pid := r.d.child.cmd.Process.Pid
	r.d.mu.Unlock()
	if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	e := lifecycleEvents(t, lines, 1, 30*time.Second)[0]
	if !isEvent(e, "app", "exited") {
		t.Fatalf("after the kill: %v, want app exited", e)
	}
	if exit, _ := e["exit"].(map[string]any); exit["signal"] != "killed" || exit["code"] != float64(-1) {
		t.Fatalf("exit = %v, want signal killed, code -1", e["exit"])
	}
	if tail, _ := e["tail"].([]any); len(tail) == 0 { // the app logs its migrations on stderr at start
		t.Fatalf("app exited has no tail: %v", e)
	}
}

// With ALDER_EXIT_ON_STDIN_EOF=1 (a TUI that owns alder run sets it), alder run stops as on
// Ctrl-C when its stdin reaches end of file, so it never outlives its owner, whose death (even
// by SIGKILL) closes the pipe; it stops its app first.
func TestRun_ExitsOnStdinEOF(t *testing.T) {
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
	cmd.Env = append(os.Environ(), "ALDER_ADDR=127.0.0.1:0", "ALDER_EXIT_ON_STDIN_EOF=1")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan error, 1)
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	waitForLine(t, out, "dev server on ", 120*time.Second)
	childPID := childOf(t, cmd.Process.Pid)
	t.Cleanup(func() { _ = syscall.Kill(childPID, syscall.SIGKILL) }) // when the test fails
	go func() { exited <- cmd.Wait() }()
	if err := stdin.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-exited:
		if err != nil {
			t.Fatalf("alder run after its stdin closed: %v, want exit 0", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("alder run still running 15 s after its stdin closed")
	}
	if processAlive(childPID) {
		t.Fatalf("the app (pid %d) outlived alder run", childPID)
	}
}

// A restart whose child does not start leaves the build state failed; the next restart that
// comes up makes it ok again (a build ok event without ms, then app started), as the overlay
// clears.
func TestRun_RestartAfterFailedStart(t *testing.T) {
	t.Parallel()
	r := startRun(t)
	lines := liveEvents(t, r)
	bin := r.d.binary // written before the announcement startRun waited for
	restart := func() {
		t.Helper()
		res, err := http.Post(r.base+"/_dev/restart", "", nil)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusAccepted {
			t.Fatalf("POST /_dev/restart = %d, want 202", res.StatusCode)
		}
	}
	// the running child keeps its executable (the inode) when the path is renamed away
	if err := os.Rename(bin, bin+".real"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, []byte("#!/bin/sh\necho broken >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	restart()
	if e := lifecycleEvents(t, lines, 1, 30*time.Second)[0]; !isEvent(e, "build", "failed") || e["text"] == nil {
		t.Fatalf("a restart whose child does not start: %v, want build failed with text", e)
	}
	if b := nextHello(t, r); b != "failed" {
		t.Fatalf("hello.build after the failed restart = %q, want failed", b)
	}
	if err := os.Rename(bin+".real", bin); err != nil {
		t.Fatal(err)
	}
	restart()
	got := lifecycleEvents(t, lines, 2, 30*time.Second)
	if !isEvent(got[0], "build", "ok") || !isEvent(got[1], "app", "started") || got[0]["child"] != got[1]["child"] {
		t.Fatalf("the restart that comes up: %v, want build ok then app started", got)
	}
	if b := nextHello(t, r); b != "ok" {
		t.Fatalf("hello.build after the good restart = %q, want ok", b)
	}
}
