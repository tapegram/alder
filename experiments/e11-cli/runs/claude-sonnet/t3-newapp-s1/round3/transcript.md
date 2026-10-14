### Round 3

```text
$ alder fill Desk.update.Toggle - <<'EOF'
{
  let _ <- Db.update Post p.id (\t -> { t | published: not t.published })
  Task.succeed Rerender
}
EOF
edited app/Desk.alder
--- a/app/Desk.alder
+++ b/app/Desk.alder
@@ -32,7 +32,10 @@
     let _ <- Db.insert Post { title: p.form.title, slug: p.form.slug, body: p.form.body, published: p.form.published }
     Task.succeed (Redirect (Desk {}))
   },
-  Toggle p -> ?toggle,
+  Toggle p -> {
+    let _ <- Db.update Post p.id (\t -> { t | published: not t.published })
+    Task.succeed Rerender
+  },
 }
 
 view : Model -> Session -> Html Msg
app/Desk.alder:98:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill 'Desk."pressing Toggle"' - --hole ?expected <<'EOF'
{ answer: r.answer, published: Maybe.map (\x -> x.published) after } == { answer: Rerendered, published: Just True }
EOF
edited app/Desk.alder
--- a/app/Desk.alder
+++ b/app/Desk.alder
@@ -85,9 +85,6 @@
   Task.succeed ({ answer: r.answer, rows: rows } == { answer: FormRejected [{ field: "title", message: "must not be empty" }], rows: 0 })
 }
 
--- Fill ?expected with what pressing Toggle must do, so that a test fails when the arm
--- does something else: its answer (`r.answer`: Rerendered, Redirected …) and what it
--- changed (`after`, the row read again), as `{ answer: r.answer, … } == { answer: Rerendered, … }`.
 test "pressing Toggle" = {
   let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: Text.Secret "passwordhash-1" }
   let _ <- Test.login { user: Just me }
@@ -95,5 +92,5 @@
   let _ <- Test.get (Desk {})
   let r <- Test.click (Toggle { id: post1 })
   let after <- Db.get Post post1
-  Task.succeed ?expected
+  Task.succeed ({ answer: r.answer, published: Maybe.map (\x -> x.published) after } == { answer: Rerendered, published: Just True })
 }
[exit 0]
$ alder fill --decl 'Posts.Show."submitting NewComment"' - <<'EOF'
{
  let _ <- Db.insert Post { title: Text.NonEmpty "Hello", slug: Slug.Slug "slug-1", body: "b", published: True }
  let _ <- Test.get (Posts.Show { slug: Slug.Slug "slug-1" })
  let r <- Test.submit NewComment { author: Text.NonEmpty "Ann", body: Text.NonEmpty "Nice" }
  let n <- Db.from Comment |> Db.count
  Task.succeed ({ answer: r.answer, n: n } == { answer: Rerendered, n: 1 })
}
EOF
edited app/Posts/Show.alder
--- a/app/Posts/Show.alder
+++ b/app/Posts/Show.alder
@@ -55,9 +55,11 @@
   </Layout.Page>
 
 test "submitting NewComment" = {
+  let _ <- Db.insert Post { title: Text.NonEmpty "Hello", slug: Slug.Slug "slug-1", body: "b", published: True }
   let _ <- Test.get (Posts.Show { slug: Slug.Slug "slug-1" })
-  let r <- Test.submit NewComment { author: Text.NonEmpty "author-1", body: Text.NonEmpty "body-1" }
-  Task.succeed (r.answer == ?expected)
+  let r <- Test.submit NewComment { author: Text.NonEmpty "Ann", body: Text.NonEmpty "Nice" }
+  let n <- Db.from Comment |> Db.count
+  Task.succeed ({ answer: r.answer, n: n } == { answer: Rerendered, n: 1 })
 }
 
 test "NewComment refuses an empty author" = {
rewriting a test: `alder test --mutate` is the check that it still tests something
[exit 0]
$ alder check
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
FAIL Posts.Show "NewComment refuses an empty author"
  error: GET /posts/slug-1: not found
  trace: ["Test.get Posts.Show", "Db.first Post"]
ok   Posts.Show "an unpublished post is not found for a reader"
ok   Posts.Show "a comment with an empty author is refused"
13 passed, 1 failed
[exit 1]
```

The harness stopped at line 20 of your script: `alder test`: it exited 1. The commands after it were not run.

Your script stopped at command 5; the app is as the commands before it left it; continue.

The program builds but its tests fail; fix the program or the tests so every test passes.

```text
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
FAIL Posts.Show "NewComment refuses an empty author"
  error: GET /posts/slug-1: not found
  trace: ["Test.get Posts.Show", "Db.first Post"]
ok   Posts.Show "an unpublished post is not found for a reader"
ok   Posts.Show "a comment with an empty author is refused"
13 passed, 1 failed
```
