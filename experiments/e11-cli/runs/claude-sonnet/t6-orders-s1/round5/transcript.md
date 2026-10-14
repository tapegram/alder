### Round 5

```text
$ alder fill Auth.Login.update.Submit - --hole ?afterLogin <<'EOF'
Orders.Index {}
EOF
edited app/Auth/Login.alder
--- a/app/Auth/Login.alder
+++ b/app/Auth/Login.alder
@@ -44,7 +44,7 @@
     match found {
       Just id -> {
         let _ <- Session.put (\s -> { s | user: Just id })
-        Task.succeed (Redirect (?afterLogin))
+        Task.succeed (Redirect (Orders.Index {}))
       },
       Nothing -> Task.succeed (Reject "Email or password is incorrect."),
     }
app/Auth/Login.alder:86:85: warning[W0002]: hole ?afterLogin has type Page
    Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
                                                                                      ^^^^^^^^^^^
[exit 0]
$ alder fill Orders.Index."pressing Create" - --hole ?expected <<'EOF'
Test.count "#orders li" after.doc == 1
EOF
alder fill: `Orders.Index.pressing Create` has no typed hole (`alder fill --list` lists the app's holes)
[exit 1]
```

The harness stopped at line 4 of your script: `alder fill Orders.Index."pressing Create" - --hole ?expected <<'EOF'`: it exited 1. The commands after it were not run.

Your script stopped at command 2; the app is as the commands before it left it; continue.

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Auth/Login.alder:86:85: warning[W0002]: hole ?afterLogin has type Page
    Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
                                                                                      ^^^^^^^^^^^
app/Orders/Index.alder:67:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Orders/Show.alder:31:16: warning[W0002]: hole ?addLine has type Task Nav
    AddLine p -> ?addLine,
                 ^^^^^^^^
app/Orders/Show.alder:40:40: warning[W0002]: hole ?addLineProduct has type Product.ProductId
    <al-button click={AddLine { product: ?addLineProduct }}>Add line</al-button>
                                         ^^^^^^^^^^^^^^^
app/Orders/Show.alder:64:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
alder build: the program has 5 typed hole(s); fill them before building
```
