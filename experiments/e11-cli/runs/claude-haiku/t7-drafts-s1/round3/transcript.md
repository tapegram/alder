### Round 3

```text
$ alder fill --decl Posts.Show.load - <<'EOF'
{
  let found <- Post.bySlug params.slug
  match found {
    Just post -> {
      let mine = session.user == Just (post.author)
      if post.published || mine then {
        let body <- Markdown.render post.body
        let comments <-
          Db.from Comment
            |> Db.where (Comment.post.eq post.id)
            |> Db.orderBy Comment.created.asc
            |> Db.all
        Query.succeed { post: post, body: body, comments: comments }
      } else Query.notFound
    },
    Nothing -> Query.notFound,
  }
}
EOF
edited app/Posts/Show.alder
--- a/app/Posts/Show.alder
+++ b/app/Posts/Show.alder
@@ -20,13 +20,16 @@
   let found <- Post.bySlug params.slug
   match found {
     Just post -> {
-      let body <- Markdown.render post.body
-      let comments <-
-        Db.from Comment
-          |> Db.where (Comment.post.eq post.id)
-          |> Db.orderBy Comment.created.asc
-          |> Db.all
-      Query.succeed { post: post, body: body, comments: comments }
+      let mine = session.user == Just (post.author)
+      if post.published || mine then {
+        let body <- Markdown.render post.body
+        let comments <-
+          Db.from Comment
+            |> Db.where (Comment.post.eq post.id)
+            |> Db.orderBy Comment.created.asc
+            |> Db.all
+        Query.succeed { post: post, body: body, comments: comments }
+      } else Query.notFound
     },
     Nothing -> Query.notFound,
   }
[exit 0]
$ alder fill 'Posts.Show."a post'"'"'s body is rendered as Markdown"' - <<'EOF'
{
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "Not *yet*.", author: author, published: True }
  let m <- load { slug: Post.Slug "hello" } { user: Nothing }
  Task.succeed ({ title: m.post.title, body: m.body } == { title: Text.NonEmpty "Hello", body: <p>Not <em>yet</em>.</p> })
}
EOF
alder fill: `Posts.Show."a post's body is rendered as Markdown"` has no typed hole (`alder fill --list` lists the app's holes)
[exit 1]
```

The harness stopped at line 23 of your script: `alder fill 'Posts.Show."a post'"'"'s body is rendered as Markdown"' - <<'EOF'`: it exited 1. The commands after it were not run.

Your script stopped at command 2; the app is as the commands before it left it; continue.

The program builds but its tests fail; fix the program or the tests so every test passes.

```text
ok   Post "a slug is lowercase letters, digits and hyphens"
ok   Admin "a Reader cannot open the admin page"
ok   Admin "creating a post redirects to its edit page"
ok   Admin "a taken slug is refused"
ok   Admin "the admin page needs an Author"
ok   Admin "an Author logs out from the admin page"
ok   Admin "pressing Publish"
ok   Admin "a draft of another author cannot be published"
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
ok   Posts.Index "a draft is neither listed nor found"
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
