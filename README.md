# Alder

Alder is a small, strict language for server-rendered web applications, inspired by Elm. Its
pages, forms, messages, tables and capabilities are language constructs, so the compiler
checks that they agree: a broken link, a form field the handler does not expect or a write
in a read-only place is a compile error, not a 404 or a 500. It is designed so that an AI
agent (or a person) has one way to do each thing, and it compiles an app to a single Go
executable with SQLite inside.

## Install

From a checkout (the way that works today; Go 1.27+ is needed either way):

```
git clone https://github.com/tapegram/alder.git
cd alder
go build -o ~/bin/alder ./cmd/alder    # ~/bin on your PATH; alder version prints devel+<commit>
```

Once a release is published:

- **Release archive**: download `alder_<version>_<os>_<arch>.tar.gz` for your platform
  (macOS or Linux, amd64 or arm64) from this repository's Releases, unpack it and put `alder`
  on your `PATH`.
- **Homebrew**: `brew install tapegram/alder/alder` (the tap `tapegram/homebrew-alder`).
- **Container image** (for CI): `ghcr.io/tapegram/alder:<version>` (e.g. `:0.1.0`), Go's image
  with `alder` in it.

Alder needs **Go 1.27 or newer** on your `PATH`: it generates Go and builds it. The first
build on a machine downloads `modernc.org/sqlite` once through Go's module proxy; later builds
are offline. `alder version` says which alder you have. (There is no `go install`.) The book's
chapter 1, "Installing", has the details.

## A first app

```
alder new guestbook      # write a small app: a guestbook with one page, one table, two tests
cd guestbook
alder check              # type-check it
alder test               # run its tests
alder test --mutate      # measure the tests (the score, and what they miss)
alder build              # build the server, build/guestbook/guestbook
alder run                # serve it on http://localhost:8080, rebuilding on every save
```

Then grow it from the command line. Every generator writes plain, checked Alder with its
tests, and refuses rather than overwrite or write what does not check:

```
alder new page About /about                          # a page
alder new table Reply entry:Entry.EntryId@ref=Entry@cascade message:Text.NonEmpty
alder new page Replies /replies --list Reply         # a page of a table's rows
alder new form Replies NewReply entry:Entry.EntryId message:Text.NonEmpty
```

## A generated admin

`alder new admin` writes a Django-class admin for every table, as plain Alder files with their
browser tests: a dashboard, and per table a list (search, sort, pages), a page per row, forms
to create and edit (references as selects of the referenced rows) and a delete page that
counts what goes with the row and refuses what the database would. Every page `requires user,
role Admin`. When the tables change, `alder regen admin` rewrites only the admin's owned
regions and keeps what you changed around them. Both examples carry one, committed exactly as
the command wrote it, and its tests kill all of its mutants. The book's chapter 15 tours it.

## More

- [The Alder book](book/README.md): the language, its tools, how the compiler works and why
  it is designed this way. Chapter 2 walks through the guestbook file by file.
- [`examples/`](examples/): two complete apps, a to-do list and a blog, each with a generated
  admin at `/admin`.
- [`experiments/`](experiments/): the prompts, outputs and scores of running models against
  the language.

## License

Apache-2.0, see [LICENSE](LICENSE). Copyright 2026 Tavish Pegram.

[![CI](https://github.com/tapegram/alder/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/tapegram/alder/actions/workflows/ci.yml)
