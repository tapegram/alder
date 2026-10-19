# M6b plan 2b — alder.nvim: the nui generator modal Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace alder.nvim's `vim.ui.select`/`vim.ui.input`/`alder://preview` generator flow with one centred nui.nvim modal — search + generator list, form, live preview, `<C-o>` review — ending in the existing buffer-edit apply.

**Architecture:** Six Lua modules under `lua/alder/`: `ui.lua` (durandal's shared look), `picker.lua`, `form.lua`, `preview.lua`, `review.lua` (each a pure data half plus a small render half) and `modal.lua` (the layout, the state singleton and the wiring to `alder/generators`, `alder/preview`, `alder/plan` and `apply.run`). `flow.lua` and `actions.lua` go away; `<leader>aa` becomes `<leader>ag`; the code-action command presets the modal. Patterns and gotchas are lifted from `~/projects/durandal.nvim/lua/durandal/{util,generate,preview}.lua`.

**Tech Stack:** Neovim 0.11 Lua, nui.nvim (`nui.popup`, `nui.layout`), the repo's headless test runner (`nvim --headless -u tests/minimal.lua -l tests/run.lua [pattern]`) against the real `alder lsp`.

**Spec:** `docs/superpowers/specs/2026-10-18-m6b-plan2-nui-modal-design.md` §2, §3, §5, §6. Companion plan (Go side, run first): `2026-10-18-m6b-plan2-alder.md` — Task 8's bool-toggle test needs its `Field.Default` (`go install ./cmd/alder` from the `m6b-plan2` branch so `~/go/bin/alder` has it; the test harness uses the `alder` on PATH).

**Branch:** `m6b-plan2` in `~/projects/alder.nvim` (from `main`). Commit after every task. Run the suite with `cd ~/projects/alder.nvim && nvim --headless -u tests/minimal.lua -l tests/run.lua <pattern>`; `NVIM_TS`, `ALDER_REPO`, `alder` and `tree-sitter` on PATH as README §Tests says.

## Global Constraints

- nui.nvim is lazy-required: `require('nui.…')` only inside functions that open a float; `ui.has_nui()` gates every entry point with the notification `alder: install nui.nvim for the generator UI`.
- The plugin never writes to disk from the modal; `apply.run(buf, id, sets, on_done, pos)` is the only exit (removals confirm there, as today).
- Every server message is shown verbatim (`vim.notify` with the server's text); the modal never invents error text except the four strings named in this plan.
- Keymaps in the modal are buffer-local and `nowait`; the entry pane (search) maps `{ 'i', 'n' }`.
- Floats never outlive the modal: closing is idempotent, `BufLeave`/`WinLeave` to a window outside the modal closes it (unless the review is open), and the origin window + cursor are restored.
- Look: rounded borders, titles top-left with one space of padding, `winhighlight = 'Normal:Normal,FloatBorder:FloatBorder'`, 80%×80% modal, stacked below 100 columns (`ui.narrow()`).
- Tests use `test(name, fn)` globals, `helpers.lua` (`H.wait`, `H.copy_tree`, `H.capture_notify`, `H.alder_repo`) and the blog example; the suite must end `N tests, 0 failed` with N ≥ 77 − (actions_spec tests removed) + new ones.

## Review Focus

1. Pressing `<CR>` while a preview is still in flight (ready from the *previous* answer) must not apply stale `sets` — Task 7 sets `ready = false` whenever a request is sent, and tests that `enter()` during `updating` does nothing.
2. Typing in the search line until the list is empty, then `<Down>`/`<CR>` must not index `filtered[0]`/`nil` — Task 6 tests the empty-list state (form cleared, footer ` 0 of N `).
3. A `vim.NIL` field (`why`, `choices`, `source`) from JSON must render as absent, not as the string "vim.NIL" — `ui.plain` is applied to every answer (Task 1 test).
4. The server dying while the modal is open: the next request errors; the modal must stay usable or close cleanly, never leave a float — Task 7's error path (`updating(false)` + pane text) is tested with a stubbed `lsp.request` error.
5. Opening the modal twice (`<leader>ag` while it is open) must reuse/close the first, never stack two layouts — Task 6 tests `open` twice → one set of floats.

---

### Task 1: `ui.lua` — shared look, nui gate, `plain`

**Files:**
- Create: `lua/alder/ui.lua`
- Test: `tests/ui_spec.lua`
- Modify: `tests/minimal.lua` (nui on the runtimepath), `lua/alder/health.lua` (report nui), `README.md` (dependency)

**Interfaces:**
- Produces: `ui.modal` (layout opts), `ui.narrow()`, `ui.has_nui()`, `ui.MISSING` (string), `ui.pane(title, opts) → nui Popup`, `ui.plain(v)` (vim.NIL → nil at any depth).

- [ ] **Step 1: Put nui on the test runtimepath**

In `tests/minimal.lua`, after the `ts` line:

```lua
-- NVIM_NUI points at a nui.nvim checkout (default: the lazy.nvim one); the generator modal needs it
local nui = os.getenv('NVIM_NUI') or vim.fn.expand('~/.local/share/nvim/lazy/nui.nvim')
```

and add `nui` to the `vim.opt.rtp = { here, ts, nui, vim.env.VIMRUNTIME, … }` list. Check it exists on this machine: `ls ~/.local/share/nvim/lazy/nui.nvim/lua/nui/popup/init.lua` (durandal.nvim depends on it, so lazy has it; if not, `git clone https://github.com/MunifTanjim/nui.nvim ~/.local/share/nvim/lazy/nui.nvim`).

- [ ] **Step 2: Write the failing test**

```lua
-- tests/ui_spec.lua
local ui = require('alder.ui')

test('ui: plain strips vim.NIL at any depth', function()
  local got = ui.plain({ a = vim.NIL, b = { c = vim.NIL, d = 1 }, e = 'x' })
  assert(got.a == nil and got.b.c == nil and got.b.d == 1 and got.e == 'x', vim.inspect(got))
  assert(ui.plain(vim.NIL) == nil)
  assert(ui.plain(3) == 3)
end)

test('ui: has_nui is true here, false when nui cannot load', function()
  assert(ui.has_nui(), 'nui.nvim not on the runtimepath (NVIM_NUI)')
  local saved = package.loaded['nui.popup']
  package.loaded['nui.popup'] = nil
  package.preload['nui.popup'] = function() error('not installed') end
  local ok = ui.has_nui()
  package.preload['nui.popup'] = nil
  package.loaded['nui.popup'] = saved
  assert(not ok, 'has_nui ignored the failing require')
end)

test('ui: pane is a rounded popup titled top-left', function()
  local p = ui.pane('Generate', { enter = true })
  p:mount()
  local text = {}
  for _, w in ipairs(vim.api.nvim_list_wins()) do
    if vim.api.nvim_win_get_config(w).relative ~= '' then
      vim.list_extend(text, vim.api.nvim_buf_get_lines(vim.api.nvim_win_get_buf(w), 0, -1, false))
    end
  end
  p:unmount()
  assert(table.concat(text, '\n'):find(' Generate ', 1, true), vim.inspect(text))
end)

test('ui: narrow below 100 columns', function()
  local cols = vim.o.columns
  vim.o.columns = 80
  assert(ui.narrow())
  vim.o.columns = 140
  assert(not ui.narrow())
  vim.o.columns = cols
end)
```

Run: `nvim --headless -u tests/minimal.lua -l tests/run.lua 'ui:'`
Expected: FAIL — `module 'alder.ui' not found`.

- [ ] **Step 3: Implement `ui.lua`**

```lua
-- ui: the look every alder float shares, lifted from durandal.nvim's util.lua — the centred
-- modal's options, rounded panes titled top-left — plus the nui.nvim gate and `plain`.
local M = {}

-- The large centred modal (the generator's): its layout options, and whether the editor is too
-- narrow for panes side by side.
M.modal = { relative = 'editor', position = '50%', size = { width = '80%', height = '80%' } }
function M.narrow() return vim.o.columns < 100 end

M.MISSING = 'alder: install nui.nvim for the generator UI'

-- has_nui reports whether nui.nvim loads; every float-opening entry point checks it first.
function M.has_nui()
  local ok = pcall(require, 'nui.popup')
  return ok
end

-- pane is a pane of such a modal: a rounded border, title (if any) top-left, no wrap. opts are
-- nui Popup options merged over these.
function M.pane(title, opts)
  local Popup = require('nui.popup')
  return Popup(vim.tbl_deep_extend('force', {
    border = { style = 'rounded', text = { top = title and (' ' .. title .. ' ') or nil, top_align = 'left' } },
    win_options = { wrap = false, cursorline = false, winhighlight = 'Normal:Normal,FloatBorder:FloatBorder' },
  }, opts or {}))
end

-- plain turns a server answer into plain Lua: its JSON nulls (vim.NIL, which is truthy) become
-- nil, at any depth.
function M.plain(v)
  if v == vim.NIL then return nil end
  if type(v) ~= 'table' then return v end
  local out = {}
  for k, x in pairs(v) do out[k] = M.plain(x) end
  return setmetatable(out, getmetatable(v))
end

return M
```

- [ ] **Step 4: Health and README**

`lua/alder/health.lua`, after the `opts.lsp` block:

```lua
  if require('alder.ui').has_nui() then
    h.ok('nui.nvim (the generator modal, <leader>ag)')
  else
    h.warn('nui.nvim is not installed: <leader>ag and :AlderGenerate need it (MunifTanjim/nui.nvim)')
  end
```

README.md, in the install snippet (the lazy.nvim spec near the top), add `dependencies = { 'MunifTanjim/nui.nvim', 'nvim-treesitter/nvim-treesitter' }` (keep whatever dependency is already listed; add nui), and in §Tests mention `NVIM_NUI` next to `NVIM_TS`.

- [ ] **Step 5: Run and commit**

Run: `nvim --headless -u tests/minimal.lua -l tests/run.lua 'ui:'` → `4 tests, 0 failed`; `… tests/run.lua health` → still passing (the health spec counts reports; if it asserts an exact count, add one).

```bash
git add lua/alder/ui.lua tests/ui_spec.lua tests/minimal.lua lua/alder/health.lua README.md
git commit -m "nvim: ui.lua, the shared float look and the nui gate"
```

---

### Task 2: `picker.lua` — filter and list lines (pure)

**Files:**
- Create: `lua/alder/picker.lua`
- Test: `tests/picker_spec.lua`

**Interfaces:**
- Produces: `picker.filter(items, query) → items` (fuzzy on `id .. ' ' .. summary`, all in server order for `''`); `picker.lines(items, grouped) → lines, marks, headings` where `marks[i] = { row, col, hl = 'Comment' }` covers the summary and `headings = { { row, text = '── here ──' }, { row, text = '── all ──' } }` only when `grouped`.

- [ ] **Step 1: Failing test**

```lua
-- tests/picker_spec.lua
local picker = require('alder.picker')

local items = {
  { id = 'new msg', summary = 'a message and its update arm', here = true },
  { id = 'new form', summary = 'a form, its message and arm', here = true },
  { id = 'new page', summary = 'a page module' },
  { id = 'edit table', summary = 'change a table' },
}

test('picker: an empty query keeps every item in order', function()
  local got = picker.filter(items, '')
  assert(#got == 4 and got[1].id == 'new msg' and got[4].id == 'edit table', vim.inspect(got))
  assert(got ~= items, 'filter must return a copy')
end)

test('picker: fuzzy on id and summary', function()
  local got = picker.filter(items, 'tbl')
  assert(#got == 1 and got[1].id == 'edit table', vim.inspect(got))
  got = picker.filter(items, 'page')
  assert(got[1].id == 'new page', vim.inspect(got))
  assert(#picker.filter(items, 'zzzz') == 0)
end)

test('picker: lines align summaries and mark the groups', function()
  local lines, marks, headings = picker.lines(items, true)
  assert(lines[1] == 'new msg     a message and its update arm', vim.inspect(lines))
  assert(lines[4] == 'edit table  change a table', vim.inspect(lines))
  assert(marks[1].row == 1 and marks[1].col == #'new msg     ' and marks[1].hl == 'Comment', vim.inspect(marks))
  assert(#headings == 2 and headings[1].row == 1 and headings[1].text == '── here ──', vim.inspect(headings))
  assert(headings[2].row == 3 and headings[2].text == '── all ──', vim.inspect(headings))
  local _, _, none = picker.lines(items, false)
  assert(#none == 0)
  local empty = picker.lines({}, true)
  assert(#empty == 0)
end)
```

Run: `… tests/run.lua 'picker:'` → FAIL (module not found).

- [ ] **Step 2: Implement**

```lua
-- picker: the generator list of the modal — the fuzzy filter over the server's items and the
-- lines of the list pane. Pure: no buffers here (modal.lua renders).
local M = {}

-- filter is the items whose `id summary` fuzzy-match query, best first; a copy of all of them,
-- in the server's order (here first), for an empty query.
function M.filter(items, query)
  if query == nil or query == '' then return vim.list_slice(items) end
  local keyed = {}
  for _, it in ipairs(items) do keyed[#keyed + 1] = { text = it.id .. ' ' .. (it.summary or ''), item = it } end
  local out = {}
  for _, k in ipairs(vim.fn.matchfuzzy(keyed, query, { key = 'text' })) do out[#out + 1] = k.item end
  return out
end

-- lines renders items: `id`, two spaces past the longest id, the summary. marks are the
-- summaries' {row, col, hl}; headings (when grouped) go above the first here item and the
-- first other one as virtual lines.
function M.lines(items, grouped)
  local lines, marks, headings = {}, {}, {}
  local width = 0
  for _, it in ipairs(items) do width = math.max(width, #it.id) end
  local seen_here, seen_rest = false, false
  for i, it in ipairs(items) do
    local pad = string.rep(' ', width - #it.id + 2)
    lines[i] = it.id .. pad .. (it.summary or '')
    marks[i] = { row = i, col = #it.id + #pad, hl = 'Comment' }
    if grouped then
      if it.here and not seen_here then
        headings[#headings + 1] = { row = i, text = '── here ──' }
        seen_here = true
      elseif not it.here and not seen_rest then
        headings[#headings + 1] = { row = i, text = '── all ──' }
        seen_rest = true
      end
    end
  end
  return lines, marks, headings
end

return M
```

- [ ] **Step 3: Run and commit**

Run: `… tests/run.lua 'picker:'` → `3 tests, 0 failed`.

```bash
git add lua/alder/picker.lua tests/picker_spec.lua
git commit -m "nvim: picker.lua, the generator list's filter and lines"
```

---

### Task 3: `form.lua` — field lines, sets, options, toggles (pure)

**Files:**
- Create: `lua/alder/form.lua`
- Test: `tests/form_spec.lua`

**Interfaces:**
- Consumes: `fields` as `alder/preview` returns them: `{ name, kind, flag, required, value, source, valid, why?, choices? }` (`kind` is `'bool'` for a switch).
- Produces: `form.lines(fields, typed) → lines, marks` (`marks[i] = { row, label, note?, note_hl? }`, `label == ''` for a bool), `form.sets(preset, typed) → table`, `form.toggle(line) → line`, `form.is_on(line) → bool`, `form.options(field, typed) → strings`, `form.first_invalid(fields) → row|nil`, `form.NOTES`.

- [ ] **Step 1: Failing test**

```lua
-- tests/form_spec.lua
local form = require('alder.form')

local fields = {
  { name = 'page', kind = 'page', required = true, value = 'Posts.Index', source = 'here', valid = true, choices = { 'Posts.Index', 'Posts.Show' } },
  { name = 'name', kind = 'text', required = true, value = '', valid = false, why = 'required' },
  { name = 'fields', kind = 'fieldList', value = '', valid = true },
  { name = 'example', kind = 'bool', flag = true, value = 'true', source = 'default', valid = true },
}

test('form: lines show values, labels, notes and toggles', function()
  local lines, marks = form.lines(fields, {})
  assert(vim.deep_equal(lines, { 'Posts.Index', '', '', '[x] example' }), vim.inspect(lines))
  assert(marks[1].label == 'page     ' and marks[1].note == 'from the cursor' and marks[1].note_hl == 'Comment', vim.inspect(marks[1]))
  assert(marks[2].label == 'name     ' and marks[2].note == 'required' and marks[2].note_hl == 'Comment', vim.inspect(marks[2]))
  assert(marks[3].note == nil, vim.inspect(marks[3]))
  assert(marks[4].label == '' and marks[4].note == 'default', vim.inspect(marks[4]))
end)

test('form: typed text wins over the value; a why that is not "required" is an error', function()
  local f = vim.deepcopy(fields)
  f[2].value, f[2].why = 'bad name', 'not a message name: write Capitalised'
  local lines, marks = form.lines(f, { name = 'Del' })
  assert(lines[2] == 'Del', vim.inspect(lines))
  assert(marks[2].note == f[2].why and marks[2].note_hl == 'DiagnosticError', vim.inspect(marks[2]))
end)

test('form: sets merge the preset and the typed text, dropping empties', function()
  local got = form.sets({ page = 'Posts.Index' }, { name = 'Delete', fields = '' })
  assert(vim.deep_equal(got, { page = 'Posts.Index', name = 'Delete' }), vim.inspect(got))
  got = form.sets({ page = 'Posts.Index' }, { page = '' })
  assert(vim.deep_equal(got, {}), 'clearing a preset field drops it')
end)

test('form: toggle, options, first_invalid', function()
  assert(form.toggle('[x] example') == '[ ] example' and form.toggle('[ ] example') == '[x] example')
  assert(form.is_on('[x] example') and not form.is_on('[ ] example'))
  assert(vim.deep_equal(form.options(fields[1], ''), { 'Posts.Index', 'Posts.Show' }))
  assert(vim.deep_equal(form.options(fields[1], 'show'), { 'Posts.Show' }))
  assert(vim.deep_equal(form.options(fields[2], 'x'), {}), 'no choices → no options')
  assert(form.first_invalid(fields) == 2)
  assert(form.first_invalid({ fields[1] }) == nil)
end)
```

Run: `… tests/run.lua 'form:'` → FAIL.

- [ ] **Step 2: Implement**

```lua
-- form: the generator's fields as the form pane shows them — one line per field whose text is
-- the value, a label and a note as virtual text, `[x] name` for a switch — and what to send
-- back. Pure: modal.lua owns the buffer.
local M = {}

M.NOTES = { here = 'from the cursor', default = 'default' }

local function is_bool(f) return f.kind == 'bool' end

function M.is_on(line) return vim.startswith(line, '[x]') end

-- toggle flips a switch line.
function M.toggle(line)
  if M.is_on(line) then return '[ ]' .. line:sub(4) end
  return '[x]' .. line:sub(4)
end

-- current is the text a field shows: what was typed, else the server's value.
local function current(f, typed)
  if typed[f.name] ~= nil then return typed[f.name] end
  return f.value or ''
end

-- lines are the form buffer's lines and, per row, the label (inline, blank for a switch), and
-- the note at the end of the line: `required` while empty, the server's why in error colour,
-- or where the value came from.
function M.lines(fields, typed)
  typed = typed or {}
  local lines, marks = {}, {}
  local width = 0
  for _, f in ipairs(fields) do width = math.max(width, #f.name) end
  for i, f in ipairs(fields) do
    local v = current(f, typed)
    if is_bool(f) then
      lines[i] = (v == 'true' and '[x] ' or '[ ] ') .. f.name
    else
      lines[i] = v
    end
    local note, hl
    if not is_bool(f) and v == '' and f.required then
      note, hl = 'required', 'Comment'
    elseif f.why and f.why ~= '' then
      note, hl = f.why, 'DiagnosticError'
    elseif f.source and M.NOTES[f.source] then
      note, hl = M.NOTES[f.source], 'Comment'
    end
    marks[i] = { row = i, label = is_bool(f) and '' or (f.name .. string.rep(' ', width - #f.name + 2)), note = note, note_hl = hl }
  end
  return lines, marks
end

-- sets is what the server gets: the preset (a code action's, or the item's here values) with
-- the typed text over it; an empty text drops the key, so the server falls back to the cursor.
function M.sets(preset, typed)
  local out = {}
  for k, v in pairs(preset or {}) do
    if v ~= '' then out[k] = v end
  end
  for k, v in pairs(typed or {}) do
    if v ~= '' then out[k] = v else out[k] = nil end
  end
  return out
end

-- options are a choice field's choices matching the typed text (all for none).
function M.options(f, typed)
  local all = f.choices or {}
  if typed == nil or typed == '' then return vim.list_slice(all) end
  return vim.fn.matchfuzzy(all, typed)
end

-- first_invalid is the row of the first field the server marked invalid.
function M.first_invalid(fields)
  for i, f in ipairs(fields or {}) do
    if not f.valid then return i end
  end
end

return M
```

- [ ] **Step 3: Run and commit**

Run: `… tests/run.lua 'form:'` → `4 tests, 0 failed`.

```bash
git add lua/alder/form.lua tests/form_spec.lua
git commit -m "nvim: form.lua, the generator form's lines and sets"
```

---

### Task 4: `preview.lua` — the preview pane's lines (pure)

**Files:**
- Create: `lua/alder/preview.lua`
- Test: `tests/preview_spec.lua`

**Interfaces:**
- Consumes: an `alder/preview` result `{ files = { { path, action, added, removed } }, diff, error, holes = { { name, type, module, decl, arm, file, line, col } } }` (`action` is `'created'`, `'edited'` or `'removed'`, as `generate.lua`'s `after_write` already reads).
- Produces: `preview.render(result) → lines, hls` (`hls[i] = { row, hl }` as `line_hl_group`), `preview.first_hunk(lines) → row`.

- [ ] **Step 1: Failing test**

```lua
-- tests/preview_spec.lua
local preview = require('alder.preview')

test('preview: files, rule, diff and holes', function()
  local lines, hls = preview.render({
    files = { { path = 'app/Posts/Index.alder', action = 'edited', added = 3, removed = 1 }, { path = 'app/New.alder', action = 'created', added = 9, removed = 0 } },
    diff = '--- a/app/Posts/Index.alder\n+++ b/app/Posts/Index.alder\n@@ -1 +1 @@\n-x\n+y',
    holes = { { name = 'delete', type = 'Task Nav' } },
  })
  assert(lines[1] == '~ app/Posts/Index.alder  +3 −1' and hls[1].hl == 'DiffChange', vim.inspect({ lines, hls }))
  assert(lines[2] == '+ app/New.alder' and hls[2].hl == 'DiffAdd', vim.inspect(lines))
  assert(lines[3] == string.rep('─', 40) and hls[3].hl == 'Comment')
  assert(lines[4] == '--- a/app/Posts/Index.alder' and lines[8] == '+y', vim.inspect(lines))
  assert(lines[9] == '? delete : Task Nav' and hls[#hls].row == 9 and hls[#hls].hl == 'Comment', vim.inspect({ lines, hls }))
  assert(preview.first_hunk(lines) == 6)
end)

test('preview: an error replaces the diff; a removal is marked', function()
  local lines, hls = preview.render({ error = 'module `About` exists (`app/About.alder`): a generator never overwrites a file', files = {} })
  assert(lines[1]:find('module `About` exists', 1, true) and hls[1].hl == 'DiagnosticError', vim.inspect({ lines, hls }))
  lines, hls = preview.render({ files = { { path = 'app/Old.alder', action = 'removed', added = 0, removed = 12 } }, diff = '' })
  assert(lines[1] == '- app/Old.alder' and hls[1].hl == 'DiffDelete', vim.inspect(lines))
  assert(preview.first_hunk(lines) == 1)
end)

test('preview: nothing yet', function()
  local lines, hls = preview.render(nil)
  assert(#lines == 0 and #hls == 0)
end)
```

Run: `… tests/run.lua 'preview:'` → FAIL.

- [ ] **Step 2: Implement**

```lua
-- preview: the lines of the modal's preview pane from an alder/preview answer — the files it
-- would touch, a rule, the unified diff, the holes the result leaves — or the refusal. Pure.
local M = {}

local ACTION = { created = { '+ ', 'DiffAdd' }, removed = { '- ', 'DiffDelete' } }

function M.render(r)
  local lines, hls = {}, {}
  if not r then return lines, hls end
  local function add(text, hl)
    lines[#lines + 1] = text
    if hl then hls[#hls + 1] = { row = #lines, hl = hl } end
  end
  if r.error and r.error ~= '' then
    for _, l in ipairs(vim.split(r.error, '\n', { plain = true })) do add(l, 'DiagnosticError') end
    return lines, hls
  end
  for _, f in ipairs(r.files or {}) do
    local a = ACTION[f.action]
    if a then
      add(a[1] .. f.path, a[2])
    else
      add(('~ %s  +%d −%d'):format(f.path, f.added or 0, f.removed or 0), 'DiffChange')
    end
  end
  if #lines > 0 and ((r.diff and r.diff ~= '') or #(r.holes or {}) > 0) then add(string.rep('─', 40), 'Comment') end
  if r.diff and r.diff ~= '' then
    for _, l in ipairs(vim.split(r.diff, '\n', { plain = true })) do add(l) end
  end
  for _, h in ipairs(r.holes or {}) do add(('? %s : %s'):format(h.name, h.type), 'Comment') end
  return lines, hls
end

-- first_hunk is the row of the first `@@` line (the first change), else 1.
function M.first_hunk(lines)
  for i, l in ipairs(lines) do
    if vim.startswith(l, '@@') then return i end
  end
  return 1
end

return M
```

- [ ] **Step 3: Run and commit**

Run: `… tests/run.lua 'preview:'` → `3 tests, 0 failed`.

```bash
git add lua/alder/preview.lua tests/preview_spec.lua
git commit -m "nvim: preview.lua, the preview pane's lines"
```

---

### Task 5: `review.lua` — the `<C-o>` review modal

**Files:**
- Create: `lua/alder/review.lua`
- Test: `tests/review_spec.lua`

**Interfaces:**
- Consumes: an `alder/plan` result `{ files = { { path, action, before?, after? } }, diagnostics = { { file, line, col, code, message, severity } }, holes, imports, migration, error }`.
- Produces: `review.model(plan) → { notes = strings, files = { { path, new, removed, diff } }, can_apply = bool }`; `review.open(plan, { id, on_apply, on_back }) → true|false` (false and a notification when there is nothing to show). Its popups use `zindex = 60` so they float over the generator modal.

- [ ] **Step 1: Failing tests**

```lua
-- tests/review_spec.lua
local review = require('alder.review')

local plan = {
  files = {
    { path = 'app/Posts/Index.alder', action = 'edited', before = 'a\nb\nc\n', after = 'a\nB\nc\n' },
    { path = 'app/New.alder', action = 'created', before = vim.NIL, after = 'module New\n' },
  },
  holes = { { name = 'delete', type = 'Task Nav', module = 'Posts.Index', decl = 'update', arm = 'Delete' } },
  imports = { 'Post' },
  migration = {},
  diagnostics = {},
  error = '',
}

test('review: model builds notes and unified diffs', function()
  local m = review.model(require('alder.ui').plain(plan))
  assert(m.can_apply)
  assert(vim.deep_equal(m.notes, { '? delete : Task Nav  (Posts.Index.update.Delete)', 'import Post' }), vim.inspect(m.notes))
  assert(#m.files == 2 and m.files[1].path == 'app/Posts/Index.alder' and not m.files[1].new, vim.inspect(m.files))
  assert(m.files[1].diff:find('@@', 1, true) and m.files[1].diff:find('%-b\n%+B'), m.files[1].diff)
  assert(m.files[2].new and m.files[2].diff:find('+module New', 1, true), m.files[2].diff)
end)

test('review: an error forbids applying and heads the notes', function()
  local m = review.model({ error = 'the result does not check', diagnostics = { { file = 'app/X.alder', line = 3, col = 1, code = 'E0003', message = 'expected Int', severity = 'error' } }, files = {} })
  assert(not m.can_apply)
  assert(m.notes[1] == '✗ the result does not check' and m.notes[2] == 'app/X.alder:3:1 E0003 expected Int', vim.inspect(m.notes))
end)

local function float_text()
  local out = {}
  for _, w in ipairs(vim.api.nvim_list_wins()) do
    if vim.api.nvim_win_get_config(w).relative ~= '' then
      vim.list_extend(out, vim.api.nvim_buf_get_lines(vim.api.nvim_win_get_buf(w), 0, -1, false))
    end
  end
  return table.concat(out, '\n')
end

test('review: open shows notes, files and the diff; q goes back, <CR> applies', function()
  local applied, back = false, false
  assert(review.open(require('alder.ui').plain(plan), { id = 'new msg', on_apply = function() applied = true end, on_back = function() back = true end }))
  local text = float_text()
  assert(text:find(' new msg  <CR> apply · Esc back ', 1, true), text)
  assert(text:find(' files ', 1, true) and text:find('~ app/Posts/Index.alder', 1, true) and text:find('@@', 1, true), text)
  vim.api.nvim_feedkeys('q', 'x', false)
  assert(back and not applied)
  assert(float_text() == '', 'floats left after q')
  review.open(require('alder.ui').plain(plan), { id = 'new msg', on_apply = function() applied = true end, on_back = function() end })
  vim.api.nvim_feedkeys(vim.keycode('<CR>'), 'x', false)
  assert(applied and float_text() == '')
end)

test('review: cannot apply → <CR> does nothing, q closes', function()
  local applied = false
  review.open({ error = 'nope', files = {}, diagnostics = {} }, { id = 'new page', on_apply = function() applied = true end, on_back = function() end })
  assert(float_text():find('✗ cannot apply · q close', 1, true), float_text())
  vim.api.nvim_feedkeys(vim.keycode('<CR>'), 'x', false)
  assert(not applied and float_text() ~= '')
  vim.api.nvim_feedkeys('q', 'x', false)
  assert(float_text() == '')
end)
```

Run: `… tests/run.lua 'review:'` → FAIL.

- [ ] **Step 2: Implement**

```lua
-- review: the second modal (<C-o>): what the plan leaves to you on top (holes, imports,
-- migration, diagnostics), the files on the left, the unified diff of the one under the cursor
-- on the right. <CR> applies, q / <Esc> go back. Lifted from durandal.nvim's preview.lua.
local M = {}

local function present(v) return v ~= nil and v ~= '' end

-- model is the plan as the review shows it. Pure.
function M.model(plan)
  plan = plan or {}
  local notes, files = {}, {}
  local can_apply = not present(plan.error)
  if not can_apply then notes[#notes + 1] = '✗ ' .. plan.error end
  for _, d in ipairs(plan.diagnostics or {}) do
    notes[#notes + 1] = ('%s:%d:%d %s %s'):format(d.file, d.line, d.col, d.code, d.message)
  end
  for _, h in ipairs(plan.holes or {}) do
    local addr = (h.module or '') .. '.' .. (h.decl or '') .. (present(h.arm) and ('.' .. h.arm) or '')
    notes[#notes + 1] = ('? %s : %s  (%s)'):format(h.name, h.type, addr)
  end
  for _, i in ipairs(plan.imports or {}) do notes[#notes + 1] = 'import ' .. i end
  for _, m in ipairs(plan.migration or {}) do notes[#notes + 1] = 'migration: ' .. m end
  for _, f in ipairs(plan.files or {}) do
    local before, after = f.before or '', f.after or ''
    files[#files + 1] = {
      path = f.path,
      new = f.before == nil,
      removed = f.after == nil,
      diff = vim.diff(before, after, { result_type = 'unified', ctxlen = 3 }),
    }
  end
  return { notes = notes, files = files, can_apply = can_apply }
end

local function title(id, m)
  return ' ' .. (id or 'plan') .. (m.can_apply and '  <CR> apply · Esc back' or '  ✗ cannot apply · q close') .. ' '
end

-- open draws the review over whatever is open (zindex 60). opts: id, on_apply(), on_back().
function M.open(plan, opts)
  opts = opts or {}
  local m = M.model(plan)
  if #m.files == 0 and m.can_apply then
    vim.notify('alder: nothing to change', vim.log.levels.INFO)
    if opts.on_back then opts.on_back() end
    return false
  end
  local Layout, Popup = require('nui.layout'), require('nui.popup')
  local hl = { winhighlight = 'Normal:Normal,FloatBorder:FloatBorder', wrap = false }
  local notes = Popup({ border = { style = 'rounded', text = { top = title(opts.id, m), top_align = 'left' } }, focusable = false, zindex = 60, win_options = hl })
  local files = Popup({ border = { style = 'rounded', text = { top = ' files ', top_align = 'left' } }, enter = true, zindex = 60, win_options = vim.tbl_extend('force', hl, { cursorline = true }) })
  local diff = Popup({ border = { style = 'rounded', text = { top = ' diff ', top_align = 'left' } }, zindex = 60, buf_options = { filetype = 'diff' }, win_options = hl })
  local note_lines = #m.notes > 0 and m.notes or { 'no holes, no warnings' }
  local layout = Layout(
    { relative = 'editor', position = '50%', size = { width = '90%', height = '80%' } },
    Layout.Box({
      Layout.Box(notes, { size = math.min(#note_lines, 6) + 2 }),
      Layout.Box({ Layout.Box(files, { size = '30%' }), Layout.Box(diff, { size = '70%' }) }, { dir = 'row', grow = 1 }),
    }, { dir = 'col' })
  )
  layout:mount()
  local function fill(buf, lines)
    vim.bo[buf].modifiable = true
    vim.api.nvim_buf_set_lines(buf, 0, -1, false, #lines > 0 and lines or { '' })
    vim.bo[buf].modifiable = false
  end
  fill(notes.bufnr, note_lines)
  fill(files.bufnr, vim.tbl_map(function(f) return (f.new and '+ ' or f.removed and '- ' or '~ ') .. f.path end, m.files))
  local function show(i)
    local f = m.files[i]
    if f then fill(diff.bufnr, vim.split(f.diff, '\n', { plain = true })) end
  end
  show(1)
  vim.api.nvim_create_autocmd('CursorMoved', { buffer = files.bufnr, callback = function() show(vim.api.nvim_win_get_cursor(files.winid)[1]) end })
  local closed = false
  local function close()
    if closed then return end
    closed = true
    pcall(function() layout:unmount() end)
  end
  -- leaving both panes for another window closes the review (it is never left floating)
  for _, p in ipairs({ files, diff }) do
    vim.api.nvim_create_autocmd('WinLeave', {
      buffer = p.bufnr,
      callback = function()
        vim.schedule(function()
          if closed then return end
          local w = vim.api.nvim_get_current_win()
          if w ~= files.winid and w ~= diff.winid then
            close()
            if opts.on_back then opts.on_back() end
          end
        end)
      end,
    })
  end
  local function back()
    close()
    if opts.on_back then opts.on_back() end
  end
  local function accept()
    if not m.can_apply then return end
    close()
    if opts.on_apply then opts.on_apply() end
  end
  for _, p in ipairs({ files, diff }) do
    p:map('n', 'q', back, { noremap = true, nowait = true })
    p:map('n', '<Esc>', back, { noremap = true, nowait = true })
    p:map('n', '<CR>', accept, { noremap = true, nowait = true })
  end
  files:map('n', '<Tab>', function() vim.api.nvim_set_current_win(diff.winid) end, { noremap = true, nowait = true })
  diff:map('n', '<Tab>', function() vim.api.nvim_set_current_win(files.winid) end, { noremap = true, nowait = true })
  return true
end

return M
```

If `vim.diff` with `result_type = 'unified'` returns a string without the `---`/`+++` header (it does — only hunks), the test's `@@` and `-b\n+B` assertions still hold.

- [ ] **Step 3: Run and commit**

Run: `… tests/run.lua 'review:'` → `4 tests, 0 failed`.

```bash
git add lua/alder/review.lua tests/review_spec.lua
git commit -m "nvim: review.lua, the plan review modal"
```

---

### Task 6: `modal.lua` — layout, list, search, open/close

**Files:**
- Create: `lua/alder/modal.lua`
- Modify: `lua/alder/lsp.lua:41-54` (`request` gains `on_err`)
- Test: `tests/modal_spec.lua`

**Interfaces:**
- Consumes: `ui`, `picker`, `form`, `preview`, `review`; `lsp.request(buf, method, params, cb, on_err)`; `alder/generators` `{ generators = { { id, summary, here, why?, sets? } } }`.
- Produces: `modal.open(buf, { generator?, sets? })`, `modal.close()`, `modal.state` (readable: `items, filtered, selected, query, sets, typed, fields, ready, focus, seq, bufs, win, popups, origin, buf`), `modal.selected()`, `modal.type_query(q)`, `modal.move(delta)`, `modal.select(i, preset)`; Task 7 adds the form and preview functions to the same module.

- [ ] **Step 1: `lsp.request` with an error callback**

In `lua/alder/lsp.lua` change `M.request`:

```lua
-- request sends method to buf's server. cb gets the result; an error is shown once with the
-- server's own message (a cancelled request, -32800, shows nothing) and on_err(err), when
-- given, runs after it so a caller can clear its "updating" state.
function M.request(buf, method, params, cb, on_err)
  local c = M.client(buf)
  if not c then
    vim.notify('alder: the language server is not attached (:checkhealth alder)', vim.log.levels.WARN)
    if on_err then on_err({ message = 'not attached' }) end
    return
  end
  c:request(method, params, function(err, result)
    if err then
      if err.code ~= -32800 then vim.notify(err.message, vim.log.levels.ERROR) end
      if on_err then on_err(err) end
      return
    end
    cb(result)
  end, buf)
end
```

Run `… tests/run.lua 'lsp:'` → still passing.

- [ ] **Step 2: Failing tests**

```lua
-- tests/modal_spec.lua
local H = require('helpers')
local modal = require('alder.modal')

local function blog_buf(rel)
  local dst = H.tmpdir() .. '/blog'
  H.copy_tree(H.alder_repo() .. '/examples/blog', dst)
  vim.cmd.edit(dst .. '/' .. rel)
  local buf = vim.api.nvim_get_current_buf()
  assert(H.wait(5000, function() return vim.b[buf].alder_lsp end), 'not attached')
  H.wait(1500, function() return false end) -- the first analysis
  return buf, dst
end

local function float_text()
  local out = {}
  for _, w in ipairs(vim.api.nvim_list_wins()) do
    if vim.api.nvim_win_get_config(w).relative ~= '' then
      vim.list_extend(out, vim.api.nvim_buf_get_lines(vim.api.nvim_win_get_buf(w), 0, -1, false))
    end
  end
  return table.concat(out, '\n')
end

local function open(buf, opts)
  modal.open(buf, opts)
  assert(H.wait(5000, function() return modal.state ~= nil and modal.state.bufs ~= nil end), 'modal did not open')
  return modal.state
end

test('modal: opens with the panes titled, here group first, search focused in insert mode', function()
  local buf = blog_buf('app/Posts/Index.alder')
  vim.fn.search('load params')
  local s = open(buf)
  local text = float_text()
  for _, t in ipairs({ ' Generate ', ' Preview ', '── here ──', '── all ──', 'new msg' }) do
    assert(text:find(t, 1, true), 'missing ' .. t .. ':\n' .. text)
  end
  assert(#s.items >= 19 and s.items[1].here and s.filtered[1].id == s.items[1].id, vim.inspect(s.items[1]))
  assert(vim.api.nvim_get_current_win() == s.win.search)
  assert(vim.fn.mode():sub(1, 1) == 'i', vim.fn.mode())
  assert(text:find((' %d of %d '):format(#s.items, #s.items), 1, true), text)
  modal.close()
end)

test('modal: Esc closes every float and restores the cursor; close is idempotent', function()
  local buf = blog_buf('app/Posts/Index.alder')
  vim.api.nvim_win_set_cursor(0, { 9, 3 })
  local win = vim.api.nvim_get_current_win()
  open(buf)
  vim.api.nvim_feedkeys(vim.keycode('<Esc>'), 'x', false)
  assert(H.wait(2000, function() return modal.state == nil end), 'state kept')
  assert(float_text() == '', 'floats left:\n' .. float_text())
  assert(vim.api.nvim_get_current_win() == win)
  assert(vim.deep_equal(vim.api.nvim_win_get_cursor(win), { 9, 3 }), vim.inspect(vim.api.nvim_win_get_cursor(win)))
  modal.close()
  modal.close()
end)

test('modal: typing filters, arrows move, an empty list is harmless', function()
  local buf = blog_buf('app/Posts/Index.alder')
  local s = open(buf)
  modal.type_query('msg')
  assert(s.filtered[1].id == 'new msg', vim.inspect(s.filtered))
  assert(modal.selected().id == 'new msg')
  modal.type_query('')
  modal.move(1)
  assert(s.selected == 2 and modal.selected().id == s.items[2].id)
  modal.move(-5)
  assert(s.selected == 1)
  modal.type_query('zzzzzz')
  assert(#s.filtered == 0 and modal.selected() == nil)
  modal.move(1)
  modal.enter()
  assert(modal.state ~= nil, 'enter on an empty list closed the modal')
  assert(float_text():find(' 0 of ', 1, true), float_text())
  -- typing in the search buffer itself drives the same path
  vim.api.nvim_buf_set_lines(s.bufs.search, 0, -1, false, { 'page' })
  assert(H.wait(1000, function() return s.query == 'page' end), s.query)
  assert(modal.selected().id:find('page', 1, true))
  modal.close()
end)

test('modal: opening twice leaves one modal; leaving its windows closes it', function()
  local buf = blog_buf('app/Posts/Index.alder')
  open(buf)
  local first = modal.state
  open(buf)
  assert(modal.state ~= first)
  local n = 0
  for _, w in ipairs(vim.api.nvim_list_wins()) do
    if vim.api.nvim_win_get_config(w).relative ~= '' then n = n + 1 end
  end
  assert(n <= 8, 'two modals: ' .. n .. ' floats') -- 4 panes, each with a border window
  vim.cmd.stopinsert()
  vim.cmd.wincmd('p') -- back to the editor window
  assert(H.wait(2000, function() return modal.state == nil end), 'leaving did not close')
  assert(float_text() == '')
end)

test('modal: narrow editors stack the panes', function()
  local cols = vim.o.columns
  vim.o.columns = 80
  local buf = blog_buf('app/Posts/Index.alder')
  local s = open(buf)
  local list, formw = vim.api.nvim_win_get_position(s.win.list), vim.api.nvim_win_get_position(s.win.form)
  assert(list[2] == formw[2] and list[1] < formw[1], vim.inspect({ list, formw }))
  modal.close()
  vim.o.columns = cols
end)

test('modal: without nui, one notification and nothing opens', function()
  local buf = blog_buf('app/Posts/Index.alder')
  local saved = package.loaded['nui.popup']
  package.loaded['nui.popup'] = nil
  package.preload['nui.popup'] = function() error('not installed') end
  local messages, restore = H.capture_notify()
  modal.open(buf)
  restore()
  package.preload['nui.popup'] = nil
  package.loaded['nui.popup'] = saved
  assert(messages[1] and messages[1].msg == require('alder.ui').MISSING, vim.inspect(messages))
  assert(modal.state == nil)
end)
```

Run: `… tests/run.lua 'modal:'` → FAIL (module not found).

- [ ] **Step 3: Implement the first half of `modal.lua`**

```lua
-- modal: the generator UI — one centred nui layout: a search line over the generator list on
-- the left, the selected generator's form above its live preview on the right. Every edit asks
-- the server for alder/preview; <CR> plans and applies through apply.lua; <C-o> opens the
-- review. Modelled on durandal.nvim's generate.lua: a state singleton, a sequence number on
-- previews, an idempotent close.
local ui = require('alder.ui')
local picker = require('alder.picker')
local form = require('alder.form')
local preview = require('alder.preview')

local M = {}
M.state = nil
local ns = vim.api.nvim_create_namespace('alder.modal')
local DEBOUNCE_MS = 80

local function request(...) return require('alder.lsp').request(...) end

-- params are a generator request's: the origin document and cursor, plus extra.
local function params(s, extra)
  local p = {
    textDocument = { uri = vim.uri_from_bufnr(s.buf) },
    position = { line = s.origin.cursor[1] - 1, character = s.origin.cursor[2] },
  }
  for k, v in pairs(extra or {}) do p[k] = v end
  return p
end

-- sets_payload is the sets to send: an object even when empty.
local function sets_payload(s)
  local out = vim.empty_dict()
  for k, v in pairs(form.sets(s.preset, s.typed)) do out[k] = v end
  return out
end

local function set_lines(buf, lines)
  vim.bo[buf].modifiable = true
  vim.api.nvim_buf_set_lines(buf, 0, -1, false, #lines > 0 and lines or { '' })
  vim.bo[buf].modifiable = false
end

-- selected is the generator item under the list cursor, nil for an empty list.
function M.selected()
  local s = M.state
  return s and s.filtered[s.selected] or nil
end

local function render_list()
  local s = M.state
  local lines, marks, headings = picker.lines(s.filtered, s.query == '')
  set_lines(s.bufs.list, lines)
  vim.api.nvim_buf_clear_namespace(s.bufs.list, ns, 0, -1)
  for _, m in ipairs(marks) do
    vim.api.nvim_buf_set_extmark(s.bufs.list, ns, m.row - 1, m.col, { end_col = #lines[m.row], hl_group = m.hl })
  end
  for _, h in ipairs(headings) do
    vim.api.nvim_buf_set_extmark(s.bufs.list, ns, h.row - 1, 0, { virt_lines = { { { h.text, 'Comment' } } }, virt_lines_above = true })
  end
  s.popups.list.border:set_text('bottom', (' %d of %d '):format(#s.filtered, #s.items), 'right')
  if #s.filtered > 0 and vim.api.nvim_win_is_valid(s.win.list) then
    pcall(vim.api.nvim_win_set_cursor, s.win.list, { s.selected, 0 })
  end
end

-- render_form and the preview functions are defined in the second half (Task 7); until then:
local render_form = function() end
M.send_preview = function() end

-- select makes filtered[i] the current generator: its here values (and preset, from a code
-- action) become the sets, the form and preview start over and a preview is asked for at once.
function M.select(i, preset)
  local s = M.state
  if not s then return end
  s.selected = math.max(1, math.min(i, #s.filtered))
  local sel = M.selected()
  s.preset = vim.tbl_extend('force', {}, (sel and sel.sets) or {}, preset or {})
  s.typed, s.fields, s.focus, s.pick, s.ready, s.preview = {}, {}, nil, 1, false, nil
  render_list()
  render_form()
  set_lines(s.bufs.preview, {})
  s.popups.preview.border:set_text('top', ' Preview ', 'left')
  if sel then M.send_preview() end
end

-- type_query filters the list; the selection survives when its item does, else the first row.
function M.type_query(q)
  local s = M.state
  if not s then return end
  local was = M.selected()
  s.query = q
  s.filtered = picker.filter(s.items, q)
  local idx = 1
  for i, it in ipairs(s.filtered) do
    if was and it.id == was.id then idx = i end
  end
  M.select(idx)
end

-- move steps the selection.
function M.move(delta)
  local s = M.state
  if not s or #s.filtered == 0 then return end
  M.select(s.selected + delta)
end

local function mount()
  local s = M.state
  local Layout = require('nui.layout')
  s.popups = {
    search = ui.pane('Generate', { enter = true }),
    list = ui.pane(nil, { win_options = { cursorline = true } }),
    form = ui.pane('Form'),
    preview = ui.pane('Preview', { buf_options = { filetype = 'diff' } }),
  }
  local box
  if ui.narrow() then
    local room = math.floor(vim.o.lines * 0.8) - 3
    box = Layout.Box({
      Layout.Box(s.popups.search, { size = 3 }),
      Layout.Box(s.popups.list, { size = math.max(5, math.floor(room * 0.3)) }),
      Layout.Box(s.popups.form, { size = math.max(4, math.floor(room * 0.25)) }),
      Layout.Box(s.popups.preview, { grow = 1 }),
    }, { dir = 'col' })
  else
    box = Layout.Box({
      Layout.Box({ Layout.Box(s.popups.search, { size = 3 }), Layout.Box(s.popups.list, { grow = 1 }) }, { dir = 'col', size = '35%' }),
      Layout.Box({ Layout.Box(s.popups.form, { size = 10 }), Layout.Box(s.popups.preview, { grow = 1 }) }, { dir = 'col', size = '65%' }),
    }, { dir = 'row' })
  end
  s.layout = Layout(vim.deepcopy(ui.modal), box)
  s.layout:mount()
  s.bufs, s.win = {}, {}
  for name, p in pairs(s.popups) do
    s.bufs[name], s.win[name] = p.bufnr, p.winid
  end
  for _, name in ipairs({ 'list', 'preview' }) do vim.bo[s.bufs[name]].modifiable = false end

  -- the search line: typing filters; the arrows move; Tab goes to the form; Esc closes
  local search = s.bufs.search
  local function map(lhs, fn) vim.keymap.set({ 'i', 'n' }, lhs, fn, { buffer = search, nowait = true }) end
  map('<Down>', function() M.move(1) end)
  map('<C-n>', function() M.move(1) end)
  map('<Up>', function() M.move(-1) end)
  map('<C-p>', function() M.move(-1) end)
  map('<Tab>', function() M.focus_field(1) end)
  map('<CR>', function() M.enter() end)
  map('<C-o>', function() M.review() end)
  map('<Esc>', function() M.close() end)
  map('<C-c>', function() M.close() end)
  vim.api.nvim_create_autocmd({ 'TextChangedI', 'TextChanged' }, {
    buffer = search,
    callback = function()
      local line = vim.api.nvim_buf_get_lines(search, 0, 1, false)[1] or ''
      if M.state and line ~= M.state.query then M.type_query(line) end
    end,
  })
  -- leaving the modal for another window closes it (unless the review is up)
  for _, p in pairs(s.popups) do
    p:on('BufLeave', function()
      vim.schedule(function()
        local st = M.state
        if not st or st.suspended then return end
        local cur = vim.api.nvim_get_current_win()
        for _, w in pairs(st.win) do
          if w == cur then return end
        end
        M.close()
      end)
    end)
  end
  M.form_keys()
  vim.api.nvim_set_current_win(s.win.search)
  vim.cmd.startinsert()
end

-- open asks for the generators at buf's cursor and shows the modal; opts.generator (an id) and
-- opts.sets preset it, as a code action does.
function M.open(buf, opts)
  opts = opts or {}
  if buf == nil or buf == 0 then buf = vim.api.nvim_get_current_buf() end
  if not ui.has_nui() then return vim.notify(ui.MISSING, vim.log.levels.WARN) end
  M.close()
  local win = vim.api.nvim_get_current_win()
  if vim.api.nvim_win_get_buf(win) ~= buf then win = vim.fn.bufwinid(buf) end
  local origin = { win = win, cursor = win ~= -1 and vim.api.nvim_win_get_cursor(win) or { 1, 0 } }
  local token = {}
  M.opening = token
  request(buf, 'alder/generators', { textDocument = { uri = vim.uri_from_bufnr(buf) }, position = { line = origin.cursor[1] - 1, character = origin.cursor[2] } }, function(result)
    if M.opening ~= token then return end
    M.opening = nil
    local items = (ui.plain(result) or {}).generators or {}
    if #items == 0 then return vim.notify('alder: no generators', vim.log.levels.WARN) end
    M.state = {
      buf = buf, origin = origin, items = items, filtered = picker.filter(items, ''), selected = 1, query = '',
      preset = {}, typed = {}, fields = {}, seq = 0, ready = false, timer = vim.uv.new_timer(),
    }
    mount()
    local idx = 1
    if opts.generator then
      for i, it in ipairs(items) do
        if it.id == opts.generator then idx = i end
      end
      M.state.focus_first_missing = true
    end
    M.select(idx, opts.sets)
  end, function() M.opening = nil end)
end

-- close is idempotent: stop the timer, leave insert, unmount, restore the origin window and
-- cursor (again scheduled: leaving insert moves the cursor a column).
function M.close()
  M.opening = nil
  local s = M.state
  if not s then return end
  M.state = nil
  if s.timer then
    s.timer:stop()
    s.timer:close()
  end
  vim.cmd.stopinsert()
  pcall(function() s.layout:unmount() end)
  local function restore()
    if vim.api.nvim_win_is_valid(s.origin.win) then
      vim.api.nvim_set_current_win(s.origin.win)
      pcall(vim.api.nvim_win_set_cursor, s.origin.win, s.origin.cursor)
    end
  end
  restore()
  vim.schedule(restore)
end

-- Task 7 fills these in; stubs keep Task 6's tests meaningful.
function M.form_keys() end
function M.focus_field() end
function M.enter() end
function M.review() end

return M
```

- [ ] **Step 4: Run**

Run: `… tests/run.lua 'modal:'`
Expected: all six `modal:` tests pass (the preview pane stays empty because `send_preview` is a stub; nothing in these tests needs it).

- [ ] **Step 5: Commit**

```bash
git add lua/alder/modal.lua lua/alder/lsp.lua tests/modal_spec.lua
git commit -m "nvim: modal.lua — layout, generator list, search, open/close"
```

---

### Task 7: `modal.lua` — form, live preview, enter, review

**Files:**
- Modify: `lua/alder/modal.lua` (replace the stubs)
- Test: `tests/modal_spec.lua` (append)

**Interfaces:**
- Produces: `modal.set_field(name, text)`, `modal.focus_field(row)`, `modal.focus_next()`, `modal.focus_prev()`, `modal.focus_search()`, `modal.option(delta) → bool`, `modal.commit_option()`, `modal.toggle()`, `modal.send_preview()`, `modal.request_preview()`, `modal.on_preview(n, result)`, `modal.enter()`, `modal.review()`; state gains `fields, typed, focus, pick, ready, preview, updating`.

- [ ] **Step 1: Failing tests (append to `tests/modal_spec.lua`)**

```lua
local function text_of(buf) return table.concat(vim.api.nvim_buf_get_lines(buf, 0, -1, false), '\n') end

local function wait_preview(s)
  assert(H.wait(10000, function() return s.preview ~= nil and not s.updating end), 'no preview answer')
end

test('modal: selecting a generator shows its form; typing a field updates the preview', function()
  local buf = blog_buf('app/Posts/Index.alder')
  vim.fn.search('load params')
  local s = open(buf)
  modal.type_query('new msg')
  wait_preview(s)
  local names = vim.tbl_map(function(f) return f.name end, s.fields)
  assert(names[1] == 'page' and names[2] == 'name', vim.inspect(names))
  local lines = vim.api.nvim_buf_get_lines(s.bufs.form, 0, -1, false)
  assert(lines[1] == 'Posts.Index' and lines[2] == '', vim.inspect(lines))
  assert(not s.ready)
  assert(float_text():find(' new msg  ', 1, true), 'form title lacks the id:\n' .. float_text())
  modal.set_field('name', 'Delete')
  assert(not s.ready, 'ready must drop while the preview is in flight')
  assert(H.wait(10000, function() return s.ready end), 'never ready: ' .. vim.inspect(s.preview))
  local pv = text_of(s.bufs.preview)
  assert(pv:find('~ app/Posts/Index.alder  +', 1, true) and pv:find('@@', 1, true) and pv:find('Delete', 1, true), pv)
  assert(float_text():find(' app/Posts/Index.alder ', 1, true), 'preview title is the main file')
  modal.close()
end)

test('modal: Tab moves search → fields → search; the cursor row focuses a field', function()
  local buf = blog_buf('app/Posts/Index.alder')
  vim.fn.search('load params')
  local s = open(buf)
  modal.type_query('new msg')
  wait_preview(s)
  modal.focus_next()
  assert(vim.api.nvim_get_current_win() == s.win.form and s.focus == 1, vim.inspect(s.focus))
  modal.focus_next()
  assert(s.focus == 2 and vim.fn.mode():sub(1, 1) == 'i')
  modal.focus_next()
  modal.focus_next()
  assert(s.focus == nil and vim.api.nvim_get_current_win() == s.win.search, 'did not wrap to search')
  modal.focus_prev()
  assert(s.focus == #s.fields)
  modal.close()
end)

test('modal: typing in the form buffer is a field edit; a joined line is put back', function()
  local buf = blog_buf('app/Posts/Index.alder')
  vim.fn.search('load params')
  local s = open(buf)
  modal.type_query('new msg')
  wait_preview(s)
  modal.focus_field(2)
  vim.api.nvim_feedkeys('Toggle', 'x', false)
  assert(H.wait(1000, function() return s.typed.name == 'Toggle' end), vim.inspect(s.typed))
  assert(H.wait(10000, function() return s.ready end))
  vim.cmd.stopinsert()
  vim.api.nvim_buf_set_lines(s.bufs.form, 0, -1, false, { 'Posts.IndexToggle' }) -- a join
  assert(H.wait(1000, function() return #vim.api.nvim_buf_get_lines(s.bufs.form, 0, -1, false) == #s.fields end), 'rows not restored')
  assert(vim.api.nvim_buf_get_lines(s.bufs.form, 1, 2, false)[1] == 'Toggle', 'value lost in the re-render')
  modal.close()
end)

test('modal: a choice field lists options; Down picks, Tab commits', function()
  local buf = blog_buf('app/Posts/Index.alder')
  vim.fn.search('load params')
  local s = open(buf)
  modal.type_query('remove page')
  wait_preview(s)
  modal.focus_field(1)
  assert(s.fields[1].choices and #s.fields[1].choices >= 2, vim.inspect(s.fields[1]))
  local marks = vim.api.nvim_buf_get_extmarks(s.bufs.form, -1, 0, -1, { details = true })
  local has_options = false
  for _, m in ipairs(marks) do
    if m[4].virt_lines then has_options = true end
  end
  assert(has_options, 'no options under the choice field')
  assert(modal.option(1))
  assert(s.pick == 2)
  modal.commit_option()
  assert(s.typed.page == form_options_second(s), vim.inspect(s.typed))
  modal.close()
end)

-- form_options_second is the second option of the focused choice field.
function form_options_second(s)
  local f = s.fields[s.focus]
  return require('alder.form').options(f, '')[2]
end

test('modal: Enter when not ready jumps to the first invalid field; when ready it applies', function()
  local buf = blog_buf('app/Posts/Index.alder')
  vim.fn.search('load params')
  local before = text_of(buf)
  local s = open(buf)
  modal.type_query('new msg')
  wait_preview(s)
  modal.enter()
  assert(modal.state == s and s.focus == 2 and vim.api.nvim_get_current_win() == s.win.form, 'enter did not jump to `name`')
  modal.set_field('name', 'Delete')
  assert(H.wait(10000, function() return s.ready end))
  local messages, restore = H.capture_notify()
  modal.enter()
  assert(H.wait(10000, function() return text_of(buf):find('Delete', 1, true) ~= nil end), 'not applied: ' .. vim.inspect(messages))
  restore()
  assert(modal.state == nil and float_text() == '', 'modal still open after apply')
  assert(vim.api.nvim_get_current_buf() == buf)
  vim.cmd('silent undo')
  assert(text_of(buf) == before, 'one u does not restore')
end)

test('modal: a preset from the code-action command selects the generator and focuses the first missing field', function()
  local buf = blog_buf('app/Posts/Index.alder')
  vim.fn.search('load params')
  vim.lsp.commands['alder.generate']({ arguments = { { id = 'new msg', sets = { page = 'Posts.Index' } } } }, { bufnr = buf })
  assert(H.wait(5000, function() return modal.state ~= nil and modal.state.bufs ~= nil end))
  local s = modal.state
  assert(modal.selected().id == 'new msg')
  assert(H.wait(10000, function() return s.focus == 2 end), 'focus ' .. tostring(s.focus))
  assert(vim.api.nvim_get_current_win() == s.win.form)
  modal.close()
end)

test('modal: C-o opens the review over the modal; q returns to it; Enter applies', function()
  local buf = blog_buf('app/Posts/Index.alder')
  vim.fn.search('load params')
  local s = open(buf)
  modal.type_query('new msg')
  wait_preview(s)
  modal.set_field('name', 'Delete')
  assert(H.wait(10000, function() return s.ready end))
  modal.review()
  assert(H.wait(10000, function() return float_text():find(' files ', 1, true) ~= nil end), float_text())
  assert(float_text():find('@@', 1, true) and s.suspended)
  vim.api.nvim_feedkeys('q', 'x', false)
  assert(H.wait(1000, function() return not s.suspended end))
  assert(modal.state == s and float_text():find(' Generate ', 1, true) and not float_text():find(' files ', 1, true))
  modal.review()
  assert(H.wait(10000, function() return float_text():find(' files ', 1, true) ~= nil end))
  vim.api.nvim_feedkeys(vim.keycode('<CR>'), 'x', false)
  assert(H.wait(10000, function() return text_of(buf):find('Delete', 1, true) ~= nil end), 'review Enter did not apply')
  assert(modal.state == nil and float_text() == '')
end)

test('modal: a failed preview request clears updating and shows the error in the pane', function()
  local buf = blog_buf('app/Posts/Index.alder')
  vim.fn.search('load params')
  local s = open(buf)
  modal.type_query('new msg')
  wait_preview(s)
  local lsp = require('alder.lsp')
  local orig = lsp.request
  lsp.request = function(_, method, _, _, on_err)
    if method == 'alder/preview' and on_err then on_err({ code = -32603, message = 'boom' }) end
  end
  local messages, restore = H.capture_notify()
  local ok, err = pcall(function()
    modal.set_field('name', 'Delete')
    assert(H.wait(2000, function() return not s.updating end), 'updating stuck')
  end)
  restore()
  lsp.request = orig
  assert(ok, err)
  assert(not s.ready and text_of(s.bufs.preview):find('boom', 1, true), text_of(s.bufs.preview))
  modal.close()
end)
```

(`vim.lsp.commands['alder.generate']` is rewired in Task 8; until then that one test fails — it is listed here so the whole modal behaviour sits in one spec. Run the others with explicit patterns.)

- [ ] **Step 2: Implement the second half**

Replace the `render_form`/`send_preview` placeholders and the four stubs at the end of `modal.lua` with:

```lua
-- ── The form ─────────────────────────────────────────────────────────────────
-- One line per field: the line's text is the value, edited in place; the label is inline
-- virtual text, the note at the end of the line; a focused choice field lists its options
-- under it; a switch is `[x] name`, toggled with <Space>.

render_form = function()
  local s = M.state
  if not s or not s.bufs then return end
  local buf = s.bufs.form
  local lines, marks = form.lines(s.fields, s.typed)
  if #lines == 0 then lines = { '' } end
  vim.bo[buf].modifiable = true
  -- only the lines that differ (a re-render is not an edit of yours), then remember the tick:
  -- a change with that tick is this render, not typing
  local current = vim.api.nvim_buf_get_lines(buf, 0, -1, false)
  if #current ~= #lines then
    vim.api.nvim_buf_set_lines(buf, 0, -1, false, lines)
  else
    for i, l in ipairs(lines) do
      if current[i] ~= l then vim.api.nvim_buf_set_lines(buf, i - 1, i, false, { l }) end
    end
  end
  s.tick = vim.api.nvim_buf_get_changedtick(buf)
  vim.api.nvim_buf_clear_namespace(buf, ns, 0, -1)
  for _, m in ipairs(marks) do
    if m.label ~= '' then
      vim.api.nvim_buf_set_extmark(buf, ns, m.row - 1, 0, { virt_text = { { m.label, 'Identifier' } }, virt_text_pos = 'inline', right_gravity = false })
    end
    if m.note then
      vim.api.nvim_buf_set_extmark(buf, ns, m.row - 1, 0, { virt_text = { { '  ' .. m.note, m.note_hl } }, virt_text_pos = 'eol' })
    end
  end
  local f = s.focus and s.fields[s.focus]
  if f and f.choices and #f.choices > 0 then
    local opts = form.options(f, s.typed[f.name])
    local pick = math.max(1, math.min(s.pick or 1, #opts))
    local from = math.max(1, math.min(pick - 5, #opts - 5))
    local shown = {}
    for k = from, math.min(#opts, from + 5) do
      shown[#shown + 1] = { { (k == pick and '▸ ' or '  ') .. opts[k], k == pick and 'PmenuSel' or 'Pmenu' } }
    end
    if #shown > 0 then vim.api.nvim_buf_set_extmark(buf, ns, s.focus - 1, 0, { virt_lines = shown }) end
  end
  local sel = M.selected()
  s.popups.form.border:set_text('top', sel and (' ' .. sel.id .. '  ' .. (sel.summary or '') .. ' ') or ' Form ', 'left')
end

-- set_field records text for field name and asks for a preview (debounced).
function M.set_field(name, text)
  local s = M.state
  if not s then return end
  s.typed[name] = text
  s.pick = 1
  render_form()
  M.request_preview()
end

-- focus_field puts the cursor on row (wrapping) in insert mode (normal for a switch).
function M.focus_field(row)
  local s = M.state
  if not s or #s.fields == 0 then return end
  row = ((row - 1) % #s.fields) + 1
  s.focus, s.pick = row, 1
  vim.api.nvim_set_current_win(s.win.form)
  local line = vim.api.nvim_buf_get_lines(s.bufs.form, row - 1, row, false)[1] or ''
  pcall(vim.api.nvim_win_set_cursor, s.win.form, { row, #line })
  render_form()
  if s.fields[row].kind == 'bool' then vim.cmd.stopinsert() else vim.cmd.startinsert({ bang = true }) end
end

function M.focus_search()
  local s = M.state
  if not s then return end
  s.focus = nil
  render_form()
  vim.api.nvim_set_current_win(s.win.search)
  vim.cmd.startinsert({ bang = true })
end

function M.focus_next()
  local s = M.state
  if not s then return end
  M.commit_option()
  if s.focus == nil then return M.focus_field(1) end
  if s.focus >= #s.fields then return M.focus_search() end
  M.focus_field(s.focus + 1)
end

function M.focus_prev()
  local s = M.state
  if not s then return end
  M.commit_option()
  if s.focus == nil then return M.focus_field(#s.fields) end
  if s.focus <= 1 then return M.focus_search() end
  M.focus_field(s.focus - 1)
end

-- option moves through the focused choice field's options; false when there are none.
function M.option(delta)
  local s = M.state
  local f = s and s.focus and s.fields[s.focus]
  if not f or not f.choices or #f.choices == 0 then return false end
  local n = #form.options(f, s.typed[f.name])
  if n == 0 then return false end
  s.pick = ((((s.pick or 1) + delta) - 1) % n) + 1
  render_form()
  return true
end

-- commit_option writes the picked option into the focused choice field (no-op elsewhere).
function M.commit_option()
  local s = M.state
  local f = s and s.focus and s.fields[s.focus]
  if not f or not f.choices or #f.choices == 0 then return end
  local opts = form.options(f, s.typed[f.name])
  local pick = opts[math.max(1, math.min(s.pick or 1, #opts))]
  if pick and pick ~= (s.typed[f.name] or f.value) then M.set_field(f.name, pick) end
end

-- toggle flips the switch under the cursor.
function M.toggle()
  local s = M.state
  if not s then return end
  local row = vim.api.nvim_win_get_cursor(s.win.form)[1]
  local f = s.fields[row]
  if not f or f.kind ~= 'bool' then return end
  local line = vim.api.nvim_buf_get_lines(s.bufs.form, row - 1, row, false)[1] or ''
  s.focus = row
  M.set_field(f.name, form.is_on(line) and 'false' or 'true')
end

-- on_form_changed: typing in the focused field, or something that broke the rows (a joined or
-- added line) — put back, never losing a value. A change with the last render's tick is the
-- render itself.
function M.on_form_changed()
  local s = M.state
  if not s or not s.bufs then return end
  local buf = s.bufs.form
  if vim.api.nvim_buf_get_changedtick(buf) == s.tick then return end
  local lines = vim.api.nvim_buf_get_lines(buf, 0, -1, false)
  if #lines ~= math.max(1, #s.fields) then return render_form() end
  local row = vim.api.nvim_win_get_cursor(s.win.form)[1]
  local f = s.fields[row]
  if not f then return end
  if f.kind == 'bool' then return render_form() end
  s.focus = row
  local shown = s.typed[f.name] ~= nil and s.typed[f.name] or (f.value or '')
  if lines[row] ~= shown then
    s.typed[f.name] = lines[row]
    s.pick = 1
    s.tick = vim.api.nvim_buf_get_changedtick(buf)
    render_form()
    M.request_preview()
  else
    s.tick = vim.api.nvim_buf_get_changedtick(buf)
  end
end

-- on_cursor_moved: the cursor on another field's line focuses it.
function M.on_cursor_moved()
  local s = M.state
  if not s or not s.bufs or vim.api.nvim_get_current_win() ~= s.win.form then return end
  local row = vim.api.nvim_win_get_cursor(s.win.form)[1]
  if s.fields[row] and row ~= s.focus then
    M.commit_option()
    s.focus, s.pick = row, 1
    render_form()
  end
end

-- form_keys maps the form pane.
function M.form_keys()
  local s = M.state
  local buf = s.bufs.form
  local function map(lhs, fn) vim.keymap.set({ 'i', 'n' }, lhs, fn, { buffer = buf, nowait = true }) end
  map('<Tab>', M.focus_next)
  map('<S-Tab>', M.focus_prev)
  map('<CR>', function() M.enter() end)
  map('<C-o>', function() M.review() end)
  map('<Esc>', function() M.close() end)
  map('<C-c>', function() M.close() end)
  for lhs, delta in pairs({ ['<Down>'] = 1, ['<C-n>'] = 1, ['<Up>'] = -1, ['<C-p>'] = -1 }) do
    map(lhs, function()
      if not M.option(delta) then
        vim.api.nvim_feedkeys(vim.api.nvim_replace_termcodes(delta > 0 and '<Down>' or '<Up>', true, false, true), 'n', false)
      end
    end)
  end
  vim.keymap.set('n', '<Space>', M.toggle, { buffer = buf, nowait = true })
  for _, k in ipairs({ 'o', 'O', 'dd', 'J', 'p', 'P' }) do vim.keymap.set('n', k, '<Nop>', { buffer = buf, nowait = true }) end
  -- <BS> at the start of a line would join it with the one above: swallow it there
  vim.keymap.set('i', '<BS>', function()
    return vim.api.nvim_win_get_cursor(0)[2] == 0 and '' or '<BS>'
  end, { buffer = buf, expr = true, nowait = true })
  vim.api.nvim_create_autocmd({ 'TextChangedI', 'TextChanged' }, { buffer = buf, callback = function() M.on_form_changed() end })
  vim.api.nvim_create_autocmd({ 'CursorMovedI', 'CursorMoved' }, { buffer = buf, callback = function() M.on_cursor_moved() end })
end

-- ── The preview ──────────────────────────────────────────────────────────────

local function updating(on)
  local s = M.state
  if not s or not s.popups then return end
  s.updating = on
  s.popups.preview.border:set_text('bottom', on and ' updating… ' or '', 'right')
  if s.win and vim.api.nvim_win_is_valid(s.win.preview) then
    vim.wo[s.win.preview].winhighlight = on and 'Normal:Comment,FloatBorder:FloatBorder' or 'Normal:Normal,FloatBorder:FloatBorder'
  end
end

-- request_preview asks after a pause (one request for a burst of typing).
function M.request_preview()
  local s = M.state
  if not s then return end
  s.ready = false
  s.timer:stop()
  s.timer:start(DEBOUNCE_MS, 0, vim.schedule_wrap(function() M.send_preview() end))
end

-- send_preview asks now; its answer counts only if it is still the latest.
M.send_preview = function()
  local s = M.state
  local sel = M.selected()
  if not s or not sel then return end
  s.timer:stop()
  s.seq = s.seq + 1
  s.ready = false
  local n = s.seq
  updating(true)
  request(s.buf, 'alder/preview', params(s, { id = sel.id, sets = sets_payload(s) }), function(result)
    vim.schedule(function() M.on_preview(n, ui.plain(result) or {}) end)
  end, function(err)
    vim.schedule(function() M.on_preview(n, { error = 'the preview failed: ' .. tostring(err and err.message or err), files = {} }) end)
  end)
end

function M.on_preview(n, r)
  local s = M.state
  if not s or n ~= s.seq then return end
  updating(false)
  s.preview = r
  s.fields = r.fields or s.fields
  s.ready = r.ready == true
  render_form()
  local lines, hls = preview.render(r)
  set_lines(s.bufs.preview, lines)
  vim.api.nvim_buf_clear_namespace(s.bufs.preview, ns, 0, -1)
  for _, h in ipairs(hls) do
    vim.api.nvim_buf_set_extmark(s.bufs.preview, ns, h.row - 1, 0, { line_hl_group = h.hl })
  end
  local main = r.files and r.files[1]
  s.popups.preview.border:set_text('top', main and (' ' .. main.path .. ' ') or ' Preview ', 'left')
  if vim.api.nvim_win_is_valid(s.win.preview) then
    vim.api.nvim_win_call(s.win.preview, function() vim.fn.winrestview({ topline = preview.first_hunk(lines) }) end)
  end
  if s.focus_first_missing then
    s.focus_first_missing = nil
    local row = form.first_invalid(s.fields)
    if row then M.focus_field(row) end
  end
end

-- ── Enter and review ─────────────────────────────────────────────────────────

-- apply closes the modal and applies id with the current sets through apply.lua.
local function apply(s, id)
  local buf, sets, pos = s.buf, sets_payload(s), s.origin.cursor
  M.close()
  require('alder.apply').run(buf, id, sets, nil, pos)
end

-- enter applies when the form is ready, else goes to the first invalid field.
function M.enter()
  local s = M.state
  local sel = M.selected()
  if not s or not sel then return end
  M.commit_option()
  if not s.ready or s.updating then
    local row = form.first_invalid(s.fields)
    if row then M.focus_field(row) end
    return
  end
  apply(s, sel.id)
end

-- review plans and opens the review modal over this one; q comes back here.
function M.review()
  local s = M.state
  local sel = M.selected()
  if not s or not sel then return end
  M.commit_option()
  request(s.buf, 'alder/plan', params(s, { id = sel.id, sets = sets_payload(s) }), function(plan)
    local st = M.state
    if st ~= s then return end
    st.suspended = true
    vim.cmd.stopinsert()
    local shown = require('alder.review').open(ui.plain(plan) or {}, {
      id = sel.id,
      on_apply = function() apply(st, sel.id) end,
      on_back = function()
        if M.state ~= st then return end
        st.suspended = false
        if st.focus then M.focus_field(st.focus) else M.focus_search() end
      end,
    })
    if not shown then st.suspended = false end
  end)
end
```

Keep Task 6's `local render_form = function() end` line where it is (above `select`): Task 7's `render_form = function() … end` assigns to that same local, so `select` calls the real one. Delete Task 6's `M.send_preview = function() end` placeholder and the four stub functions at the end of the file.

- [ ] **Step 3: Run**

Run: `… tests/run.lua 'modal:'`
Expected: all pass except `modal: a preset from the code-action command…` (Task 8 rewires the command). If `focus_next` from the search pane fails because `<Tab>` in insert mode inserted a tab first, the `map` in `mount()` already covers `{ 'i', 'n' }` — check the failing assertion text.

- [ ] **Step 4: Commit**

```bash
git add lua/alder/modal.lua tests/modal_spec.lua
git commit -m "nvim: modal.lua — form, live preview, Enter and the review"
```

---

### Task 8: Wiring — `<leader>ag`, commands, code action, removals, bool toggle

**Files:**
- Modify: `lua/alder/config.lua:9`, `lua/alder/keys.lua`, `plugin/alder.lua:68-70,91-115`
- Delete: `lua/alder/flow.lua`, `lua/alder/actions.lua`, `tests/actions_spec.lua`
- Modify: `tests/keys_spec.lua` (the `aa`/`ag` tests), `tests/modal_spec.lua` (append the bool test)

- [ ] **Step 1: Failing tests**

In `tests/keys_spec.lua` replace the first two tests:

```lua
test('keys: ag ac ae ar are buffer-local; aa is gone (plan 2)', function()
  vim.g.mapleader = ' '
  blog_buf('app/Post.alder')
  for _, k in ipairs({ '<leader>ag', '<leader>ac', '<leader>ae', '<leader>ar' }) do
    assert(mapped(k), k .. ' not mapped')
  end
  assert(not mapped('<leader>aa'))
end)

test('keys: false disables one key', function()
  vim.g.mapleader = ' '
  with_opts({ keys = { run = false } }, function()
    blog_buf('app/Post.alder')
    assert(not mapped('<leader>ar'))
    assert(mapped('<leader>ag'))
  end)
end)

test('keys: ag opens the generator modal', function()
  vim.g.mapleader = ' '
  local buf = blog_buf('app/Posts/Index.alder')
  H.wait(1500, function() return false end)
  vim.api.nvim_feedkeys(vim.keycode('<leader>ag'), 'x', false)
  local modal = require('alder.modal')
  assert(H.wait(5000, function() return modal.state ~= nil and modal.state.bufs ~= nil end), 'modal did not open')
  assert(modal.state.buf == buf)
  modal.close()
end)

test('commands: AlderActions is gone; AlderGenerate [id] presets the modal', function()
  assert(vim.fn.exists(':AlderActions') == 0)
  assert(vim.fn.exists(':AlderGenerate') == 2)
  blog_buf('app/Posts/Index.alder')
  H.wait(1500, function() return false end)
  vim.cmd('AlderGenerate new msg')
  local modal = require('alder.modal')
  assert(H.wait(5000, function() return modal.state ~= nil and modal.state.bufs ~= nil end))
  assert(modal.selected().id == 'new msg')
  modal.close()
end)
```

Append to `tests/modal_spec.lua` (needs the plan-2a `alder` with `Field.Default` on PATH):

```lua
test('modal: a bool flag renders as a switch; Space toggles it into the sets', function()
  local buf = blog_buf('app/Posts/Index.alder')
  local s = open(buf)
  modal.type_query('new page')
  wait_preview(s)
  local row
  for i, f in ipairs(s.fields) do
    if f.name == 'example' then row = i end
  end
  assert(row, 'new page has no `example` flag: build alder from the m6b-plan2 branch (go install ./cmd/alder)')
  assert(vim.api.nvim_buf_get_lines(s.bufs.form, row - 1, row, false)[1] == '[x] example', vim.inspect(vim.api.nvim_buf_get_lines(s.bufs.form, 0, -1, false)))
  modal.focus_field(row)
  assert(vim.fn.mode():sub(1, 1) == 'n', 'a switch focuses in normal mode')
  vim.api.nvim_feedkeys(' ', 'x', false)
  assert(H.wait(1000, function() return s.typed.example == 'false' end), vim.inspect(s.typed))
  assert(vim.api.nvim_buf_get_lines(s.bufs.form, row - 1, row, false)[1] == '[ ] example')
  assert(H.wait(10000, function() return s.preview ~= nil and not s.updating end))
  assert(require('alder.form').sets(s.preset, s.typed).example == 'false')
  modal.close()
end)
```

Run: `… tests/run.lua 'keys:'` and `… tests/run.lua 'commands: AlderActions'` → FAIL.

- [ ] **Step 2: Wire it**

`lua/alder/config.lua`: `keys = { generate = '<leader>ag', diagnostics = '<leader>ac', explain = '<leader>ae', run = '<leader>ar' }, -- each can be false`.

`lua/alder/keys.lua`: replace the `actions` entry and description:

```lua
M.actions = {
  generate = function() require('alder.modal').open(0) end,
  diagnostics = function() vim.diagnostic.setqflist({ title = 'alder diagnostics', open = true }) end,
  explain = function() require('alder.check').explain() end,
  run = function() require('alder.commands').alder({ 'run' }) end,
}

local DESC = {
  generate = 'alder: generate (the generator modal)',
  diagnostics = 'alder: diagnostics to quickfix',
  explain = 'alder: explain the diagnostic',
  run = 'alder: run',
}
```

`plugin/alder.lua`: delete the `AlderActions` command block; change `AlderGenerate`'s function to

```lua
vim.api.nvim_create_user_command('AlderGenerate', function(o)
  local id = vim.trim(o.args)
  require('alder.modal').open(vim.api.nvim_get_current_buf(), { generator = id ~= '' and id or nil })
end, {
```

(keep its `nargs`, `complete` and update `desc` to `'alder generate [id]: the generator modal — search, form, live preview, apply'`), and the client command to

```lua
-- Neovim's own code action menu (gra) runs a generator action through this client command: the
-- modal opens preset with the generator and the values the server filled from the cursor.
vim.lsp.commands['alder.generate'] = function(cmd, ctx)
  local g = cmd.arguments[1]
  require('alder.modal').open(ctx.bufnr, { generator = g.id, sets = g.sets })
end
```

Then:

```bash
git rm lua/alder/flow.lua lua/alder/actions.lua tests/actions_spec.lua
grep -rn "alder.flow\|alder.actions\|alder://preview" lua plugin tests README.md
```

The grep must only hit `lua/alder/generate.lua` (the CLI path's `alder://preview` scratch, which stays) and README lines Task 9 rewrites. Fix any other hit.

- [ ] **Step 3: Build the plan-2a alder and run everything**

```bash
(cd ~/projects/alder && git checkout m6b-plan2 && go install ./cmd/alder) && which alder && alder help --json | grep -o '"--example"'
cd ~/projects/alder.nvim && nvim --headless -u tests/minimal.lua -l tests/run.lua
```

Expected: `--example` printed; the suite ends `N tests, 0 failed` (N ≈ 77 − 6 actions tests + ~30 new). If `~/go/bin` is not first on PATH in the test shell, prefix `PATH=$HOME/go/bin:$PATH`.

- [ ] **Step 4: Commit**

```bash
git add -A lua plugin tests
git commit -m "nvim: <leader>ag opens the modal; code actions preset it; flow/actions removed"
```

---

### Task 9: Docs, hand-test notes, and the book's key table

**Files:**
- Modify: `README.md` (install snippet, Commands, Keys, "Applying generators"), `~/projects/alder/book/11-tooling.md:1655-1725`
- Modify (user's config, outside the repos, by hand): `~/.config/nvim/lua/plugins/alder.lua` gains `dependencies = { 'MunifTanjim/nui.nvim' }`

- [ ] **Step 1: README**

- Install snippet: `keys = { generate = '<leader>ag', diagnostics = …, explain = …, run = … }`; add nui to `dependencies`.
- Commands table: drop `:AlderActions`; `:AlderGenerate [id]` → "open the generator modal (preset to `id`): search the generators, fill the form, watch the preview, `<CR>` applies, `<C-o>` reviews".
- Keys table: `<leader>ag` | the generator modal; drop `<leader>aa`. Below it: "Fixes and the generators that apply at the cursor are in Neovim's own code-action menu (`gra`); picking a generator there opens the modal preset."
- "Applying generators" section: first paragraph becomes:

```markdown
`<leader>ag` (or `:AlderGenerate`, or a generator from `gra`) opens the generator modal: a
search line over the generators (those that apply at the cursor first), the chosen
generator's form, and a live preview of the diff that updates as you type. `<Tab>` moves
between fields, `<CR>` applies when every field is valid (else it jumps to the first that is
not), `<C-o>` opens the full review (what the plan leaves to you, file by file), `<Esc>`
closes. The modal needs [nui.nvim](https://github.com/MunifTanjim/nui.nvim); without it,
`<leader>ag` says so and `:Alder <words>` still runs the CLI.
```

- Tests section: mention `NVIM_NUI`.

- [ ] **Step 2: Book**

`~/projects/alder/book/11-tooling.md`: in the "Code actions" bullet (≈1655) replace `` (`<leader>aa`, or Neovim's own `gra`) `` with `` (Neovim's own `gra`) `` and end the bullet with "Choosing a generator opens the generator modal (below) with its fields filled from the cursor." Replace the key table row `<leader>aa …` with `` | `<leader>ag` | the generator modal: search, form, live preview, `<CR>` applies, `<C-o>` reviews | `` and rewrite the paragraph after the table (the `:AlderGenerate` one) to describe the modal in two sentences (search over all generators, here-group first; form + live preview; `<C-o>` review). Commit that in the alder repo on `m6b-plan2`.

- [ ] **Step 3: Hand test (the user)**

In AstroNvim with `~/go/bin/alder` from the branch and `dependencies = { 'MunifTanjim/nui.nvim' }` in `~/.config/nvim/lua/plugins/alder.lua`, on the sandbox app: `<leader>ag` → type `msg` → `<Tab>` → `Delete` → watch the preview → `<C-o>` → `q` → `<CR>`; then `<leader>la` on a page line → `new msg` → the preset modal; `K` on `update`; `:AlderGenerate new page` → `[x] example` on, apply, read the page. Note anything that feels off in the memory file for plan 3.

- [ ] **Step 4: Commit and finish**

```bash
cd ~/projects/alder.nvim && git add README.md && git commit -m "docs: the generator modal, <leader>ag"
cd ~/projects/alder && git add book/11-tooling.md && git commit -m "book: the editor's generator modal"
```

Then `superpowers:finishing-a-development-branch` for both repos (merge `m6b-plan2` into `main`; alder.nvim has no remote, alder pushes).
