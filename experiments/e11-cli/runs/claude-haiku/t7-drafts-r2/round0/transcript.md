### Round 0

```text
$ alder edit table Post add-field published:Bool=False
edited app/Post.alder
wrote migrations/0002_add_published.sql
  rebuild table Post: Post.published: new column filled with 0
edited migrations/schema.json
--- a/app/Post.alder
+++ b/app/Post.alder
@@ -12,6 +12,7 @@
   body: Text,
   author: User.UserId @ref User,
   created: Time @created,
+  published: Bool = False,
 }
 
 bySlug : Slug -> Query (Maybe Post)
[exit 0]
$ alder new msg Admin Publish id:Post.PostId
edited app/Admin.alder
--- a/app/Admin.alder
+++ b/app/Admin.alder
@@ -38,6 +38,8 @@
 
 msg Create form NewPost
 
+msg Publish { id: Post.PostId }
+
 update : Msg -> Model -> Session -> Task Nav
 update msg model session = match msg {
   Create p -> {
@@ -50,6 +52,7 @@
       },
     }
   },
+  Publish p -> ?publish,
 }
 
 view : Model -> Session -> Html Msg
@@ -77,6 +80,8 @@
     </form>
 
     <button click={App.Logout}>Log out</button>
+
+    <al-button click={Publish { id: ?publishId }}>Publish</al-button>
   </Layout.Page>
 
 test "a Reader cannot open the admin page" = {
@@ -130,3 +135,16 @@
   let out <- Test.click App.Logout
   Task.succeed ({ heading: Test.text "h1" page.doc, out: out.answer, user: out.session.user } == { heading: Just "Your posts", out: Redirected (Auth.Login {}), user: Nothing })
 }
+
+-- Fill ?expected with what pressing Publish must do, so that a test fails when the arm
+-- does something else: its answer (`r.answer`: Rerendered, Redirected …) and what it
+-- changed (`after`, the row read again), as `{ answer: r.answer, … } == { answer: Rerendered, … }`.
+test "pressing Publish" = {
+  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
+  let _ <- Test.login { user: Just me }
+  let post1 <- Db.insert Post { slug: Post.Slug "slug-1", title: Text.NonEmpty "title-1", body: "body-1", author: me }
+  let _ <- Test.get (Admin {})
+  let r <- Test.click (Publish { id: post1 })
+  let after <- Db.get Post post1
+  Task.succeed ?expected
+}
app/Admin.alder:55:16: warning[W0002]: hole ?publish has type Task Nav
    Publish p -> ?publish,
                 ^^^^^^^^
app/Admin.alder:84:37: warning[W0002]: hole ?publishId has type Post.PostId
      <al-button click={Publish { id: ?publishId }}>Publish</al-button>
                                      ^^^^^^^^^^
app/Admin.alder:149:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill --decl Admin.update - <<'EOF'
match msg {
  Create p -> {
    let taken <- Post.bySlug p.form.slug
    match taken {
      Just _ -> Task.succeed (Reject "Another post has that slug."),
      Nothing -> {
        let id <- Db.insert Post { slug: p.form.slug, title: p.form.title, body: p.form.body, author: model.author.id }
        Task.succeed (Redirect (Admin.Edit { id: id }))
      },
    }
  },
  Publish p -> {
    let found <- Db.get Post p.id
    match found {
      Just post -> {
        if post.author != model.author.id then Task.notFound else {
          let _ <- Db.update Post post.id (\x -> { x | published: True })
          Task.succeed (Redirect (Admin {}))
        }
      },
      Nothing -> Task.notFound,
    }
  },
}
EOF
edited app/Admin.alder
--- a/app/Admin.alder
+++ b/app/Admin.alder
@@ -52,7 +52,18 @@
       },
     }
   },
-  Publish p -> ?publish,
+  Publish p -> {
+    let found <- Db.get Post p.id
+    match found {
+      Just post -> {
+        if post.author != model.author.id then Task.notFound else {
+          let _ <- Db.update Post post.id (\x -> { x | published: True })
+          Task.succeed (Redirect (Admin {}))
+        }
+      },
+      Nothing -> Task.notFound,
+    }
+  },
 }
 
 view : Model -> Session -> Html Msg
app/Admin.alder:95:37: warning[W0002]: hole ?publishId has type Post.PostId
      <al-button click={Publish { id: ?publishId }}>Publish</al-button>
                                      ^^^^^^^^^^
app/Admin.alder:160:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill --decl Admin.view - <<'EOF'
<Layout.Page title="Your posts">
    <h1>Your posts</h1>

    <h2>Drafts</h2>
    <ul id="drafts">
      {for post in List.filter (\p -> not p.published) model.posts ->
        <li>
          <a class="title" href={Admin.Edit { id: post.id }}>{post.title}</a>
          <button click={Publish { id: post.id }}>Publish</button>
        </li>}
    </ul>

    <h2>New post</h2>
    <form submit={Create}>
      <Errors form />
      <Input field="slug" label="Slug (lowercase letters, digits and hyphens)" />
      <Errors field="slug" />
      <Input field="title" label="Title" />
      <Errors field="title" />
      <Textarea field="body" label="Body (Markdown)" />
      <Errors field="body" />
      <button>Create post</button>
    </form>

    <button click={App.Logout}>Log out</button>
  </Layout.Page>
EOF
edited app/Admin.alder
--- a/app/Admin.alder
+++ b/app/Admin.alder
@@ -69,31 +69,31 @@
 view : Model -> Session -> Html Msg
 view model session =
   <Layout.Page title="Your posts">
-    <h1>Your posts</h1>
-
-    <ul class="posts">
-      {for post in model.posts ->
-        <li>
-          <a href={Admin.Edit { id: post.id }}>{post.title}</a>
-        </li>}
-    </ul>
+      <h1>Your posts</h1>
 
-    <h2>New post</h2>
-    <form submit={Create}>
-      <Errors form />
-      <Input field="slug" label="Slug (lowercase letters, digits and hyphens)" />
-      <Errors field="slug" />
-      <Input field="title" label="Title" />
-      <Errors field="title" />
-      <Textarea field="body" label="Body (Markdown)" />
-      <Errors field="body" />
-      <button>Create post</button>
-    </form>
+      <h2>Drafts</h2>
+      <ul id="drafts">
+        {for post in List.filter (\p -> not p.published) model.posts ->
+          <li>
+            <a class="title" href={Admin.Edit { id: post.id }}>{post.title}</a>
+            <button click={Publish { id: post.id }}>Publish</button>
+          </li>}
+      </ul>
 
-    <button click={App.Logout}>Log out</button>
+      <h2>New post</h2>
+      <form submit={Create}>
+        <Errors form />
+        <Input field="slug" label="Slug (lowercase letters, digits and hyphens)" />
+        <Errors field="slug" />
+        <Input field="title" label="Title" />
+        <Errors field="title" />
+        <Textarea field="body" label="Body (Markdown)" />
+        <Errors field="body" />
+        <button>Create post</button>
+      </form>
 
-    <al-button click={Publish { id: ?publishId }}>Publish</al-button>
-  </Layout.Page>
+      <button click={App.Logout}>Log out</button>
+    </Layout.Page>
 
 test "a Reader cannot open the admin page" = {
   let reader <- Db.insert User { email: Text.Email "reader@example.com", name: Text.NonEmpty "Reader", passwordHash: "" }
app/Admin.alder:160:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill --decl Posts.Index.load - <<'EOF'
{
  let page = if params.page < 1 then 1 else if params.page > lastPage then lastPage else params.page
  let q = Text.slice 0 100 (Text.trim params.q)
  let matching =
    if Text.isEmpty q then Db.and [] else Db.or [Post.title.contains q, Post.body.contains q]
  let found <-
    Db.from Post
      |> Db.where (Post.published.eq True)
      |> Db.where matching
      |> Db.orderBy Post.created.desc
      |> Db.limit (perPage + 1)
      |> Db.offset ((page - 1) * perPage)
      |> Db.all
  let me <- match session.user {
    Just id -> User.author id,
    Nothing -> Query.succeed Nothing,
  }
  let canWrite = me != Nothing
  Query.succeed { page: page, q: q, posts: List.take perPage found, hasMore: List.length found > perPage, canWrite: canWrite }
}
EOF
edited app/Posts/Index.alder
--- a/app/Posts/Index.alder
+++ b/app/Posts/Index.alder
@@ -33,6 +33,7 @@
     if Text.isEmpty q then Db.and [] else Db.or [Post.title.contains q, Post.body.contains q]
   let found <-
     Db.from Post
+      |> Db.where (Post.published.eq True)
       |> Db.where matching
       |> Db.orderBy Post.created.desc
       |> Db.limit (perPage + 1)
[exit 0]
$ alder fill --decl Posts.Show.load - <<'EOF'
{
  let found <- Post.bySlug params.slug
  match found {
    Just post -> {
      if post.published || session.user == Just post.author then {
        let body <- Markdown.render post.body
        let comments <-
          Db.from Comment
            |> Db.where (Comment.post.eq post.id)
            |> Db.orderBy Comment.created.asc
            |> Db.all
        Query.succeed { post: post, body: body, comments: comments }
      } else Query.notFound,
    },
    Nothing -> Query.notFound,
  }
}
EOF
alder fill: `alder fill --decl` takes one expression; standard input does not parse as one: expected `}`, got `,`; the last expression of a block is its result and takes no `,` (line 13, column 28)
[exit 1]
```

The harness stopped at line 88 of your script: `alder fill --decl Posts.Show.load - <<'EOF'`: it exited 1. The commands after it were not run.

Your script stopped at command 6; the app is as the commands before it left it; continue.

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Admin.alder:160:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
alder build: the program has 1 typed hole(s); fill them before building
```
