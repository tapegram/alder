# Theme phase 2 (#38): configurator at `/_dev/theme` and `alder theme import` — implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** a live theme configurator served by `alder run` at `/_dev/theme`, and `alder theme import`, both writing the `theme { … }` declaration in `App.alder` through one validated edit.

**Architecture:** a pure importer and a `Theme` value type in `internal/theme`; a standalone theme check exported from `internal/check` (the checker's own `themeFields`, so messages are E0042's verbatim); `format.Theme` prints; `generate.ThemeEdit` validates, prints and splices, then `Apply` writes. The CLI is a generator Spec; the page is supervisor-only routes in `cmd/alder`, assets as Go strings.

**Tech Stack:** Go (stdlib only, the repo's import rule), hand-written HTML/CSS/JS, Playwright browser tests (`//go:build browser`).

**Spec:** `docs/superpowers/specs/2026-10-30-theme-configurator-import-design.md` (binding; read it with this plan; **(call)** items are settled).

## Global Constraints

- No new Go dependencies; nothing changes in `rt/`, `internal/ir`, `internal/gen` or the grammar.
- The page, handlers and importer live in the `alder` binary only (`cmd/alder`, `internal/…`), never `rt/web`.
- Nothing reaches `format.Theme` that has not passed `check.ThemeFields` with no errors.
- Body limit for `/_dev/theme/import` and `/_dev/theme/write`: 256 KiB (`256 << 10`); over → 413.
- Tokens order (`theme.Tokens`) is the only field order the printer, the page's controls and `/state` use.
- The panel's stylesheet uses no theme token (no `var(--…)` of a `theme.Tokens` CSS name) — a test enforces it.
- Messages and comments in the repo's prose style: backticked names, a colon before the fix.
- Diagnostic code is E0042 (`diag.ThemeDecl`); no new codes.
- Working-hours rule: no agent work 09:00–17:00 America/New_York on weekdays.

## Review Focus

1. **The blog's comment above `theme`.** Replacing a declaration keeps the comment lines above it and everything outside the declaration's span byte-identical (Task 3 test `TestThemeEdit_ReplaceKeepsLeadingComment`).
2. **Write after an editor save.** The page loaded theme A; the person then edited another declaration in `App.alder` in their editor; Write keeps the editor's change, because `App.alder` is re-read at write time (Task 5 test `TestThemeWrite_KeepsConcurrentEdit`).
3. **`App.alder` that does not parse.** `/state` answers `theme: null` and the parse error; `/write` refuses with that diagnostic and the file is unchanged (Task 5 test `TestThemeState_ParseError`, `TestThemeWrite_ParseErrorRefused`).
4. **Dark scheme under `@media (prefers-color-scheme: dark) { :root { … } }`.** Its `:root` declarations go to Dark, not Light (Task 1 test `TestImport_MediaDarkRoot`).
5. **Clearing every field and writing.** The declaration and one surrounding blank line are deleted; the write is not refused for "no declaration matches the guard" (Task 3 test `TestThemeEdit_EmptyDeletes`, Task 5 `TestThemeWrite_EmptyDeletes`).

---

## File map

| file | responsibility |
|---|---|
| `internal/theme/value.go` (new) | `Theme` value type, `Families`, `Theme.Empty` |
| `internal/theme/import.go` (new) | CSS scanner, `Import`, `Unplaced`, `UnplacedCSS` |
| `internal/check/theme.go` | `ThemeFields` export over the existing `themeFields` |
| `internal/format/theme.go` (new) | `Theme(fields) string` |
| `internal/generate/theme.go` (new) | `ThemeOf`, `ThemeDecl`→fields, `ThemeEdit`, `WriteTheme`, `ImportTheme` |
| `internal/generate/specs.go` | `theme import` Spec |
| `internal/generate/apply.go` | `Result.Import *ThemeImport` + its text rendering |
| `cmd/alder/main.go`, `generate.go` | `case "theme"`, `--print` output, JSON extras |
| `cmd/alder/theme_ui.go` (new) | routes, gate, state, import, write |
| `cmd/alder/theme_ui_assets.go` (new) | page HTML, panel CSS, JS, gallery markup |
| `cmd/alder/run.go` | `ServeHTTP` dispatch to `/_dev/theme…` before the proxy |
| `book/C-design-system.md`, `11-tooling.md`, `15-generators.md`, `book/site` | docs |

Dependencies: Tasks 1 and 2 are independent; 3 needs 1+2; 4 and 5 need 3; 6 needs 5; 7 last.

---

### Task 1: `theme.Theme` and the importer

**Files:**
- Create: `internal/theme/value.go`, `internal/theme/import.go`, `internal/theme/import_test.go`
- Create: `internal/theme/testdata/import/{shadcn-v4,shadcn-v3,tweakcn,sidebar-only,garbage,media-dark}.css` (realistic exports: shadcn v4 from ui.shadcn.com themes with `@theme inline` and `@custom-variant dark`; v3 with bare `H S% L%` and one `/ 0.5` alpha; tweakcn with fonts, `--shadow-*` family, `--tracking-*`, `--spacing`; `garbage` = only `@theme inline { … }`; `media-dark` = `:root{}` light + `@media (prefers-color-scheme: dark) { :root { … } }`)

**Interfaces:**
- Produces:
  ```go
  // value.go
  type Theme struct {
      Root  map[string]string `json:"root"`  // field name → value; Font values are families joined by ", "
      Light map[string]string `json:"light"`
      Dark  map[string]string `json:"dark"`
  }
  func (t Theme) Empty() bool
  func Families(v string) []string // split on commas outside quotes, trim, drop surrounding ' or ", drop empties
  // import.go
  type Unplaced struct{ Block, Name, Value, Reason string } // Block = the selector text as written, e.g. ":root, .light"
  type Skipped struct{ N int; Kinds []string }              // Kinds e.g. ["@theme inline", "@custom-variant"], in source order
  func Import(css string) (Theme, []Unplaced, Skipped, error)
  func UnplacedCSS(us []Unplaced) string                  // valid CSS, grouped by Block in first-seen order, `/* reason */` per line
  ```

- [ ] **Step 1: Write failing tests** in `import_test.go`, one per spec "Verification → Importer" bullet plus:
  - `TestFamilies` — `"'Inter', ui-sans-serif, system-ui, sans-serif"` → `["Inter","ui-sans-serif","system-ui","sans-serif"]`; `"\"Geist Mono\", monospace"` → `["Geist Mono","monospace"]`.
  - `TestImport_ShadcnV4` — every colour token present in both `Light` and `Dark`; `Root["radius"] == "0.625rem"`; `Skipped.N == 2`.
  - `TestImport_ShadcnV3WrapsHSL` — `Light["primary"] == "hsl(222.2 47.4% 11.2%)"`; the alpha case → `"hsl(217.2 32.6% 17.5% / 0.5)"`.
  - `TestImport_TweakcnUnplaced` — the spec's list of names unplaced with Reason `"not a token of alder-ui " + Version()`; dark `--shadow-sm` with Reason `"theme-wide in alder-ui; the dark value differs"`; `Root["fontSans"]` is the unquoted joined stack.
  - `TestImport_RemovedName` — `--al-font-mono` → Reason equals `` "`--al-font-mono` was removed in alder-ui v0.4: " + Removed["alFontMono"] `` (map key per `Removed`'s lowerCamel form).
  - `TestImport_BadValues` — `var(--sidebar-accent-2)` Reason ``"`var(--sidebar-accent-2)` is not a colour token"``; `--radius: calc(0.5rem + 2px)` Reason starts with ``"`--radius: calc(0.5rem + 2px)` is not a length alder-ui accepts"``.
  - `TestImport_SelectorUnplaced` (`.sidebar { --primary: red }` → Reason ``"`.sidebar` is not a scheme selector"``), `TestImport_ImportantDropped`, `TestImport_LastWins`, `TestImport_NoTheme` (garbage → error text ``"no theme in the input: expected a `:root { --primary: … }` or `.dark { … }` block"``; the CLI substitutes the file name for "the input"), `TestImport_MediaDarkRoot` (Review Focus 4), `TestUnplacedCSS_Valid` (golden string; reasons as comments; grouped by block).

- [ ] **Step 2: Run** `go test ./internal/theme/ -run 'Import|Families|Unplaced'` — expect FAIL (undefined).

- [ ] **Step 3: Implement.** Hand-written scanner: strip `/* */`, respect strings and parens, recurse into `@media`/`@layer`/`@supports` bodies (a `@media` whose prelude contains `prefers-color-scheme: dark` makes its `:root`/`html` rules dark), skip and count `@theme`, `@import`, `@tailwind`, `@plugin`, `@custom-variant` (Kinds text = at-keyword plus first prelude word, e.g. `@theme inline`). Selector routing and value rules exactly as the spec's tables. Each placed value is checked with the kind check (`ColourOK`/`Alias`→colour via `ByCSS`, `LengthOK && !NegativeLength`, `FamilyOK` per family, `ShadowOK`); failure → Unplaced with the shape wording.

- [ ] **Step 4: Run** `go test ./internal/theme/` — PASS.

- [ ] **Step 5: Commit** `theme: Theme value and the CSS importer (#38 ph2)`.

---

### Task 2: standalone check and `format.Theme`

**Files:**
- Modify: `internal/check/theme.go`; Test: `internal/check/theme_fields_test.go` (new)
- Create: `internal/format/theme.go`; Test: `internal/format/theme_test.go` (new)

**Interfaces:**
- Produces:
  ```go
  // internal/check
  // ThemeFields checks a theme declaration's fields alone, as checkTheme does in App
  // (same messages, code E0042), reporting at the fields' spans.
  func ThemeFields(fs []*syntax.DeployField) []diag.Diagnostic
  // internal/format
  // Theme prints `theme { … }` for fs as the formatter would (deployRecord, Width), no trailing newline; "" for no fields.
  func Theme(fs []*syntax.DeployField) string
  ```

- [ ] **Step 1: Failing tests.**
  - `TestThemeFields_SameMessagesAsChecker` — table: a colour at top level, `radius: "-1px"`, `light: { primary: "var(--nope)" }`, `light: { primary: "a\"b" }`, a `primary` that aliases itself; each gives exactly one diagnostic, Code `"E0042"`, Message identical to what the corpus file `testdata/corpus/E0042-*` for that case expects (reuse those expected strings).
  - `TestThemeFields_OK` — the blog's three fields → no diagnostics.
  - `TestTheme_BlogByteForByte` — parse `examples/blog/app/App.alder`, take its `ThemeDecl.Fields`, `format.Theme` equals lines 38–42 of that file joined by `\n`.
  - `TestTheme_Empty` — `Theme(nil) == ""`; `TestTheme_OneFieldOneLine` — `theme { radius: "0.5rem" }`.

- [ ] **Step 2: Run** `go test ./internal/check/ ./internal/format/ -run 'ThemeFields|TestTheme_'` — FAIL.

- [ ] **Step 3: Implement.** `ThemeFields` builds the minimal checker state `themeFields` needs (a fresh `diag.Bag`; a `checker`/`modCtx` with nothing else) and calls `themeFields(fs, "")`; `checkTheme` is unchanged. `format.Theme` builds a printer with no comments and renders `Concat(Text("theme "), p.deployRecord(nil-group, fs))` at `Width` (the group argument is whatever `braced` needs for comment lookup; none here). It does not sort — Task 3 passes fields in order.

- [ ] **Step 4: Run** `go test ./internal/check/ ./internal/format/` — PASS (whole packages, corpus included).

- [ ] **Step 5: Commit** `check, format: standalone theme check and printer (#38 ph2)`.

---

### Task 3: `generate.ThemeEdit` and `WriteTheme`

**Files:**
- Create: `internal/generate/theme.go`, `internal/generate/theme_test.go`

**Interfaces:**
- Consumes: `theme.Theme`, `theme.Families`, `check.ThemeFields`, `format.Theme`, `srcedit.go` helpers, `Apply`, `runEdits`, `LoadWithErrors`.
- Produces:
  ```go
  // ThemeOf reads a declaration into the Theme shape (fonts joined by ", "); zero Theme for nil.
  func ThemeOf(d *syntax.ThemeDecl) theme.Theme
  // ThemeInvalid is ThemeEdit's refusal: the E0042 diagnostics, located on the declaration
  // (its span, or the insertion line when App has none). Nothing was printed or written.
  type ThemeInvalid struct{ Diags []diag.Diagnostic }
  func (e *ThemeInvalid) Error() string // the first message, "; +N more" when several
  func ThemeEdit(p *Program, th theme.Theme) ([]Edit, error)
  // WriteTheme is ThemeEdit + Apply for App (LoadWithErrors, WriteDespiteErrors).
  func WriteTheme(app App, th theme.Theme, dryRun bool) (*Result, error)
  ```

- [ ] **Step 1: Failing tests** (`scaffoldApp`, a blog copy, `editAt`):
  - `TestThemeEdit_InsertAfterSession` — scaffold; result has `session { }`, blank, the print, blank, then the old next line.
  - `TestThemeEdit_InsertAfterDeploy`, `TestThemeEdit_InsertAfterImports` (App with no `session`/`deploy`), `TestThemeEdit_InsertAfterModule` (no imports either).
  - `TestThemeEdit_ReplaceOnlyDeclLines` — blog with `primary` changed: diff of old vs new `App.alder` is exactly the one `primary` line.
  - `TestThemeEdit_ReplaceKeepsLeadingComment` (Review Focus 1).
  - `TestThemeEdit_TokensOrder` — `Light{"ring":…, "primary":…}` prints `primary` before `ring`; `Root` before `light` before `dark`.
  - `TestThemeEdit_EmptyDeletes` — empty Theme on the blog removes lines 38–42 and one adjacent blank line; the comment above stays (a person deletes it); scaffold + empty Theme → no edits.
  - `TestThemeEdit_Refused` — table: `Light{"primary": "a\"b"}`, `Light{"primary": "red\n}"}`, `Root{"primary": "red"}`, `Root{"radius": "-1px"}`, `Light{"primary": "var(--nope)"}`, `Root{"bogus": "1"}` → `*ThemeInvalid` with an E0042 message; `os.ReadFile` of App unchanged.
  - `TestWriteTheme_DespiteUnrelatedError` — scaffold plus a page with a type error elsewhere: `Written == true`.
  - `TestThemeEdit_GuardSet` — a non-empty theme's edit has a `Guard` true for a `*syntax.ThemeDecl` and false for a `*syntax.SessionDecl`; the empty theme's delete edit has `Guard == nil`.

- [ ] **Step 2: Run** `go test ./internal/generate/ -run Theme` — FAIL.

- [ ] **Step 3: Implement.** Build `[]*syntax.DeployField` from `th` in `Tokens` order (Root fields, then `light`/`dark` records; Font → `Kind "list"` of `Families`; others `Kind "string"`; empty records omitted); run `check.ThemeFields`; on errors return `*ThemeInvalid` with spans relocated to the target line. Re-parse App with `syntax.ParseFile(m.File, []byte(m.Source))` at edit time (from the loaded `Program`, which `runEdits` loads fresh per call). Splice per spec step 3 with `newSrcEdit`. `Guard` = is a `*syntax.ThemeDecl` — set only when the result contains a declaration (Review Focus 5).

- [ ] **Step 4: Run** `go test ./internal/generate/` — PASS.

- [ ] **Step 5: Commit** `generate: ThemeEdit and WriteTheme (#38 ph2)`.

---

### Task 4: `alder theme import`

**Files:**
- Modify: `internal/generate/specs.go` (a `themeSpecs()` group appended in `Specs()`), `internal/generate/apply.go` (`Result.Import`, rendering), `cmd/alder/main.go` (`case "theme"` → `runGroup("theme", themeUsage, …)`), `cmd/alder/generate.go` (`specExtra` for `theme import`; `--print` output)
- Create: `internal/generate/theme_import.go`, `cmd/alder/theme_test.go`

**Interfaces:**
- Consumes: `theme.Import`, `theme.UnplacedCSS`, `WriteTheme`, `ThemeOf`.
- Produces:
  ```go
  type ThemeImport struct {
      Theme    theme.Theme
      Unplaced []theme.Unplaced
      Skipped  theme.Skipped
      Counts   struct{ Light, Dark, Wide int }
      Replaced bool   // App had a theme before
      Dropped  []string // fields the old theme set that the import does not
      Print    string   // --print: the declaration; nothing written
  }
  // Result gains: Import *ThemeImport
  func ImportTheme(app App, name string, css []byte, print, dryRun bool) (*Result, error)
  ```
  Spec: `ID "theme import"`, Summary `"replace App's theme with a shadcn or tweakcn CSS export"`, Args `{Name:"file", Kind:ArgText, Required:true, Value:"<file.css>", Help:"the export, or - for stdin", Example:"theme.css"}`, Flags `{Name:"print", Kind:ArgBool, Summary:"write the declaration to stdout, not App.alder", Agent:true}` (`--dir`, `--dry-run`, `--json` are the common CLI flags). Run reads `file` (`-` = `os.Stdin`, capped at 256 KiB).

- [ ] **Step 1: Failing tests** in `cmd/alder/theme_test.go` (temp scaffold app via `tempAppAbs`, fixtures from Task 1's testdata):
  - `TestThemeImport_Writes` — shadcn-v4: exit 0; App contains `primary:`; stdout's first line matches `^theme: \d+ tokens → app/App.alder  \(light \d+, dark \d+, theme-wide \d+\)`.
  - `TestThemeImport_ReplacedSummary` — on the blog copy, the line ends `; replaced the theme that was there)` and lists dropped fields.
  - `TestThemeImport_UnplacedBlock` — tweakcn: output contains `not placed — paste into static/app.css if you want them:` and `UnplacedCSS`'s text indented two spaces; `skipped N blocks (…)` line.
  - `TestThemeImport_Print` — stdout is exactly `format.Theme` text + `\n`; App unchanged.
  - `TestThemeImport_DryRun`, `TestThemeImport_Stdin` (`-`), `TestThemeImport_NoTheme` (garbage: exit 1, stderr ``no theme in `garbage.css`…``).
  - `TestThemeImport_JSON` — keys `written, file, theme, unplaced, skipped, diagnostics`; exit 1 when `written` is false.
  - `TestHelpJSON_ListsThemeImport`.

- [ ] **Step 2: Run** `go test ./cmd/alder/ -run 'ThemeImport|HelpJSON_ListsTheme'` — FAIL.

- [ ] **Step 3: Implement.** Text rendering of `Import` in `RenderTextStyled` before the files list, with `style` (Bold on `theme:`, Dim on comments). `--print`: `runSpecWith`'s `seen` hook (or a branch in `genReport`) writes `res.Import.Print + "\n"` raw and returns 0. `specExtra` adds `theme`, `unplaced`, `skipped` (`skipped` = `Skipped.N`). Add the `theme import` line to `alder help`'s usage text and a `themeUsage` const.

- [ ] **Step 4: Run** `go test ./cmd/alder/ ./internal/generate/` — PASS.

- [ ] **Step 5: Commit** `alder theme import (#38 ph2)`.

---

### Task 5: supervisor routes (`/_dev/theme/state|import|write`)

**Files:**
- Create: `cmd/alder/theme_ui.go`, `cmd/alder/theme_ui_test.go`
- Modify: `cmd/alder/run.go` (`ServeHTTP`: `strings.HasPrefix(r.URL.Path, "/_dev/theme")` → `d.serveTheme(w, r)`, right after the `/_dev/reload` case)

**Interfaces:**
- Consumes: `os.ReadFile` + `syntax.ParseFile` of `<dir>/app/App.alder` for `/state` (no Program load, so a type error elsewhere does not hide the theme); `generate.ThemeOf`, `theme.Import`, `generate.WriteTheme`, `diagItem`.
- Produces:
  ```go
  func (d *devServer) serveTheme(w http.ResponseWriter, r *http.Request)
  func (d *devServer) themeAllowed(r *http.Request) bool // the gate
  type themeState struct {
      Version  string                       `json:"version"`
      File     string                       `json:"file"`     // "app/App.alder"
      Theme    *theme.Theme                 `json:"theme"`    // nil when App does not parse
      Error    string                       `json:"error,omitempty"`
      Defaults map[string]theme.Default     `json:"defaults"` // keyed by FIELD name; theme-wide in .Light
      Tokens   []themeToken                 `json:"tokens"`
      Building bool                         `json:"building"` // child == nil || errs.kind == "building"
  }
  type themeToken struct{ Field, CSS, Kind, Scope string } // json lowercase; Kind = Kind.String(); Scope "scheme"|"root"
  ```
  `GET /_dev/theme` serves `themePage` (Task 6; a stub `const themePage = "<!doctype html><title>theme</title>"` here). Maps in `Theme` are never `null` in JSON (normalise to `{}`).

- [ ] **Step 1: Failing tests** (httptest against `newDevServer` on a temp blog copy; no child):
  - `TestThemeState_Blog` — `theme.light.primary == "oklch(0.51 0.23 277)"`, `root.radius == "0.5rem"`, `len(tokens) == len(theme.Tokens)`, `defaults.primary.light != ""`, `building == true`.
  - `TestThemeState_ParseError`, `TestThemeWrite_ParseErrorRefused` (Review Focus 3).
  - `TestThemeWrite_Valid` — `{written:true, file:"app/App.alder"}`, file has the new value.
  - `TestThemeWrite_Invalid` — `{written:false, diagnostics:[{code:"E0042",…}]}`, file unchanged.
  - `TestThemeWrite_KeepsConcurrentEdit` (Review Focus 2), `TestThemeWrite_EmptyDeletes`.
  - `TestThemeImport_Route` — returns `theme`, `unplaced`, `skipped`; file unchanged.
  - `TestThemeGate` — table, each 403 except the last: `Sec-Fetch-Site: cross-site`; `Origin: http://evil.example`; `Host: evil.example:PORT` (rebinding); POST `Content-Type: text/plain` (415 rather than 403); 300 KiB body (413); same-origin JSON POST (200).

- [ ] **Step 2: Run** `go test ./cmd/alder/ -run 'TestTheme(State|Write|Import_Route|Gate)'` — FAIL.

- [ ] **Step 3: Implement.** Gate: `Sec-Fetch-Site` must be absent, `same-origin` or `none`; `Origin`, when present, must equal `"http://" + r.Host`; `r.Host` must be `localhost`, `127.0.0.1` or `[::1]` with the port of `d.addr` (or `d.addr`'s own host when it names one). Bodies via `http.MaxBytesReader`. Write: `generate.WriteTheme(generate.App{Dir: d.dir, Migrate: migrateFor}, th, false)`; `*generate.ThemeInvalid` and `res.Diagnostics` mapped to `diagItem` (reuse the conversion `parseDiags` uses, or add `diagItemOf(diag.Diagnostic) diagItem` in `devui.go`).

- [ ] **Step 4: Run** `go test ./cmd/alder/` — PASS.

- [ ] **Step 5: Commit** `alder run: /_dev/theme state, import and write routes (#38 ph2)`.

---

### Task 6: the page (panel, preview, gallery)

**Files:**
- Create: `cmd/alder/theme_ui_assets.go` (`themePage` replacing the stub; `themeCSS`, `themeJS`, `themeGallery` as Go string consts, composed as `rt/web/devpanel_ui.go` does), `cmd/alder/theme_ui_assets_test.go`, `cmd/alder/theme_ui_browser_test.go`, `cmd/alder/testdata/browser/theme.cjs`

**Interfaces:**
- Consumes: Task 5's routes and JSON; `elements.Builtin()`; `theme.Tokens`.
- Produces: the page as the spec's "The page" section fixes it (layout, controls per kind, preview mechanism, sessionStorage keyed `"alder-theme:" + file` holding `{committed, working}`, every access in `try/catch`, Write disabled while working == committed or `building`, polling `/state` each second while `building`, reload on `/_dev/reload`). DOM hooks the browser test relies on: each control row `[data-field="<field>"]` with its text input `input.value`, clear button `.clear`; `#scheme` switch (`data-scheme="light|dark"`); `#write`, `#reset`, `#import`; `#tab-gallery`, `#tab-app`, `iframe#app`.

- [ ] **Step 1: Failing unit tests** (`theme_ui_assets_test.go`):
  - `TestGallery_MatchesManifest` — every tag in `elements.Builtin().Elements` except `al-devpanel`, `al-toasts` appears as `<tag` in `themeGallery`; every `<al-[a-z-]+` in it is in the manifest.
  - `TestPanelCSS_NoThemeTokens` — no `var(--X` in `themeCSS` where `--X` is a `theme.Tokens` CSS name.
  - `TestThemePage_Links` — `themePage` links `/static/al/al.css`, `/static/al/al.js` and the datastar script path the app uses.

- [ ] **Step 2: Run** `go test ./cmd/alder/ -run 'Gallery|PanelCSS|ThemePage'` — FAIL.

- [ ] **Step 3: Implement** the assets.

- [ ] **Step 4: Run** `go test ./cmd/alder/` — PASS.

- [ ] **Step 5: Browser test** `TestThemeUI_Browser` (`//go:build browser`, `devRun` on the blog, `runBrowserScript(t, browserScript(t, "theme.cjs"), r)`), checks named as in the spec's "Browser" bullet: `primary-live` (button background `rgb(255, 0, 0)`, child generation unchanged), `dark-switch`, `clear-shows-default`, `write-rebuilds` (App.alder has the value, reload, working diff empty), `iframe-follows`. Run: `go test -tags browser ./cmd/alder/ -run TestThemeUI_Browser` — PASS.

- [ ] **Step 6: Visual pass** — screenshot the page in light and dark (Playwright, saved under the scratchpad), look at both; fix anything unreadable.

- [ ] **Step 7: Commit** `alder run: the /_dev/theme page (#38 ph2)`.

---

### Task 7: book and site

**Files:** `book/C-design-system.md` (two sections after "### Changing a token": "### Trying a look: /_dev/theme", "### Importing a theme"), `book/11-tooling.md` (link `/_dev/theme` beside `/_dev`), `book/15-generators.md` (`alder theme import` in the generator list), `book/site` (regenerated), caps in `book/*_test.go` or `experiments/test_build_context.py` only if a test fails on size.

- [ ] **Step 1:** Write the sections per the spec's "Formatter, Spec, book" bullet; the import section shows the CLI summary example and `--print` for agents.
- [ ] **Step 2: Run** `go test ./book/ && python3 book/site/build.py && python3 -m pytest -q book/site experiments` — PASS.
- [ ] **Step 3: Full verification:** `go test ./...`, `go vet ./...`, `go run ./cmd/alder build --dir examples/blog` — all PASS.
- [ ] **Step 4: Commit** `book: /_dev/theme and alder theme import (#38 ph2)`.
