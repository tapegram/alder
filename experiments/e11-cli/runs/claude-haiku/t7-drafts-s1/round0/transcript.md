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
$ alder fill 'Posts.Index."the front page lists the posts, newest first"' - <<'EOF'
{
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "first", title: Text.NonEmpty "First", body: "", author: author, published: True }
  let _ <- Db.insert Post { slug: Post.Slug "second", title: Text.NonEmpty "Second", body: "", author: author, published: True }
  let model <- load { page: 1, q: "" } { user: Nothing }
  Task.succeed (List.map (\p -> p.title) model.posts == [Text.NonEmpty "Second", Text.NonEmpty "First"])
}
EOF
alder fill: `Posts.Index."the front page lists the posts, newest first"` has no typed hole (`alder fill --list` lists the app's holes)
[exit 1]
```

The harness stopped at line 29 of your script: `alder fill 'Posts.Index."the front page lists the posts, newest first"' - <<'EOF'`: it exited 1. The commands after it were not run.

Your script stopped at command 3; the app is as the commands before it left it; continue.

The program builds but its tests fail; fix the program or the tests so every test passes.

```text
ok   Post "a slug is lowercase letters, digits and hyphens"
ok   Admin "a Reader cannot open the admin page"
ok   Admin "creating a post redirects to its edit page"
ok   Admin "a taken slug is refused"
ok   Admin "the admin page needs an Author"
ok   Admin "an Author logs out from the admin page"
ok   Comment "a name or comment has text and a bound"
ok   Admin.Edit "deleting a post deletes its comments"
ok   Admin.Edit "another author's post is not found"
ok   Admin.Edit "a demoted author cannot edit"
FAIL Posts.Index "the front page lists the posts, newest first"
  left:  []
  right: ["Second", "First"]
  trace: ["Db.insert User", "Db.insert Post", "Db.insert Post", "Db.all Post"]
FAIL Posts.Index "search matches title or body"
  left:  { count: 0, title: False, body: False }
  right: { count: 2, title: True, body: True }
  trace: ["Db.insert User", "Db.insert Post", "Db.insert Post", "Db.insert Post", "Db.all Post"]
FAIL Posts.Index "a `%` in the query is literal"
  left:  []
  right: ["100% done"]
  trace: ["Db.insert User", "Db.insert Post", "Db.insert Post", "Db.all Post"]
FAIL Posts.Index "page 99 is empty"
  left:  [{ page: 1, posts: 0, hasMore: False }, { page: 2, posts: 0, hasMore: False }, { page: 99, posts: 0, hasMore: False }, { page: 1, posts: 0, hasMore: False }]
  right: [{ page: 1, posts: 10, hasMore: True }, { page: 2, posts: 1, hasMore: False }, { page: 99, posts: 0, hasMore: False }, { page: 1, posts: 10, hasMore: True }]
  trace: ["Db.insert User", "Db.insert Post", "Db.insert Post", "Db.insert Post", "Db.insert Post", "Db.insert Post", "Db.insert Post", "Db.insert Post", "Db.insert Post", "Db.insert Post", "Db.insert Post", "Db.insert Post", "Db.all Post", "Db.all Post", "Db.all Post", "Db.all Post"]
ok   Posts.Index "a negative page is page 1"
FAIL Posts.Index "a `_` in the query is literal"
  left:  []
  right: ["snake_case"]
  trace: ["Db.insert User", "Db.insert Post", "Db.insert Post", "Db.all Post"]
ok   Posts.Index "a long query is cut"
ok   Posts.Index "a search answers with the first page of its results"
ok   Posts.Index "only an Author sees the Write link"
FAIL Posts.Index "the second page links back to the first"
  left:  { firstPosts: 0, next: Nothing, secondPosts: 0, prev: Just "/", more: False }
  right: { firstPosts: 10, next: Just "/?page=2", secondPosts: 1, prev: Just "/", more: False }
  trace: ["Db.insert User", "Db.insert Post", "Db.insert Post", "Db.insert Post", "Db.insert Post", "Db.insert Post", "Db.insert Post", "Db.insert Post", "Db.insert Post", "Db.insert Post", "Db.insert Post", "Db.insert Post", "Test.get Posts.Index", "Db.all Post", "Test.get Posts.Index", "Db.all Post"]
FAIL Posts.Index "a search through the form"
  left:  { answer: Redirected /?q=tomato, titles: [], query: Just "tomato" }
  right: { answer: Redirected /?q=tomato, titles: ["Soup"], query: Just "tomato" }
  trace: ["Db.insert User", "Db.insert Post", "Db.insert Post", "Test.get Posts.Index", "Db.all Post", "Test.submit SearchForm", "Db.all Post", "Db.all Post"]
ok   Posts.Show "a post's body is rendered as Markdown"
ok   Posts.Show "a post's body is rendered through the Markdown capability"
ok   Posts.Show "a comment is listed oldest first"
ok   Posts.Show "an empty comment is rejected"
18 passed, 7 failed
```
