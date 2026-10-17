### Round 1

```text
$ alder fill 'Line."an order total sums its lines"' - <<'EOF'
{
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 250 }
  let cake <- Db.insert Product { name: Text.NonEmpty "Cake", price: Int.NonNegative 400 }
  let o <- Db.insert Order { owner: ann }
  let _ <- Db.insert Line { order: o, product: tea, quantity: Int.Positive 2 }
  let _ <- Db.insert Line { order: o, product: cake, quantity: Int.Positive 1 }
  let total <- orderTotal o
  Task.succeed (total == 900)
}
EOF
edited app/Line.alder
--- a/app/Line.alder
+++ b/app/Line.alder
@@ -47,5 +47,12 @@
 }
 
 test "an order total sums its lines" = {
-  ?test
+  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
+  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 250 }
+  let cake <- Db.insert Product { name: Text.NonEmpty "Cake", price: Int.NonNegative 400 }
+  let o <- Db.insert Order { owner: ann }
+  let _ <- Db.insert Line { order: o, product: tea, quantity: Int.Positive 2 }
+  let _ <- Db.insert Line { order: o, product: cake, quantity: Int.Positive 1 }
+  let total <- orderTotal o
+  Task.succeed (total == 900)
 }
[exit 0]
$ alder fill Auth.Login.update.Submit - --hole ?afterLogin <<'EOF'
Orders.Index {}
EOF
app/Auth/Login.alder:47:33: error[E0002]: unknown `Orders.Index`
          Task.succeed (Redirect (Orders.Index {}))
                                  ^^^^^^^^^^^^
alder fill: the result does not check (the errors above): nothing was written
[exit 1]
```

The harness stopped at line 16 of your script: `alder fill Auth.Login.update.Submit - --hole ?afterLogin <<'EOF'`: it exited 1. The commands after it were not run.

Your script stopped at command 2; the app is as the commands before it left it; continue.

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
