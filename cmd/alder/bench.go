package main

import (
	"io"
	"strings"
)

// benchUsage is the usage line of `alder bench`.
const benchUsage = "usage: alder bench [dir] [-run re] [--count N] [-timeout d] [--json] [--build-dir DIR]"

// benchHelpNote follows `alder bench --help`'s usage line: what is measured, and how far to
// trust it.
const benchHelpNote = `
Runs the app's task tests as alder test does and measures each browser step (Test.get, open,
click, submit, submitRaw, fetch): ms (the median of --count runs, default 3), bytes of the
answers, allocations, requests served, page loads, effect steps, SQL statements and N+1 repeats.
Pure and property tests are skipped. The requests run in the test's transaction with a recorder
on: compare the numbers (before/after a change, N rows against 10N), do not read them as
production latencies.`

// defaultBenchCount is how many times `alder bench` runs each test unless --count says.
const defaultBenchCount = 3

// benchRun is what runTestCmd needs for `alder bench`: the runs per test (--count).
type benchRun struct{ count int }

// runBenchTo is `alder bench [dir] [-run re] [--count N] [-timeout d] [--json] [--build-dir DIR]`:
// the app's test program, built as `alder test` builds it (build/<app>/scratch/test), run with
// rtesting.EnvBench and EnvCount, so each selected task test runs --count times and its result
// is followed by one row per browser step (rtesting.Measure; with --json the test's object
// carries "bench"). --mutate, --min, --fuzz and --seed print the usage (exit 2): a mutant or a
// property's draws have nothing to measure. Exit codes are alder test's.
func runBenchTo(args []string, stdout, stderr io.Writer) int {
	return runTestCmd(args, &benchRun{}, stdout, stderr)
}

// isFlag reports whether a is the flag name, written -name, --name, -name=… or --name=….
func isFlag(a, name string) bool {
	a = strings.TrimPrefix(strings.TrimPrefix(a, "-"), "-")
	return a == name || strings.HasPrefix(a, name+"=")
}
