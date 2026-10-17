### Round 3

```text
$ alder fill --decl Teams.Index.view - <<'EOF'
<Layout.Page title="Teams">
  <h1>Teams</h1>
  {viewBody model session}
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
 
@@ -16,7 +17,11 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = viewBody model session
+view model session =
+  <Layout.Page title="Teams">
+    <h1>Teams</h1>
+    {viewBody model session}
+  </Layout.Page>
 
 viewBody : Model -> Session -> Html Msg
 viewBody = ?viewBody
app/Teams/Index.alder:27:12: warning[W0002]: hole ?viewBody has type Teams.Index.Model -> Teams.Index.Session -> Html Teams.Index.Msg
  viewBody = ?viewBody
             ^^^^^^^^^
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
 
@@ -16,11 +17,31 @@
   Query.succeed { teams: List.sortByText (\t -> t.name) (owned ++ joined) }
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
     {viewBody model session}
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
 
 viewBody : Model -> Session -> Html Msg
@@ -30,3 +51,19 @@
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
app/Teams/Index.alder:48:12: warning[W0002]: hole ?viewBody has type Teams.Index.Model -> Teams.Index.Session -> Html Teams.Index.Msg
  viewBody = ?viewBody
             ^^^^^^^^^
app/Teams/Index.alder:60:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder fill Teams.Index.viewBody - <<'EOF'
\model session ->
  <ul id="teams">
    {for t in model.teams -> <li><a class="name" href={Teams.Show { id: t.id }}>{t.name}</a></li>}
  </ul>
EOF
edited app/Teams/Index.alder
--- a/app/Teams/Index.alder
+++ b/app/Teams/Index.alder
@@ -45,7 +45,10 @@
   </Layout.Page>
 
 viewBody : Model -> Session -> Html Msg
-viewBody = ?viewBody
+viewBody = \model session ->
+  <ul id="teams">
+    {for t in model.teams -> <li><a class="name" href={Teams.Show { id: t.id }}>{t.name}</a></li>}
+  </ul>
 
 test "a visitor is sent to log in" = {
   let r <- Test.open (Teams.Index {})
app/Teams/Index.alder:28:15: warning[W0002]: hole ?create has type Task Nav
    Create p -> ?create,
                ^^^^^^^
app/Teams/Index.alder:63:29: warning[W0002]: hole ?expected has type Answer
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
app/Teams/Index.alder:66:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder fill --decl Teams.Show.view - <<'EOF'
<Layout.Page title={model.team.name}>
  <h1 id="name">{model.team.name}</h1>
  {viewBody model session}
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
@@ -28,7 +29,11 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = viewBody model session
+view model session =
+  <Layout.Page title={model.team.name}>
+    <h1 id="name">{model.team.name}</h1>
+    {viewBody model session}
+  </Layout.Page>
 
 viewBody : Model -> Session -> Html Msg
 viewBody = ?viewBody
app/Teams/Show.alder:39:12: warning[W0002]: hole ?viewBody has type Teams.Show.Model -> Teams.Show.Session -> Html Teams.Show.Msg
  viewBody = ?viewBody
             ^^^^^^^^^
added import Layout to app/Teams/Show.alder
[exit 0]
$ alder new form Teams.Show NewInvite email:Text.Email --msg SendInvite
edited app/Teams/Show.alder
--- a/app/Teams/Show.alder
+++ b/app/Teams/Show.alder
@@ -28,11 +28,31 @@
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
   <Layout.Page title={model.team.name}>
     <h1 id="name">{model.team.name}</h1>
     {viewBody model session}
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
 
 viewBody : Model -> Session -> Html Msg
@@ -44,3 +64,21 @@
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
app/Teams/Show.alder:39:19: warning[W0002]: hole ?sendInvite has type Task Nav
    SendInvite p -> ?sendInvite,
                    ^^^^^^^^^^^
app/Teams/Show.alder:59:12: warning[W0002]: hole ?viewBody has type Teams.Show.Model -> Teams.Show.Session -> Html Teams.Show.Msg
  viewBody = ?viewBody
             ^^^^^^^^^
app/Teams/Show.alder:74:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder new msg Teams.Show Remove user:User.UserId
edited app/Teams/Show.alder
--- a/app/Teams/Show.alder
+++ b/app/Teams/Show.alder
@@ -34,9 +34,12 @@
 
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
 
 viewBody : Model -> Session -> Html Msg
@@ -82,3 +87,16 @@
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
app/Teams/Show.alder:41:19: warning[W0002]: hole ?sendInvite has type Task Nav
    SendInvite p -> ?sendInvite,
                    ^^^^^^^^^^^
app/Teams/Show.alder:42:15: warning[W0002]: hole ?remove has type Task Nav
    Remove p -> ?remove,
                ^^^^^^^
app/Teams/Show.alder:60:38: warning[W0002]: hole ?removeUser has type User.UserId
      <al-button click={Remove { user: ?removeUser }}>Remove</al-button>
                                       ^^^^^^^^^^^
app/Teams/Show.alder:64:12: warning[W0002]: hole ?viewBody has type Teams.Show.Model -> Teams.Show.Session -> Html Teams.Show.Msg
  viewBody = ?viewBody
             ^^^^^^^^^
app/Teams/Show.alder:79:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Teams/Show.alder:101:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill Teams.Show.viewBody - <<'EOF'
\model session ->
  <div>
    <ul id="members">
      <li><span class="name">{model.owner.name}</span></li>
      {for u in model.members ->
        <li>
          <span class="name">{u.name}</span>
          {if model.isOwner then <al-button class="remove" click={Remove { user: u.id }}>Remove</al-button> else Html.none}
        </li>}
    </ul>
    {if model.isOwner then
      <form submit={SendInvite}>
        <Errors form />
        <al-input field="email" label="Invite by email" />
        <al-button type="submit">Invite</al-button>
      </form>
    else Html.none}
  </div>
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
@@ -61,7 +62,24 @@
   </Layout.Page>
 
 viewBody : Model -> Session -> Html Msg
-viewBody = ?viewBody
+viewBody = \model session ->
+  <div>
+    <ul id="members">
+      <li><span class="name">{model.owner.name}</span></li>
+      {for u in model.members ->
+        <li>
+          <span class="name">{u.name}</span>
+          {if model.isOwner then <al-button class="remove" click={Remove { user: u.id }}>Remove</al-button> else Html.none}
+        </li>}
+    </ul>
+    {if model.isOwner then
+      <form submit={SendInvite}>
+        <Errors form />
+        <al-input field="email" label="Invite by email" />
+        <al-button type="submit">Invite</al-button>
+      </form>
+    else Html.none}
+  </div>
 
 test "a visitor is sent to log in" = {
   let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
app/Teams/Show.alder:3:1: warning[W0001]: import `Invite` is unused
  import Invite
  ^^^^^^^^^^^^^
app/Teams/Show.alder:42:19: warning[W0002]: hole ?sendInvite has type Task Nav
    SendInvite p -> ?sendInvite,
                    ^^^^^^^^^^^
app/Teams/Show.alder:43:15: warning[W0002]: hole ?remove has type Task Nav
    Remove p -> ?remove,
                ^^^^^^^
app/Teams/Show.alder:61:38: warning[W0002]: hole ?removeUser has type User.UserId
      <al-button click={Remove { user: ?removeUser }}>Remove</al-button>
                                       ^^^^^^^^^^^
app/Teams/Show.alder:97:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Teams/Show.alder:119:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
added import Invite to app/Teams/Show.alder
[exit 0]
$ alder fill --decl Teams.Show.view - <<'EOF'
<Layout.Page title={model.team.name}>
  <h1 id="name">{model.team.name}</h1>
  {viewBody model session}
</Layout.Page>
EOF
edited app/Teams/Show.alder
--- a/app/Teams/Show.alder
+++ b/app/Teams/Show.alder
@@ -48,17 +48,6 @@
   <Layout.Page title={model.team.name}>
     <h1 id="name">{model.team.name}</h1>
     {viewBody model session}
-
-    <al-card>
-      <h2 slot="header">New invite</h2>
-      <form submit={SendInvite}>
-        <al-input field="email" label="Email" />
-        <al-alert variant="destructive"><Errors form /></al-alert>
-        <al-button type="submit">New invite</al-button>
-      </form>
-    </al-card>
-
-    <al-button click={Remove { user: ?removeUser }}>Remove</al-button>
   </Layout.Page>
 
 viewBody : Model -> Session -> Html Msg
app/Teams/Show.alder:3:1: warning[W0001]: import `Invite` is unused
  import Invite
  ^^^^^^^^^^^^^
app/Teams/Show.alder:42:19: warning[W0002]: hole ?sendInvite has type Task Nav
    SendInvite p -> ?sendInvite,
                    ^^^^^^^^^^^
app/Teams/Show.alder:43:15: warning[W0002]: hole ?remove has type Task Nav
    Remove p -> ?remove,
                ^^^^^^^
app/Teams/Show.alder:86:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Teams/Show.alder:108:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
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
@@ -39,7 +39,10 @@
 
 update : Msg -> Model -> Session -> Task Nav
 update msg model session = match msg {
-  SendInvite p -> ?sendInvite,
+  SendInvite p -> if model.team.owner != session.user then Task.notFound else {
+    let _ <- Db.insert Invite { team: model.team.id, email: p.form.email }
+    Task.succeed (Redirect (Teams.Show { id: model.team.id }))
+  },
   Remove p -> ?remove,
 }
 
app/Teams/Show.alder:46:15: warning[W0002]: hole ?remove has type Task Nav
    Remove p -> ?remove,
                ^^^^^^^
app/Teams/Show.alder:89:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Teams/Show.alder:111:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill Teams.Show.update.Remove - <<'EOF'
if model.team.owner != session.user || p.user == model.team.owner then Task.notFound else {
  let rows <- Db.from Member |> Db.where (Member.team.eq model.team.id) |> Db.where (Member.user.eq p.user) |> Db.all
  let _ <- Task.all (List.map (\m -> Db.delete Member m.id) rows)
  Task.succeed (Redirect (Teams.Show { id: model.team.id }))
}
EOF
edited app/Teams/Show.alder
--- a/app/Teams/Show.alder
+++ b/app/Teams/Show.alder
@@ -43,7 +43,11 @@
     let _ <- Db.insert Invite { team: model.team.id, email: p.form.email }
     Task.succeed (Redirect (Teams.Show { id: model.team.id }))
   },
-  Remove p -> ?remove,
+  Remove p -> if model.team.owner != session.user || p.user == model.team.owner then Task.notFound else {
+    let rows <- Db.from Member |> Db.where (Member.team.eq model.team.id) |> Db.where (Member.user.eq p.user) |> Db.all
+    let _ <- Task.all (List.map (\m -> Db.delete Member m.id) rows)
+    Task.succeed (Redirect (Teams.Show { id: model.team.id }))
+  },
 }
 
 view : Model -> Session -> Html Msg
app/Teams/Show.alder:93:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Teams/Show.alder:115:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill --decl Invites.Index.view - <<'EOF'
<Layout.Page title="Invites">
  <h1>Invites</h1>
  {viewBody model session}
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
 
@@ -22,7 +23,11 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = viewBody model session
+view model session =
+  <Layout.Page title="Invites">
+    <h1>Invites</h1>
+    {viewBody model session}
+  </Layout.Page>
 
 viewBody : Model -> Session -> Html Msg
 viewBody = ?viewBody
app/Invites/Index.alder:33:12: warning[W0002]: hole ?viewBody has type Invites.Index.Model -> Invites.Index.Session -> Html Invites.Index.Msg
  viewBody = ?viewBody
             ^^^^^^^^^
added import Layout to app/Invites/Index.alder
[exit 0]
$ alder new msg Invites.Index Accept id:Invite.InviteId
edited app/Invites/Index.alder
--- a/app/Invites/Index.alder
+++ b/app/Invites/Index.alder
@@ -22,11 +22,20 @@
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
     {viewBody model session}
+
+    <al-button click={Accept { id: ?acceptId }}>Accept</al-button>
   </Layout.Page>
 
 viewBody : Model -> Session -> Html Msg
@@ -36,3 +45,17 @@
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
app/Invites/Index.alder:38:36: warning[W0002]: hole ?acceptId has type Invite.InviteId
      <al-button click={Accept { id: ?acceptId }}>Accept</al-button>
                                     ^^^^^^^^^
app/Invites/Index.alder:42:12: warning[W0002]: hole ?viewBody has type Invites.Index.Model -> Invites.Index.Session -> Html Invites.Index.Msg
  viewBody = ?viewBody
             ^^^^^^^^^
app/Invites/Index.alder:60:16: warning[W0002]: hole ?expected has type Bool
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
@@ -36,6 +39,8 @@
     {viewBody model session}
 
     <al-button click={Accept { id: ?acceptId }}>Accept</al-button>
+
+    <al-button click={Decline { id: ?declineId }}>Decline</al-button>
   </Layout.Page>
 
 viewBody : Model -> Session -> Html Msg
@@ -59,3 +64,17 @@
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
app/Invites/Index.alder:41:36: warning[W0002]: hole ?acceptId has type Invite.InviteId
      <al-button click={Accept { id: ?acceptId }}>Accept</al-button>
                                     ^^^^^^^^^
app/Invites/Index.alder:43:37: warning[W0002]: hole ?declineId has type Invite.InviteId
      <al-button click={Decline { id: ?declineId }}>Decline</al-button>
                                      ^^^^^^^^^^
app/Invites/Index.alder:47:12: warning[W0002]: hole ?viewBody has type Invites.Index.Model -> Invites.Index.Session -> Html Invites.Index.Msg
  viewBody = ?viewBody
             ^^^^^^^^^
app/Invites/Index.alder:65:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Invites/Index.alder:79:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill Invites.Index.viewBody - <<'EOF'
\model session ->
  <ul id="invites">
    {for i in model.items ->
      <li>
        <span class="team">{i.team}</span>
        <al-button click={Accept { id: i.id }}>Accept</al-button>
        <al-button variant="outline" click={Decline { id: i.id }}>Decline</al-button>
      </li>}
  </ul>
EOF
edited app/Invites/Index.alder
--- a/app/Invites/Index.alder
+++ b/app/Invites/Index.alder
@@ -44,7 +44,15 @@
   </Layout.Page>
 
 viewBody : Model -> Session -> Html Msg
-viewBody = ?viewBody
+viewBody = \model session ->
+  <ul id="invites">
+    {for i in model.items ->
+      <li>
+        <span class="team">{i.team}</span>
+        <al-button click={Accept { id: i.id }}>Accept</al-button>
+        <al-button variant="outline" click={Decline { id: i.id }}>Decline</al-button>
+      </li>}
+  </ul>
 
 test "a visitor is sent to log in" = {
   let r <- Test.open (Invites.Index {})
app/Invites/Index.alder:31:15: warning[W0002]: hole ?accept has type Task Nav
    Accept p -> ?accept,
                ^^^^^^^
app/Invites/Index.alder:32:16: warning[W0002]: hole ?decline has type Task Nav
    Decline p -> ?decline,
                 ^^^^^^^^
app/Invites/Index.alder:41:36: warning[W0002]: hole ?acceptId has type Invite.InviteId
      <al-button click={Accept { id: ?acceptId }}>Accept</al-button>
                                     ^^^^^^^^^
app/Invites/Index.alder:43:37: warning[W0002]: hole ?declineId has type Invite.InviteId
      <al-button click={Decline { id: ?declineId }}>Decline</al-button>
                                      ^^^^^^^^^^
app/Invites/Index.alder:73:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Invites/Index.alder:87:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill Invites.Index.update.Accept - <<'EOF'
{
  let me <- Db.get User session.user
  match me {
    Nothing -> Task.notFound,
    Just u -> {
      let found <- Db.get Invite p.id
      match found {
        Nothing -> Task.notFound,
        Just i ->
          if i.email != u.email || i.status != Pending then Task.notFound else {
            let existing <- Db.from Member |> Db.where (Member.team.eq i.team) |> Db.where (Member.user.eq session.user) |> Db.count
            let t <- Db.get Team i.team
            let isOwner = Maybe.map (\x -> x.owner == session.user) t == Just True
            let _ <- Db.update Invite i.id (\x -> { x | status: Accepted })
            let _ <- if existing == 0 && not isOwner then Task.map (\x -> ()) (Db.insert Member { team: i.team, user: session.user }) else Task.succeed ()
            Task.succeed (Redirect (Teams.Show { id: i.team }))
          },
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
 
@@ -28,7 +29,27 @@
 
 update : Msg -> Model -> Session -> Task Nav
 update msg model session = match msg {
-  Accept p -> ?accept,
+  Accept p -> {
+    let me <- Db.get User session.user
+    match me {
+      Nothing -> Task.notFound,
+      Just u -> {
+        let found <- Db.get Invite p.id
+        match found {
+          Nothing -> Task.notFound,
+          Just i ->
+            if i.email != u.email || i.status != Pending then Task.notFound else {
+              let existing <- Db.from Member |> Db.where (Member.team.eq i.team) |> Db.where (Member.user.eq session.user) |> Db.count
+              let t <- Db.get Team i.team
+              let isOwner = Maybe.map (\x -> x.owner == session.user) t == Just True
+              let _ <- Db.update Invite i.id (\x -> { x | status: Accepted })
+              let _ <- if existing == 0 && not isOwner then Task.map (\x -> ()) (Db.insert Member { team: i.team, user: session.user }) else Task.succeed ()
+              Task.succeed (Redirect (Teams.Show { id: i.team }))
+            },
+        }
+      },
+    }
+  },
   Decline p -> ?decline,
 }
 
app/Invites/Index.alder:53:16: warning[W0002]: hole ?decline has type Task Nav
    Decline p -> ?decline,
                 ^^^^^^^^
app/Invites/Index.alder:62:36: warning[W0002]: hole ?acceptId has type Invite.InviteId
      <al-button click={Accept { id: ?acceptId }}>Accept</al-button>
                                     ^^^^^^^^^
app/Invites/Index.alder:64:37: warning[W0002]: hole ?declineId has type Invite.InviteId
      <al-button click={Decline { id: ?declineId }}>Decline</al-button>
                                      ^^^^^^^^^^
app/Invites/Index.alder:94:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Invites/Index.alder:108:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
added import Member to app/Invites/Index.alder
[exit 0]
$ alder fill Invites.Index.update.Decline - <<'EOF'
{
  let me <- Db.get User session.user
  match me {
    Nothing -> Task.notFound,
    Just u -> {
      let found <- Db.get Invite p.id
      match found {
        Nothing -> Task.notFound,
        Just i ->
          if i.email != u.email || i.status != Pending then Task.notFound else {
            let _ <- Db.update Invite i.id (\x -> { x | status: Declined })
            Task.succeed (Redirect (Invites.Index {}))
          },
      }
    },
  }
}
EOF
edited app/Invites/Index.alder
--- a/app/Invites/Index.alder
+++ b/app/Invites/Index.alder
@@ -50,7 +50,23 @@
       },
     }
   },
-  Decline p -> ?decline,
+  Decline p -> {
+    let me <- Db.get User session.user
+    match me {
+      Nothing -> Task.notFound,
+      Just u -> {
+        let found <- Db.get Invite p.id
+        match found {
+          Nothing -> Task.notFound,
+          Just i ->
+            if i.email != u.email || i.status != Pending then Task.notFound else {
+              let _ <- Db.update Invite i.id (\x -> { x | status: Declined })
+              Task.succeed (Redirect (Invites.Index {}))
+            },
+        }
+      },
+    }
+  },
 }
 
 view : Model -> Session -> Html Msg
app/Invites/Index.alder:78:36: warning[W0002]: hole ?acceptId has type Invite.InviteId
      <al-button click={Accept { id: ?acceptId }}>Accept</al-button>
                                     ^^^^^^^^^
app/Invites/Index.alder:80:37: warning[W0002]: hole ?declineId has type Invite.InviteId
      <al-button click={Decline { id: ?declineId }}>Decline</al-button>
                                      ^^^^^^^^^^
app/Invites/Index.alder:110:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Invites/Index.alder:124:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill --list
Invites.Index view ?acceptId : Invite.InviteId
Invites.Index view ?declineId : Invite.InviteId
Invites.Index "pressing Accept" ?expected : Bool
Invites.Index "pressing Decline" ?expected : Bool
Teams.Index "submitting NewTeam" ?expected : Answer
Teams.Show "submitting NewInvite" ?expected : Answer
Teams.Show "pressing Remove" ?expected : Bool
[exit 0]
$ alder check
app/Invites/Index.alder:78:36: warning[W0002]: hole ?acceptId has type Invite.InviteId
      <al-button click={Accept { id: ?acceptId }}>Accept</al-button>
                                     ^^^^^^^^^
app/Invites/Index.alder:80:37: warning[W0002]: hole ?declineId has type Invite.InviteId
      <al-button click={Decline { id: ?declineId }}>Decline</al-button>
                                      ^^^^^^^^^^
app/Invites/Index.alder:110:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Invites/Index.alder:124:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Teams/Index.alder:66:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Teams/Show.alder:93:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Teams/Show.alder:115:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
alder check: 7 typed holes left
[exit 0]
```

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Invites/Index.alder:78:36: warning[W0002]: hole ?acceptId has type Invite.InviteId
      <al-button click={Accept { id: ?acceptId }}>Accept</al-button>
                                     ^^^^^^^^^
app/Invites/Index.alder:80:37: warning[W0002]: hole ?declineId has type Invite.InviteId
      <al-button click={Decline { id: ?declineId }}>Decline</al-button>
                                      ^^^^^^^^^^
app/Invites/Index.alder:110:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Invites/Index.alder:124:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Teams/Index.alder:66:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Teams/Show.alder:93:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Teams/Show.alder:115:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
alder build: the program has 7 typed hole(s); fill them before building
```
