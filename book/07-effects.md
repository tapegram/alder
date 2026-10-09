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

```alder
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

```alder
update msg model session = match msg {
  Submit p -> {
    let id <- Db.insert Todo { title: p.form.title, done: False }
    Task.succeed (Redirect (Todos.Show { id: id }))
  },
  Toggle p -> {
    let _ <- Db.update Todo p.id (\t -> { t | done: not t.done })
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
Task.map      : (a -> b) -> Task a -> Task b
Task.all      : List (Task a) -> Task (List a)
```

There is no `andThen`; use `let x <-` in a block.

## Failure

A `Task` can fail. `Task.fail "message"` fails explicitly; a capability may
fail on its own (database down, email rejected). A failed task stops
`update`, undoes everything it did (its database writes and its
`Session.put` changes), and the runtime renders `App.error`. A task that
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
a capability", below).

Capabilities are declared in any module and used by their name. The
standard library provides these; they need no declaration:

```alder
Db          -- chapter 8
Session.put : (Session -> Session) -> Task Unit
Log.info    : Text -> Task Unit
Log.warn    : Text -> Task Unit
Time.now    : Query Time
Random.int  : { min: Int, max: Int } -> Task Int
Random.id   : Task Text              -- 20-character URL-safe id
Http.get    : { url: Text } -> Task (Result HttpError Text)
Http.post   : { url: Text, body: Text } -> Task (Result HttpError Text)
```

Nothing else in the language touches the outside world. If a function's
signature has no `Query` or `Task` in it, it is pure.

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
`Query.notFound`. Building a tree does nothing. Only `eff.Run` does.

**Run is eager.** `eff.Run` (`rt/eff/run.go`) walks the tree node by node:
at a `call` it runs the step right away, at a `bind` it remembers the
**continuation** (the function that builds the rest of the block once it
is given the bound value), and when a value comes out it passes the value
on. The
`Toggle` arm of the to-do page,

```alder
Toggle p -> {
  let _ <- Db.update Todo p.id (\t -> { t | done: not t.done })
  Task.succeed Rerender
},
```

is a `bind` whose first part is the `call` named `Db.update Todo` and
whose continuation returns `pure Rerender`. `Run` performs the update,
then calls the continuation, then returns `Rerender`. Nothing is batched,
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
`Query.notFound`, an error from the database or a capability, or a Go
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
above leaves the trace `["Db.update Todo"]`; the `Add` arm of
`testdata/golden/gen_effects/app/Index.alder`, which inserts a row and
then logs, leaves `["Db.insert Todo" "Log.info"]`. The trace is the dev
log of what a request did, and it is what tests will compare (chapter 10).

**What a capability looks like in Go.** The to-do app checks
passwords outside the language
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

**A capability can write in the task's transaction.** While `Run` runs,
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

Inside the program, an implementation is found by the capability's
module-qualified name, `Env.Caps["Auth.Login.Password"]`. The generated
list `Capabilities` (in `app__alder.go`) names every key an app needs. A
call to a capability with nothing registered is a Go `panic`,
`alder: capability "Auth.Login.Password" is not wired`, which rolls the
task back like any other failure; a server never gets that far, because
it checks every capability when it starts (below).

## Implementing a capability

Your Go code goes in the app's `adapters/` directory, next to `app/`. It
is one Go package, `package adapters`, and it must declare one function:

```go
func Caps() app.Caps
```

`alder build` (chapter 11) copies the `.go` files of `adapters/` into
`build/<app>/adapters/`, inside the generated Go module. There, the
generated package is called `app`, so your code imports it as `"app"`, and
the runtime as `"alder/rt"`, `"alder/rt/db"` and so on. The generated
`cmd/main.go` calls `adapters.Caps()` and serves the app with what it
returns.

`app.Caps` is a struct with one field per capability, named
`<Module>__<Capability>` (`app__alder.go`):

```go
type Caps struct {
	Auth__Login__Password Cap__Auth__Login__Password
}
```

Every name your adapters use from `app` is exported and has a fixed
shape: `Caps`, `Cap__<Module>__<Capability>` (the interface),
`Args__<Module>__<Capability>__<Entry>` (an argument record), and `Tables`
(every table's descriptor, for `db`). The `.` in a module name becomes
`__`. That is why a module may not be named `Cap`, `Args` or `Route`
(E0015, chapter 11): its names would start the same way.

This is the to-do app's adapter, `examples/todos/adapters/password.go`,
without its last function:

```go
package adapters

import (
	"context"
	"errors"
	"time"

	"alder/rt"
	"alder/rt/db"
	"app"
)

// Caps are the implementations of the app's capabilities.
func Caps() app.Caps {
	return app.Caps{Auth__Login__Password: password{}}
}

// The one account the development stub accepts.
const (
	devEmail    = "a@b.c"
	devPassword = "password"
)

// password is a development stub of Auth.Login.Password: it accepts a@b.c with the password
// "password" and nobody else. Replace it before deploying the app.
type password struct{}

// Verify returns the id of the development account's User row for a@b.c and "password", and
// Nothing for anything else.
func (password) Verify(ctx context.Context, args app.Args__Auth__Login__Password__Verify) (rt.Maybe[string], error) {
	if args.Email != devEmail || args.Password != devPassword {
		return rt.Nothing[string](), nil
	}
	id, err := devUser(ctx)
	if err != nil {
		return rt.Nothing[string](), err
	}
	return rt.Just(id), nil
}
```

`Verify` cannot return a made-up id: a todo's `owner` is
`User.UserId @ref User`, so the id must name a real `User` row. The
missing function, `devUser`, looks the row up with `db.StoreFrom(ctx)`
and `db.TxFrom(ctx)` and inserts it the first time, inside the transaction
of the `Task` that called `Password.verify`. If that task fails, the new
row is rolled back with it.

The server refuses to start when a field of `Caps` is `nil`. `Serve`, in
`app__alder.go`, checks every capability before it opens the database,
and the binary prints the reason and exits 1:

```
alder: capability Auth.Login.Password is not wired: set Caps.Auth__Login__Password
```

An app with capabilities and no `adapters/` directory does not build at
all; chapter 11 shows the message.

Your editor will not resolve `import "app"` inside `adapters/`: that
package only exists in `build/<app>/`. For the same reason, an app inside
another Go module needs a `go.mod` of its own (chapter 11).
