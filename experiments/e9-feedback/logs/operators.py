#!/usr/bin/env python3
"""Per-operator kill rates: re-runs `alder test --mutate --json` on the final (last accepted) round of every cell
(scratch copy: app/, migrations/, adapters/) and tallies mutants and kills by operator, per condition.
The harness's summaries keep only the survivors; this recovers the denominators. Output: a markdown table.
usage: python3 experiments/e9-feedback/logs/operators.py > logs/operators.md"""
import json, os, shutil, subprocess, sys, tempfile
from collections import Counter, defaultdict
from concurrent.futures import ThreadPoolExecutor
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import extract  # noqa: E402

ALDER = os.path.join(extract.E, "alder")


def final_round(d):
    acc = [r for r in d["rounds"] if r.get("mutation") is not None and not r.get("rejected_reason")]
    return acc[-1]["n"]


def run(cell):
    c, m, t, s, d, path = cell
    n = final_round(d)
    rd = os.path.join(path, f"round{n}")
    tmp = tempfile.mkdtemp(prefix="e9ops-")
    for sub in ("app", "migrations", "adapters"):
        if os.path.isdir(os.path.join(rd, sub)):
            shutil.copytree(os.path.join(rd, sub), os.path.join(tmp, sub))
    p = subprocess.run([ALDER, "test", tmp, "--mutate", "--json"], capture_output=True, text=True, timeout=1800)
    shutil.rmtree(tmp, ignore_errors=True)
    for line in p.stdout.splitlines():
        try:
            o = json.loads(line)
        except ValueError:
            continue
        if "mutants" in o:
            return c, m, t, s, n, o["mutants"]
    return c, m, t, s, n, None


def main():
    cells = list(extract.cells())
    with ThreadPoolExecutor(4) as ex:
        res = list(ex.map(run, cells))
    tot = defaultdict(Counter)
    kil = defaultdict(Counter)
    check = []
    for c, m, t, s, n, muts in res:
        d = next(dd for cc, mm, tt, ss, dd, _ in cells if (cc, mm, tt, ss) == (c, m, t, s))
        if muts is None:
            print(f"<!-- {c}/{m}/{t}-{s}: no mutants reported -->")
            continue
        k = sum(1 for x in muts if x.get("killed"))
        if k != d["mutation"]["killed"]:
            check.append(f"{c}/{m}/{t}-{s} r{n}: rerun {k} killed, summary {d['mutation']['killed']}")
        for x in muts:
            tot[c][x["kind"]] += 1
            kil[c][x["kind"]] += bool(x.get("killed"))
    ops = sorted(set(k for c in tot for k in tot[c]), key=lambda k: (extract.CLASS.get(k, "?"), k))
    print("| class | operator | baseline killed/all | feedback killed/all |")
    print("|---|---|---|---|")
    for o in ops:
        cols = []
        for c in extract.CONDS:
            a = tot[c][o]
            cols.append(f"{kil[c][o]}/{a} ({kil[c][o] / a:.2f})" if a else "—")
        print(f"| {extract.CLASS.get(o, '?')} | {o} | {cols[0]} | {cols[1]} |")
    print()
    print("Rerun agrees with every summary's `killed`." if not check else "Rerun differs: " + "; ".join(check))


if __name__ == "__main__":
    main()
