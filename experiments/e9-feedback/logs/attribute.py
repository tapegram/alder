#!/usr/bin/env python3
"""E9: per-test evidence for the hand-scoring of fed-back tests (survivors.md).

For every feedback cell and every ACCEPTED feedback round, each test that is new (or changed) against the
previous accepted round is run alone under mutation on a scratch copy of that round
(`alder test --mutate --json -run '^<Module> "<name>"$'`, the round's app/, migrations/, adapters/), and the
mutants it kills are compared with the survivors of the previous accepted round (the ones fed back, or beyond
the ten the prompt shows). Output: JSON lines on stdout, one per test:
  {cell, round, feedback, module, file, name, kills, kills_survivors: [..], kills_survivors_same_module: [..],
   passes_alone, rule}
rule (the mechanical first pass; survivors.md's labels are hand-checked against the source):
  tautology      kills no mutant at all (it fails on nothing)
  real           kills a previous survivor in its own module
  wrong-module   kills previous survivors, none of them in its own module
  redundant      kills mutants, but none that had survived (a real test that does not answer the feedback)
usage: python3 experiments/e9-feedback/logs/attribute.py [cell-filter] > logs/attribution.jsonl
"""
import json, os, shutil, subprocess, sys, tempfile

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import extract  # noqa: E402

ALDER = os.path.join(extract.E, "alder")
META = set(".+*?()|[]{}^$\\")


def rx(s):
    return "".join("\\" + c if c in META else c for c in s)


def module_of(rel):
    return rel[:-len(".alder")].replace("/", ".")


def key(v):
    # by content, not position: a fed-back round that adds a fake or a test above a declaration moves its lines
    return (v["module"], v["decl"], v["kind"], v["before"], v["after"])


def main():
    filt = sys.argv[1] if len(sys.argv) > 1 else ""
    for c, m, t, s, d, path in extract.cells():
        cell = f"{c}/{m}/{t}-{s}"
        if c != "feedback" or filt not in cell:
            continue
        n0 = extract.first_pass(d)
        if n0 is None:
            continue
        prev_tests = extract.tests_of(os.path.join(path, f"round{n0}", "app"))
        prev_surv = {key(v) for v in d["rounds"][n0]["mutation"].get("survivors", [])}
        for r in d["rounds"]:
            if "feedback" not in r or extract.round_state(r) != "accepted":
                continue
            rd = os.path.join(path, f"round{r['n']}")
            now = extract.tests_of(os.path.join(rd, "app"))
            fresh = {k: v for k, v in now.items() if prev_tests.get(k) != v}
            tmp = tempfile.mkdtemp(prefix="e9attr-")
            for sub in ("app", "migrations", "adapters"):
                if os.path.isdir(os.path.join(rd, sub)):
                    shutil.copytree(os.path.join(rd, sub), os.path.join(tmp, sub))
            for (rel, name) in sorted(fresh):
                mod = module_of(rel)
                pat = "^" + rx(mod) + ' "' + rx(name.replace('\\"', '"')) + '"$'
                p = subprocess.run([ALDER, "test", tmp, "-run", pat, "--mutate", "--json"],
                                   capture_output=True, text=True, timeout=1800)
                muts, ran = [], False
                for line in p.stdout.splitlines():
                    try:
                        o = json.loads(line)
                    except ValueError:
                        continue
                    if "mutants" in o:
                        muts = o["mutants"]
                    elif o.get("name") == name.replace('\\"', '"') or o.get("module") == mod:
                        ran = ran or bool(o.get("ok"))
                killed = [x for x in muts if x.get("killed")]
                ks = [x for x in killed if key(x) in prev_surv]
                same = [x for x in ks if x["module"] == mod]
                if not muts:
                    rule = "not run: " + (p.stdout + p.stderr).strip().splitlines()[-1:][0] if (p.stdout + p.stderr).strip() else "not run"
                elif not killed:
                    rule = "tautology"
                elif same:
                    rule = "real"
                elif ks:
                    rule = "wrong-module"
                else:
                    rule = "redundant"
                fmt = lambda xs: [f"{x['module']} {x['decl']}:{x['line']} {x['kind']}" for x in xs]
                print(json.dumps({"cell": cell, "round": r["n"], "feedback": r["feedback"], "module": mod,
                                  "file": rel, "name": name, "kills": len(killed),
                                  "kills_survivors": fmt(ks), "kills_survivors_same_module": fmt(same),
                                  "passes_alone": ran, "rule": rule}, ensure_ascii=False), flush=True)
            shutil.rmtree(tmp, ignore_errors=True)
            prev_tests = now
            prev_surv = {key(v) for v in r["mutation"].get("survivors", [])}


if __name__ == "__main__":
    main()
