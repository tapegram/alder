### Round 5

```text
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
    <link rel="stylesheet" href={Url.external "/static/al/al.css"} />
    <link rel="stylesheet" href={Url.external "/static/app.css"} />
  </head>
  <body class={if props.dark then Just "dark" else Nothing}>
    <al-page width="narrow">
      <nav>
        <al-button id="theme" variant="outline" size="sm" click={App.SwitchTheme}>{if props.dark then "Switch to light" else "Switch to dark"}</al-button>
        <al-button id="logout" variant="ghost" size="sm" click={App.Logout}>Log out</al-button>
      </nav>
      <main>{props.children}</main>
    </al-page>
  </body>
</html>
EOF
edited app/Layout.alder
--- a/app/Layout.alder
+++ b/app/Layout.alder
@@ -17,4 +17,20 @@
   </html>
 
 shell : { title: Text, dark: Bool, children: List (Html a) } -> Html a
-shell = ?shell
+shell = \props -> <html>
+  <head>
+    <meta charset="utf-8" />
+    <title>{props.title}</title>
+    <link rel="stylesheet" href={Url.external "/static/al/al.css"} />
+    <link rel="stylesheet" href={Url.external "/static/app.css"} />
+  </head>
+  <body class={if props.dark then Just "dark" else Nothing}>
+    <al-page width="narrow">
+      <nav>
+        <al-button id="theme" variant="outline" size="sm" click={App.SwitchTheme}>{if props.dark then "Switch to light" else "Switch to dark"}</al-button>
+        <al-button id="logout" variant="ghost" size="sm" click={App.Logout}>Log out</al-button>
+      </nav>
+      <main>{props.children}</main>
+    </al-page>
+  </body>
+</html>
[exit 0]
$ alder fill App.update.SwitchTheme - --hole ?switchTheme <<'EOF'
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
app/App.alder:56:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill 'App."pressing SwitchTheme"' - --hole ?expected <<'EOF'
({ value: r.value, trace: r.trace } == { value: Ok Rerender, trace: ["Session.put"] })
EOF
edited app/App.alder
--- a/app/App.alder
+++ b/app/App.alder
@@ -49,9 +49,7 @@
 }
 
 -- A message of App is sent from any page (`click={App.SwitchTheme}`), so the test calls update itself.
--- Fill ?expected with what SwitchTheme must do, as `{ value: r.value, trace: r.trace } ==
--- { value: Ok Rerender, trace: ["Session.put"] }`: its answer and the effects it ran.
 test "pressing SwitchTheme" = {
   let r <- Test.run (update SwitchTheme { user: Nothing, theme: Light })
-  Task.succeed ?expected
+  Task.succeed ({ value: r.value, trace: r.trace } == { value: Ok Rerender, trace: ["Session.put"] })
 }
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
$ alder new page Notes.Show '/notes/{id}' --show Note --blank --requires user
wrote app/Notes/Show.alder
app/Notes/Show.alder:11:23: warning[W0002]: hole ?load has type Query Notes.Show.Model
  load params session = ?load
                        ^^^^^
app/Notes/Show.alder:14:22: warning[W0002]: hole ?view has type Html Notes.Show.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder fill Notes.Index.load - --hole ?load <<'EOF'
{
  let _ <- Db.mine Note |> Db.orderBy Note.created.desc |> Db.all
  Query.succeed { }
}
EOF
edited app/Notes/Index.alder
--- a/app/Notes/Index.alder
+++ b/app/Notes/Index.alder
@@ -1,11 +1,16 @@
 module Notes.Index
 
+import Note
+
 page "/" { } requires user
 
 type Model = { }
 
 load : Params -> Session -> Query Model
-load params session = ?load
+load params session = {
+  let _ <- Db.mine Note |> Db.orderBy Note.created.desc |> Db.all
+  Query.succeed { }
+}
 
 view : Model -> Session -> Html Msg
 view model session = ?view
app/Notes/Index.alder:16:22: warning[W0002]: hole ?view has type Html Notes.Index.Msg
  view model session = ?view
                       ^^^^^
added import Note to app/Notes/Index.alder
[exit 0]
$ alder fill --decl Notes.Index.Model - <<'EOF'
{ notes: List Note }
EOF
app/Notes/Index.alder:12:17: error[E0003]: record `Notes.Index.Model` is missing field(s): notes
    Query.succeed { }
                  ^^^
app/Notes/Index.alder:16:22: warning[W0002]: hole ?view has type Html Notes.Index.Msg
  view model session = ?view
                       ^^^^^
alder fill: the result does not check (the errors above): nothing was written
[exit 1]
```

The harness stopped at line 45 of your script: `alder fill --decl Notes.Index.Model - <<'EOF'`: it exited 1. The commands after it were not run.

Your script stopped at command 8; the app is as the commands before it left it; continue.

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Auth/Login.alder:47:33: warning[W0002]: hole ?afterLogin has type Page
          Task.succeed (Redirect (?afterLogin))
                                  ^^^^^^^^^^^
app/Auth/Login.alder:86:85: warning[W0002]: hole ?afterLogin has type Page
    Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
                                                                                      ^^^^^^^^^^^
app/Notes/Index.alder:16:22: warning[W0002]: hole ?view has type Html Notes.Index.Msg
  view model session = ?view
                       ^^^^^
app/Notes/Show.alder:11:23: warning[W0002]: hole ?load has type Query Notes.Show.Model
  load params session = ?load
                        ^^^^^
app/Notes/Show.alder:14:22: warning[W0002]: hole ?view has type Html Notes.Show.Msg
  view model session = ?view
                       ^^^^^
alder build: the program has 5 typed hole(s); fill them before building
```
