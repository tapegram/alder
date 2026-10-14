### Round 4

```text
$ alder fill --decl 'Posts.Show."NewComment refuses an empty author"' - <<'EOF'
{
  let _ <- Db.insert Post { title: Text.NonEmpty "Hello", slug: Slug.Slug "slug-1", body: "b", published: True }
  let _ <- Test.get (Posts.Show { slug: Slug.Slug "slug-1" })
  let r <- Test.submitRaw NewComment [{ name: "author", value: "" }, { name: "body", value: "x" }]
  Task.succeed (r.answer == FormRejected [{ field: "author", message: "must not be empty" }])
}
EOF
edited app/Posts/Show.alder
--- a/app/Posts/Show.alder
+++ b/app/Posts/Show.alder
@@ -63,6 +63,7 @@
 }
 
 test "NewComment refuses an empty author" = {
+  let _ <- Db.insert Post { title: Text.NonEmpty "Hello", slug: Slug.Slug "slug-1", body: "b", published: True }
   let _ <- Test.get (Posts.Show { slug: Slug.Slug "slug-1" })
   let r <- Test.submitRaw NewComment [{ name: "author", value: "" }, { name: "body", value: "x" }]
   Task.succeed (r.answer == FormRejected [{ field: "author", message: "must not be empty" }])
rewriting a test: `alder test --mutate` is the check that it still tests something
[exit 0]
$ alder test
ok   App "Logout clears the user and goes to log in"
ok   Auth.Login "an unknown email is rejected"
ok   Auth.Login "logging in through the form"
ok   Auth.Login "a signed-in user logs out here"
ok   Comment "deleting its post deletes a Comment"
ok   Desk "a visitor is sent to log in"
ok   Desk "submitting NewPost adds a row"
ok   Desk "NewPost refuses an empty title"
ok   Desk "pressing Toggle"
ok   Home "lists only published posts, newest first"
ok   Posts.Show "submitting NewComment"
ok   Posts.Show "NewComment refuses an empty author"
ok   Posts.Show "an unpublished post is not found for a reader"
ok   Posts.Show "a comment with an empty author is refused"
14 passed, 0 failed
[exit 0]
```

The harness ran `alder check`, `alder build` and `alder test` on the app: all pass.
