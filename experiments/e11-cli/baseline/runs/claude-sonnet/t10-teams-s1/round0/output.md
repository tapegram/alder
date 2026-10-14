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
      <main>{props.children}</main>
    </body>
  </html>

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

--- file: app/Auth/Login.alder ---
module Auth.Login

import User
import Layout

capability Password {
  verify : { email: Text.Email, password: Text } -> Task (Maybe User.UserId),
}

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
    <form submit={Submit}>
      <Errors form />
      <Input field="email" label="Email" />
      <Input field="password" label="Password" />
      <button>Log in</button>
    </form>
  </Layout.Page>

test "logging in through the form" = {
  let _ <- Test.fake Password
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.get (Auth.Login {})
  let r <- Test.submit Credentials { email: Text.Email "me@example.com", password: Text.Secret "secret" }
  Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (Teams.Index {}), user: Just me })
}

test "an unknown email is rejected" = {
  let _ <- Test.fake Password
  let _ <- Test.get (Auth.Login {})
  let r <- Test.submit Credentials { email: Text.Email "nobody@example.com", password: Text.Secret "secret" }
  Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: FormRejected [{ field: "", message: "Email or password is incorrect." }], user: Nothing })
}

--- file: app/Teams/Index.alder ---
module Teams.Index

import Team
import Member
import User
import Layout

page "/teams" { } requires user

type Model = { teams: List Team }

load : Params -> Session -> Query Model
load params session = {
  let rows <- Db.from Member |> Db.where (Member.user.eq session.user) |> Db.all
  let ids = List.map (\m -> m.team) rows
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
      {for t in model.teams ->
        <li><a class="name" href={Teams.Show { id: t.id }}>{t.name}</a></li>}
    </ul>
    <form submit={Create}>
      <Errors form />
      <Input field="name" label="Team name" />
      <button>Create</button>
    </form>
    <p><a href={Invites.Index {}}>Invites</a></p>
    <button click={App.Logout}>Log out</button>
  </Layout.Page>

test "creating a team makes it appear" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Teams.Index {})
  let _ <- Test.submit NewTeam { name: Text.NonEmpty "Alpha" }
  let page <- Test.get (Teams.Index {})
  Task.succeed (Test.texts "#teams .name" page.doc == ["Alpha"])
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
      let rows <- Db.from Member |> Db.where (Member.team.eq team.id) |> Db.all
      let ownerRow <- Db.get User team.owner
      let users <-
        Db.from User
          |> Db.where (User.id.in (List.map (\m -> m.user) rows))
          |> Db.orderBy User.name.asc
          |> Db.all
      let isMember = List.any (\m -> m.user == session.user) rows
      match ownerRow {
        Nothing -> Query.notFound,
        Just owner ->
          if team.owner == session.user || isMember then
            Query.succeed {
              team: team,
              owner: owner,
              members: List.filter (\u -> u.id != team.owner) users,
              isOwner: team.owner == session.user,
            }
          else Query.notFound,
      }
    },
  }
}

form NewInvite = { email: Text.Email }

msg SendInvite form NewInvite
msg Remove { user: User.UserId }

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  SendInvite p ->
    if model.isOwner then {
      let _ <- Db.insert Invite { team: model.team.id, email: p.form.email }
      Task.succeed (Redirect (Teams.Show { id: model.team.id }))
    } else Task.notFound,
  Remove p ->
    if not model.isOwner || p.user == model.owner.id then Task.notFound else {
      let rows <- Db.from Member |> Db.where (Member.team.eq model.team.id) |> Db.where (Member.user.eq p.user) |> Db.all
      let _ <- Task.all (List.map (\m -> Db.delete Member m.id) rows)
      Task.succeed (Redirect (Teams.Show { id: model.team.id }))
    },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title={model.team.name}>
    <h1 id="name">{model.team.name}</h1>
    <ul id="members">
      <li><span class="name">{model.owner.name}</span></li>
      {for u in model.members ->
        <li>
          <span class="name">{u.name}</span>
          {if model.isOwner then <button class="remove" click={Remove { user: u.id }}>Remove</button> else Html.none}
        </li>}
    </ul>
    {if model.isOwner then
      <form submit={SendInvite}>
        <Errors form />
        <Input field="email" label="Invite by email" />
        <button>Invite</button>
      </form>
    else Html.none}
    <p><a href={Teams.Index {}}>Back</a></p>
  </Layout.Page>

test "a team's page is not found for a non-member" = {
  let a <- Db.insert User { email: Text.Email "a@example.com", name: Text.NonEmpty "A", passwordHash: "" }
  let b <- Db.insert User { email: Text.Email "b@example.com", name: Text.NonEmpty "B", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "T", owner: a }
  let _ <- Test.login { user: Just b }
  let r <- Test.open (Teams.Show { id: team })
  Task.succeed (r.answer == PageMissing)
}

test "a member cannot remove another member" = {
  let a <- Db.insert User { email: Text.Email "a@example.com", name: Text.NonEmpty "A", passwordHash: "" }
  let b <- Db.insert User { email: Text.Email "b@example.com", name: Text.NonEmpty "B", passwordHash: "" }
  let c <- Db.insert User { email: Text.Email "c@example.com", name: Text.NonEmpty "C", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "T", owner: a }
  let _ <- Db.insert Member { team: team, user: b }
  let _ <- Db.insert Member { team: team, user: c }
  let _ <- Test.login { user: Just b }
  let page <- Test.get (Teams.Show { id: team })
  let r <- Test.run (update (Remove { user: c }) page.model { user: b })
  let n <- Db.from Member |> Db.where (Member.team.eq team) |> Db.count
  Task.succeed ({ value: r.value, members: n, buttons: Test.count ".remove" page.doc } == { value: Err "not found", members: 2, buttons: 0 })
}

test "the owner removes a member but not themselves" = {
  let a <- Db.insert User { email: Text.Email "a@example.com", name: Text.NonEmpty "A", passwordHash: "" }
  let b <- Db.insert User { email: Text.Email "b@example.com", name: Text.NonEmpty "B", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "T", owner: a }
  let _ <- Db.insert Member { team: team, user: b }
  let _ <- Test.login { user: Just a }
  let page <- Test.get (Teams.Show { id: team })
  let r <- Test.click (Remove { user: b })
  let n <- Db.from Member |> Db.where (Member.team.eq team) |> Db.count
  let self <- Test.run (update (Remove { user: a }) page.model { user: a })
  Task.succeed ({ answer: r.answer, members: n, self: self.value } == { answer: Redirected (Teams.Show { id: team }), members: 0, self: Err "not found" })
}

--- file: app/Invites/Index.alder ---
module Invites.Index

import Invite
import Member
import Team
import User
import Layout

page "/invites" { } requires user

type Model = { invites: List Invite, teams: List Team }

load : Params -> Session -> Query Model
load params session = {
  let me <- Db.get User session.user
  match me {
    Nothing -> Query.notFound,
    Just u -> {
      let invites <-
        Db.from Invite
          |> Db.where (Invite.email.eq u.email)
          |> Db.where (Invite.status.eq Pending)
          |> Db.all
      let teams <- Db.from Team |> Db.where (Team.id.in (List.map (\i -> i.team) invites)) |> Db.all
      Query.succeed { invites: invites, teams: teams }
    },
  }
}

msg Accept { id: Invite.InviteId }
msg Decline { id: Invite.InviteId }

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Accept p -> match List.find (\i -> i.id == p.id) model.invites {
    Nothing -> Task.notFound,
    Just inv -> {
      let _ <- Db.update Invite inv.id (\i -> { i | status: Accepted })
      let n <- Db.from Member |> Db.where (Member.team.eq inv.team) |> Db.where (Member.user.eq session.user) |> Db.count
      let team <- Db.get Team inv.team
      let ownsIt = Maybe.map (\t -> t.owner == session.user) team == Just True
      let _ <-
        if n == 0 && not ownsIt
        then Task.map (\x -> ()) (Db.insert Member { team: inv.team, user: session.user })
        else Task.succeed ()
      Task.succeed (Redirect (Teams.Show { id: inv.team }))
    },
  },
  Decline p -> match List.find (\i -> i.id == p.id) model.invites {
    Nothing -> Task.notFound,
    Just inv -> {
      let _ <- Db.update Invite inv.id (\i -> { i | status: Declined })
      Task.succeed (Redirect (Invites.Index {}))
    },
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Invites">
    <h1>Invites</h1>
    <ul id="invites">
      {for i in model.invites -> <Item invite={i} teams={model.teams} />}
    </ul>
    <p><a href={Teams.Index {}}>Teams</a></p>
  </Layout.Page>

Item : { invite: Invite, teams: List Team } -> Html Msg
Item props =
  <li>
    {match List.find (\t -> t.id == props.invite.team) props.teams {
      Just t -> <span class="team">{t.name}</span>,
      Nothing -> Html.none,
    }}
    <button class="accept" click={Accept { id: props.invite.id }}>Accept</button>
    <button class="decline" click={Decline { id: props.invite.id }}>Decline</button>
  </li>

test "accepting an invite makes the team visible" = {
  let a <- Db.insert User { email: Text.Email "a@example.com", name: Text.NonEmpty "A", passwordHash: "" }
  let b <- Db.insert User { email: Text.Email "b@example.com", name: Text.NonEmpty "B", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "T", owner: a }
  let inv <- Db.insert Invite { team: team, email: Text.Email "b@example.com" }
  let _ <- Test.login { user: Just b }
  let _ <- Test.get (Invites.Index {})
  let r <- Test.click (Accept { id: inv })
  let teams <- Test.get (Teams.Index {})
  let show <- Test.open (Teams.Show { id: team })
  let again <- Test.get (Invites.Index {})
  Task.succeed ({ answer: r.answer, teams: Test.texts "#teams .name" teams.doc, show: show.answer, waiting: Test.count "#invites li" again.doc } == { answer: Redirected (Teams.Show { id: team }), teams: ["T"], show: Shown, waiting: 0 })
}

test "declining an invite keeps the team closed" = {
  let a <- Db.insert User { email: Text.Email "a@example.com", name: Text.NonEmpty "A", passwordHash: "" }
  let b <- Db.insert User { email: Text.Email "b@example.com", name: Text.NonEmpty "B", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "T", owner: a }
  let inv <- Db.insert Invite { team: team, email: Text.Email "b@example.com" }
  let _ <- Test.login { user: Just b }
  let _ <- Test.get (Invites.Index {})
  let r <- Test.click (Decline { id: inv })
  let teams <- Test.get (Teams.Index {})
  let show <- Test.open (Teams.Show { id: team })
  let again <- Test.get (Invites.Index {})
  Task.succeed ({ answer: r.answer, teams: Test.count "#teams li" teams.doc, show: show.answer, waiting: Test.count "#invites li" again.doc } == { answer: Redirected (Invites.Index {}), teams: 0, show: PageMissing, waiting: 0 })
}
