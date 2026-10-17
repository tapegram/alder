### Round 5

```text
$ alder fill Posts.Show.load - --hole ?load <<'EOF'
{
  let found <- Db.from Post |> Db.where (Post.slug.eq params.slug) |> Db.first
  match found {
    Nothing -> Query.notFound,
    Just post -> if post.published || session.user == Just post.author then {
      let comments <-
        Db.from Comment
          |> Db.where (Comment.post.eq post.id)
          |> Db.orderBy Comment.created.asc
          |> Db.all
      Query.succeed { post: post, comments: comments }
    } else Query.notFound,
  }
}
EOF
edited app/Posts/Show.alder
--- a/app/Posts/Show.alder
+++ b/app/Posts/Show.alder
@@ -8,7 +8,20 @@
 type Model = { post: Post, comments: List Comment }
 
 load : Params -> Session -> Query Model
-load params session = ?load
+load params session = {
+  let found <- Db.from Post |> Db.where (Post.slug.eq params.slug) |> Db.first
+  match found {
+    Nothing -> Query.notFound,
+    Just post -> if post.published || session.user == Just post.author then {
+      let comments <-
+        Db.from Comment
+          |> Db.where (Comment.post.eq post.id)
+          |> Db.orderBy Comment.created.asc
+          |> Db.all
+      Query.succeed { post: post, comments: comments }
+    } else Query.notFound,
+  }
+}
 
 view : Model -> Session -> Html Msg
 view model session = ?view
app/Posts/Show.alder:27:22: warning[W0002]: hole ?view has type Html Posts.Show.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder fill Posts.Show.view - --hole ?view <<'EOF'
<Layout.Page title={model.post.title}>
  <h1>{model.post.title}</h1>
  <p>{model.post.body}</p>
  <h2>Comments</h2>
  <ul>
    {for c in model.comments -> <li><strong>{c.author}</strong> {c.body}</li>}
  </ul>
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
@@ -24,4 +25,11 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = ?view
+view model session = <Layout.Page title={model.post.title}>
+  <h1>{model.post.title}</h1>
+  <p>{model.post.body}</p>
+  <h2>Comments</h2>
+  <ul>
+    {for c in model.comments -> <li><strong>{c.author}</strong> {c.body}</li>}
+  </ul>
+</Layout.Page>
added import Layout to app/Posts/Show.alder
[exit 0]
$ alder new form Posts.Show NewComment author:Text.NonEmpty body:Text.NonEmpty --msg AddComment
edited app/Posts/Show.alder
--- a/app/Posts/Show.alder
+++ b/app/Posts/Show.alder
@@ -24,6 +24,18 @@
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
@@ -32,4 +44,26 @@
   <ul>
     {for c in model.comments -> <li><strong>{c.author}</strong> {c.body}</li>}
   </ul>
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
app/Posts/Show.alder:36:19: warning[W0002]: hole ?addComment has type Task Nav
    AddComment p -> ?addComment,
                    ^^^^^^^^^^^
app/Posts/Show.alder:62:29: warning[W0002]: hole ?expected has type Answer
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
@@ -33,7 +33,10 @@
 
 update : Msg -> Model -> Session -> Task Nav
 update msg model session = match msg {
-  AddComment p -> ?addComment,
+  AddComment p -> {
+    let _ <- Db.insert Comment { post: model.post.id, author: p.form.author, body: p.form.body }
+    Task.succeed (Redirect (Posts.Show { slug: model.post.slug }))
+  },
 }
 
 view : Model -> Session -> Html Msg
app/Posts/Show.alder:65:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder fill --decl Posts.Show.view - <<'EOF'
<Layout.Page title={model.post.title}>
  <h1>{model.post.title}</h1>
  <p>{model.post.body}</p>
  <h2>Comments</h2>
  <ul>
    {for c in model.comments -> <li><strong>{c.author}</strong> {c.body}</li>}
  </ul>
  <al-card>
    <form submit={AddComment}>
      <al-input field="author" label="Your name" />
      <Errors field="author" />
      <al-textarea field="body" label="Comment" />
      <Errors field="body" />
      <al-button type="submit">Add comment</al-button>
    </form>
  </al-card>
  <a href={Home {}}>All posts</a>
</Layout.Page>
EOF
app/Posts/Show.alder:54:9: error[E0028]: `<al-input field="author">` already shows the errors of `author`; remove `<Errors field="author" />`
          <Errors field="author" />
          ^^^^^^^^^^^^^^^^^^^^^^^^
app/Posts/Show.alder:56:9: error[E0028]: `<al-textarea field="body">` already shows the errors of `body`; remove `<Errors field="body" />`
          <Errors field="body" />
          ^^^^^^^^^^^^^^^^^^^^^^
app/Posts/Show.alder:66:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
alder fill: the result does not check (the errors above): nothing was written
[exit 1]
```

The harness stopped at line 41 of your script: `alder fill --decl Posts.Show.view - <<'EOF'`: it exited 1. The commands after it were not run.

Your script stopped at command 5; the app is as the commands before it left it; continue.

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Admin.alder:80:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Admin.alder:88:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Posts/Show.alder:65:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
alder build: the program has 3 typed hole(s); fill them before building
```
