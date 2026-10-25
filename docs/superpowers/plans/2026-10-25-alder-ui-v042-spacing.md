# alder-ui v0.4.2 spacing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Pages in Alder apps get shadcn's section spacing from their Layout's `al-stack gap="lg"`, `al-button` stops stretching in grids and columns, and `dl` reads as a detail list; then nightvote-replay picks it up and the user's verdict closes alder#21.

**Architecture:** alder-ui v0.4.2 changes three stylesheets (`al-button`, `al-stack`, typography) and ships as one PR and tag. Alder vendors it, wraps page content in a stack in every Layout it writes or ships (scaffold, admin generator, three examples), and documents the rule in Appendix C. nightvote-replay wraps its `Document` frame the same way and re-pins `ALDER_REF`.

**Tech Stack:** CSS (alder-ui), Python unittest + Playwright (alder-ui tests), Go (alder generators and tests), Alder (`.alder` Layouts), Node + Playwright (nvr `shots.mjs`).

**Spec:** `docs/superpowers/specs/2026-10-25-alder-ui-v042-spacing-design.md` (alder repo, branch `alder-ui-042-21`). Decisions D1–D11 are cited by number below.

## Global Constraints

- No outer margin on any block component; no page-level owl selector (spec §2 non-goals).
- The flow rhythm in `typography.css` (`h2/h3/h4/p + *`) stays unchanged (D5).
- The apps' `form:not(._alder-click) { display: grid; gap: 1rem }` rule stays in every app CSS file (§2, D7).
- Page stack markup, exactly: `<al-page …><main><al-stack gap="lg">{children}</al-stack></main></al-page>`; any `<header>` outside `<main>` is left alone (D1, D9).
- Gap scale: `sm` 0.5rem, `md` 1rem, `lg` 1.5rem, `xl` 2.5rem (D4).
- `dl` values (D3, D11): `dt` colour `--muted-foreground`, font-size 0.875rem, line-height 1.25rem; `dd` colour `--foreground`, `margin: 0`; `dl` `margin: 0`; at `@media (min-width: 40rem)` `grid-template-columns: max-content 1fr`, `column-gap: 1.5rem`, `row-gap: 0.5rem`; below, one column with `row-gap: 0.5rem`. All selectors under `:where(al-page:not([plain]))`.
- `full` on `al-button` = `inline-size: 100%` on host and inner `button`/`a`, in all three forms (D2).
- alder-ui gate (every alder-ui task): `.venv/bin/python scripts/build.py && .venv/bin/python -m unittest discover -s test -p 'test_*.py' && .venv/bin/python test/check.py && .venv/bin/python test/parity.py --strict` with parity reporting `failing: none, pending: none` (spec §4.5; ledger ruling R15).
- Order is fixed (D6): alder-ui PR merged and `v0.4.2` tagged and released → alder PR merged and binary reinstalled → nightvote-replay.
- Pushing nightvote-replay `main` redeploys Render and needs the user's explicit OK. Nothing in this plan pushes it.
- Working hours: no agent work 9am–5pm America/New_York on weekdays.

**Deviation from the spec, recorded here:** spec §6 names nvr branch `alder-ui-041`. That branch was merged to nvr `main` (a5d43a0) and deployed at ~14:40 ET on 2026-10-25. Task 9 works on a new branch `alder-ui-042` from `main`.

## Review Focus

1. **Row stacks keep their vertical centring.** A no-stretch host must not set `align-self`, or buttons in `al-stack direction="row"`, card actions, pagination and dialog actions stop centring. Test: Task 1 `test_row_stack_centres`.
2. **Hidden children add no gap in the page stack.** An empty-errors `al-alert`, `<input type="hidden">` and a closed `al-dialog` are `display: none`, so they are not flex items. Expect the gap between two visible siblings to stay 24px. Test: Task 3 `test_hidden_children_add_no_gap`.
3. **`dl` in the delete dialog is styled.** Alder renders `al-dialog` inside `al-page`, so its `dl` gets the grid. Test: Task 2 `test_dialog_dl_is_grid`.
4. **`al-page plain` keeps a bare `dl`.** Expect `display: block` and the browser's `dd` margin. Test: Task 2 `test_plain_dl_unstyled`.
5. **Icon buttons keep their square size in a grid.** `size="icon"` sets `--_al-w: 2.25rem`, and the no-stretch rule must not override it. Test: Task 1 `test_icon_in_grid_is_square`.

---

## Part A: alder-ui (repo `~/projects/alder-ui`, branch `spacing-042` from `main` 3c7bc47)

Before Task 1, the controller runs `git -C ~/projects/alder-ui switch -c spacing-042 main` and creates the ledger at `~/projects/alder-ui/.superpowers/sdd/2026-10-25-alder-ui-v042/progress.md`.

### Task 1: `al-button` never stretches; `full` opt-in — Opus

**Files:**
- Modify: `components/al-button.css` (host rule ~162–189, native-form rule ~220–228; add a `full` block after it)
- Modify: `custom-elements.json` (al-button attributes: append `full`)
- Modify: `test/check.py` (`ADDED["al-button"] = [("full", "boolean")]`)
- Modify: `demo/button.html` (new section "In a form and a column", before `</al-page>`)
- Modify: `README.md` (al-button row of the Elements table; replace the note at ~85)
- Modify: `test/parity-allow.json` only if parity fails on button `width`/`box-width`
- Create: `test/test_spacing.py`

**Interfaces:**
- Produces: `al-button[full]` (boolean attribute). Manifest entry: `{"name": "full", "type": {"text": "boolean"}, "description": "Makes the button as wide as its container (shadcn's w-full). Without it a button keeps its natural width, also in a grid or a column stack.", "default": "false"}`.
- Produces: `test/test_spacing.py` with a module-level Playwright fixture copied from `test/test_page.py` (server on `ROOT`, chromium, viewport 1280×800), and helper `box(page, selector) -> dict` returning `getBoundingClientRect()` as `{x, y, width, height}`. Tasks 2 and 3 add classes to this file.
- Produces in `demo/button.html`, section "In a form and a column":
  - `<form id="grid-form" action="#" style="display:grid;gap:1rem">` holding `<al-button id="g-bare">Bare</al-button>`, `<al-button id="g-button"><button type="submit">Save</button></al-button>`, `<al-button id="g-link" variant="outline"><a href="#">Cancel</a></al-button>`, `<al-button id="g-icon" size="icon" variant="outline"><button type="button" aria-label="Add">` + the plus icon the Icons section already uses + `</button></al-button>`, `<al-button id="g-full" full><button type="submit">Full width</button></al-button>`.
  - `<al-stack id="col-stack">` holding `c-bare`, `c-button` and `c-full` in the same three forms.

- [ ] **Step 1: Reproduce in the browser**

Add the demo section, run `.venv/bin/python scripts/build.py`, then write `test/test_spacing.py` class `ButtonLayoutTest` with these tests (±0.5px on positions and widths):

```python
def test_grid_buttons_start_aligned(self):
    # for id in g-bare, g-button, g-link: host.x == form content-box x; host.width < form.width / 2
def test_inner_fills_host(self):
    # g-button, g-link: inner button/a x and width == host x and width
def test_full_fills_grid(self):
    # g-full host width == form width; inner button width == host width
def test_column_stack(self):
    # c-bare, c-button: x == col-stack x, width < col-stack width / 2; c-full width == col-stack width
def test_icon_in_grid_is_square(self):
    # g-icon width == 36 and height == 36
def test_row_stack_centres(self):
    # in the Variants section's row stack: every al-button's (y + height/2) equals the stack's (y + height/2)
```

- [ ] **Step 2: Run them to see which fail**

Run: `.venv/bin/python -m unittest test.test_spacing -v`
Expected: `test_grid_buttons_start_aligned`, `test_column_stack` and `test_full_fills_grid` FAIL (bare host stretches; native-form inner button sits centred in a stretched host; `full` does nothing). Record in the ledger which forms failed. That record settles the README line "Buttons and badges keep their own width inside a column `al-stack`" (spec §4.1).

- [ ] **Step 3: Change `components/al-button.css`**

- Host default: `--_al-w: fit-content` instead of `auto`, so the host and inner `button`/`a` size to content and grid/flex `stretch` no longer applies. Icon sizes keep their explicit `--_al-w`.
- Native form (`al-button:has(> :is(button, a))`): `inline-size: fit-content` instead of `auto`.
- Do not set `align-self` on the host. It would override `align-items: center` in row stacks (Review Focus 1). Add `justify-self: start` only if Step 4 still fails without it.
- New block after the native-form rule, with a comment `/* w-full: opt in to the container's width */`: `al-button[full], al-button[full] > :is(button, a) { inline-size: 100%; }`. `[full]` scores (0,1,1), which beats the native form's `:has()` at (0,0,2).

- [ ] **Step 4: Run the tests to see them pass**

Run: `.venv/bin/python scripts/build.py && .venv/bin/python -m unittest test.test_spacing -v`
Expected: all six PASS.

- [ ] **Step 5: Manifest, check and README**

Add `full` to the manifest and `ADDED` in `check.py`. In the README Elements table, al-button's attributes gain `full`. Replace the note at ~85 with: "A button keeps its natural width in a grid form or a column `al-stack`; add `full` for a full-width button."

- [ ] **Step 6: Full gate**

Run: the alder-ui gate (Global Constraints).
Expected: all unittests pass, check ok, parity `failing: none, pending: none`. If parity fails on a button `width` or `box-width`, add one allow entry `{"ref": "button/*", "prop": "<prop>", "reason": "structural: no-stretch host; shadcn relies on w-fit/self-start"}` and re-run. Any other parity failure is a stop-and-report. Also open `demo/card.html`, `demo/pagination.html` and `demo/dialog.html` at 1280px and confirm the action and nav buttons look as before.

- [ ] **Step 7: Commit**

```bash
git add components/al-button.css custom-elements.json test/check.py test/test_spacing.py demo/button.html README.md al.css index.html test/parity-allow.json
git commit -m "fix(al-button): never stretch in grids or columns; add full (alder#39)"
```

### Task 2: `dl` as a detail list in typography — Sonnet

**Files:**
- Modify: `typography.css` (new block after the `table`/`tr` rules)
- Modify: `demo/page.html` (Typography section: a `dl id="detail"` with three pairs; Plain section: a `dl id="plain-detail"`)
- Modify: `demo/dialog.html` (the "Delete this post?" dialog: a `dl id="delete-detail"` with Title and Author pairs before its form)
- Modify: `test/test_spacing.py` (class `DetailListTest`)
- Modify: `README.md` (one sentence in the typography paragraph: "`dl` is a detail list: muted labels beside their values from 40rem, stacked below.")

**Interfaces:**
- Consumes: `test/test_spacing.py`'s fixture and `box()` from Task 1.
- Produces: the selectors `:where(al-page:not([plain])) dl`, `… dt`, `… dd` (D3).

- [ ] **Step 1: Write the failing tests**

Viewport 1280 unless noted. Read colours with `getComputedStyle`, and compare against a probe element whose `color` is `var(--muted-foreground)` / `var(--foreground)`.

```python
def test_dl_two_columns_wide(self):
    # #detail: display grid; first dt.y == first dd.y; dd.x > dt.x; column-gap 24px; row-gap 8px
def test_dl_stacked_narrow(self):
    # viewport 600: first dd.y >= first dt.y + dt.height; dd.x == dt.x
def test_dt_dd_look(self):
    # dt: color == muted probe, font-size 14px, line-height 20px; dd: color == foreground probe, margin-left 0px
def test_dialog_dl_is_grid(self):
    # #delete-detail computed display == "grid" and grid-template-columns has two tracks at 1280
def test_plain_dl_unstyled(self):
    # #plain-detail display == "block"; its dd margin-left == "40px"
```

- [ ] **Step 2: Run them to see them fail**

Run: `.venv/bin/python scripts/build.py && .venv/bin/python -m unittest test.test_spacing.DetailListTest -v`
Expected: the first four FAIL (`display: block`); `test_plain_dl_unstyled` passes.

- [ ] **Step 3: Write the `dl` block in `typography.css`**

The values are in Global Constraints. Comment it like the flow rhythm block: an alder addition beyond shadcn Typography, in shadcn's tokens. Every selector stays inside `:where(...)` (check.py's typography-weight check).

- [ ] **Step 4: Run the tests to see them pass**

Run: same as Step 2.
Expected: all five PASS.

- [ ] **Step 5: Full gate and commit**

Run: the alder-ui gate.
Expected: as Task 1 Step 6, with no new allow entries.

```bash
git add typography.css demo/page.html demo/dialog.html test/test_spacing.py README.md al.css index.html
git commit -m "feat(typography): dl as a detail list (alder#39)"
```

### Task 3: `al-stack gap="xl"`, the Spacing rule, v0.4.2 release PR — Sonnet

**Files:**
- Modify: `components/al-stack.css` (after the `lg` rule)
- Modify: `custom-elements.json` (al-stack `gap` type `"sm" | "md" | "lg" | "xl"`, description names `xl`; top-level `version` → `v0.4.2`)
- Modify: `test/check.py` (`ADDED_VALUES[("al-stack", "gap")] = ["xl"]`)
- Modify: `demo/stack.html` (an `xl` example `id="xl-stack"`, and the page-stack example below)
- Modify: `test/test_spacing.py` (class `StackTest`)
- Modify: `README.md` (al-stack row; new "Spacing" section; "Upgrading to v0.4.2" note)
- Create: `.superpowers/sdd/2026-10-25-alder-ui-v042/release-notes-v0.4.2.md` (git-ignored, used by `gh release create`)

**Interfaces:**
- Consumes: `test/test_spacing.py` fixture (Task 1); `full` (Task 1) and `dl` (Task 2) for the release notes.
- Produces: tag `v0.4.2` (after the user merges the PR), whose manifest version is `v0.4.2`. Part B's vendor script checks tag == manifest version.
- Produces in `demo/stack.html`: `<al-stack id="page-stack" gap="lg">` holding `<p id="ps-1">One</p>`, `<input type="hidden" name="x">`, `<al-alert variant="destructive"><ul class="errors"></ul></al-alert>`, `<al-dialog heading="Closed">…</al-dialog>`, `<p id="ps-2">Two</p>`.

- [ ] **Step 1: Write the failing tests**

```python
def test_gap_xl(self):
    # #xl-stack computed row-gap == "40px"
def test_hidden_children_add_no_gap(self):
    # ps-2.y - (ps-1.y + ps-1.height) == 24 (±0.5)
```

- [ ] **Step 2: Run them to see them fail**

Run: `.venv/bin/python scripts/build.py && .venv/bin/python -m unittest test.test_spacing.StackTest -v`
Expected: `test_gap_xl` FAILS (16px, the default); `test_hidden_children_add_no_gap` PASSES. It pins Review Focus 2, so a pass is fine, and a failure is a stop-and-report.

- [ ] **Step 3: Add the rule, manifest and check entries**

In `al-stack.css`: `/* gap-10 */ al-stack[gap="xl"] { gap: 2.5rem; }`.

- [ ] **Step 4: Run the tests to see them pass**

Run: same as Step 2.
Expected: both PASS.

- [ ] **Step 5: README**

- The al-stack row: `gap` (`sm`, `md` default, `lg`, `xl`).
- A "Spacing" section directly before "## Elements", with the spec §4.4 text verbatim.
- After it, a "Upgrading to v0.4.2" paragraph: `al-button` no longer stretches in grids or column stacks, so add `full` where a full-width button was relied on; put the Layout's page content in `<al-stack gap="lg">` (the snippet from Global Constraints).

- [ ] **Step 6: Bump the version, full gate**

Set the manifest `version` to `v0.4.2`. Run: the alder-ui gate.
Expected: green. `head -1 al.js` shows `alder-ui v0.4.2`.

- [ ] **Step 7: Commit, push, open the PR**

```bash
git add components/al-stack.css custom-elements.json test/check.py demo/stack.html test/test_spacing.py README.md al.css al.js index.html
git commit -m "feat(al-stack): gap xl; document spacing; v0.4.2"
git push -u origin spacing-042
gh pr create --base main --head spacing-042 --title "v0.4.2: spacing the shadcn way (alder#39)" --body "<D2–D4 summary, the Spacing rule, the upgrade note, gate output; Refs tapegram/alder#39>"
```
Write the release notes file (the same summary plus the upgrade note). Expected: CI green on the PR.

**Controller, after Task 3:** an Opus whole-branch review of `main..spacing-042`, with a fix wave if needed. Then the user merges the PR, or tells the controller to. After the merge:

```bash
git switch main && git pull && git tag v0.4.2 && git push origin v0.4.2
gh release create v0.4.2 --title v0.4.2 --notes-file .superpowers/sdd/2026-10-25-alder-ui-v042/release-notes-v0.4.2.md
git branch -d spacing-042 && git push origin --delete spacing-042
```

Then refresh GitHub Pages: no action is needed, because `index.html` is committed.

---

## Part B: alder (repo `~/projects/alder`, branch `alder-ui-042-21`, already holding the spec and this plan)

Before Task 4, the controller rebases `alder-ui-042-21` on `origin/main` if main moved, and confirms the `v0.4.2` tag exists (`gh release view v0.4.2 -R tapegram/alder-ui`).

### Task 4: Vendor alder-ui v0.4.2 — Sonnet

**Files:**
- Vendored by the script: `rt/al/{al.css,al.js,custom-elements.json,VERSION}` (fonts and licences unchanged)
- Modify: `book/C-design-system.md` (al-button table gains a `full` row; al-stack `gap` type gains `"xl"` and the meaning names 2.5rem; the shipped version at lines 38, 1087 and 1144 becomes `v0.4.2`)
- Modify: any completion or checker golden that lists the al-stack gap union or al-button attributes

**Interfaces:**
- Produces: the checker and LSP accept `<al-button full>` and `<al-stack gap="xl">`. Tasks 5 and 6 rely on `full` and `xl` passing `alder check`.

- [ ] **Step 1: Vendor**

Run: `scripts/vendor-al.sh v0.4.2`
Expected: `vendored alder-ui v0.4.2 into rt/al/`; `cat rt/al/VERSION` prints `v0.4.2`; `git status` shows only the four files above changed under `rt/al/`.

- [ ] **Step 2: See what the manifest changes break**

Run: `go test ./book/ ./internal/elements/... ./internal/lsp/... ./rt/... ./cmd/alder -short 2>&1 | grep -E '^(--- FAIL|FAIL|ok)'`
Expected: at most `TestAppendixC_ManifestDrift` and union-listing goldens fail. Any failure not explained by `full` or `xl` is a stop-and-report.

- [ ] **Step 3: Update Appendix C tables and goldens**

Edit the tables in manifest order. Regenerate goldens with the package's `-update` flag where one exists, and check the diff holds only the additions.

- [ ] **Step 4: Check the injected UI**

Run: `grep -n '<dl\|al-button' rt/web/*.go cmd/alder/devui.go`
Expected: no `dl`, and no `al-button` placed in a grid or column whose layout depended on stretching. If one is found, report it in the commit message and add `full` where the old width was intended.

- [ ] **Step 5: Run the tests to see them pass, then commit**

Run: same as Step 2.
Expected: all `ok`.

```bash
git add rt/al book/C-design-system.md <goldens>
git commit -m "chore: vendor alder-ui v0.4.2 (al-button full, al-stack gap xl, dl)"
```

### Task 5: The page stack in every Layout — Sonnet

**Files:**
- Modify: `internal/scaffold/files/app/Layout.alder:15`
- Modify: `internal/generate/admin.go` (`layout()`, the no-app-Layout branch ~571–575 only; the `g.p.Layout` branch is unchanged, per D8)
- Modify: `examples/todos/app/Layout.alder:14`, `examples/blog/app/Layout.alder:17`, `examples/workspace/app/Layout.alder:23`
- Modify: `examples/todos/static/app.css:15` (drop `margin: 1.5rem 0 0.5rem` from the `nav` rule; drop the rule if it is left empty), `examples/blog/static/app.css:58` (drop `.account { margin-top: 1.5rem }`), per D7
- Test: `cmd/alder/new_test.go` (~348), `internal/generate/admin_test.go`, `cmd/alder/example_test.go`

**Interfaces:**
- Consumes: `al-stack gap="lg"` (already in the manifest).
- Produces: every Layout Alder writes or ships renders `<main><al-stack gap="lg">…children…</al-stack></main>`.

- [ ] **Step 1: Write the failing tests**

- `new_test.go` ~348: add `<main><al-stack gap="lg">` to the `want` list of the rendered scaffold page.
- `admin_test.go`, `TestAdminLayoutPageStack`: generate admin on a project with no app Layout. The `Admin.Layout` source contains `<main><al-stack gap="lg">{props.children}</al-stack></main>`, and the nav sits before `<main>`. On a project with a Layout, the source contains `{props.children}` directly inside `<Layout.Page …>` and no `al-stack gap="lg"`.
- `example_test.go`, `TestExample_LayoutPageStack`: modelled on `TestExample_LayoutThemeScript`. For each of blog, todos and workspace, the Layout source contains `<al-stack gap="lg">{props.children}</al-stack>` inside `<main>`. The blog and workspace `<header>` still sits outside `<main>`.

- [ ] **Step 2: Run them to see them fail**

Run: `go test ./internal/generate -run TestAdminLayoutPageStack && go test ./cmd/alder -run 'TestNew|TestExample_LayoutPageStack' -short`
Expected: FAIL on the missing `al-stack`.

- [ ] **Step 3: Change the four Layouts and the generator**

Use the exact markup in Global Constraints. In `admin.go`, change only the string `"        <main>{props.children}</main>\n"`. Then run `go run ./cmd/alder fmt` on each changed `.alder` file (scaffold template excepted if fmt cannot read it standalone) and keep its output.

- [ ] **Step 4: Example CSS per D7**

Drop the two margins. Leave list-item margins, `margin: 0` resets and the `form:not(._alder-click)` rule.

- [ ] **Step 5: Run the tests to see them pass**

Run: same as Step 2, then `go test ./internal/generate ./internal/scaffold/... ./cmd/alder -short`.
Expected: PASS. Any golden that embeds the no-Layout admin frame is updated, and its diff is only the stack line.

- [ ] **Step 6: Commit**

```bash
git add internal/scaffold internal/generate cmd/alder examples
git commit -m "feat: Layouts own the page stack (al-stack gap=lg) (alder#39)"
```

### Task 6: Appendix C: the spacing rule — Sonnet

**Files:**
- Modify: `book/C-design-system.md`: a new `### Spacing` under "## The base layer and typography" (after line ~179, before "### Browser support"); the `al-button` section (~214) and the `al-stack` section (~616); the migration section (~1083)

**Interfaces:**
- Consumes: `full`, `xl` and the `dl` look (Part A); the Layout markup (Task 5).

- [ ] **Step 1: Write the section**

`### Spacing` opens with the README's Spacing text from spec §4.4, verbatim. Then the Layout snippet as an `alder` code block, without `eval=`: the scaffold's `<body>` from Task 5. Then one sentence that a page needing tighter groups adds an inner `<al-stack gap="md">`, as the admin dashboard does. Then one sentence that `dl` reads as a detail list (muted labels beside values from 40rem), which is how the generated Show and Delete pages render.

- [ ] **Step 2: Element sections**

- `al-button`: one sentence on natural width, plus an `eval=` example of `<al-button full><button type="submit">Save</button></al-button>` inside `<form style="display:grid">`.
- `al-stack`: the `gap` row already gained `xl` in Task 4; add "`xl` is 2.5rem, for space between page sections."

- [ ] **Step 3: Upgrade note (D10)**

Under "## Migrating from v0.3.x", add `### Upgrading to v0.4.2`. It says that an app made by an older `alder new` keeps its own Layout, so wrap `{props.children}` in `<al-stack gap="lg">` inside `<main>` (show the line), and add `full` to any button that relied on stretching.

- [ ] **Step 4: Book tests**

Run: `go test ./book/ -short`
Expected: `ok` (the `eval=` blocks render; the manifest drift check passes).

- [ ] **Step 5: Commit**

```bash
git add book/C-design-system.md
git commit -m "docs(book): Appendix C spacing rule, full, dl, v0.4.2 upgrade"
```

### Task 7: Examples visual pass, full suite, PR — Sonnet (controller reviews the shots)

**Files:**
- Modify (only if the shots show a regression): `examples/*/static/app.css`
- Create: screenshots in the session scratchpad (not committed)

**Interfaces:**
- Consumes: Tasks 4–6.
- Produces: alder PR `alder-ui-042-21` → `main`. Its merge hash is Part C's `ALDER_REF`.

- [ ] **Step 1: Full suite**

Run: `scripts/test.sh` (foreground, 15-minute timeout).
Expected: pass, apart from the known red tests filed in alder#41 (experiments tests, `TestSpeed_Examples`). Name any other failure in the report.

- [ ] **Step 2: Shots**

For each of `examples/{blog,todos,workspace}`: `go run ./cmd/alder build`, then `run` (a different port per example, in the background under a timeout). Screenshot with Playwright at 1280×800, light and dark (`colorScheme: 'dark'`): the index, a form page, a Show page and the Delete page of the generated admin, and the admin dashboard.
Expected: an `h1` is 24px above the first card, cards are 24px apart, submit buttons sit at the start at natural width, and Show/Delete fields are two columns of muted labels beside values. Stop the servers.

- [ ] **Step 3: Fix and re-shoot**

Fix only regressions the stack exposes, such as a doubled gap from an app rule. Re-run `go test ./cmd/alder -run Example -short` after any CSS change.

- [ ] **Step 4: Push and open the PR**

```bash
git push -u origin alder-ui-042-21
gh pr create --base main --head alder-ui-042-21 --title "alder-ui v0.4.2: Layouts own the page stack (#39)" --body "<summary of D1–D3, D7–D10; before/after shots; Closes #39; Refs #21 (the user's verdict on nightvote-replay closes it)>"
```
Expected: CI runs. A red that is not in #41 is fixed before the merge.

**Controller, after Task 7:** an Opus whole-branch review of `main..alder-ui-042-21`, with a fix wave if needed. Then the user merges, or tells the controller to. After the merge:

```bash
git switch main && git pull && go install ./cmd/alder && ~/go/bin/alder version
git branch -d alder-ui-042-21 && git push origin --delete alder-ui-042-21
```
Record the full merge hash in the ledger and tell the user to run `:LspRestart`.

---

## Part C: nightvote-replay (repo `~/projects/nightvote-replay`, new branch `alder-ui-042` from `main` a5d43a0)

### Task 8: Page stack, re-pin, after-shots — Sonnet (controller relays the shots for the verdict)

**Files:**
- Modify: `app/Layout.alder` (`Document`, line 21, which also frames `Page` and `Live`; the `test` block at the end)
- Modify: `Dockerfile:8` (`ARG ALDER_REF=`)
- Modify: `docs/alder-dev-log.md` (one dated entry)
- Create: `.shots/after042-{index,replay}-{light,dark}.png` (git-ignored)

**Interfaces:**
- Consumes: the alder merge hash (Task 7), the reinstalled `~/go/bin/alder`, and `scripts/browser/shots.mjs <base-url> <key> <outdir> <label>`.

- [ ] **Step 1: Branch and check the binary**

Run: `git -C ~/projects/nightvote-replay switch -c alder-ui-042 main && ~/go/bin/alder version`
Expected: the version names the Task 7 merge hash. Leave the untracked `ALDER_USAGE_NOTES.md` and `scratch.alder` untouched.

- [ ] **Step 2: Write the failing test**

In the existing `test "Live refreshes, Page does not"` block, add `&& Test.has "main > al-stack[gap=lg]" still && Test.has "main > al-stack[gap=lg]" live`. If `Test.has` does not support `>`, use `"al-stack[gap=lg]"` and note it in the dev log.
Run: `~/go/bin/alder test . -run 'Live refreshes'`
Expected: FAIL.

- [ ] **Step 3: Wrap the children**

In `Document`'s body: `<al-page width="narrow"><main><al-stack gap="lg">{props.children}</al-stack></main></al-page>`. Run `~/go/bin/alder fmt app/Layout.alder`.
Run: `~/go/bin/alder test . -run 'Live refreshes'`
Expected: PASS.

- [ ] **Step 4: Re-pin**

`Dockerfile:8`: `ARG ALDER_REF=<Task 7 full merge hash>`. Confirm with `git -C ~/projects/alder branch -r --contains <hash>`, which must list `origin/main`.

- [ ] **Step 5: After-shots**

Run the app in the background under a timeout:

```bash
REPLAY_KEY=$(cat ~/.config/nightvote-replay/replay_key) ALDER_SECRET=$(openssl rand -hex 16) PUBLIC_BASE=http://localhost:8080 ALDER_DB=$(mktemp -d)/db ALDER_ADDR=:8080 ~/go/bin/alder run .
```

Then:

```bash
cd scripts/browser && node shots.mjs http://localhost:8080 "$(cat ~/.config/nightvote-replay/replay_key)" ../../.shots after042
```

Expected: four PNGs. Compared with `.shots/after-*` (v0.4.1), cards, the `h1` and tables no longer touch. Stop the server. If a rule in `static/app.css` now doubles a gap, drop it and re-shoot. Keep the form grid rule.

- [ ] **Step 6: Full gate**

Run: `scripts/test.sh`
Expected: check clean, all tests pass (294 at a5d43a0, plus or minus the edited test), build ok, adapter tests ok, mutation ≥ 0.9.

- [ ] **Step 7: Dev log, commit, push the branch**

Add a dated entry to `docs/alder-dev-log.md`: the page stack moved into the Layout, what the shots showed, and any `Test.has` selector limit.

```bash
git add app/Layout.alder Dockerfile docs/alder-dev-log.md static/app.css
git commit -m "style: alder-ui v0.4.2 page stack; pin alder <short hash>"
git push -u origin alder-ui-042
```

Do not merge or push `main`.

- [ ] **Step 8: Verdict (controller)**

Send the user the four after042 shots beside the v0.4.1 after-shots. Ask two things: does the spacing close #21, and may `alder-ui-042` fast-forward `main` and push it, which redeploys Render. On yes: close alder#21 with a comment, delete the Plan 2 SDD ledger (`alder/.superpowers/sdd/2026-10-25-alder-ui-shadcn-plan2/`), then merge and push nvr and run the deployed checks from the election-replay memory (fetch-night, ux-smoke).
