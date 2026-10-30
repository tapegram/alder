# Settable test clock: `Test.setClock` and `Test.advance`

Design for [tapegram/alder#5](https://github.com/tapegram/alder/issues/5). Found while building
nightvote-replay, whose pages depend on `now − t0`. The user was not available for this design, so
everything marked **(call)** is the author's decision, not the user's.

## Intent

Today a test's clock starts at `2026-01-01T00:00:00Z` and moves one second at every reading
(`rt/testing/testing.go`, `Clock`). nightvote-replay has 16 modules that work backwards from that
constant (`t0 = 1767225600 − minute × 6`, `(s + 9) / 10` to round a Clarity second up to a tick),
and one more `Time.now` on the page's path shifts every expectation by a second. After this change
a test says what time it is and when time passes:

```alder
test "the site is 400 minutes past close" = {
  let _ <- Test.setClock (Time.fromMillis 1767225600000)   -- any t0 the cfg names
  let _ <- Test.advance (400 * 6000)                        -- minute 400 at speed 10
  let r <- Test.fetch (Sites.MnSos { cfg: "t0=1767225600,speed=10,start=21:00,key=test" })
  ...
}
```

## Goals

1. A test can set the clock to any `Time` and move it by any number of milliseconds.
2. A clock a test has set does not move on its own: `n` readings of `Time.now` give the same time.
3. The pages a test requests through the browser steps see the same clock, and `Query.delay` /
   `Task.sleep` in them keep advancing it.
4. No existing test changes meaning: a test that never touches the clock runs as it does today.

## Non-goals

- A CLI flag or `Options.Now` change (`alder test --now`): the per-test form covers the need.
- Turning the tick back on after a set, or freezing without a set. Neither has a use yet.
- Moving the browser's cookie clock (cookie expiry is untestable, as it is today).
- A `Test.now`; `Time.now` already reads the test's clock.

## Anti-goals

- A frozen default. Measured by freezing `Clock.Now` and running the examples: 27 tests fail
  (todos 5, blog 8, workspace 14), all "newest first" lists ordered by `@created`; the book's
  `Todos.Slow` ("21000") and `testdata/corpus/file-ok/Slow.alder` too; and `alder new`'s scaffold
  (`internal/scaffold/files/app/Home.alder`) and the generated admin write such tests, so every
  new app would start with failures. nightvote-replay would survive (its 52 `1767225600` sites
  are minute-granular, 6 s per minute, and no test orders by `@created`), but the break is not
  worth an opt-in nobody asked for. So: **the tick stays the default; a set clock stands still**
  **(call)**. The tick exists only to give `@created` an order without an API; once a test holds
  the clock, implicit movement is exactly the complaint in #5.
- Two clocks (one for `Time.now`, one for `@created`). One clock, as today.

## The API

Added to `stdlib/Test.alder`, after `member`, with the module's one-line comment style:

```alder
-- sets the clock: Time.now, Date.today and @created read t, and the clock stands still from
-- here on (no one-second tick); Test.advance and the virtual waits still move it
setClock : Time -> Task Unit
-- moves the clock forward ms milliseconds (negative is 0); ticking or set, it keeps that state
advance : Int -> Task Unit
```

Names **(call)**: `setClock`/`advance` as #5 proposes; they read as the module's other verbs
(`login`, `member`, `submitRaw`), and `Task Unit` is the module's existing shape for a step with
no result (`fake`, `login`, `member`; `Unit` is the Prelude type, `struct{}` in Go).

### Semantics

- **Before any set** the clock is today's: first reading is the start, each later reading one
  second on. `Test.advance ms` adds `ms` and the tick continues. `Query.delay`/`Task.sleep` add
  their delay as today (`Env.Sleep` is `Clock.Advance`).
- **`Test.setClock t`** makes the next reading `t` and turns the tick off for the rest of the
  case. Readings after it are all `t` until `Test.advance`, `Query.delay` or `Task.sleep` move it.
  A second `setClock` moves it again (backwards is fine; the clock is the test's).
- **`Test.advance ms`**: `ms` is clamped to 0 **(call)**, following the stdlib convention that
  counts clamp to zero (a negative advance is a bug, and `setClock` is the way back). No upper
  clamp **(call)**: unlike `Query.delay`'s 60000, advancing a day is the point. The unit is
  milliseconds, the unit of `Time.toMillis`, `Query.delay` and `Task.sleep` **(call)**.
- **`Date.today`** reads the same clock, so `setClock` sets today. `Time.ago now t` is pure and
  takes `now` explicitly, so a view that gets `now` from `load`'s `Time.now` follows the set
  clock with no change.
- **`@created`** reads the clock, so rows inserted after a set share one time; a test that sets the
  clock and then wants an ordered list calls `Test.advance 1000` between inserts, or seeds its rows
  before the set. The book says so (below).
- **Not transactional** **(call)**: as today's ticks, a set or advance inside a `Test.run` whose
  task fails is kept. Rolling the clock back with the savepoint would surprise more than it helps
  (the trace still shows the set; the test can set it again).
- **Trace**: both are steps and appear on it as `Test.setClock 2026-01-01T00:00:00Z` (RFC 3339
  UTC, as `Time.format` would print it) and `Test.advance 6000ms` (the clamped number, as
  `Query.delay 20000ms`) **(call)**. `Test.trace` and `Test.run`'s `trace` show them.
- **Browser steps**: a page requested by `Test.get`/`open`/`fetch`/`submit`/`click` reads the
  test's clock (the request Env copies `env.Now` and `env.Sleep`, both bound to the case's one
  `*Clock`), so a set applies to pages and a page's `Query.delay` moves the test's clock. The
  cookie clock (`cookieNow`) stays fixed at the default start **(call)**: a session a test logged
  in never expires however far the test moves time, as today.
- **Properties** (`Fuzz.forAllTask`): each attempt has a fresh Env and clock (`newCaseEnv`), so a
  `setClock` in the body applies per attempt and a `Test.advance` never carries across attempts.
  The derived `Date`/`Time` generators keep their anchors (the epoch and 2026-01-01); the anchor
  is a fact about the default start, not the clock **(call)**. Pure properties have no clock.
- **Only inside a `test`**: the checker already refuses every `Test.*` outside one
  (`internal/check/expr.go`, "`Test.%s` is only available inside a `test`"), so no new check.
  `alder eval` is not a test and cannot set the clock.
- **Mutation testing**: not a mutation target (the Test module is never mutated).

## Where things change

| layer | files |
|---|---|
| signatures | `stdlib/Test.alder`: the two lines above |
| name → Go mapping | `internal/gen/stdcall.go`: `"Test.setClock": "rtesting.SetClock"`, `"Test.advance": "rtesting.Advance"` beside the `Test.*` queries; they are StdCalls (calls returning `eff.Task[struct{}]`, as `Query.delay` and `Task.sleep` are), not new IR nodes |
| runtime | `rt/testing/testing.go`: `Clock` gets a `tick bool` (true from `NewClock`) and `Set(t)` (sets `next`, clears `tick`); `Now` adds the second only while `tick`; `Advance` unchanged. `newCaseEnv` puts the `*Clock` in `env.Ctx` under a `clockKey{}`, the way `withBrowser` stores the browser slot **(call)** (no new `eff.Env` field: the clock is a test-only thing). `SetClock(t time.Time) eff.Task[struct{}]` and `Advance(ms int64) eff.Task[struct{}]` are `eff.Call`s that find the clock in `e.Ctx` and panic `alder: this test has no clock` when absent (unreachable from a `test`; the message is for a build that bypasses the runner) |
| browser | `rt/testing/browser.go`: nothing; `EnvFor` already shares `Now`/`Sleep` |
| checker | nothing new; the builder confirms `Test.setClock`/`advance` resolve through the plain stdlib path (as `Test.doc` does) and are refused outside a `test` with the existing message |
| tests | `rt/testing/testing_test.go` (or the nearest existing file): set then three reads equal; advance on a ticking clock keeps the tick; negative advance is 0; set inside a failed `Test.run` is kept. A corpus case `testdata/corpus/clock-ok/` with a page that reads `Time.now` and `Date.today`, tested set, advanced and with a `Query.delay` after the set, and its trace |
| book | `book/10-testing.md`: section 3's "A ticking clock" bullet gains a sentence ("until the test sets it: `Test.setClock`, section 9"); a new subsection **"Setting the clock"** after "`Test.fetch`: requesting a file" (the `Todos.Slow` example is the natural lead-in) with: the two signatures, the stands-still rule, the `@created` note, the cookie note, and one example test that sets the clock to a date, fetches a page that prints `Date.today`, advances a day and fetches again. `book/09-stdlib.md` "Test" list: two bullet lines. `book/07-effects.md` "Waiting": no change (its claim stays true) |
| site | `book/site/build.py` regenerates `site/10-testing.html`, `site/09-stdlib.html`, `site/reference.md`, `site/reference-full.md` |

Also: tick #5 in the roadmap issue #52; the user reinstalls (`go install ./cmd/alder`) since LSP
completion reads the stdlib signatures; a nightvote-replay dev-log entry when it adopts this
(its `at minute` helpers become a set plus an advance, and `(s + 9) / 10` goes).

## Left to the builder

- Whether `Clock.Set` and the ctx lookup live in `testing.go` or a new `rt/testing/clock.go`.
- The exact book sentences and the example page's name.
- Whether the trace time is printed through `rt/std`'s `TimeFormat` or `time.Time.Format`
  (`"2006-01-02T15:04:05Z07:00"`); either gives the RFC 3339 form above.

## Verification

- `go test ./...` green, including the new runtime and corpus cases.
- The three example suites and `testdata/corpus` pass unchanged (goal 4; they never set the clock).
- A scratch check, not a change to nightvote-replay (it pins its own Alder): one of its
  `at minute` tests rewritten as `setClock` + `advance` gives the same body, and the same test
  with an extra `Time.now` added to the page still passes.
