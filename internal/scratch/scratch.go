// Package scratch compiles an app together with one extra module held in memory (the scratch
// module) and a Go main of the caller's, and runs the result. It is the engine under `alder
// eval`, `alder test`, the REPL and the book verifier: each supplies the scratch module's
// source and a main that calls into the generated package "app" (a top-level value `it` of
// module Scratch is app.Scratch__it), and reads what the program prints.
package scratch

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"alder/internal/buildkit"
	"alder/internal/check"
	"alder/internal/diag"
	"alder/internal/gen"
	"alder/internal/ir"
	"alder/internal/names"
	"alder/internal/syntax"
	"alder/internal/types"
)

// Module is a module held in memory. Name is its module name ("Scratch" when empty) and, with
// ".alder", the file its diagnostics name; Source may omit the `module` line. Infer infers the
// signatures of its top-level values written without one, instead of reporting E0007, and lets
// it use any module's brand constructors, as a `test` may (check.Options.InferModule).
type Module struct {
	Name, Source string
	Infer        bool
}

// ModuleExistsError is Check's (and Compile's) error when the extra module's name is taken: by
// a module of the app, or (Std) by a standard library module. The two would be one module.
type ModuleExistsError struct {
	Name string
	Std  bool // the name is a standard library module's
}

func (e *ModuleExistsError) Error() string {
	return e.Taken() + "; give the extra module another name"
}

// Taken says who has the name: "the app already has a module named X" or "X is a standard
// library module".
func (e *ModuleExistsError) Taken() string {
	if e.Std {
		return e.Name + " is a standard library module"
	}
	return "the app already has a module named " + e.Name
}

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

	unlock func() // releases the lock on out (Close); nil when not held
}

// Close releases the lock a successful Compile holds on its output directory (see Compile). It
// is safe to call more than once, and on any Build.
func (b *Build) Close() {
	if b.unlock != nil {
		b.unlock()
		b.unlock = nil
	}
}

// Timeout bounds a Run.
const Timeout = 60 * time.Second

// Compile builds the app at appDir with extra (nil for none) added as one more module, and the
// caller's Go command mainGo (package main, importing "app"), into the output directory out;
// out "" is build/<app>/scratch/<purpose> under the working directory (purpose "" is "default").
// It is Check followed by CompileChecked.
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
//     set only for I/O failures, an extra module named like an app or stdlib module
//     (*ModuleExistsError), generated or caller Go that does not vet, and go build failures;
//     its message carries Go's output.
//   - Compile writes only under out (the generated module "app", adapters/, static/ and
//     migrations/ copied from the app, cmd/main.go and the executable out/scratch) and the
//     lock file <out>.lock; it never touches build/<app> itself. Nothing is written when the
//     check fails, and the previous executable in out stays until a new one is built, so a
//     failed Compile leaves the last good one runnable.
//   - Compiles into one out serialize: Compile takes an exclusive lock on <out>.lock (flock;
//     the kernel drops it if the process dies) before writing, waiting while another holds it
//     (CompileCheckedWith's ctx ends the wait; its Options.Waiting is told), and a Runnable
//     Build keeps it until Close, so no other Compile replaces the executable while the caller
//     runs it. A Build that is not Runnable holds no lock. Callers must Close a Runnable Build.
//   - Compiles into the same out reuse Go's build cache (a second build of an unchanged app is
//     much faster).
func Compile(appDir string, extra *Module, mainGo string, purpose, out string) Build {
	return CompileChecked(Check(appDir, extra), mainGo, purpose, out)
}

// CompileChecked is the second half of Compile: it builds what c checked (Check's result for
// the same app and extra module), so a caller that checked first does not check twice. It is
// CompileCheckedWith with no cancellation and the default Options.
func CompileChecked(c Checked, mainGo string, purpose, out string) Build {
	return CompileCheckedWith(context.Background(), c, mainGo, purpose, out, Options{})
}

// Options adjust CompileCheckedWith.
type Options struct {
	// NoVet skips `go vet` of the generated module before `go build` (the REPL: the app vetted
	// when it was built, and vet costs a build's time again). When the build then fails, the
	// module is vetted after all, so the failure is reported as without NoVet: generated Go
	// that does not vet is an internal error, a main or adapters that do not vet say so.
	NoVet bool
	// Waiting, when set, is called once when another process (or Build) holds the lock on the
	// output directory, before CompileCheckedWith waits for it (a done ctx ends the wait).
	Waiting func()
}

// CompileCheckedWith is CompileChecked with options, stopped when ctx is done: `go vet` and `go
// build` are killed (so a Ctrl-C during a build stops it), and the Build has no Binary and Err
// ctx's error (context.Canceled or DeadlineExceeded; the caller, which owns ctx, says what
// happened).
func CompileCheckedWith(ctx context.Context, c Checked, mainGo string, purpose, out string, o Options) (b Build) {
	defer func() {
		if r := recover(); r != nil {
			b.Close()
			b.Binary, b.Err = "", fmt.Errorf("internal error: %v", r)
		}
	}()
	b.Diags, b.Err, b.LineOffset, b.InsertLine, b.ExtraFile = c.Diags, c.Err, c.LineOffset, c.InsertLine, c.ExtraFile
	prog, res := c.program, c.Result
	if b.Err != nil || res == nil || c.hasErrors || res.Holes > 0 {
		return b
	}
	defer func() {
		if ctx.Err() != nil { // whatever failed, it failed because it was stopped
			b.Close()
			b.Binary, b.Err = "", ctx.Err()
		}
	}()
	appDir := c.appDir
	abs, err := filepath.Abs(appDir)
	if err != nil {
		b.Err = err
		return b
	}
	if out == "" {
		if purpose == "" {
			purpose = "default"
		}
		out = filepath.Join("build", filepath.Base(abs), "scratch", purpose)
	}
	if b.Dir, err = filepath.Abs(out); err != nil {
		b.Err = err
		return b
	}
	unlock, err := lockFile(ctx, b.Dir+".lock", o.Waiting)
	if err != nil {
		b.Err = err
		return b
	}
	b.unlock = unlock
	defer func() {
		if b.Binary == "" {
			b.Close() // nothing to run: release at once
		}
	}()

	repo, err := buildkit.RepoRoot()
	if err != nil {
		b.Err = err
		return b
	}
	files, err := gen.Emit(ir.Lower(prog, res), gen.Options{RepoPath: repo, Migrations: buildkit.HasMigrations(appDir)})
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
	if err := buildkit.CopyMigrations(filepath.Join(appDir, "migrations"), filepath.Join(b.Dir, "migrations")); err != nil {
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
	if !o.NoVet {
		if b.Err = vet(ctx, b.Dir, files); b.Err != nil {
			return b
		}
	}
	if buildOut, err := buildkit.GoCmdContext(ctx, b.Dir, "build", "-mod=mod", "-o", "scratch", "./cmd"); err != nil {
		if ctx.Err() != nil {
			return b
		}
		if o.NoVet { // what vet would have said first
			if b.Err = vet(ctx, b.Dir, files); b.Err != nil {
				return b
			}
		}
		b.Err = fmt.Errorf("go build of %s failed: %v\n%s", b.Dir, err, buildOut)
		return b
	}
	b.Binary = filepath.Join(b.Dir, "scratch")
	return b
}

// vet runs `go vet` on the scratch module in dir (files are the generated ones) and returns
// nil, or why it does not vet: buildkit.VetApp's error when the generated package is at fault,
// else the main's or the adapters'.
func vet(ctx context.Context, dir string, files map[string][]byte) error {
	vetOut, err := buildkit.GoCmdContext(ctx, dir, "vet", "-mod=mod", "./...")
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if verr := buildkit.VetApp(dir, files, "."); verr != nil {
		return verr
	}
	return fmt.Errorf("the Go in %s (the main or the adapters) does not vet: %v\n%s", dir, err, vetOut)
}

// Checked is the outcome of Check.
type Checked struct {
	Diags      []diag.Diagnostic // as Build.Diags
	Err        error             // as Build.Err, for an I/O failure; never a diagnostic
	LineOffset int               // as Build.LineOffset
	InsertLine int               // as Build.InsertLine
	ExtraFile  string            // as Build.ExtraFile
	// Result is what the checker learned (Result.Values holds the extra module's signatures,
	// inferred ones too, as "<Name>.<value>"); nil when Err is set or a module failed to parse.
	// When Diags hold an error the program is not to be lowered.
	Result *check.Result

	program   *names.Program // the checked program, for CompileChecked
	hasErrors bool           // Diags hold an error
	appDir    string         // the app checked
}

// MapDiag is Build.MapDiag for a Check: it maps a diagnostic of the extra module to the
// caller's source.
func (c Checked) MapDiag(d diag.Diagnostic) (diag.Diagnostic, bool) {
	return Build{LineOffset: c.LineOffset, InsertLine: c.InsertLine, ExtraFile: c.ExtraFile}.MapDiag(d)
}

// Check loads and checks the app at appDir with extra added, the first half of Compile, and
// stops there: nothing is generated or written. A caller that must know a value's type before
// choosing the main it compiles (`alder eval` on a Query) checks first.
func Check(appDir string, extra *Module) (c Checked) {
	c.appDir = appDir
	defer func() {
		if r := recover(); r != nil {
			c.Result, c.program, c.Err = nil, nil, fmt.Errorf("internal error: %v", r)
		}
	}()
	var mods []*syntax.Module
	var implicit map[int]bool
	var opts check.Options
	if extra != nil {
		modName := extra.Name
		if modName == "" {
			modName = "Scratch"
		}
		if err := checkDeclaredName(extra.Source, modName); err != nil {
			c.Err = err
			return c
		}
		appMods, err := appModules(appDir)
		if err != nil {
			c.Err = err
			return c
		}
		if slices.Contains(appMods, modName) {
			c.Err = &ModuleExistsError{Name: modName}
			return c
		}
		std, err := types.LoadStd()
		if err != nil {
			c.Err = fmt.Errorf("internal error loading stdlib: %v", err)
			return c
		}
		if _, isStd := std[modName]; isStd {
			c.Err = &ModuleExistsError{Name: modName, Std: true}
			return c
		}
		if extra.Infer {
			opts.InferModule = modName
		}
		src, offset, lines := withImports(extra.Source, modName, appMods)
		c.LineOffset, implicit = offset, lines
		c.InsertLine, c.ExtraFile = insertInfo(extra.Source, modName, offset)
		m, ds := syntax.ParseFile(modName+".alder", []byte(src))
		if len(ds) > 0 {
			c.Diags = ds
			return c
		}
		mods = append(mods, m)
	}
	prog, res, bag, ioErrs := buildkit.LoadCheckedWith(appDir, mods, opts)
	if len(ioErrs) > 0 {
		c.Err = errors.Join(ioErrs...)
		return c
	}
	if len(prog.Modules) == len(mods) {
		c.Err = fmt.Errorf("no .alder files under %s/app", appDir)
		return c
	}
	buildkit.AddSchemaDrift(appDir, prog, res, bag) // E0019: a test, eval or REPL runs on the app's migrations
	for _, d := range bag.Items {
		if d.Code == diag.UnusedImport && extra != nil && d.Span.File == mods[0].File && implicit[d.Span.Start.Line] {
			continue
		}
		c.Diags = append(c.Diags, d)
	}
	c.Result, c.program, c.hasErrors = res, prog, bag.HasErrors()
	return c
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
// error and exit code; see RunContext. A binary that cannot be started, or is killed at the
// timeout, has exit code -1, with the reason appended to its standard error.
func Run(b Build, env []string, stdin string) (string, string, int) {
	return RunTimeout(b, env, stdin, Timeout)
}
