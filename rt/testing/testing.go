// Package rtesting runs an app's `test` declarations: the main that `alder test` generates
// calls Main with the table of test cases the app's generated code emits.
//
// Each case runs on its own: a fresh private in-memory SQLite database with the app's tables
// and then the test-only tables of its fakes (so no case sees another's rows) — for an app with
// silos also one per silo value the case touches (db.NewTestStores), each with its own
// transaction for the whole case, which Test.run's savepoints span (db.TestStore) — a clock that
// starts at 2026-01-01T00:00:00Z and ticks one second each time it is read (Ticking), its own
// copy of the capabilities it was given (so Test.fake, which replaces one in the running
// test's Env.Caps, never reaches the next case) and the session it was given. A pure test is a func() bool; a task test is an eff.Task[bool]
// run as a Task (eff.Run with KTask), so its writes happen inside one transaction, which is
// committed when the task succeeds and then thrown away with the database. A property test
// (Case.Prop) runs its body on many drawn cases, each attempt of an effect property against a
// fresh store and Env of its own as above (property.go).
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
	"maps"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"alder/rt"
	"alder/rt/db"
	"alder/rt/eff"
	"alder/rt/html"
	"alder/rt/web"
)

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
//
// t is one Task, as a request's is, so it writes to one store (book 08, "One store per Task"):
// in an app with silos its first write binds it to that store, and a write to a second fails as
// it fails in production (db's routing, eff.Env.Pending). InnerOn binds it beforehand.
func Inner[T any](env *eff.Env, t eff.Task[T]) (v T, msg string, ok bool, trace []string, err error) {
	return inner(env, "", false, t)
}

// InnerOn is Inner for Test.run of a page's update: t is bound to the store of the arm msg takes
// (key, the generated store__M(msg): a silo key, "" for the shared store), as the request's
// db.Enter binds it, so a Test.run fails where the request would.
func InnerOn[T any](env *eff.Env, key string, t eff.Task[T]) (v T, msg string, ok bool, trace []string, err error) {
	return inner(env, key, true, t)
}

func inner[T any](env *eff.Env, key string, arm bool, t eff.Task[T]) (v T, msg string, ok bool, trace []string, err error) {
	if env.Stores != nil {
		bound, pending, sk, sv := env.Bound, env.Pending, env.SiloKey, env.SiloValue
		defer func() { env.Bound, env.Pending, env.SiloKey, env.SiloValue = bound, pending, sk, sv }()
		env.SiloKey, env.SiloValue = key, ""
		env.Bound, env.Pending = arm && key == "", !(arm && key == "")
	}
	session := env.Session
	start := len(env.Trace) // the Test.run step itself is already on the trace
	tx, err := env.Transaction()
	if err != nil {
		return v, "", false, nil, err
	}
	sp, _ := env.Store.(db.Savepointer)
	useSP := sp != nil && tx != nil
	name := eff.SavepointName() // alder_t1, alder_t2, …: shared with eff.Run under a Within
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

// Step records a browser step (`Test.get Todos.Index`, `Test.login`) on the running test's
// trace, before the effects of the requests it makes, which the browser appends after it. The
// generated code records its steps as Calls do; Step is for a step made outside a Call.
func Step(env *eff.Env, name string) {
	if env.Log != nil {
		env.Log("effect", name)
	}
	env.Trace = append(env.Trace, name)
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
	lastEq    bool   // what the last Eq returned
	lastNote  string // the note of the last Eq: where two Docs differ ("" for none)
)

// Eq is rt.Eq that also records Show(l) and Show(r) as the sides of the test's final `==`. Two
// Docs that differ are recorded as where they first differ instead of their whole markup
// (docSides): each side the differing element's markup, cut short, and a note naming its path.
func Eq(l, r any) bool {
	eq, note := rt.Eq(l, r), ""
	var sides [2]string
	if ld, ok := l.(Doc); ok && !eq {
		sides, note = docSides(ld, r.(Doc))
	} else {
		sides = [2]string{rt.Show(l), rt.Show(r)}
	}
	sidesMu.Lock()
	lastSides, hasSides, lastEq, lastNote = sides, true, eq, note
	sidesMu.Unlock()
	return eq
}

// docSides are the sides and the note of a failed `==` of two Docs: the first difference
// (html.Diff, the page's parameters left out as `==` leaves them out), "the documents differ at
// body > main > ul#todos: left 1 child <li>, right 0", with that element's markup on each side.
func docSides(l, r Doc) ([2]string, string) {
	d, ok := html.Diff(html.WithoutPageParams(l.Node), html.WithoutPageParams(r.Node))
	if !ok { // unreachable: Diff and == compare the same trees
		return [2]string{rt.Show(l), rt.Show(r)}, ""
	}
	return [2]string{d.Left, d.Right}, "the documents differ at " + d.At + ": " + d.What
}

// sameSidesNote is the note a failing test's report adds when its last `==` was False although
// its two sides print the same text (a NaN, two values Show cannot tell apart).
const sameSidesNote = "the sides print the same but are different values"

// Case is one `test` declaration: exactly one of Pure, Task and Prop is set (Prop for a
// property test, property.go). File, Line and Col are the position of its `test` keyword (File
// relative to the app, "app/Todo.alder"), for --json.
type Case struct {
	Module, Name string
	File         string
	Line, Col    int
	Pure         func() bool
	Task         func() eff.Task[bool]
	Prop         *Property
}

// CaseResult is the outcome of one Case. Left and Right are the Show text of the sides of the
// test's last Eq ("" when it made none; for two Docs that differ, the differing element's
// markup); Note is sameSidesNote when that Eq was False but its sides print the same, and where
// two Docs differ when they did (docSides); Error is the failure or panic text; Trace is the task's steps (nil for
// a pure test); Ms the wall time in milliseconds.
type CaseResult struct {
	Module, Name string
	File         string // the Case's position, for --json
	Line, Col    int
	OK           bool
	Left, Right  string
	Note         string
	Error        string
	Trace        []string
	Ms           float64

	// A property test's result (property.go) also has Runs, the runs it was to make
	// (Options.Fuzz, so Runs > 0 marks a property), and Seed, the seed of its fresh draws; when
	// it failed, Run is the run that failed (from 1; 0 when none did, as when a health check
	// failed it), Shrinks the attempts the shrinker made, Args the arguments of the (shrunk)
	// failing case, Paste the plain test that keeps it ("" when the generated code prints
	// none, or a literal is not Alder, or it is too long), and Left, Right, Note, Error and
	// Trace are that case's. Crash marks the result `alder test` reports for a property that
	// crashed the whole program: only Seed and Run (from the status file, ParseStatus; Run
	// is savedRun when the saved failure was replaying) are set.
	Args      []ArgValue
	Seed      uint64
	Run, Runs int
	Shrinks   int
	Paste     string
	Crash     bool

	panicked bool // the case panicked (not a task's failure): a mutant it kills is "crashed"
}

// Options configure Run. Run, when set, selects the cases whose `Module "Name"` it matches;
// Tables are created in each case's database (by Migrations when set: when they do not
// produce Tables, every case fails, pure ones too, with notTheSchema), then TestTables;
// a copy of Caps (an empty map when nil) and Session are
// each case's Env.Caps and Env.Session; Now is when each case's clock starts (zero:
// 2026-01-01T00:00:00Z; see Ticking);
// Deadline, when set, is (100 ms after it) the deadline of a task test's context (its store
// operations and capabilities see it).
type Options struct {
	Run *regexp.Regexp
	// Tables are each task test's tables: made by applying Migrations (db.Migrate) when the
	// app has any, else by db.Ensure.
	Tables []db.TableDesc
	// TestTables are the test-only tables of the app's fakes (the generated FakeTables): made
	// in each task test's database after Tables (db.SQLite.CreateTables), never part of the
	// app's schema, so the check of Migrations against Tables never sees them.
	TestTables []db.TableDesc
	Migrations []db.Migration
	Caps       map[string]any
	Session    any
	Now        time.Time
	Deadline   time.Time
	// Routes and Web are the app's routes and router options (the generated app.Routes and
	// app.Web(): the options Serve builds, without Store, Caps, Now, Log, Dev, Secret or Reload):
	// each task test's browser (BrowserOf) serves them. nil Routes: the app has no browser, and
	// BrowserOf fails the test.
	Routes []web.Route
	Web    web.Options
	// Fuzz is how many runs each property test makes (0: 100); Seed seeds their fresh draws when
	// SeedSet (else each property picks one at random); FuzzDB is the directory of saved
	// failures (build/<app>/fuzz; "": none are read or written). Mutating (a --mutate run) fixes
	// the seed at Seed (0 unless set) and turns off shrinking and saved failures.
	Fuzz     int
	Seed     uint64
	SeedSet  bool
	FuzzDB   string
	Mutating bool
	// Status is Main's status file (EnvStatus): a property test writes its seed and run there
	// before each attempt (unless Mutating). "": none.
	Status string

	schemaErr string // why Migrations do not make Tables (set by defaults): every case fails with it
}

// notTheSchema is the failure of every test of an app whose migrations lead to a schema that
// is not its tables' (db.ErrSchemaAhead: its tables changed without `alder migrate`). Any other
// refusal of the migrations (a file edited after it was written, a broken chain) fails every
// test with the runtime's own text, which names the file and the remedy.
const notTheSchema = "the migrations do not produce the app's schema: run `alder migrate`"

// defaultNow is when a test's clock starts unless told otherwise.
var defaultNow = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// Clock is a test's time: its first reading is start, and each later one is a second after
// the one before; a delay (Env.Sleep) advances it by the delay at once. It is deterministic
// (the same test reads the same times on every run), and the rows a test inserts get distinct
// `@created` times in the order they were inserted, so a test can observe an ordering by
// creation time (`Db.orderBy T.created.desc`).
type Clock struct {
	mu   sync.Mutex
	next time.Time
}

func NewClock(start time.Time) *Clock { return &Clock{next: start} }

func (c *Clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := c.next
	c.next = c.next.Add(time.Second)
	return t
}

func (c *Clock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.next = c.next.Add(d)
}

// Ticking is a clock's Now: each reading advances it a second.
func Ticking(start time.Time) func() time.Time { return NewClock(start).Now }

// unwired is the panic message of a capability call that finds no implementation (as in
// cmd/alder/eval.go).
var unwired = regexp.MustCompile(`alder: capability "([^"]*)" is not wired`)

// key is what -run matches and the report prints: Module "Name".
func key(c Case) string { return c.Module + " \"" + c.Name + "\"" }

// Run runs the cases o.Run selects, in order, each against its own fresh store, and returns
// their results. A panic fails its case and the next case still runs.
func Run(cases []Case, o Options) []CaseResult {
	o = defaults(o)
	out := []CaseResult{}
	for _, c := range selected(cases, o) {
		out = append(out, runCase(c, o))
	}
	return out
}

// defaults fills o's zero Now and nil Caps, and applies o.Migrations once to a database of
// its own to see that they produce o.Tables (schemaErr).
func defaults(o Options) Options {
	if o.Now.IsZero() {
		o.Now = defaultNow
	}
	if o.Caps == nil {
		o.Caps = map[string]any{}
	}
	if o.Fuzz <= 0 {
		o.Fuzz = defaultFuzz
	}
	if len(o.Migrations) > 0 {
		o.schemaErr = ""
		store, err := db.OpenSQLite(":memory:")
		if err == nil {
			_, err = store.Migrate(context.Background(), o.Tables, o.Migrations, false)
			store.Close()
		}
		if errors.Is(err, db.ErrSchemaAhead) {
			o.schemaErr = notTheSchema
		} else if err != nil {
			o.schemaErr = err.Error()
		}
	}
	return o
}

// ensure gives a case's fresh store o's tables — by o.Migrations when there are any, else by
// Ensure — and then o.TestTables (CreateTables). The order is required: Migrate's final check
// requires the live tables to be exactly the app's, so it must never run after CreateTables.
func ensure(ctx context.Context, store *db.SQLite, o Options) error {
	if len(o.Migrations) > 0 {
		if _, err := store.Migrate(ctx, o.Tables, o.Migrations, false); err != nil {
			return err
		}
	} else if err := store.Ensure(ctx, o.Tables, true); err != nil {
		return err
	}
	return store.CreateTables(ctx, o.TestTables, o.Tables)
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

func runCase(c Case, o Options) (r CaseResult) {
	r = CaseResult{Module: c.Module, Name: c.Name, File: c.File, Line: c.Line, Col: c.Col}
	start := time.Now()
	defer func() { r.Ms = float64(time.Since(start).Microseconds()) / 1000 }()
	resetSides()

	ok, errText := false, ""
	switch {
	case o.schemaErr != "":
		errText = o.schemaErr
	case c.Prop != nil:
		return runProperty(c, o, r)
	case c.Pure != nil:
		ok, errText, r.panicked = guard(c.Pure)
	case c.Task != nil:
		env, done, err := newCaseEnv(o)
		if err != nil {
			r.Error = err.Error()
			return r
		}
		defer done()
		ok, errText, r.panicked = guard(func() bool { return runTask(env, c.Task()) })
		r.Trace = append([]string{}, env.Trace...)
	default:
		errText = "test has no body"
	}
	r.OK = ok && errText == ""
	r.Error = unwiredText(errText)
	r.Left, r.Right, r.Note = readSides(r.OK)
	return r
}

// newCaseEnv is a task test's Env (made again for each attempt of an effect property): a fresh
// private in-memory store with o's tables and then its test tables (and, for an app with silos,
// one per silo value the test touches), a copy of o.Caps (Test.fake replaces entries of the
// case's own), o.Session, a clock from o.Now, the browser when o has routes, and a context with
// o's deadline. done closes what it opened.
func newCaseEnv(o Options) (env *eff.Env, done func(), err error) {
	var closers []func()
	done = func() {
		for i := len(closers) - 1; i >= 0; i-- {
			closers[i]()
		}
	}
	ctx := context.Background()
	if !o.Deadline.IsZero() {
		var cancel context.CancelFunc
		// a little after the deadline, so Main's timer (not a failed step) reports it
		ctx, cancel = context.WithDeadline(ctx, o.Deadline.Add(100*time.Millisecond))
		closers = append(closers, cancel)
	}
	store, err := db.OpenSQLite(":memory:")
	if err != nil {
		done()
		return nil, nil, err
	}
	closers = append(closers, func() { store.Close() })
	if err := ensure(ctx, store, o); err != nil {
		done()
		return nil, nil, err
	}
	caps := maps.Clone(o.Caps)
	if caps == nil {
		caps = map[string]any{}
	}
	clock := NewClock(o.Now)
	env = &eff.Env{Ctx: ctx, Store: store, Caps: caps, Session: o.Session, Now: clock.Now, Sleep: clock.Advance, CheckSession: o.Web.SessionCheck}
	if len(o.Web.Silos) > 0 { // a store per silo value the test touches, each in memory
		stores, err := db.NewTestStores(store, db.StoresConfig{Silos: o.Web.Silos, Tables: o.Tables, Migrations: o.Migrations, Dev: true})
		if err != nil {
			done()
			return nil, nil, err
		}
		closers = append(closers, func() { stores.Close() })
		env.Store, env.Stores = db.TestStore{SQLite: store, Stores: stores}, stores
	}
	if o.Routes != nil {
		env.Ctx = withBrowser(env, o.Routes, o.Web)
	}
	return env, done, nil
}

// runTask runs t as a task test in env (eff.Run with KTask); its error leaves as a runErr panic,
// for guard.
func runTask(env *eff.Env, t eff.Task[bool]) bool {
	v, err := eff.Run(env, eff.KTask, t)
	if err != nil {
		panic(runErr{err})
	}
	return v
}

// unwiredText is a failure's text with an unwired capability's panic said as tests say it.
func unwiredText(errText string) string {
	return unwired.ReplaceAllString(errText, "capability $1 is not wired in tests")
}

// resetSides forgets the last Eq's sides: before each case, and each attempt of a property.
func resetSides() {
	sidesMu.Lock()
	lastSides, hasSides, lastEq, lastNote = [2]string{}, false, false, ""
	sidesMu.Unlock()
}

// readSides are the Left, Right and Note of a case that passed (ok) or not, from the last Eq
// it made (all "" when it made none).
func readSides(ok bool) (left, right, note string) {
	sidesMu.Lock()
	defer sidesMu.Unlock()
	if !hasSides {
		return "", "", ""
	}
	left, right = lastSides[0], lastSides[1]
	if !ok && !lastEq && left == right {
		note = sameSidesNote
	}
	if !ok && lastNote != "" {
		note = lastNote
	}
	return left, right, note
}

// runErr carries a task's error out of guard's function.
type runErr struct{ err error }

// guard calls f and turns a panic into failure text; panicked is true for a panic that is not
// a task's error (runErr).
func guard(f func() bool) (ok bool, errText string, panicked bool) {
	defer func() {
		if p := recover(); p != nil {
			ok, panicked = false, true
			if x, isRun := p.(runErr); isRun {
				errText, panicked = x.err.Error(), false
			} else {
				errText = panicText(p)
			}
		}
	}()
	return f(), "", false
}

// panicText is a recovered panic value as failure text.
func panicText(p any) string {
	if e, ok := p.(error); ok {
		return e.Error()
	}
	return fmt.Sprint(p)
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
	File   string   `json:"file"`
	Line   int      `json:"line"`
	Col    int      `json:"col"`
	OK     bool     `json:"ok"`
	Left   string   `json:"left,omitempty"`
	Right  string   `json:"right,omitempty"`
	Note   string   `json:"note,omitempty"`
	Error  string   `json:"error,omitempty"`
	Trace  []string `json:"trace"`
	Ms     float64  `json:"ms"`
	// Property is a property test's runs, seed and failing case (propertyJSON); nil otherwise.
	Property *jsonProperty `json:"property,omitempty"`
}

// WriteResult writes r as the report prints it: with asJSON one JSON object on one line (a
// property's with "property": propertyJSON); otherwise `ok   M "name"`, or `FAIL M "name"`
// (a property's followed on that line by where it failed and then by a line per argument:
// propertyHeader, writeArgs) followed by indented `left:`/`right:` lines
// (when the test made an Eq), `note:` (when that Eq was False but its sides print the same),
// `error:` (when it has one; its continuation lines indented by four spaces), `trace:` (a
// task test's steps, as Show of a List Text) and, for a property, `seed:` and the case to keep
// (writeSeed). `alder test` uses it to report a test whose program crashed.
func WriteResult(w io.Writer, r CaseResult, asJSON bool) error {
	if asJSON {
		tr := r.Trace
		if tr == nil {
			tr = []string{}
		}
		enc := json.NewEncoder(w)
		enc.SetEscapeHTML(false)
		return enc.Encode(jsonResult{Module: r.Module, Name: r.Name, File: r.File, Line: r.Line, Col: r.Col, OK: r.OK,
			Left: r.Left, Right: r.Right, Note: r.Note, Error: r.Error, Trace: tr, Ms: r.Ms, Property: propertyJSON(r)})
	}
	var b strings.Builder
	if r.OK {
		fmt.Fprintf(&b, "ok   %s %s\n", r.Module, QuoteName(r.Name))
	} else {
		fmt.Fprintf(&b, "FAIL %s %s%s\n", r.Module, QuoteName(r.Name), propertyHeader(r))
		writeArgs(&b, r)
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
		writeSeed(&b, r)
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
	// EnvStatus names a file Main truncates and writes `Module\tName\n` to before each test (a property test adds `\tseed\trun`, ParseStatus),
	// and empties when it returns: after a crash it names the test that crashed.
	EnvStatus = "ALDER_TEST_STATUS"
	// EnvFuzz is the runs per property test (--fuzz), EnvSeed the seed of their fresh draws
	// (--seed), EnvFuzzDB the directory of their saved failures (build/<app>/fuzz).
	EnvFuzz   = "ALDER_TEST_FUZZ"
	EnvSeed   = "ALDER_TEST_SEED"
	EnvFuzzDB = "ALDER_TEST_FUZZ_DB"
	// EnvStdinEOF=1 makes the program exit (2, "alder test: parent gone") when its standard
	// input reaches end of file: `alder test` holds the pipe open while it runs, so the program
	// dies with it.
	EnvStdinEOF = "ALDER_TEST_STDIN_EOF"
)

// Main runs cases with the flags in args (-run re, --json, --fuzz n, --seed s) and env
// (ALDER_TEST_RUN, ALDER_TEST_JSON=1, EnvFuzz, EnvSeed; flags win; EnvFuzzDB has no flag), prints the report to out/err, returns the exit code: 0 all passed, 1 a failure, 2 a usage error.
//
// The report has one entry per case (WriteResult), printed as the case finishes, and ends with
// `N passed, M failed`; an app with no tests at all (not a -run that selects none) is followed in
// text mode, without --mutate, by noTestsHint. With --json stdout has one JSON object per case and nothing else; the
// summary goes to err. A case running for slowAfter (10 s) prints `still running M "name" (10s)`
// on err, and again every 10 s.
//
// Main also reads the variables EnvDeadline, EnvTimeout, EnvStatus and EnvStdinEOF: when the
// deadline passes the running case is reported as failed ("timed out after <timeout>"), the
// summary printed, and Main returns 1 without waiting for it (the caller exits, which stops it).
//
// tables are the app's tables (Options.Tables), testTables the test-only tables of its fakes
// (Options.TestTables: the generated FakeTables of a test build).
//
// routes and webOpts are the app's routes and router options (Options.Routes and Options.Web),
// which each task test's browser serves; nil routes: no browser (a build without them).
//
// With --mutate (or EnvMutate=1; a mutate build, whose main called SetMutants first) the run
// goes on, when every selected test passed, with the mutants (mutate.go): the report then ends
// with `N mutants: K killed, S survived  (score X.XX)` and the survivors, or with --json the
// objects {"mutants": […]} and {"score", …} on stdout. A failing suite ends instead with
// "tests fail: fix them before measuring mutants" (exit 1), no selected test with "no tests:
// nothing can kill a mutant" ("no test matches -run: …" when -run selected none; exit 1 with
// --min, else 0), each where the summary goes. --min R (EnvMin) exits 1 when the score is
// below R; -timeout d (EnvTimeout, default 10s) bounds each mutant's run, which gets at least
// 3 × the suite's own run (warned about when that is over half of d); --mutate-from k (EnvMutateFrom) starts at mutant k, skipping the
// suite's own run, and EnvMutateLog names the log of decided mutants (see mutate.go for the
// continuation after a crash).
func Main(cases []Case, tables, testTables []db.TableDesc, migrations []db.Migration, caps map[string]any, session any, routes []web.Route, webOpts web.Options, args []string, out, err io.Writer) int {
	started := time.Now()
	fs := flag.NewFlagSet("alder test", flag.ContinueOnError)
	fs.SetOutput(err)
	runPat := fs.String("run", os.Getenv("ALDER_TEST_RUN"), "run only the tests whose `Module \"name\"` matches this regexp")
	asJSON := fs.Bool("json", os.Getenv("ALDER_TEST_JSON") == "1", "print one JSON object per test")
	mutating := fs.Bool("mutate", os.Getenv(EnvMutate) == "1", "after the tests pass, run them once per mutant (a mutate build)")
	minText := fs.String("min", os.Getenv(EnvMin), "with --mutate, exit 1 when the mutation score is below this")
	timeoutText := fs.String("timeout", os.Getenv(EnvTimeout), "with --mutate, how long one mutant's run may take (default 10s)")
	fromText := fs.String("mutate-from", os.Getenv(EnvMutateFrom), "with --mutate, the id of the first mutant to run (above 1: the tests' own run is skipped)")
	fuzzText := fs.String("fuzz", os.Getenv(EnvFuzz), "how many runs each property test makes (default 100)")
	seedText := fs.String("seed", os.Getenv(EnvSeed), "the seed of the property tests' fresh draws (default random; saved failures are replayed first regardless)")
	if e := fs.Parse(args); e != nil {
		return 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(err, "alder test: unexpected argument %q\n", fs.Arg(0))
		return 2
	}
	minScore, from, mutantTimeout := -1.0, 1, defaultMutantTimeout
	if *minText != "" {
		v, e := strconv.ParseFloat(*minText, 64)
		if e != nil || v < 0 || v > 1 {
			fmt.Fprintf(err, "alder test: bad --min %q: want a score from 0 to 1, such as 0.8\n", *minText)
			return 2
		}
		minScore = v
	}
	if *mutating {
		if mutation.active == nil {
			fmt.Fprintln(err, "alder test: --mutate: this test program was not built with mutants")
			return 2
		}
		if *fromText != "" {
			v, e := strconv.Atoi(*fromText)
			if e != nil || v < 1 {
				fmt.Fprintf(err, "alder test: bad --mutate-from %q\n", *fromText)
				return 2
			}
			from = v
		}
		if *timeoutText != "" {
			d, e := time.ParseDuration(*timeoutText)
			if e != nil || d <= 0 {
				fmt.Fprintf(err, "alder test: bad -timeout %q\n", *timeoutText)
				return 2
			}
			mutantTimeout = d
		}
	} else if minScore >= 0 {
		fmt.Fprintln(err, "alder test: --min needs --mutate")
		return 2
	}
	if os.Getenv(EnvStdinEOF) == "1" {
		go func() {
			io.Copy(io.Discard, os.Stdin)
			fmt.Fprintln(err, "alder test: parent gone")
			os.Exit(2)
		}()
	}
	o := Options{Tables: tables, TestTables: testTables, Migrations: migrations, Caps: caps, Session: session, Now: defaultNow, Routes: routes, Web: webOpts,
		FuzzDB: os.Getenv(EnvFuzzDB), Mutating: *mutating}
	if *fuzzText != "" {
		v, e := strconv.Atoi(*fuzzText)
		if e != nil || v < 1 {
			fmt.Fprintf(err, "alder test: bad --fuzz %q: want a number of runs, such as 100\n", *fuzzText)
			return 2
		}
		o.Fuzz = v
	}
	if *seedText != "" {
		v, e := strconv.ParseUint(*seedText, 10, 64)
		if e != nil {
			fmt.Fprintf(err, "alder test: bad --seed %q: want a whole number, such as 42\n", *seedText)
			return 2
		}
		o.Seed, o.SeedSet = v, true
	}
	if *runPat != "" {
		re, e := regexp.Compile(*runPat)
		if e != nil {
			fmt.Fprintf(err, "alder test: bad -run pattern: %v\n", e)
			return 2
		}
		o.Run = re
	}
	timeout := *timeoutText
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
	o.Status = status
	o = defaults(o)

	run := selected(cases, o)
	passed, failed := 0, 0
	// report writes a report line: on out, or with --json on err (out holds JSON only)
	report := func(line string) {
		if *asJSON {
			io.WriteString(err, line)
		} else {
			io.WriteString(out, line)
		}
	}
	summary := func() { report(fmt.Sprintf("%d passed, %d failed\n", passed, failed)) }
	mutantLog := os.Getenv(EnvMutateLog)
	if *mutating && from > 1 { // the tests passed in an earlier run of the program
		return mutate(run, o, mutantRun{timeout: mutantTimeout, baseline: readBaseline(mutantLog), status: status, log: mutantLog, from: from}, minScore, *asJSON, out, err)
	}
	var deadline <-chan time.Time
	if !o.Deadline.IsZero() {
		timer := time.NewTimer(time.Until(o.Deadline))
		defer timer.Stop()
		deadline = timer.C
	}
	suiteStart := time.Now()
	for i, c := range run {
		if status != "" {
			if e := os.WriteFile(status, []byte(c.Module+"\t"+c.Name+"\n"), 0o644); e != nil {
				fmt.Fprintf(err, "alder test: %v\n", e)
				return 2
			}
		}
		done := make(chan CaseResult, 1)
		go func() { done <- runCase(c, o) }()
		var r CaseResult
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
				r = CaseResult{Module: c.Module, Name: c.Name, File: c.File, Line: c.Line, Col: c.Col, Error: "timed out after " + timeout, Ms: float64(time.Since(start).Microseconds()) / 1000}
				if e := WriteResult(out, r, *asJSON); e != nil {
					fmt.Fprintf(err, "alder test: writing the report: %v\n", e)
					return 2
				}
				summary()
				note := ""
				if rest := len(run) - i - 1; rest > 0 {
					note = fmt.Sprintf("; %d did not run", rest)
				}
				if *mutating && mutantTimeout == defaultMutantTimeout {
					note += mutateTimeoutHint
				}
				fmt.Fprintf(err, "alder test: the tests timed out after %s%s\n", timeout, note)
				if *mutating {
					report(testsFailText + "\n")
				}
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
	baseline := time.Since(suiteStart)
	summary()
	if failed > 0 {
		if *mutating {
			report(testsFailText + "\n")
		}
		return 1
	}
	if !*mutating {
		if len(cases) == 0 && !*asJSON {
			fmt.Fprintln(out, noTestsHint)
		}
		return 0
	}
	if len(run) == 0 {
		if o.Run != nil && len(cases) > 0 {
			report(noRunMatchText + "\n")
		} else {
			report(noTestsText + "\n")
		}
		if minScore >= 0 {
			return 1
		}
		return 0
	}
	if baseline > mutantTimeout/2 {
		fmt.Fprintf(err, "alder test: the tests took %s, over half the -timeout %s: each mutant gets %s (%d × the tests' time)\n",
			baseline.Round(100*time.Millisecond), mutantTimeout, mutantLimit(mutantTimeout, baseline).Round(100*time.Millisecond), baselineFactor)
	}
	if mutantLog != "" {
		if e := LogMutant(mutantLog, MutantOutcome{BaselineMs: float64(baseline.Microseconds()) / 1000}); e != nil {
			fmt.Fprintf(err, "alder test: %v\n", e)
			return 2
		}
	}
	return mutate(run, o, mutantRun{timeout: mutantTimeout, baseline: baseline, status: status, log: mutantLog, from: from}, minScore, *asJSON, out, err)
}

// mutate is Main's mutation run over the selected cases, after they passed: the mutants, the
// report, and the exit code (1 below minScore when that is not negative; ExitMutantTimedOut
// when a mutant timed out under a log: runMutants).
func mutate(run []Case, o Options, r mutantRun, minScore float64, asJSON bool, out, errw io.Writer) int {
	outcomes, stop, err := runMutants(run, o, r)
	if err != nil {
		fmt.Fprintf(errw, "alder test: %v\n", err)
		return 2
	}
	if stop {
		return ExitMutantTimedOut
	}
	sc, err := writeMutationReport(out, outcomes, asJSON)
	if err != nil {
		fmt.Fprintf(errw, "alder test: writing the report: %v\n", err)
		return 2
	}
	if minScore >= 0 && sc < minScore {
		return 1
	}
	return 0
}
