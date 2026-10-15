# M7b: The interactive tier, dev-mode tools, the admin's upgrades — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `alder-ui` v0.2.0 adds dialog, popover, menu, tabs, toast and tooltip on platform primitives; the dev server gets a Django-Debug-Toolbar-style panel and a `/_dev` map; the admin gains a delete dialog, bulk actions and autocomplete; messages can carry a notice; forms and navigation show in-flight state.

**Architecture:** (1) The library (separate repo): six elements, `al.js`, the dev panel element, demos, checker, tag; vendored. (2) Runtime: the trace recorder, the dev panel patch, `/_dev`, `Noticed`, in-flight state, the toast region, `al.js` served. (3) Generators + examples + book: the admin's dialog, bulk actions, autocomplete; chapter 11 "Developer mode", Appendix C, chapters 5/6/13/15; the smoke suite.

**Tech Stack:** Go; CSS + small ES modules; invoker commands, Popover API, CSS anchor positioning, `<details name>`; Datastar; Playwright for the smoke suite.

**Spec:** `docs/superpowers/specs/2026-10-14-m7b-interactive-devtools-design.md` (binding: the element list and attributes, no-JS behaviour, the dev panel's tabs and dev-only rule, the admin behaviours, the `Noticed` constructor).

## Global Constraints

- Light DOM, tokens only, each script ≤ 60 lines and optional where the platform provides the behaviour; the library's checker enforces script size and the no-JS demo form; v0.2.0 tagged with CI green; vendored by `scripts/vendor-al.sh`.
- Nothing dev-mode reaches a production build: a test builds an example without `ALDER_DEV` and asserts no `al-devpanel`, no `/_dev` route, no trace recording in the binary's HTML.
- The examples' admins are regenerated and replay byte-for-byte; the mutation gates stay ≥ 0.95; `go test -tags browser ./cmd/alder` covers every interactive element and the panel.
- Gate: `gofmt -l $(git ls-files '*.go')` empty; `go vet ./...` (+ `-tags browser`); `go test ./... -count=1`; `go test -tags browser ./cmd/alder`; both examples check/build/test + `--mutate`; `alder new` out of tree; experiments unittest (reference cap raised to 46k with the tables); `go test ./book -count=1`.
- Commit messages end with the session trailers; stage by explicit path.

## Review Focus

1. **A dialog open during a Datastar morph** (the user opened Delete, a background rerender arrives): expected — the dialog stays open with focus inside; closing returns focus to its invoker; a morph that removes the dialog's element closes it cleanly. Task 2 (smoke).
2. **The dev panel on a request that 500s, redirects externally, or answers 404**: expected — the panel shows the answer and the trace up to the failure, with the Go error in dev; never breaks the error page; never appears without `ALDER_DEV`. Task 2.
3. **Autocomplete membership**: a crafted POST with an id not in the narrowed options → the field error; a valid narrowed option → accepted; the search message under a Reader on an admin page → 404. Task 3.
4. **Bulk delete over a page of rows where some are restricted**: expected — the preview lists the refused rows, the message deletes only the allowed ones, answers a notice with the counts, and the mutation gate stays ≥ 0.95. Task 3.
5. **In-flight state and a slow server**: a double submit is impossible (`inert`), the pending bar shows, the answer restores the form; Back during a pending GET aborts it (the shared controller). Task 2 (smoke with a delayed route).

---

### Task 1: alder-ui v0.2.0 (the library)

**Files (repo `tapegram/alder-ui`, scratch clone):** `components/al-{dialog,popover,menu,tabs,toast,tooltip,devpanel}.css` (+ `.js` for menu, tabs, toast, tooltip, the shim `al-shim.js` for invoker commands/popover/anchor fallbacks), `al.js` (bundle; `scripts/build.py` extended), `custom-elements.json` (entries, events), `demo/*.html` (every state, the no-JS form first), `test/check.py` (scripts ≤ 60 lines; manifest↔scripts; no-JS demo present; events documented), README, CI; tag v0.2.0. Then in Alder: `scripts/vendor-al.sh v0.2.0`, `rt/al/al.js` served at `/static/al/al.js` (module; cached like al.css), the placeholder manifest test updated.
- [ ] implement → check.py green → CI green → tag → vendor → commit.

### Task 2: Runtime — trace, dev panel, `/_dev`, `Noticed`, in-flight state, smoke suite

**Files:** `rt/eff` (a per-request `Trace`: steps with ms; capability calls marked adapter/fake), `rt/db` (SQL + args + ms + rows into the trace when dev), `rt/web` (`Noticed Text Nav` rendering: the toast region patch; the dev panel patch and inline rendering under `Options.Dev`; `/_dev` page from the app's IR summary the binary embeds (`internal/gen` emits a compact JSON of pages/messages/tables/capabilities); `ALDER_DEV_PANEL=0`; the in-flight `inert`/`aria-busy` attributes on the generated form/click markup and the pending bar via Datastar's indicator; `al.js` tag in `HeadTags`), `rt/datastar` (the indicator signal; the panel patch helper), `internal/ir`+`check` (`Noticed` as a `Nav` constructor; `page.notices` in browser tests), `rt/testing` (notices in `Doc`/answers), `cmd/alder/testdata/browser/smoke.cjs` + `browser_test.go` (RF1, RF2, RF5, every element, the panel), the production-build test (no dev code), chapters 6 (notices, in-flight, the toast region), 11 ("Developer mode"), 13 (the trace, the panel patch, `/_dev`), 10 (`page.notices`).
- [ ] RED → implement → GREEN → commits (trace + panel; `/_dev`; Noticed + in-flight; book).

### Task 3: The admin's upgrades, examples, Appendix C, reference

**Files:** `internal/generate/admin.go` (delete via `al-dialog` with the confirmation page kept; bulk actions: checkbox column, select-all, `BulkDelete`/`BulkSet<Col>` messages with the preview and a `Noticed` answer; autocomplete `al-autocomplete` + `Search<Field>` message for references over 200 rows at generation time? — RULING: at load time: the generator emits both the select and the autocomplete branch on a count, as M13's dropped design wanted, now sound because membership is against the rendered options; regions `bulk`, `search`), goldens, `TestAdmin_Mutate`/`TestAdmin_Columns` (≥ 0.95; RF3, RF4), both examples regenerated (replay byte-for-byte) and their READMEs, the scaffold's "Signed" toast, `book/C-design-system.md` (the six elements + `al-devpanel` + `al-autocomplete` tables, drift-tested; invoker commands; no-JS per element; browser support), chapter 15 (the admin's new pieces), chapter 5 (`al-autocomplete`'s rule = `<Select options>`), the reference (cap 46k; the new elements' lines), `experiments/tasks/README.md` untouched.
- [ ] RED → implement → GREEN → commits (admin; examples; book + reference).

---

## Self-review notes

- Spec coverage: Part 1 ↔ T1, Part 2 ↔ T2 (+ `Noticed`, in-flight from Part 3), Part 3's admin ↔ T3; non-goals untouched.
- Interfaces: T2 consumes T1's `al.js` and elements; T3 consumes T1's elements and T2's `Noticed`/in-flight; T1 can run in parallel with T2's trace work (different repos/files).
- Review Focus 1–5 each have a test.
- Budget note: run T1 and T2 in parallel (separate repository / worktree), T3 after; reviews on Sonnet except the final review (Opus).
