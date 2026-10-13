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
| [1. Why Alder](01-why.md) | Purpose, principles, what Alder is not; installing; your first app with `alder new` |
| [2. A tour](02-tour.md) | The guestbook `alder new` writes, file by file; then a complete to-do app, explained |
| [3. Syntax](03-syntax.md) | Modules, declarations, expressions, blocks |
| [4. Types](04-types.md) | Records, unions, brands, refinements |
| [5. HTML](05-html.md) | Markup, components, attributes, events |
| [6. Pages, messages, forms](06-pages.md) | Routes, the request lifecycle, and the declared rules: who owns a row (`@owner`, the scoped `Db` operations) and which role a page needs |
| [7. Effects](07-effects.md) | `Query`, `Task`, capabilities |
| [8. Database](08-database.md) | Tables, reads, writes, how tables are stored, migrations |
| [9. Standard library](09-stdlib.md) | Module-by-module reference |
| [10. Testing](10-testing.md) | Tests, effect tests, fakes of capabilities, tests through an in-memory browser, mutation testing, `alder test` |
| [11. Tooling](11-tooling.md) | The compiler and its outputs |
| [12. Design rationale](12-rationale.md) | Why each decision was made, with experiment results |
| [13. How the compiler works](13-compiler.md) | Lexer to generated Go, with one small program followed through every stage |
| [14. A second app](14-second-app.md) | The blog example: a relation, an owned table and a role in the session, a capability in `load`, pages and search |
| [A. Grammar](A-grammar.md) | The formal grammar |
| [B. Common mistakes](B-pitfalls.md) | The errors seen most often in experiments |

Chapters 3–9 are the **reference**: together they are what an agent is given
as context when writing Alder. They are kept short on purpose.

## Status

Alder is a design in progress. `alder new <name>` writes a first app, the guestbook of chapter 2, which checks, tests, builds and serves; Alder installs as one binary (a release archive, Homebrew, or a container image for CI) and needs only Go beside it (chapter 1, "Installing"). Two example apps exist, `examples/todos` (chapter 2) and `examples/blog` (chapter 14), and both check, build, serve and pass their tests. `alder check`, `alder dump` and `alder build` exist: the to-do app serves, and `alder build` produces a binary that runs it over HTTP against SQLite (chapter 11). `alder run` rebuilds and reloads: save a file and the browser shows the new app, or the compiler's errors (chapter 11). `alder eval` evaluates an expression against the app (chapter 11), and `go test ./book` verifies every Alder block in this book not marked as a fragment: modules and declarations are type-checked (and the modules the tours show piece by piece must equal the examples' files, and chapter 2 must quote every file `alder new` writes, as it writes it), `eval=` and `error=` expressions are evaluated, other expressions are parsed. `alder test` runs the app's tests, each effect test against a fresh in-memory database, a test can replace a capability with a `fake` written in Alder, with tables of its own, and a test can drive the app through an in-memory browser that requests its real routes inside the test's transaction: log in, get a page, submit its forms, click its buttons, and read the page and the session that follow (chapter 10). `alder test --mutate` measures the tests: it compiles every curated, well-typed mutant of the app (a `requires` dropped, a write replaced, an order reversed, …) into one test program, runs the suite once per mutant, and prints the score and each survivor as the test that is missing; `--min R` makes the score a gate in CI (chapter 10, "Mutation testing"; chapter 13 shows how). Both examples run under it: the to-do app's 32 mutants are all killed, and the blog's 63 all but three equivalent ones. The two authorization rules every app with a login needs are declared, not written: `@owner` on a table's column gives it the scoped operations `Db.mine`, `Db.getMine`, `Db.updateMine`, `Db.deleteMine` and `Db.insertMine`, which read and write only the signed-in user's rows and answer anyone else's as not found, and `requires user, role R` serves a page only to a session with that role; both examples use them, and the mutation operators `scope dropped` and `role dropped` check that their tests notice (chapters 6, 10 and 13). `alder repl` keeps a session of `let`s and evaluates expressions, queries and tasks against the app, re-checking when its files change (chapter 11). `alder migrate` writes the app's migrations from a change to its tables, and the app applies them when it starts, keeping the data; both examples have them (chapter 8). A capability's Go adapter lives in a package named by the compiler, `adapters/<module's first segment>/` with `func <rest of module><Capability>()`; `alder build` generates the wiring and lists every missing adapter with the exact package and function to write (`alder test`, `eval` and the REPL leave a missing one unwired, with a warning, so an app can be tested with fakes first), and a production build carries no test code (chapters 7 and 11). The reference
describes the language as currently specified; the rationale chapter records
what experiments have shown and what is still open.
