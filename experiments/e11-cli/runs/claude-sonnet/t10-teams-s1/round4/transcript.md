### Round 4

```text
$ alder fill 'Auth.Login."logging in through the form"' - --hole ?afterLogin <<'EOF'
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
$ alder fill 'Invites.Index."pressing Accept"' - --hole ?expected <<'EOF'
{ answer: r.answer, status: Maybe.map (\i -> i.status) after } == { answer: Redirected (Teams.Show { id: team1 }), status: Just Accepted }
EOF
edited app/Invites/Index.alder
--- a/app/Invites/Index.alder
+++ b/app/Invites/Index.alder
@@ -82,9 +82,6 @@
   Task.succeed (r.answer == Redirected (Auth.Login {}))
 }
 
--- Fill ?expected with what pressing Accept must do, so that a test fails when the arm
--- does something else: its answer (`r.answer`: Rerendered, Redirected …) and what it
--- changed (`after`, the row read again), as `{ answer: r.answer, … } == { answer: Rerendered, … }`.
 test "pressing Accept" = {
   let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
   let _ <- Test.login { user: Just me }
@@ -93,7 +90,7 @@
   let _ <- Test.get (Invites.Index {})
   let r <- Test.click (Accept { id: invite1 })
   let after <- Db.get Invite invite1
-  Task.succeed ?expected
+  Task.succeed ({ answer: r.answer, status: Maybe.map (\i -> i.status) after } == { answer: Redirected (Teams.Show { id: team1 }), status: Just Accepted })
 }
 
 -- Fill ?expected with what pressing Decline must do, so that a test fails when the arm
app/Invites/Index.alder:107:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill 'Invites.Index."pressing Decline"' - --hole ?expected <<'EOF'
{ answer: r.answer, status: Maybe.map (\i -> i.status) after } == { answer: Redirected (Invites.Index {}), status: Just Declined }
EOF
edited app/Invites/Index.alder
--- a/app/Invites/Index.alder
+++ b/app/Invites/Index.alder
@@ -93,9 +93,6 @@
   Task.succeed ({ answer: r.answer, status: Maybe.map (\i -> i.status) after } == { answer: Redirected (Teams.Show { id: team1 }), status: Just Accepted })
 }
 
--- Fill ?expected with what pressing Decline must do, so that a test fails when the arm
--- does something else: its answer (`r.answer`: Rerendered, Redirected …) and what it
--- changed (`after`, the row read again), as `{ answer: r.answer, … } == { answer: Rerendered, … }`.
 test "pressing Decline" = {
   let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
   let _ <- Test.login { user: Just me }
@@ -104,7 +101,7 @@
   let _ <- Test.get (Invites.Index {})
   let r <- Test.click (Decline { id: invite1 })
   let after <- Db.get Invite invite1
-  Task.succeed ?expected
+  Task.succeed ({ answer: r.answer, status: Maybe.map (\i -> i.status) after } == { answer: Redirected (Invites.Index {}), status: Just Declined })
 }
 
 test "accepting an invite makes the team visible" = {
[exit 0]
$ alder fill --decl 'Teams.Index."submitting NewTeam"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Teams.Index {})
  let r <- Test.submit NewTeam { name: Text.NonEmpty "Red" }
  let found <- Db.from Team |> Db.first
  let same = Just r.answer == Maybe.map (\t -> Redirected (Teams.Show { id: t.id })) found
  let owned = Maybe.map (\t -> t.owner) found == Just me
  Task.succeed (same && owned)
}
EOF
edited app/Teams/Index.alder
--- a/app/Teams/Index.alder
+++ b/app/Teams/Index.alder
@@ -54,8 +54,11 @@
   let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
   let _ <- Test.login { user: Just me }
   let _ <- Test.get (Teams.Index {})
-  let r <- Test.submit NewTeam { name: Text.NonEmpty "name-1" }
-  Task.succeed (r.answer == ?expected)
+  let r <- Test.submit NewTeam { name: Text.NonEmpty "Red" }
+  let found <- Db.from Team |> Db.first
+  let same = Just r.answer == Maybe.map (\t -> Redirected (Teams.Show { id: t.id })) found
+  let owned = Maybe.map (\t -> t.owner) found == Just me
+  Task.succeed (same && owned)
 }
 
 test "NewTeam refuses an empty name" = {
rewriting a test: `alder test --mutate` is the check that it still tests something
[exit 0]
$ alder fill 'Teams.Show."submitting NewInvite"' - --hole ?expected <<'EOF'
Redirected (Teams.Show { id: team1 })
EOF
edited app/Teams/Show.alder
--- a/app/Teams/Show.alder
+++ b/app/Teams/Show.alder
@@ -85,7 +85,7 @@
   let team1 <- Db.insert Team { name: Text.NonEmpty "name-1", owner: me }
   let _ <- Test.get (Teams.Show { id: team1 })
   let r <- Test.submit NewInvite { email: Text.Email "email-1@example.com" }
-  Task.succeed (r.answer == ?expected)
+  Task.succeed (r.answer == Redirected (Teams.Show { id: team1 }))
 }
 
 test "NewInvite refuses an empty email" = {
app/Teams/Show.alder:110:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
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
  let n <- Db.from Member |> Db.count
  Task.succeed ({ answer: r.answer, members: n } == { answer: Redirected (Teams.Show { id: team1 }), members: 0 })
}
EOF
edited app/Teams/Show.alder
--- a/app/Teams/Show.alder
+++ b/app/Teams/Show.alder
@@ -97,17 +97,16 @@
   Task.succeed (r.answer == FormRejected [{ field: "email", message: "must be an email address" }])
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
+  let n <- Db.from Member |> Db.count
+  Task.succeed ({ answer: r.answer, members: n } == { answer: Redirected (Teams.Show { id: team1 }), members: 0 })
 }
 
 test "a team page is not found for a non-member" = {
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
ok   Invites.Index "accepting an invite makes the team visible"
ok   Invites.Index "declining an invite keeps the team closed"
ok   Teams.Index "a visitor is sent to log in"
ok   Teams.Index "submitting NewTeam"
ok   Teams.Index "NewTeam refuses an empty name"
ok   Teams.Show "a visitor is sent to log in"
ok   Teams.Show "submitting NewInvite"
ok   Teams.Show "NewInvite refuses an empty email"
ok   Teams.Show "pressing Remove"
ok   Teams.Show "a team page is not found for a non-member"
ok   Teams.Show "a member cannot remove another member"
ok   Teams.Show "the owner removes a member but not themselves"
23 passed, 0 failed
[exit 0]
```

The harness ran `alder check`, `alder build` and `alder test` on the app: all pass.
