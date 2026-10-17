```alder-cli
# Admin first: its page and its messages (the session has `user` now)
alder new page Admin /admin --list Post --requires user
alder fill --decl Admin.Model - <<'EOF'
{ posts: List Post }
EOF
alder fill --decl Admin.load - <<'EOF'
\params session -> {
  let posts <- Db.from Post |> Db.orderBy Post.created.desc |> Db.all
  Query.succeed { posts: posts }
}
EOF
alder new form Admin NewPost slug:Post.Slug title:Text.NonEmpty body:Text
alder new msg Admin Toggle id:Post.PostId
alder fill --decl Admin.update - <<'EOF'
\msg model session -> match msg {
  SubmitNewPost p -> {
    let _ <- Db.insert Post { slug: p.form.slug, title: p.form.title, body: p.form.body, author: session.user }
    Task.succeed (Redirect (Admin {}))
  },
  Toggle p -> {
    let _ <- Db.update Post p.id (\x -> { x | published: not x.published })
    Task.succeed Rerender
  },
}
EOF
alder fill --decl Admin.view - <<'EOF'
\model session -> <Layout.Page title="Admin">
  <h1>Admin</h1>
  <al-card>
    <form submit={SubmitNewPost}>
      <Errors form />
      <al-input field="slug" label="Slug" />
      <al-input field="title" label="Title" />
      <al-textarea field="body" label="Body" />
      <al-button type="submit">Create</al-button>
    </form>
  </al-card>
  <ul id="posts">
    {for p in model.posts ->
      <li>
        <a href={Posts.Show { slug: p.slug }}>{p.title}</a>
        {if p.published then "published" else "draft"}
        <al-button variant="secondary" size="sm" click={Toggle { id: p.id }}>Toggle published</al-button>
      </li>}
  </ul>
  <al-button variant="outline" click={App.Logout}>Log out</al-button>
</Layout.Page>
EOF

# Login now goes to the admin page, so the home page can be replaced
alder fill Auth.Login.update.Submit - <<'EOF'
{
  let found <- Password.verify { email: p.form.email, password: p.form.password }
  match found {
    Just id -> {
      let _ <- Session.put (\s -> { s | user: Just id })
      Task.succeed (Redirect (Admin {}))
    },
    Nothing -> Task.succeed (Reject "Email or password is incorrect."),
  }
}
EOF
alder fill 'Auth.Login."logging in through the form"' - <<'EOF'
{
  let _ <- Test.fake Password
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.get (Auth.Login {})
  let r <- Test.submit Credentials { email: Text.Email "me@example.com", password: Text.Secret "secret" }
  Task.succeed ({ answer: r.answer, user: r.session.user, title: Test.text "h1" r.doc } == { answer: Redirected (Admin {}), user: Just me, title: Just "Admin" })
}
EOF

# Recreate the home page with its imports: the list of published posts
alder remove page Home
alder new page Home / --list Post
alder fill --decl Home.Model - <<'EOF'
{ posts: List Post }
EOF
alder fill --decl Home.load - <<'EOF'
\params session -> {
  let posts <- Db.from Post |> Db.where (Post.published.eq True) |> Db.orderBy Post.created.desc |> Db.all
  Query.succeed { posts: posts }
}
EOF
alder fill --decl Home.view - <<'EOF'
\model session -> <Layout.Page title="Blog">
  <h1>Blog</h1>
  <ul id="posts">
    {for p in model.posts -> <li><a href={Posts.Show { slug: p.slug }}>{p.title}</a></li>}
  </ul>
</Layout.Page>
EOF
alder new test Home "lists only published posts, newest first"
alder fill 'Home."lists only published posts, newest first"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Db.insert Post { slug: Post.Slug "old", title: Text.NonEmpty "Old", body: "b", published: True, author: me }
  let _ <- Db.insert Post { slug: Post.Slug "draft", title: Text.NonEmpty "Draft", body: "b", author: me }
  let _ <- Db.insert Post { slug: Post.Slug "new", title: Text.NonEmpty "New", body: "b", published: True, author: me }
  let page <- Test.get (Home {})
  Task.succeed (Test.texts "li a" page.doc == ["New", "Old"])
}
EOF

# The post page: the post, its comments, and the comment form
alder remove page Posts.Show
alder new page Posts.Show '/posts/{slug}' --show Post --by slug
alder new form Posts.Show NewComment author:Text.NonEmpty body:Text.NonEmpty --into Comment
alder fill --decl Posts.Show.Model - <<'EOF'
{ post: Post, comments: List Comment }
EOF
alder fill --decl Posts.Show.load - <<'EOF'
\params session -> {
  let found <- Db.from Post |> Db.where (Post.slug.eq params.slug) |> Db.first
  match found {
    Nothing -> Query.notFound,
    Just p -> if p.published || session.user == Just p.author then {
      let comments <- Db.from Comment |> Db.where (Comment.post.eq p.id) |> Db.orderBy Comment.created.asc |> Db.all
      Query.succeed { post: p, comments: comments }
    } else Query.notFound,
  }
}
EOF
alder fill --decl Posts.Show.update - <<'EOF'
\msg model session -> match msg {
  SubmitNewComment p -> {
    let _ <- Db.insert Comment { post: model.post.id, author: p.form.author, body: p.form.body }
    Task.succeed Rerender
  },
}
EOF
alder fill --decl Posts.Show.view - <<'EOF'
\model session -> <Layout.Page title={model.post.title}>
  <article>
    <h1>{model.post.title}</h1>
    <p>{model.post.body}</p>
  </article>
  <h2>Comments</h2>
  <ul id="comments">
    {for c in model.comments -> <li><strong>{c.author}</strong>: {c.body}</li>}
  </ul>
  <al-card>
    <form submit={SubmitNewComment}>
      <Errors form />
      <al-input field="author" label="Your name" />
      <al-textarea field="body" label="Comment" />
      <al-button type="submit">Post comment</al-button>
    </form>
  </al-card>
</Layout.Page>
EOF
alder new test Posts.Show "an unpublished post is not found for a reader"
alder fill 'Posts.Show."an unpublished post is not found for a reader"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Db.insert Post { slug: Post.Slug "draft", title: Text.NonEmpty "Draft", body: "b", author: me }
  let page <- Test.open (Posts.Show { slug: Post.Slug "draft" })
  Task.succeed (page.answer == PageMissing)
}
EOF
alder new test Posts.Show "a comment with an empty author is refused"
alder fill 'Posts.Show."a comment with an empty author is refused"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Db.insert Post { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "b", published: True, author: me }
  let _ <- Test.get (Posts.Show { slug: Post.Slug "hello" })
  let r <- Test.submitRaw NewComment [{ name: "author", value: "" }, { name: "body", value: "Nice" }]
  let n <- Db.from Comment |> Db.count
  Task.succeed ({ answer: r.answer, comments: n } == { answer: FormRejected [{ field: "author", message: "must not be empty" }], comments: 0 })
}
EOF

# The layout's navigation, now that both pages exist
alder fill --decl Layout.Page - <<'EOF'
\props -> <html>
  <head>
    <meta charset="utf-8" />
    <title>{props.title}</title>
  </head>
  <body>
    <main>
      <nav><a href={Home {}}>Blog</a> <a href={Admin {}}>Admin</a></nav>
      {props.children}
    </main>
  </body>
</html>
EOF

alder test
```
