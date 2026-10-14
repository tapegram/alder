```alder-cli
alder new type Slug 'Slug = brand Text where Text.matches "^[a-z0-9-]+$"'
alder new table Post title:Text.NonEmpty slug:Slug@unique body:Text published:Bool=False
alder new table Comment post:Post.PostId@ref=Post@cascade author:Text.NonEmpty body:Text.NonEmpty

alder new page Home / --blank
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
    <ul>
      {for post in model.posts -> <li><a href={Posts.Show { slug: post.slug }}>{post.title}</a></li>}
    </ul>
  </Layout.Page>
EOF

alder new auth --to Home

alder new page Posts.Show /posts/{slug} --blank --param slug:Slug
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
    <div>{model.post.body}</div>
    <h2>Comments</h2>
    <ul>
      {for c in model.comments -> <li><strong>{c.author}</strong>: {c.body}</li>}
    </ul>
  </Layout.Page>
EOF
alder new form Posts.Show NewComment author:Text.NonEmpty body:Text.NonEmpty --msg AddComment
alder fill --decl Posts.Show.view - <<'EOF'
\model session ->
  <Layout.Page title={model.post.title}>
    <h1>{model.post.title}</h1>
    <div>{model.post.body}</div>
    <h2>Comments</h2>
    <ul>
      {for c in model.comments -> <li><strong>{c.author}</strong>: {c.body}</li>}
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
alder fill Posts.Show.update.AddComment - <<'EOF'
{
  let _ <- Db.insert Comment { post: model.post.id, author: p.form.author, body: p.form.body }
  Task.succeed Rerender
}
EOF

alder new page Admin.Index /admin --blank --requires user
alder fill --decl Admin.Index.Model - <<'EOF'
{ posts: List Post }
EOF
alder fill Admin.Index.load - <<'EOF'
\params session -> {
  let posts <- Db.from Post |> Db.orderBy Post.created.desc |> Db.all
  Query.succeed { posts: posts }
}
EOF
alder fill Admin.Index.view - <<'EOF'
\model session ->
  <Layout.Page title="Admin">
    <h1>Admin</h1>
  </Layout.Page>
EOF
alder new form Admin.Index NewPost title:Text.NonEmpty slug:Slug body:Text published:Bool --msg CreatePost
alder new msg Admin.Index Toggle id:Post.PostId
alder fill --decl Admin.Index.view - <<'EOF'
\model session ->
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
      <Input field="published" label="Published" />
      <button>Create</button>
    </form>
    <ul>
      {for post in model.posts ->
        <li>
          <a href={Posts.Show { slug: post.slug }}>{post.title}</a>
          <button click={Toggle { id: post.id }}>{if post.published then "Unpublish" else "Publish"}</button>
        </li>}
    </ul>
    <button click={App.Logout}>Log out</button>
  </Layout.Page>
EOF
alder fill Admin.Index.update.CreatePost - <<'EOF'
{
  let _ <- Db.insert Post { title: p.form.title, slug: p.form.slug, body: p.form.body, published: p.form.published }
  Task.succeed Rerender
}
EOF
alder fill Admin.Index.update.Toggle - <<'EOF'
{
  let _ <- Db.update Post p.id (\x -> { x | published: not x.published })
  Task.succeed Rerender
}
EOF

alder new test Posts.Show "an unpublished post is not found for a reader"
alder fill 'Posts.Show."an unpublished post is not found for a reader"' - <<'EOF'
{
  let _ <- Db.insert Post { title: Text.NonEmpty "Draft", slug: Slug "draft", body: "b", published: False }
  let r <- Test.open (Posts.Show { slug: Slug "draft" })
  Task.succeed (r.answer == PageMissing)
}
EOF
alder new test Posts.Show "a comment with an empty author is refused"
alder fill 'Posts.Show."a comment with an empty author is refused"' - <<'EOF'
{
  let _ <- Db.insert Post { title: Text.NonEmpty "Hi", slug: Slug "hi", body: "b", published: True }
  let _ <- Test.get (Posts.Show { slug: Slug "hi" })
  let r <- Test.submitRaw NewComment [{ name: "author", value: "" }, { name: "body", value: "x" }]
  let n <- Db.from Comment |> Db.count
  let refused = match r.answer {
    FormRejected es -> True,
    _ -> False,
  }
  Task.succeed (refused && n == 0)
}
EOF
alder new test Home "lists only published posts newest first"
alder fill 'Home."lists only published posts newest first"' - <<'EOF'
{
  let _ <- Db.insert Post { title: Text.NonEmpty "First", slug: Slug "first", body: "a", published: True }
  let _ <- Db.insert Post { title: Text.NonEmpty "Hidden", slug: Slug "hidden", body: "b", published: False }
  let _ <- Db.insert Post { title: Text.NonEmpty "Third", slug: Slug "third", body: "c", published: True }
  let page <- Test.get (Home {})
  Task.succeed (Test.texts "li a" page.doc == ["Third", "First"])
}
EOF
alder check
alder test
```
