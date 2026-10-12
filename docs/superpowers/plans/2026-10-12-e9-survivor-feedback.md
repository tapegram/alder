# E9: Survivors as feedback — Experiment Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Measure the mutation score of the tests Claude Haiku, Sonnet and Opus write (H1); whether feeding a passing round's surviving mutants back raises it (H2); and keep E8's hidden tests as the behaviour check (H3).

**Design (approved 2026-10-12, option A: Claude only):** five tasks with Names blocks (T5, T6 with E8's two defects fixed, T7 on the blog with `--migrate`, new T9 bookings, new T10 teams), each with a reference solution passing its hidden tests and scoring ≥ 0.9 under `--mutate`; conditions `baseline` (E8's `with` text, `--build --test --mutate`) and `feedback` (+ up to two survivor rounds after the first passing round, app code frozen); Haiku + Sonnet × both × five × 2 samples (40), Opus × both × five × 1 (10) = 50 cells; no `--adapters`.

**Spec:** this plan is its own design record; conventions in `experiments/README.md`; M9's harness fields (`mutation`, `no_tests`, survivors with file/col/before/after); E8's material and findings (`experiments/e8-hidden/`); the M9 ledger's E9 notes (retry-prompt shape; freeze/diff app code; pin the binary).

## Global Constraints

- Feedback rounds may change only `test` declarations: the harness compares the IR of the round's program with tests removed (`alder dump --ir`, `(test …)` nodes stripped, hashed) against the passing round's; a mismatch rejects the round ("app code changed in a feedback round: only tests may change") and the retry says so; at most two feedback rounds, so a cell has at most `retries + 2` model calls after the first pass.
- The survivor retry prompt is exactly (binding): "Your tests pass, but S of N mutants survived (score X.XX). Each line below is a change to your app that no test noticed. Add a test, in the named module, that fails on it. Do not edit app code to remove a mutant. Only if no input can tell a survivor apart, say why in one comment." followed by at most 10 survivor lines in `alder test --mutate`'s text format, grouped by module, then the previous answer as usual.
- Hidden tests never appear in any prompt. The material gate: every hidden test passes on the reference and the alt solution; each reference scores ≥ 0.9 under `--mutate` (T7 on the blog base likewise); `verify.sh` prints both.
- The binary is a clean export of the E9 commit (after the codegen fix in Task 1), recorded with its sha256; prompts regenerated and committed with `sizes.md`.
- Trailers on every commit; stage by explicit path; runs committed as in E8 (`build/` and `adapters/` ignored).

## Review Focus

1. **A model "kills" a survivor with a tautology** (`test "x" = True`, or a test that calls the mutated function but asserts nothing about the changed behaviour): expected — the mutant still survives (a tautology fails on nothing), so the score does not move; `findings.md` hand-scores a sample of fed-back tests for this. Test in Task 3 (harness unit test: a FakeModel whose feedback round adds a tautology → score unchanged, round recorded).
2. **A model edits app code in a feedback round** (removes the `orderBy` a mutant named): expected — the frozen-app check rejects the round with the ruled text; the retry carries it; the cell's recorded score is the last accepted round's. Test in Task 3 (unit) and Task 2's references are unaffected.
3. **A reference that cannot reach 0.9** because of equivalent mutants: expected — the material author documents each survivor as equivalent (as the blog's three) and the gate accepts ≥ 0.9 computed over all mutants, so references must have few equivalents; if a task's shape makes that impossible, the task is reworked, not the gate. Test in Task 2 (`verify.sh` prints the survivors with a one-line reason each).
4. **Nested-pattern codegen blow-up** (M9's ledger: nested patterns with a shared fallback are exponential in the PLAIN build, 1.98 MB at depth 12): expected — fixed in Task 1 so a model's deeply nested `match` cannot time out the build; a 12-deep fixture builds linearly. Test in Task 1.
5. **Feedback rounds and hidden tests**: a feedback round adds tests only, so hidden results cannot change — the harness runs hidden tests once, after the final round, and `findings.md` states it. Test in Task 3 (unit: hidden run count 1).

---

### Task 1: Codegen fix for nested patterns with a shared fallback

**Files:** `internal/gen` (the match emitter: a shared fallback arm emitted once and jumped to, or hoisted into a closure, instead of duplicated per nested level), a golden/e2e fixture 12 deep with a size bound (linear), existing goldens unchanged unless the fix changes their text (explain). Gate: full.
- [ ] RED (the 12-deep fixture with a bound) → implement → GREEN → commit.

### Task 2: Material (`experiments/e9-feedback/`)

**Files:** `README.md` (design, matrix, scoring, gate); `tasks/<task>.md` (one condition text: E8's `with` form; T6 with the mechanism hint removed from the paragraph and the body's refusal sentence kept; T9 bookings: rooms, a booking is a room + a date range (`from`/`to` dates), no two bookings of one room overlap (`[from, to)`), a booking can be cancelled by its owner until its `from` date (not after), `/rooms` lists rooms with the next free date, `/rooms/{id}` shows the room's bookings and a form to book, `/bookings` lists the user's; T10 teams: a team has an owner and members, the owner invites by email (an `Invite` row), an invited user who logs in sees the invite on `/invites` and accepts or declines, `/teams/{id}` is members-only (404 otherwise), only the owner removes a member (a member's attempt is 404), the owner cannot remove themselves, `/teams` lists the user's teams; both with full Names blocks incl. ids/classes and redirect targets); `reference/<task>/` and `reference-alt/<task>/` (T5/T6/T7 reused from E8 with T6's stale-page test relaxed to accept `Reject` as refusal — define the hidden assertion as "no line stored and the answer is not `Redirected` to a page showing the line"; T9/T10 new, each with its own tests reaching ≥ 0.9); `hidden/<task>/Hidden.alder` (E8's for T5/T7; T6 relaxed; T9: an overlapping booking is refused and not stored; cancellation after `from` is refused (`Test.login` + the clock: use the test clock's date); the next free date; a stranger's booking page is `PageMissing`; T10: non-member 404; member cannot remove; owner cannot remove self; accept makes the team visible; decline does not); `verify.sh` (check/build/own/hidden/`--mutate` score/survivors with reasons; exits non-zero below 0.9 or on any hidden failure; the alt solutions too).
- [ ] tasks → references → hidden → `verify.sh` green → commits (tasks+references; hidden+verify+README).

### Task 3: Harness `--feedback N`, frozen-app check, report

**Files:** `experiments/run_loop.py` (`--feedback N` requires `--test --mutate`: after the first passing round with survivors, up to N extra rounds with the ruled prompt; the frozen-app check via `alder dump --ir` minus `(test …)` nodes, hashed (add `alder dump --ir --no-tests`? ruling: implement the stripping in the harness on the dump text, tests are top-level `(test …)` s-expressions); a rejected round records `rejected_reason`; `mutation` per round already recorded; `score_before`/`score_after` at the summary top level; hidden tests run once after the final round), `test_run_loop.py` (feedback round with a real test → score rises; tautology → unchanged; app edit → rejected with the text; two feedback rounds max; hidden run count 1; prompts never contain hidden tests), `report_loop.py` (a `Score` column `before → after` when any summary has feedback; E5–E8 byte-identical), `experiments/README.md`.
- [ ] RED → implement → GREEN → commit.

### Task 4: Run the matrix, score, findings

**Files:** `experiments/e9-feedback/{prompt-full.md,sizes.md,RUN.md,logs/,baseline/,feedback/,runs/**,scores.md,survivors.md,findings.md,repros/}`. Protocol as E8 (Claude cells; Haiku, Sonnet s1/s2, then Opus; one model at a time; no edits mid-run; a compiler bug → repro, continue). `survivors.md`: every final survivor (cell, module, decl, line, kind) and, for feedback cells, each fed-back test hand-scored *(hand-scored)* as real / tautology / wrong-module. `findings.md`: §0 what changed; §1 headline (own ✓, hidden k/n, score before → after, rounds); §2 H1 scores by model/task with survivor kinds; §3 H2 feedback deltas, the hand-scored test quality, rejected rounds; §4 H3 hidden results; §5 T9/T10 as harder tasks (ceiling?); §6 protocol notes; §7 what changes because of this (reference, book, language, M-next). README experiment list line.
- [ ] export/build/prompts → matrix → scores/survivors/findings → commits.

---

## Self-review notes

- H1–H3 each measured (Task 3 fields; Task 4 sections); Review Focus 1–5 each tested or gated.
- Interfaces: Task 4 consumes Task 2's material and Task 3's flags; Task 1 is independent (compiler) and lands before the export.
- Out of scope: Qwen (option A); `--adapters`; feeding hidden failures back; jobs.
