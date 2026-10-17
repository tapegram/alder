```alder-cli
# users and logging in
alder new table User email:Text.Email@unique name:Text.NonEmpty passwordHash:Text created:Time@created --no-timestamps
alder new auth --users User
alder fill --list

# the tables
alder new table Team name:Text.NonEmpty owner:User.UserId@ref=User --no-timestamps
alder new table Member team:Team.TeamId@ref=Team user:User.UserId@ref=User --no-timestamps
alder new type Invite "Status = Pending | Accepted | Declined"
alder new table Invite team:Team.TeamId@ref=Team email:Text.Email status:Status=Pending --no-timestamps

# the pages
alder new page Teams.Index /teams --requires user --blank
alder new page Teams.Show '/teams/{id}' --param id:Team.TeamId --requires user --blank
alder new page Invites.Index /invites --requires user --blank

# after login
alder fill Auth.Login.update.Submit --hole ?afterLogin - <<'EOF'
Teams.Index {}
EOF

# Teams.Index
alder fill --decl Teams.Index.Model - <<'EOF'
{ teams: List Team }
EOF
alder fill Teams.Index.load - <<'EOF'
{
  let memberships <- Db.from Member |> Db.where (Member.user.eq session.user) |> Db.all
  let teams <- Db.from Team |> Db.where (Db.or [Team.owner.eq session.user, Team.id.in (List.map (\m -> m.team) memberships)]) |> Db.orderBy Team.name.asc |> Db.all
  Query.succeed { teams: teams }
}
EOF
alder fill Teams.Index.view - <<'EOF'
<Layout.Page title="Teams">
  <h1>Teams</h1>
</Layout.Page>
EOF
alder new form Teams.Index NewTeam name:Text.NonEmpty --msg Create
alder fill --decl Teams.Index.update - <<'EOF'
match msg {
  Create p -> {
    let id <- Db.insert Team { name: p.form.name, owner: session.user }
    Task.succeed (Redirect (Teams.Show { id: id }))
  },
}
EOF
alder fill --decl Teams.Index.view - <<'EOF'
<Layout.Page title="Teams">
  <h1>Teams</h1>
  <ul id="teams">
    {for t in model.teams ->
      <li><a class="name" href={Teams.Show { id: t.id }}>{t.name}</a></li>}
  </ul>
  <form submit={Create}>
    <Errors form />
    <Input field="name" label="New team" />
    <button>Create</button>
  </form>
  <nav>
    <a href={Invites.Index {}}>Invites</a>
    <button click={App.Logout}>Log out</button>
  </nav>
</Layout.Page>
EOF

# Teams.Show
alder fill --decl Teams.Show.Model - <<'EOF'
{ team: Team, owner: User, members: List User, isOwner: Bool }
EOF
alder fill Teams.Show.load - <<'EOF'
{
  let found <- Db.get Team params.id
  let rows <- Db.from Member |> Db.where (Member.team.eq params.id) |> Db.all
  let ids = List.map (\m -> m.user) rows
  let others <- Db.from User |> Db.where (User.id.in ids) |> Db.orderBy User.name.asc |> Db.all
  match found {
    Nothing -> Query.notFound,
    Just team -> {
      let owner <- Db.get User team.owner
      match owner {
        Nothing -> Query.notFound,
        Just o -> if team.owner == session.user || List.member session.user ids then Query.succeed { team: team, owner: o, members: others, isOwner: team.owner == session.user } else Query.notFound,
      }
    },
  }
}
EOF
alder fill Teams.Show.view - <<'EOF'
<Layout.Page title="Team">
  <h1>Team</h1>
</Layout.Page>
EOF
alder new msg Teams.Show Remove user:User.UserId
alder new form Teams.Show NewInvite email:Text.Email --msg SendInvite
alder fill --decl Teams.Show.update - <<'EOF'
match msg {
  Remove p -> if not model.isOwner || p.user == session.user then Task.notFound else {
    let row <- Db.from Member |> Db.where (Member.team.eq model.team.id) |> Db.where (Member.user.eq p.user) |> Db.first
    match row {
      Nothing -> Task.notFound,
      Just m -> {
        let _ <- Db.delete Member m.id
        Task.succeed (Redirect (Teams.Show { id: model.team.id }))
      },
    }
  },
  SendInvite p -> if not model.isOwner then Task.notFound else {
    let _ <- Db.insert Invite { team: model.team.id, email: p.form.email }
    Task.succeed (Redirect (Teams.Show { id: model.team.id }))
  },
}
EOF
alder fill --decl Teams.Show.view - <<'EOF'
<Layout.Page title={model.team.name}>
  <h1 id="name">{model.team.name}</h1>
  <ul id="members">
    <li><span class="name">{model.owner.name}</span> (owner)</li>
    {for u in model.members ->
      <li>
        <span class="name">{u.name}</span>
        {if model.isOwner then <button class="remove" click={Remove { user: u.id }}>Remove</button> else Html.none}
      </li>}
  </ul>
  {if model.isOwner then
    <form submit={SendInvite}>
      <Errors form />
      <Input field="email" label="Invite by email" />
      <button>Invite</button>
    </form>
  else Html.none}
  <nav><a href={Teams.Index {}}>Teams</a></nav>
</Layout.Page>
EOF

# Invites.Index
alder fill --decl Invites.Index.Model - <<'EOF'
{ invites: List Invite, teams: List Team }
EOF
alder fill Invites.Index.load - <<'EOF'
{
  let me <- Db.get User session.user
  match me {
    Nothing -> Query.succeed { invites: [], teams: [] },
    Just u -> {
      let invites <- Db.from Invite |> Db.where (Invite.email.eq u.email) |> Db.where (Invite.status.eq Pending) |> Db.all
      let teams <- Db.from Team |> Db.where (Team.id.in (List.map (\i -> i.team) invites)) |> Db.all
      Query.succeed { invites: invites, teams: teams }
    },
  }
}
EOF
alder new decl Invites.Index teamName "List Team -> Team.TeamId -> Text"
alder fill Invites.Index.teamName - <<'EOF'
\teams id -> match List.find (\t -> t.id == id) teams {
  Just t -> "{t.name}",
  Nothing -> "",
}
EOF
alder fill Invites.Index.view - <<'EOF'
<Layout.Page title="Invites">
  <h1>Invites</h1>
</Layout.Page>
EOF
alder new msg Invites.Index Accept id:Invite.InviteId
alder new msg Invites.Index Decline id:Invite.InviteId
alder fill --decl Invites.Index.update - <<'EOF'
match msg {
  Accept p -> match List.find (\i -> i.id == p.id) model.invites {
    Nothing -> Task.notFound,
    Just inv -> {
      let already <- Db.from Member |> Db.where (Member.team.eq inv.team) |> Db.where (Member.user.eq session.user) |> Db.count
      let team <- Db.get Team inv.team
      let owns = Maybe.map (\t -> t.owner) team == Just session.user
      let _ <- Db.update Invite inv.id (\i -> { i | status: Accepted })
      let _ <- if already > 0 || owns then Task.succeed () else {
        let _ <- Db.insert Member { team: inv.team, user: session.user }
        Task.succeed ()
      }
      Task.succeed (Redirect (Teams.Show { id: inv.team }))
    },
  },
  Decline p -> match List.find (\i -> i.id == p.id) model.invites {
    Nothing -> Task.notFound,
    Just inv -> {
      let _ <- Db.update Invite inv.id (\i -> { i | status: Declined })
      Task.succeed (Redirect (Invites.Index {}))
    },
  },
}
EOF
alder fill --decl Invites.Index.view - <<'EOF'
<Layout.Page title="Invites">
  <h1>Invites</h1>
  <ul id="invites">
    {for i in model.invites ->
      <li>
        <span class="team">{teamName model.teams i.team}</span>
        <button class="accept" click={Accept { id: i.id }}>Accept</button>
        <button class="decline" click={Decline { id: i.id }}>Decline</button>
      </li>}
  </ul>
  <nav><a href={Teams.Index {}}>Teams</a></nav>
</Layout.Page>
EOF

# tests
alder new test Teams.Show "a team's page is not found for a non-member"
alder fill 'Teams.Show."a team'"'"'s page is not found for a non-member"' - <<'EOF'
{
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let eve <- Db.insert User { email: Text.Email "eve@example.com", name: Text.NonEmpty "Eve", passwordHash: "" }
  let red <- Db.insert Team { name: Text.NonEmpty "Red", owner: ann }
  let _ <- Test.login { user: Just eve }
  let r <- Test.open (Teams.Show { id: red })
  Task.succeed (r.answer == PageMissing)
}
EOF
alder new test Teams.Show "a member cannot remove another member"
alder fill 'Teams.Show."a member cannot remove another member"' - <<'EOF'
{
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let cat <- Db.insert User { email: Text.Email "cat@example.com", name: Text.NonEmpty "Cat", passwordHash: "" }
  let red <- Db.insert Team { name: Text.NonEmpty "Red", owner: ann }
  let _ <- Db.insert Member { team: red, user: bob }
  let _ <- Db.insert Member { team: red, user: cat }
  let _ <- Test.login { user: Just bob }
  let mine <- Test.get (Teams.Show { id: red })
  let _ <- Test.login { user: Just ann }
  let _ <- Test.get (Teams.Show { id: red })
  let _ <- Test.login { user: Just bob }
  let r <- Test.click (Remove { user: cat })
  let n <- Db.from Member |> Db.where (Member.team.eq red) |> Db.count
  Task.succeed ({ buttons: Test.count ".remove" mine.doc, answer: r.answer, members: n } == { buttons: 0, answer: PageMissing, members: 2 })
}
EOF
alder new test Teams.Show "the owner removes a member but not themselves"
alder fill 'Teams.Show."the owner removes a member but not themselves"' - <<'EOF'
{
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let red <- Db.insert Team { name: Text.NonEmpty "Red", owner: ann }
  let _ <- Db.insert Member { team: red, user: bob }
  let _ <- Test.login { user: Just ann }
  let pg <- Test.get (Teams.Show { id: red })
  let selfRemove <- Test.run (update (Remove { user: ann }) pg.model { user: ann })
  let r <- Test.click (Remove { user: bob })
  let n <- Db.from Member |> Db.where (Member.team.eq red) |> Db.count
  Task.succeed ({ buttons: Test.count "#members .remove" pg.doc, selfRemove: selfRemove.value, removed: r.answer, members: n, names: Test.texts "#members .name" r.doc } == { buttons: 1, selfRemove: Err "not found", removed: Redirected (Teams.Show { id: red }), members: 0, names: ["Ann"] })
}
EOF
alder new test Invites.Index "accepting an invite makes the team visible, declining does not"
alder fill 'Invites.Index."accepting an invite makes the team visible, declining does not"' - <<'EOF'
{
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let redId <- Db.insert Team { name: Text.NonEmpty "Red", owner: ann }
  let blueId <- Db.insert Team { name: Text.NonEmpty "Blue", owner: ann }
  let toRed <- Db.insert Invite { team: redId, email: Text.Email "bob@example.com" }
  let toBlue <- Db.insert Invite { team: blueId, email: Text.Email "bob@example.com" }
  let _ <- Test.login { user: Just bob }
  let _ <- Test.get (Invites.Index {})
  let a <- Test.click (Accept { id: toRed })
  let _ <- Test.get (Invites.Index {})
  let d <- Test.click (Decline { id: toBlue })
  let teams <- Test.get (Teams.Index {})
  let blue <- Test.open (Teams.Show { id: blueId })
  Task.succeed ({ accept: a.answer, decline: d.answer, invites: Test.count "#invites li" d.doc, teams: Test.texts "#teams .name" teams.doc, blue: blue.answer } == { accept: Redirected (Teams.Show { id: redId }), decline: Redirected (Invites.Index {}), invites: 0, teams: ["Red"], blue: PageMissing })
}
EOF

alder fill --list
alder check
alder test
```
