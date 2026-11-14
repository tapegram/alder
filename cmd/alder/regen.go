package main

import (
	"fmt"
	"io"

	"alder/internal/generate"
)

const regenAdminUsage = "usage: alder regen admin [--module Admin] [--path /admin] [--all] [--dir app] [--json] [--dry-run]"

const newRegenResourceUsage = "usage: alder regen resource [<Table> …] [--all] [--module <M>] [--path /p] [--dir app] [--json] [--dry-run]"

// regenUsage is the command's usage: each subcommand's line.
const regenUsage = regenAdminUsage + "\n" + newRegenResourceUsage

// regenGeneratorUsages is each subcommand's usage line (`alder regen <generator> --help`).
var regenGeneratorUsages = map[string]string{"admin": regenAdminUsage, "resource": newRegenResourceUsage}

// runRegenTo is `alder regen admin|resource`: the admin (or the resource of a table) regenerated from
// the app as it is now, and only its owned regions (`-- @admin begin <name>` … `-- @admin end`;
// `-- @resource …` for a resource) written back (generate.RegenAdmin, generate.RegenResource). The text outside the regions, the author's, is left byte for byte; a
// file the author deleted is not recreated unless --all (a table added since gets its pages).
// A second run has nothing to do. A file whose markers are broken is refused, nothing written.
// Errors in the generated files themselves do not refuse it (generate.LoadExcept): a field renamed or
// dropped breaks them in the regions regen rewrites, and what it writes is checked. With
// --json, the object also has "skipped" (the pages not recreated) and "upToDate".
func runRegenTo(args []string, stdout, stderr io.Writer) int {
	s, rest := generate.Select("regen", args, generate.Specs())
	if s == nil {
		fmt.Fprintln(stderr, regenUsage)
		return 2
	}
	var res *generate.Result
	var cli generate.CLIFlags
	usage, what := regenAdminUsage, "the admin"
	if s.ID == "regen resource" {
		usage, what = newRegenResourceUsage, "the resource"
	}
	code := runSpecWith(s, rest, usage, func(r *generate.Result, c generate.CLIFlags) { res, cli = r, c }, stdout, stderr)
	if code == 0 && !cli.JSON && res != nil {
		for _, sk := range res.Skipped {
			fmt.Fprintf(stdout, "not recreated %s (deleted; --all writes it again)\n", sk)
		}
		if res.UpToDate {
			fmt.Fprintf(stdout, "nothing to do: %s is up to date\n", what)
		}
	}
	return code
}
