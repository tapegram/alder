package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"alder/internal/diag"
	"alder/internal/scratch"
	"alder/internal/types"
)

// evalUsage is the usage line of `alder eval`.
const evalUsage = "usage: alder eval [dir] [--run] [--json] <expr | ->"

// evalPureMain prints the value of Scratch's `it` as Alder (rt.Show); {{it}} is evalIt's Go
// expression for it.
const evalPureMain = `package main

import (
	"fmt"

	"alder/rt"
	"app"
)

func main() { fmt.Print(rt.Show({{it}})) }
`

// evalQueryMain runs Scratch's `it` ({{it}}, as in evalPureMain), a Query, against the SQLite
// database ALDER_EVAL_DB (created with the app's tables when absent; an existing one is checked
// against the app's schema and never rebuilt) with no capabilities wired, and prints its result
// (rt.Show).
const evalQueryMain = `package main

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"time"

	"alder/rt"
	"alder/rt/db"
	"alder/rt/eff"
	"app"
)

// unwired is the message of a capability call that finds no implementation.
var unwired = regexp.MustCompile(` + "`" + `alder: capability "([^"]*)" is not wired` + "`" + `)

func fail(msg string) {
	fmt.Fprintln(os.Stderr, unwired.ReplaceAllString(msg, "capability $1 is not wired in eval"))
	os.Exit(1)
}

func main() {
	defer func() {
		if r := recover(); r != nil {
			fail(fmt.Sprint(r))
		}
	}()
	path := os.Getenv("ALDER_EVAL_DB")
	_, statErr := os.Stat(path)
	store, err := db.OpenSQLite(path)
	if err != nil {
		fail(err.Error())
	}
	defer store.Close()
	ctx := context.Background()
	if err := store.Ensure(ctx, app.Tables, os.IsNotExist(statErr)); err != nil {
		fail(err.Error())
	}
	env := &eff.Env{Ctx: ctx, Store: store, Caps: map[string]any{}, Now: time.Now}
	v, err := eff.RunQuery(env, {{it}})
	if err != nil {
		fail(err.Error())
	}
	fmt.Print(rt.Show(v))
}
`

// evalSource is the scratch module `alder eval` compiles for an input, and how to map its
// positions back to the input's.
type evalSource struct {
	src   string
	lines []string // the input's lines
	// shift[i] is what to add to a column on line i+1 of src to get the input's column: the
	// expression's first line gains `it = ` (-5), a let line loses `let ` (+4 and its indent).
	shift []int
}

// errNothing is an input with no expression.
var errNothing = errors.New("nothing to evaluate")

// evalModule turns the input (zero or more `let name = expr` items, then one expression; an item
// starts on a line that is not indented, blank or a comment, and continues on the indented lines
// after it) into the source of module Scratch: each let item `name = expr`, the expression
// `it = expr`, all without signatures (the checker infers them), one source line per input line.
func evalModule(input string) (evalSource, error) {
	input = strings.TrimRight(strings.ReplaceAll(input, "\r\n", "\n"), "\n")
	lines := strings.Split(input, "\n")
	es := evalSource{lines: lines, shift: make([]int, len(lines))}
	var starts []int // the first line of each item
	for i, l := range lines {
		t := strings.TrimSpace(l)
		if t == "" || strings.HasPrefix(t, "--") || l[0] == ' ' || l[0] == '\t' {
			continue
		}
		starts = append(starts, i)
	}
	if len(starts) == 0 {
		return es, errNothing
	}
	out := append([]string(nil), lines...)
	for k, i := range starts {
		l := lines[i]
		if k == len(starts)-1 {
			out[i] = "it = " + l
			es.shift[i] = -len("it = ")
			continue
		}
		rest, ok := strings.CutPrefix(l, "let ")
		if !ok {
			return es, fmt.Errorf("input line %d: only `let name = expr` lines may come before the expression", i+1)
		}
		trimmed := strings.TrimLeft(rest, " \t")
		if name, _, _ := strings.Cut(trimmed, " "); name == "it" || strings.HasPrefix(trimmed, "it=") {
			return es, fmt.Errorf("input line %d: `it` is the name eval gives the expression; let another name", i+1)
		}
		out[i] = trimmed
		es.shift[i] = len(l) - len(trimmed)
	}
	es.src = strings.Join(out, "\n") + "\n"
	return es, nil
}

// evalResult is the --json output of `alder eval`.
type evalResult struct {
	Value       string            `json:"value"`
	Type        string            `json:"type"`
	Diagnostics []json.RawMessage `json:"diagnostics"`
	Error       string            `json:"error,omitempty"`
}

// runEvalTo is `alder eval [dir] [--run] [--json] <expr | ->`: it evaluates an expression (or,
// with `-`, standard input: `let name = expr` lines, then an expression) in a scratch module
// that sees every module of the app at dir (default "."), compiled by the real compiler, and
// prints `value : Type`. A `Query` is shown as `<Query>` unless --run runs it against
// build/<app>/dev.db; a `Task` is refused. Diagnostics go to stderr, positioned in the input
// (`input:L:C`). Exit 0 on success, 1 for diagnostics or a failing evaluation, 2 for usage, I/O
// and internal errors. --json prints one object {value, type, diagnostics, error} on stdout
// instead. Evaluations of one app serialize on build/<app>/scratch/eval.lock.
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
	for _, a := range args {
		switch {
		case a == "--json":
			jsonOut = true
		case a == "--run":
			run = true
		case strings.HasPrefix(a, "--"):
			return failf(2, "%s", evalUsage)
		default:
			pos = append(pos, a)
		}
	}
	dir, expr := ".", ""
	switch len(pos) {
	case 1:
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

	mod := &scratch.Module{Name: "Scratch", Source: es.src, Infer: true}
	checked := scratch.Check(dir, mod)
	if checked.Err != nil {
		return failf(2, "alder eval: %v", checked.Err)
	}
	errs := evalDiags(dir, checked.Diags, checked.MapDiag, es, jsonOut, &res, stderr)
	if errs {
		return 1
	}
	if checked.Result == nil {
		return failf(2, "alder eval: internal error: the program was not checked")
	}
	if checked.Result.Holes > 0 {
		return failf(1, "alder eval: the input has %d typed hole(s); fill them to evaluate it", checked.Result.Holes)
	}
	sig, ok := checked.Result.Values["Scratch.it"]
	if !ok {
		return failf(2, "alder eval: internal error: no type was inferred for the expression")
	}
	sig = checked.Result.U.Apply(sig)
	typ := types.String(sig)

	mainGo := evalPureMain
	if app, ok := sig.(*types.App); ok {
		switch app.Con {
		case "Task":
			return failf(1, "alder eval: a Task is not run by eval (the REPL's :run! will); its type is %s", typ)
		case "Query":
			if !run {
				return evalPrint(stdout, jsonOut, &res, "<Query>", typ)
			}
			mainGo, typ = evalQueryMain, types.String(app.Args[0])
		}
	}
	var env []string
	isQuery := mainGo == evalQueryMain
	mainGo = strings.ReplaceAll(mainGo, "{{it}}", evalIt(sig))
	if isQuery {
		dbPath, err := filepath.Abs(filepath.Join(appBuild, "dev.db"))
		if err != nil {
			return failf(2, "alder eval: %v", err)
		}
		if err := os.MkdirAll(appBuild, 0o755); err != nil {
			return failf(2, "alder eval: %v", err)
		}
		env = append(env, "ALDER_EVAL_DB="+dbPath)
	}
	unlock, err := lockEval(filepath.Join(appBuild, "scratch", "eval.lock"))
	if err != nil {
		return failf(2, "alder eval: %v", err)
	}
	defer unlock()

	b := scratch.Compile(dir, mod, mainGo, "eval", filepath.Join(appBuild, "scratch", "eval"))
	if b.Err != nil {
		return failf(2, "alder eval: %v", b.Err)
	}
	if !b.Runnable() { // the check above passed, so this is not expected
		if evalDiags(dir, b.Diags, b.MapDiag, es, jsonOut, &res, stderr) {
			return 1
		}
		return failf(2, "alder eval: internal error: nothing was built")
	}
	out, errOut, exit := scratch.Run(b, env, "")
	switch {
	case exit == -1 && !strings.Contains(errOut, "timed out"): // could not start: internal
		return failf(2, "alder eval: %s", strings.TrimSpace(errOut))
	case exit != 0: // the evaluation failed (or ran past scratch.Timeout)
		if strings.HasPrefix(errOut, "panic: ") { // the Go goroutine dump says nothing about the input
			errOut, _, _ = strings.Cut(errOut, "\n\n")
		}
		return failf(1, "alder eval: %s", strings.TrimSpace(errOut))
	}
	return evalPrint(stdout, jsonOut, &res, out, typ)
}

// evalIt is the Go expression for Scratch's `it` of type sig in the generated package: a
// polymorphic value is a generic thunk (`func Scratch__it[A any]() …`) and a polymorphic function
// a generic function, both instantiated here at `any` (an empty `[]` shows the same at any
// element type); gen gives `it` no other thunk, since nothing can refer to it.
func evalIt(sig types.Type) string {
	n := len(typeParamsOf(sig))
	if n == 0 {
		return "app.Scratch__it"
	}
	s := "app.Scratch__it[" + strings.TrimSuffix(strings.Repeat("any, ", n), ", ") + "]"
	if _, isFun := sig.(*types.Fun); !isFun {
		s += "()"
	}
	return s
}

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

// evalDiags reports ds (to stderr, or into res for --json), the scratch module's positioned in
// the input as file "input", the app's with their own source lines, and says whether one is an
// error.
func evalDiags(dir string, ds []diag.Diagnostic, mapLine func(diag.Diagnostic) (diag.Diagnostic, bool), es evalSource, jsonOut bool, res *evalResult, stderr io.Writer) bool {
	failed := false
	appLines := map[string][]string{}
	for _, d := range ds {
		if d.Severity == "error" {
			failed = true
		}
		var lines []string
		if d.Span.File == "Scratch.alder" {
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
// lines through mapLine (scratch's MapDiag), its columns undoing evalModule's rewriting. One on a
// line Compile inserted (not mapped) is put at 1:1 with its message unchanged.
func evalMapDiag(d diag.Diagnostic, mapLine func(diag.Diagnostic) (diag.Diagnostic, bool), es evalSource) diag.Diagnostic {
	d, ok := mapLine(d)
	d.Span.File = "input"
	if !ok {
		d.Span.Start, d.Span.End, d.Fix = diag.Pos{Line: 1, Col: 1}, diag.Pos{Line: 1, Col: 1}, nil
		return d
	}
	if n := len(es.lines); d.Span.Start.Line > n { // at the end of the input
		end := diag.Pos{Line: n, Col: len(es.lines[n-1]) + 1}
		d.Span.Start, d.Span.End, d.Fix = end, end, nil
		return d
	}
	col := func(line, c int) int {
		if line < 1 || line > len(es.shift) {
			return c
		}
		return max(1, c+es.shift[line-1])
	}
	d.Span.Start.Col = col(d.Span.Start.Line, d.Span.Start.Col)
	d.Span.End.Col = col(d.Span.End.Line, d.Span.End.Col)
	if d.Fix != nil {
		f := *d.Fix
		f.Col, f.EndCol = col(f.Line, f.Col), col(f.EndLine, f.EndCol)
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

// lockEval takes an exclusive flock on path (created, with its directory, when absent) and
// returns its release. The kernel drops the lock when the process dies, so a crashed eval never
// leaves it held.
func lockEval(path string) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return nil, err
	}
	for {
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
		if err != syscall.EINTR {
			break
		}
	}
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("locking %s: %v", path, err)
	}
	return func() {
		syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}, nil
}
