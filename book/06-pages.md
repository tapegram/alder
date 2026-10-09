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

view : Model -> Session -> Html Msg
view model session = ...

msg Toggle { id: TodoId }

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Toggle p -> ...,
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
- A `List` parameter repeats its key in the URL: `{ tag: List Text }` with
  `["a", "b"]` is `?tag=a&tag=b`, and `[]` is no key at all.
- `Params` is the parameter record type for this module.
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
<a href={Todos.Show { id: todo.id }}>
<a href={Todos.Index { filter: Active }}>     -- defaulted fields may be omitted
```

## `msg`

A message belongs to the page that declares it. It has no path: the
compiler generates the POST URL from the page and message name, so two
messages can never collide.

```alder
msg Toggle { id: TodoId }
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
always has the same `Model` that `view` had. They ride in the message's URL
under keys that start with `_p.` ("How a request runs", below).

A page can only send its own messages and the app-level messages below. It
cannot send another page's message.

## App-level messages

Actions that belong to no page (log out, switch theme) are declared as `msg`
in the `App` module. Any page can send them, and `App.update` handles them.
It has no `Model`, and no page of its own to render, so its `Nav` is
answered differently: `Redirect p` is a 303 to `p`, and `Rerender` and
`Reject` are a 303 back to the page that sent the message (its address,
from the browser's `Referer` header, or `/` without one). The page is then
loaded again with the new session. A `Reject` text from `App.update` is not
shown anywhere, and neither are the errors of a form on an App message that
fails to decode: both answer with the same 303 back. ("How a request runs",
below, shows the code.)

```alder
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

`Reject` is a successful result, not a failure: the `Task` that produced it
commits, so anything it wrote and any `Session.put` it ran are kept. To
refuse without changing anything, decide before writing (`let taken <-` a
query, then `Reject` or the writes). Only a failed `Task` (`Task.fail`, a
`@unique` violation, a capability error) undoes its writes and its session
changes (chapter 7).

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
  7. Rerender: load → view → patch     Redirect: 303
     Reject: load → view with the text on the form → patch
     Task failure: App.error → HTML, status 500 (404 for Query.notFound)
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
   `/_m/<Module>/<Msg>`, such as `/_m/Todos.Index/Toggle`. Anything else
   is a 404 rendered with `App.notFound`.
4. For a `POST`, the body is read: a url-encoded form is parsed (at most
   1 MiB, else status 413), a multipart body is refused (415), and any
   other body is an empty form. Datastar sends a button's message with a
   JSON body, which Alder ignores.
5. The session cookie is decoded (below).
6. The route's handler runs, and its answer is written.

A Go panic anywhere in the request becomes a 500 page rendered with
`App.error`. The router also puts the Datastar script tag,
`<script type="module" src="/_alder/datastar.js"></script>`, right after
the `<head>` of every page it writes, so your layout stays plain Alder.

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
<button data-on:click="@post(&#39;/_m/Todos.Index/Toggle?id=DJ92YHT4U9KYifXwI_Tz&amp;_p.filter=Active&#39;)">Done</button>
```

`id` is the message's own parameter. `_p.filter` is the page's parameter
`filter`, with `_p.` in front. Your names never contain a `.`, so a page
parameter can never be mistaken for a message parameter of the same name.
A parameter at its default is left out, as in links: on `/`, the same
button posts to `/_m/Todos.Index/Toggle?id=DJ92YHT4U9KYifXwI_Tz`, and the
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
joined by a dot: the session as JSON, then an HMAC-SHA256 signature of that
JSON, both in unpadded base64url (`Sign` in `rt/web/session.go`). After
logging in to the to-do app, the first part decodes to

```json
{"Theme":"Light","User":{"Ok":true,"Val":"W9CiZnDHB01lToXBlVSH"}}
```

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
must be at least 16 bytes. When it is unset, **or shorter than 16 bytes**,
the server uses a fixed development key that is public (it is in
`rt/web/session.go`), and logs this once at startup (the message is the
same in both cases):

```
warn: warning: ALDER_SECRET not set; using the development cookie key
```

Anyone can forge a session signed with that key, so always set
`ALDER_SECRET` in production.

The cookie is `HttpOnly` (page scripts cannot read it), `SameSite=Lax`,
`Path=/`, and lasts 30 days from the last time it was stored. It is
`Secure` when the request came over HTTPS, or carries the header
`X-Forwarded-Proto: https` (set by a proxy in front of the server). It is stored:

- on every page a `GET` renders;
- on every answer to a message whose `update` succeeded, whether a page or
  a 303;
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

where `<Errors field="title" />` stands. The value of a `Text.Secret` field
is never sent back: its input is a password input, and it comes back empty.

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
`Redirect` is a 303 for both kinds of request, and the browser follows
the 303.

The two kinds of message travel differently under Datastar. A form posts
with `data-on:submit="@post('<url>', {contentType: 'form'})"`, so its
fields arrive url-encoded and are decoded as the form. A `click` button's
`@post('<url>')` sends a JSON body that Alder ignores; that is why a
button's message carries its parameters in the URL.

Without JavaScript, only forms send messages: a `<form>` has
`method="post"` and an `action` with the message URL, but a `click` button
works only through Datastar. The answer to a form is a whole page, or a
303 for `Redirect`. After a whole-page `Rerender`, the browser's address bar
shows the message URL, and refreshing the page sends the `POST` again (the
browser asks first). That is one more reason to end a create with
`Redirect`.

### App messages

The handler for an App message runs `App.update` with the session and
answers with a 303 (`update__App` in `build/todos/routes__alder.go`):

```go
	if nav.Tag == rt.NavRedirect {
		return web.RedirectResponse(rt.PageURL(nav.Page), rq.Env.Session)
	}
	return web.RedirectResponse(rt.BackURL(rq.HTTP.Referer()), rq.Env.Session)
```

`rt.BackURL` keeps only the path and query of the `Referer`, so the 303
never leads to another site. Clicking "Dark mode" on `/?filter=Active`
answers `303 /?filter=Active` and a cookie whose `Theme` is `"Dark"`.

### Failure pages

A path no route matches, a page parameter that doesn't decode, and
`Query.notFound` are answered with `App.notFound`, status 404. A failed
`Task` is answered with `App.error`, status 500, and logged as an `error`
line naming the request. With `ALDER_DEV=1` (chapter 11), the error's text
is added at the end of the page's `<body>` as an HTML comment, such as
`<!-- error: Db.update Todo: no row with id "nope" -->`. Without it, the
page never shows the text.
