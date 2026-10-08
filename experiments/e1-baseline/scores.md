# E1 scores

Book at commit f0ec8b4. One shot, no compiler, no retries. Context: chapters 3–10 + full to-do example (~11k tokens).

Verdicts: **PASS** = compiles as written. **PARTIAL** = 1–2 errors, each a one-line fix a compiler message would point at. **FAIL** = 3+ errors or a structural misunderstanding.

| Model | Task | Errors | Verdict | Notes |
|---|---|---|---|---|
| claude-opus | T1 modify | 0 | PASS | Sorts by filtering into three lists and concatenating (avoids relying on sort stability). `<Select value={label}>` guess. |
| claude-opus | T2 new page | 1 HTML | PARTIAL | Logout via raw `<form method="post" action="/logout">` — a deliberate workaround, explained in a comment. Used `Db.count` twice instead of loading rows. |
| claude-opus | T3 new app | 0 | PASS | Checks slug uniqueness before insert and `Reject`s. Splits body into paragraphs. `view : Model -> Html Never` for msg-less page. |
| claude-sonnet | T1 modify | 0 | PASS | Relies on `List.sortBy` stability (unspecified). `<Select value={label}>`. |
| claude-sonnet | T2 new page | 1 STRUCT, 1 SYN | PARTIAL | Uses `Todos.Index {}` without importing it. Writes `-- NOTE` comments inside HTML (would render as text). Logout: links to the login page instead. |
| claude-sonnet | T3 new app | 0 | PASS | Duplicates logout as `msg Logout = "/admin/logout"` on the admin page (different route, so no collision). `Html Never` for msg-less page. |
| claude-haiku | T1 modify | 1 STRUCT | PARTIAL | `priority: Priority` unqualified in Index/Show (should be `Todo.Priority`). Edit form `<Select>` has no `value`. |
| claude-haiku | T2 new page | 1 ROUTE | PARTIAL | Re-declares `msg Logout = "/logout"` — collides with Auth.Login's route. |
| claude-haiku | T3 new app | 1 ROUTE, 1 HTML | PARTIAL | Same `/logout` collision. `Html.raw model.post.body` on user-entered text (XSS). |
| qwen3-coder:30b | T1 modify | 2 TYP | PARTIAL | `Db.orderBy Todo.priority.desc` on a union column (unspecified). `Todo.filterLabel props.todo.priority` — wrong union. `<Select value={model.todo.priority}>` passes the union value (sensible). |
| qwen3-coder:30b | T2 new page | 1 TYP, 2 ROUTE, 1 STRUCT | FAIL | `user.name` on a `Maybe User` without matching. `href={App.login}` (not a page value). `Profile {}` without import. `/logout` collision. |
| qwen3-coder:30b | T3 new app | 4 TYP, 1 LIB, 1 STRUCT | FAIL | Invents `AuthorId` separate from `UserId` and `@ref`s with it. `id: Text @id` (must be a brand). `Db.get Post params.slug` (get by slug). `Date.format` on a `Time`. Test calls nonexistent `Post.comments`. Uses `Comment` type unqualified from `Post` module. Also `requires user` on the public post page (logic). |
| qwen3:8b | T1 modify | 4 TYP, 2 LIB, 1 STRUCT, 1 HTML, 1 SYN | FAIL | Tuple in `sortBy`, negates a `Time`, invents `Todo.prioritySortKey` and `Text.fromEnum`, qualifies constructors (`Todo.Normal`), `Maybe.map` on a non-Maybe, `<Input>` for a union, breaks a line inside `Task.succeed`. 188s, 6.4k output tokens (thinking). |
| qwen3:8b | T2 new page | 1 TYP, 1 STRUCT, 1 ROUTE | FAIL | `user.name` on a `Maybe User`. `Db.from Todo` without `import Todo`. `click={Auth.Login.Logout}` (another page's msg). |
| qwen3:8b | T3 new app | 3 SYN, 4 STRUCT, 3 TYP, 1 ROUTE | FAIL | `params` used inside `update`; `let <-` inside `if` without a block; `</Layout</Layout.Page>`; `Slug`, `User.UserId`, `CommentId` unqualified or undefined; `msg Submit = "/posts/{slug}/comment"` with no `{ slug }` params; `Db.delete Comment params.slug`; `<Errors field="" />`. |

## Totals

| Model | PASS | PARTIAL | FAIL | Errors |
|---|---|---|---|---|
| claude-opus | 2 | 1 | 0 | 1 |
| claude-sonnet | 2 | 1 | 0 | 2 |
| claude-haiku | 0 | 3 | 0 | 4 |
| qwen3-coder:30b | 0 | 1 | 2 | 12 |
| qwen3:8b | 0 | 0 | 3 | 23 |

Error classes across all runs: TYP 15, STRUCT 9, ROUTE 6, SYN 5, HTML 4, LIB 3, EFF 0, ANN 0.
