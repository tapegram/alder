# Short Module Qualifiers (#23) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** An imported module is qualified by its last path segment (`import Table.Todo` → `Todo.find`). `import P as Q` is allowed only where the canonical rule requires it. A full-path reference to an imported module becomes an error, and `alder fmt` migrates existing code.

**Architecture:** One pure function, `names.CanonicalQualifiers`, decides every file's qualifiers. The resolver keys a module's imports by qualifier and still writes the **full path** into `Ref.Module`, so the checker, the IR and codegen stay unchanged. The resolver reports wrong aliases (E0204) and full-path references (E0205) with fixes. `alder fmt` in a project applies exactly those fixes before formatting, which performs the migration. Generators, scratch, the LSP and tree-sitter all follow the same function.

**Tech Stack:** Go (internal/syntax, names, format, generate, scratch, lsp, cmd/alder), tree-sitter (tree-sitter-alder), the book (book/*.md, run by book_test.go).

**Spec:** There is no separate spec. The decisions were made with the user on 2026-10-24 and are recorded in a comment on tapegram/alder#23. The **Design rules** section below is the spec. Executors treat it as normative.

## Design rules (normative)

1. **Qualifier.** A non-page import's qualifier is its canonical qualifier (rule 3). References to the module's members, types, tables, capabilities and elements use that qualifier: `Todo.find`, `Todo.Status`, `<Layout.Page>`, `Todo.done` (column), `Clock.now` (capability).
   - `Q.Cap.entry` and `Q.Table.column` keep working: an imported module's capability or fake table is written after its qualifier.
2. **Eponymous declarations.** The existing rule ("a declaration named like its module's last segment is referred to by the module name alone") now reads "by the qualifier alone". With `import Table.Todo`, write `Todo` (the table or type) and `Todo { … }` where it was `Table.Todo` before. With `import Table.Todo as TableTodo`, write `TableTodo`.
3. **Canonical qualifier.** For each import path P in a file, k starts at 1. The candidate is the last k segments of P, joined with no separator (`Admin.Posts.Index`, k=2 → `PostsIndex`). Increase k while either holds:
   - another import of the file has the same last-k suffix, or
   - the candidate is *taken*.

   Taken means one of:
   - a standard-library module name, or `App`;
   - the importing module's own last segment;
   - a type, table, capability or fake table declared in the importing module;
   - the candidate of another import of the file.

   If k reaches len(P) and the candidate is still blocked by an identical suffix, the import keeps its whole path joined (`Layout` beside `Admin.Layout` stays `Layout`). If the joined whole path is itself taken, or two imports end up with the same whole join, those imports are **unqualifiable**: it gets E0206, and its fix suggests renaming the module.
4. **`as`.** An import whose canonical qualifier equals its last segment must have no `as`. Any other import must be written `import P as <canonical>`. A missing, redundant or different alias is E0204, and its fix rewrites the import line to the canonical form.
5. **Full path is an error.**
   - A reference that spells an imported non-page module's full path (k>1 segments) instead of its qualifier is E0205. This covers `Table.Todo.find`, `Table.Todo` as an eponymous type, `<Admin.Layout.Page>` and `A.B.Clock.now`.
   - The fix replaces the span's module part with the qualifier.
   - The resolver still resolves such a reference as if it were short, so no cascade of errors follows.
6. **Pages and App stay full path.** Page values (`Todos.Show { id: id }`, `Admin.User.Index {}`, `href={Auth.Login {}}`) and `App.Msg` / `App.Logout` are global and are always written with the full path, imported or not. Members of an *imported* page module go through its qualifier (`import Todos.Index` → `Index.helper`). A page-value spelling is never E0205.
7. **Codes.** E0204 is "import alias must be canonical", E0205 is "full module path instead of qualifier" and E0206 is "import has no usable qualifier". All three are errors, and each gets an `explain` entry.
8. **Migration.**
   - `alder fmt` on a file inside an app project loads the program, applies that file's E0204 and E0205 fixes, then formats.
   - `alder fmt --stdin` and files outside a project format only.
   - `--check` reports a file that would be migrated as one that needs reformatting.
9. **Generated code** always uses the canonical qualifier. Adding an import that changes an existing import's canonical qualifier also rewrites that import's line and its references in the same file.

## Global Constraints

- `Ref.Module` after resolve is always the declaring module's **full path**. Generated Go names (`internal/gen/names.go`) must not change. The gen golden files must stay byte-identical.
- `as` is contextual, recognized only after an import's module name. A value named `as` stays legal everywhere else.
- Error codes are exactly `E0204`, `E0205` and `E0206`. Add them as constants in `internal/diag/diag.go`, next to `E0201`–`E0203`.
- Existing behavior to keep: imports sorted and de-duplicated by `fmt`, the W0001 unused-import warning, the E0201 not-imported fix (it now inserts the canonical `import P [as Q]`), and cycle detection, which stays on full paths.
- Commits end with the session's Co-Authored-By and Claude-Session lines.
- **Suite state:** Tasks 3–5 change the language. `go test ./...` is expected to be red in examples and testdata until Task 6 migrates them. Tasks 1–5 run the package-scoped commands they name, and Task 6 onward must leave `go test ./...` green.

## Review Focus

1. **An imported page module.** `import Todos.Index` with `Todos.Index {}` (a page value: full path, no E0205) and `Index.helper` (a member) both resolve. Owner: Task 3.
2. **Self-collision.** Module `Sites.MnSos` importing `Data.MnSos` must write `import Data.MnSos as DataMnSos`. nightvote-replay has this case. Owner: Task 3.
3. **A local table or type named like a qualifier.** Module `Todos` declaring `table Todo` and importing `Table.Todo` must alias it to `TableTodo`. A stdlib name does the same: `Utils.List` becomes `UtilsList`. Owner: Task 2 (rule) and Task 3 (end to end).
4. **Capabilities and fake tables through a qualifier.**
   - `B.Clock.now` with `import A.B` resolves to capability `Clock` of `A.B`.
   - `Charge.cents` (a fake table column) resolves as it does today.
   - The gen goldens are unchanged. Owner: Task 3.
5. **The fmt migration is idempotent and safe.**
   - Running it twice changes nothing.
   - A project file with unrelated type errors is still migrated.
   - A file that doesn't parse is reported and left untouched.
   - `--stdin` never loads a program. Owner: Task 4.

---

### Task 1: Parse `import P as Q`

**Model:** Sonnet.

**Files:**
- Modify: `internal/syntax/ast.go:54` (Import)
- Modify: `internal/syntax/parser.go:240-244`
- Modify: `internal/syntax/dump.go:30`
- Test: `internal/syntax/parser_test.go` (or the file holding the import tests)

**Interfaces:**
- Produces: `type Import struct { Node; Module string; Alias string; AliasSpan diag.Span }`. `Alias` is "" when there is no `as`. `AliasSpan` covers the alias identifier only. `Span` still covers the whole line.

- [ ] **Step 1: Write the failing tests** `TestParse_ImportAs`:
  - `module M\n\nimport Admin.Layout as AdminLayout\n` gives `Imports[0].Module == "Admin.Layout"` and `Alias == "AdminLayout"`. `AliasSpan` is on line 3, cols 24–35.
  - `import A.B as` gives a parse error containing `expected alias after \`as\``.
  - `import A as b` gives a parse error containing `alias must be capitalized`.
  - `as : Int\nas = 1` still parses as a value declaration.
  - `Dump` of the first module contains `(import Admin.Layout as AdminLayout)`.
- [ ] **Step 2: Run** `go test ./internal/syntax -run TestParse_ImportAs`. Expected: FAIL.
- [ ] **Step 3: Implement.** After `parseModName()` in the import branch, the `as` is an `LIdent` token with text `as`, followed by a `UIdent`. Leave the lexer unchanged.
- [ ] **Step 4: Run** `go test ./internal/syntax`. Expected: PASS.
- [ ] **Step 5: Commit** with `feat(syntax): parse import ... as Alias (#23)`.

### Task 2: Canonical qualifier rule

**Model:** Sonnet.

**Files:**
- Create: `internal/names/qualify.go`
- Test: `internal/names/qualify_test.go`

**Interfaces:**
- Produces:
  ```go
  // CanonicalQualifiers maps each import path to its canonical qualifier (design rule 3).
  // self is the importing module's name; taken reports names declared in it or reserved
  // (stdlib modules, App). Unqualifiable paths map to "" and are listed in bad, sorted.
  func CanonicalQualifiers(self string, paths []string, taken func(string) bool) (quals map[string]string, bad []string)
  // NeedsAlias reports whether path's import must be written with `as q`.
  func NeedsAlias(path, q string) bool // q != last segment of path
  ```
  Tasks 3, 4, 5, 7 and 8 all call `CanonicalQualifiers`. None of them may re-implement it.

- [ ] **Step 1: Write the table test** `TestCanonicalQualifiers`, with self = `M` and taken = stdlib names + `App`:

  | paths | extra taken | want |
  |---|---|---|
  | `Table.Todo` | – | `Todo` |
  | `Layout`, `Admin.Layout` | – | `Layout`, `AdminLayout` |
  | `Posts.Index`, `Admin.Posts.Index` | – | `PostsIndex`, `AdminPostsIndex` |
  | `Utils.List` | – | `UtilsList` |
  | `Data.MnSos`, with self = `Sites.MnSos` | – | `DataMnSos` |
  | `Table.Todo` | `Todo` (local table) | `TableTodo` |
  | `A.X`, `B.X`, `AX` | – | `A.X`→"", `BX`, `AX`→"", bad=[`A.X`,`AX`] (two paths with the same whole join are both bad) |
  | `Todos.Index`, `Projects.Index`, `Blog.Show` | – | `TodosIndex`, `ProjectsIndex`, `Show` |

  Also assert that the result is independent of input order: shuffle the inputs and compare.
- [ ] **Step 2: Run** `go test ./internal/names -run TestCanonicalQualifiers`. Expected: FAIL (undefined).
- [ ] **Step 3: Implement.**
  - Iterate to a fixed point: compute every path's candidate, and for any group of equal candidates bump k on each member that can still grow.
  - Then bump candidates that are taken.
  - Repeat until nothing changes.
  - Paths that can't grow and still share a whole join are all bad. A path whose whole join is taken by a reserved name is also bad.
- [ ] **Step 4: Run** the same command. Expected: PASS.
- [ ] **Step 5: Commit** with `feat(names): canonical import qualifiers (#23)`.

### Task 3: Resolver uses qualifiers; E0204–E0206

**Model:** Opus. This is the core semantic change, with many interacting lookup paths.

**Files:**
- Modify: `internal/names/resolve.go` (moduleScope.imports at :37/:43, importedScope, Var resolution :900–1005, types :680–790, CtorRef :1356, elements, notImported :527, importFix :561, the E0202 fixes :745–775 and messages :1127–1132)
- Modify: `internal/names/graph.go`: missingImports and the cycle check stay on full paths, and only the lookups change
- Modify: `internal/diag/diag.go` (constants), `internal/diag/explain.go` (three entries)
- Audit: `internal/check/expr.go:997,1294,1377`, `check/decls.go:581,617,882`, `check/partitions.go:99`, `check/fakes.go:167`, `check/envscan.go:45`, `ir/lower_eff.go:48,374`. Each reads the raw syntax `.Module`. Switch each one that needs the module to `Ref.Module`. A reader that only takes the last segment as a capability or table name can stay as it is, but leave a comment saying why.
- Test: `internal/names/qualify_resolve_test.go` (new, using the `parse`/`Build`/`std` helpers from names_test.go)

**Interfaces:**
- Consumes: `Import.Alias`/`AliasSpan` (Task 1), `CanonicalQualifiers`/`NeedsAlias` (Task 2).
- Produces:
  - `moduleScope.imports` is keyed by **qualifier**, and a new `moduleScope.quals map[string]string` maps path to qualifier.
  - `func (p *Program) Qualifiers(module string) map[string]string` returns path to qualifier for that module. It is exported for the LSP (Task 8) and fmt (Task 4).
  - Diagnostics E0204, E0205 and E0206, each with a `Fix` (E0206: none).
  - `Ref.Module` stays the full path.

- [ ] **Step 1: Write the failing tests** in `qualify_resolve_test.go`. Each test builds a small program and asserts the codes, the messages (with `strings.Contains`) and `Fix.Replace`:
  - `TestQual_ShortForm`: with `import Table.Todo`, `Todo.find`, the type `Todo`, `Todo.Status` and `<Todo.Card>` resolve with `Ref.Module == "Table.Todo"` and produce no diagnostics.
  - `TestQual_FullPathIsE0205`: `Table.Todo.find` gives E0205 with the message "write `Todo.find`, not `Table.Todo.find`" and `Fix.Replace == "Todo.find"`. The Ref still resolves, and E0205 is the only diagnostic.
  - `TestQual_AliasRules`:
    - `import Layout` + `import Admin.Layout` gives E0204, with fix `import Admin.Layout as AdminLayout`.
    - `import Table.Todo as Todo` gives E0204 (redundant), with fix `import Table.Todo`.
    - `import Admin.Layout as AL` with `import Layout` gives E0204, with fix `as AdminLayout`.
  - `TestQual_SelfCollision`: module `Sites.MnSos` with `import Data.MnSos as DataMnSos` and `DataMnSos.x` gives no diagnostics.
  - `TestQual_LocalTableTakesName`: module `Todos` declaring `table Todo` and importing `Table.Todo` gives E0204, with fix `as TableTodo`.
  - `TestQual_Unqualifiable`: `A.X`, `B.X` and `AX` imported together gives E0206 on both `A.X` and `AX`.
  - `TestQual_PagesStayFull`:
    - With `import Todos.Index`, `Todos.Index {}` resolves as a page value with no E0205.
    - `Index.helper` resolves.
    - `Admin.User.Index {}` without an import still resolves.
    - `App.Logout` resolves.
  - `TestQual_CapabilityAndFake`:
    - With `import A.B`, where B declares `capability Clock { now … }`, `B.Clock.now` resolves to `RefCapability`, `Module "A.B"`.
    - `A.B.Clock.now` gives E0205, with fix `B.Clock.now`.
    - The existing fake-table test that uses `Payments.Charge.cents` with a single-segment `Payments` still passes.
  - `TestQual_ImportFixIsCanonical`: an unimported `Admin.Layout.Page` used where `Layout` is already imported gives E0201, with a fix inserting `import Admin.Layout as AdminLayout`.
- [ ] **Step 2: Run** `go test ./internal/names -run TestQual`. Expected: FAIL.
- [ ] **Step 3: Implement.**
  - Key the imports by qualifier.
  - Look qualified references up as follows, in this order:
    1. a page value or `App` (full path);
    2. a single-segment qualifier: an import, then stdlib, then the local capability, table or fake table;
    3. `Q.Rest`: a qualifier plus a capability or table of that module;
    4. a full path equal to an imported non-page path, or a prefix of one: E0205 plus resolve as the short form;
    5. otherwise the existing notImported/unknown handling.
  - Unqualified type and ctor names that equal a qualifier refer to that module's eponymous declaration (design rule 2).
  - Every fix or message that builds qualified text uses the qualifier, never the full path. This includes E0202's `write X.T` and `entryCaps` fixes.
  - Add the three `explain` texts. Model them on E0201: say what's wrong, then the exact fix, then the reason ("one predictable spelling per module").
- [ ] **Step 4: Run** `go test ./internal/names ./internal/check ./internal/ir ./internal/diag`. Expected: PASS, except existing tests that spell full paths. Update those to the short form in this task, with the same assertions and new spellings.
- [ ] **Step 5: Run** `go test ./internal/gen`. Expected: the gen goldens are unchanged. Fixture *source* files may need migrating, but the generated Go must not change.
- [ ] **Step 6: Commit** with `feat(names): resolve imports by canonical qualifier; E0204-E0206 (#23)`.

### Task 4: `alder fmt` prints `as` and migrates

**Model:** Sonnet.

**Files:**
- Modify: `internal/format/print_decl.go:21-64` (print `import P as Q`)
- Create: `internal/format/migrate.go`
- Modify: `cmd/alder/fmt.go` (`fmtFile`: migrate for project files)
- Test: `internal/format/migrate_test.go`, `cmd/alder/fmt_test.go`, `testdata/golden/fmt_decls.{alder,golden}` (add an `as` import)

**Interfaces:**
- Consumes: `names.LoadOverlay`, the E0204/E0205 diagnostics and fixes (Task 3), and `diag.Apply`.
- Produces: `func Migrate(prog *names.Program, bag *diag.Bag, file string, src []byte) ([]byte, bool, error)`. It applies only E0204 and E0205 fixes whose span is in `file`, and returns the new source and whether anything changed. `fmtFile` calls it before `format.Source` when the file is under a project's `app/`. Reuse the project-root lookup that `alder scratch` already uses; don't write a second one.

- [ ] **Step 1: Write the failing tests:**
  - `TestMigrate_FullPaths`: a two-module project (`app/Table/Todo.alder` and `app/Todos.alder`) whose source uses `Table.Todo.find`, the type `Table.Todo` and `<Admin.Layout.Page>` together with `import Layout`. After migration it uses `Todo.find`, `Todo` and `<AdminLayout.Page>`, and the import line reads `import Admin.Layout as AdminLayout`.
  - `TestMigrate_Idempotent`: migrating the output again returns `changed == false`.
  - `TestMigrate_KeepsPagesFull`: `Todos.Index {}` is unchanged.
  - `TestMigrate_WithTypeErrors`: a file that also has an unrelated E0002 is still migrated.
  - `cmd/alder`:
    - `TestFmt_MigratesProjectFile`: `alder fmt app/Todos.alder` rewrites the file, and `--check` exits 1 beforehand.
    - `TestFmt_StdinNoMigration`: `--stdin` output keeps `Table.Todo.find`.
- [ ] **Step 2: Run** `go test ./internal/format ./cmd/alder -run 'Migrate|TestFmt_'`. Expected: FAIL.
- [ ] **Step 3: Implement** `Migrate`, the `as` printing (`import ` + Module + ` as ` + Alias), and the `fmtFile` hook.
  - The `samePrograms` guard runs on (migrated source → formatted output), never across the migration.
  - `sortedImports` stays keyed by `Module`.
- [ ] **Step 4: Run** `go test ./internal/format ./cmd/alder -run 'Migrate|Fmt|Format' -update` once for the golden, then again without `-update`. Expected: PASS.
- [ ] **Step 5: Commit** with `feat(fmt): print import aliases and migrate full-path qualifiers (#23)`.

### Task 5: Generators emit canonical qualifiers

**Model:** Sonnet.

**Files:**
- Modify: `internal/generate/srcedit.go:127` `addImports`; `format.go:39,64` `importSet`/`newImports`; `table.go:175-211` `typeImports`; `fill.go:98-175`; `admin.go:147-151,276`; `page.go:307`; `program.go:216`; plus any other `addImports` caller that builds qualified text
- Modify: `testdata/generate/want/**`, regenerated
- Test: `internal/generate/qualify_test.go` (new) plus the existing generate tests

**Interfaces:**
- Consumes: `CanonicalQualifiers` (Task 2).
- Produces:
  - `func addImports(e *srcEdit, m *syntax.Module, want []string) map[string]string`. It now returns path to qualifier for every import of the edited file, after the additions.
  - Callers build qualified text with `qualified(quals, path, name string) string` (in srcedit.go), never with `path + "." + name`.
  - When an addition changes an existing import's qualifier, `addImports` rewrites that import line, and its references in the file, from the old qualifier to the new one (design rule 9).
- `types.String` output such as `Post.PostId` is a full path. `typeImports` must map it to (import path, qualified text).

- [ ] **Step 1: Write the failing tests:**
  - `TestAddImports_Canonical`: adding `Admin.Layout` to a file with `import Layout` emits `import Admin.Layout as AdminLayout` and returns `AdminLayout`.
  - `TestAddImports_RequalifiesExisting`: a file with `import Admin.Posts.Index` and `Index.x` gets `Posts.Index` added. The result is `import Admin.Posts.Index as AdminPostsIndex`, `AdminPostsIndex.x`, and `import Posts.Index as PostsIndex`.
  - `TestAdmin_ShortQualifiers`: the generated admin page for `Post` uses `Layout.Page` / `Layout.isAdmin` (import `Admin.Layout`). Page values stay `Admin.Post.Index {}`.
- [ ] **Step 2: Run** `go test ./internal/generate -run 'AddImports|Admin_Short'`. Expected: FAIL.
- [ ] **Step 3: Implement.** Then regenerate `testdata/generate/want` with the package's golden update flag if it has one. Otherwise update the files by hand and diff them for review.
- [ ] **Step 4: Run** `go test ./internal/generate`. Expected: PASS.
- [ ] **Step 5: Commit** with `feat(generate): emit canonical qualifiers and aliases (#23)`.

### Task 6: Migrate the repo; suite green

**Model:** Sonnet. The work is mechanical: run the tool, then fix what it can't reach.

**Files:**
- Modify: `examples/**/app/**/*.alder`, `testdata/**` .alder sources (corpus, golden inputs, fixtures), `site/` sources, `book/14-second-app.md:270,274`, `book/15-generators.md:911,1156,1778,2281`, and Go test sources with inline Alder (`buildkit/adapters_test.go`, `ir/ir_test.go`, `names/names_test.go`, `gen/{names,gen}_test.go`, `generate/{program,regions}_test.go`, `cmd/alder/{example,generate,build,test,eval,admin,main}_test.go`)
- Not migrated: `experiments/`, which holds historical LLM transcripts.

- [ ] **Step 1:** Build the binary with `go build -o /tmp/alder-q ./cmd/alder`. In each example project, run `/tmp/alder-q fmt`. Run `regen admin` where admin regions exist, so the regions match the generator.
- [ ] **Step 2:** Migrate the testdata .alder files and the inline Alder in Go tests by hand or with a throwaway script (kept in the scratchpad, not committed), using the rules. Book code blocks: same.
- [ ] **Step 3: Run** `go vet ./... && go test ./...` under a 20-minute timeout, in the foreground. Expected: PASS. A known flake is `TestScratchRerunsOnPoll`, and `internal/format TestSpeed_Examples` can trip under load. Rerun those alone before treating them as failures.
- [ ] **Step 4: Commit** with `chore: migrate examples, testdata and book to short qualifiers (#23)`.

### Task 7: Scratch imports with canonical aliases

**Model:** Sonnet.

**Files:**
- Modify: `internal/scratch/scratch.go:568` `withImports`, `internal/scratch/lines.go:77-96` `headerLines`
- Test: `internal/scratch/scratch_test.go`

**Interfaces:**
- Consumes: `CanonicalQualifiers`.
- `withImports` writes `import P` or `import P as Q` for every app module (as it does today), using qualifiers computed over the full set. That set is the user's header imports plus the auto ones.
- `headerLines` accepts `import P as Q`.

- [ ] **Step 1: Write the failing tests:**
  - `TestScratch_AutoImportsAliasClashes`: an app with `Posts.Index` and `Admin.Posts.Index` and a scratch `> AdminPostsIndex.title` evaluates without a compile error.
  - `TestScratch_HeaderAs`: a header line `import Admin.Layout as AdminLayout` is accepted.
- [ ] **Step 2: Run** `go test ./internal/scratch -run 'AutoImportsAlias|HeaderAs'`. Expected: FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** `go test ./internal/scratch ./cmd/alder -run Scratch`. Expected: PASS.
- [ ] **Step 5: Commit** with `feat(scratch): canonical qualifiers for auto-imports (#23)`.

### Task 8: LSP follows qualifiers

**Model:** Sonnet.

**Files:**
- Modify: `internal/lsp/complete.go:39,40,135,621`, `scope.go:260-273,371` (`onQualifier`, `moduleNamed`), `hover.go:81-92`, `definition.go`, `references.go`
- Test: `internal/lsp/*_test.go` (next to the existing completion, hover and definition tests)

**Interfaces:**
- Consumes: `Program.Qualifiers(module)` (Task 3).
- `moduleNamed(a, q)` resolves a qualifier (or a page full path) to a module.
- `onQualifier` uses the qualifier text length from the source.
- Member completion after `Todo.` lists `Table.Todo`'s members.
- Import completion after `import ` offers full paths, and when the canonical rule requires it, the accepted item's text includes `as Q`.

- [ ] **Step 1: Write the failing tests:**
  - `TestComplete_MemberViaQualifier`: `Todo.` lists `find`.
  - `TestComplete_ImportInsertsAlias`: the completion for `Admin.Layout` in a file importing `Layout` inserts `Admin.Layout as AdminLayout`.
  - `TestHover_QualifiedShort`: hovering `Todo.find` shows `Table.Todo.find : …`.
  - `TestDefinition_Qualifier`: on `Todo` in `Todo.find`, definition jumps to app/Table/Todo.alder.
  - `TestAction_E0205Fix`: the code action replaces `Table.Todo.find` with `Todo.find`.
- [ ] **Step 2: Run** `go test ./internal/lsp -run 'MemberViaQualifier|ImportInsertsAlias|QualifiedShort|Definition_Qualifier|E0205Fix'`. Expected: FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** `go test ./internal/lsp`. Expected: PASS.
- [ ] **Step 5: Commit** with `feat(lsp): qualifier-aware completion, hover, definition (#23)`.

### Task 9: Tree-sitter `as`

**Model:** Sonnet.

**Files:**
- Modify: `tree-sitter-alder/grammar.js:191` (`import_decl: seq('import', field('module', $.module_name), optional(seq('as', field('alias', $.constructor))))`)
- Modify: `tree-sitter-alder/queries/highlights.scm:4`, `queries/neovim/highlights.scm:18` (`"as"` → `@keyword.import`, matching how `import` is captured)
- Regenerate: the parser sources (`tree-sitter generate`, the same way the M6a/#19 tasks did)
- Test: `tree-sitter-alder/test/corpus/` (a new `imports.txt` case)

- [ ] **Step 1: Write the corpus test** `import Admin.Layout as AdminLayout`, with its expected tree `(import_decl module: (module_name …) alias: (constructor))`.
- [ ] **Step 2: Run** `cd tree-sitter-alder && tree-sitter test`. Expected: FAIL.
- [ ] **Step 3: Change the grammar and the queries, then regenerate.**
- [ ] **Step 4: Run** `tree-sitter test`, then the repo's Go test for the grammar if it exists (`go test ./... -run TreeSitter`). Expected: PASS.
- [ ] **Step 5: Commit** with `feat(tree-sitter): import ... as alias (#23)`.

### Task 10: Book and explain docs

**Model:** Sonnet.

**Files:**
- Modify: `book/03-syntax.md:11-36,343` (Modules section), `book/A-grammar.md` (the import production), `book/B-pitfalls.md` (a pitfall: full path → E0205), `book/12-rationale.md:981,1005` (add the rationale: last-segment consensus, one spelling, canonical aliases), and `book/15-generators.md` where generator output is described
- Test: `book_test.go` runs the snippets

Content: design rules 1–6, written for a beginner. Per the book's teach-everything rule, also explain *how* the resolver computes the qualifier (rule 3) with the worked examples from Task 2's table. Replace the 03-syntax line "There are no unqualified imports and no renaming".

- [ ] **Step 1: Edit the chapters.** Every new code snippet must compile under book_test.go.
- [ ] **Step 2: Run** `go test . -run Book` (or the book test's actual name). Expected: PASS.
- [ ] **Step 3: Run** `go vet ./... && go test ./...` under a 20-minute timeout. Expected: PASS.
- [ ] **Step 4: Commit** with `docs(book): short module qualifiers and import aliases (#23)`.

## After merge (controller, not a task)

- `go install ./cmd/alder`. The user runs `:LspRestart` and `:TSUpdate alder`.
- **nightvote-replay** is a separate repo with about 27 files to migrate. Ask the user before running `alder fmt` there; the `Sites.MnSos` file will get `as DataMnSos`.
- Close #23 with a summary comment. Update #20's issue: tables in `app/Table/` are now written `Todo`.
