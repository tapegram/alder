```alder-cli
alder fill 'Admin."submitting NewPost"' - <<'EOF'
Redirected (Admin {})
EOF
alder fill 'Admin."pressing TogglePublished"' - <<'EOF'
{ answer: r.answer, published: Maybe.map (\x -> x.published) after } == { answer: Rerendered, published: Just True }
EOF
alder fill --decl 'Posts.Show."submitting NewComment"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Db.insert Post { title: Text.NonEmpty "Hello", slug: Post.Slug "hello", body: "Hi", published: True, author: me }
  let _ <- Test.get (Posts.Show { slug: Post.Slug "hello" })
  let r <- Test.submit NewComment { author: Text.NonEmpty "Ann", body: Text.NonEmpty "Nice" }
  let page <- Test.get (Posts.Show { slug: Post.Slug "hello" })
  Task.succeed ({ answer: r.answer, comments: List.map (\c -> c.author) page.model.comments } == { answer: Redirected (Posts.Show { slug: Post.Slug "hello" }), comments: [Text.NonEmpty "Ann"] })
}
EOF
alder check
alder test
```
