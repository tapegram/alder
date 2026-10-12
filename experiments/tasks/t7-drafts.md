# Task T7: drafts on the blog

Change the blog app shown after this task (its `app/` as it is now) so
that a post can be a draft:

- A post is either a draft or published. A new post (the dashboard's "New
  post" form) starts as a draft.
- The front page (`/`) and its search list only published posts: a draft is
  never listed, and never found by a search.
- The author's dashboard (`/admin`) shows the author's drafts, each with a
  "Publish" button that publishes it. Once published, the post is on the
  front page and found by a search.
- The page of a draft (`/posts/{slug}`) is a 404 for everyone but its
  author, visitors included; its author can open it.
- The app keeps its tables with migrations (`migrations/`, written by
  `alder migrate`, which is run for you after your change: do not write
  migration files). Posts that exist when the change is deployed may become
  drafts.

**Names.** Use exactly these names; anything not listed here is your choice.
Every name the app already has (modules, pages, forms, messages, fields,
the `ul.posts` list of the front page) stays as it is.

- `app/Post.alder`: `table Post` gains exactly one field,
  `published: Bool = False`, and no other table changes.
- `app/App.alder`: the session stays exactly `session { user: Maybe User.UserId }`.
- `app/Admin.alder` (page `Admin`, `/admin`, the dashboard): the author's
  drafts, and only those, are listed inside an element with `id="drafts"`;
  each draft's title is the whole text of an element with `class="title"`.
  `msg Publish { id: Post.PostId }`, sent by the "Publish" button beside
  each draft, publishes that post when it is the logged-in author's (any
  other id is a 404, `Task.notFound`) and answers `Redirect (Admin {})`.
  `form NewPost` and `msg Create` stay as they are, and `Create` still
  answers `Redirect (Admin.Edit { id: … })`.
- `app/Posts/Index.alder` (page `Posts.Index`, `/`, `{ page: Int = 1, q: Text = "" }`):
  the posts stay listed in `<ul class="posts">`, each post's link having the
  post's title as its whole text; `form SearchForm` and `msg Search` stay.
- `app/Posts/Show.alder` (page `Posts.Show`, `/posts/{slug}`,
  `{ slug: Post.Slug }`): a draft is a 404 (`Query.notFound`) for anyone
  but its author.

Also write tests (`test "…" = …`, chapter 10) in the modules they test: a
draft is not on the front page and not found by a search; a reader opening a
draft's page gets not found, and its author does not; publishing a draft
from the dashboard puts it on the front page. `alder test` must pass. Do not
call the real `Password` adapter from a test; installing the fake with
`Test.fake Password` is fine.

Output only the files you add or change: not the files you leave as they are,
not the standard library, nothing outside `app/`. Give each in full, preceded by
a line `--- file: app/<path> ---`. No explanation, only files.
