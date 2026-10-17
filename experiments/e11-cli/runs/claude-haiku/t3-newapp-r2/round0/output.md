```alder-cli
# the users, and the posts and their comments
alder new table User email:Text.Email@unique name:Text.NonEmpty passwordHash:Text created:Time@created --no-timestamps
alder new type Post 'Slug = brand Text where Text.matches "^[a-z0-9-]+$"'
alder new table Post slug:Post.Slug@unique title:Text.NonEmpty body:Text published:Bool=False author:User.UserId@ref=User
alder new table Comment post:Post.PostId@ref=Post@cascade author:Text.NonEmpty body:Text.NonEmpty

# the pages: the list at /, one post at /posts/{slug}, the admin at /admin
alder new page Home / --blank
alder new page Posts.Show '/posts/{slug}' --param slug:Post.Slug --blank
alder new page Admin /admin --blank --requires user

# the login (/login) and logout; after login the visitor goes to / (Home)
alder new auth --users User

# the layout and the failure pages
alder fill --decl Layout.Page - <<'EOF'
\props -> <html>
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

alder fill --decl App.error - <<'EOF'
\err -> <Layout.Page title="Error">
  <h1>Something went wrong</h1>
  <p>Please try again.</p>
</Layout.Page>
EOF

alder fill --decl App.notFound - <<'EOF'
<Layout.Page title="Not found">
  <h1>Not found</h1>
</Layout.Page>
EOF

# Home: the published posts, newest first
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
  <p><a href={Admin {}}>Admin</a></p>
</Layout.Page>
EOF

# Posts.Show: one post, its comments, and the comment form
alder fill --decl Posts.Show.Model - <<'EOF'
{ post: Post, comments: List Comment }
EOF

alder fill --decl Posts.Show.load - <<'EOF'
\params session -> {
  let found <- Db.from Post |> Db.where (Post.slug.eq params.slug) |> Db.first
  match found {
    Nothing -> Query.notFound,
    Just post -> {
      let comments <- Db.from Comment |> Db.where (Comment.post.eq post.id) |> Db.orderBy Comment.created.asc |> Db.all
      let mine = session.user == Just post.author
      if post.published || mine then Query.succeed { post: post, comments: comments } else Query.notFound
    },
  }
}
EOF

alder fill --decl Posts.Show.view - <<'EOF'
\model session -> <Layout.Page title={model.post.title}><h1>{model.post.title}</h1></Layout.Page>
EOF

alder new form Posts.Show NewComment author:Text.NonEmpty body:Text.NonEmpty --msg AddComment

alder fill --decl Posts.Show.view - <<'EOF'
\model session -> <Layout.Page title={model.post.title}>
  <h1>{model.post.title}</h1>
  <p>{model.post.body}</p>
  <h2>Comments</h2>
  <ul id="comments">
    {for c in model.comments -> <li><strong>{c.author}</strong>: {c.body}</li>}
  </ul>
  <al-card>
    <form submit={AddComment}>
      <Errors form />
      <al-input field="author" label="Your name" />
      <al-textarea field="body" label="Comment" />
      <al-button type="submit">Post comment</al-button>
    </form>
  </al-card>
  <p><a href={Home {}}>All posts</a></p>
</Layout.Page>
EOF

alder fill --decl Posts.Show.update - <<'EOF'
\msg model session -> match msg {
  AddComment p -> {
    let _ <- Db.insert Comment { post: model.post.id, author: p.form.author, body: p.form.body }
    Task.succeed Rerender
  },
}
EOF

# Admin: every post, a form to create one, and a button to publish or unpublish each
alder fill --decl Admin.Model - <<'EOF'
{ posts: List Post }
EOF

alder fill --decl Admin.load - <<'EOF'
\params session -> {
  let posts <- Db.from Post |> Db.orderBy Post.created.desc |> Db.all
  Query.succeed { posts: posts }
}
EOF

alder fill --decl Admin.view - <<'EOF'
\model session -> <Layout.Page title="Admin"><h1>Posts</h1></Layout.Page>
EOF

alder new form Admin NewPost slug:Post.Slug title:Text.NonEmpty body:Text --msg CreatePost
alder new msg Admin TogglePublished id:Post.PostId

alder fill --decl Admin.view - <<'EOF'
\model session -> <Layout.Page title="Admin">
  <h1>Posts</h1>
  <al-card>
    <form submit={CreatePost}>
      <Errors form />
      <al-input field="slug" label="Slug" />
      <al-input field="title" label="Title" />
      <al-textarea field="body" label="Body" />
      <al-button type="submit">Create post</al-button>
    </form>
  </al-card>
  <ul id="posts">
    {for p in model.posts -> <li><a href={Posts.Show { slug: p.slug }}>{p.title}</a> {if p.published then "published" else "draft"} <al-button variant="secondary" size="sm" click={TogglePublished { id: p.id }}>{if p.published then "Unpublish" else "Publish"}</al-button></li>}
  </ul>
  <al-button variant="outline" click={App.Logout}>Log out</al-button>
  <p><a href={Home {}}>Home</a></p>
</Layout.Page>
EOF

alder fill --decl Admin.update - <<'EOF'
\msg model session -> match msg {
  CreatePost p -> {
    let _ <- Db.insert Post { slug: p.form.slug, title: p.form.title, body: p.form.body, author: session.user }
    Task.succeed (Redirect (Admin {}))
  },
  TogglePublished p -> {
    let _ <- Db.update Post p.id (\post -> { post | published: not post.published })
    Task.succeed Rerender
  },
}
EOF

# tests: the list, the post page, and the admin
alder new test Home "lists only published posts, newest first"
alder fill 'Home."lists only published posts, newest first"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Db.insert Post { slug: Post.Slug "older", title: Text.NonEmpty "Older", body: "First.", author: me, published: True }
  let _ <- Db.insert Post { slug: Post.Slug "draft", title: Text.NonEmpty "Draft", body: "Hidden.", author: me, published: False }
  let _ <- Db.insert Post { slug: Post.Slug "newer", title: Text.NonEmpty "Newer", body: "Second.", author: me, published: True }
  let page <- Test.get (Home {})
  Task.succeed (Test.texts "#posts a" page.doc == ["Newer", "Older"])
}
EOF

alder new test Posts.Show "an unpublished post is not found for a reader"
alder fill 'Posts.Show."an unpublished post is not found for a reader"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Db.insert Post { slug: Post.Slug "draft", title: Text.NonEmpty "Draft", body: "Hidden.", author: me, published: False }
  let page <- Test.open (Posts.Show { slug: Post.Slug "draft" })
  Task.succeed (page.answer == PageMissing)
}
EOF

alder new test Posts.Show "the author sees an unpublished post"
alder fill 'Posts.Show."the author sees an unpublished post"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Db.insert Post { slug: Post.Slug "draft", title: Text.NonEmpty "Draft", body: "Hidden.", author: me, published: False }
  let _ <- Test.login { user: Just me }
  let page <- Test.open (Posts.Show { slug: Post.Slug "draft" })
  Task.succeed (page.answer == Shown)
}
EOF

alder new test Posts.Show "a comment with an empty author is refused"
alder fill 'Posts.Show."a comment with an empty author is refused"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Db.insert Post { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "First post.", author: me, published: True }
  let _ <- Test.get (Posts.Show { slug: Post.Slug "hello" })
  let r <- Test.submitRaw NewComment [{ name: "author", value: "" }, { name: "body", value: "Nice." }]
  let comments <- Db.from Comment |> Db.count
  Task.succeed ({ answer: r.answer, comments: comments } == { answer: FormRejected [{ field: "author", message: "must not be empty" }], comments: 0 })
}
EOF

alder new test Posts.Show "a comment is listed under the post"
alder fill 'Posts.Show."a comment is listed under the post"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Db.insert Post { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "First post.", author: me, published: True }
  let _ <- Test.get (Posts.Show { slug: Post.Slug "hello" })
  let r <- Test.submit NewComment { author: Text.NonEmpty "Ann", body: Text.NonEmpty "Nice." }
  Task.succeed ({ answer: r.answer, shown: Test.texts "#comments li" r.doc } == { answer: Rerendered, shown: ["Ann: Nice."] })
}
EOF

alder new test Admin "a new post is a draft"
alder fill 'Admin."a new post is a draft"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Admin {})
  let r <- Test.submit NewPost { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "First post." }
  let live <- Db.from Post |> Db.where (Post.published.eq True) |> Db.count
  let drafts <- Db.from Post |> Db.count
  Task.succeed ({ answer: r.answer, live: live, drafts: drafts } == { answer: Redirected (Admin {}), live: 0, drafts: 1 })
}
EOF

alder new test Admin "toggling publishes a post"
alder fill 'Admin."toggling publishes a post"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Post { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "First post.", author: me, published: False }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Admin {})
  let r <- Test.click (TogglePublished { id: id })
  let after <- Db.get Post id
  Task.succeed ({ answer: r.answer, published: Maybe.map (\p -> p.published) after } == { answer: Rerendered, published: Just True })
}
EOF
```
