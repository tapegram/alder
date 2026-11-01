# alder-tui: a terminal UI for the humans around an Alder app

Design for [tapegram/alder#120](https://github.com/tapegram/alder/issues/120). The issue settled
the architecture (a separate program built with Charm, driving `alder` only through machine
interfaces, the CLI staying primary, the non-editor parts first). The brainstorm (2026-11-01, on
Fable) settled the scope: **local development first** (running the app, watching and searching
its logs, reviewing its resources), **then the remote side** (deploys, and monitoring and
inspecting production). Supervising an agent session is out. The user asked for the design to
be made with minimal input, so most of this document is the author's; everything marked
**(call)** is the author's decision and the rest is the user's or the issue's.

The survey this design rests on is summarised in Appendix A: what `alder` exposes today, what
it does not, and where each seam is.

## Intent

A person building an Alder app today runs `alder run` in a tmux window and barely looks at it:
the output is a wall of text, a 500 is a line somewhere in it, a slow page is a feeling, and the
database is a file they open with something else. `alder-tui` is the window they *want* to
look at. Opened in the app's directory it starts (or joins) the dev server and shows the app
the way a person thinks about it: requests as they arrive, each one openable into its effect
steps, its SQL and the log lines it wrote; logs that can be followed, paused and searched;
the tables and their rows; the tests and their failures, re-running on save; the routes and
which of them are hit. Later it turns the same screens on an environment: what each one runs,
its logs, a deploy as a checklist you watch, a rollback one key away.

Agents are Alder's main users and cannot use a TUI, so nothing is *only* in the TUI. Every
screen is backed by a `--json` command or a `/_dev/*` endpoint, and the footer of every screen
names the command that gives the same data. The TUI teaches the CLI; it never replaces it.

## Goals

1. `alder-tui`, run in an app directory, owns or attaches to `alder run` and shows **build
   state, requests, logs, data, tests and routes** for the running app, live.
2. A request opens into **its effect steps, SQL statements (with N+1 repeats marked) and the
   log lines written during it**; a log line jumps to its request and back.
3. Logs **follow, pause, filter by level, request id and regex**, over a scrollback the terminal
   would have lost.
4. Tests **re-run on save**, failures show their left/right/note/trace, a failing property shows
   its shrunk case, and a mutation run shows the score and the surviving mutants.
5. The **data browser** lists tables with row counts and schema, pages through rows, and takes a
   read-only `select`.
6. **Remote:** an environments board, each environment's builds and logs, a **deploy** and a
   **rollback** driven from the TUI as step checklists with a post-deploy watch.
7. Every piece of data the TUI shows is reachable as **plain text or `--json` from `alder`**;
   whatever is missing is added to `alder` first, as a machine interface agents can use too.
8. The TUI is **tested with golden frames** (Charm's `teatest`) against fake feeds, and the
   `alder` additions with ordinary Go tests.

## Non-goals

- Editing code, generators, `fill`, scratch files, the configurator: nvim and the browser own
  these (`alder.nvim`, `/_dev/theme`).
- Supervising an agent session (watching Claude edit the app). Dropped by the user.
- Replacing or restyling `alder`'s own CLI output (#87 did that; the TUI matches its look).
- Request drill-down on a *deployed* app, restore/PITR, silo pruning, a config checker, and
  remote data browsing: designed for later in "Remote: later", not built in this round.
- Windows **(call)**: the TUI is built and tested on macOS and Linux; Windows may work but is
  not a target.

## Anti-goals

- Charm, or any new dependency, in `alder`'s `go.mod`. The import rule (stdlib + `alder/*` +
  modernc) stands; `TestDistModule` keeps it honest.
- The TUI **parsing human-readable output** (`alder run`'s stderr, `alder status` text, gate
  rows). Every feed is JSON or a documented file. If the TUI needs something only text gives,
  the fix is in `alder`.
- A TUI-only feature. If a screen shows it, `alder` can print it.
- Touching the running app's behaviour: the dev endpoints and the request ring exist only under
  `ALDER_DEV=1`, cost nothing in production and never change what a request does.

## Constraints

- `alder`'s module path is `alder` and its `internal/` packages (`style`, `diag`, `watch`,
  `deploy`) are not importable from another module. The TUI copies the 16-colour palette and the
  status marks; it does not share code with `alder`.
- The app's logs are **stderr** text by default, JSON with `ALDER_LOG_FORMAT=json`, and nothing
  else keeps them. The dev server is the only process that sees every line.
- Deploy history lives **on the remote** (`builds/<id>/deployed.json`); there is no local record.
- CI runs `./...`, which stops at module boundaries; a nested module needs its own job.

## Shape

### One program, nested module, git-style dispatch (call)

- The TUI lives in the alder repo at `tui/` as its own Go module (`module alder/tui`), with the
  binary at `tui/cmd/alder-tui`. Nested modules are already how `examples/*` and
  `experiments/*` live. One repo lets a PR add a machine interface to `alder` and its consumer
  to the TUI together, and `release.yml` ships both binaries from one tag.
- Dependencies of the TUI module: Bubble Tea, Lip Gloss, Bubbles, `teatest` (test only),
  `modernc.org/sqlite` (read-only data browsing), and a file watcher (fsnotify, or the same
  300 ms poll `alder` uses; builder's choice). Nothing else without a reason in the PR.
- `alder tui [args]` is added to `alder`: it looks for `alder-tui` on `PATH` (and `ALDER_TUI`
  to override), `exec`s it with the arguments and environment, and otherwise prints one line
  saying how to install it. The TUI finds `alder` the same way (`PATH`, `ALDER_BIN`). On start
  the TUI runs `alder help --json`, reads `version`, and shows a one-line warning in the status
  bar when its own version differs; it still runs.
- **Shipping (call):** `alder-tui` joins the existing Homebrew formula (`tapegram/alder`), so
  `brew install`/`upgrade` gives both binaries and `alder tui` works at once; the GitHub
  release carries it as its own archive per target for people who install from releases; it is
  **not** added to the `ghcr.io` image (that image runs apps, not people). The dispatch
  message is `alder tui: alder-tui is not installed; brew upgrade alder, or download it from
  the release that matches alder <version>`. The module path `alder/tui` is not
  `go install`able from GitHub, which is already true of `alder` itself.
- A new CI job `tui` (gated by the paths filter on `tui/**`) runs `gofmt -l`, `go vet ./...`
  and `go test ./...` inside `tui/`. `release.yml` builds `alder-tui` next to `alder` for the
  same targets.

### The feed: `/_dev/events` on the dev server

`alder run` becomes a source of structured events as well as text. Nothing about its terminal
output changes for a person running it by hand.

- **The child always logs JSON to the dev server (call).** `alder run` sets
  `ALDER_LOG_FORMAT=json` on the child, parses each stderr line as a JSON log record, and
  **renders it as the text line it prints today** (dim stamp, coloured level) unless the user's
  own environment set `ALDER_LOG_FORMAT=json`, in which case the JSON passes through. Lines
  that are not JSON (a panic trace, a stray print) pass through as they are. One place knows
  the format; the terminal looks the same; every line is available as data.
- **`GET /_dev/events`**, served by the dev server itself (next to `/_dev/reload`, never
  proxied), is an SSE stream of JSON events. Each event has `type`, `t` (RFC3339Nano) and
  `seq` (a counter since the dev server started). Types:
  - `hello` — the first event: `{app, dir, alder (version), addr, child (addr or null), gen,
    build (state), since}`. The TUI checks `dir` against its own app directory.
  - `build` — `{state: building|ok|failed, gen, diagnostics?: [jsonDiag], warnings?: [jsonDiag],
    ms?}`. `jsonDiag` is `alder check --json`'s shape. `ok` also carries `child` (the new app
    address).
  - `app` — `{state: started|exited, child?, exit?: {code, signal?}, tail?: [string]}` (the last
    40 stderr lines on exit, as today).
  - `static` — `{}`: static files re-copied, pages reloaded.
  - `log` — the app's JSON log record as emitted, under `record`, plus `raw` for a line that was
    not JSON. Access records are recognised by their `rid` + `status` fields; the TUI treats
    them as both a log line and a request.
  - `run` — `{text}`: the dev server's own status lines (`building`, `rebuilt`, the app exited
    …), so a client can show them without reading stderr.
- The dev server keeps a **ring of the last 5 000 events (call)** and replays it on connect;
  `?after=<seq>` resumes a dropped connection without the backlog. `: ping` every 15 s as
  `/_dev/reload` does. Same-origin is not required: the stream is read-only and bound to the
  dev address, like `/_dev/reload`.
- **Request events are the app's access records**, not proxy instrumentation (call): the
  access line already carries the masked path, status, duration, bytes, `rid`, and at Debug the
  step count; the proxy would see unmasked URLs and know nothing about steps.
- **The child always runs at `ALDER_LOG=debug` under the dev server (call).** The feed carries
  everything; the dev server filters what it prints to the terminal to the user's `ALDER_LOG`
  (default info), so a hand-started `alder run` looks as it does today and a TUI attaching to it
  still gets step counts and `effect <name>` lines. The Logs screen filters to info+ by
  default. (The alternative, a poorer attach mode, would make the first screen lie about the
  app depending on who started the server.)
- **The debug access record gains `db_stmts`, `db_ms` and `repeats` (call):** today the
  statement count and time appear only on slow lines, and nothing carries the N+1 repeat-group
  count the dev panel computes. At Debug the record carries all three, so the Requests row can
  show `n+1` and the Data screen can tell a request wrote without fetching every request's
  detail. The info-level record is unchanged.
- `alder run --json` (the same events as NDJSON on stdout, for agents and CI) is a natural
  follow-on and is not required by this design.

### Request detail: `/_dev/requests` on the app

The app already builds an `eff.Recorder` per request in dev for the HTML dev panel. This design
keeps the record and serves it.

- `rt/web`, under `ALDER_DEV=1` (the panel's condition), keeps a **ring of the last 200 request
  records (call)**: the `applog.Access` fields, the Recorder's `Steps` and `SQL` (with the N+1
  repeat groups the panel computes), and **the log lines written during the request**: the
  request's `Env.Log` calls are captured on the record as well as logged.
- `GET /_dev/requests` returns the ring's summaries (newest first) and
  `GET /_dev/requests/{rid}` one full record:
  `{rid, t, method, path, route, status, ms, bytes, steps: [{name, ms, cap}], sql: [{stmt, args,
  ms, rows, group, repeats?}], loads, cuts, logs: [{t, level, msg}], slow, warnings: [text]}`.
  404 when the rid has left the ring or the app restarted; the TUI then shows the summary it
  has with a note. Both are proxied by the dev server like everything else under the app.
- **`rid` on log records (call):** JSON log records written during a request carry `rid`
  (text lines are unchanged). This is #118's option A in its smallest form and is what lets
  the Logs screen group lines by request; the HTML dev panel can later render from the same
  record, which is left to #118.

### The map: `/_dev/map.json`

The app's `/_dev` page is rendered from a JSON map the generator already produces
(`Options.DevMap`: build, pages, tables, session fields, capabilities, applied migrations,
routes). `GET /_dev/map.json` serves that map as JSON under the same dev-only condition. It is
the Data screen's source for tables and columns and the Routes screen's fallback when
`alder routes --json` is slower than a rebuild.

**The map gains what the Data screen needs (call):** for each table its generated **SQL
name** and each column's **SQL name and type** next to the Alder names (today the map has
`Module.Name` and `field: Type` only, and the SQLite names are generated and change on
rename), and a **`silos`** list (key, and the database file or path rule each silo uses, as
`rt/db` knows it). The generator knows all of this; the TUI must not reverse-engineer
`sqlite_master`. The footer's `sqlite3` command and the `:` prompt use the SQL names; the
screen shows the Alder names.

### Local mode

`alder-tui` with no arguments, run in an app directory (or `alder-tui [dir]`):

1. It resolves the dev address (`--addr`, else `ALDER_ADDR`, else `localhost:8080`) and tries
   `GET /_dev/events`. If a dev server answers and its `hello.dir` is this app, the TUI
   **attaches**; quitting the TUI leaves the server running. If `hello.dir` is another app,
   the TUI says so and stops, suggesting `--addr`.
2. Otherwise the TUI **starts `alder run`** as a child in the app directory with the
   environment above (`ALDER_LOG=debug` default), captures its stderr into the Logs screen
   as `run` lines until `/_dev/events` answers, then connects. Quitting the TUI stops the
   server (SIGTERM, then SIGKILL after 5 s). `--no-run` refuses to start one.
3. The status bar shows the app, the build state, the dev address and the alder version;
   `R` restarts the child (`alder run` already rebuilds on save; `R` is for a wedged app and
   is a `POST /_dev/restart` the dev server gains for it (call)); `o` opens the app in the
   browser.

#### Screens

Five tabs, `1`–`5` or `tab`/`shift-tab`. All lists move with `j`/`k` and the arrows, `enter`
opens, `esc` goes back, `/` filters the current list, `?` shows the keys, `q` quits. The footer
shows the keys for the screen and, right-aligned, the `alder` command that prints the same
data.

**Requests** (default tab) — the live table, newest at the bottom, following until you scroll
up (`G` resumes). A strip above it shows the last minute: requests/s, p50 and p95, error %,
computed from the events (no metrics scrape needed). Rows: time, method, path, status (coloured
by class), ms (yellow when `slow`), bytes, steps, and flags (`slow`, `n+1`). `enter` opens the
request; `l` jumps to Logs filtered to its `rid`; `/` filters by any column text.

```
 shelf  ● ok  localhost:8080  alder 0.5.0                         1 Requests 2 Logs 3 Data 4 Tests 5 Routes
 last 60s  12.4 req/s  p50 8ms  p95 41ms  errors 0.0%
 ─────────────────────────────────────────────────────────────────────────────────────────────
  14:02:11.204  GET   /                          200    6.1ms   4.2K   3
  14:02:12.880  GET   /books/42                  200   38.9ms   9.1K  12  n+1
  14:02:13.015  POST  /books/42/lend             303    9.7ms      0   5
  14:02:13.101  GET   /books/42                  200   12.2ms   9.1K   7
  14:02:19.440  GET   /search?q=••••             500  212.4ms   1.0K   4  slow
 ─────────────────────────────────────────────────────────────────────────────────────────────
 enter open  l logs  / filter  G follow  o browser  R restart  ? keys        alder run · ALDER_LOG=debug
```

**Request** (detail) — the header fields, then three sections you move between with `tab`:
Steps (name, ms, cap, as a list in order), SQL (stmt, args, ms, rows; repeat groups folded with
a count and marked `n+1`; `enter` unfolds), Logs (the lines written during the request). `y`
copies the `rid`. The footer names `curl localhost:8080/_dev/requests/<rid>`.

```
 ← GET /books/42  200  38.9ms  rid 5f3a2c1e  route /books/{id}  steps 12  db 11/31.2ms
 Steps                        SQL (11, 1 repeat group)                 Logs (2)
  load            0.4ms  db    ▸ select * from books where id=?  ×1   0.9ms  1 row
  Books.find      0.9ms  db    ▾ select * from loans where book=? ×9  n+1  28.1ms  9 rows
  Loans.for       28.1ms db        args [42] [42] [42] …
  render          7.2ms         select count(*) from reviews …   ×1   2.2ms  1 row
```

**Logs** — the scrollback of `log` and `run` lines (20 000 lines kept in the TUI (call)),
following until you scroll up. Filters stack and show in the strip: level (`L` cycles debug /
info / warn / error; default info), `rid` (`enter` on an access line, or `l` from Requests),
and `/` regex over the rendered text. `enter` on any line with a `rid` opens that request.
Lines render as `alder run` renders them (dim stamp, coloured level), with `effect` lines dim.
Footer: `alder run` with the matching `ALDER_LOG` / `ALDER_LOG_FORMAT`.

**Data** — left pane: the tables from `/_dev/map.json` with a live row count (one
`select count(*)` each, refreshed when a request event has written; the rule is below), then
the applied migrations. Right pane: the selected
table's rows, 100 per page, `s` sorts by the column under the cursor, `enter` shows one row
vertically (long text unfolded). `:` opens a one-line read-only SQL prompt: the TUI opens
`build/<app>/dev.db` with `mode=ro` and `query_only=1`, accepts a statement starting with
`select`/`with`/`explain`, and refuses anything else with the reason (call). If the map reports
silos, a selector above the table list picks one; how a silo maps to a database file follows
`rt/db` and is the builder's to read. Footer: `sqlite3 build/<app>/dev.db`, and the SQL when a
prompt is active. The "wrote" trigger for the row counts is any access record whose
`db_stmts` is above zero and whose method is not `GET`/`HEAD`, or any `POST`.

**Tests** — on first open and on `r` the TUI runs `alder test --json` and streams the results
into a list (module, name, ms, `ok`/`FAIL` with the marks `alder` uses); a summary line counts
passed/failed and totals the time. `enter` on a failure unfolds left/right/note/trace, and a
property's `{runs, seed, shrinks, case}`; `enter` again reruns only that test (`-run ^name$`).
`w` toggles **watch**: on a change under `app/`, `adapters/` or `migrations/` (300 ms debounce)
the suite reruns; a run in flight is finished, not killed. `m` runs `--mutate --json` and shows
the score, killed/survived/suppressed/skipped, and the surviving mutants (file:line, before →
after). A failing property whose saved case exists under `build/<app>/fuzz/` shows
`replays saved case`. Footer: the exact command the last run used.

**Routes** — `alder routes --json`, refreshed on every `build ok`: kind, path, module.name,
file:line, and **hits** this session (request events matched to the route by path pattern).
`o` opens a parameter-less page in the browser; `enter` jumps to Requests filtered to that
route. Footer: `alder routes --json`.

#### Build state and problems

The status bar's dot is the build state (`building` yellow, `ok` green, `failed` red with the
error count). `p` on any screen opens the **Problems** overlay: the diagnostics from the last
`build` event in `alder check`'s layout (`file:line:col: error[E0001]: message`, fix hinted),
`enter` copies `file:line:col`. When the app has exited the overlay shows the exit and the tail.

### Remote mode

`alder-tui envs` (or `6` / `e` from local mode when the app declares environments) opens the
environments board. It needs three additions to `alder`:

- **`alder status --json`** (call): with an environment, one document
  `{env, target: {kind, describe, address?}, current, builds: [{id, when, by, score?, current,
  running, modified, note}], database: {bytes}?, backup: {when, from, streamTo, shipped}?,
  info: [string], silos: [SiloStatus]}` — the same facts `Status` prints, from the same calls.
  **With no environment**, `{envs: [{name, kind, hosts?}]}` from the app's `deploy`
  declaration, which today is a usage error. `address` is the public URL when the target knows
  it (Render, Fly), else absent.
  **Hosts (call):** an environment with several hosts (`--host`; an Ssh target keeps each as
  `<env>-<host>`) is one entry in `envs` with its declared `hosts`; with an environment given,
  `status --json <env>` adds `hosts: [{name, current, when, by}]` for the hosts the target can
  see (declared ones, plus the live ones an Ssh target finds for a wildcard domain), and
  `--host name` narrows it to the full document for that host, as the text form does. The
  TUI shows one board row per environment that unfolds into its hosts; Deploy, Rollback and
  Logs run against the host under the cursor and pass `--host`, or against the whole
  environment when it has no hosts.
- **`--progress json`** on `alder deploy` and `alder rollback` (call): the progress lines that
  go to stderr become NDJSON there — `{"heading": "gate"}`, `{"step": "test", "status": "ok",
  "note": "42 passed"}`, `{"warning": "…"}`, `{"text": "…"}` for anything else — written at
  the single seam `line()` and the heading writer. `--json` alone keeps today's text progress
  so CI logs stay readable.
- **`alder rollback --json`**: the final record on stdout as `deploy --json` prints it, and
  `{"error": …}` on refusal, so the TUI can tell a refusal from a failure.

Remote logs need no addition: the TUI runs `alder logs <env> -f -n 500` and parses each line as
the `sinceWriter` already does (text stamp, JSON record, RFC3339 prefix, optional
`[instance] `), so JSON apps get level and `rid` filters and text apps get regex.

#### Screens

**Envs** — one row per environment: name, kind, current build (id, when, by), health (the TUI
polls `<address>/_alder/health` every 10 s when `address` is known; else `—`), backup age,
silos summary. An environment with hosts unfolds into one row per host (`enter` or `→`),
each with its own current build and health. `enter` opens the environment or host; `d`
deploys to it; `b` rolls it back; `l` opens its logs; on a host row the commands carry
`--host`. Footer: `alder status <env> [--host name]`.

**Env** — top: the builds list (`*` on the current, note as `status` prints it); bottom: the
environment's logs following, with the Logs screen's filters. `d`, `b` and `L` (full-screen
logs) as above.

**Deploy** — a sheet before anything runs: environment, target, `HEAD` (commit and whether the
tree is clean, from `git`), and the plan from `alder deploy <env> --dry-run --json`: the
migrations, with `DROPS DATA` in bold red when any step does. `--allow-data-loss` and
`--unsafe-skip-tests` are toggles that default off and are named in the sheet when on. `enter`
runs `alder deploy <env> --json --progress json` and the sheet becomes a checklist: each
`heading` a section, each `step` a row with its mark and note, `FAIL` stopping the list with
the error. On success the screen turns into the **post-deploy watch**: the new build in the
header, the logs following, health polled every 5 s, a strip with the error count since the
switch, for two minutes or until `esc`; `b` from here rolls back to the previous build with
one confirmation. Footer: the exact command.

**Rollback** — pick a build from the list (the previous one preselected), confirm, run
`alder rollback <env> <id> --json --progress json`. A `schema FAIL` shows the refusal and the
two ways out as choices: `--allow-schema-mismatch`, and `--restore-backup` when the target
supports it (the TUI confirms the data loss itself, then passes `--yes`). Success enters the
same post-deploy watch.

```
 deploy prod (render)   HEAD 3fa9c12 clean   build 20261101-140512
 gate
   check    ok
   build    ok     2.1s
   test     ok     42 passed
   mutate   ok     score 0.91 (floor 0.85)
 plan
   migrations  add_reviews  DROPS DATA: books.rating
 ship
   upload   ok     14.2 MB
   backup   ok     builds/20261031-2201/backup.db
   migrate  …
```

### Remote: later

Designed to fit, not built now: **restore/PITR** (`alder restore --to` with a time picker and a
`--out` to inspect a backup in the Data screen); **silos** (`silos prune`, per-silo state from
`status --json`); **config check** (which `ALDER_*` each environment sets, masked — needs a
target method); **incident view** (Logs preset: warn+ or 5xx, beside the builds timeline);
**remote request detail** (waits for #118's wide events and a remote ring or store); **Metrics**
tab from `/_alder/metrics` (needs a token for deployed apps; the local strip already covers the
common case).

## Look

- 16 ANSI colours only, so the terminal theme applies; the same meanings as
  `alder/internal/style`: `ok` green, `FAIL`/`error` bold red, `warning`/`SKIPPED` yellow,
  `fix:` cyan, timestamps and `alder run:` lines dim, headings bold. Status classes: 2xx plain,
  3xx dim, 4xx yellow, 5xx bold red.
- Dense, not decorated: no boxes around everything, one rule under the header, two-space
  gutters like `style.Columns`. Borders only where a pane needs a boundary.
- The footer's command is real and copyable (`y` on the footer copies it).
- Minimum 80 columns; below that, the Request detail stacks its sections instead of
  side-by-side. 24 rows minimum.

## Testing

- `alder` additions: handler tests for `/_dev/events` (hello, replay, `?after`), the JSON
  rendering of child log lines, `/_dev/requests` (ring, 404, logs captured on the record),
  `/_dev/map.json`, `status --json` (both forms, golden against `Status`'s text for the same
  fake target), `--progress json` (every `line()` call site covered by one gate test), `alder
  tui` dispatch (found, missing, `ALDER_TUI`).
- TUI: `teatest` golden frames per screen, driven by a **fake feed** (recorded `/_dev/events`
  NDJSON fixtures) and a **fake runner** (an interface the subprocess calls go through:
  `test --json`, `routes --json`, `status --json`, `deploy`, `logs -f`). One integration test
  starts a real `alder run` on an example app and asserts the hello and the first request
  event; it is skipped when `alder` is not on `PATH`.
- A fixture app under `tui/testdata/` with a page, a message, a table, a slow route and an
  N+1 route, so the screens have something real to show in goldens.

## Sequencing

PRs in order, each mergeable alone:

1. **alder — the feed.** Child JSON logging rendered as text, `/_dev/events` with ring and
   `?after`, `POST /_dev/restart`, `alder tui` dispatch. No visible change for `alder run` users.
2. **alder — request detail.** Request ring, `/_dev/requests[/{rid}]`, log lines captured on
   the record, `rid` on JSON log records, `db_stmts`/`db_ms`/`repeats` on the debug access
   record, `/_dev/map.json` with SQL names and silos.
3. **tui — skeleton, Requests, Logs.** Module, CI job, release build, attach/own, status bar,
   Problems overlay, Requests + Request detail, Logs with filters and the rid jump. Golden
   frames from the start.
4. **tui — Tests, Routes, Data.**
5. **alder — remote JSON.** `status --json` (both forms), `--progress json`, `rollback --json`.
6. **tui — Envs, Env, Deploy, Rollback, remote Logs, post-deploy watch.**

Steps 1–4 are the local milestone the user asked for first; 5–6 the remote one. #120 is
updated with this order; the later items go to their own issues when 6 lands.

## Left to the builder

- Charm versions (current stable majors) and widget choices (Bubbles' table/viewport/list or
  hand-rolled).
- Exact SSE reconnect policy beyond "resume with `?after`, back off to 5 s".
- How the Data screen maps silos to files (read `rt/db`).
- Column widths, truncation and the small-terminal fallbacks beyond the two rules above.
- Whether the request-to-route match on the Routes screen lives in the TUI or `alder routes
  --json` gains a matcher; the TUI is fine for v1.

## Appendix A: what exists today (from the survey)

- `alder run` builds, `exec`s the app as a child on `127.0.0.1:0` and reverse-proxies it on
  `ALDER_ADDR` (default `localhost:8080`); it owns `/_dev/reload` (Datastar SSE) and
  `/_dev/theme*`; it prints `alder run:` status lines and the child's stderr with colouring on a
  TTY; no pid file, no JSON, no event stream (cmd/alder/run.go).
- App logs: stderr, text or `ALDER_LOG_FORMAT=json` (slog: `time level msg method path status
  ms bytes rid`, `+steps` at Debug, `+slow route steps db_stmts db_ms` when slow). User `Log.*`
  and `effect <name>` lines carry no rid. `X-Request-Id` on every response (rt/applog, rt/web).
- `eff.Recorder` (steps, SQL, loads, cuts) exists per request in dev and is rendered only into
  the HTML dev panel (rt/web/devpanel.go).
- `/_alder/health` JSON; `/_alder/metrics` Prometheus text behind `ALDER_METRICS`; `/_dev` HTML
  from `Options.DevMap`.
- `--json` exists for check (NDJSON), routes, test (streaming NDJSON; `--mutate` adds mutants
  and score), bench, eval, scratch, the generators, deploy, help. Not for status, rollback,
  logs, restore, silos.
- Deploy: `Target` interface (Ssh, Fly, Render, Script); `Record` per build stored on the
  remote; gate and ship rows through `line(w, step, status, note)`; rollback with schema check
  and `--restore-backup`; `logs -f/-n/--since/--grep` per target with `sinceWriter` parsing
  text and JSON stamps; environments declared in the app's `.alder` source.
- `internal/style`: 16 colours, `Mark`, `Columns`, `ALDER_COLOR`/`NO_COLOR`/TTY rules.
- No import-rule test; `TestDistModule` pins `go.mod`; nested modules invisible to `./...`;
  CI has no job for a `tui/` module.

Full survey notes (file:line for each item) are in `2026-11-01-alder-tui-survey.md`, next to
this document.
