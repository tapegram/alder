```alder-cli
alder fill 'Teams.Show."the owner cannot remove themselves"' - <<'EOF'
{
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Red", owner: ann }
  let _ <- Test.login { user: Just ann }
  let u : User = { id: ann, email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "", created: Time.epoch }
  let t : Team = { id: team, name: Text.NonEmpty "Red", owner: ann }
  let r <- Test.run (update (Remove { user: ann }) { team: t, owner: u, members: [], isOwner: True } { user: ann })
  Task.succeed (r.value == Err "not found")
}
EOF
alder check
alder test
```
