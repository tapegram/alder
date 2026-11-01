# alder-tui, local milestone (PRs 1–4) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `alder-tui`, run in an app directory, owns or attaches to `alder run` and shows build state, requests (with their steps, SQL and logs), logs, data, tests and routes live, backed only by machine interfaces `alder` gains in the same milestone.

**Architecture:** PRs 1–2 add machine interfaces to `alder`: the dev server always runs the child at debug with JSON logs, renders them back to today's text, and serves every line and lifecycle change as an SSE feed (`/_dev/events`); the app serves its per-request record (`/_dev/requests`) and its map (`/_dev/map.json`). PRs 3–4 add a nested Go module `tui/` (`module alder/tui`) with Bubble Tea screens that read those feeds and run `alder … --json` through one runner interface, tested with `teatest` golden frames against hand-written feed fixtures.

**Tech Stack:** Go 1.27; `alder` stays stdlib + `alder/*` + modernc. TUI: Bubble Tea, Lip Gloss, Bubbles (current stable majors; record them in `tui/go.mod`), `teatest` (test only), `modernc.org/sqlite` (same version as the root `go.mod`, v1.60.1). File watching in the TUI is a 300 ms poll (no fsnotify).

**Spec:** `docs/superpowers/specs/2026-11-01-alder-tui-design.md` (with `…-survey.md` for file:line references). Read both. PRs 5–6 (remote) get their own plan.

## Global Constraints

- No new dependency in the root `go.mod`; `TestDistModule` must stay green. Charm lives only in `tui/go.mod`.
- The TUI never parses human-readable `alder` output. Every feed is JSON or a documented file. If something is only in text, fix `alder`.
- Nothing is TUI-only: every screen's footer names the real `alder`/`curl`/`sqlite3` command giving the same data.
- Dev endpoints and the request ring exist only under `ALDER_DEV=1` (`/_dev/requests` and the ring: the dev panel's condition, `rt.panelOn()`; `/_dev/map.json`: `rt.devOn()`, as `/_dev`). Production behaviour and allocations per request are unchanged except where a task says so.
- `alder run`'s terminal output for a person must look as it does today (same text, same colours) whatever the child's real level and format.
- Feed event shape: every event is one JSON object with `type`, `t` (RFC3339Nano, UTC) and `seq` (int, from 1, counting since the dev server started). Ring of the last **5 000** events. `: ping` every **15 s**.
- Request ring: last **200** records. TUI log scrollback: **20 000** lines.
- Child under the dev server: `ALDER_LOG=debug`, `ALDER_LOG_FORMAT=json`, always. The dev server prints at the user's `ALDER_LOG` (default info) and passes JSON through only when the user's own environment set `ALDER_LOG_FORMAT=json`.
- Look: 16 ANSI colours only. `ok` green; `FAIL`/`error` bold red; `warning`/`SKIPPED` yellow; `fix:` cyan; stamps and `alder run:` lines dim; headings bold. Status classes: 2xx plain, 3xx dim, 4xx yellow, 5xx bold red. One rule under the header, two-space gutters, borders only between panes. Minimum 80×24; below 80 columns the Request detail stacks its sections.
- Dispatch message, verbatim: `alder tui: alder-tui is not installed; brew upgrade alder, or download it from the release that matches alder <version>`
- macOS and Linux are the targets; Windows must compile, nothing more.
- Comments in this repo describe behaviour in full sentences, as the surrounding code does. Follow the house style of the file you touch.

## Review Focus

1. **Huge lines.** A panic stack or a 1 MiB log message from the child must pass through the dev server's log pipe whole, as one event and one terminal line, without blocking the child (no `bufio.Scanner` default limit). Test in Task 3.
2. **Dev server restarted under an attached TUI.** A new `alder run` restarts `seq` at 1; a client resuming with `?after=900` must get the full ring, and the TUI must notice the new `boot` in `hello` and reset its stores instead of showing stale requests. Tests in Tasks 2 and 12.
3. **TUI killed hard while it owns `alder run`.** SIGKILL of the TUI must not leave `alder run` (and its app) holding port 8080. `alder run` honours `ALDER_EXIT_ON_STDIN_EOF=1` like the app does, and the TUI starts it with a stdin pipe. Tests in Tasks 4 and 13.
4. **The address is taken by something that is not `alder run`.** A probe of `/_dev/events` answering 404 or HTML must make the TUI say `<addr> is serving something that is not alder run; pass --addr` and stop, not start a second `alder run` that fails to listen. Test in Task 13.
5. **Data screen against a changing database.** A table in the map that is missing from `dev.db` (a rename between rebuilds) or a database locked mid-migration shows an error row for that table; it never crashes the TUI or blocks input. Test in Task 20.

---

# PR 1 — alder: the feed

Branch `tui-feed` from main. Tasks 1–5. No visible change for `alder run` users.

### Task 1: Render a JSON log record as its text line

**Files:**
- Create: `rt/applog/render.go`
- Test: `rt/applog/render_test.go`

**Interfaces:**
- Produces: `func Render(line []byte, min Level) (text []byte, ok bool)` in package `applog`. `ok` is false when `line` is not a record this package writes (not a JSON object, or lacking `time`, `level`, `msg`); the caller then passes the line through. `ok` true with `text == nil` means the record is below `min` and is dropped. Otherwise `text` is exactly what a text `Logger` at level `min` would have written for the same call, newline included.

- [ ] **Step 1: Write the failing round-trip test**

`TestRender_MatchesTextLogger`: for each `min` in Debug, Info, Warn, Error, and for each call in a table, run the same call against `New(&txt, min, Text)` and `New(&js, Debug, JSON)`, both with `now` fixed to `2026-11-01T14:02:11.204Z` (set the unexported `now` field). Then assert `Render(js line, min)` equals the text output, or is `(nil, true)` when the text logger wrote nothing. The calls:

```go
lg.Log("info", "hello")
lg.Log("debug", "detail")
lg.Log("warn", "careful\nsecond line")    // continuation becomes "\n\t"
lg.Log("error", "bad \x1b[31m")           // control char escaped
lg.Log("effect", "Db.all Todo")           // "effect " prefix, Debug
lg.Log("audit", "custom level")           // "audit: " prefix, Info
lg.Access(&Access{ID: "5f3a2c1e", Method: "GET", Path: "/start?key=••••", Status: 200, Dur: 12300 * time.Microsecond, Bytes: 4821, Steps: 3})
lg.Access(&Access{ID: "a1", Method: "GET", Path: "/static/x.css", Status: 200, Quiet: true})
lg.Access(&Access{ID: "b2", Method: "GET", Path: "/todos/3", Status: 200, Dur: 812400 * time.Microsecond, Bytes: 4821, Steps: 12, Slow: true, Route: "/todos/{id}", Stmts: 9, DBDur: 640100 * time.Microsecond})
```

`TestRender_NotARecord`: `Render([]byte("panic: boom"), Info)`, `Render([]byte(`{"a":1}`), Info)` and `Render([]byte(`[1]`), Info)` all return `ok == false`.

`TestRender_IgnoresUnknownKeys`: a record with an extra `"rid":"x"` key (Task 6 adds it to `Log` records) renders to the same text as without it.

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./rt/applog -run 'TestRender' -v`
Expected: FAIL, `undefined: Render`.

- [ ] **Step 3: Implement `Render` in `rt/applog/render.go`**

Decode into a struct with `Time time.Time`, `Level string` (slog's `DEBUG`/`INFO`/`WARN`/`ERROR`), `Msg string` and the access keys as pointers (`Method, Path *string; Status *int; Ms *float64; Bytes *int64; Rid *string; Steps *int; Slow *bool; Route *string; DBStmts *int; DBMs *float64`). A record is an access record when `rid`, `status`, `method` and `path` are all present. Build the text with the existing `start`-style stamp (`stampLayout`, UTC) and `appendText`, mirroring `Access` and `Log` byte for byte: `steps=` only when `min == Debug` or slow, the slow breakdown whatever the level. Reuse `tenths`-formatted values as `strconv.AppendFloat(…, 'f', 1, 64)`. Refactor `Access`'s text branch into a helper both share if that removes duplication; the round-trip test guards it.

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./rt/applog -v`
Expected: PASS, all existing tests too.

- [ ] **Step 5: Commit**

```bash
git add rt/applog/render.go rt/applog/render_test.go rt/applog/applog.go
git commit -m "applog: Render, a JSON record as its text line (#120)"
```

### Task 2: The event hub and `GET /_dev/events`

**Files:**
- Create: `cmd/alder/run_events.go`
- Modify: `cmd/alder/run.go` (devServer gets a `hub *eventHub`; `ServeHTTP` routes `/_dev/events`)
- Test: `cmd/alder/run_events_test.go`

**Interfaces:**
- Produces, in `cmd/alder`:
  - `type eventHub struct` (unexported fields: mutex, ring of encoded events, `seq int64`, subscribers, `boot string`, `now func() time.Time`).
  - `func newEventHub(boot string, size int) *eventHub` — `size` is 5000 in production (`const eventRing = 5000`).
  - `func (h *eventHub) emit(typ string, fields map[string]any) int64` — adds `type`, `t`, `seq`, encodes one JSON object, stores it, sends it to every subscriber, returns its seq.
  - `func (h *eventHub) subscribe(after int64) (backlog [][]byte, ch <-chan []byte, cancel func())` — backlog is every ring event with `seq > after`; when `after` is greater than the current seq (a different dev server), the whole ring. `ch` is buffered (256). A subscriber whose buffer is full is dropped: its channel is closed, so its stream ends and the client resumes with `?after`.
  - `func (d *devServer) serveEvents(w http.ResponseWriter, r *http.Request)` — SSE (`Content-Type: text/event-stream`, `Cache-Control: no-cache`), each event as `data: <json>\n\n`, preceded by `hello`, then the backlog, then live events; `: ping\n\n` every 15 s; ends on `d.done` or the client leaving. `GET` only (405 otherwise).
  - `hello` fields: `app`, `dir` (absolute, cleaned), `alder` (`version.String()`), `addr` (dev address), `child` (address or `null`), `gen`, `build` (`building|ok|failed`), `since` (the dev server's start, RFC3339Nano), `boot` (the dev server's nonce), `first` (seq of the oldest event in the ring, 0 when empty). `hello`'s `seq` is the hub's current seq and it is not stored in the ring.
  - `devServer` gains `state string` (`building|ok|failed`, under `mu`) and `since time.Time`; Task 4 sets `state`.

- [ ] **Step 1: Write the failing tests**

Hub tests (no child, no HTTP):

```go
func TestEventHub_SeqAndRing(t *testing.T)       // size 3; emit 5 → subscribe(0) backlog seqs [3 4 5]
func TestEventHub_After(t *testing.T)            // emit 5 → subscribe(3) backlog seqs [4 5]
func TestEventHub_AfterFromAnotherServer(t *testing.T) // emit 2 → subscribe(900) backlog seqs [1 2]
func TestEventHub_SlowSubscriberDropped(t *testing.T)  // subscribe, never read, emit 300 → ch is closed; emit still returns
```

Handler test `TestServeEvents_HelloReplayPing` builds a `devServer` with `newDevServer` on a temp dir (no build started), emits two `run` events, sets the ping interval to 10 ms through a package var `eventPing`, and reads the stream from an `httptest.Server`: the first event is `hello` with `dir` equal to the temp dir and `build == "building"`, then the two `run` events in order, then a `: ping` line. A second connection with `?after=<first run seq>` gets `hello` then only the second `run`. `POST /_dev/events` answers 405.

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./cmd/alder -run 'TestEventHub|TestServeEvents' -v`
Expected: FAIL, `undefined: newEventHub`.

- [ ] **Step 3: Implement `run_events.go` and route it in `ServeHTTP`**

`ServeHTTP` checks `/_dev/events` before the theme routes, like `/_dev/reload`. `newDevServer` creates the hub with `boot = nonce`. The seq counter and ring live under the hub's own mutex, not `d.mu`.

- [ ] **Step 4: Run to verify they pass**

Run: `go test ./cmd/alder -run 'TestEventHub|TestServeEvents' -v -race`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/alder/run_events.go cmd/alder/run_events_test.go cmd/alder/run.go
git commit -m "alder run: /_dev/events, an SSE feed with a replay ring (#120)"
```

### Task 3: The child logs JSON at debug; the dev server renders it

**Files:**
- Create: `cmd/alder/run_logs.go`
- Modify: `cmd/alder/run.go` (`childEnv`, `startChild`, `runRunWith`, `devServer` gains `logLevel applog.Level` and `logJSON bool`)
- Test: `cmd/alder/run_logs_test.go`, `cmd/alder/run_test.go` (one end-to-end assertion)

**Interfaces:**
- Consumes: `applog.Render` (Task 1), `eventHub.emit` (Task 2).
- Produces:
  - `func userLogging(getenv func(string) string) (applog.Level, bool, error)` — the user's `ALDER_LOG` (default Info) and whether `ALDER_LOG_FORMAT` is `json`; an invalid value is an error. `runRunWith` reports it as `alder run: <err>` and returns 2 before listening.
  - `childEnv(environ []string, dbPath, staticDir, gen, devAddr string) []string` appends `ALDER_LOG=debug` and `ALDER_LOG_FORMAT=json` after the existing variables (exec keeps the last value of a duplicated key).
  - `type logPipe struct` implementing `io.Writer` and `Flush() error`: splits on `\n` with no line-length limit; for each line it emits a `log` event (`record`: the decoded JSON object as `json.RawMessage`, when `applog.Render` says ok; else `raw`: the line as a string) and writes to its terminal writer: the line itself when `logJSON`, else `Render(line, logLevel)` (skipped when dropped), else the raw line; terminal styling via the existing `logLine(st)` when `st.On()`.
  - `func newLogPipe(term io.Writer, st style.Style, level applog.Level, passJSON bool, hub *eventHub) *logPipe`
  - `startChild` takes the `*devServer` settings it needs instead of `logs io.Writer, st style.Style`: change its signature to `startChild(binary, dbPath, staticDir, gen, devAddr string, logs func() *logPipe) (*child, error)` so stdout and stderr each get their own pipe. The child's `tailBuffer` receives the pipe's terminal output (rendered text), so a crash overlay shows text, not JSON.

- [ ] **Step 1: Write the failing unit tests**

```go
func TestUserLogging(t *testing.T)            // "" → Info,false; "debug" → Debug; ALDER_LOG_FORMAT=json → true; "loud" → error mentioning ALDER_LOG=loud
func TestChildEnv_DebugJSON(t *testing.T)     // the last ALDER_LOG= is "debug", the last ALDER_LOG_FORMAT= is "json", even when environ sets ALDER_LOG=warn
func TestLogPipe_RendersAtUserLevel(t *testing.T)
	// a JSON info record and a JSON debug record written in one Write, level Info:
	// terminal has only the info line, as Render gives it; two log events emitted, both with record
func TestLogPipe_PassesJSONWhenAsked(t *testing.T) // passJSON: terminal has the JSON line unchanged
func TestLogPipe_RawLines(t *testing.T)       // "panic: boom" → terminal "panic: boom\n"; event has raw, no record
func TestLogPipe_PartialWrites(t *testing.T)  // a record split across three Writes is one event and one line; Flush emits a trailing partial line
func TestLogPipe_HugeLine(t *testing.T)       // a 1 MiB JSON message: one event, one terminal line, byte-identical to Render's
```

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./cmd/alder -run 'TestUserLogging|TestChildEnv_DebugJSON|TestLogPipe' -v`
Expected: FAIL, undefined names.

- [ ] **Step 3: Implement `run_logs.go` and wire it**

`runRunWith` calls `userLogging(os.Getenv)` before `net.Listen` and stores the result on the devServer. Keep `logLinePat`/`logLine` as they are; the text `Render` produces is what they already colour.

- [ ] **Step 4: Add the end-to-end assertion**

In `cmd/alder/run_test.go`, `TestRun_LogsRenderedAndFed`: `startRun`, subscribe to `r.base+"/_dev/events"` with the existing `sse` helper, `GET /login`; the stderr buffer contains a line matching `^\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2} info: GET /login 200 ` and no `{"time"`; the feed has a `log` event whose `record.path == "/login"` and whose `record.steps` is present (the child runs at debug).

- [ ] **Step 5: Run the run tests**

Run: `go test ./cmd/alder -run 'TestRun_|TestLogPipe|TestChildEnv|TestUserLogging' -race`
Expected: PASS, including every existing `TestRun_*` (their stderr expectations are unchanged).

- [ ] **Step 6: Commit**

```bash
git add cmd/alder/run_logs.go cmd/alder/run_logs_test.go cmd/alder/run.go cmd/alder/run_test.go
git commit -m "alder run: child logs JSON at debug, rendered as today's text and fed as events (#120)"
```

### Task 4: Lifecycle events, `POST /_dev/restart`, exit on stdin EOF

**Files:**
- Modify: `cmd/alder/run.go`, `cmd/alder/run_events.go`, `cmd/alder/main.go` (the `run` case: stdin EOF)
- Test: `cmd/alder/run_test.go`, `cmd/alder/run_unix_test.go`

**Interfaces:**
- Consumes: `eventHub.emit`, `devServer.state`.
- Produces:
  - `func (d *devServer) status(text string)` — prints `d.errSt.Dim(text)` on stderr and emits `run {text}`. Every existing `fmt.Fprintln(d.stderr, d.errSt.Dim("alder run: …"))` goes through it; the stdout announcement also emits `run {text}`.
  - `build` events: `{state: "building", gen}` when a build starts (gen: the one it will start); `{state: "failed", gen, diagnostics: [diagItem…]}` for a check failure, `{state: "failed", gen, text}` for a build failure or a child that did not start; `{state: "ok", gen, child, ms, warnings?}`. `warnings` are the check's warnings as `diagItem`s (re-run `runCheckTo --json` only when the check printed something). `d.state` follows.
  - `app` events: `{state: "started", child}` after the swap; `{state: "exited", exit: {code, signal?}, tail: [string]}` from `monitor` (tail: the last 40 lines, as the overlay).
  - `static {}` when static files are re-copied.
  - `func (d *devServer) serveRestart(w http.ResponseWriter, r *http.Request)` at `/_dev/restart`: `POST` only (405 otherwise); 403 when an `Origin` header is present and its host is not `d.addr`; otherwise 202 and a non-blocking send on `d.restart chan struct{}` (buffer 1). The worker's `select` takes it and starts a fresh child from the last good binary (`d.binary`, set by `rebuild`), or runs a full build when there is none. Extract the start-swap-announce half of `rebuild` into `func (d *devServer) swapIn(binary string)` so both paths share it.
  - `alder run` exits as on Ctrl-C when `ALDER_EXIT_ON_STDIN_EOF=1` is set and its stdin reaches EOF (main.go's `run` case closes the stop channel), and unsets the variable so its own child does not inherit it from this path (the child gets it from `childEnv` as before).

- [ ] **Step 1: Write the failing tests**

```go
func TestRun_FeedLifecycle(t *testing.T)
	// startRun; subscribe; hello.build == "ok" and hello.child == r.child
	// break app/App.alder (edit helper) → events build building, build failed with ≥1 diagnostic carrying file and line
	// fix it → build building, build ok with a child address, app started
func TestRun_FeedRunLines(t *testing.T)   // the replayed backlog has a run event whose text starts "alder run: building "
func TestRun_Restart(t *testing.T)
	// POST /_dev/restart → 202; feed shows app started with a new child address; GET /login still 200
	// POST with Origin: http://evil.example → 403; GET /_dev/restart → 405
func TestRun_FeedAppExited(t *testing.T)  // in run_unix_test.go: kill the child (SIGKILL r.d.child pid) → app exited with exit.signal "killed" and a tail
func TestRun_ExitsOnStdinEOF(t *testing.T) // in run_unix_test.go: build the alder binary (go build into t.TempDir()), run `alder run` on a copy of examples/todos with ALDER_EXIT_ON_STDIN_EOF=1 and ALDER_ADDR=127.0.0.1:0, wait for "dev server on", close its stdin → it exits 0 within 15 s and the child is gone
```

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./cmd/alder -run 'TestRun_Feed|TestRun_Restart|TestRun_ExitsOnStdinEOF' -v`
Expected: FAIL.

- [ ] **Step 3: Implement the events, the restart route and the stdin rule**

- [ ] **Step 4: Run the whole run suite**

Run: `go test ./cmd/alder -run 'TestRun|TestEventHub|TestServeEvents|TestLogPipe' -race -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/alder/run.go cmd/alder/run_events.go cmd/alder/main.go cmd/alder/run_test.go cmd/alder/run_unix_test.go
git commit -m "alder run: build/app/static/run events, POST /_dev/restart, exit on stdin EOF (#120)"
```

### Task 5: `alder tui` dispatch, help and the book

**Files:**
- Create: `cmd/alder/tui.go`, `cmd/alder/tui_test.go`
- Modify: `cmd/alder/main.go` (case `"tui"`), `cmd/alder/helpjson.go` (cmdSpec + `commandUsages["tui"]`), the text usage, `book/11-tooling.md`
- Test: `cmd/alder/tui_test.go`, existing `TestHelpJSON_MatchesUsage`

**Interfaces:**
- Produces: `func runTuiWith(args []string, stdin io.Reader, stdout, stderr io.Writer, getenv func(string) string, lookPath func(string) (string, error)) int` — the binary is `getenv("ALDER_TUI")` when set, else `lookPath("alder-tui")`; it runs it with `args`, the environment plus `ALDER_BIN=<os.Executable()>`, and the given stdio; it returns the child's exit code; while it runs, `alder` ignores SIGINT (the TUI handles Ctrl-C). Missing: the dispatch message from Global Constraints with `version.String()`, exit 1. `runTuiTo(args, stdout, stderr)` wraps it with `os.Getenv` and `exec.LookPath`.

- [ ] **Step 1: Write the failing tests**

```go
func TestTui_Missing(t *testing.T)   // lookPath fails, no ALDER_TUI → 1, stderr is the dispatch message line exactly, with version.String()
func TestTui_Found(t *testing.T)     // lookPath returns a script (unix only: t.Skip on windows) that prints "$@" and $ALDER_BIN and exits 3 → 3; stdout has the args and a non-empty ALDER_BIN
func TestTui_Override(t *testing.T)  // ALDER_TUI=<that script> wins over lookPath
```

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./cmd/alder -run 'TestTui|TestHelpJSON' -v`
Expected: FAIL (undefined `runTuiWith`; help test still passes until the case is added, then fails until usage lists it).

- [ ] **Step 3: Implement the dispatch, register it in help, write the book section**

In `book/11-tooling.md`, a section "The terminal UI and the dev feed" after the `alder run` material: what `alder tui` does, that the TUI is a separate binary from the same Homebrew formula, and the machine interfaces behind it (`/_dev/events` event types with one example line each, `POST /_dev/restart`, the child's debug/JSON logging and how the dev server renders it, `ALDER_EXIT_ON_STDIN_EOF` for `alder run`). Explain how the feed works inside (ring, `seq`, `?after`, `boot`), per the book rule that it teaches internals to a beginner.

- [ ] **Step 4: Run cmd/alder and the book check**

Run: `go test ./cmd/alder -count=1 && python3 book/site/test_site.py`
Expected: PASS.

- [ ] **Step 5: Commit and open PR 1**

```bash
git add cmd/alder/tui.go cmd/alder/tui_test.go cmd/alder/main.go cmd/alder/helpjson.go book/11-tooling.md
git commit -m "alder tui: dispatch to alder-tui on PATH; book: the dev feed (#120)"
git push -u origin tui-feed
gh pr create --title "alder run: the dev feed (/_dev/events) and alder tui dispatch (#120, PR 1/4)" --body "<summary, test plan, link to the spec>"
```

---

# PR 2 — alder: request detail

Branch `tui-requests` from main after PR 1 merges. Tasks 6–9.

### Task 6: `rid` on JSON log records written during a request

**Files:**
- Modify: `rt/applog/applog.go` (`LogID`), `rt/web/router.go` (Options, per-request `Env.Log`), `internal/gen/caps.go` (generated main wires `LogID: lg.LogID`)
- Test: `rt/applog/applog_test.go`, `rt/web/router_test.go` (or the file holding access-line tests; follow where `Options.Access` is tested), `internal/gen` golden for the generated main if one exists

**Interfaces:**
- Produces:
  - `func (l *Logger) LogID(id, level, msg string)` — `Log` with, as JSON, a `rid` key (text lines unchanged, no rid). `Log` becomes `LogID("", …)` with no `rid` key when id is empty.
  - `web.Options.LogID func(id, level, msg string)` — when set, the router's own Env for a request logs through it with the request's id; `Options.Log` stays the hook for lines outside a request.
  - `func (t *tracker) log(level, msg string)` — the per-request hook: appends `{t, level, msg}` to `t.logs` when the request has a Recorder (Task 8 reads it), then calls `LogID(t.id, …)` or `rt.log`. The router sets `rq.Env.Log = tw.log` (one method-value allocation per routed request; note it in the commit message). `Options.EnvFor` Envs are untouched.

- [ ] **Step 1: Write the failing tests**

```go
func TestLogID_JSONCarriesRid(t *testing.T)   // JSON logger: LogID("5f3a2c1e","info","hi") → record has rid "5f3a2c1e"; Log("info","hi") → no rid key
func TestLogID_TextUnchanged(t *testing.T)    // text logger: LogID and Log write identical lines
func TestRouter_LogLinesCarryRid(t *testing.T) // a route whose handler calls rq.Env.Log("info","x"), Options.LogID set: the hook gets the X-Request-Id the response carries; an effect step line ("effect …") carries it too
```

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./rt/applog ./rt/web -run 'TestLogID|TestRouter_LogLinesCarryRid' -v`
Expected: FAIL.

- [ ] **Step 3: Implement, and wire the generated main**

- [ ] **Step 4: Run rt and gen**

Run: `go test ./rt/... ./internal/gen -count=1`
Expected: PASS (update the generated-main golden if one fails only for the new `LogID:` line).

- [ ] **Step 5: Commit**

```bash
git add rt/applog rt/web internal/gen
git commit -m "applog, web: rid on JSON log records written during a request (#120, #118 option A)"
```

### Task 7: `db_stmts`, `db_ms` and `repeats` on the debug access record

**Files:**
- Modify: `rt/applog/applog.go` (`Access` fields, JSON branch), `rt/applog/render.go` (ignore the new keys), `rt/web/router.go` (count stats in dev; fill the fields), `rt/web/devpanel.go` (a helper counting N+1 groups)
- Test: `rt/applog/applog_test.go`, `rt/web` access tests

**Interfaces:**
- Produces:
  - `Access.Counted bool` (Stmts and DBDur were counted) and `Access.Repeats int` (N+1 groups found; dev mode only, else 0).
  - JSON at `min == Debug`, not slow: adds `db_stmts`, `db_ms`, `repeats` when `Counted`. Slow records keep their keys and add `repeats` at Debug when `Counted`. Info-level records and every text line are unchanged.
  - The router collects `eff.Stats` when `Metrics != nil || SlowAfter > 0 || rt.panelOn()` and sets `Counted` exactly then.
  - `func repeatCount(tw *tracker) int` in devpanel.go: the number of N+1 groups `segRepeats` finds (summed over segments); `warnRepeats` and the access record use the same computation, computed once per request.

- [ ] **Step 1: Write the failing tests**

```go
func TestAccess_DebugJSONCarriesDB(t *testing.T)  // Debug JSON, Counted, Stmts 11, DBDur 31.2ms, Repeats 1 → db_stmts 11, db_ms 31.2, repeats 1
func TestAccess_InfoJSONUnchanged(t *testing.T)   // Info JSON with the same Access: no db_stmts, db_ms, repeats keys
func TestAccess_NotCountedNoDB(t *testing.T)      // Debug JSON, Counted false → no db_* keys
func TestRouter_DevCountsRepeats(t *testing.T)    // dev router (ALDER_DEV=1 via t.Setenv), a handler running one select 5× with different args → Access.Counted, Stmts 5, Repeats 1
```

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./rt/applog ./rt/web -run 'TestAccess_|TestRouter_DevCountsRepeats' -v`
Expected: FAIL.

- [ ] **Step 3: Implement**

- [ ] **Step 4: Run**

Run: `go test ./rt/... -count=1`
Expected: PASS, including Task 1's round-trip test.

- [ ] **Step 5: Commit**

```bash
git add rt/applog rt/web
git commit -m "access record: db_stmts, db_ms and repeats at debug (#120)"
```

### Task 8: The request ring and `/_dev/requests[/{rid}]`

**Files:**
- Create: `rt/web/devrequests.go`, `rt/web/devrequests_test.go`
- Modify: `rt/web/router.go` (ring on the router; record in `ServeHTTP`'s defer; route the two paths)

**Interfaces:**
- Consumes: `tracker.logs` (Task 6), `repeatCount` and `segRepeats`/`repeatMarks` (Task 7, existing).
- Produces:
  - `const requestRing = 200`; `type reqRing struct` (mutex, fixed array, next index, by-rid lookup).
  - `type ReqRecord struct` with JSON tags giving exactly the spec's shape: `{rid, t, method, path, route, status, ms, bytes, steps: [{name, ms, cap}], sql: [{stmt, args, ms, rows, group, repeats?}], loads, cuts, logs: [{t, level, msg}], slow, warnings: [text]}`. `path` is the masked path (`Access.MaskedPath`); `args` are `fmt.Sprint` of each argument, as the panel shows them; `repeats` on a statement is its N+1 group's count (`repeatMarks`); `warnings` are the `Repeat.String()` lines `warnRepeats` logs.
  - `type ReqSummary struct` `{rid, t, method, path, route, status, ms, bytes, steps (count), stmts, repeats, slow}`.
  - The ring records every request that has a Recorder (`rt.panelOn()`) and whose access record is not `Quiet`, once, in `ServeHTTP`'s defer after the access line.
  - `GET /_dev/requests` → `200` JSON array of summaries, newest first. `GET /_dev/requests/{rid}` → the record, or `404` JSON `{"error":"no request <rid> in the last 200"}`. Both only when `rt.panelOn()`; otherwise they fall through to the app's routes as today. Neither request is itself recorded (they are under `/_dev`; make `quiet` treat `/_dev/requests…` and `/_dev/map.json` 2xx as quiet).

- [ ] **Step 1: Write the failing tests**

```go
func TestDevRequests_RingAndOrder(t *testing.T)  // dev router; 3 requests → /_dev/requests has 3 summaries, newest first, rids match X-Request-Id
func TestDevRequests_Detail(t *testing.T)        // a handler with a Log line, two steps and a 5× select → detail has logs[0].msg, steps in order, one sql entry with repeats 5, one warning starting "N+1:"
func TestDevRequests_Evicts(t *testing.T)        // 201 requests → the first rid answers 404 with the error JSON; the 201st is found
func TestDevRequests_OffOutsideDev(t *testing.T) // ALDER_DEV unset → /_dev/requests is the app's 404, nothing recorded
func TestDevRequests_NotSelfRecorded(t *testing.T) // GET /_dev/requests twice → the second list does not contain the first
func TestDevRequests_StaticNotRecorded(t *testing.T) // a 200 static file → not in the ring
```

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./rt/web -run TestDevRequests -v`
Expected: FAIL.

- [ ] **Step 3: Implement `devrequests.go` and the hooks**

- [ ] **Step 4: Run**

Run: `go test ./rt/web -race -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add rt/web
git commit -m "web: /_dev/requests, a ring of the last 200 request records (#120)"
```

### Task 9: `/_dev/map.json` with SQL names and silos

**Files:**
- Modify: `internal/gen/devmap.go`, `rt/web/devpanel.go` (`devMapData`/`devMapTable` gain the fields; JSON handler), `rt/web/router.go` (route), `book/11-tooling.md` (document `/_dev/requests` and `/_dev/map.json` next to the feed section)
- Test: `internal/gen` devmap test (create `internal/gen/devmap_test.go` if none), `rt/web/devrequests_test.go` or a new `rt/web/devmap_test.go`

**Interfaces:**
- Produces, in the generator's `devTable`: `SQL string` (`json:"sql"`, `db.SQLName(Module.Name)`), `Cols []devCol` (`json:"cols"`), `Silo string` (`json:"silo,omitempty"`); `type devCol struct{ Name, Type, SQL, SQLType string }` with tags `name`, `type` (Alder type, `types.String`), `sql` (the column's SQL name, which is the field name today; take it from where `columnLit` takes it, not by assumption), `sqlType` (`TEXT`, `INTEGER` or `REAL`, from the column's `db.ColType` storage). `devMap` gains `Silos []devSilo` (`json:"silos,omitempty"`); `type devSilo struct{ Key, Ref string; Tables []string; Path string }` with tags `key`, `ref`, `tables`, `path`, where `path` is `silos/<key>/{value}.db`, relative to the shared database's directory (read `rt/db` `SiloPath`/`SiloFileName` and state the rule it implements; if `SiloFileName` transforms the value, say so in the field's doc).
- `GET /_dev/map.json` (under `rt.devOn()`, as `/_dev`): `{build, map, caps: [{name, kind}], migrations, routes: [{method, pat}]}` — the same data `devMap` renders, as JSON, with `map` the generator's map decoded and re-encoded (so the new fields pass through).

- [ ] **Step 1: Write the failing tests**

```go
func TestDevMapJSON_SQLNames(t *testing.T) // gen: a program with table Shelf.Book {id, title: Text, pages: Int} → tables[0].sql "Shelf__Book", cols[1] {name "title", type "Text", sql "title", sqlType "TEXT"}, cols[2].sqlType "INTEGER"
func TestDevMapJSON_Silos(t *testing.T)    // gen: a program with a silo → silos[0].key, tables, path "silos/<key>/{value}.db"; the siloed table has silo set
func TestRouter_MapJSON(t *testing.T)      // web: Options.DevMap with the new fields, ALDER_DEV=1 → /_dev/map.json decodes; map.tables[0].sql present; migrations and routes present; ALDER_DEV unset → app's 404
```

Build the test programs the way neighbouring gen tests do (find the helper that parses a source string into `*ir.Program`).

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/gen ./rt/web -run 'TestDevMapJSON|TestRouter_MapJSON' -v`
Expected: FAIL.

- [ ] **Step 3: Implement and document**

- [ ] **Step 4: Run the full suite once (PR boundary)**

Run: `scripts/test.sh` (or `go test ./... -count=1` if the script needs CI-only setup)
Expected: PASS.

- [ ] **Step 5: Commit and open PR 2**

```bash
git add internal/gen rt/web book/11-tooling.md
git commit -m "dev map: /_dev/map.json with SQL names and silos (#120)"
git push -u origin tui-requests
gh pr create --title "Request detail: /_dev/requests, rid on log records, /_dev/map.json (#120, PR 2/4)" --body "<summary, test plan, spec link>"
```

---

# PR 3 — tui: skeleton, Requests, Logs

Branch `tui-requests-logs` from main after PR 2 merges. Tasks 10–17. All TUI code is under `tui/`; run its commands with `working-directory` `tui` (`cd tui && go test ./...`).

TUI package layout (decided here; tasks below fill it):

| Path | Responsibility |
|---|---|
| `tui/cmd/alder-tui/main.go` | flags, `version` var, wiring; nothing testable lives here |
| `tui/internal/alderbin` | finding `alder` (`ALDER_BIN`, `PATH`), `help --json` version check |
| `tui/internal/runner` | the one interface every subprocess call goes through, plus `Fake` |
| `tui/internal/feed` | event types, NDJSON/SSE decoding, the reconnecting client |
| `tui/internal/devproc` | attach-or-own `alder run` |
| `tui/internal/store` | log scrollback, request table, the 60 s stats strip |
| `tui/internal/ui` | Bubble Tea root model, tabs, status bar, footer, overlays, screens (one file per screen) |
| `tui/internal/look` | the 16-colour palette, marks, status-class colours, `Columns` |
| `tui/testdata/feeds/*.ndjson` | hand-written feed fixtures |
| `tui/testdata/shelf/` | the fixture Alder app (Task 17) |

Golden frames: `teatest` with `teatest.WithInitialTermSize(100, 30)` unless a test says otherwise, the colour profile forced to ANSI (16 colours) so goldens show the real escapes, a fixed clock injected into the model (`ui.Options.Now`), goldens under each package's `testdata/` and updated with `go test ./... -update`.

### Task 10: The module, CI job, release build and version check

**Files:**
- Create: `tui/go.mod`, `tui/go.sum`, `tui/cmd/alder-tui/main.go`, `tui/internal/alderbin/alderbin.go`, `tui/internal/alderbin/alderbin_test.go`, `tui/internal/look/look.go`, `tui/internal/look/look_test.go`, `tui/README.md`
- Modify: `.github/workflows/ci.yml` (job `tui`), `.goreleaser.yaml` (build, archive, brew install line), `book/11-tooling.md` (install line)

**Interfaces:**
- Produces:
  - `alderbin.Find(getenv func(string) string, lookPath func(string) (string, error)) (string, error)` — `ALDER_BIN`, else `alder` on PATH; the error says `alder is not on PATH; install it or set ALDER_BIN`.
  - `alderbin.Version(ctx context.Context, run func(ctx context.Context, args ...string) ([]byte, error)) (string, error)` — runs `help --json` through `run` and returns `version`; `main.go` passes a closure over `exec.CommandContext(alder, args...)` until Task 11's runner exists, then over the runner.
  - `alderbin.Mismatch(tui, alder string) string` — `""` when equal or either is `(devel)`; else `alder-tui <tui> with alder <alder>: upgrade both together`.
  - `look`: `Style` with `Bold, Dim, Red, Green, Yellow, Cyan, BoldRed, BoldGreen, BoldYellow func(string) string`; `Mark(status string) string` (ok/FAIL/error/warning/SKIPPED as `internal/style.Mark` does); `StatusClass(code int) func(string) string`; `Columns(rows [][]string, gutter int) []string`. Copy the meanings from `internal/style/style.go`; do not import it (it is `internal/`).
  - `main.go`: `var version = "(devel)"`; flags `--addr`, `--no-run`, `--version`; positional `[dir]`.
  - CI job `tui`: runs when `tui/**` or `.github/workflows/ci.yml` changed (add a `tui` output to `changes` via the `paths` filter step) and on main/tags; `gofmt -l .`, `go vet ./...`, `go test ./...` with `working-directory: tui`, setup-go with `go-version-file: tui/go.mod`. The root lint's `gofmt -l $(git ls-files '*.go')` already covers `tui/`.
  - GoReleaser: build `alder-tui` with `dir: tui`, `main: ./cmd/alder-tui`, `binary: alder-tui`, the same targets and flags, `ldflags: -X main.version=v{{ .Version }}` (a nested module's build info does not carry the root tag); archive `alder-tui_{{ .Version }}_{{ .Os }}_{{ .Arch }}`; the brew formula's `ids: [alder, alder-tui]` and `install` gains `bin.install "alder-tui"`. Not in `dockers_v2`.

- [ ] **Step 1: Write the failing tests**

```go
func TestFind(t *testing.T)      // ALDER_BIN wins; else lookPath("alder"); neither → error text exact
func TestVersion(t *testing.T)  // run returns {"version":"v0.6.0","commands":[]} → "v0.6.0"; invalid JSON → error
func TestMismatch(t *testing.T)  // ("v0.6.0","v0.6.0") → ""; ("(devel)","v0.6.0") → ""; ("v0.6.0","v0.5.0") → "alder-tui v0.6.0 with alder v0.5.0: upgrade both together"
func TestMark(t *testing.T)      // look.Mark("ok") is green "ok"; "FAIL" bold red; "SKIPPED" yellow; unknown unchanged
func TestStatusClass(t *testing.T) // 200 plain, 302 dim, 404 yellow, 500 bold red
```

- [ ] **Step 2: Run to verify they fail**

Run: `cd tui && go test ./...`
Expected: FAIL (packages missing).

- [ ] **Step 3: Create the module and the packages; add the CI job and release entries**

`go mod init alder/tui`, `go 1.27`, then `go get` Bubble Tea, Lip Gloss, Bubbles, teatest at their current stable majors and modernc.org/sqlite v1.60.1.

- [ ] **Step 4: Run, and check the release config**

Run: `cd tui && gofmt -l . && go vet ./... && go test ./...` then, from the root, `goreleaser check` if goreleaser is installed (skip and say so in the PR otherwise).
Expected: no gofmt output, PASS, `config is valid`.

- [ ] **Step 5: Commit**

```bash
git add tui .github/workflows/ci.yml .goreleaser.yaml book/11-tooling.md
git commit -m "tui: module alder/tui, CI job, release build, version check (#120)"
```

### Task 11: The runner

**Files:**
- Create: `tui/internal/runner/runner.go`, `tui/internal/runner/fake.go`, `tui/internal/runner/runner_test.go`

**Interfaces:**
- Produces:
  - `type Runner interface { Run(ctx context.Context, dir string, args ...string) (Result, error); Stream(ctx context.Context, dir string, args ...string) (Stream, error) }`
  - `type Result struct { Stdout, Stderr []byte; Code int }`
  - `type Stream interface { Lines() <-chan string; Wait() (code int, stderr []byte) }` — stdout lines as they arrive, no line-length limit; the channel closes at EOF.
  - `func Exec(alder string) Runner` — runs `alder <args>` in `dir`, environment inherited.
  - `type Fake struct` with `On(args string, out string, code int)` (args joined by spaces) and `Calls() []string`; `Stream` on a Fake yields `out` split on newlines. Every later task's tests use `Fake`.

- [ ] **Step 1: Write the failing tests**

```go
func TestExec_RunCode(t *testing.T)    // Exec("/bin/sh") Run(dir, "-c", "echo hi; echo err >&2; exit 3") → Stdout "hi\n", Stderr "err\n", Code 3 (unix)
func TestExec_StreamLines(t *testing.T) // Stream of a 3-line printf → 3 lines in order, then Wait code 0
func TestFake(t *testing.T)            // On("test --json", "{\"a\":1}\n{\"b\":2}", 1) → Stream yields 2 lines, Wait 1; Calls records it
```

- [ ] **Step 2: Run to verify they fail**

Run: `cd tui && go test ./internal/runner -v`
Expected: FAIL.

- [ ] **Step 3: Implement**

- [ ] **Step 4: Run**

Run: `cd tui && go test ./internal/runner -race`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add tui/internal/runner
git commit -m "tui: runner, the one seam for alder subprocesses (#120)"
```

### Task 12: The feed client

**Files:**
- Create: `tui/internal/feed/event.go`, `tui/internal/feed/client.go`, `tui/internal/feed/feed_test.go`, `tui/testdata/feeds/basic.ndjson`, `tui/testdata/feeds/failed.ndjson`, `tui/testdata/feeds/exited.ndjson`

**Interfaces:**
- Produces:
  - `type Event struct { Type string; T time.Time; Seq int64; Hello *Hello; Build *Build; App *App; Log *Log; Run *Run }` (Static has no payload).
  - `type Hello struct { App, Dir, Alder, Addr, Gen, Build, Boot string; Child *string; Since time.Time; First int64 }`
  - `type Build struct { State, Gen, Child, Text string; Diagnostics, Warnings []Diag; Ms float64 }`; `type Diag struct { Code, Severity, File, Message string; Line, Col, EndLine, EndCol int }`
  - `type App struct { State, Child string; Exit *Exit; Tail []string }`; `type Exit struct { Code int; Signal string }`
  - `type Log struct { Raw string; Record *Record }`; `type Record struct { Time time.Time; Level, Msg, Rid, Method, Path, Route string; Status, Steps, DBStmts, Repeats int; Ms, DBMs float64; Bytes int64; Slow bool; HasAccess bool }` — `HasAccess` is true when rid, status, method and path are all present.
  - `type Run struct { Text string }`
  - `func Decode(line []byte) (Event, error)`
  - `func Connect(ctx context.Context, addr string) (<-chan Event, error)` — the first connection's error is returned (the caller decides attach vs own); afterwards it reconnects on its own with `?after=<last seq>`, backing off 250 ms doubling to 5 s, and emits a synthetic `Event{Type: "disconnected"}` once per outage. A `hello` whose `Boot` differs from the previous one is passed through as is; the UI resets on it.
  - Fixtures: `basic.ndjson` has hello (app `shelf`, build ok), build ok, app started, six access `log` events (a 200 GET `/`, a 200 GET `/books/42` with `repeats` 1, a 303 POST `/books/42/lend` with `db_stmts` 2, a 404, a 500 slow `/search?q=••••`, a debug-level static file), three `Log` lines with the GET `/books/42` rid (info, warn, an `effect Books.find` debug line), a `run` line and a raw `panic:` line. `failed.ndjson`: hello with build failed and a build failed with two diagnostics. `exited.ndjson`: an app exited with code 2 and a 3-line tail. Hand-written; shapes exactly as PR 1–2 emit them (copy one real line of each type from a local `alder run` to check).

- [ ] **Step 1: Write the failing tests**

```go
func TestDecode_Fixtures(t *testing.T)   // every line of the three fixtures decodes; basic has 6 HasAccess records; the slow one has Slow and Ms
func TestConnect_Replay(t *testing.T)    // httptest SSE server serving basic.ndjson as data: lines → the channel yields them in order
func TestConnect_Resumes(t *testing.T)   // server closes after 3 events; the reconnect asks ?after=<3rd seq>; server sends hello + the rest; channel has a disconnected event then the rest, no duplicates
func TestConnect_NewBoot(t *testing.T)   // after reconnect the server sends a hello with another boot and seq 1 → channel yields that hello (UI resets) and the new events
func TestConnect_NotAlder(t *testing.T)  // server answers 404 HTML → Connect returns ErrNotAlder
```

`var ErrNotAlder = errors.New("not an alder dev server")` is returned when the answer is not `text/event-stream` or the first event is not `hello`.

- [ ] **Step 2: Run to verify they fail**

Run: `cd tui && go test ./internal/feed -v`
Expected: FAIL.

- [ ] **Step 3: Implement and write the fixtures**

- [ ] **Step 4: Run**

Run: `cd tui && go test ./internal/feed -race`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add tui/internal/feed tui/testdata/feeds
git commit -m "tui: feed client with replay, resume and fixtures (#120)"
```

### Task 13: Attach or own `alder run`

**Files:**
- Create: `tui/internal/devproc/devproc.go`, `tui/internal/devproc/devproc_test.go`

**Interfaces:**
- Consumes: `feed.Connect`, `feed.ErrNotAlder`, `runner` is not used here (the dev server is a long-lived process with its own stdin).
- Produces:
  - `type Options struct { Dir, Addr, Alder string; NoRun bool; Connect func(ctx context.Context, addr string) (<-chan feed.Event, error); Start func(alder, dir string, env []string) (Proc, error) }` — `Connect` and `Start` default to the real ones; tests replace them.
  - `type Proc interface { Stderr() <-chan string; Stop() error; Exited() <-chan struct{} }` — the real one runs `alder run [dir]` with `ALDER_ADDR=<addr>`, `ALDER_EXIT_ON_STDIN_EOF=1` and a stdin pipe it holds; `Stop` is SIGTERM, then SIGKILL after 5 s.
  - `type Session struct { Events <-chan feed.Event; Owned bool; Early <-chan string }` — `Early` carries the owned child's stderr lines until the feed answers (the Logs screen shows them as run lines).
  - `func Open(ctx context.Context, o Options) (*Session, error)` with these outcomes: a feed whose `hello.Dir` equals `filepath.Clean(abs(o.Dir))` → attach (`Owned` false); a feed for another directory → error `localhost:8080 is the dev server of <dir>; pass --addr`; `ErrNotAlder` → error `<addr> is serving something that is not alder run; pass --addr`; connection refused and `NoRun` → error `no dev server on <addr> (--no-run)`; refused otherwise → `Start`, then retry `Connect` every 200 ms until it answers (30 s cap, then the child's last stderr lines in the error).
  - `func (s *Session) Close() error` — stops the child when owned.
  - Address default: `--addr`, else `ALDER_ADDR`, else `localhost:8080` (resolved in `main.go`).

- [ ] **Step 1: Write the failing tests**

```go
func TestOpen_AttachSameDir(t *testing.T)   // fake Connect yields hello with Dir == o.Dir → Owned false; Start never called
func TestOpen_OtherApp(t *testing.T)        // hello.Dir "/elsewhere" → error text exact
func TestOpen_NotAlder(t *testing.T)        // Connect → ErrNotAlder → error text exact; Start never called
func TestOpen_NoRun(t *testing.T)           // refused + NoRun → error text exact
func TestOpen_Owns(t *testing.T)            // refused twice then hello → Start called once with ALDER_EXIT_ON_STDIN_EOF=1 in env; Owned; Early had the fake's stderr line; Close calls Stop
func TestProc_DiesWithParentPipe(t *testing.T) // unix: real Start of a shell script that exits when stdin closes; closing the pipe (simulating TUI death) ends it within 1 s
```

- [ ] **Step 2: Run to verify they fail**

Run: `cd tui && go test ./internal/devproc -v`
Expected: FAIL.

- [ ] **Step 3: Implement**

- [ ] **Step 4: Run**

Run: `cd tui && go test ./internal/devproc -race`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add tui/internal/devproc
git commit -m "tui: attach to or own alder run (#120)"
```

### Task 14: The stores: logs, requests, the 60 s strip

**Files:**
- Create: `tui/internal/store/logs.go`, `tui/internal/store/requests.go`, `tui/internal/store/stats.go`, `tui/internal/store/store_test.go`

**Interfaces:**
- Consumes: `feed.Event`, `feed.Record`.
- Produces:
  - `type Line struct { Seq int64; T time.Time; Level string; Text string; Rid string; Effect, Run, Access bool }` — `Text` is the line as `alder run` renders it (stamp `2006/01/02 15:04:05` UTC, `level: `, message; an access line in the text logger's form at the store's view level, without the debug-only `steps=` unless the level filter is debug). Port the access-line format from `applog.Render`; the TUI cannot import it, and a test pins the same examples Task 1 uses.
  - `type Logs struct` — ring of 20 000 `Line`s. `func NewLogs(max int) *Logs`; `Add(e feed.Event)` (log and run events); `type Filter struct { Level string; Rid string; Re *regexp.Regexp }`; `View(f Filter) []Line` (stacked filters; level order debug < info < warn < error; run and raw lines count as info); `Reset()`.
  - `type Request struct { feed.Record; Seq int64 }`; `type Requests struct` — every `HasAccess` record whose level is not `DEBUG` (quiet static and dev hits are left out of the table), newest last, capped at 5 000. `Add`, `All() []Request`, `ByRid(string) (Request, bool)`, `Reset()`.
  - `type Strip struct { ReqPerSec, P50, P95, ErrPct float64 }`; `func Stats(rs []Request, now time.Time) Strip` — over records with `T` in `(now-60s, now]`; ErrPct counts status ≥ 500; percentiles by nearest rank.

- [ ] **Step 1: Write the failing tests**

```go
func TestLogs_FiltersStack(t *testing.T)   // basic.ndjson: Level info hides the effect line; Rid of /books/42 shows its access line and two Log lines; Re "lend" on top of info → one line
func TestLogs_Cap(t *testing.T)            // NewLogs(3), add 5 → View has the last 3
func TestLogs_TextMatchesAlderRun(t *testing.T) // the access examples of rt/applog TestRender at info and debug render to the same strings (copy the expected strings)
func TestRequests_LeavesOutQuiet(t *testing.T)  // basic.ndjson → 5 requests (the debug static record left out)
func TestStats(t *testing.T)               // 4 requests in the window at 10, 20, 30, 40 ms, one 500, one outside the window → ReqPerSec 4/60, P50 20, P95 40, ErrPct 25
func TestReset(t *testing.T)               // Reset empties both stores
```

- [ ] **Step 2: Run to verify they fail**

Run: `cd tui && go test ./internal/store -v`
Expected: FAIL.

- [ ] **Step 3: Implement**

- [ ] **Step 4: Run**

Run: `cd tui && go test ./internal/store`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add tui/internal/store
git commit -m "tui: log, request and stats stores (#120)"
```

### Task 15: The frame: tabs, status bar, footer, keys, Problems overlay

**Files:**
- Create: `tui/internal/ui/root.go`, `tui/internal/ui/statusbar.go`, `tui/internal/ui/footer.go`, `tui/internal/ui/problems.go`, `tui/internal/ui/keys.go`, `tui/internal/ui/root_test.go`, `tui/internal/ui/testdata/*.golden`
- Modify: `tui/cmd/alder-tui/main.go` (run the program)

**Interfaces:**
- Consumes: `feed.Event`, `store.*`, `look`, `devproc.Session`, `runner.Runner`.
- Produces:
  - `type Options struct { App, Dir, Addr, Version, AlderVersion string; Events <-chan feed.Event; Early <-chan string; Runner runner.Runner; HTTP *http.Client; Now func() time.Time; Owned bool; OpenURL func(string) error; Copy func(string) error }`
  - `func New(o Options) tea.Model`
  - `type Screen interface { Title() string; Update(msg tea.Msg, c *Ctx) (Screen, tea.Cmd); View(width, height int, c *Ctx) string; Keys() []Key; Command(c *Ctx) string }` — `Command` is the footer's right-aligned real command. `type Key struct { Key, Help string }`. `Ctx` holds the stores, the last hello and build, the options and the active filters, shared by screens.
  - Tabs `1`–`5` and `tab`/`shift-tab`: Requests, Logs, Data, Tests, Routes (Tasks 18–20 fill the last three; until then each is a placeholder screen whose view is the one line `coming in the next PR` — remove the placeholders in PR 4).
  - Status bar: ` <app>  ● <state>  <addr>  alder <version>` then the tab names right-aligned with the current one bold; the dot yellow building, green ok, red failed with `failed (N)`; a mismatch warning (`alderbin.Mismatch`) or `disconnected` in yellow replaces the version when present.
  - Footer: the screen's keys as `key help` pairs, the command right-aligned and dim; `y` with the footer focused (`F` focuses it) copies the command.
  - Global keys: `?` keys overlay, `p` Problems, `R` `POST http://<addr>/_dev/restart`, `o` open `http://<addr>/` in the browser, `q`/`ctrl-c` quit (closes the session).
  - Problems overlay: each diagnostic as `file:line:col: error[CODE]: message` (`file:line:col:` bold, `error[CODE]:` bold red, warnings yellow); a build failure with `text` shows the text; after `app exited` it shows `the app exited (code N)` and the tail. `enter` copies `file:line:col`.
  - A `hello` with a new `Boot` resets the stores.

- [ ] **Step 1: Write the failing golden tests**

```go
func TestFrame_Basic(t *testing.T)       // events from basic.ndjson, final frame: status bar "shelf  ● ok  localhost:8080  alder v0.6.0", tab 1 current, footer ends with the Requests command
func TestFrame_Building(t *testing.T)    // hello with build building → yellow dot "building"
func TestFrame_Problems(t *testing.T)    // failed.ndjson, press p → both diagnostics in check's layout
func TestFrame_Exited(t *testing.T)      // exited.ndjson, press p → "the app exited (code 2)" and the 3 tail lines
func TestFrame_Mismatch(t *testing.T)    // Version v0.6.0, AlderVersion v0.5.0 → the mismatch text in the status bar
func TestFrame_NewBootResets(t *testing.T) // basic then a hello with another boot and no requests → Requests table empty
func TestFrame_Restart(t *testing.T)     // press R → the test HTTP server got POST /_dev/restart
```

Each test drives `teatest.NewTestModel(t, ui.New(opts), teatest.WithInitialTermSize(100, 30))`, feeds the fixture through `opts.Events`, waits with `teatest.WaitFor` for a known string, then `teatest.RequireEqualOutput` against the final frame.

- [ ] **Step 2: Run to verify they fail**

Run: `cd tui && go test ./internal/ui -v`
Expected: FAIL.

- [ ] **Step 3: Implement; generate goldens and read them**

Run `go test ./internal/ui -update`, then open every `.golden` and check it against the spec's mock-ups before committing them. A golden is a claim; read it.

- [ ] **Step 4: Run**

Run: `cd tui && go test ./internal/ui -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add tui/internal/ui tui/cmd/alder-tui/main.go
git commit -m "tui: frame, status bar, footer, keys and Problems overlay (#120)"
```

### Task 16: Requests and Request detail

**Files:**
- Create: `tui/internal/ui/requests.go`, `tui/internal/ui/request.go`, `tui/internal/ui/requests_test.go`, goldens

**Interfaces:**
- Consumes: `store.Requests`, `store.Stats`, `Ctx`, `Options.HTTP`.
- Produces:
  - Requests screen: strip ` last 60s  12.4 req/s  p50 8ms  p95 41ms  errors 0.0%`; rows `time method path status ms bytes steps flags` (time `15:04:05.000` local, status coloured by class, ms yellow when slow, bytes as `4.2K`, flags `slow` and `n+1` (`Repeats > 0`)); newest at the bottom, following until the cursor moves up, `G` follows again; `/` filters rows by any column's text; `enter` opens the detail; `l` switches to Logs with the row's rid filter. Footer command: `alder run · ALDER_LOG=debug`.
  - `type reqDetail struct` from `GET http://<addr>/_dev/requests/<rid>` (fields as PR 2's `ReqRecord`). Header ` ← METHOD path  status  ms  rid R  route P  steps N  db S/Tms`; three sections Steps, SQL (`n` statements, `k` repeat groups), Logs, side by side at ≥ 80 columns, stacked below 80; `tab` moves between them; SQL repeat groups (statements sharing a `repeats` value and text) fold into one row with `×N` and `n+1`, `enter` unfolds showing the args; `y` copies the rid. A 404 shows the summary row it has and the note `the dev server no longer has this request (the last 200 are kept, and a restart clears them)`. Footer command: `curl <addr>/_dev/requests/<rid>`.

- [ ] **Step 1: Write the failing golden tests**

```go
func TestRequests_Table(t *testing.T)       // basic.ndjson → 5 rows; the 500 bold red with slow; /books/42 has n+1
func TestRequests_Filter(t *testing.T)      // "/" then "lend" → one row
func TestRequests_Follow(t *testing.T)      // cursor up, a new event arrives → cursor stays; G → follows to the new row
func TestRequest_Detail(t *testing.T)       // enter on /books/42; fake HTTP returns a detail with 9 repeated selects → one folded "×9  n+1" row; Logs section has 2 lines
func TestRequest_Unfold(t *testing.T)       // tab to SQL, enter on the group → args listed
func TestRequest_Narrow(t *testing.T)       // term 79×30 → sections stacked (golden)
func TestRequest_Gone(t *testing.T)         // fake HTTP 404 → the note text
func TestRequests_LogsJump(t *testing.T)    // l on /books/42 → Logs tab current, rid filter shown in the strip
```

- [ ] **Step 2–4: Fail, implement (read every new golden), pass**

Run: `cd tui && go test ./internal/ui -run 'TestRequests|TestRequest_' -v` (FAIL, then `-update`, read, then PASS with `-count=1`).

- [ ] **Step 5: Commit**

```bash
git add tui/internal/ui
git commit -m "tui: Requests table and Request detail (#120)"
```

### Task 17: Logs, the fixture app and the live test

**Files:**
- Create: `tui/internal/ui/logs.go`, `tui/internal/ui/logs_test.go`, goldens; `tui/testdata/shelf/` (an Alder app); `tui/integration_test.go`
- Modify: `tui/README.md` (keys and screens), `book/11-tooling.md` (a short "Using alder tui" with the keys of PR 3's screens)

**Interfaces:**
- Consumes: `store.Logs`, `store.Filter`, `devproc.Open`, `feed`.
- Produces:
  - Logs screen: lines as `alder run` renders them (stamp dim, level coloured, `effect` lines dim, `run` lines dim with `alder run:`), following until scrolled up, `G` resumes; strip of active filters ` level info  rid 5f3a2c1e  /lend/`; `L` cycles debug → info → warn → error (default info); `/` sets the regex (an invalid one shows its error in the strip and keeps the old one); `x` clears the rid and regex; `enter` on a line with a rid opens that request's detail. Footer command: `ALDER_LOG=<level> alder run`, with `ALDER_LOG_FORMAT=json` added when the level is debug and a rid filter is on (the only way to see rids from the CLI).
  - `tui/testdata/shelf/`: an app named `shelf` with a page `/` listing books, a table `Book` and a table `Loan` with a ref to `Book`, a message that lends a book, and a page `/books/{id}` whose load runs one `Loan` lookup per book in a `Task.all` so the dev panel reports an N+1; migrations included; `alder check` and `alder test` pass. Model it on `examples/todos`.
  - `TestLive_HelloAndFirstRequest` (skipped unless `alder` is on PATH or `ALDER_BIN` set, and skipped under `-short`): copy `shelf` to a temp dir, `devproc.Open` with a free port, wait for `build ok`, GET `/books/1` (seed through the app's own message or a migration's insert), assert a `log` event with `HasAccess` and that event decodes with the same `feed.Decode` the fixtures use, `Repeats ≥ 1`, and `GET /_dev/requests/<rid>` returns steps; then `Close` stops `alder run` (port free within 10 s).

- [ ] **Step 1: Write the failing tests**

```go
func TestLogs_Follow(t *testing.T)        // basic.ndjson at info: the effect line hidden; golden
func TestLogs_LevelCycle(t *testing.T)    // L → debug shows the effect line dim; golden
func TestLogs_Regex(t *testing.T)         // "/" "lend" → one line; "/" "(" → strip shows the regexp error, view unchanged
func TestLogs_OpenRequest(t *testing.T)   // enter on the warn line with a rid → Request detail for that rid
```

- [ ] **Step 2: Run to verify they fail**

Run: `cd tui && go test ./... -run 'TestLogs_|TestLive' -v`
Expected: FAIL (TestLive skips when alder is missing; build `go build -o /tmp/alder ../cmd/alder` from the PR 2 tree and set `ALDER_BIN` to run it locally).

- [ ] **Step 3: Implement the screen, write the app, write the docs**

- [ ] **Step 4: Run everything in the module, with the live test**

Run:

```bash
go build -o "$TMPDIR/alder" ./cmd/alder          # from the repo root
cd tui && ALDER_BIN="$TMPDIR/alder" go test ./... -count=1 -race
```

Expected: PASS with TestLive run (not skipped). Then `cd tui/testdata/shelf && alder check . && alder test .` passes.

- [ ] **Step 5: Add the fixture app to CI**

In the `tui` job, build `alder` from the root first and export `ALDER_BIN`, so `TestLive_HelloAndFirstRequest` runs in CI.

- [ ] **Step 6: Commit and open PR 3**

```bash
git add tui book/11-tooling.md .github/workflows/ci.yml
git commit -m "tui: Logs screen, fixture app and a live test (#120)"
git push -u origin tui-requests-logs
gh pr create --title "alder-tui: skeleton, Requests, Logs (#120, PR 3/4)" --body "<summary, screenshots as text frames from the goldens, test plan>"
```

---

# PR 4 — tui: Tests, Routes, Data

Branch `tui-tests-routes-data` from main after PR 3 merges. Tasks 18–21.

### Task 18: Tests screen

**Files:**
- Create: `tui/internal/ui/tests.go`, `tui/internal/testrun/testrun.go`, `tui/internal/testrun/testrun_test.go`, `tui/internal/watch/poll.go`, `tui/internal/watch/poll_test.go`, `tui/internal/ui/tests_test.go`, goldens

**Interfaces:**
- Consumes: `runner.Runner` (`Stream`), the survey's `test --json` shapes (section 4).
- Produces:
  - `testrun.Result` `{Module, Name, File string; Line, Col int; OK bool; Left, Right, Note, Error string; Trace []string; Ms float64; Property *Property}`; `Property{Runs int; Seed int64; Run, Shrinks int; Args []string; Case string}`; `Mutant{ID, Module, File, Decl, Kind, Before, After, By, Note string; Line, Col int; Killed bool}`; `Score{Score float64; Killed, Survived, Suppressed, Skipped int}`.
  - `type Run struct { Args []string; Results []Result; Diagnostics []feed.Diag; Error string; Mutants []Mutant; Score *Score; Code int; Done bool }`
  - `func Parse(line string, r *Run)` — one NDJSON line into r: a test object, `{"diagnostics":…}`, `{"error":…}`, `{"mutants":…}` or a score object.
  - `func Args(one *Result, mutate bool) []string` — `test --json`; with one: `-run ^<QuoteMeta(Module + " \"" + Name + "\"")>$`; with mutate: `--mutate`.
  - `watch.Poll(ctx context.Context, root string, dirs []string, every, debounce time.Duration) <-chan struct{}` — size+mtime snapshot of every file under `root/<dir>` for `app`, `adapters`, `migrations`; one signal per settled change (debounce 300 ms, every 300 ms).
  - Screen: first open and `r` run the suite; list `module name ms ok|FAIL`; summary `N passed, M failed  Ts`; `enter` on a failure unfolds left/right/note/trace and `property: runs R seed S shrinks K case C`, plus `replays saved case` when `build/<app>/fuzz/<Module>/<hex(sha1(name))[:16]>` exists; `enter` on an unfolded failure reruns it alone; `w` toggles watch (a change during a run queues one rerun, never kills); `m` runs mutation and shows score, the four counts and each surviving mutant as `file:line  before → after`. Footer command: `alder ` + the last run's args joined.

- [ ] **Step 1: Write the failing tests**

```go
func TestParse_Stream(t *testing.T)     // a fixture of 3 test lines (one failing with left/right, one property with case), then nothing → 3 results, 1 failure
func TestParse_Diagnostics(t *testing.T) // {"diagnostics":[…]} → Run.Diagnostics
func TestParse_Mutate(t *testing.T)     // {"mutants":[…2…]} then {"score":0.5,"killed":1,"survived":1,…} → Score and one surviving mutant
func TestArgs_One(t *testing.T)         // Module "Books", Name `lend (twice)` → "-run", `^Books "lend \(twice\)"$`
func TestPoll_Debounced(t *testing.T)   // three writes 50 ms apart under app/ → one signal; a write under static/ → none
func TestTests_Screen(t *testing.T)     // Fake runner; golden of the list and summary
func TestTests_Unfold(t *testing.T)     // enter on the failure → left/right shown; golden
func TestTests_SavedCase(t *testing.T)  // the fuzz file present in a temp build dir → "replays saved case"
func TestTests_WatchQueues(t *testing.T) // a change signal during a running Fake stream → exactly one more call after it ends
func TestTests_Mutate(t *testing.T)     // m → Fake called with --mutate; golden shows score and the surviving mutant
```

- [ ] **Step 2–4: Fail, implement (read every golden), pass**

Run: `cd tui && go test ./internal/testrun ./internal/watch ./internal/ui -run 'TestParse|TestArgs|TestPoll|TestTests' -v -race`

- [ ] **Step 5: Commit**

```bash
git add tui/internal
git commit -m "tui: Tests screen with watch, rerun-one and mutation (#120)"
```

### Task 19: Routes screen

**Files:**
- Create: `tui/internal/ui/routes.go`, `tui/internal/ui/routes_test.go`, `tui/internal/routes/match.go`, `tui/internal/routes/match_test.go`, goldens

**Interfaces:**
- Consumes: `runner.Runner.Run(dir, "routes", "--json")` (`[{kind, path, module, name, file, line, col}]`), `store.Requests`.
- Produces:
  - `routes.Route{Kind, Path, Module, Name, File string; Line, Col int}`; `func Match(routes []Route, path string) (int, bool)` — the index of the route a request path hits: query dropped, segments compared, `{x}` matches any one segment, a literal beats a parameter at the same position (most specific first, as the router sorts).
  - Screen: refreshed on start and on every `build ok`; rows `kind path module.name file:line hits`; `o` opens a GET page without parameters in the browser; `enter` switches to Requests filtered to that route (a route filter on the Requests screen: add it to `Ctx`). Footer command: `alder routes --json`.

- [ ] **Step 1: Write the failing tests**

```go
func TestMatch(t *testing.T)          // routes "/", "/books/{id}", "/books/new" → "/books/new" hits /books/new, "/books/42?x=1" hits /books/{id}, "/nope" none
func TestRoutes_Screen(t *testing.T)  // Fake routes --json + basic.ndjson → hits column 2 for /books/{id}; golden
func TestRoutes_RefreshOnBuild(t *testing.T) // a build ok event → routes --json called again
func TestRoutes_Jump(t *testing.T)    // enter on /books/{id} → Requests tab with only its rows
```

- [ ] **Step 2–4: Fail, implement (read every golden), pass**

Run: `cd tui && go test ./internal/routes ./internal/ui -run 'TestMatch|TestRoutes' -v`

- [ ] **Step 5: Commit**

```bash
git add tui/internal
git commit -m "tui: Routes screen with hits (#120)"
```

### Task 20: Data screen

**Files:**
- Create: `tui/internal/data/data.go`, `tui/internal/data/guard.go`, `tui/internal/data/data_test.go`, `tui/internal/ui/data.go`, `tui/internal/ui/data_test.go`, goldens

**Interfaces:**
- Consumes: `GET http://<addr>/_dev/map.json` (PR 2, Task 9), `store.Requests` for the wrote trigger.
- Produces:
  - `data.Map{Build string; Tables []Table; Silos []Silo; Migrations []string}`; `Table{Name, SQL, Silo string; Cols []Col}`; `Col{Name, Type, SQL, SQLType string}`; `Silo{Key, Ref, Path string; Tables []string}`.
  - `func Open(path string) (*DB, error)` — `file:<path>?mode=ro&_pragma=query_only(1)&_pragma=busy_timeout(200)` with the modernc driver; `Close`.
  - `func (db *DB) Count(ctx context.Context, t Table) (int64, error)`; `func (db *DB) Page(ctx context.Context, t Table, orderBy string, desc bool, offset int) (cols []string, rows [][]string, err error)` (100 rows; identifiers quoted; `orderBy` must be one of `t.Cols[].SQL`, else the id).
  - `func Guard(stmt string) error` — strips leading whitespace and `--`/`/* */` comments; the first word, case-insensitive, must be `select`, `with` or `explain`; a `;` followed by anything but whitespace or comments is refused. Messages: `only select, with and explain run here (the database is opened read-only)` and `one statement at a time`.
  - `func (db *DB) Query(ctx context.Context, stmt string) (cols []string, rows [][]string, err error)` — `Guard` first; at most 1 000 rows; 5 s context timeout.
  - `func Databases(buildDir string, m Map) []Choice` — `Choice{Label, Path string}`: `shared` (`dev.db`), then one per file under `<buildDir>/silos/<key>/` for each silo as `<key>/<value>`; only shown as a selector when `m.Silos` is non-empty. Siloed tables are listed only for silo databases, shared tables only for `shared`.
  - Screen: left pane tables (Alder name, row count) then the applied migrations; right pane the selected table's rows (100 per page, `[`/`]` page, `s` sorts by the column under the cursor, `enter` shows one row vertically with long text unfolded); `:` prompt runs `Query` and shows the result or the guard's message; counts refresh (debounced 500 ms) after a request whose method is not GET/HEAD and whose `DBStmts > 0`, or any POST; the map is refetched on `build ok`. A per-table error (missing table, locked database) shows on that table's row in red and the rest keeps working; all database calls run in `tea.Cmd`s, never in `Update`. Footer command: `sqlite3 build/<app>/dev.db`, or with a prompt active `sqlite3 build/<app>/dev.db '<the SQL>'`.

- [ ] **Step 1: Write the failing tests**

```go
func TestGuard(t *testing.T)            // "select 1" ok; "  -- c\nWITH x AS (select 1) select * from x" ok; "explain query plan select 1" ok; "delete from t" refused; "select 1; drop table t" refused; "select 1;  " ok
func TestOpen_ReadOnly(t *testing.T)    // a temp db with a table; Query("select …") rows; a write through db's own handle (bypassing Guard via an unexported test hook) fails with a read-only error
func TestCountPageSort(t *testing.T)    // 150 rows → Count 150; Page offset 100 → 50 rows; orderBy "title" desc first row is the max; orderBy "nope" falls back to the id
func TestMissingTable(t *testing.T)     // Count of a Table whose SQL is absent → error, not panic
func TestDatabases_Silos(t *testing.T)  // build dir with silos/org/a1.db and b2.db → shared, org/a1, org/b2
func TestData_Screen(t *testing.T)      // map fixture + temp db → golden with counts and the first page
func TestData_Prompt(t *testing.T)      // ":" "delete from x" enter → the guard's message in the pane; golden
func TestData_ErrorRow(t *testing.T)    // map lists a table missing from the db → that row red with the error; other tables still count
func TestData_WroteRefresh(t *testing.T) // a POST access event with db_stmts 2 → Count called again after the debounce (fake clock)
```

- [ ] **Step 2–4: Fail, implement (read every golden), pass**

Run: `cd tui && go test ./internal/data ./internal/ui -run 'TestGuard|TestOpen_ReadOnly|TestCountPageSort|TestMissingTable|TestDatabases|TestData' -v -race`

- [ ] **Step 5: Commit**

```bash
git add tui/internal
git commit -m "tui: Data screen, read-only browsing with a select-only prompt (#120)"
```

### Task 21: Finish the local milestone

**Files:**
- Modify: `tui/internal/ui/root.go` (remove the placeholders), `tui/README.md`, `book/11-tooling.md` (the Tests, Routes and Data screens), `tui/integration_test.go` (extend)

**Interfaces:**
- Consumes: everything above.

- [ ] **Step 1: Extend the live test**

`TestLive_Screens`: on the `shelf` copy, with real `alder`, run the Tests screen's suite through `runner.Exec` and assert ≥ 1 passing result; fetch `/_dev/map.json` and assert `Book`'s SQL name is the one `sqlite_master` in `build/shelf/dev.db` lists; run `routes --json` and assert `/books/{id}` is present.

- [ ] **Step 2: Run the module and the root suite**

Run: `cd tui && go test ./... -count=1 -race` with `ALDER_BIN` set as in Task 17, then from the root `scripts/test.sh`.
Expected: PASS, TestLive tests run.

- [ ] **Step 3: Try it by hand**

Run `go build -o "$TMPDIR/alder-tui" ./cmd/alder-tui` in `tui/`, then in a copy of `tui/testdata/shelf` run it with the PR tree's `alder` as `ALDER_BIN`: open each tab, open a request, filter logs by rid, run the tests, run a `:` query, quit, and check `lsof -i :8080` is empty. Note anything that felt wrong in the PR description.

- [ ] **Step 4: Commit and open PR 4**

```bash
git add tui book/11-tooling.md
git commit -m "tui: local milestone complete (Tests, Routes, Data) (#120)"
git push -u origin tui-tests-routes-data
gh pr create --title "alder-tui: Tests, Routes, Data (#120, PR 4/4 of the local milestone)" --body "<summary, test plan, what PRs 5–6 add>"
```
