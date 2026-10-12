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

--- file: app/Layout.alder ---
module Layout

Page : { title: Text, children: List (Html a) } -> Html a
Page props =
  <html>
    <head>
      <meta charset="utf-8" />
      <title>{props.title}</title>
    </head>
    <body>
      <nav>
        <a href={Teams.Index {}}>Teams</a>
        <a href={Invites.Index {}}>Invites</a>
        <button click={App.Logout}>Log out</button>
      </nav>
      <main>{props.children}</main>
    </body>
  </html>

--- file: app/Team.alder ---
module Team

import User

type TeamId = brand Text

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

table Invite {
  id: InviteId @id,
  team: Team.TeamId @ref Team,
  email: Text.Email,
  status: Status = Pending,
}

--- file: app/Auth/Login.alder ---
module Auth.Login

import User
import Layout

-- Password checking is outside the language: a capability implemented in Go.
capability Password {
  verify : { email: Text.Email, password: Text } -> Task (Maybe User.UserId),
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
    <form submit={Submit}>
      <Errors form />
      <Input field="email" label="Email" />
      <Errors field="email" />
      <Input field="password" label="Password" />
      <Errors field="password" />
      <button>Log in</button>
    </form>
  </Layout.Page>

--- file: app/Teams/Index.alder ---
module Teams.Index

import Team
import Member
import Layout

page "/teams" { } requires user

type Model = { teams: List Team }

load : Params -> Session -> Query Model
load params session = {
  let memberships <- Db.from Member |> Db.where (Member.user.eq session.user) |> Db.all
  let ids = List.map (\m -> m.team) memberships
  let teams <-
    Db.from Team
      |> Db.where (Db.or [Team.owner.eq session.user, Team.id.in ids])
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
    <form submit={Create}>
      <Input field="name" label="Team name" />
      <Errors field="name" />
      <button>Create</button>
    </form>
  </Layout.Page>

--- file: app/Teams/Show.alder ---
module Teams.Show

import Team
import User
import Member
import Invite
import Layout

page "/teams/{id}" { id: Team.TeamId } requires user

type Model = {
  team: Team,
  owner: User,
  members: List User,
  isOwner: Bool,
}

load : Params -> Session -> Query Model
load params session = {
  let found <- Db.get Team params.id
  match found {
    Nothing -> Query.notFound,
    Just team -> {
      let memberships <- Db.from Member |> Db.where (Member.team.eq team.id) |> Db.all
      let isMember = List.any (\m -> m.user == session.user) memberships
      let isOwner = team.owner == session.user
      let ownerRow <- Db.get User team.owner
      let others <-
        Db.from User
          |> Db.where (User.id.in (List.map (\m -> m.user) memberships))
          |> Db.orderBy User.name.asc
          |> Db.all
      match ownerRow {
        Nothing -> Query.notFound,
        Just owner ->
          if isOwner || isMember
            then Query.succeed { team: team, owner: owner, members: others, isOwner: isOwner }
            else Query.notFound,
      }
    },
  }
}

form NewInvite = { email: Text.Email }

msg SendInvite form NewInvite
msg Remove { user: User.UserId }

-- Only the team's owner may invite or remove; anyone else gets a 404.
requireOwner : Model -> Session -> Task Unit
requireOwner model session =
  if model.team.owner == session.user then Task.succeed () else Task.notFound

-- The owner cannot be removed (the owner is the only one who can remove).
notSelf : User.UserId -> Session -> Task Unit
notSelf target session =
  if target == session.user then Task.notFound else Task.succeed ()

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  SendInvite p -> {
    let _ <- requireOwner model session
    let _ <- Db.insert Invite { team: model.team.id, email: p.form.email }
    Task.succeed (Redirect (Teams.Show { id: model.team.id }))
  },
  Remove p -> {
    let _ <- requireOwner model session
    let _ <- notSelf p.user session
    let rows <-
      Db.from Member
        |> Db.where (Member.team.eq model.team.id)
        |> Db.where (Member.user.eq p.user)
        |> Db.all
    let _ <- Task.all (List.map (\m -> Db.delete Member m.id) rows)
    Task.succeed (Redirect (Teams.Show { id: model.team.id }))
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title={model.team.name}>
    <h1 id="name">{model.team.name}</h1>
    <h2>Members</h2>
    <ul id="members">
      <li><span class="name">{model.owner.name}</span></li>
      {for u in model.members ->
        <Row user={u} canRemove={model.isOwner && u.id != session.user} />}
    </ul>
    {if model.isOwner then
      <form submit={SendInvite}>
        <Input field="email" label="Invite by email" />
        <Errors field="email" />
        <button>Invite</button>
      </form>
    else Html.none}
  </Layout.Page>

Row : { user: User, canRemove: Bool } -> Html Msg
Row props =
  <li>
    <span class="name">{props.user.name}</span>
    {if props.canRemove then <button class="remove" click={Remove { user: props.user.id }}>Remove</button> else Html.none}
  </li>

test "a team's page is not found for a stranger but opens for a member" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let stranger <- Db.insert User { email: Text.Email "stranger@example.com", name: Text.NonEmpty "Stranger", passwordHash: "" }
  let member <- Db.insert User { email: Text.Email "member@example.com", name: Text.NonEmpty "Member", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Crew", owner: owner }
  let _ <- Db.insert Member { team: team, user: member }
  let _ <- Test.login { user: Just stranger }
  let a <- Test.open (Teams.Show { id: team })
  let _ <- Test.login { user: Just member }
  let b <- Test.open (Teams.Show { id: team })
  Task.succeed ({ stranger: a.answer, member: b.answer } == { stranger: PageMissing, member: Shown })
}

test "a member cannot remove another member" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let m1 <- Db.insert User { email: Text.Email "m1@example.com", name: Text.NonEmpty "One", passwordHash: "" }
  let m2 <- Db.insert User { email: Text.Email "m2@example.com", name: Text.NonEmpty "Two", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Crew", owner: owner }
  let _ <- Db.insert Member { team: team, user: m1 }
  let _ <- Db.insert Member { team: team, user: m2 }
  let _ <- Test.login { user: Just m1 }
  let page <- Test.get (Teams.Show { id: team })
  let r <- Test.run (update (Remove { user: m2 }) page.model { user: m1 })
  let rows <- Db.from Member |> Db.where (Member.team.eq team) |> Db.count
  Task.succeed ({ value: r.value, members: rows, buttons: Test.count "button.remove" page.doc } == { value: Err "not found", members: 2, buttons: 0 })
}

test "the owner removes a member but cannot remove themselves" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let m <- Db.insert User { email: Text.Email "m@example.com", name: Text.NonEmpty "Mem", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Crew", owner: owner }
  let _ <- Db.insert Member { team: team, user: m }
  let _ <- Test.login { user: Just owner }
  let page <- Test.get (Teams.Show { id: team })
  let removed <- Test.click (Remove { user: m })
  let after <- Db.from Member |> Db.where (Member.team.eq team) |> Db.count
  let self <- Test.run (update (Remove { user: owner }) page.model { user: owner })
  Task.succeed ({ answer: removed.answer, listed: Test.count "#members li" removed.doc, members: after, self: self.value } == { answer: Redirected (Teams.Show { id: team }), listed: 1, members: 0, self: Err "not found" })
}

--- file: app/Invites/Index.alder ---
module Invites.Index

import Team
import User
import Member
import Invite
import Layout

page "/invites" { } requires user

type Model = {
  invites: List Invite,
  teams: List Team,
}

load : Params -> Session -> Query Model
load params session = {
  let me <- Db.get User session.user
  match me {
    Nothing -> Query.succeed { invites: [], teams: [] },
    Just user -> {
      let invites <-
        Db.from Invite
          |> Db.where (Invite.email.eq user.email)
          |> Db.where (Invite.status.eq Pending)
          |> Db.all
      let teams <- Db.from Team |> Db.where (Team.id.in (List.map (\i -> i.team) invites)) |> Db.all
      Query.succeed { invites: invites, teams: teams }
    },
  }
}

msg Accept { id: Invite.InviteId }
msg Decline { id: Invite.InviteId }

-- The invite a message names, when it is to the session user's email and still waiting;
-- otherwise the message is a 404.
mine : Invite.InviteId -> Session -> Task Invite
mine id session = {
  let found <- Db.get Invite id
  match found {
    Nothing -> Task.notFound,
    Just inv -> {
      let me <- Db.get User session.user
      match me {
        Nothing -> Task.notFound,
        Just user ->
          if inv.email == user.email && inv.status == Pending
            then Task.succeed inv
            else Task.notFound,
      }
    },
  }
}

-- Makes the session user a member of the team, unless already its owner or a member.
join : Team.TeamId -> Session -> Task Unit
join team session = {
  let found <- Db.get Team team
  let existing <-
    Db.from Member
      |> Db.where (Member.team.eq team)
      |> Db.where (Member.user.eq session.user)
      |> Db.first
  match found {
    Nothing -> Task.notFound,
    Just t ->
      if t.owner == session.user || Maybe.isJust existing
        then Task.succeed ()
        else Task.map (\id -> ()) (Db.insert Member { team: team, user: session.user }),
  }
}

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Accept p -> {
    let inv <- mine p.id session
    let _ <- Db.update Invite inv.id (\i -> { i | status: Accepted })
    let _ <- join inv.team session
    Task.succeed (Redirect (Teams.Show { id: inv.team }))
  },
  Decline p -> {
    let inv <- mine p.id session
    let _ <- Db.update Invite inv.id (\i -> { i | status: Declined })
    Task.succeed (Redirect (Invites.Index {}))
  },
}

teamName : List Team -> Invite -> Text
teamName teams inv = {
  let found = List.find (\t -> t.id == inv.team) teams
  match found {
    Just t -> t.name,
    Nothing -> "",
  }
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Invites">
    <h1>Invites</h1>
    <ul id="invites">
      {for inv in model.invites ->
        <li>
          <span class="team">{teamName model.teams inv}</span>
          <button click={Accept { id: inv.id }}>Accept</button>
          <button click={Decline { id: inv.id }}>Decline</button>
        </li>}
    </ul>
  </Layout.Page>

test "accepting an invite makes the team visible, declining does not" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let guest <- Db.insert User { email: Text.Email "guest@example.com", name: Text.NonEmpty "Guest", passwordHash: "" }
  let t1 <- Db.insert Team { name: Text.NonEmpty "Alpha", owner: owner }
  let t2 <- Db.insert Team { name: Text.NonEmpty "Beta", owner: owner }
  let i1 <- Db.insert Invite { team: t1, email: Text.Email "guest@example.com" }
  let i2 <- Db.insert Invite { team: t2, email: Text.Email "guest@example.com" }
  let _ <- Test.login { user: Just guest }
  let _ <- Test.get (Invites.Index {})
  let acc <- Test.click (Accept { id: i1 })
  let _ <- Test.get (Invites.Index {})
  let dec <- Test.click (Decline { id: i2 })
  let page <- Test.get (Teams.Index {})
  let a <- Test.open (Teams.Show { id: t1 })
  let b <- Test.open (Teams.Show { id: t2 })
  Task.succeed ({ accepted: acc.answer, declined: dec.answer, waiting: Test.count "#invites li" dec.doc, teams: Test.texts "#teams .name" page.doc, alpha: a.answer, beta: b.answer } == { accepted: Redirected (Teams.Show { id: t1 }), declined: Redirected (Invites.Index {}), waiting: 0, teams: ["Alpha"], alpha: Shown, beta: PageMissing })
}
