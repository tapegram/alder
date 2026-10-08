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

### Messages belong to pages **(untested)**

A `msg` is declared in the page module whose `update` handles it, and the
page's parameters travel with it. This gives `update` the same Model that
`view` had, which is what makes `Rerender` work with no extra state.

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

*Status: planned.* Give each model (Claude Haiku/Sonnet/Opus, Qwen3 8B,
Qwen3-Coder 30B) the reference chapters and the to-do example, then ask
for (a) a small modification, (b) a new page, (c) a new app. Score
outputs by hand against the error list in `experiments/README.md`.

## Open questions

- Should `view` be able to read form state (for "edit" forms that show
  the submitted values on a different page)?
- Should string interpolation accept `Int` and `Float` automatically?
  One-way says no; ergonomics says yes.
- Should `Task.succeed` be implicit for a final non-effect expression in
  an effect block?
- Is `Query.notFound` the right way to 404 from `load`?
- Do agents need `List.map` with index, or is `List.indexed` enough?
