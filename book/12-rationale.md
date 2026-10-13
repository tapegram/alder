# 12. Design rationale

This chapter records why Alder is the way it is, and what evidence there is
for each choice. It is updated as experiments run. Decisions marked
**(untested)** are design judgment awaiting data; those marked **(tested)**
link to the experiment that informed them.

## The purpose

Alder exists to test one claim: *an AI agent produces more correct web
applications in a constrained language with a checking compiler than in a
general-purpose language.* Every feature is judged by whether it improves
agent success on a fixed benchmark of application tasks, compared with
TypeScript (Next.js) and Ruby (Rails) baselines.

The reasoning behind the claim, from the literature:

- Models do badly in languages with little training data (MultiPL-E,
  Cassano et al. 2022). A new language starts with none, so Alder must be
  readable by analogy to languages models already know, and its whole
  reference must fit in a prompt.
- Putting the grammar in the prompt helps a lot for DSLs (Grammar
  Prompting, Wang et al. 2023). The reference chapters are kept short
  for this reason.
- Constraining decoding with a type checker halves compile errors
  (Type-Constrained Code Generation, Mündler et al. 2025). A small
  grammar and a fast checker make this possible later.
- Compiler feedback loops improve results substantially (Self-Debugging,
  Chen et al. 2023). The quality of error messages matters as much as
  what is checked.

## Decisions

### Derived model, not stored model **(untested)**

Elm stores the Model in memory and `update` returns a new one. Alder
rebuilds the Model from `(route, session, database)` on every request.
Reasoning: a stored model is a cache, and cache invalidation is reasoning
across time, which agents are bad at. A derived model makes every request
reproducible from three inputs and leaves nothing to invalidate. The cost is
that UI state has to live in the URL, the session, or the database.

### `page` and `msg` are keywords, not library calls **(untested)**

So the compiler can check that every link targets a real page with the
right parameters, every form posts to a real message, every message is
handled, and every page has `load` and `view`. In a library these would be
runtime failures.

### Messages have no path **(tested: E3)**

Six runs across E1–E3 declared the same POST path on two pages (`/logout`,
`/theme`, `/notes/{id}/delete`). A POST URL is never typed, bookmarked, or
shared, so letting the author choose it is a decision with no benefit and
a collision risk. `msg Delete { id: NoteId }` declares a message; the
compiler derives its URL from the page and message name. Pages keep their
paths, because GET URLs are part of the product.

### Messages belong to pages, plus app-level messages **(tested: E1)**

A `msg` is declared in the page module whose `update` handles it, and the
page's parameters travel with it. This gives `update` the same Model that
`view` had, which is what makes `Rerender` work with no extra state.

E1 showed that "a page can only send its own messages" has one common
exception: actions that belong to no page, like logging out. Every model
hit this and each worked around it differently (a raw HTML form, a link to
the login page, a duplicated msg that collided on its route). App-level
`msg` in `App` with a Model-less `App.update` closes the gap without
letting pages send each other's messages. E2 confirmed the fix (Haiku,
which had collided on `/logout` in E1, used `App.Logout` correctly) and
added two refinements: `App.update` may `Rerender` the sending page (a
theme switch should stay on the page it was clicked on; the server
answers it with a redirect back to that page, chapter 6), and app-level messages type-check
inside any `Html a`, so a shared layout can carry the buttons.

### Form state is in the request, not the Model **(untested)**

Invalid submissions never reach `update`. The runtime re-renders the page
with values and errors attached to the form, and `<Input>`/`<Errors>` read
them. This removes a `draft` field from every Model and an `invalid` handler
from every page. It also means `view` cannot see a form's state, which may
turn out to be too restrictive.

### Effect blocks with `let x <- e` **(untested)**

`Query` and `Task` need sequencing. Options were `andThen` chains (Elm),
`let x <- e` (Haskell do, Lean, Idris), and implicit applicative records.
`let x <-` was chosen because it reads top-to-bottom, needs no new
operator, and models know `<-` from Haskell. `andThen` is deliberately
absent so there is one way.

### Effects are data, interpreted eagerly **(untested)**

`load` and `update` return a description of effects, and only the runtime
performs them (chapter 7). The reasons: a page view provably cannot write,
because a `Query` has no write steps and the runtime refuses one if it
ever appears; every effect of a request goes through one place, so one
transaction per `Task` and one trace of every step come for free; and the
same `update` can be run against a test database, or a stub capability,
without changing a line.

The description is a tree of Go values (`rt/eff/eff.go`), and the
interpreter `eff.Run` walks it **eagerly**, node by node: each step runs
as soon as it is reached, and the rest of the block is built from its
result. The alternatives were a fully static plan (every step known before
anything runs) and a batching or parallel interpreter. A static plan
cannot express `let found <- Db.get Todo id` followed by a `match found`
that decides the next step, which is the most common shape of an
`update`. Batching and parallelism would make the order of steps, and so
the trace, depend on the scheduler. Eager, one-at-a-time interpretation is
the simplest model that keeps the trace deterministic and the order of
effects the order in the source.

The cost is that an effect value contains Go closures, so two effects
can't be compared with `==`. Tests compare traces instead (chapter 10).

### Constructors are unqualified **(tested: E1)**

Every model, and the hand-written example, wrote `Normal` and `[All,
Active, Completed]` without a module prefix, while qualifying the *types*
(`Todo.Priority`). The book now says so. Constructor names must be unique
across a module's imports.

### `view` receives the session **(tested: E2)**

In E2-T4 (a per-user theme), every model wanted the session in `view`:
Sonnet copied `theme` into every Model, Haiku hard-coded `theme={Light}`,
and Qwen3-Coder wrote `session.theme` inside `view`, which did not
type-check. `view : Model -> Session -> Html Msg` removes the boilerplate
and the error.

### Brands with implicit widening **(untested)**

Elm's opaque types require unwrapping at every use, which agents forget
or work around. A brand that widens to its base type costs nothing to use
and still prevents a `Text` being passed where a `TodoId` is required.
Equality is restricted to identical types because `id == text` is almost
always a bug.

### No typeclasses, no aliases, no tuples, no `andThen` **(untested)**

Each of these adds a second way to write something that already has a
way. The expected benefit is fewer choices per line for an agent, and a
faster compiler. The expected cost is some verbosity (`Text.fromInt`
everywhere).

### Braces and commas, not indentation **(untested, experiment planned)**

Indentation-sensitive syntax is fragile under line-based edits and hard to
constrain in decoding. Braces make every block's extent explicit.

### `match`, not `case of` **(untested, experiment planned)**

Rust's `match x { P -> e, }` has more training data than Elm's `case x of`
and its braces fit the rest of the syntax.

### JSX-style HTML, not `div [] []` **(untested, experiment planned)**

Models have seen far more JSX than Elm HTML. The cost is a second
expression grammar inside the language.

### Mandatory annotations **(untested)**

Every file is readable alone. It also makes the checker bidirectional,
which is what lets brand widening and record-literal typing be cheap.

### Go as compiler language and target **(untested)**

One toolchain. Datastar is Go-first. Static binaries. Emitting a typed
language forces an honest IR. The compiler runs `go build` only after our
own checks pass, so the agent's edit-check loop never waits on it.

### A REPL for people, `alder eval` for agents **(untested)**

Alder is meant to be written by agents, and an agent does not need a
REPL. It cannot see a prompt or type into a session, and it would gain
nothing from one. What it needs is one command that takes the whole input
and answers in a form it can parse. That is `alder eval`: `let` lines and
an expression on standard input, `value : Type` or diagnostics out,
`--json` for a program, and no state between calls to drift out of date.
The book verifier uses it the same way.

The REPL (`alder repl`, chapter 11) is for the person reading the
agent's work. They want to try `Todo.countOpen` on a list, see what a
query returns from the dev database, or insert a row and look at the
page again. So the REPL has what eval leaves out, and no more: `let`s
that last, `:run!` to commit a task's writes, `:html`, and a session
that re-checks when the agent edits a file. It has no `--json` and no
line editing, since a person can run it under `rlwrap`. Both commands are
one scratch runner, so the REPL cannot disagree with `eval`, or with the
compiler.

### Ids stay generated; a slug is a `@unique` column **(tested: the id probe, the blog)**

A blog wants `/posts/hello-world`, and one way to get it is a primary key
the program chooses. A probe app, written on 2026-10-10 before the blog,
did everything a chosen key would make easier, with generated ids. It
created a post and its first comment in one task, redirected by slug,
commented on the post `load` had found, toggled a post, deleted one with
its children, and tested both with chosen ids (a pure test) and with
returned ids (an effect test). It checked, built and passed with no
friction. The probe showed why. A row never exists before its insert,
so the id `Db.insert` returns is simply the next step's value. Addresses
use the slug, a second column with `@unique` and a refined brand
(chapter 14's `Post`). The blog is built the same way. `Admin` checks
`Post.bySlug` before it inserts, so a taken slug is a `Reject`, not a
500, and then redirects to the new id's edit page. A user-chosen `@id`
is declined, unless an experiment shows agents fumbling the lookup.

### Delete rules are declared on the referencing column **(tested: the blog)**

What deleting a referenced row does is `@cascade`, `@restrict` (the
default) or `@setNull` on the `@ref` field (chapter 8). It is not an
option chosen at each `Db.delete`. Then the table that holds the
reference is the one place that says what happens, whichever page
deletes, and SQLite carries the rule out inside the delete statement.
The blog's `Comment.post` is `@ref Post @cascade`. `Admin.Edit`'s
`Delete` is one `Db.delete Post`, and its test checks that the trace has
that one step and that no comment is left. `@restrict` is the default
because it loses nothing: a delete the author did not think through
fails, and leaves no row without the row it refers to.

### Search is `LIKE`-based **(tested: the blog)**

`.contains` and `.startsWith` on `Text` columns are SQL's `LIKE`, with
`%`, `_` and `\` escaped so that they match themselves, and they ignore
the case of A–Z (chapter 8). That is enough for the blog's search, which
matches the title or the body (`Db.or`). The blog's tests check that a
match in either column counts, that the case of A–Z does not matter,
and that `100%` does not find `1000 ways`. Full-text search (SQLite's
FTS5: ranking, stemming, accents) is left until the question of which
database engine the language targets is settled, because a portable
`LIKE` keeps that choice open. Until then, a capability over the same
database can do more (chapter 8, "Beyond CRUD").

### No defaulted record literals **(decided M4d, kept in M5)**

A record literal names every field: a session literal in a test, a
`Post` built by hand in a pure test (the blog's `Post` tests write
`created: Time.epoch`), an argument record. Column defaults are a
different thing. `Db.insert` may leave out a column declared with a
default (`published: Bool = False`, `role: Role = Reader`), because the
table declares that value. The stance is the user's (2026-10-10):
implicit values cause mistakes, since a value nobody wrote is one nobody
checked. Made precise in M5b: defaults apply at boundaries, where a value
comes from outside the program's literals: a session cookie that lacks a
field, a page or message parameter left out of a link or URL, a form
field left out of a request, and an `Insert T` record (that is what a
table's defaults are for). A literal of a record type itself, a table's
row, a declared `type R = {…}`, or App's or a page's `Session`, names
every field (E0003: "a default applies only to an insert, the session
cookie, a page or message parameter, or a form, not to a literal"); only
an anonymous record type's defaults (a component's props) apply to its
literals, and a default on a declared `type R = {…}` field, which could
never apply, is warning W0004. M5 needed nothing more. M5e adds one
boundary: the record `Test.login` stores is the cookie's value, so the
session's defaults apply to its literal (`Test.login { user: Just me }`
when the session also has `visits: Int = 0`); a session literal anywhere
else in a test still names every field. The blog's tests write their records in
full, and E7's evidence is under "Open questions" below. This is
revisited only if required fields prove demonstrably worse.

### Migrations are generated from the tables and versioned **(tested: both examples, the blog's worked change)**

The `table` declarations are the source of truth, as they were before
migrations; a migration is derived from a change to them, never written
by hand. `alder migrate` diffs the declarations against the snapshot of
the last migration and writes the SQL, so the schema is not stated twice
and an agent cannot write a migration that disagrees with the program.
The files are numbered and committed: a database records which it has
had, and every copy of the app, a teammate's dev database or production,
goes through the same steps. Generating is a command a person runs and
reads; applying is automatic, at start, because an app that serves
before its tables match would fail on its first query. (Data
migrations, changing values with Alder code, and changes made without
downtime are not covered: a capability in Go can do the first.)

**Renames are declared, never guessed** (`@renamed "old"`). A removed
field next to an added one of the same type may be a rename or two
unrelated changes; guessing wrong either loses data or keeps the wrong
data. The attribute costs one line, and once the migration exists it
may stay or go: the snapshot already has the new name.

**A step that loses data is marked, never silent.** A dropped table or
column, a conversion that cuts values (`Float` to `Int`), a removed
constructor: each is generated, because refusing would leave the app
unable to change, but each is printed with `!!` and written in the
file's header as `DROPS DATA`, so the person reading the file before the
commit sees it.

**Tables ahead of their migrations do not check** (E0019). Every
command but `alder migrate` refuses such an app, so a forgotten
migration is a compile error, not a server that refuses to start in
production. Tests run through the migrations for the same reason: a
passing `alder test` shows the files make the tables the program uses.

**Exact text, so some additions rebuild.** A new non-null field with a
default could be `ADD COLUMN … DEFAULT 0`, but SQLite would then store a
`CREATE TABLE` text with a `DEFAULT` the app's own statement lacks, and
the start-up check compares those texts. The check was kept exact, and
such a field rebuilds the table: slower on a large table, never a
false alarm.

### A fake is a whole capability, with tables **(tested: the blog, the Stripe-shaped fake of chapter 10)**

A test replaces a capability with a `fake`: an implementation of every
entry, written in Alder beside the capability, with tables of its own in
the test's database (chapter 10, "Faking a capability"). The stance is
the user's: a fake acts as a real implementation would, just in memory.
So a fake answers the way the service does for any sequence of calls,
not the one a test expected. A charge it made can be refunded, a refund
of a charge it never made is refused, and a test reads back what the
service holds with `Db`, as it reads the app's tables.

Per-call mocks (a test listing "when `charge` is called with 250, answer
`Ok "ch_1"`") were the other shape. They were declined. A mock answers only the calls its
test predicted, and must be written again in every test that reaches
the capability. Each test's list of answers then states what the service
does a second time, where it can disagree with the service and with the
other tests' lists. A test that checks which calls were made also
breaks when the code makes the same calls in another order or makes
fewer of them, though it still does the right thing. A fake is written
once, beside its capability, and every test that installs it gets the
same behaviour; what a test checks is the result and the rows, and the
trace when the calls themselves matter.

The rest follows from that choice. A fake's tables use the database the
test already has, so its writes are part of the test's transaction and a
failing `Test.run` undoes them like any other. Fakes, like the tests
themselves, are compiled only into test builds (`alder test`'s, and
those of the developer tools `alder eval` and `alder repl`), so no
production binary contains one, and a fake's tables are kept out of the
schema and the migrations. A fake
lives beside its capability (E0020), so there is one place to look for
it and the module that owns the capability owns the fake as well.

### Tests drive the app in memory, not a real browser **(decided M5e; tested: both examples)**

A test can use the app as a user does: get a page, submit its form,
click its button, and read the page and the session that follow
(chapter 10, "Testing through the browser"). When this was designed,
the question was put directly: why not drive a real browser, with
Playwright, as many web projects do?

Because of where the bugs are. In a server-rendered app the costly ones
sit at the seams between the code the compiler checks: a route that
reads its parameter wrongly, a form that decodes into the wrong field,
a `requires` that is missing, an ownership check that `load` makes and
`update` forgets, a redirect to the wrong page, a session write that
does not stick, a page re-rendered with stale data. Each shows only
across a request and the next. Covering them needs many tests, one per
seam and case, and a real browser makes each test slow (a browser
process, a server, a page load: seconds per test) and flaky (timing,
animations, a port in use). Teams that test that way keep few such
tests, so most seams stay untested.

An in-process browser covers the same seams. It sends the app's real
routes real requests, in memory: the same router, decoders, `requires`
checks, `load`, `update`, rendering and signed cookie as production, with
nothing between the test and the code but Go function calls. Each
request runs inside the test's transaction, so it sees the test's rows
and needs no second database. A step takes well under a millisecond (the
to-do app's browser tests take about half a millisecond each, fresh
database included, by `alder test --json`'s `ms`), the result is the
same on every run, and an app can have thousands of such tests.

What the choices inside it were:

- **Typed results and the document.** A step gives the page's `model`
  (typed, the very value `view` was given) or the `answer`, the session,
  and the parsed page. A test asserts on whichever says what it means:
  data on the model, what the user sees on the document.
- **Log in directly or through the form.** `Test.login` sets the session
  from a value, for the tests that are about something else; the login
  page is tested through its real form with a fake of its capability.
- **A click applies the answer.** After a step the browser holds what a
  real one would show (the re-rendered body, the redirect's target), and
  the raw answer is in the result too.
- **Forms by name, typed, and a raw variant.** `Test.submit` encodes a
  form's record as its inputs would and sends only the fields the page's
  form has an input for, so a test cannot send what a form could not;
  `Test.submitRaw` sends text, for the validation tests.
- **Only what the page shows.** A test can submit or click only a
  control the page it holds renders. This is what makes it a browser
  test rather than a request: a stranger's test cannot press a button
  the stranger's page does not have (chapter 10's ownership example),
  and the raw request stays the business of an `update` test.

What it leaves out is what only a real browser has: CSS, layout, the
JavaScript of a web component, and the page with JavaScript turned off.
Those are fewer and change less, and a small suite in a real browser,
planned for a later milestone, is to cover them; the many tests stay in
memory.

### Tests are measured by mutants, not line coverage **(decided M9; tested: both examples)**

`alder test --mutate` (chapter 10, "Mutation testing") measures a suite
by how many deliberately broken copies of the app it catches. Three
choices shaped it.

**A mutation score, not line coverage.** Line coverage counts the lines
that ran. In Alder nearly every line runs: the checker makes each
`match` complete, and one browser test that opens a page runs all of
its `load` and `view`. A line that runs is not a line that is checked.
The to-do app's nine tests ran every line of `Todos.Index.load` and
still let its order be reversed, and no test opened the todo page
without logging in. The score counts what the tests would catch, and
each survivor is the test that is missing, written out as module, line
and change. That is feedback a person or an agent can act on at once,
where a coverage percentage only says "more". On the examples it found
real gaps: 15 survivors in the to-do app (9 new tests, score 0.55 →
1.00) and 17 in the blog (10 new tests, 0.71 → 0.95, the 3 left
equivalent). (Since the to-do app declared its rules, M11, the same
nine tests close 14 of 32: chapter 10.)

**A curated set of operators, not every possible edit.** Generic
mutation tools change any operator or constant anywhere, and produce
thousands of mutants, many of them programs that do not compile, crash
at once, or mean nothing in particular. Alder's twenty operators are
each a bug that a server-rendered app can have and a user would notice:
a `requires` missing, an ownership check dropped, a role or an
ownership rule dropped (`role dropped`, `scope dropped`, M11), a write that does
nothing, a filter or an order wrong, a redirect that is a re-render, a
delete rule changed. Each works on the typed IR and makes a well-typed
mutant by construction, so all of them compile into one test build.
Each is named for what it breaks, so a survivor's line reads as a
sentence about the app. The to-do app has 32, the blog 58; once the
test build is made, a run of either takes about a second.

**No annotation to silence a survivor.** Many tools let a comment mark
a mutant as accepted. Alder has none. The compiler refuses the mutants
it can prove equivalent (a few structural rules, chapter 10, "What is
never a mutant"); every other survivor stays in the report. An
annotation would make the score whatever the annotations say, and an
agent under pressure to raise a score would annotate rather than test.
A legitimate survivor, like the blog's three equivalent ones, is
explained where a reader looks (`examples/blog/README.md`) and kept
visible; `--min` is then set to the score the app has reached.

`--mutate` is opt-in, and plain `alder test` is unchanged: it costs one
more build and a run of the suite per mutant, which is for CI and for
the moment a feature's tests are written, not for every edit.

### Rules are declared: `@owner` and session roles **(decided M11; tested: both examples)**

Every app with a login wrote two checks by hand: "this row is mine"
(the to-do app's `owned` helper, the blog's `post.author ==
session.user`) and "this user has role R" (the blog's `User.author`
lookup in every Admin `load`). Both are now declarations (chapter 6,
"Rules"): `@owner` on a table's column, five scoped `Db` operations, and
`requires user, role R` on a page. Five choices shaped them.

**Why declared.** A check written by hand is a check that can be left
out, and E5 and E6 showed agents leaving it out exactly when the example
did (E5: two of three T4 apps updated any id they were sent). E9's
tests mostly did kill the owner checks the tasks named (`guard dropped`
50 of 62), so the risk is less an untested check than one never
written. With the rule on
the table, the shortest code is the correct code: `Db.updateMine Todo
p.id f` is shorter than reading the row, comparing its owner and writing
it, and there is no `if` to forget or get backwards. The compiler adds
what a convention cannot: a scoped operation compiles only where a
signed-in user exists (E0026), so it cannot be used where it would mean
nothing. The rule stays explicit at each use, though: the operations
have their own names, and `Db.from` and `Db.get` keep their meaning on
an owned table. A table-wide filter that silently changed `Db.get` was
considered and rejected: an admin page that must see every row would
then need an escape hatch, and a reader could not tell from `Db.get
Todo id` which rows it can see. Page-level sugar (`owns`) is left for
later, if the operations prove too verbose.

**Why not-found.** A scoped read or write of a row that is someone
else's answers exactly as a missing row: a 404. There is then no
outcome for the page to handle and no branch to drop, so the mutation
operator has nothing to remove except the scope itself (`scope dropped`),
and a test of it is one stranger's request. It also leaks nothing: a
403 would tell a stranger that the id exists. A page that wants another
answer writes the unscoped operation and its own check, and says so.

**Why the session for roles.** A role could be read from the user's row
on every request. Keeping it in the signed cookie makes the check free
(no query) and makes it happen before `load`, in the route, where it
covers the page's messages too, so no `load` or `update` repeats it. The
cost is staleness: a role changed in the database applies at the next
login. For an author's desk that is acceptable, and the pages where it
is not have the re-read idiom, a `Db.get User session.user` in `load`
(chapter 6). The blog's test "a demoted author can edit until they log
in again" makes the trade visible rather than hiding it.

**Why never the owner column.** `Db.updateMine` drops the owner from
its `SET` list: whatever `f` returns, the row stays the user's. Without
that, `f` could hand the row to another user, and a scoped write, the
operation that exists to keep rows where they belong, would be the one
way to move them. Moving a row is a different action, and gets the
unscoped `Db.update` on a page written for it.

**The replica argument.** Alder's database is SQLite now, and may become
a replicated one (Turso: replicas that serve reads near the user, one
primary that takes writes). The rules were shaped so that nothing about
them has to change then. Scoped reads are predicates in a `SELECT`
(`WHERE "owner" = ?`), which any replica can serve. Role checks read
the cookie and no database at all. Scoped writes carry the predicate in
the statement that writes (`UPDATE … WHERE "id" = ? AND "owner" = ?`,
then a check of the rows it changed), so ownership is decided on the
primary, atomically with the write. A hand-written check reads first
and writes after; on a replica that is behind, or with another request
between the two, the read can approve a write the primary would refuse.
The predicate in the write closes that gap. Choosing a tenant (one
database per customer) is a separate, later layer.

### The runtime is embedded, not fetched **(decided M10; tested: the out-of-tree build)**

Generated code needs the runtime's source to compile (chapter 13,
section 8, "The runtime travels with the compiler"). There were two ways
to get it to a machine that has no checkout of this repository: publish
the runtime as a Go module that `go build` downloads, or carry it inside
the `alder` binary and write it next to each app. Alder carries it.

**The runtime always matches the compiler.** The generated code and the
runtime are two halves of one program: the emitter calls `rt` functions
by name, with the arguments the runtime of the same commit expects. A
downloaded runtime would be some version a `go.mod` names, and an `alder`
binary one or two releases older or newer than it would generate calls
that do not compile, or that compile and mean something else. Written
out by the binary that generated the code, the runtime is the one the
code was generated for, every time; a build directory left by another
version is refreshed, not mixed.

**Nothing depends on where the repository lives.** A fetched module is
named by its download address, and that address would be written into
every generated file. The embedded runtime keeps the name `alder`, which
no proxy is asked for, and the generated `go.mod` says `replace alder =>
./rt`. The repository can be private, renamed or moved without touching
an app.

**A build works offline.** Only the runtime's own dependency,
`modernc.org/sqlite`, comes from Go's module proxy, once per machine
(chapter 11, "The runtime in `build/<app>/rt/`"); after that, nothing
is downloaded.

The cost is small: the binary grows by about 0.4 MB, and each build
directory holds a copy of the runtime's source (about 440 KB on disk).
Go itself stays a prerequisite: `alder build` and `alder test` run `go
build`, and adapters are Go, so there is no Alder without Go installed.

## Experiments

Each experiment has a directory under `experiments/` with its prompt,
raw outputs, and scores. Summaries are recorded here.

### E1: Baseline — can agents write Alder from the reference alone?

*Done 2026-10-08.* Each model got chapters 3–10 plus the to-do example
(~11k tokens) and one of three tasks: a cross-cutting modification, a new
page, a new app. One shot, no compiler. Full scores in
`experiments/e1-baseline/scores.md`.

| Model | PASS / PARTIAL / FAIL | Errors in 3 tasks |
|---|---|---|
| Claude Opus | 2 / 1 / 0 | 1 |
| Claude Sonnet | 2 / 1 / 0 | 2 |
| Claude Haiku | 0 / 3 / 0 | 4 |
| Qwen3-Coder 30B | 0 / 1 / 2 | 12 |

What it showed:

- **Syntax was not a problem for any model.** One syntax error in twelve
  outputs. The syntax-variant experiments are deferred.
- **The Claude errors were book gaps, not model errors.** Every one was a
  case the reference didn't cover, and the models guessed. Those gaps are
  now closed: app-level messages (`App.Logout`), `<Select>` semantics,
  unqualified constructors, `Html Msg` on msg-less pages, union ordering,
  sort stability, comments in HTML, `Html.unsafe`.
- **Qwen3-Coder's errors are type errors** (unwrapped `Maybe`, `Db.get` by a
  non-id, wrong union to a function, missing import, route collision).
  These are exactly what a checker reports with a one-line fix, which is
  the argument for building the checker next. Qwen3-Coder is the model to
  measure iterations against, since the Claude models are near the ceiling
  on these tasks.

### E2: Revised book, reference-only variant, app-level messages

*Done 2026-10-08.* Same tasks on the E1-revised book, plus T4 (a theme
switch and logout on every page), with and without the example app.

| Model | Full prompt, T1–T3 errors (E1 → E2) | Reference-only |
|---|---|---|
| Claude Sonnet | — (T4 only: 1 error) | T4: 2 errors |
| Claude Haiku | 4 → 1 | T3, T4: ~11 errors |
| Qwen3-Coder 30B | 12 → 9 | structural failures, first `case ... of` |

What it showed:

- The E1 fixes held: no route collisions, `App.Logout` used correctly by
  every model, union ordering and `<Select>` used as specified.
- The example app is load-bearing for anything below Sonnet. Without it
  Haiku's errors went up tenfold and Qwen produced Elm syntax. The prompt
  always includes an example.
- T4 found three gaps, each now closed: `view` receives the session;
  `App.update` may `Rerender`; shared components are `Html a` and pages and
  app-level messages need no import.

### E3: Does a "common mistakes" appendix help?

*Done 2026-10-08.* The E2 book with and without appendix B. Haiku: 0 errors
either way (T3, T4). Qwen3-Coder: 5 errors without, 6 with (T2–T4), and it
made two of the appendix's listed mistakes verbatim. **A list of don'ts
does not change a model's habits in one shot; a checker will.** The
appendix stays for human readers and is not part of the prompt.

The E2 revisions themselves held up: like-for-like, Haiku went 4 → 0 and
Qwen 11 → 5. Qwen's remaining errors: `Db.get` by a non-id column, a field
read on a `Maybe`, and the same `msg` path on two pages. The last one is
the language's fault and is fixed below.

### E4: Path-less messages

*Done 2026-10-08.* `msg` without a path. Sonnet, Haiku and Qwen3-Coder all
used the new forms correctly on the first try; no collisions; Qwen's first
clean pass (T4). Haiku found that a message can often take no parameters
at all, since the page's params travel with it.

### Where the spike ended

46 scored runs across five models. Final from-scratch error counts (T3+T4):
Haiku 4 → 0, Qwen3-Coder 9 → 3 between E2 and E4. The remaining Qwen errors
are two habits (`Db.get` with a non-id, `.field` on a `Maybe`) that a
checker reports with a fix and that prose does not change (E3).

### E5: Checker in the loop

*Done 2026-10-08.* The first run with the compiler: `alder check --json`
after each generation, and one retry that carries the previous answer and
the diagnostics. Qwen3-Coder 30B, Haiku and Sonnet on T2–T4, E4's prompt;
errors are counted by the checker, not by hand. Full data in
`experiments/e5-loop/` (`scores.md`, `findings.md`).

| Model | Round 0 errors (T2+T3+T4) | After retry | Pass | E4 (T3+T4, hand) |
|---|---|---|---|---|
| Claude Sonnet | 0 | 0 | 3 / 3 | 0 (T3) |
| Claude Haiku | 0 | 0 | 3 / 3 | 0 |
| Qwen3-Coder 30B | 6 | 5 | 2 / 3 | 3 |

What it showed: the checker confirms E4's one-shot picture, with no false
positives, and one retry fixed Qwen's `Maybe` habit on T2 with a one-hunk
edit, but only once the retry carried the model's previous files. The
first run of this matrix fed back diagnostics alone, and the models
re-rolled instead of fixing (Haiku 1 → 3, Qwen unchanged). Qwen's T3
failure is two parse mistakes whose messages give no reason, with 15
diagnostics from three distinct mistakes, four of them introduced by the retry itself (the unqualified `Slug`), hidden behind them because a parse error stops the compiler
(`experiments/e5-loop/repros/qwen-t3-after-parse-fix/`). So
the M3 work is reporting past parse errors, plus four message fixes:
reasons for parse errors, a table declared in another table's module,
missing-import cascades, and an annotation hint for `let`-bound record
literals. The table fix did not land until M4d, after E6 and E7 met it
again: a use of a table declared in another table's module now says to
move the table to its own module (`app/Comment.alder`), where it offered
`import Post.Comment`, a module that does not exist (E5's problem 2,
closed). Still open: a parse error hides every name and type error
behind it (problem 5), which cost Qwen again in E7's T4 and T6.

### E6: Does it build?

*Done 2026-10-09.* The first run with `alder build` in the loop: a round
passes only if `alder check` is clean and `alder build --emit` then
succeeds, with up to two retries. Qwen3-Coder 30B, Haiku and Sonnet on
T2–T4 and a new T5 (shared notes; deleting another user's note by id must
answer 404), on the book at `3415cf8`, one sample per Claude cell. Full
data in `experiments/e6-build/` (`scores.md`, `findings.md`).

| Model | Round 0 errors (T2–T5) | Final errors | Pass | E5 pass (T2–T4) |
|---|---|---|---|---|
| Claude Sonnet | 1 | 0 | 4 / 4 | 3 / 3 |
| Claude Haiku | 5 | 0 | 4 / 4 | 3 / 3 |
| Qwen3-Coder 30B | 22 | 8 | 2 / 4 | 2 / 3 |

What it showed: every program that checked clean also lowered to Go that
vets (10 / 10); adapters and linking were not exercised (`--emit` stops
before them). So the build found no compiler bug. Each Claude slip (a trailing comma, a
missing `Just`) took one retry. Both of Qwen's failures trace to messages
E5 had already named: a table declared in another table's module, whose
`fix: import Post.Comment` cannot work, cost both T3 retries, and a parse
error hid seven errors in T5, after which an `unknown constructor` with no
reason led Qwen to qualify constructors. On ownership, every T5 app checks
the owner in `update`, and every T4 app now acts on the note `load`
checked, where in E5 two of three T4 apps updated and deleted any id they
were sent. The models copied the example's `owned` helper, though. What
set the pattern is the example, now backed by chapter 6's "Authorize in
`update` too"; both changed since E5, so E6 cannot separate the two, and
it does not show that agents authorize `update` unprompted. The open
question on row ownership below needs a task the example does not answer.

### The book, verified

*Measured 2026-10-09.* The book verifier (`go test ./book`, chapter 11)
runs every fenced `alder` block through the compiler: modules and
declarations are type-checked, `eval=` and `error=` expressions are
evaluated, and other expressions are parsed. The verifier's
first run, on the 78 blocks of the book as it stood, found 41 blocks that
did not compile, in 52 diagnostics. Nineteen were excerpts checked out
of their context (the tour's pieces of `Todos.Index`, a page's `load` or
`update` outside a page module, an `update` arm, `let` lines outside a
block). The tour's six pieces are now `part=` blocks, joined and
checked as the whole module; all but one of the other thirteen are marked
`fragment`, and that one is rewritten as a function that stands alone.
Three were signature tables a program cannot state: the
generic `Db` functions (`Table t`; 12 of the diagnostics) and `Http.get`'s
`HttpError`, which the checker then did not know by name. Two were the
multi-line `type Shape` / `= Circle Float` form of chapters 4 and 6,
which the grammar allows and the parser rejects. The other 17 were the
book being wrong: modules that would not compile beside the example (the
tour's `Todo` without the `filterLabel` the page uses; an `App` with an
unqualified `UserId`, no imports, and without the `SwitchTheme` its
`Layout` sends); unqualified `TodoId` and `UserId` outside their modules
(chapter 8, three blocks); elements left unclosed (three blocks), and `{…}`
children with no element around them; a component typed `Html Msg`
outside a page module; declarations and expressions mixed in one block
(two); a page skeleton with `...` for a body; a `form` with an
undeclared `Priority`; signatures with no definition (two); and chapter
10's tests calling chapter 3's `summary` and an unqualified `matches`.
Fixing them uncovered three more the first errors had hidden or the
excerpts had kept out of reach: `Todo.title.eq` takes a `Text.NonEmpty`,
not a `Text`, and two `Db.insert` records lacked the `due` and `owner`
fields an `Insert Todo` requires. Adding `eval=` values found two in the
prose: `Text.slice -3 -1 "abcdef"` and `Float.round -2.5` are type errors
(a negative argument needs parentheses). Those were the book's mistakes,
not the compiler's. The compiler's were three, first marked
`TODO(compiler)` in their blocks: the `type` line break, `HttpError`, and
a two-parameter lambda applied inline, `(\a b -> a + b) 1 2`, which
failed to infer (E0003 "expected Int, got _"), found while giving chapter
3's expressions `eval=` values. All three are now fixed: an indented `=`
may start the line after `type Shape`, `Http.HttpError` is the type's
qualified name (a bare `HttpError` is E0202 with that fix), and an inline
lambda is checked against its arguments' types; their blocks are checked
again (chapter 3's with `eval=3`). About one block in five was simply
wrong, and half did not compile as written: the case for checking the book
on every test run.

### The authorization rule, as a test

*Done 2026-10-09.* E6 showed agents copying the example's `owned`
helper, and the open question on row ownership below asks how to make
that rule harder to forget. With `alder test` (M4b) the rule is no
longer only a convention in the example: `Todos.Index` ends with the
effect test "Toggle by another user is not found" (chapter 10), which
runs `update` with a session whose user does not own the todo and
checks that the result is `Err "not found"`, that the only step was
the read, and that the row is unchanged. Removing the owner check from
`owned` now fails `alder test examples/todos`, whose report prints the
result, trace and row the test expected beside the ones it got; the
example shows
agents what a test of the rule looks like. Writing the tests also
corrected the book: chapter 7 said `Toggle` traces `["Db.update Todo"]`,
but `owned`'s read comes first, and the trace is
`["Db.get Todo", "Db.update Todo"]`. (Since M11 the rule is declared,
`owned` is gone, and `Toggle` traces `["Db.updateMine Todo"]`: the
check is in the one statement. The test now goes through the browser,
a stranger pressing my todo's button on a page still open, and a
scoped write cannot run without a signed-in test session: "Rules are
declared", above.)

### E7: prompt size, tests in the loop

*Done 2026-10-10.* Two questions in one run, all with `--build --test
--retries 2`: every task now asks for named tests, and a round passes
only if `alder test` passes too. Prompt size: Qwen3-Coder 30B on the
whole of chapters 3–10 (`full`, 32.5k tokens, run at a 64k window)
against the same with nine human-facing sections dropped (`reference`,
21.5k tokens, 32k window). T2–T5 as in E6, plus T6 (a shop's orders,
where a `Manager` may open any user's order and a `Staff` user gets 404).
One sample per Claude cell. Full data in `experiments/e7-prompt-tests/`
(`scores.md`, `findings.md`).

| Model | Prompt | Pass (T2–T6) | Round-0 pass | New tests | E6 pass (T2–T5) |
|---|---|---|---|---|---|
| Claude Sonnet | full | 5 / 5 | 3 / 5 | 19 | 4 / 4 |
| Claude Haiku | full | 5 / 5 | 3 / 5 | 24 | 4 / 4 |
| Qwen3-Coder 30B | full | 0 / 5 | 0 / 5 | — | 2 / 4 |
| Qwen3-Coder 30B | reference | 0 / 5 | 0 / 5 | — | 2 / 4 |

What it showed: a third less prompt did nothing for Qwen. No Qwen round
in either condition checked clean, so none reached the build or the
tests, and its errors fall in the same classes under both prompts, none
traceable to a dropped section. They are not invented library calls.
Two in five come from the example's `Theme` module: Qwen copies the
example's `App`, whose session has `theme: Theme = Light`, into a new app
and usually does not write `Theme.alder` (the rest are missing imports and
duplicated or over-qualified constructors), and every test that builds a
session needs `Light`. Three of Claude's four retries were the same `Light` in a
test. The rest are page and table wiring, where three messages misled
(one offers to import the module it is in, one an import that closes a
cycle). E6's 2 / 4 is not a clean control: the tests paragraph, the
book's growth and the new T6 changed together, and 39% of Qwen's errors
sit inside tests.

The test loop caught nothing: no round ever checked and built and then
failed its tests. That was not weak tests. The models wrote a test for
every rule the tasks named, mostly effect tests through `Test.run`, and
a probe run after the run, outside the measurement, that removed one
condition from each authorization rule (ten mutants over T4–T6) was
caught every time, by the test named for it.
The programs were right first. Both T6 apps put the Manager rule in
`load`, refuse a change to a Paid order with `Task.fail` and say why, in
round 0. The checker cannot see everything: Haiku's T3 retry passed
with a hard-coded password in place of the `Password` capability, after
the harness rejected its Go adapter. A model's own tests check only what it thought of; hidden
acceptance tests (E8) are the measure that can fail a program its author
believes is right.

## Open questions

- Should `view` be able to read form state (for "edit" forms that show
  the submitted values on a different page)?
- Should string interpolation accept `Int` and `Float` automatically?
  One-way says no; ergonomics says yes.
- Should payload-less unions get a derived `toText`? Three models wrote a
  label function by hand and one invented `Priority.toText`.
- Should a function body allow `let` without a `{ }` block? Haiku did this
  twice; it reads naturally but adds a second way.
- Should `Db.insert` accept a user-chosen `@id` (slug as primary key)?
  *Decided (M5): no.* Ids stay generated, and a slug is a `@unique`
  column ("Ids stay generated", above).
- Should `Task.succeed` be implicit for a final non-effect expression in
  an effect block?
- Is `Query.notFound` the right way to 404 from `load`? (`update` has the
  same in `Task.notFound`, M3c.)
- Should the language help with row ownership? *Decided (M11): yes,
  declared ("Rules are declared: `@owner` and session roles", above;
  chapter 6, "Rules").* What follows is the history that led there. The to-do example had
  `load` checking that the user owns the page's todo, but `update` acting
  on the ids its messages carried, unchecked: any user could toggle or
  delete anyone's todo by posting an id (found in the M3c review, fixed
  with an `owned` helper and `Task.notFound`, chapter 6, "Authorize in
  `update` too"). A language-level helper, such as a table marked as
  owned by a session field so that `Db.get`/`Db.update` on it check the
  owner, would make the safe way the short way. E6 found that agents
  make this mistake when the example does (two of three T4 apps in E5)
  and stop when the example and chapter 6 show the fix (none of three in
  E6, and every T5 app checks the owner in `update`), but they copied the
  example's `owned` helper. E7's T6 was the first task whose rule the
  example does not show (a `Manager` may open any user's order): both
  Claude apps put it in `load` first try and wrote tests that fail when
  it is removed, and since M4b the example's own effect test does the
  same for `owned`. For strong models that weakens the case for a
  language feature; it says nothing about weaker ones (Qwen never got
  that far). The blog (M5) is the second data point. It has a role
  (Author) checked in `load`, an owned row (a post, by `author`), and a
  second table reached through it (comments, by `post`). It needed no
  `owned` helper, because none of its messages carries an id. Each page
  names its row in its route (`/admin/{id}`, `/posts/{slug}`), `load`
  checks the role, ownership or visibility, and every message acts on
  `model.post`. A comment is reached only through a post that `load`
  found visible, and `update` refuses a comment on a draft as well. So
  the second owned table is covered by the first one's check when the
  page is the parent's page. Following chapter 6's advice (messages
  carry no id) made the safe way the short way without a language
  feature. Still open: an `Edit { id }` on a list page, where the id has
  to be in the message, and a child row with its own page.
- How large can the prompt get? Chapters 3–10 are 32.5k tokens (E7),
  past a 32k window on their own. Dropping the sections written for
  people (21.5k) did not help Qwen3-Coder 30B (E7: 0 / 5 both ways), so
  an agent-facing cut is not the lever for that model; whether a smaller
  book (chapters 3–6) or a different local model is, is not measured.
- Should a test be able to build a session without naming every field?
  The example's `theme: Theme = Light` had a default, but a session
  literal in a test still had to write `theme: Light` and import `Theme`:
  three of Claude's four retries in E7 and two in five of Qwen's errors.
  *Decided (M4d): the example changed, the language did not.* The theme
  switch left the example's skeleton for chapter 6's "App-level
  messages", where it is the worked example; the example's session is
  `{ user: Maybe User.UserId }`, and its tests write `{ user: me }`. A
  record literal, a session's included, still names every field (M5e's
  one exception: the literal `Test.login` stores in the cookie, whose
  defaults are the cookie's): no `Test.session`. Implicit
  values cause mistakes (a value nobody wrote is one nobody checked), and
  this is revisited only if required fields prove demonstrably worse. The
  one exception stays the `session` declaration's own defaults, which
  decode a cookie that lacks a field (chapter 6, "The session cookie").
- Is `Int.NonNegative * Int.Positive` an `Int`? Chapter 4's rules
  (implicit widening; operations return the base type) said yes, the
  checker said no, and both E7 T6 apps multiplied through plain `Int`s
  (one with an identity function), consistent with avoiding it.
  *Decided (M4d, 2026-10-10): chapter 4's rule 3 holds.* Two different
  brands of one base widen to it and the operation returns the base; the
  checker now agrees (`(Int.NonNegative 2) * (Int.Positive 3)` is
  `6 : Int`, chapter 4), and `Cents + Cents` stays `Cents`. Ordering
  follows rule 1 the same way: `(Int.Positive 2) < (Int.NonNegative 3)`
  compares two `Int`s; only `==` needs identical types (rule 4).
  Still open: rule 3's same-brand exception lets an operation leave its
  brand's refinement behind. `(Int.Positive 2) - (Int.Positive 3)` is
  `-1 : Int.Positive`, a value no boundary would let in, and
  `Int.NonNegative - Int.NonNegative` likewise. Should `-` (or `/` and
  `%` on the standard library's refinements, or the refinements
  altogether) be excluded from the exception, so the result is the base?
- Do agents need `List.map` with index, or is `List.indexed` enough?
- Should `alder test` also run the book's `eval=` blocks? Today
  `go test ./book` runs them, and only in this repository; an app's own
  documentation has no such check. Running a project's examples as tests
  would keep them true, at the cost of a second way to write a test.
- How should a test replace a capability with a fake? A test ran with
  the app's own adapters, or with none. A test of a login wants a
  `Password` that accepts the user it made; a `with` form on a test
  (`test "x" with Password = …`) was one shape, a test-only adapter set
  chosen by name another. *Decided (M5c): a `fake` of the whole
  capability, beside it, with tables of its own, installed by
  `Test.fake`* ("A fake is a whole capability, with tables", above;
  chapter 10, section 8).
- Should `Test.run` also give the session the task left? It restores the
  session on failure and keeps it on success, but a test cannot read it,
  so `Session.put` in a login's `update` could not be checked.
  *Decided (M5e): through the browser.* Every browser step's result
  carries the session after it, so a login is tested through its form
  and `r.session.user == Just me` ("Tests drive the app in memory",
  above; chapter 10, section 9); `Test.run` is unchanged.
