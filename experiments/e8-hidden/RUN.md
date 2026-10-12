# E8 run log

## Binary and prompts

- Commit: `8c0526f` (branch `e8-hidden`; working tree clean).
- Export: `git archive HEAD | tar -x -C /private/tmp/claude-501/-Users-tavishpegram-projects/39e6edf3-2ea1-4a0a-800c-8eb40238cb77/scratchpad/alder-8c0526f`
  (a session scratchpad, temporary: every generated `go.mod` under `*/runs/**/build/` replaces `alder =>` that path;
  to vet one again, point it at a checkout of `8c0526f`).
- Build (in the export): `go build -o experiments/e8-hidden/alder ./cmd/alder` (go1.27.1 darwin/arm64; 2026-10-11 21:00;
  sha256 `4c583e2c1086623fa2d203a5e06baec1e20a3b7d8cccc04adb3602d4a32f504a`), copied to the same path in the
  repository (git-ignored).
- Prompts (from `experiments/`): `python3 build_context.py > e8-hidden/prompt-full.md`;
  `python3 build_context.py --reference > e8-hidden/prompt-reference.md`;
  `python3 build_context.py [--reference] --size --count ollama:qwen3-coder:30b` → `sizes.md`.
  Committed in `87c82d5` (the prompts, `sizes.md`, `.gitignore`, `logs/run.sh`, the `tasks` symlinks); the matrix ran
  on that commit (no compiler, book, harness or material change since `8c0526f`).
- Ollama `ollama version is 0.35.0`; `qwen3-coder:30b`. Claude `2.1.295 (Claude Code)`.

## Layout

As E7: two experiment directories, `with/` and `without/`, each with `tasks -> ../tasks/<cond>`; runs in
`<cond>/runs/<model>/<task>-<sample>/` (`--sample`). `--base` and `--migrate` apply to a whole invocation, so
`logs/run.sh` calls the harness once per task.

## Commands

`logs/run.sh <condition> <sample> <model>...` (committed), from the repository root, per task:

```bash
E=experiments/e8-hidden
python3 -u experiments/run_loop.py --exp $E/$cond --model $model --retries 2 --prompt $PWD/$E/prompt-full.md \
  --alder $E/alder --build --test --hidden $E/hidden --condition $cond --sample $sample --task $task [base flags]
#   t4-appmsg, t5-ownership, t6-orders: no base flags
#   t7-drafts: --base $E/base/blog-nodrafts --migrate
#   t8-notify: --base examples/blog --migrate
#   an ollama: model: --prompt $PWD/$E/prompt-reference.md --num-ctx 65536 (NUM_CTX), TASKS="t5-ownership t6-orders"
```

The first invocation was started by hand (`nohup logs/run.sh with s1 claude:haiku > logs/with-s1-claude-haiku.log 2>&1 &`);
`logs/matrix.sh` (committed) waited for it and ran the other ten, one at a time, each to `logs/<cond>-<sample>-<model>.log`
(git-ignored). One invocation at a time throughout: no two Claude invocations overlapped, and Qwen ran last and alone.

## Timeline (2026-10-11, local time; from the logs' start/end lines)

| Invocation | Start | End | Wall-clock |
|---|---|---|---|
| with s1 claude:haiku | 21:03:56 | 21:12:44 | 8 min 48 s |
| without s1 claude:haiku | 21:12:45 | 21:19:14 | 6 min 29 s |
| with s1 claude:sonnet | 21:19:14 | 21:25:06 | 5 min 52 s |
| without s1 claude:sonnet | 21:25:06 | 21:30:03 | 4 min 57 s |
| with s2 claude:haiku | 21:30:03 | 21:37:06 | 7 min 3 s |
| without s2 claude:haiku | 21:37:06 | 21:43:09 | 6 min 3 s |
| with s2 claude:sonnet | 21:43:09 | 21:47:42 | 4 min 33 s |
| without s2 claude:sonnet | 21:47:42 | 21:51:31 | 3 min 49 s |
| with s1 claude:opus | 21:51:31 | 22:00:42 | 9 min 11 s |
| without s1 claude:opus | 22:00:42 | 22:06:18 | 5 min 36 s |
| with s1 ollama:qwen3-coder:30b (T5, T6) | 22:06:18 | 22:17:13 | 10 min 55 s |

Claude: 50 runs in 62 min 22 s; Qwen: 2 runs in 10 min 55 s. Prompt counting (`--count`) ran from about 21:00 to 21:07
(the `full` count overlapped the first Haiku invocation by a few minutes; Ollama only, no effect on Claude).

## Harness notes

- **All 52 cells have a `summary.json`.** No `harness_error`, no `compiler_bug`, no model error, no stuck cell (the
  longest Claude round was 199 s, Haiku T6 s2), no Claude cell re-run. `repros/` is therefore not created.
- **Qwen rounds** (`num_predict` 8192): T5 r0 42,759 prompt tokens + 1,453 generated; T6 r0–r2 43,318 / 48,047 /
  47,879 prompt tokens, at most 3,119 generated; `done_reason: stop` everywhere, no truncation or overflow warning.
- **Own tests: zero tests pass.** `--test` passes a round with no tests (`r0 ✓ 0`), in either condition: Qwen's `with`
  T5 wrote none and passed. The harness does not enforce the tests paragraph.
- **The hidden copies.** Each final round has `hidden/` (the scratch copy: `app/` with `Hidden.alder`, the base's
  `migrations/` and the round's own, `test.stderr.txt`); its `build/` and `adapters/` are git-ignored, `app/` is committed.
  No model wrote an `app/Hidden.alder` of its own (no `replaced`).
- **Probe after the run** (not part of the score): `logs/probe-t6-stale.alder` showed that the five T6 hidden failures
  are all answered `Rerendered` (findings §2).
- **`run.sh` prints the harness's exit status** (`task … end … exit N`, `$?` taken right after the harness), fixing E7's slip.

## Deviations

1. **Qwen at `num_ctx` 65,536, not 32,768.** The `reference` prompt is now 41,990 tokens (`sizes.md`; E7's was 21,527),
   so at 32,768 Ollama would have truncated the book. Qwen ran at 65,536 (E7's `full` setting), recorded as
   `harness.num_ctx`.
2. **Sample labels `s1`, `s2`** (the brief's), where the E8 README's matrix says `a`, `b`.
3. **Failure classes** are the brief's (`authorization`, `rendering`, `redirect`, `session`, `schema`, `naming`,
   `other`), finer than the README's four (`naming`, `behaviour`, `authorization`, `markup`): `rendering` ⊇ markup,
   `redirect`/`session`/`schema` ⊂ behaviour.
4. **One driver for the matrix** (`logs/matrix.sh`), running the ten remaining invocations one after another in the
   background, each with its own log, instead of ten hand-started `nohup`s; the order is the brief's.
5. **Commit trailers** are the session's attribution lines (`Co-Authored-By: Claude Opus 5.5`), as Tasks 1 and 2 used,
   not "Claude Fable 5.1".
