# M6c — test/mutate quickfix and REPL `--in` Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `alder test` failures and `--mutate` survivors land in Neovim's quickfix at their source position, and a key opens `alder repl --in <Module>` in a split that speaks as the module being edited does.

**Architecture:** The compiler records each `test` declaration's position through IR → generated `rtesting.Case` → `CaseResult` → `--json`. `scratch.Check` gains an *in-module* mode that overlays the module's file with its own source plus the session's items (`names.LoadOverlay`, already used by the LSP), so `alder eval --in M` and `alder repl --in M` resolve the module's names unqualified. alder.nvim adds `test.lua` (quickfix from `alder test --json` / `--mutate --json`, through the same `cli.run` → `setqflist` path as `check.lua`) and `repl.lua` (a terminal split over `alder repl`, with a send key).

**Tech Stack:** Go 1.2x (alder), Lua / Neovim 0.11 (alder.nvim), the custom headless test harness `nvim --headless -u tests/minimal.lua -l tests/run.lua`.

**Spec:** `docs/superpowers/specs/2026-10-19-m6c-test-quickfix-repl-design.md`

## Global Constraints

- Two repos: `~/projects/alder` (Go) and `~/projects/alder.nvim` (Lua). alder.nvim's CI builds alder from `tapegram/alder` **main**, so Tasks 1–5 (alder) are merged and pushed before Tasks 6–10 (alder.nvim) run in CI; locally, rebuild `~/go/bin/alder` (`go build -o ~/go/bin/alder ./cmd/alder`) after Task 5 and run the alder.nvim suite with `PATH=$HOME/go/bin:$PATH`.
- The human test report is **unchanged** (the book quotes it); only `--json` gains fields.
- `--in` and `--module` together are a usage error (exit 2); `--module` keeps its meaning (names the scratch module of app-scope mode).
- File paths in JSON are relative to the app directory, slash-separated (`app/Todo.alder`), as diagnostics and mutants already give them.
- Keys: `test = '<leader>at'`, `test_at = '<leader>aT'`, `mutate = '<leader>am'`, `repl = '<leader>ax'`, `send = '<leader>as'`; each can be `false`; buffer-local; `<leader>ar` stays `run`.
- Quickfix lists are replaced only when the current list is ours (by title); a user's list is never overwritten (the rule `check.lua`'s `set_qf` has).
- alder: `scripts/test.sh` green. alder.nvim: the suite green with the rebuilt binary. Book code blocks are verified by `book/book_test.go`: a `alder` block must check; use `text` blocks for transcripts.
- Commits end with the attribution lines the session reminder gives.
- No agent or background work 9am–5pm America/New_York on weekdays.

## Review Focus

1. A test name holding a regex metacharacter or a quote (`test "a (b) \"c\""`): `<leader>aT` must still select exactly that test — pinned in Task 7 (`run_pattern` escapes Go regexp metacharacters and the name's quotes stay literal).
2. `--in Todo` when `Todo.alder` has no trailing newline, or ends in a comment: the session's first line must still start a new declaration — pinned in Task 2 (`Check` appends `\n` when the file lacks one; test with a file ending in `-- comment` without newline).
3. A diagnostic *inside the module's own text* while in `--in` mode (the user broke `Todo.alder`): must be reported as `app/Todo.alder:L:C`, never mapped into the input — pinned in Task 2 (`MapDiag` leaves lines ≤ `BaseLines` alone) and Task 3 (eval prints the app diagnostic and exits 1).
4. `:AlderTest` with unsaved buffers: the run reads disk, so modified `.alder` buffers of the app are written first; a buffer that cannot be written (`nomodifiable`, write error) must not abort the run silently — pinned in Task 6 (`write_app_buffers` returns the names it could not write; the run proceeds with a WARN notification).
5. `send` with no REPL running: the REPL's banner takes a second to appear; text sent before it is lost by a plain `chansend` — pinned in Task 9 (queue until the banner line arrives on `on_stdout`, then flush; test by sending immediately after `open`).

---

## File map

alder:
- Modify `internal/ir/ir.go` (Test: File, Line, Col), `internal/ir/lower.go` (fill them), `internal/gen/caps.go` (emit them), `internal/gen/gen_test.go`.
- Modify `rt/testing/testing.go` (Case, CaseResult, jsonResult, runCase, timeout result), `cmd/alder/test_test.go`.
- Modify `internal/buildkit/buildkit.go` (`LoadCheckedOverlayWith`), `internal/scratch/scratch.go` (Module.In, Checked.BaseLines/Declares, Check in-module branch), `internal/scratch/lines.go` (Build.BaseLines, MapDiag), `internal/scratch/lines_test.go`, `internal/scratch/scratch_test.go` (or a new `inmodule_test.go`).
- Modify `cmd/alder/eval.go`, `cmd/alder/eval_test.go`, `cmd/alder/repl.go`, `cmd/alder/repl_test.go`, `cmd/alder/helpjson.go`, `cmd/alder/helpjson_test.go`.
- Modify `book/11-tooling.md`, `book/10-testing.md`.

alder.nvim:
- Create `lua/alder/buf.lua` (module name and test-at-cursor detection: tree-sitter with regex fallback), `lua/alder/test.lua` (test + mutate runs → quickfix), `lua/alder/repl.lua` (terminal REPL, send), `tests/buf_spec.lua`, `tests/test_spec.lua`, `tests/mutate_spec.lua`, `tests/repl_spec.lua`.
- Modify `lua/alder/check.lua` (export `set_qf(items, title)`), `lua/alder/config.lua`, `lua/alder/keys.lua` (modes per action), `lua/alder/health.lua`, `plugin/alder.lua` (commands), `tests/keys_spec.lua`, `tests/health_spec.lua`, `README.md`.

---

### Task 1: Test positions through IR, gen, rtesting and `--json`

**Files:**
- Modify: `internal/ir/ir.go:165-169`, `internal/ir/lower.go:63-82`, `internal/gen/caps.go:300-310`, `rt/testing/testing.go:196-215, 350, 454-480, 682`
- Test: `internal/gen/gen_test.go:3104`, `cmd/alder/test_test.go:107`

**Interfaces:**
- Produces: `rtesting.Case{Module, Name, File string; Line, Col int; Pure; Task}`, `rtesting.CaseResult{…, File string; Line, Col int}`, JSON objects with `"file"`, `"line"`, `"col"` after `"name"`.

- [ ] **Step 1: Write the failing gen test**

In `internal/gen/gen_test.go`, find the `contains(t, files["app__alder.go"], "var Tests = []rtesting.Case{\n", `{Module: "Shop", Name: "total adds", Pure: test__Shop__1},` …)` assertion near line 3104 and change the two case expectations to include positions. The e2e program in that test declares `test "total adds"` in module `Shop`; read the test's source literal above line 3104 to find the line the `test` keyword is on (call it `L1`, column 1) and the line of `"a failed run undoes its writes"` in `Shop.Orders` (`L2`):

```go
contains(t, files["app__alder.go"], "var Tests = []rtesting.Case{\n",
    fmt.Sprintf(`{Module: "Shop", Name: "total adds", File: "app/Shop.alder", Line: %d, Col: 1, Pure: test__Shop__1},`, L1),
    fmt.Sprintf(`{Module: "Shop.Orders", Name: "a failed run undoes its writes", File: "app/Shop/Orders.alder", Line: %d, Col: 1, Task: test__Shop__Orders__1}`, L2),
    "func TestSession() any {", "func (Caps) Registry() map[string]any {", "reg := caps.Registry()")
```

(If the test builds its program from an in-memory map whose file names differ from `app/<Mod>.alder`, use the names that map uses — the generated `File:` is the span's file verbatim.)

- [ ] **Step 2: Run it to see it fail**

Run: `cd ~/projects/alder && go test ./internal/gen -run 'Tests' -count=1`
Expected: FAIL, the generated table lacks `File:`.

- [ ] **Step 3: Carry the position through IR and gen**

`internal/ir/ir.go`:

```go
type Test struct {
	Module, Name string
	File         string // the test's file (as the span names it, "app/Todo.alder") and the
	Line, Col    int    // position of its `test` keyword, for alder test --json
	Body         Expr
	Effect       bool
}
```

`internal/ir/lower.go`, in `lowerTests`:

```go
test := &Test{Module: name, Name: td.Name, Effect: effect, Body: l.expr(td.Body),
	File: td.Span.File, Line: td.Span.Start.Line, Col: td.Span.Start.Col}
```

`internal/gen/caps.go`, in `testsTable`:

```go
cases = append(cases, fmt.Sprintf("{Module: %s, Name: %s, File: %s, Line: %d, Col: %d, %s: %s}",
	strconv.Quote(t.Module), strconv.Quote(t.Name), strconv.Quote(t.File), t.Line, t.Col, kind, testFunc(t.Module, i+1)))
```

- [ ] **Step 4: Extend rtesting**

`rt/testing/testing.go`:

```go
// Case is one `test` declaration: exactly one of Pure and Task is set. File, Line and Col are
// the position of its `test` keyword (File relative to the app, "app/Todo.alder"), for --json.
type Case struct {
	Module, Name string
	File         string
	Line, Col    int
	Pure         func() bool
	Task         func() eff.Task[bool]
}
```

Add the same three fields to `CaseResult` after `Module, Name string`, with the comment `// the Case's position, for --json`. In `runCase` (line ~350) and the timeout result (line ~682) copy them:

```go
r = CaseResult{Module: c.Module, Name: c.Name, File: c.File, Line: c.Line, Col: c.Col}
```
```go
r = CaseResult{Module: c.Module, Name: c.Name, File: c.File, Line: c.Line, Col: c.Col, Error: "timed out after " + timeout, Ms: float64(time.Since(start).Microseconds()) / 1000}
```

`jsonResult` gains, after `Name`:

```go
File   string   `json:"file"`
Line   int      `json:"line"`
Col    int      `json:"col"`
```

and the positional literal in `WriteResult` becomes named:

```go
return enc.Encode(jsonResult{Module: r.Module, Name: r.Name, File: r.File, Line: r.Line, Col: r.Col, OK: r.OK,
	Left: r.Left, Right: r.Right, Note: r.Note, Error: r.Error, Trace: tr, Ms: r.Ms})
```

Search `rt/testing` and `cmd/alder` for other `CaseResult{` literals (`grep -rn 'CaseResult{' rt cmd`) and leave the ones with no Case at hand (a crashed program's synthetic result) as they are — they report `line: 0`, which the plugin treats as "no position".

- [ ] **Step 5: Run the gen test and the rtesting tests**

Run: `go test ./internal/gen ./rt/testing -count=1`
Expected: PASS.

- [ ] **Step 6: Pin the JSON fields in the CLI test**

In `cmd/alder/test_test.go` `TestTest_JSON`, extend the decoded struct and the per-line check:

```go
var r struct {
	Module, Name, File string
	Line, Col          int
	OK                 *bool
	Trace              []string
}
```
and after the existing assertions on each line:
```go
if r.File == "" || !strings.HasPrefix(r.File, "app/") || r.Line <= 0 || r.Col <= 0 {
	t.Errorf("no position in %s", l)
}
```

Run: `go test ./cmd/alder -run TestTest_JSON -count=1` — Expected: PASS (it fails before Step 4 only if run early; run it now).

- [ ] **Step 7: Book**

`book/11-tooling.md` line ~929: the `--json` example object gains `"file":"app/Todos/Index.alder","line":<the line of that test in examples/todos>,"col":1` after `"name":…` (look the line up with `grep -n 'Toggle flips done' examples/todos/app/Todos/Index.alder`). Just above it, where the fields are listed, add: *`file`, `line` and `col` are the position of the `test` keyword, for editors.* `book/10-testing.md` ~L626-643 (where `--json` is introduced): add one sentence, *Each object also carries the test's `file`, `line` and `col`, so an editor can jump to it.*

Run: `go test ./book -count=1` — Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/ir internal/gen rt/testing cmd/alder/test_test.go book/11-tooling.md book/10-testing.md
git commit -m "test: carry each test's position into rtesting.Case and --json"
```

---

### Task 2: `scratch` in-module mode (`Module.In`)

**Files:**
- Modify: `internal/buildkit/buildkit.go:55-59`, `internal/scratch/scratch.go:33-37, 60-75, 346-362, 396-450`, `internal/scratch/lines.go:17-60`
- Test: `internal/scratch/lines_test.go`, new `internal/scratch/inmodule_test.go`

**Interfaces:**
- Produces: `scratch.Module{Name, Source string; Infer bool; In string}`; `Build.BaseLines int`, `Checked.BaseLines int`; `Checked.Declares(name string) bool`; `buildkit.LoadCheckedOverlayWith(dir string, overlay map[string][]byte, o check.Options)`; `scratch.ModulePath(name string) string` (`"app/Todos/Index.alder"`); `*scratch.NoModuleError{Name, App string}` with `Error()` = `no module Todo in <app>`.

- [ ] **Step 1: Write the failing tests**

`internal/scratch/inmodule_test.go`:

```go
package scratch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"alder/internal/diag"
)

// todos copies examples/todos to a temp dir so a test may edit it.
func todos(t *testing.T) string {
	t.Helper()
	dst := t.TempDir()
	if err := os.CopyFS(dst, os.DirFS("../../examples/todos")); err != nil {
		t.Fatal(err)
	}
	return dst
}

func TestCheck_InModuleSeesUnqualifiedNames(t *testing.T) {
	c := Check("../../examples/todos", &Module{In: "Todo", Source: "it = countOpen []\n", Infer: true})
	if c.Err != nil || len(c.Diags) != 0 || c.Result == nil {
		t.Fatalf("err %v, diags %v", c.Err, c.Diags)
	}
	if _, ok := c.Result.Values["Todo.it"]; !ok {
		t.Error("Todo.it was not inferred")
	}
	if c.ExtraFile != "app/Todo.alder" || c.BaseLines == 0 {
		t.Errorf("ExtraFile %q, BaseLines %d", c.ExtraFile, c.BaseLines)
	}
}

func TestCheck_InModuleMapsInputDiagnostics(t *testing.T) {
	c := Check("../../examples/todos", &Module{In: "Todo", Source: "it = countOpen 1\n", Infer: true})
	var got []diag.Diagnostic
	for _, d := range c.Diags {
		m, ok := c.MapDiag(d)
		if ok {
			got = append(got, m)
		}
	}
	if len(got) == 0 || got[0].Span.Start.Line != 1 {
		t.Fatalf("want the error on input line 1, got %+v", got)
	}
}

func TestCheck_InModuleKeepsModuleDiagnostics(t *testing.T) {
	dir := todos(t)
	p := filepath.Join(dir, "app", "Todo.alder")
	b, _ := os.ReadFile(p)
	os.WriteFile(p, append(b, []byte("\nbroken : Int\nbroken = \"x\"\n")...), 0o644)
	c := Check(dir, &Module{In: "Todo", Source: "it = 1\n", Infer: true})
	var found bool
	for _, d := range c.Diags {
		m, ok := c.MapDiag(d)
		if ok && m.Span.File == "app/Todo.alder" && m.Span.Start.Line <= c.BaseLines && m.Code == "E0003" {
			found = true
		}
	}
	if !found {
		t.Errorf("the module's own error was not reported at its line: %+v", c.Diags)
	}
}

func TestCheck_InModuleNoTrailingNewline(t *testing.T) {
	dir := todos(t)
	p := filepath.Join(dir, "app", "Todo.alder")
	b, _ := os.ReadFile(p)
	os.WriteFile(p, append([]byte(strings.TrimRight(string(b), "\n")), []byte("\n-- trailing comment")...), 0o644)
	c := Check(dir, &Module{In: "Todo", Source: "it = countOpen []\n", Infer: true})
	if c.Err != nil || len(c.Diags) != 0 {
		t.Fatalf("err %v, diags %v", c.Err, c.Diags)
	}
}

func TestCheck_InModuleDeclares(t *testing.T) {
	c := Check("../../examples/todos", &Module{In: "Todo", Source: "it = 1\n", Infer: true})
	if !c.Declares("countOpen") || c.Declares("it") || c.Declares("nope") {
		t.Error("Declares answers for the module's own values only")
	}
}

func TestCheck_InUnknownModule(t *testing.T) {
	c := Check("../../examples/todos", &Module{In: "Nope", Source: "it = 1\n", Infer: true})
	var e *NoModuleError
	if !errors.As(c.Err, &e) || e.Name != "Nope" {
		t.Errorf("want NoModuleError, got %v", c.Err)
	}
}
```

(`"errors"` goes in the imports.) In `lines_test.go` add:

```go
func TestMapDiag_BaseLines(t *testing.T) {
	b := Build{ExtraFile: "app/Todo.alder", BaseLines: 40, InsertLine: 1, LineOffset: 40}
	own := diag.Diagnostic{Span: diag.Span{File: "app/Todo.alder", Start: diag.Pos{Line: 7, Col: 1}, End: diag.Pos{Line: 7, Col: 3}}}
	if m, ok := b.MapDiag(own); !ok || m.Span.Start.Line != 7 {
		t.Errorf("a module line is left alone: %+v %v", m, ok)
	}
	in := diag.Diagnostic{Span: diag.Span{File: "app/Todo.alder", Start: diag.Pos{Line: 42, Col: 1}, End: diag.Pos{Line: 42, Col: 3}}}
	if m, ok := b.MapDiag(in); !ok || m.Span.Start.Line != 2 || m.Span.End.Line != 2 {
		t.Errorf("an input line maps to line-BaseLines: %+v %v", m, ok)
	}
}
```

- [ ] **Step 2: Run them to see them fail**

Run: `go test ./internal/scratch -run 'InModule|BaseLines|InUnknown' -count=1`
Expected: compile errors (`In`, `BaseLines`, `Declares`, `NoModuleError` undefined).

- [ ] **Step 3: buildkit overlay with options**

`internal/buildkit/buildkit.go`, after `LoadCheckedOverlay`:

```go
// LoadCheckedOverlayWith is LoadCheckedOverlay with checker options (the in-module mode of
// alder eval and the REPL: the module's file overlaid with the session appended).
func LoadCheckedOverlayWith(dir string, overlay map[string][]byte, o check.Options) (prog *names.Program, res *check.Result, bag *diag.Bag, ioErrs []error) {
	return loadChecked(dir, overlay, nil, o)
}
```

- [ ] **Step 4: scratch types**

`internal/scratch/scratch.go`:

```go
type Module struct {
	Name, Source string
	Infer        bool
	// In names an app module the Source is appended to instead of being a module of its own
	// (`alder eval --in Todo`): the session sees the module's declarations and imports
	// unqualified, as its own code does. Name is ignored; Infer applies to that module.
	In string
}

// NoModuleError is Check's error when Module.In names no module of the app.
type NoModuleError struct{ Name, App string }

func (e *NoModuleError) Error() string { return "no module " + e.Name + " in " + e.App }

// ModulePath is the file of an app module: "app/Todos/Index.alder" for Todos.Index.
func ModulePath(name string) string { return "app/" + strings.ReplaceAll(name, ".", "/") + ".alder" }
```

Add to `Build` (after `ExtraFile`) and to `Checked` (same place):

```go
// BaseLines is, in in-module mode (Module.In), the number of lines of the module's own file:
// a diagnostic of ExtraFile at a line past BaseLines is the caller's, at line L-BaseLines;
// one at or before it is the module's own and is reported as it is. 0 otherwise.
BaseLines int
```

and to `Checked` a private `baseDecls map[string]bool` plus:

```go
// Declares reports whether the module of in-module mode declares value name itself (not
// through the session); false outside that mode.
func (c Checked) Declares(name string) bool { return c.baseDecls[name] }
```

Extend `Checked.MapDiag` to pass `BaseLines`:

```go
return Build{LineOffset: c.LineOffset, InsertLine: c.InsertLine, ExtraFile: c.ExtraFile, BaseLines: c.BaseLines}.MapDiag(d)
```

and wherever `CompileCheckedWith` copies `Checked` fields into `Build` (grep `ExtraFile:` in scratch.go) add `BaseLines: c.BaseLines`.

- [ ] **Step 5: `Check`'s in-module branch**

In `Check`, before `if extra != nil {`, add the branch and make the existing one `else if`:

```go
var overlay map[string][]byte
if extra != nil && extra.In != "" {
	path := ModulePath(extra.In)
	raw, err := os.ReadFile(filepath.Join(appDir, filepath.FromSlash(path)))
	if err != nil {
		if os.IsNotExist(err) {
			c.Err = &NoModuleError{Name: extra.In, App: appDir}
		} else {
			c.Err = err
		}
		return c
	}
	base := string(raw)
	if !strings.HasSuffix(base, "\n") {
		base += "\n"
	}
	c.BaseLines = strings.Count(base, "\n")
	c.ExtraFile, c.InsertLine, c.LineOffset = path, 1, c.BaseLines
	if m, ds := syntax.ParseFile(path, raw); len(ds) == 0 {
		c.baseDecls = map[string]bool{}
		for _, d := range m.Decls {
			if v, ok := d.(*syntax.ValueDecl); ok {
				c.baseDecls[v.Name] = true
			}
		}
	}
	if extra.Infer {
		opts.InferModule = extra.In
	}
	overlay = map[string][]byte{path: []byte(base + extra.Source)}
} else if extra != nil {
	… (the existing body, unchanged)
}
```

Replace the load call:

```go
var prog *names.Program
var res *check.Result
var bag *diag.Bag
var ioErrs []error
if overlay != nil {
	prog, res, bag, ioErrs = buildkit.LoadCheckedOverlayWith(appDir, overlay, opts)
} else {
	prog, res, bag, ioErrs = buildkit.LoadCheckedWith(appDir, mods, opts)
}
```

The `len(prog.Modules) == len(mods)` "no .alder files" guard stays (mods is empty in overlay mode, and the overlaid module counts). The unused-import filter references `mods[0]`; guard it with `len(mods) > 0 &&`.

Check what `CompileCheckedWith` does with `c.program` and the extra module when generating (grep `ExtraFile` and `extra` there): in overlay mode there is no extra module to add to the program — the overlaid module is already in `c.program`. Nothing else should need the extra source; if a step re-reads `Module.Source` (e.g. for `--mutate`), skip it when `BaseLines > 0`.

- [ ] **Step 6: `MapDiag` with `BaseLines`**

`internal/scratch/lines.go`, at the top of `Build.MapDiag`:

```go
if b.ExtraFile == "" || d.Span.File != b.ExtraFile {
	return d, true
}
if b.BaseLines > 0 && d.Span.Start.Line <= b.BaseLines {
	return d, true // the module's own text, reported as it is
}
```

With `InsertLine = 1` and `LineOffset = BaseLines`, `SourceLine` already maps `L > BaseLines` to `L - BaseLines`. Update `SourceLine`'s and `MapDiag`'s comments to mention the in-module case in one sentence each.

- [ ] **Step 7: Run the scratch tests**

Run: `go test ./internal/scratch -count=1`
Expected: PASS, including the old `lines_test.go` cases.

- [ ] **Step 8: Commit**

```bash
git add internal/buildkit internal/scratch
git commit -m "scratch: in-module mode — check a session appended to an app module"
```

---

### Task 3: `alder eval --in`

**Files:**
- Modify: `cmd/alder/eval.go:24-26, 36-50, 78-140`, `cmd/alder/helpjson.go:102-108`
- Test: `cmd/alder/eval_test.go`, `cmd/alder/helpjson_test.go`

**Interfaces:**
- Consumes: `scratch.Module.In`, `scratch.NoModuleError`, `Checked.Declares`.
- Produces: `alder eval [dir] [--run] [--json] [--in Name | --module Name] <expr | ->`.

- [ ] **Step 1: Write the failing tests**

Append to `cmd/alder/eval_test.go`:

```go
func TestEval_InModule(t *testing.T) {
	needGoTool(t)
	t.Chdir(t.TempDir())
	evalOK(t, "0 : Int\n", "--in", "Todo", "countOpen []")
	// the module's own brand constructor, unqualified, and another module's, qualified
	evalOK(t, "\"a\" : Text\n", "--in", "Todo", "TodoId.toText (TodoId \"a\")")
}

func TestEval_InModuleDiagnosticsNameTheInput(t *testing.T) {
	needGoTool(t)
	t.Chdir(t.TempDir())
	code, stdout, stderr := runEval(t, "", todosDir(t), "--in", "Todo", "countOpen 1")
	if code != 1 || stdout != "" || !strings.Contains(stderr, "input:1:") {
		t.Errorf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}
}

func TestEval_InUnknownModule(t *testing.T) {
	t.Chdir(t.TempDir())
	code, _, stderr := runEval(t, "", todosDir(t), "--in", "Nope", "1")
	if code != 2 || !strings.Contains(stderr, "no module Nope") {
		t.Errorf("exit %d, stderr %q", code, stderr)
	}
}

func TestEval_InWithModuleIsUsage(t *testing.T) {
	t.Chdir(t.TempDir())
	code, _, stderr := runEval(t, "", todosDir(t), "--in", "Todo", "--module", "Probe", "1")
	if code != 2 || !strings.Contains(stderr, "--in") {
		t.Errorf("exit %d, stderr %q", code, stderr)
	}
}
```

If `TodoId.toText` does not exist in examples/todos, replace that line with another expression that uses the module's unqualified brand constructor and prints a plain value — read `examples/todos/app/Todo.alder` and pick one (e.g. `filterLabel Active` → `"Active" : Text` is enough if no brand function is handy; keep at least one unqualified-name case).

Add to `cmd/alder/helpjson_test.go`, following the file's existing flag assertion style, a check that `eval` and `repl` list a flag named `--in` with `Value: "Name"`.

- [ ] **Step 2: Run them to see them fail**

Run: `go test ./cmd/alder -run 'TestEval_In|TestHelpJSON' -count=1`
Expected: FAIL (`--in` is rejected by the usage error).

- [ ] **Step 3: Implement**

`cmd/alder/eval.go`:

- `evalUsage = "usage: alder eval [dir] [--run] [--json] [--in Name | --module Name] <expr | ->"`.
- The doc comment of `runEvalTo`: add a sentence — *With `--in Name` the expression is checked as if appended to app module Name: its declarations and imports unqualified, as a `test` in that module is written; `--module` names the scratch module of the other mode, and the two together are a usage error.*
- Arg parsing: add `inName := ""` and cases
  ```go
  case a == "--in" && i+1 < len(args):
  	i++
  	inName = args[i]
  case strings.HasPrefix(a, "--in="):
  	inName = strings.TrimPrefix(a, "--in=")
  ```
  After the loop:
  ```go
  if inName != "" && named {
  	return failf(2, "alder eval: --in and --module do not combine: --in Name evaluates inside module Name, --module names the scratch module\n%s", evalUsage)
  }
  if inName != "" && !moduleNameRE.MatchString(inName) {
  	return failf(2, "alder eval: --in wants a module name such as Todo, got %q", inName)
  }
  ```
- The session module: `sessionName := modName; if inName != "" { sessionName = inName }`. Build the module as `mod := &scratch.Module{Name: modName, In: inName, Source: es.src, Infer: true}`. After `checked := scratch.Check(dir, mod)`, before the `ModuleExistsError` check:
  ```go
  if nm := (*scratch.NoModuleError)(nil); errors.As(checked.Err, &nm) {
  	return failf(2, "alder eval: %v", nm)
  }
  ```
- Replace every later `modName` use that names the session (`evalDiags(... modName ...)`, `checked.Result.Values[modName+".it"]`, `evalIt(modName, sig)`) with `sessionName`. Check `evalDiags`/`evalMapDiag` for how they decide a diagnostic is the input's (they compare `d.Span.File` with `mod+".alder"`): make them use `checked.ExtraFile` instead, which is `"Scratch.alder"` in scratch mode and `"app/Todo.alder"` in in-module mode; `MapDiag` already returns a module-own diagnostic unchanged, so it falls through to the app-diagnostic path and prints `app/Todo.alder:L:C`.
- `it` already declared by the module: after `Check`, `if inName != "" && checked.Declares("it") { return failf(2, "alder eval: %s declares it, the name eval gives the expression", inName) }`.

`cmd/alder/helpjson.go`: `eval` gets `{Name: "--in", Value: "Name", Summary: "evaluate inside this module: its names unqualified"}` and change `--module`'s summary to `"name eval's own scratch module (default Scratch)"`.

- [ ] **Step 4: Run the eval tests**

Run: `go test ./cmd/alder -run 'TestEval|TestHelpJSON' -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/alder/eval.go cmd/alder/eval_test.go cmd/alder/helpjson.go cmd/alder/helpjson_test.go
git commit -m "eval: --in Name evaluates inside an app module"
```

---

### Task 4: `alder repl --in` and `:in`

**Files:**
- Modify: `cmd/alder/repl.go:26-40, 52-62, 81-160, 332-358, 400-458, 445, 484, 645, 687-697, 821-860`, `cmd/alder/helpjson.go:130-132`
- Test: `cmd/alder/repl_test.go`

**Interfaces:**
- Consumes: `scratch.Module.In`, `Checked.Declares`, `scratch.NoModuleError`.
- Produces: `alder repl [dir] [--in Name | --module Name]`; REPL commands `:in Name`, `:in`; banner `alder repl: todos, in Todo (:help …)`.

- [ ] **Step 1: Write the failing tests**

Append to `cmd/alder/repl_test.go`:

```go
func TestRepl_InModule(t *testing.T) {
	needGoTool(t)
	t.Chdir(t.TempDir())
	code, out := runRepl(t, "countOpen []\nlet n = countOpen []\nn + 1\n", todosDir(t), "--in", "Todo")
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	wantIn(t, out, "alder repl: todos, in Todo", "0 : Int\n", "n : Int\n", "1 : Int\n")
}

func TestRepl_InRefusesShadowingLet(t *testing.T) {
	needGoTool(t)
	t.Chdir(t.TempDir())
	_, out := runRepl(t, "let countOpen = 1\ncountOpen []\n", todosDir(t), "--in", "Todo")
	wantIn(t, out, "alder repl: Todo already declares countOpen", "0 : Int\n")
}

func TestRepl_InCommandSwitchesAndKeepsLets(t *testing.T) {
	needGoTool(t)
	t.Chdir(t.TempDir())
	// a let that uses Todo's names unqualified survives :in Todo and is dropped by :in (app scope)
	_, out := runRepl(t, "let a = 1\n:in Todo\nlet n = countOpen []\n:in\nn\na\n", todosDir(t))
	wantIn(t, out, "in Todo\n", "n : Int\n", "dropped let n:", "1 : Int\n")
	if strings.Contains(out, "dropped let a") {
		t.Errorf("a does not depend on the module:\n%s", out)
	}
}

func TestRepl_InUnknownModule(t *testing.T) {
	needGoTool(t)
	t.Chdir(t.TempDir())
	code, out := runRepl(t, "", todosDir(t), "--in", "Nope")
	if code != 2 || !strings.Contains(out, "no module Nope") {
		t.Errorf("exit %d\n%s", code, out)
	}
	_, out = runRepl(t, ":in Nope\n1\n", todosDir(t))
	wantIn(t, out, "no module Nope", "1 : Int\n")
}

func TestRepl_InWithModuleIsUsage(t *testing.T) {
	code, out := runRepl(t, "", todosDir(t), "--in", "Todo", "--module", "Probe")
	if code != 2 || !strings.Contains(out, "--in") {
		t.Errorf("exit %d\n%s", code, out)
	}
}
```

- [ ] **Step 2: Run them to see them fail**

Run: `go test ./cmd/alder -run 'TestRepl_In' -count=1`
Expected: FAIL.

- [ ] **Step 3: Implement**

`cmd/alder/repl.go`:

- `replUsage` becomes `usage: alder repl [dir] [--in Name | --module Name]`; `replHelp` gains, after `:reset`:
  ```
    :in Name          speak as module Name: its names unqualified (lets are re-checked; :in alone returns to app scope)
  ```
- `repl` struct: add `in string // the module of in-module mode ("" for app scope)`.
- Arg parsing: as in Task 3 (`--in`, `--in=`, the combination error printed to stderr with `replUsage`, the name check). Set `r.in = inName` in the literal.
- Add a method the four `r.modName` session uses call:
  ```go
  // session is the name the session's values are recorded under: the module of in-module mode,
  // else the scratch module.
  func (r *repl) session() string {
  	if r.in != "" {
  		return r.in
  	}
  	return r.modName
  }
  ```
  and use `r.session()` at lines ~445 (`Values[r.modName+"."+e.Name]`), ~484 (`Values[r.modName+".it"]`), ~645 (`evalIt(r.modName, sig)`). In `check`: `&scratch.Module{Name: r.modName, In: r.in, Source: es.src, Infer: true}`.
- Startup: after `checked, _ := r.check(nil)`, before the `ModuleExistsError` check:
  ```go
  if nm := (*scratch.NoModuleError)(nil); errors.As(checked.Err, &nm) {
  	fmt.Fprintf(stderr, "alder repl: %v\n", nm)
  	return 2
  }
  ```
  Banner: `fmt.Fprintf(stdout, "alder repl: %s%s (:help for the commands, :quit to leave)\n", filepath.Base(abs), r.inSuffix())` with
  ```go
  func (r *repl) inSuffix() string {
  	if r.in == "" {
  		return ""
  	}
  	return ", in " + r.in
  }
  ```
- `let`: after `e` is validated (`!e.Let || e.Name == ""` check) and before building `session`, refuse a shadowing let. The module's declarations are known from a base check; keep the last base `Checked` on the struct (`r.base scratch.Checked`, set in `reload` from `base` and at startup from `checked`), then:
  ```go
  if r.in != "" && r.base.Declares(e.Name) {
  	fmt.Fprintf(r.stderr, "alder repl: %s already declares %s\n", r.in, e.Name)
  	return
  }
  ```
- `command`: add
  ```go
  case ":in":
  	if arg != "" && !moduleNameRE.MatchString(arg) {
  		fmt.Fprintf(r.stderr, "alder repl: :in wants a module name such as Todo, got %q\n", arg)
  		return false
  	}
  	prev := r.in
  	r.in = arg
  	if !r.reloadTo(arg) {
  		r.in = prev
  	}
  ```
  where `reloadTo` is `reload` with the header `"in " + name` (or `"app scope"` for `""`) and returning false (after printing `alder repl: no module Nope in <app>`) when the base check's `Err` is a `NoModuleError`. Refactor: `reload(header string)` becomes `func (r *repl) reload(header string) bool` returning whether the base checked; `:reload` ignores the result. The existing early returns in `reload` return false.
- The `:in`-dropped notice reuses `reload`'s `dropped let %s: …` line (the test expects `dropped let n:`).
- The diagnostics printer (`report`/`printDiag`) decides whether a diagnostic is the session's by file name; make it use `checked.ExtraFile` (see Task 3's note) so that in in-module mode a diagnostic in the module's own text prints as `app/Todo.alder:L:C` under the app-diagnostics header and one in the session maps to the let/expression as today.

`cmd/alder/helpjson.go` `repl`: `Flags: []flagSpec{{Name: "--in", Value: "Name", Summary: "start inside this module: its names unqualified"}, {Name: "--module", Value: "Name", Summary: "name the REPL's own scratch module (default Scratch)"}}`.

- [ ] **Step 4: Run the REPL tests**

Run: `go test ./cmd/alder -run 'TestRepl' -count=1`
Expected: PASS (old and new).

- [ ] **Step 5: Commit**

```bash
git add cmd/alder/repl.go cmd/alder/repl_test.go cmd/alder/helpjson.go
git commit -m "repl: --in Name and :in — speak as an app module"
```

---

### Task 5: Book, full alder suite, merge

**Files:**
- Modify: `book/11-tooling.md:652-740 (eval), 739-890 (repl), 1614+ (What you get)`

- [ ] **Step 1: Document `--in`**

In `### alder eval`, after the paragraph ending `(alder eval . --module Probe '1 + 2')` (~L721), add:

```markdown
`--in Name` evaluates inside module `Name` instead: the expression is checked as if written
at the end of `app/Name.alder`, so the module's own declarations and imports are unqualified,
as a `test` there writes them, and the other modules stay qualified. A line copied from the
file evaluates unchanged:

```text
$ alder eval examples/todos --in Todo 'countOpen []'
0 : Int
```

`--in` and `--module` do not combine (`--module` names the scratch module of the other
mode). A module that declares `it` cannot host eval (exit 2).
```

In `### alder repl`: the usage line becomes `alder repl [dir] [--in Name | --module Name]`; add `:in Name` and `:in` rows to the commands table (*speaks as module `Name`, re-checking the kept `let`s as `:reload` does; `:in` alone returns to app scope*); add after the table: *A `let` whose name the module declares is refused (`alder repl: Todo already declares countOpen`): the module's names are the module's. The banner names the module: `alder repl: todos, in Todo (:help …)`.* Update the `- The session's module is named Scratch; --module Name renames it` bullet (~L887) to mention `--in`.

In `## Editor support` → `### What you get` (~L1614), add bullets:

```markdown
- `<leader>at` runs `alder test`; failures land in the quickfix list at their `test` line,
  compile errors as diagnostics. `<leader>aT` runs the test under the cursor (the module's
  tests when none is above it).
- `<leader>am` runs `alder test --mutate` in the background; the survivors land in the
  quickfix list at the mutated expression, `before → after`.
- `<leader>ax` opens `alder repl --in <this module>` in a split; `<leader>as` sends the
  current line or selection to it.
```

- [ ] **Step 2: Run the whole suite**

Run: `cd ~/projects/alder && scripts/test.sh`
Expected: green. (If the suite's runtime is a concern, run `go test ./book ./cmd/alder ./internal/... ./rt/... -count=1` first and `scripts/test.sh` once.)

- [ ] **Step 3: Commit, merge, push, rebuild the binary**

```bash
git add book/11-tooling.md
git commit -m "book: eval/repl --in, editor test and REPL keys"
git checkout main && git merge --no-ff m6c -m "Merge branch 'm6c': test positions, eval/repl --in" && git push
go build -o ~/go/bin/alder ./cmd/alder && ~/go/bin/alder help --json | grep -c '"--in"'
```
Expected: the last command prints `2`.

(The branch name `m6c` assumes the work started on a branch; create it before Task 1 with `git checkout -b m6c`.)

---

### Task 6: alder.nvim — `test.lua`: `alder test --json` → quickfix

**Files:**
- Create: `lua/alder/test.lua`, `tests/test_spec.lua`
- Modify: `lua/alder/check.lua:35-41`, `lua/alder/config.lua:9`, `lua/alder/keys.lua`, `plugin/alder.lua`

**Interfaces:**
- Consumes: `cli.run(args, {cwd}, on_done)` → `res{code, stdout, stderr, objects, json}`; `check.publish(app_root, objects)`; `root.for_buf(buf)`.
- Produces: `check.set_qf(items, title)`; `test.entry(app_root, r) → qf item or nil`; `test.summary(results) → string`; `test.write_app_buffers(app_root) → unwritten names`; `test.run(app_root, {args, on_done})`; `test.for_current(o)`; `keys.actions.test`.

- [ ] **Step 1: Export `set_qf` from check.lua**

Replace the local `set_qf` with an exported one taking a title (default `QF_TITLE`), and update its one caller (`M.publish`) to `M.set_qf(items)`:

```lua
-- set_qf replaces the quickfix list when the current one is ours (by title), and otherwise
-- pushes a new one, so a list the user made is never overwritten. With no items and no
-- current list of ours there is nothing to say, and the user's list stays current.
function M.set_qf(items, title)
  title = title or QF_TITLE
  local ours = vim.fn.getqflist({ title = 0 }).title == title
  if not ours and #items == 0 then return end
  vim.fn.setqflist({}, ours and 'r' or ' ', { title = title, items = items })
end
```

Run `nvim --headless -u tests/minimal.lua -l tests/run.lua check` — Expected: all check tests still pass.

- [ ] **Step 2: Write the failing pure tests**

`tests/test_spec.lua`:

```lua
local H = require('helpers')
local test_ = require('alder.test')

test('test: a failure with sides becomes a quickfix entry at its test line', function()
  local e = test_.entry('/app', { module = 'Todo', name = 'countOpen ignores done', ok = false,
    file = 'app/Todo.alder', line = 88, col = 1, left = '3', right = '2', trace = {}, ms = 1 })
  assert(e.filename == '/app/app/Todo.alder' and e.lnum == 88 and e.col == 1 and e.type == 'E', vim.inspect(e))
  assert(e.text == 'FAIL "countOpen ignores done" — left: 3  right: 2', e.text)
end)

test('test: an error and a note print instead of sides; newlines flatten', function()
  local e = test_.entry('/app', { module = 'M', name = 'n', ok = false, file = 'app/M.alder', line = 2, col = 1,
    error = 'boom\n  here', trace = {} })
  assert(e.text == 'FAIL "n" — error: boom here', e.text)
  e = test_.entry('/app', { module = 'M', name = 'n', ok = false, file = 'app/M.alder', line = 2, col = 1,
    note = 'sides print the same', trace = {} })
  assert(e.text == 'FAIL "n" — note: sides print the same', e.text)
end)

test('test: a passing result is no entry; a result without a position keeps module and name', function()
  assert(test_.entry('/app', { module = 'M', name = 'n', ok = true }) == nil)
  local e = test_.entry('/app', { module = 'M', name = 'n', ok = false, error = 'crashed' })
  assert(e.filename == nil and e.lnum == 0 and e.text == 'FAIL M "n" — error: crashed', vim.inspect(e))
end)

test('test: summary counts', function()
  assert(test_.summary({ { ok = true }, { ok = false }, { ok = true } }) == '2 passed, 1 failed')
  assert(test_.summary({}) == '0 passed, 0 failed')
end)
```

Run: `nvim --headless -u tests/minimal.lua -l tests/run.lua 'test:'`
Expected: FAIL (`alder.test` not found).

- [ ] **Step 3: Write `lua/alder/test.lua` (the pure half and the run)**

```lua
local cli = require('alder.cli')
local check = require('alder.check')
local root = require('alder.root')

local M = {}

M.QF_TITLE = 'alder test'

local function real(p) return vim.uv.fs_realpath(p) or vim.fs.normalize(p) end

local function flat(s) return (s:gsub('%s*\n%s*', ' ')) end

-- entry maps one `alder test --json` result to a quickfix item, or nil for a pass. A result
-- without a position (a crashed program's) keeps the module in the text and lnum 0.
function M.entry(app_root, r)
  if r.ok then return nil end
  local what
  if r.left or r.right then
    what = 'left: ' .. flat(r.left or '') .. '  right: ' .. flat(r.right or '')
  elseif r.error then
    what = 'error: ' .. flat(r.error)
  elseif r.note then
    what = 'note: ' .. flat(r.note)
  else
    what = 'failed'
  end
  local positioned = r.file and r.line and r.line > 0
  local name = positioned and ('"' .. r.name .. '"') or (r.module .. ' "' .. r.name .. '"')
  return {
    filename = positioned and (app_root .. '/' .. r.file) or nil,
    lnum = positioned and r.line or 0,
    col = positioned and (r.col or 1) or 0,
    type = 'E',
    text = 'FAIL ' .. name .. ' — ' .. what,
  }
end

-- summary is the CLI's closing line, from the results.
function M.summary(results)
  local passed, failed = 0, 0
  for _, r in ipairs(results) do
    if r.ok then passed = passed + 1 else failed = failed + 1 end
  end
  return ('%d passed, %d failed'):format(passed, failed)
end

-- write_app_buffers writes every modified .alder buffer under app_root (the run reads disk)
-- and returns the names of those it could not write.
function M.write_app_buffers(app_root)
  local app_real, failed = real(app_root), {}
  for _, buf in ipairs(vim.api.nvim_list_bufs()) do
    local name = vim.api.nvim_buf_get_name(buf)
    if vim.api.nvim_buf_is_loaded(buf) and vim.bo[buf].modified and name:match('%.alder$') then
      local r = root.find(name)
      if r and real(r) == app_real then
        local ok = pcall(vim.api.nvim_buf_call, buf, function() vim.cmd('silent write') end)
        if not ok or vim.bo[buf].modified then table.insert(failed, vim.fn.fnamemodify(name, ':.')) end
      end
    end
  end
  return failed
end

-- results are the test objects of a run: those with a `name` (the diagnostics and error
-- objects have none).
local function results_of(objects)
  return vim.tbl_filter(function(o) return o.name ~= nil end, objects)
end

local runs = {} -- real app root → the latest run { handle }

-- run runs `alder test --json <o.args>` for the app and reports: compile diagnostics through
-- check.publish, failures in the quickfix list, the summary as a notification. A newer run for
-- the same app cancels the older one.
function M.run(app_root, o)
  o = o or {}
  local key = real(app_root)
  local prev = runs[key]
  if prev and prev.handle then pcall(function() prev.handle:kill('sigterm') end) end
  local unwritten = M.write_app_buffers(app_root)
  if #unwritten > 0 then
    vim.notify('alder test: not written, the run reads disk: ' .. table.concat(unwritten, ', '), vim.log.levels.WARN)
  end
  local mine = {}
  runs[key] = mine
  local args = { 'test', '--json' }
  vim.list_extend(args, o.args or {})
  table.insert(args, app_root)
  mine.handle = cli.run(args, { cwd = app_root }, function(res)
    if runs[key] ~= mine then return end
    runs[key] = nil
    if res.json and res.json.diagnostics then
      local items = check.publish(app_root, res.json.diagnostics)
      vim.notify('alder test: the app does not check', vim.log.levels.ERROR)
      for _, item in ipairs(items) do
        if item.type == 'E' then vim.cmd('copen') break end
      end
      if o.on_done then o.on_done(res) end
      return
    end
    local results = results_of(res.objects)
    if res.code == 2 or (res.code ~= 0 and #results == 0) or (res.json and res.json.error) then
      cli.report(args, res)
      if o.on_done then o.on_done(res) end
      return
    end
    local items = {}
    for _, r in ipairs(results) do
      local e = M.entry(app_root, r)
      if e then table.insert(items, e) end
    end
    check.set_qf(items, M.QF_TITLE)
    if #items > 0 then
      vim.cmd('copen')
      vim.notify('alder test: ' .. M.summary(results), vim.log.levels.ERROR)
    else
      if vim.fn.getqflist({ title = 0 }).title == M.QF_TITLE then vim.cmd('cclose') end
      vim.notify('alder test: ' .. M.summary(results), vim.log.levels.INFO)
    end
    if o.on_done then o.on_done(res) end
  end)
  if not mine.handle and runs[key] == mine then runs[key] = nil end
end

-- for_current runs the tests of the current buffer's app, or says there is none.
function M.for_current(o)
  local r = root.for_buf(0)
  if not r then
    vim.notify('alder: no `app/` directory above this file', vim.log.levels.ERROR)
    return
  end
  M.run(r, o)
end

return M
```

Run the pure tests: `nvim --headless -u tests/minimal.lua -l tests/run.lua 'test:'` — Expected: PASS.

- [ ] **Step 4: Integration test with the real binary**

Append to `tests/test_spec.lua`:

```lua
-- run_tests runs test.run and waits for it to report.
local function run_tests(app, o)
  o = o or {}
  local done = false
  o.on_done = function() done = true end
  require('alder.test').run(app, o)
  assert(H.wait(120000, function() return done end), 'alder test did not finish')
end

test('test: a failing test lands in quickfix at its line; a clean run closes the list', function()
  local app = H.new_app()
  local file = app .. '/app/Demo.alder'
  H.write(file, 'module Demo\n\nanswer : Int\nanswer = 41\n\ntest "answer is 42" = answer == 42\n')
  local msgs, restore = H.capture_notify()
  run_tests(app)
  restore()
  local qf = vim.fn.getqflist({ title = 0, items = 0 })
  assert(qf.title == 'alder test', qf.title)
  local hit
  for _, it in ipairs(qf.items) do
    if it.text:find('answer is 42', 1, true) then hit = it end
  end
  assert(hit and hit.lnum == 6 and vim.api.nvim_buf_get_name(hit.bufnr):find('Demo.alder', 1, true), vim.inspect(qf.items))
  assert(msgs[#msgs].msg:find('1 failed', 1, true), vim.inspect(msgs))

  H.write(file, 'module Demo\n\nanswer : Int\nanswer = 42\n\ntest "answer is 42" = answer == 42\n')
  run_tests(app)
  assert(#vim.fn.getqflist() == 0)
  local open = false
  for _, w in ipairs(vim.api.nvim_list_wins()) do
    if vim.bo[vim.api.nvim_win_get_buf(w)].buftype == 'quickfix' then open = true end
  end
  assert(not open, 'the quickfix window stayed open after a clean run')
end)

test('test: a compile error takes the check path', function()
  local app = H.new_app()
  H.write(app .. '/app/Demo.alder', 'module Demo\n\nbroken : Int\nbroken = "x"\n')
  run_tests(app)
  local qf = vim.fn.getqflist({ title = 0, items = 0 })
  assert(qf.title == 'alder check' and #qf.items > 0 and qf.items[1].text:find('E0003', 1, true), vim.inspect(qf))
end)
```

If `alder new` scaffolds a module with the same name or the scaffold fails to check with an extra file, pick a name the scaffold does not use (read `H.new_app`'s output tree once) and keep the file self-contained as above.

Run: `PATH=$HOME/go/bin:$PATH nvim --headless -u tests/minimal.lua -l tests/run.lua 'test:'` — Expected: PASS.

- [ ] **Step 5: Command and key**

`lua/alder/config.lua` keys: `{ generate = '<leader>ag', diagnostics = '<leader>ac', explain = '<leader>ae', run = '<leader>ar', test = '<leader>at', test_at = '<leader>aT', mutate = '<leader>am', repl = '<leader>ax', send = '<leader>as' }` (the later ones get actions in Tasks 7–9; `keys.attach` maps only names with an action, so adding them now is safe).

`lua/alder/keys.lua` `M.actions.test = function() require('alder.test').for_current() end`, `DESC.test = 'alder: run the tests'`.

`plugin/alder.lua`:

```lua
vim.api.nvim_create_user_command('AlderTest', function(o)
  require('alder.test').for_current({ args = o.fargs })
end, { nargs = '*', desc = 'alder test [args]: failures in the quickfix list' })
```

Add to `tests/keys_spec.lua` (next to the existing mapped-key assertions) `assert(mapped('<leader>at'))` on an attached buffer.

Run: `PATH=$HOME/go/bin:$PATH nvim --headless -u tests/minimal.lua -l tests/run.lua` — Expected: whole suite PASS.

- [ ] **Step 6: Commit**

```bash
git add lua/alder/test.lua lua/alder/check.lua lua/alder/config.lua lua/alder/keys.lua plugin/alder.lua tests/test_spec.lua tests/keys_spec.lua
git commit -m "test: alder test --json failures in the quickfix list (<leader>at, :AlderTest)"
```

---

### Task 7: `buf.lua` — module and test-at-cursor detection; `<leader>aT`

**Files:**
- Create: `lua/alder/buf.lua`, `tests/buf_spec.lua`
- Modify: `lua/alder/test.lua`, `lua/alder/keys.lua`, `plugin/alder.lua`

**Interfaces:**
- Produces: `buf.module_name(bufnr) → string|nil`; `buf.test_at(bufnr, row0) → name|nil` (the nearest `test` at or above the 0-based row); `test.run_pattern(module, name) → string` (a Go regexp for `-run`); `test.at_cursor()`; `keys.actions.test_at`.

- [ ] **Step 1: Write the failing tests**

`tests/buf_spec.lua`:

```lua
local H = require('helpers')
local B = require('alder.buf')

local function buffer(lines, ft)
  local b = vim.api.nvim_create_buf(true, false)
  vim.api.nvim_buf_set_lines(b, 0, -1, false, lines)
  if ft then vim.bo[b].filetype = 'alder' end
  return b
end

local SRC = {
  '-- a comment', 'module Todos.Index', '', 'import Todo', '',
  'x : Int', 'x = 1', '',
  'test "first (1)" = x == 1', 'test "second \\"q\\"" =', '  x == 1', '', 'y = 2',
}

test('buf: module name by regex (no parser)', function()
  local b = buffer(SRC)
  assert(B.module_name(b) == 'Todos.Index', tostring(B.module_name(b)))
  assert(B.module_name(buffer({ 'x = 1' })) == nil)
end)

test('buf: test at or above the cursor by regex', function()
  local b = buffer(SRC)
  assert(B.test_at(b, 8) == 'first (1)')
  assert(B.test_at(b, 10) == 'second \\"q\\"') -- raw text between the quotes
  assert(B.test_at(b, 12) == 'second \\"q\\"') -- the nearest above
  assert(B.test_at(b, 6) == nil)
end)

test('buf: module name and test by tree-sitter', function()
  H.ensure_parser()
  local b = buffer(SRC, 'alder')
  assert(B.module_name(b) == 'Todos.Index')
  assert(B.test_at(b, 8) == 'first (1)')
end)

test('test: run_pattern escapes the name for Go regexp', function()
  local T = require('alder.test')
  assert(T.run_pattern('Todo', 'first (1)') == '^Todo "first \\(1\\)"$', T.run_pattern('Todo', 'first (1)'))
  assert(T.run_pattern('Todo', nil) == '^Todo ')
end)
```

Run: `nvim --headless -u tests/minimal.lua -l tests/run.lua 'buf:'` — Expected: FAIL (`alder.buf` not found).

- [ ] **Step 2: Implement `lua/alder/buf.lua`**

```lua
-- buf reads two facts from an .alder buffer: its module name and the test at the cursor.
-- Through tree-sitter when the buffer has a parser, else by regex on the lines.
local M = {}

local function parser(bufnr)
  local ok, p = pcall(vim.treesitter.get_parser, bufnr, 'alder', { error = false })
  if ok and p then return p end
end

local function root_node(bufnr)
  local p = parser(bufnr)
  if not p then return nil end
  local trees = p:parse()
  return trees and trees[1] and trees[1]:root()
end

-- module_name is the name the buffer's `module` line declares, or nil.
function M.module_name(bufnr)
  local root = root_node(bufnr)
  if root then
    for node in root:iter_children() do
      if node:type() == 'module_decl' then
        local name = node:field('name')[1]
        if name then return vim.treesitter.get_node_text(name, bufnr) end
      end
    end
  end
  for _, l in ipairs(vim.api.nvim_buf_get_lines(bufnr, 0, -1, false)) do
    local name = l:match('^module%s+([%w_%.]+)')
    if name then return name end
  end
  return nil
end

-- test_at is the name of the nearest `test "name"` declaration starting at or above row
-- (0-based), as written between its quotes, or nil.
function M.test_at(bufnr, row)
  local root = root_node(bufnr)
  if root then
    local best, best_row
    for node in root:iter_children() do
      local r = node:range()
      if node:type() == 'test_decl' and r <= row and (not best_row or r > best_row) then
        best, best_row = node, r
      end
    end
    if best then
      local s = best:field('name')[1]
      if s then return (vim.treesitter.get_node_text(s, bufnr):gsub('^"', ''):gsub('"$', '')) end
    end
  end
  local lines = vim.api.nvim_buf_get_lines(bufnr, 0, row + 1, false)
  for i = #lines, 1, -1 do
    local name = lines[i]:match('^test%s+"(.*)"%s*=')
    if name then return name end
  end
  return nil
end

return M
```

- [ ] **Step 3: `run_pattern` and `at_cursor` in test.lua; the key and command**

Add to `lua/alder/test.lua`:

```lua
-- run_pattern is the -run regexp selecting one test (`^Module "name"$`, the name's Go
-- regexp metacharacters escaped) or, without a name, every test of the module.
function M.run_pattern(module, name)
  if not name then return '^' .. module .. ' ' end
  local escaped = name:gsub('[%^%$%(%)%%%.%[%]%*%+%-%?%{%}%|\\]', '\\%0')
  return '^' .. module .. ' "' .. escaped .. '"$'
end

-- at_cursor runs the test under the cursor, or the current module's tests when none is above
-- it, or the whole suite outside a module.
function M.at_cursor()
  local buf = require('alder.buf')
  local b = vim.api.nvim_get_current_buf()
  local module = buf.module_name(b)
  if not module then
    vim.notify('alder test: no `module` line in this buffer; running every test', vim.log.levels.WARN)
    return M.for_current()
  end
  local name = buf.test_at(b, vim.api.nvim_win_get_cursor(0)[1] - 1)
  if not name then vim.notify('alder test: no test above the cursor; running ' .. module .. "'s tests", vim.log.levels.INFO) end
  M.for_current({ args = { '-run', M.run_pattern(module, name) } })
end
```

`keys.lua`: `M.actions.test_at = function() require('alder.test').at_cursor() end`, `DESC.test_at = 'alder: run the test under the cursor'`. `plugin/alder.lua`: no new command (`:AlderTest -run …` covers it).

Run: `PATH=$HOME/go/bin:$PATH nvim --headless -u tests/minimal.lua -l tests/run.lua 'buf:'` and `… 'test:'` — Expected: PASS.

- [ ] **Step 4: Pin the selection end to end**

Append to `tests/test_spec.lua`:

```lua
test('test: <leader>aT runs only the test under the cursor', function()
  local app = H.new_app()
  local file = app .. '/app/Demo.alder'
  H.write(file, 'module Demo\n\ntest "bad (1)" = 1 == 2\ntest "good" = 1 == 1\n')
  vim.cmd.edit(file)
  vim.api.nvim_win_set_cursor(0, { 4, 0 })
  local done = false
  local T = require('alder.test')
  local orig = T.for_current
  T.for_current = function(o) o = o or {}; o.on_done = function() done = true end; orig(o) end
  T.at_cursor()
  assert(H.wait(120000, function() return done end), 'did not finish')
  T.for_current = orig
  assert(#vim.fn.getqflist() == 0, 'the bad test ran: ' .. vim.inspect(vim.fn.getqflist()))
  vim.api.nvim_win_set_cursor(0, { 3, 0 })
  done = false
  T.for_current = function(o) o = o or {}; o.on_done = function() done = true end; orig(o) end
  T.at_cursor()
  assert(H.wait(120000, function() return done end), 'did not finish')
  T.for_current = orig
  assert(#vim.fn.getqflist() == 1 and vim.fn.getqflist()[1].text:find('bad (1)', 1, true))
end)
```

Run: `PATH=$HOME/go/bin:$PATH nvim --headless -u tests/minimal.lua -l tests/run.lua 'test:'` — Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add lua/alder/buf.lua lua/alder/test.lua lua/alder/keys.lua tests/buf_spec.lua tests/test_spec.lua
git commit -m "test: run the test under the cursor (<leader>aT)"
```

---

### Task 8: Mutants → quickfix (`<leader>am`, `:AlderMutate`)

**Files:**
- Create: `tests/mutate_spec.lua`
- Modify: `lua/alder/test.lua`, `lua/alder/keys.lua`, `plugin/alder.lua`

**Interfaces:**
- Produces: `test.survivor(app_root, m) → qf item or nil`; `test.mutate_title(score) → string`; `test.mutate(app_root, {args, on_done})`; `test.mutate_for_current(o)`; `keys.actions.mutate`.

- [ ] **Step 1: Write the failing pure tests**

`tests/mutate_spec.lua`:

```lua
local H = require('helpers')
local T = require('alder.test')

test('mutate: a survivor becomes a warning entry at the mutated expression', function()
  local e = T.survivor('/app', { id = 3, module = 'Todo', file = 'app/Todo.alder', decl = 'matches', line = 21, col = 14,
    kind = 'not dropped', before = 'not todo.done', after = 'todo.done', killed = false })
  assert(e.filename == '/app/app/Todo.alder' and e.lnum == 21 and e.col == 14 and e.type == 'W', vim.inspect(e))
  assert(e.text == 'survived  not dropped: not todo.done → todo.done', e.text)
  assert(T.survivor('/app', { killed = true, file = 'app/Todo.alder', line = 1, col = 1 }) == nil)
end)

test('mutate: the title carries the score', function()
  assert(T.mutate_title({ score = 0.5625, killed = 18, survived = 14, suppressed = 0, skipped = 0 })
    == 'alder test --mutate — 18 killed, 14 survived (0.56)')
end)
```

Run: `nvim --headless -u tests/minimal.lua -l tests/run.lua 'mutate:'` — Expected: FAIL.

- [ ] **Step 2: Implement in `lua/alder/test.lua`**

```lua
M.MUTATE_TITLE = 'alder test --mutate'

-- survivor maps one `--mutate --json` mutant to a quickfix item, or nil when it was killed.
function M.survivor(app_root, m)
  if m.killed or not m.file then return nil end
  return {
    filename = app_root .. '/' .. m.file,
    lnum = m.line or 0,
    col = m.col or 0,
    type = 'W',
    text = ('survived  %s: %s → %s'):format(m.kind or '?', m.before or '', m.after or ''),
  }
end

-- mutate_title is the quickfix title, from the run's score object.
function M.mutate_title(s)
  return ('%s — %d killed, %d survived (%.2f)'):format(M.MUTATE_TITLE, s.killed or 0, s.survived or 0, s.score or 0)
end

local mutations = {} -- real app root → the running mutation { handle }

-- mutate runs `alder test --mutate --json` in the background and puts the survivors in the
-- quickfix list. A second call for the same app cancels the first.
function M.mutate(app_root, o)
  o = o or {}
  local key = real(app_root)
  local prev = mutations[key]
  if prev and prev.handle then
    pcall(function() prev.handle:kill('sigterm') end)
    vim.notify('alder test --mutate: the earlier run was cancelled', vim.log.levels.WARN)
  end
  local unwritten = M.write_app_buffers(app_root)
  if #unwritten > 0 then
    vim.notify('alder test --mutate: not written, the run reads disk: ' .. table.concat(unwritten, ', '), vim.log.levels.WARN)
  end
  local mine = {}
  mutations[key] = mine
  local args = { 'test', '--mutate', '--json' }
  vim.list_extend(args, o.args or {})
  table.insert(args, app_root)
  vim.notify('alder test --mutate: running…', vim.log.levels.INFO)
  mine.handle = cli.run(args, { cwd = app_root }, function(res)
    if mutations[key] ~= mine then return end
    mutations[key] = nil
    if res.json and res.json.diagnostics then
      check.publish(app_root, res.json.diagnostics)
      vim.notify('alder test --mutate: the app does not check', vim.log.levels.ERROR)
      if o.on_done then o.on_done(res) end
      return
    end
    local score
    for _, obj in ipairs(res.objects) do
      if obj.score ~= nil and obj.killed ~= nil and obj.id == nil then score = obj end
    end
    if res.code == 2 or (res.json and res.json.error) or not score then
      cli.report(args, res)
      if o.on_done then o.on_done(res) end
      return
    end
    local items = {}
    for _, m in ipairs(res.objects) do
      local e = m.id ~= nil and M.survivor(app_root, m)
      if e then table.insert(items, e) end
    end
    table.sort(items, function(a, b)
      if a.filename ~= b.filename then return a.filename < b.filename end
      return a.lnum < b.lnum
    end)
    local title = M.mutate_title(score)
    local current = vim.fn.getqflist({ title = 0 }).title
    local ours = vim.startswith(current, M.MUTATE_TITLE)
    if #items > 0 or ours then
      vim.fn.setqflist({}, ours and 'r' or ' ', { title = title, items = items })
    end
    if #items > 0 then
      vim.cmd('copen')
      vim.notify(title, vim.log.levels.WARN)
    else
      vim.notify(title .. ': no survivors', vim.log.levels.INFO)
    end
    if o.on_done then o.on_done(res) end
  end)
  if not mine.handle and mutations[key] == mine then mutations[key] = nil end
end

-- mutate_for_current runs the mutants of the current buffer's app.
function M.mutate_for_current(o)
  local r = root.for_buf(0)
  if not r then
    vim.notify('alder: no `app/` directory above this file', vim.log.levels.ERROR)
    return
  end
  M.mutate(r, o)
end
```

(The title changes with the score, so the "ours" test is a prefix match — the one place this plan departs from `check.set_qf`'s exact-title rule.)

`keys.lua`: `M.actions.mutate = function() require('alder.test').mutate_for_current() end`, `DESC.mutate = 'alder: mutation testing (survivors to quickfix)'`. `plugin/alder.lua`:

```lua
vim.api.nvim_create_user_command('AlderMutate', function(o)
  require('alder.test').mutate_for_current({ args = o.fargs })
end, { nargs = '*', desc = 'alder test --mutate [args] in the background: survivors in the quickfix list' })
```

Run the pure tests — Expected: PASS.

- [ ] **Step 3: One short integration run**

Append to `tests/mutate_spec.lua`:

```lua
test('mutate: a survivor lands in quickfix at its expression', function()
  local app = H.new_app()
  -- `answer` is tested only for being positive: the `+ 1 → - 1` style mutants of 41 survive
  H.write(app .. '/app/Demo.alder', 'module Demo\n\nanswer : Int\nanswer = 40 + 1\n\ntest "answer is positive" = answer > 0\n')
  local done = false
  require('alder.test').mutate(app, { on_done = function() done = true end })
  assert(H.wait(300000, function() return done end), 'alder test --mutate did not finish')
  local qf = vim.fn.getqflist({ title = 0, items = 0 })
  assert(vim.startswith(qf.title, 'alder test --mutate'), qf.title)
  local hit
  for _, it in ipairs(qf.items) do
    if it.text:find('survived', 1, true) and vim.api.nvim_buf_get_name(it.bufnr):find('Demo.alder', 1, true) then hit = it end
  end
  assert(hit and hit.lnum == 4 and hit.type == 'W', vim.inspect(qf.items))
end)
```

If the scaffolded app's own tests make the run long (minutes), keep the fixture but pass `{ args = { '-run', '^Demo ' } }` so only `Demo`'s tests run against the mutants (the mutants are still generated for every module; if that is still slow, note the time in the commit message and leave the test — the spec accepts one short integration run).

Run: `PATH=$HOME/go/bin:$PATH nvim --headless -u tests/minimal.lua -l tests/run.lua 'mutate:'` — Expected: PASS; note the wall time.

- [ ] **Step 4: Commit**

```bash
git add lua/alder/test.lua lua/alder/keys.lua plugin/alder.lua tests/mutate_spec.lua
git commit -m "test: --mutate survivors in the quickfix list (<leader>am, :AlderMutate)"
```

---

### Task 9: `repl.lua` — terminal REPL and send (`<leader>ax`, `<leader>as`)

**Files:**
- Create: `lua/alder/repl.lua`, `tests/repl_spec.lua`
- Modify: `lua/alder/keys.lua`, `lua/alder/health.lua`, `lua/alder/commands.lua:60-66`, `plugin/alder.lua`, `tests/health_spec.lua`, `tests/keys_spec.lua`

**Interfaces:**
- Consumes: `buf.module_name`, `root.for_buf`, `commands.find`.
- Produces: `repl.argv(app_root, module) → list`; `repl.join(lines) → string` (common indent stripped, ` \` continuations, trailing newline); `repl.open(app_root, module) → session`; `repl.send(app_root, text)`; `repl.for_current()`, `repl.send_current(mode)`; `commands.has_flag(words, flag) → bool`; `keys.MODES` (per-action modes); `keys.actions.repl`, `keys.actions.send`.

- [ ] **Step 1: Write the failing tests**

`tests/repl_spec.lua`:

```lua
local H = require('helpers')
local R = require('alder.repl')

test('repl: argv adds --in for a module and nothing without one', function()
  assert(vim.deep_equal(R.argv('/app', 'Todo'), { 'alder', 'repl', '--in', 'Todo' }), vim.inspect(R.argv('/app', 'Todo')))
  assert(vim.deep_equal(R.argv('/app', nil), { 'alder', 'repl' }))
end)

test('repl: join strips the common indent and continues lines with a backslash', function()
  assert(R.join({ '    xs', '      |> List.map f' }) == 'xs \\\n  |> List.map f\n', vim.inspect(R.join({ '    xs', '      |> List.map f' })))
  assert(R.join({ 'x' }) == 'x\n')
  assert(R.join({ '  a', '', '  b' }) == 'a \\\n \\\nb\n')
end)

local function wait_banner(s)
  return H.wait(30000, function() return s.ready end)
end

test('repl: open starts alder repl --in in a terminal split, and a second open reuses it', function()
  local app = H.new_app()
  local s = R.open(app, 'App')
  assert(s and s.job and s.buf, 'no session')
  assert(vim.bo[s.buf].buftype == 'terminal')
  assert(wait_banner(s), 'no banner: ' .. table.concat(vim.api.nvim_buf_get_lines(s.buf, 0, -1, false), '\n'))
  local lines = table.concat(vim.api.nvim_buf_get_lines(s.buf, 0, -1, false), '\n')
  assert(lines:find('alder repl: demo, in App', 1, true), lines)
  local again = R.open(app, 'Other')
  assert(again.job == s.job, 'a second open started a new REPL')
  vim.fn.jobstop(s.job)
end)

test('repl: send before the banner is queued, not lost', function()
  local app = H.new_app()
  local s = R.open(app, nil)
  R.send(app, '1 + 2\n')
  assert(wait_banner(s), 'no banner')
  assert(H.wait(60000, function()
    return table.concat(vim.api.nvim_buf_get_lines(s.buf, 0, -1, false), '\n'):find('3 : Int', 1, true) ~= nil
  end), table.concat(vim.api.nvim_buf_get_lines(s.buf, 0, -1, false), '\n'))
  vim.fn.jobstop(s.job)
end)
```

Add to `tests/health_spec.lua` a case that `require('alder.commands').has_flag({ 'repl' }, '--in')` is true with the current binary, and to `tests/keys_spec.lua` that `<leader>as` is mapped in both `'n'` and `'x'` mode on an attached buffer (`vim.fn.maparg(vim.keycode('<leader>as'), 'x', false, true).buffer == 1`).

Run: `nvim --headless -u tests/minimal.lua -l tests/run.lua 'repl:'` — Expected: FAIL.

- [ ] **Step 2: `commands.has_flag`**

In `lua/alder/commands.lua`, make `flag` reachable and add:

```lua
-- has_flag reports whether the command at words lists the flag (`alder help --json`).
function M.has_flag(words, name)
  return flag(M.find(words), name) ~= nil
end
```

- [ ] **Step 3: Implement `lua/alder/repl.lua`**

```lua
-- repl runs `alder repl` in a terminal split, one per app, and sends text to it.
local root = require('alder.root')

local M = {}

local function real(p) return vim.uv.fs_realpath(p) or vim.fs.normalize(p) end

M.sessions = {} -- real app root → { job, buf, ready, queue }

-- argv is the command line: `alder repl --in <module>` inside a module, `alder repl` outside.
function M.argv(app_root, module)
  local argv = { require('alder.config').options.cmd, 'repl' }
  if module then vim.list_extend(argv, { '--in', module }) end
  return argv
end

-- join makes one REPL entry of lines: the common indent stripped, each line but the last
-- ended by the REPL's ` \` continuation, a newline at the end.
function M.join(lines)
  local indent
  for _, l in ipairs(lines) do
    if l:match('%S') then
      local n = #l:match('^%s*')
      indent = indent and math.min(indent, n) or n
    end
  end
  local out = {}
  for _, l in ipairs(lines) do table.insert(out, l:sub((indent or 0) + 1)) end
  return table.concat(out, ' \\\n') .. '\n'
end

local function alive(s)
  return s and s.job and vim.fn.jobwait({ s.job }, 0)[1] == -1 and vim.api.nvim_buf_is_valid(s.buf)
end

local function win_of(buf)
  for _, w in ipairs(vim.api.nvim_list_wins()) do
    if vim.api.nvim_win_get_buf(w) == buf then return w end
  end
end

local function flush(s)
  for _, text in ipairs(s.queue) do vim.fn.chansend(s.job, text) end
  s.queue = {}
end

-- open starts the app's REPL in a bottom split (speaking as module, when given) or focuses
-- the running one; the module of a running REPL is not changed (`:in` inside it does that).
function M.open(app_root, module)
  local key = real(app_root)
  local s = M.sessions[key]
  if alive(s) then
    local w = win_of(s.buf)
    if w then vim.api.nvim_set_current_win(w) else vim.cmd('botright sbuffer ' .. s.buf) end
    return s
  end
  vim.cmd('botright new')
  s = { ready = false, queue = {} }
  s.buf = vim.api.nvim_get_current_buf()
  s.job = vim.fn.jobstart(M.argv(app_root, module), {
    term = true,
    cwd = app_root,
    on_stdout = function(_, data)
      if s.ready then return end
      for _, l in ipairs(data or {}) do
        if l:find('alder repl:', 1, true) then
          s.ready = true
          vim.schedule(function() flush(s) end)
          return
        end
      end
    end,
    on_exit = function() if M.sessions[key] == s then M.sessions[key] = nil end end,
  })
  if s.job <= 0 then
    vim.notify('alder repl: could not start `' .. table.concat(M.argv(app_root, module), ' ') .. '`', vim.log.levels.ERROR)
    return nil
  end
  M.sessions[key] = s
  vim.cmd('startinsert')
  return s
end

-- send gives text to the app's REPL, starting one (in app scope) when none runs; text sent
-- before the banner waits for it.
function M.send(app_root, text)
  local key = real(app_root)
  local s = M.sessions[key]
  if not alive(s) then
    local cur = vim.api.nvim_get_current_win()
    s = M.open(app_root, nil)
    vim.cmd('stopinsert')
    vim.api.nvim_set_current_win(cur)
    if not s then return end
  end
  if s.ready then vim.fn.chansend(s.job, text) else table.insert(s.queue, text) end
end

-- for_current opens the REPL of the current buffer's app, in its module.
function M.for_current()
  local r = root.for_buf(0)
  if not r then
    vim.notify('alder: no `app/` directory above this file', vim.log.levels.ERROR)
    return
  end
  local b = vim.api.nvim_get_current_buf()
  local module = vim.bo[b].filetype == 'alder' and require('alder.buf').module_name(b) or nil
  if module and not require('alder.commands').has_flag({ 'repl' }, '--in') then
    vim.notify('alder repl: this alder has no --in; update it (opening in app scope)', vim.log.levels.WARN)
    module = nil
  end
  M.open(r, module)
end

-- send_current sends the current line ('n') or the visual selection ('x') to the REPL.
function M.send_current(mode)
  local r = root.for_buf(0)
  if not r then
    vim.notify('alder: no `app/` directory above this file', vim.log.levels.ERROR)
    return
  end
  local lines
  if mode == 'x' then
    vim.cmd('normal! \27') -- leave visual mode so the marks are set
    local a, b = vim.fn.line("'<"), vim.fn.line("'>")
    lines = vim.api.nvim_buf_get_lines(0, a - 1, b, false)
  else
    lines = { vim.api.nvim_get_current_line() }
  end
  M.send(r, M.join(lines))
end

return M
```

- [ ] **Step 4: Keys with modes, command, health**

`lua/alder/keys.lua`: add `M.MODES = { send = { 'n', 'x' } }` and change `attach`/`detach`:

```lua
function M.attach(buf)
  for name, lhs in pairs(mapped()) do
    for _, mode in ipairs(M.MODES[name] or { 'n' }) do
      local rhs = M.actions[name]
      if mode == 'x' then rhs = function() M.actions[name]('x') end end
      vim.keymap.set(mode, lhs, rhs, { buffer = buf, desc = DESC[name] })
    end
  end
  …
end

function M.detach(buf)
  for name, lhs in pairs(mapped()) do
    for _, mode in ipairs(M.MODES[name] or { 'n' }) do pcall(vim.keymap.del, mode, lhs, { buffer = buf }) end
  end
end
```

Actions: `repl = function() require('alder.repl').for_current() end`, `send = function(mode) require('alder.repl').send_current(mode or 'n') end`; `DESC.repl = 'alder: the REPL in this module'`, `DESC.send = 'alder: send to the REPL'`.

`plugin/alder.lua`:

```lua
vim.api.nvim_create_user_command('AlderRepl', function()
  require('alder.repl').for_current()
end, { desc = 'alder repl --in <this module> in a split (focuses a running one)' })
```

`lua/alder/health.lua`, inside the `help.code == 0 and decode_tree` branch after the `h.ok`:

```lua
if require('alder.commands').has_flag({ 'repl' }, '--in') then
  h.ok('`alder repl --in` (the REPL speaks as the buffer\'s module)')
else
  h.warn('this alder has no `repl --in`: <leader>ax opens the REPL in app scope; build a newer one')
end
```

In `lua/alder/commands.lua`, the `TERMINAL` branch is unchanged (`:Alder repl` still opens a plain terminal); nothing else.

Run: `PATH=$HOME/go/bin:$PATH nvim --headless -u tests/minimal.lua -l tests/run.lua 'repl:'` then `… 'keys:'`, `… 'health:'` — Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add lua/alder/repl.lua lua/alder/keys.lua lua/alder/health.lua lua/alder/commands.lua plugin/alder.lua tests/repl_spec.lua tests/keys_spec.lua tests/health_spec.lua
git commit -m "repl: alder repl --in in a terminal split, send line/selection (<leader>ax, <leader>as)"
```

---

### Task 10: README, remote, CI base; whole-suite run

**Files:**
- Modify: `README.md` (Commands, Keys), `.github/workflows/test.yml` (only if needed)
- alder: `.github/workflows/ci.yml` (only if needed)

- [ ] **Step 1: README**

Under `## Commands` add rows for `:AlderTest [args]`, `:AlderMutate [args]`, `:AlderRepl`; under `## Keys` add `test`, `test_at`, `mutate`, `repl`, `send` with the defaults and one line each (copy the wording of the book's "What you get" bullets from Task 5), and note `send` works in normal and visual mode. Mention that `repl --in` needs an alder built from this change or later (`:checkhealth alder` says).

- [ ] **Step 2: Whole suite**

Run: `cd ~/projects/alder.nvim && PATH=$HOME/go/bin:$PATH nvim --headless -u tests/minimal.lua -l tests/run.lua`
Expected: `N tests, 0 failed` with N ≥ 115 + the new cases.

- [ ] **Step 3: Commit**

```bash
git add README.md
git commit -m "docs: test, mutate and REPL commands and keys"
```

- [ ] **Step 4: Remote (ask first)**

Creating a public GitHub repository is outward-facing: **ask the user before running** `gh repo create tapegram/alder.nvim --public --source . --push`. If they prefer private, use `--private`. Then `git push -u origin main`.

- [ ] **Step 5: CI paths-filter base (alder)**

Read dorny/paths-filter v3's README section "Inputs → base" (`gh api repos/dorny/paths-filter/contents/README.md --jq .content | base64 -d | grep -n -A6 '^### base\|base:'` or open it in the browser). Its documented default is: on `push` to a branch, compare with the **merge base against the default branch**… if that is what it says, **leave `ci.yml` alone** and note it in the final hand-back. Only if the default compares against the previous commit of the same branch, add `base: ${{ github.event.repository.default_branch }}` to both filter steps and commit `ci: compare branch pushes against main in paths-filter`.

- [ ] **Step 6: Hand back**

Report: both suites' closing lines, the mutate integration test's wall time, whether CI was changed, and the remote's URL.
