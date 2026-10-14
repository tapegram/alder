```alder-cli
alder fill --decl 'Admin."pressing Publish"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1", role: Author }
  let other <- Db.insert User { email: Text.Email "email-2@example.com", name: Text.NonEmpty "name-2", passwordHash: "passwordhash-2", role: Author }
  let post1 <- Db.insert Post { slug: Post.Slug "slug-1", title: Text.NonEmpty "title-1", body: "body-1", author: me }
  let post2 <- Db.insert Post { slug: Post.Slug "slug-2", title: Text.NonEmpty "title-2", body: "body-2", author: other }
  let _ <- Test.login { user: Just other }
  let _ <- Test.get (Admin {})
  let _ <- Test.login { user: Just me }
  let stranger <- Test.click (Publish { id: post2 })
  let theirs <- Db.get Post post2
  let _ <- Test.get (Admin {})
  let r <- Test.click (Publish { id: post1 })
  let after <- Db.get Post post1
  Task.succeed ({ answer: r.answer, published: Maybe.map (\p -> p.published) after, stranger: stranger.answer, theirs: Maybe.map (\p -> p.published) theirs } == { answer: Redirected (Admin {}), published: Just True, stranger: PageMissing, theirs: Just False })
}
EOF
alder test
```
