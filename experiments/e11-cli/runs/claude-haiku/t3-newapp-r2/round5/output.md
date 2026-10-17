```alder-cli
# Posts.Show: the page's load (the fill that failed: the arm's `if` is not in a block)
alder fill Posts.Show.load - --hole ?load <<'EOF'
{
  let found <- Db.from Post |> Db.where (Post.slug.eq params.slug) |> Db.first
  match found {
    Nothing -> Query.notFound,
    Just post -> if post.published || session.user == Just post.author then {
      let comments <-
        Db.from Comment
          |> Db.where (Comment.post.eq post.id)
          |> Db.orderBy Comment.created.asc
          |> Db.all
      Query.succeed { post: post, comments: comments }
    } else Query.notFound,
  }
}
EOF
# the view, before the comment form exists (form and msg refuse while the view is a hole)
alder fill Posts.Show.view - --hole ?view <<'EOF'
<Layout.Page title={model.post.title}>
  <h1>{model.post.title}</h1>
  <p>{model.post.body}</p>
  <h2>Comments</h2>
  <ul>
    {for c in model.comments -> <li><strong>{c.author}</strong> {c.body}</li>}
  </ul>
</Layout.Page>
EOF
# the comment form and its message
alder new form Posts.Show NewComment author:Text.NonEmpty body:Text.NonEmpty --msg AddComment
# update: insert the comment on this post (the page's post comes from load)
alder fill --decl Posts.Show.update - <<'EOF'
match msg {
  AddComment p -> {
    let _ <- Db.insert Comment { post: model.post.id, author: p.form.author, body: p.form.body }
    Task.succeed (Redirect (Posts.Show { slug: model.post.slug }))
  },
}
EOF
# the full view, with the form and its error lists
alder fill --decl Posts.Show.view - <<'EOF'
<Layout.Page title={model.post.title}>
  <h1>{model.post.title}</h1>
  <p>{model.post.body}</p>
  <h2>Comments</h2>
  <ul>
    {for c in model.comments -> <li><strong>{c.author}</strong> {c.body}</li>}
  </ul>
  <al-card>
    <form submit={AddComment}>
      <al-input field="author" label="Your name" />
      <Errors field="author" />
      <al-textarea field="body" label="Comment" />
      <Errors field="body" />
      <al-button type="submit">Add comment</al-button>
    </form>
  </al-card>
  <a href={Home {}}>All posts</a>
</Layout.Page>
EOF
# tests of the post page
alder new test Posts.Show "an unpublished post is not found for a reader"
alder fill 'Posts.Show."an unpublished post is not found for a reader"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Db.insert Post { slug: Slug "draft", title: Text.NonEmpty "Draft", body: "b", author: me }
  let r <- Test.open (Posts.Show { slug: Slug "draft" })
  Task.succeed (r.answer == PageMissing)
}
EOF
alder new test Posts.Show "a comment with an empty author is refused"
alder fill 'Posts.Show."a comment with an empty author is refused"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Db.insert Post { slug: Slug "hello", title: Text.NonEmpty "Hello", body: "b", published: True, author: me }
  let _ <- Test.get (Posts.Show { slug: Slug "hello" })
  let r <- Test.submitRaw NewComment [{ name: "author", value: "" }, { name: "body", value: "Nice post" }]
  let n <- Db.from Comment |> Db.count
  Task.succeed ({ answer: r.answer, comments: n } == { answer: FormRejected [{ field: "author", message: "must not be empty" }], comments: 0 })
}
EOF
# Admin: the view with the title's error list, so a refused title is shown
alder fill --decl Admin.view - <<'EOF'
<Layout.Page title="Admin">
  <h1>Admin</h1>
  <al-button variant="outline" click={App.Logout}>Log out</al-button>
  <table>
    <tr><th>Title</th><th>Published</th><th>Action</th></tr>
    {for post in model.posts ->
      <tr>
        <td><a href={Posts.Show { slug: post.slug }}>{post.title}</a></td>
        <td>{if post.published then "yes" else "no"}</td>
        <td><al-button variant="secondary" size="sm" click={Toggle { id: post.id }}>Toggle published</al-button></td>
      </tr>}
  </table>
  <al-card>
    <form submit={CreatePost}>
      <al-input field="slug" label="Slug" />
      <al-input field="title" label="Title" />
      <Errors field="title" />
      <al-textarea field="body" label="Body" />
      <al-checkbox field="published" label="Published" />
      <al-button type="submit">Create post</al-button>
    </form>
  </al-card>
</Layout.Page>
EOF
# Admin: the holes left by the generated tests
alder fill 'Admin."pressing Toggle"' - --hole ?expected <<'EOF'
({ answer: r.answer, published: Maybe.map (\p -> p.published) after } == { answer: Rerendered, published: Just True })
EOF
alder fill 'Admin."submitting NewPost"' - --hole ?expected <<'EOF'
Redirected (Admin {})
EOF
```
