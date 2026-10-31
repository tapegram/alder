# LSP completion and hover for custom elements (#121) — implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** the LSP completes and hovers `al-*` and the app's own custom elements (tags, attributes, enum values, slot names) from the checker's merged Custom Elements Manifest.

**Architecture:** `internal/elements` keeps the descriptions the manifest already carries; `check.Result.Elements` exposes the merged manifest; `internal/lsp` reads it through one helper, falling back to `elements.Builtin()` when there is no result.

**Tech Stack:** Go, stdlib only.

**Spec:** `docs/superpowers/specs/2026-10-30-lsp-elements-design.md` (binding).

## Global Constraints

- No change to the checker's diagnostics, the grammar, or `alder.nvim`.
- Components (`n.Ref.Kind == syntax.RefComponent`) keep today's hover and completion.
- Built-in HTML completion and hover stay as they are for tags without a hyphen.
- Markdown hover format exactly as the spec's code block.

## Review Focus

1. **Two-line start tag.** `<al-button\n    variant="` completes the variants even though the tag is on the line above. The matchers run on `cx.prefix`; check it spans lines (Task 2 test `TestComplete_AttrValueAcrossLines`).
2. **Cursor after a closed tag.** `<al-button variant="ghost">x</al-button> <p v` must not offer `al-button`'s attributes for `p` (Task 2 test `TestComplete_AttrsOfInnermostTag`).
3. **Unknown hyphenated tag.** `<x-unknown v` completes the global attributes only, without a panic or `al-*` attributes (Task 2 test `TestComplete_UnknownCustomTag`).
4. **Nested slot lookup.** `<al-card><div><p slot="` completes nothing, because the slot belongs to the direct custom parent only (Task 2 test `TestComplete_SlotDirectParentOnly`).
5. **Stale app manifest.** The app's `elements/custom-elements.json` is broken after a good build. Completion still offers `al-*` (Builtin fallback) and no app elements, with no panic (Task 2 test `TestComplete_BrokenAppManifest`).

---

### Task 1: `elements` descriptions and `check.Result.Elements`

**Files:**
- Modify: `internal/elements/elements.go` (`Element.Description`; `Slots []Slot`; `Parse` keeps both descriptions; `HasSlot`/`NamedSlots` read `.Name`, `NamedSlots` still returns `[]string`), `internal/check/result.go` (`Elements *elements.Manifest`), `internal/check/check.go` (set it where the Result is finished, from `c.elements`)
- Test: `internal/elements/elements_test.go`, `internal/check/elements_test.go` (or the existing slot tests' file)

**Interfaces:**
- Produces:
  ```go
  type Slot struct{ Name, Description string } // Name "" = the default slot
  type Element struct { Tag, Description string; Attrs []Attr; Slots []Slot; Builtin bool }
  // check.Result gains:
  Elements *elements.Manifest // the merged manifest the checker used
  ```

- [ ] **Step 1: Failing tests.** `TestBuiltin_Descriptions`: every built-in element has a non-empty `Description`, and `al-button` has a default slot (`Name == ""`) with a non-empty description. `TestParse_NoDescriptions`: an app manifest without descriptions parses with empty ones. `TestResult_Elements`: checking the blog gives a non-nil `Result.Elements` with `al-button`.
- [ ] **Step 2: Run** `go test ./internal/elements/ ./internal/check/ -run 'Descriptions|NoDescriptions|Result_Elements'`. Expect FAIL.
- [ ] **Step 3: Implement.** Fix every compile error from the `Slots` type change across the repo (`go build ./...`).
- [ ] **Step 4: Run** `go test ./internal/elements/ ./internal/check/`. Expect PASS, slot corpus included.
- [ ] **Step 5: Commit** `elements: descriptions; check.Result.Elements (#121)`.

---

### Task 2: completion

**Files:**
- Modify: `internal/lsp/complete.go`
- Test: `internal/lsp/elements_complete_test.go` (new; uses `start`, `blogCopy`, `openClean`, `typing`, `complete`, `labels`)

**Interfaces:**
- Consumes: `check.Result.Elements`, `elements.Builtin()`, `(*elements.Attr).Values()`, `Boolean()`.
- Produces:
  ```go
  func (cx *completion) elements() *elements.Manifest // a.res.Elements, else elements.Builtin()
  var reAttrValue = regexp.MustCompile(`<([a-z][a-z0-9-]*)\s[^<>]*?\b([a-z-]+)="([^"]*)$`)
  func (cx *completion) attrValues() ([]completionItem, bool) // runs before attrs() in items()
  ```
  Items: tags `kindClass` (7), detail = tag, documentation = description as markdown (add a `Documentation *markupContent` field to `completionItem` if it lacks one). Attribute detail = type, plus ` = <default>` when there is a default. Enum value items use `kindEnumMember` (20; add the const if missing) and replace the typed value prefix.

- [ ] **Step 1: Failing tests.** One test per spec "Verification → internal/lsp" completion bullet, named `TestComplete_ElementTags`, `TestComplete_ElementAttrs`, `TestComplete_AttrEnumValues`, `TestComplete_BooleanAttrBare`, `TestComplete_InputAttrsNotButtons`, `TestComplete_SlotNames`, `TestComplete_AppManifestOverride`, `TestComplete_UnparsedFileStillAl`. Add the five Review Focus tests. Assertions use the spec's exact values: `variant` detail starts `"primary" |` and ends `= "primary"`, NewText `variant=""`, and the six variant labels.
- [ ] **Step 2: Run** `go test ./internal/lsp/ -run TestComplete_`. The new tests FAIL.
- [ ] **Step 3: Implement** as the spec's `complete.go` bullets.
- [ ] **Step 4: Run** `go test ./internal/lsp/`. Expect PASS.
- [ ] **Step 5: Commit** `lsp: complete custom elements, attributes, values and slots (#121)`.

---

### Task 3: hover

**Files:**
- Modify: `internal/lsp/hover.go` (`markupHover(path)` becomes `markupHover(m *elements.Manifest, path)`; the caller passes `a.res.Elements`, else `Builtin()`)
- Test: `internal/lsp/hover_test.go`

- [ ] **Step 1: Failing tests.** `TestHover_CustomElement`: hover on `al-button` in the blog is the spec's format, with the description first, then `**Attributes**` lines in manifest order, then a `**Slots**:` line. `TestHover_CustomAttr`: `variant` hovers as ``"`variant`: `<type>` = `\"primary\"` — <description>"``. `TestHover_DivAndComponentUnchanged`.
- [ ] **Step 2: Run** `go test ./internal/lsp/ -run TestHover_`. Expect FAIL.
- [ ] **Step 3: Implement.** Omit the Slots line when the element has none. Write the default slot as `default`.
- [ ] **Step 4: Run** `go test ./...`. Expect PASS.
- [ ] **Step 5: Commit** `lsp: hover custom elements and their attributes (#121)`.
