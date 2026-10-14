### Round 2

```text
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
    {for p in model.posts -> <li><a href={Posts.Show { slug: p.slug }}>{p.title}</a></li>}
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
+    {for p in model.posts -> <li><a href={Posts.Show { slug: p.slug }}>{p.title}</a></li>}
+  </ul>
+</Layout.Page>
added import Layout to app/Home.alder
[exit 0]
$ alder new test Home "lists only published posts, newest first"
edited app/Home.alder
--- a/app/Home.alder
+++ b/app/Home.alder
@@ -20,3 +20,7 @@
     {for p in model.posts -> <li><a href={Posts.Show { slug: p.slug }}>{p.title}</a></li>}
   </ul>
 </Layout.Page>
+
+test "lists only published posts, newest first" = {
+  ?test
+}
app/Home.alder:25:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Home."lists only published posts, newest first"' - <<'EOF'
{
  let _ <- Db.insert Post { title: Text.NonEmpty "First", slug: Slug.Slug "first", body: "a", published: True }
  let _ <- Db.insert Post { title: Text.NonEmpty "Draft", slug: Slug.Slug "draft", body: "b", published: False }
  let _ <- Db.insert Post { title: Text.NonEmpty "Second", slug: Slug.Slug "second", body: "c", published: True }
  let page <- Test.get (Home {})
  Task.succeed (Test.texts "#posts a" page.doc == ["Second", "First"])
}
EOF
edited app/Home.alder
--- a/app/Home.alder
+++ b/app/Home.alder
@@ -2,6 +2,7 @@
 
 import Layout
 import Post
+import Slug
 
 page "/" { }
 
@@ -22,5 +23,9 @@
 </Layout.Page>
 
 test "lists only published posts, newest first" = {
-  ?test
+  let _ <- Db.insert Post { title: Text.NonEmpty "First", slug: Slug.Slug "first", body: "a", published: True }
+  let _ <- Db.insert Post { title: Text.NonEmpty "Draft", slug: Slug.Slug "draft", body: "b", published: False }
+  let _ <- Db.insert Post { title: Text.NonEmpty "Second", slug: Slug.Slug "second", body: "c", published: True }
+  let page <- Test.get (Home {})
+  Task.succeed (Test.texts "#posts a" page.doc == ["Second", "First"])
 }
added import Slug to app/Home.alder
[exit 0]
$ alder fill --decl Posts.Show.Model - <<'EOF'
{ post: Post, comments: List Comment }
EOF
edited app/Posts/Show.alder
--- a/app/Posts/Show.alder
+++ b/app/Posts/Show.alder
@@ -1,10 +1,12 @@
 module Posts.Show
 
+import Comment
+import Post
 import Slug
 
 page "/posts/{slug}" { slug: Slug.Slug }
 
-type Model = { }
+type Model = { post: Post, comments: List Comment }
 
 load : Params -> Session -> Query Model
 load params session = ?load
app/Posts/Show.alder:12:23: warning[W0002]: hole ?load has type Query Posts.Show.Model
  load params session = ?load
                        ^^^^^
app/Posts/Show.alder:15:22: warning[W0002]: hole ?view has type Html Posts.Show.Msg
  view model session = ?view
                       ^^^^^
added import Post to app/Posts/Show.alder
added import Comment to app/Posts/Show.alder
[exit 0]
$ alder fill Posts.Show.load - <<'EOF'
{
  let found <- Db.from Post |> Db.where (Post.slug.eq params.slug) |> Db.first
  match found {
    Just post -> if post.published || session.user != Nothing then {
      let comments <- Db.from Comment |> Db.where (Comment.post.eq post.id) |> Db.orderBy Comment.created.asc |> Db.all
      Query.succeed { post: post, comments: comments }
    } else Query.notFound,
    Nothing -> Query.notFound,
  }
}
EOF
edited app/Posts/Show.alder
--- a/app/Posts/Show.alder
+++ b/app/Posts/Show.alder
@@ -9,7 +9,16 @@
 type Model = { post: Post, comments: List Comment }
 
 load : Params -> Session -> Query Model
-load params session = ?load
+load params session = {
+  let found <- Db.from Post |> Db.where (Post.slug.eq params.slug) |> Db.first
+  match found {
+    Just post -> if post.published || session.user != Nothing then {
+      let comments <- Db.from Comment |> Db.where (Comment.post.eq post.id) |> Db.orderBy Comment.created.asc |> Db.all
+      Query.succeed { post: post, comments: comments }
+    } else Query.notFound,
+    Nothing -> Query.notFound,
+  }
+}
 
 view : Model -> Session -> Html Msg
 view model session = ?view
app/Posts/Show.alder:24:22: warning[W0002]: hole ?view has type Html Posts.Show.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder fill Posts.Show.view - <<'EOF'
<Layout.Page title="Post">
  <h1>Post</h1>
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
 import Slug
 
@@ -21,4 +22,6 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = ?view
+view model session = <Layout.Page title="Post">
+  <h1>Post</h1>
+</Layout.Page>
added import Layout to app/Posts/Show.alder
[exit 0]
$ alder new form Posts.Show NewComment author:Text.NonEmpty body:Text.NonEmpty --msg AddComment
edited app/Posts/Show.alder
--- a/app/Posts/Show.alder
+++ b/app/Posts/Show.alder
@@ -21,7 +21,41 @@
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
 view model session = <Layout.Page title="Post">
   <h1>Post</h1>
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
+  let _ <- Test.get (Posts.Show { slug: Slug.Slug "slug-1" })
+  let r <- Test.submit NewComment { author: Text.NonEmpty "author-1", body: Text.NonEmpty "body-1" }
+  Task.succeed (r.answer == ?expected)
+}
+
+test "NewComment refuses an empty author" = {
+  let _ <- Test.get (Posts.Show { slug: Slug.Slug "slug-1" })
+  let r <- Test.submitRaw NewComment [{ name: "author", value: "" }, { name: "body", value: "x" }]
+  Task.succeed (r.answer == FormRejected [{ field: "author", message: "must not be empty" }])
+}
app/Posts/Show.alder:33:19: warning[W0002]: hole ?addComment has type Task Nav
    AddComment p -> ?addComment,
                    ^^^^^^^^^^^
app/Posts/Show.alder:54:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder fill --decl Posts.Show.view - <<'EOF'
<Layout.Page title={model.post.title}>
  <h1>{model.post.title}</h1>
  <article>{model.post.body}</article>
  <h2>Comments</h2>
  <ul id="comments">
    {for c in model.comments -> <li><strong>{c.author}</strong>: {c.body}</li>}
  </ul>
  <al-card>
    <form submit={AddComment}>
      <al-input field="author" label="Name" />
      <al-input field="body" label="Comment" />
      <al-button type="submit">Add comment</al-button>
    </form>
  </al-card>
</Layout.Page>
EOF
edited app/Posts/Show.alder
--- a/app/Posts/Show.alder
+++ b/app/Posts/Show.alder
@@ -34,19 +34,22 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = <Layout.Page title="Post">
-  <h1>Post</h1>
-
-  <al-card>
-    <h2 slot="header">New comment</h2>
-    <form submit={AddComment}>
-      <al-input field="author" label="Author" />
-      <al-textarea field="body" label="Body" />
-      <al-alert variant="destructive"><Errors form /></al-alert>
-      <al-button type="submit">New comment</al-button>
-    </form>
-  </al-card>
-</Layout.Page>
+view model session =
+  <Layout.Page title={model.post.title}>
+    <h1>{model.post.title}</h1>
+    <article>{model.post.body}</article>
+    <h2>Comments</h2>
+    <ul id="comments">
+      {for c in model.comments -> <li><strong>{c.author}</strong>: {c.body}</li>}
+    </ul>
+    <al-card>
+      <form submit={AddComment}>
+        <al-input field="author" label="Name" />
+        <al-input field="body" label="Comment" />
+        <al-button type="submit">Add comment</al-button>
+      </form>
+    </al-card>
+  </Layout.Page>
 
 test "submitting NewComment" = {
   let _ <- Test.get (Posts.Show { slug: Slug.Slug "slug-1" })
app/Posts/Show.alder:33:19: warning[W0002]: hole ?addComment has type Task Nav
    AddComment p -> ?addComment,
                    ^^^^^^^^^^^
app/Posts/Show.alder:57:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder fill Posts.Show.update.AddComment - <<'EOF'
{
  let _ <- Db.insert Comment { post: model.post.id, author: p.form.author, body: p.form.body }
  Task.succeed Rerender
}
EOF
edited app/Posts/Show.alder
--- a/app/Posts/Show.alder
+++ b/app/Posts/Show.alder
@@ -30,7 +30,10 @@
 
 update : Msg -> Model -> Session -> Task Nav
 update msg model session = match msg {
-  AddComment p -> ?addComment,
+  AddComment p -> {
+    let _ <- Db.insert Comment { post: model.post.id, author: p.form.author, body: p.form.body }
+    Task.succeed Rerender
+  },
 }
 
 view : Model -> Session -> Html Msg
app/Posts/Show.alder:60:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder new test Posts.Show "an unpublished post is not found for a reader"
edited app/Posts/Show.alder
--- a/app/Posts/Show.alder
+++ b/app/Posts/Show.alder
@@ -65,3 +65,7 @@
   let r <- Test.submitRaw NewComment [{ name: "author", value: "" }, { name: "body", value: "x" }]
   Task.succeed (r.answer == FormRejected [{ field: "author", message: "must not be empty" }])
 }
+
+test "an unpublished post is not found for a reader" = {
+  ?test
+}
app/Posts/Show.alder:60:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Posts/Show.alder:70:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Posts.Show."an unpublished post is not found for a reader"' - <<'EOF'
{
  let _ <- Db.insert Post { title: Text.NonEmpty "Draft", slug: Slug.Slug "draft", body: "b", published: False }
  let r <- Test.open (Posts.Show { slug: Slug.Slug "draft" })
  Task.succeed (r.answer == PageMissing)
}
EOF
edited app/Posts/Show.alder
--- a/app/Posts/Show.alder
+++ b/app/Posts/Show.alder
@@ -67,5 +67,7 @@
 }
 
 test "an unpublished post is not found for a reader" = {
-  ?test
+  let _ <- Db.insert Post { title: Text.NonEmpty "Draft", slug: Slug.Slug "draft", body: "b", published: False }
+  let r <- Test.open (Posts.Show { slug: Slug.Slug "draft" })
+  Task.succeed (r.answer == PageMissing)
 }
app/Posts/Show.alder:60:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder new test Posts.Show "a comment with an empty author is refused"
edited app/Posts/Show.alder
--- a/app/Posts/Show.alder
+++ b/app/Posts/Show.alder
@@ -71,3 +71,7 @@
   let r <- Test.open (Posts.Show { slug: Slug.Slug "draft" })
   Task.succeed (r.answer == PageMissing)
 }
+
+test "a comment with an empty author is refused" = {
+  ?test
+}
app/Posts/Show.alder:60:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Posts/Show.alder:76:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Posts.Show."a comment with an empty author is refused"' - <<'EOF'
{
  let _ <- Db.insert Post { title: Text.NonEmpty "Hello", slug: Slug.Slug "hello", body: "b", published: True }
  let _ <- Test.get (Posts.Show { slug: Slug.Slug "hello" })
  let r <- Test.submitRaw NewComment [{ name: "author", value: "" }, { name: "body", value: "hi" }]
  let n <- Db.from Comment |> Db.count
  Task.succeed ({ answer: r.answer, n: n } == { answer: FormRejected [{ field: "author", message: "must not be empty" }], n: 0 })
}
EOF
edited app/Posts/Show.alder
--- a/app/Posts/Show.alder
+++ b/app/Posts/Show.alder
@@ -73,5 +73,9 @@
 }
 
 test "a comment with an empty author is refused" = {
-  ?test
+  let _ <- Db.insert Post { title: Text.NonEmpty "Hello", slug: Slug.Slug "hello", body: "b", published: True }
+  let _ <- Test.get (Posts.Show { slug: Slug.Slug "hello" })
+  let r <- Test.submitRaw NewComment [{ name: "author", value: "" }, { name: "body", value: "hi" }]
+  let n <- Db.from Comment |> Db.count
+  Task.succeed ({ answer: r.answer, n: n } == { answer: FormRejected [{ field: "author", message: "must not be empty" }], n: 0 })
 }
app/Posts/Show.alder:60:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder fill --decl Desk.Model - <<'EOF'
{ posts: List Post }
EOF
edited app/Desk.alder
--- a/app/Desk.alder
+++ b/app/Desk.alder
@@ -1,8 +1,10 @@
 module Desk
 
+import Post
+
 page "/admin" { } requires user
 
-type Model = { }
+type Model = { posts: List Post }
 
 load : Params -> Session -> Query Model
 load params session = ?load
app/Desk.alder:10:23: warning[W0002]: hole ?load has type Query Desk.Model
  load params session = ?load
                        ^^^^^
app/Desk.alder:13:22: warning[W0002]: hole ?view has type Html Desk.Msg
  view model session = ?view
                       ^^^^^
added import Post to app/Desk.alder
[exit 0]
$ alder fill Desk.load - <<'EOF'
{
  let posts <- Db.from Post |> Db.orderBy Post.created.desc |> Db.all
  Query.succeed { posts: posts }
}
EOF
edited app/Desk.alder
--- a/app/Desk.alder
+++ b/app/Desk.alder
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
app/Desk.alder:16:22: warning[W0002]: hole ?view has type Html Desk.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder fill Desk.view - <<'EOF'
<Layout.Page title="Admin">
  <h1>Admin</h1>
</Layout.Page>
EOF
edited app/Desk.alder
--- a/app/Desk.alder
+++ b/app/Desk.alder
@@ -1,5 +1,6 @@
 module Desk
 
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
   let r <- Test.open (Desk {})
added import Layout to app/Desk.alder
[exit 0]
$ alder new form Desk NewPost title:Text.NonEmpty slug:Slug.Slug body:Text published:Bool --msg CreatePost
edited app/Desk.alder
--- a/app/Desk.alder
+++ b/app/Desk.alder
@@ -2,6 +2,8 @@
 
 import Layout
 import Post
+import Slug
+import User
 
 page "/admin" { } requires user
 
@@ -13,12 +15,59 @@
   Query.succeed { posts: posts }
 }
 
+form NewPost = {
+  title: Text.NonEmpty,
+  slug: Slug.Slug,
+  body: Text,
+  published: Bool,
+}
+
+msg CreatePost form NewPost
+
+update : Msg -> Model -> Session -> Task Nav
+update msg model session = match msg {
+  CreatePost p -> {
+    let _ <- Db.insert Post { title: p.form.title, slug: p.form.slug, body: p.form.body, published: p.form.published }
+    Task.succeed (Redirect (Desk {}))
+  },
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
+      <al-checkbox field="published" label="Published" />
+      <al-alert variant="destructive"><Errors form /></al-alert>
+      <al-button type="submit">New post</al-button>
+    </form>
+  </al-card>
 </Layout.Page>
 
 test "a visitor is sent to log in" = {
   let r <- Test.open (Desk {})
   Task.succeed (r.answer == Redirected (Auth.Login {}))
 }
+
+test "submitting NewPost adds a row" = {
+  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: Text.Secret "passwordhash-1" }
+  let _ <- Test.login { user: Just me }
+  let _ <- Test.get (Desk {})
+  let r <- Test.submit NewPost { title: Text.NonEmpty "title-1", slug: Slug.Slug "slug-1", body: "body-1", published: True }
+  let rows <- Db.from Post |> Db.all
+  Task.succeed ({ answer: r.answer, rows: List.map (\row -> { title: row.title, slug: row.slug, body: row.body, published: row.published }) rows } == { answer: Redirected (Desk {}), rows: [{ title: Text.NonEmpty "title-1", slug: Slug.Slug "slug-1", body: "body-1", published: True }] })
+}
+
+test "NewPost refuses an empty title" = {
+  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: Text.Secret "passwordhash-1" }
+  let _ <- Test.login { user: Just me }
+  let _ <- Test.get (Desk {})
+  let r <- Test.submitRaw NewPost [{ name: "title", value: "" }, { name: "slug", value: "x" }, { name: "body", value: "x" }, { name: "published", value: "True" }]
+  let rows <- Db.from Post |> Db.count
+  Task.succeed ({ answer: r.answer, rows: rows } == { answer: FormRejected [{ field: "title", message: "must not be empty" }], rows: 0 })
+}
[exit 0]
$ alder new msg Desk Toggle id:Post.PostId
edited app/Desk.alder
--- a/app/Desk.alder
+++ b/app/Desk.alder
@@ -24,12 +24,15 @@
 
 msg CreatePost form NewPost
 
+msg Toggle { id: Post.PostId }
+
 update : Msg -> Model -> Session -> Task Nav
 update msg model session = match msg {
   CreatePost p -> {
     let _ <- Db.insert Post { title: p.form.title, slug: p.form.slug, body: p.form.body, published: p.form.published }
     Task.succeed (Redirect (Desk {}))
   },
+  Toggle p -> ?toggle,
 }
 
 view : Model -> Session -> Html Msg
@@ -47,6 +50,8 @@
       <al-button type="submit">New post</al-button>
     </form>
   </al-card>
+
+  <al-button click={Toggle { id: ?toggleId }}>Toggle</al-button>
 </Layout.Page>
 
 test "a visitor is sent to log in" = {
@@ -71,3 +76,16 @@
   let rows <- Db.from Post |> Db.count
   Task.succeed ({ answer: r.answer, rows: rows } == { answer: FormRejected [{ field: "title", message: "must not be empty" }], rows: 0 })
 }
+
+-- Fill ?expected with what pressing Toggle must do, so that a test fails when the arm
+-- does something else: its answer (`r.answer`: Rerendered, Redirected …) and what it
+-- changed (`after`, the row read again), as `{ answer: r.answer, … } == { answer: Rerendered, … }`.
+test "pressing Toggle" = {
+  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: Text.Secret "passwordhash-1" }
+  let _ <- Test.login { user: Just me }
+  let post1 <- Db.insert Post { title: Text.NonEmpty "title-1", slug: Slug.Slug "slug-1", body: "body-1" }
+  let _ <- Test.get (Desk {})
+  let r <- Test.click (Toggle { id: post1 })
+  let after <- Db.get Post post1
+  Task.succeed ?expected
+}
app/Desk.alder:35:15: warning[W0002]: hole ?toggle has type Task Nav
    Toggle p -> ?toggle,
                ^^^^^^^
app/Desk.alder:54:34: warning[W0002]: hole ?toggleId has type Post.PostId
    <al-button click={Toggle { id: ?toggleId }}>Toggle</al-button>
                                   ^^^^^^^^^
app/Desk.alder:90:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill --decl Desk.view - <<'EOF'
<Layout.Page title="Admin">
  <h1>Admin</h1>
  <al-card>
    <form submit={CreatePost}>
      <al-input field="title" label="Title" />
      <al-input field="slug" label="Slug" />
      <al-textarea field="body" label="Body" />
      <al-checkbox field="published" label="Published" />
      <al-button type="submit">Create</al-button>
    </form>
  </al-card>
  <ul id="posts">
    {for p in model.posts ->
      <li>
        <a href={Posts.Show { slug: p.slug }}>{p.title}</a>
        <span>{if p.published then "published" else "draft"}</span>
        <al-button size="sm" click={Toggle { id: p.id }}>Toggle published</al-button>
      </li>}
  </ul>
  <al-button variant="outline" click={App.Logout}>Log out</al-button>
</Layout.Page>
EOF
edited app/Desk.alder
--- a/app/Desk.alder
+++ b/app/Desk.alder
@@ -36,23 +36,28 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = <Layout.Page title="Admin">
-  <h1>Admin</h1>
-
-  <al-card>
-    <h2 slot="header">New post</h2>
-    <form submit={CreatePost}>
-      <al-input field="title" label="Title" />
-      <al-input field="slug" label="Slug" />
-      <al-textarea field="body" label="Body" />
-      <al-checkbox field="published" label="Published" />
-      <al-alert variant="destructive"><Errors form /></al-alert>
-      <al-button type="submit">New post</al-button>
-    </form>
-  </al-card>
-
-  <al-button click={Toggle { id: ?toggleId }}>Toggle</al-button>
-</Layout.Page>
+view model session =
+  <Layout.Page title="Admin">
+    <h1>Admin</h1>
+    <al-card>
+      <form submit={CreatePost}>
+        <al-input field="title" label="Title" />
+        <al-input field="slug" label="Slug" />
+        <al-textarea field="body" label="Body" />
+        <al-checkbox field="published" label="Published" />
+        <al-button type="submit">Create</al-button>
+      </form>
+    </al-card>
+    <ul id="posts">
+      {for p in model.posts ->
+        <li>
+          <a href={Posts.Show { slug: p.slug }}>{p.title}</a>
+          <span>{if p.published then "published" else "draft"}</span>
+          <al-button size="sm" click={Toggle { id: p.id }}>Toggle published</al-button>
+        </li>}
+    </ul>
+    <al-button variant="outline" click={App.Logout}>Log out</al-button>
+  </Layout.Page>
 
 test "a visitor is sent to log in" = {
   let r <- Test.open (Desk {})
app/Desk.alder:35:15: warning[W0002]: hole ?toggle has type Task Nav
    Toggle p -> ?toggle,
                ^^^^^^^
app/Desk.alder:95:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill Desk.update.CreatePost - <<'EOF'
{
  let _ <- Db.insert Post { title: p.form.title, slug: p.form.slug, body: p.form.body, published: p.form.published }
  Task.succeed Rerender
}
EOF
alder fill: `Desk.update.CreatePost` has no typed hole: a fill replaces a hole, and this arm has none; to rewrite it, give `alder fill --decl Desk.update -` the whole `match msg { … }` (`alder fill --list` lists the app's holes)
[exit 1]
```

The harness stopped at line 126 of your script: `alder fill Desk.update.CreatePost - <<'EOF'`: it exited 1. The commands after it were not run.

Your script stopped at command 21; the app is as the commands before it left it; continue.

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Desk.alder:35:15: warning[W0002]: hole ?toggle has type Task Nav
    Toggle p -> ?toggle,
                ^^^^^^^
app/Desk.alder:95:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Posts/Show.alder:60:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
alder build: the program has 3 typed hole(s); fill them before building
```
