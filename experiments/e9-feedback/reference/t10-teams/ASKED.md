# Tests asked only: t10-teams

The calibration line for H1 (`findings.md` reads the baseline scores against it): the
reference's app code with only the tests the task's tests paragraph names, scored by
`alder test --mutate`.

The paragraph:

> Also write tests (`test "…" = …`, chapter 10) in the modules they test: a
> team's page is not found for a user who is not a member; a member cannot
> remove another member; the owner removes a member but cannot remove
> themselves; accepting an invite makes the team visible, declining does not.
> `alder test` must pass. Do not write tests that call the `Password`
> capability.

## How to reproduce

1. Copy `reference/t10-teams/` to a scratch directory (drop this file and `SURVIVORS.md`).
2. Delete every `test` declaration in `app/` except the ones below, and add the ones below that are missing (same module, same text).
3. Run `alder test --mutate .` there.

## Score: 15/44 = 0.341

| Class | Survivors |
|---|---|
| authorization | 4 |
| data | 14 |
| flow | 7 |
| logic | 0 |
| schema | 4 |

By operator: redirect → rerender 6, where dropped 6, cascade ↔ restrict 4, write replaced 4, requires dropped 3, session put dropped 2, orderBy reversed 2, reject → rerender 1, guard dropped 1.

```text
4 passed, 0 failed
44 mutants: 15 killed, 29 survived  (score 0.34)
survived:
  App             update:15       session put dropped   let _ <- Session.put (\s -> { s | user: Nothing }) → let _ <- Task.succeed ()
  App             update:16       redirect → rerender   Redirect (Auth.Login {}) → Rerender
  Auth.Login      update:40       session put dropped   let _ <- Session.put (\s -> { s | user: Just id }) → let _ <- Task.succeed ()
  Auth.Login      update:41       redirect → rerender   Redirect (Teams.Index {}) → Rerender
  Auth.Login      update:43       reject → rerender     Reject "Email or password is incorrect." → Rerender
  Team            Team:11         cascade ↔ restrict    owner @ref User @restrict → owner @ref User @cascade
  Invite          Invite:12       cascade ↔ restrict    team @ref Team @restrict → team @ref Team @cascade
  Member          Member:12       cascade ↔ restrict    team @ref Team @restrict → team @ref Team @cascade
  Member          Member:13       cascade ↔ restrict    user @ref User @restrict → user @ref User @cascade
  Member          row:18:35       where dropped         Db.where (Member.team.eq team) → the condition dropped
  Member          row:18:69       where dropped         Db.where (Member.user.eq user) → the condition dropped
  Invites.Index   requires:9      requires dropped      requires user → served without user (user = User.UserId "mutant")
  Invites.Index   waiting:20:33   where dropped         Db.where (Invite.email.eq u.email) → the condition dropped
  Invites.Index   waiting:20:71   where dropped         Db.where (Invite.status.eq Pending) → the condition dropped
  Invites.Index   update:46       write replaced        let _ <- Db.update Invite invite.id (\i -> { i | status: Accepted }) → let _ <- Task.succeed ()
  Invites.Index   update:48       redirect → rerender   Redirect (Teams.Show { id: invite.team }) → Rerender
  Invites.Index   update:52       write replaced        let _ <- Db.update Invite invite.id (\i -> { i | status: Declined }) → let _ <- Task.succeed ()
  Invites.Index   update:53       redirect → rerender   Redirect (Invites.Index {}) → Rerender
  Teams.Index     requires:8      requires dropped      requires user → served without user (user = User.UserId "mutant")
  Teams.Index     load:15         where dropped         Db.where (Member.user.eq session.user) → the condition dropped
  Teams.Index     load:19         orderBy reversed      Db.orderBy Team.name.asc → Db.orderBy Team.name.desc
  Teams.Index     update:33       write replaced        let id <- Db.insert Team { name: p.form.name, owner: session.user } → let id <- Task.succeed (Team.TeamId "mutant")
  Teams.Index     update:34       redirect → rerender   Redirect (Teams.Show { id: id }) → Rerender
  Teams.Show      requires:9      requires dropped      requires user → served without user (user = User.UserId "mutant")
  Teams.Show      load:24         where dropped         Db.where (User.id.in ids) → the condition dropped
  Teams.Show      load:24         orderBy reversed      Db.orderBy User.name.asc → Db.orderBy User.name.desc
  Teams.Show      update:46       guard dropped         if p.user == model.team.owner then … else … → the fail arm dropped
  Teams.Show      update:58       write replaced        let _ <- Db.insert Invite { team: model.team.id, email: p.form.email } → let _ <- Task.succeed (Invite.InviteId "mutant")
  Teams.Show      update:59       redirect → rerender   Redirect (Teams.Show { id: model.team.id }) → Rerender
```

## The tests

`app/Invites/Index.alder`:

```alder
test "accepting an invite makes the team visible, declining does not" = {
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let red <- Db.insert Team { name: Text.NonEmpty "Red", owner: ann }
  let blue <- Db.insert Team { name: Text.NonEmpty "Blue", owner: ann }
  let yes <- Db.insert Invite { team: red, email: Text.Email "bob@example.com" }
  let no <- Db.insert Invite { team: blue, email: Text.Email "bob@example.com" }
  let _ <- Test.login { user: Just bob }
  let _ <- Test.get (Invites.Index {})
  let _ <- Test.click (Accept { id: yes })
  let _ <- Test.get (Invites.Index {})
  let _ <- Test.click (Decline { id: no })
  let teams <- Test.get (Teams.Index {})
  let redPage <- Test.open (Teams.Show { id: red })
  let bluePage <- Test.open (Teams.Show { id: blue })
  Task.succeed ({ teams: Test.texts "#teams .name" teams.doc, red: redPage.answer, blue: bluePage.answer } == { teams: ["Red"], red: Shown, blue: PageMissing })
```

`app/Teams/Show.alder`:

```alder
test "a team's page is not found for a user who is not a member" = {
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let sam <- Db.insert User { email: Text.Email "sam@example.com", name: Text.NonEmpty "Sam", passwordHash: "" }
  let red <- Db.insert Team { name: Text.NonEmpty "Red", owner: ann }
  let _ <- Test.login { user: Just sam }
  let r <- Test.open (Teams.Show { id: red })
  Task.succeed (r.answer == PageMissing)

test "a member cannot remove another member" = {
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let cid <- Db.insert User { email: Text.Email "cid@example.com", name: Text.NonEmpty "Cid", passwordHash: "" }
  let red <- Db.insert Team { name: Text.NonEmpty "Red", owner: ann }
  let _ <- Db.insert Member { team: red, user: bob }
  let _ <- Db.insert Member { team: red, user: cid }
  let model <- load { id: red } { user: bob }
  let r <- Test.run (update (Remove { user: cid }) model { user: bob })
  let members <- Db.from Member |> Db.count
  Task.succeed ({ value: r.value, members: members } == { value: Err "not found", members: 2 })

test "the owner removes a member but cannot remove themselves" = {
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let red <- Db.insert Team { name: Text.NonEmpty "Red", owner: ann }
  let _ <- Db.insert Member { team: red, user: bob }
  let _ <- Test.login { user: Just ann }
  let _ <- Test.get (Teams.Show { id: red })
  let removed <- Test.click (Remove { user: bob })
  let model <- load { id: red } { user: ann }
  let self <- Test.run (update (Remove { user: ann }) model { user: ann })
  let members <- Db.from Member |> Db.count
  let team <- Db.get Team red
  Task.succeed ({ removed: removed.answer, members: members, selfRefused: Result.toMaybe self.value == Nothing, owner: Maybe.map (\t -> t.owner) team } == { removed: Redirected (Teams.Show { id: red }), members: 0, selfRefused: True, owner: Just ann })
```
