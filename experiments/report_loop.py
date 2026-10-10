#!/usr/bin/env python3
"""Aggregate run_loop summaries into a Markdown table.  Usage: report_loop.py EXP_DIR

Summaries written since M3d carry `harness`; they get one column per round (`r0 … rN`, each
`errors` or `errors (build ✓|✗)`) and a `Result` of pass / fail / compiler-bug / harness-error
(the last from `outcome`, which summaries carry since M4c). Older summaries
(E5) without `harness` keep the original two-round table, so their scores.md regenerates unchanged."""
import json, pathlib, sys


def rows(summaries):
    out = []
    for s in summaries:
        r = {"model": s["model"], "task": s["task"], "error": s["error"], "pass": s["final_pass"],
             "r0": None, "r1": None, "codes0": {}, "codes1": {},
             "harness": "harness" in s, "compiler_bug": bool(s.get("compiler_bug")),
             "outcome": s.get("outcome"), "rounds": []}
        for rd in s["rounds"]:
            r[f"r{rd['n']}"] = rd["errors"]
            r[f"codes{rd['n']}"] = rd["codes"]
            cell = str(rd["errors"])
            if "build" in rd:
                cell += " (build ✓)" if rd["build"]["ok"] else " (build ✗)"
            r["rounds"].append((cell, rd["codes"]))
        out.append(r)
    return out


def fmt_codes(c):
    return ", ".join(f"{k}×{v}" if v > 1 else k for k, v in sorted(c.items())) or "—"


def result(r):
    if r.get("outcome") == "harness_error":   # summaries since M4c carry `outcome`; older ones do not
        return "harness-error"
    return "compiler-bug" if r["compiler_bug"] else "pass" if r["pass"] else "fail"


def markdown(rs):
    if any(r.get("harness") for r in rs):
        return markdown_rounds(rs)
    lines = ["| Model | Task | Round 0 errors | Round 1 errors | Result | Round 0 codes | Round 1 codes |",
             "|---|---|---|---|---|---|---|"]
    for r in rs:
        if r["error"]:
            lines.append(f"| {r['model']} | {r['task']} | — | — | ERROR: {r['error'][:60]} | | |")
            continue
        r1 = "—" if r["r1"] is None else r["r1"]
        lines.append(f"| {r['model']} | {r['task']} | {r['r0']} | {r1} | {'pass' if r['pass'] else 'fail'} | {fmt_codes(r['codes0'])} | {fmt_codes(r['codes1'])} |")
    return "\n".join(lines) + "\n"


def markdown_rounds(rs):
    n = max([len(r["rounds"]) for r in rs] + [1])
    cols = [f"r{i}" for i in range(n)]
    lines = ["| Model | Task | " + " | ".join(cols) + " | Result | Codes |",
             "|---|---|" + "---|" * n + "---|---|"]
    for r in rs:
        if r["error"]:
            lines.append(f"| {r['model']} | {r['task']} | " + " | ".join("—" for _ in cols) + f" | ERROR: {r['error'][:60]} | |")
            continue
        cells = [c for c, _ in r["rounds"]] + ["—"] * (n - len(r["rounds"]))
        codes = "; ".join(f"r{i}: {fmt_codes(c)}" for i, (_, c) in enumerate(r["rounds"]) if c) or "—"
        lines.append(f"| {r['model']} | {r['task']} | " + " | ".join(cells) + f" | {result(r)} | {codes} |")
    return "\n".join(lines) + "\n"


def main():
    exp = pathlib.Path(sys.argv[1])
    summaries = [json.loads(p.read_text()) for p in sorted(exp.glob("runs/*/*/summary.json"))]
    print(markdown(rows(summaries)), end="")


if __name__ == "__main__":
    main()
