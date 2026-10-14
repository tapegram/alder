```alder-cli
alder fill 'Posts."submitting NewComment"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Db.insert Post { title: Text.NonEmpty "First", slug: Post.Slug "first", body: "one", published: True, author: me }
  let _ <- Test.get (Posts { slug: Post.Slug "first" })
  let r <- Test.submit NewComment { author: Text.NonEmpty "Ann", body: Text.NonEmpty "Nice post" }
  let n <- Db.from Comment |> Db.count
  Task.succeed ({ answer: r.answer, comments: n } == { answer: Rerendered, comments: 1 })
}
EOF
alder fill 'Admin."submitting NewPost"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Admin {})
  let r <- Test.submit NewPost { title: Text.NonEmpty "title-1", slug: Post.Slug "slug-1", body: "body-1" }
  let n <- Db.from Post |> Db.count
  Task.succeed ({ answer: r.answer, posts: n } == { answer: Rerendered, posts: 1 })
}
EOF
alder fill 'Admin."pressing Toggle"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
  let _ <- Test.login { user: Just me }
  let post1 <- Db.insert Post { title: Text.NonEmpty "title-1", slug: Post.Slug "slug-1", body: "body-1", author: me }
  let _ <- Test.get (Admin {})
  let r <- Test.click (Toggle { id: post1 })
  let after <- Db.get Post post1
  Task.succeed ({ answer: r.answer, published: Maybe.map (\t -> t.published) after } == { answer: Rerendered, published: Just True })
}
EOF
```
