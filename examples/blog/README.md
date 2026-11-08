# blog

The second example, which chapter 14 tours: posts with drafts, comments, a Reader/Author/Admin
role, a Markdown capability, pages of results and a search, the author's own pages under
`/write`, and a site admin under `/admin` that `alder new admin` generated (book 15). `alder
check`, `alder build` and `alder test` take this directory; chapter 11 explains each command.

To try it: `alder run`, then log in at `/login` with the password `password` as
`author@example.com` (an Author: `/write`), `reader@example.com` (a Reader) or
`admin@example.com` (an Admin: `/admin`). The accounts are the development stub's
(`adapters/auth/password.go`), which accepts them only under `alder run`.

## Layout

The tables (Comment, Post and User) live under `app/Table/`, one module each (`module Table.Comment`, `Table.Post` and `Table.User`), and route modules `import Table.<Name>`. The admin's folders are plural (`app/Admin/Todos/` and so on); its URLs are not (`/admin/todo`). `migrations/0002_rename_comment_rename_post_rename_user.sql` renames the tables from the old root modules to these; it moves no data.

## Deploying

`App` declares one environment, `local`, a `Script` target (book 16, "The Script hatch"):

```
alder deploy local
```

runs the whole gate (check, build, the tests, the mutation score), writes the artifact
`build/blog/deploy/<build id>/` (the executable, `static/`, `migrations/`,
`alder-deploy.json`) and runs `scripts/deploy-local.sh` with it. The script only records what
it was given, in `build/blog/deploy/<build id>.deploy-local.txt`, so the example deploys with
nothing installed; `go test ./book` runs this deploy for chapter 16.

To put it on a server, add an environment to the declaration in `app/App.alder`, such as
`production: Ssh "deploy@box.example.com" { domain: "blog.example.com", path: "/srv/blog" }`,
write `ALDER_SECRET=…` (16 bytes or more) into `/srv/blog/env` on the box, and run `alder
deploy production`;

`alder ci init` writes the GitHub workflow that does it on every push to
`main`.

The example's login is the development stub, which refuses every password outside
`alder run`: a real deployment needs a real `Password` adapter (book 7).

## Mutation score

`alder test --mutate examples/blog` (book 10, "Mutation testing"): 376 mutants, 227 tests, 362
killed, 14 survived (score 0.96). `TestExample_Blog` fails if the score drops below 0.96, the
exact 0.963 rounded down, or the number of mutants is not 376.

- **The generated admin** (`app/Admin/`, "Replaying the admin" below): 290 mutants, 279 killed
  by its 181 generated tests, kept as `alder new admin` wrote them.

  Three survivors are equivalent:

  - `postsTotal > 200` and `authorsTotal > 200` flipped to `>= 200` in the `where` of the Comment
    and Post New routes' lists (a search of "" matches every row).
  - `Post.author.in ids` dropped from the user bulk route's `load` (`refused` is only asked of
    the chosen rows).

  The Edit routes' lists have no such survivor: the row's own value is read by itself and comes
  first, the search leaves it out, and the generated test of exactly 200 rows tells `>` from `>=`.

  The eight others are the `step dropped` of the `adminOnly` that begins the Comment and Post
  `Create` and `Save`, and the `role dropped` of the four `Search…` routes (`SearchPost`,
  `SearchAuthor`).

- **The rest of the blog:** 86 mutants, 83 killed, 3 survived, as before the admin (below).

The `step dropped` of the `adminOnly` that begins `Create` and `Save` on a form with a select of
another table's rows is not a gap: a demoted Admin's post of that form is refused (400) by the
options check before the update runs, since the page it was posted from is rendered again to
check the options, and that page's `load` begins with `adminOnly` too. Nor is the `role dropped`
of a `Search…` route, which only redirects to the page it was posted from: another role's search
is redirected there, or, when it does not decode, that page is rendered again with the errors,
and either way that page's own role check answers the 404 the search's would. (The search's
`requires dropped` is killed: the generated test of a long list sends a visitor's search, which
must go to log in, where the mutant's role check answers 404.)

The history of the rest:

- Before: 29 tests, 41 killed, 17 survived (score 0.71), recorded before the app declared its
  rules.
- After: 42 tests, 60 killed, 3 survived (score 0.95).
- With the route operators (alder#187): a route's `requires` and role, its `Response` answers and
  its `let _ <-` steps added 31 mutants to the rest, 16 at first killed. The survivors were write
  routes' guards: the `requires dropped` and `role dropped` of `Write`'s Create and of
  `Write.Edit`'s four write routes, the `step dropped` of `User.authorOnly` in Create, Save and
  Unpublish (only Publish's and Delete's were tested), and the `refresh → invalid` of Publish and
  Unpublish (no test checked what they answer). Four tests kill them, each sending the writes from
  a page still open after the session changed: `Write "Create is refused to a visitor, a Reader
  and a demoted author"`, `Write.Edit "the write routes refuse a visitor and a Reader"`,
  `Write.Edit "the write routes turn a demoted author away"`, and `Write.Edit "publishing and
  unpublishing through the page"`. The rest now has 46 tests.

The rules are declared (book 06, "Rules"): `Post.author` is the table's `@owner`, the session
holds the user's `role` from login, and `Write` and `Write.Edit` `requires user, role Author`.
The cookie keeps the role until logout (or the session's 30 days), so both routes' `load` also
re-read the user's row (`User.isAuthor`, the re-read idiom): a demoted author is turned away at
once.

Their mutants are killed:

- **The re-reads** (`guard dropped`) by `Write "a demoted author cannot open the write page"` and
  `Write.Edit "a demoted author is turned away at once"`.
- **The two `scope dropped`** (`Db.mine Post`, `Db.getMine Post`) by
  `Write "the write page lists the author's own posts, newest first"` and
  `Write.Edit "another author's post is not found"`, which since routes (#160) also kills the
  `scope dropped` of `Write.Edit`'s write routes (no `load` runs before them, so their
  `Db.updateMine` and `Db.deleteMine` are the check).
- **The dropped `role`** of the login's and the logout's `Session.put` by
  `Auth.Login "logging in through the form"` and
  `Posts.Index "logging out from the front page goes to log in"`.
- **The `role dropped` of the two GET routes** by `Write "a promoted reader logs in again first"`
  and `Write.Edit "a promoted reader logs in again before editing"` (the row says Author while
  the cookie says Reader: only the role check turns them away).

The ten tests that closed the gaps:

- `Post "bySlug finds the post with that slug"`: `bySlug`'s `where` dropped.
- `Post "an author with posts cannot be deleted"`: `author @ref User` changed to `@cascade`.
- `Write "the write page lists the author's own posts, newest first"`: the write page's list's `where`
  dropped (now its `Db.mine Post` made `Db.from Post`, a `scope dropped`), and its `orderBy`
  reversed.
- `Posts.Index "a page that is exactly full has no next page"`: `>` flipped to `>=` in
  `hasMore`.
- `Posts.Index "logging out from the front page goes to log in"`: the dropped `role` of logout's
  `Session.put`. The front page is the one page with a Log out button that does not `require
  user`, so here the browser shows where logging out ends: it also kills logout's `redirect →
  refresh`.
- `Write.Edit "a visitor is sent to log in"`: the `requires dropped` of the `Edit` route.
- `Write.Edit "saving changes the title and the body"`: the save replaced, and each updated
  field dropped.
- `Posts.Show "commenting through the form"`: the comment's insert replaced, and the `redirect →
  refresh` after it.
- `Auth.Login "an unknown email is rejected"` and `Auth.Login "logging in through the form"`,
  with a new `fake Password`: the dropped `Session.put`, and the login's `redirect → refresh` and
  `invalid → refresh`.

The three survivors are legitimate: no test can kill them, because each mutant behaves exactly
like the original program.

- `Posts.Index Index.load:33:17 operator flipped params.page < 1 → params.page <= 1`: at the one
  value that tells the two apart, page 1, both branches give 1.
- `Posts.Index Index.load:33:48 operator flipped params.page > lastPage → params.page >= lastPage`: at
  `lastPage`, both branches give `lastPage`. (The same line's two `condition negated` mutants
  are killed.)
- `Posts.Index Index.load:42 limit dropped Db.limit (perPage + 1) → no limit`: `load` keeps only
  `List.take perPage` of what it reads, and `hasMore` only asks whether there are more than
  `perPage`. Without the limit the query reads every matching post, which is slower, but the
  page is the same.

The run also refuses 1 mutant it can prove equivalent (`User.role`'s default, which no row
leaves out) and skips 54 that do not apply (most of them a `step dropped` of a write or a
`Session.put`, which `write replaced` and `session put dropped` make); neither count is part of
the score.

## Replaying the admin

Every file under `app/Admin/` is exactly what `alder new admin` wrote; none was edited by hand.
The author's modules were `Admin` and `Admin.Edit` until then, at `/admin`; the admin needs that
module and path, so they became `Write` and `Write.Edit`, at `/write` and `/write/{id}`.

The steps, from the blog before the admin:

1. `User.Role` gains `Admin` (`type Role = Reader | Author | Admin`). The session already had
   `role: User.Role = Reader`, set at login from the user's row (`Auth.Login`), which is what
   `alder new admin` needs.
2. `migrations/0001_initial.sql` was written again from the tables (`rm migrations/*` and
   `alder migrate`) at this step: the example had no data to keep, so it kept one migration.

   The tables have since moved into `app/Table/` (`0002_rename_*`, see Layout above), so do not
   `rm migrations/*` now: it would delete that rename. An app with
   data runs `alder migrate` instead, which rebuilds `User` with `'Admin'` in the column's
   `CHECK`.

   A `blog.db` that `alder run` made before this step no longer matches `0001`, and
   the app refuses to start on it: delete it, or run once with `ALDER_DEV=1`, which recreates
   the database empty (book 8).
3. `alder new admin` writes the admin:

   ```
   $ alder new admin
   wrote app/Admin/Comments/Bulk.alder
   wrote app/Admin/Comments/Delete.alder
   wrote app/Admin/Comments/Edit.alder
   wrote app/Admin/Comments/Index.alder
   wrote app/Admin/Comments/New.alder
   wrote app/Admin/Comments/Show.alder
   wrote app/Admin/Index.alder
   wrote app/Admin/Layout.alder
   wrote app/Admin/Posts/Bulk.alder
   wrote app/Admin/Posts/Delete.alder
   wrote app/Admin/Posts/Edit.alder
   wrote app/Admin/Posts/Index.alder
   wrote app/Admin/Posts/New.alder
   wrote app/Admin/Posts/Show.alder
   wrote app/Admin/Users/Bulk.alder
   wrote app/Admin/Users/Delete.alder
   wrote app/Admin/Users/Edit.alder
   wrote app/Admin/Users/Index.alder
   wrote app/Admin/Users/New.alder
   wrote app/Admin/Users/Show.alder
   ```

4. The development login (`adapters/auth/password.go`) gains `admin@example.com`, an Admin.

The admin never shows or edits `User.passwordHash`, a secret by its name (book 15): a user it
creates starts with an empty one.

- Every admin page reads the user's row again, so an Admin made a Reader is turned away at once.
- An Admin cannot delete their own account or change their own role there.
- Each list filters by its `Bool` and union columns (`/admin/post?published=yes`,
  `/admin/user?role=Admin`).

To check that the committed admin is still what the generator writes, in a copy of the blog:
`rm -r app/Admin && alder new admin`, then `git diff --stat` shows nothing. After a change to a
table, `alder regen admin` rewrites only the admin's owned regions (book 15).
