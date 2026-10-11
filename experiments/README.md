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

With `--test` (E7; requires `--build`, otherwise a usage error, exit 2), a round that built also
runs `alder test --json <round dir>` in the round directory (its own process group, 600 s). Every
test object it prints goes in the round's `tests`: `{"passed": N, "failed": M, "results": [{module,
name, ok, left, right, error, trace, ms}, …], "exit": E}` (`alder test`'s own JSON, empty fields
omitted). Exit 0 is the pass; a program with no tests passes too (`0 passed, 0 failed`: whether the
model wrote tests is scored separately, `passed + failed == 0`). Exit 1 is an ordinary failed round:
the harness runs `alder test <round dir>` once more without `--json` and the round records that text
report, truncated to 4000 characters, as `test_output` (the `--json` run's stderr goes to
`round<N>/test.stderr.txt` on any exit but 0); the retry prompt says "The program builds
but its tests fail; fix the program or the tests so every test passes." and carries the report
under `TEST OUTPUT`. Exit 2 (a build problem or a crash outside any test, after a clean check and
build) is a compiler bug, kind `test`, unless its stderr says the tests timed out (kind `hang`);
its stderr is kept in `round<N>/test.stderr.txt` and the task stops. `alder test` needs the Go
adapters when the app declares a capability, so `--base` exports them too (below).

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

Qwen3-Coder 30B is **out of the matrix since E7**: 0/10 with no clean check. It returns once the example and message fixes land, or is replaced by a dense 14B–32B coder.

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
  `prompt_eval_count + eval_count` reaches `num_ctx - 512` (32768 - 512 by default).
  `--num-ctx N` (E7) sets `num_ctx` for every Ollama model of the invocation, the warning's
  threshold with it (`N - 512`); every Ollama run records the window it ran with as
  `harness.num_ctx`. Claude models ignore the flag.
  Since E7's T2 re-runs, every Ollama call also sends `num_predict` 8192 (recorded as `harness.num_predict`)
  and each round records Ollama's `done_reason`: at temperature 0 Qwen can fall into a repetition loop that never
  ends (E7 T2: the example app re-emitted, then invented non-Alder files and a linter list repeated until the
  3600 s HTTP timeout, a `harness_error`). A round whose `done_reason` is `length` gets `truncated: true` and is an
  ordinary failed round (no build, no tests); the retry prompt adds "Your previous answer was cut off after 8192
  tokens; output only the Alder files (`--- file: app/<path> ---`), nothing else." 8192 is above every completed
  Ollama round in E6 and E7 (at most 5,393 tokens). The last `--- file:` section of a cut-off answer is
  incomplete, so it is **not written**: a version of that path from the previous round (or the `--base`) stays,
  the round records `cut_off_file` (e.g. `"app/Todos/Show.alder"`), and the retry prompt adds
  "`app/X.alder` was cut off and not written; emit it again in full". No `cut_off_file` when the last section's
  path was rejected anyway.

## Tasks

Tasks are chosen to exercise different things:

- **T1 modify**: add a field across table, form, view (touches TYP, ROUTE, HTML).
- **T2 new page**: a new page module from scratch (touches STRUCT, EFF, ROUTE).
- **T3 new app**: a small app from scratch (everything).
- **T4 app messages**: a notes app with a theme switch and logout on every page (app-level messages, session).
- **T5 ownership** (E6): shared notes where each user sees and deletes only their own; a message carrying another user's id must 404 (authorization in `update`).
- **T6 orders** (E7): a shop's orders with line items, a computed total and a `Staff`/`Manager` role that changes what `load` may show; the floor for harder use cases.

The canonical task texts for the next experiments are in `experiments/tasks/` (T2–T6 copied from `e7-prompt-tests/tasks/`; E7's own `tasks/` stay as they were run). T2 there is reworded after E7's runaway: "Output only the files you add or change: not the files you leave as they are, not the standard library, nothing outside `app/`."

From E7 every task ends with a tests paragraph ("Also write tests … `alder test` must pass. Do not write tests that call the `Password` capability.") naming what to test.

## Experiment list

- **E1 baseline** (`e1-baseline/`): can agents write Alder from the reference alone; hand-scored, one shot.
- **E2 revised** (`e2-revised/`): the E1-revised book, with and without the example app; added T4.
- **E3 pitfalls** (`e3-pitfalls/`): does a "common mistakes" appendix help? No.
- **E4 msgpath** (`e4-msgpath/`): path-less `msg`; end of the spike.
- **E5 loop** (`e5-loop/`): first run with `alder check` in the loop (generate, check, one retry), T2–T4 × Qwen3-Coder, Haiku, Sonnet; scored by the checker. `repros/` holds each compiler problem it found as an app plus `check.json`.
- **E6 build** (`e6-build/`): the first run with `--build` (a round passes only if `alder check` is clean and `alder build --emit` succeeds), up to two retries, T2–T5 × Qwen3-Coder, Haiku, Sonnet; T5 (shared notes, each user's own) probes authorization in `update`. Every clean program built (no compiler bugs); `findings.md` hand-scores T5's owner check.
- **E7 prompt and tests** (`e7-prompt-tests/`): two questions in one run. Prompt size: Qwen3-Coder on the `full` prompt (chapters 3–10 + example, `--num-ctx 65536`) against the `reference` prompt (`build_context.py --reference`, `--num-ctx 32768`). Tests in the loop: T2–T5 with a tests paragraph and a new T6, all `--build --test --retries 2`, Qwen3-Coder × both prompts, Haiku and Sonnet × `full`; E6's T2–T5 are the control. Result: Haiku and Sonnet 10 / 10 (every run wrote the named tests, all passed; T6 in round 0 for both); Qwen3-Coder 0 / 10, no round checked clean under either prompt, so the agent-facing cut is not the lever: two in five of its errors come from the example's `Theme` module, mostly copied into new apps without `Theme.alder` (the rest missing imports and duplicated or over-qualified constructors). Haiku's T3 pass shipped a hard-coded password after its adapter was rejected. The test loop caught nothing (no round built and then failed its tests), yet a mutation probe showed the tests catch 10 / 10 removed authorization checks. The Qwen T2 runaway forced a mid-run cap (`num_predict` 8192, `0c80d80`); `findings.md` §0, §6.

- **E8 (planned)**: hidden acceptance tests (tests the model never sees, run against its program) and a condition without the tests paragraph, to separate what the paragraph adds from what the loop catches.

## Harness (M2)

`run_loop.py` closes the loop: generate, run `alder check --json`, feed the diagnostics back, retry.

```
python3 experiments/run_loop.py --exp experiments/e5-loop --model ollama:qwen3-coder:30b --task t3-newapp [--retries 1] [--prompt prompt.md] [--alder path/to/alder] [--base examples/todos] [--build [--test]] [--migrate] [--adapters] [--num-ctx 32768]
python3 experiments/report_loop.py experiments/e5-loop > experiments/e5-loop/scores.md
```

**Warning: `--adapters` executes Go code the model wrote, on this machine, with your permissions, with no sandbox** (below).

`--model` (`ollama:NAME` or `claude:NAME`) and `--task` are repeatable. Each run writes `runs/<model>/<task>/round<N>/{output.md,check.json,app/...}` and `runs/<model>/<task>/summary.json`. Round N+1 starts from round N's files, overlaid with whatever the model re-emits. `--base DIR` seeds round 0 with `DIR/app`, for tasks that modify an existing app (T2 uses `--base examples/todos`): `DIR/app` *as committed*, and with it `DIR/adapters` when that commit has one (since E7: the Go adapters `alder build` and `alder test` need for an app with capabilities, e.g. the example's `Password`; every round carries them, the model writes only `app/`) and `DIR/migrations` when it has one (since M5b: the migrations an app with a database keeps its tables by; every round starts from the base's `migrations/`, never a previous round's, so a round's own migration is that round's change). DIR must be inside a git repository; the harness resolves HEAD of that repository once, exports `git archive <that commit> -- <DIR>/app [<DIR>/adapters] [<DIR>/migrations]` into a temporary directory once per invocation and copies round 0 from there, so uncommitted edits and untracked files in the working tree never leak into a run, and the summary records `base` (DIR as given) and `base_commit` (the commit exported). When `DIR/app` has uncommitted changes the harness says so on stderr ("warning: uncommitted changes under DIR/app are ignored …") and runs on. A DIR that does not exist exits 2 ("--base DIR does not exist"), and so does one outside a git repository: "`--base` must be inside a git repository (the harness seeds from `git archive HEAD`, never the working tree)". The retry prompt carries the task, the previous round's files in full (`YOUR PREVIOUS ANSWER`), and the diagnostics; every model call is a fresh process, so without the files a retry is a re-roll (E5's first run).

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

`harness` gains `"test": true` on a `--test` run (absent otherwise) and `"num_ctx": N` for an Ollama model. A round of a `--test` run that built has `tests`, and `test_output` when they failed:

```json
{"n": 0, "errors": 0, "build": {"ok": true, "exit": 0, "stderr": ""},
 "tests": {"passed": 1, "failed": 1, "exit": 1, "results": [
   {"module": "Todo", "name": "countOpen ignores done todos", "ok": true, "left": "1", "right": "1", "trace": [], "ms": 0.003},
   {"module": "Todos.Index", "name": "Toggle flips done", "ok": false, "left": "…", "right": "…", "trace": ["Db.get Todo"], "ms": 2.8}]},
 "test_output": "ok   Todo \"countOpen ignores done todos\"\nFAIL Todos.Index \"Toggle flips done\"\n…\n1 passed, 1 failed\n"}
```

Paths in `--- file: ... ---` markers are normalized (`X.alder` and `./app/X.alder` become `app/X.alder`); absolute paths, `..` escapes, backslashes and non-`.alder` files are refused, recorded per round as `rejected`, and named in the retry prompt. So is a path under `app/stdlib/` or a module named exactly like a standard-library module (`app/stdlib/List.alder`, `app/List.alder`, `app/Db.alder`): the names are the `stdlib/*.alder` stems of this repository, read once when the harness starts, plus `stdlib`; it is listed in `rejected` as `app/stdlib/List.alder (standard-library module)` (E7 T2: Qwen invented `app/stdlib/*.alder` files). A module *under* a stdlib name (`app/Time/Slots.alder`, `module Time.Slots`; `app/Text/Editor.alder`) is legal Alder, and the harness accepts it as the compiler does.

**`--migrate`** (opt-in, off by default; for E8, schema-change tasks on a `--base` with `migrations/`). A round whose check fails with E0019 alone (the app's tables changed since the base's last migration: the model changed a `table`, which only `alder migrate` can follow) runs `alder migrate <round>` and checks again; the round goes on (build, test) with the second check, and records `migrate`: `{"exit", "file", "steps", "drops_data", "codes_before"}` — the file it wrote under `round<N>/migrations/`, that file's `-- step:` and `-- DROPS DATA:` header lines, and the codes of the check before it. A change `alder migrate` refuses (exit 1: a new non-null column without a default, a `@renamed` that names nothing, …) is an ordinary failed round; its output is kept in `round<N>/migrate.stderr.txt` and the retry prompt carries it under `MIGRATE OUTPUT`. `alder migrate` exiting 2 with "internal error" is a compiler bug (kind `migrate`). Without `--migrate` an E0019 is an ordinary error the model sees. How a run's migration is scored (were its DROPS DATA lines intended?) is E8's design, not the harness's; the summary records `harness.migrate: true`.

**`--adapters`** (opt-in, off by default). **It executes Go code the model wrote, on this machine, with your permissions, with no sandbox: use it only on a machine and in a directory you are prepared to lose.** With it, `adapters/<name>.go` sections (no subdirectories, same path safety) are written into the round's `adapters/`, shown to the model in the retry prompt, and `alder build` / `alder test` compile and run them; the summary records `harness.adapters: true`. A Go file that does not vet is an ordinary failed round (build exit 1). With a `--base` that carries adapters, the provided ones are in every round too, and the model can overwrite them (a section with the same path replaces the base's file); the retry prompt lists an adapter the round still has unchanged from the base apart, after the model's files, under "These adapters are provided, not yours (from the base app); emit one only to replace it …", and one the model rewrote with its files. Without `--adapters` a `--- file: adapters/<name>.go ---` section is refused like any non-`.alder` file, and the retry prompt adds a notice: when the base provides that very adapter, "`adapters/password.go` was not accepted (this run does not take Go files); the `Password` adapter is provided — keep the capability and do not replace it with a password in the code" (E7: Haiku's T3 pass replaced a rejected adapter with a password in the code); when it provides others but not that one, "`adapters/hash.go` was not accepted (…); the `Password` adapter is provided; `Hash` is not: declare the capability and leave its Go side unwritten"; with no adapters at all, "… declare the capability and leave its Go side unwritten".

`compiler_bug: true` appears only when the compiler failed on a clean program (see Outcomes: a `--build` or `--test` exiting 2, a checker panic, a hang). Summaries from before M3d have no `harness` key.

#Rounds seeded with `--base` carry a copy of the base's `adapters/` (so `alder test` can run against the example's capability). Those copies are not committed with the runs (`runs/**/adapters/` is ignored; they are reproducible from `base_commit`). `experiments/go.mod` makes this directory a Go module of its own, so Go files inside run output never reach `go vet ./...` or `go test ./...` at the repository root.

## Outcomes

Since M4c every summary has `outcome`, one of four:

- **`pass`**: `alder check` exited 0 with no errors in some round and, with `--build`, that round's build succeeded (`final_pass: true`).
- **`fail`**: every round had errors (or a build that refused the program, exit 1), and the retries ran out. A check that exits 2 for another reason (no files, an I/O error alder reports) is an ordinary failed round too: it is alder answering.
- **`compiler_bug`**: the compiler failed on the model's program; `compiler_bug: true` as before, and `compiler_bug_kind` says how: `build` (a `--build` exited 2 after a clean check: an internal error, or generated Go that does not vet), `internal_error` (`alder check` exited 2 with "internal error" on stderr: a checker panic; its stderr is kept in `round<N>/check.stderr.txt`), `migrate` (a `--migrate` run's `alder migrate` exited 2 with "internal error"), `test` (a `--test` run's `alder test` exited 2 after a clean check and build: a build problem in the test program, adapters that do not compile there, or a crash outside any test; its stderr is kept in `round<N>/test.stderr.txt`) or `hang` (`alder check`, `alder build` or `alder test` ran past its limit, 600 s / 900 s / 600 s, or `alder test` exited 2 saying the tests timed out: a hang is the compiler's, and the harness kills its whole process group, `go` children included). The task stops, no retry. A test that fails, crashes or times out *inside* the tests (`alder test` exit 1) is the model's: an ordinary failed round.
- **`harness_error`**: no verdict on the model's program, because the harness itself failed: `alder check`, `alder build` or `alder test` could not be started, or exited with a code alder never uses (not 0, 1 or 2: a process killed by a signal, which Python reports as a negative code, or a wrapper's 137 for SIGKILL); or the model was unavailable (`error` is then set as before). The reason is in `harness_error`; the task stops there, with no retry, and `run_loop.py` exits 1.

`final_pass` and `compiler_bug` keep their meaning for older readers. Summaries from before M4c have no `outcome`; `report_loop.py` derives their result as before.

Scores are now generated by `report_loop.py`: one column per round (`r0 … rN`, each `errors` or `errors (build ✓|✗)`), then — only when some run used `--test` — `Tests` (for each round that ran tests, `rN ✓ N` when all N passed or `rN ✗ M/N` when M of N failed, e.g. `r0 ✗ 1/3; r1 ✓ 3`), then `Result` (pass / fail / compiler-bug / harness-error: compiler-bug with "(hang)" or "(check panic)" from `compiler_bug_kind`, harness-error from `outcome`; kind `test` is plain compiler-bug). Summaries without `harness` (E5) keep the original two-round table, and E5's and E6's `scores.md` regenerate byte-identical; hand-scoring is reserved for design signals the checker cannot see. Tests: `cd experiments && python3 -m unittest -v` (`test_run_loop`, `test_build_context`).

## The prompt (`build_context.py`)

```
python3 experiments/build_context.py [--chapters 03,04,05,06,07,08,09,10] [--no-example] [--reference] > prompt.md
python3 experiments/build_context.py [--reference] --size [--count ollama:qwen3-coder:30b [--num-ctx 65536]]
```

The prompt is a fixed header, the chapters (default 3–10) and the example app (`examples/todos/app`). `--reference` (E7) drops the `## ` sections written for a human reader, listed by exact heading in `HUMAN_SECTIONS`: ch.6 "How a request runs"; ch.7 "How effects run", "Implementing a capability"; ch.8 "How tables are stored" (its "Migrations" stays: the E0019 → `alder migrate` workflow is for agents too); ch.10 "2. Running the tests", "6. What a trace says", "7. Choosing tests, and output for programs", "8. What cannot be tested yet". A section runs from its heading to the next `## ` heading (one inside a code fence does not count); the chapter's text before its first `## ` and every other section are kept, in order. A listed heading the chapter no longer has exits 2 naming the chapter file and the heading, so a renamed section never silently builds the wrong prompt. `--size` prints the size instead of the prompt: `characters: N` and `estimate (len/4): N`; `--count ollama:MODEL` (implies `--size`) adds `prompt_eval_count (ollama:MODEL, num_ctx N): N`, Ollama's own count for the prompt alone (one `/api/chat` call with the prompt as the system message and `num_predict: 1`, at `--num-ctx`, default 65536, large enough not to truncate it; a count that reaches it warns). `--book DIR` and `--example DIR` replace the book and the example's `app/` directory (for tests).
