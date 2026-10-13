package main

import (
	"errors"
	"fmt"
	"io"

	"alder/internal/generate"
)

const fillUsage = "usage: alder fill <Module>.<decl>[.<arm>] - [--hole ?name] [--dir app] [--json] [--dry-run]  |  alder fill --list [--dir app] [--json]"

// runFillTo is `alder fill`: with --list, every typed hole of the app (`Module decl ?name :
// Type`, one per line, or {"holes": […]} with --json); otherwise the expression on standard
// input fills the hole of the declaration named (its only one, or --hole's), and the file is
// written only if the app then checks (generate.Fill, generate.Apply). Exit 0, 1 for a refusal
// (nothing written), 2 for usage errors.
func runFillTo(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	g, err := parseGenArgs(args, []string{"hole"}, nil, []string{"list"})
	if err != nil {
		return genUsage(stderr, fillUsage, err)
	}
	if g.has("list") {
		if len(g.pos) != 0 {
			return genUsage(stderr, fillUsage, nil)
		}
		return fillList(g, stdout, stderr)
	}
	if len(g.pos) != 2 || g.pos[1] != "-" {
		return genUsage(stderr, fillUsage, nil)
	}
	src, err := io.ReadAll(stdin)
	if err != nil {
		fmt.Fprintf(stderr, "alder fill: %v\n", err)
		return 2
	}
	return runGenerator("alder fill", g, generate.Options{}, func(p *generate.Program) ([]generate.Edit, error) {
		edits, _, err := generate.Fill(p, g.pos[0], g.flag("hole"), string(src))
		return edits, err
	}, stdout, stderr)
}

// fillList is `alder fill --list`.
func fillList(g genArgs, stdout, stderr io.Writer) int {
	p, err := generate.Load(g.dir)
	if err != nil {
		var ref *generate.Refusal
		if errors.As(err, &ref) {
			return genReport("alder fill", g, nil, err, nil, stdout, stderr)
		}
		fmt.Fprintf(stderr, "alder fill: %v\n", err)
		return 2
	}
	holes := generate.HoleIndex(p.Prog, p.Diags)
	if g.json {
		if holes == nil {
			holes = []generate.Hole{}
		}
		writeJSON(stdout, map[string]any{"holes": holes})
		return 0
	}
	for _, h := range holes {
		fmt.Fprintf(stdout, "%s %s ?%s : %s\n", h.Module, h.DeclText(), h.Name, h.Type)
	}
	return 0
}
