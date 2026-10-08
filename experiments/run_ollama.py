#!/usr/bin/env python3
"""Run one task against one Ollama model and save the raw output.

Usage: run_ollama.py <experiment-dir> <model> <task-file> [prompt-file] [run-suffix]
Writes <experiment-dir>/runs/<model-safe>/<task>.md
"""
import json
import pathlib
import re
import sys
import time
import urllib.request

exp = pathlib.Path(sys.argv[1])
model = sys.argv[2]
task = pathlib.Path(sys.argv[3])
prompt_file = pathlib.Path(sys.argv[4]) if len(sys.argv) > 4 else exp / "prompt.md"
suffix = sys.argv[5] if len(sys.argv) > 5 else ""
prompt = prompt_file.read_text()
task_text = task.read_text()

body = {
    "model": model,
    "stream": False,
    "options": {"num_ctx": 32768, "temperature": 0},
    "messages": [
        {"role": "system", "content": prompt},
        {"role": "user", "content": task_text},
    ],
}
t0 = time.time()
req = urllib.request.Request(
    "http://localhost:11434/api/chat",
    data=json.dumps(body).encode(),
    headers={"Content-Type": "application/json"},
)
with urllib.request.urlopen(req, timeout=3600) as r:
    resp = json.load(r)
dt = time.time() - t0
text = resp["message"]["content"]
text = re.sub(r"<think>.*?</think>\s*", "", text, flags=re.S)

out_dir = exp / "runs" / (model.replace(":", "-").replace("/", "-") + suffix)
out_dir.mkdir(parents=True, exist_ok=True)
out = out_dir / (task.stem + ".md")
meta = (
    f"<!-- model={model} task={task.name} seconds={dt:.0f} "
    f"prompt_tokens={resp.get('prompt_eval_count')} output_tokens={resp.get('eval_count')} -->\n"
)
out.write_text(meta + text)
print(f"{model} {task.stem}: {dt:.0f}s, {resp.get('eval_count')} tokens -> {out}")
