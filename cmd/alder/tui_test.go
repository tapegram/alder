package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"alder/internal/version"
)

// fakeTui writes a script that prints its arguments and $ALDER_BIN and exits 3.
func fakeTui(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("needs a shell script")
	}
	p := filepath.Join(t.TempDir(), "alder-tui")
	if err := os.WriteFile(p, []byte("#!/bin/sh\necho \"args: $@\"\necho \"bin: $ALDER_BIN\"\nexit 3\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func noEnv(string) string { return "" }

func TestTui_Missing(t *testing.T) {
	var out, errb bytes.Buffer
	code := runTuiWith(nil, strings.NewReader(""), &out, &errb, noEnv,
		func(string) (string, error) { return "", errors.New("not found") })
	if code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	want := "alder tui: alder-tui is not installed; brew upgrade alder, or download it from the release that matches alder " + version.String() + "\n"
	if errb.String() != want {
		t.Fatalf("stderr %q, want %q", errb.String(), want)
	}
}

func TestTui_Found(t *testing.T) {
	script := fakeTui(t)
	var out, errb bytes.Buffer
	code := runTuiWith([]string{"--addr", "x"}, strings.NewReader(""), &out, &errb, noEnv,
		func(string) (string, error) { return script, nil })
	if code != 3 {
		t.Fatalf("exit %d, want 3 (stderr %q)", code, errb.String())
	}
	if !strings.Contains(out.String(), "args: --addr x\n") {
		t.Errorf("stdout %q lacks the arguments", out.String())
	}
	if strings.TrimSpace(strings.TrimPrefix(out.String()[strings.Index(out.String(), "bin:"):], "bin:")) == "" {
		t.Errorf("stdout %q: ALDER_BIN is empty", out.String())
	}
}

func TestTui_Override(t *testing.T) {
	script := fakeTui(t)
	var out, errb bytes.Buffer
	code := runTuiWith(nil, strings.NewReader(""), &out, &errb,
		func(k string) string {
			if k == "ALDER_TUI" {
				return script
			}
			return ""
		},
		func(string) (string, error) { return "", errors.New("lookPath must not be used") })
	if code != 3 {
		t.Fatalf("exit %d, want 3 (stderr %q)", code, errb.String())
	}
}
