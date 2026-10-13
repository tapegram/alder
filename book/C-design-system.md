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
- **CSS only.** This first tier is a stylesheet and nothing else: the
  elements are not defined in JavaScript at all. A browser shows an
  element it has no definition for as a plain box, and the stylesheet
  styles that box by its tag name and attributes. Every element works
  with JavaScript off.

The version Alder ships is **v0.1.2**: the compiler embeds a copy of
the library's stylesheet and manifest at that tag (`rt/al/VERSION`;
"Where it lives", below).

## Linking it

Every build writes the library's two files into the build's static
directory, next to the copy of your app's `static/`: `al/al.css` (the
stylesheet) and `al/custom-elements.json` (the manifest, below). So the
server `alder build` makes, and the one `alder run` starts, serve them at
`/static/al/al.css` and `/static/al/custom-elements.json`. You do not
copy them into your `static/` directory; Alder writes them, from the
copy inside the compiler (a file of yours at `static/al/al.css` would be
replaced). Both are served with a long-lived, immutable cache header,
like Datastar's script: they change only when the compiler does.

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
layers, `color-mix()` and `:has()`. The last is the newest (Chrome and
Edge 105, Safari 15.4, Firefox 121, all from 2023). It is what lets
`<al-button>` hand its look to the `<button>` inside it; in an older
browser that button shows its own border inside the element's.

## The elements

There are twelve. Each section below gives the element's attributes as
the manifest declares them (the book's tests compare every table with
the manifest the compiler embeds, so they cannot drift apart), its
slots, and an example: the Alder you write, with the HTML it renders as
the block's expected value.

Every element also takes the attributes any custom element takes: `id`,
`class`, `style`, `slot`, `hidden`, `title`, `lang`, `dir`, `tabindex`,
`role`, `data-*` and `aria-*`, and Alder's `click`, `submit` and `href`
(chapter 5, "Custom elements"). A `boolean` attribute is written bare
(`disabled`) or with a `Bool` (`disabled={model.saving}`). An attribute
whose type lists quoted values takes one of them; the checker accepts
any `Text` there, and an unknown value is simply not styled.

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
`submit`, `type` or `href`), Alder renders a real control inside it: a
`<button>` for `click`, `submit` and `type`, an `<a>` for `href`. The
element keeps its other attributes and gives the control its look:

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
`type="submit"` (or `submit`), and `type="button"` otherwise, so a
button with a `click` inside a form does not submit the form.
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

```alder eval=<form method="post" action="/_m/App/Logout" class="_alder-click" data-on:submit="@post(&#39;/_m/App/Logout&#39;, {contentType: &#39;form&#39;})"><al-button variant="outline" size="sm"><button type="submit">Log out</button></al-button></form>
<al-button variant="outline" size="sm" click={App.Logout}>Log out</al-button>
```

(The form is a block, so give it `form._alder-click { display: inline }`
in your stylesheet to keep such buttons in a line, as the examples do.)

An `<al-button>` with nothing to do renders as written. It looks like a
button and is not one, which suits a label inside something that is
clickable already:

```alder eval=<al-button variant="ghost">Looks like a button</al-button>
<al-button variant="ghost">Looks like a button</al-button>
```

If you write the `<button>` or `<a>` yourself, Alder leaves the element
alone:

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
| `help` | `string` | — | a hint drawn under the input |

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
<al-input label="Name" placeholder="Ada Lovelace" help="As it appears on your posts"><label>Name<input name="name" type="text" required placeholder="Ada Lovelace" /></label><ul class="errors" data-errors="name"></ul></al-input>
```

The `<label>` holds the text and the input, so clicking the text focuses
the input, and a screen reader names the input by it. The `label`
attribute repeats the text for other tools; the stylesheet does not draw
it. The `help` text is drawn from the attribute, under the input. The
`<ul class="errors">` is the field's error list, empty until a
submission fails. When it does, the page is rendered again with the
submitted value and the messages:

```html
<al-input label="Name" placeholder="Ada Lovelace" help="As it appears on your posts" invalid><label>Name<input name="name" type="text" required placeholder="Ada Lovelace" aria-invalid="true" aria-describedby="errors-name" /></label><ul class="errors" data-errors="name" id="errors-name"><li>must not be empty</li></ul></al-input>
```

Three things changed. The element has `invalid`, which the stylesheet
uses to colour the field. The input has `aria-invalid="true"`, which
tells a screen reader the value was refused. And the list has an `id`,
`errors-<field>`, which the input's `aria-describedby` names, so the
screen reader reads the messages along with the field. (The `id` is
there only while the list has messages: a page with one form per row,
such as a list of todos, repeats each field name, and an `id` must be
unique.) The `ul.errors[data-errors=…]` list is the same one
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
| `help` | `string` | — | a hint drawn under the textarea |

Slots: none.

From the page above:

```html
<al-textarea label="Bio" placeholder="A line or two"><label>Bio<textarea name="bio" placeholder="A line or two"></textarea></label><ul class="errors" data-errors="bio"></ul></al-textarea>
```

### `al-select`

A select, on a field whose type is a union of constructors without
arguments (chapter 5, "Forms and inputs"): one option per constructor.

| Attribute | Type | Default | Meaning |
|---|---|---|---|
| `label` | `string` | — | the label text |
| `disabled` | `boolean` | `false` | disables the select and dims the field |
| `invalid` | `boolean` | `false` | colours the border and focus ring as an error; Alder sets it |
| `help` | `string` | — | a hint drawn under the select |

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
| `help` | `string` | — | a hint drawn under the checkbox |

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

`dismissible` only makes room: the button that closes the alert comes
from the library's optional script, `components/al-alert.js`, which
Alder does not ship. Datastar's morph would remove a button a script
added, so in an Alder page render your own: an `<al-button>` with a
`click` that changes the model.

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

## From another stack

alder-ui is plain CSS, so any stack that writes HTML can use it: link
`al.css` and write the elements. The one thing Alder does for you that
you then do yourself is the native control inside `<al-button>`: write
`<al-button variant="primary"><button type="submit">Save</button></al-button>`
(or an `<a>` inside), and the button takes the element's look and works
without JavaScript. The input elements expect the markup this appendix
shows, a `<label>` around the control and an optional
`<ul class="errors">` after it; a `<label for>` beside the control works
too.

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
