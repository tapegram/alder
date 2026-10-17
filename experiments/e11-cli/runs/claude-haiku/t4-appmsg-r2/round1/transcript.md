### Round 1

```text
$ alder fill Auth.Login.update.Submit - --hole ?afterLogin <<'EOF'
Notes.Index {}
EOF
app/Auth/Login.alder:47:33: error[E0002]: unknown `Notes.Index`
          Task.succeed (Redirect (Notes.Index {}))
                                  ^^^^^^^^^^^
alder fill: the result does not check (the errors above): nothing was written
[exit 1]
```

The harness stopped at line 2 of your script: `alder fill Auth.Login.update.Submit - --hole ?afterLogin <<'EOF'`: it exited 1. The commands after it were not run.

Your script stopped at command 1; the app is as the commands before it left it; continue.

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Auth/Login.alder:47:33: warning[W0002]: hole ?afterLogin has type Page
          Task.succeed (Redirect (?afterLogin))
                                  ^^^^^^^^^^^
app/Auth/Login.alder:86:85: warning[W0002]: hole ?afterLogin has type Page
    Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
                                                                                      ^^^^^^^^^^^
alder build: the program has 2 typed hole(s); fill them before building
```
