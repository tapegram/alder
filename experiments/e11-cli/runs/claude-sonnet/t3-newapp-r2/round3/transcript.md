### Round 3

```text
$ alder fill --decl 'Admin.Index."NewPost refuses an empty title"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: Text.Secret "passwordhash-1" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Admin.Index {})
  let r <- Test.submitRaw NewPost [{ name: "title", value: "" }, { name: "slug", value: "x" }, { name: "body", value: "x" }, { name: "published", value: "True" }]
  let rows <- Db.from Post |> Db.count
  Task.succeed ({ answer: r.answer, rows: rows } == { answer: FormRejected [{ field: "", message: "title must not be empty" }], rows: 0 })
}
EOF
edited app/Admin/Index.alder
--- a/app/Admin/Index.alder
+++ b/app/Admin/Index.alder
@@ -75,7 +75,7 @@
   let _ <- Test.get (Admin.Index {})
   let r <- Test.submitRaw NewPost [{ name: "title", value: "" }, { name: "slug", value: "x" }, { name: "body", value: "x" }, { name: "published", value: "True" }]
   let rows <- Db.from Post |> Db.count
-  Task.succeed ({ answer: r.answer, rows: rows } == { answer: FormRejected [{ field: "title", message: "must not be empty" }], rows: 0 })
+  Task.succeed ({ answer: r.answer, rows: rows } == { answer: FormRejected [{ field: "", message: "title must not be empty" }], rows: 0 })
 }
 
 test "pressing Toggle" = {
rewriting a test: `alder test --mutate` is the check that it still tests something
[exit 0]
$ alder fill --decl 'Posts.Show."NewComment refuses an empty author"' - <<'EOF'
{
  let _ <- Db.insert Post { title: Text.NonEmpty "Hello", slug: Post.Slug "slug-1", body: "x", published: True }
  let _ <- Test.get (Posts.Show { slug: Post.Slug "slug-1" })
  let r <- Test.submitRaw NewComment [{ name: "author", value: "" }, { name: "body", value: "x" }]
  Task.succeed (r.answer == FormRejected [{ field: "author", message: "must not be empty" }])
}
EOF
edited app/Posts/Show.alder
--- a/app/Posts/Show.alder
+++ b/app/Posts/Show.alder
@@ -67,6 +67,7 @@
 }
 
 test "NewComment refuses an empty author" = {
+  let _ <- Db.insert Post { title: Text.NonEmpty "Hello", slug: Post.Slug "slug-1", body: "x", published: True }
   let _ <- Test.get (Posts.Show { slug: Post.Slug "slug-1" })
   let r <- Test.submitRaw NewComment [{ name: "author", value: "" }, { name: "body", value: "x" }]
   Task.succeed (r.answer == FormRejected [{ field: "author", message: "must not be empty" }])
rewriting a test: `alder test --mutate` is the check that it still tests something
[exit 0]
$ alder test
ok   Admin.Index "a visitor is sent to log in"
ok   Admin.Index "submitting NewPost adds a row"
ok   Admin.Index "NewPost refuses an empty title"
ok   Admin.Index "pressing Toggle"
ok   App "Logout clears the user and goes to log in"
ok   Auth.Login "an unknown email is rejected"
ok   Auth.Login "logging in through the form"
ok   Auth.Login "a signed-in user logs out here"
ok   Comment "deleting its post deletes a Comment"
ok   Posts.Index "the front page lists only published posts, newest first"
ok   Posts.Show "submitting NewComment"
ok   Posts.Show "NewComment refuses an empty author"
ok   Posts.Show "an unpublished post is not found for a reader"
ok   Posts.Show "a comment with an empty author is refused"
14 passed, 0 failed
[exit 0]
```

The harness ran `alder check`, `alder build` and `alder test` on the app: all pass.
