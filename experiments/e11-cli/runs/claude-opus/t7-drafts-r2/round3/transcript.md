### Round 3

```text
$ alder fill --decl 'Admin."pressing Publish"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Test.login { user: Just me }
  let post1 <- Db.insert Post { slug: Post.Slug "draft", title: Text.NonEmpty "Draft", body: "", author: me }
  let before <- Test.get (Admin {})
  let r <- Test.click (Publish { id: post1 })
  let after <- Db.get Post post1
  let front <- Test.get (Posts.Index {})
  Task.succeed ({ drafts: Test.texts "#drafts .title" before.doc, answer: r.answer, left: Test.texts "#drafts .title" r.doc, published: Maybe.map (\p -> p.published) after, front: Test.texts "ul.posts a" front.doc } == { drafts: ["Draft"], answer: Redirected (Admin {}), left: [], published: Just True, front: ["Draft"] })
}
EOF
edited app/Admin.alder
--- a/app/Admin.alder
+++ b/app/Admin.alder
@@ -155,15 +155,13 @@
   Task.succeed ({ heading: Test.text "h1" page.doc, out: out.answer, user: out.session.user } == { heading: Just "Your posts", out: Redirected (Auth.Login {}), user: Nothing })
 }
 
--- Fill ?expected with what pressing Publish must do, so that a test fails when the arm
--- does something else: its answer (`r.answer`: Rerendered, Redirected …) and what it
--- changed (`after`, the row read again), as `{ answer: r.answer, … } == { answer: Rerendered, … }`.
 test "pressing Publish" = {
-  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
+  let me <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
   let _ <- Test.login { user: Just me }
-  let post1 <- Db.insert Post { slug: Post.Slug "slug-1", title: Text.NonEmpty "title-1", body: "body-1", author: me }
-  let _ <- Test.get (Admin {})
+  let post1 <- Db.insert Post { slug: Post.Slug "draft", title: Text.NonEmpty "Draft", body: "", author: me }
+  let before <- Test.get (Admin {})
   let r <- Test.click (Publish { id: post1 })
   let after <- Db.get Post post1
-  Task.succeed ?expected
+  let front <- Test.get (Posts.Index {})
+  Task.succeed ({ drafts: Test.texts "#drafts .title" before.doc, answer: r.answer, left: Test.texts "#drafts .title" r.doc, published: Maybe.map (\p -> p.published) after, front: Test.texts "ul.posts a" front.doc } == { drafts: ["Draft"], answer: Redirected (Admin {}), left: [], published: Just True, front: ["Draft"] })
 }
rewriting a test: `alder test --mutate` is the check that it still tests something
[exit 0]
$ alder new test Posts.Index "a draft is not listed and not found by a search"
edited app/Posts/Index.alder
--- a/app/Posts/Index.alder
+++ b/app/Posts/Index.alder
@@ -179,3 +179,7 @@
   let r <- Test.submit SearchForm { q: "tomato" }
   Task.succeed ({ answer: r.answer, titles: Test.texts "ul.posts a" r.doc, query: Test.attr "value" "input[name=q]" r.doc } == { answer: Redirected (Posts.Index { page: 1, q: "tomato" }), titles: ["Soup"], query: Just "tomato" })
 }
+
+test "a draft is not listed and not found by a search" = {
+  ?test
+}
app/Posts/Index.alder:184:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Posts.Index."a draft is not listed and not found by a search"' - <<'EOF'
{
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "soup", title: Text.NonEmpty "Soup", body: "Use ripe tomatoes.", author: author, published: True }
  let _ <- Db.insert Post { slug: Post.Slug "draft", title: Text.NonEmpty "Tomato draft", body: "More tomatoes.", author: author }
  let front <- load { page: 1, q: "" } { user: Nothing }
  let search <- load { page: 1, q: "tomato" } { user: Nothing }
  Task.succeed ({ front: List.map (\p -> p.title) front.posts, search: List.map (\p -> p.title) search.posts } == { front: [Text.NonEmpty "Soup"], search: [Text.NonEmpty "Soup"] })
}
EOF
edited app/Posts/Index.alder
--- a/app/Posts/Index.alder
+++ b/app/Posts/Index.alder
@@ -181,5 +181,10 @@
 }
 
 test "a draft is not listed and not found by a search" = {
-  ?test
+  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
+  let _ <- Db.insert Post { slug: Post.Slug "soup", title: Text.NonEmpty "Soup", body: "Use ripe tomatoes.", author: author, published: True }
+  let _ <- Db.insert Post { slug: Post.Slug "draft", title: Text.NonEmpty "Tomato draft", body: "More tomatoes.", author: author }
+  let front <- load { page: 1, q: "" } { user: Nothing }
+  let search <- load { page: 1, q: "tomato" } { user: Nothing }
+  Task.succeed ({ front: List.map (\p -> p.title) front.posts, search: List.map (\p -> p.title) search.posts } == { front: [Text.NonEmpty "Soup"], search: [Text.NonEmpty "Soup"] })
 }
[exit 0]
$ alder new test Posts.Show "a draft is not found but by its author"
edited app/Posts/Show.alder
--- a/app/Posts/Show.alder
+++ b/app/Posts/Show.alder
@@ -117,3 +117,7 @@
   let comments <- Db.from Comment |> Db.count
   Task.succeed ({ answer: r.answer, author: Test.attr "value" "input[name=author]" r.doc, comments: comments } == { answer: FormRejected [{ field: "body", message: "is not valid" }], author: Just "Ann", comments: 0 })
 }
+
+test "a draft is not found but by its author" = {
+  ?test
+}
app/Posts/Show.alder:122:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Posts.Show."a draft is not found but by its author"' - <<'EOF'
{
  let _ <- Test.fake Markdown
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let reader <- Db.insert User { email: Text.Email "reader@example.com", name: Text.NonEmpty "Reader", passwordHash: "" }
  let _ <- Db.insert Post { slug: Post.Slug "draft", title: Text.NonEmpty "Draft", body: "", author: author }
  let asVisitor <- Test.open (Posts.Show { slug: Post.Slug "draft" })
  let _ <- Test.login { user: Just reader }
  let asReader <- Test.open (Posts.Show { slug: Post.Slug "draft" })
  let _ <- Test.login { user: Just author }
  let asAuthor <- Test.open (Posts.Show { slug: Post.Slug "draft" })
  Task.succeed ({ visitor: asVisitor.answer, reader: asReader.answer, author: asAuthor.answer } == { visitor: PageMissing, reader: PageMissing, author: Shown })
}
EOF
edited app/Posts/Show.alder
--- a/app/Posts/Show.alder
+++ b/app/Posts/Show.alder
@@ -119,5 +119,14 @@
 }
 
 test "a draft is not found but by its author" = {
-  ?test
+  let _ <- Test.fake Markdown
+  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
+  let reader <- Db.insert User { email: Text.Email "reader@example.com", name: Text.NonEmpty "Reader", passwordHash: "" }
+  let _ <- Db.insert Post { slug: Post.Slug "draft", title: Text.NonEmpty "Draft", body: "", author: author }
+  let asVisitor <- Test.open (Posts.Show { slug: Post.Slug "draft" })
+  let _ <- Test.login { user: Just reader }
+  let asReader <- Test.open (Posts.Show { slug: Post.Slug "draft" })
+  let _ <- Test.login { user: Just author }
+  let asAuthor <- Test.open (Posts.Show { slug: Post.Slug "draft" })
+  Task.succeed ({ visitor: asVisitor.answer, reader: asReader.answer, author: asAuthor.answer } == { visitor: PageMissing, reader: PageMissing, author: Shown })
 }
[exit 0]
$ alder test
ok   Post "a slug is lowercase letters, digits and hyphens"
ok   Admin "a Reader cannot open the admin page"
ok   Admin "creating a post redirects to its edit page"
ok   Admin "a taken slug is refused"
ok   Admin "the admin page needs an Author"
ok   Admin "an Author logs out from the admin page"
ok   Admin "pressing Publish"
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
ok   Posts.Index "a draft is not listed and not found by a search"
ok   Posts.Show "a post's body is rendered as Markdown"
FAIL Posts.Show "a post's body is rendered through the Markdown capability"
  error: not found
  trace: ["Test.fake Markdown", "Db.insert User", "Db.insert Post", "Db.first Post"]
FAIL Posts.Show "a comment is listed oldest first"
  error: not found
  trace: ["Db.insert User", "Db.insert Post", "Db.first Post"]
FAIL Posts.Show "an empty comment is rejected"
  error: GET /posts/hello: not found
  trace: ["Db.insert User", "Db.insert Post", "Test.get Posts.Show", "Db.first Post"]
ok   Posts.Show "a draft is not found but by its author"
25 passed, 3 failed
[exit 1]
```

The harness stopped at line 39 of your script: `alder test`: it exited 1. The commands after it were not run.

Your script stopped at command 6; the app is as the commands before it left it; continue.

The program builds but its tests fail; fix the program or the tests so every test passes.

```text
ok   Post "a slug is lowercase letters, digits and hyphens"
ok   Admin "a Reader cannot open the admin page"
ok   Admin "creating a post redirects to its edit page"
ok   Admin "a taken slug is refused"
ok   Admin "the admin page needs an Author"
ok   Admin "an Author logs out from the admin page"
ok   Admin "pressing Publish"
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
ok   Posts.Index "a draft is not listed and not found by a search"
ok   Posts.Show "a post's body is rendered as Markdown"
FAIL Posts.Show "a post's body is rendered through the Markdown capability"
  error: not found
  trace: ["Test.fake Markdown", "Db.insert User", "Db.insert Post", "Db.first Post"]
FAIL Posts.Show "a comment is listed oldest first"
  error: not found
  trace: ["Db.insert User", "Db.insert Post", "Db.first Post"]
FAIL Posts.Show "an empty comment is rejected"
  error: GET /posts/hello: not found
  trace: ["Db.insert User", "Db.insert Post", "Test.get Posts.Show", "Db.first Post"]
ok   Posts.Show "a draft is not found but by its author"
25 passed, 3 failed
```
