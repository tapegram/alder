# Runtime perf: `alder bench`, N+1 warnings, a statement cache

Design for [tapegram/alder#48](https://github.com/tapegram/alder/issues/48). The evidence: no
end-to-end perf numbers in the repo; every click re-runs `load` + `view` and morphs the whole
`<body>`; a `Redirect` runs `load` twice (the message's page before `update`, the target's after);
SQLite runs with WAL and no statement cache; nothing flags a `Db.get` per row; nightvote-replay's
dev log (2026-10-22): "a profiling harness had to be improvised … Alder has no `alder bench`".
PR #83 (#37) is the micro layer (Go benchmarks under `rt/`); this is the layer above it. #49
(Datastar fragments) measures with this harness first; nothing here designs #49. Everything
marked **(call)** is the author's decision, not the user's.

> **Not shipped: the statement cache.** It was built as designed below and measured: it saved
> time per statement but raised allocations on every measured row (`alder bench` and
> `BenchmarkSQLiteGetByID`), and the rule for keeping it was no regression in allocations or SQL
> counts. It is not in the runtime; the numbers are in `docs/perf/2026-10-bench-baseline.md`, and
> a lower-allocation attempt is #105. The parts below marked *(not shipped, #105)* describe it.

## Intent

`alder bench` reports, per browser step of the app's own tests, what a request costs: time,
payload bytes, allocations, requests served, loads, effect steps, SQL statements, and whether one
statement ran in a loop. The same N+1 rule warns in the dev panel and after a test's `ok` line,
before Turso turns the loop into N round trips. Reads prepare each statement once per connection *(not shipped, #105)*.
Baselines for 100, 1,000 and 10,000 rows are in the repo, so #49 and every later runtime change
has a before.

## Goals

1. `alder bench [dir] [-run re] [--count N] [--json] [--build-dir DIR]`: the app's task tests
   run as `alder test` runs them, and every browser step is measured and reported.
2. A fixture app `testdata/bench/rows/` whose tests exercise the shapes #48 names: a page of N
   rows, a click (Rerender), a form submit, a Redirect followed under Datastar, an N+1 load.
3. One N+1 rule (`eff.Repeats`) with three surfaces: the dev panel's bar and SQL tab, a `warn:`
   line under a test's result, a column of `alder bench`.
4. A prepared-statement cache in `db.SQLite`, invalidated on every DDL path, off on Turso *(not shipped, #105)*.
5. `docs/perf/2026-10-bench-baseline.md` holds the first numbers, as a record, not a target.
6. Existing programs, tests and the panel behave as before (`go test ./...` is the witness).

## Non-goals

- Changing the render model (fragments, signals, skipping the second `load`): #49, with these
  numbers in hand.
- Browser-side cost (the morph's time in the DOM): the harness is server-only and says so.
- A generic seeder: the fixture's tests seed their rows in Alder (`Task.all` of `Db.insert`).
- Pipelining several statements into one Turso request (see "What Turso would need").
- A perf gate in CI: CI runs the harness once to prove it works, and asserts shape, not time.
- `alder bench` against a production database or a running server.

## Anti-goals

- Any cost in production: the Recorder stays nil there, so `Repeats` never runs; the statement
  cache is the only production change and it is measured.
- A warning that fails a test: N+1 is a `warn:` line, never a `FAIL` **(call)**: a gate the user did
  not ask for would break nightvote-replay's suite on a page it accepted.
- A cached statement that outlives its schema (hygiene below), or an unbounded cache.

## `alder bench`

`alder bench` builds the same test program `alder test` builds (`testMain`, into
`build/<app>/scratch/test`) and runs it with `--bench` (env `ALDER_TEST_BENCH=1`) **(call)**: no
new main, no new language; the app's tests are the scenarios, written with the app's own types,
fakes and `Test.*` steps. `-run`, `--json`, `--build-dir` and the check/adapter handling are
`alder test`'s; `--count N` (default 3) runs each selected test N times. Pure and property tests
are skipped (nothing to measure). The timeout is `alder test`'s default.

Measured, per browser step (`Test.get`, `Test.open`, `Test.click`, `Test.submit`,
`Test.submitRaw`, `Test.fetch`), from `Browser.serve` to its return:

| column | meaning |
|---|---|
| `ms` | wall time of the step; with `--count`, the median over the runs |
| `bytes` | the answer's body: the document of a GET, or the SSE text of a Datastar answer (body patch, title, history, panel-less) |
| `alloc` | bytes allocated and allocations (`runtime.MemStats` `TotalAlloc`/`Mallocs` delta) |
| `req` | requests the router served for the step: 1, or 2 for a redirect it followed, plus the browser's own follows of a 303 |
| `loads` | `Options.Served` calls: how many times a page's `load` ran (a Rerender is 2: before and after `update`; a followed Redirect is 2) |
| `steps` | effect Calls (the request Envs' `Trace`) |
| `sql` | statements (the Recorder's `SQL`) |
| `n+1` | the repeats `eff.Repeats` finds (empty when none) |

The test's own effects (its seeding inserts, `Test.run`) are not measured. Text output, after
each test's `ok` line, one indented row per step, with the step named as its request
(`GET /`, `POST /_m/Rows.Index/Toggle`); `--json` adds `"bench": [{step, ms, bytes, alloc,
allocs, requests, loads, steps, sql, repeats}]` to the test's object. A test that fails is
reported as `alder test` reports it and measured anyway (its steps ran).

Fidelity, stated in the book: the browser's requests run within the test's transaction under a
savepoint and with the Recorder on, so numbers are for comparing (before/after, N against 10N),
not production latencies; a production profile is still a `bench_test.go` in the build directory
(`alder build --emit`), which the book says in one paragraph.

Where it lives: `rt/testing/bench.go` (`Measure`, the per-step hook `serve` fills:
`ReadMemStats` before and after, `w.Body.Len()`, a `Served` counter installed before
`NewBrowser`, Rec on each request Env made by `EnvFor`; `CaseResult.Bench []Measure`; the text
and JSON writers) and `cmd/alder/bench.go` (`runBenchTo`, `runTestTo`'s parsing shared;
`--mutate`, `--fuzz`, `--seed` refused with `benchUsage`). `Main` gains `--bench` and `--count`.

### The fixture app

`testdata/bench/rows/` (laid out like `testdata/corpus/browser-ok`, no auth) **(call)**:

- `Table.Row`: `title: Text`, `n: Int`, `done: Bool`, `@created`.
- `Rows.Index` at `/`: `load` is `Db.from Row |> Db.orderBy Row.created.asc |> Db.all`; `view`
  renders a `<ul>` with one `<li>` per row: title, `n`, a `Toggle { id }` button (`Rerender`), a
  `Jump { id }` button (`Redirect (Rows.Show { id })`), and above it `form NewRow = { title:
  Text.NonEmpty }` with `msg Add form NewRow` (`Rerender`).
- `Rows.Show` at `/rows/{id}`: `Db.get Row id`, 404 when missing.
- `Rows.Slow` at `/slow`: `load` reads the rows, then `Query.all (List.map (\r -> Db.get Row
  r.id) rows)`: one SELECT per row, the N+1 the rule must catch.
- `Rows.Bench` module: `seed : Int -> Task (List Row.RowId)` (`Task.all` of `List.map` over
  `List.range 1 n`) and the tests, named `index 100`, `index 1000`, `index 10000`, `toggle 1000`,
  `add 1000`, `jump 1000`, `slow 100`: each seeds, then takes exactly the steps its name says,
  and returns `True` on the answer it expects (`Shown`, `Rerendered`, `Redirected (Rows.Show …)`).

## N+1: the rule and its surfaces

`eff.Repeats(sql []SQLRec, k int) []Repeat` **(call)**, pure, in `rt/eff/eff.go` next to the
Recorder: groups `sql` by `Stmt` text (arguments are bound, so the text is the shape);
a group of `Count >= k` is a `Repeat{Stmt, Count, Args int (distinct argument sets), Ms (sum)}`,
unless every statement of the group is a write (`INSERT`, `UPDATE`, `DELETE` prefix) issued under
one `Task.all` group (`SQLRec.Group`, the same nonzero value): that is a bulk action and stays
the panel's `x3 (bulk)`. A read under a `Task.all` is never exempt: `Query.all` of `Db.get` is
the N+1 that costs N round trips on Turso. Order: by `Count` descending.

`k = 5` **(call)**, `eff.RepeatThreshold`: two to four identical selects in one `load` are
hand-written lookups (two records on one page); five with distinct arguments is a loop. The
panel keeps flagging 2–4 as `duplicate xN`. The text, `Repeat.String()`:

```
N+1: SELECT "id", "title", … FROM "Table__Row" WHERE "id" = ? ran 100 times (100 argument sets, 3.21 ms): one Db.get per row; read them together with Db.where (Row.id.in ids), or in the page's first query
```

Surfaces:

1. Dev panel (`rt/web/devpanel.go`): per answer segment (the existing `seg` split), a repeat
   marks its statements `N+1 x100` instead of `duplicate x100`, and the bar shows `N+1` after the
   SQL count; the router logs each repeat at `warn` with the request line
   (`GET /slow: N+1: …`), dev mode only, since the Recorder exists only then.
2. `alder test`: the browser's `EnvFor` gives every request Env a Recorder **(call)** (it only
   records; timing calls per step and statement are the cost, well under a test's work). After
   each step `serve` runs `Repeats` over the request Envs' `Rec.SQL` and appends
   `"<METHOD> <path>: " + Repeat.String()` to `CaseResult.Warnings`. Text: `  warn: …` lines after
   the `ok`/`FAIL` block; JSON: `"warnings": […]` (omitted when empty).
3. `alder bench`: the `n+1` column, `Count` and `Args` only (the text is in the warn line).

## The statement cache *(not shipped, #105)*

`db.SQLite` gets `stmts map[string]*sql.Stmt` under a mutex **(call)**; `on(tx)` returns a
querier whose `QueryContext`/`ExecContext`/`QueryRowContext` resolve the text through it:
`s.db.PrepareContext` once per text, then `stmt.QueryContext` (no tx) or
`tx.StmtContext(ctx, stmt).QueryContext` (database/sql reuses the connection's prepared handle;
modernc 1.60 keeps the compiled `sqlite3_stmt` in the driver `stmt`). `recQ` wraps it as today.

Measured here before the change (`rt/db`, SELECT by id of a 6-column row, 1,000 rows, file
database, Apple M5 Pro, `-benchtime=300ms`): `db.QueryContext` 16.3 µs, prepared 9.2 µs (−43%);
inside a transaction 17.1 µs against 14.5 µs through `Tx.StmtContext` (−15%, 7 more allocs). A
page's `load` is a Query with no transaction, so reads get the larger gain; a Task's writes the
smaller. It is about 7 µs per statement: real on an N+1 page and on a cold `Db.get`, small
against a 1,000-row render. It ships because it is forty lines and the harness proves it.

Rules **(call)**:

- Cap: 256 texts; at 257 every cached statement is closed and the map emptied (a `.in` list makes
  one text per list length, `IN (?, ?, ?)`, so texts are not finite).
- Invalidation: `resetStmts()` (close all, empty) at the start of `Ensure`, `MigrateLog`,
  `CreateTables` and `recreate`, and in `Close`. SQLite re-prepares a handle itself when the
  schema changes (`SQLITE_SCHEMA` under `prepare_v2`), so this is hygiene for dropped tables and
  closed handles, not correctness.
- Off when `s.remote` (Turso): the hrana driver's `PrepareContext` is a stub that resends the SQL,
  so a cached `*sql.Stmt` there would only add the `Tx.StmtContext` allocations.
- The `:memory:` single connection and the reader (`OpenSQLiteReader`) use it unchanged.

What Turso would need: hrana's `store_sql`/`sql_id` saves bytes, not round trips; those go only
by sending several `execute` requests in one `/v2/pipeline` POST, which `eff.interpret` cannot
do for a `Bind` chain but could for `Query.all`/`Task.all`, whose steps are known up front. A
later issue; its number here is `sql`, since on Turso round trips are statements.

## Baselines

`docs/perf/2026-10-bench-baseline.md`, written by the builder from `alder bench
testdata/bench/rows --count 10` on an idle machine, naming the machine, Go and the commit; one
row per fixture test and step with every column above. The rows #49 reads first: `toggle 1000`'s
`bytes` (the whole body for one flipped row) against `index 1000`'s, `loads` of `toggle` and
`jump` (2 each), and `ms` of `index 1000` against `index 10000` (how much of a click is the
render). The file also holds `BenchmarkSQLiteGetByID` (`rt/db`, the probe above, kept) and
`index 1000`/`toggle 1000` before and after the cache, which stays only if neither row is slower.

## CI versus humans

CI (`go test ./...`): `TestBench_Rows` in `cmd/alder` builds the fixture and runs
`alder bench testdata/bench/rows -run 'index 100|toggle|add|jump|slow' --count 1`, text and
`--json`, asserting shape, never time: every step has a row, `loads` is 2 for `toggle` and
`jump`, `req` is 2 for `jump`, `slow 100` has `sql` 101 and one repeat with `Count` 100,
`index 100` has none. Benchmarks (`-bench`) are not run by CI. Humans run `--count 10` with
`index 10000` and update the baseline file when a runtime change lands.

## Where things change

| layer | files |
|---|---|
| runtime: rule | `rt/eff/eff.go` (`Repeats`, `Repeat`, `RepeatThreshold`), `rt/eff/eff_test.go` |
| runtime: panel | `rt/web/devpanel.go` (bar badge, `N+1 xN`), `rt/web/router.go` (the warn line after `serve`), `rt/web/devpanel_test.go` |
| runtime: tests, bench | `rt/testing/browser.go` (Rec in `EnvFor`, `Served` counter, measure hook in `serve`), new `rt/testing/bench.go`, `rt/testing/testing.go` (`CaseResult.Warnings`, `.Bench`, `--bench`, `--count`, `WriteResult`, `jsonResult`), `rt/testing/testing_test.go`, `browser_test.go` |
| runtime: db | `rt/db/sqlite.go` (`stmts`, `resetStmts`, the querier), `rt/db/migrate.go` (`MigrateLog` reset), `rt/db/db_test.go`, new `rt/db/stmtcache_test.go` (with `BenchmarkSQLiteGetByID`) |
| cli | new `cmd/alder/bench.go`, `cmd/alder/test.go` (shared parsing), `cmd/alder/main.go`, `help.go`, `helpjson` golden, new `cmd/alder/bench_test.go` |
| fixture | `testdata/bench/rows/` (app, tests) |
| book | `11-tooling.md`: `### alder bench` after `alder test` (columns, fidelity, the `bench_test.go` paragraph); Developer mode: `N+1` in the SQL tab and bar; `10-testing.md` §2: the `warn:` line; `08-database.md`: one sentence on the statement cache under the SQLite notes |
| site | `book/site/build.py` after the book edits |
| docs | `docs/perf/2026-10-bench-baseline.md` |
| checker, parser, IR, gen, formatter, LSP, tree-sitter, stdlib | nothing |

Also: tick #48 on the roadmap #52; `go install ./cmd/alder` after merge (a new command).

## Tests the builder adds, by behaviour

`rt/eff`: `Repeats` of 4 equal selects is empty and of 5 is one repeat with `Count` 5; distinct
argument sets are counted (`Args` 3 for 5 runs over 3 ids); 5 `DELETE`s under one group are
exempt, 5 `DELETE`s in two groups are not, 5 `SELECT`s under one group are a repeat; two shapes
each over `k` give two repeats ordered by count; `Repeat.String()` matches the text above.

`rt/web`: a request whose `load` runs one select per row shows `N+1 x100` in the SQL tab and
`N+1` in the bar, and logs one warn line naming the request; the same 100 statements as `DELETE`s
under one `Task.all` show `x100 (bulk)` and log nothing; the redirect-follow segment rule still
holds (session select per hop is not a repeat).

`rt/testing`: a browser test whose page repeats a select 5 times gets one `Warnings` entry and
`ok`; `--json` carries `"warnings"`; a step's `Measure` has `Bytes` equal to the recorder body's
length, `Requests` 2 and `Loads` 2 for a followed Redirect, `Loads` 2 for a Rerender click, `SQL`
equal to the statements recorded; `--count 3` reports the median `Ms`; pure and property tests
are skipped under `--bench`; `WriteResult` prints bench rows after `ok` and `warn:` lines.

`rt/db` *(not shipped, #105; kept: the migration-then-query and dropped-table store tests and
`BenchmarkSQLiteGetByID`, in `rt/db/prepare_test.go`)*: a second `Query` of the same text reuses the `*sql.Stmt` (the map holds one entry);
`Ensure`, `MigrateLog`, `CreateTables` and `Close` empty it and close the handles (a closed stmt
errors on use: the test keeps a pointer and checks); 257 distinct texts empty it once and the
258th is cached; a statement cached before a migration that drops its table fails with the
table error after it, not a stale answer; the remote store caches nothing; `:memory:` works
through the cache with its one connection; concurrent `Query` calls (`-race`) are safe.

`cmd/alder`: `TestBench_Rows` as in "CI versus humans"; `alder bench --mutate` and `--fuzz`
print the usage and exit 2; `alder help bench` and `alder help --json` list the command.

Left to the builder: whether `runBenchTo` is a flag path inside `runTestTo` or a thin function
over shared helpers (the usages stay separate); the text table's widths and the `alloc` format;
where the `Served` counter lives.

## Verification

- `go test ./...` green, including `TestBench_Rows`, with `-race` on `rt/db`.
- `docs/perf/2026-10-bench-baseline.md` committed with the numbers of an idle run, and the
  before/after rows for the statement cache in the PR body; neither `index 1000` nor `toggle
  1000` slower with the cache. *(As shipped: the user's rule was allocations and SQL counts, not
  time; allocations rose, so the cache is out, #105. The baseline was taken under load and says
  so; it is to be re-run idle with `--count 10`.)*
- A scratch check (not committed): `alder bench` on nightvote-replay's current main (its own
  Alder pin permitting) reports its `Sites.*` pages and shows no `N+1`; if it shows one, that is
  a finding for the dev log, not a change here.
