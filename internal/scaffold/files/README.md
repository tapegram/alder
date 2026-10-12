# {{name}}

A guestbook: a visitor signs it with a name and a message, and the page lists every entry,
newest first. It was made by `alder new {{name}}`, and chapter 2 of the Alder book walks
through every file.

Run these from this directory:

    alder check            # type-check the app
    alder test             # run its tests
    alder test --mutate    # measure the tests: which changes to the app do they miss?
    alder build            # build the server, build/{{name}}/{{name}}
    alder run              # build, serve on http://localhost:8080, rebuild on every save

- `app/`: the Alder modules, one per file (`Home` is the page at `/`, `Entry` its table).
- `migrations/`: the database's migrations, written by `alder migrate` after a change to a table.
- `static/`: files served as they are, at `/static/…`.
- `adapters/`: the Go behind the app's capabilities (none yet).
- `build/`: what `alder build` writes; not source (`.gitignore` keeps it, and the databases, out of git).
