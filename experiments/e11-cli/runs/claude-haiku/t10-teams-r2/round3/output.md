```alder-cli
# helpers the pages call: users, teams, members, invites
alder new decl User add "Text.Email -> Text.NonEmpty -> Task UserId"
alder fill --decl User.add - <<'EOF'
\email name -> Db.insert User { email: email, name: name, passwordHash: "" }
EOF
alder new decl Team make "User.UserId -> Text.NonEmpty -> Task TeamId"
alder fill --decl Team.make - <<'EOF'
\owner name -> Db.insert Team { name: name, owner: owner }
EOF

alder new type Member "Person = { user: User.UserId, name: Text }"
alder new type Member "Roster = { name: Text.NonEmpty, owner: User.UserId, people: List Person }"
alder new decl Member person "User -> Person"
alder fill --decl Member.person - <<'EOF'
\u -> { user: u.id, name: u.name }
EOF
alder new decl Member teamsFor "User.UserId -> Query (List Team)"
alder fill --decl Member.teamsFor - <<'EOF'
\user -> {
  let owned <- Db.from Team |> Db.where (Team.owner.eq user) |> Db.all
  let memberships <- Db.from Member |> Db.where (Member.user.eq user) |> Db.all
  let joined <- memberships |> List.map (\m -> Db.get Team m.team) |> Query.all
  Query.succeed (owned ++ List.filterMap (\x -> x) joined)
}
EOF
alder new decl Member roster "Team.TeamId -> User.UserId -> Query (Maybe Roster)"
alder fill --decl Member.roster - <<'EOF'
\team user -> {
  let found <- Db.get Team team
  let mine <- Db.from Member |> Db.where (Member.team.eq team) |> Db.where (Member.user.eq user) |> Db.first
  let ownerRow <- match found {
    Just t -> Db.get User t.owner,
    Nothing -> Query.succeed Nothing,
  }
  let memberRows <- Db.from Member |> Db.where (Member.team.eq team) |> Db.all
  let others <- memberRows |> List.map (\m -> Db.get User m.user) |> Query.all
  let ownerPeople : List Person = [ownerRow] |> List.filterMap (\x -> x) |> List.map person
  let memberPeople : List Person = others |> List.filterMap (\x -> x) |> List.map person |> List.sortByText (\p -> p.name)
  let people : List Person = ownerPeople ++ memberPeople
  let ok = Maybe.withDefault False (Maybe.map (\t -> t.owner == user) found) || Maybe.isJust mine
  match found {
    Just t -> {
      let roster : Roster = { name: t.name, owner: t.owner, people: people }
      Query.succeed (if ok then Just roster else Nothing)
    },
    Nothing -> Query.succeed Nothing,
  }
}
EOF
alder new decl Member add "Team.TeamId -> User.UserId -> Task Unit"
alder fill --decl Member.add - <<'EOF'
\team user -> {
  let _ <- Db.insert Member { team: team, user: user }
  Task.succeed ()
}
EOF
alder new decl Member removeMember "Team.TeamId -> User.UserId -> Task Unit"
alder fill --decl Member.removeMember - <<'EOF'
\team user -> {
  let rows <- Db.from Member |> Db.where (Member.team.eq team) |> Db.where (Member.user.eq user) |> Db.all
  let gone = rows |> List.map (\m -> Db.delete Member m.id)
  let _ <- Task.all gone
  Task.succeed ()
}
EOF

alder new type Invite "Waiting = { id: InviteId, team: Text }"
alder new decl Invite pendingOf "User.UserId -> InviteId -> Query (Maybe Invite)"
alder fill --decl Invite.pendingOf - <<'EOF'
\user id -> {
  let me <- Db.get User user
  let found <- match me {
    Just u -> Db.from Invite |> Db.where (Invite.id.eq id) |> Db.where (Invite.email.eq u.email) |> Db.where (Invite.status.eq Pending) |> Db.first,
    Nothing -> Query.succeed Nothing,
  }
  Query.succeed found
}
EOF
alder new decl Invite waitingOne "Invite -> Query (Maybe Waiting)"
alder fill --decl Invite.waitingOne - <<'EOF'
\inv -> {
  let found <- Db.get Team inv.team
  match found {
    Just t -> {
      let w : Waiting = { id: inv.id, team: t.name }
      Query.succeed (Just w)
    },
    Nothing -> Query.succeed Nothing,
  }
}
EOF
alder new decl Invite waitingFor "User.UserId -> Query (List Waiting)"
alder fill --decl Invite.waitingFor - <<'EOF'
\user -> {
  let me <- Db.get User user
  let invites <- match me {
    Just u -> Db.from Invite |> Db.where (Invite.email.eq u.email) |> Db.where (Invite.status.eq Pending) |> Db.all,
    Nothing -> Query.succeed [],
  }
  let found <- invites |> List.map waitingOne |> Query.all
  Query.succeed (found |> List.filterMap (\x -> x))
}
EOF
alder new decl Invite send "Team.TeamId -> Text.Email -> Task InviteId"
alder fill --decl Invite.send - <<'EOF'
\team email -> Db.insert Invite { team: team, email: email }
EOF
alder new decl Invite accept "User.UserId -> InviteId -> Task Team.TeamId"
alder fill --decl Invite.accept - <<'EOF'
\user id -> {
  let found <- pendingOf user id
  match found {
    Nothing -> Task.notFound,
    Just inv -> {
      let _ <- Db.update Invite inv.id (\i -> { i | status: Accepted })
      let _ <- Member.add inv.team user
      Task.succeed inv.team
    },
  }
}
EOF
alder new decl Invite decline "User.UserId -> InviteId -> Task Unit"
alder fill --decl Invite.decline - <<'EOF'
\user id -> {
  let found <- pendingOf user id
  match found {
    Nothing -> Task.notFound,
    Just inv -> {
      let _ <- Db.update Invite inv.id (\i -> { i | status: Declined })
      Task.succeed ()
    },
  }
}
EOF

# Teams.Index: regenerate its body (its list-mode model is replaced)
alder fill --decl Teams.Index.load - <<'EOF'
{
  let rows <- Member.teamsFor session.user
  Query.succeed { rows: rows |> List.sortByText (\t -> t.name), page: 1, pages: 1 }
}
EOF
alder new form Teams.Index NewTeam name:Text.NonEmpty --msg Create
alder fill --decl Teams.Index.update - <<'EOF'
match msg {
  Create p -> {
    let team <- Team.make session.user p.form.name
    Task.succeed (Redirect (Teams.Show { id: team }))
  },
}
EOF
alder fill --decl Teams.Index.view - <<'EOF'
<Layout.Page title="Teams">
  <h1>Teams</h1>
  <ul id="teams">
    {for team in model.rows ->
      <li><a class="name" href={Teams.Show { id: team.id }}>{team.name}</a></li>}
  </ul>
  <al-card>
    <form submit={Create}>
      <al-input field="name" label="Team name" />
      <al-button type="submit">Create team</al-button>
    </form>
  </al-card>
  <al-button variant="outline" click={App.Logout}>Log out</al-button>
</Layout.Page>
EOF

# Teams.Show
alder fill --decl Teams.Show.Model - <<'EOF'
{ team: Team.TeamId, roster: Member.Roster, viewer: User.UserId }
EOF
alder fill --decl Teams.Show.load - <<'EOF'
{
  let found <- Member.roster params.id session.user
  match found {
    Just r -> Query.succeed { team: params.id, roster: r, viewer: session.user },
    Nothing -> Query.notFound,
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
alder fill --decl Teams.Show.update - <<'EOF'
match msg {
  Remove p -> {
    let ok = session.user == model.roster.owner && p.user != model.roster.owner
    let _ <- if ok then Member.removeMember model.team p.user else Task.notFound
    Task.succeed (Redirect (Teams.Show { id: model.team }))
  },
  SendInvite p -> {
    let ok = session.user == model.roster.owner
    let _ <- if ok then Invite.send model.team p.form.email else Task.notFound
    Task.succeed (Redirect (Teams.Show { id: model.team }))
  },
}
EOF
alder fill --decl Teams.Show.view - <<'EOF'
<Layout.Page title={model.roster.name}>
  <h1 id="name">{model.roster.name}</h1>
  <ul id="members">
    {for p in model.roster.people ->
      <li>
        <span class="name">{p.name}</span>
        {if model.roster.owner == model.viewer && p.user != model.roster.owner then <button class="remove" click={Remove { user: p.user }}>Remove</button> else Html.none}
      </li>}
  </ul>
  {if model.roster.owner == model.viewer then <form submit={SendInvite}>
      <al-input field="email" label="Invite by email" />
      <al-button type="submit">Invite</al-button>
    </form> else Html.none}
  <a href={Teams.Index {}}>Back to teams</a>
</Layout.Page>
EOF

# Invites.Index: it was generated in list mode; regenerate it blank
alder remove page Invites.Index
alder new page Invites.Index /invites --blank --requires user
alder fill --decl Invites.Index.Model - <<'EOF'
{ rows: List Invite.Waiting }
EOF
alder fill --decl Invites.Index.load - <<'EOF'
{
  let rows <- Invite.waitingFor session.user
  Query.succeed { rows: rows }
}
EOF
alder fill --decl Invites.Index.view - <<'EOF'
<Layout.Page title="Invites">
  <h1>Invites</h1>
</Layout.Page>
EOF
alder new msg Invites.Index Accept id:Invite.InviteId
alder new msg Invites.Index Decline id:Invite.InviteId
alder fill --decl Invites.Index.update - <<'EOF'
match msg {
  Accept p -> {
    let team <- Invite.accept session.user p.id
    Task.succeed (Redirect (Teams.Show { id: team }))
  },
  Decline p -> {
    let _ <- Invite.decline session.user p.id
    Task.succeed (Redirect (Invites.Index {}))
  },
}
EOF
alder fill --decl Invites.Index.view - <<'EOF'
<Layout.Page title="Invites">
  <h1>Invites</h1>
  <ul id="invites">
    {for inv in model.rows ->
      <li>
        <span class="team">{inv.team}</span>
        <al-button click={Accept { id: inv.id }}>Accept</al-button>
        <al-button variant="ghost" click={Decline { id: inv.id }}>Decline</al-button>
      </li>}
  </ul>
</Layout.Page>
EOF

# tests: Teams.Show
alder new test Teams.Show "a stranger gets a 404 for the team"
alder fill 'Teams.Show."a stranger gets a 404 for the team"' - <<'EOF'
{
  let owner <- User.add (Text.Email "owner@example.com") (Text.NonEmpty "Owner")
  let stranger <- User.add (Text.Email "stranger@example.com") (Text.NonEmpty "Stranger")
  let team <- Team.make owner (Text.NonEmpty "Red")
  let _ <- Test.login { user: Just stranger }
  let r <- Test.open (Teams.Show { id: team })
  Task.succeed (r.answer == PageMissing)
}
EOF
alder new test Teams.Show "a member cannot remove another member"
alder fill 'Teams.Show."a member cannot remove another member"' - <<'EOF'
{
  let owner <- User.add (Text.Email "owner@example.com") (Text.NonEmpty "Owner")
  let ann <- User.add (Text.Email "ann@example.com") (Text.NonEmpty "Ann")
  let bob <- User.add (Text.Email "bob@example.com") (Text.NonEmpty "Bob")
  let team <- Team.make owner (Text.NonEmpty "Red")
  let _ <- Member.add team ann
  let _ <- Member.add team bob
  let _ <- Test.login { user: Just ann }
  let page <- Test.get (Teams.Show { id: team })
  let r <- Test.run (update (Remove { user: bob }) page.model { user: ann })
  let found <- Member.roster team owner
  Task.succeed ({ value: r.value, people: Maybe.map (\x -> List.length x.people) found } == { value: Err "not found", people: Just 3 })
}
EOF
alder new test Teams.Show "the owner removes a member"
alder fill 'Teams.Show."the owner removes a member"' - <<'EOF'
{
  let owner <- User.add (Text.Email "owner@example.com") (Text.NonEmpty "Owner")
  let ann <- User.add (Text.Email "ann@example.com") (Text.NonEmpty "Ann")
  let team <- Team.make owner (Text.NonEmpty "Red")
  let _ <- Member.add team ann
  let _ <- Test.login { user: Just owner }
  let _ <- Test.get (Teams.Show { id: team })
  let r <- Test.click (Remove { user: ann })
  let found <- Member.roster team owner
  Task.succeed ({ answer: r.answer, people: Maybe.map (\x -> List.length x.people) found } == { answer: Redirected (Teams.Show { id: team }), people: Just 1 })
}
EOF
alder new test Teams.Show "the owner cannot remove themselves"
alder fill 'Teams.Show."the owner cannot remove themselves"' - <<'EOF'
{
  let owner <- User.add (Text.Email "owner@example.com") (Text.NonEmpty "Owner")
  let ann <- User.add (Text.Email "ann@example.com") (Text.NonEmpty "Ann")
  let team <- Team.make owner (Text.NonEmpty "Red")
  let _ <- Member.add team ann
  let _ <- Test.login { user: Just owner }
  let page <- Test.get (Teams.Show { id: team })
  let r <- Test.run (update (Remove { user: owner }) page.model { user: owner })
  let found <- Member.roster team owner
  Task.succeed ({ value: r.value, people: Maybe.map (\x -> List.length x.people) found } == { value: Err "not found", people: Just 2 })
}
EOF

# tests: Invites.Index
alder new test Invites.Index "accepting an invite shows the team"
alder fill 'Invites.Index."accepting an invite shows the team"' - <<'EOF'
{
  let owner <- User.add (Text.Email "owner@example.com") (Text.NonEmpty "Owner")
  let ann <- User.add (Text.Email "ann@example.com") (Text.NonEmpty "Ann")
  let team <- Team.make owner (Text.NonEmpty "Red")
  let invite <- Invite.send team (Text.Email "ann@example.com")
  let _ <- Test.login { user: Just ann }
  let _ <- Test.get (Invites.Index {})
  let r <- Test.click (Accept { id: invite })
  let teams <- Test.get (Teams.Index {})
  let shown <- Test.get (Teams.Show { id: team })
  Task.succeed ({ answer: r.answer, listed: Test.texts "#teams .name" teams.doc, name: Test.text "#name" shown.doc } == { answer: Redirected (Teams.Show { id: team }), listed: ["Red"], name: Just "Red" })
}
EOF
alder new test Invites.Index "declining an invite keeps the team closed"
alder fill 'Invites.Index."declining an invite keeps the team closed"' - <<'EOF'
{
  let owner <- User.add (Text.Email "owner@example.com") (Text.NonEmpty "Owner")
  let ann <- User.add (Text.Email "ann@example.com") (Text.NonEmpty "Ann")
  let team <- Team.make owner (Text.NonEmpty "Red")
  let invite <- Invite.send team (Text.Email "ann@example.com")
  let _ <- Test.login { user: Just ann }
  let _ <- Test.get (Invites.Index {})
  let r <- Test.click (Decline { id: invite })
  let teams <- Test.get (Teams.Index {})
  let shown <- Test.open (Teams.Show { id: team })
  Task.succeed ({ answer: r.answer, listed: Test.texts "#teams .name" teams.doc, shown: shown.answer } == { answer: Redirected (Invites.Index {}), listed: [], shown: PageMissing })
}
EOF
```
