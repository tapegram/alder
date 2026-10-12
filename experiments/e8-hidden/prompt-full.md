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
module import type brand where page msg form table capability fake session
login test let match if then else for in requires
```

The declaration-head words (`module import type page msg form table
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

Rules 1 and 3 together decide arithmetic on two *different* brands of one
base: both widen to the base, and the result is the base. An
`Int.NonNegative` times an `Int.Positive` is an `Int`, and so is `Cents +
Int.Positive`; only `Cents + Cents` stays `Cents`.

```alder eval=6
-- 6 : Int, neither brand
(Int.NonNegative 2) * (Int.Positive 3)
```

(`Int.NonNegative 2` narrows outside the brand's module, which rule 2
forbids in a program: in `alder eval`, as in a `test`, any brand
constructor may be used; chapter 11.)

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

An `if` in markup always has an `else`, as `if` does everywhere. When one
branch should show nothing, write `else Html.none`:

```alder
<nav>
  {if model.user == Nothing then <a href={Auth.Login {}}>Log in</a> else Html.none}
</nav>
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

Two page values are `==` when they lead to the same address: a field left
at its default is not in the URL, so `Todos.Index {} == Todos.Index
{ filter: All }`.

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

### Example: a theme switch

The to-do app has one app-level message, `Logout`. A switch between a
light and a dark theme is the other usual one, and it uses every piece an
app-level message can involve: a type for the state, a session field to
keep it in, the message and its arm of `App.update`, and a button that
every page shows. (The example app leaves it out, so that its `App`,
`Layout` and login page are a skeleton a new app can copy and check with
nothing to write except its own home page.) Here is how to add it.

The state is a union, in a module of its own, `app/Theme.alder`:

```alder
module Theme

type Theme = Light | Dark

flip : Theme -> Theme
flip theme = match theme {
  Light -> Dark,
  Dark -> Light,
}

test "flip alternates" = flip (flip Light) == Light
```

`App` imports it, gives the session a field for it, and declares the
message. This block is an excerpt, and the verifier does not check it: an
app declares its `session` once, and the example's `App` (in "The `App`
module", below) already does. The new lines are the `import`, the
`theme` field, `msg SwitchTheme` and its arm:

```alder fragment
-- in App
import Theme

session { user: Maybe User.UserId, theme: Theme = Light }

msg Logout
msg SwitchTheme

update : Msg -> Session -> Task Nav
update msg session = match msg {
  Logout p -> ...,
  SwitchTheme p -> {
    let _ <- Session.put (\s -> { s | theme: Theme.flip s.theme })
    Task.succeed Rerender
  },
}
```

`theme: Theme = Light` has a default, so a visitor's first request, and a
cookie from before the field existed, get `Light` ("The session cookie",
below). `Rerender` sends the browser back to the page the button was on,
which loads again with the new session.

The layout cannot read the session; it takes the theme as a prop, sets a
class on `<html>` for the stylesheet, and shows the button. Also an
excerpt, of `app/Layout.alder`:

```alder fragment
-- in Layout
import Theme

Page : { title: Text, theme: Theme, children: List (Html a) } -> Html a
Page props =
  <html class={if props.theme == Dark then "dark" else ""}>
    ...
    <body>
      <nav>
        <button click={App.SwitchTheme}>{if props.theme == Dark then "Light mode" else "Dark mode"}</button>
      </nav>
      <main>{props.children}</main>
    </body>
  </html>
```

The class does nothing until the stylesheet has rules for it. Add them to
`static/app.css` (the to-do app's, served at `/static/app.css`), after the
rules they override:

```css
html.dark body {
  color: #e6edf3;
  background: #0d1117;
}

html.dark a {
  color: #4493f8;
}
```

Every call of `Layout.Page` then passes a theme: a page passes its
session's, and `App.error` and `App.notFound`, which have no session,
pass `Light`:

```alder fragment
<Layout.Page title="Todos" theme={session.theme}>
<Layout.Page title="Error" theme={Light}>
```

That is the cost of the feature, and why the example does without it:
every page's `view` passes the theme on, and a test that builds a session
by hand names the field too, `{ user: me, theme: Light }` (chapter 10,
"Testing effects"), and the test's module imports `Theme` for `Light`,
because a record literal lists every field, defaults or not: defaults
apply at the boundaries, the cookie, a page parameter or an insert
(chapter 12, "No defaulted record literals"). A literal that leaves `theme` out is refused, for
App's `Session` and a `requires` page's alike:

```
error[E0003]: record literal for `Session` is missing `theme` (a default applies only to an insert, the session cookie, a page or message parameter, or a form, not to a literal)
```

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

```alder fragment
-- Built in; shown for reference.
type Nav
  = Rerender                 -- re-run load and view for this page, patch the result in
  | Redirect Page            -- go to another page (HTTP 303)
  | Reject Text              -- re-render with this error shown on the form that sent the msg
```

Use `Rerender` for in-place changes (toggle, delete). Use `Redirect` after
creating something, so a browser refresh doesn't resubmit. Use `Reject` when
a well-formed submission is refused for a reason only `update` can know
(wrong password, name already taken); `<Errors form />` shows the text,
and a form that can be rejected without one gets warning W0005.

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

session { user: Maybe User.UserId }

login = Auth.Login

msg Logout

update : Msg -> Session -> Task Nav
update msg session = match msg {
  Logout p -> {
    let _ <- Session.put (\s -> { s | user: Nothing })
    Task.succeed (Redirect (Auth.Login {}))
  },
}

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
(`/_m/App/Logout?_p.filter=Active`), but `App.update` has no page, so
its handler ignores them.

### The session cookie

The session lives in one cookie, `alder_session`. Its value is two parts
joined by a dot: a small JSON document, then an HMAC-SHA256 signature of
it, both in unpadded base64url (`Sign` in `rt/web/session.go`). After
logging in to the to-do app, the first part decodes to

```json
{"exp":1794165032,"session":{"User":{"Ok":true,"Val":"oV5Et5NZUPszOeLnOSSB"}}}
```

`session` is the session. `exp` is when the cookie stops counting, in
seconds since 1970: 30 days after the session was last stored. The server
checks it itself, so a copied cookie is no good after that, whatever the
browser keeps, and a cookie without `exp` is no session at all.

A union without payloads, such as the `Theme` of "App-level messages"
above, is stored by its constructor's name (`"Theme":"Dark"`), so
reordering the constructors doesn't change what an old cookie means. A field the cookie lacks, or whose value no longer decodes, gets its
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
never leads to another site. Clicking "Log out" on `/?filter=Active`
answers a cookie whose `User` is `Nothing` and a redirect to `/login`.
With the theme switch of "App-level messages" (its `update` answers
`Rerender`), clicking "Dark mode" there answers a cookie whose `Theme` is
`"Dark"` and, under Datastar, an event that runs
`window.location.assign("/?filter=Active")` (without it,
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
a capability", below). In a test, a capability can instead be replaced by
a `fake`, an implementation written in Alder beside the capability
(chapter 10, "Faking a capability").

Capabilities are declared in any module and used by their name. When
two imported modules both declare `Clock`, the bare name is E0002
"`Clock` is a capability of both `A` and `B`: write `A.Clock` or
`B.Clock`": the call names the module too, `A.Clock.now`. The
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

**A capability in `load`.** An entry whose result is a `Query` is a
read, so `load` can call it like `Db.get`. The blog (chapter 14) keeps
its Markdown renderer behind such an entry. Its module,
`examples/blog/app/Markdown.alder`, begins:

```alder part=blog/Markdown
module Markdown

-- Rendering a post's body is outside the language: a capability implemented in Go
-- (adapters/markdown/markdown.go), which builds the HTML node by node, so a post cannot inject markup.
capability Markdown {
  render : Text -> Query (Html Never),
}
```

`Posts.Show`'s `load` writes `let body <- Markdown.render post.body` and
keeps the result in its Model. `Html Never` is markup that sends no
message, so the page's `view` can embed it in its own `Html Msg`
(chapter 5). The capability has the same name as its module. From
another module, `Markdown.render` therefore names the capability's
entry, because the module `Markdown` has no value called `render`. A
module that declared both a value `render` and the entry would be
E0009. In Go, the entry is the method
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

Chapter 10 ("Faking a capability") teaches fakes: how a test installs
one, the tables a fake may keep, and what the trace shows.

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

The other types: `Int` is `int64`, `Float` `float64`, `Text` `string`,
`Bool` `bool`, `Unit` `struct{}` (an entry returning `Task Unit` returns
`struct{}{}, nil`), `Maybe a` `rt.Maybe[A]` (`rt.Just(x)`,
`rt.Nothing[A]()`), `Result e a` `rt.Result[E, A]`, `List a` `[]A`,
`Time` `time.Time` and `Html Never` `html.Node`. The stub above lives
inside the generated package; an adapter, in its own package, imports it
as `"app"` and writes `app.Cap__…` and `app.Args__…`, with `rt.Maybe`
from `"alder/rt"`. An adapter that uses the database passes
`db.TxFrom(ctx)` (from `"alder/rt/db"`), the running task's transaction,
to every query it makes.

### A capability in the task's transaction

While `Run` runs,
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

### How the program finds an implementation

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

Your Go code goes in the app's `adapters/` directory, next to `app/`. The
Go that implements one capability is its **adapter**. You write the
adapter; the compiler writes the code that hands it to the app, so the
only thing to get right is where the adapter lives and what its one
function is called. The compiler decides both from the capability's name.

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

A few words of Go, for what follows. A Go **package** is one directory of
`.go` files that all start with the same `package <name>` line. A name
that starts with a capital letter (`LoginPassword`) is **exported**: other
packages can use it; `password` cannot be seen outside its package. A Go
**interface** is a list of methods; any type that has those methods
implements it, with nothing to declare. The function the rule names is a
**constructor**: it builds the value the app will call.

### What the build does with `adapters/`

`alder build` (chapter 11) copies the whole `adapters/` tree, every file
in every directory, into `build/<app>/adapters/`. There it sits inside the
Go module the compiler generates, which is called `app` (its `go.mod`
says `module app`). Inside that module:

- the generated package (your app, compiled to Go) is imported as `"app"`;
- your adapter package in `adapters/auth/` is `"app/adapters/auth"`;
- the runtime is `"alder/rt"`, `"alder/rt/db"`, `"alder/rt/html"` and so on.

The build then writes one more package of its own,
`build/<app>/caps/caps__alder.go`, which imports each adapter package and
calls each constructor. For the blog (chapter 14) it is:

```go
// Code generated by alder; DO NOT EDIT.

// Package caps wires the app's capabilities to their adapters: capability C of module A.B is
// implemented by package app/adapters/a (the module's first segment, lower-cased), whose
// exported func BC returns it.
package caps

import (
	"app"
	a_auth "app/adapters/auth"
	a_markdown "app/adapters/markdown"
)

// Caps returns the app's capabilities, each from its adapter's constructor.
func Caps() app.Caps {
	return app.Caps{
		Auth__Login__Password: a_auth.LoginPassword(),
		Markdown__Markdown:    a_markdown.Markdown(),
	}
}
```

Each import has a name of its own, `a_<directory>` (an *alias*: the name
the file uses for that package, whatever its `package` line says). That
is why a module named `App` works: its adapters are `adapters/app/`, whose
package would otherwise clash with the generated package `"app"`, and here
it is `a_app`. The program's `main` (`build/<app>/cmd/main.go`) imports
`"app/caps"` and starts the server with `app.Serve(caps.Caps(), …)`.
Nothing under your `adapters/` is generated or changed; the wiring is
only ever this file.

`app.Caps` is a struct with one field per capability, named
`<Module>__<Capability>`, in the generated `app__alder.go`:

```go
type Caps struct {
	Auth__Login__Password Cap__Auth__Login__Password
	Markdown__Markdown    Cap__Markdown__Markdown
}
```

Every name your adapters use from `app` is exported and has a fixed
shape: `Cap__<Module>__<Capability>` (the interface, "What a capability
looks like in Go", above), `Args__<Module>__<Capability>__<Entry>` (an
argument record), `Caps`, and `Tables` (every table's descriptor, for
`db`). The `.` in a module name becomes `__`. That is why a module may not
be named `Cap`, `Args` or `Route` (E0015, chapter 11): its names would
start the same way.

### An adapter, line by line

The blog's Markdown renderer is a whole adapter in a few lines. The
capability is `Markdown` of module `Markdown`
(`render : Text -> Query (Html Never)`, above), so the rule says
`adapters/markdown/` and `func Markdown()`. This is the top of
`examples/blog/adapters/markdown/markdown.go`:

```go
// Package markdown implements the blog's capability Markdown.Markdown in Go: a capability of
// module Markdown lives in adapters/markdown/, and its constructor is named after the
// capability. The generated package caps (build/blog/caps) wires Markdown into the app.
package markdown

import (
	"context"
	"strings"

	"alder/rt"
	"alder/rt/html"
	"app"
)

// Markdown is the adapter of Markdown.Markdown.
func Markdown() app.Cap__Markdown__Markdown { return markdown{} }

// markdown implements Markdown.Markdown: a post's body as HTML.
type markdown struct{}

// Render is Markdown.render.
func (markdown) Render(_ context.Context, args app.Args__Markdown__Markdown__Render) (html.Node, error) {
	return ToHTML(args.Value), nil
}
```

- `package markdown` names the package. Any name will do, because the
  generated `caps` imports it under its alias; the directory's name is
  the one that matters.
- `func Markdown()` is the constructor the rule asks for. Its result type
  is the interface, and it returns `markdown{}`, a value of a struct type
  with no fields. A constructor may return any value that implements the
  interface, and may declare its result as that value's own type
  (`func Markdown() markdown` works too); the generated `caps` assigns it
  to the interface-typed field, and Go checks the methods there.
- `type markdown struct{}` is lower-case, so nothing outside the package
  sees it: the app only ever reaches it through the constructor.
- `Render` is the entry `render`: a capital first letter, the context
  first, the argument in `args` (one argument that is not a record, so it
  is `args.Value`), and the entry's result with an `error`. `ToHTML`, the
  rest of the file, does the rendering.

The to-do app and the blog check passwords with `Auth.Login`'s
`Password`, so each has `adapters/auth/`. The top of
`examples/blog/adapters/auth/password.go`:

```go
// Package auth implements the blog's capability of module Auth.Login in Go: a capability of
// module Auth.… lives in adapters/auth/. `alder build` copies it into the generated module
// (build/blog/adapters/auth), next to the app's package, which it imports as "app"; the
// generated package caps (build/blog/caps) wires LoginPassword into the app.
package auth

import (
	"context"
	"errors"
	"log"
	"os"
	"sync"
	"time"

	"alder/rt"
	"alder/rt/db"
	"app"
)

// LoginPassword is the adapter of Auth.Login.Password.
func LoginPassword() app.Cap__Auth__Login__Password { return password{} }
```

The rest of the file is a development stub: in dev mode (`ALDER_DEV=1`,
which `alder run` sets) its `Verify` accepts two accounts with the
password "password", and outside dev mode it refuses every login (it is
to be replaced before the app is deployed). Its `Verify` cannot return a
made-up id: a post's `author` is `User.UserId @ref User`, so the id must
name a real `User` row. Its helper `devUser` looks the row up with
`db.StoreFrom(ctx)` and `db.TxFrom(ctx)` and inserts it the first time,
inside the transaction of the `Task` that called `Password.verify`. If
that task fails, the new row is rolled back with it. Pass
`db.TxFrom(ctx)` to every query an adapter makes: under `alder test` the
database is a single in-memory connection that the running transaction
holds, so a query made through `db.StoreFrom(ctx)` outside that
transaction waits for it forever.

When a method returns an error, the step fails with it, named after the
call: `Password.verify: <the error>`. A page answers it like any other
failure, with a 500; inside a test, `Test.run` gives it back as
`Err "Password.verify: <the error>"` (chapter 10). A capability's error is
a failure the test can observe.

The server refuses to start when a field of `Caps` is `nil` (a
constructor that returned `nil`, say). `Serve`, in `app__alder.go`,
checks every capability before it opens the database, and the binary
prints the reason and exits 1:

```
alder: capability Auth.Login.Password is not wired: adapters/auth/: func LoginPassword returned nil
```

### When an adapter is missing

Before it writes anything, `alder build` looks for every adapter the app
needs: for each capability, a `.go` file in the right directory that
declares the constructor (exported, no parameters). It does not compile
anything for this; it reads the files. Every missing adapter is listed, in
one run, each with the line to write. This is a copy of the blog without
`adapters/markdown/`:

```
$ alder build .
blog is missing adapters for 1 capability:
capability `Markdown.Markdown` needs `adapters/markdown/` with `func Markdown() app.Cap__Markdown__Markdown`
```

The build exits 1 and writes nothing: no `build/blog/` appears. `alder
check` never looks at `adapters/`: adapters are a build concern, and an
app checks clean without them.

An app with capabilities and no `adapters/` at all is the same case, with
every capability listed. Here is the blog with its `adapters/` deleted:

```
$ alder build .
blog is missing adapters for 2 capabilities:
capability `Auth.Login.Password` needs `adapters/auth/` with `func LoginPassword() app.Cap__Auth__Login__Password`
capability `Markdown.Markdown` needs `adapters/markdown/` with `func Markdown() app.Cap__Markdown__Markdown`
```

`alder test`, `alder eval` and the REPL (chapter 11), which build the app
too, treat a missing adapter differently: they do not refuse it. The
capability is left *unwired* (the generated `caps` package simply has no
line for it), they print the same list once, as a warning, and carry on:

```
$ alder test .
warning: 1 capability without an adapter runs unwired in tests:
capability `Markdown.Markdown` needs `adapters/markdown/` with `func Markdown() app.Cap__Markdown__Markdown`
```

A test that replaces the capability with its fake (`Test.fake Markdown`,
chapter 10) runs as usual. A test whose code calls a capability that no
`Test.fake` replaced fails at that call:

```
FAIL Posts.Show "a draft is shown to its author"
  error: capability Markdown.Markdown is not wired in tests
  trace: ["Db.insert User", "Db.insert Post", "Db.first Post", "Markdown.render"]
```

It is the same with some adapters written and others not, and with no
`adapters/` at all (no `.go` file anywhere under it), where nothing is
wired and there is nothing to warn about. So you can write and test an app's Alder, its fakes
first, before any of its Go exists, and add the adapters one at a time;
only `alder build`, which makes the executable you deploy, insists on
every one.

A function with the right name that is not exported (`func loginPassword()`)
or is a method does not count: the adapter is reported missing. So does
one in a `_test.go` file, which the executable never has. When the
package declares the constructor's name as something else, a function
that takes a parameter or a variable, say, the line says what it found:

```
capability `Markdown.Markdown` needs `adapters/markdown/` with `func Markdown() app.Cap__Markdown__Markdown` (found func Markdown(style string): it must take no parameters)
```

The directory's name must be exactly the lower-cased segment. On macOS,
whose file system ignores letter case, `adapters/Markdown/` would
otherwise pass for `adapters/markdown/` and confuse Go later, so the
build (and `alder test`) refuses it by name, exit 1:

```
adapters/Markdown/ must be adapters/markdown/ (an adapter package's directory is its module's first segment, lower-cased)
```

A symbolic link under `adapters/` is not followed and not copied: an
adapter directory or file that is a link counts as missing, and the list
ends with `(symlinks under adapters/ are ignored)`. Copy the files
instead.

### When an adapter does not compile

A constructor with the right name but the wrong type is found, and the
mistake surfaces when Go compiles the package. The build quotes Go's
errors as they are, under a line naming the adapters, and exits 1
(`alder test`, `alder eval` and the REPL print the same line, prefixed
with their own name, and `alder test` and `alder eval` exit 1 as well:
it is your program, like a check error). In this
copy of the blog, `Render` forgot its `error` result, and the constructor
returns its struct type:

```go
func Markdown() markdown { return markdown{} }
…
func (markdown) Render(_ context.Context, args app.Args__Markdown__Markdown__Render) html.Node {
```

```
$ alder build .
alder build: the adapters of blog (copied to build/blog/adapters) do not compile with the app: exit status 1
# app/caps
caps/caps__alder.go:18:26: cannot use a_markdown.Markdown() (value of struct type markdown.markdown) as app.Cap__Markdown__Markdown value in struct literal: markdown.markdown does not implement app.Cap__Markdown__Markdown (wrong type for method Render)
		have Render(context.Context, app.Args__Markdown__Markdown__Render) "alder/rt/html".Node
		want Render(context.Context, app.Args__Markdown__Markdown__Render) ("alder/rt/html".Node, error)
```

The error is in the generated `caps/caps__alder.go`, at the line that
hands your constructor's value to the app: that is where Go first needs
`markdown` to be a `Cap__Markdown__Markdown`. `have` is what you wrote,
`want` what the interface says. (Had the constructor declared
`app.Cap__Markdown__Markdown` as its result, the same error would point at
`return markdown{}` in your own file.) Paths are relative to
`build/blog/`, where Go ran.

### Helpers, tests and other files

Everything under `adapters/` is copied, so an adapter can be more than
one file and more than one package:

- **Helper packages.** A directory that matches no module (one whose
  name is not the first segment of a module with a capability) is copied
  and compiled with the rest, and never wired: nothing calls it but your
  adapters. Put shared Go there. The conventional place is
  `adapters/internal/`: Go lets a package under a directory named
  `internal` be imported only by code under that directory's parent, here
  `adapters/`, so nothing else can depend on it. In
  `cmd/alder/testdata/subadapters` (a small app the tests build), both
  `adapters/auth/` and `adapters/app/` import the helper
  `"app/adapters/internal/dbx"`.
- **Two names a module cannot have.** Because `adapters/internal/` is for
  helpers, a module whose first segment is `Internal` cannot have
  capabilities, and neither can one starting with `Testdata` (Go skips
  every directory named `testdata`). `alder build` refuses them before
  it looks for a single adapter (only the naming error at the end of this
  section is checked earlier), and exits 1, as `alder test` and `alder
  eval` do:

  ```
  module `Internal` cannot name an adapter package: Go keeps `adapters/internal/` private to adapters/ (it holds helpers); rename the module
  ```

  ```
  module `Testdata` cannot name an adapter package: Go skips `testdata/` directories; rename the module
  ```
- **A module named like a Go keyword.** A module `Select.Menu` with a
  capability `Choice` needs `adapters/select/` with
  `func MenuChoice() app.Cap__Select__Menu__Choice`, but `package select`
  is not legal Go: `select` is a keyword. Name the package anything else,
  `package selectadapter` for instance. The directory is still
  `adapters/select/`, and the generated `caps` imports it under its alias,
  `a_select "app/adapters/select"`, so the package's own name never
  appears there. While the adapter is missing, its line says so:

  ```
  capability `Select.Menu.Choice` needs `adapters/select/` with `func MenuChoice() app.Cap__Select__Menu__Choice` (select is a Go keyword: name the package e.g. selectadapter)
  ```
- **An adapter's own tests.** Go test files (`*_test.go`) and `testdata/`
  directories are copied too, so an adapter can be tested in Go against
  the real generated `app` package. Run them inside the build directory,
  after `alder build`:

  ```
  $ alder build .
  wrote build/blog/blog
  $ cd build/blog && go test ./adapters/...
  ok  	app/adapters/auth	0.144s
  ok  	app/adapters/markdown	0.157s
  ```

  `examples/blog/adapters/markdown/markdown_test.go` tests `ToHTML`;
  `examples/blog/adapters/auth/password_test.go` tests that the stub
  refuses every login outside dev mode. The executable contains none of
  this: `go build` never compiles test files. The build's check does,
  though: `go vet` compiles every package with its tests, so an adapter
  `_test.go` that does not compile stops `alder build` like any other
  adapter that does not compile.

`alder run` (chapter 11) rebuilds when any file under `adapters/` changes,
in any directory, except your editor's own files (a name starting with
`.`, such as `.markdown.go.swp` or `.DS_Store`; one ending in `~`, `.swp`
or `.swx`; `#markdown.go#`) and symbolic links, which the build does not
copy.

Two more messages you can meet. Two capabilities whose names join to the
same function in the same directory (module `Mail`'s `OutBox` and module
`Mail.Out`'s `Box` would both be `func OutBox` in `adapters/mail/`) cannot
both be wired, and the build stops with:

```
internal naming error: capabilities Mail.OutBox and Mail.Out.Box both need func OutBox in adapters/mail/ (rename one of them)
```

And an app written for an earlier version of Alder, whose adapters were
one package at the top of `adapters/` with a hand-written function
`Caps` returning them all, is told that this form is gone. When some adapter
packages are still missing, the list follows:

```
adapters/ no longer wires capabilities: move each adapter to adapters/<first module segment, lower-cased>/ (see the list below)
```

and when every adapter is already in place, only the old function is left
to delete:

```
adapters/ no longer wires capabilities: delete func Caps (adapters/caps.go): the generated caps package wires the adapters
```

Your editor will not resolve `import "app"` inside `adapters/`: that
package only exists in `build/<app>/`. For the same reason, an app inside
another Go module needs a `go.mod` of its own (chapter 11). That `go.mod`
goes next to `app/`, never in `adapters/`: there it would cut `adapters/`
out of the generated module, and the build refuses it (exit 1):

```
adapters/go.mod: adapters/ is part of the generated module; delete it (your editor resolves `app` in build/<app>/)
```

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
  `T`'s id type, or `Maybe` of it. A delete rule may follow it:
  `@restrict` (the default), `@cascade` or `@setNull` (below, under
  Writing).
- `@created` is set to the current time on insert. `@updated` is set on
  every write.
- A field with a default may be omitted on insert, but not from a
  literal of the row type: a `Todo` written out names every field.
- `@unique` marks a field that must be unique.
- Field types: `Int`, `Float`, `Bool`, `Text`, `Date`, `Time`, brands of
  those, unions with no payloads, and `Maybe` of any of those. Nothing
  nested; model nesting with a second table and `@ref`.

A table is declared in its own module named after it (`Todo` in
`app/Todo.alder`), together with the brand for its id and any functions
that belong to it. A table declared in any other module is an error at
its declaration (E0018): `table Comment` in `app/Post.alder` belongs in
`app/Comment.alder`, even if only `Post` uses it.

A table field's default must be a **literal**: `True` or `False`, a
number, a string without `{…}`, a constructor without a payload
(`Reader`, `Nothing`), or `Just` of one of those. The default is not only
used by `Db.insert`: migrations (`alder migrate`) write it into the
database as SQL, to fill a new field in the rows that already exist, or
to replace `Nothing` when a field stops being a `Maybe`. The compiler
turns a literal into SQL text when it lowers the table (`True` becomes
`1`, `Reader` becomes `'Reader'`, `Nothing` becomes `NULL`); an
expression would have to be run by the program, so it is refused (E0022):

```alder error=E0022 "a table field's default must be a literal"
module Note

type NoteId = brand Text

defaultBody : Text
defaultBody = "(empty)"

table Note {
  id: NoteId @id,
  body: Text = defaultBody,
}
```

Defaults of forms, records, the session and page parameters are never
stored, and may be any expression.

When you rename a table or a field, say so with **`@renamed "old"`**,
after the table's name or after the field's type (and default). The next
migration then renames the table or column, keeping its data, instead of
dropping the old one and creating an empty new one; without the
attribute a rename looks exactly like a removed field and an added one.
The attribute may stay once the migration is made (the migration's
snapshot already has the new name):

```alder
module Memo

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

**Search.** `.contains` matches a column whose text has the argument
somewhere in it, and `.startsWith` one whose text begins with it. Both
work on `Text` columns and brands of `Text` only (on a brand, the
argument is still plain `Text`), and both ignore the case of the letters
A–Z. The argument is matched literally: `%`, `_` and `\` have no special
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
10,000 bytes fails the query with `search text too long`. Cut a query a
visitor typed before searching with it, as the blog does with
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
`Todo.owner: no User with that id`. What deleting a row that other rows
still refer to does is the referring field's **delete rule**, written
after `@ref T`:

- **`@restrict`**, the default, refuses: `Db.delete` fails with
  ``User QXZMRYnkC2vuE_TefkuR is referenced by a row of Todo (`Todo.owner`)``
  (the table and the id, then the table and the field that refer to it).
  The blocking row can be further down a chain: if comments cascade
  from posts, a `@restrict` reference to a comment refuses the post's
  delete, with ``Post … is referenced by another row, directly or
  through rows that `@cascade` would delete with it``. Delete or re-point
  the referring rows first.
- **`@cascade`** deletes the referring rows too, and theirs in turn if
  other tables cascade from them.
- **`@setNull`** sets the referring field to `Nothing`; the field must be
  a `Maybe`.

Either way a failure rolls back the task's earlier writes. A table of
notes attached to to-dos, which go when their to-do goes, and which keep
an optional link to another to-do that is cleared when that one goes:

```alder
module Note

import Todo

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
(chapter 10) however many notes go with it. A field takes at most one
rule, and only a `@ref` field takes one, after `@ref T` (`@cascade` alone,
or before `@ref T`, is E0011).
`@setNull` on a field that is not a `Maybe` is E0003, since the field
could not hold the `Nothing` it would be set to:

```alder error=E0003 "`@setNull` needs a `Maybe` reference, got `Todo.TodoId`"
module Note

import Todo

type NoteId = brand Text

table Note {
  id: NoteId @id,
  todo: Todo.TodoId @ref Todo @setNull,
  body: Text,
}
```

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
`@unique` adds `UNIQUE`, `@ref` adds `REFERENCES`, followed by
`ON DELETE CASCADE` for `@cascade` and `ON DELETE SET NULL` for
`@setNull` (`@restrict` adds nothing: SQLite refuses such a delete by
default), and a `Bool` or union column gets a `CHECK` listing its
allowed values. For the to-do example's
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
- **`.contains` and `.startsWith`** are SQL's `LIKE` with `%` around
  the argument (after it, for `.startsWith`), and with `%`, `_` and `\`
  in the argument escaped, so they match themselves. `LIKE` ignores case
  for the letters A–Z only; `É` and `é` differ.
- **Empty lists.** `Db.and []` matches every row, `Db.or []` matches no
  row, and `.in []` matches no row.

Every value reaches SQLite as a parameter, never as part of the SQL text,
so no input can change what a query does.

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
summary. Add the field to `table Post` in `app/Post.alder`:

```alder fragment
  created: Time @created,
  summary: Maybe Text = Nothing,
}
```

The default `= Nothing` lets every `Db.insert Post` leave the field out,
but not a `Post` written as a literal: the checker points at the one test
that builds a whole `Post` by hand, which needs `summary: Nothing`:

```text
app/Post.alder:35:22: error[E0003]: record literal for `Post` is missing `summary` (a default applies only to an insert, the session cookie, a page or message parameter, or a form, not to a literal)
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
Read those lines before you commit. A migration you have not shared yet
can be thrown away: delete the file, restore the snapshot with
`git checkout migrations/schema.json`, change the tables, and run
`alder migrate` again. Once a migration has run against a database you
keep, never edit or delete it; write the next one instead. (A file whose
statements were edited is caught when it runs: "When the app starts",
below.)

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
  append, drop and rename columns. To change a type, make a field a
  `Maybe` or not, change a union's constructors, add or remove `@unique`,
  `@ref` or a delete rule, add a field anywhere but last, reorder fields,
  or add a field that is not a `Maybe` or has a `Just` default (filled
  with the default), the migration builds the table again: it creates
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

A conversion that cannot keep every value is marked `DROPS DATA` and
says what is lost: `Float` to `Int` cuts off fractions; `Text` to `Int`
keeps the leading whole number (`'12abc'` becomes `12`) and turns other
text into `0`; a removed constructor deletes the rows that hold it (the
line adds `(to keep them, add an UPDATE first)`: "Statements you add",
below); a `Maybe` field made required replaces `Nothing` with the
default, or, without one, deletes those rows. A change with no sensible conversion
(`Date` to `Int`, say) is refused: remove the field and add one under a
new name. Adding `@unique` to a column that holds duplicates fails when
the migration runs, and nothing is changed.

A default lives in the program, not the database, so changing only a
default needs no migration. Every `@ref` column gets an index (the
`CREATE INDEX` line above), so finding a post's comments does not read
every comment.

### When the app starts

Before it serves anything, the executable brings its database up to
date (`db.Migrate`, in `rt/db/migrate.go`):

1. In one transaction that holds the database's write lock, it reads
   `_alder_migrations` (the table listing the files applied), the
   tables, and the fingerprint recorded with the last file, and checks
   that they agree with the files: everything a refusal below names is
   found here, before anything changes.
2. It applies every other file, in order, each in its own transaction:
   a check that the tables are the file's `from`, the statements, a
   check that every `@ref` still points at a row, a check that the
   tables are now the file's `to`, and the file's name into
   `_alder_migrations`. Any check that fails undoes the file.
3. It checks that the tables are exactly the app's, and creates the
   index of every `@ref` column that lacks one.

Two copies of the app starting at once take turns: the second waits for
the first's lock, then finds the files recorded and applies nothing.

The first start on an empty database logs each file (with a date and
time in front of each line); the next start applies nothing:

```text
$ ALDER_DB=blog.db ALDER_ADDR=127.0.0.1:8080 ALDER_SECRET=… ./build/blog/blog
info: migrated 0001_initial
info: migrated 0002_add_summary
info: migrated 0003_rename_summary
info: migrated 0004_drop_excerpt
listening on 127.0.0.1:8080
```

**There are no down-migrations.** A migration only goes forward; to undo
one, restore the database from a backup. Back up the database before
every deploy that brings a migration: stop the app and copy the file
(with its `-wal` file, if there is one), or, while it runs,
`sqlite3 blog.db ".backup blog-before-0005.db"`.

If the database cannot be brought up to date, the app prints `alder: `
and the reason, and exits 1. A file that fails is rolled back, the files
before it stay applied, and nothing else changes. Word for word:

- A statement fails: the app stops with
  `migration 0003_rename_summary failed: <SQLite's error>`.
- A file's statements do not make the tables its header names (someone
  edited the file after `alder migrate` wrote it):
  `migration 0004_drop_excerpt does not produce the schema its header names (to <hash>, got <hash>): the file was edited; restore it from version control`.
- A file deletes rows that other rows still point at:
  `migration 0003_change_user: rows in Post refer to rows this migration deletes: delete or re-point them first: add an UPDATE or DELETE at the top of 0003_change_user.sql (it was not applied)`
  ("Statements you add", below).
- The database has had a file this executable does not have, newer than
  all of its own (an older build started on a newer database):
  `migration 0005_add_tags is recorded as applied but its file is missing; this executable predates the database (it has had 0005_add_tags): deploy a build that includes it, or restore a backup taken before it`.
- A file the database has had is gone from the middle:
  `migration 0003_rename_summary is recorded as applied but its file is missing: restore it from version control`.
- A new file is numbered before one the database has had:
  ``migration 0005_a is not applied but 0005_b, which comes after it, is: migrations apply in order; delete the file and run `alder migrate` again (only if it has not run against a database you keep)``.
- The files do not lead from the database's tables to the app's:
  `migration 0003_add_views expects the schema <hash> but the database is at <hash> (is a file missing, or was the database changed outside migrations?)`;
  `db: the database's tables are not the app's schema: 0004_drop_excerpt leads to the schema <hash>, but the app's is <hash>: the migrations do not produce the app's schema`;
  or, with no file left to apply,
  `db: the database's tables are not the app's schema: the database is at the schema <hash> (after 0004_drop_excerpt), but the app's is <hash>: the migrations do not produce the app's schema`.
- Another copy of the app held the database longer than five seconds
  (a long rebuild):
  `db: the database is locked: another instance is migrating or serving; wait and retry (<SQLite's error>)`.
- Someone changed the tables by hand (here, added a column with the
  `sqlite3` tool):

```text
alder: db: the database's tables are not the app's schema: the database was changed outside migrations since 0004_drop_excerpt (changed tables: Post): restore a backup; to start over, run in dev mode (ALDER_DEV=1 recreates the database empty: every row is deleted)
```

`alder check` catches the file problems first, as E0019: a missing or
edited header (``migration 0004_drop_excerpt does not follow
0002_add_summary: a file is missing or edited; restore migrations/ from
version control (`git checkout migrations/`)``), a malformed file
(``migration 0002_x: line 6: a statement must end in `;` ``), and two
files with one number, which happens when two branches each add the next
migration: ``two migrations are numbered 0005 (likely two branches:
0005_a.sql, 0005_b.sql): keep the one already merged or deployed, delete
the other, restore schema.json to its result, run `alder migrate` ``.
A file whose header is intact but whose statements were edited is found
only when it runs, by the `to` check above; `alder test` runs every
file, so it finds it before a deploy does.

### Statements you add

Some changes need a statement Alder cannot write: keeping the rows a
removed constructor would delete, or deleting the comments of posts a
migration deletes. You may add statements to a file `alder migrate`
wrote, under four rules:

- **Only in a file no database you keep has applied.** A database runs
  each file once; one that has run it never sees what you add.
- **One statement per line, ending in `;`.** A line starting with `--`
  is a comment; blank lines are skipped. Leave the header as it is.
- **Data only.** `UPDATE`, `DELETE` and `INSERT` change rows, not tables:
  the runtime checks that each file leaves the tables its `to` names,
  and refuses one that does not, as an edited file.
- **At the top, before the rebuild,** so that they run on the old table.

Removing a constructor, say `Editor` from `type Role = Reader | Author |
Editor` in the blog's `User`, rebuilds the table and deletes the editors:

```text
-- step: rebuild table User: User.role: Reader | Author | Editor -> Reader | Author
-- DROPS DATA: rows whose User.role is Editor are deleted (to keep them, add an UPDATE first)
```

and, since posts point at their author, it is refused when it runs if
an editor wrote one (the foreign-key refusal above). To make the
editors authors instead, add one line at the top of the file:

```text
-- keep the editors: they become authors
UPDATE "User__User" SET "role" = 'Author' WHERE "role" = 'Editor';
CREATE TABLE "User__User__alder__new" (…) STRICT;
…
```

A table's name in SQL is its module and its name joined by `__`
(`User__User`), and a column's is its field's. The `DROPS DATA` line
stays in the header; it says what the generated statements would lose,
and your `UPDATE` is why they no longer do.

**A database from before migrations.** An app that ran before it had
`migrations/` has a database `Ensure` made (below), with tables but no
`_alder_migrations`. If its tables are exactly what one of the files
leads to, usually `0001_initial`, the executable *adopts* it: it records
that file and the ones before it as applied without running them, logs
`adopted the database at 0001_initial`, applies the rest, and creates
any `@ref` index the database lacks. If they match no file, it refuses:

```text
alder: this database was not created by migrations and matches none of them: if the app made it before it had migrations, generate 0001 from the deployed version (check out that commit, run `alder migrate`), then migrate forward; otherwise back it up; to start over, run in dev mode (ALDER_DEV=1 recreates the database empty: every row is deleted)
```

That happens when the tables changed between the deploy that made the
database and the commit that added `migrations/`: `0001_initial`
creates the new tables, not the deployed ones. To keep the data, make
the first migration the deployed tables, and the second the change:

1. On your branch, delete `migrations/` and commit (only if its files
   have not run against a database you keep).
2. Check out the commit that is deployed and run `alder migrate` (the
   current `alder`): it writes `0001_initial`, which creates the
   deployed tables.
3. Check out your branch again (the new files come along, untracked) and
   run `alder migrate`: `0002` takes the deployed tables to yours.
   Commit `migrations/`.

The database then adopts at `0001_initial` and applies `0002`.

**Dev mode** (`alder run`, or `ALDER_DEV=1`) treats the database as
disposable. Where production refuses because the database is not where
the files say (made outside migrations and matching none, changed by
hand, not what the files produce, or of another storage format), dev
mode drops every table, creates the app's tables again, empty, records
every file as applied, and logs why:

```text
info: recreated the database (dev mode): it was changed outside migrations
```

Every refusal that dev mode would get past says what it costs:
`ALDER_DEV=1 recreates the database empty: every row is deleted`. A
file missing, out of order or edited is refused in dev mode too: the
files, not the database, are wrong. `alder eval --run` and the REPL
never recreate the database; they say ``to start over, start `alder run`
(it recreates build/<app>/dev.db empty: every row is deleted) or delete
it`` instead.

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

A `Page` compares by its address: `Todos.Index {} == Todos.Index
{ filter: All }`, since a defaulted parameter is left out of the URL.

## Query, Task, Db, Session, Log, Time, Random, Http

See chapters 7 and 8. The column methods, written on a column in place
(`Todo.title.contains q`), are not functions of a module; the compiler
knows them:

| Method | Columns | Type (for a column of type `a` in table `t`) |
|---|---|---|
| `.eq`, `.ne` | any | `a -> Condition t` |
| `.lt`, `.le`, `.gt`, `.ge` | ordered: `Int`, `Float`, `Text`, `Date`, `Time`, brands, unions without payloads | `a -> Condition t` |
| `.contains` | `Text` and its brands | `Text -> Condition t` (substring, ignores A–Z case) |
| `.startsWith` | `Text` and its brands | `Text -> Condition t` (prefix, ignores A–Z case) |
| `.in` | any | `List a -> Condition t` |
| `.isNothing`, `.isJust` | `Maybe` | `Condition t` |
| `.asc`, `.desc` | any | `Order t` |

`Query.notFound` and `Task.notFound` (both
`: Query a` / `: Task a`, no arguments) stop the effect and answer 404 with
`App.notFound`; a `Task` that ends in `Task.notFound` is rolled back like
any failed `Task`.

## Test

Only a `test` may use these (chapter 10); anywhere else each is E0003
"… is only available inside a `test`".

- `Test.run : Task a -> Task { value: Result Text a, trace: List Text }`
  runs a task that may fail: `value` is `Ok` with its result or `Err`
  with the failure's message (its writes undone), and `trace` is the
  steps it took (chapter 10, section 4).
- `Test.trace : Task (List Text)` is the steps the test has taken so far
  (chapter 10, section 5).
- `Test.fake Stripe : Task Unit` installs the `fake` of capability
  `Stripe` for the rest of the test (chapter 10, section 8). It is a
  special form, not a function: its argument is a capability's name
  (`Stripe` or `Payments.Stripe`), whose module declares a `fake`, and
  never an expression. It has no type of its own to pass around, so
  `Test.fake` cannot be applied to a variable, piped into, or left
  without its argument.

### Browser steps and queries

The browser steps (chapter 10, section 9) are special forms too: each is
applied directly to all its arguments, never passed as a value or piped
into, and `get`, `open`, `submit`, `submitRaw` and `click` need an `App`
module. `Session` here is `App`'s session record.

```text
Test.login     : Session -> Task Unit
Test.get       : Page -> Task { model: Model, session: Session, doc: Doc }
Test.open      : Page -> Task { answer: Answer, session: Session, doc: Doc }
Test.submit    : Form -> record -> Task { answer: Answer, session: Session, doc: Doc }
Test.submitRaw : Form -> List { name: Text, value: Text } -> Task { answer: Answer, session: Session, doc: Doc }
Test.click     : Msg -> Task { answer: Answer, session: Session, doc: Doc }
```

- `Test.get p` and `Test.open p` take a page value written in place
  (`Todos.Index {}`, `Todos.Show { id: id }`); `model` is the `Model`
  of `p`'s page, which must be a record. `Test.get` fails the test
  unless the page is shown.
- `Test.submit F r` takes a form of a page or of `App` (`NewTodo` in its
  module, `Todos.Index.NewTodo` elsewhere) and that form's record; a
  form each row renders, or one two messages post, is written as its
  message with its parameters, `Test.submit (Rename { id: b }) { … }`.
  `Test.submitRaw` takes the same first argument and text fields.
- `Test.click m` takes a message of a page or of `App` that carries no
  form, with its parameters: `Toggle { id: id }`, `App.Logout`.

Two types of the Prelude exist only in tests (anywhere else, E0003
"`Doc` is a test type"): `Doc`, a parsed page, which has `==`, and

```text
type Answer = Shown | Rerendered | Redirected Page | FormRejected (List { field: Text, message: Text }) | PageMissing | RequestFailed Text
```

Their names, and `Answer`'s six constructors, are taken in every module
(E0012). The queries are ordinary functions on a `Doc`, available only
in tests like the rest of `Test`:

```text
Test.text  : Text -> Doc -> Maybe Text           -- the first match's text
Test.texts : Text -> Doc -> List Text            -- every match's text, in page order
Test.has   : Text -> Doc -> Bool
Test.count : Text -> Doc -> Int
Test.attr  : Text -> Text -> Doc -> Maybe Text   -- attribute name, then selector: the first match's
Test.doc   : Html a -> Doc                       -- a view rendered and parsed
```

A selector is a tag, `#id`, `.class`, `[attr]`, `[attr=value]`, or
several of those together (`li.done`), combined with spaces (inside) or
`>` (directly inside). Written as a literal it is checked when the test
compiles (E0023); any other selector that does not parse matches
nothing.

# 10. Testing

A test is a piece of the program that checks another piece and says yes or
no. In Alder a test is a declaration, written in the module it tests, after
the code, and `alder test` runs every test in the app. This chapter starts
with the simplest kind, a test of a pure function, then reads the report
`alder test` prints, then tests code that reads and writes the database,
replaces a capability with a fake written in Alder, drives the app
through an in-memory browser as a user would, and ends with what cannot
be tested yet.

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
ok   Auth.Login "an unknown email is rejected"
ok   Auth.Login "logging in through the form"
ok   Auth.Login "a visitor is sent to log in, and can log out"
ok   Todo "countOpen ignores done todos"
ok   Todos.Index "Toggle flips done"
ok   Todos.Index "Toggle by another user is not found"
ok   Todos.Index "adding a todo through the page"
ok   Todos.Index "toggling a todo through the page"
ok   Todos.Index "a stranger cannot see my todo"
9 passed, 0 failed
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
  let r <- Test.run (Todos.Index.update (Toggle { id: id }) { filter: All, todos: [] } { user: me })
  Task.succeed (r.trace == ["Db.update Todo"])
}
```

In their modules they are written without the `Todo.` and `Todos.Index.`
prefixes. Both compile; both are wrong:

```text
$ alder test examples/todos
ok   Auth.Login "an unknown email is rejected"
ok   Auth.Login "logging in through the form"
ok   Auth.Login "a visitor is sent to log in, and can log out"
ok   Todo "countOpen ignores done todos"
FAIL Todo "an empty list has one open todo"
  left:  0
  right: 1
ok   Todos.Index "Toggle flips done"
ok   Todos.Index "Toggle by another user is not found"
ok   Todos.Index "adding a todo through the page"
ok   Todos.Index "toggling a todo through the page"
ok   Todos.Index "a stranger cannot see my todo"
FAIL Todos.Index "Toggle only updates"
  left:  ["Db.get Todo", "Db.update Todo"]
  right: ["Db.update Todo"]
  trace: ["Db.insert User", "Db.insert Todo", "Test.run", "Db.get Todo", "Db.update Todo"]
9 passed, 2 failed
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
`still running Todo "slow" (10s)`, and again every ten seconds, so a
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

A `Task` cannot be compared with `==` (section 11 says why), so a test of
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
  let r <- Test.run (Todos.Index.update (Toggle { id: id }) { filter: All, todos: [] } { user: me })
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
   record literal `{ user: me }` is that type because it is
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
  let r <- Test.run (Todos.Index.update (Toggle { id: id }) { filter: All, todos: [] } { user: stranger })
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
- **A ticking clock.** `Time.now`, and the `@created` column a
  `Db.insert` fills, read a clock that starts at `2026-01-01T00:00:00Z`
  and advances one second each time it is read, so rows created in a
  test are ordered, and a test gets the same times on every run. Time is
  not transactional: a `Test.run` that fails rolls back its writes, but
  the clock keeps the ticks that run read.
- **The app's capabilities.** When the app has adapters (Go under
  `adapters/`, chapter 7, "Implementing a capability"), they are wired as
  in `alder build`, by the generated `caps` package, so a test calls the
  real Go. A capability's error is a failure the test can observe: under
  `Test.run` (section 4) it is `Err "Password.verify: …"`. A test can
  replace a capability with a fake instead (section 8). A capability
  whose adapter is missing is not wired: `alder test` prints the list
  `alder build` would refuse, as a warning (`warning: 1 capability
  without an adapter runs unwired in tests:`, then one line per
  capability), and runs the tests anyway, so you can test with fakes
  before writing any Go. When the app has no `adapters/` at all, no
  capability is wired. Either way, a call to an unwired capability that
  no `Test.fake` replaced fails the test, saying so:

  ```text
  FAIL Auth.Login "a stranger is not verified"
    error: capability Auth.Login.Password is not wired in tests
    trace: ["Password.verify"]
  ```

- **The app's default session.** A test's `Session.put` starts from
  `App`'s session with every field at its default (here
  `{ user: Nothing }`). A test of `update` passes the
  session it wants as an argument, as above.
- **A browser.** A page held and a session cookie, both empty at the
  start, for the steps of section 9.

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
- `Test.run` is a step itself, `"Test.run"`, in the test's trace;
- a browser step is its name and what it names: `"Test.login"`,
  `"Test.get Todos.Index"`, `"Test.submit NewTodo"`,
  `"Test.click Todos.Index.Toggle"`, followed by the steps of the
  requests it made (section 9).

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
$ alder test examples/todos --json -run countOpen
{"module":"Todo","name":"countOpen ignores done todos","ok":true,"left":"1","right":"1","trace":[],"ms":0.009}
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

## 8. Faking a capability

A capability's adapter is Go code that reaches outside the program
(chapter 7). A test of a checkout should not charge a real card, and a
test of a page should not depend on how Markdown is rendered. For
those, a module can declare a **fake**: a second implementation of a
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
  render : Text -> Query (Html Never),
}

-- In a test, `let _ <- Test.fake Markdown` replaces the adapter with this: the body as plain
-- text, so a test about a page asserts on the page, not on how Markdown is rendered.
fake Markdown {
  render text = Query.succeed (Html.text text)
}
```

`fake Markdown { … }` names the capability and holds one definition for
each of its entries, here `render`, without a signature: its type is
the entry's, so `text` is a `Text` and the body a `Query (Html Never)`.

A test installs the fake with a step, `let _ <- Test.fake Markdown`.
From then until the test ends, every call of `Markdown.render` (in
`load`, in a function it calls, anywhere) runs the fake instead of the
adapter. These two tests call the entry with the fake and without it:

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
every faked trace: installing is a step, `"Test.fake Markdown"`; each
call is traced by the entry's usual name, as a call to the adapter is;
and the steps a fake takes inside an entry follow that entry's step.

The blog's `Posts.Show` has one test of each kind: one through the real
adapter, so the wiring is tested; one with the fake, checking that
`load` puts the body in its Model and that `"Markdown.render"` is in its
trace. The rendering itself is tested in Go. That is the usual split:
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
type `Payments.Charge`, used with `Db`. The entries are ordinary Alder
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
`Payments.Charge.cents`. The second test charges 500000: `charge`
inserts, then `Task.fail "card declined"`, which under `Test.run`
(section 4) is `Err "card declined"`; the savepoint undid the insert, so
one row is left. (A `Result` `Err` is an answer `pay` handles;
`Task.fail` imitates a processor that does not answer.) All three tests
pass. Outside `Test.run`, that charge fails the test.

### The rules

**The declaration.**

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
  recurses until the run stops with `fatal error: stack overflow`). Not
  `Session`, which an adapter does not have: E0003 "in `fake Stripe`'s
  `charge`: a fake cannot use `Session`: an adapter has no session".

**The fake's tables.**

- A table of `fake Stripe` in module `Payments` is the record type
  `Payments.Charge`, as an app table of that module would be. It is
  exempt from E0018 (a table in its own module).
- Only a `test`, in any module, and the fake's own entries may use it,
  its type or its columns. Anywhere else (a value, a signature, a page's
  `load`, an app table's `@ref`) it is E0021: a value
  `charges = Db.from Payments.Charge |> Db.count` in `Checkout` gets
  "`Charge` is a fake's table: only tests and `fake Stripe` may use it".

- In another module, the table and its columns carry the module's name,
  `Payments.Charge` and `Payments.Charge.cents`, and that module must
  import `Payments`. Written bare, `Charge.cents.eq 250` is E0002
  "`Charge` is a table of `fake Stripe` in module `Payments`: write
  `Payments.Charge.cents`"; in a module without `import Payments`,
  `Payments.Charge` is E0002 "`Charge` is a table of `fake Stripe` in
  module `Payments`: add `import Payments`, then write `Payments.Charge`
  (columns: `Payments.Charge.column`)", with the import as its fix.
- A fake's table may `@ref` an app table (`user: User.UserId @ref User`):
  a fake can keep, say, which user each charge was for. Give that ref
  `@cascade` (or `@setNull`), so the app's deletes behave as they do in
  production, where the fake's table does not exist: with the default
  `@restrict`, deleting a user with a charge fails with
  ``User … is referenced by a row of Charge (`Charge.user`)``. An app
  table may not `@ref` a fake's (E0021).
- The tables exist only in test databases, created beside the app's
  tables (section 3). They are never in `migrations/schema.json` or a
  migration, so adding or changing a fake's table needs no migration.

**`Test.fake`.**

- `Test.fake Stripe` is a `Task Unit`, available only inside a `test`
  like the rest of `Test` (E0003 "`Test.fake` is only available inside
  a `test`"). Its argument is a capability's name, `Stripe` or
  `Payments.Stripe`, not an expression (E0003 "`Test.fake` wants a
  capability name, got an expression"), and the capability must have a
  fake (E0003 "`Test.fake` wants a capability with a `fake`: `Stripe` has
  none"). When two imported modules both declare `Clock`, the bare name
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
- Each test starts with a fresh database and its own capabilities, so a
  fake installed in one test, and its rows, are never seen by the next.

**Where fakes exist.** `alder test` compiles them, and so do `alder eval`
and `alder repl`, whose builds are test builds too (chapter 11). A
production build (`alder build`, `alder run`) contains no tests at all:
no `test`, no fake, no fake's table and no test runner. (Chapter 13
shows the generated Go.)

## 9. Testing through the browser

A test can also use the app the way a user does: open a page, fill in and
post its form, click its buttons, and read what the page then shows and
what the session then holds, through a browser that runs in memory.

### Why through the browser

The tests so far call `load` and `update` directly. That leaves out what
happens between them in the running app: the route that reads `/todos/x`
into a page value, the decoding of a submitted form, `requires` sending a
visitor to the login page, the session a login stores, the page a
`Redirect` leads to, the buttons a page actually shows. Most of the bugs a
server-rendered app ships live at those seams, and they show only across
one request and the next.

So a test can also go through those seams. No browser program runs. The test drives the
app's real routes in memory, inside the test's own transaction, and each
step takes a fraction of a millisecond, so an app can have hundreds of
these tests and they give the same answer on every run. (Chapter 12,
"Tests drive the app in memory", says why this and not a real browser;
section 10 below shows how it works inside.)

### The browser a test holds

Every effect test (section 3) has, beside its fresh database, its clock
and its capabilities, a **browser** of its own. It holds **the page**,
the document of the last page it was given (none before the first step),
and **a cookie**, the signed cookie that carries the session (chapter 6),
sent back with every request. Before the first step there is no cookie,
so the session is `App`'s default, `{ user: Nothing }`.

Six steps act through it. Each is a `Task` (a step of the test's block,
written with `let x <-`), and each works only inside a `test`:

| Step | What it does | It gives |
|---|---|---|
| `Test.login s` | puts the session `s` in the cookie, as a login would | `Unit` |
| `Test.get p` | requests the page `p`; the test fails unless the page is shown | `{ model: Model, session: Session, doc: Doc }` |
| `Test.open p` | requests the page `p`, whatever the answer | `{ answer: Answer, session: Session, doc: Doc }` |
| `Test.submit F r` | posts the record `r` through the page's form `F` | `{ answer: Answer, session: Session, doc: Doc }` |
| `Test.submitRaw F fields` | posts text fields through the page's form `F` | `{ answer: Answer, session: Session, doc: Doc }` |
| `Test.click m` | presses the page's button that sends the message `m` | `{ answer: Answer, session: Session, doc: Doc }` |

In the results, `model` is the page's `Model`, exactly the one its
`view` was given, and typed (`page.model.todos` is a `List Todo`);
`answer` says what the request came to (`Answer`, below); `session` is
`App`'s session after the step, what the cookie now carries
(`r.session.user == Just me`); and `doc` is the page the browser now
holds, a `Doc`: a parsed HTML document, read with the queries below and
compared with `==`.

### A first browser test

The to-do page's tests end with three through the browser (chapter 2
shows them in place). The first adds a todo by its form and reads the
summary line the page shows:

```alder
test "adding a todo through the page" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Todos.Index {})
  let added <- Test.submit Todos.Index.NewTodo { title: Text.NonEmpty "Milk", due: Nothing }
  let page <- Test.get (Todos.Index {})
  Task.succeed ({ answer: added.answer, todos: List.map (\t -> t.title) page.model.todos, summary: Test.text "#summary" page.doc } == { answer: Redirected (Todos.Index {}), todos: [Text.NonEmpty "Milk"], summary: Just "1 open" })
}
```

In `app/Todos/Index.alder` the form is written `NewTodo`; from another
module, as here, it is `Todos.Index.NewTodo` (no import is needed for it).

### Step by step

Line by line:

1. Insert a user, as any effect test does.
2. `Test.login { user: Just me }` puts that session in the browser's
   cookie. The record is `App`'s session, so its `user` is a
   `Maybe UserId`: `Just me`, not `me`. Nothing is requested.
3. `Test.get (Todos.Index {})` requests `/` with the cookie. The page
   `requires user`, and the session has one, so `load` runs, `view`
   renders, and the browser now holds the page. Its result is not needed
   here (`let _ <-`).
4. `Test.submit Todos.Index.NewTodo { … }` fills the page's `NewTodo`
   form with the record and posts it. The page must show that form: the
   browser posts only what a user could. The page's `update` inserts the
   todo and answers `Redirect (Todos.Index { filter: model.filter })`; the
   browser follows the redirect, as a real one does, and now holds the
   new page. `added.answer` is `Redirected (Todos.Index {})`.
5. `Test.get` again, for the page's model: `page.model.todos` is the list
   `view` was given.
6. The verdict: the answer, the titles in the model, and the text of
   `<p id="summary">`, which `Test.text "#summary"` finds.

When the test is wrong, here with `"2 open"` expected, the report shows
what the page said and every step the requests took:

```text
$ alder test examples/todos -run "adding a todo"
FAIL Todos.Index "adding a todo through the page"
  left:  { answer: Redirected /, todos: ["Milk"], summary: Just "1 open" }
  right: { answer: Redirected /, todos: ["Milk"], summary: Just "2 open" }
  trace: ["Db.insert User", "Test.login", "Test.get Todos.Index", "Db.all Todo", "Test.submit NewTodo", "Db.all Todo", "Db.insert Todo", "Db.all Todo", "Test.get Todos.Index", "Db.all Todo"]
0 passed, 1 failed
```

A page value prints as its address: `Redirected /` is
`Redirected (Todos.Index {})`, whose `filter` is the default and so not
in the URL. Two page values are equal when their addresses are. The
trace is each step's name, then the effects of the requests it made
(section 10 explains why `Db.all Todo` comes before the insert).

### `Test.get` and `Test.open`: requesting a page

`Test.get p` and `Test.open p` take a page value, written in place:
`Todos.Index {}`, `Todos.Index { filter: Active }`,
`Todos.Show { id: id }`. A variable holding a page is not accepted.
Each sends a plain `GET` of the page's address with the cookie.

`Test.get` is for a page the test expects to see. When the page is not
shown, the test fails and says why, with the address requested. Here,
from a test added to `Todos.Index` that gets the list without logging
in:

```text
FAIL Todos.Index "a visitor's list"
  error: GET /: redirected to /login (requires user)
  trace: ["Test.get Todos.Index"]
```

The other two reasons are a 404 (here `Todos.Show`'s `load` found no
todo `x` and said `Query.notFound`) and a failed request, a 500. Because
it fails otherwise, `Test.get` can give the page's `model`, typed: the
`Model` of the page module requested.

`Test.open` is the same request, but it never fails the test on the
answer: its result's `answer` says what happened, and the test checks it.
This is how a test checks a redirect or a 404:

```alder
test "a visitor is sent to the login page" = {
  let r <- Test.open (Todos.Index {})
  Task.succeed ({ answer: r.answer, heading: Test.text "h1" r.doc, user: r.session.user } == { answer: Redirected (Auth.Login {}), heading: Just "Log in", user: Nothing })
}
```

After `Redirected p` the browser already holds `p`, so the test goes on
from there without asking for it. (It could not write `Test.open p` with
the `p` it matched anyway: a step takes a page value written in place.)

### `Answer`: what a request came to

`Answer` is a union of the Prelude, which only tests can use:

```text
type Answer = Shown
            | Rerendered
            | Redirected Page
            | FormRejected (List { field: Text, message: Text })
            | PageMissing
            | RequestFailed Text
```

| Answer | When | The page the browser then holds |
|---|---|---|
| `Shown` | `get`/`open`: the page was shown | that page |
| `Rerendered` | `submit`/`click`: `update` answered `Rerender` | the page re-rendered, with the new data |
| `Redirected p` | a `Redirect p`, or a `requires` sending a visitor to the login page | `p`, which the browser requested in turn |
| `FormRejected errors` | `submit`: the form did not decode, or `update` answered `Reject` | the page with the form re-rendered: the values typed and the errors |
| `PageMissing` | a 404: `Query.notFound`, `Task.notFound`, no such page | `App.notFound` |
| `RequestFailed text` | the request failed (a `Task.fail`, a Go panic): a 500 | `App.error` |

A redirect to another site (`Url.external`, chapter 6) is reported,
`Redirected` with that address, but not followed: the browser keeps the
page it had. A redirect to a page that is then a 404 is `PageMissing`.
`RequestFailed`'s text is the failure's message: `"nope"` for a
`Task.fail "nope"`, `"panic: …"` for a Go panic.

The 404 answer is `PageMissing`, not `NotFound`, so that an app keeps the
name `NotFound` for a constructor of its own. The six names are kept for
`Answer` everywhere: a union or `msg` of the app may not use one
(E0012), nor may a type be named `Doc` or `Answer`.

### `Test.login`, `requires`, and the session

`Test.login s` signs `s` into the browser's cookie, as the router would
after a `Session.put`; `s` is checked against `App`'s `session` record
when the test compiles (`{ }` in an app without a session). It is the
cookie's value, so a field with a default may be left out: with
`session { user: Maybe UserId, visits: Int = 0 }`,
`Test.login { user: Just me }` stores `visits: 0`.

`Test.login` is the short way in, for a test about something else. A
test about logging in itself goes through the login page with its real
form. That page calls the `Password` capability, whose adapter is Go; a
test replaces it with the fake `app/Auth/Login.alder` declares beside it
(section 8), which accepts any user who has a row, whatever the
password:

```alder
test "logging in through the form" = {
  let _ <- Test.fake Password
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.get (Auth.Login {})
  let r <- Test.submit Auth.Login.Credentials { email: Text.Email "me@example.com", password: Text.Secret "secret" }
  Task.succeed ({ answer: r.answer, user: r.session.user, title: Test.text "h1" r.doc } == { answer: Redirected (Todos.Index {}), user: Just me, title: Just "Todos" })
}
```

The page's `update` called the fake and `Session.put`; the browser kept
the cookie, followed the redirect with it, and holds the list. With an
email that has no row the fake answers `Nothing`, `update` answers
`Reject "Email or password is incorrect."`, and the answer is
`FormRejected [{ field: "", message: "Email or password is incorrect." }]`.
`Test.click App.Logout` presses the list's "Log out" button: the answer
is `Redirected (Auth.Login {})` and `session.user` is `Nothing`.

### `Test.submit`: posting a form

`Test.submit F r` posts the form `F` with the record `r`. `F` is a form a
page or `App` declares: `NewTodo` in its own module, `Todos.Index.NewTodo`
from any other. `r` is checked as the form's record (a field with a
default may be left out). The browser sends it as the page's inputs
would:

| The field's type | Sent as |
|---|---|
| `Text`, `Text.NonEmpty`, `Text.Email`, `Text.Url`, `Text.Secret`, a brand of `Text` | the text |
| `Int`, a brand of `Int` | its digits: `42`, `-3` |
| `Float` | the shortest decimal that reads back the same: `1.5`, `1e-07` |
| `Bool` | `True` when `True` (a ticked checkbox); not sent when `False` (an unticked one) |
| `Date` | `2026-03-01` |
| a union without payloads | the constructor's name: `Active` |
| `Maybe a` | blank for `Nothing` (an empty input); `a`'s text for `Just` |

Decoding it gives back `r`, except that a `Maybe Text` that is
`Just ""` is sent blank and decodes as `Nothing`: neither a test nor a
user can send `Just ""`.

The form must be on the page the browser holds. The browser posts it to
the address it posts to, with the page's parameters (a page opened as
`Todos.Index { filter: Active }` posts with its filter), and sends only
the fields the form has an input for (an `<Input>`, `<Select>` or
`<Textarea>` with the field's name), as a real browser does. A field
without one fails the step, "the page's form NewTodo has no input
`due`", unless its value is what the page makes of a field that is not
sent: `Nothing` for a `Maybe` without a default, the default for a field
that has one, `False` for a `Bool`; those are left out, and the page
decodes the same record.

A `Date` in a test record: `Date.fromText "2026-03-01"` is a
`Maybe Date`, the type of a `due: Maybe Date` field. For a `Date` field,
take one from the test's clock: `let today <- Date.today` gives
`2026-01-01`, `Date.addDays 3 today` a later day.

**A form each row renders** (`<form submit={Rename { id: n.id }}>` in a
loop), or one that two messages post: the test writes the message with
its parameters instead of the form's name,
`Test.submit (Rename { id: b }) { title: Text.NonEmpty "B2" }`, which
posts the form of row `b`.

### More forms

Here is a todo with a due date, sent through the form:

```alder
test "a todo with a due date" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Todos.Index {})
  let r <- Test.submit Todos.Index.NewTodo { title: Text.NonEmpty "Milk", due: Date.fromText "2026-03-01" }
  Task.succeed ({ answer: r.answer, due: Test.text "span.due" r.doc } == { answer: Redirected (Todos.Index {}), due: Just "Mar 1" })
}
```

When two messages post the same form, the form's name cannot say which message is
meant, so the compiler asks for the message:

```alder error=E0003 "`Title` is posted by `Save` and `Draft`: write `Test.submit (Save { id: … }) { … }`"
module Drafts

page "/drafts" { }

type Model = { ids: List Text }

load : Params -> Session -> Query Model
load params session = Query.succeed { ids: ["a", "b"] }

form Title = {
  title: Text,
}

msg Save { id: Text } form Title
msg Draft { id: Text } form Title

update : Msg -> Model -> Session -> Task Nav
update msg model session = Task.succeed Rerender

view : Model -> Session -> Html Msg
view model session =
  <html>
    <body>
      <ul>
        {for id in model.ids ->
          <li>
            <form submit={Save { id: id }}><Errors form /><Input field="title" label="Title" /><button>Save</button></form>
            <form submit={Draft { id: id }}><Errors form /><Input field="title" label="Title" /><button>Keep as draft</button></form>
          </li>}
      </ul>
    </body>
  </html>

test "renaming one draft" = {
  let _ <- Test.get (Drafts {})
  let r <- Test.submit Title { title: "x" }
  Task.succeed (r.answer == Rerendered)
}
```

Written `Test.submit (Save { id: "b" }) { title: "x" }`, the test
compiles, and posts row `b`'s Save form. A test that opens the login
page and then submits `NewTodo` gets:

```text
FAIL Todos.Index "posting a form the page lacks"
  error: the page /login has no form NewTodo (Todos.Index.Submit)
  trace: ["Db.insert User", "Test.login", "Test.get Auth.Login", "Test.submit NewTodo"]
```

### `Test.submitRaw`: input the types forbid

`Test.submit`'s record can only hold valid values: a `Text.NonEmpty` is
never empty, a `Date` is always a date. A user's browser has no such
rule, and the page must answer bad input with the form and its errors.
`Test.submitRaw F fields` posts text, field by field, as typed:
`Test.submitRaw NewTodo [{ name: "title", value: "" }]`. A field not
sent is a field left out of the form. Every name sent must be a field of
the form and have an input on the page.

**`FormRejected errors`** is the answer when the page shows its form again
with errors: the form did not decode (chapter 6: `update` never ran), or
`update` answered `Reject "…"`. The browser holds the page re-rendered,
with what was typed back in each input (except a `Text.Secret`, which is
never sent back) and each error in its `<Errors>` list. `errors` is what
those lists show:

- in the order the lists come in the page (the order of the form's
  `<Errors field="…">`, usually its fields' order);
- `field` is the field's name, and `""` for the form's own list,
  `<Errors form />`, which holds `Reject`'s text;
- a field error the form has no `<Errors field>` for is shown in
  `<Errors form />`, after `Reject`'s text, prefixed with the field's
  name, so it comes back as `{ field: "", message: "title must not be
  empty" }`;
- a form with no `<Errors form />` shows no `Reject` text at all; the
  page looks unchanged, and the answer is `Rerendered`. The compiler
  warns about such a form when its message's arm of `update` can answer
  `Reject` (W0005).

The messages are the decoders' (chapter 6): "must not be empty" for a
`Text.NonEmpty`, "must be a date (YYYY-MM-DD)", "must be a whole
number", and "is not valid" for a brand whose `where` refuses the value
(the blog's comment `Body`, chapter 14).

### Bad input, an example

The to-do form with an empty title and a date that is not one:

```alder
test "an empty title is rejected" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Todos.Index {})
  let r <- Test.submitRaw Todos.Index.NewTodo [{ name: "title", value: "" }, { name: "due", value: "tomorrow" }]
  let rows <- Db.from Todo |> Db.count
  Task.succeed ({ answer: r.answer, due: Test.attr "value" "input[name=due]" r.doc, rows: rows } == { answer: FormRejected [{ field: "title", message: "must not be empty" }, { field: "due", message: "must be a date (YYYY-MM-DD)" }], due: Just "tomorrow", rows: 0 })
}
```

The title and the date come back as typed (`Test.attr "value"` reads
the input), with their errors, and no row was written.

### `Test.click`: pressing a button

`Test.click m` sends the message `m` as the page's button for it would:
`Test.click (Toggle { id: id })`, `Test.click App.Logout`. `m` is a
message of a page or of `App`, with its parameters, written as in a
`click={…}`. A message that carries a form is posted with `Test.submit`,
not clicked; a link is followed with `Test.get` or `Test.open`.

The page must show a button (or form) that sends exactly that message,
with the same parameters: a test can only do what the user it is logged
in as can see to do. The second to-do test through the page:

```alder
test "toggling a todo through the page" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Todo { title: Text.NonEmpty "Milk", due: Nothing, owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Todos.Index {})
  let r <- Test.click (Toggle { id: id })
  Task.succeed ({ answer: r.answer, done: Test.has "li.done" r.doc, summary: Test.text "#summary" r.doc } == { answer: Rerendered, done: True, summary: Just "0 open" })
}
```

`update` answered `Rerender`, so the browser holds the page rendered
again: the item now has `class="done"`, and nothing is open.

### Clicks that fail

The compiler refuses a click of a message that carries a form:

```alder error=E0003 "`Submit` carries form `NewTodo`: use `Test.submit`"
test "clicking a form's message" = {
  let _ <- Test.get (Todos.Index {})
  let r <- Test.click (Todos.Index.Submit { form: { title: Text.NonEmpty "Milk", due: Nothing } })
  Task.succeed (r.answer == Rerendered)
}
```

**Who may toggle a todo.** Chapter 6's rule, "Authorize in `update`
too", says that a user who posts `Toggle` with someone else's
todo id gets a 404. Section 3 tests that by calling `update` with a
stranger's session. Through the browser, the same attempt (a test that
logs in as the stranger, gets the list, and clicks `Toggle { id: id }`
expecting `PageMissing`) does not get that far:

```text
FAIL Todos.Index "a stranger clicks my todo"
  error: the page / has no button or form for Todos.Index.Toggle { id: "tZy4DmaadTQ3si86vA7B" }
  trace: ["Db.insert User", "Db.insert Todo", "Test.login", "Test.get Todos.Index", "Db.all Todo", "Test.click Todos.Index.Toggle"]
```

The stranger's list shows only the stranger's todos, so there is no
Toggle button for mine to press; the browser sends nothing, and the test
fails. (The id is random, and differs on each run. Had the stranger's
list shown todos of their own, the text would end with the ones it
offers: " (the page has Toggle for id: …)".) That is the right answer,
and it is why both tests are kept: the page does not offer the action,
and section 3's test shows that `update` refuses it to anyone who posts
it anyway, without a page.

### A stranger's todo

Through the browser, the ownership test is written with what a stranger
can do, open the todo's address:

```alder
test "a stranger cannot see my todo" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Todo { title: Text.NonEmpty "Milk", due: Nothing, owner: me }
  let _ <- Test.login { user: Just (User.UserId "someone-else") }
  let page <- Test.get (Todos.Index {})
  let show <- Test.open (Todos.Show { id: id })
  Task.succeed ({ items: Test.count "li" page.doc, show: show.answer } == { items: 0, show: PageMissing })
}
```

The stranger's list has no `<li>`, and `Todos.Show`'s `load` answers the
todo's page with `Query.notFound`: `PageMissing`.

### Reading the page: queries

Five functions read a `Doc`, and one makes a `Doc` from a view. They are
plain functions (not steps), so they are used in expressions, as above:

```text
Test.text  : Text -> Doc -> Maybe Text           -- the first match's text
Test.texts : Text -> Doc -> List Text            -- every match's text, in page order
Test.has   : Text -> Doc -> Bool                 -- whether anything matches
Test.count : Text -> Doc -> Int                  -- how many elements match
Test.attr  : Text -> Text -> Doc -> Maybe Text   -- an attribute of the first match
Test.doc   : Html a -> Doc                       -- a view, as a document
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
or `,`). A selector written as a literal is checked when the test
compiles (E0023). A selector that is not a literal (in a variable, or
built with `++`) is read when the test runs, and if it is not a selector
it matches nothing: `Nothing`, `[]`, `False`, `0`. A wrong one then
looks like a page that lacks the element, so write selectors as
literals.

An element's text is the text inside it (not counting a `<script>` or
`<style>`), with every run of spaces and line breaks made one space and
none at either end. No space is put between elements: the to-do item
`<li><button>Done</button><a …>Milk</a><button>Delete</button></li>` has
the text `"DoneMilkDelete"`. Select the element that holds just the text
wanted, `Test.texts "#todos a"`, not the whole item.

**`Test.doc` and `view`.** `Test.doc (view model session)` renders a view
the way the page renders it and parses it, so a test can compare the
page the browser holds with what `view` makes of the model it was given.
Two `Doc`s are equal when they are the same tree of elements, attributes
and text, leaving out the page's parameters the router adds to each
message's address (`?_p.filter=Active`, section 10), which `view` alone
does not know. `Test.doc` renders without a request, so a form in it has
no typed values and no errors: compare a `FormRejected` page by its queries,
not with `Test.doc`.

### A selector the compiler refuses

Here the `x` after a space would be an element inside `#sumary`, but
there is no element `x`:

```alder error=E0023 "`#sumary x` is not a selector: unexpected `x` after `#sumary`; `x` is not an HTML element (a custom element has a hyphen)"
test "a misspelt selector" = {
  let page <- Test.get (Todos.Index {})
  Task.succeed (Test.text "#sumary x" page.doc == Nothing)
}
```

The full text ends with the list above: "(a selector is a tag, `#id`,
`.class`, `[attr]`, `[attr=value]`, combined with spaces or `>`)".

### Comparing a page with its view

```alder
test "the page and its view agree" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Db.insert Todo { title: Text.NonEmpty "Milk", due: Nothing, owner: me }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Todos.Index { filter: Active })
  Task.succeed ({ same: page.doc == Test.doc (Todos.Index.view page.model { user: me }), links: Test.texts "nav a" page.doc, titles: Test.texts "#todos a" page.doc, items: Test.count "#todos > li" page.doc } == { same: True, links: ["All", "Active", "Completed"], titles: ["Milk"], items: 1 })
}
```

When a test's last `==` compares two `Doc`s that differ, the report does
not print the two documents. It names the first element whose content
differs, its path from the body, and shows that element on each side.
Had the test compared the page with a view of an empty model,
`page.doc == Test.doc (Todos.Index.view { page.model | todos: [] } { user: me })`,
it would print:

```text
FAIL Todos.Index "the page and an empty view"
  left:  <ul id="todos"><li class=""><form method="post" action="/_m/Todos.Index/Toggle?id=YivaANfICyEevYmgeIK_" class="_alder-c…
  right: <ul id="todos"></ul>
  note:  the documents differ at body > main > ul#todos: left 1 child <li>, right 0
  trace: ["Db.insert User", "Db.insert Todo", "Test.login", "Test.get Todos.Index", "Db.all Todo"]
```

Each side is cut to 120 characters. The path names each element by its
tag and its `id` (or its classes), so it can be pasted into
`Test.count` to look closer.

When to use which: compare `page.model` when the question is what `load`
computed; query `doc` when it is what the user sees (a summary line, a
link's address, a button's presence); compare with `Test.doc (view …)`
to check that a page shows nothing but what `view` makes of its model.

### What a failed step says

When a step cannot do what the test asks, the test fails with one of
these texts, and nothing is sent:

- `GET /: redirected to /login (requires user)`, `GET /todos/x: not
  found`, `GET /: failed: <message>`: `Test.get` of a page not shown.
- "the page /login has no form NewTodo (Todos.Index.Submit)": the held
  page renders no form for that message.
- "the page / has no button or form for Todos.Index.Toggle { id: "x" }",
  followed by " (the page has Toggle for id: a, b)" when the page
  renders the message with other parameters.
- "the page's form NewTodo has no input `due`": a field the form renders
  no input for (`Test.submit`, `Test.submitRaw`).
- "the page has 2 forms for Notes.Index.Rename: write the message with
  its parameters".
- "no page is open: `Test.get` one first".
- "form NewTodo has no field `colour`": a `Test.submitRaw` name the form
  does not declare.

And when the test compiles (E0003 unless named):

- "`Test.get` is only available inside a `test`"; "`Test.get` needs an
  `App` module: the browser requests the app's routes, which an app
  without `App` does not serve".
- "`Test.submit` takes its arguments directly, not through `|>`: …";
  E0008 "`Test.submit` takes 2 arguments, got 1: …".
- "`Test.get` wants a page value such as `Todos.Index {}`, got `p`";
  "`Test.get` needs a record `Model`: … use `Test.open`".
- "`Test.login` wants the app's session `{ user: Maybe User.UserId }`:
  expected Maybe User.UserId, got User.UserId".
- E0002 "`NewTodo` is a form of `Todos.Index`: write
  `Todos.Index.NewTodo`"; "`Test.submit` wants a form declared by a
  page: `Contact` is a form of `Shared`, which is not a page";
  "`Test.submit` wants a form a message posts: no `msg … form Orphan` in
  `Todos.Index`"; "`Title` is posted by `Save` and `Draft`: write
  `Test.submit (Save { id: … }) { … }`"; "`Toggle` posts no form: use
  `Test.click`".
- "`Submit` carries form `NewTodo`: use `Test.submit`"; "`Test.click`
  wants a message of a page or of `App`, got `Todos.Index {}` of type
  `Page`: follow a link with `Test.get` or `Test.open`".
- E0002 "unknown constructor `NotFound`; the browser's 404 answer is
  `PageMissing`"; E0012 "`PageMissing` is a constructor of the Prelude's test
  type `Answer`; …"; "`Doc` is a test type: it exists only inside a
  `test`".
- E0023 "`#sumary x` is not a selector: unexpected `x` after `#sumary`;
  `x` is not an HTML element (a custom element has a hyphen)"; its other
  hints are "tags are lower case: `h1`" and "quote the value:
  `[href="/x"]`".
- W0005 "`Submit` can answer `Reject`, but this form has no
  `<Errors form />`: the user never sees the rejection text".

### The rules

- The steps exist only inside a `test`, each applied directly to all its
  arguments (not piped into, not passed as a value); all but
  `Test.login` need an `App` module.
- **Only what the page shows.** `Test.submit` and `Test.click` act on a
  form or button the held page renders, posting it exactly as rendered
  (the page's parameters included), and a submit sends only the fields
  the form has inputs for.
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
  `Test.get Todos.Index`, `Test.open Todos.Show`,
  `Test.submit NewTodo`, `Test.submitRaw NewTodo`,
  `Test.click Todos.Index.Toggle` or `Test.click App.Logout`, and the
  effects of the requests it made follow it.
- **What it does not test.** No CSS, no layout, no JavaScript: whether a
  button is visible, whether a web component works, how the page looks.
  The browser is Datastar's client (section 10), so the path of a
  browser with JavaScript off, a plain form post answered by a
  redirect, is not tested either (section 11).

## 10. How the browser works

Section 9 can be used without this one. This section explains what
happens when a step runs, so that a surprising trace or answer can be
read.

**The real routes, in memory.** The test program is built with the
app's routes, the same list the app's binary serves, and the same router
(`rt/web`). A step does not open a network connection: it builds an HTTP
request in memory and hands it to the router, which calls the route's
handler and writes the response into memory, where the browser reads it.
Everything between, from reading `/todos/x` into a page value to
decoding the form, checking `requires`, running `load` and `update`,
rendering and setting the cookie, is the code that runs in production.

**Inside the test's transaction.** A test's database exists in memory,
with one connection; the test's block holds a transaction on it from its
first database step (section 3). In the running app, each request begins
a transaction of its own. In a test, that would wait for ever for the
one connection, and would not see the test's uncommitted rows anyway. So
the browser gives the router each request's environment itself, one that
runs inside the test's transaction. A `load` is a `Query`, which cannot
write, so it simply reads through that transaction. An `update` is a
`Task`, and runs under a savepoint (section 4) instead of a transaction
of its own:

```text
test "adding a todo through the page"   (a fresh database in memory)

  BEGIN                                    first database step
  │ Db.insert User
  │ Test.login                             the cookie; no request
  │ Test.get                               GET /
  │   Db.all Todo                          load reads in the test's transaction
  │ Test.submit                            POST /_m/Todos.Index/Submit
  │   Db.all Todo                          load, for update's model
  │   SAVEPOINT alder_t1                   update is a Task
  │   │ Db.insert Todo
  │   └ RELEASE alder_t1                   it succeeded: the insert is the test's now
  │   Db.all Todo                          GET /: the redirect, followed
  │ Test.get                               GET /
  │   Db.all Todo
  COMMIT                                   the verdict
```

When an `update` fails, its savepoint is rolled back (`ROLLBACK TO`),
so its writes are gone and the test's are not. This is also why the
trace shows `Db.all Todo` before `Db.insert Todo`: a message's request
runs the page's `load` first, to give `update` its model (chapter 6,
"How a request runs").

**The model.** `Test.get`'s `model` is not computed again: the page's
route stores the model it rendered on the request (production does too;
nothing reads it there), and the router hands each answered request to
the browser, which keeps that model. So `model` is exactly what `view`
was given, even when `load` read the clock.

**A Datastar client.** In the running app, a click or a form post is
sent by Datastar (chapter 6), which marks the request with a
`Datastar-Request: true` header and gets back events instead of a page.
The browser sends `Test.submit` and `Test.click` the same way and
applies the events as Datastar would:

- a patch of the page's `<body>` replaces the body it holds (Datastar
  merges the new body into the old one; the result is the same page).
  `Rerender` and `Reject` come back this way. The browser then looks at
  the page: an `<Errors>` list with items makes it `FormRejected`; a 404 or
  a 500, which come back as a patch too (with the status in a header,
  `Alder-Status`), are `PageMissing` and `RequestFailed`; anything else is
  `Rerendered`. A page whose `view` has no `<body>` (every page built
  on a layout such as `Layout.Page` has one) cannot be patched, and the
  step fails "the held page has no <body> to patch".
- an instruction to go to another address is a `Redirect`: the browser
  requests that page with a plain `GET`, as Datastar has the real
  browser do, and the answer is `Redirected`. An address on another site
  is not requested.

`Test.get` and `Test.open` are plain `GET`s, like typing an address. A
`303` answer, such as `requires` sending a visitor to `/login`, is
followed (at most ten in a row).

**Controls.** The page's buttons and forms post to message addresses,
`/_m/<Module>/<Msg>` with the message's parameters and the page's own
(`/_m/Todos.Index/Toggle?id=…&_p.filter=Active`: chapter 6). Before it
posts, the browser looks through the held page for a form or button
whose address names the same message with the same parameters, ignoring
the page's `_p.` ones, and posts that control's own address, so the
page's parameters go with it as they would from a real page.

**The document.** The router's HTML is parsed into a tree, and the
browser holds that, without the `<!doctype html>` line and the Datastar
`<script>` tag the router adds to every page, so the `Doc` is the page as
`view` renders it. The queries walk that tree.

**The cookie.** The browser keeps the session cookie the router sets and
sends it back with each request; `Test.login` writes the cookie itself,
signed with the router's key. A cookie the router clears (a logout)
is dropped. The cookie's clock stands still, so it never expires
during a test.

## 11. What cannot be tested yet

The browser of section 9 runs the app's real routes, so a request's round
trip, the page a step leaves, and the session after a step are all
tested. What it does not run is a browser's own machinery:

- **CSS, layout and JavaScript.** The `Doc` is the page's HTML. Whether
  an element is visible, where it is, how it looks, and what the
  JavaScript of a web component does with it are not modelled. A small
  suite in a real browser, planned for later, will cover them.
- **The page without JavaScript.** The browser is Datastar's client, as
  every visitor's browser is when its JavaScript runs. With JavaScript
  off, a form is a plain `POST` answered by a `303` redirect (chapter 6);
  that path is not exercised.

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

session { user: Maybe User.UserId }

login = Auth.Login

msg Logout

update : Msg -> Session -> Task Nav
update msg session = match msg {
  Logout p -> {
    let _ <- Session.put (\s -> { s | user: Nothing })
    Task.succeed (Redirect (Auth.Login {}))
  },
}

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

-- Password checking is outside the language: a capability implemented in Go.
capability Password {
  verify : { email: Text.Email, password: Text } -> Task (Maybe User.UserId),
}

-- In a test, `Test.fake Password` puts this in the adapter's place: it accepts any user who has
-- a row, whatever the password, so a test can log in through the form below (book 10).
fake Password {
  verify args = {
    let found <- Db.from User |> Db.where (User.email.eq args.email) |> Db.first
    Task.succeed (Maybe.map (\u -> u.id) found)
  }
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

-- Logging in through the form, with the fake: an unknown email is rejected with the form's
-- message, and a known one stores the user in the session and goes to the to-do list.
test "an unknown email is rejected" = {
  let _ <- Test.fake Password
  let _ <- Test.get (Auth.Login {})
  let r <- Test.submit Credentials { email: Text.Email "nobody@example.com", password: Text.Secret "secret" }
  Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: FormRejected [{ field: "", message: "Email or password is incorrect." }], user: Nothing })
}

test "logging in through the form" = {
  let _ <- Test.fake Password
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.get (Auth.Login {})
  let r <- Test.submit Credentials { email: Text.Email "me@example.com", password: Text.Secret "secret" }
  Task.succeed ({ answer: r.answer, user: r.session.user, title: Test.text "h1" r.doc } == { answer: Redirected (Todos.Index {}), user: Just me, title: Just "Todos" })
}

test "a visitor is sent to log in, and can log out" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let visitor <- Test.open (Todos.Index {})
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Todos.Index {})
  let out <- Test.click App.Logout
  Task.succeed ({ visitor: visitor.answer, out: out.answer, user: out.session.user } == { visitor: Redirected (Auth.Login {}), out: Redirected (Auth.Login {}), user: Nothing })
}

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
    { id: TodoId "a", title: Text.NonEmpty "x", done: False, due: Nothing, owner: User.UserId "u", created: Time.epoch },
    { id: TodoId "b", title: Text.NonEmpty "y", done: True, due: Nothing, owner: User.UserId "u", created: Time.epoch },
  ] == 1

--- file: app/Todos/Index.alder ---
module Todos.Index

import Todo
import Layout
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
  let r <- Test.run (update (Toggle { id: id }) { filter: All, todos: [] } { user: me })
  let after <- Db.get Todo id
  let done = Maybe.map (\t -> t.done) after
  Task.succeed ({ trace: r.trace, done: done } == { trace: ["Db.get Todo", "Db.update Todo"], done: Just True })
}

test "Toggle by another user is not found" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Todo { title: Text.NonEmpty "Milk", due: Nothing, owner: me }
  let stranger = User.UserId "someone-else"
  let r <- Test.run (update (Toggle { id: id }) { filter: All, todos: [] } { user: stranger })
  let after <- Db.get Todo id
  let done = Maybe.map (\t -> t.done) after
  Task.succeed ({ value: r.value, trace: r.trace, done: done } == { value: Err "not found", trace: ["Db.get Todo"], done: Just False })
}

-- The same page through the browser: the test logs in, opens the page, and uses the form and
-- the buttons the page renders, as a user would (book 10, "Testing through the browser").
test "adding a todo through the page" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Todos.Index {})
  let added <- Test.submit NewTodo { title: Text.NonEmpty "Milk", due: Nothing }
  let page <- Test.get (Todos.Index {})
  Task.succeed ({ answer: added.answer, todos: List.map (\t -> t.title) page.model.todos, summary: Test.text "#summary" page.doc } == { answer: Redirected (Todos.Index {}), todos: [Text.NonEmpty "Milk"], summary: Just "1 open" })
}

test "toggling a todo through the page" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Todo { title: Text.NonEmpty "Milk", due: Nothing, owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Todos.Index {})
  let r <- Test.click (Toggle { id: id })
  Task.succeed ({ answer: r.answer, done: Test.has "li.done" r.doc, summary: Test.text "#summary" r.doc } == { answer: Rerendered, done: True, summary: Just "0 open" })
}

-- A stranger's page lists none of my todos, so it has no Toggle button to click for them; the
-- todo's own page is a 404 for the stranger.
test "a stranger cannot see my todo" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Todo { title: Text.NonEmpty "Milk", due: Nothing, owner: me }
  let _ <- Test.login { user: Just (User.UserId "someone-else") }
  let page <- Test.get (Todos.Index {})
  let show <- Test.open (Todos.Show { id: id })
  Task.succeed ({ items: Test.count "li" page.doc, show: show.answer } == { items: 0, show: PageMissing })
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
  <Layout.Page title={model.todo.title}>
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

