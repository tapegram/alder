# Changelog

Every release of alder, newest first. "Breaking" lists what stops an app that compiled
with the previous release, each with what to do.

## Unreleased

### Breaking

- A session field that is a `Text.Secret` (or a `Maybe Text.Secret`) is E0048: the session cookie is signed, not encrypted, so the user can read it. Keep the secret in a table and store the row's id in the session (#254).

### Added

- `alder search <query>`: search the standard library and the app by name, by module (`Text.`) or by type (`Text -> Int`, `: List Text`, `_` as a wildcard), with `--json` (#166).
- Language server: `alder/search` and `alder/searchEdit` (search by name or type, and for a picked result the edit and the cursor), `workspace/symbol` for the app's declarations, and a "Search for <type>" action on typed holes for clients that declare `experimental.alderSearch` (#166).
- Doc comments: a comment above a declaration in the format of a summary sentence, detail paragraphs, examples indented four spaces and a `Book: chapter N, "Heading"` line. Hover shows it with the examples in a code block and the book line, completion items carry it, `alder search` shows the summary in its DOC column, and `alder search --doc` prints the details, examples and book line; `--json` results gain `summary`, `examples` and `book`. `List`, `Maybe`, `Result`, `Text`, `Int`, `Float`, `Bool`, `Dict`, `Bytes`, `Task`, `Query`, `Db`, `Session`, `Notice`, `Response`, `Html`, `Http`, `Url`, `Log`, `Prelude`, `Date`, `Time`, `Random`, `Json`, `File`, `Test` and `Fuzz` are documented: every module and declaration of the standard library has a doc (#119).

### Fixed

- The dev panel and its "Copy as curl" line mask a route's `Text.Secret` form fields and path parameters by type, not only by name (a `pin` field was shown), and a log line masks a `Text.Secret` path parameter. The `Text.Secret` docs now say what is masked and what is not: `Log`, `Json.string`, `++` and a view's text show it in clear (#254).

## v0.2.0 — 2026-11-11

### Breaking

- Routes replaced pages, messages and `Nav` (#160 and its follow-ups). Every request goes to a
  route: a `page` is a GET route, a `msg` is a write route, an update answers `Task Response`
  (`Response.refresh`, `Response.redirect`, `Response.invalid`), and `Html` takes no type
  argument. The removed syntax is E0046, which names the replacement (chapter 6 has the
  rewrite); there is no migration tool. A write route that declares `form F` is sent with
  `<form submit={R}>`: `click={R}` of it is E0045. `alder new page`, `edit page` and
  `remove page` are gone for `alder new route`, `edit route` and `remove route`; `alder new msg`
  is `alder new action` (the old name still works as an alias until the next release). In
  tests, `Answer`'s `Rerendered` is `Refreshed` (or `Rendered r`, with the route), and
  `Test.click` and `Test.submit` take the route (`Test.click (Toggle { id: id })`).
- alder-ui is v0.4 or later (the shadcn look; v0.1.0 shipped v0.3.3). The `--al-*` tokens of
  v0.3 are removed, so your own CSS that uses `var(--al-…)` stops applying, and `[data-theme]`
  and the OS-dark CSS are gone, so dark mode needs the theme script in your Layout's `<head>`.
  Appendix C, "Migrating from v0.3.x", has the steps and the table of removed tokens.
- The standard library gained `Json` (and `Bytes`, `File`, `Fuzz` and `Response`). An app module
  with one of those names is E0017: rename it, or use the standard library's.
- Imports have one canonical spelling (#23). An `import` whose `as` is missing, redundant or
  not the canonical qualifier is E0204, a name written with an imported module's full path
  instead of its qualifier is E0205, and an import with no usable qualifier is E0206. Run
  `alder fmt`, which rewrites the imports and the names (E0206 needs a rename by hand).
- An Int literal that does not fit in 64 bits is an error (E0001); it used to become
  9223372036854775807 (#81). `-9223372036854775808` is writable.
- The body of `<script>` and `<style>` is raw text, as in HTML. A body that is one `{ … }`
  interpolation, or that contains `</script` or `</style`, is E0001. Pass data to a script
  through `data-*` attributes.
- A query string a route cannot parse (a raw `;`, a bad `%` escape) answers 400 naming the
  parameter; it used to reach `load` as if the parameter were absent (#12).

### Added

- `alder lsp`: diagnostics, completion, hover, definition, references, code actions, semantic
  tokens, folding, selection ranges and the generators (`alder/plan`, `alder/preview`); fill
  typed holes with AI from the editor.
- `alder tui`: a terminal UI for a running app (requests, logs, tests, routes, data).
- New commands: `alder fmt`, `alder scratch`, `alder bench` (with N+1 warnings), `alder routes`,
  `alder theme` and the `/_dev/theme` configurator, `alder logs` filters; styled output.
- Standard library: `Json`, `File`, `Bytes`, `Response`, `Fuzz`, `Html.fragment`, `Text.hash`,
  a seeded `Random`, `Url.absolute`, `Test.setClock`.
- Property tests (`Fuzz.forAll`, shrinking, saved failures) and route mutation operators.
- Deploy to Render beside Fly (`alder deploy init render|fly`), a health endpoint, a metrics
  endpoint, an access log and request ids.
- Generators for routes (`alder new route`, `new action`) and the admin on resource paths.

### Fixed

- Migrations are listed by reading the directory, not by globbing it, so an app path with
  glob characters (`[`, `*`, `?`) builds and deploys (#172).
- E0017 says to rename the module or use the standard library's, and its explanation points
  here; a `Text` where a `Json.Value` is expected names `Json.string` and `File.text` (#175).

https://github.com/tapegram/alder/compare/v0.1.0...v0.2.0
