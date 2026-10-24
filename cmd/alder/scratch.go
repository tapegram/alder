package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"alder/internal/buildkit"
	"alder/internal/diag"
	"alder/internal/scratch"
)

// scratchUsage is the usage line of `alder scratch`.
const scratchUsage = "usage: alder scratch [file] [--watch] [--json] [--build-dir DIR] [--module Name]"

// scratchResult is the --json output of `alder scratch`.
type scratchResult struct {
	Watches     []scratch.WatchResult `json:"watches"`
	Diagnostics []json.RawMessage     `json:"diagnostics"`
	Ran         bool                  `json:"ran"`
	Note        string                `json:"note,omitempty"`
	Error       string                `json:"error,omitempty"`
}

// holeCode is the checker's warning for a typed hole (`?name`).
const holeCode = "W0002"

// scratchTruncatedLine ends a value cut at scratch.MaxValue.
const scratchTruncatedLine = "  … (truncated at 64 KB)"

// runScratchTo is `alder scratch [file] [--watch] [--json] [--build-dir DIR] [--module Name]`:
// it runs the scratch file (default ./scratch.alder) once against the app above it. The app is
// the nearest ancestor directory of the file that holds app/*.alder; the file must be a scratch
// file for that app (scratch.IsScratchFile). Diagnostics go to stderr, positioned in the file
// as named on the command line, then each watch on stdout as `file:line: expr` and its result
// (`  ⇒ value : Type`, `  ✗ error`, or the type alone when nothing ran); the engine's note and
// warning follow on stderr. With --json one object {watches, diagnostics, ran, note, error} goes
// to stdout instead. Exit 0 when nothing failed, 1 for error diagnostics, a typed hole, a failing
// watch, adapters or a network that refuse the build, or an interrupt (Ctrl-C, SIGTERM), 2 for
// usage, I/O and internal errors. --watch is accepted and, until it is implemented, ignored:
// the file runs once.
func runScratchTo(args []string, stdout, stderr io.Writer) (code int) {
	jsonOut := false
	res := scratchResult{Watches: []scratch.WatchResult{}, Diagnostics: []json.RawMessage{}}
	failf := func(c int, format string, a ...any) int {
		msg := fmt.Sprintf(format, a...)
		if jsonOut {
			res.Error = msg
		} else {
			fmt.Fprintln(stderr, msg)
		}
		return c
	}
	defer func() {
		if r := recover(); r != nil {
			code = failf(2, "alder scratch: internal error: %v", r)
		}
		if jsonOut {
			enc := json.NewEncoder(stdout)
			enc.SetEscapeHTML(false)
			enc.Encode(res)
		}
	}()

	var pos []string
	named := false
	modName, bd := "", ""
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
		case a == "--watch":
			// accepted; one run until watching exists
		case a == "--module" && i+1 < len(args):
			i++
			modName, named = args[i], true
		case strings.HasPrefix(a, "--module="):
			modName, named = strings.TrimPrefix(a, "--module="), true
		case strings.HasPrefix(a, "--"):
			return failf(2, "%s", scratchUsage)
		default:
			pos = append(pos, a)
		}
	}
	if len(pos) > 1 {
		return failf(2, "%s", scratchUsage)
	}
	if named && !moduleNameRE.MatchString(modName) {
		return failf(2, "alder scratch: --module wants a module name such as Probe, got %q", modName)
	}
	file := "scratch.alder"
	if len(pos) == 1 {
		file = pos[0]
	}
	abs, err := filepath.Abs(file)
	if err != nil {
		return failf(2, "alder scratch: %v", err)
	}
	appDir := ""
	for d := filepath.Dir(abs); ; d = filepath.Dir(d) {
		if dirHasApp(d) {
			appDir = d
			break
		}
		if filepath.Dir(d) == d {
			break
		}
	}
	if appDir == "" {
		return failf(2, "alder scratch: no app found above %s", file)
	}
	rel, err := filepath.Rel(appDir, abs)
	if err != nil || !scratch.IsScratchFile(filepath.ToSlash(rel)) {
		return failf(2, "alder scratch: %s is not a scratch file (a name ending in scratch.alder, outside app/)", file)
	}
	text, err := os.ReadFile(abs)
	if err != nil {
		return failf(2, "alder scratch: %v", err)
	}

	sigCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return scratchRun(sigCtx, scratchRunArgs{
		file: file, abs: abs, appDir: appDir, text: string(text), module: modName, named: named, buildDir: bd,
		jsonOut: jsonOut, stdout: stdout, stderr: stderr,
	}, &res, failf)
}

// scratchRunArgs are one run's inputs.
type scratchRunArgs struct {
	file, abs, appDir, text string // the path as given, absolute, the app's directory, the file's text
	module                  string // --module's value
	named                   bool
	buildDir                string
	jsonOut                 bool
	stdout, stderr          io.Writer
}

// scratchRun parses and runs the file once, reports it as a.jsonOut says, and returns the exit
// code. failf records and prints a fatal message.
func scratchRun(ctx context.Context, a scratchRunArgs, res *scratchResult, failf func(int, string, ...any) int) int {
	f, parsed := scratch.ParseFile(a.file, a.text)
	if err := errInFlags(f.In, a.named); err != nil {
		return failf(2, "alder scratch: %v\n%s", err, scratchUsage)
	}
	waiting := func() { fmt.Fprintln(a.stderr, "alder scratch: waiting for another run of this scratch file…") }
	out := filepath.Join(appBuildDir(a.buildDir, a.appDir), "scratch", scratch.FilePurpose(a.abs))
	r := scratch.RunFile(ctx, a.appDir, f, scratch.FileOptions{
		Module: a.module, Out: out, Parsed: parsed,
		Build: scratch.Options{Waiting: waiting, Fetching: func() { fmt.Fprintln(a.stderr, buildkit.FetchingLine) }},
	})
	if ctx.Err() != nil {
		return failf(1, "alder scratch: interrupted")
	}

	failed := false
	holes := false
	appLines := map[string][]string{}
	for _, d := range r.Diags {
		if d.Severity == "error" {
			failed = true
		}
		if d.Code == holeCode {
			holes = true
		}
		if a.jsonOut {
			res.Diagnostics = append(res.Diagnostics, json.RawMessage(diag.RenderJSON(d)))
			continue
		}
		lines := f.Lines
		if d.Span.File != f.Name {
			lines = appSourceLines(a.appDir, appLines, d.Span.File)
		}
		fmt.Fprint(a.stderr, diag.RenderText(d, lines))
	}
	for _, w := range r.Watches {
		if w.Error != "" {
			failed = true
		}
	}
	if holes {
		failed = true
	}

	if a.jsonOut {
		res.Watches, res.Ran, res.Note = r.Watches, r.Ran, r.Note
		if res.Watches == nil {
			res.Watches = []scratch.WatchResult{}
		}
		io.WriteString(a.stderr, r.Warning)
	} else {
		for _, w := range r.Watches {
			printScratchWatch(a.stdout, a.file, w)
		}
		io.WriteString(a.stderr, r.Warning)
		if r.Note != "" {
			fmt.Fprintln(a.stderr, "alder scratch: "+r.Note)
		}
	}
	if r.Err != nil {
		return failf(buildErrCode(r.Err), "alder scratch: %v", r.Err)
	}
	if failed {
		return 1
	}
	return 0
}

// printScratchWatch writes one watch: `file:line: first line of expr`, then its result.
func printScratchWatch(w io.Writer, file string, r scratch.WatchResult) {
	expr, _, _ := strings.Cut(r.Expr, "\n")
	fmt.Fprintf(w, "%s:%d: %s\n", file, r.Line, expr)
	indent := func(s string) string { return strings.ReplaceAll(s, "\n", "\n    ") }
	switch {
	case r.Error != "":
		fmt.Fprintf(w, "  ✗ %s\n", indent(r.Error))
	case r.Note != "":
		fmt.Fprintf(w, "  %s : %s   (%s)\n", r.Value, r.Type, r.Note)
	case r.Value != "":
		fmt.Fprintf(w, "  ⇒ %s : %s\n", indent(r.Value), r.Type)
		if r.Truncated {
			fmt.Fprintln(w, scratchTruncatedLine)
		}
	case r.Type != "":
		fmt.Fprintf(w, "  : %s\n", r.Type)
	}
}
