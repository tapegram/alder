# {{name}}

A guestbook: a visitor signs it with a name and a message, and the page lists every entry,
newest first. It was made by `alder new {{name}}`, and chapter 2 of the Alder book walks
through every file.

Run these from this directory:

    alder fmt              # rewrite the files into the one canonical format
    alder check            # type-check the app
    alder test             # run its tests
    alder test --mutate    # measure the tests: which changes to the app do they miss?
    alder build            # build the server, build/{{name}}/{{name}}
    alder run              # build, serve on http://localhost:8080, rebuild on every save

Write, then `alder fmt`, then `alder check`: `fmt` puts every file in the one canonical format and
never changes what the program means; `check` reports what is wrong. In CI, `alder fmt --check &&
alder check`. An editor can format on save through `alder lsp`.

Then grow it from the command line. Each of these writes plain Alder with its tests, and only
if the app still checks (chapter 15):

    alder new page About /about                    # a page
    alder new table Reply entry:Entry.EntryId@ref=Entry@cascade message:Text.NonEmpty
    alder new page Replies /replies --list Reply   # a page of a table's rows
    alder new admin                                # an admin of every table, at /admin

`alder new admin` needs someone to let in: a session with a logged-in `user` and a `role` whose
type has an `Admin` constructor (`role: User.Role = Member`, with `type Role = Member | Admin`),
set at login. The guestbook has no login yet; chapter 6, "Rules", shows what to add.

- `app/`: the Alder modules, one per file (`Home` is the page at `/`; its table `Entry` is in `app/Table/Entry.alder`, module `Table.Entry`, written `Entry` after `import Table.Entry`).
- `migrations/`: the database's migrations, written by `alder migrate` after a change to a table.
- `static/`: files served as they are, at `/static/…`.
- `adapters/`: the Go behind the app's capabilities (none yet).
- `build/`: what `alder build` writes; not source (`.gitignore` keeps it, and the databases, out of git).
