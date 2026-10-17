### Round 4

```text
$ alder fill --decl Notes.Index.load - <<'EOF'
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
$ alder fill --decl Notes.Index.view - <<'EOF'
Layout.shell { title: "Notes", theme: session.theme, children: [<h1>Notes</h1>] }
EOF
edited app/Notes/Index.alder
--- a/app/Notes/Index.alder
+++ b/app/Notes/Index.alder
@@ -1,5 +1,6 @@
 module Notes.Index
 
+import Layout
 import Note
 
 page "/" { } requires user
@@ -13,7 +14,7 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = ?view
+view model session = Layout.shell { title: "Notes", theme: session.theme, children: [<h1>Notes</h1>] }
 
 test "a visitor is sent to log in" = {
   let r <- Test.open (Notes.Index {})
added import Layout to app/Notes/Index.alder
[exit 0]
$ alder new form Notes.Index NewNote title:Text.NonEmpty body:Text --msg Add
alder new form: `Notes.Index`'s `view` is not one element over several lines (write it as `<Layout.Page …>` … `</Layout.Page>` on several lines, with `alder fill --decl Notes.Index.view -`), or add the markup yourself
[exit 1]
```

The harness stopped at line 11 of your script: `alder new form Notes.Index NewNote title:Text.NonEmpty body:Text --msg Add`: it exited 1. The commands after it were not run.

Your script stopped at command 3; the app is as the commands before it left it; continue.

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Auth/Login.alder:47:33: warning[W0002]: hole ?afterLogin has type Page
          Task.succeed (Redirect (?afterLogin))
                                  ^^^^^^^^^^^
app/Auth/Login.alder:86:85: warning[W0002]: hole ?afterLogin has type Page
    Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
                                                                                      ^^^^^^^^^^^
alder build: the program has 2 typed hole(s); fill them before building
```
