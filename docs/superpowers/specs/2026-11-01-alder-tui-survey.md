# alder machine interfaces — survey for alder-tui (2026-11-01)

Paths under ~/projects/alder. Source: Explore subagent (read-only), saved by the main session.

## 1. `alder run` (cmd/alder/run.go)
- `runRunWith` (run.go:56): `[--build-dir DIR] [dir]`; listens on `ALDER_ADDR`, default `localhost:8080` (run.go:43, 137-142).
- Watcher: `watch.WatchReady`, poll 300ms (internal/watch/watch.go:21), seam `watch.WatchTicks` (watch.go:54).
- `rebuild()` (run.go:445-498): `runCheckTo` in-process; on failure re-runs check `--json` → `parseDiags` (run.go:447-456); `buildApp(dir, build/<app>)` (build.go:90); `startChild` (run.go:572, `exec.Command(binary)`); swaps the reverse proxy; stops old child (SIGTERM, SIGKILL after 5s, run.go:628-638); broadcasts `clearEvent`+`reloadEvent`.
- Child env (`childEnv`, run.go:553-562): `ALDER_ADDR=127.0.0.1:0 ALDER_DEV=1 ALDER_RELOAD=<nonce>-<n> ALDER_DB=<build>/dev.db ALDER_STATIC=<build>/static ALDER_EXIT_ON_STDIN_EOF=1`, `ALDER_PUBLIC_URL=http://<dev addr>` unless set. Child stdin is a pipe; EOF → child exits (run.go:533-544; internal/gen/caps.go:847).
- Child prints `listening on <addr>` to stdout (caps.go:844); `startChild` waits ≤10s for it (run.go:546, 598-623).
- Proxy: `httputil.ReverseProxy`, `FlushInterval:-1` (SSE ok) (run.go:209-221). All but `/_dev/reload` and `/_dev/theme*` proxied (run.go:225-250). No child → 502 HTML (run.go:256-267).
- Output: stdout once `dev server on <addr> → app on <child addr>` (run.go:487). stderr: `alder run: building|rebuilding|rebuilt|the app exited (...)` (run.go:378,384,403,489,517), check/build errors as text, child stdout/stderr passthrough with TTY colouring of `^YYYY/MM/DD HH:MM:SS (info|warn|error):` (run.go:685-707). Nothing machine-readable.
- Live reload: `/_dev/reload?b=<gen>` SSE of Datastar `PatchEvent`s (run.go:276-283, 290-342): clear/reload/overlay(HTML); `: ping` every 15s. Static-only change → copy static + reload (run.go:405-414).
- Build dir `build/<app>/`: generated module, `rt/ adapters/ caps/ static/ migrations/ cmd/main.go`, binary `<app>`, `dev.db`, `scratch/test|mutate|test.lock`, `fuzz/`, `deploy/<id>/`. **No pid/state file.**

## 2. Logging (rt/applog, rt/web)
- Generated main: `applog.FromEnv(os.Stderr)` (internal/gen/caps.go:789). All logs to **stderr**; no file.
- `ALDER_LOG` debug|info|warn|error (applog.go:44); `ALDER_LOG_FORMAT` text|json (applog.go:64,97); `ALDER_SLOW_MS` (applog.go:291). Invalid → `alder: <err>` exit 1 (caps.go:786-801).
- Text: `YYYY/MM/DD HH:MM:SS <level>: <msg>` UTC; newlines escaped `\n\t` (applog.go:151-168).
- Access (applog.go:196-227): text `GET /start?key=••••&seed=7 200 12.3ms 4821B rid=5f3a2c1e`; `steps=N` at Debug; slow: `... rid=X slow route=/todos/{id} steps=12 db=9/640.1ms` at Warn. JSON (slog): `time level msg(=METHOD path) method path status ms bytes rid`, +`steps` at Debug, +`slow:true route steps db_stmts db_ms` when slow.
- Access level: Info; Debug for quiet (static, `/_alder/*`, `/_dev` 2xx/304) (router.go:231-240); Warn when slow. Health and 200 metrics scrapes not logged (router.go:454-457,469).
- Masking `applog.MaskURL` (rt/applog/mask.go); path cut 4 KB.
- Request id: `X-Request-Id` kept if `[A-Za-z0-9._-]{1,64}` else `NewID()` 8 hex (rt/applog/id.go:40); echoed on response (router.go:451).
- `web.Options.Access func(*applog.Access)` (router.go:46), called per routed request (router.go:469-482). `Access{ID Method Path URL Masker Status Dur Bytes Quiet Steps Slow Route Stmts DBDur}`. Wired `Access: lg.Access` (caps.go:822).
- `SlowAfter` (router.go:97,479); `eff.Stats` collected only when metrics or SlowAfter on (router.go:758-760).
- Router error lines `METHOD path: text rid=<id>` (router.go:217-229).
- **User `Log.*` lines carry no rid** (rt/std/effects.go:77-96, cut 8 KB). Effect step lines `effect <name>` at Debug, no rid (rt/eff/eff.go:373, run.go:215; applog.go:124-150).
- **Recorder** `eff.Recorder{Steps []StepRec{Name,Ms,Cap}; SQL []SQLRec{Stmt,Args,Ms,Rows,Group}; Loads; Cuts}` (rt/eff/eff.go:383-401). Created only when `rt.panelOn()` (`ALDER_DEV=1` && `ALDER_DEV_PANEL!=0`, rt/web/devpanel.go:22; router.go:765-770). Rendered **into the HTML dev panel** (devpanel.go:59 `panelInner`, `patchPanel` devpanel.go:372). N+1 via `warnRepeats` (devpanel.go:267). Not exposed as JSON.

## 3. Endpoints
- `GET|HEAD /_alder/health` (router.go:556-557,630-657): JSON `{"build":"<ALDER_BUILD>","db":"ok"}` / 503 `unreachable` or maintenance sentence. Never logged.
- `GET|HEAD /_alder/metrics` (router.go:560-587; rt/metrics): only if `ALDER_METRICS` set (`open` or ≥16-char bearer token, rt/metrics/env.go:11-55; `open` refused on Render/Fly/ssh, env.go:56-70, warns elsewhere). Prometheus text. Series: `alder_http_requests_total{route,method,status}`, `alder_http_request_duration_seconds` (buckets .005…10), `alder_http_requests_in_flight`, `alder_db_statements_total{kind}`, `alder_db_statement_seconds_total{kind}`, `alder_effect_steps_total{name}` (cap 1000), go goroutines/heap/gc, `alder_process_start_time_seconds`, `alder_build_info{build}`. Route label = pattern or `/static/ /_alder/ /_dev (none)` (router.go:598-612).
- `GET /_dev` (app's router, router.go:525; `devMap` devpanel.go:455): **HTML** only when `Options.Dev` && `ALDER_DEV=1`; lists build id, pages, tables, session fields, capabilities, applied migrations, routes. Input is the generator's JSON `Options.DevMap` (router.go:54, devpanel.go:~427).
- Dev server (alder run): `/_dev/reload` SSE; `/_dev/theme` HTML; `/_dev/theme/state` JSON; `POST /_dev/theme/import` `{css}`→`{theme,unplaced,unplacedCss,skipped}`; `POST /_dev/theme/write` `{theme}`; same-origin check `themeAllowed` (cmd/alder/theme_ui.go:95-155).
- No admin/trace/SQL JSON endpoint.

## 4. `--json` surfaces
- With `--json` (cmd/alder/helpjson.go): check, routes, eval, scratch, test, bench, deploy, help, all `new *`/edit/remove/fill/regen/`theme import|use`. **Without**: status, rollback, logs, restore, silos, run, build, fmt, dump, migrate, repl, ci, version, explain.
- `help --json` (helpjson.go:297-304): `{version, commands:[cmdSpec{name,summary,usage?,args[],required?[],flags[{name,value?,multi?,summary}],subcommands?}]}`.
- `check --json` (main.go:176-240): NDJSON `jsonDiag{code,severity,file,line,col,endLine,endCol,message,fix?{line,col,endLine,endCol,replace}}` (internal/diag/render.go:49-60), 1-based; nothing when clean; exit 0/1/2.
- `routes --json` (routes.go:20-27): array `{kind(page|message),path,module,name,file,line,col}`.
- `test --json` (test.go:81-120,355-462; rt/testing/testing.go:551-570): **streaming NDJSON** per test `{module,name,file,line,col,ok,left?,right?,note?,error?,trace[],ms,warnings?,bench?,property?{runs,seed,run,shrinks,args[],case?}}`; summary text on stderr; check failure → `{"diagnostics":[...]}`; other → `{"error":...}`; exit 0/1/2. `--mutate --json` (mutate.go:331-354,405-430): `{"mutants":[{id,module,file,decl,line,col,kind,before,after,killed,by,note,tables?}]}` then `scoreJSON{score,killed,survived,suppressed,skipped}`. Flags `-run -timeout --fuzz --seed --min`. Saved fuzz failures `build/<app>/fuzz/<Module>/<sha1(name)[:16]>` (property.go:510-513), not JSON. Lock `build/<app>/scratch/test.lock`.
- `bench --json`: per-result `bench[{step,ms,bytes,alloc,allocs,requests,loads,steps,sql,repeats[]}]` (bench.go:152-168).
- `eval --json` (eval.go:28-34): `{value,type,diagnostics,error?,hint?}`. `scratch --json` (scratch.go:28-34): `{watches[WatchResult{line,end,expr,type?,value?,error?,kind?,note?,truncated?}],diagnostics,ran,note?,error?}`; `--watch --json` one object per run.
- Generators `--json` (generate.go:59-90): `genJSON{files[{path,action}],diff,diagnostics,holes[],migration?,written,error?,...}`.
- `deploy --json` (deploy.go:292-300): stdout `deployJSON{...Record, address?}`; progress text on stderr; failure `{"error":...}`.

## 5. Deploy / ops
- `Target` (internal/deploy/target.go:15-69): Describe Connect Requires Schema BuildSchema Current Builds Upload Backup Migrate Start Health Switch Stop Record Prune Info Logs(ctx,follow,n,since,w) Restore(db) Close; optional `DataLossAllower Rerouter MigrationRefuser Locker Preflighter BackupRestorer SiloStatuser`. Impls Ssh/Fly/Render/Script (Script refused by status/rollback/logs/restore, cmd/alder/deploy.go:679-681).
- `deploy.Status` (deploy.go:768): text only. Lines: describe; `current <id>|none`; `builds` ≤5 rows `  * <id>  <note>`; `database <size>`; `backup <RFC3339> (<from>)`; `info.Lines`; `SiloTable` (deploy.go:840). `Info{DBBytes,HaveDB,LastBackup,BackupFrom,StreamTo,Shipped,Lines}`; `Build{ID,Record*,Current,Running,Modified,BackupOf}`.
- `SiloStatus{key,id,state,detail?}` (rt/db/stores.go:1279-1284) via built binary `--silo-status`; states ok migrating pending-data-loss ahead failed orphaned.
- `Record` (record.go:15-47): `{build,app,env,target,version,commit?,platform,gate:GateResult{check,warnings?,build,tests?{passed,failed},mutate?{score,floor,floorFrom,mutants,survived},checks?[],skipped?},plan:Plan{known,firstDeploy?,live?,build?,migrations[{name,steps,dropsData?}],noMigrations?},requires[],allowDataLoss?,unsafeSkipTests?,backupURL?,when?,by?,backup?,applied?,previous?,port?,rollback?,restoredFrom?}`. Local `build/<app>/deploy/<id>/alder-deploy.json`; remote `builds/<id>/deployed.json`. **History lives on the remote.** Ssh layout `$R/current`, `$R/builds/<id>/{log,deployed.json,backup.db}`, `db/app.db`.
- Gate/ship rows via `line(w, step, status, note)` (gate.go:129-134) `  %-8s %s  %s`; statuses ok FAIL SKIPPED MISMATCH; steps check build test mutate <declared> | requires upload backup migrate start health switch prune stop; headings gate plan ship target builds artifact deployed warning: DROPS DATA. `deploy.StyleLine/Styled` regex colouriser (style.go:17-60).
- Rollback (deploy.go:508-645; cmd runRollbackWith deploy.go:749): lock; pick previous; refusals (no earlier, ambiguous prefix, unfinished build, already current); schema compare → `schema FAIL`/`MISMATCH` (deploy.go:608-619); start→health→switch→`recordRollback`→drain stop (`ALDER_DEPLOY_DRAIN` 30s). `--restore-backup` (deploy.go:705-754) needs `BackupRestorer`, confirms (`--yes`; no TTY → refuse), Preflight, keeps `db/app.db.before-rollback-<time>`, rolls back on health failure.
- Logs (cmd/alder/deploy.go:822-940): `-n` 100 default; `--since` duration|RFC3339|date; `--grep` regexp. Ssh: tail `$R/builds/<current>/log`, follows build switch (ssh.go:1553). Fly: `flyctl logs` (fly.go:1033). Render: API, 100/page, `RenderSincePages`=20, follow over WebSocket (render.go:1074-1130, render_ws.go), lines `[instance] message`. `sinceWriter` parses text stamp, JSON `{"time":...}`, RFC3339 prefix (deploy.go:962-1045).
- Restore (deploy.go:1086-1270): `--to` layouts (parseRestoreTime :1326), `--from`, `--out file.db`, `Target.Restore` stop-all/restart/health/revert. Turso path :1270. Text only.
- Config: `deploy` declaration in the app's `.alder` source (`envTarget`, cmd/alder/deploy.go:666-692); kinds ssh fly render script; `--host` for multi-host (deploy.go:437-470). Env vars: `ALDER_SSH_CONFIG ALDER_DEPLOY_SUPERVISOR ALDER_DEPLOY_CADDY_ADMIN ALDER_DEPLOY_DRAIN ALDER_DEPLOY_HEALTH_TIMEOUT FLY_API_TOKEN FLY_ORG RENDER_API_KEY ALDER_RENDER_API TURSO_API_TOKEN ALDER_TURSO_API USER`. `alder ci init --github` runs `alder deploy <env> --json` (ci.go:385).

## 6. Import rule, module, CI
- No enforcing test/CI step; convention in docs (specs/2026-10-30-metrics-design.md:49; plans/2026-10-18-m6b-plan1-lsp.md:15). `TestDistModule` (rt/embed_test.go:70-125) pins go.mod/go.sum to rt/dist copies.
- `go.mod`: `module alder`, `go 1.27`, `require modernc.org/sqlite v1.60.1`. `internal/` not importable from another module (style, diag, watch…). Nested modules already exist (examples/*/go.mod, experiments/**). `./...` stops at module boundaries.
- CI `.github/workflows/ci.yml`: changes→warm→lint (`gofmt -l $(git ls-files '*.go')`, `go vet ./...`, python), test (shards; cmd/alder ×4; internal/gen; rest), examples, deploy (docker sshd), deploy-systemd, mutation nightly; macOS on main/tags. `scripts/test.sh` local. Also pages.yml, release.yml. A `tui/` module needs its own job (gofmt would touch it, vet/test would not).

## 7. Styling `alder/internal/style` (style.go)
- `For(w)`, `ForEnv`, `Plain`, `NoStyle`; `ALDER_COLOR=never|always`, else TTY && !NO_COLOR && TERM not dumb (style.go:73-90). `Width()` from terminal/COLUMNS, floor 40, default 80.
- 16 ANSI only: Bold Dim Red Green Yellow Cyan BoldRed BoldGreen BoldYellow (style.go:108-120). `Mark`: ok green; FAIL/error bold red; warning/SKIPPED yellow (124-135). `Severity`. `Wrap Columns ColumnsStyled Lines/LineWriter Strip`.
- Looks: diagnostics `file:line:col:` bold, `error[E0001]:` bold red, `^^^` underline, `fix:` cyan (internal/diag/render.go:20-37); log line dim stamp + coloured level (run.go:685-707); deploy rows (internal/deploy/style.go:31-55); `alder run` announce bold, `alder run:` dim; count line bold red/yellow (main.go:~240).

## 8. Other interfaces
- LSP (internal/lsp/server.go:35-67): standard methods + custom `alder/routes` (`[{kind,path,module,name,uri,range}]`, routes.go:20-43), `alder/generators`, `alder/preview`, `alder/plan` (generators.go:14-22), `workspace/executeCommand` `alder.explain|alder.fill|alder.generate` (actions.go:158,293-315), notification `alder/scratch` (protocol.go:65-74). `ALDER_LSP_LOG=<file>`.
- `alder dump`: text only. `internal/watch`: `SourceChange|StaticChange` (watch.go:13-17), not importable.
- Dev DB `build/<app>/dev.db` is plain SQLite (readable `mode=ro`).

## Gaps for a TUI (nothing exists today)
- No `alder run` event stream / JSON; no Recorder endpoint; no rid on `Log.*`/effect lines; no `status --json`, `rollback --json`, `logs` JSON; no pid/state file; no env list; deploy progress text only (seam: `line()` gate.go:129).
