### Round 3

```text
$ alder fill 'App."pressing SwitchTheme"' - --hole ?expected <<'EOF'
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
   let r <- Test.run (update SwitchTheme { user: Nothing, theme: Light })
-  Task.succeed ?expected
+  Task.succeed ({ value: r.value, trace: r.trace } == { value: Ok Rerender, trace: ["Session.put"] })
 }
[exit 0]
$ alder fill --decl Notes.Index.view - <<'EOF'
Layout.shell {
  title: "Notes",
  dark: Theme.isDark session.theme,
  children: [
    <h1>Notes</h1>,
    <form submit={Add}>
      <Errors form />
      <al-input field="title" label="Title" />
      <al-textarea field="body" label="Body" />
      <al-button type="submit">Add note</al-button>
    </form>,
    <ul id="notes">
      {for n in model.notes -> <li><a class="title" href={Notes.Show { id: n.id }}>{n.title}</a></li>}
    </ul>
  ]
}
EOF
app/Notes/Index.alder:47:61: error[E0002]: unknown `Notes.Show`
          {for n in model.notes -> <li><a class="title" href={Notes.Show { id: n.id }}>{n.title}</a></li>}
                                                              ^^^^^^^^^^
alder fill: the result does not check (the errors above): nothing was written
[exit 1]
```

The harness stopped at line 7 of your script: `alder fill --decl Notes.Index.view - <<'EOF'`: it exited 1. The commands after it were not run.

Your script stopped at command 2; the app is as the commands before it left it; continue.

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Auth/Login.alder:47:33: warning[W0002]: hole ?afterLogin has type Page
          Task.succeed (Redirect (?afterLogin))
                                  ^^^^^^^^^^^
app/Auth/Login.alder:86:85: warning[W0002]: hole ?afterLogin has type Page
    Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
                                                                                      ^^^^^^^^^^^
app/Notes/Index.alder:5:1: warning[W0001]: import `Theme` is unused
  import Theme
  ^^^^^^^^^^^^
alder build: the program has 2 typed hole(s); fill them before building
```
