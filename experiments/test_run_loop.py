import json, os, pathlib, posixpath, shutil, subprocess, sys, tempfile, unittest
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

    def test_check_internal_error_is_compiler_bug(self):
        # a checker panic: exit 2 with "internal error" — the compiler's fault, no retry
        chk = FakeChecker(self.tmp, [(2, "", "alder check: internal error: boom\n"), (0, "", "")])
        model = rl.FakeModel([GOOD, GOOD])
        s = rl.run_one(self.exp, model, "fake", "t", "task", "sys", chk.path, retries=1)
        self.assertEqual(s["outcome"], "compiler_bug")
        self.assertEqual(s["compiler_bug_kind"], "internal_error")
        self.assertTrue(s["compiler_bug"])
        self.assertFalse(s["final_pass"])
        self.assertEqual(s["rounds"][0]["exit"], 2)
        self.assertIn("boom", s["rounds"][0]["stderr"])
        self.assertEqual(len(model.prompts), 1)   # no retry
        self.assertIn("internal error: boom", (self.exp / "runs/fake/t/round0/check.stderr.txt").read_text())

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

    def test_build_timeout_is_compiler_bug_hang(self):
        # check is clean; build sleeps past the (shortened) timeout: a hang is the compiler's
        alder = stub_alder(self.tmp, 'if [ "$1" = check ]; then exit 0; fi\nexec sleep 5')
        saved = rl.run_build
        rl.run_build = lambda a, rd: saved(a, rd, timeout=0.3)
        try:
            model = rl.FakeModel([GOOD, GOOD])
            s = rl.run_one(self.exp, model, "fake", "t", "task", "sys", alder, retries=1, build=True)
        finally:
            rl.run_build = saved
        self.assertEqual(s["outcome"], "compiler_bug")
        self.assertEqual(s["compiler_bug_kind"], "hang")
        self.assertNotIn("harness_error", s)
        self.assertIn("timed out", s["rounds"][0]["build"]["stderr"])
        self.assertEqual(len(s["rounds"]), 1)
        self.assertEqual(len(model.prompts), 1)   # no retry
        self.assertFalse(s["final_pass"])
        self.assertEqual(json.loads((self.exp / "runs/fake/t/summary.json").read_text())["outcome"], "compiler_bug")

    def test_check_timeout_is_compiler_bug_hang(self):
        alder = stub_alder(self.tmp, "exec sleep 5")
        saved = rl.run_check
        rl.run_check = lambda a, rd: saved(a, rd, timeout=0.3)
        try:
            model = rl.FakeModel([GOOD, GOOD])
            s = rl.run_one(self.exp, model, "fake", "t", "task", "sys", alder, retries=1)
        finally:
            rl.run_check = saved
        self.assertEqual((s["outcome"], s["compiler_bug_kind"]), ("compiler_bug", "hang"))
        self.assertEqual(len(model.prompts), 1)   # no retry
        self.assertIn("timed out", (self.exp / "runs/fake/t/round0/check.stderr.txt").read_text())

    def test_timeout_kills_the_process_group(self):
        # the stub leaves a child holding stdout open: without a group kill the harness would wait
        # for it (30 s)
        alder = stub_alder(self.tmp, "sleep 30 &\nexec sleep 30")
        t0 = __import__("time").time()
        code, err, out, reason = rl.run_build(alder, pathlib.Path(self.tmp), timeout=0.5)
        self.assertLess(__import__("time").time() - t0, 10)
        self.assertIsNone(code)
        self.assertIsInstance(reason, rl.Hang)

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

    def test_base_missing_is_an_error(self):
        import contextlib, io
        err = io.StringIO()
        missing = pathlib.Path(self.tmp) / "nope"
        with contextlib.redirect_stderr(err), self.assertRaises(SystemExit) as cm:
            rl.main(["--exp", str(self.exp), "--model", "ollama:x", "--task", "t", "--alder", "/nonexistent", "--base", str(missing)])
        self.assertEqual(cm.exception.code, 2)
        self.assertIn(f"--base {missing} does not exist", err.getvalue())

    @unittest.skipUnless(shutil.which("git"), "git is not on PATH")
    def test_base_uncommitted_changes_warn(self):
        import contextlib, io
        repo = pathlib.Path(self.tmp) / "repo"
        (repo / "app").mkdir(parents=True)
        (repo / "app" / "A.alder").write_text("module A\n")
        git(repo, "init", "-q")
        git(repo, "add", ".")
        git(repo, "commit", "-q", "-m", "base")
        err = io.StringIO()
        with contextlib.redirect_stderr(err):
            exported, commit = rl.export_base(repo)
        rl.remove_export(exported)
        self.assertEqual(err.getvalue(), "")
        (repo / "app" / "A.alder").write_text("module A\n-- edit\n")
        with contextlib.redirect_stderr(err):
            exported, commit2 = rl.export_base(repo)
        rl.remove_export(exported)
        self.assertEqual(commit, commit2)
        self.assertIn(f"warning: uncommitted changes under {repo}/app are ignored", err.getvalue())

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

def tests_stub(tmp, rounds):
    """A stub `alder` whose check and build always succeed and whose `test` answers per call:
    rounds[i] = (exit, json_stdout, stderr, text_report) for the i-th `test --json`; a `test`
    without --json prints the text report of the latest --json call (the harness's second run)."""
    tmp = pathlib.Path(tmp)
    count, calls = tmp / "test-count", tmp / "test-calls"
    count.write_text("0")
    calls.write_text("")
    lines = ["#!/bin/sh", f'echo "$PWD|$*" >> {calls}',
             'if [ "$1" = check ]; then exit 0; fi',
             'if [ "$1" = build ]; then exit 0; fi',
             '[ "$1" = test ] || exit 2',
             'case " $* " in *" --json "*) json=1;; *) json=0;; esac',
             f'n=$(cat {count})',
             f'if [ $json = 1 ]; then echo $((n+1)) > {count}; else n=$((n-1)); fi']
    q = lambda t: json.dumps(t).replace("`", "\\`")   # a double-quoted sh word; a backtick stays literal
    for i, (exit_code, out, err, text) in enumerate(rounds):
        lines.append(f'if [ "$n" = "{i}" ]; then if [ $json = 1 ]; then printf %b {q(out)}; printf %b {q(err)} 1>&2; '
                     f'else printf %b {q(text)}; fi; exit {exit_code}; fi')
    lines.append("exit 2")
    path = tmp / "alder-test-stub"
    path.write_text("\n".join(lines) + "\n")
    path.chmod(0o755)
    stub = StubPath(path)
    stub.calls = calls
    return stub

class StubPath(str):
    """A stub's path (run_one takes a str or a Path) carrying the file that logs its calls."""

def tline(module, name, ok, **kw):
    return json.dumps({"module": module, "name": name, "ok": ok, **kw, "trace": [], "ms": 0.1}) + "\n"

PASS2 = tline("Todo", "a", True, left="1", right="1") + tline("Todo", "b", True)
FAIL1 = tline("Todo", "a", True, left="1", right="1") + tline("Todo", "b", False, left="2", right="3")
FAIL1_TEXT = 'ok   Todo "a"\nFAIL Todo "b"\n     left:  2\n     right: 3\n1 passed, 1 failed\n'

class TestLoopTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.mkdtemp()
        self.exp = pathlib.Path(self.tmp) / "exp"
        (self.exp / "tasks").mkdir(parents=True)

    def tearDown(self):
        shutil.rmtree(self.tmp)

    def test_test_requires_build(self):
        import contextlib, io
        (self.exp / "prompt.md").write_text("sys")
        (self.exp / "tasks" / "t.md").write_text("task")
        err = io.StringIO()
        with contextlib.redirect_stderr(err), self.assertRaises(SystemExit) as cm:
            rl.main(["--exp", str(self.exp), "--model", "ollama:x", "--task", "t", "--alder", "/nonexistent", "--test"])
        self.assertEqual(cm.exception.code, 2)
        self.assertIn("--test requires --build", err.getvalue())
        with self.assertRaises(ValueError):
            rl.run_one(self.exp, rl.FakeModel([GOOD]), "fake", "t", "task", "sys", pathlib.Path("/nonexistent"), test=True)

    def test_test_pass_records_counts(self):
        alder = tests_stub(self.tmp, [(0, PASS2, "2 passed, 0 failed\n", "")])
        model = rl.FakeModel([GOOD])
        s = rl.run_one(self.exp, model, "fake", "t", "task", "sys", alder, retries=1, build=True, test=True)
        self.assertEqual(s["outcome"], "pass")
        self.assertTrue(s["final_pass"])
        self.assertEqual(s["harness"], {"build": True, "retries": 1, "test": True})
        t = s["rounds"][0]["tests"]
        self.assertEqual((t["passed"], t["failed"]), (2, 0))
        self.assertEqual([r["name"] for r in t["results"]], ["a", "b"])
        self.assertEqual(t["results"][0], {"module": "Todo", "name": "a", "ok": True, "left": "1", "right": "1", "trace": [], "ms": 0.1})
        self.assertNotIn("test_output", s["rounds"][0])
        rd = (self.exp / "runs/fake/t/round0").resolve()
        cwd, args = alder.calls.read_text().splitlines()[-1].split("|", 1)
        self.assertEqual(args, f"test --json {rd}")
        self.assertEqual(pathlib.Path(cwd).resolve(), rd)
        self.assertEqual(len(alder.calls.read_text().splitlines()), 3)   # check, build, test: no text run on a pass

    def test_test_failure_is_retry(self):
        alder = tests_stub(self.tmp, [(1, FAIL1, "1 passed, 1 failed\n", FAIL1_TEXT), (0, PASS2, "2 passed, 0 failed\n", "")])
        model = rl.FakeModel([GOOD, GOOD])
        s = rl.run_one(self.exp, model, "fake", "t", "TASK TEXT", "sys", alder, retries=2, build=True, test=True)
        self.assertEqual(len(s["rounds"]), 2)
        r0 = s["rounds"][0]
        self.assertTrue(r0["build"]["ok"])
        self.assertEqual((r0["errors"], r0["tests"]["passed"], r0["tests"]["failed"]), (0, 1, 1))
        self.assertEqual(r0["test_output"], FAIL1_TEXT)
        self.assertEqual(s["outcome"], "pass")
        self.assertNotIn("compiler_bug", s)
        p = model.prompts[1]
        self.assertTrue(p.startswith("TASK TEXT"))
        self.assertIn("YOUR PREVIOUS ANSWER", p)
        self.assertIn("--- file: app/App.alder ---", p)
        self.assertIn("The program builds but its tests fail; fix the program or the tests so every test passes.", p)
        self.assertIn("====================  TEST OUTPUT  ====================\n" + FAIL1_TEXT.strip(), p)
        self.assertLess(p.index("COMPILER FEEDBACK"), p.index("TEST OUTPUT"))
        self.assertTrue(p.rstrip().endswith("No explanation, only files."))
        self.assertEqual(s["rounds"][1]["tests"]["failed"], 0)

    def test_test_output_truncated(self):
        long = "FAIL x\n" * 2000
        alder = tests_stub(self.tmp, [(1, FAIL1, "", long)])
        s = rl.run_one(self.exp, rl.FakeModel([GOOD]), "fake", "t", "task", "sys", alder, retries=0, build=True, test=True)
        self.assertEqual(len(s["rounds"][0]["test_output"]), 4000)
        self.assertEqual(s["outcome"], "fail")

    def test_test_exit2_is_compiler_bug(self):
        alder = tests_stub(self.tmp, [(2, '{"error": "the test program stopped: boom"}\n', "alder test: the test program stopped: boom\n", "")])
        model = rl.FakeModel([GOOD, GOOD])
        s = rl.run_one(self.exp, model, "fake", "t", "task", "sys", alder, retries=2, build=True, test=True)
        self.assertEqual((s["outcome"], s["compiler_bug_kind"]), ("compiler_bug", "test"))
        self.assertTrue(s["compiler_bug"])
        self.assertEqual(len(model.prompts), 1)   # no retry
        self.assertEqual(s["rounds"][0]["tests"]["exit"], 2)
        self.assertIn("boom", (self.exp / "runs/fake/t/round0/test.stderr.txt").read_text())
        import report_loop as rp
        self.assertIn("| compiler-bug |", rp.markdown(rp.rows([s])))

    def test_test_exit2_adapter_refusal_is_a_failed_round(self):
        # alder test refuses the app's adapters (exit 2) before building: the model's program, so an
        # ordinary failed round whose retry carries the message (real text, M5d)
        err = ("alder test: blog is missing adapters for 1 capability:\n"
               "capability `Markdown.Markdown` needs `adapters/markdown/` with `func Markdown() app.Cap__Markdown__Markdown`\n")
        alder = tests_stub(self.tmp, [(2, '{"error": "x"}\n', err, ""), (0, PASS2, "", "")])
        model = rl.FakeModel([GOOD, GOOD])
        s = rl.run_one(self.exp, model, "fake", "t", "task", "sys", alder, retries=1, build=True, test=True)
        self.assertEqual(s["outcome"], "pass")
        self.assertNotIn("compiler_bug", s)
        self.assertEqual(s["rounds"][0]["tests"]["exit"], 2)
        self.assertIn("needs `adapters/markdown/` with `func Markdown()", s["rounds"][0]["test_output"])
        self.assertIn("====================  TEST OUTPUT  ====================\nalder test: blog is missing adapters", model.prompts[1])
        for text in ("module `Internal` cannot name an adapter package: Go keeps `adapters/internal/` private to adapters/ (it holds helpers); rename the module",
                     "adapters/ no longer wires capabilities: move each adapter to adapters/<module>/ (see the list below)",
                     "internal naming error: capabilities Mail.OutBox and Mail.Out.Box both need func OutBox in adapters/mail/ (rename one of them)"):
            self.assertTrue(rl.adapter_refusal("alder test: " + text + "\n"), text)
        self.assertFalse(rl.adapter_refusal("alder test: the test program stopped: boom\n"))

    def test_test_exit2_timed_out_is_hang(self):
        alder = tests_stub(self.tmp, [(2, "", "alder test: the tests timed out after 5m0s\n", "")])
        s = rl.run_one(self.exp, rl.FakeModel([GOOD, GOOD]), "fake", "t", "task", "sys", alder, retries=1, build=True, test=True)
        self.assertEqual((s["outcome"], s["compiler_bug_kind"]), ("compiler_bug", "hang"))

    def test_test_hang_is_compiler_bug(self):
        alder = stub_alder(self.tmp, 'if [ "$1" = check ] || [ "$1" = build ]; then exit 0; fi\nsleep 30 &\nexec sleep 30')
        saved = rl.run_test
        rl.run_test = lambda a, rd: saved(a, rd, timeout=0.3)
        try:
            model = rl.FakeModel([GOOD, GOOD])
            t0 = __import__("time").time()
            s = rl.run_one(self.exp, model, "fake", "t", "task", "sys", alder, retries=1, build=True, test=True)
        finally:
            rl.run_test = saved
        self.assertLess(__import__("time").time() - t0, 10)   # the process group was killed
        self.assertEqual((s["outcome"], s["compiler_bug_kind"]), ("compiler_bug", "hang"))
        self.assertEqual(len(model.prompts), 1)
        self.assertIn("timed out", (self.exp / "runs/fake/t/round0/test.stderr.txt").read_text())

    def test_test_unknown_exit_is_harness_error(self):
        alder = tests_stub(self.tmp, [(137, "", "", "")])
        s = rl.run_one(self.exp, rl.FakeModel([GOOD]), "fake", "t", "task", "sys", alder, retries=1, build=True, test=True)
        self.assertEqual(s["outcome"], "harness_error")
        self.assertIn("alder test exited 137", s["harness_error"])

    def test_no_tests_passes(self):
        alder = tests_stub(self.tmp, [(0, "", "0 passed, 0 failed\n", "")])
        s = rl.run_one(self.exp, rl.FakeModel([GOOD]), "fake", "t", "task", "sys", alder, retries=1, build=True, test=True)
        self.assertEqual(s["outcome"], "pass")
        t = s["rounds"][0]["tests"]
        self.assertEqual((t["passed"], t["failed"], t["results"]), (0, 0, []))

    def test_no_test_without_flag(self):
        alder = tests_stub(self.tmp, [])
        s = rl.run_one(self.exp, rl.FakeModel([GOOD]), "fake", "t", "task", "sys", alder, retries=0, build=True)
        self.assertNotIn("tests", s["rounds"][0])
        self.assertNotIn("test", s["harness"])
        self.assertEqual([l.split("|", 1)[1].split()[0] for l in alder.calls.read_text().splitlines()], ["check", "build"])

    @unittest.skipUnless(shutil.which("git"), "git is not on PATH")
    def test_base_exports_adapters(self):
        repo = pathlib.Path(self.tmp) / "repo"
        (repo / "ex" / "app").mkdir(parents=True)
        (repo / "ex" / "adapters" / "x").mkdir(parents=True)
        (repo / "ex" / "app" / "App.alder").write_text("module App\n")
        (repo / "ex" / "adapters" / "x" / "x.go").write_text("package x\n")
        (repo / "other" / "adapters").mkdir(parents=True)
        (repo / "other" / "adapters" / "y.go").write_text("package adapters\n")
        git(repo, "init", "-q")
        git(repo, "add", ".")
        git(repo, "commit", "-q", "-m", "base")
        alder = tests_stub(self.tmp, [(1, FAIL1, "", FAIL1_TEXT), (0, PASS2, "", "")])
        s = rl.run_one(self.exp, rl.FakeModel([GOOD, GOOD]), "fake", "t", "task", "sys", alder, retries=1,
                       base=repo / "ex", build=True, test=True)
        self.assertEqual(s["outcome"], "pass")
        for n in (0, 1):   # every round carries the adapters (the model writes only app/)
            rd = self.exp / f"runs/fake/t/round{n}"
            self.assertEqual((rd / "adapters" / "x" / "x.go").read_text(), "package x\n")
            self.assertFalse((rd / "adapters" / "y.go").exists())
        # a base without adapters/ still exports
        (repo / "noad" / "app").mkdir(parents=True)
        (repo / "noad" / "app" / "A.alder").write_text("module A\n")
        git(repo, "add", ".")
        git(repo, "commit", "-q", "-m", "noad")
        exported, _ = rl.export_base(repo / "noad")
        try:
            self.assertTrue((exported / "app" / "A.alder").exists())
            self.assertFalse((exported / "adapters").exists())
        finally:
            rl.remove_export(exported)

    def test_num_ctx_flag_sets_warning_threshold(self):
        import io
        sent = []
        def fake_urlopen(req, timeout=None):
            sent.append(json.loads(req.data))
            return io.BytesIO(json.dumps(resp).encode())
        saved = rl.urllib.request.urlopen
        rl.urllib.request.urlopen = fake_urlopen
        prompt = "x" * 4000
        try:
            m = rl.make_model("ollama:qwen", num_ctx=65536)
            self.assertEqual(m.NUM_CTX, 65536)
            self.assertEqual(rl.OllamaModel.NUM_CTX, 32768)   # the class default is untouched
            resp = {"message": {"content": "hi"}, "prompt_eval_count": 1000, "eval_count": 40000}   # over 32768, under 65536
            _, meta = m.generate(prompt[:2000], prompt[2000:])
            self.assertNotIn("warning", meta)
            self.assertEqual(sent[-1]["options"]["num_ctx"], 65536)
            resp = {"message": {"content": "hi"}, "prompt_eval_count": 1000, "eval_count": 65536 - 512 - 1000}
            _, meta = m.generate(prompt[:2000], prompt[2000:])
            self.assertIn("context may have overflowed", meta["warning"])
            self.assertIn(">= num_ctx 65536 - 512", meta["warning"])
            resp = {"message": {"content": "hi"}, "prompt_eval_count": 1000, "eval_count": 40000}
            _, meta = rl.make_model("ollama:qwen").generate(prompt[:2000], prompt[2000:])
            self.assertIn("overflowed", meta["warning"])                 # the default stays 32768
        finally:
            rl.urllib.request.urlopen = saved
        # --num-ctx reaches the model and summary.json (harness.num_ctx)
        (self.exp / "prompt.md").write_text("sys")
        (self.exp / "tasks" / "t.md").write_text("task")
        seen = []
        saved_run = rl.run_one
        def fake_run_one(exp, model, label, task, text, system, alder, retries, **kw):
            seen.append(model)
            return {"error": None, "outcome": "pass", "rounds": []}
        rl.run_one = fake_run_one
        try:
            import contextlib
            with contextlib.redirect_stdout(io.StringIO()):
                rl.main(["--exp", str(self.exp), "--model", "ollama:x", "--task", "t", "--alder", "/nonexistent", "--num-ctx", "65536"])
        finally:
            rl.run_one = saved_run
        self.assertEqual(seen[0].NUM_CTX, 65536)
        class FakeOllama(rl.OllamaModel):
            def generate(self, system, user):
                return GOOD, {"seconds": 0.0}
        chk = FakeChecker(self.tmp, [(0, "", "")])
        s = rl.run_one(self.exp, FakeOllama("q", num_ctx=65536), "q", "t", "task", "sys", chk.path, retries=0)
        self.assertEqual(s["harness"]["num_ctx"], 65536)

    def test_ollama_length_is_truncated_failed_round(self):
        import io
        sent = []
        resp = {}
        def fake_urlopen(req, timeout=None):
            sent.append(json.loads(req.data))
            return io.BytesIO(json.dumps(resp).encode())
        saved = rl.urllib.request.urlopen
        rl.urllib.request.urlopen = fake_urlopen
        try:
            m = rl.OllamaModel("q")
            resp.update({"message": {"content": GOOD + "--- file: go.mod ---\nmodule x\n"}, "prompt_eval_count": 1000,
                         "eval_count": 8192, "done_reason": "length"})
            _, meta = m.generate("x" * 2000, "x" * 2000)
            self.assertEqual(sent[-1]["options"]["num_predict"], 8192)
            self.assertTrue(meta["truncated"])
            self.assertEqual(meta["done_reason"], "length")
            resp.update({"done_reason": "stop", "eval_count": 100})
            _, meta = m.generate("x" * 2000, "x" * 2000)
            self.assertNotIn("truncated", meta)
            self.assertEqual(meta["done_reason"], "stop")
        finally:
            rl.urllib.request.urlopen = saved

        class CutOff(rl.OllamaModel):   # round 0 cut off (a clean program, but truncated), round 1 complete
            def __init__(self):
                super().__init__("q")
                self.prompts = []
            def generate(self, system, user):
                self.prompts.append(user)
                if len(self.prompts) == 1:
                    return GOOD, {"seconds": 0.0, "eval_count": 8192, "done_reason": "length", "truncated": True}
                return GOOD, {"seconds": 0.0, "eval_count": 100, "done_reason": "stop"}
        chk = FakeChecker(self.tmp, [(0, "", ""), (0, "", "")])
        m = CutOff()
        s = rl.run_one(self.exp, m, "q", "t", "task", "sys", chk.path, retries=1)
        self.assertEqual(s["outcome"], "pass")
        self.assertTrue(s["rounds"][0]["truncated"])
        self.assertEqual(s["rounds"][0]["done_reason"], "length")
        self.assertEqual(s["rounds"][0]["errors"], 0)
        self.assertNotIn("truncated", s["rounds"][1])
        self.assertIn("Your previous answer was cut off after 8192 tokens; output only the Alder files "
                      "(`--- file: app/<path> ---`), nothing else.", m.prompts[1])
        self.assertEqual(s["harness"]["num_predict"], 8192)
        # with errors too, the sentence is appended to the compiler's feedback
        chk = FakeChecker(self.tmp, [(1, ERR, "")])
        m = CutOff()
        s = rl.run_one(self.exp, m, "q", "t", "task", "sys", chk.path, retries=0)
        self.assertEqual(s["outcome"], "fail")
        p = rl.retry_prompt("task", self.exp / "runs/q/t/round0", [json.loads(ERR)], "", 1, truncated=8192)
        self.assertIn("expected Int, got Text", p)
        self.assertIn("cut off after 8192 tokens", p)

    def test_report_tests_column(self):
        import report_loop as rp
        def rd(n, tests=None, build=True):
            r = {"n": n, "errors": 0, "codes": {}, "files": 5, "exit": 0, "seconds": 1.0, "warnings": 0, "stderr": "",
                 "build": {"ok": build, "exit": 0, "stderr": ""}}
            if tests is not None:
                r["tests"] = {"passed": tests[0], "failed": tests[1], "results": []}
            return r
        h = {"build": True, "retries": 2, "test": True}
        ss = [{"model": "m", "task": "t", "error": None, "final_pass": True, "outcome": "pass", "harness": h,
               "rounds": [rd(0, (1, 2)), rd(1, (3, 0))]},
              {"model": "m", "task": "u", "error": None, "final_pass": True, "outcome": "pass", "harness": h,
               "rounds": [rd(0, (0, 0))]},
              {"model": "m", "task": "v", "error": None, "final_pass": False, "outcome": "compiler_bug", "compiler_bug": True,
               "compiler_bug_kind": "test", "harness": h, "rounds": [rd(0, (0, 0))]}]
        md = rp.markdown(rp.rows(ss))
        self.assertIn("| Model | Task | r0 | r1 | Tests | Result | Codes |", md)
        self.assertIn("| m | t | 0 (build ✓) | 0 (build ✓) | r0 ✗ 2/3; r1 ✓ 3 | pass |", md)
        self.assertIn("| m | u | 0 (build ✓) | — | r0 ✓ 0 | pass |", md)
        self.assertIn("| m | v | 0 (build ✓) | — | r0 ✓ 0 | compiler-bug |", md)
        # without --test runs the table is unchanged (no Tests column)
        md2 = rp.markdown(rp.rows([{**ss[0], "harness": {"build": True, "retries": 2},
                                    "rounds": [rd(0), rd(1)]}]))
        self.assertNotIn("Tests", md2)

    def test_report_e6_summaries_unchanged(self):
        import report_loop as rp
        here = pathlib.Path(__file__).resolve().parent / "e6-build"
        ss = [json.loads(p.read_text()) for p in sorted(here.glob("runs/*/*/summary.json"))]
        if not ss:
            self.skipTest("no E6 runs")
        self.assertEqual(rp.markdown(rp.rows(ss)), (here / "scores.md").read_text())

class E7FalloutTests(unittest.TestCase):
    """After E7: stdlib paths, cut-off files, opt-in adapters, the Go-refusal hint."""
    def setUp(self):
        self.tmp = tempfile.mkdtemp()
        self.exp = pathlib.Path(self.tmp) / "exp"
        (self.exp / "tasks").mkdir(parents=True)

    def tearDown(self):
        shutil.rmtree(self.tmp)

    def run1(self, answers, alder, retries=1, **kw):
        model = rl.FakeModel(answers)
        return rl.run_one(self.exp, model, "fake", "t", "TASK", "sys", alder, retries=retries, **kw), model

    def test_stdlib_paths_rejected(self):
        repo_stdlib = pathlib.Path(rl.__file__).resolve().parent.parent / "stdlib"
        names = {f.stem for f in repo_stdlib.glob("*.alder")}
        self.assertTrue({"List", "Db", "Http"} <= names)
        self.assertTrue(names <= rl.STDLIB_NAMES and "stdlib" in rl.STDLIB_NAMES)
        # only app/stdlib/… and a module named exactly like a stdlib module are refused: a module
        # under a stdlib name's directory (`module Time.Slots`) is legal, and the compiler takes it
        text = ("--- file: app/stdlib/List.alder ---\nmodule List\n--- file: app/List.alder ---\nmodule List\n"
                "--- file: Db.alder ---\nmodule Db\n--- file: app/stdlib.alder ---\nmodule X\n"
                "--- file: Db/Query.alder ---\nmodule Db.Query\n--- file: app/Auth/List.alder ---\nmodule Auth.List\n"
                "--- file: app/Time/Slots.alder ---\nmodule Time.Slots\n--- file: app/Text/Editor.alder ---\nmodule Text.Editor\n"
                "--- file: app/Lists.alder ---\nmodule Lists\n")
        files, rejected = rl.split_files(text)
        self.assertEqual(sorted(files), ["app/Auth/List.alder", "app/Db/Query.alder", "app/Lists.alder", "app/Text/Editor.alder",
                                         "app/Time/Slots.alder"])
        self.assertEqual(rejected, ["app/stdlib/List.alder (standard-library module)", "app/List.alder (standard-library module)",
                                    "Db.alder (standard-library module)", "app/stdlib.alder (standard-library module)"])
        self.assertEqual(rl.normalize_path("app/Time/Slots.alder"), "app/Time/Slots.alder")
        # and through the loop: nothing written, the retry names the paths
        chk = FakeChecker(self.tmp, [(1, ERR, ""), (0, "", "")])
        s, model = self.run1([GOOD + "--- file: app/stdlib/List.alder ---\nmodule List\n", GOOD], chk.path)
        self.assertFalse((self.exp / "runs/fake/t/round0/app/stdlib").exists())
        self.assertEqual(s["rounds"][0]["rejected"], ["app/stdlib/List.alder (standard-library module)"])
        self.assertIn("These paths were rejected: app/stdlib/List.alder (standard-library module)", model.prompts[1])

    def test_truncated_last_file_not_written(self):
        class Cut(rl.FakeModel):
            def generate(self, system, user):
                text, meta = super().generate(system, user)
                if len(self.prompts) == 2:   # round 1: stopped by the token cap, mid-file
                    meta.update({"done_reason": "length", "truncated": True})
                return text, meta
        chk = FakeChecker(self.tmp, [(1, ERR, ""), (1, ERR, ""), (0, "", "")])
        model = Cut([GOOD + "--- file: app/X.alder ---\nmodule X\n\nv : Int\nv = 1\n",
                     "--- file: app/Y.alder ---\nmodule Y\n--- file: app/X.alder ---\nmodule X\n\nv : Int\nv = 2\n\nw : In",
                     GOOD])
        s = rl.run_one(self.exp, model, "fake", "t", "TASK", "sys", chk.path, retries=2)
        r1 = self.exp / "runs/fake/t/round1/app"
        self.assertTrue((r1 / "Y.alder").exists())                       # the complete file before it is written
        self.assertEqual((r1 / "X.alder").read_text(), "module X\n\nv : Int\nv = 1\n")   # round 0's version stays
        self.assertEqual(s["rounds"][1]["cut_off_file"], "app/X.alder")
        self.assertNotIn("cut_off_file", s["rounds"][0])
        self.assertIn("`app/X.alder` was cut off and not written; emit it again in full", model.prompts[2])
        # a cut-off first-time file is absent
        chk = FakeChecker(self.tmp, [(0, "", "")])
        files, rejected, cut = rl.split_answer("--- file: app/A.alder ---\nmodule A\n--- file: app/B.alder ---\nmod", cut_off=True)
        self.assertEqual((sorted(files), cut), (["app/A.alder"], "app/B.alder"))
        self.assertEqual(rl.split_answer("--- file: app/A.alder ---\nmod", cut_off=False)[2], None)

    def test_classify_path_adapters(self):
        # one package level under adapters/ (chapter 7's rule): adapters/<dir>/<name>.go, with --adapters only
        for path in ("adapters/auth/password.go", "adapters/markdown/markdown.go", "adapters/app/clock.go",
                     "adapters/auth/password_test.go", "./adapters/auth/x.go", "adapters/auth/../auth/x.go"):
            self.assertEqual(rl.classify_path(path, adapters=True), (posixpath.normpath(path), None), path)
            self.assertEqual(rl.classify_path(path), (None, ""), path)   # opt-in
        for path in ("adapters/password.go",             # the removed flat form
                     "adapters/internal/dbx/dbx.go",     # deeper than one level
                     "adapters/auth/password.txt", "adapters/auth.go/x.alder.go/y.go",
                     "adapters/../evil.go", "adapters/auth/../../evil.go", "/adapters/auth/x.go",
                     "adapters\\auth\\x.go", "adapters/a-b/x.go", "go.mod", "x.go"):
            self.assertEqual(rl.classify_path(path, adapters=True), (None, ""), path)
        self.assertEqual(rl.normalize_path("adapters/auth/password.go", adapters=True), "adapters/auth/password.go")

    def test_adapters_flag_accepts_go(self):
        text = (GOOD + "--- file: adapters/auth/password.go ---\n```go\npackage auth\n```\n"
                "--- file: adapters/flat.go ---\npackage adapters\n"
                "--- file: adapters/auth/sub/x.go ---\npackage sub\n--- file: adapters/../evil.go ---\npackage x\n"
                "--- file: adapters/auth/../../evil.go ---\npackage x\n--- file: go.mod ---\nmodule x\n")
        files, rejected = rl.split_files(text, adapters=True)
        self.assertEqual(sorted(files), ["adapters/auth/password.go", "app/App.alder"])
        self.assertEqual(files["adapters/auth/password.go"], "package auth\n")
        self.assertEqual(rejected, ["adapters/flat.go", "adapters/auth/sub/x.go", "adapters/../evil.go",
                                    "adapters/auth/../../evil.go", "go.mod"])
        self.assertEqual(rl.split_files(text)[0].keys(), {"app/App.alder"})   # opt-in: off by default
        chk = FakeChecker(self.tmp, [(0, "", "")])
        s, _ = self.run1([text], chk.path, retries=0, adapters=True)
        rd = self.exp / "runs/fake/t/round0"
        self.assertEqual((rd / "adapters/auth/password.go").read_text(), "package auth\n")
        self.assertFalse((rd / "adapters/auth/sub").exists() or (rd / "adapters/flat.go").exists()
                         or (self.exp / "runs/fake/t/evil.go").exists())
        self.assertTrue(s["harness"]["adapters"])
        chk = FakeChecker(self.tmp, [(0, "", "")])
        s, _ = self.run1([text], chk.path, retries=0)
        self.assertNotIn("adapters", s["harness"])
        self.assertFalse((self.exp / "runs/fake/t/round0/adapters").exists())
        import contextlib, io
        with contextlib.redirect_stdout(io.StringIO()), self.assertRaises(SystemExit) as cm:
            rl.main(["--adapters", "--help"])
        self.assertEqual(cm.exception.code, 0)

    def test_adapter_that_does_not_vet_is_a_failed_round(self):
        stub = pathlib.Path(self.tmp) / "alder"
        stub.write_text('#!/bin/sh\n[ "$1" = check ] && exit 0\n'
                        'if grep -q BROKEN adapters/*/*.go 2>/dev/null; then echo "adapters/auth/password.go:3:1: undefined: BROKEN" >&2; exit 1; fi\n'
                        'exit 0\n')
        stub.chmod(0o755)
        bad = GOOD + "--- file: adapters/auth/password.go ---\npackage adapters\n\nvar x = BROKEN\n"
        good = "--- file: adapters/auth/password.go ---\npackage adapters\n\nvar x = 1\n"
        s, model = self.run1([bad, good], stub, build=True, adapters=True)
        self.assertEqual(s["outcome"], "pass")
        self.assertFalse(s["rounds"][0]["build"]["ok"])
        self.assertEqual(s["rounds"][0]["build"]["exit"], 1)
        self.assertNotIn("compiler_bug", s)   # the model's Go, not the compiler's
        self.assertIn("undefined: BROKEN", model.prompts[1])
        self.assertIn("--- file: adapters/auth/password.go ---\npackage adapters\n\nvar x = BROKEN", model.prompts[1])
        self.assertTrue(s["rounds"][1]["build"]["ok"])

    @unittest.skipUnless(shutil.which("git"), "git is not on PATH")
    def test_rejected_adapter_hint_with_base(self):
        repo = pathlib.Path(self.tmp) / "repo"
        (repo / "ex" / "app").mkdir(parents=True)
        (repo / "ex" / "adapters" / "auth").mkdir(parents=True)
        (repo / "ex" / "app" / "App.alder").write_text("module App\n")
        (repo / "ex" / "adapters" / "auth" / "password.go").write_text("package adapters\n")
        git(repo, "init", "-q")
        git(repo, "add", ".")
        git(repo, "commit", "-q", "-m", "base")
        go = "--- file: adapters/auth/password.go ---\npackage adapters\n"
        chk = FakeChecker(self.tmp, [(1, ERR, ""), (0, "", "")])
        s, model = self.run1([GOOD + go, GOOD], chk.path, base=repo / "ex")
        self.assertEqual(s["rounds"][0]["rejected"], ["adapters/auth/password.go"])
        self.assertEqual((self.exp / "runs/fake/t/round0/adapters/auth/password.go").read_text(), "package adapters\n")   # base's, not the model's
        p = model.prompts[1]
        self.assertIn("These paths were rejected: adapters/auth/password.go; files must be `app/<Module>.alder`.", p)
        self.assertIn("`adapters/auth/password.go` was not accepted (this run does not take Go files); the `Password` adapter is provided"
                      " \u2014 keep the capability and do not replace it with a password in the code", p)
        # an adapter the base does not provide, next to one it does: name what is provided
        chk = FakeChecker(self.tmp, [(1, ERR, ""), (0, "", "")])
        s, model = self.run1([GOOD + "--- file: adapters/auth/hash.go ---\npackage adapters\n", GOOD], chk.path, base=repo / "ex")
        self.assertIn("`adapters/auth/hash.go` was not accepted (this run does not take Go files); the `Password` adapter is provided;"
                      " `Hash` is not: declare the capability and leave its Go side unwritten", model.prompts[1])
        self.assertNotIn("the `Hash` adapter is provided", model.prompts[1])
        # without a base carrying adapters: the other hint
        chk = FakeChecker(self.tmp, [(1, ERR, ""), (0, "", "")])
        s, model = self.run1([GOOD + go, GOOD], chk.path)
        self.assertIn("`adapters/auth/password.go` was not accepted (this run does not take Go files); "
                      "declare the capability and leave its Go side unwritten", model.prompts[1])
        self.assertNotIn("is provided", model.prompts[1])

    @unittest.skipUnless(shutil.which("git"), "git is not on PATH")
    def test_provided_adapters_labeled_in_retry(self):
        repo = pathlib.Path(self.tmp) / "repo"
        (repo / "ex" / "app").mkdir(parents=True)
        (repo / "ex" / "adapters" / "auth").mkdir(parents=True)
        (repo / "ex" / "adapters" / "mail").mkdir(parents=True)
        (repo / "ex" / "app" / "App.alder").write_text("module App\n")
        (repo / "ex" / "adapters" / "auth" / "password.go").write_text("package adapters\n")
        (repo / "ex" / "adapters" / "mail" / "mail.go").write_text("package adapters\n\n// mail\n")
        git(repo, "init", "-q")
        git(repo, "add", ".")
        git(repo, "commit", "-q", "-m", "base")
        # with --adapters the model overwrites mail/mail.go: auth/password.go stays the base's
        mine = "--- file: adapters/mail/mail.go ---\npackage adapters\n\n// mine\n"
        chk = FakeChecker(self.tmp, [(1, ERR, ""), (0, "", "")])
        s, model = self.run1([GOOD + mine, GOOD], chk.path, base=repo / "ex", adapters=True)
        p = model.prompts[1]
        prev = p[p.index("YOUR PREVIOUS ANSWER"):p.index("COMPILER FEEDBACK")]
        self.assertIn("--- file: adapters/mail/mail.go ---\npackage adapters\n\n// mine", prev)
        self.assertIn(rl.PROVIDED_ADAPTERS, prev)
        provided = prev[prev.index(rl.PROVIDED_ADAPTERS):]
        self.assertIn("--- file: adapters/auth/password.go ---\npackage adapters", provided)
        self.assertNotIn("adapters/mail/mail.go", provided)
        self.assertLess(prev.index("adapters/mail/mail.go"), prev.index(rl.PROVIDED_ADAPTERS))

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
               "rounds": [rd(0, 2, {"E0003": 2}), rd(1, 2, {"E0003": 2}), rd(2, 1, {"E0003": 1})]},
              {"model": "m", "task": "w", "error": None, "final_pass": False, "compiler_bug": True, "outcome": "compiler_bug",
               "compiler_bug_kind": "hang", "harness": {"build": True, "retries": 2}, "rounds": [rd(0, 0, {})]}]
        md = rp.markdown(rp.rows(ss))
        header = md.splitlines()[0]
        for col in ("r0", "r1", "r2", "Result"):
            self.assertIn(f"| {col} |", header)
        self.assertIn("| m | t | 3 | 1 | 0 (build ✓) | pass |", md)
        self.assertIn("| m | u | 0 (build ✗) | — | — | compiler-bug |", md)
        self.assertIn("| m | v | 2 | 2 | 1 | fail |", md)
        self.assertIn("| m | w | 0 | — | — | compiler-bug (hang) |", md)

    def test_report_e5_summaries_unchanged(self):
        import report_loop as rp
        here = pathlib.Path(__file__).resolve().parent / "e5-loop"
        ss = [json.loads(p.read_text()) for p in sorted(here.glob("runs/*/*/summary.json"))]
        if not ss:
            self.skipTest("no E5 runs")
        self.assertEqual(rp.markdown(rp.rows(ss)), (here / "scores.md").read_text())

E0019 = json.dumps({"code": "E0019", "severity": "error", "file": "migrations/schema.json", "line": 1, "col": 1, "endLine": 1, "endCol": 1,
                    "message": "the schema changed since migration 0001 (table Post: column views): run `alder migrate`"}) + "\n"

MIGRATION_0002 = ("-- alder migration 0002_add_views\n-- from aaaaaaaaaaaa\n-- to bbbbbbbbbbbb\n"
                  "-- step: rebuild table Post: Post.views: new non-null column\n-- DROPS DATA: column Post.summary\n\n"
                  'ALTER TABLE "Post__Post" DROP COLUMN "summary";\n')

def migrate_stub(tmp, checks, migrate_exit=0, migrate_out="", migrate_err=""):
    """A stub `alder`: `check` answers per call (checks[i] = (exit, json_stdout)); `migrate DIR`
    writes DIR/migrations/0002_add_views.sql (exit 0) or prints its refusal (exit migrate_exit)."""
    tmp = pathlib.Path(tmp)
    count, calls = tmp / "mig-count", tmp / "mig-calls"
    count.write_text("0")
    calls.write_text("")
    lines = ["#!/bin/sh", f'echo "$PWD|$*" >> {calls}',
             'if [ "$1" = migrate ]; then']
    if migrate_exit == 0:
        lines.append(f'  mkdir -p "$2/migrations" && printf %b {json.dumps(MIGRATION_0002)} > "$2/migrations/0002_add_views.sql"')
    lines += [f'  printf %b {json.dumps(migrate_out)}; printf %b {json.dumps(migrate_err)} 1>&2; exit {migrate_exit}', "fi",
              '[ "$1" = check ] || exit 2', f'n=$(cat {count}); echo $((n+1)) > {count}']
    for i, (exit_code, out) in enumerate(checks):
        lines.append(f'if [ "$n" = "{i}" ]; then printf %b {json.dumps(out)}; exit {exit_code}; fi')
    lines.append("exit 2")
    path = tmp / "alder-migrate-stub"
    path.write_text("\n".join(lines) + "\n")
    path.chmod(0o755)
    stub = StubPath(path)
    stub.calls = calls
    return stub

class MigrateTests(unittest.TestCase):
    """E8: --base carries migrations/, and --migrate runs `alder migrate` when E0019 is the only error."""
    def setUp(self):
        self.tmp = tempfile.mkdtemp()
        self.exp = pathlib.Path(self.tmp) / "exp"
        (self.exp / "tasks").mkdir(parents=True)

    def tearDown(self):
        shutil.rmtree(self.tmp)

    def commands(self, stub):
        return [l.split("|", 1)[1].split()[0] for l in stub.calls.read_text().splitlines()]

    def test_migrate_after_e0019_only(self):
        stub = migrate_stub(self.tmp, [(1, E0019), (0, "")])
        s = rl.run_one(self.exp, rl.FakeModel([GOOD]), "fake", "t", "task", "sys", stub, retries=0, migrate=True)
        self.assertEqual(s["outcome"], "pass")
        self.assertEqual(self.commands(stub), ["check", "migrate", "check"])
        r = s["rounds"][0]
        self.assertEqual(r["migrate"], {"exit": 0, "file": "0002_add_views.sql",
                                        "steps": ["rebuild table Post: Post.views: new non-null column"],
                                        "drops_data": ["column Post.summary"], "codes_before": {"E0019": 1}})
        self.assertEqual((r["codes"], r["errors"], r["exit"]), ({}, 0, 0))
        self.assertEqual(s["harness"]["migrate"], True)
        rd = (self.exp / "runs/fake/t/round0").resolve()
        cwd, args = stub.calls.read_text().splitlines()[1].split("|", 1)
        self.assertEqual((pathlib.Path(cwd).resolve(), args), (rd, f"migrate {rd}"))

    def test_no_migrate_with_other_errors_or_without_flag(self):
        stub = migrate_stub(self.tmp, [(1, E0019 + ERR)])
        s = rl.run_one(self.exp, rl.FakeModel([GOOD]), "fake", "t", "task", "sys", stub, retries=0, migrate=True)
        self.assertNotIn("migrate", s["rounds"][0])
        self.assertEqual(self.commands(stub), ["check"])
        stub = migrate_stub(self.tmp, [(1, E0019)])
        s = rl.run_one(self.exp, rl.FakeModel([GOOD]), "fake", "t2", "task", "sys", stub, retries=0)
        self.assertNotIn("migrate", s["rounds"][0])
        self.assertNotIn("migrate", s["harness"])
        self.assertEqual(self.commands(stub), ["check"])

    def test_migrate_refusal_is_a_failed_round_with_its_output(self):
        refusal = "alder migrate: table Post: new column views is not null and has no default: give it one\n"
        stub = migrate_stub(self.tmp, [(1, E0019), (1, E0019)], migrate_exit=1, migrate_out=refusal)
        model = rl.FakeModel([GOOD, GOOD])
        s = rl.run_one(self.exp, model, "fake", "t", "task", "sys", stub, retries=1, migrate=True)
        self.assertEqual(s["outcome"], "fail")
        self.assertEqual(s["rounds"][0]["migrate"]["exit"], 1)
        self.assertIsNone(s["rounds"][0]["migrate"]["file"])
        p = model.prompts[1]
        self.assertIn(rl.MIGRATE_REFUSED, p)
        self.assertIn("MIGRATE OUTPUT", p)
        self.assertIn("has no default: give it one", p)

    def test_migrate_internal_error_is_compiler_bug(self):
        stub = migrate_stub(self.tmp, [(1, E0019)], migrate_exit=2, migrate_err="alder migrate: internal error: boom\n")
        s = rl.run_one(self.exp, rl.FakeModel([GOOD, GOOD]), "fake", "t", "task", "sys", stub, retries=1, migrate=True)
        self.assertEqual((s["outcome"], s["compiler_bug_kind"]), ("compiler_bug", "migrate"))
        self.assertEqual(len(s["rounds"]), 1)

    @unittest.skipUnless(shutil.which("git"), "git is not on PATH")
    def test_base_carries_migrations_every_round(self):
        repo = pathlib.Path(self.tmp) / "repo"
        (repo / "ex" / "app").mkdir(parents=True)
        (repo / "ex" / "migrations").mkdir(parents=True)
        (repo / "ex" / "app" / "App.alder").write_text("module App\n")
        (repo / "ex" / "migrations" / "0001_initial.sql").write_text("-- alder migration 0001_initial\n")
        (repo / "ex" / "migrations" / "schema.json").write_text("{}\n")
        git(repo, "init", "-q")
        git(repo, "add", ".")
        git(repo, "commit", "-q", "-m", "base")
        # round 0 migrates (0002 written), fails its second check; round 1 starts from the base's
        # migrations again, not round 0's
        stub = migrate_stub(self.tmp, [(1, E0019), (1, ERR), (0, "")])
        s = rl.run_one(self.exp, rl.FakeModel([GOOD, GOOD]), "fake", "t", "task", "sys", stub, retries=1,
                       base=repo / "ex", migrate=True)
        self.assertEqual(s["outcome"], "pass")
        r0, r1 = self.exp / "runs/fake/t/round0", self.exp / "runs/fake/t/round1"
        self.assertEqual(sorted(f.name for f in (r0 / "migrations").iterdir()), ["0001_initial.sql", "0002_add_views.sql", "schema.json"])
        self.assertEqual(sorted(f.name for f in (r1 / "migrations").iterdir()), ["0001_initial.sql", "schema.json"])
        exported, _ = rl.export_base(repo / "ex")
        try:
            self.assertTrue((exported / "migrations" / "0001_initial.sql").exists())
        finally:
            rl.remove_export(exported)

if __name__ == "__main__":
    unittest.main()
