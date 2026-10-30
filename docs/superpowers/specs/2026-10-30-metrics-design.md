# Metrics, a slow-request threshold, and `alder logs --since`

Builds on #93 (branch `issue-56-observability`: `rt/applog`, `web.Options.Access`, `alder logs
-n/--grep`). Design for [tapegram/alder#92](https://github.com/tapegram/alder/issues/92), the
follow-up #56 deferred. Everything marked **(call)** is the author's decision, not the user's.

## Intent

An operator who can read an access line can also ask "how many, how slow, by which page" without
grepping: a served app exposes Prometheus text at `/_alder/metrics`, off unless the environment
turns it on, and never public by accident. A request that takes longer than a threshold the
operator sets is a warn line that says where the time went, even when access lines are filtered
away. And `alder logs --since` reaches back further than `-n` on every target that can.

## Goals

1. `ALDER_METRICS` turns on `GET /_alder/metrics`: request count and latency by route pattern,
   in-flight requests, database statements, effect steps, Go runtime basics, the build id.
2. Every label value comes from a finite set the program fixes; no series is ever made from a
   path, query, header, parameter or session value.
3. `ALDER_SLOW_MS=N` promotes the access line of a request slower than N ms to warn, with its
   route, effect steps and database count/time.
4. `alder logs <env> --since <time>` on Render (the API's time-ranged read), Fly and Ssh (what
   they keep, filtered by each line's stamp).
5. Off by default, every piece: an app that sets nothing serves and logs exactly as #93 left it.

## Non-goals

- An OpenTelemetry SDK, OTLP push, traces, exemplars, a pushgateway. See "Decisions left to the
  user".
- A per-statement latency histogram, per-effect-step timing, response bytes, a per-route error
  rate (derivable from the status class). Phase 2 if wanted.
- Fly's `[metrics]` block in the generated `fly.toml`, a Render scrape setup, a Grafana board.
- `route` on every access line. The slow line carries it; the masked path is on the rest.

## Anti-goals

- A series an attacker can grow: the route label is the matched pattern or one of four fixed
  names, the method one of four, the status one of five, the effect name a Call name from the
  generated code.
- A cost when it is off: with `ALDER_METRICS` unset and `ALDER_SLOW_MS` unset, no statement is
  timed, no `time.Now` is added, no allocation is added to a request.
- A scrape that reads a session, sets a cookie, runs the CSRF check, shows in the dev panel or
  writes an access line when it is answered 200 (it is a health check's kind of request).
- A metrics endpoint that is on without the operator writing a value into the environment.

## Decisions left to the user

- **OpenTelemetry.** The import rule (stdlib + `alder/*` + modernc) keeps the OTel SDK out.
  Prometheus text (`text/plain; version=0.0.4`) is written by hand, ~80 lines of stdlib Go, and
  is what the OTel Collector's `prometheus` receiver, Prometheus, Grafana Agent, Datadog and
  Fly's own scraper read **(call)**. Hand-written OTLP/HTTP JSON push is possible later (~200
  lines plus endpoint/header env, batching, retries) but has no consumer today; it is not done
  unless the rule changes or one appears.
- Whether `alder deploy` for Fly should write `[metrics] port/path` into `fly.toml` when
  `ALDER_METRICS=open` is in the environment (Fly's scraper sends no header).
- Whether the two database counters should become a per-statement histogram.

## The package: `rt/metrics` (new, stdlib-only)

```go
type Collector struct{ … }                     // safe for concurrent use; one sync.Mutex (call)
func New(token string) *Collector               // token "": no token (open)
func FromEnv() (*Collector, error)              // ALDER_METRICS; nil, nil when unset or empty
func (c *Collector) Authorized(r *http.Request) bool // open, or Authorization: Bearer <token> (constant time)
func (c *Collector) InFlight(delta int64)
func (c *Collector) Request(route, method string, status int, d time.Duration)
func (c *Collector) Steps(names []string)       // one count per name
func (c *Collector) Statements(s *eff.Stats)    // adds the request's exec/query counts and time
func (c *Collector) Write(w io.Writer, build string) // the exposition, metrics and series sorted
```

- `FromEnv`: `ALDER_METRICS=open` → `New("")`; any other value is a token and must be at least 16
  bytes after trimming, else `ALDER_METRICS: want "open" or a token of at least 16 characters`
  (the value is never echoed) **(call)**; the generated main prints `alder: <err>` and exits 1.
- Labels: `route` is the matched `Route.Pattern` (`/todos/{id}`), or `/static/` (a static file),
  `/_alder/` (a runtime file), `/_dev`, `(none)` (no route matched, or the path did not parse)
  **(call)**. `method` is `GET`, `POST`, `HEAD` or `other`. `status` is `1xx`…`5xx`; 0 (nothing
  written) is `2xx`. Label values are escaped per the text format (`\\`, `\"`, `\n`).
- The metrics, all prefixed `alder_` **(call)**:

| name | type | labels |
|---|---|---|
| `alder_http_requests_total` | counter | route, method, status |
| `alder_http_request_duration_seconds` | histogram, buckets .005 .01 .025 .05 .1 .25 .5 1 2.5 5 10 | route, method |
| `alder_http_requests_in_flight` | gauge | |
| `alder_db_statements_total` | counter | kind = `exec` \| `query` (QueryRow is a query) |
| `alder_db_statement_seconds_total` | counter | kind |
| `alder_effect_steps_total` | counter | name (every entry of `Env.Trace`, Notes included) |
| `alder_go_goroutines`, `alder_go_heap_bytes`, `alder_go_gc_cycles_total` | gauge/gauge/counter, read with `runtime/metrics` at scrape time | |
| `alder_process_start_time_seconds` | gauge, set by `New` | |
| `alder_build_info` | gauge, value 1 | build |

- `Write` emits `# HELP` and `# TYPE` for each, metrics in the order above, series sorted by
  label values, histogram `_bucket` (with `+Inf`), `_sum`, `_count`.

## The router: `rt/web`

- `Options` gains `Metrics *metrics.Collector` (nil: no endpoint, nothing counted) and `SlowAfter
  time.Duration` (0: no slow lines). `route` keeps `pattern string`; `tracker` gains `route
  string` (set in `serve` once `find` hits) and `stats eff.Stats` (by value: no allocation).
- `MetricsPath = alderPrefix + "metrics"`. In `ServeHTTP`, right after the health branch: a GET
  or HEAD of `MetricsPath` with `opts.Metrics` set is answered by `rt.metrics`: `Authorized` →
  200, `Content-Type: text/plain; version=0.0.4; charset=utf-8`, `Cache-Control: no-store`,
  `Write(w, opts.Build)` (headers only for HEAD); else 401 with `WWW-Authenticate: Bearer
  realm="alder"` and a plain body `unauthorized`. A 200 scrape writes no access line and is not
  counted; a 401 gets its access line (Info, not Quiet: a probe is worth seeing) **(call)**.
  With `opts.Metrics` nil the path falls through to routing as today (404).
- In-flight: when `opts.Metrics` is set and the request is neither the health check nor a
  scrape, `InFlight(+1)` before routing and `InFlight(-1)` in the deferred block.
- When `opts.Metrics != nil || opts.SlowAfter > 0`, `serve` sets `rq.Env.Stats = &tw.stats` on
  every Env the router makes (a redirect follow's too; `EnvFor`'s Env is left alone: a test's
  browser). Otherwise `Env.Stats` stays nil and nothing is timed.
- In the deferred block, after the access record is filled: `acc.Route = w.route` (`(none)` when
  unset), `acc.Stmts = w.stats.Exec + w.stats.Query`, `acc.DBDur = w.stats.ExecDur +
  w.stats.QueryDur`, `acc.Slow = opts.SlowAfter > 0 && acc.Dur >= opts.SlowAfter`; then
  `opts.Access`, then `Metrics.Request(route, method, status, dur)` and
  `Metrics.Statements(&w.stats)`. Where `tw.steps += len(rq.Env.Trace)` runs today,
  `Metrics.Steps(rq.Env.Trace)` runs too. The health check is never Slow and never counted.

## `rt/eff` and `rt/db`: statement counting

- `eff.Env` gains `Stats *Stats` (nil: no cost). `type Stats struct { Exec, Query int; ExecDur,
  QueryDur time.Duration }` with `func (s *Stats) Add(query bool, d time.Duration)`. Plain
  fields: a request's effects run on its goroutine (`rt/eff` starts none).
- `rt/db`'s `recQ` times a statement when `EnvFrom(ctx)` has a `Rec` or a `Stats`, and adds it
  to each that is set; the `Rec == nil && Stats == nil` path is unchanged.

## The slow line: `rt/applog`

- `Access` gains `Route string`, `Stmts int`, `DBDur time.Duration`, `Slow bool`.
- `Logger.Access` writes a Slow record at **Warn**, Quiet or not, so it passes an `ALDER_LOG=warn`
  filter; the line is the access line with its breakdown appended **(call)**: text

  `2026/10/30 12:00:00 warn: GET /todos/3 200 812.4ms 4821B rid=5f3a2c1e slow route=/todos/{id} steps=12 db=9/640.1ms`

  (`steps=` whatever the level; `db=<statements>/<ms with one decimal>`); JSON: the usual keys
  at level `WARN` plus `"slow":true,"route":"/todos/{id}","steps":12,"db_stmts":9,"db_ms":640.1`.
  A record that is not Slow is written exactly as #93 does (no new keys, no `route`).
- `func SlowFromEnv() (time.Duration, error)`: `ALDER_SLOW_MS` unset, empty or `0` → 0; a
  positive whole number → that many milliseconds; anything else (`abc`, `-1`, `1.5`, `250ms`) →
  `ALDER_SLOW_MS=<v>: want a whole number of milliseconds (0: off)`.

## The generated main and Serve: `internal/gen/caps.go`

- `app.Options` gains `Metrics *metrics.Collector` and `SlowAfter time.Duration`; `Serve` copies
  both to the web options. `metrics` joins the generator's reserved local names.
- `main`, after `applog.FromEnv`: `m, err := metrics.FromEnv()` and `slow, err :=
  applog.SlowFromEnv()`, each a "bad environment: `alder: <err>`, exit 1" like `ALDER_LOG`.
- The `Main` doc comment and usage text name `ALDER_METRICS` and `ALDER_SLOW_MS` beside
  `ALDER_LOG`. Goldens regenerate.
- Deploys leave both as the environment sets them (Render's dashboard, `fly secrets set`, the
  box's `$R/env`), like `ALDER_LOG`; nothing in `internal/deploy` changes.

## `alder logs --since`: `cmd/alder/deploy.go`, `internal/deploy`

```text
alder logs <env> [dir] [--host name] [-f] [-n N] [--since T] [--grep PATTERN]
```

- `T` is a Go duration (`30m`, `2h`, `48h`: that long ago), an RFC 3339 instant, or `YYYY-MM-DD`
  (UTC midnight) **(call)**; else `alder logs: --since: want a duration (2h), an RFC 3339 time
  or a date`, exit 2 with the usage. `--since` and `-n` together are refused: `alder logs: -n
  and --since do not combine: --since prints every line since T` **(call)**. `-f` after
  `--since` follows on, as today; `--grep` applies after the since filter.
- `Target.Logs(ctx, follow bool, n int, since time.Time, w io.Writer)`: `n == 0` means all
  (the fake and the three targets). cmd/alder sends `n = 0` with a since.
- **Render**: `API.Logs(ctx, owner, svc, RenderLogMax, since)` paged forward with the returned
  next start while a page is full and next advances, at most `RenderSincePages = 20` (2000
  lines); when the cap is hit a stderr note `alder logs: Render gave 2000 lines since <T>; the
  rest is cut: use a later --since`. Follow then streams as today.
- **Fly**: `flyctl logs` has no time flag; the target sends all of `--no-tail`'s output and the
  client filter keeps the lines since. The book says Fly keeps only recent lines.
- **Ssh**: with `n == 0` the script runs `cat` instead of `tail -n` on `builds/<id>/log` (the
  app's stderr, stamped UTC by applog); the client filter keeps the lines since.
- The filter is `sinceWriter`, a line-buffered writer like `grepWriter`, chained before it. It
  reads a stamp at the line's start, after an optional Render `[instance] ` prefix: applog's text
  stamp `2006/01/02 15:04:05` (UTC), applog's JSON `{"time":"<RFC 3339>"`, or an RFC 3339 stamp
  (flyctl's). A line with no stamp (a tab-indented continuation, `listening on`, flyctl's own
  text, `--- alder logs: build … is current`) takes the last stamped line's verdict; before any
  stamped line it is dropped **(call)**.

## Where things change

| layer | files |
|---|---|
| new package | `rt/metrics/metrics.go` (Collector, labels, exposition), `env.go` (FromEnv, Authorized); `metrics_test.go` |
| effects | `rt/eff/eff.go` (`Env.Stats`, `Stats`); `rt/db/sqlite.go` (`recQ` adds to Stats) |
| log | `rt/applog/applog.go` (Access fields, the slow line, `SlowFromEnv`) |
| router | `rt/web/router.go` (Options.Metrics/SlowAfter, `MetricsPath`, `rt.metrics`, route pattern, stats, in-flight, the deferred block) |
| generated code | `internal/gen/caps.go` (Options, Serve, main, docs, reserved names); goldens under `internal/gen/testdata` and `cmd/alder/testdata` |
| CLI | `cmd/alder/deploy.go` (`--since`, `sinceWriter`, the `-n` refusal), `help.go`/`helpjson.go`; `internal/deploy/target.go`, `render.go` (paging, `RenderSincePages`), `fly.go`, `ssh.go` (`cat`), `deploy_test.go` fake |
| book | `16-deploy.md`: "Operating an app" gains "Slow requests" (the knob, the line, what `db=` counts) and "Metrics" (the knob, `open` vs a token and where each fits, the metric table, the cardinality rule, a `curl -H 'Authorization: Bearer …'` example, what a 200 scrape does not do); "Reading them back" gains `--since` and its per-target reach; the knobs table gains `ALDER_METRICS`, `ALDER_SLOW_MS`. `11-tooling.md`: the env table (two rows), the `alder logs` usage line (both). `book/site/build.py` regenerates |

## Performance

- `BenchmarkRequest` (`-benchmem`) in `rt/metrics` pins 0 allocations for a known route (struct
  map key, no string building); the result goes in the PR description.
- `TestAccessLog_FilteredNoCost` (rt/web) is extended: with Metrics nil and SlowAfter 0, the
  per-request allocation count is what #93 pinned and `Env.Stats` is nil.
- `Write` holds the mutex while it formats; a scrape at 15 s intervals is nothing.

## Verification: tests the builder adds

`rt/metrics`: `TestFromEnv` (unset → nil; `open`; a 15-byte token → the error text, which does
not contain the value; a 16-byte token kept); `TestAuthorized` (open; right, wrong, missing,
`Basic` scheme); `TestCollector_Exposition` (a fixed sequence → the exact text, HELP/TYPE lines,
sorted series, `+Inf`, `_sum`, `_count`); `TestCollector_StatusClass` (0, 101, 204, 303, 404,
500); `TestCollector_MethodOther` (`PUT`); `TestCollector_HistogramBuckets` (30 ms lands in
`0.05` and above, not `0.025`); `TestCollector_LabelEscape`; `TestCollector_Steps`;
`TestCollector_Statements`; `TestCollector_Runtime` (goroutines ≥ 1, start time within a minute
of now, `alder_build_info{build="b-1"} 1`); `BenchmarkRequest`.

`rt/applog`: `TestText_AccessSlow` (the exact line); `TestJSON_AccessSlow` (the five keys and
their types; absent on a non-slow line); `TestAccess_SlowAtWarn` (written under `Warn`, Quiet
too); `TestSlowFromEnv` (unset, `0`, `250`, `abc`, `-1`, `1.5`, `250ms`).

`rt/eff`: `TestStats_Add`. `rt/db`: `TestStats_Counted` (an exec and a query with `Stats` set:
counts 1/1, durations > 0; with neither Rec nor Stats: untouched).

`rt/web`: `TestMetrics_Off` (nil Collector: 404, access-logged like any 404);
`TestMetrics_Scrape` (open: 200, content type, after one page GET the line
`alder_http_requests_total{route="/",method="GET",status="2xx"} 1`; the scrape leaves no access
record; HEAD: headers, empty body); `TestMetrics_Token` (missing → 401 + `WWW-Authenticate`, an
access record with status 401; wrong → 401; right → 200); `TestMetrics_Route404` (`(none)`);
`TestMetrics_FileRoutes` (`/static/`, `/_alder/`); `TestMetrics_InFlight` (a handler parked on a
channel: a scrape from another goroutine reads 1; 0 after); `TestMetrics_Effects` (a page running
`Db.all Todo` twice → `alder_effect_steps_total{name="Db.all Todo"} 2`);
`TestMetrics_Statements` (exec and query counts, seconds > 0); `TestMetrics_NoSessionNoPanel`
(no `Set-Cookie`, nothing in the dev panel's request list); `TestSlow_Promoted` (SlowAfter 1 ms, a
handler sleeping 5 ms: the record has Slow, Route, Stmts ≥ 1 after a Db step); `TestSlow_Off`
(0: never Slow, `Env.Stats` nil); `TestSlow_HealthNever`; the extended
`TestAccessLog_FilteredNoCost`.

`internal/gen` / `cmd/alder` (subtests of `TestGen_MainLog`, one built app): `BadALDER_METRICS`
(exit 1, the error text); `BadALDER_SLOW_MS`; `MetricsOffByDefault` (404); `MetricsOpen` (200,
`alder_build_info`); `MetricsToken` (401 without, 200 with); `SlowLine` (`ALDER_SLOW_MS=1`, a page
with `Task.sleep 20`: one `warn:` line with `slow route=`). `TestLogs_Since` (the fake receives
the parsed time; a duration is relative to an injected `logsNow`), `TestLogs_SinceBad`,
`TestLogs_SinceAndN` (the refusal), `TestSinceWriter` (text, JSON and flyctl stamps, the Render
prefix, a continuation follows its line, an unstamped line before any stamp is dropped),
`TestLogs_RenderSincePages` (three pages, stops on a short page; the cap note),
`TestLogs_FlySince` (fake flyctl output filtered), `TestSsh_LogsSince` (loopback: `cat`, every
line reaches the filter). `helpjson_test.go`'s flag check covers `--since`; the book's usage
lines (chapters 11 and 16) stay equal to `logsUsage`. `go vet ./...` clean, `go test ./...`
green, `rt/embed_test.go`'s dependency check passes (no new module).
