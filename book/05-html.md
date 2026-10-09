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

Text is **always escaped**. To insert trusted markup, use `Html.unsafe`,
which takes a `Text` and returns `Html Never`. It is the only way to produce
unescaped output, and it must never be applied to text a user entered.

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

```alder
<p>
  <b>a</b>
  <i>b</i>
</p>                       -- <p><b>a</b><i>b</i></p>: no space

<p><b>a</b> <i>b</i></p>   -- <p><b>a</b> <i>b</i></p>: one space

<p>
  Hello {name}
</p>                       -- "Hello " then the name

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

Children are passed through a `children` field of type `List (Html a)`:

```alder
Layout : { title: Text, children: List (Html a) } -> Html a
Layout props =
  <html>
    <head><title>{props.title}</title></head>
    <body>{props.children}</body>
  </html>

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
shows form-level errors (from `Reject`). Each renders a list, empty until
there is something to show, with one item per message:

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
| `Text`, brands of `Text`, `Int`, `Float`, `Date` | `<Input>` | `<input>` with a matching `type` |
| `Text` meant to be long | `<Textarea>` | `<textarea>` |
| `Bool` | `<Input>` | checkbox |
| union with no payloads | `<Select>` | `<select>` with one option per constructor, labeled by its name |
| `Maybe` of any of these | same as the inner type | not `required` |

A `Text.Email` field currently renders `type="text"`, like any other
`Text`; the decoder still refuses a value that is not an email address.
(Rendering `type="email"` is a planned follow-up.)

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
