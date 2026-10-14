### Round 2

```text
$ alder new type Theme "Theme = Light | Dark"
wrote app/Theme.alder
[exit 0]
$ alder new decl Theme isDark "Theme -> Bool"
edited app/Theme.alder
--- a/app/Theme.alder
+++ b/app/Theme.alder
@@ -1,3 +1,6 @@
 module Theme
 
 type Theme = Light | Dark
+
+isDark : Theme -> Bool
+isDark = ?isDark
app/Theme.alder:6:10: warning[W0002]: hole ?isDark has type Theme -> Bool
  isDark = ?isDark
           ^^^^^^^
[exit 0]
$ alder fill Theme.isDark - <<'EOF'
\t -> t == Dark
EOF
edited app/Theme.alder
--- a/app/Theme.alder
+++ b/app/Theme.alder
@@ -3,4 +3,4 @@
 type Theme = Light | Dark
 
 isDark : Theme -> Bool
-isDark = ?isDark
+isDark = \t -> t == Dark
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
$ alder new msg App ToggleTheme
alder new msg: `App` is not a page (it has no `page` declaration): `alder new page App /path` makes one
[exit 1]
```

The harness stopped at line 10 of your script: `alder new msg App ToggleTheme`: it exited 1. The commands after it were not run.

Your script stopped at command 5; the app is as the commands before it left it; continue.

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
