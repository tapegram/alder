// Package alderbin finds the alder binary the TUI drives and checks that it is the version the
// TUI was built with.
package alderbin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// Find returns the path of the alder binary: ALDER_BIN when set, else alder on PATH.
func Find(getenv func(string) string, lookPath func(string) (string, error)) (string, error) {
	if p := getenv("ALDER_BIN"); p != "" {
		return p, nil
	}
	p, err := lookPath("alder")
	if err != nil {
		return "", errors.New("alder is not on PATH; install it or set ALDER_BIN")
	}
	return p, nil
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
	return help.Version, nil
}

// Mismatch is the warning to show when alder-tui and alder are different versions, and "" when
// they are the same or either is a development build (version "(devel)").
func Mismatch(tui, alder string) string {
	if tui == alder || tui == "(devel)" || alder == "(devel)" {
		return ""
	}
	return fmt.Sprintf("alder-tui %s with alder %s: upgrade both together", tui, alder)
}
