# blog

The second example, which chapter 14 tours: posts with drafts, comments, a Reader/Author role,
a Markdown capability, pages of results and a search. `alder check`, `alder build` and `alder
test` take this directory; chapter 11 explains each command.

## Mutation score

`alder test --mutate examples/blog` (book 10, "Mutation testing"): 58 mutants.

- Before: 29 tests, 41 killed, 17 survived (score 0.71).
- After: 39 tests, 55 killed, 3 survived (score 0.95). `TestExample_Blog` fails if the score
  drops below 0.94, the exact 0.948 rounded down, or the number of mutants is not 58.

The ten tests that closed the gaps:

- `Post "bySlug finds the post with that slug"`: `bySlug`'s `where` dropped.
- `Post "an author with posts cannot be deleted"`: `author @ref User` changed to `@cascade`.
- `Admin "the admin page lists the author's own posts, newest first"`: the admin list's `where`
  dropped, and its `orderBy` reversed.
- `Posts.Index "a page that is exactly full has no next page"`: `>` flipped to `>=` in
  `hasMore`.
- `Posts.Index "logging out from the front page goes to log in"`: logout's `Redirect →
  Rerender`. The front page is the one page with a Log out button that does not `require user`,
  so here the browser shows the difference.
- `Admin.Edit "a visitor is sent to log in"`: `requires user` dropped.
- `Admin.Edit "saving changes the title and the body"`: the save replaced, and each updated
  field dropped.
- `Posts.Show "commenting through the form"`: the redirect after a comment.
- `Auth.Login "an unknown email is rejected"` and `Auth.Login "logging in through the form"`,
  with a new `fake Password`: the dropped `Session.put`, the login's redirect, and its
  `Reject`.

The three survivors are legitimate: no test can kill them, because each mutant behaves exactly
like the original program.

- `Posts.Index load:30:17 operator flipped params.page < 1 → params.page <= 1`: at the one
  value that tells the two apart, page 1, both branches give 1.
- `Posts.Index load:30:48 operator flipped params.page > lastPage → params.page >= lastPage`: at
  `lastPage`, both branches give `lastPage`. (The same line's two `condition negated` mutants
  are killed.)
- `Posts.Index load:39 limit dropped Db.limit (perPage + 1) → no limit`: `load` keeps only
  `List.take perPage` of what it reads, and `hasMore` only asks whether there are more than
  `perPage`. Without the limit the query reads every matching post, which is slower, but the
  page is the same.

The run also refuses 1 mutant it can prove equivalent and skips 7 that do not apply; neither
count is part of the score.
