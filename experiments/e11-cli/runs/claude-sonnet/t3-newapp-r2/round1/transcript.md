### Round 1

```text
$ alder fill Posts.Index.load - <<'EOF'
{
  let posts <- Db.from Post |> Db.where (Post.published.eq True) |> Db.orderBy Post.created.desc |> Db.all
  Query.succeed { posts: posts }
}
EOF
edited app/Posts/Index.alder
--- a/app/Posts/Index.alder
+++ b/app/Posts/Index.alder
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
app/Posts/Index.alder:16:22: warning[W0002]: hole ?view has type Html Posts.Index.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder fill Posts.Index.view - <<'EOF'
<Layout.Page title="Blog">
  <h1>Blog</h1>
  <ul id="posts">
    {for p in model.posts -> <li><a href={Posts.Show { slug: p.slug }}>{p.title}</a></li>}
  </ul>
</Layout.Page>
EOF
edited app/Posts/Index.alder
--- a/app/Posts/Index.alder
+++ b/app/Posts/Index.alder
@@ -1,5 +1,6 @@
 module Posts.Index
 
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
added import Layout to app/Posts/Index.alder
[exit 0]
$ alder new test Posts.Index "the front page lists only published posts, newest first"
edited app/Posts/Index.alder
--- a/app/Posts/Index.alder
+++ b/app/Posts/Index.alder
@@ -20,3 +20,7 @@
     {for p in model.posts -> <li><a href={Posts.Show { slug: p.slug }}>{p.title}</a></li>}
   </ul>
 </Layout.Page>
+
+test "the front page lists only published posts, newest first" = {
+  ?test
+}
app/Posts/Index.alder:25:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Posts.Index."the front page lists only published posts, newest first"' - <<'EOF'
{
  let _ <- Db.insert Post { title: Text.NonEmpty "First", slug: Post.Slug "first", body: "a", published: True }
  let _ <- Db.insert Post { title: Text.NonEmpty "Hidden", slug: Post.Slug "hidden", body: "b", published: False }
  let _ <- Db.insert Post { title: Text.NonEmpty "Second", slug: Post.Slug "second", body: "c", published: True }
  let page <- Test.get (Posts.Index {})
  Task.succeed (Test.texts "li a" page.doc == ["Second", "First"])
}
EOF
edited app/Posts/Index.alder
--- a/app/Posts/Index.alder
+++ b/app/Posts/Index.alder
@@ -22,5 +22,9 @@
 </Layout.Page>
 
 test "the front page lists only published posts, newest first" = {
-  ?test
+  let _ <- Db.insert Post { title: Text.NonEmpty "First", slug: Post.Slug "first", body: "a", published: True }
+  let _ <- Db.insert Post { title: Text.NonEmpty "Hidden", slug: Post.Slug "hidden", body: "b", published: False }
+  let _ <- Db.insert Post { title: Text.NonEmpty "Second", slug: Post.Slug "second", body: "c", published: True }
+  let page <- Test.get (Posts.Index {})
+  Task.succeed (Test.texts "li a" page.doc == ["Second", "First"])
 }
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
    Nothing -> Query.notFound,
    Just post -> {
      let ok = post.published || session.user != Nothing
      if ok then {
        let comments <- Db.from Comment |> Db.where (Comment.post.eq post.id) |> Db.orderBy Comment.created.asc |> Db.all
        Query.succeed { post: post, comments: comments }
      } else Query.notFound
    },
  }
}
EOF
edited app/Posts/Show.alder
--- a/app/Posts/Show.alder
+++ b/app/Posts/Show.alder
@@ -8,7 +8,19 @@
 type Model = { post: Post, comments: List Comment }
 
 load : Params -> Session -> Query Model
-load params session = ?load
+load params session = {
+  let found <- Db.from Post |> Db.where (Post.slug.eq params.slug) |> Db.first
+  match found {
+    Nothing -> Query.notFound,
+    Just post -> {
+      let ok = post.published || session.user != Nothing
+      if ok then {
+        let comments <- Db.from Comment |> Db.where (Comment.post.eq post.id) |> Db.orderBy Comment.created.asc |> Db.all
+        Query.succeed { post: post, comments: comments }
+      } else Query.notFound
+    },
+  }
+}
 
 view : Model -> Session -> Html Msg
 view model session = ?view
app/Posts/Show.alder:26:22: warning[W0002]: hole ?view has type Html Posts.Show.Msg
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
@@ -23,4 +24,6 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = ?view
+view model session = <Layout.Page title={model.post.title}>
+  <h1>{model.post.title}</h1>
+</Layout.Page>
added import Layout to app/Posts/Show.alder
[exit 0]
$ alder new form Posts.Show NewComment author:Text.NonEmpty body:Text.NonEmpty --msg Submit
edited app/Posts/Show.alder
--- a/app/Posts/Show.alder
+++ b/app/Posts/Show.alder
@@ -23,7 +23,41 @@
   }
 }
 
+form NewComment = {
+  author: Text.NonEmpty,
+  body: Text.NonEmpty,
+}
+
+msg Submit form NewComment
+
+update : Msg -> Model -> Session -> Task Nav
+update msg model session = match msg {
+  Submit p -> ?submit,
+}
+
 view : Model -> Session -> Html Msg
 view model session = <Layout.Page title={model.post.title}>
   <h1>{model.post.title}</h1>
+
+  <al-card>
+    <h2 slot="header">New comment</h2>
+    <form submit={Submit}>
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
app/Posts/Show.alder:35:15: warning[W0002]: hole ?submit has type Task Nav
    Submit p -> ?submit,
                ^^^^^^^
app/Posts/Show.alder:56:29: warning[W0002]: hole ?expected has type Answer
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
  <form submit={Submit}>
    <Errors form />
    <Input field="author" label="Name" />
    <Errors field="author" />
    <Textarea field="body" label="Comment" />
    <Errors field="body" />
    <button>Add comment</button>
  </form>
  <a href={Posts.Index {}}>Back</a>
</Layout.Page>
EOF
edited app/Posts/Show.alder
--- a/app/Posts/Show.alder
+++ b/app/Posts/Show.alder
@@ -36,19 +36,24 @@
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
     <form submit={Submit}>
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
+    <a href={Posts.Index {}}>Back</a>
+  </Layout.Page>
 
 test "submitting NewComment" = {
   let _ <- Test.get (Posts.Show { slug: Post.Slug "slug-1" })
app/Posts/Show.alder:35:15: warning[W0002]: hole ?submit has type Task Nav
    Submit p -> ?submit,
                ^^^^^^^
app/Posts/Show.alder:61:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder fill Posts.Show.update.Submit - <<'EOF'
{
  let _ <- Db.insert Comment { post: model.post.id, author: p.form.author, body: p.form.body }
  Task.succeed Rerender
}
EOF
edited app/Posts/Show.alder
--- a/app/Posts/Show.alder
+++ b/app/Posts/Show.alder
@@ -32,7 +32,10 @@
 
 update : Msg -> Model -> Session -> Task Nav
 update msg model session = match msg {
-  Submit p -> ?submit,
+  Submit p -> {
+    let _ <- Db.insert Comment { post: model.post.id, author: p.form.author, body: p.form.body }
+    Task.succeed Rerender
+  },
 }
 
 view : Model -> Session -> Html Msg
app/Posts/Show.alder:64:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder new test Posts.Show "an unpublished post is not found for a reader"
edited app/Posts/Show.alder
--- a/app/Posts/Show.alder
+++ b/app/Posts/Show.alder
@@ -69,3 +69,7 @@
   let r <- Test.submitRaw NewComment [{ name: "author", value: "" }, { name: "body", value: "x" }]
   Task.succeed (r.answer == FormRejected [{ field: "author", message: "must not be empty" }])
 }
+
+test "an unpublished post is not found for a reader" = {
+  ?test
+}
app/Posts/Show.alder:64:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Posts/Show.alder:74:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Posts.Show."an unpublished post is not found for a reader"' - <<'EOF'
{
  let _ <- Db.insert Post { title: Text.NonEmpty "Draft", slug: Post.Slug "draft", body: "x", published: False }
  let r <- Test.open (Posts.Show { slug: Post.Slug "draft" })
  Task.succeed (r.answer == PageMissing)
}
EOF
edited app/Posts/Show.alder
--- a/app/Posts/Show.alder
+++ b/app/Posts/Show.alder
@@ -71,5 +71,7 @@
 }
 
 test "an unpublished post is not found for a reader" = {
-  ?test
+  let _ <- Db.insert Post { title: Text.NonEmpty "Draft", slug: Post.Slug "draft", body: "x", published: False }
+  let r <- Test.open (Posts.Show { slug: Post.Slug "draft" })
+  Task.succeed (r.answer == PageMissing)
 }
app/Posts/Show.alder:64:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder new test Posts.Show "a comment with an empty author is refused"
edited app/Posts/Show.alder
--- a/app/Posts/Show.alder
+++ b/app/Posts/Show.alder
@@ -75,3 +75,7 @@
   let r <- Test.open (Posts.Show { slug: Post.Slug "draft" })
   Task.succeed (r.answer == PageMissing)
 }
+
+test "a comment with an empty author is refused" = {
+  ?test
+}
app/Posts/Show.alder:64:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Posts/Show.alder:80:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Posts.Show."a comment with an empty author is refused"' - <<'EOF'
{
  let _ <- Db.insert Post { title: Text.NonEmpty "Hello", slug: Post.Slug "hello", body: "x", published: True }
  let _ <- Test.get (Posts.Show { slug: Post.Slug "hello" })
  let r <- Test.submitRaw NewComment [{ name: "author", value: "" }, { name: "body", value: "Hi" }]
  let n <- Db.from Comment |> Db.count
  Task.succeed ({ answer: r.answer, n: n } == { answer: FormRejected [{ field: "author", message: "must not be empty" }], n: 0 })
}
EOF
edited app/Posts/Show.alder
--- a/app/Posts/Show.alder
+++ b/app/Posts/Show.alder
@@ -77,5 +77,9 @@
 }
 
 test "a comment with an empty author is refused" = {
-  ?test
+  let _ <- Db.insert Post { title: Text.NonEmpty "Hello", slug: Post.Slug "hello", body: "x", published: True }
+  let _ <- Test.get (Posts.Show { slug: Post.Slug "hello" })
+  let r <- Test.submitRaw NewComment [{ name: "author", value: "" }, { name: "body", value: "Hi" }]
+  let n <- Db.from Comment |> Db.count
+  Task.succeed ({ answer: r.answer, n: n } == { answer: FormRejected [{ field: "author", message: "must not be empty" }], n: 0 })
 }
app/Posts/Show.alder:64:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder fill --decl Admin.Index.Model - <<'EOF'
{ posts: List Post }
EOF
edited app/Admin/Index.alder
--- a/app/Admin/Index.alder
+++ b/app/Admin/Index.alder
@@ -1,8 +1,10 @@
 module Admin.Index
 
+import Post
+
 page "/admin" { } requires user
 
-type Model = { }
+type Model = { posts: List Post }
 
 load : Params -> Session -> Query Model
 load params session = ?load
app/Admin/Index.alder:10:23: warning[W0002]: hole ?load has type Query Admin.Index.Model
  load params session = ?load
                        ^^^^^
app/Admin/Index.alder:13:22: warning[W0002]: hole ?view has type Html Admin.Index.Msg
  view model session = ?view
                       ^^^^^
added import Post to app/Admin/Index.alder
[exit 0]
$ alder fill Admin.Index.load - <<'EOF'
{
  let posts <- Db.from Post |> Db.orderBy Post.created.desc |> Db.all
  Query.succeed { posts: posts }
}
EOF
edited app/Admin/Index.alder
--- a/app/Admin/Index.alder
+++ b/app/Admin/Index.alder
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
app/Admin/Index.alder:16:22: warning[W0002]: hole ?view has type Html Admin.Index.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder fill Admin.Index.view - <<'EOF'
<Layout.Page title="Admin">
  <h1>Admin</h1>
</Layout.Page>
EOF
edited app/Admin/Index.alder
--- a/app/Admin/Index.alder
+++ b/app/Admin/Index.alder
@@ -1,5 +1,6 @@
 module Admin.Index
 
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
   let r <- Test.open (Admin.Index {})
added import Layout to app/Admin/Index.alder
[exit 0]
$ alder new form Admin.Index NewPost title:Text.NonEmpty slug:Post.Slug body:Text published:Bool --msg Create
edited app/Admin/Index.alder
--- a/app/Admin/Index.alder
+++ b/app/Admin/Index.alder
@@ -2,6 +2,7 @@
 
 import Layout
 import Post
+import User
 
 page "/admin" { } requires user
 
@@ -13,12 +14,59 @@
   Query.succeed { posts: posts }
 }
 
+form NewPost = {
+  title: Text.NonEmpty,
+  slug: Post.Slug,
+  body: Text,
+  published: Bool,
+}
+
+msg Create form NewPost
+
+update : Msg -> Model -> Session -> Task Nav
+update msg model session = match msg {
+  Create p -> {
+    let _ <- Db.insert Post { title: p.form.title, slug: p.form.slug, body: p.form.body, published: p.form.published }
+    Task.succeed (Redirect (Admin.Index {}))
+  },
+}
+
 view : Model -> Session -> Html Msg
 view model session = <Layout.Page title="Admin">
   <h1>Admin</h1>
+
+  <al-card>
+    <h2 slot="header">New post</h2>
+    <form submit={Create}>
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
   let r <- Test.open (Admin.Index {})
   Task.succeed (r.answer == Redirected (Auth.Login {}))
 }
+
+test "submitting NewPost adds a row" = {
+  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: Text.Secret "passwordhash-1" }
+  let _ <- Test.login { user: Just me }
+  let _ <- Test.get (Admin.Index {})
+  let r <- Test.submit NewPost { title: Text.NonEmpty "title-1", slug: Post.Slug "slug-1", body: "body-1", published: True }
+  let rows <- Db.from Post |> Db.all
+  Task.succeed ({ answer: r.answer, rows: List.map (\row -> { title: row.title, slug: row.slug, body: row.body, published: row.published }) rows } == { answer: Redirected (Admin.Index {}), rows: [{ title: Text.NonEmpty "title-1", slug: Post.Slug "slug-1", body: "body-1", published: True }] })
+}
+
+test "NewPost refuses an empty title" = {
+  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: Text.Secret "passwordhash-1" }
+  let _ <- Test.login { user: Just me }
+  let _ <- Test.get (Admin.Index {})
+  let r <- Test.submitRaw NewPost [{ name: "title", value: "" }, { name: "slug", value: "x" }, { name: "body", value: "x" }, { name: "published", value: "True" }]
+  let rows <- Db.from Post |> Db.count
+  Task.succeed ({ answer: r.answer, rows: rows } == { answer: FormRejected [{ field: "title", message: "must not be empty" }], rows: 0 })
+}
[exit 0]
$ alder new msg Admin.Index Toggle id:Post.PostId
edited app/Admin/Index.alder
--- a/app/Admin/Index.alder
+++ b/app/Admin/Index.alder
@@ -23,12 +23,15 @@
 
 msg Create form NewPost
 
+msg Toggle { id: Post.PostId }
+
 update : Msg -> Model -> Session -> Task Nav
 update msg model session = match msg {
   Create p -> {
     let _ <- Db.insert Post { title: p.form.title, slug: p.form.slug, body: p.form.body, published: p.form.published }
     Task.succeed (Redirect (Admin.Index {}))
   },
+  Toggle p -> ?toggle,
 }
 
 view : Model -> Session -> Html Msg
@@ -46,6 +49,8 @@
       <al-button type="submit">New post</al-button>
     </form>
   </al-card>
+
+  <al-button click={Toggle { id: ?toggleId }}>Toggle</al-button>
 </Layout.Page>
 
 test "a visitor is sent to log in" = {
@@ -70,3 +75,16 @@
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
+  let post1 <- Db.insert Post { title: Text.NonEmpty "title-1", slug: Post.Slug "slug-1", body: "body-1" }
+  let _ <- Test.get (Admin.Index {})
+  let r <- Test.click (Toggle { id: post1 })
+  let after <- Db.get Post post1
+  Task.succeed ?expected
+}
app/Admin/Index.alder:34:15: warning[W0002]: hole ?toggle has type Task Nav
    Toggle p -> ?toggle,
                ^^^^^^^
app/Admin/Index.alder:53:34: warning[W0002]: hole ?toggleId has type Post.PostId
    <al-button click={Toggle { id: ?toggleId }}>Toggle</al-button>
                                   ^^^^^^^^^
app/Admin/Index.alder:89:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill --decl Admin.Index.view - <<'EOF'
<Layout.Page title="Admin">
  <h1>Admin</h1>
  <form submit={Create}>
    <Errors form />
    <Input field="title" label="Title" />
    <Input field="slug" label="Slug" />
    <Textarea field="body" label="Body" />
    <Input field="published" label="Published" />
    <button>Create</button>
  </form>
  <ul id="posts">
    {for p in model.posts -> <li><a href={Posts.Show { slug: p.slug }}>{p.title}</a> {if p.published then "published" else "draft"} <button click={Toggle { id: p.id }}>Toggle published</button></li>}
  </ul>
  <button click={App.Logout}>Log out</button>
</Layout.Page>
EOF
edited app/Admin/Index.alder
--- a/app/Admin/Index.alder
+++ b/app/Admin/Index.alder
@@ -35,23 +35,22 @@
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
     <form submit={Create}>
-      <al-input field="title" label="Title" />
-      <al-input field="slug" label="Slug" />
-      <al-textarea field="body" label="Body" />
-      <al-checkbox field="published" label="Published" />
-      <al-alert variant="destructive"><Errors form /></al-alert>
-      <al-button type="submit">New post</al-button>
+      <Errors form />
+      <Input field="title" label="Title" />
+      <Input field="slug" label="Slug" />
+      <Textarea field="body" label="Body" />
+      <Input field="published" label="Published" />
+      <button>Create</button>
     </form>
-  </al-card>
-
-  <al-button click={Toggle { id: ?toggleId }}>Toggle</al-button>
-</Layout.Page>
+    <ul id="posts">
+      {for p in model.posts -> <li><a href={Posts.Show { slug: p.slug }}>{p.title}</a> {if p.published then "published" else "draft"} <button click={Toggle { id: p.id }}>Toggle published</button></li>}
+    </ul>
+    <button click={App.Logout}>Log out</button>
+  </Layout.Page>
 
 test "a visitor is sent to log in" = {
   let r <- Test.open (Admin.Index {})
app/Admin/Index.alder:34:15: warning[W0002]: hole ?toggle has type Task Nav
    Toggle p -> ?toggle,
                ^^^^^^^
app/Admin/Index.alder:88:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill Admin.Index.update.Create - <<'EOF'
{
  let _ <- Db.insert Post { title: p.form.title, slug: p.form.slug, body: p.form.body, published: p.form.published }
  Task.succeed Rerender
}
EOF
alder fill: `Admin.Index.update.Create` has no typed hole: a fill replaces a hole, and this arm has none; to rewrite it, give `alder fill --decl Admin.Index.update -` the whole `match msg { … }` (`alder fill --list` lists the app's holes)
[exit 1]
```

The harness stopped at line 130 of your script: `alder fill Admin.Index.update.Create - <<'EOF'`: it exited 1. The commands after it were not run.

Your script stopped at command 21; the app is as the commands before it left it; continue.

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Admin/Index.alder:34:15: warning[W0002]: hole ?toggle has type Task Nav
    Toggle p -> ?toggle,
                ^^^^^^^
app/Admin/Index.alder:88:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Posts/Show.alder:64:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
alder build: the program has 3 typed hole(s); fill them before building
```
