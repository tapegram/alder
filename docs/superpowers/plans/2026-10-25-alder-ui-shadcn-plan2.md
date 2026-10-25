# alder-ui looks like shadcn (Plan 2 of 2: the Alder side and nightvote-replay) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship alder-ui `v0.4.1` inside Alder (fonts, theme script, old-token removal, injected-UI hardening, Appendix C with the migration guide), then migrate nightvote-replay: first the #23 qualifier migration, then the design-system migration with before/after screenshots for the user's verdict that closes #21.

**Architecture:** alder-ui is vendored at a tag by `scripts/vendor-al.sh` into `rt/al/`, embedded by `rt/al` and `rt`, written into every build by `internal/buildkit`, and served by `rt/web`. Plan 2 adds the two Geist fonts (and their licence) to each of those four lists, sets `font/woff2` explicitly, swaps the last `--al-*` uses for shadcn tokens, makes `alder new`, the admin generator and the examples emit the theme script, and rewrites Appendix C. Compiler, checker and LSP do not change. nightvote-replay is migrated on its own branch with the installed `alder` binary, and its Render pin (`ALDER_REF`) moves last.

**Tech Stack:** Go 1.27 (Alder), Alder (`.alder` apps), hand-written CSS strings, Playwright (Node, `cmd/alder/testdata/browser/*.cjs`, `-tags browser`; nightvote-replay `scripts/browser/*.mjs`).

**Spec:** `docs/superpowers/specs/2026-10-24-alder-ui-shadcn-design.md` (§8.6, §9, §10 are this plan; §5.3 the rulings), companion `2026-10-24-alder-ui-shadcn-reference.md`. Plan 1 (`docs/superpowers/plans/2026-10-24-alder-ui-shadcn-plan1.md`) section "Interfaces Plan 2 consumes" holds the file names, the theme script line and the token-replacement table; executors read it. The Plan 2 items on tapegram/alder#21 (final-review leftovers and the scope note) are folded in below.

**Where the work happens:**
- Alder: `~/projects/alder`, branch `alder-ui-shadcn-21` (holds the spec). Before Task 2, `git rebase main` (it is 6 docs commits over `5f71de10`). One PR → main at the end (Task 6).
- nightvote-replay: `~/projects/nightvote-replay`, new branch `alder-ui-041` from `main` (main is 1 commit ahead of origin: `fe72684`, the wave C spec; keep it). **Never push `main` there without asking the user: a push to main redeploys Render** (`render.yaml` `autoDeployTrigger: commit`). Pushing the branch is fine.

**Model hints:** Task 3 (injected UI against the base layer, browser test) is Opus. All others are Sonnet.

## Decisions taken in this plan (D1–D6)

- **D1 Nested `al-page` (alder#21 item 1).** Alder's Layouts (scaffold, examples, admin fallback) wrap every view in one `<al-page>`, and the runtime adds none. alder-ui v0.4.1 documents that typography applies wherever *some* enclosing `al-page` is not plain. So in Alder the opt-out is: put `plain` on the Layout's own `<al-page>` (one Layout component per look if an app mixes them). No new Layout prop, no checker rule, no alder-ui change. Appendix C says so, and Task 4 asserts that generator output never nests an `al-page`.
- **D2 Licences.** `fonts/OFL.txt` is vendored, embedded, written into builds and served beside the fonts (OFL 1.1 requires the licence to travel with the font files). The repo-level `LICENSE` and `LICENSE-lucide` are vendored to `rt/al/` only (not embedded or served).
- **D3 Content-Type.** Go's built-in mime table has no `.woff2` and the OS table varies by host, so `serveStatic` sets `Content-Type: font/woff2` itself for `.woff2` names. Fonts use `DesignSystemCache` like the stylesheet (unversioned URL; spec §9).
- **D4 `color-scheme` meta stays.** `rt/web` keeps injecting `<meta name="color-scheme" content="light dark">`. The stylesheet is render-blocking and sets `color-scheme` per `:root`/`.dark`, so the meta only colours the pre-CSS canvas. Appendix C's "no white flash" section is rewritten to say this, and that a user who chose light on a dark OS can see that one blank frame dark. This is not dark-mode logic in the runtime (spec §5.3 ruling 2 is about `.dark`).
- **D5 Admin fallback shell** gains the theme script only (not `al.js` or `<al-toasts>`, which it lacks today; out of scope).
- **D6 nightvote-replay order.** The qualifier migration (Task 1) runs first, with the binary installed now (`alder devel+5f71de101cdc`, which has #23 and vendors v0.3.3). That makes the app build again so the "before" screenshots can be taken on the old look. The design-system migration (Task 7) runs after the Alder PR merges and the binary is reinstalled. `ALDER_REF` moves once, in Task 7, to the Alder merge commit.

## Global Constraints

- Vendored release: alder-ui tag `v0.4.1`. `rt/al/VERSION` = `v0.4.1`; the manifest `version` and the `al.css` banner (`/*! alder-ui v0.4.1 | …`) agree (`TestDesignSystemVersion`).
- Font files, names and URLs (Plan 1 interfaces): `rt/al/fonts/Geist-Variable.woff2`, `rt/al/fonts/GeistMono-Variable.woff2`, `rt/al/fonts/OFL.txt`; served and built at `/static/al/fonts/<name>` and `static/al/fonts/<name>`. `al.css` refers to them as `url("fonts/…")`, relative to itself, so the path must be exact.
- Theme script, byte-exact, in `<head>` **before** the `al.css` link:
  `<script>try{document.documentElement.classList.toggle("dark",localStorage.theme==="dark"||(!("theme" in localStorage)&&matchMedia("(prefers-color-scheme: dark)").matches))}catch(e){}</script>`
  A `<script>` body is raw text in Alder (book ch. 5, grammar `rawElement`), so the braces are not interpolation.
- After this plan, no `--al-` custom property other than `--al-width-narrow|normal|wide` appears outside `rt/al/` in code, tests, examples or the book (docs under `docs/superpowers/` are history and exempt; build outputs under `examples/*/build/` and `internal/scratch/build/` are ignored).
- Injected UI CSS (`panelUICSS`, `errorsCSS`, `pendingStyle`) uses only `var(--x, <literal fallback>)`: a fallback that is itself a `var()` or a CSS-wide keyword (`inherit`, `initial`, `unset`) does not count as literal.
- Compiler, checker (`internal/elements`), LSP: no change. Emitted element markup: no change.
- No agent or background work 9am–5pm America/New_York, Monday–Friday (weekends free).
- nightvote-replay: `ALDER_USAGE_NOTES.md` and `scratch.alder` (untracked) belong to the user; nothing may modify, format, stage or delete them.

## Review Focus

1. **Fonts with the wrong Content-Type.** On a host without `/etc/mime.types` (Render's Docker image), `http.ServeContent` would sniff the woff2 as `application/octet-stream`, and some browsers then refuse the font. Expect `font/woff2` regardless of host. Test: Task 2, `TestDesignSystemFonts` sets nothing in the mime table and asserts the header exactly.
2. **`alder fmt` touching the theme script.** Format-on-save and `alder fmt` run over every Layout. A formatter that reflows or escapes the raw `<script>` body would break dark mode silently. Expect the script byte-identical after formatting. Test: Task 4, `TestScaffoldLayoutThemeScript` formats the scaffold Layout and the admin fallback output and compares.
3. **Built apps missing the fonts.** `alder build` output runs from another directory with only its `static/`. Expect `static/al/fonts/*.woff2` and `OFL.txt` in the build, and the built server returning 200 for them. Test: Task 2 extends `cmd/alder/build_test.go`'s output list and `run_test.go`'s dev-server GETs.
4. **The injected UI on a Preflight page, dark.** The error strip, its sheet, the dev panel's minimised tab and the pending bar appear on pages whose `al.css` resets borders, margins and buttons, and on `.dark` pages. Expect each to look as it does today (border on the strip button, mono font, visible bar colour). Test: Task 3, `TestBrowser_InjectedUIUnderBaseLayer`.
5. **A nested `al-page` from generated code.** A generated page that wrapped itself in `<al-page>` inside the Layout's would get typography it cannot opt out of (D1). Expect generator output to contain no `<al-page` outside Layout/admin-shell files. Test: Task 4, `TestGenerateNoNestedPage`.

---

## Part A: nightvote-replay qualifier migration (before the Alder change)

### Task 1: `alder fmt` qualifier migration and "before" screenshots — Sonnet

Repository: `~/projects/nightvote-replay`, branch `alder-ui-041` from `main`.

**Files:**
- Modify: every tracked `app/**/*.alder` that `alder fmt` changes (expect 32 files; 18 carry the 356 E0205 sites, 2 carry the 3 E0204 sites)
- Create: `scripts/browser/shots.mjs`
- Modify: `.git/info/exclude` (add `.shots/`)

**Interfaces:**
- Produces: `node scripts/browser/shots.mjs <base-url> <key> <outdir> <label>` writing `<outdir>/<label>-{index,replay}-{light,dark}.png` (full page, 1280-wide viewport). Dark is `colorScheme: 'dark'` emulation, which drives both the old CSS-only auto-dark (v0.3.3) and the new theme script (no `localStorage.theme`). It reaches the Test page `/r/{cfg}` the way `scripts/browser/ux-smoke.mjs` does. Task 7 reuses it.
- Produces: `.shots/before-*.png` (git-ignored via `.git/info/exclude`), kept for Task 7.

- [ ] **Step 1: Confirm the binary and the starting state**

Run: `~/go/bin/alder version && git -C ~/projects/nightvote-replay status -sb`
Expected: `alder devel+5f71de101cdc`; branch `main` ahead 1; untracked only `ALDER_USAGE_NOTES.md` and `scratch.alder`. Then `git switch -c alder-ui-041`. Copy `scratch.alder` and `ALDER_USAGE_NOTES.md` to the session scratchpad as a safety copy.

- [ ] **Step 2: Record the failing check**

Run: `~/go/bin/alder check . 2>&1 | grep -oE 'E0[0-9]{3}' | sort | uniq -c`
Expected: `356 E0205` and `3 E0204` (if the counts moved, note the new ones and continue).

- [ ] **Step 3: Migrate**

Run: `~/go/bin/alder fmt app` (format the `app/` tree only, so the untracked root `scratch.alder` is never touched).
Expected: about 32 files rewritten. `git status --porcelain` lists only tracked `app/` files as modified; `scratch.alder` and `ALDER_USAGE_NOTES.md` are byte-equal to the safety copies (`cmp`).

- [ ] **Step 4: Check the clash aliases**

Run: `grep -n 'import .*MnSos' app/ReplayStatus.alder app/Sites/MnSos.alder`
Expected: `import Data.MnSos as DataMnSos` in both files and `import Sites.MnSos as SitesMnSos` in `ReplayStatus.alder` (the shortest unique joined suffix, #23). Spot-check `app/Cfg.alder:20` reads `Curve.Curve` and `app/ClarityXml.alder:50` reads `Night.Edits`.

- [ ] **Step 5: Run the full gate**

Run: `scripts/test.sh`
Expected: `alder check` clean, 253 tests pass, build ok, adapter Go tests pass, mutation ≥ 0.9.

- [ ] **Step 6: Commit the migration**

```bash
git add app
git commit -m "refactor: short import qualifiers (alder #23) via alder fmt"
```
`Dockerfile` keeps `ALDER_REF=393f986f…` for now (it would fail on this source, but nothing deploys until Task 7 moves it).

- [ ] **Step 7: Write `scripts/browser/shots.mjs`, run it on the old look**

Run the app locally (`REPLAY_KEY=$(cat ~/.config/nightvote-replay/replay_key) ALDER_SECRET=<any 32 hex> PUBLIC_BASE=http://localhost:8080 ALDER_DB=$(mktemp -d)/db ALDER_ADDR=:8080 ~/go/bin/alder run .` in the background, under a timeout), then:
Run: `cd scripts/browser && node shots.mjs http://localhost:8080 "$(cat ~/.config/nightvote-replay/replay_key)" ../../.shots before`
Expected: four PNGs `.shots/before-{index,replay}-{light,dark}.png`, the dark pair visibly dark (v0.3.3 follows the OS). Stop the server.

- [ ] **Step 8: Commit the script**

```bash
git add scripts/browser/shots.mjs
git commit -m "test: shots.mjs, light/dark screenshots of the styled pages"
```
Push the branch (`git push -u origin alder-ui-041`), not main.

---

## Part B: the Alder side (branch `alder-ui-shadcn-21`)

### Task 2: Vendor alder-ui v0.4.1 with fonts — Sonnet

**Files:**
- Modify: `scripts/vendor-al.sh` (header comment lines 1–8; copy fonts, `OFL.txt`, `LICENSE`, `LICENSE-lucide`)
- Vendored (by running the script): `rt/al/{al.css,al.js,custom-elements.json,VERSION,LICENSE,LICENSE-lucide}`, `rt/al/fonts/{Geist-Variable.woff2,GeistMono-Variable.woff2,OFL.txt}`
- Modify: `rt/al/al.go` (`//go:embed`, `Names`, package doc), `rt/embed.go:20` (and the comment at :8), `internal/buildkit/buildkit.go:163-183`, `rt/web/router.go` (`serveStatic`, ~1247-1266)
- Modify: `book/C-design-system.md` element attribute tables only (the manifest additions, so `book/appendix_test.go` stays green); line 38's shipped version → `v0.4.1`
- Test: `rt/embed_test.go`, `rt/web/router_test.go`, `cmd/alder/build_test.go:178,438`, `cmd/alder/run_test.go:412-437`

**Interfaces:**
- Produces: `al.Names` = `al.css`, `al.js`, `custom-elements.json`, `fonts/Geist-Variable.woff2`, `fonts/GeistMono-Variable.woff2`, `fonts/OFL.txt`; `al.File("fonts/Geist-Variable.woff2")` returns data, ETag, ok. `buildkit.DesignSystemFiles` = the same six names; `WriteDesignSystem` creates `static/al/fonts/`.
- Produces: GET `/static/al/fonts/<name>.woff2` → 200, `Content-Type: font/woff2`, `Cache-Control: DesignSystemCache`, ETag, 304 on `If-None-Match`.
- Manifest additions now visible to the checker and LSP (Plan 1 interfaces): `al-button` `variant` + `"link"`, `size` + `"xs" | "icon" | "icon-sm" | "icon-lg"`; `al-card` slots `description`, `action`; `al-page` `plain` (boolean); `al-dialog` slot `description`; `al-tabs` `variant` (`"default" | "line"`).

- [ ] **Step 1: Write the failing tests**

In `rt/embed_test.go`:
```go
func TestDesignSystemFonts(t *testing.T) {
	for _, name := range []string{"al/fonts/Geist-Variable.woff2", "al/fonts/GeistMono-Variable.woff2"} {
		b, err := Files.ReadFile(name)
		// want err == nil, len(b) > 10_000, and the woff2 magic: string(b[:4]) == "wOF2"
	}
	// al/fonts/OFL.txt exists and contains "SIL OPEN FONT LICENSE Version 1.1"
	// al/al.css contains `url("fonts/Geist-Variable.woff2")` and `url("fonts/GeistMono-Variable.woff2")`
}
```
In `rt/web/router_test.go`, `TestDesignSystemFontServed`: GET `/static/al/fonts/Geist-Variable.woff2` → 200, `Content-Type` == `"font/woff2"` exactly, `Cache-Control` == `DesignSystemCache`, non-empty ETag; a second GET with `If-None-Match: <etag>` → 304; GET `/static/al/fonts/OFL.txt` → 200 with a `text/plain` content type.
In `cmd/alder/build_test.go:178` add `static/al/al.js`, `static/al/fonts/Geist-Variable.woff2`, `static/al/fonts/GeistMono-Variable.woff2`, `static/al/fonts/OFL.txt` to the expected outputs (and the fonts to the list at :438). In `run_test.go` after the `al.css` GET, GET `/static/al/fonts/GeistMono-Variable.woff2` → 200, `font/woff2`.

- [ ] **Step 2: Run them to see them fail**

Run: `go test ./rt/ ./rt/web/ -run 'TestDesignSystem' && go test ./cmd/alder -run 'TestBuild|TestRun' -short`
Expected: FAIL (`al/fonts/...: file does not exist`, 404).

- [ ] **Step 3: Extend `scripts/vendor-al.sh` and vendor**

Add `mkdir -p "$dest/fonts"` and copies of `fonts/Geist-Variable.woff2`, `fonts/GeistMono-Variable.woff2`, `fonts/OFL.txt` into `$dest/fonts/`, and `LICENSE`, `LICENSE-lucide` into `$dest/` (D2); update the header comment. Keep the tag == manifest-version check.
Run: `scripts/vendor-al.sh v0.4.1`
Expected: `vendored alder-ui v0.4.1 into rt/al/`; `cat rt/al/VERSION` → `v0.4.1`.

- [ ] **Step 4: Add the fonts to the embed, build and serve lists**

- `rt/al/al.go`: embed and `Names` gain the three `fonts/…` names (keys keep the `fonts/` prefix; the ETag table covers them automatically).
- `rt/embed.go:20`: add `al/fonts/Geist-Variable.woff2 al/fonts/GeistMono-Variable.woff2 al/fonts/OFL.txt` to the explicit list (do not switch to a glob: `LICENSE*` must stay out).
- `buildkit.go`: `DesignSystemFiles` = `al.Names`'s six names; `WriteDesignSystem` does `os.MkdirAll(filepath.Dir(dst), 0o755)` per file.
- `router.go` `serveStatic` design-system branch: `if strings.HasSuffix(r.URL.Path, ".woff2") { w.Header().Set("Content-Type", "font/woff2") }` before `http.ServeContent` (D3).

- [ ] **Step 5: Run the tests to see them pass**

Run: same as Step 2.
Expected: PASS.

- [ ] **Step 6: Bring the manifest-derived expectations up to v0.4.1**

Run: `go test ./book/ ./internal/elements/... ./internal/lsp/... ./cmd/alder -short 2>&1 | grep -E '^(--- FAIL|FAIL|ok)'`
Fix only failures whose expectation encodes the v0.3.3 manifest: Appendix C's per-element attribute tables (add the additions listed in Interfaces, in manifest order, so `TestAppendixC_ManifestDrift` passes) and any completion/checker golden listing union values (regenerate with the package's `-update` flag where one exists, then review the diff is only the additions). Line 38 says `v0.4.1`. Any failure not explained by the manifest is a stop-and-report.
Expected: all listed packages `ok`.

- [ ] **Step 7: Commit**

```bash
git add scripts/vendor-al.sh rt/al rt/embed.go rt/embed_test.go internal/buildkit rt/web/router.go rt/web/router_test.go cmd/alder/build_test.go cmd/alder/run_test.go book/C-design-system.md <regenerated goldens>
git commit -m "feat(al): vendor alder-ui v0.4.1 with Geist fonts (#21)"
```

### Task 3: Retire the old tokens and harden the injected UI — Opus

**Files:**
- Modify: `rt/web/devpanel_ui.go:20-35` (`panelUICSS`), `cmd/alder/devui.go:135-149` (`errorsCSS`), `rt/web/router.go:1117-1120` (`pendingStyle`), `examples/blog/static/app.css:61`, `examples/todos/static/app.css:42`
- Modify (regenerate): `rt/web/testdata/production_page.golden`, `rt/web/boost_test.go:~364` (expected head)
- Test: `rt/web/devpanel_ui_test.go` (or the existing `rt/web` test file for the panel), `cmd/alder/devui_test.go`, `cmd/alder/example_test.go`
- Create: `internal/csscheck/csscheck.go`, `internal/csscheck/csscheck_test.go`, `cmd/alder/testdata/browser/injected.cjs`; Modify: `cmd/alder/devui_browser_test.go`

**Interfaces:**
- Consumes: v0.4.1 tokens from Task 2 (`--primary`, `--destructive`, `--destructive-foreground`, `--foreground`, `--background`, `--border`, `--muted`, `--muted-foreground`, `--accent`, `--accent-foreground`, `--card`, `--ring`, `--radius-sm`, `--radius-md`, `--font-mono`).
- Produces: new package `internal/csscheck` with `func VarsWithoutLiteralFallback(css string) []string` (each `var(--x)` or `var(--x, <non-literal>)` occurrence, in order) and `func RemovedALTokens(css string) []string` (each `--al-[a-z0-9-]+` match not starting `--al-width-`). Both are used by the tests in Tasks 3 and 5.

- [ ] **Step 1: Write the failing string tests**

- `rt/web`: `TestInjectedCSSTokens` over `panelUICSS` and `pendingStyle`: `csscheck.VarsWithoutLiteralFallback(css)` and `csscheck.RemovedALTokens(css)` are both empty; `pendingStyle` contains `var(--primary,currentColor)`; `panelUICSS` contains `var(--radius-sm,4px)` and `var(--radius-md,6px)`.
- `cmd/alder`: `TestErrorsCSSTokens` over `errorsCSS`: same two rules; contains `var(--font-mono,ui-monospace,SFMono-Regular,Menlo,monospace)` twice.
- `cmd/alder`: `TestExampleCSSTokens`: for each `examples/*/static/*.css`, no `--al-` other than `--al-width-`; blog's line uses `var(--font-mono)`, todos' uses `var(--destructive)`.
- `internal/csscheck`, `TestVarsWithoutLiteralFallback`: `var(--a,#fff)` ok; `var(--a)`, `var(--a,inherit)`, `var(--a,var(--b))` reported; `var(--a,var(--b,#000))` reported (the outer fallback is not literal).

- [ ] **Step 2: Run them to see them fail**

Run: `go test ./internal/csscheck ./rt/web -run 'TestVars|TestInjectedCSS' && go test ./cmd/alder -run 'TestErrorsCSSTokens|TestExampleCSSTokens' -short`
Expected: FAIL naming `--al-radius-sm`, `--al-font-mono`, `--al-primary`, `var(--muted-foreground,inherit)`, `var(--accent,var(--card))` and the two example files.

- [ ] **Step 3: Replace the tokens and fix the fallbacks**

Per spec §9 and the Plan 1 table: `--al-radius-sm|md` → `--radius-sm|md` (fallbacks 4px/6px kept), `--al-font-mono` → `--font-mono`, `--al-primary` → `--primary`, examples `--al-font-mono` → `--font-mono`, `--al-destructive-text` → `--destructive`. Replace non-literal fallbacks with the literal they stand for in light: `inherit` → `currentColor` where it is a colour, and `var(--accent,var(--card))` → `var(--accent,rgba(127,127,127,.2))` (matching the hover rule).

- [ ] **Step 4: Harden against Preflight**

v0.4.1's `base.css` is global inside `@layer al`; the injected CSS is unlayered, so it wins where it declares. For each injected rule that draws a box or text, declare what it relied on from browser defaults and Preflight now resets: `box-sizing` on every element given padding+size, `border` style where it draws one (`border-left:3px solid` already full; check `._alder-strip button` and `[data-min]`), `line-height` with every `font` shorthand (already), `list-style`/`margin`/`padding` on any `ul`/`p`/`pre` the strip or sheet emits (read `errorsHTML` for the emitted tags), and `display:inline-block` on SVGs if any appear (Preflight makes `svg` block). Change nothing that is already explicit.

- [ ] **Step 5: Run the string tests and regenerate the goldens**

Run: `go test ./rt/web -update -run TestProductionPage` (or the golden's own test name/flag), review that the golden diff is only `--al-primary` → `--primary`; update the expected head in `boost_test.go` the same way. Then `go test ./internal/csscheck ./rt/web ./cmd/alder -short -run 'TestVars|TestInjectedCSS|TestErrorsCSS|TestExampleCSS|TestErrorsHTML|Boost|Production'`.
Expected: PASS.

- [ ] **Step 6: Write the browser test**

`TestBrowser_InjectedUIUnderBaseLayer` in `devui_browser_test.go` (pattern of `TestBrowser_ErrorsInPanel`: `playwrightEnv`, `browserScript(t, "injected.cjs")`, `runBrowserScript(t, script, startRun(t))`). `injected.cjs BASE APPDIR` runs twice, light and with `document.documentElement.classList.add('dark')`, and checks (computed styles):
- breaking `app/Todo.alder` as `errors.cjs` does: `._alder-strip` `font-family` starts with `"Geist Mono"`; `._alder-strip button` `border-top-style` == `solid`, `border-top-width` == `1px`, `background-color` == `rgba(0, 0, 0, 0)`; the strip's `background-color` equals the page's computed `--destructive` (resolve via a probe element with `background: var(--destructive)`); in dark the sheet (`#_alder_errors[data-show] ._alder-errs`, shown by clicking the strip button with the panel minimised) has a background different from light's;
- the minimised panel tab (`al-devpanel[data-min]`): `border-top-style` == `solid`, `border-radius` == the probe's `var(--radius-md)`;
- the pending bar: with `html[data-alder-pending]` set, `::before` `background-color` equals the probe's `var(--primary)` and `height` == `2px`;
- the page's `document.fonts.check('12px "Geist Mono"')` is true (fonts served).
It writes screenshots of each state to `$ALDER_SHOTS_DIR` when set (for the Task 6 visual pass), and restores `Todo.alder`.

- [ ] **Step 7: Run it**

Run: `go test -tags browser ./cmd/alder -run TestBrowser_InjectedUIUnderBaseLayer -v` (under a 10-minute timeout).
Expected: PASS, every check `ok`. If Playwright is missing, the test skips: report that, do not claim a pass.

- [ ] **Step 8: Commit**

```bash
git add internal/csscheck rt/web cmd/alder/devui.go cmd/alder/devui_test.go cmd/alder/example_test.go cmd/alder/devui_browser_test.go cmd/alder/testdata/browser/injected.cjs examples/blog/static/app.css examples/todos/static/app.css
git commit -m "fix(web): shadcn tokens and literal fallbacks in the injected UI (#21)"
```

### Task 4: Theme script in the scaffold, the admin shell and the examples — Sonnet

**Files:**
- Modify: `internal/scaffold/files/app/Layout.alder`, `internal/generate/admin.go:569-570`, `examples/{blog,todos,workspace}/app/Layout.alder`
- Modify (regenerate): `cmd/alder/testdata/generate/want/**` (only where they contain the admin shell), `testdata/golden/ir_todos.ir`, `testdata/golden/m11/{todos,blog}.ir`
- Test: `cmd/alder/new_test.go:345`, `internal/scaffold` (or `cmd/alder`) for `TestScaffoldLayoutThemeScript`, `cmd/alder/generate_test.go` for `TestGenerateNoNestedPage`, `cmd/alder/admin_test.go`

**Interfaces:**
- Consumes: the theme script line (Global Constraints).
- Produces: `const ThemeScript = `<script>try{…}catch(e){}</script>`` in `internal/scaffold` (exported, the single Go copy of the line); `admin.go` and the tests use it.

- [ ] **Step 1: Write the failing tests**

- `new_test.go:345`: the served home page contains `scaffold.ThemeScript`, and its index is less than the index of `<link rel="stylesheet" href="/static/al/al.css" />`.
- `TestScaffoldLayoutThemeScript`: read `internal/scaffold/files/app/Layout.alder`, format it with the formatter `alder fmt` uses (`internal/format`), assert the output equals the input (idempotent) and still contains `ThemeScript` byte-for-byte; same for the admin fallback Layout text `admin.go` writes for an app without a Layout (take it from the `admin_test.go` fixture that exercises the `else` branch).
- `admin_test.go`: the no-Layout admin output contains `ThemeScript` before the `al.css` link.
- `TestGenerateNoNestedPage`: walk `cmd/alder/testdata/generate/want/**/files/app/**/*.alder` excluding files named `Layout.alder` and the admin shell file; none contains `<al-page`.
- Each `examples/*/app/Layout.alder` contains `ThemeScript` (one table-driven test in `example_test.go`).

- [ ] **Step 2: Run them to see them fail**

Run: `go test ./internal/scaffold ./cmd/alder -short -run 'ThemeScript|NoNestedPage|TestNew|Admin|Example'`
Expected: FAIL (missing script; `TestGenerateNoNestedPage` may already pass — that is fine, it is a guard).

- [ ] **Step 3: Emit the script**

Add `ThemeScript` on its own line in each Layout's `<head>` directly after `<meta charset="utf-8" />` (before the `al.css` link), and in `admin.go`'s `else` branch the same place (D5: nothing else added there).

- [ ] **Step 4: Regenerate goldens and run**

Run: `go test ./cmd/alder -run TestGenerate -update` and regenerate the IR goldens with their package's `-update` flag; review each diff is only the script line (IR goldens: the script element). Then the Step 2 command plus `go test ./internal/ir/... ./cmd/alder -short`.
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/scaffold internal/generate/admin.go examples/*/app/Layout.alder cmd/alder testdata/golden
git commit -m "feat: alder new, admin shell and examples emit the theme script (#21)"
```

### Task 5: Appendix C rewrite and the migration guide — Sonnet

**Files:**
- Modify: `book/C-design-system.md` (lines 80–205 "Tokens and themes", plus new sections; "From another stack" ~1084; "Where it lives" ~1099), any other `book/*.md` naming `--al-` tokens other than `--al-width-*` or saying v0.3 is shipped (`grep -rn -e '--al-' -e 'v0\.3' book`)
- Test: `book/appendix_test.go` (existing), plus `TestAppendixC_NoRemovedTokens` there

**Interfaces:**
- Consumes: the theme script line, the token table from Plan 1's interfaces, D1, D4.

- [ ] **Step 1: Write the failing test**

`TestAppendixC_NoRemovedTokens`: every `csscheck.RemovedALTokens` match in `book/*.md` appears inside the "Removed tokens" table (find the table by its heading `### Removed tokens`). Also asserts the appendix contains the theme script line byte-exact and the string `:root, .light`.

- [ ] **Step 2: Run it to see it fail**

Run: `go test ./book -run TestAppendixC`
Expected: FAIL listing `--al-destructive-text`, `--al-space-…` etc. at lines 99–107.

- [ ] **Step 3: Rewrite**

Content, per spec §9 and §10 (no new decisions):
- **Tokens and themes:** the oklch sheet (shadcn names; `:root` and `.dark`), `--radius` 0.625rem and `--radius-sm|md|lg|xl`, `--shadow-xs|sm|md|lg`, `--font-sans`/`--font-mono` (Geist, bundled, served at `/static/al/fonts/`, OFL), the surviving `--al-width-*`. **Light and dark:** `.dark` on `<html>` only, set by the theme script (quote the line; where it goes; light without JS); a pasted theme must write its light block as `:root, .light { … }` or a `.light` subtree inside a dark page loses it (alder#21 item 4). **No white flash:** rewritten per D4. **AA:** shadcn's border/ring values fail 3:1; the strict override `--input`/`--border` (light `oklch(0.65 0 0)`, dark `oklch(1 0 0 / 35%)`, ruling R11).
- **New section "The base layer and typography":** Preflight is global, Typography inside `al-page:not([plain])`; `div` is a flow container, so a component author zeroes their children's margins (item 3); **D1**: in Alder the Layout owns the one `al-page`, so `plain` goes on the Layout's `al-page`, and pages never add their own.
- Element sections: `al-close` fires about 150ms after `close()` (exit motion) under `al-dialog` (item 2); a bare `<al-toast>` with no `variant` shows no icon although the manifest default is `info` (item 5).
- **New section "Migrating from v0.3.x"** with spec §10's four steps verbatim (adjusted to v0.4.1), and `### Removed tokens`: the Plan 1 replacement table.
- "From another stack": link `al.css` and keep `fonts/` beside it. "Where it lives": `rt/al/` now holds `fonts/` and the licences; vendored version `v0.4.1`.

- [ ] **Step 4: Run the book tests**

Run: `go test ./book`
Expected: PASS (including `TestAppendixC_ManifestDrift` and the reference-prompt size cap if any book test has one; if the cap trips, report the size rather than raising the cap).

- [ ] **Step 5: Commit**

```bash
git add book
git commit -m "docs(book): Appendix C for alder-ui v0.4.1, base layer, migration guide (#21)"
```

### Task 6: Examples visual pass, full suite, PR — Sonnet (controller reviews the shots)

**Files:**
- Modify (only if the pass finds them): `examples/*/static/app.css` rules that fought the old look
- No new tests; this task's gate is the full suite plus screenshots.

- [ ] **Step 1: Grep gate**

Run: `grep -rn -e '--al-' --include='*.go' --include='*.alder' --include='*.css' --include='*.md' --include='*.golden' --include='*.cjs' . | grep -v -e '^./rt/al/' -e '^./docs/superpowers/' -e '/build/' -e '--al-width-' -e '### Removed tokens' `
Expected: only lines inside Appendix C's removed-tokens table and the migration guide.

- [ ] **Step 2: Full suite**

Run: `scripts/test.sh` (foreground, 15-minute timeout), then `go test -tags browser ./cmd/alder -run TestBrowser` with `ALDER_SHOTS_DIR` set to the session scratchpad.
Expected: all green; browser checks all `ok`.

- [ ] **Step 3: Look at the examples**

For each of `examples/{blog,todos,workspace}`: `go run ./cmd/alder build` then `run` (port per example), screenshot the main pages light and dark with Playwright (dark via `colorScheme: 'dark'`, which the theme script follows), including the dev panel open, the error strip (break a file as `errors.cjs` does) and the pending bar. Delete or adjust `examples/*/static/app.css` rules that now fight the base layer (margins/borders/fonts the base layer provides); re-run `go test ./cmd/alder -run Example -short` after any change.
Expected: screenshots in the scratchpad; the controller looks at them before the PR.

- [ ] **Step 4: Commit, push, open the PR**

Delete the untracked scratch `.superpowers/brainstorm/2026-10-24-alder-ui-shadcn/` (spec §9: its content is in the spec).

```bash
git add examples
git commit -m "style(examples): drop CSS that fought the old look (#21)"   # only if Step 3 changed files
git push
gh pr create --base main --head alder-ui-shadcn-21 --title "alder-ui v0.4.1: fonts, theme script, Appendix C (#21 Plan 2)" --body "<summary, D1–D6, screenshots attached, 'Refs #21' (not Closes: the user's verdict on nightvote-replay closes it)>"
```

- [ ] **Step 5: After the user merges**

Run: `git switch main && git pull && go install ./cmd/alder && ~/go/bin/alder version`
Expected: the merge commit hash in the version. Tell the user to `:LspRestart`. Record the merge commit hash for Task 7.

---

## Part C: nightvote-replay design-system migration

### Task 7: Migrate the look, pin the new Alder, "after" screenshots — Sonnet

Repository: `~/projects/nightvote-replay`, branch `alder-ui-041`.

**Files:**
- Modify: `app/Layout.alder` (theme script in `Document`'s `<head>`), `static/app.css`, `Dockerfile:8` (`ALDER_REF`)

**Interfaces:**
- Consumes: `scripts/browser/shots.mjs` and `.shots/before-*.png` (Task 1); the Alder merge commit (Task 6 Step 5); the theme script line.

- [ ] **Step 1: Re-count the design-system surface**

Run: `grep -rhoE '<al-[a-z]+' app | sort | uniq -c; grep -rn -e '--al-' -e 'data-theme' -e 'prefers-color-scheme' app static`
Expected: matches the Task 1 survey (al-card 6, al-button 7, al-input 12, al-select 3, al-table 3, al-alert 6, al-page 1, al-toasts 1; no tokens or theme rules). Note differences in the commit message; the migration guide's step 2 has nothing to replace.

- [ ] **Step 2: Add the theme script**

In `app/Layout.alder`, `Document`'s `<head>`: the theme script directly after `<meta charset="utf-8" />`, before `{props.head}` and the `al.css` link. Run `~/go/bin/alder fmt app/Layout.alder` and confirm the script line is unchanged.

- [ ] **Step 3: Review `static/app.css` against the base layer**

Build and run as in Task 1 Step 7, take `after` shots (`node shots.mjs … ../../.shots after`). Compare with `before`. Delete rules that fought the old look or duplicate the base layer and typography (`li { margin }` and `#entries { padding-left }` against Typography's lists, `form:not(._alder-click) { display:grid; gap }` against the flow rhythm) one at a time, re-shooting after each; keep a rule only when removing it makes the page worse. The two literal state colours (`#b45309`, `#15803d`) have no shadcn token: keep them, but give the dark theme readable values under `.dark tr.stalled td.state` / `.dark tr.final td.state` if the dark shot shows them under 4.5:1 on the table background.
Expected: final `.shots/after-{index,replay}-{light,dark}.png`.

- [ ] **Step 4: Pin the Alder that carries v0.4.1**

`Dockerfile:8`: `ARG ALDER_REF=<Task 6 merge commit, full hash>`. It must be on `origin/main` of tapegram/alder (`git -C ~/projects/alder branch -r --contains <hash>` lists `origin/main`).

- [ ] **Step 5: Full gate**

Run: `scripts/test.sh`
Expected: check clean, 253 tests, build, adapter tests, mutation ≥ 0.9. `cmp` the user's two untracked files against the Task 1 safety copies.

- [ ] **Step 6: Commit and push the branch**

```bash
git add app/Layout.alder static/app.css Dockerfile
git commit -m "style: alder-ui v0.4.1 (shadcn look, theme script); pin alder <short hash>"
git push
```

- [ ] **Step 7: Hand the verdict to the user**

Send the user the four before/after pairs (`SendUserFile`). Ask: (a) is it "shadcn out of the box" (spec §10 step 4; their yes closes #21), and (b) may `alder-ui-041` be merged into `main` and pushed, which redeploys Render. After a yes to (b): merge (fast-forward), push, wait for the deploy, then run `scripts/fetch-night.sh` (needs `REPLAY_KEY`) against the deployed site and expect it to pass. After a yes to (a): close tapegram/alder#21 with a comment linking the PR, D1–D6 and the screenshots.
