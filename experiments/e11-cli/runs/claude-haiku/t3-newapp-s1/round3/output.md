```alder-cli
# Posts: the comment form, through a published post (the old test used a slug that was never inserted)
alder fill --decl 'Posts."submitting NewComment"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Db.insert Post { title: Text.NonEmpty "First", slug: Post.Slug "first", body: "one", published: True, author: me }
  let _ <- Test.get (Posts { slug: Post.Slug "first" })
  let r <- Test.submit NewComment { author: Text.NonEmpty "Ann", body: Text.NonEmpty "Nice post" }
  let n <- Db.from Comment |> Db.count
  Task.succeed ({ answer: r.answer, comments: n } == { answer: Rerendered, comments: 1 })
}
EOF

# Posts: an empty author is refused, and no comment is stored
alder fill --decl 'Posts."NewComment refuses an empty author"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Db.insert Post { title: Text.NonEmpty "First", slug: Post.Slug "first", body: "one", published: True, author: me }
  let _ <- Test.get (Posts { slug: Post.Slug "first" })
  let r <- Test.submitRaw NewComment [{ name: "author", value: "" }, { name: "body", value: "x" }]
  let n <- Db.from Comment |> Db.count
  let rejected = match r.answer {
    FormRejected _ -> True,
    _ -> False,
  }
  Task.succeed ({ rejected: rejected, comments: n } == { rejected: True, comments: 0 })
}
EOF

# Admin: creating a post
alder fill --decl 'Admin."submitting NewPost"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Admin {})
  let r <- Test.submit NewPost { title: Text.NonEmpty "title-1", slug: Post.Slug "slug-1", body: "body-1" }
  let n <- Db.from Post |> Db.count
  Task.succeed ({ answer: r.answer, posts: n } == { answer: Rerendered, posts: 1 })
}
EOF

# Admin: an empty title is refused, and no post is stored
alder fill --decl 'Admin."NewPost refuses an empty title"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Admin {})
  let r <- Test.submitRaw NewPost [{ name: "title", value: "" }, { name: "slug", value: "x" }, { name: "body", value: "x" }]
  let n <- Db.from Post |> Db.count
  let rejected = match r.answer {
    FormRejected _ -> True,
    _ -> False,
  }
  Task.succeed ({ rejected: rejected, posts: n } == { rejected: True, posts: 0 })
}
EOF

# Admin: pressing Toggle publishes an unpublished post and re-renders the page
alder fill --decl 'Admin."pressing Toggle"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
  let _ <- Test.login { user: Just me }
  let post1 <- Db.insert Post { title: Text.NonEmpty "title-1", slug: Post.Slug "slug-1", body: "body-1", author: me }
  let _ <- Test.get (Admin {})
  let r <- Test.click (Toggle { id: post1 })
  let after <- Db.get Post post1
  Task.succeed ({ answer: r.answer, published: Maybe.map (\p -> p.published) after } == { answer: Rerendered, published: Just True })
}
EOF
```
