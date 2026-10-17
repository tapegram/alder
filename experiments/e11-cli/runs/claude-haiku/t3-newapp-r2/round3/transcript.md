### Round 3

```text
$ alder fill --decl Auth.Login.update - <<'EOF'
\msg model session -> match msg {
  Submit p -> {
    let found <- Password.verify { email: p.form.email, password: p.form.password }
    match found {
      Just id -> {
        let _ <- Session.put (\s -> { s | user: Just id })
        Task.succeed (Redirect (Admin { page: 1 }))
      },
      Nothing -> Task.succeed (Reject "Email or password is incorrect."),
    }
  },
}
EOF
alder fill: the parameters `msg model session` are bound already: send the body without `\msg model session ->`
[exit 1]
```

The harness stopped at line 2 of your script: `alder fill --decl Auth.Login.update - <<'EOF'`: it exited 1. The commands after it were not run.

Your script stopped at command 1; the app is as the commands before it left it; continue.

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Home.alder:8:23: warning[W0002]: hole ?load has type Query Home.Model
  load params session = ?load
                        ^^^^^
app/Home.alder:11:22: warning[W0002]: hole ?view has type Html Home.Msg
  view model session = ?view
                       ^^^^^
app/Posts/Show.alder:10:23: warning[W0002]: hole ?load has type Query Posts.Show.Model
  load params session = ?load
                        ^^^^^
app/Posts/Show.alder:13:22: warning[W0002]: hole ?view has type Html Posts.Show.Msg
  view model session = ?view
                       ^^^^^
alder build: the program has 4 typed hole(s); fill them before building
```
