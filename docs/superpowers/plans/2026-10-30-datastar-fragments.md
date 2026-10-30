# Plan: Datastar fragments (#49, part 1)

Spec: `docs/superpowers/specs/2026-10-30-datastar-fragments-design.md`. Stacked on #104
(`alder bench`). TDD: failing test, code, green, commit per task.

## Task 0: the invariant first (rt/testing)

- `rt/testing/browser_test.go` `TestBrowser_PatchedPageEqualsFullRender`: a rows page with
  `<li id="row-N">`, click Toggle on row 7; the held doc shows the toggled row and equals a fresh
  GET's body (al-shown dropped). Written first; it passes on today's body morph and keeps passing
  through every task below.

## Task 1: regions and the hash (rt/html)

- `rt/html/html.go`: `Rendered{Page string; Regions []Region}`, `Region{ID; Start, End int}`,
  `RenderTracked(n, st) Rendered` (the renderer records every element with a non-omitted `id`
  outside `<head>`/`<template>`, offsets into Page, document order; `Unsafe` set when a Raw node
  holds `id=`: its ids cannot be proved unique, so the page has no regions).
- `rt/html/fragments.go`: `Hash(body) string` (FNV-1a 64, 16 hex), `Patch{Selector, Elements}`,
  `Fragments(before, after Rendered) []Patch` (the skeleton rule; ids matching
  `^[A-Za-z_][A-Za-z0-9_-]*$`, unique in both renders, in both), `BodyOf`.
- Tests: `TestRenderTracked_RegionsInDocumentOrder`, `TestFragments_*` (unchanged, one region,
  nested child only, skeleton change: added/removed/reordered, text outside → body, duplicated
  id, bad id, id in head, raw id, apply-patches property over the table), `TestHash`.

## Task 2: the header and ReplaceByID (rt/html)

- `ShownHeader = "Alder-Shown"`; `ClickPost`/`SubmitPost` send `headers: window._alderH?.()`;
  `datastar.BoostInit` defines `window._alderH` reading `#_alder_shown`'s `data-h`.
- `ReplaceByID(doc, id, with) (Node, bool)` in parse.go.
- Tests: `TestClickPost_SendsShownHeader`, `TestSubmitPost_SendsShownHeader`, `TestReplaceByID`,
  `TestBoostInit_DefinesShown` (rt/datastar).

## Task 3: SSE events (rt/datastar)

- `PatchRegion(w, id, elements)` (`#id`, outer), `ShownID`, `ShownTag(hash)`,
  `PatchShown(w, hash)`.
- Tests: `TestPatchRegion_EventFormat`, `TestPatchShown_EventFormat`.

## Task 4: the router (rt/web)

- `Response.Before func() html.Node`; `Options.Rendered func(rq *Request, body string)`.
- `document` and every body morph (`write`'s full path, `morph`) carry `ShownTag(Hash(body))`.
- `write`: the decision (no Before / no header → body; Before panics → logged, body; hash
  mismatch → body; equal bodies → no element event; else region patches + shown). A notice on a
  page without its own `<al-toasts>` morphs the body (the implicit region must be in the DOM).
  Tracker patches: `#id`…, `shown`, `body`, `none`. `Rendered` called on each.
- Tests (rt/web/fragments_test.go): `TestRerender_PatchesChangedRegion`,
  `TestRerender_SkeletonChangeMorphsBody`, `TestRerender_ShownMismatchMorphsBody` (absent, wrong),
  `TestRerender_NoBeforeMorphsBody`, `TestRerender_NoChangeSendsNoElements`,
  `TestRerender_BeforePanicMorphsBody`, `TestRerender_NoticeWithoutToastsMorphsBody`,
  `TestRerender_RejectedFormPatchesItsRegion`, `TestDocument_CarriesShown`,
  `TestPanel_ListsRegionPatches`, `TestRerender_CallsRendered`.

## Task 5: generated code (internal/gen)

- `update__M`: `model0, session0 := model, session` before `eff.Run`; the Rerender/Reject
  `PageResponse` gets `Before = func() html.Node { return view(model0, session0) }`; the
  form-rejected and option-check paths set `Before` to the same model's view. A small
  `web.Response.WithBefore(f)` keeps the generated lines short.
- Goldens regenerated (`go test ./internal/gen -update`).

## Task 6: the test browser (rt/testing)

- POSTs send `Alder-Shown` from the held `al-shown`; `#id` outer patches via `ReplaceByID`;
  `Options.Rendered` captures the full-render body; after a Rerender/FormRejected stream the held
  body (al-shown and the implicit al-toasts dropped) must equal it, else the step fails with
  `the patched page differs from a full render at …`. The check runs outside the bench measure.
- Tests: `TestBrowser_SendsShown`, `TestBrowser_ReportsFragmentDrift` (a route whose Before lies),
  `TestBrowser_PatchedPageEqualsFullRender` now goes through fragments (asserts a `#row-7` patch).

## Task 7: bench fixture and numbers

- `testdata/bench/rows/app/Rows/Index.alder`: `<li id={"row-" ++ …}>`.
- `cmd/alder` `TestBench_Rows`: toggle 1000 POST bytes < 2% of index 1000 GET bytes.
- Run `alder bench testdata/bench/rows --count 10` before (on #104) and after; "#49" section in
  `docs/perf/2026-10-bench-baseline.md`.

## Task 8: book

- `05-html.md` "Partial updates", `13-compiler.md` Toggle walk-through, `11-tooling.md` Patches
  tab and `bytes`; `python3 book/site/build.py`; `cd experiments && python3 -m unittest`.

## Finish

`go vet ./...`, `go test ./...`; merge latest origin/issue-48-perf-harness; push to
`issue-49-datastar`; PR `--base issue-48-perf-harness`.
