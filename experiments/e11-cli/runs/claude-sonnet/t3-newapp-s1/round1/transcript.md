### Round 1

```text
$ alder new table Post title:Text.NonEmpty slug:Slug.Slug@unique body:Text published:Bool=False
wrote app/Post.alder
wrote migrations/0001_initial.sql
  create table Post
wrote migrations/schema.json
[exit 0]
$ alder new table Comment post:Post.PostId@ref=Post@cascade author:Text.NonEmpty body:Text.NonEmpty
wrote app/Comment.alder
wrote migrations/0002_create_comment.sql
  create table Comment
  add index on Comment.post
edited migrations/schema.json
[exit 0]
$ alder new page Home / --blank
wrote app/Home.alder
app/Home.alder:8:23: warning[W0002]: hole ?load has type Query Home.Model
  load params session = ?load
                        ^^^^^
app/Home.alder:11:22: warning[W0002]: hole ?view has type Html Home.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder new auth
wrote adapters/auth/login_password.go
wrote adapters/auth/login_password_test.go
edited app/App.alder
wrote app/Auth/Login.alder
wrote app/User.alder
wrote migrations/0003_create_user.sql
  create table User
edited migrations/schema.json
--- a/app/App.alder
+++ b/app/App.alder
@@ -1,9 +1,22 @@
 module App
 
 import Layout
+import User
 
-session { }
+session { user: Maybe User.UserId }
 
+login = Auth.Login
+
+msg Logout
+
+update : Msg -> Session -> Task Nav
+update msg session = match msg {
+  Logout p -> {
+    let _ <- Session.put (\s -> { s | user: Nothing })
+    Task.succeed (Redirect (Auth.Login {}))
+  },
+}
+
 error : Error -> Html Never
 error err =
   <Layout.Page title="Error">
@@ -16,3 +29,10 @@
   <Layout.Page title="Not found">
     <h1>Not found</h1>
   </Layout.Page>
+
+-- Every Log out button is on a page that requires a user, so a browser test cannot tell this
+-- Redirect from a Rerender (book 10, "The logout redirect"): the test calls update itself.
+test "Logout clears the user and goes to log in" = {
+  let r <- Test.run (update Logout { user: Just (User.UserId "me") })
+  Task.succeed ({ value: r.value, trace: r.trace } == { value: Ok (Redirect (Auth.Login {})), trace: ["Session.put"] })
+}
[exit 0]
$ alder new page Posts.Show '/posts/{slug}' --blank --param slug:Slug.Slug
wrote app/Posts/Show.alder
app/Posts/Show.alder:10:23: warning[W0002]: hole ?load has type Query Posts.Show.Model
  load params session = ?load
                        ^^^^^
app/Posts/Show.alder:13:22: warning[W0002]: hole ?view has type Html Posts.Show.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder new page Desk /admin --blank --requires user
wrote app/Desk.alder
app/Desk.alder:8:23: warning[W0002]: hole ?load has type Query Desk.Model
  load params session = ?load
                        ^^^^^
app/Desk.alder:11:22: warning[W0002]: hole ?view has type Html Desk.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder fill --decl Home.Model - <<'EOF'
{ posts: List Post }
EOF
edited app/Home.alder
--- a/app/Home.alder
+++ b/app/Home.alder
@@ -1,8 +1,10 @@
 module Home
 
+import Post
+
 page "/" { }
 
-type Model = { }
+type Model = { posts: List Post }
 
 load : Params -> Session -> Query Model
 load params session = ?load
app/Home.alder:10:23: warning[W0002]: hole ?load has type Query Home.Model
  load params session = ?load
                        ^^^^^
app/Home.alder:13:22: warning[W0002]: hole ?view has type Html Home.Msg
  view model session = ?view
                       ^^^^^
added import Post to app/Home.alder
[exit 0]
$ alder fill Home.load - <<'EOF'
\params session -> {
  let posts <- Db.from Post |> Db.where (Post.published.eq True) |> Db.orderBy Post.created.desc |> Db.all
  Query.succeed { posts: posts }
}
EOF
app/Home.alder:10:23: error[E0003]: expected Query Home.Model, got _ -> _ -> Query { posts: List Post }
  load params session = \params session -> {
                        ^
app/Home.alder:16:22: warning[W0002]: hole ?view has type Html Home.Msg
  view model session = ?view
                       ^^^^^
alder fill: the result does not check (the errors above): nothing was written
[exit 1]
```

The harness stopped at line 11 of your script: `alder fill Home.load - <<'EOF'`: it exited 1. The commands after it were not run.

Your script stopped at command 8; the app is as the commands before it left it; continue.

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Desk.alder:8:23: warning[W0002]: hole ?load has type Query Desk.Model
  load params session = ?load
                        ^^^^^
app/Desk.alder:11:22: warning[W0002]: hole ?view has type Html Desk.Msg
  view model session = ?view
                       ^^^^^
app/Home.alder:10:23: warning[W0002]: hole ?load has type Query Home.Model
  load params session = ?load
                        ^^^^^
app/Home.alder:13:22: warning[W0002]: hole ?view has type Html Home.Msg
  view model session = ?view
                       ^^^^^
app/Posts/Show.alder:10:23: warning[W0002]: hole ?load has type Query Posts.Show.Model
  load params session = ?load
                        ^^^^^
app/Posts/Show.alder:13:22: warning[W0002]: hole ?view has type Html Posts.Show.Msg
  view model session = ?view
                       ^^^^^
alder build: the program has 6 typed hole(s); fill them before building
```
