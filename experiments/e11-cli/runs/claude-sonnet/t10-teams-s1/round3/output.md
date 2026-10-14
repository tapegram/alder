```alder-cli
# Teams.Index: a multi-line view first, so the form generator accepts it
alder fill --decl Teams.Index.view - <<'EOF'
<Layout.Page title="Teams">
  <h1>Teams</h1>
</Layout.Page>
EOF
alder new form Teams.Index NewTeam name:Text.NonEmpty --msg Create
alder fill --decl Teams.Index.view - <<'EOF'
<Layout.Page title="Teams">
  <h1>Teams</h1>
  <ul id="teams">
    {for t in model.teams -> <li><a class="name" href={Teams.Show { id: t.id }}>{t.name}</a></li>}
  </ul>
  <form submit={Create}>
    <Errors form />
    <al-input field="name" label="Name" />
    <al-button type="submit">Create</al-button>
  </form>
</Layout.Page>
EOF
alder fill Teams.Index.update.Create - <<'EOF'
{
  let id <- Db.insert Team { name: p.form.name, owner: session.user }
  Task.succeed (Redirect (Teams.Show { id: id }))
}
EOF

# Teams.Show
alder fill --decl Teams.Show.Model - <<'EOF'
{ team: Team, owner: User, members: List User, isOwner: Bool }
EOF
alder fill --decl Teams.Show.load - <<'EOF'
{
  let found <- Db.get Team params.id
  match found {
    Nothing -> Query.notFound,
    Just team -> {
      let rows <- Db.from Member |> Db.where (Member.team.eq team.id) |> Db.all
      let allow = team.owner == session.user || List.any (\m -> m.user == session.user) rows
      if not allow then Query.notFound else {
        let owner <- Db.get User team.owner
        let users <- Db.from User |> Db.where (User.id.in (List.map (\m -> m.user) rows)) |> Db.all
        match owner {
          Nothing -> Query.notFound,
          Just o -> Query.succeed { team: team, owner: o, members: List.sortByText (\u -> u.name) users, isOwner: team.owner == session.user },
        }
      }
    },
  }
}
EOF
alder fill --decl Teams.Show.view - <<'EOF'
<Layout.Page title="Team">
  <h1>Team</h1>
</Layout.Page>
EOF
alder new form Teams.Show NewInvite email:Text.Email --msg SendInvite
alder new msg Teams.Show Remove user:User.UserId
alder fill --decl Teams.Show.view - <<'EOF'
<Layout.Page title={model.team.name}>
  <h1 id="name">{model.team.name}</h1>
  <ul id="members">
    <li><span class="name">{model.owner.name}</span></li>
    {for u in model.members ->
      <li>
        <span class="name">{u.name}</span>
        {if model.isOwner then <button class="remove" click={Remove { user: u.id }}>Remove</button> else Html.none}
      </li>}
  </ul>
  {if model.isOwner then
    <form submit={SendInvite}>
      <Errors form />
      <al-input field="email" label="Invite by email" />
      <al-button type="submit">Invite</al-button>
    </form>
  else Html.none}
</Layout.Page>
EOF
alder fill Teams.Show.update.SendInvite - <<'EOF'
if model.team.owner != session.user then Task.notFound else {
  let _ <- Db.insert Invite { team: model.team.id, email: p.form.email }
  Task.succeed (Redirect (Teams.Show { id: model.team.id }))
}
EOF
alder fill Teams.Show.update.Remove - <<'EOF'
if model.team.owner != session.user then Task.notFound else if p.user == model.team.owner then Task.notFound else {
  let rows <- Db.from Member |> Db.where (Member.team.eq model.team.id) |> Db.where (Member.user.eq p.user) |> Db.all
  let _ <- Task.all (List.map (\m -> Db.delete Member m.id) rows)
  Task.succeed (Redirect (Teams.Show { id: model.team.id }))
}
EOF

# Invites.Index
alder fill --decl Invites.Index.Model - <<'EOF'
{ invites: List Invite, teams: List Team }
EOF
alder fill --decl Invites.Index.load - <<'EOF'
{
  let me <- Db.get User session.user
  match me {
    Nothing -> Query.notFound,
    Just u -> {
      let invites <- Db.from Invite |> Db.where (Invite.email.eq u.email) |> Db.where (Invite.status.eq Pending) |> Db.all
      let teams <- Db.from Team |> Db.where (Team.id.in (List.map (\i -> i.team) invites)) |> Db.all
      Query.succeed { invites: invites, teams: teams }
    },
  }
}
EOF
alder fill --decl Invites.Index.view - <<'EOF'
<Layout.Page title="Invites">
  <h1>Invites</h1>
</Layout.Page>
EOF
alder new msg Invites.Index Accept id:Invite.InviteId
alder new msg Invites.Index Decline id:Invite.InviteId
alder fill --decl Invites.Index.view - <<'EOF'
<Layout.Page title="Invites">
  <h1>Invites</h1>
  <ul id="invites">
    {for i in model.invites ->
      <li>
        <span class="team">{Maybe.withDefault "" (Maybe.map (\t -> Text.trim t.name) (List.find (\t -> t.id == i.team) model.teams))}</span>
        <button class="accept" click={Accept { id: i.id }}>Accept</button>
        <button class="decline" click={Decline { id: i.id }}>Decline</button>
      </li>}
  </ul>
</Layout.Page>
EOF
alder fill Invites.Index.update.Accept - <<'EOF'
{
  let found <- Db.get Invite p.id
  let me <- Db.get User session.user
  match found {
    Nothing -> Task.notFound,
    Just inv -> {
      let mine = Maybe.map (\u -> u.email == inv.email) me == Just True
      if not mine || inv.status != Pending then Task.notFound else {
        let team <- Db.get Team inv.team
        let owned = Maybe.map (\t -> t.owner == session.user) team == Just True
        let existing <- Db.from Member |> Db.where (Member.team.eq inv.team) |> Db.where (Member.user.eq session.user) |> Db.count
        let _ <- Db.update Invite inv.id (\i -> { i | status: Accepted })
        let _ <- if owned || existing > 0 then Task.succeed () else Task.map (\x -> ()) (Db.insert Member { team: inv.team, user: session.user })
        Task.succeed (Redirect (Teams.Show { id: inv.team }))
      }
    },
  }
}
EOF
alder fill Invites.Index.update.Decline - <<'EOF'
{
  let found <- Db.get Invite p.id
  let me <- Db.get User session.user
  match found {
    Nothing -> Task.notFound,
    Just inv -> {
      let mine = Maybe.map (\u -> u.email == inv.email) me == Just True
      if not mine || inv.status != Pending then Task.notFound else {
        let _ <- Db.update Invite inv.id (\i -> { i | status: Declined })
        Task.succeed (Redirect (Invites.Index {}))
      }
    },
  }
}
EOF

# tests
alder new test Teams.Show "a team page is not found for a non-member"
alder fill 'Teams.Show."a team page is not found for a non-member"' - <<'EOF'
{
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let eve <- Db.insert User { email: Text.Email "eve@example.com", name: Text.NonEmpty "Eve", passwordHash: "" }
  let t <- Db.insert Team { name: Text.NonEmpty "Red", owner: ann }
  let _ <- Test.login { user: Just eve }
  let r <- Test.open (Teams.Show { id: t })
  Task.succeed (r.answer == PageMissing)
}
EOF
alder new test Teams.Show "a member cannot remove another member"
alder fill 'Teams.Show."a member cannot remove another member"' - <<'EOF'
{
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let cat <- Db.insert User { email: Text.Email "cat@example.com", name: Text.NonEmpty "Cat", passwordHash: "" }
  let t <- Db.insert Team { name: Text.NonEmpty "Red", owner: ann }
  let _ <- Db.insert Member { team: t, user: bob }
  let _ <- Db.insert Member { team: t, user: cat }
  let _ <- Test.login { user: Just ann }
  let _ <- Test.get (Teams.Show { id: t })
  let _ <- Test.login { user: Just bob }
  let r <- Test.click (Remove { user: cat })
  let n <- Db.from Member |> Db.count
  Task.succeed ({ answer: r.answer, members: n } == { answer: PageMissing, members: 2 })
}
EOF
alder new test Teams.Show "the owner removes a member but not themselves"
alder fill 'Teams.Show."the owner removes a member but not themselves"' - <<'EOF'
{
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let t <- Db.insert Team { name: Text.NonEmpty "Red", owner: ann }
  let _ <- Db.insert Member { team: t, user: bob }
  let _ <- Test.login { user: Just ann }
  let page <- Test.get (Teams.Show { id: t })
  let own <- Test.run (update (Remove { user: ann }) page.model { user: ann })
  let r <- Test.click (Remove { user: bob })
  let n <- Db.from Member |> Db.count
  Task.succeed ({ own: own.value, answer: r.answer, members: n } == { own: Err "not found", answer: Redirected (Teams.Show { id: t }), members: 0 })
}
EOF
alder new test Invites.Index "accepting an invite makes the team visible"
alder fill 'Invites.Index."accepting an invite makes the team visible"' - <<'EOF'
{
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let t <- Db.insert Team { name: Text.NonEmpty "Red", owner: ann }
  let inv <- Db.insert Invite { team: t, email: Text.Email "bob@example.com" }
  let _ <- Test.login { user: Just bob }
  let _ <- Test.get (Invites.Index {})
  let r <- Test.click (Accept { id: inv })
  let page <- Test.get (Teams.Index {})
  Task.succeed ({ answer: r.answer, teams: Test.texts "#teams .name" page.doc } == { answer: Redirected (Teams.Show { id: t }), teams: ["Red"] })
}
EOF
alder new test Invites.Index "declining an invite keeps the team closed"
alder fill 'Invites.Index."declining an invite keeps the team closed"' - <<'EOF'
{
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let t <- Db.insert Team { name: Text.NonEmpty "Red", owner: ann }
  let inv <- Db.insert Invite { team: t, email: Text.Email "bob@example.com" }
  let _ <- Test.login { user: Just bob }
  let _ <- Test.get (Invites.Index {})
  let r <- Test.click (Decline { id: inv })
  let page <- Test.get (Teams.Index {})
  let show <- Test.open (Teams.Show { id: t })
  Task.succeed ({ answer: r.answer, teams: Test.count "#teams li" page.doc, show: show.answer } == { answer: Redirected (Invites.Index {}), teams: 0, show: PageMissing })
}
EOF

# login target
alder fill --list
alder fill Auth.Login.update.Submit - --hole ?afterLogin <<'EOF'
Teams.Index {}
EOF
```
