package buildkit

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// A go command cancelled mid-build (killed) leaves no work directory: GoCmdContext gives it a
// GOTMPDIR of its own and removes it.
func TestGoCmdContext_CancelLeavesNoTemp(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("no go tool")
	}
	if testing.Short() {
		t.Skip("runs go build")
	}
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp) // os.TempDir, where GoCmdContext makes its GOTMPDIR
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module m\n\ngo 1.21\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nimport \"net/http\"\n\nfunc main() { _ = http.ListenAndServe }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := GoCmdContext(ctx, dir, "build", "-a", "-o", "x", ".") // -a: long enough to cancel mid-way
		done <- err
	}()
	glob := func(p string) []string {
		m, _ := filepath.Glob(filepath.Join(tmp, p))
		return m
	}
	deadline := time.Now().Add(30 * time.Second)
	for len(glob("alder-go-*/go-build*")) == 0 { // go has made its work directory
		select {
		case <-done:
			t.Fatal("go build finished before it could be cancelled")
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("go build made no work directory")
		}
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(500 * time.Millisecond) // compiling
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("the cancelled build succeeded")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the cancelled go build did not return")
	}
	if left := glob("*"); len(left) != 0 {
		t.Errorf("left behind: %v", left)
	}
}
