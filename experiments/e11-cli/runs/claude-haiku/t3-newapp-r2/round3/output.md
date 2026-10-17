```alder-cli
# 1. Log in goes to the admin page, not to Home, so Home can be regenerated as the post list
alder fill --decl Auth.Login.update - <<'EOF'
\msg model session -> match msg {
  Submit p -> {
    let found <- Password.verify { email: p.form.email, password: p.form.password }
    match found {
      Just id -> {
        let _ <- Session.put (\s -> { s | user: Just id })
        Task.succeed (Redirect (Admin { page: 1 }))
      },
      Nothing -> Task.succeed (Reject "Email or password is incorrect."),
    }
  },
}
EOF
alder fill 'Auth.Login."logging in through the form"' - <<'EOF'
{
  let _ <- Test.fake Password
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.get (Auth.Login {})
  let r <- Test.submit Credentials { email: Text.Email "me@example.com", password: Text.Secret "secret" }
  Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (Admin { page: 1 }), user: Just me })
}
EOF

# 2. Replace the blank pages (they have no imports) with list-shaped pages that import their tables
alder remove page Home
alder remove page Posts.Show
alder new page Home / --list Post
alder new page Posts.Show '/posts/{slug}' --list Comment --param slug:Post.Slug

# 3. Admin: a create form and a toggle message
alder new form Admin NewPost slug:Post.Slug title:Text.NonEmpty body:Text published:Bool
alder new msg Admin Toggle id:Post.PostId

# 4. Comment form on the post page
alder new form Posts.Show AddComment author:Text.NonEmpty body:Text.NonEmpty

# 5. Admin: all posts, the create form, the toggle buttons
alder fill --decl Admin.load - <<'EOF'
\params session -> {
  let rows <- Db.from Post |> Db.orderBy Post.created.desc |> Db.all
  Query.succeed { rows: rows, page: 1, pages: 1 }
}
EOF
alder fill --decl Admin.update - <<'EOF'
\msg model session -> match msg {
  SubmitNewPost p -> {
    let _ <- Db.insert Post { slug: p.form.slug, title: p.form.title, body: p.form.body, published: p.form.published, author: session.user }
    Task.succeed (Redirect (Admin { page: 1 }))
  },
  Toggle p -> {
    let _ <- Db.update Post p.id (\x -> { x | published: not x.published })
    Task.succeed Rerender
  },
}
EOF
alder fill --decl Admin.view - <<'EOF'
\model session -> <Layout.Page title="Admin">
    <h1>Posts</h1>
    <al-card>
      <form submit={SubmitNewPost}>
        <Errors form />
        <al-input field="slug" label="Slug" />
        <al-input field="title" label="Title" />
        <al-textarea field="body" label="Body" />
        <al-checkbox field="published" label="Published" />
        <al-button type="submit">Create post</al-button>
      </form>
    </al-card>
    <ul id="posts">
      {for post in model.rows ->
        <li>
          <span>{post.title}</span>
          {if post.published then <al-badge>Published</al-badge> else <al-badge variant="secondary">Draft</al-badge>}
          <al-button variant="secondary" size="sm" click={Toggle { id: post.id }}>{if post.published then "Unpublish" else "Publish"}</al-button>
        </li>}
    </ul>
    <al-button variant="outline" click={App.Logout}>Log out</al-button>
  </Layout.Page>
EOF

# 6. Home: published posts, newest first
alder fill --decl Home.load - <<'EOF'
\params session -> {
  let rows <- Db.from Post |> Db.where (Post.published.eq True) |> Db.orderBy Post.created.desc |> Db.all
  Query.succeed { rows: rows, page: 1, pages: 1 }
}
EOF
alder fill --decl Home.view - <<'EOF'
\model session -> <Layout.Page title="Posts">
    <h1>Posts</h1>
    <ul>
      {for post in model.rows -> <li><a href={Posts.Show { slug: post.slug }}>{post.title}</a></li>}
    </ul>
  </Layout.Page>
EOF

# 7. Post page: the model, the load (404 for an unpublished post unless the reader wrote it), the view, the comment update
alder fill --decl Posts.Show.Model - <<'EOF'
{ postId: Post.PostId, title: Text, body: Text, rows: List Comment, page: Int, pages: Int }
EOF
alder fill Posts.Show.load - --hole ?load <<'EOF'
{
  let found <- Db.from Post |> Db.where (Post.slug.eq params.slug) |> Db.first
  match found {
    Nothing -> Query.notFound,
    Just post -> if post.published || session.user == Just post.author then {
        let comments <- Db.from Comment |> Db.where (Comment.post.eq post.id) |> Db.orderBy Comment.created.asc |> Db.all
        Query.succeed { postId: post.id, title: post.title, body: post.body, rows: comments, page: 1, pages: 1 }
      } else Query.notFound,
  }
}
EOF
alder fill --decl Posts.Show.view - <<'EOF'
\model session -> <Layout.Page title={model.title}>
    <h1>{model.title}</h1>
    <p>{model.body}</p>
    <ul id="comments">
      {for c in model.rows -> <li><strong>{c.author}</strong> {c.body}</li>}
    </ul>
    <al-card>
      <form submit={SubmitAddComment}>
        <Errors form />
        <al-input field="author" label="Your name" />
        <al-textarea field="body" label="Comment" />
        <al-button type="submit">Add comment</al-button>
      </form>
    </al-card>
  </Layout.Page>
EOF
alder fill --decl Posts.Show.update - <<'EOF'
\msg model session -> match msg {
  SubmitAddComment p -> {
    let _ <- Db.insert Comment { post: model.postId, author: p.form.author, body: p.form.body }
    Task.succeed Rerender
  },
}
EOF

# 8. Tests, each in the module it tests
alder new test Home "lists only published posts, newest first"
alder fill 'Home."lists only published posts, newest first"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Db.insert Post { slug: Post.Slug "first", title: Text.NonEmpty "First", body: "", published: True, author: me }
  let _ <- Db.insert Post { slug: Post.Slug "draft", title: Text.NonEmpty "Draft", body: "", published: False, author: me }
  let _ <- Db.insert Post { slug: Post.Slug "second", title: Text.NonEmpty "Second", body: "", published: True, author: me }
  let shown <- Test.get (Home {})
  Task.succeed (List.map (\p -> p.title) shown.model.rows == [Text.NonEmpty "Second", Text.NonEmpty "First"])
}
EOF

alder new test Posts.Show "an unpublished post is not found for a reader"
alder fill 'Posts.Show."an unpublished post is not found for a reader"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Db.insert Post { slug: Post.Slug "secret", title: Text.NonEmpty "Secret", body: "", published: False, author: me }
  let shown <- Test.open (Posts.Show { slug: Post.Slug "secret" })
  Task.succeed (shown.answer == PageMissing)
}
EOF

alder new test Posts.Show "a comment with an empty author is refused"
alder fill 'Posts.Show."a comment with an empty author is refused"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Db.insert Post { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "", published: True, author: me }
  let _ <- Test.get (Posts.Show { slug: Post.Slug "hello" })
  let r <- Test.submitRaw AddComment [{ name: "author", value: "" }, { name: "body", value: "Nice post" }]
  let n <- Db.from Comment |> Db.count
  let refused = match r.answer { FormRejected errs -> List.length errs == 1, _ -> False }
  Task.succeed (refused && n == 0)
}
EOF

alder test
```
