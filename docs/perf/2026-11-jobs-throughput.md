# Job throughput, 2026-11 (#168)

A record, not a target. It answers what the spec (§6a, §7) left to be measured: the throughput
figure in the spec is River's, not ours. `alder bench` measures one browser step inside a test
transaction, so it gives the per-request cost of an enqueue but cannot measure concurrent claiming;
throughput is the Go benchmark `BenchmarkJobs` (`rt/jobs/bench_test.go`).

Machine: Apple M5 Pro, 18 cores, 64 GB, macOS (Darwin 27.0.0); Go 1.27.4 (darwin/arm64). Code:
branch `jobs-2` at 82cf87e4 plus the commit that adds this file. Store: a file-backed WAL SQLite
database opened by `db.OpenSQLite`, as the app opens it (`busy_timeout` 5 s, `synchronous(NORMAL)`,
`BEGIN IMMEDIATE`), real time, the default one-second poll, a temp dir on the machine's disk.

**`uptime`: the machine was close to idle, not idle.** The plan asks for a 1-minute load average
under 2 before and after. A build of the user's, unrelated to this work, held it near 7 to 12 when
the task started; the runs waited until it fell. Measured:

```
20:36  up 40 days, 12:40, 5 users, load averages: 6.06 11.89 10.52   (task start: not run)
20:43  up 40 days, 12:48, 5 users, load averages: 1.84 4.36 7.09     (before the first BenchmarkJobs run, superseded)
20:45  up 40 days, 12:49, 5 users, load averages: 1.76 3.62 6.49     (before the BenchmarkJobs run below)
20:46  up 40 days, 12:51, 5 users, load averages: 2.18 3.40 6.16     (after it)
20:47  up 40 days, 12:51, 5 users, load averages: 2.00 3.32 6.10     (before the alder bench run below)
20:47  up 40 days, 12:51, 5 users, load averages: 2.00 3.32 6.10     (after it)
```

The 1-minute figure was 1.76 before the benchmark, 2.18 after it (the benchmark's own load is part
of that), and 2.00 around `alder bench`. The 5- and 15-minute figures were still high from the
other build (3.3 and 6.1), so the machine was not idle: treat the absolute figures as pessimistic.
The shape (the ratios between rows, no failures) does not depend on that.

## What `BenchmarkJobs` does

```
go test ./rt/jobs -run '^$' -bench BenchmarkJobs -benchtime=2000x -count 5 -timeout 20m
```

`b.N` = 2000 queued no-op jobs, each inserting one row; the table is filled before the timer
starts. A "request writer" is a goroutine in a tight loop: `BEGIN IMMEDIATE`, one insert, commit.

- `claim-finish/workers=W`: W workers drain the queue, nothing else running.
- `with-requests/workers=4/writers=R`: 4 workers drain the queue while R request writers run until it
  is empty.
- `requests-only/writers=R`: the same writers with no workers, for a fixed 2 s window (the baseline).

`jobs/s` = jobs / the time the drain took. `req-writes/s`, `req-p50`, `req-p99`, `req-p99.9` and
`req-max` are over the request writers' own commits (the time from `Begin` to the end of `Commit`).
The medians of 5 runs (`-count 5`); `req-failures` counts a request write that returned an error
(SQLITE_BUSY included): it was 0 in all 25 runs.

| benchmark | jobs/s | req writes/s | req p50 ms | req p99 ms | req p99.9 ms | req max ms | req failures |
|---|---:|---:|---:|---:|---:|---:|---:|
| claim-finish, workers=1 | 2,813 | | | | | | |
| claim-finish, workers=4 | 2,788 | | | | | | |
| claim-finish, workers=8 | 2,466 | | | | | | |
| with-requests, 4 workers, writers=4 | 1,580 | 21,930 | 0.016 | 0.049 | 1.26 | 1,434 | 0 |
| with-requests, 4 workers, writers=16 | 1,600 | 21,020 | 0.016 | 0.051 | 80.5 | 1,939 | 0 |
| requests-only, writers=4 | | 45,750 | 0.016 | 0.051 | 2.20 | 1,630 | 0 |
| requests-only, writers=16 | | 45,900 | 0.016 | 0.060 | 33.4 | 2,030 | 0 |

Raw runs: the 5 `jobs/s` of `claim-finish/workers=4` were 2,464 / 2,631 / 2,788 / 2,794 / 2,799;
of `with-requests/workers=4/writers=4`, 1,075 / 1,694 / 1,580 / 2,653 / 1,280 (the spread is the
load and the lock's fairness, below).

## `alder bench testdata/bench/rows -run 'add 1000|enqueue 1000' --count 10`

The new fixture row `enqueue 1000` submits a form whose route inserts a row and `Job.enqueue`s (the
fixture gained a `Noop` job, a second form on the page, and a route `Enqueue`). Server only, medians
of 10, as the baseline file's tables.

| test | step | ms | bytes | alloc | req | loads | steps | sql |
|---|---|---:|---:|---|---:|---:|---:|---:|
| add 1000 | POST /rows | 10.22 | 971.0 kB | 25.85 MB / 125.9k | 3 | 2 | 5 | 3 |
| enqueue 1000 | POST /rows/enqueue | 10.38 | 971.0 kB | 25.85 MB / 125.9k | 3 | 2 | 6 | 4 |

(The page grew by about 300 bytes with the second form: `GET /` is 974.3 kB, against 973.8 kB in the
2026-10 baseline's routes table; the other rows' bytes in that file moved by that much.)

An enqueue costs one more effect step and one more statement (the `INSERT` into `_alder_jobs`) and
0.16 ms (1.6%) on a 10 ms request that renders a 1,000-row page twice; allocations and bytes are
unchanged to the shown digits. The ms difference is inside the run-to-run noise at this load (the
two `GET /` rows, identical work, differ by 0.02 ms here and by more in the first run).

## What the numbers say

**Throughput.** One worker claims and finishes about 2,800 jobs a second on this machine (about 0.36
ms a job: a claim transaction and a run-and-mark transaction, each a commit on a WAL file), and four
workers do no better (2,790), eight a little worse (2,470): SQLite has one writer, so more workers
queue on the lock rather than run in parallel. With a request load of tight-loop writers on the same
database, four workers drain about 1,600 jobs a second, and the writers' throughput falls from about
45,800 to about 21,500 writes a second: the jobs take roughly half the lock. These are for a job
that writes one row and nothing else; a job that does work outside the database is not limited by
the lock, which is why the default is four workers and not one. For the polling and workers
questions in §6a: one-second polling is not a limit, since a worker that finds a row claims again
at once and waits one second (or for `Wake`) only when the table has nothing due; a queue of 2,000
due rows drained in about a second and a half at four workers under load. Four workers keep up with
any request rate this machine can write (the writers reached 46,000 writes a second and the jobs
still ran at 1,600 a second beside them), so the queue is bounded by the requests' own write rate,
not by the workers.

**Request p99.** With the jobs draining, the request writers' p99 is 0.049 and 0.051 ms against
0.051 and 0.060 ms with no jobs at all (4 and 16 writers): no rise, inside the noise. p50 is 0.016
ms in all four rows. No request write failed in 25 runs (`req-failures` 0): there was no SQLITE_BUSY
in the request writers. §6a's and §7's claims (a job worker's transaction does not push a request
into SQLITE_BUSY, and a request's p99 stays where it was) hold for this load.

**A caveat the p99 hides, and is not the jobs'.** The writers are closed-loop (no think time), so
SQLite's busy handler is unfair to them: one writer can take the lock again and again (p50 is 0.016
ms, a commit with no wait) while the others sleep in `busy_timeout`; the starved commits are too few
to reach the p99 and show only in the tail. The maximum is 1.4 to 2.0 seconds in every row, with
the jobs and without them (requests-only: 1.6 and 2.0 s), so it is the writers' own contention, not
the jobs', and it is under the 5 s `busy_timeout`. p99.9 is higher with 16 writers (33 ms without
jobs, 80 ms with) and was 830 to 1,440 ms in one run of each with-requests row: with the jobs
adding writers to the lock, the unfair tail is a little heavier, which these 25 runs cannot size
(the load was not idle). A real request does not loop back to back, so this saturation is a
stress test, not a prediction; it does say that a deployment whose requests write at the lock's
capacity will see multi-second waits with or without jobs.

Book chapter 17 (Task 21) quotes this file's single-worker number (about 2,800 jobs a second for a
job that inserts one row, at a load near 2), not River's.
