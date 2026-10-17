### Round 3

```text
$ alder fill Layout.shell - <<'EOF'
\props -> <html>
  <head>
    <meta charset="utf-8" />
    <title>{props.title}</title>
    <link rel="stylesheet" href={Url.external "/static/al/al.css"} />
  </head>
  <body class={if props.theme == Dark then Just "dark" else Nothing}>
    <main>
      <div>
        <button id="theme" click={App.SwitchTheme}>{if props.theme == Dark then "Switch to light" else "Switch to dark"}</button>
        <button click={App.Logout}>Log out</button>
      </div>
      {props.children}
    </main>
  </body>
</html>
EOF
edited app/Layout.alder
--- a/app/Layout.alder
+++ b/app/Layout.alder
@@ -19,4 +19,19 @@
   </html>
 
 shell : { title: Text, theme: Theme.Theme, children: List (Html a) } -> Html a
-shell = ?shell
+shell = \props -> <html>
+  <head>
+    <meta charset="utf-8" />
+    <title>{props.title}</title>
+    <link rel="stylesheet" href={Url.external "/static/al/al.css"} />
+  </head>
+  <body class={if props.theme == Dark then Just "dark" else Nothing}>
+    <main>
+      <div>
+        <button id="theme" click={App.SwitchTheme}>{if props.theme == Dark then "Switch to light" else "Switch to dark"}</button>
+        <button click={App.Logout}>Log out</button>
+      </div>
+      {props.children}
+    </main>
+  </body>
+</html>
[exit 0]
$ alder fill App.update.SwitchTheme - <<'EOF'
{
  let _ <- Session.put (\s -> { s | theme: if s.theme == Dark then Light else Dark })
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
+    let _ <- Session.put (\s -> { s | theme: if s.theme == Dark then Light else Dark })
+    Task.succeed Rerender
+  },
 }
 
 error : Error -> Html Never
app/App.alder:52:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill 'App."pressing SwitchTheme"' - <<'EOF'
{ value: r.value, trace: r.trace } == { value: Ok Rerender, trace: ["Session.put"] }
EOF
edited app/App.alder
--- a/app/App.alder
+++ b/app/App.alder
@@ -45,9 +45,7 @@
 }
 
 -- A message of App is sent from any page (`click={App.SwitchTheme}`), so the test calls update itself.
--- Fill ?expected with what SwitchTheme must do, as `{ value: r.value, trace: r.trace } ==
--- { value: Ok Rerender, trace: ["Session.put"] }`: its answer and the effects it ran.
 test "pressing SwitchTheme" = {
   let r <- Test.run (update SwitchTheme { theme: Light, user: Nothing })
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
$ alder fill --decl Notes.Index.load - <<'EOF'
\params session -> {
  let notes <- Db.mine Note |> Db.orderBy Note.created.desc |> Db.all
  Query.succeed { notes: notes }
}
EOF
alder fill: the parameters `params session` are bound already: send the body without `\params session ->`
[exit 1]
```

The harness stopped at line 32 of your script: `alder fill --decl Notes.Index.load - <<'EOF'`: it exited 1. The commands after it were not run.

Your script stopped at command 6; the app is as the commands before it left it; continue.

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Auth/Login.alder:47:33: warning[W0002]: hole ?afterLogin has type Page
          Task.succeed (Redirect (?afterLogin))
                                  ^^^^^^^^^^^
app/Auth/Login.alder:86:85: warning[W0002]: hole ?afterLogin has type Page
    Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
                                                                                      ^^^^^^^^^^^
app/Notes/Index.alder:10:23: warning[W0002]: hole ?load has type Query Notes.Index.Model
  load params session = ?load
                        ^^^^^
app/Notes/Index.alder:13:22: warning[W0002]: hole ?view has type Html Notes.Index.Msg
  view model session = ?view
                       ^^^^^
alder build: the program has 4 typed hole(s); fill them before building
```
