# E9 run log

## Binary and prompts

- Commit: `5689a70` (branch `e9-feedback`, the tip after Tasks 1–3; working tree clean).
- Export: `git archive HEAD | tar -x -C /private/tmp/claude-501/-Users-tavishpegram-projects/39e6edf3-2ea1-4a0a-800c-8eb40238cb77/scratchpad/alder-5689a70`
  (a session scratchpad, temporary: every generated `go.mod` under `*/runs/**/build/` replaces `alder =>` that path;
  to vet one again, point it at a checkout of `5689a70`).
- Build (in the export): `go build -o experiments/e9-feedback/alder ./cmd/alder` (go1.27.1 darwin/arm64; 2026-10-12 09:04;
  sha256 `97b9f629b13ba7d0abc228d06ba7eb055cccb3059a6b112e1654a400b2c3cb0e`), copied to the same path in the
  repository (git-ignored). It includes Task 1's codegen fix (`6362f24`, `c8012bd`).
- Gate before the run: `ALDER=$PWD/experiments/e9-feedback/alder experiments/e9-feedback/verify.sh` green with that
  binary (every reference and alt: check, build, own, hidden all pass; scores 0.944–1.000; the T7 survivors the blog's
  three equivalents).
- Prompt (from `experiments/`): `python3 build_context.py > e9-feedback/prompt-full.md`; `python3 build_context.py --size`
  → `sizes.md` (250,290 characters, len/4 62,572). Claude only, so no `reference` prompt and no Ollama count.
  Committed in `5b46f6c` with `sizes.md`, `.gitignore`, `logs/run.sh`, `logs/matrix.sh` and the `tasks` symlinks;
  the matrix ran on that commit (no compiler, book, harness or material change during the run).
- Claude `2.1.295 (Claude Code)`.

## Layout

Two experiment directories, `baseline/` and `feedback/`, each with `tasks -> ../tasks`; runs in
`<cond>/runs/<model>/<task>-<sample>/` (`--sample`). `--base` and `--migrate` apply to a whole invocation, so
`logs/run.sh` calls the harness once per task.

## Commands

`logs/run.sh <condition> <sample> <model>` (committed), from the repository root, per task:

```bash
E=experiments/e9-feedback
python3 -u experiments/run_loop.py --exp $E/$cond --model $model --retries 2 --prompt $PWD/$E/prompt-full.md \
  --alder $E/alder --build --test --mutate [--feedback 2] --hidden $E/hidden --condition $cond --sample $sample \
  --task $task [--base experiments/e8-hidden/base/blog-nodrafts --migrate]
#   --feedback 2: the feedback condition only
#   --base … --migrate: t7-drafts only (t5-ownership, t6-orders, t9-bookings, t10-teams: no base)
```

`logs/matrix.sh` (committed) ran the invocations one at a time in the brief's order (Haiku s1 baseline, feedback; s2
baseline, feedback; then Sonnet likewise), each to `logs/<cond>-<sample>-<model>.log` (git-ignored). At 11:05, with
`feedback s2 claude:sonnet` running, the coordinator ruled that Opus should start at once, alongside it: the driver
(`matrix.sh`, pid 5134) was killed with its running child left alone (so `feedback s2 claude:sonnet` finished
normally), and `logs/opus.sh` (committed) ran `baseline s1 claude:opus` then `feedback s1 claude:opus`. The cells
write to separate run directories; at most two Claude invocations overlapped (Opus and Sonnet, 11:05–11:25).

## Timeline (2026-10-12, local time; from the logs' start/end lines)

| Invocation | Start | End | Wall-clock |
|---|---|---|---|
| baseline s1 claude:haiku | 09:05:24 | 09:18:25 | 13 min 1 s |
| feedback s1 claude:haiku | 09:18:25 | 09:46:33 | 28 min 8 s |
| baseline s2 claude:haiku | 09:46:33 | 09:58:33 | 12 min 0 s |
| feedback s2 claude:haiku | 09:58:33 | 10:25:37 | 27 min 4 s |
| baseline s1 claude:sonnet | 10:25:37 | 10:33:10 | 7 min 33 s |
| feedback s1 claude:sonnet | 10:33:10 | 10:56:03 | 22 min 53 s |
| baseline s2 claude:sonnet | 10:56:03 | 11:04:46 | 8 min 43 s |
| feedback s2 claude:sonnet | 11:04:46 | 11:25:41 | 20 min 55 s |
| baseline s1 claude:opus (concurrent) | 11:05:07 | 11:16:23 | 11 min 16 s |
| feedback s1 claude:opus | 11:16:23 | 11:44:37 | 28 min 14 s |

50 cells from 09:05:24 to 11:44:37 (2 h 39 min). The longest cell took 9 min 4 s (Haiku T10 feedback s2, 4 rounds,
4 mutation runs); no cell came near the 25-minute "stuck" line. A `feedback` invocation took 2.2–3.0× its
`baseline` (more model calls, one mutation run per accepted round).

## Harness notes

- **All 50 cells have a `summary.json`**, all `outcome: pass`. No `harness_error`, no `compiler_bug`, no model error,
  no rate-limit or account failure (no round has stderr; the logs carry none), no stuck cell, no re-run. `repros/` is
  therefore not created.
- **Every cell's first passing round recorded a mutation score** (no `no_tests`, no mutation `error`). Every
  `feedback` cell had survivors at its first pass, so all 25 ran feedback rounds (`feedback_rounds` 1 or 2).
- **Feedback rounds:** 45 run in 25 cells; 39 accepted, 1 rejected by the frozen-app check, 5 failed (4 at check,
  1 at its own tests); `survivors.md` §3. The failed-round retry fired 3 times (Haiku T6 s2, Haiku T10 s1,
  Sonnet T6 s2); one passed (Haiku T10 s1), two failed again (`a feedback round failed twice`).
- **The hidden copies.** Each final (last accepted) round has `hidden/` (`app/` with `Hidden.alder`; T7 also the base's
  `migrations/` and the round's own); its `build/` and `adapters/` are git-ignored, `app/` is committed. In the
  feedback cells `hidden_round` is the last accepted round; the hidden tests ran once per cell.
- **Analysis scripts (committed, not part of the run):** `logs/extract.py` (cells, survivors, kinds, fedback,
  claims), `logs/attribute.py` (each fed-back test alone under mutation on its round → `logs/attribution.jsonl`),
  `logs/operators.py` (per-operator kill rates over the final rounds → `logs/operators.md`; its re-run of every
  final round agreed with every summary's `killed`), `logs/survivors.py` (`survivors.md`'s generated sections),
  `logs/scores.sh` (`scores.md`).

## Deviations

1. **Opus ran concurrently with Sonnet's last invocation** (`feedback s2 claude:sonnet`), not after it: the
   coordinator's ruling at 11:05, to save time. No rate-limit error followed (no stderr in any round of either).
2. **The matrix driver** (`logs/matrix.sh`) was stopped after it had started its last Sonnet invocation; Opus ran from
   `logs/opus.sh`. `logs/matrix.log` records the stop.
3. **Commit trailers** are the session's attribution lines (`Co-Authored-By: Claude Opus 5.5`), as in E8 and Tasks 1–3,
   not the brief's "Claude Fable 5.1".
4. **The attribution's survivor identity** is by content (module, declaration, operator, before, after), not by line:
   a feedback round that adds a `fake` or a test above a declaration moves its lines. The first pass of
   `logs/attribute.py` keyed by line and under-counted kills; the committed `attribution.jsonl` is the content-keyed run.
