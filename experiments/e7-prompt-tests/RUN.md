# E7 run log

## Binary and prompts

- Commit: `1de748d` (branch `e7-prompt-tests`; working tree clean).
- Export: `git archive HEAD | tar -x -C /private/tmp/claude-501/-Users-tavishpegram-projects/39e6edf3-2ea1-4a0a-800c-8eb40238cb77/scratchpad/alder-1de748d`
  (a session scratchpad, temporary: every generated `go.mod` under `*/runs/**/round<N>/build/` replaces `alder =>` that path;
  to vet one again, point it at a checkout of `1de748d`).
- Build (in the export): `go build -o experiments/e7-prompt-tests/alder ./cmd/alder` (go1.27.1 darwin/arm64; 2026-10-10 13:12;
  sha256 `e4dca5e969b0c89893ed09e2941cb1b58b376c95126039adae0586f35ddd5ccd`; git-ignored).
- Prompts (from `experiments/`): `python3 build_context.py > e7-prompt-tests/prompt-full.md`;
  `python3 build_context.py --reference > e7-prompt-tests/prompt-reference.md`;
  `python3 build_context.py [--reference] --size --count ollama:qwen3-coder:30b` → `sizes.md`. Committed in `097185c`
  (the prompts, `sizes.md`, `.gitignore`, the `tasks` symlinks); the matrix ran on that commit (no compiler, book or harness change since `1de748d`).

## Layout

`run_loop.py` has no condition label, and `--base` seeds every `--task` of an invocation, so (as E6):

- two experiment directories, `reference/` and `full/`, each with `tasks -> ../tasks` (the harness reads `<exp>/tasks/<task>.md`);
  runs in `<cond>/runs/<model>/<task>/`;
- per condition, T2 alone with `--base examples/todos`, then T3–T6 without.

## Commands

`logs/run.sh <condition> <num_ctx> <model>...` (git-ignored, copied here), from the repository root:

```bash
E=experiments/e7-prompt-tests
python3 experiments/run_loop.py --exp $E/$cond --model … --task t2-newpage --retries 2 \
  --prompt $PWD/$E/prompt-$cond.md --alder $E/alder --base examples/todos --build --test --num-ctx $ctx
python3 experiments/run_loop.py --exp $E/$cond --model … --task t3-newapp --task t4-appmsg --task t5-ownership --task t6-orders \
  --retries 2 --prompt $PWD/$E/prompt-$cond.md --alder $E/alder --build --test --num-ctx $ctx
```

Two background processes, started together:

- Qwen: `run.sh reference 32768 ollama:qwen3-coder:30b`, then `run.sh full 65536 ollama:qwen3-coder:30b` (sequential, one Ollama).
- Claude: `run.sh full 65536 claude:haiku claude:sonnet` (`--num-ctx` is ignored by Claude models).

(The `exit $?` the script echoes after each step reports `date`'s status, not the harness's: a script slip, not used.)

## Timeline (2026-10-10, local time)

| What | Start | End | Wall-clock |
|---|---|---|---|
| Claude, `full`: Haiku + Sonnet T2 | 13:14:49 | 13:17:04 | 2 min 15 s |
| Claude, `full`: Haiku + Sonnet T3–T6 | 13:17:04 | 13:33:22 | 16 min 18 s |
| Qwen `reference` T2, attempt 1 | 13:14:48 | 14:14:48 | 60 min (timed out) |
| Qwen `reference` T3–T6 | 14:14:48 | 14:30:02 | 15 min 14 s |
| Qwen `full` T2, attempt 1 | 14:30:02 | 15:30:03 | 60 min (timed out) |
| Qwen `full` T3–T6 | 15:30:03 | 15:50:53 | 20 min 50 s |
| Qwen `full` T2, attempt 2 (no cap) | 15:51:14 | 16:01 (killed) | 10 min, no round |
| Diagnostics (streamed T2 request, `full`, 4,096 and 12,000 tokens) | ~16:02 | ~16:10 | 2 min 8 s + 6 min 45 s |
| Qwen `reference` T2, attempt 2 (no cap, after `ollama stop`) | 16:10:57 | 16:21:07 (killed) | 10 min, no round |
| Harness change `0c80d80` (`num_predict` 8192), committed | | 16:22 | |
| Qwen `reference` T2, attempt 3 (capped, after `ollama stop`) | 16:22:17 | 16:28:52 | 6 min 35 s |
| Qwen `full` T2, attempt 3 (capped) | 16:28:52 | 16:37:15 | 8 min 23 s |

Scored cells: Claude 10 runs in 18 min 33 s; Qwen `reference` 5 runs in 21 min 49 s (T3–T6 + T2 attempt 3),
Qwen `full` 5 runs in 29 min 13 s (T3–T6 + T2 attempt 3).

**Overlap.** The Claude and Qwen processes were started together (as in E6) and overlapped from 13:14:48 to 13:33:22.
During that window Qwen's only activity was `reference` T2 attempt 1 (the runaway request); every scored Qwen round ran
after 14:14:48 with nothing else running. The overlap is a confound for that attempt's timing only, not for any
outcome or scored timing, and it did not cause the timeout (attempts 2 ran alone and stalled the same way; the
diagnostics show why). From 13:33:22 on, Qwen ran alone.

## Harness notes

- **Ollama** `ollama version is 0.35.0`; `qwen3-coder:30b` (Q4_K_M, 30.5B). Claude `2.1.295 (Claude Code)`.
- **T2 never ended (Qwen, both conditions).** Attempt 1 of each condition is `harness_error`
  ("model: ollama qwen3-coder:30b: timed out", the harness's 3600 s HTTP timeout, no round written). The coordinator's
  first diagnosis was that both stalls were the first request after a `num_ctx` change (an Ollama reload hang);
  **that diagnosis was wrong**: `full` attempt 2 ran with the model already loaded at 65536 and `reference` attempt 2
  ran after `ollama stop` and a fresh load, and both produced no round in 10 minutes (killed), while llama-server was
  busy throughout. A streamed copy of the exact T2 round-0 request (`repros/full-ollama-qwen3-coder-30b-t2-newpage/`:
  `diag.py`, its 4,096- and 12,000-token outputs) showed the cause: Qwen streams at ~30 tokens/s, re-emits every file
  of the example app plus `static/app.css`, then invents `adapters/password.go`, `cmd/main.go`, `go.mod`, `README.md`,
  `alder.toml`, `test.sh`, `Makefile`, `Dockerfile`, `.gitignore`, `.github/workflows/test.yml`, `.golangci.yml`
  and repeats a list of linter names with no end (temperature 0, no `num_predict`: Ollama's default is unlimited, and
  it shifts the context instead of stopping).
- **Deviation: harness change mid-run** (coordinator's ruling, applied uniformly): `0c80d80` makes every Ollama call
  send `num_predict` 8192 (recorded as `harness.num_predict`), records `done_reason` per round, and makes a
  `done_reason: "length"` round an ordinary failed round with `truncated: true` (no build/test); the retry prompt adds
  "Your previous answer was cut off after 8192 tokens; output only the Alder files (`--- file: app/<path> ---`),
  nothing else." Unit test `test_ollama_length_is_truncated_failed_round`; `python3 -m unittest` 58 tests OK.
  8192 is above every completed Ollama round in E6 and E7 (the largest is E7 `full` T6 r0, 5,393 tokens), so no other
  cell would have changed; only T2 was re-run with it (both conditions). T3–T6 ran on `1de748d`'s harness.
- **The capped T2 runs.** Both r0s were cut at 8192 tokens. `reference`: r0 wrote 56 `--- file` sections (33 `.alder`
  files after overlay), 194 errors, unchanged in r1 and r2; r1 and r2 carried truncation warnings
  (`prompt_eval_count` 16,386 against ~31.8k estimated: the retry prompt carries the whole re-emitted app and does not fit
  32,768). `full`: r0 wrote 18 sections (rejected `static/app.css` ×3, `adapters/password.go`), 5 parse errors (E0001,
  `Profile.alder` and a cut-off `Todos/Show.alder`), unchanged in r1 and r2.
- **Design signal for findings (Task 3):** E6's Qwen T2 round 0 was 1,743 tokens (6 file sections), r1 1,216. In E7 the
  same task, plus the tests paragraph, under the larger prompts, made Qwen re-emit the whole example app and then
  non-Alder files ("Output every file you add or change" read as "every file"). Claude models did not.
- **Kept attempts:** the first attempts' summaries are `*/runs/ollama-qwen3-coder-30b/t2-newpage/summary.harness_error.json`
  (also under `repros/`); `report_loop.py` reads only `summary.json`, so the scored cell is attempt 3. Attempts 2
  left no summary (killed; the harness removes the run directory at start).
- **Truncation/overflow warnings:** only `reference` T2 attempt 3 r1 and r2 (above). No `full` round warned at 65,536
  (largest `full` round: 36,764 + 3,178). Largest `reference` round without a warning: T6 r2, 27,426 + 3,383 = 30,809.
- **Compiler bugs:** none. No Qwen round reached a clean check, so no Qwen build or test ran.
- **Model errors / re-runs of Claude:** none; each Claude cell ran once.
- **Log slip:** `run.sh` echoes `exit $?` after `$(date)`, so it shows 0 even after a harness_error (run_loop exits 1).
  The harness's stdout is block-buffered into the logs, so outcome lines appear late; summaries are authoritative.
