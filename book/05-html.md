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
shows form-level errors (from `Reject`).

Which component to use depends on the field's type:

| Field type | Component | Renders |
|---|---|---|
| `Text`, brands of `Text`, `Int`, `Float`, `Date` | `<Input>` | `<input>` with a matching `type` |
| `Text` meant to be long | `<Textarea>` | `<textarea>` |
| `Bool` | `<Input>` | checkbox |
| union with no payloads | `<Select>` | `<select>` with one option per constructor, labeled by its name |
| `Maybe` of any of these | same as the inner type | not `required` |

`value={...}` on any of them takes the field's own type (`value={model.todo.priority}`
for a `Select` over `Priority`), or `Maybe` of it.

A `<form submit={M}>` where `M` has no `form` type is allowed; it contains
no bound inputs, only a button.

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
