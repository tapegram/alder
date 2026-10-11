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
