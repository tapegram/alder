### Round 4

```text
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
ok   Posts.Show "a post's body is rendered through the Markdown capability"
ok   Posts.Show "a comment is listed oldest first"
ok   Posts.Show "an empty comment is rejected"
ok   Posts.Show "a draft is not found but by its author"
28 passed, 0 failed
[exit 0]
```

The harness ran `alder check`, `alder build` and `alder test` on the app: all pass.
