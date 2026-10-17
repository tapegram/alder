### Round 2

```text
$ alder fill Admin.Index.update.Toggle - <<'EOF'
{
  let _ <- Db.update Post p.id (\x -> { x | published: not x.published })
  Task.succeed Rerender
}
EOF
edited app/Admin/Index.alder
--- a/app/Admin/Index.alder
+++ b/app/Admin/Index.alder
@@ -31,7 +31,10 @@
     let _ <- Db.insert Post { title: p.form.title, slug: p.form.slug, body: p.form.body, published: p.form.published }
     Task.succeed (Redirect (Admin.Index {}))
   },
-  Toggle p -> ?toggle,
+  Toggle p -> {
+    let _ <- Db.update Post p.id (\x -> { x | published: not x.published })
+    Task.succeed Rerender
+  },
 }
 
 view : Model -> Session -> Html Msg
app/Admin/Index.alder:91:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill --decl 'Admin.Index."pressing Toggle"' - <<'EOF'
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
edited app/Admin/Index.alder
--- a/app/Admin/Index.alder
+++ b/app/Admin/Index.alder
@@ -78,9 +78,6 @@
   Task.succeed ({ answer: r.answer, rows: rows } == { answer: FormRejected [{ field: "title", message: "must not be empty" }], rows: 0 })
 }
 
--- Fill ?expected with what pressing Toggle must do, so that a test fails when the arm
--- does something else: its answer (`r.answer`: Rerendered, Redirected …) and what it
--- changed (`after`, the row read again), as `{ answer: r.answer, … } == { answer: Rerendered, … }`.
 test "pressing Toggle" = {
   let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: Text.Secret "passwordhash-1" }
   let _ <- Test.login { user: Just me }
@@ -88,5 +85,5 @@
   let _ <- Test.get (Admin.Index {})
   let r <- Test.click (Toggle { id: post1 })
   let after <- Db.get Post post1
-  Task.succeed ?expected
+  Task.succeed ({ answer: r.answer, published: Maybe.map (\t -> t.published) after } == { answer: Rerendered, published: Just True })
 }
rewriting a test: `alder test --mutate` is the check that it still tests something
[exit 0]
$ alder fill --decl 'Posts.Show."submitting NewComment"' - <<'EOF'
{
  let _ <- Db.insert Post { title: Text.NonEmpty "Hello", slug: Post.Slug "slug-1", body: "x", published: True }
  let _ <- Test.get (Posts.Show { slug: Post.Slug "slug-1" })
  let r <- Test.submit NewComment { author: Text.NonEmpty "author-1", body: Text.NonEmpty "body-1" }
  let n <- Db.from Comment |> Db.count
  Task.succeed ({ answer: r.answer, n: n } == { answer: Rerendered, n: 1 })
}
EOF
edited app/Posts/Show.alder
--- a/app/Posts/Show.alder
+++ b/app/Posts/Show.alder
@@ -59,9 +59,11 @@
   </Layout.Page>
 
 test "submitting NewComment" = {
+  let _ <- Db.insert Post { title: Text.NonEmpty "Hello", slug: Post.Slug "slug-1", body: "x", published: True }
   let _ <- Test.get (Posts.Show { slug: Post.Slug "slug-1" })
   let r <- Test.submit NewComment { author: Text.NonEmpty "author-1", body: Text.NonEmpty "body-1" }
-  Task.succeed (r.answer == ?expected)
+  let n <- Db.from Comment |> Db.count
+  Task.succeed ({ answer: r.answer, n: n } == { answer: Rerendered, n: 1 })
 }
 
 test "NewComment refuses an empty author" = {
rewriting a test: `alder test --mutate` is the check that it still tests something
[exit 0]
$ alder check
[exit 0]
$ alder test
ok   Admin.Index "a visitor is sent to log in"
ok   Admin.Index "submitting NewPost adds a row"
FAIL Admin.Index "NewPost refuses an empty title"
  left:  { answer: FormRejected [{ field: "", message: "title must not be empty" }], rows: 0 }
  right: { answer: FormRejected [{ field: "title", message: "must not be empty" }], rows: 0 }
  trace: ["Db.insert User", "Test.login", "Test.get Admin.Index", "Db.all Post", "Test.submitRaw NewPost", "Db.all Post", "Db.count Post"]
ok   Admin.Index "pressing Toggle"
ok   App "Logout clears the user and goes to log in"
ok   Auth.Login "an unknown email is rejected"
ok   Auth.Login "logging in through the form"
ok   Auth.Login "a signed-in user logs out here"
ok   Comment "deleting its post deletes a Comment"
ok   Posts.Index "the front page lists only published posts, newest first"
ok   Posts.Show "submitting NewComment"
FAIL Posts.Show "NewComment refuses an empty author"
  error: GET /posts/slug-1: not found
  trace: ["Test.get Posts.Show", "Db.first Post"]
ok   Posts.Show "an unpublished post is not found for a reader"
ok   Posts.Show "a comment with an empty author is refused"
12 passed, 2 failed
[exit 1]
```

The harness stopped at line 28 of your script: `alder test`: it exited 1. The commands after it were not run.

Your script stopped at command 5; the app is as the commands before it left it; continue.

The program builds but its tests fail; fix the program or the tests so every test passes.

```text
ok   Admin.Index "a visitor is sent to log in"
ok   Admin.Index "submitting NewPost adds a row"
FAIL Admin.Index "NewPost refuses an empty title"
  left:  { answer: FormRejected [{ field: "", message: "title must not be empty" }], rows: 0 }
  right: { answer: FormRejected [{ field: "title", message: "must not be empty" }], rows: 0 }
  trace: ["Db.insert User", "Test.login", "Test.get Admin.Index", "Db.all Post", "Test.submitRaw NewPost", "Db.all Post", "Db.count Post"]
ok   Admin.Index "pressing Toggle"
ok   App "Logout clears the user and goes to log in"
ok   Auth.Login "an unknown email is rejected"
ok   Auth.Login "logging in through the form"
ok   Auth.Login "a signed-in user logs out here"
ok   Comment "deleting its post deletes a Comment"
ok   Posts.Index "the front page lists only published posts, newest first"
ok   Posts.Show "submitting NewComment"
FAIL Posts.Show "NewComment refuses an empty author"
  error: GET /posts/slug-1: not found
  trace: ["Test.get Posts.Show", "Db.first Post"]
ok   Posts.Show "an unpublished post is not found for a reader"
ok   Posts.Show "a comment with an empty author is refused"
12 passed, 2 failed
```
