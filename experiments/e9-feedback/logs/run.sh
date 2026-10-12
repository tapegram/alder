#!/bin/bash
# usage: logs/run.sh <condition> <sample> <model>   (from anywhere; runs at the repository root)
# condition: baseline (--build --test --mutate) or feedback (the same + --feedback 2). Claude models, prompt-full.md.
cd /Users/tavishpegram/projects/alder
cond=$1; sample=$2; model=$3
E=experiments/e9-feedback
case $cond in
  baseline) fb=();;
  feedback) fb=(--feedback 2);;
  *) echo "unknown condition $cond" >&2; exit 2;;
esac
TASKS=${TASKS:-"t5-ownership t6-orders t7-drafts t9-bookings t10-teams"}
COMMON=(--exp $E/$cond --model "$model" --retries 2 --prompt "$PWD/$E/prompt-full.md" --alder $E/alder
        --build --test --mutate "${fb[@]}" --hidden $E/hidden --condition "$cond" --sample "$sample")
echo "start $cond $sample $model $(date '+%F %T')"
for t in $TASKS; do
  case $t in
    t7-drafts) base=(--base experiments/e8-hidden/base/blog-nodrafts --migrate);;
    *) base=();;
  esac
  echo "task $t start $(date '+%F %T')"
  python3 -u experiments/run_loop.py "${COMMON[@]}" --task "$t" "${base[@]}"
  rc=$?
  echo "task $t end $(date '+%F %T') exit $rc"
done
echo "end $cond $sample $model $(date '+%F %T')"
