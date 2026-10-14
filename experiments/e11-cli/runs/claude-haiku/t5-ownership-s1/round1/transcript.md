### Round 1

```text
$ alder fill --decl Notes.Index.load - <<'EOF'
\params session -> {
  let notes <- Db.mine Note |> Db.orderBy Note.created.desc |> Db.all
  Query.succeed { notes: notes }
}
EOF
app/Notes/Index.alder:20:23: error[E0003]: expected Query Notes.Index.Model, got _ -> _ -> Query { notes: List Note }
  load params session = \params session -> {
                        ^
app/Notes/Index.alder:40:15: warning[W0002]: hole ?delete has type Task Nav
    Delete p -> ?delete,
                ^^^^^^^
app/Notes/Index.alder:81:36: warning[W0002]: hole ?deleteId has type Note.NoteId
      <al-button click={Delete { id: ?deleteId }}>Delete</al-button>
                                     ^^^^^^^^^
app/Notes/Index.alder:139:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
alder fill: the result does not check (the errors above): nothing was written
[exit 1]
```

The harness stopped at line 2 of your script: `alder fill --decl Notes.Index.load - <<'EOF'`: it exited 1. The commands after it were not run.

Your script stopped at command 1; the app is as the commands before it left it; continue.

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Auth/Login.alder:47:33: warning[W0002]: hole ?afterLogin has type Page
          Task.succeed (Redirect (?afterLogin))
                                  ^^^^^^^^^^^
app/Auth/Login.alder:86:85: warning[W0002]: hole ?afterLogin has type Page
    Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
                                                                                      ^^^^^^^^^^^
app/Notes/Index.alder:47:15: warning[W0002]: hole ?delete has type Task Nav
    Delete p -> ?delete,
                ^^^^^^^
app/Notes/Index.alder:88:36: warning[W0002]: hole ?deleteId has type Note.NoteId
      <al-button click={Delete { id: ?deleteId }}>Delete</al-button>
                                     ^^^^^^^^^
app/Notes/Index.alder:146:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
alder build: the program has 5 typed hole(s); fill them before building
```
