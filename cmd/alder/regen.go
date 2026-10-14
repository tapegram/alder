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
// Errors in the admin's own files do not refuse it (generate.LoadExcept): a field renamed or
// dropped breaks them in the regions regen rewrites, and what it writes is checked. With
// --json, the object also has "skipped" (the pages not recreated) and "upToDate".
func runRegenTo(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "admin" {
		fmt.Fprintln(stderr, regenUsage)
		return 2
	}
	g, err := parseGenArgs(args[1:], []string{"module", "path"}, nil, []string{"all"})
	if err != nil || len(g.pos) != 0 {
		return genUsage(stderr, regenUsage, err)
	}
	o := generate.AdminOptions{Module: g.flag("module"), Path: g.flag("path")}
	skipped := []string{}
	nothing := false
	under := generate.AdminDir(o.Module)
	load := func(dir string) (*generate.Program, error) { return generate.LoadExcept(dir, under) }
	code := runGeneratorWith("alder regen admin", g, generate.Options{}, load, func(p *generate.Program) ([]generate.Edit, error) {
		edits, sk, err := generate.RegenAdmin(p, o, g.has("all"))
		if sk != nil {
			skipped = sk
		}
		if err == nil && len(edits) == 0 {
			nothing = true
		}
		return edits, err
	}, func(out *genJSON) { out.Skipped, out.UpToDate = &skipped, &nothing }, stdout, stderr)
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
