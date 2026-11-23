# Job Controls Implementation Plan (#283)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A page can carry a `Job.Id` (route parameter, session field, column, job parameter), queue
a job `n` milliseconds from now, take a queued or running job back with `Job.cancel`, and declare a
kind's `priority N`, with the jobs table moved to schema 2 at open on every store.

**Architecture:** `Job.Id` becomes a `Text` brand, so the checker's existing brand rules carry it
everywhere. `enqueueIn` and `cancel` are new `std.Job*` effect values over `rt/jobs`. Priority is a
column (schema 2, upgraded in `db.EnsureJobsTable`), written from `rt.Job.Priority` (set by the
generated constructor) and read by a claim ordered `priority DESC, run_at, created` through one new
index. Cancel marks the row `cancelled`. It cuts off a running attempt through a per-attempt context
cancel (in-process) or a per-poll watcher (other processes). The backstop is a done mark that
refuses a row that is no longer `running`.

**Tech Stack:** Go (compiler, runtime, CLI), Alder (`stdlib/Job.alder`, `testdata/corpus/jobs-ok`),
SQLite (`modernc.org/sqlite`) and Turso over Hrana, tree-sitter (`tree-sitter-alder/`), markdown
(`book/`).

**Spec:** `docs/superpowers/specs/2026-11-22-job-controls-design.md` (ruled 2026-11-22: all twelve
recommendations accepted, so they are decisions). Read it first. Each task names the spec sections
it implements, and those sections are part of its requirements.

**Ordering with #285** (`alder jobs --env`, spec `2026-11-22-jobs-env-design.md`, its plan written
in parallel). Neither feature needs the other, but they edit the same code:
`rt/jobs/table.go` (`Row`: #285 adds json tags, this plan adds `Priority`),
`rt/jobs/admin.go` (`rowCols`, `Listing`, `Retry`'s `StateError` text, `Manual`'s signature),
`rt/jobs/run.go` (`Kind`: #285 adds `Params`, this plan adds `Priority`),
`internal/gen/jobs.go` and the `app__alder.go` goldens (both add a `Kind` field),
`cmd/alder/jobs.go` and `jobs_test.go` (#285 splits it into gather/print, this plan adds `cancel`
and the tick priority), `book/17-jobs.md` ("`alder jobs`", "What is not there yet") and
`CHANGELOG.md`. Recommended order: this plan's PRs 1–3, then #285, so that #285's `--jobs` JSON,
printer and refusal texts include `cancelled`, `priority` and the new wording from the start, and
its "schema newer than this alder's" check compares against `jobsSchema = "2"`. If #285 lands
first, Task 4 rebases onto its `Row` tags and `Kind.Params`, and Task 12 writes `cancel` over its
gather/print split. Either way, merge one PR at a time to main.

## Global Constraints

- Three PRs, each from a short branch off main, green on its own, merged before the next starts
  (spec "What is left to the builder"): **PR 1 = Tasks 1–2** (`Job.Id` brand, `enqueueIn`),
  **PR 2 = Tasks 3–8** (schema 2, priorities), **PR 3 = Tasks 9–13** (`Job.cancel`, `Cancelled`).
- No new Go dependency; `go.mod`'s require list does not change.
- Every time `rt/jobs` reads is `env.Now()` or the injected clock. `grep -n 'time\.Now' rt/jobs/*.go`
  shows only the existing defaults.
- Values pinned by the spec: priority is any whole number (negative allowed), **default 0, higher
  first**, strict (no ageing); `enqueueIn` takes **milliseconds**, and `0 or less is now`;
  `jobsSchema = "2"`; index `_alder_jobs_claim ON ("state", "priority" DESC, "run_at", "created")`
  replaces `_alder_jobs_due`; claim `ORDER BY "priority" DESC, "run_at", "created" LIMIT 1`; a
  cancelled row is pruned with done rows (`KeepDone`, 7 days); listings keep the last
  `ShownDone` (50) cancelled rows; the watcher checks once per `Options.Poll` (1 s).
- Wording pinned by the spec (quote it verbatim): trace steps `"Job.enqueueIn SendWelcome"`,
  `"Job.cancel a1b2c3d4"`; attempt line `job Signup.SendWelcome a1b2c3d4 cancelled 120ms attempt 1`
  (info); queued cancel `job Signup.SendWelcome a1b2c3d4 cancelled (was queued)` (info); retry
  refusal `only a dead or cancelled job is retried`; panel note `a running job is stopped at its
  next step in this process, within a second in another, and its writes are undone`; tick refusal
  `it is `Nightly`'s queued tick; remove its schedule to stop it`; out-of-store warn `ran to its end
  before the cancel was seen; its writes stand`.
- `Job.cancel` is a write `Call` (never in a `Query`) and does not count as a write for E0031, like
  `enqueue`. In a silo-bound Task, a cancel of a shared-store row is an autocommit statement.
- Breaking changes go in the CHANGELOG under Unreleased → Breaking: the fifth `Job.State`
  constructor `Cancelled` (exhaustive matches stop compiling), and schema 2 (an older build's claims
  fail beside or after this one). The four features go under Added.
- Verify every behaviour claim written into docs, the book or an error message against the Go,
  and cite `file:line` in the task report.
- Generated-code changes regenerate goldens with `-update`, and the diff is read, not waved through.
- Scoped tests per task; `go test ./...` once before each PR is opened.
- After each merge: `go install ./cmd/alder`. After Task 7's grammar change, also rebuild the
  alder.nvim parser (memory `alder-reinstall-after-merge`).

## Review Focus

Inputs the spec implies that no headline test meets. Each one has a test in the task named.

1. **A job that fails on its own after another process cancelled it, before the watcher saw the
   cancel.** v1's failure records (`run.go`, `WHERE "id" = ?`) are unguarded and would turn the
   `cancelled` row back into `queued` or `dead`. Expected: the row stays `cancelled` and the attempt
   is `Outcome.Cancelled`. Task 10.
2. **`Test.jobs`/`Test.work` rows compared with `==` to a constructor value of a `priority 10`
   kind** (`List.map (\r -> r.job) runs == [First {}, Second {}]` in jobs-ok). Expected: still
   equal, so the rows' `rt.Job` carries the row's priority. `Show` of a `Job` is unchanged. Tasks 4
   and 5.
3. **A schema-1 store opened inside a caller's transaction** (`prepare` in `rt/db/migrate.go:394`
   and the silo recreate in `rt/db/stores.go:827` pass a `*sql.Tx`). Expected: the upgrade runs in
   that transaction, with no nested `BEGIN`. A fresh table's `CREATE INDEX … "priority"` also must
   not run against a schema-1 table before its `ADD COLUMN`. Task 3.
4. **A cancel of a job that the same request then fails.** In the bound store the cancel is in the
   Task's transaction: the row is still `queued`/`running` afterwards, and no running attempt was
   cut off (the cut-off runs after commit). Task 10.
5. **A retried cancelled row while an `enqueueOnce` job of its kind holds the key.** Expected: a
   `KeyHeldError`, as a dead row's retry gets. Task 9.

## Spec points the plan resolves against the code

- **Index before column.** `createJobs` runs `CREATE TABLE IF NOT EXISTS` (a no-op on a schema-1
  table), so the new index cannot sit in `createJobs`: it would name a column that is not there
  yet. `_alder_jobs_claim` is created after the upgrade step, on both paths (Task 3).
- **`BEGIN IMMEDIATE`.** `EnsureJobsTable` is given a `*sql.DB`, a `*sql.Conn` or a `*sql.Tx`. The
  upgrade begins its own transaction only when `q` can begin one (`BeginTx` is `BEGIN IMMEDIATE`
  on a file, `rt/db/sqlite.go:67`, and on Hrana, `rt/db/libsql.go:428`). Otherwise it runs in the
  caller's transaction (Task 3).
- **"Not a whole number is E0050".** The parser reads `retries` as an `Int` token and reports
  anything else as a parse error (E0001). For `priority`, the parser reads `-`? and then an `Int`
  or a `Float`. It keeps a `Float` as `PriorityText`, and the checker reports that as E0050. A
  string or a missing number is a parse error, as with `retries` (Task 6).
- **`Test.work` re-reading.** The spec says the loop "re-reads due rows after each run already". It
  re-reads after each *round* (`rt/testing/jobs.go:155-175`). To give a test the worker's order
  (a newly enqueued higher-priority job runs next), `Work` takes the first due row, runs it, and
  reads again (Task 5).
- **Where the Intent example lives.** `internal/gen/testdata/e2e_jobs_test.go.txt` is Go over the
  generated `gen_jobs` app and cannot hold `Test.get`. The Alder example goes in
  `testdata/corpus/jobs-ok` (run by `cmd/alder` `TestTest_Jobs`). The e2e file gets the Go-level
  check of `Priority` on job values (Tasks 2, 6, 13).
- **`alder jobs` prints no queued rows** except schedule ticks (`cmd/alder/jobs.go:276-282`), so
  "queued rows printed with their priority" is the tick lines (Task 8).
- **"The LSP's highlighting list"** is tree-sitter's `queries/highlights.scm`. Hover reads
  `internal/langdoc` (Task 7).

---

## PR 1: `Job.Id` is a brand, `Job.enqueueIn`

### Task 1: `Job.Id` brand and `Job.enqueueIn`

Spec §1, §2, Decisions 8, 9.

**Files:**
- Modify: `stdlib/Job.alder` (`type Id = brand Text where True`, with §1's doc; `enqueueIn` after
  `enqueueAt` with a #119 doc and a checked example
  `Job.enqueueIn 3600000 (Jobs.Nightly.Nightly {})`)
- Modify: `internal/gen/types.go:64-67` (drop the `Id`/`Job` case), `internal/gen/types.go:16`
  (comment), `internal/gen/stdcall.go:273` (`"Job.enqueueIn": "std.JobEnqueueIn"`)
- Modify: `rt/std/job.go` (`JobEnqueueIn`)
- Test: `rt/std/job_test.go`, `internal/check/jobs_test.go`, `book/stdlibdocs_test.go` (existing)

**Interfaces:**
- Produces: `func JobEnqueueIn(ms int64, j rt.Job) eff.Task[string]` in `rt/std/job.go`:
  `jobs.Enqueue(e, j, now(e).Add(time.Duration(max(ms, 0))*time.Millisecond), nil)`, with trace
  step `jobStep("Job.enqueueIn", j)`. Alder: `enqueueIn : Int -> Job -> Task Id`.

- [ ] **Step 1: Write the failing tests.**
  - `TestJobEnqueueIn_Clock` (`rt/std/job_test.go`, on a test store with a fixed `Env.Now`):
    `JobEnqueueIn(60000, j)` writes `run_at == now+60000`; `JobEnqueueIn(-5, j)` and `(0, j)` give
    `run_at == now`; the trace step is `"Job.enqueueIn SendWelcome"`.
  - `TestJob_IdIsABrand` (`internal/check/jobs_test.go`, with `jobApp`): a route
    `GET "/exports/{job}" { job: Job.Id }`, a session field `export: Maybe Job.Id`, a table column
    `job: Maybe Job.Id`, and a job parameter `{ of: Job.Id }` check with no diagnostics.
    `Job.Id "x" == Job.Id "x"` type-checks.
- [ ] **Step 2: Run** `go test ./rt/std ./internal/check -run 'EnqueueIn|IdIsABrand'`. Expected:
  FAIL (no `JobEnqueueIn`; a route parameter of opaque `Job.Id` is refused).
- [ ] **Step 3: Implement** as in Interfaces. `rt/std/job.go`'s header comment lists `enqueueIn`.
- [ ] **Step 4: Run** `go test ./rt/std ./internal/check ./internal/gen ./book -run 'Job|Golden|StdlibDocs|AppendixD'`.
  Expected: PASS, except `TestAppendixD_Current`. Regenerate it with
  `go test ./book -run TestAppendixD_Current -update` and read the diff.
- [ ] **Step 5: Commit** `jobs: Job.Id is a Text brand; Job.enqueueIn (#283)`.

### Task 2: The page that waits for its job, and its docs

Spec §1 (the two shapes), §9 (chapter 17's new section, `enqueueIn`).

**Files:**
- Modify: `testdata/corpus/jobs-ok/app/Signup.alder` (or a new `Exports.alder` beside it):
  `job BuildExport = { user: User.UserId }`, routes `Start` (POST, `Job.enqueue`, then
  `Response.redirect (Export { job: id })`) and `Export` (GET `"/exports/{job}" { job: Job.Id }`,
  `load = Job.status p.job`, a view that shows the state or "no such job"), as in spec "Intent"
  without `priority` and `Stop`
- Modify: `cmd/alder/test_test.go` (`TestTest_Jobs`'s pass count)
- Modify: `book/17-jobs.md` ("Enqueueing a job": `enqueueIn`, dropping the `Time.addMillis`
  paragraph; a new section **"A page that waits for its job"** after "Where the job is stored",
  with the URL shape and the column shape `table Export = { …, job: Maybe Job.Id }`; "What is not
  there yet" loses `enqueueIn` and the `Job.Id`-on-response bullet), `book/09-stdlib.md` (`Job`
  summary), `CHANGELOG.md` (Added: `Job.Id` is a `Text` brand; `Job.enqueueIn`)

- [ ] **Step 1: Write the failing tests** in jobs-ok:
  - `test "a page finds its job by the id in its URL"`: `Test.submit Start {}` → `Test.get (Export
    { job: id })` shows `Queued` (id read from `Test.jobs`); after `Test.work` it shows `Done`.
  - `test "an id that names no job shows none"`: `Test.get (Export { job: Job.Id "00000000" })`
    shows the no-such-job text.
  - `test "enqueueIn is due after its delay"`: at `march`, `Job.enqueueIn 60000 (Second {})`;
    `Test.work` runs nothing; after `Test.advance 60000` it runs one.
- [ ] **Step 2: Run** `go test ./cmd/alder -run TestTest_Jobs`. Expected: FAIL (the count).
- [ ] **Step 3: Implement**: the corpus app, then the book and CHANGELOG text, then the new pass
  count in `TestTest_Jobs` (`"9 passed, 1 failed\n"`).
- [ ] **Step 4: Run** `go test ./cmd/alder -run 'TestTest_Jobs|TestCheck' ./book/...`. Expected: PASS
  (book snippets are checked).
- [ ] **Step 5: Commit** `book: a page that waits for its job; enqueueIn (#283)`. Then
  `go test ./...` and open PR 1.

---

## PR 2: schema 2 and priorities

### Task 3: Jobs schema 2 and its upgrade at open

Spec §5, Decision 7.

**Files:**
- Modify: `rt/db/migrate.go:868-917` (`jobsSchema = "2"`; `createJobs` gains
  `"priority" INTEGER NOT NULL DEFAULT 0` after `"scheduled_for"` and loses `_alder_jobs_due`;
  `EnsureJobsTable` runs the upgrade and then the claim index)
- Modify: `rt/jobs/table.go:22` (comment: jobs_schema = 2), `rt/jobs/claim.go:18,96-98`
  (`RETURNING` names `rowCols`)
- Test: `rt/db/migrate_test.go`, `rt/db/sqlite_test.go:26` and `rt/db/migrate_test.go:867` (expect
  `"2"`), `rt/db/libsql_test.go` (Turso)

**Interfaces:**
- Produces, in `rt/db/migrate.go`:
  - `const createJobsClaimIndex = `CREATE INDEX IF NOT EXISTS "_alder_jobs_claim" ON "_alder_jobs" ("state", "priority" DESC, "run_at", "created")``
  - `func upgradeJobs(ctx context.Context, q Execer) error`: when `q` has
    `BeginTx(context.Context, *sql.TxOptions) (*sql.Tx, error)`, it begins a transaction and
    commits it. Otherwise it runs on `q`. Inside, it re-reads `jobs_schema`. Only when that is `"1"`
    does it run `ALTER TABLE … ADD COLUMN "priority" …` (skipped when
    `SELECT 1 FROM pragma_table_info('_alder_jobs') WHERE name = 'priority'` finds it),
    `DROP INDEX IF EXISTS "_alder_jobs_due"`, and `UPDATE "_alder_meta" SET "value" = '2' WHERE
    "key" = 'jobs_schema'`. Any other value (`"2"`, or newer) leaves the table alone.
  - `EnsureJobsTable` = today's statements, the `INSERT … DO NOTHING` of `jobsSchema`,
    `upgradeJobs`, then `createJobsClaimIndex`.

- [ ] **Step 1: Write the failing tests** (`rt/db/migrate_test.go`). Keep the schema-1 statements
  verbatim from today's `createJobs`, as a test constant `schema1Jobs`.
  - `TestEnsureJobsTable_Fresh`: `jobs_schema` is `"2"`, the table has `priority` (default 0),
    `_alder_jobs_claim` exists and `_alder_jobs_due` does not.
  - `TestEnsureJobsTable_UpgradesSchema1`: on a file made by `schema1Jobs` plus
    `jobs_schema = '1'`, with one queued row, `EnsureJobsTable(ctx, db)` gives `jobs_schema` `"2"`,
    the row's `priority` 0 and the index swap. A second call changes nothing.
  - `TestEnsureJobsTable_UpgradeInCallersTx`: the same upgrade through a `*sql.Tx` the test began.
    It commits with the caller.
  - `TestEnsureJobsTable_UpgradeAlreadyWidened`: a schema-1 table that already has `priority` (a
    hand edit) upgrades without an `ALTER` error.
  - `TestEnsureJobsTable_TwoOpeners`: two `*SQLite` on one file call `EnsureJobsTable` from two
    goroutines at once. Both return nil, and the column is added once.
  - `TestEnsureJobsTable_NewerSchemaLeftAlone`: `jobs_schema = '3'` is left as it is.
  - `TestTurso_UpgradesJobsTable` (`rt/db/libsql_test.go`, `TestTurso_CreatesJobsTable`'s
    pattern): a schema-1 shared database on the fake upgrades through `EnsureRuntimeTables`, and a
    schema-1 silo child upgrades when `openEntry` opens it.
- [ ] **Step 2: Run** `go test ./rt/db -run 'EnsureJobsTable|EnsureTable|Turso_.*Jobs'`. Expected: FAIL.
- [ ] **Step 3: Implement** as in Interfaces.
- [ ] **Step 3b: Name the claim's columns.** A fresh table now has sixteen columns, so
  `claimStmt`'s `RETURNING *` (`rt/jobs/claim.go:18`) no longer fits `scanRow`'s fifteen
  destinations. Change it to `RETURNING ` + `rowCols` (still fifteen named columns), and fix
  `scanRow`'s comment ("in rowCols' order"). Task 4 appends `"priority"` to both.
- [ ] **Step 4: Run** `go test ./rt/db/... ./rt/jobs/... ./rt/testing/...`. Expected: PASS.
- [ ] **Step 5: Commit** `db: jobs schema 2, the priority column and claim index (#283)`.

### Task 4: Priority in the runtime: enqueue, claim, ticks, listing

Spec §4 (runtime), §5 "Older builds".

**Files:**
- Modify: `rt/job.go` (`Job` gains `Priority int`; `Show` unchanged), `rt/jobs/run.go:17-27`
  (`Kind.Priority int`), `rt/jobs/table.go` (`Row.Priority int`), `rt/jobs/admin.go`
  (`rowCols` gains `, "priority"` last; `List`'s queued order; `sort`'s queued comparator;
  `Manual`), `rt/jobs/claim.go` (`claimStmt`'s `ORDER BY "priority" DESC, run_at, created`;
  `scanRow` reads `&r.Priority` last), `rt/jobs/enqueue.go:67` (INSERT writes `"priority"` from
  `j.Priority`), `rt/jobs/schedule.go:181` (`insertTick` takes the kind), `cmd/alder/jobs.go:466`
  (`Manual` call)
- Test: `rt/jobs/enqueue_test.go`, `rt/jobs/worker_test.go` (or a new `rt/jobs/priority_test.go`),
  `rt/jobs/schedule_test.go`, `rt/jobs/admin_test.go`, `rt/show_test.go` (or wherever `Show` is
  tested)

**Interfaces:**
- Produces:
  - `rt.Job{Kind, Params string; Priority int}`. `Priority` is a function of `Kind`, so `==` stays
    meaningful. `rt.Show` of a `Job` prints what it printed before (kind and params only), through
    an `AlderShow` method or a case in `rt/show.go`.
  - `jobs.Kind.Priority int`, `jobs.Row.Priority int`.
  - `func Manual(ctx context.Context, q db.Execer, k Kind, now time.Time) (string, error)`: writes
    `k.Name` and `k.Priority` (was `kind string`).
  - `func insertTick(ctx context.Context, q db.Execer, k Kind, at, now time.Time) error`: the
    tick's row takes `k.Priority`. `EnsureTicksOn` and `ClaimedTick` pass `k`.
  - `Listing.Queued` is in claim order: `priority DESC, run_at, created`.

- [ ] **Step 1: Write the failing tests.**
  - `TestEnqueue_WritesPriority`: `Enqueue` of `rt.Job{Kind: "A.B", Params: "{}", Priority: 10}`
    stores `priority` 10.
  - `TestClaim_PriorityOrder`: rows `low` (priority 0, run_at 1), `high` (10, run_at 5), `neg`
    (-5, run_at 0), `tie` (10, run_at 5, created later), all due. Four claims give `high`, `tie`,
    `low`, `neg`. A `high2` (10) not yet due does not block `low`.
  - `TestClaim_ReturnsPriority`: the claimed `Row.Priority` is the column's.
  - `TestClaim_OldBuildFailsOnSchema2`: v1's claim statement, kept verbatim in the test (`RETURNING *`
    scanned into v1's fifteen destinations), against a schema-2 table returns a Scan error. This
    pins spec §5's documented behaviour.
  - `TestTicks_TakeKindPriority`: `EnsureTicksOn` with `Kind{…, Every: time.Hour, Priority: 3}`
    inserts a tick with `priority` 3, and `ClaimedTick`'s next tick does too.
  - `TestAdmin_ManualPriority`: `Manual(ctx, q, Kind{Name: "A.B", Priority: 2}, now)`'s row has 2.
    `Retry` keeps a dead row's priority.
  - `TestList_QueuedInClaimOrder`: `List` and the panel's merged `sort` put priority 10 before 0
    before -5.
  - `TestShow_JobIgnoresPriority`: `rt.Show(rt.Job{Kind: "A.B", Params: "{}", Priority: 10}) ==
    rt.Show(rt.Job{Kind: "A.B", Params: "{}"})`.
- [ ] **Step 2: Run** `go test ./rt ./rt/jobs -run 'Priority|OldBuild|Ticks_Take|Manual|QueuedInClaim|ShowJob|Show_Job'`.
  Expected: FAIL.
- [ ] **Step 3: Implement** as in Interfaces. `cmd/alder/jobs.go`'s `jobsRun` passes
  `jobs.Kind{Name: name, Priority: <the JobInfo's Priority>}`. Until Task 6 adds
  `check.JobInfo.Priority`, it passes 0 with a `// Task 6` note. Task 6 replaces it.
- [ ] **Step 4: Run** `go test ./rt/... ./cmd/alder -run 'Job|Jobs|Claim|Tick|Admin|Panel|Show'`.
  Expected: PASS.
- [ ] **Step 5: Commit** `jobs: priority on rows, claims in priority order (#283)`.

### Task 5: `Test.work` in claim order, rows with their priority

Spec §4 (`dueRows`), §7 first bullet.

**Files:**
- Modify: `rt/testing/jobs.go` (`dueRows` selects `"priority"` and sorts
  `priority DESC, run_at, created, place, rowid`; `Work` runs the first due row and then reads
  again, instead of running a whole round; `Jobs` and `Work` build `rt.Job{Kind, Params, Priority}`
  from the column)
- Test: `rt/testing/jobs_test.go`

**Interfaces:**
- Consumes: `jobs.Row.Priority`, `rt.Job.Priority` (Task 4).
- Produces: no signature change. `Work`'s doc says "highest priority first, then oldest; a job a
  run enqueues is in the next read".

- [ ] **Step 1: Write the failing tests.**
  - `TestJobs_WorkPriorityOrder`: enqueue `Low` (priority 0) and then `High` (10), both due.
    `Work`'s runs are `[High, Low]`.
  - `TestJobs_WorkEnqueuedHigherRunsNext`: `First` (0) enqueues `Urgent` (10) while `Other` (0,
    enqueued after `First`) is due. The runs are `[First, Urgent, Other]`.
  - `TestJobs_RowsCarryPriority`: `Jobs()`'s and `Work()`'s `rt.Job` values `==` the enqueued
    `rt.Job{…, Priority: 10}`.
  - The existing `TestJobs_WorkRunsJobsEnqueuedByJobs` and `TestJobs_WorkAcrossSilos` still pass
    unchanged.
- [ ] **Step 2: Run** `go test ./rt/testing -run 'TestJobs_'`. Expected: FAIL.
- [ ] **Step 3: Implement** as in Files. `MaxWorkRuns` still counts runs.
- [ ] **Step 4: Run** `go test ./rt/testing/...`. Expected: PASS.
- [ ] **Step 5: Commit** `testing: Test.work in claim order (#283)`.

### Task 6: `priority N` in the language

Spec §4 "Declaration", Decisions 4, 5.

**Files:**
- Modify: `internal/syntax/ast.go:171` (`JobDecl.Priority *IntLit`, `PriorityText string`),
  `internal/syntax/parser.go:397-431` (`priority` in the option set and the unknown-option message
  "the options are every, cron, retries, timeout, silo and priority"), `internal/syntax/dump.go:95`,
  `internal/format/print_decl.go:342` (`" priority " + value`)
- Modify: `internal/check/jobs.go` (`JobInfo.Priority int`; E0050 for `PriorityText`),
  `internal/diag/diag.go:151` (comment), `internal/diag/explain.go` (E0050's explanation names
  `priority`), `internal/ir/ir.go:333` (`Job.Priority int`), `internal/ir/lower.go:649`,
  `internal/ir/dump.go:188` (`(priority N)` when not 0)
- Modify: `internal/gen/jobs.go` (a `Priority: N,` line in the kind when not 0; `jobRef` adds
  `Priority: N` when not 0), `testdata/golden/gen_jobs/app/Signup.alder` (one job gains
  `priority 10`), regenerated goldens, `internal/gen/testdata/e2e_jobs_test.go.txt`,
  `cmd/alder/jobs.go` (`jobsRun` passes `j.Priority`)
- Modify: `testdata/corpus/jobs-ok/app/Signup.alder` (`job Urgent = {} priority 10`), and
  `cmd/alder/test_test.go` (count)
- Test: `internal/syntax/parser_test.go`, `internal/format/format_test.go`,
  `internal/check/jobs_test.go`, `internal/gen/jobs_test.go`

**Interfaces:**
- Consumes: `jobs.Kind.Priority`, `rt.Job.Priority` (Task 4).
- Produces: `syntax.JobDecl.Priority *IntLit` (its value already negated for `-N`),
  `JobDecl.PriorityText string` (the literal as written when it is a `Float`),
  `check.JobInfo.Priority int`, `ir.Job.Priority int`.

- [ ] **Step 1: Write the failing tests.**
  - `TestParse_JobDecl` gains: `job A = {} priority 10` → `Priority.Value` 10;
    `priority -5` → -5; `retries 2 priority 1 silo org` in any order. `TestParse_JobDecl_Errors`
    gains: `priority "high"` and a bare `priority` → a parse error with
    "expected a whole number after `priority`", and `priority 1 priority 2` → "given more than
    once".
  - `TestFormat_Job`: `job A={}  priority   -5` formats to `job A = {} priority -5`, and the
    spec's `job BuildExport = { user: User.UserId } priority 10` is a fixed point.
  - `TestJob_Priority` (`internal/check`): `priority 1.5` → E0050 with
    "`priority 1.5`: a priority is a whole number, as in `priority 10`". A valid one sets
    `JobInfo.Priority`. No `priority` gives 0.
  - `TestJobValue_Priority` in `e2e_jobs_test.go.txt` (added to the `runE2E` list in
    `internal/gen/jobs_test.go`): the generated constructor of the `priority 10` job gives
    `Priority == 10`, and `Jobs()`'s kind has `Priority: 10`. Other jobs give 0.
  - jobs-ok: `test "the higher priority runs first"`: enqueue `Second {}` and then `Urgent {}`.
    `Test.work`'s jobs are `[Urgent {}, Second {}]`.
- [ ] **Step 2: Run** `go test ./internal/syntax ./internal/format ./internal/check ./internal/gen -run 'Job|Golden'`.
  Expected: FAIL.
- [ ] **Step 3: Implement.** `priority` stays a contextual word after the record, not a keyword.
  Regenerate goldens with `go test ./internal/gen -run Golden -update` and read the diff (only the
  `priority 10` job changes).
- [ ] **Step 4: Run** `go test ./internal/... ./cmd/alder -run 'Job|Golden|TestTest_Jobs|Format|Parse'`.
  Expected: PASS.
- [ ] **Step 5: Commit** `language: the priority job option (#283)`.

### Task 7: Tooling: langdoc, tree-sitter

Spec §9 (`internal/langdoc`, `tree-sitter-alder`).

**Files:**
- Modify: `internal/langdoc/langdoc.go:3,379` (the option list) and a new `{"priority", "", …}`
  entry beside `retries` (`:426`): "claimed before lower; 0 by default; higher first; strict, per
  store", with a book reference to chapter 17 "Priorities"
- Modify: `tree-sitter-alder/grammar.js:336-341` (`seq('priority', field('value', …))` accepting
  `10` and `-5`), `tree-sitter-alder/queries/highlights.scm:20` (`"priority"` in the job-option
  keyword list), regenerated `src/`
- Test: `tree-sitter-alder/highlights_test.go`, `tree-sitter-alder/drift_test.go`,
  `internal/langdoc` and `internal/lsp/hover_test.go`

- [ ] **Step 1: Write the failing tests.** Highlighting marks `priority` as a keyword in a job line
  and not as an identifier elsewhere. `job A = {} priority -5` parses without an `ERROR` node.
  Hover on `priority` in a job line shows the langdoc entry.
- [ ] **Step 2: Run** `go test ./tree-sitter-alder/... ./internal/langdoc/... ./internal/lsp/... -run 'Highlight|Drift|Hover|Langdoc'`.
  Expected: FAIL.
- [ ] **Step 3: Implement**, and regenerate the parser the way the repo's drift test says.
- [ ] **Step 4: Run** the Step 2 command. Expected: PASS.
- [ ] **Step 5: Commit** `tooling: priority in tree-sitter and hover (#283)`.

### Task 8: Panel and CLI columns, bench, book, CHANGELOG

Spec §4 (index cost, fairness), §5 (rollback), §8 (priority columns), §9.

**Files:**
- Modify: `rt/web/devjobs.go` (`JobRow.Priority int \`json:"priority"\``; the queued table shows
  `priority N` next to the kind when not 0; the queued rows are already in claim order, Task 4)
- Modify: `cmd/alder/jobs.go` (a tick line shows `priority N` when not 0)
- Modify: `docs/perf/2026-11-jobs-throughput.md` (a "Schema 2" section with the before and after
  numbers)
- Modify: `book/17-jobs.md` (options table `priority 10` / "claimed before lower; 0"; "The table"
  gains the column and the claim's `ORDER BY`; a new **"Priorities"** subsection under "How the
  queue works": strict, per store, starvation, the welcome mail before the nightly digest; "Rolling
  back" gains the schema-2 bullet from spec §5 "Older builds"), `CHANGELOG.md` (Breaking: schema 2
  and older builds; Added: `priority N`)
- Test: `rt/web/devjobs_test.go`, `cmd/alder/jobs_test.go`, `go test ./book/...`

- [ ] **Step 1: Write the failing tests.** `TestDevJobs_JSON` asserts `"priority":10` on a queued
  row. `TestDevJobs_List` asserts that the page shows `priority 10` and that the priority-10 row
  comes before the 0 row. `TestJobs_List` (CLI) shows a tick of a priority-3 kind with
  `priority 3`.
- [ ] **Step 2: Run** `go test ./rt/web ./cmd/alder -run 'TestDevJobs_|TestJobs_List'`. Expected: FAIL.
- [ ] **Step 3: Implement**, then run the bench on main and on the branch:
  `go test ./rt/jobs -run '^$' -bench BenchmarkJobs -benchtime=2000x -count 5 -timeout 20m`.
  Record `uptime` before each run, as the perf doc does. If the claim loses more than 10 %, keep
  `_alder_jobs_due` beside the new index (in `createJobs` again, and not dropped by the upgrade),
  and say so in the perf doc and the PR. Then write the book and CHANGELOG text.
- [ ] **Step 4: Run** `go test ./rt/web/... ./cmd/alder/... ./book/...`. Expected: PASS.
- [ ] **Step 5: Commit** `jobs: priority in the panel and alder jobs; book and bench (#283)`. Then
  `go test ./...` and open PR 2.

---

## PR 3: `Job.cancel` and `Cancelled`

### Task 9: The `Cancelled` state

Spec §6, Decision 1, §3 "The key".

**Files:**
- Modify: `stdlib/Job.alder` (`type State = Queued | Running | Done | Dead | Cancelled`, its doc
  gains "`Cancelled` was taken back by `Job.cancel`, the Jobs page or `alder jobs cancel`"; the
  `Status` doc says `runAt` is `Nothing` for it)
- Modify: `rt/std/job.go` (`JobCancelled` after `JobDead`; `RegisterUnion` gains `"Cancelled"`;
  `JobStatusOf` maps `jobs.Cancelled`, and its error text says "none of queued, running, done, dead
  and cancelled")
- Modify: `rt/jobs/table.go` (`Cancelled State = "cancelled"`), `rt/jobs/housekeep.go:102`
  (`"state" IN ('done', 'cancelled')` under `KeepDone`), `rt/jobs/active.go:317` (the start scan's
  prunable test likewise), `rt/jobs/admin.go` (`Listing.Cancelled`, `List`, `merge`, `sort`;
  `Retry` takes `dead` or `cancelled`; `StateError.Error`'s wording), `rt/web/devjobs.go`
  (`JobsView.Cancelled []JobRow \`json:"cancelled"\``; a **Cancelled** section after Dead;
  **Retry** on a cancelled row's page with the dead row's wording), `cmd/alder/jobs.go` (the counts
  table lists `cancelled` after `dead`)
- Test: `rt/std/job_test.go`, `rt/jobs/admin_test.go`, `rt/jobs/worker_test.go` (housekeeping),
  `rt/web/devjobs_test.go`, `cmd/alder/jobs_test.go`

**Interfaces:**
- Produces: `jobs.Cancelled`, `std.JobCancelled`, `Listing.Cancelled []Row` (the last `ShownDone`
  by `finished`, the last first), `JobsView.Cancelled`. `Retry`'s statement ends
  `WHERE "id" = ? AND "state" IN ('dead', 'cancelled')`. `StateError` with `Want == Dead` reads
  `job x is queued: only a dead or cancelled job is retried`. `RetryDead` and `retry --dead` stay
  dead-only.

- [ ] **Step 1: Write the failing tests.**
  - `TestJobStatusOf_Cancelled`: `jobs.Status{State: "cancelled"}` → `Tag == JobCancelled`,
    `RunAt` Nothing. `rt.Show` of that state is `Cancelled`.
  - `TestAdmin_RetryCancelled`: a cancelled row retries to queued with attempts 0. A queued row's
    retry is refused with "only a dead or cancelled job is retried". A cancelled row with key `k`
    while a queued row of its kind holds `k` → `*KeyHeldError` (Review Focus 5).
  - `TestAdmin_ListCancelled`: 60 cancelled rows → `Listing.Cancelled` has 50, the last finished
    first. `Counts` has `cancelled`.
  - `TestHousekeep_PrunesCancelledWithDone`: a cancelled row finished 8 days ago is deleted, and
    one finished 6 days ago is kept.
  - `TestDevJobs_Cancelled`: the JSON has a `cancelled` array. The page has a Cancelled section,
    and a cancelled row's page has a Retry form.
  - `TestJobs_List` (CLI): the counts table has a `cancelled` line.
  - `TestJobs_RetryID` gains a cancelled row retried by `alder jobs retry <id>`.
- [ ] **Step 2: Run** `go test ./rt/std ./rt/jobs ./rt/web ./cmd/alder -run 'Cancelled|RetryCancelled|ListCancelled|Prunes|TestJobs_'`.
  Expected: FAIL.
- [ ] **Step 3: Implement** as in Files. `gen` needs no change (`match.go:61` and `expr.go:615`
  emit `std.Job<Name>`). Check that `go test ./internal/gen -run Golden` is unchanged.
- [ ] **Step 4: Run** `go test ./rt/... ./internal/gen/... ./cmd/alder -run 'Job|Jobs|Golden|Admin|Panel|Housekeep'`.
  Expected: PASS.
- [ ] **Step 5: Commit** `jobs: the Cancelled state (#283)`.

### Task 10: `Job.cancel` and the done-mark backstop

Spec §3 (where it looks, the statement, the key, the backstop, the self-cancel, the lock),
Decisions 2, 3, 10, 11.

**Files:**
- Create: `rt/jobs/cancel.go`
- Modify: `rt/jobs/run.go` (`Outcome.Cancelled`; the in-store done mark is guarded; `record`
  returns the rows changed; the failure records are guarded; the out-of-store mark's warn line),
  `rt/std/job.go` (`JobCancel`), `internal/gen/stdcall.go` (`"Job.cancel": "std.JobCancel"`),
  `stdlib/Job.alder` (`cancel : Id -> Task Bool` with a #119 doc: "`True` when the job was queued
  or running and is now cancelled; `False` when there is no such job, it is finished, or it is a
  schedule's queued tick. A running job is stopped at its next step and its writes are undone. In
  a silo-bound `Task`, a job in the shared store is cancelled at once, apart from the `Task`'s
  transaction", with the example `\(id : Job.Id) -> Job.cancel id`)
- Test: `rt/jobs/cancel_test.go`, `rt/jobs/run_test.go`, `rt/testing/jobs_test.go`

**Interfaces:**
- Produces, in `rt/jobs/cancel.go`:
  - `var errCancelled = errors.New("cancelled")`: the cause of a cut-off attempt's context and the
    done mark's refusal. Its text is `Test.work`'s `Err "cancelled"`.
  - `const cancelStmt = `UPDATE "_alder_jobs" SET "state" = 'cancelled', "finished" = ?, "lease_until" = NULL WHERE "id" = ? AND "state" IN ('queued', 'running') AND "schedule" = 0``
  - `func Cancel(env *eff.Env, id string) (bool, error)`: it walks `db.RuntimeReads(env)` (as
    `StatusOf` does). In the first place that has the row (`SELECT "kind", "state" …`), it runs
    `cancelStmt` on `db.On(p.Store, p.Tx)`. That is the Task's transaction in the bound store, or
    an autocommit for a shared-store place without a `Tx` (Decision 10). One row changed → `true`.
    A row that was `running` gets `cutOff(id)`: through `env.AfterCommit` when `p.Tx != nil`, at
    once otherwise. A row that was `queued` logs the info line
    `job <kind> <id> cancelled (was queued)` through `env.Log`. No row, a final row, or a queued
    tick → `false`, nil.
  - `func cutOff(id string) bool`: cancels the registered attempt of `id` in this process with
    `errCancelled`. It is false when none runs here. Task 11 adds the registry, and until then it
    is a stub that returns false.
  - `std.JobCancel(id string) eff.Task[bool]`: `eff.Call("Job.cancel "+id, true, …)`.
- `Outcome.Cancelled bool`: the attempt ended because its row was cancelled. `Err` is
  `errCancelled`, nothing is recorded, and `Dead`/`Next` are zero.
- `record(reg, at, ctx, stmt, args…) (int64, error)`: the rows changed.

- [ ] **Step 1: Write the failing tests** (`rt/jobs/cancel_test.go`, on the `run_test.go` helpers
  `claim`, `kind`, `insertNote`, `then`):
  - `TestCancel_Queued`: `Cancel` → true. The row is `cancelled`, has `finished` = now and
    `lease_until` NULL, and keeps `attempts`/`error`. A second `Cancel` → false.
  - `TestCancel_NoRowFinalTick`: an unknown id, a done row, a dead row, and a queued row with
    `schedule = 1` → false, unchanged. A claimed tick (`schedule = 0`, running) → true.
  - `TestCancel_ReleasesKey`: after cancelling an `enqueueOnce "k"` row, `Enqueue` with key `k`
    makes a new id.
  - `TestCancel_InTaskTxRollsBack`: `Cancel` inside a Task that then fails leaves the row `queued`
    (Review Focus 4).
  - `TestCancel_SiloTaskSharedRow`: a Task bound to silo `org/1` cancels a shared-store row. The
    row is cancelled even though the Task fails afterwards (autocommit).
  - `TestRunOne_Backstop` (`run_test.go`): the job's first step cancels its own row by a direct
    `UPDATE` on a second connection, before any write. Then it calls `insertNote`. `RunOne` →
    `Outcome.Cancelled`, the note is absent, and the row stays `cancelled` (not `done`).
  - `TestRunOne_FailureAfterCancelKeepsCancelled`: the row is cancelled by a direct `UPDATE`, then
    the job fails (`Task.fail`). The row is still `cancelled` and the outcome is `Cancelled`
    (Review Focus 1).
  - `TestRunOne_ApartMarkAfterCancelWarns`: a row in another store than the run's, cancelled
    during the run. The job's writes stand, and the warn line contains "ran to its end before the
    cancel was seen; its writes stand".
  - `TestJobs_WorkSelfCancel` (`rt/testing`): a job that calls `Job.cancel` on its own id → `Work`'s
    result is `Err "cancelled"`, its writes are undone, and the row is `running`.
  - `TestJobs_WorkCancelQueuedFromRun`: a job cancels another queued row. That row is
    `Cancelled` in `Test.jobs` and does not run.
- [ ] **Step 2: Run** `go test ./rt/jobs ./rt/testing -run 'Cancel|Backstop|FailureAfterCancel|ApartMark'`.
  Expected: FAIL.
- [ ] **Step 3: Implement.**
  - The in-store done mark: `… WHERE "id" = ? AND "state" = 'running'`. On zero rows it reads the
    state: `cancelled` returns `errCancelled`, and anything else re-runs v1's unguarded mark (the
    job did run).
  - The failure records (retry and dead) gain `AND "state" <> 'cancelled'`. Zero rows changed with
    a `cancelled` row makes the outcome `Cancelled`.
  - In `RunOne`, `cancelled := context.Cause(base) == errCancelled || errors.Is(err, errCancelled)`
    is checked before `stopped` and `timedOut`.
- [ ] **Step 4: Run** `go test ./rt/jobs/... ./rt/testing/... ./rt/std/... ./internal/gen/... ./book -run 'Job|Jobs|Cancel|RunOne|Golden|StdlibDocs|AppendixD'`.
  Expected: PASS after `go test ./book -run TestAppendixD_Current -update`.
- [ ] **Step 5: Commit** `jobs: Job.cancel; the done mark refuses a cancelled row (#283)`.

### Task 11: Cutting a running attempt off: in process and from elsewhere

Spec §3 "A running attempt, in the cancelling process" and "elsewhere", §6 "Log and metrics",
Decision 12.

**Files:**
- Modify: `rt/jobs/cancel.go` (the registry and `cutOff`), `rt/jobs/worker.go` (`pool.run`
  derives `context.WithCancelCause` and registers it; `start` runs the watcher), `rt/jobs/observe.go`
  (`outcome()` gives `"cancelled"`; `Line` sets no `Err` for it), `rt/applog/job.go` (`Text` case
  `"cancelled"`: `job K ID cancelled 120ms attempt 1`; `Level` info), `rt/metrics/metrics.go:39`
  (`outcomes` gains `"cancelled"`)
- Test: `rt/jobs/cancel_test.go`, `rt/jobs/worker_test.go`, `rt/applog/job_test.go`,
  `rt/metrics/metrics_test.go`

**Interfaces:**
- Produces:
  - `var running sync.Map`: job id → `context.CancelCauseFunc`, stored by `pool.run` for the
    attempt's duration (deleted and released on return). `cutOff(id)` loads it and calls it with
    `errCancelled`.
  - `pool.attempts sync.Map`: job id → `db.Place` (the row's `In`), for the watcher.
  - `func (p *pool) watch(ctx context.Context, tick <-chan time.Time)`: one goroutine per `Start`,
    on a ticker of `o.Poll`, ending with the run context. On each tick, when `p.attempts` is not
    empty, it runs `SELECT "id" FROM "_alder_jobs" WHERE "id" IN (…) AND "state" = 'cancelled'`
    once per store that has attempts: the shared store on `db.On(p.o.Store, nil)`, a silo through
    `p.at` (a closed silo is not opened). It calls `cutOff` for each id found. A failed read is a
    warn line through `claimed`'s rate limit.
  - `Outcome.outcome()` order: stopped, cancelled, done, dead, failed.

- [ ] **Step 1: Write the failing tests.**
  - `TestWorker_CancelInProcess`: the job inserts a note and then blocks on `e.Ctx.Done()`. Once it
    is running (`OnAttempt` not yet called, the row `running`), `Cancel` on a Task env → true.
    Within 1 s `OnAttempt` gets `Outcome{Cancelled: true}`, the note is absent, the row is
    `cancelled`, and attempts is 1 (nothing is recorded twice).
  - `TestWorker_CancelFromElsewhere`: the same with `Poll: 20*time.Millisecond`. The row is
    cancelled by a direct `UPDATE` on another connection, and the attempt ends `Cancelled` within
    a few polls.
  - `TestWorker_CancelInSilo`: the same for a `silo org` job, through the watcher's `p.at`.
  - `TestWorker_StopStillRequeues`: `stop(0)` during an attempt re-queues it (`Stopped`, not
    `Cancelled`).
  - `TestJob_Text` gains `{Outcome: "cancelled", Dur: 120ms, Attempt: 1}` →
    `job Signup.SendWelcome a1b2c3d4 cancelled 120ms attempt 1`, at level info.
  - `TestMetrics_Jobs` gains `alder_jobs_total{…,outcome="cancelled"} 1`.
- [ ] **Step 2: Run** `go test ./rt/jobs ./rt/applog ./rt/metrics -run 'Cancel|StopStill|TestJob_Text|TestMetrics_Jobs'`.
  Expected: FAIL.
- [ ] **Step 3: Implement** as in Interfaces. `go test -race` on the new tests.
- [ ] **Step 4: Run** `go test -race ./rt/jobs/... ./rt/applog/... ./rt/metrics/...`. Expected: PASS.
- [ ] **Step 5: Commit** `jobs: cut a cancelled attempt off, here and from elsewhere (#283)`.

### Task 12: Cancel on the Jobs page and in `alder jobs`

Spec §8.

**Files:**
- Modify: `rt/jobs/admin.go` (`CancelRow`, `TickError`), `rt/jobs/panel.go` (`Panel.Cancel`
  through `change`, then `cutOff(id)`), `rt/web/devjobs.go` (the `cancel` action in
  `devJobAction`; a **Cancel** form on queued and running rows with the spec's sentence, none on a
  tick; 409 for `*TickError`), `cmd/alder/jobs.go` (`alder jobs cancel <id>`, `jobsUsage` and
  `jobsHelpNote` lines)
- Test: `rt/jobs/admin_test.go`, `rt/web/devjobs_test.go`, `cmd/alder/jobs_test.go`

**Interfaces:**
- Consumes: `cancelStmt`, `cutOff` (Tasks 10, 11).
- Produces:
  - `func CancelRow(ctx context.Context, q db.Execer, id string, now time.Time) error` (the shape
    of `Retry`/`RunNow` for `change`): `ErrNoJob`; `*TickError` for a queued row with
    `schedule = 1`; `*StateError{Want: Cancelled}` for a final row, reading `job x is done: only a
    queued or running job is cancelled`; nil after `cancelStmt` changed one row.
  - `type TickError struct{ ID, Kind string }`: its `Error()` is
    ``job <id>: it is `<short kind>`'s queued tick; remove its schedule to stop it``.
  - `func (p *Panel) Cancel(ctx context.Context, id string) error`.
  - CLI: `jobsUsage` = `usage: alder jobs [retry <id> | retry --dead | run <Kind> | cancel <id>] [dir] [--db FILE] [--build-dir DIR]`.
    `jobsCancel(ctx, path, stores, id, stdout, stderr) int` prints
    `cancelled job <id> (<kind>)[ in silo <s>]`, then the note `a running app's workers stop it
    within a poll if it is running there` (and, for a silo, `retriedNotes`'s silo wording). Exits
    are 1 for refusals and 2 for store errors, as `jobsRetry` does.

- [ ] **Step 1: Write the failing tests.**
  - `TestAdmin_CancelRow`: queued → nil and `cancelled`; done → a `*StateError` with "only a queued
    or running job is cancelled"; a queued tick → a `*TickError` whose text contains "`Nightly`'s
    queued tick; remove its schedule to stop it"; unknown → `ErrNoJob`.
  - `TestDevJobs_Cancel`: POST `/_dev/jobs/{id}/cancel` → 303 and `cancelled`; on a tick → 409;
    on a done row → 409; cross-origin → 403. A queued row's page shows the Cancel form and the
    sentence. A tick's page has no Cancel form.
  - `TestJobs_Cancel` (CLI): `alder jobs cancel <id>` on a queued row → exit 0 and the line; on a
    tick → exit 1 with the tick text; on a done row → exit 1; an unknown id → exit 1. `--help`
    shows `cancel <id>`.
- [ ] **Step 2: Run** `go test ./rt/jobs ./rt/web ./cmd/alder -run 'CancelRow|TestDevJobs_Cancel|TestJobs_Cancel'`.
  Expected: FAIL.
- [ ] **Step 3: Implement** as in Interfaces.
- [ ] **Step 4: Run** `go test ./rt/jobs/... ./rt/web/... ./cmd/alder -run 'Job|Jobs|Panel|Admin'`.
  Expected: PASS.
- [ ] **Step 5: Commit** `jobs: Cancel on the Jobs page and alder jobs cancel (#283)`.

### Task 13: The Intent example end to end, the book, close out

Spec "Intent" and "Success", §7 last bullet, §9, "Follow-ups to file".

**Files:**
- Modify: `testdata/corpus/jobs-ok/app/Signup.alder` (or `Exports.alder`): `BuildExport` gains
  `priority 10`; the `Stop` route (`DELETE "/exports/{job}" { job: Job.Id }`, `Job.cancel`,
  redirect to `Export`); `cmd/alder/test_test.go` (count)
- Modify: `book/17-jobs.md` ("Enqueueing a job": `cancel` in three sentences from spec §3; "A page
  that waits for its job" gains `Stop`; "Retries and idempotency": "cancelled is final and kept a
  week; retry it by hand"; "Long jobs": the lock sentence; "Silos": the autocommit sentence;
  "Testing jobs": the order and `Cancelled`; "The Jobs page" and "`alder jobs`": Cancel and
  `cancel <id>`; "What is not there yet": keeps `--env`, Fly workers and streaming, and gains
  `enqueueWith`), `book/09-stdlib.md`, `book/10-testing.md`, `book/D-stdlib.md` (regenerated),
  `CHANGELOG.md` (Breaking: `Cancelled`, "add a `Cancelled ->` arm to a `match` on `Job.State`";
  Added: `Job.cancel`)
- Test: jobs-ok via `TestTest_Jobs`, `go test ./book/...`

- [ ] **Step 1: Write the failing tests** in jobs-ok:
  - `test "a cancelled job shows Cancelled and does not run"`: `Test.submit Start {}`, then
    `Test.submit Stop` for that id. `Test.get (Export { job: id })` shows `Cancelled`, `Test.work`
    runs nothing, and `Job.status` gives `state == Cancelled`.
  - `test "cancel of a finished or unknown job is False"`: after `Test.work`, `Job.cancel id` →
    False. `Job.cancel (Job.Id "00000000")` → False.
  - `test "a match on Job.State covers Cancelled"`: a function that matches all five constructors
    compiles and gives `"cancelled"` for `Cancelled`.
- [ ] **Step 2: Run** `go test ./cmd/alder -run TestTest_Jobs`. Expected: FAIL.
- [ ] **Step 3: Implement**: the corpus routes, the count, then the book and CHANGELOG. Regenerate
  appendix D with `go test ./book -run TestAppendixD_Current -update`.
- [ ] **Step 4: Run** `go test ./...`. Expected: PASS.
- [ ] **Step 5: Commit** `book: cancelling a job; jobs-ok end to end (#283)`. Open PR 3. After it
  merges, file the spec's four follow-ups (`Job.enqueueWith`, "skip next" tick, the #165 cancelled
  outcome as a comment, the alder-tui `cancelled`/`priority` columns), close #283, tick roadmap
  #52, run `go install ./cmd/alder`, and rebuild the alder.nvim parser.
