You are writing code in Alder, a small language for server-rendered web
apps. Alder is not Elm, not TypeScript, and not any language you have seen
before; the reference below is complete. Anything not in the reference does
not exist. Follow it exactly.

====================  ALDER REFERENCE  ====================

====================  COMMANDS  ====================
alder new <app>   the guestbook: App, Layout, table Entry, page Home at /, its tests. --bare: App
(empty session) and Layout only; start a task there. Typical: tables, `new auth` (before any
`--requires user` page), pages (`--blank` first, then fill: a body may name only what exists), fills.
The generators check a copy, writing only if it checks (edit table/session: written with the
breakage listed; --strict refuses); none overwrites. All take --dir D, --dry-run, --json, --help.
A hole stops `alder test` (it refuses while one is left): run it last, after the fills. Pass stdin with <<'EOF' (echo eats the `\f` of `\from`).
alder new table <Name> <field>:<Type>[=default][@ref=T][@cascade|@restrict|@setNull][@owner][@partition=k][@unique] … [--silo]
  adds created/updated (Time @created/@updated) unless --no-timestamps (then `created:Time@created`);
  `org:Org.OrgId@ref=Org@partition=org`; --silo puts it in App's `silo org` (a table referring to a siloed one must)
alder new silo <key> <T> … [--empty]   App's `silo key { T, … }`; each T must be @partition key;
  a T migrations/ create needs --empty (no rows yet: it writes the move's migration)
alder new route [<Module>] <path> [--method GET|POST|PUT|PATCH|DELETE [--form F]] [--list T | --show T [--by <unique col>] | --from T …]
  [--blank] [--param <name>:<Type> …] [--requires user|org] [--role R]   (a write route has only update)
  --requires org writes `requires user, org`; --list/--show read an org-partitioned T with Db.scoped/getScoped
  a {x} in the path needs --param x:Type (load is then ?load); --show, --list and --from make their own
  parameters: --param only with --blank or a plain route. A --list route's Model and load go together:
  replace both (fill --decl), or start --blank
alder new form <Route> <Name> <field>:<Type>[=default] … [--into T] [--hole] [--msg <Route>] [--method POST|PUT|PATCH|DELETE] [--path /p]
  --msg <Route>: the form posts to that existing write route, which then decodes it (`form F`)
alder new action <Route> <Name> [<field>:<Type> …] [--id id] [--method POST|PUT|PATCH|DELETE] [--path /p]
  a write route and its button in the page's view (`Route.Name`, update starts as Response.refresh);
  App for a button on any page. form and action refuse while view is ?view: fill view first; the view
  must be one element over several lines (`<Layout.Page …>` … `</Layout.Page>`)
alder new capability <Module> <entry> "<Type>"
alder new auth [--users User] [--role R] [--to Route]   the login (Auth.Login, Password with a real
  PBKDF2 adapter, App's session user/role, login, Logout); creates User (passwordHash: Text.Secret)
  if absent, else uses yours; R must exist; no / route and no --to: ?afterLogin, fill it later
alder edit auth --to <Route>   fills ?afterLogin in Auth.Login.update and its test (refused if none)
alder edit decl Layout.Page add-prop <name>:<Type>=<default>   a prop on a component; every use keeps checking
alder new type <Module> "Status = A | B" | "Slug = brand Text where …" | "Row = { … }"
alder new decl <Module> <name> "<Type>"   (name = ?name; fill a function with a lambda)
alder new test <Module> "<name>"   (its block is ?test : Task Bool; fill it with a block)
alder new route … --blank   Model = { }, load and view holes; with --show T keeps /{id}
alder new action … --in-for <binder> [--when <Bool expr>]   the button in that `for`'s element,
  class = the action's name, fields from the binder (u.id) or holes; its test clicks a hole:
  insert rows the loop shows the signed-in user, fill the hole with one
alder edit route <Route> add-param <name>:<Type>[=default]
alder edit session add-field <name>:<Type>=<default>
alder edit session add-field org:Org.OrgId via Membership [--choose <Route>]   a key: `org: Maybe Org.OrgId via
  Membership` (+ `choose org = Route`); the Membership table and the route must exist first
alder remove route <Route> | remove table <T>   refused while used; a table's migration drops data
alder fill --decl <Module>.<decl> -   replaces a whole body (value, test, type's right side):
  for Model (no hole) and bodies that do not fit; prefer a hole when there is one. The parameters
  of `load params session = ?load` are bound: send the body, not `\params session -> …`
alder edit table <T> add-field <f> | rename-field <old> <new> | drop-field <f> [--strict]
  add-field without a default (=Nothing for a Maybe) breaks every insert of T (tests too), which
  no command fixes: give a default, or drop-field it
alder fill <address> - [--hole ?name]   fills a hole only (an arm without one: --decl update);
  fill --list. Addresses: Module.decl, Module.update.Msg, Module."test name", Module.Fake.entry;
  an app module the expression names (Member.x) is imported
alder new admin [--module Admin] [--path /admin]   (needs a session role with Admin)
alder regen admin [--module Admin] [--path /admin] [--all]
App may declare where it ships (book 16), once: `deploy { production: Ssh "u@host" { domain: "d",
  path: "/srv/x" }, staging: Fly "ord" { app: "a" }, local: Script "./s.sh", floor: 0.9, checks: ["cmd"],
  requires: ["SMTP_URL"], backup: S3 "s3://b/p", keep: 5, proxy: None }`; names only, never a secret (E0030)
alder deploy <env> [--dry-run] [--host h] [--allow-data-loss] [--unsafe-skip-tests]   the gate (check, build,
  test, --mutate >= floor, checks) then plan and ship; alder status|logs <env>; alder rollback <env> [id]
  [--restore-backup --yes]; alder restore <env> [--silo org:<id>] [--to t] [--out f] [--yes]; alder ci init
  (Fly: experimental, refuses a deploy with a pending migration); with silos: alder migrate silos <env>
  [--allow-data-loss], alder silos prune <env> [--yes]

# 3. Syntax

## Files and modules

One module per file. The first line names the module, and the module name
is the file path: `Todos.Index` lives in `app/Todos/Index.alder`.

```alder fragment
module Todos.Index

import Table.Todo
import Layout
```

`import X` brings module `X` into scope under a short name, its
*qualifier*: `X.foo`, `X.SomeType`, `<X.Component>`.

Modules from the standard library (`List`, `Text`, `Db`, ...) are always in
scope and never imported.

> **Stdlib names are taken.** No module of the app may take a
> standard-library module's name: an app module `Test` would be hidden
> behind the stdlib's `Test`, so it is refused (E0017); rename the module,
> its file and its imports (`AppTest`, say, or `Todos.Fixtures`), or use the
> standard library's module in its place.

### The qualifier

A module is named by the last segment of its path: after `import Table.Todo`
you write `Todo.find`, never `Table.Todo.find`.

Most imports in an app have no neighbour that ends the same way, so the
qualifier is the last segment and the import line is plain
`import Table.Todo`. When two imports would share a qualifier, or the
qualifier would hide something, the import gets a longer one and says so
with `as`:

```alder fragment
module Todos

import Admin.Layout as AdminLayout
import Layout
import Table.Todo as TableTodo
```

Here `Layout` is the plain layout, `AdminLayout.Page` is the admin one, and
`TableTodo` is the module in `app/Table/Todo.alder`, because `Todos`
declares a `type Todo` of its own and `Todo` is already spoken for.

There is no choice in the spelling. The compiler computes exactly one
qualifier for each import, the *canonical* one, and the `as` must match it:

- An import whose qualifier is its last segment has no `as` (a redundant
  one is E0204).
- Any other import must spell the canonical `as` (a missing or different
  one is E0204 as well, and the error's fix rewrites the line).

You never invent an alias; you copy the one the error shows, or run
`alder fmt`.

> **A wrong alias.** The compiler looks every import up by its canonical
> qualifier whatever alias was written, so a file with `as AL` and uses of
> `AL.x` reports E0204 and unknown names until the alias is fixed.

### How the compiler computes it

The resolver works on one file's imports at a time. Every import starts
with a one-segment candidate, the last segment: `Table.Todo` starts as
`Todo`. It lengthens a candidate by one segment from the right (`Todo`
becomes `TableTodo`, the segments joined with no separator) while either of
these holds:

- another import of the file has the same candidate (two imports ending in
  `Layout`, or two ending in `Index`); or
- the candidate is *taken*: a standard-library module name or `App`, the
  last segment of the file's own module name, or the name of a type,
  table, capability or fake table the file declares.

It stops when the candidate is free or when the whole path is in it.

#### Worked examples

The imports of one file, worked:

| imports | qualifiers |
|---|---|
| `Table.Todo` | `Todo` |
| `Layout`, `Admin.Layout` | `Layout`, `AdminLayout` |
| `Posts.Index`, `Admin.Posts.Index` | `PostsIndex`, `AdminPostsIndex` |
| `Utils.List` | `UtilsList` |
| `Data.MnSos` in module `Sites.MnSos` | `DataMnSos` |
| `Table.Todo`, with a `type Todo` declared in the file | `TableTodo` |
| `Todos.Index`, `Projects.Index`, `Blog.Show` | `TodosIndex`, `ProjectsIndex`, `Show` |

Why, row by row:

- **`Table.Todo`** is `Todo`: nothing else ends in `Todo`.
- **`Layout` and `Admin.Layout`** both start as `Layout`. `Layout` is
  already its whole path and keeps it, so only `Admin.Layout` grows.
- **`Posts.Index` and `Admin.Posts.Index`** both start as `Index`, then both
  read `PostsIndex`. `Posts.Index` has no more segments, so only the other
  grows.
- **`Utils.List`** is `UtilsList`: `List` is a standard-library module.
- **`Data.MnSos` in module `Sites.MnSos`** is `DataMnSos`: `MnSos` is the
  module's own last segment.
- **`Table.Todo`, with a `type Todo` declared in the file,** is `TableTodo`:
  `Todo` is a declared name.
- **`Todos.Index`, `Projects.Index` and `Blog.Show`:** only the two `Index`
  imports collide.

#### Two details

The file's own last segment blocks a candidate only while the path can still
grow: inside module `Admin.Layout`, `import Layout` is legal and keeps
`Layout`, since its whole path is all there is.

> **Mind the reader.** Inside `Admin.Layout`, `Layout.Page` names the
> *other* module, the plain layout, not the file you are in.

And some imports cannot be saved. `A.X` and `AX` alone are fine (`X` and
`AX`), but add `B.X`: then `A.X` and `B.X` both start as `X` and grow to
`AX` and `BX`, and `A.X` now ends as the whole join `AX`, the same as the
import `AX`, which cannot grow either. Neither has a usable qualifier
(`B.X` keeps `BX`). That is E0206, and its message suggests renaming a
module.

The generators of chapter 15 and the language server's completions use this
same computation, so what they write is always canonical.

### Always the qualifier, never the path

Writing the full path of an imported module where its qualifier belongs is
E0205: `Table.Todo.find` for `import Table.Todo` must be `Todo.find`, and
`<Admin.Layout.Page>` must be `<AdminLayout.Page>`. The compiler still
understands the name, so that one error is the only one, and its fix is the
qualified spelling.

The same holds for the members of an imported module, those of its routes
included: with `import Todos.Index`, its helper is `Index.helper` and a
member of its route `Row` is `Index.Row.view`, not `Todos.Index.Row.view`.

`alder fmt` applies the E0204 and E0205 fixes for a file of an app project
(chapter 11), so one run migrates an older project.

### A declaration named like its module

A declaration named like the module's last segment is referred to by the
module's qualifier alone: module `Todo` defining `type Todo` (or
`table Todo`) is used as `Todo`, not `Todo.Todo`; module `Layout` defining a
component `Layout` is used as `<Layout>`.

With `import Table.Todo` that is `Todo` for the `type Todo` in
`app/Table/Todo.alder`, and with `import Table.Todo as TableTodo` the same
type is `TableTodo`.

### Addresses in prose

When the book or a compiler message names a declaration by where it lives,
it writes the whole address: `Todos.Index.Index.load` (the module
`Todos.Index`, its route `Index`, the member `load`) or
`Todos.Index.recent`.

That is a name for the reader, not code to type: in a file you write
`Index.recent` after `import Todos.Index`.

### Route values are global

A route value (`Todos.Show.Show { id: id }`: the module `Todos.Show`, its
route `Show`, the parameters) and an app-level route (`App.Logout`) can be
used from any module without an import.

Everything else a module uses, a route's members (`Index.Row.view`)
included, must be imported, and imports may not form a cycle. Chapter 6,
"Names", says how a dotted name is read.

### Constructors are never qualified

If `Todo` defines `type Priority = Low | Normal | High`, a module that
imports `Todo` writes `Normal`, not `Todo.Normal`, while the type is still
`Todo.Priority`. Constructor names must therefore be unique across a
module's imports and the standard library; a clash is a compile error.

#### A constructor named like a table

A constructor may share its name with a table, which a role makes likely
(`type Role = Reader | Member` beside `table Member`). Then the type
expected decides:

- Where a table is expected, as the first argument of a `Db.` operation
  (`Db.from Member`) or a `Table Member` parameter, it is the table.
- Where the union is expected (`role: Member` in a session record,
  `session.role == Member`), it is the constructor.
- Anywhere else (an unannotated `let who = Member`) it is an error naming
  both: "`Member` is both a constructor of `User.Role` and a table: write
  `Member` for the constructor only where a `User.Role` is expected, or
  qualify the table `Member.Member`".

### Names that hide each other

A route may not take the name of a constructor, brand or table in scope in
its module (E0044, chapter 6, "Names"): `Active { … }` could not mean both.

Another pair of names can hide each other, because of the order in which a
name is looked up: a qualifier is looked up before a capability. So
`import Clarity` beside a module whose `capability Clarity` has the entry
`latest` makes `Clarity.latest` a name in module `Clarity`.

The capability is still reached through its own module's qualifier
(`Feeds.Clarity.latest`, or `DataClarity.final` for a capability named like
its module).

- A call that module `Clarity` lacks is an error whose fix is that
  spelling.
- A call it has compiles to the module's value, and the import gets a
  warning (W0010) that says so.

### Visibility

Everything a module defines is visible to other modules, with one exception:
a brand's constructor (see chapter 4). There is no `export` list.

## Names

Names are letters, digits and `_`. A name that starts with a lowercase
letter or `_` is a value, a field or a type parameter; one that starts with
an uppercase letter is a type, a constructor, a module or a component.

A name may not contain two underscores in a row (`max__len` is an error),
and a module name segment may not end with `_`: the compiler joins the parts
of the Go names it generates with `__` (chapter 13), so these are the names
it keeps for itself.

### Generated types

For each route the compiler also generates types named after it:
`Index.Params`, the route's parameter record, and `Index.Session`, its
session (chapter 6, "Qualified declarations"). They are qualified by the
route, so they never meet a type of your own.

### Reserved type names

A type, table, form or union of any module may not take the name of a
Prelude type that programs write, which every module uses unqualified
(E0012):

`Query`, `Task`, `Html`, `Maybe`, `Result`, `Response`, `Route`, `File`,
`List`, `Dict`, `Int`, `Float`, `Text`, `Bool`, `Unit`, `Date`, `Time`,
`Never`, `Error`.

Nor may one take `Doc` or `Answer`, the Prelude's types for tests
(chapter 10), and no constructor may be named like one of `Answer`'s:
`Shown`, `Rendered`, `Refreshed`, `Redirected`, `FormRejected`,
`PageMissing`, `RequestFailed`.

> **Db's own types stay free.** Db's own types, such as `Order` and `Page`,
> stay free: `table Order` is fine.

## Comments

```alder
-- A line comment. There are no block comments.
```

## Declarations

A module is a list of declarations. Order does not matter; a declaration can
refer to one defined later in the file.

```alder
-- Value with a type signature. The signature is required.
greeting : Text
greeting = "hello"

-- Function. Parameters are written after the name.
add : Int -> Int -> Int
add a b = a + b

-- Type (chapter 4)
type Filter = All | Active | Completed

-- Route, form, table, capability, test (later chapters)
```

Names of values and functions start with a lowercase letter. Names of types,
constructors, modules, and components start with an uppercase letter.
Multi-word names are `camelCase` or `PascalCase`.

### Route declarations

A page, and every action it sends, is a **route**: a `route` line
(`route NAME = METHOD "path" { params }`, then an optional `form F` and `requires …`) and
the declarations written `Route.name` (`R.load`, `R.view`, `R.update`, with the signatures of
chapter 6); an unqualified declaration belongs to the module. A qualifier that is no route of
the module is E0044.

## Literals

```alder
42            -- Int
3.14          -- Float
True  False   -- Bool
"text"        -- Text
"Hi {name}"   -- Text with interpolation. The expression must be Text.
"a \"quoted\" \{brace} \n"   -- escapes: \" \\ \{ \n \r \t
[1, 2, 3]     -- List Int
{ x: 1, y: 2 }   -- record
()            -- Unit
```

String interpolation inserts a `Text` only. To insert an `Int`, write
`"Total: {Text.fromInt n}"`.

Inside `{…}` is an ordinary expression, so it may contain string literals
of its own, written as usual without escaping:
`"tags: {Text.join ", " tags}"`.

## Function application

Functions are applied by juxtaposition and are curried. With `add` from
above (a block's leading `let` lines are how `alder eval` takes definitions;
chapter 11):

```alder eval=3
let add : Int -> Int -> Int = \a b -> a + b
add 1 2
```

```alder eval=[2, 3, 4]
let add : Int -> Int -> Int = \a b -> a + b
List.map (add 1) [1, 2, 3]
```

Use parentheses to group. Use `|>` to pipe a value into the last parameter:

```alder eval=1
let todos = [{ title: "a", done: False }, { title: "b", done: True }]
todos
  |> List.filter (\t -> not t.done)
  |> List.length
```

There is no `<|`, no `<<`, no `>>`.

## Lambdas

```alder eval=<function>
\x -> x + 1
```

```alder eval=3
(\a b -> a + b) 1 2
```

A parameter may carry a type annotation in parentheses, `(name : Type)`, and
annotated and plain parameters mix freely:

```alder eval=3
(\(a : Int) b -> a + b) 1 2
```

## Operators

| Operator | Types | Meaning |
|---|---|---|
| `+ - * /` | `Int` or `Float` (both sides the same) | arithmetic; `/` on `Int` truncates |
| `%` | `Int` | remainder |
| `++` | `Text` or `List a` | concatenation |
| `== !=` | any type without functions or effects (both sides identical) | equality |
| `< <= > >=` | `Int`, `Float`, `Text`, `Date`, `Time`, payload-less unions | comparison |
| `&& \|\|` | `Bool` | logic, short-circuit |
| `not` | `Bool` | negation (a function, not an operator) |
| `??` | `Maybe a` and `a` | default: `m ?? d` is `d` when `m` is `Nothing` |
| `\|>` | | pipe |

Precedence, highest first: function application; `* / %`; `+ - ++`;
`== != < <= > >=`; `&&`; `||`; `??`; `|>`. There is no user-defined operator.

Two places in that order surprise people:

- **`??`** binds looser than `&&` and `||`, so `ready && m ?? True` is
  `(ready && m) ?? True`: the default applies to the whole Bool, and the
  checker answers "expected Maybe _, got Bool". Write
  `ready && (m ?? True)`.

- **`|>`** binds looser than the comparisons, so `x |> f == y` is
  `x |> (f == y)`: a Bool on the right of the pipe, "right side of `|>` must
  be a function". Write `(x |> f) == y`.

In both cases the error carries a hint that spells out how the line was
read and the parenthesized form, built from the expressions you wrote.

## Records

```alder eval={ x: 10, y: 0 }
let origin = { x: 0, y: 0 }
let x = origin.x                 -- field access
{ origin | x: x + 10 }           -- copy with fields changed; fields must already exist
```

Record literals are checked against the expected record type. Every field
must be present, and no extra fields are allowed. There is no field punning:
write `{ id: id }`, not `{ id }`.

## Conditionals

```alder eval="done"
let todo = { title: "a", done: True }
if todo.done then "done" else "open"
```

`if` always has an `else`. Both branches have the same type.

## Match

```alder eval=[{ title: "a", done: False }]
let filter = Active
let todos = [{ title: "a", done: False }, { title: "b", done: True }]
match filter {
  All -> todos,
  Active -> List.filter (\t -> not t.done) todos,
  Completed -> List.filter (\t -> t.done) todos,
}
```

Every arm is `pattern -> expression`, separated by commas. A trailing comma
is allowed. `match` must cover every case; there is no wildcard-by-default.

Patterns:

```alder
match m {
  Just x -> x,                 -- constructor with payload
  Nothing -> 0,
}

match shape {
  Circle r -> r * r,
  Rect { w: w, h: h } -> w * h,   -- record pattern, same syntax as a record literal
}

match n {
  0 -> "zero",                 -- literal
  _ -> "other",                -- wildcard, allowed only as the last arm
}

match xs {
  [] -> "empty",
  [x] -> "one",
  [x, ..rest] -> "many",       -- head and tail
}
```

Patterns nest at any depth: a constructor, list, record, literal, wildcard or
variable may stand inside a list element, a record field or a constructor's
argument.

```alder eval=6
let parts = Text.words "1 2 3" |> List.map Text.toInt
match parts {
  [Just a, Just b, Just c] -> a + b + c,   -- exactly three numbers
  _ -> 0,
}
```

List elements and record fields take any pattern bare: `[Just m, Nothing]`,
`{ left: Just a, right: _ }`. A constructor's argument that is itself an
applied constructor or a negative number needs parentheses, `Just (Just x)`,
`Just (-1)`, as in an expression; a list argument does not, `Just [a, ..rest]`.
Arms are tried top to bottom and the first whose whole pattern matches runs.

The checker proves nested matches exhaustive too. Without the `_` arm above,
the error lists every missing case as a pattern you can paste, `_` standing
for any value and `..rest` for any tail: `` `[]` ``,
`` `[Nothing, ..rest]` ``, `` `[Just _]` ``, … An arm that earlier arms already
cover in full, such as `[Just 1]` after `[Just x]`, is an error (E0310).

## Blocks

A block is a sequence of `let` statements followed by one expression, in
braces. Statements are separated by newlines.

```alder
summary : List Todo -> Text
summary todos = {
  let open = List.filter (\t -> not t.done) todos
  let n = List.length open
  "{Text.fromInt n} open"
}
```

A `let` may carry a type annotation: `let n : Int = List.length open`.

Inside an effect block (a block whose final expression is a `Query` or
`Task`, chapter 7), `let x <- effect` runs the effect and binds its result:

```alder
openCount : Query Int
openCount = {
  let todos <- Db.from Todo |> Db.all
  Query.succeed (Todo.countOpen todos)
}
```

`let x <- ...` is only allowed in a block whose result is a `Query` or
`Task`, and the effect on the right must be of the same kind.

A block is an expression and can appear anywhere an expression can. A
function whose body is a single expression does not need a block.

## Whitespace

Whitespace is not significant except that statements in a block are
separated by newlines. An expression may continue onto the next line
when the line ends with an operator, when it is inside unclosed brackets,
or when the next line begins with a binary operator: `|>`, `??`, `||`,
`&&`, `==`, `!=`, `<`, `<=`, `>`, `>=`, `+`, `-`, `++`, `*`, `/` or `%`.

```alder eval=True
List.length [1, 2, 3]
  == 3
```

A line that begins with anything else starts something new, so `f 1`
followed by a line `    = 2` is E0001.

Two more places take a line break. A type may continue onto a line that
begins with `->`, and may begin on the line after the `:` of a signature:

```alder fragment
fullName :
  { first: Text, last: Text }
  -> Text
```

And the attributes of an HTML open tag may each sit on their own line,
with the closing `>` or `/>` on a line of its own:

```alder fragment
<a
  href="/about"
  class="link"
>About</a>
```

`<` is the one operator that can also begin a line of markup. A line
that begins with `<` and then whitespace (`< x`) continues the
comparison; a line that begins with `<` and then a letter or `/` (`<li>`,
`</ul>`) starts an element. So write a comparison continued on the next
line with a space after the `<`:

```alder eval=True
List.length [1, 2, 3]
  < 10
```

and a block whose last line is an element ends with that element, not
with a comparison:

```alder eval=<p>3</p>
{
  let count = List.length [1, 2, 3]
  <p>{Text.fromInt count}</p>
}
```

## Style

`alder fmt` rewrites a file into the one canonical format, so write the
code, run `alder fmt`, then `alder check` (chapter 11, "Formatting"). The
rules, so that the first draft already matches:

- **Layout.** 2-space indent, no trailing whitespace, one blank line between
  declarations, a signature directly above its value. Lines break at 100
  columns where no rule below decides.

- **Imports** are sorted, one per line.

- **Records.** A record, record type, form, table, `session`, `deploy` or
  `theme` (and each record inside it) with 3 or more fields is always
  vertical, one field per line with a trailing comma; with 1 or 2 it stays
  on one line if it fits.

  - A `route` line is never wrapped, and the `route` lines that follow each
    other are aligned in columns: name, method, path, parameters, then
    `form` and `requires`.
  - The record type of a function's signature, a capability or a fake
    breaks only when too wide.
  - An empty record expression or pattern is `{}`; an empty record type,
    form or declaration body is `{ }`.

- **A record update** `{ r | a: x }` follows the same field-count rule, but
  never takes a trailing comma (the grammar forbids it): vertical, it is
  `{ r |` on the opening line, one field per line, `}` alone.

- **A `match`** has one arm per line, each ending in a comma.

- **A sum type** that does not fit on a line puts the first constructor bare
  on the line after `=`, and each of the rest on its own line led by `|`:
  `type Msg =` / `  Increment` / `  | Decrement` / `  | SetValue Int`.

- **`if`** that does not fit on a line is `if c then` / `else` with the
  branches indented; `else if` chains stay at one indent. A block branch is
  `if c then {` ... `} else {` ... `}`.

- **A pipeline** with 2 or more `|>` is always vertical: it starts on the
  line after the `=` (or `let x =`, `let x <-`), and each `|>` leads a line
  of its own, indented 2 more. A lambda argument is parenthesised:
  `List.map (\x -> x + 1) xs`.

- **A block** with statements is vertical. Parentheses are only the ones
  precedence needs, except around a lambda argument.

- **Markup:** an element with 2 or more element children (a `{for}` counts
  as 2) puts each child on its own line.

- **Comments** stay where they are; a trailing comment keeps its list
  vertical.

A record type of nine fields, written on one line:

```text
type Post = { id: Int, title: Text, body: Text, author: Text, slug: Text, views: Int, draft: Bool, created: Int, updated: Int }
```

is formatted as:

```alder fragment
type Post = {
  id: Int,
  title: Text,
  body: Text,
  author: Text,
  slug: Text,
  views: Int,
  draft: Bool,
  created: Int,
  updated: Int,
}
```

A pipeline and a record update of three fields:

```alder fragment
positives : List Int -> List Int
positives xs =
  xs
    |> List.filter (\x -> x > 0)
    |> List.reverse

reset : { a: Int, b: Int, c: Int } -> { a: Int, b: Int, c: Int }
reset r = { r |
  a: 0,
  b: 0,
  c: 0
}
```

## Reserved words

```
module import type brand where route page msg form table capability fake
session login test let match if then else for in requires
```

(`page` and `msg` are the old form of a route, E0046; rewrite it as
routes, chapter 6.)

The declaration-head words (`module import type route page msg form table
capability fake session login test`) are reserved only at the start of a line
(column 1). Elsewhere they are ordinary names, so `load params session = …`
and `{ form: 1 }` are valid. The remaining words are reserved everywhere.

# 4. Types

## Built-in types

| Type | Values |
|---|---|
| `Int` | 64-bit integers |
| `Float` | 64-bit floats |
| `Bool` | `True`, `False` |
| `Text` | UTF-8 strings |
| `Unit` | `()` |
| `List a` | `[1, 2, 3]` |
| `Maybe a` | `Just x`, `Nothing` |
| `Result e a` | `Ok x`, `Err e` |
| `Dict k v` | built with `Dict.fromList` (keys: see below) |
| `Date`, `Time` | calendar date; instant (chapter 9) |

There is no `Char` and no `String`: text is `Text`. There is no tuple: use
a record.

### Dict keys

`k` may be any type that can be compared with `==`: no functions or effect
values in it, including inside a union's payloads.

Two keys are the same key when they are `==` (so `Html` keys are compared by
their markup).

## Int

`Int` is a 64-bit two's-complement integer.

- Arithmetic that goes past the largest or smallest `Int` wraps around:
  `9223372036854775807 + 1` is `-9223372036854775808`.
- `/` truncates toward zero (`-7 / 2` is `-3`), and `%` has the sign of the
  dividend (`-7 % 2` is `-1`).
- Dividing an `Int` by zero, with `/` or `%`, is a runtime failure: the
  program stops where it happens (later milestones turn it into a failed
  task). This holds for constants too; `1 / 0` compiles and fails when it
  runs.

`Float` is a 64-bit IEEE float: `1.0 / 0.0` is infinity, not a failure.

## Type signatures

Every top-level value and function has a signature on the line before it.
Function types use `->`. Type parameters are lowercase names.

```alder
first : List a -> Maybe a
first xs = List.head xs
```

A signature may use any type parameter; there are no constraints on type
parameters (no typeclasses). Functions that need equality or ordering take
only concrete types, or take a comparison function.

## Records

A record type lists fields and their types:

```alder
type Point = { x: Float, y: Float }
```

Record types are **nominal**: a `Point` is not interchangeable with another
record type that happens to have the same fields.

An anonymous record literal `{ x: 1.0, y: 2.0 }` is given the type the
context expects, even when the context is seen later (a `let`, one side of
`==`, a lambda in a pipeline): it becomes the first record type it meets
with exactly its field names.

A signature may spell out a record type inline when it is used only once:

```alder
area : { w: Float, h: Float } -> Float
area r = r.w * r.h
```

## Unions

```alder
type Filter = All | Active | Completed

type Shape
  = Circle Float
  | Rect { w: Float, h: Float }

type Tree a
  = Leaf
  | Node { left: Tree a, value: a, right: Tree a }
```

A constructor takes zero or one argument. For several values, pass a record.
Constructors are functions: `Circle : Float -> Shape`.

`Maybe` and `Result` are ordinary unions defined in the standard library.

A union whose constructors all take no argument is **ordered by declaration**:
for `type Priority = Low | Normal | High`, `Low < Normal < High`. The
comparison operators work on such unions, and so does `Db.orderBy`.

## Brands

A brand is a type that has the same runtime representation as its base type
but is distinct to the type checker:

```alder
type TodoId = brand Text
type Cents = brand Int
```

Rules:

1. **Widening is implicit.** A `TodoId` can be used anywhere a `Text` is
   expected, with no conversion.

2. **Narrowing is explicit and local.** The constructor
   `TodoId : Text -> TodoId` exists only inside the module that declares the
   brand. Other modules receive brand values from boundaries (route
   parameters, forms, database rows) or from functions the declaring module
   exports.

3. **Operations return the base type.** `Text.trim id` is a `Text`, not a
   `TodoId`.

   > **Arithmetic on a numeric brand.** `+`, `-`, `*`, `/`, `%` on two
   > values of the same numeric brand return that brand
   > (`Cents + Cents : Cents`).

4. **Equality requires identical types.** `id == text` is an error even
   though `id` widens to `Text`.

5. **A brand's base is a built-in type**, never another brand.

### Two different brands

Rules 1 and 3 together decide arithmetic on two *different* brands of one
base: both widen to the base, and the result is the base. An
`Int.NonNegative` times an `Int.Positive` is an `Int`, and so is
`Cents + Int.Positive`; only `Cents + Cents` stays `Cents`.

```alder eval=6
-- 6 : Int, neither brand
(Int.NonNegative 2) * (Int.Positive 3)
```

> **Constructors in `alder eval`.** `Int.NonNegative 2` narrows outside the
> brand's module, which rule 2 forbids in a program: in `alder eval`, as in
> a `test`, any brand constructor may be used; chapter 11.

### Comparing brands

Comparison follows rule 1 too: `<`, `<=`, `>` and `>=` on two different
brands of one numeric base compare them as the base, as they compare a
brand with its base (`Int.Positive 2 < 3`). Only `==` needs identical
types (rule 4).

```alder eval=True
(Int.Positive 2) < (Int.NonNegative 3)
```

Rule 4 is a type error, not a silent widening:

```alder error=E0003 "`==` needs both sides to have the same type"
type Cents = brand Int

sameAmount : Cents -> Int -> Bool
sameAmount c n = c == n
```

## Refinements

A brand may carry a check. The check is a `Bool` expression over the base
value, written as a function applied at boundaries:

```alder
type Slug = brand Text where Text.matches "^[a-z0-9-]+$"
type Age = brand Int where (\n -> n >= 0 && n < 150)
```

When a value of this type is decoded from a route, a form, or a database
row, the check runs. If it fails, the form shows an error or the route is a
404. Inside the program the value is trusted and never re-checked.

The standard library defines common refinements:

```alder
Text.NonEmpty     -- length > 0 after trimming
Text.Email        -- contains an @ with text on both sides
Text.Secret       -- non-empty; rendered as a password input, never logged or echoed
Int.Positive      -- > 0
Int.NonNegative   -- >= 0
```

These are ordinary brands of `Text` and `Int` and widen the same way.

## Type parameters on records and unions

```alder
type Results a = { items: List a, total: Int }
```

## What there is not

- No type aliases. A new name is always a new type.
- No typeclasses, interfaces, or traits.
- No tuples.
- No record that contains itself by value (`type R = { r: R }`, directly
  or through other records, is E0011): a record may refer to itself only
  through `Maybe`, `List`, `Dict` or another union.
- No subtyping other than brand-to-base, and a `requires` route's `Session`
  to the app's (chapter 6).

# 5. HTML

HTML is a built-in expression form. An HTML expression has type `Html`. What
a button, a form or a link leads to is a **route** (chapter 6), written in
the markup as a value, so the type of the markup does not have to say it.

`Html` is the whole type, in a route module and in any other (`Layout`, `App`,
a shared component). It takes no type argument: `Html Msg`, `Html a` and
`Html Never` are E0046; write `Html`.

## Elements

```alder eval=<ul class="todos"><li>First</li><li>Second</li></ul>
<ul class="todos">
  <li>First</li>
  <li>Second</li>
</ul>
```

Elements are written as in HTML. Every element is closed: `<li>...</li>` or
`<br />`. Tag names are the standard HTML5 set; an unknown tag is a compile
error. A tag with a hyphen is a custom element (below).

## Expressions in HTML

Braces embed an expression:

```alder
<span>{todo.title}</span>
<ul>{items}</ul>
```

An embedded expression must have one of these types:

- `Text`: inserted as escaped text.
- `Html`: inserted as markup.
- `List Html`: inserted in order.

Nothing else is inserted implicitly. An `Int` must be converted with
`Text.fromInt`. A `Maybe Text` must be matched or defaulted with `??`.

Text is **always escaped**. To insert trusted markup, use `Html.unsafe`,
which takes a `Text` and returns markup. It is the only way to produce
unescaped output, and it must never be applied to text a user entered.

## Control flow in HTML

```alder
<div>
  {if model.todos == [] then <p>Nothing to do.</p> else <ul>{items}</ul>}

  {for todo in model.todos -> <TodoItem todo={todo} />}

  {match model.user {
    Just u -> <span>{u.name}</span>,
    Nothing -> <a href={Auth.Login.Login {}}>Log in</a>,
  }}
</div>
```

An `if` in markup always has an `else`, as `if` does everywhere. When one
branch should show nothing, write `else Html.none`:

```alder
<nav>
  {if model.user == Nothing then <a href={Auth.Login.Login {}}>Log in</a> else Html.none}
</nav>
```

`for x in list -> html` produces a `List Html`. There is no index
variable; use `List.indexed` first if an index is needed.

A line that begins with `--` between elements is a comment, as elsewhere.
`--` inside element text is text.

## Whitespace

Whitespace in element content follows the JSX rule, so a view can be
indented freely without adding spaces to the page, and a space written on
one line is never lost.

The text between two nodes (elements, `{…}` expressions, the start or end
of the parent) is one **run**.

- A run with no newline is kept as-is. In `<p><b>a</b> <i>b</i></p>` and
  `<p>{first} {last}</p>` the single space survives.
- A run that contains a newline is split at its newlines. Each line loses
  the whitespace that touches a newline; empty lines and comment lines are
  dropped; the lines left are joined with one space. A run that is only
  indentation disappears.
- Whitespace between text and a node on the same line stays: in
  `Hello {name}` the space after `Hello` is kept.

```alder eval=<p><b>a</b><i>b</i></p>
<p>
  <b>a</b>
  <i>b</i>
</p>                       -- <p><b>a</b><i>b</i></p>: no space
```

```alder eval=<p><b>a</b> <i>b</i></p>
<p><b>a</b> <i>b</i></p>   -- <p><b>a</b> <i>b</i></p>: one space
```

```alder eval=<p>Hello Ann</p>
let name = "Ann"
<p>
  Hello {name}
</p>                       -- "Hello " then the name
```

```alder eval=<p>one two</p>
<p>
  one
  two
</p>                       -- "one two"
```

A space at the end of a line touches the newline, so it is dropped too. To
put a space between elements on separate lines, insert it with `{" "}`.

## Attributes

```alder
<input type="text" name="title" value={model.draft} />
<li class={if todo.done then "done" else ""}>{todo.title}</li>
<button disabled={model.saving}>Save</button>
<a href={Todos.Show.Show { id: todo.id }}>{todo.title}</a>
```

Attribute values are either a literal string or a braced expression. The
expression's type depends on the attribute:

| Attribute | Type |
|---|---|
| most attributes (`class`, `id`, `value`, `placeholder`, ...) | `Text` |
| boolean attributes (`disabled`, `checked`, `required`, `controls`, `muted`, ...) | `Bool` |
| `href` | the value of a `GET` route (chapter 6) |
| `src` | `Text`, or a route value: `src={Url.external "https://…"}` |
| `click` | the value of any route |
| `submit` (on a `<form>`) | the value of a write route |
| `load` | the value of a `GET` route, fetched as the element appears (chapter 6, "Patterns") |
| `every` (beside `load`) | literal `Text`: a number and `ms`, `s` or `m` (`every="5s"`), repeating the `load` |
| `intersect` | the value of a `GET` route, fetched as the element scrolls into view |
| `input` (on a field) | the value of a `GET` route with a `Text` query key named as the field, fetched as the user types |
| any attribute | `Maybe Text`: omitted when `Nothing` |

A boolean attribute (HTML's: `controls`, `muted`, `async`, `defer`, …) is
written bare (`<video controls>`) or with a `Bool`, rendered present or
absent.

`href` never takes a `Text`. Links to external sites use
`<a href={Url.external "https://..."}>`.

Event attributes are not HTML event names (`onclick`); they are `click`,
`submit`, `load`, `intersect` and `input`, and they take a route, not a
function. The last three need JavaScript (chapter 6, "Patterns").

> **A route of the wrong kind** is E0045: `href`, `load`, `intersect` or
> `input` to a write route (a link, a timer or a keystroke only reads),
> `submit` to a `GET` route (a form posts), or `click` to a write route that
> declares a form (`form F`): a click sends a route without a form, and such a
> route is sent by a `<form submit={R}>`.

#### Buttons with `click`

A `<button click={…}>` works with and without JavaScript (chapter 6); a
`click` on any other element needs JavaScript.

`data-*` and `aria-*` attributes take `Text`.

## Scripts and styles

Scripts and styles belong in `static/`, but may be inline.

The body of a `<script>` or `<style>` is raw text, as in HTML: sent exactly
as written. `{`, `<`, `&&`, quotes and `--` in it are not Alder, so
`{model.name}` there is those characters, and no Alder value can be spliced
into a script or a stylesheet:

```alder eval=<script>if (a < b && ok) { go("{x}") }</script>
<script>if (a < b && ok) { go("{x}") }</script>
```

Data reaches a script through an escaped `data-*` attribute
(`<div id="p" data-clip={model.clip}>`, read as `p.dataset.clip`).

A body may not contain `</script` or `</style` in any case (split it:
`"<\/script>"`), nor be wholly one `{ … }` that is not JSON (E0001).

## Components

A component is a function from a record to `Html`, named with an
uppercase letter:

```alder
TodoItem : { todo: Todo } -> Html
TodoItem props =
  <li class={if props.todo.done then "done" else ""}>
    <span>{props.todo.title}</span>
  </li>
```

It is used as an element, with props as attributes:

```alder
<TodoItem todo={todo} />
```

Children are passed through a `children` field of type `List Html`:

```alder
Layout : { title: Text, children: List Html } -> Html
Layout props =
  <html>
    <head><title>{props.title}</title></head>
    <body>{props.children}</body>
  </html>
```

```alder
<Layout title="Todos">
  <h1>Todos</h1>
</Layout>
```

A component can contain any route: `click={Toggle { id: todo.id }}` names a
route value, so a component needs no declaration of what it sends, in a route
module or in any other. It is typed `Html` everywhere.

Components are plain functions. They have no state and no lifecycle.

## Forms and inputs

Inside `<form submit={R}>`, the `<Input>`, `<Select>`, `<Textarea>`, and
`<Errors>` components are bound to the form type of the write route `R`
(chapter 6, "`form`"):

```alder
<form submit={Submit}>
  <Errors form />
  <Input field="title" label="Title" />
  <Errors field="title" />
  <button>Add</button>
</form>
```

`<Errors field="x" />` shows the errors for one field; `<Errors form />`
shows form-level errors (from `Response.invalid`), then the errors of every
field that has no `<Errors field>` of its own, each after the field's name.

Each renders a list, empty until there is something to show, with one item
per message:

```html
<ul class="errors" data-errors="title"></ul>
<ul class="errors" data-errors="title"><li>must not be empty</li></ul>
<ul class="errors" data-errors="form"><li>Email or password is incorrect.</li></ul>
```

The first is the page as `GET` renders it; the other two are the same
pages after a failed submission (chapter 6, "Form errors"). `data-errors`
names the field, or `form`, so a stylesheet can hide an empty list with
`ul.errors:empty`.

### Which component for which field

Which component to use depends on the field's type:

| Field type | Component | Renders |
|---|---|---|
| `Text`, brands of `Text` | `<Input>` | `<input type="text">` |
| `Text.Email` | `<Input>` | `<input type="email">` |
| `Text.Url` | `<Input>` | `<input type="url">` |
| `Text.Secret` | `<Input>` | `<input type="password">` |
| `Int` | `<Input>` | `<input type="number">` |
| `Float` | `<Input>` | `<input type="number" step="any">` (see below) |
| `Date` | `<Input>` | `<input type="date">` |
| `Text` meant to be long | `<Textarea>` | `<textarea>` |
| `Bool` | `<Input>` | checkbox |
| union with no payloads | `<Select>` | `<select>` with one option per constructor, labeled by its name |
| `Text`, brands of `Text` (a row's id) | `<Select options={…}>` | `<select>` with one option per element of the list |
| `Maybe` of any of these | same as the inner type | not `required` |

Without `step` a browser takes only whole numbers, which is why a `Float`
renders `step="any"`.

### `required`

The browser's `required` is set exactly when the decoder refuses a blank
value, so the browser never blocks what the server would accept.

- **Not set** for a `Maybe`, a field with a default (blank is the default),
  a checkbox, plain `Text` (where `""` is a value) or a brand of `Text`
  without the stdlib's check.
- **Set** for `Text.NonEmpty`, `Text.Email`, `Text.Url`, `Text.Secret`,
  `Int`, `Float`, `Date` and unions.

The decoder still checks everything: the browser's checks are a
convenience.

### `value`

`value={...}` on any of them takes the field's own type
(`value={model.todo.priority}` for a `Select` over `Priority`), or `Maybe`
of it.

### Options from a list

A `<Select>` on a field of `Text` or a brand of it takes its options from
a list instead: `options={…}` is a `List { value: T, label: Text }`, where
`T` is the field's type without its `Maybe`. Each element is one
`<option>`, its `value` the text submitted and its `label` the text shown.

This is how a form picks a row of another table: the field is the row's
id, and `load` reads the rows to choose from (the admin of chapter 15
does this for every reference):

```alder
<form submit={Submit}>
  <Select field="author" label="Author" options={List.map (\u -> { value: u.id, label: u.name }) model.users} />
  <button>Save</button>
</form>
```

The list's values have the field's type, so an option cannot offer a
value the field could not hold: with `author: User.UserId`,
`{ value: "x", label: "X" }` is E0003, since only `User` makes a `UserId`
(chapter 4, "Brands"). The submitted text is decoded as the field's type,
the brand's check included, like any other input's.

The select starts with an empty option only when the field may be left
empty (a `Maybe`, a field with a default); otherwise the first option is
chosen until the user picks another.

#### Checking a posted option

A request can be crafted by hand, so the runtime checks a posted value against the options of
the page the form was posted from (`Alder-Page`, else `Referer`), drawn again. A value that is
not one of them is the field error "must be one of the options": `422`, the page again, no
`update`; an empty value passes only when the select offers the empty option; a post from a
page that does not offer the options is `400`. `options` may use only the view's parameters
(`model`, `session`) and names it binds itself, and the select must be in a `GET` route's
`view`, not in a `for`, a `let` or `match` binding, or a component (E0304).

**A list too long to render.** Keep the narrowing text as a parameter of the page's route
(`findAuthor: Text = ""`), have `load` fetch at most 50 rows that contain it, and add a second
form with its own write route whose `update` answers `Response.redirect` to the page with
`findAuthor` set (a `refresh` would keep the old text). The select is checked against the
narrowed page it is posted from.

A `<form submit={R}>` where the write route `R` has no `form` type is allowed;
it contains no bound inputs, only a button.

### Field names and values

`field` must name a field of the form type. The input's `name`, `type`,
`required`, and current value are derived from the field's type and the
submitted values.

`value={...}` (the field's own type, or `Maybe` of it) sets the initial
value for an edit form; a submitted value takes precedence over it.

A field of type `Text.Secret` renders as a password input and is never
echoed back: after a failed login, the email comes back filled in and the
password empty.

```html
<label>Password<input name="password" type="password" required /></label>
```

Each `<Input>` is wrapped in a `<label>` holding its `label` text.

> **A plain `<input>`** inside a `<form submit={...}>` is a compile error;
> use `<Input>`.

## Custom elements

A tag with a hyphen is a custom element, declared by the built-in
`alder-ui` manifest or the app's `elements/custom-elements.json`
(which wins over it). E0028: an undeclared tag, attribute or `slot="x"`; a quoted
value on a boolean attribute; a literal outside an attribute's quoted values (computed
ones are unchecked); `submit` on any custom element (it belongs on `<form>`); `slot` on
a child of a plain element (a component's children are not slot-checked). Always accepted:
`id`, `class`, `style`, `slot`, `hidden`,
`title`, `lang`, `dir`, `tabindex`, `role`, `data-*`, `aria-*`, `click`, `href`.

In a `<form submit>`, `<al-input field="f" />` is `<Input>` (E0304 rules),
`al-textarea` `<Textarea>`, `al-select` `<Select>`, `al-checkbox` `<Input>`
on a `Bool`; each renders its `help` and its own error list
(an `<Errors field>` for it is E0028).

The built-in elements (Appendix C; `!` boolean, `*` default, slots after `/`). Link
`/static/al/al.css` in the layout (every build serves it). An `<al-button>` with `click`,
`type` or `href` renders a real `<button>`/`<a>` inside, so its children are text and inline
markup only (no control, `Html` value or component) and it takes `href` or `click`, not both
(E0028); `<al-alert>` gets `role`. Link `/static/al/al.js` (`<script type="module" src="...">`,
a plain string) for the interactive elements. A button opens a dialog or popover with
`commandfor="id" command="show-modal|close|toggle-popover"` (no `click`); without
JavaScript dialog, popover, menu and tabs work natively, a tooltip is hidden (repeat its
text in `title`), a toast is not seen (use `al-alert`). Toasts come from `Notice.with
"text" nav`.

- `al-button` variant=primary*|secondary|outline|ghost|destructive|link, size=sm|md*|lg|xs|icon|icon-sm|icon-lg, disabled!, type=button*|submit, full! / default
- `al-input` label, placeholder, disabled!, invalid!, help
- `al-textarea` label, placeholder, disabled!, invalid!, help
- `al-select` label, disabled!, invalid!, help
- `al-checkbox` label, disabled!, invalid!, help
- `al-table` striped!, dense! / default
- `al-card` (none) / header, default, footer, actions, description, action
- `al-badge` variant=default*|secondary|outline|destructive / default
- `al-alert` variant=info*|success|warning|destructive, heading, dismissible! / default
- `al-pagination` page, pages / prev, default, next
- `al-stack` gap=sm|md*|lg|xl, direction=row|column* / default
- `al-page` width=narrow|normal*|wide, plain! / default
- `al-dialog` heading, size=sm|md*|lg, dismissible! / header, default, actions, description
- `al-popover` placement=top|bottom*|left|right, offset=sm*|md|lg, anchor / default
- `al-menu` anchor / default
- `al-tabs` variant=default*|line / default
- `al-tab` name, selected! / default
- `al-panel` (none) / default
- `al-toast` variant=info*|success|warning|destructive, timeout, Response.toast, Response.success, Response.warning, Response.error / default
- `al-toasts` position=top-left|top-right|bottom-left|bottom-right* / default
- `al-tooltip` for / default
- `al-devpanel` open! / default

## Partial updates

When a write route answers `Response.refresh` (or `Response.invalid`), the
runtime renders the page the browser is on twice: as it was before the
`update`, and again after it (the same `view`, of the model `load` gave each
time). It compares the two and sends the browser only the elements that
changed, each found by its `id`; Datastar morphs each into place, and leaves
the rest of the page alone.

Two other answers skip the comparison:

- `render r` sends the one element that `r`'s view draws, by its root `id`.
- `redirect r` sends the page `r` as a link does, never compared with the
  one on screen (chapter 6, "Delivery").

Without Datastar (JavaScript off), a write answers with a `303` (`invalid`:
a `422`) and the browser loads the whole page (chapter 6, "With and without
JavaScript").

### Regions

An element with an `id` is a **region**. The `<body>` is the outermost
one, and regions nest. When something changes, the runtime sends the
smallest region that holds the change, so the `id`s you give decide how
small an update is:

```alder
<ul id="todos">
  {for t in model.todos ->
    <li id={"todo-" ++ t.id}>{t.title}</li>}
</ul>
<p id="summary">{Text.fromInt (Todo.countOpen model.todos)} open</p>
```

- Ticking one to-do sends that `<li>` (and `#summary`, whose count
  changed): the list around it is the same.
- Adding or removing a to-do sends the whole `<ul id="todos">`: its
  children are not the same list any more.
- A change in text that no region holds (a heading with no `id` around
  it) sends the whole `<body>`, as a page without any `id` always does.

### What counts as a region

An `id` counts as a region only when it is a plain name (letters, digits,
`_` and `-`, not starting with a digit) and appears once on the page;
`id`s inside `<head>`, `<noscript>` or `Html.unsafe` markup do not count.

Nor does an element around markup the browser rebuilds as it reads it:

- a `<div>` inside a `<p>` (the browser closes the `<p>` first);
- a link inside a link;
- a form inside a form;
- text or a `<div>` directly inside a `<table>` or `<tr>`.

Its page is not the markup you wrote, so the runtime patches a region
further out, or the whole body.

### Why a patch is safe

None of this can make the page wrong: the runtime sends a region only when
it has proved everything outside it is unchanged, and when it cannot tell
which page the browser shows (the first click after a form showed errors,
for one) it sends the whole body.

`alder test` checks, after every click and submit, that the patched page
equals a full render (chapter 11).

# 6. Routes, responses, forms

## A route module

Every URL of an Alder app is a **route**: one `route` line with an HTTP
method and a path. A route that reads (`GET`) has a `load` and a `view`. A
route that changes something (`POST`, `PUT`, `PATCH`, `DELETE`) has an
`update`, which does the change and says what to send back. A module may
declare any number of routes. This is the whole shape of an Alder app.

```alder
module Todos.Index

import Layout
import Table.Todo

route Index  = GET   "/"                 { filter: Todo.Filter = All } requires user
route Row    = GET   "/rows/{id}"        { id: Todo.TodoId }           requires user
route Toggle = PATCH "/rows/{id}"        { id: Todo.TodoId }           requires user

type Model = { filter: Todo.Filter, todos: List Todo }

Index.load : Index.Params -> Session -> Query Model
Index.load params session = {
  let todos <- Db.mine Todo |> Db.orderBy Todo.created.desc |> Db.all
  Query.succeed { filter: params.filter, todos: List.filter (Todo.matches params.filter) todos }
}

Index.view : Model -> Session -> Html
Index.view model session =
  <Layout.Page title="Todos">
    <ul id="todos">{for todo in model.todos -> Row.view todo session}</ul>
  </Layout.Page>

Row.load : Row.Params -> Session -> Query Todo
Row.load params session = Db.getMine Todo params.id

Row.view : Todo -> Session -> Html
Row.view todo session =
  <li id={"todo-" ++ todo.id}>
    <button click={Toggle { id: todo.id }}>{todo.title}</button>
  </li>

Toggle.update : Toggle.Params -> Session -> Task Response
Toggle.update params session = {
  -- The id comes from the request: `Db.updateMine` changes the todo only when it is the
  -- signed-in user's, and answers 404 otherwise ("Rules", below).
  let _ <- Db.updateMine Todo params.id (\t -> { t | done: not t.done })
  Task.succeed (Response.render (Row { id: params.id }))
}
```

Three routes. `Index` is a page: its view includes the layout. `Row` is a
GET route whose view is one `<li>`; the page's view uses it for every todo,
and `Toggle` uses it to answer a click. `Toggle` is a `PATCH`: it flips one
todo and answers `Response.render (Row { id: … })`, which sends that one
`<li>` to the browser and nothing else.

The to-do app in `examples/todos` is this with a form to add a todo and a
button to delete one; its `Toggle` is a `PATCH` that answers
`Response.refresh`, the other way to answer ("`Response`", below).

`Db.mine Todo`, `Db.getMine` and `Db.updateMine` are the to-do table's
**scoped** operations: they read and write only the todos of the signed-in
user, because the table marks its `owner` column `@owner` ("Rules", below).

The compiler checks each route module against the shape of its routes.
A GET route has exactly one `load` and one `view`; a write route has
exactly one `update` and neither of those. There is no `Model` slot and no
`Msg`: the model is whatever `load` returns, and a button names a route.

`R.Params` is generated from the route line and must not be written by hand.
`view` receives the session so that per-user presentation (theme, "logged in
as") needs no model field.

## `route`

```alder fragment
route Show   = GET    "/todos/{id}"        { id: Todo.TodoId }
route Index  = GET    "/"                  { filter: Todo.Filter = All, q: Maybe Text }
route Toggle = PATCH  "/todos/{id}"        { id: Todo.TodoId }           requires user
route Create = POST   "/todos"             { } form NewTodo              requires user
route Delete = DELETE "/todos/{id}"        { id: Todo.TodoId }           requires user
route Write  = GET    "/write"             { }                           requires user, role Author
```

A route line has this shape:

```
route NAME = METHOD "path" { params } [form F] [requires …]
```

### Name and method

- **`NAME`** is a constructor. A **route value** is the name applied to its
  parameter record: `Index { filter: Done }`.

  A route value is what `href`, `click` and `submit` take, what
  `Response.redirect` and `Response.render` take, and what a test passes to
  `Test.get`.

- **The method** is `GET`, `POST`, `PUT`, `PATCH` or `DELETE`.

- **`form F`** is for write routes only: a GET route has no body to decode.

### The path

The path is a literal with `{name}` segments.

- Each segment must appear in the parameter record.
- Parameters not in the path ride in the query string, for every method:
  `POST "/like" { postId: PostId }` posts to `/like?postId=…`.
- A segment is letters, digits, `.`, `_`, `~`, `-`, or a whole `{param}`.
  Anything else (`//x`, a space, `?`) is E0029.
- A query parameter whose pair does not parse is a `400` naming the
  parameter, never a parameter quietly at its default. That covers a raw `;`
  (write `%3B`) and a bad `%` escape.

#### Paths name resources

A URL names a thing, and the method says what happens to it. `/replies` is
all the replies, `/replies/{id}` is one reply, and `/session` is the visitor's
sign-in. So the same path is read with a `GET`, made with a `POST`, replaced
with a `PUT` and removed with a `DELETE`. The route's name (`Toggle`, `Remove`)
is for your code: `click`, `submit` and `Response.redirect` take it, and it
never appears in the URL. A path with a verb in it (`/replies/delete`) says
the same thing twice, once in the method and once in the path, and gives you
two places to keep in step.

| To | Method and path |
|---|---|
| create a reply | `POST /replies` |
| read one | `GET /replies/{id}` |
| replace one | `PUT /replies/{id}` |
| remove one | `DELETE /replies/{id}` |

The generators (chapter 15) write paths like these. A page's own path names
the resource, and a write route takes that path or, for one row, the path
plus `{id}`.

Some things are not collections. A visitor has one session at a time, so
signing in is `POST /session` and signing out is `DELETE /session`: no id, because
there is only one to name.

When a change is not one of the four, name the part of the thing that
changes. Archiving a reply is not a new verb; it is a sub-resource that a reply
has, or does not have: `PUT /replies/{id}/archived` puts it in the archive and
`DELETE /replies/{id}/archived` takes it out.

Two routes may not share a method and a path (E0043), so when a page has
two writes that would both be `POST` at the page's path, one of them needs a
resource of its own.

Routes you write by hand may use any path the language accepts. This is a
convention that keeps URLs readable, and not a rule the checker enforces.

### Parameters

| Rule | Example |
|---|---|
| Allowed types: `Int`, `Text`, `Bool`, a brand of those, a union with no payloads, `Maybe` or `List` of those | `{ id: Todo.TodoId }` |
| A parameter with a default is optional in links and in the URL | `{ filter: Todo.Filter = All }` |
| A `List` parameter repeats its key; `[]` is no key at all | `{ tag: List Text }` with `["a", "b"]` → `?tag=a&tag=b` |

#### Query keys: `as "Key"`

A query parameter's key is its name, unless the field gives another with
`as "Key"` after its type or default. Use it for URLs whose shape is not
yours to choose, such as a capitalised key (`ElectionId`) no field name can
spell:

```alder fragment
route Results = GET "/r/{cfg}/wood/Results" { cfg: Text, electionId: Text = "" as "ElectionId", contestId: Maybe Int as "ContestId" }
```

The code still says `params.electionId` and
`Sites.Wood.Results { cfg: c, electionId: "17" }`. The request decoder reads
`?ElectionId=17`, and every URL of the route (links, redirects, a test's
`Test.get`) writes `ElectionId=17`.

Rules for the key:

- It is letters, digits, `-`, `_`, `.` and `~`.
- Two parameters may not read one key.
- A `{segment}` parameter takes no `as` (E0011).
- The `as` may start the line after the type or default.

> **`as` inside a default.** Inside parentheses, a list's brackets, and the
> braces of a record, a record update or a block, `as` is an ordinary name:
> `f (g as) as "A"` passes a value named `as` to `g`. Anywhere else (a
> `match` arm and an element's `{…}` included), `as` starts the key; wrap a
> value named `as` in parentheses.

**Hand-written GET forms** name their inputs by the key, not the field:

```html
<form method="get" action="/r/c/wood/Results"><input name="ElectionId" /></form>
```

The browser sends the input's `name` as the key, so `name="electionId"`
would set nothing. `alder check` warns about it (W0008) when the form's
`action` and the input's `name` are literal text, and its fix writes the key.

### `R.Params`

`R.Params` is the parameter record type of route `R`. On a write route with
`form F` it has one more field, `form : F`, which the runtime fills in.

### Shared paths and matching

- Two routes may share a path when their methods differ (`GET "/todos"`
  and `POST "/todos"`).
- They may not share a method and a path. `/x/{id}` and `/x/{slug}` are the
  same pattern, so the second is an error (E0043).
- Patterns that merely overlap are fine, whatever the module order. A
  literal segment is matched before a parameter, left to right, so
  `/todos/new` is served before `/todos/{id}`.
- A request whose path some route declares, but not for its method, is a
  `405` with an `Allow` header.

### `requires`

`requires` names a field of `Session` whose type is `Maybe a`. The route is
served only when that field is `Just`; otherwise the runtime redirects to
the route named in `App.login`.

`requires user, role Author` adds a role: the route is also a 404 for a
signed-in user whose session's `role` is not `Author` ("Rules", below).

Inside the route's members, `session.<field>` has type `a`, not `Maybe a`:

- The route's `Session` is a type of its own (`R.Session` names it).
- It can be passed wherever the app's `Session` is expected (a function in a
  shared module, say), where the field is `Just` its value again.
- The other direction is a type error (E0003): a function of the route that
  takes its `Session` cannot be given the app's, whose field may be
  `Nothing`.

A route value is the only thing `href` accepts, and it must be a GET
route's:

```alder
<a href={Todos.Show.Show { id: todo.id }}>{todo.title}</a>
<a href={Todos.Index.Index { filter: Active }}>Active</a>   -- defaulted fields may be omitted
```

Two route values are `==` when they lead to the same address: a field left
at its default is not in the URL, so `Todos.Index.Index {} == Todos.Index.Index
{ filter: All }`:

```alder eval=True
Todos.Index.Index {} == Todos.Index.Index { filter: All }
```

A GET route may also be asked for by `click`, which fetches it and puts the
answer where the route's root `id` stands ("Delivery", below): its view needs a
root element with an `id`, or nothing is patched. `href` to a
write route, `submit` to a GET route, and `click` to a write route that
declares a `form` (a click sends no fields: use `<form submit={R}>`), are
errors (E0045).

## Qualified declarations

Everything that belongs to a route is written `Route.name`: `Index.load`,
`Index.view`, `Toggle.update`, and the type `Index.Params`. The order of the
declarations in the file is free, and anything unqualified (a `type`, a
`form`, a helper, a component) belongs to the module. The signatures are
fixed, and, as everywhere in Alder, written out:

| declaration | on | type |
|---|---|---|
| `R.load` | a GET route | `R.Params -> Session -> Query m` |
| `R.view` | a GET route | `m -> Session -> Html` |
| `R.update` | a write route | `R.Params -> Session -> Task Response` |

`m` is one type: whatever `load` returns, and what `view` takes. The checker
ties the two together. Name it (`type Model = …`), use a table's row type
(`Todo`, as `Row` does above), or write the record type inline. A `view` may
return `File` instead of `Html` ("File pages", below). A write route has no
`view`, because it answers with a `Response`, which names the GET route to
show.

`Session` in the signature is the route's own session, narrowed by its
`requires`. Where a route's `Session` is named from outside its members (a
helper that takes it), it is `R.Session`.

A module with one GET route is a page file without a `Model` slot, which is
most files. A module with a `route` line is a route module. The old form, a
module with a `page` line and its messages, is E0046: write the module as a route
module by hand, as this chapter shows.

## Names

A route's name is a namespace inside its module: `Row.view`, `Row.load`,
`Row.Params`. Inside the module you write `Row.view todo session`. From
another module you reach the route through its module:

```alder fragment
import Todos.Index

Index.Row.view todo session         -- a member: needs the import, named by its qualifier
Todos.Index.Row { id: todo.id }     -- the route value: no import needed, the full path
```

An imported module is named by its qualifier (chapter 3), so the member is
`Index.Row.view`; the full path in a member's name is E0205. A route value is
global and keeps its full path.

A dotted name is read as the longest module path the program knows, then a
route of that module, then a member. That is why a route's module and its
name often repeat: the to-do list is the module `Todos.Index`, its page is
the route `Index`, and the page's value is **`Todos.Index.Index {}`**. Read
it as module, route, parameters.

The login page is `Auth.Login.Login {}`, and `App` names it with
`login = Auth.Login.Login`. A route that shares its module's last name is the
usual one-route file; nothing shortens it.

Two things would make a name ambiguous, and both are E0044:

- a module `Todos.Row` beside a route `Row` in the module `Todos`: `Todos.Row.view`
  could be either;
- a route named like something already in scope in its module: an import's
  qualifier (`route Todo` beside `import Table.Todo`, because `Todo.view` would
  name the table module), a standard-library module (`route List`), a
  constructor or a table.

```text
error[E0044]: named like the qualifier of `import Table.Todo`
```

Rename the route, or the module.

## The state rule

A request carries no memory of the last one. The server keeps nothing
between requests for you: a value you need later lives in one of four homes,
and which home decides what survives.

| home | survives | for |
|---|---|---|
| the URL (route parameters) | refresh, back, bookmark, share | filter, sort, page, wizard step |
| the HTML (form values, button payloads) | the next interaction only | drafts in flight, "the thing this button acts on" |
| the session | refresh, not sharing | identity, secrets, theme |
| the database | everything | durable facts |

The rule is: **If you would want it back after a refresh, it goes in the
URL. If it is secret, it goes in the session. Everything else rides in the
HTML or is a row.**

So the to-do list's filter is a parameter of `Index`
(`href={Index { filter: Done }}` is a link, and Back works); a half-typed
title is in the form until the form is sent; who you are is in the session;
the todos are rows. There is no fifth place: no model that the server holds
for a tab, and none that `update` hands to `view`. An `update` changes data,
and a GET route renders data.

## `Response`

A write route's `update` is `Params -> Session -> Task Response`. `Response`
is a type of the standard library, built with functions named after the HTTP
they produce (chapter 9 lists them with the rest):

```alder fragment
Response.redirect : Route -> Response                 -- 303, to the route's URL
Response.render   : Route -> Response                 -- 200, the route's view, as one element
Response.refresh  : Response                          -- 200, the page the browser is on, again
Response.invalid  : Text -> Response                  -- 422, the page, form kept, error shown
Response.toast    : Text -> Response -> Response      -- a toast on any response
Response.success  : Text -> Response -> Response      -- a toast of kind success
Response.warning  : Text -> Response -> Response
Response.error    : Text -> Response -> Response
```

The status code is the signal, as it is for a browser: a `3xx` is followed,
`422` is the same page again with a complaint, `200` is content. A response
can be used with `|>`: `Response.redirect (Index {}) |> Response.toast "Saved"`.

**`redirect r`** goes to the GET route `r`. After creating something, use it,
so that a browser refresh does not send the form twice. It goes to `r` even if
`r` is the page the browser is on.

**`render r`** shows the GET route `r` again, in place: the runtime runs
`r`'s `load`, renders its `view`, and sends that one element, which replaces the
element with the same `id` on the page (so that view needs a root element with
an `id`, or nothing is patched). Nothing else on the page is loaded or
sent.

**`refresh`** loads the page the browser is on again, and sends the regions of
it that changed.

**`invalid t`** refuses a well-formed submission for a reason only `update`
can know (wrong password, name already taken). It is the page the browser is
on again, with the values that were posted, and `t` shown in the form's
`<Errors form />`.

The rule a developer needs, in one sentence: **`render` is for something
already on the page; `refresh` is for things that appear or disappear, or when
more than one place changes.**

The reason is cost. `render` costs the `load` of the route it shows (one read
by key, for a row) and sends one element. `refresh` costs what loading the
page costs (twice: the runtime renders the page before the `update`, to know
what changed) and sends the regions that differ.

A toggle edits a row that is there: `render`. A delete removes one, and the
count in the page's footer changes too: `refresh`.

`invalid` is a successful result, not a failure: the `Task` that produced it
commits, so anything it wrote and any `Session.put` it ran are kept. To
refuse without changing anything, decide before writing (`let taken <-` a
query, then `invalid` or the writes). Only a failed `Task` (`Task.fail`,
`Task.notFound`, a `@unique` violation, a capability error) undoes its
writes and its session changes (chapter 7).

`toast` adds a toast to any of the four ("Notices and in-flight state",
below). The other `Response` builders a web framework has (`status`,
`header`, `json`, `file`, `remove`, `patch`, `stream`) are not in Alder yet;
the type is shaped for them, and a file answer is a GET route that returns a
`File` ("File pages", below).

## `form`

```alder
type Priority = Low | Normal | High

form NewTodo = {
  title: Text.NonEmpty,
  due: Maybe Date,
  priority: Priority = Normal,
}
```

Field types must be one of: `Text`, `Int`, `Float`, `Bool`, `Date`, a brand
of those, a union with no payloads, or `Maybe` of those. A field with a
default is optional in the request. A `Bool` field is a checkbox and is
`False` when absent.

A write route that declares `form NewTodo` gets the decoded form as
`params.form`. Here is a page to add a todo, with its two routes: the page
is at `/todos/new`, and the form posts to `/todos`, the address of the todos
themselves:

```alder
module Todos.New

import Layout
import Table.Todo

route New    = GET  "/todos/new" { }              requires user
route Create = POST "/todos"     { } form NewTodo requires user

form NewTodo = { title: Text.NonEmpty, due: Maybe Date }

New.load : New.Params -> Session -> Query Unit
New.load params session = Query.succeed ()

New.view : Unit -> Session -> Html
New.view model session =
  <Layout.Page title="New todo">
    <form submit={Create}>
      <al-input field="title" label="Title" />
      <al-input field="due" label="Due" />
      <Errors form />
      <al-button type="submit">Add</al-button>
    </form>
  </Layout.Page>

Create.update : Create.Params -> Session -> Task Response
Create.update p session = {
  let same <- Db.mine Todo |> Db.where (Todo.title.eq p.form.title) |> Db.count
  if same > 0 then
    Task.succeed (Response.invalid "You have a todo with that title.")
  else {
    let _ <- Db.insertMine Todo { title: p.form.title, due: p.form.due }
    Task.succeed (Response.redirect (Todos.Index.Index {}) |> Response.toast "Added")
  }
}
```

There are two ways for this form to fail, and they look the same to the
user.

When a submitted form fails to decode (missing field, failed refinement,
wrong type), the runtime does **not** call `update`. It answers `422` with
the page the browser is on, rendered again with the submitted values and
errors attached to that form; `<Input>` and `<Errors>` show them. The program
never sees an invalid form.

Only the form that posted shows them: on a page with one form per row, the
other rows keep their own values.

A field's errors go to its `<Errors field="…" />`; the errors of a field
without one are listed in the form's `<Errors form />`, after its name ("due
must be a date (YYYY-MM-DD)"). A form with neither for a field gets warning
W0003.

When the form decodes and `update` still refuses it, `update` answers
`Response.invalid "…"`: the same `422`, the same page, with the text in
`<Errors form />`. A form that can be refused this way and has no
`<Errors form />` gets warning W0005, because the user would see the page come
back unchanged and not know why.

## App-level routes

Actions that belong to no page (log out, switch theme) are write routes
declared in the `App` module. Any page can send them, as
`click={App.Logout}`, and a route of `App` is like any other route except
that it has no page of its own.

It answers `redirect r`, to go to the route `r`, or `refresh`, which reloads
the page the browser is on, because that is where the request came from (the
page's address is sent with every write, "Delivery", below).

```alder fragment
-- in App
route Logout = DELETE "/session" { }

Logout.update : Logout.Params -> Session -> Task Response
Logout.update p session = {
  let _ <- Session.put (\s -> { s | user: Nothing })
  Task.succeed (Response.redirect (Auth.Login.Login {}))
}

-- in any page, or any component
<button click={App.Logout}>Log out</button>
```

An `App` route's value can be written in any markup, so a layout component
typed `Html` can contain `click={App.Logout}`.

## Patterns

`href`, `click` and `submit` request a route when the user follows a link,
presses a button or sends a form. Four common interactions request a route
without any of those:

- search as you type;
- loading more rows as the list scrolls;
- a slow part of the page filled in after the page shows;
- a badge that polls.

Each one asks for a GET route and patches its view into the page by `id`,
as a `click` on a GET route does. Only the moment differs, so each moment
has an attribute:

| attribute | on | fires |
|---|---|---|
| `load={R}` | any element | once, when the element appears: on page load, or when a patch brings it in |
| `load={R} every="5s"` | any element | when it appears, then every 5 s while it is on the page |
| `intersect={R}` | any element | once, when the element first scrolls into view |
| `input={R}` | `<input>`, `<textarea>`, `<select>`, or `<Input>`, `<Textarea>`, `<Select>` in a form | 300 ms after the user stops typing |

The rules are the same for all four:

- `R` is a GET route, as for `href`. A write route is E0045: a timer, a
  scroll or a keystroke must not write. `load` and `intersect` accept a
  computed route, as `href` does; only `input` needs its route written in
  place, so the checker can read its query key.
- Custom elements (`<al-*>`) take none of the four attributes. Wrap one in
  a `<div>` and put the attribute on the `div`.
- The answer is `R`'s view, and it replaces the element whose `id` is the
  view's root `id`. So the page must have an element with that `id`: the
  trigger itself (the slow widget, the badge, the scroll sentinel), or
  another element (the search results).
- An element has at most one of `load`, `intersect` and `input`, and
  `every` goes only beside `load`. `click` and `href` may sit beside any
  of them. Breaking either rule is E0047.
- These need JavaScript. Without it nothing is requested, and the element
  shows what the server rendered, so render something useful there.

None of the four makes the page look busy. A button pressed with `click` is
made `inert` until its answer arrives, so it cannot be sent twice. These
requests are background work: a search field would freeze under the user's
fingers after every keystroke.

### Search as you type

The field asks for `Results` as the user types. `Results`'s view is a
`<ul id="results">`, and the page has an empty one for it to replace:

```alder
module Search

import Table.Todo
import Table.User

route Find    = GET "/find"         { q: Text = "" } requires user
route Results = GET "/find/results" { q: Text = "" } requires user

Find.load : Find.Params -> Session -> Query Text
Find.load params session = Query.succeed params.q

Find.view : Text -> Session -> Html
Find.view q session =
  <main>
    <input name="q" value={q} input={Results { q: q }} />
    <ul id="results"></ul>
  </main>

Results.load : Results.Params -> Session -> Query (List Todo)
Results.load params session = Db.mine Todo |> Db.where (Todo.title.contains params.q) |> Db.all

Results.view : List Todo -> Session -> Html
Results.view todos session =
  <ul id="results">{for t in todos -> <li>{t.title}</li>}</ul>

test "typing asks for the matching todos" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Db.insert Todo { title: Text.NonEmpty "Milk", due: Nothing, owner: me }
  let _ <- Db.insert Todo { title: Text.NonEmpty "Bread", due: Nothing, owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Find { q: "" })
  let r <- Test.input (Results { q: "mil" })
  Task.succeed (Test.texts "#results li" r.doc == ["Milk"])
}
```

The server renders a complete URL for the field (`/find/results`, or
`/find/results?q=mil` when the page was opened with a query). In the
browser, the request sets one query key to what the field holds and leaves
the rest of the URL alone. That key is the field's `name`, here `q`, or its
`field` on `<Input>`, `<Textarea>` and `<Select>`. This is what the field
renders:

```html
<input name="q" value="" data-on:input__debounce.300ms="@get(window._alderQ(&#39;/find/results&#39;, &#39;q&#39;, el.value), window._alderF?.())" />
```

`window._alderQ` sets that one key with the browser's own URL encoding, so
`&`, `#`, `+`, spaces and accented letters reach the route as typed.

Because the browser chooses the value, the checker makes sure the route can
take it. Each of these is E0047:

- `input` on an element that is not a field;
- a plain `<input>`, `<textarea>` or `<select>` with no literal `name`
  (an `<Input>`, `<Textarea>` or `<Select>` with no literal `field` is
  E0304 instead);
- a route with no query parameter of that name (after `as "Key"`);
- a parameter that is not `Text`.

A path parameter does not count: the browser can set a query key, but it
never rewrites a path.

The request waits until the user has stopped typing for 300 ms. If an
answer is slow and the user types again, the new request cancels the old
one, so an old answer never replaces a newer one.

### Infinite scroll

The page shows its first rows and, after them, an empty element: the
sentinel. When the sentinel scrolls into view it asks for `More`, whose view
is the next rows and the next sentinel, under the old sentinel's `id`:

```alder
module Feed

route Feed = GET "/feed"      { }
route More = GET "/feed/more" { after: Int }

total : Int
total = 25

-- The last row of the page that starts after `after`: ten rows a page.
pageEnd : Int -> Int
pageEnd after = if after + 10 > total then total else after + 10

row : Int -> Html
row n = <li>Item {Text.fromInt n}</li>

-- The element that asks for the rows after `next`; none after the last row.
sentinel : Int -> Html
sentinel next =
  if next < total then <div id={"more-" ++ Text.fromInt next} intersect={More { after: next }}></div>
  else Html.none

Feed.load : Feed.Params -> Session -> Query Int
Feed.load params session = Query.succeed 0

Feed.view : Int -> Session -> Html
Feed.view start session =
  <main>
    <ul>{for n in List.range 1 (pageEnd start) -> row n}</ul>
    {sentinel (pageEnd start)}
  </main>

More.load : More.Params -> Session -> Query Int
More.load params session = Query.succeed params.after

More.view : Int -> Session -> Html
More.view after session =
  <div id={"more-" ++ Text.fromInt after}>
    <ul>{for n in List.range (after + 1) (pageEnd after) -> row n}</ul>
    {sentinel (pageEnd after)}
  </div>

test "scrolling loads two pages, then stops" = {
  let _ <- Test.get (Feed {})
  let _ <- Test.intersect (More { after: 10 })
  let r <- Test.intersect (More { after: 20 })
  Task.succeed ({ rows: Test.count "li" r.doc, sentinel: Test.has "[data-on-intersect__once]" r.doc } == { rows: 25, sentinel: False })
}
```

The first page ends with `<div id="more-10" intersect={More { after: 10 }}>`.
`More { after: 10 }` answers a `<div id="more-10">` holding rows 11 to 20
and the sentinel `more-20`, so the empty sentinel becomes rows plus a new
sentinel. Every `id` carries its cursor, so none repeats. Each page nests
one level deeper, which does no harm.

The last page renders no sentinel, so scrolling stops asking. A sentinel
fires only once, even when it stays in view.

### A slow widget and a polling badge

`load` fills an element in once the page has shown, so a slow query does
not hold the page up. With `every` beside it, the element is loaded again
on a timer:

```alder
module Dashboard

import Table.Todo
import Table.User

route Dashboard = GET "/dashboard"        { } requires user
route Recent    = GET "/dashboard/recent" { } requires user
route Open      = GET "/dashboard/open"   { } requires user

Dashboard.load : Dashboard.Params -> Session -> Query Int
Dashboard.load params session = Query.succeed 0

Dashboard.view : Int -> Session -> Html
Dashboard.view n session =
  <main>
    <h1>Today</h1>
    <section id="recent" load={Recent {}}>Loading…</section>
    <p>Open: <span id="open" load={Open {}} every="30s">…</span></p>
  </main>

Recent.load : Recent.Params -> Session -> Query (List Todo)
Recent.load params session = Db.mine Todo |> Db.orderBy Todo.created.desc |> Db.limit 5 |> Db.all

Recent.view : List Todo -> Session -> Html
Recent.view todos session =
  <section id="recent">{for t in todos -> <p>{t.title}</p>}</section>

Open.load : Open.Params -> Session -> Query Int
Open.load params session = Db.mine Todo |> Db.where (Todo.done.eq False) |> Db.count

-- The badge renders its own `load` and `every` again: the timer keeps running.
Open.view : Int -> Session -> Html
Open.view n session = <span id="open" load={Open {}} every="30s">{Text.fromInt n}</span>

test "the open count follows the todos" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Dashboard {})
  let before <- Test.load (Open {})
  let _ <- Db.insert Todo { title: Text.NonEmpty "Milk", due: Nothing, owner: me }
  let after <- Test.load (Open {})
  Task.succeed ({ before: Test.text "#open" before.doc, after: Test.text "#open" after.doc } == { before: Just "0", after: Just "1" })
}
```

The section first shows "Loading…", then the five newest todos. The badge
asks for `Open` when the page shows, then every 30 seconds.

`every` is literal text: a whole number above zero and a unit, `ms`, `s` or
`m` (`every="500ms"`, `every="5s"`, `every="2m"`). Anything else, a
computed value, `"0s"` or a missing unit, is E0047. Minutes render as
seconds (`every="2m"` polls every 120 s), because the browser's timer reads
only `ms` and `s`. This is what the badge renders:

```html
<span id="open" data-init="@get(&#39;/dashboard/open&#39;, window._alderF?.())" data-on-interval__duration.30s="@get(&#39;/dashboard/open&#39;, window._alderF?.())">…</span>
```

`Open.view` writes the same `load` and `every` on its root. The patch keeps
the element and its attributes, so the timer keeps running and the badge
costs one request per tick. A view that left them out would remove them
with the patch, and the badge would stop after its first answer.

### Testing them

`alder test` fires nothing by itself: a `Test.get` holds the page as the
server sent it. A test says what the user did. `Test.load r`,
`Test.intersect r` and `Test.input r` find the element that requests `r` on
the page held, and fetch `r` as the browser would. Polling is `Test.load`
called again, as `Dashboard`'s test does (chapter 10, "The browser a test
holds").

## File pages

Every route so far answers with a web page, or a piece of one. Some
addresses should answer with something else: a spreadsheet to download, a
zip, a JSON document, a copy of another site's page kept exactly as it was. A
**file page** is a GET route whose `view` returns a `File` instead of `Html`.
It has an address, a `load` and parameters like any GET route, and the router
sends the file's bytes as the whole answer.

### Declaring a file page

A `File` carries a content type (`text/csv`), a status (`200`, `404`, `503`), a body and extra headers.

The compiler decides a route is a file page by the type of its `view`. Here
is one that downloads the signed-in user's todos as a CSV file:

```alder
module Todos.Export

import Table.Todo

route Export = GET "/todos.csv" { } requires user

type Model = { todos: List Todo }

Export.load : Export.Params -> Session -> Query Model
Export.load params session = {
  let todos <-
    Db.mine Todo
      |> Db.orderBy Todo.created.desc
      |> Db.all
  Query.succeed { todos: todos }
}

row : Todo -> Text
row todo = Text.join "," [todo.title, if todo.done then "done" else "open"]

Export.view : Model -> Session -> File
Export.view model session =
  File.text { contentType: "text/csv", body: Text.join "\n" (List.map row model.todos) }
    |> File.withHeader { name: "Content-Disposition", value: "attachment; filename=todos.csv" }
```

`File.withHeader` sets a header: `Content-Disposition: attachment; filename=todos.csv` saves the answer
(`File.text` etc.: chapter 9).

Path segments are whole: `"/reports/{id}/report.csv"` is valid, `"/reports/{id}.csv"` is not.

`File.html` answers a web page byte for byte. Markup in a `view` has one root element and
no doctype, so to start with `<!doctype html>` use `Html.fragment [Html.unsafe "<!doctype html>\n", <html>…</html>]`.

`File.status`, `File.contentType` and `File.body` read a file back; they
are for tests and helpers, since the router is the one that sends it.

## Rules

Most apps with a login have two rules about who may do what. A row
belongs to a user, and only that user may see or change it: a todo, a
draft post. And some routes are only for users with a role: an author's
desk, an admin page. Alder lets you **declare** both rules, and then the
shortest thing to write is the correct one: no check to remember, so no
check to forget.

### Ownership: `@owner`

A table says which of its columns names the row's owner. In the to-do
app (`examples/todos/app/Table/Todo.alder`):

```alder fragment
table Todo {
  id: TodoId @id,
  title: Text.NonEmpty,
  done: Bool = False,
  due: Maybe Date,
  owner: User.UserId @ref User @owner,
  created: Time @created,
}
```

`owner: User.UserId @ref User` was already there: each todo refers to a
user. `@owner` adds one thing: "this column is whose row it is". The
rules for it:

- A table has at most one `@owner` column.
- It must be a `@ref` to the user table, of the type of the session's
  `user` field without its `Maybe` (`session { user: Maybe User.UserId }`
  means `User.UserId`), so the column and the session can be compared.
- The app's session must have a `user` field.

Anything else is E0024 (the exact texts are in chapter 11). `@owner`
changes nothing in the database: the column and its
reference are what they were, and no migration is needed (chapter 8).

### Reading and writing your own rows

A table with an `@owner` column has five **scoped** operations in `Db`
(chapter 9). Each one does what its unscoped twin does, but only to rows
whose owner is the signed-in user:

| Operation | Type | What it does |
|---|---|---|
| `Db.mine Todo` | `Select Todo` | The rows whose `owner` is the signed-in user |
| `Db.getMine Todo id` | `Query Todo` | The row with that id, when it is the user's; otherwise not found |
| `Db.updateMine Todo id f` | `Task Unit` | Changes the row with `f`, when it is the user's; otherwise not found |
| `Db.deleteMine Todo id` | `Task Unit` | Deletes the row, when it is the user's; otherwise not found |
| `Db.insertMine Todo { … }` | `Task TodoId` | Inserts a row whose owner is the user |

`Db.mine` is a `Select`, like `Db.from Todo`, so `Db.where`, `Db.orderBy`,
`Db.limit`, `Db.all`, `Db.count` and the rest apply to it.

The to-do list uses four of them, and `Todos.Show` reads its todo with the fifth
(`examples/todos/app/Todos/Index.alder` and `Todos/Show.alder`, in the example app below).

Why a write route needs them as much as a `load` does: `load` decides what
the page shows, but a write route carries its own parameters. `Toggle { id }`
puts the id in the request's URL (`/todos/…`), and anyone signed in can
send that request with an id that is not theirs. No `load` runs before a write
route to have checked it. With `Db.updateMine` the id is only ever one of the
user's own todos.

Two details:

**`Db.insertMine` takes its row as a record literal**, written in the
call, with every field but the owner: the owner is the session's user.
Naming it is an error:

```text
error[E0003]: `owner` is set by `Db.insertMine`
```

and so is passing a row built elsewhere: "`Db.insertMine` takes its row
as a record literal: write the record literal in the `Db.insertMine`
call (its row is `Insert Todo` without `owner`)". Fields with a default
(`done`) may be left out, as with `Db.insert`.

**`Db.updateMine` never changes the owner.** If `f` returns the row with
another `owner`, the rest of its changes are written and the owner stays
as it was: a scoped write cannot give a row away. A page that transfers
rows writes that with `Db.update` ("When to use the unscoped forms",
below).

### What a stranger gets

A row that exists but belongs to someone else is answered exactly as a
row that does not exist: **not found**.

- In `load`, `Db.getMine` of another user's id is `Query.notFound`: the
  page is a 404, `App.notFound`.
- In `update`, `Db.updateMine` or `Db.deleteMine` of another user's id
  is `Task.notFound`. The whole `Task` fails, so everything it did before
  is rolled back too, writes and `Session.put` alike (chapter 7), and the
  answer is a 404.
- `Db.mine` simply has no such row in it.

There is no arm to write for the stranger's case and so no arm to get
wrong, and a stranger cannot tell someone else's todo from an id that
was never used: the answer gives nothing away. A page that wants to
answer differently (a 403 page, say) uses the unscoped operation and
writes its own check.

### Where they may be used

A scoped operation needs a signed-in user, so it is allowed only where
the compiler knows there is one: in the members of a route that `requires
user`, its `load`, `view` or `update`, and in the helpers they call, such as

```alder fragment
recent : Select Todo
recent = Db.mine Todo |> Db.orderBy Todo.created.desc |> Db.limit 5
```

Anywhere else, it is E0026: in a route without `requires user`,

```text
error[E0026]: `Db.mine` needs a route that `requires user`: `Index` does not
```

The rule follows a helper: a value that uses a scoped operation is checked
where it is used, not where it is written, so a route that does not `requires
user` and calls `recent` is E0026 at that call ("`recent` uses `Db.mine`,
which needs a route that `requires user`: `Index` does not").

A `fake` (chapter 10) has no session either: "`Db.mine` cannot be used in a
fake: a fake has no session" (an excerpt: the message begins with where the
use is, such as "in `fake Counter`'s `count`:"). Tests may use them (below).

A table without `@owner` has nothing to scope by (E0025):

```text
error[E0025]: `User` has no `@owner` column, so `Db.mine User` has nothing to scope by
```

### Whose session

A scoped operation acts for the user of the **running** request or
test, the session the runtime holds while it runs the effect. It does
not read the `session` argument of `load` or `update`. In a served
request the two are the same session, so the difference never shows.
It shows in two places:

- **A test** that calls `update` (or `load`) itself, with a session
  record it wrote, does not choose the user that way: the scoped
  operations act for the test's own session, which `Test.login` sets
  (chapter 10, "Tests that use the rules").

  A test that never logged in has no user, and a scoped operation fails with
  `Db.mine Todo: no signed-in user` (or `Db.getMine Todo: …`, and so on).
  It never quietly returns nothing.

- **`Session.put`** in the middle of an `update` changes the user for
  every scoped operation after it in the same `Task`: they act for the
  session as it now is.

### Roles: `requires user, role R`

A role lives in the session, beside the user. The blog
(`examples/blog`) has two, declared in `User`:

```alder fragment
type Role = Reader | Author
```

The user table has a column `role: Role = Reader`, and `App`'s session
has a field for it:

```alder fragment
session { user: Maybe User.UserId, role: User.Role = Reader }
```

The login page copies the role from the user's row when it signs them
in (`examples/blog/app/Auth/Login.alder`):

```alder fragment
Just id -> {
  let found <- Db.get User id
  let role = Maybe.withDefault Reader (Maybe.map (\u -> u.role) found)
  let _ <- Session.put (\s -> { s | user: Just id, role: role })
  Task.succeed (Response.redirect (Posts.Index.Index {}))
},
```

and logging out puts it back: `{ s | user: Nothing, role: Reader }`.

Adding `role` to the session changes the tests. A session record a test
writes for a route's `load` or `update` is a record literal, and names every
field: `{ user: author }` becomes `{ user: author, role: Author }`, or
E0003 says `role` is missing (a default applies at the boundaries, not
to a literal, as the theme switch earlier in this chapter shows).

`Test.login`'s record is a boundary, the cookie's value, so
`Test.login { user: Just me }` may leave `role` out and gets its default,
`Reader`.

A route for authors only says so on its `route` line:

```alder fragment
route Write  = GET "/write"        { } requires user, role Author
route Drafts = GET "/write/drafts" { } requires user, role Author | Admin
```

- `requires user` comes first: a visitor is sent to log in, as always.
- Then the role: a signed-in user whose session's `role` is not one of
  those named gets a 404, the same `App.notFound` page as a missing row,
  so a Reader cannot even tell the route exists. This is checked before
  the route's `load` or `update` runs, so neither needs a check of its own.
- `role Author | Admin` lets either in.
- Inside the route's members, `session.role` has the union's type, as
  everywhere.
- The router checks `requires` and the role, whatever the method. A test of
  them goes through the browser (`Test.open`, `Test.click`, chapter 10); a
  direct `load` or `update` call is not checked.

The rules for the clause (E0027): `role` needs `user` first (`requires
role Author` alone is an error); the session must have a field `role`
(else: "this session has no `role` field"); it must be a union, not
`Maybe`; and each name must be one of its constructors. In the to-do
app, whose roles are `Member` and `Admin`:

```text
error[E0027]: `Author` is not a constructor of this session's `role` type
```

**A role is read at login.** The check reads the session cookie and
nothing else, so it costs no query.

A role in the cookie lasts until logout, or the session's 30-day lifetime from
login (visiting does not renew the cookie: "The session cookie", below). Until
then an author demoted to Reader still passes `requires user, role Author`,
and a Reader promoted to Author must log in again.

A route where a demotion must apply at once also reads the user's row, the
**re-read idiom**, as the blog's Write routes do with helpers in `User`
(`examples/blog/app/Table/User.alder`):

```alder fragment
isAuthor : UserId -> Query Bool
isAuthor id = {
  let me <- Db.get User id
  Query.succeed (Maybe.map (\u -> u.role) me == Just Author)
}

authorOnly : UserId -> Query Unit
authorOnly id = {
  let author <- isAuthor id
  if author then Query.succeed () else Query.notFound
}
```

A GET route's `load` calls `isAuthor` first, answering `Query.notFound`
when it is `False`; a write route, which runs no `load`, begins with `authorOnly`.

That is the blog's test "a demoted author is turned away at once"
(chapter 14). The role clause stays: it turns away a Reader promoted
since their login, and keeps a Reader's request from reaching a `load`
or an `update` at all.

### When to use the unscoped forms

`Db.from`, `Db.get`, `Db.update`, `Db.delete` and `Db.insert` are still
there, and still right in these places:

- **Rows that are anyone's to read.** The blog's front page lists
  everyone's published posts with `Db.from Post`.
- **Routes for a role that acts on everyone's rows**, such as an admin
  route `requires user, role Admin` that deletes any post. The role is
  the rule there, and the route says so on its `route` line.
- **Giving a row away**, which `Db.updateMine` never does.

On an owned table outside those cases, an unscoped operation means the
check is yours to write again, and a test is yours to write for it.

A row a page has already shown is not one of the cases either:
write the scoped form; it costs nothing (a write route takes its id from its path
and runs no `load`, so the scoped write is what makes the id safe: `Todos.Show`
writes `Db.updateMine Todo p.id …`).

### Partitions

`@partition k` generalises `@owner` (which is `@partition user`) to any session
key, here `org` (`examples/workspace`):

```alder fragment
session { user: Maybe User.UserId, org: Maybe Org.OrgId via Membership, role: User.Role = Member }
choose org = Orgs.Index
table Project { id: ProjectId @id, name: Text.NonEmpty, org: Org.OrgId @ref Org @partition org }
```

- The column is one `@ref` of the key's type without `Maybe`, and the session has the key
  (E0032).
- The generic scoped operations `Db.scoped T`, `Db.getScoped T id`, `Db.updateScoped T id f`
  (never changes `org`), `Db.deleteScoped T id` and `Db.insertScoped T { … }` (a literal
  without `org`) keep to the session's org; another org's row is not found. `Db.mine` on an
  org table is E0025 ("… write `Db.scoped Project`").
- They need a route that `requires user, org` (E0026); a `via` key comes after `user`
  (E0027). There `session.org : Org.OrgId`. A visitor is sent to log in, a user without an
  org to `choose org`'s page (required for a `via` key, E0032; that page must not require
  the key); a key with no `choose` answers 404.
- `via Membership`: a `Session.put` of `org` to `Just v` fails the `Task` ("Session.put org:
  no Membership for this user and org") unless a `Membership` row of the session's user
  and `v` exists then, inside the `Task`'s transaction (a row inserted earlier in the `Task`
  counts; one deleted counts as gone). `Nothing` is always allowed, and a session that loses
  its user loses its org. The membership table has exactly one `@ref` to the user table and
  one to the key's (E0032); one `via` field per session. Picking an org:

```alder fragment
Open.update p session = {
  let _ <- Session.put (\s -> { s | org: Just p.org })
  Task.succeed (Response.redirect (Projects.Index.Index {}))
}
```

  A crafted `Open` of an org the user is no member of is refused; a test calls `update`
  directly to send it.
- The cookie keeps `org`; it is not checked again per request. A route that must notice a
  removed member at once re-reads the membership (the re-read idiom):

```alder fragment
isMember : User.UserId -> Org.OrgId -> Query Bool
isMember user org = {
  let n <- Db.from Membership |> Db.where (Membership.user.eq user) |> Db.where (Membership.org.eq org) |> Db.count
  Query.succeed (n > 0)
}
```

  `load` begins `let member <- Membership.isMember session.user session.org` and answers
  `Query.notFound` when it is `False`; a write route, which runs no `load`, begins with
  `Membership.memberOnly session.user session.org`.
- The admin form `requires user, role Admin, silo org` takes `org` from the route's
  parameter `{ org: Org.OrgId, … }`, guarded by the role, not `via` (E0033).
- Not in a fake or `App` (E0026); an org's name stays unscoped (`Db.get Org session.org`).
- With `silo org { Project, Todo }` (chapter 8, "Silos") each org's rows are in its own
  database, and a `Task` writes to one store (E0031).

## What the patterns cost

What each of these costs and how it is delivered follows from the routes it
uses. Each is a few lines of routes and uses nothing else:

| pattern | routes | delivery | loads |
|---|---|---|---|
| list page | `Index = GET` | document, or the body morph for a link | 1 |
| filter, sort, tab, page number | none: `href={Index { filter: Done }}` | a boosted link, the URL moves | 1 |
| create with a form | `Create = POST … form` answering `redirect (Index {}) \|> toast "Saved"` | the body morph and a toast; `303` without JavaScript | 1 |
| refuse a form | `Response.invalid "name taken"` | `422`, the form kept, the error shown | before-view and after |
| toggle a row | `Toggle = PATCH` answering `render (Row { id })` | one `<li>`, by its `id` | 1 row |
| delete a row | `Delete = DELETE` answering `refresh` | `#todos` patched | 2 |
| add a comment | `Comment = POST … form` answering `refresh` | `#comments` patched | 2 |
| a like and a count elsewhere | `Like = POST` answering `refresh` | both regions | 2 |
| inline edit | `EditRow = GET`, asked for by `click`; `Save = PUT` answering `render (Row …)`; cancel is `click={Row …}` | the row becomes a form and back, in place | 1 row each |
| modal form | `NewDialog = GET`, asked for by `click` into `#dialog`; `Create` answering `redirect (Index {})` | the dialog, then the page | 1 |
| wizard | `Step2 = GET { name }`; `Next1 = POST` answering `redirect (Step2 { name: … })` | one URL per step; Back works | 1 |
| login, logout, theme | a `POST` that runs `Session.put` and answers `redirect` or `refresh` | as above | 1 or 2 |
| CSV export | `Export = GET "/export.csv"` whose view returns a `File` | the file ("File pages", above) | 1 |
| search as you type | `Results = GET { q }`, asked for by `input=` | `#results`, by its `id` | 1 per pause in typing |
| infinite scroll | `More = GET { after }`, asked for by `intersect=` | the sentinel becomes rows and the next sentinel | 1 per page |
| a slow widget | `Recent = GET`, asked for by `load=` | its element, after the page shows | 1 |
| a polling badge | `Open = GET`, asked for by `load=` with `every=` | its element | 1 per tick |

The last four request a route without a click, and need JavaScript
("Patterns", above). "Loads" counts the `load`s of GET routes a click (or
one of those triggers) costs; `refresh` loads the
page twice, once for the before-view.

## The request lifecycle

```
GET  /path
  1. decode route parameters      → 404 on failure
  2. check `requires`             → redirect to App.login on failure
     check `role R`, if declared  → 404 when the session's role is not R
  3. load                         → Query, run by the runtime: 404 on
                                    Query.notFound, 303 on Query.redirect
  4. view (with the session)      → HTML (a File, for a file page)

POST, PUT, PATCH, DELETE (the route's own path)
  0. a POST carrying `_method` is routed by it; a method no route
     declares for the path → 405 with `Allow`
  1. decode the route's parameters (path and query) → 404 on failure
  2. check `requires`             → redirect to App.login on failure
     check `role R`, if declared  → 404 when the session's role is not R
  3. decode the form, if any → on failure: 422, the page the browser is
     on rendered again with the errors, and stop (update is not called)
  4. render the page the browser is on, if update can answer
     `refresh` or `invalid` (the before-view)
  5. update                       → Task, run by the runtime (no `load`)
  6. redirect: 303 (under Datastar: the target page, morphed in, and a
     history push; to another site, an event that navigates)
     render: the route's load and view, one element patched in
     refresh: the shown page loaded again, the regions that differ patched
     invalid: the shown page with the text on the form, 422
     Task failure: App.error → HTML, status 500 (404 for Query.notFound
     and Task.notFound, 303 for Query.redirect)
```

## The `App` module

Every app has an `App` module:

```alder
module App

import Table.User
import Layout

session { user: Maybe User.UserId, role: User.Role = Member }

login = Auth.Login.Login

route Logout = DELETE "/session" { }

Logout.update : Logout.Params -> Session -> Task Response
Logout.update p session = {
  let _ <- Session.put (\s -> { s | user: Nothing, role: Member })
  Task.succeed (Response.redirect (Auth.Login.Login {}))
}

error : Error -> Html
error err =
  <Layout.Page title="Error">
    <h1>Something went wrong</h1>
    <p>Please try again.</p>
  </Layout.Page>

notFound : Html
notFound =
  <Layout.Page title="Not found">
    <h1>Not found</h1>
  </Layout.Page>
```

- `session` declares the session record. The runtime stores it in a signed
  cookie. Field types follow the same rules as form fields. `Session.put`
  (chapter 7) changes it; a `Task` that fails leaves it as it was.
- `login` names the route to redirect to when `requires` fails.
- `error` and `notFound` render failure pages; an app with a `silo` may
  add `maintenance : Html`, the 503 page of a silo not served.
- `route` lines and their `update`s declare app-level routes (above). They
  are optional.

## Notices and in-flight state

### Toasts

A write route can answer with a toast. `Response.refresh |> Response.toast
"Saved"` is the `refresh` it wraps, plus a notice: after the page's regions are
patched in, the runtime appends `<al-toast variant="info" timeout="5000">Saved</al-toast>` to
the page's `<al-toasts>` region as an event of its own, so a later morph does
not show it again.

Put `<al-toasts>` in the layout, outside anything a route re-renders; a page
without one gets a region the runtime adds to the body of the answer.

`Response` has four toast functions, each `Text -> Response -> Response`, one
per kind of toast:

- `Response.toast`, an `info` toast: news that is neither good nor bad
  ("Showing archived posts").
- `Response.success`: what the person asked for was done ("Saved", "Deleted 2
  of 3").
- `Response.warning`: it was done, with something to look at ("Saved, but the
  image was too large and was left out").
- `Response.error`: it was not done ("Error: try again"). An error toast
  stays for ten seconds (`timeout="10000"`); the others close after
  five seconds. A form's own problems are better told with `invalid`, next
  to the form.

> **Old names.** `Notice.*` is deprecated (W0012). `Notice.with`,
> `Notice.success`, `Notice.warning` and `Notice.error` are the old names of
> `Response.toast`, `Response.success`, `Response.warning` and
> `Response.error`; write the new names by hand.

### Notices without JavaScript

Without JavaScript there is no toast (the region is hidden), so a request
made without the script (a form posted with JavaScript off) gets its notices
as `<al-alert>`s of the same kinds at the top of the page that answers it,
one for each toast, in order. An alert has `role="alert"` for a warning or
an error, so a screen reader reads it out at once, else `role="status"`.

The page that answers it is:

- the page itself, on an `invalid`;
- the next page, after a `303` (a `redirect`, or a `refresh` or `render` that
  went back to the referring page).

The kinds and texts ride a cookie signed with the app's key, `alder_notice`,
for that one page. A cookie holds about 4 KB, so when the toasts do not all
fit, the first ones are kept whole and the rest dropped, with a warning in
the log. Appendix C lists the exact markup of each kind.

A browser test reads the toasts of a step as `r.notices`, their texts, and
`r.toasts`, their kinds and texts (chapter 10).

### In-flight state

While a write is pending, the form (or the form around a `click` button)
that sent it is `inert` and `aria-busy="true"`, and a two-pixel bar runs along
the top of the page; both go when the answer arrives. A boosted link shows the
bar too, and Back during a pending GET still ends on the page Back returned
to.

An inert form loses the keyboard focus, so when the answer arrives the runtime
puts it back on the field that had it (or the element that now carries its
`id`), unless the focus has gone somewhere else meanwhile: a refused form
leaves the cursor where the user was typing.

The bar is a rule the runtime puts in the head (`html[data-alder-pending]`),
not part of the design library.



# 7. Effects

Alder code never performs an effect. It returns a description of effects,
and the runtime carries them out. There are two kinds.

| Type | May | Used by |
|---|---|---|
| `Query a` | read | `R.load`, of a GET route |
| `Task a` | read and write | `R.update`, of a write route |

A `Query` cannot write. A `Task` can do anything a `Query` can, plus writes.
`load` returns a `Query`, so a GET route can never change anything by being
viewed. `update` returns a `Task Response`: the writes, and what to send back.

## Building effects

Effects are built in effect blocks with `let x <- e`:

```alder fragment
Show.load : Show.Params -> Session -> Query Model
Show.load params session = {
  let post <- Db.get Post params.id
  let comments <- Db.from Comment |> Db.where (Comment.post.eq params.id) |> Db.all
  Query.succeed { post: post, comments: comments }
}
```

Each `let x <- e` runs `e` and binds its result. The block's final expression
is the result of the whole effect. `Query.succeed` and `Task.succeed` wrap a
plain value.

The same shape works for `Task`:

```alder fragment
Toggle.update : Toggle.Params -> Session -> Task Response
Toggle.update p session = {
  let todo <- Db.getMine Todo p.id  -- the user's todo, or a 404 (chapter 6, "Rules")
  let _ <- Db.update Todo todo.id (\t -> { t | done: not t.done })
  Task.succeed Response.refresh
}
```

The block's last expression is the `Response` the route sends back.

A `Query` can be run inside a `Task` block with `let x <-`. That is the
only place a `Query` stands for a `Task`: as the block's last expression,
in a `match` arm or an `if` branch, as the whole body of a `Task` value,
or as an element of `Task.all`, a `Query` is a type error (E0003,
`expected Task Int, got Query Int`). Bind it and finish with
`Task.succeed`:

```alder error=E0003 "expected Task Int, got Query Int"
recount : Task Int
recount = Db.count (Db.from Todo)
```

```alder
recount : Task Int
recount = {
  let n <- Db.count (Db.from Todo)
  Task.succeed n
}
```

A `Task` cannot be used inside a `Query` block.

Effect values are descriptions, not results, so they cannot be compared:
`==` and `!=` on a `Query` or `Task` (or on a record or union holding one)
are E0003, and neither can be a `Dict` key.

## Functions on effects

```alder
Query.succeed : a -> Query a
Query.map     : (a -> b) -> Query a -> Query b
Query.all     : List (Query a) -> Query (List a)
Query.notFound : Query a            -- stop: respond 404 with App.notFound
Query.redirect : Route -> Query a   -- stop: respond 303 to a route

Task.succeed  : a -> Task a
Task.fail     : Text -> Task a
Task.notFound : Task a              -- fail: respond 404 with App.notFound
Task.map      : (a -> b) -> Task a -> Task b
Task.all      : List (Task a) -> Task (List a)
```

There is no `andThen`; use `let x <-` in a block.

## Failure

### A `Task` that fails

A `Task` can fail. `Task.fail "message"` fails explicitly; a capability may
fail on its own (database down, email rejected). A failed task stops
`update`, undoes everything it did (its database writes and its
`Session.put` changes), and the runtime renders `App.error`.

`Task.notFound` fails the same way but answers 404 with `App.notFound`:
use it when a request names something that is not there, or not the
user's. The scoped operations of chapter 6, "Rules" (`Db.getMine`,
`Db.updateMine`, `Db.deleteMine`) fail this way by themselves when the
row is missing or someone else's.

> **`Response.invalid` is not a failure.** A task that ends in
> `Response.invalid` has not failed: it produced a result, so its writes and
> session changes are **kept** (chapter 6, "`Response`"). To refuse without
> changing anything, decide before writing.

There is no `try`/`catch`: if a failure should be handled, the capability
returns a `Result` instead of failing.

### A `Query` that stops

A `Query` cannot fail. A missing row is `Nothing`; a database outage is
an `App.error` page. It can stop: `Query.notFound` answers 404, and
`Query.redirect r` answers a 303 to the route value `r`, written as an
`href={r}` or a `Response.redirect r` is (`Url.external` included). A GET
route's `load` uses it to send the browser on without showing anything:

```alder fragment
Start.load params session =
  if params.go == "builder" then Query.redirect (Builder { seed: params.seed })
  else Query.succeed { seed: params.seed }
```

Nothing after it runs, and `view` is not called. A `Query` bound in a
`Task` block stops the `Task` the same way: `let _ <- Query.redirect r`
in `update` is a failure, its writes and `Session.put` changes undone,
and the request is answered with the 303 (in `Test.run`, chapter 10, it is
`Err "redirected to /builder/7"`). To write and then go, end the
`Task` with `Response.redirect r` instead (chapter 6).

## Capabilities

A capability is a typed interface the program uses and the host implements
in Go:

```alder
capability Email {
  send : { to: Text.Email, subject: Text, body: Html } -> Task Unit,
}

capability Weather {
  current : { city: Text } -> Query (Maybe Float),
}
```

Each entry is a function whose result is a `Query` or `Task`. Using
`Email.send { ... }` in `update` is then an ordinary effect.

The compiler generates a Go interface for each capability, and the app will
not build or start until every capability is implemented and wired in
("Implementing a capability", below).

In a test, a capability can instead be replaced by a `fake`, an
implementation written in Alder beside the capability (chapter 10, "Faking a
capability").

Capabilities are declared in any module and used by their name. When
two imported modules both declare `Clock`, the bare name is E0002
"`Clock` is a capability of both `A` and `B`: write `A.Clock` or
`B.Clock`": the call names the module too, `A.Clock.now`. The
standard library provides these; they need no declaration:

```alder
Db          -- chapter 8
Session.put : (Session -> Session) -> Task Unit
Log.debug   : Text -> Task Unit
Log.info    : Text -> Task Unit
Log.warn    : Text -> Task Unit
Log.error   : Text -> Task Unit
Time.now    : Query Time
Random.int  : { min: Int, max: Int } -> Task Int
Random.id   : Task Text              -- 20-character URL-safe id
Http.get    : { url: Text } -> Task (Result Http.HttpError Text)
Http.post   : { url: Text, body: Text } -> Task (Result Http.HttpError Text)
```

The four `Log` functions write a line at their level; `ALDER_LOG` (default `info`) chooses what is
kept, and `Log.debug` is dropped by default. A log line outlives the request and is read by
others: log ids and outcomes, not the values people typed; the runtime masks secrets in URLs,
not in a `Log` text.

The same goes for tokens in URLs. A reset or magic link carries a token
the app made with `Random.id`; the log masks it only when its
parameter's name says it is a secret, so write `route Reset = GET "/reset/{token}" { … }`
or `?token=…`, not `/reset/{t}` or `?t=…` (chapter 16, "What is never
logged").

A type of a standard module other than the Prelude is written qualified (`Http.HttpError`; a bare
`HttpError` is E0202).

`Random.int` is a fresh number on every run; for the same numbers every run use the pure,
seeded `Random.step` (chapter 9, "Random"), which is no effect.

Nothing else in the language touches the outside world. If a function's
signature has no `Query` or `Task` in it, it is pure.

### A capability in `load`

An entry whose result is a `Query` is a
read, so `load` can call it like `Db.get`. The blog (chapter 14) keeps
its Markdown renderer behind such an entry. Its module,
`examples/blog/app/Markdown.alder`, begins:

```alder part=blog/Markdown
module Markdown

-- Rendering a post's body is outside the language: a capability implemented in Go
-- (adapters/markdown/markdown.go), which builds the HTML node by node, so a post cannot inject markup.
capability Markdown {
  render : Text -> Query Html,
}
```

`Posts.Show`'s `load` writes `let body <- Markdown.render post.body` and
keeps the result in its Model. An `Html` value is
markup, so the route's `view` can embed it in its own `Html` (chapter 5).

The capability has the same name as its module. From
another module, `Markdown.render` therefore names the capability's
entry, because the module `Markdown` has no value called `render`. A
module that declared both a value `render` and the entry would be
E0009.

In Go, the entry is the method
`Render(ctx, args app.Args__Markdown__Markdown__Render) (html.Node, error)`
on the field `Markdown__Markdown` of `Caps` ("Implementing a capability",
below). An `Html` value is an `html.Node` from `alder/rt/html`, which
escapes every piece of text when the page is rendered.

The module ends with a `fake`, an implementation of the capability
written in Alder that only tests use, here one that returns the text
unrendered:

```alder part=blog/Markdown
-- In a test, `let _ <- Test.fake Markdown` replaces the adapter with this: the body as plain
-- text, so a test about a page asserts on the page, not on how Markdown is rendered.
fake Markdown {
  render text = Query.succeed (Html.text text)
}
```

## How effects run

### What a capability looks like in Go

The to-do app checks passwords outside the language
(`examples/todos/app/Auth/Login.alder`):

```alder
capability Password {
  verify : { email: Text.Email, password: Text } -> Task (Maybe User.UserId),
}
```

The compiler turns it into a Go interface and a record for its argument,
in the module's file (`build/todos/Auth__Login_alder.go`):

```go
type Cap__Auth__Login__Password interface {
	Verify(ctx context.Context, args Args__Auth__Login__Password__Verify) (rt.Maybe[string], error)
}

// Args__Auth__Login__Password__Verify are the arguments of Password.verify.
type Args__Auth__Login__Password__Verify struct {
	Email    string
	Password string
}
```

Each entry becomes one method, named after the entry with a capital first
letter. It takes a `context.Context` first and returns the entry's result
and an `error`. What comes between depends on the entry's arguments:

- One argument: a parameter `args` of type `Args__<Module>__<Capability>__<Entry>`.
  For an anonymous record like `{ email, password }`, that is a struct with
  the same fields. For a named record type, it is that type. For anything
  else, it is `struct{ Value T }`.
- Several arguments (a curried entry such as `UserId -> Bool -> Query Int`):
  one Go parameter each, `a1`, `a2`, ….
- No argument (an entry that is itself a `Query` or `Task`): only `ctx`.

From `capability Stats { total : Query Int, forOwner : UserId -> Bool -> Query Int }`
in `testdata/golden/gen_effects/app/Todo.alder`:

```go
type Cap__Todo__Stats interface {
	Total(ctx context.Context) (int64, error)
	ForOwner(ctx context.Context, a1 string, a2 bool) (int64, error)
}
```

An implementation is any Go type with those methods. This stub is from
`TestGen_RunTodos` in `internal/gen/gen_test.go`:

```go
func (p password) Verify(ctx context.Context, args Args__Auth__Login__Password__Verify) (rt.Maybe[string], error) {
	if args.Email == "a@b.c" && args.Password == "pw" {
		return rt.Just(p.user), nil
	}
	return rt.Nothing[string](), nil
}
```

Returning a non-nil `error` fails the task, which rolls it back. The
brands are gone in Go (`User.UserId` is a `string`), so the Go side must
keep the brand's promise itself.

The other types:

- `Int` is `int64`, `Float` `float64`, `Text` `string`, `Bool` `bool`.
- `Unit` `struct{}` (an entry returning `Task Unit` returns
  `struct{}{}, nil`).
- `Maybe a` `rt.Maybe[A]` (`rt.Just(x)`, `rt.Nothing[A]()`).
- `Result e a` `rt.Result[E, A]`.
- `List a` `[]A`.
- `Time` `time.Time`.
- `Html` `html.Node`.

The stub above lives
inside the generated package; an adapter, in its own package, imports it
as `"app"` and writes `app.Cap__…` and `app.Args__…`, with `rt.Maybe`
from `"alder/rt"`.

An adapter that uses the database passes
`db.TxFrom(ctx)` (from `"alder/rt/db"`), the running task's transaction,
to every query it makes.

## Implementing a capability

### The rule

A capability `X` declared in module `A.B` is implemented by the Go
package in `adapters/a/` (the module's first segment, lower-cased). That
package exports one function, with no parameters, named after the rest of
the module and the capability (`B` and `X` joined, here `BX`), returning
the capability's interface:

```go
func BX() app.Cap__A__B__X
```

Four capabilities, one per row:

| Capability | Declared in module | Directory | Function |
|---|---|---|---|
| `Password` | `Auth.Login` | `adapters/auth/` | `func LoginPassword() app.Cap__Auth__Login__Password` |
| `Markdown` | `Markdown` | `adapters/markdown/` | `func Markdown() app.Cap__Markdown__Markdown` |
| `Stripe` | `Payments` | `adapters/payments/` | `func Stripe() app.Cap__Payments__Stripe` |
| `Clock` | `App` | `adapters/app/` | `func Clock() app.Cap__App__Clock` |

When the module has one segment (`Markdown`, `Payments`, `App`), nothing
is left of it after the directory, so the function is just the
capability's name. Capabilities whose modules start with the same segment
share a package: `Auth.Login`'s `Password` and an `Auth.Signup`'s
`Password` are both in `adapters/auth/`, as `func LoginPassword()` and
`func SignupPassword()`.

The directory is the package's import path, not its name: when it is a
Go keyword (`adapters/select/` for a module `Select.Menu`), write
`package selectadapter` (any other name) in it; the directory stays.

You never have to work this out yourself. When an adapter is missing,
`alder build` prints the exact directory and function to write, one line
per missing adapter, such as
``capability `Markdown.Markdown` needs `adapters/markdown/` with `func Markdown() app.Cap__Markdown__Markdown` ``.

A few words of Go, for what follows:

- A Go **package** is one directory of `.go` files that all start with the
  same `package <name>` line.
- A name that starts with a capital letter (`LoginPassword`) is
  **exported**: other packages can use it; `password` cannot be seen outside
  its package.
- A Go **interface** is a list of methods; any type that has those methods
  implements it, with nothing to declare.
- The function the rule names is a **constructor**: it builds the value the
  app will call.


# 8. Database

## Tables

A table is declared with `table`. It defines a record type of the same name
and the storage for it.

```alder
type TodoId = brand Text

table Todo {
  id: TodoId @id,
  title: Text.NonEmpty,
  done: Bool = False,
  due: Maybe Date,
  owner: User.UserId @ref User @owner,
  created: Time @created,
}
```

### Fields and attributes

- Exactly one field has `@id`. Its type is a brand of `Text`; the runtime
  generates the value on insert.
- `@ref T` marks a foreign key to table `T`'s id. The field's type must be
  `T`'s id type, or `Maybe` of it. A delete rule may follow it:
  `@restrict` (the default), `@cascade` or `@setNull` (below, under
  Writing).
- `@created` is set to the current time on insert. `@updated` is set on
  every write.
- A field with a default may be omitted on insert, but not from a
  literal of the row type: a `Todo` written out names every field.
- `@unique` marks a field that must be unique.
- `@owner` marks the one field that names the row's owner: a `@ref` to
  the user table, of the session's user type. It gives the table the
  scoped operations `Db.mine`, `Db.getMine`, `Db.updateMine`,
  `Db.deleteMine` and `Db.insertMine` ("Your own rows", under Writing; chapter
  6, "Rules").

  It changes nothing in the database: the column is stored, indexed and
  referenced exactly as without it, so adding or removing `@owner` makes no
  migration.

- Field types: `Int`, `Float`, `Bool`, `Text`, `Date`, `Time`, brands of
  those, unions with no payloads, and `Maybe` of any of those. Nothing
  nested; model nesting with a second table and `@ref`.

### Where a table is declared

A table is declared in a module named after it, together with the brand for
its id and any functions that belong to it. By convention that module is in
`Table/`: `Todo` in `app/Table/Todo.alder`, written `Todo` after
`import Table.Todo`.

The compiler's rule is that the module's name **ends in
the table's name** (`Todo`, `Table.Todo` or `Table.Billing.Invoice` for
`Invoice`), so subfolders can group related tables, and a module holds one
table.

A table declared in any other module is an error at its declaration
(E0018): `table Comment` in `app/Post.alder` belongs in
`app/Table/Comment.alder`, even if only `Post` uses it.

Table names are app-wide (a table is `Todo` everywhere, whichever folder it is
in), so two tables of one name, in two folders, are E0037.

### Defaults

A table field's default must be a **literal**: `True` or `False`, a
number, a string without `{…}`, a constructor without a payload
(`Reader`, `Nothing`), or `Just` of one of those. The default is not only
used by `Db.insert`: migrations (`alder migrate`) write it into the
database as SQL, to fill a new field in the rows that already exist, or
to replace `Nothing` when a field stops being a `Maybe`.

The compiler turns a literal into SQL text when it lowers the table (`True`
becomes `1`, `Reader` becomes `'Reader'`, `Nothing` becomes `NULL`); an
expression would have to be run by the program, so it is refused (E0022):

```text
error[E0022]: a table field's default must be a literal
```

Defaults of forms, records, the session and route parameters are never
stored, and may be any expression.

### Renaming: `@renamed`

When you rename a table or a field, say so with **`@renamed "old"`**,
after the table's name or after the field's type (and default). The next
migration then renames the table or column, keeping its data, instead of
dropping the old one and creating an empty new one; without the
attribute a rename looks exactly like a removed field and an added one.

The attribute may stay once the migration is made (the migration's
snapshot already has the new name):

```alder
module Table.Memo

type MemoId = brand Text

table Memo @renamed "Note" {
  id: MemoId @id,
  text: Text @renamed "body",
  pinned: Bool = False,
}
```

The old name must be a name (`"body"`, not `"the body"`), not the field's
or table's own name, and not a field the table still has; a field takes
one `@renamed`, and two fields cannot both be renamed from one name (all
E0011).

Moving a table to another module, as when `app/Todo.alder` becomes
`app/Table/Todo.alder`, needs no `@renamed`: the migration sees the same
table name leave one module and arrive in another, and renames the table,
so its rows stay.

> **Short names.** `@renamed "Note"` likewise finds a table of `Table/` by
> its short name.

## Reading

Reads are `Query` values.

```alder fragment
-- Generic over tables: a program cannot state these (a function of `Table t` is E0003, below).
Db.get   : Table t -> Id t -> Query (Maybe t)
Db.from  : Table t -> Select t
Db.where : Condition t -> Select t -> Select t
Db.orderBy : Order t -> Select t -> Select t
Db.limit : Int -> Select t -> Select t
Db.offset : Int -> Select t -> Select t
Db.all   : Select t -> Query (List t)
Db.first : Select t -> Query (Maybe t)
Db.count : Select t -> Query Int
```

`Table t` is the table's name used as a value: `Db.get Todo id`. Conditions
and orders are built from the table's columns, which are values named
`Table.field`:

```alder
Db.from Todo
  |> Db.where (Todo.owner.eq session.user)
  |> Db.where (Todo.done.eq False)
  |> Db.orderBy Todo.created.desc
  |> Db.limit 50
  |> Db.all
```

Column operations:

```alder
Todo.title.eq   : Text.NonEmpty -> Condition Todo   -- the column's type; also .ne
Todo.created.lt : Time -> Condition Todo         -- also .le .gt .ge
Todo.title.contains : Text -> Condition Todo     -- substring, case-insensitive
Todo.title.startsWith : Text -> Condition Todo   -- prefix, case-insensitive
Todo.owner.in   : List User.UserId -> Condition Todo
Todo.due.isNothing : Condition Todo              -- for Maybe fields; also .isJust
Todo.created.asc, Todo.created.desc : Order Todo   -- unions order by declaration

Db.and : List (Condition t) -> Condition t
Db.or  : List (Condition t) -> Condition t
Db.not : Condition t -> Condition t
```

### How a `Select` combines

A `Select` is a description that the steps add to, not a pipeline that
runs in order: where a step sits does not change what it means.

- Several `Db.where` calls are combined with `and`.
- `Db.orderBy` keys accumulate in the order written: the first is the
  primary sort key, a later one breaks its ties.
- `Db.limit` and `Db.offset` are settings: the last one written wins, and
  they apply after every condition and ordering wherever they appear, so
  `Db.limit 10 |> Db.where c` is ten rows that satisfy `c`.
- `Db.first` is the first row: a `Select` with limit 1, or with its own
  limit when that is smaller (`Db.limit 0 |> Db.first` is `Nothing`).

### Columns and tables must be concrete

A column must be written in place, `Todo.done.eq False`: a column
method on a variable or record field holding a column (`let c = Todo.done`,
then `c.eq False`) is E0003.

And the database functions need to know their
table where they are used: `countAll : Table t -> Query Int` calling
`Db.count (Db.from t)` is E0003 (`` `Db.count` needs a concrete table; `t`
is a type parameter``); write one function per table, `Table Todo`.

### `Maybe` columns

A condition on a `Maybe` column is true or false for
every row, never unknown. `Nothing` is matched by `.isNothing`,
`.eq Nothing` and an `.in` list that contains `Nothing`
(`Todo.due.in [Just d, Nothing]`), and `.eq (Just d)` and `.ne Nothing`
never match it.

`Db.not c` matches exactly the rows `c`
does not, so `Db.not (Todo.due.eq (Just d))` includes the rows with no due
date. Ordering by a `Maybe` column puts `Nothing` first with `.asc` and
last with `.desc`.

### Search

`.contains` matches a column whose text has the argument
somewhere in it, and `.startsWith` one whose text begins with it. Both
work on `Text` columns and brands of `Text` only (on a brand, the
argument is still plain `Text`), and both ignore the case of the letters
A–Z.

The argument is matched literally: `%`, `_` and `\` have no special
meaning, so `Todo.title.startsWith "100%"` finds `100% done` and not
`1000 ways`. An empty argument matches every row. To search two columns,
combine the conditions with `Db.or`:

```alder
module Article

type ArticleId = brand Text

table Article {
  id: ArticleId @id,
  title: Text.NonEmpty,
  body: Text,
  created: Time @created,
}

search : Text -> Query (List Article)
search q =
  Db.from Article
    |> Db.where (Db.or [Article.title.contains q, Article.body.contains q])
    |> Db.orderBy Article.created.desc
    |> Db.limit 10
    |> Db.all
```

A search costs the column's length times the argument's for every row,
and SQLite refuses a pattern much over 50 KB, so a search text over
10,000 bytes fails the query with `search text too long`.

Cut a query a visitor typed before searching with it, as the blog does with
`Text.slice 0 100` (chapter 14).

On any other column the method is an error (E0003):

```alder error=E0003 "`.startsWith` needs a Text column, got `Bool`"
Db.from Todo |> Db.where (Todo.done.startsWith "T")
```

## Writing

Writes are `Task` values.

```alder fragment
-- Generic over tables, like the reads above.
Db.insert : Table t -> Insert t -> Task (Id t)
Db.update : Table t -> Id t -> (t -> t) -> Task Unit
Db.delete : Table t -> Id t -> Task Unit
```

`Insert t` is the table record without `@id`, `@created`, and `@updated`
fields, and with defaulted fields optional. `Db.update` reads the row,
applies the function, and writes it back; updating a row that doesn't exist
fails the task.

The function may not change the `@id` field: if it
returns a record with a different id, the task fails with
`Db.update Todo: update may not change the id` and nothing is written
(`UpdateRow` in `rt/db/store.go`).

Deleting a row that doesn't exist is not an error.

```alder fragment
let id <- Db.insert Todo { title: p.form.title, due: p.form.due, owner: session.user }
let _ <- Db.update Todo id (\t -> { t | done: True })
let _ <- Db.delete Todo id
```

### Transactions and `@unique`

Writes in one `update` run in a single transaction. If the task fails,
nothing is written. Inserting or updating a row that violates `@unique`
fails the task with a message such as `User.email must be unique`, and
everything the task wrote before is rolled back; check first with a query
and answer `Response.invalid` if the user should see a message.

> **`Response.invalid` keeps earlier writes.** It is a successful result, so
> the transaction commits and writes made before it are kept (chapter 6,
> "`Response`"): decide first, then write.

### Delete rules

`@ref` is checked the same way. Inserting or updating a row whose `@ref`
field names a row that does not exist fails the task with
`Todo.owner: no User with that id`. What deleting a row that other rows
still refer to does is the referring field's **delete rule**, written
after `@ref T`:

- **`@restrict`**, the default, refuses: `Db.delete` fails with
  ``User QXZMRYnkC2vuE_TefkuR is referenced by a row of Todo (`Todo.owner`)``
  (the table and the id, then the table and the field that refer to it).
  Delete or re-point the referring rows first.

  The blocking row can be further down a chain: if comments cascade
  from posts, a `@restrict` reference to a comment refuses the post's
  delete, with ``Post … is referenced by another row, directly or
  through rows that `@cascade` would delete with it``.

- **`@cascade`** deletes the referring rows too, and theirs in turn if
  other tables cascade from them.
- **`@setNull`** sets the referring field to `Nothing`; the field must be
  a `Maybe`.

Either way a failure rolls back the task's earlier writes. A table of
notes attached to to-dos, which go when their to-do goes, and which keep
an optional link to another to-do that is cleared when that one goes:

```alder
module Table.Note

import Table.Todo

type NoteId = brand Text

table Note {
  id: NoteId @id,
  todo: Todo.TodoId @ref Todo @cascade,
  seeAlso: Maybe Todo.TodoId @ref Todo @setNull,
  body: Text,
}
```

The rules are carried out by SQLite inside the delete statement, not by
the runtime: `Db.delete Todo id` is one step in a test's trace
(chapter 10) however many notes go with it.

A field takes at most one rule, and only a `@ref` field takes one, after
`@ref T` (`@cascade` alone, or before `@ref T`, is E0011).

`@setNull` on a field that is not a `Maybe` is E0003, since the field
could not hold the `Nothing` it would be set to:

```text
error[E0003]: `@setNull` needs a `Maybe` reference, got `Todo.TodoId`
```

### Your own rows

A table with an `@owner` column (above) also has five **scoped**
operations, `Db.mine`, `Db.getMine`, `Db.updateMine`, `Db.deleteMine` and
`Db.insertMine`, for the routes of a signed-in user: chapter 6, "Rules",
explains them, and chapter 9 lists their types.

## Migrations

A **migration** is a file of SQL statements that turns a database with
the old tables into one with the new tables, keeping the rows. Alder
writes them: you change a `table`, run `alder migrate`, read the file,
and commit it. The app applies new files every time it starts.

### The files

An app with migrations has a `migrations/` directory beside `app/`. Both
examples have one:

```text
examples/blog/migrations/
  0001_initial.sql    -- creates every table
  schema.json         -- the tables as of the last migration
```

Each `NNNN_<name>.sql` is one migration, numbered in the order they
apply. `schema.json`, the **snapshot**, records every table, column,
type, default and attribute as of the last migration; `alder migrate`
compares your `table` declarations with it. Commit the whole directory:
the executable carries the `.sql` files inside it (chapter 13).

For your own app, run `alder migrate` once, from the first table on:
with no `migrations/` it creates one holding `0001_initial.sql`, which
creates every table (an app without migrations loses its data on every
change: "Without migrations", below).

### A change, step by step

The steps below were run on a copy of the blog (chapter 14):
`cp -R examples/blog /tmp/blog`, then `cd /tmp/blog`. First, give posts a
summary. Add the field to `table Post` in `app/Table/Post.alder`:

```alder fragment
  created: Time @created,
  summary: Maybe Text = Nothing,
}
```

The default `= Nothing` lets every `Db.insert Post` leave the field out,
but not a `Post` written as a literal: the checker points at the one test
that builds a whole `Post` by hand, which needs `summary: Nothing`:

```text
app/Table/Post.alder:35:22: error[E0003]: record literal for `Post` is missing `summary` (a default applies only to an insert, the session cookie, a page or message parameter, or a form, not to a literal)
```

With that fixed, the tables are ahead of the migrations:

```text
$ alder check
migrations/schema.json:1:1: error[E0019]: the schema changed since migration 0001 (table Post: column summary): run `alder migrate`
```

`build`, `run`, `test` and `eval` refuse the app with the same error.
Run the command it names:

```text
$ alder migrate
wrote migrations/0002_add_summary.sql
  add column Post.summary
updated migrations/schema.json
```

```text
$ cat migrations/0002_add_summary.sql
-- alder migration 0002_add_summary
-- from 3fa31c8ee016
-- to c709adc8b82b
-- step: add column Post.summary

ALTER TABLE "Post__Post" ADD COLUMN "summary" TEXT;
```

The `--` lines are the header: `from` and `to` are short fingerprints
(*hashes*) of the tables before and after the file, and the `step:`
lines say in words what the statements below them do, one per line.
The runtime runs each file in a transaction of its own. Read the file,
then commit it with `schema.json`.

Next, rename the field. A rename looks exactly like a removed field and
an added one, so say it is a rename with `@renamed "old"` (above, under
Tables):

```alder fragment
  excerpt: Maybe Text = Nothing @renamed "summary",
```

```text
$ alder migrate
wrote migrations/0003_rename_summary.sql
  rename column Post.summary to excerpt
updated migrations/schema.json
```

The file's statement is
`ALTER TABLE "Post__Post" RENAME COLUMN "summary" TO "excerpt";`, which
keeps every post's summary. Without `@renamed`, the migration would drop
`summary`, with its data, and add an empty `excerpt`. Once the migration
exists you may delete `@renamed "summary"`: `alder check` stays clean,
because the snapshot already says `excerpt`.

Last, remove the field. `--dry-run` prints the file without writing it:

```text
$ alder migrate --dry-run
-- alder migration 0004_drop_excerpt
-- from 7af9d614a3af
-- to 3fa31c8ee016
-- step: drop column Post.excerpt
-- DROPS DATA: column Post.excerpt

ALTER TABLE "Post__Post" DROP COLUMN "excerpt";

dry run: migrations/0004_drop_excerpt.sql not written
```

```text
$ alder migrate
wrote migrations/0004_drop_excerpt.sql
  drop column Post.excerpt
!! DROPS DATA: column Post.excerpt
updated migrations/schema.json
```

**A migration never loses data silently.** Every step that can lose data
is marked `DROPS DATA`, in the output (`!!`) and in the file's header.
Read those lines before you commit.

A migration you have not shared yet
can be thrown away: delete the file, restore the snapshot with
`git checkout migrations/schema.json`, change the tables, and run
`alder migrate` again. Once a migration has run against a database you
keep, never edit or delete it; write the next one instead.

> **An edited file.** A file whose statements were edited is caught when the
> app applies it, at startup: the tables do not come out as its header says,
> and the app refuses to start; `alder test` runs every file, so it finds it
> first.

The file's name comes from its steps (`add_summary`, `change_post` for
a rebuild), or from `--name some_name`. With nothing changed,
`alder migrate` prints `no changes since 0004_drop_excerpt`.

### What a change becomes

- **A new `Maybe` field, last in the table,** with no default or
  `= Nothing` is `ADD COLUMN`; existing rows get `Nothing`.
- **A new field that is not a `Maybe`** needs a default to fill the rows
  that already exist. Without one, `alder migrate` writes nothing and
  says ``Post.views: a new non-null column needs a default (`= 0`) so
  existing rows can be filled`` (if no `Db.insert` has refused to
  compile first). A new `@ref` field has no possible
  default, so add it as a `Maybe` first.
- **A removed field** is `DROP COLUMN`, and a removed table `DROP TABLE`,
  marked `DROPS DATA`. SQLite cannot drop a `@unique` or `@ref` column in
  place, so removing one of those is a rebuild (below), marked
  `DROPS DATA` all the same.
- **A rename** declared with `@renamed` is `RENAME COLUMN` or
  `RENAME TO`.
- **Anything else is a rebuild.** SQLite's `ALTER TABLE` can only
  append, drop and rename columns. These changes build the table again:

  - changing a type;
  - making a field a `Maybe` or not;
  - changing a union's constructors;
  - adding or removing `@unique`, `@ref` or a delete rule;
  - adding a field anywhere but last, or reordering fields;
  - adding a field that is not a `Maybe` or has a `Just` default (filled with
    the default).

The migration creates
the new table under a temporary name, copies every row into it,
converting each value, drops the old table and renames the new one.
Adding `views: Int = 0` to `Post`:

```text
-- step: rebuild table Post: Post.views: new column filled with 0

CREATE TABLE "Post__Post__alder__new" ("id" TEXT NOT NULL PRIMARY KEY, "slug" TEXT NOT NULL UNIQUE, "title" TEXT NOT NULL, "body" TEXT NOT NULL, "published" INTEGER NOT NULL CHECK ("published" IN (0, 1)), "author" TEXT NOT NULL REFERENCES "User__User"("id"), "created" TEXT NOT NULL, "views" INTEGER NOT NULL) STRICT;
INSERT INTO "Post__Post__alder__new" ("id", "slug", "title", "body", "published", "author", "created", "views") SELECT "id", "slug", "title", "body", "published", "author", "created", 0 FROM "Post__Post";
DROP TABLE "Post__Post";
ALTER TABLE "Post__Post__alder__new" RENAME TO "Post__Post";
CREATE INDEX IF NOT EXISTS "Post__Post__author" ON "Post__Post"("author");
```

#### Conversions that lose data

A conversion that cannot keep every value is marked `DROPS DATA` and
says what is lost:

- `Float` to `Int` cuts off fractions.
- `Text` to `Int` keeps the leading whole number (`'12abc'` becomes `12`) and
  turns other text into `0`.
- A removed constructor deletes the rows that hold it (the line adds `(to keep
  them, add an UPDATE first)`, a statement you write at the top of the file).
- A `Maybe` field made required replaces `Nothing` with the default, or,
  without one, deletes those rows.

A change with no sensible conversion (`Date` to `Int`, say) is refused: remove
the field and add one under a new name. Adding `@unique` to a column that
holds duplicates fails when the migration runs, and nothing is changed.

#### Defaults and indexes

A default lives in the program, not the database, so changing only a
default needs no migration. Every `@ref` column gets an index (the
`CREATE INDEX` line above), so finding a post's comments does not read
every comment.

### Tests run through migrations

`alder test` builds each test's in-memory database by applying the
migrations, so a passing run shows they make the app's tables. When the
tables changed after the last migration, every test fails with
``the migrations do not produce the app's schema: run `alder migrate` ``;
when the files themselves are refused (an edited file, say), every test
fails with the runtime's refusal, which names the file.

### Without migrations

An app without `migrations/` gets its tables from `Ensure`
(`rt/db/sqlite.go`), which records their hash and the storage format
(`db.Format`, now `1`) in `_alder_meta`. If either differs at start,
dev mode recreates the tables, empty; otherwise the app refuses:

```
db: schema mismatch: the database's schema (hash 5de59b25abca) differs from the app's (hash 40d037c4462a); changed tables: Todo; to start over, run in dev mode (ALDER_DEV=1 recreates the database empty: every row is deleted)
```

or, for a database from a runtime with an older format:

```
db: database format v0 is older than this runtime (v1); to start over, run in dev mode (ALDER_DEV=1 recreates the database empty: every row is deleted)
```

An app with migrations meets a new storage format through `alder
check`: ``the storage format changed (v1 → v2): this needs a rebuild
migration of every table, which this `alder migrate` cannot write yet
(`alder migrate --format` is planned); build with the compiler that wrote
migrations/ until then`` (E0019).


## Silos

`silo org { Project, Todo }` in `App` keeps those tables in a database per org; the
rest are in the shared database. Each listed table is `@partition org`; a table is in one silo;
a shared table may not `@ref` a siloed one, a siloed one may `@ref` a shared one or one of its
silo (E0033). Every `Db` operation on a siloed table needs the key narrowed (E0026); there is
no query over every org. `Db.update` cannot move a row to another org.

### One store per `Task`

A `Task` is one transaction, so it writes to one store, the shared one or
one silo; an `update` arm whose writes (followed into helpers, any module, and `Task.all`)
fall in two is E0031: "`Create` writes `Project` (silo `org`) and `AuditLog` (shared): a
Task writes to one store. Write `AuditLog` inside the silo, or send it from its own
message." Reads may mix (one `load` reads an org's name and its projects); `Session.put`
and capabilities do not count.

## Beyond CRUD

Joins, aggregates, and full-text search are not in the language. Declare a
capability and implement it in Go against the same database, through
`db.StoreFrom(ctx)` and `db.TxFrom(ctx)` so that it reads and writes in
the running task's transaction (chapter 7):

```alder
capability Reports {
  todosPerUser : Query (List { user: User.UserId, count: Int }),
}
```

# 9. Standard library

Every module here is always in scope. This is the complete list; a function
not listed here does not exist.

## List

```alder
List.length   : List a -> Int
List.isEmpty  : List a -> Bool
List.head     : List a -> Maybe a
List.last     : List a -> Maybe a
List.take     : Int -> List a -> List a
List.drop     : Int -> List a -> List a
List.reverse  : List a -> List a
List.map      : (a -> b) -> List a -> List b
List.filter   : (a -> Bool) -> List a -> List a
List.filterMap : (a -> Maybe b) -> List a -> List b
List.find     : (a -> Bool) -> List a -> Maybe a
List.any      : (a -> Bool) -> List a -> Bool
List.all      : (a -> Bool) -> List a -> Bool
List.member   : a -> List a -> Bool
List.foldl    : (a -> b -> b) -> b -> List a -> b
List.sum      : List Int -> Int
List.sumFloat : List Float -> Float
List.sortBy   : (a -> Int) -> List a -> List a      -- stable
List.sortByText : (a -> Text) -> List a -> List a   -- stable
List.indexed  : List a -> List { index: Int, item: a }
List.range    : Int -> Int -> List Int        -- inclusive
List.concat   : List (List a) -> List a
List.append   : List a -> List a -> List a    -- same as ++
List.unique   : List a -> List a
List.groupBy  : (a -> Text) -> List a -> Dict Text (List a)
List.indexedMap : (Int -> a -> b) -> List a -> List b
List.map2     : (a -> b -> c) -> List a -> List b -> List c
List.map3     : (a -> b -> c -> d) -> List a -> List b -> List c -> List d
List.foldr    : (a -> b -> b) -> b -> List a -> b
List.concatMap : (a -> List b) -> List a -> List b
List.intersperse : a -> List a -> List a
List.partition : (a -> Bool) -> List a -> { yes: List a, no: List a }
List.tail     : List a -> Maybe (List a)
List.repeat   : Int -> a -> List a
List.maximum  : List Int -> Maybe Int
List.minimum  : List Int -> Maybe Int
List.maximumFloat : List Float -> Maybe Float
List.minimumFloat : List Float -> Maybe Float
List.sortByFloat : (a -> Float) -> List a -> List a
```

`List.partition` returns a record with a `yes` field (the elements that satisfy the test) and a `no` field, each in input order. `List.map2` and `List.map3` stop at the shortest list.

`List.take`, `List.drop` and `List.tail` cost nothing however long the list:
the result shares the original's elements, which is safe because no list is
ever changed. `++` copies both sides into a new list (unless one is empty), so
a loop that grows a list with `acc ++ [x]` copies the whole list at every
step; use `List.foldl`, `List.concatMap` or, for text, a `Text.Builder`
(below) instead.

```alder eval=[10, 30, 50]
List.range 1 5
  |> List.filter (\n -> n % 2 == 1)
  |> List.map (\n -> n * 10)
```

```alder eval=[{ name: "a", age: 1 }, { name: "c", age: 1 }, { name: "b", age: 2 }]
List.sortBy (\p -> p.age) [{ name: "b", age: 2 }, { name: "a", age: 1 }, { name: "c", age: 1 }]
```

## Maybe

```alder
Maybe.map      : (a -> b) -> Maybe a -> Maybe b
Maybe.andThen  : (a -> Maybe b) -> Maybe a -> Maybe b
Maybe.withDefault : a -> Maybe a -> a          -- same as ??
Maybe.isJust   : Maybe a -> Bool
Maybe.toList   : Maybe a -> List a
```

```alder eval=42
Maybe.withDefault 0 (Text.toInt "42")
```

## Result

```alder
Result.map     : (a -> b) -> Result e a -> Result e b
Result.mapErr  : (e -> f) -> Result e a -> Result f a
Result.andThen : (a -> Result e b) -> Result e a -> Result e b
Result.withDefault : a -> Result e a -> a
Result.toMaybe : Result e a -> Maybe a
```

## Text

```alder
Text.length    : Text -> Int
Text.isEmpty   : Text -> Bool
Text.trim      : Text -> Text
Text.toUpper   : Text -> Text
Text.toLower   : Text -> Text
Text.contains  : Text -> Text -> Bool        -- Text.contains needle haystack
Text.startsWith : Text -> Text -> Bool
Text.endsWith  : Text -> Text -> Bool
Text.replace   : Text -> Text -> Text -> Text -- Text.replace old new text
Text.split     : Text -> Text -> List Text   -- Text.split sep text
Text.join      : Text -> List Text -> Text
Text.slice     : Int -> Int -> Text -> Text
Text.matches   : Text -> Text -> Bool        -- Text.matches regex text
Text.fromInt   : Int -> Text
Text.fromFloat : Float -> Text
Text.toInt     : Text -> Maybe Int
Text.toFloat   : Text -> Maybe Float
Text.toBytes   : Text -> Bytes               -- the UTF-8 bytes (chapter 9, "File, Bytes")
Text.padLeft   : Int -> Text -> Text -> Text -- Text.padLeft 2 "0" "7" == "07"
Text.padRight  : Int -> Text -> Text -> Text
Text.left      : Int -> Text -> Text         -- first n code points
Text.right     : Int -> Text -> Text         -- last n code points
Text.dropLeft  : Int -> Text -> Text
Text.dropRight : Int -> Text -> Text
Text.repeat    : Int -> Text -> Text
Text.reverse   : Text -> Text
Text.trimLeft  : Text -> Text
Text.trimRight : Text -> Text
Text.concat    : List Text -> Text           -- Text.join ""
Text.lines     : Text -> List Text
Text.words     : Text -> List Text
Text.indexes   : Text -> Text -> List Int    -- Text.indexes needle haystack
Text.toCodePoints : Text -> List Int
Text.fromCodePoints : List Int -> Text
Text.hash      : Text -> Int                 -- stable forever, never negative, not cryptographic
Text.NonEmpty, Text.Email, Text.Url, Text.Secret   -- refinement brands
```

```alder eval=["a", "b", "c"]
Text.split ", " "a, b, c"
```

```alder eval=True
Text.contains "ell" "hello"
```

#### Length and slices

`Text.length` counts code points (`Text.length "é"` is 1), and
`Text.slice start end` takes the code points from `start` up to, not
including, `end`.

- A negative index counts from the end (`Text.slice (-3) (-1) "abcdef"` is
  `"de"`; a negative argument needs its parentheses).
- Indices past either end are clamped.
- A `start` at or after `end` gives `""`.

#### Matching

`Text.matches regex text` is true when the regex (RE2 syntax) matches
anywhere in the text: write `^…$` to match the whole text. An invalid regex
matches nothing.

#### Numbers

`Text.fromFloat` writes the shortest decimal that reads back as the same
number, never an exponent (`2.0` is `"2"`, `3.5` is `"3.5"`). Infinity is
`"+Inf"` or `"-Inf"` and not-a-number is `"NaN"`.

`Text.toInt` and `Text.toFloat` give `Nothing` for anything else than a
plain decimal number (spaces, overflow, `"NaN"`).

```alder eval=["de", "2", "3.5"]
[Text.slice (-3) (-1) "abcdef", Text.fromFloat 2.0, Text.fromFloat 3.5]
```

#### Padding, lines and offsets

`Text.padLeft n pad s` makes `s` at least `n` code points long: a pad of
several characters is repeated and cut at the outer edge, so
`Text.padLeft 5 "ab" "x"` is `"ababx"`, and `s` itself is never cut.

`Text.lines` splits on `\n`, removes a `\r` before it (so `\r\n` text works)
and drops the empty line after a final newline: `Text.lines "a\nb\n"` is
`["a", "b"]`.

`Text.indexes needle haystack` lists every code-point offset where `needle`
starts, overlapping matches included (`Text.indexes "aa" "aaa"` is
`[0, 1]`).

> **No `toChars`.** `Text.split "" s` gives the characters.

#### Hashing

`Text.hash` is FNV-1a (64-bit) over the text's UTF-8 bytes with the top
bit cleared, so it is never negative and `Text.hash s % n` is a bucket in
`0..n-1`. Its values are fixed forever, on every platform: use it for
seeds (see "Random"), buckets and cache keys.

> **Not cryptographic.** Never use it for passwords, tokens or anything an
> attacker may choose.

### Building a large Text

```alder
Text.Builder                                 -- an accumulator for a large Text
Text.builder   : Text.Builder                -- empty
Text.add       : Text -> Text.Builder -> Text.Builder   -- Text.add piece b
Text.build     : Text.Builder -> Text
```

Interpolation and `Text.join` each make a new Text, copying every byte they
are given. That is cheap for a heading, but a page that renders a large body
by nesting them (rows joined into options, options into items, items into a
file) copies each byte once per level: a 100 MB JSON file allocates close to
a gigabyte.

A `Text.Builder` avoids that. `Text.add` puts a piece after everything added
so far without copying anything, and `Text.build` copies each byte once, into
the finished Text.

A builder is a value like any other: adding to it gives a new builder and
leaves the old one as it was, so two builders can grow from the same start.

- Make each row with interpolation and add whole rows; adding many tiny
  pieces (a comma, a field) costs more than the copy it saves.
- There is no `addAll`: `List.foldl Text.add b pieces` adds a list of pieces
  in order.

```alder eval="name,votes\nKent,3\nDane,5\n"
[{ name: "Kent", votes: 3 }, { name: "Dane", votes: 5 }]
  |> List.foldl (\r b -> b |> Text.add "{r.name},{Text.fromInt r.votes}\n") (Text.builder |> Text.add "name,votes\n")
  |> Text.build
```

Under the hood a builder is a chain of the pieces, newest first, each link
remembering the total length so far. `Text.build` allocates that many bytes
once and fills them from the end, walking the chain back. A File page passes
the result straight to `File.text`, which uses the Text's bytes as the body
without copying them again.

## Int, Float

```alder
Int.toFloat  : Int -> Float
Int.min      : Int -> Int -> Int
Int.max      : Int -> Int -> Int
Int.abs      : Int -> Int
Int.clamp    : { min: Int, max: Int } -> Int -> Int
Int.pow      : Int -> Int -> Int                -- Int.pow base exponent
Int.Positive, Int.NonNegative                -- refinement brands

Float.round  : Float -> Int
Float.floor  : Float -> Int
Float.ceil   : Float -> Int
Float.min, Float.max, Float.abs
Float.format : { decimals: Int } -> Float -> Text
Float.sqrt   : Float -> Float
Float.exp    : Float -> Float
Float.log    : Float -> Float                   -- natural logarithm
Float.pow    : Float -> Float -> Float          -- Float.pow base exponent
Float.truncate : Float -> Int                   -- rounds toward zero
Float.clamp  : { min: Float, max: Float } -> Float -> Float
Float.pi, Float.e                               -- Float constants
Float.isNaN, Float.isInfinite : Float -> Bool
Float.logBase : Float -> Float -> Float         -- Float.logBase base x
Float.sin, Float.cos, Float.tan, Float.asin, Float.acos, Float.atan : Float -> Float
Float.atan2  : Float -> Float -> Float          -- Float.atan2 y x
```

`Int` arithmetic wraps on overflow; `/` truncates toward zero and `%` has
the dividend's sign; an `Int` division or remainder by zero is a runtime
failure (chapter 4). `Float` division by zero gives infinity or NaN.

`Int.pow` wraps on overflow like `*`, and a negative exponent gives `0`
(`Int.pow 2 (-1)` is `0`), never a failure. `Float.clamp` takes the same
record as `Int.clamp` and checks in the same order, so `min` above `max`
behaves as it does there. `Float.truncate` rounds toward zero.

A `Float` literal may carry an exponent: `1.5e-3`, `1e6` and `2E+3`. Any
literal with an exponent is a `Float`, including `1e6`. `Text.toFloat` reads
the same form, and `Text.fromFloat` output is unchanged. An `Int` literal
that does not fit in 64 bits (above `9223372036854775807`) is an error, and a
`Float` literal that overflows to infinity is one too; the smallest `Int` is
written `-9223372036854775808` (the `-` directly on the literal, not `-(…)`).

`Float.round` rounds half away from zero: `Float.round 2.5` is `3` and
`Float.round (-2.5)` is `-3`. `Float.format { decimals: n }` rounds the same
way to `n` decimals and pads with zeros (`Float.format { decimals: 2 }
1.005` is `"1.01"`, and `1.5` gives `"1.50"`); it never writes `"-0"`
(`-0.001` with one decimal is `"0.0"`).

```alder eval=[3, -3, -3, -1]
[Float.round 2.5, Float.round (-2.5), -7 / 2, -7 % 2]
```

```alder eval=["1.01", "1.50", "0.0"]
[Float.format { decimals: 2 } 1.005, Float.format { decimals: 2 } 1.5, Float.format { decimals: 1 } (-0.001)]
```

IEEE rules: `Float.sqrt (-1.0)` is NaN, `Float.log 0.0` -infinity; no failures.

## Bool

```alder
not : Bool -> Bool
Bool.toText : Bool -> Text
```

`Bool.toText` writes the constructor names, `"True"` and `"False"`.

## Dict

```alder
Dict.empty    : Dict k v
Dict.fromList : List { key: k, value: v } -> Dict k v
Dict.toList   : Dict k v -> List { key: k, value: v }
Dict.get      : k -> Dict k v -> Maybe v
Dict.insert   : k -> v -> Dict k v -> Dict k v
Dict.remove   : k -> Dict k v -> Dict k v
Dict.keys     : Dict k v -> List k
Dict.values   : Dict k v -> List v
Dict.size     : Dict k v -> Int
Dict.singleton : k -> v -> Dict k v
Dict.isEmpty  : Dict k v -> Bool
Dict.member   : k -> Dict k v -> Bool
Dict.update   : k -> (Maybe v -> Maybe v) -> Dict k v -> Dict k v
Dict.map      : (k -> v -> w) -> Dict k v -> Dict k w
Dict.filter   : (k -> v -> Bool) -> Dict k v -> Dict k v
Dict.foldl    : (k -> v -> b -> b) -> b -> Dict k v -> b
Dict.partition : (k -> v -> Bool) -> Dict k v -> { yes: Dict k v, no: Dict k v }
Dict.union    : Dict k v -> Dict k v -> Dict k v
Dict.intersect : Dict k v -> Dict k v -> Dict k v
Dict.diff     : Dict k v -> Dict k v -> Dict k v
```

`Dict.update` calls the function with `Just` the current value, or `Nothing`
when the key is absent:

- `Just` keeps the key's position (a new key is appended).
- `Nothing` removes the key.

`Dict.union a b` prefers the first dict's value and order, then appends the
keys only `b` has. `map`, `filter`, `partition`, `intersect` and `diff` keep
key order; `foldl` walks it.

#### Keys

A key may be of any type `==` works on: a `List`, a record, a `Time`, but
not a function or a type containing one. Two keys are the same key when
they are `==`:

- A `Time` key is its instant (the same moment written in two time zones is
  one key).
- `Html` keys are compared by their markup (`<p>ab</p>` and
  `<p>{Html.text "a"}{Html.text "b"}</p>` are one key).

A `Dict` keeps its keys in the order they were first inserted; `==` on two
dicts ignores that order.

```alder eval=Just 2
Dict.fromList [{ key: "a", value: 1 }]
  |> Dict.insert "b" 2
  |> Dict.get "b"
```

## Date, Time

`Date` is a calendar date. `Time` is an instant (UTC).

```alder
Date.today     : Query Date
Date.fromText  : Text -> Maybe Date             -- "2026-10-08"
Date.toText    : Date -> Text                   -- "2026-10-08"
Date.format    : Text -> Date -> Text           -- Date.format "Jan 2, 2006" d
Date.addDays   : Int -> Date -> Date
Date.diffDays  : Date -> Date -> Int
Date.year, Date.month, Date.day : Date -> Int
Date.fromParts : { year: Int, month: Int, day: Int } -> Maybe Date
Date.toParts   : Date -> { year: Int, month: Int, day: Int }
Date.epoch     : Date                           -- 1970-01-01
Date.weekday   : Date -> Int                    -- 1 = Monday ... 7 = Sunday
Date.addMonths : Int -> Date -> Date
Date.addYears  : Int -> Date -> Date

Time.now       : Query Time
Time.toDate    : Time -> Date
Time.fromDate  : Date -> Time                   -- midnight UTC
Time.format    : Text -> Time -> Text
Time.ago       : Time -> Time -> Text           -- Time.ago now then = "3 hours ago"
Time.toMillis  : Time -> Int                    -- milliseconds since 1970-01-01 00:00 UTC
Time.fromMillis : Int -> Time
Time.addMillis : Int -> Time -> Time
Time.diffMillis : Time -> Time -> Int           -- Time.diffMillis a b = b − a, in milliseconds
```

`Date.diffDays a b` is the number of days from `a` to `b`, that is `b − a`:
`Date.diffDays monday wednesday` is `2`, and it is negative when `b` comes
first.

`Date.fromParts` is `Nothing` for an impossible date (month 13, Feb 30); it
never rolls over into the next month. `Date.weekday` is ISO-numbered, so
Monday is `1` and Sunday is `7`, and `weekday d >= 6` means weekend.
`Date.addMonths` clamps the day to the target month's length: one month after
January 31 is February 28 (29 in a leap year). `Date.addYears 1` on February 29
gives February 28.

```alder eval=Just "2026-10-09"
Date.fromText "2026-10-08"
  |> Maybe.map (Date.addDays 1)
  |> Maybe.map Date.toText
```

## Html

```alder
Html.unsafe : Text -> Html                -- unescaped markup; never for user text
Html.none  : Html                             -- renders nothing
Html.text  : Text -> Html
Html.fragment : List Html -> Html         -- the nodes side by side, no wrapper element
Html.toText : Html -> Text                -- the markup File.html serves
Url.external : Text -> Route                    -- external URL for href, src or Response.redirect
Url.absolute : Route -> Text                    -- the route's URL with the site's address in front
```

`Html.toText` takes an `Html`, rendered as plain markup: the links and
buttons it holds are not followed, because no page is running to receive
them.

`Url.external` keeps only an absolute `http:`/`https:` URL, a path of this site (not `//host` or
`/\host`) or a fragment (`#top`); anything else (`javascript:`, `data:`, `mailto:`, relative
paths, control characters) becomes `#`, and a redirect to one is a 500. A link or
`Response.redirect` built from user input can never run script.

#### Comparing routes

A `Route` compares by its address: a defaulted parameter is left out of the URL.

#### `Url.absolute`

`Url.absolute r` is the route's full URL, for a program that is not a
browser: a link in an email, or a URL another service will fetch. It is
the text `href={r}` renders (`/todos?filter=Active`, a defaulted parameter
left out, the query encoded) with the site's public address in front:
`https://todos.example.com/todos?filter=Active`.

The address comes from `ALDER_PUBLIC_URL` (chapter 11, `alder build`), which
`alder deploy` sets to the address it serves (chapter 16); without it the
server uses its own listening address, `http://localhost:8080` by default.

A route whose URL is not a path of this site is left as it is:

- `Url.absolute (Url.external "https://example.com/a")` is
  `https://example.com/a`.
- A fragment `#top` stays `#top`, and a sanitised `#` stays `#`.
- `Url.external "/static/app.css"` gets the address in front.

The address is set as the server starts, before the app's own values are
computed, and does not change while it serves, so `Url.absolute` is an
ordinary function, usable in a `view` and in a top-level value (chapter 11
has the one exception, a port 0).

There is no way to build a full URL from text: the only input is a `Route`,
so a link to a route that does not exist is still a type error. In a test
the address is always `http://localhost:8080` (chapter 10).

#### Time and milliseconds


## File, Bytes

`Bytes` is the type of a non-text body; `Text.toBytes` makes one.

```alder
File.text       : { contentType: Text, body: Text } -> File      -- a 200 answer
File.bytes      : { contentType: Text, body: Bytes } -> File     -- a 200 answer
File.html       : Html -> File                             -- text/html; charset=utf-8, rendered verbatim: no doctype unless the node has one
File.json       : Json.Value -> File                             -- application/json: Json.encode of the value
File.withStatus : Int -> File -> File                            -- 200..599
File.withHeader : { name: Text, value: Text } -> File -> File    -- Content-Disposition, Cache-Control, ETag, Last-Modified, Retry-After, Content-Language, X-…
File.status      : File -> Int
File.contentType : File -> Text
File.body        : File -> Bytes

Bytes.length    : Bytes -> Int                                   -- the number of bytes
Bytes.toText    : Bytes -> Maybe Text                            -- Just when the bytes are valid UTF-8
Bytes.empty     : Bytes
Bytes.isEmpty   : Bytes -> Bool
Bytes.slice     : Int -> Int -> Bytes -> Bytes                   -- same index rules as Text.slice, in bytes
Bytes.append    : Bytes -> Bytes -> Bytes
Bytes.concat    : List Bytes -> Bytes
Bytes.toList    : Bytes -> List Int                              -- each 0..255
Bytes.fromList  : List Int -> Bytes                              -- the low eight bits of each Int
```


## Query, Task, Db, Session, Log, Time, Random, Http

See chapters 7 and 8.

### Column methods

The column methods, written on a column in place (`Todo.title.contains q`),
are not functions of a module; the compiler knows them:

| Method | Columns | Type (for a column of type `a` in table `t`) |
|---|---|---|
| `.eq`, `.ne` | any | `a -> Condition t` |
| `.lt`, `.le`, `.gt`, `.ge` | ordered types (below) | `a -> Condition t` |
| `.contains` | `Text` and its brands | `Text -> Condition t` (substring, ignores A–Z case) |
| `.startsWith` | `Text` and its brands | `Text -> Condition t` (prefix, ignores A–Z case) |
| `.in` | any | `List a -> Condition t` |
| `.isNothing`, `.isJust` | `Maybe` | `Condition t` |
| `.asc`, `.desc` | any | `Order t` |

The ordered column types, for `.lt`, `.le`, `.gt` and `.ge`, are `Int`,
`Float`, `Text`, `Date`, `Time`, brands, and unions without payloads.

### Scoped operations: `mine`

The scoped operations of `Db`, for a table with an `@owner` column, used
only in the members of a route that `requires user` (chapter 6, "Rules";
chapter 8, "Your own rows"). Each acts for the signed-in user of the
running request or test, and without one fails with "`Db.mine Todo: no
signed-in user`" (the operation and table as written):

| Function | Type |
|---|---|
| `Db.mine` | `Table t -> Select t` |
| `Db.getMine` | `Table t -> Id t -> Query t` |
| `Db.updateMine` | `Table t -> Id t -> (t -> t) -> Task Unit` |
| `Db.deleteMine` | `Table t -> Id t -> Task Unit` |
| `Db.insertMine` | `Table t -> Insert t -> Task (Id t)` |

What each does:

- **`Db.mine`** gives the rows whose owner is the user; a `Select` like
  `Db.from t`.
- **`Db.getMine`** gives the user's row; not found when it is missing or
  someone else's.
- **`Db.updateMine`** changes the user's row; not found otherwise; never
  changes the owner.
- **`Db.deleteMine`** deletes the user's row; not found otherwise.
- **`Db.insertMine`** inserts a row owned by the user; the row is a record
  literal in the call, without the owner column.

### Scoped operations: partitions

The same operations for any partition key (M15): a table with a
`@partition k` column (`@owner` is `@partition user`), used only where
the session's `k` is narrowed, in the members of a route that `requires k`
(chapter 6, "Partitions"). The `mine` family above is these with the key
`user`, written for tables partitioned by `user`:

| Function | Type |
|---|---|
| `Db.scoped` | `Table t -> Select t` |
| `Db.getScoped` | `Table t -> Id t -> Query t` |
| `Db.updateScoped` | `Table t -> Id t -> (t -> t) -> Task Unit` |
| `Db.deleteScoped` | `Table t -> Id t -> Task Unit` |
| `Db.insertScoped` | `Table t -> Insert t -> Task (Id t)` |

What each does:

- **`Db.scoped`** gives the rows of the session's partition; a `Select` like
  `Db.from t`.
- **`Db.getScoped`** gives the row in the session's partition; not found
  when it is missing or in another.
- **`Db.updateScoped`** changes the row in the partition; not found
  otherwise; never changes the partition column.
- **`Db.deleteScoped`** deletes the row in the partition; not found
  otherwise.
- **`Db.insertScoped`** inserts a row in the session's partition; the row is
  a record literal in the call, without the partition column.

### Without the key

Without the key they fail ("Db.insertScoped Project: the session has no org"). On a siloed
table (chapter 8, "Silos") every `Db` operation needs the key.

`Query.notFound` and `Task.notFound` (both
`: Query a` / `: Task a`, no arguments) stop the effect and answer 404 with
`App.notFound`. A `Task` that ends in `Task.notFound` is rolled back like
any failed `Task`.

`Query.redirect : Route -> Query a` stops the effect and
answers a 303 to the route value, as `Response.redirect r` does (chapter 7,
"Failure"): a GET route's `load` uses it to send the browser elsewhere.

## Response, Route

`Response` and `Route` are Prelude types (chapter 6, "`Response`"). An `update` builds its
`Response` with:

```alder
Response.redirect : Route -> Response                 -- 303, to the GET route's URL
Response.render   : Route -> Response                 -- 200, that GET route's view, as one element
Response.refresh  : Response                          -- 200, the page the browser is on, again
Response.invalid  : Text -> Response                  -- 422, that page, form kept, the text in <Errors form />
Response.toast    : Text -> Response -> Response      -- a toast on any response
Response.success  : Text -> Response -> Response      -- a toast of kind success
Response.warning  : Text -> Response -> Response      -- a toast of kind warning
Response.error    : Text -> Response -> Response      -- a toast of kind error
```

`redirect` and `render` take a GET route (E0045 otherwise). `invalid` is a successful result: its
`Task` commits. `Response.status`, `header`, `json`, `file`, `remove`, `patch` and `stream` do not
exist yet. `Route` and `Response` compare with `==`. `Notice.with`, `success`, `warning` and
`error` are the deprecated old names of the toast builders, on a `Nav`.

## Test

Only a `test` or a test helper may use these (chapter 10, "Test
helpers": a value that uses one becomes a helper); where no test can
reach, such as a route's `view`, each is E0040 "… is only available in a
`test` or a test helper".

- `Test.run : Task a -> Task { value: Result Text a, trace: List Text }`
  runs a task that may fail: `value` is `Ok` with its result or `Err`
  with the failure's message (its writes undone), and `trace` is the
  steps it took (chapter 10, section 4).

- `Test.trace : Task (List Text)` is the steps the test has taken so far
  (chapter 10, section 5).

- `Test.fake Stripe : Task Unit` installs the `fake` of capability
  `Stripe` for the rest of the test (chapter 10, section 8).

  It is a special form, not a function: its argument is a capability's name
  (`Stripe` or `Payments.Stripe`), whose module declares a `fake`, and
  never an expression. It has no type of its own to pass around, so
  `Test.fake` cannot be applied to a variable, piped into, or left
  without its argument.

- `Test.member ann acme : Task Unit` inserts the `via` table's row making the user a member
  of the key (other columns `Nothing` or their default; E0003 if one is required).
- `Test.fetch : Route -> Task { status: Int, contentType: Text, body: Bytes, headers: List { name: Text, value: Text } }`
  requests a file page: the raw answer, redirects not followed, headers sorted by name.

## Fuzz

The generators of property tests (chapter 10, section 12). Only a `test` body
may be a `Fuzz.Property`; a `Fuzz.Fuzzer a`, a description of how to draw
an `a` from random choices, may be a value anywhere, so a generator is
written once, with a signature, and shared.

```text
Fuzz.forAll     : Fuzz.Fuzzer a -> (a -> Bool) -> Fuzz.Property
Fuzz.forAllTask : Fuzz.Fuzzer a -> (a -> Task Bool) -> Fuzz.Property
Fuzz.any        : Fuzz.Fuzzer a
Fuzz.constant   : a -> Fuzz.Fuzzer a
Fuzz.bool       : Fuzz.Fuzzer Bool
Fuzz.int        : Fuzz.Fuzzer Int
Fuzz.intRange   : { min: Int, max: Int } -> Fuzz.Fuzzer Int
Fuzz.float      : Fuzz.Fuzzer Float
Fuzz.floatRange : { min: Float, max: Float } -> Fuzz.Fuzzer Float
Fuzz.text       : Fuzz.Fuzzer Text
Fuzz.textOf     : { min: Int, max: Int } -> Fuzz.Fuzzer Text
Fuzz.char       : Fuzz.Fuzzer Text
Fuzz.list       : Fuzz.Fuzzer a -> Fuzz.Fuzzer (List a)
Fuzz.listOf     : { min: Int, max: Int } -> Fuzz.Fuzzer a -> Fuzz.Fuzzer (List a)
Fuzz.maybe      : Fuzz.Fuzzer a -> Fuzz.Fuzzer (Maybe a)
Fuzz.result     : Fuzz.Fuzzer e -> Fuzz.Fuzzer a -> Fuzz.Fuzzer (Result e a)
Fuzz.pair       : Fuzz.Fuzzer a -> Fuzz.Fuzzer b -> Fuzz.Fuzzer { first: a, second: b }
Fuzz.dict       : Fuzz.Fuzzer k -> Fuzz.Fuzzer v -> Fuzz.Fuzzer (Dict k v)
Fuzz.oneOf      : List (Fuzz.Fuzzer a) -> Fuzz.Fuzzer a
Fuzz.oneOfValues : List a -> Fuzz.Fuzzer a
Fuzz.frequency  : List { weight: Int, fuzzer: Fuzz.Fuzzer a } -> Fuzz.Fuzzer a
Fuzz.map        : (a -> b) -> Fuzz.Fuzzer a -> Fuzz.Fuzzer b
Fuzz.map2       : (a -> b -> c) -> Fuzz.Fuzzer a -> Fuzz.Fuzzer b -> Fuzz.Fuzzer c
Fuzz.andThen    : (a -> Fuzz.Fuzzer b) -> Fuzz.Fuzzer a -> Fuzz.Fuzzer b
Fuzz.filter     : (a -> Bool) -> Fuzz.Fuzzer a -> Fuzz.Fuzzer a
Fuzz.fromRandom : Random.Generator a -> Fuzz.Fuzzer a
```

#### `Fuzz.any` and `forAll`

`Fuzz.any` is a special form, like `Test.fake`: the compiler gives it the
generator it derives from the type the context asks for (chapter 13), so
that type must be concrete where it is written (E0038).

`Fuzz.forAll` and `Fuzz.forAllTask` are legal only as the body of a `test`
(E0039).

#### What the generators draw

- **`int`** is mostly small and sometimes extreme.
- **`float`** is finite (no `NaN`, no infinity).
- **`text`** is mostly short ASCII, sometimes Unicode, sometimes empty, and
  `textOf` and `char` count code points.
- **`list`** is biased to short lists, and `listOf` bounds the length.
- **`filter`** is subject to the health check: a predicate that rejects over
  90% of the first 50 draws fails the test (chapter 10).

#### `Fuzz.fromRandom`

`Fuzz.fromRandom g` turns a seeded generator (see "Random") into a
fuzzer: it draws a random seed and gives `g`'s value at it.

Shrinking lowers the seed toward `Random.seed 0`, never the value inside
`g`, so a failure shrinks to a nearby seed, not to the simplest value. A
`Fuzz.Fuzzer` written with `Fuzz.*` shrinks better and is the first choice;
`fromRandom` is for reusing a generator the app already has.

# 10. Testing

A test is a piece of the program that checks another piece and says yes or
no. In Alder a test is a declaration, written in the module it tests, after
the code, and `alder test` runs every test in the app.

This chapter goes in this order:

- the simplest kind, a test of a pure function;
- the report `alder test` prints;
- tests of code that reads and writes the database;
- replacing a capability with a fake written in Alder;
- driving the app through an in-memory browser as a user would;
- measuring the tests with mutants (`alder test --mutate`);
- checking statements about all inputs instead of examples (properties,
  section 12);
- what cannot be tested yet.

## 1. A test is a `Bool`

`test "name" = expression` declares a test. When its body is not a block,
the body must be a `Bool`: `True` means the test passed. The to-do app's
`Todo` module (`examples/todos/app/Table/Todo.alder`) ends with one, which
checks that `countOpen` counts only the todos that are not done:

```alder
test "countOpen ignores done todos" =
  Todo.countOpen [
    { id: Todo.TodoId "a", title: Text.NonEmpty "x", done: False, due: Nothing, owner: User.UserId "u", created: Time.epoch },
    { id: Todo.TodoId "b", title: Text.NonEmpty "y", done: True, due: Nothing, owner: User.UserId "u", created: Time.epoch },
    { id: Todo.TodoId "c", title: Text.NonEmpty "z", done: False, due: Nothing, owner: User.UserId "u", created: Time.epoch },
  ] == 2
```

Inside `Todo` the names are written unqualified (`countOpen`,
`TodoId "a"`), and that is how the file has them; the blocks of this
chapter are checked as a module of their own, so they qualify them as any
other module must.

A test of another function looks the same. This one, not in the example,
checks that the `Active` filter leaves out a done todo:

```alder
test "matches Active excludes done" =
  not (Todo.matches Active { id: Todo.TodoId "a", title: Text.NonEmpty "x", done: True, due: Nothing, owner: User.UserId "u", created: Time.epoch })
```

Two rules are particular to tests:

- A test may use any module's brand constructors. Outside `Todo`, nothing
  else may write `Todo.TodoId "a"`: a `TodoId` must come from a route, a
  form or a row (chapter 4). A test needs to make up values, so it may.
- A test has no name you can call. It is not a value; only `alder test`
  runs it.

Because a route's `load`, `view` and `update` are functions that return
descriptions, much of a route can be tested this way. `view` returns HTML,
and two HTML values are equal when they render to the same markup:

```alder eval=True
<p>{Html.text "a"}</p> == <p>a</p>
```

The comparison is of the markup, not of how the values were built: two
text pieces side by side are the same markup as one piece holding both,

```alder eval=True
<p>{Html.text "a"}{Html.text "b"}</p> == <p>ab</p>
```

so a test of `view` passes exactly when the two sides it prints are the
same text.

## 3. Testing effects

A `Task` cannot be compared with `==` (section 13 says why), so a test of
`update` cannot just compare its result. Instead it runs the task and
looks at what happened.

A test whose body is a block, `{ … }`, is an **effect test**: the block is
a `Task Bool`, with `let x <-` lines like any `Task` (chapter 7), and it
ends with `Task.succeed` of the verdict. A block that ends in a plain
`Bool` is an error; the compiler's hint says to end it with
`Task.succeed (…)`.

The to-do list's tests are the last thing in `app/Todos/Index.alder`
(chapter 2 shows them in place). Written in another module, as the
blocks of this chapter are, the route module's declarations are named
through its import (`Index.Toggle.update`; chapter 6, "Names"):

```alder
test "Toggle flips done" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Todo { title: Text.NonEmpty "Milk", due: Nothing, owner: me }
  let _ <- Test.login { user: Just me }
  let r <- Test.run (Index.Toggle.update { id: id } { user: me, role: Member })
  let after <- Db.get Todo id
  let done = Maybe.map (\t -> t.done) after
  Task.succeed ({ trace: r.trace, done: done } == { trace: ["Db.updateMine Todo"], done: Just True })
}
```

Line by line:

1. Insert a user. The database starts empty, and `Todo.owner` is a
   reference to a `User` row (`@ref User`, chapter 8), so a todo whose
   owner does not exist cannot be inserted: the test would fail with
   `error: Todo.owner: no User with that id`. A test builds every row it
   needs, the rows they refer to included. `Db.insert` returns the new
   row's id, `me`.

2. Insert a todo owned by `me`; its id is `id`.

3. Sign `me` in. `Toggle` uses `Db.updateMine` (chapter 6, "Rules"),
   which acts for the user of the session the test is running with, and
   `Test.login` sets that session (and the browser's cookie, section 9).
   Without this line the step would fail with `Db.updateMine Todo: no
   signed-in user`.

4. Run the `Toggle` route's `update` under `Test.run` (section 4) and
   call the outcome `r`. `Toggle.update` takes the route's parameters
   (`{ id: id }`, a `Toggle.Params`, the id the URL would carry) and the
   session; no `load` runs before it, and no form is decoded.

   The `Toggle` route `requires user`, so its `Session` is a type of its
   own whose `user` is a `UserId`, not a `Maybe` (chapter 6). The record
   literal `{ user: me, role: Member }` names every field, the role too (a
   literal gets no defaults: chapter 6, "Rules"), and is that type because
   it is written where `update` expects one. Bound to a name first by a
   `let` without an annotation, it would be a plain record, which `update`
   does not accept.

   This record is `update`'s `session` argument, which the route's own
   code may read; the scoped operations do not read it (step 3, and "Tests
   that use the rules", below).

5. Read the todo again, and take its `done` (`after` is a `Maybe Todo`:
   `Nothing` if the row were gone).

6. The verdict, as one `==` between two records: `update` made one
   step, the scoped write, and the stored todo is now done. Putting everything
   the test checks into one record, rather than joining several `==`
   with `&&`, means a failure prints both records whole (section 2).

The second test is the ownership rule of chapter 6, "Rules": a user who
posts `Toggle` with someone else's todo id gets a 404 and changes
nothing. The example tests it through the browser (section 9, "A
stranger's todo"); here it is through `update` itself, the stranger signed
in with `Test.login`:

```alder
test "Toggle by a stranger is not found" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Todo { title: Text.NonEmpty "Milk", due: Nothing, owner: me }
  let stranger = User.UserId "someone-else"
  let _ <- Test.login { user: Just stranger }
  let r <- Test.run (Index.Toggle.update { id: id } { user: stranger, role: Member })
  let after <- Db.get Todo id
  let done = Maybe.map (\t -> t.done) after
  Task.succeed ({ value: r.value, trace: r.trace, done: done } == { value: Err "not found", trace: ["Db.updateMine Todo"], done: Just False })
}
```

The session's user, `stranger`, is not a row and need not be: a session
holds an id, not a reference the database checks. `Db.updateMine` finds
no todo with that id whose owner is `stranger`, and stops with
`Task.notFound`; nothing is written. If `Toggle` used the unscoped
`Db.update` instead, this test would say exactly what went wrong:

```text
$ alder test examples/todos -run 'by a stranger'
FAIL Todos.Index "Toggle by a stranger is not found"
  left:  { value: Ok Response.refresh, trace: ["Db.update Todo"], done: Just True }
  right: { value: Err "not found", trace: ["Db.updateMine Todo"], done: Just False }
  trace: ["Db.insert User", "Db.insert Todo", "Test.login", "Test.run", "Db.update Todo", "Db.get Todo"]
0 passed, 1 failed
```

The stranger's `Toggle` succeeded, the update ran, and the todo is done.

### Tests that use the rules

The scoped operations of chapter 6 (`Db.mine`, `Db.getMine`,
`Db.updateMine`, `Db.deleteMine`, `Db.insertMine`) act for the session
the test is running with, never for a session record the test passes
to a function. What sets that session:

- **`Test.login s`** sets it to `s`, as well as the browser's cookie
  (section 9). This is the way to choose the user.

- **`Session.put`**, in the test or inside an `update` it runs, changes
  it for every later step: a scoped operation after it acts for the
  new user.

- **A login through the login route's form** (`Test.submit` of its
  `Credentials`) signs the user into the **browser** only: the route's
  `Session.put` ran in the request, and the router stored the result in
  the cookie. The browser's next requests act for that user; the test's
  own steps (a `Test.run (Toggle.update …)`, a `Db.mine` in the test body)
  do not.

The route checks `requires` and the role (chapter 6); a test of them
goes through the browser (`Test.open`, `Test.click`, section 9); a
direct `update` call is not checked.

So a test that calls `update` or `load` itself, with a session record,
signs in with `Test.login` first; the record is still `update`'s
argument, and must have the route's `Session` type, but it does not choose
the user.

Without a signed-in user a scoped step fails, saying so (`Err
"Db.updateMine Todo: no signed-in user"`); it never answers as if the user
had no rows. A test may use the scoped operations itself too, after
`Test.login`.

With a key (chapter 6, "Partitions"), `Test.login { user: Just ann, org: Just acme }`
needs the membership first, as `Session.put` does: `Test.member ann acme` inserts it. Each org a
test touches has its own in-memory database (a silo); insert another org's siloed rows before
`Test.login`, as production refuses them in a request of this org. A crafted switch is a direct
call: `Test.run (Open.update { org: globex } session)` is `Err "Session.put org: no
Membership for this user and org"`, which kills the mutant `membership dropped`.

**Where an effect test runs.** Each effect test gets a fresh, empty
in-memory SQLite database with the app's tables; its block runs as one
`Task`, in one transaction, and sees its own writes. `Time.now` and the
`@created` a `Db.insert` fills read a clock that starts at
`2026-01-01T00:00:00Z` and advances one second per read, so the rows a
test creates are ordered. The app's capabilities are wired to their
adapters as in `alder build`; one without an adapter is a warning, and
calling it unfaked fails the test ("capability Auth.Login.Password is
not wired in tests"); `Test.fake` replaces one (section 8). The test's
session starts as `App`'s, every field at its default, and its browser
(section 9) holds no page and no cookie.

A pure test (section 1) touches none of this: it is an expression,
evaluated once.

## 4. `Test.run`: running a task that may fail

`Toggle.update` for another user's todo ends in `Task.notFound`. If the
test simply wrote `let response <- Index.Toggle.update …`, that failure
would be the test's failure, as a failing step is any task's failure, and
the test could never check that the 404 happened.

`Test.run` catches it. Its type,
from the standard module `Test` (`stdlib/Test.alder`), is
`Test.run : Task a -> Task { value: Result Text a, trace: List Text }`.

`Test.run t` runs the task `t` inside the test and always succeeds, with
a record:

- `value` is `Ok v` when `t` succeeded with `v`, and `Err msg` when it
  failed: `msg` is the text of a `Task.fail msg`, `"not found"` for
  `Task.notFound` (and `Query.notFound`) — the failures that become a 404
  page in the running app — or, when a capability's Go code returned an
  error, the call and the error: `"Password.verify: connection refused"`.

  A write route's `update` gives a `Result Text Response`, and `Response`
  values compare with `==`, so `r.value == Ok Response.refresh` works too.

- `trace` is the list of steps `t` performed (section 6).

When `t` fails, what it did is undone, and only that: its writes are
rolled back and the session is put back as it was before `Test.run`; the
writes the test made before it stay.

The database can do this because of
a **savepoint**: a named mark inside the open transaction. `Test.run`
sets one before `t` starts; on failure `ROLLBACK TO` the mark undoes
exactly the writes after it, and on success `RELEASE` forgets the mark,
and `t`'s writes become part of the test's.

Here is the second test as the database sees it:

```text
test "Toggle by a stranger is not found"   (a fresh database in memory)

  BEGIN                                     first database step: the transaction opens
  │ Db.insert User
  │ Db.insert Todo
  │ Test.login                              the test's session: the stranger
  │ Test.run ── SAVEPOINT alder_t1
  │             │ Db.updateMine Todo        no todo with that id is the stranger's
  │             │ Task.notFound             update fails
  │             └ ROLLBACK TO alder_t1      undo update's writes (it made none)
  │               RELEASE alder_t1          r.value = Err "not found"
  │ Db.get Todo                             the todo is still there, not done
  COMMIT                                    the verdict was produced
                                            then the database is thrown away
```

Savepoints nest, so `Test.run` inside `Test.run` works the same way. A
test that inserts inside a failing `Test.run` can see the insert undone:

```alder
test "a failed run is undone" = {
  let r <- Test.run {
    let _ <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
    Task.fail "changed my mind"
  }
  let n <- Db.from User |> Db.count
  Task.succeed ({ value: r.value, users: n } == { value: Err "changed my mind", users: 0 })
}
```

Like the route's tests, this one ends with one `==` between two records.

## 5. `Test.trace`: the steps so far

`Test.trace : Task (List Text)` is the list of steps the test itself has
performed so far, without the `Test.trace` step:

```alder
test "the steps so far" = {
  let _ <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Db.from Todo |> Db.count
  let steps <- Test.trace
  Task.succeed (steps == ["Db.insert User", "Db.count Todo"])
}
```

`Test.run` and `Test.trace` are for tests only. A value that uses one
is a *test helper* (section 9, "Test helpers"), which tests may call and
production code may not:

```alder
steps : Task (List Text)
steps = Test.trace
```

Where no test can reach (a route's `load`, `view` or `update`, `App`,
a fake's entry, a brand's check, a field's default) it is E0040, so no
route can depend on them:

```alder error=E0040 "`Test.trace` is only available in a `test` or a test helper: a field default is not a helper"
type Probe = { steps: Task (List Text) = Test.trace }
```

## 8. Faking a capability

A capability's adapter is Go code that reaches outside the program
(chapter 7). A test of a checkout should not charge a real card, and a
test of a page should not depend on how Markdown is rendered.

For those, a module can declare a **fake**: a second implementation of a
capability, written in Alder beside the capability, that only tests
use. It answers every entry of the capability, and it can keep state in
tables of its own, so a charge and then its refund behave as they would
against the real service.

### A fake without state: `Markdown`

The blog's `app/Markdown.alder` declares the capability and, after it,
its fake:

```alder
module Markdown

-- Rendering a post's body is outside the language: a capability implemented in Go
-- (adapters/markdown/markdown.go), which builds the HTML node by node, so a post cannot inject markup.
capability Markdown {
  render : Text -> Query Html,
}

-- In a test, `let _ <- Test.fake Markdown` replaces the adapter with this: the body as plain
-- text, so a test about a page asserts on the page, not on how Markdown is rendered.
fake Markdown {
  render text = Query.succeed (Html.text text)
}
```

`fake Markdown { … }` names the capability and holds one definition for
each of its entries, here `render`, without a signature: its type is
the entry's, so `text` is a `Text` and the body a `Query Html`.

A test installs the fake with a step, `let _ <- Test.fake Markdown`.
From then until the test ends, every call of `Markdown.render` (in
`load`, in a function it calls, anywhere) runs the fake instead of the
adapter.

These two tests call the entry with the fake and without it:

```alder
module Preview

import Markdown

test "the fake leaves the text as it is" = {
  let _ <- Test.fake Markdown
  let body <- Markdown.render "Not *yet*."
  let steps <- Test.trace
  Task.succeed ({ body: body, steps: steps } == { body: Html.text "Not *yet*.", steps: ["Test.fake Markdown", "Markdown.render"] })
}

test "the adapter renders the Markdown" = {
  let body <- Markdown.render "Not *yet*."
  let steps <- Test.trace
  Task.succeed ({ body: body, steps: steps } == { body: <p>Not <em>yet</em>.</p>, steps: ["Markdown.render"] })
}
```

Added to a copy of the blog, both pass. Their traces show the shape of
every faked trace:

- installing is a step, `"Test.fake Markdown"`;
- each call is traced by the entry's usual name, as a call to the adapter
  is;
- the steps a fake takes inside an entry follow that entry's step.

The blog's `Posts.Show` has one test of each kind: one through the real
adapter, so the wiring is tested; one with the fake, checking that
`load` puts the body in its Model and that `"Markdown.render"` is in its
trace. The rendering itself is tested in Go.

That is the usual split:
fake a capability in the tests of the code that uses it, and keep one
test through the real adapter.

### A fake with state: a card processor

A card processor remembers: a refund must name a charge it made. This
module, `Payments`, and `Checkout` after it were run as an app of their
own, `shop`. The fake keeps each charge as a row:

```alder
module Payments

type ChargeId = brand Text

-- The card processor. In production, Go in adapters/payments/ implements it, func Stripe (chapter 7).
capability Stripe {
  charge : { cents: Int.Positive } -> Task (Result Text ChargeId),
  refund : { id: ChargeId } -> Task (Result Text Unit),
}

-- In a test, `Test.fake Stripe` puts this in the adapter's place: a processor in memory that
-- keeps each charge as a row of its own table.
fake Stripe {
  table Charge {
    id: ChargeId @id,
    cents: Int,
    refunded: Bool = False,
  }

  -- A charge over 1000.00 fails like a processor that answers with an error.
  charge args = {
    let id <- Db.insert Charge { cents: args.cents }
    if args.cents > 100000 then Task.fail "card declined" else Task.succeed (Ok id)
  }

  refund args = {
    let found <- Db.get Charge args.id
    match found {
      Just c -> {
        let _ <- Db.update Charge c.id (\x -> { x | refunded: True })
        Task.succeed (Ok ())
      },
      Nothing -> Task.succeed (Err "no such charge"),
    }
  }
}

test "a refunded charge is marked refunded" = {
  let _ <- Test.fake Stripe
  let charged <- Stripe.charge { cents: Int.Positive 250 }
  let refunded <- match charged {
    Ok id -> Stripe.refund { id: id },
    Err e -> Task.succeed (Err e),
  }
  let found <- match charged {
    Ok id -> Db.get Charge id,
    Err e -> Query.succeed Nothing,
  }
  let row = Maybe.map (\c -> { cents: c.cents, refunded: c.refunded }) found
  Task.succeed ({ refunded: refunded, row: row } == { refunded: Ok (), row: Just { cents: 250, refunded: True } })
}
```

`table Charge { … }` is a table like any other (chapter 8), the record
type `Payments.Charge`, used with `Db`.

The entries are ordinary Alder
typed by the capability (`args` is `{ cents: Int.Positive }` in
`charge`); `charge` inserts before it decides, so the insert is there to
be undone when it fails. The test's trace is
`["Test.fake Stripe", "Stripe.charge", "Db.insert Charge", "Stripe.refund", "Db.get Charge", "Db.update Charge", "Db.get Charge"]`.

Code that uses the capability does not change; its tests install the
fake by the capability's name:

```alder
module Checkout

import Payments

-- The app's code calls the capability as always; it never names the fake.
pay : Int.Positive -> Task Text
pay cents = {
  let charged <- Stripe.charge { cents: cents }
  match charged {
    Ok id -> Task.succeed "paid",
    Err e -> Task.succeed ("refused: " ++ e),
  }
}

test "an order is paid" = {
  let _ <- Test.fake Stripe
  let said <- pay (Int.Positive 250)
  Task.succeed (said == "paid")
}

test "a declined charge leaves no row" = {
  let _ <- Test.fake Stripe
  let small <- pay (Int.Positive 250)
  let large <- Test.run (pay (Int.Positive 500000))
  let rows <- Db.from Payments.Charge |> Db.where (Payments.Charge.cents.eq 250) |> Db.count
  let all <- Db.from Payments.Charge |> Db.count
  Task.succeed ({ small: small, large: large.value, trace: large.trace, rows: rows, all: all } == { small: "paid", large: Err "card declined", trace: ["Stripe.charge", "Db.insert Charge"], rows: 1, all: 1 })
}
```

From `Checkout` the table is `Payments.Charge`, its columns
`Payments.Charge.cents`.

The second test charges 500000: `charge`
inserts, then `Task.fail "card declined"`, which under `Test.run`
(section 4) is `Err "card declined"`; the savepoint undid the insert, so
one row is left.

> **`Err` and `Task.fail`.** A `Result` `Err` is an answer `pay` handles;
> `Task.fail` imitates a processor that does not answer.

All three tests
pass. Outside `Test.run`, that charge fails the test.

### The rules

#### The declaration

- A fake is declared in the module that declares its capability. A fake
  elsewhere is E0020. The to-do app's login tests (section 9) use a
  `Password` that accepts any user who has a row, whatever the password;
  that fake is in `app/Auth/Login.alder`, beside `capability Password`.
  Written in the module that wants it instead, it is refused:

```alder error=E0020 "`fake Password` must be declared in the module that declares `capability Password`"
fake Password {
  verify args = {
    let found <- Db.from User |> Db.where (User.email.eq args.email) |> Db.first
    Task.succeed (Maybe.map (\u -> u.id) found)
  }
}
```

- One fake per capability: a second is E0009, "`fake Stripe` is already
  declared at line 7" (the line of the first). Inside a fake, a second
  definition of one entry is E0009 too: "`charge` is already defined in
  `fake Stripe` at line 8" (the first definition's line).

- It defines every entry of the capability, and nothing else. A missing
  entry is E0003, at the fake, with the type it must have; an entry the
  capability does not have is E0003 "`fake Stripe` defines `cancel`,
  which `capability Stripe` has no entry for":

```alder error=E0003 "`fake Stripe` lacks `refund` (`{ id: ChargeId } -> Task (Result Text Unit)`)"
type ChargeId = brand Text

capability Stripe {
  charge : { cents: Int } -> Task (Result Text ChargeId),
  refund : { id: ChargeId } -> Task (Result Text Unit),
}

fake Stripe {
  charge args = Task.succeed (Ok (ChargeId "ch_1"))
}
```

- An entry is written `name params = body`, with no signature (a
  signature inside a fake is a parse error): its type is the entry's.
  Every error inside an entry is named after it: "in `fake Stripe`'s
  `charge`: expected Result Text Payments.ChargeId, got Text".

- A fake holds only `table` declarations and entry definitions, in any
  order. Types the fake needs (`ChargeId`) are declared in the module.
  A top-level value cannot be named `fake`: E0001 "`fake` starts a fake
  declaration at column 1: rename the value".

- Entry bodies are ordinary Alder: `Db` on the fake's tables and the
  app's, values of their module and its imports, and the capability's
  other entries (inside `fake Stripe`, `Stripe.refund` is the fake's,
  never the adapter's; an entry that calls itself unconditionally
  recurses until the run stops with `fatal error: stack overflow`).

  Not `Session`, which an adapter does not have: E0003 "in `fake Stripe`'s
  `charge`: a fake cannot use `Session`: an adapter has no session".

#### The fake's tables

- A table of `fake Stripe` in module `Payments` is the record type
  `Payments.Charge`, as an app table of that module would be. It is
  exempt from E0018 (a table in a module named after it).

- Only a `test`, in any module, a test helper (section 9, "Test
  helpers") and the fake's own entries may use it, its type or its
  columns. A value `charges = Db.from Payments.Charge |> Db.count` in
  `Checkout` becomes a test helper.

  Anywhere else (a page's `load`,
  another fake's entry, a type declaration, an app table's `@ref`) it is
  E0021: "`Charge` is a fake's table: only tests and `fake Stripe` may use
  it".

- In another module, the table and its columns carry the module's name,
  `Payments.Charge` and `Payments.Charge.cents`, and that module must
  import `Payments`.

  Written bare, `Charge.cents.eq 250` is E0002
  "`Charge` is a table of `fake Stripe` in module `Payments`: write
  `Payments.Charge.cents`"; in a module without `import Payments`,
  `Payments.Charge` is E0002 "`Charge` is a table of `fake Stripe` in
  module `Payments`: add `import Payments`, then write `Payments.Charge`
  (columns: `Payments.Charge.column`)", with the import as its fix.

- A fake's table may `@ref` an app table (`user: User.UserId @ref User`):
  a fake can keep, say, which user each charge was for.

  Give that ref
  `@cascade` (or `@setNull`), so the app's deletes behave as they do in
  production, where the fake's table does not exist: with the default
  `@restrict`, deleting a user with a charge fails with
  ``User … is referenced by a row of Charge (`Charge.user`)``. An app
  table may not `@ref` a fake's (E0021).

- The tables exist only in test databases, created beside the app's
  tables (section 3). They are never in `migrations/schema.json` or a
  migration, so adding or changing a fake's table needs no migration.

#### `Test.fake`

- `Test.fake Stripe` is a `Task Unit`, available only in a `test` or a
  test helper like the rest of `Test` (section 9, "Test helpers"; in a
  fake's entry, E0040 "`Test.fake` is only available in a `test` or a
  test helper: a fake's entry is not a helper").

  Its argument is a capability's name, `Stripe` or
  `Payments.Stripe`, not an expression (E0003 "`Test.fake` wants a
  capability name, got an expression"), and the capability must have a
  fake (E0003 "`Test.fake` wants a capability with a `fake`: `Stripe` has
  none").

  When two imported modules both declare `Clock`, the bare name
  is E0002 "`Clock` is a capability of both `A` and `B`: write `A.Clock`
  or `B.Clock`", here and in a call (`A.Clock.now`).

- It replaces the capability for the rest of the test, in every module.
  Installed inside a `Test.run`, it stays after the run, even a failed
  one (installing is not a database write). Installing again gives a
  new copy of the fake; the rows in its tables stay.

- Without it, a test calls the app's adapter, if any (section 3).

- Its trace has the shape shown with `Markdown` above. A failing entry
  is a failing step: under `Test.run` it is `Err` with the entry's
  message, its writes undone; outside it, the test fails.

- After a `render` or an element fetch, the browser's element check
  (section 9) runs the route's `load` again, so an entry that `load`
  calls runs twice in that step. The second call's rows are undone with
  the check's savepoint and it is not in the trace; what the fake keeps
  outside its tables (the Go value `Test.fake` installs) sees both calls.

- Each test starts with a fresh database and its own capabilities, so a
  fake installed in one test, and its rows, are never seen by the next.

#### Where fakes exist

`alder test` compiles them, and so do `alder eval`
and `alder repl`, whose builds are test builds too (chapter 11). A
production build (`alder build`, `alder run`) contains no tests at all:
no `test`, no fake, no fake's table and no test runner. (Chapter 13
shows the generated Go.)

## 9. Testing through the browser

A test can also use the app the way a user does: open a page, fill in and
post its form, click its buttons, and read what the page then shows and
what the session then holds, through a browser that runs in memory. Each
step names a route (chapter 6), as a link, a form or a button does.

### The browser a test holds

Every effect test (section 3) has, beside its fresh database, its clock
and its capabilities, a **browser** of its own. It holds **the page**,
the document of the last page it was given (none before the first step),
and **a cookie**, the signed cookie that carries the session (chapter 6),
sent back with every request.

Before the first step there is no cookie,
so the session is `App`'s default, `{ user: Nothing }`.

Eleven steps act through it. Each is a `Task` (a step of the test's block,
written with `let x <-`), and each works only inside a `test`. `r` is a
route value written in place, with its parameters (`Todos.Index.Index {}`,
`Todos.Index.Toggle { id: id }`):

| Step | What it does | It gives |
|---|---|---|
| `Test.login s` | puts the session `s` in the cookie, as a login would | `Unit` |
| `Test.get r` | requests the GET route `r`; the test fails unless its page is shown | `{ model: Model, session: Session, doc: Doc, url: Text, status: Int }` |
| `Test.open r` | requests the GET route `r`, whatever the answer | `{ answer: Answer, session: Session, doc: Doc, url: Text, status: Int }` |
| `Test.fetch r` | requests the file route `r`; the raw answer, redirects not followed, headers sorted by name | `{ status: Int, contentType: Text, body: Bytes, headers: … }` |
| `Test.submit r f` | posts the record `f` through the form of the write route `r` | `{ answer: Answer, session: Session, doc: Doc, url: Text, status: Int }` |
| `Test.submitRaw r fields` | posts text fields through the form of the write route `r` | the same |
| `Test.click r` | presses the page's button for the route `r`: a write route is sent, a GET route is fetched into the page | the same |
| `Test.load r` | fetches the GET route `r` into the page, as the page's element with `load={r}` does when it appears or its timer ticks | the same |
| `Test.intersect r` | fetches the GET route `r` into the page, as the page's element with `intersect={r}` does when it scrolls into view | the same |
| `Test.input r` | fetches the GET route `r` into the page, as the page's field with `input=` does after typing; `r`'s query value stands for what was typed | the same |
| `Test.refresh` | loads the page held again, and fails unless it is the page held | the same |

In the results:

- `model` is what the route's `load` gave, exactly the one its `view` was
  given, and typed (`page.model.todos` is a `List Todo`);
- `answer` says what the request came to (`Answer`, below);
- `session` is `App`'s session after the step, what the cookie now carries
  (`r.session.user == Just me`);
- `doc` is the page the browser now holds, a `Doc`: a parsed HTML
  document, read with the queries below and compared with `==`;
- `url` is the address of that page, path and query (`"/?filter=Active"`);
- `status` is the code of the step's last response. After
  `Response.invalid` it is 422, after a missing page 404, and after a
  redirect it is the status of the page the browser then fetched (200).

`Test.fetch` holds no page, so its result has no `session`,
`doc` or `url`; its `status` is the raw answer's.

`Test.load`, `Test.intersect` and `Test.input` need exactly one element on
the page held that requests `r`'s path with that attribute. The query is
not compared, so a test can send a value the page did not render, as a
user types one. With none, the step fails:

```
error: the page /feed has no element with `intersect` requesting /feed/more
```

With several, it fails naming each. Nothing fires by itself: a badge that
polls is tested by calling `Test.load` again (chapter 6, "Patterns").

### A first browser test

The to-do list's tests end with three through the browser (chapter 2
shows them in place). The first adds a todo by its form and reads the
summary line the page shows:

```alder
test "adding a todo through the page" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Todos.Index.Index {})
  let added <- Test.submit (Todos.Index.Submit {}) { title: Text.NonEmpty "Milk", due: Nothing }
  let page <- Test.get (Todos.Index.Index {})
  Task.succeed ({ answer: added.answer, todos: List.map (\t -> t.title) page.model.todos, summary: Test.text "#summary" page.doc } == { answer: Redirected (Todos.Index.Index {}), todos: [Text.NonEmpty "Milk"], summary: Just "1 open" })
}
```

In `app/Todos/Index.alder` the routes are written `Index` and `Submit`;
from another module, as here, a route value keeps its full path,
`Todos.Index.Index {}` (no import is needed for it; chapter 6, "Names").

### `Answer`: what a request came to

`Answer` is a union of the Prelude, which only tests can use:

```text
type Answer = Shown
            | Rendered Route
            | Refreshed
            | Redirected Route
            | FormRejected (List { field: Text, message: Text })
            | PageMissing
            | RequestFailed Text
```

| Answer | When | The page the browser then holds |
|---|---|---|
| `Shown` | `get`/`open`: the page was shown | that page |
| `Rendered r` | `click` of the GET route `r`, or of a write route whose `update` answered `Response.render r` | the page it held, with `r`'s view patched in as the element with the view's root `id` |
| `Refreshed` | `click`/`submit` of a write route whose `update` answered `Response.refresh`; or `Test.refresh`, when the page loaded again is the page held | after a write, the page re-rendered, the regions the server changed patched in; after `Test.refresh`, the page loaded again |
| `Redirected r` | a `Response.redirect r`, a `load` ending in `Query.redirect r`, or a `requires` sending a visitor to the login page | `r`, which the browser requested in turn |
| `FormRejected errors` | `submit`/`click`: the form did not decode, or `update` answered `Response.invalid t` | the page with the form re-rendered: the values typed and the errors (status 422) |
| `PageMissing` | a 404: `Query.notFound`, `Task.notFound`, no such route | `App.notFound` (when only an element was asked for, a `Response.render` or a click on a GET route: the page it held, with an error toast) |
| `RequestFailed text` | the request failed (a `Task.fail`, a Go panic): a 500 | `App.error` (for an element, as above: the page it held, with an error toast) |

`Rendered` and `Refreshed` are the names of `Response.render` and
`Response.refresh` as the browser sees them: the first patches one
element, the second the regions that changed (section 10).

`Rerendered`, the answer of the old page-and-message form's re-render,
is gone with that form. A test that names it is E0002; write `Refreshed`.

A redirect to another site (`Url.external`, chapter 6) is reported,
`Redirected` with that address, but not followed: the browser keeps the
page it had. A redirect to a page that is then a 404 is `PageMissing`.

`RequestFailed`'s text is the failure's message: `"nope"` for a
`Task.fail "nope"`, `"panic: …"` for a Go panic.

The 404 answer is `PageMissing`, not `NotFound`, so that an app keeps the
name `NotFound` for a constructor of its own. The seven names are kept for
`Answer` everywhere: a union of the app may not use one (E0012), nor may
a type be named `Doc` or `Answer`.

### Setting the clock

`Slow` above works out its answer from the ticking clock: 20000 ms
of delay plus the one second the second reading adds. That is fine for
one route, but a route whose output depends on the time (a countdown, a
"closes in 3 hours", today's schedule) would make every expectation a
sum of ticks, and one more `Time.now` on the route's path would shift them
all by a second.

Two steps let a test say what time it is and when time
passes:

```text
Test.setClock : Time -> Task Unit
Test.advance  : Int -> Task Unit
```

- `Test.setClock t` sets the test's clock to `t`, and from then on the
  clock **stands still**: every `Time.now` and `Date.today` reads `t`
  (the one-second tick is off for the rest of the test). A second
  `setClock` moves it again, backwards too; the clock is the test's.

- `Test.advance ms` moves the clock `ms` milliseconds forward, the unit
  of `Time.toMillis` and `Query.delay`. A negative `ms` counts as 0 (to
  go back, set the clock again), and there is no practical upper limit
  (advancing a day is the point; anything past about 292 years stops
  there rather than wrapping into the past). A clock that ticks keeps
  ticking after an advance, and a set one stays still.

- A `Query.delay` or `Task.sleep` still moves the clock by its delay,
  set or not, and the routes a test requests through the browser read the
  same clock, so a set applies to them too.

- `@created` reads the clock as well, so rows inserted after a set all
  get the same time. A test that sets the clock and then wants a list in
  creation order calls `Test.advance 1000` between the inserts, or
  inserts its rows before the set.

- The session cookie's own clock does not move: a test that logged in
  stays logged in however far it advances time.

- Like the tick, a set or an advance is not undone by a `Test.run` that
  fails. Both are on the trace, as `Test.setClock 2026-03-04T00:00:00Z`
  (the time in UTC) and `Test.advance 86400000ms`.

A test that never sets the clock is unchanged: it starts at
`2026-01-01T00:00:00Z` and ticks. Here a page prints today's date, and
the test sets the clock to a date, fetches it, advances a day and
fetches again:

```alder
module Todos.Today

route Today = GET "/today" { }

type Model = { today: Date }

Today.load : Today.Params -> Session -> Query Model
Today.load params session = {
  let today <- Date.today
  Query.succeed { today: today }
}

Today.view : Model -> Session -> File
Today.view model session = File.text { contentType: "text/plain", body: Date.toText model.today }

-- 1772582400000 is 2026-03-04T00:00:00Z.
test "today follows the test's clock" = {
  let _ <- Test.setClock (Time.fromMillis 1772582400000)
  let before <- Test.fetch (Today {})
  let _ <- Test.advance (24 * 60 * 60 * 1000)
  let after <- Test.fetch (Today {})
  Task.succeed ({ before: Bytes.toText before.body, after: Bytes.toText after.body } == { before: Just "2026-03-04", after: Just "2026-03-05" })
}
```

### `Test.login`, `requires`, and the session

`Test.login s` signs `s` into the browser's cookie, as the router would
after a `Session.put`, and makes `s` the test's own session too, the one
the scoped operations of chapter 6 act for (section 3, "Tests that use
the rules").

`s` is checked against `App`'s `session` record
when the test compiles (`{ }` in an app without a session). It is the
cookie's value, so a field with a default may be left out: with
`session { user: Maybe UserId, visits: Int = 0 }`,
`Test.login { user: Just me }` stores `visits: 0`.

`Test.login` is the short way in, for a test about something else. A
test about logging in itself goes through the login route with its real
form.

That route calls the `Password` capability, whose adapter is Go; a
test replaces it with the fake `app/Auth/Login.alder` declares beside it
(section 8), which accepts any user who has a row, whatever the
password.

The test file imports `Auth.Login`, so the capability is
written through that import's qualifier, `Login.Password`; the routes
are route values, written with their full paths, `Auth.Login.Login {}`
for the page and `Auth.Login.Submit` for the one that posts its form:

```alder
test "logging in through the form" = {
  let _ <- Test.fake Login.Password
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.get (Auth.Login.Login {})
  let r <- Test.submit Auth.Login.Submit { email: Text.Email "me@example.com", password: Text.Secret "secret" }
  Task.succeed ({ answer: r.answer, user: r.session.user, title: Test.text "h1" r.doc } == { answer: Redirected (Todos.Index.Index {}), user: Just me, title: Just "Todos" })
}
```

The `Submit` route's `update` called the fake and `Session.put`; the
browser kept the cookie, followed the redirect with it, and holds the
list.

> **The browser's login.** That login is the browser's: the test's own
> session is still the one it started with, so a `Db.mine` in the test
> body would have no user.

With an email that has no row the fake answers `Nothing`, `update`
answers `Response.invalid "Email or password is incorrect."`, and the
answer is `FormRejected [{ field: "", message: "Email or password is
incorrect." }]`, with `status` 422.

`Test.click App.Logout` presses the
list's "Log out" button: the answer is `Redirected (Auth.Login.Login {})`
and `session.user` is `Nothing`.

### `Test.submit`: posting a form

`Test.submit r f` posts the form of the write route `r` with the record
`f`. `r` is a route with `form F`, written in place with its parameters
if it has any: `Todos.Index.Submit {}` here, `Todos.Show.Save { id: id }`
for the route that saves one todo. `f` is checked as the form's record (a
field with a default may be left out). The browser sends it as the page's
inputs would:

| The field's type | Sent as |
|---|---|
| `Text`, `Text.NonEmpty`, `Text.Email`, `Text.Url`, `Text.Secret`, a brand of `Text` | the text |
| `Int`, a brand of `Int` | its digits: `42`, `-3` |
| `Float` | the shortest decimal that reads back the same: `1.5`, `1e-07` |
| `Bool` | `True` when `True` (a ticked checkbox); not sent when `False` (an unticked one) |
| `Date` | `2026-03-01` |
| a union without payloads | the constructor's name: `Active` |
| `Maybe a` | blank for `Nothing` (an empty input); `a`'s text for `Just` |

Decoding it gives back `f`, except that a `Maybe Text` that is
`Just ""` is sent blank and decodes as `Nothing`: neither a test nor a
user can send `Just ""`.

The form must be on the page the browser holds, and the route the test
writes must be the route that form posts to, parameters included. The
browser sends it to the address the form carries, and sends only the
fields the form has an input for (an `<Input>`, `<Select>` or
`<Textarea>` with the field's name), as a real browser does.

A field
without one fails the step, "the page's form NewTodo has no input
`due`", unless its value is what the page makes of a field that is not
sent: `Nothing` for a `Maybe` without a default, the default for a field
that has one, `False` for a `Bool`; those are left out, and the route
decodes the same record.

A page opened as `Todos.Index.Index { filter: Active }` renders its form
as `Submit { filter: Active }`, whose address is `/todos?filter=Active`;
that is the route to write. `Submit {}` is `/todos`, another address,
and the step fails "the page /?filter=Active has no form NewTodo
(Todos.Index.Submit)".

A form that each row renders
(`<form submit={Rename { id: n.id }}>` in a loop) is told apart the same
way: the test writes the route with the row's parameters,
`Test.submit (Rename { id: b }) { title: Text.NonEmpty "B2" }`, which
posts the form of row `b`.

A `Date` in a test record: `Date.fromText "2026-03-01"` is a
`Maybe Date`, the type of a `due: Maybe Date` field. For a `Date` field,
take one from the test's clock: `let today <- Date.today` gives
`2026-01-01`, `Date.addDays 3 today` a later day.

### `Test.submitRaw`: input the types forbid

`Test.submit`'s record can only hold valid values: a `Text.NonEmpty` is
never empty, a `Date` is always a date. A user's browser has no such
rule, and the route must answer bad input with the form and its errors.

`Test.submitRaw r fields` posts text, field by field, as typed:
`Test.submitRaw (Todos.Index.Submit {}) [{ name: "title", value: "" }]`. A
field not sent is a field left out of the form. Every name sent must be a
field of the form and have an input on the page.

**`FormRejected errors`** is the answer when the page shows its form again
with errors: the form did not decode (chapter 6: `update` never ran), or
`update` answered `Response.invalid "…"`. The status is 422. The browser
holds the page re-rendered, with what was typed back in each input
(except a `Text.Secret`, which is never sent back) and each error in its
`<Errors>` list. `errors` is what those lists show:

- in the order the lists come in the page (the order of the form's
  `<Errors field="…">`, usually its fields' order);

- `field` is the field's name, and `""` for the form's own list,
  `<Errors form />`, which holds `invalid`'s text;

- a field error the form has no `<Errors field>` for is shown in
  `<Errors form />`, after `invalid`'s text, prefixed with the field's
  name, so it comes back as `{ field: "", message: "title must not be
  empty" }`;

- a form with no `<Errors form />` shows no `invalid` text at all, and
  `errors` is `[]`: the answer to an `invalid` is still `FormRejected` and
  the status 422, but a user sees the page unchanged.

  The compiler warns about such a form
  when the `update` of its route can answer `Response.invalid` (W0005).

- a form the re-rendered page no longer draws shows no `invalid` text
  either. A page held while an order was Open, the order paid meanwhile,
  and its add-line form posted: `update` answers `invalid`, but the Paid
  order's page has no add-line form, so the text has nowhere to go; the
  answer is `FormRejected []`, and a user sees the page change with no
  message.

  W0005 cannot see this (it depends on the data). When such a
  refusal must be visible, answer `Task.notFound` or `Task.fail` instead.

The messages are the decoders' (chapter 6): "must not be empty" for a
`Text.NonEmpty`, "must be a date (YYYY-MM-DD)", "must be a whole
number", and "is not valid" for a brand whose `where` refuses the value
(the blog's comment `Body`, chapter 14).

### `Test.click`: pressing a button

`Test.click r` sends the route `r` as the page's button for it would:
`Test.click (Todos.Index.Toggle { id: id })`, `Test.click App.Logout`. `r`
is a route without a `form`, with its parameters, written as in a
`click={…}`. What is sent depends on the route:

- A write route is sent, with the headers the page's script
  sends (`Alder-Page`, the address the browser holds, and `Alder-Build`;
  chapter 6, "Delivery, decided by the request").
- A GET route is fetched
  the way a button fetches an element, and its view is patched into the page
  held as the element with the view's root `id`.
- A route that carries a form is posted with `Test.submit`; a link is followed with `Test.get` or
  `Test.open`. The markup has the same rule: `click={R}` of a write route with a form is E0045
  too, and the page sends such a route with `<form submit={R}>`.

The page must show a button (or form) that sends exactly that route,
with the same parameters: a test can only do what the user it is logged
in as can see to do. The second to-do test through the page:

```alder
test "toggling a todo through the page" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Todo { title: Text.NonEmpty "Milk", due: Nothing, owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Todos.Index.Index {})
  let r <- Test.click (Todos.Index.Toggle { id: id })
  Task.succeed ({ answer: r.answer, done: Test.has "li.done" r.doc, summary: Test.text "#summary" r.doc } == { answer: Refreshed, done: True, summary: Just "0 open" })
}
```

`Toggle.update` answered `Response.refresh`, so the browser loaded the
page again and patched in the regions that changed: the item now has
`class="done"`, and nothing is open. The answer is `Refreshed`.

Had the
route answered `Response.render (Row { id: id })`, the answer would be
`Rendered` with the address of `Row`, and only that one item would have
changed (chapter 6, "`Response`").

### What the browser checks on every step

A click or a submit changes the page the browser holds by patching it, as
the page's script does. Two checks run on what was patched, and a patch
with nowhere to go is refused. A step that fails one fails the test.
Section 10 shows how each is made.

- **The region check.** After a `refresh`, an `invalid` or a followed
  `redirect` the server rendered the page, and the browser compares each
  region the answer patched with the same region of the full render.

  A difference fails the step: "PATCH /todos/3: the patched page differs
  from a full render at" the path of the first element that differs (named
  as a failed `Doc` comparison names it, below), with both markups, as the
  browser's parser builds them. A region the answer did not send is not
  compared. A difference means the page the user holds is not the page the
  server rendered.

- **The element check.** After `Rendered r`, the browser asks for `r` again
  on its own, as a button does, and the element it holds must equal the
  view that answers. It is the same `load` and `view`, so they are equal
  unless the view shows something that changes from one request to the
  next.

  The step fails: "the answer patched #todo-1 unlike
  Todos.Index.Row's view rendered on its own (GET /rows/…) at li#todo-1".
  The second request runs in a savepoint that is undone, and is not a step
  of the trace. It runs the route's `load` a second time, so a capability
  the `load` calls is called twice, a fake too (section 8): the savepoint
  undoes the rows in the fake's tables, not what it keeps outside the
  database.

- **A patch with nowhere to go.** A `render` whose root `id` the held page
  lacks patches nothing in a real browser, silently. The test fails the
  step: "PATCH /todos/9: the answer patched `#todo-9`, which the page does
  not have".

  A view with no root `id` is not patched either: the answer is
  `RequestFailed` ("route GET /rows/\{id} answered an element fetch with a
  view whose first root element has no id").

A `render` patches one element, and a page can have other places that
should have changed: these checks do not see that. `Test.refresh` does.

### `Test.refresh`: is the page the one a fresh load would give?

`Test.refresh` loads the address the browser holds again, with a plain
`GET` as a reload does (a redirect followed), and fails unless the body
of the page it gets is the body of the page the browser held. It then
holds the new page, and the answer is `Refreshed`.

The `<al-toasts>`
region is left out of the comparison, since a reload may fill it from a
notice that the step's toasts already delivered. A test of a toggle ends
with it:

```alder
test "a toggle leaves the page as a fresh load would" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Todo { title: Text.NonEmpty "Milk", due: Nothing, owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Todos.Index.Index {})
  let clicked <- Test.click (Todos.Index.Toggle { id: id })
  let again <- Test.refresh
  Task.succeed ({ clicked: clicked.answer, again: again.answer, url: again.url } == { clicked: Refreshed, again: Refreshed, url: "/" })
}
```

`Toggle` answers `refresh`, which patches the regions that changed, so
the page after the click is the page a fresh load gives, and this
passes. `Test.refresh` is the developer's check, not part of every step,
because some pages are right without it.

> **Pages that differ on purpose.** A test of "load more", of a draft
> preview, or of an inline edit that is open leaves the page different from a
> fresh load on purpose: do not end those with `Test.refresh`.

### A stranger's todo

**Who may toggle a todo.** Chapter 6's rule, "Rules", says that a user
who posts `Toggle` with someone else's todo id gets a 404. A stranger's
own list offers no button for my todo, but a page can be out of date. My list is open in the browser; then
someone else logs in on it (the cookie changes, the page held does not)
and presses my todo's button. The example tests exactly that:

```alder
test "Toggle by another user is not found" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Todo { title: Text.NonEmpty "Milk", due: Nothing, owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Todos.Index.Index {})
  let _ <- Test.login { user: Just (User.UserId "someone-else") }
  let r <- Test.click (Todos.Index.Toggle { id: id })
  let after <- Db.get Todo id
  Task.succeed ({ answer: r.answer, done: Maybe.map (\t -> t.done) after } == { answer: PageMissing, done: Just False })
}
```

The click posts my page's Toggle button with the stranger's cookie. The
router renders the page the stranger is on first (the before-view of a
route that can answer `refresh`; section 10), then runs `update`, where
`Db.updateMine` finds no todo of the stranger's with that id:
`PageMissing`, and the todo is not done.

"Delete by another user is not
found" does the same with the Delete button.

### Reading the page: queries

Five functions read a `Doc`, and one makes a `Doc` from a view. They are
plain functions (not steps), so they are used in expressions, as above:

```text
Test.text  : Text -> Doc -> Maybe Text           -- the first match's text
Test.texts : Text -> Doc -> List Text            -- every match's text, in page order
Test.has   : Text -> Doc -> Bool                 -- whether anything matches
Test.count : Text -> Doc -> Int                  -- how many elements match
Test.attr  : Text -> Text -> Doc -> Maybe Text   -- an attribute of the first match
Test.doc   : Html -> Doc                       -- a view, as a document
```

The first `Text` is a **selector**, as in CSS; `Test.attr`'s first is
the attribute's name, its second the selector:
`Test.attr "href" "a[rel=next]" doc`. Alder accepts a small part of
CSS's selectors:

| Selector | Matches |
|---|---|
| `li` | every `<li>` |
| `#summary` | the element with `id="summary"` |
| `.done` | every element whose `class` has `done` |
| `[href]` | every element with an `href` attribute |
| `[rel=next]`, `[name="title"]` | every element whose `rel` is `next` (quote a value that is not a plain word: `[href="/x"]`) |
| `li.done`, `input[name=due]` | both at once: an `<li>` with the class `done` |
| `ul li` | an `<li>` anywhere inside a `<ul>` |
| `ul > li` | an `<li>` directly inside a `<ul>` |

A tag is an HTML element's name in lower case, or a custom element's (a
name with a hyphen). Nothing else is accepted (no `*`, `:first-child`
or `,`).

A selector written as a literal is checked when the test
compiles (E0023). A selector that is not a literal (in a variable, or
built with `++`) is read when the test runs, and if it is not a selector
it matches nothing: `Nothing`, `[]`, `False`, `0`. A wrong one then
looks like a page that lacks the element, so write selectors as
literals.

An element's text is the text inside it (not counting a `<script>` or
`<style>`), with every run of spaces and line breaks made one space and
none at either end.

No space is put between elements: the to-do item
`<li><button>Done</button><a …>Milk</a><button>Delete</button></li>` has
the text `"DoneMilkDelete"`. Select the element that holds just the text
wanted, `Test.texts "#todos a"`, not the whole item.

#### `Test.doc` and `view`

`Test.doc (view model session)` renders a view
the way the page renders it and parses it, so a test can compare the
page the browser holds with what `view` makes of the model it was given.
Two `Doc`s are equal when they are the same tree of elements, attributes
and text. `Test.doc` renders without a request, so a form in it has
no typed values and no errors: compare a `FormRejected` page by its queries,
not with `Test.doc`.

### What a failed step says

When a step cannot do what the test asks, the test fails with one of
these texts, and nothing is sent:

- `GET /: redirected to /login (requires user)`, `GET /todos/x: not
  found`, `GET /: failed: <message>`: `Test.get` of a page not shown.

- "the page /login has no form NewTodo (Todos.Index.Submit)": the held
  page renders no form for that route, with those parameters.

- "the page / has no button or form for Todos.Index.Toggle { id: "x" }":
  the held page renders no control for that route, with those
  parameters.

- "the page's form NewTodo has no input `due`": a field the form renders
  no input for (`Test.submit`, `Test.submitRaw`).

- "no page is open: `Test.get` one first".

- "form NewTodo has no field `colour`": a `Test.submitRaw` name the form
  does not declare.

- "PATCH /todos/3: the patched page differs from a full render at …",
  "… the answer patched #todo-1 unlike Todos.Index.Row's view rendered on
  its own …" and "… the answer patched `#todo-9`, which the page does not
  have": the checks of "What the browser checks on every step".

- "Test.refresh: GET /: the page held differs from the page loaded again
  at …", "Test.refresh: GET /rows/x: not found" and "Test.refresh: the
  browser holds no page yet": `Test.refresh`.

And when the test compiles (E0003 unless named):

- E0040 "`Test.get` is only available in a `test` or a test helper:
  `Toggle.update` is the route's update, not a helper"; "`Test.get`
  needs an `App` module: the browser requests the app's routes, which an
  app without `App` does not serve".

- "`Test.submit` takes its arguments directly, not through `|>`: …";
  E0008 "`Test.submit` takes 2 arguments, got 1: …".

- "`Test.get` wants a page value such as `Todos.Index {}`, got `p`" (a
  route held in a variable: write the route in place, `Todos.Index.Index
  {}`); "`Test.get` needs a record `Model`: … use `Test.open`".

- "`Test.login` wants the app's session `{ user: Maybe User.UserId }`:
  expected Maybe User.UserId, got User.UserId".

- E0045, a route of the wrong kind for the step: "`Test.get` requests a
  GET route; `Toggle` is POST: use `Test.click`"; "`Test.submit` posts a
  route's form; `Toggle` has no `form`: use `Test.click (Toggle { id: … })`";
  "`Test.submit` posts a route's form; `Index` is GET: use `Test.get`";
  "`Test.click` sends a route without a form; `Submit` posts form
  `NewTodo`: use `Test.submit (Submit { filter: … }) { … }`".

- E0003 "`Test.submit` names the route that posts a form: write
  `Test.submit (Submit { filter: … }) …`"; E0002 "`NewTodo` is a form of
  `Todos.Index`: write `Todos.Index.NewTodo`" and "write
  `Todos.Index.Submit`: a route is written with its module's full path".

- E0002 "unknown constructor `NotFound`; the browser's 404 answer is
  `PageMissing`"; E0012 "`PageMissing` is a constructor of the Prelude's test
  type `Answer`; …"; in a type declaration, "`Doc` is a test type: it
  exists only inside a `test`".

- E0023 "`#sumary x` is not a selector: unexpected `x` after `#sumary`;
  `x` is not an HTML element (a custom element has a hyphen)"; its other
  hints are "tags are lower case: `h1`" and "quote the value:
  `[href="/x"]`".

- W0005 "`Submit` can answer `Response.invalid`, but this form has no
  `<Errors form />`: the user never sees its text".

### The rules

- The steps exist only inside a `test` or a test helper (below), each
  applied directly to all its arguments (not piped into, not passed as a value); all but
  `Test.login` need an `App` module.

- **Only what the page shows.** `Test.submit` and `Test.click` act on a
  form or button the held page renders, posting it exactly as rendered,
  and a submit sends only the fields the form has inputs for.

- **One transaction.** Every request runs inside the test's own
  transaction, its `update` under a savepoint of its own: a page sees
  the rows the test inserted, the test sees the rows a page wrote, and a
  request that fails (`RequestFailed`, `PageMissing`) undoes only its own writes.

- **The browser is not rolled back.** A `Test.run` that fails undoes
  its database writes, the writes of the steps inside it included, but
  not what happened to the browser: the cookie and the page held stay as
  the run left them. Steps inside `Test.run` work as anywhere else.

- **One browser per test.** Each test starts with no page and no
  cookie; nothing of one test's browser reaches the next.

- **Traces.** A step is a step of the trace, named `Test.login`,
  `Test.get Todos.Index.Index`, `Test.open Todos.Show.Show`,
  `Test.submit NewTodo`, `Test.submitRaw NewTodo`,
  `Test.click Todos.Index.Toggle`, `Test.click App.Logout` or
  `Test.refresh`, and the effects of the requests it made follow it.

- **What it does not test.** No CSS, no layout, no JavaScript: whether a
  button is visible, whether a web component works, how the page looks.
  The browser is Datastar's client (section 10), so the path of a
  browser with JavaScript off, a plain form post answered by a
  redirect, is not tested either (section 13).

A step's result also has `notices`, the list of toast texts the answer showed
(`Response.toast "Saved" Response.refresh` gives `["Saved"]`), empty when there were none:

    { answer: r.answer, notices: r.notices } == { answer: Refreshed, notices: ["Saved"] }

It also has `toasts`, the same toasts in the same order with their kind
(`"info"`, `"success"`, `"warning"` or `"error"`, chapter 6), for a test
that checks which kind the answer showed. `Response.success "Saved"
Response.refresh` gives:

    r.toasts == [{ kind: "success", text: "Saved" }]

### Test helpers

Tests of one page often begin the same way: the same fakes, the same
request, the same decoding of the body. That prologue can be written
once, as an ordinary top-level value, in any module, and called from any
test:

```alder fragment
fetchText : Text -> Task { status: Int, body: Text }
fetchText cfg = {
  let _ <- Test.fake Gate
  let _ <- Test.fake MnSos
  let r <- Test.fetch (Sites.MnSos.MnSos { cfg: cfg })
  Task.succeed { status: r.status, body: Bytes.toText r.body ?? "" }
}

test "at minute 400 the file is the final" = {
  let r <- fetchText (at 400 "")
  Task.succeed (r.status == 200 && Text.contains "final" r.body)
}
```

`fetchText` is a *test helper*: a value that only tests may use. There
is no keyword for it. A top-level value is a test helper when its
signature or body uses something only a test may use:

- anything of `Test`: the steps, `Test.run`, `Test.trace`, `Test.fake`,
  `Test.login`, `Test.member`, and the queries `Test.text`, `Test.has`, …;
- the Prelude's test types `Doc` and `Answer` (a `Doc -> Text` value is
  a helper), or a constructor of `Answer`;
- a fake's table (section 8);
- another test helper.

A `test`, or another test helper, in any module, may use one. Production
code may not.

Some values can never be helpers, because the compiler
or the runtime calls them: a route's `load`, `view` and `update`, every
value of `App`, a fake's entries, a brand's `where` check and a field's
default. A reference from one of those to a helper is E0040, naming the
helper and the first thing that makes it one:

    `fetchText` is a test helper (it uses `Test.fetch`): only a `test` or another test helper may use it

and along a chain, `` `at400` is a test helper (it uses `fetchText`, a
test helper) ``. Using a test-only thing directly in one of them is E0040
at the use: "`Test.fetch` is only available in a `test` or a test
helper: `Index.view` is the route's view, not a helper" (likewise "a fake's
entry is not a helper", "a field default is not a helper", "a brand
check is not a helper", "`App.update` is not a helper").

`alder build` leaves every test helper out of the program, and `alder
test --mutate` makes no mutant in one (section 11).

The browser's rules
still hold inside a helper: `Test.fetch (Sites.MnSos.MnSos { cfg: cfg })` is a
route written in place with the helper's parameter inside it, while
`Test.fetch p`, with `p` a parameter, is refused as in a test.

A fake is
still declared in its capability's module (E0020); a helper shares the
`Test.fake` call, not the fake. The editor's hover on a helper says
which it is: "test helper (uses `Test.fetch`)".

## 11. Mutation testing

Passing tests say the code does what they check, not that they check
what matters. This section measures that, and turns it into tests.

### The command

```text
alder test --mutate [--min R] [--json] [-run re] [-timeout d] [dir]
```

It runs the tests as `alder test` does; if one fails, it stops with
`tests fail: fix them before measuring mutants`. Otherwise it runs them
again once per mutant. Here is the to-do app's own code (the generated
admin of chapter 15 adds 185 mutants of its own) with the eleven tests it
had before this section's work, the others left out with `-run`:

```text
11 passed, 0 failed
41 mutants: 21 killed, 20 survived  (score 0.51)
survived:
  App           Logout.update:16   update field dropped   { s | user: Nothing, role: Member } → { s | user: Nothing }
  App           Logout.update:17   redirect → refresh     Response.redirect (Auth.Login.Login {}) → Response.refresh
  Auth.Login    Submit.update:42   update field dropped   { s | user: Just id, role: role } → { s | user: Just id }
  Table.Todo    Todo:16            cascade ↔ restrict     owner @ref User @restrict → owner @ref User @cascade
  Table.Todo    matches:21         arm swapped            match filter { All -> True, Active -> not todo.done, Complete…
  Table.Todo    matches:23         not dropped            not todo.done → todo.done
  Table.Todo    filterLabel:28     arm swapped            match filter { All -> "Al… → the All and Active bodies swapped
  Table.Todo    filterLabel:28     arm swapped            match filter { All -> "All", Active -> "Active", Completed ->…
  Todos.Index   Submit:8           requires dropped       requires user → served without user (user = Table.User.UserId "mutant")
  Todos.Index   Toggle:9           requires dropped       requires user → served without user (user = Table.User.UserId "mutant")
  Todos.Index   Index.load:17      orderBy reversed       Db.orderBy Todo.created.desc → Db.orderBy Todo.created.asc
  Todos.Show    Show:7             requires dropped       requires user → served without user (user = Table.User.UserId "mutant")
  Todos.Show    Save:12            requires dropped       requires user → served without user (user = Table.User.UserId "mutant")
  Todos.Show    Delete:13          requires dropped       requires user → served without user (user = Table.User.UserId "mutant")
  Todos.Show    Save.update:27     write replaced         let _ <- Db.updateMine Todo p.id … → let _ <- Task.succeed ()
  Todos.Show    Save.update:27     scope dropped          Db.updateMine Todo p.id (\t -> { t | title: p.form.title, due: p.form.due }) → Db.update Todo p.id …, not found when missing
  Todos.Show    Save.update:27     update field dropped   { t | title: p.form.title, due: p.form.due } → { t | due: p.form.due }
  Todos.Show    Save.update:27     update field dropped   { t | title: p.form.title, due: p.form.due } → { t | title: p.form.title }
  Todos.Show    Save.update:28     redirect → refresh     Response.redirect (Todos.Index.Index {}) → Response.refresh
  Todos.Show    Delete.update:34   redirect → refresh     Response.redirect (Todos.Index.Index {}) → Response.refresh
```

A test failed for 21 of the 41 mutants: the score is 21 / 41. A
survivor's line gives:

- its module;
- `declaration:line` (`Index.load:17` is
  line 17 of `app/Todos/Index.alder`; a route's members are named as in the
  source, `Save.update:27`; a column is added, `load:33:17`, when the line
  has two mutants of that kind);
- the operator;
- the code before `→`
  after, shortened with `…` (an arm swap names its arms).

### The operators

Each mutant comes from one of twenty-six **operators**, named by what it
breaks, in five classes; every mutant is well-typed:

| Class | Operator | What it changes |
|---|---|---|
| authorization | `requires dropped` | A route's first required field (`requires user`, say) is dropped: the route serves a visitor as `<Brand> "mutant"` (for a field that is a brand over `Text`; other fields are skipped); one mutant per route |
| authorization | `role dropped` | A route's `requires user, role R` becomes `requires user` |
| authorization | `scope dropped` | `Db.mine T` becomes `Db.from T`; the other scoped operations act on anyone's row; likewise `Db.scoped` and its family (not on a siloed table) |
| authorization | `membership dropped` | A key's `via M` check removed: a `Session.put` of any org succeeds |
| authorization | `guard dropped` | An `if` or `match` with one arm ending in `Task.notFound`, `Query.notFound` or `Task.fail` becomes its other arm: the check is gone |
| authorization | `step dropped` | A `let _ <- e` step becomes `Task.succeed ()` (or `Query.succeed ()`; `Task.succeed []` for a `List Unit`, a bulk action's writes): the access check `let _ <- Layout.adminOnly session.user` that begins a write route's `update`, say. Not an update, a delete or a `Session.put` (the two operators below make that program), nor a step whose result is neither `Unit` nor a `List` of it |
| data | `where dropped` | One `Db.where` of a query removed |
| data | `orderBy reversed` | A `Db.orderBy`'s `asc` and `desc` swapped |
| data | `limit dropped` | A query's `Db.limit` removed |
| data | `offset dropped` | A query's `Db.offset` removed |
| data | `write replaced` | A `let … <-` insert, update or delete becomes `Task.succeed` of a placeholder (`()`, or an id `"mutant"`) |
| data | `update field dropped` | One field of a record update `{ t \| a: …, b: … }` that sets two or more left out, wherever it is |
| data | `session put dropped` | A `Session.put` step becomes `Task.succeed ()` |
| flow | `redirect → refresh` | `Response.redirect r` becomes `Response.refresh` |
| flow | `render → redirect` | `Response.render r` becomes `Response.redirect r` |
| flow | `render → refresh` | `Response.render r` becomes `Response.refresh`: the whole page rendered again, not `r`'s element |
| flow | `invalid → refresh` | `Response.invalid "message"` becomes `Response.refresh`: a refused write answered as a done one |
| flow | `refresh → invalid` | `Response.refresh` becomes `Response.invalid "mutant"`: a done write answered as a refused one |
| flow | `toast dropped` | One `Response.toast`, `success`, `warning` or `error` left out: the `Response` it decorates is answered without the notice |
| flow | `arm swapped` | Two neighbouring arms of a `match` trade bodies (one mutant per pair) |
| logic | `condition negated` | `if c` becomes `if not c` |
| logic | `operator flipped` | `==`↔`!=`, `<`↔`<=`, `>`↔`>=`, `&&`↔`\|\|` |
| logic | `bool flipped` | `True`↔`False` |
| logic | `not dropped` | `not e` becomes `e` |
| schema | `default changed` | A table field's default: `False`↔`True`, a number → one more, `""`→`"x"`, a union's constructor → the next one |
| schema | `cascade ↔ restrict` | A reference's delete rule, `@cascade`↔`@restrict` |

A write route runs no `load` (chapter 6), so its own `requires`, its role
and the access check its `update` begins with are all that stop a
request: each is a mutant, and a test that sends the write from a page
still open after the session changed kills it (a visitor, a cookie whose
role is no longer the one required, a row demoted since; section 9).

`membership dropped` dies only to a test that tries an org the user is no member of.

### A surviving mutant is a test you have not written

Each survivor is a behaviour no test checks: write the test that would
notice. `Todos.Show Save.update:27` has four of them in the report above:
the save could store nothing, drop the title, drop the due date, or touch
anyone's todo. One test that saves the form and reads the row back kills
the first three:

```alder
test "saving changes the title and the due date" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Todo { title: Text.NonEmpty "Milk", due: Nothing, owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Todos.Show.Show { id: id })
  let r <- Test.submit (Todos.Show.Save { id: id }) { title: Text.NonEmpty "Bread", due: Date.fromText "2026-10-12" }
  let after <- Db.get Todo id
  Task.succeed ({ answer: r.answer, title: Maybe.map (\t -> t.title) after, due: Maybe.andThen (\t -> t.due) after } == { answer: Redirected (Todos.Index.Index {}), title: Just (Text.NonEmpty "Bread"), due: Date.fromText "2026-10-12" })
}
```

Nine tests of this kind kill those twelve, and the tests of the route
operators kill the eight others (below): the app's own code then ends
`41 mutants: 41 killed, 0 survived  (score 1.00)`.

### Survivors that are not gaps

Sometimes no test can tell a survivor from the app: then say why in a
comment or a README; otherwise write the test. No annotation silences a
survivor: it stays until a test kills it or the code changes.

### `--min` in CI

`--min R` (0 to 1) exits 1 when the exact score is below `R`; without
it the exit code is the tests'.

### Properties under `--mutate`

A property (section 12) is one test here: a mutant is killed when it
fails on any of the property's cases. A mutated run uses a fixed seed
(0, or the one `--seed` gives), does not shrink, and neither reads nor
writes saved failures, so the same code gives the same score. A `Fuzzer`
value (a generator, such as `slugs` in section 12) is not part of the
app's behaviour and gets no mutants.

## 12. Properties

A test checks one example: `countOpen` of three todos is 2. It passes,
and says nothing about four todos, or none, or a thousand.

A **property**
is a statement that should hold for every input of some type, and a
property test checks it against many inputs the tool makes up. You write
the statement; `alder test` writes the examples, a hundred of them by
default, and when one fails it shrinks the example to the smallest one
that still fails and prints it as a test you can keep.

### The function form

A test whose body is a function is a property. Its parameters are the
inputs, and its body, as in section 1, is a `Bool`:

```alder
-- moves the first element to the end: a rotation, not a reverse
rotate : List a -> List a
rotate xs = List.append (List.drop 1 xs) (List.take 1 xs)

test "rotate keeps the length" = \(xs : List Int) -> List.length (rotate xs) == List.length xs

test "rotate twice" = \xs -> rotate (rotate xs) == xs
```

`\(xs : List Int) -> …` is a lambda (chapter 3) whose parameter is
written with its type, in parentheses; a bare `\xs ->` is the same
without the type. There is no new declaration and no library: the
property is a `test`, it is listed in the report like any other, and
`-run` (section 7) picks it by name.

The first one passes. The second is
wrong, as the report below will show: rotating a list twice gives it
back only when it has at most two elements.

### Where the inputs come from

`alder test` has to make a `List Int` out of nothing, and it does it from
the **type**. The checker works out the type of each parameter (the
annotation, or what the body forces), and the compiler writes, for that
type, a **generator**: a function that builds one value of the type from
a stream of random choices. Every ordinary type has one:

- `Int`, `Float`, `Bool`, `Text`, `Unit`, `Bytes`, `Date` and `Time`.
  The distributions are chosen to find bugs, not to be uniform:

  - an `Int`
    is mostly small (over half the draws are 0 to 10), sometimes up to a
    thousand, sometimes large, and now and then a boundary such as 255,
    2^31 - 1 or the largest `Int`;
  - a `Float` is finite (no `NaN`, no
    infinity) and includes 0, tiny and huge values;
  - a `Text` is mostly
    short ASCII, sometimes Unicode, sometimes empty, and made only of
    printable characters and `\n`, `\t` and `\r`, so that every value can
    be written back as a literal;
  - a `Date` or `Time` is within a few years
    of the epoch or of 2026.

- `Maybe a`, `Result e a`, `List a` and `Dict k v` are built from the
  generators of their parts, and a list is short most of the time.

- A record is each field's generator in turn.

- A union is a choice of one constructor, then a generator for its
  payload.

  A union that contains itself (a tree) has a depth budget of four
  levels: each level down halves the chance of a constructor that
  recurses, and once the budget is spent only the others are drawn, so the
  values are finite. (A union whose every constructor needs the union
  again has no value to stop at: E0038, below.)

- A brand draws its base type and, when it has a `where` predicate,
  keeps the value only if the predicate holds. Below.

Some types have no generator: `Html`, `Route`, a function, a `Task`, a
`Query`, a capability, a `Form`. A property over one is E0038 (below),
and you write the generator yourself with `Fuzz.forAll`.

### `Int` when the type is open

In `\xs -> rotate (rotate xs) == xs` nothing says what the elements are:
`rotate` works on a list of anything, and `==` on anything comparable, so
the type of `xs` is `List a` for an `a` that nothing decides.

Rather than
refuse, the checker **defaults** a type variable that is left open in a
property's parameter to `Int`, and the report says so (below), so that
the choice is not hidden. `Int` is the simplest type that shows most
bugs. When the property is about something else, annotate the parameter:

```alder
test "unique never grows a list of text" = \(xs : List Text) -> List.length (List.unique xs) <= List.length xs
```

An annotation is how you pick any type, and the only way to get a type
the body does not force, such as a brand. Each lambda parameter is a bare
name or `(name : Type)`; the annotated form works in every lambda, not
only in a test.

### Brands as constraints

A brand with a `where` (chapter 4) is a constraint, and a property over
it draws only values that satisfy it:

```alder
type Age = brand Int where (\n -> n >= 0 && n < 150)

test "ages are under 100" = \(a : Age) -> a < 100
```

The generator for `Age` draws an `Int` and throws away the ones the
predicate refuses. For this brand that wastes few draws.

For a brand like
`Slug = brand Text where Text.matches "^[a-z0-9-]+$"` it wastes nearly
all of them: a random `Text` is almost never a slug. A generator that
rejected 94% of its draws would run the property on a handful of values,
or on none and pass, so `alder test` checks, and fails the property with
the reason instead:

```text
FAIL Probe "a slug is trimmed"
  error: `Slug`'s predicate rejects 94% of generated values; write a `Fuzz.forAll` with a generator that produces `Slug` values
  seed: 1061676061
```

The check looks after the first 50 attempts: more than 90% rejected is a
failure.

The six brands of the standard library (`Text.NonEmpty`,
`Text.Email`, `Text.Url`, `Text.Secret`, `Int.Positive`,
`Int.NonNegative`) are not made by filtering; each has a generator that
draws values of its own shape directly, so they never trip it. For your
own tight brand, write the generator.

### `Fuzz.forAll` and shared `Fuzzer`s

`Fuzz.forAll gen f` is the explicit form: the test's body is the
property, `gen` is a `Fuzzer a` that you build, and `f` takes one `a`.
The `Fuzz` module (chapter 9) has the pieces: `Fuzz.int`,
`Fuzz.intRange { min: 0, max: 5 }`, `Fuzz.text`, `Fuzz.list`,
`Fuzz.listOf { min: 1, max: 5 }`, `Fuzz.maybe`, `Fuzz.oneOfValues`,
`Fuzz.map`, `Fuzz.filter` and the rest.

A `Fuzzer` is a value like any
other and may be a top-level declaration of any module, typed by a
signature, so a generator is written once and shared by many tests:

```alder
type Slug = brand Text where Text.matches "^[a-z0-9-]+$"

slugs : Fuzz.Fuzzer Slug
slugs = Fuzz.map Slug (Fuzz.oneOfValues ["a", "b-c", "2024"])

test "a slug is not empty" = Fuzz.forAll slugs (\s -> Text.length s > 0)
```

`Fuzz.map Slug` is allowed because the brand's constructor is: the
module that declares a brand, and any test, may call it. A property over
several inputs takes `Fuzz.pair` or `Fuzz.map2` into a record, or nests
two `forAll`s; there is no `forAll2`.

`Fuzz.any : Fuzzer a` is the
generator that the function form derives from the type, as a value, so
the two forms mix: `Fuzz.pair slugs Fuzz.any` is a record of a slug and a derived
value of whatever type the context asks for. Its type must be known where
it is written: inside a function that is itself generic, it has none,
which is E0038.

`Fuzz.forAll` and `Fuzz.forAllTask` make a `Fuzz.Property`, a value that
only a `test` body can be. Anywhere else it is E0039 (below).

### Properties over the database

A property whose body is a block is a property of effects, and, as in
section 3, the block ends in `Task.succeed` of the verdict. Each attempt
gets a database of its own, empty and migrated, with the test's fakes and
session set up afresh, so one attempt never sees another's rows:

```alder
test "a saved title reads back" = \(title : Text.NonEmpty) -> {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Todo { title: title, due: Nothing, owner: me }
  let after <- Db.get Todo id
  Task.succeed (Maybe.map (\t -> t.title) after == Just title)
}
```

The same holds for each attempt the shrinker makes, which is why an
effect property is slower than a pure one: each attempt builds a database
(in memory, section 3). `Fuzz.forAllTask` is the explicit form, with a
function that returns a `Task Bool`.

### Two errors

A property needs a generator for every parameter. When a type has none,
the checker says so at the parameter:

```alder error=E0038 "`Int -> Int` cannot be generated; use `Fuzz.forAll` with a generator you write"
test "a function is the same twice" = \(f : Int -> Int) -> f 1 == f 1
```

A function type, an effect type (`Task`, `Query`), a type parameter and a
value only the runtime can make have no generator, and neither has a
union whose every constructor holds the union again (`type S = S { next:
S }`: nothing ends the recursion; let one constructor hold a `Maybe` or a
`List` of it, or nothing).

Annotate with a type
that has one, or write the generator with the `Fuzz` functions and give
it to `Fuzz.forAll`.

A `Fuzz.Property` is what a test body is, and nothing else may be one:

```alder error=E0039 "`Fuzz.forAll` makes a Property, which only a test body can use"
pending : Fuzz.Property
pending = Fuzz.forAll Fuzz.int (\n -> n == n)
```

Move the `Fuzz.forAll` into a `test`, and keep the generator in a value.



====================  EXAMPLE APP: examples/todos  ====================
A complete, correct Alder application. Match its style exactly.

--- file: app/App.alder ---
module App

import Layout
import Table.User

-- `role` is copied from the user's row at login (Auth.Login); the generated admin's pages
-- `requires user, role Admin` read it from here (book 06, "Rules"). Logging out makes it Member.
session { user: Maybe User.UserId, role: User.Role = Member }

login = Auth.Login.Login

route Logout = DELETE "/session" { }

Logout.update : Logout.Params -> Session -> Task Response
Logout.update p session = {
  let _ <- Session.put (\s -> { s | user: Nothing, role: Member })
  Task.succeed (Response.redirect (Auth.Login.Login {}))
}

error : Error -> Html
error err =
  <Layout.Page title="Error">
    <h1>Something went wrong</h1>
    <p>Please try again.</p>
  </Layout.Page>

notFound : Html
notFound = <Layout.Page title="Not found"><h1>Not found</h1></Layout.Page>

-- Logging out from a page that `requires user` ends at the login page either way: a refresh of
-- that page redirects there too. Only a test of `Logout.update` itself tells a redirect from a
-- refresh.
test "Logout clears the user and goes to log in" = {
  let r <- Test.run (Logout.update {} { user: Just (User.UserId "me"), role: Admin })
  Task.succeed (
    { value: r.value, trace: r.trace }
      == { value: Ok (Response.redirect (Auth.Login.Login {})), trace: ["Session.put"] }
  )
}

-- Logging out also makes the role Member again: an Admin who logs out is no longer one.
test "Logout makes the role Member" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
    role: Admin,
  }
  let _ <- Test.login { user: Just me, role: Admin }
  let _ <- Test.get (Todos.Index.Index {})
  let out <- Test.click Logout
  Task.succeed (
    { out: out.answer, role: out.session.role }
      == { out: Redirected (Auth.Login.Login {}), role: Member }
  )
}

-- Where `alder deploy` ships the app (book 16). `local` runs a script; nothing here is a secret.
deploy { local: Script "./scripts/deploy-local.sh" }

--- file: app/Auth/Login.alder ---
module Auth.Login

import Layout
import Table.User

-- Password checking is outside the language: a capability implemented in Go.
capability Password {
  verify : { email: Text.Email, password: Text } -> Task (Maybe User.UserId),
}

-- In a test, `Test.fake Password` puts this in the adapter's place: it accepts any user who has
-- a row, whatever the password, so a test can log in through the form below (book 10).
fake Password {
  verify args = {
    let found <-
      Db.from User
        |> Db.where (User.email.eq args.email)
        |> Db.first
    Task.succeed (Maybe.map (\u -> u.id) found)
  }
}

route Login  = GET  "/login"   { }
route Submit = POST "/session" { } form Credentials

type Model = { }

Login.load : Login.Params -> Session -> Query Model
Login.load params session = Query.succeed {}

form Credentials = { email: Text.Email, password: Text.Secret }

Submit.update : Submit.Params -> Session -> Task Response
Submit.update p session = {
  let found <- Password.verify { email: p.form.email, password: p.form.password }
  match found {
    -- The session keeps the user and their role as the row has it now: a role changed later
    -- applies when they next log in.
    Just id -> {
      let found <- Db.get User id
      let role = Maybe.withDefault Member (Maybe.map (\u -> u.role) found)
      let _ <- Session.put (\s -> { s | user: Just id, role: role })
      Task.succeed (Response.redirect (Todos.Index.Index {}))
    },
    Nothing -> Task.succeed (Response.invalid "Email or password is incorrect."),
  }
}

Login.view : Model -> Session -> Html
Login.view model session =
  <Layout.Page title="Log in">
    <h1>Log in</h1>
    <al-card>
      <form submit={Submit}>
        <Errors form />
        <al-input field="email" label="Email" />
        <al-input field="password" label="Password" />
        <al-button type="submit">Log in</al-button>
      </form>
    </al-card>
  </Layout.Page>

-- Logging in through the form, with the fake: an unknown email is rejected with the form's
-- message, and a known one stores the user in the session and goes to the to-do list.
test "an unknown email is rejected" = {
  let _ <- Test.fake Password
  let _ <- Test.get (Login {})
  let r <- Test.submit Submit {
    email: Text.Email "nobody@example.com",
    password: Text.Secret "secret",
  }
  Task.succeed (
    { answer: r.answer, user: r.session.user }
      == {
        answer: FormRejected [{ field: "", message: "Email or password is incorrect." }],
        user: Nothing,
      }
  )
}

test "logging in through the form" = {
  let _ <- Test.fake Password
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let _ <- Test.get (Login {})
  let r <- Test.submit Submit { email: Text.Email "me@example.com", password: Text.Secret "secret" }
  Task.succeed (
    {
      answer: r.answer,
      user: r.session.user,
      title: Test.text "h1" r.doc,
    } == {
      answer: Redirected (Todos.Index.Index {}),
      user: Just me,
      title: Just "Todos",
    }
  )
}

-- The session gets the role the user's row has: an Admin logs in as an Admin.
test "logging in copies the role from the user's row" = {
  let _ <- Test.fake Password
  let _ <- Db.insert User {
    email: Text.Email "admin@example.com",
    name: Text.NonEmpty "Admin",
    passwordHash: "",
    role: Admin,
  }
  let _ <- Test.get (Login {})
  let r <- Test.submit Submit {
    email: Text.Email "admin@example.com",
    password: Text.Secret "secret",
  }
  Task.succeed (r.session.role == Admin)
}

test "a visitor is sent to log in, and can log out" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let visitor <- Test.open (Todos.Index.Index {})
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Todos.Index.Index {})
  let out <- Test.click App.Logout
  Task.succeed (
    {
      visitor: visitor.answer,
      out: out.answer,
      user: out.session.user,
    } == {
      visitor: Redirected (Login {}),
      out: Redirected (Login {}),
      user: Nothing,
    }
  )
}

--- file: app/Layout.alder ---
module Layout

Page : { title: Text, children: List Html } -> Html
Page props =
  <html>
    <head>
      <meta charset="utf-8" />
      <script>try{document.documentElement.classList.toggle("dark",localStorage.theme==="dark"||(!("theme" in localStorage)&&matchMedia("(prefers-color-scheme: dark)").matches))}catch(e){}</script>
      <title>{props.title}</title>
      <link rel="stylesheet" href={Url.external "/static/al/al.css"} />
      <script type="module" src="/static/al/al.js"></script>
      <link rel="stylesheet" href={Url.external "/static/app.css"} />
    </head>
    <body>
      <al-page width="narrow"><main><al-stack gap="lg">{props.children}</al-stack></main></al-page>
    </body>
  </html>

--- file: app/Table/Todo.alder ---
module Table.Todo

import Table.User

type TodoId = brand Text

type Filter = All | Active | Completed

-- `owner` is the table's `@owner`: `Db.mine Todo`, `Db.getMine`, `Db.updateMine`, `Db.deleteMine`
-- and `Db.insertMine` read and write only the signed-in user's todos (book 06, "Rules").
table Todo {
  id: TodoId @id,
  title: Text.NonEmpty,
  done: Bool = False,
  due: Maybe Date,
  owner: User.UserId @ref User @owner,
  created: Time @created,
}

matches : Filter -> Todo -> Bool
matches filter todo = match filter {
  All -> True,
  Active -> not todo.done,
  Completed -> todo.done,
}

filterLabel : Filter -> Text
filterLabel filter = match filter {
  All -> "All",
  Active -> "Active",
  Completed -> "Completed",
}

countOpen : List Todo -> Int
countOpen todos =
  todos
    |> List.filter (\t -> not t.done)
    |> List.length

test "countOpen ignores done todos" =
  countOpen [
    {
      id: TodoId "a",
      title: Text.NonEmpty "x",
      done: False,
      due: Nothing,
      owner: User.UserId "u",
      created: Time.epoch,
    },
    {
      id: TodoId "b",
      title: Text.NonEmpty "y",
      done: True,
      due: Nothing,
      owner: User.UserId "u",
      created: Time.epoch,
    },
    {
      id: TodoId "c",
      title: Text.NonEmpty "z",
      done: False,
      due: Nothing,
      owner: User.UserId "u",
      created: Time.epoch,
    },
  ] == 2

test "Active keeps only open todos" =
  List.map (\t -> t.title) (List.filter (matches Active) [
    {
      id: TodoId "a",
      title: Text.NonEmpty "x",
      done: False,
      due: Nothing,
      owner: User.UserId "u",
      created: Time.epoch,
    },
    {
      id: TodoId "b",
      title: Text.NonEmpty "y",
      done: True,
      due: Nothing,
      owner: User.UserId "u",
      created: Time.epoch,
    },
  ]) == [Text.NonEmpty "x"]

test "each filter has its label" =
  List.map filterLabel [All, Active, Completed] == ["All", "Active", "Completed"]

-- `owner` is `@ref User` with the default rule, `@restrict`: a user who still has todos cannot
-- be deleted, and the todos stay.
test "a user with todos cannot be deleted" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let id <- Db.insert Todo {
    title: Text.NonEmpty "Milk",
    due: Nothing,
    owner: me,
  }
  let r <- Test.run (Db.delete User me)
  let after <- Db.get Todo id
  Task.succeed (
    { deleted: Result.toMaybe r.value, todo: Maybe.map (\t -> t.title) after }
      == { deleted: Nothing, todo: Just (Text.NonEmpty "Milk") }
  )
}

--- file: app/Table/User.alder ---
module Table.User

type UserId = brand Text

-- A Member keeps their own to-do list. An Admin can also open the admin that `alder new admin`
-- generated (app/Admin/, every page `requires user, role Admin`; book 15). The session holds
-- the role the user had when they logged in (App's `session`, set by Auth.Login).
type Role = Member | Admin

table User {
  id: UserId @id,
  email: Text.Email @unique,
  name: Text.NonEmpty,
  passwordHash: Text,
  created: Time @created,
  role: Role = Member,
}

--- file: app/Todos/Index.alder ---
module Todos.Index

import Layout
import Table.Todo
import Table.User

route Index  = GET   "/"           { filter: Todo.Filter = All }              requires user
route Submit = POST  "/todos"      { filter: Todo.Filter = All } form NewTodo requires user
route Toggle = PATCH "/todos/{id}" { id: Todo.TodoId }                        requires user

type Model = { filter: Todo.Filter, todos: List Todo }

Index.load : Index.Params -> Session -> Query Model
Index.load params session = {
  let todos <-
    Db.mine Todo
      |> Db.orderBy Todo.created.desc
      |> Db.all
  Query.succeed { filter: params.filter, todos: List.filter (Todo.matches params.filter) todos }
}

form NewTodo = { title: Text.NonEmpty, due: Maybe Date }

-- The filter the page shows rides in the form's URL, so a new todo lands on the same list.
Submit.update : Submit.Params -> Session -> Task Response
Submit.update p session = {
  let _ <- Db.insertMine Todo { title: p.form.title, due: p.form.due }
  Task.succeed (Response.redirect (Index { filter: p.filter }))
}

-- A write's id comes from the request: anyone can post `Toggle` with another user's id.
-- `Db.updateMine` and `Db.deleteMine` change the row only when it is the session's user's;
-- any other id, like a missing one, is a 404 (book 06, "Rules").
Toggle.update : Toggle.Params -> Session -> Task Response
Toggle.update p session = {
  let _ <- Db.updateMine Todo p.id (\t -> { t | done: not t.done })
  Task.succeed Response.refresh
}

Index.view : Model -> Session -> Html
Index.view model session =
  <Layout.Page title="Todos">
    <h1>Todos</h1>

    <al-card>
      <form submit={Submit { filter: model.filter }}>
        <al-input field="title" label="What needs doing?" />
        <al-input field="due" label="Due" />
        <al-button type="submit">Add</al-button>
      </form>
    </al-card>

    <nav>
      {for f in [All, Active, Completed] ->
        <a
          href={Index { filter: f }}
          aria-current={if model.filter == f then Just "page" else Nothing}>
          {Todo.filterLabel f}
        </a>}
    </nav>

    <ul id="todos">
      {for todo in model.todos -> <Item todo={todo} />}
    </ul>

    <p id="summary">{Text.fromInt (Todo.countOpen model.todos)} open</p>

    <al-button variant="outline" click={App.Logout}>Log out</al-button>
  </Layout.Page>

Item : { todo: Todo } -> Html
Item props =
  <li class={if props.todo.done then "done" else ""}>
    <al-button variant="secondary" size="sm" click={Toggle { id: props.todo.id }}>
      {if props.todo.done then "Undo" else "Done"}
    </al-button>
    <a href={Todos.Show.Show { id: props.todo.id }}>{props.todo.title}</a>
    {match props.todo.due {
      Just d -> <al-badge variant="outline" class="due">{Date.format "Jan 2" d}</al-badge>,
      Nothing -> Html.none,
    }}
    <al-button variant="ghost" size="sm" click={Todos.Show.Delete { id: props.todo.id }}>
      Delete
    </al-button>
  </li>

test "Toggle flips done" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let id <- Db.insert Todo {
    title: Text.NonEmpty "Milk",
    due: Nothing,
    owner: me,
  }
  let _ <- Test.login { user: Just me }
  let r <- Test.run (Toggle.update { id: id } { user: me, role: Member })
  let after <- Db.get Todo id
  let done = Maybe.map (\t -> t.done) after
  Task.succeed (
    { trace: r.trace, done: done } == { trace: ["Db.updateMine Todo"], done: Just True }
  )
}

-- A stranger's own page lists none of my todos, but a write's id comes from the request.
-- Here my page is still open in the browser when a stranger logs in on it and presses my todo's
-- Toggle: `Db.updateMine` finds no todo of the stranger's with that id, so the answer is a 404
-- and nothing changes.
test "Toggle by another user is not found" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let id <- Db.insert Todo {
    title: Text.NonEmpty "Milk",
    due: Nothing,
    owner: me,
  }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Index {})
  let _ <- Test.login { user: Just (User.UserId "someone-else") }
  let r <- Test.click (Toggle { id: id })
  let after <- Db.get Todo id
  Task.succeed (
    { answer: r.answer, done: Maybe.map (\t -> t.done) after }
      == { answer: PageMissing, done: Just False }
  )
}

test "Delete by another user is not found" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let id <- Db.insert Todo {
    title: Text.NonEmpty "Milk",
    due: Nothing,
    owner: me,
  }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Index {})
  let _ <- Test.login { user: Just (User.UserId "someone-else") }
  let r <- Test.click (Todos.Show.Delete { id: id })
  let after <- Db.get Todo id
  Task.succeed (
    { answer: r.answer, todo: Maybe.map (\t -> t.title) after }
      == { answer: PageMissing, todo: Just (Text.NonEmpty "Milk") }
  )
}

-- The same page through the browser: the test logs in, opens the page, and uses the form and
-- the buttons the page renders, as a user would (book 10, "Testing through the browser").
test "adding a todo through the page" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Index {})
  let added <- Test.submit (Submit {}) { title: Text.NonEmpty "Milk", due: Nothing }
  let page <- Test.get (Index {})
  Task.succeed (
    {
      answer: added.answer,
      todos: List.map (\t -> t.title) page.model.todos,
      summary: Test.text "#summary" page.doc,
    } == {
      answer: Redirected (Index {}),
      todos: [Text.NonEmpty "Milk"],
      summary: Just "1 open",
    }
  )
}

test "toggling a todo through the page" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let id <- Db.insert Todo {
    title: Text.NonEmpty "Milk",
    due: Nothing,
    owner: me,
  }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Index {})
  let r <- Test.click (Toggle { id: id })
  Task.succeed (
    {
      answer: r.answer,
      done: Test.has "li.done" r.doc,
      summary: Test.text "#summary" r.doc,
    } == {
      answer: Refreshed,
      done: True,
      summary: Just "0 open",
    }
  )
}

-- Each write route has its own `requires user`: a visitor who adds, toggles or deletes from a
-- page still open after the session ended is sent to log in, and nothing changes.
test "a write after logging out is sent to log in" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let id <- Db.insert Todo {
    title: Text.NonEmpty "Milk",
    due: Nothing,
    owner: me,
  }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Index {})
  let _ <- Test.login { user: Nothing }
  let added <- Test.submit (Submit {}) { title: Text.NonEmpty "Eggs", due: Nothing }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Index {})
  let _ <- Test.login { user: Nothing }
  let toggled <- Test.click (Toggle { id: id })
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Index {})
  let _ <- Test.login { user: Nothing }
  let deleted <- Test.click (Todos.Show.Delete { id: id })
  let todos <- Db.from Todo |> Db.all
  Task.succeed (
    {
      added: added.answer,
      toggled: toggled.answer,
      deleted: deleted.answer,
      todos: List.map (\t -> { title: t.title, done: t.done }) todos,
    } == {
      added: Redirected (Auth.Login.Login {}),
      toggled: Redirected (Auth.Login.Login {}),
      deleted: Redirected (Auth.Login.Login {}),
      todos: [{ title: Text.NonEmpty "Milk", done: False }],
    }
  )
}

-- A stranger's page lists none of my todos, so it has no Toggle button to click for them; the
-- todo's own page is a 404 for the stranger.
test "a stranger cannot see my todo" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let id <- Db.insert Todo {
    title: Text.NonEmpty "Milk",
    due: Nothing,
    owner: me,
  }
  let _ <- Test.login { user: Just (User.UserId "someone-else") }
  let page <- Test.get (Index {})
  let show <- Test.open (Todos.Show.Show { id: id })
  Task.succeed (
    { items: Test.count "li" page.doc, show: show.answer } == { items: 0, show: PageMissing }
  )
}

-- The list deletes through the todo's own route, `DELETE /todos/{id}`: one todo, one URL. Its
-- update redirects to the list, so the page comes back without the todo.
test "deleting from the list" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let id <- Db.insert Todo {
    title: Text.NonEmpty "Milk",
    due: Nothing,
    owner: me,
  }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Index {})
  let r <- Test.click (Todos.Show.Delete { id: id })
  let page <- Test.get (Index {})
  let after <- Db.get Todo id
  Task.succeed (
    {
      answer: r.answer,
      todo: Maybe.map (\t -> t.title) after,
      items: Test.count "li" page.doc,
    } == {
      answer: Redirected (Index {}),
      todo: Nothing,
      items: 0,
    }
  )
}

test "the newest todo is listed first" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let _ <- Db.insert Todo {
    title: Text.NonEmpty "First",
    due: Nothing,
    owner: me,
  }
  let _ <- Db.insert Todo {
    title: Text.NonEmpty "Second",
    due: Nothing,
    owner: me,
  }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Index {})
  Task.succeed (Test.texts "li a" page.doc == ["Second", "First"])
}

-- A test may use the scoped operations itself: they act for the test's session, which
-- `Test.login` sets. `Db.mine Todo` is what Me sees, and not the other user's todo.
test "Db.mine is what Me sees" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let other <- Db.insert User {
    email: Text.Email "other@example.com",
    name: Text.NonEmpty "Other",
    passwordHash: "",
  }
  let _ <- Db.insert Todo {
    title: Text.NonEmpty "Milk",
    due: Nothing,
    owner: me,
  }
  let _ <- Db.insert Todo {
    title: Text.NonEmpty "Bread",
    due: Nothing,
    owner: me,
  }
  let _ <- Db.insert Todo {
    title: Text.NonEmpty "Eggs",
    due: Nothing,
    owner: other,
  }
  let _ <- Test.login { user: Just me }
  let r <- Test.run {
    let n <- Db.mine Todo |> Db.count
    Task.succeed n
  }
  Task.succeed (
    { value: r.value, trace: r.trace } == { value: Ok 2, trace: ["Db.mine Todo", "Db.count Todo"] }
  )
}

--- file: app/Todos/Show.alder ---
module Todos.Show

import Layout
import Table.Todo
import Table.User

route Show   = GET    "/todos/{id}" { id: Todo.TodoId }               requires user
-- `Db.getMine` finds the todo only when it is the session's user's: anyone else's, like a missing
-- one, is a 404. The write routes take the id from their path, as the page does, and no `load`
-- runs before them: `Db.updateMine` and `Db.deleteMine` make the same check, so another user's id
-- is a 404 there too (book 06, "Rules").
route Save   = PUT    "/todos/{id}" { id: Todo.TodoId } form EditTodo requires user
route Delete = DELETE "/todos/{id}" { id: Todo.TodoId }               requires user

type Model = { todo: Todo }

Show.load : Show.Params -> Session -> Query Model
Show.load params session = {
  let todo <- Db.getMine Todo params.id
  Query.succeed { todo: todo }
}

form EditTodo = { title: Text.NonEmpty, due: Maybe Date }

Save.update : Save.Params -> Session -> Task Response
Save.update p session = {
  let _ <- Db.updateMine Todo p.id (\t -> { t | title: p.form.title, due: p.form.due })
  Task.succeed (Response.redirect (Todos.Index.Index {}))
}

Delete.update : Delete.Params -> Session -> Task Response
Delete.update p session = {
  let _ <- Db.deleteMine Todo p.id
  Task.succeed (Response.redirect (Todos.Index.Index {}))
}

Show.view : Model -> Session -> Html
Show.view model session =
  <Layout.Page title={model.todo.title}>
    <h1>{model.todo.title}</h1>
    <al-card>
      <form submit={Save { id: model.todo.id }}>
        <al-input field="title" label="Title" value={model.todo.title} />
        <al-input field="due" label="Due" value={model.todo.due} />
        <al-button type="submit">Save</al-button>
      </form>
      <div slot="actions">
        <al-button variant="ghost" href={Todos.Index.Index {}}>Back</al-button>
        <al-button variant="destructive" click={Delete { id: model.todo.id }}>Delete</al-button>
      </div>
    </al-card>
  </Layout.Page>

-- The page through the browser: a visitor is sent to log in; the owner saves the form and
-- deletes the todo with the buttons the page renders.
test "a visitor is sent to log in from a todo" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let id <- Db.insert Todo {
    title: Text.NonEmpty "Milk",
    due: Nothing,
    owner: me,
  }
  let visitor <- Test.open (Show { id: id })
  Task.succeed (visitor.answer == Redirected (Auth.Login.Login {}))
}

test "saving changes the title and the due date" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let id <- Db.insert Todo {
    title: Text.NonEmpty "Milk",
    due: Nothing,
    owner: me,
  }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Show { id: id })
  let r <- Test.submit (Save { id: id }) {
    title: Text.NonEmpty "Bread",
    due: Date.fromText "2026-10-12",
  }
  let after <- Db.get Todo id
  Task.succeed (
    {
      answer: r.answer,
      title: Maybe.map (\t -> t.title) after,
      due: Maybe.andThen (\t -> t.due) after,
    } == {
      answer: Redirected (Todos.Index.Index {}),
      title: Just (Text.NonEmpty "Bread"),
      due: Date.fromText "2026-10-12",
    }
  )
}

test "deleting from the todo's page" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let id <- Db.insert Todo {
    title: Text.NonEmpty "Milk",
    due: Nothing,
    owner: me,
  }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Show { id: id })
  let r <- Test.click (Delete { id: id })
  let after <- Db.get Todo id
  Task.succeed (
    { answer: r.answer, todo: Maybe.map (\t -> t.title) after }
      == { answer: Redirected (Todos.Index.Index {}), todo: Nothing }
  )
}

-- Each write route has its own `requires user`: a visitor who posts Save or Delete from a page
-- still open after the session ended is sent to log in, and the todo is left as it was.
test "saving or deleting after logging out is sent to log in" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let id <- Db.insert Todo {
    title: Text.NonEmpty "Milk",
    due: Nothing,
    owner: me,
  }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Show { id: id })
  let _ <- Test.login { user: Nothing }
  let saved <- Test.submit (Save { id: id }) { title: Text.NonEmpty "Bread", due: Nothing }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Show { id: id })
  let _ <- Test.login { user: Nothing }
  let deleted <- Test.click (Delete { id: id })
  let after <- Db.get Todo id
  Task.succeed (
    {
      saved: saved.answer,
      deleted: deleted.answer,
      todo: Maybe.map (\t -> t.title) after,
    } == {
      saved: Redirected (Auth.Login.Login {}),
      deleted: Redirected (Auth.Login.Login {}),
      todo: Just (Text.NonEmpty "Milk"),
    }
  )
}

-- No `load` runs before a write route: Save and Delete take the id from their path, and a
-- stranger who posts them for my todo, on my page still open in the browser, is answered by
-- their own `Db.updateMine` and `Db.deleteMine` with a 404.
test "saving or deleting another user's todo is not found" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let id <- Db.insert Todo {
    title: Text.NonEmpty "Milk",
    due: Nothing,
    owner: me,
  }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Show { id: id })
  let _ <- Test.login { user: Just (User.UserId "someone-else") }
  let saved <- Test.submit (Save { id: id }) { title: Text.NonEmpty "Bread", due: Nothing }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Show { id: id })
  let _ <- Test.login { user: Just (User.UserId "someone-else") }
  let deleted <- Test.click (Delete { id: id })
  let after <- Db.get Todo id
  Task.succeed (
    {
      saved: saved.answer,
      deleted: deleted.answer,
      todo: Maybe.map (\t -> t.title) after,
    } == {
      saved: PageMissing,
      deleted: PageMissing,
      todo: Just (Text.NonEmpty "Milk"),
    }
  )
}

