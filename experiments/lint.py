#!/usr/bin/env python3
"""Throwaway heuristic linter for scoring model outputs. Not a parser.

Usage: lint.py <output.md> [...]
Prints one line per finding: file:line CODE message
Then a summary count per code.
"""
import collections
import pathlib
import re
import sys

ROOT = pathlib.Path(__file__).resolve().parent.parent
BOOK = ROOT / "book"

# --- library whitelist: every `Module.name` mentioned in the reference ---
ref_text = "".join(
    f.read_text() for f in sorted(BOOK.glob("0[3-9]-*.md")) + [BOOK / "10-testing.md"]
)
KNOWN = set(re.findall(r"\b([A-Z][A-Za-z]*\.[a-zA-Z][A-Za-z]*)\b", ref_text))
STD_MODULES = {
    "List", "Maybe", "Result", "Text", "Int", "Float", "Bool", "Dict", "Date",
    "Time", "Html", "Url", "Query", "Task", "Db", "Session", "Log", "Random",
    "Http", "Form",
}

FOREIGN = [
    (r"\bcase\b.*\bof\b", "SYN", "`case ... of` (Elm) instead of `match`"),
    (r"^\s*let\b.*\bin\s*$|\blet\b[^<\n]*\bin\b", "SYN", "`let ... in`"),
    (r"\bfn\b|\bfunction\b|\bfunc\b", "SYN", "`fn`/`function` keyword"),
    (r";\s*$", "SYN", "semicolon"),
    (r"\{\s*\w+\s*\|\s*\w+\s*=", "SYN", "`=` in record update (use `:`)"),
    (r"^\s*(let\s+)?\w+\s*=\s*\{\s*\w+\s*=", "SYN", "`=` in record literal (use `:`)"),
    (r"\bonclick\b|\bonsubmit\b|\bonChange\b|\bonClick\b", "HTML", "DOM event attribute"),
    (r'href="', "ROUTE", "href with a string literal"),
    (r"href=\{\"", "ROUTE", "href with a string expression"),
    (r"\bString\.", "LIB", "`String.` module (it is `Text`)"),
    (r"\bTask\.andThen\b|\bQuery\.andThen\b", "LIB", "andThen on effects"),
    (r"\bList\.sortWith\b|\bList\.sort\b(?!By)", "LIB", "List.sort/sortWith"),
    (r"\bTodo\.Todo\b|\bUser\.User\b|\bPost\.Post\b", "STRUCT", "Module.Type for same-named type"),
    (r"\bexposing\b|\bexport\b", "STRUCT", "exposing/export"),
    (r"<input\b", "HTML", "plain <input> (check: inside form submit?)"),
    (r"\(\s*[^()]+,\s*[^()]+\s*\)\s*->", "TYP", "tuple-like"),
    (r"\bmethod=\"?post|action=", "HTML", "form method/action attribute"),
    (r"\bupdate\s*:\s*Msg\s*->\s*Model\s*->\s*Task\b", "ROUTE", "update without Session param"),
    (r"\{\s*[a-z]\w*\s*(,\s*[a-z]\w*\s*)*\}(?!\s*->)", "SYN", "record punning `{ x }`"),
]


def lint(path: pathlib.Path):
    findings = []
    lines = path.read_text().splitlines()
    in_html = False
    prev_sig = None
    for i, line in enumerate(lines, 1):
        for pat, code, msg in FOREIGN:
            if re.search(pat, line):
                findings.append((i, code, msg, line.strip()))
        for m in re.finditer(r"\b([A-Z][A-Za-z]*)\.([a-z][A-Za-z]*)\b", line):
            mod, name = m.group(1), m.group(2)
            full = f"{mod}.{name}"
            if mod in STD_MODULES and full not in KNOWN:
                # column refs like Todo.owner.eq are app-defined; only std modules checked
                findings.append((i, "LIB", f"unknown `{full}`", line.strip()))
        # annotation check: top-level `name args = ...` must follow `name : ...`
        m = re.match(r"^([a-z]\w*)(\s+\w+)*\s*=", line)
        if m and not line.startswith(" "):
            name = m.group(1)
            if name in ("login", "type", "form", "msg", "page", "table", "capability",
                        "session", "test", "module", "import", "let"):
                pass
            elif prev_sig != name:
                findings.append((i, "ANN", f"`{name}` defined without signature", line.strip()))
        m = re.match(r"^([a-zA-Z]\w*)\s*:", line)
        prev_sig = m.group(1) if m else (prev_sig if line.strip() == "" else prev_sig)
        if re.match(r"^[a-zA-Z]\w*\s*:", line):
            prev_sig = re.match(r"^([a-zA-Z]\w*)", line).group(1)
    return findings


def main():
    total = collections.Counter()
    for p in sys.argv[1:]:
        path = pathlib.Path(p)
        fs = lint(path)
        print(f"== {path} ({len(fs)} findings)")
        for i, code, msg, src in fs:
            print(f"  {i:4d} {code:6s} {msg}  |  {src[:90]}")
            total[code] += 1
    print("== totals", dict(total))


if __name__ == "__main__":
    main()
