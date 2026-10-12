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
from collections import Counter

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import extract  # noqa: E402

ALDER = os.path.join(extract.E, "alder")
META = set(".+*?()|[]{}^$\\")


def rx(s):
    return "".join("\\" + c if c in META else c for c in s)


def module_of(rel):
    return rel[:-len(".alder")].replace("/", ".")


def key(v):
    # not `before`/`after`: their text can differ for the same mutant when a round re-emits a file in another
    # spelling with the same IR (haiku/t10-teams-s2: `Db.insert Invite {…}` against the record written out)
    return (v["module"], v["decl"], v["kind"])


def copy_round(rd):
    tmp = tempfile.mkdtemp(prefix="e9attr-")
    for sub in ("app", "migrations", "adapters"):
        if os.path.isdir(os.path.join(rd, sub)):
            shutil.copytree(os.path.join(rd, sub), os.path.join(tmp, sub))
    return tmp


def mutants(tmp, pat=None):
    argv = [ALDER, "test", tmp, "--mutate", "--json"] + (["-run", pat] if pat else [])
    p = subprocess.run(argv, capture_output=True, text=True, timeout=1800)
    muts, results = [], []
    for line in p.stdout.splitlines():
        try:
            o = json.loads(line)
        except ValueError:
            continue
        if "mutants" in o:
            muts = o["mutants"]
        elif "name" in o:
            results.append(o)
    return muts, results, p


def surviving_ids(rd, summary_mut):
    """The ids of the mutants that survived a round's full suite. Mutant ids are stable across the rounds of a
    feedback cell (the app is frozen; the same mutants are enumerated, checked below), so a survivor is matched by
    id, not by line (a round that adds a test or a fake above a declaration moves its lines) nor by content (two
    mutants can have the same text)."""
    tmp = copy_round(rd)
    muts, _, _ = mutants(tmp)
    shutil.rmtree(tmp, ignore_errors=True)
    ranked = ranks(muts)
    ids = {ranked[x["id"]] for x in muts if not x.get("killed")}
    assert len(ids) == summary_mut["survived"], (rd, len(ids), summary_mut["survived"])
    return ids, Counter(ranked.values())


def ranks(muts):
    """id -> (module, decl, operator, n): the n-th such mutant in source order. Ids are not stable across rounds
    (a fed-back round can add mutable code in a fake or move declarations) and lines move, so a mutant is named by
    its content and its rank among the mutants with the same content."""
    seen = Counter()
    out = {}
    for x in sorted(muts, key=lambda x: (x["file"], x["line"], x["col"], x["id"])):
        k = key(x)
        out[x["id"]] = (k, seen[k])
        seen[k] += 1
    return out


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
        prev_surv, prev_shape = surviving_ids(os.path.join(path, f"round{n0}"), d["rounds"][n0]["mutation"])
        for r in d["rounds"]:
            if "feedback" not in r or extract.round_state(r) != "accepted":
                continue
            rd = os.path.join(path, f"round{r['n']}")
            now = extract.tests_of(os.path.join(rd, "app"))
            fresh = {k: v for k, v in now.items() if prev_tests.get(k) != v}
            tmp = copy_round(rd)
            for (rel, name) in sorted(fresh):
                mod = module_of(rel)
                pat = "^" + rx(mod) + ' "' + rx(name.replace('\\"', '"')) + '"$'
                muts, results, p = mutants(tmp, pat)
                ran = any(o.get("ok") for o in results)
                rk = ranks(muts)
                # a fed-back `fake` can add mutants (51 against 50 in haiku/t10-teams-s2 r2); the round before's
                # mutants must all still be there
                assert not (prev_shape - Counter(rk.values())), (cell, r["n"], "a mutant of the round before is gone")
                killed = [x for x in muts if x.get("killed")]
                ks = [x for x in killed if rk[x["id"]] in prev_surv]
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
            prev_surv, prev_shape = surviving_ids(rd, r["mutation"])


if __name__ == "__main__":
    main()
