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
`["Db.get Todo", "Db.update Todo"]`.

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
- Should `Task.succeed` be implicit for a final non-effect expression in
  an effect block?
- Is `Query.notFound` the right way to 404 from `load`? (`update` has the
  same in `Task.notFound`, M3c.)
- Should the language help with row ownership? The to-do example had
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
  that far). Still open: a second owned table reached through the first,
  and an `Edit { id }` on the list page.
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
  record literal, a session's included, still names every field: no
  defaulted fields that may be left out, no `Test.session`. Implicit
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
- How should a test replace a capability with a fake? Today a test runs
  with the app's own adapters, or with none (chapter 10, section 8). A
  test of a login wants a `Password` that accepts the user it made; a
  `with` form on a test (`test "x" with Password = …`) is one shape, a
  test-only adapter set chosen by name another.
- Should `Test.run` also give the session the task left? It restores the
  session on failure and keeps it on success, but a test cannot read it,
  so `Session.put` in a login's `update` cannot be checked yet.
