# Item 3 follow-ups (#177, #180, #185, #192, #193, #195) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the earlier-PR follow-ups left in #52 item 3: #177, #180, #185, #192, #193, #195 (#188 is closed).

**Architecture:** Ten small tasks on main, one or more commits each. Tasks touch disjoint areas except
Tasks 2 and 3 (both `internal/check`, `internal/diag/explain.go`, book ch. 11): run them in order.
Each task names its model: Sonnet for the well-specified ones, Opus for the runtime ones (Tasks 4, 7).

**Tech Stack:** Go (checker `internal/check`, names `internal/names`, LSP `internal/lsp`, semtok
`internal/semtok`, generators `internal/generate`, mutation `internal/mutate`, runtime `rt/web`, test
browser `rt/testing`, CLI `cmd/alder`), tree-sitter grammar `tree-sitter-alder/`, book `book/*.md`,
corpus `testdata/corpus/<CODE>-<slug>/{app/*.alder,expected.diag}`.

**Spec:** the six issues and their comments (decisions of 2026-11-10). Facts gathered for this plan
(current file:line, existing tests to model on, repro output), checked against bba9d770:
`docs/superpowers/plans/2026-11-10-item3-facts/{177-192,180,193-195-185}.md`.
Line numbers below are from bba9d770; re-grep if a line has moved.

## Settled decisions (do not reopen)

- #177: the checker rejects `click=` that targets a write route with `form F` (E0045, hint: use a submit button in a form for that route). No in-form posting of the enclosing form.
- #180: the route part of `Index.load` stays coloured as a namespace; no new colour.
- #185: report one error, not two. The message never mentions `alder fix --routes` (removed, #188 closed).
- No migration tool, no `resource` syntax.

## Already fixed on main (close with no code change)

- #185: E0010 no longer fires on an unqualified member; since #205 it is E0046 (`internal/check/removed.go:47`). What remains: the duplicate E0043 and the E0046 text (Task 2).
- #195: the `routes`, `edit` and `remove` summaries and the `edit auth --to <Route>` / `edit session --choose <Route>` usages already say route (08ae920d). What remains: `--bare` and the other stale texts in Task 10.
- #193 "Search routes' `role dropped`": premise gone. #184 made admin search a GET of the page, so the admin has no Search write routes (Task 9 re-measures).

## Global Constraints

- Commit on main; every commit message ends with `(#NNN)` naming the issue, then the attribution lines.
- Update goldens and `expected.diag` with the repo's update flags (`-update`); never hand-edit a golden. `testdata/help.json` has no flag: regenerate with `go run ./cmd/alder help --json > testdata/help.json` and keep `"version":"(devel)"`.
- Book text follows the code in the same commit. Route-era vocabulary: route, GET route, write route; never page or message for current behaviour.
- Run only the packages you touched (`go test ./<pkg> -run <Name> -count=1`); the full suite (`scripts/test.sh`) runs once, in Finish.
- `gofmt -l .` and `go vet ./...` clean before each commit.
- Working hours: no agent runs 9am–5pm America/New_York on weekdays. The mutation gate run (Task 9) takes up to 40 min: start it only if it ends before 9am.
- Close each issue with a one-line comment naming the commit(s); for "no change" items, say why in that comment.

## Review Focus

1. A button inside a form that clicks a write route WITHOUT a form must still compile and send `{}` JSON (Task 3 test pins the `sendRoute` default branch).
2. An unqualified `update` in a module with two write routes must list both, not pick one (Task 2 test).
3. A computed route (`click={model.r}`) cannot be checked statically; the E0045 rule applies to literal route values only and must not crash on a computed one (Task 3 test).
4. The access line, the error line and the metrics label must all say DELETE for a `_method=DELETE` POST, while path masking stays right (Task 7 test).
5. Insert-signatures must not add a blank line at the end of a file that already ends blank, and the inserted holes must build (Task 5 test).

---

### Task 1: one E0201 per missing module (#192) — Sonnet

**Files:** `internal/names/resolve.go:730` (`notImported`), test `internal/names/qualify_resolve_test.go`, new corpus `testdata/corpus/E0201-nested-twice/`.

- [ ] **Step 1: Test.** `TestQual_NotImportedOncePerModule` (model: `TestQual_UnqualifiableDoesNotHideRealModule`, `resolveMods` + `codes(bag)`): module `Table.Todo` declaring `type Filter = All | Active` and `type alias Todo = { … }`; module `Use` with no import, using `Todo.Filter` in one annotation and a bare `List Todo` in another. Assert exactly one E0201, naming `Table.Todo`, and that it carries the `import Table.Todo` fix. Add corpus case `E0201-nested-twice` (app with `Table/Todo.alder` and an `Index.alder` that uses both forms), `expected.diag` written with `-update`, one E0201 line. Run; expect FAIL (two E0201).
- [ ] **Step 2:** In `notImported`, resolve `x` with `r.p.moduleByLastSegment(x)` before the `r.reported` check (keep `r.missing` / `r.isUnqual` on the name as written), and key `reported` on the resolved name.
- [ ] **Step 3:** `go test ./internal/names -run TestQual_ -count=1` and `go test ./internal/check -run 'TestCorpus/E0201' -count=1` pass. Commit `names: E0201 once per missing module, keyed on its resolved path (#192)`.

### Task 2: unqualified member in a route module names its route (#185) — Sonnet

**Files:** `internal/check/removed.go:30-71` (`rejectRemoved`), `internal/check/routes.go:286` (write-route E0043), `:399`/`:402` (GET-route E0043s), `internal/diag/explain.go:15` (E0010) and `:65` (E0046), `book/11-tooling.md:2589+` (E0046 section; the E0010 section at 2498 already says route members are E0043, so only check it), corpus.

**Interfaces:** Produces the E0046 message for an unqualified `load`/`view`/`update` in a module with a route:
- one candidate: ``` `update` must be qualified with the route it belongs to: `Submit.update` (route `Submit` is the only write route in `Home`) ```
- several: ``` `update` must be qualified with the route it belongs to: one of `A.update`, `B.update` (the write routes in `Home`) ```
- none (no route of that kind in the module): keep today's E0046 text.
Candidates are the module's routes that need the member (`update`: write routes; `load`/`view`: GET routes) and do not already declare it.

- [ ] **Step 1: Tests.** Corpus `E0046-unqualified-update` (the #185 repro: `route Home = GET "/" { }`, `route Submit = POST "/_/Submit" { } form Sign`, unqualified `update : Submit.Params -> Session -> Task Response`): `expected.diag` holds exactly the one-candidate E0046 line and no E0043. Corpus `E0046-unqualified-update-two`: two write routes, both without `X.update`, one unqualified `update`: one E0046 listing both, and an E0043 for neither. Corpus `E0046-unqualified-view`: a GET route `Home` without members and unqualified `load`/`view`: two E0046 (one per member) naming `Home.load` / `Home.view`, no E0043. Run `go test ./internal/check -run 'TestCorpus/E0046' -count=1`; expect FAIL.
- [ ] **Step 2:** In `rejectRemoved`, compute the candidates per unqualified member and choose the message above. Suppress the E0043 "needs `R.member`" for every candidate route (drop those bag items in `rejectRemoved`, which already prunes the bag for rejected modules at `removed.go:~62-70`, or skip them at emission; either way the user sees one error per unqualified member).
- [ ] **Step 3: Docs.** E0010 explain text: drop the route clause (route members are E0043); keep `App` needs `error` and `notFound`, and a `login` route and `session` if any route uses `requires`. E0046 explain text and book ch. 11's E0046 section: add that an unqualified `load`/`view`/`update` in a module with routes names the route(s) to qualify it with, and that the route's own E0043 is then not shown. Check the E0010 section at 2498 still matches the explain text. No mention of `alder fix`.
- [ ] **Step 4:** `go test ./internal/check ./internal/diag -count=1` passes (explain goldens updated if any). Commit `check: an unqualified member in a route module names its route, one error (#185)`.

### Task 3: reject `click=` on a write route with a form (#177 items 1–2) — Sonnet

**Files:** `internal/check/html.go:298` (the `click`/`submit` attr case), `internal/diag/diag.go:146` (E0045 comment), `internal/diag/explain.go:63`, `internal/check/routes_test.go:~231` (message list), new corpus `testdata/corpus/E0045-click-form`; book `book/05-html.md:159` and the "Buttons with click" section (~165–230), `book/06-routes.md:~258`, `book/11-tooling.md:2578-2587`, `book/10-testing.md:1728-1745`; test `rt/testing/browser_test.go:1295` (`TestBrowser_ClickFormRouteSendsThePageBody`).

**Interfaces:** E0045 (reused; `Test.click` of a form route is already E0045, `internal/check/browser.go:~378`), message:
``` `click` sends a route without a form; `Submit` posts form `NewTodo`: use a submit button in a form for that route (`<form submit={Submit}>`) ```

`load=`/`intersect=`/`input=`/`every=` already reject every write route (E0045), so they need nothing.

- [ ] **Step 1: Tests.** Corpus `E0045-click-form` (model `E0045-test-click-form`): `<button click={Submit}>` where `route Submit = POST "/todos" {} form NewTodo`, once outside any form and once inside another form; expected two E0045 lines. Same file: a `click={Toggle {…}}` on a write route without a form, inside and outside a form, gets no diagnostic; a `click={model.r}` (computed route) gets no diagnostic. Add the message to the list in `routes_test.go`. Run `go test ./internal/check -run 'TestCorpus/E0045|TestRoutes' -count=1`; expect FAIL.
- [ ] **Step 2:** In the `click` branch, after `ct, r := mc.routeTarget(a.Value)`, when `r != nil && isWrite(r.Method) && r.Decl.Form != ""`, `c.errorf(diag.RouteTarget, …)` with the message above. Add "click of a route with a form" to the E0045 comment in `diag.go`.
- [ ] **Step 3: `sendRoute` default branch.** After Step 2 the remaining branches of `rt/testing/browser.go:959` `sendRoute` are: wrapper form (write route without form, outside a form), `GET`/`DELETE` inside a form (`datastar={}` query), and the default (POST/PUT/PATCH without form, clicked inside another form: `{}` JSON). None is dead, so keep the code. In `TestBrowser_ClickFormRouteSendsThePageBody`'s fixture page (`browser_test.go:946-1000`) add an in-form click of a PATCH route with no form and assert `s.method == "PATCH"`, `s.ctype == "application/json"`, `s.body == "{}"`. Also drop `form PurgeF` from the `rPurge` fixture (`:990`) so the fixture matches what the checker now allows, and rename the test if its name no longer fits (e.g. `TestBrowser_InFormClickSendsNoFormBody`). `go test ./rt/testing -run 'TestBrowser_Click|TestBrowser_InForm' -count=1` passes.
- [ ] **Step 4: Docs.** E0045 explain text, ch. 11 E0045 bullets, ch. 5 callout and "Buttons with click" (a click sends a route without a form; a route with `form F` is sent by a `<form submit=…>`), ch. 6 E0045 sentence, ch. 10 `Test.click` (already says a form route is posted with `Test.submit`; add that the markup has the same rule).
- [ ] **Step 5:** `go test ./internal/check ./internal/diag ./rt/testing -count=1` passes. Commits: `check: click= of a route with a form is E0045 (#177)`, `test: pin the in-form click of a write route without a form (#177)`.

### Task 4: test browser element check and build meta (#177 items 3–7) — Opus

**Files:** `rt/testing/browser.go` (`hold` :710, `elementByID` :845, `checkElement` ~:1060–1145, `quietly` :1145), `rt/web/router.go:~846-889` (element fetch and `notAllowed`), tests `rt/testing/browser_test.go`, `rt/web/method_test.go`, `book/10-testing.md` (element check paragraph, fakes 766–830).

- [ ] **Step 1: `hold` reads the build (item 3).** Test `TestBrowser_StaleBuildSent`: `tamper` the page's `<meta name="alder-build" content="…">` to `content="old"`, click a route, assert `s.build == "old"`. Implement: `hold` parses the meta's `content` (no `strings.Contains(page, web.BuildMeta())`) and sets the browser's build to it; no meta → "". Existing `s.build` assertions (browser_test.go 1169–1313) keep passing.
- [ ] **Step 2: element fetch of a path with other methods (item 4).** Test in `rt/testing`: a GET route clicked whose path is declared only as POST/PATCH answers `Answer{Tag: PageMissing}` and the `{error, "Not found"}` toast (model `TestBrowser_ClickUnmatchedGetRoute` :1429); and in `rt/web`, a Datastar element-fetch GET to such a path gets the not-found render, not the 405 body patch (model `TestRouter_405Allow` :62, case :102). Implement: in the router, before each `rt.notAllowed` call reachable by a GET (the `hit == nil && r.Method != POST` branch), `if elementFetch(r) { rt.renderFailed(w, r, nil, NotFoundResponse(html.None()), "element fetch"); return }`. A non-element 405 is unchanged.
- [ ] **Step 3: own reason for a missing id (item 5).** Test `TestCheck_ElementIDMissingAlone` (model `TestCheck_RenderTargetMissing` :1414): the fetched view on its own has no element with the id; the failure's `Why` is `on its own the view has no element with id <id>`. Implement: keep the `ok` from `elementByID(alone, id)`.
- [ ] **Step 4: `quietly` restores state (item 6).** Test `TestCheck_ElementFetchRestoresState` (model `TestCheck_ElementFetchRolledBack` :1494, `dbRenderRoutes`): snapshot `e.Trace`, the recorded statements (`b.reqEnvs`), `b.model`, `b.rendered`, `b.answered`, `b.lastErr` before `b.quietly(…)` runs a fetch that changes each, and assert all are equal after. Expect PASS on the first run (coverage); temporarily drop one restore in `quietly` to see it FAIL, then restore.
- [ ] **Step 5: fakes run twice (item 7): document, no code.** A record/replay of Query capabilities needs a hook in `rt/eff` per capability call and generated wrappers; not worth it for a check. Say in the `checkElement` comment and in ch. 10's fake section that the element check runs the route's `load` a second time, so a fake's Go-side state (not its tables, which the savepoint rolls back) sees two calls. Close the item with that reason.
- [ ] **Step 6:** `go test ./rt/testing ./rt/web -count=1` passes. One commit per step 1–4, plus one for 5, each `(#177)`.

### Task 5: LSP follow-ups (#180 LSP items) — Sonnet

**Files:** `internal/lsp/references.go` / `scope.go` (`useSpan` :117, `routeQualifierAt` :368, `onRoutePart` :357), `internal/lsp/hover.go:113-128,264-295`, `internal/lsp/actions.go:176-251` (`routeSignatureActions`, `wrap` :267), tests `internal/lsp/routes_test.go`, `internal/lsp/hover_test.go`.

- [ ] **Step 1: references (item 1).** Tighten `TestDefinition_FromClick` (routes_test.go:95) from `len(refs) >= 2` to the exact set of spans: the route declaration, every constructor use, and every member use (`Row.view` / `Row.load` / `Row.Params`, at the call, the signature and the definition; the span covers the route part `Row` only). Run; FAIL. Implement in `useSpan`: when the target is a route, a `Var` (RefValue) or `TypeName` (RefType) in the same module whose name starts with `<Route>.` and `a.prog.Route(module, <Route>)` exists counts as a use, span = the route segment. `go test ./internal/lsp -run 'TestDefinition_|TestReferences' -count=1`.
- [ ] **Step 2: hover names the member's own session (item 2).** Test in `routes_test.go`: hover on `Row.view` where `Row` shares its `requires` with `Index` shows `Todos.Row.view : Todo -> Row.Session -> Html Route` (not `Index.Session`). Implement in hover only: when printing a route member's type, print the shared narrowed record as that member's own `R.Session`. The checker's shared record (routes.go:216-221) is unchanged, so type identity across routes is unchanged; error messages may still print the first route's name (note it in the commit).
- [ ] **Step 3: insert-signatures (item 3).** Update `TestAction_InsertSignatures` (:183): with no members yet, the inserted block goes right after the route's declaration line (blank line before, none doubled); with members, after the last member (today's rule); `Count.view : List Todo -> Session -> Html Route` (no parens: `wrap` parenthesises function types only); each signature is followed by its definition with a hole, e.g. `Count.load params session = ?` and `Count.view model session = ?` (parameter names as in the starter generators). Add a case for a file ending in a blank line: the result has no double blank line at EOF. Add a test that the file after applying the edit builds with only W0002 hole warnings (so fill-with-AI can take over). Run; FAIL; implement; pass.
- [ ] **Step 4: no change for item 4** (the action disappears after an edit since the last build: same freshness rule as compiler fixes, accepted in the issue).
- [ ] **Step 5: regression tests (item 5).** `TestComplete_ModuleQualifiedRoute`: in another module importing `Todos`, `Todos.` offers the route constructors and `Todos.Row.` offers `load`/`view`/`Params`/`Session`. `TestHover_RouteGuards` on `testdata/corpus/partitions-ok`: a route with `requires user, org, role Admin | Mod, silo org` hovers with exactly that guard line (`guardText`). Expect PASS.
- [ ] **Step 6:** `go test ./internal/lsp -count=1` passes. One commit per step 1, 2, 3, 5, each `(#180)`.

### Task 6: tree-sitter and semtok (#180 tree-sitter items) — Sonnet

**Files:** `tree-sitter-alder/grammar.js:328`, generated `tree-sitter-alder/src/*`, `tree-sitter-alder/test/corpus/routes.txt`, `internal/semtok/decls_test.go:136-160`.

- [ ] **Step 1: no change for item 6** (namespace colour, decided).
- [ ] **Step 2: unused `$` (item 7).** `route_method: () => choice(…)`. Check `tree-sitter --version` is 0.27.1 (pinned in `scripts/ts.sh`); if not, stop and report. `scripts/ts.sh generate`, then `scripts/ts.sh test` and `go test ./tree-sitter-alder -count=1` (drift test) pass.
- [ ] **Step 3: corpus (item 9).** Add to `routes.txt` a route with `requires user, role Admin, silo org` and a scratch file (`in Todos` header) holding a `route` item plus a `> ` watch; write the expected trees with `tree-sitter test -u` and read them for sanity (a `route_decl` under `scratch_file`, a `silo_clause`). `scripts/ts.sh test` passes.
- [ ] **Step 4: semtok (item 8).** In `TestRouteDecls`, add cases for `Create.Params` in the signature type: `Create` as namespace, `Params` as type (confirm the kinds by running; they must match the definition-side tokens). `go test ./internal/semtok -run TestRouteDecls -count=1` passes.
- [ ] **Step 5:** Commits `tree-sitter: route_method takes no $; silo and scratch route corpus (#180)` and `semtok: test the route type in a member signature (#180)`.

### Task 7: effective method in every log line (#180 runtime items) — Opus

**Files:** `rt/web/router.go:283-296` (`line()`), `:601` (metrics), `rt/web/devrequests.go:246` (`loggedMethod`), `:450` (`maskURL`), tests `rt/web/method_test.go`, `rt/web/access_test.go`.

- [ ] **Step 1: Tests.** In `TestRequestLog_EffectiveMethod` (method_test.go:313) add: a `_method=DELETE` POST that panics and one that errors log `DELETE /x/…` in the error line (model `TestAccessLog_Panic`, access_test.go:166), and the metrics method label is `DELETE` (use the metrics test hook the router already has; find with `grep -n "Request(" rt/web/*_test.go`); a POST with `_method=PUT` to a path declared only for GET/DELETE answers 405 and logs `PUT`; a Datastar POST override that redirects to a GET route, followed by the router, logs the first request's effective method (model `boost_test.go:218` "redirect chain is followed"). Path masking in all of them stays as in `TestAccessLog_MasksOverriddenRoute`. Run; FAIL for the error line and metrics.
- [ ] **Step 2:** Use the tracker's `loggedMethod()` in `line()` and in the metrics call; keep `maskURL` getting the method it needs to mask correctly (it handles the override itself for POST; verify with the masking test).
- [ ] **Step 3:** `go test ./rt/web -run 'TestRequestLog|TestAccessLog|TestRouter_Method|TestRouter_405' -count=1` passes, and `go test ./rt/web -count=1`. Commit `rt/web: error lines and metrics say the effective method (#180)`.

### Task 8: mutation code minors (#193 items 1–4) — Sonnet

**Files:** `internal/mutate/mutate.go:1012-1047` (comments), `internal/generate/samples.go:13-28,127-171` (`testBuilder`, `login`, `logout`), `internal/generate/auth.go:355`, `internal/generate/admin.go:898-919` (`writeGate`), `internal/generate/admin_bulk.go:85-115,175,180,212`, `book/10-testing.md:2509`.

- [ ] **Step 1: comment (item 1).** In the doc comments of `answer` and `refresh` (mutate.go:1012-1017, 1037-1038), say that a mutant keeps the original route's `NeedsShown` (one binary, `internal/gen/routes.go:341-346`), so a `redirect → refresh` mutant of a route that never mentions refresh is patched under Datastar with no before-view; kills in the test browser are unaffected.
- [ ] **Step 2: `lastLogin` (item 2).** Add `lastLogin string` to `testBuilder`, set by `login`, `logout` and the direct login at auth.go:355; `writeGate` reads it instead of scanning `tb.lines`, and `relogged` compares `lastLogin` before and after `change(tb)` instead of string-matching the last line. Generated output is unchanged: `go test ./internal/generate -count=1` and `go test ./cmd/alder -run 'TestAdmin' -short -count=1` pass with no golden change.
- [ ] **Step 3: notice helper (item 3).** Constants `bulkDeletedText = "Deleted"`, `bulkUpdatedText = "Updated"` next to `deletedText` (admin.go:2757) and `bulkNotice(verb string, counts ...int) string` producing `Deleted 1 of 2` / `Updated 1`; `action(...)` at :102/:115 and the test lines at :175/:180/:212 use them. Generated output unchanged (same tests as Step 2).
- [ ] **Step 4: ch. 10 wording (item 4).** Row at book/10-testing.md:2509: "A route's first required field (`requires user`, say) is dropped: the route serves a visitor as `<Brand> "mutant"` (for a field that is a brand over `Text`; other fields are skipped); one mutant per route".
- [ ] **Step 5:** `go test ./internal/mutate ./internal/generate -count=1` passes. One commit per step, each `(#193)`.

### Task 9: re-measure the admin gates (#193 items 5–6) — Sonnet

**Files:** `cmd/alder/admin_test.go:171-216` (TestAdmin_Mutate doc comment and survivors), `:561-660` (TestAdmin_Columns).

- [ ] **Step 1:** Only if it will end before 9am New York on a weekday: run in the foreground `ALDER_MUTATION_GATES=1 go test ./cmd/alder -run '^(TestAdmin_Mutate|TestAdmin_Columns)$' -count=1 -timeout 40m -v`. Record each subtest's score and survivors.
- [ ] **Step 2:** Rewrite the survivors comment at admin_test.go:175-177 from the new run (no Search routes since #184). No change to the 0.95 floor or the mutators: item 5 is moot (no form write route is redirect-only in the admin now; a "form cannot fail to decode" suppression has nothing to win back), item 6 closes with the new scores in the comment. If a gate now fails, stop and report the survivors.
- [ ] **Step 3:** Commit `admin: survivors comment re-measured after #184 (#193)`; close #193 with the scores.

### Task 10: CLI help says routes (#195) — Sonnet

**Files:** `cmd/alder/helpjson.go:190` (`--bare`), `internal/generate/specs.go` (summaries and flag help for `new route --list/--show/--file`, `new form --path`, `new action --path`, `new admin`, `regen admin`, `regen --all`), `internal/generate/auth.go:97,107`, `editauth.go:22,25,29`, `session_edit.go:45,80` (refusal texts), `testdata/help.json`, test `cmd/alder/helpjson_test.go`.

- [ ] **Step 1: Test.** `TestHelpJSON_NoPageWords` in `helpjson_test.go`: walk every command, subcommand and flag summary in the help JSON and fail on a word matching `(?i)\b(pages?|messages?|msgs?)\b`. Allowed: none in summaries (flag names such as `--msg` are names, not summaries, and are not checked; the `alder new page is gone` refusal is not a summary). Run; FAIL listing the hits.
- [ ] **Step 2:** Rewrite the hits with route words: `--bare` "the app without its starter routes"; `--list` "a GET route listing the rows of table T"; `--show` "a GET route showing one row of table T"; `--file` "a file route: view returns File …"; `new form --path` "… else the route module's own path"; `new action --path` "the GET route's own path by default; required on App"; `new admin` "the admin routes for every table"; `regen admin` "the admin routes"; `regen --all` "recreate the admin routes the author deleted". Refusal texts: "is not a route of this app", "a login goes to a route without …", "goes to a route already", "--choose names the route that picks a `via` key". Update tests that quote these texts.
- [ ] **Step 3:** Regenerate `testdata/help.json`; `go test ./cmd/alder -run 'TestHelp' -count=1` and `go test ./internal/generate -count=1` pass. Commit `help: route words in every summary, with a test (#195)`.
- [ ] **Step 4:** Check `~/projects/alder.nvim` and `~/projects/alder-vscode*` do not match on any changed summary text (`grep -rn "starter pages\|admin pages" …`); if one does, open an issue there instead of editing it.

### Finish

- [ ] `scripts/test.sh` once, all green.
- [ ] Push main. Close #177, #180, #185, #192, #193, #195, each with its commits and its no-change reasons (#177 item 7 documented, #180 items 4 and 6, #193 items 5 and 6).
- [ ] In #52, tick item 3 ("Earlier PR follow-ups … done 2026-11-1x (<first>..<last>)") and add the date to the update line at the top.
- [ ] Reinstall per memory `alder-reinstall-after-merge`: `go install`, fast-forward `~/projects/alder.nvim`, and rebuild the parser (the grammar changed in Task 6).
