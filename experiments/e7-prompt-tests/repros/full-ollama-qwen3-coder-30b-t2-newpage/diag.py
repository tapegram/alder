# Diagnostic (not part of the matrix): the exact T2 round-0 request the harness sends, streamed, capped at num_predict.
import json, sys, time, urllib.request
cond, ctx, cap, out = sys.argv[1], int(sys.argv[2]), int(sys.argv[3]), sys.argv[4]
E = "/Users/tavishpegram/projects/alder/experiments/e7-prompt-tests/"
body = {"model": "qwen3-coder:30b", "stream": True, "options": {"num_ctx": ctx, "temperature": 0, "num_predict": cap},
        "messages": [{"role": "system", "content": open(E + f"prompt-{cond}.md").read()},
                     {"role": "user", "content": open(E + "tasks/t2-newpage.md").read()}]}
req = urllib.request.Request("http://localhost:11434/api/chat", data=json.dumps(body).encode(), headers={"Content-Type": "application/json"})
t0 = time.time(); first = None; n = 0
with open(out, "w") as f, urllib.request.urlopen(req, timeout=1800) as r:
    for line in r:
        ev = json.loads(line)
        if first is None: first = time.time() - t0
        f.write(ev.get("message", {}).get("content", "")); f.flush(); n += 1
        if ev.get("done"):
            print(json.dumps({k: ev.get(k) for k in ("done_reason", "prompt_eval_count", "eval_count", "total_duration")}), "first_chunk_s", round(first, 1), "total_s", round(time.time() - t0, 1))
