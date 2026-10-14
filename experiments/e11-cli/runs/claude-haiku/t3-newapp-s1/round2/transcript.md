### Round 2

```text
$ alder fill 'Posts."submitting NewComment"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Db.insert Post { title: Text.NonEmpty "First", slug: Post.Slug "first", body: "one", published: True, author: me }
  let _ <- Test.get (Posts { slug: Post.Slug "first" })
  let r <- Test.submit NewComment { author: Text.NonEmpty "Ann", body: Text.NonEmpty "Nice post" }
  let n <- Db.from Comment |> Db.count
  Task.succeed ({ answer: r.answer, comments: n } == { answer: Rerendered, comments: 1 })
}
EOF
app/Posts.alder:65:17: error[E0003]: cannot compare `Answer` with `Task Bool`: `==` needs both sides to have the same type
    Task.succeed (r.answer == {
                  ^
alder fill: the result does not check (the errors above): nothing was written
[exit 1]
```

The harness stopped at line 1 of your script: `alder fill 'Posts."submitting NewComment"' - <<'EOF'`: it exited 1. The commands after it were not run.

Your script stopped at command 1; the app is as the commands before it left it; continue.

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Admin.alder:84:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Admin.alder:105:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Posts.alder:65:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
alder build: the program has 3 typed hole(s); fill them before building
```
