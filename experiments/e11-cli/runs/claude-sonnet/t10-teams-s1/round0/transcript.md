### Round 0

```text
$ alder new table User email:Text.Email@unique name:Text.NonEmpty passwordHash:Text created:Time@created --no-timestamps
wrote app/User.alder
wrote migrations/0001_initial.sql
  create table User
wrote migrations/schema.json
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
+  let user1 <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
+  let target <- Db.insert Team { name: Text.NonEmpty "name-1", owner: user1 }
+  let _ <- Db.insert Invite { team: target, email: Text.Email "email-1@example.com" }
+  let r <- Test.run (Db.delete Team target)
+  let left <- Db.from Invite |> Db.count
+  Task.succeed ({ deleted: Result.toMaybe r.value, left: left } == { deleted: Nothing, left: 1 })
+}
[exit 0]
$ alder new page Teams.Index /teams --list Team --requires user
alder new page: `--requires user` needs a session with a `user` field (`session { user: Maybe User.UserId }` in `App`): this app's session has none
[exit 1]
```

The harness stopped at line 9 of your script: `alder new page Teams.Index /teams --list Team --requires user`: it exited 1. The commands after it were not run.

Your script stopped at command 6; the app is as the commands before it left it; continue.

The harness ran `alder check`, `alder build` and `alder test` on the app as the commands left it: all pass, but the round does not: the script stopped before its end.
