#!/usr/bin/env bash
# verify.sh: the E12 material gate. For each task and each of its solutions (reference/,
# reference-alt/) it copies the solution to a scratch directory and runs:
#   check   `alder check` on the solution (warnings allowed, errors not)
#   build   `alder build --emit` (the harness's --build)
#   own     `alder test` on the solution as written (its own tests; passed/total)
#   hidden  `alder test -run '^Hidden '` after adding hidden/<task>/Hidden.alder as app/Hidden.alder
#           (passed/total; total counts the module's `test "…"` lines and must be the task's
#           expected count, expect() below)
#   score   `alder test --mutate` on the solution's own tests (never the hidden ones): killed/all
#           and the exact score, which must be >= 0.9 (MIN below)
#   names   `ok`, or `N differ`: run_loop.names_check of the solution against the task's Names block
#   drops   whether migrations/ have a step marked DROPS DATA (T7 only: the reference-alt is the
#           lossy negative control, so `yes` is expected there and nowhere else)
# T2 and T3 are smoke rows: reference/<task> only; check, build and own; no hidden tests, no
# mutation. T2's reference/t2-newpage/app/ holds only the files the task adds or changes; it is
# overlaid on `git archive HEAD examples/todos`.
# Then, for every solution with survivors, the survivor lines and the solution's SURVIVORS.md
# (one line per survivor: why no input can tell it from the app). Each survivor's
# `module decl:line kind` must appear in that file.
#
# It also checks the texts of the tasks given on the command line: each has the tests paragraph
# ("Also write tests") and the output paragraph; T4–T15 have a Names block; none contains
# `page "`, a `msg, `Redirect (` or `Rerender`; T6's never names `Task.fail` or `Task.notFound`.
#
# Exit 1 when any cell is not as expected: every check, build, own and hidden test green, every
# score >= MIN with every survivor justified, every names cell `ok`, DROPS DATA only on T7's
# reference-alt, no solution missing.
#
# Usage: experiments/e12-routes/verify.sh [task ...]        (default: all thirteen)
#   ALDER=/path/to/alder   the compiler to use; default: experiments/e12-routes/alder (the pin,
#                          built by `pin.sh binary COMMIT`).
# Every alder command runs with the scratch copy as its working directory, so build/ never lands
# in the repository.
set -u

HERE="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
MIN=0.9
TASKS=("$@")
[ ${#TASKS[@]} -eq 0 ] && TASKS=(t2-newpage t3-newapp t4-appmsg t5-ownership t6-orders t7-drafts t9-bookings t10-teams t11-workspace t12-search t13-feed t14-board t15-pantry)

# The hidden module's test count each task must have.
expect() {
  case "$1" in
    t4-appmsg|t5-ownership) echo 8 ;;
    t6-orders|t11-workspace) echo 9 ;;
    t7-drafts|t9-bookings) echo 7 ;;
    t10-teams) echo 8 ;;
  esac
}

SCRATCH="$(mktemp -d "${TMPDIR:-/tmp}/e12-verify.XXXXXX")"
trap 'rm -rf "$SCRATCH"' EXIT

ALDER="${ALDER:-$HERE/alder}"
[ -x "$ALDER" ] || { echo "verify.sh: no alder binary at $ALDER (run pin.sh binary COMMIT)" >&2; exit 2; }
ALDER="$(cd "$(dirname "$ALDER")" && pwd)/$(basename "$ALDER")"

# alder DIR ARGS...: runs `alder ARGS...` inside DIR.
alder() {
  local dir="$1"; shift
  (cd "$dir" && "$ALDER" "$@")
}

fail=0

# The task texts (only the tasks given).
python3 - "$HERE" "${TASKS[@]}" <<'PYEOF' || fail=1
import pathlib, sys
here = pathlib.Path(sys.argv[1])
bad = []
for n in sys.argv[2:]:
    p = here / "tasks" / f"{n}.md"
    if not p.exists():
        bad.append(f"{n}: no task text")
        continue
    t = p.read_text()
    if "\nAlso write tests (`test \"…\" = …`, chapter 10) in the modules they test" not in t:
        bad.append(f"{n}: no tests paragraph")
    if "No explanation, only files.\n" not in t:
        bad.append(f"{n}: no output paragraph")
    if n not in ("t2-newpage", "t3-newapp") and "\n**Names.**" not in t:
        bad.append(f"{n}: no Names block")
    for w in ('page "', "`msg", "Redirect (", "Rerender"):
        if w in t:
            bad.append(f"{n}: says {w!r}")
    if n == "t6-orders" and ("Task.fail" in t or "Task.notFound" in t):
        bad.append(f"{n}: names a refusal mechanism")
for line in bad:
    print("verify.sh: task text:", line, file=sys.stderr)
sys.exit(1 if bad else 0)
PYEOF

# names_cell TASK APP_DIR: `ok`, `N differ`, `-` (no Names block), or `?` (names_check failed).
names_cell() {
  python3 - "$ROOT" "$HERE/tasks/$1.md" "$2" <<'PYEOF'
import pathlib, sys
root = pathlib.Path(sys.argv[1])
sys.path.insert(0, str(root / "experiments"))
import run_loop
try:
    text = pathlib.Path(sys.argv[2]).read_text()
    r = run_loop.names_check(text, pathlib.Path(sys.argv[3]))
except Exception as e:
    print("?", file=sys.stderr); print(f"verify.sh: names_check: {e}", file=sys.stderr)
    print("?"); sys.exit(0)
if r is None:
    print("-")
elif r["ok"]:
    print("ok")
else:
    print(f"{len(r['mismatches'])} differ")
PYEOF
}

declare -a SURV_REPORT=()
printf '%-13s %-14s %-6s %-6s %-8s %-8s %-18s %-9s %s\n' task solution check build own hidden score names drops
for task in "${TASKS[@]}"; do
  smoke=0
  case "$task" in t2-newpage|t3-newapp) smoke=1 ;; esac
  hidden="$HERE/hidden/$task/Hidden.alder"
  total=0
  if [ "$smoke" -eq 0 ] && [ -f "$hidden" ]; then
    total=$(grep -c '^test "' "$hidden")
    want="$(expect "$task")"
    if [ -n "$want" ] && [ "$total" -ne "$want" ]; then
      echo "verify.sh: $hidden has $total tests, want $want" >&2
      fail=1
    fi
  fi
  sols=(reference reference-alt)
  [ "$smoke" -eq 1 ] && sols=(reference)
  for sol in "${sols[@]}"; do
    src="$HERE/$sol/$task"
    dir="$SCRATCH/$sol-$task"
    if [ ! -d "$src/app" ]; then
      printf '%-13s %-14s missing\n' "$task" "$sol"
      fail=1
      continue
    fi
    if [ "$smoke" -eq 0 ] && [ ! -f "$hidden" ]; then
      printf '%-13s %-14s no hidden module\n' "$task" "$sol"
      fail=1
      continue
    fi
    bad=0
    if [ "$task" = t2-newpage ]; then
      mkdir -p "$dir"
      (cd "$ROOT" && git archive HEAD examples/todos) | tar -x -C "$dir" --strip-components=2
      cp -R "$src/app/." "$dir/app/"
    else
      cp -R "$src" "$dir"
    fi
    rm -rf "$dir/build" "$dir/SURVIVORS.md"

    if alder "$dir" check . >"$dir.check.log" 2>&1; then check=ok; else check=FAIL; bad=1; fi
    if alder "$dir" build --emit . >"$dir.build.log" 2>&1; then build=ok; else build=FAIL; bad=1; fi
    rm -rf "$dir/build"

    alder "$dir" test . >"$dir.own.log" 2>&1
    own_exit=$?
    own=$(grep -E '^[0-9]+ passed, [0-9]+ failed' "$dir.own.log" | tail -1 | sed -E 's/^([0-9]+) passed, ([0-9]+) failed.*/\1\/\2/')
    if [ "$own_exit" -ne 0 ] || [ -z "$own" ]; then
      own="FAIL${own:+($own)}"
      bad=1
    else
      p=${own%/*}; f=${own#*/}
      own="$p/$((p + f))"
    fi

    names=$(names_cell "$task" "$dir")
    if [ "$names" = "?" ]; then
      names="FAIL"; bad=1
    elif [ "$names" != ok ] && [ "$names" != "-" ]; then
      names="$names!"; bad=1
    fi

    if [ "$smoke" -eq 1 ]; then
      printf '%-13s %-14s %-6s %-6s %-8s %-8s %-18s %-9s %s\n' "$task" "$sol" "$check" "$build" "$own" - - "$names" -
      if [ "$bad" -ne 0 ]; then
        fail=1
        for log in check build own; do
          [ -f "$dir.$log.log" ] && grep -qE 'error|FAIL' "$dir.$log.log" && { echo "--- $sol/$task $log:"; grep -E 'error|FAIL' -A3 "$dir.$log.log" | head -20; }
        done
      fi
      continue
    fi

    # The mutation score, on the solution's own tests only (before the hidden module is added).
    alder "$dir" test --mutate . >"$dir.mutate.log" 2>&1
    line=$(grep -E '^[0-9]+ mutants: ' "$dir.mutate.log" | tail -1)
    if [ -z "$line" ]; then
      score="FAIL"
      bad=1
    else
      n=$(echo "$line" | sed -E 's/^([0-9]+) mutants: ([0-9]+) killed.*/\1/')
      k=$(echo "$line" | sed -E 's/^([0-9]+) mutants: ([0-9]+) killed.*/\2/')
      exact=$(awk -v k="$k" -v n="$n" 'BEGIN { if (n == 0) print "1.000"; else printf "%.3f", k / n }')
      score="$k/$n $exact"
      sbad=0
      if ! awk -v k="$k" -v n="$n" -v m="$MIN" 'BEGIN { exit !(n == 0 || k / n >= m) }'; then
        sbad=1
      fi
      survivors=$(sed -n '/^survived:/,$p' "$dir.mutate.log" | sed '1d' | grep -E '^  ')
      if [ -n "$survivors" ]; then
        just="$src/SURVIVORS.md"
        report="--- $sol/$task: survivors"$'\n'"$survivors"
        if [ ! -f "$just" ]; then
          report="$report"$'\n'"    (no SURVIVORS.md)"
          sbad=1
        else
          # Each survivor's "module decl:line kind" must be named in SURVIVORS.md.
          while IFS= read -r s; do
            key=$(echo "$s" | awk '{ sub(/^ +/, ""); split($0, a, /  +/); print a[1] " " a[2] " " a[3] }')
            if ! grep -qF "$key" "$just"; then
              report="$report"$'\n'"    (not justified in SURVIVORS.md: $key)"
              sbad=1
            fi
          done <<<"$survivors"
          report="$report"$'\n'"--- $sol/$task: SURVIVORS.md"$'\n'"$(cat "$just")"
        fi
        SURV_REPORT+=("$report")
      fi
      [ "$sbad" -eq 0 ] || { score="$score!"; bad=1; }
    fi

    cp "$hidden" "$dir/app/Hidden.alder"
    if alder "$dir" check . >"$dir.hcheck.log" 2>&1; then
      alder "$dir" test . -run '^Hidden ' >"$dir.hidden.log" 2>&1
      passed=$(grep -E '^[0-9]+ passed, [0-9]+ failed' "$dir.hidden.log" | tail -1 | sed -E 's/^([0-9]+) passed.*/\1/')
      passed=${passed:-0}
    else
      passed=0
      check="$check+H:FAIL"
    fi
    hidden_cell="$passed/$total"
    [ "$passed" -eq "$total" ] || { hidden_cell="$hidden_cell!"; bad=1; }

    drops=-
    if [ -d "$dir/migrations" ]; then
      if grep -q 'DROPS DATA' "$dir"/migrations/*.sql 2>/dev/null; then drops=yes; else drops=no; fi
      want=no
      [ "$task" = t7-drafts ] && [ "$sol" = reference-alt ] && want=yes
      [ "$drops" = "$want" ] || { drops="$drops!(want $want)"; bad=1; }
    fi

    printf '%-13s %-14s %-6s %-6s %-8s %-8s %-18s %-9s %s\n' "$task" "$sol" "$check" "$build" "$own" "$hidden_cell" "$score" "$names" "$drops"
    if [ "$bad" -ne 0 ]; then
      fail=1
      for log in check build own mutate hcheck hidden; do
        [ -f "$dir.$log.log" ] && grep -qE 'error|FAIL' "$dir.$log.log" && { echo "--- $sol/$task $log:"; grep -E 'error|FAIL' -A3 "$dir.$log.log" | head -20; }
      done
    fi
  done
done

for r in "${SURV_REPORT[@]+"${SURV_REPORT[@]}"}"; do
  echo
  echo "$r"
done
exit $fail
