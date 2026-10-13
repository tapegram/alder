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

## Implementing a capability

Wiring intro.

### The rule

Rule text.

```go
### not a subheading (inside a fence)
```

### What the build does

Build internals.

## After

After text.
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
        self.saved_keep = bc.KEEP_SUBSECTIONS
        self.saved_drop = bc.DROP_SUBSECTIONS
        self.saved_trims = bc.TRIMS
        bc.HUMAN_SECTIONS = {"06": ["How a request runs"], "07": ["How effects run"]}
        bc.KEEP_SUBSECTIONS = {}
        bc.DROP_SUBSECTIONS = {}
        bc.TRIMS = {}

    def tearDown(self):
        bc.HUMAN_SECTIONS = self.saved
        bc.TRIMS = self.saved_trims
        bc.KEEP_SUBSECTIONS = self.saved_keep
        bc.DROP_SUBSECTIONS = self.saved_drop
        bc.TRIMS = self.saved_trims
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

    def test_reference_keeps_listed_subsections(self):
        bc.HUMAN_SECTIONS = {"07": ["How effects run", "Implementing a capability"]}
        bc.KEEP_SUBSECTIONS = {"07": {"Implementing a capability": ["The rule"]}}
        code, out, _ = self.run_main("--reference", "--chapters", "07")
        self.assertEqual(code, 0)
        for gone in ("Effect internals.", "Wiring intro.", "### What the build does", "Build internals."):
            self.assertNotIn(gone, out)
        kept = ["## Building effects", "## Implementing a capability\n", "### The rule", "Rule text.",
                "### not a subheading (inside a fence)", "## After", "After text."]
        pos = [out.index(k) for k in kept]
        self.assertEqual(pos, sorted(pos))

    def test_reference_unknown_subsection(self):
        bc.HUMAN_SECTIONS = {"07": ["Implementing a capability"]}
        bc.KEEP_SUBSECTIONS = {"07": {"Implementing a capability": ["The rules"]}}
        code, out, err = self.run_main("--reference", "--chapters", "07")
        self.assertEqual(code, 2)
        self.assertEqual(out, "")
        self.assertIn('"### The rules"', err)

    def test_reference_drops_listed_subsections(self):
        # the inverse of KEEP_SUBSECTIONS: a kept section loses the `### ` subsections listed
        bc.HUMAN_SECTIONS = {}
        bc.DROP_SUBSECTIONS = {"07": {"Implementing a capability": ["What the build does"]}}
        code, out, _ = self.run_main("--reference", "--chapters", "07")
        self.assertEqual(code, 0)
        for gone in ("### What the build does", "Build internals."):
            self.assertNotIn(gone, out)
        kept = ["## How effects run", "Effect internals.", "## Implementing a capability\n", "Wiring intro.",
                "### The rule", "Rule text.", "### not a subheading (inside a fence)", "## After", "After text."]
        pos = [out.index(k) for k in kept]
        self.assertEqual(pos, sorted(pos))

    def test_reference_unknown_dropped_subsection(self):
        bc.HUMAN_SECTIONS = {}
        bc.DROP_SUBSECTIONS = {"07": {"Implementing a capability": ["The rules"]}}
        code, out, err = self.run_main("--reference", "--chapters", "07")
        self.assertEqual(code, 2)
        self.assertEqual(out, "")
        self.assertIn('"### The rules"', err)
        self.assertIn("DROP_SUBSECTIONS", err)
        bc.DROP_SUBSECTIONS = {"07": {"Running effects": ["The rule"]}}
        code, _, err = self.run_main("--reference", "--chapters", "07")
        self.assertEqual(code, 2)
        self.assertIn('"## Running effects"', err)

    def test_reference_trims_spans(self):
        # TRIMS replaces the text from a start up to an end, in --reference only
        bc.TRIMS = {"06": [("Page text.", "## How a request runs", "Short.\n\n")]}
        code, out, _ = self.run_main("--reference", "--chapters", "06")
        self.assertEqual(code, 0)
        self.assertIn("## A page module\n\nShort.\n\n## The `App` module", out)
        self.assertNotIn("not a heading (inside a fence)", out)
        code, out, _ = self.run_main("--chapters", "06")
        self.assertIn("Page text.", out)
        # a start that is missing, or an end that does not follow it, exits 2 naming it
        for span in (("Pages text.", "## The", ""), ("App text.", "## A page module", "")):
            bc.TRIMS = {"06": [span]}
            code, out, err = self.run_main("--reference", "--chapters", "06")
            self.assertEqual(code, 2)
            self.assertEqual(out, "")
            self.assertIn(repr(span[0]), err)
            self.assertIn("TRIMS", err)

    def test_reference_renders_module_error_blocks(self):
        # an `error=` block of a whole module becomes its error text; an expression's stays
        (self.book / "06-pages.md").write_text(CH06 + '\nRefused:\n\n```alder error=E0026 "`Db.mine` needs a page"\nmodule Public\n\nx = 1\n```\n\n'
                                              '```alder error=E0003 "same type"\n1 == "a"\n```\n')
        code, out, _ = self.run_main("--reference", "--chapters", "06")
        self.assertEqual(code, 0)
        self.assertIn("Refused:\n\n```text\nerror[E0026]: `Db.mine` needs a page\n```\n", out)
        self.assertNotIn("module Public", out)
        self.assertIn('```alder error=E0003 "same type"\n1 == "a"\n```', out)
        code, out, _ = self.run_main("--chapters", "06")
        self.assertIn("module Public", out)

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
        for table in (self.saved_keep, self.saved_drop):
            for ch, secs in table.items():
                text = next(bc.BOOK.glob(f"{ch}-*.md")).read_text()
                for h, subs in secs.items():
                    self.assertIn("\n## " + h + "\n", text)
                    for sh in subs:
                        self.assertIn("\n### " + sh + "\n", text)

    def test_real_reference_keeps_adapter_rule(self):
        # an agent implementing a capability needs where its adapter goes and what it is called:
        # chapter 7's "The rule" stays, the rest of "Implementing a capability" goes
        bc.HUMAN_SECTIONS = self.saved
        bc.TRIMS = self.saved_trims
        bc.KEEP_SUBSECTIONS = self.saved_keep
        code, out, _ = self.run_main("--reference", "--chapters", "07", "--book", str(bc.BOOK))
        self.assertEqual(code, 0)
        self.assertIn("\n## Implementing a capability\n", out)
        self.assertIn("\n### The rule\n", out)
        self.assertIn("func LoginPassword() app.Cap__Auth__Login__Password", out)
        self.assertNotIn("\n### What the build does with `adapters/`\n", out)
        self.assertIn("`package selectadapter`", out)
        # and the shape of a capability in Go (method, Args__ record, types, imports), not the
        # rest of "How effects run"
        self.assertIn("\n## How effects run\n\n### What a capability looks like in Go\n", out)
        for want in ("Verify(ctx context.Context, args Args__Auth__Login__Password__Verify) (rt.Maybe[string], error)",
                     "`a1`, `a2`", "`Unit` `struct{}`", "imports it\nas `\"app\"`", "`db.TxFrom(ctx)`"):
            self.assertIn(want, out)
        self.assertNotIn("\n### A capability in the task's transaction\n", out)
        self.assertNotIn("\n### How the program finds an implementation\n", out)
        self.assertNotIn("**An effect is a tree.**", out)

    def test_real_reference_keeps_migrations(self):
        # agents need the E0019 -> `alder migrate` workflow: chapter 8's "Migrations" stays in
        # the reference, "How tables are stored" goes
        bc.HUMAN_SECTIONS = self.saved
        bc.TRIMS = self.saved_trims
        code, out, _ = self.run_main("--reference", "--chapters", "08", "--book", str(bc.BOOK))
        self.assertEqual(code, 0)
        self.assertIn("\n## Migrations\n", out)
        self.assertNotIn("\n## How tables are stored\n", out)

    def test_real_reference_keeps_fakes(self):
        # agents write fakes and install them: chapter 10's "Faking a capability" stays in the
        # reference, "What cannot be tested yet" goes
        bc.HUMAN_SECTIONS = self.saved
        bc.TRIMS = self.saved_trims
        code, out, _ = self.run_main("--reference", "--chapters", "10", "--book", str(bc.BOOK))
        self.assertEqual(code, 0)
        self.assertIn("\n## 8. Faking a capability\n", out)
        self.assertIn("fake Stripe {", out)
        self.assertNotIn("\n## 12. What cannot be tested yet\n", out)

    def test_real_reference_keeps_mutation_testing(self):
        # agents run `alder test --mutate` and read its survivors: the command, the report's
        # shape, the operators and the rule stay; the case for a mutation score and the
        # suppression rules go
        bc.HUMAN_SECTIONS = self.saved
        bc.TRIMS = self.saved_trims
        bc.DROP_SUBSECTIONS = self.saved_drop
        code, out, _ = self.run_main("--reference", "--chapters", "10", "--book", str(bc.BOOK))
        self.assertEqual(code, 0)
        self.assertIn("\n## 11. Mutation testing\n", out)
        for kept in ("alder test --mutate [--min R] [--json] [-run re] [-timeout d] [dir]",
                     "32 mutants: 18 killed, 14 survived  (score 0.56)", "| `requires dropped` |",
                     "### A surviving mutant is a test you have not written", "test \"a visitor is sent to log in from a todo\"",
                     "### Survivors that are not gaps", "### `--min` in CI"):
            self.assertIn(kept, out)
        for gone in ("### Why a mutation score", "### What is never a mutant", "line coverage", '"tables":"ensure"',
                     "### Ids and lines change with the code", "### All fourteen, and one killed by luck",
                     "### The logout redirect", "### Mutants that are the same program", "### The exact score",
                     "ok   Todo \"countOpen ignores done todos\""):
            self.assertNotIn(gone, out)
        self.assertIn("32 mutants: 32 killed, 0 survived  (score 1.00)", out)   # the run after: its score line only
        section = out[out.index("\n## 11. Mutation testing\n"):].split("====================  EXAMPLE")[0].rstrip("\n")
        # 6.5k until M11, whose two operators (`role dropped`, `scope dropped`) are two more rows
        self.assertLessEqual(len(section), 6600, "the reference keeps at most 6.6k characters of chapter 10's section 11")

    def test_real_reference_keeps_browser_steps(self):
        # agents write browser tests: chapter 10's "Testing through the browser" (the step table,
        # the rules, the texts) stays in the reference, "How the browser works" goes
        bc.HUMAN_SECTIONS = self.saved
        bc.TRIMS = self.saved_trims
        bc.DROP_SUBSECTIONS = self.saved_drop
        code, out, _ = self.run_main("--reference", "--chapters", "09,10", "--book", str(bc.BOOK))
        self.assertEqual(code, 0)
        self.assertIn("\n## 9. Testing through the browser\n", out)
        self.assertIn("| `Test.submit F r` |", out)
        self.assertNotIn("\n## 10. How the browser works\n", out)
        # §9 keeps the table, one example, the encoding, every failure text and the rules; it
        # drops the walkthrough, the further examples and chapter 9's repeated signatures
        for kept in ("test \"adding a todo through the page\"", "| `FormRejected errors` |", "| `Maybe a` | blank",
                     "### What a failed step says", "has no button or form for", "has no input `due`",
                     "test \"Toggle by another user is not found\"", "| `ul > li` |", "\n### The rules\n",
                     "## Test\n", "Test.run : Task a"):
            self.assertIn(kept, out)
        for gone in ("### Step by step", "module Drafts", "test \"a todo with a due date\"", "1. Insert a user, as any effect test does.",
                     "When to use which", "test \"a misspelt selector\"", "Test.login     : Session -> Task Unit",
                     "test \"a stranger cannot see my todo\"", "FAIL Todos.Index \"a stranger clicks my todo\""):
            self.assertNotIn(gone, out)

    def test_real_reference_cut(self):
        # M11's final review cut the reference to at most 44k tokens (len/4, with the example):
        # the rules' facts stay, their repeated quotes and transcripts go
        bc.HUMAN_SECTIONS, bc.KEEP_SUBSECTIONS = self.saved, self.saved_keep
        bc.DROP_SUBSECTIONS, bc.TRIMS = self.saved_drop, self.saved_trims
        code, out, _ = self.run_main("--reference", "--chapters", ",".join(bc.DEFAULT_CHAPTERS), "--book", str(bc.BOOK),
                                     "--example", str(bc.EXAMPLE))
        self.assertEqual(code, 0)
        self.assertLessEqual(len(out) // 4, 44000, "the reference is at most 44k tokens (len/4)")
        for kept in ("**`Test.login s`** sets it to `s`", "**`Session.put`**, in the test", "A login through the login page's form",
                     "So a test that calls `update` or `load` itself", "Db.updateMine Todo: no signed-in user",
                     "`update` call is not checked", "error[E0026]: `Db.mine` needs a page that `requires user`: `Public` does not",
                     "isAuthor : UserId -> Query Bool", "`2026-01-01T00:00:00Z`", "chapter 6, \"Rules\",\nexplains them",
                     "write the scoped form; it costs "):
            self.assertIn(kept, out)
        for gone in ("$ alder test examples/todos -run 'without a login'", "module Public",
                     "Db.mine       : Table t -> Select t", "The to-do list's page uses four of them\n(`examples", "\n### When the app starts\n",
                     "let author <- User.isAuthor session.user"):
            self.assertNotIn(gone, out)


if __name__ == "__main__":
    unittest.main()
