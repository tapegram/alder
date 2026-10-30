# Plan: `alder bench`, N+1 warnings, statement cache (#48)

Spec: `docs/superpowers/specs/2026-10-30-perf-harness-design.md` (binding). TDD: failing test,
code, green, commit, per task. Stay out of `rt/core.go` and `rt/*_bench_test.go` (PR #83).

## Task 1: the rule (`rt/eff`)

- `rt/eff/eff_test.go`: `TestRepeats_Threshold` (4 equal selects: none; 5: one, Count 5),
  `TestRepeats_Args` (5 runs over 3 ids: Args 3), `TestRepeats_BulkWrites` (5 DELETEs, one group:
  exempt; two groups: a repeat; 5 SELECTs, one group: a repeat), `TestRepeats_Order` (two shapes
  over k, by Count), `TestRepeat_String` (the spec's text).
- `rt/eff/eff.go`: `RepeatThreshold = 5`, `Repeat{Stmt, Count, Args, Ms}`, `Repeats`,
  `Repeat.String()` (column list cut to two names and `…`; the Db.get hint for a by-id select).
- `Recorder.Loads` (+ `AddLoad`), counted by `eff.RunQuery` when `env.Rec != nil` (a page's
  `load` is the only RunQuery a request runs): the `loads` column. Test: `TestRunQuery_CountsLoads`.

## Task 2: dev panel and the router's warn line (`rt/web`)

- `rt/web/devpanel_test.go`: `TestPanel_NPlusOne` (one select per row x100: `N+1 x100` in SQL,
  `N+1` in the bar, one warn log naming `GET /…`), `TestPanel_BulkDeletesNotNPlusOne`
  (`x100 (bulk)`, no warn); the redirect-follow segment test still passes.
- `devpanel.go`: per segment, `eff.Repeats`; marks and bar badge. `router.go`: after `rt.serve`
  in `ServeHTTP`, when the tracker has a Recorder, log `warn` `"<METHOD> <path>: " + r.String()`.

## Task 3: browser warnings and measures (`rt/testing`)

- `browser.go`: `EnvFor` gives each request Env a Recorder; a step (`Open`, `Fetch`, `Click`,
  `submit`) is bracketed by `beginStep`/`endStep`; `serve` accumulates requests (EnvFor calls),
  loads (`Rec.Loads`), steps (Trace), sql, bytes; `endStep` runs `Repeats` and appends warnings;
  with bench on, `ReadMemStats` before/after and wall time, appended as a `Measure`.
- new `bench.go`: `Measure`, the `--count` merge (median), text rows, JSON shape.
- `testing.go`: `CaseResult.Warnings`, `.Bench`; `Options.Bench`, `Options.Count`; `runCase`
  reads the case's browser; `WriteResult` prints bench rows after the ok line and `  warn:` lines
  (JSON `"warnings"`, `"bench"`, omitted when empty); `Main` gains `--bench`/`ALDER_TEST_BENCH`
  and `--count`/`ALDER_TEST_COUNT`; under bench pure and property tests are skipped.
- Tests (`browser_test.go`, `testing_test.go`): `TestBrowser_NPlusOneWarning` (5 repeats: one
  warning, still ok; JSON carries it), `TestBrowser_Measure` (Bytes = body length; followed
  Redirect Requests 2 Loads 2; Rerender Loads 2; SQL = recorded), `TestMain_BenchCountMedian`,
  `TestMain_BenchSkipsPure`, `TestWriteResult_BenchRows`.

## Task 4: the statement cache (`rt/db`)

- new `rt/db/stmtcache_test.go`: reuse (one entry), `Ensure`/`MigrateLog`/`CreateTables`/`Close`
  empty and close (kept pointer errors), cap 256 (257th empties, 258th cached), a migration that
  drops the table: the old text fails with the table error; a migration that adds a column, then
  a query on the changed table: works; remote caches nothing; `:memory:` inside a transaction;
  concurrent queries (`-race`); `BenchmarkSQLiteGetByID`.
- `sqlite.go`: `stmts`, `stmtMu`, `stmtQ` querier (no tx: `stmt.QueryContext`; tx:
  `Tx.StmtContext` of the cached handle, prepared on the pool when it can be, else per
  transaction on the single `:memory:` connection, where a pool prepare would wait on the
  transaction's own connection); only SELECT/INSERT/UPDATE/DELETE cached; a handle closed under a
  user is retried uncached; `resetStmts` in `Ensure`, `CreateTables`, `recreate`, `Close`;
  `migrate.go`: `MigrateLog`.

## Task 5: the fixture and `alder bench` (`cmd/alder`, `testdata/bench/rows`)

- `testdata/bench/rows/app/`: `App.alder`, `Row.alder`, `Rows/Index.alder`, `Rows/Show.alder`,
  `Rows/Slow.alder`, `Rows/Bench.alder` (seed and the seven tests).
- new `cmd/alder/bench.go`: `runBenchTo` over `runTestTo`'s shared parsing;
  `--mutate`/`--fuzz`/`--seed` print `benchUsage`, exit 2; `--count N` (default 3).
- `main.go`, `help.go` (`bench`), `helpjson.go` + `testdata/help.json`.
- new `cmd/alder/bench_test.go`: `TestBench_Rows` (text and `--json`, shape only),
  `TestBench_RefusesMutateFuzz`, `TestHelp_Bench`.

## Task 6: book, baseline

- Book: `11-tooling.md` `### alder bench` and Developer mode N+1; `10-testing.md` §2 `warn:`;
  `08-database.md` statement cache sentence; `python3 book/site/build.py`; experiments tests.
- `docs/perf/2026-10-bench-baseline.md`: taken under load (record `uptime`), to be re-run idle
  with `--count 10`; `BenchmarkSQLiteGetByID`; `index 1000`/`toggle 1000` with and without the
  cache (allocs/op and sql decide, not ms).
