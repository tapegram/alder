# Search pickers Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A "Search" picker in alder.nvim and alder-vscode that finds functions by name or type through the language server, inserts the pick, and fills typed holes — closing #166.

**Architecture:** Three PRs in order. (1) alder: `alder/searchEdit` answers `{edit, cursor}` so no plugin computes cursor placement. (2) alder.nvim: the routes picker's float code moves into a shared picker module with an optional detail pane and an async source; `lua/alder/search.lua` drives it from `alder/search`, falling back to `alder search --json`. (3) alder-vscode: a `createQuickPick` search behind an injectable UI, same protocol, same fallback.

**Tech Stack:** Go (alder `internal/lsp`), Lua / Neovim ≥ 0.11 (alder.nvim), TypeScript / VS Code ≥ 1.91 / vscode-languageclient 11 (alder-vscode).

**Spec:** `docs/superpowers/specs/2026-11-14-search-pickers-design.md` (alder repo). It builds on `docs/superpowers/specs/2026-11-12-search-design.md` ("Language server", "Editors"); where they differ the pickers spec wins. Executors read the pickers spec in full before each task.

## Global Constraints

- Three PRs, in order, each squash-merged to its repo's `main` before the next starts: alder, alder.nvim, alder-vscode. Each starts from an up-to-date `main` on a short branch (`search-edit-cursor`, `search-picker`, `search-picker`).
- No new dependencies in any repo.
- Wire positions use the encoding negotiated at initialize; ranges are 0-based. The CLI's `file`, `line`, `col` are 1-based.
- Plugins never filter, re-rank or re-order server results, never build edits when a server is attached, never detect holes.
- A reply to an older keystroke never replaces a newer one's. Debounce 80 ms (both plugins' existing `DEBOUNCE_MS`).
- Result label: `Module.name`, or bare `name` when `module == "Prelude"`.
- Hint text (nvim hint line, VS Code placeholder): ``Search by name or type — e.g. split, Text., Text -> Int, : List Text``.
- An **empty** result list is a normal answer, never a reason to fall back to the CLI.
- Old alder (`-32601` from `alder/search`, a CLI without `search`, or a `searchEdit` reply without `cursor`) gives one message: `this alder has no search (update alder: go install ./cmd/alder in the alder repo, then restart the language server)` — nvim says `:LspRestart`, VS Code says `Alder: Restart Server`. In nvim never let the raw error through.
- Outside contributors' PRs are not touched. No release of alder, no extension publish.

## Review Focus

1. **An import line added above the fill** (an app module the file does not import): the cursor must land on the new hole / after the name on the shifted line, not one or two lines high. → Task 1 `TestSearchEdit_CursorAfterImport`.
2. **Non-ASCII text before the hole under UTF-16** (VS Code, nvim with utf-16): cursor column in wire units, applied correctly by the client. → Task 1 `TestSearchEdit_CursorUTF16`; Task 3 spec `search: cursor after a pick past non-ASCII text`.
3. **A pick the server rejects** (the entry left the index since the list was shown, or the document changed): the error is shown and the buffer is untouched. → Task 3 `search: a searchEdit error inserts nothing`; Task 6 unit test `searchEdit error leaves the document`.
4. **Esc from a picker opened by the code action or from insert mode**: buffer, cursor and mode exactly as before. → Task 3 `search: Esc restores cursor and mode`.
5. **A query that looks like a flag** (`-> Int`, `-x`) in the CLI fallback: passed as the query, not parsed as a flag. → Task 4 and Task 7 fallback tests with the queries `-> Int` and `-x`. `alder search` does not accept `--`; it already reads a lone `-x` or `-> Int` as the query, so pass `['search', '--json', q]`. A query the CLI cannot read (`-> Int`, `Text ->`) exits 2 with `alder search: <message>` on stderr and no JSON: show `<message>` as the note with an empty list, not as an error or the old-alder message.

---

## PR 1 — alder: `alder/searchEdit` returns a cursor

### Task 1: `{edit, cursor}` response

**Files:**
- Modify: `internal/lsp/search.go` (`searchEditReq`, new `searchEditResult` type)
- Test: `internal/lsp/search_test.go` (helper `searchEdit` at l.369 and its callers, new tests)
- Docs: `book/11-tooling.md` "Search in the editor" (l.3170–3245), `CHANGELOG.md` (the #166 line, l.11)

**Interfaces:**
- Produces (wire): `alder/searchEdit` result `{"edit": WorkspaceEdit, "cursor": Position}`. `cursor` is in the document **after** the edit, in the negotiated encoding: the `?` of the first new hole when `args > 0` on a hole, else the end of the inserted name. Go type `searchEditResult{Edit *workspaceEdit \`json:"edit"\`; Cursor Position \`json:"cursor"\`}`.
- Params unchanged; `args` sent off a hole is ignored.

- [ ] **Step 1: Write failing tests.** Change the test helper to `searchEdit(c, rel, p, module, name, kind, args) (*searchEditResult, *rpcError)` and make existing callers use `.Edit`. Add a helper `cursorText(got string, cur Position, enc posEncoding) string` returning the text of the edited document from `cur` to the end of that line. New tests:
  - `TestSearchEdit_CursorOnNewHole`: fill `?h` with `Text.split`, args 2 → line `fill = Text.split ?a ?b`, cursor text `?a ?b`.
  - `TestSearchEdit_CursorInParens`: `?k` with `Text.trim` args 1 → `lengthOf = Text.length (Text.trim ?a)`, cursor text `?a)`.
  - `TestSearchEdit_CursorAfterName`: `TestSearchEdit_InsertAtCursor`'s insert → cursor text ` []`; `Util.greeting` args 0 on `?k` → cursor text is empty (end of line).
  - `TestSearchEdit_CursorAfterImport`: `Zeta.zap` args 1 on `?h` (adds `import Zeta`) → cursor line is the `fill = Zeta.zap ?a` line of the edited text, cursor text `?a`; and the no-imports source of `TestSearchEdit_AddsImport` (adds two lines) → same check.
  - `TestSearchEdit_CursorUTF16`: `?u` with `Text.trim` args 1, utf-16 → cursor text `?a`; and `TestSearchEdit_NewHoleToken`'s `?fresh` case → cursor text empty.
  - `TestSearchEdit_ArgsIgnoredOffHole`: insert at the `TestSearchEdit_InsertAtCursor` position with args 0, 2 and 5 → identical edits, `NewText == "Text.split"`, no holes added.
- [ ] **Step 2: Run** `go test ./internal/lsp -run 'SearchEdit' ` — expect compile failure / FAIL.
- [ ] **Step 3: Implement.** In `searchEditReq` keep the byte-column range `at` and `text` before `wireEdit`. Cursor byte column = `at.Start.Character + i` where `i` is the index of the first `?` in `text` when new holes were added, else `len(text)`. Cursor line = `at.Start.Line` plus `strings.Count(importText, "\n")` when an import edit is added at a line `<= at.Start.Line`. Convert the column with `s.enc.toWire(newLine, col)` where `newLine = line[:at.Start.Character] + text + line[at.End.Character:]`. Return `&searchEditResult{Edit: ..., Cursor: ...}`. Update the doc comment.
- [ ] **Step 4: Run** `go test ./internal/lsp` — PASS; then `go test ./...` and `go vet ./...` — PASS.
- [ ] **Step 5: Docs.** Book: `alder/searchEdit` "answers an object: `edit`, a workspace edit…, and `cursor`, …" with the two cases above, a JSON example in the section's `->`/`<-` style, and one sentence that a client sends a result's `args` whenever it has one (ignored away from a hole). If the book lists editor keys for other features, add search's (`<leader>af`, `ctrl+alt+a f`) the same way; otherwise don't. CHANGELOG: amend the #166 line to say `alder/searchEdit` answers the edit and the cursor.
- [ ] **Step 6: Commit** `lsp: alder/searchEdit answers the edit and the cursor (#166)`; push the branch, open the PR, wait for CI green, squash-merge, `go install ./cmd/alder`, note the merge commit hash (Task 6 needs it).

---

## PR 2 — alder.nvim

All commands run in `~/projects/alder.nvim`. Suite: `tests/run.sh`; one area: `tests/run.sh <substring of test names>`.

### Task 2: Shared picker window

**Files:**
- Modify: `lua/alder/picker.lua` (add the window code; keep `filter`, `lines`)
- Modify: `lua/alder/routes.lua` (use it; keep its public functions and `M.state` semantics)
- Test: `tests/picker_spec.lua` (new), `tests/routes_spec.lua` (only where it reaches into moved internals)

**Interfaces:**
- Produces `require('alder.picker').open(opts) -> picker` with
  - `opts.title` (string), `opts.query` (starting text), `opts.hint` (string or nil, shown as a dim line when the query is empty),
  - `opts.source`: either `{ items = list, filter = fn(items, q) }` (local) or `{ fetch = fn(q, done) }` (async; `done(items, note)`; the picker owns the 80 ms debounce and the sequence counter, and drops a `done` from an older call),
  - `opts.row(item) -> label, signature` (two columns, cut with `…` to the window), `opts.detail(item) -> lines, marks` or nil (no detail pane),
  - `opts.on_pick(item)`, `opts.keys` (extra `{lhs = fn(item)}` maps, e.g. `<C-]>`), `opts.on_close()`.
- `picker` methods: `set_query(q)`, `move(delta)`, `confirm()`, `close()`, `selected()`, `set_items(items, note)`, `set_busy(bool)`. Footer `N of M` as routes has it; the note shows in the list area when there are no items, else on the detail pane's top border or a line under the query (builder's choice).
- Keys as routes today: `<CR>` confirm, `<Esc>`/`<C-c>` close, `<Down>`/`<C-n>`, `<Up>`/`<C-p>` move; `BufLeave` closes. `close()` restores the origin window, cursor and mode (stopinsert only if the origin was not insert mode).

- [ ] **Step 1: Write failing tests** in `tests/picker_spec.lua`: `picker: local source filters and confirms`, `picker: async source replaces the list`, `picker: a stale fetch reply is dropped` (two `set_query` calls, answer the first last; list shows the second's items), `picker: detail pane shows the selected item and follows move`, `picker: Esc restores window, cursor and mode`, `picker: extra keys get the selected item`. Assert through `H.float_text()` and the callbacks.
- [ ] **Step 2: Run** `tests/run.sh picker:` — FAIL.
- [ ] **Step 3: Implement** `open` by moving routes' `render`, `float`, `map`, `move`, `close`, footer code into `picker.lua`; the async path copies `modal.lua`'s `invalidate`/timer pattern (one `vim.uv.new_timer()` per picker, closed in `close`). Then rewrite `routes.show/set_query/move/confirm/close` as thin wrappers over a picker (routes uses the local source, no detail pane).
- [ ] **Step 4: Run** `tests/run.sh` — all PASS, `routes_spec.lua` unchanged except for references to moved internals.
- [ ] **Step 5: Commit** `picker: shared float picker; routes uses it`.

### Task 3: Search through the language server

**Files:**
- Create: `lua/alder/search.lua`
- Test: `tests/search_spec.lua` (stubbed client, as `routes_spec.lua` stubs `lsp.project_client`)

**Interfaces:**
- Consumes `picker.open` (Task 2), `lsp.project_client(root, buf)`, `project.root(buf)`, `ui.plain`.
- Produces `require('alder.search').open(opts)` with `opts.query` (starting text, `:AlderSearch` arg) and `opts.at = {buf, position}` (code-action entry: wire position of the hole). `M.label(result) -> string`. `M.state` (the open picker or nil) for tests.
- Behaviour, all from spec "How the plugins talk to the server":
  - Origin: buffer, window, cursor, mode. `doc` is set when the buffer is an `.alder` buffer attached to the project client: `{uri, position}` (position = the cursor's character in the client's `offset_encoding`, or `opts.at.position`).
  - Open: `alder/search {query = opts.query or "", uri, position}`; reply `query` is the box text; non-empty reply query with an empty `opts.query` ⇒ `on_hole = true`, remembered with the open position.
  - Each change: `alder/search {query, uri}` via the picker's async source.
  - Enter with `doc`: `alder/searchEdit {uri, position, module, name, kind, args?}` — position is the open position on a hole; otherwise in normal mode one character right of the cursor (the byte after the cursor's character, column 0 on an empty line), converted to the encoding; from insert mode the caret. Reply without `cursor` ⇒ the old-alder message, nothing applied. Else `vim.lsp.util.apply_workspace_edit(reply.edit, enc)` and set the cursor from `reply.cursor` (convert with `vim.str_byteindex(line, enc, character, false)`).
  - Enter without `doc`: insert `M.label(r)` as text (after the cursor character in normal mode). Unmodifiable buffer: `vim.notify('alder: this buffer is not modifiable', WARN)`, nothing inserted.
  - `<C-]>`: app result ⇒ close, `:edit` the uri's file, cursor at `range.start` (converted), `normal! zv`; stdlib ⇒ notify INFO `alder: the standard library has no source to open`, picker stays.
  - Detail pane: `label` + right-aligned tags (`source`, `effect` when set, `match` when not `general`), full signature, doc lines (none when blank).
  - Requests go through `client:request` directly (not `lsp.request`) so `-32601` becomes the old-alder message; other errors from `searchEdit` notify `alder search: <message>` at ERROR and insert nothing.

- [ ] **Step 1: Write failing tests** (stub client records every request): `search: opens empty with the hint`, `search: opens on a hole with its query and results`, `search: typing replaces the list` (debounce: wait with `H.wait`), `search: a stale reply is dropped`, `search: Enter on a hole applies the edit and moves the cursor to the new hole`, `search: Enter off a hole sends the position after the cursor character` (and column 0 on an empty line), `search: Enter sends args unchanged, 0 included`, `search: cursor after a pick past non-ASCII text` (client `offset_encoding = 'utf-16'`, line `x = "é😀" ++ ?h`), `search: a searchEdit error inserts nothing`, `search: a reply without cursor gives the update message`, `search: -32601 gives the update message` (no raw error notified: check with `H.capture_notify`), `search: Esc restores cursor and mode` (from normal and from insert), `search: C-] opens an app result / refuses stdlib`, `search: non-Alder buffer inserts the label`, `search: Prelude label is bare`.
- [ ] **Step 2: Run** `tests/run.sh search:` — FAIL.
- [ ] **Step 3: Implement** `lua/alder/search.lua` per the interface.
- [ ] **Step 4: Run** `tests/run.sh` — PASS.
- [ ] **Step 5: Commit** `search: picker on alder/search`.

### Task 4: CLI fallback

**Files:** Modify `lua/alder/search.lua`; Test `tests/search_spec.lua`.

**Interfaces:** No server (no project client, `client:request` returns false, or the request errors with anything but a normal reply) ⇒ each change runs `require('alder.cli').run({'search', '--json', q}, {cwd = root}, cb)` (see Review Focus 5), debounced through the same async source. Empty query ⇒ no run, empty list with the hint. Rows normalized to the LSP item shape with `file` (absolute, `root/file`), `lnum = line`, `col = col - 1`. Enter inserts the label; `<C-]>` opens `file` at `lnum`/`col`. Non-zero exit with no JSON: stderr `alder search: <message>` ⇒ `<message>` as the note, empty list (Review Focus 5); stderr naming an unknown command ⇒ old-alder message; anything else ⇒ notify `alder search: <stderr>`. No project root ⇒ the existing `project.require_root` message.

- [ ] **Step 1: Failing tests** with a fake `alder` script as in `routes_spec.lua`: `search: no client falls back to the CLI`, `search: an empty LSP list does not fall back`, `search: CLI rows jump with 1-based line and col`, `search: a CLI without search gives the update message`, `search: a flag-like query reaches the CLI as the query` (`-x` and `-> Int`; the script checks its argv), `search: a CLI query error is a note`.
- [ ] **Step 2: Run** `tests/run.sh search:` — FAIL. **Step 3: Implement.** **Step 4: Run** `tests/run.sh` — PASS.
- [ ] **Step 5: Commit** `search: fall back to alder search --json`.

### Task 5: Wiring, code action, real server, README

**Files:**
- Modify: `lua/alder/lsp.lua` (`capabilities` in `vim.lsp.start`; register `vim.lsp.commands['alder.search']`), `lua/alder/config.lua` (`keys.search = '<leader>af'`), `lua/alder/keys.lua` (`actions.search`, `PROJECT.search = true`, `DESC.search = 'alder: search functions by name or type (fills a hole under the cursor)'`), `plugin/alder.lua` (`:AlderSearch [query]`, `nargs='?'`), `README.md` (Options block, Commands table, Keys table and global-keys paragraph, a short "Search" section after "Filling holes")
- Test: `tests/keys_spec.lua`, `tests/lsp_spec.lua` (real `alder lsp`)

**Interfaces:**
- `capabilities = vim.tbl_deep_extend('force', vim.lsp.protocol.make_client_capabilities(), { experimental = { alderSearch = true } })`.
- `vim.lsp.commands['alder.search'] = function(cmd, ctx)` → `search.open({ at = { buf = ctx.bufnr, position = cmd.arguments[1].position } })`; the cursor is not moved until a pick.

- [ ] **Step 1: Failing tests:** keys_spec — `<leader>af` mapped globally in a project, `keys.search = false` maps nothing; lsp_spec (blog copy) — `lsp: search finds Text.split by type`, `lsp: search on a hole fills it and lands on the new hole` (write a `?h : List Text`-typed hole into a blog module, open the picker, confirm `Text.split`, assert the line and cursor), `lsp: the code action Search for opens the picker at the hole`, `lsp: initialize declares experimental.alderSearch`.
- [ ] **Step 2: Run** `tests/run.sh` — new tests FAIL. **Step 3: Implement.** **Step 4: Run** `tests/run.sh` — PASS (needs the Task 1 alder installed).
- [ ] **Step 5: Commit** `search: key, command, code action, README`; push, PR, CI green, squash-merge, fast-forward `~/projects/alder.nvim`.

---

## PR 3 — alder-vscode

All commands run in `~/projects/alder-vscode`. Unit: `npm run test:unit`; e2e: `ALDER_BIN=$(which alder) ALDER_REPO=~/projects/alder npm run test:e2e` (`ALDER_E2E_ONLY=search` for one file); `npm run lint`.

### Task 6: Pure search parts

**Files:** Create `src/searchItems.ts`; Test `test/unit/searchItems.test.ts`.

**Interfaces (produces):**
- `type SearchResult = { name; module; kind; signature; doc; source: 'stdlib'|'app'; effect?; match?; args?: number; uri?: string; range?: Range; file?: string; line?: number; col?: number }`, `type SearchReply = { query: string; mode: string; note: string; results: SearchResult[] }`.
- `searchLabel(r): string`; `toItem(r): SearchItem` where `SearchItem extends QuickPickItem { result: SearchResult }` with `label`, `description = signature`, `detail = first doc line or undefined`, `alwaysShow: true`, `buttons = [GO_TO_DEFINITION]` only when `uri` or `file` is set.
- `normalizeCli(stdout: string, root: string): SearchReply` (absolute `file`, 1-based kept); `searchCliError(res, lspError?): { note: string } | { error: string }` (`alder search: <message>` on exit 2 ⇒ note; unknown command ⇒ old-alder error; else `alder search: <stderr>` error); `editParams(uri, position, r)` (includes `args` whenever `r.args !== undefined`); `isOldServer(err): boolean` (code `-32601`); `checkEditReply(reply): { edit, cursor } | { error: string }` (missing `cursor` ⇒ old-alder error).

- [ ] **Step 1: Failing tests:** label (Prelude bare), item fields and `alwaysShow`, button only on app results, CLI normalize, `editParams` keeps `args: 0`, `checkEditReply` without cursor, `isOldServer`, `searchCliError` both branches.
- [ ] **Step 2: Run** `npm run test:unit` — FAIL. **Step 3: Implement.** **Step 4: Run** — PASS.
- [ ] **Step 5: Commit** `search: result items and helpers`.

### Task 7: The quick pick, command and server wiring

**Files:**
- Create: `src/search.ts`
- Modify: `src/extension.ts` (register `alder.search`; add `search` to the test `Api`), `src/client.ts` (register a static feature that sets `capabilities.experimental.alderSearch = true` in `fillClientCapabilities`), `package.json` (command `alder.search`, title "Search", category "Alder"; keybinding `ctrl+alt+a f` / mac `cmd+alt+a f`, `when: "alder.inProject"`), `.alder-ref` (Task 1's merge commit), `README.md` (Commands bullet `- **Search** (\`f\`): …`), `CHANGELOG.md` (`## Unreleased`)
- Test: `test/e2e/search.e2e.ts`

**Interfaces:**
- `interface SearchUi { open(init: { value: string; placeholder: string; items: SearchItem[] }): SearchSession }`, `interface SearchSession { onChange(fn: (q: string) => void); setItems(items: SearchItem[], note: string); setBusy(b: boolean); picked(): Promise<{ item: SearchItem; button?: 'definition' } | undefined> }`. Default implementation over `createQuickPick` with `matchOnDescription = false`, `matchOnDetail = false`, `ignoreFocusOut = true`, note shown as the `title` suffix or a disabled-looking first item (builder's choice; it must not be pickable).
- `searchCommand(deps, ui?, at?: { uri: string; position: Position })` registered as `alder.search` (args: a fake `SearchUi` from tests, or the code action's `{uri, position}` — distinguish by shape). Not in `SERVER_COMMANDS`.
- Same protocol as Task 3: open request with `uri`/`position` from the active alder editor's caret when it has no selection; `debounceMs` injectable (default 80), sequence counter, stale drop; Enter ⇒ `alder/searchEdit` (open position; caret otherwise) ⇒ `client.protocol2CodeConverter.asWorkspaceEdit` + `workspace.applyEdit` ⇒ `editor.selection` at `cursor`; non-alder editor or no server ⇒ insert label at the caret (selection not replaced); button ⇒ open `uri`/`file` at the range; CLI fallback via `run(bin, ['search','--json',q], {cwd: root, env: cliEnv()})` (exit 2 with `alder search: <message>` ⇒ note, per Review Focus 5) only on no running client or a request error.

- [ ] **Step 1: Failing e2e tests** (blog example, fake `SearchUi`): `type query keeps the server's order` (`: List Text`, items equal the `alder/search` reply in order, all `alwaysShow`), `pick on a hole fills it and puts the caret on the new hole`, `pick off a hole inserts at the caret`, `go to definition opens an app result`, `stale replies are dropped` (fake UI fires two changes, debounce 0, delays the first reply), `old server message` (swap `clients.binary`/stub as `routes.e2e.ts` does), `code action command opens on the hole`, `initialize declares experimental.alderSearch`, `ctrl+enter apply is not bound while search is open` (assert `alder.formOpen` is false during search).
- [ ] **Step 2: Run** e2e with `ALDER_E2E_ONLY=search` — FAIL. **Step 3: Implement.** **Step 4: Run** `npm run lint && npm run test:unit && ` the full e2e — PASS.
- [ ] **Step 5: Commit** `search: quick pick over alder/search (#166)`; push, PR, CI green on all three matrix legs, squash-merge. Then comment on and close alder#166, update roadmap alder#52.
