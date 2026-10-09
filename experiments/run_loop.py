#!/usr/bin/env python3
"""Generate → alder check → feed diagnostics back → retry.

Usage:
  run_loop.py --exp DIR --model ollama:NAME|claude:NAME|fake --task NAME [--retries 1] [--prompt prompt.md] [--alder BIN]

Writes DIR/runs/<model>/<task>/round<N>/{output.md,check.json,app/...} and DIR/runs/<model>/<task>/summary.json.
"""
import argparse, json, pathlib, re, shutil, subprocess, sys, time, urllib.request

MAX_OUTPUT = 200_000


class ModelError(Exception):
    pass


class Model:
    def generate(self, system: str, user: str):
        raise NotImplementedError


class FakeModel(Model):
    def __init__(self, responses):
        self.responses = list(responses)
        self.prompts = []

    def generate(self, system, user):
        self.prompts.append(user)
        if not self.responses:
            raise ModelError("fake model has no more responses")
        return self.responses.pop(0), {"seconds": 0.0}


class OllamaModel(Model):
    def __init__(self, name, host="http://localhost:11434"):
        self.name, self.host = name, host

    def generate(self, system, user):
        body = {"model": self.name, "stream": False, "options": {"num_ctx": 32768, "temperature": 0},
                "messages": [{"role": "system", "content": system}, {"role": "user", "content": user}]}
        t0 = time.time()
        req = urllib.request.Request(self.host + "/api/chat", data=json.dumps(body).encode(),
                                     headers={"Content-Type": "application/json"})
        try:
            with urllib.request.urlopen(req, timeout=3600) as r:
                resp = json.load(r)
        except Exception as e:  # connection refused, timeout, bad JSON
            raise ModelError(f"ollama {self.name}: {e}")
        text = re.sub(r"<think>.*?</think>\s*", "", resp["message"]["content"], flags=re.S)
        return text, {"seconds": round(time.time() - t0, 1), "tokens": resp.get("eval_count")}


class ClaudeModel(Model):
    TOOLS_OFF = "Bash,Read,Write,Edit,Glob,Grep,WebSearch,WebFetch,Agent,NotebookEdit"

    def __init__(self, name):
        self.name = name

    def generate(self, system, user):
        if shutil.which("claude") is None:
            raise ModelError("claude CLI not found")
        prompt = system + "\n\n====================  TASK  ====================\n" + user
        t0 = time.time()
        try:
            p = subprocess.run(["claude", "-p", "--model", self.name, "--output-format", "text",
                                "--no-session-persistence", "--disallowedTools", self.TOOLS_OFF],
                               input=prompt, capture_output=True, text=True, timeout=1800)
        except subprocess.TimeoutExpired:
            raise ModelError(f"claude {self.name}: timeout")
        if p.returncode != 0:
            raise ModelError(f"claude {self.name}: exit {p.returncode}: {p.stderr.strip()[:500]}")
        return p.stdout, {"seconds": round(time.time() - t0, 1)}


def make_model(spec: str) -> Model:
    kind, _, name = spec.partition(":")
    if kind == "ollama":
        return OllamaModel(name)
    if kind == "claude":
        return ClaudeModel(name)
    raise SystemExit(f"unknown model spec {spec!r}; use ollama:NAME or claude:NAME")


FILE_RE = re.compile(r"^-{3,}\s*file:\s*(\S+?)\s*-{3,}\s*$", re.M)


def split_files(text: str) -> dict:
    """Split `--- file: path ---` sections; strip surrounding ``` fences; ignore any preamble."""
    parts = FILE_RE.split(text)
    out = {}
    for i in range(1, len(parts), 2):
        path, body = parts[i], parts[i + 1]
        body = re.sub(r"^\s*```\w*\n", "", body)         # opening fence right after the marker
        body = re.sub(r"\n```\s*(\n|$)[\s\S]*$", "\n", body)  # closing fence and anything after it
        if path.startswith("app/") and path.endswith(".alder"):
            out[path] = body.strip("\n") + "\n"
    return out


def run_check(alder: pathlib.Path, app_root: pathlib.Path):
    p = subprocess.run([str(alder), "check", str(app_root), "--json"], capture_output=True, text=True)
    diags = []
    for line in p.stdout.splitlines():
        line = line.strip()
        if line.startswith("{"):
            try:
                diags.append(json.loads(line))
            except json.JSONDecodeError:
                pass
    return p.returncode, diags, p.stderr


def render_diags(diags, sources):
    out = []
    for d in diags:
        out.append(f"{d['file']}:{d['line']}:{d['col']}: {d['severity']}[{d['code']}]: {d['message']}")
        lines = sources.get(d["file"], "").split("\n")
        if 1 <= d["line"] <= len(lines):
            src = lines[d["line"] - 1]
            width = max(1, d["endCol"] - d["col"]) if d.get("endLine") == d["line"] else 1
            out.append("  " + src)
            out.append("  " + " " * max(0, d["col"] - 1) + "^" * width)
        if d.get("fix"):
            out.append("  fix: " + d["fix"]["replace"].replace("\n", "⏎"))
    return "\n".join(out)


def retry_prompt(task: str, round_dir: pathlib.Path, diags, stderr: str, nfiles: int) -> str:
    sources = {}
    for f in (round_dir / "app").rglob("*.alder"):
        sources[str(f.relative_to(round_dir))] = f.read_text()
    if nfiles == 0:
        feedback = "Your previous answer contained no files (no `--- file: app/... ---` sections were found)."
    elif diags:
        feedback = "The compiler reported these errors in your previous answer:\n\n" + render_diags(diags, sources)
    else:
        feedback = "The compiler could not check your previous answer:\n\n" + stderr.strip()
    return (task + "\n\n====================  COMPILER FEEDBACK  ====================\n" + feedback +
            "\n\nFix every error. Output every file again, in full, each preceded by `--- file: app/<path> ---`. No explanation, only files.")


def write_round(round_dir: pathlib.Path, prev_dir, text: str, files: dict):
    (round_dir / "app").mkdir(parents=True, exist_ok=True)
    if prev_dir is not None and (prev_dir / "app").exists():
        shutil.copytree(prev_dir / "app", round_dir / "app", dirs_exist_ok=True)
    (round_dir / "output.md").write_text(text[:MAX_OUTPUT])
    for path, body in files.items():
        p = round_dir / path
        p.parent.mkdir(parents=True, exist_ok=True)
        p.write_text(body)
    return sum(1 for _ in (round_dir / "app").rglob("*.alder"))


def run_one(exp: pathlib.Path, model: Model, model_label: str, task_name: str, task_text: str,
            system: str, alder: pathlib.Path, retries: int = 1) -> dict:
    run_dir = exp / "runs" / model_label / task_name
    if run_dir.exists():
        shutil.rmtree(run_dir)
    summary = {"model": model_label, "task": task_name, "retries": retries, "error": None, "rounds": [], "final_pass": False}
    user = task_text
    prev = None
    for n in range(retries + 1):
        try:
            text, meta = model.generate(system, user)
        except ModelError as e:
            summary["error"] = str(e)
            break
        round_dir = run_dir / f"round{n}"
        files = split_files(text)
        nfiles = write_round(round_dir, prev, text, files)
        exit_code, diags, stderr = run_check(alder, round_dir)
        (round_dir / "check.json").write_text("\n".join(json.dumps(d) for d in diags) + ("\n" if diags else ""))
        codes = {}
        for d in diags:
            if d["severity"] == "error":
                codes[d["code"]] = codes.get(d["code"], 0) + 1
        summary["rounds"].append({"n": n, "files": nfiles, "exit": exit_code,
                                  "errors": sum(codes.values()), "warnings": sum(1 for d in diags if d["severity"] == "warning"),
                                  "codes": codes, "seconds": meta.get("seconds", 0.0), "tokens": meta.get("tokens"), "stderr": stderr[:2000]})
        if exit_code == 0:
            summary["final_pass"] = True
            break
        user = retry_prompt(task_text, round_dir, diags, stderr, nfiles)
        prev = round_dir
    run_dir.mkdir(parents=True, exist_ok=True)
    (run_dir / "summary.json").write_text(json.dumps(summary, indent=1) + "\n")
    return summary


def main(argv=None):
    ap = argparse.ArgumentParser()
    ap.add_argument("--exp", required=True)
    ap.add_argument("--model", required=True, action="append", help="ollama:NAME or claude:NAME; repeatable")
    ap.add_argument("--task", required=True, action="append", help="task name (file tasks/<name>.md); repeatable")
    ap.add_argument("--retries", type=int, default=1)
    ap.add_argument("--prompt", default="prompt.md")
    ap.add_argument("--alder", default=None, help="path to the alder binary (default: build it)")
    a = ap.parse_args(argv)
    exp = pathlib.Path(a.exp)
    system = (exp / a.prompt).read_text()
    alder = pathlib.Path(a.alder) if a.alder else build_alder(exp)
    failed = 0
    for spec in a.model:
        model = make_model(spec)
        label = spec.replace(":", "-").replace("/", "-")
        for task in a.task:
            task_text = (exp / "tasks" / f"{task}.md").read_text()
            s = run_one(exp, model, label, task, task_text, system, alder, a.retries)
            status = "ERROR " + s["error"] if s["error"] else ("pass" if s["final_pass"] else "fail")
            counts = " → ".join(str(r["errors"]) for r in s["rounds"])
            print(f"{label} {task}: {status} ({counts} errors)")
            if s["error"]:
                failed += 1
    return 1 if failed else 0


def build_alder(exp: pathlib.Path) -> pathlib.Path:
    root = pathlib.Path(__file__).resolve().parent.parent
    out = exp / "alder"
    subprocess.run(["go", "build", "-o", str(out), "./cmd/alder"], cwd=root, check=True)
    return out


if __name__ == "__main__":
    sys.exit(main())
