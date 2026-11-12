"""Tests for run-matrix.sh: each drives the script in a clone of the repository
with DRY_RUN or PREFLIGHT_ONLY, so no model is ever called."""
import hashlib
import os
import shutil
import subprocess
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.abspath(os.path.join(HERE, "..", ".."))
BINARY = os.path.join(HERE, "alder")
HASKU = "claude:claude-haiku-5-5"
FIRST = "s1 %s t2-newpage" % HASKU


def sh(args, cwd, env=None, check=True):
    r = subprocess.run(args, cwd=cwd, env=env, capture_output=True, text=True)
    if check and r.returncode != 0:
        raise RuntimeError("%s failed: %s%s" % (args, r.stdout, r.stderr))
    return r


@unittest.skipUnless(os.path.exists(BINARY), "no pinned binary at experiments/e12-routes/alder")
class RunMatrix(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.mkdtemp(prefix="e12-matrix-test.")
        self.addCleanup(shutil.rmtree, self.tmp, True)
        self.repo = os.path.join(self.tmp, "repo")
        sh(["git", "clone", "--local", "-q", ROOT, self.repo], self.tmp)
        self.e = os.path.join(self.repo, "experiments", "e12-routes")
        for name in ("run-matrix.sh", "pin.sh"):
            shutil.copy(os.path.join(HERE, name), os.path.join(self.e, name))
        shutil.copy(BINARY, os.path.join(self.e, "alder"))  # untracked, as in the real tree
        self.git("config", "user.email", "t@example.com")
        self.git("config", "user.name", "t")
        self.git("add", "experiments/e12-routes/run-matrix.sh", "experiments/e12-routes/pin.sh")
        self.git("commit", "-q", "--allow-empty", "-m", "fixture: scripts")
        # the prompt, built from the clone's HEAD by pin.sh, committed
        sh(["bash", os.path.join(self.e, "pin.sh"), "prompt", "HEAD", self.e], self.repo)
        self.git("add", "-f", "experiments/e12-routes/prompt-reference.md")
        self.git("commit", "-q", "-m", "fixture: prompt")
        self.write_run_md(self.git("rev-parse", "HEAD").stdout.strip(), self.sha())

    def git(self, *a):
        return sh(["git", *a], self.repo)

    def sha(self):
        with open(BINARY, "rb") as f:
            return hashlib.sha256(f.read()).hexdigest()

    def write_run_md(self, commit, sha):
        with open(os.path.join(self.e, "RUN.md"), "w") as f:
            f.write("# fixture\n\nE12_COMMIT: %s\nALDER_SHA256: %s\n" % (commit, sha))

    def run_matrix(self, clock=None, **env):
        e = dict(os.environ)
        e.pop("E12_CLOCK", None)
        e.update(env)
        if clock:
            e["E12_CLOCK"] = clock
        return sh(["bash", os.path.join(self.e, "run-matrix.sh")], self.repo, e, check=False)

    def dry(self, clock="6 1200", **env):
        return self.run_matrix(clock, DRY_RUN="1", **env)

    def progress(self):
        p = os.path.join(self.e, "logs", "matrix.progress")
        if not os.path.exists(p):
            return ""
        with open(p) as f:
            return f.read()

    def test_cell_order_and_flags(self):
        r = self.dry()
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        lines = [l for l in r.stdout.splitlines() if " :: " in l]
        self.assertEqual(len(lines), 78)
        self.assertTrue(lines[0].startswith(FIRST + " ::"), lines[0])
        self.assertIn("--base examples/todos", lines[0])
        self.assertNotIn("--hidden", lines[0])
        for flag in ("--condition files", "--build", "--test", "--mutate", "--retries 5",
                     "--exp experiments/e12-routes", "--alder experiments/e12-routes/alder",
                     "--model " + HASKU, "--sample s1"):
            self.assertIn(flag, lines[0])
        self.assertIn("/experiments/e12-routes/prompt-reference.md", lines[0])
        # sample -> model -> task
        self.assertTrue(lines[13].startswith("s1 claude:claude-sonnet-5-5 t2-newpage"))
        self.assertTrue(lines[26].startswith("s1 claude:claude-opus-5-5 t2-newpage"))
        self.assertTrue(lines[39].startswith("s2 %s t2-newpage" % HASKU))
        self.assertTrue(lines[-1].startswith("s2 claude:claude-opus-5-5 t15-pantry"))
        by_task = {l.split()[2]: l for l in lines[:13]}
        self.assertEqual(len(by_task), 13)
        self.assertNotIn("--hidden", by_task["t3-newapp"])
        self.assertNotIn("--base", by_task["t3-newapp"])
        for t, l in by_task.items():
            if t not in ("t2-newpage", "t3-newapp"):
                self.assertIn("--hidden experiments/e12-routes/hidden", l, t)
            self.assertEqual("--migrate" in l, t == "t7-drafts", t)
        self.assertIn("--base experiments/e12-routes/base/blog-nodrafts", by_task["t7-drafts"])
        self.assertNotIn("--feedback", lines[0])
        self.assertNotIn("--adapters", lines[0])
        self.assertNotIn("--cli", lines[0])

    def test_env_narrows_the_matrix(self):
        r = self.dry(MODELS="claude:claude-opus-5-5", SAMPLES="s2", TASKS="t5-ownership t6-orders")
        lines = [l for l in r.stdout.splitlines() if " :: " in l]
        self.assertEqual([l.split(" ::")[0] for l in lines],
                         ["s2 claude:claude-opus-5-5 t5-ownership", "s2 claude:claude-opus-5-5 t6-orders"])

    def test_done_cell_skipped(self):
        d = os.path.join(self.e, "runs", "claude-claude-haiku-5-5", "t2-newpage-s1")
        os.makedirs(d)
        with open(os.path.join(d, "summary.json"), "w") as f:
            f.write("{}")
        r = self.dry()
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertEqual(len([l for l in r.stdout.splitlines() if " :: " in l]), 77)
        self.assertNotIn(FIRST + " ::", r.stdout)
        self.assertIn(FIRST + " skipped", self.progress())
        self.assertTrue(os.path.exists(os.path.join(d, "summary.json")))

    def test_partial_cell_is_rerun(self):
        d = os.path.join(self.e, "runs", "claude-claude-haiku-5-5", "t2-newpage-s1")
        os.makedirs(os.path.join(d, "round1"))
        with open(os.path.join(d, "round1", "x"), "w") as f:
            f.write("stale")
        r = self.dry()
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertFalse(os.path.exists(d))
        self.assertIn(FIRST + " rerun", self.progress())
        self.assertIn(FIRST + " ::", r.stdout)
        self.assertEqual(len([l for l in r.stdout.splitlines() if " :: " in l]), 78)

    def test_stop_at_weekday_only(self):
        r = self.dry("1 0850")
        self.assertEqual(r.returncode, 3, r.stdout + r.stderr)
        self.assertIn("STOPPED before " + FIRST, r.stdout)
        self.assertIn("STOPPED before " + FIRST, self.progress())
        self.assertNotIn(" :: ", r.stdout)
        for clock in ("6 0850", "7 1200", "1 1705", "5 2300", "1 0844", "1 0000"):
            r = self.dry(clock)
            self.assertEqual(r.returncode, 0, "%s: %s%s" % (clock, r.stdout, r.stderr))
        for clock in ("1 0845", "3 1200", "5 1659"):
            self.assertEqual(self.dry(clock).returncode, 3, clock)
        r = self.dry("1 0810", STOP_AT="0800")
        self.assertEqual(r.returncode, 3)
        self.assertEqual(self.dry("1 0810", STOP_AT="0900").returncode, 0)

    def test_stop_mid_matrix_names_the_next_cell(self):
        # a stop is checked before every cell; with the clock fixed inside the window it
        # always names the first cell not yet done
        d = os.path.join(self.e, "runs", "claude-claude-haiku-5-5", "t2-newpage-s1")
        os.makedirs(d)
        with open(os.path.join(d, "summary.json"), "w"):
            pass
        r = self.dry("2 0900")
        self.assertEqual(r.returncode, 3)
        self.assertIn("STOPPED before " + FIRST, r.stdout)

    def test_preflight_refuses_changed_input(self):
        ok = self.run_matrix(PREFLIGHT_ONLY="1")
        self.assertEqual(ok.returncode, 0, ok.stdout + ok.stderr)
        task = os.path.join(self.e, "..", "tasks", "t5-ownership.md")
        with open(task, "a") as f:
            f.write("\nedited after the gate\n")
        r = self.run_matrix(PREFLIGHT_ONLY="1")
        self.assertEqual(r.returncode, 2)
        self.assertIn("experiments/tasks", r.stderr)
        # committed but after the pin: still refused, path named
        self.git("commit", "-q", "-am", "edit after pin")
        r = self.run_matrix(PREFLIGHT_ONLY="1")
        self.assertEqual(r.returncode, 2)
        self.assertIn("experiments/tasks changed since", r.stderr)

    def test_preflight_refuses_untracked_input(self):
        with open(os.path.join(self.e, "hidden", "stray.alder"), "w") as f:
            f.write("x")
        r = self.run_matrix(PREFLIGHT_ONLY="1")
        self.assertEqual(r.returncode, 2)
        self.assertIn("experiments/e12-routes/hidden", r.stderr)

    def test_preflight_refuses_wrong_binary_hash(self):
        self.write_run_md(self.git("rev-parse", "HEAD").stdout.strip(), "0" * 64)
        r = self.run_matrix(PREFLIGHT_ONLY="1")
        self.assertEqual(r.returncode, 2)
        self.assertIn("experiments/e12-routes/alder", r.stderr)

    def test_preflight_refuses_prompt_built_from_other_commit(self):
        # RUN.md pins the commit before the prompt file existed in its final form:
        # a prompt that is not what E12_COMMIT builds is refused
        with open(os.path.join(self.e, "prompt-reference.md"), "a") as f:
            f.write("\nstale\n")
        self.git("add", "-f", "experiments/e12-routes/prompt-reference.md")
        self.git("commit", "-q", "-m", "stale prompt")
        self.write_run_md(self.git("rev-parse", "HEAD").stdout.strip(), self.sha())
        r = self.run_matrix(PREFLIGHT_ONLY="1")
        self.assertEqual(r.returncode, 2)
        self.assertIn("prompt-reference.md", r.stderr)


if __name__ == "__main__":
    unittest.main()
