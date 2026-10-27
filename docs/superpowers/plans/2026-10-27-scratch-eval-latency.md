# Scratch Eval Latency Implementation Plan (alder#44, part 1)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A changed scratch expression evaluates in ~0.55 s instead of ~1.9 s on a real app, with no diagnostic lost.

**Architecture:** Scratch runs keep the compile-and-run pipeline but (1) vet only the first build of a session (the LSP server, a `--watch` loop) and after a failed build, as the REPL already does; (2) link the scratch binary stripped; (3) skip gofmt on the generated module files for scratch runs. Nothing changes for `alder build`, `alder test` or the REPL.

**Tech Stack:** Go (`internal/scratch`, `internal/lsp`, `internal/gen`, `cmd/alder`), `go test`.

**Spec:** `docs/superpowers/specs/2026-10-27-scratch-send-latency-design.md`

## Global Constraints

- `alder scratch` one-shot still vets; `alder build`, `alder test`, the REPL are untouched.
- A build that fails under `NoVet` is still explained by vet (existing `CompileCheckedWith` path; keep `TestCompileCheckedWith_NoVet` green).
- Branch from `main`; one PR; commits end with the session's attribution lines.
- Keep `go test ./...` green; run the full suite once at the end (it is slow), package tests per task.

## Review Focus

1. Two scratch documents open in one server: the second document's first run must not vet again (the flag is per server, not per document) — pinned in Task 1's test.
2. A `--watch` first run that is cancelled by an edit before it builds: the rerun is the first real build, but skips vet; acceptable (a failing build still vets) — noted, not tested.
3. A stripped binary's panic trace must still name the watch that crashed (`frameOf` uses markers, not symbols) — covered by the existing `TestRunFileWatchPanicIsolated`, which Task 3 reruns.
4. `NoFormat` output must still parse for `topLevelNames` (gen's duplicate-name check parses each file) — pinned in Task 4's test.
5. `go version -m` / `debug/buildinfo` reports `-ldflags` only when the linker recorded it; if the assertion in Task 3 proves unreliable on this toolchain, assert on the binary size shrinking instead (strip removes DWARF: the file must be smaller than the unstripped build of the same program).

---

### Task 0: Branch and commit the spec

**Files:**
- Commit: `docs/superpowers/specs/2026-10-27-scratch-send-latency-design.md`, `docs/superpowers/plans/2026-10-27-scratch-eval-latency.md`, `docs/superpowers/plans/2026-10-27-scratch-send-nvim.md`

- [ ] **Step 1:** `git checkout -b scratch-eval-latency` from `main`.
- [ ] **Step 2:** `git add docs/superpowers && git commit -m "docs: scratch send + eval latency design and plans (#44)"`.

### Task 1: LSP vets the first scratch run only

**Files:**
- Modify: `internal/lsp/scratch.go` (`scratchRuns` struct ~line 28; `scratchOnce` call to `runScratchFile` ~line 173)
- Test: `internal/lsp/scratch_test.go` (`scratchCalls` ~line 40, `fakeScratch` ~line 22; template test `TestScratchSaveReruns` ~line 78)

**Interfaces:**
- Produces: `scratchRuns.vetted bool` (under `scratchRuns.mu`); `runScratchFile` is called with `FileOptions.Build = scratch.Options{NoVet: <vetted before this run>}`.

- [ ] **Step 1: Record `NoVet` in the fake.** In `scratchCalls` add `noVet []bool`; in `fakeScratch`'s stub append `o.Build.NoVet` beside `texts` (the stub's `_ scratch.FileOptions` parameter becomes `o`). Add `func (c *scratchCalls) vets() []bool` returning a copy.
- [ ] **Step 2: Write the failing test** `TestScratchVetsOncePerServer`, modelled on `TestScratchSaveReruns`: open `scratch.alder`, save it, then open a second scratch file `other.scratch.alder` (if the harness makes that awkward, save the first one twice instead). Assert `calls.vets()` is `[]bool{false, true, true}` once three calls have been recorded.
- [ ] **Step 3:** `go test ./internal/lsp -run TestScratchVetsOncePerServer` — expected FAIL (all `false`).
- [ ] **Step 4: Implement.** Add `vetted bool // a scratch run has built with vet: later runs skip it (NoVet)` to `scratchRuns`. In `scratchOnce`, before `runScratchFile`: lock `s.scratch.mu`, `noVet := s.scratch.vetted; s.scratch.vetted = true`, unlock; pass `Build: scratch.Options{NoVet: noVet}` in the `FileOptions`.
- [ ] **Step 5:** `go test ./internal/lsp` — expected PASS.
- [ ] **Step 6:** Commit: `lsp: vet the first scratch run only (#44)`.

### Task 2: `alder scratch --watch` vets the first run only

**Files:**
- Modify: `cmd/alder/scratch.go` (`scratchRunArgs` ~line 245; `scratchRun`'s `RunFile` call ~line 280; `scratchWatch`'s goroutine where `ra := a` ~line 212)
- Test: `cmd/alder/scratch_test.go`

**Interfaces:**
- Produces: `var runScratchFile = scratch.RunFile` in `cmd/alder/scratch.go` (test seam, as `internal/lsp` has); `scratchRunArgs.noVet bool`.

- [ ] **Step 1: Write the failing test** `TestScratchWatchVetsFirstRunOnly`: replace `runScratchFile` with a stub (restore in `t.Cleanup`) that records `o.Build.NoVet` and returns `scratch.FileResult{Watches: ...}` as `blockScratch` in the lsp tests does (one `WatchResult` per `f.Watches`, `Ran: true`); start a watch with `startScratchWatch`, `edit` the file once, wait until two calls are recorded, stop; assert `[]bool{false, true}`. Also assert the one-shot path: `runScratchCtx` without `--watch` records `false`.
- [ ] **Step 2:** `go test ./cmd/alder -run TestScratchWatchVetsFirstRunOnly` — expected FAIL (compile error: no `runScratchFile`).
- [ ] **Step 3: Implement.** Add the `runScratchFile` var and call it from `scratchRun`; add `noVet bool // --watch reruns: the first run vetted` to `scratchRunArgs`; in `scratchWatch`'s goroutine set `ra.noVet = reread`; in `scratchRun` pass `Build: scratch.Options{NoVet: a.noVet, Waiting: ..., Fetching: ...}`. Update the `runScratchCtx` doc comment (the `--watch` paragraph): "reruns skip `go vet`; a build that fails is still vetted to explain it".
- [ ] **Step 4:** `go test ./cmd/alder -run 'TestScratch'` — expected PASS.
- [ ] **Step 5:** Commit: `scratch: --watch reruns skip go vet (#44)`.

### Task 3: Strip the scratch binary

**Files:**
- Modify: `internal/scratch/scratch.go:311` (the `go build` call in `CompileCheckedWith`)
- Test: `internal/scratch/scratch_test.go`

- [ ] **Step 1: Write the failing test** `TestCompileCheckedWith_Stripped`: compile a good program the way `TestCompileCheckedWith_NoVet` does (reuse its fixture and `Options{}`), then `debug/buildinfo.ReadFile(b.Binary)` and assert a `Settings` entry with `Key == "-ldflags"` and `Value == "-s -w"`. If this toolchain does not record `-ldflags` (check with `go version -m` on the built binary first), assert instead that the binary is smaller than the same program built without the flag (build it twice in the test with and without, via `buildkit.GoCmdContext`).
- [ ] **Step 2:** `go test ./internal/scratch -run TestCompileCheckedWith_Stripped` — expected FAIL.
- [ ] **Step 3: Implement.** Add `"-ldflags=-s -w"` to the build args: `GoCmdContext(ctx, b.Dir, "build", "-mod=mod", "-ldflags=-s -w", "-o", "scratch", "./cmd")`. One-line comment: stripped, the link and the first exec are faster; panics keep file:line (pclntab).
- [ ] **Step 4:** `go test ./internal/scratch` — expected PASS, including `TestRunFileWatchPanicIsolated`.
- [ ] **Step 5:** Commit: `scratch: link the scratch binary stripped (#44)`.

### Task 4: Emit module files without gofmt for scratch runs

**Files:**
- Modify: `internal/gen/gen.go` (`Options` ~line 50; `Emit`'s module loop ~line 91; `sourceFile` ~line 475), `internal/scratch/scratch.go` (`Options` ~line 160; the `gen.Emit` call ~line 264), `internal/scratch/filerun.go` (`bo := o.Build` ~line 219)
- Test: `internal/gen/gen_test.go` (or the package's existing Emit test file), `internal/scratch/filerun_test.go`

**Interfaces:**
- Produces: `gen.Options.NoFormat bool`; `scratch.Options.NoFormat bool`; `sourceFile(body string, format bool) ([]byte, error)`.

- [ ] **Step 1: Write the failing gen test** `TestEmitNoFormat`: emit the same program with and without `NoFormat`; for one module file assert the unformatted bytes parse (`go/parser.ParseFile`, `parser.SkipObjectResolution`), that `format.Source` of them equals the formatted emit's bytes, and that `app__alder.go` is byte-identical in both.
- [ ] **Step 2:** `go test ./internal/gen -run TestEmitNoFormat` — expected FAIL (no field).
- [ ] **Step 3: Implement.** `Options.NoFormat bool // skip gofmt of the module files (scratch runs: nobody reads them; the small app and routes files stay formatted)`; `sourceFile(body, format bool)`: with `format` false return the assembled `src` after `importsOf` without the two `format.Source` passes; `Emit` passes `!opts.NoFormat`; update the other `sourceFile` caller to pass `true`.
- [ ] **Step 4:** `go test ./internal/gen` — expected PASS.
- [ ] **Step 5: Plumb.** `scratch.Options.NoFormat bool`; `CompileCheckedWith` passes `NoFormat: o.NoFormat` to `gen.Emit`; `RunFile` sets `bo.NoFormat = true` beside `bo.Tests, bo.Thunks`. Test in `filerun_test.go`: `TestRunFileNoFormat` — after `RunFile` on the pure fixture, read the module file from the build dir and assert `format.Source` of it differs from it (so it was not formatted) and `r.Ran`.
- [ ] **Step 6:** `go test ./internal/scratch` — expected PASS.
- [ ] **Step 7: Measure.** `go build -o /tmp/alder-44 ./cmd/alder`; in `~/projects/nightvote-replay`, write `t44scratch.alder` with `> 1 + N`, change N and time `alder scratch t44scratch.alder` three times each with the Task 3 build and the Task 4 build (stash the task 4 commit or build `main~1`). Record the medians. Keep the task if the gain is ≥ 50 ms; otherwise revert this task's commits and note the number in the PR. Delete `t44scratch.alder`.
- [ ] **Step 8:** Commit: `gen: NoFormat for scratch runs (#44)`.

### Task 5: Verify, report, file the follow-up

- [ ] **Step 1:** `go test ./...` — expected all PASS.
- [ ] **Step 2:** Time a changed-expression eval on nightvote-replay as in Task 4 step 7 with the branch build, three runs; expected median ≤ 0.7 s (baseline 1.9 s).
- [ ] **Step 3:** Open the PR (title `scratch: faster evals — vet once per session, stripped binary, no gofmt (#44)`), body: the measurement table from the spec plus the branch's numbers; closes nothing (the nvim part is separate; the issue closes after both).
- [ ] **Step 4:** File the follow-up issue `Scratch: evaluate without building a binary per run` with the spec's table as baseline and the finding that the link (~230 ms) plus first exec (~140 ms) is the floor of the compile-per-eval design; label as the roadmap issues are.
