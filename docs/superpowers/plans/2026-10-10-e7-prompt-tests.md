# E7: prompt size and `alder test` in the loop — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Answer two questions with one experiment: (1) does the agent prompt need an agent-facing cut now that the book is 118k characters (over Qwen's 32k window), and does a bigger window cost Qwen accuracy; (2) does putting `alder test` in the loop change outcomes — do agents write tests when asked, do their tests pass, and does the test loop fix bugs the checker cannot see. Plus one harder task (T6, orders with line items and a role) as the floor for the harder-use-case list.

**Architecture:** Three tasks. (1) Harness changes: `--test` (a clean build then `alder test --json`; failing tests are an ordinary retry carrying the report), `--num-ctx`, `--base` exporting `adapters/` too, `build_context.py --reference` (drops the sections written for humans) and `--size`, the E7 task texts (T2–T5 with a tests paragraph, new T6), README. (2) The run: the binary from a clean export, two prompts, the matrix, `scores.md`. (3) The write-up: `findings.md`, chapter 12 "E7", open questions, design signals.

**Tech Stack:** Python 3 stdlib (harness), Ollama (`qwen3-coder:30b`, local), `claude -p` (Haiku, Sonnet), the `alder` binary.

**Spec:** `docs/superpowers/specs/2026-10-09-m4-test-repl-design.md` names E7 ("its own plan after M4"); the design is this plan's "Design" section, agreed in conversation on 2026-10-10. The experiment record and procedure: `experiments/README.md`, `book/12-rationale.md` "Experiments".

## Global Constraints

- The harness is Python 3 stdlib only; `experiments/test_run_loop.py` stays green (`cd experiments && python3 -m unittest`).
- Experiments read from a clean export: the `alder` binary is built from `git archive HEAD` of the commit the prompts were built at; the commit is recorded in `findings.md` ("What changed since E6").
- Every number in `findings.md` and chapter 12 comes from `runs/**/summary.json` or `scores.md` unless marked *hand-scored*; the raw runs are kept (`runs/` is git-ignored as before; `summary.json`, `scores.md`, `prompt*.md`, `tasks/`, `findings.md` are committed).
- Qwen runs at temperature 0; each Claude cell is one sample (say so wherever a Claude number appears).
- The book is the single source of truth: chapter 12 gets the E7 section and any design signal that changes a decision; the experiments README documents the new flags.
- Commit messages end with `Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>` and `Claude-Session: https://claude.ai/code/session_01VSwqPTccEdvyAkyJ2WtrhG`. Stage explicitly.

## Design (binding)

**Conditions.** Qwen3-Coder 30B × {`full` = chapters 3–10 + example, `num_ctx` 65536; `reference` = the same with the human-facing sections dropped, `num_ctx` 32768} × T2–T6, all with `--build --test --retries 2`. Haiku and Sonnet × `full` × T2–T6, `--build --test --retries 2`. E6's T2–T5 results are the historical control (same tasks without the tests paragraph; a confound the write-up names). 20 runs.

**`reference` mode** (`build_context.py --reference`): drop these `## ` sections, by exact heading: ch.6 "How a request runs"; ch.7 "How effects run", "Implementing a capability"; ch.8 "How tables are stored", "Schema changes"; ch.10 "2. Running the tests", "6. What a trace says", "7. Choosing tests, and output for programs", "8. What cannot be tested yet". Measured today: 40,282 of 118,051 characters, leaving ~78k (≈21k tokens at len/4; Ollama counted 25k tokens for E6's 92k, so expect ~21–22k). `--size` prints characters, len/4, and — with `--count ollama:MODEL` — Ollama's `prompt_eval_count` for the prompt alone (one `/api/chat` call with `num_predict: 1`), so the number is measured, not estimated.

**`--test`** (run_loop.py): after a clean check and a successful `--build`, run `alder test --json <round_dir>` (timeout 600 s, process group). Each test object is recorded under `rnd["tests"] = {"passed": N, "failed": M, "results": [ … ]}`; `alder test` exit 0 → the round passes; exit 1 → an ordinary failed round whose retry prompt carries, under `TEST OUTPUT`, the text report (`alder test` without `--json`, run once more, or the JSON rendered as text — pick the text run, simpler to read) with the preface "The program builds but its tests fail; fix the program or the tests so every test passes."; exit 2 (a build problem, a crash outside any test) → `compiler_bug` kind `test` unless stderr says the tests timed out (→ `hang`). A round with zero tests and the task asked for tests is still a pass for the harness (the write-up scores "wrote tests" separately: `tests.passed + tests.failed == 0`). `--test` requires `--build` (usage error otherwise). `alder test` needs the adapters when the app declares a capability: `--base` now exports `<DIR>/adapters` too (tasks on the example app get the example's `Password` adapter); a new app that declares a capability and calls it from a test fails that test with "is not wired in tests" — the write-up counts it, the tasks avoid it (below).

**`--num-ctx N`** (run_loop.py): sets `OllamaModel.NUM_CTX` per invocation; recorded in `summary.json` (`harness.num_ctx`).

**Tasks** (`experiments/e7-prompt-tests/tasks/`): T2–T5 are E6's texts plus a final paragraph before the output instruction:

> Also write tests (`test "…" = …`, chapter 10) in the modules they test: ⟨per task⟩. `alder test` must pass. Do not write tests that call the `Password` capability.

Per task: T2 — "the profile page's `load` counts only the user's todos; renaming to an empty name is refused". T3 — "an unpublished post is not found for a reader; a comment with an empty author is refused; `/` lists only published posts, newest first". T4 — "a user cannot see another user's note (not found); the theme flips". T5 — "deleting another user's note, or a missing id, is not found; the owner can delete". T6 (new, `t6-orders.md`):

```markdown
# Task T6: a small shop's orders

Write a complete new Alder app (not the todo app) for the staff of a small
shop:

- Users log in with an email and a password, checked by a `Password`
  capability as in the example. A user has a role, `Staff` or `Manager`.
- A product has a name, a price in cents (not negative), and an active flag.
- An order belongs to the user who created it, has a status (`Open`, `Paid`
  or `Cancelled`) and a created time, and has line items: each line names a
  product and a quantity (positive).
- `/` requires login and lists the logged-in user's orders, newest first,
  with each order's status and total (the sum of quantity × price over its
  lines), linking to each order. A form creates a new, empty, Open order.
- `/orders/{id}` requires login and shows one order with its lines and
  total. While the order is Open: a form adds a line (choose an active
  product, give a quantity), and buttons mark it Paid or Cancelled. Another
  user's order is 404 — unless the logged-in user is a Manager.
- `/products` requires login and a Manager: it lists every product, has a
  form to add one, and a button per product to toggle active. A Staff user
  gets 404.
- `/login` and logout, an `App` module, a `Layout` module and a `User`
  module as in the reference.

Also write tests (`test "…" = …`, chapter 10) in the modules they test: an
Open order's total over two lines; adding a line to a Paid order is refused
(say in a comment whether you chose `Task.fail` or `Task.notFound`, and
why); a Staff user opening another user's order is not found; a Manager
opening it is. `alder test` must pass. Do not write tests that call the
`Password` capability.

Output every file, in full, each preceded by a line
`--- file: app/<path> ---`. No explanation, only files.
```

**Measures** (scores.md columns per run, from summary.json): rounds; errors per round; build; tests passed/failed per round; result (pass / fail / compiler-bug / harness-error); for Qwen also `prompt_eval_count` of round 0 and any truncation warning. The write-up compares: pass rate and round-0 errors `full` vs `reference` (Qwen); `prompt_eval_count` of each; whether any `full` round warned of truncation at 65536; tests written (count, per model); tests passing in round 0; rounds where the test loop (not the checker) found the bug (a round with 0 errors, build ✓, tests ✗ followed by a pass); T6 as the floor (hand-scored: does the Manager rule exist in `load`, where does the price live).

## Review Focus

1. **A round whose tests fail but which is otherwise clean**: expected — recorded as a failed round with `tests.failed > 0`, the retry prompt carries the test report and the previous files, and the next clean round with passing tests is the pass. Test in Task 1 (`test_test_failure_is_retry`).
2. **`alder test` exit 2 after a clean build** (an adapter that does not vet in the scratch build, a crash outside a test): expected — `compiler_bug` kind `test` (the model's program checked and built), the task stops. Test in Task 1 (`test_test_exit2_is_compiler_bug`).
3. **A `--test` run of an app with no tests**: expected — `0 passed, 0 failed`, the round passes, `tests == {"passed": 0, "failed": 0, …}`. Test in Task 1 (`test_no_tests_passes`).
4. **`--reference` dropping a heading that no longer exists** (the book changes): expected — `build_context.py` exits 2 naming the heading, never silently builds the wrong prompt. Test in Task 1 (`test_reference_unknown_heading`).
5. **A Qwen `full` round that overflows 65536**: expected — the existing truncation/overflow warning fires with the new `num_ctx`, and the write-up reports it. Test in Task 1 (`test_num_ctx_flag_sets_warning_threshold`).

---

### Task 1: Harness — `--test`, `--num-ctx`, adapters in `--base`, `build_context.py --reference`, the E7 tasks

**Files:**
- Modify: `experiments/run_loop.py`, `experiments/report_loop.py`, `experiments/test_run_loop.py`, `experiments/build_context.py`, `experiments/README.md`
- Create: `experiments/e7-prompt-tests/tasks/{t2-newpage,t3-newapp,t4-appmsg,t5-ownership,t6-orders}.md`
- Create: `experiments/test_build_context.py`

**Interfaces:**
- Consumes: `run_one`, `run_build`, `run_proc`, `retry_prompt`, `export_base`, `OllamaModel.NUM_CTX`, `report_loop.py`'s row builder; `alder test --json` (one object per test on stdout, summary on stderr; exit 0/1/2; `-timeout`).
- Produces: `run_loop.py --test` (requires `--build`), `--num-ctx N`; `summary.json`: `harness.test: true`, `harness.num_ctx`, per round `tests: {"passed", "failed", "results": [{module, name, ok, left, right, error, trace, ms}]}`, `test_output` (the text report, truncated to 4000 chars) on a failing round, `compiler_bug_kind: "test"`; `export_base` also exports `<rel>/adapters` when it exists; `report_loop.py` adds a `Tests` column (`✓ N` or `✗ M/N` per round, after the build mark) and a `Result` of `compiler-bug` for kind `test`; `build_context.py [--reference] [--size] [--count ollama:MODEL]`, the drop list as a module-level `HUMAN_SECTIONS = {"06": [...], ...}` with the exact headings above.

- [ ] **Step 1: Harness tests (RED)** — in `test_run_loop.py` (stub-`alder` pattern; the stub's `test` subcommand prints JSON lines and a summary to stderr and exits as the test needs): `test_test_requires_build`, `test_test_pass_records_counts`, `test_test_failure_is_retry` (retry prompt contains `TEST OUTPUT` and the report; the next round passes), `test_test_exit2_is_compiler_bug`, `test_test_hang_is_compiler_bug`, `test_no_tests_passes`, `test_base_exports_adapters` (a temp repo with `app/` and `adapters/x.go` committed → round 0 has `adapters/x.go`), `test_num_ctx_flag_sets_warning_threshold`, `test_report_tests_column`. In a new `test_build_context.py`: `test_reference_drops_listed_sections` (a temp book dir with two chapters; the dropped heading's text is absent, the rest present, order kept), `test_reference_unknown_heading` (exit 2 with the heading named), `test_size_prints_chars_and_estimate`. Make `build_context.py`'s `BOOK`/`EXAMPLE` injectable (`--book DIR --example DIR`) for the tests.
- [ ] **Step 2: Run (RED)**: `cd experiments && python3 -m unittest -v 2>&1 | tail -20`.
- [ ] **Step 3: Implement** per Interfaces/Design. `run_test(alder, round_dir, timeout=600)` mirrors `run_build` (process group, `Hang`); parse stdout JSON lines; on failure run `alder test <round_dir>` once more without `--json` for the text report (or render the JSON: text is what the model reads — use the text run). `retry_prompt` gains `test_output=` with the preface. `report_loop.py` column. `build_context.py`: split each chapter on `^## ` and drop by heading; `--size`; `--count`.
- [ ] **Step 4: Run (GREEN)**: all harness tests; then a real smoke: `python3 run_loop.py --exp /tmp/e7smoke --model fake --task t3-newapp --build --test` is not possible (FakeModel) — instead run the stub-based tests only, plus `alder test --json examples/todos` by hand to confirm the JSON shape the stub imitates.
- [ ] **Step 5: Tasks and README**: write the five task files (T2–T5 from `e6-build/tasks/` + the tests paragraph; T6 verbatim from Design); README: `--test`, `--num-ctx`, adapters in `--base`, `build_context.py --reference/--size/--count`, the `tests` fields, outcome kind `test`, and a line in the E-series list for E7.
- [ ] **Step 6: Commit**

```bash
git add experiments/run_loop.py experiments/report_loop.py experiments/test_run_loop.py experiments/build_context.py experiments/test_build_context.py experiments/README.md experiments/e7-prompt-tests/tasks
git commit -m "harness: --test (alder test in the loop), --num-ctx, adapters in --base; build_context --reference; E7 tasks"
```

---

### Task 2: The run

**Files:**
- Create: `experiments/e7-prompt-tests/{prompt-full.md,prompt-reference.md,sizes.md,scores.md}`; `experiments/e7-prompt-tests/runs/**` (git-ignored) and `experiments/e7-prompt-tests/alder` (git-ignored binary; confirm `.gitignore` covers `experiments/*/alder` and `runs/`).
- Modify: nothing in the compiler.

**Interfaces:**
- Consumes: Task 1's flags; Ollama at `localhost:11434` with `qwen3-coder:30b` (confirmed present); `claude` CLI 2.1.x; `examples/todos` for T2's `--base`.
- Produces: `sizes.md` (characters, len/4, Ollama's count, for both prompts; the commit); `scores.md` from `report_loop.py` over every summary; the raw runs.

**Procedure (binding):**
1. `COMMIT=$(git rev-parse --short HEAD)`; export `git archive HEAD` to a scratch dir; `go build -o experiments/e7-prompt-tests/alder ./cmd/alder` from that export (its `rt/` is the one the generated `go.mod` replaces to — note the path in `findings.md` as E6 did).
2. `python3 experiments/build_context.py > experiments/e7-prompt-tests/prompt-full.md`; `--reference > prompt-reference.md`; `--size --count ollama:qwen3-coder:30b` for each → `sizes.md`. The reference prompt must count under 26,000 tokens by Ollama; if not, extend the drop list (ch.6 "The request lifecycle" next) and record the change in `findings.md`.
3. Qwen, `reference`: `python3 experiments/run_loop.py --exp experiments/e7-prompt-tests --model ollama:qwen3-coder:30b --task t2-newpage --task t3-newapp --task t4-appmsg --task t5-ownership --task t6-orders --retries 2 --prompt experiments/e7-prompt-tests/prompt-reference.md --alder experiments/e7-prompt-tests/alder --base examples/todos --build --test --num-ctx 32768` — note `--base` applies to T2 only (check how `run_loop.py` scopes it; if it seeds every task, run T2 separately with `--base` and the others without, as E6 did). The model label must distinguish the condition: use `--label` if the harness has one, else run into `--exp experiments/e7-prompt-tests/reference` and `…/full` sub-experiments and merge the scores (the simplest: two `--exp` dirs; `report_loop.py` over both, with a `Condition` column added by hand to `scores.md`).
4. Qwen, `full`: the same with `prompt-full.md` and `--num-ctx 65536`.
5. Haiku and Sonnet, `full`: `--model claude:haiku --model claude:sonnet` (the names the harness resolves; E6 used `claude:haiku`/`claude:sonnet`), `--build --test --retries 2`.
6. `python3 experiments/report_loop.py experiments/e7-prompt-tests/{reference,full} > experiments/e7-prompt-tests/scores.md` (adapt to the layout chosen in 3); check every summary has an `outcome`; count runs = 20; list any `harness_error`/`compiler_bug` with its reason.
7. If a run hits `compiler_bug`, keep going with the rest; collect the repro under `experiments/e7-prompt-tests/repros/<model>-<task>/` (the round's `app/` + the stderr) for the write-up — compiler fixes are not part of E7.

- [ ] **Step 1: Build and size the prompts; commit `sizes.md`, the prompts, the binary excluded.**
- [ ] **Step 2: Run Qwen `reference` (5 tasks), then `full` (5 tasks); watch for truncation warnings; record wall-clock.**
- [ ] **Step 3: Run Haiku and Sonnet (5 tasks each).**
- [ ] **Step 4: `scores.md`; commit.**

```bash
git add experiments/e7-prompt-tests/prompt-full.md experiments/e7-prompt-tests/prompt-reference.md experiments/e7-prompt-tests/sizes.md experiments/e7-prompt-tests/scores.md experiments/e7-prompt-tests/*/runs/**/summary.json
git commit -m "E7: prompt size and alder test in the loop — the run"
```

---

### Task 3: The write-up

**Files:**
- Create: `experiments/e7-prompt-tests/findings.md`
- Modify: `book/12-rationale.md` (an "### E7: …" section under Experiments; the "Open questions" list; any decision whose status changes, e.g. "A REPL for people, `alder eval` for agents" stays untested — only what E7 tested changes), `experiments/README.md` (the E-series list entry with the headline), memory is the controller's.

**Design (binding):** `findings.md` follows E6's shape: §0 What changed since E6 (commit, prompts and their measured sizes, tasks, flags); §1 Headline table (every run: condition, model, task, rounds, errors, build, tests, result); §2 Prompt size (Qwen `reference` vs `full`: pass, round-0 errors, `prompt_eval_count`, warnings; what the dropped sections cost, if anything — look at the errors for signs of missing knowledge, e.g. a request-flow mistake); §3 Tests in the loop (tests written per run; passing in round 0; rounds where tests, not the checker, found the bug — quote one; tests that were wrong rather than the program; models that skipped tests); §4 T6 (hand-scored: the Manager rule, totals, where the status check lives; the floor); §5 Retries and compiler messages (classification a/b/c/d as E5/E6; any message that misled, with the exact text); §6 Harness notes; §7 What changes because of this (concrete: an agent-facing prompt mode? a chapter 10 cut? a diagnostic message? a language feature — each as a proposal with the evidence). Chapter 12's E7 section is the headline table plus §2–§3's conclusions in ~400 words, and the "what changes" bullets go to the open questions or decisions as appropriate.

- [ ] **Step 1: Write `findings.md` from the summaries (every number traceable; hand-scored items marked).**
- [ ] **Step 2: Chapter 12 and README; `go test ./book` (chapter 12 has no code blocks that change, but run it).**
- [ ] **Step 3: Commit.**

```bash
git add experiments/e7-prompt-tests/findings.md book/12-rationale.md experiments/README.md
git commit -m "E7 findings: prompt size, tests in the loop, T6"
```

---

## Self-review notes

- Both questions have a condition and a measure; T6 is the floor for the harder use cases. The historical control (E6) is named as a confound, not hidden.
- Review Focus 1–5 each have a named test in Task 1.
- The run is reproducible: commit, prompts, sizes, flags and the binary's provenance are recorded; `runs/` keeps the raw rounds.
- Not in E7: hidden acceptance tests (E8, needs tasks that pin names); qwen3:8b (floor only, not informative here); more than one Claude sample per cell (cost).
