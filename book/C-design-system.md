# Appendix C. The alder-ui design system

Every Alder app can use a small design system, **alder-ui**: a
stylesheet, and a set of HTML elements whose names start with `al-`. You
write `<al-card>`, `<al-input>` and `<al-button>` in a view, link one
stylesheet in the layout, and the page has a consistent look: cards,
fields with their labels and errors, buttons in five variants, tables,
badges, alerts, pagination, and a light and a dark theme. `alder new`
uses it, and so do both example apps.

This appendix is alder-ui's documentation. It covers what the library
is, how an app links it, the tokens that define its look and how to
change them, every element with its attributes and an example checked
by the book's tests, what Alder's renderer adds to some of them, and how
to use the library from a stack other than Alder.

Three ideas shape the library (chapter 12, "Why a design system in the
box", explains why):

- **Light DOM.** An `al-` element is an ordinary element in the page.
  It has no *shadow root*, the private subtree a JavaScript-defined
  component can hide its insides in. Everything you write inside
  `<al-card>` is in the page as you wrote it, so forms submit, labels
  work, Datastar's morph (chapter 5, "Partial updates") sees every
  element, and your own CSS reaches everything.
- **Tokens.** Every colour, radius, space and font the library uses is a
  CSS custom property (a *token*, such as `--primary`), defined once.
  Change a token and every element follows. The colour tokens use the
  names of shadcn/ui, a widely copied component library, so a theme
  written for it mostly carries over.
- **CSS first.** The first tier is a stylesheet and nothing else: the
  elements are not defined in JavaScript at all; the interactive tier adds
  small optional scripts, and uses the platform where it can. A browser shows an
  element it has no definition for as a plain box, and the stylesheet
  styles that box by its tag name and attributes. Every element works
  with JavaScript off.

The version Alder ships is **v0.2.1**: the compiler embeds a copy of
the library's stylesheet, scripts (`al.js`) and manifest at that tag (`rt/al/VERSION`;
"Where it lives", below).

## Linking it

Every build writes the library's two files into the build's static
directory, next to the copy of your app's `static/`: `al/al.css` (the
stylesheet) and `al/custom-elements.json` (the manifest, below). So the
server `alder build` makes, and the one `alder run` starts, serve them at
`/static/al/al.css` and `/static/al/custom-elements.json`. You do not
copy them into your `static/` directory; Alder writes them, from the
copy inside the compiler (a file of yours at `static/al/al.css` would be
replaced). Their address stays the same when you upgrade Alder, so they
are not cached for good: both are served with
`Cache-Control: public, max-age=300, stale-while-revalidate=86400` and an
`ETag`, a hash of their content. The browser uses its copy for five
minutes without asking, then keeps using it (for up to a day) while it
asks in the background whether it is still current, and the server
answers `304 Not Modified` (no body) until a new compiler ships a new
stylesheet. Not asking before every page is what keeps a page load from
waiting on the network before it paints (chapter 11 has the trade).
Datastar's script is different: its address carries its version
(`/_alder/datastar-v1.0.0.js`), so it is cached for a year without
asking.

A page uses them by linking the stylesheet in its `<head>`, as the
layout `alder new` writes does:

```alder fragment
<link rel="stylesheet" href={Url.external "/static/al/al.css"} />
<link rel="stylesheet" href={Url.external "/static/app.css"} />
```

Link it before your own stylesheet, so your rules come after it.

An app that does not link it still checks and renders the same markup:
the `al-` elements are in the page, unstyled, and the browser shows
their contents as plain inline text, buttons and inputs. Nothing breaks;
it only looks plain. The checker does not require the link, because a
layout may load the stylesheet another way (a CDN, a bundle of its own).

## Tokens and themes

The stylesheet starts with the tokens. The colour tokens, from shadcn/ui:

| Token | Used for |
|---|---|
| `--background`, `--foreground` | the page |
| `--card`, `--card-foreground` | `al-card` |
| `--popover`, `--popover-foreground` | reserved for the interactive tier |
| `--primary`, `--primary-foreground` | primary buttons, default badges |
| `--secondary`, `--secondary-foreground` | secondary buttons and badges |
| `--muted`, `--muted-foreground` | striped rows; help text, placeholders, table headers |
| `--accent`, `--accent-foreground` | the hover of outline and ghost buttons and of pagination links |
| `--destructive`, `--destructive-foreground` | destructive buttons and badges |
| `--border` | borders of cards, tables, badges |
| `--input` | the borders of fields |
| `--ring` | the focus ring |
| `--radius` | the corner radius |

alder-ui's own tokens start with `--al-`: `--al-destructive-text` (the
colour of error messages and of an invalid field's border), the soft
alert colours (`--al-info-soft`, `--al-info-soft-foreground`,
`--al-info-border`, and the same for `success`, `warning` and
`destructive`), spacing (`--al-space-1` to `--al-space-10`), type
(`--al-font-sans`, `--al-font-mono`, `--al-text-xs` to `--al-text-lg`,
`--al-weight-*`), control heights (`--al-control-h-sm`, `-md`, `-lg`),
the page widths (`--al-width-narrow`, `-normal`, `-wide`), and the focus
ring (`--al-ring-width`, `--al-ring-offset`).

Every pairing of a foreground with its background meets WCAG AA contrast
(4.5:1 for text, 3:1 for borders and the focus ring) in both themes; the
library's tests compute the ratios from the token values. Three values
differ from shadcn's defaults for that reason: the light `--destructive`
is darker, and `--input` is darker in both themes, so a field's border
can be seen.

### Light and dark

The light theme is the default. The dark theme applies when the
operating system asks for it (`prefers-color-scheme: dark`), unless the
page says otherwise with a `data-theme` attribute:

- `data-theme="dark"` forces the dark theme;
- `data-theme="light"` forces the light one.

Put it on `<html>` for the whole page, or on any element for that part
of the page: a dark card on a light page is
`<al-card data-theme="dark">`. `data-theme` is an ordinary `data-`
attribute, so the checker accepts it anywhere.

### Between pages: no white flash

In the dark theme, a page load used to show a white frame: the browser
paints a blank page before the stylesheet has said the page is dark.
Three things stop it, two of them Alder's and one the library's:

- **The `color-scheme` meta.** The router puts
  `<meta name="color-scheme" content="light dark" />` at the top of every
  page's `<head>` (chapter 6), so the browser knows before any CSS
  arrives that the page has a dark theme, and paints its first frame and
  its blank canvas dark when the operating system is dark.
- **Boosted links.** A click on a link of your app does not load a page
  at all: the new `<body>` is morphed into the old one, and the `<head>`
  and its stylesheet stay (chapter 6, "Links and navigation").
- **View transitions.** Since v0.1.3 the library's `tokens.css` has

  ```css
  @view-transition {
    navigation: auto;
  }
  ```

  which opts every page into *cross-document view transitions*: when the
  browser goes from one document of the site to another of the same
  origin (a link with `data-alder-reload`, a form that submits with
  `GET`, Back or Forward to a page that was loaded rather than boosted),
  it cross-fades from the old page to the new one. A first visit and a
  reload have no old page of the site to fade from, so they get no
  transition; for them the `color-scheme` meta is what keeps the first
  frame dark. (While a new document loads, the browser keeps showing the
  old one until the new one can paint; that is the browser's paint
  holding, which happens with or without the rule. The transition is the
  animation once it has painted.) Chrome and Edge (126 and later) and
  Safari (18.2 and later) support it; other browsers ignore the rule.

### Changing a token

To change a token, set it in your own stylesheet:

```css
:root {
  --primary: hsl(262 83% 58%);
  --radius: 0.25rem;
}
```

A token set on `:root` changes both themes: your rule beats the
library's theme rules (the next paragraph says why). To give the dark
theme its own value, set it after that, in both places the dark theme
comes from, the forced theme and the operating system's:

```css
[data-theme="dark"] { --primary: hsl(263 70% 70%); }
@media (prefers-color-scheme: dark) {
  :root:not([data-theme="light"]) { --primary: hsl(263 70% 70%); }
}
```

Your rules win without any `!important`, because the whole library sits
in a *cascade layer*: `al.css` wraps everything in `@layer al { … }`, and
CSS that is not in a layer always beats CSS that is, whatever the
selectors. The trade-off is that a rule of yours cannot lose to the
library by accident either: `button { padding: 0 }` in your stylesheet
also removes the padding of the button inside every `<al-button>`. Write
such rules with a narrower selector (`.toolbar button`), or put your own
CSS in a layer declared after it (`@layer al, app;`).

### Browser support

The library uses only widely available CSS: custom properties, cascade
layers, `color-mix()` and `:has()` (and `@view-transition`, above, which
a browser without it ignores). The last is the newest (Chrome and
Edge 105, Safari 15.4, Firefox 121, all from 2023). It is what lets
`<al-button>` hand its look to the `<button>` inside it; in an older
browser that button shows its own border inside the element's.

## The elements

There are twelve in the first tier and ten in the interactive tier ("The interactive tier", below). Each section below gives the element's attributes as
the manifest declares them (the book's tests compare every table with
the manifest the compiler embeds, so they cannot drift apart), its
slots, and an example: the Alder you write, with the HTML it renders as
the block's expected value.

Every element also takes the attributes any custom element takes: `id`,
`class`, `style`, `slot`, `hidden`, `title`, `lang`, `dir`, `tabindex`,
`role`, `data-*` and `aria-*`, and Alder's `click` and `href` (chapter 5,
"Custom elements"); `submit` belongs on `<form>` and is refused on an
element (E0028). A `boolean` attribute is written bare (`disabled`) or
with a `Bool` (`disabled={model.saving}`). An attribute whose type lists
quoted values takes one of them: a literal outside the list, such as
`variant="destuctive"`, is E0028, and the message lists the values. A
computed value (`variant={…}`) is not checked; an unknown one is simply
not styled.

A child with `slot="…"` must be the element's own child (through an
`if`, `for` or `match`, not inside a `<div>`), and name one of its
slots. The children you pass to a component of your own are not checked
against the element it puts them in: `<Ui.Card><p slot="aside">…</p></Ui.Card>`
renders the wrong slot quietly (chapter 5, "Slots").

### `al-button`

A button, in five variants and three sizes.

| Attribute | Type | Default | Meaning |
|---|---|---|---|
| `variant` | `"primary" \| "secondary" \| "outline" \| "ghost" \| "destructive"` | `"primary"` | the look: filled, soft, bordered, borderless until hovered, or red for deleting |
| `size` | `"sm" \| "md" \| "lg"` | `"md"` | the height and padding |
| `disabled` | `boolean` | `false` | dims the button; the button inside is disabled too |
| `type` | `"button" \| "submit"` | `"button"` | the kind of the `<button>` Alder renders inside |

Slots: default.

A custom element is not a button to the browser: it cannot take the
keyboard's focus, Enter does nothing on it, and a form does not submit
through it. So when you give an `<al-button>` something to do (`click`,
`type` or `href`), Alder renders a real control inside it: a `<button>`
for `click` and `type`, an `<a>` for `href`. The element keeps its other
attributes and gives the control its look:

```alder eval=<al-stack direction="row" gap="sm"><al-button type="submit"><button type="submit">Save</button></al-button><al-button variant="secondary" type="button"><button type="button">Secondary</button></al-button><al-button variant="outline"><a href="/">Cancel</a></al-button><al-button variant="ghost" type="button"><button type="button">Ghost</button></al-button><al-button variant="destructive" type="submit"><button type="submit">Delete</button></al-button></al-stack>
<al-stack direction="row" gap="sm">
  <al-button type="submit">Save</al-button>
  <al-button variant="secondary" type="button">Secondary</al-button>
  <al-button variant="outline" href={Todos.Index {}}>Cancel</al-button>
  <al-button variant="ghost" type="button">Ghost</al-button>
  <al-button variant="destructive" type="submit">Delete</al-button>
</al-stack>
```

The inner `<button>` gets `type="submit"` when you write
`type="submit"`, and `type="button"` otherwise, so a button with a
`click` inside a form does not submit the form.
`disabled` is copied onto the inner button, so the keyboard and the form
skip it, and stays on the element, which dims:

```alder eval=<al-stack direction="row" gap="sm"><al-button size="sm" type="button"><button type="button">Small</button></al-button><al-button size="md" type="button"><button type="button">Medium</button></al-button><al-button size="lg" type="button" disabled><button type="button" disabled>Large</button></al-button></al-stack>
<al-stack direction="row" gap="sm">
  <al-button size="sm" type="button">Small</al-button>
  <al-button size="md" type="button">Medium</al-button>
  <al-button size="lg" type="button" disabled>Large</al-button>
</al-stack>
```

A `click` is posted the way any click button's is (chapter 6, "With and
without JavaScript"): with no JavaScript, the button submits a little
form of its own. That form goes around the whole `<al-button>`, so the
button stays the element's child and keeps the look:

```alder eval=<form method="post" action="/_m/App/Logout" class="_alder-click" data-on:submit="window._alderAC?.abort();(window._alderBusy||((e,p)=&gt;p))(el, @post(&#39;/_m/App/Logout&#39;, {contentType: &#39;form&#39;, requestCancellation: window._alderAC = new AbortController()}))"><al-button variant="outline" size="sm"><button type="submit">Log out</button></al-button></form>
<al-button variant="outline" size="sm" click={App.Logout}>Log out</al-button>
```

`form._alder-click` is that form's class, and the stylesheet gives it
`display: contents`: the form takes no part in the layout, so a row of
such buttons stays a row, and a grid or flex parent places the button.
A `form` rule of your own should leave it alone:
`form:not(._alder-click) { display: grid }`, as the stylesheet
`alder new` writes does.

Because the element renders its own control, what goes inside it is
text and inline markup only. A control inside (a `<button>`, `<a>`,
`<input>`, `<select>`, `<textarea>`, or any element with `click`),
written directly or through an `if`, `for` or `match`, would be a
button inside a button, and both would answer the click:

```alder error=E0028 "`<al-button>` renders its own `<button>` for `click`; move `click` onto your `<button>`, or drop the child"
<al-button click={App.Logout}><button>Log out</button></al-button>
```

An `Html` value or a component inside is refused for the same reason,
since it may hold a control (`Html.none` is fine, and so is a `Text`
expression): "`<al-button>` with `click` takes text and inline markup
only; an `Html` value may contain a control — write the `<button>`
yourself". And an `<al-button>` takes `href` or `click`, not both: one
would navigate and the other post.

A `disabled` link is not a link: `<al-button href={…} disabled>` renders
`<a aria-disabled="true">` with no `href`, so it goes nowhere and a
screen reader says it is unavailable (with `disabled={expr}`, the
renderer picks one of the two when the page is rendered).

An `<al-button>` with nothing to do renders as written. It looks like a
button and is not one, which suits a label inside something that is
clickable already:

```alder eval=<al-button variant="ghost">Looks like a button</al-button>
<al-button variant="ghost">Looks like a button</al-button>
```

If you write the `<button>` or `<a>` yourself, with its `click` or
`href` on it and none on the element, Alder leaves the element alone:

```alder eval=<al-button variant="outline"><a href="/?filter=Active">Open todos</a></al-button>
<al-button variant="outline"><a href={Todos.Index { filter: Active }}>Open todos</a></al-button>
```

### `al-input`

A text field: the label, the `<input>`, a help line and the field's
errors.

| Attribute | Type | Default | Meaning |
|---|---|---|---|
| `label` | `string` | — | the label text |
| `placeholder` | `string` | — | the input's placeholder, copied onto it |
| `disabled` | `boolean` | `false` | disables the input and dims the field |
| `invalid` | `boolean` | `false` | colours the border and focus ring as an error; Alder sets it |
| `help` | `string` | — | a hint under the input, rendered as `<small class="help">` |

Slots: none.

Inside a `<form submit>`, or given `field`, the four input elements are
Alder's bound inputs (chapter 5, "The input elements"): `field`, `label`
and `value` mean what they mean on `<Input>`, and Alder renders the
control inside the element. They take no children. This page uses all
four:

```alder
module Settings

page "/settings" { }

type Model = { }

load : Params -> Session -> Query Model
load params session = Query.succeed { }

type Plan = Free | Pro | Team

form Profile = {
  name: Text.NonEmpty,
  bio: Text,
  plan: Plan,
  newsletter: Bool,
}

msg Save form Profile

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Save p -> Task.succeed Rerender,
}

view : Model -> Session -> Html Msg
view model session =
  <form submit={Save}>
    <al-input field="name" label="Name" placeholder="Ada Lovelace" help="As it appears on your posts" />
    <al-textarea field="bio" label="Bio" placeholder="A line or two" />
    <al-select field="plan" label="Plan" />
    <al-checkbox field="newsletter" label="Send me the newsletter" />
    <al-button type="submit">Save</al-button>
  </form>
```

The name field renders as:

```html
<al-input label="Name" placeholder="Ada Lovelace" help="As it appears on your posts"><label>Name<input name="name" type="text" required placeholder="Ada Lovelace" aria-describedby="help-name" /></label><small class="help" id="help-name">As it appears on your posts</small><ul class="errors" data-errors="name"></ul></al-input>
```

The `<label>` holds the text and the input, so clicking the text focuses
the input, and a screen reader names the input by it. The `label`
attribute repeats the text for other tools; the stylesheet does not draw
it. The `help` text is a real element, `<small class="help"
id="help-<field>">`, after the label, and the input's `aria-describedby`
names it, so a screen reader reads the hint with the field (text drawn
by CSS alone would not be read). It is there only when you write `help`;
the stylesheet then does not draw the attribute's text a second time.
The `<ul class="errors">` is the field's error list, empty until a
submission fails. When it does, the page is rendered again with the
submitted value and the messages:

```html
<al-input label="Name" placeholder="Ada Lovelace" help="As it appears on your posts" invalid><label>Name<input name="name" type="text" required placeholder="Ada Lovelace" aria-invalid="true" aria-describedby="help-name errors-name" /></label><small class="help" id="help-name">As it appears on your posts</small><ul class="errors" data-errors="name" id="errors-name"><li>must not be empty</li></ul></al-input>
```

Three things changed. The element has `invalid`, which the stylesheet
uses to colour the field. The input has `aria-invalid="true"`, which
tells a screen reader the value was refused. And the list has an `id`,
`errors-<field>`, which the input's `aria-describedby` now names after
the help, so the screen reader reads the messages along with the field.
(The `id` is there only while the list has messages: a page with one
form per row, such as a list of todos, repeats each field name, and an
`id` must be unique.) An `id` or `aria-*` attribute you write on the
element stays on the element; it is not forwarded to the input, whose
ids are derived from the field's name as above. Since the element shows
the field's errors, an `<Errors field="name" />` beside it is refused
(E0028). The `ul.errors[data-errors=…]` list is the same one
`<Errors field>` renders, so tests that look for it (chapter 10) find it
either way.

### `al-textarea`

A multi-line field, rendered like `al-input` around a `<textarea>`.

| Attribute | Type | Default | Meaning |
|---|---|---|---|
| `label` | `string` | — | the label text |
| `placeholder` | `string` | — | the textarea's placeholder, copied onto it |
| `disabled` | `boolean` | `false` | disables the textarea and dims the field |
| `invalid` | `boolean` | `false` | colours the border and focus ring as an error; Alder sets it |
| `help` | `string` | — | a hint under the textarea, rendered as `<small class="help">` |

Slots: none.

From the page above:

```html
<al-textarea label="Bio" placeholder="A line or two"><label>Bio<textarea name="bio" placeholder="A line or two"></textarea></label><ul class="errors" data-errors="bio"></ul></al-textarea>
```

### `al-select`

A select, on a field whose type is a union of constructors without
arguments (chapter 5, "Forms and inputs"): one option per constructor.
On a field of `Text` or a brand of it, `options={…}` (a `List { value:
…, label: Text }`) gives the options instead: a row's id and its label.
A submitted value that is not one of them is refused with the field
error "must be one of the options": the runtime computes the options
again from the model when the form is posted (chapter 5).

| Attribute | Type | Default | Meaning |
|---|---|---|---|
| `label` | `string` | — | the label text |
| `disabled` | `boolean` | `false` | disables the select and dims the field |
| `invalid` | `boolean` | `false` | colours the border and focus ring as an error; Alder sets it |
| `help` | `string` | — | a hint under the select, rendered as `<small class="help">` |

Slots: none.

```html
<al-select label="Plan"><label>Plan<select name="plan" required><option value="Free">Free</option><option value="Pro">Pro</option><option value="Team">Team</option></select></label><ul class="errors" data-errors="plan"></ul></al-select>
```

### `al-checkbox`

A checkbox, on a `Bool` field. The box is drawn before its text.

| Attribute | Type | Default | Meaning |
|---|---|---|---|
| `label` | `string` | — | the label text |
| `disabled` | `boolean` | `false` | disables the checkbox and dims it |
| `invalid` | `boolean` | `false` | outlines the box as an error; Alder sets it |
| `help` | `string` | — | a hint under the checkbox, rendered as `<small class="help">` |

Slots: none.

```html
<al-checkbox label="Send me the newsletter"><label>Send me the newsletter<input name="newsletter" type="checkbox" value="True" /></label><ul class="errors" data-errors="newsletter"></ul></al-checkbox>
```

### `al-table`

Styles the `<table>` you put inside it: header cells, borders, padding.
A wide table scrolls sideways inside the element instead of widening the
page.

| Attribute | Type | Default | Meaning |
|---|---|---|---|
| `striped` | `boolean` | `false` | shades every other body row |
| `dense` | `boolean` | `false` | tightens the cell padding |

Slots: default (the `<table>`).

```alder eval=<al-table striped><table><thead><tr><th>Title</th><th>Status</th><th>Comments</th></tr></thead><tbody><tr><td>Hello, world</td><td><al-badge>Published</al-badge></td><td>3</td></tr><tr><td>Drafts are fine</td><td><al-badge variant="secondary">Draft</al-badge></td><td>0</td></tr></tbody></table></al-table>
<al-table striped>
  <table>
    <thead><tr><th>Title</th><th>Status</th><th>Comments</th></tr></thead>
    <tbody>
      <tr><td>Hello, world</td><td><al-badge>Published</al-badge></td><td>3</td></tr>
      <tr><td>Drafts are fine</td><td><al-badge variant="secondary">Draft</al-badge></td><td>0</td></tr>
    </tbody>
  </table>
</al-table>
```

### `al-card`

A bordered surface. Its children are placed by slot, whatever order you
write them in: the header, the body (children without a slot), the
footer, then the actions in a row at the end.

No attributes.

Slots: `header`, default, `footer`, `actions`.

```alder eval=<al-card><h3 slot="header">Hello, world</h3><p>The first post. The body is every child without a slot.</p><p slot="footer">Posted on 12 October 2026</p><div slot="actions"><al-button variant="outline"><a href="/">Back</a></al-button><al-button variant="destructive" type="submit"><button type="submit">Delete</button></al-button></div></al-card>
<al-card>
  <h3 slot="header">Hello, world</h3>
  <p>The first post. The body is every child without a slot.</p>
  <p slot="footer">Posted on 12 October 2026</p>
  <div slot="actions">
    <al-button variant="outline" href={Todos.Index {}}>Back</al-button>
    <al-button variant="destructive" type="submit">Delete</al-button>
  </div>
</al-card>
```

### `al-badge`

A small pill for a status or a count.

| Attribute | Type | Default | Meaning |
|---|---|---|---|
| `variant` | `"default" \| "secondary" \| "outline" \| "destructive"` | `"default"` | the look: filled, soft, bordered or red |

Slots: default.

```alder eval=<al-stack direction="row" gap="sm"><al-badge>Published</al-badge><al-badge variant="secondary">Draft</al-badge><al-badge variant="outline">Archived</al-badge><al-badge variant="destructive">Failed</al-badge></al-stack>
<al-stack direction="row" gap="sm">
  <al-badge>Published</al-badge>
  <al-badge variant="secondary">Draft</al-badge>
  <al-badge variant="outline">Archived</al-badge>
  <al-badge variant="destructive">Failed</al-badge>
</al-stack>
```

### `al-alert`

A message box: information, success, a warning or an error.

| Attribute | Type | Default | Meaning |
|---|---|---|---|
| `variant` | `"info" \| "success" \| "warning" \| "destructive"` | `"info"` | the kind, which sets the colours |
| `heading` | `string` | — | a heading drawn in bold above the message |
| `dismissible` | `boolean` | `false` | makes room for a dismiss button (see below) |

Slots: default.

An alert should be announced by a screen reader when it appears, which
is what the ARIA roles `status` (polite: after what is being read) and
`alert` (at once) are for. Alder adds the role from the variant:
`role="status"` for `info` (the default) and `success`, `role="alert"`
for `warning` and `destructive`:

```alder eval=<al-stack gap="sm"><al-alert heading="Heads up" role="status">Comments are moderated before they appear.</al-alert><al-alert variant="success" heading="Saved" role="status">Your post is live.</al-alert><al-alert variant="warning" heading="Unsaved changes" role="alert">Leaving this page discards your draft.</al-alert><al-alert variant="destructive" heading="Could not save" role="alert">The title must not be empty.</al-alert></al-stack>
<al-stack gap="sm">
  <al-alert heading="Heads up">Comments are moderated before they appear.</al-alert>
  <al-alert variant="success" heading="Saved">Your post is live.</al-alert>
  <al-alert variant="warning" heading="Unsaved changes">Leaving this page discards your draft.</al-alert>
  <al-alert variant="destructive" heading="Could not save">The title must not be empty.</al-alert>
</al-stack>
```

A `role` you write is kept, and so is its absence when the variant is
computed (`variant={…}`): then the renderer cannot know it, and you
write the `role` yourself.

An alert that holds nothing but an empty error list is hidden by the
stylesheet (`al-alert:has(> ul.errors:empty) { display: none }`), so
`<al-alert variant="destructive"><Errors form /></al-alert>` shows a red
box only after a failed submission (the blog's forms do this, chapter
14).

`dismissible` only makes room: the button that closes the alert comes
from the library's optional script, `components/al-alert.js`, which
Alder does not ship. Datastar's morph would remove a button a script
added, so in an Alder page render your own: an `<al-button>` with a
`click`. The model is not kept between requests (`load` builds it again
for every render), so the message must record the dismissal where `load`
will find it, in the session or in the database, and answer `Rerender`;
`load` then leaves the alert out.

### `al-pagination`

Lays out the links of a paged list: the previous link, a middle, the
next link. The app renders the links; the element places them.

| Attribute | Type | Default | Meaning |
|---|---|---|---|
| `page` | `number` | — | the current page, from 1 |
| `pages` | `number` | — | the number of pages |

Slots: `prev`, default, `next`.

With `page` and `pages`, the element draws "Page 1 of 5" between the
links. On the first or last page, a `<span aria-disabled="true">` in the
slot keeps the layout and is drawn dimmed:

```alder eval=<al-pagination page="1" pages="5" role="navigation" aria-label="Pages"><span slot="prev" aria-disabled="true">Previous</span><a slot="next" href="/?filter=Active">Next</a></al-pagination>
<al-pagination page="1" pages="5" role="navigation" aria-label="Pages">
  <span slot="prev" aria-disabled="true">Previous</span>
  <a slot="next" href={Todos.Index { filter: Active }}>Next</a>
</al-pagination>
```

Children without a slot go in the middle instead of the drawn text, for
a different wording or a language other than English (the blog writes
its own "Page N", chapter 14):

```alder eval=<al-pagination role="navigation" aria-label="Pages"><a slot="prev" href="/">Previous</a><span>Showing 11 to 20 of 48</span><a slot="next" href="/?filter=Completed">Next</a></al-pagination>
<al-pagination role="navigation" aria-label="Pages">
  <a slot="prev" href={Todos.Index {}}>Previous</a>
  <span>Showing 11 to 20 of 48</span>
  <a slot="next" href={Todos.Index { filter: Completed }}>Next</a>
</al-pagination>
```

`page` and `pages` are numbers in the manifest, but HTML attributes are
text, so you write them as text: `page={Text.fromInt model.page}`.

### `al-stack`

Lays its children out in a column or a wrapping row, with a gap.

| Attribute | Type | Default | Meaning |
|---|---|---|---|
| `gap` | `"sm" \| "md" \| "lg"` | `"md"` | the space between children |
| `direction` | `"row" \| "column"` | `"column"` | top to bottom, or left to right (wrapping) |

Slots: default.

```alder eval=<al-stack direction="row" gap="lg"><al-badge variant="outline">One</al-badge><al-badge variant="outline">Two</al-badge><al-badge variant="outline">Three</al-badge></al-stack>
<al-stack direction="row" gap="lg">
  <al-badge variant="outline">One</al-badge>
  <al-badge variant="outline">Two</al-badge>
  <al-badge variant="outline">Three</al-badge>
</al-stack>
```

### `al-page`

The page's frame: a centred column with a maximum width and some
padding. The layouts of `alder new` and both examples put `<main>`
inside it.

| Attribute | Type | Default | Meaning |
|---|---|---|---|
| `width` | `"narrow" \| "normal" \| "wide"` | `"normal"` | at most 40rem, 56rem or 80rem |

Slots: default.

```alder eval=<al-page width="narrow"><al-card><p>At most 40rem wide, centred.</p></al-card></al-page>
<al-page width="narrow">
  <al-card><p>At most 40rem wide, centred.</p></al-card>
</al-page>
```

## The interactive tier

The twelve elements above are a stylesheet. The ten below (`al-dialog`,
`al-popover`, `al-menu`, `al-tabs`, `al-tab`, `al-panel`, `al-toast`,
`al-toasts`, `al-tooltip`, `al-devpanel`) are what v0.2 added, and they
behave: they open, close, move focus and time out. The rule that shaped
them is that **the platform does the work wherever it can**. A modal
dialog is the browser's `<dialog>`, which already traps focus, makes the
page behind it inert, closes on Escape and gives focus back to the button
that opened it. A popover is the browser's Popover API, positioned with
CSS anchor positioning. A group of tabs is a group of radio buttons. A
script (`al.js`, below) is added only where the platform has nothing: a
toast's timer, the arrow keys of a menu, a polyfill for older browsers.

Like the first tier they are light DOM, use only tokens, and are plain
elements in the page, so Datastar's morph sees every part of them.

### Invoker commands

Most of these elements are opened by a button, and the button names what
to open and what to do with two attributes on any `<button>`:

```html
<button type="button" commandfor="help" command="show-modal">Help</button>
<al-dialog id="help" heading="Help">…</al-dialog>
```

`commandfor` is the `id` of the element to act on and `command` is the
action: `show-modal` (open a dialog as a modal), `close` (close it),
`toggle-popover`, `show-popover` or `hide-popover` (for a popover). These
are the platform's *invoker commands*; there is no `click` handler and no
script of yours. In an Alder view write the `<button>` inside an
`<al-button>` (the button gets the element's look, as in `al-button`
above); `commandfor` and `command` are plain attributes of the `<button>`.
A button that only toggles a popover can also use the older pair,
`popovertarget="id"`, which every browser with popovers understands.

### Without JavaScript, and in older browsers

Each element says below what it does with no script, but the rule is
short. Dialog, popover, menu and tabs work natively: invoker commands,
popovers and dialogs are the browser's own, so in the browsers below a
`commandfor` button opens its dialog or popover with JavaScript off. A tooltip is hidden,
so put the same text in the trigger's `title`. A toast falls back to an
`<al-alert>` rendered in the page. The rest of this section is the browser
line.

The full set needs invoker commands, `popover="hint"` and CSS anchor
positioning: Chromium 135 and later (Chrome, Edge), Safari 26, Firefox
140. `al.js` includes a shim (`al-shim.js`, 26 lines) that adds invoker
commands to a browser without them (so `commandfor` / `command` open and
close a dialog or popover on older versions), and it makes a tooltip a
manual popover where `hint` is unknown. It does **not** add anchor
positioning: without it a popover, menu or tooltip shows at the browser's
default position (still visible, still usable, just not beside its
button), and a popover that would leave the screen is not flipped. The
elements' `al-close` and `al-select` events (below) come from the same
script.

### The script

`al.js` is one file of small `type="module"` scripts (each under 60
lines, no framework): the shim, the menu's keys, the tabs' keys and the
developer bar's tab switch, the toast timer and the tooltip. Link it once
in the layout, next to the stylesheet:

```html
<script type="module" src="/static/al/al.js"></script>
```

Alder writes it into the build next to `al.css` and serves it at
`/static/al/al.js`, with the same `ETag` and cache rules. It is optional:
every section below says what is lost without it. `alder new`'s layout
and both example apps link it. Write the `src` as a plain string, as
above; `Url.external` is for the `href` of a link, and a `<script src>` is
typed `Text`.

### `al-dialog`

A modal dialog around a native `<dialog>`: the browser traps focus, makes
the rest of the page inert, closes it on Escape, and returns focus to the
button that opened it. Open it with a button whose `commandfor` is the
dialog's `id` and whose `command` is `show-modal`; close it with a
`<button commandfor="id" command="close">` in the `actions` slot, with
the close button `dismissible` adds, with Escape, or by a click on the
backdrop.

| Attribute | Type | Default | Meaning |
|---|---|---|---|
| `heading` | `string` | — | the dialog's heading, rendered as the header slot's `<h2>`; the attribute itself draws nothing |
| `size` | `"sm" \| "md" \| "lg"` | `"md"` | the width |
| `dismissible` | `boolean` | `false` | makes room for a close button: a `<form method="dialog">` holding a `<button>` as the first child of the `<dialog>` |

Slots: `header`, default, `actions`.

Event: `al-close`, fired (it bubbles) on the `<al-dialog>` when its
dialog closes, however it closed.

```html
<al-button variant="destructive"><button type="button" commandfor="delete-dialog" command="show-modal">Delete</button></al-button>
<al-dialog id="delete-dialog" heading="Delete this comment?" dismissible>
  <p>Nothing else refers to it.</p>
  <form method="post" action="/comments/3/delete">
    <al-button variant="destructive"><button type="submit">Delete</button></al-button>
  </form>
  <div slot="actions"><a href="/comments/3/delete">See the details</a></div>
</al-dialog>
```

You write the element and its content; Alder's renderer writes the
native `<dialog>` the library's stylesheet and the invoker commands act
on. The element's `id` moves to the `<dialog>` (so `commandfor="delete-dialog"`
finds it), a `heading` becomes the header slot's `<h2>` (unless a child
already has `slot="header"`) and names the dialog for a screen reader
(`aria-labelledby` points at the `<h2>`, whose `id` is the dialog's plus
`-heading`), and a `dismissible` dialog gets a close button, labelled
"Close" and the heading, in a `<form method="dialog">` first, and closes
on a click on the backdrop (`closedby="any"`). `dismissible={False}`
adds neither:

```alder eval=<al-dialog heading="Sure?" dismissible><dialog id="sure" aria-labelledby="sure-heading" data-preserve-attr="open" closedby="any"><form method="dialog"><button type="submit" aria-label="Close Sure?">×</button></form><h2 slot="header" id="sure-heading">Sure?</h2><p>This cannot be undone.</p><div slot="actions"><al-button>Cancel</al-button></div></dialog></al-dialog>
<al-dialog id="sure" heading="Sure?" dismissible>
  <p>This cannot be undone.</p>
  <div slot="actions"><al-button>Cancel</al-button></div>
</al-dialog>
```

Either form works: if you write the native `<dialog>` yourself, as the
`<al-dialog>`'s direct child (as you would from another stack), the
renderer leaves the element exactly as written and adds nothing, and the
`slot`s inside your `<dialog>` are the `<al-dialog>`'s:

```alder eval=<al-dialog size="sm"><dialog id="own" closedby="any" aria-labelledby="own-title"><h2 slot="header" id="own-title">Your own</h2><p>Written by hand.</p></dialog></al-dialog>
<al-dialog size="sm">
  <dialog id="own" closedby="any" aria-labelledby="own-title">
    <h2 slot="header" id="own-title">Your own</h2>
    <p>Written by hand.</p>
  </dialog>
</al-dialog>
```

Datastar's morph keeps a dialog open. The morph would remove the `open`
attribute the browser wrote when the dialog was shown (the server never
writes it), so the renderer also writes `data-preserve-attr="open"` on the
`<dialog>`, which tells Datastar to leave that attribute alone. A body
update while the dialog is on screen then leaves it open and modal with
focus inside it, as long as the element keeps its `id`. A form in the
dialog posts a message of the page the dialog is on: messages belong to
the page that declares them (chapter 6), so the admin's Show page declares
its own `Remove`, and its dialog shows the counts the confirmation page
shows and posts it (chapter 15).

Without JavaScript the button still opens the dialog, as a modal, in the
browsers this tier supports (Chromium 135 and later, Safari 26, Firefox
140): invoker commands are the browser's, not the script's. Only an older
browser with the script off does nothing on the button; for it, a link to
`href="#delete-dialog"` shows the dialog (not as a modal: it is shown by
`:target`, and `#` hides it), or link to a page that does the same job, as
the admin's "See the details" does.

### `al-popover`

A small floating panel anchored to the button that opens it. Write
`popover` on it and open it with `popovertarget="id"` or `commandfor="id"
command="toggle-popover"` on a button. The panel is placed with CSS
anchor positioning and flips to the opposite side when it would leave the
screen.

| Attribute | Type | Default | Meaning |
|---|---|---|---|
| `placement` | `"top" \| "bottom" \| "left" \| "right"` | `"bottom"` | the side of the anchor the panel opens on |
| `offset` | `"sm" \| "md" \| "lg"` | `"sm"` | the gap between the anchor and the panel, a space token |
| `anchor` | `string` | — | the id of the element to anchor to, when it is not the invoker (the platform's own `anchor` attribute) |

Slots: default.

Event: `al-close`, fired on the `<al-popover>` when it closes.

```html
<al-button variant="outline"><button type="button" commandfor="who" command="toggle-popover">Who can see this?</button></al-button>
<al-popover id="who" popover placement="bottom">Only people with the link.</al-popover>
```

It needs no script: the popover opens, closes on Escape or a click
outside, and light-dismisses, with JavaScript off. In a browser without
anchor positioning it appears at the default position.

In an Alder view it is written exactly so: `popover`, `popovertarget`,
`popovertargetaction`, `commandfor`, `command` and `closedby` are global
HTML attributes, which the checker accepts on any element (chapter 5), and
a bare `popover` is `popover="auto"`:

```alder eval=<al-popover id="who" popover placement="bottom">Only people with the link.</al-popover>
<al-popover id="who" popover placement="bottom">Only people with the link.</al-popover>
```

### `al-menu`

A popover of links or buttons. Open it with `<button commandfor="id"
command="toggle-popover">` and give the `<al-menu>` the `popover`
attribute and the same `id`.

No attributes.

Slots: default.

The items are `<a>` or `<button>` children.

Event: `al-select`, fired on the `<al-menu>` when an item is chosen
(`detail.item` is the item), and `al-close`.

```html
<al-button variant="outline"><button type="button" commandfor="more" command="toggle-popover">More</button></al-button>
<al-menu id="more" popover>
  <a href="/posts/new">New post</a>
  <a href="/posts/drafts">Drafts</a>
  <button type="button">Sign out</button>
</al-menu>
```

In an Alder view the links' `href`s are pages (`href={Posts.New {}}`), and
the rest is written as above.

With the script the menu gets `role="menu"` and `role="menuitem"` on the
items, the arrow keys move between them, Home and End go to the first and
last, the first item takes focus on open, and choosing one closes the
menu. Without it the menu is a popover of links, which still navigate.

### `al-tabs`

Tabs backed by a radio group, so selecting a tab and the arrow keys work
with no script. Its children are `<al-tab>`s.

No attributes.

Slots: default.

Event: `al-select`, fired on the `<al-tabs>` when a tab is chosen
(`detail.tab` is the `<al-tab>`).

```html
<al-tabs>
  <al-tab name="view" selected>
    <label><input type="radio" name="view" checked> Details</label>
    <al-panel>The post's fields.</al-panel>
  </al-tab>
  <al-tab name="view">
    <label><input type="radio" name="view"> Comments</label>
    <al-panel>Its comments.</al-panel>
  </al-tab>
</al-tabs>
```

Each tab holds a `<label>` that wraps a radio input and then its panel,
and the radios of one group share a `name`. This nesting (the panel inside
its tab, rather than beside it with a `for`) is what lets a stylesheet show
the panel of the checked radio with `:has()`, with no script.

Selecting a tab shows its panel and hides the others, and the arrow
keys move between the radios. `al-tabs.js` adds Home and End, picks the
tab whose `<al-tab>` has `selected` (the server's choice), copies the
tab's `name` onto a radio without one, and fires `al-select`. Without
the script, mark the open tab with `checked` on its radio.

**The ARIA state as it is.** The script gives each panel `role="tabpanel"`
but nothing gives the labels `role="tab"` or the group `role="tablist"`;
the interaction is a radio group, which a screen reader announces as one,
and that is what is exposed today. A full tabs pattern (with
`aria-selected` and `aria-controls`) is not there yet, and the manifest
does not promise it. If your tabs must follow it, render the buttons and
the roles yourself.

### `al-tab`

One tab: a `<label>` wrapping a radio input, then its `<al-panel>`.

| Attribute | Type | Default | Meaning |
|---|---|---|---|
| `name` | `string` | — | the radio group's name; `al-tabs.js` copies it onto a radio that has none |
| `selected` | `boolean` | `false` | the tab the server picked; the script checks its radio (without the script, write `checked` on the radio) |

Slots: default.

The label comes first, then the panel.

### `al-panel`

A tab's content, shown when its tab is selected.

No attributes.

Slots: default.

### `al-toast`

A transient message, shown in an `<al-toasts>` region. The script shows the
region when a toast connects, gives the toast `role="status"` (so a
screen reader announces it) and closes it after `timeout` milliseconds.

| Attribute | Type | Default | Meaning |
|---|---|---|---|
| `variant` | `"info" \| "success" \| "warning" \| "destructive"` | `"info"` | the kind, which sets the colours, as in `al-alert` |
| `timeout` | `number` | `5000` | milliseconds before it closes; `0` keeps it until it is closed |

Slots: default (the message).

```html
<al-toasts popover="manual" position="bottom-right">
  <al-toast variant="success" timeout="5000">Saved</al-toast>
</al-toasts>
```

You rarely write one. A message that answers `Notice.with "Saved"
Rerender` (chapter 6, "Notices and in-flight") makes Alder's runtime
patch an `<al-toast variant="info" timeout="5000">` into the page's
`<al-toasts>`, after the body update, so the toast survives the morph; the
scaffold's guestbook does this for "Signed".

Without JavaScript the region stays hidden, so a toast is never seen.
Alder covers the `Notice` answer itself: a request made without the
script (a form posted with JavaScript off) gets its notice as an
`<al-alert variant="info" role="status">` at the top of the page that
answers it, the page itself on a `Rerender`, the next page on a
`Redirect` (the text rides a signed cookie, `alder_notice`, for that one
page).

### `al-toasts`

The region toasts appear in: a `popover="manual"` element in the top
layer at a corner of the viewport. Alder's renderer adds
`popover="manual"` when you leave it out, as below. Put the region
outside any part a message re-renders (Alder adds one to the body update
itself, marked `data-ignore-morph`, when the page has none).

```alder eval=<al-toasts position="top-right" popover="manual"></al-toasts>
<al-toasts position="top-right"></al-toasts>
```

| Attribute | Type | Default | Meaning |
|---|---|---|---|
| `position` | `"top-left" \| "top-right" \| "bottom-left" \| "bottom-right"` | `"bottom-right"` | the viewport corner |

Slots: default.

Event: `al-close`, fired on the region when a toast closes
(`detail.toast` is the toast).

### `al-tooltip`

A short text hint for the element whose id is in `for`. With the script
it is a `popover="hint"` (a manual popover where `hint` is unknown),
anchored to the trigger, shown on hover and on keyboard focus, hidden by
Escape, and wired to the trigger with `aria-describedby`.

| Attribute | Type | Default | Meaning |
|---|---|---|---|
| `for` | `string` | — | the id of the element the tooltip describes |

Slots: default.

```html
<al-button variant="ghost"><button type="button" id="save-help" title="Saves a draft">?</button></al-button>
<al-tooltip for="save-help">Saves a draft</al-tooltip>
```

Without JavaScript the tooltip is hidden (it is shown only while it is a
popover that is open), which is why the example repeats the text in the
trigger's `title`: that is the version a visitor with the script off gets.

### `al-devpanel`

The developer bar: a one-line bar sticky along the bottom of the page,
and with `open` the selected section above it. It is the element
Alder's developer mode renders (chapter 11, "Developer mode"), and it is
in the library so any stack can reuse it. Its content is server-rendered:
a `<nav>` of tab `<button>`s, each with `aria-controls` naming a
`<section>`'s id, then the sections. SQL and code are set in monospace.

| Attribute | Type | Default | Meaning |
|---|---|---|---|
| `open` | `boolean` | `false` | shows the sections above the bar |

Slots: default.

The content is a `<nav>`, then the `<section>`s.

`al-tabs.js` switches the sections when a tab button is clicked and
toggles `open` (clicking the selected tab again closes the panel). Alder
renders every section but the first `hidden`, so without the script the
open panel shows the Request section. Alder emits the panel only under
`ALDER_DEV=1`, and never without it.

## From another stack

alder-ui is plain CSS, so any stack that writes HTML can use it: link
`al.css` and write the elements. The one thing Alder does for you that
you then do yourself is the native control inside `<al-button>`: write
`<al-button variant="primary"><button type="submit">Save</button></al-button>`
(or an `<a>` inside), and the button takes the element's look and works
without JavaScript. The input elements expect the markup this appendix
shows, a `<label>` around the control, then an optional
`<small class="help">` (name its `id` in the control's
`aria-describedby`) and an optional `<ul class="errors">`; a
`<label for>` beside the control works too. Two rules are there for
Alder's markup and match nothing else: `form._alder-click` (the click
form, `display: contents`) and the empty-errors alert above.

## Where it lives

alder-ui is its own project, at `github.com/tapegram/alder-ui`, under
the Apache-2.0 licence. Its repository holds the token and component
sources, the bundled `al.css`, the manifest, a demo page per element,
and the tests that check them: every element and attribute in the CSS is
in the manifest and the reverse, no component hard-codes a colour, and
every colour pairing passes AA.

Alder *vendors* it: the compiler's source keeps a copy of one tagged
release in `rt/al/` (`al.css`, `custom-elements.json` and `VERSION`),
and embeds it, so a build never fetches anything. `scripts/vendor-al.sh
<tag>` refreshes the copy from the library's repository; a test checks
that `VERSION`, the manifest's `version` and the stylesheet's banner
name the same release, and another compares this appendix's tables with
the manifest. Chapter 11 shows the files; chapter 13 shows how the
checker reads the manifest and how the renderer completes `al-button`,
the input elements and `al-alert`.
