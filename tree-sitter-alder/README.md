# tree-sitter-alder

Alder's tree-sitter grammar: a second, independent description of the syntax that the
compiler's parser (`internal/syntax`) is checked against. It drives highlighting, indentation,
folds and textobjects in editors ([alder.nvim](https://github.com/tapegram/alder.nvim)).

## Layout

- `grammar.js`: the grammar. It mirrors the Go parser, not `book/A-grammar.md`.
- `src/scanner.c`: the external scanner. It ports the Go lexer's context-sensitive rules
  (newlines and operator-led continuation lines, HTML text, `<script>`/`<style>` bodies,
  strings with `{…}` interpolations), each citing the Go function it mirrors.
- `src/parser.c`, `src/grammar.json`, `src/node-types.json`: generated and committed.
- `queries/neovim/`: the queries nvim-treesitter installs (`install_info.queries`).
- `queries/highlights.scm`: a minimal set for the `tree-sitter` CLI.
- `test/corpus/`: tree-shape tests. `test/highlight/`: CLI highlight tests.
- `drift_test.go`: under `go test ./...`. Every `.alder` file the Go parser accepts must parse
  with no `ERROR` or `MISSING` node and yield the same top-level declarations.

## Working on it

You need the `tree-sitter` CLI (`brew install tree-sitter-cli`); no Node, no npm.

```sh
scripts/ts.sh generate             # regenerate src/ (only with the pinned CLI, 0.27.1)
scripts/ts.sh test                 # corpus and highlight tests
scripts/ts.sh parse some/file.alder
go test ./tree-sitter-alder        # the drift test and the rest
```

A change to the language's syntax changes the grammar, its queries and its corpus in the
same pull request; the drift test fails until they agree. Without the CLI the tests fail;
`ALDER_SKIP_TS=1` skips them (no CI job sets it).

The grammar accepts everything the compiler accepts, and may accept a little more:
diagnostics are the compiler's job. Interpolations inside strings are not parsed as Alder yet.
