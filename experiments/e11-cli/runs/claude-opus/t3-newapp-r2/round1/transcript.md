### Round 1

```text
$ alder fill 'Admin."submitting NewPost"' - <<'EOF'
Redirected (Admin {})
EOF
edited app/Admin.alder
--- a/app/Admin.alder
+++ b/app/Admin.alder
@@ -74,7 +74,7 @@
   let _ <- Test.login { user: Just me }
   let _ <- Test.get (Admin {})
   let r <- Test.submit NewPost { title: Text.NonEmpty "title-1", slug: Post.Slug "slug-1", body: "body-1" }
-  Task.succeed (r.answer == ?expected)
+  Task.succeed (r.answer == Redirected (Admin {}))
 }
 
 test "NewPost refuses an empty title" = {
app/Admin.alder:98:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill 'Admin."pressing TogglePublished"' - <<'EOF'
{ answer: r.answer, published: Maybe.map (\x -> x.published) after } == { answer: Rerendered, published: Just True }
EOF
edited app/Admin.alder
--- a/app/Admin.alder
+++ b/app/Admin.alder
@@ -85,9 +85,6 @@
   Task.succeed (r.answer == FormRejected [{ field: "title", message: "must not be empty" }])
 }
 
--- Fill ?expected with what pressing TogglePublished must do, so that a test fails when the arm
--- does something else: its answer (`r.answer`: Rerendered, Redirected …) and what it
--- changed (`after`, the row read again), as `{ answer: r.answer, … } == { answer: Rerendered, … }`.
 test "pressing TogglePublished" = {
   let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
   let _ <- Test.login { user: Just me }
@@ -95,7 +92,7 @@
   let _ <- Test.get (Admin {})
   let r <- Test.click (TogglePublished { id: post1 })
   let after <- Db.get Post post1
-  Task.succeed ?expected
+  Task.succeed ({ answer: r.answer, published: Maybe.map (\x -> x.published) after } == { answer: Rerendered, published: Just True })
 }
 
 test "toggling publishes a post" = {
[exit 0]
$ alder fill --decl 'Posts.Show."submitting NewComment"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Db.insert Post { title: Text.NonEmpty "Hello", slug: Post.Slug "hello", body: "Hi", published: True, author: me }
  let _ <- Test.get (Posts.Show { slug: Post.Slug "hello" })
  let r <- Test.submit NewComment { author: Text.NonEmpty "Ann", body: Text.NonEmpty "Nice" }
  let page <- Test.get (Posts.Show { slug: Post.Slug "hello" })
  Task.succeed ({ answer: r.answer, comments: List.map (\c -> c.author) page.model.comments } == { answer: Redirected (Posts.Show { slug: Post.Slug "hello" }), comments: [Text.NonEmpty "Ann"] })
}
EOF
edited app/Posts/Show.alder
--- a/app/Posts/Show.alder
+++ b/app/Posts/Show.alder
@@ -58,9 +58,12 @@
   </Layout.Page>
 
 test "submitting NewComment" = {
-  let _ <- Test.get (Posts.Show { slug: Post.Slug "slug-1" })
-  let r <- Test.submit NewComment { author: Text.NonEmpty "author-1", body: Text.NonEmpty "body-1" }
-  Task.succeed (r.answer == ?expected)
+  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
+  let _ <- Db.insert Post { title: Text.NonEmpty "Hello", slug: Post.Slug "hello", body: "Hi", published: True, author: me }
+  let _ <- Test.get (Posts.Show { slug: Post.Slug "hello" })
+  let r <- Test.submit NewComment { author: Text.NonEmpty "Ann", body: Text.NonEmpty "Nice" }
+  let page <- Test.get (Posts.Show { slug: Post.Slug "hello" })
+  Task.succeed ({ answer: r.answer, comments: List.map (\c -> c.author) page.model.comments } == { answer: Redirected (Posts.Show { slug: Post.Slug "hello" }), comments: [Text.NonEmpty "Ann"] })
 }
 
 test "NewComment refuses an empty author" = {
rewriting a test: `alder test --mutate` is the check that it still tests something
[exit 0]
$ alder check
[exit 0]
$ alder test
ok   Post "a Post keeps its author from being deleted"
ok   Admin "a visitor is sent to log in"
ok   Admin "submitting NewPost"
ok   Admin "NewPost refuses an empty title"
ok   Admin "pressing TogglePublished"
ok   Admin "toggling publishes a post"
ok   App "Logout clears the user and goes to log in"
ok   Auth.Login "an unknown email is rejected"
ok   Auth.Login "logging in through the form"
ok   Auth.Login "a signed-in user logs out here"
ok   Comment "deleting its post deletes a Comment"
ok   Home "/ lists only published posts, newest first"
ok   Posts.Show "submitting NewComment"
FAIL Posts.Show "NewComment refuses an empty author"
  error: GET /posts/slug-1: not found
  trace: ["Test.get Posts.Show", "Db.first Post"]
ok   Posts.Show "an unpublished post is not found for a reader"
ok   Posts.Show "a comment with an empty author is refused"
15 passed, 1 failed
[exit 1]
```

The harness stopped at line 18 of your script: `alder test`: it exited 1. The commands after it were not run.

Your script stopped at command 5; the app is as the commands before it left it; continue.

The program builds but its tests fail; fix the program or the tests so every test passes.

```text
ok   Post "a Post keeps its author from being deleted"
ok   Admin "a visitor is sent to log in"
ok   Admin "submitting NewPost"
ok   Admin "NewPost refuses an empty title"
ok   Admin "pressing TogglePublished"
ok   Admin "toggling publishes a post"
ok   App "Logout clears the user and goes to log in"
ok   Auth.Login "an unknown email is rejected"
ok   Auth.Login "logging in through the form"
ok   Auth.Login "a signed-in user logs out here"
ok   Comment "deleting its post deletes a Comment"
ok   Home "/ lists only published posts, newest first"
ok   Posts.Show "submitting NewComment"
FAIL Posts.Show "NewComment refuses an empty author"
  error: GET /posts/slug-1: not found
  trace: ["Test.get Posts.Show", "Db.first Post"]
ok   Posts.Show "an unpublished post is not found for a reader"
ok   Posts.Show "a comment with an empty author is refused"
15 passed, 1 failed
```
