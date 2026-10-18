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
`Fly` and `Render` targets, the `Script` escape hatch, and what running SQLite in
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
| `<env>: Render "region" { service, database, instances?, plan? }` | a Render web service running an image alder pushes, its data in a Turso `database` (see "The Render target") |
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

The texts of a target reach `ssh`'s command line, shell scripts, a
systemd unit and Fly's API, so each is held to the characters it can
be: an `Ssh` host is letters, digits, `.`, `-`, `_`, `@` (and `:`, `[`,
`]` for an address), a `user` letters, digits, `.`, `-`, `_`, a
`domain` letters, digits, `.` and `-` (optionally starting `*.`), none
of them beginning with `-`; a `path` is absolute, with no control
character, `"`, `\`, `$`, `` ` ``, `%` or `..`; a `Fly` region is Fly's
three-letter code and an `app` lowercase letters, digits and `-`. A
`--host` given on the command line is held to the domain's set.

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
4. **Ship.** Take the environment's lock (one deploy, rollback or
   restore at a time); prepare what the build needs (the systemd unit,
   Caddy); upload the artifact; copy the database into the build's
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
alder deploy production → Ssh alder-test-box:/…/box
gate
  check    ok
  build    ok
  test     ok  2 passed
  mutate   ok  score 1.00 ≥ floor 1.00 (last deploy; 3 mutants, 0 survived)
artifact build/demo/deploy/46ea832c79966e56 (darwin/arm64)
target
  requires ok  SMTP_URL, ALDER_SECRET
plan
  live schema 2e82b15d5f85 → build schema 9c29f6e97d0b
  0002_add_mood
    rebuild table Entry: Entry: column order changed (ALTER TABLE only appends); Entry.mood added
warning: no `backup` in the declaration: only the pre-migration copies (builds/<id>/backup.db) exist
ship
  upload   ok  builds/46ea832c79966e56/
  backup   ok  builds/46ea832c79966e56/backup.db
  migrate  ok  applied 0002_add_mood
  start    ok  port 54466
  health   ok  build 46ea832c79966e56 answers GET /
  switch   ok  current → 46ea832c79966e56 (port 54450)
  stop     ok  build 339250fe1b936905 already stopped
deployed 46ea832c79966e56 to production
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
  carried `46ea832c79966e56`.
- `switch`: with `proxy: None` the app itself listens on the public port
  (`<path>/port`), so the old build was stopped and the new one started
  again on that port: a moment without a listener. With Caddy (the
  default), the switch is one call to Caddy's admin API that changes the
  route's upstream port, and no request is dropped.
- `stop`: with `proxy: None` the switch has already stopped the old
  build, so there is nothing left to do. Behind Caddy the old build is
  still running at this point, and gets its drain time (30 seconds by
  default, `ALDER_DEPLOY_DRAIN`) to finish its requests, then a SIGTERM:
  the line reads `stop ok build … stops after 30s (unless it is current
  again by then)`, because a rollback inside that window keeps it.

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

Everything the deploy writes there is the deploy user's alone: every
script runs under `umask 077`, `builds/` is mode 750, each `run` and
`app` 700, each `backup.db` and the database 600. An environment file
that sets `ALDER_DEV` is refused (development mode, with its public
cookie key and errors shown to visitors, never runs at a target), and
`run` unsets it anyway.

Nothing about a deploy is kept on your laptop or in the repository. Two
people (or a laptop and CI) deploying the same app see the same state,
because they ask the same box.

**One run at a time.** `deploy`, `rollback` and `restore` each take the
environment's lock first, and a second run is refused while one holds
it:

```text
alder deploy: refused: another deploy, rollback or restore of production is running (the lock at /srv/blog/deploy.lock is held by deploy@box (local ada) since 2026-10-17T07:24:34Z, pid 25904): wait for it to finish
```

The lock is held by an `ssh` session that lives as long as the run: a
`flock` on `<path>/deploy.lock`, or, where `flock` is missing, a
directory `<path>/deploy.lock.d` naming its holder. A run that dies
releases it with its session; a directory left by a killed run is
taken over by the next one, which says so (`warning: took over a stale
deploy lock …`). One case needs a hand: a session that hangs without
dying (a laptop that went to sleep mid-deploy) holds the lock until the
box's `sshd` notices, which can take a while. When you are sure no run
is going on, remove it: `ssh deploy@box.example.com rm -rf
/srv/blog/deploy.lock /srv/blog/deploy.lock.d`.

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
lingering enabled so it survives your logout; systemd restarts a build
that crashes (`Restart=on-failure`), and the switch enables the current
build's unit so it starts again at boot. Without systemd (a container,
a Mac, or `ALDER_DEPLOY_SUPERVISOR=process`) the build is a background
process with a pid file, and **nothing restarts it**: a build that
crashes, or a box that reboots, stays down until the next deploy or
rollback (`alder deploy` of the current build starts it again). Either
way its output goes to `builds/<id>/log`, which `alder logs` reads, so
no journal permissions are needed. The log grows while the build runs;
when a build starts with more than 10 MB of it, it is trimmed to its
last megabyte (a line says so). A build that runs for months with a
chatty adapter wants `logrotate` with `copytruncate` on
`<path>/builds/*/log`. With `proxy: Caddy`, Caddy
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
| plan (Fly) | any pending migration on an existing database | "Fly target: migrations are not supported yet; …" ("The Fly target") |
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

The rule is also enforced where the migration is applied, not only in
the plan. The app's executable itself, at `--migrate-only` and when it
starts, refuses a pending migration whose header says DROPS DATA on a
database that already had migrations, unless the environment variable
`ALDER_ALLOW_DATA_LOSS=1` is set (development mode aside). `alder
deploy` sets that variable for the one `--migrate-only` it runs, and
only with `--allow-data-loss`. So a `Script` target, whose plan cannot
see the live schema and lists every migration ("DROPS DATA (if not
applied yet)"), is covered too: the script's environment carries
`ALDER_ALLOW_DATA_LOSS=1` only when the command line said so, and its
`app --migrate-only` refuses otherwise.

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
alder rollback <env> [<build id>] [--dir dir] [--host name] [--allow-schema-mismatch | --restore-backup [--yes]]
alder logs <env> [dir] [--host name] [-f]
```

`alder status` reads the state at the target: the current build, the
last five builds with their records, the database's size, the last
backup. Recorded, after a deploy like the one above (another session,
so other ids):

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
interrupted; kept for inspection)". When the app is told to stream its
backup but nothing has arrived yet, the last line says so instead:
`backup   never shipped to s3://blog-backups/production: …` (the app's
log says why, usually the bucket's credentials).

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
tables, and the command that would cross it (recorded against the box,
after a deploy that added `mood`):

```text
$ alder rollback production                                  (recorded)
rollback 46ea832c79966e56 → 339250fe1b936905
  schema   FAIL  the live database is newer than the build
alder rollback: refused: the live database's schema 9c29f6e97d0b is not build 339250fe1b936905's 2e82b15d5f85 (changed tables: Entry.Entry): that build would refuse to start on it, and rolling back never changes the data. Deploy a build with the schema you want, or pass --allow-schema-mismatch to try anyway; or `alder rollback production 339250fe1b936905 --restore-backup` puts back the copy of the data taken when that build was replaced (builds/46ea832c79966e56/backup.db), losing every write since
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
build never reads), not as a way back across a migration.

### Back across a migration: `--restore-backup`

The usual way back across a migration is forward: deploy a build whose
migrations take the schema where you want it. When that is not
possible, `--restore-backup` goes back with the **data**: it puts back
the copy of the database taken just before the build was replaced (the
`backup.db` of the deploy that replaced it, whose `backup.of` names the
build), and every write since is lost. That is a data-loss operation, so
it asks first, on the terminal; `--yes` answers without one (a script,
CI), and without either it changes nothing:

```text
$ alder rollback production --restore-backup < /dev/null      (recorded)
rollback 46ea832c79966e56 → 339250fe1b936905
alder rollback: not confirmed: nothing was changed (pass --yes to confirm without a terminal)
$ alder rollback production --restore-backup --yes           (recorded)
rollback 46ea832c79966e56 → 339250fe1b936905
warning: --restore-backup: the database is now builds/46ea832c79966e56/backup.db (taken 2026-10-17T08:34:02Z); every write since is gone from it (the database it replaces is kept as db/app.db.before-rollback-20261017T083403Z)
  restore  ok  builds/46ea832c79966e56/backup.db
  start    ok  port 54362
  health   ok  build 339250fe1b936905 answers GET /
  switch   ok  current → 339250fe1b936905 (port 54336)
rolled back to 339250fe1b936905 with the data of 2026-10-17T08:34:02Z
```

In order: **every** running build is stopped first (the current one,
and a previous one still draining after a switch, its delayed stop
cancelled: no process may keep the replaced file open and go on serving
it); the database (with its `-wal` and `-shm`, `schema.json` and the
migration list) is kept as `db/app.db.before-rollback-<time>`; the copy
goes in place, mode 600, with the target build's schema records; the
build is started fresh on it, health checked and switched to. If it
fails its health check on the copy, everything is undone: the kept
database goes back and the build that served before starts again. The
rollback is recorded in the build's `deployed.json` (`rolledBack`,
`restoredFrom`). `--restore-backup` works onto the current build too:
that is the way back from a deploy that failed after its migrations.

### A deploy that fails after its migrations

The migrations apply before the new build's health check (they must:
the new build cannot start on the old schema). If the health check then
fails, the previous build is still serving, now on the migrated
database. For an additive migration (a new table, a new column with a
default) the previous build does not notice; for anything else it
would refuse to start again on that database, and the refusal says how
to go back (the new build's id shortened here):

```text
alder deploy: refused: build 3c7f… did not pass its health check: it was stopped and builds/3c7f…/ is kept for inspection; nothing was switched; build 46ea832c79966e56 still serves, but the database was migrated (0003_add_tag) and that build would refuse to start again on it. To go back: `alder rollback production 46ea832c79966e56 --restore-backup` (puts builds/3c7f…/backup.db, taken before the migration, in place)
```

When the migrations **dropped data** (the plan had DROPS DATA and the
command line said `--allow-data-loss`), waiting is worse: the build
still serving is already failing its writes to the table it no longer
matches. So the deploy does not ask. It puts this run's copy back at
once and starts the previous build on it (recorded):

```text
$ alder deploy production --allow-data-loss --unsafe-skip-tests (recorded)
…
plan
  live schema 9c29f6e97d0b → build schema 2e82b15d5f85
  0003_drop_mood
    drop column Entry.mood
    DROPS DATA: column Entry.mood
…
ship
  upload   ok  builds/db5fc6ca2c62c076/
  backup   ok  builds/db5fc6ca2c62c076/backup.db
  migrate  ok  applied 0003_drop_mood
  start    FAIL
the migrations dropped data and build db5fc6ca2c62c076 failed: putting builds/db5fc6ca2c62c076/backup.db (taken before them) back, and build 46ea832c79966e56 on it
  restore  ok  builds/db5fc6ca2c62c076/backup.db (the database it replaced is kept as db/app.db.before-rollback-20261017T083409Z)
  start    ok  port 54378
  health   ok  build 46ea832c79966e56 answers GET /
  switch   ok  current → 46ea832c79966e56 (port 54336)
alder deploy: refused: build db5fc6ca2c62c076 did not pass its health check after its migrations dropped data (0003_drop_mood): the database copied before them (builds/db5fc6ca2c62c076/backup.db) was put back automatically and build 46ea832c79966e56 serves again on it; the migrated database is kept as db/app.db.before-rollback-20261017T083409Z (a write made between the migration and now is only there); builds/db5fc6ca2c62c076/ is kept for inspection
build db5fc6ca2c62c076 exited at start; its log (builds/db5fc6ca2c62c076/log):
--- alder deploy: start 2026-10-17T08:34:08Z on 127.0.0.1:0
boom: the clock adapter cannot reach its time server
```

The dropped column is back, the previous build serves and takes writes;
the few seconds between the migration and the restore are in the kept
file, if anything was written then.

`alder logs` prints the last 100 lines of the current build's log, and
`-f` follows it until you press Ctrl-C: it follows the **current**
build, so after a deploy or rollback switches builds it goes on with
the new current build's log (a line `--- alder logs: build <id> is
current` marks the change).

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
checkpoint while it ships, and checkpoints itself right after copying
each segment. Anything that breaks the chain (a restart it cannot
continue from, another process checkpointing the file) starts a new
generation with a fresh snapshot. One build ships at a time (a lock
beside the database: during a switch the new build's shipper waits for
the old one's to finish).

When the bucket fails, the copied segments wait on the box's disk and
are retried in order, the waits doubling up to five minutes; the WAL
stays small meanwhile, because the checkpoints go on. Past 256 MB of
waiting segments they are dropped, and a new generation begins when the
bucket answers again. The snapshot never counts toward that limit: a
database larger than it is shipped like any other.

Three edges are worth knowing. The snapshot is a plain copy of the
database file, which is safe because only the shipper checkpoints; a
process that checkpoints the file meanwhile (an `sqlite3` you opened on
the box, with its default automatic checkpoint) can tear that copy, and
the restore's integrity check then refuses it: the next generation is
whole again. A segment still waiting when a build stops is not
uploaded: its frames are in the database file by then, and the next
build's first snapshot holds them, so no data is missing from the
bucket, only those points in time. And `status` says `never shipped`
while nothing has arrived.

The bucket's credentials are environment variables like any other
secret (`AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, and
`AWS_ENDPOINT_URL` for an S3-compatible service other than AWS), so
they belong in `requires`; `alder deploy` warns when they are missing
from it. Without a `backup` (in the declaration or the environment
file), every deploy warns once that only the pre-migration copies
exist. The declaration takes only `S3` locations; for a directory (a
mounted disk), set `ALDER_BACKUP_URL=/mnt/backups/blog` in the box's
environment file instead (the deploy then does not warn).

`alder restore` rebuilds a database from the bucket:

```text
alder restore <env> [--from url] [--to time] [--out file.db] [--dir dir] [--yes]
```

It downloads the newest snapshot at or before `--to` (a UTC time such as
`2026-10-17T10:00:00Z`; default: the latest), applies the segments up to
that time through SQLite itself, checks the result's page count
against the last commit, and refuses a file that fails SQLite's
`PRAGMA integrity_check`. With `--out` it writes the file locally (mode
600) and stops there. Without it, it puts the database in place at the
target, after asking on the terminal (`--yes` answers without one;
without either nothing is replaced). At the target, every running build
is stopped (as for `--restore-backup`, a draining one included), the
old database is kept beside it as `app.db.before-restore-<time>`, the
file goes in place with mode 600, and the current build is started
again, health checked, and routed (Caddy's upstream moves to its new
port). The command names the kept file:

```text
replaced the database at Ssh deploy@box.example.com:/srv/blog (the previous one is kept beside it as db/app.db.before-restore-20261017T101800Z)
```

If the build refuses the restored file (a database from a newer build,
say), the restore is undone: the file is kept as
`db/app.db.refused-<time>`, the replaced database goes back, and the
build serves again; the refusal names both files. `--from` defaults to
the declaration's `backup`; the credentials come from your own
environment, because the restore runs on your machine.

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
run against the real Fly API`), and until it is finished it **refuses
any deploy that would apply a migration to an existing database** (for
data that outgrows one box, the production answer is the `Turso`
database of an `Ssh` environment, below):

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
(whether Fly's proxy holds the requests meanwhile or answers with an
error is unverified: the target has not run against Fly), and there is
no moment when both builds run. The guarantees are kept the other way
round: when no migration was pending (the only case the target accepts
on an existing database), a build that fails its health check (asked
through the public address, `https://<app>.fly.dev/`, until the page
names the build id) is replaced by the previous image again, and the refusal
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

## The Render target

**Experimental.** The `Render` target has run only against fakes of
Render's API, of a registry and of Turso in alder's own tests; it has
never run against a real Render account. The transcript below is from
those fakes, and the real-account round (a manual test, at the end of
this section) replaces it.

[Render](https://render.com) runs **an image it is handed** as a web
service, on as many instances as you ask for, and rolls a new image in
beside the old one. A service there keeps nothing between deploys, so a
`Render` environment keeps its data in a Turso database (above), and
that is the only shape it has:

```alder fragment
deploy {
  production: Render "oregon" {
    service:   "blog",
    database:  Turso "my-org" { app: "blog" },
    instances: 2,
    plan:      "starter",
  },
  preview: Render "oregon" {
    service:  "blog-preview",
    database: Turso "my-org" { app: "blog-preview" },
  },
  image:    "ghcr.io/tavish/blog",
  requires: ["SMTP_URL"],
}
```

`Render "oregon"` is one of Render's regions (`oregon`, `ohio`,
`virginia`, `frankfurt`, `singapore`). `service` is the name of the
service in your Render workspace; the first deploy creates it.
`database` is required (E0030 says so: "a Render service keeps nothing
between deploys"), and two environments that share an `app` share data,
so the preview has its own. `instances` is how many copies run (default
1), `plan` is passed to Render as given (`starter`, `standard`,
`1c-2g`, …; unset leaves Render's default). `image` is the registry path
every Render environment pushes to, tagged with the build id, without a
tag or a credential in it. Left out, it is `ghcr.io/<owner>/<repo>` from
the `origin` remote of the git repository above the app (a GitHub
remote); with no such remote the command refuses with E0030 and says
what to write. `backup: S3` does not apply to a Render environment,
since Turso's point-in-time recovery is its backup, and `--host` is
refused: the address is the service's.

### What the command reads

Four things on the machine that deploys (a laptop or the CI runner),
never in `App`:

| Variable | Used for |
|---|---|
| `RENDER_API_KEY` | Render's REST API |
| `TURSO_API_TOKEN` | Turso's platform API, the migrations and the lock, as for an `Ssh` environment with a `Turso` database; the running service gets it too, to mint each silo's token |
| `ALDER_REGISTRY_TOKEN` (and `ALDER_REGISTRY_USER`, default the first part of `image`) | the image push, and Render's credential to pull it |
| the `requires` names, and `ALDER_SECRET` | set as the service's environment variables; `ALDER_SECRET` is made once and kept on the service |

The rule of the chapter holds: none of them is in `App`, in the image,
in a record, in `--json` or in an error. The service's environment
variables are **merged**, not replaced (Render's call replaces the whole
set): a variable you set in the dashboard stays, a `requires` name unset
here but already at the service counts as set, and the values are never
printed. The variables and the instance count are set just before the
roll, so a refusal earlier (a missing name, a migration the gate stops)
has changed nothing at Render.

### A transcript

Against the test fakes (`TestDeployRender_EndToEnd`); the real-account
transcript replaces it after the real round. The first deploy creates
the registry credential and the service, so the roll is the service's
own first deploy:

```text
$ alder deploy production --unsafe-skip-tests
alder deploy production → Render oregon (service blog)
gate
  check    ok
  build    ok
  test     SKIPPED  --unsafe-skip-tests: no tests, no mutation score, no checks
artifact build/demo/deploy/a5c229befa496d47 (linux/amd64)
target
  requires ok  SMTP_URL, ALDER_SECRET
plan
  first deploy: the database is created by every migration
  0001_initial
    create table Entry
ship
image pushed ghcr.io/tavish/blog:a5c229befa496d47 (12.0 MB)
  upload   ok  image ghcr.io/tavish/blog:a5c229befa496d47
  backup   ok  no database yet
  migrate  ok  applied 0001_initial
registry credential alder-tavish/blog created (GITHUB)
Render service blog created (http://127.0.0.1:62327/svc/blog)
Render deploy dep-0000000003 live (1 instance)
  start    ok  port 443
health: 1/1 instances serve a5c229befa496d47
  health   ok  build a5c229befa496d47 answers GET /
  switch   ok  current → a5c229befa496d47 (port 443)
deployed a5c229befa496d47 to production
address  http://127.0.0.1:62327/svc/blog
```

After an edit, the second deploy pushes and rolls, and the backup is a
Turso point in time:

```text
ship
image pushed ghcr.io/tavish/blog:e010d2cf9e89376f (12.0 MB)
  upload   ok  image ghcr.io/tavish/blog:e010d2cf9e89376f
  backup   ok  Turso point in time 2026-10-18T00:44:42Z of blog
  migrate  ok  nothing to apply
Render deploy dep-0000000004 live (1 instance)
  start    ok  port 443
health: 1/1 instances serve e010d2cf9e89376f
  health   ok  build e010d2cf9e89376f answers GET /
  switch   ok  current → e010d2cf9e89376f (port 443)
  stop     ok  build a5c229befa496d47
deployed e010d2cf9e89376f to production
```

(`address` is the service's URL as Render reports it, since Render adds
a suffix to a name that is taken; before the service exists, as in a
`--dry-run`, it is `https://<service>.onrender.com (expected)`. The
`--json` record carries it as `address`, for every kind of target.)

### Zero downtime, and what it asks of migrations

Render does the roll. It starts the new instances beside the old, asks
them `/_alder/health` until they are healthy, switches the traffic and
sends the old ones SIGTERM (with 30 seconds to finish). The `switch` and
`stop` lines above are alder's bookkeeping; the engine's health check
then asks the public address until `instances` consecutive answers name
the new build, so every instance has rolled before the build is
recorded as current. `ALDER_DEPLOY_HEALTH_TIMEOUT` (a Go duration, such
as `10m`, the default) bounds both the roll and that check.

`/_alder/health`, which every build serves, answers `{"build":"<id>",
"db":"ok"}`, or 503 when `db` is not `ok`: `unreachable`, or `ahead: …`
for a database migrated past this build. A build never migrates a
remote database when it starts (several instances would race). Behind
the database, it exits with "the database is behind this build: run
`alder deploy`", which fails Render's deploy while the old instances
keep serving; ahead of it, it serves the maintenance page.

What that asks of migrations: the migrations run **first**, from the
deploying machine, and the old build keeps serving **on the new schema**
until the roll completes. That is why the gate's DROPS DATA rule
(above, "Migrations and the data-loss rule") is the guard, and why it is
not optional here: a migration that renames or drops a column the old
build still reads makes the old instances fail for as long as the roll
takes. Add the new column in one deploy, and drop the old one in the
next.

### Instances, rollback and restore

`instances: 2` makes Render run two copies, and the health check waits
for both; a changed count is set just before the roll. Because the store
is Turso, not a file, any number of instances is allowed; this is the
one target with that field.

State lives where the data does: the builds, their records, the pointer
to the current build and the lock are rows of the shared Turso database
(`_alder_deploy_builds`, `_alder_deploy_current` and `_alder_lock`), so a
laptop and a CI runner agree, and a runner that never had the old
artifact can still plan and roll back from the stored schema. Nothing is
recorded as current until Render reports the deploy `live` and the
health check has seen the build.

`alder status` adds the service's URL, its instances, the live deploy and
its age. `alder rollback <env> [id]` deploys the older image by its tag
and polls, under the same schema rule as any rollback; `--restore-backup`
copies the database as it was at the time recorded for the next build,
and sets the new `ALDER_DB` in one call before the roll. `alder restore
<env> --to <time>` (with `--silo` for one org) is the Turso restore
above, with that call in place of `turso.env`. All of them are rolls, so
none takes the site down. `alder logs <env>` prints the service's last
lines, `[instance] message`; with `-f` it streams, over Render's log
websocket (a small RFC 6455 client in alder, no dependency), until you
stop it.

### The registry, and the one token

The artifact becomes a one-layer OCI image on `scratch`: the executable,
`static/`, `migrations/` and `alder-deploy.json`, and nothing else, no
shell and no package. alder builds and pushes it itself, with the
registry's HTTP API and no Docker, so the same artifact always gives the
same image digest. Render runs `linux/amd64`, and the artifact is
cross-built for it. Two things a bare image lacks are supplied: a CA
bundle (Mozilla's, vendored in alder) at `/etc/ssl/certs/ca-certificates.crt`
for the adapters' TLS, and the time zone database, compiled into the
executable of a deploy build. (alder takes no module beyond the
standard library and the SQLite driver, so the image carries the bundle
rather than a Go package of roots.)

One token does two jobs: `ALDER_REGISTRY_TOKEN` pushes the image and is
also what Render stores as its **registry credential**, found or made
by name on each deploy (`alder-<image path>`; the registry kind comes
from the host: `ghcr.io`, Docker Hub, GitLab, Google Artifact Registry
and ECR are known). Render pulls with it for as long as the service
lives, so it must be a long-lived personal access token (for GHCR, a
classic token with `read:packages` and `write:packages`), not a
per-run one. With no token set the push is anonymous and no credential
is made: that works only for a public image. Images stay in the
registry (pruning removes only the records `keep` counts).

### CI

`alder ci init` writes the same three jobs for a Render environment,
with no SSH step. The header comment lists the repository secrets
(`RENDER_API_KEY`, `TURSO_API_TOKEN`, `ALDER_REGISTRY_TOKEN` and the
`requires` names), the job has `permissions: packages: write`, and
`ALDER_REGISTRY_TOKEN` falls back to `github.token`. The run token can
push a GHCR image under the repository's owner but cannot stand in as
Render's credential, since it dies with the run: set a personal access
token once as the `ALDER_REGISTRY_TOKEN` secret, or keep the image
public. The pull request's preview runs `alder deploy preview --json`
and comments the `address` from it (no `--host`), and every pull request
deploys to the one preview service, so the last push wins. The first
deploy creates the Render service; the dashboard is for looking.

### Why no disk mode

A Render persistent disk would put SQLite in a file, but Render stops
the instance that holds a disk before the new one starts, so no shape of
it is zero-downtime. A stateless service on a remote database is the
shape a platform that deploys images can roll without a pause.

### The manual test

The unit and end-to-end tests run against fakes. The real round is
manual, because it needs three accounts and costs a little:

```text
ALDER_RENDER_REAL=1 RENDER_API_KEY=… TURSO_API_TOKEN=… ALDER_TURSO_ORG=acme \
  ALDER_RENDER_IMAGE=ghcr.io/you/alder-m16 ALDER_REGISTRY_TOKEN=… \
  go test -tags render ./cmd/alder -run TestRenderReal -v -timeout 60m
```

It deploys the scaffold twice to a service and a Turso database named
`alder-m16-<random>`, reads `status`, rolls back, reads the logs, and
deletes nothing: it prints the commands that remove what it made.

## The Script hatch

`Script "./deploy.sh"` is for everything alder does not ship to itself:
a platform with its own CLI, a Kubernetes cluster, a box behind a
bastion. The gate and the artifact are the same as for any target;
then the script runs from the app's root with two arguments, the
artifact directory (absolute) and the environment's name, and the
`requires` names, comma-separated, in `ALDER_REQUIRES` (`ALDER_SECRET`
included; the host in `ALDER_HOST` when `--host` was given;
`ALDER_ALLOW_DATA_LOSS=1` only with `--allow-data-loss`, never
otherwise, even when your shell had it set). Its exit code is the deploy's
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
not available for a `Script` target. Its plan lists every migration (it
cannot see which the target applied), and the plan itself refuses none;
applying one is another matter: the app's `--migrate-only` refuses a
DROPS DATA migration on a database with data unless
`ALDER_ALLOW_DATA_LOSS=1`, which the script gets only with
`--allow-data-loss`.

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
| Zero downtime on one box | Two processes can open the file: the new build migrates and starts while the old one drains, which is how the `Ssh` target switches behind Caddy without dropping requests (`proxy: None` has a moment without a listener; Fly a pause of seconds). |

When the app outgrows one box's disk (many tenants, each with a
database of its own, or more data than one machine should hold), the
answer is a database built for it: SQLite's own engine run as a
service, reached over the network. That is the `Turso` database below.

## Silos at a box

An app with `silo org { Project }` (chapter 8) keeps each org's projects
in a database of its own. At an `Ssh` box with the ordinary file
database, each silo is a file next to the shared one:

```text
<path>/db/app.db                      the shared database
<path>/db/silos/org/<name>.db         one org's silo
```

`<name>` is the org's id written in a file-safe alphabet (lowercase
base32hex): ids are case-sensitive, and some file systems are not, so
the id's own text is never the file name. A silo file is created the
first time a request needs it, and checked every time a process opens
it: the `_alder_schema` row inside it says which build last migrated it
(chapter 13 explains the check). Four commands look after the silos of
a deployed app, each running the current build's executable at the box
(the same executable that serves, with a maintenance flag):

| Command | What it does |
|---|---|
| `alder status <env>` | adds the silo table: per key, how many silos are `ok`, `migrating`, `pending (data loss)`, `ahead`, `failed` and `orphaned`, then each one that is not `ok` with the reason (the build's `--silo-status`) |
| `alder migrate silos <env> [--allow-data-loss]` | migrates every silo file now (`--migrate-silos`); a silo behind by a migration that drops data needs `--allow-data-loss`, exactly as the shared database does at deploy |
| `alder silos prune <env> [--yes]` | lists the orphaned silos (their org's row was deleted, so nothing can reach them), asks, and removes their files (`--prune-silos`, which checks each again first) |
| `alder restore <env> --silo org:<id> --to <time>` | rebuilds that one silo from its own backup stream and puts it in place; the file it replaces is kept beside it (`….before-restore-<time>`), and the build is restarted so no process keeps what it read |

`--allow-data-loss` on `alder deploy` covers the shared database only.
So a deploy whose plan drops data, in an app with a silo, says where the
rest goes before it asks for the flag: "hint: the app has silos:
--allow-data-loss covers the shared database; a silo whose tables a
pending migration drops data of waits in maintenance when it is opened,
until `alder migrate silos production --allow-data-loss` (take a backup
first; `alder status production` lists them)". A typical session after
a deploy whose migration dropped a column of a siloed table:

```text
$ alder status production
…
silos    org: 41 ok, 0 migrating, 2 pending (data loss), 0 ahead, 0 failed
           org/xK3…  pending-data-loss: a pending migration DROPS DATA (0002_drop_note: column Project.note): take a backup and run `alder migrate silos <env> --allow-data-loss`
           org/9pQ…  pending-data-loss: …
$ alder migrate silos production --allow-data-loss
org/xK3…: migrated 0002_drop_note
org/9pQ…: migrated 0002_drop_note
```

The two orgs saw the app's maintenance page in between; every other
org was served. The running app notices a silo that became servable on
its next request after its recheck interval; `alder status` (a new
process) sees it at once.

With `backup:` (or `ALDER_BACKUP_URL` in the environment file) the
runtime streams every silo file as it streams the shared database, each
under its own prefix of the bucket (`silos/org/<name>/…`), so one silo
can be restored without touching the others. Each stream is a shipper
with a lock file and two connections, so there is a limit: **200 silo
files**. Past it, the runtime ships none of the silos (the log says so,
once) and `alder deploy` refuses with a sentence pointing here. Two
hundred tenants in files on one disk is where the `Turso` database
starts to pay for itself.

## The Turso database

[Turso](https://turso.tech) runs SQLite's engine (libSQL) as a
service: each database lives there, is reached over HTTPS, and is
backed up continuously with *point-in-time recovery*: you can ask for a
copy of any database as it was at any second of the last days. A
Turso organization can hold thousands of small databases, so a silo
there is what it is at a box, a database per org, only no longer a file
on your disk. (Turso also has *schema databases*, whose changes it
passes on to many databases at once; it marks them deprecated for new
organizations, and alder does not use them. Each silo database is
migrated the way a silo file is.)

An environment keeps its box (the executable still runs there, behind
Caddy) and moves its data:

```alder fragment
deploy {
  production: Ssh "deploy@box.example.com" {
    domain: "app.example.com",
    path: "/srv/app",
    database: Turso "acme" { group: "default", app: "workspace", sample: 5 },
  },
}
```

`Turso "acme"` is the organization; `group` the Turso group the
databases are created in (default `"default"`); `app` the name the
databases get; `sample` how many of the largest silos a deploy tries
its migrations on first (default 5). The names Turso ends up holding:

| Database | What it is |
|---|---|
| `workspace` | the shared database (`ALDER_DB=libsql://…` for the app) |
| `workspace-org-<id>` | one org's silo, its id written as the silo file's name is (so it must fit Turso's 64 characters: a deploy refuses a name that would not, with a sentence) |

The token: create one with `turso auth api-tokens mint alder` and put
it in two places. On the machine that deploys, as `TURSO_API_TOKEN` in
the environment (the deploy creates databases and runs migrations from
there). At the box, as `TURSO_API_TOKEN=…` in the environment file (the
running app creates each org's database the first time that org is
used, and asks Turso for a token per database, which it keeps in
memory). `alder deploy` checks both before anything changes.

### What a deploy does with it

The gate, the artifact and the plan are as for any `Ssh` environment.
Then, before anything is uploaded, the preflight checks the token here,
that the group exists, that every database name fits Turso's rules (at
most 64 characters: keep `app` short), and creates the shared database
if this is the first deploy, writing its address to
`<path>/db/turso.env` at the box (the build's run script reads it).
After the upload:

1. **The backup is a point in time.** Nothing is copied: the deploy
   records the next whole second in `builds/<id>/backup.pitr` and waits
   for it to pass, so every write before the deploy is at or before that
   time and every change the migrations make is after it.
2. **The plan is tried on the largest silos.** The deploy runs the new
   build's executable *on this machine* (built for it when the box is
   another platform) with `--verify-silos 5`: it lists the silo
   databases through Turso's API, asks for each one's size (when Turso
   gives none, it takes the first five by name), and on each of the
   five largest begins a transaction, runs the pending migrations'
   statements, checks foreign keys and that the tables come out as the
   new build's (a database changed by hand fails here), and rolls
   everything back. One failure refuses the deploy, naming the database,
   and nothing has been migrated: the shared database is as it was, the
   old build serves.
3. **The shared database is migrated** with `--migrate-only`, from
   here, over HTTPS: the same migrations, the same DROPS DATA rule, as
   for a file.
4. Start, health check, switch, as always.

The silo databases are not migrated by the deploy: each is migrated
when the new build first opens it, exactly as a silo file is (chapter
13): behind by migrations that drop no data, it is migrated there and
then and served; behind by one that drops data, it answers the org's
maintenance page and `alder status` lists it `pending (data loss)`;
until something opens it, `alder status` lists it `migrating`. To
migrate them all at once instead of one by one as orgs come back, run
`alder migrate silos <env>` (with `--allow-data-loss` for a migration
that drops data): the build's `--migrate-silos` lists the silo
databases through Turso's API and migrates each, a line per database.

### Restoring, rolling back, locking

| Command | On a Turso database |
|---|---|
| `alder restore <env> --to <time>` | asks Turso for a copy of the shared database as it was at that time, makes it the app's (`db/turso.env` names it), and restarts the build on it; the replaced database stays at Turso |
| `alder restore <env> --silo org:<id> --to <time>` | puts one org's database back as it was: Turso cannot restore a database in place, so the deploy copies it at that time, keeps a copy of it as it is now, and creates the org's database again from the first copy |
| `alder rollback <env> <id> --restore-backup` | the same copy of the shared database, at the time the next deploy recorded in `backup.pitr` |
| `alder silos prune <env>` | deletes the orphaned orgs' databases at Turso |

`--from` and `--out` do not apply: there is no stream and no file. A
rollback puts back the shared database only; the silos stay at the
newer schema, so an older build answers their maintenance page
(`ahead`) until you deploy forward again.

Two deploys of the same environment cannot overlap: besides the box's
lock, the deploy holds a row of the shared database (`_alder_lock`,
with its holder and an expiry two hours later, so a killed run does not
block the next one forever).

### What alder speaks to Turso

No Turso library: the runtime and the deploy use Go's `net/http`, and
exactly these requests. Of the platform API (`api.turso.tech`): read a
group; list the group's databases (the silos are those whose names
begin `<app>-<key>-`); read, create (empty, or seeded from another
database at a time) and delete a database; read a database's storage
size; mint a database's token. Of
the database protocol (Hrana over HTTP, `POST /v2/pipeline`): `execute`
with positional arguments and `close`, on a stream carried from request
to request by its *baton*; a transaction is `BEGIN IMMEDIATE` …
`COMMIT` on one stream, so the Task's own code runs between its
statements as it does on a file. alder's tests run all of it against a
fake Turso (`internal/tursofake`). The fake is not Turso, so before the
first production deploy run the same calls against the real service:

```text
$ TURSO_API_TOKEN=… ALDER_TURSO_ORG=acme go test -tags turso ./rt/db -run TestTursoReal -v
```

It creates and deletes databases named `alder-t-<random>`, and checks
what the fake stands in for: that a database is created and listed as
alder names it, that `BEGIN IMMEDIATE`, savepoints and DDL inside a
transaction work over the protocol, that SQLite's refusal texts (`UNIQUE
constraint failed`) come through, that the usage call gives
`storage_bytes`, and that a copy seeded at a whole second holds what was
committed before it.

## The knobs

A few environment variables change how the commands behave, for tests
and for unusual setups:

| Variable | Effect |
|---|---|
| `ALDER_SSH_CONFIG` | an `ssh_config` file every `ssh` reads (`ssh -F`): an identity, a jump host, a known-hosts file |
| `ALDER_DEPLOY_DRAIN` | how long the old build drains after the switch (default `30s`; `0` stops it at once) |
| `ALDER_DEPLOY_HEALTH_TIMEOUT` | how long a new build has to pass its health check (default `60s`; `10m` for a Render roll; a Go duration) |
| `ALDER_DEPLOY_SUPERVISOR` | `process` runs builds as background processes even where systemd runs |
| `ALDER_DEPLOY_CADDY_ADMIN` | the `host:port` of Caddy's admin API at the box (default `127.0.0.1:2019`) |
| `ALDER_BACKUP_URL` | at the box (the environment file): stream the database to a directory or bucket without a `backup` in the declaration |
| `ALDER_BACKUP_INTERVAL` | how often the runtime ships the database (default `10s`) |
| `ALDER_ALLOW_DATA_LOSS` | read by the app's executable, never set by you: `1` lets `--migrate-only` apply a DROPS DATA migration; `alder deploy` sets it for one run with `--allow-data-loss` |
| `RENDER_API_KEY` | the Render target's API key (the Render REST API) |
| `ALDER_REGISTRY_TOKEN`, `ALDER_REGISTRY_USER` | the Render target's registry token (the image push and Render's pull credential) and user (default the first part of `image`) |
| `ALDER_RENDER_API` | Render's API address (default `https://api.render.com/v1`; alder's tests point it at the fake) |
| `FLY_API_TOKEN`, `FLY_ORG` | the Fly target's token, and the organization a new Fly app is created in |
| `TURSO_API_TOKEN` | here and at the box: the Turso platform API's token (a `database: Turso` environment) |
| `ALDER_TURSO_API` | the platform API's address (default `https://api.turso.tech`; alder's tests point it at the fake, whose databases are then reached over plain HTTP) |
| `ALDER_TURSO_ORG`, `ALDER_TURSO_GROUP`, `ALDER_TURSO_APP`, `ALDER_TURSO_DB` | read by the app's executable, set by the run script from the declaration and `db/turso.env`: where its databases are |
| `ALDER_DB_TOKEN` | read by the app's executable: a token for `ALDER_DB=libsql://…` itself, instead of one minted with `TURSO_API_TOKEN` (an app run by hand against a Turso database) |

And for alder's own tests of the targets: `ALDER_DEPLOY_LOCAL=1` runs
the `Ssh` tests against a private `sshd` on this machine;
`ALDER_DEPLOY_REQUIRE_DOCKER=1` makes `-tags deploy` fail instead of
skipping when Docker does not answer (CI's `deploy` job);
`ALDER_DEPLOY_SYSTEMD=1` runs the systemd and Caddy test on a box that
has them (CI's `deploy-systemd` job); `ALDER_SILO_RECHECK` (a duration, read
by the app's executable) shortens the minute after which an app opens
again a silo it found not servable.

## What is not there yet

The `Fly` target is experimental: it refuses migrating deploys and has
no lock and no `--restore-backup`. The `Render` target is experimental too:
it has run only against fakes until the manual `-tags render` round. A preview is not
removed when its pull request closes. Wildcard certificates need a DNS
challenge alder does not configure. A build run as a plain process
(no systemd) is not restarted when it crashes. Only GitHub Actions has
a workflow. A Turso database's rollback puts back the shared
database but not the silos, and the run against the real service is
manual (`-tags turso`). The file database's silos
stream at most 200 files.
