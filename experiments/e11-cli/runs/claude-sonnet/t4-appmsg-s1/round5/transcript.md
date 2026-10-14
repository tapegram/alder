### Round 5

```text
$ alder fill Notes.Index.load - <<'EOF'
{
  let notes <- Db.mine Note |> Db.orderBy Note.created.desc |> Db.all
  Query.succeed { notes: notes }
}
EOF
edited app/Notes/Index.alder
--- a/app/Notes/Index.alder
+++ b/app/Notes/Index.alder
@@ -7,7 +7,10 @@
 type Model = { notes: List Note }
 
 load : Params -> Session -> Query Model
-load params session = ?load
+load params session = {
+  let notes <- Db.mine Note |> Db.orderBy Note.created.desc |> Db.all
+  Query.succeed { notes: notes }
+}
 
 view : Model -> Session -> Html Msg
 view model session = ?view
app/Notes/Index.alder:16:22: warning[W0002]: hole ?view has type Html Notes.Index.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder fill Notes.Index.view - <<'EOF'
Html.none
EOF
edited app/Notes/Index.alder
--- a/app/Notes/Index.alder
+++ b/app/Notes/Index.alder
@@ -13,7 +13,7 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = ?view
+view model session = Html.none
 
 test "a visitor is sent to log in" = {
   let r <- Test.open (Notes.Index {})
[exit 0]
$ alder new form Notes.Index NewNote title:Text.NonEmpty body:Text --msg Submit
alder new form: `Notes.Index`'s `view` is not one element over several lines: add the markup yourself
[exit 1]
```

The harness stopped at line 11 of your script: `alder new form Notes.Index NewNote title:Text.NonEmpty body:Text --msg Submit`: it exited 1. The commands after it were not run.

Your script stopped at command 3; the app is as the commands before it left it; continue.

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Auth/Login.alder:47:33: warning[W0002]: hole ?afterLogin has type Page
          Task.succeed (Redirect (?afterLogin))
                                  ^^^^^^^^^^^
app/Auth/Login.alder:86:85: warning[W0002]: hole ?afterLogin has type Page
    Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
                                                                                      ^^^^^^^^^^^
app/Notes/Show.alder:11:23: warning[W0002]: hole ?load has type Query Notes.Show.Model
  load params session = ?load
                        ^^^^^
app/Notes/Show.alder:14:22: warning[W0002]: hole ?view has type Html Notes.Show.Msg
  view model session = ?view
                       ^^^^^
alder build: the program has 4 typed hole(s); fill them before building
```
