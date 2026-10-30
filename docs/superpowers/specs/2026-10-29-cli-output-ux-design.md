# CLI output UX: styled, readable output on a terminal

Design for [tapegram/alder#47](https://github.com/tapegram/alder/issues/47). The user said the CLI's
output is "a poor user experience" and left two things to themselves: which dependency (Charm or an
internal package) and which outputs bother them most. They were not available, so this spec surveys
every human-facing output itself, ranks them, and designs on the default that needs no decision: a
small internal styling package. Everything marked **(call)** is the author's decision, not the
user's; the ones the user may want to overturn are collected in "Decisions left to the user" at the
end. The LSP wish in the issue (alder-ui completion from the Custom Elements Manifest) is a separate
piece of work and is not touched here.

## Intent

A person at a terminal should see at a glance what failed, where the list ends, and what the
numbers are. Today every line has the same weight: 116 `ok` lines end in a `116 passed, 0 failed`
that looks like the 117th test; a `FAIL` scrolls off the screen; `alder help` is one 1,300-character
line; `alder explain` is a 1,000-character paragraph; a diagnostic's location, severity, message,
source and caret are five lines of the same grey. Agents, Alder's main users, never see any of
this: they get `--json`, or they pipe, and both stay byte-identical.

## Goals

1. On a terminal, status words, locations, summaries and labels carry colour and weight; the
   bytes between the escape sequences are today's output.
2. Piped output, `NO_COLOR`, `TERM=dumb` and every `--json` mode are unchanged byte for byte.
3. The three outputs that read worst (`alder test`, `alder help`, diagnostics) also get the small
   structural fixes that colour alone cannot give: a failure recap, a command list, a count.
4. No new dependency: the import rule (stdlib + `alder/*` + modernc) stands.

## Non-goals

- A TUI, progress bars, spinners, or live-updating lines. Bubble Tea's moment is the TUI, as the
  issue says.
- Changing any JSON shape, any text that an existing test pins when piped (`ok   M "name"`,
  `N passed, M failed`, the gate table, `wrote …`), or the streams lines go to.
- A diff of `left:`/`right:` in a failing test, folding the `trace:` line, or restyling the app's
  own log lines beyond their level word. Worth separate issues if wanted.
- 256-colour or truecolour: the 16 ANSI colours only, so the terminal's theme applies **(call)**.
- Styling anything `rt/` writes. The runtime is copied into every built app and must not depend
  on `internal/`; `alder test` restyles the test program's report on its own side (below).

## Anti-goals

- Replacing a status word with a glyph (`ok` → `✓`) or re-ordering lines on a terminal only. The
  text a person greps for must be the text they saw. (`alder scratch`'s `⇒`/`✗` marks predate this
  spec and stay.)
- Reading `isatty` anywhere but in one place. Every command gets its `style.Style` from
  `style.For(w)` once and passes it down.

## The survey

Each command was run on a scratch copy of `examples/todos` (devel `ff0be434`), errors injected
where needed. Ranked by how hard the output is to read (density, hierarchy, buried failures):

1. **`alder test`** (the most-run command). 116 identical `ok   Module "name"` lines; the summary
   is the last of them with no distinguishing mark; a `FAIL` in the middle scrolls away; a failing
   browser test's `trace:` is one 330-character line:

   ```
   FAIL Todos.Index "adding a todo through the page"
     left:  { answer: Redirected /, todos: ["Milk"], summary: Just "1 open" }
     right: { answer: Redirected /, todos: ["Milk"], summary: Just "2 open" }
     trace: ["Db.insert User", "Test.login", "Test.get Todos.Index", "Db.mine Todo", "Db.all Todo", …
   ok   Todos.Index "Delete removes the todo"
   1 passed, 2 failed
   ```
2. **`alder help` / a usage error.** One line: `usage: alder new <name> | alder new page|table|…`
   through 36 commands, 1,300 characters, no line breaks. (`alder <command> --help` is fine.)
3. **Diagnostics** (`check`, `build`, `test`, `eval`, `scratch`, the generators). Location,
   severity, message, source and caret all plain; a 150-character message and an 85-caret line
   under it; no count of how many followed:
   ```
   app/Todos/Index.alder:109:5: error[E0003]: cannot compare `{ trace: List Text, done: Maybe Bool }` with …
         { trace: r.trace, done: done } == { trace: ["Db.updateMine Todo"], done: Just "yes" }
         ^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^
   app/Todos/Show.alder:59:16: error[E0002]: unknown name `nope`
   ```
4. **`alder test --mutate`.** `208 mutants: 206 killed, 2 survived  (score 0.99)` is printed
   *above* `survived:` and its table, so with many survivors (a `-run` of a few tests lists
   dozens) the score has scrolled off when the command ends. Columns are already aligned.
5. **`alder run` startup.** Three voices at one weight: `alder run: building todos` (stderr),
   `dev server on 127.0.0.1:8080 → app on 127.0.0.1:54896` (stdout), then the app's Go-timestamped
   log `2026/10/29 23:32:51 info: migrated 0001_initial` and a 120-character `warn:` about
   `ALDER_SECRET`.
6. **`alder explain E0003`.** One paragraph of ~1,000 characters, unwrapped.
7. **Already structured, colour only:** `deploy --dry-run` (gate table, plan tree), `status`,
   `routes` (header + columns), `new <app>` (`created`, `next:`), the generators (`wrote`,
   `would write`, a diff, `dry run: nothing written`), `migrate` (`!! DROPS DATA:`), `fmt --check`
   (`<path> would be reformatted`), `scratch`, `eval` (`value : Type`). `alder check` on a clean
   app prints nothing (documented: "no output on success").

## The package: `internal/style`

```go
package style

type Style struct{ /* ansi bool; width int */ }   // the zero value is Plain
var Plain Style

func For(w io.Writer) Style                 // the one place that looks at the terminal
func (s Style) On() bool                    // true when sequences are emitted
func (s Style) Width() int                  // columns; 80 when unknown
func (s Style) Bold, Dim, Red, Green, Yellow, Cyan(text string) string
func (s Style) Mark(status string) string   // ok → Green; FAIL, error → Bold(Red); warning, SKIPPED → Yellow; else text
func (s Style) Wrap(text string, width int) string          // at word boundaries; existing newlines kept
func Columns(indent string, rows [][]string) string        // left-aligned, widths by rune count, two spaces between
func Lines(w io.Writer, f func(line string) string) io.Writer  // applies f to each complete line written to w
func Strip(s string) string                 // removes SGR sequences (tests)
```

Semantics:

- `For(w)` returns a styled `Style` only when all hold: `ALDER_COLOR` is not `never`; and either
  `ALDER_COLOR=always`, or (`w` is an `*os.File` whose `Stat().Mode()&os.ModeCharDevice != 0`
  and is not `os.DevNull` — the test `deploy.confirm` already makes — and `NO_COLOR` is unset or
  empty (no-color.org's rule) and `TERM` is neither `dumb` nor empty). Any other writer (a
  `bytes.Buffer`, a pipe, a `MultiWriter`) is `Plain`. **(call)** `ALDER_COLOR=always|never` is
  the only override: it makes the terminal path testable and lets `alder test | less -R` keep
  colour; `CLICOLOR_FORCE` and friends are not read.
- On Windows `For` is styled only with `ALDER_COLOR=always` **(call)**: enabling VT processing
  takes console syscalls this spec does not want; Windows Terminal users set the variable.
- `Width()`: `ioctl(TIOCGWINSZ)` on the file via `syscall` (`//go:build unix`, as `pid_unix.go`
  does; verified to compile on darwin and linux with stdlib only), else `COLUMNS` if it parses,
  else 80. Never below 40.
- Every method on `Plain` returns its argument unchanged; on a styled `Style` it wraps the text
  in `ESC[<n>m … ESC[0m` with `1` bold, `2` dim, `31` red, `32` green, `33` yellow, `36` cyan.
  Nesting is not supported: a method's argument must not already carry a sequence (the builder
  styles leaf strings, never a whole composed line).
- `Lines` buffers partial writes until `\n`, calls `f` on the line without its newline, and
  writes the result plus `\n`; `f` on an unrecognised line returns it unchanged. It is how
  `alder test` and `alder run` restyle another process's output.
- Invariant, pinned by tests: `Strip(styled) == plain` for every renderer in this spec.

## What changes, output by output

The rule everywhere: `st := style.Plain` when `--json`, else `style.For(w)` for the writer the
text goes to; styling inserts sequences and nothing else, except the four structural changes
marked **structural** below, which are explicit and tested.

**Diagnostics** — `diag.RenderTextStyled(d, lines, st)`; `RenderText` becomes the `Plain` case and
every caller in `cmd/alder` (`main`, `build`, `test`, `eval`, `scratch`, `fmt`, `generate`,
`migrate`, `deploy`, `routes`, `dump`) and `internal/buildkit`, `internal/generate/apply.go`
passes its style. `file:line:col` bold; `error[E0003]` bold red, `warning[W0002]` yellow; the
caret line in the severity's colour; `fix:` cyan; the message and the source line plain.
**Structural:** `alder check` on a terminal (not `--json`) ends with one line on stderr when at
least one diagnostic was printed: `2 errors, 1 warning` (bold red with errors, yellow without);
a clean run stays silent, as the book promises **(call)**.

**`alder test`** — the report is the test program's text, passed through `resultCounter`; that
writer becomes a `style.Lines` filter and recognises the shapes `rt/testing` writes: `ok   ` →
`ok` green; `FAIL ` → `FAIL` bold red, the rest bold; `  left:`, `  right:`, `  note:`,
`  error:`, `  seed:`, `  run:` → label cyan; `  trace:` → whole line dim; `^\d+ passed, \d+
failed$` → bold green when `0 failed`, bold red otherwise; `^\d+ mutants: ` → bold; `survived:`
→ bold red; a survivor row's kind column yellow; `still running …` (stderr) → yellow; the
no-tests hint → dim. **Structural:** on a terminal, after a summary with failures, the filter
prints `  FAIL Module "name"` once per failed test it saw (the recap; the names it counted),
so the failures are on screen when the run ends **(call)**. `--mutate`'s score line is not moved
(the deploy gate and nightvote-replay read the piped form) **(call)**.

**`alder help` and a usage error** — **structural**, on every output: `usageTo` prints
`usage: alder <command> [arguments]`, a blank line, then one row per command of `commandTree`
(`helpjson.go`), `style.Columns("  ", name, summary)`, then `alder <command> --help prints its
usage; alder help --json prints the command tree`. The 36-way `|` line goes; per-command usages
are unchanged. On a terminal the names are bold. One source for the list (the tree that `help
--json` already serves) **(call)**.

**`alder test --mutate`** — colour as above; no re-ordering (Non-goals).

**`alder run`** — its own lines: `alder run: building …`/`rebuilding …`/`rebuilt; app on …`
dim; `dev server on A → app on B` bold. The child's log goes through `style.Lines` on `logs`: a
leading `YYYY/MM/DD HH:MM:SS ` dim, then `info:` green, `warn:` yellow, `error:` red; other
lines untouched. `report()` styles only what it prints: `errState.text` (the browser overlay)
stays plain.

**`alder explain`** — **structural:** on a terminal the paragraph is `st.Wrap(text, min(Width(),
100))`; piped it stays one line **(call)**.

**Colour only** (one `st.Mark`/`Bold`/`Dim` each, no text change): deploy's gate `line` in
`internal/deploy/gate.go` (status via `Mark`), `plan`/`gate`/`artifact`/`deployed … to`
headings bold, `warning:` yellow; `status` labels (`current`, `builds`, `database`, `backup`,
`silos`) bold and the `*` row green; `routes` header bold; `wrote`/`created`/`updated` green,
`would write`/`would edit` cyan, `dry run: nothing written` dim, `next:` bold; `!! DROPS DATA:`
bold red; `<path> would be reformatted` path bold; scratch's `file:line: expr` bold, `✗` line
red, `(not run)` dim; eval's ` : Type` dim; `alder <cmd>: …` error lines on stderr: the
`alder <cmd>:` prefix bold red. `internal/deploy` takes the style through `Options`/a parameter
from `cmd/alder` (`progress` is `os.Stdout`/`os.Stderr` there, so `For` sees the terminal).

## Where things change

| layer | files |
|---|---|
| new package | `internal/style/style.go`, `width_unix.go`, `width_other.go`, `style_test.go` |
| diagnostics | `internal/diag/render.go` (`RenderTextStyled`), `render_test.go` |
| CLI | `cmd/alder/main.go` (usage, check summary, explain), `help.go`, `test.go` (the filter + recap), `run.go` (log filter), `build.go`, `fmt.go`, `generate.go`, `new.go`, `eval.go`, `scratch.go`, `migrate.go`, `routes.go`, `dump.go`, `deploy.go`, `deploy_init.go`, `ci.go` (style plumbing) |
| deploy | `internal/deploy/gate.go` (`line`), `deploy.go` (headings, status), `internal/generate/apply.go`, `internal/buildkit/buildkit.go` (pass-through of a style) |
| book | `book/11-tooling.md`: the "Commands" block becomes `alder help`'s output; a paragraph after it: styling is terminal-only, `NO_COLOR`, `TERM=dumb`, `ALDER_COLOR=always\|never`, `--json` never styled; `alder check` shows a count after diagnostics on a terminal |
| site | `book/site/build.py` regenerates after the book edit |

## Tests the builder adds (by behaviour)

`internal/style`: a buffer, a pipe and `os.DevNull` give `Plain`; `NO_COLOR=1` and `TERM=dumb`
give `Plain` even with a character device (fake via `ALDER_COLOR` is not enough — test `For`'s
pieces through an internal `decide(isTTY bool, env func(string) string)`); `ALDER_COLOR=always`
styles a buffer and `never` unstyles a terminal; every method on `Plain` is identity;
`Bold("x") == "\x1b[1mx\x1b[0m"` and the five colours likewise; `Mark` maps the six status
words and leaves others; `Columns` aligns by rune count (a `→` and a `é` in a cell); `Wrap`
breaks at spaces only, keeps existing newlines, leaves a word longer than the width whole;
`Lines` styles across partial writes and flushes nothing for a trailing partial line until the
newline arrives; `Strip(Bold(Red(…)))` is the text.

`internal/diag`: `Strip(RenderTextStyled(d, lines, styled)) == RenderText(d, lines)` for a
diagnostic with a fix, one with `NoSource`, one without a code; the severity word and the
caret line carry the colour, the message does not.

`cmd/alder`: **`TestCLI_JSONIgnoresStyle`** pins goal 2: with `ALDER_COLOR=always`, `check
--json`, `routes --json`, `help --json`, `eval --json`, `new page … --dry-run --json`, `test
--json` (todos) and `deploy local --dry-run --json` produce stdout and stderr identical to the
run without the variable, and contain no `\x1b`. `TestCLI_PipedUnchanged`: the same commands
without `--json` and without the variable match today's goldens (the existing text tests keep
passing untouched). `TestHelp_ListsEveryCommand` (each `commandTree` name on its own row,
`TestCLI_BadFlag` still sees `usage`). `TestCheck_CountLineOnTerminalOnly` (`ALDER_COLOR=always`
→ `2 errors, 1 warning` on stderr; without → absent; clean app → nothing either way).
`TestTest_StyledReport` (`ALDER_COLOR=always` on a failing todos variant: `ok` green, `FAIL`
bold red, summary bold red, recap lists both failed names after it, `Strip` of the whole equals
the plain run). `TestTest_NoRecapWhenAllPass`. `TestRun_LogLineStyling` (the filter function on
`2026/10/29 23:32:51 warn: x`, a line without a timestamp, and `listening on` untouched).
`TestExplain_WrapsOnTerminalOnly`. `TestDeployGate_MarksStyled` (`  test     ok  116 passed`
with `ok` green under `ALDER_COLOR=always`; byte-equal to today when not).

## Left to the builder

- Whether `resultCounter` grows the filter or a second writer wraps it; the recap needs the names,
  which the counter already parses.
- The exact SGR for "bold red" (`1;31` in one sequence is fine).
- Order of the plumbing commits: package → diag → test → help → run → explain → colour-only.

## Verification

- `go test ./...` green, including the new tests and the book (`TestBook_Commands` runs piped,
  so it sees plain text).
- By hand on a terminal: `alder test examples/todos` (green summary), a failing variant (red
  summary, recap), `alder help`, `alder check` with two errors (count line), `NO_COLOR=1 alder
  test …` (plain), `alder test … | cat` (plain), `alder explain E0003` (wrapped).

## Decisions left to the user

> **For the coordinator to surface.** Everything above runs without these; each is reversible.

1. **Charm instead of the internal package.** This spec uses `internal/style` (about 150 lines).
   If the user prefers Lip Gloss: the import rule gets a written exception for `cmd/alder` and
   `internal/style` only (never `rt/`); `style.Style` becomes a thin facade over
   `lipgloss.Style` so the call sites above do not change; `For` still owns detection (Lip Gloss
   detects too, but through `termenv`, which honours `NO_COLOR` and `CLICOLOR_FORCE` differently
   — the facade keeps Alder's rules); `Columns` is kept (Lip Gloss's table is more than needed);
   `go.mod` gains `charmbracelet/lipgloss` and its transitive modules (`x/ansi`, `termenv`,
   `muesli/reflow`, `x/sys`, …), and the Dockerfile/`alder deploy init` images fetch them.
   The TUI question (Bubble Tea) stays open until a TUI is on the roadmap.
2. **The ranking.** If the outputs that bother the user are not `test` → `help` → diagnostics →
   `--mutate` → `run` → `explain`, the colour-only group and the structural changes can be
   re-ordered freely; the package and the JSON pin come first in any order.
3. **Three (call)s that change text on a terminal:** the `alder check` count line, the `alder
   test` failure recap, and `alder explain` wrapping. Dropping any of them removes one test and
   one paragraph of the book; nothing else depends on them.
4. **`alder help` as a command list on every output** (the one change agents also see). The
   alternative is to keep the `|` line when piped and print the list on a terminal only.
