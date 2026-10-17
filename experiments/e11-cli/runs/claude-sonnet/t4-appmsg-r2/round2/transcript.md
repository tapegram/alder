### Round 2

```text
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
@@ -37,3 +40,11 @@
   let r <- Test.run (update Logout { user: Just (User.UserId "me"), theme: Light })
   Task.succeed ({ value: r.value, trace: r.trace } == { value: Ok (Redirect (Auth.Login {})), trace: ["Session.put"] })
 }
+
+-- A message of App is sent from any page (`click={App.SwitchTheme}`), so the test calls update itself.
+-- Fill ?expected with what SwitchTheme must do, as `{ value: r.value, trace: r.trace } ==
+-- { value: Ok Rerender, trace: ["Session.put"] }`: its answer and the effects it ran.
+test "pressing SwitchTheme" = {
+  let r <- Test.run (update SwitchTheme { theme: Light, user: Nothing })
+  Task.succeed ?expected
+}
app/App.alder:21:20: warning[W0002]: hole ?switchTheme has type Task Nav
    SwitchTheme p -> ?switchTheme,
                     ^^^^^^^^^^^^
app/App.alder:49:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill --list
App update.SwitchTheme ?switchTheme : Task Nav
App "pressing SwitchTheme" ?expected : Bool
Auth.Login update.Submit ?afterLogin : Page
Auth.Login "logging in through the form" ?afterLogin : Page
Layout shell ?shell : { title: Text, theme: Theme, children: List (Html a) } -> Html a
[exit 0]
```

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/App.alder:21:20: warning[W0002]: hole ?switchTheme has type Task Nav
    SwitchTheme p -> ?switchTheme,
                     ^^^^^^^^^^^^
app/App.alder:49:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Auth/Login.alder:47:33: warning[W0002]: hole ?afterLogin has type Page
          Task.succeed (Redirect (?afterLogin))
                                  ^^^^^^^^^^^
app/Auth/Login.alder:86:85: warning[W0002]: hole ?afterLogin has type Page
    Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
                                                                                      ^^^^^^^^^^^
app/Layout.alder:22:9: warning[W0002]: hole ?shell has type { title: Text, theme: Theme, children: List (Html a) } -> Html a
  shell = ?shell
          ^^^^^^
alder build: the program has 5 typed hole(s); fill them before building
```
