### Round 1

```text
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
$ alder fill --decl 'Admin."pressing Publish"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Test.login { user: Just me }
  let post1 <- Db.insert Post { slug: Post.Slug "slug-1", title: Text.NonEmpty "title-1", body: "body-1", author: me }
  let _ <- Test.get (Admin {})
  let r <- Test.click (Publish { id: post1 })
  let after <- Db.get Post post1
  Task.succeed ({ answer: r.answer, published: Maybe.map (\p -> p.published) after } == { answer: Redirected (Admin {}), published: Just True })
}
EOF
edited app/Admin.alder
--- a/app/Admin.alder
+++ b/app/Admin.alder
@@ -147,15 +147,12 @@
   Task.succeed ({ heading: Test.text "h1" page.doc, out: out.answer, user: out.session.user } == { heading: Just "Your posts", out: Redirected (Auth.Login {}), user: Nothing })
 }
 
--- Fill ?expected with what pressing Publish must do, so that a test fails when the arm
--- does something else: its answer (`r.answer`: Rerendered, Redirected …) and what it
--- changed (`after`, the row read again), as `{ answer: r.answer, … } == { answer: Rerendered, … }`.
 test "pressing Publish" = {
-  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
+  let me <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
   let _ <- Test.login { user: Just me }
   let post1 <- Db.insert Post { slug: Post.Slug "slug-1", title: Text.NonEmpty "title-1", body: "body-1", author: me }
   let _ <- Test.get (Admin {})
   let r <- Test.click (Publish { id: post1 })
   let after <- Db.get Post post1
-  Task.succeed ?expected
+  Task.succeed ({ answer: r.answer, published: Maybe.map (\p -> p.published) after } == { answer: Redirected (Admin {}), published: Just True })
 }
rewriting a test: `alder test --mutate` is the check that it still tests something
[exit 0]
$ alder new test Admin "publishing a draft puts it on the front page"
edited app/Admin.alder
--- a/app/Admin.alder
+++ b/app/Admin.alder
@@ -156,3 +156,7 @@
   let after <- Db.get Post post1
   Task.succeed ({ answer: r.answer, published: Maybe.map (\p -> p.published) after } == { answer: Redirected (Admin {}), published: Just True })
 }
+
+test "publishing a draft puts it on the front page" = {
+  ?test
+}
app/Admin.alder:161:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Admin."publishing a draft puts it on the front page"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let id <- Db.insert Post { slug: Post.Slug "draft", title: Text.NonEmpty "Draft", body: "", author: me }
  let _ <- Test.login { user: Just me }
  let before <- Test.get (Posts.Index {})
  let _ <- Test.get (Admin {})
  let _ <- Test.click (Publish { id: id })
  let after <- Test.get (Posts.Index {})
  Task.succeed ({ before: Test.texts "ul.posts a" before.doc, after: Test.texts "ul.posts a" after.doc } == { before: [], after: ["Draft"] })
}
EOF
edited app/Admin.alder
--- a/app/Admin.alder
+++ b/app/Admin.alder
@@ -158,5 +158,12 @@
 }
 
 test "publishing a draft puts it on the front page" = {
-  ?test
+  let me <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
+  let id <- Db.insert Post { slug: Post.Slug "draft", title: Text.NonEmpty "Draft", body: "", author: me }
+  let _ <- Test.login { user: Just me }
+  let before <- Test.get (Posts.Index {})
+  let _ <- Test.get (Admin {})
+  let _ <- Test.click (Publish { id: id })
+  let after <- Test.get (Posts.Index {})
+  Task.succeed ({ before: Test.texts "ul.posts a" before.doc, after: Test.texts "ul.posts a" after.doc } == { before: [], after: ["Draft"] })
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
$ alder new test Posts.Index "a draft is not on the front page or in a search"
edited app/Posts/Index.alder
--- a/app/Posts/Index.alder
+++ b/app/Posts/Index.alder
@@ -179,3 +179,7 @@
   let r <- Test.submit SearchForm { q: "tomato" }
   Task.succeed ({ answer: r.answer, titles: Test.texts "ul.posts a" r.doc, query: Test.attr "value" "input[name=q]" r.doc } == { answer: Redirected (Posts.Index { page: 1, q: "tomato" }), titles: ["Soup"], query: Just "tomato" })
 }
+
+test "a draft is not on the front page or in a search" = {
+  ?test
+}
app/Posts/Index.alder:184:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Posts.Index."a draft is not on the front page or in a search"' - <<'EOF'
{
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "draft", title: Text.NonEmpty "Secret plans", body: "", author: author }
  let _ <- Db.insert Post { slug: Post.Slug "live", title: Text.NonEmpty "Open plans", body: "", author: author, published: True }
  let front <- load { page: 1, q: "" } { user: Nothing }
  let found <- load { page: 1, q: "plans" } { user: Nothing }
  let frontTitles = List.map (\p -> p.title) front.posts
  let hits = List.map (\p -> p.title) found.posts
  Task.succeed ({ front: frontTitles, search: hits } == { front: [Text.NonEmpty "Open plans"], search: [Text.NonEmpty "Open plans"] })
}
EOF
edited app/Posts/Index.alder
--- a/app/Posts/Index.alder
+++ b/app/Posts/Index.alder
@@ -181,5 +181,12 @@
 }
 
 test "a draft is not on the front page or in a search" = {
-  ?test
+  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
+  let _ <- Db.insert Post { slug: Post.Slug "draft", title: Text.NonEmpty "Secret plans", body: "", author: author }
+  let _ <- Db.insert Post { slug: Post.Slug "live", title: Text.NonEmpty "Open plans", body: "", author: author, published: True }
+  let front <- load { page: 1, q: "" } { user: Nothing }
+  let found <- load { page: 1, q: "plans" } { user: Nothing }
+  let frontTitles = List.map (\p -> p.title) front.posts
+  let hits = List.map (\p -> p.title) found.posts
+  Task.succeed ({ front: frontTitles, search: hits } == { front: [Text.NonEmpty "Open plans"], search: [Text.NonEmpty "Open plans"] })
 }
[exit 0]
```

The harness refused line 136 of your script: `alder fill --decl "Posts.Show.\"a post's body is rendered as Markdown\"" - <<'EOF'`: an unterminated '. Nothing after it was run.

Your script stopped at command 14; the app is as the commands before it left it; continue.

The program builds but its tests fail; fix the program or the tests so every test passes.

```text
ok   Post "a slug is lowercase letters, digits and hyphens"
ok   Admin "a Reader cannot open the admin page"
ok   Admin "creating a post redirects to its edit page"
ok   Admin "a taken slug is refused"
ok   Admin "the admin page needs an Author"
ok   Admin "an Author logs out from the admin page"
ok   Admin "pressing Publish"
ok   Admin "publishing a draft puts it on the front page"
ok   Comment "a name or comment has text and a bound"
ok   Admin.Edit "deleting a post deletes its comments"
ok   Admin.Edit "another author's post is not found"
ok   Admin.Edit "a demoted author cannot edit"
ok   Posts.Index "the front page lists the posts, newest first"
ok   Posts.Index "search matches title or body"
ok   Posts.Index "a `%` in the query is literal"
ok   Posts.Index "page 99 is empty"
ok   Posts.Index "a negative page is page 1"
ok   Posts.Index "a `_` in the query is literal"
ok   Posts.Index "a long query is cut"
ok   Posts.Index "a search answers with the first page of its results"
ok   Posts.Index "only an Author sees the Write link"
ok   Posts.Index "the second page links back to the first"
ok   Posts.Index "a search through the form"
ok   Posts.Index "a draft is not on the front page or in a search"
FAIL Posts.Show "a post's body is rendered as Markdown"
  error: not found
  trace: ["Db.insert User", "Db.insert Post", "Db.first Post"]
FAIL Posts.Show "a post's body is rendered through the Markdown capability"
  error: not found
  trace: ["Test.fake Markdown", "Db.insert User", "Db.insert Post", "Db.first Post"]
FAIL Posts.Show "a comment is listed oldest first"
  error: not found
  trace: ["Db.insert User", "Db.insert Post", "Db.first Post"]
FAIL Posts.Show "an empty comment is rejected"
  error: GET /posts/hello: not found
  trace: ["Db.insert User", "Db.insert Post", "Test.get Posts.Show", "Db.first Post"]
24 passed, 4 failed
```
