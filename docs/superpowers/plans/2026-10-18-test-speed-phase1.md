# Test speed, Phase 1: implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Cut the local suite from ~1150 s to a few minutes and CI from ~29 min (often red by timeout) to ~10 min, by running each mutant's own module's cases first, moving the mutation quality gates to a nightly job, adding a fast local loop, and fixing the CI workflow.

**Architecture:** One runtime change (case order in `rt/testing/mutate.go`). Test-only changes in `cmd/alder`: the mutation gates become opt-in through `ALDER_MUTATION_GATES=1`, and slow deploy tests skip under `-short`. A local parallel runner (`scripts/test.sh`) on top of `.github/shard.sh`. CI changes in `.github/workflows/ci.yml`: a change filter, equal timeouts, 4 shards, and a nightly `mutation` job.

**Tech Stack:** Go 1.27, bash (macOS bash 3.2 compatible), GitHub Actions.

**Spec:** the measurements in `/Users/tavishpegram/projects/alder/.superpowers/sdd/test-speed/local-report.md` (local recs #1–#3) and `.../ci-report.md` (CI recs 1, 2, 4 (the weights part), 5, 6, 7, 8, and the main-branch concurrency note). User decisions (2026-10-18): the mutation gates go to **nightly**. Scope is the list above. Deferred: scoped `--changed` mutation, splitting TestAdmin_Columns, the scratch package split, making cmd/alder parallel-safe.

## Global Constraints

- No agent or background work 9am–5pm America/New_York on weekdays. (Today, 2026-10-18, is a Sunday.)
- Never run two full suites at once, and never profile while another gate runs.
- A full local `go test ./...` needs `-timeout 30m`.
- Implementers run **focused** tests per task (`go test ./rt/testing -run X`, `go test ./cmd/alder -run '^TestX$'`). The full gate runs once, in Task 6.
- Comment style: match the surrounding code. Dense, explanatory prose comments, no TODOs.
- The repo uses no pull requests, so CI must keep running on plain branch pushes.
- Commit messages: `area: summary` (e.g. `rt/testing: …`, `ci: …`), ending with the attribution lines from the session.

## Review Focus

1. **A mutant whose own module has no cases.** Example: a helper module tested only from another module. It must still be killed by the other modules' cases, and the scores and mutant counts the examples record (todos 208 / 0.99, blog 323 / 0.98, workspace 522 / 0.97) must not change. Pinned by the Task 1 unit test and the Task 6 nightly-gate run.
2. **`Mutant.Module` and `Case.Module` in different formats** (for example a file path vs a dotted module name). The reorder would then silently do nothing. Task 1 Step 6 measures `--mutate` on todos and expects about 1 s instead of about 8 s.
3. **Crash and timeout restart** (`EnvMutateFrom`, the status file, `MutantStatus(m.ID, c.Module, c.Name)`). These must still name the running case correctly under the new order. Task 1 keeps TestTest_MutateCrashAndHang passing, with only `by` values changed.
4. **`scripts/test.sh` reporting success when a part failed.** Example: a shard exits non-zero, or flags such as `-short` / `-run` are not passed through. Task 4 Step 4 injects a failure.
5. **The CI change filter on events with no diff** (schedule, workflow_dispatch, tags, a new branch's first push). It must report "code changed", or the nightly job never runs. Task 5 encodes it, and Task 6 checks it on the real run.

---

### Task 1: Run the mutant's own module's cases first

**Files:**
- Modify: `rt/testing/mutate.go` (`runMutant`, about line 245)
- Test: `rt/testing/mutate_test.go`
- Update goldens: `cmd/alder/test_test.go` (about lines 1014-1016, `by` assertions) and the TestTest_MutateCrashAndHang JSON expectations. Also update any other golden that asserts a `by` / "killed by" value: grep for `"by"` and `killed by` under `cmd/alder`, `internal/gen/testdata`, `book/`.

**Interfaces:** Produces `func ownModuleFirst(cases []Case, module string) []Case` (unexported, rt/testing).

- [ ] **Step 1: Check the module formats.** In `rt/testing`, confirm that `Mutant.Module` and `Case.Module` hold the same form of module name (e.g. `Posts.Index`). Read where both are set: `Mutant` registration in the generated code (internal/gen/mutation.go) and `Case`. If they differ, normalise in `ownModuleFirst` and say so in its comment.

- [ ] **Step 2: Write the failing test** in `rt/testing/mutate_test.go`, following that file's style:

```go
func TestOwnModuleFirst(t *testing.T) {
	cases := []Case{{Module: "A", Name: "a1"}, {Module: "B", Name: "b1"}, {Module: "A", Name: "a2"}, {Module: "B", Name: "b2"}}
	var got []string
	for _, c := range ownModuleFirst(cases, "B") {
		got = append(got, c.Module+"."+c.Name)
	}
	if want := "B.b1 B.b2 A.a1 A.a2"; strings.Join(got, " ") != want {
		t.Errorf("ownModuleFirst(…, B) = %v, want %s", got, want)
	}
	// a module with no cases: the suite order, every case kept
	got = nil
	for _, c := range ownModuleFirst(cases, "C") {
		got = append(got, c.Module+"."+c.Name)
	}
	if want := "A.a1 B.b1 A.a2 B.b2"; strings.Join(got, " ") != want {
		t.Errorf("ownModuleFirst(…, C) = %v, want %s", got, want)
	}
}
```

Adjust the `Case` literal to that struct's real fields. If Step 1 found that a normalisation is needed, add a case for it.

- [ ] **Step 3: Run it and see it fail.** `go test ./rt/testing -run TestOwnModuleFirst -count=1` should FAIL: `undefined: ownModuleFirst`.

- [ ] **Step 4: Implement.** In `rt/testing/mutate.go`, next to `runMutant`:

```go
// ownModuleFirst returns cases with module's own first, each part in suite order. The case that
// kills a mutant is in the mutant's own module for 95% of mutants (measured on the examples,
// 2026-10-18), and it sat halfway down the suite on average, so this makes a killed mutant stop
// about 8.5× sooner. A survivor still runs every case, and each case starts from a fresh
// database, so whether a mutant is killed does not depend on the order: only which case is
// named as its killer (By) can change.
func ownModuleFirst(cases []Case, module string) []Case {
	out := make([]Case, 0, len(cases))
	for _, c := range cases {
		if c.Module == module {
			out = append(out, c)
		}
	}
	for _, c := range cases {
		if c.Module != module {
			out = append(out, c)
		}
	}
	return out
}
```

In `runMutant`, change `for _, c := range cases {` to `for _, c := range ownModuleFirst(cases, m.Module) {`. Update the doc comment of `runMutant` ("runs cases against mutant m, its own module's first, stopping at the first that fails").

- [ ] **Step 5: Run the rt/testing tests.** `go test ./rt/testing -count=1` should PASS.

- [ ] **Step 6: Measure the speedup.** Build alder (`go build -o /tmp/… ./cmd/alder` into your scratch dir). Copy `examples/todos` to a temp dir and run `time alder test --mutate <copy>`. Expect about 1 s (it was 7.4–8.8 s), with the score line still showing 208 mutants, all killed. If it is still about 8 s, the formats from Step 1 differ: go back to Step 1.

- [ ] **Step 7: Update the goldens.** Run `go test ./cmd/alder -run '^TestTest_Mutate' -count=1` and `go test ./internal/gen -run Mutate -count=1`. For each failure, check that only a `by` value (or the order of a listing derived from it) changed, never killed / survived / score / count, then update the expectation. Re-run until green. Also run `go test ./book -count=1` if `book/` mentions a killer.

- [ ] **Step 8: Mention it in the book.** If book chapter 10 (testing) says how `--mutate` runs the cases (grep `book/10-testing.md` for "mutant" and "order" / "first"), add one sentence there: own module's cases first, and why. Then `go test ./book -count=1`.

- [ ] **Step 9: Commit.** `git add -A rt/testing cmd/alder internal book && git commit -m "rt/testing: run a mutant's own module's cases first (~8.5× faster --mutate)"`

---

### Task 2: The mutation quality gates become opt-in (`ALDER_MUTATION_GATES=1`)

**Files:**
- Create: `cmd/alder/gates_test.go`
- Modify: `cmd/alder/admin_test.go` (TestAdmin_Mutate :165, TestAdmin_Columns :523), `cmd/alder/example_test.go` (`mutationAtLeast` :205, TestExample_TodosMutation :229)

**Interfaces:** Produces `func mutationGates() bool` and `func mutationGate(t *testing.T)` (cmd/alder tests). Task 5's nightly CI job sets `ALDER_MUTATION_GATES=1` and runs `-run '^(TestAdmin_Columns|TestAdmin_Mutate|TestExample_Blog|TestExample_Workspace|TestExample_TodosMutation)$'`.

Scope: only the quality gates on big inputs. Tests **of** `--mutate` (TestTest_Mutate*, TestTest_Rules), the small gates already skipped under `-short` (TestDrill, TestGenerate_Mutate, TestNew_OutOfTree, about 10 s in total), and the deploy gate (it is the deploy feature under test) are not touched.

- [ ] **Step 1: Write the helper,** `cmd/alder/gates_test.go`:

```go
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
```

- [ ] **Step 2: Use it.** In TestAdmin_Mutate and TestAdmin_Columns, call `mutationGate(t)` right after `needGoTool(t)` and keep their `testing.Short()` skip. In TestExample_TodosMutation, call `mutationGate(t)` first. `mutationAtLeast` sits at the end of TestExample_Blog and TestExample_Workspace, which test more than mutation, so it must **not** skip the whole test. Start it with:

```go
	if !mutationGates() {
		t.Logf("alder test --mutate %s: skipped, a mutation quality gate (%s=1 runs it)", dir, mutationGatesEnv)
		return
	}
```

Check that nothing in TestExample_Workspace runs after its `mutationAtLeast` call (example_test.go:406). If something does, keep that behaviour. Update the comments above `mutationAtLeast`, the score constants and TestExample_TodosMutation to say that the gates run nightly.

- [ ] **Step 3: Verify skipped and run.** Run `go test ./cmd/alder -run '^(TestAdmin_Columns|TestAdmin_Mutate|TestExample_TodosMutation|TestExample_Blog)$' -count=1 -v 2>&1 | grep -E '^(=== RUN|--- |ok|FAIL)|mutation quality gate'`. Expect SKIP for Columns, Mutate and TodosMutation, and PASS for Blog with the log line, in well under a minute. Then `ALDER_MUTATION_GATES=1 go test ./cmd/alder -run '^(TestExample_TodosMutation|TestExample_Blog)$' -count=1`: PASS. With Task 1 in place, the mutate part should take a few seconds.

- [ ] **Step 4: Commit.** `git commit -am "cmd/alder: the mutation quality gates run nightly (ALDER_MUTATION_GATES=1), not on every go test"`. Add `gates_test.go` first.

---

### Task 3: Fix the TestDeploySsh_Lifecycle prune / rollback flake

**Files:** `cmd/alder/deploy_ssh_test.go` (about lines 505-520), and whatever the root cause is in (likely `internal/deploy`'s prune ordering).

Failure seen on CI (run 39139251000, shard 2/3): `alder rollback: refused: there is no build c6669b07... (alder status lists them)` at deploy_ssh_test.go:518, right after `prune ok removed c6669b07 (keep 3)`. The test expects prune to remove `id1b`, "the oldest by its deploy (id1's record was rewritten by the rollback)". Instead it removed id1. It passed in every other run.

- [ ] **Step 1: Use superpowers:systematic-debugging.** Read how prune chooses what to remove (search `internal/deploy` and `cmd/alder/deploy*.go` for "keep" / prune). Leading hypothesis: builds are ordered by a deploy timestamp of coarse resolution (seconds, or a file mtime), and id1's rewritten record ties with id1b, with ties broken arbitrarily. Confirm or reject it from the code. If it is real, write a deterministic test that reproduces it: equal timestamps, at the unit level in the package that owns prune.
- [ ] **Step 2: Fix the root cause.** If prune itself is non-deterministic on ties, that is a product bug. Make the order total: break ties by a second key that matches the intended "oldest by its deploy" semantics. Document the rule in the comment. If instead the test's assumption is wrong, fix the test so it does not depend on timing. Do not add sleeps.
- [ ] **Step 3: Verify.** Run the new unit test, then `go test ./cmd/alder -run '^TestDeploySsh_Lifecycle$' -count=5`: PASS ×5.
- [ ] **Step 4: Commit.** `deploy: <what>` or `cmd/alder: <what>`, with a message naming the cause.

---

### Task 4: A fast local loop. `-short` skips the slow deploy tests; `scripts/test.sh` runs the suite in parallel; refreshed shard weights

**Files:**
- Modify: `cmd/alder/deploy_ssh_test.go`, `deploy_render_test.go`, `deploy_turso_test.go`, `deploy_test.go`, `deploy_silos_test.go`, `deploy_caddy_test.go` (whichever hold the tests named below)
- Modify: `.github/shard.sh` (pass extra `go test` flags), `.github/shard-weights` (re-measured)
- Create: `scripts/test.sh`

- [ ] **Step 1: The -short skips.** Each cmd/alder deploy test that took ≥ 5 s in the 2026-10-18 measurement gets this at its top (after any existing `needGoTool`):

```go
	if testing.Short() {
		t.Skip("a deploy end to end: real builds, app processes and drain windows")
	}
```

Tests: TestDeploySsh_Lifecycle, _DrainRollback, _CaddyDrainRestore, _Silos, _RestoreBackup, _DropsDataAutoRestore, _BackupRestore; TestDeployRender_EndToEnd, TestDeployRender_DefaultImage; TestDeployTurso_Lifecycle; TestDeploy_ScriptDropsData; TestDeploy_GateRefusals. The `deploy` CI job (Docker, `-tags deploy`) does not pass `-short`, so it is unaffected.

- [ ] **Step 2: Let shard.sh pass flags through.** In `.github/shard.sh`, change the usage lines to `.github/shard.sh N K [pkg [go test flags...]]`. After `n=$1 k=$2 pkg=${3:-./cmd/alder}`, add `shift $(( $# < 3 ? $# : 3 ))`, and end with `exec go test "$pkg" -count=1 -timeout 40m -run "$p" "$@"`. A later `-run` or `-timeout` in "$@" overrides the earlier one: go's flag package keeps the last value. Note that in the usage comment.

- [ ] **Step 3: Write `scripts/test.sh`** and `chmod +x` it:

```bash
#!/usr/bin/env bash
# The whole Go suite at once, in parallel: cmd/alder in shards (.github/shard.sh, by the weights in
# .github/shard-weights) and every other package in one go test, each part logging to a file.
# Prints each part's result and time, and the tail of each failing part's log.
#
#   scripts/test.sh [go test flags...]    e.g. scripts/test.sh -short   (the quick local loop)
#
# ALDER_TEST_SHARDS (default 4) sets the number of cmd/alder shards. More is slower on a laptop:
# each shard's builds already use every core (6 shards at once took 665 s, 4 about 200 s).
# The mutation quality gates run only with ALDER_MUTATION_GATES=1 (cmd/alder/gates_test.go).
set -uo pipefail
cd "$(dirname "$0")/.."
n=${ALDER_TEST_SHARDS:-4}
logs=$(mktemp -d "${TMPDIR:-/tmp}/alder-test.XXXXXX")
start=$SECONDS

part() { # part NAME CMD...: runs CMD into $logs/NAME.log, then writes "exit seconds" to $logs/NAME.res
  local name=$1 s=$SECONDS; shift
  "$@" > "$logs/$name.log" 2>&1
  echo "$? $((SECONDS - s))" > "$logs/$name.res"
}

names=()
for k in $(seq 0 $((n - 1))); do
  part "cmd-alder-$k" .github/shard.sh "$n" "$k" ./cmd/alder "$@" &
  names+=("cmd-alder-$k")
done
part rest go test $(go list ./... | grep -v '^alder/cmd/alder$') -count=1 -timeout 30m "$@" &
names+=(rest)
wait

failed=0
for name in "${names[@]}"; do
  read -r code secs < "$logs/$name.res"
  if [ "$code" = 0 ]; then echo "ok    $name  ${secs}s"; else echo "FAIL  $name  ${secs}s  ($logs/$name.log)"; failed=1; fi
done
for name in "${names[@]}"; do
  read -r code _ < "$logs/$name.res"
  [ "$code" = 0 ] || { echo; echo "--- $name (last 40 lines)"; tail -n 40 "$logs/$name.log"; }
done
echo "$((SECONDS - start))s in all; logs in $logs"
exit $failed
```

- [ ] **Step 4: Check that a failure is reported.** Temporarily add `func TestZZFail(t *testing.T) { t.Fatal("boom") }` to `cmd/alder/gates_test.go`. Run `scripts/test.sh -short -run '^(TestZZFail|TestHelp.*)$'`: exit status 1, a `FAIL cmd-alder-k` line, and the log tail showing "boom". Remove the test, run the same command again: exit 0. This also checks that the flags are passed through: the run is quick.

- [ ] **Step 5: Re-measure the weights.** Nothing else may be running. Run `go test ./cmd/alder -count=1 -timeout 30m -json > $SCRATCH/cmd-alder.json`. The gates now skip; expect about 400–450 s, which is the last full serial run this plan needs. Regenerate `.github/shard-weights` from it: every top-level test with Elapsed ≥ 2 s, as `<rounded seconds> <name>`, heaviest first. For parallel groups (TestGenerate, TestDrill), whose parent reports Elapsed 0, take the wall time from their subtests' first-start and last-end. Rewrite the header comment: measured 2026-10-18 on a laptop with the mutation gates off (they run nightly, alone, in their own job). Also list the default `-short` skips if useful. Keep the "TestAdmin_Columns gets a shard to itself" sentence only if it is still true. Run `.github/shard.sh 4 check`: "N tests of ./cmd/alder, each in exactly one of 4 shards".

- [ ] **Step 6: Time the loops.** Run `time scripts/test.sh -short`, then `time scripts/test.sh` (one after the other, never at once). Both must be green. Record the two times and each part's seconds in the commit message. Expected: about 1–2 min and about 3–5 min.

- [ ] **Step 7: Commit.** `git add scripts/test.sh .github/shard.sh .github/shard-weights cmd/alder && git commit -m "test: scripts/test.sh runs the suite in parallel; -short skips the deploy end to ends; shard weights re-measured"`

---

### Task 5: CI. Equal timeouts, 4 shards, a change filter, no duplicate runs, deploy jobs only when needed, nightly mutation gates

**Files:** `.github/workflows/ci.yml`, `.github/shard.sh` (its header comment about shard 0's timeout), and the book's description of CI if it has one (grep `book/` for `ci.yml`, "shard", "timeout-minutes").

- [ ] **Step 1: Triggers and concurrency.**

```yaml
on:
  push:
  pull_request:
  schedule:
    - cron: "0 7 * * *" # 03:00 New York (EDT): the nightly run, with the mutation gates
  workflow_dispatch:

concurrency:
  group: ci-${{ github.ref }}
  # a branch's new push replaces its running CI; on main every commit gets its full run
  cancel-in-progress: ${{ github.ref != 'refs/heads/main' }}
```

- [ ] **Step 2: The `changes` job,** first in `jobs:`. Every other job, `grammar` included, gets `needs: changes` added (warm: `needs: changes`; the others: `needs: [changes, warm]`). It also stops a pull request from this repository from running CI a second time: its branch's push already ran it, while forks' pull requests still run.

```yaml
  # What the push changed, so a docs-only push does not run the 30-minute suite. Pull requests
  # from this repository are skipped here (and so everything after): the branch's push runs CI.
  # A scheduled, manual or tag run has no diff and counts as changing everything.
  changes:
    if: github.event_name != 'pull_request' || github.event.pull_request.head.repo.full_name != github.repository
    runs-on: ubuntu-latest
    timeout-minutes: 5
    outputs:
      code: ${{ steps.all.outputs.all == 'true' || steps.filter.outputs.code == 'true' }}
      book: ${{ steps.all.outputs.all == 'true' || steps.filter.outputs.book == 'true' }}
      deploy: ${{ steps.all.outputs.all == 'true' || github.ref == 'refs/heads/main' || steps.filter.outputs.deploy == 'true' }}
    steps:
      - uses: actions/checkout@v4
      - id: all
        run: echo "all=${{ github.event_name == 'schedule' || github.event_name == 'workflow_dispatch' || startsWith(github.ref, 'refs/tags/') }}" >> "$GITHUB_OUTPUT"
      - id: filter
        if: steps.all.outputs.all != 'true'
        uses: dorny/paths-filter@v3
        with:
          predicate-quantifier: every
          filters: |
            code:
              - '**'
              - '!docs/**'
              - '!book/**'
              - '!.superpowers/**'
              - '!**/*.md'
              - '!LICENSE'
            book:
              - 'book/**'
            deploy:
              - 'cmd/alder/deploy*'
              - 'cmd/alder/testdata/deploy/**'
              - 'internal/deploy/**'
              - 'rt/backup/**'
              - 'Dockerfile'
              - '.github/workflows/ci.yml'
```

Check that `!**/*.md` does not hide code that is tested: examples' READMEs record mutation scores, but the numbers live in example_test.go, so a README-only change needs no tests. If you find a `.md` file that a Go test reads (grep for `.md"` in `*_test.go`), add its path to `code`. The book verifier is why `book` exists.

- [ ] **Step 3: Gate the jobs.**
  - `warm` and `lint`: `if: needs.changes.outputs.code == 'true' || needs.changes.outputs.book == 'true'` (lint runs book/site/test_site.py).
  - `grammar`: `if: needs.changes.outputs.code == 'true'`.
  - `test`: job `if: needs.changes.outputs.code == 'true' || needs.changes.outputs.book == 'true'`. Its cmd/alder and internal/gen steps get `&& needs.changes.outputs.code == 'true'` on their existing step `if:` (job-level `if` cannot read `matrix`). `rest` (which holds `book`) always runs inside the job.
  - `examples`: `needs.changes.outputs.code == 'true'`.
  - `deploy`, `deploy-systemd`: `if: needs.changes.outputs.deploy == 'true'` (main, tags, nightly, manual, or a deploy path changed). Note this in their header comments.

- [ ] **Step 4: Four shards, one timeout.** Matrix `part: [cmd/alder 0/4, cmd/alder 1/4, cmd/alder 2/4, cmd/alder 3/4, internal/gen, rest]`; `timeout-minutes: 30` for every part. Replace the comment that explains shard 0's 45 minutes with: CI runners are about 2–4× slower than the laptop the weights come from, and the mutation gates are no longer in these shards. Shard parsing: `k=${k%/4}`, the check step `.github/shard.sh 4 check` with `if: matrix.part == 'cmd/alder 0/4'`. In `.github/shard.sh`'s header, drop "(ci.yml gives that job the longer timeout)".

- [ ] **Step 5: The nightly `mutation` job.**

```yaml
  # The mutation quality gates (cmd/alder/gates_test.go): every mutant of the admin fixtures and
  # the example apps, against the scores and mutant counts the examples record. Nightly, on a
  # manual run and on tags: they are too slow for every push.
  mutation:
    needs: [changes, warm]
    if: github.event_name == 'schedule' || github.event_name == 'workflow_dispatch' || startsWith(github.ref, 'refs/tags/')
    runs-on: ubuntu-latest
    timeout-minutes: 45
    steps:
      # checkout, setup-go, cache restore: the same three steps as the other jobs
      - name: go test the mutation gates
        env:
          ALDER_MUTATION_GATES: "1"
        run: go test ./cmd/alder -run '^(TestAdmin_Columns|TestAdmin_Mutate|TestExample_Blog|TestExample_Workspace|TestExample_TodosMutation)$' -count=1 -timeout 40m -v
```

Copy the three setup steps verbatim from `lint`.

- [ ] **Step 6: Update the header comment** of ci.yml: the change filter, nightly, 4 shards, deploy jobs when deploy paths change, and pull requests from this repo deduplicated. Update the book's CI description if the grep under Files found one, then `go test ./book -count=1`.

- [ ] **Step 7: Lint the workflow.** `go run github.com/rhysd/actionlint/cmd/actionlint@latest .github/workflows/ci.yml`: no errors. (The tool is not added to go.mod. If `go run` would change go.mod, run it from your scratch dir with `-C`, or with `GOFLAGS=-mod=mod` in a temp module.)

- [ ] **Step 8: Commit.** `git commit -am "ci: change filter, 4 shards with one timeout, nightly mutation gates, no duplicate PR runs, full runs on main"`

---

### Task 6: Verify end to end and hand over

- [ ] **Step 1: The nightly gates locally.** Run `ALDER_MUTATION_GATES=1 go test ./cmd/alder -run '^(TestAdmin_Columns|TestAdmin_Mutate|TestExample_Blog|TestExample_Workspace|TestExample_TodosMutation)$' -count=1 -timeout 40m -v`. Record each test's time (it was 409 / 141 / 29 / 82 / 9 s) and confirm PASS: the scores and counts are unchanged under the new order.
- [ ] **Step 2: Push the branch and read CI** with `gh run watch` / `gh run view`. Then trigger the nightly path once on the branch: `gh workflow run ci.yml --ref test-speed-phase1`. Check all of these: green; per-shard times; the `mutation` job ran with `changes` reporting all=true; the deploy jobs ran (ci.yml changed); no job near its timeout.
- [ ] **Step 3: A docs-only push skips the suite.** A small docs commit (e.g. this plan's results section) must run only `changes` (and `lint` / `test`/rest if it touches `book/`).
- [ ] **Step 4: Write a results note** at the end of this plan: before and after times, local (-short, full, gates) and CI (branch push, nightly). Update memory (alder-test-time-followup). Then use superpowers:finishing-a-development-branch.
