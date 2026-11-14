# Search pickers in alder.nvim and alder-vscode

Design for the rest of #166: rollout steps 3 and 4 of
`docs/superpowers/specs/2026-11-12-search-design.md` (the "search spec"), plus one small server
change they need. Decided with the user 2026-11-13/14. Parts marked **(my call)** were chosen
while writing this and were not discussed; the rest is the user's or comes from the search spec.

Read the search spec first, sections "Language server" and "Editors". This document adds to it
and, where the two differ, this one wins.

## Intent

The engine (`alder search`, #231) and the language server requests (`alder/search`,
`alder/searchEdit`, #232) are merged. What is missing is the thing the user actually touches: a
search box in the editor.

**The main job is finding a function**, by name or by type, the way Hoogle and Unison's `find`
do: open the box, type `split` or `Text -> Int`, read the signature and description, close it or
insert the name. The user's words: "I just need to be able to search for functions by name and
type. It's pretty straightforward."

**Filling a typed hole is a side feature** of the same picker, "useful … but relatively minor".
It must not add steps, keys or modes to the plain lookup.

Success: the user finds a standard library function without leaving the editor or reading
`stdlib/*.alder`.

## Goals

- One search command in each editor that opens a picker, sends what is typed to the server, and
  shows the server's results as they come.
- The highlighted result's full signature and description are readable without picking it.
- Enter inserts the result; a second action jumps to the definition of an app result.
- On a typed hole the picker opens already filled with `: <hole type>`, and Enter fills the hole.
- The same behaviour in both editors, within what each editor's UI allows.

## Non-goals

- A search screen in alder-tui (#226), examples on results (#228), dependencies (#229).
- Telescope, fzf-lua or snacks integrations. `workspace/symbol` already serves those.
- Query history, multi-select, or a source preview of app results.
- Changing hole completion or the "Fill with AI" action.
- A release of alder, or republishing the VS Code extension. Both are the user's decision.

## Anti-goals

- **The plugin filtering, re-ranking or re-ordering results.** The server's list is shown as
  given, in its order. In particular VS Code's quick pick must not hide or reorder results with
  its own matcher: a type query such as `: List Text` matches no label text at all.
- **Plugins building edits.** With a server attached, every insert and fill is the server's
  `alder/searchEdit` edit. The one text the plugin builds itself is the plain name in the
  no-server fallback (see "Result label").
- **Stale or flickering results.** A reply to an older keystroke never replaces a newer one's.
- **Hole logic in the plugins.** The plugin never detects holes, parses types or computes where
  the cursor goes. The server does.
- Breaking the routes picker while sharing its code.

## Decided

### What the user sees

One command, "Search". Default keys **(my call)**: `<leader>af` in nvim (`keys.search`, can be
`false`) and `ctrl+alt+a f` in VS Code ("find"; `s` is taken by send-to-scratch). In nvim there
is also `:AlderSearch [query]`; a query given there is the picker's starting text **(my call)**.

**Opening.** The picker opens empty and shows a one-line hint with examples: `split`, `Text.`,
`Text -> Int`, `: List Text` **(my call)**. When the cursor is on a typed hole it opens with
`: <hole type>` in the box and that search's results already showing; the text cursor is at the
end of the query, so the user can refine it or clear it (`<C-u>` in nvim).

**Typing.** Each change sends `alder/search`, debounced. The list is replaced by the reply. A
reply's `note` (an ambiguous type name, "to search by type, start with `:`") is shown.

**Reading.** The nvim layout the user approved:

```
┌ Search ───────────────────────┐
│ : List Text                   │
├───────────────────────────────┤
│  Text.lines   Text -> List T… │
│ ▸Text.split   Text -> Text -… │
│  List.reverse List a -> List… │
├───────────────────────────────┤
│ Text.split            stdlib  │
│ Text -> Text -> List Text     │
│ Splits a text wherever the    │
│ separator appears.            │
└─────────────────────── 2 of 9 ┘
```

The query line, the result list (label and signature, cut off when long), and a detail pane for
the highlighted result: label, source (`stdlib`, `app`) and effect (`query`, `task`) tags, the
whole signature, the whole doc comment. A blank doc is shown as nothing, not as a placeholder.

In VS Code it is a quick pick: the label, the signature as the item's description, the doc's
first line as its detail, and a "Go to definition" item button on app results. The quick pick's
placeholder carries the example hint.

**Enter** inserts the result at the cursor, or fills the hole when the cursor is on one. On a
hole Enter always fills it, whatever the query has become: clearing `: List Text` and picking
`Int.toText` by name still replaces the hole **(my call)**. Then:

- after a fill that leaves new holes (`Text.split ?a ?b`), the cursor is on the first new hole.
  Pressing the search key again there opens the picker for that hole, so fills can be chained;
- after a fill with no new holes, or a plain insert, the cursor is just after the inserted name.

"On the hole" means on its `?`: in normal mode in nvim, as a caret with nothing selected in
VS Code **(my call)**. In nvim's normal mode the cursor cannot rest past the end of a line, so
"just after" is as near as the mode allows **(my call)**.

**Where a plain insert lands (my call).** In nvim's normal mode the cursor is on a character,
not between two. Off a hole the name goes **after** that character, as `p` and `a` do, so
`x = ` followed by a search gives `x = Text.split`; on an empty line it goes at column 0. From
insert mode (a user's own mapping) it goes at the caret. In VS Code it goes at the caret; a
selection is not replaced.

**Esc** closes the picker and changes nothing: buffer, cursor and mode are as they were.

**Jump to definition** closes the picker and opens the result's declaration. It is for app
results only (they have a `uri` and `range`; stdlib results have neither). In nvim the key is
`<C-]>` **(my call)**; on a stdlib result it says the standard library has no source to open.

**Other nvim keys**, as in the routes picker: `<CR>` picks, `<Esc>` and `<C-c>` close, `<Down>`
/ `<C-n>` and `<Up>` / `<C-p>` move.

**The code action** "Search for `<type>`" on a hole opens the same picker at that hole. The
plugin uses the action's position as the open position and does not move the cursor until a
pick; Esc leaves it where it was **(my call)**.

### Result label

A result is shown as `Module.name`, and as the bare `name` when its module is `Prelude` (`Just`,
`not`). This is what `alder search` prints in its NAME column. In the no-server fallback this
label is also the text Enter inserts **(my call)**.

### Server change (alder, first PR)

`alder/searchEdit` returns only a workspace edit today, so each plugin would have to work out
where the cursor goes, and an added `import` line above shifts it. Instead the server says
**(my call)**:

- The response becomes an object with the workspace edit and one cursor position:
  `{ "edit": <WorkspaceEdit>, "cursor": <Position> }`. `cursor` is a position in the document
  **after** the edit is applied, in the negotiated position encoding: the `?` of the first new
  hole when the fill made new holes, else the end of the inserted text. The field names are the
  builder's choice.
- Nothing has been released with `alder/searchEdit`, so the shape changes with no compatibility
  layer. A plugin that gets a reply with no cursor treats the server as too old: it shows the
  "update alder" message and does not apply the edit.
- Plugins send a picked result's `args` back unchanged whenever the result has it, 0 included.
  The server
  already ignores `args` away from a hole (`internal/lsp/search.go`, `searchEditReq`); add a
  test that pins this, so no plugin has to decide when to send it.
- The book's "Search in the editor" section (`book/11-tooling.md`) and the CHANGELOG are updated
  for the new shape. If the book describes the editors' keys for other features, add search's
  the same way.

After this merges: `go install` alder.

### How the plugins talk to the server

The same in both:

1. **On open**, send `alder/search` with an empty query and the document's `uri` and cursor
   `position` (in nvim's normal mode, the character the cursor is on). The reply's `query` is
   the box's starting text (empty off a hole) and its `results` the starting list. The plugin
   does not look for a hole itself. A non-empty `query` in this reply is how it knows the picker
   was opened on a hole.
2. **On each change**, send `alder/search` with the query and the same `uri`. Debounce with the
   plugin's existing pattern (nvim: the generator modal's timer and sequence counter; VS Code:
   the generate flow's injectable debounce). Drop replies that are not for the latest request.
3. **On Enter**, send `alder/searchEdit` with the `uri`, the `position` the picker was opened
   at, and the result's `module`, `name`, `kind` and `args` (when present). In nvim's normal
   mode, when the picker was not opened on a hole, the position sent is one character to the
   right of the cursor (see "Where a plain insert lands"). Apply the returned edit and put the
   cursor at the returned position. An error from this request is shown and nothing is
   inserted.
4. **On jump**, open the result's `uri` at its `range`.

The picker remembers the document and position it was opened from. It is modal, so the document
does not change under it.

**Which buffers.** The command works from any buffer **(my call)**: looking something up should
not need an Alder file to be focused. The project and its server are found exactly as the
routes picker finds them (nvim: the root and `lsp.project_client`, as `routes.lua` does). When
the buffer is an Alder document attached to that server, the picker sends its `uri` and
`position` and Enter uses `alder/searchEdit`. In any other buffer the picker still searches
through that server but sends no `uri` or `position` (so there is no hole prefill), and Enter
inserts the result label as plain text at the cursor. With no project there is no server: see
the next paragraph.

**No server** (LSP turned off, not started, or no project): fall back to `alder search --json`,
as the routes pickers fall back to `alder routes --json`. Each change runs the CLI, debounced the
same way. There is no hole prefill, Enter inserts the result label as plain text, and jump uses
the row's `file`, `line` and `col`. Fall back only when there is no client or the request fails;
an **empty** result list is a normal answer and never triggers the fallback (the VS Code routes
picker does fall back on an empty list; do not copy that).

**Old alder.** A server that answers `alder/search` with "unknown method" (-32601), or a CLI
with no `search` command, gets one clear message: update alder. In nvim do not let
`lsp.request()` show the raw error.

**The code action.** Each plugin declares `capabilities.experimental.alderSearch = true` at
initialize and handles the client-side command `alder.search`, whose one argument is
`{ uri, position }` (the hole's start). In nvim that is the `capabilities` passed to
`vim.lsp.start` in `lua/alder/lsp.lua` and a handler in `vim.lsp.commands`. In VS Code it is a
registered command that must **not** be added to `SERVER_COMMANDS` (`src/clientConfig.ts`), and
the capability needs a way in, since nothing sets experimental client capabilities today; how is
the builder's choice.

### alder.nvim (second PR)

- **Plain floating windows, not nui** **(my call)**, like the routes picker, so search works
  without nui.nvim. The search spec said nui; the routes picker it pointed to does not use it.
- **One shared picker module** **(my call)**. `lua/alder/picker.lua` holds only filter helpers
  today, and `lua/alder/routes.lua` builds its own windows. Move the window, list, highlight and
  key code into a shared module that both pickers use, with an optional detail pane and a
  results source that can be a fixed list filtered locally (routes) or an async function called
  per query (search). Routes keeps its behaviour, and `tests/routes_spec.lua` passes unchanged
  apart from any internals it reaches into.
- `keys.search` in `lua/alder/config.lua`, the action and description in `lua/alder/keys.lua`,
  `:AlderSearch` in `plugin/alder.lua`. The key is available wherever the routes key is.
- Use the client's offset encoding when sending positions and when applying the edit and the
  cursor. Pass server results through `ui.plain()`.
- Tests in the existing harness (`tests/run.sh`): specs with a stubbed client as
  `tests/routes_spec.lua` does (open on and off a hole, typing replaces the list, a stale reply
  is dropped, Enter applies the edit and moves the cursor, Esc changes nothing, jump, the CLI
  fallback, the old-alder message), and at least one spec against the real `alder lsp` as
  `tests/lsp_spec.lua` does. CI builds alder from main, so it has the server change once that
  is merged.
- README: the key in the keys table and a short "Search" section. There is no `doc/` help file
  and this work does not add one.

After this merges: fast-forward `~/projects/alder.nvim`.

### alder-vscode (third PR)

- A `createQuickPick`, following `src/generate/ui.ts` (busy state, items replaced from async
  replies) behind an injectable UI interface as `src/routes.ts` has, so tests can drive it.
- Items are shown in the server's order and none is hidden by the quick pick's own matching.
  Verify this in a test with a type query; `alwaysShow` on every item is the expected way, and
  whatever else it takes is the builder's choice.
- The item button for "Go to definition" is the first use of item buttons in the extension.
- Apply the edit with the language client's converter and `workspace.applyEdit`, then set the
  editor's selection to the returned cursor.
- `package.json`: the `alder.search` command (title "Search", category "Alder") and the
  `ctrl+alt+a f` chord. `ctrl+enter` is bound to the generator's apply while a quick open is
  showing (`alder.formOpen && inQuickOpen`); search must not trigger it.
- `.alder-ref` holds the alder commit hash CI builds. Bump it to the server change's merge
  commit on alder's main, or CI's e2e cannot see `alder/search`. (alder.nvim's CI builds alder's
  main and has no pin.)
- Tests: unit tests for the pure parts (result to item, label, fallback rows), and e2e against
  the real server as `test/e2e/routes.e2e.ts` does, with the fake UI.
- README and CHANGELOG.

When this merges, #166 is done: close it and update roadmap #52.

## Constraints

- Three PRs, in this order, each merged to its repo's main: alder, alder.nvim, alder-vscode.
- No new dependencies in any of the three.
- Positions on the wire are in the encoding negotiated at initialize; ranges are 0-based. The
  CLI's `file`, `line` and `col` are 1-based.
- `alder/search` never fails on a bad query: it returns no results and a `note`.
- Outside contributors' PRs in these repos are not touched.

## Left to the builder

- The names of the response fields, the shared nvim module and its API.
- Window sizes, highlight groups, the debounce interval, how tags and `match` (`general`,
  `reordered`) are shown in the detail pane, and where the `note` appears.
- How VS Code declares the experimental capability.
- What "any buffer" does in an unmodifiable or special buffer.
- Whether the keymap needs a `when` clause wider than the routes one in VS Code.
