### Round 3

```text
$ alder fill --decl Teams.Index.view - <<'EOF'
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
 
@@ -16,7 +17,10 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = <p>x</p>
+view model session =
+  <Layout.Page title="Teams">
+    <h1>Teams</h1>
+  </Layout.Page>
 
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
 
@@ -16,13 +17,49 @@
   Query.succeed { teams: List.sortByText (\t -> t.name) (List.unique (owned ++ joined)) }
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
 view model session =
   <Layout.Page title="Teams">
     <h1>Teams</h1>
+
+    <al-card>
+      <h2 slot="header">New team</h2>
+      <form submit={Create}>
+        <al-input field="name" label="Name" />
+        <al-alert variant="destructive"><Errors form /></al-alert>
+        <al-button type="submit">New team</al-button>
+      </form>
+    </al-card>
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
app/Teams/Index.alder:28:15: warning[W0002]: hole ?create has type Task Nav
    Create p -> ?create,
                ^^^^^^^
app/Teams/Index.alder:56:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder fill --decl Teams.Index.view - <<'EOF'
<Layout.Page title="Teams">
  <h1>Teams</h1>
  <ul id="teams">
    {for t in model.teams -> <li><a class="name" href={Teams.Show { id: t.id }}>{t.name}</a></li>}
  </ul>
  <form submit={Create}>
    <Errors form />
    <al-input field="name" label="Name" />
    <al-button type="submit">Create</al-button>
  </form>
</Layout.Page>
EOF
edited app/Teams/Index.alder
--- a/app/Teams/Index.alder
+++ b/app/Teams/Index.alder
@@ -32,15 +32,14 @@
 view model session =
   <Layout.Page title="Teams">
     <h1>Teams</h1>
-
-    <al-card>
-      <h2 slot="header">New team</h2>
-      <form submit={Create}>
-        <al-input field="name" label="Name" />
-        <al-alert variant="destructive"><Errors form /></al-alert>
-        <al-button type="submit">New team</al-button>
-      </form>
-    </al-card>
+    <ul id="teams">
+      {for t in model.teams -> <li><a class="name" href={Teams.Show { id: t.id }}>{t.name}</a></li>}
+    </ul>
+    <form submit={Create}>
+      <Errors form />
+      <al-input field="name" label="Name" />
+      <al-button type="submit">Create</al-button>
+    </form>
   </Layout.Page>
 
 test "a visitor is sent to log in" = {
app/Teams/Index.alder:28:15: warning[W0002]: hole ?create has type Task Nav
    Create p -> ?create,
                ^^^^^^^
app/Teams/Index.alder:55:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder fill Teams.Index.update.Create - <<'EOF'
{
  let id <- Db.insert Team { name: p.form.name, owner: session.user }
  Task.succeed (Redirect (Teams.Show { id: id }))
}
EOF
edited app/Teams/Index.alder
--- a/app/Teams/Index.alder
+++ b/app/Teams/Index.alder
@@ -25,7 +25,10 @@
 
 update : Msg -> Model -> Session -> Task Nav
 update msg model session = match msg {
-  Create p -> ?create,
+  Create p -> {
+    let id <- Db.insert Team { name: p.form.name, owner: session.user }
+    Task.succeed (Redirect (Teams.Show { id: id }))
+  },
 }
 
 view : Model -> Session -> Html Msg
app/Teams/Index.alder:58:29: warning[W0002]: hole ?expected has type Answer
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
$ alder fill --decl Teams.Show.load - <<'EOF'
{
  let found <- Db.get Team params.id
  match found {
    Nothing -> Query.notFound,
    Just team -> {
      let rows <- Db.from Member |> Db.where (Member.team.eq team.id) |> Db.all
      let allow = team.owner == session.user || List.any (\m -> m.user == session.user) rows
      if not allow then Query.notFound else {
        let owner <- Db.get User team.owner
        let users <- Db.from User |> Db.where (User.id.in (List.map (\m -> m.user) rows)) |> Db.all
        match owner {
          Nothing -> Query.notFound,
          Just o -> Query.succeed { team: team, owner: o, members: List.sortByText (\u -> u.name) users, isOwner: team.owner == session.user },
        }
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
 
@@ -8,7 +9,24 @@
 type Model = { team: Team, owner: User, members: List User, isOwner: Bool }
 
 load : Params -> Session -> Query Model
-load params session = ?load
+load params session = {
+  let found <- Db.get Team params.id
+  match found {
+    Nothing -> Query.notFound,
+    Just team -> {
+      let rows <- Db.from Member |> Db.where (Member.team.eq team.id) |> Db.all
+      let allow = team.owner == session.user || List.any (\m -> m.user == session.user) rows
+      if not allow then Query.notFound else {
+        let owner <- Db.get User team.owner
+        let users <- Db.from User |> Db.where (User.id.in (List.map (\m -> m.user) rows)) |> Db.all
+        match owner {
+          Nothing -> Query.notFound,
+          Just o -> Query.succeed { team: team, owner: o, members: List.sortByText (\u -> u.name) users, isOwner: team.owner == session.user },
+        }
+      }
+    },
+  }
+}
 
 view : Model -> Session -> Html Msg
 view model session = ?view
app/Teams/Show.alder:32:22: warning[W0002]: hole ?view has type Html Teams.Show.Msg
  view model session = ?view
                       ^^^^^
added import Member to app/Teams/Show.alder
[exit 0]
$ alder fill --decl Teams.Show.view - <<'EOF'
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
@@ -29,7 +30,10 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = ?view
+view model session =
+  <Layout.Page title="Team">
+    <h1>Team</h1>
+  </Layout.Page>
 
 test "a visitor is sent to log in" = {
   let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
added import Layout to app/Teams/Show.alder
[exit 0]
$ alder new form Teams.Show NewInvite email:Text.Email --msg SendInvite
edited app/Teams/Show.alder
--- a/app/Teams/Show.alder
+++ b/app/Teams/Show.alder
@@ -29,10 +29,30 @@
   }
 }
 
+form NewInvite = {
+  email: Text.Email,
+}
+
+msg SendInvite form NewInvite
+
+update : Msg -> Model -> Session -> Task Nav
+update msg model session = match msg {
+  SendInvite p -> ?sendInvite,
+}
+
 view : Model -> Session -> Html Msg
 view model session =
   <Layout.Page title="Team">
     <h1>Team</h1>
+
+    <al-card>
+      <h2 slot="header">New invite</h2>
+      <form submit={SendInvite}>
+        <al-input field="email" label="Email" />
+        <al-alert variant="destructive"><Errors form /></al-alert>
+        <al-button type="submit">New invite</al-button>
+      </form>
+    </al-card>
   </Layout.Page>
 
 test "a visitor is sent to log in" = {
@@ -41,3 +61,21 @@
   let r <- Test.open (Teams.Show { id: team1 })
   Task.succeed (r.answer == Redirected (Auth.Login {}))
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
app/Teams/Show.alder:40:19: warning[W0002]: hole ?sendInvite has type Task Nav
    SendInvite p -> ?sendInvite,
                    ^^^^^^^^^^^
app/Teams/Show.alder:71:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder new msg Teams.Show Remove user:User.UserId
edited app/Teams/Show.alder
--- a/app/Teams/Show.alder
+++ b/app/Teams/Show.alder
@@ -35,9 +35,12 @@
 
 msg SendInvite form NewInvite
 
+msg Remove { user: User.UserId }
+
 update : Msg -> Model -> Session -> Task Nav
 update msg model session = match msg {
   SendInvite p -> ?sendInvite,
+  Remove p -> ?remove,
 }
 
 view : Model -> Session -> Html Msg
@@ -53,6 +56,8 @@
         <al-button type="submit">New invite</al-button>
       </form>
     </al-card>
+
+    <al-button click={Remove { user: ?removeUser }}>Remove</al-button>
   </Layout.Page>
 
 test "a visitor is sent to log in" = {
@@ -79,3 +84,16 @@
   let r <- Test.submitRaw NewInvite [{ name: "email", value: "" }]
   Task.succeed (r.answer == FormRejected [{ field: "email", message: "must be an email address" }])
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
app/Teams/Show.alder:42:19: warning[W0002]: hole ?sendInvite has type Task Nav
    SendInvite p -> ?sendInvite,
                    ^^^^^^^^^^^
app/Teams/Show.alder:43:15: warning[W0002]: hole ?remove has type Task Nav
    Remove p -> ?remove,
                ^^^^^^^
app/Teams/Show.alder:60:38: warning[W0002]: hole ?removeUser has type User.UserId
      <al-button click={Remove { user: ?removeUser }}>Remove</al-button>
                                       ^^^^^^^^^^^
app/Teams/Show.alder:76:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Teams/Show.alder:98:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill --decl Teams.Show.view - <<'EOF'
<Layout.Page title={model.team.name}>
  <h1 id="name">{model.team.name}</h1>
  <ul id="members">
    <li><span class="name">{model.owner.name}</span></li>
    {for u in model.members ->
      <li>
        <span class="name">{u.name}</span>
        {if model.isOwner then <button class="remove" click={Remove { user: u.id }}>Remove</button> else Html.none}
      </li>}
  </ul>
  {if model.isOwner then
    <form submit={SendInvite}>
      <Errors form />
      <al-input field="email" label="Invite by email" />
      <al-button type="submit">Invite</al-button>
    </form>
  else Html.none}
</Layout.Page>
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
@@ -45,19 +46,23 @@
 
 view : Model -> Session -> Html Msg
 view model session =
-  <Layout.Page title="Team">
-    <h1>Team</h1>
-
-    <al-card>
-      <h2 slot="header">New invite</h2>
+  <Layout.Page title={model.team.name}>
+    <h1 id="name">{model.team.name}</h1>
+    <ul id="members">
+      <li><span class="name">{model.owner.name}</span></li>
+      {for u in model.members ->
+        <li>
+          <span class="name">{u.name}</span>
+          {if model.isOwner then <button class="remove" click={Remove { user: u.id }}>Remove</button> else Html.none}
+        </li>}
+    </ul>
+    {if model.isOwner then
       <form submit={SendInvite}>
-        <al-input field="email" label="Email" />
-        <al-alert variant="destructive"><Errors form /></al-alert>
-        <al-button type="submit">New invite</al-button>
+        <Errors form />
+        <al-input field="email" label="Invite by email" />
+        <al-button type="submit">Invite</al-button>
       </form>
-    </al-card>
-
-    <al-button click={Remove { user: ?removeUser }}>Remove</al-button>
+    else Html.none}
   </Layout.Page>
 
 test "a visitor is sent to log in" = {
app/Teams/Show.alder:3:1: warning[W0001]: import `Invite` is unused
  import Invite
  ^^^^^^^^^^^^^
app/Teams/Show.alder:43:19: warning[W0002]: hole ?sendInvite has type Task Nav
    SendInvite p -> ?sendInvite,
                    ^^^^^^^^^^^
app/Teams/Show.alder:44:15: warning[W0002]: hole ?remove has type Task Nav
    Remove p -> ?remove,
                ^^^^^^^
app/Teams/Show.alder:81:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Teams/Show.alder:103:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
added import Invite to app/Teams/Show.alder
[exit 0]
$ alder fill Teams.Show.update.SendInvite - <<'EOF'
if model.team.owner != session.user then Task.notFound else {
  let _ <- Db.insert Invite { team: model.team.id, email: p.form.email }
  Task.succeed (Redirect (Teams.Show { id: model.team.id }))
}
EOF
edited app/Teams/Show.alder
--- a/app/Teams/Show.alder
+++ b/app/Teams/Show.alder
@@ -40,7 +40,10 @@
 
 update : Msg -> Model -> Session -> Task Nav
 update msg model session = match msg {
-  SendInvite p -> ?sendInvite,
+  SendInvite p -> if model.team.owner != session.user then Task.notFound else {
+    let _ <- Db.insert Invite { team: model.team.id, email: p.form.email }
+    Task.succeed (Redirect (Teams.Show { id: model.team.id }))
+  },
   Remove p -> ?remove,
 }
 
app/Teams/Show.alder:47:15: warning[W0002]: hole ?remove has type Task Nav
    Remove p -> ?remove,
                ^^^^^^^
app/Teams/Show.alder:84:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Teams/Show.alder:106:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill Teams.Show.update.Remove - <<'EOF'
if model.team.owner != session.user then Task.notFound else if p.user == model.team.owner then Task.notFound else {
  let rows <- Db.from Member |> Db.where (Member.team.eq model.team.id) |> Db.where (Member.user.eq p.user) |> Db.all
  let _ <- Task.all (List.map (\m -> Db.delete Member m.id) rows)
  Task.succeed (Redirect (Teams.Show { id: model.team.id }))
}
EOF
edited app/Teams/Show.alder
--- a/app/Teams/Show.alder
+++ b/app/Teams/Show.alder
@@ -44,7 +44,11 @@
     let _ <- Db.insert Invite { team: model.team.id, email: p.form.email }
     Task.succeed (Redirect (Teams.Show { id: model.team.id }))
   },
-  Remove p -> ?remove,
+  Remove p -> if model.team.owner != session.user then Task.notFound else if p.user == model.team.owner then Task.notFound else {
+    let rows <- Db.from Member |> Db.where (Member.team.eq model.team.id) |> Db.where (Member.user.eq p.user) |> Db.all
+    let _ <- Task.all (List.map (\m -> Db.delete Member m.id) rows)
+    Task.succeed (Redirect (Teams.Show { id: model.team.id }))
+  },
 }
 
 view : Model -> Session -> Html Msg
app/Teams/Show.alder:88:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Teams/Show.alder:110:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
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
$ alder fill --decl Invites.Index.load - <<'EOF'
{
  let me <- Db.get User session.user
  match me {
    Nothing -> Query.notFound,
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
+    Nothing -> Query.notFound,
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
$ alder fill --decl Invites.Index.view - <<'EOF'
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
 
@@ -22,7 +23,10 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = ?view
+view model session =
+  <Layout.Page title="Invites">
+    <h1>Invites</h1>
+  </Layout.Page>
 
 test "a visitor is sent to log in" = {
   let r <- Test.open (Invites.Index {})
added import Layout to app/Invites/Index.alder
[exit 0]
$ alder new msg Invites.Index Accept id:Invite.InviteId
edited app/Invites/Index.alder
--- a/app/Invites/Index.alder
+++ b/app/Invites/Index.alder
@@ -22,13 +22,36 @@
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
 view model session =
   <Layout.Page title="Invites">
     <h1>Invites</h1>
+
+    <al-button click={Accept { id: ?acceptId }}>Accept</al-button>
   </Layout.Page>
 
 test "a visitor is sent to log in" = {
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
app/Invites/Index.alder:37:36: warning[W0002]: hole ?acceptId has type Invite.InviteId
      <al-button click={Accept { id: ?acceptId }}>Accept</al-button>
                                     ^^^^^^^^^
app/Invites/Index.alder:56:16: warning[W0002]: hole ?expected has type Bool
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
@@ -35,6 +38,8 @@
     <h1>Invites</h1>
 
     <al-button click={Accept { id: ?acceptId }}>Accept</al-button>
+
+    <al-button click={Decline { id: ?declineId }}>Decline</al-button>
   </Layout.Page>
 
 test "a visitor is sent to log in" = {
@@ -55,3 +60,17 @@
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
app/Invites/Index.alder:40:36: warning[W0002]: hole ?acceptId has type Invite.InviteId
      <al-button click={Accept { id: ?acceptId }}>Accept</al-button>
                                     ^^^^^^^^^
app/Invites/Index.alder:42:37: warning[W0002]: hole ?declineId has type Invite.InviteId
      <al-button click={Decline { id: ?declineId }}>Decline</al-button>
                                      ^^^^^^^^^^
app/Invites/Index.alder:61:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Invites/Index.alder:75:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill --decl Invites.Index.view - <<'EOF'
<Layout.Page title="Invites">
  <h1>Invites</h1>
  <ul id="invites">
    {for i in model.invites ->
      <li>
        <span class="team">{Maybe.withDefault "" (Maybe.map (\t -> Text.trim t.name) (List.find (\t -> t.id == i.team) model.teams))}</span>
        <button class="accept" click={Accept { id: i.id }}>Accept</button>
        <button class="decline" click={Decline { id: i.id }}>Decline</button>
      </li>}
  </ul>
</Layout.Page>
EOF
edited app/Invites/Index.alder
--- a/app/Invites/Index.alder
+++ b/app/Invites/Index.alder
@@ -36,10 +36,14 @@
 view model session =
   <Layout.Page title="Invites">
     <h1>Invites</h1>
-
-    <al-button click={Accept { id: ?acceptId }}>Accept</al-button>
-
-    <al-button click={Decline { id: ?declineId }}>Decline</al-button>
+    <ul id="invites">
+      {for i in model.invites ->
+        <li>
+          <span class="team">{Maybe.withDefault "" (Maybe.map (\t -> Text.trim t.name) (List.find (\t -> t.id == i.team) model.teams))}</span>
+          <button class="accept" click={Accept { id: i.id }}>Accept</button>
+          <button class="decline" click={Decline { id: i.id }}>Decline</button>
+        </li>}
+    </ul>
   </Layout.Page>
 
 test "a visitor is sent to log in" = {
app/Invites/Index.alder:31:15: warning[W0002]: hole ?accept has type Task Nav
    Accept p -> ?accept,
                ^^^^^^^
app/Invites/Index.alder:32:16: warning[W0002]: hole ?decline has type Task Nav
    Decline p -> ?decline,
                 ^^^^^^^^
app/Invites/Index.alder:65:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Invites/Index.alder:79:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill Invites.Index.update.Accept - <<'EOF'
{
  let found <- Db.get Invite p.id
  let me <- Db.get User session.user
  match found {
    Nothing -> Task.notFound,
    Just inv -> {
      let mine = Maybe.map (\u -> u.email == inv.email) me == Just True
      if not mine || inv.status != Pending then Task.notFound else {
        let team <- Db.get Team inv.team
        let owned = Maybe.map (\t -> t.owner == session.user) team == Just True
        let existing <- Db.from Member |> Db.where (Member.team.eq inv.team) |> Db.where (Member.user.eq session.user) |> Db.count
        let _ <- Db.update Invite inv.id (\i -> { i | status: Accepted })
        let _ <- if owned || existing > 0 then Task.succeed () else Task.map (\x -> ()) (Db.insert Member { team: inv.team, user: session.user })
        Task.succeed (Redirect (Teams.Show { id: inv.team }))
      }
    },
  }
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
 
@@ -28,7 +29,24 @@
 
 update : Msg -> Model -> Session -> Task Nav
 update msg model session = match msg {
-  Accept p -> ?accept,
+  Accept p -> {
+    let found <- Db.get Invite p.id
+    let me <- Db.get User session.user
+    match found {
+      Nothing -> Task.notFound,
+      Just inv -> {
+        let mine = Maybe.map (\u -> u.email == inv.email) me == Just True
+        if not mine || inv.status != Pending then Task.notFound else {
+          let team <- Db.get Team inv.team
+          let owned = Maybe.map (\t -> t.owner == session.user) team == Just True
+          let existing <- Db.from Member |> Db.where (Member.team.eq inv.team) |> Db.where (Member.user.eq session.user) |> Db.count
+          let _ <- Db.update Invite inv.id (\i -> { i | status: Accepted })
+          let _ <- if owned || existing > 0 then Task.succeed () else Task.map (\x -> ()) (Db.insert Member { team: inv.team, user: session.user })
+          Task.succeed (Redirect (Teams.Show { id: inv.team }))
+        }
+      },
+    }
+  },
   Decline p -> ?decline,
 }
 
app/Invites/Index.alder:50:16: warning[W0002]: hole ?decline has type Task Nav
    Decline p -> ?decline,
                 ^^^^^^^^
app/Invites/Index.alder:83:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Invites/Index.alder:97:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
added import Member to app/Invites/Index.alder
[exit 0]
$ alder fill Invites.Index.update.Decline - <<'EOF'
{
  let found <- Db.get Invite p.id
  let me <- Db.get User session.user
  match found {
    Nothing -> Task.notFound,
    Just inv -> {
      let mine = Maybe.map (\u -> u.email == inv.email) me == Just True
      if not mine || inv.status != Pending then Task.notFound else {
        let _ <- Db.update Invite inv.id (\i -> { i | status: Declined })
        Task.succeed (Redirect (Invites.Index {}))
      }
    },
  }
}
EOF
edited app/Invites/Index.alder
--- a/app/Invites/Index.alder
+++ b/app/Invites/Index.alder
@@ -47,7 +47,20 @@
       },
     }
   },
-  Decline p -> ?decline,
+  Decline p -> {
+    let found <- Db.get Invite p.id
+    let me <- Db.get User session.user
+    match found {
+      Nothing -> Task.notFound,
+      Just inv -> {
+        let mine = Maybe.map (\u -> u.email == inv.email) me == Just True
+        if not mine || inv.status != Pending then Task.notFound else {
+          let _ <- Db.update Invite inv.id (\i -> { i | status: Declined })
+          Task.succeed (Redirect (Invites.Index {}))
+        }
+      },
+    }
+  },
 }
 
 view : Model -> Session -> Html Msg
app/Invites/Index.alder:96:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Invites/Index.alder:110:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder new test Teams.Show "a team page is not found for a non-member"
edited app/Teams/Show.alder
--- a/app/Teams/Show.alder
+++ b/app/Teams/Show.alder
@@ -109,3 +109,7 @@
   let after <- Db.get User me
   Task.succeed ?expected
 }
+
+test "a team page is not found for a non-member" = {
+  ?test
+}
app/Teams/Show.alder:88:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Teams/Show.alder:110:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Teams/Show.alder:114:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Teams.Show."a team page is not found for a non-member"' - <<'EOF'
{
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let eve <- Db.insert User { email: Text.Email "eve@example.com", name: Text.NonEmpty "Eve", passwordHash: "" }
  let t <- Db.insert Team { name: Text.NonEmpty "Red", owner: ann }
  let _ <- Test.login { user: Just eve }
  let r <- Test.open (Teams.Show { id: t })
  Task.succeed (r.answer == PageMissing)
}
EOF
edited app/Teams/Show.alder
--- a/app/Teams/Show.alder
+++ b/app/Teams/Show.alder
@@ -111,5 +111,10 @@
 }
 
 test "a team page is not found for a non-member" = {
-  ?test
+  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
+  let eve <- Db.insert User { email: Text.Email "eve@example.com", name: Text.NonEmpty "Eve", passwordHash: "" }
+  let t <- Db.insert Team { name: Text.NonEmpty "Red", owner: ann }
+  let _ <- Test.login { user: Just eve }
+  let r <- Test.open (Teams.Show { id: t })
+  Task.succeed (r.answer == PageMissing)
 }
app/Teams/Show.alder:88:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Teams/Show.alder:110:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder new test Teams.Show "a member cannot remove another member"
edited app/Teams/Show.alder
--- a/app/Teams/Show.alder
+++ b/app/Teams/Show.alder
@@ -118,3 +118,7 @@
   let r <- Test.open (Teams.Show { id: t })
   Task.succeed (r.answer == PageMissing)
 }
+
+test "a member cannot remove another member" = {
+  ?test
+}
app/Teams/Show.alder:88:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Teams/Show.alder:110:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Teams/Show.alder:123:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Teams.Show."a member cannot remove another member"' - <<'EOF'
{
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let cat <- Db.insert User { email: Text.Email "cat@example.com", name: Text.NonEmpty "Cat", passwordHash: "" }
  let t <- Db.insert Team { name: Text.NonEmpty "Red", owner: ann }
  let _ <- Db.insert Member { team: t, user: bob }
  let _ <- Db.insert Member { team: t, user: cat }
  let _ <- Test.login { user: Just ann }
  let _ <- Test.get (Teams.Show { id: t })
  let _ <- Test.login { user: Just bob }
  let r <- Test.click (Remove { user: cat })
  let n <- Db.from Member |> Db.count
  Task.succeed ({ answer: r.answer, members: n } == { answer: PageMissing, members: 2 })
}
EOF
edited app/Teams/Show.alder
--- a/app/Teams/Show.alder
+++ b/app/Teams/Show.alder
@@ -120,5 +120,16 @@
 }
 
 test "a member cannot remove another member" = {
-  ?test
+  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
+  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
+  let cat <- Db.insert User { email: Text.Email "cat@example.com", name: Text.NonEmpty "Cat", passwordHash: "" }
+  let t <- Db.insert Team { name: Text.NonEmpty "Red", owner: ann }
+  let _ <- Db.insert Member { team: t, user: bob }
+  let _ <- Db.insert Member { team: t, user: cat }
+  let _ <- Test.login { user: Just ann }
+  let _ <- Test.get (Teams.Show { id: t })
+  let _ <- Test.login { user: Just bob }
+  let r <- Test.click (Remove { user: cat })
+  let n <- Db.from Member |> Db.count
+  Task.succeed ({ answer: r.answer, members: n } == { answer: PageMissing, members: 2 })
 }
app/Teams/Show.alder:88:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Teams/Show.alder:110:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder new test Teams.Show "the owner removes a member but not themselves"
edited app/Teams/Show.alder
--- a/app/Teams/Show.alder
+++ b/app/Teams/Show.alder
@@ -133,3 +133,7 @@
   let n <- Db.from Member |> Db.count
   Task.succeed ({ answer: r.answer, members: n } == { answer: PageMissing, members: 2 })
 }
+
+test "the owner removes a member but not themselves" = {
+  ?test
+}
app/Teams/Show.alder:88:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Teams/Show.alder:110:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Teams/Show.alder:138:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Teams.Show."the owner removes a member but not themselves"' - <<'EOF'
{
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let t <- Db.insert Team { name: Text.NonEmpty "Red", owner: ann }
  let _ <- Db.insert Member { team: t, user: bob }
  let _ <- Test.login { user: Just ann }
  let page <- Test.get (Teams.Show { id: t })
  let own <- Test.run (update (Remove { user: ann }) page.model { user: ann })
  let r <- Test.click (Remove { user: bob })
  let n <- Db.from Member |> Db.count
  Task.succeed ({ own: own.value, answer: r.answer, members: n } == { own: Err "not found", answer: Redirected (Teams.Show { id: t }), members: 0 })
}
EOF
edited app/Teams/Show.alder
--- a/app/Teams/Show.alder
+++ b/app/Teams/Show.alder
@@ -135,5 +135,14 @@
 }
 
 test "the owner removes a member but not themselves" = {
-  ?test
+  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
+  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
+  let t <- Db.insert Team { name: Text.NonEmpty "Red", owner: ann }
+  let _ <- Db.insert Member { team: t, user: bob }
+  let _ <- Test.login { user: Just ann }
+  let page <- Test.get (Teams.Show { id: t })
+  let own <- Test.run (update (Remove { user: ann }) page.model { user: ann })
+  let r <- Test.click (Remove { user: bob })
+  let n <- Db.from Member |> Db.count
+  Task.succeed ({ own: own.value, answer: r.answer, members: n } == { own: Err "not found", answer: Redirected (Teams.Show { id: t }), members: 0 })
 }
app/Teams/Show.alder:88:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Teams/Show.alder:110:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder new test Invites.Index "accepting an invite makes the team visible"
edited app/Invites/Index.alder
--- a/app/Invites/Index.alder
+++ b/app/Invites/Index.alder
@@ -109,3 +109,7 @@
   let after <- Db.get Invite invite1
   Task.succeed ?expected
 }
+
+test "accepting an invite makes the team visible" = {
+  ?test
+}
app/Invites/Index.alder:96:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Invites/Index.alder:110:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Invites/Index.alder:114:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Invites.Index."accepting an invite makes the team visible"' - <<'EOF'
{
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let t <- Db.insert Team { name: Text.NonEmpty "Red", owner: ann }
  let inv <- Db.insert Invite { team: t, email: Text.Email "bob@example.com" }
  let _ <- Test.login { user: Just bob }
  let _ <- Test.get (Invites.Index {})
  let r <- Test.click (Accept { id: inv })
  let page <- Test.get (Teams.Index {})
  Task.succeed ({ answer: r.answer, teams: Test.texts "#teams .name" page.doc } == { answer: Redirected (Teams.Show { id: t }), teams: ["Red"] })
}
EOF
edited app/Invites/Index.alder
--- a/app/Invites/Index.alder
+++ b/app/Invites/Index.alder
@@ -111,5 +111,13 @@
 }
 
 test "accepting an invite makes the team visible" = {
-  ?test
+  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
+  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
+  let t <- Db.insert Team { name: Text.NonEmpty "Red", owner: ann }
+  let inv <- Db.insert Invite { team: t, email: Text.Email "bob@example.com" }
+  let _ <- Test.login { user: Just bob }
+  let _ <- Test.get (Invites.Index {})
+  let r <- Test.click (Accept { id: inv })
+  let page <- Test.get (Teams.Index {})
+  Task.succeed ({ answer: r.answer, teams: Test.texts "#teams .name" page.doc } == { answer: Redirected (Teams.Show { id: t }), teams: ["Red"] })
 }
app/Invites/Index.alder:96:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Invites/Index.alder:110:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder new test Invites.Index "declining an invite keeps the team closed"
edited app/Invites/Index.alder
--- a/app/Invites/Index.alder
+++ b/app/Invites/Index.alder
@@ -121,3 +121,7 @@
   let page <- Test.get (Teams.Index {})
   Task.succeed ({ answer: r.answer, teams: Test.texts "#teams .name" page.doc } == { answer: Redirected (Teams.Show { id: t }), teams: ["Red"] })
 }
+
+test "declining an invite keeps the team closed" = {
+  ?test
+}
app/Invites/Index.alder:96:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Invites/Index.alder:110:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Invites/Index.alder:126:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Invites.Index."declining an invite keeps the team closed"' - <<'EOF'
{
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let t <- Db.insert Team { name: Text.NonEmpty "Red", owner: ann }
  let inv <- Db.insert Invite { team: t, email: Text.Email "bob@example.com" }
  let _ <- Test.login { user: Just bob }
  let _ <- Test.get (Invites.Index {})
  let r <- Test.click (Decline { id: inv })
  let page <- Test.get (Teams.Index {})
  let show <- Test.open (Teams.Show { id: t })
  Task.succeed ({ answer: r.answer, teams: Test.count "#teams li" page.doc, show: show.answer } == { answer: Redirected (Invites.Index {}), teams: 0, show: PageMissing })
}
EOF
edited app/Invites/Index.alder
--- a/app/Invites/Index.alder
+++ b/app/Invites/Index.alder
@@ -123,5 +123,14 @@
 }
 
 test "declining an invite keeps the team closed" = {
-  ?test
+  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
+  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
+  let t <- Db.insert Team { name: Text.NonEmpty "Red", owner: ann }
+  let inv <- Db.insert Invite { team: t, email: Text.Email "bob@example.com" }
+  let _ <- Test.login { user: Just bob }
+  let _ <- Test.get (Invites.Index {})
+  let r <- Test.click (Decline { id: inv })
+  let page <- Test.get (Teams.Index {})
+  let show <- Test.open (Teams.Show { id: t })
+  Task.succeed ({ answer: r.answer, teams: Test.count "#teams li" page.doc, show: show.answer } == { answer: Redirected (Invites.Index {}), teams: 0, show: PageMissing })
 }
app/Invites/Index.alder:96:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Invites/Index.alder:110:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill --list
Auth.Login update.Submit ?afterLogin : Page
Auth.Login "logging in through the form" ?afterLogin : Page
Invites.Index "pressing Accept" ?expected : Bool
Invites.Index "pressing Decline" ?expected : Bool
Teams.Index "submitting NewTeam" ?expected : Answer
Teams.Show "submitting NewInvite" ?expected : Answer
Teams.Show "pressing Remove" ?expected : Bool
[exit 0]
$ alder fill Auth.Login.update.Submit - --hole ?afterLogin <<'EOF'
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
```

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Auth/Login.alder:86:85: warning[W0002]: hole ?afterLogin has type Page
    Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
                                                                                      ^^^^^^^^^^^
app/Invites/Index.alder:96:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Invites/Index.alder:110:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Teams/Index.alder:58:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Teams/Show.alder:88:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Teams/Show.alder:110:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
alder build: the program has 6 typed hole(s); fill them before building
```
