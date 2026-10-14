### Round 1

```text
$ alder fill Profile.load - <<'EOF'
{
  let found <- Db.get User session.user
  let total <- Db.mine Todo |> Db.count
  let finished <- Db.mine Todo |> Db.where (Todo.done.eq True) |> Db.count
  match found {
    Just u -> Query.succeed { name: u.name, email: u.email, total: total, finished: finished },
    Nothing -> Query.notFound,
  }
}
EOF
edited app/Profile.alder
--- a/app/Profile.alder
+++ b/app/Profile.alder
@@ -1,11 +1,22 @@
 module Profile
 
+import Todo
+import User
+
 page "/profile" { } requires user
 
 type Model = { name: Text.NonEmpty, email: Text.Email, total: Int, finished: Int }
 
 load : Params -> Session -> Query Model
-load params session = ?load
+load params session = {
+  let found <- Db.get User session.user
+  let total <- Db.mine Todo |> Db.count
+  let finished <- Db.mine Todo |> Db.where (Todo.done.eq True) |> Db.count
+  match found {
+    Just u -> Query.succeed { name: u.name, email: u.email, total: total, finished: finished },
+    Nothing -> Query.notFound,
+  }
+}
 
 view : Model -> Session -> Html Msg
 view model session = ?view
app/Profile.alder:22:22: warning[W0002]: hole ?view has type Html Profile.Msg
  view model session = ?view
                       ^^^^^
added import User to app/Profile.alder
added import Todo to app/Profile.alder
[exit 0]
$ alder fill Profile.view - <<'EOF'
<Layout.Page title="Profile"><h1>Profile</h1></Layout.Page>
EOF
edited app/Profile.alder
--- a/app/Profile.alder
+++ b/app/Profile.alder
@@ -1,5 +1,6 @@
 module Profile
 
+import Layout
 import Todo
 import User
 
@@ -19,7 +20,7 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = ?view
+view model session = <Layout.Page title="Profile"><h1>Profile</h1></Layout.Page>
 
 test "a visitor is sent to log in" = {
   let r <- Test.open (Profile {})
added import Layout to app/Profile.alder
[exit 0]
$ alder new form Profile Rename name:Text.NonEmpty --msg SaveName
alder new form: `Profile`'s `view` is not one element over several lines: add the markup yourself
[exit 1]
```

The harness stopped at line 15 of your script: `alder new form Profile Rename name:Text.NonEmpty --msg SaveName`: it exited 1. The commands after it were not run.

Your script stopped at command 3; the app is as the commands before it left it; continue.

The harness ran `alder check`, `alder build` and `alder test` on the app as the commands left it: all pass, but the round does not: the script stopped before its end.
