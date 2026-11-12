"""Tests for run-matrix.sh: each drives the script in a clone of the repository with
DRY_RUN, PREFLIGHT_ONLY or a stub RUN_LOOP, so no model is ever called.

Slow (a clone and a prompt build per test) and not run by CI. The suite is skipped when the
pinned binary experiments/e12-routes/alder (gitignored) is absent."""
import hashlib
import os
import signal
import shutil
import subprocess
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.abspath(os.path.join(HERE, "..", ".."))
BINARY = os.path.join(HERE, "alder")
HAIKU = "claude:claude-haiku-5-5"
TASKS = ["t2-newpage", "t3-newapp", "t4-appmsg", "t5-ownership", "t6-orders", "t7-drafts", "t9-bookings",
         "t10-teams", "t11-workspace", "t12-search", "t13-feed", "t14-board", "t15-pantry"]
FIRST = "s1 %s t2-newpage" % HAIKU


STUB = r"""#!/usr/bin/env bash
# stand-in for run_loop.py: STUB_MODE = ok | harness | fail | sleep | stdin
while [ $# -gt 0 ]; do case "$1" in --model) m="$2";; --task) t="$2";; --sample) s="$2";; esac; shift; done
d="experiments/e12-routes/runs/${m//:/-}/$t-$s"
mkdir -p "$d"
case "$STUB_MODE" in
  ok) echo '{"outcome": "pass"}' > "$d/summary.json" ;;
  harness) echo '{"outcome": "harness_error"}' > "$d/summary.json"; exit 1 ;;
  fail) exit 1 ;;
  stdin) read -t 3 x; echo $? > "$STUB_PIDFILE"; echo '{"outcome": "pass"}' > "$d/summary.json" ;;
  sleep) sleep 60 & echo $! > "$STUB_PIDFILE"; wait ;;
esac
"""


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
        # tasks whose material other work has not committed yet get placeholders in the clone
        for t in TASKS:
            txt = os.path.join(self.repo, "experiments", "tasks", t + ".md")
            if not os.path.exists(txt):
                with open(txt, "w") as f:
                    f.write("placeholder\n")
            if t not in ("t2-newpage", "t3-newapp"):
                hd = os.path.join(self.e, "hidden", t)
                os.makedirs(hd, exist_ok=True)
                if not os.listdir(hd):
                    with open(os.path.join(hd, "placeholder.alder"), "w") as f:
                        f.write("placeholder\n")
        self.git("add", "experiments/tasks", "experiments/e12-routes/hidden",
                 "experiments/e12-routes/run-matrix.sh", "experiments/e12-routes/pin.sh")
        self.stub = os.path.join(self.tmp, "stub-run-loop.sh")
        with open(self.stub, "w") as f:
            f.write(STUB)
        os.chmod(self.stub, 0o755)
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

    def real(self, clock="6 1200", mode="ok", **env):
        """a real-mode run with the stub run_loop"""
        env.setdefault("STUB_MODE", mode)
        return self.run_matrix(clock, RUN_LOOP=self.stub, **env)

    def cell_dir(self, label="claude-claude-haiku-5-5", cell="t2-newpage-s1"):
        return os.path.join(self.e, "runs", label, cell)

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
                     "--model " + HAIKU, "--sample s1"):
            self.assertIn(flag, lines[0])
        self.assertIn("/experiments/e12-routes/prompt-reference.md", lines[0])
        # sample -> model -> task
        self.assertTrue(lines[13].startswith("s1 claude:claude-sonnet-5-5 t2-newpage"))
        self.assertTrue(lines[26].startswith("s1 claude:claude-opus-5-5 t2-newpage"))
        self.assertTrue(lines[39].startswith("s2 %s t2-newpage" % HAIKU))
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
        d = self.cell_dir()
        os.makedirs(d)
        with open(os.path.join(d, "summary.json"), "w") as f:
            f.write('{"outcome": "pass"}')
        r = self.dry()
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertEqual(len([l for l in r.stdout.splitlines() if " :: " in l]), 77)
        self.assertNotIn(FIRST + " ::", r.stdout)
        self.assertIn(FIRST + " would skip", r.stdout)
        self.assertEqual(self.progress(), "")  # a dry run writes no progress
        r = self.real(TASKS="t2-newpage", MODELS=HAIKU, SAMPLES="s1")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertIn(FIRST + " skipped", self.progress())
        self.assertTrue(os.path.exists(os.path.join(d, "summary.json")))

    def test_partial_cell_is_rerun(self):
        d = self.cell_dir()
        os.makedirs(os.path.join(d, "round1"))
        with open(os.path.join(d, "round1", "x"), "w") as f:
            f.write("stale")
        # a dry run touches nothing
        r = self.dry()
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertIn(FIRST + " would rerun", r.stdout)
        self.assertIn(FIRST + " ::", r.stdout)
        self.assertEqual(len([l for l in r.stdout.splitlines() if " :: " in l]), 78)
        self.assertTrue(os.path.exists(os.path.join(d, "round1", "x")))
        self.assertEqual(self.progress(), "")
        # a real run deletes the cut cell and runs it again
        r = self.real(TASKS="t2-newpage", MODELS=HAIKU, SAMPLES="s1")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertFalse(os.path.exists(os.path.join(d, "round1")))
        self.assertTrue(os.path.exists(os.path.join(d, "summary.json")))
        p = self.progress()
        self.assertIn(FIRST + " rerun", p)
        self.assertIn(FIRST + " done", p)

    def test_harness_error_cell_is_rerun_and_stops_the_matrix(self):
        d = self.cell_dir()
        os.makedirs(d)
        with open(os.path.join(d, "summary.json"), "w") as f:
            f.write('{"outcome": "harness_error", "harness_error": "usage limit"}')
        r = self.dry()
        self.assertIn(FIRST + " would rerun", r.stdout)
        r = self.real(mode="harness", TASKS="t2-newpage t3-newapp", MODELS=HAIKU, SAMPLES="s1")
        self.assertEqual(r.returncode, 4, r.stdout + r.stderr)
        self.assertIn("HARNESS_ERROR at " + FIRST, r.stdout)
        p = self.progress()
        self.assertIn(FIRST + " rerun", p)
        self.assertIn("HARNESS_ERROR at " + FIRST, p)
        self.assertFalse(os.path.exists(self.cell_dir(cell="t3-newapp-s1")))  # it stopped

    def test_failed_exit_is_logged_and_the_matrix_goes_on(self):
        r = self.real(mode="fail", TASKS="t2-newpage t3-newapp", MODELS=HAIKU, SAMPLES="s1")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        p = self.progress()
        self.assertIn(FIRST + " failed (exit 1)", p)
        self.assertIn("t3-newapp failed (exit 1)", p)
        self.assertNotIn(" done", p)

    def test_lock_refuses_a_second_run_and_reclaims_a_stale_one(self):
        lock = os.path.join(self.e, "logs", "matrix.lock")
        os.makedirs(lock)
        with open(os.path.join(lock, "pid"), "w") as f:
            f.write(str(os.getpid()))  # this test process: alive
        r = self.dry()
        self.assertEqual(r.returncode, 2)
        self.assertIn("matrix.lock", r.stderr)
        dead = subprocess.Popen(["true"])
        dead.wait()
        with open(os.path.join(lock, "pid"), "w") as f:
            f.write(str(dead.pid))
        r = self.dry()
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertFalse(os.path.exists(lock))  # released on exit

    def test_watchdog_kills_a_cell_at_nine(self):
        pidfile = os.path.join(self.tmp, "sleep.pid")
        r = self.real(mode="sleep", TASKS="t2-newpage", MODELS=HAIKU, SAMPLES="s1",
                      E12_DEADLINE_SECS="2", STUB_PIDFILE=pidfile)
        self.assertEqual(r.returncode, 3, r.stdout + r.stderr)
        self.assertIn("killed at 09:00", r.stdout)
        self.assertIn(FIRST + " killed at 09:00", self.progress())
        self.assertFalse(os.path.exists(os.path.join(self.cell_dir(), "summary.json")))
        with open(pidfile) as f:
            pid = int(f.read())
        with self.assertRaises(ProcessLookupError):
            os.kill(pid, 0)  # the cell's child died with its group
        # and the cut cell is rerun next time
        r = self.real(TASKS="t2-newpage", MODELS=HAIKU, SAMPLES="s1")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertIn(FIRST + " rerun", self.progress())

    def test_bad_deadline_refuses_to_run_unwatched(self):
        r = self.real(TASKS="t2-newpage", MODELS=HAIKU, SAMPLES="s1", E12_DEADLINE_SECS="abc")
        self.assertEqual(r.returncode, 2, r.stdout + r.stderr)
        self.assertIn("09:00", r.stderr)
        self.assertFalse(os.path.exists(self.cell_dir()))

    def test_cell_gets_no_terminal_stdin(self):
        # the stub reads stdin; with the parent's stdin an open pipe it must still see EOF (rc 1),
        # not block (rc > 128)
        out = os.path.join(self.tmp, "stdin.rc")
        e = dict(os.environ, STUB_MODE="stdin", STUB_PIDFILE=out, RUN_LOOP=self.stub, E12_CLOCK="6 1200",
                 TASKS="t2-newpage", MODELS=HAIKU, SAMPLES="s1")
        p = subprocess.Popen(["bash", os.path.join(self.e, "run-matrix.sh")], cwd=self.repo, env=e,
                             stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
        text = p.stdout.read()
        p.wait()
        p.stdin.close()
        p.stdout.close()
        self.assertEqual(p.returncode, 0, text)
        with open(out) as f:
            self.assertEqual(f.read().strip(), "1")

    def test_empty_fresh_lock_is_held(self):
        os.makedirs(os.path.join(self.e, "logs", "matrix.lock"))  # holder between mkdir and its pid write
        r = self.dry()
        self.assertEqual(r.returncode, 2)
        self.assertIn("matrix.lock", r.stderr)

    def test_stop_at_weekday_only(self):
        r = self.dry("1 0850")
        self.assertEqual(r.returncode, 3, r.stdout + r.stderr)
        self.assertIn("STOPPED before " + FIRST, r.stdout)
        self.assertNotIn(" :: ", r.stdout)
        for clock in ("6 0850", "7 1200", "1 1700", "1 1705", "5 2300", "1 0844", "1 0000"):
            r = self.dry(clock)
            self.assertEqual(r.returncode, 0, "%s: %s%s" % (clock, r.stdout, r.stderr))
        for clock in ("1 0845", "3 1200", "5 1659"):
            self.assertEqual(self.dry(clock).returncode, 3, clock)
        r = self.dry("1 0810", STOP_AT="0800")
        self.assertEqual(r.returncode, 3)
        self.assertEqual(self.dry("1 0810", STOP_AT="0900").returncode, 0)

    def test_real_stop_is_logged_in_progress(self):
        r = self.real("1 0850", TASKS="t2-newpage", MODELS=HAIKU, SAMPLES="s1")
        self.assertEqual(r.returncode, 3, r.stdout + r.stderr)
        self.assertIn("STOPPED before " + FIRST, self.progress())
        self.assertFalse(os.path.exists(self.cell_dir()))  # no cell started

    def test_stop_check_precedes_skip(self):
        # the window is checked before every cell, a done one included: a resume inside it
        # refuses to start and names the first cell
        d = self.cell_dir()
        os.makedirs(d)
        with open(os.path.join(d, "summary.json"), "w") as f:
            f.write('{"outcome": "pass"}')
        r = self.dry("2 0900")
        self.assertEqual(r.returncode, 3)
        self.assertIn("STOPPED before " + FIRST, r.stdout)

    def test_bad_stop_at_and_clock_refused(self):
        self.assertEqual(self.dry(STOP_AT="85").returncode, 2)
        self.assertEqual(self.dry("1 85").returncode, 2)
        self.assertEqual(self.dry("x 0900").returncode, 2)

    def test_preflight_refuses_changed_input(self):
        ok = self.run_matrix(PREFLIGHT_ONLY="1")
        self.assertEqual(ok.returncode, 0, ok.stdout + ok.stderr)
        task = os.path.join(self.e, "..", "tasks", "t5-ownership.md")
        with open(task, "a") as f:
            f.write("\nedited after the gate\n")
        r = self.run_matrix(PREFLIGHT_ONLY="1")
        self.assertEqual(r.returncode, 2)
        self.assertIn("experiments/tasks/t5-ownership.md", r.stderr)
        # committed but after the pin: still refused, path named
        self.git("commit", "-q", "-am", "edit after pin")
        r = self.run_matrix(PREFLIGHT_ONLY="1")
        self.assertEqual(r.returncode, 2)
        self.assertIn("changed since", r.stderr)
        self.assertIn("experiments/tasks/t5-ownership.md", r.stderr)

    def test_preflight_refuses_untracked_input(self):
        with open(os.path.join(self.e, "hidden", "stray.alder"), "w") as f:
            f.write("x")
        r = self.run_matrix(PREFLIGHT_ONLY="1")
        self.assertEqual(r.returncode, 2)
        self.assertIn("experiments/e12-routes/hidden", r.stderr)

    def test_preflight_refuses_task_without_material(self):
        r = self.run_matrix(PREFLIGHT_ONLY="1", TASKS="t99-nope")
        self.assertEqual(r.returncode, 2)
        self.assertIn("experiments/e12-routes/tasks/t99-nope.md", r.stderr)

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
