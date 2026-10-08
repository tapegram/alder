# E3 findings

## Headline

1. **The "common mistakes" appendix did nothing.** Haiku was already at zero errors without it; qwen3-coder made the same errors with it, including two that the appendix names verbatim (`Db.get` by a non-id column, `.field` on a `Maybe`). Telling a model not to do something it does by habit does not work in one shot; only a checker that reports the error will. The appendix stays in the book for human readers but is dropped from the prompt.

2. **The E2 revisions worked.** Haiku: 4 errors → 0 on T3/T4. qwen3-coder: 11 → 5 on T2–T4, with its first near-passes (T4, one error each). `view model session`, `theme={session.theme}`, `Html a` layouts, global page values: all used correctly by every run. (Caveat: the example app now contains the theme pattern, so T4 got easier.)

3. **qwen3-coder's remaining errors are two habits and one design flaw:**
   - `Db.get T x` where `x` is not the id (5 runs). A checker catches this with a one-line fix.
   - `.field` on a `Maybe` (4 runs). Same.
   - The same `msg` path declared on two pages (2 runs here, 6 across all experiments). **This one is the language's fault.** POST URLs are never seen or typed by users, so making the author choose them is a decision with no upside and a collision risk. Decision: `msg` has no path. The compiler generates the URL from the page and message name. `msg Delete { id: NoteId }`, `msg Submit form NewTodo`, `msg Logout`.

## What this changes in the plan

- The book-iteration loop has reached diminishing returns for the Claude models: Sonnet and Opus were near zero errors in E1, Haiku is at zero now. Further gains for qwen3-coder need a checker, not prose.
- E4 validates the path-less `msg` syntax (a syntax change, so it needs its own run) and then the spike is done.
- Next phase: the vertical-slice compiler. Its first deliverable should be `alder check` with the diagnostics the experiments showed matter most: wrong type to `Db.get`, `.field` on `Maybe`, missing import, unqualified type, msg sent without its params.
