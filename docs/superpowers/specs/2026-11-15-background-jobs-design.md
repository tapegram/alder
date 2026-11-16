# Background and scheduled jobs

Design for #168 (roadmap #52 item 9). The user skipped the design questions ("skip the questions.
We need something like how django has celery basically built in. Does Go already have an equivalent
we can build in or provide sugar for? and it needs to be incorporated into our deployment features,
running locally (django runs eagerly), etc."), so every choice below is the author's; the ones most
worth a veto are listed under "Calls to check" at the end. Builds on the effects model (`rt/eff`),
the test clock (`2026-10-29-test-clock-design.md`), silos (book 8) and the deploy targets (book 16).

## Intent

An Alder app can only do work while a request holds the connection. Sending the welcome mail,
calling an AI model, rebuilding a report, deleting stale rows at night: each is either done inside
`update` (the request waits; a failure after the commit is lost; `Email.send` has no outbox) or not
done at all. Django has Celery and Rails has Active Job; Alder has `Task.sleep`, documented as "a
request-scoped wait, not a scheduler".

After this change a job is a declaration beside the routes, enqueued from `update` with one effect
step, stored in the app's database in the same transaction as the writes that caused it, run by
the app's own process, retried with backoff, and visible in the dev panel, the log and the metrics.
A schedule is the same declaration with `every` or `cron`. Nothing is added to a deploy: no broker,
no worker service, no second binary. `alder run` runs jobs as it serves; `alder test` runs them
when the test says so, on the test's clock.

```alder
module Signup

import Table.User

job SendWelcome = { user: User.UserId }

SendWelcome.run : SendWelcome.Params -> Task Unit
SendWelcome.run p = {
  let user <- Db.get User p.user
  match user {
    Just u -> Email.send { to: u.email, subject: "Welcome", body: <p>Hello {u.name}</p> },
    Nothing -> Task.succeed {},          -- deleted before we got to it: nothing to send
  }
}

route Create = POST "/signup" { } form NewUser

Create.update : Create.Params -> Session -> Task Response
Create.update p session = {
  let id <- Db.insert User { email: p.form.email, name: p.form.name, passwordHash: "", role: Member }
  let _ <- Job.enqueue (SendWelcome { user: id })
  Task.succeed (Response.redirect (Todos.Index.Index {}))
}

job Nightly = {} cron "0 3 * * *"

Nightly.run : Nightly.Params -> Task Unit
Nightly.run _ = Log.info "cleanup ran"   -- the app's cleanup goes here

test "signing up sends the welcome mail" = {
  let _ <- Test.fake Email
  let out <- Test.submit Create { email: Text.Email "me@example.com", name: Text.NonEmpty "Me" }
  let jobs <- Test.jobs
  let runs <- Test.work
  Task.succeed (List.length jobs == 1 && List.all (\r -> r.result == Ok {}) runs)
}
```

Success: the example above compiles, its test is green, `alder run` sends the mail within a second
of the form post without the request waiting for it, the deployed app does the same on every
target with no new infrastructure, and a job that fails shows up in the dev panel and the log with
its error, its attempt count and when it runs next.

## Why not River or goqite

The user asked whether Go already has the equivalent to build in or sugar over. It does:

- **River** (`github.com/riverqueue/river`, MPL-2.0, v0.51.2 of 2026-11-09) is the Go Celery:
  transactional insert, unique jobs, retries with backoff, periodic jobs, leader election, a UI.
  It gained a SQLite driver in v0.23 that its docs still call experimental ("the schema may still
  change"), it wants a one-connection pool, and it polls an outbox table in place of
  LISTEN/NOTIFY. River core pulls in pgx, robfig/cron, gjson/sjson, testify and x/sync (about
  twelve direct dependencies). Durable periodic jobs are River Pro.
- **goqite** (`maragu.dev/goqite`, MIT, v0.4.0) is one SQLite table with `SendTx`/`ReceiveTx`,
  delay and priority; no schedules, no backoff.

Neither fits, for three reasons that are each enough:

1. A job's body is Alder, run by `eff.Run` against an `Env`, inside the Task's own lazy
   transaction. Both libraries dispatch to Go worker functions; the bridge would be one Go worker
   kind carrying "module, name, encoded params", and everything they offer around the worker
   (typed args, worker registries, middleware) would go unused.
2. Tests drive time with `rtesting.Clock`; `Test.advance (3 * 86400000)` must make the nightly job
   due. River's scheduler and periodic enqueuer read their own clock and run on their own
   goroutines; a deterministic `Test.work` is not something it offers.
3. alder has one direct dependency (`modernc.org/sqlite`) and a schema it owns: the jobs table has
   to be an internal table that `alder migrate` ignores, created the way `_alder_meta` is, present
   in every silo file and on Turso. Twelve dependencies and a migration tool of its own, for an
   experimental driver, is the wrong trade.

So the queue is built in (`rt/jobs`, a few hundred lines of Go around one table), and the design
borrows what River got right: enqueue in the caller's transaction, a state column with attempts and
`run_at`, a lease on running rows, uniqueness by key, schedule ticks as ordinary rows. The "sugar"
is the `job` declaration, the `Job` stdlib module, `Test.jobs`/`Test.work`, `alder new job` and
the dev panel's Jobs page. **(my call)**

## Goals

1. `job Name = { params }` beside the routes, with a `Name.run : Name.Params -> Task Unit` arm,
   enqueued with `Job.enqueue (Name { … })` from any `Task`.
2. Enqueueing is a write in the enqueuing Task's transaction: a job exists if and only if the
   writes beside it were committed. No outbox to write by hand.
3. Jobs run in the app's process, in development, in tests and in every deploy target, with no new
   service, and survive a restart (the queue is the database).
4. A failed attempt is retried with backoff; a job that keeps failing is kept with its error for
   someone to look at and retry.
5. `every "<duration>"` and `cron "<expr>"` schedules, correct with one process (Ssh, Fly) and with
   several (Render instances on Turso), without a leader.
6. Tests are deterministic: enqueued and scheduled jobs run when `Test.work` says so, on the test's
   clock, inside the test's transaction.
7. A job is visible: the log, the dev panel, alder-tui's event feed, the metrics endpoint, and
   `Job.status` for a page that waits for one.
8. `alder new job` writes the declaration, the arm and a test.

## Non-goals

- Named queues, priorities, rate limits, batches, workflows (job A then B), per-job concurrency
  limits. One queue, first due first. The row has room (`priority` is not a column; add it when
  needed).
- Results. A job gives `Task Unit`; what it computed goes into a table it writes. Celery's result
  backend is a second store with its own retention and is the part people regret.
- Push from a finished job to a page (#165). This design keeps the hook: the worker publishes
  "job finished" on an in-process channel that #165's watched route can subscribe to. Pages wait
  with `load={R} every="2s"` and `Job.status` meanwhile.
- A separate worker process or image. `ALDER_JOBS=0` lets an instance opt out of running jobs,
  which is enough to shape a dedicated worker on Render; nothing more.
- Cancelling a running job from the outside; `Job.cancel` for a queued one is a follow-up.
- Catching up missed schedule ticks one by one (see "Schedules": at most one late run).
- An `Env`/secrets module. Jobs read configuration the way routes do: through capabilities.

## Anti-goals

- A job that runs twice because of a crash between its commit and its bookkeeping. When the job
  row and the job's writes are in the same store, the "done" mark is a write *inside the job's
  transaction*; there is no window. When they are not (a siloed app, below), the window exists and
  the book says so.
- A deploy that needs one more thing. `alder deploy init`'s files do not change; no target gets a
  worker service; the health check does not gain a dependency.
- A test that sleeps or races. Nothing in the test runtime starts a goroutine for jobs.
- A silent failure. A dead job is a warn line, a row in the panel, a metric; never only a row.
- A write lock held across a slow call. A job's transaction begins at its first write, as a
  request's does; an `Http.post` to an AI model before the first write holds nothing.
- A second dependency for a cron parser.

## Constraints

- SQLite, one writer, `BEGIN IMMEDIATE`; on Turso, the same over HTTP with higher latency.
- A Task is one transaction on one store (E0031) and can be run `Within` another (savepoints).
- A job has no session: `session.user` does not exist, so the scoped `Db` operations are E0026 in
  a job, as they are outside a `requires user` route.
- Runtime-owned tables are the `internalTables` list in `rt/db/sqlite.go` and are created with
  `CREATE TABLE IF NOT EXISTS` at open; they are not migrated by `alder migrate`.
- Two builds can run at once (Ssh's 30 s drain; Render's roll; `alder run`'s child swap).
- The test clock (`rt/testing/clock.go`) is the only clock a test sees: `Env.Now`.

## Design

### 1. Declaring a job

```alder
job SendWelcome = { user: User.UserId }
job Digest      = { org: Org.OrgId } silo org
job Hourly      = {} every "1h"
job Nightly     = {} cron "0 3 * * *"
job Charge      = { invoice: Invoice.InvoiceId } retries 0 timeout "30s"
```

- `job Name = { fields }` declares a job in any module, like `route`. It defines `Name.Params`
  (the record), the constructor `Name : Name.Params -> Job` (a `Job` value, comparable with `==`
  like a `Route` value is), and requires the arm `Name.run : Name.Params -> Task Unit` in the same
  module. A missing or mistyped arm is reported the way a route's missing `update` is (same code,
  wording "job" in place of "route"; builder's choice of which existing code).
- Params are the types a table column can hold, plus records, lists and `Maybe` of them, and the
  app's own types without functions in them. Functions, `Task`, `Query`, `Html`, `Doc`, `Job`,
  `Route` and `Answer` are refused: **E0049** "A job parameter that cannot be stored", naming the
  field and the type. Params are stored as JSON. There is no generic encoder in the runtime
  (tables encode column by column in generated code; a route value is `rt.Route{Name, Method,
  Path, Query}` with its parameters already rendered into the path and query), so the generator
  emits, per job, `encode : Params -> rt.JsonValue` and `decode : rt.JsonValue -> (Params, error)`
  the way it emits a table's row codec, over the `Json.Value` tree (`rt/json.go`) with its
  `JsonEncode`. Field order, `Maybe` as null/value, brands as their base, custom types as
  `{"tag": …, "args": […]}` **(my call)**. A renamed or retyped field between builds makes the
  old rows undecodable: such a row goes dead with the decode error (see §6), it is not silently
  dropped.
- At run time a `Job` value is `rt.Job{Kind string, Params string}` (the kind's qualified name and
  the encoded params), built by the generated constructor; `==`, `Show` and Dict keys are those
  of the pair, as `rt.Route`'s are. `Job.enqueue` reads Kind and Params from it; the worker looks
  the Kind up in the generated table (§3) to decode and run. **(my call)**
- Options after the record, in any order, each at most once:
  - `every "<duration>"`: the grammar of the polling attribute (`ms`, `s`, `m`), plus `h` and `d`.
    Minimum `1s`. **(my call)**
  - `cron "<expr>"`: five fields (minute, hour, day of month, month, day of week), numbers, `*`,
    `,`, `-`, `/`; no names, no seconds, no `L`/`W`/`#`. UTC, like every `Time`. Parsed at compile
    time and at run time by one package, `rt/jobs/cron` (about a hundred lines; no dependency);
    the checker importing a runtime package has precedent (`internal/check/deploy.go` imports
    `rt/applog`, `browser.go` imports `rt/selector`).
  - `every` and `cron` together, or either with a non-empty params record, is **E0050** "A
    scheduled job takes no parameters" / "A job has two schedules" (one code, two messages, the
    way E0030 covers every deploy-block mistake). A malformed expression is E0050 too, with the
    field it could not read.
  - `retries N` (N ≥ 0): attempts after the first. Default 4 (five attempts). **(my call)**
  - `timeout "<duration>"`: the most one attempt may take. Default `5m`, maximum `1h`. **(my call)**
  - `silo k`: the job runs bound to the silo whose key `k` is the value of its `k` parameter (the
    admin form's wording for the same thing). The parameter must exist and have the key's type.
    Without `silo`, the job runs on the shared store. In an app without silos, `silo` is refused
    (the existing rule for forms applies).
- `run` is a Task arm: everything a route's `update` may do except read a session. `Session.put`
  in a job is a type error (there is no `Session` in scope) and, where a helper reaches it, the
  checker refuses it the way it refuses it in `load`. E0031 applies to `run` as to any arm.
- The scoped `Db` ops are E0026 in `run`; the E0026 explanation gains the sentence "A job's `run`
  has no session either: give the job the user as a parameter and use the unscoped operations."

Tooling touched by a new keyword: the parser and formatter, `tree-sitter-alder` (highlighting,
alder.nvim's parser rebuild), the LSP (symbols, hover on the `job` keyword through
`internal/langdoc`, search: a job is a declaration `alder search` finds), `alder routes` is not
touched (jobs are not routes). `page`/`msg` precedent: `internal/check/removed.go` shows where a
keyword's shape is checked.

### 2. Enqueueing: the `Job` module

`stdlib/Job.alder`, signature-only with the #119 docs (summary, edges, checked example, `Book:`
line), implemented in `rt/std` as `eff.Call`s and mapped in `internal/gen/stdcall.go`:

```alder
module Job

type Id                                   -- a job's id, 8 hex characters like a request's
type State = Queued | Running | Done | Dead
type Status = { state: State, attempts: Int, error: Maybe Text, runAt: Maybe Time }

enqueue     : Job -> Task Job.Id          -- as soon as a worker is free
enqueueAt   : Time -> Job -> Task Job.Id  -- not before this time
enqueueOnce : Text -> Job -> Task Job.Id  -- with a key: while a job with this key is queued or
                                          -- running, nothing is added and its id comes back
status      : Job.Id -> Query (Maybe Status)
```

- `enqueue` is a write Call: it inserts the row through `env.Transaction()`, so it is in the
  enqueuing Task's transaction and rolls back with it. It writes to the store the Task is bound to
  (`db.Enter`'s: the shared store, or the silo of a `requires org` route). It is **not** a write for
  E0031's purposes, like `Session.put`: it binds the Task to no store and follows whichever it is
  in, so a `requires org` route that only enqueues, and one that writes a project and enqueues,
  both compile.
- `enqueueOnce key`: the key is scoped to the job kind (`SendWelcome` and `Digest` may share
  `"u1"`). The uniqueness is a partial unique index on `(kind, key) WHERE state IN ('queued',
  'running')`; a conflicting insert returns the existing row's id. `Done` and `Dead` rows release
  the key.
- A job may enqueue jobs (`Job.enqueue` in `run`), including itself.
- `status` reads the store the Query is bound to (`db.Enter`'s, as `Db.get` would), then the
  shared store if that was a silo and the row was not there: the row is where the enqueuing
  route put it, and a page that waits for a job runs under the same `requires` as the route that
  made it, so the first look finds it. It is `Nothing` once the row is pruned (§5) or in a silo
  the Query is not bound to (documented; a `Job.Id` does not name its store). **(my call)**
- Not in v1: `enqueueIn` (write `Time.addMillis` on `Time.now`), `cancel`.

### 3. Running: the worker

`rt/jobs` is a package the generated `main` starts after `app.Serve` returns its handler and stops
before `stopBackup()`:

```go
stopJobs := jobs.Start(ctx, jobs.Options{Stores/Store, Kinds: app.Jobs(), Env: app.JobEnv, Workers: n, Log: lg, Metrics: m, Dev: dev})
...
srv.Shutdown(grace)
stopJobs(grace)   // cancels attempts still running after the grace; they are re-queued, not counted
```

- `app.Jobs()` is generated: the kinds this build knows (`"Signup.SendWelcome"` → decode params,
  build the Task, options). `app.JobEnv(ctx, kind)` builds an `Env` the way `router.newEnv` does
  (Store/Stores, Caps, `Now: time.Now` — the router's `opts.Now` so `Test`'s override is the same
  hook, Log through the job's id, Rec in dev mode, Stats when metrics are on, Session nil, Keys
  from the `silo k` parameter).
- **Workers**: `ALDER_JOBS=N` goroutines, default 4; `0` means this instance runs no jobs (and no
  schedule ticks). **(my call)** Each worker loops: claim one due row, run it, repeat; with none
  due, wait for the poll interval or a wake.
- **Claim**: one short `BEGIN IMMEDIATE` transaction: `UPDATE _alder_jobs SET state='running',
  attempts=attempts+1, lease_until=?, worker=? WHERE id = (SELECT id FROM _alder_jobs WHERE
  state='queued' AND run_at <= ? AND kind IN (<known kinds>) ORDER BY run_at, created LIMIT 1)
  RETURNING *`. Several instances claim from the same Turso database without a leader because the
  update is atomic. `kind IN (…)` leaves rows of a kind this build does not know (an older build
  during a roll, or a renamed job) to a build that does; a row no build claims for a day is a
  warn line from the housekeeping tick.
- **Run**: `eff.Run(env, KTask, Bind(run(params), Call("Job.done", write, markDone)))`: the done
  mark is the Task's last write, so it is in the job's transaction when the row's store is the
  run's store. A job that never writes gets a one-statement transaction at the end. The `Env.Ctx`
  carries the attempt's timeout; a Task past it fails with "timed out after 5m" (the Call loop
  checks Ctx before each step; `Http.*` honours it mid-call).
- **Failure** (`Task.fail`, `Task.notFound`, a capability's error, a panic, the timeout, an
  undecodable params row): the Task's transaction is rolled back by `Run`, then a second short
  transaction records it: `error`, and either `state='queued', run_at = now + backoff(attempts)`
  or, past `retries`, `state='dead'`. Backoff after attempt 1, 2, 3, 4: 10 s, 1 min, 10 min, 1 h;
  after the 5th attempt, dead; with `retries N > 4` the backoff stays 1 h. **(my call)** A panic
  is logged with its stack at error level, the way the router logs a route's panic.
- **Lease**: `lease_until = now + timeout + 30 s`. A running row past its lease (the process died)
  is re-queued by the housekeeping tick with `error = "lost: the process stopped"`; that attempt
  counts, so a job that crashes its process every time still goes dead.
- **Shutdown**: `stopJobs(grace)` stops claiming at once, waits up to the grace for running
  attempts, then cancels their Ctx; a cancelled attempt is re-queued with `attempts - 1` (not
  counted) and no error. The old build in an Ssh drain therefore hands its jobs to the new one
  within seconds.
- **Polling and wake**: poll every 1 s **(my call)**; in addition, a committed `Job.enqueue` in
  this process signals the workers (a channel), so in development a job starts within
  milliseconds of the request that made it, which is what "runs eagerly" is for. With several
  instances, another instance's enqueue is seen at the next poll.
- **Housekeeping** (one tick a minute, any worker): re-queue expired leases, create the next
  schedule rows (§4), prune `done` rows older than 7 days and `dead` rows older than 30 days
  **(my call)**, warn about unclaimable kinds.
- **Development**: `alder run`'s child runs the same worker. On a rebuild the old child is stopped
  through stdin EOF: its running attempts are cancelled and re-queued, and the new child claims
  them. `--migrate-only` and the other maintenance runs start no worker.

### 4. Schedules

A schedule tick is an ordinary row with `schedule = 1` and `scheduled_for`, so it is claimed,
retried, logged and shown like any job, and several instances agree through the database:

- At most one `queued` schedule row per kind: a partial unique index on `(kind) WHERE schedule = 1
  AND state = 'queued'`.
- When the worker starts and on every housekeeping tick, for each scheduled kind: `INSERT OR
  IGNORE` a row with `run_at = scheduled_for = next(now)`. `next` for `cron` is the first matching
  minute strictly after `now`; for `every "1h"` it is `now + 1h` rounded down to the interval when
  the interval divides a day (so instances and restarts land on the same grid), else `now + 1h`.
- When a tick is claimed, the claimer inserts the next one: `next(max(scheduled_for, now))`. Hence
  a tick missed while the app was down runs late, once, and the following one is on the grid; a
  three-day outage of a nightly job gives one late run, not three. Retries of a tick keep its
  `scheduled_for`; the next tick's row already exists, so a tick that retries past the next tick
  can overlap with it (documented; `enqueueOnce` is not involved).
- A kind removed from the app leaves its queued tick unclaimable; housekeeping deletes queued
  schedule rows of unknown kinds after a day (a renamed schedule is not a lost job, it is the next
  tick of the new name).
- Manual run: `alder jobs run Nightly` (§9) enqueues a non-schedule row of the kind with `{}`.

### 5. Storage

One internal table per store (the shared store and each silo file or Turso child database),
created with the other `_alder_*` tables, named in `internalTables`:

```sql
CREATE TABLE IF NOT EXISTS "_alder_jobs" (
  "id"            TEXT    NOT NULL PRIMARY KEY,   -- 8 hex, as request ids
  "kind"          TEXT    NOT NULL,               -- "Signup.SendWelcome"
  "params"        TEXT    NOT NULL,               -- JSON, at most 64 KB (E at enqueue: *ErrFail)
  "key"           TEXT,                           -- enqueueOnce's key
  "state"         TEXT    NOT NULL,               -- queued | running | done | dead
  "run_at"        INTEGER NOT NULL,               -- ms since the epoch, as Time
  "created"       INTEGER NOT NULL,
  "started"       INTEGER,
  "finished"      INTEGER,
  "attempts"      INTEGER NOT NULL DEFAULT 0,
  "error"         TEXT,                           -- last failure, at most 4 KB
  "lease_until"   INTEGER,
  "worker"        TEXT,                           -- "<host>:<pid>:<n>"
  "schedule"      INTEGER NOT NULL DEFAULT 0,     -- 1 for a tick
  "scheduled_for" INTEGER
) STRICT;
CREATE INDEX IF NOT EXISTS "_alder_jobs_due" ON "_alder_jobs" ("state", "run_at");
CREATE UNIQUE INDEX IF NOT EXISTS "_alder_jobs_once" ON "_alder_jobs" ("kind", "key") WHERE "key" IS NOT NULL AND "state" IN ('queued', 'running');
CREATE UNIQUE INDEX IF NOT EXISTS "_alder_jobs_tick" ON "_alder_jobs" ("kind") WHERE "schedule" = 1 AND "state" = 'queued';
```

The table is versioned by a row in `_alder_meta` (`jobs_schema = 1`), so a later column is added by
the runtime at open, not by the app's migrations. `alder dump` and the admin generator skip
internal tables already. Backup ships it with the rest of the file.

`internalTables` is an explicit list today; this change makes the drift check treat every
`_alder_%` table as the runtime's **(my call)**, so the next internal table needs no edit. A build
compiled by an alder older than this one still has the list and sees `_alder_jobs` as an unknown
table: in production its drift check refuses, in development `Ensure` recreates the schema. That
is the existing rule for any internal table added since that build (`_alder_deploy_*` set the
precedent), and the book's rollback section says so in one sentence: do not `alder rollback`
across an alder upgrade that added a table without a `--migrate-only` of the newer build first.

The table is created wherever the runtime opens a store for serving: the shared store at start,
a silo when it is opened and in state `ok` (after any migration it applies), a Turso child the same
way, and by `--migrate-only`. A silo in maintenance (`pending-data-loss`, `ahead`, `failed`,
`orphaned`) is not served, so nothing enqueues into it and the worker does not poll it.

### 6. Failure, retries, idempotency

What the book says, in one paragraph, and what the runtime does:

- A job's attempt is one transaction: it commits whole, with the done mark, or not at all. For a
  job that only writes the database, that is exactly-once. For a job that calls out (`Email.send`,
  `Http.post`) and then writes, a crash after the call and before the commit sends the mail twice;
  the book says to write such a job so a second run is harmless (look before you send; keep a
  `sent` column; use `enqueueOnce` to collapse duplicates at the source), as every queue's manual
  does.
- A siloed app (§7) has the window between the silo transaction's commit and the shared row's
  done mark; same advice, and a note that it is rare (a crash in that window).
- `retries 0` for a job that must not run twice at any price; then a failure goes dead at once.
- The error text is the Task's failure message, the capability's error, "timed out after …",
  "lost: the process stopped", the panic's first line, or "cannot decode the parameters: …".
- Dead jobs are retried by hand from the dev panel or `alder jobs retry <id>` (attempts reset to
  0, state queued). A dead schedule tick is retried the same way and does not disturb the next
  tick's row.

### 7. Silos, Turso and several instances

- **Row store.** `Job.enqueue` writes the row in the store its Task is bound to: a `requires org`
  route's enqueue lands in that org's silo file. This keeps enqueueing atomic with the route's
  writes, which is the point of the design.
- **Run store.** A job runs on the shared store unless declared `silo k`, in which case
  `db.Narrow(env, k, value)` + `db.Enter` bind it to the silo named by its parameter. The row's
  store and the run's store are independent; when they differ, the done mark is a second
  transaction after the run's commit (§6's window), recorded as `WHERE state = 'running' AND id =
  ?` so a lease re-queue in between does not resurrect a finished job.
- **Where workers look.** The shared store always. A silo's table is polled while the silo is in
  the worker's *active set*: every silo listed by `SiloFiles`/`StatusSilos` at start (one pass),
  plus every silo an enqueue in this process touched since; a silo leaves the set after an hour
  with no row due. A crash forgets the set and the next start rebuilds it, so no silo's job is
  lost, only delayed to the next start in the worst case. Thousands of idle silos cost one scan at
  start and nothing afterwards. **(my call)** `IdleClose` is told the worker's poll is not use, so
  a silo with nothing queued still closes. Until the silo PR lands, the worker polls the shared
  store and the silos `Stores` currently has open (those a request touched since start), so a
  siloed app's jobs run from the first worker PR; the start scan and the active set close the
  gap of a silo that was closed with a row due.
- **Turso** (`ALDER_DB=libsql://`): the same statements over Hrana with `BEGIN IMMEDIATE`; the
  claim's latency is the HTTP round trip, fine at one poll a second. The app does not migrate on
  start there, but internal tables are created at open by the runtime, as `_alder_lock` is;
  `alder deploy`'s `--migrate-only` creates it too, so the first build with jobs finds it.
- **Several instances** (Render `instances: N`): all run workers; claims are atomic; schedule ticks
  are rows with a unique index, so one instance wins each tick and no leader exists. `ALDER_JOBS=0`
  on the web instances and a second Render service of the same image with `ALDER_JOBS=8` is the
  "dedicated worker" shape, documented, not generated.
- **Two builds at once** (Ssh drain, Render roll, `alder run` swap): both claim; a row's kind
  unknown to one is left to the other; the stopping build re-queues what it was running. The jobs
  schema version in `_alder_meta` lets a newer build add a column while the older still reads
  (only additions, ever).
- **Fly** (one machine, replaced in place): nothing special; a tick missed in the restart's few
  seconds runs late by those seconds.

### 8. Development and tests

- **`alder run`**: jobs run in the child as in production; the dev panel (§9) shows them; the
  event feed carries `job` events so alder-tui's Logs screen shows them with the requests.
- **`alder test`**: the test runtime starts no worker. `Job.enqueue` inside a test (directly, or
  through a browser step's request) inserts into the test's in-memory database, inside the test's
  transaction, so it is visible to the test and gone with the case. Two steps:

  ```alder
  Test.jobs : Task (List { id: Job.Id, job: Job, status: Job.Status })    -- every row, oldest first
  Test.work : Task (List { id: Job.Id, job: Job, result: Result Text Unit })
  ```

  `Test.work` runs every job due at the test clock's time, oldest first, each as its own Task
  `Within` the test's transaction with a savepoint (a failing job's writes are undone to its
  savepoint and its row is updated the way the worker would: attempts, error, `run_at` with the
  same backoff, or dead), and keeps going while the run made new due rows, up to 1000 runs in one
  call (then the test fails with "Test.work: jobs keep enqueueing jobs"). Schedule ticks exist in
  tests too: the first `Test.work` of a case creates the first tick rows from the clock, so
  `Test.setClock` to 02:59 UTC, `Test.advance 120000`, `Test.work` runs `Nightly` once. The
  returned list says what ran and how it ended; `Test.jobs` afterwards shows what is left and when.
  In a test of an app with silos, both steps cover the test's shared store and every test silo
  store (`db.NewTestStores`), oldest row first across them.
- `Test.fake` capabilities apply in jobs, since the job's `Env` is the test's. `Test.trace` of a
  `Test.run` lists `Job.enqueue SendWelcome` as a step.
- One core, two drivers **(my call)**: `rt/jobs` exposes `RunOne(env, row) Outcome`, which decodes,
  builds the Task with the done mark, runs it, and on failure records the attempt (the bookkeeping
  given the store, or the transaction it is handed); the worker loop (claiming, polling, leases,
  housekeeping, shutdown) and `Test.work` (due rows at the test clock, savepoint per job, the same
  `RunOne`) are two callers of it. Alder tests exercise `RunOne`; the loop is tested in Go with a
  `Clock` and an in-memory store. When the row's store and the run's store differ, `Test.work`
  does what the worker does (the done mark as its own statement after the job's savepoint is
  released) and does not simulate the crash window.
- The test clock is read through `Env.Now` everywhere in `rt/jobs` (never `time.Now`), so the
  package is tested in Go with a `Clock` too.

### 9. Observability and operations

- **Log**: one line per attempt at info with the job's id as the line's id, the way requests log:
  `job Signup.SendWelcome a1b2c3d4 done 120ms attempt 1`; a failure is warn with the error and
  the next time (`failed attempt 2/5 "smtp: timeout", next in 1m`); dead is error. `Log.*` inside
  a job goes under the job's id. Effect steps at debug, as for requests.
- **Dev panel**: `/_dev/jobs` lists queued (with their due time), running, dead, and the last
  fifty done; a row opens `/_dev/jobs/{id}` with the attempt's trace (the `Recorder`, as a
  request's), params, error, and buttons Retry and Run now; schedules are listed with their next
  time. `/_dev/jobs.json` feeds alder-tui; a Jobs screen in alder-tui is a follow-up issue.
- **Metrics** (`ALDER_METRICS`): `alder_jobs_total{kind,outcome}` (done, failed, dead),
  `alder_jobs_running`, `alder_jobs_queued`, `alder_job_duration_seconds{kind}` (the histogram
  requests use).
- **Health**: `/_alder/health` gains `"jobs": "ok" | "off" | "stalled"` (stalled: a row due more
  than five minutes and no worker claim in that time), without changing the status code. **(my
  call)**
- **CLI** `alder jobs` (dev database by default, `--db` for another, `--env <name>` not in v1):
  `alder jobs` lists counts and the dead rows; `alder jobs retry <id|--dead>`; `alder jobs run
  <Kind>` for a parameterless kind. Runs against the file directly, as `alder dump` does; the
  running app sees the change at its next poll.
- **`alder logs <env>`** needs nothing: the lines are there.

### 10. The generator

`alder new job SendWelcome user:User.UserId` writes `app/Jobs/SendWelcome.alder` **(my call on the
folder; `--into Module` places it elsewhere, as other generators do)**:

```alder
module Jobs.SendWelcome

import Table.User

job SendWelcome = { user: User.UserId }

SendWelcome.run : SendWelcome.Params -> Task Unit
SendWelcome.run p = {
  let _ <- Log.info "SendWelcome ran"
  Task.succeed {}
}

test "SendWelcome runs" = {
  let _ <- Job.enqueue (SendWelcome { user: User.UserId "u1" })
  let runs <- Test.work
  Task.succeed (List.map (\r -> r.result) runs == [Ok {}])
}
```

`--every 1h` / `--cron "0 3 * * *"` write a parameterless scheduled job and a test that sets the
clock to just before a tick, advances past it and works. The generator joins the registry in
`internal/generate/specs.go`, the ID list in `specs_test.go`, the usage strings in `cmd/alder`, the
LSP's generator list, and `book/15-generators.md`. Its template follows `capability.tmpl`'s shape.
`alder new resource`/`admin` do not change.

### 11. The book and the docs

A new chapter **17, "Background jobs"** **(my call: not a section of chapter 7, which is about the
effect model; this is a feature with its own operations story)**: the declaration, enqueueing,
what a job may do, retries and idempotency, schedules, tests, the panel, deploy notes per target,
and the "dedicated worker" shape. Chapter 7 gets a pointer where `Email.send` is introduced,
chapter 10 a section on `Test.jobs`/`Test.work`, chapter 16 a paragraph per target, chapter 9 the
`Job` module summary. `stdlib/Job.alder` and the two `Test` steps carry #119-style docs with
checked examples (the examples that need a job declaration go in `examples/todos`, which gains
`Jobs/Nightly.alder` as the example app's cleanup, so appendix D's examples run). `internal/langdoc`
documents the `job` keyword and its options. CHANGELOG under Added.

## What is left to the builder

- The exact parser/checker placement for the `job` declaration and its options; which existing
  "missing arm" code is reused; how `Job` values are represented at run time (the route value's
  representation is the model).
- The JSON encoder for params (reuse what `alder dump`/admin use) and the stored-type rule's
  implementation (the table-column rule plus records/lists/Maybe).
- The `rt/jobs` package layout: claim/run/finish/housekeeping, the cron parser shared with the
  checker, the Go tests with a `Clock`.
- The generated `app.Jobs()`/`app.JobEnv` shape and how the router's `opts.Now` reaches it. The
  constraint: the worker shares the router's open store(s), capabilities, clock and logger, and
  `app.Serve(caps, o) (http.Handler, error)` today opens the store inside and returns only the
  handler. Suggested **(my call)**: split it into `Open(caps, o) (*App, error)` with
  `App.Handler()`, `App.StartJobs(n) (stop func(context.Context))` and `App.Close()`, and keep
  `Serve` as the thin wrapper the existing callers and tests use; generated `main` calls the new
  shape. `Options` gains `Jobs int` (from `ALDER_JOBS`, default 4).
- Dev panel markup (follow `devrequests.go`), the event feed's `job` event shape (follow the
  request event), the metrics names if the existing ones suggest another convention.
- PR split. Suggested, each green on its own: (1) declaration + checker + `Job` module + table +
  enqueue, with `Test.jobs`/`Test.work` so it is testable from the first PR; (2) the worker in
  `main`, log, metrics, health, shutdown; (3) schedules (cron parser, ticks); (4) silos and the
  active set, Turso check; (5) dev panel, events, `alder jobs`; (6) generator, book chapter 17,
  tree-sitter/LSP/langdoc, CHANGELOG.

## Calls to check

The author's decisions a reader would most plausibly overturn, in order:

1. **Built in, not River.** River's SQLite driver is experimental, twelve dependencies, Go-shaped
   workers and its own clock; the queue here is one table and a few hundred lines.
2. **Syntax**: `job Name = { … }` with options `every "1h"`, `cron "0 3 * * *"`, `retries N`,
   `timeout "5m"`, `silo k`; arm `Name.run : Name.Params -> Task Unit`; `Job.enqueue (Name {…})`.
3. **Retries by default** (5 attempts, 10 s / 1 min / 10 min / 1 h), `retries 0` to opt out.
   Celery defaults to no retries; Active Job and River retry.
4. **In-process workers always**, default `ALDER_JOBS=4`, `0` to opt out; no eager mode in dev
   because the wake-on-commit makes the in-process worker as immediate.
5. **Schedules as rows** with a unique index instead of a leader; a missed tick runs late once,
   never many times.
6. **Row store = the enqueuer's store; run store = the declaration's.** The alternative (jobs only
   in the shared store; enqueue refused from silo-bound Tasks) is simpler and makes siloed apps
   unable to enqueue from their main routes.
7. **Done mark inside the job's transaction** (exactly-once for database-only jobs); a second
   transaction only when the row and the run are in different stores.
8. **No results, no priorities, no queues, no cancel** in v1.
9. **`Test.work` runs due jobs until quiet** (cap 1000), on the test clock, with the first tick
   rows created at the first `Test.work`.
10. **Retention** 7 days done / 30 days dead; **poll** 1 s; **timeout** 5 m default, 1 h max;
    params 64 KB.
11. **Health** reports `"jobs"` without changing the status code.
12. **Generator folder** `app/Jobs/`; **book chapter 17**.
13. **E0049/E0050** as the two new codes (first free after E0048; retired codes not reused).
14. **Per-job generated JSON codec** over `rt.JsonValue` and `rt.Job{Kind, Params}` as the
    value (there is no generic encoder to reuse; the per-table codec is the precedent).
15. **`internalTables` becomes a `_alder_%` prefix rule**, with the rollback caveat in the book.
16. **`Serve` split into `Open`/`Handler`/`StartJobs`/`Close`** so the worker shares the
    router's store, caps, clock and log.
17. **`RunOne` as the shared core**; Alder tests cover it, Go tests cover the loop.

## Follow-ups to file when this lands

- #165: subscribe a watched route to the worker's finished channel.
- `Job.cancel`, `enqueueIn`, priorities, a `Job.Id` on the response for a page to poll.
- alder-tui Jobs screen (`/_dev/jobs.json`).
- `alder jobs --env <name>` against a deployed target through the deploy adapters.
- `alder secret` (#237) is unrelated but comes up in the same "operate a deployed app" breath.
