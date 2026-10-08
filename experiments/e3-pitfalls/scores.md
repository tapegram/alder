# E3 scores

Book at commit 56e87c8 (E2 revisions: `view` gets Session, global pages, `Html a` components, theme in example). Variants: **plain** = reference + example; **pitfalls** = same plus appendix B (common mistakes, ~230 words).

Caveat: the example app now contains a theme switch and `App.SwitchTheme`, so T4 is easier than in E2 regardless of the appendix. Compare plain vs pitfalls within E3; compare E2→E3 only loosely.

| Model | Variant | Task | Errors | Verdict | Notes |
|---|---|---|---|---|---|
| claude-haiku | plain | T4 notes+theme | 0 | PASS | Mirrors the example's Theme module and Layout exactly; `view model session`, `theme={session.theme}`. Sets `class="dark"` on both `<html>` and `<body>`. |
| claude-haiku | pitfalls | T4 notes+theme | 0 | PASS | Same shape as plain. Redirects to the new note after create. Drops `class="dark"` from `<body>` (keeps it on `<html>` only) — a requirement miss, not a language error. |
| qwen3-coder:30b | plain | T2 new page | 1 TYP | PARTIAL | `user: user` where `user : Maybe User` — third run in a row with this exact error. Uses a nested effect block (`let stats <- { ... Query.succeed ... }`) and `Db.count` correctly. `App.Logout`, `view model session`, `theme={session.theme}` all right. |
| claude-haiku | plain | T3 blog | 0 | PASS | Visibility check via `let isAuthor = match session.user {...}`; `Query.notFound` in both branches; slug lookup via `Db.where ... |> Db.first`. Down from 1 error in E2. |
| claude-haiku | pitfalls | T3 blog | 0 | PASS | Same shape; adds Theme module and `App.SwitchTheme` even though the task didn't ask. |
| qwen3-coder:30b | pitfalls | T2 new page | 1 TYP | PARTIAL | **Same `user: user` on a `Maybe User`**, with appendix item 2 stating exactly this. Otherwise identical to plain (flattened the nested block). |
| qwen3-coder:30b | plain | T3 blog | 2 TYP, 1 STRUCT | FAIL | `Db.get Post params.slug` twice (fourth run with this). `post: Post.Slug @ref Post` (a `@ref` must be the id type). No `User.alder` emitted though imported. `requires user` on the public post page (logic). Everything else idiomatic, including `view model session` and `theme={session.theme}`. Down from 5 errors in E2. |
| qwen3-coder:30b | pitfalls | T3 blog | 3 TYP, 1 STRUCT | FAIL | `Db.get`/`Db.update Post p.slug` (appendix item 1 says exactly this). `id: Text @id` on Comment. `post: Post.Slug @ref Post`. No `User.alder`. No better than plain. |
| qwen3-coder:30b | plain | T4 notes+theme | 1 ROUTE | PARTIAL | Everything right except `msg Delete = "/notes/{id}/delete"` declared on both pages: route collision. First near-pass from qwen. |
| qwen3-coder:30b | pitfalls | T4 notes+theme | 1 ROUTE | PARTIAL | Identical collision. Otherwise clean. |

## Totals

| Model | Variant | PASS | PARTIAL | FAIL | Errors |
|---|---|---|---|---|---|
| claude-haiku | plain (T3, T4) | 2 | 0 | 0 | 0 |
| claude-haiku | pitfalls (T3, T4) | 2 | 0 | 0 | 0 |
| qwen3-coder:30b | plain (T2–T4) | 0 | 2 | 1 | 5 |
| qwen3-coder:30b | pitfalls (T2–T4) | 0 | 2 | 1 | 6 |

Like-for-like with E2 (plain, T2–T4): Haiku 4 → 0 (T3, T4); qwen3-coder 11 → 5.
