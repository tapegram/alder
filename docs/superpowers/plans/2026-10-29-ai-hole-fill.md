# Plan: fill a typed hole with a model (#46)

Spec: `docs/superpowers/specs/2026-10-29-ai-hole-fill-design.md` (binding). Test-first, one commit
per task. No test calls a real model: the command backend (a shell script or a Go func) and an
`httptest` server stand in.

## Task 1: `generate.FindHole` and `generate.LineEdits`

- `internal/generate/fill.go`: factor the hole selection out of `Fill` into exported
  `FindHole(p, address, hole) (*Hole, error)` (same refusals); `Fill` calls it.
- `internal/generate/diff.go`: `LineEdit{Start, End int; Text string}` (0-based lines, End
  exclusive) and `LineEdits(before, after string) []LineEdit`, one per run of changed lines of
  `diffLines`.
- Test `internal/generate/diff_test.go`: `TestLineEdits_HunksAndImport` (an import line and a hole
  line are two edits; applying them to before gives after; equal texts give none).

## Task 2: `internal/fill` backends

- `fill.go`: `Message{Role, Content}`, `Backend` interface (`Complete(ctx, system, messages)`),
  `BackendFunc`, `Transcript(system, messages)` (the command backend's stdin and `--prompt`'s
  output), `FromEnv(getenv) (Backend, model string)`, `NoBackend` message.
- `command.go`: `Command{Cmd}` runs `sh -c`, prompt on stdin, stdout is the answer, non-zero exit
  fails with its stderr.
- `anthropic.go`: `Anthropic{Base, Key, Model, Reference, Client, Log}` POSTs `/v1/messages`
  (headers `x-api-key`, `anthropic-version`; `max_tokens 4096`; system blocks with
  `cache_control`; reference block first when set); `stop_reason` refusal / max_tokens fail with
  `NoRetry`; non-2xx is an error carrying the API's message; no key is `NoBackend`. Log, when set,
  gets the request body and the answer, never headers.
- Tests `command_test.go`, `anthropic_test.go`: `TestCommandBackend_StdinIsPromptStdoutIsExpression`,
  `TestCommandBackend_NonZeroExitFails`, `TestAnthropic_RequestShape`, `TestAnthropic_NoKey`,
  `TestAnthropic_HTTPError`, `TestAnthropic_RefusalNoRetry`, `TestAnthropic_MaxTokensNoRetry`.

## Task 3: the prompt

- `card.md` (embedded system prompt, ~150 lines, idioms from chapters 5, 6, 10).
- `prompt.go`: `Prompt(p, h) string` (address and type, doc comment above the declaration, the
  module in a fence, `Names in scope` fits-first capped at 200, stdlib signature files, closing
  line); `RetryTurn(errors)`; `StripFence`; a local scope walk (binders at the hole, keys into
  `check.Result.Binds`) since `internal/lsp`'s `scopeAt` cannot be imported here.
- Tests `prompt_test.go`: `TestPrompt_Contents`, `TestPrompt_TestHoleIncludesTest`,
  `TestPrompt_RetryCarriesErrors`, `TestStripFence`.

## Task 4: `fill.Run`

- `run.go`: `Run(ctx, b, dir, address, hole, report) (*Result, error)`: Load → FindHole → Prompt
  → up to 3 × (Complete under 120 s → StripFence → `generate.Fill` → `Apply` dry run); a
  `generate.Fill` refusal or a result with errors is retried with the errors; backend error,
  timeout, or the third refusal is `*Failure{Expression, Errors, Why, Attempts}`.
- Tests `run_test.go` on the scaffold-like fixture app built on disk:
  `TestRun_FirstAttemptApplies`, `TestRun_RetriesWithErrors`, `TestRun_GivesUpAfterThree`,
  `TestRun_NotOneExpressionRetries`, `TestRun_AddsImport`, `TestRun_LeavesHoles`,
  `TestRun_Timeout`.

## Task 5: CLI `alder fill --ai | --prompt`

- `cmd/alder/fill.go`: flags `--ai`, `--prompt`; usage errors (`-` with either, `--ai` with
  `--decl`/`--list`); `--ai` runs `fill.Run` with `fill.FromEnv`, prints `attempt N of 3: …` on
  stderr, applies the edits with `generate.Apply` (honours `--dry-run`), `--json` adds
  `expression`, `attempts`, `model`; failure prints the errors and the last expression, exit 1.
  `--prompt` prints `Transcript` of the first attempt.
- `cmd/alder/helpjson.go` + `testdata/help.json`: the two flags.
- Tests `cmd/alder/fill_ai_test.go`: `TestFill_AI`, `TestFill_AIDryRun`,
  `TestFill_AIRefusedWritesNothing`, `TestFill_AIJSON`, `TestFill_Prompt`,
  `TestFill_AIUsageErrors`; `TestHelpJSON_Golden` updated.

## Task 6: server → client requests and capabilities

- `internal/lsp/server.go`: `request(method, params)`, pending map keyed by id, filled by
  `dispatch`'s response case; `initializeParams.Capabilities` (`workspace.applyEdit`,
  `window.workDoneProgress`) recorded; `Options.Fill` (backend) and `Options.FillModel`.
- `client_test.go`: a `requests` channel, `answer(id, result)`, `startCaps`.
- Test `TestServer_RequestRoutesResponse`.

## Task 7: the code action and `alder.fill`

- `internal/lsp/actions.go`: the quickfix `Fill ?h with AI (T)` with the hole's W0002, before the
  generators, only with `workspace.applyEdit`; `executeCommandReq` routes `alder.fill`;
  `protocol.go` lists it.
- `internal/lsp/fill.go`: one fill at a time; goroutine under `s.bg`; Materialise the overlay;
  progress begin/report/end; `workspace/applyEdit` with one TextEdit per `LineEdits` hunk at the
  version of the document when the fill started; refused edit, failure, no backend → warning.
- `cmd/alder/lsp.go`: `fill.FromEnv` into `Options.Fill`, the wire log as the backend's log.
- Tests `internal/lsp/fill_test.go`: `TestCodeAction_FillHole`,
  `TestCodeAction_FillHoleWithoutApplyEdit`, `TestCodeAction_FillHoleStaleLine`,
  `TestFill_AppliesVersionedEdit`, `TestFill_RetryThenApply`, `TestFill_GivesUpShowsMessage`,
  `TestFill_ClientRefusesEdit`, `TestFill_OneAtATime`, `TestFill_NoBackendMessage`,
  `TestFill_UsesUnsavedBuffer`, `TestFill_NoProgressCapability`.

## Task 8: book

- `11-tooling.md` (fill rows/usage, "Generators in the editor", "Holes"), `13-compiler.md` §10
  "Filling a hole with a model", `15-generators.md` one paragraph; `python3 book/site/build.py`;
  `cd experiments && python3 -m unittest`.

## Task 9: alder.nvim (separate repo, branch `issue-46-ai-fill`)

- `lua/alder/apply.lua`: after the edit, when `plan.holes` is not empty, open the first hole's
  file and put the cursor on it; notice `alder: applied <gen>: <file> (N holes)`.
- `tests/apply_spec.lua`: `apply: the cursor lands on the first hole of the plan`,
  `apply: a plan without holes leaves the cursor`. Clean nvim temp dirs after the run.
- `README.md`: one paragraph (the action, the variables, the statusline progress).

## Finish

`go vet ./...`, `go test ./...` (10-minute timeout), rebase on origin/main, push, PRs in both
repos, the alder.nvim PR linked from the alder PR.
