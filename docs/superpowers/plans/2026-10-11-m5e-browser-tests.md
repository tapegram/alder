# M5e: Request and browser tests — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A test drives the app the way a user does, in memory: `Test.login`, `Test.get`/`Test.open` a page, `Test.submit`/`Test.submitRaw` a form, `Test.click` a message, read the typed model, the answer, the session and the document (`Doc` queries), with every request running inside the test's transaction under a savepoint, in milliseconds, deterministically.

**Architecture:** (1) Runtime: `rt/html` gains a parser for its own output and a selector engine (the selector grammar lives in `internal/selector`, shared with the checker); `rt/eff.Run` can run a Task inside an outer transaction under a savepoint; `rt/web` lets a caller supply the per-request `Env`; `rt/testing` gains the browser (router + cookie jar + current document + answer application). (2) Compiler: Prelude test-only types `Doc`/`Answer`/field error; the six steps and the queries typed as special forms; E0023 for selector literals; IR nodes; gen Calls into the browser; a generated `app.Web()` options value shared by `Serve` and the test main. (3) Examples, chapter 10, 9, 11, 13, 14, README, the reference.

**Tech Stack:** Go; `net/http/httptest`; the existing `rtesting.Inner` savepoint pattern.

**Spec:** `docs/superpowers/specs/2026-10-11-m5e-browser-tests-design.md` (binding). Related: M4 spec (tests), M5 spec ("Stateful fakes"), the M5d "no test code in production" rule.

## Global Constraints

- The surface is exactly the spec's: six steps, the `Answer` union (`Shown | Rerendered | Redirected Page | Rejected (List { field: Text, message: Text }) | Missing | Failed Text`), five queries plus `Test.doc`, the selector subset (tag, `#id`, `.class`, `[attr]`, `[attr=value]`, compound, descendant, child). Result records are `{ model: M, session: Session, doc: Doc }` for `Test.get` and `{ answer: Answer, session: Session, doc: Doc }` for the others. `Doc` has `==`.
- Every request runs inside the test's transaction under a savepoint: the test's rows are visible to the page, a failed request's writes are undone, and no second connection is opened. One in-memory SQLite connection per test, as today.
- The browser is a Datastar client for submits and clicks (`Datastar-Request: true`); plain GET for `get`/`open`. Answers are applied as the spec says. A step records itself on the trace, followed by the request's inner effects.
- `Test.click`/`Test.submit` act only on controls the current page renders; otherwise the ruled failure texts. `Test.get` fails unless a page was shown; `Test.open` never fails on an answer.
- Test-only: nothing of the browser, parser or steps reaches a production build (M5d rule); `Test.*` outside a `test` is the existing E0003.
- Reserved names (E0012 extends): `Shown`, `Rerendered`, `Redirected`, `Rejected`, `Missing`, `Failed`, `Doc`, `Answer`.
- New diagnostic E0023 (selector literal) with an explain text and corpus cases; every new error text is in the corpus and quoted verbatim in the book.
- Gate: `gofmt -l $(git ls-files '*.go')` empty; `go vet ./...`; `go test ./... -count=1`; both examples check/build/test (counts grow: todos 3 → 6+, blog 24 → 28+); `(cd experiments && python3 -m unittest)`; `go test ./book -count=1`.
- Commit messages end with `Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>` and `Claude-Session: https://claude.ai/code/session_01VSwqPTccEdvyAkyJ2WtrhG`. Stage by explicit path.

## Review Focus

1. **A `Rejected` submit**: the held document is the re-rendered form with the user's values and the errors in `<ul class="errors" data-errors="field">`, `errors` lists them in field order with `""` for the form-level `Reject` text, and the session is unchanged. Test in Task 1 (browser) and Task 3 (the blog comment form).
2. **A click on a message the page renders only for someone else** (a stranger's `Toggle { id }` in todos: `load` lists only the user's todos, so no button exists): expected — the test fails "the page has no button or form for Todos.Index.Toggle { id: "…" }", NOT a 404 — the 404 path is reached by the raw post, which the browser does not offer. The book shows the ownership test written as: the stranger `Test.open`s `Todos.Show { id }` → `Missing`. Test in Task 3 (todos).
3. **Page parameters in message URLs** (`_p.filter`): a click on a page opened as `Todos.Index { filter: Active }` posts a URL carrying `_p.filter=Active`, and a `Rerendered` answer re-renders with that filter. Test in Task 1 (browser, a fixture page with a parameter) and Task 2 (e2e).
4. **`requires` and `Test.login`**: `Test.open` of a page that requires a user with no login → `Redirected (Auth.Login {})` and the held page is the login page; after `Test.login { user: Just me }` → `Shown`; a `Test.submit` of the login form with the `Password` fake → `Redirected (Todos.Index {})` and `after.session.user == Just me`. Test in Task 2 (e2e) and Task 3 (todos).
5. **A step inside `Test.run`** and a failing request inside a step: `Test.get` inside `Test.run`'s task works (nested savepoints); a request whose `update` panics gives `Failed msg` from `open`/`submit`/`click`, with the test's own rows intact afterwards. Test in Task 1 (browser) and Task 2 (checker allows it; e2e).

---

### Task 1: Runtime — parser, selectors, nested Run, the browser

**Files:**
- Create: `internal/selector/selector.go` (`Parse(string) (Selector, error)`: the subset; `Selector` is a list of compound steps with combinators; `Error` names the offending token and the subset), `selector_test.go`
- Modify: `rt/html/html.go` + create `rt/html/parse.go` (`Parse(s string) (Node, error)` for the renderer's own output: lower-case tags, quoted attributes, the void-element set, raw text for `script`/`style`, entities decoded; `(Node).Equal(Node) bool` structural; `Select(n Node, sel selector.Selector) []Node`; `TextContent(n Node) string` whitespace-collapsed; `AttrOf(n Node, name string) (string, bool)`), `parse_test.go` (round-trips every golden `want` page and both examples' rendered pages: `Render(Parse(Render(x))) == Render(x)`), `select_test.go`
- Modify: `rt/eff/run.go`, `eff.go` (a Task can run **within** an outer transaction: `Env.Within` (set by the test runner: the outer `Tx` and the store's `Savepointer`) makes `Run` take a savepoint on that `Tx` instead of `Begin`, release it on success and roll back to it on failure; nested use is fine (fresh savepoint names, the existing counter); `Transaction()` returns the outer `Tx`), `run_test.go`
- Modify: `rt/web/router.go` (`Options.EnvFor func(ctx context.Context, session any) *eff.Env`: when set, the router uses it instead of building a fresh `Env` — the browser supplies one sharing the test's `Store`, `Within`, `Caps`, `Now`, `Log` and a trace sink; `nil` keeps today's behaviour), `router_test.go`
- Create: `rt/testing/browser.go` (`type Browser struct{ … }`; `NewBrowser(env *eff.Env, routes []web.Route, opts web.Options) *Browser` (the test runner builds one per case lazily); `Login(session any) error`; `Get(url string) (Result, error)`; `Open(url string) (Result, error)`; `Submit(msgURL string, fields url.Values) (Result, error)`; `Click(msgURL string) (Result, error)`; `type Result struct{ Answer Answer; Session any; Doc html.Node }`; `type Answer struct{ Tag AnswerTag; Page rt.Page; Errors []web.FieldError; Text string }`; the cookie jar; `Doc()` the held page; control checks: `Submit` requires a `<form action=msgURL>` (or `data-on:submit` with that URL) on the held page, `Click` a `button`/`form` whose `formaction`/`action`/`data-on:click` posts it, else the ruled errors; Datastar answers: parse the SSE (`datastar-patch-elements` → replace the held `<body>`; the navigate event → `Get`); a 303 (non-Datastar) → `Get`; 404/500 → `Missing`/`Failed` with the served document held; `Rejected` recognised from the patch's `FormState` errors (the re-rendered form has `data-errors` lists with items; the form-level error is `<ul class="errors" data-errors="">`? — read how `<Errors form />` renders and match it)), `browser_test.go` (a hand-written route set covering RF1, RF3, RF5, the cookie round trip, encoding is the generated code's job — here pass `url.Values`; a failing request's writes undone, the test's rows intact; nested within `Inner`)
- Modify: `rt/testing/testing.go` (`Options.Routes []web.Route`, `Options.Web web.Options` (the generated options without Store/Caps/Now); per case: `env.Within` set to the case's transaction (the case's Task runs via `Run`, which begins the transaction — the browser's requests then run `Within` it: make the browser take the env and read `env.Tx` lazily at request time), `Main` gains the parameters; `Step(env, name)` helper records a step on the trace)

**Interfaces (binding for Task 2):**
```go
// rt/testing
func NewBrowser(env *eff.Env, routes []web.Route, opts web.Options) *Browser
func (b *Browser) Login(session any) error
func (b *Browser) Get(url string) (Result, error)      // error = the ruled failure (not shown)
func (b *Browser) Open(url string) (Result, error)     // error only for internal failures
func (b *Browser) Submit(msgURL string, fields url.Values) (Result, error)
func (b *Browser) Click(msgURL string) (Result, error)
type Result struct { Answer Answer; Session any; Doc html.Node }
type AnswerTag int // Shown, Rerendered, Redirected, Rejected, Missing, Failed
type Answer struct { Tag AnswerTag; Page rt.Page; Errors []web.FieldError; Text string }
func BrowserOf(env *eff.Env) *Browser   // the case's browser (created on first use; stored in env via ctx or a registry)
// rt/html
func Parse(s string) (Node, error); func (n Node) Equal(m Node) bool
func Select(n Node, sel selector.Selector) []Node; func TextContent(n Node) string; func AttrOf(n Node, name string) (string, bool)
// internal/selector
func Parse(s string) (Selector, error)
// rt/eff
type Within struct { Tx Tx; Savepoints db.Savepointer }  // on Env
// rt/web
Options.EnvFor func(ctx context.Context, session any) *eff.Env
// rtesting.Main(cases, tables, testTables, migrations, caps, session, routes []web.Route, web web.Options, args, out, err)
```

- [ ] **Step 1 (RED):** selector tests (every form in the subset; errors name the token); parser round-trip tests; `Run` within an outer transaction (writes visible to the outer tx; a failed inner run undone; nested); router `EnvFor`; browser tests over the fixture routes (RF1, RF3, RF5; cookie; control checks with the exact texts; Datastar patch and navigate; 303; 404/500).
- [ ] **Step 2:** implement. **Step 3 (GREEN):** `go test ./rt/... ./internal/selector -count=1`; both examples still pass (the runner's new parameters have zero values until Task 2 wires them). **Step 4:** commit (two: html+selector; eff+web+testing).

---

### Task 2: Compiler — types, special forms, E0023, IR, gen, the test main

**Files:**
- Modify: `stdlib/Prelude.alder` or wherever Prelude types/`Test` live (`Doc`, `Answer` with its six constructors, the field-error record alias `FieldError = { field: Text, message: Text }`; `Test.login/get/open/submit/submitRaw/click` as special-form signature lines with comments like `Test.fake`'s; `Test.text/texts/has/count/attr/doc` as ordinary typed values: `text : Text -> Doc -> Maybe Text`, `texts : Text -> Doc -> List Text`, `has : Text -> Doc -> Bool`, `count : Text -> Doc -> Int`, `attr : Text -> Text -> Doc -> Maybe Text`, `doc : Html a -> Doc`)
- Modify: `internal/check` (`decls.go`: the reserved-name map gains the eight names; a new `browser.go`: `inferTestGet` etc. — `Test.get p`/`Test.open p`: `p` must be a `PageRef` expression (constructor of a page module with its params record) → result type `Task { model: <that page's Model>, session: Session, doc: Doc }` (`Model` a union → E0003 "`Test.get` needs a record `Model`: `Todos.Index`'s is a union — use `Test.open`"); `Test.submit F r`: `F` a form of a page module (qualified from another module), `r` checked as that form's record with request-value defaults; `Test.submitRaw F fields`; `Test.click m`: `m` a message constructor application of a page module or `App` (its payload record incl. `form`? — a form message cannot be clicked: E0003 "`Submit` carries form `NewTodo`: use `Test.submit`"); `Test.login s`: the app's session record; all only in tests; `Doc`/`Answer` outside a test: E0003 "`Doc` is a test type"; selector literal args of the five queries → `selector.Parse` → E0023 "`#sumary x` is not a selector: unexpected `x` after `#sumary` (a selector is a tag, `#id`, `.class`, `[attr]`, `[attr=value]`, combined with spaces or `>`)"), `internal/diag` (E0023 + explain), corpus: `browser-ok/` (a fixture app with a page, a form, a parameter, a click, `requires`, `Test.login`, every query), `E0023-selector/`, `E0003-test-get-union-model/`, `E0003-test-submit-not-a-form/`, `E0003-test-click-form-msg/`, `E0003-test-click-not-a-msg/`, `E0003-doc-outside-test/`, `E0012-answer-constructor/`
- Modify: `internal/ir/ir.go` (`TestLogin{Typed; Session Expr}`, `TestGet{Typed; Page *Page; Params Expr; Open bool}`, `TestSubmit{Typed; Form *Form; Msg *Msg; Page *Page; Record Expr}`, `TestSubmitRaw{Typed; Form *Form; Msg *Msg; Page *Page; Fields Expr}`, `TestClick{Typed; Page *Page (nil for App); Msg *Msg; Params Expr}`; the queries lower as ordinary std calls), `lower_eff.go`, `dump.go`, `ir_test.go`
- Modify: `internal/gen` (`effects.go`: each step → `callStmts(name, …)` emitting `rtesting.BrowserOf(env).Get(rt.PageURL(<page value>))` etc.; `TestSubmit` encodes the record into `url.Values` per field type exactly as the inputs would (reuse the decoder's expectations: `web.Text/Int/Float/Bool/Date/Enum` — write the inverse `encode__<Module>__<Form>(r) url.Values` per form in test builds); `TestClick` builds the message URL with the held page's `_p.` params (the browser adds them from the held document's form state? — ruling: the browser reads the held page's `PageParams` from its rendered message URLs: the generated code passes the message path + the message's own params, the browser appends the `_p.` keys it finds on the held page for that message); `Answer` → the Go union (rt generated unions for Prelude types: see how `Nav`/`Maybe` are mapped); `Doc` → `html.Node`; the queries → `rtesting.Text(sel, doc)` etc. with the selector parsed once per literal (`selector.MustParse` emitted for literals; `selector.Parse` at runtime for non-literals); `caps.go`/`gen.go`: `app.Web() web.Options` generated (the options `Serve` builds minus Store/Caps/Now/Log/Dev/Secret/Reload; `Serve` uses it) and the test main passes `app.Routes, app.Web()`; production emission unchanged except `Web()`), goldens (`gen_browser` test build; prod goldens gain `Web()`), `gen_test.go` (`TestGen_Browser` e2e on the corpus fixture: RF3, RF4, RF5 end to end through `alder test` on a scratch copy with an App module and the Password-style fake)
- Modify: `cmd/alder/test.go` (`testMain` passes routes + web options), `internal/scratch` mains likewise (eval/repl are test builds; they pass them too so `Test.*` typing is uniform — the steps remain test-only by the checker)

**Interfaces (binding for Task 3):** the step and query names and types above; the error texts (verbatim in the corpus); `app.Web()`.

- [ ] **Step 1 (RED):** corpus cases; IR dump golden; gen golden; e2e. **Step 2:** implement stdlib → check → ir → gen → cmd/scratch. **Step 3 (GREEN)** + full gate (examples unchanged in count). **Step 4:** commits (check+ir; gen+cmd).

---

### Task 3: Examples, book, reference

**Files:**
- Modify: `examples/todos/app/Todos/Index.alder` (tests: add a todo through the page and read `#summary` ("1 open"); a stranger cannot see the todo: `Test.login` as the stranger, `Test.get (Todos.Index {})` shows no `li`, and `Test.open (Todos.Show { id })` is `Missing`; toggle through the page: `Test.click (Toggle { id })` → `Rerendered`, `Test.has "li.done"`), `examples/todos/app/Auth/Login.alder` (login through the form with a `fake Password` (new, beside the capability) that accepts a known email: `Rejected` for a wrong password with the form-level message; `Redirected (Todos.Index {})` and `after.session.user == Just me` for the right one; logout via `Test.click App.Logout`), `examples/blog` (pagination: `Test.get (Posts.Index { page: 2 })` and the next/previous links via `Test.attr "href"`; search; a comment's validation error through `Test.submitRaw` → `Rejected [{ field: "body", message: "must not be empty" }]`; the author's dashboard requires login → `Redirected`), `TestExample_*` counts, chapter 14's transcripts
- Modify: `book/10-testing.md` (new section "Testing through the browser" before §9: the browser model, each step with a verified block and a captured transcript (a scratch copy of todos), the queries and the selector subset, `Test.doc` vs `view`, `Rejected`, `requires` + `Test.login`, the rules (acts only on rendered controls; savepoint; Datastar client; the trace), the ownership example (Review Focus 2), what remains outside (CSS, layout, web-component JavaScript; the no-script path); §9 drops three items), `book/09-stdlib.md` (the `Test` list: the six steps as special forms, the queries), `book/11-tooling.md` (E0023 row; E0012 names), `book/13-compiler.md` (the browser: `EnvFor`, `Within` and the savepoint, the parser, the Datastar client, `app.Web()`; quote generated code from a real test build), `book/12-rationale.md` (why an in-process browser, not Playwright: the user's question and the answer: coverage at the seams, milliseconds, no flakiness; the remaining gap owned by M7), `book/README.md`, `experiments/build_context.py` (keep the step table and the rules; report the size), harness README (E8 hidden tests may use the steps)
- Every quoted text and transcript from real runs; `go test ./book`.

- [ ] **Step 1:** examples + counts; **Step 2:** book + reference; **Step 3:** full gate; commits (examples; book).

---

## Self-review notes

- Spec coverage: surface (T2 types/forms, T1 behaviour), runtime (T1), compiler (T2), errors (T2 corpus; T1 runtime texts), testing (T1–T3), book (T3), non-goals stated in ch.10 (T3).
- Interfaces: T2 consumes T1's `Browser`/`Result`/`Answer`/`html` API; T3 consumes T2's names and texts.
- Review Focus 1–5 each have a named test.
- Open question deferred: the no-script form path; a real-browser smoke suite (M7).
