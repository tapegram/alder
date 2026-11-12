#!/usr/bin/env python3
"""replay_drill.py ALDER SCRIPT APP_DIR: replay a drill script (testdata/drill/<task>/script.txt).

Runs `alder new <name> --bare` in APP_DIR's parent (name = APP_DIR's last component), then each
script step in APP_DIR through run_loop.parse_script / run_loop.execute_script. Exits 1 naming the
first failed line."""
import pathlib
import sys

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parent.parent))
import run_loop  # noqa: E402


def main(argv):
    if len(argv) != 4:
        print("usage: replay_drill.py ALDER SCRIPT APP_DIR", file=sys.stderr)
        return 2
    alder, script, app_dir = pathlib.Path(argv[1]), pathlib.Path(argv[2]), pathlib.Path(argv[3])
    app_dir.parent.mkdir(parents=True, exist_ok=True)
    steps, refusal = run_loop.parse_script(script.read_text())
    if refusal:
        print(f"replay_drill: {script} line {refusal['line']}: {refusal['reason']}: {refusal['text']}", file=sys.stderr)
        return 1
    rec = run_loop.run_command(alder, ["new", app_dir.name, "--bare"], None, app_dir.parent)
    if rec["exit"] != 0:
        print(f"replay_drill: alder new {app_dir.name} --bare: {rec['stderr'] or rec['stdout']}", file=sys.stderr)
        return 1
    _, refusal, _ = run_loop.execute_script(alder, steps, app_dir, cap=10_000)
    if refusal:
        print(f"replay_drill: {script} line {refusal['line']}: {refusal['text']}: {refusal['reason']}", file=sys.stderr)
        return 1
    return 0


sys.exit(main(sys.argv))
