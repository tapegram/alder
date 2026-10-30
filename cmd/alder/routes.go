package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"alder/internal/buildkit"
	"alder/internal/diag"
	"alder/internal/style"
)

const routesUsage = "usage: alder routes [dir] [--json]"

type routeJSON struct {
	Kind   string `json:"kind"`
	Path   string `json:"path"`
	Module string `json:"module"`
	Name   string `json:"name"`
	File   string `json:"file"`
	Line   int    `json:"line"`
	Col    int    `json:"col"`
}

// runRoutesTo is `alder routes [dir] [--json]`: the app's routes, pages (GET) and messages
// (POST), sorted by path, as an aligned table or a JSON array. Diagnostics of a program that
// does not check go to stderr as `alder check` prints them, and the exit status is 1.
func runRoutesTo(args []string, stdout, stderr io.Writer) (code int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(stderr, "alder routes: internal error: %v\n", r)
			code = 2
		}
	}()
	dir, jsonOut, haveDir := ".", false, false
	for _, a := range args {
		switch {
		case a == "--json":
			jsonOut = true
		case strings.HasPrefix(a, "-") || haveDir:
			fmt.Fprintln(stderr, routesUsage)
			return 2
		default:
			dir, haveDir = a, true
		}
	}
	prog, res, bag, ioErrs := checkedApp(dir)
	if reportLoadErrs("alder routes", ioErrs, stderr) {
		return 2
	}
	if len(prog.Modules) == 0 {
		fmt.Fprintf(stderr, "alder routes: no .alder files under %s/app\n", dir)
		return 2
	}
	sources := buildkit.Sources(prog)
	for _, d := range bag.Items {
		fmt.Fprint(stderr, diag.RenderTextStyled(d, sources[d.Span.File], style.For(stderr)))
	}
	if bag.HasErrors() || res == nil {
		return 1
	}
	if res.Holes > 0 {
		fmt.Fprintf(stderr, "alder routes: the program has %d typed hole(s); fill them first\n", res.Holes)
		return 1
	}
	routes := buildkit.Routes(prog, res)
	rows := make([]routeJSON, 0, len(routes))
	for _, r := range routes {
		file := r.File
		if filepath.IsAbs(file) {
			if rel, err := filepath.Rel(dir, file); err == nil {
				file = rel
			}
		}
		rows = append(rows, routeJSON{r.Kind, r.Path, r.Module, r.Name, filepath.ToSlash(file), r.Line, r.Col})
	}
	if jsonOut {
		b, err := json.Marshal(rows)
		if err != nil {
			fmt.Fprintf(stderr, "alder routes: %v\n", err)
			return 2
		}
		fmt.Fprintln(stdout, string(b))
		return 0
	}
	tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "KIND\tPATH\tMODULE.decl\tFILE:LINE")
	for _, r := range rows {
		fmt.Fprintf(tw, "%s\t%s\t%s.%s\t%s:%d\n", r.Kind, r.Path, r.Module, r.Name, r.File, r.Line)
	}
	if err := tw.Flush(); err != nil {
		fmt.Fprintf(os.Stderr, "alder routes: %v\n", err)
		return 2
	}
	return 0
}
