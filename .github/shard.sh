#!/usr/bin/env bash
# Splits one package's tests across CI jobs (.github/workflows/ci.yml).
#
#   .github/shard.sh N K [pkg [go test flags...]]   run shard K (0..N-1) of pkg's tests (default
#                                    ./cmd/alder); the flags go to go test after the shard's -run and
#                                    -timeout, so a later -run or -timeout of theirs wins (the last
#                                    value counts)
#   .github/shard.sh N check [pkg]   assert the N shards' -run patterns together select every
#                                    test exactly once (go test -list with each shard's pattern)
#   .github/shard.sh weights FILE    print the weight lines of .github/shard-weights from FILE, the
#                                    output of `go test ./cmd/alder -count=1 -json` (needs python3)
#
# Shards are balanced by weight: .github/shard-weights lists the slow tests with their seconds
# (measured: `shard.sh weights`), every other test weighs 1. A test's weight is its Elapsed, or
# for a parallel group (a test whose subtests call t.Parallel: its own Elapsed ends before they
# run) the time from its first subtest starting (its "cont" event once t.Parallel lets it go, else
# its "run") to its last subtest ending, whichever is longer; the tests of 2 s or more are listed. The tests, heaviest first
# (ties by name), each go to the lightest shard so far (ties: the lowest k). Deterministic, and a
# new or renamed test lands in some shard without anyone editing a file; a new slow test is worth
# a line in shard-weights.
set -euo pipefail
if [ "$1" = weights ]; then
  exec python3 - "$2" <<'PY'
import datetime, json, re, sys
def at(s):  # an RFC 3339 time with nanoseconds, which Python's fromisoformat does not read
    m = re.match(r'(.*T\d\d:\d\d:\d\d)(\.\d+)?(Z|[+-]\d\d:\d\d)$', s)
    return datetime.datetime.fromisoformat(m.group(1) + m.group(3).replace('Z', '+00:00')).timestamp() + float(m.group(2) or 0)
elapsed, run, cont, end = {}, {}, {}, {}
for line in open(sys.argv[1]):
    try:
        e = json.loads(line)
    except ValueError:
        continue
    name, act = e.get('Test'), e.get('Action')
    if not name or 'Time' not in e:
        continue
    root = name.split('/')[0]
    if '/' not in name:
        if act == 'pass':
            elapsed[root] = e['Elapsed']
    elif act == 'run':
        run.setdefault(name, at(e['Time']))
    elif act == 'cont':
        cont[name] = at(e['Time'])
    elif act in ('pass', 'fail', 'skip'):
        end[root] = max(end.get(root, 0), at(e['Time']))
weight = dict(elapsed)
for root in end:
    if root in elapsed:
        first = min(cont.get(s, run[s]) for s in run if s.split('/')[0] == root)
        weight[root] = max(elapsed[root], end[root] - first)
for name, w in sorted(weight.items(), key=lambda x: (-round(x[1]), x[0])):
    if round(w) >= 2:
        print(round(w), name)
PY
fi
n=$1 k=$2 pkg=${3:-./cmd/alder}
shift $(( $# < 3 ? $# : 3 ))

list() { go test "$pkg" -list "$1" | grep -E '^(Test|Example|Fuzz)' || true; }

all=$(list '.*')
weights="$(dirname "$0")/shard-weights"
assign=$(echo "$all" | awk -v n="$n" -v wf="$weights" '
  BEGIN { while ((getline line < wf) > 0) { if (line ~ /^[0-9]/) { split(line, f, " "); w[f[2]] = f[1] } } }
  NF { print (($1 in w) ? w[$1] : 1), $1 }' | sort -k1,1nr -k2,2 | awk -v n="$n" '
  { k = 0; for (i = 1; i < n; i++) if (load[i] < load[k]) k = i; load[k] += $1; print k, $2 }')
shard() { # the names in shard $1
  echo "$assign" | awk -v k="$1" '$1 == k { print $2 }'
}
pattern() { echo "^($(shard "$1" | paste -sd'|' -))\$"; }

if [ "$k" = check ]; then
  got=$(for i in $(seq 0 $((n - 1))); do
    p=$(pattern "$i"); [ "$p" = '^()$' ] || list "$p"
  done | sort)
  if [ "$got" != "$(echo "$all" | sort)" ]; then
    echo "shards of $pkg do not cover its tests exactly once:"
    diff <(echo "$all" | sort) <(echo "$got") || true
    exit 1
  fi
  echo "$(echo "$all" | wc -l | tr -d ' ') tests of $pkg, each in exactly one of $n shards"
  exit 0
fi

p=$(pattern "$k")
if [ "$p" = '^()$' ]; then echo "shard $k of $n of $pkg is empty"; exit 0; fi
echo "shard $k of $n of $pkg: $(shard "$k" | wc -l | tr -d ' ') tests"
shard "$k" | paste -sd' ' -
# 28m: go test times out (with a goroutine dump) inside ci.yml's 30-minute job, not after it
exec go test "$pkg" -count=1 -timeout 28m -run "$p" "$@"
