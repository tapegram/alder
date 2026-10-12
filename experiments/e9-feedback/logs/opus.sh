#!/bin/bash
# The Opus cells (s1, both conditions, sequentially), started at 11:05 concurrently with feedback s2 claude:sonnet
# (coordinator ruling 2026-10-12: Opus runs alongside Sonnet's second sample). Logs: logs/<cond>-s1-claude-opus.log.
cd /Users/tavishpegram/projects/alder/experiments/e9-feedback
logs/run.sh baseline s1 claude:opus > logs/baseline-s1-claude-opus.log 2>&1
logs/run.sh feedback s1 claude:opus > logs/feedback-s1-claude-opus.log 2>&1
echo "opus done $(date '+%F %T')"
