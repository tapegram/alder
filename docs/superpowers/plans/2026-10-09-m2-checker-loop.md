# M2 Checker-in-the-Loop Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Settle the three post-M1 decisions in the compiler (fix contract, per-module gating, positional message arguments), build a harness that runs generate → `alder check --json` → feed diagnostics back → retry, and use it to run experiment E5 so the thesis is measured with a real compiler for the first time.

**Architecture:** Two small compiler changes (`internal/diag` fix spans; `internal/names`/`internal/check` per-module gating; one new E0301 shape in `internal/check`) followed by a Python harness (`experiments/run_loop.py`) that drives Ollama over HTTP and Claude through the local `claude -p` CLI, splits model output into an app directory, runs a prebuilt `alder` binary, and writes per-round JSON plus a summary. A reporter aggregates summaries into a Markdown table. E5 is the first dataset produced by it.

**Tech Stack:** Go 1.27 stdlib (compiler); Python 3 stdlib only (harness, `unittest`); Ollama HTTP API at `localhost:11434`; `claude` CLI 2.1.x in `--print` mode.

**Spec:** `docs/superpowers/specs/2026-10-08-compiler-vertical-slice-design.md` — sections "Milestones: M2", "Testing: Harness", and "Decisions recorded after M1" (the three decisions this plan implements). Language reference: `book/`. Experiment conventions: `experiments/README.md`.

## Global Constraints

- Compiler: Go `1.27`, module `alder`, stdlib only; `go vet ./... && go test ./...` green and `gofmt -l .` empty before every commit; `alder check examples/todos` stays clean.
- Fix contract (spec): JSON `fix = {line, col, endLine, endCol, replace}`, replace the half-open span `[start, end)`; insertion has `start == end`. Text output prints `fix: <replace>` unchanged. Golden/corpus `.diag` files print fixes as `  fix L:C-EL:EC text`.
- Gating (spec): parse errors stop the pipeline; after names, modules whose resolution produced errors are skipped by the body-check pass; every other module is checked; the declaration pass runs for all.
- Harness: Python 3 stdlib only; no network except `localhost:11434` and the `claude` CLI; every model call is one shot with `temperature 0` for Ollama; Claude via `claude -p --model <m> --output-format text --no-session-persistence` with tools disabled via `--disallowedTools Bash,Read,Write,Edit,Glob,Grep,WebSearch,WebFetch,Agent,NotebookEdit`; prompt on stdin.
- Harness outputs live under `experiments/<exp>/runs/<model>/<task>/round<N>/` (`app/**.alder`, `output.md`, `check.json`) plus `summary.json`; nothing else is written outside the experiment directory.
- Retries: exactly one by default (`--retries 1`); the retry prompt is the original task plus the previous round's diagnostics rendered in text form (with fixes) plus the instruction to output every file again in full.
- A round is "pass" iff `alder check --json` exits 0. Error counts are the number of JSON lines with `"severity":"error"`.
- Commit messages end with the two attribution lines given in the conversation.

## Review Focus

1. A model wraps files in ``` fences or adds prose before the first `--- file:` marker — the splitter must still extract every file (Task 3 `test_split_tolerates_fences_and_preamble`).
2. On retry a model outputs only the files it changed — the loop must overlay the new files on the previous round's app directory, never start from empty (Task 3 `test_retry_overlays_previous_round`).
3. `alder check` exits 2 (no files, unreadable file, internal error) — the round is recorded as failed with the stderr text and the loop continues to the next run; the harness does not crash (Task 3 `test_check_exit2_recorded_not_raised`).
4. A model returns nothing or garbage (no `--- file:` markers) — recorded as a failed round with `files: 0`, retry still happens with that fact in the prompt (Task 3 `test_no_files_round`).
5. Ollama is down or `claude` is missing — the run reports the error in `summary.json` and the CLI exits 1 after finishing other runs (Task 3 `test_model_unavailable`).

---

## File structure

```
internal/diag/diag.go              Fix gains EndLine, EndCol; helpers ReplaceSpan, InsertAt, Apply
internal/diag/render.go            jsonFix gains endLine, endCol
internal/diag/diag_test.go         contract tests, Apply tests
internal/check/{pattern,decls,expr}.go, internal/names/resolve.go   every Fix producer uses the helpers
internal/check/check_test.go       golden renderer prints fix as L:C-EL:EC
testdata/golden/*.diag, testdata/corpus/*/expected.diag             regenerated once, diff reviewed
internal/names/graph.go            Program.Failed map; FailedModules(bag)
internal/check/check.go            body pass skips failed modules
cmd/alder/main.go                  pipeline: parse → names → always check (skipping failed)
testdata/golden/check_gating/      cycle + unrelated type error both reported
internal/check/expr.go             positional message argument → E0301
testdata/corpus/E0301-positional-arg/
experiments/run_loop.py            the harness (models, split, check, loop, summary)
experiments/report_loop.py         summaries → Markdown table
experiments/test_run_loop.py       unittest with a fake model and a fake checker
experiments/e5-loop/               prompt.md, tasks/, runs/, scores.md (generated), findings.md
book/11-tooling.md, book/12-rationale.md, experiments/README.md     docs
```

---

### Task 1: Fix contract — spans on fixes

**Files:**
- Modify: `internal/diag/diag.go`, `internal/diag/render.go`, `internal/diag/diag_test.go`
- Modify (producers): `internal/check/pattern.go:254`, `internal/check/decls.go:126,618`, `internal/check/expr.go:189,191,529,574,916`, `internal/names/resolve.go:237,271,306,568`
- Modify: `internal/check/check_test.go` (`render`), `cmd/alder/main_test.go` (`TestCLI_TextFormat` unchanged; `TestCLI_JSONAndExit` prefix unchanged)
- Regenerate: every `testdata/golden/*.diag` and `testdata/corpus/*/expected.diag` that contains a `fix` line
- Modify: `book/11-tooling.md` (JSON sample and a "Fix contract" paragraph), spec is already updated

**Interfaces:**
- Produces:
  ```go
  package diag
  type Fix struct{ Line, Col, EndLine, EndCol int; Replace string }
  func ReplaceSpan(sp Span, text string) *Fix        // [sp.Start, sp.End) → text
  func InsertAt(line, col int, text string) *Fix     // start == end
  func Apply(src string, fixes []Fix) (string, error) // applies non-overlapping fixes, last-first; error on overlap or out of range
  ```
  JSON: `"fix":{"line":L,"col":C,"endLine":EL,"endCol":EC,"replace":"…"}`.

- [ ] **Step 1: Write the failing tests**

Add to `internal/diag/diag_test.go`:

```go
func TestFixHelpers(t *testing.T) {
	sp := Span{File: "f", Start: Pos{3, 7}, End: Pos{3, 12}}
	f := ReplaceSpan(sp, "X")
	if f.Line != 3 || f.Col != 7 || f.EndLine != 3 || f.EndCol != 12 || f.Replace != "X" {
		t.Fatalf("ReplaceSpan: %+v", f)
	}
	i := InsertAt(5, 1, "import Todo\n")
	if i.Line != 5 || i.Col != 1 || i.EndLine != 5 || i.EndCol != 1 {
		t.Fatalf("InsertAt must be a zero-width span: %+v", i)
	}
}

func TestRenderJSON_FixHasEnd(t *testing.T) {
	d := Diagnostic{Code: "E0101", Severity: "error",
		Span: Span{File: "a.alder", Start: Pos{2, 3}, End: Pos{2, 9}}, Message: "m",
		Fix: ReplaceSpan(Span{Start: Pos{2, 3}, End: Pos{2, 9}}, "Db.first")}
	got := RenderJSON(d)
	want := `{"code":"E0101","severity":"error","file":"a.alder","line":2,"col":3,"endLine":2,"endCol":9,"message":"m","fix":{"line":2,"col":3,"endLine":2,"endCol":9,"replace":"Db.first"}}`
	if got != want {
		t.Fatalf("\n got %s\nwant %s", got, want)
	}
}

func TestApply(t *testing.T) {
	src := "module M\n\nx : Int\nx = Db.get Todo slug\n"
	// replace `Db.get Todo slug` (line 4, cols 5..21) and insert an import on line 2
	out, err := Apply(src, []Fix{
		*ReplaceSpan(Span{Start: Pos{4, 5}, End: Pos{4, 21}}, "Db.first"),
		*InsertAt(2, 1, "import Todo\n"),
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "module M\nimport Todo\n\nx : Int\nx = Db.first\n"
	if out != want {
		t.Fatalf("\n got %q\nwant %q", out, want)
	}
	if _, err := Apply(src, []Fix{*ReplaceSpan(Span{Start: Pos{9, 1}, End: Pos{9, 2}}, "x")}); err == nil {
		t.Fatal("out-of-range fix must error")
	}
	if _, err := Apply(src, []Fix{
		*ReplaceSpan(Span{Start: Pos{4, 5}, End: Pos{4, 21}}, "a"),
		*ReplaceSpan(Span{Start: Pos{4, 8}, End: Pos{4, 10}}, "b"),
	}); err == nil {
		t.Fatal("overlapping fixes must error")
	}
}
```

Update the existing `TestRenderJSON` expected string to include `"endLine":3,"endCol":7` inside `fix` (the fix in that test becomes `InsertAt(3, 7, "Db.first")`).

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/diag/`
Expected: FAIL — `EndLine` undefined, `ReplaceSpan` undefined.

- [ ] **Step 3: Implement**

```go
// internal/diag/diag.go — replace the Fix type and add helpers
type Fix struct {
	Line, Col       int
	EndLine, EndCol int
	Replace         string
}

// ReplaceSpan is a fix that replaces the half-open span [sp.Start, sp.End) with text.
func ReplaceSpan(sp Span, text string) *Fix {
	return &Fix{Line: sp.Start.Line, Col: sp.Start.Col, EndLine: sp.End.Line, EndCol: sp.End.Col, Replace: text}
}

// InsertAt is a zero-width fix: text is inserted before (line, col).
func InsertAt(line, col int, text string) *Fix {
	return &Fix{Line: line, Col: col, EndLine: line, EndCol: col, Replace: text}
}

// Apply applies fixes to src (lines joined by "\n"). Fixes must not overlap and must lie within src.
func Apply(src string, fixes []Fix) (string, error) {
	lines := strings.Split(src, "\n")
	offset := func(line, col int) (int, error) {
		if line < 1 || line > len(lines) || col < 1 || col > len(lines[line-1])+1 {
			return 0, fmt.Errorf("fix position %d:%d is outside the source", line, col)
		}
		n := 0
		for i := 0; i < line-1; i++ {
			n += len(lines[i]) + 1
		}
		return n + col - 1, nil
	}
	type span struct{ s, e int; text string }
	spans := make([]span, 0, len(fixes))
	for _, f := range fixes {
		s, err := offset(f.Line, f.Col)
		if err != nil {
			return "", err
		}
		e, err := offset(f.EndLine, f.EndCol)
		if err != nil {
			return "", err
		}
		if e < s {
			return "", fmt.Errorf("fix end before start at %d:%d", f.Line, f.Col)
		}
		spans = append(spans, span{s, e, f.Replace})
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].s > spans[j].s })
	for i := 1; i < len(spans); i++ {
		if spans[i].e > spans[i-1].s {
			return "", fmt.Errorf("overlapping fixes")
		}
	}
	out := src
	for _, sp := range spans {
		out = out[:sp.s] + sp.text + out[sp.e:]
	}
	return out, nil
}
```

(`diag.go` needs `strings` and `sort` imports; `fmt` is already imported.)

```go
// internal/diag/render.go — jsonFix
type jsonFix struct {
	Line    int    `json:"line"`
	Col     int    `json:"col"`
	EndLine int    `json:"endLine"`
	EndCol  int    `json:"endCol"`
	Replace string `json:"replace"`
}
// and in RenderJSON: j.Fix = &jsonFix{d.Fix.Line, d.Fix.Col, d.Fix.EndLine, d.Fix.EndCol, d.Fix.Replace}
```

Producers — replace each literal `&diag.Fix{…}` with the helper that states its intent (line numbers are from M1's final head and may have drifted by a few lines; find by the surrounding code):

| Site | Code | New producer |
|---|---|---|
| `pattern.go` E0004/E0302 arm fix | `diag.InsertAt(m.Span.End.Line, 1, strings.Join(arms, "\n")+"\n")` (insert before the closing `}` line; add the trailing newline so the inserted arms sit on their own lines) |
| `decls.go` E0601 `Msg` → `a` | `diag.ReplaceSpan(t.Span, "a")` |
| `decls.go` E0007 signature | `diag.InsertAt(vd.Span.Start.Line, 1, vd.Name+" : "+ty+"\n")` |
| `expr.go` `Text.fromInt`/`fromFloat` | `diag.ReplaceSpan(sp, "Text.fromInt ("+src+")")` (and `fromFloat`) |
| `expr.go` E0301 bare message | `diag.ReplaceSpan(x.Span, full)` |
| `expr.go` E0102 Maybe field | `diag.ReplaceSpan(x.Span, fmt.Sprintf(...))` |
| `expr.go` E0101 Db.get | `diag.ReplaceSpan(x.Span, "Db.from …")` (the whole application) |
| `resolve.go` E0201 import | `diag.InsertAt(line, 1, "import "+x+"\n")` — note the added `\n` |
| `resolve.go` `App.Session` → `Session` | `diag.ReplaceSpan(t.Span, "Session")` |
| `resolve.go` E0202 qualify type | `diag.ReplaceSpan(t.Span, x+"."+t.Name)` |
| `resolve.go` E0203 unqualify ctor | `diag.ReplaceSpan(c.Span, c.Name)` |

Golden renderer in `internal/check/check_test.go` `render`: print `  fix %d:%d-%d:%d %s\n` with `d.Fix.Line, d.Fix.Col, d.Fix.EndLine, d.Fix.EndCol, d.Fix.Replace`. Because the E0201 and arm fixes now end with `\n`, the rendered line would wrap: print `strings.ReplaceAll(d.Fix.Replace, "\n", "⏎")` so goldens stay one line per fix.

- [ ] **Step 4: Regenerate goldens and review the diff**

Run: `go test ./internal/check/ -update` then `git diff --stat testdata/ && git diff testdata/ | grep '^[-+]  fix' | head -60`
Expected: every changed line is a `fix` line gaining `-EL:EC` (and `⏎` for the two newline-carrying fixes); no code/message line changes. If any other line changed, something regressed — stop and find out.

- [ ] **Step 5: Add an end-to-end contract test**

In `internal/check/check_test.go`:

```go
// TestFixesApply proves every corpus fix is well-formed: applying it to the source must not error
// and must change the file.
func TestFixesApply(t *testing.T) {
	dirs, _ := filepath.Glob("../../testdata/corpus/*")
	for _, dir := range dirs {
		bag := runDir(t, dir)
		for _, d := range bag.Items {
			if d.Fix == nil {
				continue
			}
			src, err := os.ReadFile(filepath.Join(dir, d.Span.File))
			if err != nil {
				t.Fatal(err)
			}
			out, err := diag.Apply(string(src), []diag.Fix{*d.Fix})
			if err != nil {
				t.Errorf("%s %s: fix does not apply: %v", dir, d.Code, err)
			} else if out == string(src) {
				t.Errorf("%s %s: fix changes nothing", dir, d.Code)
			}
		}
	}
}
```

Run: `go test ./internal/check/ -run TestFixesApply`
Expected: PASS. A failure here means a producer's span is wrong (e.g. E0101's span must cover the whole `Db.get Post params.slug`, not the argument); fix the producer, not the test.

- [ ] **Step 6: Book**

In `book/11-tooling.md` replace the JSON sample's `fix` with the new shape and add after it:

> A fix replaces the half-open span from `line:col` to `endLine:endCol` with `replace`; when the two positions are equal it is an insertion. Fixes never overlap.

- [ ] **Step 7: Full suite and commit**

Run: `gofmt -l . && go vet ./... && go test ./...`
Expected: gofmt prints nothing; all PASS.

```bash
git add -A && git commit -m "m2: fix contract: fixes carry an end position; Apply helper; goldens regenerated"
```

---

### Task 2: Per-module gating and positional message arguments

**Files:**
- Modify: `internal/names/graph.go` (`Program.Failed`, `FailedModules`), `internal/check/check.go` (skip failed modules in the body pass), `cmd/alder/main.go` and `internal/check/check_test.go` (pipeline), `internal/check/expr.go` (`inferApp`)
- Create: `testdata/golden/check_gating/app/{A,B,C}.alder` + `check_gating.diag`, `testdata/corpus/E0301-positional-arg/` + `expected.diag`, `experiments/corpus_notes.md` entry
- Modify: `book/11-tooling.md` (the gating paragraph)

**Interfaces:**
- Produces:
  ```go
  package names
  type Program struct { …; Failed map[string]bool }   // module name → resolution produced errors
  func (p *Program) MarkFailed(bag *diag.Bag)          // fills Failed from bag by Span.File → module; call after Resolve
  ```
  `check.Program(prog, bag)` reads `prog.Failed`: declaration passes run for all modules; `checkBodies` and page/App rules are skipped for failed modules (and `App` rules that need every page are still run, treating failed pages as absent).

- [ ] **Step 1: Write the gating golden**

`testdata/golden/check_gating/app/A.alder`:
```alder
module A

import B

x : Int
x = 1
```
`testdata/golden/check_gating/app/B.alder`:
```alder
module B

import A

y : Int
y = A.x
```
`testdata/golden/check_gating/app/C.alder`:
```alder
module C

z : Int
z = "not an int"
```
`testdata/golden/check_gating.diag`:
```
app/A.alder:3:1 E0006 import cycle: A -> B -> A
app/C.alder:4:5 E0003 expected Int, got Text
```
(The exact E0006 span/message is whatever `names` already emits for this cycle — copy it from the current output; the point of the golden is the presence of the E0003 line.)

Add the unit test in `internal/names/names_test.go`:

```go
func TestMarkFailed(t *testing.T) {
	a := parse(t, "app/A.alder", "module A\n\nx : Int\nx = nope\n")
	b := parse(t, "app/B.alder", "module B\n\ny : Int\ny = 1\n")
	prog, bag := Build([]*syntax.Module{a, b}, std(t))
	prog.Resolve(bag)
	prog.MarkFailed(bag)
	if !prog.Failed["A"] || prog.Failed["B"] {
		t.Fatalf("Failed = %v", prog.Failed)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/names/ -run TestMarkFailed; go test ./internal/check/ -run TestCheckGolden/check_gating`
Expected: FAIL — `MarkFailed` undefined; golden produces only the E0006 line.

- [ ] **Step 3: Implement gating**

```go
// internal/names/graph.go
// MarkFailed records which modules produced resolution errors, by diagnostic file.
func (p *Program) MarkFailed(bag *diag.Bag) {
	if p.Failed == nil {
		p.Failed = map[string]bool{}
	}
	byFile := map[string]string{}
	for name, m := range p.Modules {
		byFile[m.File] = name
	}
	for _, d := range bag.Items {
		if d.Severity != "error" {
			continue
		}
		if name, ok := byFile[d.Span.File]; ok {
			p.Failed[name] = true
		}
	}
}
```

In `check.Program`, the body pass becomes:

```go
for _, name := range prog.Order {
	if prog.Failed[name] {
		continue // resolution failed: its bodies would only cascade
	}
	c.ctx(prog.Modules[name]).checkBodies()
}
```

and wherever the App rules iterate pages (`checkApp`), skip `prog.Failed[page]`. In `cmd/alder/main.go` and `internal/check/check_test.go`'s `runDir`, replace the two `if !bag.HasErrors()` guards with:

```go
parseErrors := bag.HasErrors()
if !parseErrors {
	prog.Resolve(bag)
	prog.MarkFailed(bag)
	check.Program(prog, bag)
}
```

(`Load` already put parse diagnostics in `bag`; nothing else is in it at that point.)

- [ ] **Step 4: Positional message argument**

In `internal/check/expr.go` `inferApp`, after the "constructor applied to a record literal" block, add:

```go
// E0301: a message constructor applied to anything but a record literal
if ct, ok := x.Fn.(*syntax.CtorRef); ok && pipe == nil && ct.Ref.Kind == syntax.RefCtor {
	if info, found := c.ctors[c.key(ct.Ref.Module, ct.Ref.Name)]; found && info.Union.Name == "Msg" {
		if _, isLit := x.Args[0].(*syntax.RecordLit); !isLit {
			if rec, isRec := info.Arg.(*types.Record); isRec {
				var parts []string
				for _, f := range rec.Fields {
					if f.Name != "form" {
						parts = append(parts, f.Name+": ?"+f.Name)
					}
				}
				full := ct.Name + " { " + strings.Join(parts, ", ") + " }"
				c.errorf("E0301", x.Span, "message `%s` takes a record: `%s`", ct.Name, full).Fix = diag.ReplaceSpan(x.Span, full)
				for _, a := range x.Args {
					mc.infer(a, env, eff)
				}
				return info.Union
			}
		}
	}
}
```

Corpus case `testdata/corpus/E0301-positional-arg/` — a page with `msg Toggle { id: Todo.TodoId }` and `<button click={Toggle todo.id}>`; `expected.diag` asserts one E0301 "message `Toggle` takes a record: `Toggle { id: ?id }`" with the fix replacing the application span. Add the origin line to `experiments/corpus_notes.md` (observed in M1's final review; shape from E1 qwen3-coder T1 `Toggle todo.id` style).

- [ ] **Step 5: Book**

In `book/11-tooling.md` replace the sentence "The compiler stops after the first phase that reports errors (parse, then names, then types)…" with: "A file that does not parse stops the compiler. After that, every module is checked except those whose names could not be resolved: a cycle or a missing import in one module never hides type errors in another."

- [ ] **Step 6: Full suite and commit**

Run: `gofmt -l . && go vet ./... && go test ./...`
Expected: all PASS, including `check_gating` and the new corpus case.

```bash
git add -A && git commit -m "m2: per-module gating after names; positional message argument is E0301"
```

---

### Task 3: The harness — `run_loop.py`

**Files:**
- Create: `experiments/run_loop.py`, `experiments/report_loop.py`, `experiments/test_run_loop.py`
- Modify: `experiments/README.md` (harness section)

**Interfaces:**
- CLI:
  ```
  python3 experiments/run_loop.py --exp experiments/e5-loop --model ollama:qwen3-coder:30b --task t3-newapp [--retries 1] [--prompt prompt.md] [--alder path/to/alder]
  python3 experiments/report_loop.py experiments/e5-loop > experiments/e5-loop/scores.md
  ```
- Module API (used by the tests):
  ```python
  split_files(text: str) -> dict[str, str]          # "app/X.alder" → contents; tolerates ``` fences and preamble
  render_diags(diags: list[dict], sources: dict[str, str]) -> str   # text form with caret and fix lines
  retry_prompt(task: str, round_dir: Path, diags: list[dict], stderr: str, nfiles: int) -> str
  class Model: generate(system: str, user: str) -> tuple[str, dict]   # (text, meta{seconds, tokens?})
  OllamaModel(name), ClaudeModel(name), FakeModel(responses: list[str])
  run_check(alder: Path, app_root: Path) -> tuple[int, list[dict], str]   # (exit, diags, stderr)
  run_one(exp: Path, model: Model, model_label: str, task_name: str, task_text: str, system: str, alder: Path, retries: int) -> dict   # the summary
  ```
- Summary shape (`summary.json`):
  ```json
  {"model": "ollama:qwen3-coder:30b", "task": "t3-newapp", "retries": 1, "error": null,
   "rounds": [{"n": 0, "files": 9, "exit": 1, "errors": 3, "warnings": 0, "codes": {"E0101": 1, "E0003": 2}, "seconds": 41.2, "stderr": ""},
              {"n": 1, "files": 9, "exit": 0, "errors": 0, "warnings": 0, "codes": {}, "seconds": 38.0, "stderr": ""}],
   "final_pass": true}
  ```

- [ ] **Step 1: Write the failing tests**

```python
# experiments/test_run_loop.py
import json, os, pathlib, shutil, subprocess, sys, tempfile, unittest
sys.path.insert(0, str(pathlib.Path(__file__).resolve().parent))
import run_loop as rl

GOOD = "--- file: app/App.alder ---\nmodule App\n\nsession { }\n\nerror : Error -> Html Never\nerror err = <h1>x</h1>\n\nnotFound : Html Never\nnotFound = <h1>x</h1>\n"

class SplitTests(unittest.TestCase):
    def test_split_tolerates_fences_and_preamble(self):
        text = "Here is the app:\n```alder\n--- file: app/A.alder ---\nmodule A\n```\n\n```\n--- file: app/B/C.alder ---\nmodule B.C\n```\n"
        files = rl.split_files(text)
        self.assertEqual(sorted(files), ["app/A.alder", "app/B/C.alder"])
        self.assertEqual(files["app/A.alder"].strip(), "module A")
        self.assertEqual(files["app/B/C.alder"].strip(), "module B.C")

    def test_no_markers(self):
        self.assertEqual(rl.split_files("I cannot do this."), {})

class FakeChecker:
    """Writes a fake `alder` script whose exit code and JSON are scripted per call."""
    def __init__(self, tmp, script):
        self.path = pathlib.Path(tmp) / "alder"
        self.count = pathlib.Path(tmp) / "count"
        self.count.write_text("0")
        lines = ["#!/bin/sh", f'n=$(cat {self.count}); echo $((n+1)) > {self.count}']
        for i, (exit_code, stdout, stderr) in enumerate(script):
            lines.append(f'if [ "$n" = "{i}" ]; then printf %s {json.dumps(stdout)}; printf %s {json.dumps(stderr)} 1>&2; exit {exit_code}; fi')
        lines.append("exit 2")
        self.path.write_text("\n".join(lines) + "\n")
        self.path.chmod(0o755)

ERR = json.dumps({"code": "E0003", "severity": "error", "file": "app/App.alder", "line": 3, "col": 1, "endLine": 3, "endCol": 2, "message": "expected Int, got Text"}) + "\n"

class LoopTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.mkdtemp()
        self.exp = pathlib.Path(self.tmp) / "exp"
        (self.exp / "tasks").mkdir(parents=True)

    def tearDown(self):
        shutil.rmtree(self.tmp)

    def test_retry_overlays_previous_round(self):
        chk = FakeChecker(self.tmp, [(1, ERR, ""), (0, "", "")])
        model = rl.FakeModel([GOOD + "--- file: app/X.alder ---\nmodule X\n", "--- file: app/X.alder ---\nmodule X\n\nfixed : Int\nfixed = 1\n"])
        s = rl.run_one(self.exp, model, "fake", "t", "task text", "system", chk.path, retries=1)
        self.assertEqual([r["files"] for r in s["rounds"]], [2, 2])   # round 1 keeps App.alder from round 0
        self.assertTrue((self.exp / "runs/fake/t/round1/app/App.alder").exists())
        self.assertIn("fixed", (self.exp / "runs/fake/t/round1/app/X.alder").read_text())
        self.assertTrue(s["final_pass"])
        self.assertEqual(s["rounds"][0]["codes"], {"E0003": 1})

    def test_check_exit2_recorded_not_raised(self):
        chk = FakeChecker(self.tmp, [(2, "", "alder check: internal error: boom\n"), (2, "", "alder check: internal error: boom\n")])
        s = rl.run_one(self.exp, rl.FakeModel([GOOD, GOOD]), "fake", "t", "task", "sys", chk.path, retries=1)
        self.assertFalse(s["final_pass"])
        self.assertEqual(s["rounds"][0]["exit"], 2)
        self.assertIn("boom", s["rounds"][0]["stderr"])

    def test_no_files_round(self):
        chk = FakeChecker(self.tmp, [(2, "", "no .alder files\n"), (0, "", "")])
        model = rl.FakeModel(["Sorry, I cannot.", GOOD])
        s = rl.run_one(self.exp, model, "fake", "t", "task", "sys", chk.path, retries=1)
        self.assertEqual(s["rounds"][0]["files"], 0)
        self.assertIn("no files", model.prompts[1].lower())
        self.assertTrue(s["final_pass"])

    def test_retry_prompt_contains_rendered_diag_and_fix(self):
        diags = [json.loads(ERR.strip())]
        diags[0]["fix"] = {"line": 3, "col": 1, "endLine": 3, "endCol": 2, "replace": "Text.fromInt (x)"}
        rd = self.exp / "runs/fake/t/round0"; (rd / "app").mkdir(parents=True)
        (rd / "app/App.alder").write_text("module App\n\nx = 1\n")
        p = rl.retry_prompt("TASK", rd, diags, "", 1)
        self.assertIn("app/App.alder:3:1: error[E0003]: expected Int, got Text", p)
        self.assertIn("fix: Text.fromInt (x)", p)
        self.assertIn("TASK", p)
        self.assertIn("every file", p)

    def test_model_unavailable(self):
        class Down(rl.Model):
            def generate(self, system, user):
                raise rl.ModelError("connection refused")
        chk = FakeChecker(self.tmp, [])
        s = rl.run_one(self.exp, Down(), "down", "t", "task", "sys", chk.path, retries=1)
        self.assertIsNotNone(s["error"])
        self.assertFalse(s["final_pass"])
        self.assertEqual(s["rounds"], [])

class ReportTests(unittest.TestCase):
    def test_report_table(self):
        import report_loop as rp
        rows = rp.rows([{"model": "m", "task": "t", "error": None, "final_pass": True,
                         "rounds": [{"n": 0, "errors": 3, "codes": {"E0003": 2, "E0101": 1}, "files": 5, "exit": 1, "seconds": 1.0, "warnings": 0, "stderr": ""},
                                    {"n": 1, "errors": 0, "codes": {}, "files": 5, "exit": 0, "seconds": 1.0, "warnings": 0, "stderr": ""}]}])
        self.assertEqual(rows[0]["r0"], 3)
        self.assertEqual(rows[0]["r1"], 0)
        md = rp.markdown(rows)
        self.assertIn("| m | t | 3 | 0 | pass |", md)

if __name__ == "__main__":
    unittest.main()
```

- [ ] **Step 2: Run to verify failure**

Run: `cd experiments && python3 -m unittest test_run_loop -v 2>&1 | tail -3`
Expected: ImportError / AttributeError — `run_loop` has none of these names.

- [ ] **Step 3: Implement `run_loop.py`**

```python
#!/usr/bin/env python3
"""Generate → alder check → feed diagnostics back → retry.

Usage:
  run_loop.py --exp DIR --model ollama:NAME|claude:NAME|fake --task NAME [--retries 1] [--prompt prompt.md] [--alder BIN]

Writes DIR/runs/<model>/<task>/round<N>/{output.md,check.json,app/...} and DIR/runs/<model>/<task>/summary.json.
"""
import argparse, json, pathlib, re, shutil, subprocess, sys, time, urllib.request

MAX_OUTPUT = 200_000


class ModelError(Exception):
    pass


class Model:
    def generate(self, system: str, user: str):
        raise NotImplementedError


class FakeModel(Model):
    def __init__(self, responses):
        self.responses = list(responses)
        self.prompts = []

    def generate(self, system, user):
        self.prompts.append(user)
        if not self.responses:
            raise ModelError("fake model has no more responses")
        return self.responses.pop(0), {"seconds": 0.0}


class OllamaModel(Model):
    def __init__(self, name, host="http://localhost:11434"):
        self.name, self.host = name, host

    def generate(self, system, user):
        body = {"model": self.name, "stream": False, "options": {"num_ctx": 32768, "temperature": 0},
                "messages": [{"role": "system", "content": system}, {"role": "user", "content": user}]}
        t0 = time.time()
        req = urllib.request.Request(self.host + "/api/chat", data=json.dumps(body).encode(),
                                     headers={"Content-Type": "application/json"})
        try:
            with urllib.request.urlopen(req, timeout=3600) as r:
                resp = json.load(r)
        except Exception as e:  # connection refused, timeout, bad JSON
            raise ModelError(f"ollama {self.name}: {e}")
        text = re.sub(r"<think>.*?</think>\s*", "", resp["message"]["content"], flags=re.S)
        return text, {"seconds": round(time.time() - t0, 1), "tokens": resp.get("eval_count")}


class ClaudeModel(Model):
    TOOLS_OFF = "Bash,Read,Write,Edit,Glob,Grep,WebSearch,WebFetch,Agent,NotebookEdit"

    def __init__(self, name):
        self.name = name

    def generate(self, system, user):
        if shutil.which("claude") is None:
            raise ModelError("claude CLI not found")
        prompt = system + "\n\n====================  TASK  ====================\n" + user
        t0 = time.time()
        try:
            p = subprocess.run(["claude", "-p", "--model", self.name, "--output-format", "text",
                                "--no-session-persistence", "--disallowedTools", self.TOOLS_OFF],
                               input=prompt, capture_output=True, text=True, timeout=1800)
        except subprocess.TimeoutExpired:
            raise ModelError(f"claude {self.name}: timeout")
        if p.returncode != 0:
            raise ModelError(f"claude {self.name}: exit {p.returncode}: {p.stderr.strip()[:500]}")
        return p.stdout, {"seconds": round(time.time() - t0, 1)}


def make_model(spec: str) -> Model:
    kind, _, name = spec.partition(":")
    if kind == "ollama":
        return OllamaModel(name)
    if kind == "claude":
        return ClaudeModel(name)
    raise SystemExit(f"unknown model spec {spec!r}; use ollama:NAME or claude:NAME")


FILE_RE = re.compile(r"^-{3,}\s*file:\s*(\S+?)\s*-{3,}\s*$", re.M)


def split_files(text: str) -> dict:
    """Split `--- file: path ---` sections; strip surrounding ``` fences; ignore any preamble."""
    parts = FILE_RE.split(text)
    out = {}
    for i in range(1, len(parts), 2):
        path, body = parts[i], parts[i + 1]
        body = re.sub(r"^\s*```\w*\n", "", body)         # opening fence right after the marker
        body = re.sub(r"\n```\s*(\n|$)[\s\S]*$", "\n", body)  # closing fence and anything after it
        if path.startswith("app/") and path.endswith(".alder"):
            out[path] = body.strip("\n") + "\n"
    return out


def run_check(alder: pathlib.Path, app_root: pathlib.Path):
    p = subprocess.run([str(alder), "check", str(app_root), "--json"], capture_output=True, text=True)
    diags = []
    for line in p.stdout.splitlines():
        line = line.strip()
        if line.startswith("{"):
            try:
                diags.append(json.loads(line))
            except json.JSONDecodeError:
                pass
    return p.returncode, diags, p.stderr


def render_diags(diags, sources):
    out = []
    for d in diags:
        out.append(f"{d['file']}:{d['line']}:{d['col']}: {d['severity']}[{d['code']}]: {d['message']}")
        lines = sources.get(d["file"], "").split("\n")
        if 1 <= d["line"] <= len(lines):
            src = lines[d["line"] - 1]
            width = max(1, d["endCol"] - d["col"]) if d.get("endLine") == d["line"] else 1
            out.append("  " + src)
            out.append("  " + " " * max(0, d["col"] - 1) + "^" * width)
        if d.get("fix"):
            out.append("  fix: " + d["fix"]["replace"].replace("\n", "⏎"))
    return "\n".join(out)


def retry_prompt(task: str, round_dir: pathlib.Path, diags, stderr: str, nfiles: int) -> str:
    sources = {}
    for f in (round_dir / "app").rglob("*.alder"):
        sources[str(f.relative_to(round_dir))] = f.read_text()
    if nfiles == 0:
        feedback = "Your previous answer contained no files (no `--- file: app/... ---` sections were found)."
    elif diags:
        feedback = "The compiler reported these errors in your previous answer:\n\n" + render_diags(diags, sources)
    else:
        feedback = "The compiler could not check your previous answer:\n\n" + stderr.strip()
    return (task + "\n\n====================  COMPILER FEEDBACK  ====================\n" + feedback +
            "\n\nFix every error. Output every file again, in full, each preceded by `--- file: app/<path> ---`. No explanation, only files.")


def write_round(round_dir: pathlib.Path, prev_dir, text: str, files: dict):
    (round_dir / "app").mkdir(parents=True, exist_ok=True)
    if prev_dir is not None and (prev_dir / "app").exists():
        shutil.copytree(prev_dir / "app", round_dir / "app", dirs_exist_ok=True)
    (round_dir / "output.md").write_text(text[:MAX_OUTPUT])
    for path, body in files.items():
        p = round_dir / path
        p.parent.mkdir(parents=True, exist_ok=True)
        p.write_text(body)
    return sum(1 for _ in (round_dir / "app").rglob("*.alder"))


def run_one(exp: pathlib.Path, model: Model, model_label: str, task_name: str, task_text: str,
            system: str, alder: pathlib.Path, retries: int = 1) -> dict:
    run_dir = exp / "runs" / model_label / task_name
    if run_dir.exists():
        shutil.rmtree(run_dir)
    summary = {"model": model_label, "task": task_name, "retries": retries, "error": None, "rounds": [], "final_pass": False}
    user = task_text
    prev = None
    for n in range(retries + 1):
        try:
            text, meta = model.generate(system, user)
        except ModelError as e:
            summary["error"] = str(e)
            break
        round_dir = run_dir / f"round{n}"
        files = split_files(text)
        nfiles = write_round(round_dir, prev, text, files)
        exit_code, diags, stderr = run_check(alder, round_dir)
        (round_dir / "check.json").write_text("\n".join(json.dumps(d) for d in diags) + ("\n" if diags else ""))
        codes = {}
        for d in diags:
            if d["severity"] == "error":
                codes[d["code"]] = codes.get(d["code"], 0) + 1
        summary["rounds"].append({"n": n, "files": nfiles, "exit": exit_code,
                                  "errors": sum(codes.values()), "warnings": sum(1 for d in diags if d["severity"] == "warning"),
                                  "codes": codes, "seconds": meta.get("seconds", 0.0), "tokens": meta.get("tokens"), "stderr": stderr[:2000]})
        if exit_code == 0:
            summary["final_pass"] = True
            break
        user = retry_prompt(task_text, round_dir, diags, stderr, nfiles)
        prev = round_dir
    run_dir.mkdir(parents=True, exist_ok=True)
    (run_dir / "summary.json").write_text(json.dumps(summary, indent=1) + "\n")
    return summary


def main(argv=None):
    ap = argparse.ArgumentParser()
    ap.add_argument("--exp", required=True)
    ap.add_argument("--model", required=True, action="append", help="ollama:NAME or claude:NAME; repeatable")
    ap.add_argument("--task", required=True, action="append", help="task name (file tasks/<name>.md); repeatable")
    ap.add_argument("--retries", type=int, default=1)
    ap.add_argument("--prompt", default="prompt.md")
    ap.add_argument("--alder", default=None, help="path to the alder binary (default: build it)")
    a = ap.parse_args(argv)
    exp = pathlib.Path(a.exp)
    system = (exp / a.prompt).read_text()
    alder = pathlib.Path(a.alder) if a.alder else build_alder(exp)
    failed = 0
    for spec in a.model:
        model = make_model(spec)
        label = spec.replace(":", "-").replace("/", "-")
        for task in a.task:
            task_text = (exp / "tasks" / f"{task}.md").read_text()
            s = run_one(exp, model, label, task, task_text, system, alder, a.retries)
            status = "ERROR " + s["error"] if s["error"] else ("pass" if s["final_pass"] else "fail")
            counts = " → ".join(str(r["errors"]) for r in s["rounds"])
            print(f"{label} {task}: {status} ({counts} errors)")
            if s["error"]:
                failed += 1
    return 1 if failed else 0


def build_alder(exp: pathlib.Path) -> pathlib.Path:
    root = pathlib.Path(__file__).resolve().parent.parent
    out = exp / "alder"
    subprocess.run(["go", "build", "-o", str(out), "./cmd/alder"], cwd=root, check=True)
    return out


if __name__ == "__main__":
    sys.exit(main())
```

And `report_loop.py`:

```python
#!/usr/bin/env python3
"""Aggregate run_loop summaries into a Markdown table.  Usage: report_loop.py EXP_DIR"""
import json, pathlib, sys


def rows(summaries):
    out = []
    for s in summaries:
        r = {"model": s["model"], "task": s["task"], "error": s["error"], "pass": s["final_pass"],
             "r0": None, "r1": None, "codes0": {}, "codes1": {}}
        for rd in s["rounds"]:
            r[f"r{rd['n']}"] = rd["errors"]
            r[f"codes{rd['n']}"] = rd["codes"]
        out.append(r)
    return out


def fmt_codes(c):
    return ", ".join(f"{k}×{v}" if v > 1 else k for k, v in sorted(c.items())) or "—"


def markdown(rs):
    lines = ["| Model | Task | Round 0 errors | Round 1 errors | Result | Round 0 codes | Round 1 codes |",
             "|---|---|---|---|---|---|---|"]
    for r in rs:
        if r["error"]:
            lines.append(f"| {r['model']} | {r['task']} | — | — | ERROR: {r['error'][:60]} | | |")
            continue
        r1 = "—" if r["r1"] is None else r["r1"]
        lines.append(f"| {r['model']} | {r['task']} | {r['r0']} | {r1} | {'pass' if r['pass'] else 'fail'} | {fmt_codes(r['codes0'])} | {fmt_codes(r['codes1'])} |")
    return "\n".join(lines) + "\n"


def main():
    exp = pathlib.Path(sys.argv[1])
    summaries = [json.loads(p.read_text()) for p in sorted(exp.glob("runs/*/*/summary.json"))]
    print(markdown(rows(summaries)), end="")


if __name__ == "__main__":
    main()
```

- [ ] **Step 4: Run the tests**

Run: `cd experiments && python3 -m unittest test_run_loop -v`
Expected: all PASS (7 tests).

- [ ] **Step 5: Smoke the real path once**

Run: `cd /Users/tavishpegram/projects/alder && mkdir -p experiments/e5-loop/tasks && cp experiments/e4-msgpath/tasks/*.md experiments/e4-msgpath/../e2-revised/tasks/t2-newpage.md experiments/e5-loop/tasks/ && python3 experiments/build_context.py > experiments/e5-loop/prompt.md && python3 experiments/run_loop.py --exp experiments/e5-loop --model claude:haiku --task t4-appmsg --retries 1`
Expected: one line like `claude-haiku t4-appmsg: pass (0 errors)` or `fail (N → M errors)`, and `experiments/e5-loop/runs/claude-haiku/t4-appmsg/summary.json` exists. Delete that run directory afterwards (`rm -rf experiments/e5-loop/runs`) so E5 starts clean; keep `prompt.md` and `tasks/`. Add `experiments/*/alder` to `.gitignore`.

- [ ] **Step 6: README and commit**

Add to `experiments/README.md` a "Harness (M2)" section: the two commands, the summary shape, the pass rule, and that scores are now generated (`report_loop.py`), with hand-scoring reserved for design signals.

```bash
git add -A && git commit -m "m2: run_loop harness: generate, check, feed back, retry; report aggregator"
```

---

### Task 4: Experiment E5 — the first measurement with a compiler

**Files:**
- Create: `experiments/e5-loop/runs/**` (generated), `experiments/e5-loop/scores.md` (generated), `experiments/e5-loop/findings.md`
- Modify: `book/12-rationale.md` (E5 entry replaces the planned-M2 text), `experiments/README.md` (experiment list)

**Interfaces:** consumes Task 3's CLI only.

- [ ] **Step 1: Run the matrix**

Run (this is the experiment; it takes roughly 15–25 minutes, mostly qwen):
```bash
cd /Users/tavishpegram/projects/alder
python3 experiments/run_loop.py --exp experiments/e5-loop --retries 1 \
  --model ollama:qwen3-coder:30b --model claude:haiku --model claude:sonnet \
  --task t2-newpage --task t3-newapp --task t4-appmsg
python3 experiments/report_loop.py experiments/e5-loop > experiments/e5-loop/scores.md
```
Expected: 9 lines of `model task: pass/fail (a → b errors)`; `scores.md` has a 9-row table. If a run reports ERROR (Ollama down, CLI missing), fix the environment and re-run only that model/task (the CLI accepts single `--model`/`--task`).

- [ ] **Step 2: Read every failing final round**

For each run with `final_pass: false`, read `round1/check.json` and the diff between `round0/app` and `round1/app`. Classify, per run, in `findings.md`: (a) the model fixed what the compiler named but introduced a new error; (b) the model ignored a diagnostic; (c) the compiler's message or fix was misleading (quote it); (d) the compiler was wrong (a false positive — file it under "compiler bugs found by E5" with the reproducing source). Also read two passing runs in full to confirm "pass" means a sensible app and not an empty one (check `files` ≥ 6 for T3/T4).

- [ ] **Step 3: Write findings.md**

Structure:
1. Headline table: for each model, round-0 errors, round-1 errors, pass rate, compared with E4's hand-scored numbers for the same tasks (E4 scores: Haiku 0 errors on T3/T4; qwen3-coder 3 on T3, 0 on T4; Sonnet 0 on T3).
2. What one retry buys: for each model the fraction of round-0 errors removed by round 1, and which codes survive a retry.
3. Compiler problems found (misleading messages, false positives) — each one becomes an issue for M3 or a book change now if it is a documentation gap.
4. Harness notes: timing per round, any model-output quirks the splitter handled.
5. What this changes in the plan.

- [ ] **Step 4: Book and README**

In `book/12-rationale.md`, replace the "### E2 … *Status: next.*"-style placeholder for M2 (the section after "Where the spike ended") with an "### E5: Checker in the loop" entry: date, matrix, the headline table, and three sentences on what it showed. In `experiments/README.md`, add E5 to the experiment list with one line.

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "e5: checker-in-the-loop measurement on T2-T4; findings and rationale"
```

---

## Self-review notes

**Spec coverage.** "Decisions recorded after M1": fix contract → Task 1 (type, JSON, every producer, `Apply`, goldens, book); per-module gating → Task 2 (`MarkFailed`, `Program` skip, pipeline, golden); positional argument → Task 2 (E0301 + corpus); `claude -p` driving and automatic scoring → Task 3. "Milestones: M2" acceptance ("results recorded in chapter 12 as E5") → Task 4. "Testing: Harness" (generate → check → feed back → one retry, errors before and after) → Task 3 summary shape and Task 4.

**Placeholder scan.** None: every code step has its code; Task 4's "expected" lines describe outputs, not code.

**Type consistency.** `diag.Fix{Line, Col, EndLine, EndCol, Replace}` is used by `ReplaceSpan`/`InsertAt`/`Apply` (Task 1) and read by `render_diags` in Python via the JSON keys `line/col/endLine/endCol/replace` (Task 3) — same names as `jsonFix`. `Program.Failed`/`MarkFailed` (Task 2) are used by `check.Program`, `main.go` and `runDir`. `run_one` returns the summary shape `report_loop.rows` consumes.

**Review Focus coverage.** (1) fences/preamble → Task 3 `test_split_tolerates_fences_and_preamble`; (2) overlay on retry → `test_retry_overlays_previous_round`; (3) exit 2 recorded → `test_check_exit2_recorded_not_raised`; (4) no files → `test_no_files_round`; (5) model unavailable → `test_model_unavailable`.

**Deliberate limits.** One retry only (the spec's M2). No parallel runs (Ollama serializes anyway; Claude CLI runs are cheap). The harness does not apply fixes itself — the model is given the fix text and decides; `diag.Apply` exists to validate producers and for a future `alder fix`.
