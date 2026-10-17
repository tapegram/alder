```alder-cli
# the Invites view had the new msg buttons put back in; refill it without them
alder fill --decl Invites.Index.view - <<'EOF'
<Layout.Page title="Invites">
  <h1>Invites</h1>
  {viewBody model session}
</Layout.Page>
EOF

alder fill --decl 'Invites.Index."pressing Accept"' - <<'EOF'
{
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Red", owner: ann }
  let inv <- Db.insert Invite { team: team, email: Text.Email "bob@example.com" }
  let _ <- Test.login { user: Just bob }
  let _ <- Test.get (Invites.Index {})
  let r <- Test.click (Accept { id: inv })
  let teams <- Test.get (Teams.Index {})
  let show <- Test.open (Teams.Show { id: team })
  Task.succeed ({ answer: r.answer, teams: List.length teams.model.teams, show: show.answer } == { answer: Redirected (Teams.Show { id: team }), teams: 1, show: Shown })
}
EOF

alder fill --decl 'Invites.Index."pressing Decline"' - <<'EOF'
{
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Red", owner: ann }
  let inv <- Db.insert Invite { team: team, email: Text.Email "bob@example.com" }
  let _ <- Test.login { user: Just bob }
  let _ <- Test.get (Invites.Index {})
  let r <- Test.click (Decline { id: inv })
  let teams <- Test.get (Teams.Index {})
  let show <- Test.open (Teams.Show { id: team })
  let again <- Test.get (Invites.Index {})
  Task.succeed ({ answer: r.answer, teams: List.length teams.model.teams, show: show.answer, left: List.length again.model.items } == { answer: Redirected (Invites.Index {}), teams: 0, show: PageMissing, left: 0 })
}
EOF

alder fill --decl 'Teams.Index."submitting NewTeam"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Teams.Index {})
  let r <- Test.submit NewTeam { name: Text.NonEmpty "Red" }
  let t <- Db.from Team |> Db.first
  match t {
    Just x -> Task.succeed ({ answer: r.answer, owner: x.owner } == { answer: Redirected (Teams.Show { id: x.id }), owner: me }),
    Nothing -> Task.succeed False,
  }
}
EOF

alder fill --decl 'Teams.Show."submitting NewInvite"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let team <- Db.insert Team { name: Text.NonEmpty "Red", owner: me }
  let _ <- Test.get (Teams.Show { id: team })
  let r <- Test.submit NewInvite { email: Text.Email "bob@example.com" }
  let n <- Db.from Invite |> Db.where (Invite.email.eq (Text.Email "bob@example.com")) |> Db.count
  Task.succeed ({ answer: r.answer, n: n } == { answer: Redirected (Teams.Show { id: team }), n: 1 })
}
EOF

alder fill --decl 'Teams.Show."NewInvite refuses an empty email"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let team <- Db.insert Team { name: Text.NonEmpty "Red", owner: me }
  let _ <- Test.get (Teams.Show { id: team })
  let r <- Test.submitRaw NewInvite [{ name: "email", value: "" }]
  let n <- Db.from Invite |> Db.count
  match r.answer {
    FormRejected es -> Task.succeed (n == 0),
    _ -> Task.succeed False,
  }
}
EOF

alder fill --decl 'Teams.Show."pressing Remove"' - <<'EOF'
{
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Red", owner: ann }
  let _ <- Db.insert Member { team: team, user: bob }
  let _ <- Test.login { user: Just ann }
  let _ <- Test.get (Teams.Show { id: team })
  let r <- Test.click (Remove { user: bob })
  let n <- Db.from Member |> Db.count
  Task.succeed ({ answer: r.answer, n: n } == { answer: Redirected (Teams.Show { id: team }), n: 0 })
}
EOF

alder new test Teams.Show "a non-member gets a 404"
alder fill 'Teams.Show."a non-member gets a 404"' - <<'EOF'
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
  let carol <- Db.insert User { email: Text.Email "carol@example.com", name: Text.NonEmpty "Carol", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Red", owner: ann }
  let _ <- Db.insert Member { team: team, user: bob }
  let _ <- Db.insert Member { team: team, user: carol }
  let _ <- Test.login { user: Just ann }
  let _ <- Test.get (Teams.Show { id: team })
  let _ <- Test.login { user: Just bob }
  let r <- Test.click (Remove { user: carol })
  let n <- Db.from Member |> Db.count
  Task.succeed ({ answer: r.answer, n: n } == { answer: PageMissing, n: 2 })
}
EOF

alder new test Teams.Show "the owner cannot remove themselves"
alder fill 'Teams.Show."the owner cannot remove themselves"' - <<'EOF'
{
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Red", owner: ann }
  let _ <- Test.login { user: Just ann }
  let u = { id: ann, email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "", created: Time.epoch }
  let t = { id: team, name: Text.NonEmpty "Red", owner: ann }
  let r <- Test.run (update (Remove { user: ann }) { team: t, owner: u, members: [], isOwner: True } { user: ann })
  Task.succeed (r.value == Err "not found")
}
EOF

alder check
alder test
```
