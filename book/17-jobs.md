# 17. Background jobs

A request can only do work while it holds the connection. Sending a
welcome mail from `update` makes the person who signed up wait for the
mail server, and if the server is down, the `Task` fails, and the
sign-up with it. The mail was never the point of the request; it only
had to happen soon. Work that should happen at night, or every hour, has
nowhere to live at all.

A **job** is the answer. It is a declaration beside the routes: a name,
the parameters it needs, and a `run` arm that does the work. `update`
puts the job on a queue with one effect step, in the same transaction
as the writes that caused it, and the app's own process runs it a
moment later, outside any request. A job that fails is tried again
later; a job declared with `every` or `cron` runs on a schedule.

There is nothing new to install. The queue is a table in the app's own
database, and the workers that run it are goroutines of the app's own
process. This chapter covers:

- declaring a job and putting it on the queue;
- what a job may do;
- how the queue works inside, table, claim and all;
- retries, and writing a job that may run twice;
- long jobs, and fanning out;
- schedules;
- silos;
- testing jobs;
- watching them run;
- jobs on each deploy target, and how far it scales.

## Declaring a job

A job is declared with `job`, like a route with `route`:

```alder
capability Mailer {
  send : { to: Text.Email, subject: Text, body: Text } -> Task Unit,
}

fake Mailer {
  send mail = Task.succeed ()
}

job SendWelcome = { user: User.UserId }

SendWelcome.run : SendWelcome.Params -> Task Unit
SendWelcome.run p = {
  let found <- Db.get User p.user
  match found {
    Just u -> Mailer.send { to: u.email, subject: "Welcome", body: "Glad you are here." },
    Nothing -> Task.succeed (),
  }
}

test "the welcome mail is sent by the job" = {
  let _ <- Test.fake Mailer
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let job = SendWelcome { user: me }
  let _ <- Job.enqueue job
  let runs <- Test.work
  let own = List.filter (\r -> r.job == job) runs
  Task.succeed (List.map (\r -> r.result) own == [Ok ()])
}
```

`job SendWelcome = { user: User.UserId }` declares three things:

- **`SendWelcome.Params`**, the record of the job's parameters.

- **The constructor `SendWelcome`**, of type `SendWelcome.Params -> Job`.
  `SendWelcome { user: me }` is a `Job` value: it says which job to run
  and with what. Like a route value, it compares with `==`, and from
  another module it is written with the module's qualifier
  (`Jobs.Nightly.Nightly {}`).

- **The arm it needs**, `SendWelcome.run : SendWelcome.Params -> Task Unit`,
  in the same module. A job without its `run` is E0043, the code of a
  route without its `update`:

```alder error=E0043 "job `Cleanup` needs its `run`"
job Cleanup = {}
```

The `run` arm is a `Task`, so it runs as one transaction ("How the
queue works", below). The job above reads the user again instead of
taking the address as a parameter: a job runs later, and the row may
have changed or gone by then. `Nothing` is a normal case, not a failure.

### What a parameter may be

A job's parameters are stored in the database as JSON until a worker
runs the job. So a parameter must be a value that can be written down:
a type a table column can hold (`Int`, `Text`, `Time`, a table's id, a
brand), a record, a `List` or a `Maybe` of those, or one of the app's
own types without a function inside. Anything else is E0049:

```alder error=E0049 "A job parameter that cannot be stored"
job Later = { step: Int -> Int }
```

The JSON is written per field, in the order the record declares them:
`Maybe` as `null` or the value, a brand as its base, a `Time` as
milliseconds, a `Date` as `"YYYY-MM-DD"`, and a value of a custom type
as `{"tag": "Name", "args": […]}`.

Since the row is stored, a job outlives the build that queued it. A
build that changes a job's parameters meets rows the old build wrote,
and reads them strictly:

- **A renamed, retyped or new field makes old rows unreadable.** A row
  that cannot be decoded goes **dead** at once, with the error
  `cannot decode the parameters: …` naming the field, whatever the job's
  `retries`. That holds for a new `Maybe` field too: the old rows have
  no such key, and a missing key is an error, never a silent `Nothing`.
  A retry does not help, since the new build reads the row the same
  way. Before you change a job's parameters, let its queue drain; or
  declare a new job beside the old one, and remove the old one once its
  rows are gone.

- **Two `Float`s that are equal can make unequal jobs.** `0.0` is
  written as `0` and `-0.0` as `-0`, so for a job
  `Price = { amount: Float }`, `Price { amount: 0.0 }` and
  `Price { amount: -0.0 }` are not `==` (a `Job` value compares by its
  job and its written parameters). And `NaN` and the infinities have no
  JSON form: they are written as `null`, so a job queued with one
  cannot be read back and goes dead.

### The options

Options follow the parameters on the same line, in any order, each at
most once:

| Option | Means | Default |
|---|---|---|
| `every "1h"` | run on a fixed interval ("Schedules", below) | none |
| `cron "0 3 * * *"` | run when the cron expression matches | none |
| `retries 2` | the attempts after the first | 4 (five attempts) |
| `timeout "30s"` | the most one attempt may take, at most `1h` | `5m` |
| `silo org` | run in the silo the parameter `org` names ("Silos") | the shared store |

A duration is a whole number and a unit, `ms`, `s`, `m`, `h` or `d`.
`timeout` longer than `1h` is E0050, and so is `every` shorter than
`1s`:

```alder error=E0050 "is longer than the maximum, `1h`"
job Report = {} timeout "2h"
```

An attempt that runs out of time fails with `timed out after 5m` (its
own limit in place of `5m`). The limit is checked before each step of
the `Task`, and an `Http` call in progress is cut off; a pure
computation is not interrupted in the middle.

## Enqueueing a job

The `Job` module puts a `Job` value on the queue:

```alder
Job.enqueue     : Job -> Task Job.Id
Job.enqueueAt   : Time -> Job -> Task Job.Id
Job.enqueueIn   : Int -> Job -> Task Job.Id
Job.enqueueOnce : Text -> Job -> Task Job.Id
Job.status      : Job.Id -> Query (Maybe Job.Status)
```

- **`Job.enqueue`** queues the job, due now.
- **`Job.enqueueAt t`** queues it not before the time `t`.
- **`Job.enqueueIn ms`** queues it not before `ms` milliseconds from
  now; zero or less is now.
- **`Job.enqueueOnce key`** queues it unless a job of the same kind with
  the same key is queued or running; then nothing is added, and that
  job's id comes back. The key belongs to the kind, so `SendWelcome` and
  `Digest` may both use `"u1"`, and a job that is done or dead frees it.
- **`Job.status id`** is the job's state (`Queued`, `Running`, `Done`
  or `Dead`), its attempts so far, its last error, and while it is
  queued, when it is due; `Nothing` when there is no such job.

The first four give back the job's `Job.Id`, eight hex characters like a
request's id. A `Job.Id` is a brand of `Text` (chapter 4), so a route
parameter, a table column or another job's parameter may carry one.

The usual place is a write route's `update`:

```alder
form NewUser = { email: Text.Email, name: Text.NonEmpty }

route Signup = POST "/signup" { } form NewUser

Signup.update : Signup.Params -> Session -> Task Response
Signup.update p session = {
  let id <- Db.insert User { email: p.form.email, name: p.form.name, passwordHash: "" }
  let _ <- Job.enqueue (Jobs.Nightly.Nightly {})
  Task.succeed (Response.redirect (Auth.Login.Login {}))
}
```

(Here it queues the to-do app's own `Nightly` job; in a real sign-up it
would be `SendWelcome { user: id }`.)

The point is the transaction. The enqueue is a statement in the
`Task`'s transaction, like the `Db.insert` before it: if the `Task`
fails after it, the user's row and the job are both rolled back, and if
it commits, both are there. A job is never queued for a user that does
not exist, and a user is never created without the job.

A job may queue jobs too, itself included: `Job.enqueue` is a `Task`
step like any other.

Waiting is done with `enqueueIn`, not with a sleep:

```alder
remindTomorrow : Task Job.Id
remindTomorrow = Job.enqueueIn 86400000 (Jobs.Nightly.Nightly {})
```

A job's parameters take at most 64 KB as JSON; a larger job fails the
`Task` that queues it. Store the large thing in a table and pass its id.

### Where the job is stored

The job's row is written in the store the `Task` writes to. In an app
without silos that is the one database. In an app with silos (chapter
8, "Silos"), it is the silo of the org whose tables the route's
`update` writes, so the job commits with those writes. A route that only
queues a job writes no table, so it is bound to the shared store, and
its job is stored there.

An enqueue is not a write for E0031, the rule that a `Task` writes to
one store: it follows the store the `Task` is bound to. So a
`requires org` route that writes a project and queues a job compiles,
and so does one that only queues.

`Job.status` looks where the job was stored: first in the store its
`Query` reads (the silo of a page that `requires org`), then in the
shared store. A page that requires what the queueing route required
finds the job. A job in another silo is `Nothing`, and so is a job
removed after it finished: no sooner than a week after it is done, 30
days after it went dead ("Housekeeping", below).

### A page that waits for its job

A person who starts a job usually wants to see it finish: an export
that takes a minute, a report that is being built. The id that
`Job.enqueue` gives back is how the next page finds the job, and it can
travel in two ways.

For a page about one run, put the id in the URL. The write route
redirects to a page whose route takes a `Job.Id`, and that page reads
the job with `Job.status`:

```alder
job BuildExport = {}

BuildExport.run : BuildExport.Params -> Task Unit
BuildExport.run p = Task.succeed ()

route Start = POST "/exports" { }

route Export = GET "/exports/{job}" { job: Job.Id }

Start.update : Start.Params -> Session -> Task Response
Start.update p session = {
  let id <- Job.enqueue (BuildExport {})
  Task.succeed (Response.redirect (Export { job: id }))
}

Export.load : Export.Params -> Session -> Query { id: Job.Id, status: Maybe Job.Status }
Export.load p session =
  Job.status p.job |> Query.map (\status -> { id: p.job, status: status })

Export.view : { id: Job.Id, status: Maybe Job.Status } -> Session -> Html
Export.view model session =
  <html><body>
    {match model.status {
      Just status -> <p id="state" load={Export { job: model.id }} every="2s">{match status.state {
        Queued -> "Waiting for a worker",
        Running -> "Working",
        Done -> "Ready",
        Dead -> "Failed",
      }}</p>,
      Nothing -> <p id="state">No such job</p>,
    }}
  </body></html>
```

Any text is a valid `Job.Id`, so a person can type `/exports/abcd1234`
into the address bar: it names no job, `Job.status` is `Nothing`, and
the page says so, as it would for a job removed after it finished. The
view is the place for that case. (Only inside a `test` may code
write `Job.Id "00000000"` to make such an id; app code gets its ids from
`Job.enqueue`, a URL or a column.) The model carries the
id because the view has no route parameters: it is how the `<p>` names
its own page in `load={Export { job: model.id }} every="2s"`, which
fetches the page again every two seconds while the element is shown
(chapter 6, "Patterns").

When the thing outlives the run, put the id on the row the job is about
instead, and let any page that shows the row look the job up:

```alder fragment
table Export {
  id: ExportId @id,
  owner: User.UserId,
  job: Maybe Job.Id,
}
```

`Maybe`, because the row exists before the job is queued, or when none
is.

## What a job may do

`run` may do everything a write route's `update` may, except what needs
a session, since a job runs outside any request:

- **No `Session.put`.** It is E0003 in `run`, and in a helper `run`
  refers to.

- **No scoped `Db` operations** (`Db.mine`, `Db.getMine`, …): E0026,
  with "a job has no session; give the job the user as a parameter and
  use the unscoped operations".

- **No response.** `run` gives `Unit`; there is no page to answer.

- **No long sleeps.** `Task.sleep` is clamped to a minute, as
  everywhere (chapter 9, "Waiting"). To wait longer, queue the job with
  `enqueueAt`.

Capabilities (`Mailer.send`, `Http.post`), `Log.*`, `Time.now`,
`Random.*` and every `Db` read and write work as they do in `update`.

## How the queue works

The queue is small enough to read in an afternoon (`rt/jobs/`), and
knowing it makes the rest of this chapter obvious.

### The table

Every store has one table of jobs, `_alder_jobs`, created by the
runtime beside its other `_alder_` tables (never by your migrations):

```text
id            TEXT     8 hex characters, the job's id
kind          TEXT     the job's module and name: "Signup.SendWelcome"
params        TEXT     the parameters, as JSON
key           TEXT     enqueueOnce's key, or NULL
state         TEXT     queued | running | done | dead
run_at        INTEGER  when it is due (milliseconds since 1970)
created       INTEGER  when it was queued
started       INTEGER  when its last attempt started
finished      INTEGER  when it finished: done or dead
attempts      INTEGER  attempts so far
error         TEXT     the last failure, at most 4 KB
lease_until   INTEGER  while running: when the worker is presumed gone
worker        TEXT     while running: "<host>:<pid>:<n>"
schedule      INTEGER  1 while the row is its kind's queued tick
scheduled_for INTEGER  for a tick: the time it stands for
```

Three indexes do the work:

- `("state", "run_at")` finds the due rows.
- A unique index on `("kind", "key")`, only over the rows queued or
  running, is `enqueueOnce`: a second insert with the same key conflicts
  and is ignored, and the existing row's id comes back.
- A unique index on `("kind")`, only over the queued ticks, is the
  rule "one queued tick per schedule" ("Schedules").

A job is a row. Enqueueing is an `INSERT` with state `queued`. Nothing
else holds the queue: no memory, no broker, no file of its own. A
backup of the database is a backup of the queue.

### Claiming a job

A **worker** is a goroutine of the app's process. There are
`ALDER_JOBS` of them (default 4; `0` runs none). Each loops: claim one
due row, run it, claim again.

The claim is one statement in a short transaction of its own (`BEGIN
IMMEDIATE`, so it takes the write lock at once):

```sql
UPDATE _alder_jobs SET state='running', attempts=attempts+1, lease_until=?, worker=?, started=?
WHERE id = (SELECT id FROM _alder_jobs
            WHERE state='queued' AND run_at <= ? AND kind IN (…the kinds this build knows…)
            ORDER BY run_at, created LIMIT 1)
RETURNING *
```

(Simplified: `lease_until` is a `CASE` over the kinds, since each kind
has its own timeout; `rt/jobs/claim.go` has the real text.) It marks the
oldest due row running and hands it back. Because it is
one statement under the write lock, two workers never claim the same
row, whether they are in one process or in several processes on one
database. No worker is in charge; none needs to be.

`kind IN (…)` leaves alone a row of a job this build does not know:
the job of an older build still running beside it, or of a newer one,
waits for a build that knows it.

The **lease** is the claim's promise to finish: the job's timeout plus
30 seconds. A process that dies with a job running cannot say so; past
its lease, housekeeping takes the job for lost and queues it again.

### Running a job

The worker decodes the row's parameters, builds the job's `Task`, and
runs it as it runs a route's `update` (chapter 7, "How effects run"),
with one step added at the end: the **done mark**, an `UPDATE` that sets
the row to `done`. The mark is the `Task`'s last write, so it is in the
job's own transaction:

- the job's writes and the mark commit together, or neither does;
- a job that writes nothing commits the mark alone.

So a job that only changes the database runs **exactly once**: either
its writes are there and the row says done, or neither, and the job
runs again.

When the `Task` fails (`Task.fail`, a not found, a capability's error,
a panic, the timeout), its transaction is rolled back, and a second
short transaction records the failure in the row: the error, and
either the next attempt's time or `dead` ("Retries and idempotency",
below).

### Waking the workers

An idle worker looks for a due row every second. That alone would make
a job start up to a second after its request. So an enqueue, once its
`Task` has committed, also **wakes** the workers of its own process: a
job queued by a request starts within milliseconds. A worker that finds
a row wakes another, in case more are due. A job queued by another
process (another instance, or `alder jobs`) is seen at the next look,
within a second.

### Housekeeping

Once a minute, and when the workers start, one worker of each process
keeps the table (and, for the first two points, the tables of the silos
its workers look in):

- a running row past its lease is queued again with the error
  `lost: the process stopped`, and the attempt counts, so a job that
  crashes its process every time still goes dead;
- done rows are deleted a week after they finished, dead rows after 30
  days;
- each scheduled job gets its next tick if it has none ("Schedules");
- a job of a kind no running build knows, due for more than a day, is a
  warn line (once per kind).

### Stopping

When the app stops (`SIGINT` or `SIGTERM`), its workers stop claiming at
once, and the requests in flight and the jobs running share the same
five seconds to finish. A job a request enqueues in that time stays
queued for the next process. (`app --jobs-only`, which serves no
requests, does the same for its jobs alone.) After that, each job still running is cancelled and queued again as it
was before its claim: the attempt does not count, and its error is
cleared, so an earlier attempt's error is gone from the row too. So a
deploy, or a rebuild under `alder run`, hands
the running jobs to the next process within seconds. A process killed
outright leaves its jobs to their leases.

## Retries and idempotency

A failed attempt is tried again after a wait that grows:

| After attempt | Waits |
|---|---|
| 1 | 10 seconds |
| 2 | 1 minute |
| 3 | 10 minutes |
| 4 and later | 1 hour |

With the default `retries 4`, the fifth failure makes the job **dead**:
it stays in the table with its error and is not tried again until
someone retries it by hand (the Jobs page or `alder jobs retry`, below).
`retries 0` runs a job once: the first failure is dead.

The error is the text of what failed: the `Task.fail` message, the
capability's error, `timed out after 5m`, `lost: the process stopped`,
the first line of a panic (the whole stack is in the log), or
`cannot decode the parameters: …`.

### Writing a job that may run twice

An attempt is one transaction, so a job that only writes the database
runs exactly once. A job that calls out cannot have that promise. If
it sends a mail and then writes, and the process dies after the send and
before the commit, the job runs again and the mail goes twice. No queue
can prevent that; every queue's manual says the same thing. Write such
a job so that a second run is harmless:

- look before you act: read whether the work was done;
- keep a `sent` column, set in the same `Task` as the send, and check
  it first: then only a crash between the send and the commit repeats
  it;
- use `Job.enqueueOnce` to stop duplicates where they start;
- and for a job that must not run twice at any price, `retries 0`.

## Long jobs and fan-out

A job's `Task` is one transaction, and SQLite has one write lock. The
transaction begins at the job's first write and ends with the `Task`.
So a job that marks a row "started", then calls an AI model for forty
seconds, holds the write lock for forty seconds. Every request that
writes waits, and one that waits more than five seconds (the database's
busy timeout) fails.

The shape that avoids it: **read and call first, write last.** And for a
batch, **fan out**: one job reads the ids and queues one small job per
item, so each transaction is short and a failure retries one item, not
the batch:

```alder
job RemindAll = {} cron "0 8 * * *"

job Remind = { todo: Todo.TodoId }

RemindAll.run : RemindAll.Params -> Task Unit
RemindAll.run p = {
  let open <- Db.from Todo |> Db.where (Todo.done.eq False) |> Db.all
  let _ <- Task.all (List.map (\t -> Job.enqueue (Remind { todo: t.id })) open)
  Task.succeed ()
}

Remind.run : Remind.Params -> Task Unit
Remind.run p = Log.info "a reminder would be sent here"
```

The runtime watches for the mistake. An attempt that held the write
transaction for more than two seconds is marked, in its log line (at
`warn` at least) and on the Jobs page:

```text
job Reports.Rebuild 3f9c01aa done 41250ms attempt 1, held the write lock 41 s: write last, or fan out — book 17
```

`Db.all` gives a `List`, so a job over a million rows loads a million
rows. Fan-out covers that case too: a job reads a page with `Db.limit`
and `Db.offset`, queues the work for it, and queues itself for the next
page.

## Schedules

`every` and `cron` make a job run on its own:

```alder
job Hourly = {} every "1h"

Hourly.run : Hourly.Params -> Task Unit
Hourly.run p = Log.info "an hour passed"
```

A scheduled job takes no parameters (there is no one to fill them), and
has one schedule, not both: either mistake is E0050.

```alder error=E0050 "A scheduled job takes no parameters"
job Digest = { user: User.UserId } every "1d"
```

**`every "<duration>"`** runs at a fixed interval, at least `1s`. When
the interval divides a day (`1h`, `15m`, `1d`), the runs are on its grid
from midnight UTC: `every "1h"` runs on the hour, whichever instance or
restart scheduled it. An interval that does not divide a day (`7m`)
runs that long after the last run.

**`cron "<expression>"`** runs at the minutes the expression matches,
in UTC (every `Time` in Alder is UTC). The five fields are minute
(0–59), hour (0–23), day of month (1–31), month (1–12) and day of week
(0–6, Sunday is 0). A field is `*`, a number, or a range `a-b`, each
with an optional step (`*/15`, `10-40/5`), or a list of those joined by
commas. There are no names (`MON`), no `7` for Sunday, no seconds, and
no `L`, `W` or `#`.

The two day fields follow standard cron's rule. When both are
restricted (neither begins with `*`), a day matches if **either** does:
`0 0 13 * 5` runs on every 13th and on every Friday. When one begins
with `*` (`*`, `*/2`), a day must match both.

An expression that cannot be read is E0050, naming the field at fault
(``day of week `7`: the values are 0 to 6``); with the wrong number of
fields, the message says how many there are instead. An expression that
could never match is refused too, and there is one way to write one: a
day of month with a `*` day of week, none of whose days exists in the
months listed:

```alder error=E0050 "never happens in the months"
job Never = {} cron "0 0 30 2 *"
```

`0 0 31 2 1` is accepted: with both day fields restricted, it runs on
every Monday in February.

### Ticks are rows

A scheduled run is an ordinary job row, a **tick**: its `schedule` is 1
while it is the kind's queued tick, and `scheduled_for` is the time it
stands for. So a tick is claimed, retried, logged and shown like any
job, and several instances agree through the database, not through a
leader:

- A kind has at most one queued tick (the unique index above). When the
  workers start, and at each housekeeping, every scheduled job without
  one gets one, due at its next time.

- Claiming a tick inserts the next one, in the claim's transaction, due
  at the first time after the later of the tick's own time and now. So a
  tick missed while the app was down runs **once**, late, and the next
  one is back on the grid: a nightly job after a three-day outage runs
  once, not three times.

- A tick that fails is retried like any job, and keeps its time. The next
  tick is already queued, so a tick that retries past it can overlap
  with it. If two runs of a job must never overlap, keep its attempts,
  retries included, well inside its interval.

- `alder jobs run Nightly` (below) queues a run by hand, apart from the
  schedule, which it leaves alone.

### Changing a schedule

A changed `every` or `cron` takes effect at the next housekeeping, or
after one more tick of the old schedule. Housekeeping moves the queued
tick only when it is due later than the new schedule's next two times
(`every "1d"` made `every "1h"`); otherwise the old tick runs, and its
claim queues the next tick on the new schedule. A job whose `every` or
`cron` is removed loses its queued tick.

A job renamed or removed leaves its queued tick to no one. Housekeeping
deletes such a tick a day after its time, with a warn line; a tick whose
time has not come is kept, so a rollback to a build without a new weekly
job does not delete that job's tick.

## Silos

In an app with silos (chapter 8, "Silos"), two stores matter for a job,
and they are independent:

- **The row's store** is where it was queued ("Where the job is
  stored", above).

- **The run's store** is the shared store, unless the job is declared
  `silo k`. Then it runs bound to the silo whose key `k` is its parameter
  `k`:

```alder fragment
job Digest = { org: Org.OrgId } silo org
```

`App` must declare `silo org { … }` and a `Maybe` session field `org`,
and the parameter has that field's type without the `Maybe` (E0033
otherwise). The scoped operations narrowed to `org` work in such a
job's `run`, since its `org` is known.

A `Task` writes to one store, and the job's `Task` is bound to its run's
store, so a `silo org` job that writes a shared table is E0031:
"`Digest.run` writes `Org` (shared), but a job with `silo org` runs in
its org's silo: a Task writes to one store. Write `Org` from a job
without `silo org` (this job may enqueue one), or keep the write in the
silo."

When the row's store is the run's, the done mark is in the job's
transaction as usual. When they differ, the mark is a second short
transaction after the job's commit. A crash between the two runs the
job again: the same advice as above, for a window that is rare.

**Where the workers look.** The shared store always. A silo only while
it is in the workers' **active set**: every silo that has jobs when the
workers start (one pass that opens each silo database read-only,
without the migration check, and closes it again; none stays open), and
every silo a job of this process was queued in since. A silo leaves
the set after an hour with no job queued or running there; one with a
job waiting for later (a retry, an `enqueueAt`) stays. A worker's look
does not keep a silo open: a silo with nothing due closes as it would
without jobs, and is opened again only when one of its jobs is due. So
thousands of idle silos cost one pass at start, and nothing after.

## Testing jobs

`alder test` runs no worker. `Job.enqueue` in a test writes its row in
the test's transaction, so it is there for the test to see and gone
with the test. Two steps run and read the queue:

- **`Test.jobs`** lists every job in the test's database, oldest first,
  with its status.

- **`Test.work`** runs every job due at the test's clock, oldest first,
  each as its own `Task` (a failed one has its writes undone, and its
  row records the attempt as a worker's would), and goes on while the
  jobs it ran queued more that are due. It gives each run with its
  `result`: `Ok ()`, or `Err` with the error.

A failure, the wait before the retry, and the death, on a set clock:

```alder
job Flaky = { n: Int } retries 1

Flaky.run : Flaky.Params -> Task Unit
Flaky.run p = Task.fail "the mail server said no"

test "a failed job is retried 10 s later, then dies" = {
  let _ <- Test.setClock (Time.fromMillis 1772582400000)
  let _ <- Job.enqueue (Flaky { n: 1 })
  let first <- Test.work
  let waiting <- Test.jobs
  let _ <- Test.advance 10000
  let second <- Test.work
  let after <- Test.jobs
  let mine = \(rows : List { id: Job.Id, job: Job, status: Job.Status }) ->
    List.filter (\r -> r.job == Flaky { n: 1 }) rows
  Task.succeed (
    List.map (\r -> r.result) (first ++ second) == [Err "the mail server said no", Err "the mail server said no"]
      && List.map (\r -> r.status.runAt) (mine waiting) == [Just (Time.fromMillis 1772582410000)]
      && List.map (\r -> r.status.state) (mine after) == [Dead]
  )
}
```

`mine` picks the test's own job out of the list: the to-do app has a
scheduled job, `Nightly`, and the first `Test.work` of a test queues
its first tick, as a worker does when it starts.

That is how a scheduled job is tested: set the clock a little before
its time, call `Test.work` once (it queues the tick and runs nothing),
move the clock past the time, and call it again. The to-do app's
`Jobs.Nightly` does exactly that, and moves the clock three days more to
show the missed ticks run once.

**Set the clock for a short `every`.** A clock the test never set moves
a second at every reading, and running a job reads it. A job with
`every "1s"` is then always due again, `Test.work` never runs out of
due jobs, and the test fails after 1000 runs with "Test.work: jobs keep
enqueueing jobs". `Test.setClock` stops the clock, and `Test.advance`
moves it.

Two things about traces. The enqueue is a step,
`"Job.enqueue SendWelcome"`, named by the job without its module, so
two jobs of the same name in two modules look alike there. And the
steps a job takes inside `Test.work` are not in the test's trace: test
what the job did by reading the data back, or the job's `result`.

Chapter 10 has the same steps among the other effect tests ("Jobs:
`Test.jobs` and `Test.work`").

## Watching jobs

### The log

Each attempt is one line that names the job and its id, so
`alder logs --grep a1b2c3d4` finds every attempt of one job:

```text
job Signup.SendWelcome a1b2c3d4 done 120ms attempt 1
job Signup.SendWelcome a1b2c3d4 failed attempt 2/5 "smtp: timeout", next in 1m
job Signup.SendWelcome a1b2c3d4 failed attempt 5/5 "smtp: timeout", dead
job Signup.SendWelcome a1b2c3d4 stopped, queued again
```

A done line is `info`, a failure `warn`, a death `error`. In JSON lines
(`ALDER_LOG_FORMAT=json`), the attempt's line and the `Log.*` lines of
its `run` carry the job's id as `rid`, the key a request's lines use.

The metrics (`alder_jobs_total`, `alder_job_duration_seconds`,
`alder_jobs_running`, `alder_jobs_queued`) and the `"jobs"` field of
`/_alder/health` are in chapter 16, "Operating an app".

### The Jobs page

Under `alder run` (development mode only), `/_dev/jobs` lists the
app's jobs: each scheduled job with its next tick, then the queued,
running and dead jobs, and the last fifty done. It reads the shared
database and the silos the process has open or its workers look in. A
silo of the active set that is closed with nothing due is not opened
for the page: it is listed as a line, "closed with nothing due, or
under maintenance; its jobs are read again when one is due".

A job's own page, `/_dev/jobs/<id>`, shows its row, its parameters and
its error, and:

- **Retry**, on a dead job: it is queued again, due now, with its
  attempts at 0 and its error, start, finish, lease and worker cleared.
  A retried tick runs once and queues no next tick (its schedule's next
  tick is a row of its own). A dead job with an `enqueueOnce` key is
  refused while another job holds the key.

- **Run now**, on a queued job: it only makes the job due now and wakes
  this process's workers. Without workers in the process
  (`ALDER_JOBS=0`), nothing runs it here.

- **The last attempt** this process ran of it, with its trace and
  statements, marked when it held the write lock. The record is kept in
  memory for the last hundred jobs run, so it is gone after a restart.

The parameters are masked by name: a top-level field whose name says it
holds a secret (`password`, `token`, …) shows as `••••`. A secret under
another name shows in clear, so do not pass secrets to a job; pass the
id of the row that holds one. `/_dev/jobs.json` is the same listing as
JSON, for tools.

### `alder jobs`

`alder jobs` reads the dev database (`build/<app>/dev.db`, or the file
`--db` names) and the silo databases beside it (`silos/<key>/*.db`),
and prints the number of jobs in each state, each scheduled job's next
tick, and the dead jobs with their errors:

- `alder jobs retry <id>` retries a dead job, as the page's Retry does;
- `alder jobs retry --dead` retries every dead job;
- `alder jobs run <Kind>` queues a run of a job without parameters, due
  now, apart from its schedule. It reads the app to find the job, so the
  app must check.

It changes the file directly, and a running app's workers see the
change at their next look. In a silo, that is only while the silo is in
their active set (a job queued or running there in the last hour), or
after they restart. A database on Turso is not read: `--db` takes a
file.

## Deploying jobs

Nothing about a deploy changes for jobs by default: the app's process
runs them, on every target.

### One box: `Ssh` and `Fly`

An `Ssh` environment is one box with one database file, and the app
process runs the jobs beside the requests. `ALDER_JOBS` is the knob: a
CPU-heavy job competes with the requests, and fewer workers make it
compete less.

During a deploy, the new build starts beside the old one, and both run
workers on the same file for the drain. A claim is atomic, so they never
run the same job, and a job of a kind only one build knows is left to
that one. When the old build is stopped, its running jobs are queued
again, and the new build takes them.

A `Fly` environment is one machine, replaced in place. The old process
stops as above (a process killed outright leaves its jobs to their
leases), the machine restarts, and the new one claims what was queued.
A tick that fell in the restart runs late by those seconds.

### `Render`, and worker machines

A `Render` environment keeps its data in a shared Turso database, so
several processes can run jobs from one queue:

- With `instances: N`, each instance runs its workers. The claim is
  atomic over Turso too (the same statement, in a `BEGIN IMMEDIATE`
  transaction on the Turso connection), and a tick is a row with a
  unique index, so one instance wins each tick and no leader is
  needed.

- With **`workers: N`**, jobs leave the web service. `alder deploy`
  makes a second Render service, `<service>-worker`, that runs the same
  image as `/app/app --jobs-only` on N instances, and sets
  `ALDER_JOBS=0` on the web service, which then runs none.

`app --jobs-only` opens the database, wires the capabilities and starts
the workers, with no listener: no address, no `/_alder/health` (the
process being up is its health). `ALDER_JOBS` is 8 by default in this
mode. On Turso, a worker built for an older schema than the database's
does not run stale code: it runs nothing and looks at the schema again
every 10 seconds until a deploy brings its build in line; one built for
a newer schema exits 1, as the web build does.

One detail makes the split work with silos. The web service runs no
workers, so it has no active set, and a silo it queued a job in would
otherwise wait for the worker service's next start. So a process with
no workers that queues a job in a silo also records the silo in a small
table of the shared store, `_alder_jobs_silos`, before its transaction
commits. The `--jobs-only` workers read and clear that table once a
second, so with a worker free the job starts within about a second.

The `workers` field is chapter 16's ("The Render target"): how the
deploy waits for the worker service, `alder logs <env> --worker`, and
what happens when the field is removed.

### How far it scales

The queue scales as far as the database does.

- **One machine with a file** (`Ssh`, `Fly`): the app process runs the
  jobs. One worker claims and finishes about 2,800 jobs a second, for a
  job that inserts one row, measured on a laptop (an Apple M5 Pro) with
  a load near 2. Four workers do no better (about 2,790) and eight a
  little worse (about 2,470): SQLite has one writer, so more workers
  queue on the lock. With request writers busy on the same database,
  four workers drain about 1,600 a second. The requests' p99 did not
  rise, but they got about half the lock: with four request writers,
  45,750 writes a second alone and 21,930 with the jobs; and with
  sixteen, their p99.9 rose from 33.4 ms to 80.5 ms. The default is
  four workers, not one, for jobs that spend their time outside the
  database (a mail, an HTTP call), which the lock does not limit.

- **A shared store** (`Render` with Turso): `workers: N` worker
  instances, scaled apart from the web ones and failing apart from
  them (a worker that runs out of memory restarts without a request
  noticing), while the web instances run no jobs.

Queuing costs a request one more statement: 0.16 ms on a 10 ms request
in the same measurement. The full figures and how they were taken are
in `docs/perf/2026-11-jobs-throughput.md`.

### Rolling back

Two rollbacks fail because of jobs, and both say so instead of breaking
quietly:

- **Across the alder upgrade that brought jobs.** The jobs table is a
  table the older runtime does not know. An app built by an older alder,
  with migrations, counts every table it does not know as a change made
  outside migrations, so it refuses the database at start (on Turso it
  refuses it with no migrations too; on a file, an app with no
  migrations does not notice). The rollback's health check fails and
  nothing is switched (chapter 16, "Status, rollback and logs").

- **To a build older than `--jobs-only`, with `workers`.** The worker
  service runs the image with `--jobs-only`, which that build does not
  know: it prints its usage and exits, so the worker service's deploy
  fails, and with it the rollback.

## What is not there yet

- `Job.cancel`, and a priority or a queue per job.
- `alder jobs --env <name>`, against a deployed environment.
- Worker machines on `Fly` (they need a shared database there first).
- A streaming read for a job over a very large table; fan out instead.
