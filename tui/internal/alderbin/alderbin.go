// Package alderbin finds the alder binary the TUI drives and checks that it is the version the
// TUI was built with.
package alderbin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
)

// Find returns the path of the alder binary: ALDER_BIN when set, else alder on PATH. The path is
// made absolute, so that it names the same file from the working directory of a child.
func Find(getenv func(string) string, lookPath func(string) (string, error)) (string, error) {
	if p := getenv("ALDER_BIN"); p != "" {
		return filepath.Abs(p)
	}
	p, err := lookPath("alder")
	// A relative PATH entry finds alder with exec.ErrDot and a path relative to the working
	// directory; it is made absolute below, as a child may run elsewhere.
	if err != nil && !(errors.Is(err, exec.ErrDot) && p != "") {
		return "", errors.New("alder is not on PATH; install it or set ALDER_BIN")
	}
	return filepath.Abs(p)
}

// Version runs `alder help --json` through run and returns the version it reports.
func Version(ctx context.Context, run func(ctx context.Context, args ...string) ([]byte, error)) (string, error) {
	out, err := run(ctx, "help", "--json")
	if err != nil {
		return "", fmt.Errorf("alder help --json: %w", err)
	}
	var help struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(out, &help); err != nil {
		return "", fmt.Errorf("alder help --json: %w", err)
	}
	if help.Version == "" {
		return "", errors.New("alder help --json: alder reports no version")
	}
	return help.Version, nil
}

// Mismatch is the short warning (`alder 0.6.0 ≠ tui 0.5.0`) to show when alder-tui and alder are different versions, and "" when
// they are the same or either is a development build (version "(devel)").
func Mismatch(tui, alder string) string {
	if tui == alder || tui == "(devel)" || alder == "(devel)" {
		return ""
	}
	return fmt.Sprintf("alder %s ≠ tui %s", alder, tui)
}
