package main

import (
	"fmt"
	"io"
	"os"
)

// commandUsages are the usage lines `alder <command> --help` prints, by command; `alder new
// <generator> --help` prints the generator's (newGeneratorUsages).
var commandUsages = map[string]string{
	"check":    "usage: alder check [dir] [--json]",
	"build":    "usage: alder build [--emit] [dir]",
	"run":      "usage: alder run [dir]",
	"dump":     "usage: alder dump [--tokens|--ast|--ir] <file.alder | dir>",
	"eval":     evalUsage,
	"test":     testUsage,
	"migrate":  migrateUsage,
	"repl":     replUsage,
	"new":      newUsage,
	"edit":     editUsage,
	"remove":   removeUsage,
	"fill":     fillUsage,
	"regen":    regenUsage,
	"deploy":   deployUsage,
	"status":   statusUsage,
	"rollback": rollbackUsage,
	"logs":     logsUsage,
	"restore":  restoreUsage,
	"ci":       ciUsage,
	"silos":    silosUsage,
	"help":     helpUsage,
	"version":  "usage: alder version",
	"explain":  "usage: alder explain <code>  (such as E0016)",
}

var newGeneratorUsages = map[string]string{
	"page": newPageUsage, "table": newTableUsage, "form": newFormUsage, "msg": newMsgUsage,
	"capability": newCapabilityUsage, "admin": newAdminUsage, "auth": newAuthUsage,
	"type": newTypeUsage, "decl": newDeclUsage, "test": newTestUsage, "silo": newSiloUsage,
}

const helpUsage = "usage: alder help [<command> [<generator>]] [--json]"

// wantsHelpJSON reports whether args (os.Args[1:]) are `alder help … --json`.
func wantsHelpJSON(args []string) bool {
	if len(args) == 0 || args[0] != "help" {
		return false
	}
	for _, a := range args[1:] {
		if a == "--json" {
			return true
		}
	}
	return false
}

// helpTo is `alder <command> … --help` (or -h): when args (os.Args[1:]) ask for help, it prints
// the command's usage — the generator's for `alder new <generator> --help` — to w and reports
// true (exit 0). An unknown command prints the overall usage.
func helpTo(args []string, w io.Writer) bool {
	if wantsHelpJSON(args) {
		if err := writeHelpJSON(w); err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
		return true
	}
	asked := false
	for _, a := range args[1:] {
		if a == "--help" || a == "-help" || a == "-h" {
			asked = true
		}
	}
	if len(args) == 0 || !asked && args[0] != "help" {
		return false
	}
	cmd := args[0]
	if cmd == "help" && len(args) > 1 {
		cmd = args[1]
		args = args[1:]
	}
	if cmd == "new" && len(args) > 1 {
		if u, ok := newGeneratorUsages[args[1]]; ok {
			fmt.Fprintln(w, u)
			return true
		}
	}
	if u, ok := commandUsages[cmd]; ok {
		fmt.Fprintln(w, u)
		return true
	}
	usageTo(w)
	return true
}
