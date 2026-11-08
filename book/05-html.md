# 5. HTML

HTML is a built-in expression form. An HTML expression has type `Html`. What
a button, a form or a link leads to is a **route** (chapter 6), written in
the markup as a value, so the type of the markup does not have to say it.

In a route module `Html` is the whole type. In a module without routes
(`Layout`, an `App` without routes, a shared component) `Html` still takes a type argument for
now: write `Html a`, and `Html Never` for markup that sends nothing, such as
`App.error`. The argument is on its way out.

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
| any attribute | `Maybe Text`: omitted when `Nothing` |

A boolean attribute (HTML's: `controls`, `muted`, `async`, `defer`, …) is
written bare (`<video controls>`) or with a `Bool`, rendered present or
absent.

`href` never takes a `Text`. Links to external sites use
`<a href={Url.external "https://..."}>`.

Event attributes are not HTML event names (`onclick`); they are `click` and
`submit`, and they take a route, not a function.

> **A route of the wrong kind** is E0045: `href` to a write route (a link
> only reads), or `submit` to a `GET` route (a form posts).

#### Buttons with `click`

A `<button click={…}>` works with and without JavaScript. What it renders
depends on the route's method.

**A `POST` route.** Outside a form, the button is rendered inside a small
form of its own that posts to the route's address (here `App.Logout`,
`POST "/_/Logout"`):

```html
<form method="post" action="/_/Logout" class="_alder-click" data-on:submit="(window._alderBusy||((e,p)=&gt;p))(el, @post(&#39;/_/Logout&#39;, window._alderO?.(&#39;form&#39;)))"><button type="submit">Log out</button></form>
```

**A `PUT`, `PATCH` or `DELETE` route.** A browser form can send only `GET`
and `POST`, so such a route is sent as a `POST` that names its method in a
hidden field, `_method`. It works without JavaScript too. This is
`click={Toggle { id: "7" }}` for
`route Toggle = PATCH "/rows/{id}/toggle" { id: Text }`:

```html
<form method="post" action="/rows/7/toggle" class="_alder-click" data-on:submit="(window._alderBusy||((e,p)=&gt;p))(el, @patch(&#39;/rows/7/toggle&#39;, window._alderO?.(&#39;form&#39;)))"><input type="hidden" name="_method" value="PATCH" /><button type="submit">Toggle</button></form>
```

The router honours `_method` on a `POST` only, and never in the query
string, so a link from another site cannot become a write.

With JavaScript, Datastar sends the request with the route's own method
(`@patch` above) and the field is not needed (a `DELETE` is sent as `@post`
with `_method=DELETE`; chapter 6).

A `<form submit={R}>` whose route is a `PUT`, `PATCH` or `DELETE` does the
same: the form's own submit button carries `name="_method" value="PATCH"`,
or the form gets a hidden `_method` field when it has no submit button of
its own.

**A `GET` route.** A `click` on it is a `GET` form, with the route's query
parameters in hidden fields, so that without JavaScript the button navigates
to the route; with it, the answer is the route's view, put where the
route's root `id` stands (chapter 6, "Delivery"). `click={Index { q: "x" }}`
on `route Index = GET "/" { q: Text = "" }` is:

```html
<form method="get" action="/?q=x" class="_alder-click" data-on:submit="(window._alderBusy||((e,p)=&gt;p))(el, @get(&#39;/?q=x&#39;, window._alderF?.()))"><input type="hidden" name="q" value="x" /><button type="submit">Search</button></form>
```

**The wrapper's style.** The design system's stylesheet (Appendix C) gives
that wrapper `form._alder-click { display: contents }`, so it takes no part
in the layout and the button sits in line with the text around it.

- Without `al.css`, write that rule in your own stylesheet.
- Write a rule of your own for forms as `form:not(._alder-click) { … }`, so
  it does not catch the wrapper (the stylesheet `alder new` writes does).

**Inside a `<form submit>`** (forms cannot nest) a `click` button on a write
route is a submit button with its own `formaction`, `formmethod` and, for a
`PUT`, `PATCH` or `DELETE`, its own `_method` instead.

**Needs JavaScript.** A `click` on a `GET` route there, and a `click` on any
other element, such as a `<li>`, need JavaScript. Chapter 6, "With and
without JavaScript", has the details.

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

Children are passed through a `children` field of type `List (Html a)`:

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
module or in any other.

- In a route module it is typed `Html`.
- In a module without routes, such as `Layout`, it is typed `Html a`
  (above).
- `Html Never` (markup that sends nothing) also fits any `Html a`.

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

A request can be crafted by hand, so the browser's list is not a
guarantee: someone can post an id the page never offered, such as
another table's id, an id that names no row, or a row this user may
not see. The runtime therefore checks the value against the options
when the form is submitted.

A write carries the address of the page it was sent from (`Alder-Page`;
without JavaScript the `Referer`; chapter 6, "Delivery"), so the runtime
draws that page again and reads the options its select offered. Then:

- **A value that is not one of them** is a field error, "must be one of the
  options", answered like a form that does not decode: `422`, the page
  drawn again with the error under the select, and `update` does not run.
- **An empty value** passes only when the select offers the empty option.
- **A form posted from a page that does not offer its options** (another
  page, an address no `GET` route matches, a page that no longer shows that
  form) is refused with `400`, and the server logs why.

The check is against the page as it was drawn, so a select in a branch the
page did not draw offers nothing.

To make the check possible, `options` may use only the view's parameters
(`model` and `session`) and names it binds itself, such as a lambda's `\u`.
These are E0304:

- a select inside a `for` that takes its options from the loop's variable;
- a name a `let` or a `match` arm of the view binds;
- a select with `options` in a component, whose props the check cannot
  rebuild from the page's parameters.

Put the list in the model and the select in the `GET` route's `view`.

**A list too long to render.** A select that offers every row of a large
table is slow to draw and hard to use, and there is no `al-autocomplete`
element in the library to type into. The pattern the admin uses (chapter
15, "A long table behind a reference") needs nothing new, because the
membership rule is checked against the options just rendered.

1. Keep the narrowing text as a parameter of the page's route
   (`route New = GET "/posts/new" { findAuthor: Text = "" }`), so `New.load`
   reads it on every request.
2. Have `load` fetch at most 50 rows whose label contains it
   (`Db.where (User.email.contains params.findAuthor)`, `Db.limit 50`).
3. Add a small second form with its own write route,
   `route Find = POST "/posts/_/Find" { } form Find` with
   `form Find = { q: Text }`, whose `Find.update` answers
   `Response.redirect (New { findAuthor: p.form.q })`.

It must be a `redirect`, not a `refresh`: `refresh` draws the page at the
address it is on, with the old text, and only a request for the new address
narrows by the new text.

The select then holds the narrowed rows, and a posted value outside them
(typed by hand, or left over from an earlier search) is "must be one of the
options" like any other, because the form is posted from the narrowed page.
The pattern has no script and works with JavaScript off.

### Forms without bound inputs

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

A tag with a hyphen in its name is a *custom element*: an element a
JavaScript or CSS library defines, such as Web Awesome's `<wa-button>` or
Alder's own `<al-button>`.

The browser treats an element it has no definition for as a plain inline
box, so a typo in a custom tag would render quietly wrong.

Alder checks custom elements the way it checks built-in ones, against a
*Custom Elements Manifest*: a JSON file in a format the web-components world
shares (`custom-elements.json`, schema 1.0.0), which lists each element's
tag, attributes and slots. Libraries publish one; tools generate one from a
library's source.

**Alder's own elements, the `al-` design system that every app can use,
are documented in [Appendix C, "The alder-ui design
system"](C-design-system.md)**: how to link its stylesheet, its tokens
and themes, and every element with its attributes and examples. This
section is about custom elements in general.

### The two manifests

The checker reads two manifests:

- **The manifest of `alder-ui`**, Alder's own design system, built into the
  compiler: `al-button`, `al-input`, `al-textarea`, `al-select`,
  `al-checkbox`, `al-table`, `al-card`, `al-badge`, `al-alert`,
  `al-pagination`, `al-stack` and `al-page` are always available, with no
  file to write.

- **The app's `elements/custom-elements.json`**, if there is one, next to
  `app/`: copy a library's manifest there (Web Awesome ships one) to use
  its elements. Its own files (scripts, stylesheets) go in `static/` and
  are linked from your layout, as for any stylesheet.

When both declare a tag, the app's entry wins whole: a project can
redefine `al-button` with its own attributes, and then only those are
accepted.

### Using a library's elements

For example, to use Web Awesome's `<wa-button>`, the app's manifest
needs at least this (a library's real manifest has much more, which the
checker ignores):

```json
{
  "schemaVersion": "1.0.0",
  "modules": [
    {
      "kind": "javascript-module",
      "path": "components/button/button.js",
      "declarations": [
        {
          "kind": "class",
          "name": "WaButton",
          "tagName": "wa-button",
          "customElement": true,
          "attributes": [
            { "name": "variant", "type": { "text": "'neutral' | 'brand' | 'success' | 'warning' | 'danger'" } },
            { "name": "size", "type": { "text": "'small' | 'medium' | 'large'" } },
            { "name": "disabled", "type": { "text": "boolean" } }
          ],
          "slots": [{ "name": "" }, { "name": "start" }, { "name": "end" }]
        }
      ]
    }
  ]
}
```

and the layout loads the library, its stylesheet and its script, from
`static/` or from its CDN, in the `<head>`:

```alder fragment
<link rel="stylesheet" href={Url.external "/static/webawesome/styles/webawesome.css"} />
<script type="module" src="/static/webawesome/webawesome.loader.js"></script>
```

With both, `<wa-button variant="brand" size="small">Go</wa-button>` checks,
and `variant="primary"` or `<span slot="icon">` inside it is E0028.

### How a custom element renders

A custom element renders as written, its children inside it. Three of the
built-in ones get a little more from the renderer:

- an `<al-button>` with something to do gets a real `<button>` or `<a>`
  inside it;
- the input elements below get their control;
- an `<al-alert>` gets its ARIA role (Appendix C).

Its attributes take the same values as any element's (`Text`, `Maybe Text`,
`{expr}`):

```alder eval=<al-badge variant="secondary">New</al-badge>
<al-badge variant="secondary">New</al-badge>
```

### What E0028 refuses

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
declared:

- **The global ones:** `id`, `class`, `style`, `slot`, `hidden`, `title`,
  `lang`, `dir`, `tabindex`, `role`, `data-*` and `aria-*`.
- **The popover and invoker attributes** `popover`, `popovertarget`,
  `popovertargetaction`, `commandfor`, `command` and `closedby`, which HTML
  also allows on any element (so `<al-menu id="more" popover>` is written
  as Appendix C shows it; a bare `popover` is the platform's
  `popover="auto"`).
- **Alder's own `click` and `href`,** which mean what they mean anywhere (a
  route).

`submit` is not among them: a custom element is not a form, so nothing
would ever submit it.

```alder error=E0028 "`submit` belongs on `<form>`; use `type=\"submit\"` inside `<form submit>`"
<al-button submit={App.Logout}>Log out</al-button>
```

An attribute the manifest types `boolean` is written bare or with a
`Bool`; a quoted value is refused, because a browser treats
`disabled="false"` as disabled:

```alder error=E0028 "`<al-button>` attribute `disabled` is boolean: write `disabled` or `disabled={expr}`"
<al-button disabled="false">Go</al-button>
```

An attribute whose manifest type is a list of quoted values
(`"sm" | "md" | "lg"`) takes one of them. A literal value is checked, and
the message lists the values:

```alder error=E0028 "`<al-badge>` attribute `variant` is one of `default`, `secondary`, `outline`, `destructive`"
<al-badge variant="destuctive">Failed</al-badge>
```

A computed value (`variant={…}`) is any `Text`: it is not checked, and an
unknown one is simply not styled.

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

A slot is filled only by a direct child (an `if`, `for` or `match` in
between is fine), so `slot` on a child of a plain element is refused:

```alder error=E0028 "`slot` names a slot of the enclosing custom element; `<p slot=\"header\">` is inside `<div>`"
<al-card>
  <div><p slot="header">Notes</p></div>
</al-card>
```

> **Not checked: slots through a component.** The children you pass to a
> component are not checked against the slots of the element it puts them
> in. In `<Ui.Card><p slot="aside">…</p></Ui.Card>`, where `Ui.Card`
> renders `<al-card>{props.children}</al-card>`, the checker sees only the
> component's call, so a wrong slot name there renders quietly in the
> default slot.

Events are not checked: Alder's `click` and `submit` name routes, not
DOM events (chapter 6).

### The input elements

`al-input`, `al-textarea`, `al-select` and `al-checkbox` are the design
system's form controls, and inside a `<form submit>` they are Alder's
bound inputs too: `<al-input field="title" label="Title" />` is
`<Input field="title" label="Title" />` drawn by the design system.

`field`, `label` and `value` mean exactly what they mean on `<Input>` (the
same rules and errors, E0304).

- `al-textarea` is a `<Textarea>`.
- `al-select` is a `<Select>` (a union field).
- `al-checkbox` is an `<Input>` on a `Bool` field.

Their other attributes come from the manifest (`placeholder`, `help`,
`disabled`).

The element renders around the control, then its `help` text, then the
field's error list. So it needs no `<Errors field>` of its own; writing
one for the same field is E0028 (`<al-input field="title">` already shows
the errors of `title`), and the form's `<Errors form />` does not repeat
its errors.

`<al-input field="title" label="Title" placeholder="A title"
help="Short is good" />` renders as the first line below, and after a
failed submission as the second (the book's tests render both and
compare):

```html
<al-input label="Title" placeholder="A title" help="Short is good"><label>Title<input name="title" type="text" required placeholder="A title" aria-describedby="help-title" /></label><small class="help" id="help-title">Short is good</small><ul class="errors" data-errors="title"></ul></al-input>
<al-input label="Title" placeholder="A title" help="Short is good" invalid><label>Title<input name="title" type="text" required placeholder="A title" aria-invalid="true" aria-describedby="help-title errors-title" /></label><small class="help" id="help-title">Short is good</small><ul class="errors" data-errors="title" id="errors-title"><li>must not be empty</li></ul></al-input>
```

The help is a real `<small class="help" id="help-<field>">`, which the
control's `aria-describedby` names, so a screen reader reads it with the
field.

After the failure:

- the element carries `invalid`, which the stylesheet uses to colour it;
- the control has `aria-invalid="true"`;
- the error list gets the `id` `errors-<field>`, which `aria-describedby`
  names too (only while there are errors: one form per row would repeat
  the `id`).

The element's `disabled` and `placeholder` are copied onto the native
control, so they work with no stylesheet at all. Other attributes, an `id`
or `aria-*` included, stay on the element: they are not forwarded to the
control, which is named by its `<label>` and described by the ids Alder
derives from the field's name.

Outside a form, and without `field`, an `al-input` is an ordinary custom
element around whatever input you write inside it.

> **Only the built-in elements wrap Alder's inputs.** A third party's
> `<wa-input>` is an ordinary custom element, and inside a `<form submit>`
> it holds no bound input (use `<Input>` in it).

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
