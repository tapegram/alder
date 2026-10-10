import json, os, pathlib, shutil, subprocess, sys, tempfile, unittest
sys.path.insert(0, str(pathlib.Path(__file__).resolve().parent))
import run_loop as rl

GOOD = "--- file: app/App.alder ---\nmodule App\n\nsession { }\n\nerror : Error -> Html Never\nerror err = <h1>x</h1>\n\nnotFound : Html Never\nnotFound = <h1>x</h1>\n"

class SplitTests(unittest.TestCase):
    def test_split_tolerates_fences_and_preamble(self):
        text = "Here is the app:\n```alder\n--- file: app/A.alder ---\nmodule A\n```\n\n```\n--- file: app/B/C.alder ---\nmodule B.C\n```\n"
        files, rejected = rl.split_files(text)
        self.assertEqual(rejected, [])
        self.assertEqual(sorted(files), ["app/A.alder", "app/B/C.alder"])
        self.assertEqual(files["app/A.alder"].strip(), "module A")
        self.assertEqual(files["app/B/C.alder"].strip(), "module B.C")

    def test_no_markers(self):
        self.assertEqual(rl.split_files("I cannot do this."), ({}, []))

    def test_split_rejects_traversal(self):
        text = "--- file: app/../../x.alder ---\nmodule X\n--- file: /etc/x.alder ---\nmodule Y\n--- file: app/A/../B.alder ---\nmodule B\n--- file: app/n.txt ---\nhi\n"
        files, rejected = rl.split_files(text)
        self.assertEqual(sorted(files), ["app/B.alder"])
        self.assertEqual(rejected, ["app/../../x.alder", "/etc/x.alder", "app/n.txt"])

    def test_split_normalizes_bare_and_dot_paths(self):
        files, rejected = rl.split_files("--- file: App.alder ---\nmodule App\n--- file: ./app/B.alder ---\nmodule B\n--- file: Posts/Index.alder ---\nmodule Posts.Index\n")
        self.assertEqual(sorted(files), ["app/App.alder", "app/B.alder", "app/Posts/Index.alder"])
        self.assertEqual(rejected, [])

class FakeChecker:
    """Writes a fake `alder` script whose exit code and JSON are scripted per call."""
    def __init__(self, tmp, script):
        self.path = pathlib.Path(tmp) / "alder"
        self.count = pathlib.Path(tmp) / "count"
        self.count.write_text("0")
        lines = ["#!/bin/sh", f'n=$(cat {self.count}); echo $((n+1)) > {self.count}']
        for i, (exit_code, stdout, stderr) in enumerate(script):
            lines.append(f'if [ "$n" = "{i}" ]; then printf %b {json.dumps(stdout)}; printf %b {json.dumps(stderr)} 1>&2; exit {exit_code}; fi')
        lines.append("exit 2")
        self.path.write_text("\n".join(lines) + "\n")
        self.path.chmod(0o755)

ERR = json.dumps({"code": "E0003", "severity": "error", "file": "app/App.alder", "line": 3, "col": 1, "endLine": 3, "endCol": 2, "message": "expected Int, got Text"}) + "\n"

class LoopTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.mkdtemp()
        self.exp = pathlib.Path(self.tmp) / "exp"
        (self.exp / "tasks").mkdir(parents=True)

    def tearDown(self):
        shutil.rmtree(self.tmp)

    def test_retry_overlays_previous_round(self):
        chk = FakeChecker(self.tmp, [(1, ERR, ""), (0, "", "")])
        model = rl.FakeModel([GOOD + "--- file: app/X.alder ---\nmodule X\n", "--- file: app/X.alder ---\nmodule X\n\nfixed : Int\nfixed = 1\n"])
        s = rl.run_one(self.exp, model, "fake", "t", "task text", "system", chk.path, retries=1)
        self.assertEqual([r["files"] for r in s["rounds"]], [2, 2])   # round 1 keeps App.alder from round 0
        self.assertTrue((self.exp / "runs/fake/t/round1/app/App.alder").exists())
        self.assertIn("fixed", (self.exp / "runs/fake/t/round1/app/X.alder").read_text())
        self.assertTrue(s["final_pass"])
        self.assertEqual(s["rounds"][0]["codes"], {"E0003": 1})

    @unittest.skipUnless(shutil.which("git"), "git is not on PATH")
    def test_base_seeds_round0(self):
        # A modification task (T2) emits only changed files; round 0 must start from the base app
        # (as committed: --base reads `git archive HEAD`, test_base_exports_from_git_not_the_tree).
        base = pathlib.Path(self.tmp) / "base"
        (base / "app" / "Todos").mkdir(parents=True)
        (base / "app" / "App.alder").write_text("module App\n")
        (base / "app" / "Todos" / "Index.alder").write_text("module Todos.Index\n-- old\n")
        git(base, "init", "-q")
        git(base, "add", ".")
        git(base, "commit", "-q", "-m", "base")
        chk = FakeChecker(self.tmp, [(0, "", "")])
        model = rl.FakeModel(["--- file: app/Todos/Index.alder ---\nmodule Todos.Index\n-- new\n"
                              "--- file: app/Profile.alder ---\nmodule Profile\n"])
        s = rl.run_one(self.exp, model, "fake", "t", "task", "sys", chk.path, retries=0, base=base)
        self.assertEqual(s["rounds"][0]["files"], 3)
        self.assertTrue((self.exp / "runs/fake/t/round0/app/App.alder").exists())
        self.assertIn("new", (self.exp / "runs/fake/t/round0/app/Todos/Index.alder").read_text())
        self.assertEqual((base / "app" / "Todos" / "Index.alder").read_text(), "module Todos.Index\n-- old\n")
        self.assertEqual(s["base"], str(base))
        self.assertEqual(json.loads((self.exp / "runs/fake/t/summary.json").read_text())["base"], str(base))
        s2 = rl.run_one(self.exp, rl.FakeModel([GOOD]), "fake", "t2", "task", "sys", FakeChecker(self.tmp, [(0, "", "")]).path, retries=0)
        self.assertIsNone(s2["base"])

    def test_retry_prompt_includes_previous_files(self):
        # Every model call is a fresh process: the retry must carry the files it asks the model to fix.
        chk = FakeChecker(self.tmp, [(1, ERR, ""), (0, "", "")])
        model = rl.FakeModel(["--- file: app/App.alder ---\nmodule App\n\nvalueA : Int\nvalueA = \"a\"\n"
                              "--- file: app/Pages/Home.alder ---\nmodule Pages.Home\n\nvalueB : Int\nvalueB = 2\n",
                              GOOD])
        rl.run_one(self.exp, model, "fake", "t", "TASK TEXT", "sys", chk.path, retries=1)
        p = model.prompts[1]
        self.assertIn("--- file: app/App.alder ---\nmodule App\n\nvalueA : Int\nvalueA = \"a\"\n", p)
        self.assertIn("--- file: app/Pages/Home.alder ---\nmodule Pages.Home\n\nvalueB : Int\nvalueB = 2\n", p)
        prev = p.index("====================  YOUR PREVIOUS ANSWER  ====================")
        self.assertTrue(p.startswith("TASK TEXT"))
        self.assertLess(prev, p.index("COMPILER FEEDBACK"))
        self.assertLess(p.index("app/App.alder ---"), p.index("app/Pages/Home.alder ---"))   # sorted by path
        self.assertTrue(p.rstrip().endswith("No explanation, only files."))

    def test_check_exit2_recorded_not_raised(self):
        chk = FakeChecker(self.tmp, [(2, "", "alder check: internal error: boom\n"), (2, "", "alder check: internal error: boom\n")])
        s = rl.run_one(self.exp, rl.FakeModel([GOOD, GOOD]), "fake", "t", "task", "sys", chk.path, retries=1)
        self.assertFalse(s["final_pass"])
        self.assertEqual(s["rounds"][0]["exit"], 2)
        self.assertIn("boom", s["rounds"][0]["stderr"])

    def test_no_files_round(self):
        chk = FakeChecker(self.tmp, [(2, "", "no .alder files\n"), (0, "", "")])
        model = rl.FakeModel(["Sorry, I cannot.", GOOD])
        s = rl.run_one(self.exp, model, "fake", "t", "task", "sys", chk.path, retries=1)
        self.assertEqual(s["rounds"][0]["files"], 0)
        self.assertIn("no files", model.prompts[1].lower())
        self.assertTrue(s["final_pass"])

    def test_retry_prompt_contains_rendered_diag_and_fix(self):
        diags = [json.loads(ERR.strip())]
        diags[0]["fix"] = {"line": 3, "col": 1, "endLine": 3, "endCol": 2, "replace": "Text.fromInt (x)"}
        rd = self.exp / "runs/fake/t/round0"; (rd / "app").mkdir(parents=True)
        (rd / "app/App.alder").write_text("module App\n\nx = 1\n")
        p = rl.retry_prompt("TASK", rd, diags, "", 1)
        self.assertIn("app/App.alder:3:1: error[E0003]: expected Int, got Text", p)
        self.assertIn("fix: Text.fromInt (x)", p)
        self.assertIn("TASK", p)
        self.assertIn("every file", p)

    def test_model_unavailable(self):
        class Down(rl.Model):
            def generate(self, system, user):
                raise rl.ModelError("connection refused")
        chk = FakeChecker(self.tmp, [])
        s = rl.run_one(self.exp, Down(), "down", "t", "task", "sys", chk.path, retries=1)
        self.assertIsNotNone(s["error"])
        self.assertFalse(s["final_pass"])
        self.assertEqual(s["rounds"], [])

    def test_run_check_ignores_non_diagnostic_json(self):
        chk = FakeChecker(self.tmp, [(1, '{"ok":true}\n' + ERR, "")])
        code, diags, _, reason = rl.run_check(chk.path, pathlib.Path(self.tmp))
        self.assertIsNone(reason)
        self.assertEqual(code, 1)
        self.assertEqual(len(diags), 1)
        self.assertEqual(diags[0]["code"], "E0003")

    def test_run_check_missing_binary(self):
        missing = pathlib.Path(self.tmp) / "nope"
        code, diags, err, reason = rl.run_check(missing, pathlib.Path(self.tmp))
        self.assertEqual((code, diags), (2, []))
        self.assertIn(str(missing), err)
        self.assertIn(str(missing), reason)   # a binary that cannot start is a harness error

    def test_traversal_not_written_and_recorded(self):
        chk = FakeChecker(self.tmp, [(0, "", "")])
        model = rl.FakeModel([GOOD + "--- file: app/../../evil.alder ---\nmodule E\n"])
        s = rl.run_one(self.exp, model, "fake", "t", "task", "sys", chk.path, retries=0)
        self.assertEqual(s["rounds"][0]["rejected"], ["app/../../evil.alder"])
        self.assertFalse(list(pathlib.Path(self.tmp).rglob("evil.alder")))

class FakeAlder:
    """A fake `alder` honoring the real contract: `check DIR --json` prints diagnostics (exit 0/1/2);
    `build --emit DIR` writes build/<base of DIR>/ under the current directory and exits 0, 1 or 2."""
    def __init__(self, tmp, checks, builds):
        tmp = pathlib.Path(tmp)
        self.path = tmp / "alder"
        self.calls = tmp / "calls"
        self.calls.write_text("")
        lines = ["#!/bin/sh", f'echo "$PWD|$*" >> {self.calls}',
                 'if [ "$1" = check ]; then',
                 '  [ "$3" = --json ] || exit 2',
                 f'  n=$(grep -c "|check " {self.calls}); n=$((n-1))']
        for i, (exit_code, stdout, stderr) in enumerate(checks):
            lines.append(f'  if [ "$n" = "{i}" ]; then printf %b {json.dumps(stdout)}; printf %b {json.dumps(stderr)} 1>&2; exit {exit_code}; fi')
        lines += ["  exit 2", "fi",
                  'if [ "$1" = build ]; then',
                  '  [ "$2" = --emit ] || exit 2',
                  f'  n=$(grep -c "|build " {self.calls}); n=$((n-1))']
        for i, (exit_code, stderr) in enumerate(builds):
            lines.append(f'  if [ "$n" = "{i}" ]; then printf %b {json.dumps(stderr)} 1>&2; '
                         f'if [ {exit_code} = 0 ]; then mkdir -p "build/$(basename "$3")" && echo "package app" > "build/$(basename "$3")/app.go"; fi; exit {exit_code}; fi')
        lines += ["  exit 2", "fi", "exit 2"]
        self.path.write_text("\n".join(lines) + "\n")
        self.path.chmod(0o755)

    def log(self):
        return [l.split("|", 1) for l in self.calls.read_text().splitlines()]

class BuildTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.mkdtemp()
        self.exp = pathlib.Path(self.tmp) / "exp"
        (self.exp / "tasks").mkdir(parents=True)

    def tearDown(self):
        shutil.rmtree(self.tmp)

    def test_build_runs_after_clean_check(self):
        fa = FakeAlder(self.tmp, [(0, "", "")], [(0, "")])
        s = rl.run_one(self.exp, rl.FakeModel([GOOD]), "fake", "t", "task", "sys", fa.path, retries=1, build=True)
        rd = (self.exp / "runs/fake/t/round0").resolve()
        self.assertEqual(s["rounds"][0]["build"], {"ok": True, "exit": 0, "stderr": ""})
        self.assertTrue(s["final_pass"])
        self.assertFalse(s.get("compiler_bug"))
        self.assertEqual(s["harness"], {"build": True, "retries": 1})
        cwd, args = fa.log()[1]
        self.assertEqual(pathlib.Path(cwd).resolve(), rd)
        self.assertEqual(args, f"build --emit {rd}")
        self.assertTrue((rd / "build/round0/app.go").exists())

    def test_build_failure_recorded(self):
        fa = FakeAlder(self.tmp, [(0, "", ""), (0, "", "")], [(2, "alder build: internal error: generated Go does not vet\n")])
        model = rl.FakeModel([GOOD, GOOD])
        s = rl.run_one(self.exp, model, "fake", "t", "task", "sys", fa.path, retries=2, build=True)
        b = s["rounds"][0]["build"]
        self.assertFalse(b["ok"])
        self.assertEqual(b["exit"], 2)
        self.assertIn("does not vet", b["stderr"])
        self.assertFalse(s["final_pass"])
        self.assertTrue(s["compiler_bug"])
        self.assertEqual(len(model.prompts), 1)   # no retry: the program was fine, the compiler was not
        self.assertEqual(len(s["rounds"]), 1)
        self.assertIn("does not vet", (self.exp / "runs/fake/t/round0/build.stderr.txt").read_text())
        self.assertTrue(json.loads((self.exp / "runs/fake/t/summary.json").read_text())["compiler_bug"])

    def test_build_exit1_is_fail_with_retry(self):
        refusal = "alder build: the program has 1 typed hole(s); fill them before building\n"
        fa = FakeAlder(self.tmp, [(0, "", ""), (0, "", "")], [(1, refusal), (1, refusal)])
        model = rl.FakeModel([GOOD, GOOD])
        s = rl.run_one(self.exp, model, "fake", "t", "task", "sys", fa.path, retries=1, build=True)
        self.assertEqual(len(model.prompts), 2)
        self.assertEqual(s["rounds"][0]["build"]["exit"], 1)
        self.assertFalse(s["rounds"][0]["build"]["ok"])
        self.assertFalse(s["final_pass"])
        self.assertNotIn("compiler_bug", s)
        p = model.prompts[1]
        self.assertIn("The program type-checks but `alder build` refused it; fix the cause below.", p)
        self.assertIn("BUILD OUTPUT", p)
        self.assertIn("typed hole(s); fill them before building", p)
        self.assertLess(p.index("COMPILER FEEDBACK"), p.index("BUILD OUTPUT"))
        import report_loop as rp
        self.assertIn("| fake | t | 0 (build ✗) | 0 (build ✗) | fail |", rp.markdown(rp.rows([s])))

    def test_build_skipped_when_check_fails(self):
        fa = FakeAlder(self.tmp, [(1, ERR, "")], [])
        s = rl.run_one(self.exp, rl.FakeModel([GOOD]), "fake", "t", "task", "sys", fa.path, retries=0, build=True)
        self.assertNotIn("build", s["rounds"][0])
        self.assertFalse(s["final_pass"])
        self.assertEqual([a.split()[0] for _, a in fa.log()], ["check"])

    def test_no_build_without_flag(self):
        fa = FakeAlder(self.tmp, [(0, "", "")], [])
        s = rl.run_one(self.exp, rl.FakeModel([GOOD]), "fake", "t", "task", "sys", fa.path, retries=0)
        self.assertNotIn("build", s["rounds"][0])
        self.assertTrue(s["final_pass"])
        self.assertEqual(s["harness"], {"build": False, "retries": 0})

def stub_alder(tmp, body):
    """A stub `alder`: a shell script whose body sees the arguments as $1, $2, ..."""
    path = pathlib.Path(tmp) / "alder-stub"
    path.write_text("#!/bin/sh\n" + body + "\n")
    path.chmod(0o755)
    return path

def git(cwd, *args):
    subprocess.run(["git", "-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false", *args],
                   cwd=cwd, check=True, capture_output=True)

class HarnessErrorTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.mkdtemp()
        self.exp = pathlib.Path(self.tmp) / "exp"
        (self.exp / "tasks").mkdir(parents=True)

    def tearDown(self):
        shutil.rmtree(self.tmp)

    def test_build_timeout_is_harness_error(self):
        # check is clean; build sleeps past the (shortened) timeout
        alder = stub_alder(self.tmp, 'if [ "$1" = check ]; then exit 0; fi\nexec sleep 5')
        saved = rl.run_build
        rl.run_build = lambda a, rd: saved(a, rd, timeout=0.3)
        try:
            model = rl.FakeModel([GOOD, GOOD])
            s = rl.run_one(self.exp, model, "fake", "t", "task", "sys", alder, retries=1, build=True)
        finally:
            rl.run_build = saved
        self.assertEqual(s["outcome"], "harness_error")
        self.assertIn("timed out", s["harness_error"])
        self.assertIsNot(s.get("compiler_bug"), True)
        self.assertEqual(len(s["rounds"]), 1)
        self.assertEqual(len(model.prompts), 1)   # no retry
        self.assertFalse(s["final_pass"])
        self.assertEqual(json.loads((self.exp / "runs/fake/t/summary.json").read_text())["outcome"], "harness_error")

    def test_unknown_exit_is_harness_error(self):
        alder = stub_alder(self.tmp, "exit 137")
        model = rl.FakeModel([GOOD, GOOD])
        s = rl.run_one(self.exp, model, "fake", "t", "task", "sys", alder, retries=1)
        self.assertEqual(s["outcome"], "harness_error")
        self.assertIn("137", s["harness_error"])
        self.assertEqual(len(s["rounds"]), 1)
        self.assertEqual(s["rounds"][0]["exit"], 137)
        self.assertEqual(len(model.prompts), 1)   # no retry
        self.assertFalse(s["final_pass"])
        self.assertNotIn("compiler_bug", s)

    def test_outcome_on_every_path(self):
        ok = FakeChecker(self.tmp, [(0, "", "")])
        self.assertEqual(rl.run_one(self.exp, rl.FakeModel([GOOD]), "fake", "a", "task", "sys", ok.path, retries=0)["outcome"], "pass")
        bad = FakeChecker(self.tmp, [(1, ERR, "")])
        self.assertEqual(rl.run_one(self.exp, rl.FakeModel([GOOD]), "fake", "b", "task", "sys", bad.path, retries=0)["outcome"], "fail")
        fa = FakeAlder(self.tmp, [(0, "", "")], [(2, "alder build: internal error\n")])
        self.assertEqual(rl.run_one(self.exp, rl.FakeModel([GOOD]), "fake", "c", "task", "sys", fa.path, retries=1, build=True)["outcome"], "compiler_bug")

    @unittest.skipUnless(shutil.which("git"), "git is not on PATH")
    def test_base_exports_from_git_not_the_tree(self):
        repo = pathlib.Path(self.tmp) / "repo"
        (repo / "examples" / "x" / "app").mkdir(parents=True)
        a = repo / "examples" / "x" / "app" / "A.alder"
        a.write_text("module A\n-- committed\n")
        git(repo, "init", "-q")
        git(repo, "add", ".")
        git(repo, "commit", "-q", "-m", "base")
        a.write_text("module A\n-- uncommitted edit\n")
        (repo / "examples" / "x" / "app" / "Untracked.alder").write_text("module Untracked\n")
        chk = FakeChecker(self.tmp, [(0, "", "")])
        base = repo / "examples" / "x"
        s = rl.run_one(self.exp, rl.FakeModel([GOOD]), "fake", "t", "task", "sys", chk.path, retries=0, base=base)
        rd = self.exp / "runs/fake/t/round0/app"
        self.assertEqual((rd / "A.alder").read_text(), "module A\n-- committed\n")
        self.assertFalse((rd / "Untracked.alder").exists())
        self.assertEqual(s["base"], str(base))
        head = subprocess.run(["git", "rev-parse", "HEAD"], cwd=repo, capture_output=True, text=True).stdout.strip()
        self.assertEqual(s["base_commit"], head)

    def test_base_outside_git_is_an_error(self):
        d = pathlib.Path(self.tmp) / "notrepo"
        (d / "app").mkdir(parents=True)
        import contextlib, io
        err = io.StringIO()
        with contextlib.redirect_stderr(err), self.assertRaises(SystemExit) as cm:
            rl.main(["--exp", str(self.exp), "--model", "ollama:x", "--task", "t", "--alder", "/nonexistent", "--base", str(d)])
        self.assertEqual(cm.exception.code, 2)
        self.assertIn("`--base` must be inside a git repository (the harness seeds from `git archive HEAD`, never the working tree)", err.getvalue())

    def test_report_shows_harness_error(self):
        import report_loop as rp
        s = {"model": "m", "task": "t", "error": None, "final_pass": False, "outcome": "harness_error",
             "harness_error": "alder: build timed out after 900s", "harness": {"build": True, "retries": 1},
             "rounds": [{"n": 0, "errors": 0, "codes": {}, "files": 5, "exit": 0, "seconds": 1.0, "warnings": 0, "stderr": "",
                         "build": {"ok": False, "exit": None, "stderr": ""}}]}
        self.assertIn("| m | t | 0 (build ✗) | harness-error |", rp.markdown(rp.rows([s])))

class ModelTests(unittest.TestCase):
    def test_claude_isolated_cwd_and_system_prompt(self):
        calls = []
        def fake_run(argv, **kw):
            calls.append((argv, kw))
            out = "2.5.0 (Claude Code)\n" if "--version" in argv else "--- file: app/A.alder ---\nmodule A\n"
            return subprocess.CompletedProcess(argv, 0, stdout=out, stderr="")
        saved = (rl.subprocess.run, rl.shutil.which)
        rl.subprocess.run, rl.shutil.which = fake_run, (lambda name: "/usr/bin/claude")
        rl.ClaudeModel._version = None
        try:
            m = rl.ClaudeModel("sonnet")
            text, meta = m.generate("SYSTEM TEXT", "THE TASK")
            m.generate("SYSTEM TEXT", "THE TASK")
        finally:
            rl.subprocess.run, rl.shutil.which = saved
            rl.ClaudeModel._version = None
        self.assertIn("module A", text)
        self.assertEqual(meta["claude_version"], "2.5.0 (Claude Code)")
        self.assertEqual(meta["model"], "sonnet")
        self.assertIn("seconds", meta)
        self.assertEqual(sum(1 for a, _ in calls if "--version" in a), 1)   # cached per process
        gen = [(a, kw) for a, kw in calls if "-p" in a]
        self.assertEqual(len(gen), 2)
        argv, kw = gen[0]
        self.assertEqual(argv[argv.index("--system-prompt") + 1], "SYSTEM TEXT")
        self.assertIn("--no-session-persistence", argv)
        self.assertEqual(argv[argv.index("--setting-sources") + 1], "")   # no user/project/local settings, hooks, plugins
        self.assertIn("--strict-mcp-config", argv)                        # no MCP servers
        self.assertEqual(argv[argv.index("--tools") + 1], "")             # no tools
        self.assertIn("--disable-slash-commands", argv)                   # no skills
        self.assertNotIn("--bare", argv)                                  # it would require ANTHROPIC_API_KEY
        self.assertEqual(m.summary_fields()["claude_argv"],
                         [rl.ClaudeModel.SYSTEM_PLACEHOLDER if a == "SYSTEM TEXT" else a for a in argv])
        self.assertNotIn("THE TASK", " ".join(m.summary_fields()["claude_argv"]))
        self.assertEqual(kw["input"], "THE TASK")
        cwd = pathlib.Path(kw["cwd"])
        self.assertTrue(str(cwd.resolve()).startswith(str(pathlib.Path(tempfile.gettempdir()).resolve())))
        self.assertTrue(cwd.name.startswith("alder-claude-"))
        self.assertNotEqual(cwd.resolve(), pathlib.Path.cwd().resolve())

    def _ollama(self, resp, prompt):
        import io
        sent = []
        def fake_urlopen(req, timeout=None):
            sent.append(json.loads(req.data))
            return io.BytesIO(json.dumps(resp).encode())
        saved = rl.urllib.request.urlopen
        rl.urllib.request.urlopen = fake_urlopen
        try:
            return rl.OllamaModel("qwen").generate(prompt[:2000], prompt[2000:])
        finally:
            rl.urllib.request.urlopen = saved

    def test_ollama_prompt_truncation_warning(self):
        prompt = "x" * 4000   # ≈ 1000 tokens
        _, meta = self._ollama({"message": {"content": "hi"}, "prompt_eval_count": 10, "eval_count": 5}, prompt)
        self.assertEqual(meta["prompt_eval_count"], 10)
        self.assertEqual(meta["eval_count"], 5)
        self.assertIn("truncated", meta["warning"])
        self.assertIn("prompt_eval_count=10", meta["warning"])
        _, meta = self._ollama({"message": {"content": "hi"}, "eval_count": 5}, prompt)
        self.assertIn("warning", meta)                                 # absent count: warn too
        _, meta = self._ollama({"message": {"content": "hi"}, "prompt_eval_count": 1000, "eval_count": 5}, prompt)
        self.assertNotIn("warning", meta)

    def test_ollama_context_overflow_warning(self):
        prompt = "x" * 4000
        ctx = rl.OllamaModel.NUM_CTX
        _, meta = self._ollama({"message": {"content": "hi"}, "prompt_eval_count": 1000, "eval_count": ctx - 512 - 1000}, prompt)
        self.assertIn("context may have overflowed", meta["warning"])
        self.assertNotIn("truncated", meta["warning"])
        _, meta = self._ollama({"message": {"content": "hi"}, "prompt_eval_count": 1000, "eval_count": ctx - 513 - 1000}, prompt)
        self.assertNotIn("warning", meta)
        _, meta = self._ollama({"message": {"content": "hi"}, "prompt_eval_count": 10, "eval_count": ctx}, prompt)
        self.assertIn("truncated", meta["warning"])                    # both checks, one warning
        self.assertIn("overflowed", meta["warning"])

    def test_summary_records_claude_argv(self):
        tmp = tempfile.mkdtemp()
        try:
            chk = FakeChecker(tmp, [(0, "", "")])
            class Fake(rl.ClaudeModel):   # the real argv, a scripted answer
                def generate(self, system, user):
                    return GOOD, {"seconds": 0.0}
            s = rl.run_one(pathlib.Path(tmp) / "exp", Fake("haiku"), "claude-haiku", "t", "task", "SYSTEM", chk.path)
            saved = json.loads((pathlib.Path(tmp) / "exp/runs/claude-haiku/t/summary.json").read_text())
            self.assertEqual(saved["claude_argv"], s["claude_argv"])
            self.assertEqual(saved["claude_argv"][:4], ["claude", "-p", "--model", "haiku"])
            self.assertIn(rl.ClaudeModel.SYSTEM_PLACEHOLDER, saved["claude_argv"])
            self.assertNotIn("SYSTEM", [a for a in saved["claude_argv"]])
        finally:
            shutil.rmtree(tmp)

class ReportTests(unittest.TestCase):
    def test_report_table(self):
        import report_loop as rp
        rows = rp.rows([{"model": "m", "task": "t", "error": None, "final_pass": True,
                         "rounds": [{"n": 0, "errors": 3, "codes": {"E0003": 2, "E0101": 1}, "files": 5, "exit": 1, "seconds": 1.0, "warnings": 0, "stderr": ""},
                                    {"n": 1, "errors": 0, "codes": {}, "files": 5, "exit": 0, "seconds": 1.0, "warnings": 0, "stderr": ""}]}])
        self.assertEqual(rows[0]["r0"], 3)
        self.assertEqual(rows[0]["r1"], 0)
        md = rp.markdown(rows)
        self.assertIn("| m | t | 3 | 0 | pass |", md)

    def test_report_multi_round_and_build(self):
        import report_loop as rp
        def rd(n, errors, codes, build=None):
            r = {"n": n, "errors": errors, "codes": codes, "files": 5, "exit": 1 if errors else 0, "seconds": 1.0, "warnings": 0, "stderr": ""}
            if build is not None:
                r["build"] = {"ok": build, "exit": 0 if build else 2, "stderr": ""}
            return r
        ss = [{"model": "m", "task": "t", "error": None, "final_pass": True, "harness": {"build": True, "retries": 2},
               "rounds": [rd(0, 3, {"E0003": 2, "E0101": 1}), rd(1, 1, {"E0003": 1}), rd(2, 0, {}, build=True)]},
              {"model": "m", "task": "u", "error": None, "final_pass": False, "compiler_bug": True, "harness": {"build": True, "retries": 2},
               "rounds": [rd(0, 0, {}, build=False)]},
              {"model": "m", "task": "v", "error": None, "final_pass": False, "harness": {"build": True, "retries": 2},
               "rounds": [rd(0, 2, {"E0003": 2}), rd(1, 2, {"E0003": 2}), rd(2, 1, {"E0003": 1})]}]
        md = rp.markdown(rp.rows(ss))
        header = md.splitlines()[0]
        for col in ("r0", "r1", "r2", "Result"):
            self.assertIn(f"| {col} |", header)
        self.assertIn("| m | t | 3 | 1 | 0 (build ✓) | pass |", md)
        self.assertIn("| m | u | 0 (build ✗) | — | — | compiler-bug |", md)
        self.assertIn("| m | v | 2 | 2 | 1 | fail |", md)

    def test_report_e5_summaries_unchanged(self):
        import report_loop as rp
        here = pathlib.Path(__file__).resolve().parent / "e5-loop"
        ss = [json.loads(p.read_text()) for p in sorted(here.glob("runs/*/*/summary.json"))]
        if not ss:
            self.skipTest("no E5 runs")
        self.assertEqual(rp.markdown(rp.rows(ss)), (here / "scores.md").read_text())

if __name__ == "__main__":
    unittest.main()
