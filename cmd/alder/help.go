package main

import (
	"fmt"
	"io"
)

// commandUsages are the usage lines `alder <command> --help` prints, by command; `alder new
// <generator> --help` prints the generator's (newGeneratorUsages).
var commandUsages = map[string]string{
	"check":    "usage: alder check [dir] [--json]",
	"fmt":      fmtUsage,
	"build":    "usage: alder build [--emit] [--build-dir DIR] [dir]",
	"run":      "usage: alder run [--build-dir DIR] [dir]",
	"tui":      tuiUsage,
	"routes":   routesUsage,
	"dump":     "usage: alder dump [--tokens|--ast|--ir] <file.alder | dir>",
	"eval":     evalUsage,
	"scratch":  scratchUsage,
	"test":     testUsage,
	"bench":    benchUsage,
	"migrate":  migrateUsage,
	"repl":     replUsage,
	"new":      newUsage,
	"edit":     editUsage,
	"remove":   removeUsage,
	"fill":     fillUsage,
	"regen":    regenUsage,
	"theme":    themeUsage,
	"deploy":   deployUsage,
	"status":   statusUsage,
	"rollback": rollbackUsage,
	"logs":     logsUsage,
	"restore":  restoreUsage,
	"ci":       ciUsage,
	"silos":    silosUsage,
	"help":     helpUsage,
	"version":  "usage: alder version",
	"lsp":      "usage: alder lsp",
	"explain":  "usage: alder explain <code>  (such as E0003)",
}

var newGeneratorUsages = map[string]string{
	"route": newRouteUsage, "table": newTableUsage, "form": newFormUsage, "action": newActionUsage,
	"capability": newCapabilityUsage, "admin": newAdminUsage, "auth": newAuthUsage,
	"type": newTypeUsage, "decl": newDeclUsage, "test": newTestUsage, "silo": newSiloUsage,
}

// newRouteHelpNote follows `alder new route --help`'s usage line: how a module name maps to a file.
const newRouteHelpNote = `
A dotted module name is a nested module: each dot is a directory.
  alder new route Sites.Clarity.Index /sites/clarity
writes app/Sites/Clarity/Index.alder, whose route is named Index (a name without dots, About,
writes app/About.alder with the route About). A write route has only update:
  alder new route Like /like --method POST --param postId:Post.PostId`

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
	if cmd == "help" && len(args) == 1 { // `alder help`: the list of commands
		usageTo(w)
		return true
	}
	if cmd == "help" && len(args) > 1 {
		cmd = args[1]
		args = args[1:]
	}
	if cmd == "new" && len(args) > 1 {
		kind := args[1]
		if u, ok := newGeneratorUsages[kind]; ok {
			fmt.Fprintln(w, u)
			if kind == "route" {
				fmt.Fprintln(w, newRouteHelpNote)
			}
			return true
		}
	}
	if u, ok := commandUsages[cmd]; ok {
		fmt.Fprintln(w, u)
		if cmd == "test" {
			fmt.Fprintln(w, testHelpNote)
		}
		if cmd == "bench" {
			fmt.Fprintln(w, benchHelpNote)
		}
		return true
	}
	usageTo(w)
	return true
}
