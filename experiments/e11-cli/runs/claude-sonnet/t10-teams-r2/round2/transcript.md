### Round 2

```text
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
$ alder fill Teams.Index.load - <<'EOF'
{
  let owned <- Db.from Team |> Db.where (Team.owner.eq session.user) |> Db.all
  let mems <- Db.from Member |> Db.where (Member.user.eq session.user) |> Db.all
  let joined <- Db.from Team |> Db.where (Team.id.in (List.map (\m -> m.team) mems)) |> Db.all
  Query.succeed { teams: List.sortByText (\t -> t.name) (owned ++ joined) }
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
@@ -7,7 +8,12 @@
 type Model = { teams: List Team }
 
 load : Params -> Session -> Query Model
-load params session = ?load
+load params session = {
+  let owned <- Db.from Team |> Db.where (Team.owner.eq session.user) |> Db.all
+  let mems <- Db.from Member |> Db.where (Member.user.eq session.user) |> Db.all
+  let joined <- Db.from Team |> Db.where (Team.id.in (List.map (\m -> m.team) mems)) |> Db.all
+  Query.succeed { teams: List.sortByText (\t -> t.name) (owned ++ joined) }
+}
 
 view : Model -> Session -> Html Msg
 view model session = ?view
app/Teams/Index.alder:19:22: warning[W0002]: hole ?view has type Html Teams.Index.Msg
  view model session = ?view
                       ^^^^^
added import Member to app/Teams/Index.alder
[exit 0]
$ alder new decl Teams.Index viewBody "Model -> Session -> Html Msg"
edited app/Teams/Index.alder
--- a/app/Teams/Index.alder
+++ b/app/Teams/Index.alder
@@ -18,6 +18,9 @@
 view : Model -> Session -> Html Msg
 view model session = ?view
 
+viewBody : Model -> Session -> Html Msg
+viewBody = ?viewBody
+
 test "a visitor is sent to log in" = {
   let r <- Test.open (Teams.Index {})
   Task.succeed (r.answer == Redirected (Auth.Login {}))
app/Teams/Index.alder:19:22: warning[W0002]: hole ?view has type Html Teams.Index.Msg
  view model session = ?view
                       ^^^^^
app/Teams/Index.alder:22:12: warning[W0002]: hole ?viewBody has type Teams.Index.Model -> Teams.Index.Session -> Html Teams.Index.Msg
  viewBody = ?viewBody
             ^^^^^^^^^
[exit 0]
$ alder fill Teams.Index.view - <<'EOF'
viewBody model session
EOF
edited app/Teams/Index.alder
--- a/app/Teams/Index.alder
+++ b/app/Teams/Index.alder
@@ -16,7 +16,7 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = ?view
+view model session = viewBody model session
 
 viewBody : Model -> Session -> Html Msg
 viewBody = ?viewBody
app/Teams/Index.alder:22:12: warning[W0002]: hole ?viewBody has type Teams.Index.Model -> Teams.Index.Session -> Html Teams.Index.Msg
  viewBody = ?viewBody
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
  match found {
    Nothing -> Query.notFound,
    Just team -> {
      let rows <- Db.from Member |> Db.where (Member.team.eq team.id) |> Db.all
      let ownerRow <- Db.get User team.owner
      let users <- Db.from User |> Db.where (User.id.in (List.map (\m -> m.user) rows)) |> Db.all
      let isMember = List.any (\m -> m.user == session.user) rows
      if team.owner != session.user && not isMember then Query.notFound else
        match ownerRow {
          Nothing -> Query.notFound,
          Just o -> Query.succeed { team: team, owner: o, members: List.sortByText (\u -> u.name) users, isOwner: team.owner == session.user },
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
 
@@ -8,7 +9,23 @@
 type Model = { team: Team, owner: User, members: List User, isOwner: Bool }
 
 load : Params -> Session -> Query Model
-load params session = ?load
+load params session = {
+  let found <- Db.get Team params.id
+  match found {
+    Nothing -> Query.notFound,
+    Just team -> {
+      let rows <- Db.from Member |> Db.where (Member.team.eq team.id) |> Db.all
+      let ownerRow <- Db.get User team.owner
+      let users <- Db.from User |> Db.where (User.id.in (List.map (\m -> m.user) rows)) |> Db.all
+      let isMember = List.any (\m -> m.user == session.user) rows
+      if team.owner != session.user && not isMember then Query.notFound else
+        match ownerRow {
+          Nothing -> Query.notFound,
+          Just o -> Query.succeed { team: team, owner: o, members: List.sortByText (\u -> u.name) users, isOwner: team.owner == session.user },
+        }
+    },
+  }
+}
 
 view : Model -> Session -> Html Msg
 view model session = ?view
app/Teams/Show.alder:31:22: warning[W0002]: hole ?view has type Html Teams.Show.Msg
  view model session = ?view
                       ^^^^^
added import Member to app/Teams/Show.alder
[exit 0]
$ alder new decl Teams.Show viewBody "Model -> Session -> Html Msg"
edited app/Teams/Show.alder
--- a/app/Teams/Show.alder
+++ b/app/Teams/Show.alder
@@ -30,6 +30,9 @@
 view : Model -> Session -> Html Msg
 view model session = ?view
 
+viewBody : Model -> Session -> Html Msg
+viewBody = ?viewBody
+
 test "a visitor is sent to log in" = {
   let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
   let team1 <- Db.insert Team { name: Text.NonEmpty "name-1", owner: me }
app/Teams/Show.alder:31:22: warning[W0002]: hole ?view has type Html Teams.Show.Msg
  view model session = ?view
                       ^^^^^
app/Teams/Show.alder:34:12: warning[W0002]: hole ?viewBody has type Teams.Show.Model -> Teams.Show.Session -> Html Teams.Show.Msg
  viewBody = ?viewBody
             ^^^^^^^^^
[exit 0]
$ alder fill Teams.Show.view - <<'EOF'
viewBody model session
EOF
edited app/Teams/Show.alder
--- a/app/Teams/Show.alder
+++ b/app/Teams/Show.alder
@@ -28,7 +28,7 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = ?view
+view model session = viewBody model session
 
 viewBody : Model -> Session -> Html Msg
 viewBody = ?viewBody
app/Teams/Show.alder:34:12: warning[W0002]: hole ?viewBody has type Teams.Show.Model -> Teams.Show.Session -> Html Teams.Show.Msg
  viewBody = ?viewBody
             ^^^^^^^^^
[exit 0]
$ alder fill --decl Invites.Index.Model - <<'EOF'
{ items: List { id: Invite.InviteId, team: Text } }
EOF
edited app/Invites/Index.alder
--- a/app/Invites/Index.alder
+++ b/app/Invites/Index.alder
@@ -1,8 +1,10 @@
 module Invites.Index
 
+import Invite
+
 page "/invites" { } requires user
 
-type Model = { }
+type Model = { items: List { id: Invite.InviteId, team: Text } }
 
 load : Params -> Session -> Query Model
 load params session = ?load
app/Invites/Index.alder:10:23: warning[W0002]: hole ?load has type Query Invites.Index.Model
  load params session = ?load
                        ^^^^^
app/Invites/Index.alder:13:22: warning[W0002]: hole ?view has type Html Invites.Index.Msg
  view model session = ?view
                       ^^^^^
added import Invite to app/Invites/Index.alder
[exit 0]
$ alder fill Invites.Index.load - <<'EOF'
{
  let me <- Db.get User session.user
  match me {
    Nothing -> Query.succeed { items: [] },
    Just u -> {
      let invs <- Db.from Invite |> Db.where (Invite.email.eq u.email) |> Db.where (Invite.status.eq Pending) |> Db.all
      let teams <- Db.from Team |> Db.where (Team.id.in (List.map (\i -> i.team) invs)) |> Db.all
      Query.succeed { items: List.filterMap (\i -> Maybe.map (\t -> { id: i.id, team: t.name }) (List.find (\t -> t.id == i.team) teams)) invs }
    },
  }
}
EOF
edited app/Invites/Index.alder
--- a/app/Invites/Index.alder
+++ b/app/Invites/Index.alder
@@ -1,13 +1,25 @@
 module Invites.Index
 
 import Invite
+import Team
+import User
 
 page "/invites" { } requires user
 
 type Model = { items: List { id: Invite.InviteId, team: Text } }
 
 load : Params -> Session -> Query Model
-load params session = ?load
+load params session = {
+  let me <- Db.get User session.user
+  match me {
+    Nothing -> Query.succeed { items: [] },
+    Just u -> {
+      let invs <- Db.from Invite |> Db.where (Invite.email.eq u.email) |> Db.where (Invite.status.eq Pending) |> Db.all
+      let teams <- Db.from Team |> Db.where (Team.id.in (List.map (\i -> i.team) invs)) |> Db.all
+      Query.succeed { items: List.filterMap (\i -> Maybe.map (\t -> { id: i.id, team: t.name }) (List.find (\t -> t.id == i.team) teams)) invs }
+    },
+  }
+}
 
 view : Model -> Session -> Html Msg
 view model session = ?view
app/Invites/Index.alder:25:22: warning[W0002]: hole ?view has type Html Invites.Index.Msg
  view model session = ?view
                       ^^^^^
added import User to app/Invites/Index.alder
added import Team to app/Invites/Index.alder
[exit 0]
$ alder new decl Invites.Index viewBody "Model -> Session -> Html Msg"
edited app/Invites/Index.alder
--- a/app/Invites/Index.alder
+++ b/app/Invites/Index.alder
@@ -24,6 +24,9 @@
 view : Model -> Session -> Html Msg
 view model session = ?view
 
+viewBody : Model -> Session -> Html Msg
+viewBody = ?viewBody
+
 test "a visitor is sent to log in" = {
   let r <- Test.open (Invites.Index {})
   Task.succeed (r.answer == Redirected (Auth.Login {}))
app/Invites/Index.alder:25:22: warning[W0002]: hole ?view has type Html Invites.Index.Msg
  view model session = ?view
                       ^^^^^
app/Invites/Index.alder:28:12: warning[W0002]: hole ?viewBody has type Invites.Index.Model -> Invites.Index.Session -> Html Invites.Index.Msg
  viewBody = ?viewBody
             ^^^^^^^^^
[exit 0]
$ alder fill Invites.Index.view - <<'EOF'
viewBody model session
EOF
edited app/Invites/Index.alder
--- a/app/Invites/Index.alder
+++ b/app/Invites/Index.alder
@@ -22,7 +22,7 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = ?view
+view model session = viewBody model session
 
 viewBody : Model -> Session -> Html Msg
 viewBody = ?viewBody
app/Invites/Index.alder:28:12: warning[W0002]: hole ?viewBody has type Invites.Index.Model -> Invites.Index.Session -> Html Invites.Index.Msg
  viewBody = ?viewBody
             ^^^^^^^^^
[exit 0]
$ alder new form Teams.Index NewTeam name:Text.NonEmpty --msg Create
alder new form: `Teams.Index`'s `view` is not one element over several lines (write it as `<Layout.Page …>` … `</Layout.Page>` on several lines, with `alder fill --decl Teams.Index.view -`), or add the markup yourself
[exit 1]
```

The harness stopped at line 74 of your script: `alder new form Teams.Index NewTeam name:Text.NonEmpty --msg Create`: it exited 1. The commands after it were not run.

Your script stopped at command 14; the app is as the commands before it left it; continue.

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Invites/Index.alder:28:12: warning[W0002]: hole ?viewBody has type Invites.Index.Model -> Invites.Index.Session -> Html Invites.Index.Msg
  viewBody = ?viewBody
             ^^^^^^^^^
app/Teams/Index.alder:22:12: warning[W0002]: hole ?viewBody has type Teams.Index.Model -> Teams.Index.Session -> Html Teams.Index.Msg
  viewBody = ?viewBody
             ^^^^^^^^^
app/Teams/Show.alder:34:12: warning[W0002]: hole ?viewBody has type Teams.Show.Model -> Teams.Show.Session -> Html Teams.Show.Msg
  viewBody = ?viewBody
             ^^^^^^^^^
alder build: the program has 3 typed hole(s); fill them before building
```
