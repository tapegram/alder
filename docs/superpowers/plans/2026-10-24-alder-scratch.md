# `alder scratch` (engine + CLI) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A Unison-style scratch file: `*scratch.alder` outside `app/`, where each `> expr` line is evaluated (pure values only), run by `alder scratch [file] [--watch] [--json]`.

**Architecture:** Hoist eval's run helpers into `internal/scratch`, then add a file engine there (`ParseFile` + `RunFile`): one `scratch.Check`, one `CompileCheckedWith` with a generated multi-watch main, one `RunTethered` per run. The CLI is the engine's first consumer; the LSP and alder.nvim are its second (Plan 2), so the engine lives in `internal/` and returns JSON-ready results.

**Tech Stack:** Go; existing `internal/scratch`, `internal/gen`, `internal/watch`, `internal/snippets`, `internal/syntax`.

**Spec:** `docs/superpowers/specs/2026-10-24-alder-scratch-design.md` (the #19 proposal; its §0 decisions override the proposal).

**This is Plan 1 of 2.** Plan 2 (written after this one merges) covers spec §5 items 4–5 and the REPL steps: LSP `didSave` + `alder/scratch` notification + per-file analysis, tree-sitter support for `>` lines, `alder.nvim` `scratch.lua` (virtual text/lines), moving the REPL keys to the scratch file, and the REPL deprecation banner. The decision says the REPL is deprecated *when scratch ships to the editor*, so Plan 1 leaves the REPL as it is.

## Global Constraints

- v1 is **pure expressions only**: a watch whose type is `Query a`, `Task a` or `Select a` is never run. It is shown as `<Query>`/`<Task>`/`<Select>` with its type and the note `not run: scratch evaluates pure values only`. There is no `>!` in v1: a `>!` line is a diagnostic.
- Scratch file = base name ends in `scratch.alder` **and** the path is not under the app's `app/` directory.
- `alder new` scaffolds `scratch.alder` at the project root. It is **committed**, so it gets no `.gitignore` entry.
- Header: `in Module.Name` as the first non-blank, non-comment line selects in-module mode (`scratch.Module.In`). Without it the module is `Scratch`, or `--module Name`.
- Values are capped at 64 KB (`MaxValue = 64 << 10`), with `truncated: true`. The whole run has `scratch.Timeout` (60 s).
- The build output goes to `build/<app>/scratch/file-<first 12 hex of sha256(abs file path)>/` (purpose `file-<hash>`), and builds use `Options{Tests: true}` as eval does.
- Diagnostics in the file are positioned in the file, named as the user gave the path (`scratch.alder:7:3`). App diagnostics keep their own files.
- No change to `alder eval`/`alder repl` behaviour. The Task 1 refactor must leave their tests green unchanged.

## Review Focus

1. **A watch that panics at run time** (`> List.head []`-style crash, or a `Debug.crash`-like failure) must fail only its own line, and later watches still show values. This is why watches are thunks (Task 3). Test: `TestRunFileWatchPanicIsolated`.
2. **A stack overflow / timeout mid-file**: earlier watches keep their values, the crashing one gets the crash message, and later ones get `not run: the program crashed at line N`. Test: `TestRunFileCrashMidFile`.
3. **A file edit that fails to parse** (half-typed expression on save under `--watch`) must print diagnostics and keep watching, not exit. Test: `TestScratchWatchSurvivesBadSave`.
4. **A `> ` inside a multi-line string or after an indented continuation** is not a watch. Only column-1 item starts (`snippets.Starts`) are. Test: `TestParseFileIndentedGtIsContinuation`.
5. **`alder fmt` and the LSP on a scratch file**: today both would treat `> 1 + 1` as a parse error. fmt must skip scratch files in a walk and refuse them by name, and the LSP must not publish parse errors or format them. Tests in Task 6.

---

### Task 1: Hoist eval's run helpers into `internal/scratch`

Pure refactor; eval and REPL behave identically.

**Files:**
- Create: `internal/scratch/main.go` (from `cmd/alder/scratchmain.go`, which is deleted)
- Create: `internal/scratch/eval.go` (GoValue, TypeParams, ClassifyRun, SourceMap)
- Modify: `cmd/alder/eval.go`, `cmd/alder/repl.go`, `cmd/alder/evalinput.go` (callers; `evalSource` embeds/uses `scratch.SourceMap`)
- Test: move the unit tests of the moved functions from `cmd/alder/eval_test.go` / `evalinput_test.go` into `internal/scratch/eval_test.go`

**Interfaces (Produces):**
- `type MainKind int`; `const MainPure, MainQuery, MainTask MainKind`; `func Main(kind MainKind, withCaps bool, it string) string` (was `scratchMain`; templates unchanged)
- `func GoValue(mod, name string, sig types.Type, thunk bool) string`: was `evalIt(mod, sig)` with `name` in place of the fixed `"it"`. With `thunk` set, a non-function value is called (`app.Scratch__it3()`, or `app.Scratch__it3[any]()` when generic). Eval passes `false`.
- `func TypeParams(t types.Type) []string` (was `typeParamsOf`)
- `type RunOutcome int`; `const RunOK, RunFailed, RunCrashed, RunInternal`; `func ClassifyRun(errOut string, exit int) (string, RunOutcome)`
- `type SourceMap struct{ Lines []string; InLine, Shift []int }` and `func (m SourceMap) Position(d diag.Diagnostic, mapLine func(diag.Diagnostic) (diag.Diagnostic, bool), file string) diag.Diagnostic`. This is the span/fix/`MapLineRef` part of `evalMapDiag`. Eval's message rewrites about `it` stay in `evalMapDiag`, which now calls `Position(d, mapLine, "input")`.
- `devDBHint` stays in `cmd/alder` (it imports `alder/rt/db`).

- [ ] **Step 1:** Move the code and tests and update the callers. Keep the doc comments, adjusting their names.
- [ ] **Step 2:** Run `go vet ./... && go test ./internal/scratch ./cmd/alder -run 'Eval|Repl|Scratch'`. Expected: PASS, with no test assertion changed except for renamed identifiers.
- [ ] **Step 3:** Commit `scratch: hoist eval's run helpers into internal/scratch (#19)`.

---

### Task 2: `ParseFile` and `IsScratchFile`

**Files:**
- Create: `internal/scratch/file.go`
- Test: `internal/scratch/file_test.go`

**Interfaces:**
- Consumes: `SourceMap` (Task 1), `snippets.Starts`, `syntax.Tokens`.
- Produces:
```go
// IsScratchFile: rel is slash-separated, relative to the project root.
func IsScratchFile(rel string) bool // base ends in "scratch.alder" and rel is not under "app/"

type Watch struct {
	Line, End int    // 1-based: the `>` line and the watch's last line (continuations included)
	Name      string // "it1", "it2", … in file order
	Expr      string // as written, without the leading `> `; lines joined by "\n"
}

type File struct {
	Name    string   // the path as given, for diagnostics
	Lines   []string
	In      string   // the `in` header's module; "" without one
	Source  string   // module source: same line count as Lines; header line blanked; each watch's first line `itK = expr`
	Watches []Watch
	Map     SourceMap // InLine identity; Shift on each watch's first line = 2 - len("itK = ")
}

func ParseFile(name, text string) (File, []diag.Diagnostic)
```

Rules: a watch starts at a line that `snippets.Starts` marks as a start and that is `>` or begins with `> `. It runs until the next start. The header is recognised only on the first start line, as `in <appModuleRE name>`. The diagnostics are all `Severity: "error"`, with no `Code`, positioned in the file:
- `>!` → "`>!` (run a Task) is not supported yet: scratch evaluates pure values only"
- `>` with nothing after it → "a watch needs an expression: `> expr`"
- `in` with a bad name → "`in` wants an app module name such as Todo or Todos.Index"
- `in` that is not the first line → "`in Module` must be the file's first line"
- any identifier token matching `^it[0-9]+$` (through `syntax.Tokens`, so comments and strings are skipped), declared or referenced → "`itN` is reserved for the file's watches; rename it". A watch cannot refer to another watch, so the expression is named.

- [ ] **Step 1: Write failing tests** (table-driven where natural):
  - `TestIsScratchFile`: `scratch.alder`, `todos.scratch.alder`, `experiments/auth.scratch.alder` → true; `app/scratch.alder`, `app/X/foo.scratch.alder`, `Scratch.alder`, `scratch.alder.bak` → false.
  - `TestParseFileWatches`: for the text `sample = 2\n\n> sample + 1\n> List.map\n    (\x -> x)\n    [1]\n`, there are 2 watches `{Line 3, End 3, Name "it1", Expr "sample + 1"}` and `{Line 4, End 6, Name "it2"}`. `Source` line 3 is `it1 = sample + 1`, and `Map.Shift[2] == -4`. `len(strings.Split(Source,"\n"))` equals the input's line count.
  - `TestParseFileInHeader`: `-- notes\nin Todos.Index\n> countOpen []\n` → `In == "Todos.Index"`, and Source line 2 is blank.
  - `TestParseFileIndentedGtIsContinuation`: `x =\n    1\n  > 2\n` → no watches (Review Focus 4).
  - `TestParseFileDiagnostics`: one case per message above, asserting line:col and message.
- [ ] **Step 2:** Run `go test ./internal/scratch -run 'ParseFile|IsScratchFile'`. Expected: FAIL (undefined).
- [ ] **Step 3:** Implement `file.go`.
- [ ] **Step 4:** Run the same command. Expected: PASS.
- [ ] **Step 5:** Commit `scratch: parse scratch files (#19)`.

---

### Task 3: `RunFile`: check, multi-watch main, run

**Files:**
- Modify: `internal/gen/gen.go`: add `Options.Thunks map[string]bool` (qualified value names such as `"Scratch.it3"`). `newGen` marks each named non-function constant as a thunk. Today a plain constant is a Go package variable initialised at package init, before any `recover` can catch its panic.
- Modify: `internal/scratch/scratch.go`: add `Options.Thunks map[string]bool`, passed through to `gen.Options`.
- Create: `internal/scratch/filerun.go`
- Test: `internal/scratch/filerun_test.go` (on `examples/todos`, in a temp copy as the existing scratch tests do), plus a `gen` test that a thunked constant emits a func

**Interfaces:**
- Consumes: `File`, `ParseFile` (Task 2), `GoValue`, `ClassifyRun`, `SourceMap.Position` (Task 1); `Check`, `CompileCheckedWith`, `RunTethered`.
- Produces:
```go
const MaxValue = 64 << 10

type WatchResult struct {
	Line      int    `json:"line"`
	Expr      string `json:"expr"`
	Type      string `json:"type,omitempty"`  // "" when the file did not check far enough
	Value     string `json:"value,omitempty"`
	Error     string `json:"error,omitempty"`
	Note      string `json:"note,omitempty"`  // effect watches: "not run: scratch evaluates pure values only"
	Truncated bool   `json:"truncated,omitempty"`
}

type FileResult struct {
	Watches []WatchResult
	Diags   []diag.Diagnostic // the file's (positioned in File.Name) then the app's, as checked
	Ran     bool              // the program was built and run
	Note    string            // why nothing (or not everything) ran
	Warning string            // Build.Warning (unwired capabilities)
	Err     error             // I/O, go build or internal failure; never a diagnostic
}

type FileOptions struct {
	Module  string        // the scratch module's name without an `in` header; "" is "Scratch"
	Out     string        // the build directory (see Global Constraints)
	Timeout time.Duration // 0 is Timeout
	Build   Options       // Waiting/Fetching; RunFile sets Tests and Thunks
	OnTypes func(FileResult) // called once after the check with every watch's Type (and the diagnostics), before building; may be nil
}

func RunFile(ctx context.Context, appDir string, f File, o FileOptions) FileResult
```

Flow:
1. `Check(appDir, &Module{Name, In: f.In, Source: f.Source, Infer: true})`.
   - `NoModuleError` becomes a file diagnostic on the header line: "no module X in the app".
   - `ModuleExistsError` becomes a diagnostic at 1:1 with `Taken()` + "; pass --module <Name>".
   - Session diagnostics are mapped through `f.Map.Position(d, checked.MapDiag, f.Name)`, dropping the prefix "in the definition of `itK`: ".
2. Types: `checked.Result.Values[<mod>.itK]` through `U.Apply` and `types.String`. Then `OnTypes`.
3. Stop with no build, and `Note` set, in each of these cases:
   - any app error: "the app has errors; fix them and save";
   - any file error: no note;
   - `Result.Holes > 0`: "the file has N typed hole(s); fill them to run it";
   - no pure watch.
4. Main: a generated `package main` that evaluates each pure watch in file order through `show(k, func() string { return rt.Show(<GoValue(mod, "itK", sig, true)>) })`. `show` writes the start marker `\x1e<k>\n` to stdout before evaluating. It then writes `\x02` + value on success, or `\x15` + the recovered message on a panic. Effect watches are not in the main.
5. `RunTethered` under `context.WithTimeoutCause(ctx, timeout, …)`, then parse the frames:
   - a frame with a status byte gives its value or error;
   - a value longer than `MaxValue` is cut there, with `Truncated`;
   - if `ClassifyRun` says `RunCrashed`, the last started frame without a status byte gets the crash message, every later pure watch gets "not run: the program crashed at line N", and `Note` repeats the crash;
   - `RunFailed` with no frame at all (a panic during package init, such as the user's own constant) sets `Note` to "the program failed before any watch ran: <msg>".

- [ ] **Step 1: Write failing tests:**
  - `TestGenThunksOption`: emitting with `Thunks{"M.x": true}` makes `x` a `func`.
  - `TestRunFilePure`: `> 1 + 1` → `{Value "2", Type "Int"}`; `> Todo.countOpen` (or another exported function) → `Value "<function>"`; a polymorphic `> []` → `Value "[]"`.
  - `TestRunFileInModule`: `in <a todos module>` then a watch that uses an unqualified name of that module gets its value.
  - `TestRunFileEffectNotRun`: a `Db.all (…)` watch → `Value "<Query>"`, Type `Query (List …)`, the Note from Global Constraints, `Error ""`. A pure watch in the same file still runs.
  - `TestRunFileWatchPanicIsolated` (Review Focus 1): a crashing watch between two good ones gives its own `Error`, and both neighbours have values.
  - `TestRunFileCrashMidFile` (Review Focus 2): watches 1, 2 (unbounded recursion), 3 → value; "stack overflow (unbounded recursion?)"; "not run: the program crashed at line <2's line>".
  - `TestRunFileDiagnosticsPositioned`: `> 1 + "a"` on line 4 gives an error diagnostic with `Span.File == "scratch.alder"` and `Span.Start.Line == 4`, a column inside the expression as written, and `Ran == false`.
  - `TestRunFileHole`: a hole → Types set, `Ran == false`, and Note mentions the hole.
  - `TestRunFileTruncates`: `> String.repeat 70000 "a"` (or the stdlib equivalent) gives `len(Value) == MaxValue` and `Truncated`.
  - `TestRunFileOnTypes`: the callback runs once, before the values, with the `Type`s set and the `Value`s empty.
- [ ] **Step 2:** Run `go test ./internal/gen -run Thunks && go test ./internal/scratch -run RunFile`. Expected: FAIL.
- [ ] **Step 3:** Implement.
- [ ] **Step 4:** Run the same command. Expected: PASS. Then run `go test ./internal/scratch ./internal/gen`. Expected: PASS.
- [ ] **Step 5:** Commit `scratch: run a scratch file's watches (#19)`.

---

### Task 4: `alder scratch` (one run, text and `--json`)

**Files:**
- Create: `cmd/alder/scratch.go`, `cmd/alder/scratch_test.go`
- Modify: `cmd/alder/main.go` (dispatch + usage line), `cmd/alder/helpjson.go` (entry + flags), `commandUsages`

**Interfaces:**
- Consumes: `IsScratchFile`, `ParseFile`, `RunFile`, `FileResult` (Tasks 2–3).
- Produces: `const scratchUsage = "usage: alder scratch [file] [--watch] [--json] [--build-dir DIR] [--module Name]"`; `func runScratchTo(args []string, stdout, stderr io.Writer) int` (Task 5 adds `--watch` to it).

Behaviour:
- The file defaults to `./scratch.alder`. The app dir is the nearest ancestor of the file's directory for which `dirHasApp` holds; with none, exit 2 with "alder scratch: no app found above <file>".
- If the path relative to that app dir fails `IsScratchFile`, exit 2: "alder scratch: <file> is not a scratch file (a name ending in scratch.alder, outside app/)".
- `--module` is validated with `moduleNameRE` and refused together with an `in` header, using errInFlags' wording.
- Text output: diagnostics to stderr, as `evalDiags` renders them, with the file's lines as source. Then, on stdout, for each watch in order, `<file>:<line>: <first line of Expr>` followed by an indented result:
  - `  ⇒ <value> : <Type>`, where a multi-line value's later lines are indented 4 spaces and a truncated one ends `  … (truncated at 64 KB)`;
  - `  ✗ <error>`;
  - `  <Query> : <Type>   (not run: scratch evaluates pure values only)`;
  - `  : <Type>` when the run did not happen.
  - `Note` is printed last on stderr, prefixed `alder scratch: `, and `Warning` is printed as eval prints it.
- `--json`: one object `{"watches": [...WatchResult], "diagnostics": [RenderJSON…], "ran": bool, "note"?: "...", "error"?: "..."}`. `SetEscapeHTML(false)`, as eval does.
- Exit codes: 0 when there are no error diagnostics, no hole and no watch `Error`; 1 when there are; 2 for usage, I/O or internal errors (`Err`, except `AdapterError`/`DownloadError`, which give 1 as in eval).
- Ctrl-C and SIGTERM cancel the run, as in eval, with "alder scratch: interrupted" and exit 1. `Build.Waiting` says "alder scratch: waiting for another run of this scratch file…".

- [ ] **Step 1: Write failing tests** (on a temp copy of `examples/todos` holding a `scratch.alder`):
  - `TestScratchText`: stdout contains `scratch.alder:2: 1 + 1\n  ⇒ 2 : Int`; exit 0.
  - `TestScratchJSON`: unmarshal; `watches[0].value == "2"`, `ran == true`.
  - `TestScratchErrorsExit1`: a type error → exit 1, and stderr has `scratch.alder:L:C`.
  - `TestScratchRefusesAppFile`: `app/foo.scratch.alder` → exit 2 with the message above.
  - `TestScratchDefaultFile`: no argument, cwd = the app → runs `./scratch.alder`.
  - `TestScratchNoApp`: exit 2.
  - Help JSON test: `scratch` is listed with its flags (extend the existing helpjson test's table).
- [ ] **Step 2:** Run `go test ./cmd/alder -run 'Scratch|HelpJSON'`. Expected: FAIL.
- [ ] **Step 3:** Implement.
- [ ] **Step 4:** Run the same command. Expected: PASS.
- [ ] **Step 5:** Commit `cli: alder scratch (#19)`.

---

### Task 5: `alder scratch --watch`

**Files:**
- Modify: `internal/watch/watch.go`: `func WatchWith(dir string, extra []string, interval time.Duration, stop <-chan struct{}, changed chan<- Change)`. `extra` lists absolute files whose stamps are compared too, and a change to one is `SourceChange`. `Watch` becomes `WatchWith(dir, nil, …)`.
- Modify: `cmd/alder/scratch.go`
- Test: `internal/watch/watch_test.go`, `cmd/alder/scratch_test.go`

Behaviour:
- `--watch` runs once, then reruns on every coalesced change to the app's watched files or the scratch file. It polls at the interval `alder run` uses.
- Before each run it prints `── <HH:MM:SS> ──` on stdout.
- A change during a run cancels it (ctx) and starts again.
- A run with diagnostics or failures prints them and keeps watching (Review Focus 3). This includes a file that no longer parses, and one that was deleted, which prints "alder scratch: <file> is gone; waiting for it".
- Ctrl-C exits 0. `--watch --json` prints one JSON object per run, one per line, so agents can stream it.

- [ ] **Step 1: Write failing tests:** `TestWatchWithExtraFile` (touching the extra file sends a change); `TestScratchWatchReruns` (the file is edited from `> 1` to `> 2` → output shows `⇒ 2`); `TestScratchWatchSurvivesBadSave` (a save of `> 1 +` prints a diagnostic, and a later fix prints a value). These drive `runScratchTo` with a stop channel or ctx, following how the existing `alder run` watch tests stop. No real sleeps beyond the poll interval, in line with #24's test-time rules.
- [ ] **Step 2:** Run `go test ./internal/watch ./cmd/alder -run 'WatchWith|ScratchWatch'`. Expected: FAIL.
- [ ] **Step 3:** Implement.
- [ ] **Step 4:** Run the same command. Expected: PASS.
- [ ] **Step 5:** Commit `cli: alder scratch --watch (#19)`.

---

### Task 6: Keep scratch files out of `alder fmt` and the LSP

Until Plan 2 teaches both about watches.

**Files:**
- Modify: `cmd/alder/fmt.go`: `fmtFiles` skips paths for which `scratch.IsScratchFile` holds, relative to the walked root. An explicitly named scratch file fails with "alder fmt: <file> is a scratch file; scratch files are not formatted yet" (exit 2).
- Modify: `internal/lsp`: a document whose `rel` is a scratch file is kept out of the analysis overlay and gets no diagnostics, no formatting edits (`format.go`) and no code actions. Its `didOpen`/`didChange` are tracked, but nothing is published.
- Test: `cmd/alder/fmt_test.go`, `internal/lsp/*_test.go`

- [ ] **Step 1: Write failing tests:** `TestFmtSkipsScratchFiles` (`alder fmt --check .` passes with a `scratch.alder` holding `> 1 + 1`); `TestFmtRefusesScratchFile`; `TestLSPIgnoresScratchFile` (open `scratch.alder` with `> 1 +`: no `publishDiagnostics` for it, and formatting returns null).
- [ ] **Step 2:** Run `go test ./cmd/alder -run Fmt && go test ./internal/lsp -run Scratch`. Expected: FAIL.
- [ ] **Step 3:** Implement.
- [ ] **Step 4:** Run the same command. Expected: PASS.
- [ ] **Step 5:** Commit `fmt, lsp: leave scratch files alone for now (#19)`.

---

### Task 7: Scaffold and book

**Files:**
- Create: `internal/scaffold/files/scratch.alder`, with exactly this text:
```alder
-- A scratch file: each `> expr` line is evaluated. Run `alder scratch --watch` to see results on save.
-- Everything else is ordinary Alder: define values and helpers here, then watch them.
-- Put `in Module` on the first line to work inside an app module, with its names unqualified.

> 1 + 1
```
- Modify: `internal/scaffold/scaffold.go`: the file is in both `Files` and `BareFiles`.
- Modify: `book/11-tooling.md`: add a new `### alder scratch` section before `### alder repl`, covering the file rule, watches, the `in` header, pure-only v1, `--watch`, `--json` and exit codes. Also add `scratch.alder` to `## Project layout`. Leave the REPL section alone; Plan 2 deprecates it.
- Test: `cmd/alder/new_test.go` (the scaffold lists `scratch.alder`, and `alder scratch` on a fresh `alder new` app prints `⇒ 2 : Int`); book tests

- [ ] **Step 1:** Write the failing `TestNewScaffoldsScratch` and run `go test ./cmd/alder -run NewScaffoldsScratch`. Expected: FAIL.
- [ ] **Step 2:** Implement the scaffold file and the docs.
- [ ] **Step 3:** Run `go test ./cmd/alder -run 'New' && go test ./book ./internal/scaffold`. Expected: PASS. If the book's code-block verification picks up the new section's `alder` blocks, they must pass it, or be marked the way the REPL section's transcripts are.
- [ ] **Step 4:** Run the full suite once: `go vet ./... && go test ./...`. Expected: PASS.
- [ ] **Step 5:** Commit `scaffold, book: scratch.alder (#19)`.

---

## After merge

- Run `go install ./cmd/alder`, and have the user run `:LspRestart` (because of Task 6).
- Comment on #19 with what shipped. Plan 2 (LSP + nvim + REPL deprecation) is next.

## Spec deviations (deliberate)

- Spec §4.1 asks for a diagnostic on a scratch file under `app/`. Here `alder scratch` refuses one, and the build is unchanged: it reports whatever it reports today for a stray file there. The build-side diagnostic is deferred to Plan 2, together with the LSP's file handling.
- Spec §4.2's `module Probe` header is not added. `--module Name` covers that case.
