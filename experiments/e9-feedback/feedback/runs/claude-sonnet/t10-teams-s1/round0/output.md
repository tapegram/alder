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
import User
import Layout

page "/teams" { } requires user

type Model = { teams: List Team }

load : Params -> Session -> Query Model
load params session = {
  let owned <- Db.from Team |> Db.where (Team.owner.eq session.user) |> Db.all
  let rows <- Db.from Member |> Db.where (Member.user.eq session.user) |> Db.all
  let joined <- Db.from Team |> Db.where (Db.or (List.map (\m -> Team.id.eq m.team) rows)) |> Db.all
  let others = List.filter (\t -> t.owner != session.user) joined
  let teams = List.sortByText (\t -> Text.trim t.name) (others ++ owned)
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
        <li><a href={Teams.Show { id: t.id }}><span class="name">{t.name}</span></a></li>}
    </ul>
    <form submit={Create}>
      <Input field="name" label="New team" />
      <Errors field="name" />
      <button>Create</button>
    </form>
  </Layout.Page>

test "teams are listed by name, owned and joined" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let _ <- Db.insert Team { name: Text.NonEmpty "Zed", owner: me }
  let alpha <- Db.insert Team { name: Text.NonEmpty "Alpha", owner: other }
  let _ <- Db.insert Team { name: Text.NonEmpty "Hidden", owner: other }
  let _ <- Db.insert Member { team: alpha, user: me }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Teams.Index {})
  Task.succeed (Test.texts "#teams .name" page.doc == ["Alpha", "Zed"])
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
      let rows <- Db.from Member |> Db.where (Member.team.eq team.id) |> Db.all
      let memberIds = List.map (\m -> m.user) rows
      if team.owner == session.user || List.member session.user memberIds then {
        let ownerRow <- Db.get User team.owner
        let users <- Db.from User |> Db.where (Db.or (List.map (\m -> User.id.eq m.user) rows)) |> Db.all
        match ownerRow {
          Nothing -> Query.notFound,
          Just owner -> {
            let others = List.filter (\u -> u.id != owner.id) users
            let sorted = List.sortByText (\u -> Text.trim u.name) others
            Query.succeed { team: team, owner: owner, members: sorted }
          },
        }
      } else Query.notFound
    },
  }
}

form NewInvite = { email: Text.Email }

msg SendInvite form NewInvite
msg Remove { user: User.UserId }

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  SendInvite p ->
    if model.team.owner != session.user then Task.notFound else {
      let existing <- Db.from Invite
        |> Db.where (Invite.team.eq model.team.id)
        |> Db.where (Invite.email.eq p.form.email)
        |> Db.where (Invite.status.eq Pending)
        |> Db.first
      match existing {
        Just i -> Task.succeed (Redirect (Teams.Show { id: model.team.id })),
        Nothing -> {
          let _ <- Db.insert Invite { team: model.team.id, email: p.form.email }
          Task.succeed (Redirect (Teams.Show { id: model.team.id }))
        },
      }
    },
  Remove p ->
    if model.team.owner != session.user then Task.notFound
    else if p.user == session.user then Task.succeed (Reject "The owner cannot be removed.")
    else {
      let rows <- Db.from Member
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
    <ul id="members">
      <li><span class="name">{model.owner.name}</span></li>
      {for u in model.members ->
        <MemberItem user={u} canRemove={session.user == model.team.owner} />}
    </ul>
    {if session.user == model.team.owner then
      <form submit={SendInvite}>
        <Errors form />
        <Input field="email" label="Invite by email" />
        <Errors field="email" />
        <button>Invite</button>
      </form>
    else Html.none}
    <a href={Teams.Index {}}>Back</a>
  </Layout.Page>

MemberItem : { user: User, canRemove: Bool } -> Html Msg
MemberItem props =
  <li>
    <span class="name">{props.user.name}</span>
    {if props.canRemove then <button class="remove" click={Remove { user: props.user.id }}>Remove</button> else Html.none}
  </li>

test "a team's page is not found for a user who is not a member" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let stranger <- Db.insert User { email: Text.Email "stranger@example.com", name: Text.NonEmpty "Stranger", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Core", owner: owner }
  let _ <- Test.login { user: Just stranger }
  let r <- Test.open (Teams.Show { id: team })
  Task.succeed (r.answer == PageMissing)
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
  Task.succeed ({ value: r.value, members: n, buttons: Test.count "button.remove" page.doc } == { value: Err "not found", members: 2, buttons: 0 })
}

test "a member cannot invite" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let a <- Db.insert User { email: Text.Email "a@example.com", name: Text.NonEmpty "A", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Core", owner: owner }
  let _ <- Db.insert Member { team: team, user: a }
  let _ <- Test.login { user: Just a }
  let page <- Test.get (Teams.Show { id: team })
  let r <- Test.run (update (SendInvite { form: { email: Text.Email "x@example.com" } }) page.model { user: a })
  let n <- Db.from Invite |> Db.count
  Task.succeed ({ value: r.value, invites: n } == { value: Err "not found", invites: 0 })
}

test "the owner removes a member but cannot remove themselves" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let b <- Db.insert User { email: Text.Email "b@example.com", name: Text.NonEmpty "B", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Core", owner: owner }
  let _ <- Db.insert Member { team: team, user: b }
  let _ <- Test.login { user: Just owner }
  let page <- Test.get (Teams.Show { id: team })
  let first <- Test.run (update (Remove { user: b }) page.model { user: owner })
  let second <- Test.run (update (Remove { user: owner }) page.model { user: owner })
  let n <- Db.from Member |> Db.where (Member.team.eq team) |> Db.count
  Task.succeed ({ buttons: Test.count "#members button.remove" page.doc, first: first.value, second: second.value, members: n } == { buttons: 1, first: Ok (Redirect (Teams.Show { id: team })), second: Ok (Reject "The owner cannot be removed."), members: 0 })
}

--- file: app/Invites/Index.alder ---
module Invites.Index

import Team
import User
import Invite
import Member
import Layout

page "/invites" { } requires user

type Model = { invites: List Invite, teams: List Team }

load : Params -> Session -> Query Model
load params session = {
  let found <- Db.get User session.user
  match found {
    Nothing -> Query.notFound,
    Just me -> {
      let invites <- Db.from Invite
        |> Db.where (Invite.email.eq me.email)
        |> Db.where (Invite.status.eq Pending)
        |> Db.all
      let teams <- Db.from Team |> Db.where (Db.or (List.map (\i -> Team.id.eq i.team) invites)) |> Db.all
      Query.succeed { invites: invites, teams: teams }
    },
  }
}

msg Accept { id: Invite.InviteId }
msg Decline { id: Invite.InviteId }

-- The invite a message names, when it is waiting and addressed to the session's user;
-- otherwise the message is a 404.
mine : Invite.InviteId -> Session -> Task Invite
mine id session = {
  let found <- Db.get Invite id
  let me <- Db.get User session.user
  match found {
    Nothing -> Task.notFound,
    Just inv -> match me {
      Nothing -> Task.notFound,
      Just u -> if inv.status == Pending && inv.email == u.email then Task.succeed inv else Task.notFound,
    },
  }
}

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Accept p -> {
    let inv <- mine p.id session
    let found <- Db.get Team inv.team
    let existing <- Db.from Member
      |> Db.where (Member.team.eq inv.team)
      |> Db.where (Member.user.eq session.user)
      |> Db.first
    match found {
      Nothing -> Task.notFound,
      Just team -> {
        let _ <- Db.update Invite inv.id (\i -> { i | status: Accepted })
        let _ <- if team.owner == session.user || Maybe.isJust existing then Task.succeed () else Task.map (\_ -> ()) (Db.insert Member { team: inv.team, user: session.user })
        Task.succeed (Redirect (Teams.Show { id: inv.team }))
      },
    }
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
    Just t -> Text.trim t.name,
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
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let t1 <- Db.insert Team { name: Text.NonEmpty "Alpha", owner: owner }
  let t2 <- Db.insert Team { name: Text.NonEmpty "Beta", owner: owner }
  let inv1 <- Db.insert Invite { team: t1, email: Text.Email "me@example.com" }
  let inv2 <- Db.insert Invite { team: t2, email: Text.Email "me@example.com" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Invites.Index {})
  let a <- Test.click (Accept { id: inv1 })
  let _ <- Test.get (Invites.Index {})
  let d <- Test.click (Decline { id: inv2 })
  let closed <- Test.open (Teams.Show { id: t2 })
  let opened <- Test.open (Teams.Show { id: t1 })
  let listed <- Test.get (Teams.Index {})
  Task.succeed ({ accepted: a.answer, declined: d.answer, remaining: Test.count "#invites li" d.doc, closed: closed.answer, opened: opened.answer, teams: Test.texts "#teams .name" listed.doc } == { accepted: Redirected (Teams.Show { id: t1 }), declined: Redirected (Invites.Index {}), remaining: 0, closed: PageMissing, opened: Shown, teams: ["Alpha"] })
}

test "an invite that is not the user's cannot be answered" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let stranger <- Db.insert User { email: Text.Email "stranger@example.com", name: Text.NonEmpty "Stranger", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Alpha", owner: owner }
  let inv <- Db.insert Invite { team: team, email: Text.Email "someone@example.com" }
  let _ <- Test.login { user: Just stranger }
  let page <- Test.get (Invites.Index {})
  let r <- Test.run (update (Accept { id: inv }) page.model { user: stranger })
  let n <- Db.from Member |> Db.count
  Task.succeed ({ value: r.value, members: n, listed: Test.count "#invites li" page.doc } == { value: Err "not found", members: 0, listed: 0 })
}
