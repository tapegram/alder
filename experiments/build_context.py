#!/usr/bin/env python3
"""Build the context prompt given to models: reference chapters + example app.

Usage: build_context.py [--chapters 03,04,...] [--no-example] [--reference] [--book DIR] [--example DIR]
                        [--size [--count ollama:MODEL [--num-ctx N]]] > prompt.md

--reference drops the sections written for humans (HUMAN_SECTIONS, by exact `## ` heading),
except the `### ` subsections of a dropped section listed in KEEP_SUBSECTIONS, which stay under
the section's heading; and from a section it keeps, the `### ` subsections listed in
DROP_SUBSECTIONS (the inverse). A listed heading missing from its chapter exits 2 naming it, so
a renamed section never silently builds the wrong prompt. --size prints the prompt's size instead of the prompt: characters and
len/4; with --count ollama:MODEL also Ollama's prompt_eval_count for the prompt alone (one
/api/chat call, the prompt as the system message, num_predict 1).
"""
import argparse
import json
import pathlib
import re
import sys
import urllib.request

ROOT = pathlib.Path(__file__).resolve().parent.parent
BOOK = ROOT / "book"
EXAMPLE = ROOT / "examples" / "todos" / "app"

DEFAULT_CHAPTERS = ["03", "04", "05", "06", "07", "08", "09", "10"]

# The `## ` sections --reference drops: how the compiler and runtime work inside, schema
# migration, running the test tool — written for a human reader, not needed to write an app.
# Kept, like chapter 8's "Migrations": chapter 10's "8. Faking a capability" (an agent writes
# fakes and installs them; its rules and diagnostics are in that section) and "9. Testing through
# the browser" (the step table, the rules and the failure texts an agent's tests meet); its
# "10. How the browser works" is the inside, for a human reader.
HUMAN_SECTIONS = {
    "06": ["How a request runs"],
    "07": ["How effects run", "Implementing a capability"],
    "08": ["How tables are stored"],
    "10": ["2. Running the tests", "6. What a trace says",
           "7. Choosing tests, and output for programs", "10. How the browser works",
           "11. What cannot be tested yet"],
}

# The `### ` subsections of a dropped section that --reference keeps, by chapter and `## `
# heading: chapter 7's shape of a capability in Go (the interface and its methods: ctx first, an
# Args__ record, a1…an or nothing, result and error; the Go types; `import "app"`, rt, db.TxFrom)
# and its adapter naming rule (capability X of module A.B -> adapters/a/, func BX; a keyword
# directory), which an agent writing a capability needs; the rest of those sections is how the
# effects run and how the build wires the adapters.
KEEP_SUBSECTIONS = {
    "07": {"How effects run": ["What a capability looks like in Go"], "Implementing a capability": ["The rule"]},
}

# The `### ` subsections of a kept section that --reference drops, by chapter and `## `
# heading: the inverse of KEEP_SUBSECTIONS. Chapter 10's "9. Testing through the browser" keeps
# the step table and the result fields, one complete example (adding a todo), `Answer`, the
# session and the login through its form, how a record is sent (with the encoding table),
# `submitRaw` and `FormRejected`, the toggle and the stranger's examples, the queries and selectors,
# the list of every failure text and the rules; it drops the walkthrough of the first example
# and its failing report, the prose of `Test.get`/`Test.open`, the further form examples (the
# due date, two messages posting one form), the bad-input example, the refused click and the ownership narrative, the
# refused selector, and the comparison with a view (with a Doc's failure report and "when to use
# which"), and the case for testing through the browser at all. Chapter 9's step and query signatures repeat chapter 10's.
DROP_SUBSECTIONS = {
    "09": {"Test": ["Browser steps and queries"]},
    "10": {"9. Testing through the browser": ["Why through the browser", "Step by step", "`Test.get` and `Test.open`: requesting a page",
                                              "More forms", "Bad input, an example", "Clicks that fail", "A selector the compiler refuses",
                                              "Comparing a page with its view"]},
}

HEADER = """You are writing code in Alder, a small language for server-rendered web
apps. Alder is not Elm, not TypeScript, and not any language you have seen
before; the reference below is complete. Anything not in the reference does
not exist. Follow it exactly.

====================  ALDER REFERENCE  ====================
"""

EXAMPLE_HEADER = """
====================  EXAMPLE APP: examples/todos  ====================
A complete, correct Alder application. Match its style exactly.
"""

FENCE = re.compile(r"^\s*(```|~~~)")


def sections(text):
    """Split a chapter into [(heading or None, text)]: the part before the first `## ` line, then
    one part per `## ` heading up to the next. A `## ` line inside a code fence is not a heading.
    Joining the texts gives the chapter back."""
    out, cur, head, fenced = [], [], None, False
    for line in text.splitlines(keepends=True):
        if FENCE.match(line):
            fenced = not fenced
        if not fenced and line.startswith("## "):
            out.append((head, "".join(cur)))
            cur, head = [], line[3:].rstrip()
        cur.append(line)
    out.append((head, "".join(cur)))
    return out


def subsections(text):
    """Split a `## ` section's text into [(heading or None, text)] at its `### ` lines, outside
    code fences, as sections does at `## ` lines."""
    out, cur, head, fenced = [], [], None, False
    for line in text.splitlines(keepends=True):
        if FENCE.match(line):
            fenced = not fenced
        if not fenced and line.startswith("### "):
            out.append((head, "".join(cur)))
            cur, head = [], line[4:].rstrip()
        cur.append(line)
    out.append((head, "".join(cur)))
    return out


def check_subsections(path, h, subs, listed, table):
    """Exits 2 naming the first of listed (`### ` headings) not among subs, the `## ` section
    h's subsections; table names the dict to update."""
    names = {sh for sh, _ in subs}
    for sh in listed:
        if sh not in names:
            print(f'build_context.py: --reference: {path} has no subsection "### {sh}" in "## {h}" '
                  f"(update {table} to match the book)", file=sys.stderr)
            raise SystemExit(2)


def drop_sections(path, text, headings, keep=None, drop=None):
    """text without the `## ` sections named in headings, but for the `### ` subsections that
    keep (a dict: `## ` heading -> [`### ` headings]) lists for a dropped section, which stay
    after that section's `## ` line; and without the `### ` subsections drop (the same shape)
    lists for a kept section; exits 2 naming a heading not found."""
    keep, drop = keep or {}, drop or {}
    parts = sections(text)
    found = {h for h, _ in parts}
    for h in list(headings) + list(drop):
        if h not in found:
            print(f'build_context.py: --reference: {path} has no section "## {h}" '
                  f"(update HUMAN_SECTIONS or DROP_SUBSECTIONS to match the book)", file=sys.stderr)
            raise SystemExit(2)
    out = []
    for h, t in parts:
        if h not in headings:
            if not drop.get(h):
                out.append(t)
                continue
            subs = subsections(t)
            check_subsections(path, h, subs, drop[h], "DROP_SUBSECTIONS")
            out.extend(st for sh, st in subs if sh not in drop[h])
            continue
        if not keep.get(h):
            continue
        subs = subsections(t)
        check_subsections(path, h, subs, keep[h], "KEEP_SUBSECTIONS")
        out.append(t.splitlines(keepends=True)[0] + "\n")
        out.extend(st for sh, st in subs if sh in keep[h])
    return "".join(out)


def build(chapters, book, example, reference):
    out = [HEADER]
    for ch in chapters:
        for f in sorted(book.glob(f"{ch}-*.md")):
            text = f.read_text()
            if reference and (ch in HUMAN_SECTIONS or ch in DROP_SUBSECTIONS):
                text = drop_sections(f, text, HUMAN_SECTIONS.get(ch, []), KEEP_SUBSECTIONS.get(ch), DROP_SUBSECTIONS.get(ch))
            out.append(text)
            out.append("\n")
    if example is not None:
        out.append(EXAMPLE_HEADER)
        for f in sorted(example.rglob("*.alder")):
            rel = f.relative_to(example.parent)
            out.append(f"\n--- file: {rel} ---\n")
            out.append(f.read_text())
    return "".join(out)


def ollama_count(model, prompt, num_ctx, host="http://localhost:11434"):
    """Ollama's prompt_eval_count for prompt as the system message alone (num_predict 1)."""
    body = {"model": model, "stream": False, "options": {"num_ctx": num_ctx, "temperature": 0, "num_predict": 1},
            "messages": [{"role": "system", "content": prompt}]}
    req = urllib.request.Request(host + "/api/chat", data=json.dumps(body).encode(),
                                 headers={"Content-Type": "application/json"})
    with urllib.request.urlopen(req, timeout=3600) as r:
        return json.load(r).get("prompt_eval_count")


def main(argv=None):
    ap = argparse.ArgumentParser()
    ap.add_argument("--chapters", default=",".join(DEFAULT_CHAPTERS))
    ap.add_argument("--no-example", action="store_true")
    ap.add_argument("--reference", action="store_true", help="drop the sections written for humans (HUMAN_SECTIONS)")
    ap.add_argument("--size", action="store_true", help="print the prompt's size (characters, len/4) instead of the prompt")
    ap.add_argument("--count", metavar="ollama:MODEL", help="with the size, Ollama's prompt_eval_count for the prompt (implies --size)")
    ap.add_argument("--num-ctx", type=int, default=65536, help="num_ctx for --count (default 65536: large enough not to truncate)")
    ap.add_argument("--book", default=str(BOOK), help="the book directory (default: book/)")
    ap.add_argument("--example", default=str(EXAMPLE), help="the example's app directory (default: examples/todos/app)")
    args = ap.parse_args(argv)
    if args.count and not args.count.startswith("ollama:"):
        ap.error("--count takes ollama:MODEL")

    prompt = build(args.chapters.split(","), pathlib.Path(args.book),
                   None if args.no_example else pathlib.Path(args.example), args.reference)
    if not (args.size or args.count):
        print(prompt)
        return
    print(f"characters: {len(prompt)}")
    print(f"estimate (len/4): {len(prompt) // 4}")
    if args.count:
        model = args.count.partition(":")[2]
        try:
            n = ollama_count(model, prompt, args.num_ctx)
        except Exception as e:   # connection refused, timeout, bad JSON
            print(f"build_context.py: --count {args.count}: {e}", file=sys.stderr)
            raise SystemExit(1)
        print(f"prompt_eval_count ({args.count}, num_ctx {args.num_ctx}): {n}")
        if isinstance(n, int) and n >= args.num_ctx:
            print(f"build_context.py: warning: the count reached num_ctx {args.num_ctx}; raise --num-ctx", file=sys.stderr)


if __name__ == "__main__":
    main()
