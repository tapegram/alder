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
	"alder/internal/gen"
	"alder/internal/scratch"
	"alder/internal/types"
)

// evalUsage is the usage line of `alder eval`.
const evalUsage = "usage: alder eval [dir] [--run] [--json] [--module Name] <expr | ->"

// evalResult is the --json output of `alder eval`.
type evalResult struct {
	Value       string            `json:"value"`
	Type        string            `json:"type"`
	Diagnostics []json.RawMessage `json:"diagnostics"`
	Error       string            `json:"error,omitempty"`
	Hint        string            `json:"hint,omitempty"` // a note on a success (`--run` given a Select)
}

// runEvalTo is `alder eval [dir] [--run] [--json] [--module Name] <expr | ->`: it evaluates an
// expression (or, with `-`, standard input: `let name = expr` lines, then an expression) in a
// scratch module that sees every module of the app at dir (default "."), compiled by the real
// compiler, and prints `value : Type`. The scratch module is named Scratch, or Name with
// --module (for an app that has a module named Scratch). A `Query` is shown as `<Query>` unless
// --run runs it against build/<app>/dev.db; a `Select` is shown as `<Select>` (with --run, a
// hint to pipe it to a Db function); a `Task` is refused. --run runs with the app's default
// session (app.TestSession()) and, when adapters/ holds Go files, the adapters' capabilities
// (scratchMain). Diagnostics go to stderr, positioned
// in the input (`input:L:C`). Exit 0 on success, 1 for diagnostics or a failing (or
// interrupted) evaluation, 2 for usage, I/O and internal errors. --json prints one object
// {value, type, diagnostics, error, hint} on stdout instead. Evaluations of one app serialize on
// build/<app>/scratch/eval.lock (scratch.Compile's lock), held from the build to the end of the
// run; Ctrl-C (SIGINT) or SIGTERM during the run kills the evaluation's processes.
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
	run := false
	modName := "Scratch"
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json":
			jsonOut = true
		case a == "--run":
			run = true
		case a == "--module" && i+1 < len(args):
			i++
			modName = args[i]
		case strings.HasPrefix(a, "--module="):
			modName = strings.TrimPrefix(a, "--module=")
		case strings.HasPrefix(a, "--"):
			return failf(2, "%s", evalUsage)
		default:
			pos = append(pos, a)
		}
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
	appBuild := filepath.Join("build", filepath.Base(abs))

	mod := &scratch.Module{Name: modName, Source: es.src, Infer: true}
	checked := scratch.Check(dir, mod)
	if exists := (*scratch.ModuleExistsError)(nil); errors.As(checked.Err, &exists) {
		return failf(2, "alder eval: %s; pass --module <Name> to name eval's module otherwise", exists.Taken())
	}
	if checked.Err != nil {
		return failf(2, "alder eval: %v", checked.Err)
	}
	errs := evalDiags(dir, checked.Diags, checked.MapDiag, es, modName, jsonOut, &res, stderr)
	if errs {
		return 1
	}
	if checked.Result == nil {
		return failf(2, "alder eval: internal error: the program was not checked")
	}
	if checked.Result.Holes > 0 {
		return failf(1, "alder eval: the input has %d typed hole(s); fill them to evaluate it", checked.Result.Holes)
	}
	sig, ok := checked.Result.Values[modName+".it"]
	if !ok {
		return failf(2, "alder eval: internal error: no type was inferred for the expression")
	}
	sig = checked.Result.U.Apply(sig)
	typ := types.String(sig)

	kind := mainPure
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
			kind, typ = mainQuery, types.String(app.Args[0])
		}
	}
	var env []string
	mainGo := scratchMain(kind, buildkit.HasGoFiles(filepath.Join(dir, "adapters")), evalIt(modName, sig))
	if kind == mainQuery {
		dbPath, err := filepath.Abs(filepath.Join(appBuild, "dev.db"))
		if err != nil {
			return failf(2, "alder eval: %v", err)
		}
		if err := os.MkdirAll(appBuild, 0o755); err != nil {
			return failf(2, "alder eval: %v", err)
		}
		env = append(env, "ALDER_EVAL_DB="+dbPath)
	}

	// CompileChecked builds what was checked above (no second check) under the lock, which the
	// Build holds until Close
	b := scratch.CompileChecked(checked, mainGo, "eval", filepath.Join(appBuild, "scratch", "eval"))
	defer b.Close()
	if b.Err != nil {
		return failf(2, "alder eval: %v", b.Err)
	}
	if !b.Runnable() { // the check above passed, so this is not expected
		if evalDiags(dir, b.Diags, b.MapDiag, es, modName, jsonOut, &res, stderr) {
			return 1
		}
		return failf(2, "alder eval: internal error: nothing was built")
	}
	// the program runs in its own process group, out of the terminal's reach: an interrupt
	// reaches eval alone, which kills the program's group and stops
	sigCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeoutCause(sigCtx, scratch.Timeout, fmt.Errorf("timed out after %v", scratch.Timeout))
	defer cancel()
	out, errOut, exit := scratch.RunContext(ctx, b, env, "")
	switch {
	case sigCtx.Err() != nil:
		return failf(1, "alder eval: interrupted")
	case exit == -1 && !strings.Contains(errOut, "timed out"): // could not start: internal
		return failf(2, "alder eval: %s", strings.TrimSpace(errOut))
	case exit != 0 && (strings.Contains(errOut, "goroutine stack exceeds") || strings.Contains(errOut, "fatal error: stack overflow")):
		// Go's trace of the overflow is hundreds of lines about the generated code
		return failf(1, "alder eval: stack overflow (unbounded recursion?)")
	case exit != 0: // the evaluation failed (or ran past scratch.Timeout)
		if strings.HasPrefix(errOut, "panic: ") { // the Go goroutine dump says nothing about the input
			errOut, _, _ = strings.Cut(errOut, "\n\n")
		}
		return failf(1, "alder eval: %s", strings.TrimSpace(errOut))
	}
	return evalPrint(stdout, jsonOut, &res, out, typ)
}

// evalIt is the Go expression for the scratch module mod's `it` of type sig in the generated
// package: a polymorphic value is a generic thunk (`func Scratch__it[A any]() …`) and a
// polymorphic function a generic function, both instantiated here at `any` (an empty `[]` shows
// the same at any element type); gen gives `it` no other thunk, since nothing can refer to it.
func evalIt(mod string, sig types.Type) string {
	it := "app." + gen.GoName(mod, "it")
	n := len(typeParamsOf(sig))
	if n == 0 {
		return it
	}
	s := it + "[" + strings.TrimSuffix(strings.Repeat("any, ", n), ", ") + "]"
	if _, isFun := sig.(*types.Fun); !isFun {
		s += "()"
	}
	return s
}

// moduleNameRE is a one-segment module name, for --module.
var moduleNameRE = regexp.MustCompile(`^[A-Z][A-Za-z0-9]*(_[A-Za-z0-9]+)*$`)

// typeParamsOf lists the distinct type parameters of t the way gen counts a declaration's Go
// type parameters: not those inside an `Html` (its message type is erased).
func typeParamsOf(t types.Type) []string {
	var out []string
	seen := map[string]bool{}
	var walk func(types.Type)
	walk = func(t types.Type) {
		switch t := t.(type) {
		case *types.Param:
			if !seen[t.Name] {
				seen[t.Name] = true
				out = append(out, t.Name)
			}
		case *types.Brand:
			walk(t.Base)
		case *types.App:
			if t.Con == "Html" {
				return
			}
			for _, a := range t.Args {
				walk(a)
			}
		case *types.Record:
			if t.Name != "" && !t.Anonymous {
				for _, a := range t.Args {
					walk(a)
				}
				return
			}
			for _, f := range t.Fields {
				walk(f.Type)
			}
		case *types.Union:
			for _, a := range t.Args {
				walk(a)
			}
		case *types.Fun:
			for _, p := range t.Params {
				walk(p)
			}
			walk(t.Result)
		}
	}
	walk(t)
	return out
}

// evalPrint reports a value: `value : Type` on stdout, or into the JSON result.
func evalPrint(stdout io.Writer, jsonOut bool, res *evalResult, value, typ string) int {
	if jsonOut {
		res.Value, res.Type = value, typ
		return 0
	}
	fmt.Fprintf(stdout, "%s : %s\n", value, typ)
	return 0
}

// evalDiags reports ds (to stderr, or into res for --json), the scratch module mod's positioned
// in the input as file "input", the app's with their own source lines, and says whether one is
// an error.
func evalDiags(dir string, ds []diag.Diagnostic, mapLine func(diag.Diagnostic) (diag.Diagnostic, bool), es evalSource, mod string, jsonOut bool, res *evalResult, stderr io.Writer) bool {
	failed := false
	appLines := map[string][]string{}
	for _, d := range ds {
		if d.Severity == "error" {
			failed = true
		}
		var lines []string
		if d.Span.File == mod+".alder" {
			d = evalMapDiag(d, mapLine, es)
			lines = es.lines
		} else {
			l, ok := appLines[d.Span.File]
			if !ok {
				if b, err := os.ReadFile(filepath.Join(dir, d.Span.File)); err == nil {
					l = strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n")
				}
				appLines[d.Span.File] = l
			}
			lines = l
		}
		if jsonOut {
			res.Diagnostics = append(res.Diagnostics, json.RawMessage(diag.RenderJSON(d)))
		} else {
			fmt.Fprint(stderr, diag.RenderText(d, lines))
		}
	}
	return failed
}

// evalMapDiag positions a diagnostic of the scratch module in the input, as file "input": its
// lines through mapLine (scratch's MapDiag) and evalSource's map, its columns undoing the input
// items' rewriting (parseItems), and the line a duplicate's message names (E0009) likewise. One
// on a line Compile inserted (not mapped) is put at 1:1 with its message unchanged; one past the end
// of the source at the end of the input. The parser's "in the definition of `it`: " is
// dropped: `it` is a name the input never wrote.
func evalMapDiag(d diag.Diagnostic, mapLine func(diag.Diagnostic) (diag.Diagnostic, bool), es evalSource) diag.Diagnostic {
	d, ok := mapLine(d)
	d.Span.File = "input"
	d.Message = strings.TrimPrefix(d.Message, "in the definition of `it`: ")
	if strings.HasPrefix(d.Message, "the type of `it` cannot be inferred") {
		d.Message = "the expression uses `it`, which eval does not define"
	}
	if !ok {
		d.Span.Start, d.Span.End, d.Fix = diag.Pos{Line: 1, Col: 1}, diag.Pos{Line: 1, Col: 1}, nil
		return d
	}
	pos := func(p diag.Pos) diag.Pos {
		if p.Line < 1 {
			return diag.Pos{Line: 1, Col: 1}
		}
		if p.Line > len(es.inLine) { // past the end of the input
			n := len(es.lines)
			return diag.Pos{Line: n, Col: len(es.lines[n-1]) + 1}
		}
		return diag.Pos{Line: es.inLine[p.Line-1], Col: max(1, p.Col+es.shift[p.Line-1])}
	}
	d.Span.Start, d.Span.End = pos(d.Span.Start), pos(d.Span.End)
	d.Message = scratch.MapLineRef(d, func(l int) (int, bool) {
		if l < 1 || l > len(es.inLine) {
			return 0, false
		}
		return es.inLine[l-1], true
	})
	if d.Fix != nil {
		f := *d.Fix
		start, end := pos(diag.Pos{Line: f.Line, Col: f.Col}), pos(diag.Pos{Line: f.EndLine, Col: f.EndCol})
		f.Line, f.Col, f.EndLine, f.EndCol = start.Line, start.Col, end.Line, end.Col
		d.Fix = &f
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
