```alder-cli
alder new table Post title:Text.NonEmpty slug:Slug.Slug@unique body:Text published:Bool=False
alder new table Comment post:Post.PostId@ref=Post@cascade author:Text.NonEmpty body:Text.NonEmpty
alder new page Home / --blank
alder new auth
alder new page Posts.Show '/posts/{slug}' --blank --param slug:Slug.Slug
alder new page Desk /admin --blank --requires user

alder fill --decl Home.Model - <<'EOF'
{ posts: List Post }
EOF
alder fill Home.load - <<'EOF'
\params session -> {
  let posts <- Db.from Post |> Db.where (Post.published.eq True) |> Db.orderBy Post.created.desc |> Db.all
  Query.succeed { posts: posts }
}
EOF
alder fill Home.view - <<'EOF'
\model session ->
  <Layout.Page title="Blog">
    <h1>Blog</h1>
    <ul id="posts">
      {for p in model.posts -> <li><a href={Posts.Show { slug: p.slug }}>{p.title}</a></li>}
    </ul>
    <a href={Desk {}}>Admin</a>
  </Layout.Page>
EOF

alder fill --decl Posts.Show.Model - <<'EOF'
{ post: Post, comments: List Comment }
EOF
alder fill Posts.Show.load - <<'EOF'
\params session -> {
  let found <- Db.from Post |> Db.where (Post.slug.eq params.slug) |> Db.first
  match found {
    Nothing -> Query.notFound,
    Just post -> if post.published || session.user != Nothing then {
      let comments <- Db.from Comment |> Db.where (Comment.post.eq post.id) |> Db.orderBy Comment.created.asc |> Db.all
      Query.succeed { post: post, comments: comments }
    } else Query.notFound,
  }
}
EOF
alder fill Posts.Show.view - <<'EOF'
\model session ->
  <Layout.Page title={model.post.title}>
    <h1>{model.post.title}</h1>
  </Layout.Page>
EOF
alder new form Posts.Show NewComment author:Text.NonEmpty body:Text.NonEmpty --msg SubmitComment
alder fill --decl Posts.Show.update - <<'EOF'
\msg model session -> match msg {
  SubmitComment p -> {
    let _ <- Db.insert Comment { post: model.post.id, author: p.form.author, body: p.form.body }
    Task.succeed (Redirect (Posts.Show { slug: model.post.slug }))
  },
}
EOF
alder fill --decl Posts.Show.view - <<'EOF'
\model session ->
  <Layout.Page title={model.post.title}>
    <h1>{model.post.title}</h1>
    <p>{model.post.body}</p>
    <h2>Comments</h2>
    <ul id="comments">
      {for c in model.comments -> <li><strong>{c.author}</strong>: {c.body}</li>}
    </ul>
    <form submit={SubmitComment}>
      <Errors form />
      <Input field="author" label="Name" />
      <Errors field="author" />
      <Input field="body" label="Comment" />
      <Errors field="body" />
      <button>Add comment</button>
    </form>
  </Layout.Page>
EOF

alder fill --decl Desk.Model - <<'EOF'
{ posts: List Post }
EOF
alder fill Desk.load - <<'EOF'
\params session -> {
  let posts <- Db.from Post |> Db.orderBy Post.created.desc |> Db.all
  Query.succeed { posts: posts }
}
EOF
alder fill Desk.view - <<'EOF'
\model session ->
  <Layout.Page title="Admin">
    <h1>Admin</h1>
  </Layout.Page>
EOF
alder new msg Desk Toggle id:Post.PostId
alder new form Desk NewPost title:Text.NonEmpty slug:Slug.Slug body:Text published:Bool --msg SubmitPost
alder fill --decl Desk.update - <<'EOF'
\msg model session -> match msg {
  Toggle p -> {
    let _ <- Db.update Post p.id (\x -> { x | published: not x.published })
    Task.succeed Rerender
  },
  SubmitPost p -> {
    let taken <- Db.from Post |> Db.where (Post.slug.eq p.form.slug) |> Db.first
    match taken {
      Just t -> Task.succeed (Reject "That slug is taken."),
      Nothing -> {
        let _ <- Db.insert Post { title: p.form.title, slug: p.form.slug, body: p.form.body, published: p.form.published }
        Task.succeed Rerender
      },
    }
  },
}
EOF
alder fill --decl Desk.view - <<'EOF'
\model session ->
  <Layout.Page title="Admin">
    <h1>Admin</h1>
    <form submit={SubmitPost}>
      <Errors form />
      <Input field="title" label="Title" />
      <Errors field="title" />
      <Input field="slug" label="Slug" />
      <Errors field="slug" />
      <Textarea field="body" label="Body" />
      <Input field="published" label="Published" />
      <button>Create post</button>
    </form>
    <ul id="posts">
      {for p in model.posts ->
        <li>
          <a href={Posts.Show { slug: p.slug }}>{p.title}</a>
          <button click={Toggle { id: p.id }}>{if p.published then "Unpublish" else "Publish"}</button>
        </li>}
    </ul>
    <button click={App.Logout}>Log out</button>
  </Layout.Page>
EOF

alder new test Home "lists only published posts, newest first"
alder fill 'Home."lists only published posts, newest first"' - <<'EOF'
{
  let _ <- Db.insert Post { title: Text.NonEmpty "First", slug: Slug.Slug "first", body: "a", published: True }
  let _ <- Db.insert Post { title: Text.NonEmpty "Hidden", slug: Slug.Slug "hidden", body: "b", published: False }
  let _ <- Db.insert Post { title: Text.NonEmpty "Second", slug: Slug.Slug "second", body: "c", published: True }
  let page <- Test.get (Home {})
  Task.succeed (Test.texts "#posts a" page.doc == ["Second", "First"])
}
EOF
alder new test Posts.Show "an unpublished post is not found for a reader"
alder fill 'Posts.Show."an unpublished post is not found for a reader"' - <<'EOF'
{
  let _ <- Db.insert Post { title: Text.NonEmpty "Draft", slug: Slug.Slug "draft", body: "b", published: False }
  let r <- Test.open (Posts.Show { slug: Slug.Slug "draft" })
  Task.succeed (r.answer == PageMissing)
}
EOF
alder new test Posts.Show "a comment with an empty author is refused"
alder fill 'Posts.Show."a comment with an empty author is refused"' - <<'EOF'
{
  let _ <- Db.insert Post { title: Text.NonEmpty "Hi", slug: Slug.Slug "hi", body: "x", published: True }
  let _ <- Test.get (Posts.Show { slug: Slug.Slug "hi" })
  let r <- Test.submitRaw NewComment [{ name: "author", value: "" }, { name: "body", value: "Nice" }]
  let n <- Db.from Comment |> Db.count
  Task.succeed (match r.answer {
    FormRejected es -> n == 0,
    _ -> False,
  })
}
EOF
alder check
alder test
```
