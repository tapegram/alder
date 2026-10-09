#!/usr/bin/env python3
"""Aggregate run_loop summaries into a Markdown table.  Usage: report_loop.py EXP_DIR"""
import json, pathlib, sys


def rows(summaries):
    out = []
    for s in summaries:
        r = {"model": s["model"], "task": s["task"], "error": s["error"], "pass": s["final_pass"],
             "r0": None, "r1": None, "codes0": {}, "codes1": {}}
        for rd in s["rounds"]:
            r[f"r{rd['n']}"] = rd["errors"]
            r[f"codes{rd['n']}"] = rd["codes"]
        out.append(r)
    return out


def fmt_codes(c):
    return ", ".join(f"{k}×{v}" if v > 1 else k for k, v in sorted(c.items())) or "—"


def markdown(rs):
    lines = ["| Model | Task | Round 0 errors | Round 1 errors | Result | Round 0 codes | Round 1 codes |",
             "|---|---|---|---|---|---|---|"]
    for r in rs:
        if r["error"]:
            lines.append(f"| {r['model']} | {r['task']} | — | — | ERROR: {r['error'][:60]} | | |")
            continue
        r1 = "—" if r["r1"] is None else r["r1"]
        lines.append(f"| {r['model']} | {r['task']} | {r['r0']} | {r1} | {'pass' if r['pass'] else 'fail'} | {fmt_codes(r['codes0'])} | {fmt_codes(r['codes1'])} |")
    return "\n".join(lines) + "\n"


def main():
    exp = pathlib.Path(sys.argv[1])
    summaries = [json.loads(p.read_text()) for p in sorted(exp.glob("runs/*/*/summary.json"))]
    print(markdown(rows(summaries)), end="")


if __name__ == "__main__":
    main()
