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
	"strconv"
	"strings"
	"time"

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
	work                   string // the build directory, build/<app>/scratch/repl-<pid>
	lets                   []replEntry
	redefining             string // the let a redefinition is being checked for (printDiag)
	vet                    bool   // vet the next build (the first, and the first after a reload)
	stdout, stderr         io.Writer
	interrupt              <-chan struct{}
	terminate              <-chan struct{}  // closed on SIGTERM or SIGHUP
	seen                   map[string]stamp // the app's files as the session was last checked against
	appDiags               string           // the app's errors as last printed ("" while it checks)
	warned                 string           // the last build's warning of unwired capabilities, as printed
}

// runReplTo is `alder repl [dir] [--module Name]`: a read-eval-print loop over a persistent
// scratch module (named Scratch, or Name) that sees every module of the app at dir (default
// "."). Each line is a `let name = expr` the session keeps, an expression to evaluate (printed
// as `value : Type`), or a command (replHelp). Every line re-checks the whole session; an
// expression is compiled, with the lets it uses, into build/<app>/scratch/repl-<pid>/ (the
// REPL's own: two REPLs never wait for each other; removed when the REPL ends, and one a killed
// REPL left is removed by the next) and run. The session re-checks itself
// when the app's files change (as :reload). interrupt receives one value per Ctrl-C: it stops a
// running evaluation, and at the prompt says how to leave. Exit 0 on :quit or end of input, 2
// for usage or an app directory without app/.
func runReplTo(args []string, stdin io.Reader, stdout, stderr io.Writer, interrupt <-chan struct{}) int {
	return replMain(args, stdin, stdout, stderr, interrupt, nil)
}

// replMain is runReplTo with terminate, which main closes on SIGTERM or SIGHUP (the terminal
// closed): the REPL stops a running evaluation (killing its process group) and exits 1.
func replMain(args []string, stdin io.Reader, stdout, stderr io.Writer, interrupt, terminate <-chan struct{}) (code int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(stderr, "alder repl: internal error: %v\n", r)
			code = 2
		}
	}()
	var pos []string
	modName, named := "Scratch", false
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--module" && i+1 < len(args):
			i++
			modName, named = args[i], true
		case strings.HasPrefix(a, "--module="):
			modName, named = strings.TrimPrefix(a, "--module="), true
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
		vet: true, stdout: stdout, stderr: stderr, interrupt: interrupt, terminate: terminate}
	r.seen = snapshot(dir)
	checked, _ := r.check(nil)
	if exists := (*scratch.ModuleExistsError)(nil); errors.As(checked.Err, &exists) {
		if named {
			fmt.Fprintf(stderr, "alder repl: %s; give --module another name\n", exists.Taken())
		} else {
			fmt.Fprintf(stderr, "alder repl: %s; pass --module <Name> to name the REPL's module otherwise\n", exists.Taken())
		}
		return 2
	}
	if checked.Err != nil {
		fmt.Fprintf(stderr, "alder repl: %v\n", checked.Err)
		return 2
	}
	if r.report(checked, nil, nil, -1) {
		fmt.Fprintln(stderr, "alder repl: the app has errors; fix them (the REPL re-checks when you save) — :type and lets will work once it checks")
	}
	scratchDir := filepath.Join(r.appBuild, "scratch")
	removeStale(scratchDir)
	r.work = filepath.Join(scratchDir, fmt.Sprintf("repl-%d", os.Getpid()))
	defer func() { // on :quit, end of input and SIGTERM/SIGHUP alike; a SIGKILL leaves it to removeStale
		os.RemoveAll(r.work)
		os.Remove(r.work + ".lock")
	}()
	fmt.Fprintf(stdout, "alder repl: %s (:help for the commands, :quit to leave)\n", filepath.Base(abs))

	stop := make(chan struct{})
	defer close(stop)
	changed := make(chan change)
	go watch(dir, 300*time.Millisecond, stop, changed)
	lines := replLines(stdin, stop)

	lineNo, at := 0, 0
	var pending []string // the lines of an entry continued with `\`
	for {
		select {
		case <-terminate: // during the last evaluation, which it stopped
			fmt.Fprintln(stderr, "alder repl: terminated")
			return 1
		default:
		}
		select { // a change that arrived during the last evaluation
		case <-changed:
			r.autoReload(false)
		default:
		}
		if pending == nil {
			fmt.Fprint(stdout, "alder> ")
		} else {
			fmt.Fprint(stdout, "    …> ")
		}
		select {
		case in, ok := <-lines:
			if !ok {
				fmt.Fprintln(stdout)
				return 0
			}
			if in.err != nil {
				fmt.Fprintf(stderr, "\nalder repl: reading input: %v\n", in.err)
				return 1
			}
			lineNo++
			if in.tooLong {
				pending = nil
				fmt.Fprintf(stderr, "alder repl: line too long (limit %d MB); ignored\n", maxLine>>20)
				continue
			}
			l := in.text
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
		case <-changed:
			r.autoReload(true)
		case <-terminate:
			fmt.Fprintln(stderr, "\nalder repl: terminated")
			return 1
		case <-interrupt:
			fmt.Fprintln(stdout)
			pending = nil
			fmt.Fprintln(stdout, "(use :quit or Ctrl-D to leave)")
		}
	}
}

// replDirRE is a REPL's build directory (or its lock file) under build/<app>/scratch.
var replDirRE = regexp.MustCompile(`^repl-([0-9]+)(\.lock)?$`)

// removeStale removes the build directories (and lock files) under scratchDir of REPLs that are
// gone: a REPL removes its own when it ends, but not when it is killed outright. One whose pid
// is alive (another REPL, or a process that took the pid since) is kept.
func removeStale(scratchDir string) {
	entries, _ := os.ReadDir(scratchDir)
	for _, e := range entries {
		m := replDirRE.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		pid, err := strconv.Atoi(m[1])
		if err != nil || pid == os.Getpid() || pidAlive(pid) {
			continue
		}
		os.RemoveAll(filepath.Join(scratchDir, e.Name()))
	}
}

// maxLine is the longest line the REPL reads (bytes, without the newline).
const maxLine = 1 << 20

// inputLine is a line of the REPL's input: its text, or tooLong (longer than maxLine; its text
// is dropped), or err, the read error that ends the input (never io.EOF).
type inputLine struct {
	text    string
	tooLong bool
	err     error
}

// replLines feeds stdin's lines to the channel it returns, which is closed at end of input
// (after an inputLine with err, when reading failed); it stops when stop is closed.
func replLines(stdin io.Reader, stop <-chan struct{}) <-chan inputLine {
	lines := make(chan inputLine)
	go func() {
		defer close(lines)
		br := bufio.NewReaderSize(stdin, 64*1024)
		for {
			l, err := readLine(br)
			if err == io.EOF && l.text == "" && !l.tooLong {
				return
			}
			if err != nil && err != io.EOF {
				l = inputLine{err: err}
			}
			select {
			case lines <- l:
			case <-stop:
				return
			}
			if err != nil {
				return
			}
		}
	}()
	return lines
}

// readLine reads a line from br (a last one without a newline too): its text without the
// newline, or tooLong past maxLine (the rest of the line is read and dropped). The error is
// br's (io.EOF at the end).
func readLine(br *bufio.Reader) (inputLine, error) {
	var l inputLine
	var buf []byte
	for {
		chunk, err := br.ReadSlice('\n')
		if !l.tooLong {
			if len(buf)+len(bytes.TrimSuffix(chunk, []byte("\n"))) > maxLine {
				l.tooLong, buf = true, nil
			} else {
				buf = append(buf, chunk...)
			}
		}
		if err == bufio.ErrBufferFull {
			continue
		}
		if !l.tooLong {
			l.text = strings.TrimSuffix(string(buf), "\n")
		}
		return l, err
	}
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
		// the command is `:` and the letters (and `!`) after it: `:run!(x)` is `:run! (x)`
		cmd := lines[0]
		if i := strings.IndexFunc(cmd[1:], func(c rune) bool { return !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '!') }); i >= 0 {
			cmd = cmd[:i+1]
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
		r.reload("session re-checked")
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
	items, err := parseItems(strings.Join(src, "\n"), "the REPL")
	if err != nil && !errors.Is(err, errNothing) {
		// the entry is one item, so its first line: the REPL's messages say no "input line 1"
		return replEntry{}, errors.New(inputLineRE.ReplaceAllString(err.Error(), ""))
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

// inputLineRE is parseItems' "input line N: " before a message.
var inputLineRE = regexp.MustCompile(`^input line \d+: `)

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
	// a redefinition replaces the let in place: the lets after it that use the name see the
	// new definition (lets are definitions, not snapshots)
	session := append([]replEntry(nil), r.lets...)
	cur := len(session)
	for i, l := range session {
		if l.Name == e.Name {
			cur = i
		}
	}
	redefined := cur < len(session)
	if redefined {
		session[cur] = e
	} else {
		session = append(session, e)
	}
	checked, starts := r.check(session)
	if checked.Err != nil {
		fmt.Fprintf(r.stderr, "alder repl: %v\n", checked.Err)
		return
	}
	if redefined {
		r.redefining = e.Name // a let that uses it and no longer checks refuses the redefinition
	}
	failed := r.report(checked, session, starts, cur)
	r.redefining = ""
	if failed || checked.Result == nil {
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
	var flat []string // text on one line, for a hint
	for _, l := range lines {
		if l = strings.TrimSpace(l); l != "" {
			flat = append(flat, l)
		}
	}
	oneLine := strings.Join(flat, " ")
	if cmd == ":type" {
		fmt.Fprintf(r.stdout, "%s : %s\n", oneLine, typ)
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
			fmt.Fprintf(r.stdout, "<Query> : %s\nhint: `:run %s` runs it against the dev database\n", typ, oneLine)
			return
		case "Task":
			fmt.Fprintf(r.stdout, "<Task> : %s\nhint: `:run! %s` runs it; its writes commit to the dev database\n", typ, oneLine)
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
	// only the lets the expression uses go into the program: each let is a package variable,
	// computed whenever the program starts, so another let that fails (or is slow) is no
	// concern of this evaluation
	used := reachable(r.lets, e)
	if len(used) < len(r.lets) {
		if sub, _ := r.check(append(append([]replEntry(nil), used...), e)); sub.Err == nil && sub.Result != nil && !hasError(sub.Diags) {
			checked = sub
		} else {
			used = r.lets // not expected: the whole session checks
		}
	}
	var names []string
	for _, l := range used {
		names = append(names, l.Name)
	}
	out, ok := r.run(checked, kind, sig, names)
	if !ok {
		return
	}
	if cmd == ":html" {
		fmt.Fprintln(r.stdout, strings.TrimSuffix(out, "\n"))
		return
	}
	fmt.Fprintf(r.stdout, "%s : %s\n", out, typ)
}

// reachable is the lets (in order) the entry e uses, directly or through other lets: those whose
// name appears in its text as an identifier, then in theirs, to a fixpoint. It may take a let
// that is not used (its name in a string, a record field, a qualified name), never miss one.
func reachable(lets []replEntry, e replEntry) []replEntry {
	byName := map[string]int{}
	for i, l := range lets {
		byName[l.Name] = i
	}
	in := make([]bool, len(lets))
	work := []replEntry{e}
	for len(work) > 0 {
		x := work[len(work)-1]
		work = work[:len(work)-1]
		for _, id := range identRE.FindAllString(strings.Join(x.Lines, "\n"), -1) {
			if i, ok := byName[id]; ok && !in[i] {
				in[i] = true
				work = append(work, lets[i])
			}
		}
	}
	var out []replEntry
	for i, l := range lets {
		if in[i] {
			out = append(out, l)
		}
	}
	return out
}

// identRE is an identifier (or the identifier-like part of a longer token).
var identRE = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*`)

// run compiles what checked holds with the main of kind for its `it` (of type sig) into the
// REPL's build directory and runs it; it reports a failure itself (ok false), naming lets, the
// lets the program computes. A value on interrupt (or terminate) stops the build or the run.
// Like eval's, the build is a test build (scratch.Options.Tests): its main uses app.TestSession().
func (r *repl) run(checked scratch.Checked, kind mainKind, sig types.Type, lets []string) (out string, ok bool) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-r.interrupt:
			cancel()
		case <-r.terminate:
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
	mainGo := scratchMain(kind, kind != mainPure && checked.WiresAdapters(), evalIt(r.modName, sig))
	b := scratch.CompileCheckedWith(ctx, checked, mainGo, "repl", r.work, scratch.Options{NoVet: !r.vet, Tests: true})
	defer b.Close()
	if ctx.Err() != nil {
		return failf("interrupted")
	}
	if b.Warning != r.warned { // said once, and again when it changes
		io.WriteString(r.stderr, b.Warning)
		r.warned = b.Warning
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
	out, errOut, exit := scratch.RunTethered(tctx, b, env)
	if ctx.Err() != nil {
		return failf("interrupted")
	}
	msg, outcome := classifyRun(errOut, exit)
	switch outcome {
	case runOK:
		return out, true
	case runInternal:
		return failf("%s", msg)
	}
	msg = devDBHint(msg, r.appBuild)
	if outcome == runFailed && kind == mainTask {
		msg = "the task failed: " + msg
	}
	if len(lets) > 0 {
		msg += fmt.Sprintf(" (the lets it uses — %s — are computed with it)", strings.Join(lets, ", "))
	}
	return failf("%s", msg)
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
	var app []diag.Diagnostic
	for _, d := range checked.Diags {
		if d.Span.File != r.modName+".alder" && d.Severity == "error" {
			app = append(app, d)
		}
	}
	r.reportApp(checked, app)
	for _, d := range checked.Diags {
		if d.Severity == "error" {
			failed = true
		}
		if d.Span.File != r.modName+".alder" {
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

// reportApp prints the app's errors: the block when it differs from the one last printed,
// else only a line saying the app still has them.
func (r *repl) reportApp(checked scratch.Checked, app []diag.Diagnostic) {
	if len(app) == 0 {
		r.appDiags = ""
		return
	}
	var buf bytes.Buffer
	evalDiags(r.dir, app, checked.MapDiag, evalSource{}, r.modName, false, nil, &buf)
	if buf.String() == r.appDiags {
		fmt.Fprintln(r.stderr, "alder repl: the app has errors (shown above); fix them and save")
		return
	}
	r.appDiags = buf.String()
	io.WriteString(r.stderr, r.appDiags)
}

// letLabel is the name a let's diagnostics give it: `let x (line N)`, or `let (line N)` when
// what follows `let` is not a name.
func letLabel(e replEntry, line int) string {
	if letNameRE.MatchString(e.Name) {
		return fmt.Sprintf("let %s (line %d)", e.Name, line)
	}
	return fmt.Sprintf("let (line %d)", line)
}

// letNameRE is a value name.
var letNameRE = regexp.MustCompile(`^[a-z_][A-Za-z0-9_]*$`)

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
			label = letLabel(e, e.at+l-1) + ": "
			if r.redefining != "" && e.Name != r.redefining {
				label = fmt.Sprintf("alder repl: %s not redefined: %s uses it: ", r.redefining, letLabel(e, e.at+l-1))
			}
		}
		text = label + text[m[1]:]
	}
	io.WriteString(r.stderr, text)
}

// reload re-checks the session against the app as it is now, after header (printed once the
// app checks): each let in order, against the lets kept so far; a let that no longer checks is
// dropped with a notice. While the app itself has errors they are printed and the session is
// kept as it is. The next build is vetted.
func (r *repl) reload(header string) {
	r.vet = true
	r.seen = snapshot(r.dir)
	base, _ := r.check(nil)
	if base.Err != nil {
		fmt.Fprintf(r.stderr, "alder repl: %v\n", base.Err)
		return
	}
	if r.report(base, nil, nil, -1) {
		fmt.Fprintln(r.stderr, "alder repl: the session is kept until the app checks")
		return
	}
	fmt.Fprintln(r.stdout, header)
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

// autoReload is reload after the watcher saw the app's files change, unless nothing the session
// checks differs from what it was last checked against (a :reload since, or static/ alone).
// atPrompt: the prompt is showing, so the notice starts a new line.
func (r *repl) autoReload(atPrompt bool) {
	if diff(r.seen, snapshot(r.dir))&sourceChange == 0 {
		return
	}
	if atPrompt {
		fmt.Fprintln(r.stdout)
	}
	r.reload("app changed: session re-checked")
}
