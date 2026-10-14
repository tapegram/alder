package main

import (
	"io"
	"strings"

	"alder/internal/generate"
)

const editUsage = "usage: alder edit table <T> add-field <field>:<Type>[=default][@attr…] | rename-field <old> <new> | drop-field <field>  [--strict] [--dir app] [--json] [--dry-run]\n       alder edit page <Page module> add-param <name>:<Type>[=default] [--dir app] [--json] [--dry-run]\n       alder edit session add-field <name>:<Type>=<default> [--strict] [--dir app] [--json] [--dry-run]"

// runEditTo is `alder edit page` (generate.EditPage), `alder edit session`
// (generate.EditSession) and `alder edit table`: the table's declaration edited (generate.EditTable), its
// migration written by `alder migrate`'s generator (migrateCopy), and the copy checked. The
// code the change breaks (an insert that lacks the new field, a use of a dropped one) is printed
// as the copy's diagnostics, and the edit is still written: the diagnostics are the to-do list.
// With --strict a change that breaks anything is refused instead, and nothing is written.
func runEditTo(args []string, stdout, stderr io.Writer) int {
	g, err := parseGenArgs(args, nil, nil, []string{"strict"})
	if err == nil && len(g.pos) == 4 && g.pos[0] == "page" && !g.has("strict") {
		return runGenerator("alder edit page", g, generate.Options{}, func(p *generate.Program) ([]generate.Edit, error) {
			return generate.EditPage(p, g.pos[1], g.pos[2], g.pos[3:])
		}, stdout, stderr)
	}
	if err == nil && len(g.pos) == 3 && g.pos[0] == "session" {
		// the session literals the new field breaks (a test's `{ user: me }`) are the to-do list,
		// as a table edit's are; --strict refuses instead
		return runGenerator("alder edit session", g, generate.Options{WriteDespiteErrors: !g.has("strict")}, func(p *generate.Program) ([]generate.Edit, error) {
			return generate.EditSession(p, g.pos[1], g.pos[2:])
		}, stdout, stderr)
	}
	if err != nil || len(g.pos) < 3 || g.pos[0] != "table" {
		return genUsage(stderr, editUsage, err)
	}
	// the migration is named after the edit, whatever its steps (a rebuild is still add_<field>)
	slug := ""
	switch {
	case g.pos[2] == "add-field" && len(g.pos) == 4:
		name, _, _ := strings.Cut(g.pos[3], ":")
		slug = "add_" + generate.Snake(name)
	case g.pos[2] == "rename-field" && len(g.pos) == 5:
		slug = "rename_" + generate.Snake(g.pos[3])
	case g.pos[2] == "drop-field" && len(g.pos) == 4:
		slug = "drop_" + generate.Snake(g.pos[3])
	}
	o := generate.Options{Migrate: migrateCopyNamed(slug), WriteDespiteErrors: !g.has("strict")}
	return runGenerator("alder edit table", g, o, func(p *generate.Program) ([]generate.Edit, error) {
		return generate.EditTable(p, g.pos[1], g.pos[2], g.pos[3:])
	}, stdout, stderr)
}

const removeUsage = "usage: alder remove page <Page module> | alder remove table <T>  [--dir app] [--json] [--dry-run]"

// runRemoveTo is `alder remove page` (generate.RemovePage: the page's file) and `alder remove
// table` (generate.RemoveTable: the declaration, or its module's file, and the migration that
// drops the table). Each refuses what something else still names, listing the uses.
func runRemoveTo(args []string, stdout, stderr io.Writer) int {
	g, err := parseGenArgs(args, nil, nil, nil)
	if err != nil || len(g.pos) != 2 || (g.pos[0] != "page" && g.pos[0] != "table") {
		return genUsage(stderr, removeUsage, err)
	}
	if g.pos[0] == "page" {
		return runGenerator("alder remove page", g, generate.Options{}, func(p *generate.Program) ([]generate.Edit, error) {
			return generate.RemovePage(p, g.pos[1])
		}, stdout, stderr)
	}
	o := generate.Options{Migrate: migrateCopyNamed("drop_" + generate.Snake(g.pos[1]))}
	return runGenerator("alder remove table", g, o, func(p *generate.Program) ([]generate.Edit, error) {
		return generate.RemoveTable(p, g.pos[1])
	}, stdout, stderr)
}
