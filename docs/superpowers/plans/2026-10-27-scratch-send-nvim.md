# Scratch Send from nvim Implementation Plan (alder#44, part 2 — alder.nvim)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `<leader>as` shows under the `alder` which-key group in visual mode, and a sent definition lands in `scratch.alder` as a definition while an expression becomes a `> ` watch.

**Architecture:** Two small changes in `~/projects/alder.nvim`: `keys.attach` registers the which-key group for normal and visual mode; `scratch.append` classifies the selection's first line with a new `scratch.is_definition(line)` and inserts definitions verbatim.

**Tech Stack:** Lua (Neovim), the repo's headless test runner.

**Spec:** `~/projects/alder/docs/superpowers/specs/2026-10-27-scratch-send-latency-design.md` (sections 2 and 3)

## Global Constraints

- Repo: `~/projects/alder.nvim`, branch from `main`, one PR.
- Tests run with `nvim --headless -u tests/minimal.lua -l tests/run.lua <filter>`.
- In a scratch file, lines holding `>` watches still go in as they are (`raw`), unchanged.
- Commits end with the session's attribution lines.

## Review Focus

1. A selection whose first line is a comment (`-- …`) followed by a definition: classified as an expression and wrapped — acceptable for now; Task 2 tests that a leading comment line is wrapped, so the behaviour is at least deliberate.
2. A definition whose body spans lines must keep its indentation verbatim (no four-space prefix) — pinned in Task 2.
3. A `type` declaration (`type Shape = Circle | Square`) is a definition even though it contains `=` after a capital name — pinned in Task 2.
4. `is_definition` on a line that is only a name (`foo`) is an expression — pinned in Task 2.
5. which-key absent: `attach` must not error — already covered by `pcall(require, 'which-key')`; the Task 1 test restores `package.loaded['which-key']` so other specs see the real state.

---

### Task 1: which-key group in normal and visual mode

**Files:**
- Modify: `lua/alder/keys.lua` (`M.attach`, the `wk.add` line ~63)
- Test: `tests/keys_spec.lua`

- [ ] **Step 1: Write the failing test** `keys: which-key group covers normal and visual mode`: before `blog_buf(...)`, set `package.loaded['which-key'] = { add = function(spec) recorded = spec end }` (save and restore the previous value in a `pcall`-guarded cleanup as `with_opts` does); open the buffer; assert `recorded[1][1] == '<leader>a'`, `recorded[1].group == 'alder'`, and `vim.tbl_contains(recorded[1].mode, 'n')` and `'x'`.
- [ ] **Step 2:** `nvim --headless -u tests/minimal.lua -l tests/run.lua keys` — expected the new test FAILs (`mode` nil).
- [ ] **Step 3: Implement.** `wk.add({ { '<leader>a', group = 'alder', buffer = buf, mode = { 'n', 'x' } } })`.
- [ ] **Step 4:** Run the keys tests — expected PASS.
- [ ] **Step 5:** Commit: `keys: which-key group in visual mode too (#44)`.

### Task 2: Definitions are sent as definitions

**Files:**
- Modify: `lua/alder/scratch.lua` (`M.append` ~line 255–290; new `M.is_definition`), `README.md:142` (the `<leader>as` row)
- Test: `tests/scratch_spec.lua`

**Interfaces:**
- Produces: `M.is_definition(line: string) -> boolean` in `lua/alder/scratch.lua`; `M.append(root, lines, module, raw)` unchanged in signature.

- [ ] **Step 1: Write the failing classifier test** `scratch: is_definition` with this table (line → expected):
  - `'f x = x + 1'` → true; `'pair (a, b) = a'` → true; `'name : Int'` → true; `'x = 1'` → true
  - `'type Shape = Circle | Square'` → true; `'alias P = { x : Int }'` → true; `'import Html'` → true; `'test "adds" = 1 + 1 == 2'` → true; `'page Index = …'` → true
  - `'x == y'` → false; `'f x <= y'` → false; `'{ a = 1 }'` → false; `'let x = 1 in x'` → false; `'if a then b else c'` → false; `'foo'` → false; `'1 + 1'` → false; `'-- a comment'` → false; `'typeOf x'` → false (keyword needs a following space)
- [ ] **Step 2: Write the failing append tests**, following `scratch: send inside scratch.alder adds no second >`:
  - `scratch: a sent definition is not wrapped`: `S.append(app, { 'f x = x + 1' }, nil)` → file ends `\nf x = x + 1\n`.
  - `scratch: a sent signature and body keep their lines`: `S.append(app, { 'g : Int -> Int', 'g x =', '    x * 2' }, nil)` → file ends `\ng : Int -> Int\ng x =\n    x * 2\n`.
  - `scratch: a sent expression is a watch`: `S.append(app, { 'x == y' }, nil)` → `\n> x == y\n`; `{ '-- note', 'f x = 1' }` → `\n> -- note\n    f x = 1\n`.
- [ ] **Step 3:** `nvim --headless -u tests/minimal.lua -l tests/run.lua scratch` — expected the new tests FAIL.
- [ ] **Step 4: Implement `M.is_definition`.** Dedented first line; true when any of: starts with one of `type alias import page msg form table capability fake session login test` followed by a space; matches `^[%a_][%w_']*%s*:`; or matches `^[%l_][%w_']*` followed by only `[%w_'%s(),{}:]` characters up to a `=` that is not followed by `=` (write it as a find of the first `=`, a check of the prefix's character class, and a check of the next character) — unless the first word is `let`, `if` or `case`, which start expressions (`let x = 1 in x` would otherwise pass the binding rule). In `M.append`, when `not raw and M.is_definition(out[1])`, append `out` verbatim instead of the `> `/four-space form.
- [ ] **Step 5:** Run the scratch tests — expected PASS.
- [ ] **Step 6: README.** Replace the `<leader>as` row's description with: "send the current line (normal mode) or the selection (visual mode) to `scratch.alder` and save it: a definition (`f x = …`, `name : T`, `type …`) is added as written, an expression as a `> ` watch".
- [ ] **Step 7:** Commit: `scratch: send a definition as a definition (#44)`.

### Task 3: Verify and open the PR

- [ ] **Step 1:** `nvim --headless -u tests/minimal.lua -l tests/run.lua` — expected all PASS.
- [ ] **Step 2:** Open the PR (title `scratch send: visual-mode group, definitions as definitions (#44)`); body lists the two behaviours. The alder issue closes once both PRs are merged.
