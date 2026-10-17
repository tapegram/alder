### Round 5

```text
$ alder fill 'Teams.Show."the owner cannot remove themselves"' - <<'EOF'
{
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Red", owner: ann }
  let _ <- Test.login { user: Just ann }
  let u : User = { id: ann, email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "", created: Time.epoch }
  let t : Team = { id: team, name: Text.NonEmpty "Red", owner: ann }
  let r <- Test.run (update (Remove { user: ann }) { team: t, owner: u, members: [], isOwner: True } { user: ann })
  Task.succeed (r.value == Err "not found")
}
EOF
edited app/Teams/Show.alder
--- a/app/Teams/Show.alder
+++ b/app/Teams/Show.alder
@@ -144,5 +144,11 @@
 }
 
 test "the owner cannot remove themselves" = {
-  ?test
+  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
+  let team <- Db.insert Team { name: Text.NonEmpty "Red", owner: ann }
+  let _ <- Test.login { user: Just ann }
+  let u : User = { id: ann, email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "", created: Time.epoch }
+  let t : Team = { id: team, name: Text.NonEmpty "Red", owner: ann }
+  let r <- Test.run (update (Remove { user: ann }) { team: t, owner: u, members: [], isOwner: True } { user: ann })
+  Task.succeed (r.value == Err "not found")
 }
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
ok   Teams.Index "a visitor is sent to log in"
ok   Teams.Index "submitting NewTeam"
ok   Teams.Index "NewTeam refuses an empty name"
ok   Teams.Show "a visitor is sent to log in"
ok   Teams.Show "submitting NewInvite"
ok   Teams.Show "NewInvite refuses an empty email"
ok   Teams.Show "pressing Remove"
ok   Teams.Show "a non-member gets a 404"
ok   Teams.Show "a member cannot remove another member"
ok   Teams.Show "the owner cannot remove themselves"
21 passed, 0 failed
[exit 0]
```

The harness ran `alder check`, `alder build` and `alder test` on the app: all pass.
