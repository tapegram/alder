```alder-cli
# 1. Login: the successful login goes to the teams list (Teams.Index exists now)
alder fill Auth.Login.update - --hole ?afterLogin <<'EOF'
Teams.Index {}
EOF

# 2. Teams.Index: the model and the query behind the list
alder fill --decl Teams.Index.Model - <<'EOF'
{ teams: List Team }
EOF
alder fill --decl Teams.Index.load - <<'EOF'
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

# 3. Teams.Show: the team's page, for its owner and members only
alder new page Teams.Show '/teams/{id}' --blank --param id:Team.TeamId --requires user
alder fill --decl Teams.Show.Model - <<'EOF'
{ team: Team, isOwner: Bool, people: List User }
EOF
alder fill --decl Teams.Show.load - <<'EOF'
{
  let found <- Db.get Team params.id
  match found {
    Nothing -> Query.notFound,
    Just team -> {
      let rows <- Db.from Member |> Db.where (Member.team.eq team.id) |> Db.all
      let isMember = List.any (\m -> m.user == session.user) rows
      let isOwner = team.owner == session.user
      if not isOwner && not isMember then Query.notFound else {
        let ids = List.map (\m -> m.user) rows
        let users <- Db.from User |> Db.where (User.id.in ([team.owner] ++ ids)) |> Db.all
        let owners = List.filter (\u -> u.id == team.owner) users
        let others = List.sortByText (\u -> u.name) (List.filter (\u -> u.id != team.owner) users)
        Query.succeed { team: team, isOwner: isOwner, people: owners ++ others }
      }
    },
  }
}
EOF
alder fill --decl Teams.Show.view - <<'EOF'
<Layout.Page title={model.team.name}>
  <h1 id="name">{model.team.name}</h1>
  <ul id="members">
    {for u in model.people ->
      <li><span class="name">{u.name}</span></li>}
  </ul>
  <al-button variant="outline" click={App.Logout}>Log out</al-button>
</Layout.Page>
EOF
alder new msg Teams.Show Remove user:User.UserId
alder new form Teams.Show NewInvite email:Text.Email --msg SendInvite
alder fill --decl Teams.Show.view - <<'EOF'
<Layout.Page title={model.team.name}>
  <h1 id="name">{model.team.name}</h1>
  <ul id="members">
    {for u in model.people ->
      <li>
        <span class="name">{u.name}</span>
        {if u.id == model.team.owner then <span class="role">owner</span> else Html.none}
        {if model.isOwner && u.id != model.team.owner then <button class="remove" click={Remove { user: u.id }}>Remove</button> else Html.none}
      </li>}
  </ul>
  {if model.isOwner then <al-card><form submit={SendInvite}><Errors form /><al-input field="email" label="Invite by email" /><al-button type="submit">Invite</al-button></form></al-card> else Html.none}
  <al-button variant="outline" click={App.Logout}>Log out</al-button>
</Layout.Page>
EOF
alder fill --decl Teams.Show.update - <<'EOF'
match msg {
  Remove p -> {
    if not model.isOwner || p.user == model.team.owner then Task.notFound else {
      let rows <- Db.from Member |> Db.where (Member.team.eq model.team.id) |> Db.where (Member.user.eq p.user) |> Db.all
      let _ <- Task.all (List.map (\r -> Db.delete Member r.id) rows)
      Task.succeed (Redirect (Teams.Show { id: model.team.id }))
    }
  },
  SendInvite p -> {
    if not model.isOwner then Task.notFound else {
      let _ <- Db.insert Invite { team: model.team.id, email: p.form.email }
      Task.succeed (Redirect (Teams.Show { id: model.team.id }))
    }
  },
}
EOF

# 4. Teams.Index: the create form and its update
alder fill --decl Teams.Index.view - <<'EOF'
<Layout.Page title="Teams">
  <h1>Teams</h1>
  <ul id="teams">
    {for t in model.teams ->
      <li><a href={Teams.Show { id: t.id }}><span class="name">{t.name}</span></a></li>}
  </ul>
  <al-button variant="outline" click={App.Logout}>Log out</al-button>
</Layout.Page>
EOF
alder new form Teams.Index NewTeam name:Text.NonEmpty --msg Create
alder fill --decl Teams.Index.view - <<'EOF'
<Layout.Page title="Teams">
  <h1>Teams</h1>
  <ul id="teams">
    {for t in model.teams ->
      <li><a href={Teams.Show { id: t.id }}><span class="name">{t.name}</span></a></li>}
  </ul>
  <al-card>
    <form submit={Create}>
      <Errors form />
      <al-input field="name" label="New team name" />
      <al-button type="submit">Create team</al-button>
    </form>
  </al-card>
  <al-button variant="outline" click={App.Logout}>Log out</al-button>
</Layout.Page>
EOF
alder fill --decl Teams.Index.update - <<'EOF'
match msg {
  Create p -> {
    let id <- Db.insert Team { name: p.form.name, owner: session.user }
    Task.succeed (Redirect (Teams.Show { id: id }))
  },
}
EOF

# 5. Invites.Index: the invites waiting for the user
alder new page Invites.Index '/invites' --blank --requires user
alder new type Invites.Index "Waiting = { id: Invite.InviteId, team: Team }"
alder fill --decl Invites.Index.Model - <<'EOF'
{ waiting: List Waiting }
EOF
alder fill --decl Invites.Index.load - <<'EOF'
{
  let me <- Db.get User session.user
  match me {
    Nothing -> Query.notFound,
    Just u -> {
      let rows <- Db.from Invite |> Db.where (Invite.email.eq u.email) |> Db.where (Invite.status.eq Pending) |> Db.all
      let teams <- Db.from Team |> Db.where (Team.id.in (List.map (\i -> i.team) rows)) |> Db.all
      let pair : Invite -> Team -> Waiting = \i t -> { id: i.id, team: t }
      let waiting : List Waiting = List.filterMap (\i -> Maybe.map (pair i) (List.find (\t -> t.id == i.team) teams)) rows
      Query.succeed { waiting: waiting }
    },
  }
}
EOF
alder fill --decl Invites.Index.view - <<'EOF'
<Layout.Page title="Invites">
  <h1>Invites</h1>
  <ul id="invites">
    {for w in model.waiting ->
      <li><span class="team">{w.team.name}</span></li>}
  </ul>
  <al-button variant="outline" click={App.Logout}>Log out</al-button>
</Layout.Page>
EOF
alder new msg Invites.Index Accept id:Invite.InviteId
alder new msg Invites.Index Decline id:Invite.InviteId
alder fill --decl Invites.Index.view - <<'EOF'
<Layout.Page title="Invites">
  <h1>Invites</h1>
  <ul id="invites">
    {for w in model.waiting ->
      <li>
        <span class="team">{w.team.name}</span>
        <al-button variant="secondary" click={Accept { id: w.id }}>Accept</al-button>
        <al-button variant="ghost" click={Decline { id: w.id }}>Decline</al-button>
      </li>}
  </ul>
  <al-button variant="outline" click={App.Logout}>Log out</al-button>
</Layout.Page>
EOF
alder fill --decl Invites.Index.update - <<'EOF'
match msg {
  Accept p -> {
    let found <- Db.get Invite p.id
    let me <- Db.get User session.user
    match found {
      Nothing -> Task.notFound,
      Just inv -> {
        if not (Invite.answerable inv me) then Task.notFound else {
          let _ <- Db.update Invite inv.id (\x -> { x | status: Accepted })
          let owned <- Db.from Team |> Db.where (Team.id.eq inv.team) |> Db.where (Team.owner.eq session.user) |> Db.count
          if owned > 0 then Task.succeed (Redirect (Teams.Show { id: inv.team })) else {
            let _ <- Db.insert Member { team: inv.team, user: session.user }
            Task.succeed (Redirect (Teams.Show { id: inv.team }))
          }
        }
      },
    }
  },
  Decline p -> {
    let found <- Db.get Invite p.id
    let me <- Db.get User session.user
    match found {
      Nothing -> Task.notFound,
      Just inv -> {
        if not (Invite.answerable inv me) then Task.notFound else {
          let _ <- Db.update Invite inv.id (\x -> { x | status: Declined })
          Task.succeed (Redirect (Invites.Index {}))
        }
      },
    }
  },
}
EOF

# 6. Tests: Teams.Show
alder new test Teams.Show "a stranger is not found"
alder fill 'Teams.Show."a stranger is not found"' - <<'EOF'
{
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owen", passwordHash: "" }
  let stranger <- Db.insert User { email: Text.Email "stranger@example.com", name: Text.NonEmpty "Sam", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Red", owner: owner }
  let _ <- Test.login { user: Just stranger }
  let page <- Test.open (Teams.Show { id: team })
  Task.succeed (page.answer == PageMissing)
}
EOF

alder new test Teams.Show "a member cannot remove another member"
alder fill 'Teams.Show."a member cannot remove another member"' - <<'EOF'
{
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owen", passwordHash: "" }
  let a <- Db.insert User { email: Text.Email "a@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let b <- Db.insert User { email: Text.Email "b@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Red", owner: owner }
  let _ <- Db.insert Member { team: team, user: a }
  let _ <- Db.insert Member { team: team, user: b }
  let _ <- Test.login { user: Just owner }
  let _ <- Test.get (Teams.Show { id: team })
  let _ <- Test.login { user: Just a }
  let r <- Test.click (Remove { user: b })
  let left <- Db.from Member |> Db.where (Member.team.eq team) |> Db.count
  Task.succeed ({ answer: r.answer, left: left } == { answer: PageMissing, left: 2 })
}
EOF

alder new test Teams.Show "the owner removes a member"
alder fill 'Teams.Show."the owner removes a member"' - <<'EOF'
{
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owen", passwordHash: "" }
  let a <- Db.insert User { email: Text.Email "a@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let b <- Db.insert User { email: Text.Email "b@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Red", owner: owner }
  let _ <- Db.insert Member { team: team, user: a }
  let _ <- Db.insert Member { team: team, user: b }
  let _ <- Test.login { user: Just owner }
  let _ <- Test.get (Teams.Show { id: team })
  let r <- Test.click (Remove { user: a })
  let left <- Db.from Member |> Db.where (Member.team.eq team) |> Db.count
  Task.succeed ({ answer: r.answer, left: left } == { answer: Redirected (Teams.Show { id: team }), left: 1 })
}
EOF

alder new test Teams.Show "the owner cannot remove themselves"
alder fill 'Teams.Show."the owner cannot remove themselves"' - <<'EOF'
{
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owen", passwordHash: "" }
  let a <- Db.insert User { email: Text.Email "a@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Red", owner: owner }
  let _ <- Db.insert Member { team: team, user: a }
  let _ <- Test.login { user: Just owner }
  let page <- Test.get (Teams.Show { id: team })
  let r <- Test.run (update (Remove { user: owner }) page.model { user: owner })
  let left <- Db.from Member |> Db.where (Member.team.eq team) |> Db.count
  Task.succeed ({ value: r.value, left: left } == { value: Err "not found", left: 1 })
}
EOF

alder new test Teams.Show "a member cannot invite"
alder fill 'Teams.Show."a member cannot invite"' - <<'EOF'
{
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owen", passwordHash: "" }
  let a <- Db.insert User { email: Text.Email "a@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Red", owner: owner }
  let _ <- Db.insert Member { team: team, user: a }
  let _ <- Test.login { user: Just owner }
  let _ <- Test.get (Teams.Show { id: team })
  let _ <- Test.login { user: Just a }
  let r <- Test.submit NewInvite { email: Text.Email "new@example.com" }
  let invites <- Db.from Invite |> Db.count
  Task.succeed ({ answer: r.answer, invites: invites } == { answer: PageMissing, invites: 0 })
}
EOF

alder new test Teams.Show "the owner invites by email"
alder fill 'Teams.Show."the owner invites by email"' - <<'EOF'
{
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owen", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Red", owner: owner }
  let _ <- Test.login { user: Just owner }
  let _ <- Test.get (Teams.Show { id: team })
  let r <- Test.submit NewInvite { email: Text.Email "new@example.com" }
  let waiting <- Db.from Invite |> Db.where (Invite.email.eq (Text.Email "new@example.com")) |> Db.count
  Task.succeed ({ answer: r.answer, waiting: waiting } == { answer: Redirected (Teams.Show { id: team }), waiting: 1 })
}
EOF

# 7. Tests: Invites.Index
alder new test Invites.Index "accepting an invite makes the team visible"
alder fill 'Invites.Index."accepting an invite makes the team visible"' - <<'EOF'
{
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owen", passwordHash: "" }
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Red", owner: owner }
  let invite <- Db.insert Invite { team: team, email: Text.Email "me@example.com" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Invites.Index {})
  let r <- Test.click (Accept { id: invite })
  let page <- Test.get (Teams.Index {})
  let show <- Test.open (Teams.Show { id: team })
  Task.succeed ({ answer: r.answer, listed: Test.texts "#teams .name" page.doc, show: show.answer } == { answer: Redirected (Teams.Show { id: team }), listed: ["Red"], show: Shown })
}
EOF

alder new test Invites.Index "declining an invite keeps the team closed"
alder fill 'Invites.Index."declining an invite keeps the team closed"' - <<'EOF'
{
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owen", passwordHash: "" }
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Red", owner: owner }
  let invite <- Db.insert Invite { team: team, email: Text.Email "me@example.com" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Invites.Index {})
  let r <- Test.click (Decline { id: invite })
  let page <- Test.get (Teams.Index {})
  let show <- Test.open (Teams.Show { id: team })
  Task.succeed ({ answer: r.answer, waiting: Test.count "#invites li" r.doc, listed: Test.count "#teams li" page.doc, show: show.answer } == { answer: Redirected (Invites.Index {}), waiting: 0, listed: 0, show: PageMissing })
}
EOF

alder new test Invites.Index "a stranger cannot accept an invite"
alder fill 'Invites.Index."a stranger cannot accept an invite"' - <<'EOF'
{
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owen", passwordHash: "" }
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let stranger <- Db.insert User { email: Text.Email "stranger@example.com", name: Text.NonEmpty "Sam", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Red", owner: owner }
  let invite <- Db.insert Invite { team: team, email: Text.Email "me@example.com" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Invites.Index {})
  let _ <- Test.login { user: Just stranger }
  let r <- Test.click (Accept { id: invite })
  let members <- Db.from Member |> Db.where (Member.team.eq team) |> Db.count
  Task.succeed ({ answer: r.answer, members: members } == { answer: PageMissing, members: 0 })
}
EOF

# 8. Probe: the login test still holds a hole; this lists the holes and their addresses
alder fill --list
```
