### Round 2

```text
$ alder fill Auth.Login.update - --hole ?afterLogin <<'EOF'
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
$ alder fill --decl Teams.Index.load - <<'EOF'
{
  let memberships <- Db.from Member |> Db.where (Member.user.eq session.user) |> Db.all
  let teams <-
    Db.from Team
      |> Db.where (Db.or [Team.owner.eq session.user, Team.id.in (List.map (\m -> m.team) memberships)])
      |> Db.orderBy Team.name.asc
      |> Db.all
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
@@ -7,7 +8,15 @@
 type Model = { teams: List Team }
 
 load : Params -> Session -> Query Model
-load params session = ?load
+load params session = {
+  let memberships <- Db.from Member |> Db.where (Member.user.eq session.user) |> Db.all
+  let teams <-
+    Db.from Team
+      |> Db.where (Db.or [Team.owner.eq session.user, Team.id.in (List.map (\m -> m.team) memberships)])
+      |> Db.orderBy Team.name.asc
+      |> Db.all
+  Query.succeed { teams: teams }
+}
 
 view : Model -> Session -> Html Msg
 view model session = ?view
app/Teams/Index.alder:22:22: warning[W0002]: hole ?view has type Html Teams.Index.Msg
  view model session = ?view
                       ^^^^^
added import Member to app/Teams/Index.alder
[exit 0]
$ alder new page Teams.Show '/teams/{id}' --blank --param id:Team.TeamId --requires user
wrote app/Teams/Show.alder
app/Teams/Show.alder:11:23: warning[W0002]: hole ?load has type Query Teams.Show.Model
  load params session = ?load
                        ^^^^^
app/Teams/Show.alder:14:22: warning[W0002]: hole ?view has type Html Teams.Show.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder fill --decl Teams.Show.Model - <<'EOF'
{ team: Team, isOwner: Bool, people: List User }
EOF
edited app/Teams/Show.alder
--- a/app/Teams/Show.alder
+++ b/app/Teams/Show.alder
@@ -5,7 +5,7 @@
 
 page "/teams/{id}" { id: Team.TeamId } requires user
 
-type Model = { }
+type Model = { team: Team, isOwner: Bool, people: List User }
 
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
      let isMember = List.any (\m -> m.user == session.user) rows
      let isOwner = team.owner == session.user
      if not isOwner && not isMember then Query.notFound else {
        let ids = List.map (\m -> m.user) rows
        let users <- Db.from User |> Db.where (User.id.in ([team.owner] ++ ids)) |> Db.all
        let owners = List.filter (\u -> u.id == team.owner) users
        let others = List.sortByText (\u -> u.name) (List.filter (\u -> u.id != team.owner) users)
        Query.succeed { team: team, isOwner: isOwner, people: owners ++ others }
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
 type Model = { team: Team, isOwner: Bool, people: List User }
 
 load : Params -> Session -> Query Model
-load params session = ?load
+load params session = {
+  let found <- Db.get Team params.id
+  match found {
+    Nothing -> Query.notFound,
+    Just team -> {
+      let rows <- Db.from Member |> Db.where (Member.team.eq team.id) |> Db.all
+      let isMember = List.any (\m -> m.user == session.user) rows
+      let isOwner = team.owner == session.user
+      if not isOwner && not isMember then Query.notFound else {
+        let ids = List.map (\m -> m.user) rows
+        let users <- Db.from User |> Db.where (User.id.in ([team.owner] ++ ids)) |> Db.all
+        let owners = List.filter (\u -> u.id == team.owner) users
+        let others = List.sortByText (\u -> u.name) (List.filter (\u -> u.id != team.owner) users)
+        Query.succeed { team: team, isOwner: isOwner, people: owners ++ others }
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
<Layout.Page title={model.team.name}>
  <h1 id="name">{model.team.name}</h1>
  <ul id="members">
    {for u in model.people ->
      <li><span class="name">{u.name}</span></li>}
  </ul>
  <al-button variant="outline" click={App.Logout}>Log out</al-button>
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
@@ -29,7 +30,15 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = ?view
+view model session =
+  <Layout.Page title={model.team.name}>
+    <h1 id="name">{model.team.name}</h1>
+    <ul id="members">
+      {for u in model.people ->
+        <li><span class="name">{u.name}</span></li>}
+    </ul>
+    <al-button variant="outline" click={App.Logout}>Log out</al-button>
+  </Layout.Page>
 
 test "a visitor is sent to log in" = {
   let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
added import Layout to app/Teams/Show.alder
[exit 0]
$ alder new msg Teams.Show Remove user:User.UserId
edited app/Teams/Show.alder
--- a/app/Teams/Show.alder
+++ b/app/Teams/Show.alder
@@ -29,6 +29,13 @@
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
 view model session =
   <Layout.Page title={model.team.name}>
@@ -38,6 +45,8 @@
         <li><span class="name">{u.name}</span></li>}
     </ul>
     <al-button variant="outline" click={App.Logout}>Log out</al-button>
+
+    <al-button click={Remove { user: ?removeUser }}>Remove</al-button>
   </Layout.Page>
 
 test "a visitor is sent to log in" = {
@@ -46,3 +55,16 @@
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
app/Teams/Show.alder:36:15: warning[W0002]: hole ?remove has type Task Nav
    Remove p -> ?remove,
                ^^^^^^^
app/Teams/Show.alder:49:38: warning[W0002]: hole ?removeUser has type User.UserId
      <al-button click={Remove { user: ?removeUser }}>Remove</al-button>
                                       ^^^^^^^^^^^
app/Teams/Show.alder:69:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder new form Teams.Show NewInvite email:Text.Email --msg SendInvite
edited app/Teams/Show.alder
--- a/app/Teams/Show.alder
+++ b/app/Teams/Show.alder
@@ -31,9 +31,16 @@
 
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
@@ -47,6 +54,15 @@
     <al-button variant="outline" click={App.Logout}>Log out</al-button>
 
     <al-button click={Remove { user: ?removeUser }}>Remove</al-button>
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
@@ -68,3 +84,21 @@
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
app/Teams/Show.alder:42:15: warning[W0002]: hole ?remove has type Task Nav
    Remove p -> ?remove,
                ^^^^^^^
app/Teams/Show.alder:43:19: warning[W0002]: hole ?sendInvite has type Task Nav
    SendInvite p -> ?sendInvite,
                    ^^^^^^^^^^^
app/Teams/Show.alder:56:38: warning[W0002]: hole ?removeUser has type User.UserId
      <al-button click={Remove { user: ?removeUser }}>Remove</al-button>
                                       ^^^^^^^^^^^
app/Teams/Show.alder:85:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Teams/Show.alder:94:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder fill --decl Teams.Show.view - <<'EOF'
<Layout.Page title={model.team.name}>
  <h1 id="name">{model.team.name}</h1>
  <ul id="members">
    {for u in model.people ->
      <li>
        <span class="name">{u.name}</span>
        {if u.id == model.team.owner then <span class="role">owner</span> else Html.none}
        {if model.isOwner && u.id != model.team.owner then <button class="remove" click={Remove { user: u.id }}>Remove</button> else Html.none}
      </li>}
  </ul>
  {if model.isOwner then <al-card><form submit={SendInvite}><Errors form /><al-input field="email" label="Invite by email" /><al-button type="submit">Invite</al-button></form></al-card> else Html.none}
  <al-button variant="outline" click={App.Logout}>Log out</al-button>
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
@@ -49,20 +50,14 @@
     <h1 id="name">{model.team.name}</h1>
     <ul id="members">
       {for u in model.people ->
-        <li><span class="name">{u.name}</span></li>}
+        <li>
+          <span class="name">{u.name}</span>
+          {if u.id == model.team.owner then <span class="role">owner</span> else Html.none}
+          {if model.isOwner && u.id != model.team.owner then <button class="remove" click={Remove { user: u.id }}>Remove</button> else Html.none}
+        </li>}
     </ul>
+    {if model.isOwner then <al-card><form submit={SendInvite}><Errors form /><al-input field="email" label="Invite by email" /><al-button type="submit">Invite</al-button></form></al-card> else Html.none}
     <al-button variant="outline" click={App.Logout}>Log out</al-button>
-
-    <al-button click={Remove { user: ?removeUser }}>Remove</al-button>
-
-    <al-card>
-      <h2 slot="header">New invite</h2>
-      <form submit={SendInvite}>
-        <al-input field="email" label="Email" />
-        <al-alert variant="destructive"><Errors form /></al-alert>
-        <al-button type="submit">New invite</al-button>
-      </form>
-    </al-card>
   </Layout.Page>
 
 test "a visitor is sent to log in" = {
app/Teams/Show.alder:3:1: warning[W0001]: import `Invite` is unused
  import Invite
  ^^^^^^^^^^^^^
app/Teams/Show.alder:43:15: warning[W0002]: hole ?remove has type Task Nav
    Remove p -> ?remove,
                ^^^^^^^
app/Teams/Show.alder:44:19: warning[W0002]: hole ?sendInvite has type Task Nav
    SendInvite p -> ?sendInvite,
                    ^^^^^^^^^^^
app/Teams/Show.alder:80:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Teams/Show.alder:89:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
added import Invite to app/Teams/Show.alder
[exit 0]
$ alder fill --decl Teams.Show.update - <<'EOF'
match msg {
  Remove p -> {
    if not model.isOwner || p.user == model.team.owner then Task.notFound else {
      let rows <- Db.from Member |> Db.where (Member.team.eq model.team.id) |> Db.where (Member.user.eq p.user) |> Db.all
      let _ <- Task.all (List.map (\r -> Db.delete Member r.id) rows)
      Task.succeed (Redirect (Teams.Show { id: model.team.id }))
    }
  },
  SendInvite p -> {
    if not model.isOwner then Task.notFound else {
      let _ <- Db.insert Invite { team: model.team.id, email: p.form.email }
      Task.succeed (Redirect (Teams.Show { id: model.team.id }))
    }
  },
}
EOF
edited app/Teams/Show.alder
--- a/app/Teams/Show.alder
+++ b/app/Teams/Show.alder
@@ -40,8 +40,19 @@
 
 update : Msg -> Model -> Session -> Task Nav
 update msg model session = match msg {
-  Remove p -> ?remove,
-  SendInvite p -> ?sendInvite,
+  Remove p -> {
+    if not model.isOwner || p.user == model.team.owner then Task.notFound else {
+      let rows <- Db.from Member |> Db.where (Member.team.eq model.team.id) |> Db.where (Member.user.eq p.user) |> Db.all
+      let _ <- Task.all (List.map (\r -> Db.delete Member r.id) rows)
+      Task.succeed (Redirect (Teams.Show { id: model.team.id }))
+    }
+  },
+  SendInvite p -> {
+    if not model.isOwner then Task.notFound else {
+      let _ <- Db.insert Invite { team: model.team.id, email: p.form.email }
+      Task.succeed (Redirect (Teams.Show { id: model.team.id }))
+    }
+  },
 }
 
 view : Model -> Session -> Html Msg
app/Teams/Show.alder:91:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Teams/Show.alder:100:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder fill --decl Teams.Index.view - <<'EOF'
<Layout.Page title="Teams">
  <h1>Teams</h1>
  <ul id="teams">
    {for t in model.teams ->
      <li><a href={Teams.Show { id: t.id }}><span class="name">{t.name}</span></a></li>}
  </ul>
  <al-button variant="outline" click={App.Logout}>Log out</al-button>
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
 
@@ -19,7 +20,15 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = ?view
+view model session =
+  <Layout.Page title="Teams">
+    <h1>Teams</h1>
+    <ul id="teams">
+      {for t in model.teams ->
+        <li><a href={Teams.Show { id: t.id }}><span class="name">{t.name}</span></a></li>}
+    </ul>
+    <al-button variant="outline" click={App.Logout}>Log out</al-button>
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
 
@@ -19,6 +20,17 @@
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
 view model session =
   <Layout.Page title="Teams">
@@ -28,9 +40,34 @@
         <li><a href={Teams.Show { id: t.id }}><span class="name">{t.name}</span></a></li>}
     </ul>
     <al-button variant="outline" click={App.Logout}>Log out</al-button>
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
app/Teams/Index.alder:31:15: warning[W0002]: hole ?create has type Task Nav
    Create p -> ?create,
                ^^^^^^^
app/Teams/Index.alder:64:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder fill --decl Teams.Index.view - <<'EOF'
<Layout.Page title="Teams">
  <h1>Teams</h1>
  <ul id="teams">
    {for t in model.teams ->
      <li><a href={Teams.Show { id: t.id }}><span class="name">{t.name}</span></a></li>}
  </ul>
  <al-card>
    <form submit={Create}>
      <Errors form />
      <al-input field="name" label="New team name" />
      <al-button type="submit">Create team</al-button>
    </form>
  </al-card>
  <al-button variant="outline" click={App.Logout}>Log out</al-button>
</Layout.Page>
EOF
edited app/Teams/Index.alder
--- a/app/Teams/Index.alder
+++ b/app/Teams/Index.alder
@@ -39,16 +39,14 @@
       {for t in model.teams ->
         <li><a href={Teams.Show { id: t.id }}><span class="name">{t.name}</span></a></li>}
     </ul>
-    <al-button variant="outline" click={App.Logout}>Log out</al-button>
-
     <al-card>
-      <h2 slot="header">New team</h2>
       <form submit={Create}>
-        <al-input field="name" label="Name" />
-        <al-alert variant="destructive"><Errors form /></al-alert>
-        <al-button type="submit">New team</al-button>
+        <Errors form />
+        <al-input field="name" label="New team name" />
+        <al-button type="submit">Create team</al-button>
       </form>
     </al-card>
+    <al-button variant="outline" click={App.Logout}>Log out</al-button>
   </Layout.Page>
 
 test "a visitor is sent to log in" = {
app/Teams/Index.alder:31:15: warning[W0002]: hole ?create has type Task Nav
    Create p -> ?create,
                ^^^^^^^
app/Teams/Index.alder:62:29: warning[W0002]: hole ?expected has type Answer
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
@@ -28,7 +28,10 @@
 
 update : Msg -> Model -> Session -> Task Nav
 update msg model session = match msg {
-  Create p -> ?create,
+  Create p -> {
+    let id <- Db.insert Team { name: p.form.name, owner: session.user }
+    Task.succeed (Redirect (Teams.Show { id: id }))
+  },
 }
 
 view : Model -> Session -> Html Msg
app/Teams/Index.alder:65:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder new page Invites.Index '/invites' --blank --requires user
wrote app/Invites/Index.alder
app/Invites/Index.alder:8:23: warning[W0002]: hole ?load has type Query Invites.Index.Model
  load params session = ?load
                        ^^^^^
app/Invites/Index.alder:11:22: warning[W0002]: hole ?view has type Html Invites.Index.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder new type Invites.Index "Waiting = { id: Invite.InviteId, team: Team }"
edited app/Invites/Index.alder
--- a/app/Invites/Index.alder
+++ b/app/Invites/Index.alder
@@ -1,9 +1,14 @@
 module Invites.Index
 
+import Invite
+import Team
+
 page "/invites" { } requires user
 
 type Model = { }
 
+type Waiting = { id: Invite.InviteId, team: Team }
+
 load : Params -> Session -> Query Model
 load params session = ?load
 
app/Invites/Index.alder:13:23: warning[W0002]: hole ?load has type Query Invites.Index.Model
  load params session = ?load
                        ^^^^^
app/Invites/Index.alder:16:22: warning[W0002]: hole ?view has type Html Invites.Index.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder fill --decl Invites.Index.Model - <<'EOF'
{ waiting: List Waiting }
EOF
edited app/Invites/Index.alder
--- a/app/Invites/Index.alder
+++ b/app/Invites/Index.alder
@@ -5,7 +5,7 @@
 
 page "/invites" { } requires user
 
-type Model = { }
+type Model = { waiting: List Waiting }
 
 type Waiting = { id: Invite.InviteId, team: Team }
 
app/Invites/Index.alder:13:23: warning[W0002]: hole ?load has type Query Invites.Index.Model
  load params session = ?load
                        ^^^^^
app/Invites/Index.alder:16:22: warning[W0002]: hole ?view has type Html Invites.Index.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder fill --decl Invites.Index.load - <<'EOF'
{
  let me <- Db.get User session.user
  match me {
    Nothing -> Query.notFound,
    Just u -> {
      let rows <- Db.from Invite |> Db.where (Invite.email.eq u.email) |> Db.where (Invite.status.eq Pending) |> Db.all
      let teams <- Db.from Team |> Db.where (Team.id.in (List.map (\i -> i.team) rows)) |> Db.all
      let pair : Invite -> Team -> Waiting = \i t -> { id: i.id, team: t }
      let waiting : List Waiting = List.filterMap (\i -> Maybe.map (pair i) (List.find (\t -> t.id == i.team) teams)) rows
      Query.succeed { waiting: waiting }
    },
  }
}
EOF
edited app/Invites/Index.alder
--- a/app/Invites/Index.alder
+++ b/app/Invites/Index.alder
@@ -2,6 +2,7 @@
 
 import Invite
 import Team
+import User
 
 page "/invites" { } requires user
 
@@ -10,7 +11,19 @@
 type Waiting = { id: Invite.InviteId, team: Team }
 
 load : Params -> Session -> Query Model
-load params session = ?load
+load params session = {
+  let me <- Db.get User session.user
+  match me {
+    Nothing -> Query.notFound,
+    Just u -> {
+      let rows <- Db.from Invite |> Db.where (Invite.email.eq u.email) |> Db.where (Invite.status.eq Pending) |> Db.all
+      let teams <- Db.from Team |> Db.where (Team.id.in (List.map (\i -> i.team) rows)) |> Db.all
+      let pair : Invite -> Team -> Waiting = \i t -> { id: i.id, team: t }
+      let waiting : List Waiting = List.filterMap (\i -> Maybe.map (pair i) (List.find (\t -> t.id == i.team) teams)) rows
+      Query.succeed { waiting: waiting }
+    },
+  }
+}
 
 view : Model -> Session -> Html Msg
 view model session = ?view
app/Invites/Index.alder:29:22: warning[W0002]: hole ?view has type Html Invites.Index.Msg
  view model session = ?view
                       ^^^^^
added import User to app/Invites/Index.alder
[exit 0]
$ alder fill --decl Invites.Index.view - <<'EOF'
<Layout.Page title="Invites">
  <h1>Invites</h1>
  <ul id="invites">
    {for w in model.waiting ->
      <li><span class="team">{w.team.name}</span></li>}
  </ul>
  <al-button variant="outline" click={App.Logout}>Log out</al-button>
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
 
@@ -26,7 +27,15 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = ?view
+view model session =
+  <Layout.Page title="Invites">
+    <h1>Invites</h1>
+    <ul id="invites">
+      {for w in model.waiting ->
+        <li><span class="team">{w.team.name}</span></li>}
+    </ul>
+    <al-button variant="outline" click={App.Logout}>Log out</al-button>
+  </Layout.Page>
 
 test "a visitor is sent to log in" = {
   let r <- Test.open (Invites.Index {})
added import Layout to app/Invites/Index.alder
[exit 0]
$ alder new msg Invites.Index Accept id:Invite.InviteId
edited app/Invites/Index.alder
--- a/app/Invites/Index.alder
+++ b/app/Invites/Index.alder
@@ -26,6 +26,13 @@
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
@@ -35,9 +42,25 @@
         <li><span class="team">{w.team.name}</span></li>}
     </ul>
     <al-button variant="outline" click={App.Logout}>Log out</al-button>
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
app/Invites/Index.alder:33:15: warning[W0002]: hole ?accept has type Task Nav
    Accept p -> ?accept,
                ^^^^^^^
app/Invites/Index.alder:46:36: warning[W0002]: hole ?acceptId has type Invite.InviteId
      <al-button click={Accept { id: ?acceptId }}>Accept</al-button>
                                     ^^^^^^^^^
app/Invites/Index.alder:65:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder new msg Invites.Index Decline id:Invite.InviteId
edited app/Invites/Index.alder
--- a/app/Invites/Index.alder
+++ b/app/Invites/Index.alder
@@ -28,9 +28,12 @@
 
 msg Accept { id: Invite.InviteId }
 
+msg Decline { id: Invite.InviteId }
+
 update : Msg -> Model -> Session -> Task Nav
 update msg model session = match msg {
   Accept p -> ?accept,
+  Decline p -> ?decline,
 }
 
 view : Model -> Session -> Html Msg
@@ -44,6 +47,8 @@
     <al-button variant="outline" click={App.Logout}>Log out</al-button>
 
     <al-button click={Accept { id: ?acceptId }}>Accept</al-button>
+
+    <al-button click={Decline { id: ?declineId }}>Decline</al-button>
   </Layout.Page>
 
 test "a visitor is sent to log in" = {
@@ -64,3 +69,17 @@
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
app/Invites/Index.alder:35:15: warning[W0002]: hole ?accept has type Task Nav
    Accept p -> ?accept,
                ^^^^^^^
app/Invites/Index.alder:36:16: warning[W0002]: hole ?decline has type Task Nav
    Decline p -> ?decline,
                 ^^^^^^^^
app/Invites/Index.alder:49:36: warning[W0002]: hole ?acceptId has type Invite.InviteId
      <al-button click={Accept { id: ?acceptId }}>Accept</al-button>
                                     ^^^^^^^^^
app/Invites/Index.alder:51:37: warning[W0002]: hole ?declineId has type Invite.InviteId
      <al-button click={Decline { id: ?declineId }}>Decline</al-button>
                                      ^^^^^^^^^^
app/Invites/Index.alder:70:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Invites/Index.alder:84:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill --decl Invites.Index.view - <<'EOF'
<Layout.Page title="Invites">
  <h1>Invites</h1>
  <ul id="invites">
    {for w in model.waiting ->
      <li>
        <span class="team">{w.team.name}</span>
        <al-button variant="secondary" click={Accept { id: w.id }}>Accept</al-button>
        <al-button variant="ghost" click={Decline { id: w.id }}>Decline</al-button>
      </li>}
  </ul>
  <al-button variant="outline" click={App.Logout}>Log out</al-button>
</Layout.Page>
EOF
edited app/Invites/Index.alder
--- a/app/Invites/Index.alder
+++ b/app/Invites/Index.alder
@@ -42,13 +42,13 @@
     <h1>Invites</h1>
     <ul id="invites">
       {for w in model.waiting ->
-        <li><span class="team">{w.team.name}</span></li>}
+        <li>
+          <span class="team">{w.team.name}</span>
+          <al-button variant="secondary" click={Accept { id: w.id }}>Accept</al-button>
+          <al-button variant="ghost" click={Decline { id: w.id }}>Decline</al-button>
+        </li>}
     </ul>
     <al-button variant="outline" click={App.Logout}>Log out</al-button>
-
-    <al-button click={Accept { id: ?acceptId }}>Accept</al-button>
-
-    <al-button click={Decline { id: ?declineId }}>Decline</al-button>
   </Layout.Page>
 
 test "a visitor is sent to log in" = {
app/Invites/Index.alder:35:15: warning[W0002]: hole ?accept has type Task Nav
    Accept p -> ?accept,
                ^^^^^^^
app/Invites/Index.alder:36:16: warning[W0002]: hole ?decline has type Task Nav
    Decline p -> ?decline,
                 ^^^^^^^^
app/Invites/Index.alder:70:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Invites/Index.alder:84:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill --decl Invites.Index.update - <<'EOF'
match msg {
  Accept p -> {
    let found <- Db.get Invite p.id
    let me <- Db.get User session.user
    match found {
      Nothing -> Task.notFound,
      Just inv -> {
        if not (Invite.answerable inv me) then Task.notFound else {
          let _ <- Db.update Invite inv.id (\x -> { x | status: Accepted })
          let owned <- Db.from Team |> Db.where (Team.id.eq inv.team) |> Db.where (Team.owner.eq session.user) |> Db.count
          if owned > 0 then Task.succeed (Redirect (Teams.Show { id: inv.team })) else {
            let _ <- Db.insert Member { team: inv.team, user: session.user }
            Task.succeed (Redirect (Teams.Show { id: inv.team }))
          }
        }
      },
    }
  },
  Decline p -> {
    let found <- Db.get Invite p.id
    let me <- Db.get User session.user
    match found {
      Nothing -> Task.notFound,
      Just inv -> {
        if not (Invite.answerable inv me) then Task.notFound else {
          let _ <- Db.update Invite inv.id (\x -> { x | status: Declined })
          Task.succeed (Redirect (Invites.Index {}))
        }
      },
    }
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
 
@@ -32,8 +33,36 @@
 
 update : Msg -> Model -> Session -> Task Nav
 update msg model session = match msg {
-  Accept p -> ?accept,
-  Decline p -> ?decline,
+  Accept p -> {
+    let found <- Db.get Invite p.id
+    let me <- Db.get User session.user
+    match found {
+      Nothing -> Task.notFound,
+      Just inv -> {
+        if not (Invite.answerable inv me) then Task.notFound else {
+          let _ <- Db.update Invite inv.id (\x -> { x | status: Accepted })
+          let owned <- Db.from Team |> Db.where (Team.id.eq inv.team) |> Db.where (Team.owner.eq session.user) |> Db.count
+          if owned > 0 then Task.succeed (Redirect (Teams.Show { id: inv.team })) else {
+            let _ <- Db.insert Member { team: inv.team, user: session.user }
+            Task.succeed (Redirect (Teams.Show { id: inv.team }))
+          }
+        }
+      },
+    }
+  },
+  Decline p -> {
+    let found <- Db.get Invite p.id
+    let me <- Db.get User session.user
+    match found {
+      Nothing -> Task.notFound,
+      Just inv -> {
+        if not (Invite.answerable inv me) then Task.notFound else {
+          let _ <- Db.update Invite inv.id (\x -> { x | status: Declined })
+          Task.succeed (Redirect (Invites.Index {}))
+        }
+      },
+    }
+  },
 }
 
 view : Model -> Session -> Html Msg
app/Invites/Index.alder:99:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Invites/Index.alder:113:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
added import Member to app/Invites/Index.alder
[exit 0]
$ alder new test Teams.Show "a stranger is not found"
edited app/Teams/Show.alder
--- a/app/Teams/Show.alder
+++ b/app/Teams/Show.alder
@@ -108,3 +108,7 @@
   let r <- Test.submitRaw NewInvite [{ name: "email", value: "" }]
   Task.succeed (r.answer == FormRejected [{ field: "email", message: "must be an email address" }])
 }
+
+test "a stranger is not found" = {
+  ?test
+}
app/Teams/Show.alder:91:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Teams/Show.alder:100:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Teams/Show.alder:113:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Teams.Show."a stranger is not found"' - <<'EOF'
{
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owen", passwordHash: "" }
  let stranger <- Db.insert User { email: Text.Email "stranger@example.com", name: Text.NonEmpty "Sam", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Red", owner: owner }
  let _ <- Test.login { user: Just stranger }
  let page <- Test.open (Teams.Show { id: team })
  Task.succeed (page.answer == PageMissing)
}
EOF
edited app/Teams/Show.alder
--- a/app/Teams/Show.alder
+++ b/app/Teams/Show.alder
@@ -110,5 +110,10 @@
 }
 
 test "a stranger is not found" = {
-  ?test
+  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owen", passwordHash: "" }
+  let stranger <- Db.insert User { email: Text.Email "stranger@example.com", name: Text.NonEmpty "Sam", passwordHash: "" }
+  let team <- Db.insert Team { name: Text.NonEmpty "Red", owner: owner }
+  let _ <- Test.login { user: Just stranger }
+  let page <- Test.open (Teams.Show { id: team })
+  Task.succeed (page.answer == PageMissing)
 }
app/Teams/Show.alder:91:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Teams/Show.alder:100:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder new test Teams.Show "a member cannot remove another member"
edited app/Teams/Show.alder
--- a/app/Teams/Show.alder
+++ b/app/Teams/Show.alder
@@ -117,3 +117,7 @@
   let page <- Test.open (Teams.Show { id: team })
   Task.succeed (page.answer == PageMissing)
 }
+
+test "a member cannot remove another member" = {
+  ?test
+}
app/Teams/Show.alder:91:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Teams/Show.alder:100:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Teams/Show.alder:122:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Teams.Show."a member cannot remove another member"' - <<'EOF'
{
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owen", passwordHash: "" }
  let a <- Db.insert User { email: Text.Email "a@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let b <- Db.insert User { email: Text.Email "b@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Red", owner: owner }
  let _ <- Db.insert Member { team: team, user: a }
  let _ <- Db.insert Member { team: team, user: b }
  let _ <- Test.login { user: Just owner }
  let _ <- Test.get (Teams.Show { id: team })
  let _ <- Test.login { user: Just a }
  let r <- Test.click (Remove { user: b })
  let left <- Db.from Member |> Db.where (Member.team.eq team) |> Db.count
  Task.succeed ({ answer: r.answer, left: left } == { answer: PageMissing, left: 2 })
}
EOF
edited app/Teams/Show.alder
--- a/app/Teams/Show.alder
+++ b/app/Teams/Show.alder
@@ -119,5 +119,16 @@
 }
 
 test "a member cannot remove another member" = {
-  ?test
+  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owen", passwordHash: "" }
+  let a <- Db.insert User { email: Text.Email "a@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
+  let b <- Db.insert User { email: Text.Email "b@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
+  let team <- Db.insert Team { name: Text.NonEmpty "Red", owner: owner }
+  let _ <- Db.insert Member { team: team, user: a }
+  let _ <- Db.insert Member { team: team, user: b }
+  let _ <- Test.login { user: Just owner }
+  let _ <- Test.get (Teams.Show { id: team })
+  let _ <- Test.login { user: Just a }
+  let r <- Test.click (Remove { user: b })
+  let left <- Db.from Member |> Db.where (Member.team.eq team) |> Db.count
+  Task.succeed ({ answer: r.answer, left: left } == { answer: PageMissing, left: 2 })
 }
app/Teams/Show.alder:91:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Teams/Show.alder:100:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder new test Teams.Show "the owner removes a member"
edited app/Teams/Show.alder
--- a/app/Teams/Show.alder
+++ b/app/Teams/Show.alder
@@ -132,3 +132,7 @@
   let left <- Db.from Member |> Db.where (Member.team.eq team) |> Db.count
   Task.succeed ({ answer: r.answer, left: left } == { answer: PageMissing, left: 2 })
 }
+
+test "the owner removes a member" = {
+  ?test
+}
app/Teams/Show.alder:91:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Teams/Show.alder:100:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Teams/Show.alder:137:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Teams.Show."the owner removes a member"' - <<'EOF'
{
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owen", passwordHash: "" }
  let a <- Db.insert User { email: Text.Email "a@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let b <- Db.insert User { email: Text.Email "b@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Red", owner: owner }
  let _ <- Db.insert Member { team: team, user: a }
  let _ <- Db.insert Member { team: team, user: b }
  let _ <- Test.login { user: Just owner }
  let _ <- Test.get (Teams.Show { id: team })
  let r <- Test.click (Remove { user: a })
  let left <- Db.from Member |> Db.where (Member.team.eq team) |> Db.count
  Task.succeed ({ answer: r.answer, left: left } == { answer: Redirected (Teams.Show { id: team }), left: 1 })
}
EOF
edited app/Teams/Show.alder
--- a/app/Teams/Show.alder
+++ b/app/Teams/Show.alder
@@ -134,5 +134,15 @@
 }
 
 test "the owner removes a member" = {
-  ?test
+  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owen", passwordHash: "" }
+  let a <- Db.insert User { email: Text.Email "a@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
+  let b <- Db.insert User { email: Text.Email "b@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
+  let team <- Db.insert Team { name: Text.NonEmpty "Red", owner: owner }
+  let _ <- Db.insert Member { team: team, user: a }
+  let _ <- Db.insert Member { team: team, user: b }
+  let _ <- Test.login { user: Just owner }
+  let _ <- Test.get (Teams.Show { id: team })
+  let r <- Test.click (Remove { user: a })
+  let left <- Db.from Member |> Db.where (Member.team.eq team) |> Db.count
+  Task.succeed ({ answer: r.answer, left: left } == { answer: Redirected (Teams.Show { id: team }), left: 1 })
 }
app/Teams/Show.alder:91:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Teams/Show.alder:100:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder new test Teams.Show "the owner cannot remove themselves"
edited app/Teams/Show.alder
--- a/app/Teams/Show.alder
+++ b/app/Teams/Show.alder
@@ -146,3 +146,7 @@
   let left <- Db.from Member |> Db.where (Member.team.eq team) |> Db.count
   Task.succeed ({ answer: r.answer, left: left } == { answer: Redirected (Teams.Show { id: team }), left: 1 })
 }
+
+test "the owner cannot remove themselves" = {
+  ?test
+}
app/Teams/Show.alder:91:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Teams/Show.alder:100:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Teams/Show.alder:151:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Teams.Show."the owner cannot remove themselves"' - <<'EOF'
{
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owen", passwordHash: "" }
  let a <- Db.insert User { email: Text.Email "a@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Red", owner: owner }
  let _ <- Db.insert Member { team: team, user: a }
  let _ <- Test.login { user: Just owner }
  let page <- Test.get (Teams.Show { id: team })
  let r <- Test.run (update (Remove { user: owner }) page.model { user: owner })
  let left <- Db.from Member |> Db.where (Member.team.eq team) |> Db.count
  Task.succeed ({ value: r.value, left: left } == { value: Err "not found", left: 1 })
}
EOF
edited app/Teams/Show.alder
--- a/app/Teams/Show.alder
+++ b/app/Teams/Show.alder
@@ -148,5 +148,13 @@
 }
 
 test "the owner cannot remove themselves" = {
-  ?test
+  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owen", passwordHash: "" }
+  let a <- Db.insert User { email: Text.Email "a@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
+  let team <- Db.insert Team { name: Text.NonEmpty "Red", owner: owner }
+  let _ <- Db.insert Member { team: team, user: a }
+  let _ <- Test.login { user: Just owner }
+  let page <- Test.get (Teams.Show { id: team })
+  let r <- Test.run (update (Remove { user: owner }) page.model { user: owner })
+  let left <- Db.from Member |> Db.where (Member.team.eq team) |> Db.count
+  Task.succeed ({ value: r.value, left: left } == { value: Err "not found", left: 1 })
 }
app/Teams/Show.alder:91:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Teams/Show.alder:100:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder new test Teams.Show "a member cannot invite"
edited app/Teams/Show.alder
--- a/app/Teams/Show.alder
+++ b/app/Teams/Show.alder
@@ -158,3 +158,7 @@
   let left <- Db.from Member |> Db.where (Member.team.eq team) |> Db.count
   Task.succeed ({ value: r.value, left: left } == { value: Err "not found", left: 1 })
 }
+
+test "a member cannot invite" = {
+  ?test
+}
app/Teams/Show.alder:91:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Teams/Show.alder:100:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Teams/Show.alder:163:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Teams.Show."a member cannot invite"' - <<'EOF'
{
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owen", passwordHash: "" }
  let a <- Db.insert User { email: Text.Email "a@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Red", owner: owner }
  let _ <- Db.insert Member { team: team, user: a }
  let _ <- Test.login { user: Just owner }
  let _ <- Test.get (Teams.Show { id: team })
  let _ <- Test.login { user: Just a }
  let r <- Test.submit NewInvite { email: Text.Email "new@example.com" }
  let invites <- Db.from Invite |> Db.count
  Task.succeed ({ answer: r.answer, invites: invites } == { answer: PageMissing, invites: 0 })
}
EOF
edited app/Teams/Show.alder
--- a/app/Teams/Show.alder
+++ b/app/Teams/Show.alder
@@ -160,5 +160,14 @@
 }
 
 test "a member cannot invite" = {
-  ?test
+  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owen", passwordHash: "" }
+  let a <- Db.insert User { email: Text.Email "a@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
+  let team <- Db.insert Team { name: Text.NonEmpty "Red", owner: owner }
+  let _ <- Db.insert Member { team: team, user: a }
+  let _ <- Test.login { user: Just owner }
+  let _ <- Test.get (Teams.Show { id: team })
+  let _ <- Test.login { user: Just a }
+  let r <- Test.submit NewInvite { email: Text.Email "new@example.com" }
+  let invites <- Db.from Invite |> Db.count
+  Task.succeed ({ answer: r.answer, invites: invites } == { answer: PageMissing, invites: 0 })
 }
app/Teams/Show.alder:91:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Teams/Show.alder:100:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder new test Teams.Show "the owner invites by email"
edited app/Teams/Show.alder
--- a/app/Teams/Show.alder
+++ b/app/Teams/Show.alder
@@ -171,3 +171,7 @@
   let invites <- Db.from Invite |> Db.count
   Task.succeed ({ answer: r.answer, invites: invites } == { answer: PageMissing, invites: 0 })
 }
+
+test "the owner invites by email" = {
+  ?test
+}
app/Teams/Show.alder:91:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Teams/Show.alder:100:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Teams/Show.alder:176:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Teams.Show."the owner invites by email"' - <<'EOF'
{
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owen", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Red", owner: owner }
  let _ <- Test.login { user: Just owner }
  let _ <- Test.get (Teams.Show { id: team })
  let r <- Test.submit NewInvite { email: Text.Email "new@example.com" }
  let waiting <- Db.from Invite |> Db.where (Invite.email.eq (Text.Email "new@example.com")) |> Db.count
  Task.succeed ({ answer: r.answer, waiting: waiting } == { answer: Redirected (Teams.Show { id: team }), waiting: 1 })
}
EOF
edited app/Teams/Show.alder
--- a/app/Teams/Show.alder
+++ b/app/Teams/Show.alder
@@ -173,5 +173,11 @@
 }
 
 test "the owner invites by email" = {
-  ?test
+  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owen", passwordHash: "" }
+  let team <- Db.insert Team { name: Text.NonEmpty "Red", owner: owner }
+  let _ <- Test.login { user: Just owner }
+  let _ <- Test.get (Teams.Show { id: team })
+  let r <- Test.submit NewInvite { email: Text.Email "new@example.com" }
+  let waiting <- Db.from Invite |> Db.where (Invite.email.eq (Text.Email "new@example.com")) |> Db.count
+  Task.succeed ({ answer: r.answer, waiting: waiting } == { answer: Redirected (Teams.Show { id: team }), waiting: 1 })
 }
app/Teams/Show.alder:91:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Teams/Show.alder:100:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder new test Invites.Index "accepting an invite makes the team visible"
edited app/Invites/Index.alder
--- a/app/Invites/Index.alder
+++ b/app/Invites/Index.alder
@@ -112,3 +112,7 @@
   let after <- Db.get Invite invite1
   Task.succeed ?expected
 }
+
+test "accepting an invite makes the team visible" = {
+  ?test
+}
app/Invites/Index.alder:99:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Invites/Index.alder:113:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Invites/Index.alder:117:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Invites.Index."accepting an invite makes the team visible"' - <<'EOF'
{
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owen", passwordHash: "" }
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Red", owner: owner }
  let invite <- Db.insert Invite { team: team, email: Text.Email "me@example.com" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Invites.Index {})
  let r <- Test.click (Accept { id: invite })
  let page <- Test.get (Teams.Index {})
  let show <- Test.open (Teams.Show { id: team })
  Task.succeed ({ answer: r.answer, listed: Test.texts "#teams .name" page.doc, show: show.answer } == { answer: Redirected (Teams.Show { id: team }), listed: ["Red"], show: Shown })
}
EOF
edited app/Invites/Index.alder
--- a/app/Invites/Index.alder
+++ b/app/Invites/Index.alder
@@ -114,5 +114,14 @@
 }
 
 test "accepting an invite makes the team visible" = {
-  ?test
+  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owen", passwordHash: "" }
+  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
+  let team <- Db.insert Team { name: Text.NonEmpty "Red", owner: owner }
+  let invite <- Db.insert Invite { team: team, email: Text.Email "me@example.com" }
+  let _ <- Test.login { user: Just me }
+  let _ <- Test.get (Invites.Index {})
+  let r <- Test.click (Accept { id: invite })
+  let page <- Test.get (Teams.Index {})
+  let show <- Test.open (Teams.Show { id: team })
+  Task.succeed ({ answer: r.answer, listed: Test.texts "#teams .name" page.doc, show: show.answer } == { answer: Redirected (Teams.Show { id: team }), listed: ["Red"], show: Shown })
 }
app/Invites/Index.alder:99:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Invites/Index.alder:113:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder new test Invites.Index "declining an invite keeps the team closed"
edited app/Invites/Index.alder
--- a/app/Invites/Index.alder
+++ b/app/Invites/Index.alder
@@ -125,3 +125,7 @@
   let show <- Test.open (Teams.Show { id: team })
   Task.succeed ({ answer: r.answer, listed: Test.texts "#teams .name" page.doc, show: show.answer } == { answer: Redirected (Teams.Show { id: team }), listed: ["Red"], show: Shown })
 }
+
+test "declining an invite keeps the team closed" = {
+  ?test
+}
app/Invites/Index.alder:99:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Invites/Index.alder:113:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Invites/Index.alder:130:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Invites.Index."declining an invite keeps the team closed"' - <<'EOF'
{
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owen", passwordHash: "" }
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Red", owner: owner }
  let invite <- Db.insert Invite { team: team, email: Text.Email "me@example.com" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Invites.Index {})
  let r <- Test.click (Decline { id: invite })
  let page <- Test.get (Teams.Index {})
  let show <- Test.open (Teams.Show { id: team })
  Task.succeed ({ answer: r.answer, waiting: Test.count "#invites li" r.doc, listed: Test.count "#teams li" page.doc, show: show.answer } == { answer: Redirected (Invites.Index {}), waiting: 0, listed: 0, show: PageMissing })
}
EOF
edited app/Invites/Index.alder
--- a/app/Invites/Index.alder
+++ b/app/Invites/Index.alder
@@ -127,5 +127,14 @@
 }
 
 test "declining an invite keeps the team closed" = {
-  ?test
+  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owen", passwordHash: "" }
+  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
+  let team <- Db.insert Team { name: Text.NonEmpty "Red", owner: owner }
+  let invite <- Db.insert Invite { team: team, email: Text.Email "me@example.com" }
+  let _ <- Test.login { user: Just me }
+  let _ <- Test.get (Invites.Index {})
+  let r <- Test.click (Decline { id: invite })
+  let page <- Test.get (Teams.Index {})
+  let show <- Test.open (Teams.Show { id: team })
+  Task.succeed ({ answer: r.answer, waiting: Test.count "#invites li" r.doc, listed: Test.count "#teams li" page.doc, show: show.answer } == { answer: Redirected (Invites.Index {}), waiting: 0, listed: 0, show: PageMissing })
 }
app/Invites/Index.alder:99:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Invites/Index.alder:113:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder new test Invites.Index "a stranger cannot accept an invite"
edited app/Invites/Index.alder
--- a/app/Invites/Index.alder
+++ b/app/Invites/Index.alder
@@ -138,3 +138,7 @@
   let show <- Test.open (Teams.Show { id: team })
   Task.succeed ({ answer: r.answer, waiting: Test.count "#invites li" r.doc, listed: Test.count "#teams li" page.doc, show: show.answer } == { answer: Redirected (Invites.Index {}), waiting: 0, listed: 0, show: PageMissing })
 }
+
+test "a stranger cannot accept an invite" = {
+  ?test
+}
app/Invites/Index.alder:99:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Invites/Index.alder:113:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Invites/Index.alder:143:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Invites.Index."a stranger cannot accept an invite"' - <<'EOF'
{
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owen", passwordHash: "" }
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let stranger <- Db.insert User { email: Text.Email "stranger@example.com", name: Text.NonEmpty "Sam", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Red", owner: owner }
  let invite <- Db.insert Invite { team: team, email: Text.Email "me@example.com" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Invites.Index {})
  let _ <- Test.login { user: Just stranger }
  let r <- Test.click (Accept { id: invite })
  let members <- Db.from Member |> Db.where (Member.team.eq team) |> Db.count
  Task.succeed ({ answer: r.answer, members: members } == { answer: PageMissing, members: 0 })
}
EOF
edited app/Invites/Index.alder
--- a/app/Invites/Index.alder
+++ b/app/Invites/Index.alder
@@ -140,5 +140,15 @@
 }
 
 test "a stranger cannot accept an invite" = {
-  ?test
+  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owen", passwordHash: "" }
+  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
+  let stranger <- Db.insert User { email: Text.Email "stranger@example.com", name: Text.NonEmpty "Sam", passwordHash: "" }
+  let team <- Db.insert Team { name: Text.NonEmpty "Red", owner: owner }
+  let invite <- Db.insert Invite { team: team, email: Text.Email "me@example.com" }
+  let _ <- Test.login { user: Just me }
+  let _ <- Test.get (Invites.Index {})
+  let _ <- Test.login { user: Just stranger }
+  let r <- Test.click (Accept { id: invite })
+  let members <- Db.from Member |> Db.where (Member.team.eq team) |> Db.count
+  Task.succeed ({ answer: r.answer, members: members } == { answer: PageMissing, members: 0 })
 }
app/Invites/Index.alder:99:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Invites/Index.alder:113:16: warning[W0002]: hole ?expected has type Bool
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
```

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Auth/Login.alder:86:85: warning[W0002]: hole ?afterLogin has type Page
    Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
                                                                                      ^^^^^^^^^^^
app/Invites/Index.alder:99:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Invites/Index.alder:113:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Teams/Index.alder:65:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Teams/Show.alder:91:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Teams/Show.alder:100:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
alder build: the program has 6 typed hole(s); fill them before building
```
