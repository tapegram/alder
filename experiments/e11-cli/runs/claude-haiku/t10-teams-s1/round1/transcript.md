### Round 1

```text
$ alder new decl Member canSee "Team.TeamId -> User.UserId -> Query Bool"
edited app/Member.alder
--- a/app/Member.alder
+++ b/app/Member.alder
@@ -11,6 +11,9 @@
   user: User.UserId @ref User,
 }
 
+canSee : Team.TeamId -> User.UserId -> Query Bool
+canSee = ?canSee
+
 test "a Member keeps its team from being deleted" = {
   let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
   let target <- Db.insert Team { name: Text.NonEmpty "name-1", owner: me }
app/Member.alder:15:10: warning[W0002]: hole ?canSee has type Team.TeamId -> User.UserId -> Query Bool
  canSee = ?canSee
           ^^^^^^^
[exit 0]
$ alder fill --decl Member.canSee - <<'EOF'
\team user -> {
  let found <- Db.get Team team
  let n <- Db.from Member |> Db.where (Member.team.eq team) |> Db.where (Member.user.eq user) |> Db.count
  Query.succeed (Maybe.withDefault False (Maybe.map (\t -> t.owner == user) found) || n > 0)
}
EOF
edited app/Member.alder
--- a/app/Member.alder
+++ b/app/Member.alder
@@ -12,7 +12,11 @@
 }
 
 canSee : Team.TeamId -> User.UserId -> Query Bool
-canSee = ?canSee
+canSee = \team user -> {
+  let found <- Db.get Team team
+  let n <- Db.from Member |> Db.where (Member.team.eq team) |> Db.where (Member.user.eq user) |> Db.count
+  Query.succeed (Maybe.withDefault False (Maybe.map (\t -> t.owner == user) found) || n > 0)
+}
 
 test "a Member keeps its team from being deleted" = {
   let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
[exit 0]
$ alder new decl Invite answerable "Invite -> Maybe User -> Bool"
edited app/Invite.alder
--- a/app/Invite.alder
+++ b/app/Invite.alder
@@ -14,6 +14,9 @@
   status: Status = Pending,
 }
 
+answerable : Invite -> Maybe User -> Bool
+answerable = ?answerable
+
 test "a Invite keeps its team from being deleted" = {
   let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
   let target <- Db.insert Team { name: Text.NonEmpty "name-1", owner: me }
app/Invite.alder:18:14: warning[W0002]: hole ?answerable has type Invite -> Maybe User -> Bool
  answerable = ?answerable
               ^^^^^^^^^^^
[exit 0]
$ alder fill --decl Invite.answerable - <<'EOF'
\inv me -> inv.status == Pending && Maybe.map (\u -> u.email) me == Just inv.email
EOF
edited app/Invite.alder
--- a/app/Invite.alder
+++ b/app/Invite.alder
@@ -15,7 +15,7 @@
 }
 
 answerable : Invite -> Maybe User -> Bool
-answerable = ?answerable
+answerable = \inv me -> inv.status == Pending && Maybe.map (\u -> u.email) me == Just inv.email
 
 test "a Invite keeps its team from being deleted" = {
   let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
[exit 0]
$ alder new page Teams.Index '/teams' --blank --requires user
wrote app/Teams/Index.alder
app/Teams/Index.alder:8:23: warning[W0002]: hole ?load has type Query Teams.Index.Model
  load params session = ?load
                        ^^^^^
app/Teams/Index.alder:11:22: warning[W0002]: hole ?view has type Html Teams.Index.Msg
  view model session = ?view
                       ^^^^^
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
app/Teams/Index.alder:18:19: error[E0002]: no field `teams` on `Teams.Index.Model`
    Query.succeed { teams: teams }
                    ^^^^^^^^^^^^
app/Teams/Index.alder:22:22: warning[W0002]: hole ?view has type Html Teams.Index.Msg
  view model session = ?view
                       ^^^^^
alder fill: the result does not check (the errors above): nothing was written
[exit 1]
```

The harness stopped at line 19 of your script: `alder fill --decl Teams.Index.load - <<'EOF'`: it exited 1. The commands after it were not run.

Your script stopped at command 6; the app is as the commands before it left it; continue.

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Auth/Login.alder:47:33: warning[W0002]: hole ?afterLogin has type Page
          Task.succeed (Redirect (?afterLogin))
                                  ^^^^^^^^^^^
app/Auth/Login.alder:86:85: warning[W0002]: hole ?afterLogin has type Page
    Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
                                                                                      ^^^^^^^^^^^
app/Teams/Index.alder:8:23: warning[W0002]: hole ?load has type Query Teams.Index.Model
  load params session = ?load
                        ^^^^^
app/Teams/Index.alder:11:22: warning[W0002]: hole ?view has type Html Teams.Index.Msg
  view model session = ?view
                       ^^^^^
alder build: the program has 4 typed hole(s); fill them before building
```
