### Round 0

```text
$ alder new type Post 'Slug = brand Text where Text.matches "^[a-z0-9-]+$"'
wrote app/Post.alder
[exit 0]
$ alder new table User email:Text.Email@unique name:Text.NonEmpty passwordHash:Text created:Time@created --no-timestamps
wrote app/User.alder
wrote migrations/0001_initial.sql
  create table User
wrote migrations/schema.json
[exit 0]
$ alder new table Post title:Text.NonEmpty slug:Slug@unique body:Text published:Bool=False author:User.UserId@ref=User created:Time@created --no-timestamps
edited app/Post.alder
wrote migrations/0002_create_post.sql
  create table Post
  add index on Post.author
edited migrations/schema.json
--- a/app/Post.alder
+++ b/app/Post.alder
@@ -1,3 +1,25 @@
 module Post
 
+import User
+
 type Slug = brand Text where Text.matches "^[a-z0-9-]+$"
+
+type PostId = brand Text
+
+table Post {
+  id: PostId @id,
+  title: Text.NonEmpty,
+  slug: Slug @unique,
+  body: Text,
+  published: Bool = False,
+  author: User.UserId @ref User,
+  created: Time @created,
+}
+
+test "a Post keeps its author from being deleted" = {
+  let target <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
+  let _ <- Db.insert Post { title: Text.NonEmpty "title-1", slug: Slug "slug-1", body: "body-1", author: target }
+  let r <- Test.run (Db.delete User target)
+  let left <- Db.from Post |> Db.count
+  Task.succeed ({ deleted: Result.toMaybe r.value, left: left } == { deleted: Nothing, left: 1 })
+}
[exit 0]
$ alder new table Comment post:Post.PostId@ref=Post@cascade author:Text.NonEmpty body:Text.NonEmpty created:Time@created --no-timestamps
wrote app/Comment.alder
wrote migrations/0003_create_comment.sql
  create table Comment
  add index on Comment.post
edited migrations/schema.json
[exit 0]
$ alder fill --decl Layout.Page - <<'EOF'
\props -> <html>
    <head>
      <meta charset="utf-8" />
      <title>{props.title}</title>
      <link rel="stylesheet" href={Url.external "/static/al/al.css"} />
    </head>
    <body>
      <al-page width="narrow">
        <main>{props.children}</main>
      </al-page>
    </body>
  </html>
EOF
app/Layout.alder:5:3: error[E0003]: expected Html a, got _ -> Html _
    \props -> <html>
    ^
app/Layout.alder:8:17: error[E0003]: cannot infer the record type of `props`; add a type annotation
          <title>{props.title}</title>
                  ^^^^^^^^^^^
app/Layout.alder:13:18: error[E0003]: cannot infer the record type of `props`; add a type annotation
            <main>{props.children}</main>
                   ^^^^^^^^^^^^^^
alder fill: the result does not check (the errors above): nothing was written
[exit 1]
```

The harness stopped at line 8 of your script: `alder fill --decl Layout.Page - <<'EOF'`: it exited 1. The commands after it were not run.

Your script stopped at command 5; the app is as the commands before it left it; continue.

The harness ran `alder check`, `alder build` and `alder test` on the app as the commands left it: all pass, but the round does not: the script stopped before its end.
