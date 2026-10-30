# Query.redirect: implementation plan (#8)

Spec: `docs/superpowers/specs/2026-10-29-query-redirect-design.md` (binding). Each task is
test-first: write the failing test, run it red, write the code, run it green, commit.

## Task 1: runtime node (`rt/eff`)

- Tests in `rt/eff/eff_test.go`:
  - `TestRun_RedirectShortCircuits`: `Bind(Redirect[int]("/b?seed=7"), f)` returns
    `*ErrRedirect{Location: "/b?seed=7"}`, `Error()` is `"redirected to /b?seed=7"`, `f` not called.
  - `TestRun_RollbackOnCallErrorAndNotFound` gains a case: a write Call then `Redirect` rolls back.
  - `TestCapError`: an `*ErrRedirect` passes through unwrapped.
- Code in `rt/eff/eff.go`: node `redirect{location string}`, `Redirect[T]`, `ErrRedirect`,
  `CapError` passes it; `rt/eff/run.go` `interpret`: `case redirect`.

## Task 2: `Test.run` sees it (`rt/testing`)

- Test `TestInner_RedirectMessageAndSessionRestore` in `rt/testing/testing_test.go`: a Task that
  `Session.put`s then redirects gives `ok=false`, msg `"redirected to /b"`, session restored.
- Code: `rt/testing/testing.go` `inner`: `errors.As(rerr, &rd)` → `rd.Error()`; doc comment.

## Task 3: signature, IR, gen

- `stdlib/Query.alder`: `redirect : Page -> Query a` with its comment.
- `internal/ir`: `EffRedirect{Typed; Page Expr}`, walk child, dump `(redirect <page>)`,
  `lower_eff.go` `"Query.redirect"`.
- `internal/gen`: `effects.go` (`eff.Redirect[T](rt.PageURL(page))`, isEffNode, doc table),
  `expr.go` effect-node list, `routes.go` `route__fail` (both variants) answers
  `*eff.ErrRedirect` with `web.RedirectResponse(rd.Location, nil)`; route comments.
- Tests in `internal/gen/gen_test.go`:
  - `TestGen_QueryRedirect`: a page whose load is `Query.redirect (M { id: x })` emits
    `eff.Redirect[` and `rt.PageURL(`; the IR dump shows `(redirect`; `Query.redirect` passed as
    a value (`Maybe.map Query.redirect`) lowers through a Lambda and compiles.
  - `TestGen_QueryRedirectServes`: built app, GET answers 303 with Location `/b?seed=7`; under
    Datastar a page-of-this-app target is followed (history push, 200 SSE); a file page's
    redirect is a plain 303; `Url.external "javascript:…"` is a 500 under Datastar, as
    `Redirect` is.

## Task 4: mutation testing

- `internal/mutate/mutate_test.go` `TestMutate_RedirectGuard`: `if ok then Query.redirect p else
  Query.succeed m` gives one "guard dropped" mutant, "the redirect arm dropped", and `show`
  prints `Query.redirect …`.
- Code: `isGuard` → `"redirect"`; `show.go` `EffRedirect` → `"Query.redirect " + atom(page)`.

## Task 5: corpus

- `testdata/corpus/query-redirect-ok/` (no diagnostics): App (session with `user`, login),
  `Start` (query params, redirects to `Builder` with a path parameter), `Builder`, a file page
  `Go` redirecting, `Out` redirecting to `Url.external`, `Landing` (`requires user`, redirects to
  `Home`), `Gone` (redirects to a page that 404s), `Ping`/`Pong` (a loop), an App/page `update`
  arm binding `Query.redirect` after a `Db.insert`. Tests in the modules cover the spec's seven
  end-to-end behaviours.
- `testdata/corpus/E0003-query-redirect-text/`: `Query.redirect "/x"` → E0003.

## Task 6: end-to-end (`cmd/alder`)

- `TestTest_QueryRedirect` in `cmd/alder/test_test.go`: `alder test` on a copy of
  `query-redirect-ok` passes all; then an appended failing `Test.get` of the redirecting page
  reports `GET /start…: redirected to /builder/7…`.

## Task 7: book

- `07-effects.md`: function list, "A `Query` cannot fail" → can stop, Go node list, the
  inside-a-Task sentence pointing at `Redirect p`.
- `06-pages.md`: GET lifecycle step 3, "What stays HTML", "Failure pages".
- `09-stdlib.md`: the Query paragraph only. `10-testing.md`: `Answer` table's `Redirected p`
  row and the `Test.fetch` section (a 303 shown once).
- `python3 book/site/build.py`; `cd experiments && python3 -m unittest` (raise the token cap
  with an issue comment if needed).

## Task 8: verify and ship

`go vet ./...`, `go test ./...`, rebase on origin/main, push, PR.
