```alder-cli
# the field, and its migration
alder edit table Post add-field published:Bool=False
alder migrate

# the front page lists only published posts
alder fill --decl Posts.Index.load - <<'EOF'
{
  let page = if params.page < 1 then 1 else if params.page > lastPage then lastPage else params.page
  let q = Text.slice 0 100 (Text.trim params.q)
  let matching =
    if Text.isEmpty q then Db.and [] else Db.or [Post.title.contains q, Post.body.contains q]
  let found <-
    Db.from Post
      |> Db.where (Post.published.eq True)
      |> Db.where matching
      |> Db.orderBy Post.created.desc
      |> Db.limit (perPage + 1)
      |> Db.offset ((page - 1) * perPage)
      |> Db.all
  let me <- match session.user {
    Just id -> User.author id,
    Nothing -> Query.succeed Nothing,
  }
  let canWrite = me != Nothing
  Query.succeed { page: page, q: q, posts: List.take perPage found, hasMore: List.length found > perPage, canWrite: canWrite }
}
EOF

# a draft's page is a 404 for anyone but its author
alder fill --decl Posts.Show.load - <<'EOF'
{
  let found <- Post.bySlug params.slug
  match found {
    Just post -> if post.published || session.user == Just post.author then {
      let body <- Markdown.render post.body
      let comments <-
        Db.from Comment
          |> Db.where (Comment.post.eq post.id)
          |> Db.orderBy Comment.created.asc
          |> Db.all
      Query.succeed { post: post, body: body, comments: comments }
    } else Query.notFound,
    Nothing -> Query.notFound,
  }
}
EOF

# publishing from the dashboard
alder new msg Admin Publish id:Post.PostId
alder fill Admin.update.Publish - <<'EOF'
{
  let found <- Db.from Post |> Db.where (Post.id.eq p.id) |> Db.where (Post.author.eq model.author.id) |> Db.first
  match found {
    Just post -> {
      let _ <- Db.update Post post.id (\x -> { x | published: True })
      Task.succeed (Redirect (Admin {}))
    },
    Nothing -> Task.notFound,
  }
}
EOF
alder fill --decl Admin.view - <<'EOF'
<Layout.Page title="Your posts">
  <h1>Your posts</h1>

  <h2>Drafts</h2>
  <ul id="drafts">
    {for post in List.filter (\x -> not x.published) model.posts ->
      <li>
        <a class="title" href={Admin.Edit { id: post.id }}>{post.title}</a>
        {" "}
        <button click={Publish { id: post.id }}>Publish</button>
      </li>}
  </ul>

  <h2>All posts</h2>
  <ul class="posts">
    {for post in model.posts ->
      <li>
        <a href={Admin.Edit { id: post.id }}>{post.title}</a>
      </li>}
  </ul>

  <h2>New post</h2>
  <form submit={Create}>
    <Errors form />
    <Input field="slug" label="Slug (lowercase letters, digits and hyphens)" />
    <Errors field="slug" />
    <Input field="title" label="Title" />
    <Errors field="title" />
    <Textarea field="body" label="Body (Markdown)" />
    <Errors field="body" />
    <button>Create post</button>
  </form>

  <button click={App.Logout}>Log out</button>
</Layout.Page>
EOF

# existing front-page tests: their posts are published
alder fill --decl 'Posts.Index."the front page lists the posts, newest first"' - <<'EOF'
{
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "first", title: Text.NonEmpty "First", body: "", author: author, published: True }
  let _ <- Db.insert Post { slug: Post.Slug "second", title: Text.NonEmpty "Second", body: "", author: author, published: True }
  let model <- load { page: 1, q: "" } { user: Nothing }
  Task.succeed (List.map (\p -> p.title) model.posts == [Text.NonEmpty "Second", Text.NonEmpty "First"])
}
EOF
alder fill --decl 'Posts.Index."search matches title or body"' - <<'EOF'
{
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "tomatoes", title: Text.NonEmpty "Tomato season", body: "It is here.", author: author, published: True }
  let _ <- Db.insert Post { slug: Post.Slug "soup", title: Text.NonEmpty "Soup", body: "Use ripe tomatoes.", author: author, published: True }
  let _ <- Db.insert Post { slug: Post.Slug "trains", title: Text.NonEmpty "Trains", body: "Fast ones.", author: author, published: True }
  let model <- load { page: 1, q: "TOMATO" } { user: Nothing }
  let titles = List.map (\p -> p.title) model.posts
  Task.succeed ({ count: List.length titles, title: List.member (Text.NonEmpty "Tomato season") titles, body: List.member (Text.NonEmpty "Soup") titles } == { count: 2, title: True, body: True })
}
EOF
alder fill --decl 'Posts.Index."a `%` in the query is literal"' - <<'EOF'
{
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "done", title: Text.NonEmpty "100% done", body: "", author: author, published: True }
  let _ <- Db.insert Post { slug: Post.Slug "ways", title: Text.NonEmpty "1000 ways", body: "", author: author, published: True }
  let model <- load { page: 1, q: "100%" } { user: Nothing }
  Task.succeed (List.map (\p -> p.title) model.posts == [Text.NonEmpty "100% done"])
}
EOF
alder fill --decl 'Posts.Index."page 99 is empty"' - <<'EOF'
{
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Task.all (List.map (\n -> Db.insert Post { slug: Post.Slug ("post-" ++ Text.fromInt n), title: Text.NonEmpty "A post", body: "", author: author, published: True }) (List.range 1 11))
  let pages <- Query.all (List.map (\n -> load { page: n, q: "" } { user: Nothing }) [1, 2, 99, 0])
  let seen = List.map (\m -> { page: m.page, posts: List.length m.posts, hasMore: m.hasMore }) pages
  Task.succeed (seen == [{ page: 1, posts: 10, hasMore: True }, { page: 2, posts: 1, hasMore: False }, { page: 99, posts: 0, hasMore: False }, { page: 1, posts: 10, hasMore: True }])
}
EOF
alder fill --decl 'Posts.Index."a `_` in the query is literal"' - <<'EOF'
{
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "snake", title: Text.NonEmpty "snake_case", body: "", author: author, published: True }
  let _ <- Db.insert Post { slug: Post.Slug "camel", title: Text.NonEmpty "snakeYcase", body: "", author: author, published: True }
  let model <- load { page: 1, q: "e_c" } { user: Nothing }
  Task.succeed (List.map (\p -> p.title) model.posts == [Text.NonEmpty "snake_case"])
}
EOF
alder fill --decl 'Posts.Index."the second page links back to the first"' - <<'EOF'
{
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Task.all (List.map (\n -> Db.insert Post { slug: Post.Slug ("post-" ++ Text.fromInt n), title: Text.NonEmpty "A post", body: "", author: author, published: True }) (List.range 1 11))
  let first <- Test.get (Posts.Index {})
  let second <- Test.get (Posts.Index { page: 2 })
  Task.succeed ({ firstPosts: Test.count "ul.posts li" first.doc, next: Test.attr "href" "a[rel=next]" first.doc, secondPosts: Test.count "ul.posts li" second.doc, prev: Test.attr "href" "a[rel=prev]" second.doc, more: Test.has "a[rel=next]" second.doc } == { firstPosts: 10, next: Just "/?page=2", secondPosts: 1, prev: Just "/", more: False })
}
EOF
alder fill --decl 'Posts.Index."a search through the form"' - <<'EOF'
{
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "soup", title: Text.NonEmpty "Soup", body: "Use ripe tomatoes.", author: author, published: True }
  let _ <- Db.insert Post { slug: Post.Slug "trains", title: Text.NonEmpty "Trains", body: "Fast ones.", author: author, published: True }
  let _ <- Test.get (Posts.Index {})
  let r <- Test.submit SearchForm { q: "tomato" }
  Task.succeed ({ answer: r.answer, titles: Test.texts "ul.posts a" r.doc, query: Test.attr "value" "input[name=q]" r.doc } == { answer: Redirected (Posts.Index { page: 1, q: "tomato" }), titles: ["Soup"], query: Just "tomato" })
}
EOF

# existing post-page tests: their posts are published
alder fill --decl 'Posts.Show."a post'"'"'s body is rendered as Markdown"' - <<'EOF'
{
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "Not *yet*.", author: author, published: True }
  let m <- load { slug: Post.Slug "hello" } { user: Nothing }
  Task.succeed ({ title: m.post.title, body: m.body } == { title: Text.NonEmpty "Hello", body: <p>Not <em>yet</em>.</p> })
}
EOF
```

The line above with `'"'"'` is shell quoting that the harness does not support, so I use the double-quoted form below instead for the names holding an apostrophe.

```alder-cli
# the field, and its migration
alder edit table Post add-field published:Bool=False
alder migrate

# the front page lists only published posts
alder fill --decl Posts.Index.load - <<'EOF'
{
  let page = if params.page < 1 then 1 else if params.page > lastPage then lastPage else params.page
  let q = Text.slice 0 100 (Text.trim params.q)
  let matching =
    if Text.isEmpty q then Db.and [] else Db.or [Post.title.contains q, Post.body.contains q]
  let found <-
    Db.from Post
      |> Db.where (Post.published.eq True)
      |> Db.where matching
      |> Db.orderBy Post.created.desc
      |> Db.limit (perPage + 1)
      |> Db.offset ((page - 1) * perPage)
      |> Db.all
  let me <- match session.user {
    Just id -> User.author id,
    Nothing -> Query.succeed Nothing,
  }
  let canWrite = me != Nothing
  Query.succeed { page: page, q: q, posts: List.take perPage found, hasMore: List.length found > perPage, canWrite: canWrite }
}
EOF

# a draft's page is a 404 for anyone but its author
alder fill --decl Posts.Show.load - <<'EOF'
{
  let found <- Post.bySlug params.slug
  match found {
    Just post -> if post.published || session.user == Just post.author then {
      let body <- Markdown.render post.body
      let comments <-
        Db.from Comment
          |> Db.where (Comment.post.eq post.id)
          |> Db.orderBy Comment.created.asc
          |> Db.all
      Query.succeed { post: post, body: body, comments: comments }
    } else Query.notFound,
    Nothing -> Query.notFound,
  }
}
EOF

# publishing from the dashboard
alder new msg Admin Publish id:Post.PostId
alder fill Admin.update.Publish - <<'EOF'
{
  let found <- Db.from Post |> Db.where (Post.id.eq p.id) |> Db.where (Post.author.eq model.author.id) |> Db.first
  match found {
    Just post -> {
      let _ <- Db.update Post post.id (\x -> { x | published: True })
      Task.succeed (Redirect (Admin {}))
    },
    Nothing -> Task.notFound,
  }
}
EOF
alder fill --decl Admin.view - <<'EOF'
<Layout.Page title="Your posts">
  <h1>Your posts</h1>

  <h2>Drafts</h2>
  <ul id="drafts">
    {for post in List.filter (\x -> not x.published) model.posts ->
      <li>
        <a class="title" href={Admin.Edit { id: post.id }}>{post.title}</a>
        {" "}
        <button click={Publish { id: post.id }}>Publish</button>
      </li>}
  </ul>

  <h2>All posts</h2>
  <ul class="posts">
    {for post in model.posts ->
      <li>
        <a href={Admin.Edit { id: post.id }}>{post.title}</a>
      </li>}
  </ul>

  <h2>New post</h2>
  <form submit={Create}>
    <Errors form />
    <Input field="slug" label="Slug (lowercase letters, digits and hyphens)" />
    <Errors field="slug" />
    <Input field="title" label="Title" />
    <Errors field="title" />
    <Textarea field="body" label="Body (Markdown)" />
    <Errors field="body" />
    <button>Create post</button>
  </form>

  <button click={App.Logout}>Log out</button>
</Layout.Page>
EOF

# existing front-page tests: their posts are published
alder fill --decl 'Posts.Index."the front page lists the posts, newest first"' - <<'EOF'
{
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "first", title: Text.NonEmpty "First", body: "", author: author, published: True }
  let _ <- Db.insert Post { slug: Post.Slug "second", title: Text.NonEmpty "Second", body: "", author: author, published: True }
  let model <- load { page: 1, q: "" } { user: Nothing }
  Task.succeed (List.map (\p -> p.title) model.posts == [Text.NonEmpty "Second", Text.NonEmpty "First"])
}
EOF
alder fill --decl 'Posts.Index."search matches title or body"' - <<'EOF'
{
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "tomatoes", title: Text.NonEmpty "Tomato season", body: "It is here.", author: author, published: True }
  let _ <- Db.insert Post { slug: Post.Slug "soup", title: Text.NonEmpty "Soup", body: "Use ripe tomatoes.", author: author, published: True }
  let _ <- Db.insert Post { slug: Post.Slug "trains", title: Text.NonEmpty "Trains", body: "Fast ones.", author: author, published: True }
  let model <- load { page: 1, q: "TOMATO" } { user: Nothing }
  let titles = List.map (\p -> p.title) model.posts
  Task.succeed ({ count: List.length titles, title: List.member (Text.NonEmpty "Tomato season") titles, body: List.member (Text.NonEmpty "Soup") titles } == { count: 2, title: True, body: True })
}
EOF
alder fill --decl 'Posts.Index."a `%` in the query is literal"' - <<'EOF'
{
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "done", title: Text.NonEmpty "100% done", body: "", author: author, published: True }
  let _ <- Db.insert Post { slug: Post.Slug "ways", title: Text.NonEmpty "1000 ways", body: "", author: author, published: True }
  let model <- load { page: 1, q: "100%" } { user: Nothing }
  Task.succeed (List.map (\p -> p.title) model.posts == [Text.NonEmpty "100% done"])
}
EOF
alder fill --decl 'Posts.Index."page 99 is empty"' - <<'EOF'
{
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Task.all (List.map (\n -> Db.insert Post { slug: Post.Slug ("post-" ++ Text.fromInt n), title: Text.NonEmpty "A post", body: "", author: author, published: True }) (List.range 1 11))
  let pages <- Query.all (List.map (\n -> load { page: n, q: "" } { user: Nothing }) [1, 2, 99, 0])
  let seen = List.map (\m -> { page: m.page, posts: List.length m.posts, hasMore: m.hasMore }) pages
  Task.succeed (seen == [{ page: 1, posts: 10, hasMore: True }, { page: 2, posts: 1, hasMore: False }, { page: 99, posts: 0, hasMore: False }, { page: 1, posts: 10, hasMore: True }])
}
EOF
alder fill --decl 'Posts.Index."a `_` in the query is literal"' - <<'EOF'
{
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "snake", title: Text.NonEmpty "snake_case", body: "", author: author, published: True }
  let _ <- Db.insert Post { slug: Post.Slug "camel", title: Text.NonEmpty "snakeYcase", body: "", author: author, published: True }
  let model <- load { page: 1, q: "e_c" } { user: Nothing }
  Task.succeed (List.map (\p -> p.title) model.posts == [Text.NonEmpty "snake_case"])
}
EOF
alder fill --decl 'Posts.Index."the second page links back to the first"' - <<'EOF'
{
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Task.all (List.map (\n -> Db.insert Post { slug: Post.Slug ("post-" ++ Text.fromInt n), title: Text.NonEmpty "A post", body: "", author: author, published: True }) (List.range 1 11))
  let first <- Test.get (Posts.Index {})
  let second <- Test.get (Posts.Index { page: 2 })
  Task.succeed ({ firstPosts: Test.count "ul.posts li" first.doc, next: Test.attr "href" "a[rel=next]" first.doc, secondPosts: Test.count "ul.posts li" second.doc, prev: Test.attr "href" "a[rel=prev]" second.doc, more: Test.has "a[rel=next]" second.doc } == { firstPosts: 10, next: Just "/?page=2", secondPosts: 1, prev: Just "/", more: False })
}
EOF
alder fill --decl 'Posts.Index."a search through the form"' - <<'EOF'
{
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "soup", title: Text.NonEmpty "Soup", body: "Use ripe tomatoes.", author: author, published: True }
  let _ <- Db.insert Post { slug: Post.Slug "trains", title: Text.NonEmpty "Trains", body: "Fast ones.", author: author, published: True }
  let _ <- Test.get (Posts.Index {})
  let r <- Test.submit SearchForm { q: "tomato" }
  Task.succeed ({ answer: r.answer, titles: Test.texts "ul.posts a" r.doc, query: Test.attr "value" "input[name=q]" r.doc } == { answer: Redirected (Posts.Index { page: 1, q: "tomato" }), titles: ["Soup"], query: Just "tomato" })
}
EOF

# existing post-page tests: their posts are published
alder fill --decl "Posts.Show.\"a post's body is rendered as Markdown\"" - <<'EOF'
{
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "Not *yet*.", author: author, published: True }
  let m <- load { slug: Post.Slug "hello" } { user: Nothing }
  Task.succeed ({ title: m.post.title, body: m.body } == { title: Text.NonEmpty "Hello", body: <p>Not <em>yet</em>.</p> })
}
EOF
alder fill --decl "Posts.Show.\"a post's body is rendered through the Markdown capability\"" - <<'EOF'
{
  let _ <- Test.fake Markdown
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "Not *yet*.", author: author, published: True }
  let m <- load { slug: Post.Slug "hello" } { user: Nothing }
  let steps <- Test.trace
  Task.succeed ({ body: m.body, rendered: List.member "Markdown.render" steps } == { body: Html.text "Not *yet*.", rendered: True })
}
EOF
alder fill --decl 'Posts.Show."a comment is listed oldest first"' - <<'EOF'
{
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "", author: author, published: True }
  let before <- load { slug: Post.Slug "hello" } { user: Nothing }
  let _ <- update (AddComment { form: { author: Comment.Name "Ann", body: Comment.Body "First!" } }) before { user: Nothing }
  let _ <- update (AddComment { form: { author: Comment.Name "Bob", body: Comment.Body "Second." } }) before { user: Nothing }
  let after <- load { slug: Post.Slug "hello" } { user: Nothing }
  Task.succeed (List.map (\c -> c.body) after.comments == [Comment.Body "First!", Comment.Body "Second."])
}
EOF
alder fill --decl 'Posts.Show."an empty comment is rejected"' - <<'EOF'
{
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "", author: author, published: True }
  let _ <- Test.get (Posts.Show { slug: Post.Slug "hello" })
  let r <- Test.submitRaw NewComment [{ name: "author", value: "Ann" }, { name: "body", value: "" }]
  let comments <- Db.from Comment |> Db.count
  Task.succeed ({ answer: r.answer, author: Test.attr "value" "input[name=author]" r.doc, comments: comments } == { answer: FormRejected [{ field: "body", message: "is not valid" }], author: Just "Ann", comments: 0 })
}
EOF

# new tests
alder new test Posts.Index "a draft is not listed and not found by a search"
alder fill 'Posts.Index."a draft is not listed and not found by a search"' - <<'EOF'
{
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "soup", title: Text.NonEmpty "Soup", body: "Use ripe tomatoes.", author: author, published: True }
  let _ <- Db.insert Post { slug: Post.Slug "draft", title: Text.NonEmpty "Tomato draft", body: "Tomatoes.", author: author }
  let front <- load { page: 1, q: "" } { user: Nothing }
  let search <- load { page: 1, q: "tomato" } { user: Nothing }
  let page <- Test.get (Posts.Index {})
  Task.succeed ({ front: List.map (\p -> p.title) front.posts, search: List.map (\p -> p.title) search.posts, listed: Test.texts "ul.posts a" page.doc } == { front: [Text.NonEmpty "Soup"], search: [Text.NonEmpty "Soup"], listed: ["Soup"] })
}
EOF
alder new test Posts.Show "a draft is not found except by its author"
alder fill 'Posts.Show."a draft is not found except by its author"' - <<'EOF'
{
  let _ <- Test.fake Markdown
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let reader <- Db.insert User { email: Text.Email "reader@example.com", name: Text.NonEmpty "Reader", passwordHash: "" }
  let _ <- Db.insert Post { slug: Post.Slug "draft", title: Text.NonEmpty "Hello", body: "Soon.", author: author }
  let visitor <- Test.open (Posts.Show { slug: Post.Slug "draft" })
  let _ <- Test.login { user: Just reader }
  let asReader <- Test.open (Posts.Show { slug: Post.Slug "draft" })
  let _ <- Test.login { user: Just author }
  let asAuthor <- Test.open (Posts.Show { slug: Post.Slug "draft" })
  Task.succeed ({ visitor: visitor.answer, reader: asReader.answer, author: asAuthor.answer, title: Test.text "h1" asAuthor.doc } == { visitor: PageMissing, reader: PageMissing, author: Shown, title: Just "Hello" })
}
EOF
alder new test Admin "publishing a draft from the dashboard puts it on the front page"
alder fill 'Admin."publishing a draft from the dashboard puts it on the front page"' - <<'EOF'
{
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let id <- Db.insert Post { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "", author: author }
  let _ <- Test.login { user: Just author }
  let before <- Test.get (Posts.Index {})
  let dash <- Test.get (Admin {})
  let r <- Test.click (Publish { id: id })
  let after <- Test.get (Posts.Index {})
  Task.succeed ({ before: Test.texts "ul.posts a" before.doc, drafts: Test.texts "#drafts .title" dash.doc, answer: r.answer, left: Test.count "#drafts .title" r.doc, after: Test.texts "ul.posts a" after.doc } == { before: [], drafts: ["Hello"], answer: Redirected (Admin {}), left: 0, after: ["Hello"] })
}
EOF
alder new test Admin "another author cannot publish my draft"
alder fill 'Admin."another author cannot publish my draft"' - <<'EOF'
{
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "", role: Author }
  let id <- Db.insert Post { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "", author: author }
  let _ <- Test.login { user: Just other }
  let model <- load {} { user: other }
  let r <- Test.run (update (Publish { id: id }) model { user: other })
  let after <- Db.get Post id
  Task.succeed ({ value: r.value, published: Maybe.map (\p -> p.published) after } == { value: Err "not found", published: Just False })
}
EOF

alder fill --list
alder check
alder test
```
