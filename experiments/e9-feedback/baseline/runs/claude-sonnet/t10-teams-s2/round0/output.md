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

Authed : { title: Text, children: List (Html a) } -> Html a
Authed props =
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

--- file: app/Auth/Login.alder ---
module Auth.Login

import User
import Layout

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
import User
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

form NewTeam = {
  name: Text.NonEmpty,
}

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
  <Layout.Authed title="Teams">
    <h1>Teams</h1>
    <ul id="teams">
      {for t in model.teams ->
        <li><a class="name" href={Teams.Show { id: t.id }}>{t.name}</a></li>}
    </ul>
    <form submit={Create}>
      <Input field="name" label="Team name" />
      <Errors field="name" />
      <button>Create</button>
    </form>
  </Layout.Authed>

test "creating a team makes the user its owner" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Teams.Index {})
  let r <- Test.submit NewTeam { name: Text.NonEmpty "Core" }
  let page <- Test.get (Teams.Index {})
  Task.succeed ({ name: Test.text "#name" r.doc, members: Test.texts "#members .name" r.doc, teams: Test.texts "#teams .name" page.doc } == { name: Just "Core", members: ["Me"], teams: ["Core"] })
}

--- file: app/Teams/Show.alder ---
module Teams.Show

import Team
import Member
import Invite
import User
import Layout

page "/teams/{id}" { id: Team.TeamId } requires user

type Model = { team: Team, owner: User, members: List User }

load : Params -> Session -> Query Model
load params session = {
  let found <- Db.get Team params.id
  match found {
    Nothing -> Query.notFound,
    Just team -> {
      let mine <- Db.from Member |> Db.where (Member.team.eq team.id) |> Db.all
      let ownerRow <- Db.get User team.owner
      let users <-
        Db.from User
          |> Db.where (User.id.in (List.map (\m -> m.user) mine))
          |> Db.orderBy User.name.asc
          |> Db.all
      let allowed = team.owner == session.user || List.any (\m -> m.user == session.user) mine
      match ownerRow {
        Nothing -> Query.notFound,
        Just owner ->
          if allowed then
            Query.succeed { team: team, owner: owner, members: List.filter (\u -> u.id != team.owner) users }
          else
            Query.notFound,
      }
    },
  }
}

form NewInvite = {
  email: Text.Email,
}

msg SendInvite form NewInvite
msg Remove { user: User.UserId }

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  SendInvite p ->
    if model.team.owner != session.user then Task.notFound else {
      let existing <-
        Db.from Invite
          |> Db.where (Invite.team.eq model.team.id)
          |> Db.where (Invite.email.eq p.form.email)
          |> Db.where (Invite.status.eq Pending)
          |> Db.first
      let _ <- match existing {
        Just i -> Task.succeed (),
        Nothing -> {
          let _ <- Db.insert Invite { team: model.team.id, email: p.form.email }
          Task.succeed ()
        },
      }
      Task.succeed (Redirect (Teams.Show { id: model.team.id }))
    },
  Remove p ->
    if model.team.owner != session.user || p.user == model.team.owner then Task.notFound else {
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
  <Layout.Authed title={model.team.name}>
    <h1 id="name">{model.team.name}</h1>
    <ul id="members">
      <li><span class="name">{model.owner.name}</span> <span class="role">owner</span></li>
      {for m in model.members ->
        <li>
          <span class="name">{m.name}</span>
          {if model.team.owner == session.user then <button class="remove" click={Remove { user: m.id }}>Remove</button> else Html.none}
        </li>}
    </ul>
    {if model.team.owner == session.user then
      <form submit={SendInvite}>
        <Input field="email" label="Invite by email" />
        <Errors field="email" />
        <button>Invite</button>
      </form>
    else Html.none}
  </Layout.Authed>

test "a team's page is not found for a user who is not a member" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let member <- Db.insert User { email: Text.Email "member@example.com", name: Text.NonEmpty "Member", passwordHash: "" }
  let stranger <- Db.insert User { email: Text.Email "stranger@example.com", name: Text.NonEmpty "Stranger", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Core", owner: owner }
  let _ <- Db.insert Member { team: team, user: member }
  let _ <- Test.login { user: Just stranger }
  let denied <- Test.open (Teams.Show { id: team })
  let _ <- Test.login { user: Just member }
  let allowed <- Test.open (Teams.Show { id: team })
  Task.succeed ({ denied: denied.answer, allowed: allowed.answer, members: Test.texts "#members .name" allowed.doc } == { denied: PageMissing, allowed: Shown, members: ["Owner", "Member"] })
}

test "a member cannot remove another member" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let a <- Db.insert User { email: Text.Email "a@example.com", name: Text.NonEmpty "A", passwordHash: "" }
  let b <- Db.insert User { email: Text.Email "b@example.com", name: Text.NonEmpty "B", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Core", owner: owner }
  let _ <- Db.insert Member { team: team, user: a }
  let _ <- Db.insert Member { team: team, user: b }
  let _ <- Test.login { user: Just a }
  let page <- Test.get (Teams.Show { id: team })
  let r <- Test.run (update (Remove { user: b }) page.model { user: a })
  let n <- Db.from Member |> Db.where (Member.team.eq team) |> Db.count
  Task.succeed ({ value: r.value, buttons: Test.count ".remove" page.doc, members: n } == { value: Err "not found", buttons: 0, members: 2 })
}

test "a member cannot send an invite" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let a <- Db.insert User { email: Text.Email "a@example.com", name: Text.NonEmpty "A", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Core", owner: owner }
  let _ <- Db.insert Member { team: team, user: a }
  let _ <- Test.login { user: Just a }
  let page <- Test.get (Teams.Show { id: team })
  let r <- Test.run (update (SendInvite { form: { email: Text.Email "x@example.com" } }) page.model { user: a })
  let n <- Db.from Invite |> Db.count
  Task.succeed ({ value: r.value, invites: n, forms: Test.count "form" page.doc } == { value: Err "not found", invites: 0, forms: 0 })
}

test "the owner invites by email" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Core", owner: owner }
  let _ <- Test.login { user: Just owner }
  let _ <- Test.get (Teams.Show { id: team })
  let r <- Test.submit NewInvite { email: Text.Email "new@example.com" }
  let n <- Db.from Invite |> Db.where (Invite.email.eq (Text.Email "new@example.com")) |> Db.count
  Task.succeed ({ answer: r.answer, invites: n } == { answer: Redirected (Teams.Show { id: team }), invites: 1 })
}

test "the owner removes a member but cannot remove themselves" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let a <- Db.insert User { email: Text.Email "a@example.com", name: Text.NonEmpty "A", passwordHash: "" }
  let b <- Db.insert User { email: Text.Email "b@example.com", name: Text.NonEmpty "B", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Core", owner: owner }
  let _ <- Db.insert Member { team: team, user: a }
  let _ <- Db.insert Member { team: team, user: b }
  let _ <- Test.login { user: Just owner }
  let before <- Test.get (Teams.Show { id: team })
  let removed <- Test.click (Remove { user: b })
  let n1 <- Db.from Member |> Db.where (Member.team.eq team) |> Db.count
  let page <- Test.get (Teams.Show { id: team })
  let self <- Test.run (update (Remove { user: owner }) page.model { user: owner })
  let n2 <- Db.from Member |> Db.where (Member.team.eq team) |> Db.count
  Task.succeed ({ before: Test.count "#members .remove" before.doc, answer: removed.answer, after: n1, buttons: Test.count "#members .remove" page.doc, self: self.value, kept: n2 } == { before: 2, answer: Redirected (Teams.Show { id: team }), after: 1, buttons: 1, self: Err "not found", kept: 1 })
}

--- file: app/Invites/Index.alder ---
module Invites.Index

import Team
import Member
import Invite
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
      let teams <-
        Db.from Team
          |> Db.where (Team.id.in (List.map (\i -> i.team) invites))
          |> Db.all
      Query.succeed { invites: invites, teams: teams }
    },
  }
}

msg Accept { id: Invite.InviteId }
msg Decline { id: Invite.InviteId }

-- The invite a message names, when it is waiting and addressed to the
-- session's user; otherwise the message is a 404.
answerable : Invite.InviteId -> Session -> Task Invite
answerable id session = {
  let found <- Db.get Invite id
  let me <- Db.get User session.user
  let mine = Maybe.map (\u -> u.email) me
  match found {
    Just inv ->
      if inv.status == Pending && (Just inv.email) == mine then Task.succeed inv else Task.notFound,
    Nothing -> Task.notFound,
  }
}

addMember : Team -> User.UserId -> Task Unit
addMember team user =
  if team.owner == user then Task.succeed () else {
    let existing <-
      Db.from Member
        |> Db.where (Member.team.eq team.id)
        |> Db.where (Member.user.eq user)
        |> Db.first
    match existing {
      Just m -> Task.succeed (),
      Nothing -> {
        let _ <- Db.insert Member { team: team.id, user: user }
        Task.succeed ()
      },
    }
  }

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Accept p -> {
    let inv <- answerable p.id session
    let found <- Db.get Team inv.team
    match found {
      Nothing -> Task.notFound,
      Just team -> {
        let _ <- addMember team session.user
        let _ <- Db.update Invite inv.id (\i -> { i | status: Accepted })
        Task.succeed (Redirect (Teams.Show { id: inv.team }))
      },
    }
  },
  Decline p -> {
    let inv <- answerable p.id session
    let _ <- Db.update Invite inv.id (\i -> { i | status: Declined })
    Task.succeed (Redirect (Invites.Index {}))
  },
}

teamName : List Team -> Invite -> Text
teamName teams inv =
  match List.find (\t -> t.id == inv.team) teams {
    Just t -> t.name,
    Nothing -> "",
  }

view : Model -> Session -> Html Msg
view model session =
  <Layout.Authed title="Invites">
    <h1>Invites</h1>
    <ul id="invites">
      {for i in model.invites ->
        <li>
          <span class="team">{teamName model.teams i}</span>
          <button click={Accept { id: i.id }}>Accept</button>
          <button click={Decline { id: i.id }}>Decline</button>
        </li>}
    </ul>
  </Layout.Authed>

test "accepting an invite makes the team visible" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let guest <- Db.insert User { email: Text.Email "guest@example.com", name: Text.NonEmpty "Guest", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Core", owner: owner }
  let inv <- Db.insert Invite { team: team, email: Text.Email "guest@example.com" }
  let _ <- Test.login { user: Just guest }
  let before <- Test.get (Teams.Index {})
  let closed <- Test.open (Teams.Show { id: team })
  let _ <- Test.get (Invites.Index {})
  let r <- Test.click (Accept { id: inv })
  let after <- Test.get (Teams.Index {})
  let opened <- Test.open (Teams.Show { id: team })
  let waiting <- Test.get (Invites.Index {})
  Task.succeed ({ before: Test.count "#teams li" before.doc, closed: closed.answer, answer: r.answer, teams: Test.texts "#teams .name" after.doc, opened: opened.answer, waiting: Test.count "#invites li" waiting.doc } == { before: 0, closed: PageMissing, answer: Redirected (Teams.Show { id: team }), teams: ["Core"], opened: Shown, waiting: 0 })
}

test "declining an invite does not open the team" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let guest <- Db.insert User { email: Text.Email "guest@example.com", name: Text.NonEmpty "Guest", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Core", owner: owner }
  let inv <- Db.insert Invite { team: team, email: Text.Email "guest@example.com" }
  let _ <- Test.login { user: Just guest }
  let page <- Test.get (Invites.Index {})
  let r <- Test.click (Decline { id: inv })
  let teams <- Test.get (Teams.Index {})
  let opened <- Test.open (Teams.Show { id: team })
  let after <- Db.get Invite inv
  Task.succeed ({ listed: Test.count "#invites li" page.doc, answer: r.answer, waiting: Test.count "#invites li" r.doc, teams: Test.count "#teams li" teams.doc, opened: opened.answer, status: Maybe.map (\i -> i.status) after } == { listed: 1, answer: Redirected (Invites.Index {}), waiting: 0, teams: 0, opened: PageMissing, status: Just Declined })
}

test "an invite that is not the user's cannot be answered" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let guest <- Db.insert User { email: Text.Email "guest@example.com", name: Text.NonEmpty "Guest", passwordHash: "" }
  let stranger <- Db.insert User { email: Text.Email "stranger@example.com", name: Text.NonEmpty "Stranger", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Core", owner: owner }
  let inv <- Db.insert Invite { team: team, email: Text.Email "guest@example.com" }
  let _ <- Test.login { user: Just stranger }
  let page <- Test.get (Invites.Index {})
  let r <- Test.run (update (Accept { id: inv }) page.model { user: stranger })
  let n <- Db.from Member |> Db.count
  let after <- Db.get Invite inv
  Task.succeed ({ value: r.value, members: n, status: Maybe.map (\i -> i.status) after } == { value: Err "not found", members: 0, status: Just Pending })
}

test "an invite no longer waiting cannot be answered" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let guest <- Db.insert User { email: Text.Email "guest@example.com", name: Text.NonEmpty "Guest", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Core", owner: owner }
  let inv <- Db.insert Invite { team: team, email: Text.Email "guest@example.com" }
  let _ <- Test.login { user: Just guest }
  let page <- Test.get (Invites.Index {})
  let _ <- Test.click (Decline { id: inv })
  let r <- Test.run (update (Accept { id: inv }) page.model { user: guest })
  let n <- Db.from Member |> Db.count
  Task.succeed ({ value: r.value, members: n } == { value: Err "not found", members: 0 })
}
