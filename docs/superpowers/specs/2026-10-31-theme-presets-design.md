# Theme phase 3: starting themes, DTCG tokens, editor swatches

Design for phase 3 of [tapegram/alder#38](https://github.com/tapegram/alder/issues/38). Phase 1
(`2026-10-29-theme-config-design.md`) shipped the `theme { … }` declaration, its checker (E0042),
the token table (`internal/theme`), the CSS emission and LSP completion. Phase 2
(`2026-10-30-theme-configurator-import-design.md`) shipped `/_dev/theme` and `alder theme
import` for shadcn/tweakcn CSS. Phase 1 fixed this phase as "`documentColor`, and `Tokens` read
from the manifest's `cssProperties`". The brainstorm (2026-10-31) widened and redirected it:

- The user wants **a set of starting themes** and **custom themes per app**. Five starting themes
  were chosen from rendered mockups (all five kept): Zinc, Paper, Catppuccin, Brutal, Pebble.
- A starting theme is taken **by copy**: `alder theme use paper` (or the configurator) writes a
  full `theme { … }` into `App.alder`, and from then on the app owns it. Nothing in the build or
  the runtime knows what a preset is.
- The presets are **DTCG `tokens.json` files that live in alder-ui** (the W3C Design Tokens
  Community Group format, first stable version 2025.10, published 2025-10-28). The user agreed
  to this over Alder-syntax snippets because one reader then serves presets, a designer's file,
  and the token table.
- The manifest's `cssProperties` is dropped as the table's source: it is per-element, untyped
  and has no light/dark variants. The same preset files replace it.
- **One theme per app** for now; the design must not make several named themes with an end-user
  switch impossible later (see "Future: several named themes").

Everything marked **(call)** is the author's decision; the rest is the user's.

## Intent

A new Alder app looks like shadcn's neutral theme until someone writes tokens by hand or in the
configurator. After this phase, `alder new shelf --theme paper` (or `alder theme use paper` in an
existing app, or "Start from: Paper" in `/_dev/theme`) gives the app a complete, deliberate
look — light and dark colours, radii, fonts, shadows — as a plain `theme { … }` in `App.alder`
that the person then edits as their own. The five looks are different enough that the choice
means something: a reading site, a dev tool, a launch page and a consumer app each have a
natural pick. In the editor, every colour in the declaration shows a swatch. And the token
table alder stopped hand-writing is read from the same files alder-ui publishes its own default
look in, so a new or renamed token in alder-ui reaches the checker, the completion and the
configurator on re-vendor.

The declaration stays the only source of truth for an app's theme. A preset is an input, like
a shadcn export; it is never state.

## Goals

1. alder-ui publishes five themes under `themes/` in DTCG format, each complete over the token
   set, each passing alder-ui's AA text-contrast check in both schemes, shown on its showcase.
   Zinc is alder-ui's current `tokens.css` values; a test holds the two together.
2. `alder theme use <name>` writes the preset as the app's whole `theme { … }` (replace or
   insert, exactly as `alder theme import` does); `alder theme use` alone lists the presets with
   their one-line purposes; `alder new <name> --theme <preset>` scaffolds with it.
3. `/_dev/theme` gets a **Start from** picker: choosing a preset replaces the working theme in
   the preview; nothing is written until Write, as before.
4. `alder theme import` and the page's paste box also accept a DTCG file (a resolver document
   with light/dark contexts, or a single tokens file), through the same reader the presets use;
   what cannot be placed comes back the way the CSS importer reports it.
5. The LSP announces `colorProvider`; `textDocument/documentColor` returns a swatch for every
   colour value in `App.alder`'s theme, and `colorPresentation` writes a picked colour back as
   hex.
6. `internal/theme`'s `Tokens`, `Defaults` and per-token descriptions are read from the vendored
   Zinc preset instead of a hand-written Go list plus a parse of `al.css`. `Removed` stays
   hand-written. The test that holds the table to `al.css` stays.
7. Nothing of this ships in a built app: presets, the reader and the picker live in the `alder`
   binary; `rt/` only gains the one new alder-ui token (below).

## Non-goals

- Several themes in one app, an end-user theme switch, a theme per page or Layout.
- `alder theme export` (the app's theme as a DTCG file), `--merge`, a contrast readout in the
  page, bundling a serif font, palette generation, hex↔oklch conversion in the page.
- A mapping UI for a designer's file whose names are not alder-ui's (`color.brand.500` →
  `primary`); such tokens are reported unplaced, as unknown CSS names are today.
- Reading `$extends`, `$ref` JSON pointers to sub-properties, composite types alder-ui does not
  theme (`typography`, `border`, `gradient`, `transition`, `strokeStyle`), `duration`, `number`,
  `fontWeight`, `cubicBezier`: reported unplaced with their type as the reason.
- Theme-aware code completion of colour *values*, or swatches outside the `theme` declaration
  (a `style=` attribute, `app.css`).

## Anti-goals

- A preset with runtime existence: no `preset:` field in the declaration, no preset name in the
  emitted CSS, no `@import` of a theme stylesheet. The app's theme is the declaration's text,
  whole.
- A second source of truth for the app: the DTCG file is read and converted; alder never writes
  one for the app, keeps one beside `App.alder`, or reads one at build time.
- A write that bypasses the checker: `use` goes through phase 2's validate-then-print path, so a
  preset that fails E0042 (a vendoring mistake) is refused with the checker's message.
- Brand imitations as presets (the "Claude", "Vercel", "Supabase" presets of tweakcn): the
  starting set is two open palettes and three idioms.

## The five starting themes

Each is a complete look: light and dark colours (the 32 colour tokens), `radius` and the four
derived radii set explicitly, the three widths, both font stacks, the four shadows, and the new
`borderWidth`. The values rendered in the brainstorm's mockup are the starting point and are in
Appendix A. Where alder-ui's `test/check.py` reports a text pairing under AA, the builder tunes
in this order and no further **(call)**: first swap the *foreground* of the pair to the theme's
dark or light text colour (Pebble's coral button gets dark text rather than a darker coral);
then move the background's oklch L by at most ±0.08 and C by at most ±0.03; never change a
hue. Each change is listed in the PR. A pairing that cannot pass within that goes back to the
user with the two candidates rendered — the user approved looks, and a look that has drifted
past this is a new decision.

| name | purpose (the one line `alder theme use` prints) | character |
|---|---|---|
| **zinc** | Tools, dashboards, admin — the default, the safe choice. | alder-ui's current values, verbatim: neutral grey, radius 0.625rem, subtle shadows, Geist. |
| **paper** | Reading: blogs, docs, long-form. | Cream and warm ink, system serif stack for `fontSans` (`Charter, "Bitstream Charter", "Iowan Old Style", Georgia, serif`) **(call)**, radius 0.125rem, shadows off. |
| **catppuccin** | Developer-facing apps. | Catppuccin Latte (light) and Mocha (dark), official palette (MIT); mauve primary, radius 0.75rem, soft shadows. |
| **brutal** | Launch pages, portfolios, anything that should shout. | Pure white/black, yellow primary and hot-pink accent, radius 0, `borderWidth` 2px, hard offset shadows (`3px 3px 0 0 var(--border)`), Geist. |
| **pebble** | Consumer, mobile-feel apps. | Cool off-white, coral primary, minty accent, radius 1.25rem, diffuse shadows. |

Order everywhere (list, picker, showcase): zinc first, then alphabetical **(call)**.

Catppuccin's licence (MIT) requires its copyright notice: `themes/catppuccin/LICENSE` in
alder-ui, carried into alder's vendored copy and listed in the book's licences line **(call)**.

### The one component-library change: `borderWidth`

Brutal's thick lines cannot come from colour tokens alone. alder-ui adds a theme-wide token
`--al-border-width` (default `1px`) and every `border`/`border-<side>`/`border-width`
declaration that is today a literal `1px` (seventeen component files, plus `typography.css`'s
rules) uses it. **Only borders**: `outline-width` focus rings, `block-size: 1px` separators and
`al-tabs`' `calc(100% - 1px)` underline offset keep their literals, so under Brutal the boxes
thicken and the rings, separators and tab underline do not **(call)**. At the default the built
stylesheet is pixel-identical, so the parity check (`test/parity.py`) is unchanged. In alder the token is `borderWidth`, kind Length,
theme-wide, named like `widthNarrow` (`--al-*` → drop the prefix). Shadows may reference
`var(--border)` as a colour: `ShadowOK` must accept a `var(--x)` of a colour token inside a
shadow (Brutal's hard shadows flip with the scheme); check what it accepts today and widen it
if needed **(call)**.

## The DTCG files in alder-ui

`themes/<name>/` holds four files, the standard's resolver shape **(call)**:

```
themes/paper/theme.resolver.json
themes/paper/base.tokens.json      # theme-wide: radius, radius-*, al-width-*, font-*, shadow-*, al-border-width
themes/paper/light.tokens.json     # the 32 colours, light
themes/paper/dark.tokens.json      # the 32 colours, dark
```

```json
{
  "$schema": "https://www.designtokens.org/schemas/2025.10/resolver.json",
  "version": "2025.10",
  "name": "Paper",
  "description": "Reading: blogs, docs, long-form.",
  "sets": { "base": { "sources": [{ "$ref": "base.tokens.json" }] } },
  "modifiers": {
    "scheme": {
      "contexts": { "light": [{ "$ref": "light.tokens.json" }], "dark": [{ "$ref": "dark.tokens.json" }] },
      "default": "light"
    }
  },
  "resolutionOrder": [{ "$ref": "#/sets/base" }, { "$ref": "#/modifiers/scheme" }]
}
```

`name` and `description` are the preset's title and purpose line.

**Token names.** A token's name (the last segment of its path) is its CSS custom property
without the leading `--`: `color.primary-foreground` is `--primary-foreground`,
`width.al-width-narrow` is `--al-width-narrow`. Groups carry `$type` and are organisational, as
the standard says; alder reads nothing from them **(call)**. Each token carries `$description`:
one sentence on what the token colours (the Zinc file's descriptions are the ones that reach
hover and the configurator).

```json
{
  "color": {
    "$type": "color",
    "primary":            { "$value": { "colorSpace": "oklch", "components": [0.3, 0.03, 50] }, "$description": "Buttons, links, focus rings." },
    "primary-foreground": { "$value": { "colorSpace": "oklch", "components": [0.97, 0.012, 85] }, "$description": "Text on --primary." },
    "ring":               { "$value": "{color.primary}", "$description": "Focus ring." }
  }
}
```

**Value forms** (DTCG 2025.10): colour `{colorSpace, components[3], alpha?, hex?}`; dimension
`{value, unit: "px"|"rem"}`; fontFamily a string or array; shadow an object
`{color, offsetX, offsetY, blur, spread, inset?}` or an array of them. An alias is
`"{group.name}"` and resolves to a whole value.

**Zinc is the source of alder-ui's own tokens.** `tokens.css`'s `:root, .light`, `.dark` and
derived blocks keep their hand-written form and comments; `test/check.py` gains a `themes`
section **(call)**: for each `themes/*/`, the resolver parses, the three files declare exactly
the token set `tokens.css` declares (no more, no fewer; `--color-white`/`--color-black` and
`--_al-*` excluded), every value is of the group's `$type`, every alias resolves, and the
**contrast** check runs over the theme's resolved colours as it does over `tokens.css`'s (AA
4.5:1 text pairings fail the check; 3:1 borders warn). For `zinc`, each resolved value must
equal `tokens.css`'s value for that property, textually after normalising an oklch triple and a
dimension to CSS (the derived `--radius-*` in `tokens.css` are `calc()`s: Zinc's file carries
them as the computed dimensions, `0.375rem`/`0.5rem`/`0.625rem`/`0.875rem`, and the check
compares those against the `calc` evaluated at `--radius` — or simpler, the check skips the
four and alder's own test covers them **(call, builder's choice)**).

**Showcase.** `scripts/showcase.py` adds a theme select beside the light/dark toggle: it
inlines each preset's resolved values as a `<style>` block per theme under a class on `<html>`
(`.theme-paper.light { … }`, generated by a small Python converter sharing the value rules
below) so the whole showcase repaints — the same mechanism alder would use for several named
themes later **(call)**.

**Release and sequencing.** The themes, the `--al-border-width` token, the check and the
showcase land first, as one alder-ui PR tagged v0.5.0; the alder work starts after the tag
**(call)**. `scripts/vendor-al.sh v0.5.0` in alder copies `themes/` to
`internal/theme/presets/` (not `rt/al/`: presets are a tool input, not runtime files) along
with the usual files, and `VERSION` moves to v0.5.0. Messages that print "alder-ui v0.4.2"
follow `theme.Version()`. Inside alder the order is: vendor + reader (its tests run on
`testdata/dtcg`, so it can be written before the vendor lands, but the vendor merges first) →
presets, `theme use`, `new --theme` → the configurator's picker → `documentColor` → the table
from Zinc (last, because it deletes the hand-written list and touches every caller). Each is
its own PR; the plan may split them further.

## The reader: `theme.ImportDTCG`

Pure, in `internal/theme/dtcg.go` **(call)**:

```go
func ImportDTCG(fsys fs.FS, name string) (Theme, []Unplaced, Skipped, error)
```

`name` is a resolver document or a tokens file inside `fsys` (a preset's embedded dir; the
directory of the file given to `alder theme import`; for stdin, an `fstest.MapFS` with the one
file). The result shape is the CSS importer's, so `alder theme import`, the page's import box
and `use` share one tail.

**Which file is which.** A document with `resolutionOrder` is a resolver: its sets become
`Root` (merged in order, later wins); the modifier whose name is `scheme`, `theme`, `mode`,
`color-scheme` or `colorScheme` (first found, case-insensitive) and whose contexts include
`light` and `dark` (case-insensitive) supplies `Light` and `Dark`; a token found in a set that
is a colour goes to **both** schemes; other modifiers are resolved at their `default` and any
without one at their first context, counted in `Skipped` with the modifier's name **(call)**.
A tokens file without `resolutionOrder` is read alone: top-level groups named `light` and
`dark` (case-insensitive) are the schemes **(call)**; everything else is root, and a colour not
under either goes to both schemes. Anything the `$extensions` carry is ignored.

**Names.** For each token, the last path segment normalised (lower-case; `_` and camelCase
humps → `-`) is looked up with `theme.ByCSS("--" + seg)`, then as a field name with
`theme.Lookup`; `al-` is tried as a prefix for a bare `width-narrow` or `border-width`
**(call)**. A name in `Removed` gets the Removed message; any other is unplaced "not a token of
alder-ui v0.5.0". A `$type` that does not match the token's kind is unplaced ("`radius` is a
length; the file says `color`"). A missing `$type` with no inherited one: unplaced "no $type".

**Values → the CSS text the declaration holds.**

- colour: `oklch`, `oklab`, `lab`, `lch`, `hsl`, `hwb` → that function, components as written
  (`oklch(0.3 0.03 50)`; `hsl` with `%` on the last two); `srgb` → `hex` when present and alpha
  is 1, else `rgb(r g b)` with the components × 255 (integers when exact, else three decimals),
  so Zinc's `rgb(0 0 0 / 0.05)` shadows and a `transparent` (`rgb(0 0 0 / 0)`) come back as
  CSS everyone reads; any other space → `color(<space> c1 c2 c3)`; `alpha` ≠ 1 appended as
  `/ a`; a `"none"` component → `none`. The text then passes `ColourOK`. Numbers print in Go's
  shortest form (`0.3`, never `.3`) **(call)**.
- dimension: `0` → `"0"`, else `value` + `unit` (`0.5rem`); then `LengthOK`/`NegativeLength`.
- fontFamily: string → one family; array → the list; then `FamilyOK` per family.
- shadow: `offsetX offsetY blur spread color`, `inset` first when true, layers joined by `, `;
  each part converted by its own rule; then `ShadowOK`.
- alias `{path}`: when the target is a colour token of this theme → `var(--target)` (kept as
  the declaration's own alias form, which `Alias` accepts); otherwise the target's value is
  substituted (recursively, cycles an error) and converted. An alias to an unknown token is
  unplaced ("`{color.brand.500}` is not a token").
- Anything else (a bare string where an object is expected, an unknown type): unplaced with
  the reason.

A failing check is unplaced with the checker's wording, as in the CSS importer. The reader
rejects a document over 1 MiB or with more than 32 referenced files **(call)**; a `$ref` that
escapes `fsys` (`..`, absolute) is an error.

**`Unplaced.Block`** carries the file and path (`light.tokens.json color.brand-500`) so
`UnplacedCSS` still groups by origin; the rendered block uses the CSS name the reader would
have used and the DTCG value serialised as a CSS comment when no CSS text was produced.

### Detecting the format in `alder theme import` and the paste box

The first non-blank byte decides: `{` → DTCG, else CSS **(call)**; `--format css|dtcg` forces
it. For a file path, `fsys` is `os.DirFS(dir)` and `name` the base name, so a resolver's
sibling files resolve; for `-` and the page's paste, a single document only (a resolver that
references files is "a resolver needs its files: run `alder theme import <dir>/theme.resolver.json`").

## Presets in alder

`internal/theme/presets.go` **(call)**:

```go
type Preset struct{ Name, Title, Purpose string; Theme Theme }
func Presets() []Preset          // sync.OnceValue over the embedded presets/ dir, zinc first then by Name
func Preset(name string) (Preset, bool)
```

Built by `ImportDTCG` over `embed.FS` at first use. A preset with any `Unplaced` or an error is
a **test** failure (`TestPresets_AllPlaced`), not a runtime condition: `Presets()` panics on one,
which the test catches first. `Name` is the directory name; `Title`/`Purpose` are the
resolver's `name`/`description`.

`internal/generate`: `UsePreset(app App, name string, print, dryRun bool) (*Result, error)`
beside `ImportTheme`, ending in `WriteTheme` (replace the whole declaration; the summary says
"replaced the theme that was there" as import does). Spec `theme use` in `themeSpecs()` (arg
`name`, flags `dir`, `dry-run`, `print`, `json`) so `alder help --json` and the LSP command list
carry it.

```
$ alder theme use
starting themes (alder-ui v0.5.0):
  zinc        Tools, dashboards, admin — the default, the safe choice.
  brutal      Launch pages, portfolios, anything that should shout.
  catppuccin  Developer-facing apps.
  paper       Reading: blogs, docs, long-form.
  pebble      Consumer, mobile-feel apps.
alder theme use <name> writes one into app/App.alder; the app owns it from there.

$ alder theme use paper
theme: paper → app/App.alder  (light 32, dark 32, theme-wide 15; replaced the theme that was there)
```

An unknown name: "no starting theme `papr`; did you mean `paper`?" (edit distance, as
`theme.Suggest`) and the list, exit 1. `--json` is `ImportTheme`'s shape plus `"preset":"paper"`.

`alder new <name> --theme <preset>` **(call)**: after the scaffold is copied and before the
migrate step, `UsePreset` runs against the new app with its full output suppressed to one line
in `new`'s recap ("theme: paper"); an unknown preset fails before anything is created. Without
the flag nothing is written: the app is Zinc because alder-ui is. `--theme zinc` writes Zinc's
declaration explicitly (a person who wants the values in front of them).

## The configurator's Start from

The panel header (phase 2) gains a **Start from** select between the Light/Dark switch and
Reset **(call)**: its options are `Presets()` (title; purpose as the option's `title`
attribute), with a leading "—" when the working theme is not a preset. Choosing one replaces
the working theme with the preset's whole theme (every token set, so the preview shows the
preset exactly, not the preset over the app's leftovers), the controls refill, the Write button
enables. It is a working-state change like any control: Reset returns to the committed theme,
nothing is written until Write, and the reload-survival rule (phase 2) applies. Beside the
select, five small swatches of the chosen preset's light `primary`, `background`, `accent`,
`foreground`, `destructive` **(call)**, drawn with the panel's neutral styles. `/state` carries
`presets: [{name,title,purpose,theme}]`.

## Editor swatches: `documentColor`

`capabilities()` adds `"colorProvider": true`. New `internal/lsp/color.go` **(call)**:

- `textDocument/documentColor`: for the App module only, walk the `ThemeDecl`'s `light` and
  `dark` records; for each field whose token kind is Colour and whose value is a string
  literal, parse the text with `theme.ParseColour` (below); a parse gives a `ColorInformation`
  with the range of the literal's contents (inside the quotes) and `{red, green, blue, alpha}`
  in 0–1 sRGB. A `var(--x)` resolves to the same scheme's own value for `x` in this declaration,
  else alder-ui's default for that scheme (`Defaults()`), then parses that. Anything that does
  not parse yields nothing (no diagnostic: the checker owns errors).
- `textDocument/colorPresentation`: one presentation, `#rrggbb` (or `#rrggbbaa` when alpha < 1),
  with a `textEdit` replacing the literal's contents **(call)** — the same choice phase 2's
  picker made (a pasted oklch stays oklch until a colour is picked; a picked colour is hex).

`theme.ParseColour(text string) (c RGBA, ok bool)` in `internal/theme/colour.go` **(call)**:
hex 3/4/6/8; `rgb()`/`rgba()` (legacy commas and modern space syntax, numbers or percentages);
`hsl()`/`hsla()`; `hwb()`; `oklch()`, `oklab()`, `lab()`, `lch()` (converted through the
CSS Color 4 matrices to linear sRGB, then gamma-encoded, then **clipped** to 0–1 — a swatch,
not a gamut map); `color(srgb …)` and `color(display-p3 …)` (P3 through its matrix); the 148
CSS named colours and `transparent`; `/ alpha` and the legacy fourth argument; `none` as 0.
`currentColor`, `color-mix()`, `light-dark()` and `var()` are not parsed here (the LSP handles
`var`). Table-driven tests against known conversions (white, black, the shadcn greys,
`oklch(0.577 0.245 27.325)` ≈ `#e7000b`, P3 red clipped).

Neovim: `alder.nvim` does nothing new in this phase; whether the user's Neovim renders
`documentColor` natively (`vim.lsp.document_color`, Neovim 0.12) or needs a plugin is checked
by the builder and recorded in the alder.nvim README **(call)**.

## The token table from Zinc

After the presets are vendored, `internal/theme` reads its table from
`presets/zinc/{base,light,dark}.tokens.json` **(call)**:

- `Tokens` becomes `Tokens() []Token` (a `sync.OnceValue`), built in **file order**: base
  first, then the light file's colours (dark must declare the same set; a difference is a test
  failure). JSON key order is therefore load-bearing: the reader keeps it (a streaming
  `json.Decoder` walk, not a map), and alder-ui's check asserts the files' order against
  `tokens.css` **(call)**. The base file's order is today's `Tokens` order — `radius`, the four
  derived radii, the three widths, the two fonts, the four shadows — with `al-border-width`
  appended, so `Order(css)` and every golden CSS stay as they are except for the one new token
  (15 theme-wide tokens, 32 colours). `Token` gains `Description string` (the `$description`)
  and keeps `Field`, `CSS`, `Kind` (from `$type`: `dimension` → Length, `color` → Colour,
  `fontFamily` → Font, `shadow` → Shadow). `Field` is derived from the CSS name: strip `--`,
  then `al-`; split on `-`; the first segment stays, each later segment that starts with a
  letter is capitalised, one that starts with a digit is appended as is — `--chart-1` → `chart1`,
  `--radius-sm` → `radiusSm`, `--al-border-width` → `borderWidth`, today's names exactly (a test
  asserts the derived names equal the old literal's). Callers (`check`, `ir`, `lsp`,
  `generate`, `theme_ui`) switch from the variable to the call; any package-level use at init
  must become lazy.
- `Defaults()` is Zinc's resolved light and dark values converted to CSS text by the reader's
  value rules; it is what the configurator shows as a placeholder and what hover prints. The
  derived radii come back as plain lengths (`0.375rem`, `0.5rem`, `0.625rem`, `0.875rem`), not
  `calc()`: the declaration never held a `calc`, and `alder theme use zinc` writing them
  explicitly is the same look.
- **The guard is semantic, not textual** **(call)**: `TestTokens_MatchVendoredStylesheet`
  keeps asserting the *set* of names against `al.css`'s three blocks; `TestDefaults_EveryToken`
  asserts each default is *equivalent* to the stylesheet's value through a new
  `theme.Equivalent(kind Kind, a, b string) bool` — colours parsed with `ParseColour` and
  compared within 1/255 per channel (so `oklch(1 0 0 / 10%)` equals `rgb(255 255 255 / 0.1)`),
  lengths as number + unit (`0` equals `0px`), shadows layer by layer with the same rules,
  fonts as lists; the four `calc()` radii are evaluated at `--radius` for the comparison. The
  CSS importer's "dark value equals root's" check uses `Equivalent` too. Those two tests are the
  guard that alder-ui's `tokens.css` and its Zinc files did not drift between releases
  (alder-ui's own check guards the same on its side).
- Hover (`themeHover`) and the configurator's control labels show `Description`; completion
  items carry it as `detail`.
- `Removed`, `Suggest`, the kind checks and `LightSelector` are unchanged.

The hand-written `Tokens` literal is deleted, not kept as a fallback **(call)**: two sources
would drift, and the vendoring step is already the point where the table is allowed to change.

## Future: several named themes

Not built now; the following keeps it cheap later, and this phase must not break it:

- Theme tokens always land on `<html>` (`:root`/`.light`/`.dark` today, a `.theme-<name>`
  class alongside later), never on a nested element: alder-ui derives `--radius-*` with `calc()`
  at `:root`, so a class on `<body>` or an `al-page` would not re-derive. The configurator's
  preview already follows this.
- `theme.CSS(root, light, dark)` gains a scope parameter when needed (`""` today); the emitter
  must keep writing only custom properties under scheme selectors, never values into component
  rules.
- `Program.Theme *Theme` and `ThemeEdit`'s "the one declaration" become a list and a named
  lookup then; a `theme Paper { … }` syntax with a default is additive. Nothing in this phase
  should assume more than one *or* bake in exactly one beyond what phases 1–2 already do.
- Switching is server-driven (a session preference setting the class), like dark mode; no
  client state.

## Formatter, Spec, book

- No grammar or formatter change: `use` prints through `format.Theme` like import.
- `specs.go`: `theme use` (above); `new` gains the `theme` flag with its help text listing the
  preset names from `Presets()` at run time.
- Book: Appendix C gains **"Starting from a theme"** before "Changing a token" (the five, their
  purposes, by copy — "the app owns it from here", `alder theme use`, `--theme` on `new`, the
  picker), and its "Importing a theme" section gains the DTCG paragraph (what a `tokens.json`
  is, the resolver and single-file forms, what comes back unplaced); the licences line adds
  Catppuccin. Chapter 15's generator list gains `alder theme use`. Chapter 11's editor section
  mentions colour swatches. Site regenerated; the appendix test caps bumped as usual.
- alder-ui README: a "Themes" section (the directory layout, the token-name rule, how to add
  one, the check).

## Where things change

| layer | files |
|---|---|
| alder-ui | new `themes/{zinc,paper,catppuccin,brutal,pebble}/{theme.resolver,base.tokens,light.tokens,dark.tokens}.json`, `themes/catppuccin/LICENSE`; `tokens.css` (`--al-border-width`); every `components/al-*.css` and `typography.css` border rule; `test/check.py` (`themes` section, contrast over presets); `scripts/showcase.py` (theme select); README; tag v0.5.0 |
| vendoring | `scripts/vendor-al.sh` (copies `themes/` to `internal/theme/presets/`), `rt/al/*` at v0.5.0 |
| reader | new `internal/theme/dtcg.go`, `dtcg_test.go`, `testdata/dtcg/{resolver,single-file,aliases,foreign-names,bad-types}/…` |
| presets | new `internal/theme/presets.go`, `presets_test.go`, embedded `internal/theme/presets/**` |
| table | `internal/theme/theme.go` (`Tokens()` from Zinc, `Description`, `Defaults()` via the reader), callers in `internal/check`, `internal/ir`, `internal/lsp`, `internal/generate`, `cmd/alder/theme_ui*.go` |
| colour | new `internal/theme/colour.go`, `colour_test.go` |
| generate | `theme_import.go` (format detection, `--format`), new `theme_use.go` (`UsePreset`), `specs.go` |
| command | `cmd/alder/theme.go` (usage), `new.go` (`--theme`), `main.go` untouched |
| page | `theme_ui.go` (`presets` in `/state`), `theme_ui_assets.go` (Start from, swatches), tests |
| LSP | `protocol.go` (`colorProvider`), new `color.go`, `color_test.go`, `server.go` dispatch |
| book | `book/C-design-system.md`, `11-tooling.md`, `15-generators.md`, `book/site` |

`rt/` changes only by re-vendoring (the new token in `al.css`). `internal/gen` and the grammar
are untouched.

## Verification (tests named by behaviour)

- **alder-ui `check.py`**: each theme dir parses and declares exactly the token set; a theme
  missing `--sidebar-ring` fails naming it; a `$type: color` on `radius` fails; an unresolvable
  alias fails; Zinc equals `tokens.css` value for value; every preset's text pairings pass AA in
  light and dark (the five as shipped pass, after the builder's tuning); `build.py --check`
  still passes with `--al-border-width` in place; `parity.py` unchanged at the default.
- **Reader**: the resolver fixture places root/light/dark correctly and `ring: {color.primary}`
  becomes `var(--primary)`; a single file with `light`/`dark` groups places by group and a
  colour outside both goes to both schemes; a foreign name (`color.brand-500`) is unplaced with
  "not a token", a `_`/camelCase name normalises, a bare `width-narrow` finds `--al-width-narrow`;
  each colour space converts to the expected text and `alpha` appends `/ a`; a dimension `0`
  is `0`; a two-layer shadow with `inset` joins correctly; a type mismatch, a missing `$type`,
  a cycle of aliases, a `$ref` with `..`, and a 33-file resolver each fail as specified; a
  font array becomes a list and a bad family is unplaced with `FamilyOK`'s wording;
  `UnplacedCSS` output is valid CSS grouped by file.
- **Presets**: all five load with no unplaced and pass `ThemeText` (E0042-clean); the order is
  zinc then alphabetical; `Preset("papr")` is not found and `Suggest`-style help names `paper`.
- **Table**: `Tokens()` has every CSS name `al.css`'s three blocks declare and nothing else
  (the existing test, now against the file-built table); `Defaults()` equals the stylesheet's
  values; every token has a non-empty `Description`; `Order` matches file order; `Field` of
  `--al-border-width` is `borderWidth`.
- **Generate/CLI**: `alder theme use paper` on a temp scaffold writes the full declaration
  (byte-equal to `format.Theme` of the preset), replaces an existing one, `--print` writes to
  stdout only, `--dry-run` writes nothing, `--json` shape, no-arg lists five lines, unknown name
  exits 1 with the suggestion; `alder new x --theme pebble` creates the app with the declaration
  present and `alder build` passes; `--theme nope` creates nothing; `alder theme import
  theme.resolver.json` and `import x.tokens.json` go through the reader, `import -` with JSON
  on stdin works for a single file and names the resolver case; `--format` forces.
- **Page** (httptest): `/state` carries five presets; choosing one in the browser test
  (Playwright, as phase 2) refills every control, the gallery's `al-button` background is the
  preset's primary, `App.alder` is unchanged until Write, Reset restores the committed theme.
- **LSP**: `initialize` advertises `colorProvider`; `documentColor` on the blog's `App.alder`
  returns one entry per colour literal with the right ranges and `#ff0000` → `(1,0,0,1)`,
  `oklch(0.577 0.245 27.325)` ≈ `(0.906,0,0.043)` within 0.01, `var(--primary)` in `dark`
  resolves to the dark value; a non-colour field and a `fontSans` string yield nothing;
  `colorPresentation` for `(0,0.5,1,1)` returns `#0080ff` with an edit over the literal's
  contents; a file without a theme returns an empty list.
- `go test ./...` green, `alder build` of `examples/blog`, `go vet`, a visual pass of the five
  presets in the configurator and the alder-ui showcase in light and dark.

## Later (not this phase)

- `alder theme export [--dtcg]`: the app's theme as a resolver + three files (the exact inverse
  of the reader; `var()` → alias, `calc()`/`em` reported), so a custom theme can be contributed
  back as a preset or handed to a designer.
- Several named themes and an end-user switch (see above); a theme per Layout.
- Bundled serif (Newsreader or Source Serif 4, OFL) for Paper; a contrast readout in the page;
  `--merge`; a name-mapping step for designer files.

## Appendix A — starting values (from the brainstorm mockup)

The mockup rendered these over the real `al.css`; the user approved the five looks from it.
They are the starting point for the `themes/` files; the builder converts them to DTCG objects
(hex stays `srgb` with `hex`; `oklch(...)` becomes `{colorSpace: "oklch", components}`), fills
the tokens a block leaves out from Zinc's value for that scheme, sets the four derived radii
explicitly, and tunes lightness only where `check.py` fails AA. `--al-border-width` is `1px`
except Brutal. Zinc is `tokens.css` and is not repeated.

**Paper** — theme-wide: `--radius: .125rem; --radius-sm: .075rem; --radius-md: .1rem; --radius-lg: .125rem; --radius-xl: .175rem; --shadow-xs/sm/md/lg: 0 0 transparent; --font-sans: Charter, "Bitstream Charter", "Iowan Old Style", Georgia, serif` (`--font-mono` Zinc's).

```
light: --background: oklch(.975 .012 85); --foreground: oklch(.22 .02 60); --card: oklch(.99 .008 90); --card-foreground: oklch(.22 .02 60);
  --popover: oklch(.99 .008 90); --popover-foreground: oklch(.22 .02 60); --primary: oklch(.3 .03 50); --primary-foreground: oklch(.97 .012 85);
  --secondary: oklch(.93 .015 85); --secondary-foreground: oklch(.22 .02 60); --muted: oklch(.94 .012 85); --muted-foreground: oklch(.48 .02 60);
  --accent: oklch(.92 .03 70); --accent-foreground: oklch(.22 .02 60); --destructive: oklch(.5 .17 25); --destructive-foreground: oklch(.97 .01 85);
  --border: oklch(.85 .02 80); --input: oklch(.85 .02 80); --ring: oklch(.55 .05 50);
  --chart-1: oklch(.5 .12 40); --chart-2: oklch(.55 .08 140); --chart-3: oklch(.6 .1 70); --chart-4: oklch(.45 .07 250); --chart-5: oklch(.5 .1 350);
  --sidebar: oklch(.955 .012 85); --sidebar-foreground: oklch(.22 .02 60); --sidebar-primary: oklch(.3 .03 50); --sidebar-primary-foreground: oklch(.97 .012 85);
  --sidebar-accent: oklch(.905 .015 85); --sidebar-accent-foreground: oklch(.22 .02 60); --sidebar-border: oklch(.85 .02 80); --sidebar-ring: oklch(.55 .05 50)
dark:  --background: oklch(.21 .015 60); --foreground: oklch(.92 .02 85); --card: oklch(.25 .015 60); --card-foreground: oklch(.92 .02 85);
  --popover: oklch(.25 .015 60); --popover-foreground: oklch(.92 .02 85); --primary: oklch(.88 .03 80); --primary-foreground: oklch(.22 .02 60);
  --secondary: oklch(.3 .015 60); --secondary-foreground: oklch(.92 .02 85); --muted: oklch(.3 .015 60); --muted-foreground: oklch(.7 .02 80);
  --accent: oklch(.33 .025 60); --accent-foreground: oklch(.92 .02 85); --destructive: oklch(.62 .17 25); --destructive-foreground: oklch(.97 .01 85);
  --border: oklch(.34 .015 60); --input: oklch(.34 .015 60); --ring: oklch(.6 .03 70);
  --chart-1: oklch(.7 .12 40); --chart-2: oklch(.7 .08 140); --chart-3: oklch(.75 .1 70); --chart-4: oklch(.65 .07 250); --chart-5: oklch(.7 .1 350);
  --sidebar: oklch(.18 .015 60); --sidebar-foreground: oklch(.92 .02 85); --sidebar-primary: oklch(.88 .03 80); --sidebar-primary-foreground: oklch(.22 .02 60);
  --sidebar-accent: oklch(.27 .015 60); --sidebar-accent-foreground: oklch(.92 .02 85); --sidebar-border: oklch(.3 .015 60); --sidebar-ring: oklch(.6 .03 70)
```

**Catppuccin** (Latte / Mocha, official hex) — theme-wide: `--radius: .75rem; --radius-sm: .45rem; --radius-md: .6rem; --radius-lg: .75rem; --radius-xl: 1.05rem; --shadow-xs: 0 1px 2px rgb(76 79 105 / .06); --shadow-sm: 0 2px 6px rgb(76 79 105 / .10); --shadow-md: 0 6px 14px rgb(76 79 105 / .12); --shadow-lg: 0 12px 28px rgb(76 79 105 / .14)`; fonts Zinc's. (Shadows are theme-wide, so the mockup's darker Mocha shadows are dropped; the light ones stay **(call)**.)

```
light: --background: #eff1f5; --foreground: #4c4f69; --card: #ffffff; --card-foreground: #4c4f69; --popover: #ffffff; --popover-foreground: #4c4f69;
  --primary: #8839ef; --primary-foreground: #ffffff; --secondary: #ccd0da; --secondary-foreground: #4c4f69; --muted: #e6e9ef; --muted-foreground: #6c6f85;
  --accent: #dce0e8; --accent-foreground: #4c4f69; --destructive: #d20f39; --destructive-foreground: #ffffff; --border: #bcc0cc; --input: #bcc0cc; --ring: #8839ef;
  --chart-1: #1e66f5; --chart-2: #40a02b; --chart-3: #df8e1d; --chart-4: #fe640b; --chart-5: #ea76cb;
  --sidebar: #e6e9ef; --sidebar-foreground: #4c4f69; --sidebar-primary: #8839ef; --sidebar-primary-foreground: #ffffff; --sidebar-accent: #ccd0da;
  --sidebar-accent-foreground: #4c4f69; --sidebar-border: #bcc0cc; --sidebar-ring: #8839ef
dark:  --background: #1e1e2e; --foreground: #cdd6f4; --card: #313244; --card-foreground: #cdd6f4; --popover: #313244; --popover-foreground: #cdd6f4;
  --primary: #cba6f7; --primary-foreground: #1e1e2e; --secondary: #45475a; --secondary-foreground: #cdd6f4; --muted: #313244; --muted-foreground: #a6adc8;
  --accent: #45475a; --accent-foreground: #cdd6f4; --destructive: #f38ba8; --destructive-foreground: #1e1e2e; --border: #45475a; --input: #45475a; --ring: #cba6f7;
  --chart-1: #89b4fa; --chart-2: #a6e3a1; --chart-3: #f9e2af; --chart-4: #fab387; --chart-5: #f5c2e7;
  --sidebar: #181825; --sidebar-foreground: #cdd6f4; --sidebar-primary: #cba6f7; --sidebar-primary-foreground: #1e1e2e; --sidebar-accent: #313244;
  --sidebar-accent-foreground: #cdd6f4; --sidebar-border: #45475a; --sidebar-ring: #cba6f7
```

**Brutal** — theme-wide: `--radius: 0` and the four derived `0`; `--al-border-width: 2px`; `--shadow-xs: 2px 2px 0 0 var(--border); --shadow-sm: 3px 3px 0 0 var(--border); --shadow-md: 5px 5px 0 0 var(--border); --shadow-lg: 8px 8px 0 0 var(--border)`; fonts Zinc's.

```
light: --background: #ffffff; --foreground: #000000; --card: #ffffff; --card-foreground: #000000; --popover: #ffffff; --popover-foreground: #000000;
  --primary: oklch(.9 .18 100); --primary-foreground: #000000; --secondary: #ffffff; --secondary-foreground: #000000; --muted: oklch(.95 0 0); --muted-foreground: oklch(.35 0 0);
  --accent: oklch(.8 .2 350); --accent-foreground: #000000; --destructive: oklch(.6 .24 28); --destructive-foreground: #ffffff; --border: #000000; --input: #000000; --ring: #000000;
  --chart-1: oklch(.9 .18 100); --chart-2: oklch(.8 .2 350); --chart-3: oklch(.7 .2 150); --chart-4: oklch(.6 .22 260); --chart-5: oklch(.7 .2 40);
  --sidebar: #ffffff; --sidebar-foreground: #000000; --sidebar-primary: #000000; --sidebar-primary-foreground: #ffffff; --sidebar-accent: oklch(.9 .18 100);
  --sidebar-accent-foreground: #000000; --sidebar-border: #000000; --sidebar-ring: #000000
dark:  --background: #000000; --foreground: #ffffff; --card: #000000; --card-foreground: #ffffff; --popover: #000000; --popover-foreground: #ffffff;
  --primary: oklch(.9 .18 100); --primary-foreground: #000000; --secondary: #000000; --secondary-foreground: #ffffff; --muted: oklch(.2 0 0); --muted-foreground: oklch(.75 0 0);
  --accent: oklch(.8 .2 350); --accent-foreground: #000000; --destructive: oklch(.65 .24 28); --destructive-foreground: #000000; --border: #ffffff; --input: #ffffff; --ring: #ffffff;
  --chart-1: oklch(.9 .18 100); --chart-2: oklch(.8 .2 350); --chart-3: oklch(.75 .2 150); --chart-4: oklch(.7 .2 260); --chart-5: oklch(.75 .2 40);
  --sidebar: #000000; --sidebar-foreground: #ffffff; --sidebar-primary: #ffffff; --sidebar-primary-foreground: #000000; --sidebar-accent: oklch(.9 .18 100);
  --sidebar-accent-foreground: #000000; --sidebar-border: #ffffff; --sidebar-ring: #ffffff
```

**Pebble** — theme-wide: `--radius: 1.25rem; --radius-sm: .75rem; --radius-md: 1rem; --radius-lg: 1.25rem; --radius-xl: 1.75rem; --shadow-xs: 0 1px 4px rgb(40 50 80 / .04); --shadow-sm: 0 4px 16px rgb(40 50 80 / .06); --shadow-md: 0 8px 28px rgb(40 50 80 / .08); --shadow-lg: 0 16px 48px rgb(40 50 80 / .1)`; fonts Zinc's.

```
light: --background: oklch(.985 .006 250); --foreground: oklch(.28 .03 260); --card: #ffffff; --card-foreground: oklch(.28 .03 260); --popover: #ffffff; --popover-foreground: oklch(.28 .03 260);
  --primary: oklch(.68 .17 35); --primary-foreground: #ffffff; --secondary: oklch(.94 .03 35); --secondary-foreground: oklch(.4 .08 35); --muted: oklch(.955 .008 250); --muted-foreground: oklch(.52 .02 260);
  --accent: oklch(.93 .04 190); --accent-foreground: oklch(.35 .06 190); --destructive: oklch(.6 .2 25); --destructive-foreground: #ffffff; --border: oklch(.91 .01 250); --input: oklch(.91 .01 250); --ring: oklch(.68 .17 35);
  --chart-1: oklch(.68 .17 35); --chart-2: oklch(.7 .12 190); --chart-3: oklch(.8 .14 90); --chart-4: oklch(.65 .15 300); --chart-5: oklch(.72 .14 150);
  --sidebar: oklch(.965 .01 250); --sidebar-foreground: oklch(.28 .03 260); --sidebar-primary: oklch(.68 .17 35); --sidebar-primary-foreground: #ffffff; --sidebar-accent: oklch(.93 .04 190);
  --sidebar-accent-foreground: oklch(.35 .06 190); --sidebar-border: oklch(.91 .01 250); --sidebar-ring: oklch(.68 .17 35)
dark:  --background: oklch(.22 .02 260); --foreground: oklch(.95 .01 260); --card: oklch(.27 .02 260); --card-foreground: oklch(.95 .01 260); --popover: oklch(.27 .02 260); --popover-foreground: oklch(.95 .01 260);
  --primary: oklch(.75 .15 35); --primary-foreground: oklch(.2 .03 35); --secondary: oklch(.32 .02 260); --secondary-foreground: oklch(.95 .01 260); --muted: oklch(.3 .02 260); --muted-foreground: oklch(.72 .015 260);
  --accent: oklch(.34 .04 190); --accent-foreground: oklch(.9 .05 190); --destructive: oklch(.68 .18 25); --destructive-foreground: oklch(.2 .03 25); --border: oklch(.34 .02 260); --input: oklch(.34 .02 260); --ring: oklch(.75 .15 35);
  --chart-1: oklch(.75 .15 35); --chart-2: oklch(.75 .12 190); --chart-3: oklch(.85 .14 90); --chart-4: oklch(.72 .15 300); --chart-5: oklch(.78 .14 150);
  --sidebar: oklch(.19 .02 260); --sidebar-foreground: oklch(.95 .01 260); --sidebar-primary: oklch(.75 .15 35); --sidebar-primary-foreground: oklch(.2 .03 35); --sidebar-accent: oklch(.34 .04 190);
  --sidebar-accent-foreground: oklch(.9 .05 190); --sidebar-border: oklch(.3 .02 260); --sidebar-ring: oklch(.75 .15 35)
```

Known AA risks for the builder's tuning (rule in "The five starting themes"): Pebble's coral
primary with white text (likely under 4.5:1 — the rule says dark `primary-foreground` first),
Brutal's `muted-foreground` in dark, Catppuccin Latte's `muted-foreground` on `muted` (shadcn's
own pairing is also marginal).
The mockup itself is kept at `.superpowers/brainstorm/86143-1793462888/content/presets.html`
(git-ignored) and can be opened against `rt/al/al.css` to compare.
