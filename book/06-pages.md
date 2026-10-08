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
- `Params` is the parameter record type for this module.
- `requires` names a field of `Session` whose type is `Maybe a`. The page is
  served only when that field is `Just`; otherwise the runtime redirects to
  the page named in `App.login`. Inside this module `session.<field>` has type
  `a`, not `Maybe a`.

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
always has the same `Model` that `view` had.

A page can only send its own messages and the app-level messages below. It
cannot send another page's message.

## App-level messages

Actions that belong to no page (log out, switch theme) are declared as `msg`
in the `App` module. Any page can send them, and `App.update` handles them.
It has no `Model`. `Rerender` from `App.update` re-renders the page that sent
the message:

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

## The request lifecycle

```
GET  /path
  1. decode route parameters      → 404 on failure
  2. check `requires`             → redirect to App.login on failure
  3. load                         → Query, run by the runtime
  4. view (with the session)      → HTML

POST (generated message URL)
  1. decode message parameters and form → 404 / re-render with form errors
  2. check `requires`
  3. load                         → Model
  4. update                       → Task, run by the runtime
  5. Rerender: load → view → patch     Redirect: 303
     Task failure: App.error → HTML, status 500
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
  (chapter 7) changes it.
- `login` names the page to redirect to when `requires` fails.
- `error` and `notFound` render failure pages.
- `msg` and `update` declare app-level messages (above). They are optional.
