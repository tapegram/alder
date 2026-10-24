# Scratch files in the editor (LSP + alder.nvim + REPL deprecation) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Saving a `*scratch.alder` file in Neovim shows each `> expr` watch's type, then its value, as virtual text at the end of the watch line. Tree-sitter highlights the file, and `alder repl` is deprecated in favour of the scratch file.

**Architecture:** The LSP runs `scratch.RunFile` (Plan 1's engine) on its own goroutine per open scratch document. It runs on open, on save and when the app changes, and a newer run supersedes the older one. The LSP sends two custom `alder/scratch` notifications, one after the check (types) and one after the run (values), and publishes the file's diagnostics through the ordinary `publishDiagnostics`. alder.nvim registers a handler for the notification and draws extmarks. The tree-sitter grammar treats a file without `module` as a scratch file: `>` at column 1 starts a watch, and there may be an `in` header.

**Tech Stack:** Go (`internal/lsp`, `internal/scratch`, `internal/names`), tree-sitter (grammar.js + external scanner `src/scanner.c`, CLI pinned 0.27.1 via `scripts/ts.sh`), Lua (alder.nvim, its custom `tests/run.lua` runner).

**Spec:** `docs/superpowers/specs/2026-10-24-alder-scratch-design.md` (§0 decisions override the proposal). Plan 1: `docs/superpowers/plans/2026-10-24-alder-scratch.md` (merged at d1a9e789). Its Global Constraints still hold.

**Repos:** Tasks 1–5 are in `~/projects/alder` on branch `scratch-editor-19`, and Tasks 6–7 are in `~/projects/alder.nvim` on branch `scratch-19`. alder.nvim's CI builds alder from `main`, so **merge the alder branch first**, then alder.nvim.

## Decisions taken in this plan

- **#30 JSON shape:** `scratchResult` stays in `cmd/alder`. The shared contract is `scratch.WatchResult`, which both transports already use. The envelopes differ by transport: the CLI uses `diag.RenderJSON` diagnostics, and the LSP uses LSP diagnostics plus a `phase`. Nothing moves.
- **Triggers:** a scratch document runs on `didOpen`, on its own `didSave`, on `didSave` of any other file under the root, and on the poll watcher's `SourceChange`. It does **not** run on `didChange`: values on save, as the spec says.
- **Build dir:** the LSP passes `Out = <rootReal>/build/<base(rootReal)>/scratch/<FilePurpose(<rootReal>/<rel>)>`, which is the same directory `alder scratch` uses when run from the root. The two share the lock and the Go cache.
- **Deferred to a follow-up issue (filed in Task 5):** hover, completion, definition and code actions inside scratch files (the per-file analysis of spec §5.4), formatting scratch files, and inlay hints.

## Global Constraints

- v1 is pure expressions only: no `>!`, no `alder/scratchTasks`, and no Task or Query is ever run (Plan 1's rule).
- Notification `alder/scratch`, params exactly: `{uri, version, phase: "types"|"values", watches: [WatchResult…], ran, note?, warning?, error?}`. `WatchResult` is `scratch.WatchResult` unchanged.
- The server advertises `capabilities.experimental.alderScratch = true` and `textDocumentSync.save = {includeText: false}`.
- Values are not diagnostics. Diagnostics carry only the file's errors and holes, plus one line-1 warning when the app has errors: "the app has errors; fix them and save".
- nvim inline caps: at most **8** virtual lines and **2048** bytes per value, then `… (N more lines, <leader>ao to open)`.
- nvim highlight groups (all `default link`): `AlderScratchValue`→`Comment`, `AlderScratchPending`→`NonText`, `AlderScratchError`→`DiagnosticError`, `AlderScratchNote`→`DiagnosticInfo`. Namespace `alder.scratch`.
- Deprecation text, verbatim, for the CLI and nvim: `alder repl is deprecated: use a scratch file (alder scratch --watch, or save scratch.alder in the editor); it will be removed in the next release`.
- alder.nvim tests clean up their temp dirs. New specs use `H.tmpdir()` and delete what they make, because of the known 5.5 GB/run leak.

## Review Focus

1. **Typing after saving, while the build runs.** The values arrive for an older version than the buffer. They must land on the watches they belong to, not on whatever is now on that line number. The fix is that nvim places extmarks at the types phase, which arrives in milliseconds, and updates them by id at the values phase. Test: `scratch: values follow their watch after an edit` (Task 6).
2. **Save twice quickly.** The first run must be cancelled, and its notifications and diagnostics must never arrive after the second run's. Test: `TestScratchSupersededRunPublishesNothing` (Task 4).
3. **Close or rename the file mid-run.** No notification arrives for a closed URI, and its diagnostics are cleared. Tests: `TestScratchCloseCancelsAndClears`, `TestScratchRename` (Task 4).
4. **An existing `.alder` module file that starts with a comment, or with `>=` continuation lines.** It must parse exactly as before. Scratch mode applies only to a file whose first token is not `module`. Test: the drift test over the whole repo (Task 1), plus corpus case `module file with a column-1 >= continuation`.
5. **`<leader>as` from a module buffer when scratch.alder has no `in` header.** The appended watch uses unqualified names that won't resolve. Tell the user once instead of failing silently. Test: `scratch: append warns about the in header` (Task 7).

---

### Task 1: Tree-sitter: scratch files (model: Opus)

**Files:**
- Modify: `tree-sitter-alder/grammar.js`, `tree-sitter-alder/src/scanner.c` (then regenerate `src/` with `scripts/ts.sh generate`; the generated files are committed)
- Modify: `tree-sitter-alder/queries/neovim/highlights.scm`, `queries/neovim/folds.scm`, `queries/highlights.scm`
- Create: `tree-sitter-alder/test/corpus/scratch.txt`, `tree-sitter-alder/test/highlight/scratch.alder`
- Modify: `tree-sitter-alder/drift_test.go`

**Interfaces (Produces):** node types `scratch_file`, `scratch_header` (field `module`), `watch` (field `expr`). `source_file` becomes `choice(<today's module file>, $.scratch_file)`. The query captures are `(watch ">" @punctuation.special)` and `(scratch_header "in" @keyword.import)`, and a multi-line `watch` folds.

Rules:
- A new zero-width external token `_scratch_start` is valid only at the start of `scratch_file`. The scanner emits it at the file's first token when the next word (after blanks and `--` comments) is not `module`. It sets a `scratch` flag in the scanner state, and the flag must be in `serialize`/`deserialize`.
- When `scratch` is set, `scan_newline` treats a line that starts at column 0 with `>` followed by a space, a newline or EOF as a separator (not `is_continuation`). Inside a module file nothing changes.
- `scratch_file: seq(_scratch_start, optional(_newline), optional(seq(scratch_header, _newline)), sep(_newline, choice(_item, watch)), optional(_newline))`. `watch: seq('>', field('expr', $._expression))`. Continuation lines are indented, as for any item.

- [ ] **Step 1: Write the corpus cases** in `test/corpus/scratch.txt`: a header plus two watches, one of them multi-line (an indented `List.map` call); a declaration between watches; a comment-only first line before `in`; no header; and `module file with a column-1 >= continuation` (`module M\n\nx =\n  a\n>= b\n` parses as today, a binary expression). Add `test/highlight/scratch.alder` asserting `>` is `punctuation.special` and `in` is `keyword.import`.
- [ ] **Step 2:** Add a drift case in `drift_test.go`. Every `*scratch.alder` in the repo that `scratch.ParseFile` accepts with no diagnostics (today that is `internal/scaffold/files/scratch.alder`) must parse with no ERROR or MISSING node, give one `watch` per `File.Watches` with the same start lines, and give the same top-level declarations.
- [ ] **Step 3:** Run `scripts/ts.sh test && go test ./tree-sitter-alder`. Expected: FAIL (no `scratch_file`).
- [ ] **Step 4:** Implement the grammar and scanner, then run `scripts/ts.sh generate`.
- [ ] **Step 5:** Run `scripts/ts.sh test && go test ./tree-sitter-alder`. Expected: PASS, including the unchanged drift test over every module file (Review Focus 4).
- [ ] **Step 6:** Commit `tree-sitter: scratch files, watches and the in header (#19)`.

---

### Task 2: A scratch file under `app/` gets one diagnostic (model: Sonnet)

Plan 1 deferred this from spec §4.1.

**Files:**
- Modify: `internal/names/graph.go` (`LoadOverlay`'s `parse`), `internal/diag/diag.go` (new code `ScratchInApp`, the next free `E00xx`), `internal/diag/explain.go`
- Test: `internal/names/graph_test.go` (or the file holding `LoadOverlay` tests)

**Interfaces (Produces):** `diag.ScratchInApp` (code constant).

- [ ] **Step 1:** Write `TestLoadScratchInApp`: an app holding `app/Todos.scratch.alder` with `> 1 +` (on disk, and separately as an overlay key). The result is exactly one diagnostic, `{Code: diag.ScratchInApp, Severity "error", Span.File "app/Todos.scratch.alder", line 1 col 1, Message "a scratch file does not belong under app/; move it to the project root"}`. There is no parse diagnostic for that file, and the file is not among the program's modules.
- [ ] **Step 2:** Run `go test ./internal/names -run ScratchInApp`. Expected: FAIL.
- [ ] **Step 3:** In `parse`, a `rel` whose base ends in `scratch.alder` adds the diagnostic and returns without parsing. Write an explain entry: what a scratch file is, that it lives outside `app/`, and see `alder scratch`.
- [ ] **Step 4:** Run `go test ./internal/names ./internal/diag`. Expected: PASS. If the diag test enforces "every code has an explain entry", it passes with the entry.
- [ ] **Step 5:** Commit `names: a scratch file under app/ is refused with one diagnostic (#19)`.

---

### Task 3: LSP: run scratch documents and send `alder/scratch` (model: Opus)

**Files:**
- Create: `internal/lsp/scratch.go`, `internal/lsp/scratch_test.go`
- Modify: `internal/lsp/protocol.go` (capabilities, `scratchParams`), `internal/lsp/server.go` (the `notifications` map gets `textDocument/didSave`; the `server` struct gets a `scratch scratchRuns` field), `internal/lsp/docs.go` (`didOpen` and `didSave` call into scratch), `internal/lsp/client_test.go` (`copyExample(t, name)` generalises `blogCopy`, and `c.note(method)` waits on `c.notes`), `internal/lsp/format_test.go` (`TestLSPIgnoresScratchFile` now expects the file's diagnostics, so rename it `TestLSPDoesNotFormatScratchFile` and keep only the formatting and codeAction assertions), `internal/lsp/server_test.go` (`TestInitialize_Capabilities`: `save` and `experimental.alderScratch`)

**Interfaces:**
- Consumes: `scratch.ParseFile`, `scratch.RunFile`, `scratch.FileOptions{Out, Parsed, OnTypes}`, `scratch.FilePurpose`, `scratch.IsScratchFile`, `lspDiagnostic`, `conn.send`.
- Produces:
```go
type scratchParams struct {
	URI     string                `json:"uri"`
	Version int                   `json:"version"`
	Phase   string                `json:"phase"` // "types" | "values"
	Watches []scratch.WatchResult `json:"watches"` // never null
	Ran     bool                  `json:"ran"`
	Note    string                `json:"note,omitempty"`
	Warning string                `json:"warning,omitempty"`
	Error   string                `json:"error,omitempty"` // FileResult.Err
}

var runScratchFile = scratch.RunFile // tests replace it (as loadCheckedOverlay)

func (s *server) runScratch(rel string)  // starts (or restarts, Task 4) the run for one open scratch doc; returns at once
func (s *server) scratchOut(rel string) string
func (s *server) notify(method string, params any) // the generic sender; publish keeps its own
```

Flow of one run, on a goroutine tracked by `s.bg`:
1. Take a snapshot of the doc (`s.docs.get(rel)`): its text and version. If the doc is gone, do nothing.
2. Run `ParseFile(rel, text)` so diagnostics have `Span.File == rel`.
3. Call `RunFile(ctx, s.rootReal, f, {Out: s.scratchOut(rel), Parsed: parsed, OnTypes: …})`.
4. `OnTypes` publishes the diagnostics and sends `phase: "types"`. After the return, publish the diagnostics again and send `phase: "values"`.
5. The published diagnostics are the `r.Diags` with `Span.File == rel`, through `lspDiagnostic`. When any other diagnostic in `r.Diags` is an error, add a warning at 1:1, "the app has errors; fix them and save". Send them as `publishDiagnostics{URI: <doc's URI>, Version: &version}`, outside `publish` and outside `reported`.

- [ ] **Step 1: Write the failing tests** (`scratch_test.go`, on `copyExample(t, "todos")`):
  - `TestScratchOpenSendsTypesThenValues` (this one does a real build): open `scratch.alder` = `"> 1 + 1\n"`. The first `alder/scratch` has `phase "types"`, `watches[0].type "Int"` and `value ""`. The second has `phase "values"`, `value "2"`, `ran true`, and `version` equal to the doc's version. The file's publish is empty.
  - `TestScratchSaveReruns`: with a fake `runScratchFile` that records calls and returns `value "<k>"`, `didChange` alone does not run, and `didSave` runs with the changed text.
  - `TestScratchDiagnosticsPositioned`: the fake returns a file error plus an `app/Todo.alder` error. The publish for `scratch.alder` holds the file error (0-based line from `Span`) and the 1:1 warning, and nothing about `app/Todo.alder`.
  - `TestScratchOut`: `s.scratchOut("scratch.alder") == filepath.Join(rootReal, "build", filepath.Base(rootReal), "scratch", scratch.FilePurpose(filepath.Join(rootReal, "scratch.alder")))`.
  - `TestScratchNotOverlaid`: an open scratch doc still never reaches `loadCheckedOverlay` (the Plan 1 guarantee).
- [ ] **Step 2:** Run `go test ./internal/lsp -run 'Scratch|Initialize'`. Expected: FAIL.
- [ ] **Step 3:** Implement. `didSave` handles a scratch rel, or else every open scratch doc. Wiring the poll's `SourceChange` and the supersede is Task 4. For now, a second run waits for the first.
- [ ] **Step 4:** Run `go test ./internal/lsp`. Expected: PASS.
- [ ] **Step 5:** Commit `lsp: run scratch files on open and save, alder/scratch notification (#19)`.

---

### Task 4: LSP: supersede, close, rename, app changes (model: Opus)

**Files:**
- Modify: `internal/lsp/scratch.go`, `internal/lsp/analysis.go` (the poll's `SourceChange` also calls `s.rerunScratch()`), `internal/lsp/docs.go` (`didClose`)
- Test: `internal/lsp/scratch_test.go`

**Interfaces:**
- Consumes: Task 3's `runScratch` and `runScratchFile`.
- Produces: `type scratchRuns struct{ mu sync.Mutex; byRel map[string]*scratchRun }`, `type scratchRun struct{ seq uint64; cancel context.CancelFunc }`, `func (s *server) rerunScratch()` (all open scratch docs), `func (s *server) stopScratch(rel string)`.

Rules:
- `runScratch(rel)` cancels the rel's current run and starts one with `seq+1`.
- Every send for a run (both notifications, both publishes) happens under `scratchRuns.mu`, and only when the run's `seq` is still current and its rel is still open. A superseded run therefore sends nothing at all.
- `didClose` of a scratch rel calls `stopScratch`. That cancels the run, deletes the entry, and publishes an empty diagnostic list for the URI.
- Closing `s.stop` cancels every run. `serve` exits only after `s.bg.Wait()`, so tests leave no goroutine.

- [ ] **Step 1: Write the failing tests.** All use a fake `runScratchFile` that blocks on a channel until the test releases it, honours `ctx`, and records each `ctx.Err()`:
  - `TestScratchSupersededRunPublishesNothing` (Review Focus 2): save, then save again while the first run blocks. Release both. The first run's ctx is cancelled, and every received `alder/scratch` and publish for the file carries the second run's version.
  - `TestScratchCloseCancelsAndClears` (Review Focus 3): open, block, close. The ctx is cancelled, the last publish for the URI is empty, and `c.notes` has no `alder/scratch` after the close (checked after `c.sync()`).
  - `TestScratchRename` (#30): open `a.scratch.alder`, close it, then open `b.scratch.alder`. `a` gets an empty publish, `b` runs, and `byRel` has only `b`.
  - `TestScratchRerunsOnAppSave`: with scratch open, `didSave` of `app/Todo.alder` runs scratch again.
  - `TestScratchRerunsOnPoll`: with `Options{Poll: 20ms}` and scratch open, touching `app/Todo.alder` on disk runs scratch again.
  - `TestScratchShutdownWaits`: shutting down with a blocked run returns, and the run's ctx is cancelled.
- [ ] **Step 2:** Run `go test ./internal/lsp -run Scratch -race`. Expected: FAIL.
- [ ] **Step 3:** Implement.
- [ ] **Step 4:** Run `go test ./internal/lsp -race -count=3 -run Scratch`, then `go test ./internal/lsp`. Expected: PASS.
- [ ] **Step 5:** Commit `lsp: supersede, close and rerun scratch runs (#19)`.

---

### Task 5: Deprecate `alder repl`; book; follow-up issue (model: Sonnet)

**Files:**
- Modify: `cmd/alder/repl.go` (after the banner, write the Global Constraints deprecation line to **stderr**, so nvim's banner detection on stdout is unaffected), `cmd/alder/main.go` usage line and `cmd/alder/helpjson.go` (the repl description ends ` (deprecated: use alder scratch)`)
- Modify: `book/11-tooling.md`:
  - `### alder repl` shrinks to the deprecation note plus a table moving each REPL feature to its scratch equivalent (spec §4.7: `let`→declaration, `:type`→the types phase, `:run`/`:run!`→not in v1, `:html`→`>` on Html, `:reload`→save, `:reset`→delete lines, `:in`→the `in` header).
  - `### alder scratch` turns its main transcript into a verified block (```` ```text command=guestbook ````, `$ alder scratch` on the scaffold's own `scratch.alder`). Recheck every other transcript in the section by running it (#30).
  - `## Editor support` gets a `### Scratch files in the editor` section: the two phases, the virtual text, the keys from Task 7, and what is deferred.
- Modify: `book/13-compiler.md`. The book also teaches internals, so add a short subsection on how the LSP runs a scratch file: a goroutine per document, supersede by `seq`, two notifications, the shared build dir and lock, and why values are not diagnostics.
- Test: `cmd/alder/repl_test.go`, `book` tests

- [ ] **Step 1:** Write the failing `TestReplPrintsDeprecation` (stderr contains the line, and stdout's banner is unchanged). Extend the helpjson test so `repl`'s description ends as above.
- [ ] **Step 2:** Run `go test ./cmd/alder -run 'Repl|HelpJSON'`. Expected: FAIL.
- [ ] **Step 3:** Implement the CLI changes and write the docs.
- [ ] **Step 4:** Run `go test ./cmd/alder -run 'Repl|HelpJSON' && go test ./book`. Expected: PASS, including the new `command=` transcript.
- [ ] **Step 5:** Run the full suite once: `go vet ./... && go test ./...`. Expected: PASS.
- [ ] **Step 6:** File the follow-up issue (labels `tooling`). Title: "scratch: editor features inside scratch files". Body: hover, completion, definition and code actions via a per-file analysis (spec §5.4), `alder fmt` for scratch files, and inlay hints for plugin-less editors. Link it from #19.
- [ ] **Step 7:** Commit `repl: deprecate in favour of scratch files; book (#19)`.

---

### Task 6: alder.nvim: render `alder/scratch` (model: Sonnet)

**Files:**
- Create: `lua/alder/scratch.lua`, `tests/scratch_spec.lua`
- Modify: `lua/alder/lsp.lua` (`vim.lsp.start` config gains `handlers = {['alder/scratch'] = require('alder.scratch').handler}`), `plugin/alder.lua` (highlight groups, defined with `default = true`)

**Interfaces (Produces):**
```lua
M.ns                         -- nvim_create_namespace('alder.scratch')
M.handler(err, params, ctx)  -- LSP notification handler
M.render(buf, params)        -- the testable core; handler resolves buf from params.uri
M.marks                      -- [buf] = { version = n, ids = { [watch index] = extmark id }, watches = {...} }
M.value_at(buf, row)         -- the WatchResult whose line..end contains row (1-based), or nil
M.open_value()               -- float with the full value of the watch at the cursor
```

Rendering rules:
- **Types phase:** if `params.version ~= vim.lsp.util.buf_versions[buf]`, drop it. Otherwise clear the namespace and place one extmark per watch at `line-1`, with `virt_text_pos = 'eol'`. The text is `  : <type>  …` (`AlderScratchPending`), or `  <note>` (`AlderScratchNote`) when the watch has no type. Remember the ids by index.
- **Values phase:** if `M.marks[buf].version == params.version`, update each mark by id with `nvim_buf_set_extmark(buf, ns, row_of(id), 0, {id = id, …})`, so it stays on its watch after edits (Review Focus 1). If there are no marks for that version and the version matches the buffer, place them fresh. Otherwise drop it.
- What a value shows:
  - success: `  ⇒ <first line> : <type>` (`AlderScratchValue`), with the rest of the lines as `virt_lines` under the case's caps from Global Constraints, then `  … (N more lines, <leader>ao to open)`; a `truncated` value adds `(truncated at 64 KB)`;
  - error: `  ✗ <error>` (`AlderScratchError`);
  - effect watch: `  <value> : <type>  (<note>)` (`AlderScratchNote`).
- `params.note`, `params.error` or `params.warning` (the last one shown only once per buffer) go on line 1 as an `AlderScratchNote` (or `AlderScratchError`) virt_text.
- `TextChanged`/`TextChangedI` in a buffer with marks re-highlights every value chunk as `AlderScratchPending` (stale). Register this once per buffer, in `render`.

- [ ] **Step 1: Write the failing tests** (`tests/scratch_spec.lua`). They call `M.render` with hand-built params on a scratch buffer and set `vim.lsp.util.buf_versions[buf]` themselves:
  - `scratch: types then values`: the eol chunks read `: Int  …`, then `⇒ 2 : Int`.
  - `scratch: values follow their watch after an edit` (Review Focus 1): types at v1, two blank lines inserted above the watch (v2), then values for v1. The value mark is on the moved line.
  - `scratch: stale types are dropped`: types for v1 while the buffer is at v2 → no marks.
  - `scratch: multi-line value is capped`: a 20-line value gives 8 virt_lines, and the last one reads `… (12 more lines, <leader>ao to open)`.
  - `scratch: error and effect watches`.
  - `scratch: edit dims values`: after `TextChanged` the value's hl group is `AlderScratchPending`.
  - `scratch: open_value shows the whole value` in a float.
  - `scratch: end to end` (real `alder lsp`, real build, generous `H.wait` timeout): a new app's `scratch.alder` opened and saved shows `⇒ 2 : Int` on line 5. It cleans up its tmpdir.
- [ ] **Step 2:** Run `nvim --headless -u tests/minimal.lua -l tests/run.lua scratch`. Expected: FAIL.
- [ ] **Step 3:** Implement.
- [ ] **Step 4:** Run the same command, then the whole suite with `nvim --headless -u tests/minimal.lua -l tests/run.lua`. Expected: PASS.
- [ ] **Step 5:** Commit `scratch: render alder/scratch as virtual text (alder#19)`.

---

### Task 7: alder.nvim: keys move to the scratch file; deprecate the REPL (model: Sonnet)

**Files:**
- Modify: `lua/alder/scratch.lua`, `lua/alder/keys.lua`, `lua/alder/config.lua`, `plugin/alder.lua`, `lua/alder/repl.lua`, `lua/alder/health.lua`, `README.md`
- Test: `tests/scratch_spec.lua`, `tests/keys_spec.lua`, `tests/repl_spec.lua`

**Interfaces (Produces):**
```lua
M.TEMPLATE                     -- exactly internal/scaffold/files/scratch.alder's text
M.open(root)                   -- edit <root>/scratch.alder, writing TEMPLATE first if it is missing
M.append(root, lines, module)  -- append lines as a watch and :write; module = the sending buffer's module or nil
```

Rules:
- Config keys:
  - `repl` becomes `scratch = '<leader>ax'` (the `scratch` action calls `M.open`, project-level);
  - `send = '<leader>as'` now calls `M.append` (n and x, buffer-local, as today);
  - new `scratch_value = '<leader>ao'` is buffer-local and works only in a scratch buffer (`M.open_value`).
  - A user config that still sets `keys.repl` maps it to `scratch` and warns once.
- `M.append`:
  - strips the common indent, then writes `> <first line>` with each later line indented 4 spaces, after one blank line, at the end of the file;
  - saves the file, which triggers the run;
  - if `module` is non-nil and the scratch file's `in` header is not `module`, notifies once per session: `alder: scratch.alder is not "in <module>"; names from <module> need qualifying (add "in <module>" as its first line)` (Review Focus 5).
- `:AlderScratch` calls `M.open`. `:AlderRepl` still works, but `vim.notify`s the deprecation line from Global Constraints (WARN) first.
- Health: report ok when a running alder client has `server_capabilities.experimental.alderScratch`, and warn otherwise ("update alder, or :LspRestart"). The `repl --in` probe is now info-only.
- README: change the keys table (`<leader>ax`, `<leader>as`, `<leader>ao`), the commands table (`:AlderScratch`, and `:AlderRepl` marked deprecated) and the options block. Add a short "Scratch files" section.

- [ ] **Step 1: Write the failing tests:**
  - `scratch: template matches the scaffold`: compare with the file read from `$ALDER_REPO/internal/scaffold/files/scratch.alder`.
  - `scratch: open creates the file`.
  - `scratch: append writes an indented watch and saves`: a two-line selection gives `> a\n    b` at EOF, and the file is unmodified on disk afterwards.
  - `scratch: append warns about the in header` (Review Focus 5): one warning, even after two appends.
  - `keys: <leader>ax opens scratch.alder`, and `keys: legacy repl key maps to scratch`.
  - `repl: AlderRepl warns deprecated`.
- [ ] **Step 2:** Run `nvim --headless -u tests/minimal.lua -l tests/run.lua 'scratch'` and `... keys` and `... repl`. Expected: FAIL.
- [ ] **Step 3:** Implement and update the README.
- [ ] **Step 4:** Run the whole suite: `nvim --headless -u tests/minimal.lua -l tests/run.lua`. Expected: PASS.
- [ ] **Step 5:** Commit `scratch: keys move to scratch.alder, :AlderScratch, deprecate the REPL (alder#19)`.

---

## After merge

- alder first: merge `scratch-editor-19`, `go install ./cmd/alder`, then `:LspRestart`. Then `:TSUpdate alder`, or with `path` set, `:TSInstall alder`.
- alder.nvim: merge after alder's main has the LSP changes, so its CI builds against them.
- Comment on #19 with what shipped and close it. Tick the Plan 2 items in #30.

## Spec deviations (deliberate)

- Spec §5.4's per-file analysis (hover, completion and code actions in scratch files) is deferred to the follow-up issue from Task 5. Scratch files get highlighting, diagnostics and values, but not yet completion.
- Spec §4.5's `<leader>a!` and `alder/scratchTasks` are out because v1 is pure-only.
- An app module named `Scratch` gives Plan 1's "pass --module <Name>" diagnostic in the editor too. The LSP has no `--module`, so the fix is to rename the module. Revisit if anyone hits it.
