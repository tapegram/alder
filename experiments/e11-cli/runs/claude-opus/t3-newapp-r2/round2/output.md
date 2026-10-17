```alder-cli
alder fill --decl 'Posts.Show."NewComment refuses an empty author"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Db.insert Post { title: Text.NonEmpty "Hi", slug: Post.Slug "slug-1", body: "Hi", published: True, author: me }
  let _ <- Test.get (Posts.Show { slug: Post.Slug "slug-1" })
  let r <- Test.submitRaw NewComment [{ name: "author", value: "" }, { name: "body", value: "x" }]
  Task.succeed (r.answer == FormRejected [{ field: "author", message: "must not be empty" }])
}
EOF
alder check
alder test
```
