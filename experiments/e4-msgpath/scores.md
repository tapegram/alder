# E4 scores

Book at commit d60de71: `msg` declarations have no path. Same prompt shape as E3 plain (reference + example, no appendix).

| Model | Task | Errors | Verdict | Notes |
|---|---|---|---|---|
| claude-sonnet | T3 blog | 0 | PASS | `msg AddComment { post: Post.PostId } form NewComment`, `msg Toggle { id: Post.PostId }`, `msg Logout`. Slug via `Db.where ... |> Db.first`. |
| claude-haiku | T3 blog | 0 | PASS | `msg AddComment form NewComment` with no params, using `model.post.id` in `update` — correct, since the page's params travel with the message. |
| claude-haiku | T4 notes+theme | 0 | PASS | A page-local `Item` component typed `Html Never` inside `Html Msg` (now stated as allowed). No collision: `Delete` only on Show. |
| qwen3-coder:30b | T3 blog | 1 TYP, 2 STRUCT | FAIL | New `msg` syntax used correctly, no collision. `Db.get Post params.slug` (sixth run). `Comment` declared inside `Post` and used unqualified. No `User.alder` emitted. |
| qwen3-coder:30b | T4 notes+theme | 0 | PASS | **First clean qwen3-coder pass.** `msg Delete { id: Note.NoteId }` on both pages, which collided in E3, is now correct by construction. |

## Totals

| Model | PASS | PARTIAL | FAIL | Errors |
|---|---|---|---|---|
| claude-sonnet (T3) | 1 | 0 | 0 | 0 |
| claude-haiku (T3, T4) | 2 | 0 | 0 | 0 |
| qwen3-coder:30b (T3, T4) | 1 | 0 | 1 | 3 |
