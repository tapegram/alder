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

    def test_base_seeds_round0(self):
        # A modification task (T2) emits only changed files; round 0 must start from the base app.
        base = pathlib.Path(self.tmp) / "base"
        (base / "app" / "Todos").mkdir(parents=True)
        (base / "app" / "App.alder").write_text("module App\n")
        (base / "app" / "Todos" / "Index.alder").write_text("module Todos.Index\n-- old\n")
        chk = FakeChecker(self.tmp, [(0, "", "")])
        model = rl.FakeModel(["--- file: app/Todos/Index.alder ---\nmodule Todos.Index\n-- new\n"
                              "--- file: app/Profile.alder ---\nmodule Profile\n"])
        s = rl.run_one(self.exp, model, "fake", "t", "task", "sys", chk.path, retries=0, base=base)
        self.assertEqual(s["rounds"][0]["files"], 3)
        self.assertTrue((self.exp / "runs/fake/t/round0/app/App.alder").exists())
        self.assertIn("new", (self.exp / "runs/fake/t/round0/app/Todos/Index.alder").read_text())
        self.assertEqual((base / "app" / "Todos" / "Index.alder").read_text(), "module Todos.Index\n-- old\n")

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
        code, diags, _ = rl.run_check(chk.path, pathlib.Path(self.tmp))
        self.assertEqual(code, 1)
        self.assertEqual(len(diags), 1)
        self.assertEqual(diags[0]["code"], "E0003")

    def test_run_check_missing_binary(self):
        missing = pathlib.Path(self.tmp) / "nope"
        code, diags, err = rl.run_check(missing, pathlib.Path(self.tmp))
        self.assertEqual((code, diags), (2, []))
        self.assertIn(str(missing), err)

    def test_traversal_not_written_and_recorded(self):
        chk = FakeChecker(self.tmp, [(0, "", "")])
        model = rl.FakeModel([GOOD + "--- file: app/../../evil.alder ---\nmodule E\n"])
        s = rl.run_one(self.exp, model, "fake", "t", "task", "sys", chk.path, retries=0)
        self.assertEqual(s["rounds"][0]["rejected"], ["app/../../evil.alder"])
        self.assertFalse(list(pathlib.Path(self.tmp).rglob("evil.alder")))

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

if __name__ == "__main__":
    unittest.main()
