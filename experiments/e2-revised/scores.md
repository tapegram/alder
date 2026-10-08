# E2 scores

Book at commit 1f473e1 (E1 revisions applied). Same method as E1. Two context variants: **full** = reference + to-do example (~11k tokens); **refonly** = reference only (~8k tokens), used for the from-scratch tasks T3 and T4.

| Model | Variant | Task | Errors | Verdict | Notes |
|---|---|---|---|---|---|
| claude-sonnet | full | T4 notes+theme | 1 STRUCT | PARTIAL | `theme: Theme` in three page modules without `import Theme`. Put `Theme` in its own module with `flip`, `switchLabel`, `bodyClass` and tests. `App.SwitchTheme` redirects to Index (book said App.update must Redirect; now may Rerender). Layout typed `Html a` with `App.*` buttons inside. |
| claude-sonnet | refonly | T4 notes+theme | 1 STRUCT, 1 TYP | PARTIAL | `UserId`, `NoteId`, `NoteForm` unqualified across modules (the example would have shown `User.UserId`). `Layout.Shell : ... -> Html Msg` in a non-page module (`Msg` undefined). No `Password` capability: stores `Text.Secret` in the table and compares in `update` — the capability pattern lived only in the example. Otherwise complete and idiomatic. |
| claude-haiku | full | T1 modify | 0 | PASS | Uses `Db.orderBy Todo.priority.desc` and `<Select value={model.todo.priority}>`, both valid only under the revised book. |
| claude-haiku | full | T2 new page | 0 | PASS | `click={App.Logout}`. Form and msg both named `ChangeName` (type vs constructor namespaces; allowed). |
| claude-haiku | full | T3 new app | 1 TYP | PARTIAL | Made the slug the `@id` and then supplied it on `Db.insert` (`Insert t` excludes `@id`). `App.Logout` used correctly; `Html.unsafe` not used. |
| claude-haiku | full | T4 notes+theme | 3 (ROUTE, SYN, TYP) | FAIL | Uses `App.Logout` without declaring it. `let` statements in a function body without a `{ }` block. `Layout.Page` typed `Html App.Msg` so page children don't fit. `theme={Light}` hard-coded on every page (layout can't see the session). `App.update` returns `Rerender` (now allowed). |
| claude-haiku | refonly | T3 new app | 1 STRUCT, 1 TYP, 1 ROUTE | FAIL | `Auth.Login` without import (pages are now global, so this class of error disappears). `Html Msg` in the Layout module (third run to do this; chapter 5's own example was wrong). `<form submit={AddComment}>` without `{ slug }`. Otherwise complete: layout shows login/admin/logout by session, `Query.notFound` used as a short-circuit inside `let post <- match ...`. |
| claude-haiku | refonly | T4 notes+theme | 3 STRUCT, 2 ROUTE, 2 TYP, 1 SYN | FAIL | `Auth.Login`, `Index`, `Notes.Show` used without imports. `EditNote`/`DeleteNote {}` sent without their `{ id }` params. `type Model = Unit` (alias). `Html Msg` in the Layout module. `let` in a function body without a `{ }` block (second Haiku run to do this). Threads `session` through Model and into Layout. Duplicates `SwitchTheme` per page instead of in App. |
| qwen3-coder:30b | full | T1 modify | 1 LIB, 1 TYP | PARTIAL | Invents `Todo.Priority.toText`. `+` on `Text`. Also re-emits unchanged files. E1's union-ordering "error" is now valid. |
| qwen3-coder:30b | full | T2 new page | 1 TYP, 1 ROUTE | PARTIAL | `user: user` with `user : Maybe User` (same error as E1). `Profile.Show {}` for a page whose module is `Profile`. `App.Logout` correct. Down from 4 errors in E1. |
| qwen3-coder:30b | full | T3 new app | 2 STRUCT, 3 TYP | FAIL | `UserId` and `Comment` unqualified across modules. `Db.get Post params.slug` (same as E1). `Date.format` on a `Time` (same as E1). Tests compare a `Slug` to `Nothing`. `requires user` on the public post page (logic). The invented `AuthorId` from E1 is gone; `App.Logout` correct. |
| qwen3-coder:30b | full | T4 notes+theme | 2 STRUCT, 1 TYP, 1 ROUTE | FAIL | `Theme` declared in `Note` and used unqualified in App/Layout. `session.theme` inside `view` (no such parameter — now there is). `msg ToggleTheme = "/theme"` duplicated in both pages: route collision; should have been app-level. Otherwise idiomatic. |
| qwen3-coder:30b | refonly | T3 new app | structural | FAIL | Uses a `Todo` table in a blog app (leaked from reference prose), writes an `app/Db.alder` file, no imports, `slug: Text` rather than a brand, `Db.get` by slug. |
| qwen3-coder:30b | refonly | T4 notes+theme | 2 SYN + structural | FAIL | **First `case ... of` seen in any run** (twice), only without the example. No imports, no `requires user`, `Theme` undefined, `session.theme` in `view`, msgs with `{ id }` sent bare, hard-coded login. |

## Totals

| Model | Variant | PASS | PARTIAL | FAIL | Errors |
|---|---|---|---|---|---|
| claude-sonnet | full (T4) | 0 | 1 | 0 | 1 |
| claude-sonnet | refonly (T4) | 0 | 1 | 0 | 2 |
| claude-haiku | full (T1–T4) | 2 | 1 | 1 | 4 |
| claude-haiku | refonly (T3, T4) | 0 | 0 | 2 | 11 |
| qwen3-coder:30b | full (T1–T4) | 0 | 2 | 2 | 13 |
| qwen3-coder:30b | refonly (T3, T4) | 0 | 0 | 2 | structural |

Like-for-like with E1 (full variant, T1–T3): Haiku 4 → 1 errors; qwen3-coder 12 → 9.
