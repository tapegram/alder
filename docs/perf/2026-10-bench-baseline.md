# `alder bench` baseline, 2026-10 (#48)

> **Taken under load: re-run before using these numbers as a baseline.** The machine was shared
> with other agents and was not idle. `uptime` before and after the runs:
>
> ```
>  0:56  up 21 days, 16 hrs, 5 users, load averages: 122.19 78.40 68.40   (before the probe)
>  1:03  up 21 days, 16:07, 5 users, load averages: 7.47 47.22 60.55      (before the cache runs)
>  1:05  up 21 days, 16:09, 5 users, load averages: 11.64 35.25 54.00     (before the final run)
>  1:05  up 21 days, 16:09, 5 users, load averages: 12.55 35.04 53.82     (after it)
>  1:20  up 21 days, 16:24, 5 users, load averages: 4.43 10.63 29.08      (before the server-only re-run below)
>  1:20  up 21 days, 16:24, 5 users, load averages: 4.24 10.48 28.92      (after it)
> ```
>
> The `ms` column moved by up to 2x between two runs of the same build minutes apart. Before #49
> or anything else compares against this file, re-run
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
rows (not measured), then takes the steps its name says. `ms` and `alloc` are medians of 10 runs,
and count the server only: the time and allocations while the router served the step's requests,
not the test browser parsing the answer or applying the patch.

## `alder bench testdata/bench/rows --count 10`

| test | step | ms | bytes | alloc | req | loads | steps | sql | n+1 |
|---|---|---:|---:|---:|---:|---:|---:|---:|---|
| index 100 | GET / | 0.39 | 84.1 kB | 1.25 MB / 5,766 | 1 | 1 | 1 | 1 | |
| index 1000 | GET / | 3.94 | 811.3 kB | 12.32 MB / 57,822 | 1 | 1 | 1 | 1 | |
| index 10000 | GET / | 36.49 | 8.10 MB | 123.53 MB / 579,839 | 1 | 1 | 1 | 1 | |
| toggle 1000 | GET / | 3.90 | 811.3 kB | 12.32 MB / 57,822 | 1 | 1 | 1 | 1 | |
| toggle 1000 | POST /_m/Rows.Index/Toggle | 4.20 | 808.4 kB | 12.70 MB / 66,739 | 1 | 2 | 3 | 4 | |
| add 1000 | GET / | 3.88 | 811.3 kB | 12.32 MB / 57,822 | 1 | 1 | 1 | 1 | |
| add 1000 | POST /_m/Rows.Index/Add | 4.42 | 809.2 kB | 12.70 MB / 66,749 | 1 | 2 | 3 | 3 | |
| jump 1000 | GET / | 3.96 | 811.3 kB | 12.32 MB / 57,822 | 1 | 1 | 1 | 1 | |
| jump 1000 | POST /_m/Rows.Index/Jump | 0.66 | 766 B | 431.9 kB / 9,018 | 2 | 2 | 2 | 2 | |
| slow 100 | GET /slow | 0.68 | 3.0 kB | 329.0 kB / 6,858 | 1 | 1 | 101 | 101 | 100x (100 args) |

What #49 reads first:

- **A click sends the whole body.** `toggle 1000`'s answer is 808.4 kB for one flipped row, against
  `index 1000`'s 811.3 kB for the whole page: the Datastar patch is the body, so a click costs a
  page, on the wire and in the render (12.70 MB / 66.7k objects against 12.32 MB / 57.8k).
- **A Rerender and a followed Redirect run `load` twice.** `loads` is 2 for `toggle` (before
  `update`, and after it to render) and for `jump` (the message's page, then the target's). The
  second `load` is cheap next to the render (+8.9k objects for `toggle` over a GET, which also
  covers `update` and the 4 statements); the render runs once.
- **The render is the cost, not the query.** `index 1000` against `index 10000`: 10x the rows is
  about 10x the time, bytes and allocations (3.94 → 36.49 ms, 0.81 → 8.10 MB sent, 58k → 580k
  objects) with one SQL statement either way. A click on a 1,000-row page is mostly the render.
- **The test browser's own share was large.** Measured over the whole step (as this file first
  did), `toggle 1000`'s click was 181.1k objects and `jump`'s 83.4k: most of it parsing the
  answer into a document and patching it, which is the client's cost, not the server's.
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
once per transaction). These were taken before `alder bench` measured the server only, so they
include the test browser's work; the differences between the columns are the cache's all the
same, since the browser's share is identical in both:

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
`Tx.StmtContext` per call) and is measured against this file's re-run: #105.

## #49: fragments (a click patches what changed)

`alder bench testdata/bench/rows --count 10` (server only, as above), before (`issue-48-perf-harness`
at de343dcd, the fixture without row ids) and after (`issue-49-datastar`, rows
`<li id={"row-" ++ r.id}>`), minutes apart on the same machine, under load again (`uptime`
load averages 24 to 66): the `ms` column is not comparable, every other column is.

| test | step | bytes before | bytes after | alloc before | alloc after | req | loads | sql |
|---|---|---:|---:|---:|---:|---:|---:|---:|
| index 100 | GET / | 84.1 kB | 70.1 kB | 1.25 MB / 5.8k | 1.09 MB / 5.7k | 1 | 1 | 1 |
| index 1000 | GET / | 811.3 kB | 673.5 kB | 12.32 MB / 57.8k | 10.78 MB / 56.8k | 1 | 1 | 1 |
| index 10000 | GET / | 8.10 MB | 6.68 MB | 123.53 MB / 579.8k | 108.64 MB / 569.8k | 1 | 1 | 1 |
| toggle 1000 | POST /_m/Rows.Index/Toggle | 808.4 kB | **938 B** | 12.70 MB / 66.7k | 14.29 MB / 117.8k | 1 | 2 | 4 |
| add 1000 | POST /_m/Rows.Index/Add | 809.2 kB | 666.7 kB | 12.70 MB / 66.7k | 16.30 MB / 117.8k | 1 | 2 | 3 |
| jump 1000 | POST /_m/Rows.Index/Jump | 766 B | 838 B | 431.9 kB / 9.0k | 432.9 kB / 9.0k | 2 | 2 | 2 |
| slow 100 | GET /slow | 3.0 kB | 3.4 kB | 329.0 kB / 6.9k | 333.0 kB / 6.9k | 1 | 1 | 101 |

(The "after" column is the PR's fix round 1, load averages 26 to 59; round 2's `?.` on the helper call adds 2 bytes a button: index 1000 is 673.5 kB, toggle 938 B, the allocations unchanged.)

The keep rule holds: `toggle 1000` sends 938 B (one `<li>` and the `al-shown` event) against
the page's 673.5 kB, 0.14% (rule: at most 2%); its allocations are 1.13x the baseline (rule: at
most 1.7x); `req`, `loads` and `sql` are unchanged on every row. `loads` stays 2: the
before-view renders the model loaded before `update` and runs no query.

Where the other numbers move:

- **The page is 138 kB (17%) smaller**, ids and all. Every post action now calls one helper the
  boost defines once in the `<head>`, `@post('<url>', window._alderO?.())` (`window._alderO?.('form')`
  for a form): it aborts the navigation in flight, starts the next one's AbortController and adds
  the `Alder-Shown` header. That replaces the 86 bytes of abort and controller every button
  carried before (plus their escaping), and pays for the fixture's new `id="row-…"` on 1,000 rows
  (30 kB) and the `<al-shown>` element (60 B).
- **`toggle` and `add` render the page twice.** The before-view is a second view and render
  (+51k objects, the same as a GET's tree). Its buffer is sized from the after-page up front
  (`html.RenderTrackedSized`); grown step by step it cost 4 MB more. `add 1000` gains nothing in
  bytes for it: a row added changes `#rows`' children, so the whole `<ul>` is sent (the keyed-diff
  follow-up). Regions are tracked only when the request carries `Alder-Shown`.
- The equality check the test browser runs after every re-render (a parse, the parser's repairs
  modelled by `html.Repair`, and a diff of the body) is the browser's work, not measured.

## #160: routes (the fixture on `route`, a click `render`s one row)

`alder bench testdata/bench/rows --count 10` (server only), before on main at b6e880ca (the
fixture's `msg` pages) and after on `routes-test-browser` (the fixture as one route module:
`Index = GET /`, `Item = GET /rows/{id}`, `Toggle = PATCH …` answering `render (Item {id})`,
`Delete = DELETE …` and `Add` answering `refresh`). Both runs on a quiet machine: `uptime` 1-min
load 1.76 (before) and 1.73 (after).

Before (main b6e880ca):

| test | step | ms | bytes | alloc | req | loads | steps | sql |
|---|---|---:|---:|---|---:|---:|---:|---:|
| index 100 | GET / | 0.58 | 70.8 kB | 1.10 MB / 5.7k | 1 | 1 | 1 | 1 |
| index 1000 | GET / | 3.96 | 673.8 kB | 10.95 MB / 56.8k | 1 | 1 | 1 | 1 |
| index 10000 | GET / | 39.47 | 6.72 MB | 110.24 MB / 569.8k | 1 | 1 | 1 | 1 |
| toggle 1000 | POST /_m/Rows.Index/Toggle | 7.59 | 938 B | 14.58 MB / 117.8k | 1 | 2 | 3 | 4 |
| add 1000 | POST /_m/Rows.Index/Add | 8.00 | 670.7 kB | 16.60 MB / 117.8k | 1 | 2 | 3 | 3 |
| jump 1000 | POST /_m/Rows.Index/Jump | 0.60 | 838 B | 432.8 kB / 9.0k | 2 | 2 | 2 | 2 |
| slow 100 | GET /slow | 0.63 | 3.6 kB | 332.8 kB / 6.9k | 1 | 1 | 101 | 101 |

(The old fixture has no delete; the PR 3 hand-over measured one on a temporary copy: `delete 1000`,
Rerender after a toggle, page 967.8 kB, 10.90 ms, 963.1 kB, 23.71 MB / 159.6k, req 1, loads 2, sql 3.)

After (routes-test-browser at 2b42aa67 plus the fixture):

| test | step | ms | bytes | alloc | req | loads | steps | sql |
|---|---|---:|---:|---|---:|---:|---:|---:|
| index 100 | GET / | 0.46 | 100.8 kB | 1.40 MB / 6.1k | 1 | 1 | 1 | 1 |
| index 1000 | GET / | 4.92 | 973.8 kB | 14.89 MB / 60.8k | 1 | 1 | 1 | 1 |
| index 10000 | GET / | 48.03 | 9.72 MB | 148.84 MB / 609.8k | 1 | 1 | 1 | 1 |
| toggle 1000 | PATCH /rows/{id} | 0.11 | 1.1 kB | 26.6 kB / 321 | 2 | 1 | 2 | 3 |
| add 1000 | POST /rows | 10.23 | 971.0 kB | 25.55 MB / 125.9k | 3 | 6 | 5 | 9 |
| delete 1000 (after a toggle) | POST … (DELETE /rows/{id}) | 10.09 | 969.1 kB | 25.54 MB / 125.7k | 3 | 6 | 5 | 9 |
| jump 1000 | POST /rows/{id}/jump | 0.08 | 843 B | 19.9 kB / 204 | 2 | 1 | 1 | 1 |
| slow 100 | GET /slow | 0.63 | 3.6 kB | 332.7 kB / 6.9k | 1 | 1 | 101 | 101 |

(The `delete` and `toggle` rows' step label is the method the browser sent, `PATCH`/`POST`.)

The keep rule (R-P4, against today's 938 B and 2% of today's page):

- **`toggle 1000` holds.** It sends 1.1 kB, one `<li>` (now with a Delete button: +160 B) and
  the event, against 13.5 kB (2% of the old page; 19.5 kB of the new 973.8 kB one): 0.11%. It
  allocates 26.6 kB / 321 objects against 14.58 MB / 117.8k, and runs in 0.11 ms against 7.59 ms.
  The page's `load` runs 0 times. The `loads` column's 1 is the render's own `Item.load` (the
  second Env of the request: 0 loads and 2 statements for the write, 1 load and 1 statement for
  the render), and `sql` is 3, not 2: the write's `Db.update` is 2 statements, `Item.load` one
  more. `req` 2 is those two Envs.
- **`delete 1000` via `refresh` after a toggle stays on the regions path** (969.1 kB sent, the
  `#rows` region, not an extra navigation; PR 3's Rerender row sent 963.1 kB): 25.54 MB / 125.7k
  against 23.71 MB / 159.6k (8% more bytes, 21% fewer objects), 10.09 ms against 10.90 ms.
- `req` 3, `loads` 6, `sql` 9 on `add` and `delete` are **not** more work: the refresh's three
  Envs share one recorder, and `alder bench` adds the recorder once per Env (`rt/testing/bench.go`,
  `s.m.Loads += e.Rec.Loads`). Counted once: loads 2, sql 3, the same as before. The bench Go
  code is unchanged; this double count is a follow-up to fix there.
- `add 1000` allocates 25.55 MB against 16.60 MB (+54%), 125.9k objects against 117.8k: the
  fixture's page is 300 kB larger (the Delete button on every row), which accounts for the
  bytes (a 30% bigger page rendered twice), not for the objects.
- The index page grew from 673.8 kB to 973.8 kB for the same reason (the Delete button, 300 B a
  row); `index` times and allocations scale with it.
- `checkOptionRoutes` (a DFS per GET route) does not show on this fixture: `alder check` takes
  11 ms with three GET routes. It is quadratic in the GET routes; not measured on a larger app.
