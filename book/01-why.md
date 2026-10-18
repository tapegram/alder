# 1. Why Alder

## The bet

AI agents write more correct code when the language gives them fewer
choices and a compiler that checks the things they get wrong. Alder is a
test of that bet.

Agents writing web apps in general-purpose languages fail in predictable
ways: a link to a route that doesn't exist, a form field whose name doesn't
match the handler, a database query that doesn't match the schema, a missing
case, a side effect in the wrong place, a forgotten auth check. None of these
are hard problems. They are coordination problems between parts of a program
that the language doesn't know are related.

Alder makes those parts language constructs (`page`, `msg`, `form`, `table`,
`capability`) so the compiler can check that they agree.

## Principles

1. **One way to do each thing.** No aliases, no alternative syntaxes, no
   operator overloading, no typeclasses. If two programs mean the same thing
   they should look the same.
2. **Everything is annotated.** Every top-level definition has a type
   signature. Any file can be understood without reading any other file.
3. **Effects are data.** Code that reads the world returns a `Query`. Code
   that changes the world returns a `Task`. Both are descriptions that the
   runtime carries out. Application code never performs an effect.
4. **Invalid states are unrepresentable.** Brands and refinements mean a
   value is validated once, at the boundary, and trusted everywhere else.
5. **Familiar on purpose.** Alder borrows syntax from Elm, Rust, and JSX
   rather than inventing its own, so it is easy to read for anyone (or any
   model) that has seen those.
6. **The compiler is a partner.** Errors are structured, point at the fix,
   and arrive in milliseconds.

## The shape of an Alder app

An Alder app is a set of **pages**. Each page is a module that declares:

- a **route** (`page "/todos/{id}" { id: TodoId }`),
- a **Model** type, derived fresh for every request,
- `load`, which describes what to read to build the Model,
- `view`, which turns the Model into HTML,
- zero or more **messages** (`msg`), which are the POST routes the page's
  HTML can send, and
- `update`, which handles a message and describes what to write and where
  to go next.

This is Elm's Model–View–Update, with one change: the Model is not stored
between requests. It is rebuilt from the route, the session, and the
database each time. There is nothing to invalidate and nothing to get out
of sync.

Pages are served as plain HTML. Interactions are HTTP POSTs. With Datastar
present in the page, the server responds to a POST with a patch of the
changed HTML over SSE, so the page updates in place without a reload.
Without JavaScript the same app works as full page loads.

## What Alder is not

- Not a general-purpose language. There is no file I/O, no threads, no
  FFI except capabilities.
- Not a client-side framework. There is no client state in Alder. If you
  need a rich client, Alder is the wrong tool.
- Not an ORM. The database layer covers CRUD and filtering; anything else is
  a capability implemented in Go.

## Installing

Alder is one program, `alder`, the compiler and every tool around it.
Today it is built from a checkout of its repository; once a release is
published, three more ways install it without one.

**From a checkout** (the way that works now). With Go installed (below):

```
git clone https://github.com/tapegram/alder.git
cd alder
go build -o ~/bin/alder ./cmd/alder
```

Put `~/bin` on your `PATH` (or pick another directory that is on it).
`alder version` then prints `devel+<commit>` (the commit you built)
instead of a version. Pull and run the `go build` again to update.

**Once a release is published:**

- **A release archive.** Each release on GitHub
  (`github.com/tapegram/alder`, under Releases) has an archive per
  platform, named `alder_<version>_<os>_<arch>.tar.gz`: `darwin` (macOS)
  and `linux`, each for `amd64` (Intel and AMD) and `arm64` (Apple silicon,
  ARM servers), and a `.zip` for `windows_amd64`, which builds but is
  untested. Unpack it and put `alder` in a directory on your `PATH`
  (`/usr/local/bin`, or `~/bin`). A checksums file sits next to the
  archives.
- **Homebrew** (macOS and Linux): the tap is
  `tapegram/homebrew-alder`.

  ```
  brew install tapegram/alder/alder
  ```

  Homebrew installs Go with it.
- **The container image**, for CI: `ghcr.io/tapegram/alder:<version>`
  (the version without its `v`, such as `:0.1.0`) is Go's official image
  with `alder` added, so a pipeline can check, test and build an app
  without installing anything:

  ```
  docker run --rm -v "$PWD":/app -w /app ghcr.io/tapegram/alder:0.1.0 alder test
  ```

  A fresh container has an empty Go module cache, so each run would
  download `modernc.org/sqlite` again (below); mount a cache directory at
  `/go/pkg/mod` (`-v "$HOME/go/pkg/mod":/go/pkg/mod`, or your CI's cache)
  to keep later runs offline.

There is no `go install`: the compiler's Go module is named `alder`, not
its GitHub address, so the go command cannot fetch it by that address
(chapter 13, "The runtime travels with the compiler", says why the name
stays).

**Go is the one prerequisite**, whichever way you install. `alder` turns your app into Go and runs
`go build` on it, so `alder build`, `alder test`, `alder run`, `alder eval`
and `alder repl` need the `go` command on your `PATH`, version **1.27 or
newer** (from `go.dev/dl`, or your package manager). `alder new`, `alder
check`, `alder migrate` and `alder dump` do not use it. Nothing else is
needed: the runtime your app is built on is inside the `alder` binary,
and SQLite is compiled into every app.

Check what you have (here, a binary of the release `v0.1.0`):

```
$ alder version
alder v0.1.0 (go 1.27.1)
$ go version
go version go1.27.1 darwin/arm64
```

`alder version` prints the version and the Go it was built with; a
binary built from a checkout of the repository says `devel+<commit>`
instead of a version (chapter 11, "`alder version`").

The first build on a machine downloads one thing: `modernc.org/sqlite`,
the SQLite the runtime uses, written in Go, with the few modules it needs,
through Go's module proxy. The build says so on its first line:

```
fetching modernc.org/sqlite (first build on this machine)
```

Go keeps the download in its module cache, so every later build, of any
app, is offline. Without network access that first build stops with an
error that says what it needed (chapter 11).

### Editor support

Neovim gets highlighting, indentation, folds and `alder check` diagnostics on save from
[alder.nvim](https://github.com/tapegram/alder.nvim), and the generators run inside the
editor with a preview of the diff. It needs Neovim 0.11, nvim-treesitter's `main` branch and
the `tree-sitter` CLI:

```
brew install tree-sitter-cli
```

With lazy.nvim:

```
{ 'tapegram/alder.nvim', lazy = false, dependencies = { 'nvim-treesitter/nvim-treesitter' }, opts = {} }
```

then `:TSInstall alder` inside Neovim. The grammar the editor uses lives in this repository
(`tree-sitter-alder/`) and is tested against the compiler's own parser, so the two cannot
drift apart.

## Your first app

`alder new` writes a small, complete app to start from, a guestbook,
into a new directory:

```
$ alder new guestbook
created guestbook/
  .gitignore
  README.md
  adapters/README.md
  app/App.alder
  app/Entry.alder
  app/Home.alder
  app/Layout.alder
  migrations/0001_initial.sql
  migrations/schema.json
  static/app.css

next:
  cd guestbook
  alder check            # type-check the app
  alder test             # run its tests
  alder test --mutate    # measure the tests
  alder build            # build the server, build/guestbook/guestbook
  alder run              # serve on http://localhost:8080, rebuilding on every save
```

The name is the directory, and the name of the server `alder build`
makes, so it is letters, digits and `-`, starting with a letter; `alder
new` will not write into a directory that already has something in it.
Run the five commands it prints, from the new directory: the app checks,
its two tests pass, every mutant `alder test --mutate` makes is killed
(score 1.00), and
`alder run` serves it at `http://localhost:8080`. Chapter 2 reads every
file `alder new` wrote; chapter 11 has the details of each command.
