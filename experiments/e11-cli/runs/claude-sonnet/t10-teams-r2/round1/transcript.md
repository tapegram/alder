### Round 1

```text
$ alder fill --list
Auth.Login update.Submit ?afterLogin : Page
Auth.Login "logging in through the form" ?afterLogin : Page
Teams.Index load ?load : Query Teams.Index.Model
Teams.Index view ?view : Html Teams.Index.Msg
[exit 0]
$ alder new page Teams.Show '/teams/{id}' --param id:Team.TeamId --requires user --blank
wrote app/Teams/Show.alder
app/Teams/Show.alder:11:23: warning[W0002]: hole ?load has type Query Teams.Show.Model
  load params session = ?load
                        ^^^^^
app/Teams/Show.alder:14:22: warning[W0002]: hole ?view has type Html Teams.Show.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder new page Invites.Index /invites --requires user --blank
wrote app/Invites/Index.alder
app/Invites/Index.alder:8:23: warning[W0002]: hole ?load has type Query Invites.Index.Model
  load params session = ?load
                        ^^^^^
app/Invites/Index.alder:11:22: warning[W0002]: hole ?view has type Html Invites.Index.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder fill Auth.Login - --hole ?afterLogin <<'EOF'
Teams.Index {}
EOF
alder fill: `Auth.Login` names no module of this app: write `Module.decl`, such as `Home.update`
[exit 1]
```

The harness stopped at line 4 of your script: `alder fill Auth.Login - --hole ?afterLogin <<'EOF'`: it exited 1. The commands after it were not run.

Your script stopped at command 4; the app is as the commands before it left it; continue.

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Auth/Login.alder:47:33: warning[W0002]: hole ?afterLogin has type Page
          Task.succeed (Redirect (?afterLogin))
                                  ^^^^^^^^^^^
app/Auth/Login.alder:86:85: warning[W0002]: hole ?afterLogin has type Page
    Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
                                                                                      ^^^^^^^^^^^
app/Invites/Index.alder:8:23: warning[W0002]: hole ?load has type Query Invites.Index.Model
  load params session = ?load
                        ^^^^^
app/Invites/Index.alder:11:22: warning[W0002]: hole ?view has type Html Invites.Index.Msg
  view model session = ?view
                       ^^^^^
app/Teams/Index.alder:10:23: warning[W0002]: hole ?load has type Query Teams.Index.Model
  load params session = ?load
                        ^^^^^
app/Teams/Index.alder:13:22: warning[W0002]: hole ?view has type Html Teams.Index.Msg
  view model session = ?view
                       ^^^^^
app/Teams/Show.alder:11:23: warning[W0002]: hole ?load has type Query Teams.Show.Model
  load params session = ?load
                        ^^^^^
app/Teams/Show.alder:14:22: warning[W0002]: hole ?view has type Html Teams.Show.Msg
  view model session = ?view
                       ^^^^^
alder build: the program has 8 typed hole(s); fill them before building
```
