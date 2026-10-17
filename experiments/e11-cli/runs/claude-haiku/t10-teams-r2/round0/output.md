```alder-cli
# users and login: the User table as the task names it, then the reference's login
alder new table User email:Text.Email@unique name:Text.NonEmpty passwordHash:Text created:Time@created --no-timestamps
alder new auth --users User

# teams, memberships, invites
alder new table Team name:Text.NonEmpty owner:User.UserId@ref=User --no-timestamps
alder new table Member team:Team.TeamId@ref=Team user:User.UserId@ref=User --no-timestamps
alder new type Invite "Status = Pending | Accepted | Declined"
alder new table Invite team:Team.TeamId@ref=Team email:Text.Email status:Status=Pending --no-timestamps

# the pages, blank first; each gets a placeholder view so forms and messages can be added
alder new page Teams.Index /teams --blank --requires user
alder new page Teams.Show '/teams/{id}' --param id:Team.TeamId --blank --requires user
alder new page Invites.Index /invites --blank --requires user
alder fill --decl Teams.Index.view - <<'EOF'
\model session -> <p>placeholder</p>
EOF
alder fill --decl Teams.Show.view - <<'EOF'
\model session -> <p>placeholder</p>
EOF
alder fill --decl Invites.Index.view - <<'EOF'
\model session -> <p>placeholder</p>
EOF

# Teams.Index: the list, the create form
alder fill --decl Teams.Index.Model - <<'EOF'
{ teams: List Team }
EOF
alder fill --decl Teams.Index.load - <<'EOF'
\params session -> {
  let memberships <- Db.from Member |> Db.where (Member.user.eq session.user) |> Db.all
  let ids = List.map (\m -> m.team) memberships
  let teams <-
    Db.from Team
      |> Db.where (Db.or [Team.owner.eq session.user, Team.id.in ids])
      |> Db.orderBy Team.name.asc
      |> Db.all
  Query.succeed { teams: teams }
}
EOF
alder new form Teams.Index NewTeam name:Text.NonEmpty --msg Create
alder fill --decl Teams.Index.update - <<'EOF'
\msg model session -> match msg {
  Create p -> {
    let id <- Db.insert Team { name: p.form.name, owner: session.user }
    Task.succeed (Redirect (Teams.Show { id: id }))
  },
}
EOF
alder fill --decl Teams.Index.view - <<'EOF'
\model session -> <Layout.Page title="Teams">
  <h1>Teams</h1>
  <ul id="teams">
    {for t in model.teams -> <li><a class="name" href={Teams.Show { id: t.id }}>{t.name}</a></li>}
  </ul>
  <al-card>
    <form submit={Create}>
      <al-input field="name" label="New team" />
      <al-button type="submit">Create team</al-button>
    </form>
  </al-card>
</Layout.Page>
EOF

# Teams.Show: the members, Remove, and the owner's invite form
alder new type Teams.Show "Person = { id: User.UserId, name: Text, isOwner: Bool }"
alder fill --decl Teams.Show.Model - <<'EOF'
{ team: Team, viewerIsOwner: Bool, members: List Person }
EOF
alder new decl Teams.Show person "Bool -> User -> Person"
alder fill --decl Teams.Show.person - <<'EOF'
\isOwner u -> { id: u.id, name: u.name, isOwner: isOwner }
EOF
alder fill --decl Teams.Show.load - <<'EOF'
\params session -> {
  let found <- Db.get Team params.id
  match found {
    Nothing -> Query.notFound,
    Just team -> {
      let rows <- Db.from Member |> Db.where (Member.team.eq team.id) |> Db.all
      let mine <- Db.from Member |> Db.where (Member.team.eq team.id) |> Db.where (Member.user.eq session.user) |> Db.first
      let owner <- Db.get User team.owner
      let others <- Db.from User |> Db.where (User.id.in (List.map (\m -> m.user) rows)) |> Db.orderBy User.name.asc |> Db.all
      if team.owner == session.user || Maybe.isJust mine then {
        let ownerPeople = List.map (person True) (Maybe.toList owner)
        let otherPeople = List.map (person False) others
        Query.succeed { team: team, viewerIsOwner: team.owner == session.user, members: ownerPeople ++ otherPeople }
      } else Query.notFound
    },
  }
}
EOF
alder new msg Teams.Show Remove user:User.UserId
alder new form Teams.Show NewInvite email:Text.Email --msg SendInvite
alder fill --decl Teams.Show.update - <<'EOF'
\msg model session -> match msg {
  Remove p -> {
    if session.user != model.team.owner || p.user == model.team.owner then Task.notFound else {
      let rows <- Db.from Member |> Db.where (Member.team.eq model.team.id) |> Db.where (Member.user.eq p.user) |> Db.all
      let _ <- Task.all (List.map (\m -> Db.delete Member m.id) rows)
      Task.succeed (Redirect (Teams.Show { id: model.team.id }))
    }
  },
  SendInvite p -> {
    if session.user != model.team.owner then Task.notFound else {
      let _ <- Db.insert Invite { team: model.team.id, email: p.form.email }
      Task.succeed (Redirect (Teams.Show { id: model.team.id }))
    }
  },
}
EOF
alder fill --decl Teams.Show.view - <<'EOF'
\model session -> <Layout.Page title={model.team.name}>
  <h1 id="name">{model.team.name}</h1>
  <ul id="members">
    {for m in model.members -> <li>
      <span class="name">{m.name}</span>
      {if m.isOwner then <span class="role">owner</span> else Html.none}
      {if model.viewerIsOwner && not m.isOwner then <button class="remove" click={Remove { user: m.id }}>Remove</button> else Html.none}
    </li>}
  </ul>
  {if model.viewerIsOwner then <al-card>
    <form submit={SendInvite}>
      <al-input field="email" label="Invite by email" />
      <al-button type="submit">Invite</al-button>
    </form>
  </al-card> else Html.none}
  <a href={Teams.Index {}}>All teams</a>
</Layout.Page>
EOF

# Invites.Index: waiting invites, Accept and Decline
alder new type Invites.Index "Waiting = { id: Invite.InviteId, team: Text }"
alder fill --decl Invites.Index.Model - <<'EOF'
{ invites: List Waiting }
EOF
alder new decl Invites.Index invitedTo "List Team -> Invite -> Maybe Waiting"
alder fill --decl Invites.Index.invitedTo - <<'EOF'
\teams i -> match List.find (\t -> t.id == i.team) teams {
  Just t -> Just { id: i.id, team: t.name },
  Nothing -> Nothing,
}
EOF
alder new decl Invites.Index waitingFor "Maybe User -> Invite -> Bool"
alder fill --decl Invites.Index.waitingFor - <<'EOF'
\me inv -> inv.status == Pending && Maybe.map (\u -> u.email) me == Just inv.email
EOF
alder fill --decl Invites.Index.load - <<'EOF'
\params session -> {
  let me <- Db.get User session.user
  match me {
    Nothing -> Query.notFound,
    Just u -> {
      let rows <- Db.from Invite |> Db.where (Invite.email.eq u.email) |> Db.where (Invite.status.eq Pending) |> Db.all
      let teams <- Db.from Team |> Db.where (Team.id.in (List.map (\i -> i.team) rows)) |> Db.all
      Query.succeed { invites: List.filterMap (invitedTo teams) rows }
    },
  }
}
EOF
alder new msg Invites.Index Accept id:Invite.InviteId
alder new msg Invites.Index Decline id:Invite.InviteId
alder fill --decl Invites.Index.update - <<'EOF'
\msg model session -> match msg {
  Accept p -> {
    let me <- Db.get User session.user
    let found <- Db.get Invite p.id
    match found {
      Nothing -> Task.notFound,
      Just inv -> {
        if not (waitingFor me inv) then Task.notFound else {
          let _ <- Db.update Invite inv.id (\i -> { i | status: Accepted })
          let _ <- Db.insert Member { team: inv.team, user: session.user }
          Task.succeed (Redirect (Teams.Show { id: inv.team }))
        }
      },
    }
  },
  Decline p -> {
    let me <- Db.get User session.user
    let found <- Db.get Invite p.id
    match found {
      Nothing -> Task.notFound,
      Just inv -> {
        if not (waitingFor me inv) then Task.notFound else {
          let _ <- Db.update Invite inv.id (\i -> { i | status: Declined })
          Task.succeed (Redirect (Invites.Index {}))
        }
      },
    }
  },
}
EOF
alder fill --decl Invites.Index.view - <<'EOF'
\model session -> <Layout.Page title="Invites">
  <h1>Invites</h1>
  <ul id="invites">
    {for i in model.invites -> <li>
      <span class="team">{i.team}</span>
      <al-button variant="secondary" size="sm" click={Accept { id: i.id }}>Accept</al-button>
      <al-button variant="ghost" size="sm" click={Decline { id: i.id }}>Decline</al-button>
    </li>}
  </ul>
</Layout.Page>
EOF

# Auth.Login: after a login, go to the teams list
alder fill --decl Auth.Login.update - <<'EOF'
\msg model session -> match msg {
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

# tests: Teams.Show
alder new test Teams.Show "a team is not found for a non-member"
alder fill 'Teams.Show."a team is not found for a non-member"' - <<'EOF'
{
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owen", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Olga", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Red", owner: owner }
  let _ <- Test.login { user: Just other }
  let page <- Test.open (Teams.Show { id: team })
  Task.succeed (page.answer == PageMissing)
}
EOF

alder new test Teams.Show "a member cannot remove another member"
alder fill 'Teams.Show."a member cannot remove another member"' - <<'EOF'
{
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owen", passwordHash: "" }
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Red", owner: owner }
  let _ <- Db.insert Member { team: team, user: ann }
  let _ <- Db.insert Member { team: team, user: bob }
  let model : Model = { team: { id: team, name: Text.NonEmpty "Red", owner: owner }, viewerIsOwner: False, members: [] }
  let r <- Test.run (update (Remove { user: bob }) model { user: ann })
  let left <- Db.from Member |> Db.where (Member.user.eq bob) |> Db.count
  Task.succeed ({ value: r.value, left: left } == { value: Err "not found", left: 1 })
}
EOF

alder new test Teams.Show "the owner removes a member but not themselves"
alder fill 'Teams.Show."the owner removes a member but not themselves"' - <<'EOF'
{
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owen", passwordHash: "" }
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Red", owner: owner }
  let _ <- Db.insert Member { team: team, user: ann }
  let model : Model = { team: { id: team, name: Text.NonEmpty "Red", owner: owner }, viewerIsOwner: True, members: [] }
  let gone <- Test.run (update (Remove { user: ann }) model { user: owner })
  let self <- Test.run (update (Remove { user: owner }) model { user: owner })
  let left <- Db.from Member |> Db.where (Member.user.eq ann) |> Db.count
  Task.succeed ({ gone: gone.value, self: self.value, left: left } == { gone: Ok (Redirect (Teams.Show { id: team })), self: Err "not found", left: 0 })
}
EOF

alder new test Teams.Show "the owner sees a Remove button only on other members"
alder fill 'Teams.Show."the owner sees a Remove button only on other members"' - <<'EOF'
{
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owen", passwordHash: "" }
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Red", owner: owner }
  let _ <- Db.insert Member { team: team, user: ann }
  let _ <- Test.login { user: Just owner }
  let page <- Test.get (Teams.Show { id: team })
  Task.succeed ({ removes: Test.count "button.remove" page.doc, names: Test.texts "#members .name" page.doc } == { removes: 1, names: ["Owen", "Ann"] })
}
EOF

alder new test Teams.Show "a non-owner cannot invite"
alder fill 'Teams.Show."a non-owner cannot invite"' - <<'EOF'
{
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owen", passwordHash: "" }
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Red", owner: owner }
  let _ <- Db.insert Member { team: team, user: ann }
  let model : Model = { team: { id: team, name: Text.NonEmpty "Red", owner: owner }, viewerIsOwner: False, members: [] }
  let r <- Test.run (update (SendInvite { form: { email: Text.Email "carl@example.com" } }) model { user: ann })
  let invites <- Db.from Invite |> Db.count
  Task.succeed ({ value: r.value, invites: invites } == { value: Err "not found", invites: 0 })
}
EOF

# tests: Invites.Index
alder new test Invites.Index "accepting an invite makes the team visible"
alder fill 'Invites.Index."accepting an invite makes the team visible"' - <<'EOF'
{
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owen", passwordHash: "" }
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Red", owner: owner }
  let inv <- Db.insert Invite { team: team, email: Text.Email "me@example.com" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Invites.Index {})
  let r <- Test.click (Accept { id: inv })
  let teams <- Test.get (Teams.Index {})
  let shown <- Test.open (Teams.Show { id: team })
  Task.succeed ({ answer: r.answer, listed: Test.texts ".name" teams.doc, open: shown.answer } == { answer: Redirected (Teams.Show { id: team }), listed: ["Red"], open: Shown })
}
EOF

alder new test Invites.Index "declining an invite leaves the team closed"
alder fill 'Invites.Index."declining an invite leaves the team closed"' - <<'EOF'
{
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owen", passwordHash: "" }
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Red", owner: owner }
  let inv <- Db.insert Invite { team: team, email: Text.Email "me@example.com" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Invites.Index {})
  let r <- Test.click (Decline { id: inv })
  let shown <- Test.open (Teams.Show { id: team })
  let teams <- Test.get (Teams.Index {})
  Task.succeed ({ answer: r.answer, open: shown.answer, listed: Test.texts ".name" teams.doc, invites: Test.count "#invites li" r.doc } == { answer: Redirected (Invites.Index {}), open: PageMissing, listed: [], invites: 0 })
}
EOF

alder test
```
