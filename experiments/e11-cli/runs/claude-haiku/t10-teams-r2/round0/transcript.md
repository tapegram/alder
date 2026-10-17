### Round 0

```text
$ alder new table User email:Text.Email@unique name:Text.NonEmpty passwordHash:Text created:Time@created --no-timestamps
wrote app/User.alder
wrote migrations/0001_initial.sql
  create table User
wrote migrations/schema.json
[exit 0]
$ alder new auth --users User
wrote adapters/auth/login_password.go
wrote adapters/auth/login_password_test.go
edited app/App.alder
wrote app/Auth/Login.alder
--- a/app/App.alder
+++ b/app/App.alder
@@ -1,9 +1,22 @@
 module App
 
 import Layout
+import User
 
-session { }
+session { user: Maybe User.UserId }
 
+login = Auth.Login
+
+msg Logout
+
+update : Msg -> Session -> Task Nav
+update msg session = match msg {
+  Logout p -> {
+    let _ <- Session.put (\s -> { s | user: Nothing })
+    Task.succeed (Redirect (Auth.Login {}))
+  },
+}
+
 error : Error -> Html Never
 error err =
   <Layout.Page title="Error">
@@ -16,3 +29,10 @@
   <Layout.Page title="Not found">
     <h1>Not found</h1>
   </Layout.Page>
+
+-- Every Log out button is on a page that requires a user, so a browser test cannot tell this
+-- Redirect from a Rerender (book 10, "The logout redirect"): the test calls update itself.
+test "Logout clears the user and goes to log in" = {
+  let r <- Test.run (update Logout { user: Just (User.UserId "me") })
+  Task.succeed ({ value: r.value, trace: r.trace } == { value: Ok (Redirect (Auth.Login {})), trace: ["Session.put"] })
+}
app/Auth/Login.alder:47:33: warning[W0002]: hole ?afterLogin has type Page
          Task.succeed (Redirect (?afterLogin))
                                  ^^^^^^^^^^^
app/Auth/Login.alder:86:85: warning[W0002]: hole ?afterLogin has type Page
    Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
                                                                                      ^^^^^^^^^^^
[exit 0]
$ alder new table Team name:Text.NonEmpty owner:User.UserId@ref=User --no-timestamps
wrote app/Team.alder
wrote migrations/0002_create_team.sql
  create table Team
  add index on Team.owner
edited migrations/schema.json
[exit 0]
$ alder new table Member team:Team.TeamId@ref=Team user:User.UserId@ref=User --no-timestamps
wrote app/Member.alder
wrote migrations/0003_create_member.sql
  create table Member
  add index on Member.team
  add index on Member.user
edited migrations/schema.json
[exit 0]
$ alder new type Invite "Status = Pending | Accepted | Declined"
wrote app/Invite.alder
[exit 0]
$ alder new table Invite team:Team.TeamId@ref=Team email:Text.Email status:Status=Pending --no-timestamps
edited app/Invite.alder
wrote migrations/0004_create_invite.sql
  create table Invite
  add index on Invite.team
edited migrations/schema.json
--- a/app/Invite.alder
+++ b/app/Invite.alder
@@ -1,3 +1,24 @@
 module Invite
 
+import Team
+import User
+
 type Status = Pending | Accepted | Declined
+
+type InviteId = brand Text
+
+table Invite {
+  id: InviteId @id,
+  team: Team.TeamId @ref Team,
+  email: Text.Email,
+  status: Status = Pending,
+}
+
+test "a Invite keeps its team from being deleted" = {
+  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
+  let target <- Db.insert Team { name: Text.NonEmpty "name-1", owner: me }
+  let _ <- Db.insert Invite { team: target, email: Text.Email "email-1@example.com" }
+  let r <- Test.run (Db.delete Team target)
+  let left <- Db.from Invite |> Db.count
+  Task.succeed ({ deleted: Result.toMaybe r.value, left: left } == { deleted: Nothing, left: 1 })
+}
[exit 0]
$ alder new page Teams.Index /teams --blank --requires user
wrote app/Teams/Index.alder
app/Teams/Index.alder:8:23: warning[W0002]: hole ?load has type Query Teams.Index.Model
  load params session = ?load
                        ^^^^^
app/Teams/Index.alder:11:22: warning[W0002]: hole ?view has type Html Teams.Index.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder new page Teams.Show '/teams/{id}' --param id:Team.TeamId --blank --requires user
wrote app/Teams/Show.alder
app/Teams/Show.alder:11:23: warning[W0002]: hole ?load has type Query Teams.Show.Model
  load params session = ?load
                        ^^^^^
app/Teams/Show.alder:14:22: warning[W0002]: hole ?view has type Html Teams.Show.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder new page Invites.Index /invites --blank --requires user
wrote app/Invites/Index.alder
app/Invites/Index.alder:8:23: warning[W0002]: hole ?load has type Query Invites.Index.Model
  load params session = ?load
                        ^^^^^
app/Invites/Index.alder:11:22: warning[W0002]: hole ?view has type Html Invites.Index.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder fill --decl Teams.Index.view - <<'EOF'
\model session -> <p>placeholder</p>
EOF
alder fill: the parameters `model session` are bound already: send the body without `\model session ->`
[exit 1]
```

The harness stopped at line 15 of your script: `alder fill --decl Teams.Index.view - <<'EOF'`: it exited 1. The commands after it were not run.

Your script stopped at command 10; the app is as the commands before it left it; continue.

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Auth/Login.alder:47:33: warning[W0002]: hole ?afterLogin has type Page
          Task.succeed (Redirect (?afterLogin))
                                  ^^^^^^^^^^^
app/Auth/Login.alder:86:85: warning[W0002]: hole ?afterLogin has type Page
    Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
                                                                                      ^^^^^^^^^^^
app/Invites/Index.alder:8:23: warning[W0002]: hole ?load has type Query Invites.Index.Model
  load params session = ?load
                        ^^^^^
app/Invites/Index.alder:11:22: warning[W0002]: hole ?view has type Html Invites.Index.Msg
  view model session = ?view
                       ^^^^^
app/Teams/Index.alder:8:23: warning[W0002]: hole ?load has type Query Teams.Index.Model
  load params session = ?load
                        ^^^^^
app/Teams/Index.alder:11:22: warning[W0002]: hole ?view has type Html Teams.Index.Msg
  view model session = ?view
                       ^^^^^
app/Teams/Show.alder:11:23: warning[W0002]: hole ?load has type Query Teams.Show.Model
  load params session = ?load
                        ^^^^^
app/Teams/Show.alder:14:22: warning[W0002]: hole ?view has type Html Teams.Show.Msg
  view model session = ?view
                       ^^^^^
alder build: the program has 8 typed hole(s); fill them before building
```
