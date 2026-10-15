# M7b: The interactive tier, dev-mode tools, the admin's upgrades — Design

Date: 2026-10-14. Status: approved in conversation. Related: M7a (the
`alder-ui` first tier, vendored at v0.1.3), M12/M13 (the admin and the
option-load limit dropped for autocomplete), the user's request for
developer-mode tools (2026-10-14, "reminds me of the Django debugger"),
the ledger items: pending-navigation indicator, in-flight form state,
bulk actions, autocomplete with a conditional options check.

## Decisions recorded (2026-10-14)

- **Behaviour from platform primitives first** (A): `<dialog>` with
  invoker commands, the Popover API with CSS anchor positioning, `<details
  name>` groups, `popover="hint"` tooltips; script only where the platform
  has none (a toast timer, arrow keys in menus and tabs, an upgrade shim
  for older browsers). Every element is usable server-rendered with no
  script where the platform allows; the admin keeps its confirmation page
  as the no-JavaScript path.
- **A dev panel rendered by the dev server** (A), the Django Debug Toolbar
  shape: per request the message, the effect trace, the SQL with timings,
  the answer, the patches; a `/_dev` page; nothing in production builds.
- The admin's upgrades in full: delete dialog, in-flight form state, bulk
  actions, autocomplete for large reference tables.

## Part 1: the interactive tier (alder-ui v0.2.0)

Six elements, light DOM, tokens only, each with a demo and a manifest
entry, each tested by the library's checker and by Alder's browser smoke
suite:

- `al-dialog`: wraps a native `<dialog>`; attributes `heading`, `size`
  (`sm|md|lg`), `dismissible` (a close button; Escape and backdrop click
  via the platform); slots `header`, default, `actions`. Opened by any
  button with `commandfor="<id>" command="show-modal"` (invoker commands;
  the shim polyfills `command` on older browsers with four lines). A
  Datastar morph of the body keeps the dialog's open state when the
  element keeps its id and `open` attribute (the morph preserves
  attributes it does not rewrite; the server never writes `open`). The
  focus trap, inert background and return focus are the platform's.
- `al-popover`: a `popover` element anchored to its invoker (`anchor` by
  id; CSS anchor positioning with a top/bottom fallback via
  `position-try`); attributes `placement` (`top|bottom|left|right`),
  `offset`.
- `al-menu`: a popover holding a list of `al-button`-styled items (links
  or buttons); roving arrow keys and Home/End via a 20-line script;
  `role="menu"`/`menuitem`; closes on select; the invoker is `<al-button
  commandfor=… command="toggle-popover">`.
- `al-tabs`: `<al-tabs>` with `<al-tab name>` and `<al-panel for>`
  children; selection is a radio group (no script) with arrow keys added
  by script; `selected` attribute for the server to pick; a tab's panel
  can be a server-rendered page section (no lazy loading in this tier).
- `al-toast`: a `popover="manual"` region at a corner (`position`
  attribute); a toast is `<al-toast variant timeout>`; the script shows it
  on connect and closes it after `timeout` ms (default 5000; 0 = sticky);
  `role="status"`; Alder's runtime renders a toast for a message's
  `Notice "text"` answer (see Part 3) in a region outside the morph.
- `al-tooltip`: `popover="hint"` (where supported; a hover/focus-shown
  popover via the shim otherwise), anchored, `for` the trigger's id, text
  only, `aria-describedby` wired by the shim.
- The library: `components/<name>.css` + `<name>.js` where needed (each
  ≤ 60 lines, `type="module"`, no framework), `al.js` (the bundle of the
  scripts, built like `al.css`), `custom-elements.json` entries with
  events (`al-close`, `al-select`) documented, demos with every state,
  `test/check.py` extended (scripts referenced by the manifest exist; no
  script above 60 lines; the demo for each interactive element has the
  no-JS form), CI; tag v0.2.0; vendored (`al.css`, `al.js`,
  `custom-elements.json`, VERSION) and served under `/static/al/`
  (`al.js` as a module, same caching).
- The reference and Appendix C: the six elements' tables (drift-tested),
  invoker commands explained once, the no-JS behaviour per element, the
  browser support line (Chromium 135+, Safari 26, Firefox 140 for the full
  set; the shim covers the rest for dialog/popover/toast/tooltip).

## Part 2: dev-mode tools

- Under `ALDER_DEV=1` only (the dev server; never the production binary):
  - **The panel**: every document gets `<al-devpanel>` appended after the
    body's content, outside the morphed region (the dev server morphs the
    body as today and patches the panel separately, so it survives every
    navigation); collapsed to a one-line bar (the request's method, path,
    message, answer, total ms, SQL count); expanded: tabs for **Request**
    (the page or `Module/Msg` with its decoded parameters and form fields;
    the session decoded), **Trace** (the effect steps exactly as `alder
    test` prints them, with per-step ms; capability calls marked adapter
    or fake), **SQL** (each statement with its arguments, ms, rows; totals;
    duplicates flagged, the Django-toolbar feature), **Answer** (rerender,
    redirect target, reject text, not found, error with the Go stack in
    dev), **Patches** (the SSE events applied: body/title/history/panel),
    **History** (the last 20 requests of this tab; click to reopen),
    **Copy as curl**. Styled with the library (the dev panel is an
    `alder-ui` element in the library too, `al-devpanel`, so any stack can
    reuse it).
  - **`/_dev`**: the app's map: pages (path, requires, roles, params),
    messages per page, tables with columns and attributes, capabilities
    with the wired adapter or fake, the session shape, the migrations
    applied, the build id; links into the running app.
  - Mechanism: the runtime already logs each effect step; dev mode records
    per request a `Trace` (steps, SQL from `rt/db` with timing, answer,
    patches) and the dev server renders the panel from it; the trace is
    attached to the response as one more `datastar-patch-elements` of the
    panel (and inlined on a full document). Cost: dev mode only; a
    `ALDER_DEV_PANEL=0` switch turns it off.
- Book: chapter 11 "Developer mode" (the panel, `/_dev`, what each tab
  shows, how to read a trace, the SQL panel's duplicates), chapter 13 (how
  the trace is recorded and patched), the smoke suite covers the panel's
  presence and one trace.

## Part 3: the admin's upgrades and runtime pieces

- **`Notice "text"` answer**: a message may answer `Rerender`/`Redirect`
  with a notice attached (`Task.succeed (Notice "Saved" Rerender)`? —
  RULING: a new `Nav` constructor `Noticed Text Nav`); the runtime renders
  an `al-toast` in the page's toast region (the layout's `<al-toast-region>`
  or an implicit one before `</body>`); browser tests see it as
  `page.notices`.
- **In-flight state**: while a message is pending, the form or button
  that sent it is `inert` with a busy attribute (`aria-busy`), restored on
  the answer; a pending-navigation bar (an `al-progress` line at the top,
  the Datastar `indicator` signal) for boosted GETs; chapter 6 updated.
- **The admin**: delete through `al-dialog` (the confirmation page stays
  as the no-JS path: the dialog's confirm button posts the same `Remove`
  message); **bulk actions** on list pages (a checkbox per row, "select
  all on page", actions: delete (with the cascade/restrict preview per
  row, refused rows listed), set a Bool column, set a union column;
  messages `BulkDelete { ids }` etc.; tests); **autocomplete** for
  reference selects: `<al-autocomplete field="author" options={model.
  authors} search={SearchAuthors}>`: typing posts `SearchAuthors { q }`
  (debounced via Datastar), the server narrows `model.authors` (≤ 50 rows
  by label match) and re-renders, the control shows the narrowed options,
  the submitted value is checked for membership against the rendered
  options as `<Select options>` is — no new runtime rule; the admin uses
  it when the referenced table has more than 200 rows (the limit M13
  dropped); M12's "conditional options check" is thereby unnecessary.
  Every upgrade regenerated into both examples (replay byte-for-byte),
  tests, the mutation gates ≥ 0.95.
- The scaffold's guestbook uses a toast for "Signed".

## Testing

- The library's checker + CI; the browser smoke suite extended: dialog
  open/close/escape/backdrop/focus-return across a morph, popover
  placement, menu keys, tabs keys, toast timeout, tooltip on hover/focus,
  the dev panel's bar and tabs, autocomplete narrowing, bulk delete, the
  in-flight `inert`; all under `-tags browser`.
- Generators' goldens + mutation gates; examples; book (`go test
  ./book`); the reference ≤ 46k (raise the cap by the elements' tables).

## Non-goals

- Shadow DOM; a JavaScript API for apps; lazy tab panels; drag and drop;
  a theme editor; the dev panel in production; a browser extension;
  Firefox/WebKit smoke runs (Chromium only; noted).
