# E8: Hidden acceptance tests — Experiment Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Measure, with tests the model never sees, whether the programs Claude Haiku, Sonnet and Opus (and Qwen3-Coder as a control) write do what the task asked (H1); whether the task's "Also write tests…" paragraph changes the program, not only the tests (H2); and whether a schema-change task yields a safe migration (H3).

**Design (approved 2026-10-11, option B):** five tasks (T4 app messages, T5 ownership, T6 orders, T7 drafts on the blog with `--migrate`, T8 notify on the blog with a fixed `Mailer` capability + `fake`), each with a Names block and a verified reference solution; hidden browser tests per task under `experiments/e8-hidden/hidden/<task>/`; harness `--hidden DIR`; conditions `with` (E7's tests paragraph) and `without`; Haiku + Sonnet × both × five tasks × 2 samples (40), Opus × both × five × 1 (10), Qwen3-Coder (`reference` prompt, `--num-ctx 32768`) on T5/T6 × `with` × 1 (2); `--build --test --retries 2`; no `--adapters` (the blog/todos `Password` adapter is provided by `--base`, new apps leave Go unwritten).

**Spec:** this plan is its own design record; the harness and task conventions are `experiments/README.md`; E7's `findings.md` §7; the M5e review's E8 notes, restated here: task statements must fix page/form/message names, session shape, redirect targets, stable ids; hidden tests in their own module, compile failure counted per test; prefer `Test.open` + `Doc` queries over `.model`.

## Global Constraints

- Hidden tests never appear in any prompt; the harness copies them in only after the final round, and `runs/**` keeps them (they are part of the record). The retry prompt never mentions them.
- Every hidden test module compiles and passes against its task's reference solution before any model runs (`experiments/e8-hidden/reference/<task>/`), and `alder test` on the reference solution WITHOUT the hidden module also passes (the reference's own tests).
- Hidden tests use only names the task text fixes (its Names block), the browser steps, `Doc` queries by the ids/classes the Names block fixes, `Test.fake` for T8, `Test.login` with the fixed session shape; never `.model` fields, never a message the task did not name.
- A hidden module that fails to compile scores every one of its tests as failed with `reason: <first diagnostic>`; a crash likewise; so hidden scores are per test.
- The binary is a clean `git archive` export of the E8 commit; prompts are regenerated from `build_context.py` at that commit (`full` for Claude as E7; `reference` for Qwen) and committed with their `sizes.md`.
- Commit messages end with the session trailers; stage by explicit path; runs are committed as in E7 (`runs/**/adapters/` ignored).

## Review Focus

1. **A hidden test that over-specifies** (asserts a layout detail the task text does not fix): expected — caught by the reference-solution gate AND by a second, deliberately different reference ("alt" solution: same Names, different markup/structure) in Task 1; both must pass every hidden test.
2. **A model renames one fixed name** (e.g. `Notes.Show` → `Note.View`): expected — the hidden module fails to compile, every hidden test is recorded failed with that E0002, the own-tests column is unaffected, `findings.md` classes it `naming`. Test in Task 2 (harness unit test with a FakeModel).
3. **T7's migration drops data** (the model removes a column or changes an enum): expected — the round records `drops_data` and the hidden tests may still pass; the score table shows both. Test in Task 2 (harness) and Task 1 (a deliberately lossy alt solution for T7 flagged in the ledger as the negative control).
4. **T8's fake**: the model must declare `fake Mailer` in `Mail.alder` with the fixed entry; a hidden test does `Test.fake Mailer` then posts a comment and asserts `"Mailer.send"` in the trace; if the model's fake is missing → compile failure (E0003 "`Test.fake` wants a capability with a `fake`") → naming class. Test in Task 1 (reference) and Task 2 (unit).
5. **Condition leakage**: the `without` task text must contain no mention of tests; the harness records `condition` in the summary; `report_loop` groups by it. Test in Task 2.

---

### Task 1: Tasks, reference solutions, hidden tests

**Files:**
- Create: `experiments/e8-hidden/README.md` (the design above, the hypotheses, the matrix, the scoring rules, the run protocol), `experiments/e8-hidden/tasks/with/{t4-appmsg,t5-ownership,t6-orders,t7-drafts,t8-notify}.md` and `tasks/without/…` (identical except the tests paragraph; each with a **Names** block: page modules + paths + params with types/defaults; form names + fields + types + which message posts each; message names + params; `App`'s session shape verbatim; the redirect after each post; `id`/`class` for each element a hidden test reads — e.g. `#notes li`, `#summary`, `.total`, `a.next`; T7/T8 say "modify the blog app provided: output only files you add or change").
- Create: `experiments/e8-hidden/reference/<task>/app/**` (+ `migrations/` copied from the blog for T7/T8 bases are supplied by `--base examples/blog`; the reference for T7/T8 is the blog with the change applied), `experiments/e8-hidden/reference-alt/<task>/…` (a second solution per task differing in markup and structure, same Names; T7's alt is the lossy negative control — documented), both passing `alder check`, `alder build --emit`, `alder test`.
- Create: `experiments/e8-hidden/hidden/<task>/Hidden.alder` (module `Hidden`; imports the fixed modules; 4–8 browser tests per task per the table in the README; T7 also asserts via `Test.open` that search excludes the draft; T8 uses `Test.fake Mailer`), verified against both references (a script `experiments/e8-hidden/verify.sh` that copies each hidden module into each reference, runs `alder check` and `alder test -run Hidden`, and prints a table).
- (Task 2 owns `experiments/README.md`; Task 1 writes only under `experiments/e8-hidden/` so the two tasks can run in parallel.)

- [ ] **Step 1:** task texts with Names blocks (review each against the hidden tests you will write: every name a test uses is fixed). **Step 2:** reference solutions (check/build/test green). **Step 3:** hidden tests; `verify.sh` all green on `reference`, all green on `reference-alt` (except T7-alt's documented drops-data flag, which is a migration property, not a test). **Step 4:** commit (tasks + references; hidden + verify).

---

### Task 2: Harness `--hidden`, condition label, report

**Files:**
- Modify: `experiments/run_loop.py` (`--hidden DIR`: after the final round (pass or fail — hidden tests run whenever the final round's program CHECKS clean and builds; otherwise `hidden: {skipped: "<reason>"}`), copy `DIR/<task>/*.alder` into a scratch copy of the round (`round<N>/hidden/app/...` so the model's files are untouched), `alder check`; on diagnostics → every hidden test failed with the first diagnostic as `reason` (parse the hidden module's `test "…"` names statically to count them); else `alder test -run '^Hidden ' --json` → `hidden: {passed, failed, results: [{name, ok, left, right, trace, reason}]}`; `--condition LABEL` recorded as `harness.condition`; `--sample LABEL` suffixes the run directory (`<task>-<LABEL>`) and is recorded as `harness.sample` (the harness's existing `seed` is the base export, not a sampling seed); the summary's `outcome` is unchanged (own tests), `hidden_pass: k/n` added), `experiments/test_run_loop.py` (unit tests with `FakeModel`: hidden pass; hidden compile failure → per-test fail with reason; skipped when the program never built; condition recorded; `--hidden` with `--base`), `experiments/report_loop.py` (a **Hidden** column `k/n` (or `— (reason)`), a `Condition` column from `harness.condition`, grouping; E5–E7 `scores.md` regenerate byte-identical), `experiments/README.md` (`--hidden`, `--condition`, `--sample`, the summary shape; the E8 entry under "Experiment list" and T7/T8 under "Tasks").

- [ ] **Step 1 (RED):** unit tests. **Step 2:** implement. **Step 3 (GREEN):** `python3 -m unittest`; E5–E7 byte-identical. **Step 4:** commit.

---

### Task 3: Run the matrix, score, write findings

**Files:**
- Create: `experiments/e8-hidden/{prompt-full.md,prompt-reference.md,sizes.md}`, `RUN.md` (binary export commit + sha256, commands, timings, deviations), `logs/run.sh`, `with/` and `without/` experiment directories with `tasks -> ../tasks/<cond>` symlinks (E7's layout), `runs/**` (committed), `scores.md` (`report_loop.py` over both conditions, merged with the Condition column — generated, not hand-merged), `findings.md` (§0 what changed, §1 headline table, §2 H1 hidden results by behaviour class with examples, §3 H2 with vs without (own tests written? hidden pass rate?), §4 H3 T7 migrations (steps, drops_data), §5 Opus vs Sonnet vs Haiku, §6 Qwen control, §7 harness notes/deviations, §8 what changes because of this: the reference, the book, the language), `repros/` for any compiler problem.
- Protocol: Claude cells first (Haiku, Sonnet; then Opus), two samples per cell via Task 2's `--sample LABEL` (runs go to `runs/<model>/<task>-<LABEL>/`; the summary records `harness.sample`); Qwen last (T5, T6, `reference`, `--num-ctx 32768`). Watch for `harness_error`/`compiler_bug`; a compiler bug gets a repro and a ledger entry, the cell is re-run only if the bug is fixed in a follow-up commit (document). Total ≈ 52 runs.

- [ ] **Step 1:** export + build + prompts + sizes; `RUN.md`. **Step 2:** the matrix (background, with status checks). **Step 3:** `scores.md`, `findings.md`, repros; README experiment list; commit runs + results.

---

## Self-review notes

- Hypotheses H1–H3 each have a measurement (Task 2 fields; Task 3 sections). Review Focus 1–5 each have a test or gate.
- Interfaces: Task 3 consumes Task 2's `--hidden`/`--condition`/report; Task 1's hidden modules and references.
- Out of scope: `--adapters` tasks (no model Go runs); a real-browser layer; the reserved-name question (settled by the rename).
