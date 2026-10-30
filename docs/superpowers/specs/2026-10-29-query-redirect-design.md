# Query.redirect: a redirect from a page's `load`

Design for [tapegram/alder#8](https://github.com/tapegram/alder/issues/8). Found while building
nightvote-replay: `/start?go=builder` was to mint a value and send the browser to the builder page
with it, but only `update` can redirect (`Redirect` is a `Nav`), and `/start` is a file page, which
has no `update`. The feature was dropped. Everything marked **(call)** is the author's decision, not
the user's.

## Intent

A page's `load` can already stop a request with `Query.notFound` (a 404). It gains one more way to
stop: `Query.redirect p` answers a 303 to the page value `p`, so a GET can send the browser on,
with the target checked by the compiler as every `Redirect` and `href` is. It mirrors
`Query.notFound` in every layer: typed from its signature line, lowered to an IR node, run as an
effect that ends the `Query`, answered by the same `route__fail`, and seen by a test's browser as
the `Redirected` it already knows.

## Goals

1. `Query.redirect : Page -> Query a` works in any `load`, of an HTML page or a file page, with
   any page value: path and query parameters rendered as a link's are, `Url.external` allowed.
2. The redirect behaves as `Redirect p` from `update` does: a plain 303, or under Datastar the
   target morphed in with a history push (another site: a navigation event).
3. Tests see it without new vocabulary: `Test.open` answers `Redirected p`, `Test.fetch` shows the
   303 and its `Location`.
4. Existing programs compile and behave identically.

## Non-goals

- `Task.redirect`. A `Task` redirects by answering `Redirect p` (chapter 6); that path commits
  the Task's writes, and a second way to do the same would need a rule for which wins.
- A `Query.redirect` that carries a notice (`Notice.with`) or a status other than 303.
- Letting `Test.get`/`Test.open` name a file page (still E0003 "use `Test.fetch`").
- Scaffold changes in `internal/generate`.

## Anti-goals

- A redirect that is not a `Page`: no `Query.redirectTo : Text -> Query a`. The whole point of #8
  is that the target stays checked; `Url.external` already covers a text URL.
- A new `Answer` constructor or a new failure text in the test report where `Redirected` and
  "redirected to" already exist.

## The API

The line to add to `stdlib/Query.alder`, after `notFound`:

```alder
-- stop: answer 303 to a page (the same page with other parameters, another page, Url.external)
redirect : Page -> Query a
```

No row in `internal/gen/stdcall.go`: like `Query.notFound` it is lowered to an IR node, because
its result type is polymorphic (`Query a`) and gen must write the Go type parameter.

### Semantics

- **It ends the `Query`.** Nothing after it runs; `view` is not called and the page's model is
  never built. Under a `let x <- …` chain it is a failure of the chain, as `Query.notFound` is.
- **Status 303 See Other** **(call)**. Every Alder redirect is a 303 today (`web.RedirectResponse`,
  the `requires` login redirect, `Redirect p`), the router's Datastar follow and the test
  browser only recognise 303, and for a GET the browser does the same thing with 302, 303 and 307.
  One code keeps one path. (A 303 is never cached, which is right for a `load` that depends on
  the clock, the session or the query.)
- **The target URL** is `rt.PageURL(p)`, exactly what `href={p}` and `Redirect p` render: path
  parameters escaped, query parameters at their default left out, `List` parameters repeated.
  `Query.redirect (Builder { seed: "7" })` answers `Location: /builder?seed=7`.
- **`Url.external`** is allowed and gets `Redirect`'s safety: an `href` that `Url.external` turned
  into `#` is answered with a 500 (`refusing to navigate to …`) by the router's existing check,
  never sent.
- **Datastar.** A boosted link to a page whose `load` redirects is answered as a `requires`
  redirect is today: a page of this app is served in-process and morphed in with a history push
  (`router.follow`, the same 10-hop limit, "more than 10 redirects in a row"); another site is a
  navigation event. A file page under Datastar is already a real navigation before its `load`
  runs, so its redirect is always a plain 303.
- **Session.** A `load` cannot change the session (it is a `Query`), so the redirect carries
  none: the cookie is left as it was. The generated page function already sets `r.Session = keep`
  on `route__fail`'s answer; `keep` is `nil` for a page's GET.
- **`requires` and `role`** run before `load`, unchanged: on a page with `requires user`, a
  visitor is sent to `App.login` and never reaches the `Query.redirect`; a signed-in user whose
  role is refused gets the 404. After login, `keep` brings the user back to the page, whose
  `load` then redirects — the right order for a "landing" page that forwards signed-in users.
- **Inside a `Task`** **(call)**. A `Query` may be bound in a `Task` block (chapter 7), so
  `let _ <- Query.redirect p` compiles in `update`. It is a failure of the Task: writes and
  `Session.put` changes are undone, like `Task.notFound`, and the request is then answered with
  the 303 (by `route__fail`, through the same Datastar follow). The book says so in one sentence
  and points at `Redirect p` for "write, then go". No checker rule: there is no "Query-only
  context" today, and `Task.notFound` has the same shape.
- **In `Test.run`** **(call)**: a Task that reaches a redirect gives
  `Err "redirected to /builder?seed=7"` (the URL as `Redirected` prints it), the savepoint rolled
  back and the session restored, as `Err "not found"` is for `notFound`.
- **A capability's fake** that runs an effect which redirects: `eff.CapError` passes
  `*eff.ErrRedirect` through unwrapped, as it does `ErrNotFound`.
- **Trace** **(call)**: no entry, as `Query.notFound` adds none (only Calls are traced); the dev
  panel's answer line already reads `redirect /builder?seed=7` because the answer goes through
  `router.write`. The log line is the router's usual one for a 303.
- **Mutation testing** **(call)**: `Query.redirect p` is a guard, like `notFound` and `Task.fail`,
  so `if ok then Query.redirect p else …` gets the existing "guard dropped" operator
  ("the redirect arm dropped"); no "redirect → notFound" operator.

### What a test sees

| step | on a page whose `load` redirects to `p` |
|---|---|
| `Test.open page` | `Redirected p`; the browser then holds `p` (as after a `Redirect`), or `PageMissing` / `RequestFailed` when `p` itself fails; a loop is `RequestFailed "more than 10 redirects in a row (the last to …)"` |
| `Test.get page` | the test fails: `GET /start: redirected to /builder?seed=7` (the existing `NotShownError`) |
| `Test.fetch page` | `{ status: 303, contentType: "", body: <empty>, headers: [… { name: "Location", value: "/builder?seed=7" } …] }`, not followed |
| `Test.run t` | `{ value: Err "redirected to /builder?seed=7", trace: … }` |

`Test.fetch` is the only step for a file page (unchanged), so nightvote-replay's `/start` test
compares `r.status == 303` and finds `Location` in `r.headers`. The `Redirected` page compares by
URL (`AlderEqual`), so `r.answer == Redirected (Builder { seed: "7" })` works whichever way the
parameters were written.

## Where things change

| layer | files | change |
|---|---|---|
| signature | `stdlib/Query.alder` | the two lines above; the checker and the LSP's completion read them |
| IR | `internal/ir/ir.go` `EffRedirect{Typed; Page Expr}` next to `EffNotFound`; `walk.go` children (`x.Page`); `dump.go` `(redirect <page>)`; `lower_eff.go` case `"Query.redirect"` → `&EffRedirect{…, Page: args[0]}` (used as a value it is wrapped in a Lambda like `Task.fail`) |
| gen | `internal/gen/effects.go`: `eff.Redirect[T](rt.PageURL(<page>))`; `expr.go` line ~305 effect-node list; `routes.go` `route__fail` (both variants): `var rd *eff.ErrRedirect; if errors.As(err, &rd) { return web.RedirectResponse(rd.Location, nil) }` before the 404 case, and the route comments naming "Query.notFound" gain "or a 303 for Query.redirect" |
| runtime | `rt/eff/eff.go`: node `redirect{location string}`, `func Redirect[T any](location string) Task[T]`, `type ErrRedirect struct{ Location string }` (`Error()` is `"redirected to " + Location`), `CapError` passes it through; `rt/eff/run.go` `interpret`: `case redirect: return nil, &ErrRedirect{…}` (a Task's transaction is rolled back as for `notFound`) |
| tests runtime | `rt/testing/testing.go` `inner`: `errors.As(rerr, &rd)` → msg `rd.Error()`, session restored. `browser.go` needs no change: `navigate`, `post` and `Fetch` already handle a 303 from any route |
| mutation | `internal/mutate/mutate.go` `isGuard` → `"redirect"`; `show.go` prints `Query.redirect <page>` |
| book | `07-effects.md`: the function list (`Query.redirect : Page -> Query a  -- stop: respond 303 to a page`), "A `Query` cannot fail" → it cannot fail but can stop (`notFound`, `redirect`), the Go node list gains `redirect`, and the "inside a Task" sentence; `06-pages.md`: the GET lifecycle (step 3: "404 on `Query.notFound`, 303 on `Query.redirect`"), "What stays HTML" (a file page's redirect is a 303 like its 404), "Failure pages"; `09-stdlib.md`: the Query paragraph; `10-testing.md`: the `Answer` table's `Redirected p` row and the `Test.fetch` section (a 303 not followed, `Location` in `headers`) |
| site | `book/site/build.py` after the book edits |
| corpus | `testdata/corpus/query-redirect-ok/` (a page and a file page using it; no diagnostics expected) |

Nothing changes in the parser, formatter, tree-sitter grammar, LSP or `cmd/alder`: a stdlib
function is a name like any other. The user reinstalls (`go install ./cmd/alder`) after merge so
the LSP completes the new name.

## Tests the builder adds, by behaviour

- `rt/eff`: a `redirect` node ends the run with `*ErrRedirect` carrying the location and runs
  nothing after it; in a Task it rolls the transaction back (next to
  `TestRun_NotFoundShortCircuits` / `TestRun_RollbackOnCallErrorAndNotFound`); `CapError` returns
  an `ErrRedirect` unwrapped.
- `rt/testing`: `Inner` turns a redirect into `"redirected to <url>"` with the session restored
  (next to `TestInner_NotFoundMessageAndSessionRestore`).
- `internal/ir` or `internal/gen` golden: `Query.redirect (M { id: x })` lowers to `EffRedirect`
  and emits `eff.Redirect[…](rt.PageURL(…))`; `Query.redirect` passed as a function value lowers
  through a Lambda.
- `internal/check` corpus `query-redirect-ok`: a page with query parameters redirecting to
  another with a path parameter, a file page redirecting, `Url.external` as the target; and the
  negative `E0003`: `Query.redirect "/x"` (a `Text` is not a `Page`).
- `cmd/alder` end-to-end (`TestTest_Browser` / `TestTest_FilePages` style): an Alder test suite
  with (1) `Test.open` of a redirecting page answering `Redirected` and holding the target;
  (2) `Test.get` of it failing with "redirected to"; (3) `Test.fetch` of a redirecting file page
  giving status 303 and the `Location` header; (4) a `requires user` page that redirects after
  login: a visitor gets `Redirected (Auth.Login {})`, a user `Redirected (Home {})`; (5) a redirect
  to a page that answers 404 is `PageMissing`; (6) two pages redirecting to each other give
  `RequestFailed` naming the limit; (7) `Test.run` of an update arm that binds `Query.redirect`
  gives `Err "redirected to …"` and leaves no row written.
- `rt/web` router: a GET route answering `RedirectResponse` under Datastar is followed and
  morphed with a history push (if not already covered by the `requires` tests, which take the
  same path, the builder adds one for a page route with a query-string target).
- `internal/mutate`: `if ok then Query.redirect p else Query.succeed m` yields one "guard
  dropped" mutant and `show` prints `Query.redirect …`.

## Left to the builder

- Whether `ErrRedirect`'s `Error()` text or a separate field is what `inner` reports; the Alder
  text must be `redirected to <url>` either way.
- The exact book sentences; chapter 10 should show `Test.fetch`'s 303 once, as it shows a file's
  200 today.

## Verification

- `go test ./...` green, including the new corpus directory and the end-to-end suite.
- A scratch check (not a change to nightvote-replay, which pins its own build): a copy of
  `app/Start.alder` with `go: Text = ""` and, when `go == "builder"` and the key admits, `Query.redirect
  (Index { key: params.key, seed: params.seed, faults: params.faults })` before the JSON branch;
  its test `Test.fetch (Start { key: "test", seed: "7", go: "builder" })` sees `status == 303`
  and a `Location` of `/?seed=7&key=test` (the Index page's declaration order; `faults` at its
  default is left out); `alder run` in a browser lands on the builder.
