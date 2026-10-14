### Round 2

```text
$ alder fill --decl Admin.load - <<'EOF'
{
  let me <- User.author session.user
  match me {
    Just user -> {
      let posts <-
        Db.from Post
          |> Db.where (Post.author.eq user.id)
          |> Db.where (Post.published.eq False)
          |> Db.orderBy Post.created.desc
          |> Db.all
      Query.succeed { author: user, posts: posts }
    },
    Nothing -> Query.notFound,
  }
}
EOF
edited app/Admin.alder
--- a/app/Admin.alder
+++ b/app/Admin.alder
@@ -22,6 +22,7 @@
       let posts <-
         Db.from Post
           |> Db.where (Post.author.eq user.id)
+          |> Db.where (Post.published.eq False)
           |> Db.orderBy Post.created.desc
           |> Db.all
       Query.succeed { author: user, posts: posts }
app/Admin.alder:56:16: warning[W0002]: hole ?publish has type Task Nav
    Publish p -> ?publish,
                 ^^^^^^^^
app/Admin.alder:85:37: warning[W0002]: hole ?publishId has type Post.PostId
      <al-button click={Publish { id: ?publishId }}>Publish</al-button>
                                      ^^^^^^^^^^
app/Admin.alder:150:16: warning[W0002]: hole ?expected has type Bool
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
@@ -53,7 +53,18 @@
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
app/Admin.alder:96:37: warning[W0002]: hole ?publishId has type Post.PostId
      <al-button click={Publish { id: ?publishId }}>Publish</al-button>
                                      ^^^^^^^^^^
app/Admin.alder:161:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill --decl Admin.view - <<'EOF'
<Layout.Page title="Your posts">
    <h1>Your posts</h1>

    <h2>Drafts</h2>
    <ul id="drafts" class="posts">
      {for post in model.posts ->
        <li>
          <span class="title">{post.title}</span>
          <a href={Admin.Edit { id: post.id }}>Edit</a>
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
@@ -70,31 +70,32 @@
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
+      <ul id="drafts" class="posts">
+        {for post in model.posts ->
+          <li>
+            <span class="title">{post.title}</span>
+            <a href={Admin.Edit { id: post.id }}>Edit</a>
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
app/Admin.alder:162:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill --decl 'Admin."pressing Publish"' - <<'EOF'
{
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let id <- Db.insert Post { slug: Post.Slug "draft", title: Text.NonEmpty "Draft", body: "", author: author }
  let _ <- Test.login { user: Just author }
  let _ <- Test.get (Admin {})
  let r <- Test.click (Publish { id: id })
  let after <- Db.get Post id
  let front <- Test.get (Posts.Index {})
  Task.succeed ({ answer: r.answer, published: Maybe.map (\p -> p.published) after, titles: Test.texts "ul.posts a" front.doc } == { answer: Redirected (Admin {}), published: Just True, titles: ["Draft"] })
}
EOF
edited app/Admin.alder
--- a/app/Admin.alder
+++ b/app/Admin.alder
@@ -149,15 +149,13 @@
   Task.succeed ({ heading: Test.text "h1" page.doc, out: out.answer, user: out.session.user } == { heading: Just "Your posts", out: Redirected (Auth.Login {}), user: Nothing })
 }
 
--- Fill ?expected with what pressing Publish must do, so that a test fails when the arm
--- does something else: its answer (`r.answer`: Rerendered, Redirected …) and what it
--- changed (`after`, the row read again), as `{ answer: r.answer, … } == { answer: Rerendered, … }`.
 test "pressing Publish" = {
-  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
-  let _ <- Test.login { user: Just me }
-  let post1 <- Db.insert Post { slug: Post.Slug "slug-1", title: Text.NonEmpty "title-1", body: "body-1", author: me }
+  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
+  let id <- Db.insert Post { slug: Post.Slug "draft", title: Text.NonEmpty "Draft", body: "", author: author }
+  let _ <- Test.login { user: Just author }
   let _ <- Test.get (Admin {})
-  let r <- Test.click (Publish { id: post1 })
-  let after <- Db.get Post post1
-  Task.succeed ?expected
+  let r <- Test.click (Publish { id: id })
+  let after <- Db.get Post id
+  let front <- Test.get (Posts.Index {})
+  Task.succeed ({ answer: r.answer, published: Maybe.map (\p -> p.published) after, titles: Test.texts "ul.posts a" front.doc } == { answer: Redirected (Admin {}), published: Just True, titles: ["Draft"] })
 }
rewriting a test: `alder test --mutate` is the check that it still tests something
[exit 0]
$ alder new test Admin "a draft of another author cannot be published"
edited app/Admin.alder
--- a/app/Admin.alder
+++ b/app/Admin.alder
@@ -159,3 +159,7 @@
   let front <- Test.get (Posts.Index {})
   Task.succeed ({ answer: r.answer, published: Maybe.map (\p -> p.published) after, titles: Test.texts "ul.posts a" front.doc } == { answer: Redirected (Admin {}), published: Just True, titles: ["Draft"] })
 }
+
+test "a draft of another author cannot be published" = {
+  ?test
+}
app/Admin.alder:164:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Admin."a draft of another author cannot be published"' - <<'EOF'
{
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "", role: Author }
  let id <- Db.insert Post { slug: Post.Slug "draft", title: Text.NonEmpty "Draft", body: "", author: other }
  let model <- load {} { user: author }
  let r <- Test.run (update (Publish { id: id }) model { user: author })
  let after <- Db.get Post id
  Task.succeed ({ value: r.value, published: Maybe.map (\p -> p.published) after } == { value: Err "not found", published: Just False })
}
EOF
edited app/Admin.alder
--- a/app/Admin.alder
+++ b/app/Admin.alder
@@ -161,5 +161,11 @@
 }
 
 test "a draft of another author cannot be published" = {
-  ?test
+  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
+  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "", role: Author }
+  let id <- Db.insert Post { slug: Post.Slug "draft", title: Text.NonEmpty "Draft", body: "", author: other }
+  let model <- load {} { user: author }
+  let r <- Test.run (update (Publish { id: id }) model { user: author })
+  let after <- Db.get Post id
+  Task.succeed ({ value: r.value, published: Maybe.map (\p -> p.published) after } == { value: Err "not found", published: Just False })
 }
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
$ alder new test Posts.Index "a draft is neither listed nor found"
edited app/Posts/Index.alder
--- a/app/Posts/Index.alder
+++ b/app/Posts/Index.alder
@@ -179,3 +179,7 @@
   let r <- Test.submit SearchForm { q: "tomato" }
   Task.succeed ({ answer: r.answer, titles: Test.texts "ul.posts a" r.doc, query: Test.attr "value" "input[name=q]" r.doc } == { answer: Redirected (Posts.Index { page: 1, q: "tomato" }), titles: ["Soup"], query: Just "tomato" })
 }
+
+test "a draft is neither listed nor found" = {
+  ?test
+}
app/Posts/Index.alder:184:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Posts.Index."a draft is neither listed nor found"' - <<'EOF'
{
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "draft", title: Text.NonEmpty "Secret draft", body: "Tomato season.", author: author }
  let _ <- Db.insert Post { slug: Post.Slug "live", title: Text.NonEmpty "Live post", body: "", author: author, published: True }
  let front <- load { page: 1, q: "" } { user: Nothing }
  let found <- load { page: 1, q: "tomato" } { user: Nothing }
  Task.succeed ({ front: List.map (\p -> p.title) front.posts, found: List.length found.posts } == { front: [Text.NonEmpty "Live post"], found: 0 })
}
EOF
edited app/Posts/Index.alder
--- a/app/Posts/Index.alder
+++ b/app/Posts/Index.alder
@@ -181,5 +181,10 @@
 }
 
 test "a draft is neither listed nor found" = {
-  ?test
+  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
+  let _ <- Db.insert Post { slug: Post.Slug "draft", title: Text.NonEmpty "Secret draft", body: "Tomato season.", author: author }
+  let _ <- Db.insert Post { slug: Post.Slug "live", title: Text.NonEmpty "Live post", body: "", author: author, published: True }
+  let front <- load { page: 1, q: "" } { user: Nothing }
+  let found <- load { page: 1, q: "tomato" } { user: Nothing }
+  Task.succeed ({ front: List.map (\p -> p.title) front.posts, found: List.length found.posts } == { front: [Text.NonEmpty "Live post"], found: 0 })
 }
[exit 0]
$ alder fill --decl Posts.Show.load - <<'EOF'
{
  let found <- Post.bySlug params.slug
  match found {
    Just post -> {
      let mine = session.user == Just post.author
      if post.published || mine then {
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
alder fill: `alder fill --decl` takes one expression; standard input does not parse as one: expected `}`, got `,`; the last expression of a block is its result and takes no `,` (line 14, column 28)
[exit 1]
```

The harness stopped at line 179 of your script: `alder fill --decl Posts.Show.load - <<'EOF'`: it exited 1. The commands after it were not run.

Your script stopped at command 16; the app is as the commands before it left it; continue.

The harness ran `alder check`, `alder build` and `alder test` on the app as the commands left it: all pass, but the round does not: the script stopped before its end.
