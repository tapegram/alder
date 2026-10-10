// Package rtesting runs an app's `test` declarations: the main that `alder test` generates
// calls Main with the table of test cases the app's generated code emits.
//
// Each case runs on its own: a fresh private in-memory SQLite database with the app's tables
// (so no case sees another's rows), the fixed clock 2026-01-01T00:00:00Z, the capabilities
// and the session it was given. A pure test is a func() bool; a task test is an eff.Task[bool]
// run as a Task (eff.Run with KTask), so its writes happen inside one transaction, which is
// committed when the task succeeds and then thrown away with the database.
//
// Test.run is what lets a task test look at a failure: its generated code is a Call that
// calls Inner, which runs the task it was given inside the running Task (eff.RunInner) under
// a savepoint. A savepoint is a named mark inside the open transaction: when the inner task
// fails, ROLLBACK TO the mark undoes exactly the writes it made and the test's earlier writes
// stay; when it succeeds, RELEASE forgets the mark and its writes become part of the test's
// transaction. Savepoints nest, so Test.run inside Test.run works the same way: a failing
// inner run undoes only its own writes, and a failing outer run undoes the inner run's
// writes too, even when the inner run succeeded.
//
// Tests run one after another (Main runs each in a goroutine of its own, so it can stop
// waiting at the deadline): Eq records the sides of the test's last `==` in package state that
// Run reads after each case, so Run must not be called concurrently.
package rtesting

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"alder/rt"
	"alder/rt/db"
	"alder/rt/eff"
)

// savepoints numbers the savepoints Inner opens, so nested and sequential ones never share a
// name (alder_t1, alder_t2, …).
var savepoints atomic.Int64

// Inner runs t under a savepoint as Test.run does: ok is true with v on success; on *eff.ErrFail,
// eff.ErrNotFound or *eff.ErrCapability (a capability's adapter returned an error) the
// savepoint is rolled back, the session restored, and msg is the failure text ("not found" for
// ErrNotFound, "<Name>: <msg>" for a capability's error); trace is the steps t performed. Any
// other error is returned.
//
// Inner is called from a Call's function while a Task runs (env.Kind is KTask); it begins the
// Task's transaction if no step has yet, so the savepoint has a transaction to live in. When
// env.Store does not implement db.Savepointer (or there is no store) t runs without one and a
// failure undoes nothing in the store. On an error other than a failure the savepoint is
// still rolled back, so the store is consistent, and the error is returned for the enclosing
// Run to fail the test with. A panic in t is not recovered: it unwinds through the enclosing
// Run, which rolls back the whole transaction.
func Inner[T any](env *eff.Env, t eff.Task[T]) (v T, msg string, ok bool, trace []string, err error) {
	session := env.Session
	start := len(env.Trace) // the Test.run step itself is already on the trace
	tx, err := env.Transaction()
	if err != nil {
		return v, "", false, nil, err
	}
	sp, _ := env.Store.(db.Savepointer)
	useSP := sp != nil && tx != nil
	name := "alder_t" + strconv.FormatInt(savepoints.Add(1), 10)
	if useSP {
		if err := sp.Savepoint(env.Ctx, tx.(db.Tx), name); err != nil {
			return v, "", false, nil, err
		}
	}
	v, rerr := eff.RunInner(env, t)
	trace = append([]string{}, env.Trace[min(start, len(env.Trace)):]...)
	if rerr == nil {
		if useSP {
			if err := sp.Release(env.Ctx, tx.(db.Tx), name); err != nil {
				return v, "", false, trace, err
			}
		}
		return v, "", true, trace, nil
	}
	if useSP {
		if err := sp.RollbackTo(env.Ctx, tx.(db.Tx), name); err != nil {
			return v, "", false, trace, errors.Join(rerr, err)
		}
		// ROLLBACK TO keeps the savepoint open; release it so it does not linger.
		if err := sp.Release(env.Ctx, tx.(db.Tx), name); err != nil {
			return v, "", false, trace, errors.Join(rerr, err)
		}
	}
	var f *eff.ErrFail
	var c *eff.ErrCapability
	switch {
	case errors.As(rerr, &c):
		env.Session = session
		return v, c.Error(), false, trace, nil
	case errors.As(rerr, &f):
		env.Session = session
		return v, f.Msg, false, trace, nil
	case errors.Is(rerr, eff.ErrNotFound):
		env.Session = session
		return v, "not found", false, trace, nil
	}
	return v, "", false, trace, rerr
}

// Trace is the enclosing run's trace so far, without the Test.trace step itself.
func Trace(env *eff.Env) []string {
	if len(env.Trace) == 0 {
		return []string{}
	}
	return append([]string{}, env.Trace[:len(env.Trace)-1]...)
}

// The sides of the running test's last Eq, as Show text. Package state: runCase clears it
// before each case and reads it after, which is right because cases run one at a time. The
// mutex only keeps a case Main gave up on (timed out, still running in its goroutine) from
// racing with the next Main in the same process (rtesting's own tests).
var (
	sidesMu   sync.Mutex
	lastSides [2]string
	hasSides  bool
	lastEq    bool // what the last Eq returned
)

// Eq is rt.Eq that also records Show(l) and Show(r) as the sides of the test's final `==`.
func Eq(l, r any) bool {
	sides, eq := [2]string{rt.Show(l), rt.Show(r)}, rt.Eq(l, r)
	sidesMu.Lock()
	lastSides, hasSides, lastEq = sides, true, eq
	sidesMu.Unlock()
	return eq
}

// sameSidesNote is the note a failing test's report adds when its last `==` was False although
// its two sides print the same text (a NaN, two values Show cannot tell apart).
const sameSidesNote = "the sides print the same but are different values"

// Case is one `test` declaration: exactly one of Pure and Task is set.
type Case struct {
	Module, Name string
	Pure         func() bool
	Task         func() eff.Task[bool]
}

// Result is the outcome of one Case. Left and Right are the Show text of the sides of the
// test's last Eq ("" when it made none); Note is sameSidesNote when that Eq was False but its
// sides print the same; Error is the failure or panic text; Trace is the task's steps (nil for
// a pure test); Ms the wall time in milliseconds.
type Result struct {
	Module, Name string
	OK           bool
	Left, Right  string
	Note         string
	Error        string
	Trace        []string
	Ms           float64
}

// Options configure Run. Run, when set, selects the cases whose `Module "Name"` it matches;
// Tables are created in each case's database; Caps (an empty map when nil) and Session are
// each case's Env.Caps and Env.Session; Now is the fixed clock (zero: 2026-01-01T00:00:00Z);
// Deadline, when set, is the deadline of a task test's context (its store operations and
// capabilities see it).
type Options struct {
	Run      *regexp.Regexp
	Tables   []db.TableDesc
	Caps     map[string]any
	Session  any
	Now      time.Time
	Deadline time.Time
}

// defaultNow is the clock tests see unless told otherwise.
var defaultNow = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// unwired is the panic message of a capability call that finds no implementation (as in
// cmd/alder/eval.go).
var unwired = regexp.MustCompile(`alder: capability "([^"]*)" is not wired`)

// key is what -run matches and the report prints: Module "Name".
func key(c Case) string { return c.Module + " \"" + c.Name + "\"" }

// Run runs the cases o.Run selects, in order, each against its own fresh store, and returns
// their results. A panic fails its case and the next case still runs.
func Run(cases []Case, o Options) []Result {
	o = defaults(o)
	out := []Result{}
	for _, c := range selected(cases, o) {
		out = append(out, runCase(c, o))
	}
	return out
}

// defaults fills o's zero Now and nil Caps.
func defaults(o Options) Options {
	if o.Now.IsZero() {
		o.Now = defaultNow
	}
	if o.Caps == nil {
		o.Caps = map[string]any{}
	}
	return o
}

// selected is the cases o.Run selects, in order.
func selected(cases []Case, o Options) []Case {
	out := []Case{}
	for _, c := range cases {
		if o.Run == nil || o.Run.MatchString(key(c)) {
			out = append(out, c)
		}
	}
	return out
}

func runCase(c Case, o Options) (r Result) {
	caps, now := o.Caps, o.Now
	r = Result{Module: c.Module, Name: c.Name}
	start := time.Now()
	defer func() { r.Ms = float64(time.Since(start).Microseconds()) / 1000 }()
	sidesMu.Lock()
	lastSides, hasSides, lastEq = [2]string{}, false, false
	sidesMu.Unlock()

	ok, errText := false, ""
	switch {
	case c.Pure != nil:
		ok, errText = guard(c.Pure)
	case c.Task != nil:
		ctx := context.Background()
		if !o.Deadline.IsZero() {
			var cancel context.CancelFunc
			ctx, cancel = context.WithDeadline(ctx, o.Deadline)
			defer cancel()
		}
		store, err := db.OpenSQLite(":memory:")
		if err != nil {
			r.Error = err.Error()
			return r
		}
		defer store.Close()
		if err := store.Ensure(ctx, o.Tables, true); err != nil {
			r.Error = err.Error()
			return r
		}
		env := &eff.Env{Ctx: ctx, Store: store, Caps: caps, Session: o.Session, Now: func() time.Time { return now }}
		ok, errText = guard(func() bool {
			v, err := eff.Run(env, eff.KTask, c.Task())
			if err != nil {
				panic(runErr{err})
			}
			return v
		})
		r.Trace = append([]string{}, env.Trace...)
	default:
		errText = "test has no body"
	}
	r.OK = ok && errText == ""
	r.Error = unwired.ReplaceAllString(errText, "capability $1 is not wired in tests")
	sidesMu.Lock()
	defer sidesMu.Unlock()
	if hasSides {
		r.Left, r.Right = lastSides[0], lastSides[1]
		if !r.OK && !lastEq && r.Left == r.Right {
			r.Note = sameSidesNote
		}
	}
	return r
}

// runErr carries a task's error out of guard's function.
type runErr struct{ err error }

// guard calls f and turns a panic into failure text.
func guard(f func() bool) (ok bool, errText string) {
	defer func() {
		if p := recover(); p != nil {
			ok = false
			switch x := p.(type) {
			case runErr:
				errText = x.err.Error()
			case error:
				errText = x.Error()
			default:
				errText = fmt.Sprint(x)
			}
		}
	}()
	return f(), ""
}

// QuoteName is a test name as the text report prints it: in double quotes, escaped
// (strconv.Quote) only when it contains a newline or a double quote, so every report line
// stays one line.
func QuoteName(name string) string {
	if strings.ContainsAny(name, "\n\"") {
		return strconv.Quote(name)
	}
	return "\"" + name + "\""
}

// jsonResult is one line of --json output.
type jsonResult struct {
	Module string   `json:"module"`
	Name   string   `json:"name"`
	OK     bool     `json:"ok"`
	Left   string   `json:"left,omitempty"`
	Right  string   `json:"right,omitempty"`
	Note   string   `json:"note,omitempty"`
	Error  string   `json:"error,omitempty"`
	Trace  []string `json:"trace"`
	Ms     float64  `json:"ms"`
}

// WriteResult writes r as the report prints it: with asJSON one JSON object on one line;
// otherwise `ok   M "name"`, or `FAIL M "name"` followed by indented `left:`/`right:` lines
// (when the test made an Eq), `note:` (when that Eq was False but its sides print the same),
// `error:` (when it has one; its continuation lines indented by four spaces) and `trace:` (a
// task test's steps, as Show of a List Text). `alder test` uses it to report a test whose
// program crashed.
func WriteResult(w io.Writer, r Result, asJSON bool) error {
	if asJSON {
		tr := r.Trace
		if tr == nil {
			tr = []string{}
		}
		enc := json.NewEncoder(w)
		enc.SetEscapeHTML(false)
		return enc.Encode(jsonResult{r.Module, r.Name, r.OK, r.Left, r.Right, r.Note, r.Error, tr, r.Ms})
	}
	var b strings.Builder
	if r.OK {
		fmt.Fprintf(&b, "ok   %s %s\n", r.Module, QuoteName(r.Name))
	} else {
		fmt.Fprintf(&b, "FAIL %s %s\n", r.Module, QuoteName(r.Name))
		if r.Left != "" || r.Right != "" {
			fmt.Fprintf(&b, "  left:  %s\n  right: %s\n", r.Left, r.Right)
		}
		if r.Note != "" {
			fmt.Fprintf(&b, "  note:  %s\n", r.Note)
		}
		if r.Error != "" {
			fmt.Fprintf(&b, "  error: %s\n", strings.ReplaceAll(r.Error, "\n", "\n    "))
		}
		if r.Trace != nil {
			fmt.Fprintf(&b, "  trace: %s\n", rt.Show(r.Trace))
		}
	}
	_, e := io.WriteString(w, b.String())
	return e
}

// slowAfter is how long a test runs before Main says it is still running, and how often it
// says so again.
var slowAfter = 10 * time.Second

// The environment `alder test` passes the program besides ALDER_TEST_RUN and ALDER_TEST_JSON.
const (
	// EnvDeadline is when the run must end (RFC 3339): the test running then fails as timed out.
	EnvDeadline = "ALDER_TEST_DEADLINE"
	// EnvTimeout is the -timeout the deadline came from (a Go duration), for the message.
	EnvTimeout = "ALDER_TEST_TIMEOUT"
	// EnvStatus names a file Main truncates and writes `Module\tName\n` to before each test,
	// and empties when it returns: after a crash it names the test that crashed.
	EnvStatus = "ALDER_TEST_STATUS"
	// EnvStdinEOF=1 makes the program exit (2, "alder test: parent gone") when its standard
	// input reaches end of file: `alder test` holds the pipe open while it runs, so the program
	// dies with it.
	EnvStdinEOF = "ALDER_TEST_STDIN_EOF"
)

// Main runs cases with the flags in args (-run re, --json) and env (ALDER_TEST_RUN, ALDER_TEST_JSON=1,
// flags win), prints the report to out/err, returns the exit code: 0 all passed, 1 a failure, 2 a usage error.
//
// The report has one entry per case (WriteResult), printed as the case finishes, and ends with
// `N passed, M failed`. With --json stdout has one JSON object per case and nothing else; the
// summary goes to err. A case running for slowAfter (10 s) prints `still running M "name" (10s)`
// on err, and again every 10 s.
//
// Main also reads the variables EnvDeadline, EnvTimeout, EnvStatus and EnvStdinEOF: when the
// deadline passes the running case is reported as failed ("timed out after <timeout>"), the
// summary printed, and Main returns 1 without waiting for it (the caller exits, which stops it).
func Main(cases []Case, tables []db.TableDesc, caps map[string]any, session any, args []string, out, err io.Writer) int {
	started := time.Now()
	fs := flag.NewFlagSet("alder test", flag.ContinueOnError)
	fs.SetOutput(err)
	runPat := fs.String("run", os.Getenv("ALDER_TEST_RUN"), "run only the tests whose `Module \"name\"` matches this regexp")
	asJSON := fs.Bool("json", os.Getenv("ALDER_TEST_JSON") == "1", "print one JSON object per test")
	if e := fs.Parse(args); e != nil {
		return 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(err, "alder test: unexpected argument %q\n", fs.Arg(0))
		return 2
	}
	if os.Getenv(EnvStdinEOF) == "1" {
		go func() {
			io.Copy(io.Discard, os.Stdin)
			fmt.Fprintln(err, "alder test: parent gone")
			os.Exit(2)
		}()
	}
	o := Options{Tables: tables, Caps: caps, Session: session, Now: defaultNow}
	if *runPat != "" {
		re, e := regexp.Compile(*runPat)
		if e != nil {
			fmt.Fprintf(err, "alder test: bad -run pattern: %v\n", e)
			return 2
		}
		o.Run = re
	}
	timeout := os.Getenv(EnvTimeout)
	if d := os.Getenv(EnvDeadline); d != "" {
		t, e := time.Parse(time.RFC3339Nano, d)
		if e != nil {
			fmt.Fprintf(err, "alder test: bad %s: %v\n", EnvDeadline, e)
			return 2
		}
		o.Deadline = t
		if timeout == "" {
			timeout = t.Sub(started).Round(time.Second).String()
		}
	}
	status := os.Getenv(EnvStatus)
	if status != "" {
		defer os.WriteFile(status, nil, 0o644)
	}
	o = defaults(o)

	run := selected(cases, o)
	passed, failed := 0, 0
	summary := func() {
		line := fmt.Sprintf("%d passed, %d failed\n", passed, failed)
		if *asJSON {
			io.WriteString(err, line)
		} else {
			io.WriteString(out, line)
		}
	}
	var deadline <-chan time.Time
	if !o.Deadline.IsZero() {
		timer := time.NewTimer(time.Until(o.Deadline))
		defer timer.Stop()
		deadline = timer.C
	}
	for i, c := range run {
		if status != "" {
			if e := os.WriteFile(status, []byte(c.Module+"\t"+c.Name+"\n"), 0o644); e != nil {
				fmt.Fprintf(err, "alder test: %v\n", e)
				return 2
			}
		}
		done := make(chan Result, 1)
		go func() { done <- runCase(c, o) }()
		var r Result
		start := time.Now()
		slow := time.NewTicker(slowAfter)
	wait:
		for {
			select {
			case r = <-done:
				break wait
			case <-slow.C:
				fmt.Fprintf(err, "still running %s %s (%s)\n", c.Module, QuoteName(c.Name), time.Since(start).Round(time.Second))
			case <-deadline:
				slow.Stop()
				failed++
				r = Result{Module: c.Module, Name: c.Name, Error: "timed out after " + timeout, Ms: float64(time.Since(start).Microseconds()) / 1000}
				if e := WriteResult(out, r, *asJSON); e != nil {
					fmt.Fprintf(err, "alder test: writing the report: %v\n", e)
					return 2
				}
				summary()
				note := ""
				if rest := len(run) - i - 1; rest > 0 {
					note = fmt.Sprintf("; %d did not run", rest)
				}
				fmt.Fprintf(err, "alder test: the tests timed out after %s%s\n", timeout, note)
				return 1
			}
		}
		slow.Stop()
		if r.OK {
			passed++
		} else {
			failed++
		}
		if e := WriteResult(out, r, *asJSON); e != nil {
			fmt.Fprintf(err, "alder test: writing the report: %v\n", e)
			return 2
		}
	}
	summary()
	if failed > 0 {
		return 1
	}
	return 0
}
