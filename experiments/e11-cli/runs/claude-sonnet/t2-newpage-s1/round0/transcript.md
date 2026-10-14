### Round 0

```text
$ alder new page Profile /profile --blank --requires user
wrote app/Profile.alder
app/Profile.alder:8:23: warning[W0002]: hole ?load has type Query Profile.Model
  load params session = ?load
                        ^^^^^
app/Profile.alder:11:22: warning[W0002]: hole ?view has type Html Profile.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder fill --decl Profile.Model - <<'EOF'
{ name: Text.NonEmpty, email: Text.Email, total: Int, finished: Int }
EOF
edited app/Profile.alder
--- a/app/Profile.alder
+++ b/app/Profile.alder
@@ -2,7 +2,7 @@
 
 page "/profile" { } requires user
 
-type Model = { }
+type Model = { name: Text.NonEmpty, email: Text.Email, total: Int, finished: Int }
 
 load : Params -> Session -> Query Model
 load params session = ?load
app/Profile.alder:8:23: warning[W0002]: hole ?load has type Query Profile.Model
  load params session = ?load
                        ^^^^^
app/Profile.alder:11:22: warning[W0002]: hole ?view has type Html Profile.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder fill Profile.load - <<'EOF'
\params session -> {
  let found <- Db.get User session.user
  let total <- Db.mine Todo |> Db.count
  let finished <- Db.mine Todo |> Db.where (Todo.done.eq True) |> Db.count
  match found {
    Just u -> Query.succeed { name: u.name, email: u.email, total: total, finished: finished },
    Nothing -> Query.notFound,
  }
}
EOF
app/Profile.alder:11:23: error[E0003]: expected Query Profile.Model, got _ -> _ -> Query { name: Text.NonEmpty, email: Text.Email, total: Int, finished: Int }
  load params session = \params session -> {
                        ^
app/Profile.alder:12:28: error[E0003]: cannot infer the record type of `session`; add a type annotation
    let found <- Db.get User session.user
                             ^^^^^^^^^^^^
app/Profile.alder:22:22: warning[W0002]: hole ?view has type Html Profile.Msg
  view model session = ?view
                       ^^^^^
alder fill: the result does not check (the errors above): nothing was written
[exit 1]
```

The harness stopped at line 5 of your script: `alder fill Profile.load - <<'EOF'`: it exited 1. The commands after it were not run.

Your script stopped at command 3; the app is as the commands before it left it; continue.

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Profile.alder:8:23: warning[W0002]: hole ?load has type Query Profile.Model
  load params session = ?load
                        ^^^^^
app/Profile.alder:11:22: warning[W0002]: hole ?view has type Html Profile.Msg
  view model session = ?view
                       ^^^^^
alder build: the program has 2 typed hole(s); fill them before building
```
