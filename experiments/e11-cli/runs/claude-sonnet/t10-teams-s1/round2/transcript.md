### Round 2

```text
$ alder remove page Teams.Index
removed app/Teams/Index.alder
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
  let owned <- Db.from Team |> Db.where (Team.owner.eq session.user) |> Db.all
  let mem <- Db.from Member |> Db.where (Member.user.eq session.user) |> Db.all
  let joined <- Db.from Team |> Db.where (Team.id.in (List.map (\m -> m.team) mem)) |> Db.all
  Query.succeed { teams: List.sortByText (\t -> t.name) (List.unique (owned ++ joined)) }
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
+  let mem <- Db.from Member |> Db.where (Member.user.eq session.user) |> Db.all
+  let joined <- Db.from Team |> Db.where (Team.id.in (List.map (\m -> m.team) mem)) |> Db.all
+  Query.succeed { teams: List.sortByText (\t -> t.name) (List.unique (owned ++ joined)) }
+}
 
 view : Model -> Session -> Html Msg
 view model session = ?view
app/Teams/Index.alder:19:22: warning[W0002]: hole ?view has type Html Teams.Index.Msg
  view model session = ?view
                       ^^^^^
added import Member to app/Teams/Index.alder
[exit 0]
$ alder fill --decl Teams.Index.view - <<'EOF'
<p>x</p>
EOF
edited app/Teams/Index.alder
--- a/app/Teams/Index.alder
+++ b/app/Teams/Index.alder
@@ -16,7 +16,7 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = ?view
+view model session = <p>x</p>
 
 test "a visitor is sent to log in" = {
   let r <- Test.open (Teams.Index {})
[exit 0]
$ alder new form Teams.Index NewTeam name:Text.NonEmpty --msg Create
alder new form: `Teams.Index`'s `view` is not one element over several lines: add the markup yourself
[exit 1]
```

The harness stopped at line 18 of your script: `alder new form Teams.Index NewTeam name:Text.NonEmpty --msg Create`: it exited 1. The commands after it were not run.

Your script stopped at command 6; the app is as the commands before it left it; continue.

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
app/Teams/Show.alder:11:23: warning[W0002]: hole ?load has type Query Teams.Show.Model
  load params session = ?load
                        ^^^^^
app/Teams/Show.alder:14:22: warning[W0002]: hole ?view has type Html Teams.Show.Msg
  view model session = ?view
                       ^^^^^
alder build: the program has 6 typed hole(s); fill them before building
```
