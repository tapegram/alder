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
error. A tag with a hyphen is a custom element (below).

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

## Custom elements

A tag with a hyphen in its name is a *custom element*: an element a
JavaScript or CSS library defines, such as Web Awesome's `<wa-button>` or
Alder's own `<al-button>`. The browser treats an element it has no
definition for as a plain inline box, so a typo in a custom tag would
render quietly wrong. Alder checks custom elements the way it checks
built-in ones, against a *Custom Elements Manifest*: a JSON file in a
format the web-components world shares (`custom-elements.json`, schema
1.0.0), which lists each element's tag, attributes and slots. Libraries
publish one; tools generate one from a library's source.

**Alder's own elements, the `al-` design system that every app can use,
are documented in [Appendix C, "The alder-ui design
system"](C-design-system.md)**: how to link its stylesheet, its tokens
and themes, and every element with its attributes and examples. This
section is about custom elements in general.

The checker reads two manifests:

- the manifest of `alder-ui`, Alder's own design system, built into the
  compiler: `al-button`, `al-input`, `al-textarea`, `al-select`,
  `al-checkbox`, `al-table`, `al-card`, `al-badge`, `al-alert`,
  `al-pagination`, `al-stack` and `al-page` are always available, with no
  file to write;
- the app's `elements/custom-elements.json`, if there is one, next to
  `app/`: copy a library's manifest there (Web Awesome ships one) to use
  its elements. Its own files (scripts, stylesheets) go in `static/` and
  are linked from your layout, as for any stylesheet.

When both declare a tag, the app's entry wins whole: a project can
redefine `al-button` with its own attributes, and then only those are
accepted.

A custom element renders as written, its children inside it (three of
the built-in ones get a little more from the renderer: an `<al-button>`
with something to do gets a real `<button>` or `<a>` inside it, the input
elements below get their control, and an `<al-alert>` its ARIA role;
Appendix C). Its attributes take the same values as any element's
(`Text`, `Maybe Text`, `{expr}`):

```alder eval=<al-badge variant="secondary">New</al-badge>
<al-badge variant="secondary">New</al-badge>
```

A tag no manifest declares is E0028:

```alder error=E0028 "unknown element `<wa-button>`: declare custom elements in `elements/custom-elements.json` (chapter 5)"
<wa-button variant="brand">Go</wa-button>
```

So is an attribute the element's entry does not list. The message lists
the ones it does, in the manifest's order:

```alder error=E0028 "`<al-button>` has no attribute `tone`; it has `variant`, `size`, `disabled`, `type`"
<al-button tone="loud">Go</al-button>
```

A few attributes are accepted on every custom element without being
declared: the global ones, `id`, `class`, `style`, `slot`, `hidden`,
`title`, `lang`, `dir`, `tabindex`, `role`, `data-*` and `aria-*`, and
Alder's own `click`, `submit` and `href`, which mean what they mean
anywhere (a message, a page). An attribute the manifest types `boolean`
is written bare or with a `Bool`; a quoted value is refused, because a
browser treats `disabled="false"` as disabled:

```alder error=E0028 "`<al-button>` attribute `disabled` is boolean: write `disabled` or `disabled={expr}`"
<al-button disabled="false">Go</al-button>
```

### Slots

An element with *slots* places its children by name: a child with
`slot="header"` goes where the element puts its header, and children
without `slot` fill the default slot. `slot="x"` on a child must name a
slot its parent declares; the message lists the named ones (the default
slot is not listed):

```alder eval=<al-card><h2 slot="header">Notes</h2><p>Body</p></al-card>
<al-card>
  <h2 slot="header">Notes</h2>
  <p>Body</p>
</al-card>
```

```alder error=E0028 "`<al-card>` has no slot `aside`; it has `header`, `footer`, `actions`"
<al-card>
  <p slot="aside">Side</p>
</al-card>
```

Events are not checked: Alder's `click` and `submit` are messages, not
DOM events (chapter 6).

### The input elements

`al-input`, `al-textarea`, `al-select` and `al-checkbox` are the design
system's form controls, and inside a `<form submit>` they are Alder's
bound inputs too: `<al-input field="title" label="Title" />` is
`<Input field="title" label="Title" />` drawn by the design system.
`field`, `label` and `value` mean exactly what they mean on `<Input>`
(the same rules and errors, E0304); `al-textarea` is a `<Textarea>`,
`al-select` a `<Select>` (a union field), and `al-checkbox` an `<Input>`
on a `Bool` field. Their other attributes come from the manifest
(`placeholder`, `help`, `disabled`).

The element renders around the control, followed by the field's error
list, so it needs no `<Errors field>` of its own (and the form's
`<Errors form />` does not repeat its errors). After a failed submission
the element carries `invalid`, which the stylesheet uses to colour it:

```html
<al-input label="Title" placeholder="A title"><label>Title<input name="title" type="text" required placeholder="A title" /></label><ul class="errors" data-errors="title"></ul></al-input>
<al-input label="Title" placeholder="A title" invalid><label>Title<input name="title" type="text" required placeholder="A title" /></label><ul class="errors" data-errors="title"><li>must not be empty</li></ul></al-input>
```

The element's `disabled` and `placeholder` are copied onto the native
control, so they work with no stylesheet at all. Outside a form, and
without `field`, an `al-input` is an ordinary custom element around
whatever input you write inside it. Only the built-in elements wrap
Alder's inputs: a third party's `<wa-input>` is an ordinary custom
element, and inside a `<form submit>` it holds no bound input (use
`<Input>` in it).

## Partial updates

After a message, the runtime renders the whole page again and sends its
`<body>` to the browser, where Datastar morphs it into the page that is
there: elements that did not change are left alone. Without Datastar
(JavaScript off), the browser gets the whole new page. Nothing in your
code changes between the two (chapter 6, "With and without JavaScript").

`id` attributes are not required for this. They help the morph match an
old element with its new self when a list changes, so give the regions
that change (`<ul id="todos">`) a stable `id`.
