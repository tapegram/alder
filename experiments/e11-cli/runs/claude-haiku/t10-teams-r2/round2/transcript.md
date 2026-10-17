### Round 2

```text
$ alder fill Auth.Login.update.Submit - --hole ?afterLogin <<'EOF'
Teams.Index {}
EOF
edited app/Auth/Login.alder
--- a/app/Auth/Login.alder
+++ b/app/Auth/Login.alder
@@ -44,7 +44,7 @@
     match found {
       Just id -> {
         let _ <- Session.put (\s -> { s | user: Just id })
-        Task.succeed (Redirect (?afterLogin))
+        Task.succeed (Redirect (Teams.Index {}))
       },
       Nothing -> Task.succeed (Reject "Email or password is incorrect."),
     }
app/Auth/Login.alder:86:85: warning[W0002]: hole ?afterLogin has type Page
    Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
                                                                                      ^^^^^^^^^^^
[exit 0]
$ alder fill --list
Auth.Login "logging in through the form" ?afterLogin : Page
Teams.Show load ?load : Query Teams.Show.Model
Teams.Show view ?view : Html Teams.Show.Msg
[exit 0]
$ alder fill 'Auth.Login."logging in through the form"' - --hole ?afterLogin <<'EOF'
Teams.Index {}
EOF
edited app/Auth/Login.alder
--- a/app/Auth/Login.alder
+++ b/app/Auth/Login.alder
@@ -83,7 +83,7 @@
   let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
   let _ <- Test.get (Auth.Login {})
   let r <- Test.submit Credentials { email: Text.Email "me@example.com", password: Text.Secret "secret" }
-  Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
+  Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (Teams.Index {}), user: Just me })
 }
 
 test "a signed-in user logs out here" = {
[exit 0]
$ alder new decl Layout Page "{ title: Text, children: List (Html a) } -> Html a"
alder new decl: `Page` is not a value name: a value is named by a lower-case identifier, such as `waiting`
[exit 1]
```

The harness stopped at line 11 of your script: `alder new decl Layout Page "{ title: Text, children: List (Html a) } -> Html a"`: it exited 1. The commands after it were not run.

Your script stopped at command 4; the app is as the commands before it left it; continue.

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Teams/Show.alder:11:23: warning[W0002]: hole ?load has type Query Teams.Show.Model
  load params session = ?load
                        ^^^^^
app/Teams/Show.alder:14:22: warning[W0002]: hole ?view has type Html Teams.Show.Msg
  view model session = ?view
                       ^^^^^
alder build: the program has 2 typed hole(s); fill them before building
```
