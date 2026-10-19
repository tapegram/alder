package main

import (
	"io"

	"alder/internal/generate"
)

const editUsage = "usage: alder edit table <T> add-field <field>:<Type>[=default][@attr…] | rename-field <old> <new> | drop-field <field>  [--strict] [--dir app] [--json] [--dry-run]\n       alder edit page <Page module> add-param <name>:<Type>[=default] [--dir app] [--json] [--dry-run]\n       alder edit decl <Module>.<Name> add-prop <name>:<Type>=<default> [--dir app] [--json] [--dry-run]\n       alder edit auth --to <Page> [--dir app] [--json] [--dry-run]\n       alder edit session add-field <name>:<Type>=<default> | add-field <key>:<T>.<T>Id via <Membership> [--choose <Page>]  [--strict] [--dir app] [--json] [--dry-run]"

const removeUsage = "usage: alder remove page <Page module> | alder remove table <T>  [--dir app] [--json] [--dry-run]"

// runGroup is `alder edit|remove <sub> …`: the subcommand's Spec, or the group's usage.
// (In help --json the group node has no Usage of its own flags: each subcommand node carries
// its Args and Flags, and the group's usage line is the text printed here.)
func runGroup(group, usage string, args []string, stdout, stderr io.Writer) int {
	s, rest := generate.Select(group, args, generate.Specs())
	if s == nil {
		return genUsage(stderr, usage, nil)
	}
	return runSpec(s, rest, usage, stdout, stderr)
}

// runEditTo is `alder edit table|page|decl|auth|session`. A table edit writes its migration
// (named after the edit) and prints the code it breaks as the copy's diagnostics, still
// writing the edit; --strict refuses instead. Each subcommand takes only its own flags.
func runEditTo(args []string, stdout, stderr io.Writer) int {
	return runGroup("edit", editUsage, args, stdout, stderr)
}

// runRemoveTo is `alder remove page` (the page's file) and `alder remove table` (the
// declaration, or its module's file, and the migration that drops the table). Each refuses what
// something else still names, listing the uses.
func runRemoveTo(args []string, stdout, stderr io.Writer) int {
	return runGroup("remove", removeUsage, args, stdout, stderr)
}
