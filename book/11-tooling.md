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
tests. As of M5b, `alder migrate` writes the app's migrations. As of
M10, the runtime travels inside the `alder` binary, so a build needs
only `alder` and Go, `alder version` says which alder you have, and
`alder new` writes a new app to start from. As of M12, `alder new
page|table|form|msg|capability`, `alder edit table` and `alder fill`
generate the app's code; as of M13, `alder new auth|type|decl|test`,
`alder edit page` and `alder fill --decl` complete them, so that an app
can be built from the command line alone (chapter 15).
`alder fmt` formats the app's files (below, "Formatting"). Holes beyond W0002 and `query` are planned.

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
  adapters/           -- Go implementations of capabilities, one package per module
    auth/
      password.go     -- package auth: func LoginPassword() (module Auth.Login, capability Password)
    internal/         -- Go helpers your adapters share (optional; never wired)
  elements/
    custom-elements.json  -- the custom elements the app uses besides alder-ui's (optional)
  scratch.alder       -- a scratch file for `alder scratch` (committed)
  static/             -- files served under /static/ (stylesheets, images)
  go.mod              -- needed only inside another Go module (below)
  build/              -- generated, never edited
```

`examples/todos/` has this shape. `adapters/` is needed only when the app
declares a capability: a capability `X` of module `A.B` is implemented in
`adapters/a/` by `func BX()` (chapter 7, "Implementing a capability"),
so `adapters/` holds one directory per first segment of a module with
capabilities, plus any helper directories. `static/`
is optional: `examples/todos/static/app.css` is served at
`/static/app.css`, and the layout links it with
`<link rel="stylesheet" href={Url.external "/static/app.css"} />`.
`static/al/` is the compiler's: every build writes the design system's
stylesheet and manifest there (below), so a file of yours under
`static/al/` with one of their names is replaced.
`elements/custom-elements.json` is optional too: a Custom Elements
Manifest declaring the third-party custom elements the app's markup uses
(chapter 5, "Custom elements"). Every command that checks the app reads
it; the design system's own elements (`al-…`) need no file. A file that
does not parse is E0028 at the file, with the reason:

```
elements/custom-elements.json:4:36: error[E0028]: `elements/custom-elements.json`: invalid character '}' looking for beginning of object key string
```

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

An app that is not inside a Go module doesn't need one. `alder new`
(below) decides for you: it looks for a `go.mod` above the new directory
and writes the app's own only when it finds one.

## Commands

```
usage: alder <command> [arguments]

  check     type-check the app and print its diagnostics
  fmt       format .alder files canonically
  build     compile the app to a single binary
  run       run the app with live reload
  routes    list the app's routes: pages (GET) and messages (POST)
  dump      print a file's tokens or syntax tree, or an app's IR
  eval      evaluate an expression in the app
  scratch   run a scratch file's watches against the app
  test      run the app's tests
  bench     measure each browser step of the app's tests
  migrate   write the migration for a schema change
  repl      an interactive session in the app (deprecated: use alder scratch)
  new       create an app, or generate code in one
  edit      change a table, page, declaration, auth or the session
  remove    remove a page or a table
  fill      fill a typed hole with an expression from stdin
  regen     regenerate generated code
  theme     import a theme from a CSS export
  deploy    deploy to an environment
  status    show what an environment runs
  rollback  go back to an earlier build
  logs      print an environment's logs
  restore   restore a database from a backup
  silos     manage an environment's silos
  ci        set up continuous integration
  version   print alder's version and the Go it was built with
  lsp       language server for editors, over stdio
  explain   explain a diagnostic code
  help      print usage

alder <command> --help prints its usage; alder help --json prints the command tree
```

Every command takes `--help` (or `-h`): `alder <command> --help` prints
its usage line and exits 0, and `alder new <kind> --help` the
generator's (`alder new page --help`); `alder help <command>` does the
same.

`alder check` prints nothing when the app is clean. On a terminal, after
the diagnostics it did print, it ends with one line that counts them, such
as `2 errors, 1 warning`; a pipe and `--json` get the diagnostics alone.

On a terminal the output is coloured: status words (`ok`, `FAIL`), the
locations and severities of diagnostics, summaries and labels carry colour
and weight, and `alder test` ends a failing run with the names of the tests
that failed. The bytes between the colours are the plain output, and
nothing is styled when the output is a pipe or a file, when `NO_COLOR` is
set to anything but empty, or when `TERM` is `dumb`; `--json` is never
styled. `ALDER_COLOR=always` styles any output (so `alder test | less -R`
keeps its colour; Windows terminals need it), and with it the count line of
`alder check`, the failure recap of `alder test` and the wrapping of
`alder explain` also appear in piped output; `ALDER_COLOR=never` styles
none. Only the 16 ANSI colours are used, so the terminal's theme applies.

`alder help --json` prints the command tree (commands, subcommands, arguments and flags, each with a one-line summary) as JSON; editors build completion from it. In Neovim, alder.nvim runs every generator with a preview of its diff (chapter 1, Editor support).

### `alder new`

`alder new <name>` writes a new app into the directory `<name>`: the
guestbook that chapter 2 tours, with an empty session and the two error
pages (`app/App.alder`), a layout (`app/Layout.alder`), one table
(`app/Table/Entry.alder`), one page with a form and two tests through the browser
(`app/Home.alder`), a stylesheet, a `.gitignore` (`build/` and the
SQLite files), an `adapters/` directory that holds only
a README saying what goes there, a `README.md` with the five commands, and
its first migration:

```
$ alder new guestbook
created guestbook/
  .gitignore
  README.md
  adapters/README.md
  app/App.alder
  app/Home.alder
  app/Layout.alder
  app/Table/Entry.alder
  migrations/0001_initial.sql
  migrations/schema.json
  scratch.alder
  static/app.css

next:
  cd guestbook
  alder check            # type-check the app
  alder test             # run its tests
  alder test --mutate    # measure the tests
  alder build            # build the server, build/guestbook/guestbook
  alder run              # serve on http://localhost:8080, rebuilding on every save
```

The argument may be a path (`alder new apps/guestbook`); its last part is
the app's name. What it does, in order:

1. **Checks the name**: letters, digits and `-`, starting with a letter,
   because it names the directory, the server (`build/<name>/<name>`) and,
   inside another Go module, the app's own Go module. Anything else is
   refused, and nothing is written:

   ```
   $ alder new 9lives
   alder new: `9lives` is not a valid app name: use letters, digits and `-`, starting with a letter (it names the directory, the built binary and the app's Go module)
   ```

2. **Checks the directory**: it must not exist, or be empty (a file of
   that name is refused as one). `alder new` never writes over anything:

   ```
   $ alder new guestbook
   alder new: `guestbook` exists and is not empty: choose another name, or empty it first
   ```

3. **Writes the files.** They are embedded in the `alder` binary
   (`internal/scaffold/files/`, read with Go's `embed`, like the runtime;
   chapter 13), and the app's name is put where a file mentions it (the
   README's title and commands).
4. **Writes a `go.mod`, only inside another Go module.** It walks up from
   the new directory's parent looking for a `go.mod`; when it finds one,
   the app gets the `go.mod` of "Project layout" above (`module <name>`,
   `go 1.27`, and a comment saying why), and the output gains `  go.mod`
   in the list of files and, after it, the line `go.mod keeps the app out
   of the Go module of <that go.mod>`.
5. **Runs the migration generator** on the new app, the same code as
   `alder migrate`, so `migrations/0001_initial.sql` and
   `migrations/schema.json` are exactly what `alder migrate` would write;
   running `alder migrate` right after says `no changes since
   0001_initial`.

It exits 0 when the app is written; 1 for a refused name or directory;
2 for a usage error or a file it cannot write (it then removes what it
wrote). `alder new` does not need Go: the first command that does is
`alder test` or `alder build`.

The scaffold and the book cannot drift apart: `go test ./book` runs the
built `alder new guestbook` and requires chapter 2 to quote every file it
writes, as it is, and chapters 1 and 11 to quote its output above
(`TestBook_Scaffold`), and the tests of `alder new`
(`cmd/alder/new_test.go`) build, test and serve the scaffold from a
directory outside this repository, both outside any Go module and inside
one. Its two tests kill all three of its mutants (score 1.00), and the
tests of `alder new` fail if that ever drops.

### The generators: `alder new <kind>`, `alder edit table`, `alder fill`, `alder regen admin`

As of M12, the app's own code can be generated from the command line,
as plain Alder that the checker verifies and browser tests cover
(chapter 15 walks through each command). Each command reads the app in
`--dir` (default `.`), applies its change to a copy of the app, checks
the copy as `alder check` does, and only then writes the files; it
prints them (`wrote app/About.alder`, `edited app/Home.alder`), the
unified diff of every edited one, and the copy's diagnostics, its typed
holes (W0002) with their types among them.

| Command | What it writes |
|---|---|
| `alder new page [<Module>] <path>` | `app/<Module>.alder`: the page, its `Model`, `load` and `view` in the app's layout, and a browser test. `--list T`: a table of `T`'s rows (`al-table`, badges for Bool and unions), 20 to a page with `al-pagination`; `--show T`: one row, at `<path>/{id}`, and a test of a missing one; `--from T …`: every row of each table, a card each; `--requires user` and `--role R` add the clause, and `--list` reads an `@owner` table with `Db.mine`; `--file [--type T]`: a file page (chapter 6, "File pages"), not an HTML page: its `view` returns `File.text` with the content type `T` (default `text/plain`) and a typed hole for the body, in no layout, and a test that fetches it with `Test.fetch` and checks the status is 200; it does not combine with `--list`, `--show`, `--from`, `--blank` or `--example`. The module is optional with `--list T`, `--show T` or `--from T`: it is derived from the table's plural, in the concern's folder (`Todos.Index`, `Todos.Show`, `Todos.New`; `Activity` gives `Activities`), and written as given when you name one |
| (its tests) | written to kill the mutants of the code beside them (chapter 10): the rows newest first, 20 a page, another user's rows absent (`Db.mine`) or not found (`Db.getMine`), a visitor sent to log in, a user of another role given no page |
| `alder new table <Name> <field>:<Type> …` | `app/Table/<Name>.alder` (module `Table.<Name>`; refused when a root module `<Name>` exists): `table <Name>` with its id brand, the fields, `created` and `updated`, a test of each reference's delete rule; and its migration, as `alder migrate` writes it; the last line it prints is the next step, `next: alder new page /replies --list Reply` (`--json`: a `next` field). A field is `name:Type[=default][@attr[=arg]]…`: `done:Bool=False`, `post:Post.PostId@ref=Post@cascade`, `"note:Maybe Text"` |
| `alder new form <Page> <Name> <field>:<Type> …` | in the page: `form <Name>`, `msg Submit<Name> form <Name>`, its arm of `update` (an insert into the table whose columns the fields are, by name and type, or `--into T`'s; else the hole `?submit<Name>`), the form in `view` (wrapped inputs, `<Errors form />` in an `al-alert`), and two browser tests |
| `alder new msg <Page> <Name> [<field>:<Type> …]` | in the page: `msg <Name>`, its arm (the hole `?<name>`), an `al-button` that sends it at the end of the view, and a test that clicks it and reads again the row a parameter names, or else opens the page again with `Test.open` (the message may delete the page's row), whose verdict is the hole `?expected : Bool` |
| `alder new capability <Module> <entry> "<Type>"` | the capability (named after the module's last segment) with its entry, a `fake` whose entry is the hole `?<entry>`, a test that installs the fake, and the adapter, `adapters/<segment>/<capability>_<entry>.go`, whose methods return "not implemented" |
| `alder edit table <T> add-field <field>` | the field, after the table's last, and its migration |
| `alder edit table <T> rename-field <old> <new>` | the field renamed, with `@renamed "old"` (chapter 8), and its migration |
| `alder edit table <T> drop-field <field>` | the field removed, and its migration |
| `alder fill <Module>.<decl>[.<arm>] -` | the declaration's hole (`--hole ?name` when it has several) replaced by the expression on standard input; `Module."test name"` names a test, `Module.update.Msg` an arm, `Module.Fake.entry` a fake's entry. A module of the app the expression names qualified (`Member.x`) that the file does not import is imported: `added import Member to app/…` (`--json`: `"imports"`). `alder fill --list` lists every hole: `Home update.Clear ?clear : Task Nav` |
| `alder fill <Module>.<decl>[.<arm>] --ai` | the same hole filled by a model: the expression it answers is placed and checked exactly as standard input's, and written only if the app then checks (holes it leaves are allowed). A refused answer is sent back with the checker's errors, three attempts in all, each `alder fill: attempt 2 of 3: E0003 …` on standard error; after the third, the errors and `the model's last expression:` are printed and nothing is written (exit 1). The model is the Anthropic API's (`ANTHROPIC_API_KEY`; `ALDER_FILL_MODEL`, default `claude-opus-5-5`; `ALDER_FILL_REFERENCE`, a file such as `site/reference.md` put before the prompt), or any command (`ALDER_FILL_COMMAND='claude -p --tools "" --strict-mcp-config --no-session-persistence'`: the prompt on its standard input, the expression on its standard output; the flags give it no tools, no MCP servers and no saved session), which wins when both are set. `--json` adds `"expression"`, `"attempts"`, `"model"` and `"effects"`. An expression that reaches an effect its hole's type does not limit (a capability's entry, any `Http` request, `Session.put`, `Html.unsafe`, `Task.sleep`, or `Db.insert`, `Db.update`, `Db.delete`, which are not scoped to the session's rows), itself or through a value of the app it calls (`Mail.notify → Email.send`), is printed after the diff, with `alder fill: warning: the model's expression uses …` on standard error; a test's expectation gets the hint that `alder test --mutate` is the check that it tests something. `--prompt` prints what the first attempt would send, and sends nothing. What leaves the machine is the prompt: the module's source, its comments and string literals included, the names and types in scope (not other modules' bodies), the standard library's signatures (always List, Maybe, Result, Text, Int, Float, Dict, Bool, Task and Prelude; any other module the type or the source names) and, on a retry, the errors. `ALDER_FILL_COMMAND` must be a plain text completer, with no tools and no permission bypass: the module's comments are part of the prompt, and to a tool-using agent they are instructions it may follow |
| `alder fill --decl <Module>.<decl> -` | the declaration's whole body (a value's, a test's block, a `type`'s right-hand side) replaced by the one expression, or type, on standard input; its signature stays. Refused when the result does not check (an `update` that drops an arm the view sends). For what has no hole: a `Model`, a generated body or test that does not fit |
| `alder new auth [--users User] [--role R] [--to Page]` | the login: `app/Auth/Login.alder` (`capability Password { verify, hash }`, its fake, `page "/login"`, the form `Credentials`, the arm that puts the user, and the role, in the session or rejects the form, a Log out button for a signed-in user, three tests); `App` edited at fixed places (the session's `user` and `role` after its fields, `login = Auth.Login`, `msg Logout` and its arm, a test of it); the user table when it does not exist, in `app/Table/User.alder` (`email @unique`, `name`, `passwordHash: Text.Secret`, the role, `created`) and its migration; and the adapter `adapters/auth/login_password.go` (PBKDF2-SHA256, 600,000 iterations, a 16-byte salt, a constant-time compare; stored as `pbkdf2-sha256$<iterations>$<salt>$<hash>`) with its Go tests. `R` must be a union of the user module (`alder new type`). Refused when the session has a `user` or an `Auth` module exists. A login goes to `--to`'s page, else the page at `/`, else the hole `?afterLogin` (filled once the page exists) |
| `alder new type <Module> "<Name> = …"` | a union (`Status = Pending \| Done`), a brand (`Code = brand Text where …`) or a record (`Row = { … }`), after the module's type declarations; the module is created when it does not exist |
| `alder new decl <Module> <name> "<Type>"` (`<Module>` is a module's full path, `Table.Member`; a table's name alone, `Member`, stands for its module; so in `new test`, `fill` and `edit decl`) | `name : Type` and `name = ?name`, before the module's tests; a function's body is filled with a lambda |
| `alder new test <Module> "<name>"` | `test "<name>" = { ?test }`, the hole a `Task Bool`; a block filled into it becomes the test's block |
| `alder new page … --blank` | the page with `Model = { }` and the holes `?load` and `?view` (with `--show T`, its `/{id}` and `id` parameter), and its rules' tests; fill `Model` with `--decl`, then `load`, then `view` |
| `alder new msg … --in-for <binder> [--when <expr>]` | the button at the end of the body element of the view's `{for <binder> in …}`, its class the message's name (`class="remove"`), its fields from the binder (`user: m.user`; a table's id from a row of that table, `u.id`) or holes; `--when` shows it only when the expression holds |
| `alder edit page <Page> add-param <name>:<Type>[=default]` | the parameter, at the end of the page's parameter record (a query parameter unless the path names it) |
| `alder new <name> --bare` | the scaffold without the guestbook: `App` (an empty session, the error and not-found pages), `Layout`, `static/app.css`, `adapters/README.md`, `.gitignore`; no page, table or migration |
| `alder new table … --no-timestamps` | the table without the `created`/`updated` pair; a field may then be `@created` or `@updated` itself (`created:Time@created`) |
| `alder new page <Module> /a/{x} --param x:Type …` | a page with path parameters, one `--param` for each `{x}`, its `load` the hole `?load`; `--show T --by <column>`: the row read by a `@unique` column, at `<path>/{<column>}` |
| `alder new form … --msg <Msg>`, `alder new msg … --id <id>` | the form's message named `<Msg>` (not `Submit<Name>`); the button's `id`. Both refuse while the page's `view` is a hole: fill it first |
| `alder edit session add-field <name>:<Type>=<default>` | the field, at the end of `App`'s session; the default is required; session records written elsewhere get the field with its default; written despite other breakage (`--strict` refuses) |
| `alder edit session add-field org:Org.OrgId via Membership [--choose <Page>]` | a key (M15, chapter 6, "Partitions"): `org: Maybe Org.OrgId via Membership`, no default (a session starts without one), session records written elsewhere get `org: Nothing`; with `--choose`, `choose org = <Page>` after the session. Refused for a second `via` field, a table that is not the app's, a default |
| `alder new table … org:Org.OrgId@ref=Org@partition=org [--silo]` | the field `org: Org.OrgId @ref Org @partition org`; the delete-rule tests sign in with the key. `--silo` puts the table in its key's `silo` in `App` (writing the declaration, or adding to it), as a table that refers to a siloed one must be |
| `alder new silo <key> <Table> … [--empty]` | `App`'s `silo <key> { … }` after the session (or the tables added to it); each table must be partitioned by the key (the refusal names the column to mark, or how to make the table again with one). A table migrations/ already create (a database may hold its rows) is refused unless `--empty`, which says none does: then it is marked `"silo"` in `schema.json`, a migration of the move is written (`NNNN_silo_<key>.sql`: it creates the table in the silos made before and DROPS DATA, the shared rows left behind, so its deploy needs `--allow-data-loss`), and a `!!` line says so (chapter 8, "Moving a table into a silo") |
| `alder new page … --requires org` | `requires user, org` (a `via` key needs the user first); `--list`/`--show`/`--from` read a table partitioned by the key with `Db.scoped`/`Db.getScoped`; the tests sign in as a member of a row of the key's table, show another org's row left out, and that a session without the key goes to `choose org`'s page. `alder new form` on such a page inserts with `Db.insertScoped` |
| `alder new msg App <Name> [<field>:<Type> …]` | in `App`: `msg <Name>`, its arm of `update` (the hole `?<name>`; the `update` is created when `App` has none) and a test that calls `update` and ends in the hole `?expected`; no button, as a message of `App` is sent from any page |
| `alder new form <Page> <Form> … --msg <M>` for a `msg M { … }` declared without a form | the message rewritten as `msg M { … } form <Form>`, the form in the view and a test of `Test.submit (M { … })`; the message's own button and its `pressing` test give way; refused when `M` has a form |
| `alder edit decl <Module>.<Name> add-prop <name>:<Type>=<default>` | the property, at the end of a component's props record (`Layout.Page`); the default is required, so that every use keeps checking |
| `alder edit auth --to <Page>` | `?afterLogin` filled in `Auth.Login`'s `Submit` and its test; refused when there is no such hole |
| `alder remove page <Page>`, `alder remove table <T>` | the page's file; the table's declaration, id type and its module's tests (or the whole module), and the migration that drops it (`DROPS DATA`). Refused while anything else names it, the errors saying where |
| `alder new admin [--module Admin] [--path /admin]` | refused unless the session has a `user` and a `role` whose type has an `Admin` constructor (the sentence says what to add; `alder new auth --role` makes them); never edits `App` or the login. The admin, under `app/Admin/`, every file headed by two comment lines naming the command (and no version, so every build of `alder` writes the same files): a dashboard (`/admin`: a card per table, its count and newest five rows), a layout with a nav of the tables, and per table `T` a plural folder (`Admin/Ts/`, the URLs unchanged) with a list (`/admin/t`: search, a filter per `Bool` and union column, sort by any column, 25 to a page), a page per row, a new and an edit form (`<al-select options>` of the referenced table's rows for a reference) and a delete page (what goes with the row; refused while a `@restrict` reference holds it); on the user table, the signed-in Admin cannot delete their own row or change their own role. Every page `requires user, role Admin` and has browser tests; the parts that follow the schema are owned regions, between `-- @admin begin <name>` and `-- @admin end`. It needs a session whose `role` has an `Admin` constructor. The tables of a `silo` get no nav link and no dashboard card: a picker of the key's rows (`/admin/orgs`), each opening that org's dashboard (`/admin/orgs/{org}`; the modules are `Admin.Orgs.Pick` and `Admin.Orgs.Dashboard`), and their pages at `/admin/orgs/{org}/<table>…`, declared `requires user, role Admin, silo org` with the org as the page's first parameter (chapter 15, "The admin of a silo") |
| `alder regen admin [--all]` | the admin's owned regions, regenerated from the app as it is now (a field added, a table added): only the lines between the markers change, so a heading, a column or a test the author added outside them stays as it is. A file the author deleted is not written again unless `--all`; a table added since (one the nav does not link to) gets its pages. Errors in the admin's own files do not stop it, since a field renamed or dropped breaks them inside the regions it rewrites; errors elsewhere do, and what it writes must check. A second run says `nothing to do: the admin is up to date`; with `--json`, `"skipped"` lists the pages not written again and `"upToDate"` says there was nothing to do. A marker without its pair is refused, naming the file and the region. An admin written before the folders were plural (`Admin/Todo/`) is refused too: "admin is in the old layout (Admin/Todo/): folders are plural now (Admin/Todos/). Delete app/Admin and run `alder new admin` again; move any code you added outside `-- @admin` regions first" |

A hole stops `alder test` (`alder test: no tests ran (fill the
hole)`), so after `new msg`, `new capability`, and a `new form` whose
fields fit no table, the order is new, fill, test: `alder fill` each hole
(`alder fill --list` lists them), then run the tests.

A generator never overwrites anything and never writes a result that
does not check. It refuses, with one sentence on standard error, exit
1 and nothing written, when a file or a name it would write exists,
when a table, page or role it names does not, and when the copy has
errors (printed first, as `alder check` prints them):

```
alder new page: module `Home` exists (`app/Home.alder`): a generator never overwrites a file; choose another module name
alder new form: the result does not check (the errors above): nothing was written
```

`alder edit table` is the exception: a change to a table breaks the code
that uses it (an insert that lacks a new field without a default, a use
of a dropped one), and those errors, printed with their files and lines,
are the to-do list, so the edit is written anyway; `--strict` refuses it
instead. A change no migration can make (a new field that is neither a `Maybe`
nor given a default, which the rows already stored could not be filled
with) is refused with `alder migrate`'s reason.

`--dry-run` writes nothing and says what it would write: `would write`
and `would edit` lines, the diff of the edited files and the created
ones in full (as a diff from `/dev/null`), the diagnostics, and last
`dry run: nothing written`. A refusal prints only the diagnostics
before its sentence: nothing was written, so no file is named. `--json` prints one object:
`{"files":[{"path":…,"action":"created"|"edited"}],"diff":…,
"diagnostics":[…],"holes":[{"module":…,"decl":…,"arm":…,"name":…,"type":…,"file":…,"line":…,"col":…}],
"written":…}`, with `"migration"` (the steps) when one was written and
`"error"` (the sentence; the diagnostics are in `"diagnostics"`) for a
refusal. The words `page`, `table`,
`form`, `msg`, `capability` and `admin` name generators, so `alder new`
refuses them as app names.

### `alder routes`

`alder routes [dir] [--json]` lists every route of the app: its pages (GET) and
its messages (POST), sorted by path. Nested modules make a page's file easy to
lose (`Sites.Clarity.Index` is `app/Sites/Clarity/Index.alder`); this answers
"where is the page at this URL" without reading the tree.

```text
$ alder routes examples/blog
KIND  PATH                                  MODULE.decl                       FILE:LINE
page  /                                     Posts.Index.page                  app/Posts/Index.alder:7
msg   /_m/Admin.Comments.Bulk/BulkDelete    Admin.Comments.Bulk.BulkDelete    app/Admin/Comments/Bulk.alder:53
…
page  /posts/{slug}                         Posts.Show.page                   app/Posts/Show.alder:9
```

A page's path is the one its `page` declaration gives. A message has no path of
its own: the table shows the URL it is posted to, `/_m/<Module>/<Msg>`, which is
what the generated route table serves. The last column is the declaration's file
(relative to the app directory) and line. The routes come from the checked
program, the same one the generated route table is built from, so a program
that does not check prints its diagnostics, as `alder check` does, and exits 1.

`--json` prints one array instead, for tools:

```text
[{"kind":"page","path":"/posts/{slug}","module":"Posts.Show","name":"page","file":"app/Posts/Show.alder","line":9,"col":1}, …]
```

`name` is `page` for a page and the message's name for a message; `line` and
`col` are 1-based. The language server answers the same list as the request
`alder/routes` (below, "Editor support"), with a file URI and a 0-based range in
place of `file`, `line` and `col`; alder.nvim uses it for its route picker.

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
2. Generate the Go into `build/<app>/`, as `--emit` does (below). This
   is a production build: it contains none of the app's `test`s, no
   fakes and no test runner (`alder test` makes its own build).
   When the app has capabilities, this includes the wiring,
   `build/<app>/caps/caps__alder.go`: a package `caps` whose
   `func Caps() app.Caps` calls each adapter's constructor (chapter 7
   quotes the blog's). It also writes the runtime the generated code
   imports into `build/<app>/rt/` ("The runtime in `build/<app>/rt/`",
   below).
3. Replace `build/<app>/adapters/` with a copy of the whole
   `<dir>/adapters/` tree (every file in every directory: helper
   packages, Go test files and `testdata/` included; a file that is gone
   from yours is removed from the copy), and `build/<app>/static/` with a
   copy of `<dir>/static/`. Into that copy it writes `al/al.css`, `al/al.js`,
   `al/custom-elements.json` and `al/fonts/` (`Geist-Variable.woff2`,
   `GeistMono-Variable.woff2` and the licence `OFL.txt`), the `alder-ui`
   design system's stylesheet, script, manifest and fonts (Appendix C,
   "Where it lives"), from the copy the binary carries; the server
   then serves them at `/static/al/…` with
   `Cache-Control: public, max-age=300, stale-while-revalidate=86400` and
   an `ETag` (a hash of the content). Their address carries no version
   (it is `/static/al/al.css` whatever the compiler), so they cannot be
   cached for good: a browser uses its copy for five minutes without
   asking, then for up to a day more while it asks again in the
   background, and the server answers `304 Not Modified` until a new
   compiler ships new files. So for up to five minutes after you deploy a
   new compiler, a browser may show the previous stylesheet, and after
   them one load more: the first load past the five minutes paints with
   the old copy while the new one is fetched for the next. In exchange a
   page load never waits on the network to paint. A tab left open across
   a deploy keeps its old `<head>` (boosted links never replace it) until
   its next boosted click: every page carries `<meta name="alder-build">`,
   the SHA-256 of the server's binary (16 hex digits), the boost sends it
   with each link's `GET` as the header `Alder-Build`, and a server of
   another build answers with a real navigation, so the tab loads the
   page once, with the new build's `<head>`. Datastar's script,
   whose address carries its version, is the one file cached for good;
   the app's own files get no cache header.
4. Write `build/<app>/cmd/main.go`, the program's `main`, which imports
   `"app/caps"` when the app has capabilities.
5. Run `go vet` on the whole module, then `go build`, which writes
   `build/<app>/<app>`.

Between steps 1 and 2, before anything is written, the build checks the
adapters (chapter 7, "Implementing a capability"). In order:

- Two capabilities whose adapters would be the same function in the same
  package stop the build:
  `internal naming error: capabilities Mail.OutBox and Mail.Out.Box both need func OutBox in adapters/mail/ (rename one of them)`.
- An `adapters/go.mod` is refused:
  ``adapters/go.mod: adapters/ is part of the generated module; delete it (your editor resolves `app` in build/<app>/)``.
- A module with capabilities whose first segment is `Internal` or
  `Testdata` is refused, one line per module:
  ``module `Internal` cannot name an adapter package: Go keeps `adapters/internal/` private to adapters/ (it holds helpers); rename the module``
  or ``module `Testdata` cannot name an adapter package: Go skips `testdata/` directories; rename the module``.
- The scan: for each capability, the build reads (without compiling) the
  `.go` files of its directory under `adapters/`, skipping test files,
  files Go would not build on this machine and symbolic links, and looks
  for its exported, parameterless constructor. The directory must be
  named exactly (`adapters/Mail/` is refused:
  `adapters/Mail/ must be adapters/mail/ (an adapter package's directory is its module's first segment, lower-cased)`).
  It lists every adapter that is missing, all in one run:

  ```
  blog is missing adapters for 2 capabilities:
  capability `Auth.Login.Password` needs `adapters/auth/` with `func LoginPassword() app.Cap__Auth__Login__Password`
  capability `Markdown.Markdown` needs `adapters/markdown/` with `func Markdown() app.Cap__Markdown__Markdown`
  ```

  (`blog` is the app's name; one missing adapter says `1 capability:`.)
  An app with capabilities and no `adapters/` at all gets this list with
  every capability on it. A line can end with what the scan found under
  the constructor's name instead, `(found func Markdown(style string): it
  must take no parameters)`, or with a hint when the directory is a Go
  keyword, `(select is a Go keyword: name the package e.g.
  selectadapter)`; when a symbolic link was skipped, the list ends with
  `(symlinks under adapters/ are ignored)`.
- A `.go` file at the top level of `adapters/` that declares `func Caps`
  is the form adapters had before Alder generated the wiring, and is
  refused: `adapters/ no longer wires capabilities: move each adapter to adapters/<first module segment, lower-cased>/ (see the list below)`
  above the list, or, when no adapter is missing,
  `adapters/ no longer wires capabilities: delete func Caps (adapters/caps.go): the generated caps package wires the adapters`
  (naming the file it found).

Any of these exits 1, and `build/<app>/` is not touched. `alder check`
does none of it: it never looks at `adapters/`. `alder test` and `alder
eval` make the same check when they build an app that has adapters (a
`.go` file anywhere under `adapters/`), print the same lines after their
own prefix (`alder test: `, `alder eval: `), and exit 1 too; the REPL
prints them (after `alder repl: `) in answer to an entry and waits for
the next one. One thing they do not refuse: a missing adapter. They
leave that capability unwired, print the list once to standard error as
a warning,

```
warning: 1 capability without an adapter runs unwired in tests:
capability `Markdown.Markdown` needs `adapters/markdown/` with `func Markdown() app.Cap__Markdown__Markdown`
```

and go on (`N capabilities without adapters run …` for several), so the
app can be tested and explored with fakes before its Go is written; a
call to an unwired capability fails "capability … is not wired in tests"
(or "in eval").

If your adapters don't compile against the generated package (a method
with the wrong signature, say), the build prints Go's errors as they are,
after the line
`alder build: the adapters of <app> (copied to build/<app>/adapters) do not compile with the app: exit status 1`,
and exits 1. Chapter 7 shows one. A package under `adapters/` that no
capability needs (a helper), and an adapter's Go test file (`go vet`
compiles those), are compiled here too, so their errors are reported the
same way. `alder test` and `alder eval` print the same line (their build
directory, `build/<app>/scratch/test/adapters` or `…/eval/adapters`, in
the parentheses) and exit 1.

Exit 2 means the problem is not in your program: a usage error (an
unknown flag, two directories), a file that cannot be read or written, or
generated Go that fails `go vet` or `go build`, which is an internal
error of the compiler and is reported as one, with Go's output. (If
`build/<app>/` holds Go files alder did not write, the message names them
instead.)

The executable reads eight environment variables
(`build/<app>/cmd/main.go`):

| Variable | Meaning | Default |
|---|---|---|
| `ALDER_ADDR` | the address to listen on | `:8080` |
| `ALDER_DB` | the SQLite database file (`:memory:` for none on disk) | `<app>.db` in the working directory |
| `ALDER_SECRET` | the session cookie key, at least 16 bytes | required; in development mode, unset or shorter means a public development key, with a warning |
| `ALDER_DEV` | `1` turns on development mode | off |
| `ALDER_STATIC` | the directory served under `/static/` | the `static/` directory next to the executable |
| `ALDER_PUBLIC_URL` | the site's public address, `http(s)://host[:port]`, that `Url.absolute` puts in front of a page's URL | `http://` and the address it listens on |
| `ALDER_LOG` | the least level of the lines written to standard error: `debug`, `info`, `warn` or `error` (chapter 16, "Operating an app") | `info` |
| `ALDER_LOG_FORMAT` | the lines' shape: `text` or `json` | `text` |
| `ALDER_SLOW_MS` | a request that takes this many milliseconds or more is a `warn` line with its route, effect steps and database time (chapter 16, "Slow requests"); `0` is off | off |
| `ALDER_METRICS` | `open`, or a token of at least 16 characters: `GET /_alder/metrics` answers Prometheus text, to `Authorization: Bearer <token>`, or with `open` to anyone, which a deploy target refuses (chapter 16, "Metrics") | off: no endpoint |

Development mode shows a failed request's error text in the page (as an
HTML comment), shows the dev panel (whose Trace tab has every effect
step; `ALDER_LOG=debug` writes them to the log too), lets a database that is not
where the app's migrations say (changed by hand, say, or, for an app
without migrations, made for other tables) be dropped and created again,
empty (chapter 8, "Migrations"), and accepts a missing `ALDER_SECRET`.
Once it is listening, the server prints the address:

```
$ ALDER_DEV=1 ALDER_ADDR=127.0.0.1:8080 ./build/todos/todos
warn: ALDER_SECRET is not set (or shorter than 16 bytes); dev mode signs sessions with the public development cookie key
listening on 127.0.0.1:8080
```

(The `warn:` line is logged with a date and time in front, in UTC.)
Each request the app answers adds a line too, such as `info: GET /
200 1.4ms 2311B rid=5f3a2c1e`; chapter 16 ("Operating an app") says
what one holds and how to filter them. A bad `ALDER_LOG`,
`ALDER_LOG_FORMAT`, `ALDER_SLOW_MS` or `ALDER_METRICS` stops the server
before it listens, like a bad
`ALDER_PUBLIC_URL` below. Outside
development mode a missing or short `ALDER_SECRET` stops the server before
it listens:

```
$ ./build/todos/todos
alder: ALDER_SECRET must be set to at least 16 bytes outside dev mode (ALDER_DEV=1)
```

and it exits 1. In production, set `ALDER_SECRET` to a long random string
and keep it: changing it logs everyone out.

`ALDER_PUBLIC_URL` is the address people (and other programs) use to
reach the site, which the server cannot know by itself: behind a proxy it
listens on one address and is reached at another. Only `Url.absolute`
reads it (chapter 9). The value is a scheme, `http` or `https`, and a
host (letters, digits, `-` and `.`, or an IPv6 address in brackets) with
an optional port from 1 to 65535; trailing slashes are dropped, so
`https://todos.example.com/` is fine, but a path (`https://example.com/app`),
a query, a fragment, a user name, an empty port (`https://example.com:`) or
any other character is refused before the server listens:

```
$ ALDER_PUBLIC_URL=todos.example.com ./build/todos/todos
alder: ALDER_PUBLIC_URL must be http(s)://host[:port], such as https://blog.example.com (got "todos.example.com")
```

and it exits 1. Unset, the address is `http://` and the one the server
listens on, an unspecified host shown as `localhost` (as `alder run` shows
it): `http://localhost:8080` for the default `:8080`, and with
`ALDER_ADDR=127.0.0.1:0` the port the operating system picked.

The server reads the variable as the program starts, before the app's
own values are computed, so a top-level value built with `Url.absolute`
(`home = Url.absolute (Index {})`) has the configured address too. The
one exception is port 0 without `ALDER_PUBLIC_URL`: the port is only
known once the server listens, which is after those values are made, so
they keep `:0` while `Url.absolute` in a `view` or `load` has the real
port. Set `ALDER_PUBLIC_URL`, or a fixed port, when that matters. The server
never reads the request's `Host` header for this: a client can write any
`Host`, and a page that printed it into a link (a password-reset email,
say) would send the reader to the attacker's site.

The server waits at most 10 seconds for a request's headers. On `SIGINT`
(Ctrl-C) or `SIGTERM` it stops accepting connections, gives the requests
in flight up to 5 seconds to finish, and exits 0.

`build/<app>/` then holds the generated package (one
`<Module>_alder.go` per module, `app__alder.go`, `routes__alder.go`,
`go.mod`, `go.sum`), `rt/` (the runtime), `caps/` (when the app has
capabilities), `cmd/main.go`, `adapters/`, `static/` and the executable.
Chapter 13, section 9, reads them.

### `alder run`

`alder run [dir]` is what you use while you write an app. It builds the
app in `dir` (default `.`) the way `alder build` does, starts the
executable, and then watches your files. Every time you save, it builds
again and reloads the page in your browser. If the new code has an error,
the error appears in the browser (a strip across the top of the viewport and
the dev panel's Errors tab, below) and in the terminal, and the app you had
before keeps running until you fix it.

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
  (`65295` above). You never open that address yourself, so the dev
  server also gives the app `ALDER_PUBLIC_URL=http://localhost:8080` (its
  own address), and `Url.absolute` gives links you can open. If you set
  `ALDER_PUBLIC_URL` yourself (the address of a tunnel to your machine,
  say), it is passed on unchanged.

The dev server is a *proxy*: it passes every request from the browser on
to the app, and the app's answer back to the browser, unchanged (cookies,
redirects and Datastar's streamed answers included). Because the browser
only ever talks to the dev server, the app can be replaced by a new one
without the address in your browser changing.

The app runs in development mode (`ALDER_DEV=1`, see the table above),
with its database at `build/<app>/dev.db` (`ALDER_DB`) and the static
files from the build's copy, `build/<app>/static/` (`ALDER_STATIC`): your
`<dir>/static/` with the design system's `al/` written into it. `ALDER_SECRET`
and the rest of your environment are passed on to it. Because it is in
development mode, a dev database that does not match the app is dropped
and created again, empty (chapter 8, "Migrations"); an app with
migrations otherwise keeps its dev data across a change, through
`alder migrate`. To start from an empty database,
stop `alder run` and delete `build/<app>/dev.db`. Two `alder run`s of the
same app started from the same directory (or an `alder run` and an
`alder build`) share `build/<app>/` and `dev.db`, and each overwrites what
the other builds, so run one at a time.

The dev server checks your files every 300 milliseconds. What it does
depends on what changed:

| You change | What happens |
|---|---|
| a `.alder` file under `app/` (or add or delete one) | check, build, start the new app, reload the browser |
| any file under `adapters/`, in any directory (not an editor's swap, backup or lock file, a name starting with `.`, or a symbolic link) | the same |
| any file under `static/` | copy `static/` into the build again (with `al/`), then reload the browser: the app reads those files as it serves them, so nothing is rebuilt |
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

The first is an empty box the dev server fills when a build fails; its
content is positioned out of the page's flow (the strip is fixed to the top
of the viewport), so an error moves nothing, and `data-ignore-morph` keeps
it there when your app updates the page. The second opens a connection to
the dev server's `/_dev/reload` as soon as Datastar starts (`data-init`
runs its expression once, when Datastar sets the element up) and keeps it
open, even while the tab is in the background (`openWhenHidden`).
`afcb2a7d-1` names the build that made the page (`-2` is the next one,
and a restarted `alder run` picks new hex digits). Over that connection
the dev server sends:

1. **The errors.** When `alder check` or the build fails, the box is
   filled with the strip and the list of errors that the panel's Errors tab
   shows (below). A page opened while there is an error gets them as soon as
   it connects.
2. **Clear.** After a good build, the box is emptied.
3. **Reload.** Then the page reloads itself, and you see the new app.

A change under `static/` sends only the reload, after the copy. A page whose connection
was cut (the network dropped, or you restarted `alder run`) connects
again by itself; if it is from an older build by then, it is told to
reload at once. Each open tab of the app holds one such connection the
whole time, and a browser keeps at most six connections open to one
address over plain HTTP, so with six tabs open, requests from a seventh
(and the others' own clicks) wait; close the tabs you don't need.

If a check fails, the terminal shows this (the browser shows the same
diagnostics, one block each, as described below), and the old app goes on
serving:

```
app/Layout.alder:10:15: error[E0003]: expected Text, Html or List Html in `{…}`, got Int
        <title>{1}</title>
                ^
  fix: Text.fromInt (1)
```

In the browser the failure is out of your page's way, and the page you see is
still the last good build. A thin strip fixed to the top of the viewport says
`Build failed: 1 error. The page is the last good build.` with a "Show errors"
button; the dev panel (below) gets an **Errors** tab, first in its list and
there only while there are errors, with a count badge, and opens on it. Each
diagnostic is a block of its own: `file:line:col` and the code (`E0003`), the
message, and the source line with its caret, in the panel's monospace. They
come from the checker's structured output, the same records `alder check
--json` prints (file, line, column, code, message), with the source line
read from your file; a build failure or a crash has no such structure, so
its text is shown as one block, counted as one error. Fix the file and save:
the strip and the tab go away and the page reloads.

If the new app does not start (it exits, or doesn't print `listening on`
within 10 seconds), the last 40 lines it printed on stderr go to the
Errors tab and the old app is kept. If the app stops while it runs (it
crashes), the list says `alder run: the app exited (…); save a source
file to rebuild it`, with the last 40 lines the app printed on stderr,
and until the next good build the dev server answers every request with
502 (Bad Gateway) and a page that is that same list, styled, in the light
or dark colors of your system, which reloads itself once the app is back. The same page, saying `alder run: the app is
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
$ alder eval examples/todos 'Todo.filterLabel Active'
"Active" : Text
$ alder eval examples/todos '[]'
[] : List a
```

Every module of the app is in scope under its own name. A constructor
two modules declare is ambiguous unqualified (E0009): in an app with a
generated admin, every table's pages declare `Create`, `Save` and
`Remove`, so write those qualified (`Admin.Posts.New.Create`). With `-` in place
of the expression, `alder eval` reads standard input: `let name = expr`
lines (or `let name : Type = expr`), then the expression, which may use
them. A `let`'s name starts with a lowercase letter or `_`, like any value's
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
the real Go; an app without `adapters/`, one whose adapter is missing
(eval warns, as `alder test` does, and goes on), or whose adapter's
constructor returns `nil`, gets "capability … is not wired in eval" when
that capability is called. A `Select` is shown as `<Select> : Select
T`, even with `--run`: pipe it to `Db.all`, `Db.first` or `Db.count` to
make a `Query`. A `Task` is never run. A function that never stops calling itself ends in
`alder eval: stack overflow (unbounded recursion?)`. `--json` prints one object instead,
`{"value", "type", "diagnostics"}`, with `"error"` when the evaluation
failed for a reason that is not a diagnostic and `"hint"` for a note on a
success (`--run` given a `Select`). Diagnostics name the input as
`input:line:col`; `alder eval` exits 1 for them, for adapters that are
refused or do not compile with the app (`alder build`'s messages), and
for an evaluation that fails or is interrupted (Ctrl-C stops it and
everything it started), 2 for a usage or internal error.

The input is compiled as a module named `Scratch`. An app that has a
module of that name of its own gets an error saying so (as does a
`--module` naming a standard module, such as `List`); `--module <Name>`
gives eval's module another name (`alder eval . --module Probe '1 + 2'`).

`--in Name` evaluates inside module `Name` (its full path, `Table.Todo`) instead: the expression is checked as if written
at the end of `app/Name.alder`, so the module's own declarations and imports are unqualified
and the other modules are reachable as the module itself imports them, as a `test` there would
write them (the overlay adds no imports). A line copied from the file evaluates unchanged:

```text
$ alder eval examples/todos --in Table.Todo 'countOpen []'
0 : Int
```

A diagnostic of the expression is positioned in the input (`input:1:5`). When a `let` redeclares
a name the module declares, the earlier declaration is named by its file, since its line is not
a line of the input: ``input:1:5: error[E0009]: `countOpen` is already declared in
app/Table/Todo.alder:34``.

`--in` is deliberately more permissive than `alder check` about one thing: eval infers the
type of the module's own top-level values that have no signature (as it does the input's
`let`s), where `check` reports E0007 for them. The input has no place for
signatures, and the checker's inference switch (`InferModule`) applies to the whole module the
input is appended to, so it covers the module's own unsigned values too. That is accepted:
eval is for probing, and `alder check` gives the strict verdict.

`--in` and `--module` do not combine (`alder eval: --in and --module do not combine: --in Name
works inside module Name, --module names the scratch module`, exit 2). A module that declares
`it` cannot host eval (exit 2).

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
runner", shows what it compiles. That build is a test build, like
`alder test`'s: the app's tests and fakes are compiled in (eval is a
developer tool, never deployed), because its main runs with the
session a test starts from, `TestSession()`, which only a test build
has. `alder repl` builds the same way.

### `alder scratch`

`alder scratch [file] [--watch] [--json] [--build-dir DIR] [--module Name]`
runs a scratch file: a file of your own in the project, next to the app,
where you define values and helpers and ask the compiler what some
expressions are. `alder new` writes one, `scratch.alder`, at the project
root (and `alder new --bare` too). It is committed with the app, so it
has no `.gitignore` entry. The file is the whole interface: edit it in
your editor, save, read the results.

A scratch file is any file whose name ends in `scratch.alder` and that
is not under `app/` (`scratch.alder`, `probe.scratch.alder`,
`notes/db_scratch.alder`). `alder scratch` takes the file as an argument,
`./scratch.alder` by default, and runs it against the app above it: the
nearest directory up from the file that holds `app/*.alder`. Any other
file is refused (exit 2). A scratch file given as a symbolic link is judged by its
target (the CLI and the language server both resolve links): the target must be
named `*scratch.alder` and live in an app's project. A file with `>` lines
is not a module, so `alder fmt` formats a scratch file in a way of its own
(this chapter, "Formatting"), and the language server analyses it on its
own ("Scratch files in the editor", below).

A line that starts with `> ` at column 1 is a watch: the rest of the line
is an expression, and lines indented under it continue it. Everything
else in the file is ordinary Alder (definitions, comments, blank lines),
checked together with the watches, so a helper you define is in scope for
every watch, above or below it. The starter file:

```text
-- A scratch file: each `> expr` line is evaluated. Run `alder scratch --watch` to see results on save.
-- Everything else is ordinary Alder: define values and helpers here, then watch them.
-- Put `in Module` on the first line to work inside an app module, with its names unqualified.

> 1 + 1
```

Run on the starter file, it prints the line and the value with its type:

```text command=guestbook
$ alder scratch
scratch.alder:5: 1 + 1
  ⇒ 2 : Int
```

Each watch prints its line, then its value and type (the names `it1`,
`it2`, … are the watches' own and cannot be defined in the file). A file
with a helper, a two-line watch and a query:

```text
$ alder scratch probe.scratch.alder
probe.scratch.alder:4: double 21
  ⇒ 42 : Int
probe.scratch.alder:5: [1, 2,
  ⇒ [1, 2, 3] : List Int
probe.scratch.alder:7: Db.from Entry |> Db.count
  <Query> : Query Int   (not run: scratch evaluates pure values only)
```

Without a header the file is the module `Scratch` (`--module Name` names
it otherwise), and it sees the app's modules by their qualified names.
`in Module` as the first line of the file that is neither blank nor a
comment makes it work inside that app module instead, as `alder eval
--in` does: the module's own names are unqualified.

In this version a scratch file evaluates pure values only. A watch whose
type is a `Query`, a `Task` or a `Select` is not run: it prints
`<Query>` (or `<Task>`, `<Select>`) with its type and the note `not run:
scratch evaluates pure values only`. A line starting `>!` (which would
run a task) is an error. A watch whose value would be longer than 64 KB
is cut there and says so, and the whole run has a 60 second limit. A
watch that fails when it runs (an `error`, a crash) fails on its own
line and the watches after it still show their values; if the program
itself dies, the watches after the crash say `not run: the program
crashed at line N`.

Errors in the file are reported at their place in the file, as you named
it (`scratch.alder:7:3`), with the compiler's usual messages; errors in
the app keep the app's file names. A typed hole in a watch is a
warning-level diagnostic that still makes the run exit 1.

`--watch` keeps running: it runs the file, then again whenever the file
or any file of the app changes (polled as `alder run` does), printing
`── HH:MM:SS ──` before each run. A change during a run cancels it and
starts the next. A save that does not parse, or a file that is gone for
a moment, is reported and the watching goes on; Ctrl-C ends it with
exit 0.

`--json` prints one object instead of the text: `{"watches": […],
"diagnostics": […], "ran": bool}`, with `"note"` and `"error"` when
there is something to say. A watch is `{"line", "end", "expr", "type", "kind",
"value"}` (`"kind"` is `pure`, `query`, `task` or `select`, absent when the
type is unknown) plus `"note"`, `"truncated"` or `"error"` when they apply; diagnostics
have the shape of `alder check --json`. With `--watch` each run is one
such object on its own line.

The exit code is 0 when nothing failed; 1 for an error in the file or
the app, a typed hole, a watch that failed, an app whose adapters will
not build, or an interrupt without `--watch`; 2 for a usage or I/O
error (a file that is not a scratch file, no app above it, an unknown
flag). The build goes to `build/<app>/scratch/file-<hash>/`, one
directory per scratch file (the hash is of its absolute path, symbolic
links resolved).

### `alder repl`

`alder repl [dir] [--in Name | --module Name]` is deprecated and will be
removed in the next release. It prints, on standard error after its
banner, `alder repl is deprecated: use a scratch file (alder scratch
--watch, or save scratch.alder in the editor); it will be removed in the
next release`. Everything the REPL did a scratch file does with a file
you keep, edit and commit instead of a session that forgets itself when
you quit:

| In the REPL | In a scratch file |
|---|---|
| `let name = expr` | a declaration in the file (`name = expr`, or a function with its signature), in scope for every watch |
| `expr` | a watch: `> expr` |
| `:type expr` | the types phase: the type of every watch appears as soon as the file checks, before anything is built or run (in the editor, "Scratch files in the editor" below) |
| `:run expr`, `:run! expr` | not in this version: a scratch file evaluates pure values only, and a `Query`, a `Task` or a `Select` prints its type and `not run`; `alder eval --run` still runs them |
| `:html expr` | `> expr` on an `Html` value: the watch shows its value |
| `:reload` | save the file (`alder scratch --watch` also re-runs when the app changes) |
| `:reset` | delete the lines |
| `:in Name`, `--in Name` | the `in Name` header, the first line of the file that is not a comment |
| `:help`, `:quit` | nothing to learn, nothing to leave |

Chapter 13, "The REPL", still describes how it works, for as long as it
exists.

### `alder test`

`alder test [dir] [-run re] [-timeout d] [--json] [--mutate [--min R]]` runs every `test`
declaration of the app in `dir` (default `.`) and prints one line per
test as it finishes, `ok   M "name"` or `FAIL M "name"` with the reasons
indented under it, then `N passed, M failed`. Chapter 10 explains the
tests and the report. An app with no tests at all passes (exit 0), and
its summary is followed by a hint, since a suite of nothing kills no
mutant (`--json` prints no hint):

```text
$ alder test
0 passed, 0 failed
no tests: `alder test --mutate` kills nothing (chapter 10)
```

```text
$ alder test examples/todos -run Toggle
ok   Todos.Index "Toggle flips done"
ok   Todos.Index "Toggle by another user is not found"
2 passed, 0 failed
```

Every click and submit a test makes also checks the runtime's partial
updates (chapter 5, "Partial updates"): the test's browser applies the
answer as a browser would, region by region, and then compares the page
it holds with a full render of the same page. A difference fails the
step, naming where: `POST /_m/Todos.Index/Toggle: the patched page
differs from a full render at body > main > ul#todos: …`. Your tests do
not mention it; every suite proves the patches on its own pages.

- `-run re` runs only the tests whose `Module "name"` matches the
  regular expression `re` (Go's syntax). A pattern that matches nothing
  runs nothing: `0 passed, 0 failed`, exit 0.
- `-timeout d` bounds the whole run (a Go duration; default `5m`; with
  `--mutate`, default `10s`, it bounds each run, the tests' own and each
  mutant's, a mutant's being at least three times the tests' own run). The test running when the time is up fails with `error:
  timed out after d` and no later test runs. A test still running after ten seconds is
  announced on standard error: `still running M "name" (10s)`.
- `--json` prints one JSON object per test on standard output, and
  nothing else; the summary line goes to standard error. An object has
  `module`, `name`, `file`, `line`, `col`, `ok`, `trace` (a list, empty for a pure test) and
  `ms`, and `left`, `right`, `note`, `error` when they are not empty.
  `file`, `line` and `col` are the position of the `test` keyword, for editors:

  ```text
  {"module":"Todos.Index","name":"Toggle flips done","file":"app/Todos/Index.alder","line":92,"col":1,"ok":true,"left":"{ trace: [\"Db.updateMine Todo\"], done: Just True }","right":"{ trace: [\"Db.updateMine Todo\"], done: Just True }","trace":["Db.insert User","Db.insert Todo","Test.login","Test.run","Db.updateMine Todo","Db.get Todo"],"ms":0.466}
  ```

  When the app does not compile, `--json` prints one object,
  `{"diagnostics":[…]}`, whose items are the objects `alder check --json`
  prints, and runs nothing. Any other problem found after the arguments
  are read (a build problem, a bad `-run` or `-timeout`) is one object
  `{"error":"<message>"}`, the message also on standard error. Only a
  usage error (an unknown flag) prints no JSON.

- `--mutate` measures the tests (chapter 10, "Mutation testing"): when
  the selected tests pass, it runs them again once per mutant of the app,
  each a small well-typed change the compiler makes (a `requires`
  dropped, a write replaced, an order reversed), and adds to the report
  `N mutants: K killed, S survived  (score X.XX)` and, under
  `survived:`, one line per survivor: module, `declaration:line` (and
  `:column` when the line has mutants of that kind in two places),
  operator, and the code before `→` after. A mutant is killed by the
  first test that fails against it, crashes or runs out of time. When a
  selected test fails, the report ends with `tests fail: fix them before
  measuring mutants` (exit 1); with no test, `no tests: nothing can kill
  a mutant`, or `no test matches -run: nothing can kill a mutant` when
  `-run` selected none (exit 0, or 1 with `--min`). The build is `build/<app>/scratch/mutate/`, beside
  the plain test build, which it never replaces. With `--json`, after
  the per-test objects come `{"mutants":[…]}` (each with `id`, `module`,
  `file`, `decl`, `line`, `col`, `kind`, `before`, `after`, `killed`,
  `by`, `note`, and
  `"tables":"ensure"` for a schema mutant) and `{"score":…, "killed":…,
  "survived":…, "suppressed":…, "skipped":…}`; the refusals go to
  standard error there.
- `--mutate` caps the goroutine stack at 64 MB (Go's default is 1 GB), so
  a mutant that recurses without end crashes quickly, whatever the load;
  a suite that needs more stack than that fails its own run under
  `--mutate` (and says so).
- `--min R` (with `--mutate` only; `R` from 0 to 1, such as `0.8`) exits
  1 when the exact score is below `R`, after the same report. Without
  it, `--mutate`'s exit code is the tests'. `--min` without `--mutate`,
  or an `R` outside 0 to 1, is a usage error (exit 2).

Exit codes: 0 when every selected test passed; 1 when a test failed
(with `--mutate`, also when the score is below `--min`),
timed out (`-timeout`) or crashed the test program (a stack overflow,
say: that test is reported as a `FAIL` with the crash's first lines and
no later test runs), when the app has a check error or a typed hole (the
diagnostics are printed as `alder check` prints them; a hole adds
`alder test: no tests ran (fill the hole)`), when the adapters are
refused or do not compile with the app (the same messages as `alder
build`'s: they are your program too), or when the run was interrupted;
2 for a usage error or another build problem. A missing adapter is not
an error here: the capability runs unwired, with the warning above.

Like `alder eval`, `alder test` compiles the app through the scratch
runner (chapter 13, "Running tests"): it writes only under
`build/<app>/scratch/test/` of the working directory, and two runs for
one app take turns on `build/<app>/scratch/test.lock`. The tests use no
file database: each effect test has its own, in memory, made by
applying the app's migrations when it has them (chapter 8). The app's
`adapters/`, when it has one (a `.go` file anywhere under it), is
compiled in, wired by the same generated `caps` package as `alder build`'s,
so capability calls reach the real Go, and an error one returns is the
failure `Cap.entry: <error>`, which `Test.run` gives as an `Err`; a
capability whose adapter is missing is left out of that package (with
the warning), and without `adapters/` none is wired: a test that calls
an unwired capability fails with "capability … is not wired in tests". A test can
replace a capability with its `fake` (`Test.fake`, chapter 10). Its
build is a test build, which adds the tests themselves (`Tests`,
`TestSession`), each fake's Go and the fakes' tables (`FakeTables`),
created in each test's database after the app's. `alder eval` and
`alder repl` build the same way; `alder build` and `alder run` compile
none of it, so a production binary carries no test code (chapter 13,
"Fakes"). Ctrl-C stops
the tests' process, and so does killing `alder test` in any way (the
process exits when its standard input, a pipe `alder test` holds, closes).

### `alder bench`

```
alder bench [dir] [-run re] [--count N] [-timeout d] [--json] [--build-dir DIR]
```

`alder bench` answers "what does a request of this page cost?" with the
app's own tests as the scenarios. It builds the same test program as
`alder test` (into `build/<app>/scratch/test/`), runs it with the
environment variable `ALDER_TEST_BENCH=1`, and the program then measures
every browser step a test takes (`Test.get`, `Test.open`, `Test.click`,
`Test.submit`, `Test.submitRaw`, `Test.fetch`, chapter 10): what the
server spent on the requests the step made. There is no new language and no
second main: a scenario is a test, written with the app's types, fakes
and steps, and `-run`, `--json`, `--build-dir`, `-timeout` and the exit
codes are `alder test`'s. Pure and property tests are skipped (they make no
request); `--mutate`, `--min`, `--fuzz` and `--seed` print the usage. Each
selected test runs `--count` times (default 3), each run on its own fresh
database as always, and a step's time and allocations are the median of
the runs.

After each test's `ok` line comes one row per step, named after its first
request:

```
ok   Rows.Bench "toggle 1000"
  step                                      ms     bytes               alloc  req loads steps   sql  n+1
  GET /                                   5.25  811.3 kB    12.32 MB / 57.8k    1     1     1     1
  POST /_m/Rows.Index/Toggle              6.67  808.4 kB    12.70 MB / 66.7k    1     2     3     4
```

| column | what it counts |
|---|---|
| `ms` | the wall time the router took to serve the step's requests (summed) |
| `bytes` | the answers' bodies: the document of a GET, the event stream of a Datastar answer (the regions patched, or the body, title, history) |
| `alloc` | bytes and objects the Go runtime allocated while the router served them (`runtime.MemStats`: `TotalAlloc` and `Mallocs` read just before and just after each request) |
| `req` | requests the router served: 1, 2 when it followed a `Redirect` itself (chapter 6), more for each 303 the browser followed |
| `loads` | how many times a page's `load` ran: a `Rerender` runs it twice (before `update`, to give it the model, and after, to render), and so does a followed `Redirect` (the message's page, then the target) |
| `steps` | the effect steps the requests ran (their trace) |
| `sql` | the SQL statements they ran |
| `n+1` | the statements run in a loop (below): count and distinct argument sets |

With `--json` each test's object carries `"bench": [{"step", "ms", "bytes",
"alloc", "allocs", "requests", "loads", "steps", "sql", "repeats"}]`. The
test's own effects (its seeding inserts, `Test.run`) are not measured, only
the browser's requests. A test that fails is reported as `alder test`
reports it, and its rows still follow (its steps ran).

How the numbers are taken, so you know what they mean: a test's browser
calls the app's real router in-process (chapter 10), so there is no
network; each request's `Env` gets a recorder (`eff.Recorder`) that times
every effect step and every statement, and the runtime counts each Query
it runs on that `Env` (a page's `load` is the only Query a request runs).
Just before and just after the router serves each request the program
reads the clock and the Go runtime's memory statistics, so the test
browser's own work (parsing the answer into a document, applying a
Datastar patch to it) is not counted: that is the client's cost, which a
real browser pays in the DOM. The requests run inside the test's transaction under a
savepoint, on an in-memory database, with the recorder on, so the numbers
are for comparing (before and after a change, N rows against 10N), not
production latencies: the browser's time in the DOM is not in them at all.
For a production-shaped profile write a Go benchmark in the emitted build
(`alder build --emit`, below): a `bench_test.go` next to the generated
code can call the router with `httptest` against a file database and run
under `go test -bench . -cpuprofile`.

The repository's fixture `testdata/bench/rows` (a page of N rows, a
click, a form, a redirect, an N+1 page) is what the runtime's own baselines
are measured on (`docs/perf/`); `go test ./cmd/alder` runs it once to check
the columns' shape, never their times.

### `alder migrate`

`alder migrate [dir] [--name slug] [--dry-run]` writes the next
migration of the app in `dir` (default `.`): chapter 8, "Migrations",
walks through a change. It checks the app as `alder check` does, except
that it never reports E0019 (it is what fixes it), compares the tables
with `migrations/schema.json` (an empty schema when there is no
`migrations/` yet, which it creates), and writes
`migrations/NNNN_<slug>.sql` and the new `schema.json`:

```text
$ alder migrate
wrote migrations/0004_drop_excerpt.sql
  drop column Post.excerpt
!! DROPS DATA: column Post.excerpt
updated migrations/schema.json
```

Each step is one indented line, and a step that loses data is followed
by a `!! DROPS DATA:` line. The slug is `--name`'s (lower-case letters,
digits and single underscores), or `initial` for the first migration,
or made from the steps: `create_<table>`, `drop_<table>`,
`rename_<old name>`, `add_<field>`, `drop_<field>`, `change_<table>`
for a rebuild, in lower case with `_` between words
(`publishedAt` is `published_at`), each once, joined with `_` while the
whole stays within 40 characters. `--dry-run` prints the file it would
write, followed by `dry run: migrations/NNNN_<slug>.sql not written`,
and writes nothing. With no change since the last migration it prints
`no changes since NNNN_<slug>` and writes nothing, except that a change
of a default alone (defaults are not in the database) is recorded in
`schema.json`: `updated migrations/schema.json (defaults only)`.

`schema.json` also records each table's `silo` (M15). A table that the
migrations create in the shared database and `App` now lists in a silo
(or one moved out of a silo, or to another) is refused, since a
migration changes one database's tables and does not move rows between
databases (chapter 8, "Moving a table into a silo"):
``table Activity moves into `silo org`, but the migrations create it in
the shared database, and a migration does not move its rows into a
database per org (book 8, "Silos"): when no database holds rows of it
yet, `alder new silo org Activity --empty` declares it, writes the
migration that creates it in the silos made before and says what it
leaves behind; otherwise keep Activity shared``.

Exit 0 when it wrote a migration or found no change. Exit 1 for check
errors (printed as `alder check` prints them), for a change no migration
can make (the reason on standard output after `alder migrate: `, such as
``Post.views: a new non-null column needs a default (`= 0`) so existing
rows can be filled``), and for a `migrations/` directory that disagrees
with itself, on standard error:

```text
alder migrate: migrations/schema.json does not match 0002_add_bio.sql's result; restore migrations/schema.json from version control, or delete the last migration file and run `alder migrate` again (only if it has not run against a database you keep)
alder migrate: migrations/ has migration files but no schema.json, the schema they lead to: restore it from version control
alder migrate: migrations/schema.json has no migration file: restore the migrations from version control, or delete schema.json to start again from 0001_initial
```

It also refuses, with `alder check`'s E0019 text, files that do not
chain (a middle file deleted: ``migration 0003_add_site does not follow
0001_initial: a file is missing or edited; restore migrations/ from
version control (`git checkout migrations/`)``) and two files with one
number, rather than plan a migration on top of them.

It never changes an existing migration file. Exit 2 for a usage error
or a file that cannot be read or written.

### `alder deploy` and the commands around it

Chapter 16 is the full account; here are the usage lines and exit codes.

```text
alder deploy <env> [dir] [--host name] [--allow-data-loss] [--unsafe-skip-tests] [--dry-run] [--json]
alder status <env> [dir] [--host name]
alder rollback <env> [<build id>] [--dir dir] [--host name] [--allow-schema-mismatch | --restore-backup [--yes]]
alder logs <env> [dir] [--host name] [-f] [-n N] [--since T] [--grep PATTERN]
alder restore <env> [--silo key:id] [--from url] [--to time] [--out file.db] [--dir dir] [--yes]
alder migrate silos <env> [dir] [--host name] [--allow-data-loss]
alder silos prune <env> [dir] [--host name] [--yes]
alder ci init [dir] [--github] [--force]
```

`alder deploy` reads App's `deploy` declaration in `dir` (default `.`),
runs the gate (`check`, `build`, `test`, `test --mutate` against the
floor, the declared `checks`), writes the artifact
`build/<app>/deploy/<build id>/`, and ships it to the environment's
target. `--dry-run` stops after the migration plan; `--json` prints the
progress on standard error and the deploy record on standard output;
`--host` serves another host than the declared `domain` for one run (a
pull request's preview). Exit 0 when deployed (or the dry run passed);
1 for a refusal (the gate, a `requires` name the target does not set, a
migration that drops data without `--allow-data-loss`, a failed health
check, another run holding the environment's lock), an unreachable
target or a script that exits non-zero; 2 for a usage error, a missing
declaration or environment.

The `Fly` target is **experimental** (it has never run against Fly
itself) and refuses a deploy that would apply a migration to an
existing database: "Fly target: migrations are not supported yet;
deploy a build with no pending migrations, or use Ssh".

`alder status`, `rollback` and `logs` read the state at the target (exit
2 for a `Script` environment, which keeps its own). `alder rollback
--restore-backup` also puts back the copy of the database taken when
that build was replaced, losing every write since; it asks on the
terminal, `--yes` answers without one, and an unconfirmed run changes
nothing (exit 2). `alder restore` rebuilds the database from the bucket
at a time and puts it in place (asking the same way), or with `--out`
writes it to a file. `alder ci init` writes the GitHub
Actions workflow; it refuses to replace one without `--force`, and runs
`actionlint` on it when that is installed (exit 1 when it finds a
problem).

**Silos** (M15; chapter 8, "Silos"; chapter 16, "Silos at a box").
`alder status` of an app with a `silo` adds a table from the current
build's `--silo-status` at the target: per key, how many silos are `ok`,
`migrating` (behind, migrated when next opened), `pending (data loss)`,
`ahead`, `failed` and `orphaned`, then one line for each that is not
`ok`, with why. `alder migrate silos <env>` runs the current build's
`--migrate-silos` at the target, under the deploy lock: every silo is
opened and checked now, rather than on its next request; a silo behind
by a migration that drops data is migrated only with
`--allow-data-loss`; exit 1 when any silo is still not served, the lines
above saying which and why. `alder silos prune <env>` lists the orphaned
silos (their key's row was deleted, so no request can reach them), asks,
and removes them (`--yes` answers without a terminal; unconfirmed, it
removes nothing, exit 2). `alder restore <env> --silo org:<id> --to
<time>` puts one org's database back as it was at that time, the others
untouched. These run against an `Ssh` environment (the `Turso` database
included, chapter 16); exit 2 for an app without a `silo` or another
target.

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
| `alder` | A module or declarations: `alder check` is clean in a copy of `examples/todos` (declarations see every example module; a module replaces the example's module of that name; declarations with exactly one `table T` are checked as `module T`, since a table is declared in its own module, replacing the example's `T` if it has one, and a block with two tables is marked `fragment`). A table of `Name.x : Type` lines: each name's real type is at least as general as the table's (a `sig : Type` / `sig = Name.x` checks). Expressions: they parse, each starting where `alder eval` would start one (not type-checked) |
| `alder eval=<value>` | `alder eval examples/todos -` prints exactly `<value>` (the rest of the fence line) |
| `alder error=<CODE> "<text>"` | compiling (or, for expressions, evaluating) the block reports diagnostic `<CODE>` in the block itself, its message containing `<text>` (optional) |
| `alder part=<Module>` | one piece of a module shown piece by piece (the tour's `Todos.Index`): a chapter's `part=` blocks of one module, consecutive among its `part=` blocks, are joined in order and checked as that module, replacing the example's; when the example has that module, the joined pieces must also equal its file, blank lines aside |
| ```` ```text command=<state> ```` | a transcript (chapter 15): the first line is `$ alder …`, run in the state's app (`guestbook`, a copy of the scaffold, `notes`, a copy of a bare app (`alder new notes --bare`), or an example's name, a copy of `examples/<name>`), which the chapter's commands change in order; a line ending in `<<'EOF'` takes the lines up to `EOF` as standard input; the rest of the block must be what the command prints, standard output then standard error, a line `…` standing for any lines and a `…` inside a line for any text within it, such as a build id (`TestBook_Commands`) |
| `alder written=<state>/<path>` (any language) | the file at `<path>` in the state's app as the commands before it left it, byte for byte |
| `alder part=<example>/<Module>` | the same for a module of another example app, `examples/<example>` (chapter 14's `part=blog/Posts.Index`): the joined pieces are checked as that module in a copy of that example, and must equal its file, blank lines aside. `part=<Module>` alone means `part=todos/<Module>`; every other block is checked against `examples/todos` |
| `alder fragment` | not checked: a deliberate excerpt, such as one arm of an `update`; listed by `go test -v ./book`. Chapter 16's `deploy { … }` fragments are the exception: each is appended to the scaffold's `App` and must check (`TestBook_DeployDeclaration`) |
| ```` ```yaml golden=<path> ```` | the block is, byte for byte, the file at `<path>` (chapter 16 quotes the workflow `alder ci init` writes from its golden file: `TestBook_CIGolden`) |

A failure names the block's line, `book/03-syntax.md:120: E0003 …`, so a
change to the language that leaves the book behind fails the build like
any other test.

### The compiler's own tests

A change to the compiler is checked with `gofmt -l .`, `go vet ./...` (and
`go vet -tags browser ./...`), and

```
go test ./... -count=1 -timeout 30m
```

The `-timeout 30m` matters: `cmd/alder`'s tests build and run whole apps,
and on a loaded machine the package takes longer than `go test`'s default
limit of ten minutes, which then fails it with a panic that looks like a
hang. CI passes a like flag to each of its test parts (`.github/workflows/ci.yml`,
`.github/shard.sh`; 28 minutes for each, inside a job of 30).

That command runs everything except the mutation quality gates, the tests
that mutate an example app or the admin and demand that no mutant survive
(`TestAdmin_Columns`, `TestAdmin_Mutate`, `TestExample_TodosMutation`, and the
mutation half of `TestExample_Blog` and `TestExample_Workspace`). They take
the longest of all, so they run only with `ALDER_MUTATION_GATES=1`
(`cmd/alder/gates_test.go`). CI runs them in a job of its own, `mutation`,
every night (07:00 UTC), on a manual run and on a tag. `-short` skips the
slowest of the rest as well: the out-of-tree builds, the drill and the twelve
deploy end to ends. The formatter's two wall-clock tests (`TestSpeed_Examples`,
`TestRender_Performance`) run only with `ALDER_SPEED_TESTS=1`, on a quiet
machine: a time limit fails on a busy one whatever the code does. What always
runs in their place counts allocations as the input grows, in length and in
nesting depth (`TestScaling_Source`, `TestScaling_Render`,
`TestScaling_Depth`), which no other load changes.

The suite is slow to run in one piece, so `scripts/test.sh [go test flags]`
runs it as CI does, in parallel: `cmd/alder` in four shards and every other
package in a fifth part, each logging to a file, with the tail of any
failing part printed at the end. `ALDER_TEST_SHARDS` changes the number of
shards (more is slower on a laptop, since each shard's builds already use
every core). On the author's machine the full suite takes about three
minutes this way, `scripts/test.sh -short` about two and a half, where the
old serial run took nineteen.

Those tests compile many Go programs, and each run's are new. The go
command keeps every compiled package in its build cache (`go env
GOCACHE`) until it has gone unused for five days, with no size limit, so
through your own cache a few days of test runs would fill hundreds of
gigabytes. The packages whose tests run the go command (`cmd/alder`,
`internal/gen`, `internal/buildkit`, `internal/scratch` and `book`) point
it instead at a cache of their own, shared by every run so a warm run
stays fast: `$TMPDIR/alder-test-gocache` (`/tmp/alder-test-gocache` when
`TMPDIR` is unset), or the absolute path in `ALDER_TEST_GOCACHE`. Before
the tests start, a cache over 10 GiB (`ALDER_TEST_GOCACHE_MAX_GB` changes
the cap) is wiped, unless another test binary is using it at that moment.
Where `/tmp` is held in memory (tmpfs, on many Linux systems), lower the
cap or set `ALDER_TEST_GOCACHE` to a directory on disk. To clear it by
hand, `rm -rf "${TMPDIR:-/tmp}/alder-test-gocache"`. CI sets
`ALDER_TEST_GOCACHE` to the go command's own cache, since a runner is
thrown away after each job, and the cap to 1000: only test binaries take
the cache's lock, so a cache that the outer `go test` builds from at the
same time must never be over the cap, or a wipe would pull it from under
that command (`internal/testgocache`).

### The browser smoke test

Everything above runs without a browser: an app's own tests drive its
pages in memory (chapter 10), and the compiler's tests talk to `alder
run` over HTTP. What only a real browser can show, that Datastar applies
the router's answers as chapter 6 says, is checked by one more test,
which is not part of `go test ./...`:

```
go test -tags browser ./cmd/alder
```

The build tag `browser` turns on `cmd/alder/browser_test.go`. It serves
the blog with `alder run` and drives a headless Chromium through
Playwright (a browser-automation library for Node.js) with the script
`cmd/alder/testdata/browser/smoke.cjs`, checking that:

- a boosted click loads no document and changes the URL and the title;
- Back and Forward restore the scroll position each page was left at;
- the login form's `Redirect` is morphed in, with the session cookie;
- a link to a missing page morphs the 404 page in;
- a slow post (held back by the script for a second and a half) then a
  click on a link ends on the link's page, not the post's answer;
- a link to a file under `/static/` is a plain document load;
- the interactive elements written in an Alder page (`cmd/alder/testdata/browser/Smoke.alder`,
  added to the copy of the blog the test serves): `<al-menu id popover>` opens
  from its button with its roles, the arrow keys and Escape; `<al-popover
  popover>` opens from `popovertarget`; choosing a tab shows its panel only; a
  tooltip shows on focus; a `<dialog>` written inside `<al-dialog>` is one
  dialog and opens as a modal;
- in the admin, with the panel on: the Delete button opens a modal `al-dialog`
  with focus inside it, a body morph leaves it open and modal, Escape closes it
  and gives focus back; it shows the counts, "See the details" leads to the
  confirmation page, and its Delete button deletes the row;
- a 404 and a 500 (the dev database's table renamed under the running app)
  are morphed in with the panel answering not found and error, its tabs working;
- the panel's History lists the tab's requests (newest first, at most 20, kept
  across a page load), a click reopens one, and Copy as curl copies the line;
- a bulk delete's `Notice.with` shows a toast in the open region and it times
  out, and the panel after it is the delete's: its `DELETE`s, then "then GET
  /admin/user", on the tab chosen before;
- on 205 users, the admin's new-post form offers 50 authors and a search form;
  the search narrows the select, and a crafted post of an id outside it is
  refused; the Edit page of a post whose author sorts after the 50 offers that
  author first, chosen, and saving keeps it;
- a message the server refuses (another user's email) leaves the focus on the
  field, which the form's in-flight `inert` had taken away.

It takes a few seconds. It needs `node` on the `PATH` and Playwright with
its Chromium. Playwright is looked for in `$ALDER_PLAYWRIGHT_DIR`, else
`~/.cache/alder-playwright`; when it is
missing, set `ALDER_BROWSER_INSTALL=1` to have the test install it there
with `npm` (a download from the npm registry, so never without asking).
The browser itself is installed once with
`NODE_PATH=~/.cache/alder-playwright/node_modules npx playwright install chromium`.
Without either, the test is skipped and says which is missing.

### `alder build --emit`

`alder build --emit` writes the generated Go to `build/<app>/` and vets it,
and stops there: it copies nothing of the app's (it writes the runtime,
`rt/`, which the vet needs) and builds no executable, so it needs no
adapters and does not look for them. (It still writes `caps/` for an
app with capabilities, but vets only the app's package, the Go files at
the top of `build/<app>/`: `caps/` imports adapters it has not copied.) Chapter 13 walks through what it generates and why: see [How the compiler works](13-compiler.md).
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

### The runtime in `build/<app>/rt/`

The generated Go calls a library, the runtime (`alder/rt`,
`alder/rt/html`, … : chapter 13, section 8), and the generated `go.mod`
says where it is:

```
module app

go 1.27

require alder v0.0.0

replace alder => ./rt
```

`alder` is the runtime's Go module, and `./rt` is `build/<app>/rt/`,
which every build writes from a copy of the runtime the `alder` binary
carries inside itself. So a build needs the `alder` binary and Go and
nothing else: no checkout of Alder's repository, and the runtime always
matches the compiler that generated the code. `build/<app>/rt/` holds the
module's `go.mod` and `go.sum` and, under `rt/rt/`, its packages
(`build/todos/rt/rt/html/html.go` is the `rt/html` of chapter 13: an
import path is the module's name followed by the package's directory in
it). `alder test`, `alder eval` and the REPL write the same into their
own build directories (`build/<app>/scratch/test/rt/`, …).

Among the runtime's files is the design system, under `rt/al/`: the
stylesheet `al.css`, the script `al.js`, the two Geist font files under
`fonts/` with their licence `OFL.txt`, the manifest `custom-elements.json`
the checker reads, and `VERSION`, the `alder-ui` release they were copied from. They
are *vendored*: a copy of the library's files at one tag, kept in Alder's
repository and embedded in the binary, so no build fetches anything. In
the repository, `scripts/vendor-al.sh <tag>` replaces the copy with the
library's release `<tag>` (it clones `github.com/tapegram/alder-ui` at
the tag, copies the files and writes `VERSION`); a test fails if
`VERSION`, the manifest's `version` and the stylesheet's banner disagree,
and the book's tests fail if Appendix C's tables no longer match the
manifest.

Every build rewrites the directory: a file that the binary's copy does
not have is deleted, so a `build/` left by an older alder never mixes two
runtimes, and a file you edited there is put back. Edit the runtime in the
repository, never in `build/`. Because the `replace` path is relative, you
can move or copy `build/<app>/` and build it again there with `go build
./cmd`.

The runtime needs Go modules from outside: `modernc.org/sqlite`, SQLite
written in Go, and the few it uses. `build/<app>/go.sum` (and
`rt/go.sum`) list their checksums. The first build on a machine downloads
them once, through Go's module proxy, into Go's module cache, and says so
first on standard error:

```
fetching modernc.org/sqlite (first build on this machine)
```

Every build after that is offline (`GOPROXY=off alder build` succeeds)
and prints no such line. Before `go vet` and `go build`, every build runs
`go mod download` for the runtime's requirements with the network off,
which takes milliseconds when the cache has them; only when that fails
does it print the line and download (`buildkit.DownloadDeps`). When the
download fails too (no network, `GOPROXY=off`, a proxy that refuses), the
build stops with exit 1 and Go's own reason:

```
alder build: could not download the runtime's dependency modernc.org/sqlite through Go's module proxy (GOPROXY=off); the first build on a machine needs network access:
go: module lookup disabled by GOPROXY=off
```

(Go repeats its line once per module.) `alder test` and `alder eval` say
the same after their own prefix and exit 1; the REPL prints it and waits
for the next entry.

### `alder version`

```
$ alder version
alder devel+0be85a2f5d31 (go 1.27.1)
```

`alder version` prints which alder this is and the version of Go it was
built with. The version comes from what the go command records in every
binary it builds (`runtime/debug.ReadBuildInfo`, read by
`internal/version`):

- a released binary (a release archive, or Homebrew's) gives its module
  version: `alder v0.1.0 (go 1.27.1)`;
- one built in a checkout of the repository (`go build ./cmd/alder`, `go
  install ./cmd/alder`) gives `devel+` and the first twelve hex digits of
  the commit, with `-dirty` added when the checkout had uncommitted
  changes, as above;
- one built with no version control information (`go build
  -buildvcs=false`, or from a copy of the sources outside git) gives
  `(devel)`.

The same string heads the generated `app__alder.go`, so a build directory
says which compiler wrote it:

```go
// Code generated by alder devel+0be85a2f5d31; DO NOT EDIT.
```

The other generated files keep the header without a version. An argument
after `version` is a usage error (exit 2).

### Cutting a release

A version is cut by pushing a tag: `git tag v0.1.0 && git push origin
v0.1.0`. The tag starts `.github/workflows/release.yml`, which runs
GoReleaser with `.goreleaser.yaml`. It builds `cmd/alder` with
`CGO_ENABLED=0` (the SQLite driver is pure Go) for macOS and Linux on
amd64 and arm64, and for Windows on amd64 (it builds; running alder on
Windows is untested). Each binary goes into an archive named
`alder_<version>_<os>_<arch>.tar.gz` (`.zip` for Windows) with the LICENSE
and README, listed in `alder_<version>_checksums.txt`. GoReleaser then
writes the GitHub release, its notes taken from the commits since the
previous tag; pushes a Homebrew formula to `tapegram/homebrew-alder` (so
`brew install tapegram/alder/alder` works, and the formula's test runs
`alder version`); and pushes the container image
`ghcr.io/tapegram/alder:<version>`, Go's official image plus the binary,
for CI pipelines that run `alder test` and `alder build`. No `-ldflags`
are passed: the go command records the tag as the module version when it
builds at a clean tag, which is what `alder version` prints. CI
(`.github/workflows/ci.yml`) runs on Linux for every branch push that
changes code or the book, and also on macOS for `main` and tags, so every
tag is tested on both. A green push on `main` no longer includes the
mutation gates, so before tagging, check that the latest nightly run of CI
(the one with the gates) is green, or start one by hand.

Pushing the formula needs a token GitHub Actions does not have by
default: before the first tag, create the repository secret
`HOMEBREW_TAP_TOKEN`, a token allowed to write to `tapegram/homebrew-alder`.
`goreleaser release --snapshot --clean --skip=docker` builds every archive
locally into `dist/` and publishes nothing; a snapshot is not at a tag, so
its `alder version` prints `devel+<commit>`.

## Developer mode

`alder run` serves the app in developer mode (`ALDER_DEV=1`). Every page then
carries a panel, `<al-devpanel>`, a one-line bar along the bottom: the
request's method and path, how it was answered, the time and the number of SQL
statements. A tab opens it. Only developer mode has the panel, and the strip
and the Errors tab below: a production build contains none of their markup,
script or style. The panel is rendered after the page's body, in an
element the body's morph leaves alone, and filled by its own patch on every
answer, so it survives every navigation. Only the chosen tab's section is
shown (the server renders the others `hidden`, Request first), and the panel
remembers the tab chosen last, so the next answer opens on it again. Its tabs:

- Errors: first in the list, and there only while the last build failed: a
  badge counts the errors (`Errors 2`) and each is listed as `file:line:col`,
  its code and message, and the source line with the caret. A failed build
  opens the panel on this tab, unless you have minimised the panel (then its
  corner tab shows the badge); the strip's "Show errors" button opens it
  too. A good build removes the tab. With `ALDER_DEV_PANEL=0` there is no
  panel, and the button shows the list as a sheet along the bottom instead.
- Request: the path, the form fields, the parameters, the session (the record
  the app stores, as JSON) and a `curl` line, with a "Copy as curl" button that
  puts it on the clipboard. The line carries a placeholder for the session
  cookie (`<your session cookie>`; the real one is the browser's secret), the
  `Datastar-Request` header of a partial update and the form fields as
  `--data-urlencode`. A field whose name says it holds a password, a secret or
  a token is shown as `••••`, here, in the curl line and so in History.
- Trace: the effect steps the request ran, in order, with their time; a
  capability call says whether an adapter or a fake answered it. It is the
  list `alder test` prints for a test.
- SQL: each statement with its arguments, time and the rows it wrote or read.
  A statement that ran more than once is flagged as a duplicate: the same
  select run per row is the mark of a missing join or a query inside a loop.
  One statement run once per id under a single `Task.all` (a bulk action,
  each delete its own step), each time with other arguments, is labelled
  `x3 (bulk)` instead; a select per row outside a `Task.all` is still the
  flagged kind. Statements are counted per answer: the session's user
  selected by a message and again by the page it redirects to is not a
  duplicate. Five or more runs of one statement in one run of a page's
  `load` (or in what `update` itself ran) are an N+1, marked `N+1 x100`
  instead of `duplicate` (a red badge, like the Errors tab's), and the bar
  then shows `N+1` after the SQL count; the server's log gets a `warn` line
  naming the request (`GET /slow: N+1: …`, the same text as below). Each
  `load` run is counted on its own because a `Rerender` runs `load` twice in
  one request: a page whose `load` selects the same thing three times is
  not an N+1 when a click re-renders it, though the answer then holds six.
  The rule (`eff.RepeatsCut`: the recorder marks where each `load` began and
  ended, as it marks a redirect's hop) groups the statements by their text
  (the arguments are bound with `?`, so the text is the statement's shape;
  a `.in` list's `(?, ?)` and `(?, ?, ?)` count as one shape, `(?, …)`) and
  takes every group of five or more, except a group of writes (`INSERT`,
  `UPDATE`, `DELETE`) issued under one `Task.all`, the bulk action above. A
  read under a `Task.all` is not excused: `Query.all (List.map (\r -> Db.get
  Row r.id) rows)` is one round trip per row on a remote database (Turso,
  chapter 15), where each statement crosses the network. Two to four are
  left as duplicates: two records looked up by hand on one page is not a
  loop. The warning says what to do (when every run had the same
  arguments, it says instead that the same query ran N times and should run
  once):

  ```
  N+1: SELECT "id", "title", … FROM "Table__Row__Row" WHERE "id" = ? ran 100 times (100 argument sets, 3.21 ms): one Db.get per row; read them together with Db.where (Row.id.in ids), or in the page's first query
  ```

  The same rule runs in `alder test` (a `warn:` line under the test,
  chapter 10) and in `alder bench`'s `n+1` column.
- Answer: page, rerender, redirect, not found, or an error with its text.
- Patches: the events the answer sent: each region a message's answer
  patched (`#todo-7`) and `shown` (the page's new hash), or `body` when
  the whole body was morphed, or `none` when nothing changed; then title,
  history, toast and panel (chapter 5, "Partial updates").
- History: the last 20 requests of this browser tab, newest first. The panel
  keeps them in the browser (`sessionStorage`, so a full page load keeps them
  too and another tab has its own); click one to read its Request, Trace, SQL,
  Answer and Patches as they were.

The panel is yours to arrange, like the Django debug toolbar:

- **Resize.** Drag the panel's top edge (a thin handle, shown while it is
  open; the arrow keys work on it too) to set the height of the open tab, from
  120 px up to 80% of the window.
- **Minimise.** The `–` button at its right end, a click on the tab, or
  `Ctrl+Shift+D` collapses the panel to a small tab in the bottom-right corner,
  showing the last request's time and the error badge. The tab is out of the
  page's flow, so it reserves no space at the bottom of the page. Click it, or
  press `Ctrl+Shift+D` again, to bring the panel back.
- **Remembered.** Open or minimised, the height and the tab you last chose
  are kept per browser in `localStorage`, so a reload or another tab starts as
  you left it (a navigation of the app keeps them anyway: the panel is not
  morphed). When the browser refuses storage (a private window, say), the panel
  works as ever and simply forgets on reload.

A message that answers `Redirect` to a page of the app is answered with that
page in the same response (chapter 6), and the panel shows both: the
message's request, its steps and statements, then a line "then GET /…" and
the page's own, and in the bar both answers ("redirect /admin/user, then
page"). So the panel after a delete shows the `DELETE`s that ran, not only the
list that followed.

`/_dev` is the map of the running app, built from what the compiler knows and
what the server knows: the pages (path, `requires`, roles, parameters, and the
messages each declares, with links to those that need no parameter), the
tables (columns, the id, `@created`/`@updated`, unique columns, references and
what deleting does to them, the owner), the capabilities and whether an
adapter or a fake is wired, the session's fields, the migrations the database
has, the routes and the build id. `ALDER_DEV_PANEL=0` turns the panel off.
None of this is served without `ALDER_DEV=1`: no panel, no trace is recorded,
and `/_dev` is a 404.


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
| E0002 | unknown name | — (in a test step, a form of another page written unqualified, or a page's form or message written through its import's qualifier, `Login.Credentials`, gets its full path, `Auth.Login.Credentials`) |
| E0003 | type mismatch (also: a `Db.insertMine` row that names the owner column ("`owner` is set by `Db.insertMine`") or is not a record literal in the call ("`Db.insertMine` takes its row as a record literal: write the record literal in the `Db.insertMine` call (its row is `Insert Todo` without `owner`)"), `==` or a `Dict` key on a function or an effect value, a column method on a column held in a variable, `.contains` or `.startsWith` on a column that is not `Text`, `@setNull` on a field that is not a `Maybe`, a `Db` function over a type parameter, a `fake` that lacks an entry of its capability or defines one it does not have, `Test.fake` outside a `test`, on an expression, or on a capability without a `fake`, `Session` in a fake's entry; a browser step outside a `test`, on something not written in place (`Test.get p`), piped into or not applied, its argument of the wrong kind ("`Submit` carries form `NewTodo`: use `Test.submit`", "`Toggle` posts no form: use `Test.click`", "`Rename` is posted by `Save` and `Draft`: write `Test.submit (Save { id: … }) { … }`", "`Test.get` needs a record `Model`: …"), `Test.login` with a record that is not the app's session; `Doc`, `Answer` or one of its constructors outside a `test`, chapter 10, section 9); a hint with the parenthesized form, written with your own expressions, for the two precedence traps: `a && m ?? d`, which is `(a && m) ?? d`, and `x |> f == y`, which is `x |> (f == y)` | conversion such as `Text.fromInt (…)` where one exists |
| E0004 | non-exhaustive `match` | arm skeleton |
| E0005 | module name does not match its file path | — |
| E0006 | import cycle | — |
| E0007 | top-level value without a type signature | signature line |
| E0008 | wrong number of parameters or arguments, or of type arguments (`Box Int Text` for `type Box a`) | — |
| E0009 | duplicate definition (two tests of one module with the same name too, two `fake`s of one capability), a constructor declared in two imported modules, or a value named like an entry of the capability named like its module (`render` beside `capability Markdown { render … }` in module `Markdown`) | — |
| E0010 | required declaration missing (page `load`/`view`/`update`, `App` `error`/`notFound`/`login`/`session`) | — |
| E0011 | invalid declaration (unstorable table field, unusable page or form field, bad `@id`, brand constructor used outside its module, a record that contains itself by value, a delete rule such as `@cascade` on a field without `@ref`, before `@ref T`, or two on one field, a `@renamed "old"` whose old name is not a name, is the field's or table's own, or is still a field of the table) | — |
| E0012 | a type, table, form or component in a page module or `App` named like a generated type: `Params`, `Msg`, `Session`, or `<Msg>Params` for a declared `msg`; in any module, a type, table, form or union named like a Prelude type a program writes (`Query`, `Task`, `Html`, `Maybe`, `Result`, `Nav`, `List`, `Dict`, `Int`, `Float`, `Text`, `Bool`, `Unit`, `Date`, `Time`, `Never` and `Error`; not Db's `Order`, `Page`, `Form`, `Table`, `Id`, `Insert`, `Select`, `Condition`, `Column`) or like the Prelude's test types `Doc` and `Answer`, whose module's other errors wait for the rename; in any module, a constructor or `msg` named like one of `Answer`'s (`Shown`, `Rerendered`, `Redirected`, `FormRejected`, `PageMissing`, `RequestFailed`: "`PageMissing` is a constructor of the Prelude's test type `Answer`; name the constructor differently (for example `MissingLookup`)") | — |
| E0014 | two module names that differ only in case (`Ab` and `AB`): their files would be one on a case-insensitive file system | — |
| E0015 | a module whose first name segment is exactly `Route`, `Cap` or `Args` (`Cap.Mail` is refused, `Caps.Mail` and `My.Cap` are fine): the generated Go uses those prefixes for exported names | — |
| E0016 | two pages answer the same path (`/x/{id}` and `/x/{slug}`, or `/x` and `/x/`) | — |
| E0017 | a module named like a standard-library module (`Test`, `List`, `Http`, …): the stdlib's would shadow it; uses of its names are not reported separately | — |
| E0018 | a table declared in a module that does not end in its name, or a second table in a module (``a table is declared in a module named after it: `table Comment` belongs in `app/Table/Comment.alder` ``); reported once, at the declaration | — |
| E0037 | two tables with one name in different modules (`Table.Entry` and `Entry`): table names are app-wide | — |
| E0038 | a property argument or `Fuzz.any` whose type has no derivable generator (a function, an effect type, a type parameter, a union with no constructor that ends the recursion, or not written): annotate it, or write the generator and use `Fuzz.forAll` | — |
| E0039 | `Fuzz.forAll` or `Fuzz.forAllTask` anywhere but as a test's body: "`Fuzz.forAll` makes a Property, which only a test body can use" (a `Fuzzer` value may live in any module) | — |
| E0040 | something only tests may use (`Test.*`, `Doc`, `Answer`, a fake's table, a test helper) where no test helper can be: a page's `load`/`view`/`update`, `App`'s values, a fake's entry, a brand check, a field default. "`fetchText` is a test helper (it uses `Test.fetch`): only a `test` or another test helper may use it"; "`Test.fetch` is only available in a `test` or a test helper: `view` is the page's view, not a helper" (chapter 10, "Test helpers") | — |
| E0041 | a union constructor or brand (in its module and the modules importing it) named like a one-segment page module, where some module in that scope writes `Status { … }` and the constructor takes no record: "constructor `Status` (of `Types.Code`) shadows page `Status` in `Types` and the modules importing it: there `Status { … }` is the constructor, so the page cannot be linked or fetched; rename the constructor or the page module (for example `module StatusPage`)", reported at the constructor (W0011 when nothing breaks; a `msg` is always W0011) | — |
| E0042 | a `theme` declaration the rules refuse (Appendix C, "Changing a token"): outside `App` or declared twice; a field twice in one record; `light` or `dark` not a record; a name that is not an alder-ui token ("`primray` is not a token of alder-ui v0.4.2; did you mean `primary` (`--primary`)?") or a removed `--al-*` one ("`alFontMono` was removed in alder-ui v0.4: write `fontMono`"); a colour at the top level or a theme-wide token under `light`/`dark`; a value of the wrong shape for its kind, a negative length, a `var()` of anything but a colour token, or colours whose `var()`s lead back to themselves | — |
| E0019 | `migrations/` is behind the app or disagrees with itself, reported at `migrations/schema.json:1:1` by every command but `alder migrate`: the tables changed since the last migration (``the schema changed since migration 0003 (table Post: column body): run `alder migrate` ``); a storage format change (``the storage format changed (v1 → v2): this needs a rebuild migration of every table, which this `alder migrate` cannot write yet (`alder migrate --format` is planned); build with the compiler that wrote migrations/ until then``); `.sql` files but no `schema.json`; a `schema.json` or migration file that does not read; a malformed file (``migration 0002_x: line 6: a statement must end in `;` ``, a wrong first line, a header without `-- from` or `-- to`); two files with one number (``two migrations are numbered 0005 (likely two branches: 0005_a.sql, 0005_b.sql): keep the one already merged or deployed, delete the other, restore schema.json to its result, run `alder migrate` ``); a file that does not start where the one before it ends (``migration 0003_sub does not follow 0001_initial: a file is missing or edited; restore migrations/ from version control (`git checkout migrations/`)``); a last file that does not end at the snapshot (``migration 0003_sub does not lead to migrations/schema.json: a file is missing or edited; restore migrations/ from version control (`git checkout migrations/`)``) | — |
| E0020 | a `fake` declared outside the module of its capability: "`fake Stripe` must be declared in the module that declares `capability Stripe`" (chapter 10, "Faking a capability") | — |
| E0021 | a fake's table used outside a `test`, a test helper and the fake's own entries (a page's `load`, another fake's entry, a type declaration, an app table's `@ref`): "`Charge` is a fake's table: only tests and `fake Stripe` may use it" | — |
| E0022 | a table field's default that is not a literal (`count: Int = 1 + 1`): migrations write a table default into the database as SQL | — |
| E0023 | a selector written as a literal in a test's query that is not one: "`#sumary x` is not a selector: unexpected `x` after `#sumary`; `x` is not an HTML element (a custom element has a hyphen) (a selector is a tag, `#id`, `.class`, `[attr]`, `[attr=value]`, combined with spaces or `>`)"; the other hints are "tags are lower case: `h1`" and "quote the value: `[href="/x"]`" (chapter 10, "Reading the page: queries") | — |
| E0024 | an `@owner` the table cannot have (chapter 6, "Rules"): "`@owner` must mark a `@ref` column of the session's user type `User.UserId`; `price` is `Int`", "… `owner` is `User.UserId` without `@ref`", "a table has one `@owner` column: `Todo` marks `owner` and `editor`", "a field takes one `@owner`", "`@owner` needs a session with a `user` field: this app's session has none"; for `@partition k` (E0032) the texts are its own | — |
| E0025 | a scoped operation on a table without `@owner`: "`Todo` has no `@owner` column, so `Db.mine Todo` has nothing to scope by" (the operation as written: `Db.getMine Todo`, …); the generic operations: "`Todo` has no `@partition` column, so `Db.scoped Todo` has nothing to scope by", and the `mine` family on another key: "`Project` is partitioned by `org`, not by the session's `user`: `Db.mine` is for `@owner` tables; write `Db.scoped Project`" | — |
| E0026 | a scoped operation where there is no signed-in user: "`Db.mine` needs a page that `requires user`: `Public` does not", "`Db.deleteMine` needs a page that `requires user`: `App` is not a page"; through a value of another module, at the use: "`Todos.Index.recent` uses `Db.mine`, which needs a page that `requires user`: `Public` does not"; in a fake: "`Db.mine` cannot be used in a fake: a fake has no session"; for another key the text names it and the table: "`Db.scoped Project` needs a page that `requires org`: `Public` does not", "`Projects.Index.recent` uses `Db.scoped Project`, which needs a page that `requires org`: `Public` does not", and for an unscoped operation on a siloed table "`Db.delete Project` (`Project` lives in `silo org`) needs a page that `requires org`: `App` is not a page" | — |
| E0027 | a role clause the session cannot answer (`requires user, role R`): "`role` needs `user` first", "this session has no `role` field", "this session's `role` field is `Text`, not a union: write `role: Role = …` with `type Role = Reader \| Author`", "this session's `role` field is `Maybe User.Role`: the role must be a union, not `Maybe`", "`Editor` is not a constructor of this session's `role` type `User.Role` (Reader, Author, Admin)", "`role` names `Admin` twice"; a key with `via` before `user`: "`org` needs `user` first: `org` is `via Membership`, a membership of the signed-in user; write `requires user, org`" | — |
| E0028 | a custom element (a tag with a hyphen) the manifests do not vouch for (chapter 5, "Custom elements"): "unknown element `<wa-button>`: declare custom elements in `elements/custom-elements.json` (chapter 5)"; an attribute neither declared nor on the escape list: "`<al-button>` has no attribute `tone`; it has `variant`, `size`, `disabled`, `type`" ("it declares none" for an element without attributes); a quoted value on a boolean attribute: "`<al-button>` attribute `disabled` is boolean: write `disabled` or `disabled={expr}`"; a `slot` its parent does not declare: "`<al-card>` has no slot `aside`; it has `header`, `footer`, `actions`" ("it has only the default slot", "it declares no slots"); a manifest file that does not parse, at the file: "`elements/custom-elements.json`: no `modules`" or the JSON reader's reason | — |
| E0029 | a `page "…"` path the router cannot serve (chapter 6): "page path `//evil.com` has an empty segment; a path is `/` followed by segments of letters, digits, `.`, `_`, `~`, `-` or `{param}`"; likewise "has a space", "has a query (`?`)", "has a fragment (`#`)", "has the character `\`", "has the segment `..`, which a browser resolves away", "has the segment `a{id}`, which is neither a name nor a whole `{param}`". A trailing `/` is allowed. `alder new page` and `alder new admin --path` refuse the same paths | — |
| E0030 | a `deploy` declaration `alder deploy` cannot use (chapter 16): "unknown deploy target `Heroku` in environment `production`: a target is `Ssh`, `Fly` or `Script`", "environment `production` (Ssh) has no `domain`" (likewise `path`, and `app` for `Fly`), "a `requires` name is empty", "`requires` cannot name `ALDER_PUBLIC_URL`: `ALDER_` variables are the runtime's own…" (a deploy copies a required variable's local value to the target), "`floor` is 1.5, outside 0..1: it is a mutation score", "environment `production` is declared twice in `deploy`", "`deploy` is declared once", "`deploy` is only allowed in `App`"; a target's text outside its character set (an `Ssh` host, user or domain with another character or beginning with `-`, a `path` with a control character, `"`, `\`, `$`, `` ` ``, `%` or `..`, a `Fly` region that is not a three-letter code, an `app` that is not lowercase letters, digits and `-`); and the file carries no secret: "a secret belongs in the environment, not in `App`: name it in `requires`" for a text with a password in a URL (`scheme://user:pass@host`), beginning `sk_`, `ghp_`, `AKIA`, `xoxb-`, `xoxp-` or `-----BEGIN`, or of 32 or more base64 or hex characters without a space (a path is never taken for one) | — |
| E0031 | a `Task` that writes to two stores (chapter 8, "Partitions and silos"): "`Create` writes `Project` (silo `org`) and `AuditLog` (shared): a Task writes to one store. Write `AuditLog` inside the silo, or send it from its own message."; two silos: "… `Board` (silo `team`): a Task writes to one store. Send one of the writes from its own message."; the arm is named by its constructor, "the `_` arm", or "`update`" when the update is not a `match` on its message | — |
| E0032 | a partition declaration the rules refuse: "`@partition org` needs a session field `org` of type `Maybe Org.OrgId`", "`@partition org` must mark a `@ref` column; `name` is `Text` without `@ref`", "a table has one partition column: `Board` marks `org` and `owner`", "`@partition` needs a session: …"; `via`: "`via Seat`: `Seat` needs exactly one `@ref User` column and one `@ref Org` column; it has 0 and 1", "`org` is `via Membership`, which needs a session field `user`: a membership is a user's", "`user` cannot be `via` a table: …"; `choose`: "`org` is `via Membership` and `Projects` `requires org`: `App` needs `choose org = Page`, …", "`choose org = Picker`: `Picker` `requires org` itself, …", "`choose user`: the signed-in user is chosen by `login`; …" | — |
| E0033 | a `silo` the rules refuse: "`Note` is in `silo org` but is not partitioned by `org`", "`AuditLog.project` refers to `Project`, which lives in `silo org`: a shared table cannot point into a silo", "`Board.team` refers to `Team`, which lives in `silo team`: `Board` lives in `silo org`, and a table cannot point into another silo", "`Board` is in `silo org` and `silo team`: a table lives in one store", "`silo org` is declared twice: …"; the admin form `requires user, role Admin, silo org`: "`silo org` needs a role check: …", "`silo org` takes `org` from the page's parameter: add `org: Org.OrgId` to the page's parameters" | — |
| E0034 | a file page with a message: "a file page has no messages: `view` returns `File`, so nothing can be posted from it; move the message to an HTML page" (the same for a `form` and an `update`) | — |
| E0035 | a header a file page cannot set: "`Set-Cookie` cannot be set from a page; the headers a file may set are `Content-Disposition`, `Cache-Control`, `ETag`, `Last-Modified`, `Retry-After`, `Content-Language` and `X-…` names" | — |
| E0036 | a scratch file (a name ending in `scratch.alder`) under `app/` (this chapter, "`alder scratch`"): "a scratch file does not belong under app/; move it to the project root" | — |
| E0101 | `Db.get T x` where `x` is not `Id T` | `Db.from T \|> Db.where (T.<col>.eq x) \|> Db.first` when a column of `x`'s type exists |
| E0102 | field access on a `Maybe` | `match` skeleton |
| E0201 | name from a module that isn't imported | `import M` |
| E0202 | unqualified type from another module | `M.T` |
| E0203 | qualified constructor | unqualified name |
| E0204 | an import alias that is not the canonical one: missing, redundant or different (`import Todo as T`, or `import Admin.Layout` beside `Layout`) | the import line with the canonical `as` |
| E0205 | the full path of an imported module where its qualifier belongs (`Table.Todo.find`, `<Admin.Layout.Page>`, `Todos.Index.helper`) | the qualified spelling (`Todo.find`, `<AdminLayout.Page>`, `Index.helper`) |
| E0206 | an import with no usable qualifier: its whole joined path is taken, or equals another import's | — (the message suggests renaming a module) |
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
| E0603 | `[x, ...rest]`, three dots in a list pattern | `[x, ..rest]` |
| W0001 | unused import | — |
| W0002 | typed hole (`?name`) with its expected type | — |
| W0003 | a form field whose decoding errors no `<Errors field>` and no `<Errors form />` of its form would show | — |
| W0004 | a default on a field of a declared `type R = {…}` (``field `loud`: a default on a plain record type never applies; remove it``): a literal of a declared record names every field, so the default could never apply | — |
| W0005 | a form posting a page's message whose arm of `update` can answer `Reject`, but without `<Errors form />` to show the text: "`Submit` can answer `Reject`, but this form has no `<Errors form />`: the user never sees the rejection text" (chapter 10, "`Test.submitRaw`: input the types forbid") | — |
| W0006 | an adapter package reads (through `os.Getenv` or `os.LookupEnv` with a literal name) an environment variable that `deploy.requires` does not name (chapter 16): "adapter `auth` reads `SMTP_URL`, which `deploy.requires` does not name"; the runtime's own `ALDER_…` variables are never reported, and a name in `requires` that no adapter reads is fine | name it in `requires` |
| W0007 | the language server's warning on line 1 of an open scratch file when the app has errors, so the file does not run (this chapter, "Scratch files in the editor"): "the app has errors; fix them and save"; the errors themselves are on the app's files | fix the app's errors and save |
| W0008 | a GET `<form>` (`method="get"` or none, no `submit`) whose literal `action` is a page's path holds a plain `<input>`, `<select>` or `<textarea>` with a literal `name` that is one of the page's parameters declared with another wire name (chapter 6): "page `Wood` reads its parameter `electionId` from the query key `ElectionId`, so `name="electionId"` in this GET form sets nothing; use the wire name: `name="ElectionId"`" | the wire name as the `name` |
| W0010 | a module that hides a capability of its name: a call `Clarity.latest` that resolved to module `Clarity`'s `latest` while an imported `capability Clarity` has that entry ("module `Clarity` shadows `capability Clarity` of `Feeds` in this module: `Clarity.latest` here is the module's `latest`, not the capability's entry; write `Feeds.Clarity.latest` for the capability, or rename module `Clarity` (for example to `ClarityModule`)"), reported at the import; or a capability named like a standard-library module, reported at the capability. A call the module lacks is E0002 with the hint instead | — (the E0002 of a hidden call gets the capability's spelling) |
| W0011 | E0041's clash where nothing in the constructor's scope breaks (a role `Admin` beside `module Admin`, a `msg Go` beside `module Go`, or a Prelude constructor such as `Ok` naming a page): the program works, but the page cannot be linked or fetched from there | — |
| W0009 | `render.yaml` or `fly.toml` (the files of `alder deploy init`) exists in the project and App.alder has no `deploy` block (chapter 16): "render.yaml deploys whatever is pushed, with no check, migration plan or backup: add a `deploy` block to App.alder (book chapter 16)"; with both files, "render.yaml and fly.toml deploy …"; a Dockerfile alone is not reported | add a `deploy` block |

## Editor support

An editor that knows Alder tells you about a mistake while you type it,
shows the type of a name when you point at it, jumps to where a name is
declared, completes what you start, and lists the compiler's fixes and the
generators as menu entries. Every one of those answers comes from the
compiler itself; the editor does not parse Alder.

The program that sits between the two is a **language server**. A language
server is a separate process that an editor starts and talks to, in a
standard protocol (the Language Server Protocol, LSP). The editor says
"this file is open, and it now reads like this", or "what is at line 22,
column 3?"; the server answers in the editor's own vocabulary:
diagnostics, hover text, locations, completion items. Because the protocol
is standard, one server serves every editor that speaks it, and the server
needs to know nothing about how the editor draws things.

Alder's server is `alder lsp`. It reads requests on standard input, writes
answers on standard output, and runs until the editor closes the pipe. You
never start it by hand: the editor does, once per project (the directory
with `app/` in it), and every `.alder` file of the project shares that one
server. Chapter 13, section 10, shows how it works inside.

### What you get

The examples use the blog (`examples/blog`), file `app/Posts/Index.alder`.

- **Diagnostics while you type.** Each change to the buffer, saved or not,
  is checked as `alder check` would check it a moment after you stop
  typing, and the errors and warnings appear on their lines with the
  compiler's own message and code. A typed hole (W0002) is shown as a hint,
  not a warning. The buffer's text replaces the file's; everything else is
  read from disk.
- `<leader>at` runs `alder test`; failures land in the quickfix list at their `test` line,
  compile errors as diagnostics. `<leader>aT` runs the test under the cursor (the module's
  tests when none is above it).
- `<leader>am` runs `alder test --mutate` in the background; the survivors land in the
  quickfix list at the mutated expression, `before → after`.
- `<leader>ax` opens `scratch.alder` (made from the starter file when it is missing);
  `<leader>as` and `<leader>ao` work in it ("Scratch files in the editor", below).
- **Hover** (`K` in Neovim). On `lastPage` it shows the signature and the
  comment above the declaration:

  ```text
  lastPage : Int

  lastPage bounds `page`, so `(page - 1) * perPage` cannot overflow.
  ```

  On a local such as `q` it shows its type, on a hole `?count` the type the
  hole must have, on a module name in an `import` the file and how many
  declarations the module has, and on a standard HTML tag or attribute its
  description. On a page's `load`, `view` or `update`, hover also says what the
  hook does and what it must return; on the `page` line it shows the route, the
  parameter record and the `requires` clauses.
- **Go to definition.** On a use of `lastPage` it jumps to `lastPage : Int`;
  on a local, to the `let` or the parameter that binds it; on the `title` of
  `post.title` to the field in `app/Table/Post.alder`. Neovim's own key for it is
  `<C-]>`, and many configurations also map `gd` to it. A name from the
  standard library (`Text.trim`) has no file to jump to, so the server
  answers nothing; hover still shows its signature.
- **References** (`grr`). Every use of a declaration across the whole
  program, in every module.
- **Completion.** After `Text.` the functions and types of module `Text`;
  after `params.` the fields of `Params` (`page` and `q`); after `<` the
  standard HTML tags, after `</` the tag to close, and inside a tag its
  attributes; after `import ` the modules; on a hole, the names in scope,
  those whose type fits the hole first. Completion works on the line you
  are typing even though that line does not parse yet (chapter 13, section
  10). Neovim shows the items through your completion plugin (blink.cmp,
  nvim-cmp), through its own `<C-x><C-o>`, or after you call
  `vim.lsp.completion.enable(true, client.id, buf, { autotrigger = true })`
  from an `LspAttach` autocommand.
- **Code actions** (Neovim's own `gra`). The menu lists the
  compiler's fix for a diagnostic on the cursor's line, such as ``Fix:
  module `Post` is not imported`` (the same fix that `alder check --json`
  carries, "Diagnostics" above), and then the generators that apply where
  the cursor is. A fix that is the one obvious repair comes first and is
  marked preferred: on a page hook with the wrong signature, ``Change
  signature to `Params -> Session -> Query Model` ``. The generators are
  refactor actions, after every fix. On `Posts.Index`, a page, those are `new form`, `new msg`,
  `new decl`, `new test`, `edit page` and `remove page`, each with the page
  already filled in: `new msg (page = Posts.Index)`. Choosing one asks for
  the rest of its fields (a name, say), shows the diff it would make, and
  applies it when you press `<CR>`. Choosing a generator opens the generator
  modal (below) with its fields filled from the cursor.

### Routes in the editor

The server answers one more custom request, `alder/routes`, with params `{}`: the
array `alder routes --json` prints, from the last build that type-checked, with
`uri` (a `file://` URI) and `range` (an LSP range, 0-based, over the declaration)
in place of `file`, `line` and `col`. The other fields are the same (`kind`,
`path`, `module`, `name`). The list is empty before the first good build.

### Generators in the editor

The generators (above, "The generators") run the same code in the editor as
on the command line. What differs is where the result goes. The command line
writes files. The editor's generator computes the change on a copy of your
app, checks the copy, and hands the editor the new text of every file it
touches; the editor puts that text into **buffers**. Nothing is on disk
until you save: `:w` for one file, `:wall` for all of them. Each buffer can be undone with `u`, and the preview you see before `<CR>` is
the diff of that copy.

A file the generator creates, `app/Posts/Delete.alder` say, arrives as an
unsaved buffer, and its directory is made on the first `:w`. The one thing
the editor asks before doing is deleting a file (`remove page`, `remove
table`): a buffer edit cannot express a deletion, and undo cannot bring a
deleted file back.

Two consequences look odd the first time.

First, the generator works from your buffers, not from the files on disk,
so it sees an edit you have not saved. And if you edit one of the files it
is about to change between the preview and `<CR>`, nothing is applied:
`alder: <path> changed since planning; nothing applied`.

Second, a generator that adds a table also writes its migration, and the
checker reads `migrations/` from disk, not from buffers. Until you `:wall`,
the table exists in your buffer and the migration does not, so the server
shows E0019, "the schema changed since migration …". It is the diagnostic
`alder check` would print for that state, and saving the migration makes it
go away.

A hole is just text, so you can fill it by hand: put the cursor on
`?delete`, hover to see the type it must have, ask for completion (the
names whose type fits come first), and type the expression. The server
shows the hole as a hint until the line checks.

Or let a model write it. On a hole, the code-action menu (`gra` in Neovim)
offers `Fill ?delete with AI (Task Nav)`, first, before the generators. It
runs the loop of `alder fill --ai` in the server, on your buffers: the
model's expression is checked on a copy of the app, retried with the
checker's errors up to three times, and only an expression that checks
comes back. It arrives as one edit to the buffer, which `u` undoes in one
step; nothing is saved. While it runs, the status line shows `alder:
filling ?delete : Task Nav` and each retry (`attempt 2 of 3: E0003 …`). If
you edit the buffer meanwhile, the server sends no edit and shows the
expression instead, for you to paste. When no answer checks, a warning
shows the errors and the model's last expression. One fill runs at a time.
The server takes the model from the environment the editor was started in
(`ANTHROPIC_API_KEY`, or `ALDER_FILL_COMMAND`); without either the action is
still offered, and says which variable to set. After a generator is applied
in the editor, the cursor is on the first hole it left, so the fill is one
`gra` away.

A model's expression never lands unseen. Each applied fill is shown in a
message, `alder fill ?delete: …`. The hole's type says what the expression
returns, not what it does on the way: `Task Nav` admits deleting any row.
So an expression that calls a capability (host Go, which can do anything),
makes an HTTP request, changes the session, writes raw HTML, pauses, or
writes rows not scoped to the session (`Db.insert`, `Db.update`,
`Db.delete`), itself or through a value of the app that does
(`Mail.notify → Email.send`), is applied only when you answer `Apply` to the
question the editor shows; an editor that cannot ask gets the expression
in a warning, not applied. What leaves your machine is the prompt: the
module's source with its comments and string literals, the names and types
in scope, and the standard library's signatures (the core modules always, others when named); `alder fill <address>
--prompt` prints it. `ALDER_FILL_COMMAND` must be a plain text completer,
with no tools and no permission bypass: to a tool-using agent, a comment in
your module is an instruction it may act on.

`alder fill` without `--ai` stays a command-line tool, and it is what an
agent uses: an agent writes the expression itself.

### Scratch files in the editor

A scratch file (this chapter, "`alder scratch`") is a notebook of `> expr`
lines, and in the editor you do not run it: you save it. The language server
treats every open scratch file as a document of its own and runs it on open
and on save, and again when you save another file or the app changes on disk
(an unsaved edit never runs it). Each run has two phases, both sent to the
editor as the custom notification `alder/scratch`, with a `phase`:

- **types** arrives as soon as the file has checked, before anything is built:
  every watch has its type, and the errors of the file (and the app's errors,
  as one warning on line 1) are published as ordinary diagnostics. This is
  what `:type` was in the REPL, for the whole file at once, in milliseconds.
- **values** arrives when the build and the run have finished: the value of
  every watch.

The plugin shows them as virtual text at the end of each watch's `>` line:
`: Int` first, then `⇒ 42 : Int` when the value comes. A newer save
supersedes an older run, so what you see is always the latest file. Values
are not diagnostics, because they are not problems: they would fill the
problem list and the sign column. An inline value shows at most 8 lines below
the `>` line and 2048 bytes: the lines left out are counted on a last line
(`… (3 more lines, <leader>ao to open)`), and a first line longer than 2048
bytes is cut and ends with `…`.

| Key | What it does |
|---|---|
| `<leader>ax` | opens `scratch.alder` at the project root, creating it from the starter file (`:AlderScratch` does the same) |
| `<leader>as` | sends the selection, or the current line, as a watch appended to `scratch.alder`, and saves it |
| `<leader>ao` | opens the full value of the watch under the cursor in a float, for a value cut inline |

`:AlderRepl` still opens `alder repl`, and says that it is deprecated.

Inside a scratch file the editor works as it does in an app module:

- **Hover** shows the type of a name or an expression, in a watch or in a
  definition: on `countOpen` in `> Todo.countOpen []` it shows
  `Table.Todo.countOpen : List Todo -> Int`, with the doc comment when the
  declaration has one.
- **Go to definition** jumps to the file's own definitions and locals, and
  into the app: from `Todo.countOpen` to its line in `app/Table/Todo.alder`.
  After an `in Table.Todo` header the module's own names lead there too.
- **Completion** offers the file's definitions, the app's modules and their
  members after `Todo.`, fields, tags and the rest, as in a module. It keeps
  working while the line you type does not parse yet, from the last text of
  the file that did.
- **Code actions** list the compiler's fixes for the file's diagnostics: on
  `> Table.Todo.countOpen []` the fix rewrites it to `Todo.countOpen`. The
  generators are not offered in a scratch file, since what they write belongs
  in `app/`.
- **Formatting** formats the file as `alder fmt` does (this chapter,
  "Formatting").

None of these waits for a run: they come from checking the file, which takes
milliseconds, and they see the app as it is on disk, as a run does (an unsaved
edit to an app module counts once it is saved).

**Inlay hints** are for an editor without alder.nvim. An inlay hint is a short
label the editor draws inside a line without it being part of the text (VS
Code and Helix show them; Neovim does after `vim.lsp.inlay_hint.enable()`).
The server offers one at the end of each watch, from the last run:

```text
> 1 + 1                          ⇒ 2 : Int
> List.map (\x -> x * 2)
    [1, 2]                       ⇒ [2, 4] : List Int
> Db.from Todo |> Db.count       ⇒ <Query> : Query Int
```

The hint sits after the watch's last line. A value of several lines shows its
first line and `…`, a watch that failed shows `✗` and the first line of its
error, a watch that has only checked so far shows `: Int`, and a watch you
have edited since the run has no hint until the next save. When the editor says it can refresh its hints, the
server asks it to after each phase of a run. alder.nvim draws the same results
as virtual text, so with the plugin leave inlay hints off for Alder, or each
watch shows its value twice.

Chapter 13, section 10, shows how the server runs and analyses a scratch file.

### Setting up Neovim

You need Neovim 0.11 or newer and alder.nvim:

```text
{ 'tapegram/alder.nvim', lazy = false, dependencies = { 'nvim-treesitter/nvim-treesitter' }, opts = {} }
```

(chapter 1, "Editor support", has the prerequisites). The plugin starts
`alder lsp` for the first `.alder` buffer of a project and attaches the
project's other buffers to that server. `:checkhealth alder` says what is
running, and if the server stops the plugin says `alder lsp stopped —
:checkhealth alder`. With `opts.lsp = false` the plugin runs without a
server, as before: `alder check` on save.

The plugin adds these keys to `.alder` buffers; each can be set to `false`
in `opts.keys`, and with which-key, `<leader>a` is the group "alder":

| Key | What it does |
|---|---|
| `<leader>ag` | the generator modal: search, form, live preview, `<CR>` applies, `<C-o>` reviews |
| `<leader>ac` | the open buffers' diagnostics into the quickfix list |
| `<leader>ae` | `alder explain` for the diagnostic under the cursor |
| `<leader>ar` | `alder run` |

`<leader>ag` (or `:AlderGenerate [id]`) opens the generator modal: a search
over all the generators, those that apply at the cursor first, then the
chosen generator's form beside a live preview of the diff, which updates as
you type. The form starts filled: required fields at an example (or the
first page or table, for one that must exist), so the preview shows a
change as soon as it opens. The field under the cursor says what it means
and gives an example. Flags that matter to agents rather than people
(where `new msg` puts its button, `--strict`) are left out of the form;
the command line still takes them. `<CR>` applies when every field is
valid, and `<C-o>` opens the full review, file by file. The plugin's README lists the options and the
rest of its commands. If you set `ALDER_LSP_LOG=/some/file` in Neovim's
environment, the server writes every message it receives and sends to that
file; the transcripts in chapter 13, section 10, were recorded that way.

## Holes

`?name` is a typed hole. The program compiles with holes, and `alder check`
reports each one as warning W0002 with its expected type:

```
app/Todos/Index.alder:40:18: warning[W0002]: hole ?count has type Int
```

After the diagnostics, `alder check` prints `alder check: 2 typed holes left` on standard
error, so that a script reads "holes left" rather than "build failed" (the exit code is
0, as for any warning; `--json` carries each W0002 as an object and no summary).

`alder fill --list` lists every hole with its module, declaration and
type, and `alder fill` fills one, with an expression you give it or, with
`--ai`, one a model writes (above, "The generators"). In an editor, hover
on a hole shows the type it must have, completion on it lists the names in
scope, those whose type fits first, and the code action `Fill ?name with
AI` fills it (above, "Generators in the editor").

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

The language server answers the same kinds of question for an editor: hover
is the type at a position, and references are the uses of a name (above,
"Editor support").

## Formatting

There is one format. `alder fmt` produces it, and it is deterministic: the
same program always formats to the same text, so diffs are minimal. The
rules are in chapter 3, "Style". `alder check` does not care how a file is
laid out: it answers whether the program is correct, and never rewrites
a file. Enforcing the format is `alder fmt --check`.

```
alder fmt                  rewrite every .alder file under . in place
alder fmt app/Table/Post.alder   one file, or a directory (build/ and dot-directories are skipped)
alder fmt --check          write nothing; list each file that would change
alder fmt --stdin          format standard input to standard output
```

In an app project, `alder fmt` also migrates imports. For each file it
loads the program, applies that file's E0204 and E0205 fixes (the
canonical `as`, and the qualifier where a full module path was written;
chapter 3, "Files and modules"), then formats. That is how a project
written before short qualifiers is brought over in one run, and it is
safe to repeat: the second run changes nothing. A file with unrelated
type errors is still migrated, and a file that does not parse is left as
it is. While any file of the project does not parse, the program cannot
be loaded, so no file is migrated: fmt still formats, and says so once on
stderr, `alder fmt: app/Broken.alder does not parse, so imports were not
migrated; fix it and run again`, without changing the exit code.
`--stdin` and a file outside an app project only format, and `--check`
lists a file that would be migrated as one that would be reformatted.

A scratch file (this chapter, "`alder scratch`") is formatted too, when fmt
walks the project and when you name it. It is not a module, so fmt formats it
the way the scratch runner reads it: each `> expr` watch as the right-hand
side of a definition, written back with its `> `, every other declaration as
in a module, and the `in` header kept first. Watches on consecutive lines stay
on consecutive lines, since a run of watches is how a scratch file reads;
anything else gets the blank lines a module has:

```text
-- before                        -- after
x=1                              x = 1
> x   +1
> [x,2]                          > x + 1
                                 > [x, 2]
```

A scratch file is never migrated (it is not one of the app's modules), and
one with an error the scratch runner reports first (a `>!` line, a name such
as `it1` that the watches take) is reported and left alone, exit 1, like a
file that does not parse. A file named `*scratch.alder` under `app/` is not a
scratch file and is formatted as a module.

Rewriting prints one line per file it changed, and nothing when nothing
changed. `--check` prints `<file> would be reformatted` for each file that is
not in the format. `--stdin` is for editors and agents: a program in, the
formatted program out.

| Exit | Meaning |
|---|---|
| 0 | every file is in the format (or was rewritten) |
| 1 | `--check` found a file that would change, or a file did not parse |
| 2 | usage, an I/O error, or an internal error |

A file that does not parse is not touched: its diagnostics are printed as
`alder check` prints them. Before it writes, the formatter parses its own
output and compares it with the input, comments included; if they differ
it writes nothing and exits 2 with `alder fmt: internal error: <file>:
formatting changed the program; please file an issue`. Everything the
generators write is already formatted, and the splice commands (`alder
edit`, `alder fill`, `alder remove`, `alder regen admin`) format the
whole file they changed.

In CI, run the format check before the type check:

```
alder fmt --check && alder check
```

The agent loop is the same: write, `alder fmt`, `alder check`.

### Format on save

`alder lsp` answers `textDocument/formatting`, so any editor that formats
through the language server formats Alder on save. In Neovim:

```lua
vim.api.nvim_create_autocmd('BufWritePre', {
  pattern = '*.alder',
  callback = function(args) vim.lsp.buf.format({ bufnr = args.buf, async = false }) end,
})
```

The server returns the smallest edits that make the buffer canonical, so
the cursor and marks stay where they were. Like `alder fmt` in a project,
it first migrates the file's imports (E0204 and E0205, chapter 3), so
format on save adds the missing `as` clauses and shortens full-path
references. It skips that step, and only formats, when any file of the
project does not parse. A buffer that does not parse gets no edits and no
error popup; the diagnostics already show the error.
