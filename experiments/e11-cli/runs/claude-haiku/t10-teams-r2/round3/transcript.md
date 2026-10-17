### Round 3

```text
$ alder new decl User add "Text.Email -> Text.NonEmpty -> Task UserId"
edited app/User.alder
--- a/app/User.alder
+++ b/app/User.alder
@@ -9,3 +9,6 @@
   passwordHash: Text,
   created: Time @created,
 }
+
+add : Text.Email -> Text.NonEmpty -> Task UserId
+add = ?add
app/User.alder:14:7: warning[W0002]: hole ?add has type Text.Email -> Text.NonEmpty -> Task User.UserId
  add = ?add
        ^^^^
[exit 0]
$ alder fill --decl User.add - <<'EOF'
\email name -> Db.insert User { email: email, name: name, passwordHash: "" }
EOF
edited app/User.alder
--- a/app/User.alder
+++ b/app/User.alder
@@ -11,4 +11,4 @@
 }
 
 add : Text.Email -> Text.NonEmpty -> Task UserId
-add = ?add
+add = \email name -> Db.insert User { email: email, name: name, passwordHash: "" }
[exit 0]
$ alder new decl Team make "User.UserId -> Text.NonEmpty -> Task TeamId"
edited app/Team.alder
--- a/app/Team.alder
+++ b/app/Team.alder
@@ -10,6 +10,9 @@
   owner: User.UserId @ref User,
 }
 
+make : User.UserId -> Text.NonEmpty -> Task TeamId
+make = ?make
+
 test "a Team keeps its owner from being deleted" = {
   let target <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
   let _ <- Db.insert Team { name: Text.NonEmpty "name-1", owner: target }
app/Team.alder:14:8: warning[W0002]: hole ?make has type User.UserId -> Text.NonEmpty -> Task Team.TeamId
  make = ?make
         ^^^^^
[exit 0]
$ alder fill --decl Team.make - <<'EOF'
\owner name -> Db.insert Team { name: name, owner: owner }
EOF
edited app/Team.alder
--- a/app/Team.alder
+++ b/app/Team.alder
@@ -11,7 +11,7 @@
 }
 
 make : User.UserId -> Text.NonEmpty -> Task TeamId
-make = ?make
+make = \owner name -> Db.insert Team { name: name, owner: owner }
 
 test "a Team keeps its owner from being deleted" = {
   let target <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
[exit 0]
$ alder new type Member "Person = { user: User.UserId, name: Text }"
edited app/Member.alder
--- a/app/Member.alder
+++ b/app/Member.alder
@@ -5,6 +5,8 @@
 
 type MemberId = brand Text
 
+type Person = { user: User.UserId, name: Text }
+
 table Member {
   id: MemberId @id,
   team: Team.TeamId @ref Team,
[exit 0]
$ alder new type Member "Roster = { name: Text.NonEmpty, owner: User.UserId, people: List Person }"
edited app/Member.alder
--- a/app/Member.alder
+++ b/app/Member.alder
@@ -7,6 +7,8 @@
 
 type Person = { user: User.UserId, name: Text }
 
+type Roster = { name: Text.NonEmpty, owner: User.UserId, people: List Person }
+
 table Member {
   id: MemberId @id,
   team: Team.TeamId @ref Team,
[exit 0]
$ alder new decl Member person "User -> Person"
edited app/Member.alder
--- a/app/Member.alder
+++ b/app/Member.alder
@@ -15,6 +15,9 @@
   user: User.UserId @ref User,
 }
 
+person : User -> Person
+person = ?person
+
 test "a Member keeps its team from being deleted" = {
   let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
   let target <- Db.insert Team { name: Text.NonEmpty "name-1", owner: me }
app/Member.alder:19:10: warning[W0002]: hole ?person has type User -> Member.Person
  person = ?person
           ^^^^^^^
[exit 0]
$ alder fill --decl Member.person - <<'EOF'
\u -> { user: u.id, name: u.name }
EOF
edited app/Member.alder
--- a/app/Member.alder
+++ b/app/Member.alder
@@ -16,7 +16,7 @@
 }
 
 person : User -> Person
-person = ?person
+person = \u -> { user: u.id, name: u.name }
 
 test "a Member keeps its team from being deleted" = {
   let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
[exit 0]
$ alder new decl Member teamsFor "User.UserId -> Query (List Team)"
edited app/Member.alder
--- a/app/Member.alder
+++ b/app/Member.alder
@@ -18,6 +18,9 @@
 person : User -> Person
 person = \u -> { user: u.id, name: u.name }
 
+teamsFor : User.UserId -> Query (List Team)
+teamsFor = ?teamsFor
+
 test "a Member keeps its team from being deleted" = {
   let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
   let target <- Db.insert Team { name: Text.NonEmpty "name-1", owner: me }
app/Member.alder:22:12: warning[W0002]: hole ?teamsFor has type User.UserId -> Query (List Team)
  teamsFor = ?teamsFor
             ^^^^^^^^^
[exit 0]
$ alder fill --decl Member.teamsFor - <<'EOF'
\user -> {
  let owned <- Db.from Team |> Db.where (Team.owner.eq user) |> Db.all
  let memberships <- Db.from Member |> Db.where (Member.user.eq user) |> Db.all
  let joined <- memberships |> List.map (\m -> Db.get Team m.team) |> Query.all
  Query.succeed (owned ++ List.filterMap (\x -> x) joined)
}
EOF
edited app/Member.alder
--- a/app/Member.alder
+++ b/app/Member.alder
@@ -19,7 +19,12 @@
 person = \u -> { user: u.id, name: u.name }
 
 teamsFor : User.UserId -> Query (List Team)
-teamsFor = ?teamsFor
+teamsFor = \user -> {
+  let owned <- Db.from Team |> Db.where (Team.owner.eq user) |> Db.all
+  let memberships <- Db.from Member |> Db.where (Member.user.eq user) |> Db.all
+  let joined <- memberships |> List.map (\m -> Db.get Team m.team) |> Query.all
+  Query.succeed (owned ++ List.filterMap (\x -> x) joined)
+}
 
 test "a Member keeps its team from being deleted" = {
   let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
[exit 0]
$ alder new decl Member roster "Team.TeamId -> User.UserId -> Query (Maybe Roster)"
edited app/Member.alder
--- a/app/Member.alder
+++ b/app/Member.alder
@@ -26,6 +26,9 @@
   Query.succeed (owned ++ List.filterMap (\x -> x) joined)
 }
 
+roster : Team.TeamId -> User.UserId -> Query (Maybe Roster)
+roster = ?roster
+
 test "a Member keeps its team from being deleted" = {
   let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
   let target <- Db.insert Team { name: Text.NonEmpty "name-1", owner: me }
app/Member.alder:30:10: warning[W0002]: hole ?roster has type Team.TeamId -> User.UserId -> Query (Maybe Member.Roster)
  roster = ?roster
           ^^^^^^^
[exit 0]
$ alder fill --decl Member.roster - <<'EOF'
\team user -> {
  let found <- Db.get Team team
  let mine <- Db.from Member |> Db.where (Member.team.eq team) |> Db.where (Member.user.eq user) |> Db.first
  let ownerRow <- match found {
    Just t -> Db.get User t.owner,
    Nothing -> Query.succeed Nothing,
  }
  let memberRows <- Db.from Member |> Db.where (Member.team.eq team) |> Db.all
  let others <- memberRows |> List.map (\m -> Db.get User m.user) |> Query.all
  let ownerPeople : List Person = [ownerRow] |> List.filterMap (\x -> x) |> List.map person
  let memberPeople : List Person = others |> List.filterMap (\x -> x) |> List.map person |> List.sortByText (\p -> p.name)
  let people : List Person = ownerPeople ++ memberPeople
  let ok = Maybe.withDefault False (Maybe.map (\t -> t.owner == user) found) || Maybe.isJust mine
  match found {
    Just t -> {
      let roster : Roster = { name: t.name, owner: t.owner, people: people }
      Query.succeed (if ok then Just roster else Nothing)
    },
    Nothing -> Query.succeed Nothing,
  }
}
EOF
edited app/Member.alder
--- a/app/Member.alder
+++ b/app/Member.alder
@@ -27,7 +27,27 @@
 }
 
 roster : Team.TeamId -> User.UserId -> Query (Maybe Roster)
-roster = ?roster
+roster = \team user -> {
+  let found <- Db.get Team team
+  let mine <- Db.from Member |> Db.where (Member.team.eq team) |> Db.where (Member.user.eq user) |> Db.first
+  let ownerRow <- match found {
+    Just t -> Db.get User t.owner,
+    Nothing -> Query.succeed Nothing,
+  }
+  let memberRows <- Db.from Member |> Db.where (Member.team.eq team) |> Db.all
+  let others <- memberRows |> List.map (\m -> Db.get User m.user) |> Query.all
+  let ownerPeople : List Person = [ownerRow] |> List.filterMap (\x -> x) |> List.map person
+  let memberPeople : List Person = others |> List.filterMap (\x -> x) |> List.map person |> List.sortByText (\p -> p.name)
+  let people : List Person = ownerPeople ++ memberPeople
+  let ok = Maybe.withDefault False (Maybe.map (\t -> t.owner == user) found) || Maybe.isJust mine
+  match found {
+    Just t -> {
+      let roster : Roster = { name: t.name, owner: t.owner, people: people }
+      Query.succeed (if ok then Just roster else Nothing)
+    },
+    Nothing -> Query.succeed Nothing,
+  }
+}
 
 test "a Member keeps its team from being deleted" = {
   let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
[exit 0]
$ alder new decl Member add "Team.TeamId -> User.UserId -> Task Unit"
edited app/Member.alder
--- a/app/Member.alder
+++ b/app/Member.alder
@@ -49,6 +49,9 @@
   }
 }
 
+add : Team.TeamId -> User.UserId -> Task Unit
+add = ?add
+
 test "a Member keeps its team from being deleted" = {
   let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
   let target <- Db.insert Team { name: Text.NonEmpty "name-1", owner: me }
app/Member.alder:53:7: warning[W0002]: hole ?add has type Team.TeamId -> User.UserId -> Task Unit
  add = ?add
        ^^^^
[exit 0]
$ alder fill --decl Member.add - <<'EOF'
\team user -> {
  let _ <- Db.insert Member { team: team, user: user }
  Task.succeed ()
}
EOF
edited app/Member.alder
--- a/app/Member.alder
+++ b/app/Member.alder
@@ -50,7 +50,10 @@
 }
 
 add : Team.TeamId -> User.UserId -> Task Unit
-add = ?add
+add = \team user -> {
+  let _ <- Db.insert Member { team: team, user: user }
+  Task.succeed ()
+}
 
 test "a Member keeps its team from being deleted" = {
   let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
[exit 0]
$ alder new decl Member removeMember "Team.TeamId -> User.UserId -> Task Unit"
edited app/Member.alder
--- a/app/Member.alder
+++ b/app/Member.alder
@@ -55,6 +55,9 @@
   Task.succeed ()
 }
 
+removeMember : Team.TeamId -> User.UserId -> Task Unit
+removeMember = ?removeMember
+
 test "a Member keeps its team from being deleted" = {
   let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
   let target <- Db.insert Team { name: Text.NonEmpty "name-1", owner: me }
app/Member.alder:59:16: warning[W0002]: hole ?removeMember has type Team.TeamId -> User.UserId -> Task Unit
  removeMember = ?removeMember
                 ^^^^^^^^^^^^^
[exit 0]
$ alder fill --decl Member.removeMember - <<'EOF'
\team user -> {
  let rows <- Db.from Member |> Db.where (Member.team.eq team) |> Db.where (Member.user.eq user) |> Db.all
  let gone = rows |> List.map (\m -> Db.delete Member m.id)
  let _ <- Task.all gone
  Task.succeed ()
}
EOF
edited app/Member.alder
--- a/app/Member.alder
+++ b/app/Member.alder
@@ -56,7 +56,12 @@
 }
 
 removeMember : Team.TeamId -> User.UserId -> Task Unit
-removeMember = ?removeMember
+removeMember = \team user -> {
+  let rows <- Db.from Member |> Db.where (Member.team.eq team) |> Db.where (Member.user.eq user) |> Db.all
+  let gone = rows |> List.map (\m -> Db.delete Member m.id)
+  let _ <- Task.all gone
+  Task.succeed ()
+}
 
 test "a Member keeps its team from being deleted" = {
   let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
[exit 0]
$ alder new type Invite "Waiting = { id: InviteId, team: Text }"
edited app/Invite.alder
--- a/app/Invite.alder
+++ b/app/Invite.alder
@@ -7,6 +7,8 @@
 
 type InviteId = brand Text
 
+type Waiting = { id: InviteId, team: Text }
+
 table Invite {
   id: InviteId @id,
   team: Team.TeamId @ref Team,
[exit 0]
$ alder new decl Invite pendingOf "User.UserId -> InviteId -> Query (Maybe Invite)"
edited app/Invite.alder
--- a/app/Invite.alder
+++ b/app/Invite.alder
@@ -16,6 +16,9 @@
   status: Status = Pending,
 }
 
+pendingOf : User.UserId -> InviteId -> Query (Maybe Invite)
+pendingOf = ?pendingOf
+
 test "a Invite keeps its team from being deleted" = {
   let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
   let target <- Db.insert Team { name: Text.NonEmpty "name-1", owner: me }
app/Invite.alder:20:13: warning[W0002]: hole ?pendingOf has type User.UserId -> Invite.InviteId -> Query (Maybe Invite)
  pendingOf = ?pendingOf
              ^^^^^^^^^^
[exit 0]
$ alder fill --decl Invite.pendingOf - <<'EOF'
\user id -> {
  let me <- Db.get User user
  let found <- match me {
    Just u -> Db.from Invite |> Db.where (Invite.id.eq id) |> Db.where (Invite.email.eq u.email) |> Db.where (Invite.status.eq Pending) |> Db.first,
    Nothing -> Query.succeed Nothing,
  }
  Query.succeed found
}
EOF
edited app/Invite.alder
--- a/app/Invite.alder
+++ b/app/Invite.alder
@@ -17,7 +17,14 @@
 }
 
 pendingOf : User.UserId -> InviteId -> Query (Maybe Invite)
-pendingOf = ?pendingOf
+pendingOf = \user id -> {
+  let me <- Db.get User user
+  let found <- match me {
+    Just u -> Db.from Invite |> Db.where (Invite.id.eq id) |> Db.where (Invite.email.eq u.email) |> Db.where (Invite.status.eq Pending) |> Db.first,
+    Nothing -> Query.succeed Nothing,
+  }
+  Query.succeed found
+}
 
 test "a Invite keeps its team from being deleted" = {
   let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
[exit 0]
$ alder new decl Invite waitingOne "Invite -> Query (Maybe Waiting)"
edited app/Invite.alder
--- a/app/Invite.alder
+++ b/app/Invite.alder
@@ -26,6 +26,9 @@
   Query.succeed found
 }
 
+waitingOne : Invite -> Query (Maybe Waiting)
+waitingOne = ?waitingOne
+
 test "a Invite keeps its team from being deleted" = {
   let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
   let target <- Db.insert Team { name: Text.NonEmpty "name-1", owner: me }
app/Invite.alder:30:14: warning[W0002]: hole ?waitingOne has type Invite -> Query (Maybe Invite.Waiting)
  waitingOne = ?waitingOne
               ^^^^^^^^^^^
[exit 0]
$ alder fill --decl Invite.waitingOne - <<'EOF'
\inv -> {
  let found <- Db.get Team inv.team
  match found {
    Just t -> {
      let w : Waiting = { id: inv.id, team: t.name }
      Query.succeed (Just w)
    },
    Nothing -> Query.succeed Nothing,
  }
}
EOF
edited app/Invite.alder
--- a/app/Invite.alder
+++ b/app/Invite.alder
@@ -27,7 +27,16 @@
 }
 
 waitingOne : Invite -> Query (Maybe Waiting)
-waitingOne = ?waitingOne
+waitingOne = \inv -> {
+  let found <- Db.get Team inv.team
+  match found {
+    Just t -> {
+      let w : Waiting = { id: inv.id, team: t.name }
+      Query.succeed (Just w)
+    },
+    Nothing -> Query.succeed Nothing,
+  }
+}
 
 test "a Invite keeps its team from being deleted" = {
   let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
[exit 0]
$ alder new decl Invite waitingFor "User.UserId -> Query (List Waiting)"
edited app/Invite.alder
--- a/app/Invite.alder
+++ b/app/Invite.alder
@@ -38,6 +38,9 @@
   }
 }
 
+waitingFor : User.UserId -> Query (List Waiting)
+waitingFor = ?waitingFor
+
 test "a Invite keeps its team from being deleted" = {
   let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
   let target <- Db.insert Team { name: Text.NonEmpty "name-1", owner: me }
app/Invite.alder:42:14: warning[W0002]: hole ?waitingFor has type User.UserId -> Query (List Invite.Waiting)
  waitingFor = ?waitingFor
               ^^^^^^^^^^^
[exit 0]
$ alder fill --decl Invite.waitingFor - <<'EOF'
\user -> {
  let me <- Db.get User user
  let invites <- match me {
    Just u -> Db.from Invite |> Db.where (Invite.email.eq u.email) |> Db.where (Invite.status.eq Pending) |> Db.all,
    Nothing -> Query.succeed [],
  }
  let found <- invites |> List.map waitingOne |> Query.all
  Query.succeed (found |> List.filterMap (\x -> x))
}
EOF
edited app/Invite.alder
--- a/app/Invite.alder
+++ b/app/Invite.alder
@@ -39,7 +39,15 @@
 }
 
 waitingFor : User.UserId -> Query (List Waiting)
-waitingFor = ?waitingFor
+waitingFor = \user -> {
+  let me <- Db.get User user
+  let invites <- match me {
+    Just u -> Db.from Invite |> Db.where (Invite.email.eq u.email) |> Db.where (Invite.status.eq Pending) |> Db.all,
+    Nothing -> Query.succeed [],
+  }
+  let found <- invites |> List.map waitingOne |> Query.all
+  Query.succeed (found |> List.filterMap (\x -> x))
+}
 
 test "a Invite keeps its team from being deleted" = {
   let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
[exit 0]
$ alder new decl Invite send "Team.TeamId -> Text.Email -> Task InviteId"
edited app/Invite.alder
--- a/app/Invite.alder
+++ b/app/Invite.alder
@@ -49,6 +49,9 @@
   Query.succeed (found |> List.filterMap (\x -> x))
 }
 
+send : Team.TeamId -> Text.Email -> Task InviteId
+send = ?send
+
 test "a Invite keeps its team from being deleted" = {
   let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
   let target <- Db.insert Team { name: Text.NonEmpty "name-1", owner: me }
app/Invite.alder:53:8: warning[W0002]: hole ?send has type Team.TeamId -> Text.Email -> Task Invite.InviteId
  send = ?send
         ^^^^^
[exit 0]
$ alder fill --decl Invite.send - <<'EOF'
\team email -> Db.insert Invite { team: team, email: email }
EOF
edited app/Invite.alder
--- a/app/Invite.alder
+++ b/app/Invite.alder
@@ -50,7 +50,7 @@
 }
 
 send : Team.TeamId -> Text.Email -> Task InviteId
-send = ?send
+send = \team email -> Db.insert Invite { team: team, email: email }
 
 test "a Invite keeps its team from being deleted" = {
   let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
[exit 0]
$ alder new decl Invite accept "User.UserId -> InviteId -> Task Team.TeamId"
edited app/Invite.alder
--- a/app/Invite.alder
+++ b/app/Invite.alder
@@ -52,6 +52,9 @@
 send : Team.TeamId -> Text.Email -> Task InviteId
 send = \team email -> Db.insert Invite { team: team, email: email }
 
+accept : User.UserId -> InviteId -> Task Team.TeamId
+accept = ?accept
+
 test "a Invite keeps its team from being deleted" = {
   let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
   let target <- Db.insert Team { name: Text.NonEmpty "name-1", owner: me }
app/Invite.alder:56:10: warning[W0002]: hole ?accept has type User.UserId -> Invite.InviteId -> Task Team.TeamId
  accept = ?accept
           ^^^^^^^
[exit 0]
$ alder fill --decl Invite.accept - <<'EOF'
\user id -> {
  let found <- pendingOf user id
  match found {
    Nothing -> Task.notFound,
    Just inv -> {
      let _ <- Db.update Invite inv.id (\i -> { i | status: Accepted })
      let _ <- Member.add inv.team user
      Task.succeed inv.team
    },
  }
}
EOF
edited app/Invite.alder
--- a/app/Invite.alder
+++ b/app/Invite.alder
@@ -1,5 +1,6 @@
 module Invite
 
+import Member
 import Team
 import User
 
@@ -53,7 +54,17 @@
 send = \team email -> Db.insert Invite { team: team, email: email }
 
 accept : User.UserId -> InviteId -> Task Team.TeamId
-accept = ?accept
+accept = \user id -> {
+  let found <- pendingOf user id
+  match found {
+    Nothing -> Task.notFound,
+    Just inv -> {
+      let _ <- Db.update Invite inv.id (\i -> { i | status: Accepted })
+      let _ <- Member.add inv.team user
+      Task.succeed inv.team
+    },
+  }
+}
 
 test "a Invite keeps its team from being deleted" = {
   let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
added import Member to app/Invite.alder
[exit 0]
$ alder new decl Invite decline "User.UserId -> InviteId -> Task Unit"
edited app/Invite.alder
--- a/app/Invite.alder
+++ b/app/Invite.alder
@@ -66,6 +66,9 @@
   }
 }
 
+decline : User.UserId -> InviteId -> Task Unit
+decline = ?decline
+
 test "a Invite keeps its team from being deleted" = {
   let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
   let target <- Db.insert Team { name: Text.NonEmpty "name-1", owner: me }
app/Invite.alder:70:11: warning[W0002]: hole ?decline has type User.UserId -> Invite.InviteId -> Task Unit
  decline = ?decline
            ^^^^^^^^
[exit 0]
$ alder fill --decl Invite.decline - <<'EOF'
\user id -> {
  let found <- pendingOf user id
  match found {
    Nothing -> Task.notFound,
    Just inv -> {
      let _ <- Db.update Invite inv.id (\i -> { i | status: Declined })
      Task.succeed ()
    },
  }
}
EOF
edited app/Invite.alder
--- a/app/Invite.alder
+++ b/app/Invite.alder
@@ -67,7 +67,16 @@
 }
 
 decline : User.UserId -> InviteId -> Task Unit
-decline = ?decline
+decline = \user id -> {
+  let found <- pendingOf user id
+  match found {
+    Nothing -> Task.notFound,
+    Just inv -> {
+      let _ <- Db.update Invite inv.id (\i -> { i | status: Declined })
+      Task.succeed ()
+    },
+  }
+}
 
 test "a Invite keeps its team from being deleted" = {
   let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
[exit 0]
$ alder fill --decl Teams.Index.load - <<'EOF'
{
  let rows <- Member.teamsFor session.user
  Query.succeed { rows: rows |> List.sortByText (\t -> t.name), page: 1, pages: 1 }
}
EOF
edited app/Teams/Index.alder
--- a/app/Teams/Index.alder
+++ b/app/Teams/Index.alder
@@ -1,6 +1,7 @@
 module Teams.Index
 
 import Layout
+import Member
 import Team
 import User
 
@@ -18,15 +19,8 @@
 -- load reads one page of the rows, newest first, and how many pages there are.
 load : Params -> Session -> Query Model
 load params session = {
-  let page = Int.clamp { min: 1, max: lastPage } params.page
-  let rows <-
-    Db.from Team
-      |> Db.orderBy Team.id.desc
-      |> Db.limit perPage
-      |> Db.offset ((page - 1) * perPage)
-      |> Db.all
-  let count <- Db.from Team |> Db.count
-  Query.succeed { rows: rows, page: page, pages: Int.max 1 ((count + perPage - 1) / perPage) }
+  let rows <- Member.teamsFor session.user
+  Query.succeed { rows: rows |> List.sortByText (\t -> t.name), page: 1, pages: 1 }
 }
 
 view : Model -> Session -> Html Msg
added import Member to app/Teams/Index.alder
[exit 0]
$ alder new form Teams.Index NewTeam name:Text.NonEmpty --msg Create
edited app/Teams/Index.alder
--- a/app/Teams/Index.alder
+++ b/app/Teams/Index.alder
@@ -23,6 +23,17 @@
   Query.succeed { rows: rows |> List.sortByText (\t -> t.name), page: 1, pages: 1 }
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
@@ -48,6 +59,15 @@
       <span>Page {Text.fromInt model.page} of {Text.fromInt model.pages}</span>
       {if model.page < model.pages then <a slot="next" href={Teams.Index { page: model.page + 1 }} rel="next">Next</a> else <span slot="next" aria-disabled="true">Next</span>}
     </al-pagination>
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
 
 test "the list shows the rows, newest first" = {
@@ -72,3 +92,19 @@
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
app/Teams/Index.alder:34:15: warning[W0002]: hole ?create has type Task Nav
    Create p -> ?create,
                ^^^^^^^
app/Teams/Index.alder:101:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder fill --decl Teams.Index.update - <<'EOF'
match msg {
  Create p -> {
    let team <- Team.make session.user p.form.name
    Task.succeed (Redirect (Teams.Show { id: team }))
  },
}
EOF
edited app/Teams/Index.alder
--- a/app/Teams/Index.alder
+++ b/app/Teams/Index.alder
@@ -31,7 +31,10 @@
 
 update : Msg -> Model -> Session -> Task Nav
 update msg model session = match msg {
-  Create p -> ?create,
+  Create p -> {
+    let team <- Team.make session.user p.form.name
+    Task.succeed (Redirect (Teams.Show { id: team }))
+  },
 }
 
 view : Model -> Session -> Html Msg
app/Teams/Index.alder:104:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder fill --decl Teams.Index.view - <<'EOF'
<Layout.Page title="Teams">
  <h1>Teams</h1>
  <ul id="teams">
    {for team in model.rows ->
      <li><a class="name" href={Teams.Show { id: team.id }}>{team.name}</a></li>}
  </ul>
  <al-card>
    <form submit={Create}>
      <al-input field="name" label="Team name" />
      <al-button type="submit">Create team</al-button>
    </form>
  </al-card>
  <al-button variant="outline" click={App.Logout}>Log out</al-button>
</Layout.Page>
EOF
edited app/Teams/Index.alder
--- a/app/Teams/Index.alder
+++ b/app/Teams/Index.alder
@@ -41,36 +41,17 @@
 view model session =
   <Layout.Page title="Teams">
     <h1>Teams</h1>
-
-    <al-table>
-      <table>
-        <thead>
-          <tr><th>Name</th><th>Owner</th></tr>
-        </thead>
-        <tbody>
-          {for row in model.rows ->
-            <tr>
-              <td>{row.name}</td>
-              <td>{row.owner}</td>
-            </tr>}
-        </tbody>
-      </table>
-    </al-table>
-
-    <al-pagination role="navigation" aria-label="Pages">
-      {if model.page > 1 then <a slot="prev" href={Teams.Index { page: model.page - 1 }} rel="prev">Previous</a> else <span slot="prev" aria-disabled="true">Previous</span>}
-      <span>Page {Text.fromInt model.page} of {Text.fromInt model.pages}</span>
-      {if model.page < model.pages then <a slot="next" href={Teams.Index { page: model.page + 1 }} rel="next">Next</a> else <span slot="next" aria-disabled="true">Next</span>}
-    </al-pagination>
-
+    <ul id="teams">
+      {for team in model.rows ->
+        <li><a class="name" href={Teams.Show { id: team.id }}>{team.name}</a></li>}
+    </ul>
     <al-card>
-      <h2 slot="header">New team</h2>
       <form submit={Create}>
-        <al-input field="name" label="Name" />
-        <al-alert variant="destructive"><Errors form /></al-alert>
-        <al-button type="submit">New team</al-button>
+        <al-input field="name" label="Team name" />
+        <al-button type="submit">Create team</al-button>
       </form>
     </al-card>
+    <al-button variant="outline" click={App.Logout}>Log out</al-button>
   </Layout.Page>
 
 test "the list shows the rows, newest first" = {
app/Teams/Index.alder:85:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder fill --decl Teams.Show.Model - <<'EOF'
{ team: Team.TeamId, roster: Member.Roster, viewer: User.UserId }
EOF
edited app/Teams/Show.alder
--- a/app/Teams/Show.alder
+++ b/app/Teams/Show.alder
@@ -1,11 +1,12 @@
 module Teams.Show
 
+import Member
 import Team
 import User
 
 page "/teams/{id}" { id: Team.TeamId } requires user
 
-type Model = { }
+type Model = { team: Team.TeamId, roster: Member.Roster, viewer: User.UserId }
 
 load : Params -> Session -> Query Model
 load params session = ?load
app/Teams/Show.alder:12:23: warning[W0002]: hole ?load has type Query Teams.Show.Model
  load params session = ?load
                        ^^^^^
app/Teams/Show.alder:15:22: warning[W0002]: hole ?view has type Html Teams.Show.Msg
  view model session = ?view
                       ^^^^^
added import Member to app/Teams/Show.alder
[exit 0]
$ alder fill --decl Teams.Show.load - <<'EOF'
{
  let found <- Member.roster params.id session.user
  match found {
    Just r -> Query.succeed { team: params.id, roster: r, viewer: session.user },
    Nothing -> Query.notFound,
  }
}
EOF
edited app/Teams/Show.alder
--- a/app/Teams/Show.alder
+++ b/app/Teams/Show.alder
@@ -9,7 +9,13 @@
 type Model = { team: Team.TeamId, roster: Member.Roster, viewer: User.UserId }
 
 load : Params -> Session -> Query Model
-load params session = ?load
+load params session = {
+  let found <- Member.roster params.id session.user
+  match found {
+    Just r -> Query.succeed { team: params.id, roster: r, viewer: session.user },
+    Nothing -> Query.notFound,
+  }
+}
 
 view : Model -> Session -> Html Msg
 view model session = ?view
app/Teams/Show.alder:21:22: warning[W0002]: hole ?view has type Html Teams.Show.Msg
  view model session = ?view
                       ^^^^^
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
@@ -18,7 +19,10 @@
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
@@ -18,10 +18,30 @@
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
@@ -30,3 +50,21 @@
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
app/Teams/Show.alder:29:19: warning[W0002]: hole ?sendInvite has type Task Nav
    SendInvite p -> ?sendInvite,
                    ^^^^^^^^^^^
app/Teams/Show.alder:60:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder new msg Teams.Show Remove user:User.UserId
edited app/Teams/Show.alder
--- a/app/Teams/Show.alder
+++ b/app/Teams/Show.alder
@@ -24,9 +24,12 @@
 
 msg SendInvite form NewInvite
 
+msg Remove { user: User.UserId }
+
 update : Msg -> Model -> Session -> Task Nav
 update msg model session = match msg {
   SendInvite p -> ?sendInvite,
+  Remove p -> ?remove,
 }
 
 view : Model -> Session -> Html Msg
@@ -42,6 +45,8 @@
         <al-button type="submit">New invite</al-button>
       </form>
     </al-card>
+
+    <al-button click={Remove { user: ?removeUser }}>Remove</al-button>
   </Layout.Page>
 
 test "a visitor is sent to log in" = {
@@ -68,3 +73,16 @@
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
app/Teams/Show.alder:31:19: warning[W0002]: hole ?sendInvite has type Task Nav
    SendInvite p -> ?sendInvite,
                    ^^^^^^^^^^^
app/Teams/Show.alder:32:15: warning[W0002]: hole ?remove has type Task Nav
    Remove p -> ?remove,
                ^^^^^^^
app/Teams/Show.alder:49:38: warning[W0002]: hole ?removeUser has type User.UserId
      <al-button click={Remove { user: ?removeUser }}>Remove</al-button>
                                       ^^^^^^^^^^^
app/Teams/Show.alder:65:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Teams/Show.alder:87:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill --decl Teams.Show.update - <<'EOF'
match msg {
  Remove p -> {
    let ok = session.user == model.roster.owner && p.user != model.roster.owner
    let _ <- if ok then Member.removeMember model.team p.user else Task.notFound
    Task.succeed (Redirect (Teams.Show { id: model.team }))
  },
  SendInvite p -> {
    let ok = session.user == model.roster.owner
    let _ <- if ok then Invite.send model.team p.form.email else Task.notFound
    Task.succeed (Redirect (Teams.Show { id: model.team }))
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
@@ -28,8 +29,16 @@
 
 update : Msg -> Model -> Session -> Task Nav
 update msg model session = match msg {
-  SendInvite p -> ?sendInvite,
-  Remove p -> ?remove,
+  Remove p -> {
+    let ok = session.user == model.roster.owner && p.user != model.roster.owner
+    let _ <- if ok then Member.removeMember model.team p.user else Task.notFound
+    Task.succeed (Redirect (Teams.Show { id: model.team }))
+  },
+  SendInvite p -> {
+    let ok = session.user == model.roster.owner
+    let _ <- if ok then Invite.send model.team p.form.email else Task.notFound
+    Task.succeed (Redirect (Teams.Show { id: model.team }))
+  },
 }
 
 view : Model -> Session -> Html Msg
app/Teams/Show.alder:58:38: warning[W0002]: hole ?removeUser has type User.UserId
      <al-button click={Remove { user: ?removeUser }}>Remove</al-button>
                                       ^^^^^^^^^^^
app/Teams/Show.alder:74:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Teams/Show.alder:96:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
added import Invite to app/Teams/Show.alder
[exit 0]
$ alder fill --decl Teams.Show.view - <<'EOF'
<Layout.Page title={model.roster.name}>
  <h1 id="name">{model.roster.name}</h1>
  <ul id="members">
    {for p in model.roster.people ->
      <li>
        <span class="name">{p.name}</span>
        {if model.roster.owner == model.viewer && p.user != model.roster.owner then <button class="remove" click={Remove { user: p.user }}>Remove</button> else Html.none}
      </li>}
  </ul>
  {if model.roster.owner == model.viewer then <form submit={SendInvite}>
      <al-input field="email" label="Invite by email" />
      <al-button type="submit">Invite</al-button>
    </form> else Html.none}
  <a href={Teams.Index {}}>Back to teams</a>
</Layout.Page>
EOF
edited app/Teams/Show.alder
--- a/app/Teams/Show.alder
+++ b/app/Teams/Show.alder
@@ -43,19 +43,20 @@
 
 view : Model -> Session -> Html Msg
 view model session =
-  <Layout.Page title="Team">
-    <h1>Team</h1>
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
+  <Layout.Page title={model.roster.name}>
+    <h1 id="name">{model.roster.name}</h1>
+    <ul id="members">
+      {for p in model.roster.people ->
+        <li>
+          <span class="name">{p.name}</span>
+          {if model.roster.owner == model.viewer && p.user != model.roster.owner then <button class="remove" click={Remove { user: p.user }}>Remove</button> else Html.none}
+        </li>}
+    </ul>
+    {if model.roster.owner == model.viewer then <form submit={SendInvite}>
+        <al-input field="email" label="Invite by email" />
+        <al-button type="submit">Invite</al-button>
+      </form> else Html.none}
+    <a href={Teams.Index {}}>Back to teams</a>
   </Layout.Page>
 
 test "a visitor is sent to log in" = {
app/Teams/Show.alder:75:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Teams/Show.alder:97:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder remove page Invites.Index
removed app/Invites/Index.alder
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
$ alder fill --decl Invites.Index.Model - <<'EOF'
{ rows: List Invite.Waiting }
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
+type Model = { rows: List Invite.Waiting }
 
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
$ alder fill --decl Invites.Index.load - <<'EOF'
{
  let rows <- Invite.waitingFor session.user
  Query.succeed { rows: rows }
}
EOF
edited app/Invites/Index.alder
--- a/app/Invites/Index.alder
+++ b/app/Invites/Index.alder
@@ -7,7 +7,10 @@
 type Model = { rows: List Invite.Waiting }
 
 load : Params -> Session -> Query Model
-load params session = ?load
+load params session = {
+  let rows <- Invite.waitingFor session.user
+  Query.succeed { rows: rows }
+}
 
 view : Model -> Session -> Html Msg
 view model session = ?view
app/Invites/Index.alder:16:22: warning[W0002]: hole ?view has type Html Invites.Index.Msg
  view model session = ?view
                       ^^^^^
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
 
 page "/invites" { } requires user
 
@@ -13,7 +14,10 @@
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
@@ -2,6 +2,8 @@
 
 import Invite
 import Layout
+import Team
+import User
 
 page "/invites" { } requires user
 
@@ -13,13 +15,36 @@
   Query.succeed { rows: rows }
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
app/Invites/Index.alder:22:15: warning[W0002]: hole ?accept has type Task Nav
    Accept p -> ?accept,
                ^^^^^^^
app/Invites/Index.alder:30:36: warning[W0002]: hole ?acceptId has type Invite.InviteId
      <al-button click={Accept { id: ?acceptId }}>Accept</al-button>
                                     ^^^^^^^^^
app/Invites/Index.alder:49:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder new msg Invites.Index Decline id:Invite.InviteId
edited app/Invites/Index.alder
--- a/app/Invites/Index.alder
+++ b/app/Invites/Index.alder
@@ -17,9 +17,12 @@
 
 msg Accept { id: Invite.InviteId }
 
+msg Decline { id: Invite.InviteId }
+
 update : Msg -> Model -> Session -> Task Nav
 update msg model session = match msg {
   Accept p -> ?accept,
+  Decline p -> ?decline,
 }
 
 view : Model -> Session -> Html Msg
@@ -28,6 +31,8 @@
     <h1>Invites</h1>
 
     <al-button click={Accept { id: ?acceptId }}>Accept</al-button>
+
+    <al-button click={Decline { id: ?declineId }}>Decline</al-button>
   </Layout.Page>
 
 test "a visitor is sent to log in" = {
@@ -48,3 +53,17 @@
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
app/Invites/Index.alder:24:15: warning[W0002]: hole ?accept has type Task Nav
    Accept p -> ?accept,
                ^^^^^^^
app/Invites/Index.alder:25:16: warning[W0002]: hole ?decline has type Task Nav
    Decline p -> ?decline,
                 ^^^^^^^^
app/Invites/Index.alder:33:36: warning[W0002]: hole ?acceptId has type Invite.InviteId
      <al-button click={Accept { id: ?acceptId }}>Accept</al-button>
                                     ^^^^^^^^^
app/Invites/Index.alder:35:37: warning[W0002]: hole ?declineId has type Invite.InviteId
      <al-button click={Decline { id: ?declineId }}>Decline</al-button>
                                      ^^^^^^^^^^
app/Invites/Index.alder:54:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Invites/Index.alder:68:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill --decl Invites.Index.update - <<'EOF'
match msg {
  Accept p -> {
    let team <- Invite.accept session.user p.id
    Task.succeed (Redirect (Teams.Show { id: team }))
  },
  Decline p -> {
    let _ <- Invite.decline session.user p.id
    Task.succeed (Redirect (Invites.Index {}))
  },
}
EOF
edited app/Invites/Index.alder
--- a/app/Invites/Index.alder
+++ b/app/Invites/Index.alder
@@ -21,8 +21,14 @@
 
 update : Msg -> Model -> Session -> Task Nav
 update msg model session = match msg {
-  Accept p -> ?accept,
-  Decline p -> ?decline,
+  Accept p -> {
+    let team <- Invite.accept session.user p.id
+    Task.succeed (Redirect (Teams.Show { id: team }))
+  },
+  Decline p -> {
+    let _ <- Invite.decline session.user p.id
+    Task.succeed (Redirect (Invites.Index {}))
+  },
 }
 
 view : Model -> Session -> Html Msg
app/Invites/Index.alder:39:36: warning[W0002]: hole ?acceptId has type Invite.InviteId
      <al-button click={Accept { id: ?acceptId }}>Accept</al-button>
                                     ^^^^^^^^^
app/Invites/Index.alder:41:37: warning[W0002]: hole ?declineId has type Invite.InviteId
      <al-button click={Decline { id: ?declineId }}>Decline</al-button>
                                      ^^^^^^^^^^
app/Invites/Index.alder:60:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Invites/Index.alder:74:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill --decl Invites.Index.view - <<'EOF'
<Layout.Page title="Invites">
  <h1>Invites</h1>
  <ul id="invites">
    {for inv in model.rows ->
      <li>
        <span class="team">{inv.team}</span>
        <al-button click={Accept { id: inv.id }}>Accept</al-button>
        <al-button variant="ghost" click={Decline { id: inv.id }}>Decline</al-button>
      </li>}
  </ul>
</Layout.Page>
EOF
edited app/Invites/Index.alder
--- a/app/Invites/Index.alder
+++ b/app/Invites/Index.alder
@@ -35,10 +35,14 @@
 view model session =
   <Layout.Page title="Invites">
     <h1>Invites</h1>
-
-    <al-button click={Accept { id: ?acceptId }}>Accept</al-button>
-
-    <al-button click={Decline { id: ?declineId }}>Decline</al-button>
+    <ul id="invites">
+      {for inv in model.rows ->
+        <li>
+          <span class="team">{inv.team}</span>
+          <al-button click={Accept { id: inv.id }}>Accept</al-button>
+          <al-button variant="ghost" click={Decline { id: inv.id }}>Decline</al-button>
+        </li>}
+    </ul>
   </Layout.Page>
 
 test "a visitor is sent to log in" = {
app/Invites/Index.alder:64:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Invites/Index.alder:78:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder new test Teams.Show "a stranger gets a 404 for the team"
edited app/Teams/Show.alder
--- a/app/Teams/Show.alder
+++ b/app/Teams/Show.alder
@@ -96,3 +96,7 @@
   let after <- Db.get User me
   Task.succeed ?expected
 }
+
+test "a stranger gets a 404 for the team" = {
+  ?test
+}
app/Teams/Show.alder:75:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Teams/Show.alder:97:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Teams/Show.alder:101:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Teams.Show."a stranger gets a 404 for the team"' - <<'EOF'
{
  let owner <- User.add (Text.Email "owner@example.com") (Text.NonEmpty "Owner")
  let stranger <- User.add (Text.Email "stranger@example.com") (Text.NonEmpty "Stranger")
  let team <- Team.make owner (Text.NonEmpty "Red")
  let _ <- Test.login { user: Just stranger }
  let r <- Test.open (Teams.Show { id: team })
  Task.succeed (r.answer == PageMissing)
}
EOF
edited app/Teams/Show.alder
--- a/app/Teams/Show.alder
+++ b/app/Teams/Show.alder
@@ -98,5 +98,10 @@
 }
 
 test "a stranger gets a 404 for the team" = {
-  ?test
+  let owner <- User.add (Text.Email "owner@example.com") (Text.NonEmpty "Owner")
+  let stranger <- User.add (Text.Email "stranger@example.com") (Text.NonEmpty "Stranger")
+  let team <- Team.make owner (Text.NonEmpty "Red")
+  let _ <- Test.login { user: Just stranger }
+  let r <- Test.open (Teams.Show { id: team })
+  Task.succeed (r.answer == PageMissing)
 }
app/Teams/Show.alder:75:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Teams/Show.alder:97:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder new test Teams.Show "a member cannot remove another member"
edited app/Teams/Show.alder
--- a/app/Teams/Show.alder
+++ b/app/Teams/Show.alder
@@ -105,3 +105,7 @@
   let r <- Test.open (Teams.Show { id: team })
   Task.succeed (r.answer == PageMissing)
 }
+
+test "a member cannot remove another member" = {
+  ?test
+}
app/Teams/Show.alder:75:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Teams/Show.alder:97:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Teams/Show.alder:110:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Teams.Show."a member cannot remove another member"' - <<'EOF'
{
  let owner <- User.add (Text.Email "owner@example.com") (Text.NonEmpty "Owner")
  let ann <- User.add (Text.Email "ann@example.com") (Text.NonEmpty "Ann")
  let bob <- User.add (Text.Email "bob@example.com") (Text.NonEmpty "Bob")
  let team <- Team.make owner (Text.NonEmpty "Red")
  let _ <- Member.add team ann
  let _ <- Member.add team bob
  let _ <- Test.login { user: Just ann }
  let page <- Test.get (Teams.Show { id: team })
  let r <- Test.run (update (Remove { user: bob }) page.model { user: ann })
  let found <- Member.roster team owner
  Task.succeed ({ value: r.value, people: Maybe.map (\x -> List.length x.people) found } == { value: Err "not found", people: Just 3 })
}
EOF
edited app/Teams/Show.alder
--- a/app/Teams/Show.alder
+++ b/app/Teams/Show.alder
@@ -107,5 +107,15 @@
 }
 
 test "a member cannot remove another member" = {
-  ?test
+  let owner <- User.add (Text.Email "owner@example.com") (Text.NonEmpty "Owner")
+  let ann <- User.add (Text.Email "ann@example.com") (Text.NonEmpty "Ann")
+  let bob <- User.add (Text.Email "bob@example.com") (Text.NonEmpty "Bob")
+  let team <- Team.make owner (Text.NonEmpty "Red")
+  let _ <- Member.add team ann
+  let _ <- Member.add team bob
+  let _ <- Test.login { user: Just ann }
+  let page <- Test.get (Teams.Show { id: team })
+  let r <- Test.run (update (Remove { user: bob }) page.model { user: ann })
+  let found <- Member.roster team owner
+  Task.succeed ({ value: r.value, people: Maybe.map (\x -> List.length x.people) found } == { value: Err "not found", people: Just 3 })
 }
app/Teams/Show.alder:75:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Teams/Show.alder:97:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder new test Teams.Show "the owner removes a member"
edited app/Teams/Show.alder
--- a/app/Teams/Show.alder
+++ b/app/Teams/Show.alder
@@ -119,3 +119,7 @@
   let found <- Member.roster team owner
   Task.succeed ({ value: r.value, people: Maybe.map (\x -> List.length x.people) found } == { value: Err "not found", people: Just 3 })
 }
+
+test "the owner removes a member" = {
+  ?test
+}
app/Teams/Show.alder:75:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Teams/Show.alder:97:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Teams/Show.alder:124:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Teams.Show."the owner removes a member"' - <<'EOF'
{
  let owner <- User.add (Text.Email "owner@example.com") (Text.NonEmpty "Owner")
  let ann <- User.add (Text.Email "ann@example.com") (Text.NonEmpty "Ann")
  let team <- Team.make owner (Text.NonEmpty "Red")
  let _ <- Member.add team ann
  let _ <- Test.login { user: Just owner }
  let _ <- Test.get (Teams.Show { id: team })
  let r <- Test.click (Remove { user: ann })
  let found <- Member.roster team owner
  Task.succeed ({ answer: r.answer, people: Maybe.map (\x -> List.length x.people) found } == { answer: Redirected (Teams.Show { id: team }), people: Just 1 })
}
EOF
edited app/Teams/Show.alder
--- a/app/Teams/Show.alder
+++ b/app/Teams/Show.alder
@@ -121,5 +121,13 @@
 }
 
 test "the owner removes a member" = {
-  ?test
+  let owner <- User.add (Text.Email "owner@example.com") (Text.NonEmpty "Owner")
+  let ann <- User.add (Text.Email "ann@example.com") (Text.NonEmpty "Ann")
+  let team <- Team.make owner (Text.NonEmpty "Red")
+  let _ <- Member.add team ann
+  let _ <- Test.login { user: Just owner }
+  let _ <- Test.get (Teams.Show { id: team })
+  let r <- Test.click (Remove { user: ann })
+  let found <- Member.roster team owner
+  Task.succeed ({ answer: r.answer, people: Maybe.map (\x -> List.length x.people) found } == { answer: Redirected (Teams.Show { id: team }), people: Just 1 })
 }
app/Teams/Show.alder:75:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Teams/Show.alder:97:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder new test Teams.Show "the owner cannot remove themselves"
edited app/Teams/Show.alder
--- a/app/Teams/Show.alder
+++ b/app/Teams/Show.alder
@@ -131,3 +131,7 @@
   let found <- Member.roster team owner
   Task.succeed ({ answer: r.answer, people: Maybe.map (\x -> List.length x.people) found } == { answer: Redirected (Teams.Show { id: team }), people: Just 1 })
 }
+
+test "the owner cannot remove themselves" = {
+  ?test
+}
app/Teams/Show.alder:75:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Teams/Show.alder:97:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Teams/Show.alder:136:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Teams.Show."the owner cannot remove themselves"' - <<'EOF'
{
  let owner <- User.add (Text.Email "owner@example.com") (Text.NonEmpty "Owner")
  let ann <- User.add (Text.Email "ann@example.com") (Text.NonEmpty "Ann")
  let team <- Team.make owner (Text.NonEmpty "Red")
  let _ <- Member.add team ann
  let _ <- Test.login { user: Just owner }
  let page <- Test.get (Teams.Show { id: team })
  let r <- Test.run (update (Remove { user: owner }) page.model { user: owner })
  let found <- Member.roster team owner
  Task.succeed ({ value: r.value, people: Maybe.map (\x -> List.length x.people) found } == { value: Err "not found", people: Just 2 })
}
EOF
edited app/Teams/Show.alder
--- a/app/Teams/Show.alder
+++ b/app/Teams/Show.alder
@@ -133,5 +133,13 @@
 }
 
 test "the owner cannot remove themselves" = {
-  ?test
+  let owner <- User.add (Text.Email "owner@example.com") (Text.NonEmpty "Owner")
+  let ann <- User.add (Text.Email "ann@example.com") (Text.NonEmpty "Ann")
+  let team <- Team.make owner (Text.NonEmpty "Red")
+  let _ <- Member.add team ann
+  let _ <- Test.login { user: Just owner }
+  let page <- Test.get (Teams.Show { id: team })
+  let r <- Test.run (update (Remove { user: owner }) page.model { user: owner })
+  let found <- Member.roster team owner
+  Task.succeed ({ value: r.value, people: Maybe.map (\x -> List.length x.people) found } == { value: Err "not found", people: Just 2 })
 }
app/Teams/Show.alder:75:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Teams/Show.alder:97:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder new test Invites.Index "accepting an invite shows the team"
edited app/Invites/Index.alder
--- a/app/Invites/Index.alder
+++ b/app/Invites/Index.alder
@@ -77,3 +77,7 @@
   let after <- Db.get Invite invite1
   Task.succeed ?expected
 }
+
+test "accepting an invite shows the team" = {
+  ?test
+}
app/Invites/Index.alder:64:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Invites/Index.alder:78:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Invites/Index.alder:82:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Invites.Index."accepting an invite shows the team"' - <<'EOF'
{
  let owner <- User.add (Text.Email "owner@example.com") (Text.NonEmpty "Owner")
  let ann <- User.add (Text.Email "ann@example.com") (Text.NonEmpty "Ann")
  let team <- Team.make owner (Text.NonEmpty "Red")
  let invite <- Invite.send team (Text.Email "ann@example.com")
  let _ <- Test.login { user: Just ann }
  let _ <- Test.get (Invites.Index {})
  let r <- Test.click (Accept { id: invite })
  let teams <- Test.get (Teams.Index {})
  let shown <- Test.get (Teams.Show { id: team })
  Task.succeed ({ answer: r.answer, listed: Test.texts "#teams .name" teams.doc, name: Test.text "#name" shown.doc } == { answer: Redirected (Teams.Show { id: team }), listed: ["Red"], name: Just "Red" })
}
EOF
edited app/Invites/Index.alder
--- a/app/Invites/Index.alder
+++ b/app/Invites/Index.alder
@@ -79,5 +79,14 @@
 }
 
 test "accepting an invite shows the team" = {
-  ?test
+  let owner <- User.add (Text.Email "owner@example.com") (Text.NonEmpty "Owner")
+  let ann <- User.add (Text.Email "ann@example.com") (Text.NonEmpty "Ann")
+  let team <- Team.make owner (Text.NonEmpty "Red")
+  let invite <- Invite.send team (Text.Email "ann@example.com")
+  let _ <- Test.login { user: Just ann }
+  let _ <- Test.get (Invites.Index {})
+  let r <- Test.click (Accept { id: invite })
+  let teams <- Test.get (Teams.Index {})
+  let shown <- Test.get (Teams.Show { id: team })
+  Task.succeed ({ answer: r.answer, listed: Test.texts "#teams .name" teams.doc, name: Test.text "#name" shown.doc } == { answer: Redirected (Teams.Show { id: team }), listed: ["Red"], name: Just "Red" })
 }
app/Invites/Index.alder:64:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Invites/Index.alder:78:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder new test Invites.Index "declining an invite keeps the team closed"
edited app/Invites/Index.alder
--- a/app/Invites/Index.alder
+++ b/app/Invites/Index.alder
@@ -90,3 +90,7 @@
   let shown <- Test.get (Teams.Show { id: team })
   Task.succeed ({ answer: r.answer, listed: Test.texts "#teams .name" teams.doc, name: Test.text "#name" shown.doc } == { answer: Redirected (Teams.Show { id: team }), listed: ["Red"], name: Just "Red" })
 }
+
+test "declining an invite keeps the team closed" = {
+  ?test
+}
app/Invites/Index.alder:64:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Invites/Index.alder:78:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Invites/Index.alder:95:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Invites.Index."declining an invite keeps the team closed"' - <<'EOF'
{
  let owner <- User.add (Text.Email "owner@example.com") (Text.NonEmpty "Owner")
  let ann <- User.add (Text.Email "ann@example.com") (Text.NonEmpty "Ann")
  let team <- Team.make owner (Text.NonEmpty "Red")
  let invite <- Invite.send team (Text.Email "ann@example.com")
  let _ <- Test.login { user: Just ann }
  let _ <- Test.get (Invites.Index {})
  let r <- Test.click (Decline { id: invite })
  let teams <- Test.get (Teams.Index {})
  let shown <- Test.open (Teams.Show { id: team })
  Task.succeed ({ answer: r.answer, listed: Test.texts "#teams .name" teams.doc, shown: shown.answer } == { answer: Redirected (Invites.Index {}), listed: [], shown: PageMissing })
}
EOF
edited app/Invites/Index.alder
--- a/app/Invites/Index.alder
+++ b/app/Invites/Index.alder
@@ -92,5 +92,14 @@
 }
 
 test "declining an invite keeps the team closed" = {
-  ?test
+  let owner <- User.add (Text.Email "owner@example.com") (Text.NonEmpty "Owner")
+  let ann <- User.add (Text.Email "ann@example.com") (Text.NonEmpty "Ann")
+  let team <- Team.make owner (Text.NonEmpty "Red")
+  let invite <- Invite.send team (Text.Email "ann@example.com")
+  let _ <- Test.login { user: Just ann }
+  let _ <- Test.get (Invites.Index {})
+  let r <- Test.click (Decline { id: invite })
+  let teams <- Test.get (Teams.Index {})
+  let shown <- Test.open (Teams.Show { id: team })
+  Task.succeed ({ answer: r.answer, listed: Test.texts "#teams .name" teams.doc, shown: shown.answer } == { answer: Redirected (Invites.Index {}), listed: [], shown: PageMissing })
 }
app/Invites/Index.alder:64:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Invites/Index.alder:78:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
```

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Invites/Index.alder:64:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Invites/Index.alder:78:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Teams/Index.alder:85:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Teams/Show.alder:75:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Teams/Show.alder:97:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
alder build: the program has 5 typed hole(s); fill them before building
```
