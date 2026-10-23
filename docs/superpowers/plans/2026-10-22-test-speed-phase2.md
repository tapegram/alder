# Test speed phase 2: parallel-safe cmd/alder, no real sleeps — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Cut `go test ./cmd/alder` from ~555 s wall (baseline 2026-10-22: 257 tests, 495 s summed, almost all serial) by making the tests parallel-safe and removing real-time waits (issue #24).

**Architecture:** Tests change directory only because build output is cwd-relative (`build/<app>`). A `--build-dir` option fixes that, so `t.Chdir` goes away. Process-global state is replaced by values passed in: env reads, `os.Setenv("ALDER_HOST")`, and the `onDevServer`/`initSource` globals. Independent tests then call `t.Parallel()`. The deploy code's real waits get a sleep seam beside the existing `Now` seams, and shell poll and drain intervals become parameters.

**Tech Stack:** Go, `testing`, shell scripts embedded in `internal/deploy/ssh.go`.

**Spec:** GitHub issue tapegram/alder#24, plus the code map in `.superpowers/sdd/test-speed2/map.md`. The baseline is `.superpowers/sdd/test-speed2/before.json` (`go test -json`).

## Global Constraints

- CLI behaviour is unchanged by default: without `--build-dir`, output still goes to `./build/<app>`, relative to the cwd. Book and docs examples stay valid.
- Production timing is unchanged: the default sleep is `time.Sleep`, and default poll intervals and drain values keep today's numbers. Only tests inject fakes or small values.
- No test may lose an assertion. A test whose real wait *is* the behaviour under test (e.g. "SIGINT stops within 2 s", "the 20 s delay must be virtual") keeps its real timing. It may still run in parallel.
- A test stays serial if it needs `t.Setenv` for something that cannot reasonably be passed in, such as `PATH`-based fakes. Leave a one-line comment saying why.
- Every task ends with `go test ./cmd/alder -count=1 -timeout 30m` green, plus `go vet ./...`. `go test -race` must be clean on the packages touched.
- Commit after each task, with a message ending in the repo's Co-Authored-By line.

## Review Focus

1. Two parallel tests building the same example (e.g. `examples/todos`) with different `--build-dir` values must not touch each other's files. Nothing may write into `examples/` or `testdata/` in the repo.
2. `alder build examples/todos` with no `--build-dir`, run from the repo root, still produces `./build/todos/todos`. Pin this in a test.
3. The `alder deploy` script hook still sees `ALDER_HOST` in its environment, now via `cmd.Env` instead of `os.Setenv`.
4. Fake-clock PITR: the restore point is still strictly after every write made before the backup. The fake Turso/DB must take its timestamps from the same injected clock.
5. Flakiness under load: run the whole package 3× with `-count=1` (also `-race` on one run) after Tasks 1 and 3. Any test failing in any run is a blocker.

---

### Task 1: `--build-dir`, no `t.Chdir`, parallel non-deploy tests

**Files:**
- Modify: `cmd/alder/build.go` (`runBuildTo`, `buildApp` default at :133), `eval.go:141`, `repl.go:138`, `run.go:407`, `test.go` (scratch build path), `deploy.go:176,313,510`, and `migrate.go` if it builds. Also `help.go`/`helpjson.go` usage strings.
- Modify: `cmd/alder/run.go:104-115` (`onDevServer`, `runAddr`) and `deploy_init.go:17` (`initSource`).
- Tests: `eval_test.go`, `repl_test.go`, `build_test.go`, `test_test.go`, `smoke_test.go`, `run_unix_test.go`, `run_test.go`, `migrations_test.go`, `generate_test.go`, `deploy_init_test.go`, plus `t.Parallel()` across the package wherever it is now allowed.

**Interfaces:**
- Produces: `func buildRoot(flagValue string) string`, which returns `flagValue` if it is set, else `"build"`. Every `filepath.Join("build", …)` in cmd/alder becomes `filepath.Join(buildRoot(bd), …)`.
- Produces: the flag `--build-dir DIR`, accepted by build, eval, repl, test, run, deploy and migrate. Each command whose output is under `build/` gets it, documented in its usage line.
- Produces: `runRunTo` gets its addr default and its dev-server hook without globals. Add an unexported `runOpts struct{ addr string; onDevServer func(*devServer) }` and a `runRunWith(args, stdout, stderr, stop, runOpts)` that `runRunTo` calls with `runOpts{addr: os.Getenv("ALDER_ADDR")}`. Tests call `runRunWith`.
- Produces: `deploy_init` takes its source func as a parameter: `runDeployInitWith(args, stdout, stderr, source func() …)`, called by the wrapper with `version.SourceOf`.

- [ ] **Step 1: Failing test.** Add `TestBuild_BuildDir` to `build_test.go`. It runs `alder build --build-dir <tmp> ../../examples/todos` with no Chdir and asserts that `<tmp>/todos/todos` exists and `./build` (cmd/alder/build) does not. Add `TestBuild_DefaultBuildDir`: inside `t.Chdir(tmp)` (this test stays serial), `alder build <abs todos>` produces `tmp/build/todos/todos`.
- [ ] **Step 2: Run them.** Run `go test ./cmd/alder -run 'TestBuild_(Default)?BuildDir' -count=1`. Expect FAIL: `TestBuild_BuildDir` fails because the flag is unknown.
- [ ] **Step 3: Implement** `buildRoot` and the flag in every command above. eval, repl and test derive their scratch paths from `appBuild`, so nothing else should need changing. Grep afterwards: `grep -n '"build"' cmd/alder/*.go | grep -v _test` should show only usage/help strings.
- [ ] **Step 4: Run them.** Expect PASS.
- [ ] **Step 5: Remove globals.** Replace `onDevServer`, `runAddr`'s env read and `initSource` as described in Interfaces. Update `run_test.go` (:96, :100, :573, :577) and `deploy_init_test.go` (:153-166) to pass the values in, with no `t.Setenv`.
- [ ] **Step 6: Drop `t.Chdir`.** Remove it from every listed test file. Tests pass absolute dirs and `--build-dir t.TempDir()`; helpers like `tempTodos`, `inTemp`, `tempApp` and `evalIn` return or take the build dir. `test_test.go:760` re-execs the binary: give the child the build dir via its args, not via cwd. `generate_test.go:780` uses `GoCmd(filepath.Join(buildDir, base), …)`.
- [ ] **Step 7: Mark tests parallel.** Add `t.Parallel()` as the first line of every top-level test in cmd/alder that no longer uses `t.Chdir`/`t.Setenv` and touches no shared files. Leave the deploy tests (Task 3) and any `t.Setenv` user alone. Each remaining serial test gets a one-line `// serial: <reason>` comment.
- [ ] **Step 8: Verify.** Run `go test ./cmd/alder -count=1 -timeout 30m -json > .superpowers/sdd/test-speed2/after-task1.json` and note the wall time (the final `pass` Elapsed). Then run twice more with `-count=1`, once with `-race`. Expect all green, and `git status` showing no new files under `examples/`, `testdata/` or `cmd/alder/build`.
- [ ] **Step 9: Commit.** `test speed: --build-dir, drop t.Chdir, parallel cmd/alder tests (#24)`

### Task 2: sleep seam for deploy, no real waits in deploy tests

**Files:**
- Modify: `internal/deploy/ssh.go:657-662`, `internal/deploy/render.go:711`, `internal/deploy/turso.go:56` (or wherever `DB.Now` lives), and `internal/deploy/deploy.go:63,493` (Options/Rollback).
- Modify: the shell scripts in `internal/deploy/ssh.go` (:323, :767, :886, :1074, :1315, :1537).
- Modify: `rt/backup/silos.go` only if a test waits on its ticker. The interval is already a parameter; pass a small value from the test.
- Tests: `cmd/alder/deploy_ssh_test.go` (:625-638, :737, :746, :757, :759), `deploy_silos_test.go` (:160-165, :215, :385-387), `deploy_render_test.go:215-217`, `deploy_caddy_test.go:379`.

**Interfaces:**
- Produces: `Sleep func(time.Duration)` next to every existing `Now func() time.Time` seam (Options, Rollback, the Turso DB, Render). nil means `time.Sleep`. The PITR wait becomes `d := next.Sub(now())`, then `sleep(d + 50ms)`. Never call `time.Until` on a value from an injected clock.
- Produces: the test helper `fakeClock` in `cmd/alder/clock_test.go`, with `Now()` and `Sleep(d)`. Sleep advances the clock without waiting. It is mutex-guarded.
- Produces: a shell poll interval. The scripts read `${ALDER_POLL:-<today's value>}` for each `sleep` in a poll loop. `Ssh` gets a `Poll time.Duration` field; when non-zero it is exported into the remote script env as `ALDER_POLL`. Fixed `sleep 1` waits that are not polls become polls if possible; otherwise they also scale with `ALDER_POLL`.

- [ ] **Step 1: Failing unit test.** In `internal/deploy`, add `TestSsh_BackupPITRUsesInjectedClock`. Inject a fake clock at `12:00:00.300` and assert that Backup returns without real waiting (elapsed < 100 ms), that the fake Sleep was called with ≥ 700 ms, and that the recorded point is `12:00:01`.
- [ ] **Step 2: Run it.** Expect FAIL: it really sleeps ~750 ms, or Sleep is not called.
- [ ] **Step 3: Implement** the Sleep seam in ssh.go and render.go, and thread it through from Options/Rollback.
- [ ] **Step 4: Run it.** Expect PASS.
- [ ] **Step 5: Use the fake clock in the cmd/alder deploy tests.** Replace each `time.Sleep(1100ms)` "cross a second" wait with `clock.Sleep`/advance, and make the fake Turso/Render servers stamp writes and restore points from the same clock. Set drain to 0 or `Poll` to 10 ms instead of sleeping 1.5–3 s for drain windows. Wherever a test must still observe an outcome, it polls for it with a 20 ms interval and a generous cap, instead of sleeping a fixed time.
- [ ] **Step 6: Verify.** Run `go test ./internal/deploy/... ./rt/backup/... -count=1` and `go test ./cmd/alder -run 'TestDeploy' -count=1 -json`. Record each test's time before and after in the commit message body. The target is every deploy test under 5 s, unless its time is real build or ssh work rather than waiting.
- [ ] **Step 7: Commit.** `test speed: injectable sleep for deploy PITR and drain, fake clock in deploy tests (#24)`

### Task 3: deploy env into options, parallel deploy tests

**Files:**
- Modify: `cmd/alder/deploy.go` (:44-45, :160-186; env reads; `ALDER_HOST` Setenv) and `internal/deploy/ssh.go` (`EnvSSHConfig`, `EnvSupervisor`, `EnvCaddyAdmin` reads).
- Tests: `deploy_*_test.go`, `startcheck_test.go:58-64`, `ci_test.go:134`, `build_test.go:453`.

**Interfaces:**
- Produces: `runDeployWith(args, stdout, stderr, deployEnv)`. `deployEnv` is a struct holding Drain, HealthTimeout, SSHConfig, Supervisor, CaddyAdmin, Turso settings, and `Getenv func(string) string` for anything rarer. `runDeployTo` builds it from `os.Getenv`. The `internal/deploy` Ssh fields replace its own env reads; the `Env*` constants stay for main to read.
- Produces: the script hook gets `ALDER_HOST` via `cmd.Env = append(os.Environ(), "ALDER_HOST="+host)`. No `os.Setenv` remains in non-test cmd/alder code: `grep -n 'os.Setenv\|os.Unsetenv' cmd/alder/*.go | grep -v _test` is empty.

- [ ] **Step 1: Failing test.** Add `TestDeploy_ScriptSeesHostWithoutProcessEnv`. Two subtests with `t.Parallel()`, hosts `a.example` and `b.example`, each assert that its script hook saw its own `ALDER_HOST`, and that `os.Getenv("ALDER_HOST")` is unset afterwards.
- [ ] **Step 2: Run it.** Run with `-count=3 -race`. Expect FAIL or a race report.
- [ ] **Step 3: Implement** `runDeployWith`/`deployEnv`, the `cmd.Env` change and the Ssh fields.
- [ ] **Step 4: Run it.** Expect PASS.
- [ ] **Step 5: Convert the tests.** Turn the deploy tests' `t.Setenv` calls into `deployEnv` fields and add `t.Parallel()`. `PATH`-faked tests (fake ssh at deploy_ssh_test.go:150, actionlint at ci_test.go:134) take an explicit binary path if the code has a single lookup point; otherwise they stay serial with a comment. The loopback box must be safe for concurrent deploys: give each test its own remote root and ports. If it can't be (e.g. a shared fake Caddy admin port), group those tests under one serial parent with parallel children where possible.
- [ ] **Step 6: Verify.** Run the full package 3× with `-count=1`, and once with `-race`. Write the final `-json` to `.superpowers/sdd/test-speed2/after.json`. Re-rank the 20 slowest tests (the jq ranking used for the baseline) and append the before/after table to this plan under `## Results`.
- [ ] **Step 7: CI.** Shard weights from `.github/shard.sh` may be stale now. Regenerate them from `after.json` the way that script documents, and check that the `every cmd/alder test is in exactly one shard` step still passes locally if it can run.
- [ ] **Step 8: Commit.** `test speed: deploy env via options, parallel deploy tests (#24)`
