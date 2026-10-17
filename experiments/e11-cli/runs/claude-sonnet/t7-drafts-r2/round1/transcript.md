### Round 1

```text
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
    Just post -> if post.published || session.user == Just post.author then {
      let body <- Markdown.render post.body
      let comments <-
        Db.from Comment
          |> Db.where (Comment.post.eq post.id)
          |> Db.orderBy Comment.created.asc
          |> Db.all
      Query.succeed { post: post, body: body, comments: comments }
    } else Query.notFound,
    Nothing -> Query.notFound,
  }
}
EOF
edited app/Posts/Show.alder
--- a/app/Posts/Show.alder
+++ b/app/Posts/Show.alder
@@ -19,7 +19,7 @@
 load params session = {
   let found <- Post.bySlug params.slug
   match found {
-    Just post -> {
+    Just post -> if post.published || session.user == Just post.author then {
       let body <- Markdown.render post.body
       let comments <-
         Db.from Comment
@@ -27,7 +27,7 @@
           |> Db.orderBy Comment.created.asc
           |> Db.all
       Query.succeed { post: post, body: body, comments: comments }
-    },
+    } else Query.notFound,
     Nothing -> Query.notFound,
   }
 }
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
$ alder fill Admin.update.Publish - <<'EOF'
{
  let found <- Db.get Post p.id
  match found {
    Just post -> if post.author == model.author.id then {
      let _ <- Db.update Post post.id (\x -> { x | published: True })
      Task.succeed (Redirect (Admin {}))
    } else Task.notFound,
    Nothing -> Task.notFound,
  }
}
EOF
edited app/Admin.alder
--- a/app/Admin.alder
+++ b/app/Admin.alder
@@ -52,7 +52,16 @@
       },
     }
   },
-  Publish p -> ?publish,
+  Publish p -> {
+    let found <- Db.get Post p.id
+    match found {
+      Just post -> if post.author == model.author.id then {
+        let _ <- Db.update Post post.id (\x -> { x | published: True })
+        Task.succeed (Redirect (Admin {}))
+      } else Task.notFound,
+      Nothing -> Task.notFound,
+    }
+  },
 }
 
 view : Model -> Session -> Html Msg
app/Admin.alder:93:37: warning[W0002]: hole ?publishId has type Post.PostId
      <al-button click={Publish { id: ?publishId }}>Publish</al-button>
                                      ^^^^^^^^^^
app/Admin.alder:158:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill --decl Admin.view - <<'EOF'
{
  let drafts = List.filter (\p -> not p.published) model.posts
  let live = List.filter (\p -> p.published) model.posts
  <Layout.Page title="Your posts">
    <h1>Your posts</h1>

    <h2>Drafts</h2>
    <ul id="drafts">
      {for post in drafts ->
        <li>
          <a href={Admin.Edit { id: post.id }}><span class="title">{post.title}</span></a>
          <button click={Publish { id: post.id }}>Publish</button>
        </li>}
    </ul>

    <h2>Published</h2>
    <ul class="posts">
      {for post in live ->
        <li>
          <a href={Admin.Edit { id: post.id }}>{post.title}</a>
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
}
EOF
edited app/Admin.alder
--- a/app/Admin.alder
+++ b/app/Admin.alder
@@ -66,32 +66,44 @@
 
 view : Model -> Session -> Html Msg
 view model session =
-  <Layout.Page title="Your posts">
-    <h1>Your posts</h1>
+  {
+    let drafts = List.filter (\p -> not p.published) model.posts
+    let live = List.filter (\p -> p.published) model.posts
+    <Layout.Page title="Your posts">
+      <h1>Your posts</h1>
 
-    <ul class="posts">
-      {for post in model.posts ->
-        <li>
-          <a href={Admin.Edit { id: post.id }}>{post.title}</a>
-        </li>}
-    </ul>
+      <h2>Drafts</h2>
+      <ul id="drafts">
+        {for post in drafts ->
+          <li>
+            <a href={Admin.Edit { id: post.id }}><span class="title">{post.title}</span></a>
+            <button click={Publish { id: post.id }}>Publish</button>
+          </li>}
+      </ul>
 
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
+      <h2>Published</h2>
+      <ul class="posts">
+        {for post in live ->
+          <li>
+            <a href={Admin.Edit { id: post.id }}>{post.title}</a>
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
+  }
 
 test "a Reader cannot open the admin page" = {
   let reader <- Db.insert User { email: Text.Email "reader@example.com", name: Text.NonEmpty "Reader", passwordHash: "" }
app/Admin.alder:170:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill --decl 'Posts.Index."the front page lists the posts, newest first"' - <<'EOF'
{
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "first", title: Text.NonEmpty "First", body: "", author: author, published: True }
  let _ <- Db.insert Post { slug: Post.Slug "second", title: Text.NonEmpty "Second", body: "", author: author, published: True }
  let model <- load { page: 1, q: "" } { user: Nothing }
  Task.succeed (List.map (\p -> p.title) model.posts == [Text.NonEmpty "Second", Text.NonEmpty "First"])
}
EOF
edited app/Posts/Index.alder
--- a/app/Posts/Index.alder
+++ b/app/Posts/Index.alder
@@ -95,8 +95,8 @@
 
 test "the front page lists the posts, newest first" = {
   let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
-  let _ <- Db.insert Post { slug: Post.Slug "first", title: Text.NonEmpty "First", body: "", author: author }
-  let _ <- Db.insert Post { slug: Post.Slug "second", title: Text.NonEmpty "Second", body: "", author: author }
+  let _ <- Db.insert Post { slug: Post.Slug "first", title: Text.NonEmpty "First", body: "", author: author, published: True }
+  let _ <- Db.insert Post { slug: Post.Slug "second", title: Text.NonEmpty "Second", body: "", author: author, published: True }
   let model <- load { page: 1, q: "" } { user: Nothing }
   Task.succeed (List.map (\p -> p.title) model.posts == [Text.NonEmpty "Second", Text.NonEmpty "First"])
 }
rewriting a test: `alder test --mutate` is the check that it still tests something
[exit 0]
$ alder fill --decl 'Posts.Index."search matches title or body"' - <<'EOF'
{
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "tomatoes", title: Text.NonEmpty "Tomato season", body: "It is here.", author: author, published: True }
  let _ <- Db.insert Post { slug: Post.Slug "soup", title: Text.NonEmpty "Soup", body: "Use ripe tomatoes.", author: author, published: True }
  let _ <- Db.insert Post { slug: Post.Slug "trains", title: Text.NonEmpty "Trains", body: "Fast ones.", author: author, published: True }
  let model <- load { page: 1, q: "TOMATO" } { user: Nothing }
  let titles = List.map (\p -> p.title) model.posts
  Task.succeed ({ count: List.length titles, title: List.member (Text.NonEmpty "Tomato season") titles, body: List.member (Text.NonEmpty "Soup") titles } == { count: 2, title: True, body: True })
}
EOF
edited app/Posts/Index.alder
--- a/app/Posts/Index.alder
+++ b/app/Posts/Index.alder
@@ -103,9 +103,9 @@
 
 test "search matches title or body" = {
   let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
-  let _ <- Db.insert Post { slug: Post.Slug "tomatoes", title: Text.NonEmpty "Tomato season", body: "It is here.", author: author }
-  let _ <- Db.insert Post { slug: Post.Slug "soup", title: Text.NonEmpty "Soup", body: "Use ripe tomatoes.", author: author }
-  let _ <- Db.insert Post { slug: Post.Slug "trains", title: Text.NonEmpty "Trains", body: "Fast ones.", author: author }
+  let _ <- Db.insert Post { slug: Post.Slug "tomatoes", title: Text.NonEmpty "Tomato season", body: "It is here.", author: author, published: True }
+  let _ <- Db.insert Post { slug: Post.Slug "soup", title: Text.NonEmpty "Soup", body: "Use ripe tomatoes.", author: author, published: True }
+  let _ <- Db.insert Post { slug: Post.Slug "trains", title: Text.NonEmpty "Trains", body: "Fast ones.", author: author, published: True }
   let model <- load { page: 1, q: "TOMATO" } { user: Nothing }
   let titles = List.map (\p -> p.title) model.posts
   Task.succeed ({ count: List.length titles, title: List.member (Text.NonEmpty "Tomato season") titles, body: List.member (Text.NonEmpty "Soup") titles } == { count: 2, title: True, body: True })
rewriting a test: `alder test --mutate` is the check that it still tests something
[exit 0]
$ alder fill --decl 'Posts.Index."a `%` in the query is literal"' - <<'EOF'
{
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "done", title: Text.NonEmpty "100% done", body: "", author: author, published: True }
  let _ <- Db.insert Post { slug: Post.Slug "ways", title: Text.NonEmpty "1000 ways", body: "", author: author, published: True }
  let model <- load { page: 1, q: "100%" } { user: Nothing }
  Task.succeed (List.map (\p -> p.title) model.posts == [Text.NonEmpty "100% done"])
}
EOF
edited app/Posts/Index.alder
--- a/app/Posts/Index.alder
+++ b/app/Posts/Index.alder
@@ -113,8 +113,8 @@
 
 test "a `%` in the query is literal" = {
   let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
-  let _ <- Db.insert Post { slug: Post.Slug "done", title: Text.NonEmpty "100% done", body: "", author: author }
-  let _ <- Db.insert Post { slug: Post.Slug "ways", title: Text.NonEmpty "1000 ways", body: "", author: author }
+  let _ <- Db.insert Post { slug: Post.Slug "done", title: Text.NonEmpty "100% done", body: "", author: author, published: True }
+  let _ <- Db.insert Post { slug: Post.Slug "ways", title: Text.NonEmpty "1000 ways", body: "", author: author, published: True }
   let model <- load { page: 1, q: "100%" } { user: Nothing }
   Task.succeed (List.map (\p -> p.title) model.posts == [Text.NonEmpty "100% done"])
 }
rewriting a test: `alder test --mutate` is the check that it still tests something
[exit 0]
$ alder fill --decl 'Posts.Index."page 99 is empty"' - <<'EOF'
{
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Task.all (List.map (\n -> Db.insert Post { slug: Post.Slug ("post-" ++ Text.fromInt n), title: Text.NonEmpty "A post", body: "", author: author, published: True }) (List.range 1 11))
  let pages <- Query.all (List.map (\n -> load { page: n, q: "" } { user: Nothing }) [1, 2, 99, 0])
  let seen = List.map (\m -> { page: m.page, posts: List.length m.posts, hasMore: m.hasMore }) pages
  Task.succeed (seen == [{ page: 1, posts: 10, hasMore: True }, { page: 2, posts: 1, hasMore: False }, { page: 99, posts: 0, hasMore: False }, { page: 1, posts: 10, hasMore: True }])
}
EOF
edited app/Posts/Index.alder
--- a/app/Posts/Index.alder
+++ b/app/Posts/Index.alder
@@ -121,7 +121,7 @@
 
 test "page 99 is empty" = {
   let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
-  let _ <- Task.all (List.map (\n -> Db.insert Post { slug: Post.Slug ("post-" ++ Text.fromInt n), title: Text.NonEmpty "A post", body: "", author: author }) (List.range 1 11))
+  let _ <- Task.all (List.map (\n -> Db.insert Post { slug: Post.Slug ("post-" ++ Text.fromInt n), title: Text.NonEmpty "A post", body: "", author: author, published: True }) (List.range 1 11))
   let pages <- Query.all (List.map (\n -> load { page: n, q: "" } { user: Nothing }) [1, 2, 99, 0])
   let seen = List.map (\m -> { page: m.page, posts: List.length m.posts, hasMore: m.hasMore }) pages
   Task.succeed (seen == [{ page: 1, posts: 10, hasMore: True }, { page: 2, posts: 1, hasMore: False }, { page: 99, posts: 0, hasMore: False }, { page: 1, posts: 10, hasMore: True }])
rewriting a test: `alder test --mutate` is the check that it still tests something
[exit 0]
$ alder fill --decl 'Posts.Index."a `_` in the query is literal"' - <<'EOF'
{
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "snake", title: Text.NonEmpty "snake_case", body: "", author: author, published: True }
  let _ <- Db.insert Post { slug: Post.Slug "camel", title: Text.NonEmpty "snakeYcase", body: "", author: author, published: True }
  let model <- load { page: 1, q: "e_c" } { user: Nothing }
  Task.succeed (List.map (\p -> p.title) model.posts == [Text.NonEmpty "snake_case"])
}
EOF
edited app/Posts/Index.alder
--- a/app/Posts/Index.alder
+++ b/app/Posts/Index.alder
@@ -134,8 +134,8 @@
 
 test "a `_` in the query is literal" = {
   let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
-  let _ <- Db.insert Post { slug: Post.Slug "snake", title: Text.NonEmpty "snake_case", body: "", author: author }
-  let _ <- Db.insert Post { slug: Post.Slug "camel", title: Text.NonEmpty "snakeYcase", body: "", author: author }
+  let _ <- Db.insert Post { slug: Post.Slug "snake", title: Text.NonEmpty "snake_case", body: "", author: author, published: True }
+  let _ <- Db.insert Post { slug: Post.Slug "camel", title: Text.NonEmpty "snakeYcase", body: "", author: author, published: True }
   let model <- load { page: 1, q: "e_c" } { user: Nothing }
   Task.succeed (List.map (\p -> p.title) model.posts == [Text.NonEmpty "snake_case"])
 }
rewriting a test: `alder test --mutate` is the check that it still tests something
[exit 0]
$ alder fill --decl 'Posts.Index."the second page links back to the first"' - <<'EOF'
{
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Task.all (List.map (\n -> Db.insert Post { slug: Post.Slug ("post-" ++ Text.fromInt n), title: Text.NonEmpty "A post", body: "", author: author, published: True }) (List.range 1 11))
  let first <- Test.get (Posts.Index {})
  let second <- Test.get (Posts.Index { page: 2 })
  Task.succeed ({ firstPosts: Test.count "ul.posts li" first.doc, next: Test.attr "href" "a[rel=next]" first.doc, secondPosts: Test.count "ul.posts li" second.doc, prev: Test.attr "href" "a[rel=prev]" second.doc, more: Test.has "a[rel=next]" second.doc } == { firstPosts: 10, next: Just "/?page=2", secondPosts: 1, prev: Just "/", more: False })
}
EOF
edited app/Posts/Index.alder
--- a/app/Posts/Index.alder
+++ b/app/Posts/Index.alder
@@ -165,7 +165,7 @@
 -- user posts it (book 10, "Testing through the browser").
 test "the second page links back to the first" = {
   let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
-  let _ <- Task.all (List.map (\n -> Db.insert Post { slug: Post.Slug ("post-" ++ Text.fromInt n), title: Text.NonEmpty "A post", body: "", author: author }) (List.range 1 11))
+  let _ <- Task.all (List.map (\n -> Db.insert Post { slug: Post.Slug ("post-" ++ Text.fromInt n), title: Text.NonEmpty "A post", body: "", author: author, published: True }) (List.range 1 11))
   let first <- Test.get (Posts.Index {})
   let second <- Test.get (Posts.Index { page: 2 })
   Task.succeed ({ firstPosts: Test.count "ul.posts li" first.doc, next: Test.attr "href" "a[rel=next]" first.doc, secondPosts: Test.count "ul.posts li" second.doc, prev: Test.attr "href" "a[rel=prev]" second.doc, more: Test.has "a[rel=next]" second.doc } == { firstPosts: 10, next: Just "/?page=2", secondPosts: 1, prev: Just "/", more: False })
rewriting a test: `alder test --mutate` is the check that it still tests something
[exit 0]
$ alder fill --decl 'Posts.Index."a search through the form"' - <<'EOF'
{
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "soup", title: Text.NonEmpty "Soup", body: "Use ripe tomatoes.", author: author, published: True }
  let _ <- Db.insert Post { slug: Post.Slug "trains", title: Text.NonEmpty "Trains", body: "Fast ones.", author: author, published: True }
  let _ <- Test.get (Posts.Index {})
  let r <- Test.submit SearchForm { q: "tomato" }
  Task.succeed ({ answer: r.answer, titles: Test.texts "ul.posts a" r.doc, query: Test.attr "value" "input[name=q]" r.doc } == { answer: Redirected (Posts.Index { page: 1, q: "tomato" }), titles: ["Soup"], query: Just "tomato" })
}
EOF
edited app/Posts/Index.alder
--- a/app/Posts/Index.alder
+++ b/app/Posts/Index.alder
@@ -173,8 +173,8 @@
 
 test "a search through the form" = {
   let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
-  let _ <- Db.insert Post { slug: Post.Slug "soup", title: Text.NonEmpty "Soup", body: "Use ripe tomatoes.", author: author }
-  let _ <- Db.insert Post { slug: Post.Slug "trains", title: Text.NonEmpty "Trains", body: "Fast ones.", author: author }
+  let _ <- Db.insert Post { slug: Post.Slug "soup", title: Text.NonEmpty "Soup", body: "Use ripe tomatoes.", author: author, published: True }
+  let _ <- Db.insert Post { slug: Post.Slug "trains", title: Text.NonEmpty "Trains", body: "Fast ones.", author: author, published: True }
   let _ <- Test.get (Posts.Index {})
   let r <- Test.submit SearchForm { q: "tomato" }
   Task.succeed ({ answer: r.answer, titles: Test.texts "ul.posts a" r.doc, query: Test.attr "value" "input[name=q]" r.doc } == { answer: Redirected (Posts.Index { page: 1, q: "tomato" }), titles: ["Soup"], query: Just "tomato" })
rewriting a test: `alder test --mutate` is the check that it still tests something
[exit 0]
$ alder fill --decl 'Posts.Show."a post'"'"'s body is rendered as Markdown"' - <<'EOF'
{
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "Not *yet*.", author: author, published: True }
  let m <- load { slug: Post.Slug "hello" } { user: Nothing }
  Task.succeed ({ title: m.post.title, body: m.body } == { title: Text.NonEmpty "Hello", body: <p>Not <em>yet</em>.</p> })
}
EOF
edited app/Posts/Show.alder
--- a/app/Posts/Show.alder
+++ b/app/Posts/Show.alder
@@ -81,7 +81,7 @@
 -- The body goes through the real Markdown adapter (adapters/markdown/markdown.go).
 test "a post's body is rendered as Markdown" = {
   let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
-  let _ <- Db.insert Post { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "Not *yet*.", author: author }
+  let _ <- Db.insert Post { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "Not *yet*.", author: author, published: True }
   let m <- load { slug: Post.Slug "hello" } { user: Nothing }
   Task.succeed ({ title: m.post.title, body: m.body } == { title: Text.NonEmpty "Hello", body: <p>Not <em>yet</em>.</p> })
 }
rewriting a test: `alder test --mutate` is the check that it still tests something
[exit 0]
$ alder fill --decl 'Posts.Show."a post'"'"'s body is rendered through the Markdown capability"' - <<'EOF'
{
  let _ <- Test.fake Markdown
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "Not *yet*.", author: author, published: True }
  let m <- load { slug: Post.Slug "hello" } { user: Nothing }
  let steps <- Test.trace
  Task.succeed ({ body: m.body, rendered: List.member "Markdown.render" steps } == { body: Html.text "Not *yet*.", rendered: True })
}
EOF
edited app/Posts/Show.alder
--- a/app/Posts/Show.alder
+++ b/app/Posts/Show.alder
@@ -91,7 +91,7 @@
 test "a post's body is rendered through the Markdown capability" = {
   let _ <- Test.fake Markdown
   let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
-  let _ <- Db.insert Post { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "Not *yet*.", author: author }
+  let _ <- Db.insert Post { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "Not *yet*.", author: author, published: True }
   let m <- load { slug: Post.Slug "hello" } { user: Nothing }
   let steps <- Test.trace
   Task.succeed ({ body: m.body, rendered: List.member "Markdown.render" steps } == { body: Html.text "Not *yet*.", rendered: True })
rewriting a test: `alder test --mutate` is the check that it still tests something
[exit 0]
$ alder fill --decl 'Posts.Show."a comment is listed oldest first"' - <<'EOF'
{
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "", author: author, published: True }
  let before <- load { slug: Post.Slug "hello" } { user: Nothing }
  let _ <- update (AddComment { form: { author: Comment.Name "Ann", body: Comment.Body "First!" } }) before { user: Nothing }
  let _ <- update (AddComment { form: { author: Comment.Name "Bob", body: Comment.Body "Second." } }) before { user: Nothing }
  let after <- load { slug: Post.Slug "hello" } { user: Nothing }
  Task.succeed (List.map (\c -> c.body) after.comments == [Comment.Body "First!", Comment.Body "Second."])
}
EOF
edited app/Posts/Show.alder
--- a/app/Posts/Show.alder
+++ b/app/Posts/Show.alder
@@ -99,7 +99,7 @@
 
 test "a comment is listed oldest first" = {
   let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
-  let _ <- Db.insert Post { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "", author: author }
+  let _ <- Db.insert Post { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "", author: author, published: True }
   let before <- load { slug: Post.Slug "hello" } { user: Nothing }
   let _ <- update (AddComment { form: { author: Comment.Name "Ann", body: Comment.Body "First!" } }) before { user: Nothing }
   let _ <- update (AddComment { form: { author: Comment.Name "Bob", body: Comment.Body "Second." } }) before { user: Nothing }
rewriting a test: `alder test --mutate` is the check that it still tests something
[exit 0]
$ alder fill --decl 'Posts.Show."an empty comment is rejected"' - <<'EOF'
{
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "", author: author, published: True }
  let _ <- Test.get (Posts.Show { slug: Post.Slug "hello" })
  let r <- Test.submitRaw NewComment [{ name: "author", value: "Ann" }, { name: "body", value: "" }]
  let comments <- Db.from Comment |> Db.count
  Task.succeed ({ answer: r.answer, author: Test.attr "value" "input[name=author]" r.doc, comments: comments } == { answer: FormRejected [{ field: "body", message: "is not valid" }], author: Just "Ann", comments: 0 })
}
EOF
edited app/Posts/Show.alder
--- a/app/Posts/Show.alder
+++ b/app/Posts/Show.alder
@@ -111,7 +111,7 @@
 -- error beside the field, and no comment is stored.
 test "an empty comment is rejected" = {
   let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
-  let _ <- Db.insert Post { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "", author: author }
+  let _ <- Db.insert Post { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "", author: author, published: True }
   let _ <- Test.get (Posts.Show { slug: Post.Slug "hello" })
   let r <- Test.submitRaw NewComment [{ name: "author", value: "Ann" }, { name: "body", value: "" }]
   let comments <- Db.from Comment |> Db.count
rewriting a test: `alder test --mutate` is the check that it still tests something
[exit 0]
$ alder new test Posts.Index "a draft is not listed or found"
edited app/Posts/Index.alder
--- a/app/Posts/Index.alder
+++ b/app/Posts/Index.alder
@@ -179,3 +179,7 @@
   let r <- Test.submit SearchForm { q: "tomato" }
   Task.succeed ({ answer: r.answer, titles: Test.texts "ul.posts a" r.doc, query: Test.attr "value" "input[name=q]" r.doc } == { answer: Redirected (Posts.Index { page: 1, q: "tomato" }), titles: ["Soup"], query: Just "tomato" })
 }
+
+test "a draft is not listed or found" = {
+  ?test
+}
app/Posts/Index.alder:184:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Posts.Index."a draft is not listed or found"' - <<'EOF'
{
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "draft", title: Text.NonEmpty "Tomato draft", body: "", author: author }
  let _ <- Db.insert Post { slug: Post.Slug "live", title: Text.NonEmpty "Live", body: "", author: author, published: True }
  let all <- load { page: 1, q: "" } { user: Nothing }
  let found <- load { page: 1, q: "tomato" } { user: Nothing }
  Task.succeed ({ all: List.map (\p -> p.title) all.posts, found: List.length found.posts } == { all: [Text.NonEmpty "Live"], found: 0 })
}
EOF
edited app/Posts/Index.alder
--- a/app/Posts/Index.alder
+++ b/app/Posts/Index.alder
@@ -181,5 +181,10 @@
 }
 
 test "a draft is not listed or found" = {
-  ?test
+  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
+  let _ <- Db.insert Post { slug: Post.Slug "draft", title: Text.NonEmpty "Tomato draft", body: "", author: author }
+  let _ <- Db.insert Post { slug: Post.Slug "live", title: Text.NonEmpty "Live", body: "", author: author, published: True }
+  let all <- load { page: 1, q: "" } { user: Nothing }
+  let found <- load { page: 1, q: "tomato" } { user: Nothing }
+  Task.succeed ({ all: List.map (\p -> p.title) all.posts, found: List.length found.posts } == { all: [Text.NonEmpty "Live"], found: 0 })
 }
[exit 0]
$ alder new test Posts.Show "a draft is not found by a reader but by its author"
edited app/Posts/Show.alder
--- a/app/Posts/Show.alder
+++ b/app/Posts/Show.alder
@@ -117,3 +117,7 @@
   let comments <- Db.from Comment |> Db.count
   Task.succeed ({ answer: r.answer, author: Test.attr "value" "input[name=author]" r.doc, comments: comments } == { answer: FormRejected [{ field: "body", message: "is not valid" }], author: Just "Ann", comments: 0 })
 }
+
+test "a draft is not found by a reader but by its author" = {
+  ?test
+}
app/Posts/Show.alder:122:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Posts.Show."a draft is not found by a reader but by its author"' - <<'EOF'
{
  let _ <- Test.fake Markdown
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let reader <- Db.insert User { email: Text.Email "reader@example.com", name: Text.NonEmpty "Reader", passwordHash: "" }
  let _ <- Db.insert Post { slug: Post.Slug "secret", title: Text.NonEmpty "Secret", body: "", author: author }
  let asVisitor <- Test.open (Posts.Show { slug: Post.Slug "secret" })
  let _ <- Test.login { user: Just reader }
  let asReader <- Test.open (Posts.Show { slug: Post.Slug "secret" })
  let _ <- Test.login { user: Just author }
  let asAuthor <- Test.open (Posts.Show { slug: Post.Slug "secret" })
  Task.succeed ({ visitor: asVisitor.answer, reader: asReader.answer, author: asAuthor.answer } == { visitor: PageMissing, reader: PageMissing, author: Shown })
}
EOF
edited app/Posts/Show.alder
--- a/app/Posts/Show.alder
+++ b/app/Posts/Show.alder
@@ -119,5 +119,14 @@
 }
 
 test "a draft is not found by a reader but by its author" = {
-  ?test
+  let _ <- Test.fake Markdown
+  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
+  let reader <- Db.insert User { email: Text.Email "reader@example.com", name: Text.NonEmpty "Reader", passwordHash: "" }
+  let _ <- Db.insert Post { slug: Post.Slug "secret", title: Text.NonEmpty "Secret", body: "", author: author }
+  let asVisitor <- Test.open (Posts.Show { slug: Post.Slug "secret" })
+  let _ <- Test.login { user: Just reader }
+  let asReader <- Test.open (Posts.Show { slug: Post.Slug "secret" })
+  let _ <- Test.login { user: Just author }
+  let asAuthor <- Test.open (Posts.Show { slug: Post.Slug "secret" })
+  Task.succeed ({ visitor: asVisitor.answer, reader: asReader.answer, author: asAuthor.answer } == { visitor: PageMissing, reader: PageMissing, author: Shown })
 }
[exit 0]
$ alder new test Admin "publishing a draft puts it on the front page"
edited app/Admin.alder
--- a/app/Admin.alder
+++ b/app/Admin.alder
@@ -169,3 +169,7 @@
   let after <- Db.get Post post1
   Task.succeed ?expected
 }
+
+test "publishing a draft puts it on the front page" = {
+  ?test
+}
app/Admin.alder:170:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Admin.alder:174:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Admin."publishing a draft puts it on the front page"' - <<'EOF'
{
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let id <- Db.insert Post { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "", author: author }
  let _ <- Test.login { user: Just author }
  let b <- Test.get (Posts.Index {})
  let d <- Test.get (Admin {})
  let r <- Test.click (Publish { id: id })
  let a <- Test.get (Posts.Index {})
  Task.succeed ({ before: Test.count "ul.posts li" b.doc, drafts: Test.texts "#drafts .title" d.doc, answer: r.answer, after: Test.texts "ul.posts a" a.doc } == { before: 0, drafts: ["Hello"], answer: Redirected (Admin {}), after: ["Hello"] })
}
EOF
edited app/Admin.alder
--- a/app/Admin.alder
+++ b/app/Admin.alder
@@ -171,5 +171,12 @@
 }
 
 test "publishing a draft puts it on the front page" = {
-  ?test
+  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
+  let id <- Db.insert Post { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "", author: author }
+  let _ <- Test.login { user: Just author }
+  let b <- Test.get (Posts.Index {})
+  let d <- Test.get (Admin {})
+  let r <- Test.click (Publish { id: id })
+  let a <- Test.get (Posts.Index {})
+  Task.succeed ({ before: Test.count "ul.posts li" b.doc, drafts: Test.texts "#drafts .title" d.doc, answer: r.answer, after: Test.texts "ul.posts a" a.doc } == { before: 0, drafts: ["Hello"], answer: Redirected (Admin {}), after: ["Hello"] })
 }
app/Admin.alder:170:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder test
app/Admin.alder:170:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
alder test: no tests ran (fill the hole): ?expected : Bool
[exit 1]
```

The harness stopped at line 237 of your script: `alder test`: it exited 1. The commands after it were not run.

Your script stopped at command 23; the app is as the commands before it left it; continue.

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Admin.alder:170:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
alder build: the program has 1 typed hole(s); fill them before building
```
