package main

import (
	"io"

	"alder/internal/generate"
)

const editUsage = "usage: alder edit table <T> add-field <field>:<Type>[=default][@attr…] | rename-field <old> <new> | drop-field <field>  [--strict] [--dir app] [--json] [--dry-run]"

// runEditTo is `alder edit table`: the table's declaration edited (generate.EditTable), its
// migration written by `alder migrate`'s generator (migrateCopy), and the copy checked. The
// code the change breaks (an insert that lacks the new field, a use of a dropped one) is printed
// as the copy's diagnostics, and the edit is still written: the diagnostics are the to-do list.
// With --strict a change that breaks anything is refused instead, and nothing is written.
func runEditTo(args []string, stdout, stderr io.Writer) int {
	g, err := parseGenArgs(args, nil, nil, []string{"strict"})
	if err != nil || len(g.pos) < 3 || g.pos[0] != "table" {
		return genUsage(stderr, editUsage, err)
	}
	o := generate.Options{Migrate: migrateCopy, WriteDespiteErrors: !g.has("strict")}
	return runGenerator("alder edit table", g, o, func(p *generate.Program) ([]generate.Edit, error) {
		return generate.EditTable(p, g.pos[1], g.pos[2], g.pos[3:])
	}, stdout, stderr)
}
