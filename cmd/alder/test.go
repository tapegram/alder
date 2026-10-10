package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"alder/internal/buildkit"
	"alder/internal/diag"
	"alder/internal/scratch"
)

// testUsage is the usage line of `alder test`.
const testUsage = "usage: alder test [dir] [-run re] [--json]"

// testTimeout bounds one `alder test` run (every test together).
const testTimeout = 5 * time.Minute

// testMain is the Go main `alder test` compiles with the app: it hands the app's test table,
// tables, capabilities and session to rtesting.Main, which reads -run and --json from the
// environment (ALDER_TEST_RUN, ALDER_TEST_JSON). With withAdapters the capabilities are the
// app's adapters (adapters.Caps()); otherwise none is wired, and a test that calls one fails.
func testMain(withAdapters bool) string {
	imp, caps := "", "app.Caps{}.Registry()"
	if withAdapters {
		imp, caps = "\t\"app/adapters\"\n", "adapters.Caps().Registry()"
	}
	return `package main

import (
	"os"

	rtesting "alder/rt/testing"
	"app"
` + imp + `)

func main() {
	caps := ` + caps + `
	os.Exit(rtesting.Main(app.Tests, app.Tables, caps, app.TestSession(), os.Args[1:], os.Stdout, os.Stderr))
}
`
}

// runTestTo is `alder test [dir] [-run re] [--json]`: it compiles the app at dir (default ".")
// with a main that runs its `test` declarations (testMain) into build/<app>/scratch/test, runs
// it, and passes the program's report through: one line per test and a summary, or with
// --json one JSON object per test on stdout (the summary on stderr). -run selects the tests
// whose `Module "name"` matches the regexp. Check diagnostics are printed as `alder check`
// prints them; with --json they are one object {"diagnostics": […]} on stdout and nothing else.
// Exit 0 when every selected test passed, 1 for a failing test, a check error or an
// interrupted run, 2 for usage errors and build problems. Runs of one app serialize on
// build/<app>/scratch/test.lock (scratch.Compile's lock); Ctrl-C (SIGINT) or SIGTERM kills the
// tests' process.
func runTestTo(args []string, stdout, stderr io.Writer) (code int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(stderr, "alder test: internal error: %v\n", r)
			code = 2
		}
	}()
	dir, haveDir, jsonOut, runPat := ".", false, false, ""
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json" || a == "-json":
			jsonOut = true
		case (a == "-run" || a == "--run") && i+1 < len(args):
			i++
			runPat = args[i]
		case strings.HasPrefix(a, "-run=") || strings.HasPrefix(a, "--run="):
			_, runPat, _ = strings.Cut(a, "=")
		case strings.HasPrefix(a, "-") || haveDir:
			fmt.Fprintln(stderr, testUsage)
			return 2
		default:
			dir, haveDir = a, true
		}
	}
	if _, err := regexp.Compile(runPat); err != nil { // before the build: a typo should fail at once
		fmt.Fprintf(stderr, "alder test: bad -run pattern: %v\n", err)
		return 2
	}
	if !dirHasApp(dir) {
		fmt.Fprintf(stderr, "alder test: no .alder files under %s/app\n", dir)
		return 2
	}

	b := scratch.Compile(dir, nil, testMain(buildkit.HasGoFiles(filepath.Join(dir, "adapters"))), "test", "")
	defer b.Close()
	if b.Err != nil {
		fmt.Fprintf(stderr, "alder test: %v\n", b.Err)
		return 2
	}
	if testDiags(dir, b.Diags, !b.Runnable(), jsonOut, stdout, stderr) {
		return 1
	}

	env := []string{"ALDER_TEST_RUN=" + runPat, "ALDER_TEST_JSON="}
	if jsonOut {
		env[1] = "ALDER_TEST_JSON=1"
	}
	// the program runs in its own process group: an interrupt reaches alder test alone, which
	// kills the group and stops
	sigCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	errTimeout := fmt.Errorf("the tests timed out after %v", testTimeout)
	ctx, cancel := context.WithTimeoutCause(sigCtx, testTimeout, errTimeout)
	defer cancel()
	out, errOut, exit := scratch.RunContext(ctx, b, env, "")
	io.WriteString(stdout, out)
	switch {
	case sigCtx.Err() != nil:
		fmt.Fprintln(stderr, "alder test: interrupted")
		return 1
	case context.Cause(ctx) == errTimeout:
		fmt.Fprintf(stderr, "alder test: %v\n", errTimeout)
		return 1
	case exit == -1: // not started, or killed for a reason of its own (RunContext says which)
		fmt.Fprintf(stderr, "alder test: the test program stopped: %s\n", tail(errOut, 20))
		return 2
	}
	io.WriteString(stderr, errOut)
	return exit
}

// tail is the last n lines of s, trimmed.
func tail(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.Join(lines[max(0, len(lines)-n):], "\n")
}

// testDiags prints the check diagnostics ds as `alder check` does (an error's text on stdout,
// a warning's on stderr, each with its source line read from dir), or with jsonOut, when they
// stopped the build, the object {"diagnostics": […]} on stdout and nothing else; and reports
// whether they stopped it. stopped is true when nothing was built: an error, or a typed hole
// (whose warning W0002 then explains why, as it does for `alder build`). It is its own
// printer: `alder check` prints inline from the loaded program's sources, and eval's evalDiags
// maps scratch-module positions and writes everything to stderr.
func testDiags(dir string, ds []diag.Diagnostic, stopped, jsonOut bool, stdout, stderr io.Writer) bool {
	failed := stopped
	for _, d := range ds {
		if d.Severity == "error" {
			failed = true
		}
	}
	if jsonOut {
		if failed {
			all := []json.RawMessage{}
			for _, d := range ds {
				all = append(all, json.RawMessage(diag.RenderJSON(d)))
			}
			enc := json.NewEncoder(stdout)
			enc.SetEscapeHTML(false)
			enc.Encode(struct {
				Diagnostics []json.RawMessage `json:"diagnostics"`
			}{all})
		}
		return failed
	}
	files := map[string][]string{}
	for _, d := range ds {
		lines, ok := files[d.Span.File]
		if !ok {
			if src, err := os.ReadFile(filepath.Join(dir, d.Span.File)); err == nil {
				lines = strings.Split(strings.ReplaceAll(string(src), "\r\n", "\n"), "\n")
			}
			files[d.Span.File] = lines
		}
		w := stdout
		if d.Severity == "warning" {
			w = stderr
		}
		fmt.Fprint(w, diag.RenderText(d, lines))
	}
	return failed
}
