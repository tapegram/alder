# Scratch Files: Editor Features and fmt (alder#31)

**Goal:** inside a scratch file the language server answers hover, completion, go to definition and code actions (spec 2026-10-24-alder-scratch-design.md §5.4), and inlay hints show each watch's result for editors without alder.nvim; `alder fmt` formats scratch files, both when it walks a project and when one is named.

**Architecture:**

- **Per-file analysis** (`internal/lsp/scratchview.go`). For an open scratch document the fast handlers build a *view*: `scratch.ParseFile` + `scratch.Check` of the document's current text (cached by text and by the app analysis it was built against), giving an `analysis` whose program holds the scratch module. A view remembers how its module's lines map to the file's (the lines `Check` inserted: `module`/implicit imports, or the `in` module's own lines; each watch's `itK = ` column shift). A request maps the cursor into module coordinates, runs the existing hover/definition/completion code on the view, and maps results back (a definition in the app stays an app location). A text that does not check falls back to the last view that did, under the same freshness rule as app files (Ruling R11), computed on the module text rebuilt from the current file.
- **Code actions**: the compiler fixes of the current text's diagnostics, positioned in the file (`SourceMap.Position`), as for app files. Generators are not offered in scratch files.
- **Inlay hints**: `textDocument/inlayHint` from the last `alder/scratch` result of the document, one hint at the end of each watch's last line, only while that watch's lines are unchanged. The server sends `workspace/inlayHint/refresh` after each phase when the client supports it.
- **fmt**: `scratch.Format(name, src)`: parse the file (`ParseFile`), format its module source (with a synthesized `module` line when it has none), turn each `itK = ` back into `> `, restore the `in` header, keep originally adjacent watches adjacent, and check the round trip (the output parses to the same watches and formats to the same module). `alder fmt` and LSP formatting use it for scratch files.

## Tasks (each test-first: write the test, see it fail, implement, see it pass, commit)

1. **`scratch.Format`** (`internal/scratch/format.go`, tests in `format_test.go`): spacing and operators inside a watch; a definition; an `in` header (with a leading comment); a long watch broken over lines; adjacent watches stay adjacent; a file with ParseFile errors returns diagnostics and no output; idempotence.
2. **`alder fmt`** (`cmd/alder/fmt.go`, new `cmd/alder/fmt_scratch_test.go`; the two old skip/refuse tests in `fmt_test.go` are removed): walking a project formats its scratch files (`--check` lists them); a scratch file named on the command line is formatted; a scratch file under `app/` is still a module. Kept clear of PR #85's hunks.
3. **LSP view + hover/definition** (`internal/lsp/scratchview.go`, `hover.go`, `definition.go`; tests in `internal/lsp/scratch_editor_test.go`): hover on a scratch definition, on a watch's expression, on an app value used in a watch (named mode and `in` mode); definition to a scratch-local definition and into an app module.
4. **Completion**: members of an app module (`Todo.`) and a local scratch definition inside a watch, while the line being typed does not parse.
5. **Code actions**: a fix on a scratch diagnostic appears with its edit in file coordinates (on a watch line, the shift applied).
6. **LSP formatting** of a scratch document through `scratch.Format`.
7. **Inlay hints**: capability advertised; a hint per watch from the last result; none for a watch edited since; refresh request sent when the client supports it.
8. **Book**: chapter 11 "Scratch files in the editor" (what now works), "Formatting" (scratch files), "`alder scratch`" (fmt no longer skips them).

Verification: `go vet ./...`; `go test ./internal/lsp ./internal/scratch ./internal/format`; `go test ./cmd/alder -run 'Fmt|Scratch|Lsp'`.
