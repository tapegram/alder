### Round 3

```text
$ alder fill 'Profile."submitting Rename"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Profile {})
  let r <- Test.submit Rename { name: Text.NonEmpty "name-2" }
  let after <- Db.get User me
  Task.succeed ({ answer: r.answer, name: Maybe.map (\u -> u.name) after } == { answer: Rerendered, name: Just (Text.NonEmpty "name-2") })
}
EOF
app/Profile.alder:64:17: error[E0003]: cannot compare `Answer` with `Task Bool`: `==` needs both sides to have the same type
    Task.succeed (r.answer == {
                  ^
alder fill: the result does not check (the errors above): nothing was written
[exit 1]
```

The harness stopped at line 2 of your script: `alder fill 'Profile."submitting Rename"' - <<'EOF'`: it exited 1. The commands after it were not run.

Your script stopped at command 1; the app is as the commands before it left it; continue.

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Profile.alder:64:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
alder build: the program has 1 typed hole(s); fill them before building
```
