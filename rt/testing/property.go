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
	"strings"
	"time"

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
			r.Run = runs
			r.Error = fmt.Sprintf("run %d took %s, over the %s one case may take", runs, a.dur.Round(time.Millisecond), attemptLimit)
			r.Args = argValues(p, a.vals)
			return true
		}
		if a.failed() {
			r.Run, failing = runs, &a
			return true
		}
		return false
	}
	if seq, ok := readSaved(path); ok {
		if a := runAttempt(p, o, fuzz.Replay(seq)); !a.overrun && !a.rejected && ran(a) {
			return finish(c, o, r, failing, path)
		}
	}
	rejects := map[string]int{}
	for i := 0; runs < o.Fuzz; i++ {
		if i == healthAttempts || i == attemptsPerRun*o.Fuzz {
			if msg := filterTooTight(rejects, i); msg != "" {
				r.Error = msg
				return r
			}
			if i == attemptsPerRun*o.Fuzz {
				break // a pass on the runs made: the predicates accept enough draws, rarely
			}
		}
		a := runAttempt(p, o, fuzz.Fresh(attemptSeed(r.Seed, i)))
		switch {
		case a.overrun:
			r.Run = runs + 1
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
		best, steps := fuzz.Shrink(failing.seq, func(seq []uint64) bool {
			a := runAttempt(p, o, fuzz.Replay(seq))
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
	if p.Case != nil && final.vals != nil {
		r.Paste = p.Case(literals(p, final.vals))
	}
	if path != "" {
		if err := save(path, final.seq); err != nil {
			r.Error = strings.TrimPrefix(r.Error+"\n", "\n") + "could not save the failing case: " + err.Error()
		}
	}
	return r
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
	rand.Read(b[:])
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
	s := fmt.Sprintf("  (failed on run %d of %d", r.Run, r.Runs)
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
func writeSeed(b *strings.Builder, r CaseResult) {
	if r.Runs == 0 {
		return
	}
	fmt.Fprintf(b, "  seed: %d\n", r.Seed)
	if r.Paste != "" {
		fmt.Fprintf(b, "  to keep this case:\n    %s\n", strings.ReplaceAll(r.Paste, "\n", "\n    "))
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
	if r.Runs == 0 {
		return nil
	}
	args := []jsonArg{}
	for _, a := range r.Args {
		args = append(args, jsonArg{a.Name, a.Type, a.Value, a.Defaulted})
	}
	return &jsonProperty{Runs: r.Runs, Seed: r.Seed, Run: r.Run, Shrinks: r.Shrinks, Args: args, Case: r.Paste}
}
