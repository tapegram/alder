# M6b — `alder lsp` and the generator UI in alder.nvim

Date: 2026-10-18. Status: approved in conversation; awaiting file review.

M6 "editor support" is three subsystems. M6a (the tree-sitter grammar and
alder.nvim v1) is merged. This spec covers **M6b**: a language server,
`alder lsp`, that gives diagnostics while typing, hover, go-to-definition,
references and completion; the code generators exposed through it as code
actions and as a picker → prefilled form → live preview → apply flow in
alder.nvim, in the style of durandal.nvim. M6c (test runners, `eval`,
`repl` in the editor) keeps its own spec.

One spec, two implementation plans:

- **Plan 1** — the `Spec` refactor of the generators, the server
  (sections 1–4), alder.nvim attaching to it, `apply.lua`, and the keys
  `<leader>aa ac ae ar`. `<leader>aa` is rendered through
  `vim.ui.select`; a generator picked there asks for its missing fields
  with `vim.ui.input`, shows the plan's diff in the existing
  `alder://preview` scratch buffer, and `<CR>` applies it through
  `apply.lua`.
- **Plan 2** — the nui picker, form, live preview and review
  (`<leader>ag`); the preset entry from `<leader>aa` moves into it.

Plan 1 alone is a usable milestone.

## Goals

- In Neovim with the server attached: diagnostics appear as you type;
  `K` shows the type (and doc comment) of the name under the cursor; `gd`
  goes to its declaration, `grr` lists its uses; completion offers the
  names in scope, module members, record fields, constructors, HTML tags
  and attributes, and, at a typed hole, the values in scope that fit it.
- `<leader>aa` on any line offers the compiler's fixes for that line and
  the generators that apply there, prefilled from the cursor.
- `<leader>ag` opens a picker of every generator ("here" first, then
  recents), a form prefilled from the cursor, and a diff of the real plan
  that updates as you type; `<CR>` applies it as buffer edits you can undo.
- The server is the single editor backend: another editor gets the same
  features from the same requests.
- Go stays dependency-free (stdlib + `alder/*` + modernc); alder.nvim adds
  `nui.nvim` for the picker only.
- The plugin never rewords the compiler: every message a user sees in
  Neovim is the compiler's own text.

## Non-goals

Named so they do not creep in: rename; signature help; semantic tokens
(tree-sitter highlights); formatting (`alder fmt` does not exist yet);
loading alder-ui's Custom Elements Manifest (the slot is designed, the
loader is later); a VS Code client; `alder query` (the position walker
makes it easy later); durandal's hints, ghost text and magic circle;
workspace folders (one server per project); incremental document sync;
completion snippets; `alder fill` in the editor (see "Holes").

## Decisions made in the brainstorm

- **One spec, two plans** (above).
- **Completion covers Alder names and plain HTML**, the HTML from a
  built-in table shaped as a *manifest* so alder-ui's manifest plugs in
  later as a second source.
- **Apply = buffer edits.** A plan is applied to buffers, never to disk;
  undo is Neovim's, one `u` per buffer; the client refuses when a file
  changed since planning. The CLI path (`:Alder new …`) keeps writing to
  disk and is documented as such.
- **Five keys under `<leader>a`**: `ag` generate, `aa` actions, `ac`
  diagnostics to quickfix, `ae` explain, `ar` run. Which-key group
  "alder"; each key settable to `false`.
- **Holes.** `?name` is the agent's mechanism: a generator leaves holes,
  `alder test` refuses until they are filled, `alder fill` fills one
  through the CLI with a whole-app check. In the editor every part of
  that value is covered by the server (hover shows the hole's type, the
  W0002 hint sits on the line, diagnostics check what you type, `<leader>ac`
  lists what is left, completion at a hole lists what fits). So `:AlderFill`
  and the fill scratch buffer are removed; `alder fill` stays a CLI
  command for agents.
- **Generators run in-process** (approach B): the server imports
  `internal/generate`; previews run the real generator as a dry run on a
  temp copy of the app with the unsaved buffers written over it. Each
  generator declares its inputs as a typed `Spec`, which also renders
  `help --json` and the usage text.

## 1. The server: process, protocol, lifecycle

- `alder lsp` is a subcommand in `cmd/alder`, speaking LSP over stdio. The
  logic lives in a new package `internal/lsp`. Framing (`Content-Length`),
  the JSON-RPC 2.0 envelope and dispatch are written on `encoding/json`
  (about 300 lines). Protocol structs exist only for the methods served.
- **One server per project root.** alder.nvim starts it with
  `vim.lsp.start`, `root_dir` from `root.lua` (the nearest ancestor with
  `app/`), reused by root. A second project gets a second process.
- The server offers only `positionEncoding: "utf-8"` (Neovim 0.11
  negotiates it). The compiler's byte columns map to LSP columns by
  subtracting one; there is no UTF-16 conversion anywhere.
- Capabilities: `textDocumentSync` full; diagnostics pushed with
  `publishDiagnostics`; `hoverProvider`; `definitionProvider`;
  `referencesProvider`; `completionProvider` with trigger characters `.`,
  `<` and `?`; `codeActionProvider` with kinds `quickfix` and `refactor`;
  `executeCommandProvider` with `alder.explain`; and the custom requests
  `alder/generators`, `alder/preview`, `alder/plan` (section 4).
- Concurrency: one goroutine reads stdin and dispatches; handlers run on a
  worker under a single analysis mutex. `$/cancelRequest` is honoured for
  preview and completion by dropping stale requests; the client also
  sequences them, so a late answer is never shown.
- Logging: the server's own messages go to stderr (Neovim keeps them in
  `:LspLog`). `ALDER_LSP_LOG=<path>` mirrors the whole wire traffic.
- Shutdown: `shutdown` then `exit` per the protocol; a closed stdin exits
  the process. A panic in a handler is recovered into a JSON-RPC internal
  error with the stack in the log; one bad request never stops the server.
- `alder help --json` and the usage text gain `lsp`. `:checkhealth alder`
  gains "alder lsp running for `<root>`" (or why not).

## 2. Documents, overlay, analysis

- **Overlay.** `didOpen`/`didChange`/`didClose` keep `map[path]text` for
  open buffers. Everything the server analyses goes through one function,
  `snapshot()`: the files under `app/` on disk with the overlay written
  over them, handed to `names.Load` as in-memory `extra` modules (the hook
  that exists today). Analysis never touches a temp tree; only the
  generator path (section 4) materialises one, and `generate.Apply`
  already does.
- **Analysis** is one struct, `analysis{prog *names.Program, res
  *check.Result, diags []diag.Diagnostic, version}`, from
  `buildkit.LoadChecked` with the overlay. Rebuilds are debounced about
  150 ms after the last change on a single goroutine; a change during a
  rebuild schedules another. Requests read the latest completed analysis;
  nothing waits for a rebuild except the publication of diagnostics.
- **Last good result.** A syntax error anywhere means no names and no
  types (name resolution and checking are skipped after a parse failure).
  The server therefore keeps the *current* diagnostics (from the latest
  parse, so syntax errors show at once) and the **last good** `prog`/`res`
  (from the last build with no parse failure). Hover, definition,
  references and completion answer from the last good build; a position
  is valid when the file's text above it is unchanged since that build,
  otherwise the answer is empty. While a broken line is being typed,
  everything above it still hovers.
- **Position to node.** `syntax.NodeAt(module, pos)` returns the innermost
  node at a position and its ancestor chain, using the spans every node
  carries. It lives in `internal/syntax` so `alder query type` can use it
  later. It is the one compiler-side addition of M6b besides `Spec`.
- **Scope at a position** is computed from the ancestor chain (which
  `let`, lambda and arm binders enclose it) since resolve scopes are not
  retained; hover and completion share it.
- **Diagnostics.** Every rebuild publishes per open file, and once for
  the files it touches that are not open, so `<leader>ac` (quickfix from
  `vim.diagnostic.setqflist`) sees them all. W0002 holes publish as
  `Hint` severity with the hole's type in the message. A diagnostic with a
  `Fix` becomes a `quickfix` code action carrying the diagnostic, so
  Neovim's `gra` and `<leader>aa` both offer it. alder.nvim skips its
  check-on-save while a client is attached.
- **Disk changes under the editor** (a git checkout): the server polls
  `app/**` mtimes every 300 ms with the watcher `alder run` uses; a change
  outside the overlay triggers a rebuild.

## 3. Hover, definition, references, completion

**Hover** (`K`), by the innermost node at the cursor:

- a `Var` or `CtorRef` with a `Ref` → `name : Type` from `res.Values`
  (declarations) or `res.Binds` (let, parameter and pattern variables),
  followed by the `--` comment block directly above the declaration when
  there is one. `syntax.DocOf(decl)` scans the lines above the span;
  stdlib signature files' comments serve the same way, so `Db.query`
  shows its note.
- any other expression → its type from `res.Types`; a hole → `?load :
  Task Model`.
- a module name (`import Posts.Index`, or the qualifier of
  `Posts.Index.x`) → its file path and the number of exported
  declarations.
- an HTML tag or attribute → its one-line description from the HTML
  manifest.

Rendered as Markdown with a fenced `alder` block for the signature
(tree-sitter highlights the float).

**Definition** (`gd`): `Ref{Module, Name}` → the declaration's span in
`prog.Modules[Module]`; `import X` and qualified module names → the file;
a constructor → its arm in the union; a record field access → the field
in the record type's declaration when the type is named.

**References** (`grr`): `res` keeps no use index, so the server walks
every module comparing `Ref`s. Fine at Alder's app sizes; it also gives
`alder query uses` for free later.

**Completion** (`<C-X><C-O>`, or any completion plugin through
`nvim_lsp`), classified by position:

- after `Module.` → that module's exported declarations, detail
  `name : Type`; after `import ` → module names of the program;
- after `expr.` where `expr` has a record type → its fields;
- in a pattern → the constructors of the scrutinee's type;
- at or inside a hole `?x` → the names in scope whose type unifies with
  the hole's, ranked first, then everything else in scope;
- otherwise → names in scope (locals and parameters from `Binds`, the
  module's declarations, imports' qualifiers, keywords, stdlib modules).

**Markup.** Inside an HTML element (the parser gives element nodes):
after `<` → tag names; inside a start tag → the attributes of that tag
(global and per-tag), inserted as `name=""` with the cursor between the
quotes; after `</` → the open tag's name. Source: a package
`internal/markup` with a `Manifest` type (tags → description, attributes →
description) and one built-in, hand-written instance for HTML (about 110
tags, the global attributes, per-tag attributes for the common tags).
Completion and hover only see `[]Manifest`; alder-ui's Custom Elements
Manifest later becomes a second instance loaded from a file.

Items carry `CompletionItemKind`; `textEdit` replaces the identifier
under the cursor; no snippets.

## 4. Generators: `Spec`, "here", and the three requests

**`generate.Spec`** — one per mutating command: `new page|table|form|msg|
capability|admin|auth|type|decl|test|silo`, `edit table|page|decl|auth|
session`, `remove page|table`, `regen admin`. `fill` is not a `Spec`; it
stays CLI-only.

```go
type Spec struct {
    ID, Summary string              // "new page"
    Args  []Field                   // ordered, as on the command line
    Flags []Field
    Here  func(ctx HereCtx) *Match  // nil: never "here"
    Run   func(sets Sets, app App, dryRun bool) (Plan, error)
}
type Field struct {
    Name              string
    Kind              Kind // Module (new or existing), Page, Table, Decl,
                           // FieldList, Type, Route, Text, Path, Bool
    Required, Variadic bool
    Summary           string
}
```

`cmd/alder` parses argv through the `Spec` (one generic parser replaces
the per-command parsing), and `help --json` and the usage text are
rendered from it; the existing `helpjson_test` pin stays as the regression
check that neither changed. Choices for a `Kind` come from the analysis
(`Page` → `prog.Pages`, `Table` → `res.Tables`, `Type` → the named types
in scope, `Module` → existing modules). `Text`, `FieldList` and `Route`
are free text with validation (`FieldList` is `name:Type …` and each type
must resolve).

**Here.** `HereCtx` is the module at the cursor, the enclosing
declaration, its kind (page, table, capability, plain) and the hole if
any. Each `Spec` says what it can prefill: in a page module → `new msg`,
`new form`, `new test`, `edit page`, `remove page` with `Page` set; on a
`table` declaration → `edit table`, `remove table`; on any declaration →
`edit decl`; anywhere in a module → `new decl`, `new test` with `Module`
set. Everything else is listed, just not "here".

**Requests.** All take `textDocument` and `position` and answer from the
overlay.

- `alder/generators` → `{generators: [{id, summary, here, why}]}`.
- `alder/preview {id, sets}` → `{fields: [{name, kind, value, source,
  valid, why, choices?}], files: [{path, action, added, removed}], diff,
  holes, ready, error}`. `source` is one of `here`, `default`, `you`,
  `suggested`. The server owns the form's state: it merges what the user
  typed (`you`) over what `Here` prefilled, validates each field, and once
  every required field is valid runs the generator **as a dry run on a
  temp copy of the app with the overlay written over it**, returning the
  file list and the unified diff. The client debounces; the server drops
  stale requests.
- `alder/plan {id, sets}` → `{files: [{path, before, after}],
  diagnostics, holes, migration, imports}` — the same dry run returning
  whole file texts (`before` null for a new file), so the client can apply
  them as buffer edits and guard on `before`.

**Code actions** (`textDocument/codeAction` for a range): the fixes of
the diagnostics in the range (edit inline, kind `quickfix`), then every
"here" generator as kind `refactor` with `data: {id, sets, why}`. A
generator action is not resolved on the server: the client opens the
picker preset with `id` and `sets` (section 5), so fixes and generators
are one menu with two exits. `alder.explain` is an `executeCommand`
returning the explanation text for `<leader>ae`.

**A walk-through.** In `app/Posts/Index.alder`, cursor in `view`,
`<leader>ag`: `alder/generators` lists `new msg`, `new form`, `new test`,
`edit page`, `remove page` as *here* and the rest below. Pick `new msg`:
`alder/preview {sets: {}}` returns `Page = Posts.Index (here)`, `Name`
required and empty, `ready: false`. Type `Delete`: the next preview runs
`new msg Posts.Index Delete` dry on the overlay and returns the fields,
`app/Posts/Index.alder +12 −1`, the diff and the hole `?delete`. `<CR>`:
`alder/plan` returns `before`/`after`; the buffer is replaced (one `u`),
nothing hits disk, diagnostics re-run and `?delete` shows as a hint.

## 5. alder.nvim

**Attach.** `plugin/alder.lua` starts `alder lsp` on `FileType alder`
with `vim.lsp.start{cmd = {opts.cmd, "lsp"}, root_dir = root.for_buf(buf)}`,
reused by root; `opts.lsp = false` keeps CLI-only mode (check-on-save and
`:Alder …`). On attach, `K`, `gd`, `gra`, `grr` and `omnifunc` are
Neovim's defaults; `b:alder_lsp` makes `check.lua` skip the on-save run.
If the server stops, one notification says so (`alder lsp stopped —
:checkhealth alder`) and the keys notify instead of erroring.

**Dependency.** `nui.nvim`, for the picker only, lazy-required:
everything but `<leader>ag` and the generator exit of `<leader>aa` works
without it; `:checkhealth alder` reports it.

**Keys** (`opts.keys`, each settable to `false`; which-key group "alder"
when which-key is present): `<leader>ag` generate, `<leader>aa` actions,
`<leader>ac` diagnostics to the quickfix list (opens it), `<leader>ae`
explain the diagnostic under the cursor (the existing float), `<leader>ar`
`:Alder run` in a terminal split.

**Actions (`<leader>aa`).** `textDocument/codeAction` for the cursor
line, shown as one menu: `Fix: …` entries first, then the here
generators (`new msg (Page = Posts.Index)`). A fix applies its edit; a
generator opens the picker preset. On a miss, the menu says where in the
file something applies (the first line with a diagnostic fix or a here
generator), as durandal does. Plan 1 renders it with `vim.ui.select` and
gathers a generator's missing fields with `vim.ui.input`, previewing in
the `alder://preview` scratch; plan 2 replaces both with the nui menu and
the preset picker.

**The picker (`<leader>ag`)** — durandal's layout, lifted: one centred
modal, 80%×80% (stacked below 100 columns). Left: a search line over the
generator list, `── here ──` group first, then recents
(`stdpath("state")/alder/recent.json`, per root, ten), fuzzy by
`vim.fn.matchfuzzy`. Right: the form above the live preview. The form is
one buffer line per field, the line's text is the value, the label inline
virtual text, the note at end of line (`from the cursor`, `required`,
`you`, or the `why` in error colour); choice fields show the filtered
options as virtual lines, `<Up>`/`<Down>` pick. Every edit sends
`alder/preview` after 80 ms, sequence-guarded; the preview pane shows the
file list and the diff (`filetype=diff`). Keys: `<Tab>`/`<S-Tab>` between
fields, `<CR>` apply (jumps to the first invalid field when not `ready`),
`<C-o>` full review, `<Esc>` close.

**Review (`<C-o>`)** — a second modal: notes on top (holes the plan
leaves, migration, warnings), files 30% / diff 70%, `j`/`k` switch file,
`<CR>` apply, `q` back.

**Apply (`apply.lua`)** — `alder/plan`, then for each file: its current
text (the buffer if loaded, else disk) must equal `before`, else abort
naming the file ("changed since planning"); an existing file's buffer is
loaded and replaced with `nvim_buf_set_lines(0, -1)` (one undo step); a
new file is `bufadd`ed with its lines, unsaved; the generator's main file
(the page for `new page`) opens in the current window; a notification
lists what changed. The plugin never writes to disk.

**Commands.** `:AlderGenerate [id]`, `:AlderActions`, `:AlderDiagnostics`,
`:AlderExplain` replace `:AlderNew` and `:AlderFill`. `:Alder <words>`
stays as the escape hatch to the CLI (it writes to disk, as today).
`:AlderCheck` and `:AlderFix` stay for no-LSP mode.

## 6. Error handling

- Server: a handler panic → JSON-RPC internal error, stack in the log,
  server lives on. A generator refusal (`page exists`, `App has a
  session`) → `alder/preview` answers `error` with the CLI's exact message
  and `ready: false`; the form shows it in the preview pane. A dry run
  whose checked copy fails → its diagnostics in `plan.diagnostics`; the
  client refuses to apply and puts them in the quickfix list, as the CLI
  refuses to write.
- Client: a failed request → one `vim.notify` with the server's message;
  a stale preview answer is dropped; "changed since planning" aborts the
  apply naming the file; a missing nui → "install nui.nvim for
  `<leader>ag`".
- Every position in a request is validated against the current text
  before use; a request against a stale version returns an empty result,
  never a panic.

## 7. Testing

- `internal/lsp`: the server runs in-process over `io.Pipe` with a small
  test client (`request(method, params) → result`); each feature test is
  "open these files, send this request, compare": framing and encoding,
  overlay over disk, last-good analysis after a syntax error, hover,
  definition, references, completion (names, members, fields, patterns,
  holes, markup), code actions, the three generator requests against the
  example apps.
- `internal/generate`: `Here` and field validation per `Spec`,
  table-driven. The `Spec` refactor is covered by the existing CLI tests
  and the `helpjson` pin.
- alder.nvim: the existing headless runner against the **real**
  `alder lsp` (CI builds alder from main, as today): attach; each key;
  picker open → type → preview → apply → buffer text and `u`; `<leader>aa`
  on a table; degrade without nui; degrade when the server is killed. No
  fake server: the real one starts in milliseconds.
- Hand test: the sandbox app, as for M6a.

## Follow-ups (not M6b)

- alder-ui's Custom Elements Manifest as a second `markup.Manifest`,
  once that project produces one.
- `alder query type|uses` on `syntax.NodeAt` and the references walk.
- Doc comments in the stdlib signature files, now that hover shows them.
- Rename, signature help, a VS Code client.
