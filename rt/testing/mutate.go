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
// as crashed; a mutant whose cases are still running when the timeout (-timeout, the suite's
// per-run timeout) passes is killed as timed out. A mutant no case notices survives.
//
// A mutant can also end the process: a stack overflow is a Go fatal error, which no recover
// sees. So `alder test` runs the program with EnvMutateLog: Main appends each mutant's outcome
// to that file as it is decided and writes `#id\tModule\tName` to the status file (EnvStatus)
// before each case; when the program dies running a mutant, `alder test` logs that mutant as
// killed (crashed, by the case the status names) and runs the program again with
// EnvMutateFrom set to the next id, which skips the suite's own run and starts there; the run
// that finishes the last mutant reads the log and prints the report. With the log, a timed-out
// mutant also ends the program (ExitMutantTimedOut) after logging itself, since its case is
// still running in a goroutine Main cannot stop; without the log Main leaves that goroutine
// behind and goes on.

// Mutant is one mutant of a mutate build (the generated Mutants): its id (what Mutant is set
// to), where it is (Module, Decl and the source Line), its operator (Kind) and the code
// before and after (one line each). Tables, for a schema mutant, are the app's tables with the
// mutant's change: each case's database is made from them by db.Ensure (no migrations).
type Mutant struct {
	ID                 int
	Module, Decl, Kind string
	Line               int
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
	testsFailText = "tests fail: fix them before measuring mutants"
	noTestsText   = "no tests: nothing can kill a mutant"
)

// MutantOutcome is a decided mutant: killed (by the case By, `Module "name"`; Note "crashed"
// or "timed out" when it was killed that way) or not. It is a line of the EnvMutateLog file.
type MutantOutcome struct {
	ID     int    `json:"id"`
	Killed bool   `json:"killed"`
	By     string `json:"by"`
	Note   string `json:"note"`
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
	timeout time.Duration
	status  string // the status file, or ""
	log     string // EnvMutateLog, or ""
	from    int
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
	o.Deadline = time.Now().Add(r.timeout)
	timer := time.NewTimer(r.timeout)
	defer timer.Stop()
	for _, c := range cases {
		if r.status != "" {
			if err := os.WriteFile(r.status, []byte(MutantStatus(m.ID, c.Module, c.Name)), 0o644); err != nil {
				return oc, false, err
			}
		}
		done := make(chan CaseResult, 1)
		go func() { done <- runCase(c, o) }()
		select {
		case res := <-done:
			if res.OK {
				continue
			}
			oc.Killed, oc.By = true, c.Module+" "+QuoteName(c.Name)
			if res.panicked {
				oc.Note = "crashed"
			}
			return oc, false, nil
		case <-timer.C:
			oc.Killed, oc.By, oc.Note = true, c.Module+" "+QuoteName(c.Name), "timed out"
			return oc, true, nil
		}
	}
	return oc, false, nil
}

// mutantJSON is a mutant in the --json report.
type mutantJSON struct {
	ID     int    `json:"id"`
	Module string `json:"module"`
	Decl   string `json:"decl"`
	Line   int    `json:"line"`
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

// writeMutationReport writes the mutation report of outcomes and returns the score: as text
//
//	N mutants: K killed, S survived  (score 0.94)
//	survived:
//	  Module   Decl:Line   kind   before → after
//
// (the survived lines only when some did, their columns aligned), or with asJSON the object
// {"mutants": […]} and then {"score", "killed", "survived", "suppressed", "skipped"}.
func writeMutationReport(out io.Writer, outcomes map[int]MutantOutcome, asJSON bool) (float64, error) {
	killed, total := 0, 0
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
		} else {
			survivors = append(survivors, m)
		}
		j := mutantJSON{ID: m.ID, Module: m.Module, Decl: m.Decl, Line: m.Line, Kind: m.Kind, Before: m.Before, After: m.After,
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
	fmt.Fprintf(&b, "%d mutants: %d killed, %d survived  (score %.2f)\n", total, killed, total-killed, sc)
	if len(survivors) > 0 {
		b.WriteString("survived:\n")
		rows := make([][3]string, len(survivors))
		var width [3]int
		for i, m := range survivors {
			rows[i] = [3]string{m.Module, m.Decl + ":" + strconv.Itoa(m.Line), m.Kind}
			for k, c := range rows[i] {
				width[k] = max(width[k], utf8.RuneCountInString(c))
			}
		}
		for i, m := range survivors {
			b.WriteString("  ")
			for k, c := range rows[i] {
				b.WriteString(c + strings.Repeat(" ", width[k]-utf8.RuneCountInString(c)+3))
			}
			b.WriteString(m.Before + " → " + m.After + "\n")
		}
	}
	_, err := io.WriteString(out, b.String())
	return sc, err
}
