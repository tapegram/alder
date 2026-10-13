# M7a: Custom elements and the `al` design system — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The checker validates custom elements, attributes and slots from a Custom Elements Manifest; Alder ships the `al` design system's first (CSS-only) tier as a vendored snapshot of its own repository; `alder new` and both examples use it; a book chapter documents it and is the library's documentation.

**Architecture:** (1) Checker: manifest loading (embedded + `elements/custom-elements.json`), E0028, the wrapped inputs; renderer goldens. (2) The `al` library repository: tokens, twelve components, manifest, demos, tests, CI, a `v0.1.0` tag. (3) Vendoring into Alder (`rt/al/`), build copies the assets, `alder new` + examples adopt it, the chapter, the reference.

**Tech Stack:** Go; CSS; the Custom Elements Manifest schema (JSON); GitHub Actions in the library repo.

**Spec:** `docs/superpowers/specs/2026-10-12-m7a-elements-design.md` (binding: the checker rules and texts, the element list and attributes, the vendoring mechanism, light DOM/tokens-only).

## Global Constraints

- Custom elements are checked ONLY through manifests (the embedded `al` one and the app's file); the 66-tag HTML list is unchanged; a hyphenated tag with no manifest entry is E0028 with the spec's text; the escape list for attributes is exactly the spec's.
- The library is light DOM, tokens only (a test fails on a hard-coded colour in a component), usable without JavaScript; every element and attribute in the CSS is in the manifest and vice versa (a test).
- Vendoring is a plain copy at a tag with `VERSION` recorded; no submodule, no fetch at build time; `rt/al` is embedded and written to `build/<app>/static/al/` by every build.
- Both examples and the scaffold adopt the system; their browser tests still pass; mutation scores unchanged (todos 1.00, blog 0.95, scaffold 1.00); migrations untouched.
- Outward actions (Task 2 only): create `github.com/tapegram/al` PRIVATE, push, tag `v0.1.0` there (the library's own tag; Alder's release tags remain the user's). No Alder tag.
- Gate: `gofmt -l $(git ls-files '*.go')` empty; `go vet ./...`; `go test ./... -count=1`; both examples check/build/test + `--mutate`; `alder new` out of tree; `(cd experiments && python3 -m unittest)`; `go test ./book -count=1`.
- Commit messages end with the session trailers; stage by explicit path.

## Review Focus

1. **A third-party manifest that overlaps the built-in one** (an `al-button` entry in the app's file with different attributes): expected — the app's wins, documented; a test. Task 1.
2. **A wrapped input with a validation error**: `<al-input field="title" />` renders the element with `invalid` set and the errors list inside, and the browser tests' selectors (`ul.errors`, `data-errors`) still find them; `Test.submitRaw` → `FormRejected` unchanged. Task 1 (golden) + Task 3 (examples' tests).
3. **A page that uses `al-*` without linking the stylesheet** (the user removed the `<link>`): expected — checks and renders unstyled; no error; chapter says so. Task 3.
4. **Manifest drift between the vendored snapshot and the chapter**: expected — the book test compares the chapter's attribute tables with `rt/al/custom-elements.json` and fails on any difference; and `VERSION` ↔ manifest version. Task 3.
5. **The Datastar morph with custom elements**: a `Rerender` of a page whose body contains `al-*` elements replaces the body as today; a browser test on the blog's rerendering page passes; the dev server's reload works. Task 3.

---

### Task 1: The checker learns manifests; the wrapped inputs

**Files:** `internal/elements/` (new: `Manifest` type for the subset of the Custom Elements Manifest schema used — `modules[].declarations[]` with `tagName`, `attributes[].{name,type.text,default,description}`, `slots[].name`, `cssParts` ignored; `Load(fs, path)`, `Merge(builtin, app)`; the escape-list predicate), `internal/check/html.go` (hyphenated tags → manifest lookup; E0028 texts verbatim; attribute/slot checks; boolean attributes; the wrapped inputs: `al-input`/`al-textarea`/`al-select`/`al-checkbox` routed through `formComponent` with the element markup), `internal/diag` (E0028 + explain), `internal/gen`/`rt/html` (render the wrapped inputs: the element around the input + the errors list; set `invalid` when errors exist; goldens), a placeholder `rt/al/custom-elements.json` with the twelve elements' entries as the spec lists them (Task 2 produces the real one; Task 3 replaces this with the vendored copy — the entries must match by construction: Task 2 copies THIS file into the library as its manifest's starting point) + an empty `rt/al/al.css` + `VERSION` `v0.0.0-placeholder`, embedded; corpus: `elements-ok` (every element, attributes, slots, the wrapped inputs, a third-party manifest at the fixed path), `E0028-unknown-element` (with and without the file), `E0028-unknown-attribute`, `E0028-unknown-slot`, `E0028-bad-manifest`, `elements-app-wins`; book: chapter 5 "Custom elements" (texts verified), chapter 11 (`elements/`), chapter 13 (the manifest in the checker).
- [ ] RED → implement → GREEN → commits (elements+check+diag+corpus; gen+rt+goldens; book).

### Task 2: The `al` library repository

**Files (in a new repository `tapegram/al`, created private by `gh repo create`, worked on in a scratch clone, pushed, tagged `v0.1.0`):** `tokens.css` (shadcn's variable names and the default light/dark values), `components/al-<name>.css` for the twelve elements (+ optional `al-select.js`, `al-alert.js` enhancements, each a few lines, `type="module"`, no framework), `al.css` (the committed bundle; `scripts/build.py` concatenates tokens + components in order), `custom-elements.json` (starting from Alder's placeholder manifest; descriptions and defaults filled; `version` field = the tag), `demo/<name>.html` per component (every variant and state; plain HTML; links `../al.css`), `demo/index.html`, `test/check.py` (manifest ↔ CSS consistency; no hard-coded colours in components; demo pages use only declared attributes/slots; the bundle is up to date), `.github/workflows/ci.yml` (runs the tests), `README.md` (what it is, light DOM + tokens, how to use from any stack: link `al.css`, use the elements; the Alder chapter as the docs), `LICENSE` (Apache-2.0, "Copyright 2026 Tavish Pegram"). Accessibility: labels tied to inputs, visible focus rings, AA contrast for every variant in both themes (a contrast check in `test/check.py` from the token values).
- [ ] write tokens → components → manifest → demos → tests green → CI green on push → tag `v0.1.0`.

### Task 3: Vendoring, the build, the scaffold, the examples, the chapter

**Files:** `scripts/vendor-al.sh <tag>` (copies `al.css`, `custom-elements.json`, writes `VERSION`), `rt/al/` refreshed from `v0.1.0` (replacing the placeholder), the vendoring test (VERSION ↔ manifest version; the embed completeness test covers `rt/al`), `internal/buildkit` (write `static/al/{al.css,custom-elements.json}` on every build; `alder run` serves them; the immutable header), `internal/scaffold` (the guestbook uses `al-page`, `al-card`, `al-input`, `al-button`; `Layout` links `/static/al/al.css`), `examples/todos` and `examples/blog` (adopt the system per the spec; adjust browser tests where the markup legitimately changed; scores unchanged), chapter 2 re-synced, the new chapter "The al design system" (tokens/themes; every element with attribute tables generated-or-checked against the vendored manifest; verified blocks per element; overriding tokens; another stack; the version), chapter 11 (`static/al/`, `scripts/vendor-al.sh`), chapter 12 (why light DOM/tokens/manifest), chapter 13 (the copy), README, `experiments/build_context.py` (the reference keeps E0028's rules and the element table; report `--reference --size` — stay ≤ 44k by trimming elsewhere if needed; a unittest), the site builder picks up the new chapter.
- [ ] vendor → build copies → scaffold → examples → chapter → full gate → commits.

---

## Self-review notes

- Spec coverage: checker (T1), library (T2), vendoring/build/scaffold/examples/book (T3); non-goals untouched.
- Interfaces: T1's placeholder manifest seeds T2's; T3 replaces it with the tagged copy (same entries, so T1's corpus keeps passing).
- Review Focus 1–5 each have a test.
- Outward actions are listed in the Global Constraints: one new private repository and its tag.
