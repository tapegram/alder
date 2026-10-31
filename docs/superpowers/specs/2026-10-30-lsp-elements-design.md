# LSP: completion and hover for `al-*` and the app's custom elements

Design for [tapegram/alder#121](https://github.com/tapegram/alder/issues/121). Everything marked
**(call)** is the author's decision. Related: #119 (stdlib hover docs, the same hover surface),
#38 phase 3 (`cssProperties` from the same manifest), the alder-ui spec
(`2026-10-24-alder-ui-shadcn-design.md`).

## Intent

Inside Html in an Alder file the LSP completes and explains the built-in HTML tags and attributes
(`internal/markup.HTML`, `complete.go` `tags()`/`attrs()`, `hover.go` `markupHover`) but knows
nothing of alder-ui's 22 elements or of the app's own custom elements, although the checker
validates both from the merged Custom Elements Manifest (`internal/check/elements.go`,
`elements.Merge(elements.Builtin(), app)`). After this change the same merged manifest drives
completion (tag names, attributes, enum values, slot names) and hover (element, attribute),
with the manifest's own descriptions, so a person or a model writing `<al-` sees what the
checker will accept. Diagnostics stay the checker's. The issue's stretch goal 4 (built-in HTML)
already exists; this spec leaves it alone.

## Goals

1. After `<`, the merged manifest's elements complete beside the HTML tags, each with its
   description as documentation; the app's own elements included, overriding a built-in of the
   same tag as `Merge` already does.
2. Inside a custom element's start tag, its attributes complete with type, default and
   description; a boolean attribute inserts bare, the others `name=""`; after `name="` an
   attribute whose type is a union of literals (`Attr.Values()`) completes those values.
3. `slot="` inside a custom element's child completes the parent's named slots.
4. Hover on a custom element's tag: its description, then its attributes (name, type, default)
   and slots. Hover on one of its attributes: name, type, default, description.
5. The manifest is read once per analysis from the checker's result, so the app's manifest is
   the one the last good build parsed; no second parse in the LSP.

## Non-goals

- Diagnostics (E0028 stays the checker's), the tree-sitter grammar (the Go LSP is span-based),
  CSS-property completion or swatches (#38 phase 3), snippets that insert children or slots.
- Completing attribute values whose type is `string` or `number`.

## Changes

- **`internal/elements`**: `Element` gains `Description string`; `Slots []string` becomes
  `Slots []Slot` with `Slot{Name, Description string}` (`""` stays the default slot) — the
  manifest carries both (`declarations[].description`, `slots[].description`) and the parser
  drops them today **(call)**. The checker's two uses of `Slots` (slot validation) read `.Name`.
  `Merge` is unchanged. A test holds every built-in element to a non-empty description.
- **`internal/check`**: `Result` gains `Elements *elements.Manifest`, set from `c.elements` where
  the result is built, so the LSP reads what the checker merged (nil only when the result is nil,
  after a parse failure; the LSP then falls back to `elements.Builtin()` so `al-*` still
  completes in a file that does not parse yet **(call)**).
- **`internal/lsp/complete.go`**:
  - `tags()`: after the HTML tags, the manifest's elements in tag order, `kindClass` **(call)**
    with the description as `documentation` (markdown) and the tag as `detail`.
  - `attrs()`: when `reAttr`'s tag names a manifest element, its `Attrs` (manifest order) replace
    the HTML attribute list: `detail` = type (and `= default` when there is one), documentation
    = description; `Boolean()` inserts the name, else `name=""` with the cursor between the quotes
    (as the HTML path does today). The global attributes the HTML path appends stay appended
    **(call)**, since `class`, `id`, `slot`, `click` and the Alder-specific ones apply to custom
    elements too; `slot` is listed only when the element is a child of a custom element with named
    slots.
  - a new matcher `reAttrValue` = `<([a-z][a-z0-9-]*)\s[^<>]*?\b([a-z-]+)="([^"]*)$`: for a
    manifest element's attribute with `Values()`, each value as an item replacing the typed
    prefix, `kindEnumMember`; for `slot`, the enclosing custom element's named slots (found by
    walking the open tags `openTags` already computes). Nothing for other attributes.
  - the manifest comes from the analysis (`a.res.Elements`, else `Builtin()`), through one helper
    `(cx *completion) elements() *elements.Manifest`.
- **`internal/lsp/hover.go`** `markupHover`: before the HTML lookup, a tag with a hyphen is looked
  up in the manifest (an app override wins, as merged): the hover is
  ```
  `<al-button>` — A button, styled by variant and size. …

  **Attributes**
  - `variant`: `"primary" | "secondary" | …` = `"primary"` — The button's look: …
  - `href`: `string` — …

  **Slots**: default — The button's label.
  ```
  (markdown; attributes in manifest order; the Slots line omitted when there are none). An
  attribute of a custom element hovers as `` `variant`: `"primary" | …` = `"primary"` — The
  button's look … ``. Components (`n.Ref.Kind == RefComponent`) are untouched.
- No change to the checker's diagnostics, the grammar, or `alder.nvim` (its completion comes
  from the LSP).

## Verification

- `internal/elements`: the parsed built-in manifest gives `al-button` its description and a
  default slot with a description; an app manifest element without descriptions parses with
  empty ones; the checker's slot tests still pass.
- `internal/lsp` (the existing completion test style, a temp app with the blog's `App.alder`):
  `<al` completes `al-button` with its description and `al-devpanel` too (everything in the
  manifest); `<al-button v` completes `variant` with `detail` `"primary" | … = "primary"` and
  inserts `variant=""`; `<al-button variant="` completes the six variants; `<al-button
  disabled` is a boolean attribute and inserts bare; `<al-input l` completes `label` and not
  `variant`; `<al-card><p slot="` completes `al-card`'s named slots and `<div><p slot="`
  completes nothing; an app `elements/custom-elements.json` adding `x-chart` completes `<x-ch`
  and overrides `al-badge`'s description when it declares that tag; a file that fails to parse
  still completes `<al-`.
- Hover: on `al-button` shows the description, attributes and slots; on its `variant` the type,
  default and description; on `<div>` and a component unchanged.
- `go test ./...` green; a manual pass in nvim: `<al-` in the blog's `Posts.Index`.
