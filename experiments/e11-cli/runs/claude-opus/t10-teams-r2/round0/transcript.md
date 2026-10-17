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
$ alder fill --list
Auth.Login update.Submit ?afterLogin : Page
Auth.Login "logging in through the form" ?afterLogin : Page
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
$ alder new page Teams.Index /teams --requires user --blank
wrote app/Teams/Index.alder
app/Teams/Index.alder:8:23: warning[W0002]: hole ?load has type Query Teams.Index.Model
  load params session = ?load
                        ^^^^^
app/Teams/Index.alder:11:22: warning[W0002]: hole ?view has type Html Teams.Index.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder new page Teams.Show '/teams/{id}' --param id:Team.TeamId --requires user --blank
wrote app/Teams/Show.alder
app/Teams/Show.alder:11:23: warning[W0002]: hole ?load has type Query Teams.Show.Model
  load params session = ?load
                        ^^^^^
app/Teams/Show.alder:14:22: warning[W0002]: hole ?view has type Html Teams.Show.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder new page Invites.Index /invites --requires user --blank
wrote app/Invites/Index.alder
app/Invites/Index.alder:8:23: warning[W0002]: hole ?load has type Query Invites.Index.Model
  load params session = ?load
                        ^^^^^
app/Invites/Index.alder:11:22: warning[W0002]: hole ?view has type Html Invites.Index.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder fill Auth.Login.update.Submit --hole ?afterLogin - <<'EOF'
Teams.Index {}
EOF
edited app/Auth/Login.alder
--- a/app/Auth/Login.alder
+++ b/app/Auth/Login.alder
@@ -44,7 +44,7 @@
     match found {
       Just id -> {
         let _ <- Session.put (\s -> { s | user: Just id })
-        Task.succeed (Redirect (?afterLogin))
+        Task.succeed (Redirect (Teams.Index {}))
       },
       Nothing -> Task.succeed (Reject "Email or password is incorrect."),
     }
app/Auth/Login.alder:86:85: warning[W0002]: hole ?afterLogin has type Page
    Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
                                                                                      ^^^^^^^^^^^
[exit 0]
$ alder fill --decl Teams.Index.Model - <<'EOF'
{ teams: List Team }
EOF
edited app/Teams/Index.alder
--- a/app/Teams/Index.alder
+++ b/app/Teams/Index.alder
@@ -1,8 +1,10 @@
 module Teams.Index
 
+import Team
+
 page "/teams" { } requires user
 
-type Model = { }
+type Model = { teams: List Team }
 
 load : Params -> Session -> Query Model
 load params session = ?load
app/Teams/Index.alder:10:23: warning[W0002]: hole ?load has type Query Teams.Index.Model
  load params session = ?load
                        ^^^^^
app/Teams/Index.alder:13:22: warning[W0002]: hole ?view has type Html Teams.Index.Msg
  view model session = ?view
                       ^^^^^
added import Team to app/Teams/Index.alder
[exit 0]
$ alder fill Teams.Index.load - <<'EOF'
{
  let memberships <- Db.from Member |> Db.where (Member.user.eq session.user) |> Db.all
  let teams <- Db.from Team |> Db.where (Db.or [Team.owner.eq session.user, Team.id.in (List.map (\m -> m.team) memberships)]) |> Db.orderBy Team.name.asc |> Db.all
  Query.succeed { teams: teams }
}
EOF
edited app/Teams/Index.alder
--- a/app/Teams/Index.alder
+++ b/app/Teams/Index.alder
@@ -1,5 +1,6 @@
 module Teams.Index
 
+import Member
 import Team
 
 page "/teams" { } requires user
@@ -7,7 +8,11 @@
 type Model = { teams: List Team }
 
 load : Params -> Session -> Query Model
-load params session = ?load
+load params session = {
+  let memberships <- Db.from Member |> Db.where (Member.user.eq session.user) |> Db.all
+  let teams <- Db.from Team |> Db.where (Db.or [Team.owner.eq session.user, Team.id.in (List.map (\m -> m.team) memberships)]) |> Db.orderBy Team.name.asc |> Db.all
+  Query.succeed { teams: teams }
+}
 
 view : Model -> Session -> Html Msg
 view model session = ?view
app/Teams/Index.alder:18:22: warning[W0002]: hole ?view has type Html Teams.Index.Msg
  view model session = ?view
                       ^^^^^
added import Member to app/Teams/Index.alder
[exit 0]
$ alder fill Teams.Index.view - <<'EOF'
<Layout.Page title="Teams">
  <h1>Teams</h1>
</Layout.Page>
EOF
edited app/Teams/Index.alder
--- a/app/Teams/Index.alder
+++ b/app/Teams/Index.alder
@@ -1,5 +1,6 @@
 module Teams.Index
 
+import Layout
 import Member
 import Team
 
@@ -15,7 +16,9 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = ?view
+view model session = <Layout.Page title="Teams">
+  <h1>Teams</h1>
+</Layout.Page>
 
 test "a visitor is sent to log in" = {
   let r <- Test.open (Teams.Index {})
added import Layout to app/Teams/Index.alder
[exit 0]
$ alder new form Teams.Index NewTeam name:Text.NonEmpty --msg Create
edited app/Teams/Index.alder
--- a/app/Teams/Index.alder
+++ b/app/Teams/Index.alder
@@ -3,6 +3,7 @@
 import Layout
 import Member
 import Team
+import User
 
 page "/teams" { } requires user
 
@@ -15,12 +16,48 @@
   Query.succeed { teams: teams }
 }
 
+form NewTeam = {
+  name: Text.NonEmpty,
+}
+
+msg Create form NewTeam
+
+update : Msg -> Model -> Session -> Task Nav
+update msg model session = match msg {
+  Create p -> ?create,
+}
+
 view : Model -> Session -> Html Msg
 view model session = <Layout.Page title="Teams">
   <h1>Teams</h1>
+
+  <al-card>
+    <h2 slot="header">New team</h2>
+    <form submit={Create}>
+      <al-input field="name" label="Name" />
+      <al-alert variant="destructive"><Errors form /></al-alert>
+      <al-button type="submit">New team</al-button>
+    </form>
+  </al-card>
 </Layout.Page>
 
 test "a visitor is sent to log in" = {
   let r <- Test.open (Teams.Index {})
   Task.succeed (r.answer == Redirected (Auth.Login {}))
 }
+
+test "submitting NewTeam" = {
+  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
+  let _ <- Test.login { user: Just me }
+  let _ <- Test.get (Teams.Index {})
+  let r <- Test.submit NewTeam { name: Text.NonEmpty "name-1" }
+  Task.succeed (r.answer == ?expected)
+}
+
+test "NewTeam refuses an empty name" = {
+  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
+  let _ <- Test.login { user: Just me }
+  let _ <- Test.get (Teams.Index {})
+  let r <- Test.submitRaw NewTeam [{ name: "name", value: "" }]
+  Task.succeed (r.answer == FormRejected [{ field: "name", message: "must not be empty" }])
+}
app/Teams/Index.alder:27:15: warning[W0002]: hole ?create has type Task Nav
    Create p -> ?create,
                ^^^^^^^
app/Teams/Index.alder:54:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder fill --decl Teams.Index.update - <<'EOF'
match msg {
  Create p -> {
    let id <- Db.insert Team { name: p.form.name, owner: session.user }
    Task.succeed (Redirect (Teams.Show { id: id }))
  },
}
EOF
edited app/Teams/Index.alder
--- a/app/Teams/Index.alder
+++ b/app/Teams/Index.alder
@@ -24,7 +24,10 @@
 
 update : Msg -> Model -> Session -> Task Nav
 update msg model session = match msg {
-  Create p -> ?create,
+  Create p -> {
+    let id <- Db.insert Team { name: p.form.name, owner: session.user }
+    Task.succeed (Redirect (Teams.Show { id: id }))
+  },
 }
 
 view : Model -> Session -> Html Msg
app/Teams/Index.alder:57:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder fill --decl Teams.Index.view - <<'EOF'
<Layout.Page title="Teams">
  <h1>Teams</h1>
  <ul id="teams">
    {for t in model.teams ->
      <li><a class="name" href={Teams.Show { id: t.id }}>{t.name}</a></li>}
  </ul>
  <form submit={Create}>
    <Errors form />
    <Input field="name" label="New team" />
    <button>Create</button>
  </form>
  <nav>
    <a href={Invites.Index {}}>Invites</a>
    <button click={App.Logout}>Log out</button>
  </nav>
</Layout.Page>
EOF
edited app/Teams/Index.alder
--- a/app/Teams/Index.alder
+++ b/app/Teams/Index.alder
@@ -31,18 +31,23 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = <Layout.Page title="Teams">
-  <h1>Teams</h1>
-
-  <al-card>
-    <h2 slot="header">New team</h2>
+view model session =
+  <Layout.Page title="Teams">
+    <h1>Teams</h1>
+    <ul id="teams">
+      {for t in model.teams ->
+        <li><a class="name" href={Teams.Show { id: t.id }}>{t.name}</a></li>}
+    </ul>
     <form submit={Create}>
-      <al-input field="name" label="Name" />
-      <al-alert variant="destructive"><Errors form /></al-alert>
-      <al-button type="submit">New team</al-button>
+      <Errors form />
+      <Input field="name" label="New team" />
+      <button>Create</button>
     </form>
-  </al-card>
-</Layout.Page>
+    <nav>
+      <a href={Invites.Index {}}>Invites</a>
+      <button click={App.Logout}>Log out</button>
+    </nav>
+  </Layout.Page>
 
 test "a visitor is sent to log in" = {
   let r <- Test.open (Teams.Index {})
app/Teams/Index.alder:62:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder fill --decl Teams.Show.Model - <<'EOF'
{ team: Team, owner: User, members: List User, isOwner: Bool }
EOF
edited app/Teams/Show.alder
--- a/app/Teams/Show.alder
+++ b/app/Teams/Show.alder
@@ -5,7 +5,7 @@
 
 page "/teams/{id}" { id: Team.TeamId } requires user
 
-type Model = { }
+type Model = { team: Team, owner: User, members: List User, isOwner: Bool }
 
 load : Params -> Session -> Query Model
 load params session = ?load
app/Teams/Show.alder:11:23: warning[W0002]: hole ?load has type Query Teams.Show.Model
  load params session = ?load
                        ^^^^^
app/Teams/Show.alder:14:22: warning[W0002]: hole ?view has type Html Teams.Show.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder fill Teams.Show.load - <<'EOF'
{
  let found <- Db.get Team params.id
  let rows <- Db.from Member |> Db.where (Member.team.eq params.id) |> Db.all
  let ids = List.map (\m -> m.user) rows
  let others <- Db.from User |> Db.where (User.id.in ids) |> Db.orderBy User.name.asc |> Db.all
  match found {
    Nothing -> Query.notFound,
    Just team -> {
      let owner <- Db.get User team.owner
      match owner {
        Nothing -> Query.notFound,
        Just o -> if team.owner == session.user || List.member session.user ids then Query.succeed { team: team, owner: o, members: others, isOwner: team.owner == session.user } else Query.notFound,
      }
    },
  }
}
EOF
edited app/Teams/Show.alder
--- a/app/Teams/Show.alder
+++ b/app/Teams/Show.alder
@@ -1,5 +1,6 @@
 module Teams.Show
 
+import Member
 import Team
 import User
 
@@ -8,7 +9,22 @@
 type Model = { team: Team, owner: User, members: List User, isOwner: Bool }
 
 load : Params -> Session -> Query Model
-load params session = ?load
+load params session = {
+  let found <- Db.get Team params.id
+  let rows <- Db.from Member |> Db.where (Member.team.eq params.id) |> Db.all
+  let ids = List.map (\m -> m.user) rows
+  let others <- Db.from User |> Db.where (User.id.in ids) |> Db.orderBy User.name.asc |> Db.all
+  match found {
+    Nothing -> Query.notFound,
+    Just team -> {
+      let owner <- Db.get User team.owner
+      match owner {
+        Nothing -> Query.notFound,
+        Just o -> if team.owner == session.user || List.member session.user ids then Query.succeed { team: team, owner: o, members: others, isOwner: team.owner == session.user } else Query.notFound,
+      }
+    },
+  }
+}
 
 view : Model -> Session -> Html Msg
 view model session = ?view
app/Teams/Show.alder:30:22: warning[W0002]: hole ?view has type Html Teams.Show.Msg
  view model session = ?view
                       ^^^^^
added import Member to app/Teams/Show.alder
[exit 0]
$ alder fill Teams.Show.view - <<'EOF'
<Layout.Page title="Team">
  <h1>Team</h1>
</Layout.Page>
EOF
edited app/Teams/Show.alder
--- a/app/Teams/Show.alder
+++ b/app/Teams/Show.alder
@@ -1,5 +1,6 @@
 module Teams.Show
 
+import Layout
 import Member
 import Team
 import User
@@ -27,7 +28,9 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = ?view
+view model session = <Layout.Page title="Team">
+  <h1>Team</h1>
+</Layout.Page>
 
 test "a visitor is sent to log in" = {
   let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
added import Layout to app/Teams/Show.alder
[exit 0]
$ alder new msg Teams.Show Remove user:User.UserId
edited app/Teams/Show.alder
--- a/app/Teams/Show.alder
+++ b/app/Teams/Show.alder
@@ -27,9 +27,18 @@
   }
 }
 
+msg Remove { user: User.UserId }
+
+update : Msg -> Model -> Session -> Task Nav
+update msg model session = match msg {
+  Remove p -> ?remove,
+}
+
 view : Model -> Session -> Html Msg
 view model session = <Layout.Page title="Team">
   <h1>Team</h1>
+
+  <al-button click={Remove { user: ?removeUser }}>Remove</al-button>
 </Layout.Page>
 
 test "a visitor is sent to log in" = {
@@ -38,3 +47,16 @@
   let r <- Test.open (Teams.Show { id: team1 })
   Task.succeed (r.answer == Redirected (Auth.Login {}))
 }
+
+-- Fill ?expected with what pressing Remove must do, so that a test fails when the arm
+-- does something else: its answer (`r.answer`: Rerendered, Redirected …) and what it
+-- changed (`after`, the row read again), as `{ answer: r.answer, … } == { answer: Rerendered, … }`.
+test "pressing Remove" = {
+  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
+  let _ <- Test.login { user: Just me }
+  let team1 <- Db.insert Team { name: Text.NonEmpty "name-1", owner: me }
+  let _ <- Test.get (Teams.Show { id: team1 })
+  let r <- Test.click (Remove { user: me })
+  let after <- Db.get User me
+  Task.succeed ?expected
+}
app/Teams/Show.alder:34:15: warning[W0002]: hole ?remove has type Task Nav
    Remove p -> ?remove,
                ^^^^^^^
app/Teams/Show.alder:41:36: warning[W0002]: hole ?removeUser has type User.UserId
    <al-button click={Remove { user: ?removeUser }}>Remove</al-button>
                                     ^^^^^^^^^^^
app/Teams/Show.alder:61:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder new form Teams.Show NewInvite email:Text.Email --msg SendInvite
edited app/Teams/Show.alder
--- a/app/Teams/Show.alder
+++ b/app/Teams/Show.alder
@@ -29,9 +29,16 @@
 
 msg Remove { user: User.UserId }
 
+form NewInvite = {
+  email: Text.Email,
+}
+
+msg SendInvite form NewInvite
+
 update : Msg -> Model -> Session -> Task Nav
 update msg model session = match msg {
   Remove p -> ?remove,
+  SendInvite p -> ?sendInvite,
 }
 
 view : Model -> Session -> Html Msg
@@ -39,6 +46,15 @@
   <h1>Team</h1>
 
   <al-button click={Remove { user: ?removeUser }}>Remove</al-button>
+
+  <al-card>
+    <h2 slot="header">New invite</h2>
+    <form submit={SendInvite}>
+      <al-input field="email" label="Email" />
+      <al-alert variant="destructive"><Errors form /></al-alert>
+      <al-button type="submit">New invite</al-button>
+    </form>
+  </al-card>
 </Layout.Page>
 
 test "a visitor is sent to log in" = {
@@ -60,3 +76,21 @@
   let after <- Db.get User me
   Task.succeed ?expected
 }
+
+test "submitting NewInvite" = {
+  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
+  let _ <- Test.login { user: Just me }
+  let team1 <- Db.insert Team { name: Text.NonEmpty "name-1", owner: me }
+  let _ <- Test.get (Teams.Show { id: team1 })
+  let r <- Test.submit NewInvite { email: Text.Email "email-1@example.com" }
+  Task.succeed (r.answer == ?expected)
+}
+
+test "NewInvite refuses an empty email" = {
+  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
+  let _ <- Test.login { user: Just me }
+  let team1 <- Db.insert Team { name: Text.NonEmpty "name-1", owner: me }
+  let _ <- Test.get (Teams.Show { id: team1 })
+  let r <- Test.submitRaw NewInvite [{ name: "email", value: "" }]
+  Task.succeed (r.answer == FormRejected [{ field: "email", message: "must be an email address" }])
+}
app/Teams/Show.alder:40:15: warning[W0002]: hole ?remove has type Task Nav
    Remove p -> ?remove,
                ^^^^^^^
app/Teams/Show.alder:41:19: warning[W0002]: hole ?sendInvite has type Task Nav
    SendInvite p -> ?sendInvite,
                    ^^^^^^^^^^^
app/Teams/Show.alder:48:36: warning[W0002]: hole ?removeUser has type User.UserId
    <al-button click={Remove { user: ?removeUser }}>Remove</al-button>
                                     ^^^^^^^^^^^
app/Teams/Show.alder:77:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Teams/Show.alder:86:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder fill --decl Teams.Show.update - <<'EOF'
match msg {
  Remove p -> if not model.isOwner || p.user == session.user then Task.notFound else {
    let row <- Db.from Member |> Db.where (Member.team.eq model.team.id) |> Db.where (Member.user.eq p.user) |> Db.first
    match row {
      Nothing -> Task.notFound,
      Just m -> {
        let _ <- Db.delete Member m.id
        Task.succeed (Redirect (Teams.Show { id: model.team.id }))
      },
    }
  },
  SendInvite p -> if not model.isOwner then Task.notFound else {
    let _ <- Db.insert Invite { team: model.team.id, email: p.form.email }
    Task.succeed (Redirect (Teams.Show { id: model.team.id }))
  },
}
EOF
edited app/Teams/Show.alder
--- a/app/Teams/Show.alder
+++ b/app/Teams/Show.alder
@@ -1,5 +1,6 @@
 module Teams.Show
 
+import Invite
 import Layout
 import Member
 import Team
@@ -37,8 +38,20 @@
 
 update : Msg -> Model -> Session -> Task Nav
 update msg model session = match msg {
-  Remove p -> ?remove,
-  SendInvite p -> ?sendInvite,
+  Remove p -> if not model.isOwner || p.user == session.user then Task.notFound else {
+    let row <- Db.from Member |> Db.where (Member.team.eq model.team.id) |> Db.where (Member.user.eq p.user) |> Db.first
+    match row {
+      Nothing -> Task.notFound,
+      Just m -> {
+        let _ <- Db.delete Member m.id
+        Task.succeed (Redirect (Teams.Show { id: model.team.id }))
+      },
+    }
+  },
+  SendInvite p -> if not model.isOwner then Task.notFound else {
+    let _ <- Db.insert Invite { team: model.team.id, email: p.form.email }
+    Task.succeed (Redirect (Teams.Show { id: model.team.id }))
+  },
 }
 
 view : Model -> Session -> Html Msg
app/Teams/Show.alder:61:36: warning[W0002]: hole ?removeUser has type User.UserId
    <al-button click={Remove { user: ?removeUser }}>Remove</al-button>
                                     ^^^^^^^^^^^
app/Teams/Show.alder:90:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Teams/Show.alder:99:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
added import Invite to app/Teams/Show.alder
[exit 0]
$ alder fill --decl Teams.Show.view - <<'EOF'
<Layout.Page title={model.team.name}>
  <h1 id="name">{model.team.name}</h1>
  <ul id="members">
    <li><span class="name">{model.owner.name}</span> (owner)</li>
    {for u in model.members ->
      <li>
        <span class="name">{u.name}</span>
        {if model.isOwner then <button class="remove" click={Remove { user: u.id }}>Remove</button> else Html.none}
      </li>}
  </ul>
  {if model.isOwner then
    <form submit={SendInvite}>
      <Errors form />
      <Input field="email" label="Invite by email" />
      <button>Invite</button>
    </form>
  else Html.none}
  <nav><a href={Teams.Index {}}>Teams</a></nav>
</Layout.Page>
EOF
edited app/Teams/Show.alder
--- a/app/Teams/Show.alder
+++ b/app/Teams/Show.alder
@@ -55,20 +55,26 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = <Layout.Page title="Team">
-  <h1>Team</h1>
-
-  <al-button click={Remove { user: ?removeUser }}>Remove</al-button>
-
-  <al-card>
-    <h2 slot="header">New invite</h2>
-    <form submit={SendInvite}>
-      <al-input field="email" label="Email" />
-      <al-alert variant="destructive"><Errors form /></al-alert>
-      <al-button type="submit">New invite</al-button>
-    </form>
-  </al-card>
-</Layout.Page>
+view model session =
+  <Layout.Page title={model.team.name}>
+    <h1 id="name">{model.team.name}</h1>
+    <ul id="members">
+      <li><span class="name">{model.owner.name}</span> (owner)</li>
+      {for u in model.members ->
+        <li>
+          <span class="name">{u.name}</span>
+          {if model.isOwner then <button class="remove" click={Remove { user: u.id }}>Remove</button> else Html.none}
+        </li>}
+    </ul>
+    {if model.isOwner then
+      <form submit={SendInvite}>
+        <Errors form />
+        <Input field="email" label="Invite by email" />
+        <button>Invite</button>
+      </form>
+    else Html.none}
+    <nav><a href={Teams.Index {}}>Teams</a></nav>
+  </Layout.Page>
 
 test "a visitor is sent to log in" = {
   let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
app/Teams/Show.alder:96:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Teams/Show.alder:105:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder fill --decl Invites.Index.Model - <<'EOF'
{ invites: List Invite, teams: List Team }
EOF
edited app/Invites/Index.alder
--- a/app/Invites/Index.alder
+++ b/app/Invites/Index.alder
@@ -1,8 +1,11 @@
 module Invites.Index
 
+import Invite
+import Team
+
 page "/invites" { } requires user
 
-type Model = { }
+type Model = { invites: List Invite, teams: List Team }
 
 load : Params -> Session -> Query Model
 load params session = ?load
app/Invites/Index.alder:11:23: warning[W0002]: hole ?load has type Query Invites.Index.Model
  load params session = ?load
                        ^^^^^
app/Invites/Index.alder:14:22: warning[W0002]: hole ?view has type Html Invites.Index.Msg
  view model session = ?view
                       ^^^^^
added import Invite to app/Invites/Index.alder
added import Team to app/Invites/Index.alder
[exit 0]
$ alder fill Invites.Index.load - <<'EOF'
{
  let me <- Db.get User session.user
  match me {
    Nothing -> Query.succeed { invites: [], teams: [] },
    Just u -> {
      let invites <- Db.from Invite |> Db.where (Invite.email.eq u.email) |> Db.where (Invite.status.eq Pending) |> Db.all
      let teams <- Db.from Team |> Db.where (Team.id.in (List.map (\i -> i.team) invites)) |> Db.all
      Query.succeed { invites: invites, teams: teams }
    },
  }
}
EOF
edited app/Invites/Index.alder
--- a/app/Invites/Index.alder
+++ b/app/Invites/Index.alder
@@ -2,13 +2,24 @@
 
 import Invite
 import Team
+import User
 
 page "/invites" { } requires user
 
 type Model = { invites: List Invite, teams: List Team }
 
 load : Params -> Session -> Query Model
-load params session = ?load
+load params session = {
+  let me <- Db.get User session.user
+  match me {
+    Nothing -> Query.succeed { invites: [], teams: [] },
+    Just u -> {
+      let invites <- Db.from Invite |> Db.where (Invite.email.eq u.email) |> Db.where (Invite.status.eq Pending) |> Db.all
+      let teams <- Db.from Team |> Db.where (Team.id.in (List.map (\i -> i.team) invites)) |> Db.all
+      Query.succeed { invites: invites, teams: teams }
+    },
+  }
+}
 
 view : Model -> Session -> Html Msg
 view model session = ?view
app/Invites/Index.alder:25:22: warning[W0002]: hole ?view has type Html Invites.Index.Msg
  view model session = ?view
                       ^^^^^
added import User to app/Invites/Index.alder
[exit 0]
$ alder new decl Invites.Index teamName "List Team -> Team.TeamId -> Text"
edited app/Invites/Index.alder
--- a/app/Invites/Index.alder
+++ b/app/Invites/Index.alder
@@ -24,6 +24,9 @@
 view : Model -> Session -> Html Msg
 view model session = ?view
 
+teamName : List Team -> Team.TeamId -> Text
+teamName = ?teamName
+
 test "a visitor is sent to log in" = {
   let r <- Test.open (Invites.Index {})
   Task.succeed (r.answer == Redirected (Auth.Login {}))
app/Invites/Index.alder:25:22: warning[W0002]: hole ?view has type Html Invites.Index.Msg
  view model session = ?view
                       ^^^^^
app/Invites/Index.alder:28:12: warning[W0002]: hole ?teamName has type List Team -> Team.TeamId -> Text
  teamName = ?teamName
             ^^^^^^^^^
[exit 0]
$ alder fill Invites.Index.teamName - <<'EOF'
\teams id -> match List.find (\t -> t.id == id) teams {
  Just t -> "{t.name}",
  Nothing -> "",
}
EOF
edited app/Invites/Index.alder
--- a/app/Invites/Index.alder
+++ b/app/Invites/Index.alder
@@ -25,7 +25,10 @@
 view model session = ?view
 
 teamName : List Team -> Team.TeamId -> Text
-teamName = ?teamName
+teamName = \teams id -> match List.find (\t -> t.id == id) teams {
+  Just t -> "{t.name}",
+  Nothing -> "",
+}
 
 test "a visitor is sent to log in" = {
   let r <- Test.open (Invites.Index {})
app/Invites/Index.alder:25:22: warning[W0002]: hole ?view has type Html Invites.Index.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder fill Invites.Index.view - <<'EOF'
<Layout.Page title="Invites">
  <h1>Invites</h1>
</Layout.Page>
EOF
edited app/Invites/Index.alder
--- a/app/Invites/Index.alder
+++ b/app/Invites/Index.alder
@@ -1,6 +1,7 @@
 module Invites.Index
 
 import Invite
+import Layout
 import Team
 import User
 
@@ -22,7 +23,9 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = ?view
+view model session = <Layout.Page title="Invites">
+  <h1>Invites</h1>
+</Layout.Page>
 
 teamName : List Team -> Team.TeamId -> Text
 teamName = \teams id -> match List.find (\t -> t.id == id) teams {
added import Layout to app/Invites/Index.alder
[exit 0]
$ alder new msg Invites.Index Accept id:Invite.InviteId
edited app/Invites/Index.alder
--- a/app/Invites/Index.alder
+++ b/app/Invites/Index.alder
@@ -22,9 +22,18 @@
   }
 }
 
+msg Accept { id: Invite.InviteId }
+
+update : Msg -> Model -> Session -> Task Nav
+update msg model session = match msg {
+  Accept p -> ?accept,
+}
+
 view : Model -> Session -> Html Msg
 view model session = <Layout.Page title="Invites">
   <h1>Invites</h1>
+
+  <al-button click={Accept { id: ?acceptId }}>Accept</al-button>
 </Layout.Page>
 
 teamName : List Team -> Team.TeamId -> Text
@@ -37,3 +46,17 @@
   let r <- Test.open (Invites.Index {})
   Task.succeed (r.answer == Redirected (Auth.Login {}))
 }
+
+-- Fill ?expected with what pressing Accept must do, so that a test fails when the arm
+-- does something else: its answer (`r.answer`: Rerendered, Redirected …) and what it
+-- changed (`after`, the row read again), as `{ answer: r.answer, … } == { answer: Rerendered, … }`.
+test "pressing Accept" = {
+  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
+  let _ <- Test.login { user: Just me }
+  let team1 <- Db.insert Team { name: Text.NonEmpty "name-1", owner: me }
+  let invite1 <- Db.insert Invite { team: team1, email: Text.Email "email-1@example.com" }
+  let _ <- Test.get (Invites.Index {})
+  let r <- Test.click (Accept { id: invite1 })
+  let after <- Db.get Invite invite1
+  Task.succeed ?expected
+}
app/Invites/Index.alder:29:15: warning[W0002]: hole ?accept has type Task Nav
    Accept p -> ?accept,
                ^^^^^^^
app/Invites/Index.alder:36:34: warning[W0002]: hole ?acceptId has type Invite.InviteId
    <al-button click={Accept { id: ?acceptId }}>Accept</al-button>
                                   ^^^^^^^^^
app/Invites/Index.alder:61:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder new msg Invites.Index Decline id:Invite.InviteId
edited app/Invites/Index.alder
--- a/app/Invites/Index.alder
+++ b/app/Invites/Index.alder
@@ -24,9 +24,12 @@
 
 msg Accept { id: Invite.InviteId }
 
+msg Decline { id: Invite.InviteId }
+
 update : Msg -> Model -> Session -> Task Nav
 update msg model session = match msg {
   Accept p -> ?accept,
+  Decline p -> ?decline,
 }
 
 view : Model -> Session -> Html Msg
@@ -34,6 +37,8 @@
   <h1>Invites</h1>
 
   <al-button click={Accept { id: ?acceptId }}>Accept</al-button>
+
+  <al-button click={Decline { id: ?declineId }}>Decline</al-button>
 </Layout.Page>
 
 teamName : List Team -> Team.TeamId -> Text
@@ -60,3 +65,17 @@
   let after <- Db.get Invite invite1
   Task.succeed ?expected
 }
+
+-- Fill ?expected with what pressing Decline must do, so that a test fails when the arm
+-- does something else: its answer (`r.answer`: Rerendered, Redirected …) and what it
+-- changed (`after`, the row read again), as `{ answer: r.answer, … } == { answer: Rerendered, … }`.
+test "pressing Decline" = {
+  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
+  let _ <- Test.login { user: Just me }
+  let team1 <- Db.insert Team { name: Text.NonEmpty "name-1", owner: me }
+  let invite1 <- Db.insert Invite { team: team1, email: Text.Email "email-1@example.com" }
+  let _ <- Test.get (Invites.Index {})
+  let r <- Test.click (Decline { id: invite1 })
+  let after <- Db.get Invite invite1
+  Task.succeed ?expected
+}
app/Invites/Index.alder:31:15: warning[W0002]: hole ?accept has type Task Nav
    Accept p -> ?accept,
                ^^^^^^^
app/Invites/Index.alder:32:16: warning[W0002]: hole ?decline has type Task Nav
    Decline p -> ?decline,
                 ^^^^^^^^
app/Invites/Index.alder:39:34: warning[W0002]: hole ?acceptId has type Invite.InviteId
    <al-button click={Accept { id: ?acceptId }}>Accept</al-button>
                                   ^^^^^^^^^
app/Invites/Index.alder:41:35: warning[W0002]: hole ?declineId has type Invite.InviteId
    <al-button click={Decline { id: ?declineId }}>Decline</al-button>
                                    ^^^^^^^^^^
app/Invites/Index.alder:66:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Invites/Index.alder:80:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill --decl Invites.Index.update - <<'EOF'
match msg {
  Accept p -> match List.find (\i -> i.id == p.id) model.invites {
    Nothing -> Task.notFound,
    Just inv -> {
      let already <- Db.from Member |> Db.where (Member.team.eq inv.team) |> Db.where (Member.user.eq session.user) |> Db.count
      let team <- Db.get Team inv.team
      let owns = Maybe.map (\t -> t.owner) team == Just session.user
      let _ <- Db.update Invite inv.id (\i -> { i | status: Accepted })
      let _ <- if already > 0 || owns then Task.succeed () else {
        let _ <- Db.insert Member { team: inv.team, user: session.user }
        Task.succeed ()
      }
      Task.succeed (Redirect (Teams.Show { id: inv.team }))
    },
  },
  Decline p -> match List.find (\i -> i.id == p.id) model.invites {
    Nothing -> Task.notFound,
    Just inv -> {
      let _ <- Db.update Invite inv.id (\i -> { i | status: Declined })
      Task.succeed (Redirect (Invites.Index {}))
    },
  },
}
EOF
edited app/Invites/Index.alder
--- a/app/Invites/Index.alder
+++ b/app/Invites/Index.alder
@@ -2,6 +2,7 @@
 
 import Invite
 import Layout
+import Member
 import Team
 import User
 
@@ -28,8 +29,27 @@
 
 update : Msg -> Model -> Session -> Task Nav
 update msg model session = match msg {
-  Accept p -> ?accept,
-  Decline p -> ?decline,
+  Accept p -> match List.find (\i -> i.id == p.id) model.invites {
+    Nothing -> Task.notFound,
+    Just inv -> {
+      let already <- Db.from Member |> Db.where (Member.team.eq inv.team) |> Db.where (Member.user.eq session.user) |> Db.count
+      let team <- Db.get Team inv.team
+      let owns = Maybe.map (\t -> t.owner) team == Just session.user
+      let _ <- Db.update Invite inv.id (\i -> { i | status: Accepted })
+      let _ <- if already > 0 || owns then Task.succeed () else {
+        let _ <- Db.insert Member { team: inv.team, user: session.user }
+        Task.succeed ()
+      }
+      Task.succeed (Redirect (Teams.Show { id: inv.team }))
+    },
+  },
+  Decline p -> match List.find (\i -> i.id == p.id) model.invites {
+    Nothing -> Task.notFound,
+    Just inv -> {
+      let _ <- Db.update Invite inv.id (\i -> { i | status: Declined })
+      Task.succeed (Redirect (Invites.Index {}))
+    },
+  },
 }
 
 view : Model -> Session -> Html Msg
app/Invites/Index.alder:59:34: warning[W0002]: hole ?acceptId has type Invite.InviteId
    <al-button click={Accept { id: ?acceptId }}>Accept</al-button>
                                   ^^^^^^^^^
app/Invites/Index.alder:61:35: warning[W0002]: hole ?declineId has type Invite.InviteId
    <al-button click={Decline { id: ?declineId }}>Decline</al-button>
                                    ^^^^^^^^^^
app/Invites/Index.alder:86:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Invites/Index.alder:100:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
added import Member to app/Invites/Index.alder
[exit 0]
$ alder fill --decl Invites.Index.view - <<'EOF'
<Layout.Page title="Invites">
  <h1>Invites</h1>
  <ul id="invites">
    {for i in model.invites ->
      <li>
        <span class="team">{teamName model.teams i.team}</span>
        <button class="accept" click={Accept { id: i.id }}>Accept</button>
        <button class="decline" click={Decline { id: i.id }}>Decline</button>
      </li>}
  </ul>
  <nav><a href={Teams.Index {}}>Teams</a></nav>
</Layout.Page>
EOF
edited app/Invites/Index.alder
--- a/app/Invites/Index.alder
+++ b/app/Invites/Index.alder
@@ -53,13 +53,19 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = <Layout.Page title="Invites">
-  <h1>Invites</h1>
-
-  <al-button click={Accept { id: ?acceptId }}>Accept</al-button>
-
-  <al-button click={Decline { id: ?declineId }}>Decline</al-button>
-</Layout.Page>
+view model session =
+  <Layout.Page title="Invites">
+    <h1>Invites</h1>
+    <ul id="invites">
+      {for i in model.invites ->
+        <li>
+          <span class="team">{teamName model.teams i.team}</span>
+          <button class="accept" click={Accept { id: i.id }}>Accept</button>
+          <button class="decline" click={Decline { id: i.id }}>Decline</button>
+        </li>}
+    </ul>
+    <nav><a href={Teams.Index {}}>Teams</a></nav>
+  </Layout.Page>
 
 teamName : List Team -> Team.TeamId -> Text
 teamName = \teams id -> match List.find (\t -> t.id == id) teams {
app/Invites/Index.alder:92:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Invites/Index.alder:106:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder new test Teams.Show "a team's page is not found for a non-member"
edited app/Teams/Show.alder
--- a/app/Teams/Show.alder
+++ b/app/Teams/Show.alder
@@ -113,3 +113,7 @@
   let r <- Test.submitRaw NewInvite [{ name: "email", value: "" }]
   Task.succeed (r.answer == FormRejected [{ field: "email", message: "must be an email address" }])
 }
+
+test "a team's page is not found for a non-member" = {
+  ?test
+}
app/Teams/Show.alder:96:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Teams/Show.alder:105:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Teams/Show.alder:118:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Teams.Show."a team'"'"'s page is not found for a non-member"' - <<'EOF'
{
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let eve <- Db.insert User { email: Text.Email "eve@example.com", name: Text.NonEmpty "Eve", passwordHash: "" }
  let red <- Db.insert Team { name: Text.NonEmpty "Red", owner: ann }
  let _ <- Test.login { user: Just eve }
  let r <- Test.open (Teams.Show { id: red })
  Task.succeed (r.answer == PageMissing)
}
EOF
edited app/Teams/Show.alder
--- a/app/Teams/Show.alder
+++ b/app/Teams/Show.alder
@@ -115,5 +115,10 @@
 }
 
 test "a team's page is not found for a non-member" = {
-  ?test
+  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
+  let eve <- Db.insert User { email: Text.Email "eve@example.com", name: Text.NonEmpty "Eve", passwordHash: "" }
+  let red <- Db.insert Team { name: Text.NonEmpty "Red", owner: ann }
+  let _ <- Test.login { user: Just eve }
+  let r <- Test.open (Teams.Show { id: red })
+  Task.succeed (r.answer == PageMissing)
 }
app/Teams/Show.alder:96:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Teams/Show.alder:105:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder new test Teams.Show "a member cannot remove another member"
edited app/Teams/Show.alder
--- a/app/Teams/Show.alder
+++ b/app/Teams/Show.alder
@@ -122,3 +122,7 @@
   let r <- Test.open (Teams.Show { id: red })
   Task.succeed (r.answer == PageMissing)
 }
+
+test "a member cannot remove another member" = {
+  ?test
+}
app/Teams/Show.alder:96:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Teams/Show.alder:105:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Teams/Show.alder:127:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Teams.Show."a member cannot remove another member"' - <<'EOF'
{
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let cat <- Db.insert User { email: Text.Email "cat@example.com", name: Text.NonEmpty "Cat", passwordHash: "" }
  let red <- Db.insert Team { name: Text.NonEmpty "Red", owner: ann }
  let _ <- Db.insert Member { team: red, user: bob }
  let _ <- Db.insert Member { team: red, user: cat }
  let _ <- Test.login { user: Just bob }
  let mine <- Test.get (Teams.Show { id: red })
  let _ <- Test.login { user: Just ann }
  let _ <- Test.get (Teams.Show { id: red })
  let _ <- Test.login { user: Just bob }
  let r <- Test.click (Remove { user: cat })
  let n <- Db.from Member |> Db.where (Member.team.eq red) |> Db.count
  Task.succeed ({ buttons: Test.count ".remove" mine.doc, answer: r.answer, members: n } == { buttons: 0, answer: PageMissing, members: 2 })
}
EOF
edited app/Teams/Show.alder
--- a/app/Teams/Show.alder
+++ b/app/Teams/Show.alder
@@ -124,5 +124,18 @@
 }
 
 test "a member cannot remove another member" = {
-  ?test
+  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
+  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
+  let cat <- Db.insert User { email: Text.Email "cat@example.com", name: Text.NonEmpty "Cat", passwordHash: "" }
+  let red <- Db.insert Team { name: Text.NonEmpty "Red", owner: ann }
+  let _ <- Db.insert Member { team: red, user: bob }
+  let _ <- Db.insert Member { team: red, user: cat }
+  let _ <- Test.login { user: Just bob }
+  let mine <- Test.get (Teams.Show { id: red })
+  let _ <- Test.login { user: Just ann }
+  let _ <- Test.get (Teams.Show { id: red })
+  let _ <- Test.login { user: Just bob }
+  let r <- Test.click (Remove { user: cat })
+  let n <- Db.from Member |> Db.where (Member.team.eq red) |> Db.count
+  Task.succeed ({ buttons: Test.count ".remove" mine.doc, answer: r.answer, members: n } == { buttons: 0, answer: PageMissing, members: 2 })
 }
app/Teams/Show.alder:96:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Teams/Show.alder:105:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder new test Teams.Show "the owner removes a member but not themselves"
edited app/Teams/Show.alder
--- a/app/Teams/Show.alder
+++ b/app/Teams/Show.alder
@@ -139,3 +139,7 @@
   let n <- Db.from Member |> Db.where (Member.team.eq red) |> Db.count
   Task.succeed ({ buttons: Test.count ".remove" mine.doc, answer: r.answer, members: n } == { buttons: 0, answer: PageMissing, members: 2 })
 }
+
+test "the owner removes a member but not themselves" = {
+  ?test
+}
app/Teams/Show.alder:96:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Teams/Show.alder:105:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Teams/Show.alder:144:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Teams.Show."the owner removes a member but not themselves"' - <<'EOF'
{
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let red <- Db.insert Team { name: Text.NonEmpty "Red", owner: ann }
  let _ <- Db.insert Member { team: red, user: bob }
  let _ <- Test.login { user: Just ann }
  let pg <- Test.get (Teams.Show { id: red })
  let selfRemove <- Test.run (update (Remove { user: ann }) pg.model { user: ann })
  let r <- Test.click (Remove { user: bob })
  let n <- Db.from Member |> Db.where (Member.team.eq red) |> Db.count
  Task.succeed ({ buttons: Test.count "#members .remove" pg.doc, selfRemove: selfRemove.value, removed: r.answer, members: n, names: Test.texts "#members .name" r.doc } == { buttons: 1, selfRemove: Err "not found", removed: Redirected (Teams.Show { id: red }), members: 0, names: ["Ann"] })
}
EOF
edited app/Teams/Show.alder
--- a/app/Teams/Show.alder
+++ b/app/Teams/Show.alder
@@ -141,5 +141,14 @@
 }
 
 test "the owner removes a member but not themselves" = {
-  ?test
+  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
+  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
+  let red <- Db.insert Team { name: Text.NonEmpty "Red", owner: ann }
+  let _ <- Db.insert Member { team: red, user: bob }
+  let _ <- Test.login { user: Just ann }
+  let pg <- Test.get (Teams.Show { id: red })
+  let selfRemove <- Test.run (update (Remove { user: ann }) pg.model { user: ann })
+  let r <- Test.click (Remove { user: bob })
+  let n <- Db.from Member |> Db.where (Member.team.eq red) |> Db.count
+  Task.succeed ({ buttons: Test.count "#members .remove" pg.doc, selfRemove: selfRemove.value, removed: r.answer, members: n, names: Test.texts "#members .name" r.doc } == { buttons: 1, selfRemove: Err "not found", removed: Redirected (Teams.Show { id: red }), members: 0, names: ["Ann"] })
 }
app/Teams/Show.alder:96:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Teams/Show.alder:105:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder new test Invites.Index "accepting an invite makes the team visible, declining does not"
edited app/Invites/Index.alder
--- a/app/Invites/Index.alder
+++ b/app/Invites/Index.alder
@@ -105,3 +105,7 @@
   let after <- Db.get Invite invite1
   Task.succeed ?expected
 }
+
+test "accepting an invite makes the team visible, declining does not" = {
+  ?test
+}
app/Invites/Index.alder:92:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Invites/Index.alder:106:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Invites/Index.alder:110:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Invites.Index."accepting an invite makes the team visible, declining does not"' - <<'EOF'
{
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let redId <- Db.insert Team { name: Text.NonEmpty "Red", owner: ann }
  let blueId <- Db.insert Team { name: Text.NonEmpty "Blue", owner: ann }
  let toRed <- Db.insert Invite { team: redId, email: Text.Email "bob@example.com" }
  let toBlue <- Db.insert Invite { team: blueId, email: Text.Email "bob@example.com" }
  let _ <- Test.login { user: Just bob }
  let _ <- Test.get (Invites.Index {})
  let a <- Test.click (Accept { id: toRed })
  let _ <- Test.get (Invites.Index {})
  let d <- Test.click (Decline { id: toBlue })
  let teams <- Test.get (Teams.Index {})
  let blue <- Test.open (Teams.Show { id: blueId })
  Task.succeed ({ accept: a.answer, decline: d.answer, invites: Test.count "#invites li" d.doc, teams: Test.texts "#teams .name" teams.doc, blue: blue.answer } == { accept: Redirected (Teams.Show { id: redId }), decline: Redirected (Invites.Index {}), invites: 0, teams: ["Red"], blue: PageMissing })
}
EOF
edited app/Invites/Index.alder
--- a/app/Invites/Index.alder
+++ b/app/Invites/Index.alder
@@ -107,5 +107,18 @@
 }
 
 test "accepting an invite makes the team visible, declining does not" = {
-  ?test
+  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
+  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
+  let redId <- Db.insert Team { name: Text.NonEmpty "Red", owner: ann }
+  let blueId <- Db.insert Team { name: Text.NonEmpty "Blue", owner: ann }
+  let toRed <- Db.insert Invite { team: redId, email: Text.Email "bob@example.com" }
+  let toBlue <- Db.insert Invite { team: blueId, email: Text.Email "bob@example.com" }
+  let _ <- Test.login { user: Just bob }
+  let _ <- Test.get (Invites.Index {})
+  let a <- Test.click (Accept { id: toRed })
+  let _ <- Test.get (Invites.Index {})
+  let d <- Test.click (Decline { id: toBlue })
+  let teams <- Test.get (Teams.Index {})
+  let blue <- Test.open (Teams.Show { id: blueId })
+  Task.succeed ({ accept: a.answer, decline: d.answer, invites: Test.count "#invites li" d.doc, teams: Test.texts "#teams .name" teams.doc, blue: blue.answer } == { accept: Redirected (Teams.Show { id: redId }), decline: Redirected (Invites.Index {}), invites: 0, teams: ["Red"], blue: PageMissing })
 }
app/Invites/Index.alder:92:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Invites/Index.alder:106:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill --list
Auth.Login "logging in through the form" ?afterLogin : Page
Invites.Index "pressing Accept" ?expected : Bool
Invites.Index "pressing Decline" ?expected : Bool
Teams.Index "submitting NewTeam" ?expected : Answer
Teams.Show "pressing Remove" ?expected : Bool
Teams.Show "submitting NewInvite" ?expected : Answer
[exit 0]
$ alder check
app/Auth/Login.alder:86:85: warning[W0002]: hole ?afterLogin has type Page
    Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
                                                                                      ^^^^^^^^^^^
app/Invites/Index.alder:92:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Invites/Index.alder:106:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Teams/Index.alder:62:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Teams/Show.alder:96:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Teams/Show.alder:105:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
alder check: 6 typed holes left
[exit 0]
$ alder test
app/Auth/Login.alder:86:85: warning[W0002]: hole ?afterLogin has type Page
    Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
                                                                                      ^^^^^^^^^^^
app/Invites/Index.alder:92:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Invites/Index.alder:106:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Teams/Index.alder:62:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Teams/Show.alder:96:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Teams/Show.alder:105:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
alder test: no tests ran (fill the hole): ?afterLogin : Page, ?expected : Bool, ?expected : Bool, ?expected : Answer, ?expected : Bool, ?expected : Answer
[exit 1]
```

The harness stopped at line 274 of your script: `alder test`: it exited 1. The commands after it were not run.

Your script stopped at command 44; the app is as the commands before it left it; continue.

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Auth/Login.alder:86:85: warning[W0002]: hole ?afterLogin has type Page
    Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
                                                                                      ^^^^^^^^^^^
app/Invites/Index.alder:92:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Invites/Index.alder:106:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Teams/Index.alder:62:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Teams/Show.alder:96:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Teams/Show.alder:105:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
alder build: the program has 6 typed hole(s); fill them before building
```
