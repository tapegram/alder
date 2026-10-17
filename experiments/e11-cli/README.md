# E11: CLI-only development

The design is `docs/superpowers/specs/2026-10-14-e11-cli-only-design.md` (binding). This page
summarises it and gives the exact commands.

## Question

When a model may not write a file and may only run `alder` commands (the generators, `edit`,
`remove`, `fill` with one checked expression per hole), how close does it get to the recorded drills'
ceiling (`testdata/drill/t10-teams`, `t9-bookings`), which gaps are the model's and which the
command set's, and how does it compare with the file-emitting baseline on the same tasks?

## Hypotheses

- **H1, structure by construction.** No structural errors (naming, imports, placement, missing
  declarations): every refusal is a logic or typing error inside one expression, or a command misused.
- **H2, fewer rounds, same behaviour.** Command runs pass in no more rounds than E9's file runs, with
  hidden-test pass rates and mutation scores at least as high.
- **H3, the command set is the ceiling.** Where a run stalls the reason is classifiable, and
  "command missing" is rare after M13.

## Design

- **Condition `cli`** (`run_loop.py --cli`, `experiments/README.md` "`--cli`"): each round the model
  answers with one ```` ```alder-cli ```` script (`sh` accepted) in the drill scripts' format; the
  harness refuses any line that is not an `alder …` command, runs the commands in order on the
  round's app (`round<N>/<app>/`: round 0 from `alder new <app> --bare`, then a copy of the previous
  round's), stops at the first that fails, and records `script.txt`, `commands.jsonl` and
  `transcript.md`. The app is scored as E9 scores a round: check → build → test → `--mutate`; the
  hidden tests run once on the final round. A round passes only if every command ran and the app
  passes. A failed round's prompt carries every transcript so far and "Continue from here: the app
  is as the commands left it; send the next script."
- **Budget**: 6 rounds (`--retries 5`); a run that never passes is scored on its last round.
- **Prompt**: system prompt `prompt-reference.md` (`build_context.py --reference`, which carries the
  command table and the fill rules); user prompt the task text (`experiments/tasks/`, unchanged),
  the rule "Build it with `alder` commands only; you may not write a file.", and `prompt-cli.md`
  (the format page, with the first ten commands of the T10 drill as the worked example).
- **Baseline `files`**: as designed, E9's `baseline` cells for T5–T10 reused and T2–T4 run fresh; as
  run, all eight tasks were run fresh here in file mode with the same system prompt and budget, one
  sample each (`baseline/runs/`). E9's cells are compared in `findings.md` §3.
- **Models**: as designed, `claude:haiku`, `claude:sonnet`, `claude:opus`; as run, Haiku and Sonnet
  only in run 1 (Opus dropped for budget; run 2 adds it), recorded as `claude-haiku`, `claude-sonnet` as in E9, through the
  `claude` CLI with the harness's isolation flags; one sample (`s1`): 16 `cli` cells + 16 baseline
  cells.
- **Hidden tests**: E9's for T5, T6, T7, T9, T10; E8's for T4; none for T2 and T3.
- **Measured per cell**: pass and the round; hidden pass rate; mutation score and survivors;
  commands run / parsed; refusals by kind; stdin lines and the largest body; `--decl` uses; the
  names check (reported, not scored); the stall reason on a failing run, classified by hand in
  `findings.md`.

### Deviation: `--base` for T2 and T7

T2 ("Add a page to the example app") and T7 ("Change the blog app") change an existing app, so their
`cli` cells, like E9's and E7's file cells, run with `--base` (T2 `examples/todos`, T7
`experiments/e8-hidden/base/blog-nodrafts --migrate`): round 0's app is the base's export instead of
`alder new <app> --bare`, and the prompt shows its files after the format page. The other six start
bare.

## Results

The full write-up is `findings.md`. Matrix as run (2026-10-14): Haiku and Sonnet × the eight tasks × one
sample, `cli` and `files`, 32 cells; Opus dropped for budget.

- **Pass:** `files` 16 / 16, in 1–2 rounds; `cli` 12 / 16 (Haiku 7 / 8, Sonnet 5 / 8), in 4–6 rounds
  (mean 4.8). The `cli` failures: both T4s, Sonnet T5 (its last round's own `alder test` failed one
  generated test) and Sonnet T6 (never built).
- **Hidden (T4–T10, 94 tests):** `files` 88 / 94 (T4 5 / 8 in both cells: the canonical T4 has no Names
  block, E8's hidden T4 assumes one); `cli` 60 / 94 (Haiku T4 and T6: the hidden module does not
  compile; Sonnet T4 and T6: never built).
- **Mutation score**, the 12 cells passing in both: `cli` mean 0.80, `files` 0.67 (higher in 10 / 12),
  because most `cli` tests are the generators' (112 of 150 in the passing bare-start cells, 19 of them
  `new table`'s delete-rule tests, the rest `new form`'s, `new msg`'s, `new auth`'s and `--requires`').
- **Structure:** all 82 `cli` rounds type-checked (`files` round 0: 7 / 16 failed `alder check`); 13 of
  the 640 commands run were refused for a structural mistake (import, order, naming).
- **H3:** gaps in three task shapes (T4 had a reachable workaround; its stalls became misuses).
- **Stalls (64 refusals, hand-classified):** 49 command misused (13 of them a lambda over the
  parameters a page's `load`/`view`/`update` already binds), 8 body, 6 command missing (an `App`
  message, a prop on `Layout.Page`, a form on a message with parameters), 1 tool hang (`alder fill` loops forever
  on any 2-byte UTF-8 character after a non-name byte: `qualifiers` in `internal/generate/fill.go`
  reads one byte as a rune; a compiler bug for the next fix batch), 0 misread. `--json` and `--dry-run` were never used.

### Run 2 (2026-10-16, sample `r2`, `findings-run2.md`)

After the fix batch (`git log --oneline 1cf3747..edbb667`: `new msg App`, `edit decl … add-prop`, `new form --msg` on an
existing message, `edit auth --to`, refusals that name the fix, `alder check`'s "N typed holes left", the extended names
check, T4's Names block). Haiku, Sonnet and **Opus** × the eight tasks, `cli`: 24 cells under `runs/<model>/<task>-r2/`; Opus's
`files` cells: 8 under `baseline/runs/claude-opus/<task>-r2/` (Haiku's and Sonnet's `files` cells are run 1's `s1`).
Binary `alder-r2` (git-ignored; `devel+4c0e289`, sha256 `424b8de5…0ea23`, in full in `findings-run2.md`).
Runner `run-matrix-r2.sh`. **Two defects of the setup:** the system prompt `prompt-reference.md` is run 1's byte for byte (never
regenerated after the batch's reference changes, 1,102 bytes), so the models were not told the new commands or
the lambda line; and T2's base (`examples/todos`) fails `alder check` under the binary (4 errors), so all three T2 cells
are void.

- **Pass (own tests):** Haiku 2 / 8 (run 1: 7 / 8), Sonnet 7 / 8 (5 / 8), Opus 7 / 8 (T2 void; 7 / 7 valid), in 2–6 rounds
  (mean 3.6 Opus, 4.4 Haiku+Sonnet); Opus `files` 8 / 8 in 1–3. Hidden: Haiku+Sonnet 69 / 94 (run 1: 60; `files` 88),
  Opus 47 / 47 (`files` 47 / 47). Mutation, cells passing in both conditions: `cli` 0.74 against `files` 0.57
  (Haiku+Sonnet, 9 cells), 0.80 against 0.78 (Opus, 7).
- **The fixes:** "command missing" 6 → 0 of 82 refusals: `new msg App` and `new form --msg` on a message with parameters
  were used 3 times each (every T4 and T6 cell) and never refused; `edit decl … add-prop` and `edit auth --to` were used 0 times
  (never named to the models; the `Layout` prop was refused again in 3 cells). Lambda over bound parameters 13 → 11 (+1
  Opus), `fill` on a non-hole 8 → 5 (+1), a name before it exists 8 → 8 (+2): not reduced, without the reference lines.
  T6: Sonnet and Opus pass (hidden 9 / 9), Haiku one round short. T4: Opus 8 / 8; Sonnet's harness pass has hidden 0 / 8
  (no `Notes.Show`); Haiku never built. Rounds ending with holes: 76 % (Haiku+Sonnet; run 1: 63 %), the holes line
  printed in 7 `alder check` runs. `--json` 4 uses, `--dry-run` 0.
- **Opus:** 67 % of the commands it wrote ran (Sonnet 50 %, Haiku 33 %); refusals in 52 % of its rounds (61 %, 87 %); no
  `Model`-of-a-generated-page, quoting or block-into-hole misuses; its three body refusals are failing tests.
- **New stall patterns:** the T2 base (16 refusals, nothing repairs an app that fails `alder check`); `alder test` last with the
  generators' holes left (5 rounds); generated tests that disagree with the model's view or `load` (4 cells); `Model` of a generated
  page (5, all Haiku).

To regenerate the tables: `python3 experiments/report_loop.py experiments/e11-cli` and `python3
experiments/report_loop.py experiments/e11-cli/baseline` (E9's for comparison: `python3
experiments/report_loop.py experiments/e9-feedback/baseline`).

## Layout

| Path | What it is |
|---|---|
| `prompt-reference.md` | the system prompt (`python3 build_context.py --reference`) |
| `prompt-cli.md` | the format page (every `cli` prompt carries it) |
| `tasks` → `../tasks` | the canonical task texts |
| `runs/<model>/<task>-s1/` | the `cli` cells: `round<N>/{output.md,script.txt,commands.jsonl,transcript.md,check.json,<app>/}`, `summary.json` |
| `baseline/runs/<model>/<task>-s1/` | the file cells (all eight tasks, run fresh) |
| `run-matrix.sh`, `run-matrix-r2.sh` | the runners of run 1 and run 2 (`cli` or `files`; `MODELS` overrides the model list) |
| `findings.md` | run 1: the results, the stall table, H1–H3, the command-set gaps |
| `findings-run2.md` | run 2 (after the fix batch, with Opus): the fixes' effect, the stall table, H1–H3, the gaps for run 3 |
| `runs/<model>/<task>-r2/`, `baseline/runs/claude-opus/<task>-r2/` | run 2's cells (`alder-r2`, git-ignored, is its binary) |
| `logs/` | one log per invocation |
| `smoke/runs/` | the smoke cell (below) |
| `alder` | the binary (git-ignored), built at `87c7b42`, sha256 `2e10458a…098a6d` (in full in `findings.md`) |

## Run

The matrix ran as `experiments/e11-cli/run-matrix.sh cli` and `experiments/e11-cli/run-matrix.sh files`
(models from `MODELS`, default `claude:haiku claude:sonnet`; a cell with a `summary.json` is skipped, so
the script resumes). The loops below are what the design planned (with Opus; as run, without). From the
repository root, after `go build -o experiments/e11-cli/alder ./cmd/alder` at the commit the
matrix runs on:

```bash
E=experiments/e11-cli
COMMON=(--retries 5 --prompt $PWD/$E/prompt-reference.md --alder $E/alder --sample s1)
for model in claude:haiku claude:sonnet claude:opus; do
  for task in t2-newpage t3-newapp t4-appmsg t5-ownership t6-orders t7-drafts t9-bookings t10-teams; do
    case $task in
      t2-newpage)  extra=(--base examples/todos);;
      t3-newapp)   extra=();;
      t4-appmsg)   extra=(--hidden experiments/e8-hidden/hidden);;
      t7-drafts)   extra=(--hidden experiments/e9-feedback/hidden --base experiments/e8-hidden/base/blog-nodrafts --migrate);;
      *)           extra=(--hidden experiments/e9-feedback/hidden);;
    esac
    python3 -u experiments/run_loop.py --exp $E --model $model --task $task --cli --condition cli "${COMMON[@]}" "${extra[@]}" \
      >> $E/logs/cli-${model#claude:}.log 2>&1
  done
done
```

The designed baseline cells (file mode, E9's flags, the same system prompt and budget); as run, `run-matrix.sh
files` ran this for all eight tasks, with the `case` of the loop above:

```bash
for model in claude:haiku claude:sonnet claude:opus; do
  for task in t2-newpage t3-newapp t4-appmsg; do
    case $task in
      t2-newpage) extra=(--base examples/todos);;
      t3-newapp)  extra=();;
      t4-appmsg)  extra=(--hidden experiments/e8-hidden/hidden);;
    esac
    python3 -u experiments/run_loop.py --exp $E/baseline --model $model --task $task --build --test --mutate \
      --condition files "${COMMON[@]}" "${extra[@]}" >> $E/logs/baseline-${model#claude:}.log 2>&1
  done
done
```

Tables: `python3 experiments/report_loop.py experiments/e11-cli` (the `cli` cells, with the Cmds,
Refused, Stdin, Max body, --decl and Names columns) and `python3 experiments/report_loop.py
experiments/e11-cli/baseline experiments/e9-feedback/baseline` (the files cells).

## Smoke cell

`smoke/runs/claude-haiku/t2-newpage-s1/` (an experiment directory of its own, so the matrix's `s1`
cell and `report_loop.py experiments/e11-cli` never see it): one Haiku cell on T2 with
`--cli --retries 1 --base examples/todos --condition cli --sample s1`, run to check the harness and
the format page. It is not part of the matrix.

- First run (`logs/smoke-t2-newpage-claude-haiku.log`, harness `b0750da`; its run directory was
  replaced by the second): pass in round 1. Round 0's script stopped at its 2nd command (a `fill`
  that did not check); round 1's 5 commands all ran, and it passed although it had dropped round 0's
  unrun rename form, empty-name test and Index link. The model wrote commands, not files.
- Second run (`logs/smoke-t2-newpage-claude-haiku-2.log`, after the ruling that a script that stops
  early is not finished, `cli.complete`/`cli.stopped_at`, and the page's line saying so): fail.
  Round 0: 18 commands, stopped at command 3 (`alder fill --decl Profile.load`, exit 1); round 1
  continued from there (10 commands), stopped at command 4 (`alder fill --decl Profile.view`, exit 1);
  the budget ended: `complete: false`, `stopped_at: {round 1, command 4, line 26, exit1}`, scored on
  round 1's app (build refused: holes). 7 of 28 commands ran, 2 refused.
