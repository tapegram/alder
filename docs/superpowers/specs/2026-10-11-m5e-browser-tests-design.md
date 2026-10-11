# M5e: Request and browser tests — Design

Date: 2026-10-11. Status: approved in conversation; this is the spec the
M5e plan argues from. Related: the M4 test/REPL spec (`alder test`,
`Test.run`, traces), the M5 spec ("Stateful fakes"), chapter 10.

## Goal

A test drives the app the way a user does, in memory and in milliseconds:
get a page, submit a form, click a button, read what the page now shows and
what the session now holds. This closes three of the four gaps chapter 10
admits ("a request's round trip", "`view` in a browser", "observing the
session after a step") without a browser process, so a suite can hold
thousands of such tests and stay deterministic. What stays outside is
stated: CSS, layout and the JavaScript a web component brings (M7 owns a
small real-browser smoke suite for those).

## Decisions recorded (2026-10-11)

- **Assert against both the document and `view`, with typed results
  underneath** (user chose C): every step returns the typed model (for a
  get) or the answer, the session it left, and the document.
- **Log in directly or through the real flow** (C): `Test.login` sets the
  session from a value; the login page is tested through its form with the
  `Password` fake when login itself is under test.
- **A click applies the server's answer, and the raw answer is also in the
  result** (C): the held page is what a browser would now show.
- **Forms by name, typed, plus a raw variant** (C): `Test.submit Form {…}`
  encodes the form's record as inputs would; `Test.submitRaw` sends text
  fields for validation tests.
- Why this and not Playwright: the biggest production bugs in a
  server-rendered app live at the seams (route params, decoding,
  `requires`, ownership checks in `update`, redirects, session writes,
  stale renders), all visible only across a request and the next one; an
  in-process browser covers them at milliseconds per test with no
  flakiness source; a real browser is seconds per test and flaky, so
  teams keep few of them.
- **Renamed 2026-10-11 by the user:** `Rejected` is now `FormRejected`,
  `Missing` is now `PageMissing` and `Failed` is now `RequestFailed`,
  because `Failed`, `Rejected` and `Missing` are reserved names (E0012)
  that collide with common domain words; the rename frees them, so that
  common domain constructors stay legal (`type Status = Pending | Failed
  | Rejected | Missing` is a valid program). The spec below uses the new
  names throughout.

## The surface

A test holds an implicit browser next to its fresh database and
capability set: the current page (a document) and a session cookie. Steps
are test-only like `Test.run` (E0003 outside a `test`):

```
Test.login     : Session -> Task Unit
Test.get       : Page -> Task { model: M, session: Session, doc: Doc }
Test.open      : Page -> Task { answer: Answer, session: Session, doc: Doc }
Test.submit    : Form -> Task { answer: Answer, session: Session, doc: Doc }
Test.submitRaw : Form -> List { name: Text, value: Text }
                      -> Task { answer: Answer, session: Session, doc: Doc }
Test.click     : Msg -> Task { answer: Answer, session: Session, doc: Doc }

type Answer = Shown
            | Rerendered
            | Redirected Page
            | FormRejected (List { field: Text, message: Text })
            | PageMissing
            | RequestFailed Text
```

- **`Test.login s`** signs `s` (the app's `Session` record; the checker
  holds the test to its shape) into the browser's cookie. The session
  type without an `App` module is `{ }`.
- **`Test.get p`** requests the page value `p` (`Todos.Index { filter:
  All }`) with the cookie, follows nothing, and fails the test unless a
  page was shown: "GET /todos/x: not found" / "GET /: redirected to
  /login (requires user)" / "GET /: failed: <message>". `M` is the
  `Model` of `p`'s page module, so the result is fully typed. The
  browser now holds that page.
- **`Test.open p`** is the raw form: the same request, but the result's
  `answer` says what happened (`Shown`, `Redirected (Auth.Login {})`,
  `PageMissing`, `RequestFailed msg`) and the test asserts. After `Redirected` the
  browser holds the redirect's target page (fetched); after `PageMissing`
  the not-found document; after `RequestFailed` the error document.
- **`Test.submit F r`** needs a `form F` of a page module; `r` is `F`'s
  record (a form literal is a request value, so defaults apply as they do
  for a request). The browser encodes each field as the page's inputs
  would (`Text`/refined text as given; `Int`/`Float` as digits; `Bool` as
  a checkbox: present or absent; `Date` ISO; `Maybe` absent for
  `Nothing`; enums by constructor name) and posts to `F`'s message on the
  current page, with the page's parameters as the rendered message URL
  carries them. The current page must render that form (its `<form
  submit={Msg}>` posts the message's URL); otherwise the test fails
  "the page has no form NewTodo (Todos.Index.Submit)".
- **`Test.submitRaw F fields`** posts the given `name`/`value` pairs to
  `F`'s message the same way, for input the types forbid (an empty
  `Text.NonEmpty`, a malformed date). A name `F` does not declare fails
  the test "form NewTodo has no field `colour`".
- **`Test.click m`** posts the message `m` (`Toggle { id: id }`, or
  `App.Logout`) as the page's `click` button would. The current page must
  render a control posting exactly that message URL (same message, same
  parameters); otherwise "the page has no button or form for
  Todos.Index.Toggle { id: "…" }". That rule is what makes it a browser
  test rather than a raw POST: a test can only act on what the user sees.
- **The answer is applied.** `Rerendered`: the body patch replaces the
  held body (the same body a `Rerender` gives). `Redirected p`: the
  browser fetches `p` with the (possibly updated) cookie and holds it; a
  redirect to a page that then 404s is `PageMissing`. `FormRejected errors`: the
  held page is the re-rendered form with its errors; `errors` lists them
  in field order (`field` is the field name, `""` for the form-level
  `Reject` text). `PageMissing` / `RequestFailed`: as for `open`. The result's
  `session` is the session after the step (what `Session.put` left), so
  "observing the session after a step" is `after.session.user == Just me`.
- **Queries** are pure functions on `Doc`, the parsed document:
  `Test.text : Text -> Doc -> Maybe Text` (the first match's text content,
  whitespace collapsed), `Test.texts : Text -> Doc -> List Text`,
  `Test.has : Text -> Doc -> Bool`, `Test.count : Text -> Doc -> Int`,
  `Test.attr : Text -> Text -> Doc -> Maybe Text` (attribute name,
  selector), `Test.doc : Html a -> Doc` (render a view for comparison:
  `after.doc == Test.doc (view expected session)`). `Doc` has `==`
  (structural). Selectors are the subset: a tag, `#id`, `.class`,
  `[attr]`, `[attr=value]`, compound (`li.done`), descendant (`ul li`)
  and child (`ul > li`). A selector written as a literal is checked at
  compile time (E0023 "`#sumary x` is not a selector: …"); a non-literal
  selector that does not parse makes the query give `Nothing`/`[]`/
  `False`/`0`, which the book warns about.

## The runtime

- `rt/testing` gains a **browser**: it builds the app's real router
  (`web.NewRouter(app.Routes, opts)`, the same routes the binary serves,
  with the test's store, clock, capability set incl. installed fakes, and
  session codec) and drives it with in-process requests (`httptest`).
- **Each request runs inside the test's transaction under a savepoint**,
  exactly like `Test.run` (`rtesting.Inner`): the router's per-request
  `Env` is given the test's transaction instead of opening one, so the
  test's inserted rows are visible to the page, a failed request undoes
  only its own writes, and no second connection or network exists. This
  needs one runtime hook: `web.Options` takes an optional `Tx`/`Env`
  factory the test runner supplies.
- **The browser is a Datastar client**: submits and clicks send
  `Datastar-Request: true`, so answers are the patch or navigate events
  the real client gets; applying a patch is replacing the held body (the
  morph is semantic, not structural); a navigate is a GET. Plain
  navigation (`Test.get`/`open`) is an ordinary GET. The no-script path
  (a plain form POST answered 303) is not modelled; chapter 10 says so.
- **The cookie jar**: the browser keeps the session cookie the router
  sets and sends it back; `Test.login` writes it directly with the
  router's codec and secret (the test secret).
- **Traces**: a step records itself (`Test.get Todos.Index`,
  `Test.submit NewTodo`, `Test.click Todos.Index.Toggle`,
  `Test.login`) and the request's inner effects follow, as with
  `Test.run`; `Test.open` records `Test.open …`.
- **Parsing**: `rt/html` gains a parser for its own output (always
  well-formed: lower-case tags, quoted attributes, void elements known,
  `<script>`/`<style>` raw text) producing the node tree `Doc` wraps, and
  the selector engine. `Doc` equality compares trees.
- **Not in production**: the browser and parser are linked only by test
  builds (`gen.Options{Tests}`), like fakes.

## The compiler

- The checker types the six steps as special forms (like `Test.run`):
  `Test.get`/`open` require a page value (`Page`-typed expression whose
  head is a page constructor) and resolve the page module's `Model`;
  `Test.submit`/`submitRaw` require a `form` declared by a page module and
  resolve its message; `Test.click` requires a message of a page module
  or of `App`; `Test.login` requires the app's `Session`. Any step
  outside a `test`: the existing E0003 for `Test.*`.
- `Doc`, `Answer`, and the field-error record are Prelude types visible
  only in tests (E0003 "`Doc` is a test type" elsewhere). `Answer`'s
  constructors are Prelude names a program may not write as its own
  constructors (E0012 extends to `Shown`, `Rerendered`, `Redirected`,
  `FormRejected`, `PageMissing`, `RequestFailed`); the 404 case is spelled `PageMissing`
  rather than `NotFound` so that a program's own `NotFound` constructor
  stays legal.
- E0023: a selector literal that does not parse (the text names the
  offending token and the subset).
- Lowering: `TestGet{Page, Model}`, `TestOpen{Page}`, `TestSubmit{Form,
  Msg, Record}`, `TestSubmitRaw{Form, Msg, Fields}`, `TestClick{Msg}`,
  `TestLogin{Session}`, `TestQuery{Fn, Args}`; gen emits Calls into the
  browser; production emission never sees them (M5d: no test code).

## Error handling

- Compile time: E0023 selectors; "`Test.get` wants a page value, got
  …"; "`Test.submit` wants a form declared by a page: `NewTodo` is a
  form of `Todos.Index`, so write `Todos.Index.NewTodo`" (when
  unqualified from another module); "`Test.click` wants a message of a
  page or of `App`"; "`Test.login` wants the app's session `{ user:
  Maybe User.UserId }`".
- Run time, as test failures naming the request: `Test.get` not shown
  (status and reason); an action the page does not offer (with the
  message and the page's URL); `submitRaw` with an undeclared field; a
  request that panics (the runtime's internal error, as today).
  Everything else is an `Answer` the test asserts on.

## Testing

- `rt/html`: parser round-trips every golden page; selector table tests.
- `rt/testing`: browser tests over a small hand-written route set: get,
  cookie round trip, submit encoding per field type, Datastar patch
  application, navigate following, the "no such control" failure, the
  savepoint (a failed request's writes are gone, the test's rows remain).
- Checker: corpus cases for every error above; `Test.get` typing.
- gen: a golden test build; the production golden unchanged; an e2e
  fixture app with a page, a form, a click, `requires`, and `Test.login`.
- Examples: todos — the ownership check through the page (a stranger's
  click is `PageMissing`), login through the form with the `Password` fake,
  adding a todo and reading `#summary`; blog — pagination (`?page=2`),
  search, a comment's validation error (`FormRejected`), logout.
- Book: chapter 10 "Testing through the browser" with real transcripts;
  §9 drops three items and names what remains outside; chapter 9's
  `Test` list; chapter 11 (E0023); chapter 13 (the browser, the
  savepoint, the parser); chapter 14's tests; the agent reference keeps
  the step table and the rules.

## Non-goals

- A real browser, CSS, layout, web-component JavaScript (M7's smoke
  suite). The no-script form path. Concurrency or load. Screenshots.
- Testing adapters through the browser beyond what fakes give (an
  adapter's Go tests cover its own logic).
