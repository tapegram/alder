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
theme switch should not redirect), and app-level messages type-check
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

*Status: next.* The E2 book plus a ~200-token appendix listing the ten most
frequent E1/E2 errors, versus the same book without it. Qwen3-Coder and
Haiku, T2–T4.

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
- Is `Query.notFound` the right way to 404 from `load`?
- Do agents need `List.map` with index, or is `List.indexed` enough?
