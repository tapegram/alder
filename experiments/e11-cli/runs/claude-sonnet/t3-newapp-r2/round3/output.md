```alder-cli
alder fill --decl 'Admin.Index."NewPost refuses an empty title"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: Text.Secret "passwordhash-1" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Admin.Index {})
  let r <- Test.submitRaw NewPost [{ name: "title", value: "" }, { name: "slug", value: "x" }, { name: "body", value: "x" }, { name: "published", value: "True" }]
  let rows <- Db.from Post |> Db.count
  Task.succeed ({ answer: r.answer, rows: rows } == { answer: FormRejected [{ field: "", message: "title must not be empty" }], rows: 0 })
}
EOF
alder fill --decl 'Posts.Show."NewComment refuses an empty author"' - <<'EOF'
{
  let _ <- Db.insert Post { title: Text.NonEmpty "Hello", slug: Post.Slug "slug-1", body: "x", published: True }
  let _ <- Test.get (Posts.Show { slug: Post.Slug "slug-1" })
  let r <- Test.submitRaw NewComment [{ name: "author", value: "" }, { name: "body", value: "x" }]
  Task.succeed (r.answer == FormRejected [{ field: "author", message: "must not be empty" }])
}
EOF
alder test
```
