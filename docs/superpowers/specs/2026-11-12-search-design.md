# Search by name or by type

Design for #166. Decided with the user 2026-11-12. Parts marked **(my call)** were chosen while
writing this and were not discussed; everything else under "Decided" is the user's.

## Intent

While building an app the user keeps needing two things and has no tool for either:

1. **Find a standard library function** when they know roughly what it does, or what type it
   has, but not its name or module.
2. **Fill a typed hole**: find which function or value fits a hole of a given type.

Today the only ways are reading `stdlib/*.alder` or the book. Completion at a hole does not help:
it offers only locals and the current module's declarations, never the stdlib or imports, and only
when the whole type matches.

The model is Hoogle and Unison's `find`: one query box that takes a name or a type, searching the
code you can use right where you are. When Alder later has dependencies or published libraries,
the same search covers them (#229).

Success: the user stops leaving the editor, or reading stdlib source, to find a function.

## Goals

- One search engine inside `alder`, used by the CLI and by the language server.
- Search by name (fuzzy), by module (`Text.`), and by type, over the standard library and the
  app's own declarations.
- At a typed hole, one key searches for the hole's type without typing it, and picking a result
  fills the hole.
- Each result shows its name with its module, its signature and its description.
- Pickers in alder.nvim and alder-vscode.
- `alder search` works outside an app (stdlib only) and has `--json` for agents and scripts.

## Non-goals

Each has its own issue:

- A search screen in alder-tui (#226). The user is most interested in this one next.
- Generating the book's stdlib reference from the index (#227).
- Examples and book-section links on results (#228); they wait on #119.
- Searching dependencies (#229); there are none yet.
- Olifant's "related functions" (#230).

Also out:

- Writing the missing stdlib doc comments. That is #119 (roadmap item 8). Search shows a blank
  description where there is none.
- `textDocument/documentSymbol`.
- Any change to what source files accept. `_` as a type wildcard exists only in search queries.
- Changing hole completion. It stays as it is; search is the wider tool.
- Editor search outside an app through the language server (the server does nothing without a
  root). The plugins' CLI fallback covers that case; see "Editors".

## Anti-goals (what would make this a failure)

- **Slow or noisy loose matching.** The user: fuzzy type matching is welcome only "if it's fast
  and good"; otherwise no result is fine. An empty list is a correct answer. A long list of weak
  matches is not.
- Two engines, or plugins that build query logic or insert text themselves, so the CLI, nvim and
  VS Code disagree.
- Search that fails when the app has typed holes. Holes are the main use.
- A design that needs rework to add a third source of declarations (dependencies).

## Constraints

- No new Go dependencies (the import rule). The fuzzy scorer is hand-written.
- Stdlib declarations have no file to go to (Ruling R12 in `internal/lsp/definition.go`).
- The stdlib is always in scope and never imported; app modules need an `import`.
- Stdlib signatures have no parameter names.
- Doc comments are the `--` lines above a declaration, read with `syntax.DocOf`
  (`internal/syntax/walk.go:94`). Search must read them the same way hover does
  (`internal/lsp/hover.go:230`), so #119's comments appear in search with no further work.
- A new CLI command touches three places: the switch in `cmd/alder/main.go`, `commandUsages` in
  `cmd/alder/help.go`, and a `cmdSpec` in `cmd/alder/helpjson.go`.
- The book teaches everything: the command and the editor feature are documented in the book,
  and book snippets are tested.
- Work lands on main in small PRs.

## Decided

### Shape

One engine, a new package (`internal/search` **(my call)** on the name), with two front doors:

- **`alder search`**, for the terminal, agents and scripts.
- **The language server**, which calls the engine in-process. The editor plugins talk to the
  server and do not run the CLI while a server is attached. The server already holds the compiled
  app, including unsaved buffers, and knows the type of the hole under the cursor.

`alder routes` is the model: `buildkit.Routes` is shared by `cmd/alder/routes.go` and
`internal/lsp/routes.go`, and both plugins have a picker on it.

### The index

One entry per declaration that code in another module could refer to:

- functions and values (stdlib signatures; the app's top-level declarations),
- type declarations,
- union constructors (`Just : a -> Maybe a`), so `: Maybe Int` can find `Just` **(my call)**.

Operators and `Test` steps that the checker special-cases are not declarations and are not
indexed.

Each entry has:

| Field | Notes |
|---|---|
| `name`, `module` | |
| `kind` | `function`, `value`, `type` or `constructor` |
| `signature` | printed as hover prints it |
| `doc` | the doc comment, empty when there is none |
| `source` | `stdlib` or `app`. A plain string, so a package name can be a third value later. Nothing in matching or ranking may assume there are only two. |
| `effect` | `query` or `task` when the result type is `Query _` or `Task _`, derived from the signature **(my call)** |
| place | file, line and column; app entries only |

The app's entries come from a type-checked build. A build with typed holes is fine: search needs
types only, not lowering. (`alder routes` refuses holes; search must not.)

The stdlib-only index, used outside an app, needs checker types for the stdlib without an app
build. How to get them is the builder's choice.

### The query box

One text box, three kinds of query:

| Query | Meaning |
|---|---|
| `split`, `Text.sp`, `Text.` | **Name search.** Plain text. Fuzzy, and it matches the module too. `Text.` lists the whole module in declaration order. |
| `Text -> Text -> List Text` | **Type search.** Anything containing `->`. |
| `: List Text` | **"Something that gives me this type."** A leading `:`. Finds values of that type and functions that return it. This is what the hole key fills in. |

`_` is a wildcard in a type: `Text -> _ -> List Text`.

Type names in a query resolve like this **(my call)**:

- a Prelude type (`Int`, `Text`, `List`, `Maybe`, …) always means the Prelude type;
- a qualified name (`Todo.Row`, `Text.Builder`) means that type;
- any other unqualified name works when exactly one type in the stdlib and the app has it. An app
  type named `Builder` beside `Text.Builder` makes `Builder` ambiguous;
- an ambiguous or unknown name gives no results and a note that names the qualified choices.

A query with no `->` and no `:` that finds nothing, and that would parse as a type (`List Text`),
gets a note: "to search by type, start with `:`" **(my call)**.

### Type matching

**The rule: a result matches when it could be used where the query's type is expected.** This is
the same test hole completion uses (`u.Check(u.Instantiate(t), want)` on a clone of the unifier,
`internal/lsp/complete.go:774`).

- The candidate's type variables are instantiated to fresh unification variables, so a more
  general candidate matches: `List Int -> Int` finds `List.head`-like `List a -> a` functions.
- The query's lowercase type variables are rigid. They match only a candidate's type variable,
  consistently. So `List b -> Maybe b` finds `List.head : List a -> Maybe a`, and `a -> b` does
  not match every one-argument function **(my call)**.
- Each `_` is its own fresh variable and matches anything. It is how to say "anything here".
- **Arguments in any order** match, ranked below same-order matches: `List a -> Int -> List a`
  finds `List.take : Int -> List a -> List a`. People search because they do not remember the
  order **(my call; see the budget below)**.
- Reordering applies to plain type queries only. A `:` query never reorders, and the arguments
  it skips are always the leading ones, because that is what partial application and the hole
  fill can express **(my call)**.
- For a `:` query, a candidate may need **extra leading arguments**. Functions are curried, so
  `P1 -> … -> Pn -> R` matches `: T` when, for some k, `P(k+1) -> … -> Pn -> R` matches `T`. The
  first k parameters are what the caller still has to supply. For `: List Text`,
  `Text.split : Text -> Text -> List Text` matches with k = 2. For a function-typed hole
  `: Int -> Text`, `Text.fromInt` matches with k = 0.
- A plain type query (no `:`) matches candidates with the same number of arguments only. "One
  argument more or fewer" from the issue is left out **(my call)**: the `:` form covers the
  useful half.

**Ranking for type search (my call).** The sort keys, most significant first:

1. For `:` queries, a candidate whose result is a bare type variable (`Maybe.withDefault : a ->
   Maybe a -> a`, `List.foldl`) sorts after every other result, whatever its other keys. It
   matches every type, so it is kept (it can be the right fill) but always at the very end.
2. Fewer candidate type variables bound to concrete types: identical up to renaming first. So
   results that mention the query's own types come before generic ones.
3. Fewer extra leading arguments (`:` queries only).
4. Same argument order before reordered (plain type queries only).
5. Ties: `app` before `stdlib`, then `Module.name` alphabetically.

**Ranking for name search:** exact name, then prefix, then substring at a word boundary, then
fuzzy subsequence; ties by shorter name, then as above. A `Module.` prefix narrows by module.

**Budget.** A query answers fast enough to update as the user types: under 50 ms for the stdlib
plus an app of 2,000 declarations **(my call on the number)**. A Go benchmark in the repo records
it. The index is built once per build, not per query. If argument reordering cannot meet the
budget, or makes the golden queries below noisy, drop it and file an issue; do not ship it slow.

Results are capped at 50 by default **(my call)**.

### `alder search`

```
alder search <query> [--json] [--limit N]
```

- The query is the non-flag arguments joined with spaces, so `alder search 'Text -> Int'` and
  `alder search : List Text` both work.
- Inside an app (found the way other commands find it) it searches the stdlib and the app.
  Outside one it searches the stdlib.
- If the app does not type-check, results are stdlib-only and a note on stderr says the app's
  declarations were left out and to run `alder check` **(my call)**.
- Human output: an aligned table of `Module.name`, signature, a source/effect tag, and the first
  line of the doc.
- `--json`: an object **(my call; `alder routes` emits a bare array, but search needs to say how
  it read the query)**:

  ```json
  {
    "query": ": List Text",
    "mode": "produces",
    "note": "",
    "results": [
      { "name": "split", "module": "Text", "kind": "function",
        "signature": "Text -> Text -> List Text", "doc": "", "source": "stdlib",
        "match": "exact", "args": 2 }
    ]
  }
  ```

  `mode` is `name`, `type` or `produces`. `match` is `exact`, `general` or `reordered` (type
  search only). `args` is the number of leading arguments still to supply (`:` queries only).
  App entries add `file`, `line`, `col`. `effect` appears when set.
- Exit status: 0 with or without results; 2 for usage errors and for a type query that does not
  parse **(my call)**.

### Language server

- **`alder/search`**, a fast request. Params: `query`, and optionally `uri` and `position`.
  - With a position on a typed hole and no query, the server uses `: <the hole's type>` and
    returns that text as `query`, so the picker can show it and the user can edit it.
  - The printed hole type must parse back to the same search. Type variables the checker has not
    resolved print as `_`.
  - The response is the CLI's JSON object, with a URI and an LSP range in place of file, line and
    column (as `alder/routes` does).
  - It reads the last good build, as `alder/routes` does, so it is empty before the first good
    build and may be slightly stale while the program has errors.
- **Edits are computed by the server, at pick time (my call).** A second request takes the
  picked result and the document position and returns a workspace edit, built against the
  current document. Plugins never build insert text. The edit:
  - at a hole: replaces the hole with the qualified name, followed by one new hole per extra
    leading argument, in parentheses when the hole is in argument position
    (`f ?x` becomes `f (Text.split ?a ?b)`). New hole names must not clash; how they are named is
    the builder's choice;
  - elsewhere: inserts the qualified name at the cursor;
  - adds an `import` line when the result is in an app module the file does not import, with the
    qualifier completion would use (`importText`, `internal/lsp/complete.go:190`). Stdlib names
    need no import. A declaration of the same module is inserted unqualified.
  - Prelude declarations (`not`, `Just`, `Nothing`, `Ok`, `Err`, `True`, `False`) are inserted
    unqualified, as source code writes them. Their `module` is `Prelude`, and the name query
    `Prelude.` lists them **(my call)**.
- **`workspace/symbol`** for name search, so Telescope, fzf-lua and VS Code's Go to Symbol work
  with no plugin. It returns the app's declarations only, because a symbol needs a location and
  stdlib declarations have none.
- **A code action on a hole**, "Search for `<type>`", beside "Fill with AI". It runs a
  client-side command that opens the picker. It is offered only to clients that say they handle
  that command, so other editors do not get a dead action. How a client says so is the builder's
  choice.

### Editors

The same behaviour in alder.nvim and alder-vscode:

- **One search command.** With the cursor on a hole, the picker opens with `: <hole type>`
  already filled in and results showing. Anywhere else it opens empty.
- **Results update as the user types**, each change sending `alder/search` (debounced).
- **Looking something up needs no pick.** The highlighted result's signature and description
  are visible beside or under the list. Esc closes the picker and changes nothing.
- **Enter inserts** at the cursor, or fills the hole when the search started from one.
- **A second action jumps to the definition** instead. It is offered for app results only.
- In nvim the picker follows the existing ones (`lua/alder/picker.lua`, `lua/alder/routes.lua`,
  nui.nvim as gated in `lua/alder/ui.lua`) and the key choices just settled in alder.nvim#13.
  Which key is the second action is the builder's choice; it goes in the help.
- In VS Code it is a `createQuickPick` with the signature as the description, the doc as the
  detail, and an item button for the second action.
- **With no server attached** the picker falls back to `alder search --json`, as the routes
  pickers do **(my call)**. In that mode it finds stdlib results (and the app's when the CLI can
  build it), and offers insert of the qualified name only: no hole fill and no import edit.
- **Against an older `alder`** that has no `alder/search`, the plugin says to update alder.

## Golden queries

These are checked against the real stdlib and are the test of "good". The builder adds more.

| Query | Must find | Must not find |
|---|---|---|
| `split` | `Text.split` first | |
| `Text.` | every `Text` declaration, in file order | anything outside `Text` |
| `Text -> Text -> List Text` | `Text.split`, as `exact` | |
| `List b -> Maybe b` | `List.head`, `List.last` | `List.tail` |
| `List Int -> Int` | `List.sum` before `List.length` (`general`) | |
| `List a -> Int -> List a` | `List.take`, `List.drop`, as `reordered` | |
| `a -> b` | nothing in `List` or `Text` | `Text.length` |
| `Text -> _` | one-argument functions that take a `Text`, in any module: `Text.length`, `Text.toInt`, `Text.toBytes` | `Text.fromInt`, `Text.split` |
| `: Maybe Int` | in this order: `Text.toInt`, `List.maximum`, `List.minimum` (exact, `args` 1); then `Nothing`, `Just`, `List.head` (general); `Maybe.withDefault` last | |
| `: List Text` | in this order: `Text.lines` and `Text.words` (`args` 1), `Text.split` (`args` 2), then generic `List a` functions such as `List.reverse`; `List.foldl` last | |

## Rollout

Four PRs, in order, each merged to main **(my call)**:

1. **Engine and CLI.** The index, the query parser, matching and ranking, the benchmark,
   `alder search`, help text, the book section and the CHANGELOG.
2. **Language server.** `alder/search`, the pick edit, `workspace/symbol`, the hole code action.
3. **alder.nvim.** The picker, the command and keymap, tests, docs.
4. **alder-vscode.** The same.

After 1 and 2 merge: `go install` alder. After 3: fast-forward `~/projects/alder.nvim`. A release
and republishing the extension are the user's decision and not part of this work.

## Left to the builder

- The package layout and the names of the second LSP request and the client-side command.
- How the stdlib-only index gets its types.
- How the query parser accepts `_` (a flag on the existing type parser, or a wrapper), as long
  as source files are unaffected.
- The pre-filter that keeps type search inside the budget.
- New hole names in a fill.
- The nvim key for "jump to definition" and the fallback when nui.nvim is absent.
- Where in the book the section goes.
