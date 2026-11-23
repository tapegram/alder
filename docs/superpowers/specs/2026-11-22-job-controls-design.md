# Job controls: `Job.cancel`, `Job.enqueueIn`, priorities, and a `Job.Id` a page can carry

Design for #283, the follow-up filed when #168 landed (spec `2026-11-15-background-jobs-design.md`
§2 "Not in v1" and "Calls to check" 8; book 17 "What is not there yet"). Written without the user:
every choice is the author's, and the ones a reader would most plausibly overturn are listed first.
Line references are to main at the time of writing (98b36736 plus the silo-hints PR).

## Decisions (open questions ruled by the user 2026-11-22: every recommendation accepted)

1. **A fifth `Job.State`, `Cancelled`.** A cancelled job is a row that says so, in the panel and
   in `Job.status`. Adding a constructor to a stdlib union is breaking: an app's exhaustive
   `match` on `Job.State` stops compiling until it adds the case (chapter 3 lists the missing
   patterns). Alternatives: delete the row (no new state, but `Job.status` cannot tell "cancelled"
   from "pruned", and a cancel leaves no trace, against the v1 anti-goal "never only a row"), or
   `Dead` with `error = "cancelled"` (muddles "dead" and makes `retry --dead` revive cancels).
   *Recommendation: `Cancelled`, under Breaking in the CHANGELOG.*
2. **Cancelling a running job is honoured, not refused.** The row is marked at once; the attempt
   is cut off at its next effect step in the process that runs it, within a poll in another
   process, and as the backstop its done mark fails and its transaction rolls back at its end.
   The alternative is queued-only cancel (`False` for a running row): simpler, and what the issue
   half-suggests. *Recommendation: honoured; the mechanism is §3 and is about 80 lines.*
3. **`Job.cancel : Job.Id -> Task Bool`.** `True` when the row was queued or running and is now
   cancelled; `False` when there is no such row, it is already done, dead or cancelled, or it is a
   schedule's queued tick. The alternative, `Task Unit` that fails on `False`, makes "already
   finished" an error a page has to catch. *Recommendation: `Bool`.*
4. **Priority is per kind, a declaration option `priority N`**, not per enqueue. Per enqueue needs
   either `enqueueWithPriority`/`enqueueOnceAt`-style combinations or an options record
   (`Job.enqueueWith { at, key, priority }`), which changes the whole `Job` surface for a need
   nobody has stated. *Recommendation: per kind; `enqueueWith` stays a follow-up.*
5. **Higher runs first; any whole number; default 0.** River numbers 1 (first) to 4; Unix nice
   runs lower first. "Priority 10 beats priority 0" reads naturally in Alder's book.
   *Recommendation: higher first.*
6. **Strict priority, no ageing.** A steady stream of higher-priority jobs starves lower ones;
   the book says so and says what priority is for (the welcome mail before the nightly digest,
   not fairness). Ageing (an effective priority that rises with waiting) makes the claim's
   `ORDER BY` an expression no index serves. *Recommendation: strict, documented.*
7. **Jobs schema 2** adds a column and replaces an index; a build older than this change reads
   claimed rows with `RETURNING *` into fifteen columns, so its claims fail (a warn line a minute)
   while it runs beside the new build and after a rollback to it; its enqueues and `Job.status`
   work. From this change on, every read names its columns, so the next addition costs nothing.
   *Recommendation: accept, with the rollback sentence in chapter 17.*
8. **`Job.Id` becomes `brand Text where True`** (it is `type Id`, opaque, today). That makes it a
   route parameter, a session field, a table column and a job parameter with no new rule, and
   `Response.redirect (Export { job: id })` is how a page gets it. It also makes `Job.Id "x"`
   writable, which forges nothing: an id only looks a row up. *Recommendation: the brand.*
9. **`Job.enqueueIn` takes milliseconds** (`Int`), as `Task.sleep`, `Test.advance` and
   `Time.addMillis` do. A duration text (`"10m"`) parsed at run time would be the first run-time
   parse of a literal in the stdlib. *Recommendation: milliseconds.*
10. **A cancel from a silo-bound `Task` of a row in the shared store is an autocommit statement**,
    not part of the `Task`'s transaction (the silo-hints precedent, `rt/jobs/enqueue.go:74-80`).
    The alternative is to look in the bound store only, which cannot cancel the job of a `requires
    org` route that only enqueued (its row is in the shared store, spec §2 as amended).
    *Recommendation: the autocommit, said in the docs.*
11. **A schedule's queued tick cannot be cancelled** (`False`; 409 in the panel; refused by the
    CLI): the next housekeeping would queue the next tick anyway, so a cancel would only skip a
    slot by accident. "Skip tonight's run" is not in this design. *Recommendation: refuse.*
12. **Cross-process cancel of a running attempt within a poll**: one statement a second per store
    with attempts running in the process. The alternative is "at its end only" (the done-mark
    backstop), which makes a cancel of a forty-second AI call wait forty seconds.
    *Recommendation: the per-poll check.*

## Intent

v1 queues, runs, retries and shows jobs; it cannot take one back, cannot say "this before that",
and a page that starts a job cannot find it again without the app stringing an id through a table.
After this change:

```alder
job BuildExport = { user: User.UserId } priority 10

route Start = POST "/exports" { }

Start.update : Start.Params -> Session -> Task Response
Start.update p session = {
  let id <- Job.enqueue (BuildExport { user: session.user })
  Task.succeed (Response.redirect (Export { job: id }))
}

route Export = GET "/exports/{job}" { job: Job.Id }

Export.load : Export.Params -> Session -> Query (Maybe Job.Status)
Export.load p session = Job.status p.job

route Stop = DELETE "/exports/{job}" { job: Job.Id }

Stop.update : Stop.Params -> Session -> Task Response
Stop.update p session = {
  let _ <- Job.cancel p.job
  Task.succeed (Response.redirect (Export { job: p.job }))
}

remindLater : Task Job.Id
remindLater = Job.enqueueIn 3600000 (Jobs.Nightly.Nightly {})
```

Success: the example compiles; `Export` polls with `load={Export { job: p.job }} every="2s"` and
shows `Cancelled` after `Stop`; a cancelled running job's writes are not in the database; the mail
job claims ahead of the digest under load; a deployed app upgrades its jobs table at start on a
file and on Turso with nothing to run by hand; `alder test` sees the same order and the same cancel
semantics as the worker.

## Goals

1. `Job.cancel`: a queued job is taken back; a running one is stopped as soon as the runtime can
   and its transaction never commits; the row says `Cancelled`.
2. `Job.enqueueIn ms`: sugar, one line in the runtime.
3. `priority N` on the declaration: the claim takes the highest priority due row first, in the
   worker and in `Test.work`, with an index that keeps the claim O(log n).
4. `Job.Id` usable where a `Text` brand is: route parameter, session field, column, job parameter.
5. The jobs table moves to schema 2 at open, on every store (file, silo, Turso), once, safely with
   two processes opening at once; the panel, `alder jobs`, metrics and the book follow.

## Non-goals

- Per-enqueue priority, named queues, ageing, rate limits (question 4, 6).
- `enqueueOnceAt`, `enqueueOnceIn`, `enqueueAtOnce`: the combinations wait for `enqueueWith`.
- `Job.cancelAll kind`, cancel by `enqueueOnce` key: `enqueueOnce` gives the id; cancel that.
- Skipping a schedule tick (question 11).
- Interrupting pure computation: an attempt is cut off between effect steps, as the timeout is
  (book 17 "The options").
- A `Job.Id` on `Response` itself: a `Response` is a redirect, a render or a refresh; what the next
  page needs travels in the route's parameters or the session, which the brand makes possible.
- Results, #165's push, the alder-tui Jobs screen, `alder jobs --env`.

## Anti-goals

- A cancelled job whose writes are in the database. The done mark is the transaction's last write
  (v1 §3); it now refuses when the row is no longer running, so a job cancelled during its run
  rolls back at its end even when nothing cut it off sooner. The one exception is v1 §6's window
  (row store ≠ run store): the mark is after the commit, and the book says so.
- A second schema mechanism. The upgrade is what v1 §5 promised ("a later column is added by the
  runtime at open, not by the app's migrations"), gated by `jobs_schema`.
- A claim that sorts the queue. `ORDER BY priority DESC, run_at, created` is served by one index.
- A rule a page has to learn to carry an id. The brand gives `Job.Id` every boundary a `Text`
  brand has today.

## Constraints

- `rt.Job` is `{Kind, Params}` built by the generated constructor; `Job.enqueue` reads it
  (`rt/jobs/enqueue.go:54`). The runtime has no kinds table at enqueue time; the worker has one
  (`Options.Kinds`).
- The claim is one statement (`rt/jobs/claim.go:18`); `scanRow` reads columns positionally from
  `RETURNING *`. `admin.go` and `rt/testing/jobs.go` read by name (`rowCols`).
- The done mark is a `Call` inside the job's Task when the row is in the run's store, a second
  transaction after it otherwise (`rt/jobs/run.go:169-182`).
- `Job.status` looks in the bound store, then the shared store (`StatusOf`, `db.RuntimeReads`).
- Drift ignores `_alder_%` tables; `EnsureJobsTable` runs at every open and on `--migrate-only`,
  with `CREATE … IF NOT EXISTS` and `INSERT … DO NOTHING` for `jobs_schema` (`rt/db/migrate.go:907`).
- The checker's scalar rule (`internal/check/decls.go:652`) admits any `*types.Brand`; `Job.Id`
  is a `*types.Con` today, special-cased to `string` in `internal/gen/types.go:64`.
- Matches are exhaustive (chapter 3).

## Design

### 1. `Job.Id` is a brand

`stdlib/Job.alder`: `type Id = brand Text where True`, with the doc "a job's id, 8 lowercase hex
characters; a brand of `Text`, so a route, a session field, a table column or another job can carry
it; `Job.Id` of any text is a valid id that names no job". `internal/gen/types.go:64` loses its
case (the IR erases the brand to `string`, which every `std.Job*` function already uses);
`rt/std/job.go` is unchanged. The checker needs nothing: `urlRepresentable`, `storable`,
`formRepresentable` and the job-parameter rule all take a brand. `Show` and `==` are `Text`'s.
`Test.jobs`/`Test.work` rows keep `id: Job.Id`.

`where True`, not an eight-hex check, because no stdlib brand has a predicate yet (`Text.Email`
is `where True` too) and because a wrong id costs nothing: `Job.status` is `Nothing`, `Job.cancel`
is `False`. **(my call)** A page that takes `{ job: Job.Id }` therefore shows its own "no such job"
state, as it would for a pruned row; the book's example does.

The two shapes the book teaches: the id in the URL (`Response.redirect (Export { job: id })`),
for a page about one run; and the id on the row the job is about (`table Export = { …, job:
Maybe Job.Id }`), when the thing outlives the run. Nothing is added to `Session`; a session field
`export: Maybe Job.Id` now compiles for an app that wants it.

### 2. `Job.enqueueIn`

```alder
enqueueIn : Int -> Job -> Task Id   -- milliseconds from now; 0 or less is now
```

`std.JobEnqueueIn(ms, j)` is `jobs.Enqueue(e, j, now(e).Add(ms), nil)`: the Env's clock, so
`Test.setClock`/`Test.advance` govern it. Trace step `"Job.enqueueIn SendWelcome"`. The book's
`remindTomorrow` example becomes one line; chapter 17's "What is not there yet" loses its bullet.

### 3. `Job.cancel`

```alder
cancel : Id -> Task Bool
```

A write `Call` (it never runs in a `Query`), not a write for E0031, like `enqueue`. Trace step
`"Job.cancel a1b2c3d4"`.

- **Where it looks**: the places `StatusOf` reads (`db.RuntimeReads`): the bound store, then the
  shared store. The first place that has the row is where the statement runs:

  ```sql
  UPDATE "_alder_jobs" SET "state" = 'cancelled', "finished" = ?, "lease_until" = NULL
  WHERE "id" = ? AND "state" IN ('queued', 'running') AND "schedule" = 0
  ```

  One row changed is `True`; none is `False` (no such row, final already, or a queued tick:
  `schedule = 1`; a *running* tick has `schedule = 0` since its claim and is cancelled like any
  run). In the bound store the statement is in the Task's transaction, so a Task that fails
  afterwards un-cancels. In the shared store reached from a silo-bound Task it runs on
  `db.On(reg.Shared(), nil)` at once (question 10). `attempts`, `error`, `started`, `worker` are
  kept: the panel shows what the job was doing when it was cancelled.
- **The key**: a cancelled row is not `queued` or `running`, so `_alder_jobs_once` releases its
  key and `enqueueOnce` makes a new row. A retry of a cancelled row (§6) is refused while another
  row holds the key, as a dead row's is.
- **A running attempt, in the cancelling process.** `pool.run` derives the attempt's context with
  `context.WithCancelCause(runCtx)` and registers the cancel in a package-level `running
  sync.Map` (id → cancel) for the attempt's duration, beside `wake`. `Cancel` calls
  `env.AfterCommit(func() { cutOff(id) })`: the next step of the job's Task sees `Ctx.Err()` and
  `eff.Run` rolls back; an `Http.*` call in flight is cut. `RunOne` checks `context.Cause(base) ==
  errCancelled` as it checks `errStopped` and records nothing (the row is already cancelled):
  `Outcome.Cancelled`.
- **A running attempt, elsewhere** (another instance, the `--jobs-only` service, `alder jobs
  cancel`): the pool runs one watcher goroutine while it has attempts running, which every
  `o.Poll` asks each store that has one `SELECT "id" FROM "_alder_jobs" WHERE "id" IN (…) AND
  "state" = 'cancelled'` and cuts those off (question 12). A silo's statement goes through
  `p.at`, as a claim does; a closed silo is not opened for it (the attempt there has the store
  open anyway).
- **The backstop: the done mark refuses.** The in-transaction mark becomes `… WHERE "id" = ? AND
  "state" = 'running'`. Zero rows: the mark reads the state; `cancelled` makes the `Call` fail
  with `errCancelled`, so `eff.Run` rolls the job's writes back and the attempt is `Cancelled`;
  any other state (a lease that expired and re-queued the row, v1's case) applies the mark without
  the guard, as today, since the job did run. The out-of-store mark (`run.go:169`) is already
  guarded by `state = 'running' AND worker IS ?`; zero rows there with a cancelled row is the warn
  line "ran to its end before the cancel was seen; its writes stand" (v1 §6's window, documented).
- **A job that cancels itself** (`Job.cancel` of its own id in `run`): the row goes cancelled in
  the job's own transaction, the done mark refuses, the Task fails with "cancelled" and rolls
  back, cancel included; the row's cancel was in the rolled-back transaction, so the row is
  still `running` and goes back to queued at its lease. That is a nonsense program, and the book
  says "to stop early, `Task.succeed ()`". **(my call: not special-cased.)**
- **Stop vs cancel**: `stopJobs` still re-queues what it was running (`errStopped`); a cancelled
  attempt is not re-queued. `RunOne` tells them apart by the cause.
- **Lock contention**: a cancel of a job that is mid-write waits for its transaction like any
  write (`busy_timeout` 5 s, then the cancelling Task fails); chapter 17 "Long jobs" gains the
  sentence. A job in its read-and-call phase holds nothing, and the cancel lands at once.

### 4. Priorities

- **Declaration**: `priority N` joins `every`, `cron`, `retries`, `timeout`, `silo` (any order,
  each at most once). N is an integer literal, negative allowed (`priority -5`), default 0,
  higher first. Not a whole number is E0050 (the job-options code), wording in the existing style.
  `rt/jobs.Kind` gains `Priority int`; `rt.Job` gains `Priority int`, set by the generated
  constructor, so `Enqueue` writes it without a kinds table (`==` on `Job` values compares `Kind`
  and `Params` as before; `Priority` is a function of `Kind`). **(my call)**
- **The row**: `priority INTEGER NOT NULL DEFAULT 0`, written by `Enqueue` from the value, by
  `EnsureTicksOn`/`ClaimedTick` from the kind, by `Manual` from the kind (`alder jobs run` reads the
  app already), and kept by a retry.
- **The claim**: `ORDER BY "priority" DESC, "run_at", "created" LIMIT 1`, and `RETURNING` names
  its columns (`rowCols`, plus `"priority"`), so `scanRow` reads by position of a list the code
  owns, not of the table (question 7). `rt/testing/jobs.go`'s `dueRows` orders the same way
  across the test's stores, so a test sees the worker's order.
- **The index**: `_alder_jobs_claim ON ("state", "priority" DESC, "run_at", "created")` replaces
  `_alder_jobs_due ("state", "run_at")`. The claim's `WHERE state = 'queued' AND run_at <= ?` walks
  the index in claim order and skips not-yet-due rows per priority; the cost is bounded by the
  number of queued-but-future rows (`enqueueAt`, retries on backoff) at higher priorities, not by
  the queue. The other readers of `(state, run_at)` (`pendingStmt`, health's stalled check, the
  queued gauge, `prune`) are `state =` lookups and are served by the prefix. `bench_test.go` is
  re-run before and after, and `docs/perf/2026-11-jobs-throughput.md` gets the two numbers; if the
  claim loses more than 10 % the old index is kept beside the new one. **(my call)**
- **Fairness**: within a store, strict. Across stores, the v1 round robin (`pool.claim` takes the
  stores in turn) is untouched: a silo's priority 0 row can be claimed before the shared store's
  priority 10 row; the book says priority orders one store's queue. Health's `"stalled"` is
  unchanged (a starved row is due and claims happen, so it is `ok`); the panel's queued list is in
  claim order, so the starved rows are at its bottom.
- Schedule ticks take their kind's priority; `retries`, `timeout`, backoff ignore it.

### 5. Schema 2 and its upgrade

`createJobs` gains the column and the new index for fresh tables; `jobsSchema = "2"`. After the
`CREATE … IF NOT EXISTS` statements, `EnsureJobsTable` reads `jobs_schema` and, when it is `1`,
runs in one `BEGIN IMMEDIATE` transaction:

```sql
ALTER TABLE "_alder_jobs" ADD COLUMN "priority" INTEGER NOT NULL DEFAULT 0;   -- STRICT allows it
DROP INDEX IF EXISTS "_alder_jobs_due";
CREATE INDEX IF NOT EXISTS "_alder_jobs_claim" ON "_alder_jobs" ("state", "priority" DESC, "run_at", "created");
UPDATE "_alder_meta" SET "value" = '2' WHERE "key" = 'jobs_schema';
```

re-reading `jobs_schema` inside the transaction so a second process opening at once finds `2` and
does nothing. The `ADD COLUMN` is guarded by `pragma_table_info` for a table a hand edit already
widened. It runs wherever `EnsureJobsTable` runs: the shared file at start, each silo when it is
opened and `ok`, each Turso child the same way, and `--migrate-only`. The start scan's read-only
pass (`rt/jobs/active.go`) does not upgrade: it reads a schema-1 silo by column name and the silo
upgrades when `p.at` opens it through `Stores` for a claim. On Turso the same statements go over Hrana; libsql
supports `ALTER TABLE ADD COLUMN` on STRICT tables. A silo in maintenance is not opened and is not
upgraded until it is served, as v1 says.

**Older builds.** An older build's `INSERT` names its columns (`priority` defaults), its
`Job.status`, `Test.*` and admin reads name theirs; only its claim's `RETURNING *` breaks (fifteen
destinations, sixteen columns): a warn line, then one a minute (`claimLog`), until it stops. During
an Ssh drain or a Render roll that is thirty seconds; after a rollback it is until the next
deploy, and `/_alder/health` says `"jobs": "stalled"` after five minutes. The older build's
`JobStatusOf` refuses a `cancelled` row ("none of queued, running, done and dead": a 500 on that
page). Chapter 17 "Rolling back" gains the bullet; CHANGELOG under Breaking.

### 6. The `Cancelled` state everywhere it shows

- `stdlib/Job.alder`: `type State = Queued | Running | Done | Dead | Cancelled`; `Status.runAt` is
  `Nothing` for it; `error` is what the last attempt recorded, if any. `std.JobStateTag` gains
  `JobCancelled`; `RegisterUnion` the name.
- **Housekeeping**: pruned with done rows (`KeepDone`, 7 days): a cancel is not a failure to keep
  for a month. **(my call)** `leases` only looks at running rows; a row cancelled while running has
  `lease_until = NULL` and is left alone.
- **Retry** (`admin.Retry`): takes `dead` or `cancelled` (`WHERE "state" IN ('dead',
  'cancelled')`, the `StateError` wording "only a dead or cancelled job is retried"); `RetryDead`
  and `alder jobs retry --dead` stay dead-only.
- **Listing**: `Listing`/`JobsView` gain `Cancelled []Row` (the last fifty by `finished`, like
  done; additive in `/_dev/jobs.json`); `JobRow` gains `priority`. `Counts` picks the state up by
  itself.
- **Log and metrics**: `Outcome.Cancelled`; `outcome()` `"cancelled"`; the attempt's line is
  info: `job Signup.SendWelcome a1b2c3d4 cancelled 120ms attempt 1`; `alder_jobs_total{outcome=
  "cancelled"}` counts attempts cut off (a queued row's cancel is no attempt and is only the
  queued gauge dropping). A cancel of a queued row logs one info line under the cancelling
  request's id: `job Signup.SendWelcome a1b2c3d4 cancelled (was queued)`.

### 7. Tests

- `Test.jobs` shows `Cancelled`; `Test.work` runs `queued` rows only, highest priority first, then
  oldest (`dueRows`). A job that enqueues a higher-priority job sees it run before the rest of the
  same call's due rows (the loop re-reads due rows after each run already).
- `Job.cancel` in a test is the same statement in the test's transaction (`row.In.Tx`); a cancel
  from inside a job of another running job cannot happen (one job at a time); a cancel of a
  queued row from `run` works. The done-mark guard is shared through `RunOne`, so the self-cancel
  case of §3 gives `Err "cancelled"` in `Test.work`'s result too.
- Go tests in `rt/jobs` with the `Clock` and an in-memory store cover: cancel of a queued row;
  the in-process cut-off (an attempt blocked on a channel, cancelled from another goroutine, its
  transaction rolled back, `Outcome.Cancelled`, nothing recorded twice); the watcher (a row
  cancelled behind the pool's back is cut off within a poll); the backstop (an attempt whose row
  was cancelled by a direct `UPDATE` ends with its writes absent); the tick refusal; the key
  release; claim order under priorities with ties on `run_at`; the schema upgrade from a table
  created by the schema-1 statements (kept verbatim in the test) and its idempotence with two
  openers; the older-build claim failing (the old `RETURNING *` statement against the new table,
  to pin the documented behaviour); the bench numbers.
- `internal/gen/testdata/e2e_jobs_test.go.txt` gains the example of "Intent" minus the page's
  view: enqueue, redirect to a route with a `Job.Id` parameter, `Test.get (Export { job: id })`,
  cancel, status `Cancelled`.

### 8. The dev panel and `alder jobs`

- `/_dev/jobs`: a **Cancelled** section after Dead (last fifty); queued rows show `priority` when
  it is not 0 and are in claim order; a row's page has **Cancel** (POST `/_dev/jobs/{id}/cancel`,
  303 back) on queued and running rows, with the sentence "a running job is stopped at its next
  step in this process, within a second in another, and its writes are undone", 409 on a tick
  ("it is `Nightly`'s queued tick; remove its schedule to stop it") and on a final row; **Retry**
  on cancelled rows with the dead row's wording. `Panel.Cancel` joins `Retry`/`RunNow` through
  `change`; after the statement it calls `cutOff(id)` for an attempt of this process.
- `alder jobs`: counts include cancelled; `alder jobs cancel <id>` marks the row in the file (a
  running app's watcher sees it within a poll while the silo is in its active set, else at the
  attempt's end: the same note `retriedNotes` prints for silos); refuses a tick and a final row
  with the panel's wording; `alder jobs retry <id>` takes a cancelled row; queued rows are printed
  with their priority when not 0. The usage line gains `cancel <id>`.

### 9. The book and the docs

- Chapter 17: the options table gains `priority 10` ("claimed before lower; 0"); "Enqueueing a
  job" gains `enqueueIn` and `cancel` with the semantics of §3 in three sentences, and loses the
  `Time.addMillis` paragraph; a new section **"A page that waits for its job"** after "Where the
  job is stored", with the `Export` example (URL shape and column shape); "The table" gains the
  column and the claim statement its `ORDER BY`; a new subsection **"Priorities"** under "How the
  queue works" (strict, per store, starvation, what it is for); "Retries and idempotency" gains
  "cancelled is final and kept a week; retry it by hand"; "Long jobs" the lock sentence; "Silos"
  the autocommit sentence; "Testing jobs" the order and `Cancelled`; "The Jobs page" and
  "`alder jobs`" their buttons and verbs; "Rolling back" the schema-2 bullet; "What is not there
  yet" keeps `--env`, Fly workers, streaming, and gains `enqueueWith`.
- Chapter 9's `Job` summary and chapter 10's jobs section follow; chapter 6's parameter table needs
  nothing ("a brand of those"). Appendix D regenerates from `stdlib/Job.alder` (every new
  declaration carries the #119 doc with a checked example over `Jobs.Nightly.Nightly {}`).
- `internal/langdoc`: `priority`. `tree-sitter-alder`: `priority` as a job-line keyword (alder.nvim
  parser rebuild; the LSP's highlighting list). `alder new job` is not changed (no `--priority`;
  the line is one word to edit). CHANGELOG: Breaking (`Cancelled`; schema 2 and older builds),
  Added (the four features).

## What is left to the builder

- The `running` registry's shape (a `sync.Map` keyed by id, or on the pool with a package lookup
  like `actives`), and whether the watcher is a goroutine or a branch of `pool.work`'s idle select
  (a busy worker never idles, so a goroutine).
- `Cancel`'s place lookup: reuse `StatusOf`'s loop, returning the place with the row, then run the
  statement on its `Tx` or autocommit.
- The upgrade's exact guard (`pragma_table_info('_alder_jobs')`) and its Turso test (the
  `stores_turso_test` pattern).
- `rt.Job`'s `Priority` field versus a `CASE kind` in `Enqueue`: the field is the plan; the
  generated constructor already knows the kind's options.
- PR split, each green with its tests and book text:
  1. `Job.Id` brand + `Job.enqueueIn` (§1, §2): stdlib, gen's dropped case, docs, e2e example,
     chapter 17's page section, CHANGELOG Added.
  2. Schema 2 + priorities (§4, §5): migrate, named `RETURNING`, claim, `Test.work` order, the
     option through parser/checker/E0050/langdoc/tree-sitter/LSP, panel and CLI columns, bench,
     chapter 17 and the rollback bullet, CHANGELOG Breaking.
  3. `Job.cancel` + `Cancelled` (§3, §6, §7, §8): stdlib, runtime, watcher, done-mark guard,
     retry, panel and CLI verbs, metrics and log, chapters 9/10/17, CHANGELOG Breaking.
  PRs 1 and 2 are independent; 3 builds on 2's named `RETURNING` and `Listing` shape. All three
  touch `stdlib/Job.alder`, chapter 17 and the CHANGELOG, so they merge to main one after another
  (no long-lived branches).

## Follow-ups to file when this lands

- `Job.enqueueWith { at, key, priority }` if a per-enqueue priority or a keyed delay is asked for.
- Skipping a schedule tick from the panel ("skip next").
- #165: a cancelled outcome on the finished channel, so a watching page updates without polling.
- The alder-tui Jobs screen reads the new `cancelled` array and `priority`.
