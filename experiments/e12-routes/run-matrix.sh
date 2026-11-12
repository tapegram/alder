#!/usr/bin/env bash
# run-matrix.sh: the E12 matrix, 13 tasks x 3 models x 2 samples = 78 cells, run overnight.
# Order: sample (s1, s2) -> model (haiku, sonnet, opus) -> task (t2 .. t15).
# Env:  MODELS, SAMPLES, TASKS (space separated) narrow it; STOP_AT (default 0845);
#       E12_CLOCK="<%u> <HHMM>" overrides the New York clock (tests only);
#       DRY_RUN=1 prints each cell's run_loop.py command instead of running it;
#       PREFLIGHT_ONLY=1 runs the preflight and exits.
# Exit: 0 complete, 3 stopped for the weekday window, 2 failed preflight.
# macOS bash 3.2: no associative arrays.
set -u
HERE="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
cd "$ROOT"
E=experiments/e12-routes
RUN_MD="$E/RUN.md"
STOP_AT="${STOP_AT:-0845}"
MODELS="${MODELS:-claude:claude-haiku-5-5 claude:claude-sonnet-5-5 claude:claude-opus-5-5}"
SAMPLES="${SAMPLES:-s1 s2}"
TASKS="${TASKS:-t2-newpage t3-newapp t4-appmsg t5-ownership t6-orders t7-drafts t9-bookings t10-teams t11-workspace t12-search t13-feed t14-board t15-pantry}"
INPUTS="experiments/tasks $E/hidden $E/reference $E/reference-alt $E/base $E/verify.sh $E/run-matrix.sh $E/prompt-reference.md experiments/run_loop.py experiments/report_loop.py experiments/build_context.py examples/todos"

fail() { echo "PREFLIGHT FAILED: $1" >&2; exit 2; }

runmd() { sed -n "s/^$1: *//p" "$RUN_MD" 2>/dev/null | head -1 | tr -d '[:space:]'; }

preflight() {
  [ -f "$RUN_MD" ] || fail "missing $RUN_MD"
  local commit sha
  commit="$(runmd E12_COMMIT)"; sha="$(runmd ALDER_SHA256)"
  [ -n "$commit" ] || fail "no E12_COMMIT line in $RUN_MD"
  [ -n "$sha" ] || fail "no ALDER_SHA256 line in $RUN_MD"
  git rev-parse --verify -q "$commit^{commit}" >/dev/null || fail "E12_COMMIT $commit is not a commit"
  # 1. the binary
  [ -x "$E/alder" ] || fail "binary $E/alder missing"
  [ "$(shasum -a 256 "$E/alder" | awk '{print $1}')" = "$sha" ] || fail "binary $E/alder sha256 differs from ALDER_SHA256 in $RUN_MD"
  tmp="$(mktemp -d "${TMPDIR:-/tmp}/e12-preflight.XXXXXX")"
  trap 'rm -rf "$tmp"' EXIT
  # 2. the checker accepts the two bases, exported from the pinned commit
  local base
  for base in examples/todos $E/base/blog-nodrafts; do
    mkdir -p "$tmp/export"
    git archive "$commit" "$base" | tar -x -C "$tmp/export" || fail "cannot export $base from $commit"
    "$ROOT/$E/alder" check "$tmp/export/$base" >"$tmp/check.log" 2>&1 || { cat "$tmp/check.log" >&2; fail "alder check failed on $base at $commit"; }
    rm -rf "$tmp/export"
  done
  # 3. the prompt is what the pinned commit builds
  bash "$HERE/pin.sh" prompt "$commit" "$tmp/prompt" >/dev/null 2>&1 || fail "pin.sh prompt $commit failed"
  cmp -s "$tmp/prompt/prompt-reference.md" "$E/prompt-reference.md" || fail "$E/prompt-reference.md differs from build_context.py --reference at $commit"
  # 4. the inputs are unchanged since the pin and clean
  local p
  for p in $INPUTS; do
    git diff --quiet "$commit" -- "$p" || fail "$p changed since $commit"
    [ -z "$(git status --porcelain -- "$p")" ] || fail "$p has uncommitted or untracked changes"
  done
}

preflight
[ "${PREFLIGHT_ONLY:-}" = 1 ] && { echo "preflight ok"; exit 0; }

mkdir -p "$E/logs"
PROGRESS="$E/logs/matrix.progress"
note() { echo "$(date '+%H:%M') $*" >> "$PROGRESS"; }

stop_now() {
  local clock u hm
  clock="${E12_CLOCK:-$(TZ=America/New_York date '+%u %H%M')}"
  u="${clock%% *}"; hm="${clock##* }"
  [ $((10#$u)) -le 5 ] && [ $((10#$hm)) -ge $((10#$STOP_AT)) ] && [ $((10#$hm)) -lt 1700 ]
}

for s in $SAMPLES; do
  for model in $MODELS; do
    label="${model//:/-}"
    for task in $TASKS; do
      cell="$s $model $task"
      if stop_now; then
        echo "STOPPED before $cell"
        note "STOPPED before $cell"
        exit 3
      fi
      dir="$E/runs/$label/$task-$s"
      if [ -f "$dir/summary.json" ]; then
        note "$cell skipped"
        continue
      fi
      if [ -e "$dir" ]; then
        rm -rf "$dir"
        note "$cell rerun"
      fi
      cmd=(python3 -u experiments/run_loop.py --exp "$E" --model "$model" --task "$task" --sample "$s"
           --condition files --build --test --mutate --retries 5
           --prompt "$ROOT/$E/prompt-reference.md" --alder "$E/alder")
      case "$task" in
        t2-newpage) cmd+=(--base examples/todos) ;;
        t3-newapp) ;;
        t7-drafts) cmd+=(--hidden "$E/hidden" --base "$E/base/blog-nodrafts" --migrate) ;;
        *) cmd+=(--hidden "$E/hidden") ;;
      esac
      if [ "${DRY_RUN:-}" = 1 ]; then
        echo "$cell :: ${cmd[*]}"
        continue
      fi
      "${cmd[@]}" >> "$E/logs/$label.log" 2>&1
      note "$cell done"
    done
  done
done
echo "matrix complete"
note "MATRIX COMPLETE"
exit 0
