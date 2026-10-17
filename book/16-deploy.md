# 16. Deploying

An app is finished when it runs somewhere other people can reach. In
most languages that last step is a separate world: a CI service with its
own configuration language, a container file, a script that copies
files and restarts a server, and a page of notes about which step to run
by hand when a database migration is involved. Each piece is written
once, half understood, and feared afterwards.

In Alder that step is part of the language. The app says where it ships,
in a declaration in `App`, and one command does the rest:

```text
$ alder deploy production
```

That command runs the same gate every time (the checker, the build, the
tests, the mutation score), makes one self-contained directory holding
the build, asks the server what its database looks like, refuses
anything that would lose data, copies the build over, backs the database
up, applies the migrations, starts the new build beside the old one,
checks that the new build really answers, and only then sends the
traffic to it. A build that fails any step leaves the old one serving.

This chapter is the whole story: the declaration, the command step by
step (with a transcript `go test ./book` runs, and a recorded session
against a real server), what the gate refuses and why, `status`,
`rollback` and `logs`, backups and `restore`, the workflow `alder ci
init` writes for GitHub, the preview environment of a pull request, the
`Fly` target, the `Script` escape hatch, and what running SQLite in
production means. Chapter 12 explains why it is designed this way;
chapter 13 how the code does it.

## The declaration

An app says where it ships with one `deploy` declaration in `App`:

```alder fragment
deploy {
  production: Ssh "deploy@box.example.com" { domain: "blog.example.com", path: "/srv/blog" },
  staging:    Fly "ord" { app: "blog-staging" },
  preview:    Ssh "deploy@box.example.com" { domain: "*.preview.example.com", path: "/srv/blog-preview" },
  floor:      0.95,
  checks:     ["./scripts/lint-content"],
  requires:   ["SMTP_URL", "STRIPE_KEY"],
}
```

(The block is marked as a fragment because it is a piece of `App`, not a
module, but `go test ./book` still checks it: `TestBook_DeployDeclaration`
appends every `deploy { … }` fragment of this chapter to the scaffold's
`App` and runs `alder check` on the result, which must be clean.)

Each field whose value is a **target** is an environment: here
`production`, `staging` and `preview`. The names are yours, but two of
them mean something to `alder ci init` (below): `production` is what a
push to `main` deploys, and `preview` is what a pull request deploys.
The other fields are settings of every environment.

| Field | What it says |
|---|---|
| `<env>: Ssh "user@host" { domain, path, user?, port? }` | a Linux server reached with your SSH key; the app's state lives under `path`; `domain` is the name the proxy serves with automatic TLS; `user` and `port` override the SSH user and port (an `Int`) |
| `<env>: Fly "region" { app }` | a Fly.io app in a region: one machine and a volume for the database (see "The Fly target") |
| `<env>: Script "./path"` | your own script, handed the finished build (see "The Script hatch") |
| `floor: 0.95` | the least mutation score a build may have (`0` to `1`); without it, the score of the build deployed last, `0` before any |
| `checks: ["cmd", …]` | shell commands run from the app's root after the tests; each must exit 0 |
| `requires: ["NAME", …]` | the environment variables the target must set (the adapters' secrets); names only |
| `backup: S3 "s3://bucket/prefix"` | stream the database to a bucket while the app runs (see "Backups and restore") |
| `keep: 5` | how many builds a target keeps (default 5) |
| `proxy: None` | an `Ssh` target serves the app itself on a port instead of behind Caddy (default `Caddy`) |

The declaration is **data**. It adds no route, no page, no code to the
running program; `alder build` produces the same executable with or
without it. Only the `alder deploy`, `status`, `rollback`, `logs`,
`restore` and `ci init` commands read it.

### What the checker refuses

The checker reads the declaration like any other code, and reports
E0030 for what `alder deploy` could not use: an unknown target kind
(`Heroku`), an `Ssh` environment without `domain` or `path`, a `Fly`
environment without `app`, an empty `requires` name, a `floor` outside
0..1, an environment declared twice, a second `deploy`, or a `deploy`
outside `App`. Chapter 11's table of codes has each message.

It also refuses a secret. The file goes into version control, so it must
never hold one:

```alder fragment
deploy {
  production: Ssh "deploy@box.example.com" { domain: "blog.example.com", path: "/srv/blog" },
  requires: ["ghp_abcdefghijklmnopqrstuvwxyz0123456789"],  -- E0030
}
```

`alder check` points at the text: "a secret belongs in the environment,
not in `App`: name it in `requires`" (`go test ./book` appends this
fragment to an `App` too, and confirms that it is refused with that
message: the `-- E0030` comment tells the test so). A text that looks like a token (it starts `sk_`, `ghp_`, `AKIA`, `xoxb-`,
`xoxp-` or `-----BEGIN`, or is 32 or more base64 or hex characters
without a space) or a URL with a password in it (`smtp://user:pass@host`)
is refused wherever it appears in the declaration. The message never
echoes the value. A `requires` entry names the variable instead; where
its value comes from depends on the target, as each section below says.

The checker also compares `requires` with the code. The build already
scans the app's adapter packages (chapter 7), and when one of them reads
an environment variable by a literal name (`os.Getenv("SMTP_URL")`) that
`requires` does not list, `alder check` warns (W0006): "adapter `auth`
reads `SMTP_URL`, which `deploy.requires` does not name". The runtime's
own `ALDER_…` variables are never reported.

## The command, step by step

```text
alder deploy <env> [dir] [--host name] [--allow-data-loss] [--unsafe-skip-tests] [--dry-run] [--json]
```

The steps always run in this order, and every refusal happens before
anything at the target changes:

1. **Gate.** `alder check` (an error or a typed hole refuses), `alder
   build`, `alder test`, `alder test --mutate` against the floor, then
   each of `checks` in order.
2. **Artifact.** The directory `build/<app>/deploy/<build id>/`: the
   executable, built for the target's operating system and processor
   (a static binary: the SQLite driver is pure Go, so cross-compiling
   needs nothing but Go), the static files, the migrations, and
   `alder-deploy.json`, a description of the build (its id, the alder
   version, the git commit, the gate's results, the migration plan, the
   `requires` names).
3. **Target.** Connect; check that every `requires` name (and
   `ALDER_SECRET`, which the runtime itself needs) is set there; fetch
   the live database's schema; make the **plan**: which of the build's
   migrations the live database has not had. If one drops data and
   `--allow-data-loss` was not given, refuse. `--dry-run` stops here.
4. **Ship.** Upload the artifact; copy the database into the build's
   directory (`backup.db`); apply the migrations (`app --migrate-only`);
   start the new build on a free port; check its health; switch the
   traffic; stop the old build after its requests finish; prune old
   builds.
5. **Record.** Write `deployed.json` into the build's directory at the
   target: when, by whom, the gate's results, the plan, the flags.

The **build id** is the first 16 hex digits of the SHA-256 of the
executable. The runtime computes the same id for itself when it starts
and puts it in every page it serves, as
`<meta name="alder-build" content="…">` (chapter 13). That is how the
health check knows it is talking to the new build and not to the old one
still answering on the same machine: it asks for `/` until the page
names the new id.

### A transcript

Both example apps declare one environment, a `Script`, so they deploy
with nothing installed:

```alder fragment
deploy {
  local: Script "./scripts/deploy-local.sh",
}
```

The script only writes down what it was given. Here is a deploy of the
to-do app (`go test ./book` runs it in a copy of `examples/todos`; the
lines with `…` stand for the build id and the platform, which depend on
the machine):

```text command=todos
$ alder deploy local
alder deploy local → Script ./scripts/deploy-local.sh
gate
  check    ok
  build    ok
  test     ok  116 passed
  mutate   ok  score 0.99 ≥ floor 0.00 (none yet; 208 mutants, 2 survived)
artifact build/todos/deploy/… (…)
plan
  the target's schema is not known (the script applies the migrations): every migration is listed
  0001_initial
    create table Todo
    create table User
    add index on Todo.owner
script ./scripts/deploy-local.sh build/todos/deploy/… local
deployed … to local
```

Line by line:

- The first line names the environment and its target as the
  declaration writes it.
- `gate`: each fixed step and its result. The floor is `0.00 (none
  yet)`: the declaration has no `floor`, and a `Script` target keeps no
  record of the last deploy, so there is no earlier score to hold the
  build to. On an `Ssh` or `Fly` target the floor would be the score of
  the build serving now.
- `artifact`: the directory just written. Its name is the build id.
- `plan`: a `Script` target cannot be asked for its live schema, so
  every migration is listed and none is refused; the script is
  responsible for applying them (the executable's `--migrate-only` does
  it).
- `script`: the command that ran, with its two arguments.
- `deployed`: the script exited 0.

The status of a `Script` environment is the script's business:

```text command=todos
$ alder status local
alder status: `alder status` is not available for a Script target: environment local is deployed by its script, which keeps its own state
```

`--json` prints the same progress on standard error and the deploy
record (what `alder-deploy.json` holds) on standard output, for a
program to read.

### A deploy to a server (recorded)

The `Ssh` target is the one most apps use: a Linux box with SSH and a
public key. A test suite cannot rent a server, so this session is
**recorded**: it was run against a private `sshd` on a laptop with
`proxy: None` (the commands are real; `go test ./book` does not run
them). The app is the scaffold of chapter 2 after a migration added a
column `mood` to `Entry`:

```text
$ alder deploy production                                    (recorded)
alder deploy production → Ssh box:/…/srbox
gate
  check    ok
  build    ok
  test     ok  2 passed
  mutate   ok  score 1.00 ≥ floor 1.00 (last deploy; 3 mutants, 0 survived)
artifact build/sr/deploy/9395679f30b6bf05 (darwin/arm64)
target
  requires ok  SMTP_URL, ALDER_SECRET
plan
  live schema 2e82b15d5f85 → build schema 9c29f6e97d0b
  0002_add_mood
    rebuild table Entry: Entry: column order changed (ALTER TABLE only appends); Entry.mood added
warning: no `backup` in the declaration: only the pre-migration copies (builds/<id>/backup.db) exist
ship
  upload   ok  builds/9395679f30b6bf05/
  backup   ok  builds/9395679f30b6bf05/backup.db
  migrate  ok  applied 0002_add_mood
  start    ok  port 50665
  health   ok  build 9395679f30b6bf05 answers GET /
  switch   ok  current → 9395679f30b6bf05 (port 18977)
  stop     ok  build ac9e346ab3edd8c8 stops after 3s
deployed 9395679f30b6bf05 to production
```

What happened, in order:

- `floor 1.00 (last deploy)`: the build serving had a score of 1.00
  (its `deployed.json` says so), and with no declared `floor` that is
  the bar.
- `target`: `ssh` ran one shell script at the box that checked the
  environment file for each name. It ran `grep` on the file; no value
  crossed the connection.
- `plan`: the box's `db/schema.json` records the schema of the last
  migration its database took (`2e82b15d5f85` is the first 12 hex digits
  of its hash); the build's migrations are placed against it, and
  `0002_add_mood` is the one pending. Its step is a table rebuild
  (chapter 8, "How tables are stored").
- `upload`: the artifact went over as a tar stream into
  `builds/<id>.part`, renamed to `builds/<id>/` once complete, so a
  build directory is never half written.
- `backup`: the new build's executable copied the database with SQLite's
  `VACUUM INTO` while the old build kept serving it.
- `migrate`: `builds/<id>/app --migrate-only` applied the migration and
  exited; a refusal here (chapter 8, "When the app starts") would have
  stopped the deploy with nothing switched.
- `start`: the new build was started on a free port beside the old one.
- `health`: `curl` at the box asked the new build for `/` until its page
  carried `9395679f30b6bf05`.
- `switch`: with `proxy: None` the app itself listens on the public port
  (`<path>/port`), so the old build was stopped and the new one started
  again on that port: a moment without a listener. With Caddy (the
  default), the switch is one call to Caddy's admin API that changes the
  route's upstream port, and no request is dropped.
- `stop`: the old build gets its drain time (30 seconds by default; 3
  seconds in this session) to finish its requests, then a SIGTERM.

The state of the environment lives at the box, under `path`:

```text
<path>/builds/<id>/   app, static/, migrations/, alder-deploy.json   (the artifact)
                      run (a generated wrapper: reads <path>/env, sets ALDER_DB, ALDER_STATIC,
                           ALDER_ADDR and ALDER_BACKUP_URL; holds no value)
                      addr, log, pid, backup.db, deployed.json
<path>/current        the id of the build serving
<path>/port           the port it serves on
<path>/env            NAME=value lines, written by you (mode 600); deploy only checks names
<path>/db/app.db      the database, with db/schema.json and db/migrations (the last migration's)
```

Nothing about a deploy is kept on your laptop or in the repository. Two
people (or a laptop and CI) deploying the same app see the same state,
because they ask the same box.

Before the first deploy, write the environment file by hand, one
`NAME=value` per line, as a shell would read it (the `run` wrapper
sources it):

```text
$ ssh deploy@box.example.com 'mkdir -p /srv/blog && umask 077 && cat > /srv/blog/env'
SMTP_URL=smtp://mailer:…@mail.example.com
STRIPE_KEY=sk_live_…
ALDER_SECRET=…16 bytes or more: the output of `openssl rand -hex 32` will do
```

`alder deploy` warns when the file is readable by others.

How a build runs at the box: when systemd runs there, each build is an
instance of a template unit, `alder-<app>-<env>@<id>.service` (a
template, so the old and the new build can run side by side during the
switch); as `root` it is a system unit, otherwise a user unit with
lingering enabled so it survives your logout. Without systemd (a
container, a Mac) the build is a background process with a pid file.
Either way its output goes to `builds/<id>/log`, which `alder logs`
reads, so no journal permissions are needed. With `proxy: Caddy`, Caddy
is installed with `apt` on Debian and Ubuntu when it is missing (any
other system is refused with a sentence: install Caddy yourself or use
`proxy: None`), and the route for `domain` gets its certificate
automatically. A wildcard domain (`*.preview.example.com`) needs a
certificate by DNS challenge, which `alder deploy` does not set up:
give Caddy a DNS provider, or use one name per preview.

### Caddy

With `proxy: Caddy` the target talks to Caddy only through its admin
API (`http://127.0.0.1:2019`; `ALDER_DEPLOY_CADDY_ADMIN` names another
address): one route per environment, in a server named `alder` that
listens on `:443`, its upstream changed at each switch. A Caddy whose
admin API already answers is used as it is; one that is missing is
installed. Three things follow, and they matter most for a box that
already ran Caddy before alder came:

- **The routes live in Caddy's running configuration**, not in a
  Caddyfile. When alder installs Caddy it adds a systemd drop-in
  (`/etc/systemd/system/caddy.service.d/alder.conf`) that starts Caddy
  with `--resume`, so a restart reloads the last configuration, routes
  included. A Caddy alder did not install has no such drop-in: restarted
  from its Caddyfile, it forgets alder's routes and the app is
  unreachable until the next deploy or rollback sets the route again.
  Add `--resume` to that Caddy's command line yourself, or deploy again
  after each restart.
- **A server of your own on `:443`** (a Caddyfile site with a domain is
  one) conflicts with the `alder` server: the switch fails, with Caddy's
  error, and the previous build keeps serving. Give the app its own box,
  or use `proxy: None` behind your proxy.
- **The admin API answers every user of the box**: anyone who can run
  `curl` there can change Caddy's routes. That is Caddy's default; on a
  box shared with people you do not trust, put the admin API on a Unix
  socket only root can open (Caddy's `admin` option) and run the deploy
  as root.

alder's own tests drive the Caddy path against a fake Caddy (the admin
calls the target makes, and a proxy to the last route's upstream) with
an availability monitor that asks the public address which build
answers ten times a second; CI's `deploy-systemd` job runs the real
thing, systemd units and a Caddy the target installs, on a GitHub
runner.

## What the gate refuses, and why

The gate is fixed: the declaration can raise the floor or add checks,
but no field in the file can remove a step. Each refusal says what to do
and exits 1:

| Step | Refuses when | The message says |
|---|---|---|
| check | an error, or a typed hole is left | ``refused: `alder check` found 2 errors: fix them before deploying``, with the diagnostics |
| build | `alder build` fails | its last lines |
| test | a test fails | ``refused: `alder test` failed: 1 of 23 tests failed``, with the report |
| mutate | the score is below the floor | `refused: the mutation score 0.82 is below the floor 0.95 (declared): 4 mutants survived; …` |
| checks | a command exits non-zero | ``refused: the check `./scripts/lint-content` exited 1``, with its last 20 lines |
| requires | a name is not set at the target | `refused: the environment at Ssh … does not set SMTP_URL: add it to its environment file …; nothing was uploaded` |
| plan | a migration drops data | the DROPS DATA lines (next section) |
| health | the new build exits, or never answers with its id | its log; nothing was switched |

Why each one: a typed hole is an unfinished program; the tests are the
app's own definition of working; the mutation score is the measure of
whether the tests would notice a broken build (chapter 10, "Mutation
testing"), and holding it to the last deploy's score means a deploy can
never quietly make the tests weaker. A health check by build id catches
the failures no test can: a missing environment variable, an adapter
that cannot reach its server, an `ALDER_SECRET` of the wrong length.

There is one way past the tests, named for what it is:
`--unsafe-skip-tests` skips the tests, the mutation score and the
checks (never `check` and `build`), prints `test SKIPPED`, and records
`"unsafeSkipTests": true` in `deployed.json`, where `alder status`
shows it. It exists for an emergency fix when the test suite itself is
broken. It cannot be written in the file.

A build that fails its health check, recorded against the same box (an
adapter that exits at start):

```text
$ alder deploy production --unsafe-skip-tests                (recorded)
…
  migrate  ok  nothing to apply
  start    FAIL
alder deploy: refused: build c2e3dfbb30c5ce6f did not pass its health check: it was stopped and builds/c2e3dfbb30c5ce6f/ is kept for inspection; nothing was switched; build a615c9906f4cc26e still serves
build c2e3dfbb30c5ce6f exited at start; its log (builds/c2e3dfbb30c5ce6f/log):
--- alder deploy: start 2026-10-17T05:19:17Z on 127.0.0.1:0
boom: the clock adapter cannot reach its time server
```

## Migrations and the data-loss rule

A migration that drops a column or a table loses data the moment it
runs, and no rollback brings it back. So the plan marks those steps,
the same way `alder migrate` does (chapter 8), and the deploy refuses
them unless the command line says otherwise. Recorded, after a change
that dropped the `mood` column again:

```text
$ alder deploy production --dry-run                          (recorded)
…
plan
  live schema 9c29f6e97d0b → build schema 2e82b15d5f85
  0003_drop_mood
    drop column Entry.mood
    DROPS DATA: column Entry.mood
alder deploy: refused: the migration drops data
  DROPS DATA: 0003_drop_mood: column Entry.mood
run again with --allow-data-loss to apply it: the database is copied into the build's directory first (backup.db)
```

With `--allow-data-loss` the deploy goes ahead, and the order of the
steps is the guarantee: the database is copied into
`builds/<id>/backup.db` **before** the migration runs, and
`deployed.json` records the flag. Like `--unsafe-skip-tests`, the flag
exists only on the command line: a file that could say "always allow
data loss" would make the rule decorative.

A first deploy (a target with no database yet) drops nothing, whatever
its migrations say: every migration creates.

A migration that **rebuilds** a table (a column whose type changes, a
column added in the middle, chapter 8) copies the table under SQLite's
write lock: while it runs, the old build's writes wait. For a large
table that is a pause users notice, so run `--dry-run` first: it prints
the plan, `rebuild table …` included, without changing anything, and
you can choose a quiet moment.

## Status, rollback and logs

```text
alder status <env> [dir] [--host name]
alder rollback <env> [<build id>] [--dir dir] [--host name] [--allow-schema-mismatch]
alder logs <env> [dir] [--host name] [-f]
```

`alder status` reads the state at the target: the current build, the
last five builds with their records, the database's size, the last
backup. Recorded, after the deploy above:

```text
$ alder status production                                    (recorded)
Ssh box:/…/srbox
current  9395679f30b6bf05
builds
  * 9395679f30b6bf05  2026-10-17T05:18:46Z  by tavishpegram@tavishs-Mac-Mini.local (local tavishpegram)  score 1.00  migrated 0002_add_mood  running
    ac9e346ab3edd8c8  2026-10-17T05:17:19Z  by tavishpegram@tavishs-Mac-Mini.local (local tavishpegram)  score 1.00
database 60.2 kB
backup   2026-10-17T05:18:45Z (builds/9395679f30b6bf05/backup.db)
```

"By" is the SSH user at the box and the local `USER`: both, because a
CI runner and a laptop may log in as the same deploy user. A build with
no `deployed.json` is listed as "never switched to (failed or
interrupted; kept for inspection)".

`alder rollback` makes an earlier build current again: by default the
one that served before the current build, or the id you give (a prefix
of four or more digits will do). It starts that build beside the
current one, checks its health by build id, switches, and stops the
current one, exactly as a deploy does, without a gate (the build passed
its gate when it was deployed) and without touching the database.

That last point is the catch. A rollback never changes data, so a build
can only come back if the live database is the one it expects: its own
schema snapshot, and no migration it does not have. After a deploy that
migrated, rolling back past the migration is refused, naming the
tables:

```text
  schema   FAIL  the live database is newer than the build
alder rollback: refused: the live database's schema 9c29f6e97d0b is not build ac9e346ab3edd8c8's 2e82b15d5f85 (changed tables: Entry): that build would refuse to start on it, and rolling back never changes the data. Deploy a build with the schema you want, or pass --allow-schema-mismatch to try anyway
```

The refusal also catches the case the hashes cannot: a migration that
added a column and a later one that dropped it leave the same tables,
but the database has had a migration the older build lacks, and the
runtime refuses that database ("recorded as applied but its file is
missing", chapter 8).

`--allow-schema-mismatch` tries anyway. It is honest about what it is:
the old build's own start-up check normally refuses the newer database,
so the health check fails, nothing is switched, and the refusal says so.
It is there for the rare change the runtime accepts (a new table the old
build never reads), not as a way back across a migration. The way back
across a migration is forward: deploy a build whose migrations take the
schema where you want it, or restore a backup.

One more caveat, about a deploy that fails after its migrations ran.
The migrations apply before the new build's health check (they must:
the new build cannot start on the old schema). If the health check then
fails, the previous build is still serving, now on the migrated
database. For an additive migration (a new table, a new column with a
default) the previous build does not notice. For anything else, the
database copy taken just before, `builds/<id>/backup.db`, is the way
back: `alder restore` puts a database file in place (below).

`alder logs` prints the last 100 lines of the current build's log, and
`-f` follows it until you press Ctrl-C.

## Backups and restore

`builds/<id>/backup.db` is a copy taken before each migration: good for
undoing a deploy, useless when the disk dies. For that, the runtime
streams the database to a bucket while it serves. Add a location to the
declaration:

```alder fragment
deploy {
  production: Ssh "deploy@box.example.com" { domain: "blog.example.com", path: "/srv/blog" },
  requires: ["SMTP_URL", "AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY"],
  backup: S3 "s3://blog-backups/production",
}
```

`alder deploy` sets `ALDER_BACKUP_URL` for the app (through the `run`
wrapper), and the app ships its database there: when it begins a
generation (below), a **snapshot** of the database file; then every ten seconds
(`ALDER_BACKUP_INTERVAL`), the new pages SQLite has written to its
write-ahead log since the last upload, as a **WAL segment**; once more
when it shuts down. It is the design of Litestream, cut to what one app
needs:

```text
s3://blog-backups/production/
  generations/20261017T101500.000Z-3f2a/snapshot.db
  generations/20261017T101500.000Z-3f2a/wal/00000001-1760696110000.wal
  generations/20261017T101500.000Z-3f2a/wal/00000002-1760696120000.wal
  …
```

A **generation** is a snapshot and the unbroken run of segments after
it. To keep the log unbroken, the runtime turns off SQLite's automatic
checkpoint while it ships, and checkpoints itself right after each
upload. Anything that breaks the chain (a restart it cannot continue
from, a failed upload, another process checkpointing the file) starts a
new generation with a fresh snapshot.

The bucket's credentials are environment variables like any other
secret (`AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, and
`AWS_ENDPOINT_URL` for an S3-compatible service other than AWS), so
they belong in `requires`; `alder deploy` warns when they are missing
from it. Without a `backup`, every deploy warns once that only the
pre-migration copies exist. The declaration takes only `S3` locations;
for a directory (a mounted disk), set `ALDER_BACKUP_URL=/mnt/backups/blog`
in the box's environment file instead.

`alder restore` rebuilds a database from the bucket:

```text
alder restore <env> [--from url] [--to time] [--out file.db] [--dir dir]
```

It downloads the newest snapshot at or before `--to` (a UTC time such as
`2026-10-17T10:00:00Z`; default: the latest), applies the segments up to
that time through SQLite itself, and checks the result's page count
against the last commit. With `--out` it writes the file locally and
stops there. Without it, it puts the database in place at the target:
the current build is stopped, the old database is kept beside it as
`app.db.before-restore-<time>`, and the build is started again and
health-checked. `--from` defaults to the declaration's `backup`; the
credentials come from your own environment, because the restore runs on
your machine.

`alder status` shows the newest of the last shipment and the last
pre-migration copy: `backup 2026-10-17T10:15:30Z (streamed to
s3://blog-backups/production)`.

## CI: `alder ci init`

The pipeline is the command, so a CI workflow is short: run the same
commands a laptop would. `alder ci init` writes it from the declaration:

```text
alder ci init [dir] [--github] [--force]
```

For the to-do app, whose only environment is a `Script` named `local`,
the workflow checks and tests and deploys nothing:

```text command=todos
$ alder ci init
wrote .github/workflows/alder.yml
  pull request  alder check, alder test --mutate
  push to main  alder check, alder test --mutate
no `production` or `preview` environment in App's `deploy`: the workflow deploys nothing
repository secrets it needs: none
…
```

(When `actionlint` is on your `PATH`, `alder ci init` runs it on the
file and prints `actionlint ok`; the `…` stands for that line.)

For the declaration at the top of this chapter it writes this file,
`.github/workflows/alder.yml` (`go test ./book` compares the block with
the golden file the command's own tests use,
`cmd/alder/testdata/ci/full.yml`):

```yaml golden=cmd/alder/testdata/ci/full.yml
# Written by `alder ci init` (book chapter 16). The pipeline is alder's own commands: what
# runs here runs the same from a laptop.
#
#   pull request   alder check; alder test --mutate --min 0.95; alder deploy preview --host
#                  <number>.preview.example.com, and a comment with its address
#   push to main   alder check; alder test --mutate --min 0.95; alder deploy production
#
# Repository secrets (Settings > Secrets and variables > Actions):
#   SSH_PRIVATE_KEY  a private key deploy@box.example.com accepts (production, preview)
#   SSH_KNOWN_HOSTS  the host key lines of box.example.com: ssh-keyscan box.example.com
#
# Not repository secrets: SMTP_URL, STRIPE_KEY, ALDER_SECRET of production and preview live in
# the box's environment file (<path>/env, NAME=value lines); `alder deploy` checks that they are
# set there.
#
# A preview stays up after its pull request is closed: remove it by hand for now.
name: alder

on:
  pull_request:
  push:
    branches: [main]

permissions:
  contents: read

env:
  ALDER_VERSION: v0.1.0

jobs:
  check:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.27"
      - name: Install alder
        env:
          GH_TOKEN: ${{ github.token }}
        run: |
          gh release download "$ALDER_VERSION" --repo tapegram/alder --pattern 'alder_*_linux_amd64.tar.gz' --output "$RUNNER_TEMP/alder.tar.gz"
          tar -xzf "$RUNNER_TEMP/alder.tar.gz" -C "$RUNNER_TEMP" alder
          echo "$RUNNER_TEMP" >> "$GITHUB_PATH"
      - run: alder check
      - run: alder test --mutate --min 0.95

  preview:
    # a pull request from a fork gets no secrets: it is checked, not deployed
    if: github.event_name == 'pull_request' && github.event.pull_request.head.repo.full_name == github.repository
    needs: check
    runs-on: ubuntu-latest
    permissions:
      contents: read
      pull-requests: write
    concurrency:
      group: preview-${{ github.event.pull_request.number }}
    env:
      HOST: ${{ github.event.pull_request.number }}.preview.example.com
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.27"
      - name: Install alder
        env:
          GH_TOKEN: ${{ github.token }}
        run: |
          gh release download "$ALDER_VERSION" --repo tapegram/alder --pattern 'alder_*_linux_amd64.tar.gz' --output "$RUNNER_TEMP/alder.tar.gz"
          tar -xzf "$RUNNER_TEMP/alder.tar.gz" -C "$RUNNER_TEMP" alder
          echo "$RUNNER_TEMP" >> "$GITHUB_PATH"
      - name: SSH key
        env:
          SSH_PRIVATE_KEY: ${{ secrets.SSH_PRIVATE_KEY }}
          SSH_KNOWN_HOSTS: ${{ secrets.SSH_KNOWN_HOSTS }}
        run: |
          install -m 700 -d ~/.ssh
          printf '%s\n' "$SSH_PRIVATE_KEY" > ~/.ssh/id_ed25519
          chmod 600 ~/.ssh/id_ed25519
          printf '%s\n' "$SSH_KNOWN_HOSTS" >> ~/.ssh/known_hosts
      - name: alder deploy preview
        run: alder deploy preview --host "$HOST"
      - name: Comment the address
        env:
          GH_TOKEN: ${{ github.token }}
          PR: ${{ github.event.pull_request.number }}
          SHA: ${{ github.event.pull_request.head.sha }}
        run: |
          body="Preview of $SHA: https://$HOST"
          gh pr comment "$PR" --edit-last --body "$body" || gh pr comment "$PR" --body "$body"

  production:
    if: github.event_name == 'push' && github.ref == 'refs/heads/main'
    needs: check
    runs-on: ubuntu-latest
    concurrency:
      group: production
      cancel-in-progress: false
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.27"
      - name: Install alder
        env:
          GH_TOKEN: ${{ github.token }}
        run: |
          gh release download "$ALDER_VERSION" --repo tapegram/alder --pattern 'alder_*_linux_amd64.tar.gz' --output "$RUNNER_TEMP/alder.tar.gz"
          tar -xzf "$RUNNER_TEMP/alder.tar.gz" -C "$RUNNER_TEMP" alder
          echo "$RUNNER_TEMP" >> "$GITHUB_PATH"
      - name: SSH key
        env:
          SSH_PRIVATE_KEY: ${{ secrets.SSH_PRIVATE_KEY }}
          SSH_KNOWN_HOSTS: ${{ secrets.SSH_KNOWN_HOSTS }}
        run: |
          install -m 700 -d ~/.ssh
          printf '%s\n' "$SSH_PRIVATE_KEY" > ~/.ssh/id_ed25519
          chmod 600 ~/.ssh/id_ed25519
          printf '%s\n' "$SSH_KNOWN_HOSTS" >> ~/.ssh/known_hosts
      - name: alder deploy production
        run: alder deploy production
```

Three jobs, because the command is the pipeline: `check` runs on every
pull request and every push; `preview` and `production` each run one
`alder deploy`, which runs the whole gate again before it ships (a
deploy never trusts that another job checked). What the file is made
of:

- **Installing alder.** The workflow downloads the release archive of
  the alder that wrote it (`ALDER_VERSION`), with GitHub's own `gh`, and
  puts it on the `PATH`; a development build of alder writes a file that
  installs the latest release instead. Go comes from `actions/setup-go`,
  because `alder build` generates Go and compiles it.
- **The floor.** `--min 0.95` is the declared `floor`. Without one, the
  `check` job runs `alder test --mutate` without `--min`, and the deploy
  holds the score to the last deploy's, which only the target knows.
- **Secrets.** The header names every repository secret the jobs read,
  and what each is. For an `Ssh` environment that is the SSH key and the
  box's host key; the app's own secrets stay in the box's environment
  file, so they never pass through CI at all. For a `Fly` or `Script`
  environment the values are read from the runner's environment, so each
  `requires` name (and `ALDER_SECRET`, and `FLY_API_TOKEN`) is a
  repository secret, passed to the deploy step by name.
- **Forks.** GitHub gives a pull request from a fork no secrets, so the
  `preview` job runs only for branches of the repository itself.
- **One deploy at a time.** Each environment is a `concurrency` group,
  so two pushes to `main` deploy one after the other, never interleaved.

`alder ci init` refuses to replace an existing file (`--force`
replaces it), and lints what it wrote with `actionlint` when it is
installed. GitHub Actions is the only CI it writes for now.

### The preview environment

A pull request gets its own copy of the app, at its own address, for as
long as it is open. The declaration's `preview` is the template:

```alder fragment
deploy {
  production: Ssh "deploy@box.example.com" { domain: "blog.example.com", path: "/srv/blog" },
  preview:    Ssh "deploy@box.example.com" { domain: "*.preview.example.com", path: "/srv/blog-preview" },
}
```

and the workflow fills in the pull request's number:

```text
alder deploy preview --host 42.preview.example.com
```

`--host` overrides the environment's `domain` for that one run. The
host must match a wildcard domain (`42.other.example.com` is refused).
For an `Ssh` target, each host gets its own state: the build lives under
`<path>/<host>` (`/srv/blog-preview/42.preview.example.com/`), with its
own database, its own systemd unit and its own Caddy route, as the
environment `preview-42.preview.example.com`. So two open pull requests
never replace each other, and a second push to the same pull request is
an ordinary deploy to that host: the new build replaces the old one,
the gate and the plan and the health check included, and the workflow
edits its earlier comment instead of adding another. `alder status`,
`logs` and `rollback` take the same `--host` to reach that copy. A
`Script` target gets the host in `ALDER_HOST`; a `Fly` environment
refuses `--host` (its address is its app's, `<app>.fly.dev`), so a Fly
preview is one app that every pull request deploys to, the last push
winning.

A preview's database starts empty: its first deploy creates it from the
migrations. And when the pull request is closed, the preview **stays
up**: nothing removes `<path>/<host>`, its unit or its route yet. Remove
them by hand for now (`systemctl stop` the unit, delete the directory,
and the route goes when Caddy reloads its configuration); cleaning up
on close is planned.

## The Fly target

**Experimental.** The `Fly` target has run only against a fake of Fly's
API in alder's own tests; it has never run against Fly itself. Every Fly
deploy says so (`warning: the Fly target is experimental: it has never
run against the real Fly API`), and until it is finished (milestone
M15) it **refuses any deploy that would apply a migration to an
existing database**:

```text
alder deploy: refused: Fly target: migrations are not supported yet; deploy a build with no pending migrations, or use Ssh (pending: 0002_add_mood; nothing was shipped)
```

The reason is at the end of this section. A first deploy, which creates
the database from the migrations, is allowed: there is no data to lose
and no earlier build to go back to. Nor does the Fly target have the
`Ssh` target's deploy lock or `rollback --restore-backup` yet.

`Fly "ord" { app: "blog-staging" }` deploys to [Fly.io](https://fly.io),
which runs **OCI images** (container images) on its own machines close to
a region. The target turns the artifact into one:

- **The image.** The artifact's executable, static files and migrations
  on a small Alpine base, in the shape of the Dockerfile alder itself
  ships (chapter 11, "Cutting a release": a base image, the binary
  copied in, a command). An app needs no Go at run time, so the base is
  Alpine, which adds a shell and the CA certificates the adapters' TLS
  needs. The target writes the Dockerfile, a start script and a
  `fly.toml` beside a copy of the artifact, and runs `flyctl deploy
  --build-only --push --image-label <build id>`: Fly's remote builder
  builds the image and pushes it to `registry.fly.io/<app>:<build id>`.
  No Docker is needed on your machine; `flyctl` is. Without it the
  deploy refuses: "the Fly target builds and pushes the build's image
  with `flyctl`, which is not on PATH: install it".
- **The machine and the volume.** Everything else goes through Fly's
  Machines API over HTTP (`https://api.machines.dev/v1`), authenticated
  with `FLY_API_TOKEN`. A deploy token belongs to an app, so either
  create the app first (`fly apps create blog-staging`, then `fly tokens
  create deploy -a blog-staging`), or use an organization token (`fly
  tokens create org`) and let the first deploy create the app (in the
  organization `FLY_ORG`, default `personal`). It gives the app public addresses (`flyctl ips
  allocate-v4 --shared` and `allocate-v6`), and creates a 1 GB volume
  named `alder_data`, and one machine with the volume mounted at
  `/data`, serving ports 80 and 443 of `https://<app>.fly.dev`.
- **Secrets.** Each `requires` name and `ALDER_SECRET` are read from
  **your** environment at deploy time (or the CI runner's) and set as Fly
  secrets with `flyctl secrets import`, the values on its standard input:
  never in a file, an argument, the image or the output. A name neither
  your environment nor the Fly app has is refused before anything is
  pushed; a name the Fly app already has is fine.
- **State.** The volume holds the same layout as an `Ssh` target's
  `path` (`/data/builds/<id>/`, `/data/current`, `/data/db/app.db`), so
  `status`, the plan and the rollback checks read it the same way, by
  running small shell commands in the machine through the API.

One thing works differently, and it follows from SQLite. A Fly volume
attaches to one machine, and the database wants one writer, so a Fly
environment is one machine, and a new build **replaces** the old one in
place: the target swaps the machine's image, and Fly restarts it. The
image's start script applies the migrations (`--migrate-only`) and then
serves. There is a pause of a few seconds while the machine restarts
(Fly's proxy holds the requests meanwhile), and there is no moment when
both builds run. The guarantees are kept the other way round: a build
whose migrations refuse, or that fails its health check (asked through
the public address, `https://<app>.fly.dev/`, until the page names the
build id), is replaced by the previous image again, and the refusal
says the previous build still serves. A rollback is the same swap to an
older image. `alder logs` is `flyctl logs`; `alder restore` sends the
database into the volume through the API and restarts the machine,
whose start script puts it in place. Old images stay in Fly's registry:
pruning removes only their directories on the volume.

Why migrations are refused for now: on Fly the migrations run in the new
image's start script, on the one machine. If the new build then fails
its health check, the target puts the previous image back, and that
image's start script runs the runtime's start-up check on a database
that has had a migration the previous build does not know: it refuses
(chapter 8, "When the app starts"), and the machine stops. The site would
be down until someone restored the copy by hand. The `Ssh` target avoids
that because the old build keeps running beside the new one and the copy
can be put back (`rollback --restore-backup`, and automatically after a
deploy whose migrations dropped data). The Fly target will get the same
when its start script can put `builds/<id>/backup.db` back on a revert
and it has a lock and `--restore-backup`. Three more things a first real
run must check: a large database's copy (`VACUUM INTO`) and `alder
restore`'s upload (in pieces of 48 kB) both run through the Machines
API's `exec`, which times out after 60 seconds; and the
`--allow-data-loss` flag never reaches a Fly machine (no deploy that
could use it gets that far).

The target's unit tests run against a fake Machines API. A real deploy
is a manual test, because it needs an account and costs a little:

```text
FLY_API_TOKEN=… ALDER_FLY_APP=alder-test-you go test -tags fly ./cmd/alder -run DeployFly -v
```

It deploys the scaffold twice, checks the public address serves each
build, rolls back, and leaves the app for `fly apps destroy`.

## The Script hatch

`Script "./deploy.sh"` is for everything alder does not ship to itself:
a platform with its own CLI, a Kubernetes cluster, a box behind a
bastion. The gate and the artifact are the same as for any target;
then the script runs from the app's root with two arguments, the
artifact directory (absolute) and the environment's name, and the
`requires` names, comma-separated, in `ALDER_REQUIRES` (and the host in
`ALDER_HOST` when `--host` was given). Its exit code is the deploy's
result. The examples' script is the smallest possible one:

```text
#!/bin/sh
set -eu
artifact="$1"
env="$2"
{
  echo "args: $*"
  echo "env: $env"
  echo "build: $(basename "$artifact")"
  echo "ALDER_REQUIRES: ${ALDER_REQUIRES-}"
} > "$artifact.deploy-local.txt"
```

A real one copies `$artifact/app` and its `static/` and `migrations/`
somewhere, runs `app --migrate-only` there, and starts `app` with
`ALDER_DB`, `ALDER_ADDR`, `ALDER_STATIC` and `ALDER_SECRET` set (chapter
11, "alder build"). Since the command cannot see what the script did,
`status`, `rollback`, `logs` and `restore` (without `--out`) say they are
not available for a `Script` target, and its plan lists every migration
without refusing any.

## SQLite in production

Every Alder app stores its data in one SQLite file (chapter 8). That
choice shapes what deploying means, and the facts are worth knowing
before the first user arrives:

| Fact | What it means |
|---|---|
| One writer at a time | SQLite serializes writes; reads go on in parallel (WAL mode). A small app's writes take microseconds, so this is rarely felt, but a long write transaction makes every other write wait. |
| One box | The database is a file on one machine's disk; there is no second server to fail over to. A busier app grows by a bigger machine, not more of them. |
| The disk is the risk | Lose the disk and you lose the data since the last backup. The pre-migration copies live on the same disk; the streamed backup (`backup:`) is what survives it, within ten seconds. |
| A rebuild takes the lock | A migration that rebuilds a table copies it under the write lock; `--dry-run` shows it so you can choose when. |
| Zero downtime on one box | Two processes can open the file: the new build migrates and starts while the old one drains, which is how the `Ssh` target switches without dropping requests. |

When the app outgrows one box (writes from several regions, or more
than one server's worth of load), the answer is a database built for
it: SQLite's protocol over the network, with replicas. Turso is the
target planned for that (milestone M15, partitions and silos); until
then, one box and a streamed backup is the honest shape of an Alder app
in production.

## The knobs

A few environment variables change how the commands behave, for tests
and for unusual setups:

| Variable | Effect |
|---|---|
| `ALDER_SSH_CONFIG` | an `ssh_config` file every `ssh` reads (`ssh -F`): an identity, a jump host, a known-hosts file |
| `ALDER_DEPLOY_DRAIN` | how long the old build drains after the switch (default `30s`; `0` stops it at once) |
| `ALDER_DEPLOY_HEALTH_TIMEOUT` | how long a new build has to pass its health check (default `60s`) |
| `ALDER_DEPLOY_SUPERVISOR` | `process` runs builds as background processes even where systemd runs |
| `ALDER_BACKUP_INTERVAL` | how often the runtime ships the database (default `10s`) |
| `FLY_API_TOKEN`, `FLY_ORG` | the Fly target's token, and the organization a new Fly app is created in |

## What is not there yet

There is no lock: two deploys to one environment at the same moment
can interleave (the CI workflow's `concurrency` groups avoid it there).
A preview is not removed when its pull request closes. Wildcard
certificates need a DNS challenge alder does not configure. Only GitHub
Actions has a workflow. And an app runs on one box, by design, until
the Turso target exists.
