package main

import (
	"os"
	"testing"
)

// The mutation quality gates (TestAdmin_Columns, TestAdmin_Mutate, and the examples' recorded
// mutation scores) run every mutant of a big app: they were 60% of the suite's time
// (2026-10-18), so they run nightly in CI (.github/workflows/ci.yml, job mutation) and wherever
// ALDER_MUTATION_GATES=1 is set, not on every `go test ./...`. The tests of --mutate itself
// always run.
const mutationGatesEnv = "ALDER_MUTATION_GATES"

// mutationGates reports whether the mutation quality gates run.
func mutationGates() bool { return os.Getenv(mutationGatesEnv) == "1" }

// mutationGate skips t, a test that is a mutation quality gate and nothing else, unless the
// gates run.
func mutationGate(t *testing.T) {
	t.Helper()
	if !mutationGates() {
		t.Skip("a mutation quality gate: runs nightly in CI, or with " + mutationGatesEnv + "=1")
	}
}
