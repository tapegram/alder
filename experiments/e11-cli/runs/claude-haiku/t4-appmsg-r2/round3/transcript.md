### Round 3

```text
$ alder new test App "the theme flips"
edited app/App.alder
--- a/app/App.alder
+++ b/app/App.alder
@@ -36,3 +36,7 @@
   let r <- Test.run (update Logout { user: Just (User.UserId "me") })
   Task.succeed ({ value: r.value, trace: r.trace } == { value: Ok (Redirect (Auth.Login {})), trace: ["Session.put"] })
 }
+
+test "the theme flips" = {
+  ?test
+}
app/App.alder:41:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'App."the theme flips"' - <<'EOF'
{ Task.succeed (Theme.flip Light == Dark) }
EOF
edited app/App.alder
--- a/app/App.alder
+++ b/app/App.alder
@@ -1,6 +1,7 @@
 module App
 
 import Layout
+import Theme
 import User
 
 session { user: Maybe User.UserId }
@@ -38,5 +39,5 @@
 }
 
 test "the theme flips" = {
-  ?test
+  { Task.succeed (Theme.flip Light == Dark) }
 }
added import Theme to app/App.alder
[exit 0]
$ alder fill 'App."Logout clears the user and goes to log in"' - <<'EOF'
{ Task.succeed (Theme.isDark Dark) }
EOF
alder fill: `App."Logout clears the user and goes to log in"` has no typed hole; to rewrite it whole, `alder fill --decl 'App."Logout clears the user and goes to log in"' -` (`alder fill --list` lists the app's holes)
[exit 1]
```

The harness stopped at line 7 of your script: `alder fill 'App."Logout clears the user and goes to log in"' - <<'EOF'`: it exited 1. The commands after it were not run.

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
