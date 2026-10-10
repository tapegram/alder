# M4a Scratch Runner, `rt.Show`, `alder eval`, Verified Book — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Evaluate any Alder expression against an app through the real compiler (`alder eval`), print values as Alder syntax (`rt.Show`), and verify every Alder block in the book automatically — fixing the drift the verifier finds.

**Architecture:** `internal/scratch.Compile` is the one mechanism: it loads the app, adds an in-memory `Scratch` module with implicit imports of every app module, checks, lowers, emits into a caller-chosen output directory (so `alder run`'s `build/<app>/` is untouched), writes a caller-supplied `cmd/main.go`, vets and builds. `alder eval` compiles `it = <expr>` with a ten-line main that prints `rt.Show(it)`; the checker supplies the type. `rt.Show` renders Go values as Alder syntax using union constructor names the emitter registers at init. The book verifier is an ordinary Go test that extracts fenced `alder` blocks, classifies them, and runs them through `check`/`eval`, with expectations carried in the fence info string.

**Tech Stack:** Go 1.27 stdlib (compiler side), `rt` (stdlib + `modernc.org/sqlite` already), `go/format`.

**Spec:** `docs/superpowers/specs/2026-10-09-m4-test-repl-design.md` (sections: the scratch-module runner, `rt.Show`, `alder eval`, Book verification, Milestone M4a). Prior art: `cmd/alder/build.go` (`buildTo`, `buildApp`), `cmd/alder/dump.go` (the load pipeline), `internal/gen/gen.go` (`Emit`, `app__alder.go`), `internal/gen/names.go` (tag names), `book/` (78 fenced `alder` blocks today, all unannotated).

## Global Constraints

- Go `1.27`, module `alder`; `cmd/` and `internal/` import stdlib only; `rt` stdlib + sqlite as before.
- `gofmt -l $(git ls-files '*.go')` empty; `go vet ./... && go test ./...` green before every commit (`./book` included once it exists); `python3 -m unittest` in `experiments/` green; `alder check examples/todos` clean; `alder build examples/todos` exits 0; every `gen_vet` program builds; the E2E and `TestRun_*` tests pass.
- Every evaluation is compiled code — no interpreter anywhere.
- Scratch output goes under `build/<app>/scratch/<purpose>/` and is reused between runs (Go's build cache); `alder run`'s `build/<app>/` directory and `dev.db` are never written by scratch builds.
- `rt.Show` is deterministic: record fields in declaration order, dict keys in insertion order, floats always with a point, strings Alder-escaped.
- Book expectations live in the fence info string (`alder`, `alder eval=<value>`, `alder error=<CODE>`, `alder fragment`); the verifier runs in `go test ./...`.
- Commit messages end with the two attribution lines given in the conversation.

## Review Focus

1. An expression whose type is polymorphic or an effect (`[]`, `Db.from Todo |> Db.count`) — `alder eval` must print a sensible type and `<Query>` (or evaluate with `--run`), never a Go compile error (Task 3 `TestEval_PolymorphicAndEffect`).
2. A scratch build after the app has a type error — `eval` must print the app's diagnostics (exit 1), not an internal error, and must not leave a half-written scratch dir that breaks the next run (Task 1 `TestCompile_AppErrorIsDiagnostic`, Task 3 `TestEval_BrokenAppReportsDiagnostics`).
3. A `Show` of a record containing a boxed recursive `Maybe` and a union payload with a record — no `*rt.Maybe[...]` or Go struct noise in the output (Task 2 `TestShow_BoxedAndNested`).
4. A book block with a trailing comment line or a `-- ` comment before `module` — classified and checked correctly, not mis-detected as a fragment (Task 4 `TestBook_Classify`).
5. Two `alder eval` processes at once on the same app (an agent and a human) — separate scratch dirs by purpose aren't enough; the `eval` scratch dir must tolerate concurrent builds (Task 3 `TestEval_Concurrent`).

---

## File structure

```
cmd/alder/build.go                 Task 1: buildTo/buildApp gain an out-dir parameter; load pipeline factored into loadChecked
internal/scratch/scratch.go        Task 1: Module, Build, Compile
internal/scratch/scratch_test.go
rt/show.go, rt/show_test.go        Task 2: Show + RegisterUnion
internal/gen/gen.go                Task 2: app__alder.go registers every union's constructor names
cmd/alder/eval.go, eval_test.go    Task 3: alder eval
cmd/alder/main.go                  Task 3: dispatch
book/book_test.go, book/snippets.go  Task 4: extraction, classification, expectations
book/*.md                          Task 4: annotations and drift fixes; ch.11 `alder eval`; ch.13 §8 scratch runner
```

---

### Task 1: `buildTo` output directory and the scratch runner

**Files:**
- Modify: `cmd/alder/build.go` (`buildTo(dir string, emit bool, out string, stdout, stderr io.Writer)`; `buildApp(dir, out string, stderr)`; a new `loadChecked(dir string, extra *syntax.Module) (prog *names.Program, res *check.Result, bag *diag.Bag, ioErrs []error)` factoring the load→resolve→check sequence that `build.go` and `dump.go` duplicate — `dump.go` and `run.go` switch to it), `cmd/alder/run.go` (passes `build/<app>`), `internal/names/graph.go` (`Load` gains `extra []*syntax.Module` — modules parsed from memory, added to the graph after the directory's files; an extra module's `File` is its name + ".alder" so diagnostics name it)
- Create: `internal/scratch/scratch.go`, `internal/scratch/scratch_test.go`

**Interfaces:**
- Produces:
  ```go
  package scratch
  type Module struct{ Name, Source string }      // an in-memory module; Name "Scratch" by convention
  type Build struct {
      Dir    string              // the scratch output directory
      Binary string              // the executable, "" on failure
      Diags  []diag.Diagnostic   // check diagnostics (the app's and the extra module's)
      Err    error               // a build/vet/internal error (not a diagnostic)
  }
  // Compile loads the app at appDir, adds extra (may be nil) with an implicit `import` of every
  // app module, checks (returning diagnostics without printing), lowers, emits into out, writes
  // out/cmd/main.go from mainGo, copies adapters/ and static/, vets and builds. out defaults to
  // build/<app>/scratch/<purpose>/ when purpose is given and out is "".
  func Compile(appDir string, extra *Module, mainGo string, purpose, out string) Build
  // Run executes b.Binary with env and stdin, returning stdout, stderr and the exit code.
  func Run(b Build, env []string, stdin string) (string, string, int)
  ```
- The implicit imports: `Compile` prepends `import M` for every app module `M` (except `App`, which is global) to the extra module's source *after* its `module` line (or synthesizes `module Scratch` + imports if the source has no `module` line), and keeps a line offset so diagnostics map back to the caller's source (`Build.LineOffset`).

- [ ] **Step 1: Write the failing tests**

```go
// internal/scratch/scratch_test.go
const printMain = `package main
import ("fmt"; "app")
func main() { fmt.Println(app.Scratch__it) }`   // the emitter names Scratch's `it` Scratch__it

func TestCompile_PureValueAgainstTodos(t *testing.T) {
	b := Compile("../../examples/todos", &Module{Name: "Scratch", Source: "it : Int\nit = Todo.countOpen []\n"}, printMain, "", t.TempDir())
	if b.Err != nil || len(b.Diags) != 0 { t.Fatalf("%v %v", b.Err, b.Diags) }
	out, _, code := Run(b, nil, "")
	if code != 0 || strings.TrimSpace(out) != "0" { t.Fatalf("%q %d", out, code) }
}
func TestCompile_DiagnosticsMapToExtraModule(t *testing.T) {
	b := Compile("../../examples/todos", &Module{Name: "Scratch", Source: "it : Int\nit = \"x\"\n"}, printMain, "", t.TempDir())
	if len(b.Diags) != 1 || b.Diags[0].Code != "E0003" || b.Diags[0].Span.Start.Line-b.LineOffset != 2 { t.Fatalf("%+v", b.Diags) }
	if !strings.HasPrefix(b.Diags[0].File, "Scratch") { t.Fatal(b.Diags[0].File) }
}
func TestCompile_AppErrorIsDiagnostic(t *testing.T)   // a temp copy of todos with a type error → Diags non-empty, Err nil, Binary ""; a second Compile with the error fixed succeeds in the same out dir
func TestCompile_ReusesCache(t *testing.T)            // two Compiles into the same out dir; the second is faster than the first (assert < 50% of the first's wall time, with a floor so it isn't flaky)
func TestCompile_NoExtraModule(t *testing.T)          // extra nil, a main that calls app.Tables → builds
func TestBuildTo_OutDir(t *testing.T)                 // buildTo with an explicit out dir writes there, not build/<app>
```

- [ ] **Step 2: Run to verify failure** — package missing; `buildTo` has no out parameter.
- [ ] **Step 3: Implement** — `names.Load(dir, std, extra...)`; `loadChecked`; `buildTo`/`buildApp` out param (callers updated: `runBuildTo` passes `""` → `build/<app>`; `run.go` passes `build/<app>`); `scratch.Compile` composes `loadChecked` + `ir.Lower` + `gen.Emit` + the adapters/static copy + `mainGo` + vet + `go build` (reuse the helpers in `build.go` by exporting what `scratch` needs or by moving the shared pieces into `internal/buildkit` — the implementer chooses; `cmd/alder` must not be imported by `internal/scratch`, so shared code moves to an internal package).
- [ ] **Step 4: Run** — full gate; `TestRun_*` still pass (`alder run` unaffected).
- [ ] **Step 5: Commit** — `git add -A cmd internal && git commit -m "m4a: build output directory, loadChecked, scratch-module runner"`

---

### Task 2: `rt.Show` and union registration

**Files:**
- Create: `rt/show.go`, `rt/show_test.go`
- Modify: `internal/gen/gen.go` (`app__alder.go` gets `func init() { rt.RegisterUnion[tag__M__U](map[tag__M__U]string{…}) }` — or an equivalent registration by reflect type — for every union incl. page `Msg`, `Nav` is in `rt` already), `internal/gen/gen_test.go` (`TestGen_RegistersUnions`), goldens regenerated

**Interfaces:**
- Produces:
  ```go
  package rt
  // Show prints v as Alder syntax. Deterministic. Unknown Go values print as %v.
  func Show(v any) string
  // RegisterUnion records a union's constructor names by tag so Show can print them; generated
  // code calls it from init(). tagType is the tag's reflect.Type.
  func RegisterUnion(tagType reflect.Type, names []string)
  ```
  Rendering rules: `int64` → `3`; `float64` → `1.5`/`1.0`/`NaN`; `string` → `"a\"b\n"` (escapes `\\ \" \n \t`, and `{` as `\{`); `bool` → `True`/`False`; `struct{}` → `()`; `[]T` → `[1, 2]` / `[]`; `Maybe` → `Nothing` / `Just 3` (payload parenthesized when it is a union with a payload or a negative number: `Just (Rect { w: 1.0 })`); `Result` → `Ok 1` / `Err "x"`; `Dict` → `Dict.fromList [{ key: "a", value: 1 }]` (insertion order); `Date` → `2026-10-09`; `time.Time` → RFC 3339 UTC with the shortest fraction; a generated record struct → `{ title: "Milk", done: False }` with field names lower-camel from the Go field (`Title` → `title`; a trailing `_` keyword escape dropped); a generated union struct → `Name` for a payload-less constructor, `Name 3` for a wrapped scalar payload (`{Value}` struct), `Name { w: 1.0, h: 2.0 }` for a record payload; pointers deref (boxed `Maybe`); `html.Node` → its rendered markup; `eff.Query`/`eff.Task` → `<Query>`/`<Task>`; `Page` → its URL; `Nav` → `Rerender`/`Redirect <url>`/`Reject "text"`.

- [ ] **Step 1: Write the failing tests** — a table in `rt/show_test.go` covering every rule above (build the union/record values with locally defined tagged structs registered via `RegisterUnion`, mirroring the emitter's shapes: `struct{ Tag tagT; Circle *struct{ Value float64 }; Rect *struct{ W, H float64 } }`), `TestShow_BoxedAndNested` (a record with `Next *Maybe[Node]` and a union payload holding a record), `TestShow_Escapes`; in `internal/gen`: `TestGen_RegistersUnions` (emit `ir_basic`, grep `rt.RegisterUnion` for `Shape`; the run test asserts `rt.Show(M__area_input)` prints `Rect { w: 1.0, h: 2.0 }`).
- [ ] **Step 2–5:** failure → implement → pass → `git add -A rt internal testdata && git commit -m "m4a: rt.Show prints values as Alder; unions register their constructor names"`

---

### Task 3: `alder eval`

**Files:**
- Create: `cmd/alder/eval.go`, `cmd/alder/eval_test.go`; Modify: `cmd/alder/main.go` (dispatch), `book/11-tooling.md` (command table row + a short section — the fuller book text is Task 4's)

**Interfaces:**
- `alder eval [dir] <expr>` | `alder eval [dir] -` (stdin: zero or more `let name = expr` lines then one expression) with flags `--run` (allow a `Query`: run it against `build/<app>/dev.db`, creating it with `Ensure` if absent), `--json`. Output: `value : Type` on stdout; diagnostics on stderr (exit 1); internal errors exit 2. `--json`: `{"value": "...", "type": "...", "diagnostics": [...]}` (value/type empty on error).
- Implementation: `runEvalTo(args, stdin io.Reader, stdout, stderr io.Writer) int` builds the scratch source `it = <expr>` (plus `let` lines as `name = expr` top-level values) with no signatures (the checker's inferred-signature path: a value without a signature is E0007 — RULING: the scratch module is checked with a `check.Options{InferTopLevel: true}` (new) that infers signatures for sig-less values instead of reporting E0007; this is the same machinery `inferSig` uses for the fix text; the REPL (M4c) relies on it too), compiles with `scratch.Compile(dir, mod, evalMain, "eval", "")` where `evalMain` prints `rt.Show(app.Scratch__it)`; the type is `types.String` of `it`'s inferred signature (from `check.Result.Values["Scratch.it"]`); an `it` of type `Query T`: without `--run` print `<Query> : Query T` (no execution); with `--run` the main opens the dev DB, `Ensure`s, builds an `eff.Env` with the app's session default and runs `eff.RunQuery`; a `Task` is refused ("a Task is not run by eval; M4c's REPL has :run!").
- Concurrency: the eval scratch dir is `build/<app>/scratch/eval/`; `runEvalTo` takes an `flock` on `<dir>/.lock` (use `syscall.Flock` on Unix; it is stdlib) so two evals serialize instead of corrupting the dir.

- [ ] **Step 1: Write the failing tests**
```go
func TestEval_PureValue(t *testing.T)                 // `1 + 2` → "3 : Int\n"
func TestEval_UsesAppModules(t *testing.T)            // `Todo.countOpen []` → "0 : Int"; `Theme.flip Light` → "Dark : Theme"
func TestEval_Record(t *testing.T)                    // `{ a: 1, b: "x" }` → `{ a: 1, b: "x" } : { a: Int, b: Text }`
func TestEval_LetLines(t *testing.T)                  // stdin "let xs = [1, 2]\nList.length xs" → "2 : Int"
func TestEval_PolymorphicAndEffect(t *testing.T)      // `[]` → "[] : List a"; `Db.from Todo |> Db.count` → "<Query> : Query Int"; with --run on an empty dev DB → "0 : Int"
func TestEval_TypeError(t *testing.T)                 // `1 + "x"` → exit 1, stderr has E0003 with a position on "line 1 of your input"
func TestEval_BrokenAppReportsDiagnostics(t *testing.T)
func TestEval_JSON(t *testing.T)
func TestEval_Concurrent(t *testing.T)                // 4 goroutines evaluating different expressions → all correct
```
- [ ] **Step 2–5:** failure → implement → pass → `git add -A cmd internal book && git commit -m "m4a: alder eval"`

---

### Task 4: The book verifier and the drift it finds

**Files:**
- Create: `book/book_test.go` (`package book`), `book/snippets.go` (extraction + classification; kept out of the test file so M4b/M4c can reuse it), `book/testdata/` (a fixture chapter with one block of each kind incl. a deliberately wrong `eval=` to prove the test fails)
- Modify: every `book/*.md` with fenced `alder` blocks (annotations + fixes), `book/13-compiler.md` (§8 "The scratch runner": how `eval` and the verifier work, with the real `Compile` signature and a captured `alder eval` transcript), `book/11-tooling.md` (`alder eval` section; a note that the book is verified), `book/README.md`

**Interfaces:**
- `snippets.Extract(path string) []Snippet{File string; Line int; Info string; Source string}`; `snippets.Classify(s Snippet) Kind` ∈ `Module` (first non-comment line starts with `module`), `Decls` (first non-comment line is a declaration: `type|table|form|page|msg|capability|session|login|test|import` or `name :` / `name args =` at column 1), `Expr` (otherwise); `Expectation(info) {Kind: Check|Eval|Error|Fragment; Value, Code string}` parsed from the info string.
- Verification per kind: `Module` → written as `app/<Module>.alder` into a temp copy of `examples/todos` (replacing a same-named module) and `alder check`ed (the whole app must stay clean — so a book module that deviates from the example is a real finding); `Decls` → wrapped in `module Snippet` + imports of every example module, checked the same way; `Expr` → **parse-only by default** (the book's expression snippets routinely reference names like `todos` that exist only in prose), run through `alder eval` only when `eval=` is given; `error=` → the diagnostic code must appear; `fragment` → skipped, counted, listed at the end of the test log.
- A snippet that fails prints `book/03-syntax.md:120: E0003 …` so the fix is one click away.

- [ ] **Step 1: Write the failing tests** — `TestBook_Classify` (table: module / decls / expr / leading comment / `--` before `module` / a `test "…" =` block), `TestBook_Expectations` (info-string parsing), `TestBook_FixtureFails` (the fixture's wrong `eval=` makes the verifier report exactly one failure), and `TestBook` (every chapter; initially FAILS on real drift — that is the point).
- [ ] **Step 2: Run** `go test ./book -run TestBook -v` and **collect every failure into the report** before fixing anything: that list is the measurement ("how far had the book drifted?") and goes into `book/12-rationale.md` as a short paragraph.
- [ ] **Step 3: Fix the drift** — in the book, not the compiler: wrong snippets corrected; deliberate excerpts annotated `fragment` (keep these rare — if more than a quarter of the blocks need it, say so in the report and prefer completing the snippet); expression snippets that should be executable get `eval=<value>` (aim for every expression in chapters 3, 4 and 9 to carry one); error examples get `error=CODE`. If a snippet reveals a compiler bug (the book is right, the compiler is wrong), record it in the report and mark the block `fragment` with a `-- TODO(compiler): …` comment inside it rather than silently rewriting the book.
- [ ] **Step 4: Run** — `go test ./... ` green; `gofmt`; the verifier's run time < 60 s (it compiles ~80 snippets; batch the `Decls`/`Module` kinds into as few `alder check` invocations as correctness allows — e.g. one temp app per chapter — and parallelize with `t.Parallel()` where dirs are independent).
- [ ] **Step 5: Book** — chapter 13 §8, chapter 11, README status ("every Alder block in this book is checked by `go test ./book`").
- [ ] **Step 6: Commit** — `git add -A book && git commit -m "m4a: the book is verified; drift fixed and annotated"`

---

## Self-review notes

**Spec coverage.** Scratch runner (`Compile`, implicit imports, diagnostics mapped, out dir, cache reuse) → T1; `rt.Show` (+ registration) → T2; `alder eval` (expr/stdin, `--run`, `--json`, exit codes) → T3; book verification (extraction, kinds, fence expectations, drift fixes, run in `go test`) → T4; chapter 13/11 → T3/T4. The REPL, `Test`, `alder test` and the other deferrals are M4b/M4c.

**Placeholder scan.** T4's drift fixes are by nature discovered at run time; the procedure says exactly what to do with each class. No TBDs.

**Type consistency.** `scratch.Compile(appDir, extra, mainGo, purpose, out) Build{Dir, Binary, Diags, Err, LineOffset}` + `Run` (T1) are what `eval` (T3) and the verifier (T4) call; `rt.Show`/`RegisterUnion` (T2) are what `evalMain` prints; `check.Options{InferTopLevel}` (T3) is used by T3 and later by the REPL; `snippets.Extract/Classify/Expectation` (T4) are reused by M4b.

**Review Focus coverage.** (1) T3 `TestEval_PolymorphicAndEffect`; (2) T1 `TestCompile_AppErrorIsDiagnostic` + T3 `TestEval_BrokenAppReportsDiagnostics`; (3) T2 `TestShow_BoxedAndNested`; (4) T4 `TestBook_Classify`; (5) T3 `TestEval_Concurrent`.
