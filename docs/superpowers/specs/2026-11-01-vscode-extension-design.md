# VS Code extension: highlighting from the server, the alder.nvim UIs rebuilt on `vscode-languageclient`

Design for [tapegram/alder#144](https://github.com/tapegram/alder/issues/144). Everything marked
**(call)** is the author's decision. Inputs: the port assessment and the alder.nvim inventory
(notes, 2026-11-01), `alder.nvim`'s Lua and `tree-sitter-alder/queries/neovim/*.scm`,
`internal/lsp`. Related: #121 (the `al-*` completion and hover this extension gets for free),
#120 (alder-tui, the other non-nvim front end), the nvim modal decisions (alder.nvim#6).

## Intent

alder.nvim is a thin client: `alder lsp` does the work and ~3,800 lines of Lua add the editor
surface (grammar registration, a generator modal, scratch watches, pickers, quickfix lists). A
VS Code extension is the same client on `vscode-languageclient`, and the one thing the Go server
does not yet give an editor, highlighting, folding and structural selection, moves into the
server as LSP semantic tokens, `foldingRange` and `selectionRange`, derived from the same
conventions as the tree-sitter queries. Nothing of the grammar is compiled into the extension:
no WASM parser, no revision pin, and the next editor (Zed, Helix, the TUI's viewer) gets the
same tokens. A tiny TextMate grammar colours comments, strings and keywords until the first
tokens arrive.

The extension never rewords the compiler, as alder.nvim never does: what you see is what `alder`
printed.

## Goals

1. Opening a `.alder` file under an app root starts (or reuses) `alder lsp` for that root and
   gives diagnostics, hover, definition, references, completion, code actions, formatting, inlay
   hints and colour swatches through the stock client, and `Fill ?h with AI` end to end
   (progress, the Apply/Discard question, the versioned edit) with no extension code.
2. Highlighting, folding and expand-selection come from the server; a file of the blog example
   looks in VS Code as it does in Neovim: the same token classes on the same spans, by
   construction (a parity test against the tree-sitter captures).
3. The generator flow of the nvim modal, ported: pick a generator (the ones that apply at the
   cursor first), fill its form with a live diff beside it, review the plan, apply it to the
   editor's documents (never to disk, except a confirmed deletion), land on the first hole.
4. Routes, run, explain, tests and mutation testing, scratch files: each on the VS Code surface
   that fits (QuickPick, terminal, markdown preview, Test Explorer, inlay hints), with the same
   data the nvim plugin reads.
5. A `.vsix` anyone can install from a GitHub release, built and tested in CI against the real
   server; the binary checked at start as alder.nvim checks it, with one health command that
   says what is missing.

## Non-goals

- A tree-sitter WASM grammar in the extension, or textobjects beyond `selectionRange` (VS Code
  has no textobjects; expand/shrink selection is its equivalent).
- Embedded JavaScript and CSS highlighting inside `<script>` and `<style>` (nvim's
  `injections.scm`); a later TextMate-level follow-up, not part of the token legend.
- A CLI mode (`opts.lsp = false` in alder.nvim): the extension needs `alder lsp`. The CLI is
  still run for tests, mutation testing, `run`, and `routes --json` as a fallback.
- An `:Alder <args>` passthrough; VS Code has a terminal.
- The deprecated REPL.
- Durandal (see the last section), the Marketplace listing (a later milestone), a webview form
  (only if the native QuickPick form feels thin: see the generator section).
- Delta semantic tokens (see the server section), semantic token changes to alder.nvim's look
  (it keeps tree-sitter as its source).

## Changes: the server (`internal/lsp`)

### Capabilities and methods

`capabilities()` in `protocol.go` gains, and `TestInitialize_Capabilities` checks:

```go
"semanticTokensProvider": map[string]any{
    "legend": map[string]any{"tokenTypes": tokenTypes, "tokenModifiers": tokenModifiers},
    "full": true, "range": true},
"foldingRangeProvider":   true,
"selectionRangeProvider": true,
```

`fastMethods` in `server.go` gains `textDocument/semanticTokens/full`,
`textDocument/semanticTokens/range`, `textDocument/foldingRange` and
`textDocument/selectionRange`. All three read the document's *current* text (`s.text(rel)`), as
`documentColorReq` does, not an analysis: tokens must match what is on screen now, and the
analysis is 150 ms (`Debounce`) and a rebuild behind. Full tokens and no delta **(call)**: a
module is a few hundred lines and a token pass is a parse, so the whole answer costs about what
a hover costs; delta tokens need a result id per document and the previous stream kept, for a
saving VS Code does not need at this size. Range is the full list cut to the lines asked, so VS
Code can colour the viewport first on open.

New files: `tokens.go` (the legend, the producer, the two requests), `folding.go`,
`selection.go`, and their tests.

### One parse, two layers

`syntax.Tokens(file, src)` (parser.go) already returns the tokens the parser consumed, in
source order, the lexer driven by the parser so markup text is `Text` tokens and a string with
interpolations is one `String` token. It does not return the module. **(call)** Add
`syntax.ParseTokens(file, src) (*Module, []Token, []diag.Diagnostic)` (newParser takes the
trace pointer today; this is `Tokens` returning `parseModule()`'s module too) and leave `Tokens`
as it is. One parse gives both layers:

- **Lexical** (from the token stream): keywords (`Kind == Keyword`), operators (`Pipe`, `Arrow`,
  `LeftArrow`, `Equal`, `EqEq`, `NotEq`, `Lt`, `Le`, `Gt`, `Ge`, `Plus`, `Minus`, `Star`, `Slash`,
  `Percent`, `PlusPlus`, `AndAnd`, `OrOr`, `Question2`, `PipeGt`, `DotDot`), `Backslash`,
  strings, numbers, `Hole`, `At`. Brackets, commas, colons and dots get no token **(call)**: VS
  Code themes leave punctuation plain and bracket-pair colouring is the editor's own; the
  TextMate grammar scopes them for the themes that care.
- **Structural** (from the AST, `syntax.Children` in a walk of `Module.Decls`): what an
  identifier is by where it sits, the transcription of `highlights.scm`'s patterns. The walk
  produces a map from a token's start (`diag.Pos`) to a (type, modifiers) pair; the producer then
  walks the token stream once, and for each `LIdent`/`UIdent` takes the map's entry, else the
  default (`variable` for a lower-case identifier, `enumMember` for a capitalised one, as
  `(identifier) @variable` and `(constructor) @constructor` do). Both layers are keyed by
  position, so the token stream, not the AST, decides what is emitted and in what order.
- **Comments** come from `Module.Comments` (the parser attaches none to nodes; the lexer records
  them all, `Markup` ones included).

Two consequences of using the parser's own tokens: a tag name, attribute name or component
qualifier inside markup is a token the parser lexed in HTML mode (`SetHTMLTag`), so it is found
by position like any identifier; and text in markup is a `Text` token that produces nothing.

The AST keeps no spans for keywords or operators (`BinOp.Op` is a string, `If` holds three
expressions), which is why the lexical layer comes from the token stream and not the tree. The
contextual keywords the lexer does not reserve (`as`, `deploy`, `theme`, `silo`, `choose`,
`role`, `via`; `syntax.Keywords()` lists the reserved ones, `in` among them) are `LIdent`
tokens; the walk marks them `keyword` from the node that owns them: `Import.AliasSpan` (the
token before it), the first token of a `DeployDecl`, `ThemeDecl`, `SiloDecl`, `ChooseDecl`,
`PageDecl.RolesSpan`, `PageDecl.SiloSpan`, `Field.ViaSpan` and `Field.WireSpan` (`as`).

### The legend

Derived capture by capture from `queries/neovim/highlights.scm`. Standard LSP types wherever one
fits, since every client themes those; four custom types where none does, declared in the
legend and mapped to TextMate scopes by the extension (`contributes.semanticTokenScopes`) so
every theme colours them **(call)**. No custom modifiers. The types, in legend order:

| # | type | nvim capture(s) | emitted for |
|---|---|---|---|
| 0 | `namespace` | `@module` | `module` and `import` names, a qualifier (`Post.` of `Post.title`, `Layout.` of `<Layout.Page>`, `Db.` of `Db.all`), `type_name` qualifiers |
| 1 | `type` | `@type`, `@type.definition` | `TypeName.Name`, `TypeVar`, `BrandType.Base`, `TypeDecl.Name` (+`declaration`), a silo's table, a field attribute's type argument |
| 2 | `enumMember` | `@constructor`, `@constant.builtin` | a `CtorRef`, `PCtor`, `Ctor` (variant, +`declaration`), a deploy target kind, a role, the unit `()` |
| 3 | `function` | `@function`, `@function.call` | a value's signature and binding name (+`declaration`), a fake entry's name (+`declaration`), the function of an `App` when it is a `Var`, a qualified name's last segment |
| 4 | `method` | `@function.method.call` | the function of an `App` when it is an `Access` (`x.f y`): its field |
| 5 | `parameter` | `@variable.parameter` | `ValueDecl.Params`, `Lambda.Params`, `HtmlFor.Var` (+`declaration` at the binding site) |
| 6 | `variable` | `@variable` | every other identifier: a `Var`, a `let` name (+`declaration`), a `PVar`, a list pattern's rest |
| 7 | `property` | `@property`, `@variable.member` | `Field.Name`, `FieldInit.Name`, `PField.Name`, `DeployField.Name`, `Access.Field`, a session key, a silo or choose key, an attribute's field argument |
| 8 | `keyword` | every `@keyword.*` | the reserved words, the contextual ones above, `\` |
| 9 | `operator` | `@operator` | the operator tokens listed above, and `=` of an attribute |
| 10 | `string` | `@string`, `@string.special.url` | a `String` token (one token, interpolations included: the interpolated expressions' own tokens are emitted over it, see below), a page path |
| 11 | `number` | `@number`, `@number.float` | `Int`, `Float`, and the `-` of a negative literal |
| 12 | `comment` | `@comment` | each `Module.Comments` entry; `documentation` when the comment block ends on the line above a declaration (`syntax.DocOf`'s rule) |
| 13 | `decorator` | `@attribute` | `@` and the name of a field attribute |
| 14 | `tag` (custom) | `@tag` | an element's tag name, start and end tag; the last segment of a component (`Page` of `Layout.Page`); `al-*` elements |
| 15 | `attribute` (custom) | `@tag.attribute` | `HtmlAttr.Name` |
| 16 | `boolean` (custom) | `@boolean` | `True`, `False` |
| 17 | `hole` (custom) | `@variable.builtin` | a `Hole` token (`?h`) |

Modifiers, in legend order: `declaration` (bit 0), `documentation` (bit 1), `defaultLibrary`
(bit 2, reserved for phase 2 below, unused in phase 1). `_` (`@character.special`) gets no
token **(call)**: a one-byte placeholder a theme should not shout about. Keyword sub-classes
(`@keyword.import`, `.conditional`, `.repeat`, `.type`, `.function`) collapse into `keyword`
**(call)**: VS Code's semantic colour rules key on the type, the TextMate fallback scopes them
finely for the themes that distinguish `keyword.control` from `storage.type`, and most nvim
colourschemes link the sub-captures to `@keyword` anyway. The `@spell` captures and
`@punctuation.*` have no counterpart.

A string with interpolations: the `String` token covers `"a {x.y} b"` whole. The producer emits
the string as `string` pieces around each interpolation (the `StrLit.Parts` with an `Expr` give
the gaps by span) and the expression's tokens inside, so `x` is `variable` and `.y` is
`property` as in nvim, and no two tokens overlap (VS Code drops overlapping tokens).

Multi-line tokens are split per line: VS Code declares `multilineTokenSupport: false`. Only a
string can span lines today (`--` comments are one line); the split is done once in the encoder.

### Encoding and the wire

`tokens.go` keeps an intermediate `[]token{line, col, len, typ, mods}` (1-based, bytes, as the
compiler counts) sorted by position, and one encoder turns it into the LSP's relative five
integers per token, with each column passed through `s.enc.toWire(line, col)` (encoding.go)
and the length measured in the negotiated unit (`toWire(line, col+len) - toWire(line, col)`).
VS Code's client offers only `utf-16`, so this is the first feature that exercises the UTF-16
path on every request; a test covers a line with a non-ASCII string before a token. The range
request keeps the tokens whose line is within the range and re-encodes from the first.

### When the text does not parse

`ParseFile` "never panics and recovers at declaration boundaries": a module with an error in
one declaration still carries the others, and the token stream carries whatever the parser
consumed. The producer uses what it gets. When the parser recovered nothing useful for a region
(the tokens there are lexical only), that region is coloured lexically: keywords, strings,
numbers, operators keep their colour and identifiers fall to their defaults, which is what
tree-sitter's error recovery gives nvim too. **(call)** The server never answers `null` to hide a
half-typed line: a token answer for the current text, however partial, beats VS Code's shifted
copy of the previous one.

### Phase 2 (a later PR): what the analysis knows

Phase 1 is purely syntactic, so nvim and VS Code agree by construction and a token pass needs no
analysis. The server knows more once the names pass has run: `Var.Ref.Kind` says whether a bare
identifier is a top-level value (`RefValue`), a local (`RefLocal`), a standard-library value
(`RefStd`), a table (`RefTable`), a column (`RefColumn`), a page (`RefPage`) or a component
(`RefComponent`). Phase 2 upgrades tokens from `s.lastGood()` where `freshLines` holds for the
token's line (the same freshness rule hover uses, Ruling R11): `defaultLibrary` on `RefStd`
values and standard-library qualifiers (`a.prog.Std`), `parameter` instead of `variable` on a
reference to a parameter (`scopeAt`'s binders), `type` on a `RefTable` constructor, `property`
on a column; and sends `workspace/semanticTokens/refresh` after each rebuild that changed a
module's refs, so VS Code re-asks. It is scoped out of phase 1 so the extension ships on a
stable, nvim-identical legend; nothing in phase 1 blocks it (the legend already reserves the
modifier).

### Folding (`folding.go`)

`textDocument/foldingRange` from the current text's module (`ParseFile`), one range per
multi-line node of the kinds `folds.scm` lists: `ValueDecl`, `TypeDecl`, `TableDecl`, `CapDecl`,
`FakeDecl`, `SessionDecl`, `DeployDecl`, `ThemeDecl`, `TestDecl`, `FormDecl`, `Block`, `Match`,
`Element`, `RecordLit`, `ListLit`, and a scratch file's watch (`scratch.ParseFile`'s `Watch`
spans). `startLine` is the node's first line (0-based), `endLine` its last; a node whose span
ends at column 1 of a line ends on the line before (nothing of it is on that line). Two kinds
with a `kind`: a run of consecutive `--` comment lines (`comment`), and the run of `import`
lines (`imports`), so VS Code's "fold all comments" and "fold imports" work. Ranges are
emitted outermost first in source order; VS Code sorts and nests them itself. Single-line nodes
produce nothing.

### Selection (`selection.go`)

`textDocument/selectionRange`: for each position, `syntax.NodeAt(m, pos)` gives the path from
the module to the innermost node; the answer is the chain innermost-outermost with consecutive
equal spans collapsed and the module's own span last, as `SelectionRange{range, parent}`. It is
about thirty lines and gives `Shift+Alt+Right` the body, the arm, the match, the declaration.
No special cases from `textobjects.scm` (`@function.inner` = a binding's body): the chain passes
through those nodes already.

### Neovim

Neovim 0.11 enables semantic tokens for any server that advertises them, at a higher priority
than tree-sitter, and refreshes them after the server's debounce. Phase 1 tokens say nothing
tree-sitter does not, so they would only add a flicker between two identical colourings.
**(call)** alder.nvim's next release disables them for its client (`on_attach`:
`vim.lsp.semantic_tokens.enable(false, { bufnr = b })`, behind `opts.semantic_tokens = false`),
keeping tree-sitter its one source; revisit when phase 2 gives the tokens something tree-sitter
cannot know. Folding and selection are ignored by alder.nvim (its ftplugin sets the tree-sitter
foldexpr), so they change nothing there.

### Server tests (`internal/lsp`)

The existing harness (`start`, `openClean`, `c.at`, `blogCopy`): a decoder from the five-integer
stream back to `[]token` with names, then, on `examples/blog/app/Posts/Show.alder` for what it
holds and on a small fixture module written into the temp root (as `helpersCopy` builds one)
for the rest (`-1`, `True`, `?h`, an interpolated string, a broken declaration):

- `module` is `keyword`, `Posts.Show` is `namespace` (one token, as written); `import Layout`;
  `page` is `keyword` and `"/posts/{slug}"` is `string`; `load` on its signature and definition
  lines is `function` + `declaration`; `params`, `session` are `parameter` + `declaration` where
  bound; `Post.bySlug` is `namespace` then `function`; `found` after `let` is `variable` +
  `declaration`; `Just post` is `enumMember` then `variable`; `model.post.title` is `variable`,
  `property`, `property`; `<Layout.Page title={…}>` is `namespace`, `tag`, `attribute`, and the
  end tag's `Page` is `tag`; `<al-alert heading="Draft">` is `tag`, `attribute`, `string`;
  `<h1>` and `</h1>` are `tag`; the comment above `load` is `comment` + `documentation` and a
  comment inside a body is `comment` alone; `Db.from Comment |> Db.where` has `operator` for
  `|>`; `-1` is one `number`; `True` is `boolean`; `?h` is `hole`; `{ x }` in a string gives
  `string`, `variable`, `string`.
- A scratch file: `in Posts.Show` is `keyword`, `namespace`; `>` of a watch is `operator`
  **(call)** (nvim: `@punctuation.special`; the closest standard type).
- A client offering only `utf-16` (`startOffering`): a line `  s = "héllo" ++ x` gives `x` the
  UTF-16 column.
- `range` returns the subset and the same tokens as the full answer cut to those lines.
- A file with a broken declaration between two good ones: both good ones are fully coloured.
- A file outside the root, or an unknown URI: an empty stream, no error.
- Folding on `Show.alder`: `load`'s range, the `match` inside it, `<article>`'s, the `type
  Model` record; the `import` run has kind `imports`; a two-line comment block has kind
  `comment`; a one-line `form` declaration produces nothing.
- Selection at `title` of `model.post.title`: the chain `title`'s access, the outer access, the
  attribute value, the element, the view's body, the declaration, the module.
- `TestInitialize_Capabilities`: the three providers and the legend's exact lists (the
  extension pins the same lists; a change here must change both).

**Parity with tree-sitter** (`tree-sitter-alder/highlights_test.go`, beside `drift_test.go`,
same gating: the tree-sitter CLI, `ALDER_SKIP_TS=1` to skip, `minSources`): for every corpus
source the drift test reads, `tree-sitter query queries/neovim/highlights.scm <file>` gives the
captures (last capture on a span wins, as nvim applies them); a table maps each capture to a
legend type (the table above, `@punctuation.*`, `@spell` and `@character.special` to "none");
the Go producer runs on the same bytes; the two sets of (span, type) must be equal. The test
reports the first twenty differences with file and line. This is the guard that the two
editors keep looking the same, and it is what makes "transcribe the queries" a checked claim
rather than a hope.

## Changes: the extension

### Repository and layout

**(call)** A new repository, `tapegram/alder-vscode`, as alder.nvim is its own: a Marketplace
and Open VSX listing want a repository URL, README and CHANGELOG of their own; its CI is
Node-only and builds `alder` from a checkout of `tapegram/alder` at the ref in `.alder-ref`
(alder.nvim's workflow does exactly this: checkout, `go build -o $RUNNER_TEMP/bin/alder
./cmd/alder`, `ALDER_BIN`); and a server change that needs a client change is two PRs that name
each other, as today. Extension id `tapegram.alder`, display name "Alder", language id `alder`.

```
alder-vscode/
  package.json            contributes: language, grammar, configuration, commands, keybindings,
                          semanticTokenScopes, configurationDefaults, views (test controller)
  language-configuration.json
  syntaxes/alder.tmLanguage.json
  src/extension.ts        activation: discovery, health, one client per root, commands
  src/client.ts           LanguageClient per root, custom methods, alder/scratch handler
  src/cli.ts              run `alder <args>` (JSONL decode, report, cancel)   ← portable
  src/root.ts             app root of a path (the `app` listing rule)         ← portable
  src/generate/           picker.ts, form.ts, preview.ts, review.ts (pure), flow.ts (the steps),
                          apply.ts (plan → WorkspaceEdit)
  src/scratch.ts          open, send (is_definition), open value, the notification
  src/tests.ts            TestController, run and mutation profiles
  src/routes.ts, src/explain.ts, src/run.ts, src/health.ts
  test/unit/              node tests of the pure modules (ports of *_spec.lua)
  test/e2e/               @vscode/test-electron + mocha, against the real server
  test/fixtures/          copied from the alder checkout at test time (blog, gen_helpers)
```

TypeScript, `vscode-languageclient` 9 (LSP 3.17), esbuild bundle to `dist/extension.js`,
`@vscode/vsce` to package, engines `vscode ^1.90`, Node 20. No runtime dependencies beyond the
client library **(call)**: a diff for the review is the server's (`alder/preview` sends the
unified diff; `alder/plan` sends before/after, which `vscode.diff` renders), so no diff
library; fuzzy matching is the QuickPick's own.

### Language registration

`contributes.languages`: id `alder`, extensions `.alder`, aliases `Alder`, configuration
`language-configuration.json`:

- comments: `lineComment: "--"`;
- brackets `()`, `[]`, `{}`; autoClosingPairs the same plus `"` (not inside strings) and `<`
  is not a bracket (`a < b`);
- `wordPattern` allowing `'` in identifiers (`[A-Za-z_][\w']*|\?[a-z]\w*`, so a hole is one
  word);
- indentation, from `indents.scm`: `increaseIndentPattern` a line ending in `{`, `[`, `(`, `=`,
  `->`, `<-`, `then`, `else`, or a start tag (`<\w[^>]*>\s*$` without a matching end tag on the
  line); `decreaseIndentPattern` a line starting with `}`, `]`, `)`, `else` or an end tag;
  `onEnterRules`: after a line ending in an operator that `indents.scm` lists as a continuation
  (`|>`, `??`, `||`, `&&`, `++`, `+`, `-`, `*`, `/`, `==`, `!=`, `<=`, `>=`) indent once.
  The formatter (`alder fmt`, the server's `textDocument/formatting`) is the authority; these
  rules only place the cursor.
- `configurationDefaults` for `[alder]`: `editor.tabSize: 2`, `editor.insertSpaces: true`,
  `editor.semanticHighlighting.enabled: true`, `files.autoSave` untouched.

`syntaxes/alder.tmLanguage.json`, scope `source.alder`, the fallback and the layer under the
semantic tokens (VS Code keeps TextMate scopes under semantic tokens, so this is also where
punctuation gets its scopes): `comment.line.double-dash`, `string.quoted.double` with
`meta.embedded` for `{…}` interpolations, `constant.numeric`, `constant.language.boolean`,
`keyword.control.import` (`module`, `import`, `as`), `storage.type` (`type`, `brand`, `where`,
and the declaration words `page` … `silo`), `keyword.control.conditional` (`if then else
match`), `keyword.control.loop` (`for in`), `keyword.operator`, `punctuation.bracket`,
`variable.language.hole` (`\?[a-z]\w*`), and `entity.name.tag` / `entity.other.attribute-name`
on the lexical shape of a tag. Nothing name-dependent: the server owns that. Kept under 150
lines on purpose.

`contributes.semanticTokenScopes` for the custom types: `tag` → `entity.name.tag.alder`,
`attribute` → `entity.other.attribute-name.alder`, `boolean` → `constant.language.boolean.alder`,
`hole` → `variable.language.hole.alder`. `contributes.semanticTokenTypes` declares the four with
a `superType` (`tag` and `attribute` under `type`, `boolean` under `enumMember`, `hole` under
`variable`) so a theme with no rule for them still colours them.

### Binary discovery and the version gate (`src/health.ts`)

Setting `alder.path` (default `alder`). Resolution **(call)**: an absolute path is used as is;
otherwise the first of `PATH`, `$GOBIN`, `$GOPATH/bin`, `~/go/bin`, `~/bin` that holds an
executable `alder`, since VS Code opened from the Dock on macOS sees a shorter `PATH` than a
shell. The gate is alder.nvim's `health.lua` gate: `alder help --json` must exit 0 and decode
to a command tree holding `lsp` (an old alder exits 0 and prints text); `alder version` is read
for the health report. On failure, one error notification with the build instruction (`go build
-o ~/bin/alder ./cmd/alder` from a checkout) and an "Open settings" button; the client is not
started; no further nagging until the window reloads or `Alder: Restart Server`. After
`initialize`, `experimental.alderScratch` and `semanticTokensProvider` are read from
`client.initializeResult.capabilities`: missing scratch support warns once ("update alder for
scratch files"), missing tokens warns once ("update alder for highlighting; the fallback grammar
is in use"), and both show in the health report.

`Alder: Show Health` writes to the `Alder` output channel: the binary's path and version,
the help gate, each root with a running server and its capabilities, the fill environment
(whether `ANTHROPIC_API_KEY` or `ALDER_FILL_COMMAND` reaches the server, never their values).
It replaces `:checkhealth alder`.

### The client (`src/client.ts`)

One `LanguageClient` per app root, as alder.nvim has one client per root. The root of a file is
`root.find`'s port: the nearest ancestor whose directory *listing* has an entry named exactly
`app` that is a directory (read the listing, not `stat`, because on a case-insensitive file
system `App/` satisfies a stat of `app`); roots are compared by real path. A client starts when
the first `.alder` document under its root opens (`workspace.onDidOpenTextDocument` and the
documents open at activation); `documentSelector` is `[{ scheme: 'file', language: 'alder',
pattern: '<root>/**' }]`, so two apps in one workspace get two servers and no document reaches
the wrong one. `serverOptions`: `{ command: alderPath, args: ['lsp'], options: { cwd: root, env
} }` with `env` = `process.env` overlaid by the `alder.serverEnv` setting (an object; this is
how `ANTHROPIC_API_KEY`, `ALDER_FILL_MODEL`, `ALDER_FILL_COMMAND` and `ALDER_LSP_LOG` reach the
server when the shell does not export them). `initializationOptions`: none (the server reads
none). `workspaceFolder` set to the folder containing the root so `rootUri` is the root: the
client library sends `rootUri` from the workspace folder, not the selector, so `clientOptions`
pass `workspaceFolder: { uri: root }` explicitly **(call)**; the server resolves symlinks itself.

Client capabilities come from the library's defaults, which cover everything the server reads
in `initializeParams`: `workspace.applyEdit`, `window.workDoneProgress`, `window.showMessage`
(so the server asks Apply/Discard rather than refusing effectful fills),
`workspace.inlayHint.refreshSupport`, `general.positionEncodings: ['utf-16']`.

A server exit with a non-zero code or a signal (the library's `ErrorHandler.closed`): one
warning "alder lsp stopped — Alder: Show Health", one restart attempt (the library's default),
then off until `Alder: Restart Server`. On `dispose` of a client, the generator flow and the
routes picker close if open (the nvim modal closes on `LspDetach`).

Custom methods: `client.sendRequest('alder/generators' | 'alder/preview' | 'alder/plan' |
'alder/routes', params)` with the types from `generators.go` and `routes.go` transcribed to
TypeScript (`FieldState`, `PreviewResult`, `PlanResult`, `RouteItem`); a `ResponseError` with
code `-32800` (a cancelled or superseded preview) is swallowed by the caller, as `lsp.request`
in nvim swallows it; any other error is shown with the server's message, unreworded.
`client.onNotification('alder/scratch', …)` goes to `src/scratch.ts`.

Commands the server lists (`alder.explain`, `alder.fill`) are registered by the library as VS
Code commands that forward to `workspace/executeCommand`; the extension's own command ids avoid
those two. The `alder.generate` command a generator code action carries (its argument is the
`generatorItem`: `{ id, sets }`) is the extension's, registered with
`commands.registerCommand('alder.generate', item?)`, and opens the flow preset with it, as
`vim.lsp.commands['alder.generate']` does.

### Commands, keys, settings

Commands (`contributes.commands`, category "Alder"), each with its nvim origin:

| command | does | nvim |
|---|---|---|
| `alder.generate` | the generator flow; with an item argument, preset | `:AlderGenerate`, `<leader>ag` |
| `alder.routes` | the routes QuickPick | `:AlderRoutes`, `<leader>au` |
| `alder.run` | `alder run` in a terminal | `<leader>ar` |
| `alder.explainDiagnostic` | explain the diagnostic at the cursor (or ask for a code) | `:AlderExplain`, `<leader>ae` |
| `alder.test.run`, `alder.test.runAtCursor`, `alder.test.mutate` | the Test Explorer profiles from the palette | `<leader>at`, `aT`, `am` |
| `alder.scratch.open`, `alder.scratch.send`, `alder.scratch.openValue` | scratch files | `<leader>ax`, `as`, `ao` |
| `alder.showHealth`, `alder.restartServer` | health, restart | `:checkhealth alder`, `:LspRestart` |

No `Alder: Check` and no `Alder: Diagnostics` **(call)**: `publish` in `diagnostics.go` sends
every file's diagnostics, closed files included, so the Problems panel already is
`:AlderDiagnostics`, and `alder check` on save would duplicate the server. `:AlderFix` is the
server's quick-fix code action.

Keybindings **(call)**: a chord under `ctrl+alt+a` (`cmd+alt+a` on macOS) mirroring the
`<leader>a` group: `g` generate, `u` routes, `r` run, `e` explain, `t` test, `shift+t` test at
cursor, `m` mutate, `x` scratch, `s` send, `o` open value; the `when` clause is
`editorLangId == alder` for the buffer-local ones and `alder.inProject` (a context key set when
the active editor's file has a root) for the project-level ones, as `keys.lua` splits them.

Settings: `alder.path`, `alder.serverEnv`, `alder.trace.server` (the library's wire trace into
the output channel), `alder.test.args` (extra `alder test` arguments). `check_on_save`,
`preview`, `revision`, `path` (grammar) have no counterpart.

### The generator flow (`src/generate/`)

The nvim modal is one 80% float with a search list, a form pane and a live diff (modal.lua, 672
lines of glue over four pure modules). VS Code has no multi-pane modal; the port is a
multi-step quick input beside a live preview document, with the pure modules ported line for
line and tested by the ported specs, so the glue (`flow.ts`) stays thin. **(call)** Native quick
input first, a webview only if this feels thin in use; `form.ts`, `picker.ts`, `preview.ts` and
`review.ts` do not import `vscode` so a webview could reuse them.

**Origin.** As `project.with_alder_buf`: the active editor when it is an `.alder` document with
a running client; else `app/App.alder` of the root (else the first `.alder` under `app/`) is
opened with `workspace.openTextDocument` (not shown; the client sends `didOpen` for it) and the
flow waits up to 5 s for a client. The origin is `(uri, cursor)` and every request carries it,
as `params()` in modal.lua does.

**Step 1, the picker.** `alder/generators` at the origin; no items → "alder: no generators".
A `QuickPick` with `sortByLabel: false` (the server's order: here first), `matchOnDescription`,
items `{ label: id, description: summary, detail: why }`, a `QuickPickItemKind.Separator`
"here" above the first `here` item and "all" above the first other (`picker.lines`'s headings),
`ignoreFocusOut: true`. Enter selects; Escape closes. With a preset id (`alder.generate` with an
argument), the picker is skipped.

**Step 2, the form.** On selecting an item, `sets = item.sets` overlaid by the preset's
(`form.sets`'s preset) and the first `alder/preview` is sent. The form is a `QuickPick` whose
items are the visible fields (`form.visible`: `agent` fields hidden), one per field:
`label` the name, `description` the value as the form line shows it (`[x]`/`[ ]` for a switch,
the text otherwise), `detail` the note (`form.lines`'s note rule: the server's `why` in error
colour becomes a `$(error)` prefix; `required` while empty; `default: X` for a cleared field;
else `source · help`); plus a final item "Apply" (`$(check)`, enabled only when the last
preview said `ready`) and buttons: Back, Review (`$(diff)`), Apply (`$(check)`). The title is
`id  summary`, the bottom-right of the preview document's tab shows the main file. Enter on a
field opens its editor; on "Apply", applies.

The per-kind editors (each returns to the form item list afterwards, as Tab/Enter return to
the form in nvim):

- `bool`: a two-item QuickPick (`on`, `off`), the current one active; Enter toggles.
- `choice`: a QuickPick of `choices`, the current value active (`start_pick`), the user's
  typing filters (the QuickPick's own fuzzy match; `form.options` is only used by the tests
  and the webview later); Enter commits (`commit_option`).
- `module`: a QuickPick in "free text" mode: as the user types, the items are
  `form.module_suggest(form.module_dirs(root), text)` (the folder prefixes, `Sites.`,
  `Sites.Clarity.`) with an always-present first item showing `form.module_help(text)`
  (`Sites.Clarity.Index → app/Sites/Clarity/Index.alder`) whose acceptance commits the typed
  text; accepting a prefix item puts it into the input (`accept_suggestion`, `<C-y>`) and keeps
  the picker open. A value starting with `<` is the server's placeholder and counts as empty.
- `text`: an `InputBox` with `value` the current text, `prompt` the help, `placeholder` the
  example (`e.g. …`), and `validationMessage` the server's `why` for this field from a debounced
  (80 ms, `DEBOUNCE_MS`) `alder/preview` on each keystroke, so a bad name is red before Enter.

Every commit (`set_field`) records `typed[name]` (an empty text drops the key: the server then
falls back to the cursor), re-renders the items, and requests a preview. The sequence rule is
modal.lua's: a module-level counter; `invalidate` bumps it, an answer with an older number is
dropped, `ready` is false until a newer answer arrives, and a request in flight shows
"updating…" in the form's title. The server cancels superseded previews itself (`-32800`,
swallowed).

**The preview document.** An `alder-preview:` `TextDocumentContentProvider`, one document per
flow, opened once beside the origin (`ViewColumn.Beside`, `preserveFocus: true`, language
`diff`, read-only), its text `preview.render(r)`'s lines: the files (`+ path`, `- path`,
`~ path  +n −m`), a rule, the unified diff, the holes (`? name : type`), or the refusal; each
answer fires `onDidChange` and the editor is scrolled to the first hunk (`preview.first_hunk`).
With `ignoreFocusOut` on the quick input, updating the document does not close the form.
Closing the flow disposes the document (`workbench.action.closeActiveEditor` on its editor).

**Enter rules** (modal.lua's `enter`): "Apply" while a preview is in flight does nothing; not
`ready` → focus the first invalid field (`form.first_invalid`); from the picker with a required
field still at the server's starting value and untouched (`form.first_to_fill`) → open that
field's editor first, so nothing is generated unseen (alder.nvim#6); `Ctrl+Enter` in the form
(a keybinding on the quick input's `when` context `alder.formOpen`) applies at once, as
`<C-CR>` does.

**Review** (`review.ts`, the Review button): `alder/plan` with the current sets; if the sets
changed while it was in flight, dropped. `review.model(plan)` gives the notes (refusal,
diagnostics, holes with their address, imports, migration) and the files (`+ ~ -`). The review
is a QuickPick whose first items are the notes (separator "plan", not selectable) and then the
files; the active file's diff is shown with `vscode.diff(alder-plan://before/<path>,
alder-plan://after/<path>, title)` beside (the two sides from a second content provider; a
created file's before and a removed file's after are empty), updated on `onDidChangeActive`.
Buttons: Back, Apply (hidden when `!can_apply`). Enter applies with the planned sets; Escape
returns to the form with its focus restored.

**Apply** (`apply.ts`, the port of apply.lua):

1. `alder/plan` with the sets the preview or review showed; `plan.error` → notification plus the
   plan's `diagnostics` into a `DiagnosticCollection('alder generate')` (they describe the dry-run
   copy, so they never join the server's; cleared on the next apply), the port of `refusal_list`.
2. Staleness: for every file, the text as the user sees it (`workspace.textDocuments` entry
   when open, else the file on disk, `text_of`) must equal `before` (`null` for a created file:
   the file must not exist); else "alder: <path> changed since planning; nothing applied".
3. A removed file: `window.showWarningMessage` (modal) "Delete <path> from disk? undo cannot
   restore it" with "Delete"; then the staleness check again (the user may have typed).
4. One `WorkspaceEdit` for everything, applied with `workspace.applyEdit`, so it is one undo
   step **(call)**: an edited file is a full-range `replace` on its document (opened with
   `openTextDocument` first, not saved); a created file is `createFile(uri)` plus an `insert` of
   `after` at the start (VS Code creates the file and leaves the text dirty in an opened editor,
   the counterpart of `b:alder_new`; the directory is created by `createFile`); a removed file
   is `deleteFile(uri)`. `applyEdit` returning `false` is reported as "nothing applied".
5. The cursor: the first hole's file at `(line-1, col-1)` (`holes[0]`), else the first created
   `.alder` file, else the first edited one not already active; the information message
   "alder: applied <id>: <paths> (<n> holes)". The lightbulb at the hole offers `Fill ?h with AI`.

### Scratch files (`src/scratch.ts`)

The server already renders watch values as inlay hints: `inlayHintReq` serves `⇒ value : type`,
`✗ error` or `: type` per watch from the last `alder/scratch` it sent, and asks
`workspace/inlayHint/refresh` after each run. VS Code shows those through the stock client, so
**(call)** values are inlay hints, not decorations: no version bookkeeping, no dimming logic,
and a stale hint disappears by the server's own line comparison. The extension adds what hints
cannot carry:

- `alder/scratch` handler: keeps the last params per URI (watches with `line`, `end`, full
  `value`, `error`, `note`, `truncated`); a file-level `error`, `note` or `warning` (once per
  file) is a line-1 after-decoration in the editor showing that document, dimmed when
  `params.version !== document.version`.
- `alder.scratch.openValue`: the watch whose lines hold the cursor (`value_at`); its full text
  (error, else value, else note, then `: type`) in a read-only `alder-scratch:` document beside.
  A hover provider on `scratch.alder` shows the same in a code block for a multi-line value, the
  stand-in for nvim's virtual lines.
- `alder.scratch.open`: `<root>/scratch.alder`, created from `TEMPLATE` (the text of
  `internal/scaffold/files/scratch.alder`, duplicated as scratch.lua duplicates it) when missing.
- `alder.scratch.send`: the selection or current line appended to `scratch.alder` and saved
  (saving runs it: `didSave` → `saveRerunScratch`): `is_definition`'s rule decides between a
  definition verbatim and `> expr` with 4-space continuation lines; lines already holding
  watches go in raw; the `in Module` warning once per session when the sending module is not
  the header's. The module name comes from the first `module` line by regex (`buf.lua`'s
  fallback; there is no tree-sitter here).

### Tests, mutation, routes, run, explain

**Test Explorer** (`src/tests.ts`): one `TestController('alder', 'Alder')` per window; items
`root › Module › "name"`. Discovery **(call)**: a regex pass `^test\s+"(.*)"\s*=` over
`app/**/*.alder` (`workspace.findFiles`, re-run on change of an `.alder` document; `buf.lua`'s
regex), since the server has no test-listing method; a run's results (`alder test --json`
carries `name`, `module`, `file`, `line`, `col`) correct the tree. Run profile: `alder test
--json [alder.test.args]` from the root (`cli.ts`), after saving the dirty `.alder` documents
under the root (`write_app_buffers`: an unsaved one is reported, the run reads disk); selected
items become `-run '<pattern>'` with `run_pattern`'s escaping (`^Module "name"$`, `^Module `
for a module). Each result → `passed`, or `failed` with a `TestMessage` carrying `left`/`right`
as expected/actual (`TestMessage.diff`), `error` or `note` as the message, located at
`file:line:col`; `summary`'s "n passed, n failed" in the run output. A compile failure (the
run's objects are `diagnostics`) → `errored` on the root item with "the app does not check";
the server's Problems already show the errors, so the CLI's are not re-published. A newer run
cancels the older (`SIGTERM`); the run's `token` cancels too. Exit 2 → `cli.report`'s error
document ("This may be a compiler bug", the command, `alder version`).

**Mutation**: a second, non-default profile "Mutation testing" on the same controller:
`alder test --mutate --json`; survivors → `DiagnosticCollection('alder mutate')` warnings
`survived  kind: before → after` at their position (the port of `survivor`), cleared at the next
mutation run; the score line (`mutate_title`) in the run output and a notification. Killed
mutants produce nothing.

**Routes**: `alder/routes` through the root's client, else `alder routes --json` (an older
server answers method-not-found: the fallback, with the "update alder" message when both fail,
as `routes.fetch_cli` says it); items `label: 'GET /path'`/`'POST /path'`, `description:
Module.name`, `matchOnDescription`; Enter opens the declaration at its range.

**Run**: `window.createTerminal({ name: 'alder run', cwd: root })`, `sendText('alder run')`,
shown; reused while alive. `alder logs -f` is the same shape and is left to the user's terminal.

**Explain**: the diagnostic under the cursor from `languages.getDiagnostics(uri)` with source
`alder` and a code (the one whose range holds the cursor, else the first on the line), or an
input box for a code; `workspace/executeCommand` `alder.explain [code]`; the markdown goes into
an `alder-explain:` virtual document shown with `markdown.showPreviewToSide` **(call)** (the
built-in markdown extension renders it; no webview of our own).

**Fill with AI**: nothing to write. The code action is the server's; the library forwards
`alder.fill`, shows `$/progress` in the status bar, turns `window/showMessageRequest` into a
warning with Apply and Discard, and applies the versioned `workspace/applyEdit` (answering
`applied: false` when a document moved on, which the server reports). The README says what
leaves the machine (the module's source, to the configured model) and names the three
environment variables.

### Packaging

`npm run package` → `vsce package` → `alder-<version>.vsix`; a GitHub release workflow on a tag
attaches it; install with `code --install-extension alder-x.y.z.vsix` (the README's path until
the listings exist). Versioning is the extension's own; the compatibility contract is the
runtime gate (help tree, capabilities), never a pinned alder version, so a newer alder keeps
working and an older one says what to update. Marketplace (`vsce publish`, a publisher PAT) and
Open VSX (`ovsx publish`) are a later milestone once a few releases have been dogfooded.

### Extension tests

- **Unit** (`node --test` or mocha, no VS Code): `form.ts` (the port of `form_spec`: lines and
  notes, `sets` dropping empties, `first_invalid`, `first_to_fill`, module dirs/path/help/suggest),
  `picker.ts` (`picker_spec`), `preview.ts` (`preview_spec`: render, first_hunk), `review.ts`
  (`review_spec`: model), `routes.ts` (normalize, label), `tests.ts` (`test_spec`: entry,
  summary, run_pattern), `scratch.ts` (`is_definition`, the appended text), `cli.ts` (JSONL
  decode, report selection), `root.ts` (the `app` listing rule, symlinks).
- **Contract** (`@vscode/test-electron` + mocha inside the extension host, `ALDER_BIN` set to a
  binary built from the `.alder-ref` checkout, fixtures copied from it: `examples/blog`,
  `testdata/golden/gen_helpers`; no fake server **(call)**: the real one is fast and in process
  terms cheap, and a stub would only certify the stub):
  - activation on a blog file starts one client whose `initializeResult` has the legend, scratch
    and the two providers; `vscode.provideDocumentSemanticTokensLegend` matches the pinned lists;
    `vscode.provideDocumentSemanticTokens` on `Posts/Show.alder` gives `tag` at `<article>` and
    `function`+`declaration` at `load`; `vscode.executeFoldingRangeProvider` folds `load`;
    `vscode.executeSelectionRangeProvider` at `model.post.title` grows through the access;
  - a deliberate error typed into a document appears in `languages.getDiagnostics` with source
    `alder` within the debounce, and the quick fix of a diagnostic with a `fix` applies;
  - the generator flow driven through an injected `ui` interface (the steps take `ui.pick`,
    `ui.input` instead of `window.*`, so the test answers them; the port of `modal_spec`'s
    state tests): `new msg` on a page, a required field left at its default opens that field
    first, a stale preview answer is dropped, apply writes the message into the document unsaved
    and lands on the first hole, a removed file asks before `deleteFile`, a changed `before`
    refuses;
  - scratch: open creates the template, send appends `> 1 + 1` and saves, the inlay hint
    `⇒ 2 : Int` appears (`vscode.executeInlayHintProvider`), open value shows it;
  - routes lists the blog's pages; the test controller runs the blog's tests and marks a failing
    one with its expected/actual; mutation survivors land in Problems;
  - fill: with `ALDER_FILL_COMMAND` pointing at a script that prints a fixed expression (as the
    server's fill tests drive it), the code action at a hole runs, progress ends and the edit is
    applied; with an effectful expression the Apply/Discard question appears and Discard leaves
    the document alone;
  - two apps in one workspace get two clients and a document under each reaches its own.
- **CI**: lint (`tsc --noEmit`, eslint), unit, contract on Linux (xvfb) and macOS, package
  the `.vsix` as an artifact on every push; the alder checkout at `.alder-ref` is cached by
  `go.mod` hash, as alder.nvim's workflow caches it.

## Milestones (pull requests, in order; no estimates)

1. **alder: semantic tokens, folding, selection (phase 1).** `syntax.ParseTokens`; `tokens.go`
   with the legend and producer; `folding.go`; `selection.go`; capabilities and
   `fastMethods`; the Go tests above; `tree-sitter-alder/highlights_test.go` parity. CI's
   existing tree-sitter install covers the parity test.
2. **alder.nvim: keep tree-sitter.** `opts.semantic_tokens = false` and the `on_attach` line;
   health reports whether the server offers tokens. One small PR.
3. **alder-vscode: scaffold.** The repository; language registration, configuration and the
   TextMate grammar; discovery, the gate, health, settings; one client per root; `alder/scratch`
   wired but unused; commands and keybindings registered with "not yet" bodies; unit and contract
   harnesses with the blog fixture; CI; `vsce package`. Usable at the end of this PR: everything
   the stock client gives, highlighting and folding from PR 1, fill.
4. **alder-vscode: routes, run, explain, tests.** `cli.ts`, `routes.ts`, `run.ts`, `explain.ts`,
   `tests.ts` with both profiles and the survivors collection.
5. **alder-vscode: the generator flow.** The pure modules and their unit tests first (ports of
   the specs), then `flow.ts`, the preview document, `apply.ts`; the review in the same PR if it
   stays small, else a follow-up.
6. **alder-vscode: scratch.** Open, send, open value, the notification handler and hover.
7. **alder-vscode: release.** README (install, requirements, what fill sends), CHANGELOG, the
   tag workflow with the `.vsix`; a dogfooding pass on nightvote-replay; then the Marketplace and
   Open VSX milestone when it is warranted.
8. **Later:** server tokens phase 2 (refs, `defaultLibrary`, refresh) with alder.nvim opting in;
   embedded `<script>`/`<style>` scopes in the TextMate grammar; a webview form only if the quick
   input flow feels thin after 7; a server-side test listing (`alder/tests`) if the regex
   discovery misleads.

## Durandal: what is meant to be shared later

Out of scope here, but the shapes are the same, so these modules are written without `vscode`
imports where they can be, behind small interfaces, and named for what they do rather than for
Alder: the JSONL CLI runner and its error report (`cli.ts`), the multi-step quick-input form
over a server-described field list (`form.ts`, `flow.ts`'s step helpers), the preview content
provider and the before/after review with `vscode.diff` (`preview.ts`, `review.ts`), the plan
apply with the staleness check and the delete confirm (`apply.ts`), binary discovery and the
help/capability gate (`health.ts`). When the Durandal extension starts, these move to a shared
package (a workspace of the two repositories or a small published module); until then they
live in `alder-vscode/src` and nothing in their interfaces names a modal or a buffer.
