### Round 1

```text
$ alder fill 'Auth.Login."logging in through the form"' --hole ?afterLogin - <<'EOF'
Teams.Index {}
EOF
edited app/Auth/Login.alder
--- a/app/Auth/Login.alder
+++ b/app/Auth/Login.alder
@@ -83,7 +83,7 @@
   let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
   let _ <- Test.get (Auth.Login {})
   let r <- Test.submit Credentials { email: Text.Email "me@example.com", password: Text.Secret "secret" }
-  Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
+  Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (Teams.Index {}), user: Just me })
 }
 
 test "a signed-in user logs out here" = {
[exit 0]
$ alder fill 'Invites.Index."pressing Accept"' --hole ?expected - <<'EOF'
{ answer: r.answer, status: Maybe.map (\i -> i.status) after } == { answer: Redirected (Teams.Show { id: team1 }), status: Just Accepted }
EOF
edited app/Invites/Index.alder
--- a/app/Invites/Index.alder
+++ b/app/Invites/Index.alder
@@ -78,9 +78,6 @@
   Task.succeed (r.answer == Redirected (Auth.Login {}))
 }
 
--- Fill ?expected with what pressing Accept must do, so that a test fails when the arm
--- does something else: its answer (`r.answer`: Rerendered, Redirected …) and what it
--- changed (`after`, the row read again), as `{ answer: r.answer, … } == { answer: Rerendered, … }`.
 test "pressing Accept" = {
   let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
   let _ <- Test.login { user: Just me }
@@ -89,7 +86,7 @@
   let _ <- Test.get (Invites.Index {})
   let r <- Test.click (Accept { id: invite1 })
   let after <- Db.get Invite invite1
-  Task.succeed ?expected
+  Task.succeed ({ answer: r.answer, status: Maybe.map (\i -> i.status) after } == { answer: Redirected (Teams.Show { id: team1 }), status: Just Accepted })
 }
 
 -- Fill ?expected with what pressing Decline must do, so that a test fails when the arm
app/Invites/Index.alder:103:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill 'Invites.Index."pressing Decline"' --hole ?expected - <<'EOF'
{ answer: r.answer, status: Maybe.map (\i -> i.status) after } == { answer: Redirected (Invites.Index {}), status: Just Declined }
EOF
edited app/Invites/Index.alder
--- a/app/Invites/Index.alder
+++ b/app/Invites/Index.alder
@@ -89,9 +89,6 @@
   Task.succeed ({ answer: r.answer, status: Maybe.map (\i -> i.status) after } == { answer: Redirected (Teams.Show { id: team1 }), status: Just Accepted })
 }
 
--- Fill ?expected with what pressing Decline must do, so that a test fails when the arm
--- does something else: its answer (`r.answer`: Rerendered, Redirected …) and what it
--- changed (`after`, the row read again), as `{ answer: r.answer, … } == { answer: Rerendered, … }`.
 test "pressing Decline" = {
   let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
   let _ <- Test.login { user: Just me }
@@ -100,7 +97,7 @@
   let _ <- Test.get (Invites.Index {})
   let r <- Test.click (Decline { id: invite1 })
   let after <- Db.get Invite invite1
-  Task.succeed ?expected
+  Task.succeed ({ answer: r.answer, status: Maybe.map (\i -> i.status) after } == { answer: Redirected (Invites.Index {}), status: Just Declined })
 }
 
 test "accepting an invite makes the team visible, declining does not" = {
[exit 0]
$ alder fill 'Teams.Show."submitting NewInvite"' --hole ?expected - <<'EOF'
Redirected (Teams.Show { id: team1 })
EOF
edited app/Teams/Show.alder
--- a/app/Teams/Show.alder
+++ b/app/Teams/Show.alder
@@ -102,7 +102,7 @@
   let team1 <- Db.insert Team { name: Text.NonEmpty "name-1", owner: me }
   let _ <- Test.get (Teams.Show { id: team1 })
   let r <- Test.submit NewInvite { email: Text.Email "email-1@example.com" }
-  Task.succeed (r.answer == ?expected)
+  Task.succeed (r.answer == Redirected (Teams.Show { id: team1 }))
 }
 
 test "NewInvite refuses an empty email" = {
app/Teams/Show.alder:96:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill --decl 'Teams.Index."submitting NewTeam"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Teams.Index {})
  let r <- Test.submit NewTeam { name: Text.NonEmpty "Red" }
  let made <- Db.from Team |> Db.first
  Task.succeed ({ answer: Just r.answer, owner: Maybe.map (\t -> t.owner) made } == { answer: Maybe.map (\t -> Redirected (Teams.Show { id: t.id })) made, owner: Just me })
}
EOF
edited app/Teams/Index.alder
--- a/app/Teams/Index.alder
+++ b/app/Teams/Index.alder
@@ -58,8 +58,9 @@
   let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
   let _ <- Test.login { user: Just me }
   let _ <- Test.get (Teams.Index {})
-  let r <- Test.submit NewTeam { name: Text.NonEmpty "name-1" }
-  Task.succeed (r.answer == ?expected)
+  let r <- Test.submit NewTeam { name: Text.NonEmpty "Red" }
+  let made <- Db.from Team |> Db.first
+  Task.succeed ({ answer: Just r.answer, owner: Maybe.map (\t -> t.owner) made } == { answer: Maybe.map (\t -> Redirected (Teams.Show { id: t.id })) made, owner: Just me })
 }
 
 test "NewTeam refuses an empty name" = {
rewriting a test: `alder test --mutate` is the check that it still tests something
[exit 0]
$ alder fill --decl 'Teams.Show."pressing Remove"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let team1 <- Db.insert Team { name: Text.NonEmpty "name-1", owner: me }
  let _ <- Db.insert Member { team: team1, user: bob }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Teams.Show { id: team1 })
  let r <- Test.click (Remove { user: bob })
  let n <- Db.from Member |> Db.where (Member.team.eq team1) |> Db.count
  Task.succeed ({ answer: r.answer, members: n } == { answer: Redirected (Teams.Show { id: team1 }), members: 0 })
}
EOF
edited app/Teams/Show.alder
--- a/app/Teams/Show.alder
+++ b/app/Teams/Show.alder
@@ -83,17 +83,16 @@
   Task.succeed (r.answer == Redirected (Auth.Login {}))
 }
 
--- Fill ?expected with what pressing Remove must do, so that a test fails when the arm
--- does something else: its answer (`r.answer`: Rerendered, Redirected …) and what it
--- changed (`after`, the row read again), as `{ answer: r.answer, … } == { answer: Rerendered, … }`.
 test "pressing Remove" = {
   let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
-  let _ <- Test.login { user: Just me }
+  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
   let team1 <- Db.insert Team { name: Text.NonEmpty "name-1", owner: me }
+  let _ <- Db.insert Member { team: team1, user: bob }
+  let _ <- Test.login { user: Just me }
   let _ <- Test.get (Teams.Show { id: team1 })
-  let r <- Test.click (Remove { user: me })
-  let after <- Db.get User me
-  Task.succeed ?expected
+  let r <- Test.click (Remove { user: bob })
+  let n <- Db.from Member |> Db.where (Member.team.eq team1) |> Db.count
+  Task.succeed ({ answer: r.answer, members: n } == { answer: Redirected (Teams.Show { id: team1 }), members: 0 })
 }
 
 test "submitting NewInvite" = {
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
FAIL Teams.Index "NewTeam refuses an empty name"
  left:  FormRejected [{ field: "", message: "name must not be empty" }]
  right: FormRejected [{ field: "name", message: "must not be empty" }]
  trace: ["Db.insert User", "Test.login", "Test.get Teams.Index", "Db.all Member", "Db.all Team", "Test.submitRaw NewTeam", "Db.all Member", "Db.all Team"]
ok   Teams.Show "a visitor is sent to log in"
ok   Teams.Show "pressing Remove"
ok   Teams.Show "submitting NewInvite"
FAIL Teams.Show "NewInvite refuses an empty email"
  left:  FormRejected [{ field: "", message: "email must be an email address" }]
  right: FormRejected [{ field: "email", message: "must be an email address" }]
  trace: ["Db.insert User", "Test.login", "Db.insert Team", "Test.get Teams.Show", "Db.get Team", "Db.all Member", "Db.all User", "Db.get User", "Test.submitRaw NewInvite", "Db.get Team", "Db.all Member", "Db.all User", "Db.get User"]
ok   Teams.Show "a team's page is not found for a non-member"
ok   Teams.Show "a member cannot remove another member"
ok   Teams.Show "the owner removes a member but not themselves"
20 passed, 2 failed
[exit 1]
```

The harness stopped at line 37 of your script: `alder test`: it exited 1. The commands after it were not run.

Your script stopped at command 8; the app is as the commands before it left it; continue.

The program builds but its tests fail; fix the program or the tests so every test passes.

```text
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
FAIL Teams.Index "NewTeam refuses an empty name"
  left:  FormRejected [{ field: "", message: "name must not be empty" }]
  right: FormRejected [{ field: "name", message: "must not be empty" }]
  trace: ["Db.insert User", "Test.login", "Test.get Teams.Index", "Db.all Member", "Db.all Team", "Test.submitRaw NewTeam", "Db.all Member", "Db.all Team"]
ok   Teams.Show "a visitor is sent to log in"
ok   Teams.Show "pressing Remove"
ok   Teams.Show "submitting NewInvite"
FAIL Teams.Show "NewInvite refuses an empty email"
  left:  FormRejected [{ field: "", message: "email must be an email address" }]
  right: FormRejected [{ field: "email", message: "must be an email address" }]
  trace: ["Db.insert User", "Test.login", "Db.insert Team", "Test.get Teams.Show", "Db.get Team", "Db.all Member", "Db.all User", "Db.get User", "Test.submitRaw NewInvite", "Db.get Team", "Db.all Member", "Db.all User", "Db.get User"]
ok   Teams.Show "a team's page is not found for a non-member"
ok   Teams.Show "a member cannot remove another member"
ok   Teams.Show "the owner removes a member but not themselves"
20 passed, 2 failed
```
