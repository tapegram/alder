# Observability (#56): implementation plan

Spec: `docs/superpowers/specs/2026-10-29-observability-design.md` (binding). Stacked on #77
(`issue-9-absolute-urls`: `publicurl.Err` in the generated main). TDD: each task starts with its
failing tests, then code, then green, then one commit.

## Task 1: `rt/applog` masking (`mask.go`, `mask_test.go`)

- `Sensitive(name)`: lowercase contains key/token/secret/pass/auth/sig/code/session.
- `LooksSecret(s)`: moved verbatim from `internal/check/deploy.go`.
- `MaskURL(u)`: path segments that LooksSecret → `••••`; query pairs kept in order, value masked
  when the key is Sensitive or the value LooksSecret; an unparsable pair masked on the raw text
  (never dropped); a plain path with no query returns `u.Path` without allocating.
- Tests: `TestSensitive` (a table row per key word, mixed case, embedded: `access_token`,
  `X-Api-Key`, `Authorization`, `zipcode`; negatives `seed`, `page`, `q`), `TestLooksSecret`
  (URL with password, every prefix, 32 hex, 40 base64, 31 chars, paths, spaces),
  `TestMaskURL` (each key word, `sk_` under a plain name, a 40-hex path segment, order kept,
  `seed=7` untouched, `%zz` masked pairwise, `a;b`, a key without `=`, an empty value, repeated
  keys, escaped secret in the path), `TestMaskURL_PlainPathNoAlloc`.

## Task 2: `rt/applog` ids (`id.go`)

- `NewID()` 8 lowercase hex (crypto/rand seed + atomic counter); `RequestID(r)`.
- Tests: `TestNewID_UniqueAndHex`, `TestRequestID` (accepted, 64 ok, 65 replaced, bad char,
  empty, absent).

## Task 3: `rt/applog` Logger (`applog.go`, `applog_test.go`)

- Level, ParseLevel, Format, New, FromEnv, Log, Access, Enabled; text lines into a pooled
  buffer, written once under a mutex; JSON through `slog.JSONHandler` with UTC records.
- Tests: `TestParseLevel`, `TestFromEnv_Defaults`, `_BadLevel`, `_BadFormat`, `TestLog_Filtered`,
  `TestLog_EffectIsDebug`, `TestLog_UnknownLevelKept`, `TestText_Line`, `TestText_Access`,
  `TestJSON_Access`, `TestJSON_StackInMsg`, `TestAccess_QuietIsDebug`, `TestLogger_FilteredAllocs`,
  `TestLogger_ConcurrentLinesWhole`, `BenchmarkAccess_Text`, `BenchmarkAccess_JSON`.
- `rt/embed.go` lists the three files.

## Task 4: the checker uses `applog.LooksSecret`

- `internal/check/deploy.go`: delete `looksSecret`, call `applog.LooksSecret`. Corpus green.

## Task 5: `rt/web` access hook, request ids, masked error lines

- `Options.Access`; tracker `status`, `bytes`, `id`; X-Request-Id set first; deferred access
  call after recover; Health not logged; Quiet for `/static/`, `/_alder/` 2xx/304 and `/_dev` 200.
- Every request-naming error/warn line: `applog.MaskURL(r.URL)` and ` rid=<id>` ending the first
  line; 413/415 get a warn line.
- Dev panel: `masked` via `applog.Sensitive`; `id <rid>` first in Request; query masked in the
  Request line and the curl line.
- Tests (new `rt/web/access_test.go`): `TestAccessLog_PageGET`, `_POSTRedirect`, `_NotFound`,
  `_HealthSilent`, `_StaticQuiet`, `_Panic`, `_IncomingID`, `_NilHook`, `TestErrorLine_Masked`,
  `TestBadRequest_Masked`, `TestLogLines_NoHeadersCookiesBodies` (secrets planted in every header,
  cookie, form body and session: in no log line nor access record), `TestDevPanel_ShowsID`,
  `TestDevPanel_MasksQuery`; update `response_test.go` exact match and prefix tests.

## Task 6: `Log.debug`, `Log.error`

- `stdlib/Log.alder`, `rt/std/effects.go` (`LogDebug`, `LogError`), `internal/gen/stdcall.go`.
- `TestLog` extended.

## Task 7: the generated main and Serve

- `internal/gen/caps.go`: `Options.Access`, `Serve` copies it; main builds `applog.FromEnv` after
  the `publicurl.Err` check, exit 1 on error; Log/Access/backup from the Logger; docs name
  `ALDER_LOG`, `ALDER_LOG_FORMAT`; goldens regenerated (`go test ./internal/gen -update`).
- `cmd/alder/observability_test.go`: `TestGen_MainBadALDER_LOG`, `TestGen_MainAccessLine`,
  `TestGen_MainJSON`, `TestGen_MainWarnSilencesAccess` (one built app, subtests).

## Task 8: `alder logs -n N --grep PATTERN`

- `Target.Logs(ctx, follow, n, w)`: Render `Tail(min(n,100))`, Ssh `tail -n N`, Fly client-side
  last N; fake target updated.
- `cmd/alder/deploy.go`: flags, `grepWriter` (line-buffered, flushes the partial last line),
  usage text, help JSON.
- Tests: `TestLogs_Grep`, `TestLogs_N`, `TestLogs_BadFlags` (cmd/alder, Render fake),
  `TestLogs_FlyLastN` (internal/deploy, the fake flyctl), Render/Ssh tests with `n`,
  `TestGrepWriter` (split writes, partial line, no trailing newline).

## Task 9: book

- `16-deploy.md` "Operating an app"; knobs table rows. `11-tooling.md` env rows, effect sentence,
  logs usage. `07-effects.md` `Log.debug`/`Log.error`. `13-compiler.md` Debug level.
- `python3 book/site/build.py`; `cd experiments && python3 -m unittest`.

## Task 10: follow-up issue, verification, PR

- Open the follow-up issue (metrics/OTel/`--since`, naming `Options.Access` and
  `Render.API.Logs(start)`).
- `go vet ./...`, `go test ./...`, benchmarks for the PR body; push; PR `--base issue-9-absolute-urls`.
