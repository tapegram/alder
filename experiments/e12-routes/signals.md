# E12 signals: "reasonable but not Alder"

A **signal** is a choice a web developer would make and Alder does not, whether the checker accepts it or
refuses it (`experiments/README.md`, Method step 3; the spec's paragraph on signals). Signals are recorded here
and in `findings.md` §9 only. None enters the pass rate, the error taxonomy or the hidden score. A signal that
coincides with a check error is listed here, and the error is counted in `findings.md`.

**What was read:** all 131 rounds of the 78 cells. For each round, the files are the ones the model wrote: the
`--- file: … ---` blocks of `runs/<model>/<cell>/round<N>/output.md`, 934 files in all. The base files that T2
and T7 copy in are not included. The search was scripted, with `{- … -}` and `--` comments stripped before
matching. Every hit was then read in its file. The checker's answer comes from that round's `check.json`, and
the methods of the final write routes come from each cell's `summary.json` (`names.matches`). Book sentences are
cited by file and heading. Every sentence cited below is also in the prompt the models got
(`prompt-reference.md`), except one that is marked as missing from it.

## The signals found

Both are the same choice: a change that is not create, replace or remove, written as `POST` to a path that ends
in a verb.

| Cell | Round | What | The book sentence it fights | Checker's answer |
|---|---|---|---|---|
| *Haiku t6 s1* | r0, r1 (final) | `route Pay = POST "/orders/{id}/pay"` | `book/06-routes.md`, "`route`" › "The path" › "Paths name resources": "A path with a verb in it (`/replies/delete`) says the same thing twice, once in the method and once in the path"; "When a change is not one of the four, name the part of the thing that changes … `PUT /replies/{id}/archived`" | Accepted: no diagnostic on the line in either round. Names check `ok` |
| *Haiku t6 s1* | r0, r1 (final) | `route Cancel = POST "/orders/{id}/cancel"` | same | Accepted, as above |
| *Haiku t6 s1* | r0, r1 (final) | `route Toggle = POST "/products/{id}/toggle"` | same | Accepted in r1. In r0 the same line had E0027 on its `requires user, role Manager` (the session has no `role`). That error is about `requires`, not the method, and is counted in `findings.md` §3 |
| *Haiku t11 s1* | r0, r1 (final) | `route Open = POST "/orgs/{org}/open"` | same | Accepted: no diagnostic on the line. Names check `ok` |
| *Sonnet t11 s1* | r0 (final) | `route Open = POST "/orgs/{org}/open"` | same | Accepted, as above |

That is 5 routes in 3 cells, over 9 round occurrences. Each of them was still in the final round. Every one of
these cells passed and passed all its hidden tests. The hidden tests send routes as values, so a method never
changes their result. The book calls these paths "a convention that keeps URLs readable, and not a rule the
checker enforces", so no diagnostic was expected. The other cells gave the same routes a resource path instead: `PUT /orders/{id}/paid` and `PUT /orders/{id}/cancelled` (5 final cells each), `PATCH /products/{id}`
(5), `PUT /session/org` (3) and `PUT /orgs/{org}/opened` (1).

## The listed signals that never occurred (0 each)

| Signal | How it was searched | Result |
|---|---|---|
| A `DELETE` written as `POST` | Every write-route declaration in every round, 586 of them (`route X = POST\|PUT\|PATCH\|DELETE "…"`). For each, its `X.update` body (or the helper it calls) was checked for a `Db.delete…` call | 55 declarations delete a row: T4 and T5 `Delete`, T7 `Delete`, T9 `Cancel`, T10 `Remove`, T15 `Remove`. All 55 are `DELETE`. No route named `Delete`, `Remove`, `Cancel` or `Decline` is `POST` in any round |
| `render` where the rule says `refresh` (`book/06-routes.md`, "`Response`": "`render` is for something already on the page; `refresh` is for things that appear or disappear, or when more than one place changes") | Every `Response.render` in every round (7) | All 7 are T15's `Bump` answering `Response.render (Row { id: … })`, which the task text asks for: one row already on the page, changed in place |
| A trigger attribute on an `<al-*>` element (`book/06-routes.md`, "Patterns": "Custom elements (`<al-*>`) take none of the four attributes"; `book/05-html.md`, "Custom elements" › "What E0028 refuses", for `submit`) | Each of the 1,407 `<al-*>` open tags, attributes parsed up to the tag's `>`, then a second pass up to the next `<`, looking for `load`, `every`, `intersect`, `input` or `submit` | None. The trigger attributes are all on plain elements: `intersect=` on `<div>` (14), `load=` on `<span>` (12, all with `every=`) and on `<section>` (6), `input=` on `<input>` (11). The attributes on `<al-*>` were `field`, `label`, `type`, `variant`, `click`, `width`, `gap`, `size`, `value`, `href`, `class`, `id` and `help`. No E0028 or E0047 on any `<al-*>` trigger in any `check.json` |
| A `Response` built by hand (`book/06-routes.md`, "`Response`": "The other `Response` builders a web framework has (`status`, `header`, `json`, `file`, `remove`, `patch`, `stream`) are not in Alder yet") | Every `Response` in every round. Also searched: `Response.status`/`header`/`json`/`file`/`ok`/`notFound` and similar, a record built as a `Response`, and the page-era constructors (`Redirect (…)`, `Rerender`) | None. The only builders used are the book's: `redirect` 595, `invalid` 161, `refresh` 62, `toast` 11, `error` 11, `render` 7, `success` 6 (comments excluded) |
| A path parameter in an `input=` route (`book/06-routes.md`, "Patterns" › "Search as you type": "A path parameter does not count: the browser can set a query key, but it never rewrites a path") | Every `input={…}` (11, all T12, one per round), with the route it names looked up in the same round | All 11 name `Results = GET "/contacts/results" { q: Text = "" }`, a query parameter with no path segment |

No round's `check.json` has E0045, E0046 or E0047 (`findings.md` §5). So none of the coincidences the spec
anticipates (E0045 on a timer that writes) happened.

## The write routes' methods, as the names check reports them

These are the final rounds of the 66 cells that have a Names block (T2 and T3 have none). The number is how many
cells chose each method and path. The task texts name these routes without a method, except where noted.

| Task | Route | Method and path (cells) |
|---|---|---|
| T4 | `Add`, `Save`, `Delete` | `POST /notes` (6), `PUT /notes/{id}` (6), `DELETE /notes/{id}` (6) |
| T4 | `SwitchTheme` | `POST /theme` (4), `PATCH /theme` (1), `PUT /theme` (1) |
| T5 | `Add`, `Delete` | `POST /notes` (6), `DELETE /notes/{id}` (6) |
| T6 | `Create`, `Add`, `AddLine` | `POST /orders` (6), `POST /products` (6), `POST /orders/{id}/lines` (6) |
| T6 | `Pay` | `PUT /orders/{id}/paid` (5), **`POST /orders/{id}/pay` (1)** |
| T6 | `Cancel` | `PUT /orders/{id}/cancelled` (5), **`POST /orders/{id}/cancel` (1)** |
| T6 | `Toggle` | `PATCH /products/{id}` (5), **`POST /products/{id}/toggle` (1)** |
| T7 | `Publish` | `PUT /admin/{id}/published` (6). `Create` and `Search` are the base's |
| T9 | `Book`, `Cancel` | `POST /rooms/{id}/bookings` (5), `POST /rooms/{id}` (1); `DELETE /bookings/{id}` (6) |
| T10 | `Create`, `SendInvite`, `Remove` | `POST /teams` (6), `POST /teams/{id}/invites` (6), `DELETE /teams/{id}/members/{user}` (6) |
| T10 | `Accept`, `Decline` | `PUT /invites/{id}/accepted` (5), `PUT /invites/{id}` (1); `PUT /invites/{id}/declined` (5), `DELETE /invites/{id}` (1) |
| T11 | `Create` ×2, `Add`, `Toggle` | `POST /orgs` (6), `POST /projects` (6), `POST /projects/{id}/todos` (6), `PATCH /projects/{id}/todos/{todo}` (6) |
| T11 | `Open` | `PUT /session/org` (3), **`POST /orgs/{org}/open` (2)**, `PUT /orgs/{org}/opened` (1) |
| T12, T13 | `Add`, `Post` | `POST /contacts` (6), `POST /feed` (6) |
| T14 | `Close` | `PUT /board/tickets/{id}/closed` (3), `PATCH /tickets/{id}` (2), `PUT /tickets/{id}/closed` (1) |
| T15 | `Rename`, `Remove` | `PUT /pantry/items/{id}` (5), `PUT /pantry/items/{id}/name` (1); `DELETE /pantry/items/{id}` (6) |
| T15 | `Bump` | `PATCH /pantry/items/{id}` (3), `POST /pantry/items/{id}/quantity` (2), `PATCH /pantry/items/{id}/quantity` (1) |
| every task with a login | `Logout`, `Submit` | `DELETE /session` and `POST /session` in every cell |

Names check: `ok` in all 66. The rows in bold are the signals above. No other method was counted as a signal,
for these reasons:

- **`POST /theme` (T4: *Haiku t4 s2*, *Opus t4 s1*, *Sonnet t4 s1*, *s2*; 9 rounds) follows the prompt.**
  `book/06-routes.md`, "What the patterns cost", has the row "login, logout, theme | a `POST` that runs
  `Session.put`". The same chapter's own `Logout` is `DELETE "/session"`, and its theme example
  ("App-level routes" › "Example: a theme switch", `route SwitchTheme = PUT "/theme" { }`) is not in the
  reference prompt. So the book contradicts itself, and the models followed its table. This is a candidate
  for the issue list (§10), not a signal.
- **`POST /pantry/items/{id}/quantity` (T15 `Bump`, both Opus cells)** adds one each time it is sent. That is not
  idempotent, so `POST` is the right method in plain HTTP, and the book has no sentence about it.
- **`DELETE /invites/{id}` for `Decline` (*Haiku t10 s1*, r0 and r1)** marks the invite `Declined` and keeps the
  row. A web developer would not usually do this, but it is not a `POST` habit either.
- **`POST /rooms/{id}` for `Book` (*Sonnet t9 s1*, all 4 rounds)** posts to the room's own path. Chapter 6
  allows exactly that ("Paths name resources": "a write route takes that path").

## Seen, not counted

- **The opposite of the listed `render`/`refresh` signal.** T3's publish toggle (`Toggle` or `Publish`, all 10
  T3 rounds) answers `Response.refresh`. The patterns table (`book/06-routes.md`, "What the patterns cost") writes "toggle a row | `Toggle = PATCH` answering
  `render (Row { id })`". The example app's own `Toggle` also answers `refresh`. `refresh` is correct, only
  dearer, so this is not a "web developer" choice.
- **A hand-written `<form method="get" action="/contacts">`** around the search field (*Opus t12 s1* r0). The
  prompt allows it (`book/06-routes.md`, "Parameters" › "Query keys: `as "Key"`": "Hand-written GET forms name
  their inputs by the key"), and the check was clean.
