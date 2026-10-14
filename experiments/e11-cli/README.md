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
- **Baseline `files`**: E9's `baseline` cells for T5, T6, T7, T9, T10
  (`e9-feedback/baseline/runs/`, reused); T2, T3, T4 run fresh here in file mode with the same
  system prompt, one sample each (`baseline/runs/`).
- **Models**: `claude:haiku`, `claude:sonnet`, `claude:opus` (recorded as `claude-haiku`, …, as in
  E9) through the `claude` CLI with the harness's isolation flags; one sample (`s1`): 24 `cli` cells
  + 9 baseline cells.
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

## Layout

| Path | What it is |
|---|---|
| `prompt-reference.md` | the system prompt (`python3 build_context.py --reference`) |
| `prompt-cli.md` | the format page (every `cli` prompt carries it) |
| `tasks` → `../tasks` | the canonical task texts |
| `runs/<model>/<task>-s1/` | the `cli` cells: `round<N>/{output.md,script.txt,commands.jsonl,transcript.md,check.json,<app>/}`, `summary.json` |
| `baseline/runs/<model>/<task>-s1/` | the fresh T2–T4 file cells |
| `logs/` | one log per invocation |
| `smoke/runs/` | the smoke cell (below) |
| `alder` | the binary (git-ignored; its sha256 in `RUN.md` once the matrix runs) |

## Run

From the repository root, after `go build -o experiments/e11-cli/alder ./cmd/alder` at the commit the
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

The nine fresh baseline cells (file mode, E9's flags, the same system prompt and budget):

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
