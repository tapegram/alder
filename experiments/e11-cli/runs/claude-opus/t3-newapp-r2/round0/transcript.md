### Round 0

```text
$ alder new table User email:Text.Email@unique name:Text.NonEmpty passwordHash:Text created:Time@created --no-timestamps
wrote app/User.alder
wrote migrations/0001_initial.sql
  create table User
wrote migrations/schema.json
[exit 0]
$ alder new type Post 'Slug = brand Text where Text.matches "^[a-z0-9-]+$"'
wrote app/Post.alder
[exit 0]
$ alder new table Post title:Text.NonEmpty slug:Slug@unique body:Text published:Bool=False author:User.UserId@ref=User created:Time@created --no-timestamps
edited app/Post.alder
wrote migrations/0002_create_post.sql
  create table Post
  add index on Post.author
edited migrations/schema.json
--- a/app/Post.alder
+++ b/app/Post.alder
@@ -1,3 +1,25 @@
 module Post
 
+import User
+
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
+  author: User.UserId @ref User,
+  created: Time @created,
+}
+
+test "a Post keeps its author from being deleted" = {
+  let target <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
+  let _ <- Db.insert Post { title: Text.NonEmpty "title-1", slug: Slug "slug-1", body: "body-1", author: target }
+  let r <- Test.run (Db.delete User target)
+  let left <- Db.from Post |> Db.count
+  Task.succeed ({ deleted: Result.toMaybe r.value, left: left } == { deleted: Nothing, left: 1 })
+}
[exit 0]
$ alder new table Comment post:Post.PostId@ref=Post@cascade author:Text.NonEmpty body:Text.NonEmpty created:Time@created --no-timestamps
wrote app/Comment.alder
wrote migrations/0003_create_comment.sql
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
$ alder new page Posts.Show '/posts/{slug}' --param slug:Post.Slug --blank
wrote app/Posts/Show.alder
app/Posts/Show.alder:10:23: warning[W0002]: hole ?load has type Query Posts.Show.Model
  load params session = ?load
                        ^^^^^
app/Posts/Show.alder:13:22: warning[W0002]: hole ?view has type Html Posts.Show.Msg
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
{
  let posts <- Db.from Post |> Db.where (Post.published.eq True) |> Db.orderBy Post.created.desc |> Db.all
  Query.succeed { posts: posts }
}
EOF
edited app/Home.alder
--- a/app/Home.alder
+++ b/app/Home.alder
@@ -7,7 +7,10 @@
 type Model = { posts: List Post }
 
 load : Params -> Session -> Query Model
-load params session = ?load
+load params session = {
+  let posts <- Db.from Post |> Db.where (Post.published.eq True) |> Db.orderBy Post.created.desc |> Db.all
+  Query.succeed { posts: posts }
+}
 
 view : Model -> Session -> Html Msg
 view model session = ?view
app/Home.alder:16:22: warning[W0002]: hole ?view has type Html Home.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder fill Home.view - <<'EOF'
<Layout.Page title="Blog">
  <h1>Blog</h1>
  <ul id="posts">
    {for post in model.posts -> <li><a href={Posts.Show { slug: post.slug }}>{post.title}</a></li>}
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
@@ -13,4 +14,9 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = ?view
+view model session = <Layout.Page title="Blog">
+  <h1>Blog</h1>
+  <ul id="posts">
+    {for post in model.posts -> <li><a href={Posts.Show { slug: post.slug }}>{post.title}</a></li>}
+  </ul>
+</Layout.Page>
added import Layout to app/Home.alder
[exit 0]
$ alder new auth --users User
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
$ alder fill --decl Posts.Show.Model - <<'EOF'
{ post: Post, comments: List Comment }
EOF
edited app/Posts/Show.alder
--- a/app/Posts/Show.alder
+++ b/app/Posts/Show.alder
@@ -1,10 +1,11 @@
 module Posts.Show
 
+import Comment
 import Post
 
 page "/posts/{slug}" { slug: Post.Slug }
 
-type Model = { }
+type Model = { post: Post, comments: List Comment }
 
 load : Params -> Session -> Query Model
 load params session = ?load
app/Posts/Show.alder:11:23: warning[W0002]: hole ?load has type Query Posts.Show.Model
  load params session = ?load
                        ^^^^^
app/Posts/Show.alder:14:22: warning[W0002]: hole ?view has type Html Posts.Show.Msg
  view model session = ?view
                       ^^^^^
added import Comment to app/Posts/Show.alder
[exit 0]
$ alder fill Posts.Show.load - <<'EOF'
{
  let found <- Db.from Post |> Db.where (Post.slug.eq params.slug) |> Db.first
  match found {
    Just post -> {
      if post.published || session.user == Just post.author then {
        let comments <- Db.from Comment |> Db.where (Comment.post.eq post.id) |> Db.orderBy Comment.created.asc |> Db.all
        Query.succeed { post: post, comments: comments }
      } else Query.notFound
    },
    Nothing -> Query.notFound,
  }
}
EOF
edited app/Posts/Show.alder
--- a/app/Posts/Show.alder
+++ b/app/Posts/Show.alder
@@ -8,7 +8,18 @@
 type Model = { post: Post, comments: List Comment }
 
 load : Params -> Session -> Query Model
-load params session = ?load
+load params session = {
+  let found <- Db.from Post |> Db.where (Post.slug.eq params.slug) |> Db.first
+  match found {
+    Just post -> {
+      if post.published || session.user == Just post.author then {
+        let comments <- Db.from Comment |> Db.where (Comment.post.eq post.id) |> Db.orderBy Comment.created.asc |> Db.all
+        Query.succeed { post: post, comments: comments }
+      } else Query.notFound
+    },
+    Nothing -> Query.notFound,
+  }
+}
 
 view : Model -> Session -> Html Msg
 view model session = ?view
app/Posts/Show.alder:25:22: warning[W0002]: hole ?view has type Html Posts.Show.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder fill Posts.Show.view - <<'EOF'
<Layout.Page title={model.post.title}>
  <h1>{model.post.title}</h1>
</Layout.Page>
EOF
edited app/Posts/Show.alder
--- a/app/Posts/Show.alder
+++ b/app/Posts/Show.alder
@@ -1,6 +1,7 @@
 module Posts.Show
 
 import Comment
+import Layout
 import Post
 
 page "/posts/{slug}" { slug: Post.Slug }
@@ -22,4 +23,6 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = ?view
+view model session = <Layout.Page title={model.post.title}>
+  <h1>{model.post.title}</h1>
+</Layout.Page>
added import Layout to app/Posts/Show.alder
[exit 0]
$ alder new form Posts.Show NewComment author:Text.NonEmpty body:Text.NonEmpty --msg AddComment
edited app/Posts/Show.alder
--- a/app/Posts/Show.alder
+++ b/app/Posts/Show.alder
@@ -22,7 +22,41 @@
   }
 }
 
+form NewComment = {
+  author: Text.NonEmpty,
+  body: Text.NonEmpty,
+}
+
+msg AddComment form NewComment
+
+update : Msg -> Model -> Session -> Task Nav
+update msg model session = match msg {
+  AddComment p -> ?addComment,
+}
+
 view : Model -> Session -> Html Msg
 view model session = <Layout.Page title={model.post.title}>
   <h1>{model.post.title}</h1>
+
+  <al-card>
+    <h2 slot="header">New comment</h2>
+    <form submit={AddComment}>
+      <al-input field="author" label="Author" />
+      <al-textarea field="body" label="Body" />
+      <al-alert variant="destructive"><Errors form /></al-alert>
+      <al-button type="submit">New comment</al-button>
+    </form>
+  </al-card>
 </Layout.Page>
+
+test "submitting NewComment" = {
+  let _ <- Test.get (Posts.Show { slug: Post.Slug "slug-1" })
+  let r <- Test.submit NewComment { author: Text.NonEmpty "author-1", body: Text.NonEmpty "body-1" }
+  Task.succeed (r.answer == ?expected)
+}
+
+test "NewComment refuses an empty author" = {
+  let _ <- Test.get (Posts.Show { slug: Post.Slug "slug-1" })
+  let r <- Test.submitRaw NewComment [{ name: "author", value: "" }, { name: "body", value: "x" }]
+  Task.succeed (r.answer == FormRejected [{ field: "author", message: "must not be empty" }])
+}
app/Posts/Show.alder:34:19: warning[W0002]: hole ?addComment has type Task Nav
    AddComment p -> ?addComment,
                    ^^^^^^^^^^^
app/Posts/Show.alder:55:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder fill --decl Posts.Show.view - <<'EOF'
<Layout.Page title={model.post.title}>
  <h1>{model.post.title}</h1>
  <article>{model.post.body}</article>
  <h2>Comments</h2>
  <ul id="comments">
    {for c in model.comments -> <li><strong>{c.author}</strong> {c.body}</li>}
  </ul>
  <form submit={AddComment}>
    <Errors form />
    <Input field="author" label="Name" />
    <Errors field="author" />
    <Textarea field="body" label="Comment" />
    <Errors field="body" />
    <button>Add comment</button>
  </form>
</Layout.Page>
EOF
edited app/Posts/Show.alder
--- a/app/Posts/Show.alder
+++ b/app/Posts/Show.alder
@@ -35,19 +35,23 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = <Layout.Page title={model.post.title}>
-  <h1>{model.post.title}</h1>
-
-  <al-card>
-    <h2 slot="header">New comment</h2>
+view model session =
+  <Layout.Page title={model.post.title}>
+    <h1>{model.post.title}</h1>
+    <article>{model.post.body}</article>
+    <h2>Comments</h2>
+    <ul id="comments">
+      {for c in model.comments -> <li><strong>{c.author}</strong> {c.body}</li>}
+    </ul>
     <form submit={AddComment}>
-      <al-input field="author" label="Author" />
-      <al-textarea field="body" label="Body" />
-      <al-alert variant="destructive"><Errors form /></al-alert>
-      <al-button type="submit">New comment</al-button>
+      <Errors form />
+      <Input field="author" label="Name" />
+      <Errors field="author" />
+      <Textarea field="body" label="Comment" />
+      <Errors field="body" />
+      <button>Add comment</button>
     </form>
-  </al-card>
-</Layout.Page>
+  </Layout.Page>
 
 test "submitting NewComment" = {
   let _ <- Test.get (Posts.Show { slug: Post.Slug "slug-1" })
app/Posts/Show.alder:34:19: warning[W0002]: hole ?addComment has type Task Nav
    AddComment p -> ?addComment,
                    ^^^^^^^^^^^
app/Posts/Show.alder:59:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder fill --decl Posts.Show.update - <<'EOF'
match msg {
  AddComment p -> {
    let _ <- Db.insert Comment { post: model.post.id, author: p.form.author, body: p.form.body }
    Task.succeed (Redirect (Posts.Show { slug: model.post.slug }))
  },
}
EOF
edited app/Posts/Show.alder
--- a/app/Posts/Show.alder
+++ b/app/Posts/Show.alder
@@ -31,7 +31,10 @@
 
 update : Msg -> Model -> Session -> Task Nav
 update msg model session = match msg {
-  AddComment p -> ?addComment,
+  AddComment p -> {
+    let _ <- Db.insert Comment { post: model.post.id, author: p.form.author, body: p.form.body }
+    Task.succeed (Redirect (Posts.Show { slug: model.post.slug }))
+  },
 }
 
 view : Model -> Session -> Html Msg
app/Posts/Show.alder:62:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder new page Admin /admin --requires user --blank
wrote app/Admin.alder
app/Admin.alder:8:23: warning[W0002]: hole ?load has type Query Admin.Model
  load params session = ?load
                        ^^^^^
app/Admin.alder:11:22: warning[W0002]: hole ?view has type Html Admin.Msg
  view model session = ?view
                       ^^^^^
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
$ alder fill Admin.load - <<'EOF'
{
  let posts <- Db.from Post |> Db.orderBy Post.created.desc |> Db.all
  Query.succeed { posts: posts }
}
EOF
edited app/Admin.alder
--- a/app/Admin.alder
+++ b/app/Admin.alder
@@ -7,7 +7,10 @@
 type Model = { posts: List Post }
 
 load : Params -> Session -> Query Model
-load params session = ?load
+load params session = {
+  let posts <- Db.from Post |> Db.orderBy Post.created.desc |> Db.all
+  Query.succeed { posts: posts }
+}
 
 view : Model -> Session -> Html Msg
 view model session = ?view
app/Admin.alder:16:22: warning[W0002]: hole ?view has type Html Admin.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder fill Admin.view - <<'EOF'
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
@@ -13,7 +14,9 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = ?view
+view model session = <Layout.Page title="Admin">
+  <h1>Admin</h1>
+</Layout.Page>
 
 test "a visitor is sent to log in" = {
   let r <- Test.open (Admin {})
added import Layout to app/Admin.alder
[exit 0]
$ alder new form Admin NewPost title:Text.NonEmpty slug:Post.Slug body:Text --msg CreatePost
edited app/Admin.alder
--- a/app/Admin.alder
+++ b/app/Admin.alder
@@ -2,6 +2,7 @@
 
 import Layout
 import Post
+import User
 
 page "/admin" { } requires user
 
@@ -13,12 +14,52 @@
   Query.succeed { posts: posts }
 }
 
+form NewPost = {
+  title: Text.NonEmpty,
+  slug: Post.Slug,
+  body: Text,
+}
+
+msg CreatePost form NewPost
+
+update : Msg -> Model -> Session -> Task Nav
+update msg model session = match msg {
+  CreatePost p -> ?createPost,
+}
+
 view : Model -> Session -> Html Msg
 view model session = <Layout.Page title="Admin">
   <h1>Admin</h1>
+
+  <al-card>
+    <h2 slot="header">New post</h2>
+    <form submit={CreatePost}>
+      <al-input field="title" label="Title" />
+      <al-input field="slug" label="Slug" />
+      <al-textarea field="body" label="Body" />
+      <al-alert variant="destructive"><Errors form /></al-alert>
+      <al-button type="submit">New post</al-button>
+    </form>
+  </al-card>
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
app/Admin.alder:27:19: warning[W0002]: hole ?createPost has type Task Nav
    CreatePost p -> ?createPost,
                    ^^^^^^^^^^^
app/Admin.alder:56:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder new msg Admin TogglePublished id:Post.PostId
edited app/Admin.alder
--- a/app/Admin.alder
+++ b/app/Admin.alder
@@ -22,9 +22,12 @@
 
 msg CreatePost form NewPost
 
+msg TogglePublished { id: Post.PostId }
+
 update : Msg -> Model -> Session -> Task Nav
 update msg model session = match msg {
   CreatePost p -> ?createPost,
+  TogglePublished p -> ?togglePublished,
 }
 
 view : Model -> Session -> Html Msg
@@ -41,6 +44,8 @@
       <al-button type="submit">New post</al-button>
     </form>
   </al-card>
+
+  <al-button click={TogglePublished { id: ?togglePublishedId }}>Toggle published</al-button>
 </Layout.Page>
 
 test "a visitor is sent to log in" = {
@@ -63,3 +68,16 @@
   let r <- Test.submitRaw NewPost [{ name: "title", value: "" }, { name: "slug", value: "x" }, { name: "body", value: "x" }]
   Task.succeed (r.answer == FormRejected [{ field: "title", message: "must not be empty" }])
 }
+
+-- Fill ?expected with what pressing TogglePublished must do, so that a test fails when the arm
+-- does something else: its answer (`r.answer`: Rerendered, Redirected …) and what it
+-- changed (`after`, the row read again), as `{ answer: r.answer, … } == { answer: Rerendered, … }`.
+test "pressing TogglePublished" = {
+  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
+  let _ <- Test.login { user: Just me }
+  let post1 <- Db.insert Post { title: Text.NonEmpty "title-1", slug: Post.Slug "slug-1", body: "body-1", author: me }
+  let _ <- Test.get (Admin {})
+  let r <- Test.click (TogglePublished { id: post1 })
+  let after <- Db.get Post post1
+  Task.succeed ?expected
+}
app/Admin.alder:29:19: warning[W0002]: hole ?createPost has type Task Nav
    CreatePost p -> ?createPost,
                    ^^^^^^^^^^^
app/Admin.alder:30:24: warning[W0002]: hole ?togglePublished has type Task Nav
    TogglePublished p -> ?togglePublished,
                         ^^^^^^^^^^^^^^^^
app/Admin.alder:48:43: warning[W0002]: hole ?togglePublishedId has type Post.PostId
    <al-button click={TogglePublished { id: ?togglePublishedId }}>Toggle published</al-button>
                                            ^^^^^^^^^^^^^^^^^^
app/Admin.alder:61:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Admin.alder:82:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill --decl Admin.view - <<'EOF'
<Layout.Page title="Admin">
  <h1>Admin</h1>
  <form submit={CreatePost}>
    <Errors form />
    <Input field="title" label="Title" />
    <Errors field="title" />
    <Input field="slug" label="Slug" />
    <Errors field="slug" />
    <Textarea field="body" label="Body" />
    <Errors field="body" />
    <button>Create post</button>
  </form>
  <ul id="posts">
    {for post in model.posts ->
      <li>
        <a href={Posts.Show { slug: post.slug }}>{post.title}</a>
        {if post.published then " (published)" else " (draft)"}
        <button click={TogglePublished { id: post.id }}>{if post.published then "Unpublish" else "Publish"}</button>
      </li>}
  </ul>
  <button click={App.Logout}>Log out</button>
</Layout.Page>
EOF
edited app/Admin.alder
--- a/app/Admin.alder
+++ b/app/Admin.alder
@@ -31,22 +31,29 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = <Layout.Page title="Admin">
-  <h1>Admin</h1>
-
-  <al-card>
-    <h2 slot="header">New post</h2>
+view model session =
+  <Layout.Page title="Admin">
+    <h1>Admin</h1>
     <form submit={CreatePost}>
-      <al-input field="title" label="Title" />
-      <al-input field="slug" label="Slug" />
-      <al-textarea field="body" label="Body" />
-      <al-alert variant="destructive"><Errors form /></al-alert>
-      <al-button type="submit">New post</al-button>
+      <Errors form />
+      <Input field="title" label="Title" />
+      <Errors field="title" />
+      <Input field="slug" label="Slug" />
+      <Errors field="slug" />
+      <Textarea field="body" label="Body" />
+      <Errors field="body" />
+      <button>Create post</button>
     </form>
-  </al-card>
-
-  <al-button click={TogglePublished { id: ?togglePublishedId }}>Toggle published</al-button>
-</Layout.Page>
+    <ul id="posts">
+      {for post in model.posts ->
+        <li>
+          <a href={Posts.Show { slug: post.slug }}>{post.title}</a>
+          {if post.published then " (published)" else " (draft)"}
+          <button click={TogglePublished { id: post.id }}>{if post.published then "Unpublish" else "Publish"}</button>
+        </li>}
+    </ul>
+    <button click={App.Logout}>Log out</button>
+  </Layout.Page>
 
 test "a visitor is sent to log in" = {
   let r <- Test.open (Admin {})
app/Admin.alder:29:19: warning[W0002]: hole ?createPost has type Task Nav
    CreatePost p -> ?createPost,
                    ^^^^^^^^^^^
app/Admin.alder:30:24: warning[W0002]: hole ?togglePublished has type Task Nav
    TogglePublished p -> ?togglePublished,
                         ^^^^^^^^^^^^^^^^
app/Admin.alder:68:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Admin.alder:89:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill --decl Admin.update - <<'EOF'
match msg {
  CreatePost p -> {
    let taken <- Db.from Post |> Db.where (Post.slug.eq p.form.slug) |> Db.count
    if taken > 0 then Task.succeed (Reject "That slug is already taken.") else {
      let _ <- Db.insert Post { title: p.form.title, slug: p.form.slug, body: p.form.body, author: session.user }
      Task.succeed (Redirect (Admin {}))
    }
  },
  TogglePublished p -> {
    let _ <- Db.update Post p.id (\x -> { x | published: not x.published })
    Task.succeed Rerender
  },
}
EOF
edited app/Admin.alder
--- a/app/Admin.alder
+++ b/app/Admin.alder
@@ -26,8 +26,17 @@
 
 update : Msg -> Model -> Session -> Task Nav
 update msg model session = match msg {
-  CreatePost p -> ?createPost,
-  TogglePublished p -> ?togglePublished,
+  CreatePost p -> {
+    let taken <- Db.from Post |> Db.where (Post.slug.eq p.form.slug) |> Db.count
+    if taken > 0 then Task.succeed (Reject "That slug is already taken.") else {
+      let _ <- Db.insert Post { title: p.form.title, slug: p.form.slug, body: p.form.body, author: session.user }
+      Task.succeed (Redirect (Admin {}))
+    }
+  },
+  TogglePublished p -> {
+    let _ <- Db.update Post p.id (\x -> { x | published: not x.published })
+    Task.succeed Rerender
+  },
 }
 
 view : Model -> Session -> Html Msg
app/Admin.alder:77:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Admin.alder:98:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder new test Home "/ lists only published posts, newest first"
edited app/Home.alder
--- a/app/Home.alder
+++ b/app/Home.alder
@@ -20,3 +20,7 @@
     {for post in model.posts -> <li><a href={Posts.Show { slug: post.slug }}>{post.title}</a></li>}
   </ul>
 </Layout.Page>
+
+test "/ lists only published posts, newest first" = {
+  ?test
+}
app/Home.alder:25:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Home."/ lists only published posts, newest first"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Db.insert Post { title: Text.NonEmpty "Old", slug: Post.Slug "old", body: "a", published: True, author: me }
  let _ <- Db.insert Post { title: Text.NonEmpty "Draft", slug: Post.Slug "draft", body: "b", published: False, author: me }
  let _ <- Db.insert Post { title: Text.NonEmpty "New", slug: Post.Slug "new", body: "c", published: True, author: me }
  let page <- Test.get (Home {})
  Task.succeed (Test.texts "#posts a" page.doc == ["New", "Old"])
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
 
@@ -22,5 +23,10 @@
 </Layout.Page>
 
 test "/ lists only published posts, newest first" = {
-  ?test
+  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
+  let _ <- Db.insert Post { title: Text.NonEmpty "Old", slug: Post.Slug "old", body: "a", published: True, author: me }
+  let _ <- Db.insert Post { title: Text.NonEmpty "Draft", slug: Post.Slug "draft", body: "b", published: False, author: me }
+  let _ <- Db.insert Post { title: Text.NonEmpty "New", slug: Post.Slug "new", body: "c", published: True, author: me }
+  let page <- Test.get (Home {})
+  Task.succeed (Test.texts "#posts a" page.doc == ["New", "Old"])
 }
added import User to app/Home.alder
[exit 0]
$ alder new test Posts.Show "an unpublished post is not found for a reader"
edited app/Posts/Show.alder
--- a/app/Posts/Show.alder
+++ b/app/Posts/Show.alder
@@ -67,3 +67,7 @@
   let r <- Test.submitRaw NewComment [{ name: "author", value: "" }, { name: "body", value: "x" }]
   Task.succeed (r.answer == FormRejected [{ field: "author", message: "must not be empty" }])
 }
+
+test "an unpublished post is not found for a reader" = {
+  ?test
+}
app/Posts/Show.alder:62:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Posts/Show.alder:72:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Posts.Show."an unpublished post is not found for a reader"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Db.insert Post { title: Text.NonEmpty "Draft", slug: Post.Slug "draft", body: "b", published: False, author: me }
  let hidden <- Test.open (Posts.Show { slug: Post.Slug "draft" })
  let _ <- Test.login { user: Just me }
  let mine <- Test.open (Posts.Show { slug: Post.Slug "draft" })
  Task.succeed ({ reader: hidden.answer, author: mine.answer } == { reader: PageMissing, author: Shown })
}
EOF
edited app/Posts/Show.alder
--- a/app/Posts/Show.alder
+++ b/app/Posts/Show.alder
@@ -3,6 +3,7 @@
 import Comment
 import Layout
 import Post
+import User
 
 page "/posts/{slug}" { slug: Post.Slug }
 
@@ -69,5 +70,10 @@
 }
 
 test "an unpublished post is not found for a reader" = {
-  ?test
+  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
+  let _ <- Db.insert Post { title: Text.NonEmpty "Draft", slug: Post.Slug "draft", body: "b", published: False, author: me }
+  let hidden <- Test.open (Posts.Show { slug: Post.Slug "draft" })
+  let _ <- Test.login { user: Just me }
+  let mine <- Test.open (Posts.Show { slug: Post.Slug "draft" })
+  Task.succeed ({ reader: hidden.answer, author: mine.answer } == { reader: PageMissing, author: Shown })
 }
app/Posts/Show.alder:63:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
added import User to app/Posts/Show.alder
[exit 0]
$ alder new test Posts.Show "a comment with an empty author is refused"
edited app/Posts/Show.alder
--- a/app/Posts/Show.alder
+++ b/app/Posts/Show.alder
@@ -77,3 +77,7 @@
   let mine <- Test.open (Posts.Show { slug: Post.Slug "draft" })
   Task.succeed ({ reader: hidden.answer, author: mine.answer } == { reader: PageMissing, author: Shown })
 }
+
+test "a comment with an empty author is refused" = {
+  ?test
+}
app/Posts/Show.alder:63:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Posts/Show.alder:82:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Posts.Show."a comment with an empty author is refused"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Db.insert Post { title: Text.NonEmpty "Hello", slug: Post.Slug "hello", body: "Hi", published: True, author: me }
  let _ <- Test.get (Posts.Show { slug: Post.Slug "hello" })
  let r <- Test.submitRaw NewComment [{ name: "author", value: "" }, { name: "body", value: "Nice" }]
  let n <- Db.from Comment |> Db.count
  Task.succeed ({ answer: r.answer, comments: n } == { answer: FormRejected [{ field: "author", message: "must not be empty" }], comments: 0 })
}
EOF
edited app/Posts/Show.alder
--- a/app/Posts/Show.alder
+++ b/app/Posts/Show.alder
@@ -79,5 +79,10 @@
 }
 
 test "a comment with an empty author is refused" = {
-  ?test
+  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
+  let _ <- Db.insert Post { title: Text.NonEmpty "Hello", slug: Post.Slug "hello", body: "Hi", published: True, author: me }
+  let _ <- Test.get (Posts.Show { slug: Post.Slug "hello" })
+  let r <- Test.submitRaw NewComment [{ name: "author", value: "" }, { name: "body", value: "Nice" }]
+  let n <- Db.from Comment |> Db.count
+  Task.succeed ({ answer: r.answer, comments: n } == { answer: FormRejected [{ field: "author", message: "must not be empty" }], comments: 0 })
 }
app/Posts/Show.alder:63:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder new test Admin "toggling publishes a post"
edited app/Admin.alder
--- a/app/Admin.alder
+++ b/app/Admin.alder
@@ -97,3 +97,7 @@
   let after <- Db.get Post post1
   Task.succeed ?expected
 }
+
+test "toggling publishes a post" = {
+  ?test
+}
app/Admin.alder:77:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Admin.alder:98:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Admin.alder:102:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Admin."toggling publishes a post"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Post { title: Text.NonEmpty "Draft", slug: Post.Slug "draft", body: "b", published: False, author: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Admin {})
  let r <- Test.click (TogglePublished { id: id })
  let after <- Db.get Post id
  Task.succeed ({ answer: r.answer, published: Maybe.map (\x -> x.published) after } == { answer: Rerendered, published: Just True })
}
EOF
edited app/Admin.alder
--- a/app/Admin.alder
+++ b/app/Admin.alder
@@ -99,5 +99,11 @@
 }
 
 test "toggling publishes a post" = {
-  ?test
+  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
+  let id <- Db.insert Post { title: Text.NonEmpty "Draft", slug: Post.Slug "draft", body: "b", published: False, author: me }
+  let _ <- Test.login { user: Just me }
+  let _ <- Test.get (Admin {})
+  let r <- Test.click (TogglePublished { id: id })
+  let after <- Db.get Post id
+  Task.succeed ({ answer: r.answer, published: Maybe.map (\x -> x.published) after } == { answer: Rerendered, published: Just True })
 }
app/Admin.alder:77:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Admin.alder:98:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill --list
Admin "submitting NewPost" ?expected : Answer
Admin "pressing TogglePublished" ?expected : Bool
Posts.Show "submitting NewComment" ?expected : Answer
[exit 0]
$ alder check
app/Admin.alder:77:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Admin.alder:98:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Posts/Show.alder:63:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
alder check: 3 typed holes left
[exit 0]
```

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Admin.alder:77:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Admin.alder:98:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Posts/Show.alder:63:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
alder build: the program has 3 typed hole(s); fill them before building
```
