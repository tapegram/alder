#!/usr/bin/env python3
"""Tests of book/site/build.py: python3 book/site/test_site.py

Builds the site into a temporary directory once and checks it: every chapter of book/README.md
has a page, every page's sidebar lists the chapters in the README's order, code blocks carry
their language class, links between chapters point at pages and anchors that exist, the two
reference files equal build_context.py's output, no page has JavaScript or unrendered Markdown.
"""
import pathlib
import re
import subprocess
import sys
import tempfile
import unittest

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parent))
import build  # noqa: E402

ROOT = build.ROOT
BOOK = build.BOOK


class SiteTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.tmp = tempfile.TemporaryDirectory()
        cls.out = pathlib.Path(cls.tmp.name) / "site"
        cls.written = build.build(cls.out)
        cls.order = build.chapters()

    @classmethod
    def tearDownClass(cls):
        cls.tmp.cleanup()

    def pages(self):
        return sorted(self.out.glob("*.html"))

    def test_every_chapter_has_a_page(self):
        linked = re.findall(r"^\|\s*\[[^\]]+\]\(([^)]+\.md)\)", (BOOK / "README.md").read_text(), re.M)
        self.assertGreaterEqual(len(linked), 16)
        self.assertEqual([n for n, _ in self.order], linked)
        for name in linked:
            self.assertTrue((self.out / build.page_name(name)).exists(), name)
        self.assertTrue((self.out / "index.html").exists())
        self.assertTrue((self.out / "style.css").exists())
        # every chapter file in book/ is in the reading order: none is left off the site
        on_disk = sorted(p.name for p in BOOK.glob("*.md") if p.name != "README.md")
        self.assertEqual(sorted(linked), on_disk)

    def test_sidebar_follows_the_readme_order(self):
        want = [build.page_name(n) for n, _ in self.order]
        for page in self.pages():
            nav = re.search(r"<nav>(.*?)</nav>", page.read_text(), re.S).group(1)
            got = re.findall(r'<li><a href="([^"]+\.html)"', nav)
            self.assertEqual(got, want, page.name)
            if page.name != "index.html":
                self.assertIn(f'<a href="{page.name}" aria-current="page">', nav, page.name)

    def test_code_blocks_carry_their_language(self):
        for name, _ in self.order:
            md = (BOOK / name).read_text()
            fences = re.findall(r"^ *```(\w+)", md, re.M)
            html = (self.out / build.page_name(name)).read_text()
            for lang in set(fences):
                self.assertEqual(html.count(f'<pre><code class="language-{lang}">'), fences.count(lang), f"{name} {lang}")

    def test_links_between_chapters_resolve(self):
        ids = {p.name: set(re.findall(r' id="([^"]+)"', p.read_text())) for p in self.pages()}
        for page in self.pages():
            markup = re.sub(r"<pre>.*?</pre>|<code>.*?</code>", "", page.read_text(), flags=re.S)
            for href in re.findall(r'href="([^"]+)"', markup):
                if href.startswith(("http://", "https://")) or href in ("style.css", "reference.md", "reference-full.md"):
                    continue
                self.assertFalse(href.split("#")[0].endswith(".md"), f"{page.name}: {href} not rewritten")
                target, _, anchor = href.partition("#")
                target = target or page.name
                self.assertIn(target, ids, f"{page.name}: {href}")
                if anchor:
                    self.assertIn(anchor, ids[target], f"{page.name}: {href}")

    def test_reference_files(self):
        for name, args in (("reference.md", ["--reference"]), ("reference-full.md", [])):
            want = subprocess.run([sys.executable, str(ROOT / "experiments" / "build_context.py"), *args],
                                  capture_output=True, text=True, check=True).stdout
            self.assertEqual((self.out / name).read_text(), want, name)
        self.assertLess(len((self.out / "reference.md").read_text()), len((self.out / "reference-full.md").read_text()))

    def test_no_javascript_and_no_unrendered_markdown(self):
        for page in self.pages():
            html = page.read_text()
            self.assertNotIn("<script", html.lower(), page.name)
            prose = re.sub(r"<pre>.*?</pre>|<code>.*?</code>", "", html, flags=re.S)
            for leftover in ("```", "**", "`", "]("):
                self.assertNotIn(leftover, prose, f"{page.name}: {leftover!r}")


class RenderTest(unittest.TestCase):
    def r(self, md):
        return build.render(md, {"README.md", "02-tour.md"})

    def test_heading_ids_match_github(self):
        self.assertEqual(self.r("## `alder build --emit`"),
                         '<h2 id="alder-build---emit"><code>alder build --emit</code></h2>')
        self.assertIn('id="go-as-compiler-language-and-target-untested"', self.r("### Go as compiler language and target (untested)"))

    def test_fence_language_and_escaping(self):
        self.assertEqual(self.r('```alder eval=<p>3</p>\nview = <p>{x}</p>\n```'),
                         '<pre><code class="language-alder">view = &lt;p&gt;{x}&lt;/p&gt;</code></pre>')
        self.assertEqual(self.r("```\nplain\n```"), "<pre><code>plain</code></pre>")

    def test_links(self):
        self.assertIn('<a href="02-tour.html#the-model">the tour</a>', self.r("See [the tour](02-tour.md#the-model)."))
        self.assertIn('<a href="index.html">', self.r("[back](README.md)"))
        self.assertIn('href="https://github.com/tapegram/alder/blob/main/docs/x.md"', self.r("[spec](../docs/x.md)"))
        self.assertIn('href="https://example.com"', self.r("[e](https://example.com)"))

    def test_inline(self):
        self.assertEqual(self.r("**A capability in `load`.** Then *this* and `a < b`."),
                         "<p><strong>A capability in <code>load</code>.</strong> Then <em>this</em> and <code>a &lt; b</code>.</p>")
        self.assertEqual(self.r("snake_case_name stays"), "<p>snake_case_name stays</p>")

    def test_lists_tables_quotes(self):
        self.assertEqual(self.r("- one\n  more\n- two"), "<ul>\n<li>one more</li>\n<li>two</li>\n</ul>")
        self.assertEqual(self.r("3. c\n4. d"), '<ol start="3">\n<li>c</li>\n<li>d</li>\n</ol>')
        self.assertEqual(self.r("- a\n  - b\n- c"), "<ul>\n<li><p>a</p>\n<ul>\n<li>b</li>\n</ul></li>\n<li>c</li>\n</ul>")
        self.assertIn("<td><code>a|b</code></td>", self.r("| x | y |\n|---|---|\n| `a|b` | z |"))
        self.assertEqual(self.r("> quoted"), "<blockquote><p>quoted</p></blockquote>")
        # a line starting with a number or `>` inside a paragraph continues it
        self.assertEqual(self.r("or a\n404. Then"), "<p>or a 404. Then</p>")


if __name__ == "__main__":
    unittest.main()
