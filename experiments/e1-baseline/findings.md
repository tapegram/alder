# E1 findings

## Headline

1. **Syntax is not the problem.** Across 12 outputs there was one syntax error (a comment inside HTML). No `case of`, no `let in`, no semicolons, no punning, no missing annotations, no effect misuse. The reference plus one example is enough for every model tested to produce Alder that *looks* right. The planned syntax experiments (`match` vs `case`, braces vs indentation) are lower priority than expected.

2. **The errors that remain are the ones a compiler would catch, and they cluster in the weaker model.** qwen3-coder makes type errors (unwrapped `Maybe`, wrong union, `Db.get` by a non-id); the Claude models make almost none. This supports the compiler-feedback loop as the main lever, and makes qwen3-coder the right model to measure iterations against: it has headroom.

3. **Most Claude "errors" were book gaps.** Each was something the reference didn't say, and every model guessed. These are fixed in the book as of this experiment:

| Gap | What models did | Decision |
|---|---|---|
| Cross-page messages (logout from the profile page) | Opus: raw HTML form. Sonnet: link to login page. Haiku and qwen: re-declare the msg, colliding on `/logout`. | **App-level messages.** `msg` may be declared in `App`; any page can send it; `App.update : Msg -> Session -> Task Nav`. |
| `<Select>` for a union field | `value={label}` (Opus, Sonnet), `value={union}` (qwen), no value (Haiku) | `<Select>` options are the union's constructors; `value` has the field's type. |
| Qualifying constructors from other modules | Every model (and the example) wrote `= Normal`, `[All, Active]` unqualified | Constructors are never qualified. Types still are. |
| `view` type on a page with no `msg` | Opus, Sonnet: `Html Never`. Haiku, qwen: `Html Msg`. | Always `Html Msg`; `Msg` may be empty. |
| Ordering by a union column | qwen: `Db.orderBy Todo.priority.desc` | Payload-less unions are ordered by declaration order, in `Db.orderBy` and with `<`. |
| `List.sortBy` stability | Sonnet relied on it; Opus avoided it | Documented as stable. |
| Comments inside HTML | Sonnet wrote `--` lines between elements | Allowed: a line beginning with `--` between HTML children is a comment. |
| `Html.raw` on user text | Haiku | Renamed `Html.unsafe`. The name is the only guard a one-shot model sees. |
| `<form submit={M}>` for a msg with no form | Haiku, qwen | Allowed; such a form has no fields. |

4. **One design question stays open: `@unique` violations.** Opus pre-checked slug uniqueness with a query and `Reject`ed. The alternative is `Db.insert` returning a `Result`. Pre-checking is racy but simple; deferred.

## What this changes in the plan

- E2 should re-run T1–T3 with the revised book, with **qwen3-coder as the primary subject** and Haiku as the secondary, and add a **T4 that exercises app-level messages**.
- E2 should also run a **reference-only** variant (no example app) to measure how much the example carries. If the reference alone is enough, the prompt halves.
- The syntax-variant experiments are deferred until something shows syntax errors.
- A real type checker is the next big source of signal. The errors qwen makes are exactly the ones `alder check` would report with a one-line fix.
