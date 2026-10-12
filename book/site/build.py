#!/usr/bin/env python3
"""Render the Alder book as a static site.

Usage: python3 book/site/build.py [--out DIR]     (default: site/ at the repository root)

Writes one HTML page per chapter (book/NN-*.md -> NN-*.html; book/README.md -> index.html),
each with a sidebar listing the chapters in book/README.md's reading order, code blocks with a
`language-<lang>` class, and links between chapters rewritten to the pages. No JavaScript; one
small stylesheet (style.css). Also writes the agent's reference as Markdown:
site/reference.md (`experiments/build_context.py --reference`) and site/reference-full.md
(`build_context.py` with no flags: the reference chapters in full plus the example app).

The site is a rendering of verified sources (`go test ./book` checks the book); nothing here
checks the language. The Markdown renderer below covers what the book uses: ATX headings,
fenced code blocks, paragraphs, lists (nested by indentation), tables, block quotes, rules,
and inline code, links, bold and italic. Standard library only.
"""
import argparse
import html
import pathlib
import re
import shutil
import subprocess
import sys

ROOT = pathlib.Path(__file__).resolve().parent.parent.parent
BOOK = ROOT / "book"
SITE = ROOT / "site"
REPO_URL = "https://github.com/tapegram/alder"

CSS = """\
:root { --fg: #1d1d1f; --muted: #5f6368; --bg: #fdfdfb; --side: #f3f2ee; --code: #f4f3ef;
  --line: #dddbd3; --link: #2c5f2d; }
@media (prefers-color-scheme: dark) {
  :root { --fg: #e6e4de; --muted: #a09e98; --bg: #1b1c1a; --side: #232421; --code: #262824;
    --line: #3a3c37; --link: #9cc79a; }
}
* { box-sizing: border-box; }
body { margin: 0; background: var(--bg); color: var(--fg);
  font: 17px/1.6 Georgia, "Iowan Old Style", serif; }
.layout { display: flex; min-height: 100vh; }
nav { flex: 0 0 16rem; background: var(--side); border-right: 1px solid var(--line);
  padding: 1.5rem 1rem; font: 15px/1.4 system-ui, sans-serif; }
nav .title { font-weight: 700; margin: 0 0 1rem; display: block; color: var(--fg); }
nav ol { list-style: none; padding: 0; margin: 0; }
nav li { margin: 0.15rem 0; }
nav a { color: var(--fg); text-decoration: none; display: block; padding: 0.2rem 0.4rem;
  border-radius: 4px; }
nav a:hover { background: var(--line); }
nav a[aria-current="page"] { background: var(--line); font-weight: 600; }
nav .extra { margin-top: 1.25rem; padding-top: 1rem; border-top: 1px solid var(--line); }
main { flex: 1; min-width: 0; padding: 2rem 2.5rem 4rem; max-width: 52rem; }
a { color: var(--link); }
h1, h2, h3, h4 { font-family: system-ui, sans-serif; line-height: 1.25; }
h1 { font-size: 2rem; } h2 { margin-top: 2.2rem; } h3 { margin-top: 1.8rem; }
code { font: 0.86em/1.5 ui-monospace, SFMono-Regular, Menlo, monospace; background: var(--code);
  padding: 0.1em 0.3em; border-radius: 3px; }
pre { background: var(--code); border: 1px solid var(--line); border-radius: 6px;
  padding: 0.8rem 1rem; overflow-x: auto; }
pre code { background: none; padding: 0; font-size: 0.82em; }
table { border-collapse: collapse; margin: 1rem 0; display: block; overflow-x: auto; }
th, td { border: 1px solid var(--line); padding: 0.35rem 0.6rem; text-align: left;
  vertical-align: top; }
blockquote { margin: 1rem 0; padding: 0 1rem; border-left: 3px solid var(--line);
  color: var(--muted); }
hr { border: 0; border-top: 1px solid var(--line); margin: 2rem 0; }
.pager { display: flex; justify-content: space-between; margin-top: 3rem; padding-top: 1rem;
  border-top: 1px solid var(--line); font: 15px system-ui, sans-serif; }
@media (max-width: 760px) {
  .layout { display: block; }
  nav { border-right: 0; border-bottom: 1px solid var(--line); }
  main { padding: 1.25rem 1rem 3rem; }
}
"""


# ---- chapters -------------------------------------------------------------------------------

def chapters(book=BOOK):
    """The chapters in book/README.md's reading order: [(file name, title)]. The title is the
    link text of the README's table ("1. Why Alder")."""
    out = []
    for m in re.finditer(r"^\|\s*\[([^\]]+)\]\(([^)#]+\.md)\)", (book / "README.md").read_text(), re.M):
        title, name = m.group(1), m.group(2)
        if (book / name).exists() and name not in [n for n, _ in out]:
            out.append((name, title))
    return out


def page_name(md_name):
    """The page a book file becomes: README.md -> index.html, 01-why.md -> 01-why.html."""
    return "index.html" if md_name == "README.md" else md_name[:-3] + ".html"


# ---- Markdown -------------------------------------------------------------------------------

def slug(text):
    """GitHub's heading anchor: lower case, punctuation dropped, spaces to hyphens."""
    text = re.sub(r"<[^>]+>", "", text).strip().lower()
    text = re.sub(r"[^\w\- ]", "", text)
    return text.replace(" ", "-")


def rewrite_link(url, book_files):
    """A link to another book file becomes a link to its page (keeping the #anchor); a relative
    link out of the book becomes a link to the file in the repository."""
    if re.match(r"^[a-z]+:|^#|^/", url):
        return url
    path, _, anchor = url.partition("#")
    anchor = "#" + anchor if anchor else ""
    if path in book_files:
        return page_name(path) + anchor
    if path:
        target = (BOOK / path).resolve()
        try:
            rel = target.relative_to(ROOT)
        except ValueError:
            return url
        return f"{REPO_URL}/blob/main/{rel.as_posix()}{anchor}"
    return url


def inline(text, book_files):
    """Inline Markdown: code spans, links, bold, italic; everything else escaped. Code spans and
    links are set aside as placeholders first, so emphasis may contain them."""
    held = []

    def hold(s):
        held.append(s)
        return f"\x00{len(held) - 1}\x00"

    def code(m):
        body = m.group(2)
        return hold("<code>" + html.escape(body.strip() or body, quote=False) + "</code>")

    def link(m):
        href = html.escape(rewrite_link(m.group(2), book_files))
        return hold(f'<a href="{href}">') + m.group(1) + hold("</a>")

    text = re.sub(r"(`+)(.+?)\1", code, text)
    text = re.sub(r"\[([^\]]+)\]\(([^)\s]+)\)", link, text)
    text = emphasis(html.escape(text, quote=False))
    return re.sub(r"\x00(\d+)\x00", lambda m: held[int(m.group(1))], text)


def emphasis(s):
    s = re.sub(r"\*\*(?=\S)(.+?)(?<=\S)\*\*", r"<strong>\1</strong>", s)
    s = re.sub(r"(?<![\w*])\*(?=[^\s*])(.+?)(?<=[^\s*])\*(?![\w*])", r"<em>\1</em>", s)
    s = re.sub(r"(?<!\w)_(?=\S)([^_]+?)(?<=\S)_(?!\w)", r"<em>\1</em>", s)
    return s


LIST_ITEM = re.compile(r"^( *)([-*+]|\d+[.)]) +(.*)$")
FENCE = re.compile(r"^( *)(```+|~~~+)(.*)$")


def render(md, book_files):
    """Block-level Markdown to HTML."""
    lines = md.split("\n")
    out = []
    i = 0
    para = []

    def flush():
        if para:
            out.append("<p>" + inline(" ".join(l.strip() for l in para), book_files) + "</p>")
            para.clear()

    while i < len(lines):
        line = lines[i]
        fence = FENCE.match(line)
        if fence:
            flush()
            indent, marker, info = fence.groups()
            body = []
            i += 1
            while i < len(lines) and not lines[i].strip().startswith(marker):
                body.append(lines[i][len(indent):] if lines[i].startswith(indent) else lines[i])
                i += 1
            i += 1  # the closing fence
            lang = info.strip().split(" ")[0] if info.strip() else ""
            cls = f' class="language-{html.escape(lang)}"' if lang else ""
            out.append(f"<pre><code{cls}>" + html.escape("\n".join(body), quote=False) + "</code></pre>")
            continue
        if not line.strip():
            flush()
            i += 1
            continue
        h = re.match(r"^(#{1,6}) +(.*?) *#*$", line)
        if h:
            flush()
            level, text = len(h.group(1)), h.group(2)
            out.append(f'<h{level} id="{slug(text)}">{inline(text, book_files)}</h{level}>')
            i += 1
            continue
        if re.match(r"^ {0,3}([-*_])( *\1){2,} *$", line):
            flush()
            out.append("<hr>")
            i += 1
            continue
        if line.startswith("|") and i + 1 < len(lines) and re.match(r"^\|[\s:|-]+\|?\s*$", lines[i + 1]):
            flush()
            rows = []
            while i < len(lines) and lines[i].startswith("|"):
                rows.append(lines[i])
                i += 1
            out.append(table(rows, book_files))
            continue
        if line.startswith(">") and not para:
            quoted = []
            while i < len(lines) and lines[i].startswith(">"):
                quoted.append(re.sub(r"^> ?", "", lines[i]))
                i += 1
            out.append("<blockquote>" + render("\n".join(quoted), book_files) + "</blockquote>")
            continue
        item = LIST_ITEM.match(line)
        # as in CommonMark, a list interrupts a paragraph only with a bullet or "1."; so
        # "a\n404. Then" stays one paragraph
        if item and (not para or not item.group(2)[0].isdigit() or item.group(2)[:-1] == "1"):
            flush()
            i = list_block(lines, i, out, book_files)
            continue
        para.append(line)
        i += 1
    flush()
    return "\n".join(out)


def split_row(row):
    row = row.strip()
    if row.startswith("|"):
        row = row[1:]
    if row.endswith("|"):
        row = row[:-1]
    cells, cur, in_code = [], "", False
    for ch in row:
        if ch == "`":
            in_code = not in_code
        if ch == "|" and not in_code and not cur.endswith("\\"):
            cells.append(cur)
            cur = ""
        else:
            cur += ch
    cells.append(cur)
    return [c.strip().replace("\\|", "|") for c in cells]


def table(rows, book_files):
    head = split_row(rows[0])
    out = ["<table>", "<thead><tr>" + "".join(f"<th>{inline(c, book_files)}</th>" for c in head) + "</tr></thead>", "<tbody>"]
    for r in rows[2:]:
        out.append("<tr>" + "".join(f"<td>{inline(c, book_files)}</td>" for c in split_row(r)) + "</tr>")
    out.append("</tbody></table>")
    return "\n".join(out)


def list_block(lines, i, out, book_files):
    """A list starting at lines[i]; returns the index after it. An item's content is every
    following line indented past the marker (or blank between such lines), rendered as blocks."""
    first = LIST_ITEM.match(lines[i])
    base = len(first.group(1))
    ordered = first.group(2)[0].isdigit()
    tag = "ol" if ordered else "ul"
    start = int(re.match(r"\d+", first.group(2)).group()) if ordered else 1
    items = []
    while i < len(lines):
        m = LIST_ITEM.match(lines[i])
        if not m or len(m.group(1)) != base or m.group(2)[0].isdigit() != ordered:
            break
        content_indent = len(m.group(1)) + len(m.group(2)) + 1
        body = [m.group(3)]
        i += 1
        while i < len(lines):
            l = lines[i]
            if not l.strip():
                # a blank line continues the item only if the next line is indented into it
                j = i
                while j < len(lines) and not lines[j].strip():
                    j += 1
                if j < len(lines) and len(lines[j]) - len(lines[j].lstrip(" ")) > base:
                    body.extend([""] * (j - i))
                    i = j
                    continue
                break
            ind = len(l) - len(l.lstrip(" "))
            if ind <= base and (LIST_ITEM.match(l) or ind < base or FENCE.match(l) or l.startswith(("#", "|", ">"))):
                break
            body.append(l[min(ind, content_indent):])
            i += 1
        items.append(body)
    attr = f' start="{start}"' if ordered and start != 1 else ""
    out.append(f"<{tag}{attr}>")
    for body in items:
        rendered = render("\n".join(body), book_files)
        if rendered.startswith("<p>") and rendered.count("<p>") == 1 and "\n" not in rendered:
            rendered = rendered[3:-4]  # a one-paragraph item stays tight
        out.append(f"<li>{rendered}</li>")
    out.append(f"</{tag}>")
    return i


# ---- pages ----------------------------------------------------------------------------------

def page(title, body, current, order):
    nav = ['<nav>', '<a class="title" href="index.html">The Alder Book</a>', "<ol>"]
    for name, label in order:
        cur = ' aria-current="page"' if name == current else ""
        nav.append(f'<li><a href="{page_name(name)}"{cur}>{html.escape(label)}</a></li>')
    nav.append("</ol>")
    nav.append('<ol class="extra"><li><a href="reference.md">Agent reference (Markdown)</a></li>'
               '<li><a href="reference-full.md">Full reference (Markdown)</a></li></ol>')
    nav.append("</nav>")
    names = [n for n, _ in order]
    pager = ""
    if current in names:
        k = names.index(current)
        prev_link = f'<a href="{page_name(names[k - 1])}">← {html.escape(order[k - 1][1])}</a>' if k > 0 else '<a href="index.html">← Contents</a>'
        next_link = f'<a href="{page_name(names[k + 1])}">{html.escape(order[k + 1][1])} →</a>' if k + 1 < len(names) else "<span></span>"
        pager = f'<div class="pager">{prev_link}{next_link}</div>'
    return f"""<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{html.escape(title)} · The Alder Book</title>
<link rel="stylesheet" href="style.css">
</head>
<body>
<div class="layout">
{chr(10).join(nav)}
<main>
{body}
{pager}
</main>
</div>
</body>
</html>
"""


def first_heading(md):
    m = re.search(r"^# +(.*)$", md, re.M)
    return re.sub(r"[`*]", "", m.group(1)).strip() if m else "The Alder Book"


def reference(args):
    """The output of build_context.py with args."""
    r = subprocess.run([sys.executable, str(ROOT / "experiments" / "build_context.py"), *args],
                       capture_output=True, text=True, cwd=ROOT)
    if r.returncode != 0:
        sys.exit(f"build_context.py {' '.join(args)} failed:\n{r.stderr}")
    return r.stdout


def build(out=SITE, book=BOOK):
    order = chapters(book)
    book_files = {"README.md"} | {n for n, _ in order} | {p.name for p in book.glob("*.md")}
    if out.exists():
        shutil.rmtree(out)
    out.mkdir(parents=True)
    (out / "style.css").write_text(CSS)
    written = []
    for name in ["README.md"] + [n for n, _ in order]:
        md = (book / name).read_text()
        title = "Contents" if name == "README.md" else first_heading(md)
        (out / page_name(name)).write_text(page(title, render(md, book_files), name, order))
        written.append(page_name(name))
    (out / "reference.md").write_text(reference(["--reference"]))
    (out / "reference-full.md").write_text(reference([]))
    (out / ".nojekyll").write_text("")
    return written


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--out", default=str(SITE), help="the output directory (default: site/)")
    args = ap.parse_args(argv)
    out = pathlib.Path(args.out).resolve()
    written = build(out)
    print(f"wrote {len(written)} pages, reference.md and reference-full.md to {out}")


if __name__ == "__main__":
    main()
