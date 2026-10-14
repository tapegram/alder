```alder-cli
# Teams.Index is rebuilt blank, since the old generated model is unknown
alder remove page Teams.Index
alder new page Teams.Index /teams --blank --requires user
alder fill --decl Teams.Index.Model - <<'EOF'
{ teams: List Team }
EOF
alder fill --decl Teams.Index.load - <<'EOF'
{
  let owned <- Db.from Team |> Db.where (Team.owner.eq session.user) |> Db.all
  let mem <- Db.from Member |> Db.where (Member.user.eq session.user) |> Db.all
  let joined <- Db.from Team |> Db.where (Team.id.in (List.map (\m -> m.team) mem)) |> Db.all
  Query.succeed { teams: List.sortByText (\t -> t.name) (List.unique (owned ++ joined)) }
}
EOF
alder fill --decl Teams.Index.view - <<'EOF'
<p>x</p>
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
    <Input field="name" label="Name" />
    <button>Create</button>
  </form>
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

# Teams.Show
alder fill --decl Teams.Show.Model - <<'EOF'
{ team: Team, members: List User, isOwner: Bool }
EOF
alder fill --decl Teams.Show.view - <<'EOF'
<p>x</p>
EOF
alder new msg Teams.Show Remove user:User.UserId
alder new form Teams.Show NewInvite email:Text.Email --msg SendInvite
alder fill --decl Teams.Show.load - <<'EOF'
{
  let found <- Db.get Team params.id
  match found {
    Nothing -> Query.notFound,
    Just team -> {
      let mine <- Db.from Member |> Db.where (Member.team.eq team.id) |> Db.where (Member.user.eq session.user) |> Db.first
      if team.owner != session.user && not (Maybe.isJust mine) then Query.notFound else {
        let rows <- Db.from Member |> Db.where (Member.team.eq team.id) |> Db.all
        let others <- Db.from User |> Db.where (User.id.in (List.map (\m -> m.user) rows)) |> Db.orderBy User.name.asc |> Db.all
        let owner <- Db.get User team.owner
        Query.succeed { team: team, members: Maybe.toList owner ++ others, isOwner: team.owner == session.user }
      }
    },
  }
}
EOF
alder fill --decl Teams.Show.view - <<'EOF'
<Layout.Page title={model.team.name}>
  <h1 id="name">{model.team.name}</h1>
  <ul id="members">
    {for u in model.members ->
      <li>
        <span class="name">{u.name}</span>
        {if model.isOwner && u.id != model.team.owner then <button class="remove" click={Remove { user: u.id }}>Remove</button> else Html.none}
      </li>}
  </ul>
  {if model.isOwner then <form submit={SendInvite}>
    <Errors form />
    <Input field="email" label="Invite by email" />
    <button>Invite</button>
  </form> else Html.none}
</Layout.Page>
EOF
alder fill --decl Teams.Show.update - <<'EOF'
match msg {
  Remove p ->
    if not model.isOwner || p.user == model.team.owner then Task.notFound else {
      let rows <- Db.from Member |> Db.where (Member.team.eq model.team.id) |> Db.where (Member.user.eq p.user) |> Db.all
      let _ <- Task.all (List.map (\m -> Db.delete Member m.id) rows)
      Task.succeed (Redirect (Teams.Show { id: model.team.id }))
    },
  SendInvite p ->
    if not model.isOwner then Task.notFound else {
      let _ <- Db.insert Invite { team: model.team.id, email: p.form.email }
      Task.succeed (Redirect (Teams.Show { id: model.team.id }))
    },
}
EOF

# Invites.Index
alder fill --decl Invites.Index.Model - <<'EOF'
{ rows: List { invite: Invite, team: Team } }
EOF
alder fill --decl Invites.Index.view - <<'EOF'
<p>x</p>
EOF
alder new msg Invites.Index Accept id:Invite.InviteId
alder new msg Invites.Index Decline id:Invite.InviteId
alder fill --decl Invites.Index.load - <<'EOF'
{
  let me <- Db.get User session.user
  match me {
    Nothing -> Query.succeed { rows: [] },
    Just u -> {
      let invs <- Db.from Invite |> Db.where (Invite.email.eq u.email) |> Db.where (Invite.status.eq Pending) |> Db.all
      let teams <- Db.from Team |> Db.where (Team.id.in (List.map (\i -> i.team) invs)) |> Db.all
      Query.succeed { rows: List.filterMap (\i -> Maybe.map (\t -> { invite: i, team: t }) (List.find (\t -> t.id == i.team) teams)) invs }
    },
  }
}
EOF
alder fill --decl Invites.Index.view - <<'EOF'
<Layout.Page title="Invites">
  <h1>Invites</h1>
  <ul id="invites">
    {for r in model.rows ->
      <li>
        <span class="team">{r.team.name}</span>
        <button click={Accept { id: r.invite.id }}>Accept</button>
        <button click={Decline { id: r.invite.id }}>Decline</button>
      </li>}
  </ul>
</Layout.Page>
EOF
alder fill --decl Invites.Index.update - <<'EOF'
match msg {
  Accept p -> {
    let me <- Db.get User session.user
    let inv <- Db.get Invite p.id
    match me {
      Nothing -> Task.notFound,
      Just u -> {
        match inv {
          Nothing -> Task.notFound,
          Just i ->
            if i.email != u.email || i.status != Pending then Task.notFound else {
              let _ <- Db.update Invite i.id (\x -> { x | status: Accepted })
              let existing <- Db.from Member |> Db.where (Member.team.eq i.team) |> Db.where (Member.user.eq session.user) |> Db.first
              let team <- Db.get Team i.team
              let isOwner = Maybe.map (\t -> t.owner == session.user) team == Just True
              let skip = isOwner || Maybe.isJust existing
              let _ <- if skip then Task.succeed () else Task.map (\x -> ()) (Db.insert Member { team: i.team, user: session.user })
              Task.succeed (Redirect (Teams.Show { id: i.team }))
            },
        }
      },
    }
  },
  Decline p -> {
    let me <- Db.get User session.user
    let inv <- Db.get Invite p.id
    match me {
      Nothing -> Task.notFound,
      Just u -> {
        match inv {
          Nothing -> Task.notFound,
          Just i ->
            if i.email != u.email || i.status != Pending then Task.notFound else {
              let _ <- Db.update Invite i.id (\x -> { x | status: Declined })
              Task.succeed (Redirect (Invites.Index {}))
            },
        }
      },
    }
  },
}
EOF

# login target
alder fill Auth.Login.update - --hole ?afterLogin <<'EOF'
Teams.Index {}
EOF

# tests
alder new test Teams.Show "a stranger gets a 404 for a team page"
alder fill 'Teams.Show."a stranger gets a 404 for a team page"' - <<'EOF'
{
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let eve <- Db.insert User { email: Text.Email "eve@example.com", name: Text.NonEmpty "Eve", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Red", owner: ann }
  let _ <- Test.login { user: Just eve }
  let r <- Test.open (Teams.Show { id: team })
  Task.succeed (r.answer == PageMissing)
}
EOF
alder new test Teams.Show "a member cannot remove another member"
alder fill 'Teams.Show."a member cannot remove another member"' - <<'EOF'
{
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let cat <- Db.insert User { email: Text.Email "cat@example.com", name: Text.NonEmpty "Cat", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Red", owner: ann }
  let _ <- Db.insert Member { team: team, user: bob }
  let _ <- Db.insert Member { team: team, user: cat }
  let _ <- Test.login { user: Just ann }
  let _ <- Test.get (Teams.Show { id: team })
  let _ <- Test.login { user: Just bob }
  let r <- Test.click (Remove { user: cat })
  let n <- Db.from Member |> Db.count
  Task.succeed ({ answer: r.answer, n: n } == { answer: PageMissing, n: 2 })
}
EOF
alder new test Teams.Show "the owner removes a member but not themselves"
alder fill 'Teams.Show."the owner removes a member but not themselves"' - <<'EOF'
{
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Red", owner: ann }
  let _ <- Db.insert Member { team: team, user: bob }
  let _ <- Test.login { user: Just ann }
  let page <- Test.get (Teams.Show { id: team })
  let self <- Test.run (update (Remove { user: ann }) page.model { user: ann })
  let r <- Test.click (Remove { user: bob })
  let n <- Db.from Member |> Db.count
  Task.succeed ({ self: self.value, answer: r.answer, n: n } == { self: Err "not found", answer: Redirected (Teams.Show { id: team }), n: 0 })
}
EOF
alder new test Invites.Index "accepting makes the team visible, declining does not"
alder fill 'Invites.Index."accepting makes the team visible, declining does not"' - <<'EOF'
{
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let cat <- Db.insert User { email: Text.Email "cat@example.com", name: Text.NonEmpty "Cat", passwordHash: "" }
  let red <- Db.insert Team { name: Text.NonEmpty "Red", owner: ann }
  let blue <- Db.insert Team { name: Text.NonEmpty "Blue", owner: ann }
  let inv1 <- Db.insert Invite { team: red, email: Text.Email "bob@example.com" }
  let inv2 <- Db.insert Invite { team: blue, email: Text.Email "bob@example.com" }
  let _ <- Test.login { user: Just bob }
  let _ <- Test.get (Invites.Index {})
  let a <- Test.click (Accept { id: inv1 })
  let _ <- Test.get (Invites.Index {})
  let d <- Test.click (Decline { id: inv2 })
  let teams <- Test.get (Teams.Index {})
  let openRed <- Test.open (Teams.Show { id: red })
  let openBlue <- Test.open (Teams.Show { id: blue })
  Task.succeed ({ a: a.answer, d: d.answer, names: Test.texts "#teams .name" teams.doc, openRed: openRed.answer, openBlue: openBlue.answer } == { a: Redirected (Teams.Show { id: red }), d: Redirected (Invites.Index {}), names: ["Red"], openRed: Shown, openBlue: PageMissing })
}
EOF
alder fill --list
```
