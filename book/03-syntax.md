# 3. Syntax

## Files and modules

One module per file. The first line names the module, and the module name
is the file path: `Todos.Index` lives in `app/Todos/Index.alder`.

```alder fragment
module Todos.Index

import Todo
import Layout
```

`import X` brings module `X` into scope under a short name, its
*qualifier*: `X.foo`, `X.SomeType`, `<X.Component>`. Modules from the
standard library (`List`, `Text`, `Db`, ...) are always in scope and never
imported. For the same reason no module of the app may take a
standard-library module's name: an app module `Test` would be hidden behind
the stdlib's `Test`, so it is refused (E0017); call it `TestHelpers` or
`Todos.Fixtures` instead.

**The qualifier.** A module is named by the last segment of its path:
after `import Table.Todo` you write `Todo.find`, never `Table.Todo.find`.
Most imports in an app have no neighbour that ends the same way, so the
qualifier is the last segment and the import line is plain `import
Table.Todo`. When two imports would share a qualifier, or the qualifier
would hide something, the import gets a longer one and says so with `as`:

```alder fragment
module Todos

import Admin.Layout as AdminLayout
import Layout
import Table.Todo as TableTodo
```

Here `Layout` is the plain layout, `AdminLayout.Page` is the admin one, and
`TableTodo` is the table of `app/Table/Todo.alder`, because `Todos`
declares a `table Todo` of its own and `Todo` is already spoken for.

There is no choice in the spelling. The compiler computes exactly one
qualifier for each import, the *canonical* one, and the `as` must match it:
an import whose qualifier is its last segment has no `as` (a redundant one
is E0204), and any other import must spell the canonical `as` (a missing
or different one is E0204 as well, and the error's fix rewrites the line).
You never invent an alias; you copy the one the error shows, or run
`alder fmt`. The compiler looks every import up by its canonical
qualifier whatever alias was written, so a file with `as AL` and uses of
`AL.x` reports E0204 and unknown names until the alias is fixed.

**How the compiler computes it.** The resolver works on one file's imports
at a time. Every import starts with a one-segment candidate, the last
segment: `Table.Todo` starts as `Todo`. It lengthens a candidate by one
segment from the right (`Todo` becomes `TableTodo`, the segments joined
with no separator) while either of these holds:

- another import of the file has the same candidate (two imports ending in
  `Layout`, or two ending in `Index`); or
- the candidate is *taken*: a standard-library module name or `App`, the
  last segment of the file's own module name, or the name of a type,
  table, capability or fake table the file declares.

It stops when the candidate is free or when the whole path is in it. The
imports of one file, worked:

| imports | qualifiers | why |
|---|---|---|
| `Table.Todo` | `Todo` | nothing else ends in `Todo` |
| `Layout`, `Admin.Layout` | `Layout`, `AdminLayout` | both start as `Layout`; `Layout` is already its whole path and keeps it, so only `Admin.Layout` grows |
| `Posts.Index`, `Admin.Posts.Index` | `PostsIndex`, `AdminPostsIndex` | both start as `Index`, then both read `PostsIndex`; `Posts.Index` has no more segments, so only the other grows |
| `Utils.List` | `UtilsList` | `List` is a standard-library module |
| `Data.MnSos` in module `Sites.MnSos` | `DataMnSos` | `MnSos` is the module's own last segment |
| `Table.Todo`, with a `table Todo` declared in the file | `TableTodo` | `Todo` is a declared name |
| `Todos.Index`, `Projects.Index`, `Blog.Show` | `TodosIndex`, `ProjectsIndex`, `Show` | only the two `Index` imports collide |

Two details. The file's own last segment blocks a candidate only while
the path can still grow: inside module `Admin.Layout`, `import Layout` is
legal and keeps `Layout`, since its whole path is all there is. Mind what
that means to a reader: inside `Admin.Layout`, `Layout.Page` names the
*other* module, the plain layout, not the file you are in. And some imports
cannot be saved: `A.X` and `AX` in the same file both end as the whole
join `AX`, so neither has a usable qualifier. That is E0206, and its fix
suggests renaming a module. The generators of chapter 15 and the language
server's completions use this same computation, so what they write is
always canonical.

**Always the qualifier, never the path.** Writing the full path of an
imported module where its qualifier belongs is E0205: `Table.Todo.find`
for `import Table.Todo` must be `Todo.find`, and `<Admin.Layout.Page>`
must be `<AdminLayout.Page>`. The compiler still understands the name, so
that one error is the only one, and its fix is the qualified spelling. The
same holds for the members of an imported page module: with `import
Todos.Index`, its helper is `Index.helper`, not `Todos.Index.helper`.
`alder fmt` applies the E0204 and E0205 fixes for a file of an app project
(chapter 11), so one run migrates an older project.

A declaration whose name equals the qualifier is referred to by the
qualifier alone: module `Todo` defining `type Todo` (or `table Todo`) is
used as `Todo`, not `Todo.Todo`; module `Layout` defining a component
`Layout` is used as `<Layout>`. With `import Table.Todo` that is `Todo`
for the table in `app/Table/Todo.alder`, and with `import Table.Todo as
TableTodo` it is `TableTodo`.

**Addresses in prose.** When the book or a compiler message names a
declaration of a page by where it lives, it writes the whole address,
`Todos.Index.load` or `Todos.Index.recent`. That is a name for the reader,
not code to type: in a file you write `Index.recent` after `import
Todos.Index`.

**Pages and app-level messages are global.** A page value (`Todos.Show { id:
id }`), an app-level message (`App.Logout`), and their type `App.Msg` can be
used from any module without an import. Everything else a module uses must
be imported, and imports may not form a cycle.

**Constructors are never qualified.** If `Todo` defines `type Priority = Low
| Normal | High`, a module that imports `Todo` writes `Normal`, not
`Todo.Normal`, while the type is still `Todo.Priority`. Constructor names
must therefore be unique across a module's imports and the standard library;
a clash is a compile error.

A constructor may share its name with a table, which a role makes likely
(`type Role = Reader | Member` beside `table Member`). Then the type
expected decides: where a table is expected, as the first argument of a
`Db.` operation (`Db.from Member`) or a `Table Member` parameter, it is
the table; where the union is expected (`role: Member` in a session
record, `session.role == Member`), the constructor. Anywhere else (an
unannotated `let who = Member`) it is an error naming both: "`Member` is
both a constructor of `User.Role` and a table: write `Member` for the
constructor only where a `User.Role` is expected, or qualify the table
`Member.Member`".

Everything a module defines is visible to other modules, with one exception:
a brand's constructor (see chapter 4). There is no `export` list.

## Names

Names are letters, digits and `_`. A name that starts with a lowercase
letter or `_` is a value, a field or a type parameter; one that starts with
an uppercase letter is a type, a constructor, a module or a component. A
name may not contain two underscores in a row (`max__len` is an error), and
a module name segment may not end with `_`: the compiler joins the parts of
the Go names it generates with `__` (chapter 13), so these are the names it
keeps for itself.

In a page module and in `App`, the compiler also generates the types
`Params`, `Msg`, `Session` and, for each `msg Pick`, `PickParams`. A type,
table, form or component of yours may not take one of those names (E0012).
Nor may a type, table, form or union of any module take the name of a
Prelude type that programs write (`Query`, `Task`, `Html`, `Maybe`,
`Result`, `Nav`, `List`, `Dict`, `Int`, `Float`, `Text`, `Bool`, `Unit`,
`Date`, `Time`, `Never`, `Error`), which every module uses unqualified
(E0012 too). Nor may one take `Doc` or `Answer`, the Prelude's types
for tests (chapter 10), and no constructor or `msg` may be named like
one of `Answer`'s: `Shown`, `Rerendered`, `Redirected`, `FormRejected`,
`PageMissing`, `RequestFailed`. Db's own types, such as `Order` and `Page`, stay
free: `table Order` is fine.

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

-- Page, msg, form, table, capability, test (later chapters)
```

Names of values and functions start with a lowercase letter. Names of types,
constructors, modules, and components start with an uppercase letter.
Multi-word names are `camelCase` or `PascalCase`.

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

Patterns do not nest more than one level deep, except for records. Match on
the inner value in a second `match`.

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

- 2-space indent, no trailing whitespace, one blank line between
  declarations, a signature directly above its value. Lines break at 100
  columns where no rule below decides.
- Imports are sorted, one per line.
- A record, record type, form, table, page-params list, `session` or
  `deploy` with 3 or more fields is always vertical, one field per line
  with a trailing comma; with 1 or 2 it stays on one line if it fits. The
  record type of a function's signature, a capability or a fake breaks
  only when too wide. An empty record expression or pattern is `{}`; an
  empty record type, form or declaration body is `{ }`.
- A record update `{ r | a: x }` follows the same field-count rule, but
  never takes a trailing comma (the grammar forbids it): vertical, it is
  `{ r |` on the opening line, one field per line, `}` alone.
- A `match` has one arm per line, each ending in a comma. A sum type that
  does not fit on a line puts the first constructor bare on the line after
  `=`, and each of the rest on its own line led by `|`:
  `type Msg =` / `  Increment` / `  | Decrement` / `  | SetValue Int`.
- `if` that does not fit on a line is `if c then` / `else` with the
  branches indented; `else if` chains stay at one indent. A block branch is
  `if c then {` ... `} else {` ... `}`.
- A pipeline with 2 or more `|>` is always vertical: it starts on the line
  after the `=` (or `let x =`, `let x <-`), and each `|>` leads a line of
  its own, indented 2 more. A lambda argument is parenthesised:
  `List.map (\x -> x + 1) xs`.
- A block with statements is vertical. Parentheses are only the ones
  precedence needs, except around a lambda argument.
- Markup: an element with 2 or more element children (a `{for}` counts as 2)
  puts each child on its own line.
- Comments stay where they are; a trailing comment keeps its list vertical.

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
module import type brand where page msg form table capability fake session
login test let match if then else for in requires
```

The declaration-head words (`module import type page msg form table
capability fake session login test`) are reserved only at the start of a line
(column 1). Elsewhere they are ordinary names, so `load params session = …`
and `{ form: 1 }` are valid. The remaining words are reserved everywhere.
