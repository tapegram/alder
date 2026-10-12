#!/usr/bin/env python3
"""Generate → alder check → feed diagnostics back → retry.

Usage:
  run_loop.py --exp DIR --model ollama:NAME|claude:NAME --task NAME [--retries 1] [--prompt prompt.md] [--alder BIN]
              [--base APPDIR] [--build [--test] [--hidden DIR]] [--migrate] [--adapters] [--num-ctx N]
              [--condition LABEL] [--sample LABEL]

Writes DIR/runs/<model>/<task>/round<N>/{output.md,check.json,app/...} and DIR/runs/<model>/<task>/summary.json.
With --build, a round whose check is clean also runs `alder build --emit` (output in round<N>/build/);
with --test too, a round that built also runs `alder test --json` and passes only if every test does.
--base DIR seeds round 0 from DIR/app (and DIR/adapters and DIR/migrations, when there are) as
committed (`git archive HEAD`), never from the working tree. --migrate: a round whose only check
errors are E0019 (the tables changed since the last migration) runs `alder migrate` and checks
again, recording the new file's `-- step:` / `-- DROPS DATA:` lines in the round. --adapters (opt-in) also accepts adapters/<dir>/<name>.go
sections (one package level: capability X of module A.B is adapters/a/, func BX) and lets build and test run them. WARNING: it executes Go the model wrote, unsandboxed. --num-ctx sets Ollama's context window.
--hidden DIR (with --build): after the final round, if it checked clean and built, DIR/<task>/*.alder
run on a scratch copy of it (round<N>/hidden/) with `alder test -run '^Hidden '`; summary.hidden and
summary.hidden_pass record them, never a prompt. --condition and --sample are labels (harness.condition,
harness.sample); --sample also suffixes the run directory: runs/<model>/<task>-<LABEL>/.
summary.json's `outcome` is pass, fail, compiler_bug or harness_error (see README "Outcomes").
"""
import argparse, io, json, os, pathlib, posixpath, re, shutil, signal, subprocess, sys, tarfile, tempfile, time, urllib.request

MAX_OUTPUT = 200_000


class ModelError(Exception):
    pass


class Model:
    def generate(self, system: str, user: str):
        raise NotImplementedError

    def summary_fields(self) -> dict:
        """Fields this model adds to summary.json (how it was run)."""
        return {}


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
    NUM_CTX = 32768
    CTX_MARGIN = 512   # prompt + answer this close to num_ctx: the prompt's start may have been dropped
    NUM_PREDICT = 8192   # E7: cap the answer; a capped answer (done_reason "length") is a failed, truncated round

    def __init__(self, name, host="http://localhost:11434", num_ctx=None):
        self.name, self.host = name, host
        if num_ctx is not None:   # --num-ctx: this instance's window (and overflow threshold)
            self.NUM_CTX = num_ctx

    def generate(self, system, user):
        body = {"model": self.name, "stream": False, "options": {"num_ctx": self.NUM_CTX, "temperature": 0, "num_predict": self.NUM_PREDICT},
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
                "prompt_eval_count": resp.get("prompt_eval_count"), "eval_count": resp.get("eval_count"),
                "done_reason": resp.get("done_reason")}
        if resp.get("done_reason") == "length":   # stopped by num_predict: the answer is cut off
            meta["truncated"] = True
        # Ollama silently drops the start of a prompt longer than num_ctx; the count it evaluated tells:
        # well below the estimate, or prompt + answer at the context's end.
        estimated = (len(system) + len(user)) // 4
        pec, ec = resp.get("prompt_eval_count"), resp.get("eval_count")
        warnings = []
        if not isinstance(pec, int) or pec < 0.9 * estimated:
            warnings.append(f"prompt may have been truncated (prompt_eval_count={pec}, estimated={estimated})")
        if isinstance(pec, int) and isinstance(ec, int) and pec + ec >= self.NUM_CTX - self.CTX_MARGIN:
            warnings.append(f"context may have overflowed (prompt_eval_count={pec} + eval_count={ec} "
                            f">= num_ctx {self.NUM_CTX} - {self.CTX_MARGIN})")
        if warnings:
            meta["warning"] = "; ".join(warnings)
        return text, meta


class ClaudeModel(Model):
    # Isolation (see README "Models"): no session saved; no user, project or local settings files,
    # so none of their hooks, enabled plugins or permissions; no MCP server but those of
    # --mcp-config (none); no tools at all (--tools ""); no skills or slash commands.
    ISOLATION = ["--no-session-persistence", "--setting-sources", "", "--strict-mcp-config",
                 "--tools", "", "--disable-slash-commands"]
    SYSTEM_PLACEHOLDER = "<system prompt>"

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

    def argv(self, system):
        """The command line of one generation; the task goes on stdin."""
        return (["claude", "-p", "--model", self.name, "--output-format", "text", "--system-prompt", system]
                + self.ISOLATION)

    def summary_fields(self):
        return {"claude_argv": self.argv(self.SYSTEM_PLACEHOLDER)}

    def generate(self, system, user):
        if shutil.which("claude") is None:
            raise ModelError("claude CLI not found")
        # An empty temp cwd: no CLAUDE.md, no project settings or memory leak into the run.
        cwd = tempfile.mkdtemp(prefix="alder-claude-")
        t0 = time.time()
        try:
            p = subprocess.run(self.argv(system), input=user, capture_output=True, text=True, timeout=1800, cwd=cwd)
        except subprocess.TimeoutExpired:
            raise ModelError(f"claude {self.name}: timeout")
        finally:
            shutil.rmtree(cwd, ignore_errors=True)
        if p.returncode != 0:
            raise ModelError(f"claude {self.name}: exit {p.returncode}: {p.stderr.strip()[:500]}")
        return p.stdout, {"seconds": round(time.time() - t0, 1), "claude_version": self.version(), "model": self.name}


def make_model(spec: str, num_ctx=None) -> Model:
    """`num_ctx` (--num-ctx) applies to Ollama models; Claude ignores it."""
    kind, _, name = spec.partition(":")
    if kind == "ollama":
        return OllamaModel(name, num_ctx=num_ctx)
    if kind == "claude":
        return ClaudeModel(name)
    raise SystemExit(f"unknown model spec {spec!r}; use ollama:NAME or claude:NAME")


FILE_RE = re.compile(r"^-{3,}\s*file:\s*(\S+?)\s*-{3,}\s*$", re.M)

STDLIB_REASON = "standard-library module"
# only adapters/<dir>/<name>.go: one package per module's first segment (capability X of module A.B
# is adapters/a/, func BX; book chapter 7), and adapters/internal/<dir>/<name>.go, a helper package the
# adapters share (the book's layout); nothing deeper, and not the removed flat adapters/<name>.go
ADAPTER_RE = re.compile(r"^adapters/(?:internal/)?[A-Za-z0-9_]+/[A-Za-z0-9_]+\.go$")


def adapter_files(root: pathlib.Path):
    """The Go files under root/adapters that ADAPTER_RE accepts, sorted by their path relative to root."""
    root = pathlib.Path(root)
    return sorted((f for f in (root / "adapters").rglob("*.go")
                   if f.is_file() and ADAPTER_RE.match(f.relative_to(root).as_posix())),
                  key=lambda f: f.relative_to(root).as_posix())


def stdlib_names(root=None):
    """The standard library's module names (`stdlib/*.alder` stems) plus `stdlib` itself, read from the
    repository's stdlib/ directory next to this directory. Computed once, at harness start."""
    root = pathlib.Path(root) if root else pathlib.Path(__file__).resolve().parent.parent / "stdlib"
    return frozenset(f.stem for f in root.glob("*.alder")) | {"stdlib"}


STDLIB_NAMES = stdlib_names()


def classify_path(path: str, adapters: bool = False):
    """(safe path, None), or (None, reason): reason is STDLIB_REASON for `app/stdlib/…` and for a module
    named exactly like a standard-library module (`app/List.alder`), else "" (traversal, absolute, wrong
    extension). A module under a stdlib name's directory (`app/Time/Slots.alder`, `module Time.Slots`)
    is legal Alder and accepted. With `adapters`, `adapters/<dir>/<name>.go` is a safe path too."""
    if not path or path.startswith("/") or "\\" in path:
        return None, ""
    p = posixpath.normpath(path)
    if p.startswith("/") or p == ".." or p.startswith("../") or ".." in p.split("/"):
        return None, ""
    if p.endswith(".go"):
        return (p, None) if adapters and ADAPTER_RE.match(p) else (None, "")
    if not p.endswith(".alder"):
        return None, ""
    if not p.startswith("app/"):
        p = "app/" + p
    segs = p.split("/")
    if segs[1] == "stdlib" or (len(segs) == 2 and segs[1].removesuffix(".alder") in STDLIB_NAMES):
        return None, STDLIB_REASON
    return p, None


def normalize_path(path: str, adapters: bool = False):
    """Return a safe `app/...alder` path (or `adapters/<dir>/<name>.go` with `adapters`), or None if the path must be rejected."""
    return classify_path(path, adapters)[0]


def split_answer(text: str, adapters: bool = False, cut_off: bool = False):
    """Split `--- file: path ---` sections; strip surrounding ``` fences; ignore any preamble.

    Returns (files, rejected, cut_off_file): files maps safe paths to contents; rejected lists the raw paths
    that were refused (traversal, absolute, wrong extension; a standard-library module carries its reason).
    With `cut_off` (the answer stopped at the token cap) the last section is incomplete: it is not in files,
    and cut_off_file names it (None when that section was rejected anyway or there is none)."""
    parts = FILE_RE.split(text)
    out, rejected, cut = {}, [], None
    last = len(parts) - 2
    for i in range(1, len(parts), 2):
        raw, body = parts[i], parts[i + 1]
        body = re.sub(r"^\s*```\w*\n", "", body)         # opening fence right after the marker
        body = re.sub(r"\n```\s*(\n|$)[\s\S]*$", "\n", body)  # closing fence and anything after it
        path, reason = classify_path(raw, adapters)
        if path is None:
            rejected.append(f"{raw} ({reason})" if reason else raw)
        elif cut_off and i == last:
            cut = path
        else:
            out[path] = body.strip("\n") + "\n"
    return out, rejected, cut


def split_files(text: str, adapters: bool = False):
    """split_answer without the cut-off handling: (files, rejected)."""
    files, rejected, _ = split_answer(text, adapters)
    return files, rejected


DIAG_KEYS = ("code", "severity", "file", "line", "col")


ALDER_EXITS = (0, 1, 2)   # alder's own exit codes; anything else (a signal, a crash) is the harness's problem


def unknown_exit(what: str, code: int):
    """The harness-error reason for an exit code alder never uses, else None."""
    return None if code in ALDER_EXITS else f"alder {what} exited {code} (not 0, 1 or 2)"


class Hang(str):
    """A harness reason that is a timeout: alder check, build or test ran past its limit, which is almost
    certainly the compiler hanging (run_one records it as compiler_bug, kind "hang")."""


def run_proc(argv, timeout, **kw):
    """subprocess.run(argv, capture_output=True, text=True, timeout=timeout, **kw), except that on
    the timeout the whole process group is killed (the process runs in a session of its own), so a
    child it started (go's compiler under `alder build`) cannot keep the pipes open and the harness
    waiting. Raises subprocess.TimeoutExpired after the kill, as subprocess.run does."""
    p = subprocess.Popen(argv, stdout=subprocess.PIPE, stderr=subprocess.PIPE, stdin=subprocess.DEVNULL,
                         text=True, start_new_session=True, **kw)
    try:
        out, err = p.communicate(timeout=timeout)
    except subprocess.TimeoutExpired:
        try:
            os.killpg(p.pid, signal.SIGKILL)
        except ProcessLookupError:
            pass
        p.communicate()
        raise
    return subprocess.CompletedProcess(argv, p.returncode, out, err)


def run_check(alder: pathlib.Path, app_root: pathlib.Path, timeout: float = 600):
    """`alder check --json` on app_root. Returns (exit, diagnostics, stderr, harness_reason):
    harness_reason is None normally, and a string when the check did not run as a check (a
    timeout — a Hang —, a binary that cannot start, an exit code other than 0, 1 or 2); exit is
    then 2, or the exit code."""
    try:
        p = run_proc([str(alder), "check", str(app_root), "--json"], timeout)
    except subprocess.TimeoutExpired:
        msg = f"{alder}: check timed out after {timeout}s"
        return 2, [], msg, Hang(msg)
    except OSError as e:
        msg = f"{alder}: {e}"
        return 2, [], msg, msg
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
    return p.returncode, diags, p.stderr, unknown_exit("check", p.returncode)


def run_build(alder: pathlib.Path, round_dir: pathlib.Path, timeout: float = 900):
    """`alder build --emit` in round_dir, so the generated Go lands in round_dir/build/.

    Returns (exit, stderr, stdout, harness_reason): harness_reason is None normally, and a string
    for a timeout (a Hang), a binary that cannot start, or an exit code other than 0, 1 or 2 (exit
    is then None, or the exit code)."""
    rd = round_dir.resolve()
    try:
        p = run_proc([str(pathlib.Path(alder).resolve()), "build", "--emit", str(rd)], timeout, cwd=rd)
    except subprocess.TimeoutExpired:
        msg = f"{alder}: build timed out after {timeout}s"
        return None, msg, "", Hang(msg)
    except OSError as e:
        msg = f"{alder}: {e}"
        return None, msg, "", msg
    return p.returncode, p.stderr, p.stdout, unknown_exit("build", p.returncode)


TEST_OUTPUT_MAX = 4000   # characters of `alder test`'s text report kept in summary.json and the retry prompt


def parse_tests(stdout: str):
    """The test results in `alder test --json`'s stdout: one object per test (module, name, ok,
    and left, right, error, trace, ms when present). Other objects ({"error": …},
    {"diagnostics": […]}) and other lines are skipped."""
    results = []
    for line in stdout.splitlines():
        line = line.strip()
        if not line.startswith("{"):
            continue
        try:
            d = json.loads(line)
        except json.JSONDecodeError:
            continue
        if isinstance(d, dict) and "name" in d and "ok" in d:
            results.append(d)
    return results


def run_test(alder: pathlib.Path, round_dir: pathlib.Path, timeout: float = 600):
    """`alder test --json <round_dir>` in round_dir, its own process group (run_proc).

    Returns (exit, results, stderr, stdout, harness_reason), as run_build: harness_reason is None
    normally, and a string for a timeout (a Hang), a binary that cannot start, or an exit code
    other than 0, 1 or 2 (exit is then None, or the exit code)."""
    rd = round_dir.resolve()
    try:
        p = run_proc([str(pathlib.Path(alder).resolve()), "test", "--json", str(rd)], timeout, cwd=rd)
    except subprocess.TimeoutExpired:
        msg = f"{alder}: test timed out after {timeout}s"
        return None, [], msg, "", Hang(msg)
    except OSError as e:
        msg = f"{alder}: {e}"
        return None, [], msg, "", msg
    return p.returncode, parse_tests(p.stdout), p.stderr, p.stdout, unknown_exit("test", p.returncode)


def test_report(alder: pathlib.Path, round_dir: pathlib.Path, timeout: float = 600) -> str:
    """`alder test <round_dir>` without --json, run once more after a failing `--json` run: the
    text report (stdout, then stderr) is what the retry prompt carries; never raises."""
    rd = round_dir.resolve()
    try:
        p = run_proc([str(pathlib.Path(alder).resolve()), "test", str(rd)], timeout, cwd=rd)
    except subprocess.TimeoutExpired:
        return f"alder test timed out after {timeout}s"
    except OSError as e:
        return f"{alder}: {e}"
    return (p.stdout.rstrip("\n") + ("\n" + p.stderr.rstrip("\n") if p.stderr.strip() else "")).strip("\n") + "\n"


def run_migrate(alder: pathlib.Path, round_dir: pathlib.Path, timeout: float = 600):
    """`alder migrate <round_dir>` in round_dir. Returns (exit, record, stdout, stderr,
    harness_reason): record is {"exit", "file", "steps", "drops_data"} — the migration file it
    wrote (its name under migrations/, or None) and that file's `-- step:` and `-- DROPS DATA:`
    header lines —; harness_reason as run_build's."""
    rd = round_dir.resolve()
    before = {f.name for f in (rd / "migrations").glob("*.sql")}
    try:
        p = run_proc([str(pathlib.Path(alder).resolve()), "migrate", str(rd)], timeout, cwd=rd)
    except subprocess.TimeoutExpired:
        msg = f"{alder}: migrate timed out after {timeout}s"
        return None, {"exit": None, "file": None, "steps": [], "drops_data": []}, "", msg, Hang(msg)
    except OSError as e:
        msg = f"{alder}: {e}"
        return None, {"exit": None, "file": None, "steps": [], "drops_data": []}, "", msg, msg
    new = sorted({f.name for f in (rd / "migrations").glob("*.sql")} - before)
    record = {"exit": p.returncode, "file": new[-1] if new else None, "steps": [], "drops_data": []}
    if new:
        for line in (rd / "migrations" / new[-1]).read_text().splitlines():
            if not line.startswith("--"):
                break
            if line.startswith("-- step: "):
                record["steps"].append(line[len("-- step: "):])
            elif line.startswith("-- DROPS DATA: "):
                record["drops_data"].append(line[len("-- DROPS DATA: "):])
    return p.returncode, record, p.stdout, p.stderr, unknown_exit("migrate", p.returncode)


HIDDEN_RUN = "^Hidden "   # `alder test -run`: the tests of module Hidden (`Hidden "name"`) only
TEST_HEADER_RE = re.compile(r'^test\s+"((?:[^"\\]|\\.)*)"\s*=', re.M)
MODULE_RE = re.compile(r"^module\s+(\S+)", re.M)


def hidden_test_names(files):
    """The names of the `test "…" =` declarations in those of `files` (paths) whose module is
    `Hidden`, read statically (the count a hidden module that does not compile is scored by), as
    the source spells them, escapes kept: `alder test --json` reports a name so (`the form \\"Add\\"`).
    None when no file is module Hidden."""
    names, found = [], False
    for f in files:
        text = pathlib.Path(f).read_text()
        m = MODULE_RE.search(text)
        if m and m.group(1) == "Hidden":
            found = True
            names += TEST_HEADER_RE.findall(text)
    return names if found else None


def hidden_failed(names, reason, **extra):
    """Every hidden test failed for one reason (a hidden module that does not compile, a crash)."""
    return {"passed": 0, "failed": len(names), "reason": reason,
            "results": [{"name": n, "ok": False, "reason": reason} for n in names], **extra}


def final_round_skip(rnd):
    """Why the hidden tests cannot run on this final round (None when they can): it must have
    checked clean and built."""
    if rnd is None:
        return "no round (the model gave no answer)"
    if rnd.get("truncated"):
        return "the final round was cut off"
    if rnd.get("exit") != 0 or rnd.get("errors"):
        return f"the final round did not check clean ({rnd.get('errors', 0)} errors)"
    if "build" not in rnd:
        return "the final round was not built"
    if not rnd["build"].get("ok"):
        return "the final round did not build"
    return None


def run_hidden(alder: pathlib.Path, round_dir: pathlib.Path, hidden_src: pathlib.Path):
    """The hidden acceptance tests on a round that checked clean and built: a scratch copy of the
    round (round_dir/hidden/{app,adapters,migrations}; the round's own files are untouched) with
    hidden_src/*.alder copied into its app/, `alder check`, then `alder test -run '^Hidden ' --json`.
    Returns the record {"passed", "failed", "results", "exit"}: a hidden module that does not check
    or a run that crashes fails every hidden test (counted statically) with the first diagnostic or
    the error as `reason`."""
    scratch = round_dir / "hidden"
    if scratch.exists():
        shutil.rmtree(scratch)
    shutil.copytree(round_dir / "app", scratch / "app")
    for extra in ("adapters", "migrations"):   # the base's adapters and the round's migrations (--base, --migrate)
        if (round_dir / extra).is_dir():
            shutil.copytree(round_dir / extra, scratch / extra)
    copied, replaced = [], []
    for f in sorted(pathlib.Path(hidden_src).glob("*.alder")):
        dst = scratch / "app" / f.name
        if dst.exists():
            replaced.append(f"app/{f.name}")
        shutil.copyfile(f, dst)
        copied.append(dst)
    names = hidden_test_names(copied)
    if names is None:   # an authoring error, never a valid empty success
        return {"skipped": "no module Hidden"}
    if not names:
        return {"skipped": "no hidden tests"}
    extra = {"replaced": replaced} if replaced else {}
    exit_code, diags, stderr, reason = run_check(alder, scratch)
    if reason is not None:
        return hidden_failed(names, f"alder check: {reason}", exit=None, check={"exit": exit_code, "codes": {}}, **extra)
    errors = [d for d in diags if d["severity"] == "error"]
    if exit_code != 0 or errors:
        codes = {}
        for d in errors:
            codes[d["code"]] = codes.get(d["code"], 0) + 1
        first = errors[0] if errors else None
        why = (f"{first['file']}:{first['line']}:{first['col']}: error[{first['code']}]: {first['message']}" if first
               else (stderr.strip().splitlines() or [f"alder check exited {exit_code}"])[0])
        return hidden_failed(names, why, exit=None, check={"exit": exit_code, "codes": codes}, **extra)
    sd = scratch.resolve()
    try:
        p = run_proc([str(pathlib.Path(alder).resolve()), "test", "-run", HIDDEN_RUN, "--json", str(sd)], 600, cwd=sd)
    except subprocess.TimeoutExpired:
        return hidden_failed(names, "alder test timed out after 600s", exit=None, **extra)
    except OSError as e:
        return hidden_failed(names, f"{alder}: {e}", exit=None, **extra)
    if p.returncode != 0:
        (scratch / "test.stderr.txt").write_text(p.stderr + (("\n--- stdout ---\n" + p.stdout) if p.stdout.strip() else ""))
    results = [r for r in parse_tests(p.stdout) if r.get("module", "Hidden") == "Hidden"]
    if p.returncode not in (0, 1) or (not results and names):   # a crash outside any test, a build problem
        err = next((d["error"] for d in map(_json_or_none, p.stdout.splitlines()) if isinstance(d, dict) and "error" in d), None)
        why = err or (p.stderr.strip().splitlines() or [f"alder test exited {p.returncode}"])[0]
        return hidden_failed(names, why, exit=p.returncode, **extra)
    seen = {r.get("name") for r in results}
    missing = [n for n in names if n not in seen]
    if missing:   # a crash or a timeout stops the run: the tests after it ran not, and count as failed
        failing = [r for r in results if r.get("ok") is not True]
        why = (next((r.get("error") for r in reversed(failing) if r.get("error")), None)
               or (p.stderr.strip().splitlines() or [f"alder test exited {p.returncode}"])[-1])
        results = results + [{"name": n, "ok": False, "reason": f"not run: {why}"} for n in missing]
    return {"passed": sum(1 for r in results if r.get("ok") is True),
            "failed": sum(1 for r in results if r.get("ok") is not True),
            "results": results, "exit": p.returncode, **extra}


def _json_or_none(line):
    line = line.strip()
    if not line.startswith("{"):
        return None
    try:
        return json.loads(line)
    except json.JSONDecodeError:
        return None


MIGRATE_REFUSED = ("Your change to the tables needs a migration, and `alder migrate` could not write one for it; "
                   "change the tables so it can (its output is below).")


BASE_NOT_IN_GIT = ("`--base` must be inside a git repository "
                   "(the harness seeds from `git archive HEAD`, never the working tree)")


def _git(cwd, *args) -> bytes:
    return subprocess.run(["git", "-C", str(cwd), *args], capture_output=True, check=True).stdout


def export_base(base: pathlib.Path):
    """Export `<base>/app`, and `<base>/adapters` and `<base>/migrations` when the commit has them
    (the Go adapters `alder build` and `alder test` need for an app with capabilities; the
    migrations an app with a database keeps its tables by), as committed at HEAD of the
    git repository containing base into a new temporary directory, and return (the exported base — tmp/<base relative to the repository> —,
    the commit it was exported from): HEAD is resolved once and that commit archived. Uncommitted
    edits and untracked files are never seen; when `<base>/app` has some, a warning says so on
    stderr. Exits 2 when base does not exist, is not inside a git repository, or HEAD has no
    `<base>/app`. The caller removes the directory (remove_export)."""
    base = pathlib.Path(base)
    if not base.is_dir():
        print(f"run_loop.py: --base {base} does not exist (or is not a directory)", file=sys.stderr)
        raise SystemExit(2)
    try:
        top = pathlib.Path(_git(base, "rev-parse", "--show-toplevel").decode().strip()).resolve()
        commit = _git(top, "rev-parse", "HEAD").decode().strip()
    except (OSError, subprocess.CalledProcessError):
        print(f"run_loop.py: {BASE_NOT_IN_GIT}", file=sys.stderr)
        raise SystemExit(2)
    rel = base.resolve().relative_to(top).as_posix()
    app = "app" if rel == "." else rel + "/app"
    paths = [app]
    for extra in ("adapters", "migrations"):   # the Go adapters; the app's migrations (E8)
        path = extra if rel == "." else rel + "/" + extra
        if subprocess.run(["git", "-C", str(top), "cat-file", "-e", f"{commit}:{path}"], capture_output=True).returncode == 0:
            paths.append(path)
    try:
        tar = _git(top, "archive", "--format=tar", commit, "--", *paths)
    except subprocess.CalledProcessError as e:
        print(f"run_loop.py: --base {base}: git archive {commit} -- {app}: {e.stderr.decode().strip()}", file=sys.stderr)
        raise SystemExit(2)
    try:
        dirty = _git(top, "status", "--porcelain", "--", app).decode().strip()
    except (OSError, subprocess.CalledProcessError):
        dirty = ""
    if dirty:
        print(f"run_loop.py: warning: uncommitted changes under {base}/app are ignored "
              f"(round 0 is seeded from commit {commit[:12]})", file=sys.stderr)
    out = pathlib.Path(tempfile.mkdtemp(prefix="alder-base-"))
    with tarfile.open(fileobj=io.BytesIO(tar)) as tf:
        tf.extractall(out, filter="data")
    _exports[out / rel] = out
    return out / rel, commit


def remove_export(exported: pathlib.Path):
    """Remove the temporary directory export_base made (exported is what it returned)."""
    tmp = _exports.pop(pathlib.Path(exported), None)
    if tmp is not None:
        shutil.rmtree(tmp, ignore_errors=True)


_exports = {}   # export_base's result → the temporary directory holding it


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
TESTS_FAILED = "The program builds but its tests fail; fix the program or the tests so every test passes."
TRUNCATED = ("Your previous answer was cut off after {n} tokens; output only the Alder files "
             "(`--- file: app/<path> ---`), nothing else.")


def go_refusal_hint(rejected, round_dir: pathlib.Path) -> str:
    """For each rejected `adapters/<dir>/<name>.go` (a run without --adapters): the notice that keeps a model
    from replacing a capability with a password in the code (E7: Haiku's T3)."""
    out = []
    for raw in rejected:
        p = posixpath.normpath(raw)
        if not ADAPTER_RE.match(p) or p.startswith("adapters/internal/") or p.endswith("_test.go"):
            continue   # a helper or a test file is no capability's adapter
        cap = _cap_name(pathlib.PurePosixPath(p).stem)
        notice = f"`{p}` was not accepted (this run does not take Go files); "
        provided = sorted({_cap_name(f.stem) for f in adapter_files(round_dir)
                           if f.parent.parent.name == "adapters" and f.parent.name != "internal" and not f.name.endswith("_test.go")})
        if (round_dir / p).is_file():   # the base provides this very adapter
            out.append(notice + f"the `{cap}` adapter is provided \u2014 keep the capability and do not replace it "
                       "with a password in the code")
        elif provided:   # the base provides others, not this one
            names = " and ".join(f"`{c}`" for c in provided)
            out.append(notice + f"the {names} adapter{'s are' if len(provided) > 1 else ' is'} provided; `{cap}` is not: "
                       "declare the capability and leave its Go side unwritten")
        else:
            out.append(notice + "declare the capability and leave its Go side unwritten")
    return "\n\n".join(out)


def _cap_name(stem: str) -> str:
    """The capability an adapter file is for, by its name: `password` (adapters/auth/password.go) → `Password`;
    a test file's stem names the file it tests (`password_test` → `Password`)."""
    stem = stem.removesuffix("_test")
    return stem[:1].upper() + stem[1:]


PROVIDED_ADAPTERS = ("These adapters are provided, not yours (from the base app); emit one only to replace it, "
                     "and otherwise leave it out:")


def retry_prompt(task: str, round_dir: pathlib.Path, diags, stderr: str, nfiles: int, rejected=(), build_output=None,
                 test_output=None, truncated=None, cut_off_file=None, show_adapters=False, provided=None,
                 migrate_output=None) -> str:
    """`build_output`, when given, is what `alder build --emit` printed when it refused (exit 1) a clean program;
    `test_output`, the text report of `alder test` when tests of a program that built failed (exit 1);
    `truncated`, the token cap when the model's answer was cut off by it (Ollama's done_reason "length");
    `cut_off_file`, the path of the file it was cut off in (not written); `show_adapters`, carry the round's
    adapters/<dir>/*.go in the previous answer (a --adapters run); `provided`, the base's adapters ({path: text}):
    one the round still has unchanged is listed apart, after the model's files, as provided, not yours;
    `migrate_output`, what `alder migrate` printed when it refused (exit 1) the change (a --migrate run)."""
    sources = {}
    for f in (round_dir / "app").rglob("*.alder"):
        sources[str(f.relative_to(round_dir))] = f.read_text()
    if nfiles == 0 and not cut_off_file:
        feedback = "Your previous answer contained no files (no `--- file: app/... ---` sections were found)."
    elif diags:
        feedback = "The compiler reported these errors in your previous answer:\n\n" + render_diags(diags, sources)
    elif build_output is not None:
        feedback = BUILD_REFUSED
    elif test_output is not None:
        feedback = TESTS_FAILED
    elif truncated is not None:
        feedback = TRUNCATED.format(n=truncated)
    else:
        feedback = "The compiler could not check your previous answer:\n\n" + stderr.strip()
    if build_output is not None:
        if diags:
            feedback += "\n\n" + BUILD_REFUSED
        feedback += "\n\n====================  BUILD OUTPUT  ====================\n" + build_output.strip()
    if migrate_output is not None:
        feedback += "\n\n" + MIGRATE_REFUSED + "\n\n====================  MIGRATE OUTPUT  ====================\n" + migrate_output.strip()
    if test_output is not None:
        if diags:
            feedback += "\n\n" + TESTS_FAILED
        feedback += "\n\n====================  TEST OUTPUT  ====================\n" + test_output.strip()
    if truncated is not None and (nfiles == 0 or diags or build_output is not None or test_output is not None):
        feedback += "\n\n" + TRUNCATED.format(n=truncated)
    if cut_off_file:
        feedback += f"\n\n`{cut_off_file}` was cut off and not written; emit it again in full."
    if rejected:
        feedback += ("\n\nThese paths were rejected: " + ", ".join(rejected) +
                     "; files must be `app/<Module>.alder`.")
        hint = go_refusal_hint(rejected, round_dir)
        if hint:
            feedback += "\n\n" + hint
    theirs = {}
    if show_adapters:
        for f in adapter_files(round_dir):
            path, text = f.relative_to(round_dir).as_posix(), f.read_text()
            if provided and provided.get(path) == text:
                theirs[path] = text
            else:
                sources[path] = text
    previous = ""
    if sources or theirs:  # every model call is a fresh process: the retry must carry the files it asks to fix
        section = file_section
        previous = "\n\n====================  YOUR PREVIOUS ANSWER  ====================\n" + section(sources)
        if theirs:
            previous += "\n" + PROVIDED_ADAPTERS + "\n\n" + section(theirs)
    return (task + previous + "\n\n====================  COMPILER FEEDBACK  ====================\n" + feedback +
            "\n\nFix every error. Output every file again, in full, each preceded by `--- file: app/<path> ---`. No explanation, only files.")


def file_section(files) -> str:
    """{path: text} as `--- file: path ---` sections, by path: how a prompt shows files."""
    return "\n".join(f"--- file: {path} ---\n{files[path].rstrip(chr(10))}\n" for path in sorted(files))


BASE_APP = "====================  THE APP YOU ARE CHANGING  ===================="


def first_prompt(task: str, base_dir, show_adapters=False, provided=None) -> str:
    """Round 0's prompt: the task, and with a `--base` (base_dir, the export) every app/**/*.alder of
    it under THE APP YOU ARE CHANGING (the retry prompt's file form), then, in an --adapters run, the
    base's adapters apart as provided. Without a base, the task alone."""
    if base_dir is None:
        return task
    app = {f.relative_to(base_dir).as_posix(): f.read_text() for f in (pathlib.Path(base_dir) / "app").rglob("*.alder")}
    out = task + "\n\n" + BASE_APP + "\n" + file_section(app)
    if show_adapters and provided:
        out += "\n" + PROVIDED_ADAPTERS + "\n\n" + file_section(provided)
    return out


def write_round(round_dir: pathlib.Path, prev_dir, text: str, files: dict, base_dir=None):
    """Write the round: prev_dir's app/ (and adapters/) overlaid with the model's files; the
    base's migrations/ (base_dir, the export), never a previous round's: a round's own `alder
    migrate` (--migrate) adds one file to the base's, so each round's migration is its change."""
    (round_dir / "app").mkdir(parents=True, exist_ok=True)
    if base_dir is not None and (base_dir / "migrations").is_dir():
        shutil.copytree(base_dir / "migrations", round_dir / "migrations", dirs_exist_ok=True)
    if prev_dir is not None and (prev_dir / "app").exists():
        shutil.copytree(prev_dir / "app", round_dir / "app", dirs_exist_ok=True)
    if prev_dir is not None and (prev_dir / "adapters").is_dir():   # from --base; the model writes only app/
        shutil.copytree(prev_dir / "adapters", round_dir / "adapters", dirs_exist_ok=True)
    (round_dir / "output.md").write_text(text[:MAX_OUTPUT])
    roots = [(round_dir / "app").resolve(), (round_dir / "adapters").resolve()]
    skipped = []
    for path, body in files.items():
        p = (round_dir / path).resolve()
        if not any(p.is_relative_to(r) for r in roots):
            skipped.append(path)
            continue
        p.parent.mkdir(parents=True, exist_ok=True)
        p.write_text(body)
    return sum(1 for _ in (round_dir / "app").rglob("*.alder")), skipped


def run_one(exp: pathlib.Path, model: Model, model_label: str, task_name: str, task_text: str,
            system: str, alder: pathlib.Path, retries: int = 1, base=None, build: bool = False, seed=None,
            test: bool = False, adapters: bool = False, migrate: bool = False, hidden=None,
            condition=None, sample=None) -> dict:
    """`base`, if given, is a directory inside a git repository whose `app/` as committed (`git archive
    HEAD`, never the working tree) seeds round 0 (for tasks that modify an existing app). `seed`, if
    given, is that export already made, `export_base(base)` (the exported base and its commit): main
    exports once for every task; without it run_one exports (and removes the export) itself.

    With `build`, a clean round also runs `alder build --emit`; the run passes only if that succeeds.
    A build that exits 2 (internal error, generated Go that does not vet) after a clean check is a
    compiler bug, not the model's: it is recorded (`compiler_bug`, round<N>/build.stderr.txt) and the
    task stops there, with no retry. Exit 1 (holes, or anything else the build refuses with
    diagnostics) is an ordinary failed round: the retry prompt carries the build's output.

    With `test` (requires `build`; ValueError otherwise), a round that built also runs `alder test
    --json`; its results go in `rnd["tests"]` ({"passed", "failed", "results", "exit"}). Exit 0 is the
    pass (zero tests included). Exit 1 is an ordinary failed round: `rnd["test_output"]` and the
    retry prompt (under TEST OUTPUT) carry the text report of a second, non-JSON run; that includes
    `alder test` refusing the app's adapters or finding that they do not compile (exit 1 since M5d:
    the model's program; a missing adapter is not refused, only warned about). Exit 2 (a
    build problem or a crash outside any test, after a clean build) is a compiler bug, kind "test"
    — or "hang" when its stderr says the tests timed out, as when the 600 s limit kills it —
    with its stderr in round<N>/test.stderr.txt; the task stops.

    With `migrate`, a round whose check fails with E0019 alone (the app's tables changed since the
    base's last migration) runs `alder migrate` (run_migrate) and checks again; `rnd["migrate"]`
    records it (the file, its `-- step:` and `-- DROPS DATA:` lines, the codes of the check before
    it) and the round goes on with the second check. Exit 1 (a change it refuses, such as a new
    non-null column without a default) is an ordinary failed round whose retry prompt carries its
    output; exit 2 with "internal error" is a compiler bug (kind "migrate"); a hang, kind "hang".

    A check that exits 2 with "internal error" on stderr (a checker panic) is a compiler bug too
    (kind "internal_error", round<N>/check.stderr.txt), and so is a check or build that times out
    (kind "hang": a hang is the compiler's). `summary["compiler_bug_kind"]` says which ("build" for
    a build exit 2).

    `outcome` is set on every path: pass, fail, compiler_bug, or harness_error when the harness
    itself failed (the model was unavailable; check or build could not start, or exited with a code
    alder never uses), with the reason in `harness_error`; the task stops there.

    With `hidden` (a directory; requires `build`), after the final round — the pass, or the last
    round run — the hidden acceptance tests `hidden/<task_name>/*.alder` run on a scratch copy of it
    (run_hidden) when it checked clean and built; `summary["hidden"]` is their record, or
    {"skipped": reason}, and `summary["hidden_pass"]` is "k/n" (None when skipped). They never
    reach a prompt, and `outcome` stays the own tests'. `condition` and `sample` are labels recorded
    as `harness.condition` and `harness.sample`; `sample` also names the run directory `<task>-<sample>`."""
    if test and not build:
        raise ValueError("--test requires --build")
    if hidden is not None and not build:
        raise ValueError("--hidden requires --build")
    if base and seed is None:
        seed = export_base(base)
        try:
            return run_one(exp, model, model_label, task_name, task_text, system, alder, retries, base, build, seed=seed, test=test, adapters=adapters,
                           migrate=migrate, hidden=hidden, condition=condition, sample=sample)
        finally:
            remove_export(seed[0])
    run_dir = exp / "runs" / model_label / (f"{task_name}-{sample}" if sample else task_name)
    if run_dir.exists():
        shutil.rmtree(run_dir)
    summary = {"model": model_label, "task": task_name, "retries": retries, "base": str(base) if base else None,
               "harness": {"build": build, "retries": retries, **({"test": True} if test else {}), **({"adapters": True} if adapters else {}),
                           **({"migrate": True} if migrate else {}),
                           **({"hidden": str(hidden)} if hidden is not None else {}),
                           **({"condition": condition} if condition is not None else {}),
                           **({"sample": sample} if sample is not None else {}),
                           **({"num_ctx": model.NUM_CTX, "num_predict": model.NUM_PREDICT} if isinstance(model, OllamaModel) else {})}, "error": None, "rounds": [], "final_pass": False,
               "outcome": "fail"}
    if seed is not None:
        summary["base_commit"] = seed[1]
    summary.update(model.summary_fields())
    prev = pathlib.Path(seed[0]) if seed is not None else None
    provided = {}   # the base's adapters, as exported: shown apart in a retry (provided, not the model's)
    if prev is not None and (prev / "adapters").is_dir():
        provided = {f.relative_to(prev).as_posix(): f.read_text() for f in adapter_files(prev)}
    user = first_prompt(task_text, prev, show_adapters=adapters, provided=provided)   # with --base: the app it changes

    def harness_error(reason):
        summary["outcome"] = "harness_error"
        summary["harness_error"] = reason

    def compiler_bug(kind):
        summary["compiler_bug"] = True
        summary["outcome"] = "compiler_bug"
        summary["compiler_bug_kind"] = kind

    for n in range(retries + 1):
        try:
            text, meta = model.generate(system, user)
        except ModelError as e:
            summary["error"] = str(e)
            harness_error(f"model: {e}")
            break
        round_dir = run_dir / f"round{n}"
        cut = bool(meta.get("truncated")) or meta.get("done_reason") == "length"
        files, rejected, cut_off_file = split_answer(text, adapters=adapters, cut_off=cut)
        nfiles, skipped = write_round(round_dir, prev, text, files, base_dir=pathlib.Path(seed[0]) if seed is not None else None)
        rejected = rejected + skipped
        exit_code, diags, stderr, reason = run_check(alder, round_dir)
        (round_dir / "check.json").write_text("\n".join(json.dumps(d) for d in diags) + ("\n" if diags else ""))
        codes = {}
        for d in diags:
            if d["severity"] == "error":
                codes[d["code"]] = codes.get(d["code"], 0) + 1
        rnd = {"n": n, "files": nfiles, "exit": exit_code,
               "errors": sum(codes.values()), "warnings": sum(1 for d in diags if d["severity"] == "warning"),
               "codes": codes, "rejected": rejected, "seconds": meta.get("seconds", 0.0), "tokens": meta.get("tokens"), "stderr": stderr[:2000]}
        for k in ("prompt_eval_count", "eval_count", "done_reason", "truncated", "warning", "claude_version", "model"):
            if k in meta:
                rnd[k] = meta[k]
        if cut_off_file:
            rnd["cut_off_file"] = cut_off_file
        summary["rounds"].append(rnd)
        if isinstance(reason, Hang):   # the checker hung on the model's program
            (round_dir / "check.stderr.txt").write_text(stderr)
            compiler_bug("hang")
            break
        if reason is not None:   # the check did not run as a check: no verdict on the model's program
            harness_error(reason)
            break
        if exit_code == 2 and "internal error" in stderr:   # the checker panicked: never the model's fault
            (round_dir / "check.stderr.txt").write_text(stderr)
            compiler_bug("internal_error")
            break
        migrate_output = None
        if migrate and exit_code == 1 and set(codes) == {"E0019"}:   # only the tables are ahead of the migrations
            mexit, record, mstdout, mstderr, reason = run_migrate(alder, round_dir)
            record["codes_before"] = codes
            rnd["migrate"] = record
            if mexit != 0:
                (round_dir / "migrate.stderr.txt").write_text(mstderr + (("\n--- stdout ---\n" + mstdout) if mstdout.strip() else ""))
                if isinstance(reason, Hang):
                    compiler_bug("hang")
                    break
                if reason is not None:
                    harness_error(reason)
                    break
                if mexit != 1:
                    if "internal error" in mstderr:
                        compiler_bug("migrate")
                    else:
                        harness_error(f"alder migrate exited {mexit}: {mstderr.strip()[:500]}")
                    break
                migrate_output = (mstdout + mstderr)[:TEST_OUTPUT_MAX]
            else:   # check again: the round goes on with the migrated app
                exit_code, diags, stderr, reason = run_check(alder, round_dir)
                (round_dir / "check.json").write_text("\n".join(json.dumps(d) for d in diags) + ("\n" if diags else ""))
                codes = {}
                for d in diags:
                    if d["severity"] == "error":
                        codes[d["code"]] = codes.get(d["code"], 0) + 1
                rnd.update({"exit": exit_code, "errors": sum(codes.values()),
                            "warnings": sum(1 for d in diags if d["severity"] == "warning"), "codes": codes, "stderr": stderr[:2000]})
                if isinstance(reason, Hang):
                    (round_dir / "check.stderr.txt").write_text(stderr)
                    compiler_bug("hang")
                    break
                if reason is not None:
                    harness_error(reason)
                    break
        clean = exit_code == 0 and rnd["errors"] == 0 and not rnd.get("truncated")   # a cut-off answer fails its round
        build_output = None
        if clean and build:
            bexit, bstderr, bstdout, reason = run_build(alder, round_dir)
            rnd["build"] = {"ok": bexit == 0, "exit": bexit, "stderr": bstderr[-4000:]}
            if bexit != 0:
                build_output = bstderr + (("\n" + bstdout) if bstdout.strip() else "")
                (round_dir / "build.stderr.txt").write_text(bstderr + (("\n--- stdout ---\n" + bstdout) if bstdout.strip() else ""))
                if isinstance(reason, Hang):   # the build hung: the compiler's
                    compiler_bug("hang")
                    break
                if reason is not None:   # did not start, or killed: neither the model's nor the compiler's
                    harness_error(reason)
                    break
                if bexit != 1:   # 2: internal error / does not vet — the compiler's fault
                    compiler_bug("build")
                    break
                clean = False
        test_output = None
        if clean and test:
            texit, results, tstderr, tstdout, reason = run_test(alder, round_dir)
            rnd["tests"] = {"passed": sum(1 for r in results if r.get("ok") is True),
                            "failed": sum(1 for r in results if r.get("ok") is not True),
                            "results": results, "exit": texit}
            if texit != 0:
                (round_dir / "test.stderr.txt").write_text(tstderr + (("\n--- stdout ---\n" + tstdout) if tstdout.strip() else ""))
                if isinstance(reason, Hang):   # alder test ran past the harness's limit
                    compiler_bug("hang")
                    break
                if reason is not None:
                    harness_error(reason)
                    break
                if texit != 1:   # 2: a build problem or a crash outside any test, after a clean build
                    compiler_bug("hang" if "timed out" in tstderr else "test")
                    break
                else:
                    test_output = test_report(alder, round_dir)[:TEST_OUTPUT_MAX]
                    rnd["test_output"] = test_output
                    clean = False
        if clean:
            summary["final_pass"] = True
            summary["outcome"] = "pass"
            break
        user = retry_prompt(task_text, round_dir, diags, stderr, nfiles, rejected, build_output=build_output,
                            test_output=test_output, truncated=getattr(model, "NUM_PREDICT", None) if rnd.get("truncated") else None,
                            cut_off_file=cut_off_file, show_adapters=adapters, provided=provided, migrate_output=migrate_output)
        prev = round_dir
    if hidden is not None:   # after the final round: never seen by the model
        last = summary["rounds"][-1] if summary["rounds"] else None
        skip = final_round_skip(last)
        if skip is None:
            h = run_hidden(alder, run_dir / f"round{last['n']}", pathlib.Path(hidden) / task_name)
            summary["hidden"] = h
            summary["hidden_pass"] = None if "skipped" in h else f"{h['passed']}/{h['passed'] + h['failed']}"
        else:
            summary["hidden"] = {"skipped": skip}
            summary["hidden_pass"] = None
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
    ap.add_argument("--base", default=None, help="directory (inside a git repository) whose app/ as committed at HEAD seeds round 0 "
                                                 "(modification tasks, e.g. examples/todos for t2); never the working tree")
    ap.add_argument("--build", action="store_true", help="after a clean check, run `alder build --emit`; the run passes only if it succeeds (exit 2 = compiler bug, stop)")
    ap.add_argument("--test", action="store_true", help="with --build: after a successful build, run `alder test --json`; the run passes only if every test passes "
                                                        "(exit 1 = retry with the report, exit 2 = compiler bug, stop)")
    ap.add_argument("--migrate", action="store_true", help="a round whose only check errors are E0019 runs `alder migrate` and checks again; "
                                                           "the new file's step and DROPS DATA lines are recorded in the round (E8)")
    ap.add_argument("--adapters", action="store_true", help="WARNING: executes Go code the model wrote, on this machine, unsandboxed. Opt-in: accept "
                                                            "`--- file: adapters/<dir>/<name>.go ---` sections (one package level, as chapter 7's rule names it: capability X of "
                                                            "module A.B is adapters/a/, func BX), write them into the round's adapters/ and let build/test run them")
    ap.add_argument("--hidden", default=None, metavar="DIR", help="with --build: after the final round, run the hidden acceptance tests DIR/<task>/*.alder "
                                                                  "on a scratch copy of it (`alder test -run '^Hidden '`); recorded as summary.hidden (E8)")
    ap.add_argument("--condition", default=None, metavar="LABEL", help="a label for the experimental condition, recorded as harness.condition")
    ap.add_argument("--sample", default=None, metavar="LABEL", help="a sample label: the run goes to runs/<model>/<task>-LABEL/, recorded as harness.sample")
    ap.add_argument("--num-ctx", type=int, default=None, help=f"Ollama's num_ctx for this invocation (default {OllamaModel.NUM_CTX}); recorded as harness.num_ctx")
    a = ap.parse_args(argv)
    if a.test and not a.build:
        ap.error("--test requires --build")
    if a.hidden is not None:
        if not a.build:
            ap.error("--hidden requires --build")
        for task in a.task:
            if not sorted(pathlib.Path(a.hidden, task).glob("*.alder")):
                ap.error(f"--hidden {a.hidden}: no {task}/*.alder")
    if a.sample is not None and not re.fullmatch(r"[A-Za-z0-9_.-]+", a.sample):
        ap.error("--sample LABEL: letters, digits, '_', '.', '-' only")
    seed = None
    if a.base:   # first: a --base outside git is a usage error before any work
        seed = export_base(pathlib.Path(a.base))
    try:
        return run_all(a, seed)
    finally:
        if seed is not None:
            remove_export(seed[0])


def run_all(a, seed):
    exp = pathlib.Path(a.exp)
    system = (exp / a.prompt).read_text()
    alder = pathlib.Path(a.alder) if a.alder else build_alder(exp)
    failed = 0
    for spec in a.model:
        model = make_model(spec, a.num_ctx)
        label = spec.replace(":", "-").replace("/", "-")
        for task in a.task:
            task_text = (exp / "tasks" / f"{task}.md").read_text()
            s = run_one(exp, model, label, task, task_text, system, alder, a.retries, base=a.base, build=a.build, seed=seed,
                        test=a.test, adapters=a.adapters, migrate=a.migrate, hidden=a.hidden, condition=a.condition, sample=a.sample)
            status = ("ERROR " + s["error"] if s["error"]
                      else f"harness-error ({s['harness_error']})" if s["outcome"] == "harness_error"
                      else s["outcome"].replace("_", "-"))
            counts = " → ".join(str(r["errors"]) + ("" if "build" not in r else " (build ok)" if r["build"]["ok"] else " (build FAILED)")
                                + ("" if "tests" not in r else f" (tests {r['tests']['passed']}/{r['tests']['passed'] + r['tests']['failed']})")
                                for r in s["rounds"])
            for r in s["rounds"]:
                if r.get("warning"):
                    print(f"  warning: round {r['n']}: {r['warning']}", file=sys.stderr)
            hid = "" if "hidden" not in s else (f"; hidden {s['hidden_pass']}" if s["hidden_pass"] else f"; hidden skipped: {s['hidden']['skipped']}")
            print(f"{label} {task}{'-' + a.sample if a.sample else ''}: {status} ({counts} errors{hid})")
            if s["outcome"] == "harness_error":   # the model unavailable, or check/build broken: not a measurement
                failed += 1
    return 1 if failed else 0


def build_alder(exp: pathlib.Path) -> pathlib.Path:
    root = pathlib.Path(__file__).resolve().parent.parent
    out = exp / "alder"
    subprocess.run(["go", "build", "-o", str(out), "./cmd/alder"], cwd=root, check=True)
    return out


if __name__ == "__main__":
    sys.exit(main())
