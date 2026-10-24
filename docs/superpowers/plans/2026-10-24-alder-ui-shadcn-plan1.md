# alder-ui looks like shadcn (Plan 1 of 2: the alder-ui repository) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restyle alder-ui's 22 elements so a page with no app CSS matches shadcn/ui (new-york-v4, Tailwind v4) in light and dark, prove it with a CI-gated computed-style diff against a committed shadcn reference, and tag `v0.4.0-rc.1` and then `v0.4.0`.

**Architecture:** First a dev-only shadcn reference app (`ref/`, committed `ref/dist/`) and a twin page of alder-ui elements (`demo/parity.html`), compared property by property by `test/parity.py` (Playwright for Python). Components not yet restyled sit on a `pending` list that is reported but does not fail. Then tokens, base layer and fonts land (phase 1, rc tag), and the controls, surfaces and overlays are restyled one phase at a time. Each phase takes its components off `pending`. `al.css` stays hand-written CSS bundled by `scripts/build.py`.

**Tech Stack:** Hand-written CSS in `@layer al`. Python 3.12 stdlib for build and check. Playwright for Python and Pillow as dev-only test dependencies. Node, Vite, React, Tailwind v4 and shadcn only inside `ref/`.

**Spec:** `~/projects/alder/docs/superpowers/specs/2026-10-24-alder-ui-shadcn-design.md` with its companion `2026-10-24-alder-ui-shadcn-reference.md` (the token sheet, Tailwind values and recipes). Executors read both. "Spec §N" below means the design file, and "Ref §N" means the reference file.

**Where the work happens:** clone `tapegram/alder-ui` to `~/projects/alder-ui` (`gh repo clone tapegram/alder-ui ~/projects/alder-ui`) and work on branch `shadcn-21`. Each phase ends with a PR `shadcn-21` → `main` with its triptychs attached. After the user merges it, rebase `shadcn-21` on `main` and continue. Plan 2 (the Alder repository, spec §9–10) is out of scope here. Do not touch `~/projects/alder` or `~/projects/nightvote-replay`.

**Model hints:** each task header names its model. Opus is for the oracle and harness, the tokens and base layer, and the overlays and motion with the shim. Everything else uses Sonnet.

## Global Constraints

- The element API only grows. Never rename or drop an element name, attribute, union value, default or slot in `custom-elements.json`. Additions go through `test/check.py`'s `ADDED`, `ADDED_VALUES` and `ADDED_SLOTS` (Task 3).
- No compiler-contract change: style exactly the markup Alder emits (spec §6). The only classes are `help`, `errors` and `_alder-click`, plus `al-`-prefixed ones.
- Everything shipped sits inside `@layer al`, including the reset, typography, `@font-face` and icons. App CSS that is unlayered always wins.
- No Node outside `ref/`. `scripts/build.py` and `test/check.py` stay stdlib-only. `requirements-dev.txt` holds only `playwright` and `Pillow`, pinned exactly.
- Token sheet: the shadcn `:root` and `.dark` blocks are verbatim from Ref §1, with `--radius: 0.625rem`. Selectors are `.dark` and `.light` only. There is no `[data-theme]` and no `prefers-color-scheme` block.
- Of alder-ui's own tokens, only `--al-width-narrow` (40rem), `--al-width-normal` (56rem) and `--al-width-wide` (80rem) survive. Private per-component properties use the existing `--_al-` prefix.
- `components/`, `base.css`, `typography.css`: no literal colour (hex, colour functions, named colours) and no literal `font-family`. Lengths and literal font-size, weight and line-height are allowed. Every `var(--x)` must be defined.
- Text pairings need 4.5:1 (a hard failure). Border and ring pairings need 3:1 (a printed warning only).
- Each `components/*.js` is at most 60 lines. The first tier works with no JavaScript. Without the theme script, a page is light.
- Motion is `@starting-style` plus `transition-behavior: allow-discrete`, 150ms (dialog 200ms), `cubic-bezier(.4, 0, .2, 1)`, all under `@media (prefers-reduced-motion: no-preference)`.
- Translation convention (spec §5.2): above each rule, put a comment with the shadcn class string it implements. `bg-x/NN` becomes `color-mix(in oklab, var(--x) NN%, transparent)`, and so on per spec §5.2.
- Version strings: the manifest `version` is `v0.4.0-rc.1` from Task 7 to Task 18, and `v0.4.0` in Task 19. Tags equal the manifest version.

## Review Focus

1. **Colour serialisation.** Chromium serialises `oklch()`, `color-mix()` and Tailwind's emitted colours differently, so a string compare gives false diffs or false passes. `test/parity.py` must normalise every colour (including those inside `box-shadow` and `outline`) to 8-bit RGBA before comparing, with ±1 per channel. The test lives in Task 2.
2. **A pasted theme must win.** A tweakcn/ui.shadcn.com export (unlayered `:root{…}` + `.dark{…}`) must re-theme in light and dark, and inside a `.light` subtree of a `.dark` page. The test lives in Task 4 (`test/test_theme.py`).
3. **Hidden empties stay hidden under Preflight.** An empty `ul.errors` must take zero height and add no gap. `al-alert:has(> ul.errors:empty)` must stay hidden. The test lives in Task 9 (twin-page `data-expect`).
4. **Overlays must close completely with motion on.** After `close()` or `hidePopover()`, with the exit transition run, the overlay is `display: none` and does not take clicks. This holds with the shim path (invoker commands removed) and on `:target` dialogs. The test lives in Tasks 14 and 15.
5. **Fonts must really load.** `font-family` matching proves nothing if Geist fell back. Both pages must report `document.fonts.check('500 14px Geist')` and `check('14px "Geist Mono"')` as true before diffing. The test lives in Task 2.

## Interfaces Plan 2 consumes (the Alder side)

- **Files at the tag:** `al.css`, `al.js`, `custom-elements.json` (as today), plus `fonts/Geist-Variable.woff2`, `fonts/GeistMono-Variable.woff2` and `fonts/OFL.txt`. Upstream names them `Geist[wght].woff2` and `GeistMono[wght].woff2` (vercel/geist-font `fonts/Geist/webfonts/`, `fonts/GeistMono/webfonts/`). They are renamed because `[` `]` are glob metacharacters in `//go:embed` and unsafe in URL paths.
- **Font URLs:** `al.css` uses `url("fonts/Geist-Variable.woff2")` relative to itself. Alder must therefore serve and write them at `/static/al/fonts/Geist-Variable.woff2` and `/static/al/fonts/GeistMono-Variable.woff2`. `vendor-al.sh` copies to `rt/al/fonts/`, `al.Names` gains `fonts/…`, and `buildkit` creates the subdirectory. `Content-Type: font/woff2`.
- **Font family names:** `"Geist"` and `"Geist Mono"`, weight range `100 900`, `font-display: swap`.
- **Theme script line** (README, demos, and what Alder's Layout and generators emit, in `<head>` before the stylesheet link):
  `<script>try{document.documentElement.classList.toggle("dark",localStorage.theme==="dark"||(!("theme" in localStorage)&&matchMedia("(prefers-color-scheme: dark)").matches))}catch(e){}</script>`
- **Manifest additions** (all additive):
  - `al-button` `variant` gains `"link"`, and `size` gains `"xs"`, `"icon"`, `"icon-sm"` and `"icon-lg"` (appended in that order).
  - `al-card` slots gain `description` and `action`.
  - `al-page` gains `plain` (boolean).
  - `al-dialog` gains the slot `description`.
  - `al-tabs` gains `variant` (`"default" | "line"`, default `"default"`).
- **Token replacements** for each removed `--al-*`:

| removed | replacement |
|---|---|
| `--al-radius-sm` / `--al-radius-full` | `--radius-sm` (6px at default `--radius`) / literal `9999px` |
| `--radius` (was 0.5rem) | `--radius` (now 0.625rem); also `--radius-md`, `--radius-lg`, `--radius-xl` |
| `--al-font-sans` / `--al-font-mono` | `--font-sans` / `--font-mono` |
| `--al-destructive-text` | `--destructive` |
| `--al-shadow-sm` | `--shadow-sm` (also new: `--shadow-xs`, `--shadow-md`, `--shadow-lg`) |
| `--al-space-0-5 … -10` | literal `0.125rem 0.25rem 0.375rem 0.5rem 0.75rem 1rem 1.5rem 2rem 2.5rem` |
| `--al-text-xs/sm/base/lg` | literal `0.75rem / 0.875rem / 1rem / 1.125rem` |
| `--al-leading` / `--al-leading-tight` | literal `1.5` / `1.25` |
| `--al-weight-normal/medium/semibold` | literal `400 / 500 / 600` |
| `--al-control-h-sm/md/lg` | literal `2rem / 2.25rem / 2.5rem` (shadcn h-8/h-9/h-10) |
| `--al-checkbox-size`, `--al-border-width` | literal `1rem`, `1px` |
| `--al-ring-width`, `--al-ring-offset` | none: the focus ring is `0 0 0 3px color-mix(in oklab, var(--ring) 50%, transparent)` |
| `--al-disabled-opacity`, `--al-duration` | literal `0.5`, `150ms` |
| `--al-{info,success,warning,destructive}-soft[-foreground]`, `--al-*-border` | no replacement (alerts use `--card`, `--card-foreground`, `--border`, `--destructive`) |
| `--al-width-narrow/normal/wide` | **kept** |

  New non-prefixed tokens beyond the shadcn sheet: `--destructive-foreground`, `--color-white: #fff` and `--color-black: #000` (Tailwind's names, used for shadcn's `text-white` and `bg-black/50`), plus `--radius-sm/md/lg/xl`, `--shadow-xs/sm/md/lg`, `--font-sans` and `--font-mono`.

---

## Phase 0: the oracle (PR 1; nothing visual changes)

### Task 1: shadcn reference app and committed build — **Opus**

**Files:**
- Create: `ref/` (Vite + React + TS app: `package.json`, `package-lock.json`, `vite.config.ts`, `components.json`, `index.html`, `src/main.tsx`, `src/App.tsx`, `src/index.css`, `src/components/ui/*.tsx` from `npx shadcn@latest add`), `ref/README.md`, `ref/dist/` (committed build output)
- Create: `fonts/Geist-Variable.woff2`, `fonts/GeistMono-Variable.woff2`, `fonts/OFL.txt` (from the vercel/geist-font release `v1.8.2`, files `fonts/Geist/webfonts/Geist[wght].woff2`, `fonts/GeistMono/webfonts/GeistMono[wght].woff2`, `OFL.txt`)
- Modify: `.gitignore` (add `ref/node_modules/`, `ref/out/`)

**Interfaces:**
- Produces: `ref/dist/index.html`, served at `/ref/dist/index.html` from the repo root. A `#dark` hash puts `.dark` on `<html>` (for humans; the harness toggles the class itself). Every compared element carries `data-ref="<component>/<variant>/<state-or-size>"`. Optional `data-ref-states="hover focus"` lists the states to drive. Overlays carry `data-ref-open` and are rendered open (Radix `open` / `defaultOpen`). Family names are `"Geist"` and `"Geist Mono"`, from `../fonts/*.woff2` via `@font-face` in `src/index.css`.
- **Marker inventory** (Task 2's twin uses the same set). `data-ref` goes on the box that draws the look:
  - `button/{default,destructive,outline,secondary,ghost,link}/default` (states hover, focus)
  - `button/default/{xs,sm,lg,icon,icon-sm,icon-lg}`
  - `button/default/disabled`
  - `button/default/with-icon`
  - `label/default/default`
  - `input/default/{default,disabled,invalid}` (focus on default)
  - `input/description/default`
  - `input/error/default` (the `text-destructive font-medium` message)
  - `textarea/default/{default,invalid}`
  - `select/trigger/{default,disabled}`
  - `checkbox/default/{unchecked,checked,disabled,invalid}`
  - `card/{root,title,description,action,content,footer}/default`
  - `badge/{default,secondary,destructive,outline}/default`
  - `alert/{default,destructive}/{root,title,description,icon}`
  - `table/{wrapper,table,head,row,cell,caption}/default` and `table/row/hover`
  - `pagination/{previous,next,link,active}/default`
  - `dialog/{content,title,description,close,footer}/open` and `dialog/overlay/open`
  - `popover/content/open`
  - `menu/{content,item,item-destructive,separator}/open`
  - `tooltip/{content,arrow}/open`
  - `toast/{default,success,info,warning,error}/default`
  - `tabs/{list,trigger,trigger-active,content}/default` and `tabs/{list,trigger-active}/line`
  - `typography/{h1,h2,h3,h4,p,ul,li,code,a,blockquote,hr,table,th,td}/default`
- Fixed text throughout (`Button`, `Email`, `Card Title`, …) so widths compare.

- [ ] **Step 1:** Scaffold with `npm create vite@latest ref -- --template react-ts`, then add Tailwind v4 via `@tailwindcss/vite` and run `npx shadcn@latest init` (style new-york, base colour neutral, CSS variables on). Pin exact versions in `package.json` and commit `package-lock.json`. Set `base: "./"` in `vite.config.ts` so `dist/` works under `/ref/dist/`. Make sure `src/index.css` carries the canonical sheet from Ref §1 verbatim. If shadcn's init writes the docs-site variant, replace it with the canonical one.
- [ ] **Step 2:** Add the components: `npx shadcn@latest add button input textarea label select checkbox card badge alert table pagination dialog popover dropdown-menu tooltip sonner tabs separator`. If any recipe class string differs from Ref §3, amend the reference file with a dated note (spec §12) rather than editing the generated `.tsx`. Re-fetch `tabs.tsx` and record the exact `line` classes in Ref §3.
- [ ] **Step 3:** Copy the Geist files into `fonts/` with the names above. In `src/index.css`, add `@font-face` for both (`font-weight: 100 900; font-display: swap`) and set `@theme { --font-sans: "Geist", ui-sans-serif, system-ui, sans-serif, "Apple Color Emoji", "Segoe UI Emoji", "Segoe UI Symbol", "Noto Color Emoji"; --font-mono: "Geist Mono", ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace; }`. Task 4 uses these exact strings.
- [ ] **Step 4:** Write `src/App.tsx` as one fixed grid (one section per component, each in a fixed-width cell) with the marker inventory. Typography is raw `h1…td` with the Ref §3 typography classes. The toasts are a static `<ol>` rendered with sonner's real DOM, from `toast()` calls at mount, with `duration: Infinity`.
- [ ] **Step 5:** Run `cd ref && npm run build`. Then run `python3 -m http.server 8000` from the repo root and open `http://localhost:8000/ref/dist/` and `#dark`. Expected: every section renders, with Geist in DevTools' Rendered Fonts.
- [ ] **Step 6:** Write `ref/README.md`: what this is (the dev-only oracle) and how to regenerate it (`npm ci`, `npx shadcn@latest add … --overwrite`, `npm run build`, commit `dist/`, re-run `test/parity.py`). Also state that Node is never needed to build or test alder-ui. Commit:

```bash
git add ref fonts .gitignore && git commit -m "ref: shadcn new-york-v4 reference grid (dev-only), committed build; Geist fonts (OFL)"
```

### Task 2: twin page, parity harness, allow-list, CI job — **Opus**

**Files:**
- Create: `demo/parity.html`, `test/parity.py`, `test/parity-allow.json`, `test/test_parity.py`, `requirements-dev.txt`
- Modify: `.github/workflows/ci.yml` (new job `parity`), `README.md` (Tests section)

**Interfaces:**
- Consumes: Task 1's marker inventory and `ref/dist/index.html`.
- Produces:
  - `python3 test/parity.py [--shots] [--strict] [--only <component>]`. It exits 1 on any unallowed diff in a non-pending component, and with `--strict` also when `pending` is non-empty.
  - Pure functions for the unit tests: `normalise_colour(s: str) -> tuple[int,int,int,int]`, `normalise_value(prop: str, s: str) -> str` (every colour inside it normalised), `diff(ref: dict, twin: dict, tol_px=0.5) -> list[tuple[prop, ref_val, twin_val]]`, `allowed(marker: str, prop: str, allow: dict) -> bool`, `target(marker: str, allow: dict) -> str` (applies `map`).
  - `test/parity-allow.json` schema:
    `{"pending": ["<component>", …], "map": {"alert/info/root": "alert/default/root", …}, "allow": [{"ref": "<fnmatch glob>", "prop": "<property>", "reason": "<one line>"}]}`
  - `data-expect="prop:value; prop:value"` on twin-only elements (no `data-ref`) is asserted against the twin's computed style. It is used for things shadcn lacks: empty errors, devpanel, `al-page plain`.
  - The property list compared is exactly spec §8.3's, plus the bounding-box size.

- [ ] **Step 1: Write the failing unit tests** in `test/test_parity.py` (stdlib `unittest`, no browser):
  - `test_colour_forms_normalise_equal`: `normalise_colour("rgb(23, 23, 23)") == normalise_colour("oklch(0.205 0 0)")` within ±1 per channel. `normalise_colour("color-mix(in oklab, oklch(0.205 0 0) 90%, transparent)")[3] == 230`.
  - `test_shadow_colours_normalised`: `normalise_value("box-shadow", "rgba(0, 0, 0, 0.05) 0px 1px 2px 0px") == normalise_value("box-shadow", "oklab(0 0 0 / 0.05) 0px 1px 2px 0px")`.
  - `test_size_tolerance`: `diff({"height": "36px"}, {"height": "36.4px"}) == []`, and `diff({"height": "36px"}, {"height": "37px"}) == [("height", "36px", "37px")]`.
  - `test_allow_and_map`: an `allow` entry `{"ref": "checkbox/*", "prop": "width"}` allows `checkbox/default/checked` `width` and not `height`. `target("alert/info/root", allow) == "alert/default/root"`.
  - `test_allow_entries_have_reasons`: loading `test/parity-allow.json` fails on an entry without `reason`.
- [ ] **Step 2:** Run `python3 -m unittest test.test_parity -v`. Expected: FAIL (`ModuleNotFoundError: parity`).
- [ ] **Step 3: Implement `test/parity.py`.**
  - Serving: a `http.server.ThreadingHTTPServer` on `127.0.0.1:0` in a thread serves the repo root. Load `/ref/dist/index.html` and `/demo/parity.html` in Chromium at 1280×2000.
  - Before diffing: inject `*, *::before, *::after { transition: none !important; animation: none !important }` into both pages. Await `document.fonts.ready`, then assert `document.fonts.check('500 14px Geist')` and `document.fonts.check('14px "Geist Mono"')` on both pages (Review Focus 5).
  - Colour normalisation: in pure Python, implement oklch, oklab, rgb, hex, `color(srgb …)` and `color-mix(in oklab|srgb, …)` (shared with Task 3's engine: import `test/colour.py` once Task 3 lands; until then define the functions in `parity.py` and move them in Task 3).
  - For each theme (toggle `.dark` on `<html>` in both pages) and each marker on both pages:
    - read the property list and the size at rest;
    - for each `data-ref-states` entry, drive it identically on both pages: hover via `locator.hover()`, then move the mouse to (0,0) after; focus via `page.keyboard.press("Shift")` then `locator.focus()`, asserting `el.matches(":focus-visible")` on both;
    - for `data-ref-open` overlays on the twin, open via `showModal()` / `showPopover()` one at a time and close after reading.
  - Fail if the two pages' marker sets or state lists differ.
  - Report: a Markdown table per component (marker, theme, state, prop, ref, twin). Components in `pending` are reported under "pending" and do not fail.
  - `--shots`: per component and theme, write `ref/out/<component>-<theme>.png` as ref | twin | Pillow `ImageChops.difference` diff, thresholded at 16/255 to ignore anti-aliasing. These are never a gate.
- [ ] **Step 4: Write `demo/parity.html`** with the same grid, text and markers, using the markup Alder emits (spec §6) and today's attributes only.
  - `data-ref` goes on the box that draws the look: the inner `<button>`, `<input>`, `<dialog>`, …
  - Follow every `test/check.py` demo rule: link `../al.css`, no `style=` attribute, no non-`al-` class, and only declared attributes. A `<style>` block for the grid's cell widths is not allowed. Lay the grid out with `al-stack` and `al-page width="wide"`, and give the ref app the same cell widths.
  - Load `../al.js`.
- [ ] **Step 5: Write `test/parity-allow.json`.**
  - `pending` holds every component: `button label input textarea select checkbox card badge alert table pagination dialog popover menu tooltip toast tabs typography`.
  - `map` holds the alert/toast variants shadcn lacks.
  - `allow` holds spec §8.3's standing deviations: the native `<select>` listbox; the radio-label tabs with no JS; `checkbox/*` `display` (a native input versus a button). Each has a one-line reason.
  - Structural entries (spec §5.3 4a) may be added later, but never for colour, spacing, radius, type or shadow props.
- [ ] **Step 6:** Write `requirements-dev.txt` with `playwright==<current>` and `Pillow==<current>`, pinned exactly.
- [ ] **Step 7:** Run `python3 -m unittest test.test_parity -v`. Expected: PASS. Then run `pip install -r requirements-dev.txt && python3 -m playwright install chromium && python3 test/parity.py`. Expected: exit 0, with every component under "pending" and a non-empty diff table printed. `python3 test/check.py` still prints `ok:` because `parity.html` obeys the demo rules.
- [ ] **Step 8: CI.** Add a job `parity` to `.github/workflows/ci.yml`:
  - `actions/setup-python@v5` (3.12);
  - `pip install -r requirements-dev.txt`;
  - `python3 -m playwright install --with-deps chromium`;
  - `python3 -m unittest discover -s test -p 'test_*.py'`;
  - `python3 test/parity.py`;
  - `python3 test/parity.py --shots` with `if: always()`;
  - `actions/upload-artifact@v4` of `ref/out/` named `triptychs`.

  Document the commands in README "Tests". Commit, push `shadcn-21` and open PR 1. Ask the user to merge.

## Phase 1: tokens and base (PR 2; tag `v0.4.0-rc.1`)

### Task 3: check.py engine: oklch, oklab mixes, additive unions and slots, rc versions — **Sonnet**

**Files:**
- Create: `test/colour.py`, `test/test_check.py`
- Modify: `test/check.py` (`colour()` → `test/colour.py`; `check_manifest`; version regex), `test/parity.py` (import from `colour.py`)

**Interfaces:**
- Produces:
  - `colour.parse(expr: str, env: dict[str,str]) -> tuple[float,float,float,float]` (sRGB 0..1 + alpha). It understands `oklch(L C H [/ A%])`, `oklab(…)`, `hsl()`, hex, `rgb()`, `transparent`, `var(--x)` and `color-mix(in oklab|srgb, A [p%], B [q%])`, with premultiplied alpha as in CSS Color 5.
  - `colour.over(top, bottom)`.
  - `colour.contrast(fg, bg, env) -> float`, composited over `var(--background)` as today's `ratio`.
  - `check.ADDED_VALUES: dict[tuple[str,str], list[str]]` and `check.ADDED_SLOTS: dict[str, list[str]]`, both empty at this task.
- [ ] **Step 1: Write the failing tests** in `test/test_check.py` (`unittest`):
  - `test_oklch_white_black`: `parse("oklch(1 0 0)", {})[:3]` ≈ `(1,1,1)` and `parse("oklch(0 0 0)", {})[:3]` ≈ `(0,0,0)` to 1e-3.
  - `test_oklch_red600`: `parse("oklch(0.577 0.245 27.325)", {})` rounds to sRGB `(231, 0, 11)` ±1 (Tailwind red-600 `#e7000b`).
  - `test_oklch_alpha`: `parse("oklch(1 0 0 / 10%)", {})[3] == 0.1`.
  - `test_mix_transparent`: `parse("color-mix(in oklab, var(--p) 90%, transparent)", {"--p": "oklch(0.205 0 0)"})[3] == 0.9`.
  - `test_contrast_known`: contrast of `oklch(0.556 0 0)` on `oklch(1 0 0)` is 4.73 ±0.02.
  - `test_union_growth_allowed`: `check_manifest` given a manifest whose `al-button` `variant` text is the API's plus `| "link"`, with `ADDED_VALUES[("al-button","variant")] == ["link"]`, records no failure. The same manifest without the `ADDED_VALUES` entry records one.
  - `test_slot_growth_allowed`: the same, for `al-card` slots + `["description", "action"]` via `ADDED_SLOTS`.
  - `test_rc_version`: versions `v0.4.0-rc.1` and `v0.4.0` pass, and `v0.4` fails.
- [ ] **Step 2:** Run `python3 -m unittest test.test_check -v`. Expected: FAIL (no module `colour`).
- [ ] **Step 3: Implement `test/colour.py`** using the OKLab→linear-sRGB matrices from CSS Color 4, then refactor `check.py` to use it. The `check_manifest` changes:
  - the expected union text is the API text with `| "v"` appended for each `ADDED_VALUES` entry;
  - the expected slots are the API slots + `ADDED_SLOTS`;
  - the version regex is `v\d+\.\d+\.\d+(-rc\.\d+)?`.
- [ ] **Step 4:** Run `python3 -m unittest discover -s test -p 'test_*.py' && python3 test/check.py`. Expected: both pass. The contrast table is unchanged (hsl still works).
- [ ] **Step 5:** Commit: `test: colour engine handles oklch and oklab mixes; additive unions/slots; rc versions`.

### Task 4: tokens.css rewrite, old tokens out of every component, check.py token rules — **Opus**

**Files:**
- Modify: `tokens.css` (rewrite), every `components/*.css` (token sweep), `test/check.py` (`load_tokens`, `check_tokens`, `PAIRS`, `check_tokens_only`, `button_pairs`, contrast warnings), `al.css` (rebuilt)
- Create: `test/test_theme.py` (Playwright)

**Interfaces:**
- Consumes: `colour.parse` and `colour.contrast` (Task 3).
- Produces: the token blocks, which later tasks and Plan 2 rely on.
  - `:root, .light { color-scheme: light; <Ref §1 :root verbatim>; --destructive-foreground: oklch(0.97 0.01 17) }`
  - `.dark { color-scheme: dark; <Ref §1 .dark verbatim>; --destructive-foreground: oklch(0.58 0.22 27) }`, written after the light block (equal specificity; order decides on `<html class="dark">`)
  - `:root { --radius-sm/md/lg/xl (Ref §1 calc forms); --shadow-xs/sm/md/lg (Ref §2 values); --font-sans/--font-mono (Task 1 step 3 strings); --color-white: #fff; --color-black: #000; --al-width-narrow: 40rem; --al-width-normal: 56rem; --al-width-wide: 80rem }`
- `check.py`'s `load_tokens()` returns `light = blocks[":root, .light"]`, `dark = blocks[".dark"]` and `common = blocks[":root"]`.
- [ ] **Step 1: Write the failing tests.** In `test/check.py`'s `check_tokens`, add:
  - every Ref §1 name (including `chart-1..5` and the 8 `sidebar*`) in both themes;
  - `--radius == 0.625rem`;
  - no `[data-theme` and no `prefers-color-scheme` anywhere in `tokens.css`;
  - no `--al-` name defined other than `--al-width-{narrow,normal,wide}`;
  - no `var(--al-…)` other than those three in `components/`, `base.css` or `typography.css`.

  Write `test/test_theme.py` (unittest + Playwright, skipped if Playwright is not importable). It serves the repo root and loads `demo/button.html`.
  - `test_pasted_theme_wins`: an injected unlayered `<style>:root{--primary:oklch(0.6 0.2 260)} .dark{--primary:oklch(0.7 0.2 140)}</style>` changes the computed `background-color` of `al-button[variant=primary] > button` in light, and to the other value with `.dark` on `<html>`.
  - `test_light_subtree_in_dark`: an `<al-card class="light">` (inserted by the test, not in the demo file) inside a `.dark` page computes `background-color` equal to `oklch(1 0 0)` (normalised), and `color-scheme: light`.
- [ ] **Step 2:** Run `python3 test/check.py` and `python3 -m unittest test.test_theme -v`. Expected: FAIL (old tokens, `[data-theme]` blocks).
- [ ] **Step 3: Rewrite `tokens.css`** as specified above. Rewrite the header comment: how to re-theme (paste the shadcn/tweakcn `:root` + `.dark` blocks unlayered), the `.dark`/`.light` switches, the AA note. Move `@view-transition` and the old base block out (Task 5 owns `base.css`; until then keep `@view-transition { navigation: auto }` at the end of `tokens.css`).
- [ ] **Step 4: Sweep `components/*.css`** off every removed token, using the replacement table in "Interfaces Plan 2 consumes". This is a mechanical substitution and no restyle. Two exceptions: in `al-alert.css` and `al-toast.css`, the soft colours become `var(--card)`, `var(--card-foreground)` and `var(--border)`, and the destructive text becomes `var(--destructive)`. `focus-visible` rings become the 3px `--ring/50` box-shadow. Literal `white` and `black` become `var(--color-white)` and `var(--color-black)`.
- [ ] **Step 5: Update `check.py` rules.**
  - `check_tokens_only`: drop the length rule and the font-size/weight/line-height rule. Keep the colour rules. `font-family` must be `var(…)` or `inherit`. Also scan `base.css`, `typography.css` and `components/icons.css` when they exist. The `defined` set includes custom properties declared in those files.
  - `PAIRS`: replace the `--al-*` rows with what components now draw:
    - `ul.errors`: `destructive` on `background`;
    - `al-alert destructive`: `destructive` on `card`;
    - `al-badge destructive` and `al-button destructive`: `--color-white` on `destructive`;
    - `al-toast`: `popover-foreground` on `popover`.
  - Mark the three non-text rows (`input`, `ring`, the invalid border) as warnings: print `WARN …` and do not fail.
  - `button_pairs` reads the new `al-button.css` shape. Task 8 rewrites it, so for now just keep it passing.
- [ ] **Step 6:** Run `python3 scripts/build.py && python3 test/check.py --table && python3 -m unittest discover -s test -p 'test_*.py'`. Expected: `ok:`, with WARN lines for border/ring only. Every text pair is ≥4.5. If a verbatim shadcn text pairing a component draws is below 4.5 (light `muted-foreground` on `muted` is 4.34), do not change the token. Stop and report it to the controller (a spec gap).
- [ ] **Step 7:** Commit: `feat!: shadcn oklch token sheet, .dark/.light, radius/shadow/font scale; old --al-* tokens removed`.

### Task 5: base layer, typography, fonts in the bundle — **Opus**

**Files:**
- Create: `base.css`, `typography.css`
- Modify: `scripts/build.py` (bundle order), `test/check.py` (`check_css` counts selectors in `typography.css` toward `seen`; tokens-only scans the new files), `components/al-page.css` (`[plain]` hook, `py-8 px-4`, `px-6` from 40rem), `custom-elements.json` (`al-page` `plain`), `test/check.py` `ADDED["al-page"] = [("plain","boolean")]`, `demo/page.html` (shows `plain`), `demo/parity.html` (typography section, `data-expect` rows), `test/parity-allow.json` (remove `typography` from `pending`)

**Interfaces:**
- Consumes: Task 4's tokens. `fonts/*.woff2` from Task 1.
- Produces:
  - `build.py` bundle order: `tokens.css`, `base.css`, `typography.css`, `components/icons.css` (if present), then manifest CSS.
  - `base.css` holds `@font-face` for `"Geist"` and `"Geist Mono"`, with `src: url("fonts/Geist-Variable.woff2") format("woff2")` and `url("fonts/GeistMono-Variable.woff2") format("woff2")`, `font-weight: 100 900` and `font-display: swap`. It then holds spec §7's Preflight translation and `@view-transition`.
  - `typography.css` holds selectors under `al-page:not([plain])`.
- [ ] **Step 1: Write the failing tests.** Add typography markers to `demo/parity.html` (Task 1 inventory `typography/*`). Then add twin-only `data-expect` rows:
  - a `<p>` in `al-page plain` has `margin-top:0px`;
  - `body` has `font-family` starting `Geist`;
  - an `<hr>` has `height:1px`.

  Add `plain` to the manifest and to `ADDED`. Remove `typography` from `pending`.
- [ ] **Step 2:** Run `python3 test/parity.py --only typography && python3 test/check.py`. Expected: FAIL (`typography` diffs; `al-page.css never styles [plain]`).
- [ ] **Step 3:** Write `base.css` per spec §7, using the "Base layer" paragraph rule for rule. Write `typography.css` per Ref §3 "typography": `hr` as Separator (`height: 1px; background: var(--border)`), and a bare `table` gets the shadcn **table** recipe (Ref §3 "table"), not the Typography page's bordered one. Update `build.py`. In `al-page.css`, the `[plain]` rule documents that `typography.css` is scoped off. Add `al-page` padding.
- [ ] **Step 4:** Run `python3 scripts/build.py && python3 test/check.py && python3 test/parity.py`. Expected: `ok:`. Typography passes with no unallowed diff, and the other components are still pending. The font assertion passes on the twin.
- [ ] **Step 5:** Commit: `feat: Preflight base layer, shadcn Typography for flow content, Geist @font-face; al-page plain`.

### Task 6: icons as CSS masks — **Sonnet**

**Files:**
- Create: `components/icons.css`, `LICENSE-lucide` (repo root, lucide ISC licence text)
- Modify: `test/check.py` (`check_css`: `icons` is a known non-element stem), `al.css`

**Interfaces:**
- Produces: `:root` custom properties `--_al-icon-check`, `--_al-icon-minus`, `--_al-icon-chevron-down`, `--_al-icon-chevron-left`, `--_al-icon-chevron-right`, `--_al-icon-x`, `--_al-icon-circle-check`, `--_al-icon-info`, `--_al-icon-triangle-alert`, `--_al-icon-octagon-x`, `--_al-icon-loader-circle` and `--_al-icon-ellipsis`. Each is `url("data:image/svg+xml,…")` (quoted, URL-encoded lucide 24×24 SVG, stroke-width 2). Components use them as `mask: var(--_al-icon-x) center / contain no-repeat; background: currentColor` on a `::before`/`::after`.
- [ ] **Step 1: Write the failing test:** a `check.py` rule that every `--_al-icon-*` used in `components/` is defined in `icons.css`, and that each of the 12 names exists. Run `python3 test/check.py`. Expected: FAIL (12 missing).
- [ ] **Step 2:** Write `icons.css` from lucide's SVG sources at a pinned lucide release (name the release in the header comment). Add `LICENSE-lucide`.
- [ ] **Step 3:** Run `python3 scripts/build.py && python3 test/check.py`. Expected: `ok:`.
- [ ] **Step 4:** Commit: `feat: lucide icons as mask-image custom properties`.

### Task 7: theme script in demos and README; rc.1 — **Sonnet**

**Files:**
- Modify: all 20 `demo/*.html` (replace the `toggleTheme` script), `demo/parity.html`, `README.md` (Use it, design rules, tokens, theme script, AA override), `custom-elements.json` (`version` `v0.4.0-rc.1`), `al.css`, `al.js`

**Interfaces:**
- Produces: the theme script line (exact text in "Interfaces Plan 2 consumes"), placed in every demo's `<head>` before the stylesheet link. Demo toggle: `function toggleTheme(){var d=document.documentElement.classList.toggle("dark");try{localStorage.theme=d?"dark":"light"}catch(e){}}`.
- [ ] **Step 1:** Add a `check.py` demo rule: every `demo/*.html` contains the exact theme script line before `../al.css`, and none contains `data-theme`. Run it. Expected: FAIL on 21 pages.
- [ ] **Step 2:** Update the demos.
- [ ] **Step 3:** Update the README. Add the theme line. Remove the `prefers-color-scheme`/`[data-theme]` instructions. Re-theming is "paste the `:root` + `.dark` blocks". Add the strict-AA override `:root{--input:oklch(0.6 0 0);--border:oklch(0.708 0 0)} .dark{--input:oklch(1 0 0 / 35%);--border:oklch(1 0 0 / 25%)}` (verify each reaches 3:1 with `check.py`'s engine before writing it). Add the removed-token table.
- [ ] **Step 4:** Set the manifest version to `v0.4.0-rc.1`. Then run `python3 scripts/build.py && python3 test/check.py && python3 -m unittest discover -s test -p 'test_*.py' && python3 test/parity.py --shots`. Expected: all pass. Triptychs are in `ref/out/`.
- [ ] **Step 5:** Commit, push and open PR 2 with the triptychs. After the user merges, tag `main`: `git tag v0.4.0-rc.1 && git push origin v0.4.0-rc.1` (the spec §5.3 4b early look; the user approved tagging in the spec). Report the tag to the controller: Plan 2 may vendor it on Alder's branch only.

## Phase 2: controls (PR 3)

Every component task below follows the same cycle:
1. Remove the component(s) from `pending` in `test/parity-allow.json`.
2. Add any API additions to `custom-elements.json` and to `ADDED`/`ADDED_VALUES`/`ADDED_SLOTS`. Add the new values and slots to the element's demo page (`check_demos` requires every value shown and every slot filled) and to `demo/parity.html`.
3. Run `python3 test/parity.py --only <c>`. Expected: FAIL, with diffs listed.
4. Rewrite `components/al-<c>.css` from Ref §3, with the class string as a comment above each rule. Add allow-list entries only for structural gaps, each with a reason.
5. Run `python3 scripts/build.py && python3 test/check.py && python3 test/parity.py --only <c>` in light and dark. Expected: `ok:` and no unallowed diff.
6. Commit with `feat(<c>): shadcn recipe`.

The steps listed per task are what differs.

### Task 8: al-button — **Sonnet**

**Files:** `components/al-button.css`, `custom-elements.json`, `test/check.py` (`ADDED_VALUES`, `button_pairs`), `demo/button.html`, `demo/parity.html`, `test/parity-allow.json`

**Interfaces:**
- Mapping to shadcn: `variant` `primary` → `default`, other variants by name, plus the added `link`. `size` `md` → `default`, `sm` → `sm`, `lg` → `lg`, plus the added `xs`, `icon`, `icon-sm` and `icon-lg` (Ref §3 button).
- `ADDED_VALUES[("al-button","variant")] = ["link"]`, `ADDED_VALUES[("al-button","size")] = ["xs","icon","icon-sm","icon-lg"]`.
- `button_pairs()` now yields, per variant, `--_al-fg` on `--_al-bg` and on `--_al-hover`. Hovers are `color-mix(in oklab, var(--x) 90%, transparent)` etc., composited over `--background` by `colour.contrast`.
- [ ] **Steps:** the shared cycle.
  - Keep the three forms (`> button`, `> a`, presentational), the element-level `[disabled]` dimming and `form._alder-click { display: contents }`.
  - Border only on `outline`. `:has(> svg)` padding per size. `al-button svg { width: 1rem; height: 1rem; flex: none; pointer-events: none }` (`xs`: 0.75rem).
  - Destructive text is `var(--color-white)`, with dark `bg-destructive/60`.
  - `button_pairs` must report the destructive hover pair (4.52 in light) and pass.

### Task 9: al-input, al-textarea, label, help, errors — **Sonnet**

**Files:** `components/al-input.css`, `components/al-textarea.css`, `demo/input.html`, `demo/textarea.html`, `demo/parity.html`, `test/parity-allow.json`

**Interfaces:**
- Field = `al-input > label` laid out as `display: grid; gap: 0.5rem` holding the label text and control.
- Label text uses Ref §3 label.
- `.help` = description (`text-sm text-muted-foreground`).
- `ul.errors` = `text-sm text-destructive font-medium`.
- `[invalid]` = the `aria-invalid` recipe, and `[aria-invalid=true]` on the control does the same.
- textarea `min-height: 4rem; field-sizing: content`.
- [ ] **Steps:** the shared cycle. Remove `label input textarea` from `pending`. Allow-list structural `display`/`gap` entries if the label wraps the control (spec §5.3 4a). Add the Review Focus 3 rows to `demo/parity.html`:
  - `<al-input>` with an empty `<ul class="errors"></ul>` and `data-expect="height:0px"` on the `ul`;
  - the field's total height equal to a field without the list, via `data-expect` on the field;
  - `<al-alert>` holding only an empty `ul.errors`, with `data-expect="display:none"`.

### Task 10: al-select, al-checkbox — **Sonnet**

**Files:** `components/al-select.css`, `components/al-checkbox.css`, `demo/select.html`, `demo/checkbox.html`, `demo/parity.html`, `test/parity-allow.json`

**Interfaces:**
- Consumes: `--_al-icon-chevron-down`, `--_al-icon-check` and `--_al-icon-minus` (Task 6).
- Select: `appearance: none`, `padding-right: 2rem`, masked chevron at `right: 0.75rem`, `opacity: .5`, `color-scheme` inherited for the native listbox.
- Checkbox: `appearance: none; width/height 1rem; border-radius 4px; border 1px var(--input); box-shadow var(--shadow-xs)`. `:checked` uses `background/border var(--primary)` with a masked check (0.875rem) in `--primary-foreground`. `:indeterminate` uses a minus. The label row has `gap: 0.5rem`, box first (today's `row-reverse` stays).
- [ ] **Steps:** the shared cycle. Remove `select checkbox` from `pending`. Standing allow-list entries stay (listbox; checkbox `display`). Then push and open PR 3 with triptychs.

## Phase 3: surfaces (PR 4)

### Task 11: al-card (+ slots), al-badge — **Sonnet**

**Files:** `components/al-card.css`, `components/al-badge.css`, `custom-elements.json` (card slots), `test/check.py` (`ADDED_SLOTS["al-card"] = ["description","action"]`), `demo/card.html`, `demo/badge.html`, `demo/parity.html`, `test/parity-allow.json`

**Interfaces:**
- Card: `flex column; gap 1.5rem; padding-block 1.5rem; border 1px var(--border); radius var(--radius-xl); shadow var(--shadow-sm)`.
- The header region is a grid `1fr auto` when `[slot=action]` is present: `header` = CardTitle, `description` = CardDescription and `action` = CardAction.
- Default children get `padding-inline: 1.5rem` (CardContent). `footer` = CardFooter. `actions` = CardFooter + `justify-content: flex-end; gap: 0.5rem`.
- An unslotted leading `h2`/`h3` is CardTitle and a `p` right after it is CardDescription. Use `:nth-child(1 of :not([slot]))` so a slotted header before it does not break the rule. Document the choice in a comment (spec §12).
- Badge: Ref §3 badge, with `al-badge > a` hover per `[a&]:hover`.
- [ ] **Steps:** the shared cycle. Remove `card badge` from `pending`. Add twin markers for the unslotted-heading card (compared against `card/title` and `card/description` via `map`).

### Task 12: al-alert, al-table, al-pagination — **Sonnet**

**Files:** `components/al-alert.css`, `components/al-table.css`, `components/al-pagination.css`, `demo/alert.html`, `demo/table.html`, `demo/pagination.html`, `demo/parity.html`, `test/parity-allow.json`

**Interfaces:**
- Consumes: the icon properties `info`, `circle-check`, `triangle-alert`, `octagon-x`, `x`, `chevron-left` and `chevron-right`.
- Alert (spec §5.2):
  - `info`, `success` and `warning` render as `default` (card surface, border), with the icon from a masked `::before` in the grid's icon column. `destructive` = `text-destructive`, description `destructive/90`.
  - `heading` → AlertTitle. The dismiss button is a ghost icon `size-7` at top-right (masked x).
  - `map` entries send `alert/{info,success,warning}/*` → `alert/default/*`.
- Table: the shadcn recipe on `al-table > table`, with the outer frame removed. `striped` → `tbody tr:nth-child(even) { background: color-mix(in oklab, var(--muted) 50%, transparent) }`. `dense` → `th` height 2rem and cell padding 0.25rem. The wrapper is `overflow-x: auto`.
- Pagination: `prev`/`next` are ghost buttons `gap: 0.25rem; padding-inline: 0.625rem` with masked chevrons. "Page n of m" and unslotted text get `text-sm text-muted-foreground`. Unslotted `a` children are ghost `2.25rem` squares, and `[aria-current=page]` is outline.
- [ ] **Steps:** the shared cycle. Remove `alert table pagination` from `pending`. `al-alert.js` stays ≤60 lines.

### Task 13: al-stack, al-page — **Sonnet**

**Files:** `components/al-stack.css`, `components/al-page.css`, `demo/stack.html`, `demo/page.html`, `demo/parity.html`

**Interfaces:**
- Stack gaps: `sm` 0.5rem, `md` 1rem, `lg` 1.5rem.
- Page: `padding: 2rem 1rem`, and `padding-inline: 1.5rem` at `@media (min-width: 40rem)`. Widths use `--al-width-*`.
- [ ] **Steps:** shadcn has no counterpart, so the twin uses `data-expect` rows: each gap value, both page paddings at 1280px, `al-page[plain]` `p` margin 0. Run `python3 test/parity.py` with no `--only`. Expected: only phase-4 components remain pending. Push and open PR 4 with triptychs.

## Phase 4: overlays and motion (PR 5)

Motion rules for Tasks 14–16 and 17's tab content:
- Enter: `@starting-style` from `opacity: 0; scale: .95` (popover, menu and tooltip also `translate` 0.5rem from the anchored side) to rest.
- Exit: mirror it, with `transition: opacity, scale, translate, display allow-discrete, overlay allow-discrete` over 150ms (dialog 200ms), inside `@media (prefers-reduced-motion: no-preference)`.
- `al-menu` and `al-tooltip` are `display: none` today. Their open state must be `:popover-open` (or the shim's/`al-tooltip.js` state), so `display` animates discretely. Read `components/al-shim.js` and `components/al-tooltip.js` and keep their state hooks unchanged.

### Task 14: al-dialog (+ description slot) — **Opus**

**Files:** `components/al-dialog.css`, `custom-elements.json` (`al-dialog` slot `description`), `test/check.py` (`ADDED_SLOTS["al-dialog"] = ["description"]`), `demo/dialog.html`, `demo/parity.html`, `test/parity.py` (close-state assertion), `test/test_parity.py`, `test/parity-allow.json`

**Interfaces:**
- `<dialog>`: `background: var(--background); padding: 1.5rem; display: grid; gap: 1rem; radius var(--radius-lg); border; shadow var(--shadow-lg); width: calc(100% - 2rem); max-width: 32rem`. `size` `sm`/`md`/`lg` = `max-width` `28rem`/`32rem`/`42rem`.
- `::backdrop` is `color-mix(in oklab, var(--color-black) 50%, transparent)`, fading.
- `header` = Title (`1.125rem/1`, 600), `description` = Description (`text-sm text-muted-foreground`), `actions` = Footer (`flex-direction: column-reverse; gap: .5rem`, row + `justify-content: flex-end` from 40rem).
- `form[method=dialog] > button` = Close (masked x, `top: 1rem; right: 1rem; opacity: .7`, 1 on hover).
- The `:target` no-JS form still shows, with no motion required.
- [ ] **Step 1: Write the failing tests.**
  - In `test/parity.py`, add `check_closes(page, selector, close_js)`: open, close, wait 300ms, then assert `getComputedStyle(el).display == "none"` and `document.elementFromPoint(center)` is not inside `el` (Review Focus 4). Run it for the dialog via `close()`, via the close button, and with `HTMLButtonElement.prototype.commandForElement` deleted before load (the shim path).
  - Unit-test the helper's decision logic in `test/test_parity.py` with stubbed values.
  - Remove `dialog` from `pending`.
- [ ] **Step 2:** Run `python3 test/parity.py --only dialog`. Expected: FAIL.
- [ ] **Step 3:** Rewrite `al-dialog.css`. Keep `check_dialog_shape` passing.
- [ ] **Step 4:** Run `python3 scripts/build.py && python3 test/check.py && python3 test/parity.py --only dialog`. Expected: pass, including the three close paths in light and dark.
- [ ] **Step 5:** Commit: `feat(dialog): shadcn dialog, description slot, fade+zoom 200ms`.

### Task 15: al-popover, al-menu, al-tooltip — **Opus**

**Files:** `components/al-popover.css`, `components/al-menu.css`, `components/al-tooltip.css` (and `components/al-tooltip.js` / `al-menu.js` only if a state hook must change; ≤60 lines), `demo/popover.html`, `demo/menu.html`, `demo/tooltip.html`, `demo/parity.html`, `test/parity-allow.json`

**Interfaces:**
- Popover: `width: 18rem; padding: 1rem; radius var(--radius-md); border; shadow var(--shadow-md)`, on the popover surface. `offset` `sm`/`md`/`lg` = 4/8/16px.
- Menu: `min-width: 8rem; padding: .25rem`. `a`/`button` items get `gap .5rem; radius var(--radius-sm); padding .375rem .5rem; text-sm`. `:hover`/`:focus-visible` = `bg-accent text-accent-foreground`. `al-menu > hr` = Separator (`margin .25rem -.25rem; height 1px; background var(--border)`). `[data-variant=destructive]` items are `text-destructive`, with focus `destructive/10` (dark `/20`).
- Tooltip: `padding .375rem .75rem; radius var(--radius-md); text-xs; background var(--foreground); color var(--background); text-wrap: balance`. The arrow is an `::after` 10px square, rotated 45°, radius 2px, on the anchored side.
- [ ] **Steps:**
  - Write the failing tests: remove `popover menu tooltip` from `pending`. Add `check_closes` runs for popover (`hidePopover()`, light dismiss, shim path), menu (select an item) and tooltip (blur/mouseleave).
  - Run them and expect FAIL.
  - Restyle the three files.
  - Run `python3 scripts/build.py && python3 test/check.py && python3 test/parity.py --only popover && python3 test/parity.py --only menu && python3 test/parity.py --only tooltip`. Expected: pass.
  - Commit.

### Task 16: al-toast, al-toasts — **Opus**

**Files:** `components/al-toast.css` (and `al-toast.js` only if needed; ≤60 lines), `demo/toast.html`, `demo/parity.html`, `test/parity-allow.json`

**Interfaces:**
- Consumes: the icons `info`, `circle-check`, `triangle-alert` and `octagon-x`.
- Toast: `width: 356px; padding: 16px; gap: 6px; radius var(--radius); border 1px var(--border); background var(--popover); color var(--popover-foreground); box-shadow: 0 4px 12px color-mix(in oklab, var(--color-black) 10%, transparent)`. Title 13px/1.5/500 and description `opacity .8`.
- The icon is 1rem, per variant (`destructive` → octagon-x). `map`: `toast/{info,success,warning,destructive}` → `toast/{info,success,warning,error}`.
- Region: offsets 32px and gap 14px per `position`. The enter transition slides in from the region's edge.
- [ ] **Steps:** the shared cycle, plus a `check_closes` run for timeout close (with `timeout="100"`). Remove `toast` from `pending`.

### Task 17: al-tabs (+ variant), al-tab, al-panel — **Sonnet**

**Files:** `components/al-tabs.css`, `custom-elements.json` (`al-tabs` attribute `variant`), `test/check.py` (`ADDED["al-tabs"] = [("variant", '"default" | "line"')]`), `demo/tabs.html`, `demo/parity.html`, `test/parity-allow.json`

**Interfaces:**
- Both the no-JS radio labels and the `al-tabs.js` tablist (`[role=tablist] > button[role=tab][aria-selected=true]`) get the recipe.
- List: `background var(--muted); radius var(--radius-lg); padding 3px; height 2.25rem`.
- Trigger: `radius var(--radius-md); padding .25rem .5rem; text-sm 500; color color-mix(in oklab, var(--foreground) 60%, transparent)`.
- Active: `background var(--background); color var(--foreground); shadow var(--shadow-sm)`, and in dark `background input/30; border-color var(--input)`.
- `line`: list `background: transparent; gap: .25rem`, and the active trigger gets an `::after` underline `height 2px; background var(--foreground)` using the exact classes Task 1 recorded from `tabs.tsx`.
- Panel `flex: 1`.
- [ ] **Steps:** the shared cycle. Remove `tabs` from `pending`. Keep the allow-list entry for the no-JS radio labels and keep `check_tabs_nojs` passing.

### Task 18: al-devpanel re-pin — **Sonnet**

**Files:** `components/al-devpanel.css`, `demo/devpanel.html`, `demo/parity.html`

**Interfaces:**
- The new tokens are already swept in (Task 4). Mono is `var(--font-mono)`.
- State explicitly what the panel relies on under Preflight: `box-sizing`, `border`, `margin`, `list-style`, `line-height`, and button `background`/`font`/`color`. Otherwise unchanged.
- [ ] **Steps:**
  - Add twin `data-expect` rows: panel font-family starts `"Geist Mono"`, nav buttons have a non-zero padding, and section lists have `list-style-type: none`.
  - Run `python3 test/parity.py`. Expected: `pending` empty and no unallowed diff, light and dark.
  - Commit. Push and open PR 5 with triptychs.

## Phase 5 (first sentence only): v0.4.0

### Task 19: release v0.4.0 — **Sonnet**

**Files:** `custom-elements.json` (`version` `v0.4.0`), `README.md` (element table with the additions; "The interactive tier (v0.4.0)"; migration section), `al.css`, `al.js`, `.github/workflows/ci.yml` (`parity` job uses `--strict`)

- [ ] **Step 1:** Switch CI to `python3 test/parity.py --strict`, and run it locally. Expected: exit 0 (empty `pending`).
- [ ] **Step 2:** Make the README edits:
  - the element table gains `link`, `xs`, `icon*`, card `description`/`action`, dialog `description`, `al-page plain` and `al-tabs variant`;
  - "Tests" lists `check.py`, unittest and `parity.py`;
  - a "Migrating from v0.3.x" section carries spec §10's guide verbatim, with the token table from this plan.
- [ ] **Step 3:** Set the manifest version to `v0.4.0`. Run `python3 scripts/build.py && python3 test/check.py && python3 -m unittest discover -s test -p 'test_*.py' && python3 test/parity.py --strict --shots`. Expected: all pass. The header of `al.css` reads `alder-ui v0.4.0`.
- [ ] **Step 4:** Commit (`release: v0.4.0 (breaking: tokens removed, dark mode by .dark + theme script, shadcn look)`), then push and open the PR. After the user merges, tag `v0.4.0` on `main`, push the tag, and create a GitHub release: `gh release create v0.4.0 --notes-file <notes>`. The notes say it is breaking, then give the migration steps, the theme line and the font files. Hand back to the controller for Plan 2.
