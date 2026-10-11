package main

import (
	"bytes"
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
	"time"

	"alder/internal/diag"
	"alder/internal/scratch"
	rtesting "alder/rt/testing"
)

// testUsage is the usage line of `alder test`.
const testUsage = "usage: alder test [dir] [-run re] [-timeout d] [--json]"

// defaultTestTimeout bounds one `alder test` run (every test together) unless -timeout says.
const defaultTestTimeout = 5 * time.Minute

// testGrace is how long after the deadline `alder test` waits for the test program to report
// the test that timed out before it kills the program itself.
const testGrace = 5 * time.Second

// testMain is the Go main `alder test` compiles with the app (a test build, so app.FakeTables
// exists): it hands the app's test table, tables, its fakes' test-only tables, migrations,
// capabilities and session to rtesting.Main, which reads -run and --json from the
// environment (ALDER_TEST_RUN, ALDER_TEST_JSON). With withCaps (scratch.Checked.WiresAdapters)
// the capabilities are the app's adapters, as the generated caps package wires them
// (caps.Caps(): a capability whose adapter is missing is left unwired); otherwise none is
// wired, and a test that calls one without Test.fake fails.
func testMain(withCaps bool) string {
	imp, caps := "", "app.Caps{}.Registry()"
	if withCaps {
		imp, caps = "\t\"app/caps\"\n", "caps.Caps().Registry()"
	}
	return `package main

import (
	"os"

	rtesting "alder/rt/testing"
	"alder/rt/web"
	"app"
` + imp + `)

func main() {
	caps := ` + caps + `
	os.Exit(rtesting.Main(app.Tests, app.Tables, app.FakeTables, app.Migrations, caps, app.TestSession(), nil, web.Options{}, os.Args[1:], os.Stdout, os.Stderr))
}
`
}

// runTestTo is `alder test [dir] [-run re] [-timeout d] [--json]`: it compiles the app at dir
// (default ".") with a main that runs its `test` declarations (testMain) into
// build/<app>/scratch/test, runs it, and passes the program's report through as it is written:
// one entry per test as the test finishes and a summary, or with --json one JSON object per
// test on stdout (the summary on stderr). -run selects the tests whose `Module "name"` matches
// the regexp. -timeout (default 5m) bounds the run: the program gets the deadline
// (rtesting.EnvDeadline) and fails the test running when it passes. Check diagnostics are
// printed as `alder check` prints them; with --json they are one object {"diagnostics": […]}
// on stdout and nothing else. With --json any other problem after the arguments are read (a
// build failure, a bad -run, a program that stopped outside any test) is the one object
// {"error": "<message>"} on stdout, its message also on stderr.
//
// The program writes the test it is about to run to a status file (rtesting.EnvStatus); when
// it dies some other way than exiting 0 or 1 (a stack overflow, a fatal runtime error), the
// test it was running is reported as failed with the first lines of the crash report (the
// error and the top frame), never the whole Go trace, and no later test runs. The program's
// standard input is a pipe this process holds open (rtesting.EnvStdinEOF), so if `alder test`
// is killed, even with SIGKILL, the program sees end of file and exits.
//
// The adapters are checked as alder build checks them, with the same messages, except that a
// capability without an adapter is not refused: it runs unwired (a test that calls it without
// Test.fake fails "not wired in tests"), and a warning listing each one goes to stderr first.
//
// Exit 0 when every selected test passed; 1 for a failing, crashed or timed-out test, a check
// error, a typed hole, adapters refused or that do not compile with the app (alder build's
// message), or an interrupted run; 2 for usage errors and other build problems. Runs of one
// app serialize on build/<app>/scratch/test.lock (scratch.Compile's lock); Ctrl-C (SIGINT) or
// SIGTERM kills the tests' process.
func runTestTo(args []string, stdout, stderr io.Writer) (code int) {
	jsonOut := false
	// fail reports a problem that is not a test's: on stderr, and with --json as {"error": msg}.
	fail := func(msg string) {
		fmt.Fprintf(stderr, "alder test: %s\n", msg)
		if jsonOut {
			enc := json.NewEncoder(stdout)
			enc.SetEscapeHTML(false)
			enc.Encode(map[string]string{"error": msg})
		}
	}
	defer func() {
		if r := recover(); r != nil {
			fail(fmt.Sprintf("internal error: %v", r))
			code = 2
		}
	}()
	dir, haveDir, runPat, timeout := ".", false, "", defaultTestTimeout
	timeoutArg := ""
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
		case (a == "-timeout" || a == "--timeout") && i+1 < len(args):
			i++
			timeoutArg = args[i]
		case strings.HasPrefix(a, "-timeout=") || strings.HasPrefix(a, "--timeout="):
			_, timeoutArg, _ = strings.Cut(a, "=")
		case strings.HasPrefix(a, "-") || haveDir:
			fmt.Fprintln(stderr, testUsage)
			return 2
		default:
			dir, haveDir = a, true
		}
	}
	if timeoutArg != "" {
		d, err := time.ParseDuration(timeoutArg)
		if err != nil || d <= 0 {
			fail(fmt.Sprintf("bad -timeout %q: want a positive duration such as 30s or 2m", timeoutArg))
			return 2
		}
		timeout = d
	}
	if _, err := regexp.Compile(runPat); err != nil { // before the build: a typo should fail at once
		fail(fmt.Sprintf("bad -run pattern: %v", err))
		return 2
	}
	if !dirHasApp(dir) {
		fail(fmt.Sprintf("no .alder files under %s/app", dir))
		return 2
	}

	checked := scratch.Check(dir, nil)
	b := scratch.CompileCheckedWith(context.Background(), checked, testMain(checked.WiresAdapters()), "test", "", scratch.Options{Tests: true})
	defer b.Close()
	io.WriteString(stderr, b.Warning) // capabilities without adapters run unwired
	if b.Err != nil {
		fail(b.Err.Error())
		if adapters := (*scratch.AdapterError)(nil); errors.As(b.Err, &adapters) {
			return 1 // the user's adapters, as alder build says
		}
		return 2
	}
	if testDiags(dir, b.Diags, !b.Runnable(), jsonOut, stdout, stderr) {
		if !hasError(b.Diags) { // stopped by a typed hole: its W0002 says where
			fmt.Fprintln(stderr, "alder test: no tests ran (fill the hole)")
		}
		return 1
	}
	return runTests(b, runPat, timeout, jsonOut, stdout, stderr, fail)
}

// hasError reports whether ds holds an error.
func hasError(ds []diag.Diagnostic) bool {
	for _, d := range ds {
		if d.Severity == "error" {
			return true
		}
	}
	return false
}

// runTests runs the built test program b (see runTestTo) and returns the exit code.
func runTests(b scratch.Build, runPat string, timeout time.Duration, jsonOut bool, stdout, stderr io.Writer, fail func(string)) int {
	status := filepath.Join(b.Dir, "status")
	os.Remove(status)
	defer os.Remove(status)
	// the program's stdin: it exits at end of file, which comes when this process closes the
	// write end below or dies
	pr, pw, err := os.Pipe()
	if err != nil {
		fail(err.Error())
		return 2
	}
	defer pr.Close()
	defer pw.Close()

	deadline := time.Now().Add(timeout)
	env := []string{"ALDER_TEST_RUN=" + runPat, "ALDER_TEST_JSON=",
		rtesting.EnvDeadline + "=" + deadline.Format(time.RFC3339Nano),
		rtesting.EnvTimeout + "=" + timeout.String(),
		rtesting.EnvStatus + "=" + status,
		rtesting.EnvStdinEOF + "=1"}
	if jsonOut {
		env[1] = "ALDER_TEST_JSON=1"
	}
	// the program runs in its own process group: an interrupt reaches alder test alone, which
	// kills the group and stops
	sigCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	errTimeout := fmt.Errorf("the tests timed out after %v", timeout)
	ctx, cancel := context.WithDeadlineCause(sigCtx, deadline.Add(testGrace), errTimeout)
	defer cancel()
	crash := &crashFilter{w: stderr}
	counted := &resultCounter{w: stdout, json: jsonOut}
	exit := scratch.RunStream(ctx, b, env, pr, counted, crash)
	crash.flush()
	if sigCtx.Err() != nil {
		fmt.Fprintln(stderr, "alder test: interrupted")
		return 1
	}
	if exit == 0 || exit == 1 {
		crash.passKept()
		return exit
	}
	timedOut := context.Cause(ctx) == errTimeout
	if mod, name, ok := readStatus(status); ok {
		msg := crash.summary()
		if timedOut {
			msg = "timed out after " + timeout.String()
		}
		rtesting.WriteResult(stdout, rtesting.CaseResult{Module: mod, Name: name, Error: msg}, jsonOut)
		if timedOut {
			fmt.Fprintf(stderr, "alder test: %v\n", errTimeout)
		} else {
			fmt.Fprintf(stderr, "alder test: the test program crashed running %s %s; no later test ran\n", mod, rtesting.QuoteName(name))
		}
		// the summary the program could not print: the results it streamed and this failure
		summary := fmt.Sprintf("%d passed, %d failed\n", counted.passed, counted.failed+1)
		if jsonOut {
			io.WriteString(stderr, summary)
		} else {
			io.WriteString(stdout, summary)
		}
		return 1
	}
	switch {
	case exit == 2 && !crash.crashed: // the program's own usage error, already on stderr
		return 2
	case timedOut:
		fail(errTimeout.Error())
		return 1
	}
	fail("the test program stopped: " + crash.summary())
	return 2
}

// resultCounter passes the test program's standard output through and counts the results in
// it, line by line: `ok   …` and `FAIL …` lines, or with json each object's "ok".
type resultCounter struct {
	w              io.Writer
	json           bool
	partial        []byte
	passed, failed int
}

func (c *resultCounter) Write(p []byte) (int, error) {
	c.partial = append(c.partial, p...)
	for {
		i := bytes.IndexByte(c.partial, '\n')
		if i < 0 {
			break
		}
		line := c.partial[:i]
		if c.json {
			var r struct {
				OK *bool `json:"ok"`
			}
			if json.Unmarshal(line, &r) == nil && r.OK != nil {
				if *r.OK {
					c.passed++
				} else {
					c.failed++
				}
			}
		} else if bytes.HasPrefix(line, []byte("ok   ")) {
			c.passed++
		} else if bytes.HasPrefix(line, []byte("FAIL ")) {
			c.failed++
		}
		c.partial = c.partial[i+1:]
	}
	return c.w.Write(p)
}

// readStatus reads the status file the test program writes before each test: `Module\tName\n`,
// or nothing when no test was running.
func readStatus(path string) (mod, name string, ok bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", "", false
	}
	line := strings.TrimSuffix(string(b), "\n")
	mod, name, ok = strings.Cut(line, "\t")
	return mod, name, ok && mod != ""
}

// crashFilter passes the test program's standard error through line by line until a line that
// starts a Go crash report (or a note of the runner's), and from that line on keeps it instead
// (at most maxCrash bytes), so a crash is summarized, not dumped.
type crashFilter struct {
	w       io.Writer
	partial []byte
	kept    strings.Builder
	crashed bool
}

const maxCrash = 64 << 10

// crashStart are the prefixes of the first line of a crash report.
var crashStart = []string{"panic: ", "fatal error: ", "runtime: ", "goroutine ", "SIG", "unexpected fault address", "[signal ", "scratch: "}

func (c *crashFilter) Write(p []byte) (int, error) {
	c.partial = append(c.partial, p...)
	for {
		i := bytes.IndexByte(c.partial, '\n')
		if i < 0 {
			break
		}
		c.line(string(c.partial[:i+1]))
		c.partial = c.partial[i+1:]
	}
	return len(p), nil
}

func (c *crashFilter) line(l string) {
	if !c.crashed {
		for _, p := range crashStart {
			if strings.HasPrefix(l, p) {
				c.crashed = true
			}
		}
	}
	if !c.crashed {
		io.WriteString(c.w, l)
		return
	}
	if c.kept.Len() < maxCrash {
		c.kept.WriteString(l)
	}
}

// flush handles a last line with no newline.
func (c *crashFilter) flush() {
	if len(c.partial) > 0 {
		c.line(string(c.partial) + "\n")
		c.partial = nil
	}
}

// passKept writes what was kept through after all (the program exited normally).
func (c *crashFilter) passKept() { io.WriteString(c.w, c.kept.String()) }

// summary is the first three lines worth showing of the kept crash report: its error line
// ("fatal error: stack overflow", "panic: …") and the top frame of the running goroutine (the
// function, and its file and line without the offsets).
func (c *crashFilter) summary() string {
	lines := strings.Split(strings.TrimSpace(c.kept.String()), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return "the test program stopped with no output"
	}
	out := []string{}
	for _, l := range lines {
		if strings.HasPrefix(l, "fatal error: ") || strings.HasPrefix(l, "panic: ") {
			out = append(out, l)
			break
		}
	}
	if len(out) == 0 {
		out = append(out, lines[0])
	}
	g := -1
	for i, l := range lines {
		if strings.HasPrefix(l, "goroutine ") && (g < 0 || strings.Contains(l, "[running]")) {
			g = i
			if strings.Contains(l, "[running]") {
				break
			}
		}
	}
	if g >= 0 {
		for _, l := range lines[g+1 : min(g+3, len(lines))] {
			l = strings.TrimSpace(l)
			if i := strings.Index(l, " +0x"); i > 0 {
				l = l[:i]
			}
			if l != "" {
				out = append(out, l)
			}
		}
	}
	return strings.Join(out, "\n")
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
