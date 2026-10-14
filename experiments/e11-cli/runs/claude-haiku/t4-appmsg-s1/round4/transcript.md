### Round 4

```text
$ alder new decl App isDark "Theme -> Bool"
edited app/App.alder
--- a/app/App.alder
+++ b/app/App.alder
@@ -33,6 +33,9 @@
     <h1>Not found</h1>
   </Layout.Page>
 
+isDark : Theme -> Bool
+isDark = ?isDark
+
 -- Every Log out button is on a page that requires a user, so a browser test cannot tell this
 -- Redirect from a Rerender (book 10, "The logout redirect"): the test calls update itself.
 test "Logout clears the user and goes to log in" = {
app/App.alder:4:1: warning[W0001]: import `Theme` is unused
  import Theme
  ^^^^^^^^^^^^
app/App.alder:37:10: warning[W0002]: hole ?isDark has type App.Theme -> Bool
  isDark = ?isDark
           ^^^^^^^
[exit 0]
$ alder fill --decl App.isDark - <<'EOF'
\t -> match t { Light -> False, Dark -> True }
EOF
edited app/App.alder
--- a/app/App.alder
+++ b/app/App.alder
@@ -34,7 +34,7 @@
   </Layout.Page>
 
 isDark : Theme -> Bool
-isDark = ?isDark
+isDark = \t -> match t { Light -> False, Dark -> True }
 
 -- Every Log out button is on a page that requires a user, so a browser test cannot tell this
 -- Redirect from a Rerender (book 10, "The logout redirect"): the test calls update itself.
app/App.alder:4:1: warning[W0001]: import `Theme` is unused
  import Theme
  ^^^^^^^^^^^^
[exit 0]
$ alder new decl App toggled "Theme -> Theme"
edited app/App.alder
--- a/app/App.alder
+++ b/app/App.alder
@@ -36,6 +36,9 @@
 isDark : Theme -> Bool
 isDark = \t -> match t { Light -> False, Dark -> True }
 
+toggled : Theme -> Theme
+toggled = ?toggled
+
 -- Every Log out button is on a page that requires a user, so a browser test cannot tell this
 -- Redirect from a Rerender (book 10, "The logout redirect"): the test calls update itself.
 test "Logout clears the user and goes to log in" = {
app/App.alder:4:1: warning[W0001]: import `Theme` is unused
  import Theme
  ^^^^^^^^^^^^
app/App.alder:40:11: warning[W0002]: hole ?toggled has type App.Theme -> App.Theme
  toggled = ?toggled
            ^^^^^^^^
[exit 0]
$ alder fill --decl App.toggled - <<'EOF'
\t -> match t { Light -> Dark, Dark -> Light }
EOF
edited app/App.alder
--- a/app/App.alder
+++ b/app/App.alder
@@ -37,7 +37,7 @@
 isDark = \t -> match t { Light -> False, Dark -> True }
 
 toggled : Theme -> Theme
-toggled = ?toggled
+toggled = \t -> match t { Light -> Dark, Dark -> Light }
 
 -- Every Log out button is on a page that requires a user, so a browser test cannot tell this
 -- Redirect from a Rerender (book 10, "The logout redirect"): the test calls update itself.
app/App.alder:4:1: warning[W0001]: import `Theme` is unused
  import Theme
  ^^^^^^^^^^^^
[exit 0]
$ alder new test App "the theme flips"
edited app/App.alder
--- a/app/App.alder
+++ b/app/App.alder
@@ -45,3 +45,7 @@
   let r <- Test.run (update Logout { user: Just (User.UserId "me"), theme: Light })
   Task.succeed ({ value: r.value, trace: r.trace } == { value: Ok (Redirect (Auth.Login {})), trace: ["Session.put"] })
 }
+
+test "the theme flips" = {
+  ?test
+}
app/App.alder:4:1: warning[W0001]: import `Theme` is unused
  import Theme
  ^^^^^^^^^^^^
app/App.alder:50:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'App."the theme flips"' - <<'EOF'
isDark (toggled Light) && not (isDark (toggled Dark))
EOF
app/App.alder:4:1: warning[W0001]: import `Theme` is unused
  import Theme
  ^^^^^^^^^^^^
app/App.alder:50:3: error[E0003]: expected Task Bool, got Bool; a block test is a Task: end it with `Task.succeed (…)`
    isDark (toggled Light) && not (isDark (toggled Dark))
    ^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^
alder fill: the result does not check (the errors above): nothing was written
[exit 1]
```

The harness stopped at line 11 of your script: `alder fill 'App."the theme flips"' - <<'EOF'`: it exited 1. The commands after it were not run.

Your script stopped at command 6; the app is as the commands before it left it; continue.

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/App.alder:4:1: warning[W0001]: import `Theme` is unused
  import Theme
  ^^^^^^^^^^^^
app/App.alder:50:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
alder build: the program has 1 typed hole(s); fill them before building
```
