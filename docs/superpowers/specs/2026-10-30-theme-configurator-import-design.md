# Theme phase 2: the configurator at `/_dev/theme` and `alder theme import`

Design for phase 2 of [tapegram/alder#38](https://github.com/tapegram/alder/issues/38). Phase 1
(`2026-10-29-theme-config-design.md`) shipped the `theme { … }` declaration, its checker (E0042),
the token table (`internal/theme`), the CSS emission and the LSP completion. Its "Later phases"
section fixed this phase's shape: a page `alder run` serves at `/_dev/theme` with alder-ui's
components live, a control per token, a `.dark` toggle, and a write that edits `App.alder` through
the formatter; and `alder theme import <file.css>`, which turns a pasted shadcn/tweakcn export
into the same declaration, listing what it could not place. The user skipped the brainstorm
questions, so everything marked **(call)** is the author's decision from that shape, the
2026-10-17 note ("quick and playful, not hand-editing token files") and the code as it stands.

## Intent

A person who wants to try a look on their app today edits `theme { … }` in `App.alder`, saves,
waits for the rebuild, and looks. The loop is a few seconds long and blind: nothing shows a
colour before it is written, nothing shows the components a token touches, and a tweakcn or
shadcn export has to be transcribed by hand into field names. After this phase, while `alder run`
is up, `/_dev/theme` shows alder-ui's components (and the app's own pages) in the working theme,
every token has a control, a change shows at once without a rebuild, dark and light are one
click apart, and one button writes the declaration into `App.alder` exactly as a person would
have written it. A CSS export is pasted into the page, or given to `alder theme import`, and
becomes that declaration, with what alder-ui does not know handed back as an `app.css` block.

The declaration stays the only source of truth: the page and the importer write it and read it
back; neither keeps state of its own, and nothing they do is needed to use `theme` by hand.

## Goals

1. `/_dev/theme` under `alder run`: a live preview of every alder-ui element and of the app's own
   pages in the working theme, a control per token in `internal/theme.Tokens`, light/dark, and a
   write to `App.alder`. A change shows in the preview without a rebuild.
2. The write produces the declaration the formatter would print, in the one place `theme` is
   allowed, replacing an existing declaration or inserting one; nothing else in the file changes
   beyond what the formatter would change.
3. `alder theme import <file.css>` and the page's paste box share one importer: a shadcn v4
   export (oklch), a shadcn v3 export (bare HSL triplets) and a tweakcn export each become a
   checked theme; what cannot be placed comes back as a ready-to-paste `app.css` block, each line
   with its reason.
4. A theme the tools write passes E0042: a value that would not check is refused before anything
   is written, with the checker's message. The write can never put anything in `App.alder` that
   is not a `theme` declaration over the closed token and value sets.
5. Nothing of this ships in a built app: the page, its handlers and the importer live in the
   `alder` binary (the `alder run` supervisor and the CLI), not in `rt/web`.

## Non-goals

- Phase 3: `textDocument/documentColor`, tokens read from the manifest's `cssProperties`.
- Colour-space conversion (hex ↔ oklch), palette generation from one colour, presets or a theme
  library, an AA-contrast checker. (A contrast readout is noted under "Later" below.)
- Editing `static/app.css`, new tokens (`--spacing`, `--tracking-*`, `--font-serif`,
  `--shadow-2xs`…): the importer hands those back for `app.css`; the page does not show them.
- `alder theme export|show`, `--merge` on import, a TUI, a per-page or per-Layout theme.
- Serving the page from a built app or the admin. Phase 1's `/_dev` panel is unchanged.

## Anti-goals

- A second format: the page's state is the parsed declaration and the write is the formatter's
  print of it. No JSON theme file, no sidecar, nothing in `.alder/`.
- A write that bypasses the checker, or a value that can break out of its string literal: the
  printer writes `"` + text + `"` as written (`internal/format/print_decl.go` `deployValue`), so
  every value is accepted by its kind's check (`theme.ColourOK`, `LengthOK`, `FamilyOK`,
  `ShadowOK`, none of which admit `"`, `;`, `{`, `}` or a newline) **before** it is printed.
- Writing on every slider tick: the preview is client-side; the file changes only on "Write".

## Where it lives

- The page and its handlers: the `alder run` supervisor (`cmd/alder/run.go`'s `devServer`),
  which owns the source tree (`d.dir`, `d.app`), the watcher and the reload stream, and already
  intercepts `/_dev/reload` before proxying everything else to the child **(call)**. The child
  (the built app in dev mode) serves `/_dev` as today and knows nothing of the configurator.
  New files `cmd/alder/theme_ui.go` (handlers, state, write) and `cmd/alder/theme_ui_assets.go`
  (the page's HTML, CSS and JS as Go strings, as `rt/web/devpanel_ui.go` does), tests beside
  them, following `devui.go`'s layout **(call)**.
- The importer: `internal/theme/import.go`, pure, no I/O.
- The source edit: `internal/generate/theme.go`, a generator in the existing copy-check-write
  pipeline (`generate.Apply`), plus its Spec in `internal/generate/specs.go` so `alder help
  --json` and the LSP's command surface list `alder theme import`.
- The printer: `internal/format` gains `Theme(fields []*syntax.DeployField) string`, the
  declaration printed standalone through the existing `deployRecord` path (the file is then
  formatted whole by `Apply`, as every generator's output is).
- The command: `cmd/alder/theme.go` (`alder theme import`), `main.go` case `"theme"` as a group
  like `edit` (`runGroup`).

## The page

### Routes (supervisor-only, under `alder run`)

| route | answer |
|---|---|
| `GET /_dev/theme` | the page (HTML; links `/static/al/al.css`, `/static/al/al.js` and the Datastar script through the proxy, so the components are the vendored ones the app serves) |
| `GET /_dev/theme/state` | JSON: `{version, file, theme, defaults, tokens, building}` — alder-ui's version (`theme.Version()`), the path of `App.alder` relative to the app dir, the committed theme parsed from it (`{root:{field:value}, light:{…}, dark:{…}}`; a file that does not parse gives `theme: null` and an `error`), `Defaults` keyed by field name, the `Tokens` table (field, css, kind, scope) so the page builds its controls from the server's table and never carries one of its own, and whether a build is in progress |
| `POST /_dev/theme/import` | body `{css}` → `{theme, unplaced:[{block, name, value, reason}], skipped}` from `theme.Import`; nothing written |
| `POST /_dev/theme/write` | body `{theme}` → `{written:true, file}` or `{written:false, diagnostics:[diagItem…]}` (the `diagItem` shape `devui.go` already has) |

When no child is up the page is still served (it needs no app state), but its stylesheet links
answer 502 until the build is up; the page shows the supervisor's "building…" state from
`/state` and retries the stylesheet on the reload event **(call)**. The page subscribes to
`/_dev/reload` like every page, so a rebuild (from a write, or from an editor save) reloads it.

### Layout

Two regions **(call)**:

- **The panel** (left, 22rem; stacked above the preview under 60rem): the controls. It is styled
  by its own small stylesheet with neutral colours following `prefers-color-scheme`, and uses
  **no** theme token, so a working theme that puts white on white can never hide the Reset or
  Write buttons. Header: `theme · alder-ui v0.4.2 · app/App.alder`, a Light/Dark switch, and
  three buttons: **Reset** (back to the committed theme), **Import…** (opens the paste box),
  **Write to App.alder** (disabled while the working theme equals the committed one or a build is
  in progress). Below, two groups: **Theme-wide** (`radius`, `radiusSm…Xl`, `widthNarrow/
  Normal/Wide`, `fontSans`, `fontMono`, `shadowXs…Lg`, in `Tokens` order) and **Colours of the
  light|dark scheme** (the colour tokens, in `Tokens` order, for the scheme the switch shows; the
  switch flips both the preview and this group).
- **The preview** (the rest): tabs **Gallery** and **App**. Gallery: one card per alder-ui
  element in the vendored manifest except `al-devpanel` and `al-toasts` (a toast is shown as a
  static `al-toast`), each in its common variants (`al-button` in every `variant` and `size`,
  `al-alert` in every variant, a filled `al-input`/`al-select`/`al-textarea`/`al-checkbox`, an
  `al-table` with three rows, `al-tabs` with two panels, an open `al-popover` and `al-menu`, an
  `al-dialog` opened by a button, `al-badge`s, `al-pagination`, `al-card`, `al-stack`, `al-page`
  around the gallery itself); then a typography block (h1–h3, a paragraph with a link and inline
  `code`, a `pre`), a row of `chart1…chart5` swatches, and a sidebar block using the eight
  `sidebar*` tokens. The markup is hand-written in `theme_ui_assets.go` and **a test holds it to
  the manifest**: every element of `elements.Builtin()` but the two excluded appears as a tag in
  the gallery, and every `al-*` tag in the gallery is in the manifest. App: an `<iframe>` of `/`
  (same origin through the proxy).

### How the preview follows the controls (no rebuild)

The working theme is applied as inline custom properties on the page's `<html>`
(`document.documentElement.style.setProperty("--primary", …)`), **(call)**: the theme-wide
tokens always, plus the colours of the scheme the switch shows; the switch also toggles `class=
"dark"` on `<html>`, as the app does. Inline properties on the root beat every stylesheet, and
alder-ui's derived tokens (`--radius-sm: calc(var(--radius) - 4px)` and kin, computed at
`:root`) re-derive from them, which a scoped subtree would not do. A token the working theme does
not set is removed from the inline style so the stylesheets decide (alder-ui's value, or the
committed theme appended to `al.css`); a token the **committed** theme sets and the working theme
has cleared is set inline to alder-ui's default from `defaults`, so the preview shows what the
build would show. The App tab's iframe gets the same properties and class on its own `<html>`
on every change and on each `load` (a navigation inside it reloads the document); a Datastar
morph inside it leaves `<html>` alone.

The panel ignores the tokens for its own styling but still inherits the properties (custom
properties inherit regardless), which is how its swatches render.

### Controls per kind

Every control has a **set** state: a token not in the working theme shows alder-ui's default as
a placeholder (light or dark default, per the switch, for colours) with a dimmed swatch; typing
sets it; a small × clears it. Each field shows its CSS name on hover (`title`).

- **colour**: a text input holding the value as written (`oklch(0.55 0.22 285)`, `#4f46e5`,
  `var(--accent)`), a swatch whose `background` is that text (the browser renders any CSS colour,
  and `var(--x)` resolves against the inherited properties, so an alias shows the colour it
  aliases; an invalid value shows a hatched swatch), and a native `<input type="color">` that,
  when used, writes `#rrggbb` into the text **(call)**. When the text is an `oklch(L C H)` with
  three plain numbers, three range inputs L (0–1, step .005), C (0–0.4, step .005) and H (0–360,
  step 1) appear under it and rewrite those numbers in place; no conversion between colour
  spaces anywhere **(call)**: a pasted oklch stays oklch, a picked colour is hex.
- **length**: a number input and a unit select (`rem em px % vw vh ch lh`), joined as text;
  `0` prints as `"0"`.
- **font stack**: one text input, families separated by commas (`Inter, system-ui,
  sans-serif`), split and trimmed into the list, surrounding quotes dropped; a `<datalist>` of
  six common stacks (the system sans and mono stacks, Inter, Geist, Geist Mono, a serif) as
  suggestions **(call)**.
- **shadow**: a text input.

The client does not validate; the preview just shows what the browser makes of a value. The
write validates (below) and the page shows each diagnostic under its field (matched by the field
name in the message) and the rest above the groups.

### Working state across reloads

The working theme is kept in `sessionStorage` under the app dir, with the committed theme it was
started from; after a reload the page restores it when the committed theme is unchanged and
discards it otherwise (the editor changed the declaration, or the write landed) **(call)**.
Every access is in `try/catch`, as the dev panel's are.

## The write

`POST /_dev/theme/write` and `alder theme import` both end in `generate.ThemeEdit(p *Program,
th theme.Theme) ([]Edit, error)` followed by `generate.Apply`:

1. **Validate first.** Every field is looked up in `theme.Tokens` (an unknown field is an error:
   the page cannot produce one, the importer never does) and every value passes its kind's check
   (`ColourOK` with `Alias` resolving to a colour token, `LengthOK` and not `NegativeLength`,
   `FamilyOK` per family and a non-empty list, `ShadowOK`); colours only under `light`/`dark`,
   the rest only at the top. A failure is returned as the E0042 message the checker would give,
   located on the declaration, and **nothing is printed or written**. This is what keeps rule 4
   and the anti-goal on string literals: no text reaches the printer that the checker's closed
   sets did not admit.
2. **Print.** `format.Theme` prints `theme { … }` with the top-level fields in `Tokens` order,
   then `light: { … }`, then `dark: { … }` (each in `Tokens` order; an empty record omitted), so
   two writes of the same theme are byte-identical and the diff of a change is one line **(call)**.
   An empty theme (nothing set) prints nothing and the edit deletes the existing declaration;
   no `theme { }` is ever written **(call)**.
3. **Splice.** `App.alder` is parsed fresh at write time (`syntax.Parse`, through the Program's
   App module); with a `ThemeDecl`, its span's lines are replaced by the print; without one, the
   print is inserted with a blank line each side after the `deploy` declaration if there is one,
   else after `session`, else after the last `import`, else after the `module App` line
   **(call)**. The `srcEdit` helpers in `internal/generate/srcedit.go` do the line work.
4. **Apply.** `generate.Apply` formats the copy whole (as every generator's output is formatted),
   checks it, and writes back. Options: `WriteDespiteErrors` with `Guard` matching the
   `ThemeDecl`, as `alder edit table` does **(call)**: an error located in the theme refuses the
   write (it cannot happen after step 1, but the checker is the authority); errors elsewhere in
   the app are the person's existing to-do list and do not stop a theme from being written. The
   result's diagnostics come back to the page as `diagItem`s.

The supervisor's watcher then sees `App.alder` change, rebuilds, and broadcasts the reload; the
page's Write button stays disabled from the request until that reload (the `/state` `building`
flag, polled once a second while true) **(call)**.

### Protecting the write

The write changes a file on disk from a browser, so **(call)**:

- POST bodies are JSON and read only with `Content-Type: application/json`; a request whose
  `Sec-Fetch-Site` is `cross-site` or whose `Origin`, when present, is not the dev server's own
  origin is answered 403 (a form or a cross-origin fetch from another site cannot reach the
  handler; DNS rebinding changes `Host`, which is compared with the address the server listens
  on). `GET /_dev/theme/state` carries nothing secret (the declaration is the app's source) but
  is gated the same way for symmetry.
- The body is limited to 256 KiB; the importer's input likewise.
- Only `App.alder` under the app dir is ever written, through `Apply`'s copy-check-write, and
  only with the print of a validated theme.

## The importer: `theme.Import(css string) (Theme, []Unplaced, Skipped)`

Pure, in `internal/theme/import.go`. `Theme{Root, Light, Dark map[string]string}` keyed by field
name (the same shape the page and the edit use); `Unplaced{Block, Name, Value, Reason string}`;
`Skipped` counts the blocks dropped silently.

**Blocks.** The CSS is tokenised into rules (a small hand-written scanner: comments stripped,
strings and parentheses respected, nested `@media`/`@layer` bodies entered, `@theme`/`@import`/
`@tailwind`/`@plugin`/`@custom-variant` at-rules skipped and counted). A rule's selector list
decides where its declarations go:

| selector contains | goes to |
|---|---|
| `:root`, `html`, `.light`, `[data-theme="light"]`, `:root.light` (and not `.dark`) | light (colours) and root (theme-wide) |
| `.dark`, `html.dark`, `:root.dark`, `[data-theme="dark"]`, `.dark:root` | dark (colours); theme-wide tokens compared with root's (below) |
| anything else | every `--` declaration is unplaced, reason "`.sidebar` is not a scheme selector" |

Only custom-property declarations (`--name: value;`) are read; other declarations are ignored,
not reported (a `body { font-family: … }` is not a theme). The last declaration of a name in a
block wins, as in CSS.

**Names.** `theme.ByCSS(name)` places a known token: a colour from a light block → `Light`, from
a dark block → `Dark`; a theme-wide token from a light block → `Root`; a theme-wide token in a
dark block is accepted when its value equals root's and unplaced otherwise, reason "`--shadow-sm`
is theme-wide in alder-ui; the dark value differs". A `--al-*` name in `theme.Removed` is
unplaced with the Removed message ("`--al-font-mono` was removed in alder-ui v0.4: write
`fontMono`"). Any other `--name` is unplaced, reason "not a token of alder-ui v0.4.2".

**Values.**
- A colour whose value is three bare channels `H S% L%` (shadcn v3: `222.2 47.4% 11.2%`, meant
  for `hsl(var(--x))`) is wrapped as `hsl(222.2 47.4% 11.2%)` **(call)**; a bare `H S% L% / A`
  too. Anything else is kept as written, trimmed. `var(--x)` is kept when `x` is a colour token's
  CSS name and unplaced otherwise ("`var(--sidebar-accent-2)` is not a colour token").
- A font stack value is split on commas outside quotes, each family trimmed and unquoted
  (`'Inter', ui-sans-serif, system-ui, sans-serif` → `["Inter", "ui-sans-serif", "system-ui",
  "sans-serif"]`).
- A length and a shadow are kept as written.
- Every placed value is then checked with its kind's check; one that fails is unplaced with the
  checker's wording ("`--radius: calc(0.5rem + 2px)` is not a length alder-ui accepts: `0.5rem`,
  `8px`").
- `!important` is dropped silently.

**Nothing found** (no light or dark block with a known token) is an error: "no theme in
`theme.css`: expected a `:root { --primary: … }` or `.dark { … }` block".

### `alder theme import`

```
alder theme import <file.css> [--dir app] [--dry-run] [--print] [--json]
```

`<file.css>` or `-` for stdin (a paste). Reads, imports, validates, then `ThemeEdit` + `Apply`
exactly as the page's write (the whole declaration is **replaced**: an export is a complete
theme; a field the export does not set is dropped, and the summary says so **(call)**).
`--print` writes the declaration to stdout instead of the file (the way to use it from an agent
or to look first); `--dry-run` runs the pipeline without writing, as every generator's does.

Output, through `internal/style` like the other generators:

```
theme: 31 tokens → app/App.alder  (light 14, dark 14, theme-wide 3; replaced the theme that was there)
6 not placed — paste into static/app.css if you want them:
  :root, .light {
    --font-serif: Lora, Georgia, serif;      /* not a token of alder-ui v0.4.2 */
    --tracking-normal: 0em;                 /* not a token of alder-ui v0.4.2 */
    --shadow-2xs: 0 1px 2px 0 hsl(0 0% 0% / 0.05); /* not a token of alder-ui v0.4.2 */
  }
  .dark {
    --shadow-sm: 0 1px 3px 0 hsl(0 0% 0% / 0.4);   /* theme-wide in alder-ui; the dark value differs */
  }
skipped 2 blocks (@theme inline, @custom-variant)
```

The unplaced block is valid CSS with the reasons as comments, grouped by the block each came
from, so a paste into `app.css` keeps working as the escape hatch phase 1 fixed. `--json` prints
`{"written":bool,"file":…,"theme":{…},"unplaced":[…],"skipped":n,"diagnostics":[…]}`; exit 1
when nothing was written. The page's **Import…** box runs the same importer, puts the result in
the working theme (not written), and lists the unplaced block beside it with a copy button.

## Formatter, Spec, book

- `internal/format.Theme(fields)`: builds the `Doc` through `deployRecord` and renders at the
  formatter's width; the 3-or-more-fields-vertical rule applies to each record, so `light` with
  three colours goes vertical and `radius` alone stays on one line, as chapter 3 says.
- `internal/generate/specs.go`: a `theme` group with the `import` Spec (args `file`, flags
  `dir`, `dry-run`, `print`, `json`), so `alder help --json` and the LSP's command list carry it.
- Book: Appendix C gains two sections after "Changing a token": **"Trying a look: /_dev/theme"**
  (what the page shows, that nothing is written until Write, that the panel never uses the
  theme, the App tab) and **"Importing a theme"** (`alder theme import`, the three export shapes,
  what comes back for `app.css`, `--print` for agents); chapter 15's generator list gains `alder
  theme import`; chapter 11's dev-tools section links `/_dev/theme` beside `/_dev`. Site
  regenerated (`book/site/build.py`). `appendix_test.go`'s caps may need the usual bump.

## Where things change

| layer | files |
|---|---|
| importer | new `internal/theme/import.go`, `import_test.go`, `testdata/import/{shadcn-v4,shadcn-v3,tweakcn,sidebar-only,garbage}.css` |
| printer | `internal/format/theme.go` (`Theme`), `format_test.go` |
| edit | new `internal/generate/theme.go` (`ThemeEdit`), `theme_test.go`; `specs.go` (`theme import`) |
| command | new `cmd/alder/theme.go`, `theme_test.go`; `main.go` (`case "theme"`) |
| page | new `cmd/alder/theme_ui.go` (routes, state, write, origin check), `theme_ui_assets.go` (HTML/CSS/JS, gallery), `theme_ui_test.go`, `theme_ui_browser_test.go` + `testdata/browser/theme.js`; `run.go` (`ServeHTTP` dispatches `/_dev/theme…` before the proxy) |
| book | `book/C-design-system.md`, `11-tooling.md`, `15-generators.md`, `book/site` |

Nothing changes in `rt/`, `internal/check`, `internal/ir`, `internal/gen` or the grammar.

## Verification (tests named by behaviour)

- **Importer**: a shadcn v4 export (oklch, `@theme inline`, `--radius`) places every colour in
  both schemes and `radius` at root, skips one block; a shadcn v3 export wraps bare HSL triplets
  (with and without `/ alpha`) in `hsl()`; a tweakcn export places the shadcn tokens and returns
  `--font-serif`, `--tracking-normal`, `--spacing`, `--shadow-2xs`, `--shadow-xl`, `--shadow-2xl`,
  `--shadow-color`, `--shadow-opacity`, `--shadow-blur`, `--shadow-spread`, `--shadow-offset-*`
  unplaced with "not a token" and a dark `--shadow-sm` that differs unplaced with "theme-wide";
  a font stack with quoted families splits and unquotes; `--al-font-mono` returns the Removed
  message; `var(--sidebar-accent-2)` and `calc(…)` as a length are unplaced with the checker's
  wording; a `.sidebar { --x }` block is unplaced by selector; `!important` is dropped; a file
  with only `@theme inline` is "no theme"; a block declared twice keeps the last value; the
  unplaced block renders as valid CSS grouped by block with reasons as comments.
- **Printer**: `format.Theme` of the blog's theme is the blog's text byte for byte; fields come
  out in `Tokens` order whatever order they went in; an empty record is omitted.
- **Edit**: inserting into the scaffold's `App.alder` puts the declaration after `session { }`
  with a blank line each side; into a file with `deploy`, after `deploy`; into one with neither,
  after the imports; replacing the blog's declaration changes only those lines (the rest of the
  file byte-identical); an empty theme deletes the declaration and its surrounding blank line;
  a value with `"` or a newline, a colour at top level, a negative length, an unknown `var()`
  are each refused before `Apply` with the E0042 message and nothing on disk changes; an
  unrelated type error elsewhere in the app does not stop the write (`WriteDespiteErrors`), an
  error in the theme does (`Guard`).
- **Page** (httptest against a temp app, no child): `/state` returns the blog's declaration,
  `Defaults` and `Tokens`; the gallery holds every manifest element but the two excluded, and no
  tag the manifest lacks; `/write` of a valid theme writes the file and answers `written:true`;
  of an invalid one answers the diagnostics and leaves the file; a POST with `Sec-Fetch-Site:
  cross-site`, a foreign `Origin`, a non-JSON content type or a 300 KiB body is refused; `/import`
  returns the importer's result and writes nothing.
- **Browser** (`//go:build browser`, Playwright, as `devui_browser_test.go`): open `/_dev/theme`
  on the blog under `alder run`; change `primary` to `#ff0000` → the gallery's `al-button`'s
  computed background is `rgb(255, 0, 0)` with no rebuild (the child's generation unchanged);
  switch to Dark → `<html class="dark">` and the dark colours' controls; clear `primary` → the
  button shows alder-ui's default, not the committed theme's value (the default is set inline);
  Write → `App.alder` contains the new `primary`, a rebuild runs, the
  page reloads with the new committed theme and an empty working diff; the App tab's iframe
  `<html>` carries the same inline properties after a navigation inside it.
- **CLI**: `alder theme import` on the three fixtures against a temp scaffold app (writes,
  summary line counts, unplaced block, exit codes); `--print` writes the declaration and not the
  file; `--dry-run` writes nothing; `--json` shape; `-` reads stdin; `alder help --json` lists it.
- `go test ./...` green, `alder build` of `examples/blog`, and a visual pass of the page in
  light and dark.

## Later (not this phase)

- A contrast readout per foreground/background pair, computed from the rendered swatches'
  `getComputedStyle` colours (no colour-space code needed); would make the AA-strict advice of
  Appendix C checkable in the page.
- `--merge` on import; `alder theme export` (the CSS the build emits, for sharing); presets.
- Phase 3 as fixed in phase 1: `documentColor` and `cssProperties` from the manifest.
