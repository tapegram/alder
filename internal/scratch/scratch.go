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
	"alder/internal/version"
)

// Module is a module held in memory. Name is its module name ("Scratch" when empty) and, with
// ".alder", the file its diagnostics name; Source may omit the `module` line. Infer infers the
// signatures of its top-level values written without one, instead of reporting E0007, and lets
// it use any module's brand constructors, as a `test` may (check.Options.InferModule).
type Module struct {
	Name, Source string
	Infer        bool
	// In names an app module the Source is appended to instead of being a module of its own
	// (`alder eval --in Todo`): the session sees the module's declarations and imports
	// unqualified, as its own code does. Name is ignored; Infer applies to that module.
	In string
}

// NoModuleError is Check's error when Module.In names no module of the app.
type NoModuleError struct{ Name, App string }

func (e *NoModuleError) Error() string { return "no module " + e.Name + " in " + e.App }

// ModulePath is the file of an app module: "app/Todos/Index.alder" for Todos.Index.
func ModulePath(name string) string { return "app/" + strings.ReplaceAll(name, ".", "/") + ".alder" }

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
	Diags  []diag.Diagnostic // check diagnostics, sorted: the app's (File "app/…") and the extra module's (File "<Name>.alder", or "app/<In>.alder" in in-module mode)
	Err    error             // an I/O, vet or go build failure (with Go's output); never a diagnostic
	// LineOffset is the number of lines Compile inserted into the extra module's source: a
	// diagnostic of the extra module at line L is at line L-LineOffset of the caller's source
	// (for L after the inserted lines; a source without a `module` line has them at the top).
	//
	// Callers should map diagnostics with MapDiag (or lines with SourceLine), which also handle
	// the lines before and on the inserted ones.
	LineOffset int
	InsertLine int    // the first inserted line (1-based) of the rewritten source; 0 when none
	ExtraFile  string // the extra module's file, "<Name>.alder" or (Module.In) "app/<In>.alder"; "" without one
	// BaseLines is, in in-module mode (Module.In), the number of lines of the module's own file:
	// a diagnostic of ExtraFile at a line past BaseLines is the caller's, at line L-BaseLines;
	// one at or before it is the module's own and is reported as it is. 0 otherwise.
	BaseLines int
	// Warning, for a test build (Options.Tests) of an app missing adapters, is the warning to
	// print once (buildkit.CheckTestAdapters): those capabilities run unwired. "" when none.
	Warning string

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

// AdapterError is Build.Err when the app's adapters are refused or do not compile with the
// app: the user's program is at fault, as with a check error (the commands exit 1). Msg is
// what alder build prints: buildkit.CheckAdapters' lines, or "the adapters of <app> (copied to
// <dir>) do not compile with the app: …" with Go's output.
type AdapterError struct{ Msg string }

func (e *AdapterError) Error() string { return e.Msg }

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
//     the app ("app/Todo.alder"); the extra module's is "<Name>.alder" (in in-module mode,
//     Module.In, the module's own file, ModulePath(In)). Diags may hold warnings alone and
//     still produce a binary.
//   - Binary is "" when Diags hold an error or a typed hole (W0002), or when Err is set. Err is
//     set only for I/O failures, an extra module named like an app or stdlib module
//     (*ModuleExistsError), adapters that are refused or do not compile with the app
//     (*AdapterError: the user's, as a check error is), the runtime's dependencies that cannot
//     be downloaded (*buildkit.DownloadError: the machine's), generated or caller Go that does not
//     vet, and go build failures; its message carries Go's output.
//   - Compile writes only under out (the generated module "app" with its caps/ when it wires
//     the adapters (Checked.WiresAdapters), the runtime it requires in rt/ with go.sum
//     (buildkit.WriteRuntime), adapters/, static/ and migrations/ copied from the app,
//     cmd/main.go and the executable out/scratch) and the lock file <out>.lock; it never
//     touches build/<app> itself. Nothing is written when the check fails, and the previous
//     executable in out stays until a new one is built, so a failed Compile leaves the last
//     good one runnable.
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
	// Fetching, when set, is called once before the runtime's dependencies are downloaded into
	// Go's module cache (buildkit.DownloadDeps), which happens on a machine's first build; the
	// commands print buildkit.FetchingLine. A failed download is Build.Err, a
	// *buildkit.DownloadError.
	Fetching func()
	// Tests makes a test build (gen.Options.Tests, for `alder test`, `alder eval` and the REPL,
	// whose scratch mains use app.TestSession()): the app's tests (Tests, TestSession), its fakes,
	// their tables (FakeTables) and Test.fake's capability swap are compiled in. Without it the
	// generated module is production-shaped, as `alder build`'s: no test code at all.
	Tests bool
	// Mutate, with Tests, makes the mutate build of `alder test --mutate` (gen.Options.Mutate):
	// every mutant compiled in as a switch on app.Mutant, and the table app.Mutants.
	Mutate bool
	// Thunks names the constants ("Module.name") to emit as functions of no parameters
	// (gen.Options.Thunks): a scratch file's watches, called by its main (RunFile, GoValue).
	Thunks map[string]bool
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
	b.Diags, b.Err, b.LineOffset, b.InsertLine, b.ExtraFile, b.BaseLines = c.Diags, c.Err, c.LineOffset, c.InsertLine, c.ExtraFile, c.BaseLines
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

	p := ir.Lower(prog, res)
	adapters := filepath.Join(appDir, "adapters")
	hasAdapters := buildkit.HasAdapters(appDir)
	wire := c.WiresAdapters() // the caller's main imports app/caps exactly when this holds
	var unwired map[string]bool
	if wire {
		var msg strings.Builder
		ok := true
		if o.Tests { // a missing adapter runs unwired, with a warning
			var missing []buildkit.Need
			missing, b.Warning, ok = buildkit.CheckTestAdapters(appDir, p.Caps, &msg)
			for _, n := range missing {
				if unwired == nil {
					unwired = map[string]bool{}
				}
				unwired[n.Module+"."+n.Cap] = true
			}
		} else {
			ok = buildkit.CheckAdapters(appDir, p.Caps, &msg)
		}
		if !ok {
			b.Err = &AdapterError{strings.TrimSuffix(msg.String(), "\n")}
			return b
		}
	}
	files, err := gen.Emit(p, gen.Options{Version: version.String(), Migrations: buildkit.HasMigrations(appDir), Tests: o.Tests, Mutate: o.Mutate, Unwired: unwired, Thunks: o.Thunks})
	if err != nil {
		b.Err = fmt.Errorf("internal error: %v", err)
		return b
	}
	if !wire {
		delete(files, gen.CapsFile) // nothing to wire: the main uses app.Caps{}
	}
	if err := buildkit.WriteGenerated(b.Dir, files); err != nil {
		b.Err = err
		return b
	}
	if err := buildkit.WriteRuntime(b.Dir); err != nil {
		b.Err = err
		return b
	}
	if err := buildkit.DownloadDeps(ctx, b.Dir, o.Fetching); err != nil {
		b.Err = err // a *buildkit.DownloadError: the machine's network, not the program
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
	vetMod := func() error { return vet(ctx, b.Dir, files, abs, wire || hasAdapters) }
	if !o.NoVet {
		if b.Err = vetMod(); b.Err != nil {
			return b
		}
	}
	if buildOut, err := buildkit.GoCmdContext(ctx, b.Dir, "build", "-mod=mod", "-o", "scratch", "./cmd"); err != nil {
		if ctx.Err() != nil {
			return b
		}
		if o.NoVet { // what vet would have said first
			if b.Err = vetMod(); b.Err != nil {
				return b
			}
		}
		b.Err = fmt.Errorf("go build of %s failed: %v\n%s", b.Dir, err, buildOut)
		return b
	}
	b.Binary = filepath.Join(b.Dir, "scratch")
	return b
}

// vet runs `go vet` on the scratch module in dir (files are the generated ones; appDir is the
// app's absolute directory; adapters: the module has adapters/ or caps/) and returns nil, or
// why it does not vet: buildkit.VetApp's error when the generated package is at fault; else,
// when the adapters and the caps package wiring them do not vet on their own, an AdapterError
// with alder build's message; else the main's.
func vet(ctx context.Context, dir string, files map[string][]byte, appDir string, adapters bool) error {
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
	if adapters {
		var pkgs []string
		if _, ok := files[gen.CapsFile]; ok {
			pkgs = append(pkgs, "./caps")
		}
		if buildkit.IsDir(filepath.Join(dir, "adapters")) {
			pkgs = append(pkgs, "./adapters/...")
		}
		if len(pkgs) > 0 {
			if aOut, aErr := buildkit.GoCmdContext(ctx, dir, append([]string{"vet", "-mod=mod"}, pkgs...)...); aErr != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				copied := filepath.Join(dir, "adapters")
				if wd, err := os.Getwd(); err == nil {
					if rel, err := filepath.Rel(wd, copied); err == nil && !strings.HasPrefix(rel, "..") {
						copied = rel // as alder build names it: build/<app>/…
					}
				}
				msg := buildkit.AdaptersCompileMessage(filepath.Base(appDir), copied, aErr, aOut)
				return &AdapterError{strings.TrimRight(msg, "\n")}
			}
		}
	}
	return fmt.Errorf("the Go main in %s does not vet: %v\n%s", dir, err, vetOut)
}

// Checked is the outcome of Check.
type Checked struct {
	Diags      []diag.Diagnostic // as Build.Diags
	Err        error             // as Build.Err, for an I/O failure; never a diagnostic
	LineOffset int               // as Build.LineOffset
	InsertLine int               // as Build.InsertLine
	ExtraFile  string            // as Build.ExtraFile
	BaseLines  int               // as Build.BaseLines
	// Result is what the checker learned (Result.Values holds the extra module's signatures,
	// inferred ones too, as "<Name>.<value>", or "<In>.<value>" in in-module mode); nil when
	// Err is set or a module failed to parse. When Diags hold an error the program is not to be
	// lowered.
	Result *check.Result

	program   *names.Program  // the checked program, for CompileChecked
	hasErrors bool            // Diags hold an error
	appDir    string          // the app checked
	baseDecls map[string]bool // in-module mode: the values the module declares itself
}

// Declares reports whether the module of in-module mode declares value name itself (not
// through the session); false outside that mode.
func (c Checked) Declares(name string) bool { return c.baseDecls[name] }

// WiresAdapters reports whether a build of what c checked wires the app's adapters: the app
// has capabilities and adapters (a .go file under adapters/). Then CompileChecked checks them
// first and writes the generated package caps, and the caller's main must serve them with
// caps.Caps() (importing "app/caps"); otherwise the main uses app.Caps{}, and a capability
// call finds no implementation. The check is buildkit.CheckAdapters' (a refusal is an
// *AdapterError with the lines alder build prints), except that a test build
// (Options.Tests) leaves a missing adapter unwired, as if there were no adapters, and says so
// in Build.Warning (buildkit.CheckTestAdapters).
func (c Checked) WiresAdapters() bool {
	if c.Result == nil || !buildkit.HasAdapters(c.appDir) {
		return false
	}
	for _, m := range c.Result.Caps {
		if len(m) > 0 {
			return true
		}
	}
	return false
}

// MapDiag is Build.MapDiag for a Check: it maps a diagnostic of the extra module to the
// caller's source.
func (c Checked) MapDiag(d diag.Diagnostic) (diag.Diagnostic, bool) {
	return c.build().MapDiag(d)
}

// InSession is Build.InSession for a Check.
func (c Checked) InSession(d diag.Diagnostic) bool { return c.build().InSession(d) }

// build is the Build fields that map c's diagnostics.
func (c Checked) build() Build {
	return Build{LineOffset: c.LineOffset, InsertLine: c.InsertLine, ExtraFile: c.ExtraFile, BaseLines: c.BaseLines}
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
	var overlay map[string][]byte
	if extra != nil && extra.In != "" {
		// Only a module of the app, by its exact name: on a case-insensitive file system "TODO"
		// would read app/Todo.alder and load it twice, and "../X" would read outside app/.
		appMods, err := appModules(appDir)
		if err != nil {
			c.Err = err
			return c
		}
		if !slices.Contains(appMods, extra.In) {
			c.Err = &NoModuleError{Name: extra.In, App: appDir}
			return c
		}
		path := ModulePath(extra.In)
		raw, err := os.ReadFile(filepath.Join(appDir, filepath.FromSlash(path)))
		if err != nil {
			if os.IsNotExist(err) {
				c.Err = &NoModuleError{Name: extra.In, App: appDir}
			} else {
				c.Err = err
			}
			return c
		}
		base := string(raw)
		if !strings.HasSuffix(base, "\n") { // the session's first line starts a declaration
			base += "\n"
		}
		c.BaseLines = strings.Count(base, "\n")
		c.ExtraFile, c.InsertLine, c.LineOffset = path, 1, c.BaseLines
		if m, ds := syntax.ParseFile(path, raw); len(ds) == 0 {
			c.baseDecls = map[string]bool{}
			for _, d := range m.Decls {
				if v, ok := d.(*syntax.ValueDecl); ok {
					c.baseDecls[v.Name] = true
				}
			}
		}
		if extra.Infer {
			opts.InferModule = extra.In
		}
		overlay = map[string][]byte{path: []byte(base + extra.Source)}
	} else if extra != nil {
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
		src, offset, lines := withImports(extra.Source, modName, appMods, func(n string) bool { _, ok := std[n]; return ok })
		c.LineOffset, implicit = offset, lines
		c.InsertLine, c.ExtraFile = insertInfo(extra.Source, modName, offset)
		m, ds := syntax.ParseFile(modName+".alder", []byte(src))
		if len(ds) > 0 {
			c.Diags = ds
			return c
		}
		mods = append(mods, m)
	}
	var prog *names.Program
	var res *check.Result
	var bag *diag.Bag
	var ioErrs []error
	if overlay != nil {
		prog, res, bag, ioErrs = buildkit.LoadCheckedOverlayWith(appDir, overlay, opts)
	} else {
		prog, res, bag, ioErrs = buildkit.LoadCheckedWith(appDir, mods, opts)
	}
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
		if d.Code == diag.UnusedImport && len(mods) > 0 && d.Span.File == mods[0].File && implicit[d.Span.Start.Line] {
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
// and those src already imports: after src's `module` line (its first line that is neither blank
// nor a `--` comment, when that starts with `module`), or at the top after a synthesized
// `module <name>`. isStd says which names are standard-library modules. Each inserted import is
// written with its canonical alias (`import Admin.Layout as AdminLayout` when `Layout` is
// taken), counting the imports of src's header (only those, and whatever alias they carry) and
// its declarations; a header import that is not canonical is the caller's to fix, and is
// reported (E0204) on its own line. A module with no usable qualifier beside these (E0206) is
// left out, so that error never lands on a synthesized line. It also returns the number of lines
// inserted and the (1-based) lines of the inserted imports.
func withImports(src, name string, appMods []string, isStd func(string) bool) (string, int, map[int]bool) {
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
	var missing []string
	for _, m := range appMods {
		if !have[m] {
			missing = append(missing, m)
		}
	}
	taken := func(n string) bool { return n == "App" || isStd(n) }
	if pm, _ := syntax.ParseFile(name+".alder", []byte(src)); pm != nil {
		taken = names.Taken(pm, isStd)
	}
	var paths []string
	for _, m := range append(slices.Clone(imports), missing...) {
		if !isStd(m) {
			paths = append(paths, m)
		}
	}
	quals, _ := names.CanonicalQualifiers(name, paths, taken)
	for _, m := range missing {
		if !isStd(m) && quals[m] == "" {
			continue // unqualifiable: not imported implicitly
		}
		if q := quals[m]; q != "" && names.NeedsAlias(m, q) {
			ins = append(ins, "import "+m+" as "+q)
		} else {
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
