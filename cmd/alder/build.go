package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"alder/internal/buildkit"
	"alder/internal/diag"
	"alder/internal/gen"
	"alder/internal/ir"
)

// runBuildTo is `alder build [--emit] [dir]`: check the app in dir and, when it has no errors
// and no holes, lower it and emit its Go into build/<app>/ (app is dir's base name), the module
// "app". With --emit it stops there, after vetting that package. Without, it builds the app's
// executable build/<app>/<app>: it copies dir/adapters/*.go (the Go implementing the app's
// capabilities, package adapters; only the top-level .go files, and a directory without any
// counts as absent) to build/<app>/adapters/ and dir/static/ to
// build/<app>/static/ (replacing earlier copies), writes the command build/<app>/cmd/main.go,
// vets the module and builds the command.
//
// Exit 0 on success; 1 when the program has errors or holes, when it has capabilities but no
// adapters directory, or when the adapters do not compile (Go's errors are printed as they
// are); 2 for usage errors and internal errors (including generated Go that does not vet).
func runBuildTo(args []string, stdout, stderr io.Writer) int {
	dir, emit, haveDir := ".", false, false
	for _, a := range args {
		switch {
		case a == "--emit":
			emit = true
		case strings.HasPrefix(a, "-") || haveDir:
			usageTo(stderr)
			return 2
		default:
			dir, haveDir = a, true
		}
	}
	path, code := buildTo(dir, emit, "", stdout, stderr)
	if code == 0 {
		fmt.Fprintf(stdout, "wrote %s\n", path)
	}
	return code
}

// buildApp builds the executable of the app in dir as `alder build dir` does (buildTo), into the
// output directory out ("" is build/<app>), every message (diagnostics included) going to
// stderr, and returns its absolute path with exit code 0, or "" with the build's exit code.
// `alder run` rebuilds with it.
func buildApp(dir, out string, stderr io.Writer) (binary string, code int) {
	path, code := buildTo(dir, false, out, stderr, stderr)
	if code != 0 {
		return "", code
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		fmt.Fprintf(stderr, "alder build: %v\n", err)
		return "", 2
	}
	return abs, 0
}

// buildTo is the work of `alder build` after its arguments are parsed: it writes the generated
// module into the output directory out ("" is build/<app>, relative to the working directory)
// and returns the path it wrote (out with emit, else the executable out/<app>) and exit code 0,
// or "" and the exit code runBuildTo documents. Error diagnostics go to stdout, everything else
// to stderr.
func buildTo(dir string, emit bool, out string, stdout, stderr io.Writer) (written string, code int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(stderr, "alder build: internal error: %v\n", r)
			written, code = "", 2
		}
	}()
	abs, err := filepath.Abs(dir)
	if err != nil {
		fmt.Fprintf(stderr, "alder build: %v\n", err)
		return "", 2
	}
	repo, err := buildkit.RepoRoot()
	if err != nil {
		fmt.Fprintf(stderr, "alder build: %v\n", err)
		return "", 2
	}

	prog, res, bag, ioErrs := buildkit.LoadChecked(dir, nil)
	if reportLoadErrs("alder build", ioErrs, stderr) {
		return "", 2
	}
	if len(prog.Modules) == 0 {
		fmt.Fprintf(stderr, "alder build: no .alder files under %s/app\n", dir)
		return "", 2
	}
	sources := buildkit.Sources(prog)
	for _, d := range bag.Items {
		w := stdout
		if d.Severity == "warning" {
			w = stderr
		}
		fmt.Fprint(w, diag.RenderText(d, sources[d.Span.File]))
	}
	if bag.HasErrors() || res == nil {
		return "", 1
	}
	if res.Holes > 0 {
		fmt.Fprintf(stderr, "alder build: the program has %d typed hole(s); fill them before building\n", res.Holes)
		return "", 1
	}

	p := ir.Lower(prog, res)
	name := filepath.Base(abs)
	adapters := filepath.Join(dir, "adapters")
	hasAdapters := buildkit.HasGoFiles(adapters) // an empty adapters/ (or one of notes only) is none
	if !emit && !hasAdapters && len(p.Caps) > 0 {
		var list []string
		for _, c := range p.Caps {
			list = append(list, fmt.Sprintf("%s.%s (interface app.Cap__%s__%s)", c.Module, c.Name, strings.ReplaceAll(c.Module, ".", "__"), c.Name))
		}
		fmt.Fprintf(stderr, "%s needs an adapters package: create adapters/caps.go with `func Caps() app.Caps` implementing: %s\n", dir, strings.Join(list, ", "))
		return "", 1
	}
	files, err := gen.Emit(p, gen.Options{RepoPath: repo})
	if err != nil {
		fmt.Fprintf(stderr, "alder build: internal error: %v\n", err)
		return "", 2
	}
	if out == "" {
		out = filepath.Join("build", name)
	}
	if err := buildkit.WriteGenerated(out, files); err != nil {
		fmt.Fprintf(stderr, "alder build: %v\n", err)
		return "", 2
	}
	if emit {
		// the app package alone: a cmd/ or adapters/ of an earlier full build is not vetted
		if err := buildkit.VetApp(out, files, "."); err != nil {
			fmt.Fprintf(stderr, "alder build: %v", err)
			return "", 2
		}
		return out, 0
	}

	// the executable
	if err := buildkit.CopyAdapters(adapters, filepath.Join(out, "adapters"), hasAdapters); err != nil {
		fmt.Fprintf(stderr, "alder build: %v\n", err)
		return "", 2
	}
	if err := buildkit.CopyStatic(filepath.Join(dir, "static"), filepath.Join(out, "static")); err != nil {
		fmt.Fprintf(stderr, "alder build: %v\n", err)
		return "", 2
	}
	if err := os.MkdirAll(filepath.Join(out, "cmd"), 0o755); err != nil {
		fmt.Fprintf(stderr, "alder build: %v\n", err)
		return "", 2
	}
	if err := os.WriteFile(filepath.Join(out, "cmd", "main.go"), gen.Main(name, hasAdapters), 0o644); err != nil {
		fmt.Fprintf(stderr, "alder build: %v\n", err)
		return "", 2
	}
	if vetOut, err := buildkit.GoCmd(out, "vet", "-mod=mod", "./..."); err != nil {
		// the generated package first: when it does not vet, that is not the adapters' fault
		if err := buildkit.VetApp(out, files, "."); err != nil {
			fmt.Fprintf(stderr, "alder build: %v", err)
			return "", 2
		}
		if hasAdapters {
			fmt.Fprintf(stderr, "alder build: the adapters package (%s, copied to %s) does not compile with the app: %v\n%s",
				adapters, filepath.Join(out, "adapters"), err, vetOut)
			if strings.Contains(string(vetOut), "adapters.Caps") {
				fmt.Fprintf(stderr, "the adapters package must declare `func Caps() app.Caps`\n")
			}
			return "", 1
		}
		fmt.Fprintf(stderr, "alder build: internal error: the generated Go in %s does not vet: %v\n%s", out, err, vetOut)
		return "", 2
	}
	if buildOut, err := buildkit.GoCmd(out, "build", "-mod=mod", "-o", name, "./cmd"); err != nil {
		fmt.Fprintf(stderr, "alder build: internal error: go build of %s failed: %v\n%s", out, err, buildOut)
		return "", 2
	}
	return filepath.Join(out, name), 0
}

// reportLoadErrs prints the failures buildkit.LoadChecked returned, each prefixed with the
// command's name (a standard library that does not load is an internal error of alder), and
// reports whether there were any.
func reportLoadErrs(cmd string, ioErrs []error, stderr io.Writer) bool {
	for _, e := range ioErrs {
		if se := (*buildkit.StdError)(nil); errors.As(e, &se) {
			fmt.Fprintf(stderr, "alder: internal error loading stdlib: %v\n", se.Err)
			continue
		}
		fmt.Fprintf(stderr, "%s: %v\n", cmd, e)
	}
	return len(ioErrs) > 0
}
