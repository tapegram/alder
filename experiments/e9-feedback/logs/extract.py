#!/usr/bin/env python3
"""E9 extraction: every number in findings.md / survivors.md comes from the summaries through this script.

usage: python3 experiments/e9-feedback/logs/extract.py MODE
  cells      one row per cell: condition, model, task, sample, own, rounds, hidden, score before -> after, feedback
  survivors  every final survivor (the last accepted round's mutation): cell, module, decl, line, kind, class
  kinds      survivor classes and operators tallied by condition x model (final survivors)
  claims     equivalence claims (comment blocks) in feedback rounds
  fedback    for every feedback cell: each feedback round (accepted / rejected / failed), the tests it added
             or changed against the first passing round (with their source), and the survivors it killed
"""
import json, os, re, sys
from collections import Counter, defaultdict

E = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..")
CONDS = ["baseline", "feedback"]
MODELS = ["claude-haiku", "claude-sonnet", "claude-opus"]
TASKS = ["t5-ownership", "t6-orders", "t7-drafts", "t9-bookings", "t10-teams"]
SAMPLES = ["s1", "s2"]
CLASS = {
    "requires dropped": "authorization", "guard dropped": "authorization",
    "where dropped": "data", "orderBy reversed": "data", "limit dropped": "data", "offset dropped": "data",
    "write replaced": "data", "update field dropped": "data", "session put dropped": "data",
    "redirect → rerender": "flow", "reject → rerender": "flow", "arm swapped": "flow",
    "condition negated": "logic", "operator flipped": "logic", "bool flipped": "logic", "not dropped": "logic",
    "default changed": "schema", "cascade ↔ restrict": "schema",
}
CLASSES = ["authorization", "data", "flow", "logic", "schema"]


def cells():
    for c in CONDS:
        for m in MODELS:
            for t in TASKS:
                for s in SAMPLES:
                    p = os.path.join(E, c, "runs", m, f"{t}-{s}", "summary.json")
                    if os.path.exists(p):
                        yield c, m, t, s, json.load(open(p)), os.path.dirname(p)


def sc(x):
    return "—" if x is None else f"{x:.2f}"


def ks(mu):
    if not mu:
        return "—"
    if mu.get("error"):
        return "error"
    if mu.get("no_tests"):
        return "no tests"
    return f"{mu['killed']}/{mu['killed'] + mu['survived']}"


def first_pass(d):
    for r in d["rounds"]:
        if "feedback" not in r and r.get("mutation") is not None:
            return r["n"]
    return None


def round_state(r):
    if r.get("rejected_reason"):
        return "rejected"
    if r.get("mutation") is not None:
        return "accepted"
    return "failed"


def own(d):
    rs = [r for r in d["rounds"] if "feedback" not in r]
    last = rs[-1] if rs else None
    t = (last or {}).get("tests") or {}
    mark = "✓" if d.get("final_pass") else "✗"
    return f"{mark} {t.get('passed', 0)}/{t.get('passed', 0) + t.get('failed', 0)}", len(rs)


def mode_cells():
    print("| cond | model | task | sample | outcome | own | main rounds | hidden | killed/all | score before → after | fb rounds | fb states | feedback_end | secs |")
    print("|---|---|---|---|---|---|---|---|---|---|---|---|---|---|")
    for c, m, t, s, d, _ in cells():
        o, nmain = own(d)
        fb = [r for r in d["rounds"] if "feedback" in r]
        st = ",".join(round_state(r) for r in fb) or "—"
        secs = sum(r.get("seconds") or 0 for r in d["rounds"])
        if c == "feedback":
            score = f"{sc(d.get('score_before'))} → {sc(d.get('score_after'))}"
        else:
            score = sc((d.get("mutation") or {}).get("score"))
        print(f"| {c} | {m} | {t} | {s} | {d['outcome']} | {o} | {nmain} | {d.get('hidden_pass') or '—'} | "
              f"{ks(d.get('mutation'))} | {score} | {d.get('feedback_rounds', '—')} | {st} | {d.get('feedback_end', '—')} | {secs:.0f} |")


def mode_survivors():
    print("| cell | module | decl | line | kind | class |")
    print("|---|---|---|---|---|---|")
    for c, m, t, s, d, _ in cells():
        for v in (d.get("mutation") or {}).get("survivors", []):
            print(f"| {c}/{m}/{t}-{s} | {v['module']} | {v['decl']} | {v['line']} | {v['kind']} | {CLASS.get(v['kind'], '?')} |")


def mode_kinds():
    cls = defaultdict(Counter)
    ops = defaultdict(Counter)
    for c, m, t, s, d, _ in cells():
        for v in (d.get("mutation") or {}).get("survivors", []):
            cls[(c, m)][CLASS.get(v["kind"], "?")] += 1
            ops[c][v["kind"]] += 1
    print("| cond | model | " + " | ".join(CLASSES) + " | total |")
    print("|---|---|" + "---|" * (len(CLASSES) + 1))
    for (c, m), k in sorted(cls.items()):
        print(f"| {c} | {m} | " + " | ".join(str(k[x]) for x in CLASSES) + f" | {sum(k.values())} |")
    for c, k in ops.items():
        print(f"\n{c} by operator: " + ", ".join(f"{a} {n}" for a, n in k.most_common()))
    # survivors by task across all cells (where models leave gaps)
    bytask = defaultdict(Counter)
    for c, m, t, s, d, _ in cells():
        for v in (d.get("mutation") or {}).get("survivors", []):
            bytask[(c, t)][CLASS.get(v["kind"], "?")] += 1
    print("\n| cond | task | " + " | ".join(CLASSES) + " |")
    print("|---|---|" + "---|" * len(CLASSES))
    for (c, t), k in sorted(bytask.items()):
        print(f"| {c} | {t} | " + " | ".join(str(k[x]) for x in CLASSES) + " |")


TEST_RE = re.compile(r'^test\s+"((?:[^"\\]|\\.)*)"')


def tests_of(app):
    """{(module file, test name): source} for every top-level test in app/."""
    out = {}
    for root, _, files in os.walk(app):
        for f in files:
            if not f.endswith(".alder"):
                continue
            p = os.path.join(root, f)
            rel = os.path.relpath(p, app)
            lines = open(p).read().split("\n")
            i = 0
            while i < len(lines):
                mt = TEST_RE.match(lines[i])
                if mt:
                    j = i + 1
                    while j < len(lines) and not (lines[j] and not lines[j][0].isspace() and lines[j][0] != "}"):
                        j += 1
                    out[(rel, mt.group(1))] = "\n".join(lines[i:j]).rstrip()
                    i = j
                else:
                    i += 1
    return out


def skey(v):
    return (v["module"], v["decl"], v["kind"], v["before"], v["after"])


def mode_fedback():
    for c, m, t, s, d, path in cells():
        if c != "feedback":
            continue
        fb = [r for r in d["rounds"] if "feedback" in r]
        print(f"\n## {m}/{t}-{s}: score {sc(d.get('score_before'))} → {sc(d.get('score_after'))}; "
              f"feedback_rounds {d.get('feedback_rounds')}; end: {d.get('feedback_end')}")
        n0 = first_pass(d)
        if n0 is None or not fb:
            continue
        base_round = d["rounds"][n0]
        base_tests = tests_of(os.path.join(path, f"round{n0}", "app"))
        prev_surv = {skey(v): v for v in base_round["mutation"].get("survivors", [])}
        print(f"first passing round r{n0}: {ks(base_round['mutation'])}, {len(base_tests)} tests")
        accepted_tests = base_tests
        for r in fb:
            st = round_state(r)
            app = os.path.join(path, f"round{r['n']}", "app")
            now = tests_of(app) if os.path.isdir(app) else {}
            new = {k: v for k, v in now.items() if k not in accepted_tests}
            changed = {k: v for k, v in now.items() if k in accepted_tests and accepted_tests[k] != v}
            gone = [k for k in accepted_tests if k not in now]
            extra = ""
            if st == "rejected":
                extra = f" — {r['rejected_reason']}"
            elif st == "failed":
                tt = r.get("tests") or {}
                extra = (f" — errors {r.get('errors')}, build {(r.get('build') or {}).get('ok')}, "
                         f"tests {tt.get('passed')}/{(tt.get('passed') or 0) + (tt.get('failed') or 0)}")
            killed = []
            if st == "accepted":
                cur = {skey(v) for v in r["mutation"].get("survivors", [])}
                killed = [v for k, v in prev_surv.items() if k not in cur]
                extra = f" — {ks(r['mutation'])} score {sc(r['mutation'].get('score'))}"
            print(f"\n### r{r['n']} (feedback {r['feedback']}): {st}{extra}")
            print(f"new tests {len(new)}, changed {len(changed)}, removed {len(gone)}")
            for v in killed:
                print(f"  KILLED {v['module']} {v['decl']}:{v['line']} {v['kind']}  {v['before']} → {v['after']}")
            if st == "accepted":
                for v in r["mutation"].get("survivors", []):
                    print(f"  STILL  {v['module']} {v['decl']}:{v['line']} {v['kind']}  {v['before']} → {v['after']}")
            for (f, name), src in list(new.items()) + list(changed.items()):
                tag = "NEW" if (f, name) in new else "CHANGED"
                print(f"\n{tag} {f} :: {name}\n```alder\n{src}\n```")
            if st == "accepted":
                accepted_tests = now
                prev_surv = {skey(v): v for v in r["mutation"].get("survivors", [])}


CLAIM_RE = re.compile(r"equivalent|no input|cannot be (told|distinguish)|can't be (told|distinguish)|"
                      r"indistinguishable|not observable|unobservable|no test can|tell (them|the two) apart|same program", re.I)


def mode_claims():
    """Equivalence claims: `--` comment blocks in feedback rounds' answers that match CLAIM_RE."""
    for c, m, t, s, d, path in cells():
        if c != "feedback":
            continue
        for r in d["rounds"]:
            if "feedback" not in r:
                continue
            p = os.path.join(path, f"round{r['n']}", "output.md")
            if not os.path.exists(p):
                continue
            lines = open(p).read().split("\n")
            i = 0
            while i < len(lines):
                if lines[i].lstrip().startswith("--") and not lines[i].lstrip().startswith("--- file"):
                    j = i
                    while j < len(lines) and lines[j].lstrip().startswith("--") and not lines[j].lstrip().startswith("--- file"):
                        j += 1
                    block = "\n".join(lines[i:j])
                    if CLAIM_RE.search(block):
                        print(f"\n## {m}/{t}-{s} r{r['n']} ({round_state(r)}) output.md:{i + 1}\n{block}")
                    i = j
                else:
                    i += 1


if __name__ == "__main__":
    {"claims": mode_claims, "cells": mode_cells, "survivors": mode_survivors, "kinds": mode_kinds, "fedback": mode_fedback}[sys.argv[1]]()
