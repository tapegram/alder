# M6c — `alder test` and `--mutate` in quickfix, the REPL in a split

Date: 2026-10-19. Follows M6b plan 2 (`2026-10-18-m6b-plan2-nui-modal-design.md`, merged as
alder 1f3d8df0 and alder.nvim 4ce2339). One spec for both repos, as the M6b specs were.

## 1. Goal

Close the loop a person runs in Neovim while writing Alder: edit, run the tests, jump to
the failure, fix; run the mutants, jump to a survivor, write the test that kills it; and
open a REPL that speaks as the module being edited does, to try a function on a value.
Today each of these is a terminal round trip whose output is read by eye and whose
positions are found by name.

Success: `<leader>at` puts a failing test in quickfix at its `test` line; `<leader>am`
puts the survivors there at their expression; `<leader>ax` opens `alder repl --in Todo`
for `Todo.alder`, and a line copied from the file evaluates unchanged. The user hand-tests
in AstroNvim on the sandbox app; `scripts/test.sh` and the alder.nvim headless suite are
green.

Decisions taken while brainstorming (2026-10-19):

- **Eval is the REPL, not inline.** The user wants to "pop open the repl on demand with as
  much context as we can get away with". Inline eval-at-cursor (virtual text), typed-hole
  prompts for free locals, and save-as-test are not in M6c. They remain compatible with
  this design: an inline eval would be `alder eval --in M` on the same engine.
- **Context of a REPL session**, settled: the app's code (already), a module's point of
  view (new: `--in`), the dev database (already, `:run`), session `let`s (already). Runtime
  values of a request are a debugger, not a REPL, and not this milestone.
- **Tests run on demand only.** No run on save: a run is a Go build plus the suite, seconds
  not milliseconds. Failures go to quickfix, not to diagnostics.
- **Channels.** Tests and mutants go through the CLI's `--json` (as `:AlderCheck` does),
  not a new LSP request: they read disk either way, and agents get the same JSON. The REPL
  is a terminal split over the real `alder repl`, not a prompt buffer or an LSP session.
- Plan 3 polish (cursor-on-first-hole, `--no-button`) stays skipped.

## 2. Compiler and CLI (alder)

### 2.1 Test positions

`rtesting.Case` gains `File string` and `Line, Col int`: the position of the `test`
keyword, with `File` relative to the app directory as diagnostics and mutants give it
(`app/Todo.alder`). `testsTable` (`internal/gen/caps.go`) fills them from the checked
declaration's span. `CaseResult` carries the three fields, and `jsonResult` adds
`"file"`, `"line"`, `"col"` to every `--json` object. The human report is unchanged: the
book quotes it verbatim in several places, and the editor reads JSON.

### 2.2 `--in Name`

`alder eval [dir] [--in Name | --module Name] …` and `alder repl [dir] [--in Name |
--module Name]`. With `--in`, the session speaks as module `Name` does: the module's own
declarations and imports unqualified, every other module of the app qualified, the
module's brand constructors as its own code writes them, and every other module's brand
constructors too (the rule a `test` has). `Name` must be a module of the app; otherwise
`alder eval: no module Name in <app>` (exit 2, listing is not attempted).

Implementation: the session module is a copy of module `Name`'s source with the session's
items (the REPL's `let`s and the `it` expression) appended after its last declaration,
checked in the module's place through the existing `scratch` path (`Infer: true`, the
line-offset mapping for diagnostics, `evalIt` on `Name.it`). The appended region is an
insertion like the synthesized `module` line today: `InsertLine`/`LineOffset` map a
diagnostic in it back to the input, and a diagnostic in the module's own text is reported
with the module's file and line, as a diagnostic of any other module is.

A session `let` whose name the module declares is refused: `alder repl: Todo already
declares countOpen` — the `let` is not kept, like a `let` with an error. A `let` named
`it` stays refused as today. `--in` and `--module` together is a usage error (exit 2):
`--module` names the scratch module of app-scope mode and keeps its meaning.

REPL: the banner becomes `alder repl: todos, in Todo (:help …)`. `:in Name` switches the
session to module `Name`; `:in` alone returns to app scope (`Scratch`). Both re-check the
kept `let`s against the new context as `:reload` does, dropping (with the same notice) a
`let` that no longer checks and the `let`s that used it. `:help` lists `:in`. The
`alder help --json` flag spec gains `--in` for `eval` and `repl`.

### 2.3 Unchanged

`--mutate --json` already reports `file`, `line`, `col`, `kind`, `before`, `after`,
`killed` per mutant and a final score object; `-run`, `-timeout`, `--min` and the exit
codes are as documented.

## 3. alder.nvim

### 3.1 Tests

New `lua/alder/test.lua`, shaped like `check.lua` (`cli.run` → `setqflist`).

`:AlderTest [args]` and `keys.test` (`<leader>at`): from the buffer's app root
(`root.for_buf`), first write every modified `.alder` buffer under that root (the run reads
disk), then run `alder test --json <args>`. On completion:

- compile diagnostics (`{"diagnostics": […]}`): the existing `check` path — quickfix and
  `vim.diagnostic`, as `:AlderCheck` shows them;
- failures: one quickfix entry per failed result, `filename` (resolved against the app
  root), `lnum`/`col` from the result, `type = 'E'`, `text` on one line: `FAIL "name" —
  left: <left>  right: <right>`, or `error: <error>` / `note: <note>` when there is no
  `left`; a `trace` is not shown (the terminal has it);
- a notification with the summary (`12 passed, 1 failed`; `0 passed, 0 failed` keeps the
  CLI's hint); the quickfix list, titled `alder test`, opens when there are failures and
  closes when it is open, holds the previous run, and the run is clean;
- `{"error": msg}` or a non-zero exit with no results: an error notification with the
  CLI's stderr.

`keys.test_at` (`<leader>aT`): the test under the cursor — the nearest `test "name"`
declaration at or above the cursor (tree-sitter `test` node; regex fallback on
`^test "…" =`). `-run` matches the report key `Module "name"`, so the test runs as
`-run '^<Module> "<name>"$'` (name regex-escaped, module from the buffer's `module`
line). With no test above the cursor, the module's tests run: `-run '^<Module> '`.

### 3.2 Mutants

`:AlderMutate [args]` and `keys.mutate` (`<leader>am`): `alder test --mutate --json
<args>` from the app root, after the same buffer write, in the background with a start
notification (`alder test --mutate: running…`). A second call while one runs kills the
first (`vim.system` kill) and says so. On completion:

- survivors only go to quickfix: `filename`, `lnum`, `col`, `type = 'W'`, `text =
  'survived  <kind>: <before> → <after>'`, title `alder test --mutate — <killed> killed,
  <survived> survived (<score>)`; the list opens when there are survivors;
- no survivors: a notification with the score; compile diagnostics and errors as in 3.1.

Streaming survivors into the list as they arrive is deferred until a run is long enough
to hurt.

### 3.3 REPL

`:AlderRepl` and `keys.repl` (`<leader>ax`): a terminal split (`botright new`,
`jobstart(term = true)`, cwd the app root, as `:Alder <cmd>` runs a terminal command)
running `alder repl --in <Module>`, where `<Module>` is the current buffer's `module`
line (tree-sitter; regex fallback); a buffer outside any module, or outside an app, gets
no `--in`. One REPL per app root: a second call focuses the existing terminal window
(opening a window for it when closed) and starts a new one only when the job has ended.
`:AlderRepl` never switches the running REPL's module; `:in` inside the REPL does that.

`keys.send` (`<leader>as`): in normal mode the current line, in visual mode the selection,
sent to the app root's REPL with `chansend`, lines joined with the REPL's ` \`
continuation and a final newline; the cursor stays where it was. Without a REPL for this
root, the key starts one and sends after its banner. Leading indentation of a selection
is stripped by the common indent, so a multi-line expression pasted from a function body
arrives as the REPL's continuation rule wants.

### 3.4 Keys and health

`config.keys` gains `test = '<leader>at'`, `test_at = '<leader>aT'`, `mutate =
'<leader>am'`, `repl = '<leader>ax'`, `send = '<leader>as'`; each can be `false`;
buffer-local like the existing keys (`keys.lua`). `<leader>ar` stays `run`. `send` is
mapped in normal and visual mode.

`:checkhealth alder` reports whether the binary's `alder help --json` lists `--in` on
`repl` (an older binary gets `alder repl: this alder has no --in; update it` from the
plugin instead of a REPL error at startup); `test` falls back to no positions (entries with `lnum = 0`, module and
name in the text) when results lack `line`, with a health warning.

## 4. Testing

alder:

- `internal/gen`: `Tests` table entries carry `File`, `Line`, `Col` (gen_test).
- `rt/testing`: `jsonResult` has the three fields; `WriteResult` text unchanged.
- `cmd/alder`: `eval --in Todo 'countOpen []'` resolves unqualified; `TodoId "a"` is
  allowed; a diagnostic in the input maps to `input:L:C`; `--in` of an unknown module exits
  2; `--in` with `--module` exits 2. REPL: `let` of a declared name refused and not kept;
  `:in` switches and keeps a `let` that still checks, drops one that does not with the
  notice; `:in` alone returns to app scope.
- `internal/scratch`: line mapping for an appended region (the insertion is at the end,
  not the top).
- Book blocks: `book_test.go` verifies the examples added to chapter 11.

alder.nvim (custom headless harness, `tests/*_spec.lua`):

- `test_spec.lua`: pure — quickfix entries from result objects (failure with `left`/
  `right`, with `error`, without position), test-name detection at the cursor (node and
  regex), summary text; integration on `helpers.new_app` with the real binary — a failing
  test lands in quickfix at its line, a clean run closes the list, a compile error takes
  the check path.
- `mutate_spec.lua`: pure — survivors filtered, entry text, title with score; one
  integration run on a tiny app with a known survivor, kept short: a mutation run is the
  slowest thing the suite would do, and the suite's time is already a concern.
- `repl_spec.lua`: module detection; `:AlderRepl` opens a terminal whose command has
  `--in <Module>`; a second call reuses it; `send` joins lines with ` \` and strips the
  common indent.
- `keys_spec.lua`, `health_spec.lua`: the new keys and the `--in` check.

## 5. Docs

- `book/11-tooling.md`: `alder test` (`--json` fields), `alder eval` and `alder repl`
  (`--in`, `:in`, the banner), Editor support → "What you get" (test, mutate, REPL keys).
- `book/10-testing.md`: a sentence where the report is read, that `--json` carries the
  test's position for editors.
- `cmd/alder/helpjson.go`: `--in` on `eval` and `repl`.
- alder.nvim `README.md`: Commands and Keys sections.
- `13-compiler.md` is untouched: the LSP does not change.

## 6. Housekeeping (fold into the M6c branch if cheap)

- Create `tapegram/alder.nvim` on GitHub and push `main`; CI there checks out
  `tapegram/alder` already.
- `alder/.github/workflows/ci.yml` has no `base:` on its paths-filter steps. Check
  dorny/paths-filter's documented default for branch pushes before touching it; change it
  only if a branch push is compared against the previous commit rather than the merge base
  with `main`.

## 7. Out of scope

Inline eval and virtual-text results; typed-hole prompts for free locals; save-as-test;
test run on save; failures as diagnostics; a prompt-buffer REPL; an LSP eval request;
streaming mutant results; runtime values from a served request.
