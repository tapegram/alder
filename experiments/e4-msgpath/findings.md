# E4 findings

1. **Path-less `msg` was picked up by every model on the first try**, in all three forms (`msg Logout`, `msg Toggle { id: ... }`, `msg Submit form F`). No model wrote a path. Route collisions, six across E1–E3, are gone by construction.

2. **Haiku found an idiom the rule enables**: `msg AddComment form NewComment` with no parameters, reading `model.post.id` in `update`, because the page's own params travel with every message. That's less to declare and nothing to get wrong.

3. **qwen3-coder's first PASS** (T4). Its T3 failure is the same two habits as before (`Db.get` by slug, a type used unqualified) plus one missing file.

## Spike conclusion

Four experiments, 46 scored runs, five models. The book went through three revisions, each driven by errors several models made in common. Final like-for-like numbers on the from-scratch tasks (T3, T4):

| Model | E2 | E4 |
|---|---|---|
| Haiku | 4 errors | 0 |
| qwen3-coder:30b | 9 | 3 |

What remains for the weakest useful model is two habits a type checker reports with a one-line fix. Prose has stopped helping (E3). The next lever is `alder check`.
