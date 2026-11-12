#!/usr/bin/env bash
# run-matrix.sh: the E12 matrix, 13 tasks x 3 models x 2 samples = 78 cells, run overnight.
# Order: sample (s1, s2) -> model (haiku, sonnet, opus) -> task (t2 .. t15).
# Env:  MODELS, SAMPLES, TASKS (space separated) narrow it; STOP_AT (default 0845);
#       E12_CLOCK="<%u> <HHMM>" overrides the New York clock (honoured only with DRY_RUN=1 or RUN_LOOP);
#       DRY_RUN=1 prints each cell's run_loop.py command instead of running it, and what it would
#         skip or rerun; it deletes nothing and writes no progress;
#       RUN_LOOP=<script> replaces run_loop.py, E12_DEADLINE_SECS=<n> replaces the seconds until
#         the 09:00 weekday kill (both for tests only);
#       PREFLIGHT_ONLY=1 runs the preflight and exits.
# Exit: 0 complete, 3 stopped for the weekday window (before a cell, or a cell killed at 09:00),
#       2 failed preflight or lock held, 4 a cell ended in harness_error (usage limit, network, auth).
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
INPUTS="experiments/tasks $E/tasks $E/hidden $E/reference $E/reference-alt $E/base $E/verify.sh $E/run-matrix.sh $E/prompt-reference.md experiments/run_loop.py experiments/report_loop.py experiments/build_context.py examples/todos"


TMP=""; LOCK=""
cleanup() { [ -z "$TMP" ] || rm -rf "$TMP"; [ -z "$LOCK" ] || rm -rf "$LOCK"; }
trap cleanup EXIT
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
  TMP="$(mktemp -d "${TMPDIR:-/tmp}/e12-preflight.XXXXXX")"
  # 2. the checker accepts the two bases, exported from the pinned commit
  local base
  for base in examples/todos $E/base/blog-nodrafts; do
    mkdir -p "$TMP/export"
    git archive "$commit" "$base" | tar -x -C "$TMP/export" || fail "cannot export $base from $commit"
    "$ROOT/$E/alder" check "$TMP/export/$base" >"$TMP/check.log" 2>&1 || { cat "$TMP/check.log" >&2; fail "alder check failed on $base at $commit"; }
    rm -rf "$TMP/export"
  done
  # 3. the prompt is what the pinned commit builds
  bash "$HERE/pin.sh" prompt "$commit" "$TMP/prompt" >"$TMP/pin.log" 2>&1 || { cat "$TMP/pin.log" >&2; fail "pin.sh prompt $commit failed"; }
  cmp -s "$TMP/prompt/prompt-reference.md" "$E/prompt-reference.md" || fail "$E/prompt-reference.md differs from build_context.py --reference at $commit"
  # 4. the inputs are unchanged since the pin and clean
  local names
  names="$(git diff --name-only "$commit" -- $INPUTS)" || fail "git diff against $commit failed"
  [ -z "$names" ] || fail "changed since $commit: $(echo $names)"
  names="$(git status --porcelain -- $INPUTS)" || fail "git status failed"
  [ -z "$names" ] || fail "uncommitted or untracked: $(echo $names | sed 's/^...//')"
  # 5. every requested task has its text, and T4 on its hidden tests
  local t
  for t in $TASKS; do
    [ -f "$E/tasks/$t.md" ] || fail "missing $E/tasks/$t.md"
    case "$t" in t2-newpage|t3-newapp) ;; *) [ -d "$E/hidden/$t" ] || fail "missing $E/hidden/$t" ;; esac
  done
}

case "$STOP_AT" in [0-9][0-9][0-9][0-9]) ;; *) echo "STOP_AT must be four digits (HHMM), got '$STOP_AT'" >&2; exit 2 ;; esac

preflight
[ "${PREFLIGHT_ONLY:-}" = 1 ] && { echo "preflight ok"; exit 0; }

# one matrix at a time, dry runs included
mkdir -p "$E/logs"
mtime() { stat -f %m "$1" 2>/dev/null || stat -c %Y "$1" 2>/dev/null || echo 0; }
if ! mkdir "$E/logs/matrix.lock" 2>/dev/null; then
  other="$(cat "$E/logs/matrix.lock/pid" 2>/dev/null || true)"
  if [ -n "$other" ]; then
    if kill -0 "$other" 2>/dev/null; then
      echo "another run-matrix.sh (pid $other) holds $E/logs/matrix.lock" >&2
      exit 2
    fi
  elif [ $(( $(date +%s) - $(mtime "$E/logs/matrix.lock") )) -lt 10 ]; then
    # a holder between mkdir and writing its pid: held, not stale
    echo "another run-matrix.sh is taking $E/logs/matrix.lock" >&2
    exit 2
  fi
  rm -rf "$E/logs/matrix.lock"
  mkdir "$E/logs/matrix.lock" 2>/dev/null || { echo "cannot take $E/logs/matrix.lock" >&2; exit 2; }
fi
LOCK="$E/logs/matrix.lock"
echo $$ > "$LOCK/pid.tmp" && mv "$LOCK/pid.tmp" "$LOCK/pid"

PROGRESS="$E/logs/matrix.progress"
note() { echo "$(TZ=America/New_York date '+%a %F %H:%M') $*" >> "$PROGRESS"; }

# the clock override is a test hook: never honoured in a plain real run
clock=""
if [ "${DRY_RUN:-}" = 1 ] || [ -n "${RUN_LOOP:-}" ]; then clock="${E12_CLOCK:-}"; fi
stop_now() {
  local c u hm
  c="${clock:-$(TZ=America/New_York date '+%u %H%M')}"
  u="${c%% *}"; hm="${c##* }"
  case "$u" in [1-7]) ;; *) echo "bad clock '$c' (want '<1-7> <HHMM>')" >&2; exit 2 ;; esac
  case "$hm" in [0-9][0-9][0-9][0-9]) ;; *) echo "bad clock '$c' (want '<1-7> <HHMM>')" >&2; exit 2 ;; esac
  [ "$u" -le 5 ] && [ $((10#$hm)) -ge $((10#$STOP_AT)) ] && [ $((10#$hm)) -lt 1700 ]
}

# seconds until the next weekday 09:00 in New York. The kill at that time reaches the claude CLI
# (the cell's process group) but NOT children run_loop.py starts with start_new_session (alder
# build/test, go): those can outlive the kill and finish on their own; they write nothing the
# next invocation trusts, because the cut cell has no summary.json and is deleted and rerun.
secs_to_nine() {
  if [ -n "${RUN_LOOP:-}" ] && [ -n "${E12_DEADLINE_SECS:-}" ]; then echo "$E12_DEADLINE_SECS"; return; fi
  python3 - <<'PY'
from datetime import datetime, timedelta
from zoneinfo import ZoneInfo
now = datetime.now(ZoneInfo("America/New_York"))
d = now.replace(hour=9, minute=0, second=0, microsecond=0)
while d <= now or d.weekday() > 4:
    d = (d + timedelta(days=1)).replace(hour=9, minute=0, second=0, microsecond=0)
print(int((d - now).total_seconds()))
PY
}

harness_error() { grep -Eq '"outcome"[[:space:]]*:[[:space:]]*"harness_error"' "$1" 2>/dev/null; }

for s in $SAMPLES; do
  for model in $MODELS; do
    label="${model//:/-}"
    for task in $TASKS; do
      cell="$s $model $task"
      if stop_now; then
        echo "STOPPED before $cell"
        [ "${DRY_RUN:-}" = 1 ] || note "STOPPED before $cell"
        exit 3
      fi
      dir="$E/runs/$label/$task-$s"
      rerun=0
      if [ -f "$dir/summary.json" ] && ! harness_error "$dir/summary.json"; then
        if [ "${DRY_RUN:-}" = 1 ]; then echo "$cell would skip"; else note "$cell skipped"; fi
        continue
      fi
      if [ -e "$dir" ]; then
        if [ "${DRY_RUN:-}" = 1 ]; then
          echo "$cell would rerun"
        else
          rm -rf "$dir"
          note "$cell rerun"
        fi
      fi
      if [ -n "${RUN_LOOP:-}" ]; then runner=("$RUN_LOOP"); else runner=(python3 -u experiments/run_loop.py); fi
      cmd=("${runner[@]}" --exp "$E" --model "$model" --task "$task" --sample "$s"
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
      # the cell runs in its own process group under a watchdog that kills it at 09:00 on a weekday
      deadline="$(secs_to_nine)"
      case "$deadline" in ''|*[!0-9]*) echo "cannot compute the seconds until 09:00 (got '$deadline'); refusing to run unwatched" >&2; exit 2 ;; esac
      set -m
      "${cmd[@]}" </dev/null >> "$E/logs/$label.log" 2>&1 &
      pid=$!
      set +m
      started="$(date +%s)"; killed=0
      while kill -0 "$pid" 2>/dev/null; do
        if [ $(( $(date +%s) - started )) -ge "$deadline" ]; then
          kill -TERM -- "-$pid" 2>/dev/null
          sleep 2
          kill -KILL -- "-$pid" 2>/dev/null
          killed=1
          break
        fi
        sleep 1
      done
      rc=0; wait "$pid" 2>/dev/null || rc=$?
      if [ "$killed" = 1 ]; then
        echo "STOPPED: killed at 09:00 during $cell"
        note "$cell killed at 09:00"
        exit 3
      fi
      if harness_error "$dir/summary.json"; then
        echo "HARNESS_ERROR at $cell"
        note "HARNESS_ERROR at $cell"
        exit 4
      fi
      if [ "$rc" -ne 0 ]; then
        note "$cell failed (exit $rc)"
      else
        note "$cell done"
      fi
    done
  done
done
echo "matrix complete"
[ "${DRY_RUN:-}" = 1 ] || note "MATRIX COMPLETE"
exit 0
