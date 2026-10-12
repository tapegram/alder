#!/bin/bash
# The E9 matrix, one invocation at a time, each to its own log: logs/<cond>-<sample>-<model>.log.
# Order (the protocol): Haiku, then Sonnet, samples s1 then s2, both conditions; then Opus s1, both conditions.
cd /Users/tavishpegram/projects/alder/experiments/e9-feedback
run() { local cond=$1 sample=$2 model=$3
  logs/run.sh $cond $sample $model > logs/$cond-$sample-${model//:/-}.log 2>&1; }
for model in claude:haiku claude:sonnet; do
  for sample in s1 s2; do
    run baseline $sample $model
    run feedback $sample $model
  done
done
run baseline s1 claude:opus
run feedback s1 claude:opus
echo "matrix done $(date '+%F %T')"
