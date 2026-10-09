# 11. Tooling

`alder check` exists as of M1, `alder build --emit` as of M3a, and
`alder dump` as of M3b. Since M3b, `alder build --emit` generates and vets
Go for every module of an app, effects and database included: `load` and `update`
run against SQLite (chapter 7, "How effects run"). Serving the app over
HTTP arrives in M3c and `alder run` in M3d; `test`, `fmt`, holes and
`query` are planned.

## Project layout

```
myapp/
  alder.toml          -- name, database adapter, Go module path
  app/
    App.alder         -- session, error pages, login page
    Todo.alder        -- table Todo, TodoId, helpers
    Todos/
      Index.alder     -- page "/"
      Show.alder      -- page "/todos/{id}"
    Layout.alder      -- shared components
  adapters/           -- Go implementations of capabilities (optional)
  build/              -- generated Go, never edited
```

## Commands

```
alder check          type-check everything; milliseconds; no output on success
alder build          check, generate Go, go build → ./build/myapp
alder run            build and run with the dev database, reloading on change
alder test           run every `test`
alder fmt            rewrite every file in canonical format
alder dump <path>    print a file's tokens, AST, or an app's IR
alder explain <id>   explain a diagnostic code
```

`alder build --emit` writes the generated Go to `build/<app>/` and vets it. Chapter 13 walks through what it generates and why: see [How the compiler works](13-compiler.md).
It replaces only the files it generated; anything else in `build/<app>/` is
kept, and vetted with the rest. `build/<app>/` must contain only generated
files: if the vet fails and the directory holds Go files alder did not
write (a test file left there, say), the error names them; move them out
and build again.

`alder dump` shows what the compiler sees:

```
alder dump <file.alder | dir>            the AST, as syntax.Dump prints it (also --ast)
alder dump --tokens <file.alder | dir>   the tokens, one per line: L:C KIND "text"
alder dump --ir <dir>                    the app's IR, after a clean check
```

Given a directory, `--ast` and `--tokens` cover every `.alder` file under
its `app/`, in path order; `--tokens` heads each file's listing with
`-- <path>`. The tokens are the ones the parser consumed, so HTML text
appears as `Text` tokens. `--ir` needs an app directory: it checks the app
like `alder check` and prints the IR only when there are no errors and no
holes. The dump goes to stdout and diagnostics to stderr. `alder dump`
exits 0 when clean; 1 when there are errors, which for the AST and the
tokens means parse errors (they are still printed) and with `--ir` means
any check error or a hole (the IR is not printed); and 2 on a usage or I/O
failure, including `--ir` given a file. Chapter 13 reads these dumps.

The generated `go.mod` points at the alder repository, which holds the
runtime the code imports (`replace alder => "<path>"`). The `alder` binary
finds the repository it was built from; set `ALDER_ROOT` to the
repository's path when the binary was built elsewhere or moved, or to use
another checkout's runtime.

## Diagnostics

Every diagnostic has a stable code, a location, a one-line message, and,
when possible, a fix. With `--json`, diagnostics are emitted as JSON lines:

```json
{"code":"E0301","severity":"error","file":"app/Todos/Index.alder","line":28,"col":52,"endLine":28,"endCol":58,"message":"message `Toggle` needs its parameters: `Toggle { id: ?id }`","fix":{"line":28,"col":52,"endLine":28,"endCol":58,"replace":"Toggle { id: ?id }"}}
```

A fix replaces the half-open span from `line:col` to `endLine:endCol` with `replace`; when the two positions are equal it is an insertion. A diagnostic carries at most one fix; two diagnostics may propose the same insertion (for example two E0201s for one missing import), and a tool applying fixes must dedupe identical zero-width fixes.

`?name` in a fix is a hole for the agent to fill.

`alder check` exits 0 when there are no errors, 1 when there are errors, and
2 on a usage or I/O failure. A file that does not parse stops the compiler. After that, every module's declarations are checked; the bodies of a module whose names could not be resolved are skipped, so a missing import never hides type errors in another module. `alder explain <code>` prints a paragraph for any code below.

### Diagnostic codes

| Code | Meaning | Fix offered |
|---|---|---|
| E0001 | parse error (also: an identifier with `__` in it, a module name segment ending in `_`) | — |
| E0002 | unknown name | — |
| E0003 | type mismatch (also: `==` or a `Dict` key on a function or an effect value, a column method on a column held in a variable, a `Db` function over a type parameter) | conversion such as `Text.fromInt (…)` where one exists |
| E0004 | non-exhaustive `match` | arm skeleton |
| E0005 | module name does not match its file path | — |
| E0006 | import cycle | — |
| E0007 | top-level value without a type signature | signature line |
| E0008 | wrong number of parameters or arguments, or of type arguments (`Box Int Text` for `type Box a`) | — |
| E0009 | duplicate definition, or a constructor declared in two imported modules | — |
| E0010 | required declaration missing (page `load`/`view`/`update`, `App` `error`/`notFound`/`login`/`session`) | — |
| E0011 | invalid declaration (unstorable table field, unusable page or form field, bad `@id`, brand constructor used outside its module, a record that contains itself by value) | — |
| E0012 | a type, table, form or component in a page module or `App` named like a generated type: `Params`, `Msg`, `Session`, or `<Msg>Params` for a declared `msg` | — |
| E0014 | two module names that differ only in case (`Ab` and `AB`): their files would be one on a case-insensitive file system | — |
| E0101 | `Db.get T x` where `x` is not `Id T` | `Db.from T \|> Db.where (T.<col>.eq x) \|> Db.first` when a column of `x`'s type exists |
| E0102 | field access on a `Maybe` | `match` skeleton |
| E0201 | name from a module that isn't imported | `import M` |
| E0202 | unqualified type from another module | `M.T` |
| E0203 | qualified constructor | unqualified name |
| E0301 | message sent without its parameters | `M { field: ?field }` |
| E0302 | `update` missing an arm | arm skeleton |
| E0303 | `href` with a `Text` | — |
| E0304 | form field not in the form type, or wrong input component | list of fields (in message) |
| E0310 | unreachable `match` arm | — |
| E0401 | `Task` inside `load` or a `Query` block | — |
| E0402 | `let x <-` outside an effect block | — |
| E0501 | `@id`, `@created`, `@updated` supplied on insert | — |
| E0502 | `@ref` with the wrong id type | — |
| E0601 | `Html Msg` in a non-page module | `Html a` |
| E0602 | `case ... of`, `let ... in`, tuple, alias | the Alder form (in message) |
| W0001 | unused import | — |
| W0002 | typed hole (`?name`) with its expected type | — |

## Holes

`?name` is a typed hole. The program compiles with holes, and `alder check`
reports each one as warning W0002 with its expected type:

```
app/Todos/Index.alder:40:18: warning[W0002]: hole ?count has type Int
```

Listing the values in scope that could fill a hole is planned.

## Queries

`alder query` answers questions about the program as JSON:

```
alder query type app/Todos/Index.alder:40:18     type at a position
alder query cases Filter                         constructors of a union
alder query pages                                every page, its route and params
alder query msgs Todos.Index                     every msg of a page
alder query uses TodoId                          every use of a name
alder query capabilities                         every capability and whether it is implemented
```

## Formatting

There is one format. `alder fmt` produces it; `alder check` warns on files
that are not in it. The format is deterministic: the same program always
formats to the same text, so diffs are minimal.
