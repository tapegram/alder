# M5d: Adapter sub-packages and generated wiring — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A capability's Go adapter lives in `adapters/<first module segment>/` and exports a constructor named by the compiler; `alder build` generates the wiring (`build/<app>/caps/`), reports every missing adapter by the exact package and signature to write, and production builds carry no test code.

**Architecture:** (1) `buildkit` scans `adapters/` recursively and copies the whole tree; gen emits a `caps` package from `Program.Caps` (import per needed adapter package, aliased; one constructor call per capability) and the mains import `app/caps`; `alder build`/`test`/`eval`/`repl` check that every needed package and constructor exists before `go build` and print the ruled texts. (2) gen with `Options{Tests}` unset emits no test code at all (no `Tests`, `TestSession`, test bodies, `Test.fake` stubs, `rtesting` import). (3) Both examples move to sub-packages; chapter 7 teaches the rule; 11/13/14/README/harness follow.

**Tech Stack:** Go; `go/parser` (to find exported constructors in an adapter package without building it).

**Spec:** `docs/superpowers/specs/2026-10-10-m5-blog-migrations-fakes-design.md`, "M5d — adapter sub-packages" (binding). Related: the same spec's "Decisions recorded"; M3 spec "Milestones".

## Global Constraints

- The naming rule is exact: capability `X` of module `A.B.C` → package `adapters/a/` (first segment lower-cased), constructor `BCX` (remaining segments concatenated, then the capability name), returning `app.Cap__A__B__C__X`. Module `Markdown`'s `Markdown` → `adapters/markdown/`, `Markdown()`. A module named `App` → `adapters/app/`, imported under an alias in generated code.
- The flat form (`adapters/*.go`, `func Caps() app.Caps`) is removed everywhere: examples, book, harness, tests. `adapters/*.go` top-level files that remain are compiled as package `adapters` but never wired; a top-level `Caps` function is reported: "adapters/ no longer wires capabilities: move each adapter to adapters/<module>/ (see the list below)".
- Nothing under `adapters/` is generated or modified; the generated wiring is `build/<app>/caps/caps__alder.go` only.
- Every missing adapter is reported in one build run (all of them, not the first), each on its own line in the ruled text, and the build stops before `go build`. `alder check` never looks at `adapters/`.
- Production emission (`gen.Options{Tests: false}`): no `Tests`, `TestSession`, test bodies, fake stubs, or `rt/testing` import. Test emission is unchanged in behaviour. Goldens: every prod golden shrinks accordingly (explain in the commit); `gen_fakes/want_prod` and the new `gen_caps` golden.
- Gate: `gofmt -l $(git ls-files '*.go')` empty; `go vet ./...`; `go test ./... -count=1`; both examples check/build/test (todos 3, blog 24), and `go test ./adapters/...` inside each example's build directory passes (the blog has two adapter test files); `(cd experiments && python3 -m unittest)`; `go test ./book -count=1`.
- Commit messages end with `Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>` and `Claude-Session: https://claude.ai/code/session_01VSwqPTccEdvyAkyJ2WtrhG`. Stage explicitly.

## Review Focus

1. **Two modules whose first segments differ only by case or by a Go-illegal character** (`Mail` and `mail` cannot both exist — Alder modules are capitalised; but `Auth.Login` and `Auth.Signup` share `adapters/auth/` with constructors `LoginPassword` and `SignupPassword`): expected — one package, two constructors, no collision; a collision that CAN happen (module `A.BC` cap `D` vs module `A.B` cap `CD` → both `BCD`) is detected by gen and reported as an internal naming error at build time with both capabilities named. Test in Task 1 (names test + a build test).
2. **An adapter package with the right function but the wrong return type** (returns `app.Cap__Markdown__Markdown` where `Cap__Payments__Stripe` is needed; or an unexported `stripe()`): expected — the scan reports the missing exported constructor by name; a type mismatch surfaces as Go's error quoted under "the adapters do not compile with the app". Test in Task 1.
3. **A helper package `adapters/internal/dbx/` used by two adapter packages**: expected — copied, compiles, never wired, no warning. Test in Task 1 (a fixture app).
4. **`alder test` on an app with an adapter package AND a fake for the same capability** (the blog): expected — the test main wires the adapters; `Test.fake` overrides per test; a test without `Test.fake` reaches the real adapter. Test in Task 2 (the blog's 24).
5. **A production build after the trim**: the binary serves, `alder eval --run` and `repl` (which use scratch mains with `Tests: false`? — check: the REPL evaluates expressions, not tests; they must still build) work, and `TestExample_*` assert the prod module has no `rtesting` import. Test in Task 2.

---

### Task 1: Adapter scan, generated `caps` package, build-time wiring errors

**Files:**
- Modify: `internal/buildkit/buildkit.go` (`CopyAdapters` copies the whole tree recursively, `.go` files and `testdata/` dirs, skipping nothing else; removes stale files in `dst` that no longer exist in `src`; `HasGoFiles` → `HasAdapters(dir)`: any `.go` file anywhere under `adapters/`), new `internal/buildkit/adapters.go` (`AdapterPackage(module string) (dir, importPath string)` and `Constructor(module, cap string) string` implementing the rule; `ScanAdapters(dir string, caps []Need) (missing []Need, flat bool, err error)` where `Need{Module, Cap, Package, Func, Iface}`: parses each needed package with `go/parser` (ParseDir, no type-check) and reports a need missing when the package dir has no `.go` files or no exported func of that name with no parameters; `flat` = a top-level `adapters/*.go` declares `func Caps`), `adapters_test.go` (table tests for the rule incl. `App`, dotted modules, the collision check; a fixture tree under `testdata/adapters/`)
- Modify: `internal/gen/caps.go` (new `CapsPackage(p *ir.Program) []byte`: `package caps`; imports `"app"` and each needed `app/adapters/<dir>` under alias `a_<dir>`; `func Caps() app.Caps { return app.Caps{ Auth__Login__Password: a_auth.LoginPassword(), … } }`; empty struct when no caps; `Main`/the mains import `"app/caps"` and use `caps.Caps()` when the app has capabilities, else `app.Caps{}`), `internal/gen/names.go` (`adapterPkg`, `adapterFunc` + the injectivity/collision test extended: for every pair of capabilities, `(pkg, func)` distinct), `internal/gen/gen.go` (`Emit` returns the `caps/caps__alder.go` file when `len(p.Caps) > 0`), goldens (`gen_caps`: an app with `Auth.Login.Password`, `Markdown.Markdown`, `Payments.Stripe` and `App`-module capability `Clock`; existing goldens' `cmd/main.go` change from `adapters.Caps()` to `caps.Caps()`)
- Modify: `cmd/alder/build.go` (replace the "needs an adapters package" check with `ScanAdapters`: print every missing need as "capability `Auth.Login.Password` needs `adapters/auth/` with `func LoginPassword() app.Cap__Auth__Login__Password`", one per line, after the heading "<dir> is missing adapters for N capabilit(y|ies):"; the flat-form message; exit 1 before emitting), `cmd/alder/test.go` + `eval.go` + `repl.go` and `internal/scratch/scratch.go` (`HasAdapters`; the mains import `app/caps`; the same scan with the same texts — factor `buildkit.CheckAdapters(dir, p.Caps, stderr) bool`), `internal/scratch` copies `caps/`
- Modify: `cmd/alder/build_test.go` (+ test fixtures: an app whose adapters are in sub-packages with a helper package; the missing-constructor case; the wrong-return-type case (Go error quoted); the flat-form case), `internal/gen/gen_test.go`

**Interfaces (binding for Tasks 2–3):**
```go
// buildkit
func AdapterPackage(module string) (dir string)         // "Auth.Login" → "auth"
func Constructor(module, capability string) string      // ("Auth.Login","Password") → "LoginPassword"
type Need struct{ Module, Cap, Package, Func, Iface string }
func ScanAdapters(appDir string, caps []Need) (missing []Need, flat bool, err error)
func CheckAdapters(appDir string, caps []*ir.Capability, stderr io.Writer) (ok bool) // prints the ruled texts
func HasAdapters(appDir string) bool
func CopyAdapters(src, dst string, present bool) error  // recursive
// gen
func CapsPackage(p *ir.Program) []byte   // caps/caps__alder.go
// generated: package caps; func Caps() app.Caps
```

- [ ] **Step 1 (RED):** buildkit table tests (rule, scan, flat, recursive copy with a stale file removed); gen golden `gen_caps` + the collision test; build tests for the four fixture cases with the exact texts.
- [ ] **Step 2:** implement buildkit → gen → cmd/scratch. **Step 3 (GREEN):** `go test ./internal/... ./cmd/... -count=1`; both examples will FAIL to build until Task 2 moves their adapters — so in this task also move the examples' adapters (`examples/todos/adapters/auth/password.go` with `func LoginPassword()`, `examples/blog/adapters/auth/password.go` + `adapters/markdown/markdown.go` with their `_test.go` files, package names `auth`/`markdown`, imports of `app` unchanged) so the gate is green at the end of the task; `TestExample_*` and the examples' README comments updated. **Step 4:** commit (two commits: compiler; examples).

---

### Task 2: Production builds carry no test code

**Files:**
- Modify: `internal/gen/caps.go` (`testsTable`, `fakesTable`, `TestSession` emitted only when `g.tests`), `internal/gen/gen.go` (test bodies — `m.Tests` — lowered/emitted only when `g.tests`; the `fakeStub`/`usesFakeTable` walk and the failing `Test.fake` Call are deleted: with no test bodies in production there is nothing to stub; `rt/testing` is imported only by test builds), `internal/gen/effects.go` (`TestRun`/`TestTrace`/`TestEq`/`TestFake` emission paths assert `g.tests` — a production emission reaching one is an internal error), goldens: every prod golden loses `Tests`/`TestSession` (explain), `gen_fakes/want_prod` loses the stubs, `want` unchanged
- Modify: `cmd/alder/test.go` (unchanged behaviour: `Tests: true`), `cmd/alder/eval.go`/`repl.go`/`internal/scratch` (the scratch mains use `app.TestSession()` today — ruling: keep `TestSession` in production emission? NO: eval/repl mains are built with a scratch purpose; give `scratch.Options{Tests}` the value `true` for eval/repl (they are developer tools, never deployed) OR emit `TestSession` as `Session__alder()` default in both. RULING: eval and repl build with `Tests: true` — simplest, and `Test.fake` becomes usable from the REPL for free; note it in chapter 11), `cmd/alder/build_test.go`/`TestExample_*` (assert no `rtesting` import and no `Tests` symbol in `build/<app>/app__alder.go`; the binary serves)
- Modify: `book/13-compiler.md` (the "test builds" paragraphs: production has no test code; delete the stub explanation), `book/10-testing.md` §8 (the "in production a test using a fake's table is a stub" sentences → "production builds contain no tests at all"), `book/11-tooling.md` (`alder build` vs `alder test`; eval/repl build as test builds)

- [ ] **Step 1 (RED):** golden updates + `TestGen_ProdHasNoTests` (no `Tests`, `TestSession`, `rtesting`, `fake__` in prod output of `fake-ok` and of the todos); eval/repl tests still pass. **Step 2:** implement. **Step 3 (GREEN)** + full gate. **Step 4:** commit.

---

### Task 3: The book, README, harness

**Files:**
- Modify: `book/07-effects.md` "Implementing a capability" (the rule with the three worked names; the generated `caps` package quoted from a real build of the blog; the missing-adapter transcript captured from a real `alder build` of a scratch app missing one; "an adapter's own tests: `go test ./adapters/...` in the build directory" with a real transcript; helpers under `adapters/internal/`), `book/11-tooling.md` (`alder build` section: the scan, the texts; the `adapters/` layout table), `book/13-compiler.md` (the build pipeline section: copy, scan, `caps`), `book/14-second-app.md` (adapter paths and any `part=blog/adapters/…` blocks — check how chapter 14 quotes the adapters; re-copy), `book/README.md`, `examples/*/README` or top comments, `experiments/run_loop.py` (`ADAPTER_RE` accepts `adapters/<dir>/<name>.go` and `adapters/<dir>/<sub>/…`? — RULING: one level: `adapters/<dir>/<name>.go`; the flat form is no longer accepted), `experiments/README.md`, harness tests, `experiments/build_context.py` (reference: the rule paragraph is kept)
- Verify every quoted text against the code (grep the texts).

- [ ] **Step 1:** capture transcripts in scratch; write; `go test ./book`. **Step 2:** harness + tests. **Step 3:** full gate; commit (book; harness).

---

## Self-review notes

- Spec coverage: the rule (T1), generated wiring (T1), errors (T1), whole-tree copy + adapter tests (T1 + T3 prose), no test code in production (T2), examples + book + harness (T1 examples, T3).
- Interfaces: T2 and T3 consume T1's names and texts; T3 quotes T2's chapter-13 change.
- Review Focus 1–5 each have a named test (RF5 in T2).
- Ledger carry-over from M5c not in scope: unterminated-fake recovery; dump naming; test-DB template caching.
