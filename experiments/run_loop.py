#!/usr/bin/env python3
"""Generate → alder check → feed diagnostics back → retry.

Usage:
  run_loop.py --exp DIR --model ollama:NAME|claude:NAME --task NAME [--retries 1] [--prompt prompt.md] [--alder BIN] [--base APPDIR] [--build]

Writes DIR/runs/<model>/<task>/round<N>/{output.md,check.json,app/...} and DIR/runs/<model>/<task>/summary.json.
With --build, a round whose check is clean also runs `alder build --emit` (output in round<N>/build/).
"""
import argparse, json, pathlib, posixpath, re, shutil, subprocess, sys, tempfile, time, urllib.request

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
        meta = {"seconds": round(time.time() - t0, 1), "tokens": resp.get("eval_count"),
                "prompt_eval_count": resp.get("prompt_eval_count"), "eval_count": resp.get("eval_count")}
        # Ollama silently drops the start of a prompt longer than num_ctx; the count it evaluated tells.
        estimated = (len(system) + len(user)) // 4
        pec = resp.get("prompt_eval_count")
        if not isinstance(pec, int) or pec < 0.9 * estimated:
            meta["warning"] = f"prompt may have been truncated (prompt_eval_count={pec}, estimated={estimated})"
        return text, meta


class ClaudeModel(Model):
    TOOLS_OFF = "Bash,Read,Write,Edit,Glob,Grep,WebSearch,WebFetch,Agent,NotebookEdit"

    _version = None   # `claude --version`, once per process

    def __init__(self, name):
        self.name = name

    @classmethod
    def version(cls):
        if cls._version is None:
            try:
                p = subprocess.run(["claude", "--version"], capture_output=True, text=True, timeout=60)
                cls._version = p.stdout.strip() if p.returncode == 0 else f"unknown (exit {p.returncode})"
            except (OSError, subprocess.TimeoutExpired) as e:
                cls._version = f"unknown ({e})"
        return cls._version

    def generate(self, system, user):
        if shutil.which("claude") is None:
            raise ModelError("claude CLI not found")
        # An empty temp cwd: no CLAUDE.md, no project settings or memory leak into the run.
        cwd = tempfile.mkdtemp(prefix="alder-claude-")
        t0 = time.time()
        try:
            p = subprocess.run(["claude", "-p", "--model", self.name, "--output-format", "text",
                                "--system-prompt", system,
                                "--no-session-persistence", "--disallowedTools", self.TOOLS_OFF],
                               input=user, capture_output=True, text=True, timeout=1800, cwd=cwd)
        except subprocess.TimeoutExpired:
            raise ModelError(f"claude {self.name}: timeout")
        finally:
            shutil.rmtree(cwd, ignore_errors=True)
        if p.returncode != 0:
            raise ModelError(f"claude {self.name}: exit {p.returncode}: {p.stderr.strip()[:500]}")
        return p.stdout, {"seconds": round(time.time() - t0, 1), "claude_version": self.version(), "model": self.name}


def make_model(spec: str) -> Model:
    kind, _, name = spec.partition(":")
    if kind == "ollama":
        return OllamaModel(name)
    if kind == "claude":
        return ClaudeModel(name)
    raise SystemExit(f"unknown model spec {spec!r}; use ollama:NAME or claude:NAME")


FILE_RE = re.compile(r"^-{3,}\s*file:\s*(\S+?)\s*-{3,}\s*$", re.M)


def normalize_path(path: str):
    """Return a safe `app/...alder` path, or None if the path must be rejected."""
    if not path or path.startswith("/") or "\\" in path:
        return None
    p = posixpath.normpath(path)
    if p.startswith("/") or p == ".." or p.startswith("../") or ".." in p.split("/"):
        return None
    if not p.endswith(".alder"):
        return None
    if not p.startswith("app/"):
        p = "app/" + p
    return p


def split_files(text: str):
    """Split `--- file: path ---` sections; strip surrounding ``` fences; ignore any preamble.

    Returns (files, rejected): files maps safe `app/...alder` paths to contents;
    rejected lists the raw paths that were refused (traversal, absolute, non-.alder)."""
    parts = FILE_RE.split(text)
    out, rejected = {}, []
    for i in range(1, len(parts), 2):
        raw, body = parts[i], parts[i + 1]
        body = re.sub(r"^\s*```\w*\n", "", body)         # opening fence right after the marker
        body = re.sub(r"\n```\s*(\n|$)[\s\S]*$", "\n", body)  # closing fence and anything after it
        path = normalize_path(raw)
        if path is None:
            rejected.append(raw)
        else:
            out[path] = body.strip("\n") + "\n"
    return out, rejected


DIAG_KEYS = ("code", "severity", "file", "line", "col")


def run_check(alder: pathlib.Path, app_root: pathlib.Path):
    try:
        p = subprocess.run([str(alder), "check", str(app_root), "--json"], capture_output=True, text=True, timeout=600)
    except subprocess.TimeoutExpired:
        return 2, [], f"{alder}: check timed out after 600s"
    except OSError as e:
        return 2, [], f"{alder}: {e}"
    diags = []
    for line in p.stdout.splitlines():
        line = line.strip()
        if line.startswith("{"):
            try:
                d = json.loads(line)
            except json.JSONDecodeError:
                continue
            if isinstance(d, dict) and all(k in d for k in DIAG_KEYS):
                diags.append(d)
    return p.returncode, diags, p.stderr


def run_build(alder: pathlib.Path, round_dir: pathlib.Path):
    """`alder build --emit` in round_dir, so the generated Go lands in round_dir/build/.

    Returns (exit, stderr, stdout); exit 2 for a timeout or a missing binary, like `alder` itself."""
    rd = round_dir.resolve()
    try:
        p = subprocess.run([str(pathlib.Path(alder).resolve()), "build", "--emit", str(rd)],
                           cwd=rd, capture_output=True, text=True, timeout=900)
    except subprocess.TimeoutExpired:
        return 2, f"{alder}: build timed out after 900s", ""
    except OSError as e:
        return 2, f"{alder}: {e}", ""
    return p.returncode, p.stderr, p.stdout


def render_diags(diags, sources):
    out = []
    for d in diags:
        out.append(f"{d['file']}:{d['line']}:{d['col']}: {d['severity']}[{d['code']}]: {d['message']}")
        lines = sources.get(d["file"], "").split("\n")
        if 1 <= d["line"] <= len(lines):
            src = lines[d["line"] - 1]
            width = max(1, d.get("endCol", d["col"]) - d["col"]) if d.get("endLine") == d["line"] else 1
            out.append("  " + src)
            out.append("  " + " " * max(0, d["col"] - 1) + "^" * width)
        if isinstance(d.get("fix"), dict):
            out.append("  fix: " + str(d["fix"].get("replace", "")).replace("\n", "⏎"))
    return "\n".join(out)


BUILD_REFUSED = "The program type-checks but `alder build` refused it; fix the cause below."


def retry_prompt(task: str, round_dir: pathlib.Path, diags, stderr: str, nfiles: int, rejected=(), build_output=None) -> str:
    """`build_output`, when given, is what `alder build --emit` printed when it refused (exit 1) a clean program."""
    sources = {}
    for f in (round_dir / "app").rglob("*.alder"):
        sources[str(f.relative_to(round_dir))] = f.read_text()
    if nfiles == 0:
        feedback = "Your previous answer contained no files (no `--- file: app/... ---` sections were found)."
    elif diags:
        feedback = "The compiler reported these errors in your previous answer:\n\n" + render_diags(diags, sources)
    elif build_output is not None:
        feedback = BUILD_REFUSED
    else:
        feedback = "The compiler could not check your previous answer:\n\n" + stderr.strip()
    if build_output is not None:
        if diags:
            feedback += "\n\n" + BUILD_REFUSED
        feedback += "\n\n====================  BUILD OUTPUT  ====================\n" + build_output.strip()
    if rejected:
        feedback += ("\n\nThese paths were rejected: " + ", ".join(rejected) +
                     "; files must be `app/<Module>.alder`.")
    previous = ""
    if sources:  # every model call is a fresh process: the retry must carry the files it asks to fix
        previous = ("\n\n====================  YOUR PREVIOUS ANSWER  ====================\n" +
                    "\n".join(f"--- file: {path} ---\n{sources[path].rstrip(chr(10))}\n" for path in sorted(sources)))
    return (task + previous + "\n\n====================  COMPILER FEEDBACK  ====================\n" + feedback +
            "\n\nFix every error. Output every file again, in full, each preceded by `--- file: app/<path> ---`. No explanation, only files.")


def write_round(round_dir: pathlib.Path, prev_dir, text: str, files: dict):
    (round_dir / "app").mkdir(parents=True, exist_ok=True)
    if prev_dir is not None and (prev_dir / "app").exists():
        shutil.copytree(prev_dir / "app", round_dir / "app", dirs_exist_ok=True)
    (round_dir / "output.md").write_text(text[:MAX_OUTPUT])
    app_root = (round_dir / "app").resolve()
    skipped = []
    for path, body in files.items():
        p = (round_dir / path).resolve()
        if not p.is_relative_to(app_root):
            skipped.append(path)
            continue
        p.parent.mkdir(parents=True, exist_ok=True)
        p.write_text(body)
    return sum(1 for _ in (round_dir / "app").rglob("*.alder")), skipped


def run_one(exp: pathlib.Path, model: Model, model_label: str, task_name: str, task_text: str,
            system: str, alder: pathlib.Path, retries: int = 1, base=None, build: bool = False) -> dict:
    """`base`, if given, is a directory whose `app/` seeds round 0 (for tasks that modify an existing app).

    With `build`, a clean round also runs `alder build --emit`; the run passes only if that succeeds.
    A build that exits 2 (internal error, generated Go that does not vet) after a clean check is a
    compiler bug, not the model's: it is recorded (`compiler_bug`, round<N>/build.stderr.txt) and the
    task stops there, with no retry. Exit 1 (holes, or anything else the build refuses with
    diagnostics) is an ordinary failed round: the retry prompt carries the build's output."""
    run_dir = exp / "runs" / model_label / task_name
    if run_dir.exists():
        shutil.rmtree(run_dir)
    summary = {"model": model_label, "task": task_name, "retries": retries, "base": str(base) if base else None,
               "harness": {"build": build, "retries": retries}, "error": None, "rounds": [], "final_pass": False}
    user = task_text
    prev = pathlib.Path(base) if base else None
    for n in range(retries + 1):
        try:
            text, meta = model.generate(system, user)
        except ModelError as e:
            summary["error"] = str(e)
            break
        round_dir = run_dir / f"round{n}"
        files, rejected = split_files(text)
        nfiles, skipped = write_round(round_dir, prev, text, files)
        rejected = rejected + skipped
        exit_code, diags, stderr = run_check(alder, round_dir)
        (round_dir / "check.json").write_text("\n".join(json.dumps(d) for d in diags) + ("\n" if diags else ""))
        codes = {}
        for d in diags:
            if d["severity"] == "error":
                codes[d["code"]] = codes.get(d["code"], 0) + 1
        rnd = {"n": n, "files": nfiles, "exit": exit_code,
               "errors": sum(codes.values()), "warnings": sum(1 for d in diags if d["severity"] == "warning"),
               "codes": codes, "rejected": rejected, "seconds": meta.get("seconds", 0.0), "tokens": meta.get("tokens"), "stderr": stderr[:2000]}
        for k in ("prompt_eval_count", "eval_count", "warning", "claude_version", "model"):
            if k in meta:
                rnd[k] = meta[k]
        summary["rounds"].append(rnd)
        clean = exit_code == 0 and rnd["errors"] == 0
        build_output = None
        if clean and build:
            bexit, bstderr, bstdout = run_build(alder, round_dir)
            rnd["build"] = {"ok": bexit == 0, "exit": bexit, "stderr": bstderr[-4000:]}
            if bexit != 0:
                build_output = bstderr + (("\n" + bstdout) if bstdout.strip() else "")
                (round_dir / "build.stderr.txt").write_text(bstderr + (("\n--- stdout ---\n" + bstdout) if bstdout.strip() else ""))
                if bexit != 1:   # 2: internal error / does not vet — the compiler's fault
                    summary["compiler_bug"] = True
                    break
                clean = False
        if clean:
            summary["final_pass"] = True
            break
        user = retry_prompt(task_text, round_dir, diags, stderr, nfiles, rejected, build_output=build_output)
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
    ap.add_argument("--base", default=None, help="directory whose app/ seeds round 0 (modification tasks, e.g. examples/todos for t2)")
    ap.add_argument("--build", action="store_true", help="after a clean check, run `alder build --emit`; the run passes only if it succeeds (exit 2 = compiler bug, stop)")
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
            s = run_one(exp, model, label, task, task_text, system, alder, a.retries, base=a.base, build=a.build)
            status = ("ERROR " + s["error"] if s["error"] else "compiler-bug" if s.get("compiler_bug")
                      else "pass" if s["final_pass"] else "fail")
            counts = " → ".join(str(r["errors"]) + ("" if "build" not in r else " (build ok)" if r["build"]["ok"] else " (build FAILED)")
                                for r in s["rounds"])
            for r in s["rounds"]:
                if r.get("warning"):
                    print(f"  warning: round {r['n']}: {r['warning']}", file=sys.stderr)
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
