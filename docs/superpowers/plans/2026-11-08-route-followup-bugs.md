# #160 follow-ups: user-visible route bugs (#189, #179, #183, #173) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix the four user-visible bugs from roadmap #52 item 3. A refresh or redirect under
Datastar updates the `<html>` attributes (#189). A navigation keeps every toast (#179).
Generated route code no longer leaves holes and red tests (#183). `alder/plan` refuses to guess
a page (#173).

**Architecture:** There are four independent tasks, one per issue, and none uses another's code.
- #189: add `datastar.PatchHtmlAttrs`, a self-removing script like `Navigate` that syncs the
  attributes of `document.documentElement`. `patchRegions` and `morph` emit it.
- #179: the signed `alder_notice` cookie carries a list of toasts, and the tracker holds a list
  of alerts.
- #183: a form on a `--show` route fills its path param from `model.row`, and `new msg App`
  writes a comment instead of a red test.
- #173: `planReq` treats a required field that holds a guessed starting value as unset.

**Tech Stack:** Go (runtime `rt/web`, `rt/datastar`; generators `internal/generate`; LSP
`internal/lsp`), Alder goldens, Playwright browser tests.

**Spec:** GitHub issues tapegram/alder#189, #179, #183, #173 (`gh issue view N`). The user made
these rulings on 2026-11-08:
- #189: patch every `<html>` attribute in place, with no reload.
- #179: when the toasts don't fit in the cookie, keep the earlier ones whole and drop the later
  ones.
- #183: `new msg App` and `new route --method POST` agree by both writing a comment, not a test.

## Global Constraints

- Every task ends with `go build ./...` and `go vet ./...` clean, plus `go test` for the packages
  it touches. Before the PR, run the full `scripts/test.sh`.
- Goldens are rewritten with `go test ./cmd/alder -run TestGenerate -update`. Review the diff of
  every rewritten golden; don't accept it blind.
- The book changes in the same task as the behavior it describes. Run `go test ./book/...` when
  the book changes. The experiments agent-reference budget lint may need a raise, as in
  b743128d; do that only if CI says so.
- Comments and messages follow the house style: plain sentences, as in the surrounding code.
- Commit messages end with the session's attribution lines (the controller supplies them).

## Review Focus

- #189: a theme value with `"`, `<` or `</script>` in it must not break out of the script.
  `json.Marshal` escapes `<`; this is pinned in Task 1.
- #189: a page that has no `<html>` start tag (a fragment page) must send no attrs event, so it
  can't strip the document's attributes. Pinned in Task 1.
- #189: a refresh where only `<html>` changed must not log or answer "none". Pinned in Task 1.
- #179: a cookie signed before this change (`notice:<kind>:<text>` and `notice:<text>`) must
  still show after an upgrade. Pinned in Task 2.
- #173: the editors' preview must still open with a page filled in (issue 45). Only plan
  changes. Pinned in Task 4 by keeping `TestForm_RequiredFieldsStartFilled` green, unchanged.

---

### Task 1: #189 — sync `<html>` attributes on refresh and redirect under Datastar

**Files:**
- Modify: `rt/datastar/datastar.go`: add `PatchHtmlAttrs` next to `Navigate` (:103).
- Modify: `rt/web/router.go`:
  - add `htmlAttrs` next to `bodyOf` (:1810);
  - call it from `morph` (:1375).
- Modify: `rt/web/shown.go:175-241`, the `patchRegions` switch.
- Modify: `book/06-routes.md`:
  - drop the limitation note and the #189 link at :1978-1979;
  - check that the theme-switch prose at :501-576 and the patterns table at :1966 don't promise
    otherwise.
- Test: `rt/datastar/datastar_test.go`, `rt/web/fragments_test.go`, and a new
  `TestBrowser_ThemeSwitch` in `cmd/alder/patterns_browser_test.go`.

**Interfaces:**
- Produces: `func PatchHtmlAttrs(w http.ResponseWriter, attrs [][2]string) error`, in
  `rt/datastar`.
  - It appends `<script data-effect="el.remove()">…</script>` to `body`, the same way
    `Navigate` does. The script sets every `[name, value]` pair on `document.documentElement` and
    removes the attributes the list doesn't name.
  - `attrs` goes into the script as JSON (`json.Marshal`).
- Produces: `func htmlAttrs(page string) ([][2]string, bool)`, in `rt/web`.
  - It returns the attributes of the page's `<html …>` start tag, in source order, with entities
    decoded. `false` means the page has no `<html` start tag; use `startTag(page, "html")`.
  - A boolean attribute gets the value `""`.

- [ ] **Step 1: Write the failing tests**
  - `datastar_test.go` `TestPatchHtmlAttrs`: `[["class","dark"],["lang","en"]]` yields one
    `patch-elements` event with selector `body`, mode `append`, and a script that contains
    `"class","dark"`.
  - `TestPatchHtmlAttrs_Escapes`: a value of `</script><b>` must not appear literally in the
    event; `</script` does.
  - `router_test.go` (or the file next to `bodyOf`'s tests) `TestHtmlAttrs`:
    - `<html lang="en" class="a &amp; b" hidden>` gives `[[lang en] [class a & b] [hidden ""]]`
      and true;
    - a page with no `<html>` gives false;
    - `<htmlx>` doesn't count.
  - `fragments_test.go` `TestRerender_HtmlAttrsOnly`: a page whose `update` changes only the
    `<html class>` answers a refresh with the attrs event and no region event. The tracker names
    the patch `html`, not `none`.
  - `TestRerender_HtmlAttrsUnchanged`: no attrs event when the `<html>` attributes are equal.
  - `TestRedirect_HtmlAttrs`: a boosted redirect's events include the attrs event after the
    title.
- [ ] **Step 2: Run them and see them fail:** `go test ./rt/datastar ./rt/web -run 'HtmlAttrs'`.
- [ ] **Step 3: Implement `PatchHtmlAttrs` and `htmlAttrs`.**
- [ ] **Step 4: Emit the attrs event in `patchRegions`.** It comes after the body or region
  events and the shown event, and before `rt.notify`.
  - Send it when the after page has an `<html>` tag and either there is no before page or the
    attrs differ from the before page's.
  - In the `len(patches) == 0` case, still set the event-stream headers. Note `html` instead of
    `none` when the event is sent.
- [ ] **Step 5: Emit the attrs event in `morph`.** It comes after `PatchTitle`, whenever the page
  has an `<html>` tag, because the page being left is unknown and the script is idempotent. Add
  `html` to `tw.patches`.
- [ ] **Step 6: Run the tests and see them pass.**
- [ ] **Step 7: Add the browser test.** `TestBrowser_ThemeSwitch` uses the book's theme-switch
  app (book ch. 6 :501-576), set up the way the other `TestBrowser_*` tests in that file are.
  Press the switch and assert that `document.documentElement.classList.contains("dark")` turns
  true without a reload: a `window` marker set before the click survives. Press it again and
  assert the class is false.
- [ ] **Step 8: Update the book** (see Files), then run `go test ./book/...`.
- [ ] **Step 9: Commit:** `rt: sync <html> attributes on refresh and redirect under Datastar (#189)`.

### Task 2: #179 — every toast survives a navigation or a 303

**Files:**
- Modify: `rt/web/router.go`:
  - `storeNotice`/`takeNotice` (:1608-1645) become `storeNotices`/`takeNotices`;
  - the tracker field `alert`/`kind` (:203-206) becomes `alerts`;
  - the uses at :925 and :1205;
  - the alert rendering at :1430-1432.
- Modify: `rt/web/answer.go`: `setToasts` (:167-177) and `navigateAway` (:404-416). Also grep
  for every other `storeNotice`/`tw.alert` use, for example in `file.go`.
- Test: `rt/web/devpanel_test.go` (beside `TestNoticedWithoutScript`) and
  `rt/web/answer_test.go`.

**Interfaces:**
- Produces: `func (rt *router) storeNotices(w http.ResponseWriter, r *http.Request, toasts []rtpkg.Toast)`.
  - The payload is `notices:` plus a JSON array of `{"k": kind, "t": text}`, signed with
    `Sign`. The cookie attributes stay as they are (MaxAge 60, HttpOnly, Lax, Secure rule).
  - Toasts are kept in order while the signed value stays at or under **3800 bytes**; the rest
    are dropped.
  - When any are dropped, log at warn through `rt.logReq`: `dropped N toasts: the notice cookie
    is full`.
  - An empty list sets no cookie.
- Produces: `func (rt *router) takeNotices(w http.ResponseWriter, r *http.Request) []rtpkg.Toast`.
  - It clears the cookie and reads `notices:` JSON. It still reads the legacy
    `notice:<kind>:<text>` and `notice:<text>` payloads (one toast) with the existing kind rule.
  - It returns nil when the cookie is missing, unsigned or malformed. Kinds pass through
    `knownKind`.
- Tracker: `alerts []rtpkg.Toast` replaces `alert string`. `kind` stays only if `notice` still
  needs it.

- [ ] **Step 1: Write the failing tests.**
  - `TestNoticedWithoutScript_All`: a form posted without Datastar whose `update` answers three
    toasts (info, success, error) and a redirect. The next page shows three `<al-alert>`
    elements in order, with the right variants.
  - `TestNavigateAway_KeepsAllToasts`: a Datastar write that answers two toasts and a redirect
    to an external or unmatched URL (the `navigateAway` path, answer.go:211 or :293). The
    response sets a cookie that `takeNotices` reads as both toasts.
  - `TestTakeNotices_Legacy`: cookies signed as `notice:error:Boom` and `notice:hi` give one
    toast each: error "Boom" and info "hi".
  - `TestStoreNotices_Full`: 10 toasts of 1000 characters each. The stored cookie value is at
    most 3800 bytes, `takeNotices` returns the first k toasts whole (k ≥ 1), and the warn line is
    logged.
  - The existing notice tests (`TestNoticedWithoutScript*`, `TestNoticeOnlyForPageGET`,
    `TestBuild_StaleWrite`, `TestFile_KeepsNotice`) must stay green. Update their helpers
    (`noticeOf`) to the new API only.
- [ ] **Step 2: Run them and see them fail:** `go test ./rt/web -run 'Notice|NavigateAway'`.
- [ ] **Step 3: Implement.**
  - `setToasts` without Datastar sets `tw.alerts = toasts`.
  - `navigateAway` stores `notice` plus `more` as one list.
  - :1205 stores `tw.alerts`, and :925 sets `tw.alerts = rt.takeNotices(w, r)`.
  - :1430 renders one `<al-alert>` per alert, in order, with `noticeLook`'s variant and role
    each.
  - Update the doc comments that say "the first one".
- [ ] **Step 4: Run `go test ./rt/...` and see it pass.**
- [ ] **Step 5: Commit:** `rt: keep every toast across a navigation and a no-script redirect (#179)`.

### Task 3: #183 — generated route code passes `alder test` out of the box

**Files:**
- Modify: `internal/generate/route_form.go:137-153`, where the path-param `holes` are built.
- Modify: `internal/generate/appmsg.go:~82-103`, the test builder. Remove it and write the
  comment instead.
- Modify: `cmd/alder/generate_test.go`:
  - add a `form-route-show-test` case;
  - keep `app-msg` (:270) at `test: true` but drop its `pressing ToggleTheme` fill;
  - leave `fixture-app-msg-hole` (:274) as it is.
- Goldens: `testdata/generate/want/form-route-show`, `app-msg`, `app-msg-auth`,
  `fixture-app-msg-hole` and the new `form-route-show-test` (rewritten with `-update`).

**Interfaces:**
- Produces: `func rowKeyExpr(p *Program, page *Module, par *Param) (string, bool)` in
  `route_form.go`.
  - It returns `model.row.<field>` when the page's `Model` is `{ row: T }` (the shape that
    `new route --show T` writes), `T` is a table, and `par` has the type of `T`'s key field (its
    name is `<field>`).
  - Otherwise it returns false, and the existing `?<route><Param>` hole stays.
- The comment that `new msg App X` writes above its `update` matches the `route-post` golden's
  wording, adapted:

  ```
  -- Press X from a page that has its button to test it: `Test.get` that page, then
  -- `Test.click (App.X { … })`, and `r.answer` is Refreshed until `update` says otherwise.
  ```

  Without args it is `Test.click App.X`. Also fix the `newWriteRoute` doc comment
  (route_write.go:10-14), which says "the test says so in a comment" although there is no test.

- [ ] **Step 1: Write the failing golden case.** `form-route-show-test`: base `fixture`, pre
  `new route Posts.Show /posts --show Post`, then `new form Posts.Show Reply body:Text --msg
  SendReply`, with **no `--hole`** and `test: true`. Run `go test ./cmd/alder -run
  'TestGenerate/form-route-show-test'`; it should fail because the golden is missing, or
  `alder test` reports the hole.
- [ ] **Step 2: Implement `rowKeyExpr`** and use it in the path-param loop. `holes` gets
  `id: model.row.id` when it returns true. The test side (`pathValue`, :244-262) needs no change.
- [ ] **Step 3: Replace the test in `appmsg.go` with the comment.** Remove the `?<name>Page`
  page search when nothing else uses it.
- [ ] **Step 4: Rewrite the goldens** with `go test ./cmd/alder -run TestGenerate -update`.
  Check the diff:
  - `form-route-show` shows `SendReply { id: model.row.id }`, while its `--hole` holes for the
    form fields stay;
  - `app-msg` and the other app-msg goldens lose the test and gain the comment;
  - no other golden changes.
- [ ] **Step 5: Run the generator tests and see them pass:** `go test ./cmd/alder -run
  TestGenerate` and `go test ./internal/generate/...`.
- [ ] **Step 6: Commit:** `generate: fill a show route's form id from model.row; new msg App writes no red test (#183)`.

### Task 4: #173 — `alder/plan` refuses a guessed page

**Files:**
- Modify: `internal/lsp/generators.go`, `planReq` (:243), after `s.dryRun`.
- Test: `internal/lsp/generators_test.go`.

**Interfaces:**
- Consumes: `generate.FieldState` (`Source` is "default" for a start value; `Choices`; `Required`).
- Behaviour: plan refuses when a field is `Required`, has `Source == "default"`, has a non-empty
  `Choices`, and its `Value` is one of those choices.
  - Only the fields that `startValue` fills from choices match: page, table, decl, choice, and
    an Existing module.
  - The refusal is the same as for an invalid field: `<name>: required`, for example
    `page: required`. Use the path `planReq` already takes for a form that isn't ready.
  - Preview (`dryRun` and `alder/preview`) doesn't change.

- [ ] **Step 1: Write the failing tests.**
  - `TestPlan_NoPageStaleBuild`: blog example, `new msg` with only `name` set, no `page`, and a
    cursor in a file the last good build doesn't cover, or with no fresh `here`. Set this up the
    way the tests near :144/:167 do. The answer is the refusal with `page: required`, and no
    files.
  - `TestPlan_PageFromHere`: the same request with a fresh cursor context still plans. This
    keeps the :117 behaviour.
  - `TestPreview_StillStartsFilled`: `alder/preview` for the same request still shows `page` as
    `Admin.Comments.Bulk` with source `default`.
- [ ] **Step 2: Run them and see them fail:** `go test ./internal/lsp -run 'Plan_|Preview_StillStartsFilled'`.
- [ ] **Step 3: Implement the check in `planReq`.** Update its doc comment to say that a guessed
  starting value counts as unset.
- [ ] **Step 4: Run `go test ./internal/lsp ./internal/generate` and see it pass**, with
  `TestForm_RequiredFieldsStartFilled` unchanged.
- [ ] **Step 5: Commit:** `lsp: alder/plan refuses a required field left at its guessed start value (#173)`.

### Finish

- [ ] Run `go vet ./...` and the full `scripts/test.sh`, Playwright included.
- [ ] Open one PR for all four tasks. Its body says `Fixes #189, fixes #179, fixes #183, fixes
  #173`, and it ticks those four in roadmap #52 item 3 after the merge.
