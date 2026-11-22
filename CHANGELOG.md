# Changelog

Every release of alder, newest first. "Breaking" lists what stops an app that compiled
with the previous release, each with what to do.

## Unreleased

### Breaking

- `job` is a reserved keyword (it opens a job declaration), so an identifier named `job` stops parsing. Rename it (#168).
- A session field that is a `Text.Secret` (or a `Maybe Text.Secret`) is E0048: the session cookie is signed, not encrypted, so the user can read it. Keep the secret in a table and store the row's id in the session (#254).

### Added

- Background jobs (#168): a module declares a job with `job Name = { … }` and its arm `Name.run : Name.Params -> Task Unit`, with the options `every "1h"`, `cron "0 3 * * *"`, `retries`, `timeout` and `silo`; `every`, `retries`, `timeout`, `silo` and `cron` are checked: a `cron` expression has five fields (minute, hour, day of month, month, day of week) of numbers, `*`, `,`, `-` and `/`, in UTC, and an unreadable one, or one that can never match (`0 0 31 2 *`), is E0050 naming the field at fault. `Name { … }` builds a `Job`, and the `Job` module puts it on the queue: `Job.enqueue`, `Job.enqueueAt` (not before a time), `Job.enqueueOnce` (unless one of its kind with that key is queued or running) and `Job.status`. A job is stored in the `Task`'s transaction, so a failed `Task` drops it. In tests, `Test.jobs` lists every job with its status (queued, running, done or dead) and `Test.work` runs the due ones. A parameter that cannot be stored is E0049, and a job's options the rules refuse (two schedules, parameters on a scheduled job, an unreadable or out-of-range `every` or `timeout`) are E0050. Jobs run in the app's own process: `ALDER_JOBS=N` workers start with the app (default 4; `0` starts none; a bad value stops it before it serves), each claiming the oldest due row and running it as one transaction with the row's mark, and a failure is retried with a backoff until the job is dead. With `ALDER_METRICS`, `alder_jobs_total` (by kind and outcome), `alder_job_duration_seconds`, `alder_jobs_running` and `alder_jobs_queued` are exposed, and `/_alder/health` of an app that declares a job carries `"jobs": "ok"`, `"off"` or `"stalled"` (it never changes the status). Under `alder run`, a rebuild stops the old app with 10 s before it is killed (was 5 s), so its running jobs are queued again for the new one. `every` and `cron` schedules fire: a scheduled job has one queued tick, made when the workers start and at each housekeeping (every minute), due at the next minute its `cron` matches (UTC) or `every` later, put on the interval's grid from midnight UTC when the interval divides a day (`every "1h"` runs on the hour). Running a tick queues the next one, so a tick missed while the app was down runs once, late, not once per missed time, and a failed tick is retried like any job. A changed schedule applies at the next housekeeping, or after one more tick of the old one: a queued tick due later than the new schedule's next two times moves to the first of them, and a job whose `every` or `cron` was removed loses its queued tick. A queued tick of a job the build no longer has is deleted after a day. In tests, the first `Test.work` of a test makes the ticks from the test's clock. In an app with silos, a job is stored in the store its `Task`'s writes bind it to (the silo of the org a `requires org` route writes the tables of, else the shared store: a route that only queues a job writes nothing, so its job is in the shared store), and a `silo k` job runs bound to the silo its parameter names. The workers look for jobs in the shared database and in every silo that had jobs when they started (one pass over the silo files, or the Turso databases, opening none) or that a job was queued in since, until it has had no job queued or running for an hour; their looks do not keep a silo open, a closed silo is opened again only when one of its jobs is due, and a silo under maintenance is not looked in. `alder_jobs_queued` and the `"stalled"` check count those silos' jobs too. On Turso the jobs table is created by `alder deploy`'s migration run and when the app opens the database. A database whose claims keep failing is one warn line, then at most one a minute, and one line when it works again. `app --jobs-only` runs the app's job workers and no listener (`ALDER_ADDR` is ignored, and there is no `/_alder/health`): `ALDER_JOBS` defaults to 8 in this mode, and `SIGINT` or `SIGTERM` stops claiming, gives the running jobs 5 s, then cancels them and queues them again before it exits 0. On Turso it exits 1 when the database is behind the build, as serving does, and when a newer build migrated the database it runs no job and reads the database's schema again every 10 s until it matches. An app run with `ALDER_JOBS=0` that queues a job in a silo records the silo in the shared database (`_alder_jobs_silos`) before the `Task` commits, and the `--jobs-only` workers read those records every poll, so the job starts within a poll. A Render environment's `workers: N` (a whole number of at least 1, else E0030) moves the jobs to a worker service: `alder deploy` creates, or scales and rolls, a background worker `<service>-worker` running the same image as `app --jobs-only` on N instances, sets `ALDER_JOBS=0` on the web service, and waits for the worker's deploy to be live after the web health check; `alder status` lists both services and `alder logs <env> --worker` reads the worker service's log (a usage error naming `workers:` on an environment without it). `alder deploy init render` refuses an app with `workers` (worker machines need a shared Turso store, which `alder deploy` sets up and `render.yaml` cannot), and an environment without `workers` refuses every Render command (`deploy`, `status`, `logs`, `rollback`, `restore`) while a `<service>-worker` service still exists, until it is deleted or suspended.
- Book appendix D, the standard library reference, generated from the doc comments in `stdlib/` and held current by a test (#227).
- `alder search <query>`: search the standard library and the app by name, by module (`Text.`) or by type (`Text -> Int`, `: List Text`, `_` as a wildcard), with `--json` (#166).
- Language server: `alder/search` and `alder/searchEdit` (search by name or type, and for a picked result the edit and the cursor), `workspace/symbol` for the app's declarations, and a "Search for <type>" action on typed holes for clients that declare `experimental.alderSearch` (#166).
- Doc comments: a comment above a declaration in the format of a summary sentence, detail paragraphs, examples indented four spaces and a `Book: chapter N, "Heading"` line. Hover shows it with the examples in a code block and the book line, completion items carry it, `alder search` shows the summary in its DOC column, and `alder search --doc` prints the details, examples and book line; `--json` results gain `summary`, `examples` and `book`. `List`, `Maybe`, `Result`, `Text`, `Int`, `Float`, `Bool`, `Dict`, `Bytes`, `Task`, `Query`, `Db`, `Session`, `Notice`, `Response`, `Html`, `Http`, `Url`, `Log`, `Prelude`, `Date`, `Time`, `Random`, `Json`, `File`, `Test` and `Fuzz` are documented: every module and declaration of the standard library has a doc (#119).
- Hover on an operator (`|>`, `??`, `==`, `+`, …), on `<-` in an effect block or on a keyword (`match`, `route`, `requires`, …) shows what it does, its signature when it has one, examples and a book reference; hover on a typed hole adds what a hole is and how `alder fill` fills it (#119).

### Fixed

- Two `alder rollback --restore-backup` or `alder restore` runs within one second kept the database each replaced under one name (`db/app.db.before-rollback-<time>`), so the second overwrote the first's. A second kept file in the same second now gets `-2`, `-3`, … after the time, on SSH (and silo and Turso) targets and on Fly.
- The dev panel and its "Copy as curl" line mask a route's `Text.Secret` form fields and path parameters by type, not only by name (a `pin` field was shown), and a log line masks a `Text.Secret` path parameter. The `Text.Secret` docs now say what is masked and what is not: `Log`, `Json.string`, `++` and a view's text show it in clear (#254).

## v0.2.0 — 2026-11-11

### Breaking

- Routes replaced pages, messages and `Nav` (#160 and its follow-ups). Every request goes to a
  route: a `page` is a GET route, a `msg` is a write route, an update answers `Task Response`
  (`Response.refresh`, `Response.redirect`, `Response.invalid`), and `Html` takes no type
  argument. The removed syntax is E0046, which names the replacement (chapter 6 has the
  rewrite); there is no migration tool. A write route that declares `form F` is sent with
  `<form submit={R}>`: `click={R}` of it is E0045. `alder new page`, `edit page` and
  `remove page` are gone for `alder new route`, `edit route` and `remove route`; `alder new msg`
  is `alder new action` (the old name still works as an alias until the next release). In
  tests, `Answer`'s `Rerendered` is `Refreshed` (or `Rendered r`, with the route), and
  `Test.click` and `Test.submit` take the route (`Test.click (Toggle { id: id })`).
- alder-ui is v0.4 or later (the shadcn look; v0.1.0 shipped v0.3.3). The `--al-*` tokens of
  v0.3 are removed, so your own CSS that uses `var(--al-…)` stops applying, and `[data-theme]`
  and the OS-dark CSS are gone, so dark mode needs the theme script in your Layout's `<head>`.
  Appendix C, "Migrating from v0.3.x", has the steps and the table of removed tokens.
- The standard library gained `Json` (and `Bytes`, `File`, `Fuzz` and `Response`). An app module
  with one of those names is E0017: rename it, or use the standard library's.
- Imports have one canonical spelling (#23). An `import` whose `as` is missing, redundant or
  not the canonical qualifier is E0204, a name written with an imported module's full path
  instead of its qualifier is E0205, and an import with no usable qualifier is E0206. Run
  `alder fmt`, which rewrites the imports and the names (E0206 needs a rename by hand).
- An Int literal that does not fit in 64 bits is an error (E0001); it used to become
  9223372036854775807 (#81). `-9223372036854775808` is writable.
- The body of `<script>` and `<style>` is raw text, as in HTML. A body that is one `{ … }`
  interpolation, or that contains `</script` or `</style`, is E0001. Pass data to a script
  through `data-*` attributes.
- A query string a route cannot parse (a raw `;`, a bad `%` escape) answers 400 naming the
  parameter; it used to reach `load` as if the parameter were absent (#12).

### Added

- `alder lsp`: diagnostics, completion, hover, definition, references, code actions, semantic
  tokens, folding, selection ranges and the generators (`alder/plan`, `alder/preview`); fill
  typed holes with AI from the editor.
- `alder tui`: a terminal UI for a running app (requests, logs, tests, routes, data).
- New commands: `alder fmt`, `alder scratch`, `alder bench` (with N+1 warnings), `alder routes`,
  `alder theme` and the `/_dev/theme` configurator, `alder logs` filters; styled output.
- Standard library: `Json`, `File`, `Bytes`, `Response`, `Fuzz`, `Html.fragment`, `Text.hash`,
  a seeded `Random`, `Url.absolute`, `Test.setClock`.
- Property tests (`Fuzz.forAll`, shrinking, saved failures) and route mutation operators.
- Deploy to Render beside Fly (`alder deploy init render|fly`), a health endpoint, a metrics
  endpoint, an access log and request ids.
- Generators for routes (`alder new route`, `new action`) and the admin on resource paths.

### Fixed

- Migrations are listed by reading the directory, not by globbing it, so an app path with
  glob characters (`[`, `*`, `?`) builds and deploys (#172).
- E0017 says to rename the module or use the standard library's, and its explanation points
  here; a `Text` where a `Json.Value` is expected names `Json.string` and `File.text` (#175).

https://github.com/tapegram/alder/compare/v0.1.0...v0.2.0
