#!/usr/bin/env python3
"""Split a model output with `--- file: path ---` markers into files under <outdir>."""
import pathlib, re, sys
src = pathlib.Path(sys.argv[1]); out = pathlib.Path(sys.argv[2])
text = src.read_text()
parts = re.split(r"^-{3,}\s*file:\s*(\S+?)\s*-{3,}\s*$", text, flags=re.M)
# parts: [preamble, path1, body1, path2, body2, ...]
for i in range(1, len(parts), 2):
    p = out / parts[i]; p.parent.mkdir(parents=True, exist_ok=True)
    body = parts[i+1].strip("\n")
    body = re.sub(r"^```\w*\n|\n```\s*$", "", body)
    p.write_text(body + "\n")
    print(p)
