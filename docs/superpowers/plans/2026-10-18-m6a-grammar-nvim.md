# M6a — tree-sitter grammar and alder.nvim v1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A tree-sitter grammar for Alder in this repo, gated by a drift test against the Go parser, with Neovim queries, plus `alder help --json` and a new plugin repo `tapegram/alder.nvim` that gives highlighting, indentation, folds, check-on-save diagnostics with fixes, and the generators inside Neovim.

**Architecture:** `tree-sitter-alder/` holds `grammar.js`, an external scanner `src/scanner.c` that ports the Go lexer's newline/continuation, HTML-text, raw-text and string rules, the committed generated parser, corpus tests, and the queries for Neovim. A Go test package in the same directory runs the `tree-sitter` CLI over every `.alder` file the Go parser accepts and compares top-level declarations. The plugin is thin Lua: it registers the parser with nvim-treesitter main, runs `alder … --json` with `vim.system`, and shows the compiler's output verbatim.

**Tech Stack:** tree-sitter CLI 0.27.1 (`TREE_SITTER_JS_RUNTIME=native`, no Node), C (scanner), Go 1.27 (drift test, `alder help --json`), Lua on Neovim ≥ 0.11 (dev machine: 0.12.5), nvim-treesitter `main`, GitHub Actions.

**Spec:** `docs/superpowers/specs/2026-10-18-m6a-grammar-nvim-design.md` (read it with this plan).

## Global Constraints

- tree-sitter CLI: **0.27.1** pinned everywhere the parser is generated (CI and `scripts/ts.sh generate`); the spec's floor is ≥ 0.26.1. Generated `src/` files differ between CLI versions, so only 0.27.1 may regenerate them.
- No npm, no `package.json`, no Node bindings. `tree-sitter.json` sets every binding to `false`. Generation uses `TREE_SITTER_JS_RUNTIME=native`.
- `src/parser.c`, `src/grammar.json`, `src/node-types.json`, `src/tree_sitter/*.h` are generated **and committed**.
- The grammar mirrors `internal/syntax` (the Go parser), never `book/A-grammar.md` (stale).
- Acceptance: the grammar accepts everything the compiler accepts and may accept a little more. No negative corpus.
- The drift test **fails** when the CLI is absent unless `ALDER_SKIP_TS=1`; no CI job sets `ALDER_SKIP_TS`.
- A PR that changes the syntax changes the grammar, its queries and its corpus in the same PR.
- `.github/shard-weights` is untouched.
- Plugin: Neovim ≥ 0.11, nvim-treesitter `main` only, `alder` and `tree-sitter` on PATH, Apache-2.0, no AstroNvim-specific code, no test framework (plain `assert` under headless Neovim).
- **The plugin never rewords the compiler**: diagnostics, refusals (`error`), stderr and generator output are shown verbatim. The plugin's own sentences are only the ones listed in the spec's Error handling section.
- Diagnostics columns from `alder check --json` are 1-based **bytes** with exclusive ends; Neovim's are 0-based bytes, so subtract 1 from line, col, endLine and endCol.
- No generator runs while any `.alder` buffer in the app has unsaved changes: `alder: save all buffers first (:wall)`.
- Working hours (user rule): no agent or background work 9am–5pm America/New_York on weekdays; weekends are free. Stop before 9 and record the resume point.
- Branch in alder: `m6a-grammar-nvim`. Plugin repo: `~/projects/alder.nvim`, work on branch `v0.1`, public GitHub repo created only after the user confirms (Task 14).

## Rulings over the spec

These were found while planning (2026-10-18) and override the spec where they differ:

- **R1 Precedence.** The spec's line "`pipe_expr` > `or_expr` > `default_expr`" is wrong. The Go parser (`parser_expr.go` `parsePipe` → `parseDefault` → `parseOr`) makes `??` looser than `||`, and `??` is right-associative: lowest to highest `|>`, `??`, `||`, `&&`, comparison, `+ - ++`, `* / %`, application, `.field`.
- **R2 Column-1 keywords.** The Go lexer demotes `module import type page msg form table capability fake session login test` to identifiers when they are not at column 1. Tree-sitter's keyword extraction gives the same effect: a keyword is only produced in a parse state where it is valid, so `msg`, `form`, `table`, `page`, `test` work as variable names inside expressions. `silo`, `choose`, `deploy`, `role`, `via` are also grammar keywords for the same reason.
- **R3 Signature files.** `stdlib/*.alder` are read by `syntax.ParseSignatures` (bodies optional, opaque `type X a`, values named `where`, `fake`, `login`). The grammar accepts these, and the drift test parses `stdlib/` with `ParseSignatures`.
- **R4 Field attributes vs HTML attributes.** `@id`, `@ref T` and the rest are `field_attribute` nodes, and HTML `name="v"` is `attribute`. The spec used one name for both.
- **R5 Raw elements.** `<script>`/`<style>` tag names come from the external scanner (`_raw_tag_name`), so the grammar has no `'script'` literal that would fight the HTML name token. Close tags are compared case-insensitively by the compiler and accepted leniently by the grammar.
- **R6 Plugin `path`.** nvim-treesitter main resolves `install_info.queries` relative to `install_info.path` when `path` is set (`install.lua` `try_install_lang`). So `setup{ path = … }` is the **alder checkout root** (`~/projects/alder`), not `…/tree-sitter-alder`. The plugin always adds `location = 'tree-sitter-alder'` and `queries = 'tree-sitter-alder/queries/neovim'`.
- **R7 Passthrough output.** `:Alder <generator> …` (new, edit, remove, fill, regen) runs with `--json --dir <root>` and renders files, diff, migration, holes and diagnostics. `:Alder check` publishes diagnostics. `:Alder run`, `:Alder repl` and `:Alder logs … -f` open a terminal split. Every other command runs without `--json` and its text output is shown verbatim in a scratch split, because those commands' JSON is M6c's business.
- **R8 CI install.** CI installs the pinned CLI from the tree-sitter GitHub release (`.github/install-tree-sitter.sh`), not with brew, because the regeneration check needs an exact version. Developers use `brew install tree-sitter-cli` and `scripts/ts.sh` refuses to regenerate with any version but 0.27.1.
- **R9 Help table.** Text help is hand-written usage strings with no flag table (`cmd/alder/help.go`). `alder help --json` adds a structured `commandTree` whose `Usage` fields are those same strings, and a test pins every flag in the tree to the flags the strings show. The missing `silo` (generator) and `silos` (command) usage entries are added.
- **R10 Parser module.** The parser registration lives in `lua/alder/parser.lua` (called from `plugin/alder.lua`) so tests can call it; `lua/alder/config.lua` holds options.

## Review Focus

The inputs most likely to bite a person using this, which no spec section tests directly. Each has a test in the task that owns the code.

1. **Code mid-edit.** An unterminated string, an unclosed `<div>` or a dangling `let x =` must not turn the rest of the file into one error or into HTML text: the next column-1 declaration still parses as a declaration. Test: Task 5 `TestRecovery`.
2. **macOS case-insensitive file system.** A file in `app/App/…` must not make `app/` look like the app root, because a stat of `app/app` succeeds on APFS for `app/App`. Test: Task 9 root test.
3. **Stale and superseded diagnostics.** Saving twice quickly must not let the first run's late result overwrite the second, and a file whose error is fixed must lose its diagnostics. Test: Task 10.
4. **Symlinked paths.** `/tmp` is `/private/tmp` on macOS. A buffer opened through one spelling must still receive diagnostics the CLI reports for the other. Test: Task 10.
5. **Multi-byte text before a diagnostic or fix.** Columns are bytes, and a fix must land on the right bytes after `é` or `→` earlier on the line. Test: Task 10.

---

## File structure

**alder repo** (branch `m6a-grammar-nvim`):

| Path | Responsibility |
|---|---|
| `tree-sitter-alder/grammar.js` | the grammar (Task 1, refined in 2–5) |
| `tree-sitter-alder/src/scanner.c` | external scanner: `_newline`, `string`, `html_text`, `raw_text`, `_raw_tag_name` |
| `tree-sitter-alder/src/{parser.c,grammar.json,node-types.json,tree_sitter/}` | generated, committed |
| `tree-sitter-alder/tree-sitter.json` | grammar metadata, all bindings off |
| `tree-sitter-alder/.gitignore` | build outputs (`parser.so`, `*.dylib`, …) |
| `tree-sitter-alder/test/corpus/*.txt` | corpus tests, one file per construct family |
| `tree-sitter-alder/test/highlight/basic.alder` | CLI highlight test for the root query |
| `tree-sitter-alder/test/drift.scm` | the query the drift test reads declarations with |
| `tree-sitter-alder/drift_test.go` | Go package `grammar`: drift, recovery, corpus, regeneration tests |
| `tree-sitter-alder/queries/highlights.scm` | minimal CLI highlight query |
| `tree-sitter-alder/queries/neovim/*.scm` | highlights, locals, indents, folds, injections, textobjects |
| `tree-sitter-alder/README.md` | how to develop the grammar |
| `scripts/ts.sh` | `generate|test|parse` wrapper with the version check |
| `.github/install-tree-sitter.sh` | CI install of the pinned CLI |
| `.github/workflows/ci.yml` | `grammar` job; CLI install in the `test` job |
| `cmd/alder/helpjson.go` | `commandTree`, `writeHelpJSON` |
| `cmd/alder/helpjson_test.go` | pins the tree to the usage strings |
| `cmd/alder/help.go` | `help --json` dispatch, `silo`/`silos`/`help` usage entries |
| `book/01-why.md`, `book/11-tooling.md` | "Editor support" |

**alder.nvim repo** (`~/projects/alder.nvim`):

| Path | Responsibility |
|---|---|
| `plugin/alder.lua` | filetype, `User TSUpdate`, user commands, save autocmd |
| `ftplugin/alder.lua` | buffer options, treesitter start, folds, indent |
| `lua/alder/init.lua` | `setup{}` |
| `lua/alder/config.lua` | options and defaults |
| `lua/alder/parser.lua` | `install_info`, `register()` |
| `lua/alder/root.lua` | app root: nearest ancestor with a directory named exactly `app` |
| `lua/alder/cli.lua` | run `alder` async, JSON/JSONL decoding, error reporting |
| `lua/alder/check.lua` | diagnostics, quickfix, `:AlderCheck`, `:AlderFix`, `:AlderExplain` |
| `lua/alder/commands.lua` | `alder help --json` cache, `:Alder` and completion |
| `lua/alder/generate.lua` | preview-then-apply, `:AlderNew`, after-write |
| `lua/alder/fill.lua` | `:AlderFill` |
| `lua/alder/health.lua` | `:checkhealth alder` |
| `tests/minimal.lua`, `tests/run.lua`, `tests/helpers.lua`, `tests/*_spec.lua` | headless tests |
| `.github/workflows/test.yml` | plugin CI |
| `README.md`, `LICENSE` | docs, Apache-2.0 |

---

## Task 0: Branch and tool

- [ ] **Step 1: Branch**

```bash
cd ~/projects/alder && git switch main && git pull --ff-only && git switch -c m6a-grammar-nvim
```

- [ ] **Step 2: Check the CLI**

Run: `tree-sitter --version`
Expected: `tree-sitter 0.27.1` (installed with `brew install tree-sitter-cli` on 2026-10-18). If brew has moved on, install 0.27.1 from the release instead: `curl -fsSL https://github.com/tree-sitter/tree-sitter/releases/download/v0.27.1/tree-sitter-macos-arm64.gz | gunzip > ~/bin/tree-sitter && chmod +x ~/bin/tree-sitter`.

The CLI prints a warning about unconfigured parser directories to stderr on every run. It is harmless; ignore it everywhere.

---

## Task 1: Grammar, scanner and the core corpus

The whole grammar is written here because its precedences interact; Tasks 2–4 add corpus families and fix what they reveal.

**Files:**
- Create: `tree-sitter-alder/grammar.js`, `tree-sitter-alder/src/scanner.c`, `tree-sitter-alder/tree-sitter.json`, `tree-sitter-alder/.gitignore`, `scripts/ts.sh`
- Create: `tree-sitter-alder/test/corpus/module.txt`, `newlines.txt`, `expressions.txt`, `strings.txt`, `comments.txt`
- Generated: `tree-sitter-alder/src/parser.c`, `grammar.json`, `node-types.json`, `tree_sitter/*.h`

**Interfaces:**
- Produces (node names every later task and query uses): `source_file`, `module_decl{name}`, `import_decl{module}`, `module_name`, `value_decl{signature,binding}`, `signature{name,type}`, `binding{name,params,body}`, `parameters`, `type_decl{name,param,body}`, `union_type`, `variant{name,argument}`, `record_type`, `brand_type{base,check}`, `field_decl{name,type,via,default,attribute}`, `field_attribute{name,argument}`, `page_decl{path,params,requires}`, `requires_clause`, `session_key`, `role_clause{role}`, `silo_clause{key}`, `msg_decl{name,params,form}`, `form_decl{name,fields}`, `table_decl{name,attribute,fields}`, `capability_decl{name,entries}`, `fake_decl{name,body}`, `fake_body`, `fake_entry{name,params,body}`, `session_decl{fields}`, `login_decl{page}`, `test_decl{name,body}`, `deploy_decl{body}`, `deploy_record`, `deploy_field{name,value}`, `deploy_list`, `deploy_target{kind,argument,options}`, `silo_decl{key,table}`, `choose_decl{key,page}`, types `type_arrow{parameter,result}`, `type_app{constructor,argument}`, `type_name`, `type_var`, `unit_type`, `parenthesized_type`, expressions `pipe_expr`, `default_expr`, `or_expr`, `and_expr`, `comparison_expr`, `additive_expr`, `multiplicative_expr` (all `{left,right}`), `application{function,argument}`, `field_access{value,field}`, `qualified_name{module,name}`, `lambda{param,body}`, `if_expr{condition,consequence,alternative}`, `match_expr{subject}`, `arm{pattern,body}`, `record`, `field_init{name,value}`, `record_update{record}`, `block{result}`, `let_stmt{name,type,value}`, `bind_stmt{name,type,value}`, `list`, `parenthesized_expression`, `unit`, `hole`, `integer`, `float`, `negative_number`, `string`, `boolean`, `identifier`, `constructor`, patterns `wildcard`, `constructor_pattern{constructor,argument}`, `record_pattern`, `field_pattern{name,pattern}`, `list_pattern`, `rest_pattern{name}`, `parenthesized_pattern`, HTML `element{open,close}`, `start_tag{tag}`, `end_tag{tag}`, `self_closing_element{tag}`, `raw_element{tag,close}`, `raw_text`, `tag_name`, `component_name`, `attribute{name,value}`, `attribute_name`, `html_text`, `interpolation{expression}`, `html_for{binder,items,body}`, `html_if{condition,consequence,alternative}`, `html_match{subject}`, `comment`.

- [ ] **Step 1: Write `tree-sitter-alder/tree-sitter.json`**

```json
{
  "grammars": [
    {
      "name": "alder",
      "camelcase": "Alder",
      "title": "Alder",
      "scope": "source.alder",
      "path": ".",
      "file-types": ["alder"],
      "highlights": "queries/highlights.scm",
      "injection-regex": "^alder$"
    }
  ],
  "metadata": {
    "version": "0.1.0",
    "license": "Apache-2.0",
    "description": "Alder grammar for tree-sitter",
    "links": { "repository": "https://github.com/tapegram/alder" }
  },
  "bindings": { "c": false, "go": false, "node": false, "python": false, "rust": false, "swift": false }
}
```

Also create an empty `tree-sitter-alder/queries/highlights.scm` (Task 6 fills it; the CLI wants the file to exist).

- [ ] **Step 2: Write `tree-sitter-alder/.gitignore`**

```
# outputs of `tree-sitter build` and nvim-treesitter's compile step (it compiles in place when
# alder.nvim's `path` points at this checkout)
parser.so
*.so
*.dylib
*.dll
*.wasm
*.o
log.html
```

- [ ] **Step 3: Write `scripts/ts.sh`** (and `chmod +x scripts/ts.sh`)

```sh
#!/bin/sh
# scripts/ts.sh generate|test|parse [args…] — the tree-sitter CLI for tree-sitter-alder/.
# generate and test run inside tree-sitter-alder/; parse runs from the current directory with
# -p, so file arguments are relative to where you are. Only the CLI version CI pins may
# regenerate src/: another version writes a different parser.c and the regeneration check fails.
set -eu
want=0.27.1
grammar=$(cd "$(dirname "$0")/../tree-sitter-alder" && pwd)
command -v tree-sitter >/dev/null 2>&1 || { echo "tree-sitter CLI not found: brew install tree-sitter-cli" >&2; exit 2; }
cmd=${1:-}
[ $# -gt 0 ] && shift
case "$cmd" in
  generate)
    have=$(tree-sitter --version | awk '{print $2}')
    if [ "$have" != "$want" ]; then
      echo "tree-sitter $have: src/ must be generated with $want, the version CI pins (.github/install-tree-sitter.sh)" >&2
      exit 2
    fi
    (cd "$grammar" && TREE_SITTER_JS_RUNTIME=native tree-sitter generate "$@") ;;
  test)
    (cd "$grammar" && tree-sitter test "$@") ;;
  parse)
    tree-sitter parse -p "$grammar" "$@" ;;
  *)
    echo "usage: scripts/ts.sh generate|test|parse [args…]" >&2
    exit 2 ;;
esac
```

- [ ] **Step 4: Write `tree-sitter-alder/grammar.js`**

```js
// tree-sitter-alder/grammar.js
//
// A second, independent description of Alder's syntax. It mirrors internal/syntax (the Go
// parser), not book/A-grammar.md. Newlines, HTML text, raw text and strings come from
// src/scanner.c, which ports the Go lexer's rules. Policy: accept everything the compiler
// accepts, perhaps a little more; diagnostics are the compiler's job.

const PREC = {
  body: -1, // lambda and if/else bodies swallow everything to their right
  pipe: 1, // |>
  default: 2, // ??  right-associative, looser than || (parser_expr.go parseDefault)
  or: 3, // ||
  and: 4, // &&
  compare: 5, // == != < <= > >=  (non-associative in the compiler; accepted left here)
  add: 6, // + - ++
  mul: 7, // * / %
  app: 8, // f a b
  postfix: 9, // x.field
};

const sep1 = (rule, separator) => seq(rule, repeat(seq(separator, rule)));
const commaSep1 = (rule) => seq(sep1(rule, ','), optional(','));
const commaSep = (rule) => optional(commaSep1(rule));

// Keywords that the Go parser accepts as deploy field names (`requires:` and the rest).
const TRUE_KEYWORDS = ['brand', 'where', 'let', 'match', 'if', 'then', 'else', 'for', 'in', 'requires'];

module.exports = grammar({
  name: 'alder',

  word: ($) => $.identifier,

  externals: ($) => [
    $._newline, // scanner.c scan_newline: a separator, not emitted before an operator-led line
    $.string, // scanner.c scan_string: one token including its {…} interpolations
    $.html_text, // scanner.c scan_html_text: element text up to `<`+letter, `{` or a comment line
    $.raw_text, // scanner.c scan_raw_text: <script>/<style> bodies
    $._raw_tag_name, // scanner.c scan_raw_tag_name: `script` or `style` right after `<`
    $._error_sentinel, // never in a rule: valid only during error recovery
  ],

  extras: ($) => [/[ \t\r\n]/, $.comment],

  conflicts: ($) => [
    // `x : T` NEWLINE `x = …` is one value declaration; in a signature file the next line may
    // be another signature. Both stay alive until the token after the name (`:` or not).
    [$.value_decl],
  ],

  rules: {
    source_file: ($) =>
      seq(optional($._newline), $.module_decl, repeat(seq($._newline, $._item)), optional($._newline)),

    _item: ($) => choice($.import_decl, $._declaration),

    module_decl: ($) => seq('module', field('name', $.module_name)),
    import_decl: ($) => seq('import', field('module', $.module_name)),

    // Module, type and component names: dotted uppercase segments (`Auth.Login`).
    module_name: ($) => seq($.constructor, repeat(seq('.', $.constructor))),

    _declaration: ($) =>
      choice(
        $.value_decl,
        $.type_decl,
        $.page_decl,
        $.msg_decl,
        $.form_decl,
        $.table_decl,
        $.capability_decl,
        $.fake_decl,
        $.session_decl,
        $.login_decl,
        $.test_decl,
        $.deploy_decl,
        $.silo_decl,
        $.choose_decl,
      ),

    // ---- values -------------------------------------------------------------------------

    value_decl: ($) =>
      choice(
        prec.dynamic(1, seq(field('signature', $.signature), $._newline, field('binding', $.binding))),
        field('signature', $.signature),
        field('binding', $.binding),
      ),

    signature: ($) => seq(field('name', $._value_name), ':', field('type', $._type)),

    binding: ($) =>
      seq(field('name', $._value_name), optional(field('params', $.parameters)), '=', field('body', $._expression)),

    parameters: ($) => repeat1($.identifier),

    // Components are uppercase values; stdlib signature files name values after keywords.
    _value_name: ($) => choice($.identifier, $.constructor, alias(choice('where', 'fake', 'login'), $.identifier)),

    // ---- types --------------------------------------------------------------------------

    type_decl: ($) =>
      seq(
        'type',
        field('name', $.constructor),
        repeat(field('param', alias($.identifier, $.type_var))),
        optional(seq('=', field('body', choice($.brand_type, $.record_type, $.union_type)))),
      ),

    union_type: ($) => seq(optional('|'), sep1($.variant, '|')),
    variant: ($) => prec.right(seq(field('name', $.constructor), optional(field('argument', $._type_atom)))),

    brand_type: ($) =>
      prec.right(seq('brand', field('base', $.constructor), optional(seq('where', field('check', $._expression))))),

    _type: ($) => choice($.type_arrow, $._type_term),
    type_arrow: ($) => prec.right(seq(field('parameter', $._type_term), '->', field('result', $._type))),
    _type_term: ($) => choice($.type_app, $._type_atom),
    type_app: ($) => prec.left(seq(field('constructor', $.type_name), repeat1(field('argument', $._type_atom)))),
    _type_atom: ($) =>
      choice($.type_name, alias($.identifier, $.type_var), $.record_type, $.unit_type, $.parenthesized_type),
    type_name: ($) => prec.left(seq($.constructor, repeat(seq('.', $.constructor)))),
    unit_type: ($) => seq('(', ')'),
    parenthesized_type: ($) => seq('(', $._type, ')'),

    record_type: ($) => seq('{', commaSep($.field_decl), '}'),

    // One field shape for records, forms, tables, capabilities and the session: `via` is legal
    // only in the session and attributes only in tables, which the compiler checks.
    field_decl: ($) =>
      seq(
        field('name', $.identifier),
        ':',
        field('type', $._type),
        optional(seq('via', field('via', $.constructor))),
        optional(seq('=', field('default', $._expression))),
        repeat(field('attribute', $.field_attribute)),
      ),

    // @id @created @unique @owner @cascade … ; @ref T ; @partition key ; @renamed "old"
    field_attribute: ($) =>
      prec.right(
        seq('@', field('name', $.identifier), optional(field('argument', choice($.type_name, $.identifier, $.string)))),
      ),

    // ---- declarations -------------------------------------------------------------------

    page_decl: ($) =>
      seq('page', field('path', $.string), field('params', $.record_type), optional(field('requires', $.requires_clause))),

    requires_clause: ($) =>
      seq('requires', sep1(choice(alias($.identifier, $.session_key), $.role_clause, $.silo_clause), ',')),
    role_clause: ($) => seq('role', sep1(field('role', $.constructor), '|')),
    silo_clause: ($) => seq('silo', field('key', $.identifier)),

    msg_decl: ($) =>
      seq(
        'msg',
        field('name', $.constructor),
        optional(field('params', $.record_type)),
        optional(seq('form', field('form', $.constructor))),
      ),

    form_decl: ($) => seq('form', field('name', $.constructor), '=', field('fields', $.record_type)),

    table_decl: ($) =>
      seq('table', field('name', $.constructor), repeat(field('attribute', $.field_attribute)), field('fields', $.record_type)),

    capability_decl: ($) => seq('capability', field('name', $.constructor), field('entries', $.record_type)),

    fake_decl: ($) => seq('fake', field('name', $.constructor), field('body', $.fake_body)),
    fake_body: ($) => seq('{', repeat(choice($._newline, $.table_decl, $.fake_entry)), '}'),
    fake_entry: ($) =>
      seq(field('name', $.identifier), optional(field('params', $.parameters)), '=', field('body', $._expression)),

    session_decl: ($) => seq('session', field('fields', $.record_type)),

    login_decl: ($) => seq('login', '=', field('page', $.module_name)),

    test_decl: ($) => seq('test', field('name', $.string), '=', field('body', $._expression)),

    deploy_decl: ($) => seq('deploy', field('body', $.deploy_record)),
    deploy_record: ($) => seq('{', commaSep($.deploy_field), '}'),
    deploy_field: ($) =>
      seq(
        field('name', choice($.identifier, alias(choice(...TRUE_KEYWORDS), $.identifier))),
        ':',
        field('value', $._deploy_value),
      ),
    _deploy_value: ($) => choice($.string, $.integer, $.float, $.negative_number, $.deploy_list, $.deploy_target),
    deploy_list: ($) => seq('[', commaSep($._deploy_value), ']'),
    deploy_target: ($) =>
      prec.right(
        seq(field('kind', $.constructor), optional(field('argument', $.string)), optional(field('options', $.deploy_record))),
      ),

    silo_decl: ($) => seq('silo', field('key', $.identifier), '{', commaSep(field('table', $.constructor)), '}'),

    choose_decl: ($) => seq('choose', field('key', $.identifier), '=', field('page', $.module_name)),

    // ---- expressions --------------------------------------------------------------------

    _expression: ($) =>
      choice(
        $.pipe_expr,
        $.default_expr,
        $.or_expr,
        $.and_expr,
        $.comparison_expr,
        $.additive_expr,
        $.multiplicative_expr,
        $.application,
        $._term,
      ),

    pipe_expr: ($) => prec.left(PREC.pipe, seq(field('left', $._expression), '|>', field('right', $._expression))),
    default_expr: ($) => prec.right(PREC.default, seq(field('left', $._expression), '??', field('right', $._expression))),
    or_expr: ($) => prec.left(PREC.or, seq(field('left', $._expression), '||', field('right', $._expression))),
    and_expr: ($) => prec.left(PREC.and, seq(field('left', $._expression), '&&', field('right', $._expression))),
    comparison_expr: ($) =>
      prec.left(
        PREC.compare,
        seq(field('left', $._expression), choice('==', '!=', '<', '<=', '>', '>='), field('right', $._expression)),
      ),
    additive_expr: ($) =>
      prec.left(PREC.add, seq(field('left', $._expression), choice('+', '-', '++'), field('right', $._expression))),
    multiplicative_expr: ($) =>
      prec.left(PREC.mul, seq(field('left', $._expression), choice('*', '/', '%'), field('right', $._expression))),

    // Arguments sit on the function's line: a newline ends the application wherever the
    // grammar expects a separator (scanner.c). `if`, `match`, elements and negative numbers
    // are not arguments; a lambda is, and its body runs to the end.
    application: ($) =>
      prec.left(PREC.app, seq(field('function', $._simple), repeat1(field('argument', $._argument)))),

    _term: ($) => choice($._argument, $.negative_number, $.if_expr, $.match_expr, $._html),
    _argument: ($) => choice($._simple, $.lambda, $.record, $.record_update, $.block),
    _simple: ($) =>
      choice(
        $.identifier,
        $.constructor,
        $.qualified_name,
        $.field_access,
        $.integer,
        $.float,
        $.string,
        $.boolean,
        $.unit,
        $.hole,
        $.list,
        $.parenthesized_expression,
      ),

    // `Db.from`, `Todo.TodoId`, `A.B.c`: a module segment then a value or constructor.
    qualified_name: ($) =>
      prec.right(
        seq(field('module', $.constructor), '.', field('name', choice($.identifier, $.constructor, $.qualified_name))),
      ),

    // `post.title`, `Post.slug.eq`, `(f x).y`; never on a bare constructor.
    field_access: ($) =>
      prec(
        PREC.postfix,
        seq(
          field(
            'value',
            choice(
              $.identifier,
              $.qualified_name,
              $.field_access,
              $.parenthesized_expression,
              $.record,
              $.record_update,
              $.block,
              $.hole,
            ),
          ),
          '.',
          field('field', $.identifier),
        ),
      ),

    lambda: ($) => prec.right(PREC.body, seq('\\', repeat(field('param', $.identifier)), '->', field('body', $._expression))),

    if_expr: ($) =>
      prec.right(
        PREC.body,
        seq(
          'if',
          field('condition', $._expression),
          'then',
          field('consequence', $._expression),
          'else',
          field('alternative', $._expression),
        ),
      ),

    // The subject takes no record argument, so `match f x {` opens the arms (the Go parser's
    // noBrace rule).
    match_expr: ($) => seq('match', field('subject', $._subject), '{', commaSep($.arm), '}'),
    _subject: ($) => choice($._simple, alias($._subject_application, $.application)),
    _subject_application: ($) =>
      prec.left(PREC.app, seq(field('function', $._simple), repeat1(field('argument', choice($._simple, $.lambda))))),
    arm: ($) => seq(field('pattern', $._pattern), '->', field('body', $._expression)),

    record: ($) => seq('{', commaSep($.field_init), '}'),
    field_init: ($) => seq(field('name', $.identifier), ':', field('value', $._expression)),
    record_update: ($) => seq('{', field('record', $._expression), '|', commaSep1($.field_init), '}'),

    block: ($) => seq('{', repeat(choice($.let_stmt, $.bind_stmt)), field('result', $._expression), '}'),
    let_stmt: ($) =>
      seq(
        'let',
        field('name', choice($.identifier, $.wildcard)),
        optional(seq(':', field('type', $._type))),
        '=',
        field('value', $._expression),
        $._newline,
      ),
    bind_stmt: ($) =>
      seq(
        'let',
        field('name', choice($.identifier, $.wildcard)),
        optional(seq(':', field('type', $._type))),
        '<-',
        field('value', $._expression),
        $._newline,
      ),

    list: ($) => seq('[', commaSep($._expression), ']'),
    parenthesized_expression: ($) => seq('(', $._expression, ')'),
    unit: ($) => seq('(', ')'),

    negative_number: ($) => seq('-', choice($.integer, $.float)),
    integer: ($) => /[0-9]+/,
    float: ($) => /[0-9]+\.[0-9]+/,
    boolean: ($) => choice('True', 'False'),
    hole: ($) => /\?[a-zA-Z0-9_]*/,

    identifier: ($) => /[a-z][a-zA-Z0-9_]*|_[a-zA-Z0-9_]+/,
    constructor: ($) => /[A-Z][a-zA-Z0-9_]*/,

    // ---- patterns -----------------------------------------------------------------------

    _pattern: ($) =>
      choice(
        $.wildcard,
        $.identifier,
        $.integer,
        $.negative_number,
        $.string,
        $.boolean,
        $.constructor_pattern,
        $.record_pattern,
        $.list_pattern,
        $.parenthesized_pattern,
      ),
    wildcard: ($) => '_',
    constructor_pattern: ($) =>
      prec.right(seq(field('constructor', $.constructor), optional(field('argument', $._pattern_argument)))),
    _pattern_argument: ($) =>
      choice(
        $.wildcard,
        $.identifier,
        $.integer,
        $.string,
        $.boolean,
        alias($._bare_constructor_pattern, $.constructor_pattern),
        $.record_pattern,
        $.parenthesized_pattern,
      ),
    _bare_constructor_pattern: ($) => field('constructor', $.constructor),
    record_pattern: ($) => seq('{', commaSep($.field_pattern), '}'),
    field_pattern: ($) => seq(field('name', $.identifier), ':', field('pattern', $._pattern)),
    list_pattern: ($) =>
      seq(
        '[',
        optional(choice(seq(sep1($._pattern, ','), optional(seq(',', optional($.rest_pattern)))), $.rest_pattern)),
        ']',
      ),
    rest_pattern: ($) => seq('..', field('name', $.identifier)),
    parenthesized_pattern: ($) => seq('(', $._pattern, ')'),

    // ---- HTML ---------------------------------------------------------------------------

    _html: ($) => choice($.element, $.self_closing_element, $.raw_element),

    element: ($) => seq(field('open', $.start_tag), repeat($._html_child), field('close', $.end_tag)),
    start_tag: ($) => seq('<', field('tag', $._tag_name), repeat($.attribute), '>'),
    end_tag: ($) => seq('</', field('tag', $._tag_name), '>'),
    self_closing_element: ($) => seq('<', field('tag', $._tag_name), repeat($.attribute), '/', '>'),

    raw_element: ($) =>
      choice(
        seq(
          '<',
          field('tag', alias($._raw_tag_name, $.tag_name)),
          repeat($.attribute),
          '>',
          optional($.raw_text),
          '</',
          field('close', $._tag_name),
          '>',
        ),
        seq('<', field('tag', alias($._raw_tag_name, $.tag_name)), repeat($.attribute), '/', '>'),
      ),

    // Lowercase names may contain `-` (custom elements) and may be keywords (`<table>`,
    // `<form>`): this token is not the word token, so keywords are never extracted from it.
    _tag_name: ($) => choice(alias($._html_name, $.tag_name), alias($.module_name, $.component_name)),
    _html_name: ($) => /[a-z][a-zA-Z0-9_-]*/,

    attribute: ($) =>
      seq(
        field('name', alias($._html_name, $.attribute_name)),
        optional(seq('=', field('value', choice($.string, $.interpolation)))),
      ),

    _html_child: ($) => choice($.html_text, $._html, $.interpolation, $.html_for, $.html_if, $.html_match),

    interpolation: ($) => seq('{', field('expression', $._expression), '}'),

    // `for`, `if` and `match` are special only as the first token inside a content brace.
    html_for: ($) =>
      seq(
        '{',
        'for',
        field('binder', $.identifier),
        'in',
        field('items', $._expression),
        '->',
        field('body', $._expression),
        '}',
      ),
    html_if: ($) =>
      prec(
        1,
        seq(
          '{',
          'if',
          field('condition', $._expression),
          'then',
          field('consequence', $._expression),
          'else',
          field('alternative', $._expression),
          '}',
        ),
      ),
    html_match: ($) => prec(1, seq('{', 'match', field('subject', $._subject), '{', commaSep($.arm), '}', '}')),

    comment: ($) => token(seq('--', /.*/)),
  },
});
```

- [ ] **Step 5: Write `tree-sitter-alder/src/scanner.c`**

```c
// tree-sitter-alder/src/scanner.c
//
// The external scanner: the Go lexer's context-sensitive rules, each ported from the function
// named in its comment (internal/syntax/lexer.go, parser_expr.go). Stateless.

#include "tree_sitter/parser.h"

#include <stdbool.h>
#include <stdint.h>
#include <string.h>

enum TokenType {
  NEWLINE,
  STRING,
  HTML_TEXT,
  RAW_TEXT,
  RAW_TAG_NAME,
  ERROR_SENTINEL,
};

void *tree_sitter_alder_external_scanner_create(void) { return NULL; }
void tree_sitter_alder_external_scanner_destroy(void *payload) { (void)payload; }
unsigned tree_sitter_alder_external_scanner_serialize(void *payload, char *buffer) {
  (void)payload;
  (void)buffer;
  return 0;
}
void tree_sitter_alder_external_scanner_deserialize(void *payload, const char *buffer, unsigned length) {
  (void)payload;
  (void)buffer;
  (void)length;
}

static void advance(TSLexer *lexer) { lexer->advance(lexer, false); }
static void skip(TSLexer *lexer) { lexer->advance(lexer, true); }
static bool is_blank(int32_t c) { return c == ' ' || c == '\t' || c == '\r'; }
static bool is_letter(int32_t c) { return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z'); }
static bool is_name_char(int32_t c) {
  return is_letter(c) || (c >= '0' && c <= '9') || c == '_' || c == '-';
}

// ---- strings: lexer.go scanString / scanInterp -------------------------------------------

static bool scan_interp(TSLexer *lexer);

// scan_string_body is scanString after the opening quote: a backslash skips the next byte, a
// quote ends the string, `{` opens an interpolation, and a newline or EOF means the string is
// unterminated, which is not a string token (the compiler reports it).
static bool scan_string_body(TSLexer *lexer) {
  for (;;) {
    if (lexer->eof(lexer) || lexer->lookahead == '\n') return false;
    int32_t c = lexer->lookahead;
    if (c == '\\') {
      advance(lexer);
      if (!lexer->eof(lexer) && lexer->lookahead != '\n') advance(lexer);
      continue;
    }
    if (c == '"') {
      advance(lexer);
      return true;
    }
    if (c == '{') {
      advance(lexer);
      if (!scan_interp(lexer)) return false;
      continue;
    }
    advance(lexer);
  }
}

// scan_interp is scanInterp after `{`: nested strings recurse, braces nest, and a newline
// fails the interpolation (and so the string). Backslash is not special here.
static bool scan_interp(TSLexer *lexer) {
  int depth = 1;
  for (;;) {
    if (lexer->eof(lexer) || lexer->lookahead == '\n') return false;
    int32_t c = lexer->lookahead;
    if (c == '"') {
      advance(lexer);
      if (!scan_string_body(lexer)) return false;
      continue;
    }
    if (c == '{') depth++;
    if (c == '}') {
      depth--;
      if (depth == 0) {
        advance(lexer);
        return true;
      }
    }
    advance(lexer);
  }
}

// ---- newlines: parser_expr.go atOpAfterNL / ltIsOperator ---------------------------------

// is_continuation reports whether the first character of a line (the lexer's lookahead) begins
// a binary operator that continues the previous line: |> || ?? && == != < <= > >= + - ++ * / %.
// Also `|` (union variants) and a lone `=` (a type's `=` on the next line), which can never
// begin a declaration. `<` continues only when whitespace follows it; `<-` and `->` never do.
static bool is_continuation(TSLexer *lexer) {
  switch (lexer->lookahead) {
    case '|':
    case '&':
    case '=':
    case '>':
    case '+':
    case '*':
    case '/':
    case '%':
      return true;
    case '!':
      advance(lexer);
      return lexer->lookahead == '=';
    case '?':
      advance(lexer);
      return lexer->lookahead == '?';
    case '<':
      advance(lexer);
      if (lexer->lookahead == '=') return true;
      return lexer->lookahead == ' ' || lexer->lookahead == '\t' || lexer->lookahead == '\r' ||
             lexer->lookahead == '\n';
    case '-':
      advance(lexer);
      return lexer->lookahead != '>';  // `--` comment lines were skipped by the caller
    default:
      return false;
  }
}

// scan_newline runs at '\n' where the grammar accepts a separator. The token is the newline
// plus any blank lines after it; it stops before a comment line so the comment stays a node.
// It then looks past blank and comment lines: if the next code line starts with an operator
// the newline is not a separator (no token; the newline is whitespace). During error recovery
// it only separates before a line that starts at column 1, which is a declaration boundary.
static bool scan_newline(TSLexer *lexer, bool recovering) {
  advance(lexer);
  lexer->mark_end(lexer);
  bool seen_comment = false;
  for (;;) {
    while (is_blank(lexer->lookahead)) advance(lexer);
    if (lexer->eof(lexer)) break;
    if (lexer->lookahead == '\n') {
      advance(lexer);
      if (!seen_comment) lexer->mark_end(lexer);
      continue;
    }
    if (lexer->lookahead == '-') {
      advance(lexer);
      if (lexer->lookahead == '-') {
        seen_comment = true;
        while (!lexer->eof(lexer) && lexer->lookahead != '\n') advance(lexer);
        continue;
      }
      // a line starting with `-` (not `->`) continues: subtraction
      if (recovering) return false;
      if (lexer->lookahead == '>') break;
      return false;
    }
    if (recovering && lexer->get_column(lexer) != 0) return false;
    if (is_continuation(lexer)) return false;
    break;
  }
  lexer->result_symbol = NEWLINE;
  return true;
}

// ---- HTML: lexer.go ScanText / ScanRawText, parser_html.go addText -----------------------

// scan_html_text reads element content up to `{`, a `<` followed by a letter, `/` or `!`, or
// EOF. It stops before a newline whose next non-blank line starts with `--` (a comment inside
// markup, parser_html.go addText) so the comment is lexed as a comment. It also stops before a
// newline whose next line starts in column 1 with anything but `<`, `{` or whitespace: element
// content is indented in practice, and stopping there keeps an unclosed tag from turning the
// rest of the file into text while code is being typed (the drift test shows no valid file
// relies on column-1 text).
static bool scan_html_text(TSLexer *lexer) {
  bool has_text = false;
  for (;;) {
    if (lexer->eof(lexer)) break;
    int32_t c = lexer->lookahead;
    if (c == '{') break;
    if (c == '<') {
      lexer->mark_end(lexer);
      advance(lexer);
      int32_t d = lexer->lookahead;
      if (is_letter(d) || d == '/' || d == '!') {
        lexer->result_symbol = HTML_TEXT;
        return has_text;
      }
      has_text = true;
      continue;
    }
    if (c == '\n') {
      lexer->mark_end(lexer);
      advance(lexer);
      int32_t first = lexer->lookahead;
      if (!lexer->eof(lexer) && !is_blank(first) && first != '\n' && first != '<' && first != '{') {
        lexer->result_symbol = HTML_TEXT;
        return has_text;
      }
      while (is_blank(lexer->lookahead)) advance(lexer);
      if (lexer->lookahead == '-') {
        advance(lexer);
        if (lexer->lookahead == '-') {
          lexer->result_symbol = HTML_TEXT;
          return has_text;
        }
      }
      has_text = true;
      continue;
    }
    advance(lexer);
    has_text = true;
  }
  lexer->mark_end(lexer);
  lexer->result_symbol = HTML_TEXT;
  return has_text;
}

// scan_raw_text reads a <script>/<style> body up to `</script` or `</style` (ASCII case-
// insensitive) followed by `>`, whitespace, `/` or EOF. Either closer ends either body, which
// accepts a little more than the compiler.
static bool scan_raw_text(TSLexer *lexer) {
  bool has_text = false;
  for (;;) {
    if (lexer->eof(lexer)) break;
    if (lexer->lookahead != '<') {
      advance(lexer);
      has_text = true;
      continue;
    }
    lexer->mark_end(lexer);
    advance(lexer);
    if (lexer->lookahead != '/') {
      has_text = true;
      continue;
    }
    advance(lexer);
    char name[8];
    int n = 0;
    while (n < 7 && is_letter(lexer->lookahead)) {
      int32_t c = lexer->lookahead;
      name[n++] = (char)(c >= 'A' && c <= 'Z' ? c + 32 : c);
      advance(lexer);
    }
    name[n] = 0;
    int32_t after = lexer->lookahead;
    bool closer = (strcmp(name, "script") == 0 || strcmp(name, "style") == 0) &&
                  (lexer->eof(lexer) || after == '>' || after == '/' || is_blank(after) || after == '\n');
    if (closer) {
      lexer->result_symbol = RAW_TEXT;
      return has_text;
    }
    has_text = true;
  }
  lexer->mark_end(lexer);
  lexer->result_symbol = RAW_TEXT;
  return has_text;
}

// scan_raw_tag_name recognises `script` or `style` (exactly, lowercase) right after `<`, the
// tags whose bodies are raw text (parser_html.go).
static bool scan_raw_tag_name(TSLexer *lexer) {
  char name[8];
  int n = 0;
  while (n < 7 && lexer->lookahead >= 'a' && lexer->lookahead <= 'z') {
    name[n++] = (char)lexer->lookahead;
    advance(lexer);
  }
  name[n] = 0;
  if (is_name_char(lexer->lookahead)) return false;
  if (strcmp(name, "script") != 0 && strcmp(name, "style") != 0) return false;
  lexer->mark_end(lexer);
  lexer->result_symbol = RAW_TAG_NAME;
  return true;
}

bool tree_sitter_alder_external_scanner_scan(void *payload, TSLexer *lexer, const bool *valid) {
  (void)payload;
  bool recovering = valid[ERROR_SENTINEL];

  // Text tokens include their leading whitespace, so they are tried before skipping it, and
  // never during error recovery, where they would swallow code.
  if (!recovering) {
    if (valid[RAW_TEXT]) return scan_raw_text(lexer);
    if (valid[HTML_TEXT]) return scan_html_text(lexer);
    if (valid[RAW_TAG_NAME] && lexer->lookahead >= 'a' && lexer->lookahead <= 'z') {
      return scan_raw_tag_name(lexer);
    }
  }

  while (is_blank(lexer->lookahead)) skip(lexer);

  if (valid[NEWLINE] && lexer->lookahead == '\n') return scan_newline(lexer, recovering);

  if (valid[STRING]) {
    while (is_blank(lexer->lookahead) || lexer->lookahead == '\n') skip(lexer);
    if (lexer->lookahead == '"') {
      advance(lexer);
      if (scan_string_body(lexer)) {
        lexer->result_symbol = STRING;
        return true;
      }
    }
  }
  return false;
}
```

- [ ] **Step 6: Generate**

Run: `scripts/ts.sh generate`
Expected: exit 0, and `tree-sitter-alder/src/parser.c` exists. If generate reports a conflict, resolve it with the precedence the rule's comment names (`PREC`, `prec.right` for "swallows to the right", `prec.dynamic` for a genuine two-parse moment). Add to `conflicts` only when both parses are legal Alder up to that token, and give each entry a comment like the `value_decl` one. Record every conflict you add in the task report.

- [ ] **Step 7: Write the core corpus**

`tree-sitter-alder/test/corpus/module.txt`:

```
================================================================================
module, import, signature and binding
================================================================================

module Auth.Login

import User

x : Int
x = 1

--------------------------------------------------------------------------------

(source_file
  (module_decl
    name: (module_name
      (constructor)
      (constructor)))
  (import_decl
    module: (module_name
      (constructor)))
  (value_decl
    signature: (signature
      name: (identifier)
      type: (type_name
        (constructor)))
    binding: (binding
      name: (identifier)
      body: (integer))))

================================================================================
a component is an uppercase value with parameters
================================================================================

module Ui

Card props = props

--------------------------------------------------------------------------------

(source_file
  (module_decl
    name: (module_name
      (constructor)))
  (value_decl
    binding: (binding
      name: (constructor)
      params: (parameters
        (identifier))
      body: (identifier))))

================================================================================
function types and type application
================================================================================

module M

bySlug : Slug -> Query (Maybe Post)
bySlug slug = slug

--------------------------------------------------------------------------------

(source_file
  (module_decl
    name: (module_name
      (constructor)))
  (value_decl
    signature: (signature
      name: (identifier)
      type: (type_arrow
        parameter: (type_name
          (constructor))
        result: (type_app
          constructor: (type_name
            (constructor))
          argument: (parenthesized_type
            (type_app
              constructor: (type_name
                (constructor))
              argument: (type_name
                (constructor)))))))
    binding: (binding
      name: (identifier)
      params: (parameters
        (identifier))
      body: (identifier))))
```

`tree-sitter-alder/test/corpus/newlines.txt`:

```
================================================================================
a newline ends an application
================================================================================

module M

a = f x
b = g

--------------------------------------------------------------------------------

(source_file
  (module_decl
    name: (module_name
      (constructor)))
  (value_decl
    binding: (binding
      name: (identifier)
      body: (application
        function: (identifier)
        argument: (identifier))))
  (value_decl
    binding: (binding
      name: (identifier)
      body: (identifier))))

================================================================================
an operator at the start of a line continues the expression
================================================================================

module M

both n =
  n > 0
    && n < 10
    || n == 20

--------------------------------------------------------------------------------

(source_file
  (module_decl
    name: (module_name
      (constructor)))
  (value_decl
    binding: (binding
      name: (identifier)
      params: (parameters
        (identifier))
      body: (or_expr
        left: (and_expr
          left: (comparison_expr
            left: (identifier)
            right: (integer))
          right: (comparison_expr
            left: (identifier)
            right: (integer)))
        right: (comparison_expr
          left: (identifier)
          right: (integer))))))

================================================================================
continuation across blank and comment lines; a pipe chain
================================================================================

module M

total xs =
  xs

    -- keep the evens
    |> List.filter isEven
    |> List.sum

--------------------------------------------------------------------------------

(source_file
  (module_decl
    name: (module_name
      (constructor)))
  (value_decl
    binding: (binding
      name: (identifier)
      params: (parameters
        (identifier))
      body: (pipe_expr
        left: (pipe_expr
          left: (identifier)
          (comment)
          right: (application
            function: (qualified_name
              module: (constructor)
              name: (identifier))
            argument: (identifier)))
        right: (qualified_name
          module: (constructor)
          name: (identifier))))))

================================================================================
a line starting with < and a letter is not a comparison
================================================================================

module M

v = {
  let n = 1
  <p>hi</p>
}

--------------------------------------------------------------------------------

(source_file
  (module_decl
    name: (module_name
      (constructor)))
  (value_decl
    binding: (binding
      name: (identifier)
      body: (block
        (let_stmt
          name: (identifier)
          value: (integer))
        result: (element
          open: (start_tag
            tag: (tag_name))
          (html_text)
          close: (end_tag
            tag: (tag_name)))))))

================================================================================
a line starting with < and a space is a comparison
================================================================================

module M

small n =
  n
    < 10

--------------------------------------------------------------------------------

(source_file
  (module_decl
    name: (module_name
      (constructor)))
  (value_decl
    binding: (binding
      name: (identifier)
      params: (parameters
        (identifier))
      body: (comparison_expr
        left: (identifier)
        right: (integer)))))
```

`tree-sitter-alder/test/corpus/expressions.txt`:

```
================================================================================
precedence: |> lowest, ?? below ||, then && comparison + *
================================================================================

module M

x = a |> f ?? b || c && d == e + f * g

--------------------------------------------------------------------------------

(source_file
  (module_decl
    name: (module_name
      (constructor)))
  (value_decl
    binding: (binding
      name: (identifier)
      body: (pipe_expr
        left: (identifier)
        right: (default_expr
          left: (identifier)
          right: (or_expr
            left: (identifier)
            right: (and_expr
              left: (identifier)
              right: (comparison_expr
                left: (identifier)
                right: (additive_expr
                  left: (identifier)
                  right: (multiplicative_expr
                    left: (identifier)
                    right: (identifier)))))))))))

================================================================================
?? is right-associative
================================================================================

module M

y = a ?? b ?? c

--------------------------------------------------------------------------------

(source_file
  (module_decl
    name: (module_name
      (constructor)))
  (value_decl
    binding: (binding
      name: (identifier)
      body: (default_expr
        left: (identifier)
        right: (default_expr
          left: (identifier)
          right: (identifier))))))

================================================================================
application, qualified names and field access
================================================================================

module M

z = Post.slug.eq slug
w = Todo.TodoId "x"
u = f a b

--------------------------------------------------------------------------------

(source_file
  (module_decl
    name: (module_name
      (constructor)))
  (value_decl
    binding: (binding
      name: (identifier)
      body: (application
        function: (field_access
          value: (qualified_name
            module: (constructor)
            name: (identifier))
          field: (identifier))
        argument: (identifier))))
  (value_decl
    binding: (binding
      name: (identifier)
      body: (application
        function: (qualified_name
          module: (constructor)
          name: (constructor))
        argument: (string))))
  (value_decl
    binding: (binding
      name: (identifier)
      body: (application
        function: (identifier)
        argument: (identifier)
        argument: (identifier)))))

================================================================================
lambda, if, lists, negative numbers, holes, unit
================================================================================

module M

a = List.map (\t -> t.id) xs
b = if done then 1 else 2 + 3
c = [1, -1, ?count]
d = Task.succeed ()

--------------------------------------------------------------------------------

(source_file
  (module_decl
    name: (module_name
      (constructor)))
  (value_decl
    binding: (binding
      name: (identifier)
      body: (application
        function: (qualified_name
          module: (constructor)
          name: (identifier))
        argument: (parenthesized_expression
          (lambda
            param: (identifier)
            body: (field_access
              value: (identifier)
              field: (identifier))))
        argument: (identifier))))
  (value_decl
    binding: (binding
      name: (identifier)
      body: (if_expr
        condition: (identifier)
        consequence: (integer)
        alternative: (additive_expr
          left: (integer)
          right: (integer)))))
  (value_decl
    binding: (binding
      name: (identifier)
      body: (list
        (integer)
        (negative_number
          (integer))
        (hole))))
  (value_decl
    binding: (binding
      name: (identifier)
      body: (application
        function: (qualified_name
          module: (constructor)
          name: (identifier))
        argument: (unit)))))

================================================================================
keywords that are column-1 only are names elsewhere
================================================================================

module M

update msg form = match msg {
  _ -> form,
}

--------------------------------------------------------------------------------

(source_file
  (module_decl
    name: (module_name
      (constructor)))
  (value_decl
    binding: (binding
      name: (identifier)
      params: (parameters
        (identifier)
        (identifier))
      body: (match_expr
        subject: (identifier)
        (arm
          pattern: (wildcard)
          body: (identifier))))))
```

`tree-sitter-alder/test/corpus/strings.txt`:

```
================================================================================
interpolations with nested strings and braces are one token
================================================================================

module M

s = "a {f "-" x} b {{ n: 1 }.n} \" \{ done"

--------------------------------------------------------------------------------

(source_file
  (module_decl
    name: (module_name
      (constructor)))
  (value_decl
    binding: (binding
      name: (identifier)
      body: (string))))

================================================================================
a lone } and -- inside a string are text
================================================================================

module M

t = "} -- not a comment"

--------------------------------------------------------------------------------

(source_file
  (module_decl
    name: (module_name
      (constructor)))
  (value_decl
    binding: (binding
      name: (identifier)
      body: (string))))
```

`tree-sitter-alder/test/corpus/comments.txt`:

```
================================================================================
comments before the module, after code and between declarations
================================================================================

-- a header
module M

a = 1 -- trailing

-- between
b = 2

--------------------------------------------------------------------------------

(source_file
  (comment)
  (module_decl
    name: (module_name
      (constructor)))
  (value_decl
    binding: (binding
      name: (identifier)
      body: (integer)))
  (comment)
  (comment)
  (value_decl
    binding: (binding
      name: (identifier)
      body: (integer))))
```

- [ ] **Step 8: Run the corpus**

Run: `scripts/ts.sh test`
Expected: all cases pass. When one fails, decide from the node shape in this task's Interfaces and the Go parser which side is wrong. If the tree is right and the expectation is off only in where an extra (a `comment`) attaches, fix the expectation. Otherwise fix the grammar or scanner. **Never run `tree-sitter test --update`**: it turns whatever the parser does into the expectation.

- [ ] **Step 9: Commit**

```bash
git add tree-sitter-alder scripts/ts.sh
git commit -m "grammar: tree-sitter grammar and external scanner for Alder"
```

---

## Task 2: Blocks, records and match

**Files:**
- Create: `tree-sitter-alder/test/corpus/blocks.txt`, `tree-sitter-alder/test/corpus/match.txt`
- Modify: `tree-sitter-alder/grammar.js`, `src/scanner.c` (only as the cases require), regenerated `src/`

**Interfaces:**
- Consumes: the node names of Task 1.
- Produces: no new names; the shapes below are pinned.

- [ ] **Step 1: Write `test/corpus/blocks.txt`**

```
================================================================================
block with bind, typed let and a result
================================================================================

module M

b = {
  let x <- f
  let y : Int = 1
  Task.succeed x
}

--------------------------------------------------------------------------------

(source_file
  (module_decl
    name: (module_name
      (constructor)))
  (value_decl
    binding: (binding
      name: (identifier)
      body: (block
        (bind_stmt
          name: (identifier)
          value: (identifier))
        (let_stmt
          name: (identifier)
          type: (type_name
            (constructor))
          value: (integer))
        result: (application
          function: (qualified_name
            module: (constructor)
            name: (identifier))
          argument: (identifier))))))

================================================================================
record literal, empty record, record update, one-expression block
================================================================================

module M

r = { a: 1, b: x, }
e = Projects.Index {}
u = Session.put (\s -> { s | user: Nothing, role: Member })
o = { x }

--------------------------------------------------------------------------------

(source_file
  (module_decl
    name: (module_name
      (constructor)))
  (value_decl
    binding: (binding
      name: (identifier)
      body: (record
        (field_init
          name: (identifier)
          value: (integer))
        (field_init
          name: (identifier)
          value: (identifier)))))
  (value_decl
    binding: (binding
      name: (identifier)
      body: (application
        function: (qualified_name
          module: (constructor)
          name: (constructor))
        argument: (record))))
  (value_decl
    binding: (binding
      name: (identifier)
      body: (application
        function: (qualified_name
          module: (constructor)
          name: (identifier))
        argument: (parenthesized_expression
          (lambda
            param: (identifier)
            body: (record_update
              record: (identifier)
              (field_init
                name: (identifier)
                value: (constructor))
              (field_init
                name: (identifier)
                value: (constructor))))))))
  (value_decl
    binding: (binding
      name: (identifier)
      body: (block
        result: (identifier)))))

================================================================================
a let value continues on an operator-led line
================================================================================

module M

t = {
  let n = xs
    |> List.length
  n
}

--------------------------------------------------------------------------------

(source_file
  (module_decl
    name: (module_name
      (constructor)))
  (value_decl
    binding: (binding
      name: (identifier)
      body: (block
        (let_stmt
          name: (identifier)
          value: (pipe_expr
            left: (identifier)
            right: (qualified_name
              module: (constructor)
              name: (identifier))))
        result: (identifier)))))
```

- [ ] **Step 2: Write `test/corpus/match.txt`**

```
================================================================================
patterns: constructor with argument, nested, list with rest, record, wildcard
================================================================================

module M

m = match msg {
  Logout p -> 1,
  Just (Ok x) -> 2,
  Just Nothing -> 3,
  [a, ..rest] -> 4,
  { id: i } -> 5,
  -1 -> 6,
  _ -> 7,
}

--------------------------------------------------------------------------------

(source_file
  (module_decl
    name: (module_name
      (constructor)))
  (value_decl
    binding: (binding
      name: (identifier)
      body: (match_expr
        subject: (identifier)
        (arm
          pattern: (constructor_pattern
            constructor: (constructor)
            argument: (identifier))
          body: (integer))
        (arm
          pattern: (constructor_pattern
            constructor: (constructor)
            argument: (parenthesized_pattern
              (constructor_pattern
                constructor: (constructor)
                argument: (identifier))))
          body: (integer))
        (arm
          pattern: (constructor_pattern
            constructor: (constructor)
            argument: (constructor_pattern
              constructor: (constructor)))
          body: (integer))
        (arm
          pattern: (list_pattern
            (identifier)
            (rest_pattern
              name: (identifier)))
          body: (integer))
        (arm
          pattern: (record_pattern
            (field_pattern
              name: (identifier)
              pattern: (identifier)))
          body: (integer))
        (arm
          pattern: (negative_number
            (integer))
          body: (integer))
        (arm
          pattern: (wildcard)
          body: (integer))))))

================================================================================
the subject of match takes arguments but no record
================================================================================

module M

n = match Dict.get k d {
  Nothing -> 0,
  Just v -> v,
}

--------------------------------------------------------------------------------

(source_file
  (module_decl
    name: (module_name
      (constructor)))
  (value_decl
    binding: (binding
      name: (identifier)
      body: (match_expr
        subject: (application
          function: (qualified_name
            module: (constructor)
            name: (identifier))
          argument: (identifier)
          argument: (identifier))
        (arm
          pattern: (constructor_pattern
            constructor: (constructor))
          body: (integer))
        (arm
          pattern: (constructor_pattern
            constructor: (constructor)
            argument: (identifier))
          body: (identifier))))))

================================================================================
arm bodies may be blocks
================================================================================

module M

update msg session = match msg {
  -- logging out clears the user
  Logout p -> {
    let _ <- Session.put (\s -> { s | user: Nothing })
    Task.succeed (Redirect (Auth.Login {}))
  },
}

--------------------------------------------------------------------------------

(source_file
  (module_decl
    name: (module_name
      (constructor)))
  (value_decl
    binding: (binding
      name: (identifier)
      params: (parameters
        (identifier)
        (identifier))
      body: (match_expr
        subject: (identifier)
        (comment)
        (arm
          pattern: (constructor_pattern
            constructor: (constructor)
            argument: (identifier))
          body: (block
            (bind_stmt
              name: (wildcard)
              value: (application
                function: (qualified_name
                  module: (constructor)
                  name: (identifier))
                argument: (parenthesized_expression
                  (lambda
                    param: (identifier)
                    body: (record_update
                      record: (identifier)
                      (field_init
                        name: (identifier)
                        value: (constructor)))))))
            result: (application
              function: (qualified_name
                module: (constructor)
                name: (identifier))
              argument: (parenthesized_expression
                (application
                  function: (constructor)
                  argument: (parenthesized_expression
                    (application
                      function: (qualified_name
                        module: (constructor)
                        name: (constructor))
                      argument: (record))))))))))))
```

- [ ] **Step 3: Run**

Run: `scripts/ts.sh test`
Expected: Task 1's cases still pass. Any new failure is fixed as in Task 1 Step 8, then regenerate with `scripts/ts.sh generate`.

- [ ] **Step 4: Commit**

```bash
git add tree-sitter-alder
git commit -m "grammar: corpus for blocks, records and match"
```

---

## Task 3: Declarations and signature files

**Files:**
- Create: `tree-sitter-alder/test/corpus/declarations.txt`, `tree-sitter-alder/test/corpus/signatures.txt`
- Modify: `grammar.js` / `scanner.c` as the cases require, regenerated `src/`

**Interfaces:**
- Consumes: Task 1 node names. Produces: the declaration shapes below.

- [ ] **Step 1: Write `test/corpus/declarations.txt`**

```
================================================================================
types: brand with where, union with a payload, record, opaque
================================================================================

module M

type Slug = brand Text where (\s -> isSlug s)
type Filter = All | Active | Done (List Int)
type Model = { title: Text, n: Int }
type Status
  = Open
  | Closed

--------------------------------------------------------------------------------

(source_file
  (module_decl
    name: (module_name
      (constructor)))
  (type_decl
    name: (constructor)
    body: (brand_type
      base: (constructor)
      check: (parenthesized_expression
        (lambda
          param: (identifier)
          body: (application
            function: (identifier)
            argument: (identifier))))))
  (type_decl
    name: (constructor)
    body: (union_type
      (variant
        name: (constructor))
      (variant
        name: (constructor))
      (variant
        name: (constructor)
        argument: (parenthesized_type
          (type_app
            constructor: (type_name
              (constructor))
            argument: (type_name
              (constructor)))))))
  (type_decl
    name: (constructor)
    body: (record_type
      (field_decl
        name: (identifier)
        type: (type_name
          (constructor)))
      (field_decl
        name: (identifier)
        type: (type_name
          (constructor)))))
  (type_decl
    name: (constructor)
    body: (union_type
      (variant
        name: (constructor))
      (variant
        name: (constructor)))))

================================================================================
table with attributes, defaults and a renamed table
================================================================================

module Post

table Post @renamed "Article" {
  id: PostId @id,
  author: User.UserId @ref User @owner,
  org: Org.OrgId @ref Org @cascade @partition org,
  published: Bool = False,
}

--------------------------------------------------------------------------------

(source_file
  (module_decl
    name: (module_name
      (constructor)))
  (table_decl
    name: (constructor)
    attribute: (field_attribute
      name: (identifier)
      argument: (string))
    fields: (record_type
      (field_decl
        name: (identifier)
        type: (type_name
          (constructor))
        attribute: (field_attribute
          name: (identifier)))
      (field_decl
        name: (identifier)
        type: (type_name
          (constructor)
          (constructor))
        attribute: (field_attribute
          name: (identifier)
          argument: (type_name
            (constructor)))
        attribute: (field_attribute
          name: (identifier)))
      (field_decl
        name: (identifier)
        type: (type_name
          (constructor)
          (constructor))
        attribute: (field_attribute
          name: (identifier)
          argument: (type_name
            (constructor)))
        attribute: (field_attribute
          name: (identifier))
        attribute: (field_attribute
          name: (identifier)
          argument: (identifier)))
      (field_decl
        name: (identifier)
        type: (type_name
          (constructor))
        default: (boolean)))))

================================================================================
page with requires, msg, form
================================================================================

module Write.Edit

page "/write/{id}" { id: Post.PostId } requires user, role Author | Admin, silo org

msg Add form NewTodo
msg Toggle { id: Todo.TodoId }
form NewTodo = { title: Text.NonEmpty }

--------------------------------------------------------------------------------

(source_file
  (module_decl
    name: (module_name
      (constructor)
      (constructor)))
  (page_decl
    path: (string)
    params: (record_type
      (field_decl
        name: (identifier)
        type: (type_name
          (constructor)
          (constructor))))
    requires: (requires_clause
      (session_key)
      (role_clause
        role: (constructor)
        role: (constructor))
      (silo_clause
        key: (identifier))))
  (msg_decl
    name: (constructor)
    form: (constructor))
  (msg_decl
    name: (constructor)
    params: (record_type
      (field_decl
        name: (identifier)
        type: (type_name
          (constructor)
          (constructor)))))
  (form_decl
    name: (constructor)
    fields: (record_type
      (field_decl
        name: (identifier)
        type: (type_name
          (constructor)
          (constructor))))))

================================================================================
capability and its fake with a table
================================================================================

module Markdown

capability Markdown {
  render : Text -> Query (Html Never),
}

fake Markdown {
  table Seen { id: SeenId @id }
  render text = Query.succeed (Html.text text)
}

--------------------------------------------------------------------------------

(source_file
  (module_decl
    name: (module_name
      (constructor)))
  (capability_decl
    name: (constructor)
    entries: (record_type
      (field_decl
        name: (identifier)
        type: (type_arrow
          parameter: (type_name
            (constructor))
          result: (type_app
            constructor: (type_name
              (constructor))
            argument: (parenthesized_type
              (type_app
                constructor: (type_name
                  (constructor))
                argument: (type_name
                  (constructor)))))))))
  (fake_decl
    name: (constructor)
    body: (fake_body
      (table_decl
        name: (constructor)
        fields: (record_type
          (field_decl
            name: (identifier)
            type: (type_name
              (constructor))
            attribute: (field_attribute
              name: (identifier)))))
      (fake_entry
        name: (identifier)
        params: (parameters
          (identifier))
        body: (application
          function: (qualified_name
            module: (constructor)
            name: (identifier))
          argument: (parenthesized_expression
            (application
              function: (qualified_name
                module: (constructor)
                name: (identifier))
              argument: (identifier))))))))

================================================================================
session, login, choose, silo, deploy, test
================================================================================

module App

session { user: Maybe User.UserId, org: Maybe Org.OrgId via Membership, role: Role = Member }

login = Auth.Login

choose org = Orgs.Index

silo org { Project, Todo }

deploy {
  production: Ssh "deploy@box" { domain: "d", path: "/srv" },
  floor: 0.95,
  checks: ["./x"],
  requires: ["SMTP_URL"],
  proxy: None,
}

test "one is one" = 1 == 1

--------------------------------------------------------------------------------

(source_file
  (module_decl
    name: (module_name
      (constructor)))
  (session_decl
    fields: (record_type
      (field_decl
        name: (identifier)
        type: (type_app
          constructor: (type_name
            (constructor))
          argument: (type_name
            (constructor)
            (constructor))))
      (field_decl
        name: (identifier)
        type: (type_app
          constructor: (type_name
            (constructor))
          argument: (type_name
            (constructor)
            (constructor)))
        via: (constructor))
      (field_decl
        name: (identifier)
        type: (type_name
          (constructor))
        default: (constructor))))
  (login_decl
    page: (module_name
      (constructor)
      (constructor)))
  (choose_decl
    key: (identifier)
    page: (module_name
      (constructor)
      (constructor)))
  (silo_decl
    key: (identifier)
    table: (constructor)
    table: (constructor))
  (deploy_decl
    body: (deploy_record
      (deploy_field
        name: (identifier)
        value: (deploy_target
          kind: (constructor)
          argument: (string)
          options: (deploy_record
            (deploy_field
              name: (identifier)
              value: (string))
            (deploy_field
              name: (identifier)
              value: (string)))))
      (deploy_field
        name: (identifier)
        value: (float))
      (deploy_field
        name: (identifier)
        value: (deploy_list
          (string)))
      (deploy_field
        name: (identifier)
        value: (deploy_list
          (string)))
      (deploy_field
        name: (identifier)
        value: (deploy_target
          kind: (constructor)))))
  (test_decl
    name: (string)
    body: (comparison_expr
      left: (integer)
      right: (integer))))
```

- [ ] **Step 2: Write `test/corpus/signatures.txt`** (the stdlib shape, ruling R3)

```
================================================================================
signature files: no bodies, opaque types, values named after keywords
================================================================================

module Test

type Query a
run : Task a -> Task { value: Result Text a, trace: List Text }
where : Bool -> Int
fake : Capability -> Task Unit
login : Session -> Task Unit

--------------------------------------------------------------------------------

(source_file
  (module_decl
    name: (module_name
      (constructor)))
  (type_decl
    name: (constructor)
    param: (type_var))
  (value_decl
    signature: (signature
      name: (identifier)
      type: (type_arrow
        parameter: (type_app
          constructor: (type_name
            (constructor))
          argument: (type_var))
        result: (type_app
          constructor: (type_name
            (constructor))
          argument: (record_type
            (field_decl
              name: (identifier)
              type: (type_app
                constructor: (type_name
                  (constructor))
                argument: (type_name
                  (constructor))
                argument: (type_var)))
            (field_decl
              name: (identifier)
              type: (type_app
                constructor: (type_name
                  (constructor))
                argument: (type_name
                  (constructor)))))))))
  (value_decl
    signature: (signature
      name: (identifier)
      type: (type_arrow
        parameter: (type_name
          (constructor))
        result: (type_name
          (constructor)))))
  (value_decl
    signature: (signature
      name: (identifier)
      type: (type_arrow
        parameter: (type_name
          (constructor))
        result: (type_app
          constructor: (type_name
            (constructor))
          argument: (type_name
            (constructor))))))
  (value_decl
    signature: (signature
      name: (identifier)
      type: (type_arrow
        parameter: (type_name
          (constructor))
        result: (type_app
          constructor: (type_name
            (constructor))
          argument: (type_name
            (constructor)))))))
```

- [ ] **Step 3: Run**

Run: `scripts/ts.sh test`
Expected: all pass, fixing as in Task 1 Step 8; regenerate after grammar changes.

- [ ] **Step 4: Commit**

```bash
git add tree-sitter-alder
git commit -m "grammar: corpus for declarations and signature files"
```

---

## Task 4: HTML

**Files:**
- Create: `tree-sitter-alder/test/corpus/html.txt`
- Modify: `grammar.js` / `scanner.c` as the cases require, regenerated `src/`

**Interfaces:**
- Consumes: Task 1 HTML node names. Produces: the HTML shapes below.

- [ ] **Step 1: Write `test/corpus/html.txt`**

```
================================================================================
element with attributes, for, if, interpolation and a self-closing custom element
================================================================================

module M

view m =
  <ul id="todos">
    {for t in m.todos ->
      <li class={if t.done then "done" else "open"}>{t.title}</li>}
    <al-input field="title" />
  </ul>

--------------------------------------------------------------------------------

(source_file
  (module_decl
    name: (module_name
      (constructor)))
  (value_decl
    binding: (binding
      name: (identifier)
      params: (parameters
        (identifier))
      body: (element
        open: (start_tag
          tag: (tag_name)
          (attribute
            name: (attribute_name)
            value: (string)))
        (html_text)
        (html_for
          binder: (identifier)
          items: (field_access
            value: (identifier)
            field: (identifier))
          body: (element
            open: (start_tag
              tag: (tag_name)
              (attribute
                name: (attribute_name)
                value: (interpolation
                  expression: (if_expr
                    condition: (field_access
                      value: (identifier)
                      field: (identifier))
                    consequence: (string)
                    alternative: (string)))))
            (interpolation
              expression: (field_access
                value: (identifier)
                field: (identifier)))
            close: (end_tag
              tag: (tag_name))))
        (html_text)
        (self_closing_element
          tag: (tag_name)
          (attribute
            name: (attribute_name)
            value: (string)))
        (html_text)
        close: (end_tag
          tag: (tag_name))))))

================================================================================
components, keyword tag names, boolean attributes, if and match children
================================================================================

module M

page m =
  <Layout.Page title={m.title}>
    <form submit={Add}><Errors form /></form>
    {if m.empty then <p>none</p> else Html.none}
    {match m.filter {
      All -> <b>all</b>,
      _ -> Html.none,
    }}
  </Layout.Page>

--------------------------------------------------------------------------------

(source_file
  (module_decl
    name: (module_name
      (constructor)))
  (value_decl
    binding: (binding
      name: (identifier)
      params: (parameters
        (identifier))
      body: (element
        open: (start_tag
          tag: (component_name
            (constructor)
            (constructor))
          (attribute
            name: (attribute_name)
            value: (interpolation
              expression: (field_access
                value: (identifier)
                field: (identifier)))))
        (html_text)
        (element
          open: (start_tag
            tag: (tag_name)
            (attribute
              name: (attribute_name)
              value: (interpolation
                expression: (constructor))))
          (self_closing_element
            tag: (component_name
              (constructor))
            (attribute
              name: (attribute_name)))
          close: (end_tag
            tag: (tag_name)))
        (html_text)
        (html_if
          condition: (field_access
            value: (identifier)
            field: (identifier))
          consequence: (element
            open: (start_tag
              tag: (tag_name))
            (html_text)
            close: (end_tag
              tag: (tag_name)))
          alternative: (qualified_name
            module: (constructor)
            name: (identifier)))
        (html_text)
        (html_match
          subject: (field_access
            value: (identifier)
            field: (identifier))
          (arm
            pattern: (constructor_pattern
              constructor: (constructor))
            body: (element
              open: (start_tag
                tag: (tag_name))
              (html_text)
              close: (end_tag
                tag: (tag_name))))
          (arm
            pattern: (wildcard)
            body: (qualified_name
              module: (constructor)
              name: (identifier))))
        (html_text)
        close: (end_tag
          tag: (component_name
            (constructor)
            (constructor)))))))

================================================================================
raw script and style bodies; a mixed-case closer
================================================================================

module M

player =
  <div>
    <style>.p > video { width: 100%; } a::after { content: "<→>"; }</style>
    <script>if (1 < 2 && true) { go("</div>"); }</Script>
  </div>

--------------------------------------------------------------------------------

(source_file
  (module_decl
    name: (module_name
      (constructor)))
  (value_decl
    binding: (binding
      name: (identifier)
      body: (element
        open: (start_tag
          tag: (tag_name))
        (html_text)
        (raw_element
          tag: (tag_name)
          (raw_text)
          close: (tag_name))
        (html_text)
        (raw_element
          tag: (tag_name)
          (raw_text)
          close: (component_name
            (constructor)))
        (html_text)
        close: (end_tag
          tag: (tag_name))))))

================================================================================
a comment line inside markup text
================================================================================

module M

note =
  <p>
    hello
    -- a note
    world
  </p>

--------------------------------------------------------------------------------

(source_file
  (module_decl
    name: (module_name
      (constructor)))
  (value_decl
    binding: (binding
      name: (identifier)
      body: (element
        open: (start_tag
          tag: (tag_name))
        (html_text)
        (comment)
        (html_text)
        close: (end_tag
          tag: (tag_name))))))
```

- [ ] **Step 2: Run**

Run: `scripts/ts.sh test`
Expected: all pass, fixing as in Task 1 Step 8. The `<→>` inside the style body checks that `<` followed by a non-letter is text and that UTF-8 passes through the scanner.

- [ ] **Step 3: Commit**

```bash
git add tree-sitter-alder
git commit -m "grammar: corpus for HTML, raw text and markup comments"
```

---
## Task 5: Drift test, recovery, corpus and regeneration under `go test`

This task is the gate. Its main loop is: run the drift test, read the first mismatches, fix the grammar or scanner, regenerate, and repeat until the whole repository parses clean.

**Files:**
- Create: `tree-sitter-alder/drift_test.go`, `tree-sitter-alder/test/drift.scm`
- Modify: `grammar.js`, `src/scanner.c`, regenerated `src/`, and new corpus cases for each class of fix

**Interfaces:**
- Consumes: `syntax.ParseFile(file string, src []byte) (*syntax.Module, []diag.Diagnostic)` and `syntax.ParseSignatures` (same signature), both in `alder/internal/syntax`. The Decl types are `*ValueDecl{Name}`, `*TypeDecl{Name}`, `*PageDecl{Path}`, `*MsgDecl{Name}`, `*FormDecl{Name}`, `*TableDecl{Name}`, `*CapDecl{Name}`, `*FakeDecl{Name}`, `*SessionDecl`, `*DeployDecl`, `*SiloDecl{Key}`, `*ChooseDecl{Key}`, `*LoginDecl{Page}`, `*TestDecl{Name}`. `Module.Imports []*Import{Module}` is separate from `Module.Decls`. From `alder/book`: `Extract(path string) ([]Snippet, error)`, `Snippet{File, Line, Info, Source}`, `Classify(Snippet) Kind`, `book.Module`.
- Produces: `go test ./tree-sitter-alder` with `TestDrift`, `TestRecovery`, `TestCorpus`, `TestGenerated`, `TestTSScript_RefusesOtherVersion`. Package name `grammar`, import path `alder/tree-sitter-alder`.

- [ ] **Step 1: Write `tree-sitter-alder/test/drift.scm`**

```scheme
; Top-level declarations, read by drift_test.go: one capture per declaration, named by kind.
(source_file (import_decl module: (module_name) @import))
(source_file (value_decl . (signature name: (_) @value)))
(source_file (value_decl . (binding name: (_) @value)))
(source_file (type_decl name: (constructor) @type))
(source_file (page_decl path: (string) @page))
(source_file (msg_decl name: (constructor) @msg))
(source_file (form_decl name: (constructor) @form))
(source_file (table_decl name: (constructor) @table))
(source_file (capability_decl name: (constructor) @capability))
(source_file (fake_decl name: (constructor) @fake))
(source_file (session_decl "session" @session))
(source_file (deploy_decl "deploy" @deploy))
(source_file (silo_decl key: (identifier) @silo))
(source_file (choose_decl key: (identifier) @choose))
(source_file (login_decl page: (module_name) @login))
(source_file (test_decl name: (string) @test))
```

- [ ] **Step 2: Write `tree-sitter-alder/drift_test.go`**

```go
// Package grammar holds the tests that keep tree-sitter-alder in step with the Go parser. They
// shell out to the tree-sitter CLI, and fail without it unless ALDER_SKIP_TS=1: a drift test
// that skips silently stops guarding anything.
package grammar

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"alder/book"
	"alder/internal/syntax"
)

// pinnedCLI is the tree-sitter version src/ is generated with (scripts/ts.sh, CI).
const pinnedCLI = "0.27.1"

// minSources guards the drift test against a silently shrinking corpus.
const minSources = 1000

func tsCLI(t *testing.T) string {
	t.Helper()
	p, err := exec.LookPath("tree-sitter")
	if err != nil {
		if os.Getenv("ALDER_SKIP_TS") == "1" {
			t.Skip("ALDER_SKIP_TS=1: the tree-sitter grammar tests are skipped")
		}
		t.Fatal("the tree-sitter CLI (>= 0.26.1) is required: brew install tree-sitter-cli, or set ALDER_SKIP_TS=1 to skip the grammar tests")
	}
	return p
}

type decl struct{ Kind, Name string }

type source struct {
	Path    string // the file the CLI reads
	Label   string // where it came from, for messages
	Imports []decl
	Decls   []decl
}

func goDecls(m *syntax.Module) (imports, decls []decl) {
	for _, im := range m.Imports {
		imports = append(imports, decl{"import", im.Module})
	}
	for _, d := range m.Decls {
		switch d := d.(type) {
		case *syntax.ValueDecl:
			decls = append(decls, decl{"value", d.Name})
		case *syntax.TypeDecl:
			decls = append(decls, decl{"type", d.Name})
		case *syntax.PageDecl:
			decls = append(decls, decl{"page", d.Path})
		case *syntax.MsgDecl:
			decls = append(decls, decl{"msg", d.Name})
		case *syntax.FormDecl:
			decls = append(decls, decl{"form", d.Name})
		case *syntax.TableDecl:
			decls = append(decls, decl{"table", d.Name})
		case *syntax.CapDecl:
			decls = append(decls, decl{"capability", d.Name})
		case *syntax.FakeDecl:
			decls = append(decls, decl{"fake", d.Name})
		case *syntax.SessionDecl:
			decls = append(decls, decl{"session", "session"})
		case *syntax.DeployDecl:
			decls = append(decls, decl{"deploy", "deploy"})
		case *syntax.SiloDecl:
			decls = append(decls, decl{"silo", d.Key})
		case *syntax.ChooseDecl:
			decls = append(decls, decl{"choose", d.Key})
		case *syntax.LoginDecl:
			decls = append(decls, decl{"login", d.Page})
		case *syntax.TestDecl:
			decls = append(decls, decl{"test", d.Name})
		default:
			decls = append(decls, decl{fmt.Sprintf("%T", d), "?"})
		}
	}
	return imports, decls
}

// sources is every .alder file under examples/, stdlib/ and testdata/, and every book snippet
// that is a whole module, that the Go parser parses without a diagnostic. stdlib/ is read the
// way the compiler reads it, with ParseSignatures. Snippets are written into dir.
func sources(t *testing.T, dir string) []source {
	t.Helper()
	var out []source
	for _, root := range []string{"../examples", "../stdlib", "../testdata"} {
		sig := root == "../stdlib"
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".alder") {
				return err
			}
			src, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			parse := syntax.ParseFile
			if sig {
				parse = syntax.ParseSignatures
			}
			m, diags := parse(path, src)
			if len(diags) > 0 || m == nil {
				return nil
			}
			abs, _ := filepath.Abs(path)
			im, ds := goDecls(m)
			out = append(out, source{Path: abs, Label: path, Imports: im, Decls: ds})
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	chapters, _ := filepath.Glob("../book/*.md")
	for _, ch := range chapters {
		snips, err := book.Extract(ch)
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range snips {
			if f := strings.Fields(s.Info); len(f) == 0 || f[0] != "alder" || book.Classify(s) != book.Module {
				continue
			}
			label := fmt.Sprintf("%s:%d", ch, s.Line)
			m, diags := syntax.ParseFile(label, []byte(s.Source))
			if len(diags) > 0 || m == nil {
				continue
			}
			path := filepath.Join(dir, fmt.Sprintf("%s-%d.alder", strings.TrimSuffix(filepath.Base(ch), ".md"), s.Line))
			if err := os.WriteFile(path, []byte(s.Source), 0o644); err != nil {
				t.Fatal(err)
			}
			im, ds := goDecls(m)
			out = append(out, source{Path: path, Label: label, Imports: im, Decls: ds})
		}
	}
	return out
}

func writePaths(t *testing.T, dir string, srcs []source) string {
	t.Helper()
	var b strings.Builder
	for _, s := range srcs {
		b.WriteString(s.Path + "\n")
	}
	p := filepath.Join(dir, "paths.txt")
	if err := os.WriteFile(p, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func grammarDir(t *testing.T) string {
	t.Helper()
	d, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// captureRE matches one line of `tree-sitter query` output:
//
//	    capture: 0 - value, start: (3, 0), end: (3, 6), text: `update`
var captureRE = regexp.MustCompile("^\\s+capture: \\d+ - ([a-z]+), start: \\((\\d+), (\\d+)\\), end: \\(\\d+, \\d+\\), text: `(.*)`$")

type capture struct {
	row, col int
	d        decl
}

// queryDecls runs test/drift.scm over the files and returns each file's captures in source order.
func queryDecls(t *testing.T, cli, paths string) map[string][]decl {
	t.Helper()
	cmd := exec.Command(cli, "query", "-p", grammarDir(t), "test/drift.scm", "--paths", paths)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("tree-sitter query: %v\n%s", err, stderr.String())
	}
	caps := map[string][]capture{}
	file := ""
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		if line != "" && !strings.HasPrefix(line, " ") {
			file = line
			continue
		}
		m := captureRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		row, _ := strconv.Atoi(m[2])
		col, _ := strconv.Atoi(m[3])
		name := m[4]
		switch m[1] {
		case "page", "test":
			name = strings.TrimSuffix(strings.TrimPrefix(name, `"`), `"`)
		}
		caps[file] = append(caps[file], capture{row, col, decl{m[1], name}})
	}
	res := map[string][]decl{}
	for f, cs := range caps {
		sort.SliceStable(cs, func(i, j int) bool {
			if cs[i].row != cs[j].row {
				return cs[i].row < cs[j].row
			}
			return cs[i].col < cs[j].col
		})
		for _, c := range cs {
			res[f] = append(res[f], c.d)
		}
	}
	return res
}

func split(ds []decl) (imports, rest []decl) {
	for _, d := range ds {
		if d.Kind == "import" {
			imports = append(imports, d)
		} else {
			rest = append(rest, d)
		}
	}
	return imports, rest
}

func equal(a, b []decl) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestDrift: every file the Go parser accepts parses with no ERROR or MISSING node, and its
// top-level declarations (kind and name, in order) and imports are the Go parser's.
func TestDrift(t *testing.T) {
	cli := tsCLI(t)
	dir := t.TempDir()
	srcs := sources(t, dir)
	if len(srcs) < minSources {
		t.Fatalf("only %d sources parse clean with the Go parser; expected at least %d", len(srcs), minSources)
	}
	paths := writePaths(t, dir, srcs)

	cmd := exec.Command(cli, "parse", "-q", "-p", grammarDir(t), "--paths", paths)
	out, err := cmd.Output()
	if err != nil || len(bytes.TrimSpace(out)) > 0 {
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		if len(lines) > 20 {
			lines = append(lines[:20], fmt.Sprintf("… and %d more", len(lines)-20))
		}
		t.Fatalf("%d sources; tree-sitter reports errors (%v):\n%s", len(srcs), err, strings.Join(lines, "\n"))
	}

	got := queryDecls(t, cli, paths)
	var bad []string
	for _, s := range srcs {
		gi, gd := split(got[s.Path])
		if !equal(gi, s.Imports) || !equal(gd, s.Decls) {
			bad = append(bad, fmt.Sprintf("%s\n  go:          %v %v\n  tree-sitter: %v %v", s.Label, s.Imports, s.Decls, gi, gd))
		}
	}
	if len(bad) > 0 {
		n := len(bad)
		if n > 20 {
			bad = bad[:20]
		}
		t.Fatalf("%d of %d sources disagree on top-level declarations:\n%s", n, len(srcs), strings.Join(bad, "\n"))
	}
	t.Logf("%d sources agree", len(srcs))
}

// TestRecovery: code being typed (an unterminated string, an unclosed element, a dangling
// let) leaves the next column-1 declaration parsed as a declaration.
func TestRecovery(t *testing.T) {
	cli := tsCLI(t)
	dir := t.TempDir()
	cases := map[string]string{
		"string.alder": "module M\n\na = \"unterminated\n\nafter : Int\nafter = 1\n",
		"element.alder": "module M\n\nview m =\n  <div>\n    <p>hello\n\nafter : Int\nafter = 1\n",
		"let.alder":     "module M\n\nb = {\n  let x =\n}\n\nafter : Int\nafter = 1\n",
	}
	var paths []string
	for name, src := range cases {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, p)
	}
	sort.Strings(paths)
	list := filepath.Join(dir, "paths.txt")
	if err := os.WriteFile(list, []byte(strings.Join(paths, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := queryDecls(t, cli, list)
	for _, p := range paths {
		found := false
		for _, d := range got[p] {
			if d == (decl{"value", "after"}) {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: the declaration after the broken one is lost; captures: %v", filepath.Base(p), got[p])
		}
	}
}

// TestCorpus runs the tree-sitter corpus and highlight tests (tree-sitter test).
func TestCorpus(t *testing.T) {
	cli := tsCLI(t)
	cmd := exec.Command(cli, "test")
	cmd.Dir = grammarDir(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("tree-sitter test: %v\n%s", err, out)
	}
}

// TestGenerated regenerates the parser in a copy and compares it with the committed src/, so
// a grammar change without its regenerated parser fails.
func TestGenerated(t *testing.T) {
	cli := tsCLI(t)
	v, err := exec.Command(cli, "--version").Output()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(v), pinnedCLI) {
		t.Skipf("tree-sitter %s generates differently from the pinned %s; CI checks regeneration", strings.TrimSpace(string(v)), pinnedCLI)
	}
	dir := t.TempDir()
	for _, f := range []string{"grammar.js", "tree-sitter.json", "src/scanner.c"} {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		dst := filepath.Join(dir, f)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dst, b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command(cli, "generate")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "TREE_SITTER_JS_RUNTIME=native")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("tree-sitter generate: %v\n%s", err, out)
	}
	for _, f := range []string{"src/parser.c", "src/grammar.json", "src/node-types.json"} {
		want, _ := os.ReadFile(f)
		got, _ := os.ReadFile(filepath.Join(dir, f))
		if !bytes.Equal(want, got) {
			t.Errorf("%s is stale: run scripts/ts.sh generate and commit tree-sitter-alder/src", f)
		}
	}
}

// TestTSScript_RefusesOtherVersion: scripts/ts.sh generate refuses a CLI other than the pinned one.
func TestTSScript_RefusesOtherVersion(t *testing.T) {
	bin := t.TempDir()
	shim := "#!/bin/sh\necho 'tree-sitter 0.99.0'\n"
	if err := os.WriteFile(filepath.Join(bin, "tree-sitter"), []byte(shim), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", "../scripts/ts.sh", "generate")
	cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("ts.sh generate ran with tree-sitter 0.99.0:\n%s", out)
	}
	if !strings.Contains(string(out), "must be generated with "+pinnedCLI) {
		t.Fatalf("ts.sh generate refusal does not name the pinned version:\n%s", out)
	}
}
```

TestTSScript_RefusesOtherVersion does not call `tsCLI`, because it uses a fake `tree-sitter` and needs no real one.

- [ ] **Step 3: Run the tests**

Run: `go test ./tree-sitter-alder -count=1 -run 'TestCorpus|TestGenerated|TestTSScript' -v`
Expected: PASS. If TestGenerated fails, run `scripts/ts.sh generate` and commit `src/`.

Run: `go test ./tree-sitter-alder -count=1 -run TestDrift -v`
Expected at first: FAIL, with the first 20 files tree-sitter cannot parse.

- [ ] **Step 4: Fix until TestDrift passes**

Loop:

1. Take the first failing file. Run `scripts/ts.sh parse <file>`, find the `ERROR` or `MISSING` node, and read the construct in the Go parser (`internal/syntax`) to see what it accepts.
2. Fix `grammar.js` or `src/scanner.c`, keeping each change a port of a Go rule and citing it in a comment.
3. Add a minimal case for the construct to the matching `test/corpus/*.txt` file, with the tree written by hand.
4. Run `scripts/ts.sh generate && scripts/ts.sh test`, then the drift test again.

Then do the same for the declaration mismatches the test prints. Typical causes are a newline swallowed (two Go declarations become one tree-sitter declaration) or a keyword winning where the Go parser saw an identifier. If a valid file relies on column-1 text inside an element, relax the column-1 rule in `scan_html_text` and record that in the scanner comment.

Expected at the end: `ok alder/tree-sitter-alder`, with the log line showing at least 1,000 sources.

- [ ] **Step 5: Run the recovery test**

Run: `go test ./tree-sitter-alder -count=1 -run TestRecovery -v`
Expected: PASS. If a case loses `after`, the cause is in the scanner's recovery path: `scan_newline` must separate before a column-1 line while `valid[ERROR_SENTINEL]` is set, and `scan_html_text` must stop at a column-1 line. Fix it there, not by special-casing the test. For the unclosed element, the newline before `after` is lexed as whitespace in element content, so recovery has no separator to resync on. One fix: when `scan_html_text` stops at a column-1 line having read no text, it returns a `_newline` token for that newline (an external scanner may return a token the state does not expect; the parser then recovers by popping to `source_file`, where `_newline` is valid). Pin whichever fix you choose with this test.

- [ ] **Step 6: Run the whole suite once**

Run: `go test ./... -count=1 2>&1 | tail -30`
Expected: every package passes. This is the first time a new package with tests runs in CI's `rest` part.

- [ ] **Step 7: Commit**

```bash
git add tree-sitter-alder
git commit -m "grammar: drift test against the Go parser, recovery, corpus and regeneration checks"
```

---

## Task 6: Queries

**Files:**
- Create: `tree-sitter-alder/queries/neovim/highlights.scm`, `locals.scm`, `indents.scm`, `folds.scm`, `injections.scm`, `textobjects.scm`
- Modify: `tree-sitter-alder/queries/highlights.scm`
- Create: `tree-sitter-alder/test/highlight/basic.alder`
- Modify: `tree-sitter-alder/drift_test.go` (add `TestQueriesCompile`)

**Interfaces:**
- Consumes: the node names and fields of Task 1, as finally shaped by Task 5.
- Produces: the query files nvim-treesitter installs through `install_info.queries = 'tree-sitter-alder/queries/neovim'`. Task 9's plugin tests assert captures from `highlights.scm` at known positions: `keyword.import` on `module`, `function` on a signature name, `type` on a type name, `tag` on a tag name, `tag.attribute` on an attribute name, `string` on a string.

- [ ] **Step 1: Write the failing compile test** (append to `drift_test.go`)

```go
// TestQueriesCompile runs every query file over a real module, so a query naming a node or
// field the grammar does not have fails here rather than in the editor. Neovim-only predicates
// and directives are not evaluated by the CLI, which is fine: the plugin tests load the
// queries in Neovim (alder.nvim tests/filetype_spec.lua).
func TestQueriesCompile(t *testing.T) {
	cli := tsCLI(t)
	queries, _ := filepath.Glob("queries/neovim/*.scm")
	queries = append(queries, "queries/highlights.scm")
	want := map[string]bool{"highlights": true, "locals": true, "indents": true, "folds": true, "injections": true, "textobjects": true}
	for _, q := range queries {
		delete(want, strings.TrimSuffix(filepath.Base(q), ".scm"))
	}
	if len(want) > 0 {
		t.Fatalf("missing query files: %v", want)
	}
	sample, err := filepath.Abs("../examples/blog/app/App.alder")
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range queries {
		cmd := exec.Command(cli, "query", "-q", "-p", grammarDir(t), q, sample)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("%s: %v\n%s", q, err, out)
		}
	}
}
```

Run: `go test ./tree-sitter-alder -count=1 -run TestQueriesCompile -v`
Expected: FAIL with `missing query files`.

- [ ] **Step 2: Write `queries/neovim/highlights.scm`**

Neovim gives a later pattern priority over an earlier one for the same node, so this file goes from general to specific.

```scheme
; tree-sitter-alder/queries/neovim/highlights.scm — nvim-treesitter capture conventions.
; General first: in Neovim a later pattern wins over an earlier one for the same range.

(identifier) @variable
(constructor) @constructor
(comment) @comment @spell

(string) @string
(integer) @number
(float) @number.float
(negative_number "-" @number)
(boolean) @boolean
(unit) @constant.builtin
(hole) @variable.builtin
(wildcard) @character.special

; ---- keywords
["module" "import"] @keyword.import
["type" "brand" "where"] @keyword.type
["page" "msg" "form" "table" "capability" "fake" "session" "login" "test" "deploy" "silo"
 "choose" "requires" "role" "via" "let"] @keyword
["if" "then" "else" "match"] @keyword.conditional
["for" "in"] @keyword.repeat
"\\" @keyword.function

; ---- operators and punctuation
["|>" "??" "||" "&&" "==" "!=" "<" "<=" ">" ">=" "+" "-" "++" "*" "/" "%" "->" "<-" "="
 "|" ".."] @operator
["(" ")" "[" "]" "{" "}"] @punctuation.bracket
["," ":" "."] @punctuation.delimiter

; ---- names in declarations
(signature name: (identifier) @function)
(signature name: (constructor) @function)
(binding name: (identifier) @function)
(binding name: (constructor) @function)
(fake_entry name: (identifier) @function)
(parameters (identifier) @variable.parameter)
(lambda param: (identifier) @variable.parameter)
(html_for binder: (identifier) @variable.parameter)

; ---- types
(type_var) @type
(type_name (constructor) @module)
(type_name (constructor) @type .)
(type_decl name: (constructor) @type.definition)
(variant name: (constructor) @constructor)
(brand_type base: (constructor) @type)

; ---- modules and qualified names
(module_name (constructor) @module)
(qualified_name module: (constructor) @module)
(qualified_name name: (identifier) @function.call)

; ---- calls and fields
(application function: (identifier) @function.call)
(field_access field: (identifier) @variable.member)
(application function: (field_access field: (identifier) @function.method.call))
(field_decl name: (identifier) @property)
(field_init name: (identifier) @property)
(field_pattern name: (identifier) @property)
(deploy_field name: (identifier) @property)
(deploy_target kind: (constructor) @constructor)

; ---- attributes on fields and tables
(field_attribute "@" @attribute)
(field_attribute name: (identifier) @attribute)
(field_attribute argument: (type_name (constructor) @type))
(field_attribute argument: (identifier) @property)

; ---- page, requires, silos
(page_decl path: (string) @string.special.url)
(session_key) @variable.member
(role_clause role: (constructor) @constructor)
(silo_clause key: (identifier) @variable.member)
(silo_decl key: (identifier) @variable.member)
(silo_decl table: (constructor) @type)
(choose_decl key: (identifier) @variable.member)

; ---- HTML
(tag_name) @tag
(component_name (constructor) @module)
(component_name (constructor) @tag .)
(attribute_name) @tag.attribute
(attribute "=" @operator)
(start_tag ["<" ">"] @tag.delimiter)
(end_tag ["</" ">"] @tag.delimiter)
(self_closing_element ["<" "/" ">"] @tag.delimiter)
(raw_element ["<" "</" ">" "/"] @tag.delimiter)
(html_text) @spell
(interpolation ["{" "}"] @punctuation.special)
(html_for ["{" "}"] @punctuation.special)
(html_if ["{" "}"] @punctuation.special)
(html_match "{" @punctuation.special .)
```

The spec named `@markup` for HTML text. nvim-treesitter has no plain `@markup` capture, so text gets `@spell` and the default colour.

- [ ] **Step 3: Write `queries/neovim/locals.scm`**

```scheme
; Scopes, definitions and references (nvim-treesitter locals).
(block) @local.scope
(lambda) @local.scope
(arm) @local.scope
(html_for) @local.scope
(value_decl) @local.scope
(fake_entry) @local.scope

(let_stmt name: (identifier) @local.definition.var)
(bind_stmt name: (identifier) @local.definition.var)
(parameters (identifier) @local.definition.parameter)
(lambda param: (identifier) @local.definition.parameter)
(html_for binder: (identifier) @local.definition.var)
(arm pattern: (identifier) @local.definition.var)
(constructor_pattern argument: (identifier) @local.definition.var)
(field_pattern pattern: (identifier) @local.definition.var)
(list_pattern (identifier) @local.definition.var)
(rest_pattern name: (identifier) @local.definition.var)

(identifier) @local.reference
```

- [ ] **Step 4: Write `queries/neovim/indents.scm`**

nvim-treesitter indents a row once even when several nodes begin on it, so a binding and the block it opens add one level, not two.

```scheme
; nvim-treesitter indent captures.
[
  (binding)
  (fake_entry)
  (test_decl)
  (type_decl)
  (record)
  (record_update)
  (block)
  (list)
  (record_type)
  (match_expr)
  (arm)
  (lambda)
  (if_expr)
  (fake_body)
  (deploy_record)
  (deploy_list)
  (silo_decl)
  (parenthesized_expression)
  (element)
  (html_for)
  (html_if)
  (html_match)
  (interpolation)
  (record_pattern)
  (list_pattern)
  ; an operator-led continuation line is one level in
  (pipe_expr)
  (default_expr)
  (or_expr)
  (and_expr)
  (comparison_expr)
  (additive_expr)
  (multiplicative_expr)
] @indent.begin

["}" "]" ")"] @indent.branch @indent.end
(end_tag) @indent.branch @indent.end
(if_expr "else" @indent.branch)
(html_if "else" @indent.branch)

(comment) @indent.auto
(ERROR) @indent.auto
```

- [ ] **Step 5: Write `queries/neovim/folds.scm`**

```scheme
[
  (value_decl)
  (type_decl)
  (table_decl)
  (capability_decl)
  (fake_decl)
  (session_decl)
  (deploy_decl)
  (test_decl)
  (form_decl)
  (block)
  (match_expr)
  (element)
  (record)
  (list)
] @fold
```

- [ ] **Step 6: Write `queries/neovim/injections.scm`**

```scheme
((raw_element
  tag: (tag_name) @_tag
  (raw_text) @injection.content)
  (#eq? @_tag "script")
  (#set! injection.language "javascript"))

((raw_element
  tag: (tag_name) @_tag
  (raw_text) @injection.content)
  (#eq? @_tag "style")
  (#set! injection.language "css"))

((comment) @injection.content
  (#set! injection.language "comment"))
```

- [ ] **Step 7: Write `queries/neovim/textobjects.scm`**

```scheme
; nvim-treesitter-textobjects captures.
(value_decl) @function.outer
(value_decl binding: (binding body: (_) @function.inner))
(fake_entry) @function.outer
(fake_entry body: (_) @function.inner)
(test_decl) @function.outer
(test_decl body: (_) @function.inner)
(lambda) @function.outer
(lambda body: (_) @function.inner)

[
  (type_decl)
  (table_decl)
  (capability_decl)
  (page_decl)
  (msg_decl)
  (form_decl)
] @class.outer
(type_decl body: (_) @class.inner)
(table_decl fields: (_) @class.inner)
(capability_decl entries: (_) @class.inner)
(form_decl fields: (_) @class.inner)
(page_decl params: (_) @class.inner)

(parameters (identifier) @parameter.inner @parameter.outer)
(lambda param: (identifier) @parameter.inner @parameter.outer)
(application argument: (_) @parameter.inner)

(arm) @block.outer
(arm body: (_) @block.inner)
(block) @block.outer
(block result: (_) @block.inner)

(comment) @comment.outer
```

- [ ] **Step 8: Write the minimal CLI query `queries/highlights.scm`**

The tree-sitter CLI gives the **first** matching pattern priority, so this file goes from specific to general.

```scheme
; Minimal highlights for the tree-sitter CLI (`tree-sitter highlight`, the docs-site follow-up).
; Standard capture names; the first matching pattern wins.
(signature name: (_) @function)
(binding name: (_) @function)
(page_decl path: (string) @string.special)
(type_name) @type
(field_decl name: (identifier) @property)
(field_attribute) @attribute
(tag_name) @tag
(component_name) @tag
(attribute_name) @attribute
(hole) @variable.builtin
(comment) @comment
(string) @string
[(integer) (float)] @number
(constructor) @constructor
["module" "import" "type" "brand" "where" "page" "msg" "form" "table" "capability" "fake"
 "session" "login" "test" "deploy" "silo" "choose" "requires" "role" "via" "let" "if" "then"
 "else" "match" "for" "in"] @keyword
```

- [ ] **Step 9: Write `test/highlight/basic.alder`**

Each assertion comment points at the column of the line above it. `<-` points at the comment's own first column.

```
module Demo
-- <- keyword

view : Int -> Html Never
-- <- function
--     ^ type

view n = <p class="x">{n}</p>
--        ^ tag
--          ^ attribute
--                ^ string
```

- [ ] **Step 10: Run**

Run: `go test ./tree-sitter-alder -count=1 -run 'TestQueriesCompile|TestCorpus' -v`
Expected: PASS. If the CLI rejects a node or field name, the query is wrong: fix the query to the grammar's names, not the other way round.

- [ ] **Step 11: Commit**

```bash
git add tree-sitter-alder
git commit -m "grammar: Neovim queries (highlights, locals, indents, folds, injections, textobjects) and CLI highlights"
```

---

## Task 7: `alder help --json`

**Files:**
- Create: `cmd/alder/helpjson.go`, `cmd/alder/helpjson_test.go`
- Modify: `cmd/alder/help.go` (usage entries for `silo`, `silos`, `help`; `help --json` dispatch)

**Interfaces:**
- Consumes: the usage strings in `commandUsages` and `newGeneratorUsages` (`cmd/alder/help.go`) and the consts they name (`evalUsage`, `testUsage`, `migrateUsage`, `migrateSilosUsage`, `silosUsage`, `replUsage`, `newUsage`, `editUsage`, `removeUsage`, `fillUsage`, `regenUsage`, `deployUsage`, `statusUsage`, `rollbackUsage`, `logsUsage`, `restoreUsage`, `ciUsage`, `new*Usage`), and `version.String()` from `alder/internal/version`.
- Produces: `alder help --json` prints one JSON object `{"version": string, "commands": [cmd]}`, where `cmd = {"name", "summary", "usage"?, "args": [string], "flags": [{"name", "value"?, "multi"?, "summary"}], "subcommands"?: [cmd]}`. `args` and `flags` are always arrays. A flag with a `value` takes one, and a flag without one is a switch. Tasks 11 and 13 read this shape.

- [ ] **Step 1: Write the failing tests** `cmd/alder/helpjson_test.go`

```go
package main

import (
	"bytes"
	"encoding/json"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// usageFlagRE finds flags in a usage string: `--json`, `-run`, `[--list T | --show T`, `|--ast`.
var usageFlagRE = regexp.MustCompile(`(?:^|[\s\[|(])(--?[a-z][a-z-]*)`)

func usageFlags(u string) []string {
	set := map[string]bool{}
	for _, m := range usageFlagRE.FindAllStringSubmatch(u, -1) {
		set[m[1]] = true
	}
	return sortedKeys(set)
}

// specFlags is a node's flags plus those of its subcommands that have no usage of their own.
func specFlags(c cmdSpec) []string {
	set := map[string]bool{}
	var add func(c cmdSpec)
	add = func(c cmdSpec) {
		for _, f := range c.Flags {
			set[f.Name] = true
		}
		for _, s := range c.Subcommands {
			if s.Usage == "" {
				add(s)
			}
		}
	}
	add(c)
	return sortedKeys(set)
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// TestHelpJSON_MatchesUsage pins the structured tree to the text help: the same commands and
// generators, each node's usage string the one `--help` prints, and the same flags.
func TestHelpJSON_MatchesUsage(t *testing.T) {
	top := map[string]bool{}
	for _, c := range commandTree {
		top[c.Name] = true
		if want, ok := commandUsages[c.Name]; !ok {
			t.Errorf("command %q is in commandTree but not in commandUsages", c.Name)
		} else if c.Usage != want {
			t.Errorf("command %q: usage differs from commandUsages", c.Name)
		}
	}
	for name := range commandUsages {
		if !top[name] {
			t.Errorf("command %q has usage text but no commandTree entry", name)
		}
	}
	var gens map[string]bool
	for _, c := range commandTree {
		if c.Name != "new" {
			continue
		}
		gens = map[string]bool{}
		for _, s := range c.Subcommands {
			gens[s.Name] = true
			if s.Usage != newGeneratorUsages[s.Name] {
				t.Errorf("generator %q: usage differs from newGeneratorUsages", s.Name)
			}
		}
	}
	for name := range newGeneratorUsages {
		if !gens[name] {
			t.Errorf("generator %q has usage text but no commandTree entry", name)
		}
	}
	var walk func(path string, c cmdSpec)
	walk = func(path string, c cmdSpec) {
		if c.Summary == "" {
			t.Errorf("%s: no summary", path)
		}
		for _, f := range c.Flags {
			if f.Summary == "" {
				t.Errorf("%s %s: no summary", path, f.Name)
			}
		}
		if c.Usage != "" {
			got, want := strings.Join(specFlags(c), " "), strings.Join(usageFlags(c.Usage), " ")
			if got != want {
				t.Errorf("%s: flags in commandTree [%s] differ from its usage text [%s]", path, got, want)
			}
		}
		for _, s := range c.Subcommands {
			walk(path+" "+s.Name, s)
		}
	}
	for _, c := range commandTree {
		walk("alder "+c.Name, c)
	}
}

func TestHelpJSON_Output(t *testing.T) {
	var out bytes.Buffer
	if !helpTo([]string{"help", "--json"}, &out) {
		t.Fatal("helpTo did not handle `help --json`")
	}
	var got struct {
		Version  string            `json:"version"`
		Commands []json.RawMessage `json:"commands"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("not one JSON object: %v\n%s", err, out.String())
	}
	if got.Version == "" || len(got.Commands) != len(commandTree) {
		t.Fatalf("version %q, %d commands; want a version and %d", got.Version, len(got.Commands), len(commandTree))
	}
	if strings.Contains(out.String(), `"flags":null`) || strings.Contains(out.String(), `"args":null`) {
		t.Fatalf("args and flags must be arrays, never null:\n%s", out.String())
	}
	if !strings.Contains(out.String(), `"name":"--list","value":"T"`) {
		t.Fatalf("`new page --list T` missing:\n%s", out.String())
	}
}

func TestHelp_SiloAndSilosHaveUsage(t *testing.T) {
	for _, args := range [][]string{{"help", "silos"}, {"new", "silo", "--help"}} {
		var out bytes.Buffer
		if !helpTo(args, &out) {
			t.Fatalf("%v: not handled", args)
		}
		if strings.Contains(out.String(), "usage: alder new <name> |") {
			t.Errorf("%v printed the general usage instead of its own:\n%s", args, out.String())
		}
	}
}
```

- [ ] **Step 2: Run them to see them fail**

Run: `go test ./cmd/alder -count=1 -run 'TestHelpJSON|TestHelp_Silo' -v`
Expected: FAIL to compile: `undefined: commandTree`, `undefined: cmdSpec`.

- [ ] **Step 3: Add the usage entries and the dispatch in `cmd/alder/help.go`**

In `commandUsages`, add these three entries:

```go
	"silos":    silosUsage,
	"help":     helpUsage,
```

and add `migrateSilosUsage` nowhere (it is the `migrate silos` subcommand's usage, reached from the tree). In `newGeneratorUsages` add `"silo": newSiloUsage,`. Add the const next to the maps:

```go
const helpUsage = "usage: alder help [<command> [<generator>]] [--json]"
```

At the top of `helpTo`, before the existing handling of `args[0] == "help"`, add:

```go
	if len(args) > 0 && args[0] == "help" {
		for _, a := range args[1:] {
			if a == "--json" {
				if err := writeHelpJSON(w); err != nil {
					fmt.Fprintln(w, err)
				}
				return true
			}
		}
	}
```

Read the rest of `helpTo` first. If `alder help silos` or `alder new silo --help` still fall through to `usageTo`, route them the way their neighbours are routed: the test in Step 1 names both.

- [ ] **Step 4: Write `cmd/alder/helpjson.go`**

```go
package main

import (
	"encoding/json"
	"io"

	"alder/internal/version"
)

// flagSpec is one flag in `alder help --json`.
type flagSpec struct {
	Name    string `json:"name"`            // as typed: "--json", "-run"
	Value   string `json:"value,omitempty"` // the value's placeholder ("T", "re"); empty for a switch
	Multi   bool   `json:"multi,omitempty"` // may be given more than once
	Summary string `json:"summary"`
}

// cmdSpec is a command or subcommand in `alder help --json`. Usage is the very string
// `alder <command> --help` prints (commandUsages, newGeneratorUsages); a subcommand with no
// Usage is covered by its parent's. TestHelpJSON_MatchesUsage pins Flags to those strings.
type cmdSpec struct {
	Name        string     `json:"name"`
	Summary     string     `json:"summary"`
	Usage       string     `json:"usage,omitempty"`
	Args        []string   `json:"args"`
	Flags       []flagSpec `json:"flags"`
	Subcommands []cmdSpec  `json:"subcommands,omitempty"`
}

// genFlags are the flags every generator takes (parseGenArgs).
var genFlags = []flagSpec{
	{Name: "--dir", Value: "app", Summary: "the app root, the directory holding app/ (default: the current directory)"},
	{Name: "--json", Summary: "print the result as one JSON object"},
	{Name: "--dry-run", Summary: "check the change on a copy and print the diff; write nothing"},
}

func gen(fs ...flagSpec) []flagSpec { return append(fs, genFlags...) }

var hostFlag = flagSpec{Name: "--host", Value: "name", Summary: "the host of a multi-host environment"}

// commandTree is `alder help --json`: what the plugin completes and offers in pickers.
var commandTree = []cmdSpec{
	{Name: "check", Summary: "type-check the app and print its diagnostics", Usage: commandUsages["check"],
		Args:  []string{"[dir]"},
		Flags: []flagSpec{{Name: "--json", Summary: "one JSON object per diagnostic, one per line"}}},
	{Name: "build", Summary: "compile the app to a single binary", Usage: commandUsages["build"],
		Args:  []string{"[dir]"},
		Flags: []flagSpec{{Name: "--emit", Summary: "write the generated Go source and stop"}}},
	{Name: "run", Summary: "run the app with live reload", Usage: commandUsages["run"], Args: []string{"[dir]"}},
	{Name: "dump", Summary: "print a file's tokens, syntax tree or IR", Usage: commandUsages["dump"],
		Args: []string{"<file.alder | dir>"},
		Flags: []flagSpec{
			{Name: "--tokens", Summary: "the tokens"},
			{Name: "--ast", Summary: "the syntax tree"},
			{Name: "--ir", Summary: "the typed IR"},
		}},
	{Name: "eval", Summary: "evaluate an expression in the app", Usage: commandUsages["eval"],
		Args: []string{"[dir]", "<expr | ->"},
		Flags: []flagSpec{
			{Name: "--run", Summary: "run the effect the expression describes"},
			{Name: "--json", Summary: "print the result as JSON"},
			{Name: "--module", Value: "Name", Summary: "evaluate inside this module"},
		}},
	{Name: "test", Summary: "run the app's tests", Usage: commandUsages["test"],
		Args: []string{"[dir]"},
		Flags: []flagSpec{
			{Name: "-run", Value: "re", Summary: "only the tests whose name matches"},
			{Name: "-timeout", Value: "d", Summary: "fail a test that runs longer"},
			{Name: "--json", Summary: "one JSON object per result"},
			{Name: "--mutate", Summary: "mutation testing: list the mutants no test kills"},
			{Name: "--min", Value: "R", Summary: "with --mutate, fail below this mutation score"},
		}},
	{Name: "migrate", Summary: "write the migration for a schema change", Usage: commandUsages["migrate"],
		Args: []string{"[dir]"},
		Flags: []flagSpec{
			{Name: "--name", Value: "slug", Summary: "the migration's name"},
			{Name: "--dry-run", Summary: "print the migration; write nothing"},
		},
		Subcommands: []cmdSpec{
			{Name: "silos", Summary: "migrate every silo of an environment", Usage: migrateSilosUsage,
				Args: []string{"<env>", "[dir]"},
				Flags: []flagSpec{hostFlag,
					{Name: "--allow-data-loss", Summary: "apply a migration that drops data"}}},
		}},
	{Name: "repl", Summary: "an interactive session in the app", Usage: commandUsages["repl"],
		Args:  []string{"[dir]"},
		Flags: []flagSpec{{Name: "--module", Value: "Name", Summary: "start inside this module"}}},
	{Name: "new", Summary: "create an app, or generate code in one", Usage: commandUsages["new"],
		Args:  []string{"<name>"},
		Flags: []flagSpec{{Name: "--bare", Summary: "the app without its starter pages"}},
		Subcommands: []cmdSpec{
			{Name: "page", Summary: "a page module", Usage: newGeneratorUsages["page"],
				Args: []string{"<Module>", "<path>"},
				Flags: gen(
					flagSpec{Name: "--list", Value: "T", Summary: "a page listing the rows of table T"},
					flagSpec{Name: "--show", Value: "T", Summary: "a page showing one row of table T"},
					flagSpec{Name: "--by", Value: "column", Summary: "with --show, the unique column in the path"},
					flagSpec{Name: "--from", Value: "T", Multi: true, Summary: "load from table T (repeatable)"},
					flagSpec{Name: "--blank", Summary: "an empty page with typed holes"},
					flagSpec{Name: "--param", Value: "name:Type", Multi: true, Summary: "a page parameter (repeatable)"},
					flagSpec{Name: "--requires", Value: "user", Summary: "require a signed-in user"},
					flagSpec{Name: "--role", Value: "R", Summary: "require role R"},
				)},
			{Name: "table", Summary: "a table and its migration", Usage: newGeneratorUsages["table"],
				Args: []string{"<Name>", "<field>:<Type>…"},
				Flags: gen(
					flagSpec{Name: "--no-timestamps", Summary: "without created and updated columns"},
					flagSpec{Name: "--silo", Summary: "a table in a silo"},
				)},
			{Name: "form", Summary: "a form with its msg, handler and view", Usage: newGeneratorUsages["form"],
				Args: []string{"<Page module>", "<Name>", "<field>:<Type>…"},
				Flags: gen(
					flagSpec{Name: "--into", Value: "T", Summary: "insert the submitted form into table T"},
					flagSpec{Name: "--msg", Value: "Msg", Summary: "the msg's name"},
				)},
			{Name: "msg", Summary: "a msg with its arm, button and test", Usage: newGeneratorUsages["msg"],
				Args: []string{"<Page module>", "<Name>", "<field>:<Type>…"},
				Flags: gen(
					flagSpec{Name: "--in-for", Value: "binder", Summary: "the button goes inside this for loop"},
					flagSpec{Name: "--when", Value: "Bool expression", Summary: "show the button only when this holds"},
					flagSpec{Name: "--id", Value: "id", Summary: "the element id the button goes in"},
				)},
			{Name: "capability", Summary: "a capability entry with its fake and adapter", Usage: newGeneratorUsages["capability"],
				Args: []string{"<Module>", "<entry>", "<Type>"}, Flags: gen()},
			{Name: "admin", Summary: "the admin pages for every table", Usage: newGeneratorUsages["admin"],
				Flags: gen(
					flagSpec{Name: "--module", Value: "Admin", Summary: "the admin's module prefix"},
					flagSpec{Name: "--path", Value: "/admin", Summary: "the admin's path prefix"},
				)},
			{Name: "auth", Summary: "sign-in, sign-out and the user table", Usage: newGeneratorUsages["auth"],
				Flags: gen(
					flagSpec{Name: "--users", Value: "User", Summary: "the user table"},
					flagSpec{Name: "--role", Value: "Role", Summary: "the role type"},
					flagSpec{Name: "--to", Value: "Page", Summary: "the page after sign-in"},
				)},
			{Name: "type", Summary: "a type declaration", Usage: newGeneratorUsages["type"],
				Args: []string{"<Module>", "<declaration>"}, Flags: gen()},
			{Name: "decl", Summary: "a value declaration with a typed hole for its body", Usage: newGeneratorUsages["decl"],
				Args: []string{"<Module>", "<name>", "<Type>"}, Flags: gen()},
			{Name: "test", Summary: "a test skeleton", Usage: newGeneratorUsages["test"],
				Args: []string{"<Module>", "<test name>"}, Flags: gen()},
			{Name: "silo", Summary: "a silo over tables", Usage: newGeneratorUsages["silo"],
				Args:  []string{"<key>", "<Table>…"},
				Flags: gen(flagSpec{Name: "--empty", Summary: "a silo with no tables yet"})},
		}},
	{Name: "edit", Summary: "change a table, page, declaration, auth or the session", Usage: commandUsages["edit"],
		Flags: gen(),
		Subcommands: []cmdSpec{
			{Name: "table", Summary: "add, rename or drop a field (and write the migration)",
				Args:  []string{"<T>", "add-field|rename-field|drop-field", "…"},
				Flags: []flagSpec{{Name: "--strict", Summary: "refuse a change that breaks code"}}},
			{Name: "page", Summary: "add a page parameter", Args: []string{"<Page module>", "add-param", "<name>:<Type>[=default]"}},
			{Name: "decl", Summary: "add a component prop", Args: []string{"<Module>.<Name>", "add-prop", "<name>:<Type>=<default>"}},
			{Name: "auth", Summary: "change where sign-in leads",
				Flags: []flagSpec{{Name: "--to", Value: "Page", Summary: "the page after sign-in"}}},
			{Name: "session", Summary: "add a session field",
				Args: []string{"add-field", "<name>:<Type>=<default> | <key>:<T>.<T>Id via <Membership>"},
				Flags: []flagSpec{
					{Name: "--choose", Value: "Page", Summary: "the page that chooses the key"},
					{Name: "--strict", Summary: "refuse a change that breaks code"},
				}},
		}},
	{Name: "remove", Summary: "remove a page or a table", Usage: commandUsages["remove"],
		Flags: gen(),
		Subcommands: []cmdSpec{
			{Name: "page", Summary: "remove a page module", Args: []string{"<Page module>"}},
			{Name: "table", Summary: "remove a table (and write the migration)", Args: []string{"<T>"}},
		}},
	{Name: "fill", Summary: "fill a typed hole with an expression from stdin", Usage: commandUsages["fill"],
		Args: []string{"<Module>.<decl>[.<arm>]", "-"},
		Flags: gen(
			flagSpec{Name: "--hole", Value: "?name", Summary: "the hole to fill when the declaration has several"},
			flagSpec{Name: "--decl", Summary: "replace the whole declaration body"},
			flagSpec{Name: "--list", Summary: "list the app's holes"},
		)},
	{Name: "regen", Summary: "regenerate generated code", Usage: commandUsages["regen"],
		Flags: gen(
			flagSpec{Name: "--module", Value: "Admin", Summary: "the admin's module prefix"},
			flagSpec{Name: "--path", Value: "/admin", Summary: "the admin's path prefix"},
			flagSpec{Name: "--all", Summary: "regenerate edited regions too"},
		),
		Subcommands: []cmdSpec{{Name: "admin", Summary: "the admin pages"}}},
	{Name: "deploy", Summary: "deploy to an environment", Usage: commandUsages["deploy"],
		Args: []string{"<env>", "[dir]"},
		Flags: []flagSpec{hostFlag,
			{Name: "--allow-data-loss", Summary: "apply a migration that drops data"},
			{Name: "--unsafe-skip-tests", Summary: "deploy without running the tests"},
			{Name: "--dry-run", Summary: "print the plan; change nothing"},
			{Name: "--json", Summary: "print the result as JSON"},
		}},
	{Name: "status", Summary: "show what an environment runs", Usage: commandUsages["status"],
		Args: []string{"<env>", "[dir]"}, Flags: []flagSpec{hostFlag}},
	{Name: "rollback", Summary: "go back to an earlier build", Usage: commandUsages["rollback"],
		Args: []string{"<env>", "[<build id>]"},
		Flags: []flagSpec{
			{Name: "--dir", Value: "dir", Summary: "the app root"},
			hostFlag,
			{Name: "--allow-schema-mismatch", Summary: "roll back across a schema change"},
			{Name: "--restore-backup", Summary: "restore the database from the build's backup"},
			{Name: "--yes", Summary: "do not ask for confirmation"},
		}},
	{Name: "logs", Summary: "print an environment's logs", Usage: commandUsages["logs"],
		Args: []string{"<env>", "[dir]"},
		Flags: []flagSpec{hostFlag, {Name: "-f", Summary: "follow"}}},
	{Name: "restore", Summary: "restore a database from a backup", Usage: commandUsages["restore"],
		Args: []string{"<env>"},
		Flags: []flagSpec{
			{Name: "--silo", Value: "key:id", Summary: "one silo's database"},
			{Name: "--from", Value: "url", Summary: "the backup to restore from"},
			{Name: "--to", Value: "time", Summary: "the point in time"},
			{Name: "--out", Value: "file.db", Summary: "write to a file instead of the live database"},
			{Name: "--dir", Value: "dir", Summary: "the app root"},
			{Name: "--yes", Summary: "do not ask for confirmation"},
		}},
	{Name: "silos", Summary: "manage an environment's silos", Usage: commandUsages["silos"],
		Subcommands: []cmdSpec{
			{Name: "prune", Summary: "delete the databases of silos that no longer exist",
				Args: []string{"<env>", "[dir]"},
				Flags: []flagSpec{hostFlag, {Name: "--yes", Summary: "do not ask for confirmation"}}},
		}},
	{Name: "ci", Summary: "set up continuous integration", Usage: commandUsages["ci"],
		Subcommands: []cmdSpec{
			{Name: "init", Summary: "write the CI workflow", Args: []string{"[dir]"},
				Flags: []flagSpec{
					{Name: "--github", Summary: "a GitHub Actions workflow"},
					{Name: "--force", Summary: "overwrite an existing workflow"},
				}},
		}},
	{Name: "version", Summary: "print the version", Usage: commandUsages["version"]},
	{Name: "explain", Summary: "explain a diagnostic code", Usage: commandUsages["explain"], Args: []string{"<code>"}},
	{Name: "help", Summary: "print usage", Usage: commandUsages["help"],
		Args:  []string{"[<command> [<generator>]]"},
		Flags: []flagSpec{{Name: "--json", Summary: "the command tree as JSON"}}},
}

// filled returns c with nil Args, Flags and Subcommands' slices replaced by empty ones, so
// the JSON always has arrays.
func filled(cs []cmdSpec) []cmdSpec {
	out := make([]cmdSpec, len(cs))
	for i, c := range cs {
		if c.Args == nil {
			c.Args = []string{}
		}
		if c.Flags == nil {
			c.Flags = []flagSpec{}
		}
		c.Subcommands = filled(c.Subcommands)
		if len(c.Subcommands) == 0 {
			c.Subcommands = nil
		}
		out[i] = c
	}
	return out
}

// writeHelpJSON prints `alder help --json`.
func writeHelpJSON(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(struct {
		Version  string    `json:"version"`
		Commands []cmdSpec `json:"commands"`
	}{version.String(), filled(commandTree)})
}
```

- [ ] **Step 5: Run the tests**

Run: `go test ./cmd/alder -count=1 -run 'TestHelpJSON|TestHelp_Silo' -v`
Expected: PASS. When `TestHelpJSON_MatchesUsage` reports a flag difference, the usage string is the authority: correct the tree, never the usage string. Then check each `Summary` against the command's section in `book/11-tooling.md` and the comment above its `run…To` function, and correct any that disagree (several summaries above were written from usage strings alone).

- [ ] **Step 6: Run the package and the book**

Run: `go test ./cmd/alder ./book -count=1 2>&1 | tail -20`
Expected: PASS. A test that lists every command, or that compares help text with the book, may need the new `help`, `silos` and `silo` entries. Update that test's expectation, and the book if it lists commands (Task 13 adds the book text for `help --json`).

- [ ] **Step 7: Commit**

```bash
git add cmd/alder/helpjson.go cmd/alder/helpjson_test.go cmd/alder/help.go
git commit -m "cli: alder help --json, the command tree pinned to the usage text"
```

---

## Task 8: CI

**Files:**
- Create: `.github/install-tree-sitter.sh`
- Modify: `.github/workflows/ci.yml`

**Interfaces:**
- Consumes: `go test ./tree-sitter-alder` (Task 5) and `scripts/ts.sh` (Task 1).
- Produces: `.github/install-tree-sitter.sh`, which alder.nvim's CI (Task 13) also runs from its checkout of this repo.

- [ ] **Step 1: Write `.github/install-tree-sitter.sh`** (and `chmod +x`)

```sh
#!/bin/sh
# Installs the tree-sitter CLI that tree-sitter-alder/src is generated with, pinned so the
# regeneration check compares like with like. Puts it on PATH for later steps (GITHUB_PATH).
set -eu
version=0.27.1
case "$(uname -s)-$(uname -m)" in
  Linux-x86_64) asset=linux-x64 ;;
  Linux-aarch64) asset=linux-arm64 ;;
  Darwin-arm64) asset=macos-arm64 ;;
  Darwin-x86_64) asset=macos-x64 ;;
  *) echo "no tree-sitter $version build for $(uname -sm)" >&2; exit 1 ;;
esac
dir="${RUNNER_TEMP:-/tmp}/tree-sitter-bin"
mkdir -p "$dir"
curl -fsSL "https://github.com/tree-sitter/tree-sitter/releases/download/v$version/tree-sitter-$asset.gz" | gunzip > "$dir/tree-sitter"
chmod +x "$dir/tree-sitter"
if [ -n "${GITHUB_PATH:-}" ]; then echo "$dir" >> "$GITHUB_PATH"; fi
"$dir/tree-sitter" --version
```

- [ ] **Step 2: Add the install step to the `test` job**

In `.github/workflows/ci.yml`, job `test`, after the `actions/cache/restore@v4` step:

```yaml
      - name: tree-sitter CLI (the grammar's drift test needs it; no job skips it)
        run: .github/install-tree-sitter.sh
```

- [ ] **Step 3: Add the `grammar` job**

```yaml
  grammar:
    name: grammar (corpus, highlights, regeneration)
    runs-on: ubuntu-latest
    timeout-minutes: 10
    steps:
      - uses: actions/checkout@v4
      - name: tree-sitter CLI
        run: .github/install-tree-sitter.sh
      - name: corpus and highlight tests
        run: scripts/ts.sh test
      - name: the committed parser is the generated one
        run: |
          scripts/ts.sh generate
          git diff --exit-code tree-sitter-alder/src
```

- [ ] **Step 4: Check the workflow**

Run: `command -v actionlint >/dev/null && actionlint .github/workflows/ci.yml; go test ./cmd/alder -count=1 -run TestCI -v 2>&1 | tail -5`
Expected: no actionlint errors (when it is installed), and the repository's CI tests pass. If a test pins the job list, add `grammar` to it.

- [ ] **Step 5: Commit**

```bash
git add .github/install-tree-sitter.sh .github/workflows/ci.yml
git commit -m "ci: install the pinned tree-sitter CLI for go test; grammar job"
```

---
## Task 9: alder.nvim scaffold — filetype, parser, ftplugin, root, test harness

From here on, work happens in `~/projects/alder.nvim`, a new repository. The alder branch from Tasks 0–8 must be checked out at `~/projects/alder`, because the tests install the parser and queries from that checkout.

**Files (all in `~/projects/alder.nvim`):**
- Create: `LICENSE`, `.gitignore`, `plugin/alder.lua`, `ftplugin/alder.lua`, `lua/alder/init.lua`, `lua/alder/config.lua`, `lua/alder/parser.lua`, `lua/alder/root.lua`
- Create: `tests/minimal.lua`, `tests/run.lua`, `tests/helpers.lua`, `tests/filetype_spec.lua`, `tests/root_spec.lua`

**Interfaces:**
- Consumes: the queries of Task 6 and the captures it promises.
- Produces:
  - `require('alder.config').options` holds `{ cmd = 'alder', check_on_save = true, preview = true, path = nil, revision = nil }`, and `config.set(opts)` replaces it.
  - `require('alder').setup(opts)`
  - `require('alder.parser').install_info() -> table` and `register()`
  - `require('alder.root').find(path) -> string|nil` is the absolute, normalized app root. `require('alder.root').for_buf(buf) -> string|nil` does the same for a buffer.
  - The test harness has the global `test(name, fn)` and the helpers `H.ensure_parser()`, `H.tmpdir() -> string`, `H.wait(ms, cond) -> bool`, `H.alder_repo() -> string`, `H.capture_notify() -> messages, restore`.

- [ ] **Step 1: Create the repository**

```bash
mkdir -p ~/projects/alder.nvim && cd ~/projects/alder.nvim && git init -b main
curl -fsSL https://www.apache.org/licenses/LICENSE-2.0.txt -o LICENSE
printf '.tests/\n' > .gitignore
git add LICENSE .gitignore && git commit -m "Apache-2.0 license"
git switch -c v0.1
```

- [ ] **Step 2: Write the test harness**

`tests/minimal.lua`:

```lua
-- tests/minimal.lua: a bare Neovim with nvim-treesitter (main) and this plugin.
-- NVIM_TS points at an nvim-treesitter checkout (default: the lazy.nvim one), ALDER_REPO at an
-- alder checkout whose tree-sitter-alder/ is installed as the parser (default ~/projects/alder).
local here = vim.fn.fnamemodify(debug.getinfo(1, 'S').source:sub(2), ':p:h:h')
local ts = os.getenv('NVIM_TS') or vim.fn.expand('~/.local/share/nvim/lazy/nvim-treesitter')
local data = here .. '/.tests'
vim.opt.rtp = { here, ts, vim.env.VIMRUNTIME }
vim.opt.packpath = {}
vim.opt.swapfile = false
vim.cmd('filetype plugin indent on')
require('nvim-treesitter').setup({ install_dir = data .. '/site' })
vim.cmd('runtime plugin/alder.lua')
require('alder').setup({ path = os.getenv('ALDER_REPO') or vim.fn.expand('~/projects/alder') })
```

`tests/run.lua`:

```lua
-- tests/run.lua: nvim --headless -u tests/minimal.lua -l tests/run.lua [pattern]
-- Loads tests/*_spec.lua, runs each `test(name, fn)` whose name matches the pattern, prints
-- ok/FAIL per test, and exits non-zero when any fails.
local here = vim.fn.fnamemodify(debug.getinfo(1, 'S').source:sub(2), ':p:h')
package.path = here .. '/?.lua;' .. package.path
local cases = {}
_G.test = function(name, fn) table.insert(cases, { name = name, fn = fn }) end
for _, f in ipairs(vim.fn.glob(here .. '/*_spec.lua', false, true)) do dofile(f) end
local pattern = _G.arg and _G.arg[1] or os.getenv('ALDER_TEST')
local failed, ran = 0, 0
for _, c in ipairs(cases) do
  if not pattern or c.name:find(pattern, 1, true) then
    ran = ran + 1
    local ok, err = xpcall(c.fn, debug.traceback)
    io.stdout:write((ok and 'ok   ' or 'FAIL ') .. c.name .. '\n')
    if not ok then
      failed = failed + 1
      io.stdout:write(tostring(err) .. '\n')
    end
    vim.cmd('silent! %bwipeout!')
  end
end
io.stdout:write(('%d tests, %d failed\n'):format(ran, failed))
os.exit(failed == 0 and ran > 0 and 0 or 1)
```

`tests/helpers.lua`:

```lua
local H = {}

function H.alder_repo()
  return os.getenv('ALDER_REPO') or vim.fn.expand('~/projects/alder')
end

local installed = false
-- ensure_parser compiles and installs the alder parser and links its queries from ALDER_REPO.
function H.ensure_parser()
  if installed then return end
  local ok = require('nvim-treesitter').install({ 'alder' }, { force = true }):wait(300000)
  assert(ok, 'installing the alder parser failed (see :messages / nvim-treesitter log)')
  installed = true
end

function H.tmpdir()
  local d = vim.fn.tempname()
  vim.fn.mkdir(d, 'p')
  return vim.uv.fs_realpath(d)
end

function H.wait(ms, cond)
  return vim.wait(ms, cond, 20)
end

function H.write(path, text)
  vim.fn.mkdir(vim.fs.dirname(path), 'p')
  local f = assert(io.open(path, 'w'))
  f:write(text)
  f:close()
end

function H.read(path)
  local f = io.open(path, 'r')
  if not f then return nil end
  local s = f:read('*a')
  f:close()
  return s
end

-- capture_notify records vim.notify messages until restore() is called.
function H.capture_notify()
  local messages, orig = {}, vim.notify
  vim.notify = function(msg, level) table.insert(messages, { msg = msg, level = level }) end
  return messages, function() vim.notify = orig end
end

-- new_app runs `alder new <dir>/demo` and returns the app root.
function H.new_app()
  local root = H.tmpdir() .. '/demo'
  local r = vim.system({ 'alder', 'new', root }, { text = true }):wait()
  assert(r.code == 0, 'alder new failed: ' .. (r.stderr or ''))
  return root
end

-- copy_tree copies a directory recursively (fixtures from the alder repo's testdata).
function H.copy_tree(src, dst)
  vim.fn.mkdir(dst, 'p')
  for name, typ in vim.fs.dir(src) do
    local s, d = src .. '/' .. name, dst .. '/' .. name
    if typ == 'directory' then H.copy_tree(s, d) else vim.uv.fs_copyfile(s, d) end
  end
end

return H
```

- [ ] **Step 3: Write the failing specs**

`tests/filetype_spec.lua`:

```lua
local H = require('helpers')

test('filetype: .alder files are alder', function()
  assert(vim.filetype.match({ filename = 'x/app/Home.alder' }) == 'alder')
end)

test('parser: registered with location and queries from the alder repo', function()
  local info = require('alder.parser').install_info()
  assert(info.location == 'tree-sitter-alder', vim.inspect(info))
  assert(info.queries == 'tree-sitter-alder/queries/neovim', vim.inspect(info))
  assert(info.path == vim.fs.normalize(H.alder_repo()), vim.inspect(info))
end)

test('parser: without path it downloads tapegram/alder', function()
  local config = require('alder.config')
  local saved = config.options
  config.set({ revision = 'v9.9.9' })
  local info = require('alder.parser').install_info()
  config.options = saved
  assert(info.url == 'https://github.com/tapegram/alder' and info.revision == 'v9.9.9' and info.path == nil, vim.inspect(info))
end)

test('queries: all six compile against the installed parser', function()
  H.ensure_parser()
  for _, name in ipairs({ 'highlights', 'locals', 'indents', 'folds', 'injections', 'textobjects' }) do
    local ok, q = pcall(vim.treesitter.query.get, 'alder', name)
    assert(ok and q, name .. ': ' .. tostring(q))
  end
end)

test('highlights: captures at known positions', function()
  H.ensure_parser()
  local file = H.tmpdir() .. '/Demo.alder'
  H.write(file, 'module Demo\n\nview : Int -> Html Never\nview n = <p class="x">{n}</p>\n')
  vim.cmd.edit(file)
  local buf = vim.api.nvim_get_current_buf()
  assert(vim.bo[buf].filetype == 'alder')
  vim.treesitter.start(buf, 'alder')
  vim.treesitter.get_parser(buf):parse()
  local function has(row, col, capture)
    for _, c in ipairs(vim.treesitter.get_captures_at_pos(buf, row, col)) do
      if c.capture == capture then return true end
    end
    local names = vim.tbl_map(function(c) return c.capture end, vim.treesitter.get_captures_at_pos(buf, row, col))
    error(('(%d,%d): want @%s, have %s'):format(row, col, capture, vim.inspect(names)))
  end
  has(0, 0, 'keyword.import')
  has(2, 0, 'function')
  has(2, 7, 'type')
  has(3, 10, 'tag')
  has(3, 12, 'tag.attribute')
  has(3, 18, 'string')
end)

test('ftplugin: comments, indentation and folds', function()
  H.ensure_parser()
  local file = H.tmpdir() .. '/Demo.alder'
  H.write(file, 'module Demo\n')
  vim.cmd.edit(file)
  assert(vim.bo.commentstring == '-- %s')
  assert(vim.bo.shiftwidth == 2 and vim.bo.expandtab)
  assert(vim.wo.foldmethod == 'expr' and vim.wo.foldexpr:find('treesitter'))
  assert(vim.bo.indentexpr:find('nvim%-treesitter'))
end)
```

`tests/root_spec.lua`:

```lua
local H = require('helpers')
local root = require('alder.root')

test('root: nearest ancestor holding app/', function()
  local dir = H.tmpdir()
  H.write(dir .. '/app/Home.alder', 'module Home\n')
  H.write(dir .. '/app/Admin/Post/Index.alder', 'module Admin.Post.Index\n')
  assert(root.find(dir .. '/app/Home.alder') == dir)
  assert(root.find(dir .. '/app/Admin/Post/Index.alder') == dir)
end)

-- Review Focus 2: on a case-insensitive file system (macOS) a stat of app/app finds app/App,
-- so a module directory named App must not make app/ look like the root.
test('root: a module directory named App is not the app directory', function()
  local dir = H.tmpdir()
  H.write(dir .. '/app/App.alder', 'module App\n')
  H.write(dir .. '/app/App/Settings.alder', 'module App.Settings\n')
  assert(root.find(dir .. '/app/App/Settings.alder') == dir, root.find(dir .. '/app/App/Settings.alder'))
end)

test('root: none above a stray file', function()
  local dir = H.tmpdir()
  H.write(dir .. '/x/Stray.alder', 'module Stray\n')
  assert(root.find(dir .. '/x/Stray.alder') == nil)
end)
```

- [ ] **Step 4: Run them to see them fail**

Run: `cd ~/projects/alder.nvim && nvim --headless -u tests/minimal.lua -l tests/run.lua`
Expected: errors loading `alder` (`module 'alder' not found`), and exit 1.

- [ ] **Step 5: Write the plugin files**

`lua/alder/config.lua`:

```lua
local M = {}

M.defaults = {
  cmd = 'alder', -- the alder binary
  check_on_save = true, -- run `alder check` after writing an .alder buffer
  preview = true, -- generators show their diff before writing
  path = nil, -- an alder checkout to install the parser from (grammar development)
  revision = nil, -- a tag of tapegram/alder to install the parser from, e.g. 'v0.2.0'
}

M.options = vim.deepcopy(M.defaults)

function M.set(opts)
  M.options = vim.tbl_deep_extend('force', vim.deepcopy(M.defaults), opts or {})
end

return M
```

`lua/alder/init.lua`:

```lua
local M = {}

function M.setup(opts)
  require('alder.config').set(opts)
  require('alder.parser').register()
end

return M
```

`lua/alder/parser.lua`:

```lua
local M = {}

-- install_info is nvim-treesitter main's description of the alder parser. `queries` is relative
-- to the repository root, also when `path` points at a local checkout (install.lua), so `path`
-- is the alder checkout itself, not its tree-sitter-alder/ directory.
function M.install_info()
  local o = require('alder.config').options
  local info = { location = 'tree-sitter-alder', queries = 'tree-sitter-alder/queries/neovim' }
  if o.path then
    info.path = vim.fs.normalize(o.path)
  else
    info.url = 'https://github.com/tapegram/alder'
    info.revision = o.revision
  end
  return info
end

-- register adds alder to nvim-treesitter's parser table. It runs at startup and on every
-- `User TSUpdate` (nvim-treesitter reloads its table before installing).
function M.register()
  local ok, parsers = pcall(require, 'nvim-treesitter.parsers')
  if not ok then return end
  parsers.alder = { install_info = M.install_info() }
end

return M
```

`lua/alder/root.lua`:

```lua
local M = {}

-- has_app reports whether dir has an entry named exactly `app` that is a directory. It reads
-- the listing instead of stat-ing dir/app: on a case-insensitive file system that stat
-- succeeds for a module directory named App.
local function has_app(dir)
  local handle = vim.uv.fs_scandir(dir)
  if not handle then return false end
  while true do
    local name, typ = vim.uv.fs_scandir_next(handle)
    if not name then return false end
    if name == 'app' then
      return typ == 'directory' or vim.fn.isdirectory(dir .. '/app') == 1
    end
  end
end

-- find returns the app root for a file or directory: its nearest ancestor holding app/.
function M.find(path)
  path = vim.fs.normalize(vim.fn.fnamemodify(path, ':p'))
  local dir = vim.fn.isdirectory(path) == 1 and path or vim.fs.dirname(path)
  dir = dir:gsub('/$', '')
  while dir and dir ~= '' do
    if has_app(dir) then return dir end
    local parent = vim.fs.dirname(dir)
    if parent == dir then return nil end
    dir = parent
  end
  return nil
end

function M.for_buf(buf)
  local name = vim.api.nvim_buf_get_name(buf or 0)
  if name == '' or name:match('^%a+://') then return M.find(vim.fn.getcwd()) end
  return M.find(name)
end

return M
```

`plugin/alder.lua` (Tasks 10–12 add the commands to it):

```lua
if vim.g.loaded_alder then return end
vim.g.loaded_alder = true

vim.filetype.add({ extension = { alder = 'alder' } })

local group = vim.api.nvim_create_augroup('alder', { clear = true })

vim.api.nvim_create_autocmd('User', {
  group = group,
  pattern = 'TSUpdate',
  callback = function() require('alder.parser').register() end,
})

require('alder.parser').register()
```

`ftplugin/alder.lua`:

```lua
if vim.b.did_ftplugin_alder then return end
vim.b.did_ftplugin_alder = true

vim.bo.commentstring = '-- %s'
vim.bo.comments = ':--'
vim.bo.shiftwidth = 2
vim.bo.tabstop = 2
vim.bo.softtabstop = 2
vim.bo.expandtab = true
-- generators edit files on disk; reload them without a W11 prompt (they refuse while modified)
vim.bo.autoread = true

if pcall(vim.treesitter.start) then
  vim.wo[0][0].foldmethod = 'expr'
  vim.wo[0][0].foldexpr = 'v:lua.vim.treesitter.foldexpr()'
  vim.wo[0][0].foldlevel = 99
  if pcall(require, 'nvim-treesitter') then
    vim.bo.indentexpr = "v:lua.require'nvim-treesitter'.indentexpr()"
  end
end

vim.b.undo_ftplugin = (vim.b.undo_ftplugin and (vim.b.undo_ftplugin .. ' | ') or '')
  .. 'setl commentstring< comments< shiftwidth< tabstop< softtabstop< expandtab< autoread< indentexpr<'
  .. ' | unlet! b:did_ftplugin_alder'
```

- [ ] **Step 6: Run the specs**

Run: `nvim --headless -u tests/minimal.lua -l tests/run.lua`
Expected: every test prints `ok`, then `9 tests, 0 failed`, and the exit code is 0. The first run compiles the parser in `~/projects/alder/tree-sitter-alder` (which is gitignored) and links the queries into `.tests/site/queries/alder`. If a highlight capture is missing, fix `queries/neovim/highlights.scm` in the alder repo (its pattern order, or a field name) and commit it there as `queries: …`. The plugin test is that query's Neovim check.

- [ ] **Step 7: Commit**

```bash
git add -A && git commit -m "alder.nvim: filetype, parser registration, ftplugin, app root, test harness"
```

---

## Task 10: Diagnostics on save, `:AlderCheck`, `:AlderFix`, `:AlderExplain`

**Files:**
- Create: `lua/alder/cli.lua`, `lua/alder/check.lua`, `tests/check_spec.lua`
- Modify: `plugin/alder.lua`

**Interfaces:**
- Consumes: `root.find`, `root.for_buf`, `config.options`.
- Produces:
  - `cli.run(args, { cwd, stdin? }, on_done) -> handle|nil`, where `on_done(res)` runs on the main loop with `res = { code, stdout, stderr, objects = {decoded JSON lines}, json = objects[1] }`.
  - `cli.report(args, res, { bug_on_2 = true })` shows a failure verbatim.
  - `cli.available() -> bool` notifies once when `alder` is missing.
  - `check.ns` is the namespace `alder`.
  - `check.to_diagnostic(d) -> vim.Diagnostic`
  - `check.publish(app_root, objects) -> qf_items`
  - `check.run(app_root, { open_qf?, on_done? })`
  - `check.apply_fix(buf, fix) -> bool, err`
  - `check.fix_at_cursor() -> bool`
  - `check.explain(code?)`
  - Commands `:AlderCheck`, `:AlderFix`, `:AlderExplain [code]`, and a `BufWritePost *.alder` autocmd.

- [ ] **Step 1: Write the failing specs** `tests/check_spec.lua`

```lua
local H = require('helpers')
local check = require('alder.check')

-- the E0102 corpus case from the alder repo: one diagnostic with a fix
local function fixture()
  local app = H.tmpdir() .. '/e0102'
  H.copy_tree(H.alder_repo() .. '/testdata/corpus/E0102-maybe-field', app)
  local expected = H.read(app .. '/expected.diag')
  local file = expected:match('(app/[%w/]+%.alder):%d+:%d+ E0102')
  assert(file, 'no E0102 line in expected.diag:\n' .. expected)
  return app, file
end

local function diags(buf, code)
  return vim.tbl_filter(function(d) return code == nil or d.code == code end,
    vim.diagnostic.get(buf, { namespace = check.ns }))
end

test('check: 1-based byte columns become 0-based, ends exclusive', function()
  local d = check.to_diagnostic({ code = 'E0001', severity = 'warning', file = 'app/A.alder', line = 3, col = 5,
    endLine = 4, endCol = 2, message = 'm', fix = { line = 3, col = 5, endLine = 3, endCol = 6, replace = 'x' } })
  assert(d.lnum == 2 and d.col == 4 and d.end_lnum == 3 and d.end_col == 1, vim.inspect(d))
  assert(d.severity == vim.diagnostic.severity.WARN and d.code == 'E0001' and d.source == 'alder')
  assert(d.user_data.fix.replace == 'x')
end)

test('check: a file not open goes to the quickfix list; an open one gets diagnostics', function()
  local app, file = fixture()
  local done = false
  check.run(app, { on_done = function() done = true end })
  assert(H.wait(20000, function() return done end), 'alder check did not finish')
  local qf = vim.fn.getqflist()
  assert(#qf >= 1 and qf[1].text:find('E0102'), vim.inspect(qf))

  vim.cmd.edit(app .. '/' .. file)
  local buf = vim.api.nvim_get_current_buf()
  done = false
  check.run(app, { on_done = function() done = true end })
  assert(H.wait(20000, function() return done end))
  assert(#diags(buf, 'E0102') == 1, vim.inspect(diags(buf)))
  for _, item in ipairs(vim.fn.getqflist()) do
    assert(not item.text:find('E0102'), 'an open file is still in the quickfix list')
  end
end)

test('fix: :AlderFix applies the fix under the cursor; saving clears the diagnostic', function()
  local app, file = fixture()
  vim.cmd.edit(app .. '/' .. file)
  local buf = vim.api.nvim_get_current_buf()
  local done = false
  check.run(app, { on_done = function() done = true end })
  assert(H.wait(20000, function() return done end))
  local d = assert(diags(buf, 'E0102')[1], 'no E0102')
  vim.api.nvim_win_set_cursor(0, { d.lnum + 1, d.col })
  assert(check.fix_at_cursor(), 'no fix applied')
  local text = table.concat(vim.api.nvim_buf_get_lines(buf, 0, -1, false), '\n')
  assert(text:find('(match user { Just x -> x.name, Nothing -> ?default }) }', 1, true), text)
  vim.cmd('silent write') -- BufWritePost runs alder check (check_on_save)
  -- Review Focus 3: the fixed file loses its diagnostic
  assert(H.wait(20000, function() return #diags(buf, 'E0102') == 0 end), vim.inspect(diags(buf)))
end)

-- Review Focus 3: a superseded run must not publish late.
test('check: a newer run supersedes an older one', function()
  local app, file = fixture()
  vim.cmd.edit(app .. '/' .. file)
  local buf = vim.api.nvim_get_current_buf()
  local first, second = false, false
  check.run(app, { on_done = function() first = true end })
  check.run(app, { on_done = function() second = true end })
  assert(H.wait(20000, function() return second end), 'the second run did not finish')
  vim.wait(500)
  assert(not first, 'the superseded run still reported')
  assert(#diags(buf, 'E0102') == 1)
end)

-- Review Focus 4: the buffer is opened through a symlink; the CLI reports the real path.
test('check: diagnostics reach a buffer opened through a symlink', function()
  local app, file = fixture()
  local link = H.tmpdir() .. '/link'
  assert(vim.uv.fs_symlink(app, link))
  vim.cmd.edit(link .. '/' .. file)
  local buf = vim.api.nvim_get_current_buf()
  local done = false
  check.run(require('alder.root').for_buf(buf), { on_done = function() done = true end })
  assert(H.wait(20000, function() return done end))
  assert(#diags(buf, 'E0102') == 1, vim.inspect(diags(buf)))
end)

-- Review Focus 5: byte columns after multi-byte characters.
test('fix: byte columns after multi-byte text', function()
  local buf = vim.api.nvim_create_buf(false, true)
  vim.api.nvim_buf_set_lines(buf, 0, -1, false, { 'x = "é→" ++ nmae' })
  -- `nmae` starts at byte 16 (1-based): "é" is 2 bytes and "→" 3
  local ok = check.apply_fix(buf, { line = 1, col = 16, endLine = 1, endCol = 20, replace = 'name' })
  assert(ok)
  assert(vim.api.nvim_buf_get_lines(buf, 0, 1, false)[1] == 'x = "é→" ++ name')
end)

test('fix: a stale fix is refused, not applied elsewhere', function()
  local buf = vim.api.nvim_create_buf(false, true)
  vim.api.nvim_buf_set_lines(buf, 0, -1, false, { 'short' })
  local ok, err = check.apply_fix(buf, { line = 4, col = 1, endLine = 4, endCol = 3, replace = 'x' })
  assert(not ok and err:find('save to re%-check'), tostring(err))
end)

test('explain: the compiler text in a float', function()
  local before = #vim.api.nvim_list_wins()
  check.explain('E0102')
  assert(H.wait(10000, function() return #vim.api.nvim_list_wins() > before end), 'no float opened')
end)

test('errors: no app root is said once, plainly', function()
  local messages, restore = H.capture_notify()
  local dir = H.tmpdir()
  H.write(dir .. '/Stray.alder', 'module Stray\n')
  vim.cmd.edit(dir .. '/Stray.alder')
  vim.cmd('AlderCheck')
  restore()
  assert(messages[1] and messages[1].msg == 'alder: no `app/` directory above this file', vim.inspect(messages))
end)
```

- [ ] **Step 2: Run them to see them fail**

Run: `nvim --headless -u tests/minimal.lua -l tests/run.lua check`
Expected: FAIL with `module 'alder.check' not found`.

- [ ] **Step 3: Write `lua/alder/cli.lua`**

```lua
local M = {}

local function opts() return require('alder.config').options end

local warned = false
-- available reports whether the alder binary is on PATH, and says how to get it once.
function M.available()
  if vim.fn.executable(opts().cmd) == 1 then return true end
  if not warned then
    warned = true
    vim.notify(('alder: `%s` is not on PATH. Build it from a checkout: go build -o ~/bin/alder ./cmd/alder'):format(opts().cmd),
      vim.log.levels.ERROR)
  end
  return false
end

-- decode returns every JSON object printed one per line on stdout (JSONL, or one object).
function M.decode(stdout)
  local out = {}
  for line in vim.gsplit(stdout or '', '\n', { plain = true }) do
    if line:match('^%s*{') then
      local ok, v = pcall(vim.json.decode, line, { luanil = { object = true, array = true } })
      if ok and type(v) == 'table' then table.insert(out, v) end
    end
  end
  return out
end

-- run starts `alder <args>` and calls on_done(res) on the main loop.
function M.run(args, o, on_done)
  if not M.available() then return nil end
  local argv = { opts().cmd }
  vim.list_extend(argv, args)
  return vim.system(argv, { cwd = o.cwd, stdin = o.stdin, text = true }, function(r)
    local objects = M.decode(r.stdout)
    local res = { code = r.code, signal = r.signal, stdout = r.stdout or '', stderr = r.stderr or '',
      objects = objects, json = objects[1] }
    vim.schedule(function() on_done(res) end)
  end)
end

-- scratch opens a bottom split on a new scratch buffer holding lines and returns the buffer.
function M.scratch(name, lines, filetype)
  vim.cmd('botright new')
  local buf = vim.api.nvim_get_current_buf()
  vim.bo[buf].buftype = 'nofile'
  vim.bo[buf].bufhidden = 'wipe'
  vim.bo[buf].swapfile = false
  pcall(vim.api.nvim_buf_set_name, buf, name)
  vim.api.nvim_buf_set_lines(buf, 0, -1, false, lines)
  vim.bo[buf].modifiable = false
  if filetype then vim.bo[buf].filetype = filetype end
  vim.keymap.set('n', 'q', '<cmd>close<cr>', { buffer = buf, nowait = true })
  return buf
end

-- report shows a failed run without rewording it: a JSON `error` as is, a usage or internal
-- failure (exit 2) with its stderr in a split, anything else as its stderr.
function M.report(args, res, o)
  o = o or {}
  if res.json and res.json.error then
    vim.notify(res.json.error, vim.log.levels.ERROR)
  elseif res.code == 2 and o.bug_on_2 ~= false then
    local v = vim.system({ opts().cmd, 'version' }, { text = true }):wait()
    local lines = { 'This may be a compiler bug; `alder version` and the command are in this buffer.', '',
      '$ alder ' .. table.concat(args, ' '), '' }
    vim.list_extend(lines, vim.split(vim.trim(res.stderr ~= '' and res.stderr or res.stdout), '\n', { plain = true }))
    vim.list_extend(lines, { '', '$ alder version', vim.trim(v.stdout or '') })
    M.scratch('alder://error', lines)
  else
    local text = vim.trim(res.stderr ~= '' and res.stderr or res.stdout)
    vim.notify(text ~= '' and text or ('alder ' .. args[1] .. ' exited ' .. res.code), vim.log.levels.ERROR)
  end
end

return M
```

- [ ] **Step 4: Write `lua/alder/check.lua`**

```lua
local cli = require('alder.cli')
local root = require('alder.root')

local M = {}
M.ns = vim.api.nvim_create_namespace('alder')

local function real(p) return vim.uv.fs_realpath(p) or vim.fs.normalize(p) end

-- to_diagnostic maps one `alder check --json` object. Its columns are 1-based bytes with
-- exclusive ends; Neovim's are 0-based bytes.
function M.to_diagnostic(d)
  local line, col = d.line or 1, d.col or 1
  return {
    lnum = line - 1,
    col = col - 1,
    end_lnum = (d.endLine or line) - 1,
    end_col = (d.endCol or col) - 1,
    severity = d.severity == 'warning' and vim.diagnostic.severity.WARN or vim.diagnostic.severity.ERROR,
    message = d.message,
    code = d.code,
    source = 'alder',
    user_data = { fix = d.fix },
  }
end

-- publish replaces the alder diagnostics of every loaded .alder buffer of the app (so a fixed
-- file loses its old ones) and puts the diagnostics of files that are not open in the
-- quickfix list. Paths are compared after resolving symlinks.
function M.publish(app_root, objects)
  local app_real = real(app_root)
  local by_file = {}
  for _, d in ipairs(objects) do
    if d.code and d.file then
      local p = real(app_root .. '/' .. d.file)
      by_file[p] = by_file[p] or {}
      table.insert(by_file[p], d)
    end
  end
  local open = {}
  for _, buf in ipairs(vim.api.nvim_list_bufs()) do
    local name = vim.api.nvim_buf_get_name(buf)
    if vim.api.nvim_buf_is_loaded(buf) and name:match('%.alder$') then
      local r = root.find(name)
      if r and real(r) == app_real then
        local p = real(name)
        open[p] = true
        vim.diagnostic.set(M.ns, buf, vim.tbl_map(M.to_diagnostic, by_file[p] or {}))
      end
    end
  end
  local items = {}
  for p, ds in pairs(by_file) do
    if not open[p] then
      for _, d in ipairs(ds) do
        table.insert(items, { filename = p, lnum = d.line, col = d.col, end_lnum = d.endLine, end_col = d.endCol,
          text = d.code .. ' ' .. d.message, type = d.severity == 'warning' and 'W' or 'E' })
      end
    end
  end
  table.sort(items, function(a, b)
    if a.filename ~= b.filename then return a.filename < b.filename end
    return a.lnum < b.lnum
  end)
  vim.fn.setqflist({}, 'r', { title = 'alder check', items = items })
  return items
end

local runs = {} -- app root → { gen, handle }
local generation = 0

-- run checks the app. A newer run for the same app cancels the older one, whose result is
-- then dropped even if it arrives.
function M.run(app_root, o)
  o = o or {}
  local prev = runs[app_root]
  if prev and prev.handle then pcall(function() prev.handle:kill('sigterm') end) end
  generation = generation + 1
  local mine = { gen = generation }
  runs[app_root] = mine
  local args = { 'check', '--json', app_root }
  mine.handle = cli.run(args, { cwd = app_root }, function(res)
    if runs[app_root] ~= mine then return end
    runs[app_root] = nil
    if res.code == 2 or (res.code ~= 0 and #res.objects == 0) then
      cli.report(args, res)
      return
    end
    local items = M.publish(app_root, res.objects)
    if o.open_qf and #items > 0 then vim.cmd('copen') end
    if o.on_done then o.on_done(res) end
  end)
  if not mine.handle then runs[app_root] = nil end
end

-- apply_fix replaces the fix's byte range (1-based, exclusive end) with its text.
function M.apply_fix(buf, fix)
  local ok, err = pcall(vim.api.nvim_buf_set_text, buf, fix.line - 1, fix.col - 1, fix.endLine - 1, fix.endCol - 1,
    vim.split(fix.replace, '\n', { plain = true }))
  if not ok then return false, 'alder: the fix no longer matches the buffer; save to re-check (' .. tostring(err) .. ')' end
  return true
end

local function at_cursor(buf)
  local row, col = unpack(vim.api.nvim_win_get_cursor(0))
  row = row - 1
  local here, line = nil, nil
  for _, d in ipairs(vim.diagnostic.get(buf, { namespace = M.ns, lnum = row })) do
    line = line or d
    local ends = d.end_lnum > row or col < d.end_col or d.end_col == d.col
    if col >= d.col and ends then here = here or d end
  end
  return here, line
end

-- fix_at_cursor applies the fix of the diagnostic under the cursor (else the first one with a
-- fix on the cursor's line).
function M.fix_at_cursor()
  local buf = vim.api.nvim_get_current_buf()
  local row = vim.api.nvim_win_get_cursor(0)[1] - 1
  local here = at_cursor(buf)
  local pick = here and here.user_data and here.user_data.fix and here
  if not pick then
    for _, d in ipairs(vim.diagnostic.get(buf, { namespace = M.ns, lnum = row })) do
      if d.user_data and d.user_data.fix then pick = d break end
    end
  end
  if not pick then
    vim.notify('alder: no fix under the cursor', vim.log.levels.WARN)
    return false
  end
  local ok, err = M.apply_fix(buf, pick.user_data.fix)
  if not ok then vim.notify(err, vim.log.levels.ERROR) end
  return ok
end

-- explain shows `alder explain <code>` for the given code or the diagnostic under the cursor.
function M.explain(code)
  if not code or code == '' then
    local here, line = at_cursor(vim.api.nvim_get_current_buf())
    local d = here or line
    code = d and d.code
  end
  if not code then
    vim.notify('alder: no diagnostic under the cursor', vim.log.levels.WARN)
    return
  end
  cli.run({ 'explain', code }, { cwd = vim.fn.getcwd() }, function(res)
    if res.code ~= 0 then
      cli.report({ 'explain', code }, res, { bug_on_2 = false })
      return
    end
    vim.lsp.util.open_floating_preview(vim.split(vim.trim(res.stdout), '\n', { plain = true }), 'markdown',
      { border = 'rounded', max_width = 80, focus_id = 'alder-explain' })
  end)
end

-- for_current runs a check for the current buffer's app, or says there is none.
function M.for_current(o)
  local r = root.for_buf(0)
  if not r then
    vim.notify('alder: no `app/` directory above this file', vim.log.levels.ERROR)
    return
  end
  M.run(r, o)
end

return M
```

- [ ] **Step 5: Add the commands and the save autocmd to `plugin/alder.lua`** (append)

```lua
vim.api.nvim_create_autocmd('BufWritePost', {
  group = group,
  pattern = '*.alder',
  callback = function(ev)
    if not require('alder.config').options.check_on_save then return end
    local r = require('alder.root').for_buf(ev.buf)
    if r then require('alder.check').run(r) end
  end,
})

vim.api.nvim_create_user_command('AlderCheck', function()
  require('alder.check').for_current({ open_qf = true })
end, { desc = 'alder check: diagnostics in buffers, other files in the quickfix list' })

vim.api.nvim_create_user_command('AlderFix', function()
  require('alder.check').fix_at_cursor()
end, { desc = 'apply the compiler fix under the cursor' })

vim.api.nvim_create_user_command('AlderExplain', function(o)
  require('alder.check').explain(o.args)
end, { nargs = '?', desc = 'alder explain <code> (default: the diagnostic under the cursor)' })
```

Saving a file with no app root above it says nothing on save. Only `:AlderCheck` reports `alder: no \`app/\` directory above this file`, because a stray `.alder` file is not an error worth a message on every write.

- [ ] **Step 6: Run the specs**

Run: `nvim --headless -u tests/minimal.lua -l tests/run.lua`
Expected: all tests `ok`. If the E0102 fixture's fix text differs, read `internal/check/check_test.go` `TestFixesApply` in the alder repo for the current expected substring and use it.

- [ ] **Step 7: Commit**

```bash
git add -A && git commit -m "alder.nvim: check-on-save diagnostics, quickfix, :AlderFix, :AlderExplain"
```

---

## Task 11: `:Alder`, completion, generators with preview, `:AlderNew`

**Files:**
- Create: `lua/alder/commands.lua`, `lua/alder/generate.lua`, `tests/commands_spec.lua`, `tests/generate_spec.lua`
- Modify: `plugin/alder.lua`

**Interfaces:**
- Consumes: `alder help --json` (Task 7: `{version, commands: [{name, summary, usage?, args, flags: [{name, value?, multi?, summary}], subcommands?}]}`), `cli.run`, `cli.report`, `cli.scratch`, `check.run`, `check.publish`, `root.for_buf`, and the generator JSON `{files: [{path, action}], diff, diagnostics, holes, migration?, written, error?}` with `action` one of `created`, `edited`, `removed`.
- Produces:
  - `commands.tree() -> table|nil`, cached per `alder version`
  - `commands.find(path_words) -> node|nil`
  - `commands.complete(arglead, cmdline) -> string[]`
  - `commands.alder(fargs)`
  - `generate.modified(app_root) -> buf[]`
  - `generate.run(fargs, { preview?, stdin?, on_done? })`
  - `generate.render(json) -> string[]`
  - `generate.apply_preview(buf)`
  - `generate.after_write(app_root, json)`
  - `generate.new_interactive()`
  - Commands `:Alder` and `:AlderNew`.
  - The preview buffer is named `alder://preview`, and `<CR>` and `q` are mapped in it.

- [ ] **Step 1: Check the CLI by hand**

Run (in a scratch dir): `alder new /tmp/m6a-demo && cd /tmp/m6a-demo && alder new page Hello /hello --json --dry-run --dir . | head -c 600; ls app`
Expected: one JSON object whose `files` includes `{"path":"app/Hello.alder","action":"created"}` and whose `diff` is a non-empty unified diff; nothing written. If the generator needs another flag for a plain page (such as `--blank`), use that form in the test below and in the README example. Remove `/tmp/m6a-demo` afterwards.

- [ ] **Step 2: Write the failing specs**

`tests/commands_spec.lua`:

```lua
local commands = require('alder.commands')

local function has(list, x) return vim.tbl_contains(list, x) end

test('completion: commands, generators and flags from alder help --json', function()
  assert(commands.tree(), 'alder help --json failed: is the alder on PATH built from the m6a branch?')
  local top = commands.complete('', 'Alder ')
  assert(has(top, 'check') and has(top, 'new') and has(top, 'fill'), vim.inspect(top))
  local gens = commands.complete('', 'Alder new ')
  assert(has(gens, 'page') and has(gens, 'table'), vim.inspect(gens))
  local flags = commands.complete('--l', 'Alder new page --l')
  assert(has(flags, '--list') and not has(flags, '--json'), vim.inspect(flags))
  assert(#commands.complete('', 'Alder new page --list ') == 0, 'a flag value is free text')
end)
```

`tests/generate_spec.lua`:

```lua
local H = require('helpers')
local generate = require('alder.generate')

local function preview_buf()
  for _, b in ipairs(vim.api.nvim_list_bufs()) do
    if vim.api.nvim_buf_get_name(b):match('alder://preview$') then return b end
  end
end

test('generate: :Alder new page previews, then <CR> applies and opens the page', function()
  local app = H.new_app()
  vim.cmd.edit(app .. '/app/App.alder')
  vim.cmd('Alder new page Hello /hello')
  assert(H.wait(20000, function() return preview_buf() ~= nil end), 'no preview')
  local buf = preview_buf()
  local text = table.concat(vim.api.nvim_buf_get_lines(buf, 0, -1, false), '\n')
  assert(text:find('created app/Hello.alder', 1, true), text)
  assert(text:find('page "/hello"', 1, true), text)
  assert(not vim.uv.fs_stat(app .. '/app/Hello.alder'), 'the preview wrote the file')
  generate.apply_preview(buf)
  assert(H.wait(20000, function() return vim.uv.fs_stat(app .. '/app/Hello.alder') ~= nil end), 'not applied')
  assert(H.wait(5000, function() return vim.api.nvim_buf_get_name(0):match('Hello%.alder$') end), 'the new page did not open')
end)

test('generate: refused while an .alder buffer has unsaved changes', function()
  local app = H.new_app()
  vim.cmd.edit(app .. '/app/App.alder')
  vim.api.nvim_buf_set_lines(0, -1, -1, false, { '-- unsaved' })
  local messages, restore = H.capture_notify()
  vim.cmd('Alder new page Hello /hello')
  restore()
  assert(messages[1] and messages[1].msg == 'alder: save all buffers first (:wall)', vim.inspect(messages))
  assert(preview_buf() == nil)
end)

test('generate: a refusal is shown verbatim', function()
  local app = H.new_app()
  vim.cmd.edit(app .. '/app/App.alder')
  local messages, restore = H.capture_notify()
  local done = false
  generate.run({ 'new', 'page', 'Hello', '/hello', '--list', 'NoSuchTable' }, { on_done = function() done = true end })
  assert(H.wait(20000, function() return done end))
  restore()
  local r = vim.system({ 'alder', 'new', 'page', 'Hello', '/hello', '--list', 'NoSuchTable', '--json', '--dry-run', '--dir', app },
    { text = true, cwd = app }):wait()
  local err = vim.json.decode(r.stdout).error
  assert(err and messages[1] and messages[1].msg == err, vim.inspect({ messages, err }))
end)

test('generate: render lists files, migration, holes, diagnostics and the diff', function()
  local lines = generate.render({
    files = { { path = 'app/T.alder', action = 'created' } }, diff = '--- a\n+++ b',
    migration = { 'add table t' }, holes = { { module = 'T', decl = 'x', name = 'v', type = 'Int', file = 'app/T.alder', line = 3, col = 5 } },
    diagnostics = { { file = 'app/T.alder', line = 3, col = 5, code = 'W0002', message = 'a hole' } },
  })
  local text = table.concat(lines, '\n')
  for _, want in ipairs({ 'created app/T.alder', 'add table t', '?v : Int', 'app/T.alder:3:5 W0002 a hole', '+++ b' }) do
    assert(text:find(want, 1, true), want .. ' missing from:\n' .. text)
  end
end)
```

- [ ] **Step 3: Run them to see them fail**

Run: `nvim --headless -u tests/minimal.lua -l tests/run.lua generate`
Expected: FAIL with `module 'alder.generate' not found`.

- [ ] **Step 4: Write `lua/alder/commands.lua`**

```lua
local cli = require('alder.cli')

local M = {}

local cache = { version = nil, tree = nil }

-- tree is `alder help --json`, read once per `alder version`.
function M.tree()
  local cmd = require('alder.config').options.cmd
  if vim.fn.executable(cmd) ~= 1 then return nil end
  local v = vim.system({ cmd, 'version' }, { text = true }):wait()
  local version = vim.trim(v.stdout or '')
  if cache.tree and cache.version == version then return cache.tree end
  local r = vim.system({ cmd, 'help', '--json' }, { text = true }):wait()
  if r.code ~= 0 then return nil end
  local ok, t = pcall(vim.json.decode, r.stdout, { luanil = { object = true, array = true } })
  if not ok or type(t) ~= 'table' then return nil end
  cache.version, cache.tree = version, t
  return t
end

local function child(list, name)
  for _, c in ipairs(list or {}) do
    if c.name == name then return c end
  end
end

-- find walks the tree along words (`{'new', 'page'}`) and returns the deepest command reached.
function M.find(words)
  local t = M.tree()
  if not t then return nil end
  local node, list = nil, t.commands
  for _, w in ipairs(words) do
    local c = child(list, w)
    if not c then break end
    node, list = c, c.subcommands
  end
  return node
end

local function flag(node, name)
  for _, f in ipairs(node and node.flags or {}) do
    if f.name == name then return f end
  end
end

function M.complete(arglead, cmdline)
  local t = M.tree()
  if not t then return {} end
  local words = vim.split(vim.trim(cmdline), '%s+')
  table.remove(words, 1) -- the command name
  if arglead ~= '' then table.remove(words) end
  local node, list = nil, t.commands
  for _, w in ipairs(words) do
    local c = not w:match('^%-') and child(list, w)
    if c then node, list = c, c.subcommands else list = nil end
  end
  local prev = words[#words]
  local f = prev and flag(node, prev)
  if f and f.value then return {} end
  local out = {}
  for _, c in ipairs(list or {}) do table.insert(out, c.name) end
  for _, fl in ipairs(node and node.flags or {}) do table.insert(out, fl.name) end
  return vim.tbl_filter(function(s) return vim.startswith(s, arglead) end, out)
end

local GENERATORS = { new = true, edit = true, remove = true, fill = true, regen = true }
local TERMINAL = { run = true, repl = true }

-- alder is :Alder <args…>: generators preview and apply, check publishes diagnostics,
-- long-running commands get a terminal, everything else prints its output verbatim.
function M.alder(fargs)
  local name = fargs[1]
  if not name then
    vim.notify('alder: :Alder <command> [args…]', vim.log.levels.WARN)
    return
  end
  local app_root = require('alder.root').for_buf(0)
  if name == 'check' then
    require('alder.check').for_current({ open_qf = true })
    return
  end
  if TERMINAL[name] or (name == 'logs' and vim.tbl_contains(fargs, '-f')) then
    if not cli.available() then return end
    vim.cmd('botright new')
    local argv = { require('alder.config').options.cmd }
    vim.list_extend(argv, fargs)
    vim.fn.jobstart(argv, { term = true, cwd = app_root or vim.fn.getcwd() })
    vim.cmd('startinsert')
    return
  end
  -- `new <name>` (not a generator kind) scaffolds an app, from the current directory
  local new = M.find({ 'new' })
  local scaffold = name == 'new' and fargs[2] ~= nil and not (new and child(new.subcommands, fargs[2]))
  if GENERATORS[name] and not scaffold then
    require('alder.generate').run(fargs)
    return
  end
  local cwd = (scaffold or not app_root) and vim.fn.getcwd() or app_root
  cli.run(fargs, { cwd = cwd }, function(res)
    local text = res.stdout .. (res.stderr ~= '' and ('\n' .. res.stderr) or '')
    cli.scratch('alder://' .. name, vim.split(vim.trim(text), '\n', { plain = true }))
  end)
end

return M
```

- [ ] **Step 5: Write `lua/alder/generate.lua`**

```lua
local cli = require('alder.cli')
local root = require('alder.root')

local M = {}

-- modified lists the loaded .alder buffers of the app with unsaved changes.
function M.modified(app_root)
  local out = {}
  for _, buf in ipairs(vim.api.nvim_list_bufs()) do
    local name = vim.api.nvim_buf_get_name(buf)
    if vim.api.nvim_buf_is_loaded(buf) and vim.bo[buf].modified and name:match('%.alder$') and root.find(name) == app_root then
      table.insert(out, buf)
    end
  end
  return out
end

-- render is the preview's text: the files, the migration, the holes, the diagnostics and the
-- diff, all as the compiler printed them.
function M.render(json)
  local lines = {}
  for _, f in ipairs(json.files or {}) do table.insert(lines, f.action .. ' ' .. f.path) end
  if json.migration and #json.migration > 0 then
    table.insert(lines, '')
    table.insert(lines, 'migration:')
    for _, step in ipairs(json.migration) do table.insert(lines, '  ' .. step) end
  end
  if json.holes and #json.holes > 0 then
    table.insert(lines, '')
    for _, h in ipairs(json.holes) do
      local addr = h.module .. '.' .. h.decl .. (h.arm and ('.' .. h.arm) or '')
      table.insert(lines, ('hole %s ?%s : %s  (%s:%d:%d)'):format(addr, h.name, h.type, h.file, h.line, h.col))
    end
  end
  if json.diagnostics and #json.diagnostics > 0 then
    table.insert(lines, '')
    for _, d in ipairs(json.diagnostics) do
      table.insert(lines, ('%s:%d:%d %s %s'):format(d.file, d.line, d.col, d.code, d.message))
    end
  end
  if json.diff and json.diff ~= '' then
    table.insert(lines, '')
    vim.list_extend(lines, vim.split(json.diff, '\n', { plain = true }))
  end
  return lines
end

-- after_write reloads edited buffers, opens the first created file and re-checks the app.
function M.after_write(app_root, json)
  vim.cmd('checktime')
  for _, f in ipairs(json.files or {}) do
    if f.action == 'created' and f.path:match('%.alder$') then
      vim.cmd.edit(vim.fn.fnameescape(app_root .. '/' .. f.path))
      break
    end
  end
  require('alder.check').run(app_root)
end

local pending = {} -- preview buffer → { fargs, app_root }

function M.apply_preview(buf)
  local p = pending[buf]
  if not p then return end
  pending[buf] = nil
  if vim.api.nvim_buf_is_valid(buf) then vim.api.nvim_buf_delete(buf, { force = true }) end
  M.run(p.fargs, { preview = false, app_root = p.app_root })
end

local function show_preview(app_root, fargs, json)
  local lines = { ('-- <CR> applies, q discards: alder %s'):format(table.concat(fargs, ' ')), '' }
  vim.list_extend(lines, M.render(json))
  local buf = cli.scratch('alder://preview', lines, 'diff')
  pending[buf] = { fargs = fargs, app_root = app_root }
  vim.keymap.set('n', '<CR>', function() M.apply_preview(buf) end, { buffer = buf, nowait = true })
  vim.api.nvim_create_autocmd('BufWipeout', { buffer = buf, once = true, callback = function() pending[buf] = nil end })
end

-- run is a generator (`new <kind>`, `edit`, `remove`, `fill`, `regen`). It refuses while any
-- .alder buffer of the app has unsaved changes (the CLI edits files on disk), runs from the
-- app root with --json --dir, and with preview on shows the --dry-run diff first.
function M.run(fargs, o)
  o = o or {}
  local app_root = o.app_root or root.for_buf(0)
  if not app_root then
    vim.notify('alder: no `app/` directory above this file', vim.log.levels.ERROR)
    return
  end
  if #M.modified(app_root) > 0 then
    vim.notify('alder: save all buffers first (:wall)', vim.log.levels.ERROR)
    return
  end
  local preview = o.preview
  if preview == nil then preview = require('alder.config').options.preview end
  local asked_dry = vim.tbl_contains(fargs, '--dry-run')
  local args = vim.deepcopy(fargs)
  if not vim.tbl_contains(args, '--json') then table.insert(args, '--json') end
  vim.list_extend(args, { '--dir', app_root })
  if preview and not asked_dry then table.insert(args, '--dry-run') end
  cli.run(args, { cwd = app_root, stdin = o.stdin }, function(res)
    local json = res.json
    if not json or res.code == 2 then
      cli.report(args, res)
      if o.on_done then o.on_done(res) end
      return
    end
    if json.error then
      if json.diagnostics and #json.diagnostics > 0 then require('alder.check').publish(app_root, json.diagnostics) end
      vim.notify(json.error, vim.log.levels.ERROR)
    elseif preview or asked_dry then
      show_preview(app_root, fargs, json)
    else
      M.after_write(app_root, json)
    end
    if o.on_done then o.on_done(res) end
  end)
end

-- ask runs the prompts in order, each a function(done) that calls done(value|nil); nil stops.
local function ask(steps, finish)
  local values = {}
  local function step(i)
    if i > #steps then return finish(values) end
    steps[i](function(v)
      if v == nil then return end
      table.insert(values, v)
      step(i + 1)
    end)
  end
  step(1)
end

-- new_interactive is :AlderNew: pick a generator, type its arguments, add flags, preview.
function M.new_interactive()
  local new = require('alder.commands').find({ 'new' })
  if not new then
    vim.notify('alder: `alder help --json` failed; is alder on PATH and recent enough?', vim.log.levels.ERROR)
    return
  end
  vim.ui.select(new.subcommands or {}, {
    prompt = 'alder new',
    format_item = function(c) return c.name .. ' — ' .. c.summary end,
  }, function(kind)
    if not kind then return end
    local steps = {}
    for _, a in ipairs(kind.args or {}) do
      table.insert(steps, function(done)
        vim.ui.input({ prompt = a .. ': ' }, function(v)
          if v == nil then return done(nil) end
          if v == '' and a:match('^%[') then return done({}) end
          if v == '' then return done(nil) end
          -- a variadic argument (`<field>:<Type>…`) is several words; any other is one
          done(a:match('…$') and vim.split(vim.trim(v), '%s+') or { v })
        end)
      end)
    end
    ask(steps, function(values)
      local fargs = { 'new', kind.name }
      for _, v in ipairs(values) do vim.list_extend(fargs, v) end
      local function more()
        local choices = { { name = 'done' } }
        for _, f in ipairs(kind.flags or {}) do
          if f.name ~= '--dir' and f.name ~= '--json' and f.name ~= '--dry-run' then table.insert(choices, f) end
        end
        vim.ui.select(choices, {
          prompt = 'alder new ' .. kind.name .. ': add a flag',
          format_item = function(f) return f.summary and (f.name .. (f.value and (' ' .. f.value) or '') .. ' — ' .. f.summary) or f.name end,
        }, function(f)
          if not f then return end
          if f.name == 'done' then return M.run(fargs) end
          if not f.value then
            table.insert(fargs, f.name)
            return more()
          end
          vim.ui.input({ prompt = f.name .. ' ' .. f.value .. ': ' }, function(v)
            if v and v ~= '' then vim.list_extend(fargs, { f.name, v }) end
            more()
          end)
        end)
      end
      more()
    end)
  end)
end

return M
```

- [ ] **Step 6: Add the commands to `plugin/alder.lua`** (append)

```lua
vim.api.nvim_create_user_command('Alder', function(o)
  require('alder.commands').alder(o.fargs)
end, {
  nargs = '+',
  complete = function(arglead, cmdline) return require('alder.commands').complete(arglead, cmdline) end,
  desc = 'run alder <command>: generators preview their diff first',
})

vim.api.nvim_create_user_command('AlderNew', function()
  require('alder.generate').new_interactive()
end, { desc = 'pick a generator, type its arguments, preview, apply' })
```

- [ ] **Step 7: Run the specs**

Run: `nvim --headless -u tests/minimal.lua -l tests/run.lua`
Expected: all `ok`. The completion test needs an `alder` on PATH built from the `m6a-grammar-nvim` branch: `cd ~/projects/alder && go build -o ~/bin/alder ./cmd/alder`.

- [ ] **Step 8: Commit**

```bash
git add -A && git commit -m "alder.nvim: :Alder with completion from alder help --json, preview-then-apply generators, :AlderNew"
```

---

## Task 12: `:AlderFill`

**Files:**
- Create: `lua/alder/fill.lua`, `tests/fill_spec.lua`
- Modify: `plugin/alder.lua`

**Interfaces:**
- Consumes:
  - `alder fill --list --json --dir <root>` prints `{holes: [{module, decl, arm?, name, type, file, line, col}]}`, where `name` has no `?`, `file` is relative to the root, and line and col are 1-based bytes at the `?`.
  - `alder fill <Module>.<decl>[.<Arm>] - --hole ?name --json --dir <root>` and `alder fill --decl <Module>.<decl> - --json --dir <root>` read the expression on stdin. They print the generator JSON and exit 1 on a refusal, which carries `error` and `diagnostics`.
  - `generate.modified`, `check.run`, `check.ns`, `cli.run`, `cli.report`, `root.for_buf`.
- Produces:
  - `fill.hole_at(holes, rel_file, line, col) -> hole|nil` takes a 1-based line and byte col.
  - `fill.address(hole) -> string`
  - `fill.fill(bang)`
  - `fill.submit(buf)`
  - Command `:AlderFill[!]`. The scratch buffer is named `alder://fill/<address>`, and its first two lines are the header.

- [ ] **Step 1: Write the failing specs** `tests/fill_spec.lua`

```lua
local H = require('helpers')
local fill = require('alder.fill')

local function scratch()
  for _, b in ipairs(vim.api.nvim_list_bufs()) do
    if vim.api.nvim_buf_get_name(b):match('alder://fill/') then return b end
  end
end

local function app_with_hole()
  local app = H.new_app()
  H.write(app .. '/app/Calc.alder', 'module Calc\n\nanswer : Int\nanswer = ?value\n')
  vim.cmd.edit(app .. '/app/Calc.alder')
  vim.api.nvim_win_set_cursor(0, { 4, 10 }) -- on `?value`
  return app
end

test('fill: hole_at picks the hole under the cursor, else the only one in the file', function()
  local holes = {
    { module = 'A', decl = 'x', name = 'v', type = 'Int', file = 'app/A.alder', line = 4, col = 5 },
    { module = 'A', decl = 'y', name = 'w', type = 'Int', file = 'app/A.alder', line = 7, col = 5 },
    { module = 'B', decl = 'z', name = 'u', type = 'Int', file = 'app/B.alder', line = 2, col = 1 },
  }
  assert(fill.hole_at(holes, 'app/A.alder', 4, 6).name == 'v')
  assert(fill.hole_at(holes, 'app/A.alder', 4, 7) == nil, 'past `?v`')
  assert(fill.hole_at(holes, 'app/A.alder', 1, 1) == nil, 'two holes in the file and none under the cursor')
  assert(fill.hole_at(holes, 'app/B.alder', 9, 9).name == 'u', 'the only hole in the file')
  assert(fill.address({ module = 'A', decl = 'update', arm = 'Add', name = 'v' }) == 'A.update.Add')
end)

test('fill: a scratch headed with the hole and its type; <CR> fills it', function()
  local app = app_with_hole()
  fill.fill(false)
  assert(H.wait(20000, function() return scratch() ~= nil end), 'no scratch')
  local buf = scratch()
  local lines = vim.api.nvim_buf_get_lines(buf, 0, -1, false)
  assert(lines[1] == '-- ?value : Int' and lines[2] == '-- Calc.answer', vim.inspect(lines))
  vim.api.nvim_buf_set_lines(buf, 2, -1, false, { '41 + 1' })
  fill.submit(buf)
  assert(H.wait(20000, function() return (H.read(app .. '/app/Calc.alder') or ''):find('answer = 41 + 1', 1, true) end),
    H.read(app .. '/app/Calc.alder'))
  assert(H.wait(5000, function() return scratch() == nil end), 'the scratch stayed open after success')
end)

test('fill: a refusal is a diagnostic in the scratch and the expression stays', function()
  local app = app_with_hole()
  fill.fill(false)
  assert(H.wait(20000, function() return scratch() ~= nil end))
  local buf = scratch()
  vim.api.nvim_buf_set_lines(buf, 2, -1, false, { '"not an Int"' })
  fill.submit(buf)
  assert(H.wait(20000, function() return #vim.diagnostic.get(buf, { namespace = require('alder.check').ns }) > 0 end),
    'no diagnostic for the refusal')
  assert(vim.api.nvim_buf_get_lines(buf, 2, 3, false)[1] == '"not an Int"')
  assert((H.read(app .. '/app/Calc.alder') or ''):find('?value', 1, true), 'the file changed despite the refusal')
end)

test('fill: no hole under the cursor', function()
  local app = H.new_app()
  H.write(app .. '/app/Calc.alder', 'module Calc\n\na : Int\na = ?x\n\nb : Int\nb = ?y\n')
  vim.cmd.edit(app .. '/app/Calc.alder')
  vim.api.nvim_win_set_cursor(0, { 1, 0 })
  local messages, restore = H.capture_notify()
  fill.fill(false)
  H.wait(20000, function() return #messages > 0 end)
  restore()
  assert(messages[1] and messages[1].msg == 'alder: no hole under the cursor', vim.inspect(messages))
end)
```

- [ ] **Step 2: Run them to see them fail**

Run: `nvim --headless -u tests/minimal.lua -l tests/run.lua fill`
Expected: FAIL with `module 'alder.fill' not found`.

- [ ] **Step 3: Write `lua/alder/fill.lua`**

```lua
local cli = require('alder.cli')
local root = require('alder.root')

local M = {}

function M.address(h)
  return h.module .. '.' .. h.decl .. (h.arm and h.arm ~= '' and ('.' .. h.arm) or '')
end

-- hole_at returns the hole of rel_file whose `?name` contains line/col (1-based, bytes), or the
-- file's only hole when there is just one.
function M.hole_at(holes, rel_file, line, col)
  local in_file = {}
  for _, h in ipairs(holes) do
    if h.file == rel_file then table.insert(in_file, h) end
  end
  for _, h in ipairs(in_file) do
    if h.line == line and col >= h.col and col < h.col + 1 + #h.name then return h end
  end
  if #in_file == 1 then return in_file[1] end
  return nil
end

local function relative(app_root, path)
  local r, p = vim.uv.fs_realpath(app_root) or app_root, vim.uv.fs_realpath(path) or path
  if p:sub(1, #r + 1) == r .. '/' then return p:sub(#r + 2) end
  return nil
end

local scratches = {} -- buf → { app_root, hole, bang }

local function close(buf)
  scratches[buf] = nil
  if vim.api.nvim_buf_is_valid(buf) then vim.api.nvim_buf_delete(buf, { force = true }) end
end

-- submit pipes the expression below the header to alder fill. A refusal becomes diagnostics in
-- the scratch buffer (the expression stays for editing); success closes it, reloads the file
-- and re-checks the app.
function M.submit(buf)
  local s = scratches[buf]
  if not s then return end
  local ns = require('alder.check').ns
  local body = table.concat(vim.api.nvim_buf_get_lines(buf, 2, -1, false), '\n')
  local addr = M.address(s.hole)
  local args = s.bang and { 'fill', '--decl', s.hole.module .. '.' .. s.hole.decl, '-' }
    or { 'fill', addr, '-', '--hole', '?' .. s.hole.name }
  vim.list_extend(args, { '--json', '--dir', s.app_root })
  cli.run(args, { cwd = s.app_root, stdin = body }, function(res)
    if not vim.api.nvim_buf_is_valid(buf) then return end
    vim.diagnostic.reset(ns, buf)
    local json = res.json
    if not json or res.code == 2 then
      cli.report(args, res)
      return
    end
    if json.error then
      local ds = { { lnum = 2, col = 0, message = json.error, severity = vim.diagnostic.severity.ERROR, source = 'alder' } }
      for _, d in ipairs(json.diagnostics or {}) do
        table.insert(ds, { lnum = 2, col = 0, code = d.code, source = 'alder',
          severity = d.severity == 'warning' and vim.diagnostic.severity.WARN or vim.diagnostic.severity.ERROR,
          message = ('%s:%d:%d %s %s'):format(d.file, d.line, d.col, d.code, d.message) })
      end
      vim.diagnostic.set(ns, buf, ds)
      return
    end
    local app_root = s.app_root
    close(buf)
    vim.cmd('checktime')
    require('alder.check').run(app_root)
  end)
end

local function open_scratch(app_root, hole, bang)
  local addr = M.address(hole)
  vim.cmd('botright new')
  local buf = vim.api.nvim_get_current_buf()
  vim.bo[buf].buftype = 'acwrite'
  vim.bo[buf].bufhidden = 'wipe'
  vim.bo[buf].swapfile = false
  pcall(vim.api.nvim_buf_set_name, buf, 'alder://fill/' .. addr .. (bang and '' or ('/?' .. hole.name)))
  vim.api.nvim_buf_set_lines(buf, 0, -1, false, { ('-- ?%s : %s'):format(hole.name, hole.type), '-- ' .. addr, '' })
  vim.bo[buf].filetype = 'alder'
  vim.bo[buf].modified = false
  scratches[buf] = { app_root = app_root, hole = hole, bang = bang }
  vim.keymap.set('n', '<CR>', function() M.submit(buf) end, { buffer = buf, nowait = true })
  vim.keymap.set('n', 'q', function() close(buf) end, { buffer = buf, nowait = true })
  vim.api.nvim_create_autocmd('BufWriteCmd', { buffer = buf, callback = function()
    vim.bo[buf].modified = false
    M.submit(buf)
  end })
  vim.api.nvim_create_autocmd('BufWipeout', { buffer = buf, once = true, callback = function() scratches[buf] = nil end })
  vim.api.nvim_win_set_cursor(0, { 3, 0 })
end

-- fill is :AlderFill[!]: the hole under the cursor (or the file's only hole) in a scratch
-- split; with bang, the hole's whole declaration body (--decl).
function M.fill(bang)
  local file = vim.api.nvim_buf_get_name(0)
  local app_root = root.for_buf(0)
  if not app_root then
    vim.notify('alder: no `app/` directory above this file', vim.log.levels.ERROR)
    return
  end
  if #require('alder.generate').modified(app_root) > 0 then
    vim.notify('alder: save all buffers first (:wall)', vim.log.levels.ERROR)
    return
  end
  local rel = relative(app_root, file)
  local row, col = unpack(vim.api.nvim_win_get_cursor(0))
  local args = { 'fill', '--list', '--json', '--dir', app_root }
  cli.run(args, { cwd = app_root }, function(res)
    if not res.json or res.code ~= 0 then
      cli.report(args, res)
      return
    end
    local hole = rel and M.hole_at(res.json.holes or {}, rel, row, col + 1)
    if not hole then
      vim.notify('alder: no hole under the cursor', vim.log.levels.WARN)
      return
    end
    open_scratch(app_root, hole, bang)
  end)
end

return M
```

- [ ] **Step 4: Add the command to `plugin/alder.lua`** (append)

```lua
vim.api.nvim_create_user_command('AlderFill', function(o)
  require('alder.fill').fill(o.bang)
end, { bang = true, desc = 'fill the typed hole under the cursor (! replaces the whole declaration body)' })
```

- [ ] **Step 5: Run the specs**

Run: `nvim --headless -u tests/minimal.lua -l tests/run.lua`
Expected: all `ok`. If `alder fill --list` reports the hole's `col` at the name rather than at `?`, adjust `hole_at`'s range (`col - 1` start) and its unit test together, and note it in the code comment.

- [ ] **Step 6: Commit**

```bash
git add -A && git commit -m "alder.nvim: :AlderFill on the hole under the cursor"
```

---
## Task 13: `:checkhealth alder`, README, plugin CI

**Files:**
- Create: `lua/alder/health.lua`, `tests/health_spec.lua`, `README.md`, `.github/workflows/test.yml`

**Interfaces:**
- Consumes: `config.options`, `alder help --json`, `vim.treesitter.query.get`, nvim-treesitter main (`require('nvim-treesitter.config')` exists on main only).
- Produces: `require('alder.health').check()`, which `:checkhealth alder` discovers.

- [ ] **Step 1: Write the failing spec** `tests/health_spec.lua`

```lua
local H = require('helpers')

test('health: every check reports and none is an error on a working setup', function()
  H.ensure_parser()
  local reports = {}
  local orig = vim.health
  vim.health = {
    start = function() end,
    ok = function(m) table.insert(reports, { 'ok', m }) end,
    warn = function(m) table.insert(reports, { 'warn', m }) end,
    error = function(m) table.insert(reports, { 'error', m }) end,
    info = function() end,
  }
  local ok, err = pcall(require('alder.health').check)
  vim.health = orig
  assert(ok, err)
  assert(#reports >= 5, vim.inspect(reports))
  for _, r in ipairs(reports) do assert(r[1] ~= 'error', vim.inspect(reports)) end
end)
```

- [ ] **Step 2: Run it to see it fail**

Run: `nvim --headless -u tests/minimal.lua -l tests/run.lua health`
Expected: FAIL with `module 'alder.health' not found`.

- [ ] **Step 3: Write `lua/alder/health.lua`**

```lua
local M = {}

local QUERIES = { 'highlights', 'locals', 'indents', 'folds', 'injections', 'textobjects' }

local function version_at_least(v, min)
  local a = vim.version.parse(v or '')
  return a ~= nil and vim.version.ge(a, min)
end

function M.check()
  local h = vim.health
  h.start('alder')

  local cmd = require('alder.config').options.cmd
  if vim.fn.executable(cmd) == 1 then
    local v = vim.system({ cmd, 'version' }, { text = true }):wait()
    h.ok(vim.trim(v.stdout or cmd))
    local help = vim.system({ cmd, 'help', '--json' }, { text = true }):wait()
    if help.code == 0 then
      h.ok('`alder help --json` works (completion and :AlderNew)')
    else
      h.error('`alder help --json` failed: this alder predates editor support; build a newer one')
    end
  else
    h.error(('`%s` is not on PATH'):format(cmd))
  end

  if vim.fn.executable('tree-sitter') == 1 then
    local v = vim.system({ 'tree-sitter', '--version' }, { text = true }):wait()
    local version = (v.stdout or ''):match('(%d+%.%d+%.%d+)')
    if version_at_least(version, '0.26.1') then
      h.ok('tree-sitter ' .. version)
    else
      h.error('tree-sitter ' .. tostring(version) .. ': nvim-treesitter main needs 0.26.1 or newer (brew install tree-sitter-cli)')
    end
  else
    h.error('the tree-sitter CLI is not on PATH: brew install tree-sitter-cli')
  end

  if pcall(require, 'nvim-treesitter.config') then
    h.ok('nvim-treesitter (main)')
  else
    h.error('nvim-treesitter main is not installed (alder.nvim does not support the master branch)')
  end

  if pcall(vim.treesitter.language.add, 'alder') and vim.treesitter.language.add('alder') then
    h.ok('parser `alder` installed')
  else
    h.error('parser `alder` is not installed: run :TSInstall alder')
  end

  local missing = {}
  for _, name in ipairs(QUERIES) do
    local ok, q = pcall(vim.treesitter.query.get, 'alder', name)
    if not ok or not q then table.insert(missing, name .. (ok and '' or (' (' .. tostring(q) .. ')'))) end
  end
  if #missing == 0 then
    h.ok('queries installed: ' .. table.concat(QUERIES, ', '))
  else
    h.error('queries missing or broken: ' .. table.concat(missing, ', ') .. ' — run :TSUpdate alder')
  end
end

return M
```

- [ ] **Step 4: Run all specs**

Run: `nvim --headless -u tests/minimal.lua -l tests/run.lua`
Expected: all `ok`.

- [ ] **Step 5: Write `README.md`**

````markdown
# alder.nvim

Neovim support for [Alder](https://github.com/tapegram/alder): highlighting, indentation,
folds and textobjects from Alder's tree-sitter grammar; `alder check` diagnostics on save, with
the compiler's fixes; and the code generators (`alder new`, `edit`, `remove`, `regen`, `fill`)
with a preview of the diff before anything is written.

The plugin never rewords the compiler: what you see is what `alder` printed.

## Requirements

- Neovim 0.11 or newer
- [nvim-treesitter](https://github.com/nvim-treesitter/nvim-treesitter) on its `main` branch
- the `tree-sitter` CLI, 0.26.1 or newer: `brew install tree-sitter-cli`
- `alder` on your `PATH`, from the first release with editor support (it has
  `alder help --json`); until that is tagged, a build from `main`

## Install

lazy.nvim:

```lua
{
  'tapegram/alder.nvim',
  lazy = false, -- it is small, and it must register the parser before :TSInstall
  dependencies = { 'nvim-treesitter/nvim-treesitter' },
  opts = {},
}
```

AstroNvim, `lua/plugins/alder.lua`:

```lua
---@type LazySpec
return {
  { 'tapegram/alder.nvim', lazy = false, opts = {} },
  {
    'AstroNvim/astrocore',
    opts = function(_, opts)
      opts.treesitter = opts.treesitter or {}
      opts.treesitter.ensure_installed =
        require('astrocore').list_insert_unique(opts.treesitter.ensure_installed, { 'alder' })
    end,
  },
}
```

Then `:TSInstall alder`, and `:checkhealth alder`.

## Options

```lua
require('alder').setup({
  cmd = 'alder',        -- the alder binary
  check_on_save = true, -- run alder check after writing an .alder file
  preview = true,       -- generators show their diff first; <CR> applies, q discards
  revision = nil,       -- install the grammar from this tag of tapegram/alder, e.g. 'v0.2.0'
  path = nil,           -- install the grammar from a local alder checkout (see below)
})
```

The grammar lives in the alder repository (`tree-sitter-alder/`) and changes with the language.
By default `:TSInstall alder` takes it from `main`; set `revision` to the tag of the `alder` you
run so the grammar matches your compiler.

**Grammar development:** set `path = '~/projects/alder'` (the checkout's root, not its
`tree-sitter-alder/` directory), edit the grammar or its queries there, then `:TSUpdate alder`.

## Commands

| Command | What it does |
|---|---|
| `:AlderCheck` | `alder check`: diagnostics in open buffers, other files in the quickfix list |
| `:AlderFix` | apply the compiler's fix for the diagnostic under the cursor |
| `:AlderExplain [code]` | `alder explain` for the code (default: the diagnostic under the cursor) |
| `:Alder <args…>` | any `alder` command, with completion; generators preview their diff |
| `:AlderNew` | pick a generator, type its arguments, add flags, preview, apply |
| `:AlderFill[!]` | fill the typed hole under the cursor; `!` replaces the whole declaration body |

Generators refuse to run while an `.alder` buffer of the app has unsaved changes, because the
CLI edits files on disk: `:wall` first.

## Development

```sh
nvim --headless -u tests/minimal.lua -l tests/run.lua          # all tests
nvim --headless -u tests/minimal.lua -l tests/run.lua fill     # tests whose name contains "fill"
```

`NVIM_TS` points at an nvim-treesitter checkout (default: lazy.nvim's) and `ALDER_REPO` at an
alder checkout (default `~/projects/alder`); `alder` and `tree-sitter` must be on `PATH`.

## License

Apache-2.0
````

- [ ] **Step 6: Write `.github/workflows/test.yml`**

```yaml
name: test
on:
  push:
  pull_request:
permissions:
  contents: read
jobs:
  test:
    runs-on: ubuntu-latest
    timeout-minutes: 15
    steps:
      - uses: actions/checkout@v4
      - uses: actions/checkout@v4
        with:
          repository: tapegram/alder
          path: alder
      - uses: actions/checkout@v4
        with:
          repository: nvim-treesitter/nvim-treesitter
          ref: main
          path: nvim-treesitter
      - uses: actions/setup-go@v5
        with:
          go-version-file: alder/go.mod
      - name: alder
        run: |
          go build -o "$RUNNER_TEMP/bin/alder" ./cmd/alder
          echo "$RUNNER_TEMP/bin" >> "$GITHUB_PATH"
        working-directory: alder
      - name: tree-sitter CLI
        run: alder/.github/install-tree-sitter.sh
      - uses: rhysd/action-setup-vim@v1
        with:
          neovim: true
          version: stable
      - name: tests
        run: nvim --headless -u tests/minimal.lua -l tests/run.lua
        env:
          NVIM_TS: ${{ github.workspace }}/nvim-treesitter
          ALDER_REPO: ${{ github.workspace }}/alder
```

CI needs the grammar and `alder help --json` on alder's `main`, so this workflow can only pass after the alder branch is merged (Task 14).

- [ ] **Step 7: Commit**

```bash
git add -A && git commit -m "alder.nvim: :checkhealth alder, README, CI"
```

---

## Task 14: Book, grammar README, verification, merge and release

**Files (alder repo):**
- Create: `tree-sitter-alder/README.md`
- Modify: `book/01-why.md` (after "## Installing"), `book/11-tooling.md` (the `## Commands` list and a link)

**Interfaces:**
- Consumes: everything above.
- Produces: the merged alder branch, then a public `tapegram/alder.nvim` at `v0.1.0`, created only after the user confirms.

- [ ] **Step 1: Write `tree-sitter-alder/README.md`**

````markdown
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
````

- [ ] **Step 2: Add "Editor support" to `book/01-why.md`**

At the end of the "## Installing" section, before the next `##` heading, add:

````markdown
### Editor support

Neovim gets highlighting, indentation, folds and `alder check` diagnostics on save from
[alder.nvim](https://github.com/tapegram/alder.nvim), and the generators run inside the
editor with a preview of the diff. It needs Neovim 0.11, nvim-treesitter's `main` branch and
the `tree-sitter` CLI:

```
brew install tree-sitter-cli
```

With lazy.nvim:

```lua
{ 'tapegram/alder.nvim', lazy = false, dependencies = { 'nvim-treesitter/nvim-treesitter' }, opts = {} }
```

then `:TSInstall alder` inside Neovim. The grammar the editor uses lives in this repository
(`tree-sitter-alder/`) and is tested against the compiler's own parser, so the two cannot
drift apart.
````

Use a fence the book verifier accepts for non-Alder blocks: look at how chapter 1 marks its other shell and Lua blocks and copy that info string. A bare fence or `sh` is usual. The book tests fail on an `alder` fence that does not check.

- [ ] **Step 3: Update `book/11-tooling.md`**

In `## Commands`, add `alder help --json` to the list with one sentence: "prints the command tree (commands, subcommands, arguments and flags, each with a one-line summary) as JSON; editors build completion from it." Next to the existing description of the generators, add a pointer to editor use: "In Neovim, alder.nvim runs every generator with a preview of its diff (chapter 1, Editor support)."

- [ ] **Step 4: Verify the alder branch**

Run: `cd ~/projects/alder && gofmt -l $(git ls-files '*.go') && go vet ./... && go test ./... -count=1 2>&1 | tail -30`
Expected: no gofmt output and every package `ok`, including `alder/tree-sitter-alder` and `alder/book`. Paste the failing output into the task report if anything fails, and fix it before continuing.

Run: `scripts/ts.sh test && scripts/ts.sh generate && git diff --exit-code tree-sitter-alder/src`
Expected: tests pass and there is no diff.

- [ ] **Step 5: Commit the docs**

```bash
git add tree-sitter-alder/README.md book/01-why.md book/11-tooling.md
git commit -m "docs: editor support — tree-sitter grammar, alder.nvim, alder help --json"
```

- [ ] **Step 6: Visual check with the user**

Ask the user to open `examples/todos`, `examples/blog`, `examples/workspace` and a chapter-3 example in Neovim with the plugin (`path = '~/projects/alder'`, `:TSUpdate alder`). They should check highlighting, `=` re-indent of a view, folds, and one save showing a diagnostic, and send one screenshot for the plugin README. Record what they report. Fix queries in the alder repo for anything they flag, with a corpus or plugin test where the fault can be pinned.

- [ ] **Step 7: Whole-branch review and merge (alder)**

Run the final whole-branch review the execution method calls for. Per the user's model policy that is Fable for a final review of a branch like this. Then merge `m6a-grammar-nvim` into `main` (fast-forward) and push, following superpowers:finishing-a-development-branch.

- [ ] **Step 8: Publish the plugin (after the user confirms)**

Creating a public repository is outward-facing: ask the user first. On a yes:

```bash
cd ~/projects/alder.nvim && git switch main && git merge --ff-only v0.1
gh repo create tapegram/alder.nvim --public --description "Neovim support for the Alder language" --source . --push
git push origin v0.1
```

Wait for the `test` workflow on `main` (`gh run watch`). It passes only once alder's `main` has the grammar and `alder help --json` (Step 7). Then tag:

```bash
git tag -a v0.1.0 -m "alder.nvim v0.1.0" && git push origin v0.1.0
```

- [ ] **Step 9: Handover**

Update the Alder project memory with the state. M6a is merged, the plugin is at v0.1.0, and these items are open:

- the spec's ledger;
- any conflicts added to the grammar;
- whether column-1 HTML text was relaxed;
- the visual check's findings.

Name the next step: M6b `alder lsp` spec, on Fable. Leave both trees clean and pushed, and give the user the one-line first prompt for the next session.

---

## Self-review (done while writing the plan)

**Spec coverage.**

| Spec item | Where |
|---|---|
| §1 layout, node shape, scanner tokens, acceptance policy | Tasks 1–4 |
| §2 corpus tests | Tasks 1–4 |
| §2 drift test: zero ERROR/MISSING, declaration kinds and names, fails without the CLI | Task 5 |
| §2 regeneration check | Task 5 `TestGenerated` and the Task 8 job |
| §2 CI | Task 8 |
| §3 all six queries plus the CLI highlights | Task 6 |
| §3 visual check | Task 14 Step 6 |
| §4 layout, setup options, filetype and parser registration | Task 9 |
| §4 diagnostics on save, quickfix, `:AlderFix`, `:AlderExplain` | Task 10 |
| §4 `alder help --json` | Task 7 |
| §4 `:Alder` and completion, preview-then-apply, `:AlderNew`, after-write, safety rule | Task 11 |
| §4 `:AlderFill` | Task 12 |
| §4 health | Task 13 |
| §5 plugin tests | Tasks 9–13 |
| §5 error handling | `cli.report` and the messages in Tasks 10–12 |
| §5 release: chapters 1 and 11, plugin v0.1.0 | Task 14 |

The "release notes list editor support" item is carried by Task 14's commit message, because GoReleaser writes release notes from commits.

**Placeholders.** None remain. Some steps say "fix until it passes" (Task 5 Step 4, and Step 8 in Tasks 1–4). Each gives the loop, the authority (the Go parser) and the rule against `--update`. The grammar cannot be known to be complete before the corpus runs against it.

**Type consistency.** The names are used the same way across tasks:

- Node and field names: Task 1 Interfaces, corpus trees, `drift.scm`, queries, plugin capture assertions.
- The `commandTree` JSON shape: Task 7 Produces, and `commands.lua` reading `subcommands`, `flags[].value` and `args`.
- Generator JSON fields: `files[].action` values `created|edited|removed`.
- The hole fields: `module, decl, arm, name, type, file, line, col`.

**Review Focus.** Each item has its test:

1. Task 5 `TestRecovery`
2. Task 9 `root: a module directory named App…`
3. Task 10 `superseded` and `saving clears`
4. Task 10 `symlink`
5. Task 10 `multi-byte`
