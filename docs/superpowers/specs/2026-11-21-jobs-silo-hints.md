# Task 15 design: silo jobs enqueued by a web instance with `ALDER_JOBS=0`

Problem (carried from PR 4): a web instance with `ALDER_JOBS=0` runs no `Start`, so it has no
active set (`actives.m[o.Stores]` is registered only past the `Workers <= 0` return,
`rt/jobs/worker.go:119-140`); `touch` finds nothing (`rt/jobs/active.go:183-187`). A silo job it
enqueues reaches the `--jobs-only` worker only at that worker's next start scan
(`rt/jobs/active.go:249-276`, once per Start: `:250`).

## Decision

**A hint table in the shared store, written intent-first by the enqueuing process when it runs
no workers, read and cleared every poll by the `--jobs-only` process. No periodic rescan.**

- Table, added to `createJobs` (`rt/db/migrate.go:869-887`), so it exists wherever `_alder_jobs`
  does — Ensure `rt/db/sqlite.go:619`, Migrate/`--migrate-only` `rt/db/migrate.go:394`, the Turso
  shared database `rt/db/stores_turso.go:417` (called by the generated `Open`,
  `internal/gen/caps.go:545`); it is also created, empty and unused, in silos
  (`rt/db/stores.go:611,823`, `rt/db/stores_turso.go:47`):

  ```sql
  CREATE TABLE IF NOT EXISTS "_alder_jobs_silos" (
    "key"     TEXT    NOT NULL,   -- the silo's key, "org"
    "value"   TEXT    NOT NULL,   -- its value
    "touched" INTEGER NOT NULL,   -- ms since the epoch, the enqueuer's env.Now
    PRIMARY KEY ("key", "value")
  ) STRICT;
  ```

  `jobs_schema` stays `1` (`rt/db/migrate.go:864`): nothing reads it yet (`:900` only writes
  it), `CREATE TABLE IF NOT EXISTS` is idempotent, and the drift check treats every `_alder_%`
  table as the runtime's (`rt/db/sqlite.go:799-806`), so an older build sees nothing new.
- **Writer: `Enqueue`** (`rt/jobs/enqueue.go:67-75`). After the INSERT took (`n == 1`) and
  *before returning* — inside the Task's silo transaction, so before its commit — when the row
  is in a silo (`at.Key != ""`), `reg` is a non-test `*db.Stores` (new accessor
  `(*Stores).Testing()`, beside `Shared()` at `rt/db/stores.go:299`) and this process has no
  active set (`activeOf(reg) == nil`, `rt/jobs/active.go:175-179`), run on
  `db.On(reg.Shared(), nil)`:

  ```sql
  INSERT INTO "_alder_jobs_silos" ("key", "value", "touched") VALUES (?, ?, ?)
    ON CONFLICT ("key", "value") DO UPDATE SET "touched" = excluded."touched"
  ```

  with `touched = env.Now().UnixMilli()`. An error is returned: the Task fails and rolls back, so
  **a committed silo row always has a hint**. The existing `AfterCommit` branch (`touch` + `Wake`,
  `:69-74`) is kept for a process with an active set; `Wake` is still called in both cases (it is
  harmless without workers). Precedent for touching the shared connection while bound to a silo:
  `keyExists` (`rt/db/stores.go:455-469`). Nothing in `rt/db` changes in a transaction's shape:
  the hint is an autocommit statement on another database file (or Turso database).
- **Reader: `Options.Hints bool`** (`rt/jobs/worker.go:37-68`), set only by the generated
  `--jobs-only` main (Task 15's new branch beside `internal/gen/caps.go:961`; `StartJobs`
  `:454-457` gains the flag). In `pool.work`, before `p.claim` (`rt/jobs/worker.go:313-314`),
  `p.hints(claimCtx, now)`: pool-level, at most once per `o.Poll` (a `sync.Mutex` and a
  `hintAt time.Time` on `pool`, `:201-216`, compared with `o.Now`), so eight workers cost one read a
  second, not eight. (Amended in PR 6, Task 18b: the throttle is a `time.Ticker` of `o.Poll`
  plus a first-read token, `pool.hintTick`/`hintDue`, not `o.Now`, so a frozen test clock still
  reads every poll; a tick also wakes an idle worker, which reads before its claim, so a hint
  waits at most one `o.Poll` for an idle worker.) It runs on `db.On(p.o.Store, nil)`:
  `SELECT "key", "value" FROM "_alder_jobs_silos" ORDER BY "touched" LIMIT 100` (`HintBatch =
  100`); for each row `p.active.add(siloID{key, value})` (`rt/jobs/active.go:64-73`) then
  `DELETE FROM "_alder_jobs_silos" WHERE "key" = ? AND "value" = ?`. The claim that follows in the
  same loop iteration looks in the silo (`rt/jobs/worker.go:253-301`). An error is a warn line
  through `claimed` with the store name `the silo hints` (`rt/jobs/active.go:326-343`: first
  failure, then one a minute, one line when it works again). No change to the start scan,
  `keepSilos`, `each` or `Health`.

**Exact behaviour.** Enqueue → claim latency on `workers:` deploys, with a worker idle: at most
one poll (1 s: the next tick of the hint ticker wakes an idle worker, which reads the hints) plus
one claim turn (with every worker busy, the hint is read when the first of them finishes its
attempt and looks again), instead of "the worker's next start". Cost: one upsert per silo enqueue on the
web instance (one Hrana round trip inside the request's silo transaction on Turso — milliseconds,
far under §6a's 2 s mark), one SELECT a second per `--jobs-only` process on the shared store it
already claims from eight times a second, and nothing per silo. Spurious hints (hint committed,
Task rolled back or the process died before the silo commit) make the worker poll an empty
silo once; it leaves the set after `ActiveFor` (`rt/jobs/active.go:119-136`). A hint read by
two worker processes (SELECT before the other's DELETE) puts the silo in both sets; both claim
atomically, and the duplicate leaves after `ActiveFor`. A worker that cleared a hint and died
before claiming is restarted (Render's worker type) and its start scan finds the queued row
(`rt/jobs/active.go:259`); a worker crash is the only path that still waits for a start.

## Why this over the alternatives

- **Periodic rescan** (the plan's first option): `ScanSilos` opens every silo database
  (`rt/db/stores.go:1383-1402`); on Turso that is `remote.Children` per key plus `remote.Open` and
  a query per child (`rt/db/stores_turso.go:104-137`) — thousands of HTTP round trips per rescan,
  breaking §7's "thousands of idle silos cost one scan at start and nothing afterwards", with
  latency equal to the interval. Not taken even as a safety net: intent-first ordering plus
  fail-on-error closes the cross-store crash window the rescan would cover.
- **Hint written in `AfterCommit`**: a crash between the silo commit and the hint loses the job
  until the next start, which forces the rescan back in.
- **A marker row in the shared `_alder_jobs`** (an internal kind claimed like a job): it would
  show in the queued gauge, Health, the panel and `alder jobs` (`each`, `rt/jobs/active.go:219`),
  and leave done rows per enqueue to prune; a two-column table is smaller.
- **Watermark read (no DELETE, `touched > last`)**: `touched` comes from the web instances'
  clocks, and skew between two of them makes a watermark miss hints; delete-on-read uses no
  clock on the reader.
- **Rate-limiting the writer** (skip a hint for a silo hinted recently): unsafe — a worker
  restarted between the hint and the next enqueue has only its start scan, so the second row waits
  for yet another restart. One upsert per silo enqueue is the price; the table is bounded by the
  number of silos.
- **Reading hints in every worker process**: hints exist only when some process runs
  `ALDER_JOBS=0`, and that deploy shape has a `--jobs-only` reader (§7a; the dev split runs the
  same), so a web process with workers would only add a Turso round trip a second for an always
  empty table. Gated by `Options.Hints`: the single-process case (Ssh, Fly, Render without
  `workers:`) writes and reads nothing — unchanged. `ALDER_JOBS=0` with no worker anywhere
  accumulates at most one row per silo and runs nothing, which is what `0` means.

## Changes

- `rt/db/migrate.go:860-887`: the table in `createJobs`; comment at `:860`.
- `rt/db/stores.go:299`: `func (s *Stores) Testing() bool { return s.test }`.
- `rt/jobs/enqueue.go:34-41` (doc), `:67-75`: the hint write.
- `rt/jobs/worker.go:37-68` `Options.Hints`; `:201-216` pool fields; `:313` the call; doc
  `:88-105`. `rt/jobs/active.go` after `:187`: `HintBatch`, `(*pool).hints`.
- `rt/jobs/export_test.go`: nothing new needed (tests read the table with `db.On(shared, nil)`).
- `internal/gen/caps.go:454-457, :961` and the new `--jobs-only` branch: pass `Hints: true`;
  goldens regenerated and read.
- `docs/superpowers/plans/2026-11-16-background-jobs.md:817-837`: Task 15 steps gain the tests
  below; the constraint list (`:40-48`) gains "`--jobs-only` reads the silo hints every poll
  (`HintBatch` 100)".

## Tests (`rt/jobs/active_test.go`, package `jobs_test`, on `siloApp`/`orgEnv`/`startSilos`)

- `TestHint_WrittenWithoutWorkers`: no `Start`; `runEnqueue(orgEnv A)` leaves one
  `_alder_jobs_silos` row `(org, A, t0 ms)` in `shared`; a second enqueue at `t0+1s` updates
  `touched` and the row count stays 1; an enqueue on `sharedEnv` writes no row; `Woken()` is true.
- `TestHint_NotWrittenWithWorkers`: `startSilos` on `s`, then the same enqueue: no hint row,
  `Active(s) == ["org/A"]` (today's path, unchanged).
- `TestHint_OnlyJobsOnlyReads`: workers started on `reopened(dir)` with `Hints: false`, `scanned`;
  enqueue on `s` (no workers): no outcome within 50 ms, the hint row remains; stop; start on a
  second `reopened` with `Hints: true`: the outcome's `ID` is the enqueued id, the hint row is
  gone, `Active == ["org/A"]`.
- `TestHint_FailedTaskIsHarmless`: a Task that enqueues then fails (`eff.ErrFail`): no job row,
  the hint row exists; a `Hints: true` worker clears it, `Active == ["org/A"]`, no outcome; at
  `t0 + 1h + 1min` the silo leaves the set.
- `TestHint_ReadErrorRateLimited`: drop the table in `shared` before `Start` with `Hints: true`:
  one warn line `jobs: claiming a job in the silo hints: …` within a minute of clock; recreate it
  and the "works again" line follows.
- `rt/db/migrate_test.go` `TestEnsureJobsTable_SiloHints`: `Ensure` creates the table; running
  it twice is fine; `appTables` leaves it out.
- `internal/gen` golden: the `--jobs-only` main passes `Hints: true`; the serving main does not.

## Spec sentences to amend in Task 21

- §5 (`docs/superpowers/specs/2026-11-15-background-jobs-design.md:353-373`): after the jobs
  table, the `_alder_jobs_silos` table above, "in the shared store; a silo's copy is unused".
- §7 "Where workers look" (`:448-457`), append: "A process that runs no workers (`ALDER_JOBS=0`)
  records each silo it enqueues into in the shared store's `_alder_jobs_silos` before its
  transaction commits; the `--jobs-only` workers read and clear that table every poll, so such a
  job starts within a poll, not at the worker's next start. A process with workers writes no
  hint: its own active set has the silo."
- §7a (`:493`, after "sets `ALDER_JOBS=0` on the web service"): "… whose silo enqueues the worker
  service learns of through `_alder_jobs_silos` (§7)."
