# E12: AI usability after routes

Design: `docs/superpowers/specs/2026-11-11-e12-routes-experiment-design.md` (binding); issue #208. This
page summarises it and gives the exact commands. Status: designed; the matrix has not run. Results go to
`findings.md` and `scores.md` (Task 19).

## Question

E7-E11 asked how well Claude models write Alder from the reference (book chapters 3-10 as
`build_context.py --reference` cuts them). #160 then replaced pages and messages with routes (a route module's
GET route, write routes, `Response`, `input=`, `load=`, `intersect=`, `every=`). E12 asks the same question
of the route-era language, after porting the experiment's tasks, hidden tests, reference solutions and bases.

## Design

- 13 tasks: the ported T2-T7 and T9-T11 (the texts, with what changed in each, in `experiments/tasks/README.md`)
  and four new ones, T12 search, T13 feed, T14 board, T15 pantry. T2 and T3 have no hidden tests (their cells run without `--hidden`); the other
  eleven do (56 tests on the ported tasks, as E8-E11; 14 on the new).
- 3 models (`claude:claude-haiku-5-5`, `claude:claude-sonnet-5-5`, `claude:claude-opus-5-5`) × 2 samples
  (`s1`, `s2`) = 78 cells; condition `files` only (no `cli`, no `--feedback`, no `--adapters`); `--retries 5`
  (6 rounds); `--build --test --mutate --hidden experiments/e12-routes/hidden --condition files`; T2 adds
  `--base examples/todos`, T7 `--base experiments/e12-routes/base/blog-nodrafts --migrate`.
- One pinned commit: the binary is built from `git archive E12_COMMIT`, the prompt is built inside that
  export, and `RUN.md` records the commit, the binary's sha256 and `alder version`. Later commits never move
  the pin.
- No model run 9:00-17:00 America/New_York on weekdays; `run-matrix.sh` stops before 08:45 and resumes.

## Layout

```
experiments/e12-routes/
  README.md           this page
  RUN.md              binary, prompt, gate output, gate-time fixes, rulings, run log (E12_COMMIT, ALDER_SHA256)
  tasks -> ../tasks   the canonical texts (symlink, as e11-cli); experiments/tasks/README.md records the port
  hidden/<task>/Hidden.alder
  reference/<task>/   a verified solution (app/, SURVIVORS.md where a survivor is equivalent)
  reference-alt/<task>/   a second, differently written solution (T7's is the lossy-migration negative control)
  base/blog-nodrafts/ T7's base app
  alder               the pinned binary (git-ignored)
  replay_drill.py     replays a recorded drill to rebuild a drill reference
  test_run_matrix.py  run-matrix.sh's tests
  pin.sh              builds the binary and the prompt from one commit
  verify.sh           the material gate
  run-matrix.sh       the overnight matrix
  prompt-reference.md, sizes.md   the prompt and its size, from pin.sh prompt
  logs/               one log per model, plus matrix.lock and matrix.progress
  runs/<model>/<task>-<sample>/   run_loop.py output (runs/*/raw/ ignored)
  scores.md           from report_loop.py
  findings.md         the write-up (Task 19)
```

`prompt-reference.md`, `RUN.md`, `runs/`, `logs/`, `scores.md` and `findings.md` appear as the tasks that
produce them land.

## Commands

Run from the repository root.

**Pin the binary and the prompt** (`E12_COMMIT` is the commit at which the gate passed):

```
experiments/e12-routes/pin.sh binary COMMIT                              # builds experiments/e12-routes/alder; prints sha256 and `alder version`
experiments/e12-routes/pin.sh prompt COMMIT experiments/e12-routes      # writes prompt-reference.md and sizes.md from a fresh export of COMMIT
```

**The gate** (run once at the pin; mutation runs take minutes per solution):

```
experiments/e12-routes/verify.sh [task ...]      # no arguments: all thirteen tasks
ALDER=/path/to/alder experiments/e12-routes/verify.sh t6-orders
```

For each task and each solution (`reference/`, `reference-alt/`) it runs `alder check`, `alder build --emit`,
the solution's own tests, the hidden module (its test count must be the task's: T4 8, T5 8, T6 9, T7 7, T9 7,
T10 8, T11 9, T12 4, T13 3, T14 3, T15 4), `alder test --mutate` on the solution's own tests (score at least
0.9, every survivor justified in the solution's `SURVIVORS.md`), the names check and, for T7, whether the
migrations drop data (only the alt may). T2 and T3 are smoke rows. It also checks the task texts (tests
paragraph, output paragraph, a Names block on T4-T15, no page-era words). Exit 0 means the material is
complete; exit 1 names the failing cell; exit 2 is a missing binary.

**The matrix:**

```
experiments/e12-routes/run-matrix.sh
MODELS="claude:claude-haiku-5-5" SAMPLES="s1" TASKS="t2-newpage t4-appmsg" experiments/e12-routes/run-matrix.sh
DRY_RUN=1 experiments/e12-routes/run-matrix.sh        # prints each cell's command; runs and deletes nothing
PREFLIGHT_ONLY=1 experiments/e12-routes/run-matrix.sh # the preflight alone
STOP_AT=0830 experiments/e12-routes/run-matrix.sh
```

Environment: `MODELS`, `SAMPLES`, `TASKS` (space separated) narrow the matrix; `STOP_AT` (default `0845`,
New York time, weekdays) is checked before each cell; `DRY_RUN=1`; `PREFLIGHT_ONLY=1`. Cells run in the order
sample, then model, then task; a cell whose `summary.json` exists is skipped, so a re-run resumes. Before the
first cell of every invocation the preflight checks that the binary's sha256 is `RUN.md`'s, that `alder check`
accepts `examples/todos` and `base/blog-nodrafts` under it, that `prompt-reference.md` is byte-equal to what
the pinned commit builds, and that the inputs (tasks, hidden tests, references, bases, `verify.sh`,
`run-matrix.sh`, the harness) are unchanged since the pin and clean.

Exit codes: 0 complete; 3 stopped for the weekday window (before a cell, or a cell killed at 09:00; print
the next cell, re-run after 17:00 or on a weekend); 2 preflight failed (or the lock is held); 4 harness error
(a usage limit, network or auth failure ends the matrix rather than burn the remaining cells).
`runs/`, `logs/` and `scores.md` are committed by hand at each stop and at the end (Task 18's procedure), not by the script; they are outputs, not preflight
inputs.

**The tables:**

```
python3 experiments/report_loop.py experiments/e12-routes > experiments/e12-routes/scores.md
```

Columns: a round per `rN`, Tests, Hidden `k/n`, Mut (`k/n` killed), Result, Names, and the error codes per
round (the taxonomy in `experiments/README.md`, with the ROUTE row).

## Comparability with E8-E11

Hidden test counts per ported task are unchanged, so hidden pass rates compare one to one. The prompt differs
(the route-era chapters, a rewritten command table, three dropped subsections of chapter 10). One cut of the
old texts is corrected (T6's form on a parameterised write route), and four texts state implicitly-held
checks (a write route on another user's record is 404) that E8/E9 had through `load`; see
`experiments/tasks/README.md`. T9 and T10's references are the drill replay plus four tests added at the gate
(`RUN.md`).
