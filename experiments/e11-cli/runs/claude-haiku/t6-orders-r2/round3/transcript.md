### Round 3

```text
$ alder remove page Orders.Index
removed app/Orders/Index.alder
[exit 0]
$ alder remove page Orders.Show
removed app/Orders/Show.alder
[exit 0]
$ alder new page Orders.Index '/' --blank --requires user
wrote app/Orders/Index.alder
app/Orders/Index.alder:8:23: warning[W0002]: hole ?load has type Query Orders.Index.Model
  load params session = ?load
                        ^^^^^
app/Orders/Index.alder:11:22: warning[W0002]: hole ?view has type Html Orders.Index.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder fill --decl Orders.Index.view - <<'EOF'
\model session -> <Layout.Page title="Orders"><h1>Orders</h1></Layout.Page>
EOF
alder fill: the parameters `model session` are bound already: send the body without `\model session ->`
[exit 1]
```

The harness stopped at line 5 of your script: `alder fill --decl Orders.Index.view - <<'EOF'`: it exited 1. The commands after it were not run.

Your script stopped at command 4; the app is as the commands before it left it; continue.

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Auth/Login.alder:47:33: warning[W0002]: hole ?afterLogin has type Page
          Task.succeed (Redirect (?afterLogin))
                                  ^^^^^^^^^^^
app/Auth/Login.alder:86:85: warning[W0002]: hole ?afterLogin has type Page
    Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
                                                                                      ^^^^^^^^^^^
app/Orders/Index.alder:8:23: warning[W0002]: hole ?load has type Query Orders.Index.Model
  load params session = ?load
                        ^^^^^
app/Orders/Index.alder:11:22: warning[W0002]: hole ?view has type Html Orders.Index.Msg
  view model session = ?view
                       ^^^^^
alder build: the program has 4 typed hole(s); fill them before building
```
