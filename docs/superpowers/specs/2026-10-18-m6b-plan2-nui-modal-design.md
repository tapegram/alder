# M6b plan 2 — the generator modal, hook hover, `new msg` comma, `--example`

Date: 2026-10-18. Follows plan 1 (`2026-10-18-m6b-lsp-generator-ui-design.md`, merged as
alder 732f6ede and alder.nvim 86a9af8). This spec supersedes §5 "The picker", "Actions" and
"Review" of that document where they differ.

## 1. Goal

Replace plan 1's `vim.ui.select` / `vim.ui.input` / `alder://preview` stopgap with one
modal generator UI in alder.nvim, built on nui.nvim and modelled on durandal.nvim's
generation modal (`~/projects/durandal.nvim/lua/durandal/generate.lua`, `preview.lua`,
`util.lua`): a centred float with a searchable generator list, a form and a live preview,
ending in the buffer-edit apply that plan 1 shipped. Three compiler-side items ride along:
hover text for the page hooks, `new msg` inserting the trailing comma a hand-written
`update` lacks, and `--example` scaffolding on `new page`.

Success: the user hand-tests the flow in AstroNvim on the sandbox app and it feels like
durandal's modal; agents keep a minimal, flag-driven CLI; `scripts/test.sh` and the
alder.nvim headless suite are green.

Decisions taken while brainstorming (2026-10-18):

- The picker is durandal's: type-to-filter list, cursor navigation. The earlier
  "which-key-like single keys" idea is dropped — the big modal is itself the discovery UI.
- `<leader>ag` opens the modal. `<leader>aa` and its actions menu are removed: fixes and
  here-generators already reach Neovim's own code-action menu (`gra`, AstroNvim `<leader>la`),
  the one place every LSP contributes to. Picking a generator there opens the modal preset.
- Recents in the picker (old spec) are dropped: the here-group already puts the likely pick
  on top.
- `--example` is marked by comments only; no checker warning, no hole.

## 2. alder.nvim surface

**Dependency.** `nui.nvim`, lazy-required (`require` inside the modal's `open`). Without it,
`<leader>ag`, `:AlderGenerate` and the `alder.generate` code-action command notify
`install nui.nvim for the generator UI`; `:Alder <words>` (the CLI, writes to disk) stays the
escape hatch. `:checkhealth alder` reports whether nui is found.

**Removed.** `flow.lua`, `actions.lua`, the plan-1 flow's use of the `alder://preview` scratch buffer (`:Alder <words>`, the CLI, still
shows its result there; amended during execution),
`:AlderActions`, the `actions` key. `apply.lua` stays as it is: it is the contract the modal
ends in (plan → `before`-guard → buffer edits, removal is the only disk write).

**Keys** (`opts.keys`, each settable to `false`; which-key group "alder" when present):
`generate = '<leader>ag'`, `diagnostics = '<leader>ac'`, `explain = '<leader>ae'`,
`run = '<leader>ar'`.

**Commands.** `:AlderGenerate [id]` opens the modal, preset when `id` is given (completion
unchanged). `:AlderCheck`, `:AlderFix`, `:AlderDiagnostics`, `:AlderExplain`, `:Alder`
unchanged.

**Entry points.**
- `<leader>ag` / `:AlderGenerate`: modal with the here-group first and its first item
  selected; focus in the search line.
- Code action (`vim.lsp.commands['alder.generate']`): `modal.open{generator = id, sets = …}`;
  the list shows that generator selected, focus on the first missing form field.

## 3. The modal

Modules under `lua/alder/`: `ui.lua` (shared look), `picker.lua` (list + search),
`form.lua`, `preview.lua`, `review.lua`, `modal.lua` (layout, state and wiring). Pure
functions (`picker.filter`, `form.lines`, `form.sets`, `preview.render`, `review.model`)
have no nui dependency and are unit-tested; the modal itself is smoke-tested.

### 3.1 Look (`ui.lua`, copied from durandal `util.lua:17-28`)

- `ui.modal = {relative = "editor", position = "50%", size = {width = "80%", height = "80%"}}`
  — pass a `vim.deepcopy`, nui mutates it.
- `ui.narrow()` → `vim.o.columns < 100`.
- `ui.pane(title, opts)` → `Popup{border = {style = "rounded", text = {top = " "..title.." ",
  top_align = "left"}}, win_options = {winhighlight = "Normal:Normal,FloatBorder:FloatBorder"}}`
  plus `enter`, `focusable`, `buf_options`.
- Layout, wide: row [col 35%: search (size 3) + list (grow 1)] | [col 65%: form (size 10) +
  preview (grow 1)]. Narrow: one column, search 3, list 30%, form 25%, preview grow.
- Titles: ` Generate ` on the search pane, ` <id>  <summary> ` on the form (set with
  `border:set_text` when the selection changes), the main file's path on the preview. Footers:
  list ` N of M ` bottom-right; preview ` updating… ` bottom-right while a request is in
  flight.

### 3.2 Search and list (`picker.lua`)

- On open, `alder/generators {textDocument, position}`; the answer's order is kept (here
  matches first, then the rest, Spec order within each).
- Render: one line per item, `id` then the summary in `Comment`; a `── here ──` virtual
  line above the first here item and `── all ──` above the first other item; `cursorline`.
- `picker.filter(items, query)` → `vim.fn.matchfuzzy` on `id .. " " .. summary`; an empty
  query returns all. Typing in the search line re-renders the list and selects the first row
  (durandal's behaviour, not the surviving selection; amended during execution).
- `<Down>/<C-n>`, `<Up>/<C-p>` move the selection; moving calls `select(id)`: the form is
  rebuilt from the item's `sets` and a preview is requested. Search maps are `{"i","n"}`,
  buffer-local, `nowait`.

### 3.3 Form (`form.lua`)

- One buffer line per field of `preview.fields`, in the server's order; the line text is the
  value. Label as inline `virt_text` (`Identifier`, `right_gravity = false`). End-of-line note:
  `from the cursor` (source `here`, `Comment`), `required` (empty and required, `Comment`), or
  the field's `why` in `DiagnosticError`.
- Choice fields (`choices` present): the line is a filter; matching options are `virt_lines`
  under it (`Pmenu`, the current one `PmenuSel` with `▸ `); `<Up>/<Down>/<C-p>/<C-n>` move
  through them, `<Tab>` or `<CR>` commits the current option into the line.
- Bool fields (kind `bool`): rendered `[x] <name>` / `[ ] <name>`; `<Space>` in normal mode
  toggles; the line is not editable. The initial state comes from the field's `value`
  (`"true"`/`"false"`, see §4.3); a value of `""` renders unchecked. An untouched bool whose
  source is `"default"` is not sent, so the server re-decides it on each request (see
  `Field.DefaultUnless`, §4.3); a toggled bool is sent `"true"`/`"false"` (amended during
  execution).
- The form buffer is modifiable as a whole, with a line-count guard (not only the focused
  line; amended during execution). A `TextChanged` whose
  changedtick equals the one recorded at render is ignored (durandal's tick trick); a
  line-count change re-renders from state; `o`, `O`, `dd`, `J`, `p`, `P` are no-ops and
  `<BS>` at column 0 is swallowed (expr map returning the replaced keycode).
- `form.lines(fields)` → lines + extmark specs (pure). `form.sets(fields, lines)` → the
  `sets` map to send (pure; bool fields contribute `"true"`/`"false"`, empty lines are
  omitted).

### 3.4 Preview (`preview.lua`)

- Every change to `sets` bumps `state.seq`, dims the pane (`winhighlight Normal:Comment`),
  sets the footer and, after 80 ms (`vim.defer_fn`), sends
  `alder/preview {textDocument, position, id, sets}`. An answer whose seq is not the current
  one is dropped. The server already cancels superseded previews.
- `preview.render(result)` (pure) → lines + highlights: for each file `+ path` (`DiffAdd`)
  or `~ path  +N −M` (`DiffChange`), then a `─`×40 rule (`Comment`), then `result.diff`;
  `result.error` as a `DiagnosticError` paragraph instead of the diff; holes as
  `? name : type` lines (`Comment`) after the diff. The pane's buffer has
  `filetype = "diff"`.
- On answer: fields re-rendered (the server fills `source`, `valid`, `why`, `choices`),
  pane undimmed, view scrolled to the first hunk (`winrestview{topline}`).

### 3.5 Keys and lifecycle (`modal.lua`)

- `<Tab>` / `<S-Tab>`: search → first field → … → last field → search.
- `<CR>`: when `ready`, close the modal and `apply.run(buf, id, sets, nil, pos)`; otherwise
  move focus to the first invalid field (no notification).
- `<C-o>`: the review modal (§3.6). `<Esc>` / `<C-c>`: close.
- `state` is a module singleton: `items, filtered, selected, query, fields, sets, ready,
  seq, timer, popups, bufs, layout, origin = {win, cursor}`; `modal.state` is readable by
  tests.
- Close is idempotent (`closed` flag, `pcall(layout.unmount)`), runs `stopinsert` first,
  cancels the timer, restores the origin window and cursor — once immediately and once via
  `vim.schedule`, because leaving insert moves the cursor a column. `BufLeave`/`WinLeave` on
  any pane schedule a check: if the current window is no longer one of ours, close.
- A failed request → one `vim.notify` with the server's message; the modal stays open.
  A dead server → close, then plan 1's `alder lsp stopped — :checkhealth alder`.

### 3.6 Review (`review.lua`, `<C-o>`)

- Sends `alder/plan`; on `plan.error` the review still opens, showing the error and
  diagnostics in the notes pane with the footer `✗ cannot apply · q back`.
- A second Layout, 90%×80%: notes pane on top (holes `? name : type`, `imports`,
  `migration`, diagnostics; height `min(#notes, 6) + 2`, not focusable), then files 30% /
  diff 70%. `review.model(plan)` (pure) builds the notes and, per file, the unified diff via
  `vim.diff(before or "", after or "", {result_type = "unified", ctxlen = 3})`; the diff pane
  is `filetype = "diff"`.
- `j`/`k` switch file (a `CursorMoved` autocmd re-renders the diff), `<Tab>` swaps panes,
  `<CR>` applies (same path as §3.5), `q`/`<Esc>` return to the form modal, which stays
  underneath: the review opens as an overlay (zindex 60) over the form modal rather than
  hiding it (amended during execution); its state and `sets` survive either way.
- Title text doubles as the hint: ` <id>  <CR> apply · Esc back `.

## 4. alder (Go)

### 4.1 Hover for page hooks (`internal/lsp/hover.go`)

- A table in `internal/check/pages.go`, next to `checkPage`'s required signatures, gives
  each hook its signature text and a role paragraph (`check.HookDoc(name) (sig, role string,
  ok bool)`), so the two cannot drift:
  - `load`: "Page hook. Runs on every request before `view`: reads the page parameters and
    the session and builds the `Model`. Required: `Params -> Session -> Query Model`."
  - `view`: "Page hook. Renders the `Model` to HTML; `click` and `submit` in it send `Msg`
    values to `update`. Required: `Model -> Session -> Html Msg`."
  - `update`: "Page hook. Handles one `Msg` and answers with a `Task Nav`: `Rerender` runs
    `load` and `view` again, `Redirect` goes to a page (the same one with new parameters is
    how a page keeps state), `Reject` answers with an error. State lives in the page
    parameters, the session or a table — `Model` is rebuilt by `load` every request.
    Required: `Msg -> Model -> Session -> Task Nav`."
- `hoverText`: when the hovered name is a `ValueDecl` (its name or a `RefValue` to it) called
  `load`, `view` or `update` in a module with `a.prog.Pages[mod] != nil`, the role paragraph
  follows the signature block; the declaration's doc comment, if any, follows that.
- Hovering the `page` keyword or anywhere in the `PageDecl` header (new case on
  `*syntax.PageDecl` in the path switch) shows a block
  `page "<path>"` and lines `params <record type>`, `requires …`, `roles …`, `silo …` as
  present, from `check.PageInfo`.
- `alder query` and the CLI are untouched. Tests: `TestHover_PageHook_{Load,View,Update}`,
  `TestHover_PageDecl`, and one check that `load` outside a page module gets no role text.

### 4.2 `new msg` trailing comma (`internal/generate/form.go addDecls`)

- Alder allows the last `match` arm without a trailing comma, so a hand-written page often
  lacks one; today the inserted arm then fails the dry-run check and the command refuses
  with the generic "the declaration the command edits does not check".
- Fix: after `updateArmSpot` finds the closing `}`, look back to the last non-blank,
  non-comment line of the match body; if it does not end in `,` (after trimming), add a
  second edit appending `,` to that line. Applies to every caller of `addDecls` (`new msg`,
  `new form`). Tests: `new msg` on a page whose last arm has / lacks the comma, both
  checked by the dry run; the arm-line detection with a trailing `-- comment` on the arm.

### 4.3 `--example` on `new page` (`internal/generate/page.go`, `specs.go`, templates)

- New flag `Field{Name: "example", Kind: ArgBool, Summary: "a working state → msg →
  update → view example to delete"}` on `new page`. Off on the CLI. Refused together with
  `--blank`, `--list`, `--show`, `--from` or `--param`:
  `--example goes with a plain page: a --list, --show or --from page already shows load
  reading a table`.
- Content (plain page, `page.tmpl` gains an `[[if .Example]]` branch; `Open`/`Close` as
  today). With module `About` at `/about`:

  ```alder
  -- example: a page keeps state in its parameters (or the session, or a table), never in
  -- Model between requests — `load` rebuilds Model on every request. Delete the parts
  -- marked `example` when you have seen how they fit together.
  page "/about" { count: Int = 0 }

  type Model = { count: Int }

  load : Params -> Session -> Query Model
  load params session = Query.succeed { count: params.count }

  -- example: a message carries a record; the view sends it from `click`.
  msg Bump { by: Int }

  update : Msg -> Model -> Session -> Task Nav
  update msg model session = match msg {
    -- example: `update` answers with a Nav — Redirect to the same page with the new count
    -- (Rerender would run `load` again with the old parameters), or Reject with an error.
    Bump p -> Task.succeed (Redirect (About { count: model.count + p.by })),
  }

  -- example: the view shows the state and sends a message from a button.
  view : Model -> Session -> Html Msg
  view model session =
    <Layout.Page title="About">
      <h1>About</h1>
      <p id="count">Count: {Text.fromInt model.count}</p>
      <al-button click={Bump { by: 1 }}>Bump</al-button>
    </Layout.Page>
  ```

  The exact text is fixed when the template is written against the checker (comment syntax
  inside markup, `Redirect` argument form, the page's own module name for the redirect); the
  generated page must check clean, and so must the page after every `example` part is
  deleted (which is the minimal page). The generated page carries a test that
  presses `Bump` (amended during execution).
- `help --json`: the flag appears like every other; `testdata/help.json` regenerated; book
  chapter 13 §10 gets one sentence on `--example`.
- Editor default on: `new page` is never a here-item (it creates a module), so the default
  rides on the `Field`: a new `Field.Default string` is the value `generate.Form` starts a
  field with when neither the cursor (`here`) nor the user (`you`) set it, with
  `FieldState.source = "default"`; only the editor calls `Form`, so the CLI default stays off
  and agents get the minimal page. A new `Field.DefaultUnless []string` names the flags that
  switch the default off (for `example`: `--blank`, `--list`, `--show`, `--from`, `--param`):
  the editor default must not start a combination the generator refuses, so `Form` drops the
  default when one of them is set (amended during execution). `FieldState.value` for a bool flag is `"true"`/`"false"`
  (today's `ArgBool` validation in `here.go` accepts both); `help --json` does not show
  `Default`.
- Comments stay outside markup (a `-- example:` line inside `<Layout.Page>` would be text);
  the view's comment sits above `view`'s signature.

### 4.4 Protocol

No new methods or fields. The review uses `plan.holes`, `plan.imports`, `plan.migration`,
`plan.diagnostics` as they are.

## 5. Error handling

- Server refusals arrive as `preview.error` / `plan.error`: shown in the preview pane and
  the review notes; `<CR>` is refused while not `ready`.
- `apply.lua` keeps its "changed since planning" abort and the removal confirm.
- Missing nui: one notification, nothing else breaks.
- A stale preview answer is dropped by seq; a request error notifies once.

## 6. Testing

- Go: `scripts/test.sh` green; tests of §4.1–4.3; golden `help.json` and usage pins
  regenerated.
- alder.nvim, the existing headless runner (`nvim --headless -u tests/minimal.lua -l
  tests/run.lua`) against the real `alder lsp`; `tests/minimal.lua` adds nui from
  `$NVIM_NUI` (default: lazy's install dir), as `NVIM_TS` does for tree-sitter:
  - unit: `picker.filter`, `form.lines`/`form.sets` (text, choice, bool), `preview.render`
    (new file, changed file, error, holes), `review.model`.
  - smoke: open `<leader>ag` on a page → floats present with titles ` Generate `, ` Form `,
    ` Preview ` → type `msg` → `<Down>` → `<Tab>` → type `Delete` → wait for `ready` →
    `<CR>` → no floats, buffer contains the arm, `u` restores; preset open via
    `vim.lsp.commands['alder.generate']`; `<C-o>` renders files and a `@@` hunk;
    `<Esc>` leaves no float and restores the cursor; missing nui notifies (stub
    `package.loaded["nui.popup"]`); narrow layout stacks (set `columns = 80`).
- Hand test: AstroNvim, sandbox app; the user's `~/.config/nvim/lua/plugins/alder.lua` gains
  `dependencies = { "MunifTanjim/nui.nvim" }`.

## 7. Out of scope

Direct per-generator keybindings; recents; a checker warning for example regions; cancel of
in-flight server work from the client; hover text for `Model`/`Msg`.
