# E2 findings

## Headline

1. **The E1 fixes worked.** On the same three tasks, Haiku went from 4 errors to 1 and qwen3-coder from 12 to 9. Haiku used `App.Logout`, `Db.orderBy Todo.priority.desc` and `<Select value={union}>` exactly as the revised book says; nobody collided on `/logout` again.

2. **The example app matters a lot more for weaker models.** Reference-only: Sonnet lost one error class (qualification); Haiku went from 1 to ~11 errors; qwen3-coder produced the first `case ... of` seen in any run and leaked a `Todo` table into a blog. The reference alone is not enough below Sonnet. A worked example is part of the prompt, not an optional extra.

3. **T4 (theme + logout) found three more gaps, all now fixed in the book:**

| Gap | What models did | Decision |
|---|---|---|
| `view` cannot see the session | Sonnet copied `theme` into every Model; Haiku hard-coded `theme={Light}`; qwen3-coder wrote `session.theme` in `view` (type error) | `view : Model -> Session -> Html Msg` |
| `App.update` had to `Redirect` | Sonnet redirected to Index after a theme switch; Haiku and qwen returned `Rerender` anyway | `App.update` may `Rerender` the sending page |
| Shared layout needs a message type | Three runs typed a Layout component `Html Msg` outside a page module (chapter 5's own example did too) | Shared components are `Html a`; app-level msgs fit any `Html a` |
| Page values need imports | Five runs used a page without importing it | Pages and app-level msgs are global |

4. **Recurring wants, not yet acted on:**
   - A derived `toText`/label for payload-less unions (qwen invented `Todo.Priority.toText`; three models hand-wrote `priorityLabel`).
   - `let` in a function body without a `{ }` block (Haiku, twice).
   - Supplying the `@id` on insert when the id is user-chosen (Haiku made the slug the primary key).
   - `Db.get` by a non-id column (qwen, three times). The fix is a compiler message, not a language change.

## What this changes in the plan

- The prompt always includes an example app. The reference-only variant is dropped.
- E3 tests whether a short "common mistakes" appendix, distilled from E1/E2, reduces error counts for qwen3-coder and Haiku. It costs ~200 tokens and is the cheapest possible intervention.
- A type checker remains the biggest lever for qwen3-coder: 7 of its 13 full-variant errors are type errors the checker would report with a fix.
