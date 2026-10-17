### Round 2

```text
$ alder fill --decl 'Admin."pressing Publish"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1", role: Author }
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
@@ -157,17 +157,14 @@
   Task.succeed ({ heading: Test.text "h1" page.doc, out: out.answer, user: out.session.user } == { heading: Just "Your posts", out: Redirected (Auth.Login {}), user: Nothing })
 }
 
--- Fill ?expected with what pressing Publish must do, so that a test fails when the arm
--- does something else: its answer (`r.answer`: Rerendered, Redirected …) and what it
--- changed (`after`, the row read again), as `{ answer: r.answer, … } == { answer: Rerendered, … }`.
 test "pressing Publish" = {
-  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
+  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1", role: Author }
   let _ <- Test.login { user: Just me }
   let post1 <- Db.insert Post { slug: Post.Slug "slug-1", title: Text.NonEmpty "title-1", body: "body-1", author: me }
   let _ <- Test.get (Admin {})
   let r <- Test.click (Publish { id: post1 })
   let after <- Db.get Post post1
-  Task.succeed ?expected
+  Task.succeed ({ answer: r.answer, published: Maybe.map (\p -> p.published) after } == { answer: Redirected (Admin {}), published: Just True })
 }
 
 test "publishing a draft puts it on the front page" = {
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
ok   Posts.Index "a draft is not listed or found"
ok   Posts.Show "a post's body is rendered as Markdown"
ok   Posts.Show "a post's body is rendered through the Markdown capability"
ok   Posts.Show "a comment is listed oldest first"
ok   Posts.Show "an empty comment is rejected"
ok   Posts.Show "a draft is not found by a reader but by its author"
29 passed, 0 failed
[exit 0]
```

The harness ran `alder check`, `alder build` and `alder test` on the app: all pass.
