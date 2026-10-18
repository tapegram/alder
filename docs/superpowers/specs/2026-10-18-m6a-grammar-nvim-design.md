# M6a — tree-sitter grammar and alder.nvim v1

Date: 2026-10-18. Status: approved in conversation; awaiting file review.

M6 "editor support" is three subsystems. This spec covers the first,
**M6a**: a tree-sitter grammar for Alder living in this repo, and a thin
Neovim plugin, `tapegram/alder.nvim`, that gives highlighting,
indentation, folds, `alder check` diagnostics on save, and the code
generators (`alder new/edit/remove/regen/fill`) inside the editor. The
other two are sketched in the last section and get their own specs:
**M6b** `alder lsp`, **M6c** test runners and `eval`/`repl` in the plugin.

## Goals

- Open any `.alder` file in Neovim and see it highlighted and indented
  correctly, with folds and textobjects, with no language server running.
- On save, `alder check` diagnostics appear in the buffer (and in the
  quickfix list for files that are not open), with the compiler's fixes
  applicable from the editor.
- Run every generator from Neovim with completion, a preview of the diff
  before it is applied, and `fill` on the hole under the cursor.
- The grammar is a second, independent description of the syntax that
  fails CI whenever the Go parser drifts from it.

## Non-goals (v1)

- Diagnostics while typing, hover, go-to-definition, completion of names
  or alder-ui elements — M6b.
- `alder test`, `--mutate`, `eval`, `repl` in the editor — M6c.
- Parsing the Alder inside string interpolations `"{…}"` as code (ledgered).
- Docs-site highlighting, upstream nvim-treesitter listing, GitHub
  linguist, Helix/Zed configs (ledgered).

## Facts the design rests on

Found 2026-10-18, before designing:

- The parser (`internal/syntax`) is hand-written recursive descent with
  recovery at declaration boundaries. Newlines are tokens: statements in a
  block separate on newlines, declarations start at column 1, and a line
  that begins with a binary operator continues the previous line
  (`parser_expr.go` `atOpAfterNL`; `<` continues only when followed by
  whitespace, `ltIsOperator`). Lexing is contextual: HTML text runs, raw
  text in `<script>`/`<style>`, `-` legal in tag and attribute names,
  keywords legal as tag names, a string literal including its `{…}`
  interpolations is one token (`lexer.go` `scanString`/`scanInterp`:
  nested `"` inside `{…}`, backslash escapes, no newlines).
- Comments (`-- …`) are discarded by the lexer and appear nowhere in the
  AST.
- The EBNF appendix (`book/A-grammar.md`) is **not tested** against the
  parser (`book/appendix_test.go` covers only the manifest appendix) and
  is stale (no `deploy`, `silo`, `choose`, `via`, `@partition`). It is
  not a source of truth for the grammar.
- Corpus available for a drift test: 1,242 `.alder` files under
  `examples/`, `stdlib/`, `book/` (snippets extracted by
  `internal/snippets`, as the book verifier does) and `testdata/`.
- `alder check --json` prints JSONL: `{code, severity, file, line, col,
  endLine, endCol, message, fix?: {line, col, endLine, endCol, replace}}`;
  columns are 1-based **bytes**, ends exclusive; `file` is app-relative.
  Exit 0 clean, 1 errors, 2 usage/IO/internal.
- Generators (`cmd/alder/generate.go`) share `--json --dry-run` and print
  `{files: [{path, action}], diff, diagnostics, holes, migration?,
  written, error?, …}`; `alder fill --list --json` prints
  `{holes: [{module, decl, arm?, name, type, file, line, col}]}`;
  `alder fill <Module>.<decl>[.<Arm>] - [--hole ?name]` reads one
  expression from stdin; `--decl` replaces a whole body.
- nvim-treesitter **main** (the user's branch; master is frozen) requires
  the `tree-sitter` CLI ≥ 0.26.1 for every user ("installed via your
  package manager, not npm") and compiles parsers with it. A custom parser
  is registered in a `User TSUpdate` autocommand as
  `require('nvim-treesitter.parsers').alder = { install_info = { url,
  revision?, location?, queries?, path? } }`; `location` names a
  subdirectory of a monorepo, `queries` a directory of queries to install
  from the same repo, `path` a local checkout.
- No editor support of any kind exists today; the docs site emits
  `<code class="language-alder">` with no highlighter.

## Decisions

1. **Order**: grammar + thin plugin with check-on-save diagnostics and
   generators first (M6a), then the LSP (M6b), then test runners and
   eval/repl (M6c). The user wants highlighting and diagnostics in daily
   use first; the LSP is the big piece and replaces the plugin's CLI
   diagnostics when it lands.
2. **Layout**: the grammar lives in this repo at `tree-sitter-alder/`
   with its drift test in `go test`; the plugin is its own public repo
   `tapegram/alder.nvim` (Apache-2.0, like alder-ui). The language changes
   weekly, so grammar and parser change in one PR; the plugin is thin and
   changes rarely.
3. **Full-fidelity grammar with an external scanner** (approach A), not a
   loose highlight-only grammar: newlines, HTML text, raw text and strings
   are scanned by `scanner.c` porting the Go lexer's rules, so indentation,
   folds, textobjects and injections are right, and the drift test is a
   real gate.
4. **Queries live with the grammar** (`tree-sitter-alder/queries/neovim/`)
   and are installed by nvim-treesitter through `install_info.queries`;
   the plugin ships none. A grammar change and its query change are one
   PR.
5. **The plugin never rewords the compiler.** Diagnostics, refusals and
   generator output are shown verbatim.
6. **`alder help --json`** is added to the CLI so the plugin's command
   completion and pickers are generated from the binary, not maintained by
   hand.

## 1. Grammar (`tree-sitter-alder/`)

### Layout

```
tree-sitter-alder/
  grammar.js            # the grammar
  src/scanner.c         # external scanner (newline, html_text, raw_text, string)
  src/parser.c          # GENERATED, committed
  src/grammar.json      # GENERATED, committed
  src/node-types.json   # GENERATED, committed
  tree-sitter.json      # metadata (file-types: alder)
  queries/highlights.scm           # minimal, for the tree-sitter CLI
  queries/neovim/{highlights,locals,indents,folds,injections,textobjects}.scm
  test/corpus/*.txt     # tree-sitter corpus tests
  drift_test.go         # Go test: corpus parses clean + declaration drift
  README.md
```

The `tree-sitter` CLI (`brew install tree-sitter-cli`, ≥ 0.26.1) is the
one tool, for developers and users alike. A wrapper
`scripts/ts.sh generate|test|parse` runs it from the right directory. No
npm, no `package.json`, no Node binding.

### Node shape

Mirrors the Go AST, not the EBNF. Names are what the queries capture;
fields (`name`, `type`, `body`, `tag`, `params`, `value`, …) wherever a
query needs to pick a child.

- `source_file` → `module_decl` `import_decl*` declarations.
- Declarations: `value_decl` (child `signature` with `name` + `type`, and
  `binding` with `name`, `params`, `body`), `type_decl` (`union_type`
  with `constructor`s, `record_type`, `brand_type` with optional `where`),
  `page_decl` (`path` string, `params` record, `requires` clause with
  session keys and `role` alternatives), `msg_decl` (fields, optional
  `form`), `form_decl`, `table_decl` (fields with `attribute`s such as
  `@id`, `@ref T`, `@owner`, `@partition k`, `@renamed "x"`, delete
  rules), `capability_decl`, `fake_decl` (nested tables and entries),
  `session_decl` (fields incl. `via`), `login_decl`, `test_decl`,
  `deploy_decl` (environments, targets and their records), `silo_decl`,
  `choose_decl`.
- Types: `type_arrow`, `type_app`, `type_name` (qualified), `type_var`,
  `record_type` with `field`s, parenthesised.
- Expressions by the compiler's precedence: `pipe_expr` > `or_expr` >
  `default_expr` (`??`) > `and_expr` > `comparison_expr` > `additive_expr`
  > `multiplicative_expr` > `application` > primary. Primary:
  `identifier`, `qualified_name`, `constructor`, literals (`integer`,
  `float`, `string`, `boolean`, `unit`), `list`, `record`,
  `record_update`, `field_access`, `lambda`, `if_expr`, `match_expr` with
  `arm`s and patterns (`wildcard`, literal, `identifier`,
  `constructor_pattern`, `record_pattern`, `list_pattern` with `..rest`),
  `block` with `let_stmt` and `bind_stmt` (`<-`), `hole` (`?name`),
  parenthesised.
- HTML: `element` (open tag `tag`, `attribute*`, children, close tag),
  `self_closing_element`, `raw_element` (`script`/`style` with
  `raw_text`), `attribute` (`name` + `string` or `{expr}` value, or bare),
  `text`, `interpolation` (`{expr}` child), `html_for`, `html_if`,
  `html_match`. Tags: lowercase and hyphenated names (custom elements),
  `Component`, `Module.Component`; keywords are legal as tag and attribute
  names (`word` is set so `<table>` is a tag).
- `comment` nodes in `extras` — the one thing the grammar has that the
  compiler does not.

### External scanner

Four tokens, each a direct port of a Go lexer rule, with a comment naming
the Go function it mirrors:

1. `_newline`: emitted where the grammar needs a separator (between
   declarations, between statements in a block, before a continuation
   check); **not** emitted when the next non-blank line begins with a
   binary operator (`|> || ?? && == != < <= > >= + - ++ * / %`), with the
   rule that `<` counts as an operator only when followed by whitespace.
   Blank lines and comment-only lines collapse into one. The grammar lists
   `_newline` explicitly where it may occur; everywhere else newlines are
   whitespace.
2. `html_text`: a text run inside an element up to `<`, `{` or end of
   input; may span lines. Whitespace rules are the compiler's business.
3. `raw_text`: inside `<script>`/`<style>`, everything up to `</script`
   or `</style` case-insensitively.
4. `string`: one token including its `{…}` interpolations, ported from
   `scanString`/`scanInterp` (nested strings inside interpolations,
   backslash escapes, terminates at a newline — an unterminated string is
   an error island, not a runaway). Interpolations are **not** parsed as
   Alder in v1; they are highlighted as string. Ledgered: hand the parser
   control inside `{…}` (tree-sitter-javascript's template-string
   technique).

### Acceptance policy

The grammar accepts everything the compiler accepts and may accept a
little more. Diagnostics are the compiler's job; the grammar's job is the
right tree on valid code and small `ERROR` islands on invalid code. There
is no negative corpus.

## 2. Tests, tooling, CI

Three layers, all under `go test ./...` and one CI job.

1. **Corpus tests** (`tree-sitter test`): hand-written cases in
   `test/corpus/*.txt`, one file per construct family, seeded from
   chapter 3's examples. These assert tree shape.
2. **Drift test** (`tree-sitter-alder/drift_test.go`), the gate:
   - every `.alder` file under `examples/`, `stdlib/`, `testdata/corpus`
     (the parse-clean cases) and every book snippet that the verifier
     treats as a module (`internal/snippets`) parses with **zero**
     `ERROR`/`MISSING` nodes (`tree-sitter parse -q`, batched);
   - for each file, the list of top-level declaration **kinds and names**
     read from the tree-sitter tree equals the list `syntax.ParseFile`
     yields. A grammar that "parses" by swallowing a declaration into its
     neighbour fails here.
   - The test **fails** when the `tree-sitter` CLI is absent, unless
     `ALDER_SKIP_TS=1`; a silent skip is how drift tests die.
3. **Regeneration check**: `tree-sitter generate` then `git diff --exit-code
   tree-sitter-alder/src` — a stale committed parser fails CI.

CI: a `grammar` job (about a minute) in the existing workflow runs the
corpus tests and the regeneration check. The drift test is an ordinary Go
test, so every job that runs `go test` installs `tree-sitter-cli` (one
package-manager line) — no job sets `ALDER_SKIP_TS`. A PR that changes
the syntax must change the grammar, its queries and its corpus in the
same PR. `.github/shard-weights` is untouched (the drift test is seconds).

Versioning: the grammar has no version of its own; it is whatever the
alder tag carries. The plugin registers `url = tapegram/alder`,
`location = tree-sitter-alder`, default branch `main`; users on a release
binary may set `revision = "v0.x.y"` (documented). `alder help --json`
exists from the release that ships the grammar; the plugin README records
that minimum.

## 3. Queries (`tree-sitter-alder/queries/neovim/`)

nvim-treesitter capture conventions throughout.

- **highlights.scm**: `@keyword` (+ `.conditional`, `.function`,
  `.import`, `.type`, `.return`-style variants as they fit), `@function`
  for value-declaration names, `@variable`, `@variable.parameter` (via
  locals), `@constructor`, `@type`, `@module`, `@property` for record
  fields and columns, `@string`, `@string.special.url` for page paths,
  `@number`, `@boolean`, `@operator`, `@punctuation.bracket`,
  `@punctuation.delimiter`, `@attribute` for `@id/@ref/@owner/…` with
  their arguments as `@type`, `@comment` + `@spell`, holes as
  `@variable.builtin` so they stand out. HTML: `@tag` (custom elements and
  components alike), `@tag.attribute`, `@tag.delimiter`, `@markup` text
  with `@spell`; `{…}` children are plain Alder.
- **locals.scm**: scopes = block, lambda, arm, `for … in`, declaration
  bodies; definitions = `let`/bind names, lambda and function parameters,
  pattern bindings; references = identifiers.
- **indents.scm**: `@indent.begin` on `{ [ (`, open tags, `match`
  bodies, `then`/`else`, lambda bodies on the next line; `@indent.end` /
  `@indent.dedent` on closers and close tags; `@indent.branch` for `else`
  and union `|`; operator-led continuation lines indent one level.
- **folds.scm**: declarations, blocks, `match`, elements with children,
  `table`/`capability`/`fake`/`deploy` bodies.
- **injections.scm**: `raw_text` of `<script>` → `javascript`, of
  `<style>` → `css`; comments → `comment`.
- **textobjects.scm**: `@function.outer/inner` = a value declaration
  including its signature line; `@class.outer` = type, table, capability,
  page, msg, form declarations; `@parameter.outer/inner`;
  `@block.outer/inner` for match arms.

`queries/highlights.scm` at the grammar root is the minimal CLI set
(standard tree-sitter captures) for `tree-sitter highlight` and the
docs-site follow-up.

Visual check at the end: the three examples and chapter 3 open in Neovim;
a screenshot goes in the plugin README.

## 4. alder.nvim v1 (`tapegram/alder.nvim`)

Requires Neovim ≥ 0.11, nvim-treesitter main, the `tree-sitter` CLI, and
`alder` on PATH. No AstroNvim-specific code; the README shows the lazy
spec and an AstroNvim `lua/plugins/alder.lua` snippet, then
`:TSInstall alder`.

```
alder.nvim/
  plugin/alder.lua          # filetype, parser registration, commands
  ftplugin/alder.lua        # commentstring, shiftwidth=2 expandtab, ts folds/indent
  lua/alder/init.lua        # setup{}
  lua/alder/root.lua        # app root: nearest ancestor containing app/
  lua/alder/cli.lua         # run `alder … --json` async from the root; JSON/JSONL parsing
  lua/alder/check.lua       # diagnostics on save, :AlderCheck, :AlderFix, :AlderExplain
  lua/alder/commands.lua    # :Alder passthrough + completion from `alder help --json`
  lua/alder/generate.lua    # preview-then-apply, :AlderNew, after-write reload
  lua/alder/fill.lua        # :AlderFill
  lua/alder/health.lua      # :checkhealth alder
  tests/                    # headless nvim tests (section 5)
```

`setup{ cmd = 'alder', check_on_save = true, preview = true,
path = nil }` — `path` points parser installation at a local checkout
(`~/projects/alder/tree-sitter-alder`) for grammar development:
edit → `:TSUpdate alder`.

### Filetype and parser

`vim.filetype.add{ extension = { alder = 'alder' } }`. On `User
TSUpdate`: `parsers.alder = { install_info = { url =
'https://github.com/tapegram/alder', location = 'tree-sitter-alder',
queries = 'tree-sitter-alder/queries/neovim', revision = opts.revision,
path = opts.path } }`.

### Diagnostics on save

`BufWritePost *.alder` (and `:AlderCheck`): find the app root, run
`alder check --json <root>` with `vim.system` (one run at a time; a new
save cancels the previous), parse JSONL, publish with
`vim.diagnostic.set` in namespace `alder` for every loaded buffer whose
path matches `file`: 1-based byte columns → 0-based, `endLine/endCol`
used, severity mapped (`error`/`warning`), `code` shown, `source =
'alder'`, the `fix` kept in `user_data`. Diagnostics for files not open go
to the quickfix list; `:AlderCheck` opens it when any error is elsewhere.
Exit 2 or no JSON: stderr shown (see error handling).

`:AlderFix` applies the fix of the diagnostic under the cursor with
`nvim_buf_set_text` (byte columns, exclusive end); `:AlderExplain` runs
`alder explain <code>` in a float.

### Code generation

- **`alder help --json`** (new in the CLI): the command tree — for every
  command and subcommand its name, positional arguments, flags (name,
  takes-value, one-line description) and one-line description — derived
  from the same tables the text help prints, with a test pinning the two
  equal. The plugin runs it once per session (cached per `alder version`).
- **`:Alder <args…>`**: passthrough with command-line completion of
  subcommands and flags from that tree. Runs from the app root with
  `--json`. Output rendered in a scratch split: files
  (created/edited/removed), the unified diff, migration notes, holes;
  `diagnostics` go to the `alder` namespace; `error` is a notification
  with the CLI's sentence.
- **Preview-then-apply** (default on): a generator runs with `--dry-run`
  first; the diff opens in a split; `<CR>` applies (re-runs without
  `--dry-run`), `q` discards. `setup{ preview = false }` applies
  directly.
- **`:AlderNew`**: `vim.ui.select` over the kinds from the tree, then
  `vim.ui.input` for the name and each value-taking flag the user picks
  (`--list T`, `--show T`, `--from T`, `--into T`, `--requires k`, …).
  Values are typed in v1; the CLI refuses an unknown table or page with
  its own message. Ends in the same preview. Ledgered: an `alder list
  --json` (tables, pages, forms, msgs, capabilities) so pickers can offer
  the program's names — no such listing exists today (`alder dump` prints
  tokens/AST/IR only).
- **`:AlderFill`**: `alder fill --list --json` → the hole whose
  `file/line/col` contains the cursor (else "no hole under the cursor";
  with one hole in the file, that one) → a scratch split headed
  `-- ?name : <type>` and `-- Module.decl[.Arm]` → `<CR>` or `:w` pipes
  the body below the header to `alder fill <addr> --hole ?name -`; a
  refusal becomes a diagnostic in the scratch buffer (the expression stays
  for editing); success closes it, reloads the file, re-checks.
  `:AlderFill!` uses `--decl` for a whole declaration body.
- **After any write**: `checktime` reloads edited buffers; the first
  created file opens; `alder check` re-runs.
- **Safety rule**: no generator runs while any `.alder` buffer in the app
  has unsaved changes — the command refuses with "save all buffers first
  (:wall)". The CLI edits files on disk and must not race the editor.

### Health

`:checkhealth alder`: `alder` found + version; `tree-sitter` CLI found +
version; nvim-treesitter main present; parser `alder` installed; queries
installed.

## 5. Testing, error handling, release

### Plugin tests

Headless Neovim, no framework: `nvim --headless -u tests/minimal.lua -l
tests/run.lua` with plain `assert`s. The runner installs the parser from
`install_info.path` = a checkout of the alder repo (CI checks out both
repos; locally `~/projects/alder`). Cases:

- filetype detection;
- the parser loads and all six query files compile against it
  (`vim.treesitter.query.get` fails on an unknown node type — the
  grammar⇄queries drift check);
- highlight captures at a few known positions in an example file;
- `alder check --json` → diagnostics mapping, and a fix round-trip through
  `:AlderFix`;
- `:Alder new page …` preview and apply on a scratch app made by `alder
  new`, and the unsaved-buffer refusal;
- `:AlderFill` success and refusal;
- completion entries from `alder help --json`.

GitHub Actions in the plugin repo: Neovim stable, `tree-sitter-cli`, the
alder binary built from the checked-out alder repo (or a release).

### Error handling

One rule: the plugin never rewords the compiler.

- `alder` not on PATH → one notification with the install line.
- Non-zero exit with no JSON → stderr in a notification.
- JSON `error` → shown verbatim.
- Exit 2 (usage/internal) → stderr in a split with "this may be a
  compiler bug; `alder version` and the command are in the buffer".
- No app root → "no `app/` directory above this file".
- Modified buffers before a generator → the `:wall` refusal.

### Release

- alder: the grammar, its tests, `alder help --json`, and chapter 1
  "Installing" gains "Editor support" (brew `tree-sitter-cli`, the lazy
  spec, `:TSInstall alder`); chapter 11 "Tooling" links it; release notes
  list editor support. Ships with the next tag.
- alder.nvim: `v0.1.0` on merge; README states the minimum alder version.

### Ledger (follow-ups)

- Interpolations parsed as Alder inside strings.
- Docs-site highlighting via `tree-sitter highlight` in `book/site/build.py`.
- Upstream listing in nvim-treesitter; GitHub linguist.
- Helix and Zed configurations (tree-sitter + LSP; cheap after M6b).
- The EBNF appendix: test it against the parser, or drop it in favour of
  the grammar.
- `alder list --json` so `:AlderNew` can offer the program's tables and
  pages instead of typed names.
- The plugin's CLI diagnostics path is replaced by the LSP in M6b.

## M6b / M6c scope sketch (own specs later)

**M6b — `alder lsp`** (in the alder binary, stdlib-only JSON-RPC over
stdio): in-memory overlays (parse buffers with `syntax.ParseFile`, build
the program with `names.Build` + `Resolve` + `check.ProgramWith`, no disk
read for open files — the first compiler change, plus a policy for
checking modules when another module has a parse error, since today any
parse error disables checking app-wide); byte→UTF-16 column conversion;
diagnostics on change (debounced; `alder check` is ~0.3 s on the blog);
hover from `Result.Types`/`Binds` and signatures; go-to-definition from
`Ref` (locals need a binding-site span the names pass does not record
yet); completion of names, record fields, messages, pages, and alder-ui
elements/attributes/values from the merged Custom Elements Manifest with
the manifest's descriptions as docs; code actions: `Fix` → TextEdit,
`fill` for the hole under the cursor, generators as `workspace/
executeCommand` so other editors get them; inlay hint with a hole's
expected type; semantic tokens optional. A doc-comment convention for
stdlib hover docs (the lexer discards comments today). The plugin
switches from CLI-on-save to the LSP and drops `check.lua`.

**M6c — alder.nvim v2**: `alder test --json` and `--mutate --json`
runners (results in quickfix; survivors as diagnostics at `file/line/col`;
`-run` from the test under the cursor), `alder eval` of the expression
under the cursor, `alder repl` in a toggleterm; possibly a neotest
adapter.
