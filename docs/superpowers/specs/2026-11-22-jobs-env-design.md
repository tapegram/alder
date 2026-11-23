# `alder jobs --env <name>`: the jobs of a deployed environment

Design for #285 (follow-up of #168, PR 7/7, 98b36736). The user was not available for this
brainstorm, so every choice below is the author's; the ones most worth a veto are at the top,
each with a recommendation. Builds on the jobs design (`2026-11-15-background-jobs-design.md`,
§7, §7a, §9), the silo hints (`2026-11-21-jobs-silo-hints.md`), the deploy targets (book 16) and
the silo commands (`cmd/alder/silos.go`, `internal/deploy/ssh_silos.go`).

## Open questions for the user

1. **Render: open Turso from this machine, or run the build's binary here?** The CLI opening the
   shared database and the silo children directly over Turso's URL and a minted token (as every
   Render command already reads the builds table, `internal/deploy/render.go:328-345`), versus
   running the current build's executable on this machine with the Turso environment (as
   `Turso.Migrate` does, `internal/deploy/turso.go:311-352`). **Recommendation: open Turso
   directly.** The local-binary path needs the current build's artifact on this machine, which a
   fresh checkout or CI does not have, and `localBinary` then builds the *working tree*
   (`cmd/alder/deploy.go:616-640`), which is not the deployed build. The one thing lost is the
   deployed build's own check of `run <Kind>` (question 4).
2. **Writes confirm on the terminal, with `--yes` to skip?** `retry`, `retry --dead` and `run`
   against an environment ask "type yes", as `alder restore`, `alder silos prune` and `alder
   rollback --restore-backup` do (`cmd/alder/deploy.go:1348-1359`), versus a flag only
   (`--env` being the operator's intent already). **Recommendation: confirm + `--yes`**, the
   convention of every command that changes a target; listing stays read-only and silent.
3. **Writes under the deploy lock?** `retry` and `run` take the target's `Locker` (as `restore`
   and the silo commands do, `cmd/alder/deploy.go:1240-1246`, `cmd/alder/silos.go:45-49`), so
   they are refused while a deploy runs, naming the holder; versus no lock (a row update does
   not conflict with a deploy). **Recommendation: take it** — one `if l, ok := t.(deploy.Locker)`
   block, and two operators' `retry --dead` do not interleave their summaries.
4. **`run <Kind>` on Render checks the kind against the local app, not the deployed build.** On
   Ssh and Fly the build's binary resolves the kind (its `app.Jobs()`); on Render the CLI has
   only the working tree (today's `jobsRun` check, `cmd/alder/jobs.go:419-456`). A kind the
   deployed build does not know sits queued until a build that knows it serves (workers leave
   other kinds' rows, `rt/jobs/worker.go:42`). **Recommendation: allow it**, with the line
   `queued for build <id>; a kind that build does not declare waits for one that does` and the
   confirmation naming the build; the alternative is refusing `run` on Render in v1.
5. **A silo retry writes a silo hint.** `alder jobs retry` of a row in a silo also upserts the
   silo into `_alder_jobs_silos` (the enqueuer's rule for a process without workers,
   `rt/jobs/enqueue.go:67-80`), so a `--jobs-only` worker (`workers:` on Render) finds it within
   a poll instead of at its next start. Harmless elsewhere: nothing reads the table in a
   single-process deploy, and it holds at most one row per silo. **Recommendation: include**
   (a few lines; it fixes the deploy shape this feature is for). The local `alder jobs` does the
   same, so the dev split (`--jobs-only` beside `alder run`) benefits too.
6. **`--env` as a flag, not a positional.** `alder logs <env>` takes the environment first; `alder
   jobs` has `retry <id>`, `run <Kind>` and `[dir]` in its positional slots, and the issue is
   titled with `--env`. **Recommendation: `--env <name>`** (with `--host name` for previews, as
   every deploy command); `--env` and `--db` do not combine.
7. **Fly is tested through the engine's fake only.** `internal/deploy/fly_test.go`'s fake Machines
   API runs `exec` scripts locally with a shell-script "app" (`:134-152`), so the adapter's path
   is covered there; the command end to end on Fly runs only in `TestDeployFly_Real` (gated), as
   `alder logs` does. **Recommendation: accept.**
8. **No `--silo key:id` narrowing in v1.** On Turso the listing opens every child database once
   (one `Open` and two queries each), the cost `--silo-status` already pays on every `alder
   status`. **Recommendation: not in v1**; file it if a tenant count makes the listing slow.

## Intent

`alder jobs` reads a database file and refuses Turso (`cmd/alder/jobs.go:114-116`): once an app
is deployed, the only ways to see a dead job are `alder logs --grep` and a `sqlite3` session at
the box, and the only way to retry one is a redeploy or a hand-written `UPDATE`. After this
change the same four operations — list, `retry <id>`, `retry --dead`, `run <Kind>` — work against
any environment of the `deploy` declaration, with the same lines, over the paths the deploy
adapters already use to reach an environment's state: the current build's executable at the box
(Ssh, Fly), and the shared Turso database and its silo children (Render).

```text
$ alder jobs --env production
jobs at Ssh deploy@box.example.com:/srv/app, build 20261122-1403-a1b2c3, and 2 silo databases:
  queued   3
  running  1
  dead     2
  done     418
next schedule ticks:
  Reports.Nightly  2026-11-23 03:00 UTC
dead jobs, the last finished first:
  dead0001  Signup.SendWelcome  5 attempts  smtp: timeout …
  dead0003  Reports.Digest      1 attempt   x (in silo org/42)
`alder jobs retry <id> --env production` queues one again; `alder jobs retry --dead --env production` queues them all again
$ alder jobs retry dead0003 --env production
queue job dead0003 again at Ssh deploy@box.example.com:/srv/app, due now? type yes: yes
queued job dead0003 (Reports.Digest) again in silo org/42, due now: attempts 0, its error cleared
  in silo org/42, a running build's workers find it while the silo is in their active set (a job queued or running there in the last hour) or after they restart; a `--jobs-only` worker reads the silo hint within a poll
```

Success: the transcript above holds on an Ssh box with a file and with `database: Turso`, on Fly,
and on Render with and without `workers:`; the operations are `rt/jobs/admin.go`'s, unchanged;
nothing new is installed at any target and no new credential is needed on this machine.

## Goals

- The four operations of `alder jobs` against every target that holds state: Ssh (file and
  Turso), Fly, Render (Turso; `workers:` or not); the same output as the local command, the
  environment named where the file was.
- Silos covered as locally: every silo database of the environment is read, a silo's dead row
  is retried where it is, and the output says the silo.
- Read-only by default; a write confirms, takes the deploy lock, and prints what it changed.
- No new credential: an Ssh environment uses its ssh and the box's environment file; Fly its
  token; Render the `RENDER_API_KEY` and `TURSO_API_TOKEN` every Render command needs.
- A build made before this alder is told apart and named, never misread.
- Fakes for every adapter, as the deploy tests have them.

## Non-goals

- Params, traces or the done rows: the local command shows none, and `alder logs --grep <id>`
  has the attempts. A `--json` for `alder jobs` (local or remote) is not in v1.
- An HTTP admin surface on the app (see §2's alternative C).
- Narrowing to one silo (`--silo`), cancel, priorities, a Jobs screen in alder-tui.
- Worker machines on Fly (the jobs spec's follow-up); the Script target (`ScriptUnavailable`,
  as every command).

## Constraints

- The deploy commands read this machine's environment once into `deployEnv`
  (`cmd/alder/deploy.go:50-101`) and nothing else; `envTarget` builds the target
  (`:666-690`); tests run in parallel with their own `deployEnv` (`deployCmds`,
  `cmd/alder/deploy_test.go:37-70`).
- At a box the only way to the data is the current build's executable with a maintenance flag,
  run by `builds/<id>/run` so it gets `ALDER_DB`, the environment file and `turso.env`
  (`Ssh.current`, `internal/deploy/ssh_silos.go:29-51`; the run script,
  `internal/deploy/ssh.go:622-650`). The image at Render has no shell and Render has no exec
  API; the image at Fly has the binary at `/app/app` and the Machines API's `exec`
  (`internal/deploy/fly.go:413-428`), 60 s a call.
- The binary's maintenance tasks print JSON lines for the CLI to format (`--silo-status`,
  `rt/db/stores.go:1524-1560`; parsed at `ssh_silos.go:66-77`); a task the build does not know
  prints its usage and exits 2 (`maintain`, `internal/gen/caps.go:1084-1117`).
- `rt/jobs/admin.go`'s operations take a `db.Execer` and now: the dev panel, `alder jobs` and
  this feature share them. The jobs schema only ever gains columns (`jobs_schema` in
  `_alder_meta`, `rt/db/migrate.go:864`).

## Design

### 1. The command

```text
alder jobs [retry <id> | retry --dead | run <Kind>] [dir] --env <name> [--host name] [--yes] [--build-dir DIR]
```

- `dir` is the app's directory, read for its `deploy` declaration as every deploy command reads
  it (`envTarget("jobs", env, dir, host, e)`, with `e.BuildDir` from `--build-dir`); `--host`
  names a preview (`targetEnv`). Without `--env` the command is exactly today's.
- `--env` with `--db` is a usage error (`--db takes a file; --env an environment`); `--yes`
  without `retry` or `run` is one too; a Script environment is `ScriptUnavailable("jobs", env)`.
- Exit codes as today: 0; 1 for a refused operation (an unknown id, a job not dead, a key held,
  a build older than this feature's), the deploy lock held, a target that cannot be reached;
  2 for usage, a declaration or environment that is missing, a store that cannot be read, a
  write not confirmed, and `run` of a kind the build lacks or one with parameters.
- `cmd/alder/jobs.go` splits each operation into a *gather* over a source and a *print* over
  what it gathered: `jobsList` becomes `printJobs(report)` over a `jobsReport{counts, ticks,
  dead []placed, missed []string}` (`missed` as `jobs.Listing.Missed`, `rt/jobs/admin.go:62-66`:
  a store that could not be read is printed after the table, `silo org/42: not read: …`, exit 2
  — the local command gains this instead of stopping at the first failure); `jobsRetry` prints
  a `placed` row; `jobsRetryDead` prints retried rows and refusals; `jobsRun` prints an id. A
  `jobsStore` gains `open func(ctx, write bool) (*db.SQLite, error)` in place of `path`, so the
  local files and the Turso stores share `withJobs` (`:166-189`). Two sources:
  - **stores**: today's code over `[]jobsStore` (local files, and Render's Turso stores, §3);
  - **binary**: the JSON lines of the build's `--jobs` task (§4), decoded into the same
    `placed` rows, over a `deploy.JobsRunner`.

### 2. Where the operations run

Three ways to reach a deployed environment's jobs table:

- **A. A maintenance task of the app's binary at the target**, run as `--silo-status` is: Ssh
  `current(ctx, "--jobs", …)`, Fly `exec` of `/app/app --jobs …`. The build that serves reads
  its own database with its own runtime: its kinds, its schema, its token at the box. No
  credential leaves the target. Render has no exec path; the binary could instead run on this
  machine with the Turso environment, as `Turso.Migrate` runs `--verify-silos` and
  `--migrate-only` (`internal/deploy/turso.go:286-352`), which needs the build's artifact here.
- **B. The CLI opens the database directly.** Possible only where the store is Turso: the
  shared database by its name (from the service's `ALDER_TURSO_DB` or the box's `turso.env`)
  and the children by `Turso.Children` (`rt/db/turso.go:220-240`), each opened with a token
  the API token mints (`Turso.Open`, `:357-376`). A file at a box is out of reach: no `sqlite3`
  is promised there and the one write path alder has to a box's file is `restore`, which ships
  a whole file.
- **C. An authenticated HTTP endpoint on the app** (`/_alder/jobs`, a bearer token), reached at
  the public address: one path for every target. Rejected for v1: it is an admin surface on the
  internet with no credential story (`ALDER_SECRET` is the cookie key and must not travel;
  `alder secret` #237 does not exist), `--jobs-only` has no listener, a stalled app is when the
  operator most needs the command, and the Jobs page is dev-only on purpose
  (`rt/web/devpanel.go:438`).

**Decision: A where there is a box or a machine (Ssh with a file or Turso, Fly), B where there
is none (Render).** A is the precedent for every silo command at a box and makes the deployed
build authoritative for `run <Kind>`; B is the precedent for every Render command, which already
reads `_alder_deploy_builds` and `_alder_deploy_current` over the same `store()` and holds the
same tokens (`Render.Connect`, `render.go:203-264`). A on Render through a local binary was
rejected for question 1's reasons. B on Ssh+Turso would work too, but the box path is how
`status`, `migrate silos`, `silos prune` and `restore --silo` reach the silos there
(`ssh_silos.go`), the token is at the box, and the deployed build checks the kind.

### 3. Per target

- **Ssh, file database.** `deploy.JobsRunner` on `*Ssh`: `Jobs(ctx, args ...string) (out
  string, code int, err error)` is `s.current(ctx, append([]string{"--jobs"}, args...)...)`: the
  run script sources `<path>/env`, exports `ALDER_DB=<path>/db/app.db`, execs `builds/<id>/app
  --jobs …`. `ErrNotDeployed` → `environment production is not deployed yet: no build serves
  there` (exit 1). The output has stderr merged (`2>&1`): lines starting `{` are the report,
  the rest is the binary's message, printed as the CLI's own.
- **Ssh, `database: Turso`.** The same call: the run script sources `turso.env`
  (`ALDER_DB=libsql://…`, `ALDER_TURSO_DB`) and the environment file's `TURSO_API_TOKEN`
  (`ssh.go:628-633`), so the task opens the shared database as `--migrate-only` does and the
  children through `StoresConfig{Dir: libsql://…}` (`rt/db/stores_turso.go:104-137`).
- **Fly.** `JobsRunner` on `*Fly`: `f.read(ctx, script)` with `ALDER_DB="$R/db/app.db"
  ALDER_STATIC=/app/static exec /app/app --jobs "$@"` (the arguments quoted with `q`, as
  `current` does), through a new `execCode` beside `exec` (`fly.go:413-428`) that returns the
  `execResult`'s stdout, stderr and exit code rather than wrapping a non-zero exit in an error
  (1 and 2 are answers here). `read` returns `""` for no machine or one that does not start →
  not deployed (exit 1). The binary at `/app/app` is the image's, so the build that serves. The
  60 s limit is far above a file listing; Fly has no silos (`targetFor`, `deploy.go:387-391`).
- **Render.** `deploy.JobStorer` on `*Render`: `JobStores(ctx) ([]JobStore, error)`, after
  `Connect` (the service, its `ALDER_TURSO_DB`, `SetShared`); no service, or no database at
  Turso (`DB.sharedDB`), → not deployed. The stores: the shared one (`r.DB.api().Open(ctx,
  name, reader)`, the token minted as `store()` mints it) and, for each of `r.DB.Silos` (set by
  `targetFor`, `deploy.go:402-404`), each of `Children(ctx, App, key)` whose name decodes
  (`ChildValue`), opened the same way — `JobStore{Key, Value string; Open func(ctx, reader
  bool) (*db.SQLite, error)}`, the CLI wrapping each as a `jobsStore`. `Current` gives the
  header's build. With `instances: N` any instance claims a retried row; with `workers:` the
  `--jobs-only` service does, and a silo retry's hint (§5) reaches it within a poll. Nothing
  is read from the service's environment but the database's name, as today.
- **Script.** `ScriptUnavailable`, exit 2.
- **The header.** `jobs at <t.Describe()>, build <current>, and N silo databases:` — `Ssh
  deploy@box:/srv/app`, `Fly ord (app blog)`, `Render blog (oregon)`; the closing hint line
  repeats `--env <name>` (and `--host`).

### 4. The binary's task: `--jobs`

- **Grammar**, in the generated `maintain` (`caps.go:1084-1117`; the usage line gains `--jobs
  list | --jobs retry <id> | --jobs retry --dead | --jobs run <Kind>`): one call,
  `jobs.Admin(ctx, cfg, app.Jobs(), args, os.Stdout, os.Stderr) int`, in `rt/jobs/admin.go`, so
  the logic is Go-tested without a build; the generator adds six lines, with silos or without
  (`cfg` is the `db.StoresConfig` `siloCases` builds, `caps.go:1130`, or one with only `Shared`
  for an app without `silo`). The shared store opens as `--migrate-only` opens it (`db.OpenSQLite`
  on a file or `libsql://`), read-only for `list` (`OpenSQLiteReader`, or Turso's reader);
  no migration check, no `ALDER_SECRET` check (`--silo-status` has neither).
- **The silos**: `db.SiloDBs(ctx, cfg) ([]SiloDB, error)` exported from `entries`/`openEntry`
  (`stores_turso.go:104-137`): `SiloDB{Key, Value string; Open func(ctx, reader bool)
  (*db.SQLite, error)}`, the files under `Dir/silos/<key>/` or the Turso children. (Render's
  `JobStore` is the same shape; `JobStores` is `SiloDBs` over the API client plus the shared
  store.)
- **Output**, JSON lines on stdout, every message on stderr, `Row` given json tags (`In` as
  `json:"-"`, pointers as null):
  - `list`: one line per store, `{"silo":"org/42","counts":{"queued":1,"dead":2},
    "ticks":[Row…],"dead":[Row…]}` (`""` for the shared store; `ticks` the queued rows with
    `Schedule`; a store without a jobs table has zero counts), and `{"silo":"…","error":"…"}` for
    one that cannot be read, the rest still listed; exit 0, or 2 when a store was missed.
  - `retry <id>`: `{"silo":"…","row":Row}` (the row as it was) exit 0; `{"refused":"<the
    StateError, KeyHeldError or ErrNoJob text>"}` exit 1; a failure is stderr and exit 2.
  - `retry --dead`: a `{"silo","row"}` line per retried row, then `{"refused":[…]}`; exit 1 when
    one was refused; a store failure part way prints the lines so far, the error, exit 2 (each
    retry commits on its own, as locally, `jobs.go:359-362`).
  - `run <Kind>`: `{"id":"…","kind":"Reports.Nightly"}`; the kind resolved against
    `app.Jobs()` by `jobs.ResolveKind(kinds, name)` (the name or the qualified name; several
    matches, none, or parameters are the refusals `jobsRun` prints today, `jobs.go:445-456`,
    on stderr, exit 2). `jobs.Kind` gains `Params []string` (`rt/jobs/run.go:17-27`; generated
    from the declaration) for the parameters check; the queued row is `jobs.Manual`'s in the
    shared store after `EnsureTable`.
- **The CLI's decoder** (the binary source of §1) reads the `{` lines into `placed` rows and
  `jobsReport`, the other lines into the message it prints; the exit code is the task's, except
  §7's old build.

### 5. Silos

- **Which are read**: at a box with a file, every file under `db/silos/<key>/` of the app's keys
  (`SiloFiles`); on Turso, every child of each key; the active set and the panel's "closed with
  nothing due" skip do not apply — a maintenance process opens each once, reads, closes (the
  start scan's cost, `rt/jobs/active.go:249-276`; `--silo-status`'s). An orphaned silo (its key
  row gone) is read like any other: its rows are dead to the workers, and a retry there is
  refused with `silo org/42 is orphaned: nothing runs there (alder silos prune)`, checked by
  `StoresConfig.Shared`'s `orphans` where the task runs (`stores_turso.go:146`); Render's path
  does the same from the shared store.
- **Where a retry lands**: in the store that has the row, as locally (`jobsRetry` walks the
  stores). A silo's run store is the job's declaration's, untouched.
- **The hint** (question 5): after a retry of a row in silo `key/value`, `jobs.Hint(ctx, shared,
  key, value, now)` — the upsert of `rt/jobs/enqueue.go:67-80` exported — on the shared store:
  in the binary task (it has no active set, so the enqueuer's rule applies), in Render's direct
  path, and locally. The note after a silo retry gains "a `--jobs-only` worker reads the silo
  hint within a poll". `run <Kind>` lands in the shared store and needs no hint.
- **A silo under maintenance** (behind by a data-loss migration): the task opens it as a reader
  without the migration check, so its dead rows are listed and retried; the note says the
  workers do not look in a silo under maintenance (`rt/jobs/active.go` `at`, a
  `*MaintenanceError` idles it) until `alder migrate silos` serves it again.

### 6. Credentials and safety

- **Nothing new on this machine.** Ssh: the ssh key and `ALDER_SSH_CONFIG`; the token for Turso
  is the box's environment file, read by the run script, never by the CLI. Fly: `FLY_API_TOKEN`
  (the Machines API; the token is not sent to the machine). Render: `RENDER_API_KEY` (the
  service and its database's name) and `TURSO_API_TOKEN` (minting a database token), both
  already required by `Connect`. No value of any environment is printed; params are never
  printed (the local command prints none); an error's first line is, as locally and in the log.
- **Read-only by default.** `list` opens every store as a reader (`OpenSQLiteReader`, the Turso
  reader) and takes no lock, as `alder status` takes none.
- **Writes confirm** (question 2) before anything is opened for writing: `queue job <id> again
  at <Describe()>, due now? type yes`; `queue the N dead jobs at <Describe()> again, due now?
  type yes` (after a read listing them, so N is known); `queue a run of <Kind> at
  <Describe()>, build <id>, due now? type yes`. `--yes` answers; without a terminal and without
  `--yes` the command exits 2 with `not confirmed: nothing was queued (pass --yes to confirm
  without a terminal)` — `confirm` is false on a pipe (`deploy.go:1348-1359`), so CI must pass
  `--yes`.
- **Writes take the lock** (question 3): `Connect`, then `Lock` where the target is a `Locker`
  (Ssh's lock file, Render's `_alder_lock` row — only after `sharedDB` found the database, so
  the lock never creates one); a deploy, rollback or restore in progress refuses with the holder
  named, exit 1. Released at exit; a `SIGINT` releases it (`signal.NotifyContext`, as the silo
  commands).
- **Two builds at once** (an Ssh drain): the task runs the *current* build; a retried row is
  claimed by whichever build knows the kind. A tick retried runs once (`Retry`'s rule). Nothing
  here restarts, stops or migrates anything.

### 7. An app built before this

- **At a box or a machine**: the build's `maintain` does not know `--jobs`, prints `usage: app
  [--jobs-only | --migrate-only | …]` and exits 2 (`caps.go:1115-1116`; `--jobs-only`'s
  precedent on a rollback, jobs spec §7a). The CLI recognises exit 2 with a first line starting
  `usage:` and says `build <id> at <env> was made by an alder without \`alder jobs --env\`
  (before <version>): \`alder deploy <env>\` with this alder, then try again` — exit 1, never
  the usage text itself. A build older still (before background jobs) answers the same way.
- **On Render** the direct path needs no support from the build: a database without
  `_alder_jobs` lists zero counts and `no jobs table at Render blog (oregon): the app declares
  no job, or build <id> was made before background jobs`; a `jobs_schema` in `_alder_meta`
  newer than this alder's refuses `the jobs table at <target> is schema N, newer than this
  alder's M: upgrade alder` (the schema only gains columns, so an older table is read fine; a
  newer one may hold a column this alder's `rowCols` does not select, which would still read,
  but `Retry`'s UPDATE could leave a newer invariant unset — refuse rather than guess). `run`
  on an old build queues a row that build may not know (question 4).
- **This alder older than the build**: the binary's JSON may carry fields the CLI does not
  know; they are ignored (`encoding/json`'s default), so a newer build's listing still prints.

### 8. Output parity

- The local and remote printers are one function over `jobsReport`/`placed`: the counts table,
  `next schedule ticks:`, the dead rows with `(in silo org/42)` and `(the schedule tick of …)`,
  `no dead job`, the closing hint — identical, with the header's `jobs in <file>` replaced by
  `jobs at <target>, build <id>` and the hint's commands carrying `--env`. `retriedNotes` are
  the same words ("a running build's workers" for "a running app's workers"), plus the hint
  line of §5. `retry --dead`'s summary and partial-failure order are unchanged.
- The local command gains the `missed` lines (§1) and the hint write (§5); its tests are
  updated for those two and nothing else (`cmd/alder/jobs_test.go`).
- Times are UTC minutes as today (`utcMinute`); a box's clock is not consulted (the rows carry
  epoch milliseconds).

### 9. Testing

- **rt/jobs** (`admin_test.go`): `Admin` over temp files — list's lines and counts, a store
  without a table, a silo file that cannot be read (a directory in its place) listed as `error`
  and exit 2; `retry` 0/1/2 and the refused texts; `retry --dead` with a refusal and a failure
  part way; `run` with `Params` (refusals' texts), `ResolveKind`'s three refusals; the hint
  row after a silo retry, none after a shared one; the JSON round trip against the CLI's
  decoder (a shared test helper in `rt/jobs/export_test.go`). A Turso case over
  `internal/tursofake` as `rt/db`'s Turso tests do: the shared database and two children.
- **internal/gen**: the `maintain` goldens (with silos and without) regenerated and read.
- **internal/deploy**: `TestFly_Jobs` with `fakeFly` (`fly_test.go:48`): the fake app script
  answers `--jobs list` with canned lines and `--jobs retry x` with exit 1 and a refusal, and
  its default branch prints usage (the old build); `JobsRunner`'s exit codes and outputs;
  the machine stopped → started first (`running`). `TestRender_JobStores` with `renderfake`
  and `tursofake`: the shared store and the children of two keys, a service without a
  database → not deployed.
- **cmd/alder** (`jobs_env_test.go`):
  - `TestJobs_EnvSsh` (the loopback box, `startBox`, `-short` skips; `deploy_silos_test.go:127`'s
    shape): deploy the jobs corpus app with a silo, seed rows with `sqlExec` into
    `<root>/db/app.db` and a silo file, `alder jobs --env production` compared line for line
    with the local printer's output for the same rows; `retry <id> --env production --yes` in
    the silo, the row's state and the hint row checked at the box; `retry --dead`; `run
    Nightly`; the lock held by a running deploy → the refusal; `run` of a kind with parameters
    → the build's text; a build id in the header.
  - `TestJobs_EnvSshTurso` (`deploy_silos_test.go:290`'s shape, `tursofake`): the same over
    `database: Turso`, rows seeded with `f.Exec`, children created by the app.
  - `TestJobs_EnvRender` (`renderApp`, `deploy_render_test.go:45`; `TestLogs_Worker`'s shape):
    seed `_alder_jobs` in the fake's shared database and in `blog-org-<name>` children; list,
    retry with `--yes` (the hint row asserted through `f.turso.DB`), `run` with the "queued for
    build" line; no service → not deployed; `TURSO_API_TOKEN` unset → `Connect`'s refusal.
  - Unit: `--env` with `--db`, `--yes` alone, a Script environment, the old-build detection on
    `("usage: …", 2)`, the no-terminal refusal (stdin a pipe) exit 2 with nothing written.
- **Real** (`-tags deploy`, by hand): `TestDeployFly_Real` and `TestRenderReal` gain a listing
  after their deploy.

### 10. The book and the docs

- **Chapter 17**, "`alder jobs`" (`book/17-jobs.md:731-748`): a second paragraph — `--env
  <name>` against an environment, what runs where (the build at the box or machine; the shared
  database and its children on Render), the confirmation and `--yes`, the hint for `workers:`,
  and the old-build sentence; "What is not there yet" (`:853-861`) loses its line.
- **Chapter 16**: "Silos at a box" (`:2344-2391`) lists a fifth command running the current
  build's executable, `alder jobs --env`; "The Render target"'s command list (`:2061-2103`)
  gains it with "reads the shared database and the silo databases from this machine"; "The Fly
  target" (`:1691`) one sentence; "Operating an app" → "Job workers" (`:1067`) a pointer to
  chapter 17's section.
- `jobsUsage`, `jobsHelpNote` (`cmd/alder/jobs.go:28-41`): the `--env` line and "A database on
  Turso is read through its environment: `--env`"; the generated main's package comment
  (`caps.go:820-837`) for `--jobs`; CHANGELOG's jobs entry (`CHANGELOG.md:16`) gains the
  command; the dev log rule does not apply (no Alder-language observation).

## PR breakdown

Each green on its own, in order:

1. **rt/jobs `Admin` and the binary's `--jobs`**: `jobs.Admin`, `ResolveKind`, `Kind.Params`,
   `Row`'s json tags, `jobs.Hint`, `db.SiloDBs`; the generator's case and goldens; the Go tests
   of §9's first two items. No CLI change; `alder jobs` locally unchanged.
2. **`--env` over the binary (Ssh, Fly)**: `deploy.JobsRunner` on `Ssh` (`current`) and `Fly`
   (`execCode`); `cmd/alder/jobs.go`'s split into sources and printer, `jobsStore.open`, the
   `missed` lines and the local hint write; `--env`/`--host`/`--yes`, the confirmation, the
   lock, the old-build sentence; the box tests, the Fly engine test, the unit tests; help text.
3. **`--env` on Render**: `deploy.JobStorer`, `Render.JobStores` over Turso; the stores source
   over them; the `jobs_schema` check; the renderfake/tursofake tests.
4. **Book, CHANGELOG, close #285**: chapters 16 and 17, the CHANGELOG line, the real tests'
   listing step.
