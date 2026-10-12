#!/usr/bin/env python3
"""E8 data extraction from the summaries (no hand edits):
  extract.py failures   -> one block per failed hidden test (cond, model, task, sample, name, reason/left/right/trace)
  extract.py migrate    -> per T7/T8 round: migrate file, steps, drops_data
  extract.py cells      -> one line per cell: cond model task sample outcome rounds own-tests hidden
  extract.py tests      -> own tests per cell: count in the final round, and names (for H2)
  extract.py newtests   -> per cell: tests in the final round's app/, how many of them the base app does not have
                           (T4-T6: all), and how many base tests are gone (for H2)
  extract.py basetests  -> T7/T8: of the base app's tests, how many the final round kept with a different body,
                           and how many it removed (for the H2 confounds)
Run from the repository root or anywhere; reads experiments/e8-hidden/{with,without}/runs/*/*/summary.json."""
import json, re, sys, pathlib
TEST = re.compile(r'^test "((?:[^"\\\\]|\\\\.)*)"', re.M)
E = pathlib.Path(__file__).resolve().parent.parent

def cells():
    for cond in ("with", "without"):
        for p in sorted((E / cond / "runs").glob("*/*/summary.json")):
            s = json.loads(p.read_text())
            yield cond, s["model"], s["task"], s["harness"].get("sample", ""), s, p.parent

def final_tests(s):
    for r in reversed(s.get("rounds", [])):
        if r.get("tests"):
            return r["tests"]
    return None

mode = sys.argv[1] if len(sys.argv) > 1 else "cells"
for cond, model, task, sample, s, d in cells():
    h = s.get("hidden") or {}
    if mode == "cells":
        t = final_tests(s)
        own = f"{t['passed']}/{t['passed']+t['failed']}" if t else "-"
        print(cond, model, task, sample, s["outcome"], len(s.get("rounds", [])), own, s.get("hidden_pass"), h.get("skipped", ""), sep="\t")
    elif mode == "failures":
        for r in h.get("results", []):
            if not r.get("ok"):
                print(f"=== {cond} {model} {task} {sample} :: {r['name']}")
                for k in ("reason", "error", "left", "right"):
                    if r.get(k):
                        print(f"  {k}: {r[k]}")
                if r.get("trace"):
                    print("  trace: " + " | ".join(r["trace"]))
    elif mode == "migrate":
        if task not in ("t7-drafts", "t8-notify"):
            continue
        for r in s.get("rounds", []):
            m = r.get("migrate")
            print(cond, model, task, sample, f"r{r['n']}", json.dumps(m) if m else "-", sep="\t")
    elif mode == "newtests":
        # tests in the final round's app/ whose name the base app does not have (T4-T6: all tests)
        rs = [r for r in s.get("rounds", [])]
        if not rs:
            continue
        fin = d / f"round{rs[-1]['n']}" / "app"
        base = {"t7-drafts": E / "base/blog-nodrafts/app", "t8-notify": E.parent.parent / "examples/blog/app"}.get(task)
        names = lambda root: set(m for f in root.rglob("*.alder") for m in TEST.findall(f.read_text())) if root and root.exists() else set()
        mine, theirs = names(fin), names(base)
        print(cond, model, task, sample, len(mine), len(mine - theirs), len(theirs - mine), sep="\t")
    elif mode == "basetests":
        # T7/T8: of the base app's tests, how many the final round kept with a different body, and how many it removed
        base = {"t7-drafts": E / "base/blog-nodrafts/app", "t8-notify": E.parent.parent / "examples/blog/app"}.get(task)
        if not base or not s.get("rounds"):
            continue
        def bodies(root):
            out = {}
            for f in root.rglob("*.alder"):
                for m in re.finditer(r'^test "((?:[^"\\]|\\.)*)"(.*?)(?=^test "|^[a-zA-Z]|\Z)', f.read_text(), re.M | re.S):
                    out[m.group(1)] = m.group(2).strip()
            return out
        b = bodies(base); m = bodies(d / f"round{s['rounds'][-1]['n']}" / "app")
        print(cond, model, task, sample, len(b), sum(1 for n in b if n in m and m[n] != b[n]), sum(1 for n in b if n not in m), sep="\t")
    elif mode == "tests":
        t = final_tests(s)
        names = [x["name"] for x in (t or {}).get("results", [])]
        print(cond, model, task, sample, len(names), "; ".join(names)[:300], sep="\t")
