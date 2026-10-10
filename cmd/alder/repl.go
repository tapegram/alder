package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"alder/internal/buildkit"
	"alder/internal/diag"
	"alder/internal/scratch"
	"alder/internal/snippets"
	"alder/internal/types"
)

// replUsage is the usage line of `alder repl`.
const replUsage = "usage: alder repl [dir] [--module Name]"

// replHelp is what :help prints.
const replHelp = `  let name = expr   keep a value in the session (a later let of the name replaces it)
  expr              evaluate; a Query, Task or Select shows its type
  :type expr        the type of expr, without running it
  :run expr         run a Query against build/<app>/dev.db
  :run! expr        run a Task against build/<app>/dev.db; its writes commit
  :html expr        render an Html value as markup
  :reload           re-check the session against the app; lets that no longer check are dropped
  :reset            forget every let
  :help, :h         this list
  :quit, :q         leave (Ctrl-D too)
  A line ending in \ continues on the next. The session re-checks when the app's files change.
`

// replEntry is one thing typed at the REPL (its lines joined by `\` continuations) as an
// item of the session module: a let the session keeps, or the expression being evaluated.
type replEntry struct {
	item
	at    int      // the REPL line its first line was entered at
	lines []string // its lines as typed (for diagnostics' source display)
}

// repl is a REPL session over the app at dir.
type repl struct {
	dir, modName, appBuild string
	lets                   []replEntry
	vet                    bool // vet the next build (the first, and the first after a reload)
	stdout, stderr         io.Writer
	interrupt              <-chan struct{}
}

// runReplTo is `alder repl [dir] [--module Name]`: a read-eval-print loop over a persistent
// scratch module (named Scratch, or Name) that sees every module of the app at dir (default
// "."). Each line is a `let name = expr` the session keeps, an expression to evaluate (printed
// as `value : Type`), or a command (replHelp). Every line re-checks the whole session; an
// expression is compiled into build/<app>/scratch/repl/ and run. The session re-checks itself
// when the app's files change (as :reload). interrupt receives one value per Ctrl-C: it stops a
// running evaluation, and at the prompt says how to leave. Exit 0 on :quit or end of input, 2
// for usage or an app directory without app/.
func runReplTo(args []string, stdin io.Reader, stdout, stderr io.Writer, interrupt <-chan struct{}) (code int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(stderr, "alder repl: internal error: %v\n", r)
			code = 2
		}
	}()
	var pos []string
	modName := "Scratch"
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--module" && i+1 < len(args):
			i++
			modName = args[i]
		case strings.HasPrefix(a, "--module="):
			modName = strings.TrimPrefix(a, "--module=")
		case strings.HasPrefix(a, "-"):
			fmt.Fprintln(stderr, replUsage)
			return 2
		default:
			pos = append(pos, a)
		}
	}
	if len(pos) > 1 {
		fmt.Fprintln(stderr, replUsage)
		return 2
	}
	if !moduleNameRE.MatchString(modName) {
		fmt.Fprintf(stderr, "alder repl: --module wants a module name such as Probe, got %q\n", modName)
		return 2
	}
	dir := "."
	if len(pos) == 1 {
		dir = pos[0]
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		fmt.Fprintf(stderr, "alder repl: %v\n", err)
		return 2
	}
	if !dirHasApp(abs) {
		fmt.Fprintf(stderr, "alder repl: no .alder files under %s/app\n", dir)
		return 2
	}
	r := &repl{dir: dir, modName: modName, appBuild: filepath.Join("build", filepath.Base(abs)),
		vet: true, stdout: stdout, stderr: stderr, interrupt: interrupt}
	checked, _ := r.check(nil)
	if exists := (*scratch.ModuleExistsError)(nil); errors.As(checked.Err, &exists) {
		fmt.Fprintf(stderr, "alder repl: %s; pass --module <Name> to name the REPL's module otherwise\n", exists.Taken())
		return 2
	}
	if checked.Err != nil {
		fmt.Fprintf(stderr, "alder repl: %v\n", checked.Err)
		return 2
	}
	r.report(checked, nil, nil, -1)
	fmt.Fprintf(stdout, "alder repl: %s (:help for the commands, :quit to leave)\n", filepath.Base(abs))

	stop := make(chan struct{})
	defer close(stop)
	changed := make(chan change)
	go watch(dir, 300*time.Millisecond, stop, changed)
	lines := replLines(stdin, stop)

	lineNo, at := 0, 0
	var pending []string // the lines of an entry continued with `\`
	for {
		select { // a change that arrived during the last evaluation
		case k := <-changed:
			r.autoReload(k)
		default:
		}
		if pending == nil {
			fmt.Fprint(stdout, "alder> ")
		} else {
			fmt.Fprint(stdout, "    …> ")
		}
		select {
		case l, ok := <-lines:
			if !ok {
				fmt.Fprintln(stdout)
				return 0
			}
			lineNo++
			if pending == nil {
				at = lineNo
			}
			l = strings.TrimSuffix(l, "\r")
			if body, cont := strings.CutSuffix(l, `\`); cont {
				pending = append(pending, body)
				continue
			}
			entry := append(pending, l)
			pending = nil
			if r.line(entry, at) {
				return 0
			}
		case k := <-changed:
			fmt.Fprintln(stdout)
			r.autoReload(k)
		case <-interrupt:
			fmt.Fprintln(stdout)
			pending = nil
			fmt.Fprintln(stdout, "(use :quit or Ctrl-D to leave)")
		}
	}
}

// replLines feeds stdin's lines (up to 1 MB each) to the channel it returns, which is closed
// at end of input; it stops when stop is closed.
func replLines(stdin io.Reader, stop <-chan struct{}) <-chan string {
	lines := make(chan string)
	go func() {
		defer close(lines)
		sc := bufio.NewScanner(stdin)
		sc.Buffer(make([]byte, 64*1024), 1<<20)
		for sc.Scan() {
			select {
			case lines <- sc.Text():
			case <-stop:
				return
			}
		}
	}()
	return lines
}

// line handles one entry (its lines, the first entered at REPL line at) and says whether the
// REPL is to quit.
func (r *repl) line(lines []string, at int) (quit bool) {
	text := strings.TrimSpace(strings.Join(lines, "\n"))
	switch {
	case allBlank(lines): // blank, or a comment
		return false
	case strings.HasPrefix(text, ":"):
		lines[0] = strings.TrimLeft(lines[0], " \t")
		cmd := lines[0]
		if i := strings.IndexAny(cmd, " \t"); i >= 0 {
			cmd = cmd[:i]
		}
		lines[0] = strings.TrimLeft(lines[0][len(cmd):], " \t")
		return r.command(cmd, lines, at)
	case text == "let" || strings.HasPrefix(text, "let "):
		r.let(lines, at)
	default:
		r.eval("", lines, at)
	}
	return false
}

// command runs :cmd with the argument lines and says whether the REPL is to quit.
func (r *repl) command(cmd string, lines []string, at int) (quit bool) {
	arg := strings.TrimSpace(strings.Join(lines, "\n"))
	switch cmd {
	case ":quit", ":q":
		return true
	case ":help", ":h":
		fmt.Fprint(r.stdout, replHelp)
	case ":reset":
		r.lets = nil
		fmt.Fprintln(r.stdout, "session reset")
	case ":reload":
		fmt.Fprintln(r.stdout, "session re-checked")
		r.reload()
	case ":type", ":run", ":run!", ":html":
		if arg == "" {
			fmt.Fprintf(r.stderr, "alder repl: `%s` wants an expression: %s <expr>\n", cmd, cmd)
			return false
		}
		r.eval(cmd, lines, at)
	default:
		fmt.Fprintf(r.stderr, "unknown command %s (try :help)\n", cmd)
	}
	return false
}

// entry makes the item of an entry's lines. The entry is one item whatever its layout: a
// continuation line that parseItems would take for the start of another item (a line at
// column 1 that is not a closer) is indented by two spaces, which its shift undoes. It returns
// errNothing for a let.
func entry(lines []string, at int) (replEntry, error) {
	src := append([]string(nil), lines...)
	indented := map[int]bool{}
	for i, st := range snippets.Starts(src) {
		if st && i > 0 && !allBlank(src[:i]) {
			src[i] = "  " + src[i]
			indented[i+1] = true
		}
	}
	items, err := parseItems(strings.Join(src, "\n"))
	if err != nil && !errors.Is(err, errNothing) {
		return replEntry{}, err
	}
	if len(items) != 1 {
		return replEntry{}, errNothing
	}
	it := items[0]
	for j, in := range it.InLine {
		if indented[in] {
			it.Shift[j] -= 2
		}
	}
	return replEntry{item: it, at: at, lines: lines}, err
}

// allBlank says whether every line is blank or a `--` comment.
func allBlank(lines []string) bool {
	for _, l := range lines {
		if t := strings.TrimSpace(l); t != "" && !strings.HasPrefix(t, "--") {
			return false
		}
	}
	return true
}

// let handles `let name = expr`: checked with the session's other lets, kept when clean.
func (r *repl) let(lines []string, at int) {
	e, err := entry(lines, at)
	if !errors.Is(err, errNothing) {
		if err == nil {
			err = errors.New("a let is `let name = expr`")
		}
		fmt.Fprintf(r.stderr, "alder repl: %v\n", err)
		return
	}
	if !e.Let || e.Name == "" {
		fmt.Fprintln(r.stderr, "alder repl: a let is `let name = expr`")
		return
	}
	var others []replEntry
	redefined := false
	for _, l := range r.lets {
		if l.Name == e.Name {
			redefined = true
			continue
		}
		others = append(others, l)
	}
	session := append(others, e)
	checked, starts := r.check(session)
	if checked.Err != nil {
		fmt.Fprintf(r.stderr, "alder repl: %v\n", checked.Err)
		return
	}
	if r.report(checked, session, starts, len(session)-1) || checked.Result == nil {
		return
	}
	if checked.Result.Holes > 0 {
		fmt.Fprintf(r.stderr, "alder repl: the let has %d typed hole(s); fill them to keep it\n", checked.Result.Holes)
		return
	}
	sig, ok := checked.Result.Values[r.modName+"."+e.Name]
	if !ok {
		fmt.Fprintf(r.stderr, "alder repl: internal error: no type was inferred for %s\n", e.Name)
		return
	}
	r.lets = session
	if redefined {
		fmt.Fprintf(r.stdout, "redefined %s\n", e.Name)
	}
	fmt.Fprintf(r.stdout, "%s : %s\n", e.Name, types.String(checked.Result.U.Apply(sig)))
}

// eval evaluates an expression for cmd: "" (a bare expression), ":type", ":run", ":run!" or
// ":html".
func (r *repl) eval(cmd string, lines []string, at int) {
	e, err := entry(lines, at)
	if err != nil {
		if errors.Is(err, errNothing) {
			err = fmt.Errorf("`%s` wants an expression, not a let", cmd)
			if cmd == "" {
				err = errors.New("expected an expression")
			}
		}
		fmt.Fprintf(r.stderr, "alder repl: %v\n", err)
		return
	}
	session := append(append([]replEntry(nil), r.lets...), e)
	checked, starts := r.check(session)
	if checked.Err != nil {
		fmt.Fprintf(r.stderr, "alder repl: %v\n", checked.Err)
		return
	}
	if r.report(checked, session, starts, len(session)-1) || checked.Result == nil {
		return
	}
	if checked.Result.Holes > 0 {
		fmt.Fprintf(r.stderr, "alder repl: the expression has %d typed hole(s); fill them to evaluate it\n", checked.Result.Holes)
		return
	}
	sig, ok := checked.Result.Values[r.modName+".it"]
	if !ok {
		fmt.Fprintln(r.stderr, "alder repl: internal error: no type was inferred for the expression")
		return
	}
	sig = checked.Result.U.Apply(sig)
	typ := types.String(sig)
	text := strings.TrimSpace(strings.Join(lines, "\n"))
	if cmd == ":type" {
		fmt.Fprintf(r.stdout, "%s : %s\n", text, typ)
		return
	}
	con := ""
	app, _ := sig.(*types.App)
	if app != nil {
		con = app.Con
	}
	kind := mainPure
	switch cmd {
	case "":
		switch con {
		case "Query":
			fmt.Fprintf(r.stdout, "<Query> : %s\nhint: `:run %s` runs it against the dev database\n", typ, text)
			return
		case "Task":
			fmt.Fprintf(r.stdout, "<Task> : %s\nhint: `:run! %s` runs it; its writes commit to the dev database\n", typ, text)
			return
		case "Select":
			fmt.Fprintf(r.stdout, "<Select> : %s\nhint: pipe it to Db.all, Db.first or Db.count to make a Query\n", typ)
			return
		}
	case ":run":
		switch con {
		case "Query":
			kind, typ = mainQuery, types.String(app.Args[0])
		case "Task":
			fmt.Fprintln(r.stderr, "alder repl: `:run` wants a Query; use `:run!` for a Task")
			return
		case "Select":
			fmt.Fprintln(r.stderr, "alder repl: a Select is not a Query: pipe it to Db.all, Db.first or Db.count to run it")
			return
		default:
			fmt.Fprintf(r.stderr, "alder repl: `:run` wants a Query; %s : %s (evaluate it without :run)\n", text, typ)
			return
		}
	case ":run!":
		switch con {
		case "Task":
			kind, typ = mainTask, types.String(app.Args[0])
		case "Query":
			fmt.Fprintln(r.stderr, "alder repl: `:run!` wants a Task; use `:run` for a Query")
			return
		default:
			fmt.Fprintf(r.stderr, "alder repl: `:run!` wants a Task; %s : %s\n", text, typ)
			return
		}
	case ":html":
		if con != "Html" {
			fmt.Fprintf(r.stderr, "alder repl: `:html` wants an Html value; %s : %s\n", text, typ)
			return
		}
	}
	out, ok := r.run(checked, kind, sig)
	if !ok {
		return
	}
	if cmd == ":html" {
		fmt.Fprintln(r.stdout, strings.TrimSuffix(out, "\n"))
		return
	}
	fmt.Fprintf(r.stdout, "%s : %s\n", out, typ)
}

// run compiles what checked holds with the main of kind for its `it` (of type sig) into
// build/<app>/scratch/repl/ and runs it; it reports a failure itself (ok false). The lock on
// the build directory is held from the build to the end of the run. A value on interrupt stops
// the build or the run.
func (r *repl) run(checked scratch.Checked, kind mainKind, sig types.Type) (out string, ok bool) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-r.interrupt:
			cancel()
		case <-done:
		}
	}()
	failf := func(format string, a ...any) (string, bool) {
		fmt.Fprintf(r.stderr, "alder repl: "+format+"\n", a...)
		return "", false
	}

	var env []string
	if kind != mainPure {
		dbPath, err := filepath.Abs(filepath.Join(r.appBuild, "dev.db"))
		if err != nil {
			return failf("%v", err)
		}
		if err := os.MkdirAll(r.appBuild, 0o755); err != nil {
			return failf("%v", err)
		}
		env = append(env, "ALDER_EVAL_DB="+dbPath)
	}
	mainGo := scratchMain(kind, kind != mainPure && buildkit.HasGoFiles(filepath.Join(r.dir, "adapters")), evalIt(r.modName, sig))
	b := scratch.CompileCheckedWith(checked, mainGo, "repl", filepath.Join(r.appBuild, "scratch", "repl"), scratch.Options{NoVet: !r.vet})
	defer b.Close()
	if ctx.Err() != nil {
		return failf("interrupted")
	}
	if b.Err != nil {
		return failf("%v", b.Err)
	}
	if !b.Runnable() {
		return failf("internal error: nothing was built")
	}
	r.vet = false
	tctx, tcancel := context.WithTimeoutCause(ctx, scratch.Timeout, fmt.Errorf("timed out after %v", scratch.Timeout))
	defer tcancel()
	out, errOut, exit := scratch.RunContext(tctx, b, env, "")
	switch {
	case ctx.Err() != nil:
		return failf("interrupted")
	case exit == -1 && !strings.Contains(errOut, "timed out"):
		return failf("%s", strings.TrimSpace(errOut))
	case exit != 0 && (strings.Contains(errOut, "goroutine stack exceeds") || strings.Contains(errOut, "fatal error: stack overflow")):
		return failf("stack overflow (unbounded recursion?)")
	case exit != 0:
		if strings.HasPrefix(errOut, "panic: ") {
			errOut, _, _ = strings.Cut(errOut, "\n\n")
		}
		if kind == mainTask {
			return failf("the task failed: %s", strings.TrimSpace(errOut))
		}
		return failf("%s", strings.TrimSpace(errOut))
	}
	return out, true
}

// check checks the session module made of entries, and returns where each entry starts in it
// (0-based line offsets of the module's source).
func (r *repl) check(entries []replEntry) (scratch.Checked, []int) {
	items := make([]item, len(entries))
	starts := make([]int, len(entries))
	n := 0
	for i, e := range entries {
		items[i], starts[i] = e.item, n
		n += len(e.Lines)
	}
	es := assemble(items)
	return scratch.Check(r.dir, &scratch.Module{Name: r.modName, Source: es.src, Infer: true}), starts
}

// owner is the entry (index) a line (1-based) of the session module belongs to: by the
// entries' lengths (InLine is each entry's own), the last for a line past the end.
func owner(entries []replEntry, starts []int, line int) int {
	for i := len(starts) - 1; i >= 0; i-- {
		if line > starts[i] {
			return i
		}
	}
	return 0
}

// inputPos is the position evalDiags gives a diagnostic of the session module.
var inputPos = regexp.MustCompile(`^input:(\d+):(\d+): `)

// report prints checked's diagnostics and says whether one is an error. The session module's
// are positioned in the entry they fall in, over its text: `expr:L:C` for the expression (L
// counted in its own lines), `let x (line N)` for a let (N the REPL line). Errors are printed
// wherever they are; warnings only for the entry cur (the one just typed), so a let's or the
// app's warnings are not repeated on every line.
func (r *repl) report(checked scratch.Checked, entries []replEntry, starts []int, cur int) bool {
	failed := false
	for _, d := range checked.Diags {
		if d.Severity == "error" {
			failed = true
		}
		if d.Span.File != r.modName+".alder" {
			if d.Severity == "error" {
				evalDiags(r.dir, []diag.Diagnostic{d}, checked.MapDiag, evalSource{}, r.modName, false, nil, r.stderr)
			}
			continue
		}
		k := cur
		if m, ok := checked.MapDiag(d); ok && len(entries) > 0 {
			k = owner(entries, starts, m.Span.Start.Line)
		}
		if k < 0 {
			continue
		}
		if d.Severity != "error" && k != cur {
			continue
		}
		r.printDiag(checked, d, entries[k], starts[k])
	}
	return failed
}

// printDiag prints d, a diagnostic of the session module in entry e (which starts at line
// offset start of the module), through evalDiags over e's own text, labelled as report says.
func (r *repl) printDiag(checked scratch.Checked, d diag.Diagnostic, e replEntry, start int) {
	within := func(l int) (int, bool) { return l - start, l-start >= 1 }
	mapLine := func(d diag.Diagnostic) (diag.Diagnostic, bool) {
		d, ok := checked.MapDiag(d)
		if !ok {
			return d, false
		}
		d.Message = scratch.MapLineRef(d, within)
		d.Span.Start.Line -= start
		d.Span.End.Line -= start
		if d.Fix != nil {
			f := *d.Fix
			f.Line, f.EndLine = f.Line-start, f.EndLine-start
			d.Fix = &f
		}
		return d, true
	}
	es := assemble([]item{e.item})
	es.lines = e.lines
	var buf bytes.Buffer
	evalDiags(r.dir, []diag.Diagnostic{d}, mapLine, es, r.modName, false, nil, &buf)
	text := buf.String()
	if m := inputPos.FindStringSubmatchIndex(text); m != nil {
		label := "expr:" + text[m[2]:m[5]] + ": "
		if e.Let {
			var l int
			fmt.Sscan(text[m[2]:m[3]], &l)
			label = fmt.Sprintf("let %s (line %d): ", e.Name, e.at+l-1)
		}
		text = label + text[m[1]:]
	}
	io.WriteString(r.stderr, text)
}

// reload re-checks the session against the app as it is now: each let in order, against the
// lets kept so far; a let that no longer checks is dropped with a notice. While the app itself
// has errors they are printed and the session is kept as it is. The next build is vetted.
func (r *repl) reload() {
	r.vet = true
	base, _ := r.check(nil)
	if base.Err != nil {
		fmt.Fprintf(r.stderr, "alder repl: %v\n", base.Err)
		return
	}
	if r.report(base, nil, nil, -1) {
		fmt.Fprintln(r.stderr, "alder repl: the app has errors; the session is kept until it checks")
		return
	}
	var kept []replEntry
	for _, l := range r.lets {
		session := append(append([]replEntry(nil), kept...), l)
		checked, _ := r.check(session)
		if checked.Err != nil {
			fmt.Fprintf(r.stderr, "alder repl: %v\n", checked.Err)
			return
		}
		var first *diag.Diagnostic
		for i, d := range checked.Diags {
			if d.Severity == "error" {
				first = &checked.Diags[i]
				break
			}
		}
		if first == nil {
			kept = append(kept, l)
			continue
		}
		fmt.Fprintf(r.stdout, "dropped let %s: %s[%s]: %s\n", l.Name, first.Severity, first.Code, first.Message)
	}
	r.lets = kept
}

// autoReload is reload after the watcher saw the app's files change (k); a change to static/
// alone changes nothing the session checks.
func (r *repl) autoReload(k change) {
	if k&sourceChange == 0 {
		return
	}
	fmt.Fprintln(r.stdout, "app changed: session re-checked")
	r.reload()
}
