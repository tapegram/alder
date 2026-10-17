#!/bin/bash
# E11 matrix: $1 = cli | files. Runs 3 models x 8 tasks sequentially, one sample.
set +u
cd "$(dirname "$0")/../.."
E=experiments/e11-cli
COMMON=(--retries 5 --prompt $PWD/$E/prompt-reference.md --alder $E/alder-r2 --sample r2)
for model in ${MODELS:-claude:haiku claude:sonnet claude:opus}; do
  for task in t2-newpage t3-newapp t4-appmsg t5-ownership t6-orders t7-drafts t9-bookings t10-teams; do
    case $task in
      t2-newpage)  extra=(--base examples/todos);;
      t3-newapp)   extra=();;
      t4-appmsg)   extra=(--hidden experiments/e8-hidden/hidden);;
      t7-drafts)   extra=(--hidden experiments/e9-feedback/hidden --base experiments/e8-hidden/base/blog-nodrafts --migrate);;
      *)           extra=(--hidden experiments/e9-feedback/hidden);;
    esac
    if [ "$1" = cli ]; then
      [ -f $E/runs/${model#claude:}/$task-r2/summary.json ] || [ -f $E/runs/claude-${model#claude:}/$task-r2/summary.json ] && { echo "$(date '+%H:%M') $1 $model $task skipped (done)" >> $E/logs/matrix-r2-$1.progress; continue; }
      python3 -u experiments/run_loop.py --exp $E --model $model --task $task --cli --condition cli "${COMMON[@]}" "${extra[@]}" >> $E/logs/r2-cli-${model#claude:}.log 2>&1
    else
      [ -f $E/baseline/runs/claude-${model#claude:}/$task-r2/summary.json ] && { echo "$(date '+%H:%M') $1 $model $task skipped (done)" >> $E/logs/matrix-r2-$1.progress; continue; }
      python3 -u experiments/run_loop.py --exp $E/baseline --model $model --task $task --build --test --mutate --condition files "${COMMON[@]}" "${extra[@]}" >> $E/logs/r2-baseline-${model#claude:}.log 2>&1
    fi
    echo "$(date '+%H:%M') $1 $model $task done" >> $E/logs/matrix-r2-$1.progress
  done
done
echo "$(date '+%H:%M') $1 MATRIX COMPLETE" >> $E/logs/matrix-r2-$1.progress
