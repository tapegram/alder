#!/usr/bin/env python3
"""Build the context prompt given to models: reference chapters + example app.

Usage: build_context.py [--chapters 03,04,...] [--no-example] [--reference] [--book DIR] [--example DIR]
                        [--size [--count ollama:MODEL [--num-ctx N]]] > prompt.md

--reference drops the sections written for humans (HUMAN_SECTIONS, by exact `## ` heading),
except the `### ` subsections of a dropped section listed in KEEP_SUBSECTIONS, which stay under
the section's heading; and from a section it keeps, the `### ` subsections listed in
DROP_SUBSECTIONS (the inverse); it shortens the spans TRIMS lists, and renders an `error=` block of
a whole module as its error text alone. A listed heading or span missing from its chapter exits 2
naming it, so a renamed section never silently builds the wrong prompt. --size prints the prompt's size instead of the prompt: characters and
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
           "12. What cannot be tested yet"],
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
# due date, two messages posting one form), the bad-input example, the refused click, the
# refused selector, and the comparison with a view (with a Doc's failure report and "when to use
# which"), and the case for testing through the browser at all. Chapter 9's step and query signatures repeat chapter 10's.
# Chapter 10's "11. Mutation testing" keeps the command and the score and survivor lines of the
# to-do app's run before its new tests, the operators, the rule with its two worked tests (and the
# score line after them), the three sentences on survivors that are not gaps and `--min` in one
# sentence (at most 6.6k characters in all: test_build_context checks); it drops why a mutation
# score (the case against line coverage), the reports of the rules' mutants (M11), all fourteen
# survivors with the countOpen story and the
# run after, what is never a mutant (the suppression rules: an agent sees only their counts), the
# logout redirect and the equivalent mutants in detail, the exact score with its transcript,
# `-run`/`-timeout`/`--json` (as section 7 is dropped) and how ids and lines change, all for a
# human reader. Chapter 6's "Rules" (M11) stays whole for an agent (`@owner`, the five scoped
# operations, the not-found answer, where they may be used, whose session, roles, the unscoped
# forms) but for its last subsection, the case for the design on a replicated database.
# Chapter 8's "Migrations" drops "When the app starts" (M11 final review, the reference cut):
# how the runtime applies the files and what it refuses, the inside, like "How tables are
# stored"; the workflow an agent follows (E0019, `alder migrate`, the files) stays.
DROP_SUBSECTIONS = {
    "06": {"Rules": ["The rules and a replicated database"]},
    "08": {"Migrations": ["When the app starts"]},
    "09": {"Test": ["Browser steps and queries"]},
    "10": {"11. Mutation testing": ["Why a mutation score", "The rules' mutants in a report", "How the two tests kill them", "All fourteen, and one killed by luck", "What is never a mutant",
                                    "The logout redirect", "Mutants that are the same program", "The exact score",
                                    "`-run`, `-timeout` and `--json`", "Ids and lines change with the code"],
           "9. Testing through the browser": ["Why through the browser", "Step by step", "`Test.get` and `Test.open`: requesting a page",
                                              "More forms", "Bad input, an example", "Clicks that fail", "A selector the compiler refuses",
                                              "Comparing a page with its view"]},
}

# Spans of text --reference shortens in a chapter (before dropping sections), by chapter: (start, end,
# replacement), the text from start up to (not including) end replaced by replacement; each start
# must occur once in the chapter and end after it, or the tool exits 2 naming it. They shorten
# what an agent needs in fewer words (final review of M11, the reference cut): chapter 6 points
# at the example app instead of quoting its load and update; chapter 8's "Your own rows" points at
# chapter 6 (chapter 9 lists the signatures); chapter 10's "Tests that use the rules" keeps the
# facts about whose session a test runs with and the failure line, without its transcript and
# its worked test of `Db.mine`, and says where an effect test runs in one paragraph; "A
# stranger's todo" keeps one test, the stale page's; chapter 6's re-read idiom keeps its helper
# and says how `load` uses it. Chapter 5's "Custom elements" (M7a) keeps its rules in one
# paragraph, and the input elements in another, without the manifest's story and the examples;
# its click-button paragraph keeps the fact, not the markup.
TRIMS = {
    "05": [("A `<button click={…}>` works with and without JavaScript.", "`data-*` and `aria-*` attributes take `Text`.",
            "A `<button click={…}>` works with and without JavaScript (chapter 6); a\n`click` on any other element needs JavaScript.\n\n"),
           ("## Custom elements\n", "## Partial updates",
            "## Custom elements\n\nA tag with a hyphen is a custom element, declared by the built-in\n"
            "`alder-ui` manifest or the app's `elements/custom-elements.json`\n"
            "(which wins over it). E0028: an undeclared tag, attribute or `slot=\"x\"`,\n"
            "or a quoted value on a boolean attribute. Always accepted: `id`, `class`, `style`, `slot`, `hidden`,\n`title`, `lang`, `dir`, "
            "`tabindex`, `role`, `data-*`, `aria-*`, `click`,\n`submit`, `href`.\n\n"
            "In a `<form submit>`, `<al-input field=\"f\" />` is `<Input>` (E0304 rules),\n`al-textarea` `<Textarea>`, "
            "`al-select` `<Select>`, `al-checkbox` `<Input>`\non a `Bool`; each renders its own error list.\n\n")],
    "06": [("and, in `Admin.Edit`,\n\n```alder fragment\nload params session = {\n  let author <- User.isAuthor", "Every message of the page runs `load` first",
            "which each admin page's `load` calls first, answering `Query.notFound`\nwhen it is `False`. "),
           ("A row `load` already found with `Db.getMine` is not one of the cases:", "### The rules and a replicated database",
            "A row `load` already found with `Db.getMine` is not one of the cases:\nwrite the scoped form; it costs "
            "nothing (`Todos.Show` writes\n`Db.updateMine Todo model.todo.id …`, and the mutation tool counts it\nas "
            "the same program as the unscoped one there).\n\n"),
           ("The to-do list's page uses four of them\n", "Why `update` needs them as much as `load` does",
            "The to-do list's page uses four of them, and `Todos.Show` reads its todo with the fifth\n"
            "(`examples/todos/app/Todos/Index.alder` and `Todos/Show.alder`, in the example app below).\n\n")],
    "08": [("A table with an `@owner` column (above) also has five **scoped**\n", "## How tables are stored",
            "A table with an `@owner` column (above) also has five **scoped**\noperations, `Db.mine`, "
            "`Db.getMine`, `Db.updateMine`, `Db.deleteMine` and\n`Db.insertMine`, for the pages of a signed-in "
            "user: chapter 6, \"Rules\",\nexplains them, and chapter 9 lists their types.\n\n")],
    "10": [("Without a signed-in user a scoped step fails, and says why.", "**Where an effect test runs.**",
            "Without a signed-in user a scoped step fails, saying so (`Err\n\"Db.updateMine Todo: no signed-in user\"`); "
            "it never answers as if the user\nhad no rows. A test may use the scoped operations itself too, after\n"
            "`Test.login`.\n\n"),
           ("**Where an effect test runs.** Every effect test gets a world of its own:", "A pure test (section 1) touches none of this",
            "**Where an effect test runs.** Each effect test gets a fresh, empty\nin-memory SQLite database with the "
            "app's tables; its block runs as one\n`Task`, in one transaction, and sees its own writes. `Time.now` and "
            "the\n`@created` a `Db.insert` fills read a clock that starts at\n`2026-01-01T00:00:00Z` and advances one "
            "second per read, so the rows a\ntest creates are ordered. The app's capabilities are wired to their\n"
            "adapters as in `alder build`; one without an adapter is a warning, and\ncalling it unfaked fails the test "
            "(\"capability Auth.Login.Password is\nnot wired in tests\"); `Test.fake` replaces one (section 8). The "
            "test's\nsession starts as `App`'s, every field at its default, and its browser\n(section 9) holds no page "
            "and no cookie.\n\n"),
           ("**Who may toggle a todo.** Chapter 6's rule", "My list is open in the browser;",
            "**Who may toggle a todo.** Chapter 6's rule, \"Rules\", says that a user\nwho posts `Toggle` with "
            "someone else's todo id gets a 404. A stranger's\nown list offers no button for my todo, but a page can "
            "be out of date. "),
           ("\"Delete by another user is not\nfound\" does the same with the Delete button.", "### Reading the page: queries",
            "\"Delete by another user is not\nfound\" does the same with the Delete button.\n\n")],
}

# A `alder error=CODE "text"` block of a whole module (written to show the compiler refusing it)
# is rendered by --reference as the error alone, `error[CODE]: text`, in a text block: the
# prose around it says what the module does wrong.
ERROR_MODULE = re.compile(r'^```alder error=(E\d+) "([^\n]*)"\nmodule [^\n]*\n.*?^```\n', re.S | re.M)


def error_text(m):
    return "```text\nerror[" + m.group(1) + "]: " + m.group(2) + "\n```\n"


def trim(path, text, spans):
    """text with each (start, end, replacement) of spans applied; exits 2 naming a span whose start
    is not found exactly once or whose end does not follow it."""
    for start, end, repl in spans:
        i = text.find(start)
        j = text.find(end, i + len(start)) if i >= 0 else -1
        if i < 0 or text.count(start) != 1 or j < 0:
            print(f'build_context.py: --reference: {path} has no single span from {start!r} to {end!r} '
                  f"(update TRIMS to match the book)", file=sys.stderr)
            raise SystemExit(2)
        text = text[:i] + repl + text[j:]
    return text


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
            if reference:
                text = trim(f, text, TRIMS.get(ch, []))
            if reference and (ch in HUMAN_SECTIONS or ch in DROP_SUBSECTIONS):
                text = drop_sections(f, text, HUMAN_SECTIONS.get(ch, []), KEEP_SUBSECTIONS.get(ch), DROP_SUBSECTIONS.get(ch))
            if reference:
                text = ERROR_MODULE.sub(error_text, text)
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
