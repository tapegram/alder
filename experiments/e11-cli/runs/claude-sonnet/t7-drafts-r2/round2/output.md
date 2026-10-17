```alder-cli
alder fill --decl 'Admin."pressing Publish"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1", role: Author }
  let _ <- Test.login { user: Just me }
  let post1 <- Db.insert Post { slug: Post.Slug "slug-1", title: Text.NonEmpty "title-1", body: "body-1", author: me }
  let _ <- Test.get (Admin {})
  let r <- Test.click (Publish { id: post1 })
  let after <- Db.get Post post1
  Task.succeed ({ answer: r.answer, published: Maybe.map (\p -> p.published) after } == { answer: Redirected (Admin {}), published: Just True })
}
EOF
alder test
```
