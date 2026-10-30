# CLI output UX: implementation plan (#47)

Spec: `docs/superpowers/specs/2026-10-29-cli-output-ux-design.md` (binding). Test first in every
task; one commit per task.

## Task 0: the JSON pin

`cmd/alder/style_cli_test.go`: `TestCLI_JSONIgnoresStyle` runs `check --json`, `routes --json`,
`help --json`, `eval --json`, `new page --dry-run --json`, `test --json`,
`deploy local --dry-run --json` with `ALDER_COLOR=never` and `=always`: equal output (ms and
random ids masked), no `\x1b`. Green before and after every later task.

## Task 1: `internal/style`

Files: `style.go`, `width_unix.go`, `width_other.go`, `style_test.go`.
Tests: `TestFor_NonTerminalsPlain`, `TestDecide` (table over isTTY, NO_COLOR, TERM,
ALDER_COLOR), `TestPlain_Identity`, `TestSGR`, `TestMark`, `TestColumns_RuneWidths`,
`TestWrap`, `TestLines_PartialWrites`, `TestStrip`. API as the spec, plus `ForEnv(w, getenv)`
so the deploy commands (which read no process environment) can use it.

## Task 2: diagnostics

`diag.RenderTextStyled(d, lines, st)`; `RenderText` is the Plain case. Test
`TestRenderTextStyled_StripEqualsPlain` (fix, NoSource, no code) and
`TestRenderTextStyled_Colours`. All `cmd/alder`, `buildkit`, `generate/apply.go` callers pass a
style (`--json` and non-terminal: Plain). `alder check` count line (`TestCheck_CountLineOnTerminalOnly`).

## Task 3: `alder test`

`resultCounter` gets a `style.Lines` filter on the report writer; recap on a terminal after a
failing summary. Tests `TestTest_StyledReport`, `TestTest_NoRecapWhenAllPass`, unit tests for
the line styler.

## Task 4: `alder help` / usage

`usageTo` prints the command list from `commandTree`. Tests `TestHelp_ListsEveryCommand`,
`TestCLI_BadFlag` unchanged.

## Task 5: `alder run`, `alder explain`

`logLine` styler (`TestRun_LogLineStyling`), run's own lines; explain wraps on a terminal
(`TestExplain_WrapsOnTerminalOnly`).

## Task 6: colour only

Deploy gate `Mark` (`TestDeployGate_MarksStyled`), headings, status, routes header, generator
verbs, migrate, fmt --check, scratch, eval type, `alder <cmd>:` error prefix.

## Task 7: book and verification

`book/11-tooling.md` (Commands block = `alder help` output, styling paragraph, check count);
`python3 book/site/build.py`; `go vet ./...`, `go test ./...`, `cd experiments && python3 -m
unittest`; rebase on origin/main; PR.
