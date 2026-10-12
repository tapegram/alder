package rtesting

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"alder/rt/db"
)

// Mutation testing (alder test --mutate).
//
// A mutate build (gen.Options.Mutate) compiles every mutant of the app into the test program:
// the generated package variable Mutant names the mutant the code switches on (0: none) and
// Mutants lists them. The test main registers both with SetMutants; Main with --mutate then
// runs the suite as usual and, when it passed, once per mutant: Mutant set to the mutant's id,
// a schema mutant's Tables (the app's tables with its change) created by db.Ensure instead of
// the migrations, every selected case run as usual (a fresh database each), stopping at the
// first case that fails, which kills the mutant (`by` that case). A case that panics kills it
// as crashed; a mutant whose cases are still running when its time is up is killed as timed
// out. A mutant's time is relative to the suite's: the larger of -timeout and 3 × the suite's
// own run (the baseline), so a suite near the timeout does not time out on jitter; a baseline
// over half the timeout is warned about. A mutant no case notices survives.
//
// A mutant can also end the process: a stack overflow is a Go fatal error, which no recover
// sees. So `alder test` runs the program with EnvMutateLog: Main appends each mutant's outcome
// to that file as it is decided and writes `#id\tModule\tName` to the status file (EnvStatus)
// before each case; when the program dies running a mutant, `alder test` logs that mutant as
// killed (crashed, by the case the status names) and runs the program again with
// EnvMutateFrom set to the next id, which skips the suite's own run and starts there; the run
// that finishes the last mutant reads the log and prints the report. The log's first line is the
// baseline (id 0), which the later runs read back. With the log, a timed-out
// mutant also ends the program (ExitMutantTimedOut) after logging itself, since its case is
// still running in a goroutine Main cannot stop; without the log Main leaves that goroutine
// behind and goes on.

// Mutant is one mutant of a mutate build (the generated Mutants): its id (what Mutant is set
// to), where it is (Module, Decl, the source File, Line and Col), its operator (Kind) and the code
// before and after (one line each). Tables, for a schema mutant, are the app's tables with the
// mutant's change: each case's database is made from them by db.Ensure (no migrations).
type Mutant struct {
	ID                 int
	Module, Decl, Kind string
	File               string // the module's source file, "app/Posts/Index.alder"
	Line, Col          int
	Before, After      string
	Tables             []db.TableDesc
}

// mutation is the registered mutate build (SetMutants).
var mutation struct {
	active              *int
	mutants             []Mutant
	suppressed, skipped int
}

// SetMutants registers a mutate build's mutants for Main's --mutate: active is the generated
// Mutant variable, mutants the generated Mutants, suppressed and skipped the counts of the
// mutants the compiler refused as equivalent and as not applicable.
func SetMutants(active *int, mutants []Mutant, suppressed, skipped int) {
	mutation.active, mutation.mutants = active, mutants
	mutation.suppressed, mutation.skipped = suppressed, skipped
}

// The environment of a mutation run besides Main's others (see Main).
const (
	// EnvMutate=1 is --mutate.
	EnvMutate = "ALDER_TEST_MUTATE"
	// EnvMin is --min.
	EnvMin = "ALDER_TEST_MIN"
	// EnvMutateFrom is the id of the first mutant to run (default 1); above 1 the suite's own
	// run is skipped (it passed in an earlier run of the program).
	EnvMutateFrom = "ALDER_TEST_MUTATE_FROM"
	// EnvMutateLog names the file of decided mutants (one JSON MutantOutcome per line), which
	// Main appends to and reads back for the report.
	EnvMutateLog = "ALDER_TEST_MUTATE_LOG"
)

// ExitMutantTimedOut is Main's exit code when a mutant timed out under EnvMutateLog: its
// outcome is logged, and the run goes on in a new process from the next mutant.
const ExitMutantTimedOut = 3

// defaultMutantTimeout is the per-mutant timeout when neither -timeout nor EnvTimeout says.
const defaultMutantTimeout = 10 * time.Second

// The texts of a mutation run that measures nothing.
const (
	testsFailText  = "tests fail: fix them before measuring mutants"
	noTestsText    = "no tests: nothing can kill a mutant"
	noRunMatchText = "no test matches -run: nothing can kill a mutant"
	// noTestsHint follows the summary of a plain `alder test` of an app that has no tests.
	noTestsHint = "no tests: `alder test --mutate` would score 0 (chapter 10)"
)

// baselineFactor times the suite's own run (the baseline) is the least time a mutant's run gets.
const baselineFactor = 3

// mutateTimeoutHint ends the timeout message of a suite that ran out of --mutate's default
// -timeout, which is shorter than a plain run's (5m).
const mutateTimeoutHint = " (--mutate's default 10s; raise it with -timeout)"

// TestsFailText is the line that ends a --mutate run whose tests did not pass, unmutated; alder
// test prints it too when the test program crashed in that run.
const TestsFailText = testsFailText

// MutantOutcome is a decided mutant: killed (by the case By, `Module "name"`; Note "crashed"
// or "timed out" when it was killed that way) or not. It is a line of the EnvMutateLog file;
// the line of ID 0 is no mutant's but the baseline's, BaselineMs the suite's own run.
type MutantOutcome struct {
	ID         int     `json:"id"`
	Killed     bool    `json:"killed"`
	By         string  `json:"by"`
	Note       string  `json:"note"`
	BaselineMs float64 `json:"baseline_ms,omitempty"`
}

// readBaseline reads the baseline logged in the log file path (0 when there is none).
func readBaseline(path string) time.Duration {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var o MutantOutcome
		if json.Unmarshal(sc.Bytes(), &o) == nil && o.ID == 0 && o.BaselineMs > 0 {
			return time.Duration(o.BaselineMs * float64(time.Millisecond))
		}
	}
	return 0
}

// mutantLimit is a mutant's time: the larger of the timeout and baselineFactor × the baseline.
func mutantLimit(timeout, baseline time.Duration) time.Duration {
	return max(timeout, baselineFactor*baseline)
}

// LogMutant appends o to the log file path.
func LogMutant(path string, o MutantOutcome) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644)
	if err != nil {
		return err
	}
	b, _ := json.Marshal(o)
	if _, err := f.Write(append(b, '\n')); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// ReadMutantLog reads the log file path (missing: none), by id.
func ReadMutantLog(path string) (map[int]MutantOutcome, error) {
	out := map[int]MutantOutcome{}
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return out, nil
	} else if err != nil {
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var o MutantOutcome
		if json.Unmarshal(sc.Bytes(), &o) == nil && o.ID > 0 {
			out[o.ID] = o
		}
	}
	return out, sc.Err()
}

// MutantStatus is the status-file line Main writes before running case (module, name) for
// mutant id; ParseMutantStatus reads one back.
func MutantStatus(id int, module, name string) string {
	return "#" + strconv.Itoa(id) + "\t" + module + "\t" + name + "\n"
}

// ParseMutantStatus reads a status-file line MutantStatus wrote.
func ParseMutantStatus(line string) (id int, module, name string, ok bool) {
	rest, isMutant := strings.CutPrefix(strings.TrimSuffix(line, "\n"), "#")
	if !isMutant {
		return 0, "", "", false
	}
	idText, rest, _ := strings.Cut(rest, "\t")
	module, name, ok = strings.Cut(rest, "\t")
	id, err := strconv.Atoi(idText)
	return id, module, name, ok && err == nil && id > 0
}

// mutantRun configures runMutants.
type mutantRun struct {
	timeout  time.Duration
	baseline time.Duration // the suite's own run (from the log in a later run)
	status   string        // the status file, or ""
	log      string        // EnvMutateLog, or ""
	from     int
}

// runMutants runs cases (already selected) once per registered mutant from r.from on and
// returns every mutant's outcome by id (those before r.from read from the log), and whether
// the program must stop because a mutant timed out (with a log: its goroutine is still
// running). A non-nil error is a problem of the run itself (the log or status file).
func runMutants(cases []Case, o Options, r mutantRun) (map[int]MutantOutcome, bool, error) {
	outcomes := map[int]MutantOutcome{}
	if r.log != "" {
		logged, err := ReadMutantLog(r.log)
		if err != nil {
			return nil, false, err
		}
		for id, oc := range logged {
			if id < r.from {
				outcomes[id] = oc
			}
		}
	}
	for _, m := range mutation.mutants {
		if m.ID < r.from {
			continue
		}
		oc, timedOut, err := runMutant(m, cases, o, r)
		if err != nil {
			return nil, false, err
		}
		outcomes[m.ID] = oc
		if r.log != "" {
			if err := LogMutant(r.log, oc); err != nil {
				return nil, false, err
			}
			if timedOut {
				return outcomes, true, nil
			}
		}
	}
	return outcomes, false, nil
}

// runMutant runs cases against mutant m, stopping at the first that fails.
func runMutant(m Mutant, cases []Case, o Options, r mutantRun) (oc MutantOutcome, timedOut bool, err error) {
	oc = MutantOutcome{ID: m.ID}
	*mutation.active = m.ID
	defer func() { *mutation.active = 0 }()
	if m.Tables != nil { // a schema mutant: its tables by Ensure, not the migrations
		o.Tables, o.Migrations, o.schemaErr = m.Tables, nil, ""
	}
	limit := mutantLimit(r.timeout, r.baseline)
	o.Deadline = time.Now().Add(limit)
	timer := time.NewTimer(limit)
	defer timer.Stop()
	for _, c := range cases {
		if r.status != "" {
			if err := os.WriteFile(r.status, []byte(MutantStatus(m.ID, c.Module, c.Name)), 0o644); err != nil {
				return oc, false, err
			}
		}
		done := make(chan CaseResult, 1)
		go func() { done <- runCase(c, o) }()
		res, up, finished := await(done, timer.C, o.Deadline)
		if up {
			oc.Killed, oc.By, oc.Note = true, c.Module+" "+QuoteName(c.Name), "timed out"
			return oc, !finished, nil
		}
		if res.OK {
			continue
		}
		oc.Killed, oc.By = true, c.Module+" "+QuoteName(c.Name)
		if res.panicked {
			oc.Note = "crashed"
		}
		return oc, false, nil
	}
	return oc, false, nil
}

// await waits for the running case's result (done) or the end of the mutant's time (timeUp,
// at deadline), and reports the result, whether the time is up, and whether the case finished.
// The time is decided first: a result that comes with or after the end of the time (a case
// failing by its own deadline, or both ready at once) is a timeout, not a plain kill.
func await(done <-chan CaseResult, timeUp <-chan time.Time, deadline time.Time) (res CaseResult, up, finished bool) {
	select {
	case res = <-done:
		select {
		case <-timeUp:
			return res, true, true
		default:
		}
		return res, !time.Now().Before(deadline), true
	case <-timeUp:
		select {
		case res = <-done:
			return res, true, true
		default:
			return CaseResult{}, true, false
		}
	}
}

// mutantJSON is a mutant in the --json report.
type mutantJSON struct {
	ID     int    `json:"id"`
	Module string `json:"module"`
	File   string `json:"file"`
	Decl   string `json:"decl"`
	Line   int    `json:"line"`
	Col    int    `json:"col"`
	Kind   string `json:"kind"`
	Before string `json:"before"`
	After  string `json:"after"`
	Killed bool   `json:"killed"`
	By     string `json:"by"`
	Note   string `json:"note"`
	// Tables is "ensure" for a schema mutant: its tables were made by db.Ensure, not the
	// app's migrations
	Tables string `json:"tables,omitempty"`
}

// scoreJSON is the --json report's last object.
type scoreJSON struct {
	Score      float64 `json:"score"`
	Killed     int     `json:"killed"`
	Survived   int     `json:"survived"`
	Suppressed int     `json:"suppressed"`
	Skipped    int     `json:"skipped"`
}

// score is killed / total over the mutants that ran; 1 when none did (nothing survived).
func score(killed, total int) float64 {
	if total == 0 {
		return 1
	}
	return float64(killed) / float64(total)
}

// survivorWidth elides an arm-swapped survivor's line (a whole match) to this many columns.
const survivorWidth = 120

// survivorLine is survivor m's report line after its columns (cols): `before → after`, an arm
// swap's elided at survivorWidth by shortening its before (the whole match), so the after (which
// names the swap) stays whole; and from its end when that is not enough.
func survivorLine(cols string, m Mutant) string {
	line := cols + m.Before + " → " + m.After
	n := utf8.RuneCountInString(line)
	if m.Kind != "arm swapped" || n <= survivorWidth {
		return line
	}
	if keep := utf8.RuneCountInString(m.Before) - (n - survivorWidth) - 1; keep >= 20 {
		return cols + string([]rune(m.Before)[:keep]) + "… → " + m.After
	}
	return string([]rune(line)[:survivorWidth-1]) + "…"
}

// writeMutationReport writes the mutation report of outcomes and returns the score: as text
//
//	N mutants: K killed, S survived  (score 0.94)
//	survived:
//	  Module   Decl:Line   kind   before → after
//
// (`K killed (of which T timed out)` when some timed out; the survived lines only when some
// did, their columns aligned, `Decl:Line:Col` when the line has several mutants of the kind, an
// arm swap's elided at survivorWidth), or with asJSON the object {"mutants": […]} and then
// {"score", "killed", "survived", "suppressed", "skipped"}.
func writeMutationReport(out io.Writer, outcomes map[int]MutantOutcome, asJSON bool) (float64, error) {
	killed, total, timedOut := 0, 0, 0
	var ms []mutantJSON
	var survivors []Mutant
	for _, m := range mutation.mutants {
		oc, ok := outcomes[m.ID]
		if !ok {
			continue
		}
		total++
		if oc.Killed {
			killed++
			if oc.Note == "timed out" {
				timedOut++
			}
		} else {
			survivors = append(survivors, m)
		}
		j := mutantJSON{ID: m.ID, Module: m.Module, File: m.File, Decl: m.Decl, Line: m.Line, Col: m.Col, Kind: m.Kind, Before: m.Before, After: m.After,
			Killed: oc.Killed, By: oc.By, Note: oc.Note}
		if m.Tables != nil {
			j.Tables = "ensure"
		}
		ms = append(ms, j)
	}
	sc := score(killed, total)
	if asJSON {
		if ms == nil {
			ms = []mutantJSON{}
		}
		enc := json.NewEncoder(out)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(struct {
			Mutants []mutantJSON `json:"mutants"`
		}{ms}); err != nil {
			return sc, err
		}
		return sc, enc.Encode(scoreJSON{sc, killed, total - killed, mutation.suppressed, mutation.skipped})
	}
	var b strings.Builder
	timeouts := ""
	if timedOut > 0 {
		timeouts = fmt.Sprintf(" (of which %d timed out)", timedOut)
	}
	fmt.Fprintf(&b, "%d mutants: %d killed%s, %d survived  (score %.2f)\n", total, killed, timeouts, total-killed, sc)
	if len(survivors) > 0 {
		// a line with mutants of one kind at several columns names each by its column too
		type at struct {
			module, kind string
			line         int
		}
		cols := map[at]map[int]bool{}
		for _, m := range mutation.mutants {
			k := at{m.Module, m.Kind, m.Line}
			if cols[k] == nil {
				cols[k] = map[int]bool{}
			}
			cols[k][m.Col] = true
		}
		b.WriteString("survived:\n")
		rows := make([][3]string, len(survivors))
		var width [3]int
		for i, m := range survivors {
			where := m.Decl + ":" + strconv.Itoa(m.Line)
			if len(cols[at{m.Module, m.Kind, m.Line}]) > 1 && m.Col > 0 {
				where += ":" + strconv.Itoa(m.Col)
			}
			rows[i] = [3]string{m.Module, where, m.Kind}
			for k, c := range rows[i] {
				width[k] = max(width[k], utf8.RuneCountInString(c))
			}
		}
		for i, m := range survivors {
			var l strings.Builder
			l.WriteString("  ")
			for k, c := range rows[i] {
				l.WriteString(c + strings.Repeat(" ", width[k]-utf8.RuneCountInString(c)+3))
			}
			b.WriteString(survivorLine(l.String(), m) + "\n")
		}
	}
	_, err := io.WriteString(out, b.String())
	return sc, err
}
