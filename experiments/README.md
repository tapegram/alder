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
under `TEST OUTPUT`. That includes `alder test` refusing the app's adapters (a module that
"cannot name an adapter package", the removed flat form, the "internal naming error" of two
capabilities, a mis-cased package directory, an `adapters/go.mod`) or finding that they "do not
compile with the app": since the M5d fixes those exit 1, the model's program, with `alder build`'s
message (`--build`'s `alder build --emit` does not look at adapters, so `alder test` is the first
to see them). A *missing* adapter is not refused by `alder test`: the capability runs unwired, a
warning on stderr lists it ("warning: 1 capability without an adapter runs unwired in tests:"),
and a test that calls it without `Test.fake` fails "not wired in tests" (exit 1); so a round can
pass `--test` with a capability whose Go is unwritten when its tests fake it, as fakes-first
development intends, and only a full `alder build` would refuse it. Any exit 2 (a build problem or a
crash outside any test, after a clean check and build) is a compiler bug, kind `test`, unless its stderr says the tests timed out (kind `hang`);
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
- **T7 drafts** (E8): modify the blog app with drafts taken out (`--base experiments/e8-hidden/base/blog-nodrafts`): posts gain a draft state that readers and search never see; a schema change, so it runs with `--migrate` and the round's migration (its steps and any `DROPS DATA` lines) is part of the score.
- **T8 notify** (E8): modify the blog app: a new comment notifies the post's author through a fixed `Mailer` capability (`Mail.alder`) that the model declares with a `fake`; the hidden tests use `Test.fake Mailer` and look for `Mailer.send` in the trace.

From E8 each task text has a **Names** block fixing every name a hidden test uses (page modules, paths and params; forms, fields and the message each posts; messages; `App`'s session shape; the redirect after each post; the `id`/`class` of each element a test reads). E8's task texts are in `e8-hidden/tasks/{with,without}/`.

The canonical task texts for the next experiments are in `experiments/tasks/` (T2–T6 copied from `e7-prompt-tests/tasks/`; E7's own `tasks/` stay as they were run). T2 there is reworded after E7's runaway: "Output only the files you add or change: not the files you leave as they are, not the standard library, nothing outside `app/`." After E9 it holds E9's T5, T6, T7, T9 and T10 with E9's two text fixes (the Password sentence, T10's redirect lines); `tasks/README.md` lists them, and E10 runs these texts.

From E7 every task ends with a tests paragraph ("Also write tests … `alder test` must pass. Do not write tests that call the `Password` capability.") (E7's wording; the canonical texts in `tasks/` now say "Do not call the real `Password` adapter from a test; installing the fake with `Test.fake Password` is fine") naming what to test.

## Experiment list

- **E1 baseline** (`e1-baseline/`): can agents write Alder from the reference alone; hand-scored, one shot.
- **E2 revised** (`e2-revised/`): the E1-revised book, with and without the example app; added T4.
- **E3 pitfalls** (`e3-pitfalls/`): does a "common mistakes" appendix help? No.
- **E4 msgpath** (`e4-msgpath/`): path-less `msg`; end of the spike.
- **E5 loop** (`e5-loop/`): first run with `alder check` in the loop (generate, check, one retry), T2–T4 × Qwen3-Coder, Haiku, Sonnet; scored by the checker. `repros/` holds each compiler problem it found as an app plus `check.json`.
- **E6 build** (`e6-build/`): the first run with `--build` (a round passes only if `alder check` is clean and `alder build --emit` succeeds), up to two retries, T2–T5 × Qwen3-Coder, Haiku, Sonnet; T5 (shared notes, each user's own) probes authorization in `update`. Every clean program built (no compiler bugs); `findings.md` hand-scores T5's owner check.
- **E7 prompt and tests** (`e7-prompt-tests/`): two questions in one run. Prompt size: Qwen3-Coder on the `full` prompt (chapters 3–10 + example, `--num-ctx 65536`) against the `reference` prompt (`build_context.py --reference`, `--num-ctx 32768`). Tests in the loop: T2–T5 with a tests paragraph and a new T6, all `--build --test --retries 2`, Qwen3-Coder × both prompts, Haiku and Sonnet × `full`; E6's T2–T5 are the control. Result: Haiku and Sonnet 10 / 10 (every run wrote the named tests, all passed; T6 in round 0 for both); Qwen3-Coder 0 / 10, no round checked clean under either prompt, so the agent-facing cut is not the lever: two in five of its errors come from the example's `Theme` module, mostly copied into new apps without `Theme.alder` (the rest missing imports and duplicated or over-qualified constructors). Haiku's T3 pass shipped a hard-coded password after its adapter was rejected. The test loop caught nothing (no round built and then failed its tests), yet a mutation probe showed the tests catch 10 / 10 removed authorization checks. The Qwen T2 runaway forced a mid-run cap (`num_predict` 8192, `0c80d80`); `findings.md` §0, §6.

- **E8 hidden acceptance tests** (`e8-hidden/`): tests the model never sees, run against its program after the final round (`--hidden`, below), to measure whether the programs do what the task asked (H1); whether the task's tests paragraph changes the program, not only the tests: conditions `with` (E7's paragraph) and `without` (`--condition`) (H2); whether a schema-change task yields a safe migration (H3). Five tasks with Names blocks and verified reference solutions: T4 app messages, T5 ownership, T6 orders, T7 drafts (the blog, `--migrate`), T8 notify (the blog, a fixed `Mailer` capability with a `fake`). Matrix: Haiku and Sonnet × both conditions × five tasks × 2 samples (`--sample`), Opus × both × five × 1, Qwen3-Coder (`reference` prompt; planned at `--num-ctx 32768`, run at 65536 because the prompt is now 42k tokens) on T5/T6 × `with` × 1 as a control; all `--build --test --retries 2`, no `--adapters`. A hidden module that does not compile against the program (a renamed page or message) scores every one of its tests failed. Since M5e the hidden tests can drive the model's app through the in-memory browser (chapter 10, section 9: `Test.login`, `Test.get`/`Test.open` a page, `Test.submit`/`Test.submitRaw` a form, `Test.click` a message, and the `Doc` queries), so they check behaviour across requests (a route's parameters, `requires`, redirects, the session, the page a user sees) without knowing the model's `update` signatures or `Model` fields: only the task's pages, forms and messages, which the task text names. A browser step acts only on controls the page renders, so a hidden test fails, with the step's text, when the model's page lacks the form or button the task asked for. Result: Claude 50 / 50 own passes and 365 / 370 hidden tests (Haiku and Sonnet 74/74 `with`, 72/74 `without`; Opus 37/37, 36/37); every failure is T6's stale-page test in a `without` run, where all three models refused with `Reject` (answered `Rerendered`, nothing stored) instead of the `Task.fail` the `with` paragraph names, a test stricter than the task text; no `naming` failure, no T7 migration dropped data (0 / 10); Qwen3-Coder passed T5 (8/8 hidden, its first pass) and failed T6; `e8-hidden/findings.md`.

- **E9 survivors as feedback** (`e9-feedback/`): the mutation score of the tests Claude writes (H1); whether feeding a passing round's surviving mutants back raises it (H2, `--feedback 2`, app code frozen); E8's hidden tests stay the behaviour check (H3). Five tasks with Names blocks (T5, T6 with E8's two defects fixed, T7 on the blog with `--migrate`, new T9 bookings, new T10 teams), each with a reference scoring ≥ 0.9 under `--mutate` and a calibration line (the reference's app with only the asked tests: 0.17–0.67); conditions `baseline` (`--build --test --mutate`) and `feedback` (+ `--feedback 2`); Haiku and Sonnet × both × five × 2 samples, Opus × both × five × 1 (50 cells); no `--adapters`. Result: 50 / 50 own passes; first-pass mutation scores 0.52 (Haiku), 0.51 (Sonnet), 0.73 (Opus), above the calibration line except on T7 (at it); no first pass tested login: the tasks' "Do not write tests that call the `Password` capability" literally bans the book's fake-based login test (23 of 50 first passes declared the example's `fake Password`, none used it), a material defect E8's texts share; feedback raised the score in 23 / 25 cells (Haiku 0.50 → 0.90, Sonnet 0.51 → 0.91, Opus 0.72 → 0.96; in the two that did not, a feedback round failed on the line-continuation rule (E0001), its retry fixed that and failed on something else), with 355 fed-back tests hand-scored 350 real, 5 wrong-module, 0 tautologies; one round rejected by the frozen-app rule (a testability refactor); 386 / 390 hidden tests (Haiku 152 / 156, Sonnet 156 / 156, Opus 78 / 78; Haiku's T10 Remove answers `Rerender` where an ambiguous Names-block sentence asks for `Redirect`, and its own test asserts it); `e9-feedback/findings.md`.

- **E11 CLI-only development** (`e11-cli/`): the model may not write a file, only run `alder` commands (`--cli`, below): each round's answer is one script of commands, run on the app and scored as E9 scores a round (own tests, hidden tests, mutation score), against the file-emitting baseline. Designed: the eight canonical tasks × Haiku, Sonnet, Opus × 1 sample, 6 rounds, E9's T5–T10 cells reused; as run (2026-10-14): Haiku and Sonnet only (Opus dropped for budget), the baseline re-run for all eight tasks, 32 cells. Result: `files` 16 / 16 in 1–2 rounds, `cli` 12 / 16 in 4–6 (both T4s, Sonnet T5 and T6 fail); hidden 88 / 94 against 60 / 94; every `cli` round's app type-checked (82 / 82; `files` round 0: 7 / 16 did not), but 13 of 640 commands run were refused for structural mistakes; mutation score higher in `cli` (mean 0.80 against 0.67 on 12 cells) because most of its tests are the generators'; 64 refusals hand-classified 49 command misused (13 a lambda over a page's bound parameters), 8 body, 6 command missing (an `App` message, a prop on `Layout.Page`, a form on a message with parameters), 1 `alder fill` hang on `×`; `--json` and `--dry-run` never used; `e11-cli/README.md`, `e11-cli/findings.md`.

## Harness (M2)

`run_loop.py` closes the loop: generate, run `alder check --json`, feed the diagnostics back, retry.

```
python3 experiments/run_loop.py --exp experiments/e5-loop --model ollama:qwen3-coder:30b --task t3-newapp [--retries 1] [--prompt prompt.md] [--alder path/to/alder] [--base examples/todos] [--build [--test [--mutate [--feedback N]]] [--hidden DIR]] [--migrate] [--adapters] [--num-ctx 32768] [--condition LABEL] [--sample LABEL]
python3 experiments/report_loop.py experiments/e5-loop > experiments/e5-loop/scores.md
python3 experiments/report_loop.py experiments/e8-hidden/with experiments/e8-hidden/without   # several directories: one table
```

**Warning: `--adapters` executes Go code the model wrote, on this machine, with your permissions, with no sandbox** (below).

`--model` (`ollama:NAME` or `claude:NAME`) and `--task` are repeatable. Each run writes `runs/<model>/<task>/round<N>/{output.md,check.json,app/...}` and `runs/<model>/<task>/summary.json`. Round N+1 starts from round N's files, overlaid with whatever the model re-emits. `--base DIR` seeds round 0 with `DIR/app`, for tasks that modify an existing app (T2 uses `--base examples/todos`): `DIR/app` *as committed*, and with it `DIR/adapters` when that commit has one (since E7: the Go adapters `alder build` and `alder test` need for an app with capabilities, e.g. the example's `Password`; every round carries them, the model writes only `app/`) and `DIR/migrations` when it has one (since M5b: the migrations an app with a database keeps its tables by; every round starts from the base's `migrations/`, never a previous round's, so a round's own migration is that round's change). DIR must be inside a git repository; the harness resolves HEAD of that repository once, exports `git archive <that commit> -- <DIR>/app [<DIR>/adapters] [<DIR>/migrations]` into a temporary directory once per invocation and copies round 0 from there, so uncommitted edits and untracked files in the working tree never leak into a run, and the summary records `base` (DIR as given) and `base_commit` (the commit exported). With `--base`, round 0's prompt is the task followed by `====================  THE APP YOU ARE CHANGING  ====================` and every `app/**/*.alder` of the export in the retry prompt's `--- file: … ---` form (in an `--adapters` run, then the base's adapters apart, as provided); retries are unchanged (they carry the round's files), and without `--base` round 0 is the task alone (since E8; E7's and earlier runs' prompts were the task alone). When `DIR/app` has uncommitted changes the harness says so on stderr ("warning: uncommitted changes under DIR/app are ignored …") and runs on. A DIR that does not exist exits 2 ("--base DIR does not exist"), and so does one outside a git repository: "`--base` must be inside a git repository (the harness seeds from `git archive HEAD`, never the working tree)". The retry prompt carries the task, the previous round's files in full (`YOUR PREVIOUS ANSWER`), and the diagnostics; every model call is a fresh process, so without the files a retry is a re-roll (E5's first run).

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

**`--adapters`** (opt-in, off by default). **It executes Go code the model wrote, on this machine, with your permissions, with no sandbox: use it only on a machine and in a directory you are prepared to lose.** With it, `adapters/<dir>/<name>.go` sections (one package level, as the book's chapter 7 names an adapter: capability `X` of module `A.B` is `adapters/a/`, `func BX()`) and `adapters/internal/<dir>/<name>.go` sections (a helper package the adapters share, the book's layout) are accepted (same path safety; the removed flat `adapters/<name>.go` and anything deeper are refused) are written into the round's `adapters/`, shown to the model in the retry prompt, and `alder build` / `alder test` compile and run them; the summary records `harness.adapters: true`. A Go file that does not vet is an ordinary failed round (`alder build` and `alder test` exit 1). With a `--base` that carries adapters, the provided ones are in every round too, and the model can overwrite them (a section with the same path replaces the base's file); the retry prompt lists an adapter the round still has unchanged from the base apart, after the model's files, under "These adapters are provided, not yours (from the base app); emit one only to replace it …", and one the model rewrote with its files. Without `--adapters` a `--- file: adapters/<dir>/<name>.go ---` section is refused like any non-`.alder` file, and the retry prompt adds a notice: when the base provides that very adapter, "`adapters/auth/password.go` was not accepted (this run does not take Go files); the `Password` adapter is provided — keep the capability and do not replace it with a password in the code" (E7: Haiku's T3 pass replaced a rejected adapter with a password in the code); when it provides others but not that one, "`adapters/auth/hash.go` was not accepted (…); the `Password` adapter is provided; `Hash` is not: declare the capability and leave its Go side unwritten"; with no adapters at all, "… declare the capability and leave its Go side unwritten" (a rejected helper under `adapters/internal/` or `_test.go` file gets no notice, and neither counts as a provided adapter).

**`--hidden DIR`** (E8; requires `--build`, otherwise a usage error, exit 2; and `DIR/<task>/*.alder` must exist for every `--task`). After the run's **final round** — the round that passed, or the last retry — the harness runs hidden acceptance tests the model never sees, if that round checked clean and built (with `--test`, whether or not its own tests passed). It makes a scratch copy of the round, `round<N>/hidden/` (`app/`, and the round's `adapters/` and `migrations/` when it has them: with `--base` the provided adapters, and with `--migrate` the base's migrations plus the round's own, so `alder test` runs through them as it did for the round), copies `DIR/<task>/*.alder` into its `app/` (the hidden module is `app/Hidden.alder`, `module Hidden`; the round's own files are untouched; a model file of the same name is replaced in the copy and listed as `replaced`), and runs `alder check --json` there. A check with an error (a hidden module that does not compile against the program: a renamed page, form or message) fails **every** hidden test, with the first error (`file:line:col: error[CODE]: message`) as `reason`; the tests are counted by reading the `test "…" =` headers of the hidden module, statically, their names kept as the source spells them (escapes included: `alder test --json` reports `the form \"Add\"` so). Otherwise it runs `alder test -run '^Hidden ' --json` in the scratch copy (the program's own tests are not run again) and records each `Hidden` test object. A test that crashes the program, or times out, is reported failed by `alder test` and no later test runs: each statically counted test with no result is added as `{"name", "ok": false, "reason": "not run: <the crash's error>"}` and counted in `failed`, so the denominator is always the module's test count (a crash in the second of four tests scores `1/4`, not `1/2`). An `alder test` that crashes outside any test, exits 2, prints no result, or runs past the harness's 600 s fails every hidden test with its error as `reason` (stderr in `round<N>/hidden/test.stderr.txt`). A hidden directory with no file of `module Hidden` is `{"skipped": "no module Hidden"}`, and a `Hidden` module with no test `{"skipped": "no hidden tests"}`: an authoring error is never a valid empty `0/0`. When the final round did not check clean or build (or there is none), the hidden tests are skipped: `hidden: {"skipped": "<reason>"}`. The hidden module never reaches a prompt: it is copied in only after the last model call. The hidden tests never change `outcome`, `final_pass` or the rounds (those stay the own tests'); the summary records `hidden`, `hidden_pass` (`"k/n"`, `null` when skipped) and `harness.hidden` (DIR as given).

**`--condition LABEL`** records an experimental condition (E8: `with`, `without` the tests paragraph) as `harness.condition`. **`--sample LABEL`** (letters, digits, `_ . -`) runs a repeated sample into its own directory, `runs/<model>/<task>-<LABEL>/`, and records `harness.sample`; `task` stays the task's name and the hidden tests stay `DIR/<task>/`. The harness's `seed` is the base export, not a sampling seed: Claude models sample as they do, and a second sample is a second run.

The E8 fields, on a `--build --test --hidden` run (`with`, sample `s1`); an illustrative record (the test names and counts are made up; the real E8 runs are in `e8-hidden/*/runs/`):

```json
{"model": "claude-sonnet", "task": "t5-ownership", "outcome": "pass", "final_pass": true,
 "harness": {"build": true, "retries": 2, "test": true, "hidden": "experiments/e8-hidden/hidden", "condition": "with", "sample": "s1"},
 "rounds": [{"n": 0, "errors": 0, "build": {"ok": true, "exit": 0, "stderr": ""}, "tests": {"passed": 4, "failed": 0, "results": […], "exit": 0}}],
 "hidden": {"passed": 4, "failed": 1, "exit": 1, "results": [
   {"module": "Hidden", "name": "a stranger's note is not listed", "ok": true, "left": "…", "right": "…", "trace": ["Db.select Note"], "ms": 3.1},
   {"module": "Hidden", "name": "deleting another user's note is not found", "ok": false, "left": "…", "right": "…", "trace": […], "ms": 2.4}, …]},
 "hidden_pass": "4/5"}
```

A hidden module that does not compile against the program: `"hidden": {"passed": 0, "failed": 5, "exit": null, "reason": "app/Hidden.alder:3:8: error[E0002]: …", "check": {"exit": 1, "codes": {"E0002": 1}}, "results": [{"name": "…", "ok": false, "reason": "app/Hidden.alder:3:8: error[E0002]: …"}, …]}`, `"hidden_pass": "0/5"`; a crash: the same without `check`, `exit` the test run's. Skipped: `"hidden": {"skipped": "the final round did not build"}, "hidden_pass": null`. A run without `--hidden` has none of these fields, and `harness` no `hidden`, `condition` or `sample`.

**`--mutate`** (M9; requires `--test`, otherwise a usage error, exit 2; opt-in, since it runs the suite once per mutant). The round whose own tests passed also runs `alder test --mutate --json` (chapter 10, "Mutation testing"), with a 1800 s limit, and the summary (and that round) records `mutation`: `{"score": 0.94, "killed": 44, "survived": 3, "suppressed": 2, "skipped": 5, "survivors": [{"module": "Todos.Index", "file": "app/Todos/Index.alder", "decl": "load", "line": 19, "col": 8, "kind": "orderBy reversed", "before": "Db.orderBy Todo.created.desc", "after": "Db.orderBy Todo.created.asc"}, …]}` (the score over the mutants that ran; `suppressed` the mutants refused as equivalent, `skipped` those an operator does not apply to; each survivor in full, so a feedback condition can point at the code), and `harness` gains `"mutate": true`. A passing round with no tests is a measured outcome, not an error: `"mutation": {"no_tests": true, "score": 0, "killed": 0, "survived": 0, "suppressed": 0, "skipped": 0, "survivors": []}`. A mutation run that does not report a score (exit other than 0, a timeout, no score object) is `"mutation": {"error": "<reason>"}`, its output in `round<N>/mutate.stderr.txt`. It is a measurement: the survivors never reach a prompt (except under `--feedback`, below: E9's design), and `mutation` never changes `outcome`, `final_pass` or the rounds. A run without `--mutate`, or one whose tests never passed, has no `mutation`. E9 may use `--mutate`: as a measurement of the tests a model writes (the examples it is given score 1.00 for `examples/todos` and 0.95 for `examples/blog`, their READMEs list why), and, in a condition of its own, to feed a round's survivors back to the model as the tests it has not written; that feedback is `--feedback`'s (below). The score alone can be gamed: a model can raise it by deleting the code its tests do not cover (fewer mutants, the same kills). E9's design answers that, not the harness: hidden tests (`--hidden`) judge the behaviour, and the app's code is frozen or diffed against the round before, so a score only counts for a program that still does the task.

**`--feedback N`** (E9; requires `--test --mutate`, otherwise a usage error, exit 2; N ≥ 1). After the **first passing round** (own tests pass, built, `mutation` recorded) whose mutation has survivors (`survived > 0`, not `no_tests`, no `error`), the harness runs up to N **feedback rounds**. Each feedback round's prompt is the task, then (under `MUTATION FEEDBACK`) the ruled text, verbatim,

> Your tests pass, but S of N mutants survived (score X.XX). Each line below is a change to your app that no test noticed. Add a test, in the named module, that fails on it. Do not edit app code to remove a mutant. Only if no input can tell a survivor apart, say why in one comment.

then at most 10 survivor lines in `alder test --mutate`'s text format (`  Module   Decl:Line   kind   before → after`, columns aligned, `Decl:Line:Col` when several survivors of one kind share a line, an arm swap elided at 120 columns), grouped by module, the module with most survivors first, `  (and K more)` when cut; then the last accepted round's files under `YOUR PREVIOUS ANSWER` as in any retry, and "Output every file again, in full, …". The round is written over the last accepted round's files (never a rejected round's).

**The frozen-app rule.** Only `test` and `fake` declarations may change; put helpers inside the test. When it checks clean, the harness runs `alder dump --ir <round>`, strips the top-level `(test …)` and `(fake …)` forms (a fake is test-only code, never in a production build; and the `(module M)` headers, so a new module holding only tests is allowed; every other form names its module), hashes the rest (sha256, recorded as the round's `app_ir_sha256`) and compares it with the passing round's. A mismatch **rejects** the round: `"rejected_reason": "app code changed in a feedback round: only tests may change"`, no build, no tests, no mutation run; the next feedback round (if any remain) is prompted again from the last accepted round, with "Your previous answer changed app code; it was not accepted. Only `test` and `fake` declarations may change." before the ruled text. Anything else outside a `test` or a `fake` changes the IR (a helper value, for one), so it is rejected too. A feedback round that checks, builds, passes its tests and keeps the app is **accepted**: `--mutate` runs again, its `mutation` becomes the summary's, and its score `score_after`; zero survivors ends the feedback early. A feedback round that fails (it does not check, build or pass its tests) gets **one retry**, within the N: the retry prompt is the main loop's retry (the round's files, then its diagnostics, build output or test output under `COMPILER FEEDBACK`) with the same ruled text and survivor block after the task (no new score exists), written over the failed round; a second failure in a row ends the feedback (`feedback_end: "a feedback round failed twice"`) and the cell keeps the last accepted score. A mutation run that reports no score ends the feedback at once. A compiler bug, harness error or model error in a feedback round also ends the feedback without changing `outcome` (the pass stands); `feedback_end` says which. A tautology (a test that fails on nothing) is accepted and leaves the score where it was.

`--hidden` runs once, after the final round, on the **last accepted** round (`hidden_round`): a feedback round changes tests only, so hidden results cannot change. The summary of a `--feedback` run (illustrative; round 1 changed the app, round 2 added a test that killed one survivor):

```json
{"harness": {"build": true, "retries": 2, "test": true, "mutate": true, "feedback": 2, "hidden": "experiments/e9-feedback/hidden"},
 "outcome": "pass", "final_pass": true,
 "rounds": [{"n": 0, "errors": 0, "build": {"ok": true, …}, "tests": {…}, "mutation": {"score": 0.7, "killed": 7, "survived": 3, …}, "app_ir_sha256": "252b…"},
            {"n": 1, "errors": 0, "app_ir_sha256": "8a56…", "rejected_reason": "app code changed in a feedback round: only tests may change", "feedback": 1},
            {"n": 2, "errors": 0, "app_ir_sha256": "252b…", "build": {"ok": true, …}, "tests": {…}, "mutation": {"score": 0.8, "killed": 8, "survived": 2, …}, "feedback": 2}],
 "mutation": {"score": 0.8, "killed": 8, "survived": 2, …},
 "score_before": 0.7, "score_after": 0.8, "feedback_rounds": 2, "feedback_end": "feedback rounds exhausted",
 "hidden": {…}, "hidden_pass": "5/5", "hidden_round": 2}
```

`score_before` is the first passing round's score, `score_after` the last accepted round's (both `null` when no round passed or its mutation run failed; 0 with `no_tests`), `feedback_rounds` the feedback rounds run (rejected and failed ones included), `feedback_end` one of `no passing round`, `the mutation run failed`, `no tests`, `no survivors`, `feedback rounds exhausted`, `a feedback round failed twice`, `compiler bug (<kind>)`, `harness error: <reason>`. A cell makes at most `retries + 1 + N` model calls: up to `retries + 1` until the first pass, then at most N (feedback rounds and their retries alike).

**`--cli`** (E11; implies `--build --test --mutate`; a usage error with `--feedback` or `--adapters`). The model may not write a file: each answer must hold exactly one fenced block tagged `alder-cli` (or `sh`) with a script in the drill format of `testdata/drill/*/script.txt` — one `alder …` command a line, `#` comments and blank lines skipped, `'…'`/`"…"` quoting without escapes, a here-document (`<<'EOF'` … a line `EOF`) as the command's standard input; `parse_script` ports `cmd/alder/drill_test.go`'s `parseDrill` and `drillWords` (not `shlex`; they agree on both drills, which the tests check). The round's app is `round<N>/<app>/`: round 0 runs `alder new <app> --bare` in `round0/` (the app named by the task, `t5-ownership` → `ownership`; recorded in `cli.setup`), or with `--base` copies the export's `app/`, `adapters/` and `migrations/` there (T2 and T7 change an existing app); round N+1 starts from a copy of round N's (without `build/`, `hidden/` or databases). The harness runs the commands in order with the `--alder` binary in the app's directory (600 s and a process group each, at most 80 a round), stops at the first that fails, and records `round<N>/script.txt` (the block as received), `commands.jsonl` (per command: `argv`, `stdin_lines`, `exit`, `seconds`, `stdout`, `stderr`, `line`) and `transcript.md`. Refusals end the round's commands: `not-a-command` (a line that is not `alder …`, or `alder run`/`alder repl`), `parse` (no script block or more than one, an unterminated quote or here-document), `exit1`/`exit2` (the command's exit code, its first output line as the reason), `timeout`, `crash`, `cap`; `alder` exiting 2 with "internal error" is a compiler bug (kind `command`). The app is then scored as any round (check, `--migrate`, build, test, mutate on a pass, hidden tests once on the final round), and the round is a pass candidate only when every command of its script ran with exit 0: a round cut short by a refusal never passes, even when the partial app checks and its tests pass, and the next round (within the budget) continues it, its transcript ending "Your script stopped at command N; the app is as the commands before it left it; continue." A run whose last round was cut short is scored on that round's app and records `cli.complete: false`. A failed round's retry is the task, the rule sentence ("Build it with `alder` commands only; you may not write a file."), the format page (`--cli-page FILE`, default `EXP/prompt-cli.md`), with `--base` the app as given, then every round's `transcript.md` so far (each command as written, its output, the refusal, the harness's verdict: check errors, build or test output) and "Continue from here: the app is as the commands left it; send the next script." The summary records `harness.cli: true` and `cli`: `{"app_name", "rounds", "complete", "stopped_at": null | {"round", "command", "line", "kind"}, "commands", "executed", "refused": {"count", "kinds": {kind: n}, "items": [{"round", "kind", "line", "text", "reason"}]}, "stdin_lines", "max_body", "decl_uses", "names", "setup"}`; each round its own `cli` (`fence`, `commands`, `executed`, `refused`, `complete`, `stopped_at`, `stdin_lines`, `max_body`, `decl_uses`). `names` is the drill's names check, ported to the task text (`names_check`): the Names block's tables and their columns and the session exactly (no table is checked when the block lists none, T7), each named page's path, messages and forms present; `{"ok", "matches", "mismatches"}`, `null` for a task without a Names block; reported, never scored. `report_loop.py` adds the columns Cmds, Refused, Stdin, Max body, --decl, Names.

`compiler_bug: true` appears only when the compiler failed on a clean program (see Outcomes: a `--build` or `--test` exiting 2, a checker panic, a hang). Summaries from before M3d have no `harness` key.

#Rounds seeded with `--base` carry a copy of the base's `adapters/` (so `alder test` can run against the example's capability). Those copies are not committed with the runs (`runs/**/adapters/` is ignored; they are reproducible from `base_commit`). `experiments/go.mod` makes this directory a Go module of its own, so Go files inside run output never reach `go vet ./...` or `go test ./...` at the repository root.

## Outcomes

Since M4c every summary has `outcome`, one of four:

- **`pass`**: `alder check` exited 0 with no errors in some round and, with `--build`, that round's build succeeded (`final_pass: true`).
- **`fail`**: every round had errors (or a build that refused the program, exit 1), and the retries ran out. A check that exits 2 for another reason (no files, an I/O error alder reports) is an ordinary failed round too: it is alder answering.
- **`compiler_bug`**: the compiler failed on the model's program; `compiler_bug: true` as before, and `compiler_bug_kind` says how: `build` (a `--build` exited 2 after a clean check: an internal error, or generated Go that does not vet), `internal_error` (`alder check` exited 2 with "internal error" on stderr: a checker panic; its stderr is kept in `round<N>/check.stderr.txt`), `migrate` (a `--migrate` run's `alder migrate` exited 2 with "internal error"), `test` (a `--test` run's `alder test` exited 2 after a clean check and build: a build problem in the test program or a crash outside any test; the app's adapters, refused or not compiling, are exit 1 and the model's (above); its stderr is kept in `round<N>/test.stderr.txt`) or `hang` (`alder check`, `alder build` or `alder test` ran past its limit, 600 s / 900 s / 600 s, or `alder test` exited 2 saying the tests timed out: a hang is the compiler's, and the harness kills its whole process group, `go` children included). The task stops, no retry. A test that fails, crashes or times out *inside* the tests (`alder test` exit 1) is the model's: an ordinary failed round.
- **`harness_error`**: no verdict on the model's program, because the harness itself failed: `alder check`, `alder build` or `alder test` could not be started, or exited with a code alder never uses (not 0, 1 or 2: a process killed by a signal, which Python reports as a negative code, or a wrapper's 137 for SIGKILL); or the model was unavailable (`error` is then set as before). The reason is in `harness_error`; the task stops there, with no retry, and `run_loop.py` exits 1.

`final_pass` and `compiler_bug` keep their meaning for older readers. Summaries from before M4c have no `outcome`; `report_loop.py` derives their result as before.

Scores are now generated by `report_loop.py`: one column per round (`r0 … rN`, each `errors` or `errors (build ✓|✗)`), then — only when some run used `--test` — `Tests` (for each round that ran tests, `rN ✓ N` when all N passed or `rN ✗ M/N` when M of N failed, e.g. `r0 ✗ 1/3; r1 ✓ 3`), then `Result` (pass / fail / compiler-bug / harness-error: compiler-bug with "(hang)" or "(check panic)" from `compiler_bug_kind`, harness-error from `outcome`; kind `test` is plain compiler-bug). E8 adds, only when a summary has the field: `Hidden` (before `Result`: `k/n` hidden tests passed, `0/n (check: E0002)` when the hidden module did not compile against the program, `0/n (reason)` for a crash, `— (skipped: reason)`), `Condition` (first; the rows grouped by `harness.condition`, each condition in the order it first appears, `—` for a summary without one), and `harness.sample` folded into the task as the run directory shows it (`t5-ownership-s1`). M9 adds `Mut` (after `Hidden`, before `Result`) only when a summary has `mutation`: `k/n` mutants killed of those that ran, `— (error)` for a failed mutation run, `—` for a row without one. E9: when a summary has `harness.feedback`, `Score` takes `Mut`'s place: `0.70 → 0.80` (`score_before → score_after`) for a `--feedback` cell, the score alone (`0.70`) for a cell without, `0 (no tests)`, `— (error)` or `—`; a rejected feedback round's column reads `0 (app changed)`. `report_loop.py` takes several experiment directories and makes one table of them. Summaries without `harness` (E5) keep the original two-round table, and E5's and E6's `scores.md` (and each of E7's per-condition tables) regenerate byte-identical; hand-scoring is reserved for design signals the checker cannot see. Tests: `cd experiments && python3 -m unittest -v` (`test_run_loop`, `test_build_context`).

## The prompt (`build_context.py`)

```
python3 experiments/build_context.py [--chapters 03,04,05,06,07,08,09,10] [--no-example] [--reference] > prompt.md
python3 experiments/build_context.py [--reference] --size [--count ollama:qwen3-coder:30b [--num-ctx 65536]]
```

The prompt is a fixed header, the chapters (default 3–10) and the example app (`examples/todos/app`). `--reference` (E7) drops the `## ` sections written for a human reader, listed by exact heading in `HUMAN_SECTIONS`: ch.6 "How a request runs"; ch.7 "How effects run", "Implementing a capability"; ch.8 "How tables are stored" (its "Migrations" stays: the E0019 → `alder migrate` workflow is for agents too); ch.10 "2. Running the tests", "6. What a trace says", "7. Choosing tests, and output for programs", "10. How the browser works", "11. What cannot be tested yet" (its "8. Faking a capability" and "9. Testing through the browser" stay: agents write fakes and browser tests). `KEEP_SUBSECTIONS` keeps named `### ` subsections of a dropped section (ch.7's "What a capability looks like in Go" and "The rule"); `DROP_SUBSECTIONS`, its inverse, drops named `### ` subsections of a kept one: from ch.10 §9 the walkthrough of the first example, the `Test.get`/`Test.open` prose, the further form examples, the bad-input example, the refused click and the ownership narrative, the refused selector, the comparison with a view and the case for browser tests at all; from ch.9's "Test" the browser steps' and queries' signatures, which §9 repeats. The reference keeps §9's step table, one complete example, `Answer`, the login, the encoding of a record, `Rejected`, the toggle and stranger examples, the queries, every failure text and the rules. A section runs from its heading to the next `## ` heading (one inside a code fence does not count); the chapter's text before its first `## ` and every other section are kept, in order. A listed heading the chapter no longer has exits 2 naming the chapter file and the heading, so a renamed section never silently builds the wrong prompt. `--size` prints the size instead of the prompt: `characters: N` and `estimate (len/4): N`; `--count ollama:MODEL` (implies `--size`) adds `prompt_eval_count (ollama:MODEL, num_ctx N): N`, Ollama's own count for the prompt alone (one `/api/chat` call with the prompt as the system message and `num_predict: 1`, at `--num-ctx`, default 65536, large enough not to truncate it; a count that reaches it warns). `--book DIR` and `--example DIR` replace the book and the example's `app/` directory (for tests).
