```alder-cli
alder fill 'Auth.Login."logging in through the form"' --hole ?afterLogin - <<'EOF'
Teams.Index {}
EOF
alder fill 'Invites.Index."pressing Accept"' --hole ?expected - <<'EOF'
{ answer: r.answer, status: Maybe.map (\i -> i.status) after } == { answer: Redirected (Teams.Show { id: team1 }), status: Just Accepted }
EOF
alder fill 'Invites.Index."pressing Decline"' --hole ?expected - <<'EOF'
{ answer: r.answer, status: Maybe.map (\i -> i.status) after } == { answer: Redirected (Invites.Index {}), status: Just Declined }
EOF
alder fill 'Teams.Show."submitting NewInvite"' --hole ?expected - <<'EOF'
Redirected (Teams.Show { id: team1 })
EOF
alder fill --decl 'Teams.Index."submitting NewTeam"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Teams.Index {})
  let r <- Test.submit NewTeam { name: Text.NonEmpty "Red" }
  let made <- Db.from Team |> Db.first
  Task.succeed ({ answer: Just r.answer, owner: Maybe.map (\t -> t.owner) made } == { answer: Maybe.map (\t -> Redirected (Teams.Show { id: t.id })) made, owner: Just me })
}
EOF
alder fill --decl 'Teams.Show."pressing Remove"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let team1 <- Db.insert Team { name: Text.NonEmpty "name-1", owner: me }
  let _ <- Db.insert Member { team: team1, user: bob }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Teams.Show { id: team1 })
  let r <- Test.click (Remove { user: bob })
  let n <- Db.from Member |> Db.where (Member.team.eq team1) |> Db.count
  Task.succeed ({ answer: r.answer, members: n } == { answer: Redirected (Teams.Show { id: team1 }), members: 0 })
}
EOF
alder check
alder test
```
