# Observability: access log, log levels, and reading them

Design for [tapegram/alder#56](https://github.com/tapegram/alder/issues/56). A healthy Alder app
under traffic writes nothing, so Render's empty log viewer for nightvote-replay could not be told
from a dead service; and the lines it does write on an error carry the URL unmasked
(nightvote-replay `docs/alder-dev-log.md:89`). Everything marked **(call)** is the author's
decision, not the user's.

## Intent

One line per request, safe to keep forever, readable raw in a platform's viewer and filterable
by level; the error lines that exist today masked the same way and tied to their request by an
id; a level knob so a quiet production app and a chatty debugging session are the same binary;
and `alder logs` able to narrow what it prints. Metrics are not the fix for "is it alive": the
access line is.

## Goals

1. A served app logs every request it routes: method, masked path and query, status, duration,
   bytes, request id. Health checks are silent; files are quiet when healthy.
2. `ALDER_LOG=debug|info|warn|error` filters everything the runtime and the app (`Log.*`) write;
   `ALDER_LOG_FORMAT=text|json` picks the line shape.
3. No URL reaches the log with a secret in it; no cookie, header, body, form value or session
   ever does.
4. Error lines about a request carry its id, and the id is answered as `X-Request-Id`.
5. `alder logs` takes `-n` and `--grep` on every target.
6. The book's chapter 16 says what is logged and how to read it.

## Non-goals

- `/_alder/metrics`, OpenTelemetry, a slow-request threshold, `--since` for `alder logs`. Deferred
  to a follow-up issue the builder opens, naming `Options.Access` (below) as the seam a collector
  hangs on and `Render.API.Logs(start)` as what `--since` would use **(call)**: nothing scrapes an
  Alder app today, Fly and Ssh have no time-ranged log read, and the real case is the empty log.
- Per-request SQL timing in the log. The dev panel's SQL tab has it; it needs the Recorder, which
  costs on every request.
- A TTY check. The format is chosen by the environment only.

## Anti-goals

- A hot path that formats a line nobody will see: a filtered line allocates nothing.
- A knob that changes the dev panel. It keeps every tab as it is.
- A log line shape that differs between the runtime's lines and the app's `Log.*` lines.

## The package: `rt/applog` (new, stdlib-only)

```go
type Level int // Debug, Info, Warn, Error; ParseLevel("debug"|"info"|"warn"|"error")
type Format int // Text, JSON

type Logger struct{ … }                      // safe for concurrent use; one Write per line
func New(w io.Writer, min Level, f Format) *Logger
func FromEnv(w io.Writer) (*Logger, error)   // ALDER_LOG (default info), ALDER_LOG_FORMAT (default text)
func (l *Logger) Log(level, msg string)      // the Env.Log hook: "effect" is Debug, "debug"/"info"/"warn"/"error" themselves
func (l *Logger) Access(a *Access)           // one request line
func (l *Logger) Enabled(Level) bool

type Access struct {
    ID           string        // the request id
    Method, Path string        // Path is already masked (MaskURL)
    Status       int           // 0: the handler wrote nothing
    Dur          time.Duration
    Bytes        int64
    Quiet        bool          // a file answered 2xx/304: logged at Debug, not Info
    Steps        int           // effect calls run (len(Env.Trace)); shown at Debug only
}

func MaskURL(u *url.URL) string   // path + "?" + query with sensitive values replaced
func Sensitive(name string) bool  // a parameter or field name that holds a secret
func LooksSecret(s string) bool   // moved from internal/check/deploy.go (the checker imports it)
func NewID() string               // 8 lowercase hex characters
func RequestID(r *http.Request) string // the inbound X-Request-Id when well-formed, else NewID()
```

- `FromEnv`: an unknown `ALDER_LOG` or `ALDER_LOG_FORMAT` is an error (`ALDER_LOG=verbose: want
  debug, info, warn or error`); the generated main prints `alder: <err>` and exits 1 before
  serving, as PR #77 does for a bad `ALDER_PUBLIC_URL`. Values are case-insensitive, trimmed.
- `Log` with an unknown level string logs it at Info with the level kept in the text (nothing is
  lost); the four known names and `effect` are the only ones the runtime produces.
- **Text line** **(call)**: `2026/10/29 16:00:00 <level>: <msg>` — the shape `log.Printf` gives
  today, so every existing line keeps its look and `alder run`'s terminal stays readable. An
  access line's msg is `GET /start?key=••••&seed=7 200 12.3ms 4821B rid=5f3a2c1e`; at Debug it
  ends ` steps=4`. The stamp is UTC, written with a fixed layout into a pooled buffer.
- **JSON line** **(call)**: one object per line through `log/slog`'s `JSONHandler` (stdlib, pooled
  buffers, correct escaping): `{"time":"2026-10-29T16:00:00.123Z","level":"INFO","msg":"GET
  /start?key=••••&seed=7","method":"GET","path":"/start?key=••••&seed=7","status":200,"ms":12.3,
  "bytes":4821,"rid":"5f3a2c1e"}`; plain lines are `{"time","level","msg"}`. A panic's stack stays
  inside `msg`. slog's level names (`DEBUG`…`ERROR`) and `time` key are kept as they are.
- `NewID` is `uint32(seed) + counter` in hex, seed from `crypto/rand` once per process: unique
  per process, cheap, not a secret. `RequestID` accepts an inbound `X-Request-Id` of 1–64 bytes
  of `[A-Za-z0-9._-]`, else makes one **(call)**: a platform's id (Fly's) correlates with its own
  logs; anything else is replaced, never echoed.

### Masking rule

`MaskURL` keeps the path's shape and the query's keys and order, and replaces a value with `••••`
(the dev panel's mask) when:

- the key is `Sensitive`: its lowercase form contains `key`, `token`, `secret`, `pass`, `auth`,
  `sig`, `code` or `session` **(call)**: `key` is nightvote-replay's case; the rest are what
  OAuth, reset links and signed URLs name theirs. A false positive hides a harmless value; a miss
  leaks a secret;
- or the value `LooksSecret` (a URL with a password, a known key prefix, 32+ base64/hex
  characters).

A path segment that `LooksSecret` is replaced too (nightvote-replay once carried its key in the
path). A query that does not parse is masked pair by pair on the raw string (split on `&`, `=`),
never dropped. `Sensitive` is also what the dev panel's `masked()` uses for form fields (it keeps
its own output). Headers, cookies, bodies and sessions are never passed to the log by the
runtime; the book says `Log.info` is the app's responsibility.

## The router: `rt/web`

- `Options.Access func(*applog.Access)`; nil: no access log (rt/testing's browser, every
  existing test). `Options.Log` stays `func(level, msg string)`.
- `tracker` gains `status int` (set by `WriteHeader`; a `Write` without one sets 200), `bytes
  int64` (summed in `Write`) and `id string`. `ServeHTTP` sets `id = applog.RequestID(r)`, puts it
  in the response header `X-Request-Id` before anything is written, and in a `defer` after the
  recover calls `Options.Access` with the filled record. `Dur` is `time.Since(w.start)`.
- Not logged: `HealthPath` (its comment already promises "no request log line") **(call)**.
  `Quiet` is set for `/static/…` and `/_alder/…` (the Datastar script) answered 2xx or 304, so a
  healthy asset is Debug and a missing one is Info; `/_dev` is Quiet when 200.
- A panic after the response started still produces the access line (status as written) and
  the error line; both carry the id.
- Every error and warn line that names a request (`fail`, `badRequest`, `maintenance`,
  navigation and patch write failures, the dropped-header warning) uses `applog.MaskURL(r.URL)`
  instead of `r.URL.Path` and ends its first line with ` rid=<id>` **(call)**: the id goes last so
  the `"error: GET /boom: "` prefix tests keep passing; `response_test.go:205`'s exact match is
  updated. The stack, when present, follows on the next lines as today.
- The dev panel's Request section gains a first line `id <rid>`; its bar is unchanged.

## Levels: what each shows

| level | runtime lines | app |
|---|---|---|
| error | panics (with stack), response/patch/navigation write failures, `the database: …` at start | `Log.error` |
| warn | 400/413/415, 503 maintenance, oversized cookie, dev-key warning, health flaps, rollback failures, a page's dropped header | `Log.warn` |
| info (default) | access lines (routes, files that fail), `listening on`, migrations, backup shipper, `the database is ahead` | `Log.info` |
| debug | access lines for healthy files, effect steps (today's `effect:` lines), `steps=` on access lines | `Log.debug` |

`ALDER_DEV=1` no longer implies effect lines **(call)**: the default is `info` everywhere, the dev
panel's Trace tab has every step, and `ALDER_LOG=debug alder run` brings the lines back (`alder
run` passes the environment through). The generated main's `level != "effect" || dev` test goes
away; filtering is the Logger's alone.

`stdlib/Log.alder` gains `debug : Text -> Task Unit` and `error : Text -> Task Unit` (`std.LogDebug`,
`std.LogError` through `logAt`) **(call)**: an app must be able to speak at the levels an operator
filters on.

## The generated main and Serve: `internal/gen/caps.go`

- `Options` gains `Access func(*applog.Access)`; `Serve` copies it to `w.Access` beside `w.Log`.
- `main` builds `lg, err := applog.FromEnv(os.Stderr)` right after the `publicurl.Err` check PR #77
  adds (both are "bad environment: exit 1 before serving"); passes `Log: lg.Log, Access:
  lg.Access`; the backup shipper gets `lg.Log` too. `log.Printf` leaves the generated main.
- The `Main` doc comment and the usage text name `ALDER_LOG` and `ALDER_LOG_FORMAT` next to
  `ALDER_PUBLIC_URL`. Goldens regenerate.

## `alder logs`: `cmd/alder/deploy.go`

```text
alder logs <env> [dir] [--host name] [-f] [-n N] [--grep PATTERN]
```

- `-n N` (default 100): the last N lines. Render asks `Tail(N)`; Ssh tails N; Fly's `flyctl logs
  --no-tail` has no count, so cmd/alder keeps the last N client side. `-f` is unchanged.
- `--grep PATTERN`: a Go regexp applied line by line to everything the target writes, implemented
  once in cmd/alder as a line-buffered `io.Writer` wrapping stdout, so all three targets and
  `-f` get it **(call)**. `--grep rid=5f3a2c1e` is the request-id filter; no separate flag.
- `Target.Logs` gains the count: `Logs(ctx, follow bool, n int, w io.Writer)`; the fake target
  and the three real ones take it.
- An environment file or Render/Fly environment may set `ALDER_LOG` and `ALDER_LOG_FORMAT`; the
  deploy does not touch them (unlike `ALDER_DEV`, which it refuses).

## Where things change

| layer | files |
|---|---|
| new package | `rt/applog/applog.go` (Logger, levels, formats), `mask.go` (MaskURL, Sensitive, LooksSecret), `id.go`; `applog_test.go`, `mask_test.go` |
| checker | `internal/check/deploy.go`: `looksSecret` → `applog.LooksSecret` |
| router | `rt/web/router.go` (Options.Access, tracker status/bytes/id, the deferred access call, masked error lines), `devpanel.go` (`masked` via `Sensitive`; `id` line) |
| effects | `rt/std/effects.go` (`LogDebug`, `LogError`), `stdlib/Log.alder`, `internal/gen/stdcall.go` |
| generated code | `internal/gen/caps.go` (Options.Access, Serve, main, docs); goldens under `internal/gen/testdata` and `cmd/alder/testdata` |
| CLI | `cmd/alder/deploy.go` (`-n`, `--grep`, the filtering writer), `help.go`/`helpjson.go` usage, `internal/deploy/target.go`, `render.go`, `fly.go`, `ssh.go`, `deploy_test.go` fake |
| book | `16-deploy.md`: a section "Operating an app" after "Status, rollback and logs" (what a line holds, the levels table, the masking rule, `X-Request-Id`, the JSON shape, `alder logs -n/--grep`, what `/_alder/health` means); the knobs table gains `ALDER_LOG`, `ALDER_LOG_FORMAT`. `11-tooling.md`: the env table (two rows), the "logs every effect step" sentence, the `alder logs` usage line. `07-effects.md`: `Log.debug`, `Log.error`. `13-compiler.md:1144` mentions the Debug level. `book/site/build.py` regenerates |

## Performance

- `Access` and `Log` return before formatting when the level is filtered. `TestLogger_FilteredAllocs`
  pins `testing.AllocsPerRun` at 0 for a Debug access line under Info.
- `BenchmarkAccess_Text` and `BenchmarkAccess_JSON` (`-benchmem`) go in the PR description. The text
  line is appended with `strconv.Append*` into a `sync.Pool` buffer and written once; the builder
  reports allocations and does not pin a number for the JSON path (slog's).
- `MaskURL` on a URL without a query and without a secret-looking segment returns `u.Path` without
  allocating (`TestMaskURL_PlainPathNoAlloc`).

## Verification: tests the builder adds

`rt/applog`: `TestParseLevel` (names, case, bad); `TestFromEnv_Defaults` (info, text), `_BadLevel`,
`_BadFormat` (error text); `TestLog_Filtered` (each level under each min); `TestLog_EffectIsDebug`;
`TestLog_UnknownLevelKept`; `TestText_Line` (exact stamp shape and `info: msg`); `TestText_Access`
(the exact line, `ms` with one decimal, `B` suffix, `rid=`, `steps=` at Debug only); `TestJSON_Access`
(parsed back: every key, number types); `TestJSON_StackInMsg`; `TestAccess_QuietIsDebug`;
`TestMaskURL` (`key=`, `token`, `access_token`, `Authorization`, a `sk_` value under a plain
name, a 40-hex path segment, keys and order kept, a plain `seed=7` untouched, a `%zz` query masked
pair-wise and not dropped); `TestSensitive`; `TestLooksSecret` (the checker's cases moved here);
`TestNewID_UniqueAndHex`; `TestRequestID` (accepted, too long, bad character, absent);
`TestLogger_FilteredAllocs`; the two benchmarks.

`rt/web`: `TestAccessLog_PageGET` (one call: method, masked path, 200, Dur > 0, Bytes = body
length, ID = the `X-Request-Id` header); `TestAccessLog_POSTRedirect` (303, bytes 0);
`TestAccessLog_NotFound` (404 at Info, not Quiet); `TestAccessLog_HealthSilent`;
`TestAccessLog_StaticQuiet` (200 Quiet, 404 not); `TestAccessLog_Panic` (500 line and the error
line share the rid; after-start panic keeps the written status); `TestAccessLog_IncomingID`
(`X-Request-Id: abc-1` echoed; `X-Request-Id: <65 chars>` replaced); `TestAccessLog_NilHook` (no
call, no header change beyond the id); `TestErrorLine_Masked` (`fail` on `GET /start?key=s3cret`
logs `GET /start?key=•••• … rid=`); `TestBadRequest_Masked`; `TestDevPanel_ShowsID`;
`TestDevPanel_MasksQuery`. Existing `response_test.go:205` and the `logs[0]` prefix tests are
updated for ` rid=`.

`rt/std`: `TestLog` extended to `debug` and `error`. `internal/check`: the `deploySecrets` corpus
case still passes.

`internal/gen` / `cmd/alder`: `TestGen_MainBadALDER_LOG` (exit 1, `alder: ALDER_LOG=verbose: want
…`); `TestGen_MainAccessLine` (a served app's stderr has one `info:` line for a page GET, none for
`/_alder/health`); `TestGen_MainJSON` (`ALDER_LOG_FORMAT=json`: the line parses, `status` 200);
`TestGen_MainWarnSilencesAccess`; `TestLogs_Grep` and `TestLogs_N` against the fake target;
`TestLogs_FlyLastN` (client-side tail); the Render and Ssh target tests gain the `n` argument.

`cmd/alder/helpjson_test.go`'s flag check covers the new `-n`/`--grep` in `logsUsage`; the book's
`alder logs` usage lines (chapters 11 and 16) are kept equal to it. `go vet ./...` clean, `go test
./...` green, `rt/embed_test.go`'s dependency check passes (no new module).
