#!/usr/bin/env bash
# Splits one package's tests across CI jobs (.github/workflows/ci.yml).
#
#   .github/shard.sh N K [pkg [go test flags...]]   run shard K (0..N-1) of pkg's tests (default
#                                    ./cmd/alder); the flags go to go test after the shard's -run and
#                                    -timeout, so a later -run or -timeout of theirs wins (the last
#                                    value counts)
#   .github/shard.sh N check [pkg]   assert the N shards' -run patterns together select every
#                                    test exactly once (go test -list with each shard's pattern)
#
# Shards are balanced by weight: .github/shard-weights lists the slow tests with their seconds
# (measured; `go test -v` prints them), every other test weighs 1. The tests, heaviest first
# (ties by name), each go to the lightest shard so far (ties: the lowest k). Deterministic, and a
# new or renamed test lands in some shard without anyone editing a file; a new slow test is worth
# a line in shard-weights.
set -euo pipefail
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
exec go test "$pkg" -count=1 -timeout 40m -run "$p" "$@"
