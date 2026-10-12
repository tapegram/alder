#!/usr/bin/env bash
# verify.sh: the E8 gate for Task 1's material. For each task and each of its two solutions
# (reference/, reference-alt/) it copies the solution to a scratch directory and runs:
#   check   `alder check` on the solution (warnings allowed, errors not)
#   build   `alder build --emit` (the harness's --build)
#   own     `alder test` on the solution as written (its own tests)
#   hidden  `alder test -run '^Hidden '` after adding hidden/<task>/Hidden.alder as app/Hidden.alder
#           (passed/total; total counts the module's `test "…"` lines)
#   drops   whether the solution's migrations/ have a step marked DROPS DATA (T7, T8 only)
# and prints one row per solution. It exits 1 when any cell is not as expected: every check,
# build and test green, and DROPS DATA exactly where expected (only T7's reference-alt, the lossy
# negative control; see README.md).
#
# Usage: experiments/e8-hidden/verify.sh [task ...]        (default: all five)
#   ALDER=/path/to/alder   the compiler to use; default: this tree's ./cmd/alder (`go build` once
#                          into the scratch directory, which is `go run ./cmd/alder` without
#                          recompiling per call).
# Every alder command runs with the scratch copy as its working directory: `alder build` and
# `alder test` write build/<app>/ under the working directory, never under the repository.
set -u

HERE="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
TASKS=("$@")
[ ${#TASKS[@]} -eq 0 ] && TASKS=(t4-appmsg t5-ownership t6-orders t7-drafts t8-notify)

SCRATCH="$(mktemp -d "${TMPDIR:-/tmp}/e8-verify.XXXXXX")"
trap 'rm -rf "$SCRATCH"' EXIT

if [ -z "${ALDER:-}" ]; then
  ALDER="$SCRATCH/alder"
  (cd "$ROOT" && go build -o "$ALDER" ./cmd/alder) || { echo "verify.sh: go build ./cmd/alder failed" >&2; exit 2; }
fi
ALDER="$(cd "$(dirname "$ALDER")" && pwd)/$(basename "$ALDER")"

# alder DIR ARGS...: runs `alder ARGS... .` inside DIR.
alder() {
  local dir="$1"; shift
  (cd "$dir" && "$ALDER" "$@")
}

fail=0

# The task texts: `without` is `with` minus its tests paragraph, and mentions no test.
python3 - "$HERE" "$ROOT" <<'EOF' || fail=1
import pathlib, re, sys
here, root = pathlib.Path(sys.argv[1]), pathlib.Path(sys.argv[2])
bad = []
for w in sorted((here / "tasks/with").glob("*.md")):
    wo = here / "tasks/without" / w.name
    a, b = w.read_text(), wo.read_text() if wo.exists() else ""
    m = re.search(r"^Also write tests.*?\n\n", a, re.S | re.M)
    if not m or a[:m.start()] + a[m.end():] != b:
        bad.append(f"{wo.name}: not `with` minus its tests paragraph")
    if re.search(r"test", b, re.I):
        bad.append(f"{wo.name}: mentions tests")
for line in bad:
    print("verify.sh: task text:", line, file=sys.stderr)
sys.exit(1 if bad else 0)
EOF

printf '%-14s %-14s %-6s %-6s %-10s %-8s %s\n' task solution check build own hidden drops
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
      printf '%-14s %-14s missing\n' "$task" "$sol"
      fail=1
      continue
    fi
    bad=0
    cp -R "$src" "$dir"
    rm -rf "$dir/build"

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
    if [ "$task" = t7-drafts ] || [ "$task" = t8-notify ]; then
      if grep -q 'DROPS DATA' "$dir"/migrations/*.sql 2>/dev/null; then drops=yes; else drops=no; fi
      want=no
      [ "$task" = t7-drafts ] && [ "$sol" = reference-alt ] && want=yes
      [ "$drops" = "$want" ] || { drops="$drops!(want $want)"; bad=1; }
    fi

    printf '%-14s %-14s %-6s %-6s %-10s %-8s %s\n' "$task" "$sol" "$check" "$build" "$own" "$hidden_cell" "$drops"
    if [ "$bad" -ne 0 ]; then
      fail=1
      for log in check build own hcheck hidden; do
        [ -f "$dir.$log.log" ] && grep -qE 'error|FAIL' "$dir.$log.log" && { echo "--- $sol/$task $log:"; grep -E 'error|FAIL' -A3 "$dir.$log.log" | head -20; }
      done
    fi
  done
done
exit $fail
