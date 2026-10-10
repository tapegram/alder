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

- `claude:NAME` runs `claude -p` isolated, in a fresh empty temp directory, with the prompt as
  `--system-prompt` and only the task on stdin. Each round records `claude_version`
  (`claude --version`) and `model`; `summary.json` records the exact command line as
  `claude_argv` (the system prompt replaced by `<system prompt>`):

  ```
  claude -p --model NAME --output-format text --system-prompt <system prompt>
    --no-session-persistence --setting-sources "" --strict-mcp-config --tools "" --disable-slash-commands
  ```

  **Excluded:** `CLAUDE.md` and project memory (the empty cwd); the user, project and local
  settings files (`--setting-sources ""`), and with them their hooks (SessionStart and the rest),
  enabled plugins and permissions; every MCP server, the user's connectors included
  (`--strict-mcp-config` with no `--mcp-config`); every tool, built-in or deferred (Skill and
  ToolSearch included: `--tools ""`); skills and slash commands (`--disable-slash-commands`);
  the saved session. Checked with `--output-format stream-json --verbose` on claude 2.1.295: the
  init event lists no tools, no MCP servers, no skills and no slash commands.
  **Not excluded:** managed (policy) settings; Claude Code's built-in plugins (that init event
  still lists three `@builtin` ones); the auto-memory directory keyed by the temp cwd (new and
  empty every call). `--bare` would drop more but requires `ANTHROPIC_API_KEY` (it reads no
  OAuth login), so it is not used. **E6 ran before this**, in the empty temp cwd with
  `--no-session-persistence` and `--disallowedTools` naming ten built-in tools: user settings
  (their plugins' SessionStart hooks), user MCP connectors and the Skill/ToolSearch tools were
  not excluded then (E5 also ran in the repository as its cwd, with the prompt on stdin). No
  contamination was seen in the outputs; E6 was not re-run.
- `ollama:NAME` records `prompt_eval_count` and `eval_count` per round. Ollama silently drops the
  start of a prompt longer than `num_ctx`, so the round gets a `warning` (also printed to stderr)
  when `prompt_eval_count` is missing or below 90% of the estimate (`len(prompt) / 4`), or when
  `prompt_eval_count + eval_count` reaches `num_ctx - 512` (32768 - 512).

## Tasks

Tasks are chosen to exercise different things:

- **T1 modify**: add a field across table, form, view (touches TYP, ROUTE, HTML).
- **T2 new page**: a new page module from scratch (touches STRUCT, EFF, ROUTE).
- **T3 new app**: a small app from scratch (everything).
- **T4 app messages**: a notes app with a theme switch and logout on every page (app-level messages, session).
- **T5 ownership** (E6): shared notes where each user sees and deletes only their own; a message carrying another user's id must 404 (authorization in `update`).

## Experiment list

- **E1 baseline** (`e1-baseline/`): can agents write Alder from the reference alone; hand-scored, one shot.
- **E2 revised** (`e2-revised/`): the E1-revised book, with and without the example app; added T4.
- **E3 pitfalls** (`e3-pitfalls/`): does a "common mistakes" appendix help? No.
- **E4 msgpath** (`e4-msgpath/`): path-less `msg`; end of the spike.
- **E5 loop** (`e5-loop/`): first run with `alder check` in the loop (generate, check, one retry), T2–T4 × Qwen3-Coder, Haiku, Sonnet; scored by the checker. `repros/` holds each compiler problem it found as an app plus `check.json`.
- **E6 build** (`e6-build/`): the first run with `--build` (a round passes only if `alder check` is clean and `alder build --emit` succeeds), up to two retries, T2–T5 × Qwen3-Coder, Haiku, Sonnet; T5 (shared notes, each user's own) probes authorization in `update`. Every clean program built (no compiler bugs); `findings.md` hand-scores T5's owner check.

## Harness (M2)

`run_loop.py` closes the loop: generate, run `alder check --json`, feed the diagnostics back, retry.

```
python3 experiments/run_loop.py --exp experiments/e5-loop --model ollama:qwen3-coder:30b --task t3-newapp [--retries 1] [--prompt prompt.md] [--alder path/to/alder] [--base examples/todos] [--build]
python3 experiments/report_loop.py experiments/e5-loop > experiments/e5-loop/scores.md
```

`--model` (`ollama:NAME` or `claude:NAME`) and `--task` are repeatable. Each run writes `runs/<model>/<task>/round<N>/{output.md,check.json,app/...}` and `runs/<model>/<task>/summary.json`. Round N+1 starts from round N's files, overlaid with whatever the model re-emits. `--base DIR` seeds round 0 with `DIR/app`, for tasks that modify an existing app (T2 uses `--base examples/todos`): `DIR/app` *as committed*. DIR must be inside a git repository; the harness resolves HEAD of that repository once, exports `git archive <that commit> -- <DIR>/app` into a temporary directory once per invocation and copies round 0 from there, so uncommitted edits and untracked files in the working tree never leak into a run, and the summary records `base` (DIR as given) and `base_commit` (the commit exported). When `DIR/app` has uncommitted changes the harness says so on stderr ("warning: uncommitted changes under DIR/app are ignored …") and runs on. A DIR that does not exist exits 2 ("--base DIR does not exist"), and so does one outside a git repository: "`--base` must be inside a git repository (the harness seeds from `git archive HEAD`, never the working tree)". The retry prompt carries the task, the previous round's files in full (`YOUR PREVIOUS ANSWER`), and the diagnostics; every model call is a fresh process, so without the files a retry is a re-roll (E5's first run).

Summary shape:

```json
{"model": "ollama-qwen3-coder-30b", "task": "t3-newapp", "retries": 1, "base": null,
 "harness": {"build": true, "retries": 1}, "error": null, "outcome": "pass",
 "rounds": [{"n": 0, "files": 9, "exit": 1, "errors": 3, "warnings": 0, "codes": {"E0101": 1, "E0003": 2}, "rejected": [], "seconds": 41.2, "tokens": 2138,
             "prompt_eval_count": 24310, "eval_count": 2138, "stderr": ""},
            {"n": 1, "files": 9, "exit": 0, "errors": 0, "warnings": 0, "codes": {}, "rejected": [], "seconds": 38.0, "tokens": 2191,
             "prompt_eval_count": 27002, "eval_count": 2191, "stderr": "", "build": {"ok": true, "exit": 0, "stderr": ""}}],
 "final_pass": true}
```

Paths in `--- file: ... ---` markers are normalized (`X.alder` and `./app/X.alder` become `app/X.alder`); absolute paths, `..` escapes, backslashes and non-`.alder` files are refused, recorded per round as `rejected`, and named in the retry prompt.

`compiler_bug: true` appears only when a `--build` exited 2 after a clean check. Summaries from before M3d have no `harness` key.

### Outcomes

Since M4c every summary has `outcome`, one of four:

- **`pass`**: `alder check` exited 0 with no errors in some round and, with `--build`, that round's build succeeded (`final_pass: true`).
- **`fail`**: every round had errors (or a build that refused the program, exit 1), and the retries ran out. A check that exits 2 for another reason (no files, an I/O error alder reports) is an ordinary failed round too: it is alder answering.
- **`compiler_bug`**: the compiler failed on the model's program; `compiler_bug: true` as before, and `compiler_bug_kind` says how: `build` (a `--build` exited 2 after a clean check: an internal error, or generated Go that does not vet), `internal_error` (`alder check` exited 2 with "internal error" on stderr: a checker panic; its stderr is kept in `round<N>/check.stderr.txt`) or `hang` (`alder check` or `alder build` ran past its limit, 600 s / 900 s: a hang is the compiler's, and the harness kills its whole process group, `go` children included). The task stops, no retry.
- **`harness_error`**: no verdict on the model's program, because the harness itself failed: `alder check` or `alder build` could not be started, or exited with a code alder never uses (not 0, 1 or 2: a process killed by a signal, which Python reports as a negative code, or a wrapper's 137 for SIGKILL); or the model was unavailable (`error` is then set as before). The reason is in `harness_error`; the task stops there, with no retry, and `run_loop.py` exits 1.

`final_pass` and `compiler_bug` keep their meaning for older readers. Summaries from before M4c have no `outcome`; `report_loop.py` derives their result as before.

Scores are now generated by `report_loop.py`: one column per round (`r0 … rN`, each `errors` or `errors (build ✓|✗)`), then `Result` (pass / fail / compiler-bug / harness-error: compiler-bug with "(hang)" or "(check panic)" from `compiler_bug_kind`, harness-error from `outcome`). Summaries without `harness` (E5) keep the original two-round table; hand-scoring is reserved for design signals the checker cannot see. Tests: `cd experiments && python3 -m unittest test_run_loop -v`.
