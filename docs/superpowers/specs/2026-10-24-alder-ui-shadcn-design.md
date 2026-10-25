# alder-ui looks like shadcn out of the box — design

Issue: tapegram/alder#21. Date: 2026-10-24. Status: decisions approved by the user in
conversation ("Yes sounds good" to the five recommendations below), then amended the
same day by four rulings (section 5.3: breaking changes accepted, "I'm ok with breaking
changes that require apps to manually update. There is only one app at the moment.");
this is the spec a plan argues from. Companion: `2026-10-24-alder-ui-shadcn-reference.md` (the shadcn token
sheet, resolved Tailwind values and verbatim class recipes). Related: the M7a spec
(`2026-10-12-m7a-elements-design.md`: the manifest, the checker, the vendor flow), the
book's Appendix C (`book/C-design-system.md`), memory note "shadcn web components idea".

Two repositories are involved. Unless a section says otherwise, **"alder-ui" means
`github.com/tapegram/alder-ui`** (public, Apache-2.0, Python-only build; its `al.css`,
`al.js` and `custom-elements.json` are vendored into this repository at a tag by
`scripts/vendor-al.sh`) and **"Alder" means this repository**.

## 1. Intent

alder-ui is Alder's built-in design system: light-DOM custom elements (`<al-button>`,
`<al-card>` …) styled by one stylesheet, using shadcn/ui's token names so a shadcn theme
can be pasted in. Today it is shadcn-named but not shadcn-looking: the tokens are the
old slate HSL values, nothing between the components is styled, radii, shadows,
heights, borders and focus rings are all off from shadcn's, alerts are coloured
Bootstrap boxes, form controls show the OS look, and there is no motion. The user's
verdict on a real app (nightvote-replay): "All of it, it's very ugly."

The intent is that an Alder app, with no CSS of its own, is indistinguishable from a
shadcn/ui (new-york-v4, Tailwind v4 era) page using the same components, in light and
dark; and that a theme exported from ui.shadcn.com/themes or tweakcn pastes into the
app's stylesheet unchanged and re-themes everything. In the user's words: "I basically
want shadcn as web components." Full component parity is not asked for yet: "We don't
need full parity with all components YET."

Why it matters: Alder is written mostly by agents and read by people; a fresh app that
looks finished is the point of a built-in system (M7a's goal), the generated admin
(M12) is built from these elements, and the user does not want to maintain a React
toolchain again to get shadcn's look.

## 2. Goals

1. Every element alder-ui ships today (22, listed in section 6) renders as its shadcn
   counterpart's recipe, rule for rule, in light and dark.
2. shadcn's token sheet, verbatim (oklch, `:root` and `.dark`, including `chart-*` and
   `sidebar-*`, `--radius` 0.625rem with the derived `--radius-sm/md/lg/xl`), so a
   pasted theme wins without edits.
3. Raw HTML between the components (`h1`–`h4`, `p`, `ul`, `code`, `a`, `table`, `hr`) looks
   like shadcn's Typography page by default, so a page written with elements plus flow
   content looks designed with no app CSS.
4. The element API (names, attributes, values, slots, the markup Alder emits) does not
   change. Additions only. The CSS surface does change, deliberately: the old `--al-*`
   tokens are removed and dark mode needs the theme script in the page (section 5.3).
   Alder's own examples, scaffold and generator output are updated in the same change;
   nightvote-replay, the one external app, is migrated by hand with the guide in section 10.
5. Parity is measured, not eyeballed: a committed shadcn reference grid, a
   computed-style diff that gates CI in alder-ui, and before/after screenshots for
   review.
6. The Alder book's Appendix C stays the library's documentation and stays true
   (`book/appendix_test.go` holds it to the manifest).

## 3. Non-goals (this pass)

- New elements: `al-separator`, `al-skeleton`, `al-switch`, `al-radio-group`,
  `al-avatar`, `al-progress`, `al-breadcrumb`, `al-accordion`, `al-sheet`, `al-icon`,
  sidebar, charts. (Decision 5 below. `switch` and `radio` also need the renderer's
  form-wrapping; see section 9.) A bare `<hr>` is styled by the base layer meanwhile.
- A custom listbox for `al-select` (the native popup stays; `color-scheme` keeps it
  matching the theme). `appearance: base-select` is a later enhancement.
- A theme configurator or token editor (a later idea; this spec only fixes the token
  format it would write: shadcn's `:root`/`.dark` blocks).
- Datastar, animations beyond shadcn's enter/exit fades, right-to-left review, print
  styles, container queries in cards (`@container/card-header` is dropped; the
  `action` slot uses a plain grid).
- Shadow DOM, `::part`, properties as API, events beyond the existing `al-close` and
  `al-select`.

## 4. Anti-goals

- **Not** "inspired by shadcn": where shadcn and alder-ui differ, alder-ui is wrong,
  unless the deviation is listed in the parity allow-list (section 8) with a reason.
- **No Node toolchain in alder-ui's build or in Alder.** Tailwind is used only as the
  oracle that renders the reference grid, in a dev-only directory, regenerated by hand.
  `al.css` stays hand-written, readable CSS.
- **No per-element `--al-*` theming surface** that duplicates shadcn's. Spacing and
  type sizes are Tailwind's fixed scale written as literal rem values; theming is done
  through shadcn's tokens (colours, `--radius`, fonts) and the `--shadow-*` tokens.
- **No breaking the compiler's contract**: no renamed or removed attribute, value, slot
  or emitted markup; no class names Alder must emit beyond today's `help`, `errors`,
  `_alder-click`. (The stylesheet's custom properties are not part of that contract: the
  old `--al-*` tokens go, section 5.3.)

## 5. Decisions

### 5.1 Made by the user (2026-10-24; recommended by the design lead, accepted as stated)

1. **Dark mode is the `.dark` class on `<html>`** (shadcn's convention), so theme
   exports paste in unchanged. There is no `[data-theme]` alias (ruling 2, section 5.3). The CSS-only `prefers-color-scheme`
   auto-dark block is dropped: with a pasted theme it is a trap (the pasted, unlayered
   `:root` beats the layered OS-dark block, so an OS-dark user would get light tokens
   on whatever the page assumed). Following the OS is done the way shadcn apps do it: a
   one-line blocking script in `<head>` that sets `.dark` from `localStorage.theme`
   or `matchMedia("(prefers-color-scheme: dark)")` before first paint. alder-ui ships
   the line in its README and demos; `alder new`'s Layout and the generators emit it
   (section 9); the runtime injects nothing. Without JavaScript a page is light, as a
   shadcn page is.
2. **Fidelity over non-text AA.** shadcn's `--border`/`--input` (oklch 0.922 on white)
   and `--ring` fail WCAG's 3:1 for control edges; alder-ui had darkened them. Ship
   shadcn's exact values. In `test/check.py`, text-pairing contrast (4.5:1) stays a hard
   failure; border/ring contrast becomes a reported warning. Appendix C documents a
   two-line override (`--input`, `--border`) for strict AA.
3. **Base layer on by default**: a Preflight-equivalent reset plus shadcn's Typography
   recipe for flow content inside `al-page`. New attribute `al-page plain` (boolean)
   switches the typography off for a page that brings its own. The reset itself is
   not switchable.
4. **Bundle Geist and Geist Mono** (SIL OFL 1.1) as variable woff2, self-hosted under
   `/static/al/`, as `--font-sans`/`--font-mono` with `system-ui`/`ui-monospace`
   fallbacks. Overriding the font is one token.
5. **Scope: restyle the 22 existing elements with additive API only**; defer new
   elements (section 3) to a second pass.

### 5.2 Made by the design lead (mine; the builder may revisit with a note in the plan)

- **Alerts and toasts lose their coloured backgrounds.** The variant values
  `info|success|warning|destructive` stay (the checker's closed union, the API table in
  check.py), but `info`, `success`, `warning` all render as shadcn's `default` alert
  (card surface, border); the variant drives only the leading icon and, for
  `destructive`, the text tint. `al-toast` follows sonner: popover surface, border,
  icon per variant. The `--al-*-soft` tokens are removed.
- **Translation convention**: each component's CSS carries the shadcn class string it
  implements as a comment above the rule, so a future diff against shadcn is
  mechanical. Translation rules:
  - `bg-x/NN` → `color-mix(in oklab, var(--x) NN%, transparent)` (Tailwind v4 mixes in
    oklab); `dark:` → a rule under `.dark`.
  - `ring-[3px] ring-x/50` → `box-shadow: 0 0 0 3px color-mix(in oklab, var(--x) 50%, transparent)`;
    where `shadow-xs` also applies, the two are comma-joined.
  - `shadow-xs/sm/md/lg` → `var(--shadow-xs|sm|md|lg)`; `rounded-sm/md/lg/xl` →
    `var(--radius-sm|md|lg|xl)`; `text-xs/sm/base/lg` → the size/line-height pairs in
    the reference, literal.
  - Spacing (`h-9`, `px-4`, `gap-2`, `p-[3px]`) → literal rem/px values.
  - `[&_svg:not([class*='size-'])]:size-4` → `al-x svg { width: 1rem; height: 1rem; flex: none; pointer-events: none }`.
  - Radix `data-[state=…]` → the platform's state: `:checked`, `:indeterminate`,
    `:popover-open`, `[open]`, `:focus-visible`, `[aria-selected]`, `[invalid]`/`[aria-invalid]`,
    `:disabled`/`[disabled]`.
  - `transition-all`/`transition-[color,box-shadow]` → the named properties, 150ms,
    `cubic-bezier(.4, 0, .2, 1)`; enter/exit animations → `@starting-style` +
    `transition-behavior: allow-discrete` on `[open]`/`:popover-open` (150ms; dialog
    200ms), under `@media (prefers-reduced-motion: no-preference)`.
- **Token file layout** (`tokens.css`): shadcn's `:root` block verbatim; shadcn's
  `.dark` block verbatim under `.dark`; a `.light`
  block re-declaring the light values (so a dark subtree can hold a light card);
  `color-scheme: light|dark` with each; then one `:root` block of the Tailwind-emitted
  scale written out (`--radius-sm/md/lg/xl`, `--shadow-xs/sm/md/lg`, `--font-sans`,
  `--font-mono`) and `--destructive-foreground` (the docs site's value); then alder-ui's
  own tokens, `--al-` prefixed, reduced to the page widths
  (`--al-width-narrow|normal|wide`). `--al-space-*`, `--al-text-*`, `--al-leading*`,
  `--al-weight-*`, `--al-control-h-*`, `--al-radius-*`, `--al-ring-*`, `--al-shadow-sm`,
  `--al-font-*`, `--al-destructive-text` and the `--al-*-soft/border` tokens are
  removed outright, with no alias block (section 5.3). `test/check.py`'s tokens-only rule becomes: no literal colours or font
  families in `components/`; lengths are allowed.
- **Icons** as CSS: the lucide glyphs in the reference (section 4) as inline SVG data
  URIs applied with `mask-image` on `::before`/`::after`, coloured by `currentColor`,
  in `components/icons.css`. No `<al-icon>` yet; an inline `<svg>` written by the app is
  sized by the `svg` rule above.
- **Base layer scope**: Preflight is global (inside `@layer al`, so app CSS still wins);
  the Typography recipe is scoped to `al-page:not([plain])` and components re-scale
  what they contain: an unslotted `h2`/`h3` first in `al-card` renders as CardTitle and a
  `p` directly after it as CardDescription; `al-dialog`, `al-alert`, `al-menu`,
  `al-popover`, `al-toast` reset headings/paragraphs/lists inside them to their own
  recipes; a bare `<table>` anywhere in `al-page` gets the shadcn table recipe, so
  `al-table` becomes a thin wrapper (`overflow-x: auto`, `striped`, `dense`).
- **Versioning**: this lands as alder-ui `v0.4.0`. The vendored version today is
  `v0.3.3`; under pre-1.0 semver a breaking change bumps the minor (0.3.x to 0.4.0), so
  `v0.4.0` is the breaking release, not merely an additive one (token removal, dark mode
  by script, visual change). The release notes say so and carry the migration steps.
  Alder vendors it once at the end of the alder-ui work; the early look after phase 1
  is section 5.3 ruling 4b. There is no `v0.5.0` alias-removal release.

### 5.3 Rulings after the first approval (2026-10-24; the user: "I'm ok with breaking changes that require apps to manually update. There is only one app at the moment.")

1. **Old `--al-*` tokens are removed outright, no alias block.** This is a breaking
   alder-ui release (`v0.4.0`, pre-1.0 breaking bump, section 5.2). Alder updates its
   own uses in the same change (section 9). The overlays `rt/web` injects are hardened
   against the global Preflight/base layer (section 9).
2. **Dark mode has no runtime injection.** `alder new`'s Layout and the generators
   (including `internal/generate/admin.go`) emit the theme script; generator goldens
   change accordingly. Existing apps add it by hand. The one-release `[data-theme]`
   alias from 5.1 decision 1 is dropped too: the user accepted it earlier only as part
   of a recommendation and now prefers breaking changes, so `.dark` / `.light` are the
   only selectors.
3. **"Zero source changes" is no longer the acceptance bar** for nightvote-replay. It
   is migrated by hand (old tokens replaced, theme script added to its Layout) and a
   short migration guide lists the steps (section 10; Appendix C carries the same list).
4. **Controller's calls** (the controller may revisit with a note in the plan):
   a. **Parity gate scope (controller's call).** The gate tests Alder's emitted markup.
      The builder may add allow-list entries for structural gaps on their own (a label
      wrapping the input, a native checkbox versus a button, display/layout
      differences), provided each entry is in the committed `test/parity-allow.json`
      with a one-line reason and is reviewed with the work. Colour, spacing, radius,
      type and shadow values must still match exactly.
   b. **Early look (controller's call).** After phase 1 the look is vendored from a
      pre-release tag (`v0.4.0-rc.1`) on the Alder branch only. Alder main never ships a
      mixed old/new look. `vendor-al.sh`'s tag == manifest-version rule is satisfied by
      setting the manifest `version` to `v0.4.0-rc.1` in the rc; the final tag resets it
      to `v0.4.0`.

## 6. Constraints

- **The element API is stable** (the CSS custom properties are not; section 5.3).
  `custom-elements.json` names, attribute types
  (closed unions), defaults and slots may only gain entries. Upstream `test/check.py`
  holds an `API` table that fails on a rename or drop; its `ADDED` list takes the
  additions below. Alder's checker (`internal/elements`, E0028) accepts new
  attributes/values/slots the moment the manifest is re-vendored; the LSP completes
  from the same manifest.
- **The markup Alder emits is fixed** (`internal/ir/lower_html.go`, `completeBuiltin`,
  `completeDialog`): `al-input`/`al-textarea`/`al-select`/`al-checkbox` wrap
  `<label>Text<control></label>` + `<small class="help">` + `<ul class="errors">`;
  `al-button` wraps `<button>`/`<a>` (and `form._alder-click` for click messages);
  `al-dialog` wraps `<dialog>` with `<form method="dialog">` close + `<h2 slot="header">`;
  `al-alert` gets `role`; `al-toasts` gets `popover="manual"`. The CSS must style exactly
  these shapes (and the hand-written shapes the book shows), as today.
- **Separate repository, vendored at a tag**: `scripts/vendor-al.sh <tag>` copies
  `al.css`, `al.js`, `custom-elements.json` into `rt/al/` and writes `VERSION`; a test
  checks `VERSION` against the manifest's `version`; `rt/al/al.go` embeds and ETags
  them; `rt/web/router.go` serves `/static/al/*` with `DesignSystemCache`;
  `internal/buildkit.WriteDesignSystem` copies `DesignSystemFiles` into every build.
  Fonts join that flow (section 9).
- **alder-ui stays Python-stdlib for build and test** (`scripts/build.py`,
  `test/check.py`); the parity tooling may add Playwright for Python as a dev
  dependency, and Node only inside `ref/` for regenerating the reference.
- **`@layer al`** wraps everything alder-ui ships, including the new base layer, so an
  app's unlayered CSS always wins.
- **No JavaScript required** for the first tier remains true; the theme script is
  optional (light without it); `al.js` scripts stay at most 60 lines each
  (check.py).
- **Browser support** as today: custom properties, `@layer`, `color-mix()`, `:has()`,
  anchor positioning and invoker commands with the existing shim. New: `oklch()`
  (Baseline 2023), `@starting-style` and `transition-behavior` (Baseline 2024; without
  them overlays appear without motion, which is acceptable), `field-sizing: content`
  (progressive; `min-height` carries the fallback), `mask-image` (Baseline 2023).

## 7. The design, per element

The recipes are in the reference file, section 3; this table says what each alder-ui
element becomes and what is added to its API. "Same" means every rule is a translation
of the recipe with no deviation.

| element | shadcn recipe | API additions | notes |
|---|---|---|---|
| `al-button` | button, 6 variants, sizes | `variant` gains `link`; `size` gains `xs`, `icon`, `icon-sm`, `icon-lg` | border only on `outline`; `has(> svg)` padding; element-level `[disabled]` dimming kept; presentational form (no inner button) keeps drawing the look |
| `al-input`, `al-textarea` | label + input / textarea; field = `grid gap-2` | — | label `text-sm leading-none font-medium`; `.help` = description; `ul.errors` = `text-sm text-destructive font-medium`; `[invalid]` = `aria-invalid` recipe; textarea `min-h-16 field-sizing-content` |
| `al-select` | select trigger (`h-9`, `size=sm` later) | — | `appearance: none`, masked chevron-down `opacity-50` at `right-3`, `pr-8`; native listbox |
| `al-checkbox` | checkbox | — | `appearance: none; 1rem; rounded 4px; border-input; shadow-xs; :checked bg-primary border-primary` + masked check (`size-3.5`), `:indeterminate` minus; label row `gap-2`, box first (today's `row-reverse` stays) |
| `al-card` | card + parts | slots `description`, `action` | `py-6 gap-6 rounded-xl shadow-sm`; `header` = CardTitle; `description` = CardDescription; `action` = CardAction (header grid `1fr auto`); default children = CardContent (`px-6`); `footer` = CardFooter; `actions` = CardFooter with `justify-content: flex-end; gap-2`; an unslotted leading `h2`/`h3` (+ following `p`) is styled as title (+ description) — mine |
| `al-badge` | badge | `variant` unchanged (shadcn's `ghost`/`link` not added) | `font-medium`, `border-transparent`; `al-badge > a` gets `[a&]:hover` |
| `al-alert` | alert | — | `rounded-lg px-4 py-3 grid` with icon column (amended: block flow, A3); `heading` → AlertTitle; body → AlertDescription; variants per 5.2; dismiss button = ghost icon `size-7` at top-right (lucide x) |
| `al-table` | table | — | outer frame and radius removed; `striped` → `tbody tr:nth-child(even) { background: muted/50 }`; `dense` → `h-8`/`p-1`; `caption-bottom mt-4` |
| `al-pagination` | pagination | — | `prev`/`next` = ghost buttons `gap-1 px-2.5` (amended: `px-3`, A2) with masked chevrons; the generated "Page n of m" and unslotted children sit in `text-sm text-muted-foreground`; unslotted `a` children are ghost `size-9` links, `[aria-current=page]` outline |
| `al-stack` | — | — | gaps re-pinned: sm 0.5rem, md 1rem, lg 1.5rem (Tailwind 2/4/6) |
| `al-page` | — | `plain` (boolean) | `py-8 px-4`, `px-6` from 40rem; typography scope per 5.2 |
| `al-dialog` | dialog | slot `description` | `<dialog>`: `bg-background p-6 grid gap-4 rounded-lg shadow-lg w-[calc(100%-2rem)] max-w-lg`; `::backdrop` `rgb(0 0 0 / .5)`; `header` = Title (`text-lg leading-none font-semibold`), `description` = Description, `actions` = Footer; `form[method=dialog] > button` = Close (masked x, `top-4 right-4 opacity-70`); `size` sm/md/lg = `max-w-md/lg/2xl`; fade+zoom-95 200ms in and out |
| `al-popover` | popover content | — | `w-72 p-4 rounded-md shadow-md`; fade+zoom+slide 0.5rem from the anchored side; `offset` sm/md/lg = 4/8/16 px |
| `al-menu` | dropdown-menu | — | `min-w-[8rem] p-1 rounded-md shadow-md`; `a`/`button` items `gap-2 rounded-sm px-2 py-1.5 text-sm`, `:hover`/`:focus-visible` = `focus:bg-accent`; `al-menu > hr` = Separator; `[data-variant=destructive]` honoured as a `data-` attribute (free in the checker) — mine; submenu/label/shortcut later |
| `al-tabs`, `al-tab`, `al-panel` | tabs | `al-tabs variant`: `default` \| `line` | both the no-JS radio labels and the JS-built tablist get the recipe: list `bg-muted rounded-lg p-[3px] h-9`, trigger `rounded-md px-2 py-1 text-sm font-medium text-foreground/60`, active `bg-background text-foreground shadow-sm` (dark `bg-input/30 border-input`); `line` = `bg-transparent gap-1`, active underline `h-0.5 bg-foreground`; panel `flex-1` |
| `al-toast`, `al-toasts` | sonner | — | toast `w-[356px] p-4 rounded-[var(--radius)] border bg-popover text-popover-foreground shadow-[0_4px_12px_rgb(0_0_0/.1)]`, icon `size-4` per variant, title 13px/500; region offsets 32px (amended: 24px, A1), gap 14px; slide-in from the edge |
| `al-tooltip` | tooltip | — | `px-3 py-1.5 rounded-md text-xs bg-foreground text-background text-balance`; arrow = `::after` 10px square rotated 45° on the anchored side; fade+zoom |
| `al-devpanel` | — (Alder's own) | — | re-pinned to the new tokens; mono = `--font-mono`; otherwise unchanged |

**Amendments (2026-10-25, alder-ui v0.4.0 build).** The committed shadcn reference
build (alder-ui `ref/`) is the measurable authority; where it disagrees with this table,
it wins. The table keeps the original values for history; do not reinstate them:

- **A1, `al-toasts` offset: 24px, not 32px.** Sonner's `VIEWPORT_OFFSET` is 24px and the
  oracle measures 24px (ruling R4). Sonner's 16px offset below 600px is not adopted.
- **A2, `al-pagination` prev/next padding: `px-3` (0.75rem), not `px-2.5`.** The oracle
  computes 12px: `has-[>svg]:px-3` beats `px-2.5` on the links that carry a chevron
  (ruling R17).
- **A3, `al-alert` is block flow, not a grid.** In a grid every inline child (`a`,
  `code`, `strong`) and every text run between them becomes its own row. The icon is an
  absolute `::after` in a `calc(1rem + 1rem + 0.75rem)` start padding (px-4 + size-4 +
  gap-x-3); block children (`p`, `div`, `ul`, `ol`) are the description with gap-y-0.5 as
  a 2px top margin (final review I1, the same change `al-toast` took).

Base layer (new files `base.css` and `typography.css`, first in the bundle after
tokens): Tailwind v4 Preflight translated (border-box; zero margins; `* { border: 0 solid
var(--border) }`; headings/lists/`hr`/`pre` reset; `button, input, select, textarea
{ font: inherit; color: inherit; background: transparent }`; `img, svg, video { display:
block; vertical-align: middle }`; `a { color: inherit; text-decoration: inherit }`;
`::placeholder` muted; `body { background: var(--background); color: var(--foreground);
font-family: var(--font-sans); line-height: 1.5; -webkit-font-smoothing: antialiased;
font-synthesis-weight: none }`; `:focus-visible` default `outline-color: ring/50`), then
the Typography recipe (reference section 3, "typography") under `al-page:not([plain])`,
with `hr` as Separator, and the shadcn **table** recipe (not the Typography page's
bordered table) for a bare `<table>`.

## 8. Verification

All of this lives in alder-ui except the last item.

1. **Reference grid** `ref/`: a Vite + React + shadcn (new-york-v4, Tailwind v4,
   neutral theme, Geist) app, dev-only, rendering every component in every variant and
   state in a fixed grid with fixed text and a `data-ref="button/outline/sm"` marker on
   each control, in a light and a dark copy (`.dark` on `<html>`). Its **built static
   output** (`ref/dist/`) is committed so CI and the user need only a browser;
   `ref/README.md` says how to regenerate it when shadcn moves (`npx shadcn@latest add
   …`, `npm run build`).
2. **Twin page** `demo/parity.html`: the same grid with alder-ui elements, same text,
   same markers.
3. **Computed-style diff, the gate**: `test/parity.py` (Playwright for Python,
   Chromium) loads both pages at 1280×2000, light and dark, and for each marker
   compares the control's `getComputedStyle` on a fixed property list (color,
   background-color, border-{width,style,color,radius}, box-shadow, font-family,
   font-size, font-weight, line-height, letter-spacing, padding, height, width for
   fixed-size controls, gap, opacity, outline, text-decoration) plus `getBoundingClientRect`
   size (±0.5px), in rest, hover, focus-visible and disabled states (driven by
   Playwright). Output: a per-component table of differing properties and values.
   Any difference not in `test/parity-allow.json` fails CI. The twin page uses the
   markup Alder emits, so structural gaps (the label wrapping the input, a native
   checkbox versus shadcn's button, display/layout differences) are allow-listed by the
   builder on their own, each entry with a one-line reason, reviewed with the work;
   colour, spacing, radius, type and shadow values must still match exactly
   (controller's call). The allow-list holds deliberate deviations, each with a reason: the native `<select>` listbox; the
   radio-backed tab labels when JavaScript is off; `font-family` on machines without
   Geist installed only if the woff2 failed to load (the script asserts it loaded);
   the alert/toast variants shadcn lacks (compared against `default`).
4. **Screenshot triptychs, the review**: `test/parity.py --shots` writes, per component
   and theme, reference | alder-ui | pixel diff to `ref/out/` (pixel diff via Pillow,
   anti-aliasing tolerance), attached to each PR. Not a gate.
5. **check.py** keeps: manifest API (with ADDED), selectors name declared things, no
   literal colours/fonts in components, every `var(--x)` defined, text AA hard /
   border AA warning, demos cover every variant and link `../al.css`, bundle equals
   build output.
6. **Alder (this repository)**: after vendoring, `go test ./...` (the generator and
   scaffold goldens change for the theme script and are regenerated and reviewed; the
   `rt/web` production-page golden changes with the `--al-primary` replacement;
   `book/appendix_test.go` after the Appendix C rewrite; `cmd/alder` build/run/new tests
   extended for the font files); a grep that no `--al-` token other than
   `--al-width-*` remains outside the vendored `rt/al/`; `alder build` + `alder run` of
   `examples/*` and a visual pass over them, the dev panel, error strip and pending bar
   included, against the new base layer; and the nightvote-replay check (section 10).

## 9. Alder-side changes (this repository)

- `scripts/vendor-al.sh`: copy `fonts/Geist[wght].woff2` and `fonts/GeistMono[wght].woff2`
  (names per upstream) as well; keep the version check.
- `rt/al/al.go`: `Names` and the `//go:embed` gain the two font files; the ETag table
  covers them. `rt/embed.go` (`rt.Files`, which `internal/elements` reads the manifest
  from) embeds `al/*` likewise — the builder checks its pattern.
- `internal/buildkit.DesignSystemFiles` gains the two files (every build writes them to
  `static/al/`).
- `rt/web/router.go`: `/static/al/<font>` is served from the embedded copy by the
  existing branch; the builder verifies `Content-Type` (`http.ServeContent` infers from
  the extension and Go's built-in table may not know `.woff2`; if not, register
  `font/woff2` with `mime.AddExtensionType` or set the header). Fonts may take
  `DesignSystemCache` as well; immutable caching is not possible without a versioned URL
  (same reasoning as the stylesheet).
- `alder new` scaffold (Layout template; `cmd/alder/new_test.go`): add the one-line theme
  script to `<head>` before the stylesheet link. Keep linking `al.css` and `al.js`.
- Generators: every generator that emits a layout or page shell emits the same theme
  script, including `internal/generate/admin.go` (the admin layout it writes when the
  app has none; where it reuses the app's `Layout`, the app's script serves). Goldens
  under the generator tests change accordingly and are reviewed. The runtime injects
  nothing for dark mode. Optional follow-up: use the card `description`/`action` slots.
- Old tokens, Alder's own uses (all of them found by grep; none other exists outside the
  vendored `rt/al/al.css`, the book and the golden):
  - `rt/web/devpanel_ui.go`: `--al-radius-sm` and `--al-radius-md` become
    `--radius-sm` and `--radius-md`.
  - `cmd/alder/devui.go`: `--al-font-mono` becomes `--font-mono` (both the error strip
    and overlay rules).
  - `rt/web/router.go` (`pendingStyle`): `--al-primary` becomes `--primary`;
    `rt/web/testdata/production_page.golden` is regenerated.
  - `examples/blog/static/app.css`: `--al-font-mono` becomes `--font-mono`.
  - `examples/todos/static/app.css`: `--al-destructive-text` becomes `--destructive`
    (the text tint on the destructive alert/error text).
  - `book/C-design-system.md` is rewritten (below).
  Each replacement keeps its fallback value, so the surfaces still render if a page
  carries no stylesheet.
- Hardening the injected UI against the global base layer: the dev panel
  (`al-devpanel`), the error strip/overlay (`cmd/alder/devui.go`) and the pending bar
  (`rt/web/router.go`) are injected into apps whose `al.css` now carries Preflight
  (zero margins, `* { border: 0 solid }`, reset buttons/lists/headings, `img/svg
  { display: block }`). Each injected rule set states what it relies on (`box-sizing`,
  `border`, `font`, `margin`, `list-style`, `line-height`, `color`, `background` on
  buttons) explicitly instead of inheriting browser defaults, uses only
  `var(--x, fallback)` with literal fallbacks so it also works without alder-ui, and a
  test or screenshot pass checks the panel and strip in a default and a dark page.
- `book/C-design-system.md` (the Appendix C rewrite also carries the migration guide,
  section 10): rewrite the tokens section (oklch sheet, `.dark`, the one-line script, `--radius-*`/`--shadow-*`/fonts, the AA note with the strict
  override), add "The base layer and typography" (`al-page plain`), update each element's
  attribute table and demo for the additions, update "From another stack" and the
  vendored version; list the removed `--al-*` tokens with their replacements. `appendix_test.go` enforces the tables.
- Tests: `cmd/alder/{build,run,new}_test.go` assert the font files are written/served and
  that the scaffold's Layout carries the theme script; generator tests assert it in
  their goldens; the vendoring test still checks `VERSION` (set to the rc during the
  early look, `v0.4.0` at the end).
- `.superpowers/brainstorm/2026-10-24-alder-ui-shadcn/` is scratch; the reference
  material it needs is this spec's companion file under `docs/`.

Compiler, checker, LSP: **no change**. Emitted element markup is unchanged; the
manifest additions are picked up by vendoring. Renderer: only the injected dev/pending
CSS above, and the theme script in scaffold and generator output; no dark-mode logic.

## 10. The nightvote-replay check and migration guide

`~/projects/nightvote-replay` uses `al-page width="narrow"`, `al-card` ×6, `al-button` ×2,
`al-alert` ×2, `al-toasts`, and raw `h2`/`p`/`code`/`ul`/`table` inside the cards (the
builder re-counts these with a grep before the check and corrects this list if the app
has moved). It is the real-page acceptance test for decision 3, and it is migrated by
hand: zero source changes is not the bar (section 5.3, ruling 3).

1. Before: `alder build && alder run`, screenshot `/` light and dark (toggle `.dark` by
   hand), keep in `ref/out/nightvote-before-*.png` (or the PR).
2. Migrate by hand with the guide below.
3. After vendoring: rebuild with the new compiler (`go install ./cmd/alder` first, per
   the "reinstall alder after merge" note), screenshot the same; the cards should
   read as shadcn cards with CardTitle/CardDescription-like headings, the bare tables
   as shadcn tables, links and code as Typography's. `app.css` is reviewed for overrides
   that fought the old look and can go.
4. The user looks at both pairs and says whether it is "shadcn out of the box". That
   judgement, not the diff script, closes #21.

### Migration guide (alder-ui v0.3.x to v0.4.0; goes in the release notes and Appendix C)

1. Re-vendor / rebuild with the Alder release that carries alder-ui `v0.4.0`.
2. Replace every old token in your own CSS: `--al-radius-*` to `--radius-sm|md|lg|xl`;
   `--al-font-mono` / `--al-font-sans` to `--font-mono` / `--font-sans`;
   `--al-destructive-text` to `--destructive`; `--al-primary` and the other plain colour
   tokens to the shadcn name without the prefix; `--al-shadow-sm` to `--shadow-sm`;
   `--al-space-*`, `--al-text-*`, `--al-leading*`, `--al-weight-*`, `--al-control-h-*`,
   `--al-ring-*` to literal rem values or Tailwind-scale equivalents; `--al-*-soft` and
   `--al-*-border` have no replacement (alerts no longer use them). Only
   `--al-width-narrow|normal|wide` survive. `grep -rn -e '--al-' static/ src/` finds them.
3. Dark mode: add the theme script to the `<head>` of your Layout, before the stylesheet
   link (the exact line is in Appendix C and the alder-ui README). The CSS-only
   OS-dark block and `[data-theme="dark"]` are gone; `.dark` on `<html>` is the only switch.
4. Rebuild, look at each page in light and dark, and delete any app CSS that fought the
   old look.

For nightvote-replay: steps 2 and 3 are applied to its `static/app.css` (or equivalent)
and its Layout; steps 1 and 4 are the check above.

## 11. Phasing (each phase one alder-ui PR with its triptychs)

0. Oracle: `ref/` app and committed `ref/dist/`, `demo/parity.html`, `test/parity.py`
   (+ allow-list), CI job. Nothing visual changes yet.
1. Tokens and base: oklch sheet, `.dark` (no alias), old `--al-*` tokens removed, `--radius-*`/`--shadow-*`/fonts,
   Geist woff2 + `@font-face`, Preflight, Typography, icons, focus ring, check.py rule
   changes. Most of the visible change lands here. Tag `v0.4.0-rc.1` (manifest `version`
   set to match) for the Alder branch's early look (section 5.3, ruling 4b).
2. Controls: button, input, textarea, select, checkbox, label/help/errors.
3. Surfaces: card (+ slots), badge, alert, table, pagination, stack, page (`plain`).
4. Overlays and motion: dialog (+ slot), popover, menu, tooltip, toast, tabs (+ variant),
   devpanel re-pin.
5. Tag `v0.4.0` (breaking; manifest `version` back to `v0.4.0`). Then in Alder: vendor,
   fonts in the embed/build/serve lists, `--al-*` replacements, injected-UI hardening,
   Appendix C with the migration guide, `alder new` and generator theme script with
   regenerated goldens, tests, nightvote-replay hand migration and before/after,
   examples pass.

## 12. Left to the builder

- Exact `@font-face` declarations (unicode-range subsetting or not; `font-display`),
  the upstream file names, and the licence file placement for Geist and lucide.
- The `.light`/`.dark` block mechanics for nested subtrees (a light card in a dark
  page) and whether `color-scheme` on a subtree is worth it.
- The exact `@starting-style` transitions and how they interact with the `al-shim.js`
  polyfill paths and `display: none` on `al-menu`/`al-tooltip`.
- Whether `al-card`'s unslotted-heading rule needs `:first-child` or
  `:nth-child(1 of :not([slot]))`, and the same for the description `p`.
- How `test/parity.py` drives hover/focus states for both pages identically, and the
  initial allow-list contents (structural entries per section 5.3, ruling 4a).
- Where Playwright for Python is installed for CI (a `requirements-dev.txt` in alder-ui)
  and how the triptychs are attached (artifact upload).
- The `line` tabs' exact classes (re-fetch `tabs.tsx`; the reference marks this).
- Any shadcn change on `main` between this spec's fetch (2026-10-24) and the build: the
  reference file names its sources; re-fetch and amend the companion file rather than
  guessing.
