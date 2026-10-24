# Design: a Unison-style scratch file (alder #19)

Source: the proposal and decisions in the comments on tapegram/alder#19, copied here so the plan travels with its spec. **The decisions (§0) override the proposal wherever they differ.**

## 0. Decisions (2026-10-22)

- **Scope v1: pure expressions only.** No Query or Task evaluation; effectful watches are a later step.
- Results are virtual text at the end of the watch line, with virtual lines for multi-line values.
- `alder repl` is deprecated when scratch ships; nvim keys move to the scratch file; the REPL is removed a release later.
- `scratch.alder` is committed, as executable notes.
- Defaults taken from the proposal: any `*scratch.alder` outside app/ (`scratch.alder` is scaffolded); an `in Module` header for context; build the engine + `alder scratch --watch` first, then LSP + nvim.

Status: proposal, 2026-10-22. Read-only survey of `~/projects/alder` and `~/projects/alder.nvim`; nothing edited.

### 1. The complaint

From `nightvote-replay/ALDER_USAGE_NOTES.md`: the REPL has no highlighting or completion, multi-line entry (`\` continuations over a `bufio` line reader) is painful, and what the user actually wants is Unison's model: a text file, run on save, where each `>` line evaluates an expression. The editor supplies editing, highlighting and completion; the tool supplies results.

### 2. What exists and can be reused

| Piece | Where | Reusable for the scratch file |
|---|---|---|
| `scratch.Module` + `scratch.Check` | `internal/scratch/scratch.go` | Checks the app with one extra in-memory module: either a named module (`Scratch`) with every app module implicitly imported, or **in-module mode** (`Module.In`) appended to an app module's own file. `Infer: true` infers unannotated top-level values. `Result.Values["Scratch.<name>"]` gives every top-level value's type. Exactly what a scratch file needs, unchanged. |
| `scratch.CompileCheckedWith` + `RunTethered` | `internal/scratch/scratch.go`, `run.go` | Builds a caller-supplied `main.go` against the checked program into `build/<app>/scratch/<purpose>/` under a flock, runs it tethered (dies with the parent), 60 s `scratch.Timeout`, cancellable ctx. Unchanged. |
| `scratchMain`, `evalIt`, `typeParamsOf`, `classifyRun`, `devDBHint` | `cmd/alder/scratchmain.go`, `eval.go` | Generates the pure/Query/Task mains (dev.db via `ALDER_EVAL_DB`, `app.TestSession()`, adapters wired via `caps.Caps()`), instantiates polymorphic `it`, classifies crashes. **Lives in `cmd/alder`, so the LSP cannot import it** — must move to `internal/`. |
| `parseItems`, `splitItems`, `assemble`, `evalSource` | `cmd/alder/evalinput.go` | Splits input into items at column-1 lines (`snippets.Starts`: closers continue), builds a source and a line/column map so diagnostics land on the input. The `> expr` → `itN = expr` rewrite with a column shift is this code with a different prefix. Also `cmd/alder`; move. |
| REPL `report`/`printDiag`/`owner` | `cmd/alder/repl.go` | Diagnostic mapping back to entries. Superseded by a simpler one-file map, since the whole file is one module. |
| REPL `reachable` (lets as package vars, only used ones compiled) | `repl.go` | Not needed: a file is a module; every declaration is compiled once. |
| `watch.Watch` / `Snapshot` / `Diff` | `internal/watch` | Polls `app/**`, `adapters/**`, `migrations/`, `static/**`. Needs one extra: the scratch file itself. |
| LSP `analysis` snapshot, `publish`, fast/slow workers, `latestPreview` supersede, custom methods (`alder/preview`, `alder/plan`, `alder/generators`) | `internal/lsp` | Pattern for a custom `alder/scratch` notification and a slow handler that is superseded by a newer save. `s.rel()` accepts any file under the root (not only `app/`). No `didSave` yet (`textDocumentSync.save` absent). |
| `rt.Show` | `rt/show.go` | Prints any value as Alder, `Html` as markup, `<Query>`/`<Task>` for effects. The `:html` command is just `Show` on an Html value. |
| alder.nvim | `lua/alder/repl.lua`, `check.lua`, `lsp.lua` | Terminal-split REPL with `send` (joins lines with `\`); diagnostics in the `alder` namespace; `lsp.request` helper. No extmark/virtual-text code yet, but `vim.api.nvim_buf_set_extmark` is all that is needed. |
| `alder eval --json` | `eval.go` | The agent-facing contract `{value, type, diagnostics, error, hint}`; the file runner's JSON should be a list of these keyed by line. |

Costs today: the REPL does one `Check` + one `go build` + one run **per expression**. A file does one of each **per save**, for all watches — strictly cheaper per result.

### 3. Prior art

- **Unison** (`scratch.u`): UCM watches `*.u` files; `> expr` lines are watch expressions evaluated on save, results printed in UCM's terminal (never written into the file), cached by content hash so unchanged watches are not re-run; `test> ` marks a test watch; definitions in the file are typechecked and only enter the codebase on an explicit `add`/`update`. Watches are pure; IO needs `run`. Closest model to this proposal.
- **Haskell**: `doctest` (`-- >>> expr` with expected output), `ghcid` (`-- $> expr` re-evaluated on every reload, printed in ghcid's pane), and HLS's Eval code lens, which **writes results back into the source as comments**. Three answers to "where do results go": a terminal, or the file.
- **Elm**: nothing. `elm repl` is line-based, backslash continuations, no editor support — the state Alder is in. The community uses Ellie or `elm-live` with a scratch `Main.elm`.
- **Clojure**: `(comment …)` "rich comment blocks" plus eval-form-at-point (CIDER/Calva) with the result as an inline overlay. Stateful nREPL; the inline-overlay UX is what the nvim side below copies.
- **Jupyter / Pluto.jl / Livebook**: cells with stored outputs. Jupyter's hidden-state and order problems come from a mutable kernel; Pluto re-runs dependents reactively. Alder's declarative module makes a scratch file order-independent for free — closer to Pluto than Jupyter without any dependency tracking.
- **Swift Playgrounds / Xcode**: results in a gutter column, re-run on edit, multi-line values expandable. The "types instantly, values after the run" split below is in that spirit.

### 4. The design

#### 4.1 File name and location

Rule: **any file whose name ends in `scratch.alder`, anywhere under the project root except `app/`**. `scratch.alder` at the root is the default that `alder new` scaffolds; `todos.scratch.alder` or `experiments/auth.scratch.alder` are as valid. The `.alder` extension gives filetype, highlighting and LSP attachment for free; the `scratch` suffix tells the LSP and CLI to run it through the scratch pipeline rather than the app overlay. The build reads only `app/**` (`buildkit.Load*`), so a scratch file outside `app/` can never ship. A scratch file **under** `app/` gets one diagnostic at line 1 ("a scratch file does not belong under app/; move it to the project root") and is otherwise ignored by the build.

Alternative (open question 4): the name selects the module context, `Todos.Index.scratch.alder` meaning `in Todos.Index`.

#### 4.2 Syntax

The file is an ordinary Alder module with one addition: **watch lines**.

```alder
-- app scope: every app module is visible qualified (Todo.countOpen, Todos.Index.view)
in Todos.Index          -- optional first line: in-module mode, as `:in` / `--in`

sample : List Todo
sample = [ { title: Text.NonEmpty "Milk", due: Nothing, owner: me } ]

> countOpen sample                      -- ⇒ 1 : Int
> List.map (\t -> t.title) sample       -- ⇒ [Text.NonEmpty "Milk"] : List Text.NonEmpty
> view { filter: All, todos: sample }   -- ⇒ <ul class="todos">… : Html Msg

> Db.all (Db.select Todo)               -- a Query: runs against dev.db on save (reads only)
>! Db.insert Todo { title: … }          -- a Task: never on save; run on demand (§4.4)
> Todo.countOpen                        -- ⇒ <function> : List Todo -> Int
```

- `> expr` at column 1 is a watch. Continuation lines are indented (or closers `}` `)` `]` `</tag>` at column 1), exactly the `snippets.Starts` rule `parseItems` already uses, so a multi-line `match`, record or element pastes as written.
- Everything else is module code: declarations with or without signatures (`Infer: true` so no E0007), `type`s, `test`s, explicit `import`s. No `let` keyword: in a file, a definition is a definition. Editing one and saving replaces it; "redefinition" semantics, `:reset` and `:reload` all disappear.
- Watches are anonymous: a later watch cannot refer to an earlier one. Name it instead (`x = expr` then `> x`).
- The preprocessor rewrites watch `k` to `it<k> = expr` with a column shift (`> ` is 2 bytes, `it3 = ` is 6; the existing `shift` map handles it) and refuses a file that declares a name matching `it[0-9]+`, as eval refuses `it`.
- Comments are comments. `> ` inside a `--` comment or a string is not a watch (the preprocessor only looks at column 1 of lines outside items, after `snippets.Starts`).
- Blank lines and comments between watches: nothing special.

#### 4.3 Module context

`in Todos.Index` as the first non-comment line selects in-module mode (`scratch.Module{In: "Todos.Index"}`): the module's declarations and imports unqualified, as a `test` written in that module. The file's own declarations are appended to the module, so a name the module already declares is refused with a diagnostic on the declaration (today's "`Todo` already declares x", positioned instead of printed). Without the line the file is module `Scratch` (`--module`'s renaming stays available as a CLI flag / `module Probe` header for an app that has a module named `Scratch`).

#### 4.4 Effects

Effects are typed, so the tool knows before running anything what each watch would do:

| Watch type | `>` on save | `>!` |
|---|---|---|
| pure value | evaluated, shown | same as `>` |
| `Query a` (reads) | **run against `build/<app>/dev.db`**, shown as `value : a` | same |
| `Task a` (reads and writes) | not run; shown `<Task> : Task a  (>! runs it)` | run **only on an explicit command**, writes commit; result `value : a` |
| `Select` | shown `<Select>` with the REPL's hint | same |
| typed hole anywhere in the file | nothing runs; types still shown; the hole is a diagnostic (W0002, hint severity) | — |

Why Queries run on save: reads are safe and the whole point of a scratch file next to an app is to look at data. Why Tasks do not: a file re-runs on **every save**, including saves for unrelated edits; a `>!` that inserted a row on each would be a footgun no warning fixes. The explicit command (nvim `<leader>a!` on the line, or every `>!` in the file with a bang; CLI `alder scratch --tasks`) is the "I mean it" step, and it is still one keystroke. Open question 2 lists the alternatives (Unison-style change-hashing, a throwaway in-memory DB like `alder test` uses).

Both run as the REPL does: `app.TestSession()`, adapters wired when `adapters/` has Go (`Checked.WiresAdapters`), unwired capabilities warned once (`Build.Warning`), never any environment but the dev database.

#### 4.5 Where results appear

Two phases per save, because the check is milliseconds and the build-and-run is seconds:

1. **After `Check`**: every watch's type (`: Int`), every diagnostic positioned in the file. The user sees type errors and types immediately.
2. **After the run**: values (`⇒ 3 : Int`), run failures per watch, crash notice.

Transport, in order of preference:

- **LSP, custom notification `alder/scratch`** `{uri, version, phase: "types"|"values", watches: [{line, kind, type, value?, error?, truncated?}], ran: bool, note?}`. Errors in the file go through ordinary `textDocument/publishDiagnostics` (so quickfix, `]d`, code actions all work as in app files). Values are **not** diagnostics: they would inflate diagnostic counts, pollute `]d`, and read as problems.
- **alder.nvim**: renders the notification as extmarks in a `alder/scratch` namespace: `virt_text` `⇒ value : Type` at the end of the `>` line (`Comment`-ish highlight), `virt_lines` below for multi-line values (Html markup, long lists), capped at ~8 lines / 2 KB with `… (N more lines, <leader>ao to open)`; `<leader>ao` opens the full value in a float or split. While a run is in flight the old values dim (stale highlight); a failed watch shows `✗ message` in `DiagnosticError`. One Lua module, ~150 lines.
- **Why not inlay hints**: pull-based, one line only, no stale state, and VS Code/nvim truncate them. They can be added later from the same cached result for editors without a plugin (cheap: `textDocument/inlayHint` returning `⇒ value` per watch line).
- **CLI fallback `alder scratch [file] [--watch] [--json] [--tasks] [--in Name]`**: prints each watch as `file:line: expr\n  ⇒ value : Type`, diagnostics as `alder check` does; `--watch` re-runs on the file's or the app's changes (ghcid in a tmux pane; works in every editor); `--json` is the agent contract, a list of per-watch `{line, expr, value, type, error}` plus `diagnostics`. This is the engine's first consumer and the thing to build first.

#### 4.6 Errors, HTML, long output, crashes

- **App errors**: the app itself does not check → one diagnostic at the file's line 1 ("the app has errors; fix them and save") plus the app diagnostics on their own files (the LSP publishes those anyway). Nothing runs.
- **File errors**: diagnostics in the file's coordinates (via `Checked.MapDiag` + the `>` shift map); the fixes carried as `data.fix` so the quick-fix code action works. Watches still get types where inference succeeded.
- **Run failures**: the generated main wraps **each** watch in `recover` and prints a per-watch error line, so a failed Query on line 12 does not hide line 14's value. A stack overflow or timeout is fatal (Go cannot recover a stack overflow): every watch not yet printed gets "not run: the program crashed at line 12 (stack overflow)" — the main prints watches in file order, so the crash line is known.
- **Html**: `rt.Show` renders markup; shown as a multi-line value, truncated as above. A later nicety: `<leader>ao` on an Html watch opens it in the browser through the dev UI; not in scope.
- **Long output**: the engine caps a value at 64 KB (with `truncated: true`); the client caps the inline rendering further. `--json` carries the capped value.
- **Side effects and safety**: Tasks are explicit (§4.4); only `dev.db`; `TestSession`; 60 s timeout; tethered process group killed on cancel or supersede; writes only under `build/<app>/scratch/file-<hash>/`; the file can never be under `app/`. The old-REPL risk of `:run!` on a hand-typed line is unchanged in kind and smaller in practice (the line is in a file you can read before running it).
- **Saves during a run**: a new save cancels the running build/run (the `latestPreview` supersede pattern) and starts over; the terminal watcher coalesces bursts as `watch.Watch` does.

#### 4.7 The REPL

Share the engine, deprecate the loop. Everything the REPL does the file does better: `let` → a declaration; `:type` → the type phase; `:run` → `>` on a Query; `:run!` → `>!`; `:html` → `>` on Html; `:reload` → save; `:reset` → delete lines; `:in` → the `in` header. What a line REPL keeps is "no file on disk", which nobody asked for. Agents keep `alder eval` (`--json`, whole input at once) and gain `alder scratch --json`.

Recommended path: ship scratch; the next release prints a deprecation line in the REPL banner pointing at `alder scratch`; alder.nvim's `<leader>ax` opens (creating if needed) `scratch.alder` and `<leader>as` **appends the selection as a `>` watch** to it and saves — the muscle memory survives; remove `alder repl` and `repl.lua` one release later. Keeping both forever means two diagnostic mappers and two sets of docs for one engine.

### 5. Implementation pieces (in order)

1. **Hoist eval's run machinery into `internal/`** (S, ~0.5 d). Move `scratchmain.go`, `evalIt`, `typeParamsOf`, `classifyRun`, `devDBHint` and `evalinput.go` (`item`, `parseItems`, `splitItems`, `assemble`, `evalSource`) to `internal/scratch` (or a sibling `internal/scratchrun` if the import of `alder/rt/db` for `DevRemedy` is unwelcome in `scratch`). `eval.go` and `repl.go` keep working through the new names; tests move with them. Pure refactor, its own PR.
2. **File runner `internal/scratch/file.go`** (L, ~2 d). `ParseFile(text) (File, []diag)` → header (`in`/`module`), module source, watches `[{line, expr, name}]`, position map. `Run(ctx, dir, File, Options{Tasks, Timeout, Work, OnTypes func(...)}) Result`: one `Check`, types phase via callback, one `CompileCheckedWith` with a **multi-watch main** (pure/Query via `eff.RunQuery`, Task via `eff.Run` only when enabled, per-watch `recover`, a line-framed stdout protocol such as `\x1e<k>\n<value>\n`), one `RunTethered`, outputs mapped to watches, failures classified. `Result` is JSON-ready. Tests on `examples/todos`: pure, Query, Task skipped/run, multi-line watch, in-module header, hole, app error, crash mid-file, truncation.
3. **`alder scratch` CLI** (M, ~1 d). `cmd/alder/scratch.go`: flags, text and `--json` renderers, `--watch` using `watch.Watch` plus a stamp of the scratch file (small `watch` change: `WatchWith(dir, extraFiles…)` or a second poller), supersede on burst, Ctrl-C handling as `eval`. Help/usage line in `main.go`.
4. **LSP** (M/L, ~1.5 d). `textDocumentSync.save: true` and a `textDocument/didSave` notification; recognise `*scratch.alder` docs; on open/save (open too, so a reopened file shows results) queue a scratch run on the slow worker with its own supersede key; publish the file's diagnostics through `publish` (keyed by its rel; the analysis goroutine's `reported` set already clears stale ones); send `alder/scratch` twice (types, values); `alder/scratchTasks` request `{uri, line?}` to run `>!` lines. Keep scratch docs out of `rebuild`'s overlay (they are not app modules). Completion/hover in a scratch file: the fast handlers look up `a.lines[rel]`; the scratch run should store its `Checked` as a per-file analysis the handlers consult when `rel` is a scratch file (hover/definition/completion then work via the same scope code, since the scratch module is a real module of that program). If that is too much for the first cut, completion of app names already works through the last good app analysis only for app files; defer scratch-file completion to a follow-up and say so.
5. **alder.nvim** (M, ~1 d). `lua/alder/scratch.lua`: handler for `alder/scratch` (extmarks, truncation, stale dimming, per-watch errors), `:AlderScratch` (open/create `scratch.alder` from `alder new`'s template), `<leader>a!` run task at cursor / `:AlderScratchTasks` all, `<leader>ao` open full value, `<leader>as` append selection as a watch and save; `checkhealth` says whether the server has `alder/scratch`; README and keys table.
6. **Docs and scaffold** (S, ~0.5 d). `alder new` writes `scratch.alder` with a three-line comment header (and, per open question 7, a `.gitignore` entry); book chapter 11 gets a "Scratch file" section replacing most of the REPL section; REPL banner deprecation line.
7. **Later, optional**: inlay-hint provider for plugin-less editors; results-into-file mode (`alder scratch --write`, HLS-eval style) for sharing; Unison-style per-watch hashing to skip unchanged watches; generating the scratch module as its own Go package so the app package stays in Go's build cache between saves (the per-save `go build` cost is the one real UX risk — the REPL pays it per expression today, so this is no regression, but it is the first thing to measure once the CLI exists).

Rough total for 1–6: 6–7 days of subagent work; 1 and 3 land independently, 4 and 5 together.

### 6. Open questions (user decides)

1. **File naming.** (a) exactly one `scratch.alder` at the root; **(b) any `*scratch.alder` outside `app/`, `scratch.alder` scaffolded** (recommended); (c) a `scratch/` directory of plain `.alder` files.
2. **Task lines (`>!`).** (a) run on every save against `dev.db` (REPL `:run!` parity; duplicate writes per save); **(b) run only by explicit command, the line under the cursor or all with a bang** (recommended); (c) run on save only when the watch's text or anything it depends on changed since its last run (Unison hashing; subtle); (d) run on save against a fresh in-memory database as `alder test` does (safe, but starts empty every time).
3. **Query lines.** **(a) `>` runs Queries on save, reads being safe** (recommended); (b) `>` shows `<Query>` as the REPL does and `>?` opts in.
4. **Module context.** **(a) `in Todos.Index` header line** (recommended, mirrors `:in`/`--in`); (b) the file name: `Todos.Index.scratch.alder`; (c) CLI flag / nvim buffer setting only.
5. **Result display in nvim.** **(a) virtual text at the end of the watch line, virtual lines for multi-line values** (recommended); (b) Information-severity diagnostics (zero plugin code, works in any editor, but noisy and reads as problems); (c) written into the file as `-- ⇒ 3 : Int` comments under each watch (HLS-eval style; diffable and editor-agnostic, but the tool edits your buffer).
6. **The REPL.** (a) keep both indefinitely; **(b) deprecate when scratch ships, nvim keys move to the scratch file, remove a release later** (recommended); (c) remove with the same PR.
7. **Build order.** **(a) engine + `alder scratch --watch` first, then LSP + nvim** (recommended: proves the engine, usable in any editor from day one); (b) LSP + nvim first, CLI after.
8. **`scratch.alder` in git.** (a) `alder new` adds it to `.gitignore` (personal workbench); (b) commit it (Unison projects commit scratch files; useful as executable notes).

