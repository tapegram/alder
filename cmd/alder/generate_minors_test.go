package main

import (
	"slices"
	"strings"
	"testing"

	"alder/internal/generate"
)

// #35: help --json exposes which of a generator's args are required: `new page`'s module is
// optional, its path is not.
func TestHelpJSON_RequiredArgs(t *testing.T) {
	t.Parallel()
	for _, s := range generate.Specs() {
		group, sub, _ := strings.Cut(s.ID, " ")
		for _, c := range commandTree {
			for _, n := range c.Subcommands {
				if c.Name != group || n.Name != sub {
					continue
				}
				var want []bool
				for _, a := range s.Args {
					want = append(want, a.Required)
				}
				if !slices.Contains(want, true) {
					want = nil
				}
				if !slices.Equal(n.Required, want) {
					t.Errorf("%s: required %v, want %v", s.ID, n.Required, want)
				}
			}
		}
	}
}

// #35: `alder new page --list T --show U` is a usage error (exit 2), not a silent --list.
func TestGenerate_NewPageListAndShowConflict(t *testing.T) {
	t.Parallel()
	needGoTool(t)
	dir := genApp(t, "scaffold")
	code, out, errs := alderIn(t, dir, "", "new", "page", "/both", "--list", "Entry", "--show", "Entry")
	if code != 2 || !strings.Contains(errs, "--list and --show") || !strings.Contains(errs, "usage: alder new page") {
		t.Errorf("exit %d\n%s%s", code, out, errs)
	}
}
