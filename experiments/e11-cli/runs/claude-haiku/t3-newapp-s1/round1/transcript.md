### Round 1

```text
$ alder fill --decl Layout.Page - <<'EOF'
<html>
  <head>
    <meta charset="utf-8" />
    <title>{props.title}</title>
    <link rel="stylesheet" href={Url.external "/static/al/al.css"} />
  </head>
  <body>
    <al-page width="narrow">
      <main>{props.children}</main>
    </al-page>
  </body>
</html>
EOF
edited app/Layout.alder
--- a/app/Layout.alder
+++ b/app/Layout.alder
@@ -7,7 +7,6 @@
       <meta charset="utf-8" />
       <title>{props.title}</title>
       <link rel="stylesheet" href={Url.external "/static/al/al.css"} />
-      <link rel="stylesheet" href={Url.external "/static/app.css"} />
     </head>
     <body>
       <al-page width="narrow">
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
$ alder new auth --users User --to Home
wrote adapters/auth/login_password.go
wrote adapters/auth/login_password_test.go
edited app/App.alder
wrote app/Auth/Login.alder
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
$ alder new page Posts "/posts/{slug}" --param slug:Post.Slug --blank
wrote app/Posts.alder
app/Posts.alder:10:23: warning[W0002]: hole ?load has type Query Posts.Model
  load params session = ?load
                        ^^^^^
app/Posts.alder:13:22: warning[W0002]: hole ?view has type Html Posts.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder new page Admin /admin --blank --requires user
wrote app/Admin.alder
app/Admin.alder:8:23: warning[W0002]: hole ?load has type Query Admin.Model
  load params session = ?load
                        ^^^^^
app/Admin.alder:11:22: warning[W0002]: hole ?view has type Html Admin.Msg
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
$ alder fill --decl Home.load - <<'EOF'
{
  let posts <-
    Db.from Post
      |> Db.where (Post.published.eq True)
      |> Db.orderBy Post.created.desc
      |> Db.all
  Query.succeed { posts: posts }
}
EOF
edited app/Home.alder
--- a/app/Home.alder
+++ b/app/Home.alder
@@ -7,7 +7,14 @@
 type Model = { posts: List Post }
 
 load : Params -> Session -> Query Model
-load params session = ?load
+load params session = {
+  let posts <-
+    Db.from Post
+      |> Db.where (Post.published.eq True)
+      |> Db.orderBy Post.created.desc
+      |> Db.all
+  Query.succeed { posts: posts }
+}
 
 view : Model -> Session -> Html Msg
 view model session = ?view
app/Home.alder:20:22: warning[W0002]: hole ?view has type Html Home.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder fill --decl Home.view - <<'EOF'
<Layout.Page title="Blog">
  <h1>Blog</h1>
  <ul>
    {for post in model.posts -> <li><a href={Posts { slug: post.slug }}>{post.title}</a></li>}
  </ul>
</Layout.Page>
EOF
edited app/Home.alder
--- a/app/Home.alder
+++ b/app/Home.alder
@@ -1,5 +1,6 @@
 module Home
 
+import Layout
 import Post
 
 page "/" { }
@@ -17,4 +18,10 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = ?view
+view model session =
+  <Layout.Page title="Blog">
+    <h1>Blog</h1>
+    <ul>
+      {for post in model.posts -> <li><a href={Posts { slug: post.slug }}>{post.title}</a></li>}
+    </ul>
+  </Layout.Page>
added import Layout to app/Home.alder
[exit 0]
$ alder new test Home "lists only published posts newest first"
edited app/Home.alder
--- a/app/Home.alder
+++ b/app/Home.alder
@@ -25,3 +25,7 @@
       {for post in model.posts -> <li><a href={Posts { slug: post.slug }}>{post.title}</a></li>}
     </ul>
   </Layout.Page>
+
+test "lists only published posts newest first" = {
+  ?test
+}
app/Home.alder:30:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Home."lists only published posts newest first"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Db.insert Post { title: Text.NonEmpty "First", slug: Post.Slug "first", body: "one", published: True, author: me }
  let _ <- Db.insert Post { title: Text.NonEmpty "Draft", slug: Post.Slug "draft", body: "two", author: me }
  let _ <- Db.insert Post { title: Text.NonEmpty "Second", slug: Post.Slug "second", body: "three", published: True, author: me }
  let page <- Test.get (Home {})
  Task.succeed (Test.texts "li a" page.doc == ["Second", "First"])
}
EOF
edited app/Home.alder
--- a/app/Home.alder
+++ b/app/Home.alder
@@ -2,6 +2,7 @@
 
 import Layout
 import Post
+import User
 
 page "/" { }
 
@@ -27,5 +28,10 @@
   </Layout.Page>
 
 test "lists only published posts newest first" = {
-  ?test
+  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
+  let _ <- Db.insert Post { title: Text.NonEmpty "First", slug: Post.Slug "first", body: "one", published: True, author: me }
+  let _ <- Db.insert Post { title: Text.NonEmpty "Draft", slug: Post.Slug "draft", body: "two", author: me }
+  let _ <- Db.insert Post { title: Text.NonEmpty "Second", slug: Post.Slug "second", body: "three", published: True, author: me }
+  let page <- Test.get (Home {})
+  Task.succeed (Test.texts "li a" page.doc == ["Second", "First"])
 }
added import User to app/Home.alder
[exit 0]
$ alder fill --decl Posts.Model - <<'EOF'
{ post: Post, comments: List Comment }
EOF
edited app/Posts.alder
--- a/app/Posts.alder
+++ b/app/Posts.alder
@@ -1,10 +1,11 @@
 module Posts
 
+import Comment
 import Post
 
 page "/posts/{slug}" { slug: Post.Slug }
 
-type Model = { }
+type Model = { post: Post, comments: List Comment }
 
 load : Params -> Session -> Query Model
 load params session = ?load
app/Posts.alder:11:23: warning[W0002]: hole ?load has type Query Posts.Model
  load params session = ?load
                        ^^^^^
app/Posts.alder:14:22: warning[W0002]: hole ?view has type Html Posts.Msg
  view model session = ?view
                       ^^^^^
added import Comment to app/Posts.alder
[exit 0]
$ alder fill --decl Posts.load - <<'EOF'
{
  let found <- Db.from Post |> Db.where (Post.slug.eq params.slug) |> Db.first
  match found {
    Nothing -> Query.notFound,
    Just post -> {
      let comments <-
        Db.from Comment
          |> Db.where (Comment.post.eq post.id)
          |> Db.orderBy Comment.created.asc
          |> Db.all
      if post.published || session.user == Just (post.author) then Query.succeed { post: post, comments: comments } else Query.notFound
    },
  }
}
EOF
edited app/Posts.alder
--- a/app/Posts.alder
+++ b/app/Posts.alder
@@ -8,7 +8,20 @@
 type Model = { post: Post, comments: List Comment }
 
 load : Params -> Session -> Query Model
-load params session = ?load
+load params session = {
+  let found <- Db.from Post |> Db.where (Post.slug.eq params.slug) |> Db.first
+  match found {
+    Nothing -> Query.notFound,
+    Just post -> {
+      let comments <-
+        Db.from Comment
+          |> Db.where (Comment.post.eq post.id)
+          |> Db.orderBy Comment.created.asc
+          |> Db.all
+      if post.published || session.user == Just (post.author) then Query.succeed { post: post, comments: comments } else Query.notFound
+    },
+  }
+}
 
 view : Model -> Session -> Html Msg
 view model session = ?view
app/Posts.alder:27:22: warning[W0002]: hole ?view has type Html Posts.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder fill --decl Posts.view - <<'EOF'
<Layout.Page title={model.post.title}>
  <h1>{model.post.title}</h1>
</Layout.Page>
EOF
edited app/Posts.alder
--- a/app/Posts.alder
+++ b/app/Posts.alder
@@ -1,6 +1,7 @@
 module Posts
 
 import Comment
+import Layout
 import Post
 
 page "/posts/{slug}" { slug: Post.Slug }
@@ -24,4 +25,7 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = ?view
+view model session =
+  <Layout.Page title={model.post.title}>
+    <h1>{model.post.title}</h1>
+  </Layout.Page>
added import Layout to app/Posts.alder
[exit 0]
$ alder new form Posts NewComment author:Text.NonEmpty body:Text.NonEmpty --msg Reply
edited app/Posts.alder
--- a/app/Posts.alder
+++ b/app/Posts.alder
@@ -24,8 +24,42 @@
   }
 }
 
+form NewComment = {
+  author: Text.NonEmpty,
+  body: Text.NonEmpty,
+}
+
+msg Reply form NewComment
+
+update : Msg -> Model -> Session -> Task Nav
+update msg model session = match msg {
+  Reply p -> ?reply,
+}
+
 view : Model -> Session -> Html Msg
 view model session =
   <Layout.Page title={model.post.title}>
     <h1>{model.post.title}</h1>
+
+    <al-card>
+      <h2 slot="header">New comment</h2>
+      <form submit={Reply}>
+        <al-input field="author" label="Author" />
+        <al-textarea field="body" label="Body" />
+        <al-alert variant="destructive"><Errors form /></al-alert>
+        <al-button type="submit">New comment</al-button>
+      </form>
+    </al-card>
   </Layout.Page>
+
+test "submitting NewComment" = {
+  let _ <- Test.get (Posts { slug: Post.Slug "slug-1" })
+  let r <- Test.submit NewComment { author: Text.NonEmpty "author-1", body: Text.NonEmpty "body-1" }
+  Task.succeed (r.answer == ?expected)
+}
+
+test "NewComment refuses an empty author" = {
+  let _ <- Test.get (Posts { slug: Post.Slug "slug-1" })
+  let r <- Test.submitRaw NewComment [{ name: "author", value: "" }, { name: "body", value: "x" }]
+  Task.succeed (r.answer == FormRejected [{ field: "author", message: "must not be empty" }])
+}
app/Posts.alder:36:14: warning[W0002]: hole ?reply has type Task Nav
    Reply p -> ?reply,
               ^^^^^^
app/Posts.alder:58:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder fill --decl Posts.view - <<'EOF'
<Layout.Page title={model.post.title}>
  <h1>{model.post.title}</h1>
  <article>{model.post.body}</article>
  <h2>Comments</h2>
  <ul id="comments">
    {for c in model.comments -> <li><strong>{c.author}</strong>: {c.body}</li>}
  </ul>
  <al-card>
    <form submit={Reply}>
      <Errors form />
      <al-input field="author" label="Your name" />
      <al-textarea field="body" label="Comment" />
      <al-button type="submit">Add comment</al-button>
    </form>
  </al-card>
</Layout.Page>
EOF
edited app/Posts.alder
--- a/app/Posts.alder
+++ b/app/Posts.alder
@@ -40,14 +40,17 @@
 view model session =
   <Layout.Page title={model.post.title}>
     <h1>{model.post.title}</h1>
-
+    <article>{model.post.body}</article>
+    <h2>Comments</h2>
+    <ul id="comments">
+      {for c in model.comments -> <li><strong>{c.author}</strong>: {c.body}</li>}
+    </ul>
     <al-card>
-      <h2 slot="header">New comment</h2>
       <form submit={Reply}>
-        <al-input field="author" label="Author" />
-        <al-textarea field="body" label="Body" />
-        <al-alert variant="destructive"><Errors form /></al-alert>
-        <al-button type="submit">New comment</al-button>
+        <Errors form />
+        <al-input field="author" label="Your name" />
+        <al-textarea field="body" label="Comment" />
+        <al-button type="submit">Add comment</al-button>
       </form>
     </al-card>
   </Layout.Page>
app/Posts.alder:36:14: warning[W0002]: hole ?reply has type Task Nav
    Reply p -> ?reply,
               ^^^^^^
app/Posts.alder:61:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder fill --decl Posts.update - <<'EOF'
match msg {
  Reply p -> {
    let _ <- Db.insert Comment { post: model.post.id, author: p.form.author, body: p.form.body }
    Task.succeed Rerender
  },
}
EOF
edited app/Posts.alder
--- a/app/Posts.alder
+++ b/app/Posts.alder
@@ -33,7 +33,10 @@
 
 update : Msg -> Model -> Session -> Task Nav
 update msg model session = match msg {
-  Reply p -> ?reply,
+  Reply p -> {
+    let _ <- Db.insert Comment { post: model.post.id, author: p.form.author, body: p.form.body }
+    Task.succeed Rerender
+  },
 }
 
 view : Model -> Session -> Html Msg
app/Posts.alder:64:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder new test Posts "an unpublished post is not found for a reader"
edited app/Posts.alder
--- a/app/Posts.alder
+++ b/app/Posts.alder
@@ -69,3 +69,7 @@
   let r <- Test.submitRaw NewComment [{ name: "author", value: "" }, { name: "body", value: "x" }]
   Task.succeed (r.answer == FormRejected [{ field: "author", message: "must not be empty" }])
 }
+
+test "an unpublished post is not found for a reader" = {
+  ?test
+}
app/Posts.alder:64:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Posts.alder:74:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Posts."an unpublished post is not found for a reader"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Db.insert Post { title: Text.NonEmpty "Draft", slug: Post.Slug "draft", body: "two", author: me }
  let page <- Test.open (Posts { slug: Post.Slug "draft" })
  Task.succeed (page.answer == PageMissing)
}
EOF
edited app/Posts.alder
--- a/app/Posts.alder
+++ b/app/Posts.alder
@@ -3,6 +3,7 @@
 import Comment
 import Layout
 import Post
+import User
 
 page "/posts/{slug}" { slug: Post.Slug }
 
@@ -71,5 +72,8 @@
 }
 
 test "an unpublished post is not found for a reader" = {
-  ?test
+  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
+  let _ <- Db.insert Post { title: Text.NonEmpty "Draft", slug: Post.Slug "draft", body: "two", author: me }
+  let page <- Test.open (Posts { slug: Post.Slug "draft" })
+  Task.succeed (page.answer == PageMissing)
 }
app/Posts.alder:65:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
added import User to app/Posts.alder
[exit 0]
$ alder new test Posts "an unpublished post is shown to its author"
edited app/Posts.alder
--- a/app/Posts.alder
+++ b/app/Posts.alder
@@ -77,3 +77,7 @@
   let page <- Test.open (Posts { slug: Post.Slug "draft" })
   Task.succeed (page.answer == PageMissing)
 }
+
+test "an unpublished post is shown to its author" = {
+  ?test
+}
app/Posts.alder:65:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Posts.alder:82:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Posts."an unpublished post is shown to its author"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Db.insert Post { title: Text.NonEmpty "Draft", slug: Post.Slug "draft", body: "two", author: me }
  let _ <- Test.login { user: Just me }
  let page <- Test.open (Posts { slug: Post.Slug "draft" })
  Task.succeed (page.answer == Shown)
}
EOF
edited app/Posts.alder
--- a/app/Posts.alder
+++ b/app/Posts.alder
@@ -79,5 +79,9 @@
 }
 
 test "an unpublished post is shown to its author" = {
-  ?test
+  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
+  let _ <- Db.insert Post { title: Text.NonEmpty "Draft", slug: Post.Slug "draft", body: "two", author: me }
+  let _ <- Test.login { user: Just me }
+  let page <- Test.open (Posts { slug: Post.Slug "draft" })
+  Task.succeed (page.answer == Shown)
 }
app/Posts.alder:65:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder new test Posts "a comment with an empty author is refused"
edited app/Posts.alder
--- a/app/Posts.alder
+++ b/app/Posts.alder
@@ -85,3 +85,7 @@
   let page <- Test.open (Posts { slug: Post.Slug "draft" })
   Task.succeed (page.answer == Shown)
 }
+
+test "a comment with an empty author is refused" = {
+  ?test
+}
app/Posts.alder:65:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Posts.alder:90:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Posts."a comment with an empty author is refused"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Db.insert Post { title: Text.NonEmpty "First", slug: Post.Slug "first", body: "one", published: True, author: me }
  let _ <- Test.get (Posts { slug: Post.Slug "first" })
  let r <- Test.submitRaw NewComment [{ name: "author", value: "" }, { name: "body", value: "Nice post" }]
  let n <- Db.from Comment |> Db.count
  let rejected = match r.answer {
    FormRejected _ -> True,
    _ -> False,
  }
  Task.succeed ({ rejected: rejected, comments: n } == { rejected: True, comments: 0 })
}
EOF
edited app/Posts.alder
--- a/app/Posts.alder
+++ b/app/Posts.alder
@@ -87,5 +87,14 @@
 }
 
 test "a comment with an empty author is refused" = {
-  ?test
+  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
+  let _ <- Db.insert Post { title: Text.NonEmpty "First", slug: Post.Slug "first", body: "one", published: True, author: me }
+  let _ <- Test.get (Posts { slug: Post.Slug "first" })
+  let r <- Test.submitRaw NewComment [{ name: "author", value: "" }, { name: "body", value: "Nice post" }]
+  let n <- Db.from Comment |> Db.count
+  let rejected = match r.answer {
+    FormRejected _ -> True,
+    _ -> False,
+  }
+  Task.succeed ({ rejected: rejected, comments: n } == { rejected: True, comments: 0 })
 }
app/Posts.alder:65:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder fill --decl Admin.Model - <<'EOF'
{ posts: List Post }
EOF
edited app/Admin.alder
--- a/app/Admin.alder
+++ b/app/Admin.alder
@@ -1,8 +1,10 @@
 module Admin
 
+import Post
+
 page "/admin" { } requires user
 
-type Model = { }
+type Model = { posts: List Post }
 
 load : Params -> Session -> Query Model
 load params session = ?load
app/Admin.alder:10:23: warning[W0002]: hole ?load has type Query Admin.Model
  load params session = ?load
                        ^^^^^
app/Admin.alder:13:22: warning[W0002]: hole ?view has type Html Admin.Msg
  view model session = ?view
                       ^^^^^
added import Post to app/Admin.alder
[exit 0]
$ alder fill --decl Admin.load - <<'EOF'
{
  let posts <-
    Db.from Post
      |> Db.orderBy Post.created.desc
      |> Db.all
  Query.succeed { posts: posts }
}
EOF
edited app/Admin.alder
--- a/app/Admin.alder
+++ b/app/Admin.alder
@@ -7,7 +7,13 @@
 type Model = { posts: List Post }
 
 load : Params -> Session -> Query Model
-load params session = ?load
+load params session = {
+  let posts <-
+    Db.from Post
+      |> Db.orderBy Post.created.desc
+      |> Db.all
+  Query.succeed { posts: posts }
+}
 
 view : Model -> Session -> Html Msg
 view model session = ?view
app/Admin.alder:19:22: warning[W0002]: hole ?view has type Html Admin.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder fill --decl Admin.view - <<'EOF'
<Layout.Page title="Admin">
  <h1>Admin</h1>
</Layout.Page>
EOF
edited app/Admin.alder
--- a/app/Admin.alder
+++ b/app/Admin.alder
@@ -1,5 +1,6 @@
 module Admin
 
+import Layout
 import Post
 
 page "/admin" { } requires user
@@ -16,7 +17,10 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = ?view
+view model session =
+  <Layout.Page title="Admin">
+    <h1>Admin</h1>
+  </Layout.Page>
 
 test "a visitor is sent to log in" = {
   let r <- Test.open (Admin {})
added import Layout to app/Admin.alder
[exit 0]
$ alder new form Admin NewPost title:Text.NonEmpty slug:Post.Slug body:Text --msg Create
edited app/Admin.alder
--- a/app/Admin.alder
+++ b/app/Admin.alder
@@ -2,6 +2,7 @@
 
 import Layout
 import Post
+import User
 
 page "/admin" { } requires user
 
@@ -16,13 +17,53 @@
   Query.succeed { posts: posts }
 }
 
+form NewPost = {
+  title: Text.NonEmpty,
+  slug: Post.Slug,
+  body: Text,
+}
+
+msg Create form NewPost
+
+update : Msg -> Model -> Session -> Task Nav
+update msg model session = match msg {
+  Create p -> ?create,
+}
+
 view : Model -> Session -> Html Msg
 view model session =
   <Layout.Page title="Admin">
     <h1>Admin</h1>
+
+    <al-card>
+      <h2 slot="header">New post</h2>
+      <form submit={Create}>
+        <al-input field="title" label="Title" />
+        <al-input field="slug" label="Slug" />
+        <al-textarea field="body" label="Body" />
+        <al-alert variant="destructive"><Errors form /></al-alert>
+        <al-button type="submit">New post</al-button>
+      </form>
+    </al-card>
   </Layout.Page>
 
 test "a visitor is sent to log in" = {
   let r <- Test.open (Admin {})
   Task.succeed (r.answer == Redirected (Auth.Login {}))
 }
+
+test "submitting NewPost" = {
+  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
+  let _ <- Test.login { user: Just me }
+  let _ <- Test.get (Admin {})
+  let r <- Test.submit NewPost { title: Text.NonEmpty "title-1", slug: Post.Slug "slug-1", body: "body-1" }
+  Task.succeed (r.answer == ?expected)
+}
+
+test "NewPost refuses an empty title" = {
+  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
+  let _ <- Test.login { user: Just me }
+  let _ <- Test.get (Admin {})
+  let r <- Test.submitRaw NewPost [{ name: "title", value: "" }, { name: "slug", value: "x" }, { name: "body", value: "x" }]
+  Task.succeed (r.answer == FormRejected [{ field: "title", message: "must not be empty" }])
+}
app/Admin.alder:30:15: warning[W0002]: hole ?create has type Task Nav
    Create p -> ?create,
                ^^^^^^^
app/Admin.alder:60:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder new msg Admin Toggle id:Post.PostId
edited app/Admin.alder
--- a/app/Admin.alder
+++ b/app/Admin.alder
@@ -25,9 +25,12 @@
 
 msg Create form NewPost
 
+msg Toggle { id: Post.PostId }
+
 update : Msg -> Model -> Session -> Task Nav
 update msg model session = match msg {
   Create p -> ?create,
+  Toggle p -> ?toggle,
 }
 
 view : Model -> Session -> Html Msg
@@ -45,6 +48,8 @@
         <al-button type="submit">New post</al-button>
       </form>
     </al-card>
+
+    <al-button click={Toggle { id: ?toggleId }}>Toggle</al-button>
   </Layout.Page>
 
 test "a visitor is sent to log in" = {
@@ -67,3 +72,16 @@
   let r <- Test.submitRaw NewPost [{ name: "title", value: "" }, { name: "slug", value: "x" }, { name: "body", value: "x" }]
   Task.succeed (r.answer == FormRejected [{ field: "title", message: "must not be empty" }])
 }
+
+-- Fill ?expected with what pressing Toggle must do, so that a test fails when the arm
+-- does something else: its answer (`r.answer`: Rerendered, Redirected …) and what it
+-- changed (`after`, the row read again), as `{ answer: r.answer, … } == { answer: Rerendered, … }`.
+test "pressing Toggle" = {
+  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
+  let _ <- Test.login { user: Just me }
+  let post1 <- Db.insert Post { title: Text.NonEmpty "title-1", slug: Post.Slug "slug-1", body: "body-1", author: me }
+  let _ <- Test.get (Admin {})
+  let r <- Test.click (Toggle { id: post1 })
+  let after <- Db.get Post post1
+  Task.succeed ?expected
+}
app/Admin.alder:32:15: warning[W0002]: hole ?create has type Task Nav
    Create p -> ?create,
                ^^^^^^^
app/Admin.alder:33:15: warning[W0002]: hole ?toggle has type Task Nav
    Toggle p -> ?toggle,
                ^^^^^^^
app/Admin.alder:52:36: warning[W0002]: hole ?toggleId has type Post.PostId
      <al-button click={Toggle { id: ?toggleId }}>Toggle</al-button>
                                     ^^^^^^^^^
app/Admin.alder:65:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Admin.alder:86:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill --decl Admin.view - <<'EOF'
<Layout.Page title="Admin">
  <h1>Admin</h1>
  <al-card>
    <form submit={Create}>
      <Errors form />
      <al-input field="title" label="Title" />
      <al-input field="slug" label="Slug" />
      <al-textarea field="body" label="Body" />
      <al-button type="submit">Create post</al-button>
    </form>
  </al-card>
  <ul id="posts">
    {for post in model.posts ->
      <li>
        <a href={Posts { slug: post.slug }}>{post.title}</a>
        {if post.published then " (published)" else " (draft)"}
        <al-button variant="secondary" size="sm" click={Toggle { id: post.id }}>{if post.published then "Unpublish" else "Publish"}</al-button>
      </li>}
  </ul>
  <al-button variant="outline" click={App.Logout}>Log out</al-button>
</Layout.Page>
EOF
edited app/Admin.alder
--- a/app/Admin.alder
+++ b/app/Admin.alder
@@ -37,19 +37,24 @@
 view model session =
   <Layout.Page title="Admin">
     <h1>Admin</h1>
-
     <al-card>
-      <h2 slot="header">New post</h2>
       <form submit={Create}>
+        <Errors form />
         <al-input field="title" label="Title" />
         <al-input field="slug" label="Slug" />
         <al-textarea field="body" label="Body" />
-        <al-alert variant="destructive"><Errors form /></al-alert>
-        <al-button type="submit">New post</al-button>
+        <al-button type="submit">Create post</al-button>
       </form>
     </al-card>
-
-    <al-button click={Toggle { id: ?toggleId }}>Toggle</al-button>
+    <ul id="posts">
+      {for post in model.posts ->
+        <li>
+          <a href={Posts { slug: post.slug }}>{post.title}</a>
+          {if post.published then " (published)" else " (draft)"}
+          <al-button variant="secondary" size="sm" click={Toggle { id: post.id }}>{if post.published then "Unpublish" else "Publish"}</al-button>
+        </li>}
+    </ul>
+    <al-button variant="outline" click={App.Logout}>Log out</al-button>
   </Layout.Page>
 
 test "a visitor is sent to log in" = {
app/Admin.alder:32:15: warning[W0002]: hole ?create has type Task Nav
    Create p -> ?create,
                ^^^^^^^
app/Admin.alder:33:15: warning[W0002]: hole ?toggle has type Task Nav
    Toggle p -> ?toggle,
                ^^^^^^^
app/Admin.alder:70:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Admin.alder:91:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill --decl Admin.update - <<'EOF'
match msg {
  Create p -> {
    let _ <- Db.insert Post { title: p.form.title, slug: p.form.slug, body: p.form.body, author: session.user }
    Task.succeed Rerender
  },
  Toggle p -> {
    let found <- Db.get Post p.id
    match found {
      Just post -> {
        if post.author == session.user then {
          let _ <- Db.update Post p.id (\t -> { t | published: not t.published })
          Task.succeed Rerender
        } else Task.notFound
      },
      Nothing -> Task.notFound,
    }
  },
}
EOF
edited app/Admin.alder
--- a/app/Admin.alder
+++ b/app/Admin.alder
@@ -29,8 +29,22 @@
 
 update : Msg -> Model -> Session -> Task Nav
 update msg model session = match msg {
-  Create p -> ?create,
-  Toggle p -> ?toggle,
+  Create p -> {
+    let _ <- Db.insert Post { title: p.form.title, slug: p.form.slug, body: p.form.body, author: session.user }
+    Task.succeed Rerender
+  },
+  Toggle p -> {
+    let found <- Db.get Post p.id
+    match found {
+      Just post -> {
+        if post.author == session.user then {
+          let _ <- Db.update Post p.id (\t -> { t | published: not t.published })
+          Task.succeed Rerender
+        } else Task.notFound
+      },
+      Nothing -> Task.notFound,
+    }
+  },
 }
 
 view : Model -> Session -> Html Msg
app/Admin.alder:84:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Admin.alder:105:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder new test Admin "a visitor is sent to log in"
alder new test: `Admin` already has a test "a visitor is sent to log in": choose another name
[exit 1]
```

The harness stopped at line 207 of your script: `alder new test Admin "a visitor is sent to log in"`: it exited 1. The commands after it were not run.

Your script stopped at command 30; the app is as the commands before it left it; continue.

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Admin.alder:84:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Admin.alder:105:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Posts.alder:65:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
alder build: the program has 3 typed hole(s); fill them before building
```
