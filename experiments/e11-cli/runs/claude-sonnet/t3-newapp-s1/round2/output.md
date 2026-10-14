```alder-cli
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
    {for p in model.posts -> <li><a href={Posts.Show { slug: p.slug }}>{p.title}</a></li>}
  </ul>
</Layout.Page>
EOF
alder new test Home "lists only published posts, newest first"
alder fill 'Home."lists only published posts, newest first"' - <<'EOF'
{
  let _ <- Db.insert Post { title: Text.NonEmpty "First", slug: Slug.Slug "first", body: "a", published: True }
  let _ <- Db.insert Post { title: Text.NonEmpty "Draft", slug: Slug.Slug "draft", body: "b", published: False }
  let _ <- Db.insert Post { title: Text.NonEmpty "Second", slug: Slug.Slug "second", body: "c", published: True }
  let page <- Test.get (Home {})
  Task.succeed (Test.texts "#posts a" page.doc == ["Second", "First"])
}
EOF
alder fill --decl Posts.Show.Model - <<'EOF'
{ post: Post, comments: List Comment }
EOF
alder fill Posts.Show.load - <<'EOF'
{
  let found <- Db.from Post |> Db.where (Post.slug.eq params.slug) |> Db.first
  match found {
    Just post -> if post.published || session.user != Nothing then {
      let comments <- Db.from Comment |> Db.where (Comment.post.eq post.id) |> Db.orderBy Comment.created.asc |> Db.all
      Query.succeed { post: post, comments: comments }
    } else Query.notFound,
    Nothing -> Query.notFound,
  }
}
EOF
alder fill Posts.Show.view - <<'EOF'
<Layout.Page title="Post">
  <h1>Post</h1>
</Layout.Page>
EOF
alder new form Posts.Show NewComment author:Text.NonEmpty body:Text.NonEmpty --msg AddComment
alder fill --decl Posts.Show.view - <<'EOF'
<Layout.Page title={model.post.title}>
  <h1>{model.post.title}</h1>
  <article>{model.post.body}</article>
  <h2>Comments</h2>
  <ul id="comments">
    {for c in model.comments -> <li><strong>{c.author}</strong>: {c.body}</li>}
  </ul>
  <al-card>
    <form submit={AddComment}>
      <al-input field="author" label="Name" />
      <al-input field="body" label="Comment" />
      <al-button type="submit">Add comment</al-button>
    </form>
  </al-card>
</Layout.Page>
EOF
alder fill Posts.Show.update.AddComment - <<'EOF'
{
  let _ <- Db.insert Comment { post: model.post.id, author: p.form.author, body: p.form.body }
  Task.succeed Rerender
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
  let _ <- Db.insert Post { title: Text.NonEmpty "Hello", slug: Slug.Slug "hello", body: "b", published: True }
  let _ <- Test.get (Posts.Show { slug: Slug.Slug "hello" })
  let r <- Test.submitRaw NewComment [{ name: "author", value: "" }, { name: "body", value: "hi" }]
  let n <- Db.from Comment |> Db.count
  Task.succeed ({ answer: r.answer, n: n } == { answer: FormRejected [{ field: "author", message: "must not be empty" }], n: 0 })
}
EOF
alder fill --decl Desk.Model - <<'EOF'
{ posts: List Post }
EOF
alder fill Desk.load - <<'EOF'
{
  let posts <- Db.from Post |> Db.orderBy Post.created.desc |> Db.all
  Query.succeed { posts: posts }
}
EOF
alder fill Desk.view - <<'EOF'
<Layout.Page title="Admin">
  <h1>Admin</h1>
</Layout.Page>
EOF
alder new form Desk NewPost title:Text.NonEmpty slug:Slug.Slug body:Text published:Bool --msg CreatePost
alder new msg Desk Toggle id:Post.PostId
alder fill --decl Desk.view - <<'EOF'
<Layout.Page title="Admin">
  <h1>Admin</h1>
  <al-card>
    <form submit={CreatePost}>
      <al-input field="title" label="Title" />
      <al-input field="slug" label="Slug" />
      <al-textarea field="body" label="Body" />
      <al-checkbox field="published" label="Published" />
      <al-button type="submit">Create</al-button>
    </form>
  </al-card>
  <ul id="posts">
    {for p in model.posts ->
      <li>
        <a href={Posts.Show { slug: p.slug }}>{p.title}</a>
        <span>{if p.published then "published" else "draft"}</span>
        <al-button size="sm" click={Toggle { id: p.id }}>Toggle published</al-button>
      </li>}
  </ul>
  <al-button variant="outline" click={App.Logout}>Log out</al-button>
</Layout.Page>
EOF
alder fill Desk.update.CreatePost - <<'EOF'
{
  let _ <- Db.insert Post { title: p.form.title, slug: p.form.slug, body: p.form.body, published: p.form.published }
  Task.succeed Rerender
}
EOF
alder fill Desk.update.Toggle - <<'EOF'
{
  let _ <- Db.update Post p.id (\x -> { x | published: not x.published })
  Task.succeed Rerender
}
EOF
alder check
alder test
```
