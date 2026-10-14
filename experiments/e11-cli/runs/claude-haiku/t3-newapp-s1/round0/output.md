```alder-cli
# the slug type, then the tables
alder new type Post 'Slug = brand Text where Text.matches "^[a-z0-9-]+$"'
alder new table User email:Text.Email@unique name:Text.NonEmpty passwordHash:Text created:Time@created --no-timestamps
alder new table Post title:Text.NonEmpty slug:Slug@unique body:Text published:Bool=False author:User.UserId@ref=User created:Time@created --no-timestamps
alder new table Comment post:Post.PostId@ref=Post@cascade author:Text.NonEmpty body:Text.NonEmpty created:Time@created --no-timestamps

# the layout
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

# the front page (created before the login, so the login returns to it)
alder new page Home / --list Post
alder fill --decl Home.Model - <<'EOF'
{ posts: List Post }
EOF
alder fill --decl Home.load - <<'EOF'
\params session -> {
  let posts <-
    Db.from Post
      |> Db.where (Post.published.eq True)
      |> Db.orderBy Post.created.desc
      |> Db.all
  Query.succeed { posts: posts }
}
EOF

# the login, logout, and the session's user
alder new auth --users User

# a post page with its comments
alder new page Posts.Show '/posts/{slug}' --param slug:Post.Slug --list Comment
alder fill --decl Posts.Show.Model - <<'EOF'
{ post: Post, comments: List Comment }
EOF
alder fill --decl Posts.Show.load - <<'EOF'
\params session -> {
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
alder new form Posts.Show NewComment author:Text.NonEmpty body:Text.NonEmpty
alder fill --decl Posts.Show.view - <<'EOF'
\model session -> <Layout.Page title={model.post.title}>
    <h1>{model.post.title}</h1>
    <p>{model.post.body}</p>
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
    <a href={Home {}}>All posts</a>
  </Layout.Page>
EOF
alder fill --decl Posts.Show.update - <<'EOF'
\msg model session -> match msg {
  SubmitNewComment p -> {
    let _ <- Db.insert Comment { post: model.post.id, author: p.form.author, body: p.form.body }
    Task.succeed Rerender
  },
}
EOF

# the admin page: every post, a form to create one, and a toggle per post
alder new page Admin /admin --list Post --requires user
alder new form Admin NewPost title:Text.NonEmpty slug:Post.Slug body:Text
alder new msg Admin Toggle id:Post.PostId
alder new decl Admin Row "{ post: Post } -> Html Msg"
alder fill --decl Admin.Model - <<'EOF'
{ posts: List Post }
EOF
alder fill --decl Admin.load - <<'EOF'
\params session -> {
  let posts <-
    Db.from Post
      |> Db.orderBy Post.created.desc
      |> Db.all
  Query.succeed { posts: posts }
}
EOF
alder fill --decl Admin.update - <<'EOF'
\msg model session -> match msg {
  SubmitNewPost p -> {
    let _ <- Db.insert Post { title: p.form.title, slug: p.form.slug, body: p.form.body, published: False, author: session.user }
    Task.succeed (Redirect (Admin {}))
  },
  Toggle p -> {
    let found <- Db.get Post p.id
    match found {
      Just post -> if post.author == session.user then {
        let _ <- Db.update Post p.id (\x -> { x | published: not x.published })
        Task.succeed Rerender
      } else Task.notFound,
      Nothing -> Task.notFound,
    }
  },
}
EOF
alder fill --decl Admin.Row - <<'EOF'
\props -> <li>
    <a href={Posts.Show { slug: props.post.slug }}>{props.post.title}</a>
    <span>{if props.post.published then "published" else "draft"}</span>
    <al-button variant="secondary" size="sm" click={Toggle { id: props.post.id }}>{if props.post.published then "Unpublish" else "Publish"}</al-button>
  </li>
EOF
alder fill --decl Admin.view - <<'EOF'
\model session -> <Layout.Page title="Admin">
    <h1>Posts</h1>
    <al-card>
      <form submit={SubmitNewPost}>
        <Errors form />
        <al-input field="title" label="Title" />
        <al-input field="slug" label="Slug" />
        <al-textarea field="body" label="Body" />
        <al-button type="submit">Create post</al-button>
      </form>
    </al-card>
    <ul id="posts">
      {for post in model.posts -> <Row post={post} />}
    </ul>
    <al-button variant="outline" click={App.Logout}>Log out</al-button>
  </Layout.Page>
EOF

# the front page's view, now that the login, the post page and the admin exist
alder fill --decl Home.view - <<'EOF'
\model session -> <Layout.Page title="Blog">
    <h1>Blog</h1>
    <ul id="posts">
      {for post in model.posts -> <li><a href={Posts.Show { slug: post.slug }}>{post.title}</a></li>}
    </ul>
    <p><a href={Admin {}}>Admin</a></p>
    {if session.user == Nothing then <a href={Auth.Login {}}>Log in</a> else <al-button variant="outline" click={App.Logout}>Log out</al-button>}
  </Layout.Page>
EOF

# tests: an unpublished post is not found for a reader
alder new test Posts.Show "an unpublished post is not found for a reader"
alder fill 'Posts.Show."an unpublished post is not found for a reader"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Db.insert Post { title: Text.NonEmpty "Draft", slug: Post.Slug "draft", body: "Not yet.", published: False, author: me }
  let r <- Test.open (Posts.Show { slug: Post.Slug "draft" })
  Task.succeed (r.answer == PageMissing)
}
EOF

# tests: a comment with an empty author is refused
alder new test Posts.Show "a comment with an empty author is refused"
alder fill 'Posts.Show."a comment with an empty author is refused"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Db.insert Post { title: Text.NonEmpty "Hello", slug: Post.Slug "hello", body: "First.", published: True, author: me }
  let _ <- Test.get (Posts.Show { slug: Post.Slug "hello" })
  let r <- Test.submitRaw NewComment [{ name: "author", value: "" }, { name: "body", value: "Nice post." }]
  let n <- Db.from Comment |> Db.count
  let rejected = match r.answer {
    FormRejected errs -> List.length errs > 0,
    _ -> False,
  }
  Task.succeed (rejected && n == 0)
}
EOF

# tests: the front page lists only published posts, newest first
alder new test Home "the front page lists only published posts, newest first"
alder fill 'Home."the front page lists only published posts, newest first"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Db.insert Post { title: Text.NonEmpty "Old", slug: Post.Slug "old", body: "First.", published: True, author: me }
  let _ <- Db.insert Post { title: Text.NonEmpty "Draft", slug: Post.Slug "draft", body: "Hidden.", published: False, author: me }
  let _ <- Db.insert Post { title: Text.NonEmpty "New", slug: Post.Slug "new", body: "Second.", published: True, author: me }
  let front <- Test.get (Home {})
  Task.succeed (Test.texts "#posts a" front.doc == ["New", "Old"])
}
EOF
```
