#!/usr/bin/env python3
"""Aggregate run_loop summaries into a Markdown table.  Usage: report_loop.py EXP_DIR [EXP_DIR ...]

Summaries written since M3d carry `harness`; they get one column per round (`r0 … rN`, each
`errors` or `errors (build ✓|✗)`) and a `Result` of pass / fail / compiler-bug / harness-error
(the last from `outcome`, which summaries carry since M4c; compiler-bug says "(hang)" or "(check
panic)" from `compiler_bug_kind`; kind "test", `alder test` exiting 2, is plain compiler-bug as
"build" is). When any summary has rounds run with `--test`, a `Tests` column follows the rounds:
per round that ran tests, `rN ✓ N` (all N passed) or `rN ✗ M/N` (M of N failed). Older summaries (E5) without `harness` keep the original
two-round table, so their scores.md regenerates unchanged.

E8: a `Hidden` column (before `Result`) when any summary has `hidden` (run with `--hidden`): `k/n`
hidden tests passed, with "(check: CODES)" when the hidden module did not compile against the
program, or `— (skipped: reason)`; a `Condition` column (first) when any summary has
`harness.condition`, rows then grouped by it (stable: within a condition, the order read);
`harness.sample` is folded into the task as the run directory shows it (`t5-ownership-s1`).
Without those fields the table is as before. Several EXP_DIRs make one table.

M9: a `Mut` column (after `Hidden`, before `Result`) when any summary has `mutation` (run with
`--mutate`): `k/n` mutants killed of those that ran, `0 (no tests)` when the round had no tests,
or `— (error)` when the mutation run failed;
`—` for a row without one. Without it the table is as before (E5–E8 regenerate unchanged)."""
import json, pathlib, sys


def rows(summaries):
    out = []
    for s in summaries:
        h = s.get("harness") or {}
        task = f"{s['task']}-{h['sample']}" if h.get("sample") else s["task"]
        r = {"model": s["model"], "task": task, "error": s["error"], "pass": s["final_pass"],
             "condition": h.get("condition"), "hidden": hidden_cell(s["hidden"]) if "hidden" in s else None,
             "mutation": mutation_cell(s["mutation"]) if "mutation" in s else None,
             "r0": None, "r1": None, "codes0": {}, "codes1": {},
             "harness": "harness" in s, "compiler_bug": bool(s.get("compiler_bug")),
             "outcome": s.get("outcome"), "bug_kind": s.get("compiler_bug_kind"), "rounds": [], "tests": []}
        for rd in s["rounds"]:
            r[f"r{rd['n']}"] = rd["errors"]
            r[f"codes{rd['n']}"] = rd["codes"]
            cell = str(rd["errors"])
            if "build" in rd:
                cell += " (build ✓)" if rd["build"]["ok"] else " (build ✗)"
            r["rounds"].append((cell, rd["codes"]))
            if "tests" in rd:
                t = rd["tests"]
                total = t["passed"] + t["failed"]
                r["tests"].append(f"r{rd['n']} ✓ {total}" if t["failed"] == 0 else f"r{rd['n']} ✗ {t['failed']}/{total}")
        out.append(r)
    return out


def hidden_cell(h):
    if "skipped" in h:
        return f"— (skipped: {cell_text(h['skipped'])})"
    out = f"{h['passed']}/{h['passed'] + h['failed']}"
    if h.get("check", {}).get("codes"):   # the hidden module did not compile against the program
        out += f" (check: {fmt_codes(h['check']['codes'])})"
    elif h.get("reason"):
        out += f" ({cell_text(h['reason'])[:60]})"
    return out


def mutation_cell(m):
    if "error" in m:
        return "— (error)"
    if m.get("no_tests"):
        return "0 (no tests)"
    return f"{m['killed']}/{m['killed'] + m['survived']}"


def cell_text(t):
    return str(t).replace("|", "\\|").replace("\n", " ")


def fmt_codes(c):
    return ", ".join(f"{k}×{v}" if v > 1 else k for k, v in sorted(c.items())) or "—"


def result(r):
    if r.get("outcome") == "harness_error":   # summaries since M4c carry `outcome`; older ones do not
        return "harness-error"
    if r["compiler_bug"]:   # a hang or a checker panic says so; a build exit 2 is plain compiler-bug
        kind = {"hang": " (hang)", "internal_error": " (check panic)"}.get(r.get("bug_kind"), "")
        return "compiler-bug" + kind
    return "pass" if r["pass"] else "fail"


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
    tests = any(r.get("tests") for r in rs)   # a Tests column only for runs with --test: E6 regenerates unchanged
    hidden = any(r.get("hidden") is not None for r in rs)   # E8 --hidden; absent before: E5–E7 unchanged
    mut = any(r.get("mutation") is not None for r in rs)   # M9 --mutate; absent before: E5–E8 unchanged
    cond = any(r.get("condition") is not None for r in rs)
    if cond:   # grouped by condition, in the order each condition first appears
        order = {}
        for r in rs:
            order.setdefault(r.get("condition"), len(order))
        rs = sorted(rs, key=lambda r: order[r.get("condition")])
    lines = ["| " + ("Condition | " if cond else "") + "Model | Task | " + " | ".join(cols) + (" | Tests" if tests else "")
             + (" | Hidden" if hidden else "") + (" | Mut" if mut else "") + " | Result | Codes |",
             "|" + ("---|" if cond else "") + "---|---|" + "---|" * n + ("---|" if tests else "") + ("---|" if hidden else "")
             + ("---|" if mut else "") + "---|---|"]
    for r in rs:
        lead = (f"| {r.get('condition') or '—'} " if cond else "") + f"| {r['model']} | {r['task']} | "
        if r["error"]:
            lines.append(lead + " | ".join("—" for _ in cols) + (" | —" if tests else "") + (" | " + (r.get("hidden") or "—") if hidden else "")
                         + (" | " + (r.get("mutation") or "—") if mut else "") + f" | ERROR: {r['error'][:60]} | |")
            continue
        cells = [c for c, _ in r["rounds"]] + ["—"] * (n - len(r["rounds"]))
        codes = "; ".join(f"r{i}: {fmt_codes(c)}" for i, (_, c) in enumerate(r["rounds"]) if c) or "—"
        tcell = (" | " + ("; ".join(r["tests"]) or "—")) if tests else ""
        hcell = (" | " + (r.get("hidden") or "—")) if hidden else ""
        mcell = (" | " + (r.get("mutation") or "—")) if mut else ""
        lines.append(lead + " | ".join(cells) + tcell + hcell + mcell + f" | {result(r)} | {codes} |")
    return "\n".join(lines) + "\n"


def main():
    if len(sys.argv) < 2:
        raise SystemExit("usage: report_loop.py EXP_DIR [EXP_DIR ...]")
    summaries = [json.loads(p.read_text()) for exp in map(pathlib.Path, sys.argv[1:])
                 for p in sorted(exp.glob("runs/*/*/summary.json"))]
    print(markdown(rows(summaries)), end="")


if __name__ == "__main__":
    main()
