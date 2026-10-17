### Round 0

```text
$ alder new type Post 'Slug = brand Text where Text.matches "^[a-z0-9-]+$"'
wrote app/Post.alder
[exit 0]
$ alder new table Post title:Text.NonEmpty slug:Slug@unique body:Text published:Bool=False
edited app/Post.alder
wrote migrations/0001_initial.sql
  create table Post
wrote migrations/schema.json
--- a/app/Post.alder
+++ b/app/Post.alder
@@ -1,3 +1,15 @@
 module Post
 
 type Slug = brand Text where Text.matches "^[a-z0-9-]+$"
+
+type PostId = brand Text
+
+table Post {
+  id: PostId @id,
+  title: Text.NonEmpty,
+  slug: Slug @unique,
+  body: Text,
+  published: Bool = False,
+  created: Time @created,
+  updated: Time @updated,
+}
[exit 0]
$ alder new table Comment post:Post.PostId@ref=Post@cascade author:Text.NonEmpty body:Text.NonEmpty
wrote app/Comment.alder
wrote migrations/0002_create_comment.sql
  create table Comment
  add index on Comment.post
edited migrations/schema.json
[exit 0]
$ alder new page Posts.Index / --blank
wrote app/Posts/Index.alder
app/Posts/Index.alder:8:23: warning[W0002]: hole ?load has type Query Posts.Index.Model
  load params session = ?load
                        ^^^^^
app/Posts/Index.alder:11:22: warning[W0002]: hole ?view has type Html Posts.Index.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder new auth --to Posts.Index
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
$ alder new page Posts.Show '/posts/{slug}' --blank --param slug:Post.Slug
wrote app/Posts/Show.alder
app/Posts/Show.alder:10:23: warning[W0002]: hole ?load has type Query Posts.Show.Model
  load params session = ?load
                        ^^^^^
app/Posts/Show.alder:13:22: warning[W0002]: hole ?view has type Html Posts.Show.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder new page Admin.Index /admin --blank --requires user
wrote app/Admin/Index.alder
app/Admin/Index.alder:8:23: warning[W0002]: hole ?load has type Query Admin.Index.Model
  load params session = ?load
                        ^^^^^
app/Admin/Index.alder:11:22: warning[W0002]: hole ?view has type Html Admin.Index.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder fill --decl Posts.Index.Model - <<'EOF'
{ posts: List Post }
EOF
edited app/Posts/Index.alder
--- a/app/Posts/Index.alder
+++ b/app/Posts/Index.alder
@@ -1,8 +1,10 @@
 module Posts.Index
 
+import Post
+
 page "/" { }
 
-type Model = { }
+type Model = { posts: List Post }
 
 load : Params -> Session -> Query Model
 load params session = ?load
app/Posts/Index.alder:10:23: warning[W0002]: hole ?load has type Query Posts.Index.Model
  load params session = ?load
                        ^^^^^
app/Posts/Index.alder:13:22: warning[W0002]: hole ?view has type Html Posts.Index.Msg
  view model session = ?view
                       ^^^^^
added import Post to app/Posts/Index.alder
[exit 0]
$ alder fill Posts.Index.load - <<'EOF'
\params session -> {
  let posts <- Db.from Post |> Db.where (Post.published.eq True) |> Db.orderBy Post.created.desc |> Db.all
  Query.succeed { posts: posts }
}
EOF
alder fill: the parameters `params session` are bound already: send the body without `\params session ->`
[exit 1]
```

The harness stopped at line 16 of your script: `alder fill Posts.Index.load - <<'EOF'`: it exited 1. The commands after it were not run.

Your script stopped at command 9; the app is as the commands before it left it; continue.

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Admin/Index.alder:8:23: warning[W0002]: hole ?load has type Query Admin.Index.Model
  load params session = ?load
                        ^^^^^
app/Admin/Index.alder:11:22: warning[W0002]: hole ?view has type Html Admin.Index.Msg
  view model session = ?view
                       ^^^^^
app/Posts/Index.alder:10:23: warning[W0002]: hole ?load has type Query Posts.Index.Model
  load params session = ?load
                        ^^^^^
app/Posts/Index.alder:13:22: warning[W0002]: hole ?view has type Html Posts.Index.Msg
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
