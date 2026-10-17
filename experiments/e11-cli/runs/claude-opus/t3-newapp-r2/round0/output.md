```alder-cli
# the users and the posts
alder new table User email:Text.Email@unique name:Text.NonEmpty passwordHash:Text created:Time@created --no-timestamps
alder new type Post 'Slug = brand Text where Text.matches "^[a-z0-9-]+$"'
alder new table Post title:Text.NonEmpty slug:Slug@unique body:Text published:Bool=False author:User.UserId@ref=User created:Time@created --no-timestamps
alder new table Comment post:Post.PostId@ref=Post@cascade author:Text.NonEmpty body:Text.NonEmpty created:Time@created --no-timestamps

# the public pages
alder new page Home / --blank
alder new page Posts.Show '/posts/{slug}' --param slug:Post.Slug --blank

alder fill --decl Home.Model - <<'EOF'
{ posts: List Post }
EOF
alder fill Home.load - <<'EOF'
{
  let posts <- Db.from Post |> Db.where (Post.published.eq True) |> Db.orderBy Post.created.desc |> Db.all
  Query.succeed { posts: posts }
}
EOF
alder fill Home.view - <<'EOF'
<Layout.Page title="Blog">
  <h1>Blog</h1>
  <ul id="posts">
    {for post in model.posts -> <li><a href={Posts.Show { slug: post.slug }}>{post.title}</a></li>}
  </ul>
</Layout.Page>
EOF

# logging in
alder new auth --users User

# one post
alder fill --decl Posts.Show.Model - <<'EOF'
{ post: Post, comments: List Comment }
EOF
alder fill Posts.Show.load - <<'EOF'
{
  let found <- Db.from Post |> Db.where (Post.slug.eq params.slug) |> Db.first
  match found {
    Just post -> {
      if post.published || session.user == Just post.author then {
        let comments <- Db.from Comment |> Db.where (Comment.post.eq post.id) |> Db.orderBy Comment.created.asc |> Db.all
        Query.succeed { post: post, comments: comments }
      } else Query.notFound
    },
    Nothing -> Query.notFound,
  }
}
EOF
alder fill Posts.Show.view - <<'EOF'
<Layout.Page title={model.post.title}>
  <h1>{model.post.title}</h1>
</Layout.Page>
EOF
alder new form Posts.Show NewComment author:Text.NonEmpty body:Text.NonEmpty --msg AddComment
alder fill --decl Posts.Show.view - <<'EOF'
<Layout.Page title={model.post.title}>
  <h1>{model.post.title}</h1>
  <article>{model.post.body}</article>
  <h2>Comments</h2>
  <ul id="comments">
    {for c in model.comments -> <li><strong>{c.author}</strong> {c.body}</li>}
  </ul>
  <form submit={AddComment}>
    <Errors form />
    <Input field="author" label="Name" />
    <Errors field="author" />
    <Textarea field="body" label="Comment" />
    <Errors field="body" />
    <button>Add comment</button>
  </form>
</Layout.Page>
EOF
alder fill --decl Posts.Show.update - <<'EOF'
match msg {
  AddComment p -> {
    let _ <- Db.insert Comment { post: model.post.id, author: p.form.author, body: p.form.body }
    Task.succeed (Redirect (Posts.Show { slug: model.post.slug }))
  },
}
EOF

# the admin
alder new page Admin /admin --requires user --blank
alder fill --decl Admin.Model - <<'EOF'
{ posts: List Post }
EOF
alder fill Admin.load - <<'EOF'
{
  let posts <- Db.from Post |> Db.orderBy Post.created.desc |> Db.all
  Query.succeed { posts: posts }
}
EOF
alder fill Admin.view - <<'EOF'
<Layout.Page title="Admin">
  <h1>Admin</h1>
</Layout.Page>
EOF
alder new form Admin NewPost title:Text.NonEmpty slug:Post.Slug body:Text --msg CreatePost
alder new msg Admin TogglePublished id:Post.PostId
alder fill --decl Admin.view - <<'EOF'
<Layout.Page title="Admin">
  <h1>Admin</h1>
  <form submit={CreatePost}>
    <Errors form />
    <Input field="title" label="Title" />
    <Errors field="title" />
    <Input field="slug" label="Slug" />
    <Errors field="slug" />
    <Textarea field="body" label="Body" />
    <Errors field="body" />
    <button>Create post</button>
  </form>
  <ul id="posts">
    {for post in model.posts ->
      <li>
        <a href={Posts.Show { slug: post.slug }}>{post.title}</a>
        {if post.published then " (published)" else " (draft)"}
        <button click={TogglePublished { id: post.id }}>{if post.published then "Unpublish" else "Publish"}</button>
      </li>}
  </ul>
  <button click={App.Logout}>Log out</button>
</Layout.Page>
EOF
alder fill --decl Admin.update - <<'EOF'
match msg {
  CreatePost p -> {
    let taken <- Db.from Post |> Db.where (Post.slug.eq p.form.slug) |> Db.count
    if taken > 0 then Task.succeed (Reject "That slug is already taken.") else {
      let _ <- Db.insert Post { title: p.form.title, slug: p.form.slug, body: p.form.body, author: session.user }
      Task.succeed (Redirect (Admin {}))
    }
  },
  TogglePublished p -> {
    let _ <- Db.update Post p.id (\x -> { x | published: not x.published })
    Task.succeed Rerender
  },
}
EOF

# tests
alder new test Home "/ lists only published posts, newest first"
alder fill 'Home."/ lists only published posts, newest first"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Db.insert Post { title: Text.NonEmpty "Old", slug: Post.Slug "old", body: "a", published: True, author: me }
  let _ <- Db.insert Post { title: Text.NonEmpty "Draft", slug: Post.Slug "draft", body: "b", published: False, author: me }
  let _ <- Db.insert Post { title: Text.NonEmpty "New", slug: Post.Slug "new", body: "c", published: True, author: me }
  let page <- Test.get (Home {})
  Task.succeed (Test.texts "#posts a" page.doc == ["New", "Old"])
}
EOF
alder new test Posts.Show "an unpublished post is not found for a reader"
alder fill 'Posts.Show."an unpublished post is not found for a reader"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Db.insert Post { title: Text.NonEmpty "Draft", slug: Post.Slug "draft", body: "b", published: False, author: me }
  let hidden <- Test.open (Posts.Show { slug: Post.Slug "draft" })
  let _ <- Test.login { user: Just me }
  let mine <- Test.open (Posts.Show { slug: Post.Slug "draft" })
  Task.succeed ({ reader: hidden.answer, author: mine.answer } == { reader: PageMissing, author: Shown })
}
EOF
alder new test Posts.Show "a comment with an empty author is refused"
alder fill 'Posts.Show."a comment with an empty author is refused"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Db.insert Post { title: Text.NonEmpty "Hello", slug: Post.Slug "hello", body: "Hi", published: True, author: me }
  let _ <- Test.get (Posts.Show { slug: Post.Slug "hello" })
  let r <- Test.submitRaw NewComment [{ name: "author", value: "" }, { name: "body", value: "Nice" }]
  let n <- Db.from Comment |> Db.count
  Task.succeed ({ answer: r.answer, comments: n } == { answer: FormRejected [{ field: "author", message: "must not be empty" }], comments: 0 })
}
EOF
alder new test Admin "toggling publishes a post"
alder fill 'Admin."toggling publishes a post"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Post { title: Text.NonEmpty "Draft", slug: Post.Slug "draft", body: "b", published: False, author: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Admin {})
  let r <- Test.click (TogglePublished { id: id })
  let after <- Db.get Post id
  Task.succeed ({ answer: r.answer, published: Maybe.map (\x -> x.published) after } == { answer: Rerendered, published: Just True })
}
EOF

alder fill --list
alder check
```
