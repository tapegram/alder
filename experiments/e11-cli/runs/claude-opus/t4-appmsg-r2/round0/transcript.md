### Round 0

```text
$ alder new table User email:Text.Email@unique name:Text.NonEmpty passwordHash:Text created:Time@created --no-timestamps
wrote app/User.alder
wrote migrations/0001_initial.sql
  create table User
wrote migrations/schema.json
[exit 0]
$ alder new auth --users User
wrote adapters/auth/login_password.go
wrote adapters/auth/login_password_test.go
edited app/App.alder
wrote app/Auth/Login.alder
--- a/app/App.alder
+++ b/app/App.alder
@@ -1,9 +1,22 @@
 module App
 
 import Layout
+import User
 
-session { }
+session { user: Maybe User.UserId }
 
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
@@ -16,3 +29,10 @@
   <Layout.Page title="Not found">
     <h1>Not found</h1>
   </Layout.Page>
+
+-- Every Log out button is on a page that requires a user, so a browser test cannot tell this
+-- Redirect from a Rerender (book 10, "The logout redirect"): the test calls update itself.
+test "Logout clears the user and goes to log in" = {
+  let r <- Test.run (update Logout { user: Just (User.UserId "me") })
+  Task.succeed ({ value: r.value, trace: r.trace } == { value: Ok (Redirect (Auth.Login {})), trace: ["Session.put"] })
+}
app/Auth/Login.alder:47:33: warning[W0002]: hole ?afterLogin has type Page
          Task.succeed (Redirect (?afterLogin))
                                  ^^^^^^^^^^^
app/Auth/Login.alder:86:85: warning[W0002]: hole ?afterLogin has type Page
    Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
                                                                                      ^^^^^^^^^^^
[exit 0]
$ alder new type Theme "Theme = Light | Dark"
wrote app/Theme.alder
[exit 0]
$ alder new decl Theme flip "Theme -> Theme"
edited app/Theme.alder
--- a/app/Theme.alder
+++ b/app/Theme.alder
@@ -1,3 +1,6 @@
 module Theme
 
 type Theme = Light | Dark
+
+flip : Theme -> Theme
+flip = ?flip
app/Theme.alder:6:8: warning[W0002]: hole ?flip has type Theme -> Theme
  flip = ?flip
         ^^^^^
[exit 0]
$ alder fill Theme.flip - <<'EOF'
\t -> match t {
  Light -> Dark,
  Dark -> Light,
}
EOF
edited app/Theme.alder
--- a/app/Theme.alder
+++ b/app/Theme.alder
@@ -3,4 +3,7 @@
 type Theme = Light | Dark
 
 flip : Theme -> Theme
-flip = ?flip
+flip = \t -> match t {
+  Light -> Dark,
+  Dark -> Light,
+}
[exit 0]
$ alder new decl Theme buttonLabel "Theme -> Text"
edited app/Theme.alder
--- a/app/Theme.alder
+++ b/app/Theme.alder
@@ -7,3 +7,6 @@
   Light -> Dark,
   Dark -> Light,
 }
+
+buttonLabel : Theme -> Text
+buttonLabel = ?buttonLabel
app/Theme.alder:12:15: warning[W0002]: hole ?buttonLabel has type Theme -> Text
  buttonLabel = ?buttonLabel
                ^^^^^^^^^^^^
[exit 0]
$ alder fill Theme.buttonLabel - <<'EOF'
\t -> match t {
  Light -> "Switch to dark",
  Dark -> "Switch to light",
}
EOF
edited app/Theme.alder
--- a/app/Theme.alder
+++ b/app/Theme.alder
@@ -9,4 +9,7 @@
 }
 
 buttonLabel : Theme -> Text
-buttonLabel = ?buttonLabel
+buttonLabel = \t -> match t {
+  Light -> "Switch to dark",
+  Dark -> "Switch to light",
+}
[exit 0]
$ alder new decl Theme bodyClass "Theme -> Maybe Text"
edited app/Theme.alder
--- a/app/Theme.alder
+++ b/app/Theme.alder
@@ -13,3 +13,6 @@
   Light -> "Switch to dark",
   Dark -> "Switch to light",
 }
+
+bodyClass : Theme -> Maybe Text
+bodyClass = ?bodyClass
app/Theme.alder:18:13: warning[W0002]: hole ?bodyClass has type Theme -> Maybe Text
  bodyClass = ?bodyClass
              ^^^^^^^^^^
[exit 0]
$ alder fill Theme.bodyClass - <<'EOF'
\t -> match t {
  Light -> Nothing,
  Dark -> Just "dark",
}
EOF
edited app/Theme.alder
--- a/app/Theme.alder
+++ b/app/Theme.alder
@@ -15,4 +15,7 @@
 }
 
 bodyClass : Theme -> Maybe Text
-bodyClass = ?bodyClass
+bodyClass = \t -> match t {
+  Light -> Nothing,
+  Dark -> Just "dark",
+}
[exit 0]
$ alder new test Theme "flip swaps the theme"
edited app/Theme.alder
--- a/app/Theme.alder
+++ b/app/Theme.alder
@@ -19,3 +19,7 @@
   Light -> Nothing,
   Dark -> Just "dark",
 }
+
+test "flip swaps the theme" = {
+  ?test
+}
app/Theme.alder:24:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Theme."flip swaps the theme"' - <<'EOF'
{
  let flipped = List.map flip [Light, Dark]
  let labels = List.map buttonLabel [Light, Dark]
  Task.succeed ({ flipped: flipped, labels: labels } == { flipped: [Dark, Light], labels: ["Switch to dark", "Switch to light"] })
}
EOF
edited app/Theme.alder
--- a/app/Theme.alder
+++ b/app/Theme.alder
@@ -21,5 +21,7 @@
 }
 
 test "flip swaps the theme" = {
-  ?test
+  let flipped = List.map flip [Light, Dark]
+  let labels = List.map buttonLabel [Light, Dark]
+  Task.succeed ({ flipped: flipped, labels: labels } == { flipped: [Dark, Light], labels: ["Switch to dark", "Switch to light"] })
 }
[exit 0]
$ alder edit session add-field theme:Theme.Theme=Light
edited app/App.alder
--- a/app/App.alder
+++ b/app/App.alder
@@ -1,9 +1,10 @@
 module App
 
 import Layout
+import Theme
 import User
 
-session { user: Maybe User.UserId }
+session { user: Maybe User.UserId, theme: Theme.Theme = Light }
 
 login = Auth.Login
 
@@ -33,6 +34,6 @@
 -- Every Log out button is on a page that requires a user, so a browser test cannot tell this
 -- Redirect from a Rerender (book 10, "The logout redirect"): the test calls update itself.
 test "Logout clears the user and goes to log in" = {
-  let r <- Test.run (update Logout { user: Just (User.UserId "me") })
+  let r <- Test.run (update Logout { user: Just (User.UserId "me"), theme: Light })
   Task.succeed ({ value: r.value, trace: r.trace } == { value: Ok (Redirect (Auth.Login {})), trace: ["Session.put"] })
 }
[exit 0]
$ alder new decl Layout Themed "{ title: Text, theme: Theme.Theme, children: List (Html a) } -> Html a"
alder new decl: `Themed` is not a value name: a value is named by a lower-case identifier, such as `waiting`
[exit 1]
```

The harness stopped at line 39 of your script: `alder new decl Layout Themed "{ title: Text, theme: Theme.Theme, children: List (Html a) } -> Html a"`: it exited 1. The commands after it were not run.

Your script stopped at command 13; the app is as the commands before it left it; continue.

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
