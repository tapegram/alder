// cmd/alder/main_test.go
package main

import (
	"bytes"
	"fmt"
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
	t.Parallel()
	code, out, errs := run(t, "../../examples/todos")
	if code != 0 || out != "" || errs != "" {
		t.Fatalf("exit %d, out: %s, err: %s", code, out, errs)
	}
}

func TestCLI_JSONAndExit(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	dir := t.TempDir()
	code, out, errs := run(t, dir, "--json")
	if code != 2 || out != "" || !strings.Contains(errs, "no .alder files") {
		t.Fatalf("exit %d out %q err %q", code, out, errs)
	}
}

func TestCLI_TextFormat(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "app"), 0o755)
	os.WriteFile(filepath.Join(dir, "app", "X.alder"), []byte("module X\n\nx : Text\nx = 1\n"), 0o644)
	code, out, _ := run(t, dir)
	if code != 1 || !strings.Contains(out, "app/X.alder:4:5: error[E0003]: expected Text, got Int") || !strings.Contains(out, "fix: Text.fromInt (1)") {
		t.Fatalf("text: %s", out)
	}
}

func TestCLI_UnreadableFile(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "app", "A", "B", "C"), 0o755)
	os.WriteFile(filepath.Join(dir, "app", "A", "B", "C", "D.alder"), []byte("module A.B.C.D\n"), 0o644)
	code, _, errs := run(t, dir)
	if code == 2 {
		t.Fatalf("deep file not found: %s", errs)
	}
}

func TestCLI_BadFlag(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{{"--jsno"}, {"-x", "."}, {"a", "b"}} {
		code, out, errs := run(t, args...)
		if code != 2 || out != "" || !strings.Contains(errs, "usage") {
			t.Fatalf("%v: exit %d out %q err %q", args, code, out, errs)
		}
	}
}

// TestCLI_HoleSpanJSON: W0002's span covers the whole hole, `?` and name.
func TestCLI_HoleSpanJSON(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "app"), 0o755)
	os.WriteFile(filepath.Join(dir, "app", "X.alder"), []byte("module X\n\nx : Int\nx = 1 + ?count\n"), 0o644)
	code, out, _ := run(t, dir, "--json")
	want := `{"code":"W0002","severity":"warning","file":"app/X.alder","line":4,"col":9,"endLine":4,"endCol":15,"message":"hole ?count has type Int"}`
	if code != 0 || strings.TrimSpace(out) != want {
		t.Errorf("exit %d\n%s\nwant\n%s", code, out, want)
	}
}

// TestLsp_InitializeOverStdio: `alder lsp` answers initialize on stdout and stops at exit.
func TestLsp_InitializeOverStdio(t *testing.T) {
	t.Parallel()
	frame := func(body string) string { return fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(body), body) }
	in := frame(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`) +
		frame(`{"jsonrpc":"2.0","method":"exit"}`)
	var out, errb bytes.Buffer
	if code := runLspTo(nil, strings.NewReader(in), &out, &errb); code != 0 {
		t.Fatalf("code %d: %s", code, errb.String())
	}
	if !strings.Contains(out.String(), `"positionEncoding":"utf-8"`) {
		t.Fatalf("out %s", out.String())
	}
	if code := runLspTo([]string{"x"}, strings.NewReader(""), &out, &errb); code != 2 || !strings.Contains(errb.String(), "usage: alder lsp") {
		t.Fatalf("an argument: exit %d, %s", code, errb.String())
	}
}

// inTemp makes a fresh temporary directory the working directory. Only the tests that still
// need one (the example tests, which name their app by a relative path) use it; they run
// serially.
func inTemp(t *testing.T) { t.Chdir(t.TempDir()) }
