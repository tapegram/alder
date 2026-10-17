### Round 4

```text
$ alder fill 'Teams.Index."the list shows the rows, newest first"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
  let _ <- Team.make me (Text.NonEmpty "Second")
  let _ <- Team.make me (Text.NonEmpty "First")
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Teams.Index {})
  Task.succeed (Test.texts "#teams .name" page.doc == ["First", "Second"])
}
EOF
alder fill: `Teams.Index."the list shows the rows, newest first"` has no typed hole; to rewrite it whole, `alder fill --decl 'Teams.Index."the list shows the rows, newest first"' -` (`alder fill --list` lists the app's holes)
[exit 1]
```

The harness stopped at line 2 of your script: `alder fill 'Teams.Index."the list shows the rows, newest first"' - <<'EOF'`: it exited 1. The commands after it were not run.

Your script stopped at command 1; the app is as the commands before it left it; continue.

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Invites/Index.alder:64:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Invites/Index.alder:78:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Teams/Index.alder:85:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Teams/Show.alder:75:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Teams/Show.alder:97:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
alder build: the program has 5 typed hole(s); fill them before building
```
