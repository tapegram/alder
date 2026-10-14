```alder-cli
# the layout (the stub from --bare has no body)
alder fill --decl Layout.Page - <<'EOF'
<html>
  <head>
    <meta charset="utf-8" />
    <title>{props.title}</title>
    <link rel="stylesheet" href={Url.external "/static/al/al.css"} />
  </head>
  <body>
    <al-page width="narrow">
      <main>{props.children}</main>
    </al-page>
  </body>
</html>
EOF

# the public pages
alder new page Home / --blank
alder new auth --users User --to Home
alder new page Posts "/posts/{slug}" --param slug:Post.Slug --blank
alder new page Admin /admin --blank --requires user

# the home page: published posts, newest first
alder fill --decl Home.Model - <<'EOF'
{ posts: List Post }
EOF
alder fill --decl Home.load - <<'EOF'
{
  let posts <-
    Db.from Post
      |> Db.where (Post.published.eq True)
      |> Db.orderBy Post.created.desc
      |> Db.all
  Query.succeed { posts: posts }
}
EOF
alder fill --decl Home.view - <<'EOF'
<Layout.Page title="Blog">
  <h1>Blog</h1>
  <ul>
    {for post in model.posts -> <li><a href={Posts { slug: post.slug }}>{post.title}</a></li>}
  </ul>
</Layout.Page>
EOF
alder new test Home "lists only published posts newest first"
alder fill 'Home."lists only published posts newest first"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Db.insert Post { title: Text.NonEmpty "First", slug: Post.Slug "first", body: "one", published: True, author: me }
  let _ <- Db.insert Post { title: Text.NonEmpty "Draft", slug: Post.Slug "draft", body: "two", author: me }
  let _ <- Db.insert Post { title: Text.NonEmpty "Second", slug: Post.Slug "second", body: "three", published: True, author: me }
  let page <- Test.get (Home {})
  Task.succeed (Test.texts "li a" page.doc == ["Second", "First"])
}
EOF

# the post page: load, comments, and the comment form
alder fill --decl Posts.Model - <<'EOF'
{ post: Post, comments: List Comment }
EOF
alder fill --decl Posts.load - <<'EOF'
{
  let found <- Db.from Post |> Db.where (Post.slug.eq params.slug) |> Db.first
  match found {
    Nothing -> Query.notFound,
    Just post -> {
      let comments <-
        Db.from Comment
          |> Db.where (Comment.post.eq post.id)
          |> Db.orderBy Comment.created.asc
          |> Db.all
      if post.published || session.user == Just (post.author) then Query.succeed { post: post, comments: comments } else Query.notFound
    },
  }
}
EOF
alder fill --decl Posts.view - <<'EOF'
<Layout.Page title={model.post.title}>
  <h1>{model.post.title}</h1>
</Layout.Page>
EOF
alder new form Posts NewComment author:Text.NonEmpty body:Text.NonEmpty --msg Reply
alder fill --decl Posts.view - <<'EOF'
<Layout.Page title={model.post.title}>
  <h1>{model.post.title}</h1>
  <article>{model.post.body}</article>
  <h2>Comments</h2>
  <ul id="comments">
    {for c in model.comments -> <li><strong>{c.author}</strong>: {c.body}</li>}
  </ul>
  <al-card>
    <form submit={Reply}>
      <Errors form />
      <al-input field="author" label="Your name" />
      <al-textarea field="body" label="Comment" />
      <al-button type="submit">Add comment</al-button>
    </form>
  </al-card>
</Layout.Page>
EOF
alder fill --decl Posts.update - <<'EOF'
match msg {
  Reply p -> {
    let _ <- Db.insert Comment { post: model.post.id, author: p.form.author, body: p.form.body }
    Task.succeed Rerender
  },
}
EOF
alder new test Posts "an unpublished post is not found for a reader"
alder fill 'Posts."an unpublished post is not found for a reader"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Db.insert Post { title: Text.NonEmpty "Draft", slug: Post.Slug "draft", body: "two", author: me }
  let page <- Test.open (Posts { slug: Post.Slug "draft" })
  Task.succeed (page.answer == PageMissing)
}
EOF
alder new test Posts "an unpublished post is shown to its author"
alder fill 'Posts."an unpublished post is shown to its author"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Db.insert Post { title: Text.NonEmpty "Draft", slug: Post.Slug "draft", body: "two", author: me }
  let _ <- Test.login { user: Just me }
  let page <- Test.open (Posts { slug: Post.Slug "draft" })
  Task.succeed (page.answer == Shown)
}
EOF
alder new test Posts "a comment with an empty author is refused"
alder fill 'Posts."a comment with an empty author is refused"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Db.insert Post { title: Text.NonEmpty "First", slug: Post.Slug "first", body: "one", published: True, author: me }
  let _ <- Test.get (Posts { slug: Post.Slug "first" })
  let r <- Test.submitRaw NewComment [{ name: "author", value: "" }, { name: "body", value: "Nice post" }]
  let n <- Db.from Comment |> Db.count
  let rejected = match r.answer {
    FormRejected _ -> True,
    _ -> False,
  }
  Task.succeed ({ rejected: rejected, comments: n } == { rejected: True, comments: 0 })
}
EOF

# the admin page: list, create, toggle
alder fill --decl Admin.Model - <<'EOF'
{ posts: List Post }
EOF
alder fill --decl Admin.load - <<'EOF'
{
  let posts <-
    Db.from Post
      |> Db.orderBy Post.created.desc
      |> Db.all
  Query.succeed { posts: posts }
}
EOF
alder fill --decl Admin.view - <<'EOF'
<Layout.Page title="Admin">
  <h1>Admin</h1>
</Layout.Page>
EOF
alder new form Admin NewPost title:Text.NonEmpty slug:Post.Slug body:Text --msg Create
alder new msg Admin Toggle id:Post.PostId
alder fill --decl Admin.view - <<'EOF'
<Layout.Page title="Admin">
  <h1>Admin</h1>
  <al-card>
    <form submit={Create}>
      <Errors form />
      <al-input field="title" label="Title" />
      <al-input field="slug" label="Slug" />
      <al-textarea field="body" label="Body" />
      <al-button type="submit">Create post</al-button>
    </form>
  </al-card>
  <ul id="posts">
    {for post in model.posts ->
      <li>
        <a href={Posts { slug: post.slug }}>{post.title}</a>
        {if post.published then " (published)" else " (draft)"}
        <al-button variant="secondary" size="sm" click={Toggle { id: post.id }}>{if post.published then "Unpublish" else "Publish"}</al-button>
      </li>}
  </ul>
  <al-button variant="outline" click={App.Logout}>Log out</al-button>
</Layout.Page>
EOF
alder fill --decl Admin.update - <<'EOF'
match msg {
  Create p -> {
    let _ <- Db.insert Post { title: p.form.title, slug: p.form.slug, body: p.form.body, author: session.user }
    Task.succeed Rerender
  },
  Toggle p -> {
    let found <- Db.get Post p.id
    match found {
      Just post -> {
        if post.author == session.user then {
          let _ <- Db.update Post p.id (\t -> { t | published: not t.published })
          Task.succeed Rerender
        } else Task.notFound
      },
      Nothing -> Task.notFound,
    }
  },
}
EOF
alder new test Admin "a visitor is sent to log in"
alder fill 'Admin."a visitor is sent to log in"' - <<'EOF'
{
  let page <- Test.open (Admin {})
  Task.succeed (page.answer == Redirected (Auth.Login {}))
}
EOF
```
