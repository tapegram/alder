# 11. Tooling

`alder check` exists as of M1, `alder build --emit` as of M3a, and
`alder dump` as of M3b. As of M3c, `alder build` makes a server: one
executable that serves the app over HTTP, with its database, sessions and
forms (chapter 6, "How a request runs"). As of M3d, `alder run` rebuilds
the app every time you save a file and reloads the browser. As of M4a,
`alder eval` evaluates an expression against the app, and `go test ./book`
verifies this book's Alder blocks: its modules and declarations are
type-checked, its `eval=` and `error=` expressions are evaluated, and its
other expressions are parsed. As of M4b, `alder test` runs the app's
tests. `fmt`, holes beyond W0002 and `query` are planned.

## Project layout

```
myapp/
  app/
    App.alder         -- session, error pages, login page
    Todo.alder        -- table Todo, TodoId, helpers
    Todos/
      Index.alder     -- page "/"
      Show.alder      -- page "/todos/{id}"
    Layout.alder      -- shared components
  adapters/           -- Go implementations of capabilities (package adapters)
  static/             -- files served under /static/ (stylesheets, images)
  go.mod              -- needed only inside another Go module (below)
  build/              -- generated, never edited
```

`examples/todos/` has this shape. `adapters/` is needed only when the app
declares a capability (chapter 7, "Implementing a capability"). `static/`
is optional: `examples/todos/static/app.css` is served at
`/static/app.css`, and the layout links it with
`<link rel="stylesheet" href={Url.external "/static/app.css"} />`.

The `go.mod` is there for Go, not for Alder. Go treats a directory that
holds `.go` files as part of the nearest Go module above it. If your app
sits inside another Go module (as `examples/todos/` sits inside this
repository), `go vet ./...` and `go test ./...` in that module would try
to compile `adapters/`, whose `import "app"` resolves only in
`build/<app>/`. A `go.mod` in the app's directory makes it a module of its
own, which the outer one skips. `examples/todos/go.mod` is just that:

```
// This go.mod only keeps the example app out of the alder module: `go vet ./...` and
// `go test ./...` at the repository root would otherwise compile adapters/, whose import of
// "app" (the generated package) resolves only in build/todos, where `alder build` copies it.
module todos

go 1.27
```

An app that is not inside a Go module doesn't need one.

## Commands

```
alder check          type-check everything; milliseconds; no output on success
alder build          check, generate Go, go build → ./build/myapp/myapp
alder run            build and run with the dev database, reloading on change
alder eval <expr>    evaluate an expression against the app: value : Type
alder repl           a session: let, evaluate, :type, :run, :run!, :html, :reload
alder test           run every `test`, each against a fresh database: ok/FAIL
alder fmt            rewrite every file in canonical format
alder dump <path>    print a file's tokens, AST, or an app's IR
alder explain <id>   explain a diagnostic code
```

### `alder build`

`alder build [dir]` turns the app in `dir` (default `.`) into one
executable:

```
$ go run ./cmd/alder build examples/todos
wrote build/todos/todos
```

`build/` is relative to where you run the command, and `<app>` is the last
part of `dir` once it is made absolute (so `alder build .` inside
`myapp/` writes `build/myapp/`). The steps are:

1. Check the program. With errors or holes, print them and exit 1, as
   `alder check` would.
2. Generate the Go into `build/<app>/`, as `--emit` does (below).
3. Replace `build/<app>/adapters/` with copies of the `.go` files at the
   top level of `<dir>/adapters/` (subdirectories and other files are not
   copied), and `build/<app>/static/` with a copy of `<dir>/static/`.
4. Write `build/<app>/cmd/main.go`, the program's `main`.
5. Run `go vet` on the whole module, then `go build`, which writes
   `build/<app>/<app>`.

If the app declares capabilities and has no `adapters/` directory (or one
with no `.go` file at its top level), nothing is written, and the build
exits 1 naming each interface to implement:

```
examples/todos needs an adapters package: create adapters/caps.go with `func Caps() app.Caps` implementing: Auth.Login.Password (interface app.Cap__Auth__Login__Password)
```

If your adapters don't compile against the generated package, the build
prints Go's errors as they are, with a line naming the adapters
directory, and exits 1. If `Caps` is missing, it says the package must
declare `func Caps() app.Caps`.

Exit 2 means the problem is not in your program: a usage error (an
unknown flag, two directories), a file that cannot be read or written, or
generated Go that fails `go vet` or `go build`, which is an internal
error of the compiler and is reported as one, with Go's output. (If
`build/<app>/` holds Go files alder did not write, the message names them
instead.)

The executable reads five environment variables
(`build/<app>/cmd/main.go`):

| Variable | Meaning | Default |
|---|---|---|
| `ALDER_ADDR` | the address to listen on | `:8080` |
| `ALDER_DB` | the SQLite database file (`:memory:` for none on disk) | `<app>.db` in the working directory |
| `ALDER_SECRET` | the session cookie key, at least 16 bytes | required; in development mode, unset or shorter means a public development key, with a warning |
| `ALDER_DEV` | `1` turns on development mode | off |
| `ALDER_STATIC` | the directory served under `/static/` | the `static/` directory next to the executable |

Development mode shows a failed request's error text in the page (as an
HTML comment), logs every effect step, lets a changed schema drop and
recreate the tables (chapter 8), and accepts a missing `ALDER_SECRET`.
Once it is listening, the server prints the address:

```
$ ALDER_DEV=1 ALDER_ADDR=127.0.0.1:8080 ./build/todos/todos
warn: ALDER_SECRET is not set (or shorter than 16 bytes); dev mode signs sessions with the public development cookie key
listening on 127.0.0.1:8080
```

(The `warn:` line is logged with a date and time in front.) Outside
development mode a missing or short `ALDER_SECRET` stops the server before
it listens:

```
$ ./build/todos/todos
alder: ALDER_SECRET must be set to at least 16 bytes outside dev mode (ALDER_DEV=1)
```

and it exits 1. In production, set `ALDER_SECRET` to a long random string
and keep it: changing it logs everyone out.

The server waits at most 10 seconds for a request's headers. On `SIGINT`
(Ctrl-C) or `SIGTERM` it stops accepting connections, gives the requests
in flight up to 5 seconds to finish, and exits 0.

`build/<app>/` then holds the generated package (one
`<Module>_alder.go` per module, `app__alder.go`, `routes__alder.go`,
`go.mod`), `cmd/main.go`, `adapters/`, `static/` and the executable.
Chapter 13, section 9, reads them.

### `alder run`

`alder run [dir]` is what you use while you write an app. It builds the
app in `dir` (default `.`) the way `alder build` does, starts the
executable, and then watches your files. Every time you save, it builds
again and reloads the page in your browser. If the new code has an error,
the error appears at the top of the page and in the terminal, and the app
you had before keeps running until you fix it.

```
$ go run ./cmd/alder run examples/todos
alder run: building todos
warn: ALDER_SECRET is not set (or shorter than 16 bytes); dev mode signs sessions with the public development cookie key
dev server on localhost:8080 → app on 127.0.0.1:65295
```

(The `warn:` line comes from the app, with a date and time in front, as
in "`alder build`" above.) Open the first address,
`http://localhost:8080/`. Two programs are now running:

- **The dev server** is `alder run` itself. It listens on `ALDER_ADDR`
  (default `localhost:8080`), the address you open in the browser. Only
  your own computer can reach that default, because the app behind it
  runs in development mode, which shows error details on its pages; set
  `ALDER_ADDR=:8080` to let other machines in.
- **The app** is the executable `alder run` built,
  `build/<app>/<app>`. It runs as a *child process* of the dev server
  (a program the dev server started and will stop). The dev server
  starts it with `ALDER_ADDR=127.0.0.1:0`, and port 0 means "any free
  port", so the app gets a different port each time it starts
  (`65295` above). You never open that address yourself.

The dev server is a *proxy*: it passes every request from the browser on
to the app, and the app's answer back to the browser, unchanged (cookies,
redirects and Datastar's streamed answers included). Because the browser
only ever talks to the dev server, the app can be replaced by a new one
without the address in your browser changing.

The app runs in development mode (`ALDER_DEV=1`, see the table above),
with its database at `build/<app>/dev.db` (`ALDER_DB`) and the static
files straight from your `<dir>/static/` (`ALDER_STATIC`). `ALDER_SECRET`
and the rest of your environment are passed on to it. Because it is in
development mode, a change to your tables drops every table and creates
them again, empty (chapter 8, "Schema changes"). To start from an empty database,
stop `alder run` and delete `build/<app>/dev.db`. Two `alder run`s of the
same app started from the same directory (or an `alder run` and an
`alder build`) share `build/<app>/` and `dev.db`, and each overwrites what
the other builds, so run one at a time.

The dev server checks your files every 300 milliseconds. What it does
depends on what changed:

| You change | What happens |
|---|---|
| a `.alder` file under `app/` (or add or delete one) | check, build, start the new app, reload the browser |
| a `.go` file at the top level of `adapters/` | the same |
| any file under `static/` | reload the browser only: the app reads those files as it serves them, so nothing is rebuilt |
| anything else | nothing |

Several saves close together (a "save all" in your editor, say) count as
one change, and a change made while a build is running is picked up by the
next build, so builds never overlap. Each build prints
`alder run: rebuilding <app>` on stderr; a good one ends with
`alder run: rebuilt; app on <address>`, the new app's port.

**The old app keeps answering until the new one is up.** The new
executable is started next to the old one, on its own port. Only once it
prints `listening on` does the dev server send requests to it instead and
stop the old one. So a page you load in the middle of a build is answered
by the old app, never by an error. (For that moment the two apps share
`dev.db`.)

The browser learns what happened through three messages. Every page the
app serves under `alder run` gets two extra elements right after
`<body>`, which `alder run` asks the app to add (`ALDER_RELOAD`):

```html
<div id="_alder_errors" data-ignore-morph></div><div data-init="@get('/_dev/reload?b=afcb2a7d-1', {openWhenHidden: true})"></div>
```

The first is an empty box for errors; `data-ignore-morph` keeps an error
in it when your app updates the page. The second opens a connection to
the dev server's `/_dev/reload` as soon as Datastar starts (`data-init`
runs its expression once, when Datastar sets the element up) and keeps it
open, even while the tab is in the background (`openWhenHidden`).
`afcb2a7d-1` names the build that made the page (`-2` is the next one,
and a restarted `alder run` picks new hex digits). Over that connection
the dev server sends:

1. **The errors.** When `alder check` or the build fails, the box is
   filled with the same text the terminal shows, in a `<pre>`. A page
   opened while there is an error gets the error as soon as it connects.
2. **Clear.** After a good build, the box is emptied.
3. **Reload.** Then the page reloads itself, and you see the new app.

A change under `static/` sends only the reload. A page whose connection
was cut (the network dropped, or you restarted `alder run`) connects
again by itself; if it is from an older build by then, it is told to
reload at once. Each open tab of the app holds one such connection the
whole time, and a browser keeps at most six connections open to one
address over plain HTTP, so with six tabs open, requests from a seventh
(and the others' own clicks) wait; close the tabs you don't need.

If a check fails, you see something like this at the top of the page
(and on stderr), and the old app goes on serving:

```
app/Layout.alder:10:15: error[E0003]: expected Text, Html or List Html in `{…}`, got Int
        <title>{1}</title>
                ^
  fix: Text.fromInt (1)
```

The error box has no styling yet: it is plain preformatted text above your
page. Fix the file and save; the box clears and the page reloads.

If the new app does not start (it exits, or doesn't print `listening on`
within 10 seconds), the last 40 lines it printed on stderr go to the
error box and the old app is kept. If the app stops while it runs (it
crashes), the box says `alder run: the app exited (…); save a source
file to rebuild it`, with the last 40 lines the app printed on stderr,
and until the next good build the dev server answers every request with
502 (Bad Gateway) and a page that shows the error and reloads itself
once the app is back. The same page, saying `alder run: the app is
building…`, is what you get if you open the address before the first
build is done. Everything the app prints goes to `alder run`'s stderr.

Ctrl-C (or `SIGTERM`) stops the dev server: a build in progress finishes,
the app is stopped (`SIGTERM`, then killed if it hasn't exited after 5
seconds), and `alder run` exits 0. A second Ctrl-C kills it at once.

The app also exits when the dev server dies without stopping it: killed
with `kill -9`, say, or by a test harness's timeout, when no clean-up
code gets to run. `alder run` gives the app a *pipe* (a one-way channel
between two processes) as its standard input and keeps the other end
open, and sets `ALDER_EXIT_ON_STDIN_EOF=1`. The dev server never writes
to the pipe; the app reads it, and that read simply waits. When the dev
server's process ends, for whatever reason, the operating system closes
its end, the app's read returns *end of file* (EOF: there is no more
input, ever), and the app shuts down the same way it does on `SIGTERM`.
So a killed `alder run` leaves no app behind holding a port. An app you
start yourself, without that variable, never reads its standard input,
so a server started with its input closed (`< /dev/null`, as a service
manager does) keeps running.

`alder run` exits 2 when its arguments are wrong, when `dir` has no
`app/` directory, or when it cannot listen on `ALDER_ADDR` (another
program already uses the port, say). Chapter 13, "The dev server", shows
the exact messages and the code.

### `alder eval`

`alder eval [dir] <expr>` evaluates one expression in the context of the
app in `dir` (default `.`), through the real compiler, and prints the
value and its type:

```
$ alder eval examples/todos '1 + 2'
3 : Int
$ alder eval examples/todos 'Theme.flip Light'
Dark : Theme
$ alder eval examples/todos '[]'
[] : List a
```

Every module of the app is in scope under its own name. With `-` in place
of the expression, `alder eval` reads standard input: `let name = expr`
lines (or `let name : Type = expr`), then the expression, which may use
them. A `let`'s name starts with a lowercase letter, like any value's
(`let Todo = …` would be shadowed by the module `Todo`), and may not be
`it`, the name eval gives the expression. A `let` may define a function
that calls itself (`let fact n = if n <= 1 then 1 else n * fact (n - 1)`),
but not one defined after it. Each
`let` and the final expression starts on a line at column 1; an indented
line continues the one before it, so a long expression can span lines, and
so does a line that starts with a closer, `}`, `)`, `]` or `</tag>`, so a
multi-line `match` or element pastes as written:

```
let xs =
    [1, 2, 3]
let size n = match n {
  0 -> "none",
  _ -> "some",
}
xs
  |> List.map (\x -> x * 2)
```

The type is the one the checker infers; nothing needs a signature. The
input is like the body of a `test` in one respect: it may use any module's
brand constructors (`Todos.Show { id: TodoId "x" }`, `Text.NonEmpty
"milk"`), which elsewhere only their own module may (E0011). A `Query` is
shown as `<Query> : Query T` and not run; `--run` runs it against the dev
database `build/<app>/dev.db` (creating it, with the app's tables, when
absent) and prints the result. The Query runs with the app's default
session, every field at its default, as a test does. The app's `adapters/`,
when it has one, is compiled in, so a capability the Query calls reaches
the real Go; an app without `adapters/`, or whose `Caps` leaves a
capability's field `nil`, gets "capability … is not wired in eval" when
that capability is called. A `Select` is shown as `<Select> : Select
T`, even with `--run`: pipe it to `Db.all`, `Db.first` or `Db.count` to
make a `Query`. A `Task` is never run. A function that never stops calling itself ends in
`alder eval: stack overflow (unbounded recursion?)`. `--json` prints one object instead,
`{"value", "type", "diagnostics"}`, with `"error"` when the evaluation
failed for a reason that is not a diagnostic and `"hint"` for a note on a
success (`--run` given a `Select`). Diagnostics name the input as
`input:line:col`; `alder eval` exits 1 for them and for an evaluation
that fails or is interrupted (Ctrl-C stops it and everything it started),
2 for a usage or internal error.

The input is compiled as a module named `Scratch`. An app that has a
module of that name of its own gets an error saying so (as does a
`--module` naming a standard module, such as `List`); `--module <Name>`
gives eval's module another name (`alder eval . --module Probe '1 + 2'`).

`alder eval` writes only under `build/<app>/scratch/` of the working
directory (and, with `--run`, the dev database `build/<app>/dev.db`), and
two evaluations of one app take turns on the lock file
`build/<app>/scratch/eval.lock`, held from the build until the value is
printed, so it is safe to run from several terminals or tests at once.
One that has to wait says `alder eval: waiting for another alder eval of
this app…` on stderr first, and Ctrl-C stops the wait. Each evaluation
compiles and builds the whole app with the expression; Go's build cache
keeps that to about 0.6 s once the runtime's packages are in it (0.2 s
for an expression evaluated just before). Chapter 13, "The scratch
runner", shows what it compiles.

### `alder repl`

`alder repl [dir] [--module Name]` is `alder eval` with a memory: a
read-eval-print loop over one scratch module that lives as long as the
session. It is a tool for a person exploring an app: trying a function
on a value, looking at a type, running a query against the dev
database. An agent uses `alder eval`, which takes its whole input at
once and has `--json`; the REPL has neither.

Each line you type is one of three things:

| You type | The REPL |
|---|---|
| `let name = expr` (or `let name : Type = expr`) | checks it with the session's other `let`s; when it is clean, keeps it and prints `name : Type`. The name starts with a lowercase letter and is not `it`. A later `let` of the same name replaces it where it stands (`redefined name`): a `let` is a definition, not a snapshot, so the `let`s that use the name see the new one (`let a = 1`, `let b = a + 1`, `let a = 5`, then `b` is `6`). A `let` with an error is not kept, and a redefinition that breaks another `let` is refused, leaving the old one: `alder repl: a not redefined: let b (line 2) uses it: error[E0003]: …`. |
| `expr` | evaluates it and prints `value : Type`. A `Query`, a `Task` and a `Select` are not run: they print `<Query> : Query T` (and so on) with a hint naming the command that runs them. |
| `:command …` | one of the commands below. |

| Command | What it does |
|---|---|
| `:type expr` | prints `expr : Type` (the expression as you typed it, on one line) without building or running anything |
| `:run expr` | runs a `Query` against the dev database `build/<app>/dev.db` (the one `alder run` uses; created when absent) and prints the result |
| `:run! expr` | runs a `Task` against the same database; its writes **commit**. A task that fails (`alder repl: the task failed: …`) commits nothing: its writes are rolled back with it |
| `:html expr` | evaluates an `Html` value and prints its markup |
| `:reload` | re-checks the session against the app as it is now; a `let` that no longer checks is dropped, with a notice, and so is a `let` that used it |
| `:reset` | forgets every `let` |
| `:help`, `:h` | lists the commands |
| `:quit`, `:q` | leaves (so does Ctrl-D) |

A command ends where its letters (and `!`) do, so `:run!(Db.insert …)`
needs no space. `:run` given a `Task`, or `:run!` given a `Query`, says which one to
use instead; a `Select` needs `Db.all`, `Db.first` or `Db.count` first.
Both run with the app's default session (`TestSession()`, every field
at its default, as a test has) and, when the app has `adapters/`, its
real capabilities, exactly as `alder eval --run` does. A `Query` that
needs a session takes it as an argument you write, like any other
value. A `Task` that changes the session (`Session.put`) changes only
that run's copy: it is discarded, so `:run!` cannot log you in to the
app that `alder run` serves.

A captured session against `examples/todos`, from a fresh working
directory (the id is random):

```text
$ alder repl examples/todos
alder repl: todos (:help for the commands, :quit to leave)
alder> let xs = [3, 1, 2]
xs : List Int
alder> List.sortBy (\n -> n) xs
[1, 2, 3] : List Int
alder> :type Todo.countOpen
Todo.countOpen : List Todo -> Int
alder> Db.from Todo |> Db.count
<Query> : Query Int
hint: `:run Db.from Todo |> Db.count` runs it against the dev database
alder> :run (Db.from Todo |> Db.count)
0 : Int
alder> :run! (Db.insert User { email: Text.Email "ada@example.com", name: Text.NonEmpty "Ada", passwordHash: "" })
"6Pm-PR7HhdkHXHb9kzij" : User.UserId
alder> :run (Db.from User |> Db.count)
1 : Int
alder> :html <p>{Text.fromInt (List.length xs)} items</p>
<p>3 items</p>
alder> xs ++ "four"
expr:1:7: error[E0003]: expected List Int, got Text
  xs ++ "four"
        ^^^^^^
alder> let total = List.sum xs \
    …>   + 10
total : Int
alder> :reload
session re-checked
alder> total * 2
32 : Int
alder> :quit
```

Diagnostics are the compiler's, positioned in what you typed. One in
the expression is `expr:line:col`, counted in the expression's own
lines. One in a `let` names it and the REPL line it was typed on,
`let total (line 10): …`. Either way the source line and a caret
follow. The app's own errors are printed with their files, as `alder
check` prints them.

The REPL watches `app/` and `adapters/` as `alder run` does, every
300 ms. When you save a file it re-checks the session before the
next prompt (`app changed: session re-checked`), as `:reload` does. A
`let` that used something the edit removed is dropped. Here
`Todo.shout` was deleted from `app/Todo.alder` between the second and
third lines:

```text
alder> let greeting = Todo.shout "hello"
greeting : Text
alder> let n = 2
n : Int
alder> :reload
session re-checked
dropped let greeting: error[E0002]: module `Todo` has no `shout`
alder> greeting
expr:1:1: error[E0002]: unknown name `greeting`
  greeting
  ^^^^^^^^
```

While the app itself has errors, nothing can be evaluated. The REPL
prints the app's errors once, at start or when they appear, and each
line you type after that says only `alder repl: the app has errors
(shown above); fix them and save` (a changed set of errors is printed
again). `:reload` says the same (or prints the errors, when they have
changed) and keeps the session as it is (`the session is kept until the
app checks`), so fixing the app gets the `let`s back.

An evaluation computes only the `let`s its expression uses, directly or
through other `let`s. Each `let` becomes a value of the scratch module,
and the program computes its values when it starts, so without that a
`let` that fails, or takes a long time, would fail or slow every line
after it. With it, after `let loop n = loop (n + 1)` and `let boom =
loop 0`, `1 + 1` still prints `2 : Int`, and only an expression that
uses `boom` fails, naming the `let`s: `alder repl: stack overflow (unbounded
recursion?) (the lets it uses — loop, boom — are computed with it)`.
`:type` and `let` check with the whole session, which costs nothing.

Ctrl-C during an evaluation (or its build) stops it and everything it
started (`alder repl: interrupted`) and keeps the session; at the prompt it
only says `(use :quit or Ctrl-D to leave)`. SIGTERM, or closing the
terminal (SIGHUP), stops a running evaluation and ends the REPL with
exit 1. Killed outright (SIGKILL), the REPL cannot stop anything, but
the evaluation's program notices that the REPL is gone and exits
(chapter 13, "The REPL").

Limits:

- There is no line editing beyond what the terminal gives: no history,
  no arrow keys (the REPL uses only Go's standard library). `rlwrap
  alder repl` adds both. A line longer than 1 MB is refused (`line too
  long (limit 1 MB); ignored`), and the REPL reads on.
- A line ending in `\` continues on the next (the `\` is dropped, the
  line break kept), with the prompt `    …>`. Pasting a multi-line
  `match` needs a `\` at the end of every line but the last.
- `:run!` writes to the dev database, the one `alder run` serves. Use it
  to set up data to look at, knowing that it stays.
- Like `alder eval`, every evaluation compiles the app with the session
  module: about half a second each (0.1 s for the same expression
  again), in `build/<app>/scratch/repl-<pid>/` of the working
  directory, a directory of the REPL's own (`<pid>` is its process id),
  so two REPLs on one app never wait for each other. The REPL removes it
  when it ends; one a killed REPL left behind is removed by the next
  `alder repl` there. `:type` and `let` only check, and answer at once.
- The session's module is named `Scratch`; `--module Name` renames it
  for an app that has a `Scratch` module of its own.

Chapter 13, "The REPL", shows how it works.

### `alder test`

`alder test [dir] [-run re] [-timeout d] [--json]` runs every `test`
declaration of the app in `dir` (default `.`) and prints one line per
test as it finishes, `ok   M "name"` or `FAIL M "name"` with the reasons
indented under it, then `N passed, M failed`. Chapter 10 explains the
tests and the report.

```text
$ alder test examples/todos -run Toggle
ok   Todos.Index "Toggle flips done"
ok   Todos.Index "Toggle by another user is not found"
2 passed, 0 failed
```

- `-run re` runs only the tests whose `Module "name"` matches the
  regular expression `re` (Go's syntax). A pattern that matches nothing
  runs nothing: `0 passed, 0 failed`, exit 0.
- `-timeout d` bounds the whole run (a Go duration; default `5m`). The
  test running when the time is up fails with `error: timed out after d`
  and no later test runs. A test still running after ten seconds is
  announced on standard error: `still running M "name" (10s)`.
- `--json` prints one JSON object per test on standard output, and
  nothing else; the summary line goes to standard error. An object has
  `module`, `name`, `ok`, `trace` (a list, empty for a pure test) and
  `ms`, and `left`, `right`, `note`, `error` when they are not empty:

  ```text
  {"module":"Todos.Index","name":"Toggle flips done","ok":true,"left":"{ trace: [\"Db.get Todo\", \"Db.update Todo\"], done: Just True }","right":"{ trace: [\"Db.get Todo\", \"Db.update Todo\"], done: Just True }","trace":["Db.insert User","Db.insert Todo","Test.run","Db.get Todo","Db.update Todo","Db.get Todo"],"ms":0.704}
  ```

  When the app does not compile, `--json` prints one object,
  `{"diagnostics":[…]}`, whose items are the objects `alder check --json`
  prints, and runs nothing. Any other problem found after the arguments
  are read (a build problem, a bad `-run` or `-timeout`) is one object
  `{"error":"<message>"}`, the message also on standard error. Only a
  usage error (an unknown flag) prints no JSON.

Exit codes: 0 when every selected test passed; 1 when a test failed,
timed out (`-timeout`) or crashed the test program (a stack overflow,
say: that test is reported as a `FAIL` with the crash's first lines and
no later test runs), when the app has a check error or a typed hole (the
diagnostics are printed as `alder check` prints them; a hole adds
`alder test: no tests ran (fill the hole)`), or when the run was
interrupted; 2 for a usage error or a build problem (adapters that do
not compile, say).

Like `alder eval`, `alder test` compiles the app through the scratch
runner (chapter 13, "Running tests"): it writes only under
`build/<app>/scratch/test/` of the working directory, and two runs for
one app take turns on `build/<app>/scratch/test.lock`. The tests use no
file database: each effect test has its own, in memory. The app's
`adapters/`, when it has one, is compiled in, so capability calls reach
the real Go, and an error one returns is the failure `Cap.entry: <error>`,
which `Test.run` gives as an `Err`; without it, a test that calls a
capability fails with "capability … is not wired in tests". Ctrl-C stops
the tests' process, and so does killing `alder test` in any way (the
process exits when its standard input, a pipe `alder test` holds, closes).

### The book is verified

Every fenced `alder` block in this book that is not marked `fragment` is
verified against the compiler by `go test ./book` (`book/book_test.go`;
the parsing of chapters is `book/snippets.go`): modules, declarations and
signature tables are type-checked, `eval=` and `error=` expressions are
evaluated, and other expressions are only parsed. A block is a whole
module, declarations, a table of signatures, or expressions, and its fence
says what to expect:

| Fence | Checked |
|---|---|
| `alder` | A module or declarations: `alder check` is clean in a copy of `examples/todos` (declarations see every example module; a module replaces the example's module of that name). A table of `Name.x : Type` lines: each name's real type is at least as general as the table's (a `sig : Type` / `sig = Name.x` checks). Expressions: they parse, each starting where `alder eval` would start one (not type-checked) |
| `alder eval=<value>` | `alder eval examples/todos -` prints exactly `<value>` (the rest of the fence line) |
| `alder error=<CODE> "<text>"` | compiling (or, for expressions, evaluating) the block reports diagnostic `<CODE>` in the block itself, its message containing `<text>` (optional) |
| `alder part=<Module>` | one piece of a module shown piece by piece (the tour's `Todos.Index`): a chapter's `part=` blocks of one module, consecutive among its `part=` blocks, are joined in order and checked as that module, replacing the example's; when the example has that module, the joined pieces must also equal its file, blank lines aside |
| `alder fragment` | not checked: a deliberate excerpt, such as one arm of an `update`; listed by `go test -v ./book` |

A failure names the block's line, `book/03-syntax.md:120: E0003 …`, so a
change to the language that leaves the book behind fails the build like
any other test.

### `alder build --emit`

`alder build --emit` writes the generated Go to `build/<app>/` and vets it,
and stops there: it copies nothing and builds no executable, so it needs
no adapters. Chapter 13 walks through what it generates and why: see [How the compiler works](13-compiler.md).
It replaces only the files it generated; anything else in `build/<app>/` is
kept, and vetted with the rest. `build/<app>/` must contain only generated
files: if the vet fails and the directory holds Go files alder did not
write (a test file left there, say), the error names them; move them out
and build again.

`alder dump` shows what the compiler sees:

```
alder dump <file.alder | dir>            the AST, as syntax.Dump prints it (also --ast)
alder dump --tokens <file.alder | dir>   the tokens, one per line: L:C KIND "text"
alder dump --ir <dir>                    the app's IR, after a clean check
```

Given a directory, `--ast` and `--tokens` cover every `.alder` file under
its `app/`, in path order; `--tokens` heads each file's listing with
`-- <path>`. The tokens are the ones the parser consumed, so HTML text
appears as `Text` tokens. `--ir` needs an app directory: it checks the app
like `alder check` and prints the IR only when there are no errors and no
holes. The dump goes to stdout and diagnostics to stderr. `alder dump`
exits 0 when clean; 1 when there are errors, which for the AST and the
tokens means parse errors (they are still printed) and with `--ir` means
any check error or a hole (the IR is not printed); and 2 on a usage or I/O
failure, including `--ir` given a file. Chapter 13 reads these dumps.

The generated `go.mod` points at the alder repository, which holds the
runtime the code imports (`replace alder => "<path>"`). The `alder` binary
finds the repository it was built from; set `ALDER_ROOT` to the
repository's path when the binary was built elsewhere or moved, or to use
another checkout's runtime.

## Diagnostics

Every diagnostic has a stable code, a location, a one-line message, and,
when possible, a fix. With `--json`, diagnostics are emitted as JSON lines:

```json
{"code":"E0301","severity":"error","file":"app/Todos/Index.alder","line":28,"col":52,"endLine":28,"endCol":58,"message":"message `Toggle` needs its parameters: `Toggle { id: ?id }`","fix":{"line":28,"col":52,"endLine":28,"endCol":58,"replace":"Toggle { id: ?id }"}}
```

A fix replaces the half-open span from `line:col` to `endLine:endCol` with `replace`; when the two positions are equal it is an insertion. A diagnostic carries at most one fix; two diagnostics may propose the same insertion (for example two E0201s for one missing import), and a tool applying fixes must dedupe identical zero-width fixes.

`?name` in a fix is a hole for the agent to fill.

`alder check` exits 0 when there are no errors, 1 when there are errors, and
2 on a usage or I/O failure. A file that does not parse stops the compiler. After that, every module's declarations are checked; the bodies of a module whose names could not be resolved are skipped, so a missing import never hides type errors in another module. `alder explain <code>` prints a paragraph for any code below.

### Diagnostic codes

| Code | Meaning | Fix offered |
|---|---|---|
| E0001 | parse error (also: an identifier with `__` in it, a module name segment ending in `_`) | — |
| E0002 | unknown name | — |
| E0003 | type mismatch (also: `==` or a `Dict` key on a function or an effect value, a column method on a column held in a variable, a `Db` function over a type parameter) | conversion such as `Text.fromInt (…)` where one exists |
| E0004 | non-exhaustive `match` | arm skeleton |
| E0005 | module name does not match its file path | — |
| E0006 | import cycle | — |
| E0007 | top-level value without a type signature | signature line |
| E0008 | wrong number of parameters or arguments, or of type arguments (`Box Int Text` for `type Box a`) | — |
| E0009 | duplicate definition (two tests of one module with the same name too), or a constructor declared in two imported modules | — |
| E0010 | required declaration missing (page `load`/`view`/`update`, `App` `error`/`notFound`/`login`/`session`) | — |
| E0011 | invalid declaration (unstorable table field, unusable page or form field, bad `@id`, brand constructor used outside its module, a record that contains itself by value) | — |
| E0012 | a type, table, form or component in a page module or `App` named like a generated type: `Params`, `Msg`, `Session`, or `<Msg>Params` for a declared `msg` | — |
| E0014 | two module names that differ only in case (`Ab` and `AB`): their files would be one on a case-insensitive file system | — |
| E0015 | a module whose first name segment is exactly `Route`, `Cap` or `Args` (`Cap.Mail` is refused, `Caps.Mail` and `My.Cap` are fine): the generated Go uses those prefixes for exported names | — |
| E0016 | two pages answer the same path (`/x/{id}` and `/x/{slug}`, or `/x` and `/x/`) | — |
| E0017 | a module named like a standard-library module (`Test`, `List`, `Http`, …): the stdlib's would shadow it; uses of its names are not reported separately | — |
| E0101 | `Db.get T x` where `x` is not `Id T` | `Db.from T \|> Db.where (T.<col>.eq x) \|> Db.first` when a column of `x`'s type exists |
| E0102 | field access on a `Maybe` | `match` skeleton |
| E0201 | name from a module that isn't imported | `import M` |
| E0202 | unqualified type from another module | `M.T` |
| E0203 | qualified constructor | unqualified name |
| E0301 | message sent without its parameters | `M { field: ?field }` |
| E0302 | `update` missing an arm | arm skeleton |
| E0303 | `href` with a `Text` | — |
| E0304 | form field not in the form type, or wrong input component | list of fields (in message) |
| E0310 | unreachable `match` arm | — |
| E0401 | `Task` inside `load` or a `Query` block | — |
| E0402 | `let x <-` outside an effect block | — |
| E0501 | `@id`, `@created`, `@updated` supplied on insert | — |
| E0502 | `@ref` with the wrong id type | — |
| E0601 | `Html Msg` in a non-page module | `Html a` |
| E0602 | `case ... of`, `let ... in`, tuple, alias | the Alder form (in message) |
| W0001 | unused import | — |
| W0002 | typed hole (`?name`) with its expected type | — |
| W0003 | a form field whose decoding errors no `<Errors field>` and no `<Errors form />` of its form would show | — |

## Holes

`?name` is a typed hole. The program compiles with holes, and `alder check`
reports each one as warning W0002 with its expected type:

```
app/Todos/Index.alder:40:18: warning[W0002]: hole ?count has type Int
```

Listing the values in scope that could fill a hole is planned.

## Queries

`alder query` answers questions about the program as JSON:

```
alder query type app/Todos/Index.alder:40:18     type at a position
alder query cases Filter                         constructors of a union
alder query pages                                every page, its route and params
alder query msgs Todos.Index                     every msg of a page
alder query uses TodoId                          every use of a name
alder query capabilities                         every capability and whether it is implemented
```

## Formatting

There is one format. `alder fmt` produces it; `alder check` warns on files
that are not in it. The format is deterministic: the same program always
formats to the same text, so diffs are minimal.
