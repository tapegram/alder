### Round 1

```text
$ alder new type App "Theme = Light | Dark"
edited app/App.alder
--- a/app/App.alder
+++ b/app/App.alder
@@ -1,8 +1,11 @@
 module App
 
 import Layout
+import Theme
 import User
 
+type Theme = Light | Dark
+
 session { user: Maybe User.UserId }
 
 login = Auth.Login
app/App.alder:4:1: warning[W0001]: import `Theme` is unused
  import Theme
  ^^^^^^^^^^^^
[exit 0]
$ alder edit session add-field theme:Theme=Light
edited app/App.alder
--- a/app/App.alder
+++ b/app/App.alder
@@ -6,7 +6,7 @@
 
 type Theme = Light | Dark
 
-session { user: Maybe User.UserId }
+session { user: Maybe User.UserId, theme: Theme = Light }
 
 login = Auth.Login
 
@@ -36,6 +36,6 @@
 -- Every Log out button is on a page that requires a user, so a browser test cannot tell this
 -- Redirect from a Rerender (book 10, "The logout redirect"): the test calls update itself.
 test "Logout clears the user and goes to log in" = {
-  let r <- Test.run (update Logout { user: Just (User.UserId "me") })
+  let r <- Test.run (update Logout { user: Just (User.UserId "me"), theme: Light })
   Task.succeed ({ value: r.value, trace: r.trace } == { value: Ok (Redirect (Auth.Login {})), trace: ["Session.put"] })
 }
app/App.alder:4:1: warning[W0001]: import `Theme` is unused
  import Theme
  ^^^^^^^^^^^^
[exit 0]
$ alder new table Note title:Text.NonEmpty body:Text owner:User.UserId@ref=User@owner created:Time@created --no-timestamps
wrote app/Note.alder
wrote migrations/0002_create_note.sql
  create table Note
  add index on Note.owner
edited migrations/schema.json
[exit 0]
$ alder new page Notes.Index / --list Note --requires user
wrote app/Notes/Index.alder
[exit 0]
$ alder new page Notes.Show '/notes/{id}' --param id:Note.NoteId --show Note --requires user
alder new page: --param goes with a page of its own or a --blank one: a --list, --show or --from page has its parameters made for it
[exit 1]
```

The harness stopped at line 10 of your script: `alder new page Notes.Show '/notes/{id}' --param id:Note.NoteId --show Note --requires user`: it exited 1. The commands after it were not run.

Your script stopped at command 5; the app is as the commands before it left it; continue.

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/App.alder:4:1: warning[W0001]: import `Theme` is unused
  import Theme
  ^^^^^^^^^^^^
app/Auth/Login.alder:47:33: warning[W0002]: hole ?afterLogin has type Page
          Task.succeed (Redirect (?afterLogin))
                                  ^^^^^^^^^^^
app/Auth/Login.alder:86:85: warning[W0002]: hole ?afterLogin has type Page
    Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
                                                                                      ^^^^^^^^^^^
alder build: the program has 2 typed hole(s); fill them before building
```
