### Round 1

```text
$ alder fill Auth.Login.update - --hole ?afterLogin <<'EOF'
Notes.Index {}
EOF
edited app/Auth/Login.alder
--- a/app/Auth/Login.alder
+++ b/app/Auth/Login.alder
@@ -44,7 +44,7 @@
     match found {
       Just id -> {
         let _ <- Session.put (\s -> { s | user: Just id })
-        Task.succeed (Redirect (?afterLogin))
+        Task.succeed (Redirect (Notes.Index {}))
       },
       Nothing -> Task.succeed (Reject "Email or password is incorrect."),
     }
app/Auth/Login.alder:86:85: warning[W0002]: hole ?afterLogin has type Page
    Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
                                                                                      ^^^^^^^^^^^
[exit 0]
$ alder new page Notes.Show "/notes/{id}" --show Note --requires user
wrote app/Notes/Show.alder
[exit 0]
$ alder fill --decl Notes.Show.Model - <<'EOF'
{ note: Note }
EOF
app/Notes/Show.alder:15:31: error[E0003]: record `Notes.Show.Model` is missing field(s): note
      Just row -> Query.succeed { row: row },
                                ^^^^^^^^^^^^
app/Notes/Show.alder:15:33: error[E0002]: no field `row` on `Notes.Show.Model`
      Just row -> Query.succeed { row: row },
                                  ^^^^^^^^
app/Notes/Show.alder:28:14: error[E0002]: no field `row` on `Notes.Show.Model`
          <dd>{model.row.id}</dd>
               ^^^^^^^^^
app/Notes/Show.alder:28:14: error[E0003]: cannot infer the record type of `model.row`; add a type annotation
          <dd>{model.row.id}</dd>
               ^^^^^^^^^^^^
app/Notes/Show.alder:30:14: error[E0002]: no field `row` on `Notes.Show.Model`
          <dd>{model.row.title}</dd>
               ^^^^^^^^^
app/Notes/Show.alder:30:14: error[E0003]: cannot infer the record type of `model.row`; add a type annotation
          <dd>{model.row.title}</dd>
               ^^^^^^^^^^^^^^^
app/Notes/Show.alder:32:14: error[E0002]: no field `row` on `Notes.Show.Model`
          <dd>{model.row.body}</dd>
               ^^^^^^^^^
app/Notes/Show.alder:32:14: error[E0003]: cannot infer the record type of `model.row`; add a type annotation
          <dd>{model.row.body}</dd>
               ^^^^^^^^^^^^^^
app/Notes/Show.alder:34:14: error[E0002]: no field `row` on `Notes.Show.Model`
          <dd>{model.row.owner}</dd>
               ^^^^^^^^^
app/Notes/Show.alder:34:14: error[E0003]: cannot infer the record type of `model.row`; add a type annotation
          <dd>{model.row.owner}</dd>
               ^^^^^^^^^^^^^^^
app/Notes/Show.alder:36:40: error[E0002]: no field `row` on `Notes.Show.Model`
          <dd>{Time.format "Jan 2, 2006" model.row.created}</dd>
                                         ^^^^^^^^^
app/Notes/Show.alder:36:40: error[E0003]: cannot infer the record type of `model.row`; add a type annotation
          <dd>{Time.format "Jan 2, 2006" model.row.created}</dd>
                                         ^^^^^^^^^^^^^^^^^
alder fill: the result does not check (the errors above): nothing was written
[exit 1]
```

The harness stopped at line 8 of your script: `alder fill --decl Notes.Show.Model - <<'EOF'`: it exited 1. The commands after it were not run.

Your script stopped at command 3; the app is as the commands before it left it; continue.

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Auth/Login.alder:86:85: warning[W0002]: hole ?afterLogin has type Page
    Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
                                                                                      ^^^^^^^^^^^
app/Notes/Index.alder:32:12: warning[W0002]: hole ?add has type Task Nav
    Add p -> ?add,
             ^^^^
app/Notes/Index.alder:33:15: warning[W0002]: hole ?delete has type Task Nav
    Delete p -> ?delete,
                ^^^^^^^
app/Notes/Index.alder:50:34: warning[W0002]: hole ?deleteId has type Note.NoteId
    <al-button click={Delete { id: ?deleteId }}>Delete</al-button>
                                   ^^^^^^^^^
app/Notes/Index.alder:63:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Notes/Index.alder:84:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
alder build: the program has 6 typed hole(s); fill them before building
```
