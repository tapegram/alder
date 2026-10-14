"""E11's --cli: the script parser (cmd/alder/drill_test.go's grammar, ported), the executor against the
real binary, the transcript, the names check and summary["cli"]."""
import contextlib, io, json, os, pathlib, shlex, shutil, subprocess, sys, tempfile, unittest
sys.path.insert(0, str(pathlib.Path(__file__).resolve().parent))
import run_loop as rl

ROOT = pathlib.Path(__file__).resolve().parent.parent
DRILL = ROOT / "testdata" / "drill"
TASKS = pathlib.Path(__file__).resolve().parent / "tasks"


def drill(task):
    return (DRILL / task / "script.txt").read_text()


_alder = None


def alder_bin():
    """$ALDER, or ./cmd/alder built once into a temporary directory."""
    global _alder
    if _alder is None:
        if os.environ.get("ALDER"):
            _alder = pathlib.Path(os.environ["ALDER"]).resolve()
        else:
            out = pathlib.Path(tempfile.mkdtemp(prefix="alder-cli-test-")) / "alder"
            subprocess.run(["go", "build", "-o", str(out), "./cmd/alder"], cwd=ROOT, check=True)
            _alder = out
    return _alder


class ParseTests(unittest.TestCase):
    def test_the_two_drills(self):
        for task, n, heredocs in (("t10-teams", 64, 36), ("t9-bookings", 52, 29)):
            steps, refusal = rl.parse_script(drill(task))
            self.assertIsNone(refusal, task)
            self.assertEqual(len(steps), n, task)
            self.assertEqual(sum(1 for st in steps if st["stdin"] is not None), heredocs, task)
            for st in steps:   # the ported word splitter agrees with a shell on every drill line
                line = st["src"].split("\n")[0]
                words = shlex.split(line)[1:]
                if st["stdin"] is not None:
                    words = words[:-1]
                self.assertEqual(st["args"], words, line)

    def test_heredoc_bodies(self):
        steps, _ = rl.parse_script(drill("t10-teams"))
        fill = next(st for st in steps if st["args"][:2] == ["fill", "Member.row"])
        self.assertEqual(fill["args"], ["fill", "Member.row", "-"])
        self.assertEqual(fill["stdin"], "\\team user -> Db.from Member |> Db.where (Member.team.eq team) |> Db.where (Member.user.eq user) |> Db.first\n")
        test = next(st for st in steps if st["args"][1] == 'Member."row is the row of the user in that team"')
        self.assertTrue(test["stdin"].startswith("{\n  let ann <- Db.insert User"))
        self.assertTrue(test["stdin"].endswith("Task.succeed (Maybe.map (\\m -> m.id) found == Just want)\n}\n"))
        self.assertEqual(test["src"].split("\n")[-1], "EOF")
        t9, _ = rl.parse_script(drill("t9-bookings"))
        self.assertEqual(t9[5]["args"], ["fill", "Booking.holds", "-"])
        self.assertEqual(t9[5]["stdin"], "\\d b -> b.from <= d && d < b.to\n")

    def test_comments_and_quotes(self):
        steps, refusal = rl.parse_script("# a comment\n\nalder new type Invite \"Status = A | B\"\n  alder check  --json\n")
        self.assertIsNone(refusal)
        self.assertEqual([st["args"] for st in steps], [["new", "type", "Invite", "Status = A | B"], ["check", "--json"]])
        self.assertEqual(steps[1]["line"], 4)

    def test_a_line_that_is_not_alder_is_refused(self):
        steps, refusal = rl.parse_script("alder check\ncd teams\nalder check\n")
        self.assertEqual(len(steps), 1)   # the commands before it still run
        self.assertEqual(refusal["kind"], "not-a-command")
        self.assertEqual((refusal["line"], refusal["text"]), (2, "cd teams"))
        for bad in ("ls app", "alder", "echo x > app/A.alder", "  # an indented comment is a line too"):
            self.assertEqual(rl.parse_script(bad + "\n")[1]["kind"], "not-a-command", bad)

    def test_parse_errors(self):
        self.assertEqual(rl.parse_script("alder new type X 'A | B\n")[1]["kind"], "parse")
        steps, refusal = rl.parse_script("alder check\nalder fill A.x - <<'EOF'\n1\n")
        self.assertEqual((len(steps), refusal["kind"], refusal["line"]), (1, "parse", 2))
        self.assertIn("no EOF line", refusal["reason"])

    def test_extract_script(self):
        script, tag, refusal = rl.extract_script("Here it is:\n```alder-cli\nalder check\n```\nDone.\n")
        self.assertEqual((script, tag, refusal), ("alder check\n", "alder-cli", None))
        script, tag, refusal = rl.extract_script("```alder\nmodule A\n```\n```sh\nalder check\n```\n")
        self.assertEqual((script, tag), ("alder check\n", "sh"))   # other blocks are ignored
        _, _, refusal = rl.extract_script("```alder-cli\nalder check\n```\n```sh\nalder test\n```\n")
        self.assertIn("2 script blocks", refusal["reason"])
        _, _, refusal = rl.extract_script("--- file: app/A.alder ---\n```alder\nmodule A\n```\n")
        self.assertEqual(refusal["kind"], "parse")
        self.assertIn("no ```alder-cli block (it has: alder)", refusal["reason"])
        self.assertIn("you may not write files", refusal["reason"])

    def test_app_name(self):
        self.assertEqual([rl.cli_app_name(t) for t in ("t5-ownership", "t10-teams", "t2-newpage", "t9-bookings")],
                         ["ownership", "teams", "newpage", "bookings"])


@unittest.skipUnless(shutil.which("go") or os.environ.get("ALDER"), "go is not on PATH (or set ALDER)")
class ExecuteTests(unittest.TestCase):
    def setUp(self):
        self.alder = alder_bin()
        self.tmp = pathlib.Path(tempfile.mkdtemp())
        self.addCleanup(shutil.rmtree, self.tmp, True)
        rec = rl.run_command(self.alder, ["new", "bookings", "--bare"], None, self.tmp)
        self.assertEqual(rec["exit"], 0, rec["stderr"])
        self.app = self.tmp / "bookings"

    def test_the_t9_drill_first_six(self):
        steps, _ = rl.parse_script(drill("t9-bookings"))
        records, refusal, stop = rl.execute_script(self.alder, steps[:6], self.app)
        self.assertIsNone(refusal)
        self.assertIsNone(stop)
        self.assertEqual([r["exit"] for r in records], [0] * 6)
        self.assertEqual(records[5]["stdin_lines"], ["\\d b -> b.from <= d && d < b.to"])
        self.assertEqual(records[0]["argv"][:3], ["alder", "new", "table"])
        exit_code, diags, _, reason = rl.run_check(self.alder, self.app)
        self.assertIsNone(reason)
        self.assertEqual((exit_code, [d for d in diags if d["severity"] == "error"]), (0, []))
        self.assertIn("holds", (self.app / "app" / "Booking.alder").read_text())

    def test_stops_at_the_first_failure(self):
        steps, _ = rl.parse_script("alder new table Room name:Text.NonEmpty\nalder new table Room name:Text\nalder check\n")
        records, refusal, stop = rl.execute_script(self.alder, steps, self.app)
        self.assertEqual([r["exit"] for r in records], [0, 1])
        self.assertEqual((refusal["kind"], refusal["line"], stop), ("exit1", 2, None))
        self.assertIn("a generator never overwrites a file", refusal["reason"])

    def test_not_allowed_and_cap(self):
        steps, _ = rl.parse_script("alder check\nalder run\n")
        records, refusal, _ = rl.execute_script(self.alder, steps, self.app)
        self.assertEqual((len(records), refusal["kind"], refusal["line"]), (1, "not-a-command", 2))
        steps, _ = rl.parse_script("alder check\n" * 3)
        records, refusal, _ = rl.execute_script(self.alder, steps, self.app, cap=2)
        self.assertEqual((len(records), refusal["kind"], refusal["line"]), (2, "cap", 3))

    def test_usage_error_is_exit2(self):
        steps, _ = rl.parse_script("alder new tabel X\n")
        _, refusal, stop = rl.execute_script(self.alder, steps, self.app)
        self.assertEqual((refusal["kind"], stop), ("exit2", None))


class SummaryTests(unittest.TestCase):
    def canned(self):
        """Two rounds: round 0 ran 3 of 4 commands (a fill of 2 lines, one `--decl`) and failed on
        the third; round 1's answer had no script."""
        steps, _ = rl.parse_script("alder new decl A x \"Int\"\nalder fill --decl A.x - <<'EOF'\n1\n+ 1\nEOF\n"
                                   "alder fill A.y - <<'EOF'\n2\nEOF\nalder check\n")
        records = [{"line": st["line"], "exit": e, "stdout": "", "stderr": err} for st, e, err in
                   zip(steps, (0, 0, 1), ("", "", "alder fill: `A.y` names no declaration\n"))]
        refusal = {"kind": "exit1", "line": 6, "text": "alder fill A.y - <<'EOF'", "reason": "alder fill: `A.y` names no declaration"}
        r0 = rl.cli_round_record("…", "alder-cli", steps, records, refusal)
        r1 = rl.cli_round_record(None, None, [], [], {"kind": "parse", "line": 0, "text": "", "reason": "no block"})
        return steps, records, refusal, [{"n": 0, "cli": r0}, {"n": 1, "cli": r1}]

    def test_summary_fields(self):
        _, _, _, rounds = self.canned()
        self.assertEqual(rounds[0]["cli"]["stdin_lines"], 3)
        s = rl.cli_summary(rounds, "x", {"ok": True, "matches": [], "mismatches": []})
        self.assertEqual({k: s[k] for k in ("app_name", "rounds", "commands", "executed", "stdin_lines", "max_body", "decl_uses")},
                         {"app_name": "x", "rounds": 2, "commands": 4, "executed": 3, "stdin_lines": 3, "max_body": 2, "decl_uses": 1})
        self.assertEqual(s["refused"]["count"], 2)
        self.assertEqual(s["refused"]["kinds"], {"exit1": 1, "parse": 1})
        self.assertEqual(s["refused"]["items"][0], {"round": 0, "kind": "exit1", "line": 6, "text": "alder fill A.y - <<'EOF'",
                                                    "reason": "alder fill: `A.y` names no declaration"})
        self.assertTrue(s["names"]["ok"])

    def test_transcript(self):
        steps, records, refusal, _ = self.canned()
        t = rl.render_transcript(0, steps, records, refusal, "VERDICT")
        self.assertIn("$ alder fill --decl A.x - <<'EOF'\n1\n+ 1\nEOF\n[exit 0]", t)
        self.assertIn("alder fill: `A.y` names no declaration\n[exit 1]", t)
        self.assertIn("The harness stopped at line 6 of your script: `alder fill A.y - <<'EOF'`: it exited 1. The commands after it were not run.", t)
        self.assertNotIn("alder check", t)
        self.assertTrue(t.rstrip().endswith("VERDICT"))
        p = rl.cli_prompt("TASK", "PAGE", None, [t])
        self.assertTrue(p.startswith("TASK\n\n" + rl.CLI_RULE + "\n\nPAGE\n"))
        self.assertTrue(p.rstrip().endswith(rl.CLI_CONTINUE))

    def test_names_check(self):
        task = (TASKS / "t5-ownership.md").read_text()
        self.assertIsNone(rl.names_check((TASKS / "t2-newpage.md").read_text(), pathlib.Path(".")))
        tmp = pathlib.Path(tempfile.mkdtemp())
        self.addCleanup(shutil.rmtree, tmp, True)
        (tmp / "app").mkdir()
        (tmp / "app" / "User.alder").write_text("module User\n\ntable User {\n  id: UserId @id,\n  email: Text.Email @unique,\n"
                                                "  name: Text.NonEmpty,\n  passwordHash: Text,\n  created: Time @created,\n}\n")
        (tmp / "app" / "Note.alder").write_text("module Note\n\ntable Note {\n  id: NoteId @id,\n  title: Text,\n}\n")
        (tmp / "app" / "App.alder").write_text("module App\n\nsession { user: Maybe User.UserId }\n\nmsg Logout\n")
        r = rl.names_check(task, tmp)
        self.assertFalse(r["ok"])
        self.assertIn("table User", r["matches"])
        self.assertIn("session", r["matches"])
        self.assertIn("msg App.Logout", r["matches"])
        self.assertTrue(any(m.startswith("table Note: ") for m in r["mismatches"]))
        self.assertIn("page Notes.Index /: missing", r["mismatches"])
        self.assertIn("form Notes.Index.NewNote: missing", r["mismatches"])


class ReportTests(unittest.TestCase):
    def test_cli_columns(self):
        import report_loop
        base = {"model": "claude-haiku", "task": "t9-bookings", "error": None, "final_pass": True, "outcome": "pass",
                "harness": {"build": True, "test": True, "mutate": True, "cli": True, "sample": "s1"},
                "rounds": [{"n": 0, "errors": 0, "codes": {}, "build": {"ok": True}}]}
        s = dict(base, cli={"commands": 50, "executed": 48, "refused": {"count": 2, "kinds": {"exit1": 2}, "items": []},
                            "stdin_lines": 120, "max_body": 14, "decl_uses": 3, "names": {"ok": False, "matches": [], "mismatches": ["x"]}})
        out = report_loop.markdown(report_loop.rows([s, dict(base, task="t5-ownership")]))
        head, _, row, other = out.splitlines()
        self.assertIn("| Cmds | Refused | Stdin | Max body | --decl | Names | Result |", head)
        self.assertIn("| 48/50 | 2 (exit1×2) | 120 | 14 | 3 | 1 differ | pass |", row)
        self.assertIn("| — | — | — | — | — | — | pass |", other)
        self.assertNotIn("Cmds", report_loop.markdown(report_loop.rows([dict(base, harness={"build": True})])))


@unittest.skipUnless(shutil.which("go") or os.environ.get("ALDER"), "go is not on PATH (or set ALDER)")
class RunOneTests(unittest.TestCase):
    def test_two_failing_rounds(self):
        """Round 0: the T9 drill's first six commands, then `ls`; round 1: files, no script. Neither
        passes; the records, the copy forward and the retry prompt."""
        alder = alder_bin()
        exp = pathlib.Path(tempfile.mkdtemp())
        self.addCleanup(shutil.rmtree, exp, True)
        steps, _ = rl.parse_script(drill("t9-bookings"))
        script = "\n".join(st["src"] for st in steps[:6]) + "\nls app\n"
        model = rl.FakeModel(["Here:\n```alder-cli\n" + script + "```\n", "--- file: app/A.alder ---\nmodule A\n"])
        task = (TASKS / "t9-bookings.md").read_text()
        s = rl.run_one(exp, model, "fake", "t9-bookings", task, "SYSTEM", alder, retries=1, build=True, test=True,
                       mutate=True, cli=True, cli_page="PAGE")
        run = exp / "runs" / "fake" / "t9-bookings"
        self.assertEqual(s["outcome"], "fail")
        self.assertTrue(s["harness"]["cli"])
        self.assertEqual(s["cli"]["setup"]["exit"], 0)
        c = s["cli"]
        self.assertEqual((c["app_name"], c["rounds"], c["commands"], c["executed"]), ("bookings", 2, 6, 6))
        self.assertEqual(c["refused"]["kinds"], {"not-a-command": 1, "parse": 1})
        self.assertEqual(c["stdin_lines"], 1)
        self.assertFalse(c["names"]["ok"])
        self.assertIn("table User", c["names"]["matches"])
        self.assertEqual(s["rounds"][0]["errors"], 0)
        self.assertEqual(len((run / "round0" / "commands.jsonl").read_text().splitlines()), 6)
        self.assertEqual((run / "round0" / "script.txt").read_text(), script)
        self.assertFalse((run / "round1" / "script.txt").exists())
        self.assertTrue((run / "round1" / "bookings" / "app" / "Booking.alder").is_file())   # copied forward
        self.assertFalse((run / "round1" / "bookings" / "build").exists())
        t0 = (run / "round0" / "transcript.md").read_text()
        self.assertIn("The harness refused line 9 of your script: `ls app`: a step is an `alder` command", t0)
        self.assertIn("hole ?afterLogin", t0)   # the build refuses the login's hole: the verdict
        self.assertEqual(model.prompts[0], rl.cli_prompt(task, "PAGE"))
        self.assertTrue(model.prompts[1].endswith(t0 + "\n" + rl.CLI_CONTINUE + "\n"))
        t1 = (run / "round1" / "transcript.md").read_text()
        self.assertIn("you may not write files", t1)
        summary = json.loads((run / "summary.json").read_text())
        self.assertEqual(summary["cli"]["refused"]["count"], 2)

    def test_flags(self):
        with self.assertRaises(ValueError):
            rl.run_one(pathlib.Path("."), rl.FakeModel([]), "f", "t", "", "", pathlib.Path("alder"), build=True, test=True, cli=True)
        with self.assertRaises(ValueError):
            rl.run_one(pathlib.Path("."), rl.FakeModel([]), "f", "t", "", "", pathlib.Path("alder"), build=True, test=True,
                       mutate=True, feedback=1, cli=True)
        with self.assertRaises(SystemExit), contextlib.redirect_stderr(io.StringIO()):
            rl.main(["--exp", ".", "--model", "claude:x", "--task", "t", "--cli", "--feedback", "1"])


if __name__ == "__main__":
    unittest.main()
