#!/usr/bin/env bash
# Splits one package's tests across CI jobs (.github/workflows/ci.yml).
#
#   .github/shard.sh N K [pkg]       run shard K (0..N-1) of pkg's tests (default ./cmd/alder)
#   .github/shard.sh N check [pkg]   assert the N shards' -run patterns together select every
#                                    test exactly once (go test -list with each shard's pattern)
#
# A test's shard is cksum(name) mod N: stable, so a test stays put until it is renamed, and a
# renamed or new test lands in some shard without anyone editing this file.
set -euo pipefail
n=$1 k=$2 pkg=${3:-./cmd/alder}

list() { go test "$pkg" -list "$1" | grep -E '^(Test|Example|Fuzz)' || true; }

all=$(list '.*')
shard() { # the names in shard $1
  local name
  for name in $all; do
    if [ $(( $(printf %s "$name" | cksum | cut -d' ' -f1) % n )) -eq "$1" ]; then echo "$name"; fi
  done
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
exec go test "$pkg" -count=1 -run "$p"
