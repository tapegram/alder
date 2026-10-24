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
alder fmt                # put the files in the canonical format
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

## Orgs, and a database per org

The rule "this row is the user's" is a declaration (`@owner`), and so is "this row is the
org's, and the user works in an org they belong to":

```alder
session { user: Maybe User.UserId, org: Maybe Org.OrgId via Membership }
choose org = Orgs.Index
silo org { Project, Todo }

table Project { id: ProjectId @id, name: Text.NonEmpty, org: Org.OrgId @ref Org @partition org }
```

A page that `requires user, org` reads with `Db.scoped Project`, which never sees another org's
rows; `via Membership` makes the runtime refuse an org the user is not a member of; and `silo
org` gives each org its own database, checked and migrated when the running app first opens
it. A `Task` writes to one database, which the compiler checks. `examples/workspace` is the
whole app, with its admin; the book's chapters 6 and 8 explain it.

## Shipping it

Where an app ships is a declaration in `App`, checked like the rest of the code (a secret
pasted into it is a compile error), and one command ships it:

```alder
deploy {
  production: Ssh "deploy@box.example.com" { domain: "blog.example.com", path: "/srv/blog" },
  preview:    Ssh "deploy@box.example.com" { domain: "*.preview.example.com", path: "/srv/blog-preview" },
  requires:   ["SMTP_URL"],
  backup:     S3 "s3://blog-backups/production",
}
```

```
alder deploy production    # check, build, test, mutation score ≥ the floor; plan the migrations;
                           # upload, back up, migrate, start beside the old build, health-check
                           # by build id, switch; a failure leaves the old build serving
alder status production    # the current build, the last five, the database, the last backup
alder rollback production  # the previous build again (never across a migration)
alder ci init              # .github/workflows/alder.yml: the same commands on every PR and push
```

A migration that drops data needs `--allow-data-loss` on the command line; the gate can be
skipped only with `--unsafe-skip-tests`, which the record keeps. Targets: a Linux box over SSH
(with Caddy and automatic TLS), Fly.io, or a script of your own. The database streams to S3
while the app runs. The book's chapter 16 has the whole story.

## More

- [The Alder book](book/README.md): the language, its tools, how the compiler works and why
  it is designed this way. Chapter 2 walks through the guestbook file by file.
- [`examples/`](examples/): three complete apps, a to-do list, a blog and a workspace of orgs
  with a database per org, each with a generated admin at `/admin`.
- [`experiments/`](experiments/): the prompts, outputs and scores of running models against
  the language.

## License

Apache-2.0, see [LICENSE](LICENSE). Copyright 2026 Tavish Pegram.

[![CI](https://github.com/tapegram/alder/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/tapegram/alder/actions/workflows/ci.yml)
