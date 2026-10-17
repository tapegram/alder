### Round 1

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
\params session -> {
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
alder fill: the parameters `params session` are bound already: send the body without `\params session ->`
[exit 1]
```

The harness stopped at line 5 of your script: `alder fill --decl Posts.Index.load - <<'EOF'`: it exited 1. The commands after it were not run.

Your script stopped at command 2; the app is as the commands before it left it; continue.

The harness ran `alder check`, `alder build` and `alder test` on the app as the commands left it: all pass, but the round does not: the script stopped before its end.
