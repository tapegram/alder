package main

import (
	"fmt"
	"io"

	"alder/internal/generate"
)

const regenUsage = "usage: alder regen admin [--module Admin] [--path /admin] [--all] [--dir app] [--json] [--dry-run]"

// runRegenTo is `alder regen admin`: the admin regenerated from the app as it is now, and only
// its owned regions (`-- @admin begin <name>` … `-- @admin end`) written back
// (generate.RegenAdmin). The text outside the regions, the author's, is left byte for byte; a
// file the author deleted is not recreated unless --all (a table added since gets its pages).
// A second run has nothing to do. A file whose markers are broken is refused, nothing written.
func runRegenTo(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "admin" {
		fmt.Fprintln(stderr, regenUsage)
		return 2
	}
	g, err := parseGenArgs(args[1:], []string{"module", "path"}, nil, []string{"all"})
	if err != nil || len(g.pos) != 0 {
		return genUsage(stderr, regenUsage, err)
	}
	o := generate.AdminOptions{Module: g.flag("module"), Path: g.flag("path"), Version: adminVersion()}
	var skipped []string
	nothing := false
	code := runGenerator("alder regen admin", g, generate.Options{}, func(p *generate.Program) ([]generate.Edit, error) {
		edits, sk, err := generate.RegenAdmin(p, o, g.has("all"))
		skipped = sk
		if err == nil && len(edits) == 0 {
			nothing = true
		}
		return edits, err
	}, stdout, stderr)
	if code == 0 && !g.json {
		for _, s := range skipped {
			fmt.Fprintf(stdout, "not recreated %s (deleted; --all writes it again)\n", s)
		}
		if nothing {
			fmt.Fprintln(stdout, "nothing to do: the admin is up to date")
		}
	}
	return code
}
