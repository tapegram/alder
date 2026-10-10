# 11. Tooling

`alder check` exists as of M1, `alder build --emit` as of M3a, and
`alder dump` as of M3b. As of M3c, `alder build` makes a server: one
executable that serves the app over HTTP, with its database, sessions and
forms (chapter 6, "How a request runs"). As of M3d, `alder run` rebuilds
the app every time you save a file and reloads the browser. `alder test`
arrives in M4; `fmt`, holes beyond W0002 and `query` are planned.

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
alder test           run every `test`
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
lines, then the expression, which may use them. The type is the one the
checker infers; nothing needs a signature. A `Query` is shown as
`<Query> : Query T` and not run; `--run` runs it against the dev database
`build/<app>/dev.db` (creating it when absent) and prints the result. A
`Task` is never run. `--json` prints `{"value", "type", "diagnostics"}`
instead. Diagnostics name the input as `input:line:col`; `alder eval`
exits 1 for them, 2 for a usage or internal error.

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
| E0009 | duplicate definition, or a constructor declared in two imported modules | — |
| E0010 | required declaration missing (page `load`/`view`/`update`, `App` `error`/`notFound`/`login`/`session`) | — |
| E0011 | invalid declaration (unstorable table field, unusable page or form field, bad `@id`, brand constructor used outside its module, a record that contains itself by value) | — |
| E0012 | a type, table, form or component in a page module or `App` named like a generated type: `Params`, `Msg`, `Session`, or `<Msg>Params` for a declared `msg` | — |
| E0014 | two module names that differ only in case (`Ab` and `AB`): their files would be one on a case-insensitive file system | — |
| E0015 | a module whose first name segment is exactly `Route`, `Cap` or `Args` (`Cap.Mail` is refused, `Caps.Mail` and `My.Cap` are fine): the generated Go uses those prefixes for exported names | — |
| E0016 | two pages answer the same path (`/x/{id}` and `/x/{slug}`, or `/x` and `/x/`) | — |
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
