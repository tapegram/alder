#!/usr/bin/env bash
# verify.sh: the E9 material gate. For each task and each of its two solutions (reference/,
# reference-alt/) it copies the solution to a scratch directory and runs:
#   check   `alder check` on the solution (warnings allowed, errors not)
#   build   `alder build --emit` (the harness's --build)
#   own     `alder test` on the solution as written (its own tests; passed/total)
#   hidden  `alder test -run '^Hidden '` after adding hidden/<task>/Hidden.alder as app/Hidden.alder
#           (passed/total; total counts the module's `test "…"` lines)
#   score   `alder test --mutate` on the solution's own tests (never the hidden ones): killed/all
#           and the exact score, which must be >= 0.9 (MIN below)
#   drops   whether migrations/ have a step marked DROPS DATA (T7 only: the reference-alt is the
#           lossy negative control of E8, so `yes` is expected there and nowhere else)
# Then, for every solution with survivors, the survivor lines and the solution's SURVIVORS.md
# (one line per survivor: why no input can tell it from the app). Each survivor's
# `module decl:line kind` must appear in that file.
#
# It also checks the task texts: each has the tests paragraph ("Also write tests") and the output
# paragraph; T6's never names `Task.fail` or `Task.notFound`; T5's and T7's are E8's `with` texts
# byte for byte.
#
# Exit 1 when any cell is not as expected: every check, build, own and hidden test green, every
# score >= MIN with every survivor justified, DROPS DATA only on T7's reference-alt.
#
# Usage: experiments/e9-feedback/verify.sh [task ...]        (default: all five)
#   ALDER=/path/to/alder   the compiler to use; default: this tree's ./cmd/alder, built once into
#                          the scratch directory.
# Every alder command runs with the scratch copy as its working directory, so build/ never lands
# in the repository.
set -u

HERE="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
MIN=0.9
TASKS=("$@")
[ ${#TASKS[@]} -eq 0 ] && TASKS=(t5-ownership t6-orders t7-drafts t9-bookings t10-teams)

SCRATCH="$(mktemp -d "${TMPDIR:-/tmp}/e9-verify.XXXXXX")"
trap 'rm -rf "$SCRATCH"' EXIT

if [ -z "${ALDER:-}" ]; then
  ALDER="$SCRATCH/alder"
  (cd "$ROOT" && go build -o "$ALDER" ./cmd/alder) || { echo "verify.sh: go build ./cmd/alder failed" >&2; exit 2; }
fi
ALDER="$(cd "$(dirname "$ALDER")" && pwd)/$(basename "$ALDER")"

# alder DIR ARGS...: runs `alder ARGS...` inside DIR.
alder() {
  local dir="$1"; shift
  (cd "$dir" && "$ALDER" "$@")
}

fail=0

# The task texts.
python3 - "$HERE" "$ROOT" <<'EOF' || fail=1
import pathlib, sys
here, root = pathlib.Path(sys.argv[1]), pathlib.Path(sys.argv[2])
bad = []
names = ["t5-ownership", "t6-orders", "t7-drafts", "t9-bookings", "t10-teams"]
for n in names:
    p = here / "tasks" / f"{n}.md"
    if not p.exists():
        bad.append(f"{n}: no task text")
        continue
    t = p.read_text()
    if "\nAlso write tests (`test \"…\" = …`, chapter 10) in the modules they test" not in t:
        bad.append(f"{n}: no tests paragraph")
    if "No explanation, only files.\n" not in t or "\n**Names.**" not in t:
        bad.append(f"{n}: no output paragraph or no Names block")
    if n in ("t5-ownership", "t7-drafts"):
        e8 = root / "experiments/e8-hidden/tasks/with" / f"{n}.md"
        if t != e8.read_text():
            bad.append(f"{n}: not E8's `with` text")
    if n == "t6-orders" and ("Task.fail" in t or "Task.notFound" in t):
        bad.append(f"{n}: names a refusal mechanism")
for line in bad:
    print("verify.sh: task text:", line, file=sys.stderr)
sys.exit(1 if bad else 0)
EOF

declare -a SURV_REPORT=()
printf '%-13s %-14s %-6s %-6s %-8s %-8s %-18s %s\n' task solution check build own hidden score drops
for task in "${TASKS[@]}"; do
  hidden="$HERE/hidden/$task/Hidden.alder"
  if [ ! -f "$hidden" ]; then
    echo "verify.sh: no $hidden" >&2
    fail=1
    continue
  fi
  total=$(grep -c '^test "' "$hidden")
  for sol in reference reference-alt; do
    src="$HERE/$sol/$task"
    dir="$SCRATCH/$sol-$task"
    if [ ! -d "$src/app" ]; then
      printf '%-13s %-14s missing\n' "$task" "$sol"
      fail=1
      continue
    fi
    bad=0
    cp -R "$src" "$dir"
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
      if ! awk -v k="$k" -v n="$n" -v m="$MIN" 'BEGIN { exit !(n == 0 || k / n >= m) }'; then
        score="$score!"
        bad=1
      fi
      survivors=$(sed -n '/^survived:/,$p' "$dir.mutate.log" | sed '1d' | grep -E '^  ')
      if [ -n "$survivors" ]; then
        just="$src/SURVIVORS.md"
        report="--- $sol/$task: survivors"$'\n'"$survivors"
        if [ ! -f "$just" ]; then
          report="$report"$'\n'"    (no SURVIVORS.md)"
          score="$score!"
          bad=1
        else
          # Each survivor's "module decl:line kind" must be named in SURVIVORS.md.
          while IFS= read -r s; do
            key=$(echo "$s" | awk '{ sub(/^ +/, ""); split($0, a, /  +/); print a[1] " " a[2] " " a[3] }')
            if ! grep -qF "$key" "$just"; then
              report="$report"$'\n'"    (not justified in SURVIVORS.md: $key)"
              score="$score!"
              bad=1
            fi
          done <<<"$survivors"
          report="$report"$'\n'"--- $sol/$task: SURVIVORS.md"$'\n'"$(cat "$just")"
        fi
        SURV_REPORT+=("$report")
      fi
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

    printf '%-13s %-14s %-6s %-6s %-8s %-8s %-18s %s\n' "$task" "$sol" "$check" "$build" "$own" "$hidden_cell" "$score" "$drops"
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
