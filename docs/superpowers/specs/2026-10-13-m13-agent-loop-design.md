# M13: The agent's loop — commands for E11, boosted navigation, admin filters — Design

Date: 2026-10-13. Status: approved in conversation; the M13 plan argues from
this. Related: M12 (the generators and the admin; its final review ran the
E11 drill and found the command set cannot build a login app), the user's
localhost session of 2026-10-13 ("a full page load every click … the screen
flashes"), E11 "CLI-only development" (runs after this milestone), M7b (the
interactive tier: dialogs; waits).

## Goal

Three things, one purpose: the loop an agent runs when it may only use
commands. (1) The commands E11 lacks, verified by the drill: from the
reference alone, the E9 teams app is built with `alder new/edit/fill` and
no file edit until its hidden tests pass. (2) Navigation without reloads:
same-origin links and redirects morph the page in place, with no flash
between pages. (3) The admin's remaining pieces that need no dialog:
per-column filters, a self-delete/self-demote guard, a limit for large
reference tables.

## Decisions recorded (2026-10-13)

- `alder new auth` writes a REAL password adapter (PBKDF2-SHA256 from Go's
  `crypto/pbkdf2`, 600,000 iterations, a 16-byte salt, constant-time
  compare; a `Password.hash` entry for registration), not a dev stub; the
  dev accounts the blog uses stay the blog's own.
- Boosted navigation is ON by default for every app (same-origin links and
  redirects), with an opt-out attribute `data-alder-reload` on a link and
  `Redirect` unchanged in the language (the runtime decides how it travels).

## Part 1: the commands

Every command keeps M12's contract: compute → copy → check → write, refuse-
not-overwrite, deterministic, `--json`, `--dry-run`.

- **`alder new auth [--users User] [--role Role]`**: refused when the
  session already has `user` or an `Auth` module exists. Writes
  `app/Auth/Login.alder` (the blog's shape: `capability Password { verify,
  hash }`, the fake (any user with a row; `hash` returns the text), `page
  "/login"`, `form Credentials`, `Submit` with the `Session.put` of user
  and role (when the user table has a `role` column and the session a
  `role` field), `Reject "Email or password is incorrect."`, the view, the
  two tests), edits `app/App.alder` (`session { user: Maybe User.UserId
  [, role: …] }`, `login = Auth.Login`, `msg Logout` + arm — inserted at
  fixed anchors: after `module`/imports for `session`, after the session
  for `login`, the `update` grows an arm or is created), creates the user
  table when `--users` names a table that does not exist (`id`, `email:
  Text.Email @unique`, `name: Text.NonEmpty`, `passwordHash: Text.Secret`,
  `[role: Role = <first>]`, `created`) plus its migration, and the adapter
  `adapters/auth/login_password.go` with PBKDF2 (`Verify` looks the user
  up by email through the store and compares; `Hash` derives) and its
  test. The Logout button is not placed (a layout is the author's; the
  chapter shows the line).
- **`alder new type <Module> "<Name> = A | B | …"`** (a union) and
  **`alder new type <Module> "<Name> = <base> where <pred>"`** (a brand,
  as chapter 4 writes them): appended after the module's existing type
  declarations; refused when the name exists.
- **`alder new decl <Module> <name> "<Type>"`**: `name : Type` + `name =
  ?name`; refused when the name exists; `--msg` is not needed (msgs have
  `new msg`).
- **`alder new test <Module> "<name>"`**: `test "<name>" = ?test` with
  type `Task Bool`; refused when the name exists.
- **`alder new page … --blank`**: `Model`, `load` and `view` are holes
  (`type Model = ?model`? — no: a type cannot be a hole; RULING: `Model`
  is `{ }` and `load`/`view` bodies are holes `?load : Query Model` and
  `?view : Html Msg`; the agent fills `view` first and extends `Model`
  with `alder edit model`? — out of scope: the agent edits `Model` by
  `alder fill --decl Model "{ … }"`, see next).
- **`alder fill --decl <Module>.<decl> -`**: replaces a WHOLE declaration
  body (a type alias's right-hand side, a value's body, a `load`/`view`/
  `update` body) with one parsed expression or type from stdin; the
  declaration's signature stays; refused if the result does not check.
  This is the one escape from "holes only": still typed, still checked,
  still one declaration at a time; the book says when to prefer a hole.
- **`alder new msg … --in-for <binder>`**: the button is placed inside the
  `for` whose binder is named (`{for m in model.members -> …}`), at the
  end of the loop body's element, with the message's fields filled from
  the binder where names match (`user: m.user`) and holes otherwise.
- **`alder edit page <Module> add-param <name>:<Type>[=default]`**: adds a
  page parameter (the list page's `q`, a show page's `tab`); refused if
  taken.
- **The drill as a test**: `cmd/alder/drill_test.go` replays a recorded
  command script (the teams app from the E9 reference, every command and
  every `fill` body) from `alder new teams` to `alder test` passing the
  app's own tests and the E9 hidden tests for T10, with no file edit; the
  script is the chapter's transcript. If a step needs a command this spec
  lacks, that is a finding: the plan's Task 1 adds it and says so.

## Part 2: boosted navigation and the flash

- **Runtime** (`rt/web`, `rt/datastar`): the page's `<body>` gets
  `data-on:click__capture` (or the Datastar idiom the vendored version
  supports; the implementer states it) that intercepts a click on a
  same-origin `<a href>` without `data-alder-reload`, `target`, a
  modifier key or a download, and runs `@get(href)` with
  `Datastar-Request`; the router answers a Datastar GET of a page with a
  200 SSE patch of the body (as a message answer does), a `<title>`
  patch, and a `history.pushState` script patch (`datastar.Navigate`
  gains a `push` mode); 404/500 as today under Datastar; a `Redirect`
  answer becomes a GET-and-morph with `pushState` (not
  `window.location.assign`) when the target is same-origin, a real
  navigation otherwise; `popstate` refetches the URL and morphs
  (`replaceState`); scroll to top on push, restored on pop; focus moves
  to the body (`tabindex=-1`) after a morph; the Datastar morph keeps
  `<head>` as is (the stylesheet never reloads). A cross-origin link, a
  `data-alder-reload` link, a form `GET`, a `mailto:` are untouched.
- **The flash**: the router's `withScript` also inserts `<meta
  name="color-scheme" content="light dark">`; `alder-ui` v0.1.3 adds
  `@view-transition { navigation: auto }` and re-vendors; `/static/al/`
  is served `Cache-Control: max-age=300, stale-while-revalidate=86400`
  with the ETag (the five-minute staleness window after an upgrade is
  the documented trade).
- **Tests**: `rt/testing`'s browser answers unchanged (`Test.open`/
  `Test.click` already model the morph); new router tests: a Datastar GET
  of a page returns the body patch + title + push script; a Redirect
  under Datastar to a same-origin page returns the morph of the target;
  cross-origin stays a navigation script; the opt-out attribute; the
  examples' browser tests pass unchanged; an e2e test with a real
  headless browser is NOT in scope (M7b's smoke suite) — the dev server
  is exercised by curl in `run_test.go`.
- **Book**: chapter 6 (links and navigation: what a click does now; the
  opt-out), chapter 13 (the boosted GET, the SSE answers, history),
  chapter 12 (why boost by default: server-rendered pages that feel like
  an app without client state), chapter 11 (the cache header), Appendix C
  (view transitions).

## Part 3: the admin

- **Filters**: for every Bool and union column, a filter in the list page
  (`al-select` in a filter form with `q`, GET-submitted: params gain
  `<col>: Text = ""`); the generated tests cover one filter per kind;
  the regions gain `filters`.
- **Self-guard**: the user table's admin refuses deleting the signed-in
  user's own row and demoting it (the Delete page shows "You cannot
  delete your own account."; Edit's Save with the role changed on one's
  own row is `Reject`ed); generated tests.
- **Option-load limit**: a reference select loads at most 200 rows,
  ordered by label; above that the form shows a text input for the id
  with the label looked up on submit (membership checked against the
  table); the chapter names the limit. (Autocomplete is M7b.)
- `regen` carries the new regions; the examples are regenerated (replay
  byte-for-byte).

## Book

- Chapter 15 gains the new commands with verified transcripts, the
  `--decl` rule, the auth generator's output (the adapter quoted), the
  drill as a worked example ("an app from the command line"), and the
  admin's filters and guard; chapter 7 (capabilities) points at `new
  capability`/`new auth`; chapter 14 if the blog changes (it does not:
  the blog keeps its hand-written login and dev stub; the generated auth
  is shown on the scaffold); the reference carries the new commands and
  the `--decl` rule (≤ 45k; trim if needed).

## Testing

- `internal/generate`: the anchors in `App.alder` (session/login/update
  insertion on an `App` with and without an `update`); `--decl`
  replacement spans; `--in-for` placement; the type/decl/test generators.
- `cmd/alder`: goldens per command; refusals; the drill test; the adapter
  test (hash/verify round trip; wrong password; timing-safe compare).
- `rt/web`: the boosted GET, redirects, popstate script, opt-out, cache
  header, the meta tag; `run_test.go` curl checks.
- Examples' gates; `alder new` out of tree + `alder new auth` on it →
  check/test/build/run; the book's transcripts.

## Non-goals

- Dialogs, toasts, autocomplete (M7b); bulk actions; an audit log; page
  edits beyond `add-param`; prefetching on hover; a service worker; a
  JavaScript API for apps; E11 itself (next).
