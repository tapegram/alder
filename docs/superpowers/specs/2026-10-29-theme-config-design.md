# Theme config: a typed `theme` declaration, checked and emitted as CSS

Design for [tapegram/alder#38](https://github.com/tapegram/alder/issues/38). Everything marked
**(call)** is the author's decision, not the user's. Related: the alder-ui spec
(`2026-10-24-alder-ui-shadcn-design.md`, which fixed the token format: shadcn's `:root, .light`
and `.dark` blocks), Appendix C of the book, memory notes "Design system configurator idea" and
"shadcn web components idea".

## Intent

Today an app changes the look by writing CSS custom properties by hand in `static/app.css`
over the shadcn token names. Nothing checks the names (a typo or a removed `--al-*` token
silently does nothing) or the values, and a model writing Alder has no completion for them.
After this change the app declares its look once, in `App`, in a form the compiler checks
against the vendored design system and turns into the CSS; hand-written CSS keeps working.

**Phase 1**, specified fully here, is the declaration, checker, CSS emission, formatter,
grammar, LSP completion and hover, and the book. **Phase 2** (the configurator and a CSS
importer) and **phase 3** (editor swatches) get their own specs; the last section fixes only
what phase 1 must leave in place for them.

## Goals

1. One `theme { … }` declaration in `App` sets any of alder-ui's tokens for light and dark;
   the build emits the CSS; no new file to link, nothing to change in `Layout`.
2. Every token name and value is checked: an unknown or removed name, a colour where a length
   belongs, a colour outside `light`/`dark`, a malformed value, are diagnostics with the fix in
   the message.
3. The list of tokens comes from the vendored alder-ui (`rt/al/al.css`), so re-vendoring that
   adds or drops a token fails a test until the table follows.
4. Hand-written CSS (a pasted shadcn/tweakcn theme, an app-specific token, a one-off rule)
   keeps working and keeps winning, exactly as today.
5. The LSP completes token names in a theme and shows a token's CSS name, kind and defaults.
6. Existing programs keep compiling and behaving identically; `theme` stays usable as a name.

## Non-goals

- The configurator, `alder theme import` (phase 2), colour swatches (phase 3), a TUI.
- A per-Layout or per-page theme. The theme is the app's **(call)**: a subtree that needs the
  other scheme uses `.light`/`.dark`, as today.
- New tokens (`--spacing`, `--tracking-*`, `--brand-*`): a token alder-ui does not define is not
  a theme field; it goes in `app.css` as today. Checking `app.css` itself.
- Expressions in values: no `calc()`, no Alder values, no `var()` except to alias one colour
  token to another. No colour literal type in the language.

## Anti-goals

- A second way to write the same theme: phase 2's configurator writes this declaration through
  the formatter's printer, and it is never the only way in.
- Emitting CSS the app did not write: an unset token keeps alder-ui's value; `theme { }` emits
  nothing. A value that can break out of its declaration: every value is from a checked set.

## The declaration

In `App`, once, parsed like `deploy` (`theme` is contextual: an `LIdent` whose text is `theme`
followed by `{`, so `localStorage.theme`-style names elsewhere are untouched) **(call)**:

```alder
theme {
  radius: "0.5rem",
  fontSans: ["Inter", "system-ui", "sans-serif"],
  light: {
    primary: "oklch(0.55 0.22 285)",
    primaryForeground: "#ffffff",
    ring: "var(--primary)",
  },
  dark: {
    primary: "oklch(0.72 0.16 285)",
    primaryForeground: "oklch(0.145 0 0)",
  },
}
```

Alder syntax, not a data file **(call)**: `deploy` shows a checked data declaration in `App`
works, and the formatter, grammar and LSP then cover it, where a data file would need its own
loader, checker, formatter and completion. The AST reuses `DeployField`/`DeployValue` with one
new value kind, `"record"` (a bare `{ fields }`, `Rec` set, `Ctor` empty), in a new `ThemeDecl`.
`deploy` refuses a record value where it refuses any non-target today (no message change).

**Field names** are the CSS names without `--`, in lowerCamel: `--muted-foreground` is
`mutedForeground`, `--chart-1` is `chart1`, `--al-width-narrow` is `widthNarrow` **(call)**;
the mapping is mechanical both ways and the message for an unknown name shows the CSS name.

**Kinds and where each field may appear:**

| kind | fields | value |
|---|---|---|
| colour, under `light` or `dark` only | `background`, `foreground`, `card`, `cardForeground`, `popover`, `popoverForeground`, `primary`, `primaryForeground`, `secondary`, `secondaryForeground`, `muted`, `mutedForeground`, `accent`, `accentForeground`, `destructive`, `destructiveForeground`, `border`, `input`, `ring`, `chart1`…`chart5`, `sidebar`, `sidebarForeground`, `sidebarPrimary`, `sidebarPrimaryForeground`, `sidebarAccent`, `sidebarAccentForeground`, `sidebarBorder`, `sidebarRing` | text: `#rgb`/`#rgba`/`#rrggbb`/`#rrggbbaa`; `transparent`, `currentColor`; `fn(args)` with fn one of `oklch oklab lch lab hsl hwb rgb rgba hsla color color-mix light-dark` and args from `[A-Za-z0-9 .%/,()-]`, parentheses balanced; or `var(--x)` where `x` is a colour token's CSS name |
| length, top level only | `radius`, `radiusSm`, `radiusMd`, `radiusLg`, `radiusXl`, `widthNarrow`, `widthNormal`, `widthWide` | text: `0`, or a non-negative decimal with unit `rem em px % vw vh ch lh` |
| font stack, top level only | `fontSans`, `fontMono` | a non-empty list of text; each entry non-empty, without `"`, `;`, `{`, `}` or a newline |
| shadow, top level only | `shadowXs`, `shadowSm`, `shadowMd`, `shadowLg` | text: `none`, or non-empty text from `[A-Za-z0-9 .%/,()#-]` |

Values are text, not numbers or constructors **(call)**: a pasted `--primary: oklch(0.55 0.22
285);` becomes `primary: "oklch(0.55 0.22 285)"` by inspection, and a colour or length type in
the language is more than the problem needs. Values are trimmed, otherwise emitted as written.
`light` and `dark` are each optional and independent **(call)**: a colour set in one keeps
alder-ui's value in the other (a light-only app sets `light` alone). Shadows are theme-wide
**(call)**: shadcn's are the same in both schemes. `radius` alone keeps shadcn's derived ratios.

## Checking (E0040 `ThemeDecl`, one code as `deploy` has E0030)

Every refusal names the fix. In source order:

- `theme` outside `App`: "`theme` is only allowed in `App`" (where `deploy`'s is reported,
  `decls.go` ~1328). A second `theme` in `App`: "`theme` is declared once: merge this one into
  the first" (the first is kept).
- A field declared twice in the same record (top, `light`, `dark`): "`primary` is declared
  twice in `light`: declare it once".
- `light`/`dark` whose value is not a record: "`light` is a record of colours: `light: { primary:
  \"…\" }`".
- An unknown name: "`primray` is not a token of alder-ui v0.4.2; did you mean `primary`
  (`--primary`)?" with the nearest known name by edit distance (≤ 2) when there is one, else
  "the tokens are listed in Appendix C". A removed `--al-*` name in lowerCamel (`alRadiusSm`,
  `alFontMono`, `alDestructiveText`, every row of Appendix C's "Removed tokens") answers with
  the replacement from that table: "`alFontMono` was removed in alder-ui v0.4: write
  `fontMono`"; a removed token without a replacement says so ("it has none: alerts no longer use
  it").
- A colour at the top level: "`primary` is a colour: set it under `light` or `dark`". A
  non-colour under `light`/`dark`: "`radius` is theme-wide: set it outside `light` and `dark`".
- A value of the wrong shape, one message per kind, each showing an example: colour
  ("`primary` is a colour written as text: `\"oklch(0.55 0.22 285)\"`, `\"#4f46e5\"` or
  `\"var(--accent)\"`"), length ("`radius` is a length written as text, such as `\"0.5rem\"`"),
  font stack ("`fontSans` is a list of family names, such as `[\"Inter\", \"sans-serif\"]`"),
  shadow. A constructor value anywhere gets the kind's message (no special case). A negative
  length, `var(--x)` of an unknown token, or of a non-colour token ("`var(--radius)` is a
  length, not a colour"), are reported with the same code.

The token table lives in a new package `internal/theme` **(call)**: a hand-written ordered list
`Tokens` (field name, CSS name, kind, scope), `Removed` (lowerCamel name → replacement text),
and `Defaults`, the light and dark values parsed once from the embedded `rt/al/al.css` (its
`:root, .light`, `.dark` and derived `:root` blocks, custom properties only). The manifest
carries no `cssProperties`, so the stylesheet is the source of truth, and a test holds the two
together: every `--` name those blocks declare is in `Tokens` (except `--color-white`,
`--color-black` and the private `--_al-*`) and every entry is declared there. Re-vendoring
alder-ui with a new or dropped token fails that test until the table follows.

## Emission

The IR gets `Program.Theme *Theme` (`Theme{Root, Light, Dark []Setting}`, `Setting{Name,
Value string}`, `Name` the CSS name; nil without a declaration; dump form `(theme (root
(--radius "0.5rem")) (light (--primary "…")) (dark …))`). `internal/theme.CSS(*ir.Theme)
string` writes, in `Tokens` order (not source order, so the output is stable) **(call)**:

```css
/* theme: App */
:root { --radius: 0.5rem; --font-sans: "Inter", system-ui, sans-serif; }
:root, .light { --primary: oklch(0.55 0.22 285); --primary-foreground: #ffffff; --ring: var(--primary); }
.dark { --primary: oklch(0.72 0.16 285); --primary-foreground: oklch(0.145 0 0); }
```

An empty block is omitted; an empty theme gives `""`. Families are quoted unless generic
(`serif sans-serif monospace system-ui ui-sans-serif ui-serif ui-monospace ui-rounded cursive
fantasy math emoji fangsong`). The blocks are unlayered, so they beat alder-ui's `@layer al`
values, and `.light` is named so a light subtree in a dark page gets the theme's light values.

**Where the CSS goes: appended to `al.css`** **(call)**. The generated `Web()` carries it as
`web.Options.Theme` (a Go string; the generated program never parses it); `NewRouter`, when
`Theme != ""`, builds the served `al.css` once as the embedded bytes + `"\n"` + the theme and a
strong ETag of that, and `serveStatic` uses them for `al.css` (every other `al.Names` file is
unchanged). `buildkit.CopyStatic` and `WriteDesignSystem` take the theme text and write the
same bytes to `static/al/al.css` on disk, so the build directory and the served file stay
equal (callers: `cmd/alder/build.go`, `cmd/alder/run.go`, `internal/scratch`). The app links
nothing new; the generated admin, `alder test`'s browser and `alder scratch` get the theme
through the same options. Under `alder run` (`Options.Reload != ""`) `al.css` is served
`Cache-Control: no-cache` **(call)** so an edit to the theme shows on the next load; production
keeps `DesignSystemCache`.

**The escape hatch** is unchanged `app.css` **(call)**: it is linked after `al.css`, unlayered,
so at equal specificity its later `:root, .light` wins over the theme's. A pasted shadcn or
tweakcn export therefore still works and overrides the declaration; a token alder-ui does not
know (`--tracking-normal`, `--brand-x`) lives there. There is no raw-CSS field in `theme`.
The AA-strict border override (Appendix C) is two theme fields, `input` and `border`, under
`light` and `dark`; no preset **(call)**.

## Formatter, grammar, LSP, book

- `internal/format/print_decl.go`: `theme ` + `deployRecord` (nested records print through the
  existing `deployValue` with the new kind); the "3 or more fields is vertical" rule of chapter 3
  applies to each record; comments follow `deploy`'s handling.
- `tree-sitter-alder/grammar.js`: `theme_decl: seq('theme', field('body', $.deploy_record))` in
  the declaration choice; `_deploy_value` gains `$.deploy_record`; `highlights.scm` adds
  `"theme"`; a corpus case in `test/corpus/declarations.txt`; regenerate (`drift_test.go` holds
  the grammar to the Go parser).
- LSP: completion, when the cursor's line is `^\s*[a-z][A-Za-z0-9]*?$` and the cursor lies in a
  `ThemeDecl` of the last good build: `light`, `dark` and the top-level tokens at depth 0, the
  colour tokens inside `light`/`dark`, each inserted as `name: ` with the CSS name and kind as
  detail. Hover on a theme field: the CSS name, kind, and alder-ui's default light and dark
  values from `Defaults`. Diagnostics flow through `alder check` as E0040 does.
- `internal/diag/explain.go`: the E0040 entry (what a theme is, the kinds, where colours go,
  the removed-token answer).
- Book: Appendix C "Changing a token" is rewritten to lead with `theme` (the example above, the
  table of fields and kinds, the cascade: layered al.css < theme < app.css, the AA-strict
  example, pasted themes still pasted into `app.css`); chapter 3's formatter list gains `theme`;
  the site is regenerated (`book/site/build.py`).
- `examples/blog/app/App.alder` declares a small theme (`radius`, `primary` and
  `primaryForeground` in both schemes) so the e2e build and the examples' visual pass cover
  emission **(call)**. The scaffold's `App.alder` is unchanged: a fresh app's look is alder-ui's.

## Where things change

| layer | files |
|---|---|
| AST, parser | `internal/syntax/ast.go` (`ThemeDecl`; `DeployValue.Kind` `"record"`), `parser.go` (contextual `theme`, record value), `parser_test.go` |
| checker | new `internal/check/theme.go` (as `deploy.go`), `decls.go` (collect in `defineApp`; App-only), `internal/diag/diag.go` (E0040), `explain.go` |
| token table | new `internal/theme/{theme.go,css.go,theme_test.go}` |
| IR | `internal/ir/ir.go`, `lower.go`, new `lower_theme.go`, `dump.go` |
| gen, runtime | `internal/gen/caps.go` (`Theme:` in `Web()`), `rt/web/router.go` (`Options.Theme`, served `al.css`, dev `no-cache`), `internal/buildkit/buildkit.go` and its three callers |
| formatter, grammar | `internal/format/print_decl.go`, `format_test.go`; `tree-sitter-alder/grammar.js`, `queries/highlights.scm`, `test/corpus/declarations.txt` |
| LSP | `internal/lsp/complete.go`, `hover.go`, tests |
| corpus, goldens | `testdata/corpus/theme-ok`, `E0040-*` (one per message above), `testdata/golden/gen_theme`, `check_app` |
| book, examples | `book/C-design-system.md`, `03-syntax.md`, `appendix_test.go`, `book/site`; `examples/blog/app/App.alder` |

## Verification (tests named by behaviour)

- `internal/theme`: the table matches the vendored `al.css` (both directions); `Defaults` holds
  every token's light and dark value; `CSS` of the example above is the block above byte for
  byte; `CSS` of an empty theme is `""`; a font list quotes `"Geist Mono"` and not `monospace`.
- Checker corpus: a clean theme; `theme` outside App; twice in App; duplicate field; `light`
  not a record; unknown name with suggestion; removed `--al-*` name with replacement and
  without; colour at top level; length under `light`; each kind's bad value; negative length;
  `var()` of an unknown and of a non-colour token; a constructor value; an empty theme is clean.
- Parser/formatter: `theme` as a variable and a field name still parse; the example formats to
  itself; a one-field theme stays on one line, three fields go vertical; a comment inside is kept.
- gen golden: `Web()` carries the CSS; `rt/web`: `/static/al/al.css` ends with the theme block,
  its ETag differs from the bare stylesheet's and gives a 304 on `If-None-Match`, `al.js` is
  untouched, and under `Reload` the header is `no-cache`; `buildkit`: the on-disk `al.css`
  equals the served bytes.
- LSP: completion inside `light: {` lists `primary` and not `radius`; at depth 0 lists `light`
  and `radius` and not `primary`; hover on `primary` shows `--primary` and both defaults.
- `go test ./...` green, `alder build` of `examples/blog`, and a visual pass of the blog in light
  and dark with the new primary.

## Later phases (fixed here so phase 1 leaves room)

- **Phase 2, the configurator and the importer.** A page `alder run` serves at `/_dev/theme`
  **(call)**: alder-ui's components live, a control per token in the table, a `.dark` toggle,
  the current theme from the last good build, and a "write" that edits `App.alder` through
  `internal/format`'s printer (insert or replace the one `theme` declaration; nothing else
  moves). A web page over a TUI **(call)**: the preview is the point and a terminal cannot show
  the components. `alder theme import <file.css>` turns a pasted shadcn/tweakcn export into the
  same declaration, listing what it could not place (that stays for `app.css`).
- **Phase 3.** `textDocument/documentColor` from the theme's colours; if alder-ui publishes the
  tokens as `cssProperties` in its manifest, `Tokens` is read from `elements.Builtin` instead.
