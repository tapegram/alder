package alderbin

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
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
		{"v0.6.0", "v0.5.0", "alder-tui v0.6.0 with alder v0.5.0: upgrade both together"},
	}
	for _, c := range cases {
		if got := Mismatch(c.tui, c.alder); got != c.want {
			t.Errorf("Mismatch(%q, %q) = %q, want %q", c.tui, c.alder, got, c.want)
		}
	}
}
