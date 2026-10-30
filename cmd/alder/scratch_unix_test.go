//go:build unix

package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// Ctrl-C on a running `alder scratch` stops it and the program it runs, with exit 1 and no
// results: a real SIGINT to a child alder process, not an already-cancelled context.
func TestScratchInterruptSignal(t *testing.T) {
	t.Parallel()
	needGoTool(t)
	work := t.TempDir()
	bin := filepath.Join(work, "alder")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	app := filepath.Join(work, "todos")
	copyDir(t, todosDir(t), app)
	os.RemoveAll(filepath.Join(app, "build"))
	file := filepath.Join(app, "scratch.alder")
	writeScratch(t, file, "> List.foldl (\\x acc -> acc + List.length (List.range 1 100000)) 0 (List.range 1 1000000)\n")
	buildDir := filepath.Join(work, "b")
	cmd := exec.Command(bin, "scratch", "--build-dir", buildDir, file)
	cmd.Dir = work
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	programs := "^" + filepath.Join(buildDir, "todos", "scratch") + "/"
	running := func() bool { return exec.Command("pgrep", "-f", programs).Run() == nil }
	deadline := time.Now().Add(2 * time.Minute)
	for !running() {
		if time.Now().After(deadline) {
			cmd.Process.Kill()
			t.Fatalf("the program never started\nstderr: %s", errb.String())
		}
		time.Sleep(100 * time.Millisecond)
	}
	start := time.Now()
	if err := cmd.Process.Signal(syscall.SIGINT); err != nil {
		t.Fatal(err)
	}
	err := cmd.Wait()
	if took := time.Since(start); took > 5*time.Second {
		t.Errorf("scratch took %v to stop", took)
	}
	if ee, ok := err.(*exec.ExitError); !ok || ee.ExitCode() != 1 || out.String() != "" || !strings.Contains(errb.String(), "alder scratch: interrupted") {
		t.Errorf("scratch: %v\nstdout: %s\nstderr: %s", err, out.String(), errb.String())
	}
	if running() {
		exec.Command("pkill", "-f", programs).Run()
		t.Error("the program outlived scratch")
	}
}
