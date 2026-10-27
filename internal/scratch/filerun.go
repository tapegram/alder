package scratch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"alder/internal/diag"
	"alder/internal/types"
)

// MaxValue caps a watch's shown value (bytes); a longer one is cut there, with Truncated.
const MaxValue = 64 << 10

// effectNote is a watch's Note when its value is an effect, which scratch does not run.
const effectNote = "not run: scratch evaluates pure values only"

// AppErrorsNote is FileResult.Note when the app has errors, so nothing was built.
const AppErrorsNote = "the app has errors; fix them and save"

// WatchResult is what RunFile learned of one watch.
type WatchResult struct {
	Line      int    `json:"line"`
	End       int    `json:"end"` // the watch's last line (Line for a one-line watch)
	Expr      string `json:"expr"`
	Type      string `json:"type,omitempty"` // "" when the file did not check far enough
	Value     string `json:"value,omitempty"`
	Error     string `json:"error,omitempty"`
	Kind      string `json:"kind,omitempty"` // "pure", "query", "task" or "select"; "" when the type is unknown
	Note      string `json:"note,omitempty"` // effect watches: "not run: scratch evaluates pure values only"
	Truncated bool   `json:"truncated,omitempty"`
}

// FileResult is the outcome of RunFile.
type FileResult struct {
	Watches []WatchResult
	Diags   []diag.Diagnostic // the file's (positioned in File.Name) then the app's, as checked
	Ran     bool              // the program was built and run
	Note    string            // why nothing (or not everything) ran
	Warning string            // Build.Warning (unwired capabilities)
	Err     error             // I/O, go build or internal failure; never a diagnostic
}

// FileOptions adjust RunFile.
type FileOptions struct {
	Module  string        // the scratch module's name without an `in` header; "" is "Scratch"
	Out     string        // the build directory; "" is build/<app>/scratch/file-<hash> (FilePurpose)
	Timeout time.Duration // 0 is Timeout
	Build   Options       // Waiting/Fetching; RunFile sets Tests and Thunks
	// OnTypes is called once after the check, before building, with every watch's Type it
	// inferred (an effect watch's placeholder Value and Note too, which are final) and the
	// diagnostics; also when the file has errors. Not called when the check produced no result
	// (an unknown `in` module, a taken module name, a source that does not parse, an I/O
	// failure). May be nil.
	OnTypes func(FileResult)
	// Parsed are ParseFile's diagnostics for the file: they come first in Diags, and an error
	// among them stops the build as a check error does.
	Parsed []diag.Diagnostic
}

// FilePurpose is the build purpose of the scratch file at path: "file-" and the first 12 hex
// digits of the SHA-256 of its absolute path, so each file builds into a directory of its own.
func FilePurpose(path string) string {
	abs, err := filepath.Abs(path) // a relative name is taken against the current directory
	if err != nil {
		abs = path
	}
	sum := sha256.Sum256([]byte(abs))
	return "file-" + hex.EncodeToString(sum[:])[:12]
}

// itPrefixRE is the parser's prefix of a diagnostic in a watch's generated definition.
var itPrefixRE = regexp.MustCompile("^in the definition of `it[0-9]+`: ")

// RunFile checks the scratch file f against the app at appDir, and builds and runs its pure
// watches in one program, each a thunk (Options.Thunks) the main calls under a recover: a watch
// that panics fails alone; one that crashes the program (a stack overflow, the timeout) keeps
// the values before it and leaves those after it not run. Effect watches (Query, Task, Select)
// are never run. Nothing is built when the file or the app has errors or typed holes, or there
// is no pure watch; Note says why, except for the file's own errors, which Diags show.
func RunFile(ctx context.Context, appDir string, f File, o FileOptions) (r FileResult) {
	defer func() {
		if p := recover(); p != nil {
			r.Err = fmt.Errorf("internal error: %v", p)
		}
	}()
	r.Watches = make([]WatchResult, len(f.Watches))
	for i, w := range f.Watches {
		r.Watches[i] = WatchResult{Line: w.Line, End: w.End, Expr: w.Expr}
	}
	modName := o.Module
	if modName == "" {
		modName = "Scratch"
	}
	session := modName
	if f.In != "" {
		session = f.In
	}
	fileDiags := slices.Clone(o.Parsed)
	fileErr := hasError(o.Parsed)

	// 1. Check.
	checked := Check(appDir, &Module{Name: modName, In: f.In, Source: f.Source, Infer: true})
	if nm := (*NoModuleError)(nil); errors.As(checked.Err, &nm) {
		r.Diags = append(fileDiags, f.errAt(headerPos(f), "no module "+nm.Name+" in the app"))
		return r
	}
	if exists := (*ModuleExistsError)(nil); errors.As(checked.Err, &exists) {
		r.Diags = append(fileDiags, f.errAt(diag.Pos{Line: 1, Col: 1}, exists.Taken()+"; pass --module <Name>"))
		return r
	}
	if checked.Err != nil {
		r.Diags, r.Err = fileDiags, checked.Err
		return r
	}
	var appDiags []diag.Diagnostic
	appErr := false
	fileHoles := 0 // the typed holes in the file (W0002); the rest of Result.Holes are the app's
	for _, d := range checked.Diags {
		if d.Code == "W0002" && checked.InSession(d) {
			fileHoles++
		}
		if !checked.InSession(d) {
			appDiags = append(appDiags, d)
			appErr = appErr || d.Severity == "error"
			continue
		}
		if inc, ok := f.incomplete(d, checked.MapDiag); ok {
			d = inc
		} else {
			d = f.Map.Position(d, checked.MapDiag, f.Name)
			d.Message = itPrefixRE.ReplaceAllString(d.Message, "")
		}
		fileDiags = append(fileDiags, d)
		fileErr = fileErr || d.Severity == "error"
	}
	r.Diags = append(fileDiags, appDiags...)
	if checked.Result == nil {
		if len(checked.Diags) == 0 {
			r.Err = errors.New("internal error: the file was not checked")
		}
		return r
	}

	// 2. Types.
	res := checked.Result
	sigs := make([]types.Type, len(f.Watches))
	pure := 0
	for i, w := range f.Watches {
		sig, ok := res.Values[session+"."+w.Name]
		if !ok {
			continue
		}
		sigs[i] = res.U.Apply(sig)
		r.Watches[i].Type = checked.TypeString(sigs[i], session)
		r.Watches[i].Kind = "pure"
		if eff := effectOf(sigs[i]); eff != "" {
			r.Watches[i].Kind = strings.ToLower(eff)
			r.Watches[i].Value, r.Watches[i].Note = "<"+eff+">", effectNote
		} else {
			pure++
		}
	}
	if o.OnTypes != nil {
		cp := r
		cp.Watches, cp.Diags = slices.Clone(r.Watches), slices.Clone(r.Diags)
		o.OnTypes(cp)
	}

	// 3. Whether to build.
	switch {
	case appErr:
		r.Note = AppErrorsNote
		return r
	case fileErr:
		return r
	case fileHoles > 0:
		r.Note = fmt.Sprintf("the file has %d typed hole(s); fill them to run it", fileHoles)
		return r
	case res.Holes > 0:
		r.Note = fmt.Sprintf("the app has %d typed hole(s); fill them to run the file", res.Holes)
		return r
	case len(f.Watches) == 0:
		r.Note = "the file has no watches; add a line `> expr`"
		return r
	case pure == 0:
		r.Note = "nothing to run: the file's watches are all effects"
		return r
	}

	// 4. The main.
	thunks := map[string]bool{}
	var calls strings.Builder
	for i, w := range f.Watches {
		if sigs[i] == nil || effectOf(sigs[i]) != "" {
			continue
		}
		thunks[session+"."+w.Name] = true
		fmt.Fprintf(&calls, "\tshow(%d, func() string { return rt.Show(%s) })\n", i+1, GoValue(session, w.Name, sigs[i], true))
	}
	bo := o.Build
	bo.Tests, bo.Thunks = true, thunks
	bo.NoFormat = true
	out := o.Out
	purpose := ""
	if out == "" {
		purpose = FilePurpose(f.Name)
	}
	b := CompileCheckedWith(ctx, checked, strings.NewReplacer("{{calls}}", calls.String(), "{{max}}", strconv.Itoa(MaxValue+1)).Replace(fileMain), purpose, out, bo)
	defer b.Close()
	r.Warning = b.Warning
	if b.Err != nil {
		r.Err = b.Err
		return r
	}
	if !b.Runnable() {
		r.Err = errors.New("internal error: nothing was built")
		return r
	}

	// 5. Run, and read the frames.
	timeout := o.Timeout
	if timeout == 0 {
		timeout = Timeout
	}
	runCtx, cancel := context.WithTimeoutCause(ctx, timeout, fmt.Errorf("timed out after %v", timeout))
	defer cancel()
	stdout, errOut, exit := RunTethered(runCtx, b, nil)
	if ctx.Err() != nil { // the caller's own cancellation, not the timeout
		r.Err = ctx.Err()
		return r
	}
	msg, outcome := ClassifyRun(errOut, exit)
	if outcome == RunInternal { // it could not be started
		r.Err = errors.New(msg)
		return r
	}
	r.Ran = true
	crashed := -1 // index of the watch the program died in
	started := false
	for i := range f.Watches {
		if sigs[i] == nil || effectOf(sigs[i]) != "" {
			continue
		}
		w := &r.Watches[i]
		frame, ok := frameOf(stdout, i+1)
		switch {
		case crashed >= 0:
			w.Error = fmt.Sprintf("not run: the program crashed at line %d", r.Watches[crashed].Line)
		case !ok && outcome == RunOK:
			w.Error = "internal error: the program gave no result"
		case !ok: // never started: the program died before it
			crashed = i
			if !started { // every pure watch is unrun: say so on each, so the run fails
				why := "the program failed before any watch ran: " + strings.TrimPrefix(msg, "panic: ")
				for j := range f.Watches {
					if sigs[j] != nil && effectOf(sigs[j]) == "" {
						r.Watches[j].Error = "not run: " + why
					}
				}
				r.Note = why
				return r
			}
			w.Error = msg
		case frame == "":
			if outcome == RunOK {
				w.Error = "internal error: the program gave no result"
				break
			}
			crashed, started = i, true
			w.Error = msg
		case frame[0] == '\x02':
			started = true
			w.Value = frame[1:]
			w.Value, w.Truncated = truncate(w.Value)
		default: // '\x15'
			started = true
			w.Error = frame[1:]
		}
	}
	switch {
	case crashed >= 0:
		r.Note = fmt.Sprintf("the program crashed at line %d: %s", r.Watches[crashed].Line, msg)
	case outcome != RunOK:
		r.Note = msg
	}
	return r
}

// frameOf is the frame watch k wrote to the program's output: its status byte (\x02 value,
// \x15 error) and text, "" when it started and wrote nothing more; ok false when it never
// started. A frame runs to the start marker of a later watch (watches are numbered in file
// order; effect watches write none), which a value is unlikely to hold.
func frameOf(out string, k int) (frame string, ok bool) {
	marker := "\x1e" + strconv.Itoa(k) + "\n"
	i := strings.Index(out, marker)
	if i < 0 {
		return "", false
	}
	out = out[i+len(marker):]
	for _, m := range markerRE.FindAllStringSubmatchIndex(out, -1) {
		if n, _ := strconv.Atoi(out[m[2]:m[3]]); n > k {
			return out[:m[0]], true
		}
	}
	return out, true
}

var markerRE = regexp.MustCompile("\x1e([0-9]+)\n")

// effectOf is "Query", "Task" or "Select" for a watch of that type, else "".
func effectOf(t types.Type) string {
	if a, ok := t.(*types.App); ok && (a.Con == "Query" || a.Con == "Task" || a.Con == "Select") {
		return a.Con
	}
	return ""
}

// hasError reports whether ds hold an error.
func hasError(ds []diag.Diagnostic) bool {
	return slices.ContainsFunc(ds, func(d diag.Diagnostic) bool { return d.Severity == "error" })
}

// errAt is an error diagnostic of the file at p.
func (f File) errAt(p diag.Pos, msg string) diag.Diagnostic {
	return diag.Diagnostic{Severity: "error", Message: msg, Span: diag.Span{File: f.Name, Start: p, End: p}}
}

// incomplete re-anchors a diagnostic of the scratch module that falls inside a watch's
// generated `itK = ` prefix: the item before it is incomplete and the parser ran on into the
// watch, as with `> 1 +` followed by `> 2`. The diagnostic would name syntax the user never
// wrote, so it is put at the end of the item's last line (the last line before the watch that
// is neither blank nor a comment) with a message of its own; ok is false for any other one.
func (f File) incomplete(d diag.Diagnostic, mapLine func(diag.Diagnostic) (diag.Diagnostic, bool)) (diag.Diagnostic, bool) {
	m, ok := mapLine(d)
	if !ok || d.Severity != "error" {
		return d, false
	}
	line, col := m.Span.Start.Line, m.Span.Start.Col
	for _, w := range f.Watches {
		if w.Line != line || col > len(w.Name+" = ") {
			continue
		}
		for i := line - 2; i >= 0; i-- {
			t := strings.TrimSpace(f.Lines[i])
			if t == "" || strings.HasPrefix(t, "--") {
				continue
			}
			end := diag.Pos{Line: i + 1, Col: len(strings.TrimRight(f.Lines[i], " \t")) + 1}
			d.Span = diag.Span{File: f.Name, Start: end, End: end}
			d.Fix = nil
			d.Message = fmt.Sprintf("this is incomplete: the watch on line %d starts a new item", w.Line)
			return d, true
		}
	}
	return d, false
}

// headerPos is the position of the module name in f's `in` header (1:1 without one).
func headerPos(f File) diag.Pos {
	for i, l := range f.Lines {
		if rest, ok := strings.CutPrefix(l, "in"); ok && strings.TrimSpace(rest) == f.In && rest != strings.TrimLeft(rest, " \t") {
			return diag.Pos{Line: i + 1, Col: 3 + len(rest) - len(strings.TrimLeft(rest, " \t"))}
		}
	}
	return diag.Pos{Line: 1, Col: 1}
}

// truncate cuts s to at most MaxValue bytes, at a rune boundary, and says whether it did.
func truncate(s string) (string, bool) {
	if len(s) <= MaxValue {
		return s, false
	}
	i := MaxValue
	for i > 0 && !utf8.RuneStart(s[i]) { // s[i] is the first byte left out
		i--
	}
	return s[:i], true
}

// fileMain is the generated cmd/main.go of a scratch file's build: {{calls}} are a show call per
// pure watch, in file order. show writes the start marker \x1e<k>\n before evaluating watch k,
// then \x02 and the value, or \x15 and the message of a panic it recovers. A payload's own \x1e
// becomes U+FFFD, so it cannot end the frame, and it is cut to {{max}} (MaxValue+1) bytes, so a
// huge value never crosses the pipe and RunFile still sees it is too long. Writes go straight
// to the unbuffered os.Stdout, so a crash that ends the program (a stack overflow) leaves the
// frames before it whole and its own started.
const fileMain = `package main

import (
	"fmt"
	"os"
	"strings"

	"alder/rt"
	"app"
)

func write(status, s string) {
	s = strings.ReplaceAll(s, "\x1e", "\uFFFD")
	if len(s) > {{max}} {
		s = s[:{{max}}]
	}
	os.Stdout.WriteString(status + s)
}

func show(k int, v func() string) {
	os.Stdout.WriteString(fmt.Sprintf("\x1e%d\n", k))
	defer func() {
		if r := recover(); r != nil {
			write("\x15", fmt.Sprint(r))
		}
	}()
	write("\x02", v())
}

func main() {
{{calls}}}
`
