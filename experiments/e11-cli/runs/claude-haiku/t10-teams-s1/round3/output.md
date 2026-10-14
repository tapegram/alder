```alder-cli
# the login's test: the page the browser reaches after logging in
alder fill 'Auth.Login."logging in through the form"' - --hole ?afterLogin <<'EOF'
Teams.Index {}
EOF

# accepting: the owner is the signed-in user here, so Accept only answers the redirect and marks the invite
alder fill 'Invites.Index."pressing Accept"' - --hole ?expected <<'EOF'
(r.answer == Redirected (Teams.Show { id: team1 })) && Maybe.map (\i -> i.status) after == Just Accepted
EOF

alder fill 'Invites.Index."pressing Decline"' - --hole ?expected <<'EOF'
(r.answer == Redirected (Invites.Index {})) && Maybe.map (\i -> i.status) after == Just Declined
EOF

alder fill 'Teams.Show."submitting NewInvite"' - --hole ?expected <<'EOF'
Redirected (Teams.Show { id: team1 })
EOF

# the team's id is not a test variable, so these bodies are replaced whole
alder fill --decl 'Teams.Index."submitting NewTeam"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Teams.Index {})
  let r <- Test.submit NewTeam { name: Text.NonEmpty "Red" }
  let found <- Db.from Team |> Db.where (Team.owner.eq me) |> Db.first
  let target = Maybe.map (\t -> Redirected (Teams.Show { id: t.id })) found
  Task.succeed (target == Just (r.answer) && Maybe.map (\t -> t.name) found == Just (Text.NonEmpty "Red"))
}
EOF

alder fill --decl 'Teams.Index."NewTeam refuses an empty name"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Teams.Index {})
  let r <- Test.submitRaw NewTeam [{ name: "name", value: "" }]
  Task.succeed (match r.answer { FormRejected es -> List.length es == 1, _ -> False })
}
EOF

alder fill --decl 'Teams.Show."NewInvite refuses an empty email"' - <<'EOF'
{
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owen", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Red", owner: owner }
  let _ <- Test.login { user: Just owner }
  let _ <- Test.get (Teams.Show { id: team })
  let r <- Test.submitRaw NewInvite [{ name: "email", value: "" }]
  Task.succeed (match r.answer { FormRejected es -> List.length es == 1, _ -> False })
}
EOF

# pressing Remove: the owner has a button for a member, and the click removes that member
alder fill --decl 'Teams.Show."pressing Remove"' - <<'EOF'
{
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owen", passwordHash: "" }
  let a <- Db.insert User { email: Text.Email "a@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Red", owner: owner }
  let _ <- Db.insert Member { team: team, user: a }
  let _ <- Test.login { user: Just owner }
  let page <- Test.get (Teams.Show { id: team })
  let r <- Test.click (Remove { user: a })
  let left <- Db.from Member |> Db.where (Member.team.eq team) |> Db.count
  Task.succeed ({ answer: r.answer, buttons: Test.count "button.remove" page.doc, left: left } == { answer: Redirected (Teams.Show { id: team }), buttons: 1, left: 0 })
}
EOF
```
