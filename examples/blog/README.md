# blog

The second example, which chapter 14 tours: posts with drafts, comments, a Reader/Author/Admin
role, a Markdown capability, pages of results and a search, the author's own pages under
`/write`, and a site admin under `/admin` that `alder new admin` generated (book 15). `alder
check`, `alder build` and `alder test` take this directory; chapter 11 explains each command.

To try it: `alder run`, then log in at `/login` with the password `password` as
`author@example.com` (an Author: `/write`), `reader@example.com` (a Reader) or
`admin@example.com` (an Admin: `/admin`). The accounts are the development stub's
(`adapters/auth/password.go`), which accepts them only under `alder run`.

## Mutation score

`alder test --mutate examples/blog` (book 10, "Mutation testing"): 182 mutants, 124 tests, 179
killed, 3 survived (score 0.98). `TestExample_Blog` fails if the score drops below 0.98, the
exact 0.983 rounded down, or the number of mutants is not 182.

- The generated admin (`app/Admin/`, "Replaying the admin" below): 119 mutants, all 119 killed
  by its 82 generated tests, kept as `alder new admin` wrote them.
- The rest of the blog: 63 mutants, 60 killed, 3 survived, as before the admin (below).

The history of the rest:

- Before: 29 tests, 41 killed, 17 survived (score 0.71), recorded before the app declared its
  rules.
- After: 42 tests, 60 killed, 3 survived (score 0.95).

The rules are declared (book 06, "Rules"): `Post.author` is the table's `@owner`, the session
holds the user's `role` from login, and `Write` and `Write.Edit` `requires user, role Author`.
The cookie keeps the role until logout (or the session's 30 days), so both pages' `load` also
re-read the user's row (`User.isAuthor`, the re-read idiom): a demoted author is turned away at
once. Their new mutants are killed: the two `role dropped` by `Write "a promoted reader logs in
again first"` and `Write.Edit "a promoted reader logs in again before editing"` (the row says
Author, the cookie still Reader: only the role check turns them away); the re-reads by `Write "a
demoted author cannot open the write page"` and `Write.Edit "a demoted author is turned away at
once"`; the two `scope dropped` (`Db.mine Post`, `Db.getMine Post`)
by `Write "the write page lists the author's own posts, newest first"` and `Write.Edit "another
author's post is not found"`; the dropped `role` of the login's and the logout's `Session.put` by
`Auth.Login "logging in through the form"` and `Posts.Index "logging out from the front page
goes to log in"`.

The ten tests that closed the gaps:

- `Post "bySlug finds the post with that slug"`: `bySlug`'s `where` dropped.
- `Post "an author with posts cannot be deleted"`: `author @ref User` changed to `@cascade`.
- `Write "the write page lists the author's own posts, newest first"`: the write page's list's `where`
  dropped (now its `Db.mine Post` made `Db.from Post`, a `scope dropped`), and its `orderBy`
  reversed.
- `Posts.Index "a page that is exactly full has no next page"`: `>` flipped to `>=` in
  `hasMore`.
- `Posts.Index "logging out from the front page goes to log in"`: logout's `Redirect →
  Rerender`. The front page is the one page with a Log out button that does not `require user`,
  so here the browser shows the difference.
- `Write.Edit "a visitor is sent to log in"`: `requires user` dropped.
- `Write.Edit "saving changes the title and the body"`: the save replaced, and each updated
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

The run also refuses 5 mutants it can prove equivalent (`User.role`'s default, which no row
leaves out, and the `scope dropped` of `Write.Edit`'s four scoped writes of `model.post.id`,
which its `load` read with `Db.getMine`) and skips 3 that do not apply; neither count is part
of the score.

## Replaying the admin

Every file under `app/Admin/` is exactly what `alder new admin` wrote; none was edited by hand.
The author's pages were `Admin` and `Admin.Edit` until then, at `/admin`; the admin needs that
module and path, so they became `Write` and `Write.Edit`, at `/write` and `/write/{id}`. The
steps, from the blog before the admin:

1. `User.Role` gains `Admin` (`type Role = Reader | Author | Admin`). The session already had
   `role: User.Role = Reader`, set at login from the user's row (`Auth.Login`), which is what
   `alder new admin` needs.
2. `migrations/0001_initial.sql` is written again from the tables (`rm migrations/*` and
   `alder migrate`): an example keeps one migration, since it has no data to keep. An app with
   data runs `alder migrate` instead, which rebuilds `User` with `'Admin'` in the column's
   `CHECK`.
3. `alder new admin` writes the admin:

   ```
   $ alder new admin
   wrote app/Admin/Comment/Delete.alder
   wrote app/Admin/Comment/Edit.alder
   wrote app/Admin/Comment/Index.alder
   wrote app/Admin/Comment/New.alder
   wrote app/Admin/Comment/Show.alder
   wrote app/Admin/Index.alder
   wrote app/Admin/Layout.alder
   wrote app/Admin/Post/Delete.alder
   wrote app/Admin/Post/Edit.alder
   wrote app/Admin/Post/Index.alder
   wrote app/Admin/Post/New.alder
   wrote app/Admin/Post/Show.alder
   wrote app/Admin/User/Delete.alder
   wrote app/Admin/User/Edit.alder
   wrote app/Admin/User/Index.alder
   wrote app/Admin/User/New.alder
   wrote app/Admin/User/Show.alder
   ```

4. The development login (`adapters/auth/password.go`) gains `admin@example.com`, an Admin.

To check that the committed admin is still what the generator writes, in a copy of the blog:
`rm -r app/Admin && alder new admin`, then `git diff --stat` shows nothing. After a change to a
table, `alder regen admin` rewrites only the admin's owned regions (book 15).
