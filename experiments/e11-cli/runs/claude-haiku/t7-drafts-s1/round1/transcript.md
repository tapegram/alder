### Round 1

```text
$ alder new msg Admin Publish id:Post.PostId
edited app/Admin.alder
--- a/app/Admin.alder
+++ b/app/Admin.alder
@@ -38,6 +38,8 @@
 
 msg Create form NewPost
 
+msg Publish { id: Post.PostId }
+
 update : Msg -> Model -> Session -> Task Nav
 update msg model session = match msg {
   Create p -> {
@@ -50,6 +52,7 @@
       },
     }
   },
+  Publish p -> ?publish,
 }
 
 view : Model -> Session -> Html Msg
@@ -77,6 +80,8 @@
     </form>
 
     <button click={App.Logout}>Log out</button>
+
+    <al-button click={Publish { id: ?publishId }}>Publish</al-button>
   </Layout.Page>
 
 test "a Reader cannot open the admin page" = {
@@ -130,3 +135,16 @@
   let out <- Test.click App.Logout
   Task.succeed ({ heading: Test.text "h1" page.doc, out: out.answer, user: out.session.user } == { heading: Just "Your posts", out: Redirected (Auth.Login {}), user: Nothing })
 }
+
+-- Fill ?expected with what pressing Publish must do, so that a test fails when the arm
+-- does something else: its answer (`r.answer`: Rerendered, Redirected …) and what it
+-- changed (`after`, the row read again), as `{ answer: r.answer, … } == { answer: Rerendered, … }`.
+test "pressing Publish" = {
+  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
+  let _ <- Test.login { user: Just me }
+  let post1 <- Db.insert Post { slug: Post.Slug "slug-1", title: Text.NonEmpty "title-1", body: "body-1", author: me }
+  let _ <- Test.get (Admin {})
+  let r <- Test.click (Publish { id: post1 })
+  let after <- Db.get Post post1
+  Task.succeed ?expected
+}
app/Admin.alder:55:16: warning[W0002]: hole ?publish has type Task Nav
    Publish p -> ?publish,
                 ^^^^^^^^
app/Admin.alder:84:37: warning[W0002]: hole ?publishId has type Post.PostId
      <al-button click={Publish { id: ?publishId }}>Publish</al-button>
                                      ^^^^^^^^^^
app/Admin.alder:149:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
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
      Query.succeed { author: user, posts: posts },
    },
    Nothing -> Query.notFound,
  }
}
EOF
alder fill: `alder fill --decl` takes one expression; standard input does not parse as one: expected `}`, got `,`; the last expression of a block is its result and takes no `,` (line 11, column 51)
[exit 1]
```

The harness stopped at line 5 of your script: `alder fill --decl Admin.load - <<'EOF'`: it exited 1. The commands after it were not run.

Your script stopped at command 2; the app is as the commands before it left it; continue.

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Admin.alder:55:16: warning[W0002]: hole ?publish has type Task Nav
    Publish p -> ?publish,
                 ^^^^^^^^
app/Admin.alder:84:37: warning[W0002]: hole ?publishId has type Post.PostId
      <al-button click={Publish { id: ?publishId }}>Publish</al-button>
                                      ^^^^^^^^^^
app/Admin.alder:149:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
alder build: the program has 3 typed hole(s); fill them before building
```
