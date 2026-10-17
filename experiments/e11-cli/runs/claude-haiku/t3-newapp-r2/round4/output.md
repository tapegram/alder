```alder-cli
# the login lands on the home page (the admin page is rebuilt below)
alder fill --decl Auth.Login.update - <<'EOF'
match msg {
  Submit p -> {
    let found <- Password.verify { email: p.form.email, password: p.form.password }
    match found {
      Just id -> {
        let _ <- Session.put (\s -> { s | user: Just id })
        Task.succeed (Redirect (Home {}))
      },
      Nothing -> Task.succeed (Reject "Email or password is incorrect."),
    }
  },
}
EOF

# the admin page: rebuilt as a blank page for signed-in users
alder remove page Admin
alder new page Admin /admin --blank --requires user
alder fill --decl Admin.Model - <<'EOF'
{ posts: List Post }
EOF
alder fill Admin.load - --hole ?load <<'EOF'
{
  let posts <- Db.from Post |> Db.orderBy Post.created.desc |> Db.all
  Query.succeed { posts: posts }
}
EOF
alder fill Admin.view - --hole ?view <<'EOF'
<Layout.Page title="Admin">
  <h1>Admin</h1>
</Layout.Page>
EOF
alder new msg Admin Toggle id:Post.PostId
alder new form Admin NewPost slug:Post.Slug title:Text.NonEmpty body:Text published:Bool --msg CreatePost
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
      <al-textarea field="body" label="Body" />
      <al-checkbox field="published" label="Published" />
      <al-button type="submit">Create post</al-button>
    </form>
  </al-card>
</Layout.Page>
EOF
alder fill --decl Admin.update - <<'EOF'
match msg {
  CreatePost p -> {
    let _ <- Db.insert Post { slug: p.form.slug, title: p.form.title, body: p.form.body, published: p.form.published, author: session.user }
    Task.succeed (Redirect (Admin {}))
  },
  Toggle p -> {
    let _ <- Db.update Post p.id (\x -> { x | published: not x.published })
    Task.succeed Rerender
  },
}
EOF
alder new test Admin "Toggle publishes a draft"
alder fill 'Admin."Toggle publishes a draft"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Post { slug: Slug "draft", title: Text.NonEmpty "Draft", body: "b", author: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Admin {})
  let _ <- Test.click (Toggle { id: id })
  let after <- Db.get Post id
  Task.succeed (Maybe.map (\p -> p.published) after == Just True)
}
EOF

# the home page: published posts, newest first
alder fill --decl Home.Model - <<'EOF'
{ posts: List Post }
EOF
alder fill Home.load - --hole ?load <<'EOF'
{
  let posts <-
    Db.from Post
      |> Db.where (Post.published.eq True)
      |> Db.orderBy Post.created.desc
      |> Db.all
  Query.succeed { posts: posts }
}
EOF
alder fill Home.view - --hole ?view <<'EOF'
<Layout.Page title="Blog">
  <h1>Blog</h1>
  <ul>
    {for post in model.posts ->
      <li><a href={Posts.Show { slug: post.slug }}>{post.title}</a></li>}
  </ul>
</Layout.Page>
EOF
alder new test Home "the front page lists published posts, newest first"
alder fill 'Home."the front page lists published posts, newest first"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Db.insert Post { slug: Slug "old", title: Text.NonEmpty "Old", body: "a", published: True, author: me }
  let _ <- Db.insert Post { slug: Slug "new", title: Text.NonEmpty "New", body: "b", published: True, author: me }
  let _ <- Db.insert Post { slug: Slug "draft", title: Text.NonEmpty "Draft", body: "c", published: False, author: me }
  let page <- Test.get (Home {})
  Task.succeed (Test.texts "li a" page.doc == ["New", "Old"])
}
EOF

# a post and its comments
alder fill --decl Posts.Show.Model - <<'EOF'
{ post: Post, comments: List Comment }
EOF
alder fill Posts.Show.load - --hole ?load <<'EOF'
{
  let found <- Db.from Post |> Db.where (Post.slug.eq params.slug) |> Db.first
  match found {
    Nothing -> Query.notFound,
    Just post -> {
      if post.published || session.user == Just post.author then {
        let comments <-
          Db.from Comment
            |> Db.where (Comment.post.eq post.id)
            |> Db.orderBy Comment.created.asc
            |> Db.all
        Query.succeed { post: post, comments: comments }
      } else Query.notFound,
    },
  }
}
EOF
alder fill Posts.Show.view - --hole ?view <<'EOF'
<Layout.Page title={model.post.title}>
  <h1>{model.post.title}</h1>
  <p>{model.post.body}</p>
  <h2>Comments</h2>
  <ul id="comments">
    {for c in model.comments -> <li><strong>{c.author}</strong> {c.body}</li>}
  </ul>
</Layout.Page>
EOF
alder new form Posts.Show NewComment author:Text.NonEmpty body:Text.NonEmpty --msg AddComment
alder fill --decl Posts.Show.view - <<'EOF'
<Layout.Page title={model.post.title}>
  <h1>{model.post.title}</h1>
  <p>{model.post.body}</p>
  <h2>Comments</h2>
  <ul id="comments">
    {for c in model.comments -> <li><strong>{c.author}</strong> {c.body}</li>}
  </ul>
  <al-card>
    <form submit={AddComment}>
      <al-input field="author" label="Your name" />
      <al-textarea field="body" label="Comment" />
      <al-button type="submit">Post comment</al-button>
    </form>
  </al-card>
</Layout.Page>
EOF
alder fill --decl Posts.Show.update - <<'EOF'
match msg {
  AddComment p -> {
    let _ <- Db.insert Comment { post: model.post.id, author: p.form.author, body: p.form.body }
    Task.succeed Rerender
  },
}
EOF
alder new test Posts.Show "an unpublished post is not found for a reader"
alder fill 'Posts.Show."an unpublished post is not found for a reader"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Db.insert Post { slug: Slug "draft", title: Text.NonEmpty "Draft", body: "secret", author: me }
  let r <- Test.open (Posts.Show { slug: Slug "draft" })
  Task.succeed (r.answer == PageMissing)
}
EOF
alder new test Posts.Show "an author sees their own unpublished post"
alder fill 'Posts.Show."an author sees their own unpublished post"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Db.insert Post { slug: Slug "draft", title: Text.NonEmpty "Draft", body: "secret", author: me }
  let _ <- Test.login { user: Just me }
  let r <- Test.open (Posts.Show { slug: Slug "draft" })
  Task.succeed (r.answer == Shown)
}
EOF
alder new test Posts.Show "a comment with an empty author is refused"
alder fill 'Posts.Show."a comment with an empty author is refused"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Db.insert Post { slug: Slug "hello", title: Text.NonEmpty "Hello", body: "hi", published: True, author: me }
  let _ <- Test.get (Posts.Show { slug: Slug "hello" })
  let r <- Test.submitRaw NewComment [{ name: "author", value: "" }, { name: "body", value: "Nice" }]
  let n <- Db.from Comment |> Db.count
  let refused = match r.answer {
    FormRejected _ -> True,
    _ -> False,
  }
  Task.succeed ({ refused: refused, comments: n } == { refused: True, comments: 0 })
}
EOF
alder check
```
