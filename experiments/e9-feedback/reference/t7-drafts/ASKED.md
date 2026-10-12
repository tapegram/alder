# Tests asked only: t7-drafts

The calibration line for H1 (`findings.md` reads the baseline scores against it): the
reference's app code with only the tests the task's tests paragraph names, scored by
`alder test --mutate`.

The paragraph:

> Also write tests (`test "…" = …`, chapter 10) in the modules they test: a
> draft is not on the front page and not found by a search; a reader opening
> a draft's page gets not found, and its author does not; publishing a draft
> from the dashboard puts it on the front page. `alder test` must pass. Do not
> write tests that call the `Password` capability.

## How to reproduce

1. Copy `reference/t7-drafts/` to a scratch directory (drop this file and `SURVIVORS.md`).
2. Delete every `test` declaration in `app/` except the ones below, and add the ones below that are missing (same module, same text).
3. Run `alder test --mutate .` there.

## Score: 38/57 = 0.667

| Class | Survivors |
|---|---|
| authorization | 3 |
| data | 8 |
| flow | 4 |
| logic | 3 |
| schema | 1 |

By operator: redirect → rerender 3, operator flipped 3, where dropped 2, guard dropped 2, update field dropped 2, cascade ↔ restrict 1, orderBy reversed 1, requires dropped 1, write replaced 1, session put dropped 1, reject → rerender 1, limit dropped 1.

```text
28 passed, 0 failed
57 mutants: 38 killed, 19 survived  (score 0.67)
survived:
  Post          Post:14      cascade ↔ restrict     author @ref User @restrict → author @ref User @cascade
  Post          bySlug:19    where dropped          Db.where (Post.slug.eq slug) → the condition dropped
  Admin         load:24      where dropped          Db.where (Post.author.eq user.id) → the condition dropped
  Admin         load:25      orderBy reversed       Db.orderBy Post.created.desc → Db.orderBy Post.created.asc
  Admin         update:60    guard dropped          if post.author == model.author.id then … else Task.notFound → the notFound arm dropped
  Admin.Edit    requires:8   requires dropped       requires user → served without user (user = User.UserId "mutant")
  Admin.Edit    update:37    write replaced         let _ <- Db.update Post model.post.id … → let _ <- Task.succeed ()
  Admin.Edit    update:37    update field dropped   { post | title: p.form.title, body: p.form.body } → { post | body: p.form.body }
  Admin.Edit    update:37    update field dropped   { post | title: p.form.title, body: p.form.body } → { post | title: p.form.title }
  App           update:16    redirect → rerender    Redirect (Auth.Login {}) → Rerender
  Auth.Login    update:40    session put dropped    let _ <- Session.put (\s -> { s | user: Just id }) → let _ <- Task.succeed ()
  Auth.Login    update:41    redirect → rerender    Redirect (Posts.Index {}) → Rerender
  Auth.Login    update:43    reject → rerender      Reject "Email or password is incorrect." → Rerender
  Posts.Index   load:30:17   operator flipped       params.page < 1 → params.page <= 1
  Posts.Index   load:30:48   operator flipped       params.page > lastPage → params.page >= lastPage
  Posts.Index   load:39      limit dropped          Db.limit (perPage + 1) → no limit
  Posts.Index   load:47      operator flipped       List.length found > perPage → List.length found >= perPage
  Posts.Show    update:49    guard dropped          if model.post.published then … else Task.notFound → the notFound arm dropped
  Posts.Show    update:51    redirect → rerender    Redirect (Posts.Show { slug: model.post.slug }) → Rerender
```

## The tests

T7 changes the blog, whose own tests a model receives with the base app, so the line keeps
the base's 25 tests (those of `experiments/e8-hidden/base/blog-nodrafts`, by name, in the
reference's versions, which mark posts meant to be listed `published: True`) and adds the
three asked ones. The asked tests:

`app/Admin.alder`:

```alder
test "publishing a draft from the dashboard" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let id <- Db.insert Post { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "", author: author }
  let _ <- Test.login { user: Just author }
  let before <- Test.get (Admin {})
  let r <- Test.click (Publish { id: id })
  let front <- Test.get (Posts.Index {})
  Task.succeed ({ before: Test.texts "#drafts .title" before.doc, answer: r.answer, after: Test.texts "#drafts .title" r.doc, front: Test.texts "ul.posts a" front.doc } == { before: ["Hello"], answer: Redirected (Admin {}), after: [], front: ["Hello"] })
```

`app/Posts/Index.alder`:

```alder
test "a draft is neither listed nor found" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "soup", title: Text.NonEmpty "Tomato soup", body: "", published: True, author: author }
  let _ <- Db.insert Post { slug: Post.Slug "secret", title: Text.NonEmpty "Tomato secret", body: "", author: author }
  let all <- load { page: 1, q: "" } { user: Just author }
  let found <- load { page: 1, q: "tomato" } { user: Just author }
  Task.succeed ({ all: List.map (\p -> p.title) all.posts, found: List.map (\p -> p.title) found.posts } == { all: [Text.NonEmpty "Tomato soup"], found: [Text.NonEmpty "Tomato soup"] })
```

`app/Posts/Show.alder`:

```alder
test "a draft is not found for a reader or a visitor, and shown to its author" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let reader <- Db.insert User { email: Text.Email "reader@example.com", name: Text.NonEmpty "Reader", passwordHash: "" }
  let _ <- Db.insert Post { slug: Post.Slug "draft", title: Text.NonEmpty "Draft", body: "", author: author }
  let visitor <- Test.open (Posts.Show { slug: Post.Slug "draft" })
  let _ <- Test.login { user: Just reader }
  let asReader <- Test.open (Posts.Show { slug: Post.Slug "draft" })
  let _ <- Test.login { user: Just author }
  let asAuthor <- Test.open (Posts.Show { slug: Post.Slug "draft" })
  Task.succeed ({ visitor: visitor.answer, reader: asReader.answer, author: asAuthor.answer, form: Test.has "textarea" asAuthor.doc } == { visitor: PageMissing, reader: PageMissing, author: Shown, form: False })
```
