# Background and Scheduled Jobs Implementation Plan (#168)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** An Alder app declares `job Name = { … }` beside its routes, enqueues it from any `Task`
in the same transaction as its writes, and has it run, retried, scheduled and shown by its own
process, in development, in tests and on every deploy target, with `workers: N` on Render moving
the jobs to worker instances of the same image.

**Architecture:** A built-in queue (`rt/jobs`) over one runtime-owned table, `_alder_jobs`, per
store. The compiler adds a `job` declaration, a `Job` stdlib module and two test steps, and
generates per job a JSON codec and an entry in `app.Jobs()`. One core, `jobs.RunOne`, runs an
attempt and does its bookkeeping; the worker loop and `Test.work` are its two callers. The
generated `Serve` is split so the worker shares the router's store, capabilities, clock and log.

**Tech Stack:** Go (compiler, runtime, CLI), Alder (`stdlib/Job.alder`, `examples/todos`), SQLite
through `modernc.org/sqlite` and Turso over Hrana, tree-sitter (`tree-sitter-alder/`), markdown
(`book/`).

**Spec:** `docs/superpowers/specs/2026-11-15-background-jobs-design.md` (at 9f2fa905). Read it
first. This plan argues from it and does not repeat its design sections; a task names the spec
section it implements, and that section is part of the task's requirements.

## Global Constraints

- Seven PRs, each from a short branch off main, green on its own, merged before the next starts:
  PR 1 = Tasks 1–6, PR 2 = Tasks 7–10, PR 3 = Tasks 11–12, PR 4 = Tasks 13–14, PR 5 = Tasks
  15–16, PR 6 = Tasks 17–18, PR 7 = Tasks 19–21.
- No new Go dependency. `go.mod`'s require list does not change. The cron parser is hand-written.
- Every time `rt/jobs` reads is `env.Now()` or an injected `func() time.Time`; `time.Now` does
  not appear in `rt/jobs` outside the default it is given. `grep -n 'time\.Now' rt/jobs/*.go`
  shows only that default.
- Nothing in the test runtime starts a goroutine for jobs.
- Times in the table are milliseconds since the epoch (`Time`'s representation). Ids are 8 hex
  characters from `applog.NewID` (`rt/applog/id.go:20`).
- Values pinned by the spec: default retries 4 (five attempts); backoff after attempt 1, 2, 3, 4
  = 10 s, 1 min, 10 min, 1 h, and 1 h for every later one; timeout default `5m`, maximum `1h`;
  lease = timeout + 30 s; poll 1 s; housekeeping every 1 min; prune done after 7 days, dead after
  30 days; unclaimable warning and unknown-kind tick deletion after 1 day; params at most 64 KB
  (65536 bytes); error text at most 4 KB (4096 bytes); `Test.work` cap 1000 runs; `every`
  minimum `1s`; write-lock mark above 2 s; `ALDER_JOBS` default 4, 8 under `--jobs-only`, 0 =
  off; `--jobs-only` ahead of the database polls every 10 s; stalled = a row due for more than
  5 min with no claim in that time; silo active set drops a silo after 1 h with nothing due.
- New codes: **E0049** "A job parameter that cannot be stored"; **E0050** "A scheduled job takes
  no parameters" / "A job has two schedules" (one code, several messages). Register them in
  `internal/diag/diag.go` after E0048 (`:149`) with explanations in `internal/diag/explain.go`.
  A missing or mistyped `run` arm reuses **E0043** (`diag.RouteDecl`) with "job" in the wording.
- Logged errors, panel text and book prose quote the spec's wording where it gives one
  (`held the write lock 41 s: write last, or fan out — book 17`, `lost: the process stopped`,
  `timed out after 5m`, `cannot decode the parameters: …`, `Test.work: jobs keep enqueueing
  jobs`).
- Verify every behaviour claim written into docs, the book or an error message against the Go,
  and cite `file:line` in the task report. Reviews of #119 caught invented claims in every PR.
- Generated-code changes regenerate goldens with the repo's `-update` flag and the diff is read,
  not waved through.
- Scoped tests per task; `go test ./...` once before each PR is opened. The CHANGELOG entry is
  added in PR 1 under Unreleased → Added and extended by each later PR.
- After each merge: `go install ./cmd/alder`; after a grammar change also rebuild the parser for
  alder.nvim (see memory `alder-reinstall-after-merge`).

## Review Focus

Inputs the spec implies that no headline test would meet. Each has a test in the task named.

1. **A job's parameter type changed between two builds** (field renamed, `Int` became `Text`). The
   old row goes `dead` with `cannot decode the parameters: …`; it is not dropped, and the worker
   keeps running the other rows. Task 4.
2. **A route that enqueues and then fails** (`Task.fail` after `Job.enqueue`). No row exists
   afterwards, in the server and in a test. Task 5.
3. **A job whose `run` panics on every attempt.** Each attempt is logged at error with the stack,
   the row is retried with the backoff and goes dead after the fifth, and the worker goroutine
   survives to run the next row. Task 8.
4. **The app is down across several ticks** (a nightly job, three days off). One late run on
   start, then the next run on the grid; `Test.work` after `Test.advance (3 * 86400000)` runs it
   once. Task 12.
5. **`enqueueOnce` with a key whose last job is `done` or `dead`.** A new row is added with a new
   id; with a `queued` or `running` row the existing id comes back and no row is added; the same
   key on two kinds gives two rows. Task 3.

## Facts about the code (from the survey; check before relying on one)

| What | Where |
|---|---|
| keywords, decl starts, `route` parse | `internal/syntax/token.go:72`, `parser.go:247`, `parser.go:400` |
| route AST, IR, IR dump | `internal/syntax/ast.go:154`, `internal/ir/ir.go:303`, `internal/ir/dump.go:131` |
| `every` grammar | `internal/syntax/every.go:9` (`EveryDuration`) |
| formatter | `internal/format/print_decl.go:264` (`routeLines`), grouping `:73-76` |
| route checks, missing `update` | `internal/check/routes.go:176-207`, `:289` |
| E0026 report, E0031 write sets | `internal/check/rules.go:262-272`; `internal/check/writeset.go:56` (`dbWrites`), `:28` |
| E0046 removed keywords | `internal/check/removed.go` |
| route value | `rt/route.go:9`; generated by `routeLit` `internal/gen/expr.go:646` |
| JSON value | `rt/json.go:26` (`JsonValue`), `:52` (`JsonEncode`); a JSON codec precedent: `sessionCodec` `internal/gen/caps.go:1031` |
| stdlib → Go | `internal/gen/stdcall.go:18` (`StdCalls`), `:313` (`Effectful`) |
| effects | `rt/eff/eff.go:45` (`Task`), `:158` (`Call`), `:173` (`Env`), `:245` (`Transaction`), `:263` (`Within`), `:386` (`Recorder`); `rt/eff/run.go:49` (`Run`) |
| router env, panics, `Now` | `rt/web/router.go:976` (`newEnv`), `:578`, `:41` |
| internal tables, `Ensure`, `_alder_meta` | `rt/db/sqlite.go:774`, `:590`, `rt/db/migrate.go:857` |
| silos | `rt/db/stores.go`: `IdleClose :108`, `Stores :118`, `OpenStores :188`, `NewTestStores :212`, `Enter :1108`, `Narrow :1136`, `SiloFiles :1261`, `StatusSilos :1290` |
| generated `Serve`, `Options`, `main`, `--migrate-only` | `internal/gen/caps.go:434`, `:406`, `:714`/`:780`, `:876`; goldens `testdata/golden/*/want/app__alder.go` |
| test clock, test transaction | `rt/testing/clock.go`, `rt/testing/testing.go:93`, `rt/testing/browser.go:238` |
| dev panel, event feed, metrics, health | `rt/web/devrequests.go`; `cmd/alder/run_events.go:61`; `rt/metrics/metrics.go:226-248`; `rt/web/router.go:783` |
| Render deploy | `internal/check/deploy.go:165-203`; `internal/deploy/init.go:86`; `internal/deploy/render.go:49,620,1086`; `internal/deploy/render_api.go:334`; `cmd/alder/deploy.go:700` |
| bench harness | `alder bench` (`cmd/alder/bench.go`, `rt/testing/bench.go`, fixture `testdata/bench/rows`); baseline `docs/perf/2026-10-bench-baseline.md` |

Two spec sentences do not match the code; the plan follows the code:

- The spec says `_alder_lock` is created at open. It is created lazily in `TakeLock`
  (`rt/db/stores_turso.go:407-413`). `_alder_jobs` is created at open (Task 3) and by
  `--migrate-only`; nothing copies `_alder_lock`'s path.
- `alder dump` dumps compiler stages, not a database. `alder jobs` (Task 18) opens the database
  file the way `cmd/alder/silos.go` does.
- The event feed has no request event: requests arrive as `log` events carrying access records.
  A job's attempt line (Task 9) arrives the same way, which is the spec's "`job` events".

---

## PR 1: declaration, `Job` module, table, enqueue, `Test.jobs`/`Test.work`

### Task 1: Parse and format `job`

Spec §1 (syntax only).

**Files:**
- Modify: `internal/syntax/token.go`, `internal/syntax/parser.go`, `internal/syntax/ast.go`,
  `internal/syntax/every.go`, `internal/format/print_decl.go`
- Modify if its drift test demands it: `tree-sitter-alder/grammar.js` (a `job_decl` rule beside
  `route_decl` at `:317`), regenerated `src/`
- Test: `internal/syntax/parser_test.go`, `internal/syntax/every_test.go`,
  `internal/format/format_test.go` (or the package's golden files, as the route tests do)

**Interfaces:**
- Produces:

  ```go
  // internal/syntax/ast.go
  type JobDecl struct {
      Name     string
      Params   *RecordType
      Every    *StringLit // nil when absent; the literal, unparsed
      Cron     *StringLit
      Retries  *IntLit
      Timeout  *StringLit
      Silo     string     // "" when absent
      Options  []JobOption // in source order, for the formatter and for "at most once" errors
      Pos      Pos
  }
  type JobOption struct { Name string; Pos Pos }
  ```
  `EveryDuration` accepts `h` and `d` units in addition to `ms`, `s`, `m`.

- [ ] **Step 1: Write the failing tests.** `TestParse_JobDecl`: each of the five lines in spec §1's
  first code block parses to a `JobDecl` with the fields set (`retries 0` → `Retries` 0, `silo
  org` → `Silo` "org"); options in any order parse. `TestParse_JobDecl_Errors`: an unknown
  option (`job A = {} weekly`) and a repeated one (`retries 1 retries 2`) are parse errors naming
  the option. `TestEveryDuration_HoursDays`: `"1h"` = 1 h, `"2d"` = 48 h, existing cases
  unchanged. `TestFormat_Job`: the spec's block is a fixed point, and `job   A={}  retries 2`
  formats to `job A = {} retries 2`.
- [ ] **Step 2: Run** `go test ./internal/syntax ./internal/format -run 'Job|EveryDuration'`;
  expected FAIL (unknown keyword `job`).
- [ ] **Step 3: Implement.** Add `job` to `keywords` and `declKeywords`; parse beside the `route`
  case. Options are contextual words after the record, not new keywords (`every` and `silo` are
  already words in the language; `cron`, `retries`, `timeout` must stay usable as identifiers
  elsewhere). Format like `routeLines`, one declaration per line, options in source order.
- [ ] **Step 4: Run** the Step 2 command, then `go test ./internal/syntax/... ./internal/format/...
  ./tree-sitter-alder/...`; expected PASS. If the tree-sitter drift test fails on the keyword,
  add the grammar rule now (highlight queries wait for Task 20).
- [ ] **Step 5: Commit** `syntax: job declarations (#168)`.

### Task 2: Check `job` and its arm

Spec §1 (checker rules). Cron *expressions* are checked in Task 11; here `cron "…"` checks only
that the literal is present and not combined wrongly.

**Files:**
- Create: `internal/check/jobs.go`
- Modify: `internal/check/check.go` (call it beside the route checks), `internal/check/rules.go`
  (E0026 in `run`), `internal/check/writeset.go` (no change expected; test only),
  `internal/diag/diag.go`, `internal/diag/explain.go`, `internal/ir/ir.go`, `internal/ir/dump.go`,
  the lowering file that lowers routes
- Test: `internal/check/jobs_test.go`

**Interfaces:**
- Consumes: `syntax.JobDecl` (Task 1).
- Produces: `diag.JobParam` = E0049, `diag.JobSchedule` = E0050; `ir.Job{Name, Module string;
  Params ir.RecordType; Run *ir.Func; Retries int; Timeout time.Duration; Every time.Duration;
  Cron string; SiloParam string}` with defaults filled (Retries 4, Timeout 5 min); the checker
  type `Job` (a nominal type, comparable) and the constructor `Name : Name.Params -> Job` in the
  module's scope, `Name.Params` as a type.

- [ ] **Step 1: Write the failing tests**, each a small module source checked with the package's
  existing helper, asserting the code and a substring of the message:
  - `TestJob_OK`: the spec's opening `Signup` module checks with no diagnostics.
  - `TestJob_MissingRun`: E0043, message contains `job SendWelcome` and `run`.
  - `TestJob_RunWrongType` (`run : Params -> Task Int`): E0043.
  - `TestJob_ParamNotStorable`: one case each for a function, `Task`, `Query`, `Html`, `Job`,
    `Route` field → E0049 naming the field and the type; `List (Maybe User.UserId)`, a record, a
    brand, an app custom type without functions → no error.
  - `TestJob_Schedules`: `every` + `cron` → E0050 "A job has two schedules"; `every "1h"` with a
    non-empty record → E0050 "A scheduled job takes no parameters"; `every "500ms"` → E0050
    (below `1s`); `every "3x"` → E0050 naming what it could not read.
  - `TestJob_Options`: `timeout "2h"` → E0050 (above `1h`); `retries -1` → error; `silo org`
    where `org` is not a field, or has the wrong type, or the app has no silos → error with the
    form rule's wording.
  - `TestJob_ScopedDbInRun`: `Db.insertMine` in `run` → E0026; `explain E0026` contains "A
    job's `run` has no session either".
  - `TestJob_NoSession`: a `run` that names `session` → unknown name.
  - `TestJob_EnqueueNotAWrite`: a `requires org` route that only calls `Job.enqueue` and one that
    `Db.insert`s a project and enqueues both compile (no E0031).
- [ ] **Step 2: Run** `go test ./internal/check -run TestJob`; expected FAIL.
- [ ] **Step 3: Implement** `checkJobs` in `internal/check/jobs.go`. The storable rule is the table
  column rule plus records, lists and `Maybe` of storable types and app types whose constructors'
  arguments are storable (walk recursively, stop on cycles). `Job.enqueue` stays out of
  `dbWrites`. Lower to `ir.Job`; add it to the IR dump.
- [ ] **Step 4: Run** `go test ./internal/check/... ./internal/ir/... ./internal/diag/...`;
  expected PASS.
- [ ] **Step 5: Commit** `check: job declarations, E0049, E0050 (#168)`.

### Task 3: `rt/jobs` core: table, enqueue, status, `RunOne`

Spec §2 (runtime half), §3 Run and Failure, §5, §6.

**Files:**
- Create: `rt/jobs/table.go`, `rt/jobs/enqueue.go`, `rt/jobs/run.go`, `rt/job.go` (the value)
- Modify: `rt/db/sqlite.go` (prefix rule; create the table where `_alder_meta` is), `rt/db/migrate.go`
  (`jobs_schema` row), `internal/gen/caps.go` (`maintain` creates it under `--migrate-only`)
- Test: `rt/jobs/enqueue_test.go`, `rt/jobs/run_test.go`, `rt/db/sqlite_test.go`

**Interfaces:**
- Produces:

  ```go
  // rt/job.go
  type Job struct{ Kind, Params string } // Kind "Signup.SendWelcome"; Params encoded JSON

  // rt/jobs
  const Table = "_alder_jobs"
  func EnsureTable(ctx context.Context, x Execer) error // the spec §5 DDL + indexes + jobs_schema = 1
  type Row struct {
      ID, Kind, Params, State, Worker string
      Key                                 *string
      RunAt, Created                      int64
      Started, Finished, LeaseUntil, ScheduledFor *int64
      Attempts                            int
      Error                               *string
      Schedule                            bool
  }
  type Kind struct {
      Name      string
      Run       func(env *eff.Env, params string) (eff.Task[struct{}], error) // error = decode failure
      Retries   int
      Timeout   time.Duration
      Every     time.Duration // 0 when not scheduled by interval
      Cron      string        // "" when not scheduled by cron
      SiloParam string        // "" when the job runs on the shared store
  }
  func Enqueue(env *eff.Env, j rt.Job, runAt time.Time, key *string) (string, error)
  type State string // "queued" | "running" | "done" | "dead"
  type Status struct{ State State; Attempts int; Error *string; RunAt *time.Time }
  func StatusOf(env *eff.Env, id string) (*Status, error)
  type Outcome struct {
      ID, Kind  string
      Err       error          // nil when done
      Dead      bool
      Next      time.Time      // the retry's run_at when Err != nil && !Dead
      Took      time.Duration
      LockHeld  time.Duration  // how long the attempt's write transaction was open
  }
  func Backoff(attempt int) time.Duration
  func RunOne(env *eff.Env, k Kind, row Row) Outcome
  ```
  `Execer` is whatever `Ensure` already executes DDL through in `rt/db` (a `*sql.DB`/`*sql.Tx`
  shaped interface); reuse it rather than defining a second.

- [ ] **Step 1: Write the failing tests** against an in-memory store and `rtesting.NewClock`:
  - `TestEnsureTable_Idempotent`: twice on one store, no error; `jobs_schema` is `1`.
  - `TestAppTables_IgnoresAlderPrefix`: a store with `_alder_jobs` and an invented
    `_alder_future` has no drift and neither is in `appTables`.
  - `TestEnqueue_InCallersTransaction`: enqueue inside a Task that then fails → zero rows; inside
    one that succeeds → one `queued` row, `run_at` = clock now.
  - `TestEnqueueOnce` (Review Focus 5): same kind + key while queued → same id, one row; after
    the row is `done` → new id; after `dead` → new id; same key on another kind → second row.
  - `TestEnqueue_ParamsTooLarge`: 65537 bytes → `*ErrFail` (the error type `Task.fail` produces)
    with a message naming 64 KB; 65536 → ok.
  - `TestStatus`: queued row → `Queued`, attempts 0; unknown id → nil.
  - `TestRunOne_Done`: a Task that inserts a row → table row and job row `done` committed
    together; a Task that writes nothing → `done` in a one-statement transaction.
  - `TestRunOne_FailRetries`: fails on attempt 1..4 → `queued`, `run_at` = now + 10 s, 1 min,
    10 min, 1 h, the Task's writes rolled back, `error` set; attempt 5 → `dead`. `retries 0` →
    dead after attempt 1. `retries 6` → attempt 5 and 6 back off 1 h.
  - `TestRunOne_ErrorText`: `Task.fail "x"` → `x`; panic `boom` → first line `boom`; a 5000-byte
    message is stored as 4096 bytes.
  - `TestRunOne_DecodeError` (Review Focus 1): `Kind.Run` returns a decode error → row `dead`
    after this one attempt, error starting `cannot decode the parameters:`, whatever `retries`
    says. Ruling: spec §1 says such a row "goes dead"; retrying a decode with the same build
    cannot succeed, and a build that can decode it would be a newer one, which a dev-panel
    Retry covers.
  - `TestRunOne_Timeout`: `Timeout` 50 ms, a Task that sleeps on the clock's context past it →
    error `timed out after 50ms`.
  - `TestBackoff`: table of attempt → duration, including 7 → 1 h.
- [ ] **Step 2: Run** `go test ./rt/jobs ./rt/db -run 'Ensure|AppTables|Enqueue|Status|RunOne|Backoff'`;
  expected FAIL (package missing).
- [ ] **Step 3: Implement.** The DDL is spec §5 verbatim. `appTables` and the drift check use
  `"name" NOT LIKE '\_alder\_%' ESCAPE '\'` in place of `NOT IN (internalTables)`; delete the
  constant and update its comment. `EnsureTable` runs wherever `createMetaTable` runs and in
  `maintain`. `RunOne` builds `eff.Bind(run, eff.Call("Job.done", true, markDone))` so the done
  mark is the Task's last write, runs it with `eff.Run(env, eff.KTask, …)` under a context with
  the timeout, records the write transaction's open duration in `LockHeld`, and on failure
  writes the attempt's bookkeeping in a second short transaction on `env.Store`. The claim
  (attempts + 1, `running`) is the caller's job (Task 8, Task 5); `RunOne` takes a claimed row.
- [ ] **Step 4: Run** the Step 2 command and `go test ./rt/...`; expected PASS.
- [ ] **Step 5: Commit** `rt/jobs: table, enqueue, RunOne (#168)`.

### Task 4: Generate jobs and the `Job` module

Spec §1 (codec, value), §2.

**Files:**
- Create: `stdlib/Job.alder`, `rt/std/job.go`, `internal/gen/jobs.go`
- Modify: `internal/gen/stdcall.go`, `internal/gen/expr.go` (constructor beside `routeLit`),
  `internal/gen/caps.go` (emit `Jobs()`), `internal/gen/types.go` (`Job`, `Job.Id`, `Job.State`,
  `Job.Status`), goldens
- Test: `internal/gen/jobs_test.go`, a golden app under `testdata/golden/` with the spec's
  `Signup` module, `book/stdlibdocs_test.go` (no edit expected; it must pass)

**Interfaces:**
- Consumes: `ir.Job` (Task 2); `rt.Job`, `jobs.Kind`, `jobs.Enqueue`, `jobs.StatusOf` (Task 3).
- Produces: generated per job `encode<Mod>_<Name>(p) rt.JsonValue` and `decode<Mod>_<Name>(v
  rt.JsonValue) (P, error)`; generated `func Jobs() []jobs.Kind` in the app package (empty slice
  in an app without jobs); `std.JobEnqueue`, `std.JobEnqueueAt`, `std.JobEnqueueOnce`,
  `std.JobStatus` as `eff.Call`s (`enqueue*` with `write = true`, `status` with `write = false`).

- [ ] **Step 1: Write the failing tests.**
  - `TestJobCodec_RoundTrip` (Go, over generated code in the golden app's test): a params record
    with `Int`, `Text`, a brand, `Maybe` both ways, a `List`, a nested record and a custom type
    with arguments round-trips; the encoding of the custom type is `{"tag":"T","args":[…]}`, a
    brand is its base, `Nothing` is `null`.
  - `TestJobCodec_Drift` (Review Focus 1): decoding `{"user": 3}` where `user` is `Text`, and `{}`
    where `user` is required, both return an error naming the field.
  - `TestJobValue_Eq`: two constructors with equal params give equal `rt.Job`; `Show` gives the
    kind and params.
  - The golden for the `Signup` app; `go test ./internal/gen -run Golden -update` writes it and
    the reviewer reads the `Jobs()` and codec output.
  - `stdlib/Job.alder` with #119 docs on every declaration: `TestStdlibDocs` passes. Examples
    that need a job declaration run in `examples/todos`, which gains
    `app/Jobs/Nightly.alder` = `job Nightly = {}` with a `run` that logs (Task 12 adds its
    `cron`). `status` and `enqueue*` get usage examples, `Job.Id`/`State`/`Status` prose.
- [ ] **Step 2: Run** `go test ./internal/gen ./book -run 'Job|Golden|StdlibDocs'`; expected FAIL.
- [ ] **Step 3: Implement.** Model the codec on `sessionCodec` (`internal/gen/caps.go:1031`), one
  pair per job, over `rt.JsonValue`. The constructor builds `rt.Job{Kind, Params:
  JsonEncode(encode(p))}`. `Kind.Run` decodes, then calls the generated `run` arm. Map the four
  calls in `StdCalls` and mark them `Effectful`.
- [ ] **Step 4: Run** `go test ./internal/gen/... ./book/... ./rt/std/...`; expected PASS.
- [ ] **Step 5: Commit** `gen: job values, codecs, Jobs(), Job module (#168)`.

### Task 5: `Test.jobs` and `Test.work`

Spec §8 (`alder test`). Schedule ticks in `Test.work` come in Task 12; silos in Task 13.

**Files:**
- Create: `rt/testing/jobs.go`
- Modify: `stdlib/Test.alder` (two signatures with #119 docs), `internal/gen/stdcall.go`,
  `rt/eff` or `rt/testing/trace.go` (so `Test.trace` lists `Job.enqueue SendWelcome`)
- Test: `rt/testing/jobs_test.go`, a golden or `testdata` app whose Alder tests are the spec's
  opening test and the ones below

**Interfaces:**
- Consumes: `jobs.RunOne`, `jobs.Kind`, `Jobs()` (Tasks 3–4).
- Produces: `rtesting.Jobs(kinds []jobs.Kind) eff.Task[[]JobRow]` and `rtesting.Work(kinds
  []jobs.Kind) eff.Task[[]JobRun]`, where `JobRow{ID string; Job rt.Job; Status jobs.Status}`
  and `JobRun{ID string; Job rt.Job; Result rt.Result[string, struct{}]}` (use the repo's Go
  representation of `Result`).

- [ ] **Step 1: Write the failing tests** (Alder tests in the test app, run by `alder test` in a Go
  test the way existing `rt/testing` behaviour is pinned):
  - The spec's "signing up sends the welcome mail" test passes.
  - `work runs nothing that is not due`: `Job.enqueueAt` one minute ahead, `Test.work` → `[]`;
    `Test.advance 60000`, `Test.work` → one `Ok`.
  - `a failing job is retried on the clock`: a job that `Task.fail`s; first `Test.work` → one
    `Err`; `Test.jobs` shows `Queued`, attempts 1, `runAt` = now + 10 s; its writes before the
    failure are gone.
  - `jobs that enqueue jobs run in the same work`: A enqueues B → `Test.work` returns A then B.
  - `a job that enqueues itself forever`: the test fails with `Test.work: jobs keep enqueueing
    jobs`.
  - Review Focus 2: a route that `Job.enqueue`s then `Task.fail`s, submitted with `Test.submit`
    → `Test.jobs` is `[]`.
  - `Test.trace` of a `Test.run` that enqueues contains `Job.enqueue SendWelcome`.
- [ ] **Step 2: Run** the test app's Go test (`go test ./rt/testing -run Jobs`); expected FAIL.
- [ ] **Step 3: Implement.** `Work` selects due rows at `env.Now()` oldest first (`run_at`,
  `created`), claims each (attempts + 1) and calls `RunOne` with an `Env` that is the test's,
  `Within` its transaction with a savepoint per job; loop while the last pass ran something,
  stop with the cap error at 1000 runs.
- [ ] **Step 4: Run** `go test ./rt/testing/... ./internal/gen/...`; expected PASS.
- [ ] **Step 5: Commit** `test: Test.jobs and Test.work (#168)`.

### Task 6: PR 1 close

- [ ] Add the CHANGELOG entry under Unreleased → Added: job declarations, `Job` module,
  `Test.jobs`/`Test.work`, E0049/E0050 (jobs do not run outside tests yet).
- [ ] `go test ./...`; expected PASS. Open PR 1 ("jobs 1/7"), body lists rulings made.

---

## PR 2: the worker

### Task 7: Split `Serve`; `ALDER_JOBS`

Spec "What is left to the builder" (the `Serve` split), §3 opening.

**Files:**
- Modify: `internal/gen/caps.go` (`Options :406`, `Serve :434`, `Main :714`), goldens, the
  generated-main doc comment (`:683-757`, which lists every `ALDER_*`)
- Test: `internal/gen/caps_test.go` or the golden tests; an `rt/web` or app-level test that
  serves through `Open`

**Interfaces:**
- Produces, in the generated app package:

  ```go
  type App struct{ /* store(s), caps, log, now, handler */ }
  func Open(caps Caps, o Options) (*App, error)
  func (a *App) Handler() http.Handler
  func (a *App) StartJobs(n int) (stop func(grace time.Duration))
  func (a *App) Close() error
  func Serve(caps Caps, o Options) (http.Handler, error) // = Open + Handler; existing callers unchanged
  ```
  `Options.Jobs int` (from `ALDER_JOBS`, default 4; a non-integer or negative value exits 2 with
  a message, as the other `ALDER_*` reads do). `App.JobEnv(ctx, kind) *eff.Env` builds an Env as
  `router.newEnv` does (`rt/web/router.go:976`) with Session nil and the router's `Now`.

- [ ] **Step 1: Write the failing test** `TestMain_StartsJobs` over the golden `main`: it calls
  `Open`, `Handler`, `StartJobs(o.Jobs)`, and on shutdown `srv.Shutdown(grace)`, then the jobs'
  stop, then `stopBackup()`, then `Close`; the `maintain` (`--migrate-only`) path calls no
  `StartJobs`. `TestOptions_AlderJobs`: unset → 4, `0` → 0, `x` →
  exit 2.
- [ ] **Step 2: Run** `go test ./internal/gen -run 'Main|AlderJobs|Golden'`; expected FAIL.
- [ ] **Step 3: Implement.** In this task `StartJobs` returns a stop that does nothing; Task 8
  makes it call `jobs.Start`. `--migrate-only` and the other maintenance runs never call
  `StartJobs`.
- [ ] **Step 4: Run** `go test ./internal/gen/... ./rt/web/...` and regenerate goldens; expected
  PASS with only the intended diff.
- [ ] **Step 5: Commit** `gen: split Serve into Open/Handler/StartJobs/Close (#168)`.

### Task 8: The worker loop

Spec §3 (Workers, Claim, Lease, Shutdown, Polling and wake, Housekeeping, Development).

**Files:**
- Create: `rt/jobs/worker.go`, `rt/jobs/claim.go`, `rt/jobs/housekeep.go`
- Modify: `rt/jobs/enqueue.go` (signal the wake channel after commit), `cmd/alder/run.go` only if
  the child swap needs a change (expected none: stdin EOF already drives shutdown)
- Test: `rt/jobs/worker_test.go`

**Interfaces:**
- Produces:

  ```go
  type Options struct {
      Store   db.Store          // the shared store (Task 13 adds the silos)
      Kinds   []Kind
      Env     func(ctx context.Context, k Kind) *eff.Env
      Workers int
      Now     func() time.Time
      Log     *applog.Logger
      OnAttempt func(Outcome)   // Task 9's log/metrics/panel hook; nil = none
      Poll, Housekeep time.Duration // 1 s, 1 min; tests shorten them
  }
  func Start(ctx context.Context, o Options) (stop func(grace time.Duration))
  func Claim(ctx context.Context, s db.Store, kinds []string, now time.Time, lease time.Duration, worker string) (*Row, error)
  func Wake() // called after an enqueue's commit
  ```
  Use the store type the router holds (`db.Store` or what `Env.Store` is); worker id is
  `"<host>:<pid>:<n>"`.

- [ ] **Step 1: Write the failing tests**, on a file-backed temporary SQLite store with a
  controllable clock and `Poll` of a few ms:
  - `TestClaim_Atomic`: 2 goroutines × 50 claims over 50 due rows → each row claimed exactly
    once.
  - `TestClaim_SkipsUnknownKinds` and `TestClaim_NotBeforeRunAt`.
  - `TestWorker_RunsEnqueued`: an enqueue committed → `done` without waiting for the poll (poll
    set to 1 h, wake does it).
  - Review Focus 3, `TestWorker_PanicSurvives`: a kind that panics → error-level line with a
    stack, row retried; a second, healthy row enqueued after it is `done`; after 5 attempts the
    first is `dead`.
  - `TestWorker_LeaseExpired`: a `running` row with `lease_until` in the past → housekeeping
    re-queues it with `lost: the process stopped`, attempts unchanged (that attempt counted).
  - `TestWorker_ShutdownRequeues`: a job blocked on its context; `stop(10ms)` → row `queued`,
    attempts back to its value before the claim, `error` NULL; nothing claimed after `stop`.
  - `TestHousekeep_Prune`: done 8 days old and dead 31 days old deleted; done 6 days and dead 29
    kept.
  - `TestHousekeep_Unclaimable`: a queued row of an unknown kind due 25 h ago → one warn line
    naming the kind.
  - `TestWorker_ZeroWorkers`: `Workers: 0` → `Start` returns a no-op stop and runs nothing.
- [ ] **Step 2: Run** `go test ./rt/jobs -run 'Claim|Worker|Housekeep' -race`; expected FAIL.
- [ ] **Step 3: Implement.** The claim is spec §3's statement verbatim in one `BEGIN IMMEDIATE`.
  Housekeeping runs on one worker at a time per process (a mutex), at start and every
  `Housekeep`. Wake is a buffered channel of one; `Enqueue` registers a commit hook on the
  `Env` (or the transaction) so the signal is sent only after commit.
- [ ] **Step 4: Run** the Step 2 command; expected PASS, no race reports.
- [ ] **Step 5: Commit** `rt/jobs: worker loop, claim, lease, housekeeping (#168)`.

### Task 9: Log, metrics, health, the write-lock mark

Spec §9 (Log, Metrics, Health), §6a (the mark).

**Files:**
- Modify: `rt/jobs/worker.go` (`OnAttempt` wiring), `rt/applog` (a job attempt record shaped like
  `applog.Access`, so the dev event feed carries it as a `log` event), `rt/metrics/metrics.go`,
  `rt/web/router.go` (`health :783`)
- Test: `rt/jobs/observe_test.go`, `rt/metrics/metrics_test.go`, `rt/web/health_test.go`

**Interfaces:**
- Consumes: `jobs.Outcome` (Task 3), `Options.OnAttempt` (Task 8).
- Produces: metrics `alder_jobs_total{kind,outcome}` (outcome `done|failed|dead`),
  `alder_jobs_running`, `alder_jobs_queued`, `alder_job_duration_seconds{kind}` (same buckets as
  `alder_http_request_duration_seconds`); health JSON gains `"jobs": "ok"|"off"|"stalled"`.

- [ ] **Step 1: Write the failing tests.**
  - `TestJobLog_Lines`: done → info `job Signup.SendWelcome <id> done <ms>ms attempt 1`; failure
    → warn containing `failed attempt 2/5 "smtp: timeout", next in 1m`; dead → error. `Log.info`
    inside the job carries the job's id.
  - `TestJobLog_LockHeld`: `Outcome.LockHeld` 2.1 s → the line contains `held the write lock 2 s:
    write last, or fan out — book 17`; 1.9 s → no mark.
  - `TestMetrics_Jobs`: after one done and one failed attempt the exposition has the four
    series with those labels.
  - `TestHealth_Jobs`: `ALDER_JOBS=0` → `"off"`; a due row 6 min old and no claim in 6 min →
    `"stalled"` with status code unchanged (200); otherwise `"ok"`.
- [ ] **Step 2: Run** `go test ./rt/jobs ./rt/metrics ./rt/web -run 'JobLog|Metrics_Jobs|Health_Jobs'`;
  expected FAIL.
- [ ] **Step 3: Implement.** The worker records its last claim time for health. The panel half
  of the mark is Task 17.
- [ ] **Step 4: Run** the Step 2 command; expected PASS.
- [ ] **Step 5: Commit** `rt/jobs: log lines, metrics, health, write-lock mark (#168)`.

### Task 10: Throughput benchmark; PR 2 close

Owed by the spec review: the spec's throughput figure is River's, not ours. `alder bench`
measures one browser step inside a test transaction, so it gives the per-request cost of an
enqueue but cannot measure concurrent claiming; throughput is a Go benchmark recorded beside the
existing baseline.

**Files:**
- Create: `rt/jobs/bench_test.go`, `docs/perf/2026-11-jobs-throughput.md`
- Modify: `testdata/bench/rows` (a route that inserts and enqueues, and a task test that submits
  it)

- [ ] **Step 1: Write `BenchmarkJobs`** in `rt/jobs/bench_test.go` on a file-backed WAL SQLite
  store opened the way the app opens it (same pragmas, `busy_timeout` 5 s), with real time:
  - `claim-finish/workers=W` for W in 1, 4, 8: `b.N` queued no-op jobs that each insert one row;
    report `jobs/s` with `b.ReportMetric`.
  - `with-requests/workers=4/writers=R` for R in 4, 16: R goroutines run request-shaped write
    transactions (one `BEGIN IMMEDIATE`, one insert, commit) in a loop while the workers drain
    `b.N` jobs; report `jobs/s`, request writes/s, and request write p50 and p99 in ms.
  - `requests-only/writers=R`: the same writers with no workers, as the baseline for the p99.
- [ ] **Step 2: Add the `alder bench` row**: in `testdata/bench/rows`, a route that inserts a row
  and `Job.enqueue`s, and a task test `enqueue 1000` that submits it; compare its row with `add
  1000`.
- [ ] **Step 3: Run on an idle machine** (`uptime` load average under 2 before and after; record
  both), foreground, under a timeout:

  ```bash
  go test ./rt/jobs -run '^$' -bench BenchmarkJobs -benchtime=2000x -count 5 -timeout 20m
  alder bench testdata/bench/rows -run 'add 1000|enqueue 1000' --count 10
  ```
  Expected: both complete; no `SQLITE_BUSY` failures in the request writers.
- [ ] **Step 4: Write `docs/perf/2026-11-jobs-throughput.md`** in the baseline file's form
  (machine, Go version, commit, `uptime`, tables, the medians), and one paragraph stating what
  the numbers say about the spec's claims: whether one-second polling and four workers keep up
  with a request load, and how much a job load raises request write p99. If the p99 rise or
  `SQLITE_BUSY` contradicts §6a or §7, say so in the PR body and stop for the user's ruling
  before PR 3. Book chapter 17 (Task 21) quotes this file's number, not River's.
- [ ] **Step 5: CHANGELOG** (jobs now run in the app's process; `ALDER_JOBS`; metrics; health);
  `go test ./...`; open PR 2 ("jobs 2/7").
- [ ] **Step 6: Commit** `perf: job throughput benchmark and record (#168)`.

---

## PR 3: schedules

### Task 11: The cron parser and its checks

Spec §1 (`cron` grammar), §4 (`next`).

**Files:**
- Create: `rt/jobs/cron/cron.go`
- Modify: `internal/check/jobs.go` (E0050 on a malformed expression)
- Test: `rt/jobs/cron/cron_test.go`, `internal/check/jobs_test.go`

**Interfaces:**
- Produces:

  ```go
  package cron
  type Schedule struct{ /* minute, hour, dom, month, dow bitsets */ }
  func Parse(expr string) (Schedule, error) // error names the field: "minute", "hour", "day of month", "month", "day of week"
  func (s Schedule) Next(after time.Time) time.Time // first matching minute strictly after `after`, UTC
  ```

- [ ] **Step 1: Write the failing tests.** `TestParse`: `0 3 * * *`, `*/15 * * * *`,
  `0 9-17 * * 1-5`, `0 0 1,15 * *` parse; four or six fields, `60`, `MON`, `L`, `5/0`, `3-1`
  fail naming the field. `TestNext`: `0 3 * * *` after 02:59 → 03:00 same day; after 03:00 →
  03:00 next day; `0 0 29 2 *` after 2026-03-01 → 2028-02-29; day-of-month and day-of-week both
  restricted follow the standard cron OR rule (document it in the package comment). Non-UTC
  input is converted to UTC. `TestJob_CronMalformed`: `cron "0 25 * * *"` → E0050 naming
  `hour`.
- [ ] **Step 2: Run** `go test ./rt/jobs/cron ./internal/check -run 'Parse|Next|Cron'`; expected
  FAIL.
- [ ] **Step 3: Implement** (about a hundred lines; no dependency). The checker imports
  `rt/jobs/cron` (precedent: `internal/check/deploy.go` imports `rt/applog`).
- [ ] **Step 4: Run** the Step 2 command; expected PASS.
- [ ] **Step 5: Commit** `rt/jobs/cron: five-field cron parser (#168)`.

### Task 12: Schedule ticks

Spec §4, §8 (ticks in tests).

**Files:**
- Create: `rt/jobs/schedule.go`
- Modify: `rt/jobs/worker.go`, `rt/jobs/housekeep.go`, `rt/testing/jobs.go`,
  `examples/todos/app/Jobs/Nightly.alder` (add `cron "0 3 * * *"` and a test)
- Test: `rt/jobs/schedule_test.go`, Alder tests in the test app

**Interfaces:**
- Consumes: `cron.Schedule` (Task 11), `jobs.Kind.Every`/`Cron`.
- Produces: `func Next(k Kind, after time.Time) time.Time` (cron's `Next`, or the interval rule:
  `after + every` rounded down to the interval when the interval divides 24 h, else `after +
  every`); `func EnsureTicks(ctx, s db.Store, kinds []Kind, now time.Time) error` (`INSERT OR
  IGNORE` one queued tick per scheduled kind).

- [ ] **Step 1: Write the failing tests.**
  - `TestNext_Every`: `1h` after 10:17 → 11:00; `7m` (does not divide a day) after 10:17 →
    10:24; `1d` after 10:17 → next 00:00 UTC; `90s` (divides a day) after 10:17:10 → 10:18:00.
  - `TestEnsureTicks_OnePerKind`: called twice and from two goroutines → one queued tick row.
  - `TestClaimTick_InsertsNext`: claiming a tick inserts the next with `scheduled_for =
    Next(max(scheduled_for, now))`.
  - Review Focus 4, `TestTicks_Outage`: nightly tick for day 1 03:00 left queued, clock moved to
    day 4 10:00 → one claim runs it, the next row is day 5 03:00.
  - `TestTick_RetryKeepsScheduledFor` and `TestTick_DeadRetryLeavesNextAlone`.
  - `TestHousekeep_UnknownTick`: a queued tick of an unknown kind older than a day is deleted; a
    non-tick row of an unknown kind is not.
  - Alder: `Test.setClock` to 02:59 UTC, `Test.advance 120000`, `Test.work` → `Nightly` ran once;
    `Test.advance (3 * 86400000)`, `Test.work` → once more, not three times.
  - `alder test examples/todos` passes.
- [ ] **Step 2: Run** `go test ./rt/jobs ./rt/testing -run 'Next|Tick|Outage|Schedule'`; expected
  FAIL.
- [ ] **Step 3: Implement.** Ticks are created at worker start and every housekeeping tick; in
  tests by the first `Test.work` of a case. CHANGELOG line; `go test ./...`; open PR 3.
- [ ] **Step 4: Run** the Step 2 command and `alder test examples/todos`; expected PASS.
- [ ] **Step 5: Commit** `rt/jobs: schedule ticks (#168)`.

---

## PR 4: silos and Turso

### Task 13: Run store, row store, done mark across stores

Spec §7 (Row store, Run store), §6 (the window), §8 (silos in tests).

**Files:**
- Modify: `rt/jobs/run.go`, `rt/jobs/worker.go`, `rt/testing/jobs.go`, `internal/gen/jobs.go`
  (`SiloParam` and the `Keys` from it), the generated `JobEnv`
- Test: `rt/jobs/silo_test.go`, Alder tests in a siloed test app (reuse the one silo tests use)

**Interfaces:**
- Consumes: `db.Narrow`, `db.Enter`, `db.NewTestStores` (`rt/db/stores.go`).
- Produces: `Options.Stores` (the `*db.Stores` when the app has silos); `RunOne` does the done
  mark as a separate transaction `WHERE state = 'running' AND id = ?` when the row's store is not
  the run's store.

- [ ] **Step 1: Write the failing tests.**
  - `TestEnqueue_LandsInSilo`: a `requires org` route enqueues → the row is in that org's silo
    file, not the shared store.
  - `TestRun_SiloJob`: `job Digest = { org: Org.OrgId } silo org` enqueued on the shared store
    runs bound to that org's silo (its `Db.insert` lands there) and the shared row is `done`.
  - `TestDoneMark_NoResurrect`: between the run's commit and the done mark, the row is
    re-queued by a lease expiry → the done mark changes nothing and the re-queued row stays.
  - Alder: in a siloed test app, `Test.work` runs a row from a test silo and one from the shared
    store, oldest first across them; `Test.jobs` lists both.
- [ ] **Step 2: Run** `go test ./rt/jobs ./rt/testing -run 'Silo|DoneMark'`; expected FAIL.
- [ ] **Step 3: Implement.** Until Task 14, the worker polls the shared store and the silos
  `Stores` has open.
- [ ] **Step 4: Run** the Step 2 command; expected PASS.
- [ ] **Step 5: Commit** `rt/jobs: silo row and run stores (#168)`.

### Task 14: The active set, maintenance silos, Turso

Spec §5 (where the table is created), §7 (Where workers look, Turso).

**Files:**
- Modify: `rt/jobs/worker.go`, `rt/db/stores.go` (create `_alder_jobs` on a silo's open in state
  `ok`; tell `IdleClose` a worker poll is not use), `rt/db/stores_turso.go` (create at open)
- Test: `rt/jobs/active_test.go`, `rt/db/stores_test.go`, the Turso test the repo already runs
  against a local `sqld`/fake, if one exists (otherwise a unit test on the statement sequence)

- [ ] **Step 1: Write the failing tests.**
  - `TestActiveSet_StartScan`: two silo files on disk, one with a due row, neither open → the
    worker runs the row after start (one scan of `SiloFiles`/`StatusSilos`).
  - `TestActiveSet_Enqueue`: a silo touched by an enqueue joins the set; with nothing due for 1 h
    it leaves.
  - `TestIdleClose_IgnoresWorkerPoll`: a silo polled by the worker with nothing queued still
    closes at its idle time.
  - `TestMaintenanceSilo_NotPolled`: a silo in `pending-data-loss` is not polled and gets no
    table.
  - `TestTurso_CreatesJobsTable`: opening a Turso store and running `--migrate-only` both leave
    `_alder_jobs` present.
- [ ] **Step 2: Run** `go test ./rt/jobs ./rt/db -run 'ActiveSet|IdleClose|Maintenance|Turso'`;
  expected FAIL.
- [ ] **Step 3: Implement.** CHANGELOG line; `go test ./...`; open PR 4.
- [ ] **Step 4: Run** the Step 2 command; expected PASS.
- [ ] **Step 5: Commit** `rt/jobs: silo active set, Turso (#168)`.

---

## PR 5: worker machines

### Task 15: `app --jobs-only`

Spec §7a (`--jobs-only`).

**Files:**
- Modify: `internal/gen/caps.go` (`main`: the flag beside `--migrate-only`, its doc comment),
  goldens
- Test: `internal/gen` golden; a process-level test that builds a small app and runs it with
  `--jobs-only` (follow how existing tests run a built app binary)

- [ ] **Step 1: Write the failing tests.**
  - `TestJobsOnly_NoListener`: the process opens no port (`ALDER_ADDR` ignored), runs an
    enqueued job, and exits 0 on SIGTERM after re-queueing a running one.
  - `TestJobsOnly_DefaultWorkers`: `ALDER_JOBS` unset → 8.
  - `TestJobsOnly_Behind`: store schema behind the build → exit 1 with the web build's message.
  - `TestJobsOnly_Ahead`: store ahead → no claims; it polls the schema row every 10 s (clock
    injected) and starts working when the build matches.
- [ ] **Step 2: Run** the new tests; expected FAIL.
- [ ] **Step 3: Implement** using `Open`/`StartJobs`/`Close` (Task 7) without `Handler`.
- [ ] **Step 4: Run** them and the goldens; expected PASS.
- [ ] **Step 5: Commit** `gen: --jobs-only worker mode (#168)`.

### Task 16: `workers:` on the Render target

Spec §7a (`Render … workers`, deploy, status, logs).

**Files:**
- Modify: `internal/check/deploy.go` (`workers` beside `instances :200-203`), `internal/deploy/init.go`
  (`renderYAML :86`), `internal/deploy/render.go` (scale `:620`, wait for the worker deploy,
  status, `Logs :1086` with a service choice), `internal/deploy/render_api.go`,
  `cmd/alder/deploy.go`, `cmd/alder/main.go` (`logs --worker`), `book/16-deploy.md` deploy test
  fixtures if `book/deploy_test.go` pins the Render block
- Test: `internal/check/deploy_test.go`, `internal/deploy/init_test.go`,
  `internal/deploy/render_test.go` (against the existing fake Render API)

- [ ] **Step 1: Write the failing tests.**
  - `TestRender_Workers`: `workers: 2` checks; `workers: 0`, `workers: "2"` → E0030 in the
    `instances` wording.
  - `TestRenderYAML_Workers`: with `workers` the file has the spec's worker service block exactly
    (name `<app>-worker`, `dockerCommand: /app --jobs-only`, the web env vars minus
    `ALDER_ADDR`, no `ALDER_JOBS`) and `ALDER_JOBS=0` on the web service; without `workers` the
    file is byte-identical to today's golden.
  - `TestRenderDeploy_ScalesWorker`: the fake API sees a scale call for the worker service to 2
    and a wait on its deploy status after the web health check.
  - `TestStatus_ListsWorker` and `TestLogs_Worker`: `alder status` shows both services' counts;
    `alder logs <env> --worker` reads the worker service; `--worker` on a target without workers
    is an error naming `workers:`.
- [ ] **Step 2: Run** `go test ./internal/check ./internal/deploy ./cmd/alder -run 'Worker|Render'`;
  expected FAIL.
- [ ] **Step 3: Implement.** File the follow-up issue "Fly with Turso, then `workers:` on Fly".
  CHANGELOG line; `go test ./...`; open PR 5.
- [ ] **Step 4: Run** the Step 2 command; expected PASS.
- [ ] **Step 5: Commit** `deploy: workers on the Render target (#168)`.

---

## PR 6: operating jobs

### Task 17: Dev panel Jobs page

Spec §9 (Dev panel), §6a (the panel half of the mark).

**Files:**
- Create: `rt/web/devjobs.go`
- Modify: the dev panel's nav and routes (where `devrequests.go` is registered), `rt/jobs`
  (keep the last attempt's `Recorder` per job id in a small ring, dev mode only)
- Test: `rt/web/devjobs_test.go`

- [ ] **Step 1: Write the failing tests.** `TestDevJobs_List`: `/_dev/jobs` shows queued with due
  time, running, dead, the last 50 done, and each scheduled kind with its next time.
  `TestDevJobs_Detail`: `/_dev/jobs/{id}` shows params, error, attempts, the trace, and the
  lock-held mark for an attempt above 2 s. `TestDevJobs_Retry`: POST Retry on a dead row → queued,
  attempts 0; Run now on a queued row → `run_at` = now. `TestDevJobs_JSON`: `/_dev/jobs.json`
  carries the same rows. `TestDevJobs_NotInProd`: 404 without dev mode.
- [ ] **Step 2: Run** `go test ./rt/web -run DevJobs`; expected FAIL.
- [ ] **Step 3: Implement** following `devrequests.go`'s markup and JSON style.
- [ ] **Step 4: Run** the Step 2 command; expected PASS.
- [ ] **Step 5: Commit** `dev: jobs page (#168)`.

### Task 18: `alder jobs`

Spec §9 (CLI), §4 (manual run).

**Files:**
- Create: `cmd/alder/jobs.go`
- Modify: `cmd/alder/main.go` (switch and usage), `internal/lsp` command list if it lists CLI
  commands
- Test: `cmd/alder/jobs_test.go`

- [ ] **Step 1: Write the failing tests.** `alder jobs` on a dev database prints counts per state
  and the dead rows (id, kind, attempts, error). `alder jobs retry <id>` and `alder jobs retry
  --dead` reset to queued, attempts 0. `alder jobs run Nightly` enqueues a non-tick row with
  `{}`; `run SendWelcome` (has params) exits 2 naming the parameters. `--db <file>` reads another
  file; an unknown id exits 1.
- [ ] **Step 2: Run** `go test ./cmd/alder -run Jobs`; expected FAIL.
- [ ] **Step 3: Implement**, opening the file the way `cmd/alder/silos.go` does. CHANGELOG line;
  `go test ./...`; open PR 6.
- [ ] **Step 4: Run** the Step 2 command; expected PASS.
- [ ] **Step 5: Commit** `cli: alder jobs (#168)`.

---

## PR 7: generator, editors, book

### Task 19: `alder new job`

Spec §10.

**Files:**
- Create: `internal/generate/templates/job.tmpl`, `internal/generate/job.go`
- Modify: `internal/generate/specs.go`, `internal/generate/specs_test.go` (ID list),
  `cmd/alder` usage strings, the LSP generator list, the generator-count test
  (`TestGenerators_HereFirst` counted generators in #239; update its number)
- Test: `internal/generate/job_test.go`

- [ ] **Step 1: Write the failing tests.** `alder new job SendWelcome user:User.UserId` writes
  `app/Jobs/SendWelcome.alder` equal to spec §10's block; `--into Signup` places it in that
  module; `--every 1h` and `--cron "0 3 * * *"` write a parameterless scheduled job whose test
  sets the clock just before a tick, advances past it and works; `--every` with fields, or both
  flags, is refused with E0050's wording. The generated app passes `alder check` and `alder test`.
- [ ] **Step 2: Run** `go test ./internal/generate -run Job`; expected FAIL.
- [ ] **Step 3: Implement** after `capability.tmpl`'s shape.
- [ ] **Step 4: Run** `go test ./internal/generate/... ./cmd/alder/...`; expected PASS.
- [ ] **Step 5: Commit** `generate: alder new job (#168)`.

### Task 20: tree-sitter, LSP, langdoc

Spec §1 (tooling).

**Files:**
- Modify: `tree-sitter-alder/grammar.js`, `queries/highlights.scm`, regenerated `src/` and
  `parser.so`; `internal/langdoc/langdoc.go` (`job`, `every`, `cron`, `retries`, `timeout`, and
  `silo` in a job); `internal/lsp/scope.go` (job symbols beside the route cases), `internal/lsp/search.go`
  (`symbolKinds`), `internal/search/index.go` (a job is a found declaration)
- Test: `tree-sitter-alder/highlights_test.go`, `drift_test.go`, `internal/langdoc` and `internal/lsp`
  tests

- [ ] **Step 1: Write the failing tests.** Highlighting marks `job` as a keyword and the options as
  keywords in a job line only; hover on `job` and on `cron` shows the langdoc entry;
  `workspace/symbol` and `alder search SendWelcome` find the job and its `run` arm; document
  symbols list it.
- [ ] **Step 2: Run** `go test ./tree-sitter-alder/... ./internal/langdoc/... ./internal/lsp/...
  ./internal/search/...`; expected FAIL.
- [ ] **Step 3: Implement.** Langdoc text is verified against the checker (Task 2, 11) and cites
  `file:line` in the report.
- [ ] **Step 4: Run** the Step 2 command; expected PASS.
- [ ] **Step 5: Commit** `tooling: job in tree-sitter, LSP, langdoc (#168)`.

### Task 21: Book chapter 17, close out

Spec §11, §6, §6a, §7a (scaling story), §5 (rollback sentence), "Follow-ups to file".

**Files:**
- Create: `book/17-jobs.md`
- Modify: `book/07-effects.md` (pointer where `Email.send` is introduced), `book/09-stdlib.md`
  (`Job` summary), `book/10-testing.md` (`Test.jobs`/`Test.work`), `book/16-deploy.md` (a
  paragraph per target, `workers:`, the rollback sentence), `book/15-generators.md`, `book/D-stdlib.md`
  (regenerate: `go test ./book -run TestAppendixD_Current -update`), the book's index/TOC if
  `book/book_test.go` checks one, `CHANGELOG.md`
- Test: `go test ./book/...`

- [ ] **Step 1: Write chapter 17**: declaration, enqueueing, what a job may do, retries and
  idempotency (spec §6's paragraph), long jobs and fan-out (§6a), schedules, tests, the panel,
  deploy notes per target, the dedicated worker shape and the scaling story (§7a), with the
  throughput numbers from `docs/perf/2026-11-jobs-throughput.md`. Every code block is checked by
  the book tests the way other chapters' are.
- [ ] **Step 2: Run** `go test ./book/...`; expected PASS.
- [ ] **Step 3: File follow-ups** (spec list): #165 subscription hook comment, `Job.cancel` /
  `enqueueIn` / priorities / `Job.Id` on the response, alder-tui Jobs screen, `alder jobs --env`,
  streaming `Db` reads. Fly with Turso was filed in Task 16.
- [ ] **Step 4: Close out.** `go test ./...`; open PR 7; after merge tick #52 item 9, close #168,
  `go install ./cmd/alder`, rebuild the tree-sitter parser for alder.nvim.
- [ ] **Step 5: Commit** `book: chapter 17, background jobs (#168)`.
