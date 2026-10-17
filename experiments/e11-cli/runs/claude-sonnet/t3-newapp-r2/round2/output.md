```alder-cli
alder fill Admin.Index.update.Toggle - <<'EOF'
{
  let _ <- Db.update Post p.id (\x -> { x | published: not x.published })
  Task.succeed Rerender
}
EOF
alder fill --decl 'Admin.Index."pressing Toggle"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: Text.Secret "passwordhash-1" }
  let _ <- Test.login { user: Just me }
  let post1 <- Db.insert Post { title: Text.NonEmpty "title-1", slug: Post.Slug "slug-1", body: "body-1" }
  let _ <- Test.get (Admin.Index {})
  let r <- Test.click (Toggle { id: post1 })
  let after <- Db.get Post post1
  Task.succeed ({ answer: r.answer, published: Maybe.map (\t -> t.published) after } == { answer: Rerendered, published: Just True })
}
EOF
alder fill --decl 'Posts.Show."submitting NewComment"' - <<'EOF'
{
  let _ <- Db.insert Post { title: Text.NonEmpty "Hello", slug: Post.Slug "slug-1", body: "x", published: True }
  let _ <- Test.get (Posts.Show { slug: Post.Slug "slug-1" })
  let r <- Test.submit NewComment { author: Text.NonEmpty "author-1", body: Text.NonEmpty "body-1" }
  let n <- Db.from Comment |> Db.count
  Task.succeed ({ answer: r.answer, n: n } == { answer: Rerendered, n: 1 })
}
EOF
alder check
alder test
```
