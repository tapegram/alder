# Route attributes `input=` / `load=` / `intersect=` / `every=` — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Search as you type, infinite scroll, a slow widget and a polling badge are writable in Alder with `input=`, `intersect=`, `load=` and `load= every=`, checked by `alder check`, driven by `Test.input/intersect/load` in `alder test`, and taught in book ch. 6 (#160 PR 10).

**Architecture:** Each attribute is an element fetch of a GET route (`@get(url, window._alderF?.())`, no `busy` wrapper) on a different Datastar trigger. The runtime grows render helpers and one browser helper (`window._alderQ`); the checker grows four attribute cases and one new code; IR/gen lower them to `html.RouteAttr`/`html.RouteInput`; the test browser grows one step `TriggerRoute` that three stdlib steps share.

**Tech Stack:** Go (`internal/` compiler, `rt/` runtime), vendored Datastar v1.0.0 (`rt/datastar/datastar.js`), Playwright (`-tags browser`), book Markdown checked by `go test ./book`.

**Spec:** `docs/superpowers/specs/2026-11-08-routes-attrs-design.md` (addendum to `2026-11-03-routes-design.md`). Read both before any task.

Branch/worktree: `git worktree add .worktrees/routes-attrs -b routes-attrs main`. One PR, merged to main at the end.

## Global Constraints

- Rendered attribute names, verbatim (checked against vendored Datastar's parser: plugin = text before the first `:`, mods after `__`):
  - `load={R}` → `data-init`
  - `every="…"` beside `load` → `data-on-interval__duration.<d>` (dash form: `on-interval` is its own plugin)
  - `intersect={R}` → `data-on-intersect__once` (dash form: `on-intersect` is its own plugin)
  - `input={R}` → `data-on:input__debounce.300ms` (colon form: the `on` plugin with event `input`)
- Request value for load/intersect/interval, verbatim: `@get('<url>', window._alderF?.())`. Input: `@get(window._alderQ('<url>', '<key>', el.value), window._alderF?.())`. **No `_alderBusy` wrapper on any of the four** (spec body and its last test line; the spec's two mentions of a busy wrapper in the input rendering line and the "Rendering" test bullet are slips).
- `'` in the URL is `%27`, as `routeRequest` already does.
- `window._alderQ = (u, k, v) => { const x = new URL(u, location.href); x.searchParams.set(k, v); return x.pathname + x.search; };` — one key set, others untouched, same-origin path returned.
- `_alderF` keeps setting no `requestCancellation` (Datastar's default `"auto"` aborts the element's previous request).
- `every` grammar: `^[1-9][0-9]*(ms|s|m)$`, literal Text only. **Datastar's duration parser has no `m` unit** (`parseFloat("1m")` = 1 ms), so `Nm` renders as `<N*60>s`; `ms` and `s` pass through unchanged.
- One trigger per element: at most one of `load`, `intersect`, `input`. `every` only beside `load`. `click`/`href` may coexist.
- Debounce 300 ms; no tunable debounce, no sub-second-poll guard beyond the grammar, no polling-while-visible.
- Codes: write-route targets are **E0045**. Every shape error is the new **E0047** `diag.RouteTrigger` ("route trigger attribute misused"). Both get explain entries; E0047's records the `every` grammar and units.
- The test browser fires nothing on its own. `Test.load/intersect/input` are explicit steps; no step for `every`.
- Generators emit none of these. No grammar change.
- Commits: `area: what (#160)`. Quick loop `scripts/test.sh -short`; full `scripts/test.sh` plus `go vet ./...` before the PR.
- Models (user CLAUDE.md): each task names its model; final whole-branch review on **Opus** (not security- or data-critical).
- Weekdays 09:00–17:00 America/New_York: no subagents or background runs.

## Plan calls (decisions the spec leaves to the builder)

1. **`m` maps to seconds** in lowering (`every="2m"` renders `__duration.120s`), because the vendored parser misreads `m`. The book shows the user's spelling and says minutes are converted.
2. **One code, E0047**, for all shape errors, with a message per case (exact wording in Task 2).
3. **Custom elements (`<al-*>`)** do not take the four in this slice: E0047 "`<al-x>` does not take `load`; wrap it in a `<div load=…>`". Not added to `elements.Escaped`.
4. **`input=` on `<Input>`/`<Textarea>`/`<Select>`** goes through `formControl`, a new `ir.Input.Trigger` field and `html.Control.Trigger`. A plain `<input>` inside `<form submit>` stays E0304 as today.
5. **One test-browser entry point** `TriggerRoute(kind, r, label)` for the three steps; one IR node `TestTrigger{Kind}`.
6. **Element matching ignores the query**: route method + path. Zero matches is the existing `NoControlError`; several is a new `*SeveralControlsError` naming each element by tag and id.
7. **Test.load matches** `data-init` and `data-on-interval__duration.*` on the same element once (so the interval-only fallback still matches).
8. **Book section** "## Patterns" goes after "## App-level routes" in ch. 6 (not under "File pages", where "Links and the boost" lives).
9. **Playwright tests** run the built `routes-patterns-ok` app behind an `httptest` reverse proxy in the Go test, which counts requests per path (`loads`) and delays the first `/results` answer for the ordering test.

## Review Focus

1. **`every="1m"` polling every millisecond.** The rendered duration for `m` must be seconds. Pinned in Task 3 (`TestLower_EveryMinutes`) and Task 5's polling recipe uses `every="1s"` only for speed.
2. **A typed value with `&`, `#`, `+`, spaces or non-ASCII.** `_alderQ` must encode via `searchParams.set`; the server must decode the exact text. Pinned in Task 5's search Playwright test (types `a&b #c+é`).
3. **A polled view re-running `data-init` after its own patch** (two requests per tick). Pinned by Task 5's polling test over two ticks; fallback per spec if it fails.
4. **A server URL already containing `'`** (`Results { q: "o'neil" }`): `%27` in the URL, `_alderQ` still parses it. Pinned in Task 1 (`TestRender_RouteInputQuote`).
5. **A page with two widgets loading the same route path** in a test: `Test.load` must fail naming both elements, not silently pick one. Pinned in Task 4 (`TestTest_TriggerSeveral`).

---

### Task 1: Runtime rendering and `_alderQ` (Sonnet)

**Files:**
- Modify: `rt/html/html.go` (`Attr`, `Attr.value`, new helpers), `rt/datastar/datastar.go` (`BoostInit`)
- Test: `rt/html/html_test.go`, `rt/datastar/datastar_test.go`; update size/content pins in `rt/web/access_alloc_test.go:184`, `rt/web/devpanel_test.go:406`

**Interfaces:**
- Produces:
  - `func RouteLoad(r rt.Route) string` → `@get('<url>', window._alderF?.())` (`routeRequest(r, FetchOptions+"?.()", false)`, no `busy`)
  - `func RouteInput(key string, r rt.Route) Attr` → `Attr{Name: "data-on:input__debounce.300ms", route: &r, key: key}`; new unexported field `key string` on `Attr`
  - `func RouteInputValue(r rt.Route, key string) string` → `@get(window._alderQ('<url>', '<key>', el.value), window._alderF?.())`; `key` JS-escaped (`\` → `\\`, `'` → `\'`)
  - `Attr.value()` cases: `"data-init"`, `"data-on-intersect__once"`, any name with prefix `"data-on-interval__duration."` → `RouteLoad`; `"data-on:input__debounce.300ms"` → `RouteInputValue(*a.route, a.key)`
  - `const InputAttr = "data-on:input__debounce.300ms"`, `const LoadAttr = "data-init"`, `const IntersectAttr = "data-on-intersect__once"`, `const IntervalPrefix = "data-on-interval__duration."` (used by gen and the test browser)
  - `window._alderQ` in `BoostInit`, defined right after `_alderF`, text per Global Constraints

- [ ] **Step 1: Write failing tests** in `html_test.go`, in the `TestRender_ClickGetRoute` style with `listRoute` (`/todos?filter=Active`):
  - `TestRender_RouteLoad`: `Render(El("div", []Attr{RouteAttr(LoadAttr, listRoute)}))` == `<div data-init="@get(&#39;/todos?filter=Active&#39;, window._alderF?.())"></div>`
  - `TestRender_RouteLoadEvery`: attrs `RouteAttr(LoadAttr, listRoute), RouteAttr(IntervalPrefix+"5s", listRoute)` render both with the same value.
  - `TestRender_RouteIntersect`: `data-on-intersect__once="@get(&#39;/todos?filter=Active&#39;, window._alderF?.())"`
  - `TestRender_RouteInput`: `RouteInput("filter", listRoute)` renders `data-on:input__debounce.300ms="@get(window._alderQ(&#39;/todos?filter=Active&#39;, &#39;filter&#39;, el.value), window._alderF?.())"`
  - `TestRender_RouteInputQuote`: a route with query `q=o'neil` renders `%27` in the URL; a key `a'b` renders `a\&#39;b`.
  - `TestRender_RouteTriggersNoBusy`: none of the four values contains `_alderBusy`; none of the elements gains any other attribute (no no-JS fallback).
  - `datastar_test.go` `TestBoostInit_AlderQ`: `strings.Contains(BoostInit, <the exact _alderQ line>)`; and `_alderF`'s slice still has no `requestCancellation`.
- [ ] **Step 2: Run** `go test ./rt/html ./rt/datastar` — FAIL (undefined names).
- [ ] **Step 3: Implement** the Interfaces above. A `<button>` with these attributes is not rewritten by `clickButton` (`clickOf` matches `data-on:click` only); leave that as is.
- [ ] **Step 4: Run** `go test ./rt/...` — PASS after updating the two boost-tag pins in `rt/web` to the new size/content.
- [ ] **Step 5: Commit** `rt: load/intersect/input/interval route attributes and _alderQ (#160)`

### Task 2: Checker, E0047, explain (Opus)

**Files:**
- Modify: `internal/check/html.go` (`attribute`, `element`, `formControl`), `internal/check/elements.go` (`customAttr`), `internal/diag/diag.go` (`RouteTrigger = "E0047"`, E0045 comment), `internal/diag/explain.go` (E0045 extended, E0047 new), `internal/diag/diag_test.go` if its code list is explicit
- Create corpus cases under `testdata/corpus/` (each `app/App.alder` + `app/Todos.alder` like `E0045-href-write`, plus `expected.diag`)

**Interfaces:**
- Produces (used by Task 3 and Task 4's checker):
  - `func (mc *modCtx) triggerAttrs(x *syntax.Element)` — runs once per element before attributes: reports two of `load`/`intersect`/`input`, `every` without `load`
  - `func (mc *modCtx) triggerRoute(a *syntax.HtmlAttr, env *env, eff effect) (*syntax.CtorRef, *RouteInfo)` — `bareRoute`, `routeTarget`, write → E0045, then `c.u.Check(got, types.Route)`
  - `func queryKey(r *RouteInfo, key string) (*syntax.Field, bool)` — key = `f.Wire` else `f.Name`, skipping names in `pathParams(r.Path)` and the `form` field
  - `func everyDuration(lit string) (string, bool)` in a shared place both checker and IR import (`internal/syntax` or a small `internal/attrs` file): returns the Datastar duration (`"5s"`, `"500ms"`, `"120s"` for `"2m"`) and whether the literal matches the grammar

Messages (exact; `%s` filled in order):

| case | code | message |
|---|---|---|
| `load` to write route | E0045 | `` `load` fetches a GET route; `%s` is %s: use `click` `` |
| `load` + `every` to write route | E0045 | `` `load … every` fetches a GET route; `%s` is %s: a timer must not write `` |
| `intersect` to write route | E0045 | `` `intersect` fetches a GET route; `%s` is %s: use `click` `` |
| `input` to write route | E0045 | `` `input` fetches a GET route; `%s` is %s: use `submit` `` |
| `input` not on a control | E0047 | `` `input` fires as a field changes; put it on `<input>`, `<textarea>`, `<select>` or `<Input field=…>` `` |
| control without literal `name`/`field` | E0047 | `` `input` sends the field's value under its name; give `<%s>` a literal `name` `` |
| no query key | E0047 | `` `input` sends field `%s`; `%s` has no query parameter `%s` `` |
| key is a path param | E0047 | `` `input` sends field `%s`; `%s` has `%s` in its path, and the browser can only set a query parameter `` |
| key not `Text` | E0047 | `` `input` sends field `%s` as text; `%s.%s` is %s, so make it `Text` `` |
| two triggers | E0047 | `` an element takes one of `load`, `intersect`, `input`; this one has `%s` and `%s` `` |
| `every` without `load` | E0047 | `` `every` repeats a `load`; add `load={Route}` beside it `` |
| `every` malformed or computed | E0047 | `` `every` takes a literal like "5s": a whole number above 0 then `ms`, `s` or `m` `` |
| on `<al-*>` | E0047 | `` `<%s>` does not take `%s`; wrap it in a `<div %s=…>` `` |

- [ ] **Step 1: Write failing corpus cases** (expected.diag lines with real positions once written):
  - `E0045-load-write`, `E0045-every-write`, `E0045-intersect-write`, `E0045-input-write`
  - `E0047-input-div`, `E0047-input-no-name`, `E0047-input-no-key`, `E0047-input-path-key`, `E0047-input-int-key`, `E0047-two-triggers`, `E0047-every-no-load`, `E0047-every-malformed` (three errors: `"5"`, `"0s"`, `{model.d}`), `E0047-custom-element`
  - `routes-attrs-ok` (empty expected.diag): `load`, `load every` with `"500ms"`, `"5s"`, `"1m"`; `intersect`; `input` on `<input name="q">`; `input` on `<Input field=q>` inside `<form submit>`; `input` to a route with `query: Text as "q"` (rename matches); `input` with a `Text` key that has a default; `click` beside `load` on one element
- [ ] **Step 2: Run** `go test ./internal/check -run TestCorpus` — FAIL.
- [ ] **Step 3: Implement.** New `case` arms in `attribute()` for `load`, `intersect`, `input`, `every` (so they no longer fall to `textAttr`); `input` on `<Input>`/`<Textarea>`/`<Select>` is a new case in `formControl`'s switch whose key is the `field` literal. Add `RouteTrigger` and both explain entries (E0045 names the four attributes and `Test.load/intersect/input`; E0047 lists every case above and the grammar `^[1-9][0-9]*(ms|s|m)$`, saying minutes render as seconds).
- [ ] **Step 4: Run** `go test ./internal/check ./internal/diag` — PASS (including `TestExplainCoversEveryCode`).
- [ ] **Step 5: Commit** `check: load/intersect/input/every attributes, E0047 (#160)`

### Task 3: IR and code generation (Sonnet)

**Files:**
- Modify: `internal/ir/ir.go` (`AttrKind`, `HAttr`, `Input`), `internal/ir/lower_html.go` (`attr`, element lowering, form-control lowering), `internal/ir/dump.go:707` (kinds slice), `internal/gen/html.go` (`attrs`, `input`), `rt/html/state.go` (`Control`) and the control renderer
- Test: `internal/ir` lowering test file beside existing ones; `cmd/alder/routes_test.go` (pattern at line 74)

**Interfaces:**
- Consumes: Task 1 constants and `RouteInput`; Task 2 `everyDuration`.
- Produces:
  - `AttrLoad`, `AttrIntersect`, `AttrInput` appended to `AttrKind`; `dump.go` kinds gain `"load","intersect","input"`
  - `HAttr` gains `Every string` (Datastar duration, set on the `load` attr; the `every` attribute itself is consumed and not emitted) and `Key string` (set on `input`: the control's `name` literal)
  - `ir.Input` gains `Trigger Expr` and `TriggerKey string`; `html.Control` gains `Trigger *rt.Route` and `TriggerKey string`; the control renderer adds `RouteInput(TriggerKey, *Trigger)` to the rendered control
  - gen emits: load → `html.RouteAttr(html.LoadAttr, ref)` plus, if `Every != ""`, `html.RouteAttr(html.IntervalPrefix+"<every>", ref)`; intersect → `html.RouteAttr(html.IntersectAttr, ref)`; input → `html.RouteInput("<key>", ref)`

- [ ] **Step 1: Write failing tests:**
  - `TestLower_RouteTriggers`: lowering `<div load={Stats {}} every="5s">` gives one `HAttr{Kind: AttrLoad, Every: "5s"}` and no `every` attr; `<input name="q" input={Results { q: model.q }}>` gives `HAttr{Kind: AttrInput, Key: "q"}`.
  - `TestLower_EveryMinutes`: `every="2m"` gives `Every: "120s"`; `"500ms"` stays `"500ms"`.
  - `TestRoutes_TriggerAttrsRender` in `cmd/alder/routes_test.go`: build `testdata/corpus/routes-attrs-ok`, GET `/`, and assert the body contains `data-init="@get(&#39;/stats`, `data-on-interval__duration.60s=`, `data-on-intersect__once=`, two `data-on:input__debounce.300ms=` (plain and `<Input>`), and no `every=`.
- [ ] **Step 2: Run** `go test ./internal/ir -run TestLower_ && go test ./cmd/alder -run TestRoutes_TriggerAttrsRender` — FAIL.
- [ ] **Step 3: Implement** per Interfaces. `go test ./internal/ir -run Dump` must not panic on the new kinds.
- [ ] **Step 4: Run** the same commands plus `scripts/test.sh -short` — PASS.
- [ ] **Step 5: Commit** `gen: render load/intersect/input/every (#160)`

### Task 4: Test-browser steps `Test.load/intersect/input` (Opus)

**Files:**
- Modify: `stdlib/Test.alder` (three entries beside `click`, same `target -> Task {…}` shape), `internal/check/browser.go` (`testSteps`, `stepExample`, `inferTestStep`, new `testTrigger`), `internal/ir/ir.go` + `walk.go:111` + `dump.go:622` (`TestTrigger`), `internal/ir/lower_eff.go:121`, `internal/gen/browser.go:140`, registries `internal/gen/effects.go:124,294`, `internal/gen/expr.go:309`, `internal/gen/stdcall.go:316`, `rt/testing/steps.go`, `rt/testing/browser.go`
- Test: `cmd/alder/test_test.go` (beside `TestTest_RouteSteps`), corpus `E0045-test-load-write/`

**Interfaces:**
- Consumes: Task 1 attribute constants.
- Produces:
  - `type Trigger int` in `rt/testing` with `TriggerLoad`, `TriggerIntersect`, `TriggerInput`
  - `func (b *Browser) TriggerRoute(kind Trigger, r rt.Route, label string) (Result, error)` — finds the one element, sends `rt.RouteURL(r)` through `sendRoute` with `fetch: true`, returns `b.result(ans)` (so `checkedElement` runs)
  - `type SeveralControlsError struct{ Step, Path string; Elements []string }` with `Error()` = `Test.<step> found <n> elements requesting <path>: <el>, <el>`; elements printed as `<tag id="…">` or `<tag>` without id
  - `ir.TestTrigger{Typed; Kind string; Route string; Params Expr}` with `Kind` in `"load" | "intersect" | "input"`
  - checker: `testTrigger(name string, arg syntax.Expr, env, eff)` — `unresolvedHead`, `bareRoute(arg, true)`, `routeTarget`; write route → E0045 `` `Test.%s` fetches a GET route; `%s` is %s: use `Test.click` ``; else `mc.check(arg, types.Route)` and `stepResult("answer", answerType())`; non-route → E0003 `` `Test.%s` wants a route, got … ``

Element matching (`rt/testing/steps.go`): walk `b.doc` in document order; per kind read `data-init` or any `data-on-interval__duration.*` (load, counted once per element), `data-on-intersect__once` (intersect), `data-on:input__debounce.300ms` (input). Parse the URL with `regexp.MustCompile(`@get\((?:window\._alderQ\()?'([^']*)'`)`; match on `routeMethod(r) == "GET"` and the URL's path equal to `rt.RouteURL(r)`'s path. Zero → `NoControlError` as `ClickRoute`; several → `SeveralControlsError`.

- [ ] **Step 1: Write failing tests** in `test_test.go`, using `tempAppAbs(t, "../../testdata/corpus/routes-attrs-ok", "todos")` and appending tests to the app as `TestTest_RouteSteps` does:
  - `TestTest_TriggerSteps`: `Test.load (Stats {})`, `Test.intersect (More { after: 10 })`, `Test.input (Results { q: "mil" })` each answer `Rendered` and pass the element check; the request `Test.input` sent has `q=mil` (assert via the view, which echoes `q`).
  - `TestTest_TriggerMissing`: `Test.intersect` on a page without a sentinel fails with the `NoControlError` text.
  - `TestTest_TriggerSeveral`: a page with two `load={Stats {}}` divs (ids `a`, `b`) fails with `found 2 elements requesting /stats: <div id="a">, <div id="b">`.
  - `TestTest_TriggerAnswers`: a route whose view has no root id or that is missing answers as `Test.click` of a GET route does (`PageMissing`/`RequestFailed`).
  - Corpus `E0045-test-load-write`: `Test.load (Toggle { id: "a" })` → E0045 message above.
- [ ] **Step 2: Run** `go test ./cmd/alder -run 'TestTest_Trigger' && go test ./internal/check -run TestCorpus` — FAIL.
- [ ] **Step 3: Implement** per Interfaces; `stepParams` needs no change (each arg is recorded as its route type).
- [ ] **Step 4: Run** the same plus `scripts/test.sh -short` — PASS.
- [ ] **Step 5: Commit** `test: Test.load, Test.intersect, Test.input (#160)`

### Task 5: Patterns app, Playwright, book (Opus)

**Files:**
- Create: `testdata/corpus/routes-patterns-ok/` (`app/App.alder`, `app/Patterns.alder`, empty `expected.diag`), `cmd/alder/patterns_browser_test.go` (`//go:build browser`), `cmd/alder/testdata/browser/patterns.cjs`
- Modify: `cmd/alder/test_test.go` (`TestTest_Patterns`), `book/06-routes.md` (new `## Patterns` after `## App-level routes`), `book/05-html.md` (attributes table rows for the four), `book/10-*.md` (rows for the three `Test.*` steps beside `Test.click r`); then `python3 book/site/build.py`

**Interfaces:**
- Consumes: everything above.
- The app, in-memory, no auth, routes:
  - `Index = GET "/" { q: Text = "" }` page with `<input name="q" value={q} input={Results { q: q }}>`, `<div id="slow" load={Slow {}}>…</div>`, `<span id="badge" load={Badge {}} every="1s">…</span>`, first 10 rows and sentinel `<div id="more-10" intersect={More { after: 10 }}>`
  - `Results = GET "/results" { q: Text = "" }` view root `<ul id="results">` echoing rows containing `q` and the exact `q` text
  - `More = GET "/todos/more" { after: Int }`: 25 rows total; view per spec's shape; last page renders no sentinel
  - `Slow = GET "/slow" {}`, `Badge = GET "/badge" {}` (count from an in-memory counter row bumped by a write route the test calls)

- [ ] **Step 1: Write failing tests.**
  - `TestTest_Patterns` runs the app's own `alder test` tests (in `Patterns.alder`): search `Test.input (Results { q: "mi" })` patches `#results`; infinite scroll two `Test.intersect` calls then the third page has no sentinel (`Test.intersect (More { after: 30 })` fails as missing); `Test.load (Slow {})`; `Test.load (Badge {})` twice around a write that bumps the count.
  - `TestBrowser_Patterns` (Playwright) starts the built app, fronts it with an `httptest` reverse proxy recording `loads[path]` under a mutex, and runs `patterns.cjs`. Asserts, each as one JSON error list:
    - search: typing `mil` quickly sends one `/results` request with `q=mil`; typing `a&b #c+é` lands that exact text echoed in `#results`; with the proxy delaying the first `/results` answer 800 ms, typing `mi`, waiting 400 ms, typing `l` ends with `#results` showing `mil` (older answer aborted)
    - scroll: scrolling to the bottom twice loads `/todos/more?after=10` and `?after=20` once each; 25 rows; no further request
    - slow: one `/slow` request on load; `#slow` patched
    - badge: over ~2.5 s, `/badge` requests == 1 (init) + ticks, never 2 per tick
- [ ] **Step 2: Run** `go test ./cmd/alder -run TestTest_Patterns` and `go test -tags browser ./cmd/alder -run TestBrowser_Patterns` — FAIL. If the badge count shows `data-init` re-running after the patch, apply the spec's fallback: render the interval only (drop `data-init` when `Every != ""` in gen), add `__duration.<d>.leading`, and say so in the book.
- [ ] **Step 3: Implement** the app and fix anything the end-to-end runs expose. Write the book section: the four patterns, each with its `alder` snippet (checked by `go test ./book`), the `More` shape, the `every` units with "minutes render as seconds", and a sentence that these need JavaScript.
- [ ] **Step 4: Run** `go test ./book`, `go test -tags browser ./cmd/alder -run TestBrowser_Patterns`, `go vet ./...`, `scripts/test.sh` — PASS.
- [ ] **Step 5: Commit** `book: route trigger patterns (#160)` (app/tests may be a separate `test:` commit); push; open PR 10.
