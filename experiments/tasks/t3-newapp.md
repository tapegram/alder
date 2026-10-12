# Task T3: a small blog

Write a complete new Alder app (not the todo app) for a personal blog:

- Posts have a title, a slug (lowercase letters, digits, hyphens), a body,
  a published flag, and a created time.
- `/` lists published posts, newest first, with links to each post.
- `/posts/{slug}` shows one post and its comments, and has a form to add a
  comment (author name and body, both required). Unpublished posts are 404
  for everyone except the logged-in author.
- `/admin` requires login, lists all posts (published or not), and has a
  form to create a post. Each post row has a button to toggle published.
- `/login` and logout, as in the example.
- An `App` module and a `Layout` module.

Also write tests (`test "…" = …`, chapter 10) in the modules they test: an
unpublished post is not found for a reader; a comment with an empty author
is refused; `/` lists only published posts, newest first. `alder test` must
pass. Do not call the real `Password` adapter from a test; installing the
fake with `Test.fake Password` is fine.

Output every file, in full, each preceded by a line
`--- file: app/<path> ---`. No explanation, only files.
