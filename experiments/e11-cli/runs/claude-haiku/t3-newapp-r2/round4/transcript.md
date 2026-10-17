### Round 4

```text
$ alder fill --decl Auth.Login.update - <<'EOF'
match msg {
  Submit p -> {
    let found <- Password.verify { email: p.form.email, password: p.form.password }
    match found {
      Just id -> {
        let _ <- Session.put (\s -> { s | user: Just id })
        Task.succeed (Redirect (Home {}))
      },
      Nothing -> Task.succeed (Reject "Email or password is incorrect."),
    }
  },
}
EOF
[exit 0]
$ alder remove page Admin
removed app/Admin.alder
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
$ alder fill Admin.load - --hole ?load <<'EOF'
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
$ alder fill Admin.view - --hole ?view <<'EOF'
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
$ alder new msg Admin Toggle id:Post.PostId
edited app/Admin.alder
--- a/app/Admin.alder
+++ b/app/Admin.alder
@@ -2,6 +2,7 @@
 
 import Layout
 import Post
+import User
 
 page "/admin" { } requires user
 
@@ -13,12 +14,34 @@
   Query.succeed { posts: posts }
 }
 
+msg Toggle { id: Post.PostId }
+
+update : Msg -> Model -> Session -> Task Nav
+update msg model session = match msg {
+  Toggle p -> ?toggle,
+}
+
 view : Model -> Session -> Html Msg
 view model session = <Layout.Page title="Admin">
   <h1>Admin</h1>
+
+  <al-button click={Toggle { id: ?toggleId }}>Toggle</al-button>
 </Layout.Page>
 
 test "a visitor is sent to log in" = {
   let r <- Test.open (Admin {})
   Task.succeed (r.answer == Redirected (Auth.Login {}))
 }
+
+-- Fill ?expected with what pressing Toggle must do, so that a test fails when the arm
+-- does something else: its answer (`r.answer`: Rerendered, Redirected …) and what it
+-- changed (`after`, the row read again), as `{ answer: r.answer, … } == { answer: Rerendered, … }`.
+test "pressing Toggle" = {
+  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
+  let _ <- Test.login { user: Just me }
+  let post1 <- Db.insert Post { slug: Post.Slug "slug-1", title: Text.NonEmpty "title-1", body: "body-1", author: me }
+  let _ <- Test.get (Admin {})
+  let r <- Test.click (Toggle { id: post1 })
+  let after <- Db.get Post post1
+  Task.succeed ?expected
+}
app/Admin.alder:21:15: warning[W0002]: hole ?toggle has type Task Nav
    Toggle p -> ?toggle,
                ^^^^^^^
app/Admin.alder:28:34: warning[W0002]: hole ?toggleId has type Post.PostId
    <al-button click={Toggle { id: ?toggleId }}>Toggle</al-button>
                                   ^^^^^^^^^
app/Admin.alder:46:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder new form Admin NewPost slug:Post.Slug title:Text.NonEmpty body:Text published:Bool --msg CreatePost
edited app/Admin.alder
--- a/app/Admin.alder
+++ b/app/Admin.alder
@@ -16,9 +16,19 @@
 
 msg Toggle { id: Post.PostId }
 
+form NewPost = {
+  slug: Post.Slug,
+  title: Text.NonEmpty,
+  body: Text,
+  published: Bool,
+}
+
+msg CreatePost form NewPost
+
 update : Msg -> Model -> Session -> Task Nav
 update msg model session = match msg {
   Toggle p -> ?toggle,
+  CreatePost p -> ?createPost,
 }
 
 view : Model -> Session -> Html Msg
@@ -26,6 +36,18 @@
   <h1>Admin</h1>
 
   <al-button click={Toggle { id: ?toggleId }}>Toggle</al-button>
+
+  <al-card>
+    <h2 slot="header">New post</h2>
+    <form submit={CreatePost}>
+      <al-input field="slug" label="Slug" />
+      <al-input field="title" label="Title" />
+      <al-textarea field="body" label="Body" />
+      <al-checkbox field="published" label="Published" />
+      <al-alert variant="destructive"><Errors form /></al-alert>
+      <al-button type="submit">New post</al-button>
+    </form>
+  </al-card>
 </Layout.Page>
 
 test "a visitor is sent to log in" = {
@@ -45,3 +67,19 @@
   let after <- Db.get Post post1
   Task.succeed ?expected
 }
+
+test "submitting NewPost" = {
+  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
+  let _ <- Test.login { user: Just me }
+  let _ <- Test.get (Admin {})
+  let r <- Test.submit NewPost { slug: Post.Slug "slug-1", title: Text.NonEmpty "title-1", body: "body-1", published: True }
+  Task.succeed (r.answer == ?expected)
+}
+
+test "NewPost refuses an empty title" = {
+  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
+  let _ <- Test.login { user: Just me }
+  let _ <- Test.get (Admin {})
+  let r <- Test.submitRaw NewPost [{ name: "slug", value: "x" }, { name: "title", value: "" }, { name: "body", value: "x" }, { name: "published", value: "True" }]
+  Task.succeed (r.answer == FormRejected [{ field: "title", message: "must not be empty" }])
+}
app/Admin.alder:30:15: warning[W0002]: hole ?toggle has type Task Nav
    Toggle p -> ?toggle,
                ^^^^^^^
app/Admin.alder:31:19: warning[W0002]: hole ?createPost has type Task Nav
    CreatePost p -> ?createPost,
                    ^^^^^^^^^^^
app/Admin.alder:38:34: warning[W0002]: hole ?toggleId has type Post.PostId
    <al-button click={Toggle { id: ?toggleId }}>Toggle</al-button>
                                   ^^^^^^^^^
app/Admin.alder:68:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Admin.alder:76:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder fill --decl Admin.view - <<'EOF'
<Layout.Page title="Admin">
  <h1>Admin</h1>
  <al-button variant="outline" click={App.Logout}>Log out</al-button>
  <table>
    <tr><th>Title</th><th>Published</th><th>Action</th></tr>
    {for post in model.posts ->
      <tr>
        <td><a href={Posts.Show { slug: post.slug }}>{post.title}</a></td>
        <td>{if post.published then "yes" else "no"}</td>
        <td><al-button variant="secondary" size="sm" click={Toggle { id: post.id }}>Toggle published</al-button></td>
      </tr>}
  </table>
  <al-card>
    <form submit={CreatePost}>
      <al-input field="slug" label="Slug" />
      <al-input field="title" label="Title" />
      <al-textarea field="body" label="Body" />
      <al-checkbox field="published" label="Published" />
      <al-button type="submit">Create post</al-button>
    </form>
  </al-card>
</Layout.Page>
EOF
edited app/Admin.alder
--- a/app/Admin.alder
+++ b/app/Admin.alder
@@ -32,23 +32,29 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = <Layout.Page title="Admin">
-  <h1>Admin</h1>
-
-  <al-button click={Toggle { id: ?toggleId }}>Toggle</al-button>
-
-  <al-card>
-    <h2 slot="header">New post</h2>
-    <form submit={CreatePost}>
-      <al-input field="slug" label="Slug" />
-      <al-input field="title" label="Title" />
-      <al-textarea field="body" label="Body" />
-      <al-checkbox field="published" label="Published" />
-      <al-alert variant="destructive"><Errors form /></al-alert>
-      <al-button type="submit">New post</al-button>
-    </form>
-  </al-card>
-</Layout.Page>
+view model session =
+  <Layout.Page title="Admin">
+    <h1>Admin</h1>
+    <al-button variant="outline" click={App.Logout}>Log out</al-button>
+    <table>
+      <tr><th>Title</th><th>Published</th><th>Action</th></tr>
+      {for post in model.posts ->
+        <tr>
+          <td><a href={Posts.Show { slug: post.slug }}>{post.title}</a></td>
+          <td>{if post.published then "yes" else "no"}</td>
+          <td><al-button variant="secondary" size="sm" click={Toggle { id: post.id }}>Toggle published</al-button></td>
+        </tr>}
+    </table>
+    <al-card>
+      <form submit={CreatePost}>
+        <al-input field="slug" label="Slug" />
+        <al-input field="title" label="Title" />
+        <al-textarea field="body" label="Body" />
+        <al-checkbox field="published" label="Published" />
+        <al-button type="submit">Create post</al-button>
+      </form>
+    </al-card>
+  </Layout.Page>
 
 test "a visitor is sent to log in" = {
   let r <- Test.open (Admin {})
app/Admin.alder:30:15: warning[W0002]: hole ?toggle has type Task Nav
    Toggle p -> ?toggle,
                ^^^^^^^
app/Admin.alder:31:19: warning[W0002]: hole ?createPost has type Task Nav
    CreatePost p -> ?createPost,
                    ^^^^^^^^^^^
app/Admin.alder:74:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Admin.alder:82:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder fill --decl Admin.update - <<'EOF'
match msg {
  CreatePost p -> {
    let _ <- Db.insert Post { slug: p.form.slug, title: p.form.title, body: p.form.body, published: p.form.published, author: session.user }
    Task.succeed (Redirect (Admin {}))
  },
  Toggle p -> {
    let _ <- Db.update Post p.id (\x -> { x | published: not x.published })
    Task.succeed Rerender
  },
}
EOF
edited app/Admin.alder
--- a/app/Admin.alder
+++ b/app/Admin.alder
@@ -27,8 +27,14 @@
 
 update : Msg -> Model -> Session -> Task Nav
 update msg model session = match msg {
-  Toggle p -> ?toggle,
-  CreatePost p -> ?createPost,
+  CreatePost p -> {
+    let _ <- Db.insert Post { slug: p.form.slug, title: p.form.title, body: p.form.body, published: p.form.published, author: session.user }
+    Task.succeed (Redirect (Admin {}))
+  },
+  Toggle p -> {
+    let _ <- Db.update Post p.id (\x -> { x | published: not x.published })
+    Task.succeed Rerender
+  },
 }
 
 view : Model -> Session -> Html Msg
app/Admin.alder:80:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Admin.alder:88:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder new test Admin "Toggle publishes a draft"
edited app/Admin.alder
--- a/app/Admin.alder
+++ b/app/Admin.alder
@@ -95,3 +95,7 @@
   let r <- Test.submitRaw NewPost [{ name: "slug", value: "x" }, { name: "title", value: "" }, { name: "body", value: "x" }, { name: "published", value: "True" }]
   Task.succeed (r.answer == FormRejected [{ field: "title", message: "must not be empty" }])
 }
+
+test "Toggle publishes a draft" = {
+  ?test
+}
app/Admin.alder:80:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Admin.alder:88:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Admin.alder:100:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Admin."Toggle publishes a draft"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Post { slug: Slug "draft", title: Text.NonEmpty "Draft", body: "b", author: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Admin {})
  let _ <- Test.click (Toggle { id: id })
  let after <- Db.get Post id
  Task.succeed (Maybe.map (\p -> p.published) after == Just True)
}
EOF
edited app/Admin.alder
--- a/app/Admin.alder
+++ b/app/Admin.alder
@@ -97,5 +97,11 @@
 }
 
 test "Toggle publishes a draft" = {
-  ?test
+  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
+  let id <- Db.insert Post { slug: Slug "draft", title: Text.NonEmpty "Draft", body: "b", author: me }
+  let _ <- Test.login { user: Just me }
+  let _ <- Test.get (Admin {})
+  let _ <- Test.click (Toggle { id: id })
+  let after <- Db.get Post id
+  Task.succeed (Maybe.map (\p -> p.published) after == Just True)
 }
app/Admin.alder:80:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Admin.alder:88:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
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
$ alder fill Home.load - --hole ?load <<'EOF'
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
$ alder fill Home.view - --hole ?view <<'EOF'
<Layout.Page title="Blog">
  <h1>Blog</h1>
  <ul>
    {for post in model.posts ->
      <li><a href={Posts.Show { slug: post.slug }}>{post.title}</a></li>}
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
+view model session = <Layout.Page title="Blog">
+  <h1>Blog</h1>
+  <ul>
+    {for post in model.posts ->
+      <li><a href={Posts.Show { slug: post.slug }}>{post.title}</a></li>}
+  </ul>
+</Layout.Page>
added import Layout to app/Home.alder
[exit 0]
$ alder new test Home "the front page lists published posts, newest first"
edited app/Home.alder
--- a/app/Home.alder
+++ b/app/Home.alder
@@ -25,3 +25,7 @@
       <li><a href={Posts.Show { slug: post.slug }}>{post.title}</a></li>}
   </ul>
 </Layout.Page>
+
+test "the front page lists published posts, newest first" = {
+  ?test
+}
app/Home.alder:30:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Home."the front page lists published posts, newest first"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Db.insert Post { slug: Slug "old", title: Text.NonEmpty "Old", body: "a", published: True, author: me }
  let _ <- Db.insert Post { slug: Slug "new", title: Text.NonEmpty "New", body: "b", published: True, author: me }
  let _ <- Db.insert Post { slug: Slug "draft", title: Text.NonEmpty "Draft", body: "c", published: False, author: me }
  let page <- Test.get (Home {})
  Task.succeed (Test.texts "li a" page.doc == ["New", "Old"])
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
 
 test "the front page lists published posts, newest first" = {
-  ?test
+  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
+  let _ <- Db.insert Post { slug: Slug "old", title: Text.NonEmpty "Old", body: "a", published: True, author: me }
+  let _ <- Db.insert Post { slug: Slug "new", title: Text.NonEmpty "New", body: "b", published: True, author: me }
+  let _ <- Db.insert Post { slug: Slug "draft", title: Text.NonEmpty "Draft", body: "c", published: False, author: me }
+  let page <- Test.get (Home {})
+  Task.succeed (Test.texts "li a" page.doc == ["New", "Old"])
 }
added import User to app/Home.alder
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
$ alder fill Posts.Show.load - --hole ?load <<'EOF'
{
  let found <- Db.from Post |> Db.where (Post.slug.eq params.slug) |> Db.first
  match found {
    Nothing -> Query.notFound,
    Just post -> {
      if post.published || session.user == Just post.author then {
        let comments <-
          Db.from Comment
            |> Db.where (Comment.post.eq post.id)
            |> Db.orderBy Comment.created.asc
            |> Db.all
        Query.succeed { post: post, comments: comments }
      } else Query.notFound,
    },
  }
}
EOF
alder fill: `alder fill` takes one expression; standard input does not parse as one: expected `}`, got `,`; the last expression of a block is its result and takes no `,` (line 13, column 28)
[exit 1]
```

The harness stopped at line 124 of your script: `alder fill Posts.Show.load - --hole ?load <<'EOF'`: it exited 1. The commands after it were not run.

Your script stopped at command 19; the app is as the commands before it left it; continue.

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Admin.alder:80:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Admin.alder:88:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Posts/Show.alder:11:23: warning[W0002]: hole ?load has type Query Posts.Show.Model
  load params session = ?load
                        ^^^^^
app/Posts/Show.alder:14:22: warning[W0002]: hole ?view has type Html Posts.Show.Msg
  view model session = ?view
                       ^^^^^
alder build: the program has 4 typed hole(s); fill them before building
```
