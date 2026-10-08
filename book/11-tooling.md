# 11. Tooling

This chapter describes the compiler as designed. None of it exists yet.

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

## Diagnostics

Every diagnostic has a stable code, a location, a one-line message, and,
when possible, a fix. With `--json`, diagnostics are emitted as JSON lines:

```json
{"code":"E0412","severity":"error","file":"app/Todos/Index.alder","line":31,"col":12,
 "message":"page Todos.Show needs field `id: TodoId`, got `{ }`",
 "fix":{"line":31,"col":12,"replace":"Todos.Show { id: ?id }"}}
```

`?name` in a fix is a hole for the agent to fill.

## Holes

`?name` is a typed hole. The program compiles with holes, and `alder check`
reports each one with its expected type and the values in scope that could
fill it:

```
app/Todos/Index.alder:40:18: hole ?count has type Int
  in scope: List.length model.todos : Int
            model.page : Int
```

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
