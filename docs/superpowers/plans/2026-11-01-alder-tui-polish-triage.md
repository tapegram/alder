# Triage of #149 and #150 against origin/main 3fb3894a
Paths are under /Users/tavishpegram/projects/alder-tui-polish. `tui/i/` = `tui/internal/`.
Compound checklist bullets are split into sub-items. F = FIXED, O = OPEN, M = MOOT.

## Non-tui files (root module)
| # | Issue | Item | Class | Evidence / fix site / minimal fix |
|---|---|---|---|---|
| 1 | 149 T10 | book/11-tooling.md + .goreleaser.yaml "no -ldflags" | FIXED | book/11-tooling.md:1883-1887 and .goreleaser.yaml:15-18 now say alder passes none, alder-tui gets `-X main.version`; ldflags at .goreleaser.yaml:51-52. Header reads coherently. |
| 2 | 149 T10 | ci.yml tui comment garbled | FIXED (low confidence) | .github/workflows/ci.yml:22-24 and 400-403 read coherently. File untouched since #148, so confirm against the original complaint. |
| 3 | 149 T17b | ci.yml `$ALDER_BIN` unquoted | OPEN | ci.yml:437 `run: $ALDER_BIN check . && $ALDER_BIN test .` -> quote both as `"$ALDER_BIN"`. |

## tui/cmd/alder-tui/main.go
| # | Issue | Item | Class | Evidence / fix |
|---|---|---|---|---|
| 4 | 149 T13 | usage string backticks | FIXED | main.go:39 has no backticks. |
| 5 | 149 T15 | Close not deferred | FIXED | main.go:120 `defer stop()` (stop also runs at :128). |
| 6 | 149 T11 | version check runs in `dir` (bad dir gives opaque chdir error) | OPEN | main.go:66 `Run(ctx, dir, ...)` -> pass "" (or os.Getwd), or stat `dir` first for a clear error. |

## tui/i/alderbin
| # | Issue | Item | Class | Evidence / fix |
|---|---|---|---|---|
| 7 | 149 T10 | Version returns "" with no error when help lacks version | OPEN | alderbin.go:38 -> `if help.Version == "" { return "", errors.New("... reports no version") }`. |
| 8 | 150 Other | relative PATH entry may give relative alder path | OPEN | alderbin.go:19-23 returns lookPath's `p` as is -> `filepath.Abs(p)`. |

## tui/i/look
| # | Issue | Item | Class | Evidence / fix |
|---|---|---|---|---|
| 9 | 149 T10 | Columns measures rune count; styled cells misalign | OPEN | look.go:68-86 uses utf8.RuneCountInString. Use ansi.StringWidth (x/ansi is already a dep) or document "cells must be unstyled". Callers today style only the last column (ui/tests.go:350), so latent. |

## tui/i/runner
| # | Issue | Item | Class | Evidence / fix |
|---|---|---|---|---|
| 10 | 149 T11 | Run/Wait lose ctx.Err() on cancel (Code -1, nil err) | OPEN | runner.go:59-63 (Run), :108-112 (Wait) -> after a failed Wait, if ctx.Err()!=nil return it. Stream.Wait has no error return, so the interface needs one or the stderr needs a note. |
| 11 | 149 T11 | second Wait returns -1 | OPEN | runner.go:105-113 calls cmd.Wait twice -> cache code/stderr in a sync.Once. |
| 12 | 149 T11 | Exec strips \r, Fake doesn't | OPEN | runner.go:91 strips; fake.go:67 only splits on "\n" -> TrimSuffix "\r" in Fake. |
| 13 | 149 T11 | no Stream start-failure / cancel tests | OPEN | tui/i/runner/runner_test.go has only RunCode, RunMissingBinary, StreamLines, StreamHugeLine, Fake -> add missing-binary Stream and cancelled-ctx tests. |

## tui/i/feed
| # | Issue | Item | Class | Evidence / fix |
|---|---|---|---|---|
| 14 | 149 T12 | Decode drops log event whose record fails typed decode | OPEN | event.go:178-180 returns error -> on error keep `ev.Log` with Raw only (Record nil). |
| 15 | 149 T12 | no idle deadline / response-header timeout | OPEN | client.go:69 uses http.DefaultClient; next() at :92-119 blocks forever -> custom client with ResponseHeaderTimeout plus a read deadline reset on each line (server pings). |
| 16 | 149 T12 | no contiguity check hello.First vs last+1 | OPEN | client.go:136-153 -> on same boot, if hello.Hello.First > last+1 emit a gap marker or reset. |
| 17 | 149 T12 | test feedServer records ?after but never applies it | FIXED | feed_test.go:176-195 (`serve`) applies ?after like run_events.go. |

## tui/i/devproc
| # | Issue | Item | Class | Evidence / fix |
|---|---|---|---|---|
| 18 | 149 T13 | 30 s cap and stderr-tail error / child-exits-early untested; make startTimeout injectable | OPEN | devproc.go:32 is a const used at :138,:168 -> move to Options.StartTimeout (default 30s) and add tests. Only 9 tests, no deadline test. |
| 19 | 149 T13 | stderr reader goroutine can leak on failed Open | OPEN | devproc.go:166-171 (deadline, ctx) stop without draining p.Stderr(); goroutine at :300-316 blocks on `p.stderr <- l` (cap 64) -> drain in a goroutine on those paths, like :179-182 does for success. |
| 20 | 149 T13 | dir check compares Clean(abs) only (symlinks, /var vs /private/var) | OPEN | devproc.go:92,99 -> filepath.EvalSymlinks both sides, fall back to Clean on error. |
| 21 | 149 T13 | probe timer edge: hello arrived but timer fired first -> errNoAnswer | OPEN | devproc.go:219-220 -> check `err == nil && hello received` before treating fctx.Err() as no-answer. |
| 22 | 149 final review | owned `alder run` dying after feed answers shows only `disconnected` | OPEN | devproc.go:179-183 and `session` (:230) never watch p.Exited() -> on exit emit a synthetic app/exit event or a Session.Exited channel. |

## tui/i/store, ui (frame, problems, statusbar, footer)
| # | Issue | Item | Class | Evidence / fix |
|---|---|---|---|---|
| 23 | 149 T14 | no test for Rid+Re on debug re-render | OPEN | store_test.go:231 covers Re at debug, no Rid+Re combination -> add a case with Filter{Level:"debug", Rid:..., Re:...}. |
| 24 | 149 T14 | no test for Requests ring wraparound | FIXED | store_test.go:187-200 `TestRequests_At` uses NewRequests(3) over a longer fixture and checks Len==3 and At() order. |
| 25 | 149 T15 | Problems tail lines unstyled | OPEN | problems.go:99,126-136 `tailLine` returns plain `.Text` -> use the renderLine path (ui/logs.go:233). |
| 26 | 149 T15 | tailLine allocates NewLogs(1) per line per View | OPEN | problems.go:133 -> decode and format without a store, or cache per Tail. |
| 27 | 149 T15 | problems.view clamps cursor (View mutates) | OPEN | problems.go:80 -> clamp in problemsKey / on build event. |
| 28 | 149 T15 | feedClosedMsg dropped silently | OPEN | root.go:280-281 `return m, nil` -> set m.ctx.Disconnected = true. |
| 29 | 149 T15 | mismatch text at 100 cols pushes tabs 2-5 off | OPEN | alderbin.go:47 text is long, shown at statusbar.go:83-84 -> shorter form such as `alder X != tui Y`. |
| 30 | 149 T15 | stand-in screens copy stores every frame | MOOT | The stand-in screens are gone: root.go:159-161 builds real Data, Tests, Routes screens, and nothing named standin/placeholder remains. |
| 31 | 149 Parked | Task 15 "fix hinted": feed.Diag has no fix field | OPEN (spec gap, not a tui bug) | feed/event.go:49-58 has no Fix, and cmd/alder/run_events.go has none. Needs an alder-side feed field first. |
| 32 | 149 Parked | Task 17b orphaned app, not reproduced | MOOT | Parked as unreproduced; devproc stdin-EOF design (devproc.go:1-4) plus TestProc_DiesWithParentPipe cover it. |
| 33 | 149 T17a | footer drops `? keys` on Logs at 100 cols | OPEN | footer.go:12,34,47-49 drops from the end; logs.go:294 keys + globals + cmd exceed 100 cols -> put `?` first among globals or shorten the keys. |

## tui/i/ui/request.go (Requests detail)
| # | Issue | Item | Class | Evidence / fix |
|---|---|---|---|---|
| 34 | 149 T16 | narrow header cut (db figures lost) | OPEN | request.go:258-263 builds one long line -> wrap or drop fields below 80. |
| 35 | 149 T16 | rid not url.PathEscape'd | OPEN | request.go:83 -> `url.PathEscape(rid)`. |
| 36 | 149 T16 | detail body decoded without size cap | OPEN | request.go:103 -> `io.LimitReader(resp.Body, N)` as data.go:127 does. |
| 37 | 149 T16 | no test for non-404 error branch or loading state | OPEN | requests_test.go has Gone but no 500 or pre-answer test; the branches are at request.go:275-278. |
| 38 | 149 T16 | Logs pane floor 8 at 100 cols; cap SQL ~60% | FIXED | request.go:135-138 `logsFloor = max(8, room/4)`, :286 SQL capped at room*55/100. |
| 39 | 149 T16 | SQL row inner spacing 1 space | OPEN | request.go:384 `sqlChrome` and :439-440 use single spaces between count, ms, rows -> widen to 2. |
| 40 | 149 T16 | sectionLines doc comment collapsed onto one long line | FIXED | request.go:331-333 is wrapped over 3 lines. |
| 41 | 149 T16 | footer at 79 cols drops `y copy rid  esc back` | OPEN | footer.go:47-49 drops trailing keys; the long curl command at requests.go:361 eats the room -> truncate cmd first or lead with `esc back`. |

## tui/i/ui/requests.go (Requests table)
| # | Issue | Item | Class | Evidence / fix |
|---|---|---|---|---|
| 42 | 149 T16 | table column widths from visible rows only | OPEN | requests.go:268-277 -> measure over all rows (as routes.go:203-215 does) or fixed minimums. |
| 43 | 149 T16 | View mutates s.top | OPEN | requests.go:262-266 -> clamp in Update (needs height) or accept and drop the item. |
| 44 | 149 T16 | strip assumes time-ordered arrival | OPEN | requests.go:323-329 `break` at the first old row -> `continue` (bounded by 200-5000 rows). |
| 45 | 149 T16 | strip doesn't tick without a redraw | OPEN | No tick anywhere in ui except data.go:132 -> add a tea.Tick 1s message on the Requests screen. |
| 46 | 149 T17a | user-typed strip strings not cleaned | OPEN | requests.go:338,340 use raw `s.filter`; logs.go:~262 `"/"+s.input` -> wrap in clean(). |
| 47 | 150 Routes | Requests rows() re-matches up to 5000 rows per call | OPEN | requests.go:169-186 runs MatchRequest per row on each View/key -> cache idx keyed by (filter, route, Requests.End()). |

## tui/i/ui/logs.go
| # | Issue | Item | Class | Evidence / fix |
|---|---|---|---|---|
| 48 | 149 T17a | text[:limit] byte cut can split a rune | OPEN | logs.go:237-238 -> back up to a rune boundary (utf8.RuneStart loop) or use ansi.Truncate. |
| 49 | 149 T17a | TestLogs_Regex pokes LogFilter.Rid directly | OPEN | logs_test.go:131 `m.ctx.LogFilter.Rid = ...` -> drive via the Requests `l` key (TestRequests_LogsJump path). |
| 50 | 149 T17a | ring-overwrite idx pruning untested at screen level | OPEN | No wrap or overflow case in logs_test.go (only store-level TestLogs_Cap) -> small-ring screen test. |

## tui/i/ui/routes.go
| # | Issue | Item | Class | Evidence / fix |
|---|---|---|---|---|
| 51 | 150 Routes | store-restart detection by Seq only | OPEN | routes.go:151 `Seq < s.scanned` misses a restart that already passed the old seq -> also reset on a Hello.Boot change (c.Hello.Boot) or Requests Reset counter. |
| 52 | 150 Routes | path column min width overflows below 80 cols | OPEN | routes.go:216 `max(..., 4)` -> let non-path columns shrink or truncate the line to width. Same pattern at requests.go:285. |
| 53 | 150 Routes | footer lacks an `o` hint | OPEN | routes.go:234-236 returns only enter. Global `o browser` (footer.go:12) is shadowed by the claim at :58-60 -> add Key{"o","open"} when the cursor route is Openable. |

## tui/i/ui/tests.go, testrun, watch
| # | Issue | Item | Class | Evidence / fix |
|---|---|---|---|---|
| 54 | 150 Tests | stream uses context.Background(), not cancelled on quit | OPEN | tests.go:113 -> keep a cancel on the screen, cancel on quit (needs a Stop hook from root) and fix runner so cancel yields ctx.Err (item 10). |
| 55 | 150 Tests | testrun Unmarshal errors for diagnostics and mutants discarded | OPEN | testrun.go:124,126 -> set r.Error (or a ParseErr field) on failure, as the Result and Score branches do. |
| 56 | 150 Tests | rerunning one test drops previous score / mutants / diagnostics | OPEN | tests.go:103 `s.run = &testrun.Run{...}` -> when one != nil, copy Score, Mutants, Diagnostics from the old run. |
| 57 | 150 Tests | no test for empty list or error path; ad hoc bounds in enter/Keys | OPEN | tests_test.go has 6 tests, none for `no tests` or stderr (body at tests.go:364-375); bounds at tests.go:247 and :463 -> add a `cur() (Result, bool)` helper and the tests. |
| 58 | 150 Tests | shellJoin may duplicate quoting logic used for F/y copy | MOOT | tests.go:485-501 is the only shell quoting in tui/. F/y copy goes through each screen's Command(), which calls shellJoin (data.go:808). No duplicate. |
| 59 | 150 Tests | watch/poll_test.go uses real timers | OPEN | poll_test.go:14,50,65 use time.After and time.Sleep with short windows -> inject a ticker or widen the margins. |

## tui/i/ui/data.go, data package
| # | Issue | Item | Class | Evidence / fix |
|---|---|---|---|---|
| 60 | 150 Data | footer offers "enter row" after a refused query | OPEN | data.go:780-781 -> drop `enter row` when s.query.page.err != "" or rows is empty. |
| 61 | 150 Data | View mutates scroll state | OPEN | data.go:725-728 (s.top) and :770 (s.dtop) -> clamp in Update/key. |
| 62 | 150 Data | silo directory listing runs in Update | OPEN | data.go:352 setMap -> data.Databases (os.ReadDir, data.go:153) -> move the listing into the fetch cmd and send it in dataMapMsg. |
| 63 | 150 Data | failed map refetch after first load shows nothing | OPEN | data.go:277-280 sets mapErr, but View only shows it when !loaded (:547-551) -> show a yellow line in the left pane when loaded && mapErr != "". |
| 64 | 150 Data | read-only test can't tell which layer refused | OPEN | data/data_test.go:107-111 accepts "readonly" or "read-only" or "query_only" -> test mode=ro and query_only separately. |
| 65 | 150 Data | "the first 1000 rows" shown at exactly 1000 | OPEN | data.go:671-673 and data.go:270 (rows stops at limit) -> read limit+1 and flag truncation. |
| 66 | 150 Data | silo labels show file stem not decoded id | OPEN | data/data.go:166-167 (documented at :142-143) -> decode base32hex (check the root module's silo file naming). |

## Docs
| # | Issue | Item | Class | Evidence / fix |
|---|---|---|---|---|
| 67 | 150 Other | README row-detail key list omits `left` | OPEN | tui/README.md:105 (Data row view: `esc`, `h`, `enter` or `backspace`) -> add `left` (code at data.go:423). The Requests detail table at README.md:85 already has `left`. |
| 68 | 149 T17b | Show.alder stray "(book 11)" note | OPEN | tui/testdata/shelf/app/Books/Show.alder:12 -> delete "(book 11)". |
| 69 | 149 T17b | copyApp copies stray *.db/-wal/-shm | OPEN | tui/integration_test.go:253-263 skips only `build` -> skip `*.db`, `*-wal`, `*-shm`. |

## Parked / final-review notes (149)
Items 22, 31 and 32 above.

## Overlaps between the two issues
- Item 54 (150 Tests, stream not cancelled) depends on item 10 (149 T11, runner loses ctx.Err). Fix them together.
- Item 52 (150 Routes path min width) is the same pattern as requests.go:285. 149 T16 has a nearby narrow-terminal complaint (items 34, 41).
- Items 43 and 61 (149 T16 / 150 Data): "View mutates state" appears in 149 T15 (27), 149 T16 (43), 150 Data (61). One decision, "clamp in Update vs accept in View", settles them all.
- 150 Data "map refetch error" (63) and 149 T15 feedClosedMsg (28) both come down to errors that never reach the screen.
- Items 7 and 8 both touch alderbin.go.
- Items 4, 17, 38, 40 and 24 were counted in the issue but are already done.
