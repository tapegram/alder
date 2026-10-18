#!/usr/bin/env bash
# The whole Go suite at once, in parallel: cmd/alder in shards (.github/shard.sh, by the weights in
# .github/shard-weights) and every other package in one go test, each part logging to a file.
# Prints each part's result and time, and the tail of each failing part's log.
#
#   scripts/test.sh [go test flags...]    e.g. scripts/test.sh -short   (the quick local loop)
#
# ALDER_TEST_SHARDS (default 4) sets the number of cmd/alder shards. More is slower on a laptop:
# each shard's builds already use every core (6 shards at once took 665 s, 4 about 200 s).
# The mutation quality gates run only with ALDER_MUTATION_GATES=1 (cmd/alder/gates_test.go).
set -uo pipefail
cd "$(dirname "$0")/.."
n=${ALDER_TEST_SHARDS:-4}
logs=$(mktemp -d "${TMPDIR:-/tmp}/alder-test.XXXXXX")
start=$SECONDS

part() { # part NAME CMD...: runs CMD into $logs/NAME.log, then writes "exit seconds" to $logs/NAME.res
  local name=$1 s=$SECONDS; shift
  "$@" > "$logs/$name.log" 2>&1
  echo "$? $((SECONDS - s))" > "$logs/$name.res"
}

res() { # res NAME: sets code and secs from NAME's .res; a part that never finished (killed) fails
  code=killed secs=?
  [ -f "$logs/$1.res" ] && read -r code secs < "$logs/$1.res"
}

names=()
for k in $(seq 0 $((n - 1))); do
  part "cmd-alder-$k" .github/shard.sh "$n" "$k" ./cmd/alder "$@" &
  names+=("cmd-alder-$k")
done
part rest go test $(go list ./... | grep -v '^alder/cmd/alder$') -count=1 -timeout 30m "$@" &
names+=(rest)
wait

failed=0
for name in "${names[@]}"; do
  res "$name"
  if [ "$code" = 0 ]; then echo "ok    $name  ${secs}s"; else echo "FAIL  $name  ${secs}s  ($logs/$name.log)"; failed=1; fi
done
for name in "${names[@]}"; do
  res "$name"
  [ "$code" = 0 ] || { echo; echo "--- $name (last 40 lines)"; tail -n 40 "$logs/$name.log"; }
done
echo "$((SECONDS - start))s in all; logs in $logs"
exit $failed
