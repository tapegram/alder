# alder-tui polish (#149, #150) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the open polish items from #149 and #150 in the `alder-tui` module, one PR.

**Architecture:** No new features. Each task touches one package or screen and fixes the items listed for it, test-first where behaviour changes. The item numbers (#N) refer to the triage table: `docs/superpowers/plans/2026-11-01-alder-tui-polish-triage.md`. That table has the file:line and the minimal fix for every item. Read your task's rows before you start.

**Tech Stack:** Go, Bubble Tea, Lip Gloss, `charmbracelet/x/ansi`. Nested module `tui/` (`alder/tui`).

**Spec:** `.superpowers/plans/2026-11-01-alder-tui-design.md` (untracked, in the main checkout `~/projects/alder`). The issues #149 and #150 are the requirement. The spec only matters where an item names a behaviour.

## Global Constraints

- Work in the worktree `~/projects/alder-tui-polish` on branch `tui-polish`. Never touch `~/projects/alder`.
- Verify a task with `cd tui && go test ./... -race`. Task 1 also runs `go test ./...` in the root module, without the TestLive tests.
- Add no new module dependencies. `x/ansi` is already a dependency.
- Match the existing style: doc comments on every exported name, comments wrapped at about 100 columns, tests named `TestScreen_Behaviour`.
- One commit per task. End each message with `(#149)`, `(#150)` or `(#149, #150)`.
- Leave items marked FIXED or MOOT in the triage alone. Item 31 ("fix hinted" in Problems) is out of scope: it needs a feed field on the alder side. The controller files it as an issue.

## Rulings (apply across tasks)

- **R1, View never mutates.** Clamp cursor, scroll and `top` in Update, on keys, `tea.WindowSizeMsg` and data messages. Each screen gets one `clamp()` that runs at the end of Update. View reads the state as it is. Covers items 27, 43 and 61.
- **R2, footer priority.** When the footer is too narrow, first truncate the command text with `…`, then drop the screen's keys from the end. `? keys` and the screen's `esc back` are never dropped. Covers items 33 and 41.
- **R3, errors reach the screen.** A failure after the first load shows as one yellow line on that screen. It never wipes the content already shown. Covers items 28 and 63.

## Review Focus

1. Quitting the TUI during a long `alder test --mutate` leaves no `alder` child running. Test this in Task 3.
2. A half-open feed connection, with no pings and no bytes, turns into `disconnected` within the idle deadline. It must not stay `connected` forever. Test this in Task 4.
3. If an owned `alder run` dies after connecting, the status bar says it exited and gives the code. A bare `disconnected` is not enough. Test this in Task 5.
4. At widths 79, 80 and 100 no line is wider than the terminal, and `? keys` stays visible. Golden or width assertions in Tasks 6–8.
5. A wide-rune log line, such as CJK text or emoji, is cut at a rune boundary and the output is valid UTF-8. Test this in Task 6.

---

### Task 1: Repo trivia (Sonnet)

Items 2 (re-check the ci.yml tui comment, 22-24 and 400-403, and fix it if any part still reads garbled), 3, 67, 68, 69.

**Files:** `.github/workflows/ci.yml`, `tui/README.md`, `tui/testdata/shelf/app/Books/Show.alder`, `tui/integration_test.go`

- [ ] Quote `"$ALDER_BIN"` in ci.yml. Add `left` to the README row-detail keys. Delete "(book 11)".
- [ ] `copyApp` skips `*.db`, `*-wal` and `*-shm` as well as `build/`.
- [ ] Run `cd tui && go test ./... -race` and `go vet ./...` in both modules. Both pass. Commit.

### Task 2: alderbin, main, status-bar mismatch (Sonnet)

Items 6, 7, 8, 29.

**Files:** `tui/internal/alderbin/alderbin.go` (+ test), `tui/cmd/alder-tui/main.go`, `tui/internal/ui/statusbar.go`

- [ ] Write the failing tests:
  - `TestVersion_Missing`: a fake `help --json` with no version makes `Version` return a non-nil error that mentions "version".
  - `TestFind_RelativePath`: with PATH=`rel` and an `alder` executable in `./rel`, the path that comes back passes `filepath.IsAbs`.
  - `TestMismatch_Short`: the mismatch text is at most 30 cols and looks like `alder 0.6.0 ≠ tui 0.5.0`.
  - `TestStatusBar_Mismatch100`: at width 100, all five tab labels are in the status bar.
- [ ] Implement the fixes. The version check runs with dir `""`. main checks first that `dir` exists and is a directory, and otherwise exits with `alder-tui: <dir>: not a directory`.
- [ ] `go test ./... -race` passes. Commit.

### Task 3: runner cancel semantics and Tests screen (Opus)

Items 10, 11, 12, 13, 54, 55, 56, 57, 59.

**Files:** `tui/internal/runner/{runner.go,fake.go,runner_test.go}`, `tui/internal/testrun/testrun.go` (+ test), `tui/internal/ui/{tests.go,tests_test.go,root.go}`, `tui/internal/watch/poll_test.go`

**Interfaces:**
- `Stream.Wait() (code int, stderr string, err error)`. `err` is `ctx.Err()` when the stream was cancelled. A second call returns the same values, cached with `sync.Once`. `Run` returns `ctx.Err()` the same way. Update every caller and `Fake`.
- The Tests screen has `Stop()`. Root calls `Stop()` on every screen that implements `interface{ Stop() }` before `tea.Quit`. Task 5 uses the same hook.

- [ ] Write the failing tests:
  - `TestStream_MissingBinary`: Start returns an error and no goroutine is left behind.
  - `TestStream_Cancel`: a `sleep 30` stream cancelled after the first line makes Wait return `context.Canceled` within 2 s.
  - `TestWait_Twice`: two Wait calls give equal results.
  - `TestFake_CRLF`: Fake strips `\r` the way Exec does.
  - `TestParse_BadDiagnostic`: bad diagnostic or mutant JSON sets `Run.Error`.
  - `TestTests_RerunKeepsScore`: after a one-test rerun, the earlier Score, Mutants and Diagnostics are still there.
  - `TestTests_Empty`: an empty list shows `no tests`.
  - `TestTests_Error`: when `alder` is missing, the screen shows the stderr text.
  - `TestTests_StopCancels`: `Stop()` cancels the stream's ctx.
- [ ] Implement. Tests keeps a `cancel` for the run it is streaming. A `cur() (testrun.Result, bool)` helper replaces the bounds checks at tests.go:247 and :463.
- [ ] Make `poll_test.go` deterministic. Use the existing interval seam if there is one. If not, change the waits to wide-margin polling, `eventually(t, 2*time.Second, …)`, and drop the fixed sleeps.
- [ ] `go test ./... -race -count=3 ./internal/runner/... ./internal/watch/...` passes, then the full suite. Commit.

### Task 4: feed client robustness (Opus)

Items 14, 15, 16.

**Files:** `tui/internal/feed/{event.go,client.go,feed_test.go}`, `tui/internal/store/logs.go` (gap marker), `tui/internal/ui/logs.go` (renders the marker)

**Interfaces:**
- `feed.Options.Idle time.Duration`. The default is 45 s, three times the server's 15 s `eventPing`. The `http.Client` has `Transport.ResponseHeaderTimeout = 10 s`. Every line, pings included, pushes the read deadline back. When the deadline passes, the stream ends with `errIdle`, and the reconnect loop treats that like any other drop.
- A new event kind is `feed.Gap{Missed uint64}`. It is emitted when a reconnect within the same boot gets `hello.First > last+1`. The Logs store appends it as a dim line: `… N events missed (ring overflow)`.
- A log record that fails typed decode is kept: `Log{Raw: …, Record: nil}`. It is shown as raw text.

- [ ] Write the failing tests:
  - `TestClient_IdleDeadline`: the server sends hello and then nothing, with `Idle=200ms`. The client reports a disconnect within 1 s.
  - `TestClient_Gap`: on reconnect, the same boot answers with `First=last+5`, so the client emits `Gap{Missed:4}`.
  - `TestDecode_BadAttrKeepsRaw`: a record with `"repeats":"x"` decodes to a Log whose Raw is set.
  - `TestLogs_GapMarker`: a store-level test of the marker line.
- [ ] Implement. `go test ./... -race` passes. Commit.

### Task 5: devproc lifecycle (Opus)

Items 18, 19, 20, 21, 22, 28.

**Files:** `tui/internal/devproc/{devproc.go,devproc_test.go}`, `tui/internal/ui/{root.go,statusbar.go}`

**Interfaces:**
- `devproc.Options.StartTimeout time.Duration`. The default is 30 s and replaces the const.
- `Session.Exited() <-chan Exit`, with `Exit{Code int, StderrTail string}`. It is nil when the session is attached. Root turns it into a message. The status bar then shows `alder run exited (code N)` in red, in place of `disconnected`, and the Logs screen gets the stderr tail as error lines.
- `feedClosedMsg` sets `ctx.Disconnected = true` (R3).

- [ ] Write the failing tests:
  - `TestOpen_StartTimeout`: a child that never serves, with `StartTimeout=300ms`, gives an error that contains the stderr tail.
  - `TestOpen_ChildExitsEarly`: the child exits 3 before serving, and the error names the code.
  - `TestOpen_NoGoroutineLeak`: after a failed Open, `runtime.NumGoroutine` settles back to the baseline, checked with a polling loop.
  - `TestDirCheck_Symlink`: an app dir that is reached through a symlink, such as t.TempDir under /var, matches itself.
  - `TestProbe_HelloBeatsTimer`: a hello that arrives in the same tick as the probe deadline counts as an answer. Use an injected clock or the existing seam.
  - `TestSession_Exited`: killing the owned child after connect delivers an `Exit` value.
  - `TestRoot_ExitedStatus`: the status bar shows the exit code.
  - `TestRoot_FeedClosed`: a `feedClosedMsg` makes the status bar show `disconnected`.
- [ ] Implement. `filepath.EvalSymlinks` runs on both sides and falls back to `Clean` on error. Failed-Open paths drain stderr in a goroutine.
- [ ] `go test ./... -race` passes. Commit.

### Task 6: look, Logs, Problems, footer (Sonnet)

Items 9, 23, 25, 26, 27, 33, 46 (the logs part), 48, 49, 50, plus the R2 footer and R1 for Problems.

**Files:** `tui/internal/look/look.go`, `tui/internal/ui/{footer.go,logs.go,problems.go}` and their tests, `tui/internal/store/store_test.go`

- [ ] Write the failing tests:
  - `TestColumns_Styled`: a cell styled with lipgloss lines up with the unstyled cells. Measure with `ansi.StringWidth`.
  - `TestLogs_TruncateRune`: a line of `界` cut at an odd byte limit is valid UTF-8 (`utf8.ValidString`) and fits within the limit.
  - `TestLogs_RidRegexDebug`: the store test for Filter `{Level: "debug", Rid, Re}`.
  - `TestLogs_RingWrap`: on the screen, with a ring of 3 and 5 lines, the cursor and the filtered index stay valid.
  - `TestLogs_Regex`: drive the rid filter with the Requests `l` key. Do not set `LogFilter.Rid` directly.
  - `TestFooter_Priority`: at widths 79 and 100, with a long command, `? keys` and `esc back` are present and the line is no wider than the width.
  - `TestProblems_TailStyled`: tail lines carry the level colour.
  - `TestProblems_ViewPure`: calling View twice on an out-of-range cursor does not change it.
- [ ] Implement. Problems formats tail lines through the same render path as Logs, without allocating a store for each line. Typed strip strings go through `clean()`.
- [ ] `go test ./... -race` passes. Commit.

### Task 7: Requests table and detail (Sonnet)

Items 34, 35, 36, 37, 39, 41, 42, 43, 44, 45, 46 (the requests part), 47.

**Files:** `tui/internal/ui/{requests.go,request.go,requests_test.go}`

**Interfaces:**
- Consumes Task 6's footer (R2).
- A new `stripTickMsg` comes from `tea.Tick(time.Second)`. It is scheduled only while the Requests screen is the current tab, and it is re-armed on each tick.

- [ ] Write the failing tests:
  - `TestRequests_StripOutOfOrder`: a record older than 60 s followed by a newer one, and the newer one is counted.
  - `TestRequests_StripDecays`: after advancing the injected clock by 61 s and sending a tick, the strip is empty.
  - `TestRequests_StableColumns`: the column widths do not change when scrolling.
  - `TestRequests_ViewPure` (R1).
  - `TestRequests_RowsCached`: `rows()` does not call the matcher again while the filter, route and `End()` are unchanged. Use a counting matcher seam.
  - `TestRequest_EscapedRid`: a rid with `/` and space is requested escaped.
  - `TestRequest_BodyCap`: a body over the cap gives an error line and no hang. The cap is 4 MiB, the same as the Data screen's limit if that one differs.
  - `TestRequest_ServerError`: a 500 shows an error line.
  - `TestRequest_Loading`: the screen shows `loading…` before the answer arrives.
  - `TestRequest_Header79`: at 79 cols the header wraps onto 2 lines and keeps the db figures.
- [ ] Implement. Put 2 spaces between the SQL row fields. The column widths are measured over all rows, as routes.go does.
- [ ] `go test ./... -race` passes. Commit.

### Task 8: Routes screen (Sonnet)

Items 51, 52, 53.

**Files:** `tui/internal/ui/{routes.go,routes_test.go}`

- [ ] Write the failing tests:
  - `TestRoutes_RestartByBoot`: a new `Hello.Boot` resets the hit counts even when the seq is past the old scan.
  - `TestRoutes_Narrow60`: at width 60 no line is wider than 60.
  - `TestRoutes_OpenHint`: the footer shows `o open` on an openable route and leaves it out on other routes.
- [ ] Implement. `go test ./... -race` passes. Commit.

### Task 9: Data screen (Sonnet)

Items 60, 61, 62, 63, 64, 65, 66.

**Files:** `tui/internal/ui/{data.go,data_test.go}`, `tui/internal/data/{data.go,data_test.go}`

**Interfaces:**
- `data.Databases` runs inside the map fetch `tea.Cmd`. Its result rides on `dataMapMsg` as `Choices []data.Choice`.
- A silo label decodes the file stem with the same encoding the runtime uses for `{value}`. Grep `rt/` for the silo path encoder, which is lowercase base32hex. If the stem does not decode, the label shows the stem.
- Queries read `limit+1` rows. `Page.Truncated bool` drives the "first 1000 rows" note.

- [ ] Write the failing tests:
  - `TestData_NoEnterAfterRefused`: the footer does not offer `enter row` after a refused query.
  - `TestData_ViewPure` (R1).
  - `TestData_MapRefetchError` (R3): after a failed refetch, the yellow line shows and the old tables are still there.
  - `TestQuery_Exactly1000`: there is no truncation note at exactly 1000 rows, and there is one at 1001.
  - `TestOpen_ReadOnlyMode` and `TestOpen_QueryOnly`: each layer is tested alone, and each refuses a write on its own.
  - `TestDatabases_DecodedLabel`: the label shows the decoded id.
- [ ] Implement. `go test ./... -race` passes. Commit.

### Final

- [ ] Opus whole-branch review against #149, #150 and this plan.
- [ ] Run the full `tui` suite with the TestLive tests and the root `scripts/test.sh`.
- [ ] Open a PR that closes #149 and #150 and lists item 31 as filed separately.
