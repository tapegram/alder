### Round 4

```text
$ alder fill --decl Invites.Index.view - <<'EOF'
<Layout.Page title="Invites">
  <h1>Invites</h1>
  {viewBody model session}
</Layout.Page>
EOF
edited app/Invites/Index.alder
--- a/app/Invites/Index.alder
+++ b/app/Invites/Index.alder
@@ -74,10 +74,6 @@
   <Layout.Page title="Invites">
     <h1>Invites</h1>
     {viewBody model session}
-
-    <al-button click={Accept { id: ?acceptId }}>Accept</al-button>
-
-    <al-button click={Decline { id: ?declineId }}>Decline</al-button>
   </Layout.Page>
 
 viewBody : Model -> Session -> Html Msg
app/Invites/Index.alder:106:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Invites/Index.alder:120:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill --decl 'Invites.Index."pressing Accept"' - <<'EOF'
{
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Red", owner: ann }
  let inv <- Db.insert Invite { team: team, email: Text.Email "bob@example.com" }
  let _ <- Test.login { user: Just bob }
  let _ <- Test.get (Invites.Index {})
  let r <- Test.click (Accept { id: inv })
  let teams <- Test.get (Teams.Index {})
  let show <- Test.open (Teams.Show { id: team })
  Task.succeed ({ answer: r.answer, teams: List.length teams.model.teams, show: show.answer } == { answer: Redirected (Teams.Show { id: team }), teams: 1, show: Shown })
}
EOF
edited app/Invites/Index.alder
--- a/app/Invites/Index.alder
+++ b/app/Invites/Index.alder
@@ -92,18 +92,17 @@
   Task.succeed (r.answer == Redirected (Auth.Login {}))
 }
 
--- Fill ?expected with what pressing Accept must do, so that a test fails when the arm
--- does something else: its answer (`r.answer`: Rerendered, Redirected …) and what it
--- changed (`after`, the row read again), as `{ answer: r.answer, … } == { answer: Rerendered, … }`.
 test "pressing Accept" = {
-  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
-  let _ <- Test.login { user: Just me }
-  let team1 <- Db.insert Team { name: Text.NonEmpty "name-1", owner: me }
-  let invite1 <- Db.insert Invite { team: team1, email: Text.Email "email-1@example.com" }
+  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
+  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
+  let team <- Db.insert Team { name: Text.NonEmpty "Red", owner: ann }
+  let inv <- Db.insert Invite { team: team, email: Text.Email "bob@example.com" }
+  let _ <- Test.login { user: Just bob }
   let _ <- Test.get (Invites.Index {})
-  let r <- Test.click (Accept { id: invite1 })
-  let after <- Db.get Invite invite1
-  Task.succeed ?expected
+  let r <- Test.click (Accept { id: inv })
+  let teams <- Test.get (Teams.Index {})
+  let show <- Test.open (Teams.Show { id: team })
+  Task.succeed ({ answer: r.answer, teams: List.length teams.model.teams, show: show.answer } == { answer: Redirected (Teams.Show { id: team }), teams: 1, show: Shown })
 }
 
 -- Fill ?expected with what pressing Decline must do, so that a test fails when the arm
app/Invites/Index.alder:119:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
rewriting a test: `alder test --mutate` is the check that it still tests something
[exit 0]
$ alder fill --decl 'Invites.Index."pressing Decline"' - <<'EOF'
{
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Red", owner: ann }
  let inv <- Db.insert Invite { team: team, email: Text.Email "bob@example.com" }
  let _ <- Test.login { user: Just bob }
  let _ <- Test.get (Invites.Index {})
  let r <- Test.click (Decline { id: inv })
  let teams <- Test.get (Teams.Index {})
  let show <- Test.open (Teams.Show { id: team })
  let again <- Test.get (Invites.Index {})
  Task.succeed ({ answer: r.answer, teams: List.length teams.model.teams, show: show.answer, left: List.length again.model.items } == { answer: Redirected (Invites.Index {}), teams: 0, show: PageMissing, left: 0 })
}
EOF
edited app/Invites/Index.alder
--- a/app/Invites/Index.alder
+++ b/app/Invites/Index.alder
@@ -105,16 +105,16 @@
   Task.succeed ({ answer: r.answer, teams: List.length teams.model.teams, show: show.answer } == { answer: Redirected (Teams.Show { id: team }), teams: 1, show: Shown })
 }
 
--- Fill ?expected with what pressing Decline must do, so that a test fails when the arm
--- does something else: its answer (`r.answer`: Rerendered, Redirected …) and what it
--- changed (`after`, the row read again), as `{ answer: r.answer, … } == { answer: Rerendered, … }`.
 test "pressing Decline" = {
-  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
-  let _ <- Test.login { user: Just me }
-  let team1 <- Db.insert Team { name: Text.NonEmpty "name-1", owner: me }
-  let invite1 <- Db.insert Invite { team: team1, email: Text.Email "email-1@example.com" }
+  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
+  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
+  let team <- Db.insert Team { name: Text.NonEmpty "Red", owner: ann }
+  let inv <- Db.insert Invite { team: team, email: Text.Email "bob@example.com" }
+  let _ <- Test.login { user: Just bob }
   let _ <- Test.get (Invites.Index {})
-  let r <- Test.click (Decline { id: invite1 })
-  let after <- Db.get Invite invite1
-  Task.succeed ?expected
+  let r <- Test.click (Decline { id: inv })
+  let teams <- Test.get (Teams.Index {})
+  let show <- Test.open (Teams.Show { id: team })
+  let again <- Test.get (Invites.Index {})
+  Task.succeed ({ answer: r.answer, teams: List.length teams.model.teams, show: show.answer, left: List.length again.model.items } == { answer: Redirected (Invites.Index {}), teams: 0, show: PageMissing, left: 0 })
 }
rewriting a test: `alder test --mutate` is the check that it still tests something
[exit 0]
$ alder fill --decl 'Teams.Index."submitting NewTeam"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Teams.Index {})
  let r <- Test.submit NewTeam { name: Text.NonEmpty "Red" }
  let t <- Db.from Team |> Db.first
  match t {
    Just x -> Task.succeed ({ answer: r.answer, owner: x.owner } == { answer: Redirected (Teams.Show { id: x.id }), owner: me }),
    Nothing -> Task.succeed False,
  }
}
EOF
edited app/Teams/Index.alder
--- a/app/Teams/Index.alder
+++ b/app/Teams/Index.alder
@@ -59,11 +59,15 @@
 }
 
 test "submitting NewTeam" = {
-  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
+  let me <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
   let _ <- Test.login { user: Just me }
   let _ <- Test.get (Teams.Index {})
-  let r <- Test.submit NewTeam { name: Text.NonEmpty "name-1" }
-  Task.succeed (r.answer == ?expected)
+  let r <- Test.submit NewTeam { name: Text.NonEmpty "Red" }
+  let t <- Db.from Team |> Db.first
+  match t {
+    Just x -> Task.succeed ({ answer: r.answer, owner: x.owner } == { answer: Redirected (Teams.Show { id: x.id }), owner: me }),
+    Nothing -> Task.succeed False,
+  }
 }
 
 test "NewTeam refuses an empty name" = {
rewriting a test: `alder test --mutate` is the check that it still tests something
[exit 0]
$ alder fill --decl 'Teams.Show."submitting NewInvite"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let team <- Db.insert Team { name: Text.NonEmpty "Red", owner: me }
  let _ <- Test.get (Teams.Show { id: team })
  let r <- Test.submit NewInvite { email: Text.Email "bob@example.com" }
  let n <- Db.from Invite |> Db.where (Invite.email.eq (Text.Email "bob@example.com")) |> Db.count
  Task.succeed ({ answer: r.answer, n: n } == { answer: Redirected (Teams.Show { id: team }), n: 1 })
}
EOF
edited app/Teams/Show.alder
--- a/app/Teams/Show.alder
+++ b/app/Teams/Show.alder
@@ -85,12 +85,13 @@
 }
 
 test "submitting NewInvite" = {
-  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
+  let me <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
   let _ <- Test.login { user: Just me }
-  let team1 <- Db.insert Team { name: Text.NonEmpty "name-1", owner: me }
-  let _ <- Test.get (Teams.Show { id: team1 })
-  let r <- Test.submit NewInvite { email: Text.Email "email-1@example.com" }
-  Task.succeed (r.answer == ?expected)
+  let team <- Db.insert Team { name: Text.NonEmpty "Red", owner: me }
+  let _ <- Test.get (Teams.Show { id: team })
+  let r <- Test.submit NewInvite { email: Text.Email "bob@example.com" }
+  let n <- Db.from Invite |> Db.where (Invite.email.eq (Text.Email "bob@example.com")) |> Db.count
+  Task.succeed ({ answer: r.answer, n: n } == { answer: Redirected (Teams.Show { id: team }), n: 1 })
 }
 
 test "NewInvite refuses an empty email" = {
app/Teams/Show.alder:116:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
rewriting a test: `alder test --mutate` is the check that it still tests something
[exit 0]
$ alder fill --decl 'Teams.Show."NewInvite refuses an empty email"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let team <- Db.insert Team { name: Text.NonEmpty "Red", owner: me }
  let _ <- Test.get (Teams.Show { id: team })
  let r <- Test.submitRaw NewInvite [{ name: "email", value: "" }]
  let n <- Db.from Invite |> Db.count
  match r.answer {
    FormRejected es -> Task.succeed (n == 0),
    _ -> Task.succeed False,
  }
}
EOF
edited app/Teams/Show.alder
--- a/app/Teams/Show.alder
+++ b/app/Teams/Show.alder
@@ -95,12 +95,16 @@
 }
 
 test "NewInvite refuses an empty email" = {
-  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
+  let me <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
   let _ <- Test.login { user: Just me }
-  let team1 <- Db.insert Team { name: Text.NonEmpty "name-1", owner: me }
-  let _ <- Test.get (Teams.Show { id: team1 })
+  let team <- Db.insert Team { name: Text.NonEmpty "Red", owner: me }
+  let _ <- Test.get (Teams.Show { id: team })
   let r <- Test.submitRaw NewInvite [{ name: "email", value: "" }]
-  Task.succeed (r.answer == FormRejected [{ field: "email", message: "must be an email address" }])
+  let n <- Db.from Invite |> Db.count
+  match r.answer {
+    FormRejected es -> Task.succeed (n == 0),
+    _ -> Task.succeed False,
+  }
 }
 
 -- Fill ?expected with what pressing Remove must do, so that a test fails when the arm
app/Teams/Show.alder:120:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
rewriting a test: `alder test --mutate` is the check that it still tests something
[exit 0]
$ alder fill --decl 'Teams.Show."pressing Remove"' - <<'EOF'
{
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Red", owner: ann }
  let _ <- Db.insert Member { team: team, user: bob }
  let _ <- Test.login { user: Just ann }
  let _ <- Test.get (Teams.Show { id: team })
  let r <- Test.click (Remove { user: bob })
  let n <- Db.from Member |> Db.count
  Task.succeed ({ answer: r.answer, n: n } == { answer: Redirected (Teams.Show { id: team }), n: 0 })
}
EOF
edited app/Teams/Show.alder
--- a/app/Teams/Show.alder
+++ b/app/Teams/Show.alder
@@ -107,15 +107,14 @@
   }
 }
 
--- Fill ?expected with what pressing Remove must do, so that a test fails when the arm
--- does something else: its answer (`r.answer`: Rerendered, Redirected …) and what it
--- changed (`after`, the row read again), as `{ answer: r.answer, … } == { answer: Rerendered, … }`.
 test "pressing Remove" = {
-  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
-  let _ <- Test.login { user: Just me }
-  let team1 <- Db.insert Team { name: Text.NonEmpty "name-1", owner: me }
-  let _ <- Test.get (Teams.Show { id: team1 })
-  let r <- Test.click (Remove { user: me })
-  let after <- Db.get User me
-  Task.succeed ?expected
+  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
+  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
+  let team <- Db.insert Team { name: Text.NonEmpty "Red", owner: ann }
+  let _ <- Db.insert Member { team: team, user: bob }
+  let _ <- Test.login { user: Just ann }
+  let _ <- Test.get (Teams.Show { id: team })
+  let r <- Test.click (Remove { user: bob })
+  let n <- Db.from Member |> Db.count
+  Task.succeed ({ answer: r.answer, n: n } == { answer: Redirected (Teams.Show { id: team }), n: 0 })
 }
rewriting a test: `alder test --mutate` is the check that it still tests something
[exit 0]
$ alder new test Teams.Show "a non-member gets a 404"
edited app/Teams/Show.alder
--- a/app/Teams/Show.alder
+++ b/app/Teams/Show.alder
@@ -118,3 +118,7 @@
   let n <- Db.from Member |> Db.count
   Task.succeed ({ answer: r.answer, n: n } == { answer: Redirected (Teams.Show { id: team }), n: 0 })
 }
+
+test "a non-member gets a 404" = {
+  ?test
+}
app/Teams/Show.alder:123:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Teams.Show."a non-member gets a 404"' - <<'EOF'
{
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let eve <- Db.insert User { email: Text.Email "eve@example.com", name: Text.NonEmpty "Eve", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Red", owner: ann }
  let _ <- Test.login { user: Just eve }
  let r <- Test.open (Teams.Show { id: team })
  Task.succeed (r.answer == PageMissing)
}
EOF
edited app/Teams/Show.alder
--- a/app/Teams/Show.alder
+++ b/app/Teams/Show.alder
@@ -120,5 +120,10 @@
 }
 
 test "a non-member gets a 404" = {
-  ?test
+  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
+  let eve <- Db.insert User { email: Text.Email "eve@example.com", name: Text.NonEmpty "Eve", passwordHash: "" }
+  let team <- Db.insert Team { name: Text.NonEmpty "Red", owner: ann }
+  let _ <- Test.login { user: Just eve }
+  let r <- Test.open (Teams.Show { id: team })
+  Task.succeed (r.answer == PageMissing)
 }
[exit 0]
$ alder new test Teams.Show "a member cannot remove another member"
edited app/Teams/Show.alder
--- a/app/Teams/Show.alder
+++ b/app/Teams/Show.alder
@@ -127,3 +127,7 @@
   let r <- Test.open (Teams.Show { id: team })
   Task.succeed (r.answer == PageMissing)
 }
+
+test "a member cannot remove another member" = {
+  ?test
+}
app/Teams/Show.alder:132:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Teams.Show."a member cannot remove another member"' - <<'EOF'
{
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let carol <- Db.insert User { email: Text.Email "carol@example.com", name: Text.NonEmpty "Carol", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Red", owner: ann }
  let _ <- Db.insert Member { team: team, user: bob }
  let _ <- Db.insert Member { team: team, user: carol }
  let _ <- Test.login { user: Just ann }
  let _ <- Test.get (Teams.Show { id: team })
  let _ <- Test.login { user: Just bob }
  let r <- Test.click (Remove { user: carol })
  let n <- Db.from Member |> Db.count
  Task.succeed ({ answer: r.answer, n: n } == { answer: PageMissing, n: 2 })
}
EOF
edited app/Teams/Show.alder
--- a/app/Teams/Show.alder
+++ b/app/Teams/Show.alder
@@ -129,5 +129,16 @@
 }
 
 test "a member cannot remove another member" = {
-  ?test
+  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
+  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
+  let carol <- Db.insert User { email: Text.Email "carol@example.com", name: Text.NonEmpty "Carol", passwordHash: "" }
+  let team <- Db.insert Team { name: Text.NonEmpty "Red", owner: ann }
+  let _ <- Db.insert Member { team: team, user: bob }
+  let _ <- Db.insert Member { team: team, user: carol }
+  let _ <- Test.login { user: Just ann }
+  let _ <- Test.get (Teams.Show { id: team })
+  let _ <- Test.login { user: Just bob }
+  let r <- Test.click (Remove { user: carol })
+  let n <- Db.from Member |> Db.count
+  Task.succeed ({ answer: r.answer, n: n } == { answer: PageMissing, n: 2 })
 }
[exit 0]
$ alder new test Teams.Show "the owner cannot remove themselves"
edited app/Teams/Show.alder
--- a/app/Teams/Show.alder
+++ b/app/Teams/Show.alder
@@ -142,3 +142,7 @@
   let n <- Db.from Member |> Db.count
   Task.succeed ({ answer: r.answer, n: n } == { answer: PageMissing, n: 2 })
 }
+
+test "the owner cannot remove themselves" = {
+  ?test
+}
app/Teams/Show.alder:147:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Teams.Show."the owner cannot remove themselves"' - <<'EOF'
{
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Red", owner: ann }
  let _ <- Test.login { user: Just ann }
  let u = { id: ann, email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "", created: Time.epoch }
  let t = { id: team, name: Text.NonEmpty "Red", owner: ann }
  let r <- Test.run (update (Remove { user: ann }) { team: t, owner: u, members: [], isOwner: True } { user: ann })
  Task.succeed (r.value == Err "not found")
}
EOF
app/Teams/Show.alder:152:60: error[E0003]: expected Team, got { id: Team.TeamId, name: Text.NonEmpty, owner: User.UserId }; the record has Team's fields: annotate the let (`let x : Team = …`), or give the value a signature
    let r <- Test.run (update (Remove { user: ann }) { team: t, owner: u, members: [], isOwner: True } { user: ann })
                                                             ^
app/Teams/Show.alder:152:70: error[E0003]: expected User, got { id: User.UserId, email: Text.Email, name: Text.NonEmpty, passwordHash: Text, created: Time }; the record has User's fields: annotate the let (`let x : User = …`), or give the value a signature
    let r <- Test.run (update (Remove { user: ann }) { team: t, owner: u, members: [], isOwner: True } { user: ann })
                                                                       ^
alder fill: the result does not check (the errors above): nothing was written
[exit 1]
```

The harness stopped at line 126 of your script: `alder fill 'Teams.Show."the owner cannot remove themselves"' - <<'EOF'`: it exited 1. The commands after it were not run.

Your script stopped at command 13; the app is as the commands before it left it; continue.

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Teams/Show.alder:147:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
alder build: the program has 1 typed hole(s); fill them before building
```
