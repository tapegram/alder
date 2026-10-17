package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"alder/internal/buildkit"
	"alder/internal/check"
	"alder/internal/diag"
	"alder/internal/gen"
	"alder/internal/ir"
	"alder/internal/names"
	"alder/internal/version"
)

// runBuildTo is `alder build [--emit] [dir]`: check the app in dir and, when it has no errors
// and no holes, lower it and emit its Go into build/<app>/ (app is dir's base name), the module
// "app", with the runtime it requires in build/<app>/rt/ (buildkit.WriteRuntime: the copy
// embedded in this binary, refreshed on every build) and its dependencies in Go's module cache
// (buildkit.DownloadDeps: downloaded once per machine, with the line buildkit.FetchingLine; a
// failed download is exit 1). With --emit it stops there, after vetting that package. Without, it builds the app's
// executable build/<app>/<app>: it checks that dir/adapters/ has an adapter for every
// capability (buildkit.CheckAdapters: capability X of module A.B is `func BX()` of package
// adapters/a/; every missing one is printed, and nothing is written), copies the whole
// dir/adapters/ tree (a directory without any .go file counts as absent) to
// build/<app>/adapters/, next to the generated wiring build/<app>/caps/ (gen.CapsPackage), and
// dir/static/ to build/<app>/static/ and dir/migrations/*.sql (with migrations/schema.json) to
// build/<app>/migrations/ for app__alder.go to embed (replacing earlier copies), writes the
// command build/<app>/cmd/main.go, vets the module and builds the command. With --emit the
// adapters are not checked.
//
// Exit 0 on success; 1 when the program has errors or holes, when an adapter is missing (or
// the adapters are in the removed flat form), or when the adapters do not compile with the app
// (Go's errors are printed as they are); 2 for usage errors and internal errors (including
// generated Go that does not vet).
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

	prog, res, bag, ioErrs := checkedApp(dir)
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
	hasAdapters := buildkit.HasAdapters(dir) // an empty adapters/ (or one of notes only) is none
	if !emit && !buildkit.CheckAdapters(dir, p.Caps, stderr) {
		return "", 1
	}
	files, err := gen.Emit(p, gen.Options{Version: version.String(), Migrations: buildkit.HasMigrations(dir)})
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
	if err := buildkit.WriteRuntime(out); err != nil {
		fmt.Fprintf(stderr, "alder build: %v\n", err)
		return "", 2
	}
	fetching := func() { fmt.Fprintln(stderr, buildkit.FetchingLine) }
	if err := buildkit.DownloadDeps(context.Background(), out, fetching); err != nil {
		fmt.Fprintf(stderr, "alder build: %v\n", err)
		if de := (*buildkit.DownloadError)(nil); errors.As(err, &de) {
			return "", 1 // the machine's network or proxy, not the program or the compiler
		}
		return "", 2
	}
	if err := buildkit.CopyMigrations(filepath.Join(dir, "migrations"), filepath.Join(out, "migrations")); err != nil {
		fmt.Fprintf(stderr, "alder build: %v\n", err)
		return "", 1
	}
	if emit {
		// the app package alone: a cmd/, caps/ or adapters/ of an earlier full build is not vetted
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
	if err := os.WriteFile(filepath.Join(out, "cmd", "main.go"), gen.Main(name, len(p.Caps) > 0, len(p.Silos) > 0), 0o644); err != nil {
		fmt.Fprintf(stderr, "alder build: %v\n", err)
		return "", 2
	}
	if vetOut, err := buildkit.GoCmd(out, "vet", "-mod=mod", "./..."); err != nil {
		// the generated package first: when it does not vet, that is not the adapters' fault
		if err := buildkit.VetApp(out, files, "."); err != nil {
			fmt.Fprintf(stderr, "alder build: %v", err)
			return "", 2
		}
		if hasAdapters || len(p.Caps) > 0 { // the adapters, or the wiring of their constructors
			fmt.Fprintf(stderr, "alder build: %s", buildkit.AdaptersCompileMessage(name, filepath.Join(out, "adapters"), err, vetOut))
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

// checkedApp is buildkit.LoadChecked of the app in dir (no extra module) followed by the check
// of its migrations, buildkit.AddSchemaDrift: E0019 when its tables changed since the last
// migration. `alder check` and `alder build` (and so `alder run`) check with it; `alder test`,
// `eval` and the REPL get the same through scratch.Check.
func checkedApp(dir string) (*names.Program, *check.Result, *diag.Bag, []error) {
	prog, res, bag, ioErrs := buildkit.LoadChecked(dir, nil)
	if len(ioErrs) == 0 {
		buildkit.AddSchemaDrift(dir, prog, res, bag)
	}
	return prog, res, bag, ioErrs
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
