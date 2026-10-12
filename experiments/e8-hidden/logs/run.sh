#!/bin/bash
# usage: logs/run.sh <condition> <sample> <model>...   (from anywhere; runs at the repository root)
# Claude models: prompt-full.md (num_ctx is ignored). Ollama models: prompt-reference.md, num_ctx $NUM_CTX
# (default 65536), tasks $TASKS (default all five).
cd /Users/tavishpegram/projects/alder
cond=$1; sample=$2; shift 2
E=experiments/e8-hidden
M=(); prompt=$PWD/$E/prompt-full.md; ctx=()
for m in "$@"; do
  M+=(--model "$m")
  case $m in ollama:*) prompt=$PWD/$E/prompt-reference.md; ctx=(--num-ctx "${NUM_CTX:-65536}");; esac
done
TASKS=${TASKS:-"t4-appmsg t5-ownership t6-orders t7-drafts t8-notify"}
COMMON=(--exp $E/$cond "${M[@]}" --retries 2 --prompt "$prompt" --alder $E/alder --build --test
        --hidden $E/hidden --condition "$cond" --sample "$sample" "${ctx[@]}")
echo "start $cond $sample $* $(date '+%F %T')"
for t in $TASKS; do
  case $t in
    t7-drafts) base=(--base $E/base/blog-nodrafts --migrate);;
    t8-notify) base=(--base examples/blog --migrate);;
    *) base=();;
  esac
  echo "task $t start $(date '+%F %T')"
  python3 -u experiments/run_loop.py "${COMMON[@]}" --task "$t" "${base[@]}"
  rc=$?
  echo "task $t end $(date '+%F %T') exit $rc"
done
echo "end $cond $sample $* $(date '+%F %T')"
