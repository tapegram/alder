# E8: hidden acceptance tests

E7 asked models to write tests and they did, but a model's own tests only check
what the model thought of. E8 runs tests the model never sees, written against
names the task text fixes, after the model's last round. They measure whether
the program does what the task asked.

## Hypotheses

- **H1, behaviour.** Some programs that check, build and pass their own tests
  still fail hidden tests. The hidden pass rate per model and task, and the
  classes of failure (a behaviour missed, a name changed, an authorization
  hole), show what "the program is right" means beyond "it compiles and its
  tests pass".
- **H2, the tests paragraph.** E7's "Also write tests …" paragraph changes the
  program, not only the tests. Each task runs in two conditions: `with` (E7's
  paragraph) and `without` (the same text minus that paragraph, with no
  mention of tests anywhere). Compare the hidden pass rates and the own tests
  written.
- **H3, migrations.** A schema-change task (T7) yields a migration that loses
  no data. The harness's `--migrate` records each round's migration steps and
  any `DROPS DATA` line.

## Design

- **Five tasks.** T4 app messages, T5 ownership and T6 orders are E7's tasks
  (the same behaviour asked) with a **Names** block added. T7 drafts and T8
  notify modify the blog app. T7 is a schema change and runs with
  `--migrate`. T8 adds a fixed `Mailer` capability with a fixed `fake`.
- **Names block.** Each task text fixes every identifier a hidden test uses:
  - page modules, paths and parameters (types and defaults);
  - forms, their fields and types, and the message that posts each;
  - messages and their parameters;
  - `App`'s session, verbatim;
  - the redirect after each successful post;
  - the tables the tests insert rows into, verbatim;
  - an `id` or `class` for every element a test reads, as "the whole text of
    an element with `id="total"`". Only the attribute is fixed, never the tag
    or the nesting.
  Anything the block does not name is the model's choice. A name the model
  changes makes the hidden module fail to compile, and every hidden test
  then fails (a `naming` failure).
- **Hidden tests.** There is one module per task, `hidden/<task>/Hidden.alder`
  (`module Hidden`). Its tests use the browser of chapter 10 §9:
  - `Test.login` with the fixed session (never the login form, so no
    `Password` fake is needed);
  - `Test.get`, `Test.open`, `Test.submit`, `Test.submitRaw` and `Test.click`
    on fixed pages, forms and messages;
  - the `Doc` queries on fixed ids and classes;
  - `Db.insert` into the fixed tables, and `Db` reads of them;
  - `Test.fake Mailer` with its fixed table `Mail.Sent` (T8).

  They never read `.model` or call a page's `load` or `update`. A hidden test
  can only do what the page lets a user do: a missing form or button fails
  its step.
- **Two solutions per task.** A test that checks a detail the text does not
  fix is caught by a gate: every hidden test must pass on two solutions.
  `reference/` is one. `reference-alt/` is the other, with the same Names but
  deliberately different markup and structure: other nesting, forms instead
  of buttons, a `<table>` instead of a list, helper modules, the App controls
  in a component, a different email body. Both also pass `alder check`,
  `alder build --emit` and their own `alder test`.
- **Negative control (T7).** `reference-alt/t7-drafts` also removes
  `User.created` (a field no page uses). Its migration
  `0002_drop_created_change_post.sql` therefore has a `DROPS DATA` step, and
  its hidden tests still pass. Data loss is a property of the migration, not
  of behaviour, and the score table must show both. `reference/t7-drafts`'s
  migration (`0002_change_post.sql`, a rebuild of `Post` that adds
  `published` filled with 0) has none.

### T7's base: `base/blog-nodrafts/`

`examples/blog` already has drafts: `Post.published`, the front page filters
on it, the Publish and Unpublish buttons in `Admin.Edit`, and a draft is a 404
for readers. T7 as designed ("posts gain `published: Bool = False` …") would
change nothing there.

So T7 runs on `base/blog-nodrafts/`: the blog with drafts taken out. It has no
`published` field, every post is public, `Admin.Edit` has no Publish or
Unpublish, and the base's tests that were about drafts are removed. Its
`migrations/0001_initial.sql` was written by `alder migrate` from those
tables, and its `adapters/` are the blog's. Its own `alder check`,
`alder build --emit` and `alder test` (25 tests) pass.

Run T7 with `--base experiments/e8-hidden/base/blog-nodrafts`. T8 runs on
`--base examples/blog`.

The chapters in the prompt (3–10) and its example (todos) never show the
blog. Round 0's prompt shows the base app's files under "THE APP YOU ARE
CHANGING" (the harness, Task 2). The T7 and T8 texts refer to the blog
"shown after this task" and do not list it themselves.

## Matrix and run protocol

Conditions are `with` and `without`. Every run uses
`--build --test --retries 2 --hidden experiments/e8-hidden/hidden`; none uses
`--adapters`.

| Model | Tasks | Conditions | Samples | Runs |
|---|---|---|---|---|
| Haiku | T4–T8 | with, without | 2 (`--sample a`, `--sample b`) | 20 |
| Sonnet | T4–T8 | with, without | 2 | 20 |
| Opus | T4–T8 | with, without | 1 | 10 |
| Qwen3-Coder 30B (control) | T5, T6 | with | 1, `reference` prompt, `--num-ctx 32768` | 2 |

Additional flags by task:

| Task | Flags |
|---|---|
| T4, T5, T6 | no base. The `Password` capability's Go is not the model's; test builds run unwired. |
| T7 | `--base experiments/e8-hidden/base/blog-nodrafts --migrate` |
| T8 | `--base examples/blog` |

The task texts are `tasks/<condition>/<task>.md`. Each condition gets its own
experiment directory (`with/`, `without/`, with `tasks -> ../tasks/<cond>`, as
in E7) and is labelled with `--condition`.

Order of runs:

1. Run the Claude cells first: Haiku and Sonnet, then Opus.
2. Run Qwen last.
3. If a run is a `harness_error` or `compiler_bug`, stop and write a repro.

## Scoring

- **Own.** `outcome` and `final_pass` as in E7 (check, build, the model's own
  tests).
- **Hidden.** `hidden_pass` is `k/n` over the hidden module's tests:
  - The hidden tests run after the final round, when that round checked
    clean and built.
  - If the hidden module does not compile against the program, every hidden
    test fails, with the first diagnostic as the reason. If it crashes, the
    remaining tests fail.
  - Skipped (`—`) when the final round did not build.
- **Failure classes** (per hidden failure, in `findings.md`):
  - `naming`: a fixed name changed, so the module did not compile.
  - `behaviour`: the program does something other than what was asked.
  - `authorization`: another user's data was reachable.
  - `markup`: a fixed id or class is missing.
- **H3 (T7).** The round's `migrate` record: its steps and its `drops_data`.
  A migration with `DROPS DATA` is unsafe whatever the hidden score.

## The tasks and their hidden tests

| Task | Hidden tests |
|---|---|
| **T4** app messages | 8: the theme starts light and the list's `#theme` button switches it to dark (`body.dark`, the button's text, `session.theme`); the theme stays in the session from page to page (`Notes.Show`, `Notes.Index`); a note's page switches it there and back; a visitor switches it on `/login`; logging out from the list, and from a note's page, is `Redirected (Auth.Login {})` with `user: Nothing`, and the page then sends you to log in; another user's note is `PageMissing` and not listed; a note added on the list (`NewNote`) and edited on its page (`EditNote`), with the fixed redirects |
| **T5** ownership | 6: a visitor is sent to log in; the list (`#notes li`, `#notes .title`) shows only the user's notes; a note added through `NewNote` is the user's, and another user's list stays empty; the owner opens a note (`#title`, `#body`); another user's note is `PageMissing`; the owner deletes from the list (`Delete { id }`, `Redirected (Notes.Index {})`) |
| **T6** orders | 8: an order's page shows `#total` over two lines (`1250`) and `#status`; the list shows each of the user's orders with `.status` and `.total` and no one else's; a new order (`Create`), a line added through `AddLine { product }`, then `Pay`, each with its redirect, with no `quantity` input left after paying; a Paid order has no add-line form (`Test.has "input[name=quantity]"`) while an Open one has; `Cancel` shows `Cancelled` and removes the form; another user's order is `PageMissing` for Staff and `Shown` for a Manager; `/products` is `PageMissing` for Staff and `Shown` for a Manager (`#products .name`); a Manager adds a product (`NewProduct`) and toggles it inactive (`Toggle { id }`) |
| **T7** drafts | 6: a post created through `Admin.NewPost` is a draft, listed in `#drafts .title` and absent from `ul.posts`; the dashboard's drafts are only the author's drafts; a search, by address (`Posts.Index { q }`) or through `SearchForm`, does not find a draft; a draft's page is `PageMissing` for a visitor, a reader and another author; it is `Shown` to its author; `Admin.Publish { id }` answers `Redirected (Admin {})`, empties `#drafts`, and puts the post on the front page, in a search and on its page for a reader |
| **T8** notify | 5, each with `Test.fake Mailer`: a comment through `NewComment` is `Redirected` to the post and emails the post's author (`"Mailer.send"` in the trace; one `Mail.Sent` row, `to` the author's email, subject `New comment on Hello world`); the body contains the commenter's name and the comment; the mail goes to the author of the post commented on, not to anyone else; each comment sends one mail; a rejected comment (`submitRaw` with an empty body) sends none |

### What a hidden test cannot check

- **A message carrying another user's id (T5's Delete, T7's Publish).** The
  browser posts only what the held page renders. A stranger's page never
  shows a Delete or Publish button for someone else's row, so no browser
  test can post one. A test outside the page module cannot build the
  page's message to call `update` directly either: E0002, "`Delete` belongs
  to page `Notes.Index`". So the 404 on a forged Delete (T5) or Publish (T7)
  is asked by the text, but only the model's own tests can check it. The
  hidden tests check the visible half: the list shows only the user's rows,
  and the page of someone else's row is `PageMissing`.
- **Refusing a line on a Paid order (T6).** The same holds. The hidden test
  checks that the Paid page offers no form. Whether `update` refuses a
  forged post (`Task.fail` or `Task.notFound`) is the model's choice and its
  own test.
- **Ordering (newest first).** The test clock stands still, so rows inserted
  in one test share a `created` time and their order is not defined. No
  hidden test asserts an order.

## Layout

| Path | What it is |
|---|---|
| `tasks/with/`, `tasks/without/` | the task texts, by condition |
| `base/blog-nodrafts/` | T7's base (`app/`, `adapters/`, `migrations/`, `static/`, `go.mod`) |
| `reference/<task>/`, `reference-alt/<task>/` | the two solutions per task. T7 and T8 are complete blog copies with `migrations/` and the blog's `adapters/`; T4–T6 have only `app/`. |
| `hidden/<task>/Hidden.alder` | the hidden tests |
| `verify.sh` | the gate |

## Verify

```text
experiments/e8-hidden/verify.sh [task ...]           # ALDER=path/to/alder to skip `go run`
```

For each task and solution it copies the solution to a scratch directory and
prints one row:

| Column | What it shows |
|---|---|
| `check` | `alder check` |
| `build` | `alder build --emit` |
| `own` | the solution's own `alder test` |
| `hidden` | `alder test -run '^Hidden '` with `app/Hidden.alder` added |
| `drops` | `DROPS DATA` in `migrations/` (T7 and T8) |

It also checks the task texts:

- each `without` text is its `with` text minus the tests paragraph;
- a `without` text never says "test".

It exits 1 when anything differs from what is expected: every cell green,
`DROPS DATA` only on `reference-alt/t7-drafts`. Run it after any change to the
compiler, the blog, a reference or a hidden module.

```text
task           solution       check  build  own        hidden   drops
t4-appmsg      reference      ok     ok     6/6        8/8      -
t4-appmsg      reference-alt  ok     ok     3/3        8/8      -
t5-ownership   reference      ok     ok     5/5        6/6      -
t5-ownership   reference-alt  ok     ok     2/2        6/6      -
t6-orders      reference      ok     ok     7/7        8/8      -
t6-orders      reference-alt  ok     ok     3/3        8/8      -
t7-drafts      reference      ok     ok     30/30      6/6      no
t7-drafts      reference-alt  ok     ok     25/25      6/6      yes
t8-notify      reference      ok     ok     30/30      5/5      no
t8-notify      reference-alt  ok     ok     30/30      5/5      no
```

Mutants of the references, made by hand while writing the hidden tests, each
fail some hidden tests:

| Mutant | Hidden tests passed |
|---|---|
| T4: the theme class on `<html>` | 4/8 |
| T5: the list not filtered by owner | 4/6 |
| T6: no Manager exception | 7/8 |
| T7: drafts not filtered from the front page | 4/6 |
| T8: a different subject | 3/5 |
