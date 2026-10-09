// cmd/alder/main_test.go
package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func run(t *testing.T, args ...string) (int, string, string) {
	var out, errb bytes.Buffer
	code := runCheckTo(args, &out, &errb)
	return code, out.String(), errb.String()
}

func TestCLI_ExampleClean(t *testing.T) {
	code, out, errs := run(t, "../../examples/todos")
	if code != 0 || out != "" || errs != "" {
		t.Fatalf("exit %d, out: %s, err: %s", code, out, errs)
	}
}

func TestCLI_JSONAndExit(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "app"), 0o755)
	os.WriteFile(filepath.Join(dir, "app", "X.alder"), []byte("module X\n\nx : Int\nx = \"s\"\n"), 0o644)
	code, out, _ := run(t, dir, "--json")
	if code != 1 {
		t.Fatalf("exit %d", code)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 1 || !strings.HasPrefix(lines[0], `{"code":"E0003"`) {
		t.Fatalf("json: %q", out)
	}
}

func TestCLI_NoFiles(t *testing.T) {
	dir := t.TempDir()
	code, out, errs := run(t, dir, "--json")
	if code != 2 || out != "" || !strings.Contains(errs, "no .alder files") {
		t.Fatalf("exit %d out %q err %q", code, out, errs)
	}
}

func TestCLI_TextFormat(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "app"), 0o755)
	os.WriteFile(filepath.Join(dir, "app", "X.alder"), []byte("module X\n\nx : Text\nx = 1\n"), 0o644)
	code, out, _ := run(t, dir)
	if code != 1 || !strings.Contains(out, "app/X.alder:4:5: error[E0003]: expected Text, got Int") || !strings.Contains(out, "fix: Text.fromInt (1)") {
		t.Fatalf("text: %s", out)
	}
}

func TestCLI_UnreadableFile(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can read anything")
	}
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "app"), 0o755)
	path := filepath.Join(dir, "app", "X.alder")
	os.WriteFile(path, []byte("module X\n"), 0o644)
	os.Chmod(path, 0)
	defer os.Chmod(path, 0o644)
	code, out, errs := run(t, dir)
	if code != 2 || out != "" || !strings.Contains(errs, "X.alder") {
		t.Fatalf("exit %d out %q err %q", code, out, errs)
	}
}

func TestCLI_DeepFile(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "app", "A", "B", "C"), 0o755)
	os.WriteFile(filepath.Join(dir, "app", "A", "B", "C", "D.alder"), []byte("module A.B.C.D\n"), 0o644)
	code, _, errs := run(t, dir)
	if code == 2 {
		t.Fatalf("deep file not found: %s", errs)
	}
}

func TestCLI_BadFlag(t *testing.T) {
	for _, args := range [][]string{{"--jsno"}, {"-x", "."}, {"a", "b"}} {
		code, out, errs := run(t, args...)
		if code != 2 || out != "" || !strings.Contains(errs, "usage") {
			t.Fatalf("%v: exit %d out %q err %q", args, code, out, errs)
		}
	}
}
