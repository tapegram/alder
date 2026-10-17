```alder-cli
# Posts.Index
alder fill Posts.Index.load - <<'EOF'
{
  let posts <- Db.from Post |> Db.where (Post.published.eq True) |> Db.orderBy Post.created.desc |> Db.all
  Query.succeed { posts: posts }
}
EOF
alder fill Posts.Index.view - <<'EOF'
<Layout.Page title="Blog">
  <h1>Blog</h1>
  <ul id="posts">
    {for p in model.posts -> <li><a href={Posts.Show { slug: p.slug }}>{p.title}</a></li>}
  </ul>
</Layout.Page>
EOF
alder new test Posts.Index "the front page lists only published posts, newest first"
alder fill 'Posts.Index."the front page lists only published posts, newest first"' - <<'EOF'
{
  let _ <- Db.insert Post { title: Text.NonEmpty "First", slug: Post.Slug "first", body: "a", published: True }
  let _ <- Db.insert Post { title: Text.NonEmpty "Hidden", slug: Post.Slug "hidden", body: "b", published: False }
  let _ <- Db.insert Post { title: Text.NonEmpty "Second", slug: Post.Slug "second", body: "c", published: True }
  let page <- Test.get (Posts.Index {})
  Task.succeed (Test.texts "li a" page.doc == ["Second", "First"])
}
EOF

# Posts.Show
alder fill --decl Posts.Show.Model - <<'EOF'
{ post: Post, comments: List Comment }
EOF
alder fill Posts.Show.load - <<'EOF'
{
  let found <- Db.from Post |> Db.where (Post.slug.eq params.slug) |> Db.first
  match found {
    Nothing -> Query.notFound,
    Just post -> {
      let ok = post.published || session.user != Nothing
      if ok then {
        let comments <- Db.from Comment |> Db.where (Comment.post.eq post.id) |> Db.orderBy Comment.created.asc |> Db.all
        Query.succeed { post: post, comments: comments }
      } else Query.notFound
    },
  }
}
EOF
alder fill Posts.Show.view - <<'EOF'
<Layout.Page title={model.post.title}>
  <h1>{model.post.title}</h1>
</Layout.Page>
EOF
alder new form Posts.Show NewComment author:Text.NonEmpty body:Text.NonEmpty --msg Submit
alder fill --decl Posts.Show.view - <<'EOF'
<Layout.Page title={model.post.title}>
  <h1>{model.post.title}</h1>
  <article>{model.post.body}</article>
  <h2>Comments</h2>
  <ul id="comments">
    {for c in model.comments -> <li><strong>{c.author}</strong> {c.body}</li>}
  </ul>
  <form submit={Submit}>
    <Errors form />
    <Input field="author" label="Name" />
    <Errors field="author" />
    <Textarea field="body" label="Comment" />
    <Errors field="body" />
    <button>Add comment</button>
  </form>
  <a href={Posts.Index {}}>Back</a>
</Layout.Page>
EOF
alder fill Posts.Show.update.Submit - <<'EOF'
{
  let _ <- Db.insert Comment { post: model.post.id, author: p.form.author, body: p.form.body }
  Task.succeed Rerender
}
EOF
alder new test Posts.Show "an unpublished post is not found for a reader"
alder fill 'Posts.Show."an unpublished post is not found for a reader"' - <<'EOF'
{
  let _ <- Db.insert Post { title: Text.NonEmpty "Draft", slug: Post.Slug "draft", body: "x", published: False }
  let r <- Test.open (Posts.Show { slug: Post.Slug "draft" })
  Task.succeed (r.answer == PageMissing)
}
EOF
alder new test Posts.Show "a comment with an empty author is refused"
alder fill 'Posts.Show."a comment with an empty author is refused"' - <<'EOF'
{
  let _ <- Db.insert Post { title: Text.NonEmpty "Hello", slug: Post.Slug "hello", body: "x", published: True }
  let _ <- Test.get (Posts.Show { slug: Post.Slug "hello" })
  let r <- Test.submitRaw NewComment [{ name: "author", value: "" }, { name: "body", value: "Hi" }]
  let n <- Db.from Comment |> Db.count
  Task.succeed ({ answer: r.answer, n: n } == { answer: FormRejected [{ field: "author", message: "must not be empty" }], n: 0 })
}
EOF

# Admin.Index
alder fill --decl Admin.Index.Model - <<'EOF'
{ posts: List Post }
EOF
alder fill Admin.Index.load - <<'EOF'
{
  let posts <- Db.from Post |> Db.orderBy Post.created.desc |> Db.all
  Query.succeed { posts: posts }
}
EOF
alder fill Admin.Index.view - <<'EOF'
<Layout.Page title="Admin">
  <h1>Admin</h1>
</Layout.Page>
EOF
alder new form Admin.Index NewPost title:Text.NonEmpty slug:Post.Slug body:Text published:Bool --msg Create
alder new msg Admin.Index Toggle id:Post.PostId
alder fill --decl Admin.Index.view - <<'EOF'
<Layout.Page title="Admin">
  <h1>Admin</h1>
  <form submit={Create}>
    <Errors form />
    <Input field="title" label="Title" />
    <Input field="slug" label="Slug" />
    <Textarea field="body" label="Body" />
    <Input field="published" label="Published" />
    <button>Create</button>
  </form>
  <ul id="posts">
    {for p in model.posts -> <li><a href={Posts.Show { slug: p.slug }}>{p.title}</a> {if p.published then "published" else "draft"} <button click={Toggle { id: p.id }}>Toggle published</button></li>}
  </ul>
  <button click={App.Logout}>Log out</button>
</Layout.Page>
EOF
alder fill Admin.Index.update.Create - <<'EOF'
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
alder check
```
