package buildkit

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

// WriteRuntime writes the embedded runtime as the module alder in dst/rt (go.mod and go.sum at
// its root, the packages under rt/rt/) and dst/go.sum; a second call refreshes it (RF1): a stray
// file, a stray directory and a file an older compiler wrote are removed, an edited file is
// restored, and files already right are left as they are.
func TestWriteRuntime(t *testing.T) {
	dst := t.TempDir()
	if err := WriteRuntime(dst); err != nil {
		t.Fatal(err)
	}
	gomod, err := os.ReadFile(filepath.Join(dst, "rt", "go.mod"))
	if err != nil || !strings.HasPrefix(string(gomod), "module alder\n") {
		t.Fatalf("rt/go.mod = %q, %v", gomod, err)
	}
	for _, f := range []string{"rt/go.sum", "go.sum", "rt/rt/core.go", "rt/rt/html/html.go", "rt/rt/datastar/datastar.js", "rt/rt/selector/selector.go"} {
		if _, err := os.Stat(filepath.Join(dst, f)); err != nil {
			t.Errorf("%s: %v", f, err)
		}
	}
	for _, f := range []string{"rt/rt/embed.go", "rt/rt/core_test.go", "rt/rt/dist", "rt/rt/html/testdata"} {
		if _, err := os.Stat(filepath.Join(dst, f)); err == nil {
			t.Errorf("%s was written", f)
		}
	}
	core := filepath.Join(dst, "rt", "rt", "core.go")
	want, _ := os.ReadFile(core)
	stamp := time.Now().Add(-time.Hour).Truncate(time.Second)
	html := filepath.Join(dst, "rt", "rt", "html", "html.go")
	if err := os.Chtimes(html, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	for f, b := range map[string]string{
		"rt/rt/stray.go":         "package rt\n\nfunc Old() {}\n", // a file an older runtime had
		"rt/rt/gone/gone.go":     "package gone\n",                // a package it had
		"rt/rt/html/notes.txt":   "x",
		"rt/rt/core.go":          "package rt\n", // edited
		"rt/rt/web/old/deep.go":  "package old\n",
		"rt/rt/datastar/old.js":  "x",
		"rt/README-of-someone":   "x",
		"rt/rt/std/zz_extra.go":  "package std\n",
		"rt/rt/testing/extra.go": "package testing\n",
	} {
		path := filepath.Join(dst, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(b), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := WriteRuntime(dst); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"rt/rt/stray.go", "rt/rt/gone", "rt/rt/html/notes.txt", "rt/rt/web/old", "rt/rt/datastar/old.js", "rt/README-of-someone", "rt/rt/std/zz_extra.go", "rt/rt/testing/extra.go"} {
		if _, err := os.Stat(filepath.Join(dst, filepath.FromSlash(f))); err == nil {
			t.Errorf("%s survived the refresh", f)
		}
	}
	if got, _ := os.ReadFile(core); string(got) != string(want) {
		t.Errorf("rt/rt/core.go was not restored")
	}
	if st, err := os.Stat(html); err != nil || !st.ModTime().Equal(stamp) {
		t.Errorf("rt/rt/html/html.go was rewritten although it was right (%v)", err)
	}
}
