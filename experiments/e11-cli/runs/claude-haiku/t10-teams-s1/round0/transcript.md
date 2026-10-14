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
$ alder fill --decl Layout.Page - <<'EOF'
<html>
  <head>
    <meta charset="utf-8" />
    <title>{props.title}</title>
    <link rel="stylesheet" href={Url.external "/static/al/al.css"} />
    <link rel="stylesheet" href={Url.external "/static/app.css"} />
  </head>
  <body>
    <al-page width="narrow">
      <main>{props.children}</main>
    </al-page>
  </body>
</html>
EOF
[exit 0]
$ alder fill --decl Auth.Login.update - <<'EOF'
match msg {
  Submit p -> {
    let found <- Password.verify { email: p.form.email, password: p.form.password }
    match found {
      Just id -> {
        let _ <- Session.put (\s -> { s | user: Just id })
        Task.succeed (Redirect (Teams.Index {}))
      },
      Nothing -> Task.succeed (Reject "Email or password is incorrect."),
    }
  },
}
EOF
app/Auth/Login.alder:47:33: error[E0002]: unknown `Teams.Index`
          Task.succeed (Redirect (Teams.Index {}))
                                  ^^^^^^^^^^^
alder fill: the result does not check (the errors above): nothing was written
[exit 1]
```

The harness stopped at line 29 of your script: `alder fill --decl Auth.Login.update - <<'EOF'`: it exited 1. The commands after it were not run.

Your script stopped at command 8; the app is as the commands before it left it; continue.

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Auth/Login.alder:47:33: warning[W0002]: hole ?afterLogin has type Page
          Task.succeed (Redirect (?afterLogin))
                                  ^^^^^^^^^^^
app/Auth/Login.alder:86:85: warning[W0002]: hole ?afterLogin has type Page
    Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
                                                                                      ^^^^^^^^^^^
alder build: the program has 2 typed hole(s); fill them before building
```
