# 11. Tooling

`alder check` exists as of M1; `build`, `run`, `test`, `fmt`, holes and `query` are planned.
`alder build --emit` generates and vets Go for the pure parts of an app (M3a); effects, pages and `alder run` arrive in M3b–M3d.

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
  db/
    schema.snapshot
    migrations/
  build/              -- generated Go, never edited
```

## Commands

```
alder check          type-check everything; milliseconds; no output on success
alder build          check, generate Go, go build → ./build/myapp
alder run            build and run with the dev database, reloading on change
alder test           run every `test`
alder fmt            rewrite every file in canonical format
alder explain <id>   explain a diagnostic code
```

`alder build --emit` writes the generated Go to `build/<app>/` and vets it. Chapter 13 walks through what it generates and why: see [How the compiler works](13-compiler.md).

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
| E0003 | type mismatch | conversion such as `Text.fromInt (…)` where one exists |
| E0004 | non-exhaustive `match` | arm skeleton |
| E0005 | module name does not match its file path | — |
| E0006 | import cycle | — |
| E0007 | top-level value without a type signature | signature line |
| E0008 | wrong number of parameters or arguments | — |
| E0009 | duplicate definition, or a constructor declared in two imported modules | — |
| E0010 | required declaration missing (page `load`/`view`/`update`, `App` `error`/`notFound`/`login`/`session`) | — |
| E0011 | invalid declaration (unstorable table field, unusable page or form field, bad `@id`, brand constructor used outside its module) | — |
| E0012 | a type, table, form or component in a page module or `App` named like a generated type: `Params`, `Msg`, `Session`, or `<Msg>Params` for a declared `msg` | — |
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
