# E11: CLI-only development — Design

Date: 2026-10-14. Status: approved in conversation (the user chose "script
rounds" for the first run). Related: M12 (the generators), M13 (the E11
commands and the two recorded drills), E9 (the baseline runs and the hidden
tests), `experiments/tasks/README.md` (the canonical texts).

## Question

When a model may not write a file and may only run `alder` commands (the
generators, `edit`, `remove`, `fill` with one checked expression per hole),
how close does it get to the recorded drills' ceiling, which gaps are the
model's and which are the command set's, and how does it compare with the
file-emitting baseline on the same tasks?

## Hypotheses

- **H1, structure by construction.** Structural errors (naming, imports,
  placement, missing declarations — E7's failure classes) do not occur:
  every refusal in a run is a logic or typing error inside one expression,
  or a command misused.
- **H2, fewer rounds, same behaviour.** Command runs reach a passing app in
  no more rounds than E9's file-emitting runs, with hidden-test pass rates
  and mutation scores at least as high.
- **H3, the command set is the ceiling.** Where a run stalls, the reason is
  classifiable, and "command missing" is rare after M13.

## Design

- **Condition `cli`**: a new harness mode `--cli`. Each round the model
  answers with ONE script: a fenced block of `alder` command lines, each
  optionally followed by a stdin body in a heredoc (`<<'EOF' … EOF`), in
  the drill scripts' format. The harness parses the script, refuses any
  line that is not an `alder …` command (the round ends there with the
  refusal recorded), runs the commands in order on the round's app
  directory (a copy of the previous round's state; round 0 starts from
  `alder new <app> --bare`), stops at the first command that exits
  non-zero, and records every command's stdout, stderr, exit code and
  `--json` when the model asked for it. The round's app is then scored as
  E9 scores a round: `alder check` → `build` → `test` → `--mutate`; the
  hidden tests run once on the final round. A failing round's feedback is
  the transcript (every command and its output, the refusal in full) plus
  the task text, and "continue from here: the app is as the commands left
  it". A passing round ends the run.
- **Budget**: 6 rounds. A run that uses all 6 without passing is scored on
  its last round (hidden tests still run if it checks and builds).
- **Baseline `files`**: E9's runs for T5, T6, T7, T9, T10 (`baseline`
  condition; summaries reused, not re-run); T2, T3, T4 run fresh in the
  file-emitting mode with today's reference, one sample each.
- **Tasks**: the eight canonical texts in `experiments/tasks/` (T2–T7,
  T9, T10), unchanged, plus one sentence per run in the prompt: "Build it
  with `alder` commands only; you may not write a file." The Names blocks
  are binding (the drill's names check runs on the final app and is
  reported, not scored).
- **Models**: `claude:haiku`, `claude:sonnet`, `claude:opus` through the
  `claude` CLI as E9 (the harness's isolation flags); one sample each: 24
  `cli` cells + 9 fresh baseline cells.
- **Prompt**: the system prompt is the reference (`build_context.py
  --reference`, which carries the command table and the fill rules); the
  user prompt is the task text, the one-sentence rule, one page on the
  script format (the fence, the heredoc, one command per line, `--json`
  allowed, "the harness stops at the first failure and shows you
  everything"), and the first ten commands of the T10 drill as the worked
  example (names from T10, which is one of the tasks — recorded as a
  threat to validity; the example is the format, not the solution).
- **Measured per cell**: pass (own tests) and the round it passed; hidden
  pass rate; mutation score and survivors; commands run / refused;
  refusals by kind (parse, not-an-alder-command, exit 1 with the
  command's one-sentence reason classified as: existing target, unknown
  name, check error in a body, precondition); stdin lines and the largest
  body; `--decl` uses; the names check; the stall reason on a failing run,
  classified by hand in the findings as "command missing", "command
  misused", "hole unfillable (logic)", "misread task".
- **Harness changes** (`experiments/run_loop.py`): `--cli` (implies
  `--build --test --mutate`), the script parser (shared with
  `cmd/alder/drill_test.go`'s format: port the grammar, keep it in
  Python), the executor (subprocess per command, cwd = the round's app,
  timeouts, the refusal of non-`alder` lines, a cap of 80 commands per
  round), the transcript writer (`roundN/script.txt`, `roundN/transcript.
  md`, `roundN/commands.jsonl`), the feedback prompt builder, the summary
  fields (`cli.commands`, `cli.refused`, `cli.stdin_lines`, `cli.max_body`,
  `cli.decl_uses`, `cli.names`), `report_loop.py` columns; unit tests for
  the parser and the executor on a canned script (the T9 drill's first
  commands, run against the real binary); `--sample`, `--condition` as
  today.
- **Threats**: the worked example comes from T10; the models may have the
  book in training data (as in every experiment); one sample per cell;
  Opus cost (~24 cells × up to 6 rounds).

## Output

- `experiments/e11-cli/`: `README.md` (this design + how to run), `prompt-
  cli.md` (the format page), `runs/`, `logs/`, `findings.md` (per task ×
  model: rounds, hidden, score, commands, refusals, stall; the E9
  comparison table; H1–H3 verdicts; the list of command-set gaps found,
  each with the task and the command the run wanted — the input to the
  next milestone).
- `experiments/tasks/README.md` gains the E11 line; chapter 15 gains one
  paragraph with the headline numbers (verified against `findings.md` by
  hand, not by the book test).

## Non-goals

- One-command-per-turn (option B; a follow-up); Ollama models (the
  command format needs reliable long outputs); changing the task texts;
  scoring the names check (reported only); a browser check of the built
  apps.
