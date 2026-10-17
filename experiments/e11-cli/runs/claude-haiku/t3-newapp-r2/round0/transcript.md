### Round 0

```text
$ alder new table User email:Text.Email@unique name:Text.NonEmpty passwordHash:Text created:Time@created --no-timestamps
wrote app/User.alder
wrote migrations/0001_initial.sql
  create table User
wrote migrations/schema.json
[exit 0]
$ alder new type Post 'Slug = brand Text where Text.matches "^[a-z0-9-]+$"'
wrote app/Post.alder
[exit 0]
$ alder new table Post slug:Post.Slug@unique title:Text.NonEmpty body:Text published:Bool=False author:User.UserId@ref=User
edited app/Post.alder
wrote migrations/0002_create_post.sql
  create table Post
  add index on Post.author
edited migrations/schema.json
--- a/app/Post.alder
+++ b/app/Post.alder
@@ -1,3 +1,26 @@
 module Post
 
+import User
+
 type Slug = brand Text where Text.matches "^[a-z0-9-]+$"
+
+type PostId = brand Text
+
+table Post {
+  id: PostId @id,
+  slug: Post.Slug @unique,
+  title: Text.NonEmpty,
+  body: Text,
+  published: Bool = False,
+  author: User.UserId @ref User,
+  created: Time @created,
+  updated: Time @updated,
+}
+
+test "a Post keeps its author from being deleted" = {
+  let target <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
+  let _ <- Db.insert Post { slug: Slug "slug-1", title: Text.NonEmpty "title-1", body: "body-1", author: target }
+  let r <- Test.run (Db.delete User target)
+  let left <- Db.from Post |> Db.count
+  Task.succeed ({ deleted: Result.toMaybe r.value, left: left } == { deleted: Nothing, left: 1 })
+}
[exit 0]
$ alder new table Comment post:Post.PostId@ref=Post@cascade author:Text.NonEmpty body:Text.NonEmpty
wrote app/Comment.alder
wrote migrations/0003_create_comment.sql
  create table Comment
  add index on Comment.post
edited migrations/schema.json
[exit 0]
$ alder new page Home / --blank
wrote app/Home.alder
app/Home.alder:8:23: warning[W0002]: hole ?load has type Query Home.Model
  load params session = ?load
                        ^^^^^
app/Home.alder:11:22: warning[W0002]: hole ?view has type Html Home.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder new page Posts.Show '/posts/{slug}' --param slug:Post.Slug --blank
wrote app/Posts/Show.alder
app/Posts/Show.alder:10:23: warning[W0002]: hole ?load has type Query Posts.Show.Model
  load params session = ?load
                        ^^^^^
app/Posts/Show.alder:13:22: warning[W0002]: hole ?view has type Html Posts.Show.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder new page Admin /admin --blank --requires user
alder new page: `--requires user` needs a session with a `user` field (`session { user: Maybe User.UserId }` in `App`): this app's session has none
[exit 1]
```

The harness stopped at line 10 of your script: `alder new page Admin /admin --blank --requires user`: it exited 1. The commands after it were not run.

Your script stopped at command 7; the app is as the commands before it left it; continue.

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Home.alder:8:23: warning[W0002]: hole ?load has type Query Home.Model
  load params session = ?load
                        ^^^^^
app/Home.alder:11:22: warning[W0002]: hole ?view has type Html Home.Msg
  view model session = ?view
                       ^^^^^
app/Posts/Show.alder:10:23: warning[W0002]: hole ?load has type Query Posts.Show.Model
  load params session = ?load
                        ^^^^^
app/Posts/Show.alder:13:22: warning[W0002]: hole ?view has type Html Posts.Show.Msg
  view model session = ?view
                       ^^^^^
alder build: the program has 4 typed hole(s); fill them before building
```
