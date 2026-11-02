package alderbin

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestFind(t *testing.T) {
	lookPath := func(name string) (string, error) {
		if name == "alder" {
			return "/usr/bin/alder", nil
		}
		return "", errors.New("not found")
	}
	none := func(string) (string, error) { return "", errors.New("not found") }

	got, err := Find(func(k string) string {
		if k == "ALDER_BIN" {
			return "/opt/alder"
		}
		return ""
	}, lookPath)
	if err != nil || got != "/opt/alder" {
		t.Fatalf("ALDER_BIN: got %q, %v", got, err)
	}
	got, err = Find(func(string) string { return "" }, lookPath)
	if err != nil || got != "/usr/bin/alder" {
		t.Fatalf("PATH: got %q, %v", got, err)
	}
	_, err = Find(func(string) string { return "" }, none)
	if err == nil || err.Error() != "alder is not on PATH; install it or set ALDER_BIN" {
		t.Fatalf("missing: got %v", err)
	}
}

// A relative ALDER_BIN is made absolute, since alder run starts in the app's directory.
func TestFind_RelativeALDERBIN(t *testing.T) {
	got, err := Find(func(string) string { return "./alder" }, nil)
	if err != nil || !filepath.IsAbs(got) || filepath.Base(got) != "alder" {
		t.Fatalf("relative ALDER_BIN: got %q, %v", got, err)
	}
}

func TestVersion(t *testing.T) {
	var gotArgs []string
	run := func(_ context.Context, args ...string) ([]byte, error) {
		gotArgs = args
		return []byte(`{"version":"v0.6.0","commands":[]}`), nil
	}
	v, err := Version(context.Background(), run)
	if err != nil || v != "v0.6.0" {
		t.Fatalf("got %q, %v", v, err)
	}
	if strings.Join(gotArgs, " ") != "help --json" {
		t.Fatalf("args %v", gotArgs)
	}
	bad := func(context.Context, ...string) ([]byte, error) { return []byte("not json"), nil }
	if _, err := Version(context.Background(), bad); err == nil {
		t.Fatal("invalid JSON: want an error")
	}
	failing := func(context.Context, ...string) ([]byte, error) { return nil, errors.New("boom") }
	if _, err := Version(context.Background(), failing); err == nil {
		t.Fatal("run error: want an error")
	}
}

func TestMismatch(t *testing.T) {
	cases := []struct{ tui, alder, want string }{
		{"v0.6.0", "v0.6.0", ""},
		{"(devel)", "v0.6.0", ""},
		{"v0.6.0", "(devel)", ""},
		{"v0.6.0", "v0.5.0", "alder v0.5.0 ≠ tui v0.6.0"},
	}
	for _, c := range cases {
		if got := Mismatch(c.tui, c.alder); got != c.want {
			t.Errorf("Mismatch(%q, %q) = %q, want %q", c.tui, c.alder, got, c.want)
		}
	}
}

func TestVersion_Missing(t *testing.T) {
	run := func(context.Context, ...string) ([]byte, error) { return []byte(`{"commands":[]}`), nil }
	v, err := Version(context.Background(), run)
	if err == nil || !strings.Contains(err.Error(), "version") {
		t.Fatalf("got %q, %v; want an error that mentions version", v, err)
	}
}

// A relative PATH entry gives a path relative to the working directory; Find makes it absolute.
func TestFind_RelativePath(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "rel"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "rel", "alder"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	t.Setenv("PATH", "rel")
	got, err := Find(func(string) string { return "" }, exec.LookPath)
	if err != nil || !filepath.IsAbs(got) || filepath.Base(got) != "alder" {
		t.Fatalf("got %q, %v; want an absolute path to alder", got, err)
	}
}

func TestMismatch_Short(t *testing.T) {
	got := Mismatch("0.5.0", "0.6.0")
	if got != "alder 0.6.0 ≠ tui 0.5.0" {
		t.Fatalf("got %q", got)
	}
	if n := utf8.RuneCountInString(got); n > 30 {
		t.Fatalf("%d cols, want at most 30", n)
	}
}
