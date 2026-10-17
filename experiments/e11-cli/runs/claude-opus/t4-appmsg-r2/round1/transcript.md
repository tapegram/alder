### Round 1

```text
$ alder new decl Theme isDark "Theme -> Bool"
edited app/Theme.alder
--- a/app/Theme.alder
+++ b/app/Theme.alder
@@ -20,6 +20,9 @@
   Dark -> Just "dark",
 }
 
+isDark : Theme -> Bool
+isDark = ?isDark
+
 test "flip swaps the theme" = {
   let flipped = List.map flip [Light, Dark]
   let labels = List.map buttonLabel [Light, Dark]
app/Theme.alder:24:10: warning[W0002]: hole ?isDark has type Theme -> Bool
  isDark = ?isDark
           ^^^^^^^
[exit 0]
$ alder fill Theme.isDark - <<'EOF'
\t -> t == Dark
EOF
edited app/Theme.alder
--- a/app/Theme.alder
+++ b/app/Theme.alder
@@ -21,7 +21,7 @@
 }
 
 isDark : Theme -> Bool
-isDark = ?isDark
+isDark = \t -> t == Dark
 
 test "flip swaps the theme" = {
   let flipped = List.map flip [Light, Dark]
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
@@ -37,3 +40,11 @@
   let r <- Test.run (update Logout { user: Just (User.UserId "me"), theme: Light })
   Task.succeed ({ value: r.value, trace: r.trace } == { value: Ok (Redirect (Auth.Login {})), trace: ["Session.put"] })
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
app/App.alder:49:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill App.update.SwitchTheme - <<'EOF'
{
  let _ <- Session.put (\s -> { s | theme: Theme.flip s.theme })
  Task.succeed Rerender
}
EOF
edited app/App.alder
--- a/app/App.alder
+++ b/app/App.alder
@@ -18,7 +18,10 @@
     let _ <- Session.put (\s -> { s | user: Nothing })
     Task.succeed (Redirect (Auth.Login {}))
   },
-  SwitchTheme p -> ?switchTheme,
+  SwitchTheme p -> {
+    let _ <- Session.put (\s -> { s | theme: Theme.flip s.theme })
+    Task.succeed Rerender
+  },
 }
 
 error : Error -> Html Never
app/App.alder:52:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder new decl Layout shell "{ title: Text, dark: Bool, children: List (Html a) } -> Html a"
edited app/Layout.alder
--- a/app/Layout.alder
+++ b/app/Layout.alder
@@ -15,3 +15,6 @@
       </al-page>
     </body>
   </html>
+
+shell : { title: Text, dark: Bool, children: List (Html a) } -> Html a
+shell = ?shell
app/Layout.alder:20:9: warning[W0002]: hole ?shell has type { title: Text, dark: Bool, children: List (Html a) } -> Html a
  shell = ?shell
          ^^^^^^
[exit 0]
$ alder fill Layout.shell - <<'EOF'
\props -> <html>
  <head>
    <meta charset="utf-8" />
    <title>{props.title}</title>
  </head>
  <body class={if props.dark then Just "dark" else Nothing}>
    <header>
      <button id="theme" click={App.SwitchTheme}>{if props.dark then "Switch to light" else "Switch to dark"}</button>
      <button click={App.Logout}>Log out</button>
    </header>
    <main>{props.children}</main>
  </body>
</html>
EOF
edited app/Layout.alder
--- a/app/Layout.alder
+++ b/app/Layout.alder
@@ -17,4 +17,16 @@
   </html>
 
 shell : { title: Text, dark: Bool, children: List (Html a) } -> Html a
-shell = ?shell
+shell = \props -> <html>
+  <head>
+    <meta charset="utf-8" />
+    <title>{props.title}</title>
+  </head>
+  <body class={if props.dark then Just "dark" else Nothing}>
+    <header>
+      <button id="theme" click={App.SwitchTheme}>{if props.dark then "Switch to light" else "Switch to dark"}</button>
+      <button click={App.Logout}>Log out</button>
+    </header>
+    <main>{props.children}</main>
+  </body>
+</html>
[exit 0]
$ alder new table Note title:Text.NonEmpty body:Text owner:User.UserId@ref=User@owner created:Time@created --no-timestamps
wrote app/Note.alder
wrote migrations/0002_create_note.sql
  create table Note
  add index on Note.owner
edited migrations/schema.json
[exit 0]
$ alder new page Notes.Index / --blank --requires user
wrote app/Notes/Index.alder
app/Notes/Index.alder:8:23: warning[W0002]: hole ?load has type Query Notes.Index.Model
  load params session = ?load
                        ^^^^^
app/Notes/Index.alder:11:22: warning[W0002]: hole ?view has type Html Notes.Index.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder fill --decl Notes.Index.Model - <<'EOF'
{ notes: List Note }
EOF
edited app/Notes/Index.alder
--- a/app/Notes/Index.alder
+++ b/app/Notes/Index.alder
@@ -1,8 +1,10 @@
 module Notes.Index
 
+import Note
+
 page "/" { } requires user
 
-type Model = { }
+type Model = { notes: List Note }
 
 load : Params -> Session -> Query Model
 load params session = ?load
app/Notes/Index.alder:10:23: warning[W0002]: hole ?load has type Query Notes.Index.Model
  load params session = ?load
                        ^^^^^
app/Notes/Index.alder:13:22: warning[W0002]: hole ?view has type Html Notes.Index.Msg
  view model session = ?view
                       ^^^^^
added import Note to app/Notes/Index.alder
[exit 0]
$ alder fill Notes.Index.load - <<'EOF'
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
$ alder fill Notes.Index.view - <<'EOF'
Layout.shell { title: "Notes", dark: Theme.isDark session.theme, children: [] }
EOF
edited app/Notes/Index.alder
--- a/app/Notes/Index.alder
+++ b/app/Notes/Index.alder
@@ -1,6 +1,8 @@
 module Notes.Index
 
+import Layout
 import Note
+import Theme
 
 page "/" { } requires user
 
@@ -13,7 +15,7 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = ?view
+view model session = Layout.shell { title: "Notes", dark: Theme.isDark session.theme, children: [] }
 
 test "a visitor is sent to log in" = {
   let r <- Test.open (Notes.Index {})
added import Layout to app/Notes/Index.alder
added import Theme to app/Notes/Index.alder
[exit 0]
$ alder new form Notes.Index NewNote title:Text.NonEmpty body:Text --msg Add
alder new form: `Notes.Index`'s `view` is not one element over several lines (write it as `<Layout.Page …>` … `</Layout.Page>` on several lines, with `alder fill --decl Notes.Index.view -`), or add the markup yourself
[exit 1]
```

The harness stopped at line 51 of your script: `alder new form Notes.Index NewNote title:Text.NonEmpty body:Text --msg Add`: it exited 1. The commands after it were not run.

Your script stopped at command 12; the app is as the commands before it left it; continue.

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/App.alder:52:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Auth/Login.alder:47:33: warning[W0002]: hole ?afterLogin has type Page
          Task.succeed (Redirect (?afterLogin))
                                  ^^^^^^^^^^^
app/Auth/Login.alder:86:85: warning[W0002]: hole ?afterLogin has type Page
    Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
                                                                                      ^^^^^^^^^^^
alder build: the program has 3 typed hole(s); fill them before building
```
