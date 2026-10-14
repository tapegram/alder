### Round 4

```text
$ alder new test Profile "load counts only my todos"
edited app/Profile.alder
--- a/app/Profile.alder
+++ b/app/Profile.alder
@@ -71,3 +71,7 @@
   let r <- Test.submitRaw Rename [{ name: "name", value: "" }]
   Task.succeed (r.answer == FormRejected [{ field: "name", message: "must not be empty" }])
 }
+
+test "load counts only my todos" = {
+  ?test
+}
app/Profile.alder:64:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Profile.alder:76:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Profile."load counts only my todos"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
  let other <- Db.insert User { email: Text.Email "email-2@example.com", name: Text.NonEmpty "name-2", passwordHash: "passwordhash-2" }
  let _ <- Db.insert Todo { title: Text.NonEmpty "Milk", done: True, due: Nothing, owner: me }
  let _ <- Db.insert Todo { title: Text.NonEmpty "Bread", due: Nothing, owner: me }
  let _ <- Db.insert Todo { title: Text.NonEmpty "Eggs", due: Nothing, owner: other }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Profile {})
  Task.succeed ({ total: page.model.total, done: page.model.done } == { total: 2, done: 1 })
}
EOF
edited app/Profile.alder
--- a/app/Profile.alder
+++ b/app/Profile.alder
@@ -73,5 +73,12 @@
 }
 
 test "load counts only my todos" = {
-  ?test
+  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
+  let other <- Db.insert User { email: Text.Email "email-2@example.com", name: Text.NonEmpty "name-2", passwordHash: "passwordhash-2" }
+  let _ <- Db.insert Todo { title: Text.NonEmpty "Milk", done: True, due: Nothing, owner: me }
+  let _ <- Db.insert Todo { title: Text.NonEmpty "Bread", due: Nothing, owner: me }
+  let _ <- Db.insert Todo { title: Text.NonEmpty "Eggs", due: Nothing, owner: other }
+  let _ <- Test.login { user: Just me }
+  let page <- Test.get (Profile {})
+  Task.succeed ({ total: page.model.total, done: page.model.done } == { total: 2, done: 1 })
 }
app/Profile.alder:64:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder fill 'Profile."submitting Rename"' - --hole ?expected <<'EOF'
Rerendered
EOF
edited app/Profile.alder
--- a/app/Profile.alder
+++ b/app/Profile.alder
@@ -61,7 +61,7 @@
   let _ <- Test.login { user: Just me }
   let _ <- Test.get (Profile {})
   let r <- Test.submit Rename { name: Text.NonEmpty "name-1" }
-  Task.succeed (r.answer == ?expected)
+  Task.succeed (r.answer == Rerendered)
 }
 
 test "Rename refuses an empty name" = {
[exit 0]
$ alder fill 'Profile."Rename refuses an empty name"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Profile {})
  let r <- Test.submitRaw Rename [{ name: "name", value: "" }]
  let after <- Db.get User me
  Task.succeed ({ answer: r.answer, name: Maybe.map (\u -> u.name) after } == { answer: FormRejected [{ field: "name", message: "must not be empty" }], name: Just (Text.NonEmpty "name-1") })
}
EOF
alder fill: `Profile."Rename refuses an empty name"` has no typed hole (`alder fill --list` lists the app's holes)
[exit 1]
```

The harness stopped at line 22 of your script: `alder fill 'Profile."Rename refuses an empty name"' - <<'EOF'`: it exited 1. The commands after it were not run.

Your script stopped at command 4; the app is as the commands before it left it; continue.

The harness ran `alder check`, `alder build` and `alder test` on the app as the commands left it: all pass, but the round does not: the script stopped before its end.
