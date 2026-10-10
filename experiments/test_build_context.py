import contextlib, io, json, pathlib, shutil, sys, tempfile, unittest
sys.path.insert(0, str(pathlib.Path(__file__).resolve().parent))
import build_context as bc

CH06 = """# Pages

Intro to pages.

## A page module

Page text.

```markdown
## not a heading (inside a fence)
```

## How a request runs

Internals of a request.

## The `App` module

App text.
"""

CH07 = """# Effects

## Building effects

Effects text.

## How effects run

Effect internals.
"""

class BuildContextTests(unittest.TestCase):
    def setUp(self):
        self.tmp = pathlib.Path(tempfile.mkdtemp())
        self.book = self.tmp / "book"
        self.book.mkdir()
        (self.book / "06-pages.md").write_text(CH06)
        (self.book / "07-effects.md").write_text(CH07)
        self.example = self.tmp / "ex" / "app"
        self.example.mkdir(parents=True)
        (self.example / "App.alder").write_text("module App\n")
        self.saved = bc.HUMAN_SECTIONS
        bc.HUMAN_SECTIONS = {"06": ["How a request runs"], "07": ["How effects run"]}

    def tearDown(self):
        bc.HUMAN_SECTIONS = self.saved
        shutil.rmtree(self.tmp)

    def run_main(self, *args):
        out, err = io.StringIO(), io.StringIO()
        code = 0
        with contextlib.redirect_stdout(out), contextlib.redirect_stderr(err):
            try:
                bc.main(["--book", str(self.book), "--example", str(self.example), "--chapters", "06,07", *args])
            except SystemExit as e:
                code = e.code
        return code, out.getvalue(), err.getvalue()

    def test_full_keeps_everything(self):
        code, out, _ = self.run_main()
        self.assertEqual(code, 0)
        self.assertIn("Internals of a request.", out)
        self.assertIn("--- file: app/App.alder ---\nmodule App\n", out)

    def test_reference_drops_listed_sections(self):
        code, out, _ = self.run_main("--reference")
        self.assertEqual(code, 0)
        for gone in ("## How a request runs", "Internals of a request.", "## How effects run", "Effect internals."):
            self.assertNotIn(gone, out)
        kept = ["# Pages", "Intro to pages.", "## A page module", "## not a heading (inside a fence)",
                "## The `App` module", "App text.", "# Effects", "## Building effects", "Effects text.", "module App"]
        pos = [out.index(k) for k in kept]
        self.assertEqual(pos, sorted(pos))   # order kept
        self.assertTrue(out.startswith(bc.HEADER))

    def test_reference_only_selected_chapters(self):
        code, out, _ = self.run_main("--reference", "--chapters", "06")
        self.assertEqual(code, 0)
        self.assertNotIn("Effects text.", out)

    def test_reference_unknown_heading(self):
        bc.HUMAN_SECTIONS = {"06": ["How a request runs", "Schema changes"]}
        code, out, err = self.run_main("--reference")
        self.assertEqual(code, 2)
        self.assertEqual(out, "")
        self.assertIn('"## Schema changes"', err)
        self.assertIn("06-pages.md", err)

    def test_size_prints_chars_and_estimate(self):
        _, full, _ = self.run_main()
        code, out, _ = self.run_main("--size")
        self.assertEqual(code, 0)
        prompt = full[:-1] if full.endswith("\n") else full   # print adds the final newline
        self.assertEqual(out, f"characters: {len(prompt)}\nestimate (len/4): {len(prompt) // 4}\n")
        _, ref, _ = self.run_main("--reference", "--size")
        self.assertLess(int(ref.split()[1]), len(prompt))

    def test_count_asks_ollama(self):
        sent = []
        def fake_urlopen(req, timeout=None):
            sent.append((req.full_url, json.loads(req.data)))
            return io.BytesIO(json.dumps({"message": {"content": "x"}, "prompt_eval_count": 123, "eval_count": 1}).encode())
        saved = bc.urllib.request.urlopen
        bc.urllib.request.urlopen = fake_urlopen
        try:
            code, out, _ = self.run_main("--count", "ollama:qwen3-coder:30b")
        finally:
            bc.urllib.request.urlopen = saved
        self.assertEqual(code, 0)
        self.assertIn("prompt_eval_count (ollama:qwen3-coder:30b, num_ctx 65536): 123\n", out)
        self.assertIn("characters: ", out)
        url, body = sent[0]
        self.assertTrue(url.endswith("/api/chat"))
        self.assertEqual(body["model"], "qwen3-coder:30b")
        self.assertEqual(body["options"]["num_predict"], 1)
        self.assertEqual(body["messages"][0]["role"], "system")
        self.assertIn("Intro to pages.", body["messages"][0]["content"])

    def test_real_book_headings_exist(self):
        # the drop list matches the book as it is (a renamed heading fails here, and the tool exits 2)
        for ch, heads in self.saved.items():
            text = next(bc.BOOK.glob(f"{ch}-*.md")).read_text()
            for h in heads:
                self.assertIn("\n## " + h + "\n", text)

if __name__ == "__main__":
    unittest.main()
