# Theme phase 3: starting themes, DTCG, swatches — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Alder apps can start from one of five alder-ui themes (`alder theme use`, `alder new --theme`, a Start-from picker in `/_dev/theme`), `alder theme import` reads DTCG token files, the LSP shows colour swatches in the `theme` declaration, and the token table is read from the vendored Zinc preset.

**Architecture:** A pure DTCG reader in `internal/theme` turns a resolver or tokens file into the same `(Theme, []Unplaced, Skipped)` the CSS importer returns, so `use`, `import` and the page share one tail ending in `WriteTheme`. Presets are alder-ui's `themes/` vendored to `internal/theme/presets/` and embedded; nothing about them reaches `rt/` or a built app. Five PRs, in order: **A** vendor + reader + import detection, **B** presets + `theme use` + `new --theme`, **C** configurator picker, **D** `ParseColour` + `documentColor`, **E** table from Zinc.

**Tech Stack:** Go (stdlib `encoding/json` streaming, `embed`, `io/fs`), the existing LSP server, Playwright browser tests (`-tags browser`).

**Spec:** `docs/superpowers/specs/2026-10-31-theme-presets-design.md`. Prerequisite: alder-ui tag `v0.5.0` (plan `~/projects/alder-ui/docs/superpowers/plans/2026-10-31-themes-v0.5.0.md`).

## Global Constraints

- **Counts correction:** the table has **32** colour tokens and, with `borderWidth`, **15** theme-wide. The spec's "35" is a miscount; every count, summary line and test uses 32/15. `alder theme use paper` prints `theme: paper → app/App.alder  (light 32, dark 32, theme-wide 15; replaced the theme that was there)`.
- New token: field `borderWidth`, CSS `--al-border-width`, kind `Length`, theme-wide, appended after `shadowLg`.
- Preset order everywhere: `zinc`, `brutal`, `catppuccin`, `paper`, `pebble`.
- Purposes (verbatim): zinc `Tools, dashboards, admin — the default, the safe choice.`; paper `Reading: blogs, docs, long-form.`; catppuccin `Developer-facing apps.`; brutal `Launch pages, portfolios, anything that should shout.`; pebble `Consumer, mobile-feel apps.`
- Nothing written outside the validate-then-print path: every write goes through `ThemeText` (E0042) and `WriteTheme`.
- Presets, the reader and the picker live in the `alder` binary only. `rt/` changes only by re-vendoring.
- Reader limits: a document over 1 MiB or a resolver referencing more than 32 files is an error; a `$ref` with `..` or absolute is an error.
- Unplaced reasons (verbatim shapes): `not a token of alder-ui <Version()>`; `` `radius` is a length; the file says `color` ``; `no $type`; `` `{color.brand.500}` is not a token ``.
- Numbers print in Go's shortest form (`strconv.FormatFloat(f, 'f', -1, 64)`): `0.3`, never `.3`.
- Messages that name the alder-ui version use `theme.Version()`, never a literal.
- Theme tokens land on `<html>` only; `theme.CSS` keeps writing only custom properties under scheme selectors (keeps several named themes cheap later).
- `Preset` is a type, so the lookup is `theme.PresetNamed(name string) (Preset, bool)` (the spec's `Preset(name)` would collide with the type in Go).
- Model per task: Sonnet for A1, A4, B2-B4, C1, D1, D3; Opus for A2, A3, C2, D2, E1, E2; Fable whole-branch review of PR E before merge.
- Run the quick loop with `scripts/test.sh -short`, the full suite with `scripts/test.sh` before each PR.

## Review Focus

- Import input with a UTF-8 BOM or leading blank lines before `{`: detected as DTCG (strip a BOM, skip whitespace), not parsed as CSS.
- A resolver whose `$ref` names a missing sibling file: a refusal naming the missing file, not a panic or an empty theme.
- `alder theme use paper` in an app whose `App.alder` does not parse: refused with the parser's message, nothing written (as `import` does).
- `alder new x --bare --theme paper`: either the bare scaffold gets the declaration and builds, or the command refuses before creating the directory; never a half-created app.
- Choosing a preset in `/_dev/theme`, then reloading before Write: the working theme is still the preset (sessionStorage rule), and Reset still returns to the committed theme.

Each line's test sits in the owning task below.

---

## PR A — vendor v0.5.0, DTCG reader, import detection

Branch `theme-phase-3-a` from `main` (merge the spec branch first or rebase onto it).

### Task A1: Vendor alder-ui v0.5.0 and the `borderWidth` token

**Files:**
- Modify: `scripts/vendor-al.sh`, `rt/al/*` (by running it), `internal/theme/theme.go` (`Tokens` literal), version-literal tests: `cmd/alder/run_test.go:426`, `internal/theme/import_test.go:278-284`, `testdata/corpus/E0042-{unknown-no-suggest,var-unknown,unknown-suggest}/expected.diag`, goldens that list the token table
- Create: `internal/theme/presets/**` (by running the script)

**Interfaces:**
- Produces: `internal/theme/presets/<name>/{theme.resolver.json,base.tokens.json,light.tokens.json,dark.tokens.json}` for the five names, plus `presets/catppuccin/LICENSE`; `Tokens` gains `{"borderWidth", "--al-border-width", Length}` after `shadowLg`.

- [ ] **Step 1:** In `vendor-al.sh`, after the existing copies: `rm -rf internal/theme/presets && cp -R "$src/themes" internal/theme/presets`. Run `scripts/vendor-al.sh v0.5.0`.
- [ ] **Step 2:** Run `go test ./internal/theme/ -run TestTokens_MatchVendoredStylesheet` — FAIL: `al.css declares --al-border-width, which theme.Tokens lacks`.
- [ ] **Step 3:** Add the token. Replace hard-coded `v0.4.2` in the listed tests and corpus files with `v0.5.0` (prefer `theme.Version()` where the test is Go).
- [ ] **Step 4:** Run `scripts/test.sh -short`, then regenerate any golden that lists every token (hover/completion goldens) with the repo's update flag and inspect the diff: only `borderWidth` added. Expected: PASS.
- [ ] **Step 5:** Commit `vendor alder-ui v0.5.0: themes, --al-border-width (#38)`.

### Task A2: Reader structure — files, resolver, schemes, order

**Files:**
- Create: `internal/theme/dtcg.go`, `internal/theme/dtcg_test.go`, `internal/theme/testdata/dtcg/{resolver,single-file,limits}/…`

**Interfaces:**
- Produces (unexported, used by A3 and E2 — **the table build in E2 must not depend on `ByCSS`/`Lookup`, so this layer knows nothing about alder's names**):
  ```go
  type dtcgToken struct {
      Path        []string        // e.g. ["color","primary"]
      Type        string          // own or inherited $type, "" if none
      Value       json.RawMessage
      Description string
      File        string          // file it came from, for Unplaced.Block
  }
  type dtcgDoc struct {
      Name, Description string     // resolver's name/description ("" for a tokens file)
      Root, Light, Dark []dtcgToken // file order; a colour in Root is copied to Light and Dark by A3
      Skipped Skipped
      byPath map[string]dtcgToken  // "color.primary" → token, over Root+Light (and Root+Dark for dark lookups)
  }
  func loadDTCG(fsys fs.FS, name string) (*dtcgDoc, error)
  ```

- [ ] **Step 1: Write failing tests** in `dtcg_test.go`:
  - `TestLoadDTCG_Resolver`: `testdata/dtcg/resolver/theme.resolver.json` (base + light/dark contexts) → `Root`, `Light`, `Dark` hold the fixture's tokens in file order; `Name`/`Description` from the resolver.
  - `TestLoadDTCG_SchemeModifierNames`: modifiers named `Mode` and `colorScheme` with contexts `Light`/`DARK` are picked; an extra modifier `density` with `default: "compact"` resolves at its default and `Skipped.Kinds` contains `density`; one without a default uses its first context.
  - `TestLoadDTCG_SingleFile`: `single-file/tokens.json` with top-level `light`/`dark` groups places by group; other tokens are `Root`.
  - `TestLoadDTCG_TypeInherited` and `TestLoadDTCG_NoType` (`Type == ""`).
  - `TestLoadDTCG_Limits`: a `$ref` with `../x.json` errors; absolute `/x.json` errors; a 33-file resolver errors; a >1 MiB document errors; a missing sibling file errors with its name in the message.
- [ ] **Step 2:** Run `go test ./internal/theme -run LoadDTCG` — FAIL.
- [ ] **Step 3: Implement** `loadDTCG`. Walk with `json.Decoder.Token()` so key order is kept (no `map` decode of groups). A document with `resolutionOrder` is a resolver: sets merge into `Root` in order, later wins; the scheme modifier is the first (case-insensitive) of `scheme, theme, mode, color-scheme, colorScheme` whose contexts include `light` and `dark` (case-insensitive). Read `$ref`s through `fs.ReadFile` with `fs.ValidPath` and an `io.LimitReader(1<<20+1)`. `$extensions` is ignored.
- [ ] **Step 4:** Run — PASS. Commit `theme: DTCG document loader (#38)`.

### Task A3: `ImportDTCG` — names, values, aliases, unplaced

**Files:**
- Create: `internal/theme/dtcg_value.go`
- Modify: `internal/theme/dtcg.go`, `dtcg_test.go`; fixtures `testdata/dtcg/{aliases,foreign-names,bad-types}/…`

**Interfaces:**
- Consumes: `loadDTCG`, `dtcgDoc`, `dtcgToken` (A2).
- Produces:
  ```go
  func ImportDTCG(fsys fs.FS, name string) (Theme, []Unplaced, Skipped, error)
  // unexported, used by E2 without name lookup:
  func (d *dtcgDoc) cssText(t dtcgToken, scheme string) (string, error) // scheme "light"|"dark"|""; alias to a colour-typed token → "var(--<last segment>)"
  ```
  `Unplaced.Block` is `"<file> <dotted path>"` (e.g. `light.tokens.json color.brand-500`); `Unplaced.Name` is the CSS name the reader would have used; `Unplaced.Value` is the CSS text, or the raw JSON as `/* … */` when none was produced.

- [ ] **Step 1: Write failing tests** (table-driven where the spec lists cases):
  - `TestImportDTCG_Resolver`: the A2 resolver fixture → `Root["radius"]`, `Light["primary"]`, `Dark["primary"]` set; `ring: "{color.primary}"` → `Light["ring"] == "var(--primary)"`.
  - `TestImportDTCG_SingleFileColourToBoth`: a colour outside `light`/`dark` is in both `Light` and `Dark`.
  - `TestImportDTCG_Names`: `color.brand-500` → unplaced, reason `not a token of alder-ui ` + `Version()`; `color.primary_foreground` and `color.primaryForeground` → `primaryForeground`; `width.width-narrow` → `widthNarrow`; `border.border-width` → `borderWidth`; a name in `Removed` gets the Removed message.
  - `TestImportDTCG_Colours`: oklch `[0.3,0.03,50]` → `oklch(0.3 0.03 50)`; `hsl` `[210, 40, 50]` → `hsl(210 40% 50%)`; srgb with hex, alpha 1 → the hex; srgb `[0,0,0]` alpha `0.05` → `rgb(0 0 0 / 0.05)`; srgb `[1,0.5,0]` no hex → `rgb(255 127.500 0)`; `display-p3` → `color(display-p3 1 0 0)`; alpha 0.5 → `/ 0.5` appended; a `"none"` component → `none`.
  - `TestImportDTCG_Dimensions`: `0rem` → `0`; `{0.5, rem}` → `0.5rem`; a negative radius unplaced with `NegativeLength`'s wording.
  - `TestImportDTCG_Shadow`: two layers, second `inset: true` → `0 1px 2px 0 rgb(0 0 0 / 0.05), inset 0 0 0 1px #000000`; a shadow colour `{color.border}` → `var(--border)` inside the text.
  - `TestImportDTCG_Fonts`: `["Geist","ui-sans-serif"]` → `Root["fontSans"]` with both families; a family failing `FamilyOK` → unplaced with its wording.
  - `TestImportDTCG_BadTypes`: `radius` with `$type: color` → `` `radius` is a length; the file says `color` ``; no `$type` → `no $type`; `{color.nope}` → `` `{color.nope}` is not a token ``; an alias cycle → error; `typography`/`duration` types → unplaced with the type as reason.
  - `TestUnplacedCSS_DTCG`: `UnplacedCSS` of the foreign-names result is valid CSS grouped by file.
- [ ] **Step 2:** Run `go test ./internal/theme -run 'ImportDTCG|UnplacedCSS_DTCG'` — FAIL.
- [ ] **Step 3: Implement.** Name lookup: last segment lower-cased, `_` and camelCase humps → `-`, then `ByCSS("--"+seg)`, then `Lookup(seg)`, then `ByCSS("--al-"+seg)`. Kind ↔ `$type`: `dimension`↔Length, `color`↔Colour, `fontFamily`↔Font, `shadow`↔Shadow. Each converted value passes the existing checker for its kind (`ColourOK`/`Alias`, `LengthOK`+`NegativeLength`, `FamilyOK`, `ShadowOK`); a failure is unplaced with the checker's wording. Non-colour alias targets are substituted recursively.
- [ ] **Step 4:** Run — PASS; `go vet ./internal/theme`. Commit `theme: ImportDTCG (#38)`.

### Task A4: Format detection in `alder theme import` and the paste box

**Files:**
- Modify: `internal/generate/specs.go` (`theme import` spec, ~L430-452), `internal/generate/theme_import.go`, `cmd/alder/theme.go` (`themeUsage`), `cmd/alder/theme_ui.go` (POST `/_dev/theme/import`)
- Test: `cmd/alder/theme_test.go`, `cmd/alder/theme_ui_test.go`, `internal/generate/theme_import_test.go` (or the existing import test file)

**Interfaces:**
- Consumes: `theme.ImportDTCG` (A3).
- Produces: `ImportTheme(app App, name string, src []byte, format string, print, dryRun bool) (*Result, error)` where `format` is `"css"`, `"dtcg"` or `""` (detect); a helper `func detectFormat(src []byte) string` (strip a UTF-8 BOM, first non-whitespace byte `{` → `"dtcg"`, else `"css"`). For a file path the DTCG branch calls `ImportDTCG(os.DirFS(filepath.Dir(name)), filepath.Base(name))`; for `-` and the page it uses `fstest.MapFS{"stdin.json": {Data: src}}`. Spec flag `format` (ArgText, values `css|dtcg`). `ImportTheme`'s tail after the importer is unchanged and is what B2 reuses — extract it as `func themeResult(app App, th theme.Theme, un []theme.Unplaced, sk theme.Skipped, print, dryRun bool) (*Result, error)`.

- [ ] **Step 1: Write failing tests:**
  - `TestThemeImport_DTCGResolver`: copy `internal/theme/presets/paper` to a temp dir; `alder theme import <tmp>/theme.resolver.json --dir <app>` writes 32 light, 32 dark, 15 theme-wide.
  - `TestThemeImport_DTCGSingleFile`: a `x.tokens.json` with one `color.primary` writes it.
  - `TestThemeImport_DTCGStdin`: `import -` with a single-file JSON on stdin works; with a resolver that has `$ref`s, refused with `a resolver needs its files: run \`alder theme import <dir>/theme.resolver.json\``.
  - `TestThemeImport_FormatForced`: `--format css` on a JSON file gives the CSS importer's no-theme refusal; `--format dtcg` on CSS gives a JSON error.
  - `TestDetectFormat`: BOM + `\n\n{` → `dtcg`; `:root {` → `css`; `  /* c */ :root{` → `css`.
  - `TestThemeImport_Route_DTCG` (httptest): POST `/_dev/theme/import` with `{"css": "<single-file JSON>"}` returns the theme.
- [ ] **Step 2:** Run `go test ./internal/generate ./cmd/alder -run 'ThemeImport|DetectFormat'` — FAIL.
- [ ] **Step 3: Implement.** Update `themeUsage` to `usage: alder theme import <file.css | file.json | -> [--format css|dtcg] [--print] [--dir app] [--json] [--dry-run]` and the spec's arg help (`the export (CSS or DTCG JSON), or - for stdin`).
- [ ] **Step 4:** Run — PASS. Book: in `book/C-design-system.md` "### Importing a theme" add the DTCG paragraph (what a `tokens.json` is, resolver vs single file, `--format`, what comes back unplaced). Run `go test ./book`.
- [ ] **Step 5:** `scripts/test.sh`, commit `theme import: DTCG input (#38)`, open PR A.

## PR B — presets, `alder theme use`, `alder new --theme`

### Task B1: `theme.Presets()`

**Files:**
- Create: `internal/theme/presets.go`, `internal/theme/presets_test.go`

**Interfaces:**
- Consumes: `ImportDTCG` (A3), vendored `presets/` (A1).
- Produces:
  ```go
  type Preset struct{ Name, Title, Purpose string; Theme Theme }
  func Presets() []Preset                       // sync.OnceValue over //go:embed presets; zinc first, then by Name; panics on any Unplaced or error
  func PresetNamed(name string) (Preset, bool)
  func SuggestPreset(name string) (string, bool) // edit distance ≤ 2, as Suggest
  ```
  `Title`/`Purpose` come from `dtcgDoc.Name`/`Description` (`ImportDTCG` does not return them: add an unexported `importDTCGDoc` returning the doc too, or have `Presets` call `loadDTCG` for the two strings).

- [ ] **Step 1: Write failing tests:** `TestPresets_AllPlaced` (five load, no unplaced, no error; E0042-cleanness is asserted in B2's `TestThemeUse_PresetsCheck` through `generate.ThemeText`, because `internal/check` imports `internal/theme`); `TestPresets_Order` (`zinc, brutal, catppuccin, paper, pebble`); `TestPresets_Purposes` (the five purposes verbatim); `TestPresetNamed_Unknown` (`papr` not found; `SuggestPreset("papr") == "paper"`); `TestPresets_Counts` (each: 32 light, 32 dark, 15 root; Brutal `Root["borderWidth"] == "2px"`).
- [ ] **Step 2:** Run `go test ./internal/theme -run Preset` — FAIL. **Step 3:** implement. **Step 4:** PASS. **Step 5:** commit `theme: embedded starting themes (#38)`.

### Task B2: `alder theme use`

**Files:**
- Create: `internal/generate/theme_use.go`, `internal/generate/theme_use_test.go`
- Modify: `internal/generate/specs.go` (`themeSpecs()`), `internal/generate/theme_import.go` (`ThemeImport` gains `Preset string` with json tag `preset,omitempty`; `renderText` prints the spec line when `Preset != ""`), `cmd/alder/theme.go` (`themeUsage` lists `use`)
- Test: `cmd/alder/theme_test.go`

**Interfaces:**
- Consumes: `theme.Presets`, `theme.PresetNamed`, `theme.SuggestPreset` (B1); `themeResult` (A4).
- Produces: `func UsePreset(app App, name string, print, dryRun bool) (*Result, error)`; spec ID `theme use`, arg `name` (ArgText, not required), flags `print`; global `dir`, `dry-run`, `json` as for import.

- [ ] **Step 1: Write failing tests:**
  - `TestThemeUse_List`: no arg prints exactly the spec's block (header `starting themes (alder-ui ` + `Version()` + `):`, five lines name-padded to 12, the trailing hint line), exit 0.
  - `TestThemeUse_Writes`: on a temp scaffold, `theme use paper` makes `App.alder`'s declaration byte-equal to `format.Theme` of `generate.ThemeText(PresetNamed("paper").Theme)`; summary line as in Global Constraints.
  - `TestThemeUse_Replaces`: after `use paper`, `use pebble` replaces and says `replaced the theme that was there`.
  - `TestThemeUse_PrintDryRun`: `--print` writes stdout only; `--dry-run` writes nothing.
  - `TestThemeUse_JSON`: `--json` has `ImportTheme`'s shape plus `"preset":"paper"`.
  - `TestThemeUse_Unknown`: `use papr` exits 1, stderr has ``no starting theme `papr`; did you mean `paper`?`` and the list.
  - `TestThemeUse_AppDoesNotParse`: an `App.alder` with a syntax error → refused with the parser's message, file unchanged.
  - `TestThemeUse_PresetsCheck`: `generate.ThemeText(p.Theme)` returns no error for every `theme.Presets()` entry.
  - `TestHelpJSON_ThemeUse`: `alder help --json` lists `theme use`.
- [ ] **Step 2:** FAIL. **Step 3:** implement. **Step 4:** PASS.
- [ ] **Step 5:** Commit `alder theme use (#38)`.

### Task B3: `alder new --theme`

**Files:**
- Modify: `cmd/alder/new.go` (flag loop ~L45-52, between scaffold write ~L99-110 and `runMigrateTo` L112, recap ~L121-143, `newUsage`), `internal/generate/specs.go` (`new`'s `theme` flag, help lists `Presets()` names at run time)
- Test: `cmd/alder/new_test.go`

**Interfaces:**
- Consumes: `generate.UsePreset` (B2), `theme.PresetNamed`/`SuggestPreset` (B1).

- [ ] **Step 1: Write failing tests:** `TestNew_Theme` (`alder new x --theme pebble`: declaration present, recap has the line `theme: pebble`, `alder build` in the app passes); `TestNew_ThemeUnknown` (`--theme nope`: exit 1 with the suggestion, target dir does not exist); `TestNew_ThemeZinc` (declaration written, equals Zinc's); `TestNew_BareTheme` (`--bare --theme paper`: if the bare scaffold has an `App.alder`, declaration written and build passes; if not, refused before the directory is created — pick by reading `scaffold.BareFiles`, and pin the behaviour in the test).
- [ ] **Step 2:** FAIL. **Step 3:** implement: validate the name before `CheckTarget`; run `UsePreset(app, name, false, false)` after the scaffold loop, add `app/App.alder` to `written` if not there, suppress its output to the one recap line. **Step 4:** PASS.
- [ ] **Step 5:** Commit `alder new --theme (#38)`.

### Task B4: Book and site for PR B

**Files:** `book/C-design-system.md` (new `### Starting from a theme` before `### Changing a token`; licences line adds Catppuccin), `book/15-generators.md` (generator list gains `alder theme use`), `book/11-tooling.md` (generator table row near L234 if theme import is listed there), `book/site` regenerated, `experiments/test_build_context.py` cap (`test_real_reference_cut`, ~L355-380) bumped with a history line if it trips.

- [ ] **Step 1:** Write the section: the five and their purposes (a table), by copy ("the app owns it from here"), `alder theme use`, `--theme` on `new`. Use at least one `alder fragment` block only if it is a complete `theme { … }` (TestAppendixC_ThemeFragments checks them in the scaffold).
- [ ] **Step 2:** Run `go test ./book && python3 book/site/build.py && python3 book/site/test_site.py && python3 -m pytest experiments/test_build_context.py -q` (or the repo's runner for it) — PASS after the cap bump.
- [ ] **Step 3:** `scripts/test.sh`, commit `book: starting themes (#38)`, open PR B.

## PR C — the configurator's Start from

### Task C1: `presets` in `/state`

**Files:** `cmd/alder/theme_ui.go` (`themeState` struct ~L35, `themeState()` ~L201), `cmd/alder/theme_ui_test.go`

**Interfaces:**
- Produces: `/state` JSON gains `"presets": [{"name","title","purpose","theme": {root,light,dark}}]` in `Presets()` order.

- [ ] **Step 1:** `TestThemeState_Presets`: GET `/_dev/theme/state` on the blog → five presets, first `zinc`, each `theme.light` has 32 entries. FAIL. **Step 2:** implement. **Step 3:** PASS, commit `theme page: presets in state (#38)`.

### Task C2: Start-from select and swatches

**Files:** `cmd/alder/theme_ui_assets.go` (header bar ~L30-40, `themeJS` ~L291, panel CSS), `cmd/alder/testdata/browser/theme.cjs`, `cmd/alder/theme_ui_assets_test.go`

**Interfaces:**
- Consumes: `/state.presets` (C1).
- Produces: `<select id="start-from">` between `#scheme` and `#reset`; five `<span class="tp-sw">` swatches after it showing the chosen preset's light `primary, background, accent, foreground, destructive`.

- [ ] **Step 1: Write failing tests.** Go: `TestPanel_StartFrom` (served HTML has `id="start-from"` positioned after `id="scheme"` and before `id="reset"`). Browser (`theme.cjs`): choose `paper` → every control shows Paper's value; the gallery `al-button`'s computed background equals Paper's light primary; `App.alder` on disk unchanged; reload → the select still shows Paper and controls hold Paper's values; Reset → controls back to the committed theme and the select shows `—`; choose `brutal`, Write → `App.alder` holds Brutal's declaration.
- [ ] **Step 2:** Run `go test ./cmd/alder -run Panel_StartFrom` and `go test -tags browser -run Browser ./cmd/alder` — FAIL.
- [ ] **Step 3: Implement.** Options are each preset's `title` with `title=` attribute `purpose`; a leading `—` option is selected whenever the working theme deep-equals no preset. Choosing replaces the whole working theme (all three maps), refills controls, enables Write, saves to sessionStorage like any control. Swatches use the panel's neutral styles (`TestPanelCSS_NoThemeTokens` must still pass).
- [ ] **Step 4:** PASS both. Visual pass: each preset in light and dark in the gallery and app tabs. Book: one sentence on Start from in "### Trying a look: /_dev/theme". Commit `theme page: Start from (#38)`, `scripts/test.sh`, open PR C.

## PR D — colour parsing and editor swatches

### Task D1: `theme.ParseColour`

**Files:** Create `internal/theme/colour.go`, `internal/theme/colour_test.go`

**Interfaces:**
- Produces: `type RGBA struct{ R, G, B, A float64 }` (0–1 sRGB, gamma-encoded, clipped); `func ParseColour(text string) (RGBA, bool)`.

- [ ] **Step 1:** Table test `TestParseColour` (within 0.002 per channel): `#fff`, `#000`, `#ff000080` (A 0.502), `#abcd`; `rgb(255 0 0)`, `rgba(255, 0, 0, 0.5)`, `rgb(100% 0% 0% / 50%)`; `hsl(0 100% 50%)` red, `hsla(120, 100%, 50%, 1)`; `hwb(0 0% 0%)` red; `oklch(1 0 0)` white, `oklch(0 0 0)` black, `oklch(0.577 0.245 27.325)` ≈ `(0.906, 0, 0.043)` within 0.01, `oklch(0.205 0 0)` ≈ shadcn grey `#171717`; `oklab(…)`, `lab(…)`, `lch(…)` one each against known values; `color(srgb 1 0 0)`; `color(display-p3 1 0 0)` clipped to `(1, 0, 0)`; `red`, `rebeccapurple`, `transparent` (A 0); `oklch(none 0 0)` = black. Not ok: `currentColor`, `color-mix(…)`, `light-dark(…)`, `var(--x)`, `""`, `oklch(1 0)`.
- [ ] **Step 2:** FAIL. **Step 3:** implement with the CSS Color 4 matrices (OKLab→LMS→linear sRGB; Lab D50→XYZ→Bradford D65→linear sRGB; P3→XYZ→sRGB), then the sRGB transfer function, then clip. The 148 named colours as a map. **Step 4:** PASS. Commit `theme: ParseColour (#38)`.

### Task D2: `documentColor` and `colorPresentation`

**Files:** Create `internal/lsp/color.go`, `internal/lsp/color_test.go`; modify `internal/lsp/protocol.go` (`capabilities()` L112), `internal/lsp/server.go` (request map ~L36-63)

**Interfaces:**
- Consumes: `theme.ParseColour` (D1), `theme.Defaults()`, `theme.ByCSS`/`Lookup`.
- Produces: handlers `documentColorReq`, `colorPresentationReq` with the `handler` signature; LSP types `ColorInformation{Range, Color}`, `Color{Red, Green, Blue, Alpha float64}`, `ColorPresentation{Label string; TextEdit *TextEdit}`.

- [ ] **Step 1: Write failing tests** (helpers `start`, `blogCopy`, `openThemeApp` from `helpers_test.go`/`theme_test.go`):
  - `TestInitialize_ColorProvider`: capabilities has `colorProvider: true`.
  - `TestDocumentColor_Blog`: one entry per colour string literal in the blog `App.alder`'s `light`/`dark` records; each range covers the literal's contents (inside the quotes, via `spanRange` shrunk by one byte each side, then converted to the client's encoding the way diagnostics are).
  - `TestDocumentColor_Values`: a theme with `primary: "#ff0000"` → `(1,0,0,1)`; `oklch(0.577 0.245 27.325)` ≈ `(0.906,0,0.043)` within 0.01; `dark { ring: "var(--primary)" }` resolves to the dark record's `primary`, and to `Defaults()["--primary"].Dark` when the record lacks it.
  - `TestDocumentColor_Skips`: `radius` and `fontSans` fields yield nothing; an unparsable colour yields nothing; a non-App module, an `App.alder` without a theme, and an `App.alder` with a syntax error each return `[]` (not null, no error).
  - `TestColorPresentation`: `(0, 0.5, 1, 1)` → one presentation `#0080ff` with a `TextEdit` over the given range; alpha 0.5 → `#0080ff80`.
- [ ] **Step 2:** `go test ./internal/lsp -run 'Color|ColorProvider'` — FAIL.
- [ ] **Step 3: Implement.** Find the App module from the analysis (`a.prog.Modules["App"]`, `File == rel`), its `*syntax.ThemeDecl`, walk `light`/`dark` record fields whose `theme.Lookup(field).Kind == Colour` and `Value.Kind == "string"`. Register both methods as fast requests.
- [ ] **Step 4:** PASS. Commit `lsp: colour swatches in the theme declaration (#38)`.

### Task D3: Neovim note and book

**Files:** `~/projects/alder.nvim/README.md` (separate repo, own small PR), `book/11-tooling.md` (`### What you get` ~L1856)

- [ ] **Step 1:** Check Neovim's support: `nvim --version`, then in a scratch config with the alder LSP attached, `:lua vim.lsp.document_color.enable(true)` on `examples/blog/app/App.alder`. Record in alder.nvim's README whether swatches show natively (and the Neovim version) or which plugin is needed.
- [ ] **Step 2:** Add the swatch bullet to chapter 11's "What you get". `go test ./book`, site rebuild. Commit `book: colour swatches (#38)`, `scripts/test.sh`, open PR D (and the alder.nvim PR).

## PR E — the token table from Zinc (Fable review before merge)

### Task E1: `theme.Equivalent`

**Files:** Create `internal/theme/equivalent.go`, `equivalent_test.go`; modify `internal/theme/import.go` (`resolveDark` L224-231 uses it)

**Interfaces:**
- Consumes: `ParseColour` (D1), `Families`.
- Produces: `func Equivalent(k Kind, a, b string) bool`.

- [ ] **Step 1:** `TestEquivalent`: `oklch(1 0 0 / 10%)` ≡ `rgb(255 255 255 / 0.1)`; `#ffffff` ≡ `white`; `#000000` ≢ `#020202` (2/255 apart); `rgb(0 0 0)` ≡ `rgb(0.5 0 0)` (0.5/255 apart); `0` ≡ `0px` ≡ `0rem`; `0.5rem` ≢ `8px`; shadows layer by layer (`0 1px 2px 0 rgb(0 0 0 / 0.05)` ≡ `0px 1px 2px 0px rgb(0 0 0 / 5%)`); fonts as lists (`"Geist", sans-serif` ≡ `Geist, sans-serif`); `var(--primary)` ≡ only itself. `TestImport_DarkEqualsRootEquivalent`: a CSS import whose `:root` has `--radius: 0` and `.dark` has `--radius: 0px` keeps `radius` and reports nothing unplaced.
- [ ] **Step 2:** FAIL. **Step 3:** implement; colours within 1/255 per channel. **Step 4:** PASS. Commit `theme: Equivalent (#38)`.

### Task E2: `Tokens()` and `Defaults()` from Zinc

**Files:** Modify `internal/theme/theme.go` (delete the `Tokens` literal; `Token` gains `Description string`; `Tokens()`; `Defaults()`), `theme_test.go`; callers: `cmd/alder/theme_ui.go:203-204`, `cmd/alder/theme_ui_assets_test.go:46`, `cmd/alder/theme_ui_test.go:86,87,136`, `internal/lsp/complete.go:932`, `internal/lsp/hover.go:338`, `internal/generate/theme.go:111`, `book/theme_test.go:~53`, doc comments at `internal/ir/lower_theme.go:14`, `internal/check/theme.go:14`, `internal/generate/theme.go:92`; any package-level `var` initialised from the table becomes lazy.

**Interfaces:**
- Consumes: `loadDTCG`, `dtcgDoc.cssText` (A2/A3) — **not** `ImportDTCG`, which calls `ByCSS` and would recurse into `Tokens()`. `Equivalent` (E1). `FieldName` (existing) derives `Field`.
- Produces: `var Tokens = sync.OnceValue(func() []Token {…})` (call sites `theme.Tokens()`); `Defaults` keeps its type `func() map[string]Default`, now from Zinc (theme-wide tokens have `Light == Dark`; derived radii are plain lengths).

- [ ] **Step 1: Write failing tests:**
  - `TestTokens_FromZinc`: 47 tokens; order equals the deleted literal's order with `borderWidth` last of the theme-wide (copy the old `[]string` of CSS names into the test as the expected order); `Field` equals the old field names; every `Description` non-empty; `ByCSS("--al-border-width").Field == "borderWidth"`; light and dark files declare the same colour set.
  - `TestTokens_MatchVendoredStylesheet` (kept, now over `Tokens()`).
  - `TestDefaults_EveryToken` rewritten: for every token, `Equivalent(kind, Defaults()[css].Light, alCSSLight[css])` and the same for dark, where `alCSS*` come from `Blocks()`; the four `calc()` radii are evaluated at `--radius` (`calc(var(--radius) - 4px)` at `0.625rem` = `0.375rem`, taking 1rem = 16px) before comparing.
  - `TestDefaults_RadiiPlain`: `Defaults()["--radius-sm"].Light == "0.375rem"`.
- [ ] **Step 2:** FAIL (compile errors at callers count as failing).
- [ ] **Step 3: Implement**, then switch every caller to `Tokens()`. `Order(css)` keeps meaning "index in the table".
- [ ] **Step 4:** `go vet ./... && scripts/test.sh` — PASS with every golden unchanged except where PR A already added `borderWidth`. Any other golden diff is a bug: stop and find the order difference.
- [ ] **Step 5:** Commit `theme: token table from the vendored Zinc preset (#38)`.

### Task E3: Descriptions in hover, completion, configurator

**Files:** `internal/lsp/hover.go` (`themeHover` L326), `internal/lsp/complete.go` (~L932), `cmd/alder/theme_ui.go` (`themeToken` gains `desc`), `cmd/alder/theme_ui_assets.go` (control label `title`), tests `internal/lsp/theme_test.go`, `cmd/alder/theme_ui_test.go`

- [ ] **Step 1:** Tests: `TestHover_ThemeField` expects the token's `Description` line; `TestComplete_ThemeLight` items carry `detail == Description`; `TestThemeState_Blog` tokens carry `desc`. FAIL.
- [ ] **Step 2:** Implement; PASS; `scripts/test.sh`.
- [ ] **Step 3:** Commit `theme: token descriptions in hover, completion and the page (#38)`, open PR E, dispatch the Fable whole-branch review (it touches every caller), fix, merge.
- [ ] **Step 4:** After merge: `go install ./cmd/alder`, close #38's phase 3 checklist items, update memory.
