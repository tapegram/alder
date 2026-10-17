### Round 1

```text
$ alder fill --list
Auth.Login update.Submit ?afterLogin : Page
Auth.Login "logging in through the form" ?afterLogin : Page
[exit 0]
$ alder new decl Layout shell "{ title: Text, theme: Theme.Theme, children: List (Html a) } -> Html a"
edited app/Layout.alder
--- a/app/Layout.alder
+++ b/app/Layout.alder
@@ -1,5 +1,7 @@
 module Layout
 
+import Theme
+
 Page : { title: Text, children: List (Html a) } -> Html a
 Page props =
   <html>
@@ -15,3 +17,6 @@
       </al-page>
     </body>
   </html>
+
+shell : { title: Text, theme: Theme.Theme, children: List (Html a) } -> Html a
+shell = ?shell
app/Layout.alder:22:9: warning[W0002]: hole ?shell has type { title: Text, theme: Theme, children: List (Html a) } -> Html a
  shell = ?shell
          ^^^^^^
[exit 0]
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
app/Layout.alder:31:35: error[E0002]: unknown `App.SwitchTheme`
          <button id="theme" click={App.SwitchTheme}>{if props.theme == Dark then "Switch to light" else "Switch to dark"}</button>
                                    ^^^^^^^^^^^^^^^
alder fill: the result does not check (the errors above): nothing was written
[exit 1]
```

The harness stopped at line 3 of your script: `alder fill Layout.shell - <<'EOF'`: it exited 1. The commands after it were not run.

Your script stopped at command 3; the app is as the commands before it left it; continue.

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Auth/Login.alder:47:33: warning[W0002]: hole ?afterLogin has type Page
          Task.succeed (Redirect (?afterLogin))
                                  ^^^^^^^^^^^
app/Auth/Login.alder:86:85: warning[W0002]: hole ?afterLogin has type Page
    Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
                                                                                      ^^^^^^^^^^^
app/Layout.alder:22:9: warning[W0002]: hole ?shell has type { title: Text, theme: Theme, children: List (Html a) } -> Html a
  shell = ?shell
          ^^^^^^
alder build: the program has 3 typed hole(s); fill them before building
```
