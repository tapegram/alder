### Round 0

```text
$ alder new table User email:Text.Email@unique name:Text.NonEmpty passwordHash:Text created:Time@created --no-timestamps
wrote app/User.alder
wrote migrations/0001_initial.sql
  create table User
wrote migrations/schema.json
[exit 0]
$ alder new type Theme "Theme = Light | Dark"
wrote app/Theme.alder
[exit 0]
$ alder edit session add-field theme:Theme.Theme=Light
edited app/App.alder
--- a/app/App.alder
+++ b/app/App.alder
@@ -1,8 +1,9 @@
 module App
 
 import Layout
+import Theme
 
-session { }
+session { theme: Theme.Theme = Light }
 
 error : Error -> Html Never
 error err =
[exit 0]
$ alder new auth --users User
wrote adapters/auth/login_password.go
wrote adapters/auth/login_password_test.go
edited app/App.alder
wrote app/Auth/Login.alder
--- a/app/App.alder
+++ b/app/App.alder
@@ -2,9 +2,22 @@
 
 import Layout
 import Theme
+import User
 
-session { theme: Theme.Theme = Light }
+session { theme: Theme.Theme = Light, user: Maybe User.UserId }
 
+login = Auth.Login
+
+msg Logout
+
+update : Msg -> Session -> Task Nav
+update msg session = match msg {
+  Logout p -> {
+    let _ <- Session.put (\s -> { s | user: Nothing })
+    Task.succeed (Redirect (Auth.Login {}))
+  },
+}
+
 error : Error -> Html Never
 error err =
   <Layout.Page title="Error">
@@ -17,3 +30,10 @@
   <Layout.Page title="Not found">
     <h1>Not found</h1>
   </Layout.Page>
+
+-- Every Log out button is on a page that requires a user, so a browser test cannot tell this
+-- Redirect from a Rerender (book 10, "The logout redirect"): the test calls update itself.
+test "Logout clears the user and goes to log in" = {
+  let r <- Test.run (update Logout { user: Just (User.UserId "me"), theme: Light })
+  Task.succeed ({ value: r.value, trace: r.trace } == { value: Ok (Redirect (Auth.Login {})), trace: ["Session.put"] })
+}
app/Auth/Login.alder:47:33: warning[W0002]: hole ?afterLogin has type Page
          Task.succeed (Redirect (?afterLogin))
                                  ^^^^^^^^^^^
app/Auth/Login.alder:86:85: warning[W0002]: hole ?afterLogin has type Page
    Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
                                                                                      ^^^^^^^^^^^
[exit 0]
$ alder new table Note title:Text.NonEmpty body:Text owner:User.UserId@ref=User@owner created:Time@created --no-timestamps
wrote app/Note.alder
wrote migrations/0002_create_note.sql
  create table Note
  add index on Note.owner
edited migrations/schema.json
[exit 0]
$ alder new decl Layout Frame "{ title: Text, theme: Theme.Theme, children: List (Html a) } -> Html a"
alder new decl: `Frame` is not a value name: a value is named by a lower-case identifier, such as `waiting`
[exit 1]
```

The harness stopped at line 7 of your script: `alder new decl Layout Frame "{ title: Text, theme: Theme.Theme, children: List (Html a) } -> Html a"`: it exited 1. The commands after it were not run.

Your script stopped at command 6; the app is as the commands before it left it; continue.

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
