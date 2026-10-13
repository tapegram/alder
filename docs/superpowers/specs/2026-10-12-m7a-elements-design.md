# M7a: Custom elements and the `al` design system (first tier) — Design

Date: 2026-10-12. Status: approved in conversation; the M7a plan argues from
this. Related: the M3 spec's M7 paragraph (Web Awesome, the manifest
direction), memory note "shadcn web components idea", M12 (generators and
the admin, which emit these components).

## Goal

Alder's checker learns custom elements from a Custom Elements Manifest, so
a web-component design system is checked like built-in HTML; and Alder
ships its own light-DOM design system, `al` (elements `<al-button>` …), as
a separate open-source library vendored into the compiler, so a fresh app
looks finished with zero setup and the generated admin (M12) has something
to be built from.

## Decisions recorded (2026-10-12)

- **Manifest at a fixed path, and the built-in system needs no file** (C):
  `elements/custom-elements.json` declares third-party custom elements;
  Alder's own system is embedded (manifest + CSS) and always available;
  both go through the same manifest format in the checker.
- **Linked assets** (A): `alder build` writes the system's CSS and manifest
  to `build/<app>/static/al/`; `Layout` links `/static/al/al.css`; an app
  that does not link it renders the elements unstyled but they still check.
- **A separate repository, vendored at a tag; the Alder book chapter is
  the library's documentation** (C). Prefix `al-`; library and repository
  name `al` (`github.com/tapegram/al`, private at first, Apache-2.0).
- The first tier is CSS-only: `al-button`, `al-input`, `al-textarea`,
  `al-select`, `al-checkbox`, `al-table`, `al-card`, `al-badge`,
  `al-alert`, `al-pagination`, `al-stack`, `al-page`. Light DOM, shadcn's
  token sheet verbatim, light and dark themes, no JavaScript required
  (a few elements register a class for progressive enhancement only).

## The checker

- A tag containing `-` is a custom element. The checker looks it up in the
  union of the embedded `al` manifest and the app's
  `elements/custom-elements.json` (if present). Unknown → E0028 "unknown
  element `<wa-button>`: declare custom elements in
  `elements/custom-elements.json` (chapter 5)". A manifest that does not
  parse → E0028 "`elements/custom-elements.json`: <reason>". The same tag
  in both → the app's manifest wins (documented).
- Attributes: a custom element's attributes must be declared in its
  manifest entry (`attributes[].name`), except `id`, `class`, `style`,
  `slot`, `hidden`, `title`, `lang`, `dir`, `tabindex`, `role`, `data-*`,
  `aria-*`, and Alder's own message/form attributes (`click`, `submit`,
  `href`, `field` where the element wraps an input — see below). Unknown →
  E0028 "`<al-button>` has no attribute `size`; it has `variant`, `type`,
  `disabled`". A boolean attribute (manifest `type.text` of `boolean`)
  takes no value or `{bool}`; others take text or `{expr}` as today.
- Slots: `slot="x"` on a child must name a slot the parent declares
  (manifest `slots[].name`); unknown → E0028 "`<al-card>` has no slot
  `footer`; it has `header`, `actions`".
- Events are not modelled (Alder's `click`/`submit` are message
  attributes, not DOM events).
- `al-input`, `al-textarea`, `al-select`, `al-checkbox` wrap Alder's form
  machinery: `<al-input field="title" label="Title" />` is the form
  component `<Input>` with the element's markup; the checker treats
  `field`/`label`/`value` exactly as for `<Input>` (same errors), and the
  renderer emits the element around the input plus the `<al-errors>`
  list. Only the embedded system's input elements get this; a third-party
  input is an ordinary custom element.
- Rendering: custom elements render as written (light DOM); the Datastar
  morph is unaffected; attribute escaping as for any element.

## The `al` library (repository `tapegram/al`)

- Layout: `tokens.css` (shadcn's variables, light + dark via
  `prefers-color-scheme` and `[data-theme]`), `components/<name>.css` (+
  `<name>.js` only where enhancement is needed; none in the first tier
  except `al-select`'s keyboard focus ring and `al-alert`'s dismiss, which
  are optional), `al.css` (the bundle: tokens + every component, built by a
  script, committed), `custom-elements.json` (the manifest: every element,
  attribute with type/default/description, slot, CSS part), `demo/<name>.html`
  (one page per component showing every variant, used by the book),
  `test/` (a Node-free test: a Go or Python script checks that every
  selector in the CSS names a declared element/attribute/slot and every
  manifest entry has CSS; a snapshot of `al.css` against the components;
  the demo pages parse and use only declared attributes), `README.md`,
  `LICENSE` (Apache-2.0), a GitHub Actions workflow running the tests, a
  tag per release (`v0.1.0`).
- Design rules (from the brainstorm): light DOM (no shadow root), tokens
  only (no hard-coded colours), every element usable server-rendered with
  no script, attributes are the API (no properties), `::part` not needed,
  class names prefixed `al-`, accessible by default (labels, roles,
  focus rings, contrast at AA).
- Elements and their attributes (first tier):
  - `al-button`: `variant` (`primary|secondary|outline|ghost|destructive`,
    default `primary`), `size` (`sm|md|lg`), `disabled`, `type`
    (`button|submit`); slot default. Works around a native `<button>`
    inside or as the element itself.
  - `al-input` / `al-textarea`: `label`, `placeholder`, `disabled`,
    `invalid` (set by the renderer when the field has errors), `help`;
    wraps `<input>`/`<textarea>`; the errors list renders inside.
  - `al-select`: `label`, `disabled`, `invalid`; wraps `<select>`.
  - `al-checkbox`: `label`, `disabled`; wraps `<input type=checkbox>`.
  - `al-table`: `striped`, `dense`; styles a child `<table>`.
  - `al-card`: slots `header`, default, `footer`, `actions`.
  - `al-badge`: `variant` (`default|secondary|outline|destructive`).
  - `al-alert`: `variant` (`info|success|warning|destructive`), `title`,
    `dismissible`.
  - `al-pagination`: `page`, `pages`, slots `prev`, `next` (the app renders
    the links; the element lays them out).
  - `al-stack`: `gap` (`sm|md|lg`), `direction` (`row|column`).
  - `al-page`: `width` (`narrow|normal|wide`); the content frame.

## Vendoring into Alder

- `rt/al/` holds a tagged snapshot: `al.css`, `custom-elements.json`,
  `VERSION` (the tag), embedded with the runtime (`rt/embed.go`
  patterns); a script `scripts/vendor-al.sh <tag>` refreshes it from the
  library's repository (a plain copy; no submodule, no build-time fetch);
  a test checks `VERSION` matches the manifest's `version`.
- `alder build` (and scratch builds) write `static/al/al.css` and
  `static/al/custom-elements.json` next to the app's `static/` copy;
  `alder run` serves them; the immutable cache header applies.
- `alder new` links the stylesheet in `Layout` and uses `al-page`,
  `al-card`, `al-input`, `al-button` in the guestbook; both examples adopt
  the system (todos: the list as `al-table`? — keep `<ul>` with
  `al-card`/`al-button`/`al-input`/`al-badge`; blog: `al-page`, `al-card`,
  `al-pagination`, `al-alert` for the form-level error).

## Book

- Chapter 5 (HTML) gains "Custom elements": the manifest, E0028, the
  escape list, slots, the built-in system and how to add Web Awesome
  (its manifest at the fixed path, its assets in `static/`).
- A new chapter "The al design system" (after chapter 6 or as 6b):
  tokens and themes, every element with its attributes and a verified
  block per element quoting the library's demo (the book's checker renders
  each demo block through `alder check` on a module that uses it, and a
  test compares the chapter's attribute tables with the vendored manifest
  so they cannot drift), overriding tokens, using it from another stack
  (one paragraph), the version vendored. This chapter is the library's
  documentation; the library's README points at it.
- Chapter 11: `elements/`, `static/al/`, `alder build`'s copy, the
  vendoring script. Chapter 13: the manifest in the checker; the input
  wrapping. Chapter 12: why light DOM, why tokens, why a manifest. README.
- The reference keeps the rules (E0028, the escape list) and the element
  table.

## Testing

- Checker: corpus cases for E0028 (unknown element with/without a
  manifest; unknown attribute; unknown slot; a bad manifest file; the
  app-wins override; boolean attributes); the wrapped inputs' errors.
- Renderer: goldens for each element incl. the wrapped input with errors.
- The vendoring test (VERSION ↔ manifest); the build copies the assets;
  `alder new`'s scaffold links and uses them; the examples' browser tests
  keep passing (the selectors the tests use must survive the markup
  change — adjust tests where the markup legitimately changed).
- The library's own tests in its repository, run by its CI.

## Non-goals

- The interactive tier (dialog, popover, menu, tabs, toast, tooltip):
  M7b. JavaScript-dependent behaviour; shadow DOM; `::part`; events;
  a theme editor; Web Awesome's own manifest shipped by Alder (users add
  it); a real-browser visual test (the demo pages are the fixture; M7b's
  smoke suite can screenshot them).
