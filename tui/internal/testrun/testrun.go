// Package testrun reads the output of `alder test --json`: one JSON object per line, each a
// test's result, the check diagnostics, an error, the mutants or the mutation score. The shapes
// are alder's own (rt/testing: jsonResult, jsonProperty, mutantJSON, scoreJSON), copied here
// since that package is internal to the root module.
package testrun

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"alder/tui/internal/feed"
)

// Result is one test's result: where the test is, whether it passed, and when it failed the
// sides of the failed expectation, its note, the error and the trace; how long it took; and a
// property test's property.
type Result struct {
	Module   string    `json:"module"`
	Name     string    `json:"name"`
	File     string    `json:"file"`
	Line     int       `json:"line"`
	Col      int       `json:"col"`
	OK       bool      `json:"ok"`
	Left     string    `json:"left"`
	Right    string    `json:"right"`
	Note     string    `json:"note"`
	Error    string    `json:"error"`
	Trace    []string  `json:"trace"`
	Ms       float64   `json:"ms"`
	Property *Property `json:"property"`
}

// Property is a property test's runs, seed, failing run and shrinks, the arguments of the case
// it reports, and when it failed the case as a line of Alder.
type Property struct {
	Runs    int    `json:"runs"`
	Seed    uint64 `json:"seed"`
	Run     int    `json:"run"`
	Shrinks int    `json:"shrinks"`
	Args    []Arg  `json:"args"`
	Case    string `json:"case"`
}

// Arg is one argument of a property's case: its name, type and value, and whether the value is
// the type's default.
type Arg struct {
	Name      string `json:"name"`
	Type      string `json:"type"`
	Value     string `json:"value"`
	Defaulted bool   `json:"defaulted"`
}

// Mutant is one mutant of a mutation run: what was changed where, and whether a test killed it,
// which one, and why (a note such as "timed out" or "crashed").
type Mutant struct {
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
}

// Score is a mutation run's score, killed over the mutants that ran, and its counts.
type Score struct {
	Score      float64 `json:"score"`
	Killed     int     `json:"killed"`
	Survived   int     `json:"survived"`
	Suppressed int     `json:"suppressed"`
	Skipped    int     `json:"skipped"`
}

// Run is a run of `alder test`: its arguments, the results as they arrive, the check diagnostics
// or the error that stopped it, a mutation run's mutants and score, and once it has ended
// (Done) its exit code.
type Run struct {
	Args        []string
	Results     []Result
	Diagnostics []feed.Diag
	Error       string
	Mutants     []Mutant
	Score       *Score
	Code        int
	Done        bool
}

// kind is the keys whose presence says what a line of `alder test --json` is: a test's result
// has a module, and the other objects each have their own key.
type kind struct {
	Module      *string          `json:"module"`
	Diagnostics *json.RawMessage `json:"diagnostics"`
	Error       *string          `json:"error"`
	Mutants     *json.RawMessage `json:"mutants"`
	Score       *float64         `json:"score"`
}

// Parse reads one line of `alder test --json` into r: a test's result is appended to the
// results; {"diagnostics": […]} sets the diagnostics, {"error": …} the error, {"mutants": […]}
// the mutants, and {"score": …, "killed": …} the score. A diagnostics or mutants line whose
// list does not read sets the error instead, so a run that reported them never shows as clean. A
// blank line, or one that is not one of these objects, changes nothing.
func Parse(text string, r *Run) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	var k kind
	if json.Unmarshal([]byte(text), &k) != nil {
		return
	}
	switch {
	case k.Module != nil:
		var res Result
		if json.Unmarshal([]byte(text), &res) == nil {
			r.Results = append(r.Results, res)
		}
	case k.Diagnostics != nil:
		var ds []feed.Diag
		if err := json.Unmarshal(*k.Diagnostics, &ds); err != nil {
			r.Error = fmt.Sprintf("alder test: unreadable diagnostics: %v", err)
		} else {
			r.Diagnostics = ds
		}
	case k.Mutants != nil:
		var ms []Mutant
		if err := json.Unmarshal(*k.Mutants, &ms); err != nil {
			r.Error = fmt.Sprintf("alder test: unreadable mutants: %v", err)
		} else {
			r.Mutants = ms
		}
	case k.Score != nil:
		var sc Score
		if json.Unmarshal([]byte(text), &sc) == nil {
			r.Score = &sc
		}
	case k.Error != nil:
		r.Error = *k.Error
	}
}

// Args are the arguments of `alder test` for a run: `test --json`; with one, `-run` and the
// regexp that selects only that test (alder matches it against `Module "name"`); with mutate,
// `--mutate`.
func Args(one *Result, mutate bool) []string {
	args := []string{"test", "--json"}
	if one != nil {
		args = append(args, "-run", "^"+regexp.QuoteMeta(one.Module+" \""+one.Name+"\"")+"$")
	}
	if mutate {
		args = append(args, "--mutate")
	}
	return args
}
