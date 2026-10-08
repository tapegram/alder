You are writing code in Alder, a small language for server-rendered web
apps. Alder is not Elm, not TypeScript, and not any language you have seen
before; the reference below is complete. Anything not in the reference does
not exist. Follow it exactly.

====================  ALDER REFERENCE  ====================
# 3. Syntax

## Files and modules

One module per file. The first line names the module, and the module name
is the file path: `Todos.Index` lives in `app/Todos/Index.alder`.

```alder
module Todos.Index

import Todo
import Layout
```

`import X` brings module `X` into scope as a qualified name: `X.foo`,
`X.SomeType`, `<X.Component>`. There are no unqualified imports and no
renaming. Modules from the standard library (`List`, `Text`, `Db`, ...) are
always in scope and never imported.

A type whose name equals the last segment of its module's name is referred
to by the module name alone: module `Todo` defining `type Todo` (or `table
Todo`) is used as `Todo`, not `Todo.Todo`. Page modules are imported like any
other module, and a page value is written `Todos.Show { id: id }`.

Everything a module defines is visible to other modules, with one exception:
a brand's constructor (see chapter 4). There is no `export` list.

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
"a \"quoted\" \{brace} \n"   -- escapes: \" \\ \{ \n \t
[1, 2, 3]     -- List Int
{ x: 1, y: 2 }   -- record
()            -- Unit
```

String interpolation inserts a `Text` only. To insert an `Int`, write
`"Total: {Text.fromInt n}"`.

## Function application

Functions are applied by juxtaposition and are curried:

```alder
add 1 2
List.map (add 1) [1, 2, 3]
```

Use parentheses to group. Use `|>` to pipe a value into the last parameter:

```alder
todos
  |> List.filter (\t -> not t.done)
  |> List.length
```

There is no `<|`, no `<<`, no `>>`.

## Lambdas

```alder
\x -> x + 1
\a b -> a + b
```

## Operators

| Operator | Types | Meaning |
|---|---|---|
| `+ - * /` | `Int` or `Float` (both sides the same) | arithmetic; `/` on `Int` truncates |
| `%` | `Int` | remainder |
| `++` | `Text` or `List a` | concatenation |
| `== !=` | any type without functions (both sides identical) | equality |
| `< <= > >=` | `Int`, `Float`, `Text` | comparison |
| `&& \|\|` | `Bool` | logic, short-circuit |
| `not` | `Bool` | negation (a function, not an operator) |
| `??` | `Maybe a` and `a` | default: `m ?? d` is `d` when `m` is `Nothing` |
| `\|>` | | pipe |

Precedence, highest first: function application; `* / %`; `+ - ++`;
`== != < <= > >=`; `&&`; `||`; `??`; `|>`. There is no user-defined operator.

## Records

```alder
origin : Point
origin = { x: 0, y: 0 }

origin.x                 -- field access
{ origin | x: 10 }       -- copy with fields changed; fields must already exist
```

Record literals are checked against the expected record type. Every field
must be present, and no extra fields are allowed. There is no field punning:
write `{ id: id }`, not `{ id }`.

## Conditionals

```alder
if todo.done then "done" else "open"
```

`if` always has an `else`. Both branches have the same type.

## Match

```alder
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
load : Params -> Session -> Query Model
load params session = {
  let todos <- Db.from Todo |> Db.all
  Query.succeed { filter: params.filter, todos: todos }
}
```

`let x <- ...` is only allowed in a block whose result is a `Query` or
`Task`, and the effect on the right must be of the same kind.

A block is an expression and can appear anywhere an expression can. A
function whose body is a single expression does not need a block.

## Whitespace

Whitespace is not significant except that statements in a block are
separated by newlines. An expression may continue onto the next line
when the line ends with an operator or the next line begins with `|>`,
an operator, or is inside unclosed brackets.

## Reserved words

```
module import type brand where page msg form table capability session
test let match if then else for in requires
```

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
| `Dict k v` | built with `Dict.fromList`; `k` is `Int` or `Text` or a brand of one |
| `Date`, `Time` | calendar date; instant (chapter 9) |

There is no `Char` and no `String`: text is `Text`. There is no tuple: use
a record.

## Type signatures

Every top-level value and function has a signature on the line before it.
Function types use `->`. Type parameters are lowercase names.

```alder
first : List a -> Maybe a
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
record type that happens to have the same fields. An anonymous record
literal `{ x: 1.0, y: 2.0 }` is given the type the context expects. A
signature may spell out a record type inline when it is used only once:

```alder
area : { w: Float, h: Float } -> Float
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
2. **Narrowing is explicit and local.** The constructor `TodoId : Text ->
   TodoId` exists only inside the module that declares the brand. Other
   modules receive brand values from boundaries (route parameters, forms,
   database rows) or from functions the declaring module exports.
3. **Operations return the base type.** `Text.trim id` is a `Text`, not a
   `TodoId`. Exception: `+`, `-`, `*`, `/`, `%` on two values of the same
   numeric brand return that brand (`Cents + Cents : Cents`).
4. **Equality requires identical types.** `id == text` is an error even
   though `id` widens to `Text`.
5. **A brand's base is a built-in type**, never another brand.

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
type Page a = { items: List a, total: Int }
```

## What there is not

- No type aliases. A new name is always a new type.
- No typeclasses, interfaces, or traits.
- No tuples.
- No recursive record types without going through a union.
- No subtyping other than brand-to-base.

# 5. HTML

HTML is a built-in expression form. An HTML expression has type `Html Msg`,
where `Msg` is the message type of the page it can send. HTML that sends
nothing has type `Html Never`.

## Elements

```alder
<ul class="todos">
  <li>First</li>
  <li>Second</li>
</ul>
```

Elements are written as in HTML. Every element is closed: `<li>...</li>` or
`<br />`. Tag names are the standard HTML5 set; an unknown tag is a compile
error.

## Expressions in HTML

Braces embed an expression:

```alder
<span>{todo.title}</span>
<ul>{items}</ul>
```

An embedded expression must have one of these types:

- `Text`: inserted as escaped text.
- `Html Msg`: inserted as markup.
- `List (Html Msg)`: inserted in order.

Nothing else is inserted implicitly. An `Int` must be converted with
`Text.fromInt`. A `Maybe Text` must be matched or defaulted with `??`.

Text is **always escaped**. To insert trusted markup, use `Html.raw`, which
takes a `Text` and returns `Html Never`. It is the only way to produce
unescaped output.

## Control flow in HTML

```alder
{if model.todos == [] then <p>Nothing to do.</p> else <ul>{items}</ul>}

{for todo in model.todos -> <TodoItem todo={todo} />}

{match model.user {
  Just u -> <span>{u.name}</span>,
  Nothing -> <a href={Auth.Login {}}>Log in</a>,
}}
```

`for x in list -> html` produces a `List (Html Msg)`. There is no index
variable; use `List.indexed` first if an index is needed.

## Attributes

```alder
<input type="text" name="title" value={model.draft} />
<li class={if todo.done then "done" else ""}>
<button disabled={model.saving}>
<a href={Todos.Show { id: todo.id }}>
```

Attribute values are either a literal string or a braced expression. The
expression's type depends on the attribute:

| Attribute | Type |
|---|---|
| most attributes (`class`, `id`, `value`, `placeholder`, ...) | `Text` |
| boolean attributes (`disabled`, `checked`, `required`, `hidden`, ...) | `Bool` |
| `href` | a page value (chapter 6) |
| `click`, `submit` | a message value (chapter 6) |
| any attribute | `Maybe Text`: omitted when `Nothing` |

`href` never takes a `Text`. Links to external sites use `<a href={Url.external "https://..."}>`.
Event attributes are not HTML event names (`onclick`); they are `click` and
`submit`, and they take a message, not a function.

`data-*` and `aria-*` attributes take `Text`.

## Components

A component is a function from a record to `Html Msg`, named with an
uppercase letter:

```alder
TodoItem : { todo: Todo } -> Html Msg
TodoItem props =
  <li class={if props.todo.done then "done" else ""}>
    <span>{props.todo.title}</span>
  </li>
```

It is used as an element, with props as attributes:

```alder
<TodoItem todo={todo} />
```

Children are passed through a `children` field of type `List (Html Msg)`:

```alder
Layout : { title: Text, children: List (Html Msg) } -> Html Msg
Layout props =
  <html>
    <head><title>{props.title}</title></head>
    <body>{props.children}</body>
  </html>

<Layout title="Todos">
  <h1>Todos</h1>
</Layout>
```

Components are plain functions. They have no state and no lifecycle.

## Forms and inputs

Inside `<form submit={Msg}>`, the `<Input>`, `<Select>`, `<Textarea>`, and
`<Errors>` components are bound to that message's form type (chapter 6):

```alder
<form submit={Submit}>
  <Errors form />
  <Input field="title" label="Title" />
  <Errors field="title" />
  <button>Add</button>
</form>
```

`<Errors field="x" />` shows the errors for one field; `<Errors form />`
shows form-level errors (from `Reject`).

`field` must name a field of the form type. The input's `name`, `type`,
`required`, and current value are derived from the field's type and the
submitted values. `value={...}` (a `Text` or `Maybe Text`) sets the initial
value for an edit form; a submitted value takes precedence over it. A field
of type `Text.Secret` renders as a password input and is never echoed back.
A plain `<input>` inside a `<form submit={...}>` is a compile error; use
`<Input>`.

## Partial updates

Add `id` attributes to the regions that change. After a message, the
runtime re-renders the page and sends the elements whose `id` is present on
both the old and new page; Datastar morphs them in place. Without Datastar,
the whole page is replaced. Nothing else is needed.

# 6. Pages, messages, forms

## A page module

A page is a module that declares a route and the five things that go with
it. This is the whole shape of an Alder app.

```alder
module Todos.Index

page "/" { filter: Filter = All }

type Model = { filter: Filter, todos: List Todo }

load : Params -> Session -> Query Model
load params session = {
  let todos <- Db.from Todo |> Db.all
  Query.succeed { filter: params.filter, todos: todos }
}

view : Model -> Html Msg
view model = ...

msg Toggle = "/todos/{id}/toggle" { id: TodoId }

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Toggle p -> ...,
}
```

The compiler checks that each page module has exactly: one `page`, one
`Model`, one `load`, one `view`, and, if it has any `msg`, one `update`.
`Params` and `Msg` are generated from the declarations and must not be
written by hand.

## `page`

```alder
page "/todos/{id}" { id: TodoId }
page "/" { filter: Filter = All, q: Maybe Text }
page "/admin" { } requires user
```

- The path is a literal with `{name}` segments. Each segment must appear in
  the parameter record; parameters not in the path are query parameters.
- Parameter types must be one of: `Int`, `Text`, `Bool`, a brand of those,
  a union with no payloads, `Maybe` of those, or `List` of those.
- A parameter with a default is optional in links and in the URL.
- `Params` is the parameter record type for this module.
- `requires` names a field of `Session` whose type is `Maybe a`. The page is
  served only when that field is `Just`; otherwise the runtime redirects to
  the page named in `App.login`. Inside this module `session.<field>` has type
  `a`, not `Maybe a`.

A page value is the module name applied to its parameter record. It is the
only thing `href` accepts:

```alder
<a href={Todos.Show { id: todo.id }}>
<a href={Todos.Index { filter: Active }}>     -- defaulted fields may be omitted
```

## `msg`

A message is a POST route belonging to the page that declares it.

```alder
msg Toggle = "/todos/{id}/toggle" { id: TodoId }
msg Submit = "/todos" form NewTodo
msg Comment = "/posts/{post}/comments" { post: PostId } form NewComment
```

- Path and parameters follow the same rules as `page`.
- `form F` means the request body is decoded as form type `F`.
- `Msg` is the union of the module's messages. Each constructor takes its
  parameter record. If the message has a form, the record has an extra
  field `form: F` that the runtime fills in.

A message value in HTML is the constructor applied to its parameters,
without `form`:

```alder
<button click={Toggle { id: todo.id }}>
<form submit={Comment { post: model.post.id }}>
<form submit={Submit}>                         -- no parameters: bare constructor
```

In `update`, the same constructor carries the full record:

```alder
update msg model session = match msg {
  Toggle p -> ...,                 -- p.id : TodoId
  Submit p -> ...,                 -- p.form : NewTodo
  Comment p -> ...,                -- p.post : PostId, p.form : NewComment
}
```

The page's own parameters travel with every message it sends, so `update`
always has the same `Model` that `view` had.

## `form`

```alder
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

When a submitted form fails to decode (missing field, failed refinement,
wrong type), the runtime does **not** call `update`. It re-renders the page
with the submitted values and errors attached to that form; `<Input>` and
`<Errors>` show them. The program never sees an invalid form.

## `update` and `Nav`

```alder
update : Msg -> Model -> Session -> Task Nav
```

`update` returns a `Task` (chapter 7) that ends in a `Nav`:

```alder
type Nav
  = Rerender                 -- re-run load and view for this page, patch the result in
  | Redirect Page            -- HTTP 303 to another page
  | Reject Text              -- re-render with this error shown on the form that sent the msg
```

Use `Rerender` for in-place changes (toggle, delete). Use `Redirect` after
creating something, so a browser refresh doesn't resubmit. Use `Reject` when
a well-formed submission is refused for a reason only `update` can know
(wrong password, name already taken); `<Errors form />` shows the text.

## The request lifecycle

```
GET  /path
  1. decode route parameters      → 404 on failure
  2. check `requires`             → redirect to App.login on failure
  3. load                         → Query, run by the runtime
  4. view                         → HTML

POST /path
  1. decode route parameters and form   → 404 / re-render with form errors
  2. check `requires`
  3. load                         → Model
  4. update                       → Task, run by the runtime
  5. Rerender: load → view → patch     Redirect: 303
     Task failure: App.error → HTML, status 500
```

## The `App` module

Every app has an `App` module:

```alder
module App

session { user: Maybe UserId, theme: Theme = Light }

login = Auth.Login

error : Error -> Html Never
error err = <h1>Something went wrong</h1>

notFound : Html Never
notFound = <h1>Not found</h1>
```

- `session` declares the session record. The runtime stores it in a signed
  cookie. Field types follow the same rules as form fields. `Session.put`
  (chapter 7) changes it.
- `login` names the page to redirect to when `requires` fails.
- `error` and `notFound` render failure pages.

# 7. Effects

Alder code never performs an effect. It returns a description of effects,
and the runtime carries them out. There are two kinds.

| Type | May | Used by |
|---|---|---|
| `Query a` | read | `load` |
| `Task a` | read and write | `update` |

A `Query` cannot write. A `Task` can do anything a `Query` can, plus writes.
`load` returns a `Query`, so a page can never change anything by being
viewed.

## Building effects

Effects are built in effect blocks with `let x <- e`:

```alder
load : Params -> Session -> Query Model
load params session = {
  let post <- Db.get Post params.id
  let comments <- Db.from Comment |> Db.where (Comment.post.eq params.id) |> Db.all
  Query.succeed { post: post, comments: comments }
}
```

Each `let x <- e` runs `e` and binds its result. The block's final expression
is the result of the whole effect. `Query.succeed` and `Task.succeed` wrap a
plain value.

The same shape works for `Task`:

```alder
update msg model session = match msg {
  Submit p -> {
    let id <- Db.insert Todo { title: p.form.title, done: False }
    Task.succeed (Redirect (Todos.Show { id: id }))
  },
  Toggle p -> {
    let _ <- Db.update Todo p.id (\t -> { t | done: not t.done })
    Task.succeed Rerender
  },
}
```

A `Query` can be used inside a `Task` block directly. A `Task` cannot be
used inside a `Query` block.

## Functions on effects

```alder
Query.succeed : a -> Query a
Query.map     : (a -> b) -> Query a -> Query b
Query.all     : List (Query a) -> Query (List a)
Query.notFound : Query a            -- stop: respond 404 with App.notFound

Task.succeed  : a -> Task a
Task.fail     : Text -> Task a
Task.map      : (a -> b) -> Task a -> Task b
Task.all      : List (Task a) -> Task (List a)
```

There is no `andThen`; use `let x <-` in a block.

## Failure

A `Task` can fail. `Task.fail "message"` fails explicitly; a capability may
fail on its own (database down, email rejected). A failed task stops
`update`, and the runtime renders `App.error`. There is no `try`/`catch`:
if a failure should be handled, the capability returns a `Result` instead
of failing.

A `Query` cannot fail. A missing row is `Nothing`; a database outage is
an `App.error` page.

## Capabilities

A capability is a typed interface the program uses and the host implements
in Go:

```alder
capability Email {
  send : { to: Text.Email, subject: Text, body: Html Never } -> Task Unit,
}

capability Weather {
  current : { city: Text } -> Query (Maybe Float),
}
```

Each entry is a function whose result is a `Query` or `Task`. Using
`Email.send { ... }` in `update` is then an ordinary effect. The compiler
generates a Go interface for each capability, and the app will not build
until every capability is implemented and wired in.

Capabilities are declared in any module and used by their name. The
standard library provides these; they need no declaration:

```alder
Db          -- chapter 8
Session.put : (Session -> Session) -> Task Unit
Log.info    : Text -> Task Unit
Log.warn    : Text -> Task Unit
Time.now    : Query Time
Random.int  : { min: Int, max: Int } -> Task Int
Random.id   : Task Text              -- 20-character URL-safe id
Http.get    : { url: Text } -> Task (Result HttpError Text)
Http.post   : { url: Text, body: Text } -> Task (Result HttpError Text)
```

Nothing else in the language touches the outside world. If a function's
signature has no `Query` or `Task` in it, it is pure.

# 8. Database

## Tables

A table is declared with `table`. It defines a record type of the same name
and the storage for it.

```alder
table Todo {
  id: TodoId @id,
  title: Text.NonEmpty,
  done: Bool = False,
  owner: UserId @ref User,
  created: Time @created,
}
```

- Exactly one field has `@id`. Its type is a brand of `Text`; the runtime
  generates the value on insert.
- `@ref T` marks a foreign key to table `T`'s id. The field's type must be
  `T`'s id type, or `Maybe` of it.
- `@created` is set to the current time on insert. `@updated` is set on
  every write.
- A field with a default may be omitted on insert.
- `@unique` marks a field that must be unique.
- Field types: `Int`, `Float`, `Bool`, `Text`, `Date`, `Time`, brands of
  those, unions with no payloads, and `Maybe` of any of those. Nothing
  nested; model nesting with a second table and `@ref`.

A table is declared in its own module named after it (`Todo` in
`app/Todo.alder`), together with the brand for its id and any functions
that belong to it.

## Reading

Reads are `Query` values.

```alder
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
Todo.title.eq   : Text -> Condition Todo         -- also .ne
Todo.created.lt : Time -> Condition Todo         -- also .le .gt .ge
Todo.title.contains : Text -> Condition Todo     -- substring, case-insensitive
Todo.owner.in   : List UserId -> Condition Todo
Todo.due.isNothing : Condition Todo              -- for Maybe fields; also .isJust
Todo.created.asc, Todo.created.desc : Order Todo

Db.and : List (Condition t) -> Condition t
Db.or  : List (Condition t) -> Condition t
Db.not : Condition t -> Condition t
```

Several `Db.where` calls are combined with `and`.

## Writing

Writes are `Task` values.

```alder
Db.insert : Table t -> Insert t -> Task (Id t)
Db.update : Table t -> Id t -> (t -> t) -> Task Unit
Db.delete : Table t -> Id t -> Task Unit
```

`Insert t` is the table record without `@id`, `@created`, and `@updated`
fields, and with defaulted fields optional. `Db.update` reads the row,
applies the function, and writes it back; updating a row that doesn't exist
fails the task. The function may not change the `@id` field.

```alder
let id <- Db.insert Todo { title: p.form.title, owner: session.user }
let _ <- Db.update Todo id (\t -> { t | done: True })
let _ <- Db.delete Todo id
```

Writes in one `update` run in a single transaction. If the task fails,
nothing is written.

## Migrations

The compiler keeps a snapshot of the schema in `db/schema.snapshot`. When
the tables change, `alder build` writes a migration to `db/migrations/` with
the SQL to go from the snapshot to the new schema, and updates the snapshot.
Migrations run on startup. A change the compiler cannot migrate safely
(changing a column's type, removing a `Maybe`) is a build error until the
migration is edited by hand.

## Beyond CRUD

Joins, aggregates, and full-text search are not in the language. Declare a
capability and implement it in Go against the same database:

```alder
capability Reports {
  todosPerUser : Query (List { user: UserId, count: Int }),
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
List.sortBy   : (a -> Int) -> List a -> List a
List.sortByText : (a -> Text) -> List a -> List a
List.indexed  : List a -> List { index: Int, item: a }
List.range    : Int -> Int -> List Int        -- inclusive
List.concat   : List (List a) -> List a
List.append   : List a -> List a -> List a    -- same as ++
List.unique   : List a -> List a
List.groupBy  : (a -> Text) -> List a -> Dict Text (List a)
```

## Maybe

```alder
Maybe.map      : (a -> b) -> Maybe a -> Maybe b
Maybe.andThen  : (a -> Maybe b) -> Maybe a -> Maybe b
Maybe.withDefault : a -> Maybe a -> a          -- same as ??
Maybe.isJust   : Maybe a -> Bool
Maybe.toList   : Maybe a -> List a
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
Text.NonEmpty, Text.Email, Text.Url, Text.Secret   -- refinement brands
```

## Int, Float

```alder
Int.toFloat  : Int -> Float
Int.min      : Int -> Int -> Int
Int.max      : Int -> Int -> Int
Int.abs      : Int -> Int
Int.clamp    : { min: Int, max: Int } -> Int -> Int
Int.Positive, Int.NonNegative                -- refinement brands

Float.round  : Float -> Int
Float.floor  : Float -> Int
Float.ceil   : Float -> Int
Float.min, Float.max, Float.abs
Float.format : { decimals: Int } -> Float -> Text
```

## Bool

```alder
not : Bool -> Bool
Bool.toText : Bool -> Text
```

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

Time.now       : Query Time
Time.toDate    : Time -> Date
Time.format    : Text -> Time -> Text
Time.ago       : Time -> Time -> Text           -- Time.ago now then = "3 hours ago"
```

## Html

```alder
Html.raw   : Text -> Html Never                 -- unescaped, trusted markup
Html.none  : Html a                             -- renders nothing
Html.text  : Text -> Html a
Url.external : Text -> Page                     -- external link for href
```

## Query, Task, Db, Session, Log, Time, Random, Http

See chapters 7 and 8.

## Form

```alder
Form.values : Form f -> f            -- for use in tests only
```

# 10. Testing

A test is a declaration whose body is a `Bool`:

```alder
test "summary counts open todos" =
  summary [{ id: TodoId "a", title: "x", done: False, owner: UserId "u", created: Time.epoch }] == "1 open"

test "matches Active excludes done" =
  not (matches Active { id: TodoId "a", title: "x", done: True, owner: UserId "u", created: Time.epoch })
```

Tests live in the module they test, after the code. `alder test` runs every
test in the project and reports each failing test with both sides of the
final `==` if the body is an equality.

Because `load`, `view`, and `update` are pure functions that return
descriptions, they can be tested without a database or a server:

```alder
test "delete removes the row and stays on the page" =
  update (Delete { id: TodoId "a" }) model session
    == { let _ <- Db.delete Todo (TodoId "a")
         Task.succeed Rerender }
```

Two effect values are equal when they describe the same operations with
the same arguments. An effect that contains a function (`Db.update`) cannot
be compared with `==`; test the function separately. HTML values are equal
when they render to the same markup.

Within a test, a brand constructor may be used from any module.


====================  EXAMPLE APP: examples/todos  ====================
A complete, correct Alder application. Match its style exactly.

--- file: app/App.alder ---
module App

import User
import Layout
import Auth.Login

session { user: Maybe User.UserId }

login = Auth.Login

error : Error -> Html Never
error err =
  <Layout.Page title="Error">
    <h1>Something went wrong</h1>
    <p>Please try again.</p>
  </Layout.Page>

notFound : Html Never
notFound =
  <Layout.Page title="Not found">
    <h1>Not found</h1>
  </Layout.Page>

--- file: app/Auth/Login.alder ---
module Auth.Login

import User
import Layout
import Todos.Index

-- Password checking is outside the language: a capability implemented in Go.
capability Password {
  verify : { email: Text.Email, password: Text } -> Task (Maybe User.UserId),
}

page "/login" { }

type Model = { }

load : Params -> Session -> Query Model
load params session = Query.succeed { }

form Credentials = {
  email: Text.Email,
  password: Text.Secret,
}

msg Submit = "/login" form Credentials
msg Logout = "/logout" { }

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Submit p -> {
    let found <- Password.verify { email: p.form.email, password: p.form.password }
    match found {
      Just id -> {
        let _ <- Session.put (\s -> { s | user: Just id })
        Task.succeed (Redirect (Todos.Index {}))
      },
      Nothing -> Task.succeed (Reject "Email or password is incorrect."),
    }
  },
  Logout p -> {
    let _ <- Session.put (\s -> { s | user: Nothing })
    Task.succeed (Redirect (Auth.Login {}))
  },
}

view : Model -> Html Msg
view model =
  <Layout.Page title="Log in">
    <h1>Log in</h1>
    <form submit={Submit}>
      <Errors form />
      <Input field="email" label="Email" />
      <Errors field="email" />
      <Input field="password" label="Password" />
      <Errors field="password" />
      <button>Log in</button>
    </form>
  </Layout.Page>

--- file: app/Layout.alder ---
module Layout

Page : { title: Text, children: List (Html a) } -> Html a
Page props =
  <html>
    <head>
      <meta charset="utf-8" />
      <title>{props.title}</title>
      <link rel="stylesheet" href={Url.external "/static/app.css"} />
    </head>
    <body>
      <main>{props.children}</main>
    </body>
  </html>

--- file: app/Todo.alder ---
module Todo

import User

type TodoId = brand Text

type Filter = All | Active | Completed

table Todo {
  id: TodoId @id,
  title: Text.NonEmpty,
  done: Bool = False,
  due: Maybe Date,
  owner: User.UserId @ref User,
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
countOpen todos = todos |> List.filter (\t -> not t.done) |> List.length

test "countOpen ignores done todos" =
  countOpen [
    { id: TodoId "a", title: "x", done: False, due: Nothing, owner: User.UserId "u", created: Time.epoch },
    { id: TodoId "b", title: "y", done: True, due: Nothing, owner: User.UserId "u", created: Time.epoch },
  ] == 1

--- file: app/Todos/Index.alder ---
module Todos.Index

import Todo
import User
import Layout
import Todos.Show

page "/" { filter: Todo.Filter = All } requires user

type Model = {
  filter: Todo.Filter,
  todos: List Todo,
}

load : Params -> Session -> Query Model
load params session = {
  let todos <-
    Db.from Todo
      |> Db.where (Todo.owner.eq session.user)
      |> Db.orderBy Todo.created.desc
      |> Db.all
  Query.succeed { filter: params.filter, todos: List.filter (Todo.matches params.filter) todos }
}

form NewTodo = {
  title: Text.NonEmpty,
  due: Maybe Date,
}

msg Submit = "/todos" form NewTodo
msg Toggle = "/todos/{id}/toggle" { id: Todo.TodoId }
msg Delete = "/todos/{id}/delete" { id: Todo.TodoId }

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Submit p -> {
    let _ <- Db.insert Todo { title: p.form.title, due: p.form.due, owner: session.user }
    Task.succeed (Redirect (Todos.Index { filter: model.filter }))
  },
  Toggle p -> {
    let _ <- Db.update Todo p.id (\t -> { t | done: not t.done })
    Task.succeed Rerender
  },
  Delete p -> {
    let _ <- Db.delete Todo p.id
    Task.succeed Rerender
  },
}

view : Model -> Html Msg
view model =
  <Layout.Page title="Todos">
    <h1>Todos</h1>

    <form submit={Submit}>
      <Input field="title" label="What needs doing?" />
      <Errors field="title" />
      <Input field="due" label="Due" />
      <Errors field="due" />
      <button>Add</button>
    </form>

    <nav>
      {for f in [All, Active, Completed] ->
        <a href={Todos.Index { filter: f }} aria-current={if model.filter == f then Just "page" else Nothing}>
          {Todo.filterLabel f}
        </a>}
    </nav>

    <ul id="todos">
      {for todo in model.todos -> <Item todo={todo} />}
    </ul>

    <p id="summary">{Text.fromInt (Todo.countOpen model.todos)} open</p>
  </Layout.Page>

Item : { todo: Todo } -> Html Msg
Item props =
  <li class={if props.todo.done then "done" else ""}>
    <button click={Toggle { id: props.todo.id }}>{if props.todo.done then "Undo" else "Done"}</button>
    <a href={Todos.Show { id: props.todo.id }}>{props.todo.title}</a>
    {match props.todo.due {
      Just d -> <span class="due">{Date.format "Jan 2" d}</span>,
      Nothing -> Html.none,
    }}
    <button click={Delete { id: props.todo.id }}>Delete</button>
  </li>

--- file: app/Todos/Show.alder ---
module Todos.Show

import Todo
import Layout
import Todos.Index

page "/todos/{id}" { id: Todo.TodoId } requires user

type Model = { todo: Todo }

load : Params -> Session -> Query Model
load params session = {
  let found <- Db.get Todo params.id
  match found {
    Just todo -> if todo.owner == session.user then Query.succeed { todo: todo } else Query.notFound,
    Nothing -> Query.notFound,
  }
}

form EditTodo = {
  title: Text.NonEmpty,
  due: Maybe Date,
}

msg Save = "/todos/{id}/save" { id: Todo.TodoId } form EditTodo
msg Delete = "/todos/{id}/delete" { id: Todo.TodoId }

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Save p -> {
    let _ <- Db.update Todo p.id (\t -> { t | title: p.form.title, due: p.form.due })
    Task.succeed (Redirect (Todos.Index {}))
  },
  Delete p -> {
    let _ <- Db.delete Todo p.id
    Task.succeed (Redirect (Todos.Index {}))
  },
}

view : Model -> Html Msg
view model =
  <Layout.Page title={model.todo.title}>
    <h1>{model.todo.title}</h1>
    <form submit={Save { id: model.todo.id }}>
      <Input field="title" label="Title" value={model.todo.title} />
      <Errors field="title" />
      <Input field="due" label="Due" value={Maybe.map Date.toText model.todo.due} />
      <Errors field="due" />
      <button>Save</button>
    </form>
    <button click={Delete { id: model.todo.id }}>Delete</button>
    <a href={Todos.Index {}}>Back</a>
  </Layout.Page>

--- file: app/User.alder ---
module User

type UserId = brand Text

table User {
  id: UserId @id,
  email: Text.Email @unique,
  name: Text.NonEmpty,
  passwordHash: Text,
  created: Time @created,
}

