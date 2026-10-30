package rtesting

import (
	"crypto/rand"
	"crypto/sha1"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"alder/rt"
	"alder/rt/eff"
	"alder/rt/fuzz"
)

// Arg is one argument of a property test: its name in the lambda (or of forAll's function),
// its type as the program writes it, and whether a type variable in it defaulted to Int (spec
// §4.4), which the report then shows.
type Arg struct {
	Name, Type string
	Defaulted  bool
}

// Property is a property test's body as the generated code builds it: Draw draws one value per
// argument from the choices, Lit prints each as an Alder literal, and exactly one of Pure and
// Task runs the body on them (Task for a block body or Fuzz.forAllTask). Case, when set, is the
// paste-ready plain test for the literals of a failing case.
type Property struct {
	Args []Arg
	Draw func(*fuzz.Choices) []any
	Lit  func([]any) []string
	Pure func([]any) bool
	Task func([]any) eff.Task[bool]
	Case func(lits []string) string
}

// ForAll is `Fuzz.forAll g f`: one argument drawn by g. The generated code fills in Args and
// Lit.
func ForAll[A any](g fuzz.Fuzzer[A], f func(A) bool) *Property {
	return &Property{
		Draw: func(c *fuzz.Choices) []any { return []any{g(c)} },
		Pure: func(vs []any) bool { return f(vs[0].(A)) },
	}
}

// ForAllTask is `Fuzz.forAllTask g f`, whose body is a Task.
func ForAllTask[A any](g fuzz.Fuzzer[A], f func(A) eff.Task[bool]) *Property {
	return &Property{
		Draw: func(c *fuzz.Choices) []any { return []any{g(c)} },
		Task: func(vs []any) eff.Task[bool] { return f(vs[0].(A)) },
	}
}

// ArgValue is one argument of a failing property's case: Arg with the value as a literal.
type ArgValue struct {
	Name, Type, Value string
	Defaulted         bool
}

// The property loop (spec §5, §7). A run is an attempt whose values were drawn (a rejected or
// overrun draw is an attempt but no run).
const (
	defaultFuzz = 100 // runs per property unless Options.Fuzz says otherwise
	// shrinkBudget bounds the attempts the shrinker makes, so shrinking cannot hang.
	shrinkBudget = 1000
	// attemptsPerRun caps a property's fresh attempts at attemptsPerRun × its runs.
	attemptsPerRun = 10
	// healthAttempts is when the filter health check looks: after this many fresh attempts a
	// predicate rejecting more than maxRejected of them fails the property.
	healthAttempts = 50
	maxRejected    = 0.9
)

// How a health check stopped a property (CaseResult.stopped); "" for a failing case.
const (
	stoppedOn     = "on"     // a run that was too slow
	stoppedBefore = "before" // a draw that never made its run (it drew too much)
)

// attemptLimit is how long one attempt may take before it fails the property (a variable, so
// tests can shorten it).
var attemptLimit = 5 * time.Second

// attempt is the outcome of drawing values from one Choices and running the body on them.
type attempt struct {
	vals     []any
	seq      []uint64 // the choices made, as recorded
	overrun  bool     // the draw ran past a replayed sequence or MaxChoices
	rejected bool     // a Fuzz.filter or brand predicate said no …
	reject   string   // … and this is its name
	ok       bool
	errText  string
	panicked bool
	trace    []string // an effect property's steps
	dur      time.Duration
	// the sides of its last Eq (readSides)
	left, right, note string
}

// failed reports whether a drew values and the body did not hold on them.
func (a attempt) failed() bool { return !a.overrun && !a.rejected && !a.ok }

// runAttempt draws p's values from ch and runs p's body on them: a Pure body directly, a Task
// body as a task test against an Env of its own (newCaseEnv: a fresh store, caps, clock and
// browser each attempt, so no attempt sees another's writes). A Go panic, in the draw or the
// body, is a failure.
func runAttempt(p *Property, o Options, ch *fuzz.Choices) (a attempt) {
	resetSides()
	start := time.Now()
	defer func() {
		a.dur, a.seq = time.Since(start), ch.Recorded()
		a.left, a.right, a.note = readSides(a.ok)
	}()
	if !draw(p, ch, &a) {
		return a
	}
	switch {
	case p.Pure != nil:
		a.ok, a.errText, a.panicked = guard(func() bool { return p.Pure(a.vals) })
	case p.Task != nil:
		env, done, err := newCaseEnv(o)
		if err != nil {
			a.errText = err.Error()
			return a
		}
		defer done()
		a.ok, a.errText, a.panicked = guard(func() bool { return runTask(env, p.Task(a.vals)) })
		a.trace = append([]string{}, env.Trace...)
	default:
		a.errText = "test has no body"
	}
	a.ok = a.ok && a.errText == ""
	return a
}

// draw draws p's values into a.vals, reporting whether it did: an Overrun or Reject panic marks
// a as overrun or rejected, any other panic fails it.
func draw(p *Property, ch *fuzz.Choices, a *attempt) (drawn bool) {
	defer func() {
		if v := recover(); v != nil {
			switch x := v.(type) {
			case fuzz.Overrun:
				a.overrun = true
			case fuzz.Reject:
				a.rejected, a.reject = true, x.What
			default:
				a.errText, a.panicked = panicText(v), true
			}
		}
	}()
	a.vals = p.Draw(ch)
	return true
}

// runProperty runs property test c (r is its result so far) as spec §7 says: the saved failure
// first (unless o.Mutating), then fresh attempts from the seed until o.Fuzz runs are made or
// one fails; a failure is shrunk (unless o.Mutating), saved, and reported from one more run of
// the shrunk case. A property that passes every run deletes its saved failure. The health
// checks fail it when its draws are mostly rejected (filterTooTight), when a fresh draw makes
// more than fuzz.MaxChoices choices, and when one attempt takes over attemptLimit.
func runProperty(c Case, o Options, r CaseResult) CaseResult {
	p := c.Prop
	r.Runs, r.Seed = o.Fuzz, o.Seed
	if !o.SeedSet && !o.Mutating {
		r.Seed = randomSeed()
	}
	path := ""
	if o.FuzzDB != "" && !o.Mutating {
		path = savedPath(o.FuzzDB, c.Module, c.Name)
	}

	var failing *attempt
	runs := 0
	// ran counts a run and reports whether it ends the loop (too slow, or failed)
	ran := func(a attempt) bool {
		runs++
		if a.dur > attemptLimit {
			r.Run, r.stopped = runs, stoppedOn
			r.Error = fmt.Sprintf("run %d took %s, over the %s one case may take", runs, a.dur.Round(time.Millisecond), attemptLimit)
			r.Args = argValues(p, a.vals)
			if path != "" && !o.Mutating { // saved like a failure: the next run replays it first
				if err := save(path, a.seq); err != nil {
					r.Error += "\ncould not save the slow case: " + err.Error()
				}
			}
			return true
		}
		if a.failed() {
			r.Run, failing = runs, &a
			return true
		}
		return false
	}
	st := statusLine{c: c, seed: r.Seed}
	if !o.Mutating { // a --mutate run's status names the mutant (MutantStatus)
		st.path = o.Status
	}
	if seq, ok := readSaved(path); ok {
		st.at(savedRun)
		if a := runAttempt(p, o, fuzz.Replay(seq)); !a.overrun && !a.rejected && ran(a) {
			r.Run = savedRun // not a run of this seed: the report says so
			return finish(c, o, r, failing, path)
		}
	}
	rejects := map[string]int{}
	for i := 0; runs < o.Fuzz; i++ {
		if i == healthAttempts || i == attemptsPerRun*o.Fuzz {
			// At the cap fewer than o.Fuzz of the attempts made a run, so over 90% were
			// rejected and the check always fails the property here.
			if msg := filterTooTight(rejects, i); msg != "" {
				r.Error = msg
				return r
			}
		}
		st.at(runs + 1)
		a := runAttempt(p, o, fuzz.Fresh(attemptSeed(r.Seed, i)))
		switch {
		case a.overrun:
			r.Run, r.stopped = runs+1, stoppedBefore
			r.Error = fmt.Sprintf("generator draws too much: one case made over %d random choices; draw fewer or smaller values (Fuzz.listOf bounds a list)", fuzz.MaxChoices)
			return r
		case a.rejected:
			rejects[a.reject]++
			continue
		}
		if ran(a) {
			return finish(c, o, r, failing, path)
		}
	}
	if path != "" {
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			r.Error = "could not delete the saved failure: " + err.Error()
			return r
		}
	}
	r.OK = true
	return r
}

// finish completes the result of a property that failed: a too-slow run (failing nil) is
// reported as it is; a failing case is shrunk, run once more so its values, sides and
// recorded choices are those of the shrunk case (fuzz.Shrink returns the candidate as given),
// and saved at path.
func finish(c Case, o Options, r CaseResult, failing *attempt, path string) CaseResult {
	p := c.Prop
	if failing == nil {
		return r
	}
	final := *failing
	if !o.Mutating {
		tooSlow := false // a candidate ran over attemptLimit: the body is too slow to shrink on
		best, steps := fuzz.Shrink(failing.seq, func(seq []uint64) bool {
			if tooSlow {
				return false
			}
			a := runAttempt(p, o, fuzz.Replay(seq))
			if a.dur > attemptLimit {
				tooSlow = true
				return false
			}
			switch {
			case a.overrun:
				panic(fuzz.Overrun{})
			case a.rejected:
				panic(fuzz.Reject{What: a.reject})
			}
			return a.failed()
		}, shrinkBudget)
		r.Shrinks = steps
		if a := runAttempt(p, o, fuzz.Replay(best)); a.failed() {
			final = a
		}
	}
	r.Args = argValues(p, final.vals)
	r.Left, r.Right, r.Note = final.left, final.right, final.note
	r.Error, r.Trace, r.panicked = unwiredText(final.errText), final.trace, final.panicked
	r.Paste = pasteCase(p, final.vals)
	if path != "" {
		if err := save(path, final.seq); err != nil {
			r.Error = strings.TrimPrefix(r.Error+"\n", "\n") + "could not save the failing case: " + err.Error()
		}
	}
	return r
}

// maxPasteLines is the longest paste-ready case the report prints (spec §6); a longer one, or
// one with a literal that is no Alder, is pasteFallback instead.
const (
	maxPasteLines = 20
	pasteWidth    = 100 // a line wider than this counts as the lines it would wrap to
	pasteFallback = "paste the values above into a plain test"
)

// pasteCase is the plain test that keeps the failing case vs (p.Case of their literals): ""
// when there is none, when a literal is not Alder (fuzz.Pasteable), or when it is over
// maxPasteLines lines.
func pasteCase(p *Property, vs []any) string {
	if p.Case == nil || vs == nil {
		return ""
	}
	lits := literals(p, vs)
	for _, l := range lits {
		if !fuzz.Pasteable(l) {
			return ""
		}
	}
	s := p.Case(lits)
	if pastedLines(s) > maxPasteLines {
		return ""
	}
	return s
}

// pastedLines is the lines s takes, a line wider than pasteWidth code points counting once for
// each pasteWidth of it, so a 500-element list on one line is a long case.
func pastedLines(s string) int {
	n := 0
	for _, l := range strings.Split(strings.TrimSuffix(s, "\n"), "\n") {
		n += max(1, (utf8.RuneCountInString(l)+pasteWidth-1)/pasteWidth)
	}
	return n
}

// Paste is the paste-ready case of template tmpl (the generated Property.Case's) with literal
// lits[i] put in for each placeholder "\x00" mode i "\x00". The mode is the place's: 'a' where
// an application would need parentheses (an argument, a field access's base), so a literal
// that is no atom gets them (fuzz.Atom), and 'p' where it would not; 'A' and 'P' are those
// inside a match subject, where a literal that starts with `{` also needs them (it would
// open the arms).
func Paste(tmpl string, lits []string) string {
	var b strings.Builder
	for {
		i := strings.IndexByte(tmpl, 0)
		if i < 0 || i+2 >= len(tmpl) {
			b.WriteString(tmpl)
			return b.String()
		}
		j := strings.IndexByte(tmpl[i+2:], 0)
		if j < 0 {
			b.WriteString(tmpl)
			return b.String()
		}
		mode, n := tmpl[i+1], 0
		for _, d := range tmpl[i+2 : i+2+j] {
			n = n*10 + int(d-'0')
		}
		lit := lits[n]
		if mode == 'a' || mode == 'A' {
			lit = fuzz.Atom(lit)
		}
		if (mode == 'A' || mode == 'P') && opensBraces(lit) {
			lit = "(" + lit + ")"
		}
		b.WriteString(tmpl[:i])
		b.WriteString(lit)
		tmpl = tmpl[i+2+j+1:]
	}
}

// opensBraces reports whether lit has a `{` outside any string, parentheses or brackets: in a
// match subject it would read as the opening of the arms (`match Just { a: 1 } {`).
func opensBraces(lit string) bool {
	depth, inString := 0, false
	for i := 0; i < len(lit); i++ {
		switch c := lit[i]; {
		case inString:
			if c == '\\' {
				i++
			} else if c == '"' {
				inString = false
			}
		case c == '"':
			inString = true
		case c == '(' || c == '[':
			depth++
		case c == ')' || c == ']':
			depth--
		case c == '{' && depth <= 0:
			return true
		}
	}
	return false
}

// savedRun is the run a status line gives while the saved failure is replayed: no seed and run
// reproduce that attempt, the saved file does (it is replayed first again).
const savedRun = -1

// statusLine writes property test c's line of Main's status file (EnvStatus) before each
// attempt: `Module\tName\t\x00fuzz\tseed\trun` (the \x00fuzz tag is what tells it from a plain
// test whose name ends in two numbers), the run being the one the attempt makes (or the failing
// run while it is shrunk; savedRun while the saved failure is replayed), so after a crash of
// the whole program the report says how to reproduce it (ParseStatus). No path: nothing is
// written.
type statusLine struct {
	path string
	c    Case
	seed uint64
	run  int // the run last written
}

func (s *statusLine) at(run int) {
	if s.path == "" || run == s.run {
		return
	}
	s.run = run
	os.WriteFile(s.path, []byte(fmt.Sprintf("%s\t%s\t%s\t%d\t%d\n", s.c.Module, s.c.Name, statusTag, s.seed, run)), 0o644)
}

// statusTag marks a property test's status line, after its name (no test name has a NUL).
const statusTag = "\x00fuzz"

// ParseStatus reads a line of Main's status file: `Module\tName` before a test, followed by
// `\t<statusTag>\tseed\trun` while a property test runs (run is then from 1, or savedRun while its saved
// failure is replayed; 0 for any other line). ok is false for an empty file.
func ParseStatus(line string) (mod, name string, seed uint64, run int, ok bool) {
	mod, name, ok = strings.Cut(strings.TrimSuffix(line, "\n"), "\t")
	if !ok || mod == "" {
		return "", "", 0, 0, false
	}
	// a name may hold a tab: the tag, seed and run are the last three fields
	if i := strings.LastIndex(name, "\t"+statusTag+"\t"); i >= 0 {
		seedRun := strings.Split(name[i+len(statusTag)+2:], "\t")
		if len(seedRun) == 2 {
			s, e1 := strconv.ParseUint(seedRun[0], 10, 64)
			r, e2 := strconv.Atoi(seedRun[1])
			if e1 == nil && e2 == nil && (r > 0 || r == savedRun) {
				return mod, name[:i], s, r, true
			}
		}
	}
	return mod, name, 0, 0, true
}

// filterTooTight is the filter health check's failure after attempts fresh attempts, of which
// rejects[what] were rejected by the predicate what: "" unless they rejected over maxRejected.
func filterTooTight(rejects map[string]int, attempts int) string {
	total, worst, what := 0, 0, ""
	for w, n := range rejects {
		total += n
		if n > worst || n == worst && w < what {
			worst, what = n, w
		}
	}
	if attempts == 0 || float64(total) <= maxRejected*float64(attempts) {
		return ""
	}
	pct := total * 100 / attempts
	if what == "Fuzz.filter" {
		return fmt.Sprintf("`Fuzz.filter`'s predicate rejects %d%% of generated values; write a `Fuzz.forAll` with a generator that produces only values it accepts", pct)
	}
	return fmt.Sprintf("`%s`'s predicate rejects %d%% of generated values; write a `Fuzz.forAll` with a generator that produces `%s` values", what, pct, what)
}

// literals are vs as Alder literals: p.Lit's, or Show when the property has no printer.
func literals(p *Property, vs []any) []string {
	if p.Lit != nil {
		return p.Lit(vs)
	}
	out := make([]string, len(vs))
	for i, v := range vs {
		out[i] = rt.Show(v)
	}
	return out
}

// argValues are p's arguments with the values vs (nil when none were drawn).
func argValues(p *Property, vs []any) []ArgValue {
	if vs == nil {
		return nil
	}
	lits := literals(p, vs)
	out := make([]ArgValue, len(vs))
	for i := range vs {
		a := Arg{Name: "value"}
		if i < len(p.Args) {
			a = p.Args[i]
		}
		out[i] = ArgValue{Name: a.Name, Type: a.Type, Value: lits[i], Defaulted: a.Defaulted}
	}
	return out
}

// attemptSeed is the seed of fresh attempt i of a property run with seed: splitmix64 of the
// two, so neighbouring attempts draw unrelated choices.
func attemptSeed(seed uint64, i int) uint64 {
	z := seed + uint64(i+1)*0x9e3779b97f4a7c15
	z = (z ^ z>>30) * 0xbf58476d1ce4e5b9
	z = (z ^ z>>27) * 0x94d049bb133111eb
	return z ^ z>>31
}

// randomSeed is a seed for a run without --seed: random, below 10^10 so the report's `seed:`
// is short to copy and exact as a JSON number.
func randomSeed() uint64 {
	var b [8]byte
	rand.Read(b[:]) // crypto/rand: it cannot fail (a failure crashes the program), so there is no error to handle
	return binary.LittleEndian.Uint64(b[:]) % 1e10
}

// savedPath is where test name of module's saved failure lives under dir:
// <dir>/<module>/<first 16 hex digits of sha1(name)>.
func savedPath(dir, module, name string) string {
	sum := sha1.Sum([]byte(name))
	return filepath.Join(dir, module, hex.EncodeToString(sum[:])[:16])
}

// readSaved is the choice sequence saved at path, ok false when there is none (or path is "",
// or the file is unreadable: a failure is then found afresh and saved over it).
func readSaved(path string) ([]uint64, bool) {
	if path == "" {
		return nil, false
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	seq, err := fuzz.Decode(b)
	return seq, err == nil
}

func save(path string, seq []uint64) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, fuzz.Encode(seq), 0o644)
}

// propertyHeader is what a failing property's FAIL line adds: where it failed.
func propertyHeader(r CaseResult) string {
	if r.Runs == 0 || r.Run == 0 {
		return ""
	}
	verb := "failed on run"
	switch r.stopped {
	case stoppedOn:
		verb = "stopped on run" // a run that was too slow
	case stoppedBefore:
		verb = "stopped before run" // a draw that never made its run
	}
	s := fmt.Sprintf("  (%s %d of %d", verb, r.Run, r.Runs)
	if r.Run == savedRun {
		s = "  (failed on the saved case"
		if r.stopped == stoppedOn {
			s = "  (stopped on the saved case"
		}
	}
	if r.Shrinks > 0 {
		s += fmt.Sprintf(", shrunk in %d steps", r.Shrinks)
	}
	return s + ")"
}

// writeArgs writes a failing property's arguments, one `  name: literal` line each; an
// argument whose type defaulted to Int ends with its type, `  (xs : List Int)`.
func writeArgs(b *strings.Builder, r CaseResult) {
	for _, a := range r.Args {
		fmt.Fprintf(b, "  %s: %s", a.Name, a.Value)
		if a.Defaulted {
			fmt.Fprintf(b, "  (%s : %s)", a.Name, a.Type)
		}
		b.WriteByte('\n')
	}
}

// writeSeed writes a failing property's `  seed:` line and the plain test that keeps its case.
// A failing case with no paste-ready test (none made, over maxPasteLines, a literal that is no
// Alder) says to paste the argument lines instead. A property that crashed the program (Crash)
// gives the seed and the run that reproduce it.
func writeSeed(b *strings.Builder, r CaseResult) {
	saved := func() {
		fmt.Fprintf(b, "  run: the saved failing case, replayed first (build/<app>/fuzz/%s/)\n", r.Module)
	}
	if r.Crash {
		if r.Run == savedRun {
			saved()
		} else {
			fmt.Fprintf(b, "  seed: %d\n  run: %d\n", r.Seed, r.Run)
		}
		return
	}
	if r.Runs == 0 {
		return
	}
	if r.Run == savedRun {
		saved()
	} else {
		fmt.Fprintf(b, "  seed: %d\n", r.Seed)
	}
	switch {
	case r.Paste != "":
		fmt.Fprintf(b, "  to keep this case:\n    %s\n", strings.ReplaceAll(r.Paste, "\n", "\n    "))
	case len(r.Args) > 0:
		fmt.Fprintf(b, "  %s\n", pasteFallback)
	}
}

// jsonProperty is the "property" of a property test's --json line.
type jsonProperty struct {
	Runs    int       `json:"runs"`
	Seed    uint64    `json:"seed"`
	Run     int       `json:"run"`
	Shrinks int       `json:"shrinks"`
	Args    []jsonArg `json:"args"`
	Case    string    `json:"case,omitempty"`
}

type jsonArg struct {
	Name      string `json:"name"`
	Type      string `json:"type"`
	Value     string `json:"value"`
	Defaulted bool   `json:"defaulted,omitempty"`
}

// propertyJSON is r's "property" object: nil unless r is a property's result.
func propertyJSON(r CaseResult) *jsonProperty {
	if r.Runs == 0 && !r.Crash {
		return nil
	}
	args := []jsonArg{}
	for _, a := range r.Args {
		args = append(args, jsonArg{a.Name, a.Type, a.Value, a.Defaulted})
	}
	return &jsonProperty{Runs: r.Runs, Seed: r.Seed, Run: r.Run, Shrinks: r.Shrinks, Args: args, Case: r.Paste}
}
