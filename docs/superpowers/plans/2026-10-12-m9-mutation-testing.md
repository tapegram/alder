# M9: Mutation testing — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `alder test --mutate [--min R] [--json]` compiles every curated, well-typed mutant of the app into the one test binary, runs the suite per mutant, and prints the score and every survivor as the test that is missing.

**Architecture:** (1) `internal/mutate` collects the mutant table from the typed IR (fifteen operators, structural suppression, stable ids); the mutated IR nodes gain source positions. (2) gen, with `Options{Mutate}`, emits each mutation point as a switch on `app.Mutant` plus the `Mutants` table and schema overrides; `rtesting.Main --mutate` runs the suite per mutant with early stop, crash and timeout kills, `--min`, the JSON shape; `alder test` wires the flags; the harness records a score. (3) The examples run under `--mutate`, survivors become tests where real, and the book teaches it.

**Tech Stack:** Go; the existing test build (`gen.Options{Tests}`) and runner.

**Spec:** `docs/superpowers/specs/2026-10-12-m9-mutation-testing-design.md` (binding: the command, the operator table, the suppression list, schemata, the JSON shape).

## Global Constraints

- Exactly the spec's operators and suppression rules; every mutant well-typed by construction (a gen e2e test vets the mutated build of every fixture).
- Never mutate `test` bodies, `fake` entries, `view`/components, the stdlib.
- Mutant ids stable: IR order (module, declaration, node pre-order); documented renumbering on edit.
- Production emission (`Mutate` unset) byte-identical: every existing golden unchanged.
- Plain `alder test` unchanged in behaviour and cost.
- Output texts exactly as the spec; `--json` shape exactly as the spec.
- Gate: `gofmt -l $(git ls-files '*.go')` empty; `go vet ./...`; `go test ./... -count=1`; both examples check/build/test and `alder test --mutate` on each (scores reported); `(cd experiments && python3 -m unittest)`; `go test ./book -count=1`.
- Commit messages end with `Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>` and `Claude-Session: https://claude.ai/code/session_01VSwqPTccEdvyAkyJ2WtrhG`. Stage by explicit path.

## Review Focus

1. **A mutant that hangs** (a negated loop condition in a recursive helper; a `where dropped` that makes a query return thousands of rows into a `List.map` that recurses): expected — the per-mutant deadline kills it with `note: timed out`, the run continues, the total time is bounded by mutants × timeout. Test in Task 2 (rtesting with a fixture that spins).
2. **A `guard dropped` whose other arm references a binding only the guard's condition established** (`match found { Just x -> … x …, Nothing -> Task.notFound }` → the other arm uses `x`): expected — the mutant is NOT "the other arm" naively; for `Match` the guard-drop replaces the `notFound` arm's body by the sibling's body only when the sibling binds nothing; otherwise the operator is skipped (suppressed as "unscopable"). Test in Task 1 (the suppression) and Task 2 (the e2e: the mutant builds).
3. **`requires dropped` on a page whose `load` uses the narrowed session field as non-Maybe** (`session.user` is `UserId` under `requires user`, `Maybe UserId` without): expected — the mutant skips the `requires` CHECK in the route (serves everyone) but keeps the narrowing by using the session's default-value for the field when absent? NO — ruling: the mutant keeps the route's narrowing but drops the redirect: a request without the field is served with the field's zero value only if that is typable… Simplest correct shape: `requires dropped` makes the route treat an absent session field as "present with a placeholder" ONLY for brands over Text (`UserId "mutant"`); for other field types the operator is skipped. Test in Task 2 (todos: the `requires user` pages served to a stranger → the ownership tests kill it).
4. **Schema mutants and migrations**: a `default changed` must apply to the test database the suite creates (via `Tables`/`CreateTables`), not to any migration file; an app with migrations runs the mutant against tables created by… ruling: schema mutants run the suite with `Ensure` on the mutated `TableDesc`s instead of `Migrate` (the migrations are the production path; the mutant asks "would a test notice the default"), and the JSON notes `tables: "ensure"`. Test in Task 2 (the blog, which has migrations).
5. **Score when some mutants are suppressed or skipped**: expected — `total` counts only the mutants that ran; `suppressed` is reported separately; `--min` is computed over `total`. Test in Task 2.

---

### Task 1: `internal/mutate` and IR positions

**Files:**
- Modify: `internal/ir/ir.go` (`Pos struct{Line, Col int}` on `If`, `Match`, `Op`, `Lit`, `DbSelect`, `DbInsert`/`DbUpdate`/`DbDelete`, `Update`, `Ctor`, `EffBind`; set by `internal/ir/lower*.go` from the syntax span — add the field where the node is built), `dump.go` unchanged (positions not dumped, so IR goldens stay), `ir_test.go` (positions present on a fixture)
- Create: `internal/mutate/mutate.go` (`type Mutant struct{ ID int; Module, Decl, Kind string; Line int; Before, After string; Node ir.Expr; Replacement ir.Expr; Page *ir.Page (requires dropped); Table *ir.Table; Field string; Tables []db.TableDesc (schema) }`; `Collect(p *ir.Program) (ms []Mutant, suppressed []Suppressed)`; the walker over every non-test, non-fake, non-view value; one function per operator; `Before`/`After` rendered by a small IR pretty-printer `internal/mutate/show.go` (one line, elided at 80 columns); the suppression rules of the spec incl. Review Focus 2's "unscopable" guard; id assignment), `mutate_test.go` (a golden per operator under `testdata/mutate/<op>/{app/*.alder, want.txt}` listing the mutant table; suppression positive/negative cases; id stability; "never in tests/fakes/view"), `testdata/golden/mutate_*`
- Modify: `rt/db` (nothing) ; `internal/ir` helpers to clone/replace an expression for `Replacement` (a `Clone` for the handful of node kinds involved)

**Interfaces (binding for Task 2):**
```go
package mutate
type Mutant struct { ID int; Module, Decl, Kind string; Line int; Before, After string; Node ir.Expr; Replacement ir.Expr; Page *ir.Page; Field string; Tables []db.TableDesc }
type Suppressed struct { Kind, Module, Decl string; Line int; Why string }
func Collect(p *ir.Program) ([]Mutant, []Suppressed)
// Kinds: "requires dropped", "guard dropped", "where dropped", "orderBy reversed", "limit dropped", "offset dropped",
//        "write replaced", "update field dropped", "redirect → rerender", "reject → rerender", "arm swapped",
//        "condition negated", "operator flipped", "bool flipped", "default changed", "cascade ↔ restrict"
```

- [ ] **Step 1 (RED):** operator goldens; suppression tests; stability test. **Step 2:** positions in the lowerer; `Collect`. **Step 3 (GREEN):** `go test ./internal/... -count=1`; IR goldens unchanged. **Step 4:** commit.

---

### Task 2: gen schemata, the runner, the command, the harness field

**Files:**
- Modify: `internal/gen/gen.go` (`Options{Mutate bool}`; `Emit` calls `mutate.Collect` when set, builds `node → []mutant`), `internal/gen/expr.go`+`effects.go` (`code(e)`: when `e` has mutants, emit `func() T { if app.Mutant == k { return <code(Replacement)> } ... return <code(e)> }()` or an `if` chain in statement form where the node is a step; `requires dropped` in `routes.go` (the check skipped under the id, with the Review Focus 3 placeholder); `caps.go`: `var Mutant int`, `var Mutants = []rtesting.Mutant{…}` with `Tables` overrides for schema mutants; production emission untouched), goldens `gen_mutate` (test build with mutants; the todos production golden unchanged), `gen_test.go` (`TestGen_Mutate` e2e: the fixture builds and vets with every mutant; each kills/survives as expected per class)
- Modify: `rt/testing/testing.go` (`type Mutant struct{ID int; Module, Decl, Kind string; Line int; Before, After string; Tables []db.TableDesc}`; `Main` gains `--mutate`, `--min`; `runMutants(cases, mutants, opts)`: per mutant set `Mutant`, override tables (Ensure path, Review Focus 4), run cases with early stop, crash/timeout kills, collect `by`/`note`; text report + JSON as the spec; "tests fail: fix them before measuring mutants"; "no tests: nothing can kill a mutant"), `testing_test.go` (kill by failure/crash/timeout; `--min`; `-run`; the two refusals; score over ran-only, suppressed separate)
- Modify: `cmd/alder/test.go` (`--mutate`, `--min` → the scratch build with `scratch.Options{Tests: true, Mutate: true}` and the runner flags; help text), `internal/scratch/scratch.go` (`Options{Mutate}` → `gen.Options`), `cmd/alder/test_test.go` (e2e on the fixture: the survivors text verbatim; `--json`; exit codes)
- Modify: `experiments/run_loop.py` (`--mutate` opt-in: a `--test` round that passed also runs `alder test --mutate --json` and records `mutation: {score, killed, survived, suppressed, survivors: [{module, decl, line, kind}]}`; `harness.mutate: true`), `test_run_loop.py`, `report_loop.py` (a `Mut` column `k/n` when any summary has it; E5–E8 byte-identical), `experiments/README.md`

- [ ] **Step 1 (RED):** rtesting tests; gen golden + e2e; cmd e2e; harness tests. **Step 2:** implement gen → rtesting → cmd/scratch → harness. **Step 3 (GREEN)** + full gate (both examples: `alder test --mutate` runs and reports; production goldens unchanged). **Step 4:** commits (gen+rtesting; cmd+scratch; harness).

---

### Task 3: Examples, book, reference

**Files:**
- Modify: `examples/todos`, `examples/blog` (run `alder test --mutate`; for each survivor that is a real gap, add the test that kills it, in the module it belongs to; keep legitimate survivors and quote them; record the before/after scores in the examples' READMEs or module comments; `TestExample_*` assert the score ≥ the recorded one), chapter 14's counts/transcripts
- Modify: `book/10-testing.md` (new section "Mutation testing" per the spec, with the REAL todos run captured before and after the new tests; the operator table; suppression; the rule), `book/11-tooling.md` (flags), `book/13-compiler.md` (schemata: one real switch quoted from a mutate build; kills; the per-mutant timeout; schema overrides), `book/12-rationale.md` (why mutation score, not line coverage; why no annotation; why curated), `book/README.md`, `experiments/build_context.py` (the reference keeps the command, the rule, the output shape; report `--reference --size`), harness README (E9 may use `--mutate`)
- Every quoted output from real runs; `go test ./book`.

- [ ] **Step 1:** examples + scores; **Step 2:** book + reference; **Step 3:** full gate; commits (examples; book).

---

## Self-review notes

- Spec coverage: operators + suppression + ids + positions (T1); schemata, runner, command, JSON, harness (T2); book, examples, reference (T3). Non-goals untouched.
- Interfaces: T2 consumes T1's `mutate.Collect`/`Mutant`; T3 consumes T2's command and texts.
- Review Focus 1–5 each have a test; RF3/RF4 carry rulings (placeholder for `requires dropped`; Ensure path for schema mutants) that the spec's table should reflect — the plan is authoritative where the spec's table is vaguer (the `limit` row: dropped, not zeroed; the `requires` row: the placeholder rule).
