```alder-cli
# the Posts.Show tests insert posts that must be published to be shown to a reader
alder fill --decl Posts.Show."a post's body is rendered as Markdown" - <<'EOF'
{
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "Not *yet*.", author: author, published: True }
  let m <- load { slug: Post.Slug "hello" } { user: Nothing }
  Task.succeed ({ title: m.post.title, body: m.body } == { title: Text.NonEmpty "Hello", body: <p>Not <em>yet</em>.</p> })
}
EOF
alder fill --decl Posts.Show."a post's body is rendered through the Markdown capability" - <<'EOF'
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
# a draft's page: a 404 for a reader, shown to its author
alder new test Posts.Show "a draft is a 404 for a reader and shown to its author"
alder fill --decl 'Posts.Show."a draft is a 404 for a reader and shown to its author"' - <<'EOF'
{
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let reader <- Db.insert User { email: Text.Email "reader@example.com", name: Text.NonEmpty "Reader", passwordHash: "" }
  let _ <- Db.insert Post { slug: Post.Slug "draft", title: Text.NonEmpty "Draft", body: "", author: author }
  let _ <- Test.login { user: Just reader }
  let asReader <- Test.open (Posts.Show { slug: Post.Slug "draft" })
  let _ <- Test.login { user: Just author }
  let asAuthor <- Test.open (Posts.Show { slug: Post.Slug "draft" })
  Task.succeed ({ reader: asReader.answer, author: asAuthor.answer } == { reader: PageMissing, author: Shown })
}
EOF
alder test
```
