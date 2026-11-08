# The Alder Book

Alder is a small, strict language for building server-rendered web
applications. It is designed so that an AI agent (or a person) has exactly
one way to do each thing, and so that the compiler catches the mistakes
that normally show up as a 404, a 500, or a broken form.

This book is the single source of truth for the language. When the language
changes, this book changes first.

## Reading order

| Chapter | What it covers |
|---|---|
| [1. Why Alder](01-why.md) | Purpose, principles, the shape of an app, what Alder is not; installing; your first app |
| [2. A tour](02-tour.md) | The guestbook `alder new` writes, file by file; then a complete to-do app, explained |
| [3. Syntax](03-syntax.md) | Modules, declarations, expressions, blocks |
| [4. Types](04-types.md) | Records, unions, brands, refinements; what there is not |
| [5. HTML](05-html.md) | Markup, components, attributes, events |
| [6. Routes, responses, forms](06-routes.md) | Routes, responses and forms; the declared rules |
| [7. Effects](07-effects.md) | `Query`, `Task`, capabilities |
| [8. Database](08-database.md) | Tables, reads, writes, how tables are stored, migrations, and silos: a database per org (`silo org { … }`), one store per `Task`, the open-time check |
| [9. Standard library](09-stdlib.md) | Module-by-module reference |
| [10. Testing](10-testing.md) | Tests, effect tests, fakes of capabilities, tests through an in-memory browser, mutation testing, `alder test` |
| [11. Tooling](11-tooling.md) | The compiler and its outputs; editor support through `alder lsp` |
| [12. Design rationale](12-rationale.md) | Why each decision was made, with experiment results; why routes replaced pages and messages, and what `Response` is for |
| [13. How the compiler works](13-compiler.md) | Lexer to generated Go; then the language server |
| [14. A second app](14-second-app.md) | The blog and workspace examples |
| [15. Generators and the admin](15-generators.md) | Building an app from the command line |
| [16. Deploying](16-deploy.md) | The `deploy` declaration and `alder deploy` |
| [A. Grammar](A-grammar.md) | The formal grammar, including the `route` declaration and qualified names |
| [B. Common mistakes](B-pitfalls.md) | The errors seen most often in experiments, and the route-name mistakes the compiler catches |
| [C. The alder-ui design system](C-design-system.md) | The `al-` elements every app can use: linking the stylesheet, tokens and themes, every element's attributes with examples, what the renderer adds; the library's documentation |

### The longer chapters in full

- **[1. Why Alder](01-why.md)** covers the purpose, the principles, the
  shape of an app (routes, and a state that lives in the URL, the HTML, the
  session or the database), what Alder is not; installing; your first app
  with `alder new`.

- **[6. Routes, responses, forms](06-routes.md)** covers `route` (a URL with
  a method), qualified declarations (`Index.load`, `Toggle.update`) and names
  across modules, the rule for where state lives, `Response` (`redirect`,
  `render`, `refresh`, `invalid`, `toast`) and when to use which, forms, how a
  response is delivered with and without JavaScript, the request lifecycle,
  and the declared rules: who owns a row (`@owner`, the scoped `Db`
  operations), which role a route needs, and partitions by any session key
  (`@partition org`, `via Membership`, `requires user, org`, `choose`).

- **[13. How the compiler works](13-compiler.md)** follows one small program
  through every stage, from the lexer to generated Go; then the language
  server: framing, overlays, snapshots, completion and dry runs.

- **[14. A second app](14-second-app.md)** is the blog example: a relation,
  an owned table and a role in the session, a capability in `load`, routes
  and search; then the workspace example: orgs, memberships, and a database
  per org.

- **[15. Generators and the admin](15-generators.md)** covers building an app
  from the command line: `alder new route/table/form/msg/capability`,
  `alder edit table`, filling typed holes with `alder fill`, and
  `alder new admin`, a generated admin of every table that `alder regen admin`
  keeps up to date through owned regions; every transcript verified.

- **[16. Deploying](16-deploy.md)** covers the `deploy` declaration and
  `alder deploy`: the fixed gate, the migration plan and the data-loss rule,
  the `Ssh`, `Fly`, `Render` and `Script` targets, `status`, `rollback` and
  `logs`, the streamed backup and `restore`, `alder ci init` and a pull
  request's preview, SQLite in production; the `Script` transcript verified.

Chapters 3–9 are the **reference**: together they are what an agent is given
as context when writing Alder. They are kept short on purpose.

## Status

Alder is a design in progress. The reference describes the language as
currently specified; the rationale chapter records what experiments have
shown and what is still open.

### What exists

`alder new <name>` writes a first app, the guestbook of chapter 2, which
checks, tests, builds and serves. Alder installs as one binary (a release
archive, Homebrew, or a container image for CI) and needs only Go beside it
(chapter 1, "Installing").

Three example apps exist: `examples/todos` (chapter 2), `examples/blog` and
`examples/workspace` (chapter 14). All check, build, serve and pass their
tests.

### Routes and state

Every URL of an app is a `route` with an HTTP method. A `GET` route has a
`load` and a `view`; a write route has an `update` that answers with a
`Response` (`redirect`, `render`, `refresh`, `invalid`). State lives in the
URL, the HTML, the session or the database (chapter 6). The three examples
are written that way.

> **Old form.** The old `page` and `msg` form is still accepted until it is
> removed, and `alder fix --routes` rewrites an app from it.

> **Not yet:** `Response.status`, `header`, `json`, `file`, `remove`,
> `patch` and `stream` and the `input=`, `load=`, `intersect=` and `every=`
> attributes are not built yet.

### Building an app from the command line

- `alder new route`, `table`, `form`, `msg` and `capability` write plain,
  checked Alder with tests that kill its mutants.
- `alder edit table` changes a table and its migration and lists what the
  change breaks.
- `alder fill` fills a typed hole with one expression and writes it only if
  the app still checks.
- `alder new admin` writes an admin of every table, which `alder regen admin`
  keeps up to date through owned regions.

All three examples carry an admin, unedited. Chapter 15 shows every command
with a transcript `go test ./book` runs.

### Running and checking

`alder check`, `alder dump` and `alder build` exist: the to-do app serves,
and `alder build` produces a binary that runs it over HTTP against SQLite
(chapter 11).

`alder run` rebuilds and reloads: save a file and the browser shows the new
app, or the compiler's errors (chapter 11). `alder eval` evaluates an
expression against the app (chapter 11). `alder repl` keeps a session of
`let`s and evaluates expressions, queries and tasks against the app,
re-checking when its files change (chapter 11).

`go test ./book` verifies every Alder block in this book not marked as a
fragment:

- Modules and declarations are type-checked. The modules the tours show
  piece by piece must equal the examples' files, and chapter 2 must quote
  every file `alder new` writes, as it writes it.
- `eval=` and `error=` expressions are evaluated.
- Other expressions are parsed.

### Testing

`alder test` runs the app's tests, each effect test against a fresh
in-memory database. A test can replace a capability with a `fake` written in
Alder, with tables of its own. A test can also drive the app through an
in-memory browser that requests its real routes inside the test's
transaction: log in, get a page, submit its forms, click its buttons, and
read the page and the session that follow (chapter 10).

`alder test --mutate` measures the tests. It compiles every curated,
well-typed mutant of the app (a write replaced, an order reversed, a scope
dropped, …) into one test program, runs the suite once per mutant, and
prints the score and each survivor as the test that is missing. `--min R`
makes the score a gate in CI (chapter 10, "Mutation testing"; chapter 13
shows how).

All three examples run under it:

- the to-do app's 124 mutants, 122 killed (the two survivors are in its
  generated admin and equivalent);
- the blog's 202, 196 killed;
- the workspace's 300, 289 killed.

Every survivor is explained in the example's README.

> **Not yet:** the operators for a route's `requires`, its role and its
> `Response` do not exist yet (alder#187), so those rules are held by the
> browser tests alone.

### Declared authorization

The two authorization rules every app with a login needs are declared, not
written:

- **`@owner`** on a table's column gives it the scoped operations
  `Db.mine`, `Db.getMine`, `Db.updateMine`, `Db.deleteMine` and
  `Db.insertMine`, which read and write only the signed-in user's rows and
  answer anyone else's as not found.
- **`requires user, role R`** serves a route only to a session with that
  role.

The examples use them, and the mutation operator `scope dropped` checks that
their tests notice (chapters 6, 10 and 13).

The same rule holds for any session key:

- `@partition org` on a column;
- a key `org … via Membership` that the runtime sets only to an org the user
  is a member of;
- `requires user, org` and `choose org` on the routes;
- `silo org { … }`, which keeps each org's tables in a database of its own,
  opened and checked when first used, with the compiler refusing a `Task`
  that writes to two databases.

`examples/workspace` is built on them (chapters 6, 8 and 14).

### The design system

Every app has a design system, `alder-ui`: a stylesheet and twelve `al-`
elements (cards, fields, buttons, tables, badges, alerts, pagination,
layout), checked against its manifest like any custom element and served by
every build at `/static/al/al.css`. `alder new` and all three examples use
it (Appendix C).

### Migrations

`alder migrate` writes the app's migrations from a change to its tables, and
the app applies them when it starts, keeping the data. All three examples
have them (chapter 8).

### Deploying

An app ships with one command. A `deploy` declaration in `App` names its
environments: a Linux box over SSH with Caddy, a Fly.io app, a Render
service, or a script.

`alder deploy production` then:

1. runs the gate (check, build, tests, the mutation score against a floor);
2. plans the migrations against the live database and refuses one that drops
   data without `--allow-data-loss`;
3. uploads, backs up, migrates, and starts the new build beside the old one;
4. switches only when the new build answers with its own build id.

`alder status`, `rollback`, `logs` and `restore` read the state kept at the
target. The runtime streams the database to S3. `alder ci init` writes the
GitHub workflow that runs the same commands, with a preview per pull request
(chapter 16).

### Capability adapters

A capability's Go adapter lives in a package named by the compiler,
`adapters/<module's first segment>/` with
`func <rest of module><Capability>()`. `alder build` generates the wiring and
lists every missing adapter with the exact package and function to write. A
production build carries no test code (chapters 7 and 11).

> **Unwired adapters in tests.** `alder test`, `eval` and the REPL leave a
> missing adapter unwired, with a warning, so an app can be tested with
> fakes first.
