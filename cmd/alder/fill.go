package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"alder/internal/fill"
	"alder/internal/generate"
)

const fillUsage = "usage: alder fill <Module>.<decl>[.<arm>] (- | --ai | --prompt) [--hole ?name] [--dir app] [--json] [--dry-run]  |  alder fill --decl <Module>.<decl> - [--dir app] [--json] [--dry-run]  |  alder fill --list [--dir app] [--json]"

// runFillTo is `alder fill`: with --list, every typed hole of the app (`Module decl ?name :
// Type`, one per line, or {"holes": […]} with --json); otherwise the expression on standard
// input fills the hole of the declaration named (its only one, or --hole's), and the file is
// written only if the app then checks (generate.Fill, generate.Apply). Exit 0, 1 for a refusal
// (nothing written), 2 for usage errors. With --ai the expression comes from a model
// (fillAI); --prompt prints what the model would be sent.
func runFillTo(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	g, err := parseGenArgs(args, []string{"hole"}, nil, []string{"list", "decl", "ai", "prompt"})
	if err != nil {
		return genUsage(stderr, fillUsage, err)
	}
	if g.has("ai") || g.has("prompt") {
		flag := map[bool]string{true: "--ai", false: "--prompt"}[g.has("ai")]
		switch {
		case g.has("ai") && g.has("prompt"):
			return genUsage(stderr, fillUsage, errors.New("alder fill: --ai and --prompt are two ways to run: give one"))
		case g.has("list") || g.has("decl"):
			return genUsage(stderr, fillUsage, fmt.Errorf("alder fill: %s fills a hole: it takes no --list or --decl", flag))
		case len(g.pos) == 2 && g.pos[1] == "-":
			return genUsage(stderr, fillUsage, fmt.Errorf("alder fill: %s reads no standard input: drop the `-`", flag))
		case len(g.pos) != 1:
			return genUsage(stderr, fillUsage, nil)
		}
		if g.has("prompt") {
			return fillPrompt(g, stdout, stderr)
		}
		return fillAI(g, stdout, stderr)
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
	imports, file := []string{}, ""
	code := runGeneratorWith("alder fill", g, generate.Options{}, nil, func(p *generate.Program) ([]generate.Edit, error) {
		var edits []generate.Edit
		var added []string
		var err error
		if g.has("decl") {
			if g.flag("hole") != "" {
				return nil, generate.Refusef("--decl replaces a whole declaration: it takes no --hole")
			}
			edits, added, err = generate.FillDecl(p, g.pos[0], string(src))
		} else {
			edits, _, added, err = generate.Fill(p, g.pos[0], g.flag("hole"), string(src))
		}
		if err == nil {
			imports, file = append(imports, added...), edits[0].Path
		}
		return edits, err
	}, func(out *genJSON) {
		if out.Error != "" { // refused: nothing imported
			imports = []string{}
		}
		out.Imports = &imports
	}, stdout, stderr)
	if code == 0 && !g.json {
		printImports(g, imports, file, stdout)
		if g.has("decl") && strings.Contains(g.pos[0], `."`) {
			// the checker checks a test's types, not what it asserts
			fmt.Fprintln(stdout, "rewriting a test: `alder test --mutate` is the check that it still tests something")
		}
	}
	return code
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

// printImports prints the imports a fill added to file.
func printImports(g genArgs, imports []string, file string, stdout io.Writer) {
	verb := "added"
	if g.dry {
		verb = "would add"
	}
	for _, m := range imports {
		fmt.Fprintf(stdout, "%s import %s to %s\n", verb, m, file)
	}
}

// fillBackend is the model backend of `alder fill --ai` (a variable for tests).
var fillBackend = fill.Env

// fillPrompt is `alder fill <address> --prompt`: the first attempt's conversation, exactly as
// the command backend reads it, with the ALDER_FILL_REFERENCE file's text first when it is set;
// nothing is sent.
func fillPrompt(g genArgs, stdout, stderr io.Writer) int {
	p, err := generate.Load(g.dir)
	if err == nil {
		var h *generate.Hole
		if h, err = generate.FindHole(p, g.pos[0], g.flag("hole")); err == nil {
			system, err := fill.System(os.Getenv)
			if err != nil {
				fmt.Fprintf(stderr, "alder fill: %v\n", err)
				return 2
			}
			io.WriteString(stdout, fill.Transcript(system, []fill.Message{{Role: "user", Content: fill.Prompt(p, *h)}}))
			return 0
		}
	}
	return genReport("alder fill", g, nil, err, nil, stdout, stderr)
}

// fillAI is `alder fill <address> --ai`: fill.Run with the environment's backend (each retry
// on standard error), then the returned edits applied as `alder fill -` applies its own (the
// copy checked again, --dry-run honoured). --json adds the expression, the attempts and the
// model. When no answer checks, the errors and the model's last expression are printed and
// nothing is written (exit 1).
func fillAI(g genArgs, stdout, stderr io.Writer) int {
	b, model := fillBackend()
	if b == nil {
		fmt.Fprintln(stderr, "alder fill: set ANTHROPIC_API_KEY, or ALDER_FILL_COMMAND, in the environment")
		return 2
	}
	report := func(a fill.Attempt) {
		fmt.Fprintf(stderr, "alder fill: attempt %d of %d: %s\n", a.N, fill.Attempts, a.Why)
	}
	r, err := fill.Run(context.Background(), b, g.dir, g.pos[0], g.flag("hole"), report)
	var f *fill.Failure
	if errors.As(err, &f) {
		if g.json {
			writeJSON(stdout, map[string]any{"files": []any{}, "diff": "", "diagnostics": []any{}, "holes": []any{}, "written": false,
				"error": f.Why, "errors": f.Errors, "expression": f.Expression, "attempts": f.Attempts, "model": model})
		} else {
			if f.Errors != "" {
				io.WriteString(stdout, strings.TrimRight(f.Errors, "\n")+"\n")
			}
			if f.Expression != "" {
				fmt.Fprintf(stdout, "the model's last expression:\n%s\n", f.Expression)
			}
		}
		fmt.Fprintf(stderr, "alder fill: %s\n", f.Why)
		return 1
	}
	if err != nil {
		var ref *generate.Refusal
		if errors.As(err, &ref) {
			return genReport("alder fill", g, nil, err, nil, stdout, stderr)
		}
		fmt.Fprintf(stderr, "alder fill: %v\n", err)
		return 2
	}
	imports := r.Imports
	if imports == nil {
		imports = []string{}
	}
	// the hole's type does not limit these: they are named, never applied silently
	effects := r.Effects
	if effects == nil {
		effects = []string{}
	}
	res, err := generate.Apply(g.dir, r.Edits, generate.Options{DryRun: g.dry})
	code := genReport("alder fill", g, res, err, func(out *genJSON) {
		if out.Error != "" {
			imports = []string{}
		}
		out.Imports, out.Expression, out.Attempts, out.Model, out.Effects = &imports, &r.Expression, &r.Attempts, &model, &effects
	}, stdout, stderr)
	if code == 0 && !g.json {
		printImports(g, imports, r.Edits[0].Path, stdout)
		if len(effects) > 0 {
			fmt.Fprintf(stdout, "the model's expression:\n%s\n", r.Expression)
			fmt.Fprintf(stderr, "alder fill: warning: %s\n", fill.EffectsWarning(effects))
		}
		if strings.HasPrefix(r.Hole.Decl, `"`) {
			fmt.Fprintln(stdout, "a model wrote this test's expectation: `alder test --mutate` is the check that it tests something")
		}
	}
	return code
}
