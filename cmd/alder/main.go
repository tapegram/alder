package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"alder/internal/check"
	"alder/internal/diag"
	"alder/internal/names"
	"alder/internal/types"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "check":
		os.Exit(runCheckTo(os.Args[2:], os.Stdout, os.Stderr))
	case "explain":
		if len(os.Args) < 3 {
			usage()
			os.Exit(2)
		}
		text, ok := diag.Explain[os.Args[2]]
		if !ok {
			fmt.Fprintf(os.Stderr, "unknown diagnostic code %s\n", os.Args[2])
			os.Exit(2)
		}
		fmt.Println(text)
	default:
		usage()
		os.Exit(2)
	}
}

func usage() { fmt.Fprintln(os.Stderr, "usage: alder check [dir] [--json] | alder explain <code>") }

func runCheckTo(args []string, stdout, stderr io.Writer) int {
	dir, jsonOut := ".", false
	for _, a := range args {
		if a == "--json" {
			jsonOut = true
		} else {
			dir = a
		}
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "app", "*.alder"))
	more, _ := filepath.Glob(filepath.Join(dir, "app", "*", "*.alder"))
	deeper, _ := filepath.Glob(filepath.Join(dir, "app", "*", "*", "*.alder"))
	if len(matches)+len(more)+len(deeper) == 0 {
		fmt.Fprintf(stderr, "alder check: no .alder files under %s/app\n", dir)
		return 2
	}
	std, err := types.LoadStd()
	if err != nil {
		fmt.Fprintf(stderr, "alder: internal error loading stdlib: %v\n", err)
		return 2
	}
	// The pipeline stops after the first phase that produced errors (parse, names, check).
	prog, bag := names.Load(dir, std)
	if !bag.HasErrors() {
		prog.Resolve(bag)
	}
	if !bag.HasErrors() {
		check.Program(prog, bag)
	}
	bag.Sort()
	sources := map[string][]string{}
	for _, m := range prog.Modules {
		sources[m.File] = m.Lines
	}
	for _, d := range bag.Items {
		if jsonOut {
			fmt.Fprintln(stdout, diag.RenderJSON(d))
			continue
		}
		w := stdout
		if d.Severity == "warning" {
			w = stderr
		}
		fmt.Fprint(w, diag.RenderText(d, sources[d.Span.File]))
	}
	if bag.HasErrors() {
		return 1
	}
	return 0
}
