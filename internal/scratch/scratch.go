// Package scratch compiles an app together with one extra module held in memory (the scratch
// module) and a Go main of the caller's, and runs the result. It is the engine under `alder
// eval`, `alder test`, the REPL and the book verifier: each supplies the scratch module's
// source and a main that calls into the generated package "app" (a top-level value `it` of
// module Scratch is app.Scratch__it), and reads what the program prints.
package scratch

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"alder/internal/buildkit"
	"alder/internal/diag"
	"alder/internal/gen"
	"alder/internal/ir"
	"alder/internal/names"
	"alder/internal/syntax"
	"alder/internal/types"
)

// Module is a module held in memory. Name is its module name ("Scratch" by convention) and,
// with ".alder", the file its diagnostics name; Source may omit the `module` line.
type Module struct{ Name, Source string }

// Build is the outcome of Compile.
type Build struct {
	Dir    string            // the output directory (absolute)
	Binary string            // the executable (absolute), "" unless the build succeeded
	Diags  []diag.Diagnostic // check diagnostics, sorted: the app's (File "app/…") and the extra module's (File "<Name>.alder")
	Err    error             // an I/O, vet or go build failure (with Go's output); never a diagnostic
	// LineOffset is the number of lines Compile inserted into the extra module's source: a
	// diagnostic of the extra module at line L is at line L-LineOffset of the caller's source
	// (for L after the inserted lines; a source without a `module` line has them at the top).
	//
	// Callers should map diagnostics with MapDiag (or lines with SourceLine), which also handle
	// the lines before and on the inserted ones.
	LineOffset int
	InsertLine int    // the first inserted line (1-based) of the rewritten source; 0 when none
	ExtraFile  string // the extra module's file, "<Name>.alder" ("" without an extra module)
}

// Timeout bounds a Run.
const Timeout = 60 * time.Second

// Compile builds the app at appDir with extra (nil for none) added as one more module, and the
// caller's Go command mainGo (package main, importing "app"), into the output directory out;
// out "" is build/<app>/scratch/<purpose> under the working directory (purpose "" is "default").
//
// The extra module implicitly imports every app module except App (which needs no import)
// and those its source already imports: Compile inserts `import M` lines after the source's
// `module` line, or, when it has none, prepends `module <Name>` and the imports; unused
// implicit imports are not reported. LineOffset says how many lines were inserted.
//
// What callers may assume:
//   - Diagnostics are returned, never printed. An app diagnostic's Span.File is the file under
//     the app ("app/Todo.alder"); the extra module's is "<Name>.alder". Diags may hold warnings
//     alone and still produce a binary.
//   - Binary is "" when Diags hold an error or a typed hole (W0002), or when Err is set. Err is
//     set only for I/O failures, generated or caller Go that does not vet, and go build
//     failures; its message carries Go's output.
//   - Compile writes only under out (the generated module "app", adapters/ and static/ copied
//     from the app, cmd/main.go and the executable out/scratch); it never touches build/<app>
//     itself. Nothing is written when the check fails, and the previous executable in out stays
//     until a new one is built, so a failed Compile leaves the last good one runnable.
//   - Compiles into the same out reuse Go's build cache (a second build of an unchanged app is
//     much faster). Two Compiles must not run into the same out at once.
func Compile(appDir string, extra *Module, mainGo string, purpose, out string) (b Build) {
	defer func() {
		if r := recover(); r != nil {
			b.Binary, b.Err = "", fmt.Errorf("internal error: %v", r)
		}
	}()
	abs, err := filepath.Abs(appDir)
	if err != nil {
		return Build{Err: err}
	}
	name := filepath.Base(abs)
	if out == "" {
		if purpose == "" {
			purpose = "default"
		}
		out = filepath.Join("build", name, "scratch", purpose)
	}
	if b.Dir, err = filepath.Abs(out); err != nil {
		return Build{Err: err}
	}

	var mods []*syntax.Module
	var implicit map[int]bool // the inserted import lines of extra
	if extra != nil {
		modName := extra.Name
		if modName == "" {
			modName = "Scratch"
		}
		if err := checkDeclaredName(extra.Source, modName); err != nil {
			b.Err = err
			return b
		}
		appMods, err := appModules(appDir)
		if err != nil {
			b.Err = err
			return b
		}
		src, offset, lines := withImports(extra.Source, modName, appMods)
		b.LineOffset, implicit = offset, lines
		b.InsertLine, b.ExtraFile = insertInfo(extra.Source, modName, offset)
		m, ds := syntax.ParseFile(modName+".alder", []byte(src))
		if len(ds) > 0 {
			b.Diags = ds
			return b
		}
		mods = append(mods, m)
	}

	prog, res, bag, ioErrs := buildkit.LoadChecked(appDir, mods)
	if len(ioErrs) > 0 {
		b.Err = errors.Join(ioErrs...)
		return b
	}
	if len(prog.Modules) == len(mods) {
		b.Err = fmt.Errorf("no .alder files under %s/app", appDir)
		return b
	}
	for _, d := range bag.Items {
		if d.Code == diag.UnusedImport && extra != nil && d.Span.File == mods[0].File && implicit[d.Span.Start.Line] {
			continue
		}
		b.Diags = append(b.Diags, d)
	}
	if bag.HasErrors() || res == nil || res.Holes > 0 {
		return b
	}

	repo, err := buildkit.RepoRoot()
	if err != nil {
		b.Err = err
		return b
	}
	files, err := gen.Emit(ir.Lower(prog, res), gen.Options{RepoPath: repo})
	if err != nil {
		b.Err = fmt.Errorf("internal error: %v", err)
		return b
	}
	adapters := filepath.Join(appDir, "adapters")
	hasAdapters := buildkit.HasGoFiles(adapters)
	if err := buildkit.WriteGenerated(b.Dir, files); err != nil {
		b.Err = err
		return b
	}
	if err := buildkit.CopyAdapters(adapters, filepath.Join(b.Dir, "adapters"), hasAdapters); err != nil {
		b.Err = err
		return b
	}
	if err := buildkit.CopyStatic(filepath.Join(appDir, "static"), filepath.Join(b.Dir, "static")); err != nil {
		b.Err = err
		return b
	}
	if err := os.MkdirAll(filepath.Join(b.Dir, "cmd"), 0o755); err != nil {
		b.Err = err
		return b
	}
	if err := os.WriteFile(filepath.Join(b.Dir, "cmd", "main.go"), []byte(mainGo), 0o644); err != nil {
		b.Err = err
		return b
	}
	if vetOut, err := buildkit.GoCmd(b.Dir, "vet", "-mod=mod", "./..."); err != nil {
		if verr := buildkit.VetApp(b.Dir, files, "."); verr != nil {
			b.Err = verr
			return b
		}
		b.Err = fmt.Errorf("the Go in %s (the main or the adapters) does not vet: %v\n%s", b.Dir, err, vetOut)
		return b
	}
	if buildOut, err := buildkit.GoCmd(b.Dir, "build", "-mod=mod", "-o", "scratch", "./cmd"); err != nil {
		b.Err = fmt.Errorf("go build of %s failed: %v\n%s", b.Dir, err, buildOut)
		return b
	}
	b.Binary = filepath.Join(b.Dir, "scratch")
	return b
}

// appModules lists the names of the app's modules (sorted).
func appModules(appDir string) ([]string, error) {
	std, err := types.LoadStd()
	if err != nil {
		return nil, fmt.Errorf("internal error loading stdlib: %v", err)
	}
	prog, _, ioErrs := names.Load(appDir, std)
	if len(ioErrs) > 0 {
		return nil, errors.Join(ioErrs...)
	}
	out := make([]string, 0, len(prog.Modules))
	for n := range prog.Modules {
		out = append(out, n)
	}
	slices.Sort(out)
	return out, nil
}

// withImports returns src with `import M` inserted for every module of appMods but App, name
// and those src already imports: after src's `module` line (its first line that is neither
// blank nor a `--` comment, when that starts with `module`), or at the top after a synthesized
// `module <name>`. It also returns the number of lines inserted and the (1-based) lines of the
// inserted imports.
func withImports(src, name string, appMods []string) (string, int, map[int]bool) {
	lines := strings.Split(src, "\n")
	have := map[string]bool{"App": true, name: true}
	modLine, declared, imports := headerLines(src) // only the header's imports count
	if declared != "" {
		have[declared] = true
	}
	for _, m := range imports {
		have[m] = true
	}
	var ins []string
	if modLine < 0 {
		ins = append(ins, "module "+name)
	}
	for _, m := range appMods {
		if !have[m] {
			ins = append(ins, "import "+m)
		}
	}
	at := modLine + 1 // 0 when synthesized
	implicit := map[int]bool{}
	for i, l := range ins {
		if strings.HasPrefix(l, "import ") {
			implicit[at+i+1] = true
		}
	}
	out := slices.Concat(lines[:at], ins, lines[at:])
	return strings.Join(out, "\n"), len(ins), implicit
}

// Run executes b.Binary in the working directory with env appended to the environment and
// stdin as its standard input, for at most Timeout, and returns its standard output, standard
// error and exit code; see RunTimeout. A binary that cannot be started, or is killed at the
// timeout, has exit code -1, with the reason appended to its standard error.
func Run(b Build, env []string, stdin string) (string, string, int) {
	return RunTimeout(b, env, stdin, Timeout)
}
