#!/usr/bin/env python3
"""Build the context prompt given to models: reference chapters + example app.

Usage: build_context.py [--chapters 03,04,...] [--no-example] > prompt.md
"""
import argparse
import pathlib

ROOT = pathlib.Path(__file__).resolve().parent.parent
BOOK = ROOT / "book"
EXAMPLE = ROOT / "examples" / "todos" / "app"

DEFAULT_CHAPTERS = ["03", "04", "05", "06", "07", "08", "09", "10"]

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


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--chapters", default=",".join(DEFAULT_CHAPTERS))
    ap.add_argument("--no-example", action="store_true")
    args = ap.parse_args()

    out = [HEADER]
    for ch in args.chapters.split(","):
        files = sorted(BOOK.glob(f"{ch}-*.md"))
        for f in files:
            out.append(f.read_text())
            out.append("\n")
    if not args.no_example:
        out.append(EXAMPLE_HEADER)
        for f in sorted(EXAMPLE.rglob("*.alder")):
            rel = f.relative_to(EXAMPLE.parent)
            out.append(f"\n--- file: {rel} ---\n")
            out.append(f.read_text())
    print("".join(out))


if __name__ == "__main__":
    main()
