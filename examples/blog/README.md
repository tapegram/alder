# blog

The second example, which chapter 14 tours: posts with drafts, comments, a Reader/Author role,
a Markdown capability, pages of results and a search. `alder check`, `alder build` and `alder
test` take this directory; chapter 11 explains each command.

## Mutation score

`alder test --mutate examples/blog` (book 10, "Mutation testing"): 63 mutants.

- Before: 29 tests, 41 killed, 17 survived (score 0.71), recorded before the app declared its
  rules.
- After: 42 tests, 60 killed, 3 survived (score 0.95). `TestExample_Blog` fails if the score
  drops below 0.95, the exact 0.952 rounded down, or the number of mutants is not 63.

The rules are declared (book 06, "Rules"): `Post.author` is the table's `@owner`, the session
holds the user's `role` from login, and `Admin` and `Admin.Edit` `requires user, role Author`.
The cookie keeps the role until logout (or the session's 30 days), so both pages' `load` also
re-read the user's row (`User.isAuthor`, the re-read idiom): a demoted author is turned away at
once. Their new mutants are killed: the two `role dropped` by `Admin "a promoted reader logs in
again first"` and `Admin.Edit "a promoted reader logs in again before editing"` (the row says
Author, the cookie still Reader: only the role check turns them away); the re-reads by `Admin "a
demoted author cannot open the admin page"` and `Admin.Edit "a demoted author is turned away at
once"`; the two `scope dropped` (`Db.mine Post`, `Db.getMine Post`)
by `Admin "the admin page lists the author's own posts, newest first"` and `Admin.Edit "another
author's post is not found"`; the dropped `role` of the login's and the logout's `Session.put` by
`Auth.Login "logging in through the form"` and `Posts.Index "logging out from the front page
goes to log in"`.

The ten tests that closed the gaps:

- `Post "bySlug finds the post with that slug"`: `bySlug`'s `where` dropped.
- `Post "an author with posts cannot be deleted"`: `author @ref User` changed to `@cascade`.
- `Admin "the admin page lists the author's own posts, newest first"`: the admin list's `where`
  dropped (now its `Db.mine Post` made `Db.from Post`, a `scope dropped`), and its `orderBy`
  reversed.
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

The run also refuses 1 mutant it can prove equivalent and skips 3 that do not apply; neither
count is part of the score.
