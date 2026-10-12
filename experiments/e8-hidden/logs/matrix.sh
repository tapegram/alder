#!/bin/bash
# The E8 matrix after the first invocation (with s1 claude:haiku, started by hand), one invocation at a time,
# each to its own log: logs/<cond>-<sample>-<model>.log.
cd /Users/tavishpegram/projects/alder/experiments/e8-hidden
run() { local cond=$1 sample=$2 model=$3; shift 3
  env "$@" logs/run.sh $cond $sample $model > logs/$cond-$sample-${model//:/-}.log 2>&1; }
while pgrep -f 'run.sh with s1 claude:haiku' > /dev/null; do sleep 10; done
run without s1 claude:haiku
run with s1 claude:sonnet
run without s1 claude:sonnet
run with s2 claude:haiku
run without s2 claude:haiku
run with s2 claude:sonnet
run without s2 claude:sonnet
run with s1 claude:opus
run without s1 claude:opus
run with s1 ollama:qwen3-coder:30b TASKS="t5-ownership t6-orders" NUM_CTX=65536
echo "matrix done $(date '+%F %T')"
