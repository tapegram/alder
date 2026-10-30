# Theme config, phase 1 (#38): implementation plan

Spec: `docs/superpowers/specs/2026-10-29-theme-config-design.md` (binding). Test-first, one commit
per task. Diagnostic code: **E0042** `ThemeDecl` (E0040 is #75's, E0041 #86's; the spec is
updated to match).

Import-cycle note: `internal/ir` lowers names through `internal/theme`, so `theme` cannot import
`ir`. `ir.Setting` is an alias of `theme.Setting`, and `theme.CSS` takes the three blocks
(`CSS(root, light, dark []Setting)`); `(*ir.Theme).CSS()` calls it. Same output as the spec's
`theme.CSS(*ir.Theme)`.

## Task 1: `internal/theme` (table, defaults, values, CSS)

Files: `internal/theme/theme.go`, `css.go`, `theme_test.go`.

- `Kind` (Colour, Length, Font, Shadow); `Token{Field, CSS, Kind}`; `Tokens` ordered as al.css
  declares them; `Lookup(field)`; `Removed` (lowerCamel `--al-*` name → replacement or "none"
  reason, every row of Appendix C's table); `Defaults` (light, dark by CSS name, parsed once from
  the embedded `rt/al/al.css`: `:root, .light`, `.dark`, and the derived `:root` block).
- Value shape checks: `ColourOK`, `LengthOK`, `FamilyOK`, `ShadowOK` (spec's grammar);
  `Alias(value) (css string, ok)` for `var(--x)`.
- `FontStack([]string) string` (quotes non-generic families), `CSS(root, light, dark)`.
- Tests: `TestTokens_MatchVendoredStylesheet` (both directions, minus `--color-white`,
  `--color-black`, `--_al-*`), `TestDefaults_EveryToken`, `TestCSS_SpecExample` (byte for byte),
  `TestCSS_Empty`, `TestFontStack_QuotesNonGeneric`, `TestValueShapes` (table of good/bad),
  `TestFieldName_Mechanical` (lowerCamel ↔ CSS name).

## Task 2: AST and parser

Files: `internal/syntax/ast.go` (`ThemeDecl{Fields []*DeployField}`, `DeployValue.Kind`
`"record"`), `parser.go` (contextual `theme` + `{`; `{` as a record value), `dump.go`,
`parser_test.go`.

- Tests: `TestParse_ThemeDecl` (the spec example dumps with a nested record),
  `TestParse_ThemeStaysAName` (`theme = 1`, `x.theme`, `{ theme: 1 }`, `theme x = x`).
- `deploy` gets a record value in its existing "not a target" refusal (corpus
  `E0030-deploy-record-value`).

## Task 3: checker (E0042)

Files: `internal/diag/diag.go` (`ThemeDecl = "E0042"`), `explain.go`, `diag_test.go`;
new `internal/check/theme.go`; `decls.go` (collect in `defineApp`); `partitions.go`
(`m15Placement`: outside App).

Corpus (`testdata/corpus/`): `theme-ok` (spec example), `theme-empty-ok`,
`E0042-theme-outside-app`, `E0042-theme-twice`, `E0042-duplicate-field`,
`E0042-light-not-record`, `E0042-unknown-suggest`, `E0042-unknown-no-suggest`,
`E0042-removed-replacement`, `E0042-removed-none`, `E0042-colour-top-level`,
`E0042-length-under-light`, `E0042-bad-colour`, `E0042-bad-length`, `E0042-bad-font`,
`E0042-bad-shadow`, `E0042-negative-length`, `E0042-var-unknown`, `E0042-var-non-colour`,
`E0042-ctor-value`.

## Task 4: IR

Files: `internal/ir/ir.go` (`Program.Theme *Theme`, `Theme{Root, Light, Dark []Setting}`),
new `lower_theme.go`, `lower.go`, `dump.go`; test `TestLowerTheme` (dump form
`(theme (root …) (light …) (dark …))`, Tokens order, font stack joined, nil without a theme).

## Task 5: runtime

Files: `rt/web/router.go` (`Options.Theme`; `NewRouter` builds the served `al.css` + ETag once;
`serveStatic` uses it; `Reload != ""` → `Cache-Control: no-cache` for al.css).
Tests (`rt/web`): `TestStatic_ThemeAppended` (ends with the block; ETag differs from the bare
one; 304 on If-None-Match; al.js unchanged), `TestStatic_ThemeReloadNoCache`.

## Task 6: gen and buildkit

Files: `internal/gen/caps.go` (`Theme:` in `Web()` when non-empty), golden
`testdata/golden/gen_theme/{app,want}`; `internal/buildkit/buildkit.go` (`CopyStatic(src, dst,
theme)`, `WriteDesignSystem(dir, theme)`, `WrittenTheme(dir)`), callers `cmd/alder/build.go`,
`cmd/alder/run.go` (a static-only refresh keeps the last build's theme), `internal/scratch`.
Test: `TestWriteDesignSystem_Theme` (on-disk `al.css` = embedded + "\n" + theme, the bytes the
router serves; `WrittenTheme` reads it back).

## Task 7: formatter

Files: `internal/format/print_decl.go` (`theme ` + `deployRecord`; `deployValue` "record"),
`comments.go` (ThemeDecl items), `format_test.go`.
Tests: the spec example formats to itself; `theme { radius: "0.5rem" }` stays on one line; three
fields go vertical; a comment inside `light` is kept.

## Task 8: tree-sitter

Files: `tree-sitter-alder/grammar.js` (`theme_decl`, `_deploy_value` gains `deploy_record`),
`queries/highlights.scm`, `test/corpus/declarations.txt`, `drift_test.go`; regenerate with
`scripts/ts.sh generate` (CLI 0.27.1), `scripts/ts.sh test`.

## Task 9: LSP

Files: `internal/lsp/complete.go` (`themeFields` row first in the table), `hover.go`
(`themeHover`), tests `TestCompletion_ThemeLight`, `TestCompletion_ThemeTop`,
`TestHover_ThemeField`.

## Task 10: book, example, docs

- `book/C-design-system.md` "Changing a token" leads with `theme` (example as an
  `alder fragment`, kinds table, cascade, AA-strict example, pasted themes in `app.css`);
  `03-syntax.md` formatter list gains `theme`; `11-tooling.md` E0042 row.
- `book/appendix_test.go`: `TestAppendixC_ThemeFragments` (each `theme {` fragment checks in
  the scaffold's App) and the field table matches `theme.Tokens`.
- `examples/blog/app/App.alder`: `radius` and `primary`/`primaryForeground` in both schemes.
- Spec: E0040 → E0042.
- `python3 book/site/build.py`; `go vet ./...`; `go test ./...`; `cd experiments && python3 -m
  unittest`.
