### Round 2

```text
$ alder fill --decl 'Teams.Index."NewTeam refuses an empty name"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Teams.Index {})
  let r <- Test.submitRaw NewTeam [{ name: "name", value: "" }]
  let n <- Db.from Team |> Db.count
  Task.succeed ({ answer: r.answer, teams: n } == { answer: FormRejected [{ field: "", message: "name must not be empty" }], teams: 0 })
}
EOF
edited app/Teams/Index.alder
--- a/app/Teams/Index.alder
+++ b/app/Teams/Index.alder
@@ -68,5 +68,6 @@
   let _ <- Test.login { user: Just me }
   let _ <- Test.get (Teams.Index {})
   let r <- Test.submitRaw NewTeam [{ name: "name", value: "" }]
-  Task.succeed (r.answer == FormRejected [{ field: "name", message: "must not be empty" }])
+  let n <- Db.from Team |> Db.count
+  Task.succeed ({ answer: r.answer, teams: n } == { answer: FormRejected [{ field: "", message: "name must not be empty" }], teams: 0 })
 }
rewriting a test: `alder test --mutate` is the check that it still tests something
[exit 0]
$ alder fill --decl 'Teams.Show."NewInvite refuses an empty email"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
  let _ <- Test.login { user: Just me }
  let team1 <- Db.insert Team { name: Text.NonEmpty "name-1", owner: me }
  let _ <- Test.get (Teams.Show { id: team1 })
  let r <- Test.submitRaw NewInvite [{ name: "email", value: "" }]
  let n <- Db.from Invite |> Db.count
  Task.succeed ({ answer: r.answer, invites: n } == { answer: FormRejected [{ field: "", message: "email must be an email address" }], invites: 0 })
}
EOF
edited app/Teams/Show.alder
--- a/app/Teams/Show.alder
+++ b/app/Teams/Show.alder
@@ -110,7 +110,8 @@
   let team1 <- Db.insert Team { name: Text.NonEmpty "name-1", owner: me }
   let _ <- Test.get (Teams.Show { id: team1 })
   let r <- Test.submitRaw NewInvite [{ name: "email", value: "" }]
-  Task.succeed (r.answer == FormRejected [{ field: "email", message: "must be an email address" }])
+  let n <- Db.from Invite |> Db.count
+  Task.succeed ({ answer: r.answer, invites: n } == { answer: FormRejected [{ field: "", message: "email must be an email address" }], invites: 0 })
 }
 
 test "a team's page is not found for a non-member" = {
rewriting a test: `alder test --mutate` is the check that it still tests something
[exit 0]
$ alder check
[exit 0]
$ alder test
ok   App "Logout clears the user and goes to log in"
ok   Auth.Login "an unknown email is rejected"
ok   Auth.Login "logging in through the form"
ok   Auth.Login "a signed-in user logs out here"
ok   Team "a Team keeps its owner from being deleted"
ok   Invite "a Invite keeps its team from being deleted"
ok   Member "a Member keeps its team from being deleted"
ok   Member "a Member keeps its user from being deleted"
ok   Invites.Index "a visitor is sent to log in"
ok   Invites.Index "pressing Accept"
ok   Invites.Index "pressing Decline"
ok   Invites.Index "accepting an invite makes the team visible, declining does not"
ok   Teams.Index "a visitor is sent to log in"
ok   Teams.Index "submitting NewTeam"
ok   Teams.Index "NewTeam refuses an empty name"
ok   Teams.Show "a visitor is sent to log in"
ok   Teams.Show "pressing Remove"
ok   Teams.Show "submitting NewInvite"
ok   Teams.Show "NewInvite refuses an empty email"
ok   Teams.Show "a team's page is not found for a non-member"
ok   Teams.Show "a member cannot remove another member"
ok   Teams.Show "the owner removes a member but not themselves"
22 passed, 0 failed
[exit 0]
```

The harness ran `alder check`, `alder build` and `alder test` on the app: all pass.
