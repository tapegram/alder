```alder-cli
# the login target
alder fill Auth.Login.update.Submit - --hole ?afterLogin <<'EOF'
Teams.Index {}
EOF
alder fill 'Auth.Login."logging in through the form"' - --hole ?afterLogin <<'EOF'
Teams.Index {}
EOF

# Teams.Index
alder fill Teams.Index.load - <<'EOF'
{
  let owned <- Db.from Team |> Db.where (Team.owner.eq session.user) |> Db.all
  let mems <- Db.from Member |> Db.where (Member.user.eq session.user) |> Db.all
  let joined <- Db.from Team |> Db.where (Team.id.in (List.map (\m -> m.team) mems)) |> Db.all
  Query.succeed { teams: List.sortByText (\t -> t.name) (owned ++ joined) }
}
EOF
alder new decl Teams.Index viewBody "Model -> Session -> Html Msg"
alder fill Teams.Index.view - <<'EOF'
viewBody model session
EOF

# Teams.Show
alder fill --decl Teams.Show.Model - <<'EOF'
{ team: Team, owner: User, members: List User, isOwner: Bool }
EOF
alder fill Teams.Show.load - <<'EOF'
{
  let found <- Db.get Team params.id
  match found {
    Nothing -> Query.notFound,
    Just team -> {
      let rows <- Db.from Member |> Db.where (Member.team.eq team.id) |> Db.all
      let ownerRow <- Db.get User team.owner
      let users <- Db.from User |> Db.where (User.id.in (List.map (\m -> m.user) rows)) |> Db.all
      let isMember = List.any (\m -> m.user == session.user) rows
      if team.owner != session.user && not isMember then Query.notFound else
        match ownerRow {
          Nothing -> Query.notFound,
          Just o -> Query.succeed { team: team, owner: o, members: List.sortByText (\u -> u.name) users, isOwner: team.owner == session.user },
        }
    },
  }
}
EOF
alder new decl Teams.Show viewBody "Model -> Session -> Html Msg"
alder fill Teams.Show.view - <<'EOF'
viewBody model session
EOF

# Invites.Index
alder fill --decl Invites.Index.Model - <<'EOF'
{ items: List { id: Invite.InviteId, team: Text } }
EOF
alder fill Invites.Index.load - <<'EOF'
{
  let me <- Db.get User session.user
  match me {
    Nothing -> Query.succeed { items: [] },
    Just u -> {
      let invs <- Db.from Invite |> Db.where (Invite.email.eq u.email) |> Db.where (Invite.status.eq Pending) |> Db.all
      let teams <- Db.from Team |> Db.where (Team.id.in (List.map (\i -> i.team) invs)) |> Db.all
      Query.succeed { items: List.filterMap (\i -> Maybe.map (\t -> { id: i.id, team: t.name }) (List.find (\t -> t.id == i.team) teams)) invs }
    },
  }
}
EOF
alder new decl Invites.Index viewBody "Model -> Session -> Html Msg"
alder fill Invites.Index.view - <<'EOF'
viewBody model session
EOF

# forms and messages
alder new form Teams.Index NewTeam name:Text.NonEmpty --msg Create
alder new msg Teams.Show Remove user:User.UserId
alder new form Teams.Show NewInvite email:Text.Email --msg SendInvite
alder new msg Invites.Index Accept id:Invite.InviteId
alder new msg Invites.Index Decline id:Invite.InviteId
alder fill --list
```
