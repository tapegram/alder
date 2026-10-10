You are writing code in Alder, a small language for server-rendered web
apps. Alder is not Elm, not TypeScript, and not any language you have seen
before; the reference below is complete. Anything not in the reference does
not exist. Follow it exactly.

====================  ALDER REFERENCE  ====================
# 3. Syntax

## Files and modules

One module per file. The first line names the module, and the module name
is the file path: `Todos.Index` lives in `app/Todos/Index.alder`.

```alder fragment
module Todos.Index

import Todo
import Layout
```

`import X` brings module `X` into scope as a qualified name: `X.foo`,
`X.SomeType`, `<X.Component>`. There are no unqualified imports and no
renaming. Modules from the standard library (`List`, `Text`, `Db`, ...) are
always in scope and never imported. For the same reason no module of the
app may take a standard-library module's name: an app module `Test` would
be hidden behind the stdlib's `Test`, so it is refused (E0017); call it
`TestHelpers` or `Todos.Fixtures` instead.

A declaration whose name equals the last segment of its module's name is
referred to by the module name alone: module `Todo` defining `type Todo` (or
`table Todo`) is used as `Todo`, not `Todo.Todo`; module `Layout` defining a
component `Layout` is used as `<Layout>`.

**Pages and app-level messages are global.** A page value (`Todos.Show { id:
id }`), an app-level message (`App.Logout`), and their type `App.Msg` can be
used from any module without an import. Everything else a module uses must
be imported, and imports may not form a cycle.

**Constructors are never qualified.** If `Todo` defines `type Priority = Low
| Normal | High`, a module that imports `Todo` writes `Normal`, not
`Todo.Normal`, while the type is still `Todo.Priority`. Constructor names
must therefore be unique across a module's imports and the standard library;
a clash is a compile error.

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
when the line ends with an operator or the next line begins with `|>`,
an operator, or is inside unclosed brackets.

## Reserved words

```
module import type brand where page msg form table capability session login
test let match if then else for in requires
```

The declaration-head words (`module import type page msg form table
capability session login test`) are reserved only at the start of a line
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
| `Dict k v` | built with `Dict.fromList`; `k` may be any type that can be compared with `==` (no functions or effect values in it, including inside a union's payloads), and two keys are the same key when they are `==` (so `Html` keys are compared by their markup) |
| `Date`, `Time` | calendar date; instant (chapter 9) |

There is no `Char` and no `String`: text is `Text`. There is no tuple: use
a record.

## Int

`Int` is a 64-bit two's-complement integer. Arithmetic that goes past the
largest or smallest `Int` wraps around: `9223372036854775807 + 1` is
`-9223372036854775808`. `/` truncates toward zero (`-7 / 2` is `-3`), and
`%` has the sign of the dividend (`-7 % 2` is `-1`). Dividing an `Int` by
zero, with `/` or `%`, is a runtime failure: the program stops where it
happens (later milestones turn it into a failed task). This holds for
constants too; `1 / 0` compiles and fails when it runs.

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
record type that happens to have the same fields. An anonymous record
literal `{ x: 1.0, y: 2.0 }` is given the type the context expects. A
signature may spell out a record type inline when it is used only once:

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
type Page a = { items: List a, total: Int }
```

## What there is not

- No type aliases. A new name is always a new type.
- No typeclasses, interfaces, or traits.
- No tuples.
- No record that contains itself by value (`type R = { r: R }`, directly
  or through other records, is E0011): a record may refer to itself only
  through `Maybe`, `List`, `Dict` or another union.
- No subtyping other than brand-to-base, and a `requires` page's `Session`
  to the app's (chapter 6).

# 5. HTML

HTML is a built-in expression form. An HTML expression has type `Html Msg`,
where `Msg` is the message type of the page it can send. HTML that sends
nothing has type `Html Never`.

## Elements

```alder eval=<ul class="todos"><li>First</li><li>Second</li></ul>
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

Text is **always escaped**. To insert trusted markup, use `Html.unsafe`,
which takes a `Text` and returns `Html Never`. It is the only way to produce
unescaped output, and it must never be applied to text a user entered.

## Control flow in HTML

```alder
<div>
  {if model.todos == [] then <p>Nothing to do.</p> else <ul>{items}</ul>}

  {for todo in model.todos -> <TodoItem todo={todo} />}

  {match model.user {
    Just u -> <span>{u.name}</span>,
    Nothing -> <a href={Auth.Login {}}>Log in</a>,
  }}
</div>
```

`for x in list -> html` produces a `List (Html Msg)`. There is no index
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
<a href={Todos.Show { id: todo.id }}>{todo.title}</a>
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

A `<button click={…}>` works with and without JavaScript. Outside a form it
is rendered inside a small form of its own that posts the message:

```html
<form method="post" action="/_m/App/Logout" class="_alder-click" data-on:submit="@post(&#39;/_m/App/Logout&#39;, {contentType: &#39;form&#39;})"><button type="submit">Log out</button></form>
```

A form is a block element, so give the wrapper
`form._alder-click { display: inline }` in your stylesheet to keep such
buttons in line with the text around them. Inside a `<form submit>` (forms
cannot nest) a `click` button is a submit button with its own
`formaction` instead. A `click` on any other element, such as a `<li>`,
needs JavaScript. Chapter 6, "With and without JavaScript", has the
details.

`data-*` and `aria-*` attributes take `Text`.

## Components

A component is a function from a record to `Html`, named with an
uppercase letter:

```alder
TodoItem : { todo: Todo } -> Html a
TodoItem props =
  <li class={if props.todo.done then "done" else ""}>
    <span>{props.todo.title}</span>
  </li>
```

It is used as an element, with props as attributes:

```alder
<TodoItem todo={todo} />
```

Children are passed through a `children` field of type `List (Html a)`:

```alder
Layout : { title: Text, children: List (Html a) } -> Html a
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

A component in a page module may be typed `Html Msg` and send that page's
messages. A component in any other module has no `Msg`; it is typed `Html a`
and can send only app-level messages (`App.Logout`), which fit any `Html a`.
`Html Never` (markup that sends nothing) also fits any `Html a`.

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
shows form-level errors (from `Reject`), then the errors of every field
that has no `<Errors field>` of its own, each after the field's name. Each
renders a list, empty until there is something to show, with one item per
message:

```html
<ul class="errors" data-errors="title"></ul>
<ul class="errors" data-errors="title"><li>must not be empty</li></ul>
<ul class="errors" data-errors="form"><li>Email or password is incorrect.</li></ul>
```

The first is the page as `GET` renders it; the other two are the same
pages after a failed submission (chapter 6, "Form errors"). `data-errors`
names the field, or `form`, so a stylesheet can hide an empty list with
`ul.errors:empty`.

Which component to use depends on the field's type:

| Field type | Component | Renders |
|---|---|---|
| `Text`, brands of `Text` | `<Input>` | `<input type="text">` |
| `Text.Email` | `<Input>` | `<input type="email">` |
| `Text.Url` | `<Input>` | `<input type="url">` |
| `Text.Secret` | `<Input>` | `<input type="password">` |
| `Int` | `<Input>` | `<input type="number">` |
| `Float` | `<Input>` | `<input type="number" step="any">` (without `step` a browser takes only whole numbers) |
| `Date` | `<Input>` | `<input type="date">` |
| `Text` meant to be long | `<Textarea>` | `<textarea>` |
| `Bool` | `<Input>` | checkbox |
| union with no payloads | `<Select>` | `<select>` with one option per constructor, labeled by its name |
| `Maybe` of any of these | same as the inner type | not `required` |

The browser's `required` is set exactly when the decoder refuses a blank
value, so the browser never blocks what the server would accept: not for
a `Maybe`, a field with a default (blank is the default), a checkbox, plain
`Text` (where `""` is a value) or a brand of `Text` without the stdlib's
check; it is set for `Text.NonEmpty`, `Text.Email`, `Text.Url`,
`Text.Secret`, `Int`, `Float`, `Date` and unions. The decoder still checks
everything: the browser's checks are a convenience.

`value={...}` on any of them takes the field's own type (`value={model.todo.priority}`
for a `Select` over `Priority`), or `Maybe` of it.

A `<form submit={M}>` where `M` has no `form` type is allowed; it contains
no bound inputs, only a button.

`field` must name a field of the form type. The input's `name`, `type`,
`required`, and current value are derived from the field's type and the
submitted values. `value={...}` (the field's own type, or `Maybe` of it) sets the initial
value for an edit form; a submitted value takes precedence over it. A field
of type `Text.Secret` renders as a password input and is never echoed back:
after a failed login, the email comes back filled in and the password
empty.

```html
<label>Password<input name="password" type="password" required /></label>
```

Each `<Input>` is wrapped in a `<label>` holding its `label` text.
A plain `<input>` inside a `<form submit={...}>` is a compile error; use
`<Input>`.

## Partial updates

After a message, the runtime renders the whole page again and sends its
`<body>` to the browser, where Datastar morphs it into the page that is
there: elements that did not change are left alone. Without Datastar
(JavaScript off), the browser gets the whole new page. Nothing in your
code changes between the two (chapter 6, "With and without JavaScript").

`id` attributes are not required for this. They help the morph match an
old element with its new self when a list changes, so give the regions
that change (`<ul id="todos">`) a stable `id`.

# 6. Pages, messages, forms

## A page module

A page is a module that declares a route and the five things that go with
it. This is the whole shape of an Alder app.

```alder
module Todos.Index

import Todo

page "/" { filter: Todo.Filter = All } requires user

type Model = { filter: Todo.Filter, todos: List Todo }

load : Params -> Session -> Query Model
load params session = {
  let todos <- Db.from Todo |> Db.where (Todo.owner.eq session.user) |> Db.all
  Query.succeed { filter: params.filter, todos: todos }
}

view : Model -> Session -> Html Msg
view model session =
  <ul>
    {for todo in model.todos ->
      <li><button click={Toggle { id: todo.id }}>{todo.title}</button></li>}
  </ul>

msg Toggle { id: Todo.TodoId }

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Toggle p -> {
    let todo <- owned p.id session
    let _ <- Db.update Todo todo.id (\t -> { t | done: not t.done })
    Task.succeed Rerender
  },
}

-- The id in a message comes from the request: check that it is the user's ("Authorize in
-- `update` too", below).
owned : Todo.TodoId -> Session -> Task Todo
owned id session = {
  let found <- Db.get Todo id
  match found {
    Just todo -> if todo.owner == session.user then Task.succeed todo else Task.notFound,
    Nothing -> Task.notFound,
  }
}
```

The compiler checks that each page module has exactly: one `page`, one
`Model`, one `load`, one `view`, and, if it has any `msg`, one `update`.
`Params` and `Msg` are generated from the declarations and must not be
written by hand. `view` is always `Model -> Session -> Html Msg`, even on
a page with no `msg` (its `Msg` is then empty; `Html Never` is not used for
pages). `view` receives the session so that per-user presentation (theme,
"logged in as") needs no Model field.

## `page`

```alder fragment
page "/todos/{id}" { id: Todo.TodoId }
page "/" { filter: Todo.Filter = All, q: Maybe Text }
page "/admin" { } requires user
```

- The path is a literal with `{name}` segments. Each segment must appear in
  the parameter record; parameters not in the path are query parameters.
- Parameter types must be one of: `Int`, `Text`, `Bool`, a brand of those,
  a union with no payloads, `Maybe` of those, or `List` of those.
- A parameter with a default is optional in links and in the URL.
- A `List` parameter repeats its key in the URL: `{ tag: List Text }` with
  `["a", "b"]` is `?tag=a&tag=b`, and `[]` is no key at all.
- `Params` is the parameter record type for this module.
- Two pages may not answer the same path: `/x/{id}` and `/x/{slug}` are the
  same pattern, so the second is an error (E0016). Patterns that merely
  overlap are fine, whatever the module order: a literal segment is matched
  before a parameter, left to right, so `/todos/new` is served before
  `/todos/{id}`.
- `requires` names a field of `Session` whose type is `Maybe a`. The page is
  served only when that field is `Just`; otherwise the runtime redirects to
  the page named in `App.login`. Inside this module `session.<field>` has type
  `a`, not `Maybe a`. The page's `Session` is then a type of its own, and it
  can be passed wherever the app's `Session` is expected (to a function in a
  shared module, say), where the field is `Just` its value again. The other
  direction is a type error (E0003): a function of the page that takes its
  `Session` cannot be given the app's, whose field may be `Nothing`.

A page value is the module name applied to its parameter record. It is the
only thing `href` accepts:

```alder
<a href={Todos.Show { id: todo.id }}>{todo.title}</a>
<a href={Todos.Index { filter: Active }}>Active</a>   -- defaulted fields may be omitted
```

## `msg`

A message belongs to the page that declares it. It has no path: the
compiler generates the POST URL from the page and message name, so two
messages can never collide.

```alder fragment
msg Toggle { id: Todo.TodoId }
msg Submit form NewTodo
msg Comment { post: PostId } form NewComment
```

- Parameter types follow the same rules as `page` parameters. A message
  with no parameters is declared bare: `msg Logout`.
- `form F` means the request body is decoded as form type `F`.
- `Msg` is the union of the module's messages. Each constructor takes its
  parameter record. If the message has a form, the record has an extra
  field `form: F` that the runtime fills in.

A message value in HTML is the constructor applied to its parameters,
without `form`:

```alder
<button click={Toggle { id: todo.id }}>Done</button>
<form submit={Comment { post: model.post.id }}>…</form>
<form submit={Submit}>…</form>                 -- no parameters: bare constructor
```

In `update`, the same constructor carries the full record:

```alder fragment
update msg model session = match msg {
  Toggle p -> ...,                 -- p.id : Todo.TodoId
  Submit p -> ...,                 -- p.form : NewTodo
  Comment p -> ...,                -- p.post : PostId, p.form : NewComment
}
```

The page's own parameters travel with every message it sends, so `update`
always has the same `Model` that `view` had. They ride in the message's URL
under keys that start with `_p.` ("How a request runs", below).

A page can only send its own messages and the app-level messages below. It
cannot send another page's message.

## App-level messages

Actions that belong to no page (log out, switch theme) are declared as `msg`
in the `App` module. Any page can send them, and `App.update` handles them.
It has no `Model`, and no page of its own to render, so its `Nav` is
answered differently: `Redirect p` is a redirect to `p`, and `Rerender` and
`Reject` are a redirect back to the page that sent the message (its
address, from the browser's `Referer` header, or `/` without one). The page
is then loaded again with the new session. A `Reject` text from
`App.update` is not shown anywhere, and neither are the errors of a form on
an App message that fails to decode: both answer with the same redirect
back, and the errors are dropped. Give a form that needs its errors shown a
page message. ("How a request runs", below, shows the code.)

```alder fragment
-- in App
msg Logout

update : Msg -> Session -> Task Nav
update msg session = match msg {
  Logout p -> {
    let _ <- Session.put (\s -> { s | user: Nothing })
    Task.succeed (Redirect (Auth.Login {}))
  },
}

-- in any page, or any component
<button click={App.Logout}>Log out</button>
```

An app-level message value is accepted in `Html a` for every `a`, so a
layout component typed `Html a` can contain `click={App.Logout}`.

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

When a submitted form fails to decode (missing field, failed refinement,
wrong type), the runtime does **not** call `update`. It re-renders the page
with the submitted values and errors attached to that form; `<Input>` and
`<Errors>` show them. The program never sees an invalid form. Only the form
that posted the message shows them: on a page with one form per row, the
other rows keep their own values. A field's errors go to its
`<Errors field="…" />`; the errors of a field without one are listed in the
form's `<Errors form />`, after its name ("due must be a date
(YYYY-MM-DD)"). A form with neither for a field gets warning W0003.

## `update` and `Nav`

```alder fragment
update : Msg -> Model -> Session -> Task Nav
```

`update` returns a `Task` (chapter 7) that ends in a `Nav`:

```alder
-- Built in; shown for reference.
type Nav
  = Rerender                 -- re-run load and view for this page, patch the result in
  | Redirect Page            -- go to another page (HTTP 303)
  | Reject Text              -- re-render with this error shown on the form that sent the msg
```

Use `Rerender` for in-place changes (toggle, delete). Use `Redirect` after
creating something, so a browser refresh doesn't resubmit. Use `Reject` when
a well-formed submission is refused for a reason only `update` can know
(wrong password, name already taken); `<Errors form />` shows the text.

`Reject` is a successful result, not a failure: the `Task` that produced it
commits, so anything it wrote and any `Session.put` it ran are kept. To
refuse without changing anything, decide before writing (`let taken <-` a
query, then `Reject` or the writes). Only a failed `Task` (`Task.fail`,
`Task.notFound`, a `@unique` violation, a capability error) undoes its
writes and its session changes (chapter 7).

## Authorize in `update` too

`load`'s checks cover the page, not the ids a message carries. The to-do
list's `load` reads only the user's own todos, so the page never shows
anyone else's; but `Toggle { id }` puts the id in the message's URL, and
anyone can post `/_m/Todos.Index/Toggle?id=` with an id that is not theirs.
`update` must check it again. The example does it with a helper that ends
in `Task.notFound` (a 404) when the row is missing or someone else's:

```alder fragment
owned : Todo.TodoId -> Session -> Task Todo
owned id session = {
  let found <- Db.get Todo id
  match found {
    Just todo -> if todo.owner == session.user then Task.succeed todo else Task.notFound,
    Nothing -> Task.notFound,
  }
}

update msg model session = match msg {
  Toggle p -> {
    let todo <- owned p.id session
    let _ <- Db.update Todo todo.id (\t -> { t | done: not t.done })
    Task.succeed Rerender
  },
  ...
}
```

When the page's own parameter names the thing, let the message carry no
id at all and act on what `load` checked. `Todos.Show` declares
`msg Save form EditTodo` and `msg Delete`, and its `update` writes
`model.todo.id`: the page's `{id}` was authorized by `load`, and a message
cannot name another row.

## The request lifecycle

```
GET  /path
  1. decode route parameters      → 404 on failure
  2. check `requires`             → redirect to App.login on failure
  3. load                         → Query, run by the runtime
  4. view (with the session)      → HTML

POST (generated message URL)
  1. decode the page's parameters (`_p.` keys) → 404 on failure
  2. decode the message's parameters           → 404 on failure
  3. decode the form, if any → on failure: requires, load, view with the
     errors, and stop (update is not called)
  4. check `requires`             → redirect to App.login on failure
  5. load                         → Model
  6. update                       → Task, run by the runtime
  7. Rerender: load → view → patch (without JavaScript: 303 to the page)
     Redirect: 303 (under Datastar: an event that navigates)
     Reject: load → view with the text on the form → patch
     Task failure: App.error → HTML, status 500 (404 for Query.notFound
     and Task.notFound)
```

## The `App` module

Every app has an `App` module:

```alder
module App

import User
import Layout
import Theme

session { user: Maybe User.UserId, theme: Theme = Light }

login = Auth.Login

msg Logout
msg SwitchTheme

update : Msg -> Session -> Task Nav
update msg session = match msg {
  Logout p -> {
    let _ <- Session.put (\s -> { s | user: Nothing })
    Task.succeed (Redirect (Auth.Login {}))
  },
  SwitchTheme p -> {
    let _ <- Session.put (\s -> { s | theme: Theme.flip s.theme })
    Task.succeed Rerender
  },
}

error : Error -> Html Never
error err =
  <Layout.Page title="Error" theme={Light}>
    <h1>Something went wrong</h1>
    <p>Please try again.</p>
  </Layout.Page>

notFound : Html Never
notFound =
  <Layout.Page title="Not found" theme={Light}>
    <h1>Not found</h1>
  </Layout.Page>
```

- `session` declares the session record. The runtime stores it in a signed
  cookie. Field types follow the same rules as form fields. `Session.put`
  (chapter 7) changes it; a `Task` that fails leaves it as it was.
- `login` names the page to redirect to when `requires` fails.
- `error` and `notFound` render failure pages.
- `msg` and `update` declare app-level messages (above). They are optional.

## How a request runs

You can write a whole app without reading this section. It explains what
the server that `alder build` makes (chapter 11) does with each request, so
that nothing in the lifecycle above is a mystery. Chapter 13 follows one
request through it byte by byte.

Four pieces of Go do the work. `rt/web` is the router and the session
cookie. `build/<app>/routes__alder.go`, which the compiler writes for your
app, holds one handler for each page and one for each message. `rt/html`
renders the page, and `rt/datastar` writes the answer a Datastar request
gets.

### The router

`web.NewRouter` in `rt/web/router.go` is the app's HTTP handler. For each
request it does these things, in order:

1. `GET /_alder/datastar.js` is answered with the Datastar script, which
   is built into the binary (`rt/datastar/datastar.js`, version in
   `rt/datastar/VERSION`).
2. `GET /static/<file>` is answered with that file from your app's
   `static/` directory. Directories are never listed, and a path with `..`
   in it is refused.
3. The method and path are matched against the generated list `Routes`. A
   page is a `GET` of its pattern; a message is a `POST` to
   `/_m/<Module>/<Msg>`, such as `/_m/Todos.Index/Toggle`. The most specific
   pattern is tried first: segment by segment from the left, a literal
   before a parameter. Anything else is a 404 rendered with `App.notFound`.
4. For a `POST`, the body is read: a url-encoded form is parsed (at most
   1 MiB, else status 413), a multipart body is refused (415), and any
   other body is an empty form. Datastar sends a button's message with a
   JSON body, which Alder ignores.
5. The session cookie is decoded (below).
6. The route's handler runs, and its answer is written.

A Go panic anywhere in the request becomes a 500 page rendered with
`App.error`. The router also puts the Datastar script tag,
`<script type="module" src="/_alder/datastar.js"></script>`, right after
the `<head>` of every page it writes (on a page without `<head>`, right
after `<html>`; without that, before `</body>`), so your layout stays plain
Alder.

### One handler per message

This is the handler for `Toggle`, from `build/todos/routes__alder.go`:

```go
func Route__Todos__Index__Toggle() web.Route {
	return web.Route{Method: "POST", Pattern: "/_m/Todos.Index/Toggle", Handle: func(rq *web.Request) web.Response {
		params, errs := decode__Todos__Index__Params(rt.PageParams(rq.Query))
		if errs != nil {
			return web.NotFoundResponse(html.None())
		}
		p, errs := decode__Todos__Index__ToggleParams(rq.Query)
		if errs != nil {
			return web.NotFoundResponse(html.None())
		}
		return update__Todos__Index(rq, params, Todos__Index__Msg__Toggle__(p), nil)
	}}
}
```

It decodes the page's parameters, then the message's, and answers 404 if
either fails. (`rt.PageParams` picks the page's parameters out of the URL,
as the next part explains; `html.None()` is an empty document, which the
router replaces with `App.notFound`.) Then `update__Todos__Index` does the rest of the lifecycle:
`requires`, `load`, `update`, and the answer to the `Nav`. Every page gets
one `update__<Page>` and one handler per message. A page's `GET` has its
own handler, `Route__<Page>__page`, which decodes the path and query,
checks `requires`, runs `load` and renders `view`.

### Page parameters travel in the message URL

A message must know which page sent it. `Toggle` on `/?filter=Active` has
to load the Active list again, not the full list. So the renderer adds the
page's parameters to every message URL it writes. This is the Done button
of `GET /?filter=Active`, as the running app sends it:

```html
<form method="post" action="/_m/Todos.Index/Toggle?id=Em2Gv4v9BaA0Wf9ujESN&amp;_p.filter=Active" class="_alder-click" data-on:submit="@post(&#39;/_m/Todos.Index/Toggle?id=Em2Gv4v9BaA0Wf9ujESN&amp;_p.filter=Active&#39;, {contentType: &#39;form&#39;})"><button type="submit">Done</button></form>
```

(A `click` button is wrapped in a form of its own so that it works without
JavaScript too; "With and without JavaScript", below, explains it.)

`id` is the message's own parameter. `_p.filter` is the page's parameter
`filter`, with `_p.` in front. Your names never contain a `.`, so a page
parameter can never be mistaken for a message parameter of the same name.
A parameter at its default is left out, as in links: on `/`, the same
button posts to `/_m/Todos.Index/Toggle?id=Em2Gv4v9BaA0Wf9ujESN`, and the
handler decodes `filter` as `All`.

A `List` parameter repeats its key, as in links (`rt.QueryKVs` in
`rt/core.go`): a page parameter `tag` holding `["a", "b"]` is
`_p.tag=a&_p.tag=b` in a message URL, and a message parameter is
`tag=a&tag=b`. The handler collects every value of the key.

The handler reads the page's parameters only from the `_p.` keys
(`rt.PageParams` in `rt/core.go`). It does not look at the `Referer`
header. A message URL without the keys a page needs, such as a `Save` for
`/todos/{id}` without `_p.id`, gets a 404.

The URLs of App messages carry the `_p.` keys too
(`/_m/App/SwitchTheme?_p.filter=Active`), but `App.update` has no page, so
its handler ignores them.

### The session cookie

The session lives in one cookie, `alder_session`. Its value is two parts
joined by a dot: a small JSON document, then an HMAC-SHA256 signature of
it, both in unpadded base64url (`Sign` in `rt/web/session.go`). After
logging in to the to-do app, the first part decodes to

```json
{"exp":1794165032,"session":{"Theme":"Light","User":{"Ok":true,"Val":"oV5Et5NZUPszOeLnOSSB"}}}
```

`session` is the session. `exp` is when the cookie stops counting, in
seconds since 1970: 30 days after the session was last stored. The server
checks it itself, so a copied cookie is no good after that, whatever the
browser keeps, and a cookie without `exp` is no session at all.

A union without payloads, such as `Theme`, is stored by its constructor's
name, so reordering the constructors doesn't change what an old cookie
means. A field the cookie lacks, or whose value no longer decodes, gets its
default.

The cookie is **signed, not encrypted**. Anyone holding it can read it, so
never put a `Text.Secret`, or anything the user must not see, in the
session. What the signature does is stop anyone from changing it. A cookie
whose signature doesn't match is ignored, and the request sees the default
session: every field at its default, so `user` is `Nothing`.

The signing key comes from the environment variable `ALDER_SECRET`, which
must be at least 16 bytes. Without it (unset, **or shorter than 16
bytes**) the server refuses to start:

```
alder: ALDER_SECRET must be set to at least 16 bytes outside dev mode (ALDER_DEV=1)
```

and exits with status 1. Only in development mode (`ALDER_DEV=1`, chapter
11) does it use a fixed development key instead, which is public (it is in
`rt/web/session.go`), and log this once at startup:

```
warn: ALDER_SECRET is not set (or shorter than 16 bytes); dev mode signs sessions with the public development cookie key
```

Anyone can forge a session signed with that key, which is why production
needs its own.

The cookie is `HttpOnly` (page scripts cannot read it), `SameSite=Lax`,
`Path=/`, and lasts 30 days from the last time it was stored (its
`Max-Age`, and the `exp` signed into it). It is
`Secure` when the request came over HTTPS, or carries the header
`X-Forwarded-Proto: https` (set by a proxy in front of the server). It is stored:

- on every page a `GET` renders;
- on every answer to a message whose `update` succeeded, whether a page, a
  303 or a navigation event;
- on a 404 or 500 only when an `update` succeeded and the page then failed
  to load again: what the `update` committed is kept.

It is not stored when a form fails to decode or a `Task` fails, since
neither changed the session.

### Form errors

When a form fails to decode, the handler does not call `update`. It runs
`requires` and `load`, and renders `view` with the form's state: what was
submitted, and a list of messages per field. The status is 200. Posting
`title=` to the to-do page renders

```html
<ul class="errors" data-errors="title"><li>must not be empty</li></ul>
```

where `<Errors field="title" />` stands. The values and errors go only to
the form that posted the message: the form state names the message, with
its own parameters (`rt.MsgKey`), and the renderer matches it against each
`<form>`'s message, so a per-row form of another row keeps its initial
values. The value of a `Text.Secret` field is never sent back: its input is
a password input, and it comes back empty.

`Reject t` renders the same way, with `t` in `<Errors form />` of the form
that sent the message. Unlike a form error, the `update` that rejected ran
and committed. A message without a form that ends in `Reject` is answered
like `Rerender`, and the text is not shown.

### With and without JavaScript

The browser either runs Datastar (the usual case) or does not. The answers
differ.

With Datastar, a message is sent with the header `Datastar-Request: true`.
For `Rerender`, `Reject`, a form error, a 404 or a 500, the answer is
status 200 with one server-sent event that replaces the page's `<body>`
(`datastar.PatchBody` in `rt/datastar/datastar.go`). Datastar morphs the
new body into the old one, so only what changed is touched. A 404 or 500
is sent that way too, because Datastar ignores answers that are not 2xx.

A `Redirect` (and a `requires` that sends the user to the login page) is
not a 303 under Datastar. Datastar posts with `fetch`, which would follow a
303 by itself: the new page would be morphed into the old one while the
address bar and the history stayed at the old URL. So the answer is a
200 event that appends a script to the `<body>`; the script removes itself
and tells the browser to go to the new URL (`datastar.Navigate`, what the
official Datastar SDKs do). Logging in from `/login` answers

```
event: datastar-patch-elements
data: selector body
data: mode append
data: elements <script data-effect="el.remove()">window.location.assign("/")</script>

```

The URL is a JSON string, so `<`, `>` and `&` in it are escaped and it can
never end the script. Without Datastar, `Redirect` is a plain 303. Either
way the target must be a path of this site or an `http:`/`https:` URL:
`Url.external` turns anything else (a `javascript:` URL from user input,
say) into `#` (chapter 9), and the router answers a redirect to such a URL
with a 500 rather than send it.

Every line of an event's elements is a `data: elements` line, and an event
ends at a blank line. A carriage return in your data could end a line
early and start a field of its own, so the renderer writes it as `&#13;`
and `PatchBody` turns any line end into a plain newline first.

The two kinds of message travel differently under Datastar. A form posts
with `data-on:submit="@post('<url>', {contentType: 'form'})"`, so its
fields arrive url-encoded and are decoded as the form. A `click` button's
message carries its parameters in the URL, and its body is ignored.

A `click` button works without JavaScript too. Outside a form, it is
rendered inside a form of its own, as the Done button above shows: without
Datastar the browser posts that form to the message URL; with it, Datastar
posts it on submit. The wrapper has the class `_alder-click`; give it
`form._alder-click { display: inline }` in your stylesheet to keep the
button in line (the to-do app's `static/app.css` does). Inside a
`<form submit>`, where forms cannot nest, a `click` button is a submit
button with its own target instead:

```html
<button type="submit" formaction="/_m/App/Reset" formmethod="post" formnovalidate data-on:click__prevent="@post(&#39;/_m/App/Reset&#39;)">Reset</button>
```

Datastar posts the message on click and stops the form's own submission.
Without JavaScript the browser posts the enclosing form's fields to the
button's URL; a message without a form ignores them. (`formnovalidate`
keeps an empty `required` field of the form from blocking the button,
and so does leaving the fields out of the Datastar post: the browser would
otherwise refuse to send a form with an empty required field.) A `click` on another element, such as a `<li>`, needs
JavaScript.

Without JavaScript, the answer to a message is a whole page for a form
error or a `Reject` (they must show what was submitted), and a 303 for
everything else: `Redirect` goes to its page, and `Rerender` goes back to
the page that sent the message, rebuilt from its parameters (`rt.PageAt`
in `rt/core.go`), so the address bar never shows a message URL and a
refresh never posts again (Post/Redirect/Get). Toggling Milk on
`/?filter=Active` without JavaScript answers

```
HTTP/1.1 303 See Other
Location: /?filter=Active
```

and the browser loads the list again.

### App messages

The handler for an App message runs `App.update` with the session and
answers with a redirect (`update__App` in `build/todos/routes__alder.go`),
which the router sends as a 303, or under Datastar as the navigation event
above:

```go
	if nav.Tag == rt.NavRedirect {
		return web.RedirectResponse(rt.PageURL(nav.Page), rq.Env.Session)
	}
	return web.RedirectResponse(rt.BackURL(rq.HTTP.Referer()), rq.Env.Session)
```

`rt.BackURL` keeps only the path and query of the `Referer`, so the 303
never leads to another site. Clicking "Dark mode" on `/?filter=Active`
answers a cookie whose `Theme` is `"Dark"` and, under Datastar, an event
that runs `window.location.assign("/?filter=Active")` (without it,
`303 /?filter=Active`).

### Failure pages

A path no route matches, a page parameter that doesn't decode,
`Query.notFound` and `Task.notFound` are answered with `App.notFound`,
status 404. A failed
`Task` is answered with `App.error`, status 500, and logged as an `error`
line naming the request. With `ALDER_DEV=1` (chapter 11), the error's text
is added at the end of the page's `<body>` as an HTML comment, such as
`<!-- error: Db.update Todo: no row with id "nope" -->`. Without it, the
page never shows the text.

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

```alder fragment
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

```alder fragment
update msg model session = match msg {
  Submit p -> {
    let id <- Db.insert Todo { title: p.form.title, due: p.form.due, owner: session.user }
    Task.succeed (Redirect (Todos.Show { id: id }))
  },
  Toggle p -> {
    let todo <- owned p.id session  -- the user's todo, or a 404 (chapter 6)
    let _ <- Db.update Todo todo.id (\t -> { t | done: not t.done })
    Task.succeed Rerender
  },
}
```

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

Task.succeed  : a -> Task a
Task.fail     : Text -> Task a
Task.notFound : Task a              -- fail: respond 404 with App.notFound
Task.map      : (a -> b) -> Task a -> Task b
Task.all      : List (Task a) -> Task (List a)
```

There is no `andThen`; use `let x <-` in a block.

## Failure

A `Task` can fail. `Task.fail "message"` fails explicitly; a capability may
fail on its own (database down, email rejected). A failed task stops
`update`, undoes everything it did (its database writes and its
`Session.put` changes), and the runtime renders `App.error`.
`Task.notFound` fails the same way but answers 404 with `App.notFound`:
use it when a message names something that is not there, or not the
user's (chapter 6, "Authorize in `update` too"). A task that
ends in `Reject` has not failed: its writes and session changes are kept
(chapter 6). There is no `try`/`catch`:
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
or start until every capability is implemented and wired in ("Implementing
a capability", below).

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
Http.get    : { url: Text } -> Task (Result Http.HttpError Text)
Http.post   : { url: Text, body: Text } -> Task (Result Http.HttpError Text)
```

`Http.HttpError` is the stdlib's type for a failed request: a type of a
standard module other than the Prelude is always written qualified (a bare
`HttpError` is E0202, with the fix).

Nothing else in the language touches the outside world. If a function's
signature has no `Query` or `Task` in it, it is pure.

## How effects run

This section describes the runtime in `rt/eff`, the Go package the
generated code imports. You don't need it to write Alder, but it explains
when things happen and what a failure undoes. Chapter 13 follows one
`load` through it step by step.

**An effect is a tree.** A `Query` or `Task` value is data: a small tree
of steps, built by the generated code and never inspected by it. There are
five kinds of node (`rt/eff/eff.go`):

```go
type pure struct{ v any }

type bind struct {
	e node
	f func(any) node
}

type call struct {
	name  string
	write bool
	fn    func(*Env) (any, error)
}

type fail struct{ msg string }

type notFound struct{}
```

`pure` is `Task.succeed`/`Query.succeed`. `bind` is one `let x <- e`: run
`e`, then hand its result to `f`, a Go function that builds the rest of
the block. `call` is one step that touches the outside world: a `Db.*`
operation, a capability, `Time.now`, `Log.info`. It carries a name for the
log, whether it writes, and the Go function that does the work. `fail` is
`Task.fail` (and a `@unique` violation, chapter 8); `notFound` is
`Query.notFound` or `Task.notFound`. Building a tree does nothing. Only `eff.Run` does.

**Run is eager.** `eff.Run` (`rt/eff/run.go`) walks the tree node by node:
at a `call` it runs the step right away, at a `bind` it remembers the
**continuation** (the function that builds the rest of the block once it
is given the bound value), and when a value comes out it passes the value
on. The
`Toggle` arm of the to-do page,

```alder fragment
Toggle p -> {
  let todo <- owned p.id session
  let _ <- Db.update Todo todo.id (\t -> { t | done: not t.done })
  Task.succeed Rerender
},
```

is a `bind` whose first part is `owned p.id session` (itself a `bind`: the
`call` named `Db.get Todo`, then a `match` that ends in `pure todo` or in
`notFound`) and whose continuation is a second `bind`: the `call` named
`Db.update Todo`, then `pure Rerender`. `Run` performs the read, checks
the owner, performs the update, then returns `Rerender`; when `owned` ends
in `notFound`, it stops there and nothing is written. Nothing is batched,
reordered or run in parallel: steps run in the order the block lists
them, one at a time.

**One transaction per `Task`, begun when needed.** `Run` does not begin
a transaction up front. The first step of a `Task` that touches the
database begins one (`db.TxOf`, through `Env.Transaction`), and every
later step uses the same one, so reads inside a `Task` see the task's own
earlier writes. A `Task` that never touches the database (it only calls
`Session.put`, a capability or `Http.get`) begins none, and a slow
capability or HTTP call made before the first database step holds no
database lock: other requests' tasks run meanwhile. If the task produces
its result, `Run` commits the transaction, if one was begun. If it fails
in any way (`Task.fail`, `Db.update` on a row that doesn't exist,
`Query.notFound`, `Task.notFound`, an error from the database or a capability, or a Go
panic), `Run` rolls the transaction back instead. `Db.get` finding
nothing is not a failure: it produces `Nothing`. Either every write of an
`update` happens or none does.

**The session is part of the same promise.** `Run` remembers the session
a `Task` starts with. If the task fails, `Run` puts that session back, so
a `Session.put` before a `Task.fail` is undone like a write. A task that
succeeds keeps its session changes, and so does one producing `Reject`
(chapter 6): it is a successful result.

**A `Query` has no transaction and cannot write.** `Run` runs a `Query`
directly against the database. A step marked as a write (`Db.insert`,
`Db.update`, `Db.delete`, `Session.put`, a capability entry that returns a
`Task`) is refused with an internal error if a `Query` ever reaches it.
The checker already makes that impossible (E0401); the runtime checks
again so that a compiler bug can't turn a page view into a write.

**Every step is traced.** Before it runs a `call`, `Run` appends the
step's name to `Env.Trace` and logs it at level `effect`. The `Toggle`
above leaves the trace `["Db.get Todo", "Db.update Todo"]`: `owned`'s
read, then the update. The `Add` arm of
`testdata/golden/gen_effects/app/Index.alder`, which inserts a row and
then logs, leaves `["Db.insert Todo" "Log.info"]`. The trace is the dev
log of what a request did, and what a test can compare (chapter 10).

**What a capability looks like in Go.** The to-do app checks
passwords outside the language
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

**A capability can write in the task's transaction.** While `Run` runs,
the `ctx` it passes to a capability carries the running effect
(`eff.EnvFrom(ctx)`), so a Go implementation reaches the task's store and
transaction with `db.StoreFrom(ctx)` and `db.TxFrom(ctx)`; the
transaction is begun on first use, as for a `Db.*` step. What it writes
that way commits or rolls back with the rest of the task. This
implementation of `Pay.charge` (from `TestGen_RunEffectsCorpus` in
`internal/gen/gen_test.go`, over `testdata/golden/gen_vet/m3b-effects`)
inserts a `Customer` row in the running task's transaction:

```go
func (pay) Charge(ctx context.Context, args Args__Shop__Pay__Charge) (rt.Maybe[string], error) {
	row := insertvals__Shop__Customer(insert__Shop__Customer{Email: args.Card + "@pay", Seen: rt.Nothing[time.Time]()})
	_, err := db.InsertRow(ctx, db.StoreFrom(ctx), db.TxFrom(ctx), table__Shop__Customer, row, time.Now())
	return rt.Just(args.Card), err
}
```

When the task that called `Pay.charge` then fails, the row is gone.
A capability that writes through the store without `db.TxFrom(ctx)` (with
a `nil` transaction) is outside the task: its writes survive a rollback,
and once the task has begun its transaction the write waits for it,
forever on `:memory:` (one connection) and up to SQLite's five-second busy
timeout on a file. In a `Query` (a capability entry typed `Query`),
`db.TxFrom(ctx)` is `nil` and `db.StoreFrom(ctx)` is read-only: it reads
without a transaction, and its `Insert`, `Update`, `Delete` and `Begin`
return an internal error (`eff.ErrInternal`, "write from a Query") and
write nothing.

Inside the program, an implementation is found by the capability's
module-qualified name, `Env.Caps["Auth.Login.Password"]`. The generated
list `Capabilities` (in `app__alder.go`) names every key an app needs. A
call to a capability with nothing registered is a Go `panic`,
`alder: capability "Auth.Login.Password" is not wired`, which rolls the
task back like any other failure; a server never gets that far, because
it checks every capability when it starts (below). An error the
implementation returns is wrapped with the call's name
(`*eff.ErrCapability`, whose text is `Password.verify: <the error>`) and
fails the task too.

## Implementing a capability

Your Go code goes in the app's `adapters/` directory, next to `app/`. It
is one Go package, `package adapters`, and it must declare one function:

```go
func Caps() app.Caps
```

`alder build` (chapter 11) copies the `.go` files at the top level of `adapters/` into
`build/<app>/adapters/`, inside the generated Go module. There, the
generated package is called `app`, so your code imports it as `"app"`, and
the runtime as `"alder/rt"`, `"alder/rt/db"` and so on. The generated
`cmd/main.go` calls `adapters.Caps()` and serves the app with what it
returns.

`app.Caps` is a struct with one field per capability, named
`<Module>__<Capability>` (`app__alder.go`):

```go
type Caps struct {
	Auth__Login__Password Cap__Auth__Login__Password
}
```

Every name your adapters use from `app` is exported and has a fixed
shape: `Caps`, `Cap__<Module>__<Capability>` (the interface),
`Args__<Module>__<Capability>__<Entry>` (an argument record), and `Tables`
(every table's descriptor, for `db`). The `.` in a module name becomes
`__`. That is why a module may not be named `Cap`, `Args` or `Route`
(E0015, chapter 11): its names would start the same way.

This is the to-do app's adapter, `examples/todos/adapters/password.go`,
without its last function:

```go
package adapters

import (
	"context"
	"errors"
	"time"

	"alder/rt"
	"alder/rt/db"
	"app"
)

// Caps are the implementations of the app's capabilities.
func Caps() app.Caps {
	return app.Caps{Auth__Login__Password: password{}}
}

// The one account the development stub accepts.
const (
	devEmail    = "a@b.c"
	devPassword = "password"
)

// password is a development stub of Auth.Login.Password: it accepts a@b.c with the password
// "password" and nobody else. Replace it before deploying the app.
type password struct{}

// Verify returns the id of the development account's User row for a@b.c and "password", and
// Nothing for anything else.
func (password) Verify(ctx context.Context, args app.Args__Auth__Login__Password__Verify) (rt.Maybe[string], error) {
	if args.Email != devEmail || args.Password != devPassword {
		return rt.Nothing[string](), nil
	}
	id, err := devUser(ctx)
	if err != nil {
		return rt.Nothing[string](), err
	}
	return rt.Just(id), nil
}
```

`Verify` cannot return a made-up id: a todo's `owner` is
`User.UserId @ref User`, so the id must name a real `User` row. The
missing function, `devUser`, looks the row up with `db.StoreFrom(ctx)`
and `db.TxFrom(ctx)` and inserts it the first time, inside the transaction
of the `Task` that called `Password.verify`. If that task fails, the new
row is rolled back with it. Pass `db.TxFrom(ctx)` to every query an
adapter makes: under `alder test` the database is a single in-memory
connection that the running transaction holds, so a query made through
`db.StoreFrom(ctx)` outside that transaction waits for it forever.

When a method returns an error, the step fails with it, named after the
call: `Password.verify: <the error>`. A page answers it like any other
failure, with a 500; inside a test, `Test.run` gives it back as
`Err "Password.verify: <the error>"` (chapter 10). A capability's error is
a failure the test can observe.

The server refuses to start when a field of `Caps` is `nil`. `Serve`, in
`app__alder.go`, checks every capability before it opens the database,
and the binary prints the reason and exits 1:

```
alder: capability Auth.Login.Password is not wired: set Caps.Auth__Login__Password
```

An app with capabilities and no `adapters/` directory does not build at
all; chapter 11 shows the message.

Your editor will not resolve `import "app"` inside `adapters/`: that
package only exists in `build/<app>/`. For the same reason, an app inside
another Go module needs a `go.mod` of its own (chapter 11).

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
  owner: User.UserId @ref User,
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
Todo.owner.in   : List User.UserId -> Condition Todo
Todo.due.isNothing : Condition Todo              -- for Maybe fields; also .isJust
Todo.created.asc, Todo.created.desc : Order Todo   -- unions order by declaration

Db.and : List (Condition t) -> Condition t
Db.or  : List (Condition t) -> Condition t
Db.not : Condition t -> Condition t
```

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

A column must be written in place, `Todo.done.eq False`: a column
method on a variable or record field holding a column (`let c = Todo.done`,
then `c.eq False`) is E0003. And the database functions need to know their
table where they are used: `countAll : Table t -> Query Int` calling
`Db.count (Db.from t)` is E0003 (`` `Db.count` needs a concrete table; `t`
is a type parameter``); write one function per table, `Table Todo`.

**`Maybe` columns.** A condition on a `Maybe` column is true or false for
every row, never unknown. `Nothing` is matched by `.isNothing`,
`.eq Nothing` and an `.in` list that contains `Nothing`
(`Todo.due.in [Just d, Nothing]`), and `.eq (Just d)` and `.ne Nothing`
never match it. `Db.not c` matches exactly the rows `c`
does not, so `Db.not (Todo.due.eq (Just d))` includes the rows with no due
date. Ordering by a `Maybe` column puts `Nothing` first with `.asc` and
last with `.desc`.

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
fails the task. The function may not change the `@id` field: if it
returns a record with a different id, the task fails with
`Db.update Todo: update may not change the id` and nothing is written
(`UpdateRow` in `rt/db/store.go`). Deleting a row that doesn't exist is
not an error.

```alder fragment
let id <- Db.insert Todo { title: p.form.title, due: p.form.due, owner: session.user }
let _ <- Db.update Todo id (\t -> { t | done: True })
let _ <- Db.delete Todo id
```

Writes in one `update` run in a single transaction. If the task fails,
nothing is written. Inserting or updating a row that violates `@unique`
fails the task with a message such as `User.email must be unique`, and
everything the task wrote before is rolled back; check first with a query
and `Reject` if the user should see a message. `Reject` is a successful
result, so writes made before it are kept (chapter 6): decide first, then
write.

`@ref` is checked the same way. Inserting or updating a row whose `@ref`
field names a row that does not exist fails the task with
`Todo.owner: no User with that id`; deleting a row that another row still
refers to fails with `User QXZMRYnkC2vuE_TefkuR is referenced by another
row` (the table and the id). Either way the task's earlier writes are
rolled back.

## How tables are stored

The database is SQLite (`rt/db/sqlite.go`). Each `table` is one SQL
table, named by its module-qualified name with `.` replaced by `__`:
`table Todo` in module `Todo` is `Todo__Todo`. Each field is one column
with the field's name, in declaration order. Values are stored like this
(the package comment of `rt/db/table.go`):

| Alder | SQL column |
|---|---|
| `Text`, a brand of `Text` | `TEXT` |
| `Int` | `INTEGER` |
| `Float` | `REAL` |
| `Bool` | `INTEGER`, `0` or `1` |
| `Date` | `TEXT`, `YYYY-MM-DD` |
| `Time` | `TEXT`, RFC 3339 in UTC with a fixed nine-digit fraction (`2026-10-09T12:00:00.000000000Z`), so text order equals time order |
| a union with no payloads | `TEXT`, the constructor's name (`Active`) |
| `Maybe a` | `a`'s column, `NULL` for `Nothing` |

A `Time` is always written with nine digits after the second, so every
stored time has the same width, and comparing the text compares the
times. `EncodeTime` in `rt/db/encode.go` does it:

```go
// EncodeTime is t as stored: RFC 3339, fixed 9-digit fraction, UTC
// ("2026-10-09T12:00:00.500000000Z"). Every stored time has the same width and the same zone,
// so comparing and ordering the text (Time conditions, Db.orderBy) compares the times.
func EncodeTime(t time.Time) string { return t.UTC().Format(timeLayout) }
```

Every column except a `Maybe` is `NOT NULL`. The `@id` is the primary key,
`@unique` adds `UNIQUE`, `@ref` adds `REFERENCES`, and a `Bool` or union
column gets a `CHECK` listing its allowed values. For the to-do example's
`Todo` table the runtime creates:

```
CREATE TABLE "Todo__Todo" ("id" TEXT NOT NULL PRIMARY KEY, "title" TEXT NOT NULL, "done" INTEGER NOT NULL CHECK ("done" IN (0, 1)), "due" TEXT, "owner" TEXT NOT NULL REFERENCES "User__User"("id"), "created" TEXT NOT NULL) STRICT
```

A few rules follow from the mapping:

- **Ids** are 20 random characters from `A–Z a–z 0–9 - _`, safe in a URL
  (`NewID` in `rt/db/store.go`), such as `QXZMRYnkC2vuE_TefkuR`.
- **`@created` and `@updated`** are set from the runtime's clock when the
  row is written. `Db.update` never rewrites the id or `@created`.
- **Unions order by declaration.** `orderBy` and `.lt`/`.gt` on a union
  column follow the order the constructors are declared in, not
  alphabetical order.
- **`.contains`** ignores case for the letters A–Z only; `É` and `é`
  differ.
- **Empty lists.** `Db.and []` matches every row, `Db.or []` matches no
  row, and `.in []` matches no row.

Every value reaches SQLite as a parameter, never as part of the SQL text,
so no input can change what a query does.

## Schema changes

When an app starts, the runtime creates the tables (`Ensure` in
`rt/db/sqlite.go`): the generated `Serve` calls it before it serves the
first request, and so do tests (`TestGen_RunTodos` in
`internal/gen/gen_test.go`). It keeps two things in a table of its own, `_alder_meta`: a
hash of the `CREATE TABLE` statements, sorted so that the order of your
`table` declarations does not matter, and the storage format version of
the runtime that created the database (`db.Format`, now `1`, raised
whenever the runtime changes how a table is laid out). If both match,
nothing happens. If your tables changed since the database was created, or
it was created by a runtime with another format:

- **In dev mode** every table in the database is dropped, except
  SQLite's own (`sqlite_*`) and `_alder_meta`, and the app's tables are
  created again, empty. That includes tables your Go capability code
  created itself. Your dev data is lost on every schema change; that is
  the price of not writing migrations while the schema is still moving.
- **Otherwise** the app refuses to start and changes none of your tables
  (only `_alder_meta` is created, if it was missing). The error names the
  tables that changed:

```
db: schema mismatch: the database's schema (hash 5de59b25abca) differs from the app's (hash 40d037c4462a); changed tables: Todo.Todo; run in dev mode to recreate the database
```

or, for a database from a runtime with an older format:

```
db: database format v0 is older than this runtime (v1); run in dev mode to recreate the database
```

Migrations, which would keep the data across a change, are planned.

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
```

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
Text.NonEmpty, Text.Email, Text.Url, Text.Secret   -- refinement brands
```

```alder eval=["a", "b", "c"]
Text.split ", " "a, b, c"
```

```alder eval=True
Text.contains "ell" "hello"
```

`Text.length` counts code points (`Text.length "é"` is 1), and
`Text.slice start end` takes the code points from `start` up to, not
including, `end`. A negative index counts from the end (`Text.slice (-3) (-1)
"abcdef"` is `"de"`; a negative argument needs its parentheses), indices past either end are clamped, and a `start` at
or after `end` gives `""`. `Text.matches regex text` is true when the
regex (RE2 syntax) matches anywhere in the text: write `^…$` to match the
whole text. An invalid regex matches nothing. `Text.fromFloat` writes the
shortest decimal that reads back as the same number, never an exponent
(`2.0` is `"2"`, `3.5` is `"3.5"`); infinity is `"+Inf"` or `"-Inf"` and
not-a-number is `"NaN"`. `Text.toInt` and `Text.toFloat` give `Nothing`
for anything else than a plain decimal number (spaces, overflow, `"NaN"`).

```alder eval=["de", "2", "3.5"]
[Text.slice (-3) (-1) "abcdef", Text.fromFloat 2.0, Text.fromFloat 3.5]
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

`Int` arithmetic wraps on overflow; `/` truncates toward zero and `%` has
the dividend's sign; an `Int` division or remainder by zero is a runtime
failure (chapter 4). `Float` division by zero gives infinity or NaN.

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
```

A key may be of any type `==` works on: a `List`, a record, a `Time`, but
not a function or a type containing one. Two keys are the
same key when they are `==`, so a `Time` key is its instant (the same
moment written in two time zones is one key), and `Html` keys are
compared by their markup (`<p>ab</p>` and
`<p>{Html.text "a"}{Html.text "b"}</p>` are one key). A `Dict` keeps its keys in
the order they were first inserted; `==` on two dicts ignores that order.

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

Time.now       : Query Time
Time.toDate    : Time -> Date
Time.format    : Text -> Time -> Text
Time.ago       : Time -> Time -> Text           -- Time.ago now then = "3 hours ago"
```

`Date.diffDays a b` is the number of days from `a` to `b`, that is `b − a`:
`Date.diffDays monday wednesday` is `2`, and it is negative when `b` comes
first.

```alder eval=Just "2026-10-09"
Date.fromText "2026-10-08"
  |> Maybe.map (Date.addDays 1)
  |> Maybe.map Date.toText
```

## Html

```alder
Html.unsafe : Text -> Html Never                -- unescaped markup; never for user text
Html.none  : Html a                             -- renders nothing
Html.text  : Text -> Html a
Url.external : Text -> Page                     -- external link for href or Redirect
```

`Url.external` keeps a URL only when it is an absolute `http:` or `https:`
URL (`https://example.com/a`), a path of this site (`/static/app.css`,
but not `//host/…` or `/\host`, which browsers read as another site), or a
fragment (`#top`). Anything else becomes `#`, a link and a redirect that go
nowhere: `javascript:` and `data:` URLs in any case (`JavaScript:`), other
schemes (`mailto:`), URLs with leading spaces, relative paths, and any URL
holding a control character (browsers drop tabs and newlines, so `/\t/evil`
would be `//evil`). So a link or a
`Redirect` built from user input can never run script. The runtime checks
again: an `href` that is not such a URL renders as `#`, and a redirect to
one is answered with a 500 (`refusing to navigate to javascript:`).

## Query, Task, Db, Session, Log, Time, Random, Http

See chapters 7 and 8. `Query.notFound` and `Task.notFound` (both
`: Query a` / `: Task a`, no arguments) stop the effect and answer 404 with
`App.notFound`; a `Task` that ends in `Task.notFound` is rolled back like
any failed `Task`.

# 10. Testing

A test is a piece of the program that checks another piece and says yes or
no. In Alder a test is a declaration, written in the module it tests, after
the code, and `alder test` runs every test in the app. This chapter starts
with the simplest kind, a test of a pure function, then reads the report
`alder test` prints, then tests code that reads and writes the database,
and ends with what cannot be tested yet.

## 1. A test is a `Bool`

`test "name" = expression` declares a test. When its body is not a block,
the body must be a `Bool`: `True` means the test passed. The to-do app's
`Todo` module (`examples/todos/app/Todo.alder`) ends with one, which
checks that `countOpen` counts only the todos that are not done:

```alder
test "countOpen ignores done todos" =
  Todo.countOpen [
    { id: Todo.TodoId "a", title: Text.NonEmpty "x", done: False, due: Nothing, owner: User.UserId "u", created: Time.epoch },
    { id: Todo.TodoId "b", title: Text.NonEmpty "y", done: True, due: Nothing, owner: User.UserId "u", created: Time.epoch },
  ] == 1
```

Inside `Todo` the names are written unqualified (`countOpen`,
`TodoId "a"`), and that is how the file has them; the blocks of this
chapter are checked as a module of their own, so they qualify them as any
other module must. A test of another function looks the same. This one,
not in the example, checks that the `Active` filter leaves out a done
todo:

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

Because `load`, `view` and `update` are functions that return
descriptions, much of a page can be tested this way. `view` returns HTML,
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

## 2. Running the tests

`alder test` checks the app, compiles it with its tests, runs every test
and prints one line per test. In the repository:

```text
$ alder test examples/todos
ok   Theme "flip alternates"
ok   Todo "countOpen ignores done todos"
ok   Todos.Index "Toggle flips done"
ok   Todos.Index "Toggle by another user is not found"
4 passed, 0 failed
```

Each line names the module and the test, and is printed as soon as that
test finishes, so a slow test shows which tests ran before it. Tests run
in module order (a module after the modules it imports), and within a
module in the order they are written. The last line counts them.
`alder test` exits with 0 when every test passed and 1 when one failed,
so a script or CI job can use it directly. Two tests of one module may
not have the same name (E0009).

Here is the same app with two wrong tests added: one at the end of
`Todo`,

```alder
test "an empty list has one open todo" = Todo.countOpen [] == 1
```

and one at the end of `Todos.Index` that expects `Toggle` to make a
single step (sections 3 to 6 explain effect tests and traces):

```alder
test "Toggle only updates" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Todo { title: Text.NonEmpty "Milk", due: Nothing, owner: me }
  let r <- Test.run (Todos.Index.update (Toggle { id: id }) { filter: All, todos: [] } { user: me, theme: Light })
  Task.succeed (r.trace == ["Db.update Todo"])
}
```

In their modules they are written without the `Todo.` and `Todos.Index.`
prefixes. Both compile; both are wrong:

```text
$ alder test examples/todos
ok   Theme "flip alternates"
ok   Todo "countOpen ignores done todos"
FAIL Todo "an empty list has one open todo"
  left:  0
  right: 1
ok   Todos.Index "Toggle flips done"
ok   Todos.Index "Toggle by another user is not found"
FAIL Todos.Index "Toggle only updates"
  left:  ["Db.get Todo", "Db.update Todo"]
  right: ["Db.update Todo"]
  trace: ["Db.insert User", "Db.insert Todo", "Test.run", "Db.get Todo", "Db.update Todo"]
4 passed, 2 failed
```

A failing test is `FAIL`, followed by indented lines that say why:

- `left:` and `right:` are the two sides of the test's `==`, written as
  Alder values (as `alder eval` writes them). They appear when the test's
  body is an `==`, or, for a block (section 3), when the value its final
  `Task.succeed` returns is an `==`. A body such as `a == b && c == d`
  has no single `==` to show, so its failure has no sides: when you
  want the sides, end a test with one `==`, comparing records if there
  is more than one thing to check.
- `note:` appears under the sides when they print the same text but
  `==` still said `False`: `the sides print the same but are different
  values`. A `Float` that is not a number (`0.0 / 0.0` prints as `NaN`
  and is not equal to itself) is one way to get there.
- `error:` is why the test stopped before it produced `True` or `False`:
  a task that failed, a capability with no implementation, a Go panic.
  Two more stop the whole run at that test, which then fails with the
  reason and no later test runs (exit 1): a test that ran past the time
  limit (`error: timed out after 5m0s`; `-timeout`, section 7), and a
  test that crashed the program, such as a function that recurses
  forever and overflows the stack: the error is the first lines of the
  crash, `fatal error: stack overflow` and the Go function and line it
  happened in, not the whole Go trace.
- `trace:` is, for a block test, every step it performed (section 6).

A test that runs for ten seconds is announced on standard error,
`still running Theme "slow" (10s)`, and again every ten seconds, so a
test that hangs is not mistaken for a quiet one.

If the app does not compile, no test runs: `alder test` prints the
diagnostics as `alder check` does and exits 1. A typed hole (`?todo`,
chapter 11) compiles to nothing that can run either: its warning W0002 is
followed by `alder test: no tests ran (fill the hole)`, exit 1. Here the app has gained a
module `Probe` that uses `Test.trace` outside a test (section 5) and
declares `test "one" = { Task.succeed 1 }`, a block whose verdict is not
a `Bool`:

```text
$ alder test examples/todos
app/Probe.alder:4:9: error[E0003]: `Test.trace` is only available inside a `test`
  steps = Test.trace
          ^^^^^^^^^^
app/Probe.alder:6:29: error[E0003]: expected Bool, got Int
  test "one" = { Task.succeed 1 }
                              ^
```

## 3. Testing effects

A `Task` cannot be compared with `==` (section 8 says why), so a test of
`update` cannot just compare its result. Instead it runs the task and
looks at what happened. A test whose body is a block, `{ … }`, is an
**effect test**: the block is a `Task Bool`, with `let x <-` lines like
any `Task` (chapter 7), and it ends with `Task.succeed` of the verdict. A
block that ends in a plain `Bool` is an error; the compiler's hint says
to end it with `Task.succeed (…)`.

The to-do page's tests are the last thing in `app/Todos/Index.alder`
(chapter 2 shows them in place). Written in another module, as the
blocks of this chapter are, the page's names are qualified:

```alder
test "Toggle flips done" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Todo { title: Text.NonEmpty "Milk", due: Nothing, owner: me }
  let r <- Test.run (Todos.Index.update (Toggle { id: id }) { filter: All, todos: [] } { user: me, theme: Light })
  let after <- Db.get Todo id
  let done = Maybe.map (\t -> t.done) after
  Task.succeed ({ trace: r.trace, done: done } == { trace: ["Db.get Todo", "Db.update Todo"], done: Just True })
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
3. Run `update` under `Test.run` (section 4) and call the outcome `r`.
   `update` takes the message (`Toggle` for that todo), the page's model
   (`load` is not called; `update` only needs a value of the type), and
   the session. `Todos.Index` `requires user`, so its `Session` is a type
   of its own whose `user` is a `UserId`, not a `Maybe` (chapter 6). The
   record literal `{ user: me, theme: Light }` is that type because it is
   written where `update` expects one. Bound to a name first by a `let`
   without an annotation, it would be a plain record, which `update`
   does not accept.
4. Read the todo again, and take its `done` (`after` is a `Maybe Todo`:
   `Nothing` if the row were gone).
5. The verdict, as one `==` between two records: `update` read the todo
   and updated it, and the stored todo is now done. Putting everything
   the test checks into one record, rather than joining several `==`
   with `&&`, means a failure prints both records whole (section 2).

The second test is the authorization rule of chapter 6, "Authorize in
`update` too": a user who posts `Toggle` with someone else's todo id gets
a 404 and changes nothing.

```alder
test "Toggle by another user is not found" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Todo { title: Text.NonEmpty "Milk", due: Nothing, owner: me }
  let stranger = User.UserId "someone-else"
  let r <- Test.run (Todos.Index.update (Toggle { id: id }) { filter: All, todos: [] } { user: stranger, theme: Light })
  let after <- Db.get Todo id
  let done = Maybe.map (\t -> t.done) after
  Task.succeed ({ value: r.value, trace: r.trace, done: done } == { value: Err "not found", trace: ["Db.get Todo"], done: Just False })
}
```

The session's user, `stranger`, is not a row and need not be: a session
holds an id, not a reference the database checks. `owned` reads
the todo, sees another owner and stops with `Task.notFound`; the update
never runs. If someone deleted the owner check from `owned`, this test
would say exactly what went wrong:

```text
$ alder test examples/todos -run another
FAIL Todos.Index "Toggle by another user is not found"
  left:  { value: Ok Rerender, trace: ["Db.get Todo", "Db.update Todo"], done: Just True }
  right: { value: Err "not found", trace: ["Db.get Todo"], done: Just False }
  trace: ["Db.insert User", "Db.insert Todo", "Test.run", "Db.get Todo", "Db.update Todo", "Db.get Todo"]
0 passed, 1 failed
```

The stranger's `Toggle` succeeded, the update ran, and the todo is done.

**Where an effect test runs.** Every effect test gets a world of its own:

- **A fresh database.** An empty SQLite database in memory, with the
  app's tables and nothing in them. Each test has its own, so no test
  sees another's rows, and the order tests run in never matters. When the
  test ends, the database is thrown away.
- **One transaction.** The block runs as one `Task` (chapter 7, "How
  effects run"): its first database step begins a transaction, every
  later step uses it, and the block's own reads see its own writes.
- **A fixed clock.** `Time.now`, and the `@created` column a `Db.insert`
  fills, are always `2026-01-01T00:00:00Z`, so a test that stores a time
  gets the same one on every run.
- **The app's capabilities.** When the app has an `adapters/` package
  (chapter 7, "Implementing a capability"), its `Caps()` are wired, so a
  test calls the real Go. A capability's error is a failure the test can
  observe: under `Test.run` (section 4) it is `Err "Password.verify: …"`.
  When the app has no `adapters/`, a capability call fails the test,
  saying so:

  ```text
  FAIL Auth.Login "a stranger is not verified"
    error: capability Auth.Login.Password is not wired in tests
    trace: ["Password.verify"]
  ```

- **The app's default session.** A test's `Session.put` starts from
  `App`'s session with every field at its default (here
  `{ user: Nothing, theme: Light }`). A test of `update` passes the
  session it wants as an argument, as above.

A pure test (section 1) touches none of this: it is an expression,
evaluated once.

## 4. `Test.run`: running a task that may fail

`update` for another user's todo ends in `Task.notFound`. If the test
simply wrote `let nav <- Todos.Index.update …`, that failure would be the
test's failure, as a failing step is any task's failure, and the test
could never check that the 404 happened. `Test.run` catches it. Its type,
from the standard module `Test` (`stdlib/Test.alder`), is
`Test.run : Task a -> Task { value: Result Text a, trace: List Text }`.

`Test.run t` runs the task `t` inside the test and always succeeds, with
a record:

- `value` is `Ok v` when `t` succeeded with `v`, and `Err msg` when it
  failed: `msg` is the text of a `Task.fail msg`, `"not found"` for
  `Task.notFound` (and `Query.notFound`) — the failures that become a 404
  page in the running app — or, when a capability's Go code returned an
  error, the call and the error: `"Password.verify: connection refused"`. `update`'s `value` is a `Result Text Nav`, and
  `Nav` values compare with `==`, so `r.value == Ok Rerender` works too.
- `trace` is the list of steps `t` performed (section 6).

When `t` fails, what it did is undone, and only that: its writes are
rolled back and the session is put back as it was before `Test.run`; the
writes the test made before it stay. The database can do this because of
a **savepoint**: a named mark inside the open transaction. `Test.run`
sets one before `t` starts; on failure `ROLLBACK TO` the mark undoes
exactly the writes after it, and on success `RELEASE` forgets the mark,
and `t`'s writes become part of the test's. Here is the second test as
the database sees it:

```text
test "Toggle by another user is not found"   (a fresh database in memory)

  BEGIN                                       first database step: the transaction opens
  │ Db.insert User
  │ Db.insert Todo
  │ Test.run ── SAVEPOINT alder_t1
  │             │ Db.get Todo                 owned: the todo is someone else's
  │             │ Task.notFound               update fails
  │             └ ROLLBACK TO alder_t1        undo update's writes (it made none)
  │               RELEASE alder_t1            r.value = Err "not found"
  │ Db.get Todo                               the todo is still there, not done
  COMMIT                                      the verdict was produced
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

Like the page's tests, this one ends with one `==` between two records.

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

`Test.run` and `Test.trace` exist only inside a `test`. Anywhere else
they are an error, so no page can depend on them:

```alder error=E0003 "is only available inside a `test`"
steps : Task (List Text)
steps = Test.trace
```

## 6. What a trace says

Every step a `Task` performs (every `Db.*` operation, capability call,
`Time.now`, `Log.info`, `Session.put`) is recorded by name, in order,
before it runs (chapter 7, "Every step is traced"). The names:

- a database step is its function and table: `"Db.insert User"`,
  `"Db.get Todo"`, `"Db.update Todo"`, `"Db.delete Todo"`,
  `"Db.count Todo"`;
- a capability call is the capability and its function, as the calling
  module writes them: `"Password.verify"`;
- `Test.run` is a step itself, `"Test.run"`, in the test's trace.

The steps inside a `Test.run` appear twice: in `r.trace`, which holds
only them, and in the test's own trace, after `"Test.run"`, even when
the run failed and its writes were undone. That is why the report of
"Toggle only updates" above has the trace
`["Db.insert User", "Db.insert Todo", "Test.run", "Db.get Todo", "Db.update Todo"]`
while its `r.trace` was `["Db.get Todo", "Db.update Todo"]`. Compare
`r.trace` when the question is what `update` did; the test's own trace
is what the report prints to help you find out why a test failed.

A trace says what was attempted, not what the data became: it is the
same whether `Toggle` set `done` to `True` or to `False`. A test that
cares about the data reads it back, as both tests above do.

## 7. Choosing tests, and output for programs

`-run` takes a regular expression and runs only the tests whose
`Module "name"` matches it:

```text
$ alder test examples/todos -run Toggle
ok   Todos.Index "Toggle flips done"
ok   Todos.Index "Toggle by another user is not found"
2 passed, 0 failed
```

A pattern that matches nothing runs nothing and passes:
`0 passed, 0 failed`.

`-timeout` bounds the whole run (default `5m`; any Go duration, such as
`30s` or `2m`). When the time is up, the test running then fails with
`error: timed out after 30s`, the summary counts the tests that ran, and
`alder test` exits 1. The test program also stops if `alder test` itself
is killed, even with `kill -9`: it reads its standard input, which
`alder test` holds open, and exits when that closes.

`--json` is for programs, an agent among them: standard output has one
JSON object per test and nothing else, and the summary goes to standard
error.

```text
$ alder test examples/todos --json -run Theme
{"module":"Theme","name":"flip alternates","ok":true,"left":"Light","right":"Light","trace":[],"ms":0.009}
1 passed, 0 failed
```

Each object has `module`, `name`, `ok`, `trace` (empty for a pure test)
and `ms`, the time it took in milliseconds (it differs from run to run);
and `left`, `right`, `note` and `error` when they have something to say.
Unlike the text report, it gives the sides of a passing test's `==` too.
A test that crashed the program or timed out is an object like any
failing test's, with the reason in `error`. If the app does not compile,
the output is one object, `{"diagnostics":[…]}`, holding what
`alder check --json` would print; any other problem after the arguments
are read (a build failure, a bad `-run` pattern) is one object
`{"error":"…"}`. So a program always gets JSON, except for a usage
error such as an unknown flag, which prints the usage line. Chapter 11
lists the exit codes.

## 8. What cannot be tested yet

- **A request's round trip.** A test calls `load` and `update` directly.
  Nothing yet sends an HTTP request to the app and reads the response,
  so routing, the decoding of a submitted form, `requires` redirecting
  to the login page and the page that a `Nav` turns into are checked by
  the compiler and by the CLI's own Go tests, not by `alder test`.
- **`view` in a browser.** A test can compare `view`'s `Html` with
  another `Html` (section 1). What a browser does with it, and the
  in-place updates of chapter 6, are not testable from Alder.
- **Replacing a capability with a fake.** A test runs with the app's own
  adapters (section 3), or with none. There is no way yet to give one
  test a stand-in `Password` that, say, accepts everyone.
- **Observing the session after `Test.run`.** `Test.run` gives the run's
  value and trace, not the session it left; a test of a login cannot yet
  check that `Session.put` stored the user.

And one thing will not change: effects are tested by what they do, not
by `==`. A `Query` or `Task` cannot be compared; `==` or `!=` on one is a
type error:

```alder error=E0003 "effect values cannot be compared"
Query.succeed 1 == Query.succeed 1
```

An effect holds functions (each `let x <-` continues with a function of
`x`; chapter 7, "How effects run"), and two functions cannot be compared:
there is no way to tell whether they would always do the same thing. So
a test runs the effect, against a fresh database, and compares what it
did (its trace, its value) and what it left behind (the rows it reads
back).


====================  EXAMPLE APP: examples/todos  ====================
A complete, correct Alder application. Match its style exactly.

--- file: app/App.alder ---
module App

import User
import Layout
import Theme

session { user: Maybe User.UserId, theme: Theme = Light }

login = Auth.Login

msg Logout
msg SwitchTheme

update : Msg -> Session -> Task Nav
update msg session = match msg {
  Logout p -> {
    let _ <- Session.put (\s -> { s | user: Nothing })
    Task.succeed (Redirect (Auth.Login {}))
  },
  SwitchTheme p -> {
    let _ <- Session.put (\s -> { s | theme: Theme.flip s.theme })
    Task.succeed Rerender
  },
}

error : Error -> Html Never
error err =
  <Layout.Page title="Error" theme={Light}>
    <h1>Something went wrong</h1>
    <p>Please try again.</p>
  </Layout.Page>

notFound : Html Never
notFound =
  <Layout.Page title="Not found" theme={Light}>
    <h1>Not found</h1>
  </Layout.Page>

--- file: app/Auth/Login.alder ---
module Auth.Login

import User
import Layout

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

msg Submit form Credentials

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
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Log in" theme={session.theme}>
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

import Theme

Page : { title: Text, theme: Theme, children: List (Html a) } -> Html a
Page props =
  <html class={if props.theme == Dark then "dark" else ""}>
    <head>
      <meta charset="utf-8" />
      <title>{props.title}</title>
      <link rel="stylesheet" href={Url.external "/static/app.css"} />
    </head>
    <body>
      <nav>
        <button click={App.SwitchTheme}>{if props.theme == Dark then "Light mode" else "Dark mode"}</button>
      </nav>
      <main>{props.children}</main>
    </body>
  </html>

--- file: app/Theme.alder ---
module Theme

type Theme = Light | Dark

flip : Theme -> Theme
flip theme = match theme {
  Light -> Dark,
  Dark -> Light,
}

test "flip alternates" = flip (flip Light) == Light

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
    { id: TodoId "a", title: Text.NonEmpty "x", done: False, due: Nothing, owner: User.UserId "u", created: Time.epoch },
    { id: TodoId "b", title: Text.NonEmpty "y", done: True, due: Nothing, owner: User.UserId "u", created: Time.epoch },
  ] == 1

--- file: app/Todos/Index.alder ---
module Todos.Index

import Todo
import Layout
import Theme
import User

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

msg Submit form NewTodo
msg Toggle { id: Todo.TodoId }
msg Delete { id: Todo.TodoId }

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Submit p -> {
    let _ <- Db.insert Todo { title: p.form.title, due: p.form.due, owner: session.user }
    Task.succeed (Redirect (Todos.Index { filter: model.filter }))
  },
  Toggle p -> {
    let todo <- owned p.id session
    let _ <- Db.update Todo todo.id (\t -> { t | done: not t.done })
    Task.succeed Rerender
  },
  Delete p -> {
    let todo <- owned p.id session
    let _ <- Db.delete Todo todo.id
    Task.succeed Rerender
  },
}

-- owned is the todo a message names, when the session's user owns it; otherwise the message is
-- a 404. `load` only lists the user's todos, but a message's id comes from the request: anyone
-- can post `Toggle` with another user's id, so `update` checks it again.
owned : Todo.TodoId -> Session -> Task Todo
owned id session = {
  let found <- Db.get Todo id
  match found {
    Just todo -> if todo.owner == session.user then Task.succeed todo else Task.notFound,
    Nothing -> Task.notFound,
  }
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Todos" theme={session.theme}>
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

    <button click={App.Logout}>Log out</button>
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

test "Toggle flips done" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Todo { title: Text.NonEmpty "Milk", due: Nothing, owner: me }
  let r <- Test.run (update (Toggle { id: id }) { filter: All, todos: [] } { user: me, theme: Light })
  let after <- Db.get Todo id
  let done = Maybe.map (\t -> t.done) after
  Task.succeed ({ trace: r.trace, done: done } == { trace: ["Db.get Todo", "Db.update Todo"], done: Just True })
}

test "Toggle by another user is not found" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Todo { title: Text.NonEmpty "Milk", due: Nothing, owner: me }
  let stranger = User.UserId "someone-else"
  let r <- Test.run (update (Toggle { id: id }) { filter: All, todos: [] } { user: stranger, theme: Light })
  let after <- Db.get Todo id
  let done = Maybe.map (\t -> t.done) after
  Task.succeed ({ value: r.value, trace: r.trace, done: done } == { value: Err "not found", trace: ["Db.get Todo"], done: Just False })
}

--- file: app/Todos/Show.alder ---
module Todos.Show

import Todo
import Layout

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

-- The messages carry no id: the page's parameter names the todo, and `load` has checked that
-- the session's user owns it, so `update` acts on `model.todo` (book 06, "Authorize in update").
msg Save form EditTodo
msg Delete

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Save p -> {
    let _ <- Db.update Todo model.todo.id (\t -> { t | title: p.form.title, due: p.form.due })
    Task.succeed (Redirect (Todos.Index {}))
  },
  Delete p -> {
    let _ <- Db.delete Todo model.todo.id
    Task.succeed (Redirect (Todos.Index {}))
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title={model.todo.title} theme={session.theme}>
    <h1>{model.todo.title}</h1>
    <form submit={Save}>
      <Input field="title" label="Title" value={model.todo.title} />
      <Errors field="title" />
      <Input field="due" label="Due" value={model.todo.due} />
      <Errors field="due" />
      <button>Save</button>
    </form>
    <button click={Delete}>Delete</button>
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

