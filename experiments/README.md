# Experiments

Each experiment lives in `experiments/<id>-<name>/` with:

- `prompt.md`: the exact context given to the model (built by `build_context.py`)
- `tasks/`: one file per task
- `runs/<model>/<task>.md`: raw model output
- `scores.md`: hand scores, one row per (model, task)
- `findings.md`: what was learned, and what changed in the book as a result

## Method

1. The model receives the reference chapters (3–9) and the full to-do
   example as context, then one task. No compiler, no retries, one shot.
2. Output is scored by hand against the error taxonomy below. Each
   distinct error is counted once per output. **PASS** = compiles as
   written. **PARTIAL** = 1–2 errors, each a one-line fix a compiler
   message would point at. **FAIL** = 3+ errors or a structural
   misunderstanding.
3. Anything the model did that is *reasonable but not Alder* is recorded as
   a design signal, separately from errors. These are the most valuable
   outputs: they show where the language fights the model's prior.

From E5 on, the loop harness (below) replaces steps 1–2: generate, `alder check`, retry with the
diagnostics. With `--build` (M3d), a round whose check is clean also runs
`alder build --emit <round dir>` in the round directory (the generated Go is vetted and lands in
`round<N>/build/`). The run passes only if that build succeeds. A failed build writes its
output to `round<N>/build.stderr.txt`. Exit 2 after a clean check (an internal error, or generated
Go that does not vet) is a compiler bug, not a model error: the run records `compiler_bug: true`
and stops (no retry; the model's program was fine). Exit 1 (typed holes, or anything else the
build refuses with diagnostics) is an ordinary failed round: the retry prompt says "The program
type-checks but `alder build` refused it; fix the cause below." and carries the build's output
under `BUILD OUTPUT`.

## Error taxonomy

| Code | Meaning | Examples |
|---|---|---|
| SYN | invented or wrong syntax | `case ... of`, `let ... in`, `fn`, semicolons, `=` in records, missing commas in `match` |
| LIB | hallucinated library | `List.mapIndexed`, `String.fromInt`, `Db.query`, `Task.andThen` |
| TYP | type misuse | `Int` in interpolation, `Maybe` used as value, brand constructed outside its module, `==` across brand and base, tuple |
| EFF | effect misuse | `Db.*` outside an effect block, write in `load`, missing `let x <-`, missing `Task.succeed`, `Task` in `Query` |
| ROUTE | page/msg/form wiring | `href` with a string, msg params missing, `update` arm missing, `form` field mismatch, `Params`/`Msg` written by hand |
| HTML | markup rules | `onclick`, unclosed tag, non-Text in braces, `<input>` inside `<form submit>` |
| STRUCT | module structure | wrong module name, missing import, unqualified cross-module name, `Todo.Todo` |
| ANN | missing annotation | top-level without signature |

## Models

| Name | How run |
|---|---|
| claude-haiku, claude-sonnet, claude-opus | subagent with the prompt + task, fresh context |
| qwen3-coder:30b, qwen3:8b | Ollama, `run_ollama.py`, `num_ctx` 32768, temperature 0 |

In the loop harness (`run_loop.py`):

- `claude:NAME` runs `claude -p` isolated: in a fresh empty temp directory (no `CLAUDE.md`, no
  project settings or memory), with the prompt as `--system-prompt` and only the task on stdin,
  `--no-session-persistence`, and every tool disallowed. Each round records `claude_version`
  (`claude --version`) and `model`.
- `ollama:NAME` records `prompt_eval_count` and `eval_count` per round. Ollama silently drops the
  start of a prompt longer than `num_ctx`, so when `prompt_eval_count` is missing or below 90% of
  the estimate (`len(prompt) / 4`), the round gets a `warning` (also printed to stderr).

## Tasks

Tasks are chosen to exercise different things:

- **T1 modify**: add a field across table, form, view (touches TYP, ROUTE, HTML).
- **T2 new page**: a new page module from scratch (touches STRUCT, EFF, ROUTE).
- **T3 new app**: a small app from scratch (everything).
- **T4 app messages**: a notes app with a theme switch and logout on every page (app-level messages, session).

## Experiment list

- **E1 baseline** (`e1-baseline/`): can agents write Alder from the reference alone; hand-scored, one shot.
- **E2 revised** (`e2-revised/`): the E1-revised book, with and without the example app; added T4.
- **E3 pitfalls** (`e3-pitfalls/`): does a "common mistakes" appendix help? No.
- **E4 msgpath** (`e4-msgpath/`): path-less `msg`; end of the spike.
- **E5 loop** (`e5-loop/`): first run with `alder check` in the loop (generate, check, one retry), T2–T4 × Qwen3-Coder, Haiku, Sonnet; scored by the checker. `repros/` holds each compiler problem it found as an app plus `check.json`.

## Harness (M2)

`run_loop.py` closes the loop: generate, run `alder check --json`, feed the diagnostics back, retry.

```
python3 experiments/run_loop.py --exp experiments/e5-loop --model ollama:qwen3-coder:30b --task t3-newapp [--retries 1] [--prompt prompt.md] [--alder path/to/alder] [--base examples/todos] [--build]
python3 experiments/report_loop.py experiments/e5-loop > experiments/e5-loop/scores.md
```

`--model` (`ollama:NAME` or `claude:NAME`) and `--task` are repeatable. Each run writes `runs/<model>/<task>/round<N>/{output.md,check.json,app/...}` and `runs/<model>/<task>/summary.json`. Round N+1 starts from round N's files, overlaid with whatever the model re-emits. `--base DIR` seeds round 0 with `DIR/app`, for tasks that modify an existing app (T2 uses `--base examples/todos`). The retry prompt carries the task, the previous round's files in full (`YOUR PREVIOUS ANSWER`), and the diagnostics; every model call is a fresh process, so without the files a retry is a re-roll (E5's first run).

Summary shape:

```json
{"model": "ollama-qwen3-coder-30b", "task": "t3-newapp", "retries": 1, "base": null,
 "harness": {"build": true, "retries": 1}, "error": null,
 "rounds": [{"n": 0, "files": 9, "exit": 1, "errors": 3, "warnings": 0, "codes": {"E0101": 1, "E0003": 2}, "rejected": [], "seconds": 41.2, "tokens": 2138,
             "prompt_eval_count": 24310, "eval_count": 2138, "stderr": ""},
            {"n": 1, "files": 9, "exit": 0, "errors": 0, "warnings": 0, "codes": {}, "rejected": [], "seconds": 38.0, "tokens": 2191,
             "prompt_eval_count": 27002, "eval_count": 2191, "stderr": "", "build": {"ok": true, "exit": 0, "stderr": ""}}],
 "final_pass": true}
```

Paths in `--- file: ... ---` markers are normalized (`X.alder` and `./app/X.alder` become `app/X.alder`); absolute paths, `..` escapes, backslashes and non-`.alder` files are refused, recorded per round as `rejected`, and named in the retry prompt.

`compiler_bug: true` appears only when a `--build` exited 2 after a clean check. Summaries from before M3d have no `harness` key.

Pass rule: a run passes when `alder check` exits 0 in some round (`final_pass`) and, with `--build`, that round's build succeeds. Exit 1 means errors, exit 2 means a checker or I/O failure (recorded, not raised). `error` is set only when the model itself was unavailable.

Scores are now generated by `report_loop.py`: one column per round (`r0 … rN`, each `errors` or `errors (build ✓|✗)`), then `Result` (pass / fail / compiler-bug, the last only for a build exit 2). Summaries without `harness` (E5) keep the original two-round table; hand-scoring is reserved for design signals the checker cannot see. Tests: `cd experiments && python3 -m unittest test_run_loop -v`.
