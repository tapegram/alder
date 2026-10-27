# Scratch send from nvim: visual map, definitions, eval latency (alder#44)

Design for [tapegram/alder#44](https://github.com/tapegram/alder/issues/44). The user opted out of the
brainstorm questions; every choice below is the author's call and is marked so.

## Intent

Sending code from an app module to `scratch.alder` with `<leader>as` and watching the result should
feel like a REPL: the map is visible in both modes, a definition lands as a definition, and `1 + 1`
answers in well under a second.

## What was measured (2026-10-26, nightvote-replay: 39 modules, ~22k lines of generated Go)

`alder scratch` with a one-line file `> 1 + N`, timed per phase. The LSP runs the same
`scratch.RunFile` on every save with no debounce, so the editor sees the same numbers.

| phase | changed expression | unchanged (Go cache hit) |
|---|---|---|
| check (Alder) | 36 ms | 38 ms |
| ir.Lower | 9 ms | 9 ms |
| gen.Emit (of which gofmt ~80 ms) | 185 ms | 185 ms |
| write files + deps | 38 ms | 38 ms |
| `go vet ./...` | **1.10 s** | 150 ms |
| `go build` (link of a 14.7 MB binary dominates; a main-only change costs the same 300 ms) | 310 ms | 110 ms |
| run the binary (first exec of a new binary on macOS) | 165 ms | 25 ms |
| total | **~1.9 s** | ~0.6 s |

Findings that shape the design:

- The scratch source lives in the app's Go package (`Scratch_alder.go` beside the app, or appended
  to the `in` module's file), so every edit re-vets the whole app. Vet is ~60% of the latency.
- Moving the scratch thunks into their own Go package would **not** help: the build floor is the
  link, not the compile. That idea is dropped.
- `-ldflags=-s -w` cuts the link to ~230 ms and the first exec to ~140 ms.
- Anything well under ~0.5 s needs no new binary per eval (a long-lived process with the app
  compiled in, evaluating scratch expressions). That is a separate design and gets its own issue.

## Goals

1. A changed-expression eval in ~0.5–0.6 s instead of ~1.9 s, with no loss of diagnostics.
2. `<leader>as` shows in visual mode under the `alder` which-key group.
3. A sent definition is inserted as a definition; only an expression becomes a `> ` watch.

## Non-goals

- A long-lived eval process / interpreter (filed as a follow-up issue).
- Per-module caching of emitted Go across runs (~100 ms; revisit with the follow-up).
- Changing what `alder build`, `alder test` or the REPL vet.

## Anti-goals

- Hiding a codegen bug: generated Go that compiles but does not vet must still surface somewhere.
- Guessing wrong about a definition and silently mangling what the user sent.

## Design

### 1. Eval latency (alder)

**Vet once per session, not per run** (author's call). The REPL vets its first build and again after
each `:reload`; the editor vets its first run that builds and does not re-vet after an app-file
change (app code is vetted by `alder build`/`alder run`, and re-vetting would add ~1.1 s to the
first eval after every app save). The flag is set only after a run that built and ran. Callers pass it as
`FileOptions.Build.NoVet` (`Build` is already a `scratch.Options`, and `RunFile` copies it through).
`CompileCheckedWith` already vets after a failed build when `NoVet` is set, so a build failure is
still explained by vet's message.

Who sets it:

- LSP (`internal/lsp/scratch.go`): the server's first scratch run vets (`s.scratch.vetted` flag,
  guarded by the existing mutex); every later run of any scratch document skips vet. The first run
  happens on `didOpen`, before the user types, so the slow run is never the one they are waiting on.
- `alder scratch` one-shot: unchanged, it vets.
- `alder scratch --watch`: the first run vets, reruns skip it.

**Strip the scratch binary**: `CompileCheckedWith` builds the scratch binary with
`-ldflags=-s -w`. Panic traces still carry file:line (pclntab stays); only DWARF goes. Applies to
every scratch build (file, REPL, eval, test, mutate all go through this function) — check that
nothing reads DWARF from these binaries (nothing does today; `go test -race` is not involved).

**Emit without gofmt for scratch runs** (author's call, optional, last): `gen.Options.NoFormat`
skips the two `format.Source` passes on the per-module files (`sourceFile`); the small app and
routes files stay formatted. `RunFile` sets it through `scratch.Options.NoFormat`. One pass over
all files measured ~80 ms, so the saving is ~80–160 ms. Generated Go in
`build/<app>/scratch/file-*/` becomes unformatted, which nobody reads. The task is to measure the
gain on nightvote-replay first and drop it if the saving is under 50 ms, since `format.Source` also
acts as a parse check for gen (a build failure reports the same problem, less prettily).

Expected result: ~1.9 s → ~0.55 s for a changed expression. The PR reports the measured numbers.

### 2. Visual-mode map visibility (alder.nvim)

`keys.attach` registers the which-key group `<leader>a` = alder only for normal mode
(`wk.add` defaults to `n`). The user's global config labels `<Leader>a` as "Avante" in visual mode,
so in visual mode which-key shows that group and the alder map is not where they look. Fix:
register the group for `{ 'n', 'x' }` (`mode = { 'n', 'x' }` in the `wk.add` spec). The map itself
is already set in `x`.

Test: load a fake `which-key` module into `package.loaded` that records `add` calls; attach a
buffer; assert the group spec lists both modes.

### 3. Definitions vs runs (alder.nvim)

`scratch.send_current` / `scratch.append` classify the (dedented, blank-trimmed) selection by its
first line:

- **Definition**, inserted verbatim (no `> `, no extra indent on later lines):
  - starts with a declaration keyword followed by a space: `type`, `alias`, `import`, `page`, `msg`,
    `form`, `table`, `capability`, `fake`, `session`, `login`, `test`;
  - a signature: `^[%a_][%w_']*%s*:` (`name : Type`);
  - a binding: a lowercase name, then only pattern characters (`[%w_'%s(),{}:]`), then `=` not
    followed by `=` — so `f x = …`, `pair (a, b) = …` are definitions, while `x == y`, `f x <= y`,
    `{ a = 1 }`, `let …`, `if …` are expressions.
- **Expression**, wrapped as today: first line `> expr`, later lines indented four spaces.

In a scratch file (`raw`), lines holding `>` watches still go in as they are.

A definition sent from module `M` into a scratch `in M` duplicates `M`'s name; the checker reports
E0009 ("already declared") as a scratch diagnostic. That is left as is (author's call): the user
renames or drops the `in` line, and the diagnostic says which.

README's keys table gets the two behaviours.

## Constraints

- Go and nvim changes are separate PRs in separate repos; land alder first (the nvim part does not
  depend on it).
- Both test suites stay green; no new `go-build` cache growth beyond the usual.

## Left to the builder

- Exact placement of the `vetted` flag and whether `--watch` shares the CLI's code path for it.
- Lua pattern details, as long as the listed examples classify as stated and are tested.
- Whether `NoFormat` is kept, by measurement.

## Follow-up issue to file

"Scratch: evaluate without building a binary per run" — a long-lived process with the app compiled
in (or an IR interpreter), the only route well under 0.5 s; include the table above as its baseline.
