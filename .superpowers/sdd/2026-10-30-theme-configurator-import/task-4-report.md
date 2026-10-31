# Task 4 report: `alder theme import`

Built: `generate.ImportTheme` + `ThemeImport` (internal/generate/theme_import.go), `Result.Import` and its text rendering (apply.go), `theme import` Spec (specs.go; reads file or stdin, 256 KiB cap), `cmd/alder/theme.go` (`runThemeTo`, `themeUsage`), `--print` branch and `themeImportExtra` JSON (generate.go; `genJSON.Skipped` is now `any`, shared by regen admin's list and the import's count), `theme` in main.go, help.go, helpjson.go commandTree, testdata/help.json, book/11-tooling.md Commands block.
Choices: dropped fields print on their own line (`light.primary` form) so the summary line ends with `; replaced the theme that was there)`; `--print` writes unplaced block to stderr; invalid theme becomes a Refusal (exit 1); a result with Import is returned alongside write refusals so --json keeps theme.
TDD: RED = compile failure (runThemeTo undefined), GREEN = `go test ./cmd/alder/ -run 'ThemeImport|HelpJSON'` ok.
Pins updated: TestSpecs_AllIDs, TestSpecs_AgentFlags, lsp TestGenerators_HereFirst (19 -> 20), help.json golden, book Commands block.
Full: `go test ./cmd/alder/ ./internal/generate/ ./internal/lsp/` ok; `go vet` clean.
Not done: book/site regeneration (later task per spec).
