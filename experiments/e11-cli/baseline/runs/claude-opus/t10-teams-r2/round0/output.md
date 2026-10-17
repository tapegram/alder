--- file: app/User.alder ---
module User

type UserId = brand Text

table User {
  id: UserId @id,
  email: Text.Email @unique,
  name: Text.NonEmpty,
  passwordHash: Text,
  created: Time @created,
}

--- file: app/Team.alder ---
module Team

import User

type TeamId = brand Text

-- The owner is the user who created the team. Other members have a `Member` row; the owner has none.
table Team {
  id: TeamId @id,
  name: Text.NonEmpty,
  owner: User.UserId @ref User,
}

--- file: app/Member.alder ---
module Member

import Team
import User

type MemberId = brand Text

-- One row per member of a team who is not its owner.
table Member {
  id: MemberId @id,
  team: Team.TeamId @ref Team,
  user: User.UserId @ref User,
}

--- file: app/Invite.alder ---
module Invite

import Team

type InviteId = brand Text

type Status = Pending | Accepted | Declined

-- An invite is to an email, whether or not a user has that email yet.
table Invite {
  id: InviteId @id,
  team: Team.TeamId @ref Team,
  email: Text.Email,
  status: Status = Pending,
}

--- file: app/App.alder ---
module App

import User
import Layout

session { user: Maybe User.UserId }

login = Auth.Login

msg Logout

update : Msg -> Session -> Task Nav
update msg session = match msg {
  Logout p -> {
    let _ <- Session.put (\s -> { s | user: Nothing })
    Task.succeed (Redirect (Auth.Login {}))
  },
}

error : Error -> Html Never
error err =
  <Layout.Page title="Error">
    <h1>Something went wrong</h1>
    <p>Please try again.</p>
  </Layout.Page>

notFound : Html Never
notFound =
  <Layout.Page title="Not found">
    <h1>Not found</h1>
  </Layout.Page>

test "Logout clears the user and goes to log in" = {
  let r <- Test.run (update Logout { user: Just (User.UserId "me") })
  Task.succeed ({ value: r.value, trace: r.trace } == { value: Ok (Redirect (Auth.Login {})), trace: ["Session.put"] })
}

--- file: app/Layout.alder ---
module Layout

Page : { title: Text, children: List (Html a) } -> Html a
Page props =
  <html>
    <head>
      <meta charset="utf-8" />
      <title>{props.title}</title>
      <link rel="stylesheet" href={Url.external "/static/al/al.css"} />
    </head>
    <body>
      <al-page width="narrow">
        <main>{props.children}</main>
      </al-page>
    </body>
  </html>

--- file: app/Auth/Login.alder ---
module Auth.Login

import User
import Layout

-- Password checking is outside the language: a capability implemented in Go.
capability Password {
  verify : { email: Text.Email, password: Text } -> Task (Maybe User.UserId),
}

-- In a test, `Test.fake Password` puts this in the adapter's place: it accepts any user who has
-- a row, whatever the password.
fake Password {
  verify args = {
    let found <- Db.from User |> Db.where (User.email.eq args.email) |> Db.first
    Task.succeed (Maybe.map (\u -> u.id) found)
  }
}

page "/login" { }

type Model = { }

load : Params -> Session -> Query Model
load params session = Query.succeed { }

form Credentials = {
  email: Text.Email,
  password: Text.Secret,
}

msg Submit form Credentials

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Submit p -> {
    let found <- Password.verify { email: p.form.email, password: p.form.password }
    match found {
      Just id -> {
        let _ <- Session.put (\s -> { s | user: Just id })
        Task.succeed (Redirect (Teams.Index {}))
      },
      Nothing -> Task.succeed (Reject "Email or password is incorrect."),
    }
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Log in">
    <h1>Log in</h1>
    <al-card>
      <form submit={Submit}>
        <Errors form />
        <al-input field="email" label="Email" />
        <al-input field="password" label="Password" />
        <al-button type="submit">Log in</al-button>
      </form>
    </al-card>
  </Layout.Page>

test "an unknown email is rejected" = {
  let _ <- Test.fake Password
  let _ <- Test.get (Auth.Login {})
  let r <- Test.submit Credentials { email: Text.Email "nobody@example.com", password: Text.Secret "secret" }
  Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: FormRejected [{ field: "", message: "Email or password is incorrect." }], user: Nothing })
}

test "logging in through the form" = {
  let _ <- Test.fake Password
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.get (Auth.Login {})
  let r <- Test.submit Credentials { email: Text.Email "me@example.com", password: Text.Secret "secret" }
  Task.succeed ({ answer: r.answer, user: r.session.user, title: Test.text "h1" r.doc } == { answer: Redirected (Teams.Index {}), user: Just me, title: Just "Teams" })
}

test "a visitor is sent to log in, and can log out" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let visitor <- Test.open (Teams.Index {})
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Teams.Index {})
  let out <- Test.click App.Logout
  Task.succeed ({ visitor: visitor.answer, out: out.answer, user: out.session.user } == { visitor: Redirected (Auth.Login {}), out: Redirected (Auth.Login {}), user: Nothing })
}

--- file: app/Teams/Index.alder ---
module Teams.Index

import Team
import Member
import User
import Layout

page "/teams" { } requires user

type Model = { teams: List Team }

-- The teams the user owns, and those they are a member of.
load : Params -> Session -> Query Model
load params session = {
  let memberships <- Db.from Member |> Db.where (Member.user.eq session.user) |> Db.all
  let teams <- Db.from Team
    |> Db.where (Db.or [Team.owner.eq session.user, Team.id.in (List.map (\m -> m.team) memberships)])
    |> Db.orderBy Team.name.asc
    |> Db.all
  Query.succeed { teams: teams }
}

form NewTeam = { name: Text.NonEmpty }

msg Create form NewTeam

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Create p -> {
    let id <- Db.insert Team { name: p.form.name, owner: session.user }
    Task.succeed (Redirect (Teams.Show { id: id }))
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Teams">
    <h1>Teams</h1>
    <ul id="teams">
      {for team in model.teams ->
        <li><a class="name" href={Teams.Show { id: team.id }}>{team.name}</a></li>}
    </ul>
    <al-card>
      <form submit={Create}>
        <al-input field="name" label="New team" />
        <al-button type="submit">Create</al-button>
      </form>
    </al-card>
    <p><a href={Invites.Index {}}>Invites</a></p>
    <al-button variant="outline" click={App.Logout}>Log out</al-button>
  </Layout.Page>

test "creating a team makes its creator the owner" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Teams.Index {})
  let r <- Test.submit NewTeam { name: Text.NonEmpty "Core" }
  let made <- Db.from Team |> Db.first
  let page <- Test.get (Teams.Index {})
  Task.succeed ({ answer: Just r.answer, owner: Maybe.map (\t -> t.owner) made, names: Test.texts "#teams .name" page.doc } == { answer: Maybe.map (\t -> Redirected (Teams.Show { id: t.id })) made, owner: Just me, names: ["Core"] })
}

test "the list has owned and joined teams, by name" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let _ <- Db.insert Team { name: Text.NonEmpty "Zeta", owner: me }
  let alpha <- Db.insert Team { name: Text.NonEmpty "Alpha", owner: other }
  let _ <- Db.insert Team { name: Text.NonEmpty "Mid", owner: other }
  let _ <- Db.insert Member { team: alpha, user: me }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Teams.Index {})
  Task.succeed (Test.texts "#teams .name" page.doc == ["Alpha", "Zeta"])
}

--- file: app/Teams/Show.alder ---
module Teams.Show

import Team
import Member
import Invite
import User
import Layout

page "/teams/{id}" { id: Team.TeamId } requires user

type Model = {
  team: Team,
  members: List User,
  isOwner: Bool,
}

-- Only the owner and the members may see the team: anyone else gets a 404.
load : Params -> Session -> Query Model
load params session = {
  let found <- Db.get Team params.id
  let memberRows <- Db.from Member |> Db.where (Member.team.eq params.id) |> Db.all
  let users <- Db.from User
    |> Db.where (User.id.in (List.map (\m -> m.user) memberRows))
    |> Db.orderBy User.name.asc
    |> Db.all
  match found {
    Nothing -> Query.notFound,
    Just team -> {
      let allowed = team.owner == session.user || List.any (\m -> m.user == session.user) memberRows
      if allowed then {
        let owner <- Db.get User team.owner
        Query.succeed { team: team, members: Maybe.toList owner ++ users, isOwner: team.owner == session.user }
      } else Query.notFound
    },
  }
}

form NewInvite = { email: Text.Email }

msg Remove { user: User.UserId }
msg SendInvite form NewInvite

-- Only the owner may remove or invite; anyone else is a 404. The owner cannot be removed.
update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Remove p -> {
    if session.user != model.team.owner || p.user == model.team.owner then Task.notFound else {
      let rows <- Db.from Member
        |> Db.where (Member.team.eq model.team.id)
        |> Db.where (Member.user.eq p.user)
        |> Db.all
      let _ <- Task.all (List.map (\m -> Db.delete Member m.id) rows)
      Task.succeed (Redirect (Teams.Show { id: model.team.id }))
    }
  },
  SendInvite p -> {
    if session.user != model.team.owner then Task.notFound else {
      let _ <- Db.insert Invite { team: model.team.id, email: p.form.email }
      Task.succeed (Redirect (Teams.Show { id: model.team.id }))
    }
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title={model.team.name}>
    <h1 id="name">{model.team.name}</h1>
    <ul id="members">
      {for u in model.members ->
        <li>
          <span class="name">{u.name}</span>
          {if model.isOwner && u.id != model.team.owner then <button class="remove" click={Remove { user: u.id }}>Remove</button> else Html.none}
        </li>}
    </ul>
    {if model.isOwner then
      <al-card>
        <form submit={SendInvite}>
          <al-input field="email" label="Invite by email" />
          <al-button type="submit">Invite</al-button>
        </form>
      </al-card>
    else Html.none}
    <p><a href={Teams.Index {}}>All teams</a></p>
    <al-button variant="outline" click={App.Logout}>Log out</al-button>
  </Layout.Page>

test "a team's page is not found for a user who is not a member" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let stranger <- Db.insert User { email: Text.Email "stranger@example.com", name: Text.NonEmpty "Stranger", passwordHash: "" }
  let t <- Db.insert Team { name: Text.NonEmpty "Core", owner: owner }
  let _ <- Db.insert Member { team: t, user: ann }
  let _ <- Test.login { user: Just stranger }
  let s <- Test.open (Teams.Show { id: t })
  let _ <- Test.login { user: Just ann }
  let a <- Test.open (Teams.Show { id: t })
  Task.succeed ({ stranger: s.answer, member: a.answer, name: Test.text "#name" a.doc } == { stranger: PageMissing, member: Shown, name: Just "Core" })
}

test "a member cannot remove another member" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let t <- Db.insert Team { name: Text.NonEmpty "Core", owner: owner }
  let _ <- Db.insert Member { team: t, user: ann }
  let _ <- Db.insert Member { team: t, user: bob }
  let _ <- Test.login { user: Just ann }
  let annPage <- Test.get (Teams.Show { id: t })
  let _ <- Test.login { user: Just owner }
  let _ <- Test.get (Teams.Show { id: t })
  let _ <- Test.login { user: Just ann }
  let r <- Test.click (Remove { user: bob })
  let left <- Db.from Member |> Db.where (Member.team.eq t) |> Db.count
  Task.succeed ({ buttons: Test.count ".remove" annPage.doc, answer: r.answer, left: left } == { buttons: 0, answer: PageMissing, left: 2 })
}

test "the owner removes a member but cannot remove themselves" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let t <- Db.insert Team { name: Text.NonEmpty "Core", owner: owner }
  let _ <- Db.insert Member { team: t, user: ann }
  let _ <- Test.login { user: Just owner }
  let page <- Test.get (Teams.Show { id: t })
  let removed <- Test.click (Remove { user: ann })
  let self <- Test.run (update (Remove { user: owner }) { team: { id: t, name: Text.NonEmpty "Core", owner: owner }, members: [], isOwner: True } { user: owner })
  let left <- Db.from Member |> Db.where (Member.team.eq t) |> Db.count
  let _ <- Test.login { user: Just ann }
  let annNow <- Test.open (Teams.Show { id: t })
  Task.succeed ({ names: Test.texts "#members .name" page.doc, buttons: Test.count ".remove" page.doc, removed: removed.answer, self: self.value, left: left, ann: annNow.answer } == { names: ["Owner", "Ann"], buttons: 1, removed: Redirected (Teams.Show { id: t }), self: Err "not found", left: 0, ann: PageMissing })
}

test "the owner invites by email, a member cannot" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let t <- Db.insert Team { name: Text.NonEmpty "Core", owner: owner }
  let _ <- Db.insert Member { team: t, user: ann }
  let _ <- Test.login { user: Just owner }
  let _ <- Test.get (Teams.Show { id: t })
  let sent <- Test.submit NewInvite { email: Text.Email "new@example.com" }
  let _ <- Test.login { user: Just ann }
  let refused <- Test.submit NewInvite { email: Text.Email "evil@example.com" }
  let invites <- Db.from Invite |> Db.all
  Task.succeed ({ sent: sent.answer, refused: refused.answer, invites: List.map (\i -> { email: i.email, status: i.status }) invites } == { sent: Redirected (Teams.Show { id: t }), refused: PageMissing, invites: [{ email: Text.Email "new@example.com", status: Pending }] })
}

--- file: app/Invites/Index.alder ---
module Invites.Index

import Invite
import Team
import Member
import User
import Layout

page "/invites" { } requires user

type Row = { id: Invite.InviteId, team: Text }

type Model = { invites: List Row }

toRow : List Team -> Invite -> Maybe Row
toRow teams invite = match List.find (\t -> t.id == invite.team) teams {
  Just t -> Just { id: invite.id, team: t.name },
  Nothing -> Nothing,
}

-- The invites waiting for the user: those to the user's email that are still Pending.
load : Params -> Session -> Query Model
load params session = {
  let me <- Db.get User session.user
  match me {
    Nothing -> Query.notFound,
    Just u -> {
      let invites <- Db.from Invite
        |> Db.where (Invite.email.eq u.email)
        |> Db.where (Invite.status.eq Pending)
        |> Db.all
      let teams <- Db.from Team |> Db.where (Team.id.in (List.map (\i -> i.team) invites)) |> Db.all
      Query.succeed { invites: List.filterMap (toRow teams) invites }
    },
  }
}

msg Accept { id: Invite.InviteId }
msg Decline { id: Invite.InviteId }

-- An invite can be answered only by the user it is to, and only while it is Pending: else 404.
update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Accept p -> {
    let found <- Db.get Invite p.id
    let me <- Db.get User session.user
    match found {
      Nothing -> Task.notFound,
      Just invite -> {
        let mine = Maybe.map (\u -> u.email) me == Just invite.email
        if mine && invite.status == Pending then {
          let team <- Db.get Team invite.team
          let existing <- Db.from Member
            |> Db.where (Member.team.eq invite.team)
            |> Db.where (Member.user.eq session.user)
            |> Db.count
          let already = existing > 0 || Maybe.map (\t -> t.owner) team == Just session.user
          let _ <- Db.update Invite invite.id (\i -> { i | status: Accepted })
          let _ <- if already then Task.succeed () else Task.map (\x -> ()) (Db.insert Member { team: invite.team, user: session.user })
          Task.succeed (Redirect (Teams.Show { id: invite.team }))
        } else Task.notFound
      },
    }
  },
  Decline p -> {
    let found <- Db.get Invite p.id
    let me <- Db.get User session.user
    match found {
      Nothing -> Task.notFound,
      Just invite -> {
        let mine = Maybe.map (\u -> u.email) me == Just invite.email
        if mine && invite.status == Pending then {
          let _ <- Db.update Invite invite.id (\i -> { i | status: Declined })
          Task.succeed (Redirect (Invites.Index {}))
        } else Task.notFound
      },
    }
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Invites">
    <h1>Invites</h1>
    <ul id="invites">
      {for row in model.invites ->
        <li>
          <span class="team">{row.team}</span>
          <button class="accept" click={Accept { id: row.id }}>Accept</button>
          <button class="decline" click={Decline { id: row.id }}>Decline</button>
        </li>}
    </ul>
    <p><a href={Teams.Index {}}>Teams</a></p>
    <al-button variant="outline" click={App.Logout}>Log out</al-button>
  </Layout.Page>

test "accepting an invite makes the team visible" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let t <- Db.insert Team { name: Text.NonEmpty "Core", owner: owner }
  let inv <- Db.insert Invite { team: t, email: Text.Email "me@example.com" }
  let _ <- Test.login { user: Just me }
  let before <- Test.get (Invites.Index {})
  let r <- Test.click (Accept { id: inv })
  let teams <- Test.get (Teams.Index {})
  let after <- Test.get (Invites.Index {})
  Task.succeed ({ before: Test.texts "#invites .team" before.doc, answer: r.answer, teams: Test.texts "#teams .name" teams.doc, after: Test.count "#invites li" after.doc } == { before: ["Core"], answer: Redirected (Teams.Show { id: t }), teams: ["Core"], after: 0 })
}

test "declining an invite does not" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let t <- Db.insert Team { name: Text.NonEmpty "Core", owner: owner }
  let inv <- Db.insert Invite { team: t, email: Text.Email "me@example.com" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Invites.Index {})
  let r <- Test.click (Decline { id: inv })
  let teams <- Test.get (Teams.Index {})
  let show <- Test.open (Teams.Show { id: t })
  let again <- Test.run (update (Accept { id: inv }) { invites: [] } { user: me })
  Task.succeed ({ answer: r.answer, list: Test.count "#invites li" r.doc, teams: Test.count "#teams li" teams.doc, show: show.answer, again: again.value } == { answer: Redirected (Invites.Index {}), list: 0, teams: 0, show: PageMissing, again: Err "not found" })
}

test "an invite to someone else cannot be answered" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let t <- Db.insert Team { name: Text.NonEmpty "Core", owner: owner }
  let inv <- Db.insert Invite { team: t, email: Text.Email "me@example.com" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Invites.Index {})
  let _ <- Test.login { user: Just other }
  let r <- Test.click (Accept { id: inv })
  let members <- Db.from Member |> Db.count
  let found <- Db.get Invite inv
  Task.succeed ({ answer: r.answer, members: members, status: Maybe.map (\i -> i.status) found } == { answer: PageMissing, members: 0, status: Just Pending })
}
