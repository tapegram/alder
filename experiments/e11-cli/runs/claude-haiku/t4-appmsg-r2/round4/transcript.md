### Round 4

```text
$ alder edit session add-field theme:Theme.Theme=Light
edited app/App.alder
--- a/app/App.alder
+++ b/app/App.alder
@@ -4,7 +4,7 @@
 import Theme
 import User
 
-session { user: Maybe User.UserId }
+session { user: Maybe User.UserId, theme: Theme.Theme = Light }
 
 login = Auth.Login
 
@@ -34,7 +34,7 @@
 -- Every Log out button is on a page that requires a user, so a browser test cannot tell this
 -- Redirect from a Rerender (book 10, "The logout redirect"): the test calls update itself.
 test "Logout clears the user and goes to log in" = {
-  let r <- Test.run (update Logout { user: Just (User.UserId "me") })
+  let r <- Test.run (update Logout { user: Just (User.UserId "me"), theme: Light })
   Task.succeed ({ value: r.value, trace: r.trace } == { value: Ok (Redirect (Auth.Login {})), trace: ["Session.put"] })
 }
 
[exit 0]
$ alder fill --decl 'App."Logout clears the user and goes to log in"' - <<'EOF'
{
  let r <- Test.run (update Logout { user: Just (User.UserId "me"), theme: Light })
  Task.succeed ({ value: r.value, trace: r.trace } == { value: Ok (Redirect (Auth.Login {})), trace: ["Session.put"] })
}
EOF
rewriting a test: `alder test --mutate` is the check that it still tests something
[exit 0]
$ alder new msg App SwitchTheme
edited app/App.alder
--- a/app/App.alder
+++ b/app/App.alder
@@ -10,12 +10,15 @@
 
 msg Logout
 
+msg SwitchTheme
+
 update : Msg -> Session -> Task Nav
 update msg session = match msg {
   Logout p -> {
     let _ <- Session.put (\s -> { s | user: Nothing })
     Task.succeed (Redirect (Auth.Login {}))
   },
+  SwitchTheme p -> ?switchTheme,
 }
 
 error : Error -> Html Never
@@ -41,3 +44,11 @@
 test "the theme flips" = {
   { Task.succeed (Theme.flip Light == Dark) }
 }
+
+-- A message of App is sent from any page (`click={App.SwitchTheme}`), so the test calls update itself.
+-- Fill ?expected with what SwitchTheme must do, as `{ value: r.value, trace: r.trace } ==
+-- { value: Ok Rerender, trace: ["Session.put"] }`: its answer and the effects it ran.
+test "pressing SwitchTheme" = {
+  let r <- Test.run (update SwitchTheme { user: Nothing, theme: Light })
+  Task.succeed ?expected
+}
app/App.alder:21:20: warning[W0002]: hole ?switchTheme has type Task Nav
    SwitchTheme p -> ?switchTheme,
                     ^^^^^^^^^^^^
app/App.alder:53:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill --decl App.update - <<'EOF'
\msg session -> match msg {
  SwitchTheme p -> {
    let _ <- Session.put (\s -> { s | theme: Theme.flip s.theme })
    Task.succeed Rerender
  },
  Logout p -> {
    let _ <- Session.put (\s -> { s | user: Nothing })
    Task.succeed (Redirect (Auth.Login {}))
  },
}
EOF
alder fill: the parameters `msg session` are bound already: send the body without `\msg session ->`
[exit 1]
```

The harness stopped at line 14 of your script: `alder fill --decl App.update - <<'EOF'`: it exited 1. The commands after it were not run.

Your script stopped at command 4; the app is as the commands before it left it; continue.

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/App.alder:21:20: warning[W0002]: hole ?switchTheme has type Task Nav
    SwitchTheme p -> ?switchTheme,
                     ^^^^^^^^^^^^
app/App.alder:53:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Auth/Login.alder:47:33: warning[W0002]: hole ?afterLogin has type Page
          Task.succeed (Redirect (?afterLogin))
                                  ^^^^^^^^^^^
app/Auth/Login.alder:86:85: warning[W0002]: hole ?afterLogin has type Page
    Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
                                                                                      ^^^^^^^^^^^
alder build: the program has 4 typed hole(s); fill them before building
```
