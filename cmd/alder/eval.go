package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"

	"alder/internal/buildkit"
	"alder/internal/diag"
	"alder/internal/scratch"
	"alder/internal/style"
	"alder/internal/types"
	"alder/rt/db"
)

// evalUsage is the usage line of `alder eval`.
const evalUsage = "usage: alder eval [dir] [--run] [--json] [--build-dir DIR] [--in Name | --module Name] <expr | ->"

// evalResult is the --json output of `alder eval`.
type evalResult struct {
	Value       string            `json:"value"`
	Type        string            `json:"type"`
	Diagnostics []json.RawMessage `json:"diagnostics"`
	Error       string            `json:"error,omitempty"`
	Hint        string            `json:"hint,omitempty"` // a note on a success (`--run` given a Select)
}

// runEvalTo is `alder eval [dir] [--run] [--json] [--build-dir DIR] [--in Name | --module Name] <expr | ->`: it evaluates an
// expression (or, with `-`, standard input: `let name = expr` lines, then an expression) in a
// scratch module that sees every module of the app at dir (default "."), compiled by the real
// compiler, and prints `value : Type`. The scratch module is named Scratch, or Name with
// --module (for an app that has a module named Scratch). With `--in Name` the expression is
// checked as if appended to app module Name: its declarations and imports unqualified, as a
// `test` in that module is written; `--module` names the scratch module of the other mode, and
// the two together are a usage error. A `Query` is shown as `<Query>` unless
// --run runs it against build/<app>/dev.db; a `Select` is shown as `<Select>` (with --run, a
// hint to pipe it to a Db function); a `Task` is refused. --run runs with the app's default
// session (app.TestSession()) and, when adapters/ holds Go files, the adapters' capabilities
// (scratchMain); a capability without an adapter is left unwired, with a warning on stderr,
// as in alder test. The scratch build is a test build (scratch.Options.Tests: the app's tests,
// TestSession and fakes are compiled in; eval is a developer tool, never deployed). Diagnostics
// go to stderr, positioned
// in the input (`input:L:C`). Exit 0 on success, 1 for diagnostics, adapters refused or that
// do not compile with the app (alder build's message), or a failing (or interrupted)
// evaluation, 2 for usage, I/O and internal errors. --json prints one object
// {value, type, diagnostics, error, hint} on stdout instead. Evaluations of one app serialize on
// build/<app>/scratch/eval.lock (scratch.Compile's lock), held from the build to the end of the
// run; one that has to wait says so on stderr. Ctrl-C (SIGINT) or SIGTERM stops the wait, the
// build, or the run (killing the evaluation's processes).
func runEvalTo(args []string, stdin io.Reader, stdout, stderr io.Writer) (code int) {
	jsonOut := false
	res := evalResult{Diagnostics: []json.RawMessage{}}
	failf := func(c int, format string, a ...any) int {
		msg := fmt.Sprintf(format, a...)
		if jsonOut {
			res.Value, res.Type, res.Error = "", "", msg
		} else {
			fmt.Fprintln(stderr, msg)
		}
		return c
	}
	defer func() {
		if r := recover(); r != nil {
			code = failf(2, "alder eval: internal error: %v", r)
		}
		if jsonOut {
			enc := json.NewEncoder(stdout)
			enc.SetEscapeHTML(false)
			enc.Encode(res)
		}
	}()

	var pos []string
	run, named := false, false
	modName, inName, bd := "Scratch", "", ""
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--build-dir" && i+1 < len(args):
			i++
			bd = args[i]
		case strings.HasPrefix(a, "--build-dir="):
			bd = strings.TrimPrefix(a, "--build-dir=")
		case a == "--json":
			jsonOut = true
		case a == "--run":
			run = true
		case a == "--module" && i+1 < len(args):
			i++
			modName, named = args[i], true
		case strings.HasPrefix(a, "--module="):
			modName, named = strings.TrimPrefix(a, "--module="), true
		case strings.HasPrefix(a, "--in=") || a == "--in" && i+1 < len(args):
			name, next := inFlagValue(args, i)
			inName, i = name, next
		case strings.HasPrefix(a, "--"):
			return failf(2, "%s", evalUsage)
		default:
			pos = append(pos, a)
		}
	}
	if err := errInFlags(inName, named); err != nil {
		return failf(2, "alder eval: %v\n%s", err, evalUsage)
	}
	if !moduleNameRE.MatchString(modName) {
		return failf(2, "alder eval: --module wants a module name such as Probe, got %q", modName)
	}
	dir, expr := ".", ""
	switch len(pos) {
	case 1:
		if pos[0] != "-" && dirHasApp(pos[0]) {
			return failf(2, "alder eval: expected an expression after the app directory %s\n%s", pos[0], evalUsage)
		}
		expr = pos[0]
	case 2:
		dir, expr = pos[0], pos[1]
	default:
		return failf(2, "%s", evalUsage)
	}
	if expr == "-" {
		b, err := io.ReadAll(stdin)
		if err != nil {
			return failf(2, "alder eval: %v", err)
		}
		expr = string(b)
	}
	es, err := evalModule(expr)
	if err != nil {
		return failf(2, "alder eval: %v", err)
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return failf(2, "alder eval: %v", err)
	}
	if !dirHasApp(abs) {
		return failf(2, "alder eval: no .alder files under %s/app", dir)
	}
	appBuild := filepath.Join(buildRoot(bd), filepath.Base(abs))

	sessionName := modName
	if inName != "" {
		sessionName = inName
	}
	mod := &scratch.Module{Name: modName, In: inName, Source: es.src, Infer: true}
	checked := scratch.Check(dir, mod)
	if nm := (*scratch.NoModuleError)(nil); errors.As(checked.Err, &nm) {
		return failf(2, "alder eval: %v", nm)
	}
	if exists := (*scratch.ModuleExistsError)(nil); errors.As(checked.Err, &exists) {
		if named {
			return failf(2, "alder eval: %s; give --module another name", exists.Taken())
		}
		return failf(2, "alder eval: %s; pass --module <Name> to name eval's module otherwise", exists.Taken())
	}
	if checked.Err != nil {
		return failf(2, "alder eval: %v", checked.Err)
	}
	if inName != "" && checked.Declares("it") {
		return failf(2, "alder eval: %s declares it, the name eval gives the expression", inName)
	}
	errs := evalDiags(dir, checked.Diags, checked.MapDiag, es, checked.InSession, jsonOut, &res, stderr)
	if errs {
		return 1
	}
	if checked.Result == nil {
		return failf(2, "alder eval: internal error: the program was not checked")
	}
	if checked.Result.Holes > 0 {
		return failf(1, "alder eval: the input has %d typed hole(s); fill them to evaluate it", checked.Result.Holes)
	}
	sig, ok := checked.Result.Values[sessionName+".it"]
	if !ok {
		return failf(2, "alder eval: internal error: no type was inferred for the expression")
	}
	sig = checked.Result.U.Apply(sig)
	typ := checked.TypeString(sig, sessionName)

	kind := scratch.MainPure
	if app, ok := sig.(*types.App); ok {
		switch app.Con {
		case "Task":
			return failf(1, "alder eval: a Task is not run by eval (the REPL's :run! will); its type is %s", typ)
		case "Select":
			if run {
				res.Hint = "pipe it to Db.all, Db.first or Db.count to run it"
				if !jsonOut {
					fmt.Fprintf(stderr, "alder eval: a Select is not a Query: %s\n", res.Hint)
				}
			}
			return evalPrint(stdout, jsonOut, &res, "<Select>", typ)
		case "Query":
			if !run {
				return evalPrint(stdout, jsonOut, &res, "<Query>", typ)
			}
			kind, typ = scratch.MainQuery, checked.TypeString(app.Args[0], sessionName)
		}
	}
	var env []string
	mainGo := scratch.Main(kind, checked.WiresAdapters(), scratch.GoValue(sessionName, "it", sig, false))
	if kind == scratch.MainQuery {
		dbPath, err := filepath.Abs(filepath.Join(appBuild, "dev.db"))
		if err != nil {
			return failf(2, "alder eval: %v", err)
		}
		if err := os.MkdirAll(appBuild, 0o755); err != nil {
			return failf(2, "alder eval: %v", err)
		}
		env = append(env, "ALDER_EVAL_DB="+dbPath)
	}

	// Ctrl-C (or SIGTERM) stops the wait for the lock, the build and the run. The program runs
	// in its own process group, out of the terminal's reach: an interrupt reaches eval alone,
	// which kills the program's group and stops
	sigCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	// CompileCheckedWith builds what was checked above (no second check) under the lock, which
	// the Build holds until Close; another eval of the app holding it is waited for, said once
	waiting := func() { fmt.Fprintln(stderr, "alder eval: waiting for another alder eval of this app…") }
	b := scratch.CompileCheckedWith(sigCtx, checked, mainGo, "eval", filepath.Join(appBuild, "scratch", "eval"), scratch.Options{Waiting: waiting, Tests: true, Fetching: func() { fmt.Fprintln(stderr, buildkit.FetchingLine) }})
	defer b.Close()
	if sigCtx.Err() != nil {
		return failf(1, "alder eval: interrupted")
	}
	io.WriteString(stderr, b.Warning) // capabilities without adapters run unwired
	if b.Err != nil {
		return failf(buildErrCode(b.Err), "alder eval: %v", b.Err)
	}
	if !b.Runnable() { // the check above passed, so this is not expected
		if evalDiags(dir, b.Diags, b.MapDiag, es, b.InSession, jsonOut, &res, stderr) {
			return 1
		}
		return failf(2, "alder eval: internal error: nothing was built")
	}
	ctx, cancel := context.WithTimeoutCause(sigCtx, scratch.Timeout, fmt.Errorf("timed out after %v", scratch.Timeout))
	defer cancel()
	out, errOut, exit := scratch.RunTethered(ctx, b, env)
	if sigCtx.Err() != nil {
		return failf(1, "alder eval: interrupted")
	}
	switch msg, outcome := scratch.ClassifyRun(errOut, exit); outcome {
	case scratch.RunInternal: // could not start
		return failf(2, "alder eval: %s", msg)
	case scratch.RunOK:
	default:
		return failf(1, "alder eval: %s", devDBHint(msg, appBuild))
	}
	return evalPrint(stdout, jsonOut, &res, out, typ)
}

// devDBHint rewrites the database's dev-mode remedy (db.DevRemedy, which ends Ensure's and
// Migrate's refusals: what the app's server says) for eval and the REPL, which never recreate
// the database.
func devDBHint(msg, appBuild string) string {
	return strings.Replace(msg, db.DevRemedy,
		"to start over, start `alder run` (it recreates "+filepath.Join(appBuild, "dev.db")+" empty: every row is deleted) or delete it", 1)
}

// moduleNameRE is a one-segment module name, for --module.
var moduleNameRE = regexp.MustCompile(`^[A-Z][A-Za-z0-9]*(_[A-Za-z0-9]+)*$`)

// appModuleRE is an app module's name, dotted for a nested one (Todos.Index), for --in.
var appModuleRE = scratch.AppModuleRE

// inFlagValue reads the value of the `--in Name` or `--in=Name` at args[i] and returns it with
// the index of the last argument it used. The caller has checked that args[i] is one of the two
// forms, the second with a following argument. eval and the REPL share it; the name is
// validated by errInFlags.
func inFlagValue(args []string, i int) (name string, last int) {
	if v, ok := strings.CutPrefix(args[i], "--in="); ok {
		return v, i
	}
	return args[i+1], i + 1
}

// errInFlags is nil when the --in given (inName; "" for none) can be taken, with or without a
// --module (named), else the usage error: --in and --module do not combine, and --in wants an
// app module name (appModuleRE).
func errInFlags(inName string, named bool) error {
	switch {
	case inName == "":
		return nil
	case named:
		return errors.New("--in and --module do not combine: --in Name works inside module Name, --module names the scratch module")
	case !appModuleRE.MatchString(inName):
		return fmt.Errorf("--in wants an app module name such as Todo or Todos.Index, got %q", inName)
	}
	return nil
}

// evalPrint reports a value: `value : Type` on stdout, or into the JSON result.
func evalPrint(stdout io.Writer, jsonOut bool, res *evalResult, value, typ string) int {
	if jsonOut {
		res.Value, res.Type = value, typ
		return 0
	}
	fmt.Fprintf(stdout, "%s%s\n", value, style.For(stdout).Dim(" : "+typ))
	return 0
}

// evalDiags reports ds (to stderr, or into res for --json), those inSession accepts (the
// session's own lines, Build.InSession) positioned in the input as file "input", the app's with their own source lines, and says whether one is
// an error.
func evalDiags(dir string, ds []diag.Diagnostic, mapLine func(diag.Diagnostic) (diag.Diagnostic, bool), es evalSource, inSession func(diag.Diagnostic) bool, jsonOut bool, res *evalResult, stderr io.Writer) bool {
	failed := false
	appLines := map[string][]string{}
	for _, d := range ds {
		if d.Severity == "error" {
			failed = true
		}
		var lines []string
		if inSession(d) {
			d = evalMapDiag(d, mapLine, es)
			lines = es.Lines
		} else {
			lines = appSourceLines(dir, appLines, d.Span.File)
		}
		if jsonOut {
			res.Diagnostics = append(res.Diagnostics, json.RawMessage(diag.RenderJSON(d)))
		} else {
			fmt.Fprint(stderr, diag.RenderTextStyled(d, lines, style.For(stderr)))
		}
	}
	return failed
}

// evalMapDiag positions a diagnostic of the scratch module in the input, as file "input"
// (evalSource's SourceMap.Position) and rewrites the messages about `it`: the parser's "in the
// definition of `it`: " is dropped, `it` being a name the input never wrote.
func evalMapDiag(d diag.Diagnostic, mapLine func(diag.Diagnostic) (diag.Diagnostic, bool), es evalSource) diag.Diagnostic {
	d = es.Position(d, mapLine, "input")
	d.Message = strings.TrimPrefix(d.Message, "in the definition of `it`: ")
	if strings.HasPrefix(d.Message, "the type of `it` cannot be inferred") {
		d.Message = "the expression uses `it`, which eval does not define"
	}
	return d
}

// dirHasApp reports whether dir/app holds an .alder file.
func dirHasApp(dir string) bool {
	found := false
	filepath.WalkDir(filepath.Join(dir, "app"), func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(p, ".alder") {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

// findAppRoot is the nearest directory at or above start that holds an app (dirHasApp), "" when
// there is none.
func findAppRoot(start string) string {
	for d := start; ; d = filepath.Dir(d) {
		if dirHasApp(d) {
			return d
		}
		if filepath.Dir(d) == d {
			return ""
		}
	}
}

// buildErrCode is the exit code of a failed build or run: 1 for the user's adapters
// (scratch.AdapterError) and the machine's network (buildkit.DownloadError), as alder build says
// them, else 2 (internal).
func buildErrCode(err error) int {
	if adapters := (*scratch.AdapterError)(nil); errors.As(err, &adapters) {
		return 1
	}
	if de := (*buildkit.DownloadError)(nil); errors.As(err, &de) {
		return 1
	}
	return 2
}

// appSourceLines is the lines of the app's file (relative to dir) for a diagnostic's source
// excerpt, read once into cache; nil when it cannot be read.
func appSourceLines(dir string, cache map[string][]string, file string) []string {
	l, ok := cache[file]
	if !ok {
		if b, err := os.ReadFile(filepath.Join(dir, file)); err == nil {
			l = strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n")
		}
		cache[file] = l
	}
	return l
}
