# Datastar fragments: patch what changed, keep MVU

Design for [tapegram/alder#49](https://github.com/tapegram/alder/issues/49). Part 1 (fragments) is
specified for building; part 2 (signals vs model/update/view) is a position, marked later work.
Everything marked **(call)** is the author's decision, not the user's. Builds on PR #104
(`alder bench`, branch `issue-48-perf-harness`): merge it first; the numbers below are its baseline.

## Intent

Today every `Rerender` answers a Datastar request with one `datastar-patch-elements` event carrying
the whole `<body>` (`rt/web/router.go` `write` → `datastar.PatchBody`). `alder bench` on
`testdata/bench/rows`: `toggle 1000`'s POST sends 808.4 kB for one flipped row, against 811.3 kB
for the whole page (`index 1000`) — a click costs a page, and the morph has to walk 1,000 rows on
the client. The ratio, not the ms (taken under load), is the evidence: toggle bytes / index bytes =
0.996. After this change the server renders the page the client is showing and the page it should
show, and sends only the elements that differ, each found by its `id`. The view stays a pure
function of the model; the server stays the source of truth; the client holds one 16-character
hash.

## Goals

1. `toggle 1000` POST `bytes` ≤ 2% of `index 1000` GET `bytes` (expected: about 0.5 kB).
2. The patched DOM equals a full render, checked on every `Rerender` of every `alder test`.
3. No language change: an author opts into finer patches by giving elements an `id`, as the
   book already advises (ch 5 "Partial updates").
4. Every path that cannot prove the client's page falls back to today's full-body morph.

## Non-goals

- Boosted navigations and followed Redirects (`jump`): they keep the body + title + history morph.
  Different pages share no "before" the server can render **(call)**.
- Keyed child diffs (`add 1000` keeps patching `#rows` whole: the `<ul>`'s child count changed).
  A follow-up issue, measured against this one's numbers.
- Compressing the SSE stream. gzip would cut wire bytes but not the client's morph of 1,000 rows;
  it is orthogonal and can be added by the host.
- Any Datastar signal, `data-show`, `data-bind` or client-side expression (part 2).

## Anti-goals

- Author-declared patch targets (`msg Toggle patches "#rows"`): a wrong one silently diverges the
  DOM from `view model`; the equality guarantee of goal 2 would be unprovable.
- Server-side session state per tab (a cache of the last body): breaks across instances and
  restarts, and the hash does the same job in 16 bytes.
- Positional selectors (`body > :nth-child(3) > li:nth-child(7)`): a browser extension that
  prepends to `<body>` would shift every patch. Only `#id` and `body` are ever used.

## Part 1: fragments

### Regions

A **region** is an element whose `id` is unique in both renders being compared and matches
`^[A-Za-z_][A-Za-z0-9_-]*$` **(call)** (so `#id` is a safe selector without escaping). The
`<body>` is the outermost region, selector `body`. Regions nest. Elements inside `<head>`, and
ids that are duplicated in either render or do not match the pattern, are not regions: they are
plain content of the nearest enclosing region.

The renderer records regions as it writes (`html.RenderTracked(n, st) Rendered`:
`Page string` and `Regions []Region{ID string; Start, End int}` in document order, offsets into
`Page`). No parse, no second walk.

### The before and after

The generated update route (`internal/gen/routes.go`, `update__M`) already loads the model before
`update` and again after it. It gains `Before`: a thunk rendering `view model0 session0` with the
plain state a GET uses (`&html.FormState{PageParams: …}`), captured before `update` runs, set on
the `Response` (`web.Response.Before func() html.Node`, nil for GETs). The form-rejected path
(decode failure, no `update`) sets it too: same model, plain state. A `Reject` on a form is a
`Rerender` with `FormError`: the same thunk.

### The shown hash

`hash(body)` is FNV-1a 64 of `bodyOf(page)` as the view rendered it (before `shellBody`'s and
`dev`'s additions), 16 lower-case hex characters **(call)** — not security relevant: a client
that lies only mispatches its own DOM, and only to what a full morph would have shown anyway.

- Every page the router sends carries `<al-shown id="_alder_shown" data-h="<hash>" hidden></al-shown>`
  at the end of `<body>` (`endOfBody`, as the implicit `<al-toasts>` is added today): in
  `document`, in a full-body morph, and after fragments as its own patch (`#_alder_shown`,
  outer). `al-shown` is an unregistered custom element: inline, empty, hidden.
- `ClickPost` and `SubmitPost` (`rt/html`) send header `Alder-Shown` (`html.ShownHeader`) with
  `document.getElementById('_alder_shown')?.dataset.h||''`. The boost's GETs send nothing.
- Without JavaScript nothing changes: a plain POST is a 303 (Post/Redirect/Get).

### The decision

In `write`, for a 200 with `Patch` under Datastar, the router renders `after :=
RenderTracked(resp.Node, resp.State)` as today and then:

1. `resp.Before == nil`, or the request has no `Alder-Shown` → full morph (today's path).
2. `before := RenderTracked(resp.Before(), plain)`; a panic in it is recovered and logged →
   full morph **(call)**.
3. `hash(bodyOf(before)) != Alder-Shown` → full morph. (The first click after a rejected form,
   a page whose `load` reads the clock, a tab from an older build.)
4. `bodyOf(before) == bodyOf(after)` → no element event at all; the `al-shown` patch is not sent
   either (the hash is unchanged). Toast and panel events follow as today.
5. Otherwise `html.Fragments(before, after)` gives the patches, written in document order, then
   the `al-shown` patch with `hash(bodyOf(after))`.

`html.Fragments` **(call)**, on the body substrings with the region tables (regions in both,
by id): compare(region R):

- the markup of R is equal on both sides → nothing;
- else its **skeleton** — R's markup with each direct child region replaced by `<!--r:id-->` —
  is equal on both sides (so the same child regions, in the same order, and everything between
  them identical) → compare each direct child region;
- else → one patch: selector `#id` (or `body`), mode `outer`, elements = R's after-markup.

Start at the body. The result is a set of disjoint patches; applying them to the before-DOM gives
the after-markup exactly, because everything outside a patched region was proved equal by a
skeleton and everything inside by the patch. A changed region's id is the same on both sides, so
`#id` exists in the client's DOM. Text outside any region (`<h1>Count: 5</h1>` with no id'd
ancestor) patches the body, as today. Regions added, removed or reordered change their parent's
skeleton, so the parent is patched.

Datastar v1.0.0 applies `outer` as a morph of the matched element, keeping `data-ignore-morph`
children and `data-preserve-attr` attributes, so the dev panel, the toasts region, an open
`<al-dialog>` and typed input values behave as they do under a body morph — and the regions
not patched are not touched at all, which is the point.

### Tooling and measurement

- The tracker's `patches` (dev panel, Patches tab) lists what was sent: `#row-7`, `shown`, or
  `body`, or `none`.
- `alder bench`: `bytes` is the SSE text, so it falls by itself. The fixture's rows get ids:
  `<li id={"row-" ++ r.id}>` in `testdata/bench/rows/app/Rows/Index.alder` (a brand widens to
  `Text`). Expected rows after the change, same machine, idle, `--count 10`:
  `toggle 1000` POST `bytes` ≈ 0.5 kB (one `<li>` plus the `al-shown` event; was 808.4 kB);
  `alloc` rises by about one render (the before-view: ≈ +15 MB of the 31.97 MB), `loads` stays 2,
  `sql` 4, `req` 1; `add 1000` POST stays ≈ the `<ul>` (≈ 790 kB; non-goal); `index` rows
  unchanged but for the `al-shown` element (+~60 B).
- Keep rule for this change **(call)**: `toggle 1000` `bytes` ≤ 2% of `index 1000` `bytes`;
  `toggle 1000` `alloc` ≤ 1.7× its baseline; `sql`, `loads`, `req` unchanged on every row. `ms`
  is reported, not gated. The builder re-runs the baseline idle before and after (the baseline
  file says so) and adds a "#49" section to `docs/perf/2026-10-bench-baseline.md` with both.

### The correctness check

`web.Options` gains `Rendered func(rq *Request, body string)` **(call)**: called for every
Datastar page answer (full morph or fragments) with `bodyOf(after)` — the body a full morph
would have sent. Nil in production (the generated `main`); `rt/testing`'s `Browser` sets it.

The `Browser` sends `Alder-Shown` from the `al-shown` in the page it holds, applies `#id`/outer
patches by replacing the element with that id (`html.ReplaceByID`), and after every event stream
of a `Rerender` compares the body it now holds against the captured body (`html.Diff`, both with
`al-shown` and the implicit `al-toasts` dropped). A difference fails the step:
`the patched page differs from a full render at body > ul#rows > li:nth-child(7): …`. This runs
in every `alder test` and `alder bench` click and submit, so each app's own suite proves the
fragment path on its own pages; it costs a parse and a diff after the measured request, no bytes.

## Part 2: signals vs model/update/view — a position (later work)

**Rule (call).** Client-only state is allowed exactly when `update` could never need it: the
server never reads it, never stores it, and `view model` is still the page with that state at its
initial value. If a message would ever branch on it, it is model state and goes through `update`.
Nothing in part 2 is needed for part 1; it becomes its own issue once nightvote-replay has a
concrete case beyond the ones named here.

**Tier 1 — platform state, no language change.** Open `<details>`, a `<dialog>` or popover, the
active element, scroll, half-typed inputs. Fragments are the real fix: a region that did not
change is not morphed, so this state survives every click outside it. The one addition worth
making: the renderer adds `data-preserve-attr="open"` to `<details>` as it already does for
`<al-dialog>` (ch 13), so a morph of the region keeps the disclosure where the user left it.

**Tier 2 — declared locals, a language change.** A page may declare
`local tab : Text = "all"` (types `Bool`, `Int`, `Text` only; a literal initial value). A local is
a Datastar signal, initialised on `<body>` with `data-signals__ifmissing` so a body morph does
not reset it, and is usable only in three attributes of the view, each compiled to a Datastar
expression over a tiny client language (locals, literals, `==`, `/=`, `not`, `&&`, `||`):
`show={tab == "done"}` → `data-show`, `set={tab = "done"}` on a button → `data-on:click`,
`toggle={open}` → `data-on:click="$open=!$open"`. A local cannot appear in any other expression:
the checker rejects `if tab == "done"` in `view`, since the server does not know `tab`, and
rejects it in `update`, since it is not in `Msg` or `Model`. Why this keeps MVU: the markup is
still `view model`, byte for byte, with the locals as attributes; the DOM is that markup with
`data-show` applied — a layered invariant, `DOM = apply locals (view model)`, where the second
layer is typed, declared in one place and never reaches the server. `alder test` compares
markup, so goal 2's check holds unchanged, and a test cannot observe a local (it is not state
the app can act on). Why not `data-bind`, `data-text`, `data-computed`: they would put logic in
two languages, let the page show something `view` did not compute, and make the equality check
meaningless.

**Not covered by either tier:** timers (nightvote-replay's Test page refreshes with a
body-level `data-effect`; dev log 2026-10-24). That is a server round trip on a clock, not
client state: an `every={30s}` page attribute compiling to `data-on-interval` with a message is
the shape, and belongs with tier 2's issue.

## Where things change

| layer | files |
|---|---|
| renderer | `rt/html/html.go`: `RenderTracked`, `Rendered`, `Region`; `ShownHeader`, the header in `ClickPost`/`SubmitPost`; `ReplaceByID` |
| fragments | new `rt/html/fragments.go`: `Fragments(before, after Rendered) []Patch`, `Hash(body string) string`, the skeleton rule, the id pattern |
| SSE | `rt/datastar/datastar.go`: `PatchRegion(w, id, elements)`, `PatchShown(w, hash)`, `ShownTag(hash)` |
| router | `rt/web/request.go`: `Response.Before`, `Options.Rendered`; `rt/web/router.go` `write`: the decision, `al-shown` in `document` and full morphs, tracker `patches`; `rt/web/devpanel.go`: the Patches tab wording |
| generated code | `internal/gen/routes.go`: `model0, session0` kept before `eff.Run`, `Before` on the update route's and the form-rejected `PageResponse`; `testdata/` goldens regenerated |
| test browser | `rt/testing/browser.go`: `Alder-Shown` on POSTs, `#id` patches, the `Rendered` hook and the equality check |
| bench fixture | `testdata/bench/rows/app/Rows/Index.alder`: `id` on `<li>`; `docs/perf/2026-10-bench-baseline.md`: "#49" section |
| book | `05-html.md` "Partial updates" (regions, what is patched, the guarantee); `13-compiler.md` the Toggle walk-through (before-view, `Fragments`, the SSE now two small events); `11-tooling.md` the Patches tab and the `bytes` column; `book/site/build.py` regenerates |

No parser, checker, IR, formatter, LSP or tree-sitter change in part 1.

## Tests the builder adds (by behaviour)

- `rt/html` `TestFragments_*`: unchanged bodies give no patch; one changed region gives one
  `#id` patch; a changed region inside an unchanged-skeleton parent patches the child only; a
  parent whose skeleton changed (child region added, removed, reordered) patches the parent;
  text outside any region patches `body`; an id duplicated in either render is not a region; an
  id outside the pattern is not a region; an id in `<head>` is ignored; applying the patches to
  the before-markup reproduces the after-markup (property over the table's cases).
- `rt/html` `TestRenderTracked_RegionsInDocumentOrder`, `TestClickPost_SendsShownHeader`,
  `TestSubmitPost_SendsShownHeader`, `TestReplaceByID`.
- `rt/datastar` `TestPatchRegion_EventFormat` (`selector #row-7`, `mode outer`),
  `TestPatchShown_EventFormat`.
- `rt/web` `TestRerender_PatchesChangedRegion`, `TestRerender_SkeletonChangeMorphsBody`,
  `TestRerender_ShownMismatchMorphsBody` (absent header, wrong hash), `TestRerender_NoChangeSendsNoElements`,
  `TestRerender_BeforePanicMorphsBody`, `TestRerender_RejectedFormPatchesItsRegion` (form with
  an id: errors arrive as one patch; the next click, whose before differs from shown, morphs the
  body), `TestDocument_CarriesShown` (document and full morph carry `al-shown` with the body's
  hash), `TestPanel_ListsRegionPatches`.
- `rt/testing` `TestBrowser_PatchedPageEqualsFullRender` (a click on a rows page: the held doc
  shows the toggled row and equals a fresh GET), `TestBrowser_ReportsFragmentDrift` (a route
  whose `Before` lies: the step fails naming the difference), `TestBrowser_SendsShown`.
- `internal/gen` golden: the update route keeps `model0, session0` and sets `Before`.
- `cmd/alder` `TestBench_Rows`: `toggle 1000` `bytes` under 2% of `index 1000`'s.

## Left to the builder

- Whether `Rendered` is a struct or `RenderWith` grows a second return; `Patch`'s field names.
- How `Browser` drops `al-shown`/`al-toasts` before the diff (a small strip helper is fine).
- Book wording; the exact golden updates.

## Verification

- `go vet ./...`, `go test ./...` green including every test above; `cd experiments && python3 -m unittest`.
- `alder bench testdata/bench/rows --count 10` idle, before and after, recorded in the baseline
  file: the keep rule holds (toggle bytes ≤ 2% of index bytes; alloc ≤ 1.7×; sql/loads/req equal).
- Scratch check on nightvote-replay (`alder test` with this build, not a change to that repo):
  every suite passes — the equality check in every click is the proof the fragment path holds
  on real pages — and the dev panel's Patches tab on a Start/Restart shows region ids, not `body`.
