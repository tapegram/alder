#!/usr/bin/env python3
"""Writes the generated sections of experiments/e9-feedback/survivors.md (stdout):
  1. every final survivor (cell, module, decl, line, kind, class), from the summaries;
  2. every fed-back test of an accepted feedback round, with its label and one line of evidence, from
     logs/attribution.jsonl (logs/attribute.py) and the HAND labels below;
  3. the rejected and failed feedback rounds; 4. the feedback_end tally.
The label is hand-scored: a test that fails on some mutant cannot be a tautology, so `attribute.py`'s kill
evidence settles real vs wrong-module for those; every test that kills no mutant was read, and its label and
reason are in HAND.
usage: python3 experiments/e9-feedback/logs/survivors.py > /tmp/generated.md
"""
import json, os, sys
from collections import Counter

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import extract  # noqa: E402

# (cell, round, test name) -> (label, evidence): the tests that kill no mutant, read by hand.
HAND = {
    ("feedback/claude-haiku/t6-orders-s1", 2, "a new user is staff"):
        ("real", "asserts a fresh user's `roleOf` is `Staff` (the table default); module `User` has no mutant in this round"),
    ("feedback/claude-haiku/t9-bookings-s1", 2, "a booking that ends today leaves today free"):
        ("real", "`nextFree today [ended] == today`, the `[from, to)` boundary; no mutant fails it (the boundary flip is equivalent)"),
    ("feedback/claude-haiku/t9-bookings-s1", 2, "overlapping finds a booking inside the query"):
        ("real", "`Maybe.isJust` of an inner overlap: positive-only, a dropped condition still finds it, so no mutant fails it"),
    ("feedback/claude-haiku/t9-bookings-s1", 2, "overlapping finds a booking that holds one of the days"):
        ("real", "`Maybe.isJust` of a partial overlap: positive-only, as above"),
    ("feedback/claude-haiku/t9-bookings-s1", 2, "your bookings name their room"):
        ("real", "`#mine .room` is the room's name; no mutant of the round fails it"),
    ("feedback/claude-haiku/t9-bookings-s1", 2, "an unknown booking is not found"):
        ("real", "an unknown id is `PageMissing`; no operator mutates a `Db.get` miss"),
    ("feedback/claude-haiku/t9-bookings-s1", 2, "a room's free day is today when its bookings have ended"):
        ("real", "`#rooms .free` is today after past bookings; aimed at the `to > today` where, which is equivalent in this app"),
    ("feedback/claude-haiku/t9-bookings-s1", 2, "an unknown room is not found"):
        ("real", "an unknown id is `PageMissing`; no operator mutates a `Db.get` miss"),
    ("feedback/claude-haiku/t9-bookings-s2", 1, "a booking that ends today does not hold it"):
        ("real", "the `[from, to)` boundary of `nextFree`; its boundary flip is equivalent (the model says so in a comment)"),
    ("feedback/claude-haiku/t9-bookings-s2", 1, "a room that does not exist is not found"):
        ("real", "an unknown id is `PageMissing`; no operator mutates a `Db.get` miss"),
    ("feedback/claude-haiku/t10-teams-s1", 2, "an unknown invite cannot be accepted"):
        ("real", "`update (Accept unknown)` is `Err \"not found\"`; no operator mutates a `Db.get` miss"),
    ("feedback/claude-haiku/t10-teams-s2", 2, "the login page shows the form"):
        ("real", "the login page has its heading and one form: markup, which no operator changes"),
    ("feedback/claude-haiku/t10-teams-s2", 2, "an unknown team is not found"):
        ("real", "an unknown id is `PageMissing`; no operator mutates a `Db.get` miss"),
    ("feedback/claude-sonnet/t6-orders-s1", 1, "a missing order is not found, even for a manager"):
        ("real", "an unknown id is `PageMissing` for a Manager; no operator mutates a `Db.get` miss"),
    ("feedback/claude-sonnet/t7-drafts-s1", 1, "a post lists only its own comments"):
        ("real", "`load`'s comments are the post's own; the round has no mutant on the comments query"),
    ("feedback/claude-sonnet/t7-drafts-s2", 1, "a post lists only its own comments"):
        ("real", "as in s1"),
}


def main():
    print("## 1. Final survivors (generated)\n")
    print("The last accepted round's survivors, every cell. `class` is book 10's (logs/extract.py `CLASS`).\n")
    extract.mode_survivors()

    rows = [json.loads(l) for l in open(os.path.join(extract.E, "logs", "attribution.jsonl"))]
    print("\n## 2. Fed-back tests *(hand-scored)*\n")
    print("Every test that an accepted feedback round added (or changed) against the round before it, run alone under")
    print("mutation on that round (`logs/attribute.py`). *kills*: mutants it kills alone; *survivors killed*: those that")
    print("had survived the round before (the ones fed back, or beyond the ten the prompt shows).\n")
    print("| cell | round | module | test | label | kills | evidence |")
    print("|---|---|---|---|---|---|---|")
    tally = Counter()
    sub = Counter()
    for r in rows:
        k = (r["cell"], r["round"], r["name"])
        if k in HAND:
            label, ev = HAND[k]
            sub["that kill no mutant (each read)"] += 1
        elif r["rule"] == "real":
            label, ev = "real", "kills " + "; ".join(r["kills_survivors_same_module"][:3]) + (" …" if len(r["kills_survivors_same_module"]) > 3 else "")
            sub["that kill a survivor of their own module"] += 1
        elif r["rule"] == "wrong-module":
            label, ev = "wrong-module", "kills only another module's survivor: " + "; ".join(r["kills_survivors"])
            sub["wrong-module"] += 1
        elif r["rule"] == "redundant":
            label, ev = "real", f"kills {r['kills']} mutants, none that had survived (a duplicate of an earlier test's check)"
            sub["that kill only mutants already killed"] += 1
        else:
            raise SystemExit(f"unlabelled: {k} {r['rule']}")
        tally[(r["cell"].split("/")[1], label)] += 1
        name = r["name"].replace("|", "\\|")
        print(f"| {r['cell'].split('/', 1)[1]} | r{r['round']} | {r['module']} | {name} | {label} | {r['kills']} | {ev.replace('|', '/')} |")
    print("\n**Tally** *(hand-scored)*:\n")
    print("| model | real | wrong-module | tautology |")
    print("|---|---|---|---|")
    for m in extract.MODELS:
        print(f"| {m} | {tally[(m, 'real')]} | {tally[(m, 'wrong-module')]} | {tally[(m, 'tautology')]} |")
    tot = Counter()
    for (m, l), n in tally.items():
        tot[l] += n
    print(f"| all | {tot['real']} | {tot['wrong-module']} | {tot['tautology']} |")
    print("\nOf the real ones: " + ", ".join(f"{v} {k}" for k, v in sub.items() if k != "wrong-module") + ".")

    print("\n## 3. Rejected and failed feedback rounds (generated)\n")
    print("| cell | round | feedback | state | reason |")
    print("|---|---|---|---|---|")
    for c, m, t, s, d, path in extract.cells():
        for r in d["rounds"]:
            if "feedback" not in r:
                continue
            st = extract.round_state(r)
            if st == "accepted":
                continue
            if st == "rejected":
                why = r["rejected_reason"]
            else:
                codes = ", ".join(f"{k}×{v}" for k, v in (r.get("codes") or {}).items())
                tt = r.get("tests") or {}
                why = f"check: {codes}" if r.get("errors") else f"tests {tt.get('failed')} of {tt.get('passed', 0) + tt.get('failed', 0)} failed"
            print(f"| {m}/{t}-{s} | r{r['n']} | {r['feedback']} | {st} | {why} |")

    print("\n## 4. `feedback_end` (generated)\n")
    ends = Counter(d.get("feedback_end") for c, m, t, s, d, _ in extract.cells() if c == "feedback")
    print("| feedback_end | cells |")
    print("|---|---|")
    for k, v in ends.most_common():
        print(f"| {k} | {v} |")


if __name__ == "__main__":
    main()
