# `alder bench` baseline, 2026-10 (#48)

> **Taken under load: re-run before using these numbers as a baseline.** The machine was shared
> with other agents and was not idle. `uptime` before and after the runs:
>
> ```
>  0:56  up 21 days, 16 hrs, 5 users, load averages: 122.19 78.40 68.40   (before the probe)
>  1:03  up 21 days, 16:07, 5 users, load averages: 7.47 47.22 60.55      (before the cache runs)
>  1:05  up 21 days, 16:09, 5 users, load averages: 11.64 35.25 54.00     (before the final run)
>  1:05  up 21 days, 16:09, 5 users, load averages: 12.55 35.04 53.82     (after it)
> ```
>
> The `ms` column moved by up to 2x between two runs of the same build minutes apart (`toggle 1000`'s
> `GET /`: 6.09 ms, then 10.12 ms). Before #49 or anything else compares against this file, re-run
> it on an idle machine and replace the tables below:
>
> ```
> alder bench testdata/bench/rows --count 10
> go test ./rt/db -run '^$' -bench BenchmarkSQLiteGetByID -benchtime=300ms -benchmem -count 5
> ```
>
> The columns other than `ms` (`bytes`, `alloc`, `req`, `loads`, `steps`, `sql`, `n+1`) do not
> depend on the load and are good as they are.

A record, not a target. Machine: Apple M5 Pro, 18 cores, 64 GB, macOS (Darwin 27.0.0); Go 1.27.2
(darwin/arm64). Code: branch `issue-48-perf-harness` at the commit that adds this file (the
runtime without a statement cache, see below). Fixture: `testdata/bench/rows`; each test seeds its
rows (not measured), then takes the steps its name says. `ms` and `alloc` are medians of 10 runs.

## `alder bench testdata/bench/rows --count 10`

| test | step | ms | bytes | alloc | req | loads | steps | sql | n+1 |
|---|---|---:|---:|---:|---:|---:|---:|---:|---|
| index 100 | GET / | 0.66 | 84.1 kB | 1.87 MB / 8.1k | 1 | 1 | 1 | 1 | |
| index 1000 | GET / | 5.71 | 811.3 kB | 17.96 MB / 79.9k | 1 | 1 | 1 | 1 | |
| index 10000 | GET / | 56.66 | 8.10 MB | 181.42 MB / 800.0k | 1 | 1 | 1 | 1 | |
| toggle 1000 | GET / | 10.12 | 811.3 kB | 17.96 MB / 79.9k | 1 | 1 | 1 | 1 | |
| toggle 1000 | POST /_m/Rows.Index/Toggle | 18.07 | 808.4 kB | 31.97 MB / 181.1k | 1 | 2 | 3 | 4 | |
| add 1000 | GET / | 8.49 | 811.3 kB | 17.96 MB / 79.9k | 1 | 1 | 1 | 1 | |
| add 1000 | POST /_m/Rows.Index/Add | 12.15 | 809.2 kB | 24.77 MB / 125.1k | 1 | 2 | 3 | 3 | |
| jump 1000 | GET / | 11.65 | 811.3 kB | 17.95 MB / 79.9k | 1 | 1 | 1 | 1 | |
| jump 1000 | POST /_m/Rows.Index/Jump | 6.86 | 766 B | 10.65 MB / 83.3k | 2 | 2 | 2 | 2 | |
| slow 100 | GET /slow | 1.02 | 3.0 kB | 390.6 kB / 7.3k | 1 | 1 | 101 | 101 | 100x (100 args) |

What #49 reads first:

- **A click sends the whole body.** `toggle 1000`'s answer is 808.4 kB for one flipped row, against
  `index 1000`'s 811.3 kB for the whole page: the Datastar patch is the body, so a click costs a
  page.
- **A Rerender and a followed Redirect run `load` twice.** `loads` is 2 for `toggle` (before
  `update`, and after it to render) and for `jump` (the message's page, then the target's).
  `toggle`'s allocations (31.97 MB) are nearly two `index 1000` renders' worth (17.96 MB each).
- **The render is the cost, not the query.** `index 1000` against `index 10000`: 10x the rows is
  about 10x the time, bytes and allocations (5.71 → 56.66 ms, 0.81 → 8.10 MB sent, 80k → 800k
  objects) with one SQL statement either way. A click on a 1,000-row page is mostly the render.
- **The N+1 page** (`slow 100`, a `Db.get` per row): 101 statements and one repeat of 100, which
  `alder test` warns about and the dev panel marks.

## The statement cache: measured, not kept

#48's spec proposed caching prepared statements in `db.SQLite`. It was built (a per-store cache
of `*sql.Stmt`, reset on every schema change, off on Turso) and measured; it is not in the
runtime, because the rule for keeping it was that neither allocations nor SQL counts regress, and
allocations did.

`BenchmarkSQLiteGetByID` (`rt/db/prepare_test.go`, kept): a select by id of a 6-column row among
1,000, file database, `-benchtime=300ms -benchmem`, under the load above:

| | ns/op | B/op | allocs/op |
|---|---:|---:|---:|
| unprepared (`db.QueryContext`, what the store does) | 7,094 | 997 | 34 |
| prepared once (`stmt.QueryContext`) | 2,829 | 1,012 | 35 |
| in a transaction, unprepared | 5,968 | 1,178 | 38 |
| in a transaction, `Tx.StmtContext` of the prepared one | 2,696 | 1,860 | 45 |

(An earlier run at load 122 gave 64,966 / 24,722 / 70,390 / 15,489 ns/op with the same
allocations: the time is the load's, the allocations are not.)

`alder bench`, `--count 10`, the same fixture with and without the cache (the cache's build
prepared each text once per store, and inside a test's transaction on the `:memory:` database
once per transaction):

| test | step | allocs without | allocs with | sql without | sql with | ms without | ms with |
|---|---|---:|---:|---:|---:|---:|---:|
| index 1000 | GET / | 79,929 | 79,939 | 1 | 1 | 6.25 | 8.02 |
| toggle 1000 | GET / | 79,928 | 79,938 | 1 | 1 | 6.05 | 6.72 |
| toggle 1000 | POST /_m/Rows.Index/Toggle | 181,102 | 181,128 | 4 | 4 | 10.25 | 12.11 |
| slow 100 | GET /slow | 7,255 | 7,773 | 101 | 101 | 0.75 | 0.46 |

The SQL counts are the same; the allocations are higher with the cache on every row (+10 per
cold statement, +5 per statement on the N+1 page, +1 to +7 per statement warm in the probe),
so it stays out. The time it saves (about 4 µs a statement here, 7 µs on an idle machine per the
spec's probe) matters on the N+1 page (0.75 → 0.46 ms) and not against a 1,000-row render. A
later attempt could lower its allocations (reusing the transaction's statement instead of
`Tx.StmtContext` per call) and is measured against this file's re-run.
