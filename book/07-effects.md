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
