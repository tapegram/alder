# Settable test clock: implementation plan (#5)

Spec: `docs/superpowers/specs/2026-10-29-test-clock-design.md` (binding). TDD: each task starts with
a failing test, then the code, then green, then a commit.

## Task 1: `Clock.Set` and the stand-still rule (runtime)

- Test first, `rt/testing/testing_test.go`:
  - `TestClock_SetStandsStill`: `NewClock(start)`, `Set(t)`, three `Now()` all equal `t`;
    `Advance(6s)` then `Now()` is `t+6s`, and again `t+6s`.
  - `TestClock_AdvanceKeepsTick`: ticking clock, `Now`, `Advance(5s)`, `Now` is start+6s, next
    `Now` start+7s (the tick continues).
  - `TestClock_SetTwiceMovesBack`: a second `Set` to an earlier time is read next.
- Code, new `rt/testing/clock.go` (moved from `testing.go`): `Clock` gains `tick bool` (true from
  `NewClock`); `Now` adds the second only while `tick`; `Set(t)` sets `next` and clears `tick`.

## Task 2: `SetClock` / `Advance` steps (runtime)

- Test first, `rt/testing/testing_test.go`:
  - `TestRun_SetClockStandsStill`: a case running `SetClock(t)` then three reads of `e.Now()`:
    all `t`; the trace has `Test.setClock 2026-03-04T05:06:07Z`.
  - `TestRun_AdvanceClampsAndTraces`: `Advance(-5)` leaves the clock (trace `Test.advance 0ms`);
    `Advance(6000)` after a set moves it 6 s (trace `Test.advance 6000ms`).
  - `TestRun_SetClockKeptAfterFailedRun`: `Test.run` (`RunTest`) of `SetClock(t)` then a failing
    step; the next read is `t`.
  - `TestSetClock_NoClockPanics`: an Env without the clock panics `alder: this test has no clock`.
- Code, `rt/testing/clock.go`: `clockKey{}`; `SetClock(t time.Time) eff.Task[struct{}]` and
  `Advance(ms int64) eff.Task[struct{}]` as `eff.Call`s named `Test.setClock <RFC 3339 UTC>` and
  `Test.advance <ms>ms`. `newCaseEnv` stores the case clock in the context (before the browser
  slot wraps it).

## Task 3: stdlib signatures, the Go mapping, the corpus

- Test first: `testdata/corpus/clock-ok/` (App.alder, a `Today` file page that reads `Time.now`
  twice and `Date.today`, and a `Wait` page with `Query.delay` and then `Time.now`), with tests:
  set + equal readings; set, fetch, advance a day, fetch; a delay after a set moves the clock
  20 s; advance on a ticking clock; the trace lines; `Test.advance -5` is `0ms`. Run by
  `TestTest_Clock` in `cmd/alder/test_test.go` ("N passed, 0 failed"). Empty `expected.diag`.
- A diagnostic corpus case for `Test.setClock` outside a `test` (the existing "is only available
  inside a `test`" message, under the code that message already has).
- Code: `stdlib/Test.alder` gains the two signatures after `member`; `internal/gen/stdcall.go`
  gains `"Test.setClock": "rtesting.SetClock"`, `"Test.advance": "rtesting.Advance"`.

## Task 4: book

- `book/10-testing.md`: section 3's ticking-clock bullet gains "until the test sets it"; a new
  subsection "Setting the clock" after "`Test.fetch`: requesting a file" with the signatures, the
  stand-still rule, the `@created` note, the cookie note and one example (set a date, fetch a
  page printing `Date.today`, advance a day, fetch again).
- `book/09-stdlib.md` Test list: two bullets.
- `python3 book/site/build.py`; `cd experiments && python3 -m unittest` (raise the token cap with
  a comment naming #5 if needed).

## Task 5: verify and PR

- `go vet ./...`, `go test ./...` (foreground, 10-minute timeout), experiments unittest.
- Rebase on origin/main if it moved; push to `issue-5-test-clock`; `gh pr create`.
