# M17: File answers — pages that answer any body, any status; Bytes; delays; Time arithmetic — Design

Date: 2026-10-20. Status: drafted from the replay-sites brainstorm
(`~/projects/nightvote-replay`, spec `docs/superpowers/specs/2026-10-20-replay-sites-design.md`),
the first app that needs it. Related: M3c (the router), M5e (browser tests), M7c
(boosted navigation), M16 (the Render target). Branch `m17-file-answers`.

## 1. Goal

An Alder page can answer something other than an HTML document: a text file, a JSON
body, a CSV, a zip, a PDF, with its own content type, status code and a few headers,
written verbatim — no doctype, no injected head tags, no dev panel. Today every page
answer is `"<!doctype html>\n" + page` with the Datastar script, the boost meta, the
build meta and the pending style spliced into `<head>` (`rt/web/router.go:876-882,
1029-1036, 1079-1100`), `Content-Type: text/html`, status 200/303/404/500/503 only
(`rt/web/request.go:43-52`). No Alder type carries bytes.

Three general needs ride along, each a built-in in the stdlib rather than an adapter,
in the spirit of `Time.now`, `Random.int` and `Http.get` (`rt/std/effects.go`): a
`Bytes` type the runtime serves and capabilities return; a delay effect the test
environment stubs as it stubs the clock; and `Time` arithmetic (today `now, epoch,
toDate, format, ago` only, `stdlib/Time.alder`: nothing turns an instant into a number).

Success: the replay app serves `allracesbyprecinct.txt` as `text/plain`, a Clarity
`detailxml.zip` as `application/zip` from a capability returning `Bytes`, a
LiveVoterTurnout page as byte-exact HTML written in Alder markup, a 503 on demand and a
20-second answer with `Query.delay`; `alder test` checks all five with `Test.fetch` in
milliseconds (the delay is virtual); the book's chapter 6 has a section "File pages";
the reference prompt stays under its cap.

## 2. Decisions recorded (2026-10-20)

- **A page, not a new keyword.** A file page is an ordinary `page` whose `view` returns
  `File` instead of `Html Msg`. Same `load`, same `Params`, same `requires`, same links
  (`href={Report.Csv { id }}` is a checked `Page` value). The checker tells the two
  apart by `view`'s declared type, as it already tells `Html Never` from `Html Msg`
  (`internal/check/pages.go:54`).
- **No messages on a file page.** `msg`/`form` in a file page module is an error
  (E0034): there is no document to post from, and no `update`.
- **`Bytes` is built in** (user direction: built-in where the need is general). An
  opaque Prelude type, built by `Text.toBytes` or returned by a capability (Go
  `[]byte`), served by `File.bytes`, measured by `Bytes.length`. No byte-level
  operations in Alder: encoders (zip, xlsx, PDF) stay in Go adapters, which is what
  capabilities are for.
- **A delay is a built-in effect**, `Query.delay`/`Task.sleep`, run through `Env` like
  the clock, so a test advances its ticking clock instead of waiting. *Proposed, user
  to confirm.*
- **Statuses are the file's.** `File.withStatus 503 f` is served as 503 with the body
  given, never `App.error`. `Query.notFound` in a file page's `load` is still the app's
  404 page (`App.notFound`, HTML), and a failing capability is still the 500 page: an
  HTML error where a file was expected is what real sites do.
- **Headers by allow-list.** `File.withHeader` sets `Content-Disposition`,
  `Cache-Control`, `ETag`, `Last-Modified`, `Retry-After`, `Content-Language` and any
  `X-…` name; anything else (`Set-Cookie`, `Location`, `Content-Length`,
  `Transfer-Encoding`, `Content-Type`, hop-by-hop) is E0035 when the name is a
  literal, and dropped with a log line at run time otherwise.
- **Milliseconds** for the `Time` numbers (Elm's `posixToMillis`; the runtime keeps
  `time.Time`, finer, and `fromMillis (toMillis t)` is exact on what `toMillis` gives).
- **`@key` is M17b, not M17.** A URL name a parameter cannot spell (`?ElectionId=`,
  `?f=json`) touches the parser (an attribute on a param field), the checker (name
  clashes, the `_p.` message encoding), `PageURL`/link generation, decoding, `alder
  new page --param`, `edit page add-param`, the tree-sitter grammar and the LSP hover:
  six places for one feature, so a milestone of its own, needed when the replay app
  reaches Wood, ArcGIS and TotalVote. Sketch to decide then: `{ electionId: Int @key
  "ElectionId" }`, or case-insensitive query matching plus a `@key` only for names
  that are not Alder names (`f`, `where` are fine; `ElectionId` is a case).

## 3. The language

### 3.1 `File` and `Bytes`

`type File` and `type Bytes` join `Prelude.alder`'s opaque types.

```alder
module File

text        : { contentType: Text, body: Text } -> File      -- 200
bytes       : { contentType: Text, body: Bytes } -> File     -- 200
html        : Html Never -> File                             -- text/html; charset=utf-8, rendered verbatim, 200
json        : Text -> File                                   -- application/json, 200
withStatus  : Int -> File -> File                            -- 100..599; a literal outside is E0003, at run time 500
withHeader  : { name: Text, value: Text } -> File -> File    -- the allow-list in §2
status      : File -> Int
contentType : File -> Text
body        : File -> Bytes
```

```alder
module Bytes

length : Bytes -> Int
toText : Bytes -> Maybe Text      -- Just when valid UTF-8
```

and `Text.toBytes : Text -> Bytes`. Nothing else: no indexing, slicing or
concatenation of bytes in Alder (non-goal; an adapter does that).

`File.html` renders the node as `view` would (`rt/html`) and writes it as-is: no
`<!doctype html>` unless the node's text has one (`Html.unsafe "<!doctype html>"` is
the way), no head tags, no panel. This is how a page that must match another site byte
for byte is written: in checked Alder markup.

### 3.2 A file page

```alder
module Report.Csv

page "/reports/{id}.csv" { id: Report.ReportId }

type Model = { rows: List Report.Row }

load : Params -> Session -> Query Model
load params session = { … }

view : Model -> Session -> File
view model session =
  File.text { contentType: "text/csv", body: Report.toCsv model.rows }
    |> File.withHeader { name: "Content-Disposition", value: "attachment; filename=report.csv" }
```

Rules (`internal/check/pages.go`):

- `view : Model -> Session -> File` is accepted as the page's view type; the E0003
  message for a wrong `view` names both forms (`Html Msg`, or `File` for a file page).
- A file page module declaring a `msg`, a `form` or an `update` is **E0034** "a file
  page has no messages: `view` returns `File`, so nothing can be posted from it; move
  the message to an HTML page".
- `File.withStatus` with a literal outside 100..599 is E0003 with the range in the
  text; `File.withHeader` with a literal name outside the allow-list is **E0035**
  "`Set-Cookie` cannot be set from a page; the headers a file may set are …".
- A link to a file page renders with `data-alder-reload` (chapter 6 "What the boost
  leaves to the browser"): a file is not a document to morph in. `Redirect (Report.Csv
  { id })` is answered under Datastar with the navigate event, the path `followable`
  takes for `/static/` today (`rt/web/router.go:790`).
- `requires` works as on any page: a failed `requires` redirects to `App.login`.
- `load` runs under the request lifecycle as on any page (session decoded, effect
  trace recorded, `Query` only).

### 3.3 Capabilities returning `Bytes` or `File`

```alder
capability Zip {
  detail : { election: Text, precincts: List Precinct } -> Query Bytes,
}
```

Generated Go maps `Bytes` to `[]byte` and `File` to `rt.File`:

```go
type File struct {
	Status      int        // 0 → 200
	ContentType string     // "" → application/octet-stream
	Headers     [][2]string
	Body        []byte
}
```

A `fake` for such a capability builds its value with `Text.toBytes` or `File.text`.
Adapters that encode zip, xlsx and PDF return `Bytes`; the page wraps them with
`File.bytes { contentType: "application/zip", body }`.

### 3.4 `Query.delay` and `Task.sleep` (proposed)

```alder
Query.delay : Int -> Query Unit      -- milliseconds; 0..60000, clamped
Task.sleep  : Int -> Task Unit
```

Built on `eff.Call("Query.delay", …)` in `rt/std/effects.go`, which calls
`e.Sleep(d)` — a new `Env.Sleep func(time.Duration)` beside `Env.Now`, nil meaning
`time.Sleep` bounded by `e.Ctx` (a cancelled request stops the wait). The trace line is
`Query.delay 20000ms`. The test environment (`rt/testing/testing.go:386`, where `Now:
Ticking(now)` is set) sets `Sleep` to advance the ticking clock by `d` and return at
once, so a test of a slow page runs in milliseconds and `Time.now` read after the delay
shows the time passed. `alder eval`/`repl` sleep for real. A delay before the first
database step holds no transaction (chapter 7's rule for capabilities and HTTP applies
unchanged); a delay inside a `Task` after a write holds its transaction, and the book
says not to.

### 3.5 `Time`

```alder
toMillis   : Time -> Int            -- milliseconds since Time.epoch (UTC)
fromMillis : Int -> Time
addMillis  : Int -> Time -> Time
diffMillis : Time -> Time -> Int    -- Time.diffMillis a b = b − a, like Date.diffDays
```

Pure, in `rt/std/date.go` beside `TimeEpoch`. `Time.toMillis Time.epoch == 0`.

Also the `Float` gaps the replay curve hit, each one Go line in `rt/std/num.go`:
`Float.sqrt`, `Float.exp`, `Float.log`, `Float.pow : Float -> Float -> Float`. They
are general (any curve, any statistics) and cheap; included unless the book budget
says otherwise.

## 4. The runtime

- `rt/web/request.go`: `Response` gains `Raw *rt.File` and a constructor
  `FileResponse(f rt.File, session any) Response`; its `Status` is the file's. In
  `router.write` (`rt/web/router.go:659`) a raw response is written first: the
  allow-listed headers, `Content-Type`, the status, the body; no `withScript`, no
  `dev` panel, no `Alder-Status`, no morph. The session cookie is set only when the
  handler changed the session (a file page cannot). A Datastar request (`rq.Datastar`)
  for a file route is answered with the navigate event to the same URL, as `/static/`
  is (`router.go:415`).
- The tracker's `setAnswer` records `file <status> <content-type> <bytes>` so the dev
  server's log and the devpanel History (on the next HTML page) show it.
- `internal/gen/routes.go:236`: the GET route of a file page calls `view` and returns
  `web.FileResponse`; `rq.Model` is set as for HTML pages.
- `internal/gen/caps.go`'s type mapping: `Bytes` ↔ `[]byte`, `File` ↔ `rt.File`.
  Stdlib functions map to `std.FileText/FileBytes/FileHtml/FileJson/FileWithStatus/
  FileWithHeader`, the getters, `std.BytesLength/BytesToText/TextToBytes`,
  `std.QueryDelay/TaskSleep`, the `Time` and `Float` additions.
- `rt.Equaler`/`rt.Keyer`: `Bytes` compares by content; `File` by status, content
  type, headers and body (so `==` in tests works).
- The health endpoint, `/static/`, `/_alder/` are unchanged.

## 5. Tests

`Test.fetch : Page -> Task { status: Int, contentType: Text, body: Bytes, headers: List
{ name: Text, value: Text } }` in `stdlib/Test.alder`, implemented in
`rt/testing/browser.go` beside `Test.open`: a GET through the test browser (cookie,
savepoint, trace), the raw response returned; `Bytes.toText` reads a text body.
`Test.open`/`Test.get` on a file page fail the test with "`Report.Csv` is a file page:
use `Test.fetch`". `Answer` is not extended (a new constructor would break every
exhaustive `match` on it in existing tests). A file page whose `requires` redirects
answers `Test.fetch` with 303 and a `Location` header.

Mutation testing: no mutants for `File.withStatus`/`withHeader`/`Query.delay`; the
existing kinds apply inside a file page's `load` and `view`.

Tests to add (corpus, `cmd/alder/testdata`, `rt/std`, `rt/web`, `rt/testing`): a text
file page; `File.html` with a doctype via `Html.unsafe`; a capability returning `Bytes`
with a fake; 503; a disallowed header literal (E0035); a `msg` in a file page (E0034);
`Test.fetch` status/body/headers; a redirecting `requires`; `Query.delay` advancing the
test clock and the trace line; a real `Query.delay` cancelled by the request context
(`rt/web`); the boost attribute on a link to a file page plus one browser-smoke check
(`go test -tags browser`: clicking it loads the file as a document); `Time` round
trips and the sign of `diffMillis`; `Bytes` equality.

## 6. Generators, tools, book

- `alder new page <Module> <path> --file [--type text/csv]`: a file page with `view`
  holding `?body : Text` under `File.text { contentType: "…", body: ?body }`.
  `--list/--show` refuse `--file`; `alder new msg` on a file page refuses with E0034's
  text; `new capability … "… -> Query Bytes"` works unchanged.
- Tree-sitter and the LSP need nothing (`File`, `Bytes` are types like any other).
- `internal/diag/explain.go`: E0034, E0035.
- Book: chapter 6 "File pages" (declaration, `File`, status and headers, links and the
  boost, what stays HTML: 404/500/login); chapter 7 "A capability returning bytes" and
  the delay rule; chapter 9 `File`, `Bytes`, `Query.delay`/`Task.sleep`, the `Time`
  and `Float` additions; chapter 10 `Test.fetch` and the virtual delay; chapter 11 the
  diagnostics table and `--file`; chapter 12 a paragraph on why bytes have no
  operations. The reference prompt regenerated and kept under 47k (today 45.8k): trim
  with `DROP_SUBSECTIONS` if needed.

## 7. Non-goals

- `@key` for query parameter names (M17b).
- Streaming or ranged responses, `HEAD` beyond Go's default, compression.
- Byte operations in Alder (slice, concat, decode beyond UTF-8), file uploads
  (multipart stays refused).
- A file page answering POST.
- Jobs or timers: `Task.sleep` is a request-scoped wait, not a scheduler.

## 8. Decisions for the user to confirm

1. `File` as a page whose `view` returns `File` (vs. a `file` keyword, or `load`
   returning the file with no `view`). Chosen for the smallest grammar and tooling
   footprint.
2. `Query.delay`/`Task.sleep` as built-in effects with a virtual delay in tests
   (proposed per the user's direction; the clamp of 60 s is a guess).
3. `Bytes` with only `length`/`toText`/`Text.toBytes` (no byte operations).
4. Milliseconds for `Time.toMillis`/`diffMillis` (vs. seconds).
5. 404/500/login stay HTML pages even for a file page.
6. The header allow-list (vs. any header but a deny-list).
7. `Float.sqrt/exp/log/pow` in M17.
8. `@key` deferred to M17b; the milestone number M17 and branch `m17-file-answers`
   (M16 is the Render target; M6c the latest merged).
