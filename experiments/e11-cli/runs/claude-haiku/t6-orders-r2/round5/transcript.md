### Round 5

```text
$ alder fill 'Orders.Index."pressing Create"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Orders.Index {})
  let r <- Test.click Create
  let made <- Db.from Order |> Db.first
  Task.succeed ({ answer: Maybe.map (\o -> Redirected (Orders.Show { id: o.id })) made, owner: Maybe.map (\o -> o.owner) made, status: Maybe.map (\o -> o.status) made } == { answer: Just r.answer, owner: Just me, status: Just Open })
}
EOF
alder fill: the hole ?expected is a `Bool`, not a test's whole block: send the expression for it, or, to replace the whole test, use `alder fill --decl 'Orders.Index."pressing Create"' -`
[exit 1]
```

The harness stopped at line 2 of your script: `alder fill 'Orders.Index."pressing Create"' - <<'EOF'`: it exited 1. The commands after it were not run.

Your script stopped at command 1; the app is as the commands before it left it; continue.

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Orders/Index.alder:66:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Orders/Show.alder:95:44: warning[W0002]: hole ?addLineProduct has type Product.ProductId
    let r <- Test.submit (AddLine { product: ?addLineProduct }) { quantity: Int.Positive 1 }
                                             ^^^^^^^^^^^^^^^
app/Orders/Show.alder:96:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Orders/Show.alder:111:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Orders/Show.alder:126:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Products/Index.alder:97:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
alder build: the program has 6 typed hole(s); fill them before building
```
